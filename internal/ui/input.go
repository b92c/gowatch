package ui

import (
	"github.com/b92c/gowatch/internal/docker"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// handleInput is the application-wide input capture. It looks the key up in the
// keymap and runs the action bound to it; what it does not consume goes on to
// the primitive with focus.
func (d *Dashboard) handleInput(event *tcell.EventKey) *tcell.EventKey {
	// While the search field has focus every key is typed into it.
	if d.searchField.HasFocus() || d.app.GetFocus() == d.searchField {
		return event
	}
	if d.showingHelp {
		return d.handleHelpInput(event)
	}
	// The scroll keys belong to the panel with focus, whatever the keymap says.
	if _, native := scrollKeys[event.Key()]; native {
		return event
	}
	action, ok := d.keymap.Lookup(event)
	if !ok {
		return event
	}
	d.runAction(action)
	return nil
}

// handleHelpInput is handleInput while the help page is open. The keys of the
// help, quit and clear_filter (Esc) actions close it without quitting, the
// scroll actions move through the page, no other action runs, and keys without
// an action (the arrows, Ctrl+C, ...) go on as usual.
func (d *Dashboard) handleHelpInput(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyEscape {
		d.closeHelp()
		return nil
	}
	if _, native := scrollKeys[event.Key()]; native {
		return event
	}
	action, ok := d.keymap.Lookup(event)
	if !ok {
		return event
	}
	switch {
	case action == ActionHelp || action == ActionQuit:
		d.closeHelp()
	case isScrollAction(action):
		scrollTable(d.helpTable, action)
	}
	return nil
}

// runAction does what an action says.
func (d *Dashboard) runAction(action Action) {
	switch action {
	case ActionQuit:
		d.Stop()
	case ActionSearch:
		d.app.SetFocus(d.searchField)
	case ActionCycleLogLevel:
		d.filterState.CycleMinLogLevel()
	case ActionExportLogs:
		d.exportLogs()
	case ActionDockerView:
		d.awsViewMode = false
	case ActionAWSView:
		d.awsViewMode = true
	case ActionToggleAlerts:
		d.toggleAlertsModal()
	case ActionClearResolvedAlerts:
		docker.GetGlobalAlertEngine().ClearResolvedAlerts()
	case ActionClearFilter:
		d.clearFilters()
	case ActionHelp:
		if d.showingHelp {
			d.closeHelp()
		} else {
			d.openHelp()
		}
	default:
		if isScrollAction(action) {
			d.scroll(action)
		}
	}
}

// toggleAlertsModal shows the alerts list, or goes back to the dashboard.
func (d *Dashboard) toggleAlertsModal() {
	d.showingAlertsModal = !d.showingAlertsModal
	if d.showingAlertsModal {
		d.pages.SwitchToPage("alerts_modal")
		d.app.SetFocus(d.alertsModal)
	} else {
		d.pages.SwitchToPage("dashboard")
		d.app.SetFocus(d.mainPanel())
	}
}

// clearFilters closes the alerts list when it is open and otherwise clears every
// filter, including the ones that came from the config file, and gives the focus
// back to the main panel.
func (d *Dashboard) clearFilters() {
	if d.showingAlertsModal {
		d.showingAlertsModal = false
		d.pages.SwitchToPage("dashboard")
		d.app.SetFocus(d.mainPanel())
		return
	}
	d.filterState.Clear()
	d.searchField.SetText("")
	d.app.SetFocus(d.mainPanel())
}

// scroll moves what the user is looking at. With the alerts list open that is
// the list; otherwise the logs, and with the logs hidden there is nothing to
// scroll. Only keys that are not scroll keys of the panels get here (j and k in
// the vim preset, for example).
func (d *Dashboard) scroll(action Action) {
	switch {
	case d.showingAlertsModal:
		scrollTable(d.alertsModal, action)
	case d.visible.logs:
		d.scrollLogs(action)
	}
}

// scrollLogs scrolls the logs view for a scroll key that the panel does not
// handle itself (j and k in the vim preset, for example). scroll_bottom goes back
// to following the newest line; every other action leaves the view where the user
// put it. The native keys, End included, do not follow: the logs panel marks the
// user as scrolling on any key it receives.
func (d *Dashboard) scrollLogs(action Action) {
	view := d.logsView
	row, col := view.GetScrollOffset()
	row = max(row, 0) // negative until the view has been drawn
	_, _, _, height := view.GetInnerRect()
	page := max(height, 1)

	switch action {
	case ActionScrollUp:
		view.ScrollTo(max(row-1, 0), col)
	case ActionScrollDown:
		view.ScrollTo(row+1, col)
	case ActionPageUp:
		view.ScrollTo(max(row-page, 0), col)
	case ActionPageDown:
		view.ScrollTo(row+page, col)
	case ActionScrollTop:
		view.ScrollToBeginning()
	case ActionScrollBottom:
		view.ScrollToEnd()
		d.userScrolling = false
		return
	}
	d.userScrolling = true
}

// scrollTable moves a table with the key it already understands for the action.
func scrollTable(table *tview.Table, action Action) {
	key, ok := scrollKeyOf[action]
	if !ok {
		return
	}
	table.InputHandler()(tcell.NewEventKey(key, 0, tcell.ModNone), func(tview.Primitive) {})
}
