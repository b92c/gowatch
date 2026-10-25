package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/b92c/gowatch/internal/alert"
	"github.com/b92c/gowatch/internal/config"
	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/gdamore/tcell/v2"
)

func mustKeymap(t *testing.T, cfg config.KeybindingsConfig) Keymap {
	t.Helper()
	km, err := BuildKeymap(cfg)
	if err != nil {
		t.Fatalf("BuildKeymap() error = %v", err)
	}
	return km
}

// newKeyedDashboard builds a dashboard with the keymap of cfg, a simulated
// screen attached (so that quitting shows: Stop finalizes the screen, which
// then reports a size of 0x0) and the search field wired up as Run does.
func newKeyedDashboard(t *testing.T, cfg config.KeybindingsConfig, layout config.LayoutConfig) (*Dashboard, tcell.SimulationScreen) {
	t.Helper()
	km := mustKeymap(t, cfg)
	d := newTestDashboard(t, Options{Keymap: &km, Layout: layout})
	screen := tcell.NewSimulationScreen("")
	d.app.SetScreen(screen)
	d.SetupInputCapture()
	return d, screen
}

func quitRequested(screen tcell.SimulationScreen) bool {
	w, h := screen.Size()
	return w == 0 && h == 0
}

func TestHandleInputWithRemappedQuit(t *testing.T) {
	d, screen := newKeyedDashboard(t, config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"x"}}}, config.LayoutConfig{})

	q := runeEvent('q')
	if got := d.handleInput(q); got != q {
		t.Errorf("q after quit was remapped: handleInput = %v, want the event passed on", got)
	}
	if quitRequested(screen) {
		t.Fatal("q quit the dashboard although quit is bound to x")
	}

	if got := d.handleInput(runeEvent('x')); got != nil {
		t.Errorf("x: handleInput = %v, want the event consumed", got)
	}
	if !quitRequested(screen) {
		t.Error("x did not quit the dashboard")
	}
}

func TestHandleInputRunsRemappedActions(t *testing.T) {
	d, _ := newKeyedDashboard(t, config.KeybindingsConfig{Bindings: map[string][]string{
		"aws_view":        {"v"},
		"docker_view":     {"b"},
		"cycle_log_level": {"m"},
		"toggle_alerts":   {"x"},
		"search":          {"s"},
		"clear_filter":    {"z"},
	}}, config.LayoutConfig{})
	d.ApplyInitialFilters("web", nil, metrics.LogLevelUnknown)

	press := func(r rune) *tcell.EventKey { return d.handleInput(runeEvent(r)) }

	if press('v') != nil || !d.awsViewMode {
		t.Error("v should show the AWS view")
	}
	if press('b') != nil || d.awsViewMode {
		t.Error("b should show the Docker view")
	}
	press('a')
	if d.awsViewMode {
		t.Error("a is no longer the AWS view key")
	}

	if press('m') != nil || d.filterState.MinLogLevel != metrics.LogLevelInfo {
		t.Errorf("m should cycle the log level, now %v", d.filterState.MinLogLevel)
	}
	press('l')
	if d.filterState.MinLogLevel != metrics.LogLevelInfo {
		t.Error("l is no longer the log level key")
	}

	if press('x') != nil || !d.showingAlertsModal || d.app.GetFocus() != d.alertsModal {
		t.Error("x should open the alerts list")
	}
	if press('z') != nil || d.showingAlertsModal {
		t.Error("the clear_filter key should close the alerts list first")
	}
	if d.filterState.SearchText != "web" {
		t.Error("closing the alerts list must not clear the filters")
	}
	press('z')
	if d.filterState.Active || d.searchField.GetText() != "" {
		t.Error("the clear_filter key should clear the filters when the alerts list is closed")
	}
	if d.app.GetFocus() != d.mainPanel() {
		t.Errorf("focus = %T, want the main panel", d.app.GetFocus())
	}

	press('s')
	if d.app.GetFocus() != d.searchField {
		t.Error("s should focus the search field")
	}
	// Esc was bound to clear_filter by the preset and is not any more.
	d.app.SetFocus(d.logsView)
	esc := tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)
	if got := d.handleInput(esc); got != esc {
		t.Errorf("Esc after clear_filter moved to z: handleInput = %v, want it passed on", got)
	}
}

