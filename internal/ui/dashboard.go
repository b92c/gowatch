package ui

import (
	"fmt"
	"time"

	"github.com/b92c/gowatch/internal/alert"
	"github.com/b92c/gowatch/internal/config"
	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/internal/filter"
	"github.com/b92c/gowatch/internal/trace"
	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Dashboard struct {
	daemonError        error
	app                *tview.Application
	alertsModal        *tview.Table
	searchField        *tview.InputField
	logsView           *tview.TextView
	resourcesView      *tview.TextView
	helpBar            *tview.TextView
	grid               *tview.Grid
	pages              *tview.Pages
	servicesTable      *tview.Table
	daemonErrView      *tview.TextView
	filterState        filter.FilterState
	theme              Theme
	visible            panelVisibility
	userScrolling      bool
	firstRender        bool
	awsViewMode        bool
	showingAlertsModal bool
}

// Options customizes a Dashboard. The zero value gives the built-in look: the
// dark theme and every panel visible with equal heights.
type Options struct {
	// Theme is the resolved color theme; nil means the dark theme.
	Theme *Theme
	// Layout says which panels are shown and their relative heights. It is
	// expected to have passed config.Validate.
	Layout config.LayoutConfig
}

// NewDashboard creates a dashboard with the built-in look.
func NewDashboard() *Dashboard {
	return NewDashboardWithOptions(Options{})
}

// NewDashboardWithOptions creates a dashboard with the given theme and layout.
// It applies the theme to tview's global styles, so it must run before another
// dashboard with a different theme is created.
func NewDashboardWithOptions(opts Options) *Dashboard {
	theme := darkTheme()
	if opts.Theme != nil {
		theme = *opts.Theme
	}
	theme.Apply()

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
		SetText(helpBarText(theme))
	helpBar.SetBorder(false)
	helpBar.SetBackgroundColor(theme.HelpBarBg) // the TextView version, which also colors the text cells

	alertsModal := tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false).
		SetFixed(1, 0)
	alertsModal.SetBorder(true).
		SetTitle(alertsModalTitle(theme)).
		SetTitleAlign(tview.AlignCenter).
		SetBorderColor(theme.StatusError)

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
		SetBorderColor(theme.StatusError)

	errorHelpBar := tview.NewTextView().
		SetDynamicColors(true).
		SetText(errorHelpBarText(theme))
	errorHelpBar.SetBorder(false)
	errorHelpBar.SetBackgroundColor(theme.HelpBarBg)

	grid := buildGrid(opts.Layout, [panelCount]tview.Primitive{
		panelServices:  servicesTable,
		panelResources: resourcesView,
		panelSearch:    searchField,
		panelLogs:      logsView,
		panelHelpBar:   helpBar,
	})

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

	dash := &Dashboard{
		app:           app,
		theme:         theme,
		visible:       visibilityOf(opts.Layout),
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
	app.SetFocus(dash.mainPanel())

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
	if d.visible.resources {
		d.updateResourcesView(filtered.Host, filtered.C, filtered.Traces, containers.Alerts)
	}
	if d.visible.logs {
		d.updateLogsView(filtered)
	}
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
			d.app.SetFocus(d.mainPanel())
			return nil
		}
		if event.Key() == tcell.KeyEnter {
			d.filterState.SetSearch(d.searchField.GetText())
			d.app.SetFocus(d.mainPanel())
			return nil
		}
		return event
	})
}

