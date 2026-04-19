// Package ui comment :)
package ui

import (
	"fmt"
	"time"

	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/internal/filter"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Dashboard struct {
	app           *tview.Application
	servicesTable *tview.Table
	logsView      *tview.TextView
	resourcesView *tview.TextView
	helpBar       *tview.TextView
	grid          *tview.Grid
	searchField   *tview.InputField
	filterState   filter.FilterState
	userScrolling bool
	firstRender   bool
	filterMode    bool
}

func NewDashboard() *Dashboard {
	app := tview.NewApplication()

	servicesTable := tview.NewTable().
		SetBorders(true).
		SetFixed(1, 0)
	servicesTable.SetBorder(true).SetTitle(" Docker Services ").SetTitleAlign(tview.AlignLeft)

	logsView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetChangedFunc(func() {
			app.Draw()
		})
	logsView.SetBorder(true).SetTitle(" Logs ").SetTitleAlign(tview.AlignLeft)

	resourcesView := tview.NewTextView().
		SetDynamicColors(true)
	resourcesView.SetBorder(true).SetTitle(" System Resources ").SetTitleAlign(tview.AlignLeft)

	helpBar := tview.NewTextView().
		SetDynamicColors(true).
		SetText("[/][yellow] Search[white] | [f][yellow] Filter[white] | [Esc][yellow] Clear[white] | [↑↓][yellow] Scroll[white] | [q][yellow] Quit[white]")
	helpBar.SetBorder(false).SetBackgroundColor(tcell.ColorBlack)

	searchField := tview.NewInputField().
		SetLabel("Search: ").
		SetPlaceholder("name, id or image...").
		SetFieldWidth(0)
	searchField.SetBorder(true).SetTitle(" Filter ")

	grid := tview.NewGrid().
		SetRows(0, 0, 3, 0, 1).
		SetColumns(0).
		AddItem(servicesTable, 0, 0, 1, 1, 0, 0, false).
		AddItem(resourcesView, 1, 0, 1, 1, 0, 0, false).
		AddItem(searchField, 2, 0, 1, 1, 0, 0, false).
		AddItem(logsView, 3, 0, 1, 1, 0, 0, true).
		AddItem(helpBar, 4, 0, 1, 1, 0, 0, false)

	app.SetRoot(grid, true)
	app.EnableMouse(true)
	app.SetFocus(logsView)

	dash := &Dashboard{
		app:           app,
		servicesTable: servicesTable,
		logsView:      logsView,
		resourcesView: resourcesView,
		helpBar:       helpBar,
		grid:          grid,
		searchField:   searchField,
		filterState:   filter.NewFilterState(),
		userScrolling: false,
		firstRender:   true,
	}

	logsView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		dash.userScrolling = true
		return event
	})

	logsView.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseScrollUp || action == tview.MouseScrollDown {
			dash.userScrolling = true
		}
		return action, event
	})

	return dash
}

func (d *Dashboard) Update(containers docker.Containers) {
	filtered := filter.FilterContainers(containers, d.filterState)
	d.updateServicesTable(filtered)
	d.updateResourcesView(filtered.Host)
	d.updateLogsView(filtered)
}

func (d *Dashboard) SetupInputCapture() {
	d.searchField.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			d.filterState.Clear()
			d.searchField.SetText("")
			d.app.SetFocus(d.logsView)
			d.filterMode = false
			return nil
		}
		if event.Key() == tcell.KeyEnter {
			d.filterState.SetSearch(d.searchField.GetText())
			d.app.SetFocus(d.logsView)
			d.filterMode = false
			return nil
		}
		return event
	})
}

