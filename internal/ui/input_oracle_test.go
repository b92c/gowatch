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
	"github.com/b92c/gowatch/internal/filter"
	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// The promise behind the key bindings is that, with no keybindings in the
// config, every key does what it did before they existed. This test holds the
// proof: legacyHandleInput is a verbatim copy of handleInput as of commit
// 155d4ff (the last version with the keys written in the code, renamed and
// taking the dashboard as an argument), and the sweep below runs it and the
// real handleInput over a broad set of key events in every state a dashboard
// can be in, comparing what each returns and every piece of state it leaves
// behind. The one permitted difference is the new ? key, which opens the help
// page; it is checked on its own.

// legacyHandleInput is handleInput of commit 155d4ff.
func legacyHandleInput(d *Dashboard, event *tcell.EventKey) *tcell.EventKey {
	if d.searchField.HasFocus() || d.app.GetFocus() == d.searchField {
		return event
	}

	if event.Key() == tcell.KeyRune {
		switch event.Rune() {
		case 'q':
			d.app.Stop()
			return nil
		case '/', 'f':
			d.app.SetFocus(d.searchField)
			return nil
		case 'l', 'L':
			d.filterState.CycleMinLogLevel()
			return nil
		case 'd', 'D':
			d.awsViewMode = false
			return nil
		case 'a', 'A':
			d.awsViewMode = true
			return nil
		case 'w', 'W':
			d.showingAlertsModal = !d.showingAlertsModal
			if d.showingAlertsModal {
				d.pages.SwitchToPage("alerts_modal")
				d.app.SetFocus(d.alertsModal)
			} else {
				d.pages.SwitchToPage("dashboard")
				d.app.SetFocus(d.mainPanel())
			}
			return nil
		case 'c', 'C':
			docker.GetGlobalAlertEngine().ClearResolvedAlerts()
			return nil
		}
	}
	if event.Key() == tcell.KeyEscape {
		if d.showingAlertsModal {
			d.showingAlertsModal = false
			d.pages.SwitchToPage("dashboard")
			d.app.SetFocus(d.mainPanel())
			return nil
		}
		d.filterState.Clear()
		d.searchField.SetText("")
		d.app.SetFocus(d.mainPanel())
		return nil
	}
	return event
}

// oracleEvent builds a fresh key event each time, because the handlers answer
// with the event they were given and identity is part of what is compared.
type oracleEvent struct {
	new  func() *tcell.EventKey
	name string
}

// oracleEvents is every printable ASCII rune (plus Alt on the keys that matter,
// a modifier the dashboard has always ignored), a few non-ASCII runes, the special keys with
// each modifier, and Ctrl+A to Ctrl+Z.
func oracleEvents() []oracleEvent {
	var events []oracleEvent
	add := func(name string, key tcell.Key, r rune, mod tcell.ModMask) {
		events = append(events, oracleEvent{name: name, new: func() *tcell.EventKey { return tcell.NewEventKey(key, r, mod) }})
	}

	// Alt is tried on every key the dashboard binds (in the original or since)
	// and on a few that nothing binds.
	const withAlt = "qfdDaAwWcClL/?xjkgG "
	for r := rune(0x20); r <= 0x7e; r++ {
		add(fmt.Sprintf("%q", r), tcell.KeyRune, r, tcell.ModNone)
		if strings.ContainsRune(withAlt, r) {
			add(fmt.Sprintf("Alt+%q", r), tcell.KeyRune, r, tcell.ModAlt)
		}
	}
	for _, r := range []rune{'é', 'ß', '世', '😀'} {
		add(fmt.Sprintf("%q", r), tcell.KeyRune, r, tcell.ModNone)
	}

	specials := map[string]tcell.Key{
		"Esc": tcell.KeyEscape, "Enter": tcell.KeyEnter, "Tab": tcell.KeyTab, "Backtab": tcell.KeyBacktab,
		"Backspace": tcell.KeyBackspace, "Up": tcell.KeyUp, "Down": tcell.KeyDown, "Left": tcell.KeyLeft,
		"Right": tcell.KeyRight, "PgUp": tcell.KeyPgUp, "PgDn": tcell.KeyPgDn, "Home": tcell.KeyHome,
		"End": tcell.KeyEnd, "Insert": tcell.KeyInsert, "Delete": tcell.KeyDelete, "F1": tcell.KeyF1, "F12": tcell.KeyF12,
	}
	mods := map[string]tcell.ModMask{"": tcell.ModNone, "Shift+": tcell.ModShift, "Ctrl+": tcell.ModCtrl, "Alt+": tcell.ModAlt}
	for name, key := range specials {
		for modName, mod := range mods {
			add(modName+name, key, 0, mod)
		}
	}

	add("Ctrl+Space", tcell.KeyCtrlSpace, 0, tcell.ModCtrl)
	for k := tcell.KeyCtrlA; k <= tcell.KeyCtrlZ; k++ {
		add(fmt.Sprintf("Ctrl+%c", 'A'+rune(k-tcell.KeyCtrlA)), k, 0, tcell.ModCtrl)
	}
	return events
}

