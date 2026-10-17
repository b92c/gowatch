package ui

import (
	"testing"
	"time"

	"github.com/b92c/gowatch/internal/alert"
	"github.com/b92c/gowatch/internal/trace"
	"github.com/gdamore/tcell/v2"
)

// shown is the color the terminal gets for a role. The two legacy markers of
// the dark theme stand for tags that tview draws in the default foreground.
func shown(c tcell.Color) tcell.Color {
	if c == colorLegacyCyan || c == colorLegacyMagenta {
		return tcell.ColorDefault
	}
	return c
}

// Every built-in theme has to reach the screen: the cells of each panel carry
// the colors of the theme's roles, whether they were set directly on a cell, in
// a tview tag or through tview's global styles.
func TestEveryThemeIsDrawnWithItsColors(t *testing.T) {
	root := &trace.Span{OperationName: "GET /users", ServiceName: "api"}
	containers := sampleContainers()
	containers.Traces = []trace.Trace{{TraceID: "abc", RootSpan: root, Spans: []*trace.Span{root}, Duration: time.Second}}

	for name, theme := range BuiltinThemes() {
		t.Run(name, func(t *testing.T) {
			d := newTestDashboard(t, Options{Theme: &theme})
			d.Update(containers)
			shot := drawDashboard(t, d, 160, 40)

			checks := []struct {
				text   string
				rowOf  string // when set, text is looked up on the row that contains this one
				fg, bg tcell.Color
				bgAny  bool // the background is not part of the check
			}{
				// Panel titles and the table.
				{text: "Docker Services", fg: theme.Title, bg: theme.Background},
				{text: "State", fg: theme.Accent, bg: theme.Background},
				{text: "┼", fg: theme.Border, bg: theme.Background},
				{text: "running", fg: theme.StatusOK, bg: theme.Background},
				{text: "exited", fg: theme.StatusError, bg: theme.Background},
				{text: "nginx:latest", fg: theme.Info, bg: theme.Background},
				{text: "12.50%", fg: theme.Text, bgAny: true},
				{text: "0 lines", fg: theme.Muted, bg: theme.Background},
				// System resources: label, status and sub-heading come from tags.
				{text: "CPU Cores (Total):", fg: theme.Accent, bg: theme.Background},
				{text: "OK", fg: theme.StatusOK, bg: theme.Background},
				{text: "Recent Traces:", fg: shown(theme.Subheading), bg: theme.Background},
				{text: "Updated:", fg: theme.Muted, bg: theme.Background},
				{text: "16.00 GB", fg: theme.Text, bg: theme.Background},
				// Logs: the service name, then the text in the service's palette color.
				{text: "web", rowOf: "listening", fg: theme.Accent, bg: theme.Background}, // the log line, not the table cell
				{text: "listening", fg: shown(theme.ServicePalette[0]), bg: theme.Background},
				{text: "disk full", fg: shown(theme.ServicePalette[1]), bgAny: true}, // a FATAL line leaves its badge background on
				// Help bar.
				{text: "/", fg: theme.HelpKey, bg: theme.HelpBarBg},
				{text: "Docker View", fg: theme.HelpText, bg: theme.HelpBarBg},
				// Search field: tview reads these from its global styles.
				{text: "Search:", fg: theme.Accent, bgAny: true},
				{text: "name, id or image...", fg: theme.FieldHint, bg: theme.FieldBg},
			}
			for _, c := range checks {
				x, y, ok := shot.find(c.text)
				if c.rowOf != "" {
					_, y, ok = shot.find(c.rowOf)
					x = indexInRow(shot, y, c.text)
				}
				if c.text == "/" { // the help bar is the last row
					x, y, ok = indexInRow(shot, shot.height-1, "/"), shot.height-1, true
				}
				if c.text == "OK" { // the first "OK" is the alert summary
					x, y, ok = shot.find("Alerts: OK")
					x += len("Alerts: ")
				}
				if !ok || x < 0 {
					t.Errorf("%q is not on screen", c.text)
					continue
				}
				fg, bg, _ := shot.cell(x, y).Style.Decompose()
				if fg != c.fg {
					t.Errorf("%q foreground = %v, want %v", c.text, fg, c.fg)
				}
				if !c.bgAny && bg != c.bg {
					t.Errorf("%q background = %v, want %v", c.text, bg, c.bg)
				}
			}

			// A border cell: the top-left corner of the services table.
			if fg, bg, _ := shot.cell(0, 0).Style.Decompose(); fg != theme.Border || bg != theme.Background {
				t.Errorf("table border = %v on %v, want %v on %v", fg, bg, theme.Border, theme.Background)
			}
		})
	}
}

