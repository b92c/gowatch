package ui

import (
	"strings"

	"github.com/rivo/tview"
)

// keyHint is one entry of the help bar: the key as it is written in the tview
// text (already escaped) and what it does.
type keyHint struct{ key, action string }

// keyText is how a key is written inside a tview text: its label, with any
// square brackets escaped so the library does not read them as a color tag.
func keyText(k KeyCombo) string { return tview.Escape(k.Label()) }

// barHints lists the entries of the help bar, from the first key of each main
// action of the active keymap. A second key of the search action is offered as
// the filter key, as the bar has always done with / and f.
func barHints(km Keymap) []keyHint {
	var hints []keyHint
	add := func(a Action, label string) {
		if k, ok := km.firstKey(a); ok {
			hints = append(hints, keyHint{key: keyText(k), action: label})
		}
	}

	add(ActionSearch, "Search")
	if keys := km.Keys(ActionSearch); len(keys) > 1 {
		hints = append(hints, keyHint{key: keyText(keys[1]), action: "Filter"})
	}
	add(ActionCycleLogLevel, "Log Level")
	add(ActionToggleAlerts, "Alerts")
	add(ActionDockerView, "Docker View")
	add(ActionAWSView, "AWS View")
	add(ActionClearFilter, "Clear")
	if up, ok := km.firstKey(ActionScrollUp); ok {
		if down, ok := km.firstKey(ActionScrollDown); ok {
			hints = append(hints, keyHint{key: keyText(up) + keyText(down), action: "Scroll"})
		}
	}
	add(ActionHelp, "Help")
	add(ActionQuit, "Quit")
	return hints
}

// helpBarText is the text of the key hints at the bottom of the dashboard, built
// from the keymap. With the default keymap it is the bar GoWatch has always
// had plus the "? Help" entry.
func helpBarText(t Theme, km Keymap) string {
	hints := barHints(km)
	key, text := t.Tag(t.HelpKey), t.Tag(t.HelpText)
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = key + h.key + text + " " + h.action
	}
	return strings.Join(parts, " | ")
}

// errorHelpBarText is the help bar of the "Docker daemon offline" page. It has
// always started with a literal "[q]", which tview reads as a color tag and so
// never draws; the default keymap keeps that text as it is. Another quit key is
// shown for real.
func errorHelpBarText(t Theme, km Keymap) string {
	quit, ok := km.firstKey(ActionQuit)
	if ok && quit != runeCombo('q') {
		return t.Tag(t.HelpKey) + keyText(quit) + t.Tag(t.HelpText) + " Quit | " + t.Tag(t.Muted) + "Waiting for Docker daemon...[-]"
	}
	return "[q]" + t.Tag(t.HelpKey) + " Quit" + t.Tag(t.HelpText) + " | " + t.Tag(t.Muted) + "Waiting for Docker daemon...[-]"
}

// alertsModalTitle is the title of the alerts modal, with its key hints. The
// first key of clear_filter closes the modal (Esc by default), as does the first
// key of toggle_alerts.
func alertsModalTitle(t Theme, km Keymap) string {
	key, text := t.Tag(t.HelpKey), t.Tag(t.HelpText)
	keyOf := func(a Action) string {
		if k, ok := km.firstKey(a); ok {
			return keyText(k)
		}
		return ""
	}
	return " 🚨 Active & Recent Alerts (" + key + keyOf(ActionClearFilter) + text + "/" + key + keyOf(ActionToggleAlerts) + text +
		" Close | " + key + keyOf(ActionClearResolvedAlerts) + text + " Clear Resolved) "
}

// daemonErrorText is the content of the "Docker daemon offline" page.
func daemonErrorText(t Theme, err error) string {
	red := t.Tag(t.StatusError)
	redBold := "[" + colorName(t.StatusError) + "::b]"
	accent, text, muted, faint := t.Tag(t.Accent), t.Tag(t.Text), t.Tag(t.Muted), t.Tag(t.Faint)
	return "\n\n\n" +
		"    " + red + "██████╗  █████╗ ███████╗███╗   ███╗ ██████╗ ███╗   ██╗[-]\n" +
		"    " + red + "██╔══██╗██╔══██╗██╔════╝████╗ ████║██╔═══██╗████╗  ██║[-]\n" +
		"    " + red + "██║  ██║███████║█████╗  ██╔████╔██║██║   ██║██╔██╗ ██║[-]\n" +
		"    " + red + "██║  ██║██╔══██║██╔══╝  ██║╚██╔╝██║██║   ██║██║╚██╗██║[-]\n" +
		"    " + red + "██████╔╝██║  ██║███████╗██║ ╚═╝ ██║╚██████╔╝██║ ╚████║[-]\n" +
		"    " + red + "╚═════╝ ╚═╝  ╚═╝╚══════╝╚═╝     ╚═╝ ╚═════╝ ╚═╝  ╚═══╝[-]\n\n" +
		"    " + redBold + "⚠  Docker Daemon is not running[-]\n\n" +
		"    " + text + "GoWatch requires the Docker daemon to monitor containers.[-]\n" +
		"    " + text + "Please start Docker and GoWatch will reconnect automatically.[-]\n\n" +
		"    " + muted + "How to start Docker:[-]\n" +
		"    " + accent + "  • macOS:" + text + "  Open Docker Desktop application[-]\n" +
		"    " + accent + "  • Linux:" + text + "  sudo systemctl start docker[-]\n\n" +
		"    " + muted + "Error: " + err.Error() + "[-]\n\n" +
		"    " + faint + "Retrying connection every 2 seconds...[-]"
}
