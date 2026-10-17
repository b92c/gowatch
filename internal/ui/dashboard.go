package ui

import (
	"fmt"
	"time"

	"github.com/b92c/gowatch/internal/alert"
	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/internal/filter"
	"github.com/b92c/gowatch/internal/trace"
	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Dashboard struct {
	daemonError        error
	searchField        *tview.InputField
	logsView           *tview.TextView
	resourcesView      *tview.TextView
	helpBar            *tview.TextView
	grid               *tview.Grid
	app                *tview.Application
	servicesTable      *tview.Table
	alertsModal        *tview.Table
	daemonErrView      *tview.TextView
	pages              *tview.Pages
	filterState        filter.FilterState
	userScrolling      bool
	firstRender        bool
	awsViewMode        bool
	showingAlertsModal bool
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
		SetText("[yellow]/[white] Search | [yellow]f[white] Filter | [yellow]l[white] Log Level | [yellow]w[white] Alerts | [yellow]d[white] Docker View | [yellow]a[white] AWS View | [yellow]Esc[white] Clear | [yellow]↑↓[white] Scroll | [yellow]q[white] Quit")
	helpBar.SetBorder(false).SetBackgroundColor(tcell.ColorBlack)

	alertsModal := tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false).
		SetFixed(1, 0)
	alertsModal.SetBorder(true).
		SetTitle(" 🚨 Active & Recent Alerts ([yellow]Esc[white]/[yellow]w[white] Close | [yellow]c[white] Clear Resolved) ").
		SetTitleAlign(tview.AlignCenter).
		SetBorderColor(tcell.ColorRed)

	searchField := tview.NewInputField().
		SetLabel("Search: ").
		SetPlaceholder("name, id or image...").
		SetFieldWidth(0)
	searchField.SetBorder(true).SetTitle(" Filter ")

	daemonErrView := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	daemonErrView.SetBorder(true).
		SetTitle(" ⚠ Docker Daemon Offline ").
		SetTitleAlign(tview.AlignCenter).
		SetBorderColor(tcell.ColorRed)

	errorHelpBar := tview.NewTextView().
		SetDynamicColors(true).
		SetText("[q][yellow] Quit[white] | [gray]Waiting for Docker daemon...[-]")
	errorHelpBar.SetBorder(false).SetBackgroundColor(tcell.ColorBlack)

	grid := tview.NewGrid().
		SetRows(0, 0, 3, 0, 1).
		SetColumns(0).
		AddItem(servicesTable, 0, 0, 1, 1, 0, 0, false).
		AddItem(resourcesView, 1, 0, 1, 1, 0, 0, false).
		AddItem(searchField, 2, 0, 1, 1, 0, 0, false).
		AddItem(logsView, 3, 0, 1, 1, 0, 0, true).
		AddItem(helpBar, 4, 0, 1, 1, 0, 0, false)

	errorGrid := tview.NewGrid().
		SetRows(0, 1).
		SetColumns(0).
		AddItem(daemonErrView, 0, 0, 1, 1, 0, 0, false).
		AddItem(errorHelpBar, 1, 0, 1, 1, 0, 0, false)

	pages := tview.NewPages().
		AddPage("dashboard", grid, true, true).
		AddPage("alerts_modal", alertsModal, true, false).
		AddPage("error", errorGrid, true, false)

	app.SetRoot(pages, true)
	app.EnableMouse(true)
	app.SetFocus(logsView)

	dash := &Dashboard{
		app:           app,
		servicesTable: servicesTable,
		alertsModal:   alertsModal,
		logsView:      logsView,
		resourcesView: resourcesView,
		helpBar:       helpBar,
		grid:          grid,
		pages:         pages,
		daemonErrView: daemonErrView,
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

// ApplyInitialFilters sets the filters the dashboard starts with, for example
// the ones from the config file, and shows the search text in the search field.
// Empty values leave that filter off. Esc still clears all of them.
func (d *Dashboard) ApplyInitialFilters(search string, status []string, minLogLevel metrics.LogLevel) {
	d.filterState.SetSearch(search)
	d.filterState.SetStatusFilter(append([]string(nil), status...))
	d.filterState.SetMinLogLevel(minLogLevel)
	d.searchField.SetText(d.filterState.SearchText)
}

func (d *Dashboard) Update(containers docker.Containers) {
	filtered := filter.FilterContainers(containers, d.filterState)
	d.updateServicesTable(filtered)
	d.updateResourcesView(filtered.Host, filtered.C, filtered.Traces, containers.Alerts)
	d.updateLogsView(filtered)
	d.updateAlertsModal(containers.Alerts)
}

func (d *Dashboard) SetupInputCapture() {
	d.searchField.SetChangedFunc(func(text string) {
		d.filterState.SetSearch(text)
	})

	d.searchField.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			d.filterState.Clear()
			d.searchField.SetText("")
			d.app.SetFocus(d.logsView)
			return nil
		}
		if event.Key() == tcell.KeyEnter {
			d.filterState.SetSearch(d.searchField.GetText())
			d.app.SetFocus(d.logsView)
			return nil
		}
		return event
	})
}

