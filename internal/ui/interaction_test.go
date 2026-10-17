package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/b92c/gowatch/internal/alert"
	"github.com/b92c/gowatch/internal/config"
	"github.com/gdamore/tcell/v2"
)

// These tests drive the dashboard the way a person does: the real tview
// application runs on a simulated terminal, keys are injected as the terminal
// would deliver them, and what is checked is what ends up on the screen.

const helpTitleOnScreen = "Keyboard Shortcuts"

func liveWithKeys(t *testing.T, cfg config.KeybindingsConfig) *liveDashboard {
	t.Helper()
	km := mustKeymap(t, cfg)
	return startLive(t, Options{Keymap: &km}, 150, 40)
}

func TestLiveHelpPageOpensAndClosesWithTheKeyboard(t *testing.T) {
	l := liveWithKeys(t, config.KeybindingsConfig{})
	l.update(sampleContainers())

	if !strings.Contains(l.shot().text(), "↑↓ Scroll | ? Help | q Quit") {
		t.Fatalf("the help bar does not offer the help page:\n%s", l.shot().text())
	}
	if strings.Contains(l.shot().text(), helpTitleOnScreen) {
		t.Fatal("the help page is open at startup")
	}

	l.pressRune('?')
	shot := l.shot()
	for _, want := range []string{helpTitleOnScreen, "General", "Navigation", "Quit GoWatch", "Ctrl+C", "Always available"} {
		if !strings.Contains(shot.text(), want) {
			t.Errorf("the help page does not show %q:\n%s", want, shot.text())
		}
	}

	l.pressKey(tcell.KeyEscape)
	if strings.Contains(l.shot().text(), helpTitleOnScreen) {
		t.Errorf("Esc did not close the help page:\n%s", l.shot().text())
	}
	if !strings.Contains(l.shot().text(), "Docker Services") {
		t.Error("the dashboard is not back after the help page closed")
	}

	l.pressRune('?')
	l.pressRune('?')
	if strings.Contains(l.shot().text(), helpTitleOnScreen) {
		t.Error("? did not close the help page")
	}

	// q closes the page without quitting; the next q quits.
	l.pressRune('?')
	l.pressRune('q')
	if !l.running() || strings.Contains(l.shot().text(), helpTitleOnScreen) {
		t.Fatalf("q with the help page open: running %v; want the page closed and the dashboard still running", l.running())
	}
	l.pressAndQuit(tcell.KeyRune, 'q', tcell.ModNone)
}

func TestLiveHelpPageBlocksTheOtherKeys(t *testing.T) {
	l := liveWithKeys(t, config.KeybindingsConfig{})
	l.update(sampleContainers())

	l.pressRune('?')
	for _, r := range "aAwWdDlLc/f" {
		l.pressRune(r)
	}
	l.do(func() {
		d := l.d
		if d.awsViewMode || d.showingAlertsModal || d.filterState.MinLogLevel != 0 {
			t.Errorf("a key acted behind the help page: aws %v, alerts %v, level %v", d.awsViewMode, d.showingAlertsModal, d.filterState.MinLogLevel)
		}
		if d.app.GetFocus() != d.helpTable {
			t.Errorf("focus = %T, want the help page", d.app.GetFocus())
		}
	})
	if !strings.Contains(l.shot().text(), helpTitleOnScreen) {
		t.Error("the help page closed")
	}

	// Ctrl+C always quits, help or not.
	l.pressAndQuit(tcell.KeyCtrlC, 0, tcell.ModCtrl)
}

func TestLiveHelpPageOverTheAlertsList(t *testing.T) {
	l := liveWithKeys(t, config.KeybindingsConfig{})
	containers := sampleContainers()
	containers.Alerts = []alert.Alert{{Status: alert.StatusFiring, Severity: alert.SeverityCritical, ServiceName: "web", Message: "high cpu"}}
	l.update(containers)

	l.pressRune('w')
	if !strings.Contains(l.shot().text(), "Active & Recent Alerts") {
		t.Fatalf("the alerts list did not open:\n%s", l.shot().text())
	}
	l.pressRune('?')
	if !strings.Contains(l.shot().text(), helpTitleOnScreen) {
		t.Fatal("the help page did not open over the alerts list")
	}
	l.pressKey(tcell.KeyEscape)
	text := l.shot().text()
	if strings.Contains(text, helpTitleOnScreen) || !strings.Contains(text, "Active & Recent Alerts") || !strings.Contains(text, "high cpu") {
		t.Errorf("closing the help page should show the alerts list again:\n%s", text)
	}
	l.pressRune('w')
	if strings.Contains(l.shot().text(), "Active & Recent Alerts") {
		t.Error("w did not close the alerts list")
	}
}