// oracleStates are the situations a key can arrive in. Each starts from a
// dashboard that has filters set, so Esc and l have something to change.
var oracleStates = []struct {
	setup func(*Dashboard)
	name  string
}{
	{name: "main panel", setup: func(*Dashboard) {}},
	{name: "alerts modal open", setup: func(d *Dashboard) {
		d.showingAlertsModal = true
		d.pages.SwitchToPage("alerts_modal")
		d.app.SetFocus(d.alertsModal)
	}},
	{name: "search field focused", setup: func(d *Dashboard) { d.app.SetFocus(d.searchField) }},
	{name: "daemon error page", setup: func(d *Dashboard) {
		d.daemonError = fmt.Errorf("daemon is down")
		d.pages.SwitchToPage("error")
	}},
}

// oracleLayouts: the Esc key sends the focus to the logs, or to the services
// table when the logs are hidden.
var oracleLayouts = map[string]config.LayoutConfig{
	"default layout": {},
	"logs hidden":    {Logs: config.PanelLayout{Visible: boolPtr(false)}},
}

// observation is everything a key press can change that the test looks at.
type observation struct {
	returned        string
	focus           string
	front           string
	focusAfter      string
	frontAfter      string
	searchText      string
	searchTextAfter string
	filter          filter.FilterState
	logsOffset      [2]int
	alertsSelection [2]int
	servicesOffset  [2]int
	userScrolling   bool
	aws             bool
	alertsModal     bool
	quit            bool
	resolvedCleared bool
}

// oracleDashboard builds a dashboard in the given state, with a simulated
// screen attached so that quitting is observable: Stop finalizes the screen,
// which then reports a size of 0x0.
func oracleDashboard(opts Options, aws bool, setup func(*Dashboard)) (*Dashboard, tcell.SimulationScreen) {
	d := NewDashboardWithOptions(opts)
	d.logsView.SetChangedFunc(nil)
	screen := tcell.NewSimulationScreen("")
	d.app.SetScreen(screen)
	d.SetupInputCapture()
	d.ApplyInitialFilters("web", []string{"running"}, metrics.LogLevelWarn)
	d.awsViewMode = aws

	setup(d)
	return d, screen
}

func focusName(d *Dashboard) string {
	switch d.app.GetFocus() {
	case d.logsView:
		return "logs"
	case d.servicesTable:
		return "services"
	case d.searchField:
		return "search"
	case d.alertsModal:
		return "alerts"
	case d.helpTable:
		return "help"
	case d.resourcesView:
		return "resources"
	case d.grid:
		return "grid"
	}
	return fmt.Sprintf("%T", d.app.GetFocus())
}