func (d *Dashboard) updateServicesTable(containers docker.Containers) {
	d.servicesTable.Clear()

	if d.awsViewMode {
		d.servicesTable.SetTitle(fmt.Sprintf(" AWS Cloud Resources (%s) ", containers.AWS.Region))
		headers := []string{"Resource Name", "Type", "Region", "Status", "Metrics Summary", "Last Updated"}
		for i, header := range headers {
			cell := tview.NewTableCell(header).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignCenter).
				SetSelectable(false)
			d.servicesTable.SetCell(0, i, cell)
		}

		for row, r := range containers.AWS.Resources {
			statusColor := tcell.ColorGreen
			if r.Status != "Active" && r.Status != "CREATE_COMPLETE" && r.Status != "OK" {
				statusColor = tcell.ColorYellow
			}

			metricsStr := ""
			for k, v := range r.Metrics {
				metricsStr += fmt.Sprintf("%s:%.1f ", k, v)
			}
			if metricsStr == "" {
				metricsStr = "-"
			}

			cells := []struct {
				text  string
				color tcell.Color
			}{
				{r.Name, tcell.ColorWhite},
				{r.Type, tcell.ColorLightBlue},
				{r.Region, tcell.ColorGray},
				{r.Status, statusColor},
				{metricsStr, tcell.ColorWhite},
				{r.LastUpdated.Format("15:04:05"), tcell.ColorGray},
			}

			for col, cell := range cells {
				tableCell := tview.NewTableCell(cell.text).
					SetTextColor(cell.color).
					SetAlign(tview.AlignLeft)
				d.servicesTable.SetCell(row+1, col, tableCell)
			}
		}
		return
	}

	d.servicesTable.SetTitle(" Docker Services ")

	headers := []string{"Service", "State", "Image", "CPU %", "Memory", "Net Rx/Tx", "Net Pkts", "Disk R/W", "Disk Ops", "PIDs", "OOM", "Logs"}
	for i, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignCenter).
			SetSelectable(false)
		if i == 2 {
			cell.SetMaxWidth(25)
		}
		d.servicesTable.SetCell(0, i, cell)
	}

	for row, c := range containers.C {
		serviceName := c.Service
		if serviceName == "" {
			serviceName = c.ID[:12]
		}

		stateColor := tcell.ColorGreen
		if c.State != "running" {
			stateColor = tcell.ColorRed
		}

		cpuSpark := tview.Escape(RenderSparkline(c.CPUHistory, 10))
		memSpark := tview.Escape(RenderSparkline(c.MemHistory, 10))

		memMB := fmt.Sprintf("%.2f MB", float64(c.MemUsage)/1024/1024)
		cpuStr := fmt.Sprintf("%s %.2f%%", cpuSpark, c.CPUPercent)
		memStr := fmt.Sprintf("%s %s", memSpark, memMB)

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
			{memStr, tcell.ColorWhite},
			{netBytes, tcell.ColorWhite},
			{netPackets, tcell.ColorWhite},
			{diskBytes, tcell.ColorWhite},
			{diskOps, tcell.ColorWhite},
			{pids, tcell.ColorWhite},
			{oomEvents, oomColor},
			{logCount, tcell.ColorGray},
		}

		for col, cell := range cells {
			tableCell := tview.NewTableCell(cell.text).
				SetTextColor(cell.color).
				SetAlign(tview.AlignLeft)
			if col == 2 {
				tableCell.SetMaxWidth(25)
			}
			d.servicesTable.SetCell(row+1, col, tableCell)
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

func (d *Dashboard) updateResourcesView(host metrics.HostInfo, containers []docker.Container, traces []trace.Trace, alerts []alert.Alert) {
	d.resourcesView.Clear()

	var totalCPU float64
	var totalMem uint64
	for _, c := range containers {
		totalCPU += c.CPUPercent
		totalMem += c.MemUsage
	}

	cpuCoresUsed := totalCPU / 100.0

	critCount := 0
	warnCount := 0
	for _, a := range alerts {
		if a.Status == alert.StatusFiring || a.Status == alert.StatusAcknowledged {
			if a.Severity == alert.SeverityCritical {
				critCount++
			} else if a.Severity == alert.SeverityWarning {
				warnCount++
			}
		}
	}

	alertSummary := "[green]OK[-]"
	if critCount > 0 || warnCount > 0 {
		alertSummary = fmt.Sprintf("[red]%d CRIT[-], [yellow]%d WARN[-] ([yellow]w[-] for details)", critCount, warnCount)
	}

	fmt.Fprintf(d.resourcesView, "[yellow]CPU Cores (Total):[-] %d  |  [yellow]CPU Active:[-] %.2f cores (%.1f%%)  |  [yellow]Alerts:[-] %s\n", host.CPUCount, cpuCoresUsed, totalCPU, alertSummary)
	fmt.Fprintf(d.resourcesView, "[yellow]Memory Total:[-] %.2f GB  |  [yellow]Used:[-] %.2f MB  |  [yellow]Free:[-] %.2f MB\n", float64(host.MemTotal)/1024/1024/1024, float64(totalMem)/1024/1024, float64(host.MemFree)/1024/1024)

	errTraces := 0
	for _, t := range traces {
		if t.HasError {
			errTraces++
		}
	}
	traceStatus := "[green]OK[-]"
	if errTraces > 0 {
		traceStatus = fmt.Sprintf("[red]%d Error(s)[-]", errTraces)
	}
	fmt.Fprintf(d.resourcesView, "[yellow]Traces Active:[-] %d (%s)  |  [gray]Updated: %s[-]", len(traces), traceStatus, time.Now().Format("15:04:05"))

	if len(traces) > 0 {
		fmt.Fprintf(d.resourcesView, "\n[cyan]Recent Traces:[-]\n")
		startIdx := len(traces) - 2
		if startIdx < 0 {
			startIdx = 0
		}
		for i := len(traces) - 1; i >= startIdx; i-- {
			tr := traces[i]
			rootOp := "unknown"
			rootSvc := "unknown"
			if tr.RootSpan != nil {
				rootOp = tr.RootSpan.OperationName
				rootSvc = tr.RootSpan.ServiceName
			}
			errFlag := ""
			if tr.HasError {
				errFlag = " [red][ERR][-]"
			}
			tID := tr.TraceID
			if len(tID) > 12 {
				tID = tID[:12]
			}
			fmt.Fprintf(d.resourcesView, "  • [yellow]%s[-] (%d spans, %v) [%s] %s%s", tID, len(tr.Spans), tr.Duration.Truncate(time.Millisecond), rootSvc, rootOp, errFlag)
		}
	}
}

func (d *Dashboard) updateAlertsModal(alerts []alert.Alert) {
	d.alertsModal.Clear()

	headers := []string{"Status", "Severity", "Service", "Message", "Value", "Fired At"}
	for i, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignCenter).
			SetSelectable(false)
		d.alertsModal.SetCell(0, i, cell)
	}

	if len(alerts) == 0 {
		cell := tview.NewTableCell("No active or recent alerts").
			SetTextColor(tcell.ColorGreen).
			SetAlign(tview.AlignCenter)
		d.alertsModal.SetCell(1, 0, cell)
		return
	}

	for row, a := range alerts {
		statusColor := tcell.ColorRed
		if a.Status == alert.StatusAcknowledged {
			statusColor = tcell.ColorYellow
		} else if a.Status == alert.StatusResolved {
			statusColor = tcell.ColorGreen
		}

		severityColor := tcell.ColorYellow
		if a.Severity == alert.SeverityCritical {
			severityColor = tcell.ColorRed
		}

		firedStr := a.FiredAt.Format("15:04:05")
		valStr := fmt.Sprintf("%.1f", a.Value)

		cells := []struct {
			text  string
			color tcell.Color
		}{
			{string(a.Status), statusColor},
			{string(a.Severity), severityColor},
			{a.ServiceName, tcell.ColorWhite},
			{a.Message, tcell.ColorWhite},
			{valStr, tcell.ColorWhite},
			{firedStr, tcell.ColorGray},
		}

		for col, cell := range cells {
			tableCell := tview.NewTableCell(cell.text).
				SetTextColor(cell.color).
				SetAlign(tview.AlignLeft)
			d.alertsModal.SetCell(row+1, col, tableCell)
		}
	}
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