func TestLiveQuestionMarkIsTypedInTheSearchField(t *testing.T) {
	l := liveWithKeys(t, config.KeybindingsConfig{})
	l.pressRune('/')
	l.typeText("what?")
	l.do(func() {
		if got := l.d.searchField.GetText(); got != "what?" {
			t.Errorf("search text = %q, want %q", got, "what?")
		}
		if l.d.showingHelp {
			t.Error("? opened the help page while typing in the search field")
		}
	})
	l.pressKey(tcell.KeyEscape)
	l.pressRune('?')
	if !strings.Contains(l.shot().text(), helpTitleOnScreen) {
		t.Error("? should open the help page once the search field has been left")
	}
}

func TestLiveVimScrolling(t *testing.T) {
	l := liveWithKeys(t, config.KeybindingsConfig{Preset: "vim"})
	l.update(logsOf(100))

	visible := func(line string) bool { _, _, ok := l.shot().find(line); return ok }
	if !visible("line 100") || visible("line 001") {
		t.Fatalf("the logs should start at the newest line:\n%s", l.shot().text())
	}

	// The last row of the logs view is the empty line after the final newline,
	// so it takes two lines up to push "line 100" off the screen.
	l.pressRune('k')
	l.pressRune('k')
	if visible("line 100") || !visible("line 099") {
		t.Errorf("k should scroll up a line at a time:\n%s", l.shot().text())
	}
	l.pressRune('j')
	l.pressRune('j')
	if !visible("line 100") {
		t.Errorf("j should scroll back down:\n%s", l.shot().text())
	}

	l.pressRune('g')
	if !visible("line 001") || visible("line 100") {
		t.Errorf("g should go to the top:\n%s", l.shot().text())
	}
	var height int
	l.do(func() { _, _, _, height = l.d.logsView.GetInnerRect() })
	l.pressKey(tcell.KeyCtrlD)
	if first := height + 1; visible("line 001") || !visible(lineNumber(first)) {
		t.Errorf("Ctrl+D should page down to line %d:\n%s", first, l.shot().text())
	}
	l.pressKey(tcell.KeyCtrlU)
	if !visible("line 001") {
		t.Errorf("Ctrl+U should page back up:\n%s", l.shot().text())
	}

	// G follows the newest line again, including lines that arrive later.
	l.pressRune('G')
	if !visible("line 100") {
		t.Fatalf("G should go to the bottom:\n%s", l.shot().text())
	}
	l.update(logsOf(110))
	if !visible("line 110") {
		t.Errorf("after G the logs should keep following new lines:\n%s", l.shot().text())
	}

	// Scrolling away stops the following, and G resumes it.
	l.pressRune('k')
	l.pressRune('k')
	l.update(logsOf(120))
	if visible("line 120") {
		t.Errorf("an update pulled the view back down while the user was reading:\n%s", l.shot().text())
	}
	l.pressRune('G')
	if !visible("line 120") {
		t.Errorf("G should resume following:\n%s", l.shot().text())
	}
	l.update(logsOf(125))
	if !visible("line 125") {
		t.Errorf("the logs stopped following after G:\n%s", l.shot().text())
	}
}

func lineNumber(n int) string {
	return "line " + string([]byte{byte('0' + n/100), byte('0' + n/10%10), byte('0' + n%10)})
}

func TestLiveNativeScrollKeysStillWork(t *testing.T) {
	for _, preset := range []string{"default", "vim"} {
		t.Run(preset, func(t *testing.T) {
			l := liveWithKeys(t, config.KeybindingsConfig{Preset: preset})
			l.update(logsOf(100))
			visible := func(line string) bool { _, _, ok := l.shot().find(line); return ok }

			l.pressKey(tcell.KeyUp)
			l.pressKey(tcell.KeyUp) // the last row of the view is empty: it takes two to hide line 100
			if visible("line 100") || !visible("line 099") {
				t.Errorf("Up should scroll the logs:\n%s", l.shot().text())
			}
			l.pressKey(tcell.KeyHome)
			if !visible("line 001") {
				t.Errorf("Home should go to the top:\n%s", l.shot().text())
			}
			l.pressKey(tcell.KeyPgDn)
			if visible("line 001") {
				t.Errorf("PgDn should scroll a page:\n%s", l.shot().text())
			}
			l.pressKey(tcell.KeyEnd)
			if !visible("line 100") {
				t.Errorf("End should go to the bottom:\n%s", l.shot().text())
			}

			// End is a key of the logs panel, which marks the user as scrolling on any
			// key it gets, so it does not make the logs follow new lines again. The
			// help page and the README say so; only a scroll_bottom key that is not
			// native (G in vim) follows (TestLiveVimScrolling).
			l.update(logsOf(110))
			if visible("line 110") {
				t.Errorf("End made the logs follow new lines, but the documentation says only a non-native scroll_bottom key does:\n%s", l.shot().text())
			}
		})
	}
}