func (d *Dashboard) updateServicesTable(containers docker.Containers) {
	t := d.theme
	d.servicesTable.Clear()

	if d.awsViewMode {
		d.servicesTable.SetTitle(fmt.Sprintf(" AWS Cloud Resources (%s) ", containers.AWS.Region))
		headers := []string{"Resource Name", "Type", "Region", "Status", "Metrics Summary", "Last Updated"}
		for i, header := range headers {
			cell := tview.NewTableCell(header).
				SetTextColor(t.Accent).
				SetAlign(tview.AlignCenter).
				SetSelectable(false)
			d.servicesTable.SetCell(0, i, cell)
		}

		for row, r := range containers.AWS.Resources {
			statusColor := t.StatusOK
			if r.Status != "Active" && r.Status != "CREATE_COMPLETE" && r.Status != "OK" {
				statusColor = t.StatusWarn
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
				{r.Name, t.Text},
				{r.Type, t.Info},
				{r.Region, t.Muted},
				{r.Status, statusColor},
				{metricsStr, t.Text},
				{r.LastUpdated.Format("15:04:05"), t.Muted},
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
			SetTextColor(t.Accent).
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

		stateColor := t.StatusOK
		if c.State != "running" {
			stateColor = t.StatusError
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
		oomColor := t.StatusOK
		if c.OOMEvents > 0 {
			oomColor = t.StatusError
		}

		cells := []struct {
			text  string
			color tcell.Color
		}{
			{serviceName, t.Text},
			{c.State, stateColor},
			{c.Image, t.Info},
			{cpuStr, t.Text},
			{memStr, t.Text},
			{netBytes, t.Text},
			{netPackets, t.Text},
			{diskBytes, t.Text},
			{diskOps, t.Text},
			{pids, t.Text},
			{oomEvents, oomColor},
			{logCount, t.Muted},
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
	th := d.theme
	accent, key, muted := th.Tag(th.Accent), th.Tag(th.HelpKey), th.Tag(th.Muted)
	okTag, warnTag, errTag := th.Tag(th.StatusOK), th.Tag(th.StatusWarn), th.Tag(th.StatusError)
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

	alertSummary := okTag + "OK[-]"
	if critCount > 0 || warnCount > 0 {
		alertSummary = fmt.Sprintf("%s%d CRIT[-], %s%d WARN[-] (%sw[-] for details)", errTag, critCount, warnTag, warnCount, key)
	}

	fmt.Fprintf(d.resourcesView, accent+"CPU Cores (Total):[-] %d  |  "+accent+"CPU Active:[-] %.2f cores (%.1f%%)  |  "+accent+"Alerts:[-] %s\n", host.CPUCount, cpuCoresUsed, totalCPU, alertSummary)
	fmt.Fprintf(d.resourcesView, accent+"Memory Total:[-] %.2f GB  |  "+accent+"Used:[-] %.2f MB  |  "+accent+"Free:[-] %.2f MB\n", float64(host.MemTotal)/1024/1024/1024, float64(totalMem)/1024/1024, float64(host.MemFree)/1024/1024)

	errTraces := 0
	for _, t := range traces {
		if t.HasError {
			errTraces++
		}
	}
	traceStatus := okTag + "OK[-]"
	if errTraces > 0 {
		traceStatus = fmt.Sprintf("%s%d Error(s)[-]", errTag, errTraces)
	}
	fmt.Fprintf(d.resourcesView, accent+"Traces Active:[-] %d (%s)  |  "+muted+"Updated: %s[-]", len(traces), traceStatus, time.Now().Format("15:04:05"))

	if len(traces) > 0 {
		fmt.Fprintf(d.resourcesView, "\n%sRecent Traces:[-]\n", th.Tag(th.Subheading))
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
				errFlag = " " + errTag + "[ERR][-]"
			}
			tID := tr.TraceID
			if len(tID) > 12 {
				tID = tID[:12]
			}
			fmt.Fprintf(d.resourcesView, "  • "+accent+"%s[-] (%d spans, %v) [%s] %s%s", tID, len(tr.Spans), tr.Duration.Truncate(time.Millisecond), rootSvc, rootOp, errFlag)
		}
	}
}

func (d *Dashboard) updateAlertsModal(alerts []alert.Alert) {
	t := d.theme
	d.alertsModal.Clear()

	headers := []string{"Status", "Severity", "Service", "Message", "Value", "Fired At"}
	for i, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(t.Accent).
			SetAlign(tview.AlignCenter).
			SetSelectable(false)
		d.alertsModal.SetCell(0, i, cell)
	}

	if len(alerts) == 0 {
		cell := tview.NewTableCell("No active or recent alerts").
			SetTextColor(t.StatusOK).
			SetAlign(tview.AlignCenter)
		d.alertsModal.SetCell(1, 0, cell)
		return
	}

	for row, a := range alerts {
		statusColor := t.StatusError
		if a.Status == alert.StatusAcknowledged {
			statusColor = t.StatusWarn
		} else if a.Status == alert.StatusResolved {
			statusColor = t.StatusOK
		}

		severityColor := t.StatusWarn
		if a.Severity == alert.SeverityCritical {
			severityColor = t.StatusError
		}

		firedStr := a.FiredAt.Format("15:04:05")
		valStr := fmt.Sprintf("%.1f", a.Value)

		cells := []struct {
			text  string
			color tcell.Color
		}{
			{string(a.Status), statusColor},
			{string(a.Severity), severityColor},
			{a.ServiceName, t.Text},
			{a.Message, t.Text},
			{valStr, t.Text},
			{firedStr, t.Muted},
		}

		for col, cell := range cells {
			tableCell := tview.NewTableCell(cell.text).
				SetTextColor(cell.color).
				SetAlign(tview.AlignLeft)
			d.alertsModal.SetCell(row+1, col, tableCell)
		}
	}
}

// getServiceColor returns the name of the palette color of a service, to be
// used as a tview tag, [name]. It follows the position of the service in the
// container list; services that are not listed get the regular text color.
func (d *Dashboard) getServiceColor(serviceName string, containers docker.Containers) string {
	palette := d.theme.ServicePalette
	if len(palette) > 0 {
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
				return colorName(palette[i%len(palette)])
			}
		}
	}
	return colorName(d.theme.Text)
}