// Whatever the keymap, while the search field has focus every key is typed into
// it, the new ones included.
func TestSearchFieldGuardHoldsForEveryBoundKey(t *testing.T) {
	configs := map[string]config.KeybindingsConfig{
		"default":  {},
		"vim":      {Preset: "vim"},
		"remapped": {Bindings: map[string][]string{"quit": {"x"}, "help": {"F"}, "search": {"s"}}},
	}
	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			d, screen := newKeyedDashboard(t, cfg, config.LayoutConfig{})
			d.app.SetFocus(d.searchField)

			for k := range d.keymap.lookup {
				var event *tcell.EventKey
				if k.Key == tcell.KeyRune {
					event = runeEvent(k.Rune)
				} else {
					event = keyEvent(k.Key)
				}
				if got := d.handleInput(event); got != event {
					t.Errorf("key %s was intercepted with the search field focused", k)
				}
			}
			if d.showingHelp || d.showingAlertsModal || d.awsViewMode || quitRequested(screen) {
				t.Error("a key acted on the dashboard while the search field had focus")
			}
		})
	}
}

// The keys the panels scroll with are never taken over, whatever the keymap.
func TestScrollKeysAreAlwaysHandedToThePanel(t *testing.T) {
	configs := map[string]config.KeybindingsConfig{
		"default": {},
		"vim":     {Preset: "vim"},
		"scroll keys taken away from their actions": {Bindings: map[string][]string{
			"scroll_up": {"k"}, "scroll_down": {"j"}, "page_up": {"u"}, "page_down": {"p"},
			"scroll_top": {"g"}, "scroll_bottom": {"G"},
		}},
	}
	natives := []tcell.Key{tcell.KeyUp, tcell.KeyDown, tcell.KeyPgUp, tcell.KeyPgDn, tcell.KeyHome, tcell.KeyEnd, tcell.KeyCtrlF, tcell.KeyCtrlB}
	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			d, _ := newKeyedDashboard(t, cfg, config.LayoutConfig{})
			for _, k := range natives {
				ev := keyEvent(k)
				if got := d.handleInput(ev); got != ev {
					t.Errorf("%v was not handed to the panel with focus", k)
				}
			}
			if d.userScrolling {
				t.Error("handleInput must leave userScrolling to the logs panel")
			}
		})
	}
}

// fillLogs gives the dashboard n log lines, "line 001" to "line n", draws it
// and returns the height of the logs panel.
func fillLogs(t *testing.T, d *Dashboard, n int) int {
	t.Helper()
	containers := sampleContainers()
	containers.FlatLogs = nil
	for i := 1; i <= n; i++ {
		containers.FlatLogs = append(containers.FlatLogs, docker.FormattedLog{Service: "web", Line: fmt.Sprintf("line %03d", i), Level: metrics.LogLevelInfo})
	}
	d.Update(containers)
	drawDashboard(t, d, 120, 40)
	_, _, _, h := d.logsView.GetInnerRect()
	if h < 5 {
		t.Fatalf("logs panel is only %d rows high", h)
	}
	return h
}