// observe sends the event through handle, then delivers it to the primitive
// with focus the way the application loop does when handle lets it through, and
// reports what changed.
func observe(d *Dashboard, screen tcell.SimulationScreen, handle func(*Dashboard, *tcell.EventKey) *tcell.EventKey, event *tcell.EventKey, resolvedID string) observation {
	var o observation

	ret := handle(d, event)
	switch {
	case ret == nil:
		o.returned = "consumed"
	case ret == event:
		o.returned = "passed on"
	default:
		o.returned = "replaced"
	}
	o.aws, o.alertsModal = d.awsViewMode, d.showingAlertsModal
	o.filter, o.searchText = d.filterState, d.searchField.GetText()
	o.focus = focusName(d)
	o.front, _ = d.pages.GetFrontPage()
	w, h := screen.Size()
	o.quit = w == 0 && h == 0
	o.resolvedCleared = !alertIsPresent(resolvedID)

	// The application hands a key that was not consumed to the root, which passes
	// it down the chain of containers (none of which has an input capture) to
	// the primitive with focus; deliver it straight to that primitive.
	if focus := d.app.GetFocus(); ret != nil && focus != nil {
		focus.InputHandler()(ret, func(p tview.Primitive) { d.app.SetFocus(p) })
	}
	o.searchTextAfter = d.searchField.GetText()
	o.focusAfter = focusName(d)
	o.frontAfter, _ = d.pages.GetFrontPage()
	o.userScrolling = d.userScrolling
	row, col := d.logsView.GetScrollOffset()
	o.logsOffset = [2]int{row, col}
	row, col = d.alertsModal.GetSelection()
	o.alertsSelection = [2]int{row, col}
	row, col = d.servicesTable.GetOffset()
	o.servicesOffset = [2]int{row, col}
	return o
}

// seedResolvedAlert puts a resolved alert for a fresh container into the global
// alert engine, so that clearing the resolved alerts can be observed, and
// returns the container ID.
func seedResolvedAlert(t *testing.T) string {
	t.Helper()
	id := fmt.Sprintf("oracle-%d", time.Now().UnixNano())
	engine := docker.GetGlobalAlertEngine()
	engine.ClearResolvedAlerts() // keeps the engine small over thousands of seeds
	host := metrics.HostInfo{CPUCount: 1, MemTotal: 1 << 30}
	engine.Evaluate([]alert.ContainerEvaluatorData{{ID: id, Service: id, State: "dead"}}, host)
	engine.Evaluate([]alert.ContainerEvaluatorData{{ID: id, Service: id, State: "running"}}, host)
	if status, ok := alertStatus(id); !ok || status != alert.StatusResolved {
		t.Fatalf("could not seed a resolved alert for %s (found %v, status %q)", id, ok, status)
	}
	return id
}

func alertStatus(containerID string) (alert.AlertStatus, bool) {
	for _, a := range docker.GetGlobalAlertEngine().GetActiveAlertsSnapshot() {
		if a.ContainerID == containerID {
			return a.Status, true
		}
	}
	return "", false
}

func alertIsPresent(containerID string) bool {
	_, ok := alertStatus(containerID)
	return ok
}

// isHelpKeyEvent tells the events that press ?, with or without Alt.
func isHelpKeyEvent(name string) bool { return name == "'?'" || name == "Alt+'?'" }