// formatLogBadge returns the tag text of a level badge. tview reads the
// bracketed level name ([INFO], [FATAL]...) as a color tag, so the name itself
// is not drawn; the original dashboard behaved the same way and the dark theme
// must keep these exact strings.
func (d *Dashboard) formatLogBadge(level metrics.LogLevel) string {
	t := d.theme
	switch level {
	case metrics.LogLevelFatal:
		return "[" + colorName(t.LogFatalText) + ":" + colorName(t.LogFatal) + ":b][FATAL][-]"
	case metrics.LogLevelError:
		return "[" + colorName(t.LogError) + "::b][ERROR][-]"
	case metrics.LogLevelWarn:
		return "[" + colorName(t.LogWarn) + "::b][WARN][-]"
	case metrics.LogLevelInfo:
		return t.Tag(t.LogInfo) + "[INFO][-]"
	case metrics.LogLevelDebug:
		return t.Tag(t.LogDebug) + "[DEBUG][-]"
	case metrics.LogLevelTrace:
		return t.Tag(t.LogDebug) + "[TRACE][-]"
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
	accent := d.theme.Tag(d.theme.Accent)
	for _, fl := range containers.FlatLogs {
		color := d.getServiceColor(fl.Service, containers)
		badge := d.formatLogBadge(fl.Level)
		if badge != "" {
			badge = badge + " "
		}
		fmt.Fprintf(d.logsView, "%s%s[-] %s[%s]%s[-]\n", accent, fl.Service, badge, color, tview.Escape(fl.Line))
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
				d.app.SetFocus(d.mainPanel())
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
			d.app.SetFocus(d.mainPanel())
			return nil
		}
		d.filterState.Clear()
		d.searchField.SetText("")
		d.app.SetFocus(d.mainPanel())
		return nil
	}
	return event
}

func (d *Dashboard) ShowDaemonError(err error) {
	d.app.QueueUpdateDraw(func() {
		d.daemonError = err
		d.daemonErrView.Clear()
		fmt.Fprint(d.daemonErrView, daemonErrorText(d.theme, err))
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
