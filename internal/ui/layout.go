package ui

import (
	"github.com/b92c/gowatch/internal/config"
	"github.com/rivo/tview"
)

// panelID names the primitives the main grid is made of.
type panelID int

const (
	panelServices panelID = iota
	panelResources
	panelSearch
	panelLogs
	panelHelpBar
	panelCount
)

// Fixed row heights, in terminal rows.
const (
	searchRowHeight  = 3
	helpBarRowHeight = 1
)

// gridSlot is one row of the main grid. Height follows tview's grid rows: a
// positive value is a fixed number of terminal rows, a negative one a
// proportional weight.
type gridSlot struct {
	panel  panelID
	height int
}

// panelVisibility tells which optional panels are on screen. The services table
// and the search field are always shown.
type panelVisibility struct {
	resources bool
	logs      bool
	helpBar   bool
}

// isVisible reads a layout "visible" flag; leaving it out means visible.
func isVisible(flag *bool) bool { return flag == nil || *flag }

func visibilityOf(layout config.LayoutConfig) panelVisibility {
	return panelVisibility{
		resources: isVisible(layout.Resources.Visible),
		logs:      isVisible(layout.Logs.Visible),
		helpBar:   isVisible(layout.HelpBar.Visible),
	}
}

// weightRow turns a configured height into a proportional grid row. A height
// of 0 (not set) counts as 1; the config validation rejects negative values.
func weightRow(height int) int {
	if height < 1 {
		height = 1
	}
	return -height
}

// planGrid lists the grid rows from top to bottom: services, resources, search,
// logs and help bar, leaving out the hidden panels. Heights are weights shared
// by the proportional rows; the search field and the help bar keep a fixed
// height. With every height at 1 the rows are the ones the dashboard always
// had, because tview gives a row of -1 the same share as a row of 0.
func planGrid(layout config.LayoutConfig) []gridSlot {
	vis := visibilityOf(layout)

	slots := []gridSlot{{panel: panelServices, height: weightRow(layout.Services.Height)}}
	if vis.resources {
		slots = append(slots, gridSlot{panel: panelResources, height: weightRow(layout.Resources.Height)})
	}
	slots = append(slots, gridSlot{panel: panelSearch, height: searchRowHeight})
	if vis.logs {
		slots = append(slots, gridSlot{panel: panelLogs, height: weightRow(layout.Logs.Height)})
	}
	if vis.helpBar {
		slots = append(slots, gridSlot{panel: panelHelpBar, height: helpBarRowHeight})
	}
	return slots
}

// buildGrid lays the panels out following planGrid. Hidden panels are not added
// to the grid at all. The grid hands the focus to the logs, or to the services
// table when the logs are hidden.
func buildGrid(layout config.LayoutConfig, panels [panelCount]tview.Primitive) *tview.Grid {
	slots := planGrid(layout)
	logsVisible := visibilityOf(layout).logs

	rows := make([]int, len(slots))
	grid := tview.NewGrid().SetColumns(0)
	for i, slot := range slots {
		rows[i] = slot.height
		focus := slot.panel == panelLogs || (slot.panel == panelServices && !logsVisible)
		grid.AddItem(panels[slot.panel], i, 0, 1, 1, 0, 0, focus)
	}
	return grid.SetRows(rows...)
}

// mainPanel is the panel that holds the focus when no overlay (the search
// field or the alerts modal) does: the logs, or the services table when the
// logs are hidden.
func (d *Dashboard) mainPanel() tview.Primitive {
	if d.visible.logs {
		return d.logsView
	}
	return d.servicesTable
}
