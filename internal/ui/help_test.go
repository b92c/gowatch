package ui

import (
	"strings"
	"testing"

	"github.com/b92c/gowatch/internal/config"
	"github.com/gdamore/tcell/v2"
)

// helpScreen draws the help page of a keymap on a screen tall enough to hold it.
func helpScreen(t *testing.T, cfg config.KeybindingsConfig) screenshot {
	t.Helper()
	d := newKeyedTestDashboard(t, cfg)
	return drawPrimitive(t, d.helpTable, 110, 45)
}

func newKeyedTestDashboard(t *testing.T, cfg config.KeybindingsConfig) *Dashboard {
	t.Helper()
	km := mustKeymap(t, cfg)
	return newTestDashboard(t, Options{Keymap: &km})
}

// lineOf returns the row of the screenshot that holds text, failing the test
// when no row does.
func lineOf(t *testing.T, shot screenshot, text string) string {
	t.Helper()
	for y := 0; y < shot.height; y++ {
		if row := string(shot.row(y)); strings.Contains(row, text) {
			return row
		}
	}
	t.Fatalf("%q is not on the help page:\n%s", text, shot.text())
	return ""
}

// rowIndexOf is the position of the row that holds text, or -1.
func rowIndexOf(shot screenshot, text string) int {
	for y := 0; y < shot.height; y++ {
		if strings.Contains(string(shot.row(y)), text) {
			return y
		}
	}
	return -1
}

func TestHelpPageListsEveryActionByGroup(t *testing.T) {
	shot := helpScreen(t, config.KeybindingsConfig{})

	// Sections come in order, and an action sits under its own section.
	lastRow := -1
	for _, group := range groupOrder {
		y := rowIndexOf(shot, group)
		if y < 0 {
			t.Fatalf("section %q is not on the help page:\n%s", group, shot.text())
		}
		if y <= lastRow {
			t.Errorf("section %q is on row %d, not after the previous section (row %d)", group, y, lastRow)
		}
		lastRow = y
	}
	if y := rowIndexOf(shot, "Always available"); y <= lastRow {
		t.Errorf("the always-available section is on row %d, want it after the groups (last at %d)", y, lastRow)
	}

	for _, action := range Actions() {
		desc := action.Description()
		y := rowIndexOf(shot, desc)
		if y < 0 {
			t.Errorf("the description of %s (%q) is not on the help page", action, desc)
			continue
		}
		groupRow := rowIndexOf(shot, action.Group())
		next := shot.height
		for i, g := range groupOrder {
			if g == action.Group() && i+1 < len(groupOrder) {
				next = rowIndexOf(shot, groupOrder[i+1])
			}
		}
		if action.Group() == groupNavigation {
			next = rowIndexOf(shot, "Always available")
		}
		if y <= groupRow || y >= next {
			t.Errorf("%s is on row %d, outside its section %q (rows %d-%d)", action, y, action.Group(), groupRow, next)
		}
	}
}

func TestHelpPageShowsTheDefaultKeys(t *testing.T) {
	shot := helpScreen(t, config.KeybindingsConfig{})

	tests := []struct {
		description string
		keys        string
	}{
		{"Show or hide this help", "?"},
		{"Quit GoWatch", "q"},
		{"Show the Docker services", "d, D"},
		{"Show the AWS resources", "a, A"},
		{"Search by service, ID, image or log text", "/, f"},
		{"Cycle the minimum log level", "l, L"},
		{"Clear all filters", "Esc"},
		{"Show or hide the alerts list", "w, W"},
		{"Clear the resolved alerts", "c, C"},
		{"Scroll up one line", "↑"},
		{"Scroll down one line", "↓"},
		{"Scroll up one page", "PgUp"},
		{"Scroll down one page", "PgDn"},
		{"Jump to the top", "Home"},
		{"Jump to the bottom", "End"},
	}
	for _, tt := range tests {
		row := lineOf(t, shot, tt.description)
		keys, _, _ := strings.Cut(row, tt.description)
		if !strings.Contains(keys, tt.keys) {
			t.Errorf("row of %q = %q: keys do not include %q", tt.description, strings.TrimSpace(row), tt.keys)
		}
	}

	// What always works is listed too.
	for _, text := range []string{"Ctrl+C", "cannot be rebound", "PgUp PgDn", "Home End", "always active"} {
		lineOf(t, shot, text)
	}
	if y := rowIndexOf(shot, "Keyboard Shortcuts (?, Esc, q to close)"); y < 0 {
		t.Errorf("the title does not say how to close the page:\n%s", shot.text())
	}
}