func TestScrollActionsMoveTheLogs(t *testing.T) {
	d, _ := newKeyedDashboard(t, config.KeybindingsConfig{Preset: "vim"}, config.LayoutConfig{})
	height := fillLogs(t, d, 100)

	row := func() int { r, _ := d.logsView.GetScrollOffset(); return r }
	press := func(ev *tcell.EventKey) {
		t.Helper()
		if got := d.handleInput(ev); got != nil {
			t.Fatalf("%v was not consumed", ev.Key())
		}
	}

	// The logs start out following the newest line, so the first draw put the
	// view at its last possible row.
	end := row()
	if end < 2*height || d.userScrolling {
		t.Fatalf("before scrolling: row %d (the logs hold 100 lines in a %d row panel), userScrolling %v", end, height, d.userScrolling)
	}

	press(runeEvent('k'))
	if row() != end-1 || !d.userScrolling {
		t.Errorf("k: row %d (want %d), userScrolling %v", row(), end-1, d.userScrolling)
	}
	press(runeEvent('j'))
	if row() != end || !d.userScrolling {
		t.Errorf("j: row %d (want %d)", row(), end)
	}

	press(runeEvent('g'))
	if row() != 0 || !d.userScrolling {
		t.Errorf("g: row %d (want 0), userScrolling %v", row(), d.userScrolling)
	}
	press(runeEvent('k'))
	if row() != 0 {
		t.Errorf("k at the top: row %d, want 0", row())
	}
	press(keyEvent(tcell.KeyCtrlD))
	if row() != height {
		t.Errorf("Ctrl+D: row %d, want one page (%d)", row(), height)
	}
	press(keyEvent(tcell.KeyCtrlU))
	if row() != 0 {
		t.Errorf("Ctrl+U: row %d, want 0", row())
	}
	press(runeEvent('j'))
	press(runeEvent('j'))
	if row() != 2 {
		t.Errorf("jj: row %d, want 2", row())
	}

	d.userScrolling = true
	press(runeEvent('G'))
	if d.userScrolling {
		t.Error("G should go back to following the newest line")
	}
	drawDashboard(t, d, 120, 40)
	if row() != end {
		t.Errorf("G then draw: row %d, want the end (%d)", row(), end)
	}
}

// G takes the logs back to following new lines: after it, an update scrolls to
// the end again, which it does not while the user is reading older lines.
func TestScrollBottomResumesFollowing(t *testing.T) {
	d, _ := newKeyedDashboard(t, config.KeybindingsConfig{Preset: "vim"}, config.LayoutConfig{})
	const total = 100
	fillLogs(t, d, total)
	end := func() int { r, _ := d.logsView.GetScrollOffset(); return r }()

	more := func(n int) {
		containers := sampleContainers()
		containers.FlatLogs = nil
		for i := 1; i <= n; i++ {
			containers.FlatLogs = append(containers.FlatLogs, docker.FormattedLog{Service: "web", Line: fmt.Sprintf("line %03d", i), Level: metrics.LogLevelInfo})
		}
		d.Update(containers)
		drawDashboard(t, d, 120, 40)
	}
	row := func() int { r, _ := d.logsView.GetScrollOffset(); return r }

	d.handleInput(runeEvent('k'))
	d.handleInput(runeEvent('k'))
	held := row()
	more(total + 10)
	if row() != held {
		t.Fatalf("an update moved the view to row %d while the user was reading at row %d", row(), held)
	}

	d.handleInput(runeEvent('G'))
	more(total + 20)
	if want := end + 20; row() != want {
		t.Errorf("after G and an update the view is at row %d, want the end (%d)", row(), want)
	}
	more(total + 30)
	if want := end + 30; row() != want {
		t.Errorf("the view stopped following: row %d, want %d", row(), want)
	}
}

func TestScrollActionsWithLogsHiddenDoNothing(t *testing.T) {
	hidden := config.LayoutConfig{Logs: config.PanelLayout{Visible: boolPtr(false)}}
	d, _ := newKeyedDashboard(t, config.KeybindingsConfig{Preset: "vim"}, hidden)
	d.Update(sampleContainers())

	for _, ev := range []*tcell.EventKey{runeEvent('j'), runeEvent('k'), runeEvent('g'), runeEvent('G'), keyEvent(tcell.KeyCtrlD), keyEvent(tcell.KeyCtrlU)} {
		if got := d.handleInput(ev); got != nil {
			t.Errorf("%v was not consumed", ev.Key())
		}
	}
	if d.userScrolling {
		t.Error("scrolling hidden logs must not mark the user as scrolling")
	}
	if r, c := d.servicesTable.GetOffset(); r != 0 || c != 0 {
		t.Errorf("the services table moved to %d,%d", r, c)
	}
}