func (d *Dashboard) updateServicesTable(containers docker.Containers) {
	d.servicesTable.Clear()

	// Headers
	headers := []string{"Service", "State", "Image", "CPU %", "Memory", "Net Bytes (Rx/Tx)", "Net Pkts (Rx/Tx)", "Disk Bytes (R/W)", "Disk Ops (R/W)", "PIDs", "OOM", "Logs"}
	for i, header := range headers {
		d.servicesTable.SetCell(0, i,
			tview.NewTableCell(header).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignCenter).
				SetSelectable(false))
	}

	// Data rows
	for row, c := range containers.C {
		serviceName := c.Service
		if serviceName == "" {
			serviceName = c.ID[:12]
		}

		stateColor := tcell.ColorGreen
		if c.State != "running" {
			stateColor = tcell.ColorRed
		}

		memMB := fmt.Sprintf("%.2f MB", float64(c.MemUsage)/1024/1024)
		cpuStr := fmt.Sprintf("%.2f", c.CPUPercent)
		netBytes := fmt.Sprintf("%s/%s", formatBytes(c.NetRxBytes), formatBytes(c.NetTxBytes))
		netPackets := fmt.Sprintf("%d/%d", c.NetRxPackets, c.NetTxPackets)
		diskBytes := fmt.Sprintf("%s/%s", formatBytes(c.DiskReadBytes), formatBytes(c.DiskWriteBytes))
		diskOps := fmt.Sprintf("%d/%d", c.DiskReadOps, c.DiskWriteOps)
		pids := fmt.Sprintf("%d", c.PIDsCurrent)
		oomEvents := fmt.Sprintf("%d", c.OOMEvents)
		logCount := fmt.Sprintf("%d lines", len(c.Log))
		oomColor := tcell.ColorGreen
		if c.OOMEvents > 0 {
			oomColor = tcell.ColorRed
		}

		cells := []struct {
			text  string
			color tcell.Color
		}{
			{serviceName, tcell.ColorWhite},
			{c.State, stateColor},
			{c.Image, tcell.ColorLightBlue},
			{cpuStr, tcell.ColorWhite},
			{memMB, tcell.ColorWhite},
			{netBytes, tcell.ColorWhite},
			{netPackets, tcell.ColorWhite},
			{diskBytes, tcell.ColorWhite},
			{diskOps, tcell.ColorWhite},
			{pids, tcell.ColorWhite},
			{oomEvents, oomColor},
			{logCount, tcell.ColorGray},
		}

		for col, cell := range cells {
			d.servicesTable.SetCell(row+1, col,
				tview.NewTableCell(cell.text).
					SetTextColor(cell.color).
					SetAlign(tview.AlignLeft))
		}
	}
}

func formatBytes(value uint64) string {
	const unit = 1024.0
	bytes := float64(value)
	if bytes < unit {
		return fmt.Sprintf("%d B", value)
	}
	if bytes < unit*unit {
		return fmt.Sprintf("%.1f KB", bytes/unit)
	}
	if bytes < unit*unit*unit {
		return fmt.Sprintf("%.1f MB", bytes/(unit*unit))
	}
	return fmt.Sprintf("%.1f GB", bytes/(unit*unit*unit))
}

func (d *Dashboard) updateResourcesView(host docker.HostInfo) {
	d.resourcesView.Clear()
	fmt.Fprintf(d.resourcesView, "[yellow]CPU Cores:[-] %d\n\n", host.CPUCount)
	fmt.Fprintf(d.resourcesView, "[yellow]Memory Total:[-] %.2f GB\n", float64(host.MemTotal)/1024/1024/1024)
	fmt.Fprintf(d.resourcesView, "[yellow]Memory Free:[-] %.2f MB\n\n", float64(host.MemFree)/1024/1024)
	fmt.Fprintf(d.resourcesView, "[gray]Updated: %s[-]", time.Now().Format("15:04:05"))
}

var serviceColors = []string{
	"yellow", "cyan", "magenta", "green", "blue", "red",
	"darkcyan", "darkmagenta", "olive", "teal",
}

func (d *Dashboard) getServiceColor(serviceName string, containers docker.Containers) string {
	for i, c := range containers.C {
		name := c.Service
		if name == "" {
			if len(c.ID) >= 12 {
				name = c.ID[:12]
			} else {
				name = c.ID
			}
		}
		if name == serviceName {
			return serviceColors[i%len(serviceColors)]
		}
	}
	return "white"
}

func (d *Dashboard) updateLogsView(containers docker.Containers) {
	row, col := d.logsView.GetScrollOffset()

	d.logsView.Clear()
	for _, fl := range containers.FlatLogs {
		color := d.getServiceColor(fl.Service, containers)
		fmt.Fprintf(d.logsView, "[yellow]%s[-] [%s]%s[-]\n", fl.Service, color, tview.Escape(fl.Line))
	}

	if d.firstRender {
		d.logsView.ScrollToEnd()
		d.firstRender = false
	} else if !d.userScrolling {
		d.logsView.ScrollToEnd()
	} else {
		d.logsView.ScrollTo(row, col)
	}
}

func (d *Dashboard) Run() error {
	d.SetupInputCapture()
	d.app.SetInputCapture(d.handleInput)
	return d.app.Run()
}

func (d *Dashboard) Stop() {
	d.app.Stop()
}

func (d *Dashboard) handleInput(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyRune {
		switch event.Rune() {
		case '/':
			d.app.SetFocus(d.searchField)
			d.filterMode = true
			return nil
		case 'f':
			d.app.SetFocus(d.searchField)
			d.filterMode = true
			return nil
		}
	}
	if event.Key() == tcell.KeyEscape {
		if d.filterMode {
			d.filterState.Clear()
			d.searchField.SetText("")
			d.app.SetFocus(d.logsView)
			d.filterMode = false
			return nil
		}
		d.filterState.Clear()
	}
	return event
}