func formatLogBadge(level metrics.LogLevel) string {
	switch level {
	case metrics.LogLevelFatal:
		return "[white:red:b][FATAL][-]"
	case metrics.LogLevelError:
		return "[red::b][ERROR][-]"
	case metrics.LogLevelWarn:
		return "[yellow::b][WARN][-]"
	case metrics.LogLevelInfo:
		return "[green][INFO][-]"
	case metrics.LogLevelDebug:
		return "[darkgray][DEBUG][-]"
	case metrics.LogLevelTrace:
		return "[darkgray][TRACE][-]"
	default:
		return ""
	}
}

func (d *Dashboard) updateLogsView(containers docker.Containers) {
	row, col := d.logsView.GetScrollOffset()

	title := " Logs "
	if d.filterState.MinLogLevel > metrics.LogLevelUnknown {
		title = fmt.Sprintf(" Logs [%s+] ", d.filterState.MinLogLevel.String())
	}
	d.logsView.SetTitle(title)

	d.logsView.Clear()
	for _, fl := range containers.FlatLogs {
		color := d.getServiceColor(fl.Service, containers)
		badge := formatLogBadge(fl.Level)
		if badge != "" {
			badge = badge + " "
		}
		fmt.Fprintf(d.logsView, "[yellow]%s[-] %s[%s]%s[-]\n", fl.Service, badge, color, tview.Escape(fl.Line))
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
				d.app.SetFocus(d.logsView)
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
			d.app.SetFocus(d.logsView)
			return nil
		}
		d.filterState.Clear()
		d.searchField.SetText("")
		d.app.SetFocus(d.logsView)
		return nil
	}
	return event
}