func TestDefaultKeymapMatchesTheOriginalHandleInput(t *testing.T) {
	restoreStyles(t)

	// What the original did with each key that it intercepted at all.
	legacyConsumed := map[string]bool{}
	for _, name := range []string{
		"'q'", "'/'", "'f'", "'l'", "'L'", "'d'", "'D'", "'a'", "'A'", "'w'", "'W'", "'c'", "'C'",
		"Esc", "Shift+Esc", "Ctrl+Esc", "Alt+Esc",
		"Alt+'q'", "Alt+'/'", "Alt+'f'", "Alt+'l'", "Alt+'L'", "Alt+'d'", "Alt+'D'", "Alt+'a'", "Alt+'A'", "Alt+'w'", "Alt+'W'", "Alt+'c'", "Alt+'C'",
	} {
		legacyConsumed[name] = true
	}

	events := oracleEvents()
	compared, consumedByLegacy := 0, 0
	seenConsumed := map[string]bool{}

	for layoutName, layout := range oracleLayouts {
		for _, state := range oracleStates {
			for _, aws := range []bool{false, true} {
				if aws && layoutName != "default layout" {
					continue // which view is showing does not interact with the layout
				}
				for _, ev := range events {
					if isHelpKeyEvent(ev.name) && state.name != "search field focused" {
						continue // the one new key, checked by TestHelpKeyIsTheOnlyDifferenceFromTheOriginal
					}
					opts := Options{Layout: layout}
					resolvedLegacy := seedResolvedAlert(t)
					legacy, legacyScreen := oracleDashboard(opts, aws, state.setup)
					want := observe(legacy, legacyScreen, legacyHandleInput, ev.new(), resolvedLegacy)

					resolvedNew := seedResolvedAlert(t)
					current, currentScreen := oracleDashboard(opts, aws, state.setup)
					got := observe(current, currentScreen, (*Dashboard).handleInput, ev.new(), resolvedNew)

					where := fmt.Sprintf("%s, %s, aws view %v, key %s", layoutName, state.name, aws, ev.name)
					compared++
					if want.returned == "consumed" {
						consumedByLegacy++
						seenConsumed[ev.name] = true
					}
					if !reflect.DeepEqual(got, want) {
						t.Errorf("%s:\nhandleInput:        %+v\noriginal behaviour: %+v", where, got, want)
					}
				}
			}
		}
	}

	// The sweep must not be vacuous: the original consumed these keys, in some
	// state at least, and nothing else.
	for name := range legacyConsumed {
		if !seenConsumed[name] {
			t.Errorf("the original never consumed %s in the sweep, the test is not exercising it", name)
		}
	}
	for name := range seenConsumed {
		if !legacyConsumed[name] {
			t.Errorf("the original consumed %s, which the test does not list as a bound key", name)
		}
	}
	if compared < 2500 || consumedByLegacy == 0 {
		t.Errorf("compared %d events, %d consumed by the original: the sweep is too small", compared, consumedByLegacy)
	}
	t.Logf("compared %d (layout, state, view, key) combinations; the original consumed the key in %d of them", compared, consumedByLegacy)
}

// The one key whose behavior is new. The original let ? through everywhere; the
// help page now takes it (unless the search field has focus, where it is typed
// like any other character, which the sweep above already compares).
func TestHelpKeyIsTheOnlyDifferenceFromTheOriginal(t *testing.T) {
	restoreStyles(t)
	questions := map[string]func() *tcell.EventKey{
		"?":     func() *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, '?', tcell.ModNone) },
		"Alt+?": func() *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, '?', tcell.ModAlt) },
	}

	for _, state := range oracleStates {
		if state.name == "search field focused" {
			continue
		}
		for keyName, question := range questions {
			t.Run(state.name+", "+keyName, func(t *testing.T) {
				legacy, legacyScreen := oracleDashboard(Options{}, false, state.setup)
				before := observe(legacy, legacyScreen, legacyHandleInput, question(), "")
				if before.returned != "passed on" {
					t.Fatalf("the original no longer ignores ?, got %q", before.returned)
				}

				d, _ := oracleDashboard(Options{}, false, state.setup)
				beforeFilter, beforeAWS, beforeModal := d.filterState, d.awsViewMode, d.showingAlertsModal

				if ret := d.handleInput(question()); ret != nil {
					t.Errorf("? should be consumed, got %v", ret)
				}
				if !d.showingHelp {
					t.Error("? should open the help page")
				}
				if front, _ := d.pages.GetFrontPage(); front != helpPage {
					t.Errorf("front page = %q, want %q", front, helpPage)
				}
				if got := focusName(d); got != "help" {
					t.Errorf("focus = %q, want the help page", got)
				}
				if !reflect.DeepEqual(d.filterState, beforeFilter) || d.awsViewMode != beforeAWS || d.showingAlertsModal != beforeModal {
					t.Error("opening the help page must not change any other state")
				}
			})
		}
	}

	t.Run("search field focused: ? is typed, as before", func(t *testing.T) {
		search := oracleStates[2]
		legacy, legacyScreen := oracleDashboard(Options{}, false, search.setup)
		current, currentScreen := oracleDashboard(Options{}, false, search.setup)
		want := observe(legacy, legacyScreen, legacyHandleInput, questions["?"](), "")
		got := observe(current, currentScreen, (*Dashboard).handleInput, questions["?"](), "")
		if !reflect.DeepEqual(got, want) || got.searchTextAfter != "web?" {
			t.Errorf("got %+v\nwant %+v (with the search text %q)", got, want, "web?")
		}
	})
}