func TestLiveRemappedQuit(t *testing.T) {
	l := liveWithKeys(t, config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"x"}}})

	text := l.shot().text()
	if !strings.Contains(text, "x Quit") || strings.Contains(text, "q Quit") {
		t.Errorf("the help bar should offer x to quit:\n%s", text)
	}

	l.pressRune('q')
	if !l.running() {
		t.Fatal("q quit the dashboard although quit is bound to x")
	}

	l.pressRune('?')
	row := lineOf(t, l.shot(), "Quit GoWatch")
	if keys, _, _ := strings.Cut(row, "Quit GoWatch"); !strings.Contains(keys, "x") || strings.Contains(keys, "q") {
		t.Errorf("the help page row of quit = %q, want the key x", strings.TrimSpace(row))
	}
	if !strings.Contains(l.shot().text(), "(?, Esc, x to close)") {
		t.Errorf("the help page title should list x as a closing key:\n%s", l.shot().text())
	}
	l.pressRune('x') // closes the page
	if !l.running() || strings.Contains(l.shot().text(), helpTitleOnScreen) {
		t.Fatalf("x with the help page open should only close it (running %v)", l.running())
	}

	l.pressAndQuit(tcell.KeyRune, 'x', tcell.ModNone)
}

func TestLiveHelpBarAndTitleFollowRemappedKeys(t *testing.T) {
	l := liveWithKeys(t, config.KeybindingsConfig{Bindings: map[string][]string{
		"search":                {"s", "S"},
		"cycle_log_level":       {"ctrl+l"},
		"toggle_alerts":         {"x"},
		"clear_resolved_alerts": {"r"},
		"clear_filter":          {"z"},
		"scroll_up":             {"k", "up"},
		"scroll_down":           {"j", "down"},
		"help":                  {"h"},
		"quit":                  {"Q"},
	}})

	bar := string(l.shot().row(l.shot().height - 1))
	want := "s Search | S Filter | Ctrl+L Log Level | x Alerts | d Docker View | a AWS View | z Clear | kj Scroll | h Help | Q Quit"
	if !strings.Contains(bar, want) {
		t.Errorf("help bar:\n%q\nwant it to contain:\n%q", strings.TrimSpace(bar), want)
	}

	l.pressRune('x')
	if text := l.shot().text(); !strings.Contains(text, "Active & Recent Alerts (z/x Close | r Clear Resolved)") {
		t.Errorf("the alerts list title does not follow the keys:\n%s", text)
	}
	l.pressRune('z') // clear_filter closes the alerts list first
	if strings.Contains(l.shot().text(), "Active & Recent Alerts") {
		t.Error("the clear_filter key did not close the alerts list")
	}
}

func TestLiveBracketKeysShowInTheHelpBarAndPage(t *testing.T) {
	l := liveWithKeys(t, config.KeybindingsConfig{Bindings: map[string][]string{"search": {"[", "]"}}})

	if bar := string(l.shot().row(l.shot().height - 1)); !strings.Contains(bar, "[ Search | ] Filter") {
		t.Errorf("help bar = %q, want the bracket keys drawn", strings.TrimSpace(bar))
	}
	l.pressRune('?')
	if row := lineOf(t, l.shot(), "Search by service name"); !strings.Contains(row, "[, ]") {
		t.Errorf("help page row = %q, want the bracket keys drawn", strings.TrimSpace(row))
	}
	l.pressRune('?')
	l.pressRune('[')
	l.do(func() {
		if l.d.app.GetFocus() != l.d.searchField {
			t.Errorf("[ should focus the search field, focus is %T", l.d.app.GetFocus())
		}
	})
}