// With the alerts list open, the scroll keys move the list, not the logs behind
// it.
func TestScrollActionsMoveTheAlertsListWhenItIsOpen(t *testing.T) {
	d, _ := newKeyedDashboard(t, config.KeybindingsConfig{Preset: "vim"}, config.LayoutConfig{})
	containers := sampleContainers()
	for i := range 6 {
		containers.Alerts = append(containers.Alerts, alert.Alert{
			Status: alert.StatusFiring, Severity: alert.SeverityWarning, ServiceName: fmt.Sprintf("svc%d", i),
			Message: "m", FiredAt: time.Unix(0, 0),
		})
	}
	d.Update(containers)
	d.handleInput(runeEvent('w'))
	if !d.showingAlertsModal {
		t.Fatal("w did not open the alerts list")
	}
	start, _ := d.alertsModal.GetSelection()
	logsRow, _ := d.logsView.GetScrollOffset()

	d.handleInput(runeEvent('j'))
	d.handleInput(runeEvent('j'))
	if row, _ := d.alertsModal.GetSelection(); row != start+2 {
		t.Errorf("after jj the selected alert is row %d, want %d", row, start+2)
	}
	d.handleInput(runeEvent('k'))
	if row, _ := d.alertsModal.GetSelection(); row != start+1 {
		t.Errorf("after k the selected alert is row %d, want %d", row, start+1)
	}
	d.handleInput(runeEvent('G'))
	if row, _ := d.alertsModal.GetSelection(); row != 6 {
		t.Errorf("after G the selected alert is row %d, want the last (6)", row)
	}
	d.handleInput(runeEvent('g'))
	if row, _ := d.alertsModal.GetSelection(); row != 1 {
		t.Errorf("after g the selected alert is row %d, want the first (1)", row)
	}
	if got, _ := d.logsView.GetScrollOffset(); got != logsRow || d.userScrolling {
		t.Error("the logs behind the alerts list were scrolled")
	}
}

func TestClearResolvedAlertsKeyActsOnEveryScreen(t *testing.T) {
	for _, state := range oracleStates {
		for _, key := range []rune{'c', 'C'} {
			t.Run(fmt.Sprintf("%s, %c", state.name, key), func(t *testing.T) {
				d, _ := newKeyedDashboard(t, config.KeybindingsConfig{}, config.LayoutConfig{})
				state.setup(d)
				id := seedResolvedAlert(t)

				got := d.handleInput(runeEvent(key))

				if state.name == "search field focused" {
					if got == nil || !alertIsPresent(id) {
						t.Error("the key must be typed into the search field and clear nothing")
					}
					return
				}
				if got != nil || alertIsPresent(id) {
					t.Errorf("consumed = %v, resolved alert still present = %v, want the alert cleared", got == nil, alertIsPresent(id))
				}
			})
		}
	}

	t.Run("a remapped key clears them", func(t *testing.T) {
		d, _ := newKeyedDashboard(t, config.KeybindingsConfig{Bindings: map[string][]string{"clear_resolved_alerts": {"r"}}}, config.LayoutConfig{})
		id := seedResolvedAlert(t)
		d.handleInput(runeEvent('c'))
		if !alertIsPresent(id) {
			t.Fatal("c cleared the alert although the action moved to r")
		}
		d.handleInput(runeEvent('r'))
		if alertIsPresent(id) {
			t.Error("r did not clear the resolved alert")
		}
	})
}

// --- the help page -------------------------------------------------------

func TestHelpPageOpensAndCloses(t *testing.T) {
	d, screen := newKeyedDashboard(t, config.KeybindingsConfig{}, config.LayoutConfig{})
	state := func() (bool, string) {
		front, _ := d.pages.GetFrontPage()
		return d.showingHelp, front
	}

	if open, front := state(); open || front != "dashboard" {
		t.Fatalf("initial state: help %v, front page %q", open, front)
	}
	if got := d.handleInput(runeEvent('?')); got != nil {
		t.Errorf("? was not consumed: %v", got)
	}
	if open, front := state(); !open || front != helpPage || d.app.GetFocus() != d.helpTable {
		t.Errorf("after ?: help %v, front page %q, focus %T", open, front, d.app.GetFocus())
	}
	d.closeHelp()

	for _, tt := range []struct {
		ev   *tcell.EventKey
		name string
	}{
		{name: "Esc", ev: tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)},
		{name: "?", ev: runeEvent('?')},
		{name: "q", ev: runeEvent('q')},
	} {
		t.Run("closes with "+tt.name, func(t *testing.T) {
			d.handleInput(runeEvent('?'))
			if open, _ := state(); !open {
				t.Fatal("help did not open")
			}
			if got := d.handleInput(tt.ev); got != nil {
				t.Errorf("%s was not consumed while help is open: %v", tt.name, got)
			}
			if open, front := state(); open || front != "dashboard" {
				t.Errorf("after %s: help %v, front page %q", tt.name, open, front)
			}
			if d.app.GetFocus() != d.mainPanel() {
				t.Errorf("focus after closing = %T, want the main panel", d.app.GetFocus())
			}
			if quitRequested(screen) {
				t.Errorf("%s closed the help page but also quit the dashboard", tt.name)
			}
		})
	}
}