func TestHelpPageShowsRemappedKeys(t *testing.T) {
	shot := helpScreen(t, config.KeybindingsConfig{
		Preset: "vim",
		Bindings: map[string][]string{
			"quit":   {"x", "ctrl+q"},
			"help":   {"h"},
			"search": {"["},
		},
	})

	tests := []struct {
		description string
		keys        string
		notKeys     string
	}{
		{"Quit GoWatch", "x, Ctrl+Q", "q,"},
		{"Show or hide this help", "h", "?"},
		{"Search by service, ID, image or log text", "[", "/"},
		{"Scroll down one line", "↓, j", ""},
		{"Scroll up one line", "↑, k", ""},
		{"Jump to the top", "Home, g", ""},
		{"Jump to the bottom", "End, G", ""},
		{"Scroll down one page", "PgDn, Ctrl+D", ""},
		{"Scroll up one page", "PgUp, Ctrl+U", ""},
	}
	for _, tt := range tests {
		row := lineOf(t, shot, tt.description)
		keys, _, _ := strings.Cut(row, tt.description)
		if !strings.Contains(keys, tt.keys) {
			t.Errorf("row of %q = %q: keys do not include %q", tt.description, strings.TrimSpace(row), tt.keys)
		}
		if tt.notKeys != "" && strings.Contains(keys, tt.notKeys) {
			t.Errorf("row of %q = %q: still shows the old key %q", tt.description, strings.TrimSpace(row), tt.notKeys)
		}
	}
	lineOf(t, shot, "Keyboard Shortcuts (h, Esc, x, Ctrl+Q to close)")
}

func TestHelpPageUsesTheThemeColors(t *testing.T) {
	theme := darkTheme()
	theme.Accent = tcell.ColorRed
	theme.HelpKey = tcell.ColorGreen
	theme.Text = tcell.ColorBlue
	theme.Border = tcell.ColorPurple
	theme.Title = tcell.ColorOrange
	d := newTestDashboard(t, Options{Theme: &theme})
	shot := drawPrimitive(t, d.helpTable, 110, 45)

	if fg, _ := shot.colorsOf(t, "General"); fg != theme.Accent {
		t.Errorf("section heading color = %v, want the accent %v", fg, theme.Accent)
	}
	if fg, _ := shot.colorsOf(t, "Navigation"); fg != theme.Accent {
		t.Errorf("section heading color = %v, want the accent %v", fg, theme.Accent)
	}
	if fg, _ := shot.colorsOnRowOf(t, "Quit GoWatch", "q"); fg != theme.HelpKey {
		t.Errorf("key color = %v, want the help key color %v", fg, theme.HelpKey)
	}
	if fg, _ := shot.colorsOf(t, "Quit GoWatch"); fg != theme.Text {
		t.Errorf("description color = %v, want the text color %v", fg, theme.Text)
	}
	if fg, _ := shot.colorsOf(t, "Keyboard Shortcuts"); fg != theme.Title {
		t.Errorf("title color = %v, want %v", fg, theme.Title)
	}
	if fg, _ := shot.colorsOf(t, "┌"); fg != theme.Border {
		t.Errorf("border color = %v, want %v", fg, theme.Border)
	}
}

