package ui

import "strings"

// helpBarText is the text of the key hints at the bottom of the dashboard.
func helpBarText(t Theme) string {
	hints := []struct{ key, action string }{
		{"/", "Search"}, {"f", "Filter"}, {"l", "Log Level"}, {"w", "Alerts"}, {"d", "Docker View"},
		{"a", "AWS View"}, {"Esc", "Clear"}, {"↑↓", "Scroll"}, {"q", "Quit"},
	}
	key, text := t.Tag(t.HelpKey), t.Tag(t.HelpText)
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = key + h.key + text + " " + h.action
	}
	return strings.Join(parts, " | ")
}

// errorHelpBarText is the help bar of the "Docker daemon offline" page.
func errorHelpBarText(t Theme) string {
	return "[q]" + t.Tag(t.HelpKey) + " Quit" + t.Tag(t.HelpText) + " | " + t.Tag(t.Muted) + "Waiting for Docker daemon...[-]"
}

// alertsModalTitle is the title of the alerts modal, with its key hints.
func alertsModalTitle(t Theme) string {
	key, text := t.Tag(t.HelpKey), t.Tag(t.HelpText)
	return " 🚨 Active & Recent Alerts (" + key + "Esc" + text + "/" + key + "w" + text + " Close | " + key + "c" + text + " Clear Resolved) "
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