func TestHelpPageClosesWithTheRemappedKeys(t *testing.T) {
	d, screen := newKeyedDashboard(t, config.KeybindingsConfig{Bindings: map[string][]string{
		"help": {"h", "F"}, "quit": {"x", "Z"},
	}}, config.LayoutConfig{})

	for _, r := range []rune{'?', 'q', 'x', 'Z', 'h', 'F'} {
		d.handleInput(runeEvent('h'))
		if !d.showingHelp {
			t.Fatalf("h did not open the help page before %q", r)
		}
		d.handleInput(runeEvent(r))
		switch r {
		case '?', 'q': // no longer bound to anything: ignored by the page
			if !d.showingHelp {
				t.Errorf("%q closed the help page, but it is bound to nothing", r)
			}
			d.handleInput(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
		default:
			if d.showingHelp {
				t.Errorf("%q should close the help page", r)
			}
		}
		if quitRequested(screen) {
			t.Fatalf("%q quit the dashboard while help was open", r)
		}
	}
}

// While the help page is open nothing behind it may change, whichever page is
// under it: the dashboard or the alerts list. Every key is tried on a fresh
// dashboard so that one key cannot hide another (a and d undo each other), with
// both views as the starting point.
func TestNoOtherActionRunsWhileHelpIsOpen(t *testing.T) {
	type keyCase struct {
		event *tcell.EventKey
		name  string
	}
	keysFor := func(preset string) []keyCase {
		var cases []keyCase
		for _, r := range "aAdDwWcClL/f" {
			cases = append(cases, keyCase{event: runeEvent(r), name: string(r)})
		}
		if preset == "vim" {
			for _, r := range "gGjk" {
				cases = append(cases, keyCase{event: runeEvent(r), name: string(r)})
			}
			cases = append(cases, keyCase{event: keyEvent(tcell.KeyCtrlD), name: "Ctrl+D"}, keyCase{event: keyEvent(tcell.KeyCtrlU), name: "Ctrl+U"})
		}
		return cases
	}

	for _, preset := range []string{"default", "vim"} {
		for _, underModal := range []bool{false, true} {
			for _, aws := range []bool{false, true} {
				where := fmt.Sprintf("%s preset, help over the %s, aws view %v", preset, map[bool]string{false: "dashboard", true: "alerts list"}[underModal], aws)
				t.Run(where, func(t *testing.T) {
					for _, kc := range keysFor(preset) {
						d, screen := newKeyedDashboard(t, config.KeybindingsConfig{Preset: preset}, config.LayoutConfig{})
						d.ApplyInitialFilters("web", []string{"running"}, metrics.LogLevelWarn)
						d.awsViewMode = aws
						resolved := seedResolvedAlert(t)
						containers := sampleContainers()
						containers.Alerts = []alert.Alert{
							{Status: alert.StatusFiring, Severity: alert.SeverityWarning, ServiceName: "a", Message: "m", FiredAt: time.Unix(0, 0)},
							{Status: alert.StatusFiring, Severity: alert.SeverityWarning, ServiceName: "b", Message: "m", FiredAt: time.Unix(0, 0)},
						}
						if preset == "vim" { // only its keys scroll the logs, which then need many lines and a drawn view
							containers.FlatLogs = logsOf(50).FlatLogs
						}
						d.Update(containers)
						if preset == "vim" {
							drawDashboard(t, d, 120, 40)
						}
						if underModal {
							d.handleInput(runeEvent('w'))
						}
						d.handleInput(runeEvent('?'))
						if !d.showingHelp || d.showingAlertsModal != underModal {
							t.Fatalf("%s: setup went wrong: help %v, alerts list %v", where, d.showingHelp, d.showingAlertsModal)
						}
						filters := d.filterState
						logsRow, logsCol := d.logsView.GetScrollOffset()
						selRow, selCol := d.alertsModal.GetSelection()

						if got := d.handleInput(kc.event); got != nil {
							t.Errorf("%s, key %s: passed on while help is open", where, kc.name)
						}

						if !d.showingHelp {
							t.Errorf("%s, key %s: the help page closed", where, kc.name)
						}
						if d.awsViewMode != aws {
							t.Errorf("%s, key %s: the view changed behind the help page", where, kc.name)
						}
						if d.showingAlertsModal != underModal {
							t.Errorf("%s, key %s: the alerts list is showing %v, want %v", where, kc.name, d.showingAlertsModal, underModal)
						}
						if front, _ := d.pages.GetFrontPage(); front != helpPage {
							t.Errorf("%s, key %s: front page = %q, want the help page", where, kc.name, front)
						}
						if !reflect.DeepEqual(d.filterState, filters) {
							t.Errorf("%s, key %s: the filters changed behind the help page: %+v", where, kc.name, d.filterState)
						}
						if !alertIsPresent(resolved) {
							t.Errorf("%s, key %s: the resolved alerts were cleared behind the help page", where, kc.name)
						}
						if d.app.GetFocus() != d.helpTable {
							t.Errorf("%s, key %s: focus = %T, want the help page (/ must not open the search)", where, kc.name, d.app.GetFocus())
						}
						if row, col := d.logsView.GetScrollOffset(); row != logsRow || col != logsCol || d.userScrolling {
							t.Errorf("%s, key %s: the logs scrolled behind the help page", where, kc.name)
						}
						if row, col := d.alertsModal.GetSelection(); row != selRow || col != selCol {
							t.Errorf("%s, key %s: the alerts list moved behind the help page", where, kc.name)
						}
						if quitRequested(screen) {
							t.Errorf("%s, key %s: the dashboard quit while the help page was open", where, kc.name)
						}

						// Esc closes the page without clearing the filters (as clear_filter
						// would) and gives back what was under it.
						d.handleInput(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
						wantFocus := d.mainPanel()
						if underModal {
							wantFocus = d.alertsModal
						}
						if d.showingHelp || !d.filterState.Active || d.showingAlertsModal != underModal || d.app.GetFocus() != wantFocus {
							t.Errorf("%s, key %s: after Esc: help open %v, filters active %v, alerts list %v, focus %T; want the page closed, the filters kept and the focus back on %T",
								where, kc.name, d.showingHelp, d.filterState.Active, d.showingAlertsModal, d.app.GetFocus(), wantFocus)
						}
					}
				})
			}
		}
	}
}

func TestHelpPageKeepsTheKeysItDoesNotOwn(t *testing.T) {
	d, _ := newKeyedDashboard(t, config.KeybindingsConfig{}, config.LayoutConfig{})
	d.handleInput(runeEvent('?'))

	for _, ev := range []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModCtrl), // the application quits on it
		keyEvent(tcell.KeyUp), keyEvent(tcell.KeyDown), keyEvent(tcell.KeyPgUp), keyEvent(tcell.KeyPgDn),
		keyEvent(tcell.KeyHome), keyEvent(tcell.KeyEnd), runeEvent('z'),
	} {
		if got := d.handleInput(ev); got != ev {
			t.Errorf("%v should reach the primitive with focus untouched, got %v", ev.Key(), got)
		}
	}
}

