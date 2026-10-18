package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/internal/filter"
	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/gdamore/tcell/v2"
)

// TestMain runs the package's tests in an empty temporary directory, so that a
// test that presses the export key without choosing a directory cannot leave
// files in the source tree.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gowatch-ui-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Chdir(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

var exportTime = time.Date(2026, 10, 17, 15, 30, 45, 0, time.Local)

func sampleExportLogs() []docker.FormattedLog {
	return []docker.FormattedLog{
		{Service: "api", Line: "[ERROR] boom <&> \"quoted\"", Level: metrics.LogLevelError},
		{Service: "web", Line: "plain line", Level: metrics.LogLevelUnknown},
	}
}

func readExport(t *testing.T, path string) (map[string]any, string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, raw)
	}
	return doc, string(raw)
}

func TestWriteLogsExportContent(t *testing.T) {
	dir := t.TempDir()
	path, err := writeLogsExport(dir, exportTime, "api", metrics.LogLevelWarn, sampleExportLogs())
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "gowatch-logs-20261017-153045.json"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "exported_at": "` + exportTime.Format(time.RFC3339) + `",
  "filters": {
    "search": "api",
    "min_log_level": "WARN"
  },
  "count": 2,
  "logs": [
    {
      "service": "api",
      "level": "ERROR",
      "line": "[ERROR] boom <&> \"quoted\""
    },
    {
      "service": "web",
      "line": "plain line"
    }
  ]
}
`
	if string(raw) != want {
		t.Errorf("content:\n%s\nwant:\n%s", raw, want)
	}

	var back logsExport
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Count != 2 || len(back.Logs) != 2 || back.Logs[0].Line != sampleExportLogs()[0].Line || back.Filters.Search != "api" {
		t.Errorf("round trip lost data: %+v", back)
	}
}

func TestWriteLogsExportEmptyLogsIsAnEmptyArray(t *testing.T) {
	for name, logs := range map[string][]docker.FormattedLog{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			path, err := writeLogsExport(t.TempDir(), exportTime, "", metrics.LogLevelUnknown, logs)
			if err != nil {
				t.Fatal(err)
			}
			doc, raw := readExport(t, path)
			if !strings.Contains(raw, `"logs": []`) {
				t.Errorf(`want "logs": [] in:\n%s`, raw)
			}
			if doc["count"] != float64(0) {
				t.Errorf("count = %v, want 0", doc["count"])
			}
			filters := doc["filters"].(map[string]any)
			if level, has := filters["min_log_level"]; !has || level != "" {
				t.Errorf("an unset minimum level must be an empty string, got %v (present %v)", level, has)
			}
			if filters["search"] != "" {
				t.Errorf("search = %v, want an empty string", filters["search"])
			}
		})
	}
}

func TestWriteLogsExportUnknownLevelIsOmitted(t *testing.T) {
	path, err := writeLogsExport(t.TempDir(), exportTime, "", metrics.LogLevelUnknown, sampleExportLogs())
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := readExport(t, path)
	logs := doc["logs"].([]any)
	if first := logs[0].(map[string]any); first["level"] != "ERROR" {
		t.Errorf("first level = %v", first["level"])
	}
	if second := logs[1].(map[string]any); second["level"] != nil {
		t.Errorf("the unknown level must be left out, got %v", second)
	}
}

func TestWriteLogsExportNameCollisionAddsASuffix(t *testing.T) {
	dir := t.TempDir()
	var names []string
	for range 3 {
		path, err := writeLogsExport(dir, exportTime, "", metrics.LogLevelUnknown, sampleExportLogs())
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, filepath.Base(path))
	}
	want := []string{"gowatch-logs-20261017-153045.json", "gowatch-logs-20261017-153045-1.json", "gowatch-logs-20261017-153045-2.json"}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names = %v, want %v", names, want)
			break
		}
	}
}

func TestWriteLogsExportKeepsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "gowatch-logs-20261017-153045.json")
	writeTestFile(t, existing, "precious")
	if _, err := writeLogsExport(dir, exportTime, "", metrics.LogLevelUnknown, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(existing); string(got) != "precious" {
		t.Errorf("an existing file was overwritten: %q", got)
	}
}

func TestWriteLogsExportFileMode(t *testing.T) {
	path, err := writeLogsExport(t.TempDir(), exportTime, "", metrics.LogLevelUnknown, nil)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %o, want 600", mode)
	}
}

func TestWriteLogsExportErrors(t *testing.T) {
	t.Run("missing directory", func(t *testing.T) {
		if _, err := writeLogsExport(filepath.Join(t.TempDir(), "nope"), exportTime, "", metrics.LogLevelUnknown, nil); err == nil {
			t.Error("expected an error")
		}
	})
	t.Run("directory is not writable", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can write anywhere")
		}
		dir := t.TempDir()
		if err := os.Chmod(dir, 0o500); err != nil { // #nosec G302 -- a directory that must be searchable but not writable
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) // #nosec G302 -- lets the test directory be removed
		if _, err := writeLogsExport(dir, exportTime, "", metrics.LogLevelUnknown, nil); err == nil {
			t.Error("expected an error")
		}
	})
	t.Run("a file is not a directory", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "file")
		writeTestFile(t, file, "x")
		if _, err := writeLogsExport(file, exportTime, "", metrics.LogLevelUnknown, nil); err == nil {
			t.Error("expected an error")
		}
	})
}

func exportFilesIn(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "gowatch-logs-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// The export keys are the second difference from the original handleInput, next
// to ?: the original let e and E through everywhere, and now they export. In the
// search field they are typed, which the sweep in input_oracle_test.go compares.
func TestExportKeysAreTheOtherDifferenceFromTheOriginal(t *testing.T) {
	restoreStyles(t)
	keys := map[string]func() *tcell.EventKey{
		"e":     func() *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, 'e', tcell.ModNone) },
		"E":     func() *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, 'E', tcell.ModNone) },
		"Alt+e": func() *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, 'e', tcell.ModAlt) },
		"Alt+E": func() *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, 'E', tcell.ModAlt) },
	}

	for _, state := range oracleStates {
		if state.name == "search field focused" {
			continue
		}
		for keyName, key := range keys {
			t.Run(state.name+", "+keyName, func(t *testing.T) {
				dir := t.TempDir()
				t.Chdir(dir)

				legacy, legacyScreen := oracleDashboard(Options{}, false, state.setup)
				if before := observe(legacy, legacyScreen, legacyHandleInput, key(), ""); before.returned != "passed on" {
					t.Fatalf("the original no longer ignores %s, got %q", keyName, before.returned)
				}
				if got := exportFilesIn(t, dir); len(got) != 0 {
					t.Fatalf("the original handler wrote files: %v", got)
				}

				d, _ := oracleDashboard(Options{}, false, state.setup)
				beforeFilter, beforeAWS, beforeModal := d.filterState, d.awsViewMode, d.showingAlertsModal
				beforeFocus, beforeFront := focusName(d), ""
				beforeFront, _ = d.pages.GetFrontPage()

				if ret := d.handleInput(key()); ret != nil {
					t.Errorf("%s should be consumed, got %v", keyName, ret)
				}
				if got := exportFilesIn(t, dir); len(got) != 1 {
					t.Errorf("want one export file, got %v", got)
				}
				front, _ := d.pages.GetFrontPage()
				if focusName(d) != beforeFocus || front != beforeFront || d.awsViewMode != beforeAWS || d.showingAlertsModal != beforeModal {
					t.Error("exporting must not change the focus, the page or the view")
				}
				if !reflect.DeepEqual(d.filterState, beforeFilter) {
					t.Error("exporting must not change the filters")
				}
			})
		}
	}

	t.Run("search field focused: e is typed, nothing is exported", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		search := oracleStates[2]
		d, _ := oracleDashboard(Options{}, false, search.setup)
		if ret := d.handleInput(keys["e"]()); ret == nil {
			t.Error("e must go on to the search field")
		}
		if got := exportFilesIn(t, dir); len(got) != 0 {
			t.Errorf("exported while typing: %v", got)
		}
	})
}

func TestExportKeysAreInTheDefaultKeymapOnly(t *testing.T) {
	for _, r := range "eE" {
		if action, ok := DefaultKeymap().Lookup(runeEvent(r)); !ok || action != ActionExportLogs {
			t.Errorf("%q = %q (bound %v), want export_logs", r, action, ok)
		}
	}
	if got := ActionExportLogs.Group(); got == "" {
		t.Error("export_logs has no help group")
	}
}

func TestExportWritesWhatLastPassedTheFilters(t *testing.T) {
	restoreStyles(t)
	dir := t.TempDir()
	t.Chdir(dir)

	d := NewDashboard()
	d.logsView.SetChangedFunc(nil)
	d.filterState.SetSearch("slow")
	d.filterState.SetMinLogLevel(metrics.LogLevelWarn)
	c := sampleContainers()
	c.C[0].Log = []string{"[INFO] slow start", "[WARN] slow query"}
	d.Update(c)

	d.exportLogs()
	files := exportFilesIn(t, dir)
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	doc, _ := readExport(t, files[0])
	if doc["count"] != float64(1) {
		t.Errorf("count = %v, want 1: %v", doc["count"], doc)
	}
	filters := doc["filters"].(map[string]any)
	if filters["search"] != "slow" || filters["min_log_level"] != "WARN" {
		t.Errorf("filters = %v", filters)
	}
}

func TestExportWithTheLogsPanelHiddenDoesNotPanic(t *testing.T) {
	restoreStyles(t)
	t.Chdir(t.TempDir())
	d := NewDashboardWithOptions(oracleLayoutsHidden())
	d.Update(sampleContainers())
	d.exportLogs() // the help bar may be hidden too: no panic either way
}

func oracleLayoutsHidden() Options {
	return Options{Layout: oracleLayouts["logs hidden"]}
}

func TestExportDescribesTheSnapshotThatProducedTheLines(t *testing.T) {
	restoreStyles(t)
	dir := t.TempDir()
	t.Chdir(dir)

	d := NewDashboard()
	d.logsView.SetChangedFunc(nil)
	d.filterState.SetSearch("slow")
	d.filterState.SetMinLogLevel(metrics.LogLevelWarn)
	c := sampleContainers()
	c.C[0].Log = []string{"[WARN] slow query"}
	d.Update(c)

	// The filters change, but the screen still shows the old lines.
	d.filterState.SetSearch("other")
	d.filterState.SetMinLogLevel(metrics.LogLevelError)
	d.exportLogs()

	files := exportFilesIn(t, dir)
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	doc, _ := readExport(t, files[0])
	filters := doc["filters"].(map[string]any)
	if filters["search"] != "slow" || filters["min_log_level"] != "WARN" {
		t.Errorf("filters = %v, want the ones of the update that produced the lines", filters)
	}
}

func TestLogsOfTextOnlyMatchesKeepTheServiceColor(t *testing.T) {
	restoreStyles(t)
	d := NewDashboard()
	all := sampleContainers() // web is first, db second
	all.C[1].Log = []string{"[ERROR] needle here"}
	d.filterState.SetSearch("needle")

	d.Update(all)
	filtered := filterForTest(all, d)
	if len(filtered.C) != 0 {
		t.Fatalf("the db container should not be listed: %v", filtered.C)
	}
	want := colorName(d.theme.ServicePalette[1%len(d.theme.ServicePalette)])
	if got := d.getServiceColor("db", filtered); got != want {
		t.Errorf("color of a text-only match = %q, want its palette color %q", got, want)
	}
	if got := d.getServiceColor("web", filtered); got != colorName(d.theme.ServicePalette[0]) {
		t.Errorf("color of web = %q", got)
	}
	// A listed service keeps the color of its position in the filtered list.
	d.filterState.SetSearch("db")
	d.Update(all)
	if got := d.getServiceColor("db", filterForTest(all, d)); got != colorName(d.theme.ServicePalette[0]) {
		t.Errorf("a name match keeps its position in the filtered list, got %q", got)
	}
}

func filterForTest(c docker.Containers, d *Dashboard) docker.Containers {
	return filter.FilterContainers(c, d.filterState)
}