func (d *Dashboard) ShowDaemonError(err error) {
	d.app.QueueUpdateDraw(func() {
		d.daemonError = err
		d.daemonErrView.Clear()
		fmt.Fprintf(d.daemonErrView,
			"\n\n\n"+
				"    [red]██████╗  █████╗ ███████╗███╗   ███╗ ██████╗ ███╗   ██╗[-]\n"+
				"    [red]██╔══██╗██╔══██╗██╔════╝████╗ ████║██╔═══██╗████╗  ██║[-]\n"+
				"    [red]██║  ██║███████║█████╗  ██╔████╔██║██║   ██║██╔██╗ ██║[-]\n"+
				"    [red]██║  ██║██╔══██║██╔══╝  ██║╚██╔╝██║██║   ██║██║╚██╗██║[-]\n"+
				"    [red]██████╔╝██║  ██║███████╗██║ ╚═╝ ██║╚██████╔╝██║ ╚████║[-]\n"+
				"    [red]╚═════╝ ╚═╝  ╚═╝╚══════╝╚═╝     ╚═╝ ╚═════╝ ╚═╝  ╚═══╝[-]\n\n"+
				"    [red::b]⚠  Docker Daemon is not running[-]\n\n"+
				"    [white]GoWatch requires the Docker daemon to monitor containers.[-]\n"+
				"    [white]Please start Docker and GoWatch will reconnect automatically.[-]\n\n"+
				"    [gray]How to start Docker:[-]\n"+
				"    [yellow]  • macOS:[white]  Open Docker Desktop application[-]\n"+
				"    [yellow]  • Linux:[white]  sudo systemctl start docker[-]\n\n"+
				"    [gray]Error: %s[-]\n\n"+
				"    [darkgray]Retrying connection every 2 seconds...[-]",
			err.Error(),
		)
		d.pages.SwitchToPage("error")
	})
}

func (d *Dashboard) ClearDaemonError() {
	d.app.QueueUpdateDraw(func() {
		if d.daemonError == nil {
			return
		}
		d.daemonError = nil
		d.pages.SwitchToPage("dashboard")
	})
}