func TestHelpOverTheAlertsList(t *testing.T) {
	d, _ := newKeyedDashboard(t, config.KeybindingsConfig{}, config.LayoutConfig{})
	d.handleInput(runeEvent('w'))
	if !d.showingAlertsModal {
		t.Fatal("w did not open the alerts list")
	}

	d.handleInput(runeEvent('?'))
	if front, _ := d.pages.GetFrontPage(); front != helpPage {
		t.Errorf("front page = %q, want the help page over the alerts list", front)
	}
	if !d.showingAlertsModal {
		t.Error("opening help closed the alerts list")
	}
	if got := d.pages.GetPageNames(true); len(got) != 2 {
		t.Errorf("visible pages = %v, want the alerts list and the help page", got)
	}

	d.handleInput(runeEvent('?'))
	if front, _ := d.pages.GetFrontPage(); front != "alerts_modal" || d.app.GetFocus() != d.alertsModal {
		t.Errorf("after closing help: front page %q, focus %T; want the alerts list back", front, d.app.GetFocus())
	}
	if !d.showingAlertsModal {
		t.Error("the alerts list is no longer marked as showing")
	}

	// The alerts list works as before: w closes it, and w while help is open does not.
	d.handleInput(runeEvent('?'))
	d.handleInput(runeEvent('w'))
	if !d.showingAlertsModal {
		t.Error("w closed the alerts list behind the help page")
	}
	d.handleInput(runeEvent('?'))
	d.handleInput(runeEvent('w'))
	if d.showingAlertsModal {
		t.Error("w should close the alerts list")
	}
	if d.app.GetFocus() != d.mainPanel() {
		t.Errorf("focus = %T, want the main panel", d.app.GetFocus())
	}
}