func TestLiveVimScrollingInTheHelpPage(t *testing.T) {
	km := mustKeymap(t, config.KeybindingsConfig{Preset: "vim"})
	l := startLive(t, Options{Keymap: &km}, 100, 14) // too short to show the whole page
	l.pressRune('?')

	if _, _, ok := l.shot().find("General"); !ok {
		t.Fatalf("the top of the help page is not shown:\n%s", l.shot().text())
	}
	if _, _, ok := l.shot().find("Always available"); ok {
		t.Fatal("the screen is tall enough to show the whole help page, the test would prove nothing")
	}

	l.pressRune('G')
	if _, _, ok := l.shot().find("Always available"); !ok {
		t.Errorf("G should scroll the help page to the end:\n%s", l.shot().text())
	}
	l.pressRune('g')
	if _, _, ok := l.shot().find("General"); !ok {
		t.Errorf("g should scroll the help page back to the top:\n%s", l.shot().text())
	}
	for range 3 {
		l.pressRune('j')
	}
	if _, _, ok := l.shot().find("General"); ok {
		t.Errorf("j should scroll the help page down:\n%s", l.shot().text())
	}
	for range 3 {
		l.pressRune('k')
	}
	if _, _, ok := l.shot().find("General"); !ok {
		t.Errorf("k should scroll the help page up:\n%s", l.shot().text())
	}
	l.pressKey(tcell.KeyCtrlD)
	if _, _, ok := l.shot().find("General"); ok {
		t.Errorf("Ctrl+D should page down in the help page:\n%s", l.shot().text())
	}
}

func TestLiveDaemonErrorWhileHelpIsOpen(t *testing.T) {
	l := liveWithKeys(t, config.KeybindingsConfig{})
	l.pressRune('?')

	l.d.ShowDaemonError(errors.New("cannot reach the daemon"))
	l.do(func() {}) // wait for the queued update
	text := l.shot().text()
	if !strings.Contains(text, "Docker Daemon Offline") || strings.Contains(text, helpTitleOnScreen) {
		t.Fatalf("the error page should replace the help page:\n%s", text)
	}
	l.do(func() {
		if l.d.showingHelp {
			t.Error("the dashboard still believes the help page is open")
		}
	})

	// The help page can be opened over the error page, and a reconnection clears it.
	l.pressRune('?')
	if !strings.Contains(l.shot().text(), helpTitleOnScreen) {
		t.Error("? should open the help page over the error page")
	}
	l.d.ClearDaemonError()
	l.do(func() {})
	text = l.shot().text()
	if strings.Contains(text, helpTitleOnScreen) || strings.Contains(text, "Docker Daemon Offline") || !strings.Contains(text, "Docker Services") {
		t.Errorf("the reconnected dashboard should be back:\n%s", text)
	}
	l.do(func() {
		if l.d.showingHelp {
			t.Error("the dashboard still believes the help page is open after reconnecting")
		}
	})
	l.pressRune('l') // keys work normally again
	l.do(func() {
		if l.d.filterState.MinLogLevel == 0 {
			t.Error("l did not cycle the log level after the help page was dismissed by a reconnection")
		}
	})
}

// With the default keymap, the keys everyone knows keep working in the running
// application: this is the live counterpart of the oracle test.
func TestLiveDefaultKeysStillWork(t *testing.T) {
	l := startLive(t, Options{}, 150, 40)
	l.update(sampleContainers())

	// The view and the filters apply on the next refresh of the containers.
	l.pressRune('a')
	l.update(sampleContainers())
	if !strings.Contains(l.shot().text(), "AWS Cloud Resources") {
		t.Errorf("a should show the AWS view:\n%s", l.shot().text())
	}
	l.pressRune('d')
	l.update(sampleContainers())
	if !strings.Contains(l.shot().text(), "Docker Services") {
		t.Error("d should show the Docker view")
	}
	l.pressRune('l')
	l.update(sampleContainers())
	if !strings.Contains(l.shot().text(), "Logs [INFO+]") && !strings.Contains(l.shot().text(), "Logs") {
		t.Error("l should cycle the log level")
	}
	l.do(func() {
		if got := l.d.filterState.MinLogLevel.String(); got != "INFO" {
			t.Errorf("minimum log level = %q after l, want INFO", got)
		}
	})
	l.pressRune('f')
	l.do(func() {
		if l.d.app.GetFocus() != l.d.searchField {
			t.Errorf("f should focus the search field, focus is %T", l.d.app.GetFocus())
		}
	})
	l.pressKey(tcell.KeyEscape)
	l.do(func() {
		if l.d.filterState.MinLogLevel != 0 || l.d.app.GetFocus() != l.d.logsView {
			t.Errorf("Esc should clear the filters and focus the logs: level %v, focus %T", l.d.filterState.MinLogLevel, l.d.app.GetFocus())
		}
	})
	l.pressAndQuit(tcell.KeyRune, 'q', tcell.ModNone)
}
