package ui

import (
	"strings"
	"testing"

	"github.com/b92c/gowatch/internal/config"
)

func TestHelpBarTextFollowsTheKeymap(t *testing.T) {
	theme := darkTheme()
	bar := func(t *testing.T, cfg config.KeybindingsConfig) string {
		t.Helper()
		return helpBarText(theme, mustKeymap(t, cfg))
	}

	t.Run("default keymap is the original bar plus the Help hint", func(t *testing.T) {
		got := bar(t, config.KeybindingsConfig{})
		if got != legacyHelpBarWithHint {
			t.Errorf("help bar:\n%q\nwant:\n%q", got, legacyHelpBarWithHint)
		}
		// Taking the hint out gives back the original, byte for byte.
		if without := strings.Replace(got, "[yellow]?[white] Help | ", "", 1); without != legacyHelpBar {
			t.Errorf("the default bar minus the Help hint differs from the original:\n%q\n%q", without, legacyHelpBar)
		}
		if !strings.Contains(got, "↑↓[white] Scroll | [yellow]?[white] Help | [yellow]q[white] Quit") {
			t.Errorf("the Help hint is not between Scroll and Quit: %q", got)
		}
	})

	t.Run("vim shows the same bar", func(t *testing.T) {
		if got := bar(t, config.KeybindingsConfig{Preset: "vim"}); got != legacyHelpBarWithHint {
			t.Errorf("the vim preset only adds keys behind the first ones, so the bar should not change:\n%q", got)
		}
	})

	t.Run("remapped keys appear", func(t *testing.T) {
		got := bar(t, config.KeybindingsConfig{Bindings: map[string][]string{
			"quit":            {"x", "q"},
			"help":            {"h"},
			"search":          {"s", "S"},
			"cycle_log_level": {"ctrl+l"},
			"toggle_alerts":   {"esc"},
			"clear_filter":    {"ctrl+x"},
			"docker_view":     {"1"},
			"aws_view":        {"2"},
			"scroll_up":       {"k", "up"},
			"scroll_down":     {"j", "down"},
		}})
		want := "[yellow]s[white] Search | [yellow]S[white] Filter | [yellow]Ctrl+L[white] Log Level | [yellow]Esc[white] Alerts | " +
			"[yellow]1[white] Docker View | [yellow]2[white] AWS View | [yellow]Ctrl+X[white] Clear | [yellow]kj[white] Scroll | " +
			"[yellow]h[white] Help | [yellow]x[white] Quit"
		if got != want {
			t.Errorf("help bar:\n%q\nwant:\n%q", got, want)
		}
	})

	t.Run("without a second search key there is no Filter hint", func(t *testing.T) {
		got := bar(t, config.KeybindingsConfig{Bindings: map[string][]string{"search": {"s"}}})
		if strings.Contains(got, "Filter") || !strings.Contains(got, "[yellow]s[white] Search | [yellow]l[white] Log Level") {
			t.Errorf("help bar: %q", got)
		}
	})

	t.Run("colors come from the theme", func(t *testing.T) {
		th := darkTheme()
		th.HelpKey, th.HelpText = th.StatusOK, th.StatusError
		if got := helpBarText(th, DefaultKeymap()); !strings.HasPrefix(got, "[green]/[red] Search | [green]f[red] Filter") {
			t.Errorf("help bar = %q", got)
		}
	})
}

func TestAlertsModalTitleFollowsTheKeymap(t *testing.T) {
	theme := darkTheme()
	got := alertsModalTitle(theme, mustKeymap(t, config.KeybindingsConfig{Bindings: map[string][]string{
		"clear_filter": {"z"}, "toggle_alerts": {"x"}, "clear_resolved_alerts": {"r", "R"},
	}}))
	want := " 🚨 Active & Recent Alerts ([yellow]z[white]/[yellow]x[white] Close | [yellow]r[white] Clear Resolved) "
	if got != want {
		t.Errorf("title:\n%q\nwant:\n%q", got, want)
	}
	if got := alertsModalTitle(theme, DefaultKeymap()); got != legacyAlertsTitle {
		t.Errorf("default title:\n%q\nwant the original:\n%q", got, legacyAlertsTitle)
	}
}

func TestErrorHelpBarTextFollowsTheKeymap(t *testing.T) {
	theme := darkTheme()
	if got := errorHelpBarText(theme, DefaultKeymap()); got != legacyErrorHelpBar {
		t.Errorf("default error bar:\n%q\nwant the original:\n%q", got, legacyErrorHelpBar)
	}
	// vim keeps q as the first quit key.
	if got := errorHelpBarText(theme, mustKeymap(t, config.KeybindingsConfig{Preset: "vim"})); got != legacyErrorHelpBar {
		t.Errorf("vim error bar:\n%q\nwant the original:\n%q", got, legacyErrorHelpBar)
	}

	got := errorHelpBarText(theme, mustKeymap(t, config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"x"}}}))
	want := "[yellow]x[white] Quit | [gray]Waiting for Docker daemon...[-]"
	if got != want {
		t.Errorf("error bar with quit on x:\n%q\nwant:\n%q", got, want)
	}
	if strings.Contains(got, "[q]") {
		t.Error("the error bar still mentions q")
	}

	got = errorHelpBarText(theme, mustKeymap(t, config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"ctrl+q"}}}))
	if !strings.Contains(got, "Ctrl+Q") {
		t.Errorf("error bar with quit on Ctrl+Q = %q", got)
	}
}

// tview reads [word] as a color tag, so a bracket used as a key must be escaped
// or the hint would vanish.
func TestBracketKeysAreDrawnInTheTexts(t *testing.T) {
	km := mustKeymap(t, config.KeybindingsConfig{Bindings: map[string][]string{
		"search": {"["}, "docker_view": {"]"}, "quit": {"{"}, "toggle_alerts": {"}"},
	}})
	d := newTestDashboard(t, Options{Keymap: &km})

	bar := drawPrimitive(t, d.helpBar, 200, 1).text()
	for _, want := range []string{"[ Search", "] Docker View", "{ Quit", "} Alerts"} {
		if !strings.Contains(bar, want) {
			t.Errorf("help bar %q does not show %q", strings.TrimSpace(bar), want)
		}
	}
	if title := d.alertsModal.GetTitle(); !strings.Contains(title, "}") {
		t.Errorf("alerts modal title %q lost the bracket key", title)
	}

	errBar := errorHelpBarText(d.theme, mustKeymap(t, config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"["}}}))
	d.helpBar.SetText(errBar)
	if text := drawPrimitive(t, d.helpBar, 200, 1).text(); !strings.Contains(text, "[ Quit") {
		t.Errorf("error bar %q does not show the bracket quit key", strings.TrimSpace(text))
	}
}