func TestHelpPageEscapesBracketKeys(t *testing.T) {
	shot := helpScreen(t, config.KeybindingsConfig{Bindings: map[string][]string{"search": {"[", "]"}}})
	row := lineOf(t, shot, "Search by service, ID")
	if !strings.Contains(row, "[, ]") {
		t.Errorf("row of the search action = %q, want the keys [ and ] drawn", strings.TrimSpace(row))
	}
}

// The help page is static text built from the keymap: it must not change the
// dark theme's look elsewhere, and it is invisible until opened.
func TestHelpPageIsHiddenUntilOpened(t *testing.T) {
	d := newTestDashboard(t, Options{})
	if got := d.pages.GetPageNames(true); len(got) != 1 || got[0] != "dashboard" {
		t.Errorf("visible pages = %v, want only the dashboard", got)
	}
	if strings.Contains(drawDashboard(t, d, 120, 40).text(), "Keyboard Shortcuts") {
		t.Error("the help page is on screen before it was opened")
	}
	if !d.pages.HasPage(helpPage) {
		t.Error("the help page is not registered")
	}
}

// What the help page says about scrolling has to be true for every key it is
// shown next to. The native keys (End included) stop the logs from following new
// lines, so no description may promise that, and the page says who does follow.
func TestHelpPageDoesNotPromiseFollowingOnEnd(t *testing.T) {
	for _, action := range Actions() {
		desc := action.Description()
		if strings.Contains(strings.ToLower(desc), "follow") {
			t.Errorf("the description of %s promises following new logs: %q", action, desc)
		}
		if action.Group() == groupNavigation && strings.Contains(strings.ToLower(desc), "log") {
			t.Errorf("the description of %s names the logs, but the native keys scroll the panel with focus: %q", action, desc)
		}
	}
	if got, want := ActionScrollBottom.Description(), "Jump to the bottom"; got != want {
		t.Errorf("scroll_bottom description = %q, want %q", got, want)
	}

	for _, cfg := range []config.KeybindingsConfig{{}, {Preset: "vim"}} {
		shot := helpScreen(t, cfg)
		// The note sits right under the Home/End row, as three rows without keys.
		home := rowIndexOf(shot, "Jump to the top or bottom of the panel (always active)")
		if home < 0 {
			t.Fatalf("the Home/End row is not on the help page:\n%s", shot.text())
		}
		var note []string
		for i, line := range []string{
			"These keys stop following new logs. A key bound to",
			"scroll_bottom that is not one of them (G in vim)",
			"follows the newest log line again.",
		} {
			y := rowIndexOf(shot, line)
			if y != home+1+i {
				t.Errorf("note line %q is on row %d, want row %d (right under the Home End row)", line, y, home+1+i)
			}
			note = append(note, line)
		}
		joined := strings.Join(note, " ")
		for _, want := range []string{"stop following new logs", "scroll_bottom", "G in vim", "follows the newest log line again"} {
			if !strings.Contains(joined, want) {
				t.Errorf("the note does not say %q", want)
			}
		}
	}
}

// On a small terminal nothing the help page says may be cut off: every
// description and every note has to be on screen in full at 80 columns, for the
// presets and for keys that need more room.
func TestHelpPageFitsEightyColumns(t *testing.T) {
	configs := map[string]config.KeybindingsConfig{
		"default": {},
		"vim":     {Preset: "vim"},
		"long key lists": {Bindings: map[string][]string{
			"quit": {"q", "x", "ctrl+q"}, "scroll_down": {"j", "down", "n"},
		}},
	}
	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			km := mustKeymap(t, cfg)
			d := newTestDashboard(t, Options{Keymap: &km})
			shot := drawPrimitive(t, d.helpTable, 80, 45)

			var texts []string
			for _, action := range Actions() {
				texts = append(texts, action.Description())
			}
			for _, entry := range alwaysAvailable {
				texts = append(texts, entry.description)
			}
			texts = append(texts, helpTitle(km))
			for _, text := range texts {
				if _, _, ok := shot.find(strings.TrimSpace(text)); !ok {
					t.Errorf("%q is cut off or missing at 80 columns:\n%s", text, shot.text())
				}
			}
		})
	}
}