// indexInRow is the column of the first occurrence of text in row y, or -1.
func indexInRow(s screenshot, y int, text string) int {
	row := s.row(y)
	for x := 0; x+len(text) <= len(row); x++ {
		if string(row[x:x+len(text)]) == text {
			return x
		}
	}
	return -1
}

// The alerts modal and the error page are drawn from the same theme.
func TestThemeColorsReachTheModalAndTheErrorPage(t *testing.T) {
	containers := sampleContainers()
	// The modal selects its first row, which tview draws inverted, so the rows
	// whose colors are checked come after it.
	containers.Alerts = []alert.Alert{
		{Status: alert.StatusAcknowledged, Severity: alert.SeverityWarning, ServiceName: "api", Message: "selected row", FiredAt: time.Unix(0, 0)},
		{Status: alert.StatusFiring, Severity: alert.SeverityCritical, ServiceName: "web", Message: "high cpu", FiredAt: time.Unix(0, 0)},
		{Status: alert.StatusResolved, Severity: alert.SeverityWarning, ServiceName: "db", Message: "slow", FiredAt: time.Unix(0, 0)},
	}

	for name, theme := range BuiltinThemes() {
		t.Run(name, func(t *testing.T) {
			d := newTestDashboard(t, Options{Theme: &theme})
			d.Update(containers)

			d.pages.SwitchToPage("alerts_modal")
			modal := drawDashboard(t, d, 160, 20)
			for _, c := range []struct {
				text string
				want tcell.Color
			}{
				{"FIRING", theme.StatusError},
				{"CRIT", theme.StatusError},
				{"RESOLVED", theme.StatusOK},
				{"Severity", theme.Accent},
				{"high cpu", theme.Text},
			} {
				if fg, _ := modal.colorsOf(t, c.text); fg != c.want {
					t.Errorf("modal %q = %v, want %v", c.text, fg, c.want)
				}
			}
			if fg, _ := modal.colorsOnRowOf(t, "RESOLVED", "WARN"); fg != theme.StatusWarn {
				t.Errorf("modal severity WARN = %v, want %v", fg, theme.StatusWarn)
			}
			if fg, _, _ := modal.cell(0, 0).Style.Decompose(); fg != theme.StatusError {
				t.Errorf("modal border = %v, want %v", fg, theme.StatusError)
			}
			if _, bg := modal.colorsOf(t, "Severity"); bg != theme.Background {
				t.Errorf("modal background = %v, want %v", bg, theme.Background)
			}

			d.daemonErrView.Clear()
			if _, err := d.daemonErrView.Write([]byte(daemonErrorText(theme, errTest("socket closed")))); err != nil {
				t.Fatal(err)
			}
			d.pages.SwitchToPage("error")
			page := drawDashboard(t, d, 160, 30)
			for _, c := range []struct {
				text string
				want tcell.Color
			}{
				{"Docker Daemon is not running", theme.StatusError},
				{"GoWatch requires", theme.Text},
				{"How to start Docker:", theme.Muted},
				{"macOS:", theme.Accent},
				{"Retrying connection", theme.Faint},
				{"Quit", theme.HelpKey},
				{"Waiting for Docker daemon...", theme.Muted},
			} {
				if fg, _ := page.colorsOf(t, c.text); fg != c.want {
					t.Errorf("error page %q = %v, want %v", c.text, fg, c.want)
				}
			}
			if _, bg := page.colorsOf(t, "Quit"); bg != theme.HelpBarBg {
				t.Errorf("error help bar background = %v, want %v", bg, theme.HelpBarBg)
			}
		})
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
