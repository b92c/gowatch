package ui

import (
	"slices"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// helpPage is the name of the help page in the dashboard's pages.
const helpPage = "help"

// alwaysAvailable are the keys that work whatever the keymap says: Ctrl+C is
// reserved and the scroll keys belong to the panel with focus. A row with no keys
// goes on with the description of the row above it.
var alwaysAvailable = []struct{ keys, description string }{
	{keys: "Ctrl+C", description: "Quit GoWatch from anywhere (cannot be rebound)"},
	{keys: "↑ ↓", description: "Scroll the panel with focus a line (always active)"},
	{keys: "PgUp PgDn", description: "Scroll the panel with focus a page (always active)"},
	{keys: "Home End", description: "Jump to the top or bottom of the panel (always active)"},
	// The panel with focus stops following new logs on any key it scrolls with,
	// End included; only a scroll_bottom key that is not one of these goes back
	// to following (G in the vim preset). Each row has to fit the 80 columns of a
	// small terminal.
	{keys: "", description: "These keys stop following new logs. A key bound to"},
	{keys: "", description: "scroll_bottom that is not one of them (G in vim)"},
	{keys: "", description: "follows the newest log line again."},
}

// keyLabels writes the keys of an action the way the help page does, separated
// by commas, with brackets escaped.
func keyLabels(keys []KeyCombo) string {
	labels := make([]string, len(keys))
	for i, k := range keys {
		labels[i] = keyText(k)
	}
	return strings.Join(labels, ", ")
}

// helpTitle is the title of the help page; it says how to close it, with the
// keys of the keymap that do so.
func helpTitle(km Keymap) string {
	closing := make([]KeyCombo, 0, 4)
	add := func(keys ...KeyCombo) {
		for _, k := range keys {
			if !slices.Contains(closing, k) {
				closing = append(closing, k)
			}
		}
	}
	add(km.Keys(ActionHelp)...)
	add(KeyCombo{Key: tcell.KeyEscape})
	add(km.Keys(ActionQuit)...)

	labels := make([]string, len(closing))
	for i, k := range closing {
		labels[i] = keyText(k)
	}
	return " Keyboard Shortcuts (" + strings.Join(labels, ", ") + " to close) "
}

// newHelpTable builds the content of the help page from the keymap: the
// actions grouped by section, each with its keys, followed by the keys that
// always work. The keys shown are the ones of the active keymap, so remapped
// keys appear as the user set them.
func newHelpTable(t Theme, km Keymap) *tview.Table {
	table := tview.NewTable().SetBorders(false).SetSelectable(false, false)
	table.SetBorder(true).
		SetTitle(helpTitle(km)).
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(t.Title).
		SetBorderColor(t.Border).
		SetBorderPadding(1, 1, 2, 2)

	row := 0
	section := func(name string) {
		if row > 0 {
			row++ // blank line between sections
		}
		table.SetCell(row, 0, tview.NewTableCell(name).
			SetTextColor(t.Accent).
			SetAttributes(tcell.AttrBold).
			SetSelectable(false))
		row++
	}
	line := func(keys, description string) {
		table.SetCell(row, 0, tview.NewTableCell(keys).
			SetTextColor(t.HelpKey).
			SetSelectable(false))
		table.SetCell(row, 1, tview.NewTableCell(description).
			SetTextColor(t.Text).
			SetExpansion(1).
			SetSelectable(false))
		row++
	}

	for _, group := range groupOrder {
		section(group)
		for _, info := range actionTable {
			if info.group == group {
				line(keyLabels(km.Keys(info.action)), info.description)
			}
		}
	}
	section("Always available")
	for _, entry := range alwaysAvailable {
		line(tview.Escape(entry.keys), entry.description)
	}
	return table
}

// openHelp shows the help page on top of whatever is on screen, the alerts list
// included, and gives it the focus.
func (d *Dashboard) openHelp() {
	d.showingHelp = true
	d.helpTable.ScrollToBeginning()
	d.pages.ShowPage(helpPage)
	d.app.SetFocus(d.helpTable)
}

// closeHelp hides the help page and gives the focus back to what was under it:
// the alerts list when it is open, the main panel otherwise.
func (d *Dashboard) closeHelp() {
	d.showingHelp = false
	d.pages.HidePage(helpPage)
	if d.showingAlertsModal {
		d.app.SetFocus(d.alertsModal)
		return
	}
	d.app.SetFocus(d.mainPanel())
}