func TestHelpFocusReturnsToTheServicesTableWhenLogsAreHidden(t *testing.T) {
	hidden := config.LayoutConfig{Logs: config.PanelLayout{Visible: boolPtr(false)}}
	d, _ := newKeyedDashboard(t, config.KeybindingsConfig{}, hidden)
	d.handleInput(runeEvent('?'))
	d.handleInput(runeEvent('?'))
	if d.app.GetFocus() != d.servicesTable {
		t.Errorf("focus after help = %T, want the services table", d.app.GetFocus())
	}
}

func TestHelpScrollActionsMoveThePage(t *testing.T) {
	d, _ := newKeyedDashboard(t, config.KeybindingsConfig{Preset: "vim"}, config.LayoutConfig{})
	d.handleInput(runeEvent('?'))
	drawPrimitive(t, d.pages, 80, 12) // short enough that the page has to scroll

	row := func() int { r, _ := d.helpTable.GetOffset(); return r }
	d.handleInput(runeEvent('j'))
	d.handleInput(runeEvent('j'))
	if row() != 2 {
		t.Errorf("jj: help page at row %d, want 2", row())
	}
	d.handleInput(runeEvent('k'))
	if row() != 1 {
		t.Errorf("k: help page at row %d, want 1", row())
	}
	d.handleInput(runeEvent('G'))
	drawPrimitive(t, d.pages, 80, 12) // the table follows its end when drawn
	if row() < 10 {
		t.Errorf("G: help page at row %d, want the bottom", row())
	}
	d.handleInput(runeEvent('g'))
	if row() != 0 {
		t.Errorf("g: help page at row %d, want the top", row())
	}
	d.handleInput(keyEvent(tcell.KeyCtrlD))
	if row() == 0 {
		t.Error("Ctrl+D did not page down")
	}
	d.handleInput(keyEvent(tcell.KeyCtrlU))
	if row() != 0 {
		t.Errorf("Ctrl+U: help page at row %d, want 0", row())
	}

	// Reopening starts at the top again.
	d.handleInput(runeEvent('j'))
	d.handleInput(runeEvent('?'))
	d.handleInput(runeEvent('?'))
	if row() != 0 {
		t.Errorf("reopened help page at row %d, want the top", row())
	}
}

func TestHelpPageIsRebuiltPerKeymap(t *testing.T) {
	def := newTestDashboard(t, Options{})
	km := mustKeymap(t, config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"x"}}})
	custom := newTestDashboard(t, Options{Keymap: &km})

	if def.helpTable == custom.helpTable {
		t.Fatal("dashboards share a help table")
	}
	if !strings.Contains(drawPrimitive(t, custom.helpTable, 100, 40).text(), "x ") {
		t.Error("the help page of the remapped dashboard does not show x")
	}
}
