package ui

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/b92c/gowatch/internal/alert"
	"github.com/b92c/gowatch/internal/aws"
	"github.com/b92c/gowatch/internal/config"
	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/internal/trace"
	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// These tests protect the promise that, without a theme in the configuration,
// the dashboard looks exactly as it did before themes existed. The legacy*
// values below are copied from internal/ui/dashboard.go at commit 0a4c422, the
// last version with the colors written in the code, and the dark theme has to
// reproduce them character for character.

// legacyHelpBar, legacyErrorHelpBar and legacyAlertsTitle are the texts of the
// help bar, of the help bar of the "Docker daemon offline" page and of the
// alerts modal title.
const (
	legacyHelpBar      = "[yellow]/[white] Search | [yellow]f[white] Filter | [yellow]l[white] Log Level | [yellow]w[white] Alerts | [yellow]d[white] Docker View | [yellow]a[white] AWS View | [yellow]Esc[white] Clear | [yellow]↑↓[white] Scroll | [yellow]q[white] Quit"
	legacyErrorHelpBar = "[q][yellow] Quit[white] | [gray]Waiting for Docker daemon...[-]"
	legacyAlertsTitle  = " 🚨 Active & Recent Alerts ([yellow]Esc[white]/[yellow]w[white] Close | [yellow]c[white] Clear Resolved) "
)

// legacyDaemonErrorFormat is the format of the "Docker daemon offline" page; its
// only argument is the error text.
const legacyDaemonErrorFormat = "\n\n\n" +
	"    [red]██████╗  █████╗ ███████╗███╗   ███╗ ██████╗ ███╗   ██╗[-]\n" +
	"    [red]██╔══██╗██╔══██╗██╔════╝████╗ ████║██╔═══██╗████╗  ██║[-]\n" +
	"    [red]██║  ██║███████║█████╗  ██╔████╔██║██║   ██║██╔██╗ ██║[-]\n" +
	"    [red]██║  ██║██╔══██║██╔══╝  ██║╚██╔╝██║██║   ██║██║╚██╗██║[-]\n" +
	"    [red]██████╔╝██║  ██║███████╗██║ ╚═╝ ██║╚██████╔╝██║ ╚████║[-]\n" +
	"    [red]╚═════╝ ╚═╝  ╚═╝╚══════╝╚═╝     ╚═╝ ╚═════╝ ╚═╝  ╚═══╝[-]\n\n" +
	"    [red::b]⚠  Docker Daemon is not running[-]\n\n" +
	"    [white]GoWatch requires the Docker daemon to monitor containers.[-]\n" +
	"    [white]Please start Docker and GoWatch will reconnect automatically.[-]\n\n" +
	"    [gray]How to start Docker:[-]\n" +
	"    [yellow]  • macOS:[white]  Open Docker Desktop application[-]\n" +
	"    [yellow]  • Linux:[white]  sudo systemctl start docker[-]\n\n" +
	"    [gray]Error: %s[-]\n\n" +
	"    [darkgray]Retrying connection every 2 seconds...[-]"

var legacyServiceColors = []string{
	"yellow", "cyan", "magenta", "green", "blue", "red",
	"darkcyan", "darkmagenta", "olive", "teal",
}

func legacyGetServiceColor(serviceName string, containers docker.Containers) string {
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
			return legacyServiceColors[i%len(legacyServiceColors)]
		}
	}
	return "white"
}

func legacyFormatLogBadge(level metrics.LogLevel) string {
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

func legacyResourcesText(host metrics.HostInfo, containers []docker.Container, traces []trace.Trace, alerts []alert.Alert) string {
	var out strings.Builder

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

	fmt.Fprintf(&out, "[yellow]CPU Cores (Total):[-] %d  |  [yellow]CPU Active:[-] %.2f cores (%.1f%%)  |  [yellow]Alerts:[-] %s\n", host.CPUCount, cpuCoresUsed, totalCPU, alertSummary)
	fmt.Fprintf(&out, "[yellow]Memory Total:[-] %.2f GB  |  [yellow]Used:[-] %.2f MB  |  [yellow]Free:[-] %.2f MB\n", float64(host.MemTotal)/1024/1024/1024, float64(totalMem)/1024/1024, float64(host.MemFree)/1024/1024)

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
	fmt.Fprintf(&out, "[yellow]Traces Active:[-] %d (%s)  |  [gray]Updated: %s[-]", len(traces), traceStatus, time.Now().Format("15:04:05"))

	if len(traces) > 0 {
		fmt.Fprintf(&out, "\n[cyan]Recent Traces:[-]\n")
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
			fmt.Fprintf(&out, "  • [yellow]%s[-] (%d spans, %v) [%s] %s%s", tID, len(tr.Spans), tr.Duration.Truncate(time.Millisecond), rootSvc, rootOp, errFlag)
		}
	}
	return out.String()
}

// legacyStyles are tview's default styles (tview v0.42.0, styles.go), which the
// dashboard used without changing them.
var legacyStyles = tview.Theme{
	PrimitiveBackgroundColor:    tcell.ColorBlack,
	ContrastBackgroundColor:     tcell.ColorBlue,
	MoreContrastBackgroundColor: tcell.ColorGreen,
	BorderColor:                 tcell.ColorWhite,
	TitleColor:                  tcell.ColorWhite,
	GraphicsColor:               tcell.ColorWhite,
	PrimaryTextColor:            tcell.ColorWhite,
	SecondaryTextColor:          tcell.ColorYellow,
	TertiaryTextColor:           tcell.ColorGreen,
	InverseTextColor:            tcell.ColorBlue,
	ContrastSecondaryTextColor:  tcell.ColorNavy,
}

// legacyContainers has twelve services, more than the ten palette colors, plus
// two without a service name.
func legacyContainers() docker.Containers {
	var containers docker.Containers
	for i := 0; i < 12; i++ {
		containers.C = append(containers.C, docker.Container{
			ID:      fmt.Sprintf("%02dcccccccccccccccc", i),
			Service: fmt.Sprintf("svc%d", i),
		})
	}
	containers.C = append(containers.C,
		docker.Container{ID: "abcdef0123456789abcdef"},
		docker.Container{ID: "short"},
	)
	return containers
}

func TestDarkThemeLogBadgesMatchTheOriginal(t *testing.T) {
	d := NewDashboard()
	levels := []metrics.LogLevel{
		metrics.LogLevelUnknown, metrics.LogLevelTrace, metrics.LogLevelDebug, metrics.LogLevelInfo,
		metrics.LogLevelWarn, metrics.LogLevelError, metrics.LogLevelFatal, metrics.LogLevel(99),
	}
	for _, level := range levels {
		if got, want := d.formatLogBadge(level), legacyFormatLogBadge(level); got != want {
			t.Errorf("formatLogBadge(%v) = %q, want the original %q", level, got, want)
		}
	}
}

func TestDarkThemeServiceColorsMatchTheOriginal(t *testing.T) {
	d := NewDashboard()
	containers := legacyContainers()

	names := []string{"not-listed", ""}
	for _, c := range containers.C {
		names = append(names, c.Service, c.ID, c.ID[:min(12, len(c.ID))])
	}
	for _, name := range names {
		if got, want := d.getServiceColor(name, containers), legacyGetServiceColor(name, containers); got != want {
			t.Errorf("getServiceColor(%q) = %q, want the original %q", name, got, want)
		}
	}

	t.Run("the palette has the original ten colors in the original order", func(t *testing.T) {
		var got []string
		for i := range legacyServiceColors {
			got = append(got, d.getServiceColor(containers.C[i].Service, containers))
		}
		if !reflect.DeepEqual(got, legacyServiceColors) {
			t.Errorf("palette = %v, want %v", got, legacyServiceColors)
		}
	})

	t.Run("an empty palette falls back to the text color", func(t *testing.T) {
		theme := darkTheme()
		theme.ServicePalette = nil
		d := newTestDashboard(t, Options{Theme: &theme})
		if got := d.getServiceColor("svc0", containers); got != "white" {
			t.Errorf("getServiceColor() with no palette = %q, want %q", got, "white")
		}
	})
}

func TestDarkThemeLogLinesMatchTheOriginal(t *testing.T) {
	d := newTestDashboard(t, Options{})
	containers := legacyContainers()
	for i, level := range []metrics.LogLevel{
		metrics.LogLevelUnknown, metrics.LogLevelTrace, metrics.LogLevelDebug, metrics.LogLevelInfo,
		metrics.LogLevelWarn, metrics.LogLevelError, metrics.LogLevelFatal,
	} {
		containers.FlatLogs = append(containers.FlatLogs, docker.FormattedLog{
			Service: containers.C[i].Service, Line: fmt.Sprintf("line [%d] with [brackets]", i), Level: level,
		})
	}
	var want strings.Builder
	for _, fl := range containers.FlatLogs {
		badge := legacyFormatLogBadge(fl.Level)
		if badge != "" {
			badge += " "
		}
		fmt.Fprintf(&want, "[yellow]%s[-] %s[%s]%s[-]\n", fl.Service, badge, legacyGetServiceColor(fl.Service, containers), tview.Escape(fl.Line))
	}

	d.updateLogsView(containers)

	if got := d.logsView.GetText(false); got != want.String() {
		t.Errorf("logs view text:\n%q\nwant the original:\n%q", got, want.String())
	}
}

func TestDarkThemeResourcesViewMatchesTheOriginal(t *testing.T) {
	root := &trace.Span{OperationName: "GET /users", ServiceName: "api"}
	traces := []trace.Trace{
		{TraceID: "11111111111111111111", RootSpan: root, Spans: []*trace.Span{root}, Duration: 1234 * time.Millisecond},
		{TraceID: "short", HasError: true, Duration: 50 * time.Millisecond},
		{TraceID: "33333333", RootSpan: root, Spans: []*trace.Span{root, root}, Duration: time.Second, HasError: true},
	}
	alerts := []alert.Alert{
		{Status: alert.StatusFiring, Severity: alert.SeverityCritical},
		{Status: alert.StatusAcknowledged, Severity: alert.SeverityWarning},
		{Status: alert.StatusFiring, Severity: alert.SeverityWarning},
		{Status: alert.StatusResolved, Severity: alert.SeverityCritical},
	}
	host := metrics.HostInfo{CPUCount: 8, MemTotal: 16 << 30, MemFree: 4 << 30}
	containers := []docker.Container{{CPUPercent: 12.5, MemUsage: 100 << 20}, {CPUPercent: 30, MemUsage: 50 << 20}}

	tests := []struct {
		name   string
		traces []trace.Trace
		alerts []alert.Alert
	}{
		{name: "nothing to report"},
		{name: "alerts", alerts: alerts},
		{name: "one trace", traces: traces[:1]},
		{name: "traces with errors and a missing root span", traces: traces},
		{name: "traces and alerts", traces: traces, alerts: alerts},
	}
	// The line that shows the clock cannot match to the second.
	clock := regexpClock()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDashboard(t, Options{})

			d.updateResourcesView(host, containers, tt.traces, tt.alerts)

			got := clock.ReplaceAllString(d.resourcesView.GetText(false), "Updated: HH:MM:SS")
			want := clock.ReplaceAllString(legacyResourcesText(host, containers, tt.traces, tt.alerts), "Updated: HH:MM:SS")
			if got != want {
				t.Errorf("resources view:\n%q\nwant the original:\n%q", got, want)
			}
		})
	}
}

func TestDarkThemeTextsMatchTheOriginal(t *testing.T) {
	d := newTestDashboard(t, Options{})
	theme := darkTheme()

	if got := d.helpBar.GetText(false); got != legacyHelpBar {
		t.Errorf("help bar:\n%q\nwant the original:\n%q", got, legacyHelpBar)
	}
	if got := helpBarText(theme); got != legacyHelpBar {
		t.Errorf("helpBarText():\n%q\nwant the original:\n%q", got, legacyHelpBar)
	}
	if got := errorHelpBarText(theme); got != legacyErrorHelpBar {
		t.Errorf("errorHelpBarText():\n%q\nwant the original:\n%q", got, legacyErrorHelpBar)
	}
	if got := d.alertsModal.GetTitle(); got != legacyAlertsTitle {
		t.Errorf("alerts modal title:\n%q\nwant the original:\n%q", got, legacyAlertsTitle)
	}
	for _, msg := range []string{"Cannot connect to the Docker daemon", "100% broken [x]"} {
		err := fmt.Errorf("%s", msg)
		if got, want := daemonErrorText(theme, err), fmt.Sprintf(legacyDaemonErrorFormat, err.Error()); got != want {
			t.Errorf("daemonErrorText(%q):\n%q\nwant the original:\n%q", msg, got, want)
		}
	}
}

func TestDarkThemeBordersAndBackgroundsMatchTheOriginal(t *testing.T) {
	d := newTestDashboard(t, Options{})

	if got := d.helpBar.GetBackgroundColor(); got != tcell.ColorBlack {
		t.Errorf("help bar background = %v, want black", got)
	}
	if got := d.alertsModal.GetBorderColor(); got != tcell.ColorRed {
		t.Errorf("alerts modal border = %v, want red", got)
	}
	if got := d.daemonErrView.GetBorderColor(); got != tcell.ColorRed {
		t.Errorf("daemon error border = %v, want red", got)
	}
	for name, p := range map[string]interface{ GetBackgroundColor() tcell.Color }{
		"services": d.servicesTable, "logs": d.logsView, "resources": d.resourcesView, "search": d.searchField,
	} {
		if got := p.GetBackgroundColor(); got != tcell.ColorBlack {
			t.Errorf("%s background = %v, want black", name, got)
		}
	}

	// The search field takes its colors from tview's styles: a blue field with
	// navy placeholder text and a yellow label.
	shot := drawDashboard(t, d, 120, 30)
	if fg, bg := shot.colorsOf(t, "name, id or image..."); fg != tcell.ColorNavy || bg != tcell.ColorBlue {
		t.Errorf("search placeholder = %v on %v, want navy on blue", fg, bg)
	}
	if fg, _ := shot.colorsOf(t, "Search:"); fg != tcell.ColorYellow {
		t.Errorf("search label = %v, want yellow", fg)
	}
}

func TestDarkThemeLeavesTviewStylesAtTheirDefaults(t *testing.T) {
	restoreStyles(t)
	// Start from a state other than the default, as a previous theme would.
	lightTheme().Apply()

	NewDashboard()

	if tview.Styles != legacyStyles {
		t.Errorf("tview.Styles after the dark theme = %+v, want tview's defaults %+v", tview.Styles, legacyStyles)
	}
}

// fgOf returns the foreground color of a table cell.
func fgOf(cell *tview.TableCell) tcell.Color {
	fg, _, _ := cell.Style.Decompose()
	return fg
}

// The colors of the table cells by column, as the original code set them.
func TestDarkThemeTableColorsMatchTheOriginal(t *testing.T) {
	const (
		y, w, g, r, gr, lb = tcell.ColorYellow, tcell.ColorWhite, tcell.ColorGreen, tcell.ColorRed, tcell.ColorGray, tcell.ColorLightBlue
	)
	checkRow := func(t *testing.T, table *tview.Table, row int, want []tcell.Color) {
		t.Helper()
		for col, wantColor := range want {
			cell := table.GetCell(row, col)
			if got := fgOf(cell); got != wantColor {
				t.Errorf("row %d column %d (%q) color = %v, want %v", row, col, cell.Text, got, wantColor)
			}
		}
		if table.GetCell(row, len(want)).Text != "" {
			t.Errorf("row %d has more columns than the %d expected", row, len(want))
		}
	}

	t.Run("docker services", func(t *testing.T) {
		d := newTestDashboard(t, Options{})
		d.Update(sampleContainers())
		checkRow(t, d.servicesTable, 0, []tcell.Color{y, y, y, y, y, y, y, y, y, y, y, y})
		checkRow(t, d.servicesTable, 1, []tcell.Color{w, g, lb, w, w, w, w, w, w, w, g, gr}) // running, no OOM
		checkRow(t, d.servicesTable, 2, []tcell.Color{w, r, lb, w, w, w, w, w, w, w, r, gr}) // exited, OOM events
	})

	t.Run("aws resources", func(t *testing.T) {
		d := newTestDashboard(t, Options{})
		d.awsViewMode = true
		containers := sampleContainers()
		containers.AWS.Region = "us-east-1"
		containers.AWS.Resources = []aws.AWSResource{
			{Name: "fn", Type: "Lambda", Region: "us-east-1", Status: "Active", LastUpdated: time.Unix(0, 0)},
			{Name: "stack", Type: "CloudFormation", Region: "us-east-1", Status: "UPDATE_IN_PROGRESS", LastUpdated: time.Unix(0, 0)},
		}

		d.Update(containers)

		checkRow(t, d.servicesTable, 0, []tcell.Color{y, y, y, y, y, y})
		checkRow(t, d.servicesTable, 1, []tcell.Color{w, lb, gr, g, w, gr})
		checkRow(t, d.servicesTable, 2, []tcell.Color{w, lb, gr, y, w, gr})
	})

	t.Run("alerts modal", func(t *testing.T) {
		d := newTestDashboard(t, Options{})
		d.Update(sampleContainers())
		if got := fgOf(d.alertsModal.GetCell(1, 0)); got != g {
			t.Errorf("empty alerts message color = %v, want green", got)
		}

		containers := sampleContainers()
		containers.Alerts = []alert.Alert{
			{Status: alert.StatusFiring, Severity: alert.SeverityCritical, ServiceName: "web", Message: "m", FiredAt: time.Unix(0, 0)},
			{Status: alert.StatusAcknowledged, Severity: alert.SeverityWarning, ServiceName: "web", Message: "m", FiredAt: time.Unix(0, 0)},
			{Status: alert.StatusResolved, Severity: alert.SeverityWarning, ServiceName: "web", Message: "m", FiredAt: time.Unix(0, 0)},
		}
		d.Update(containers)

		checkRow(t, d.alertsModal, 0, []tcell.Color{y, y, y, y, y, y})
		checkRow(t, d.alertsModal, 1, []tcell.Color{r, r, w, w, w, gr})
		checkRow(t, d.alertsModal, 2, []tcell.Color{y, y, w, w, w, gr})
		checkRow(t, d.alertsModal, 3, []tcell.Color{g, y, w, w, w, gr})
	})
}

// A dashboard created with the dark theme and the layout the example
// configuration spells out is the same dashboard as NewDashboard(): every
// character and every color of every page matches.
func TestDefaultDashboardEqualsExplicitDarkDashboard(t *testing.T) {
	explicitTheme := darkTheme()
	explicit := config.LayoutConfig{
		Services:  config.ServicesLayout{Height: 1},
		Resources: config.PanelLayout{Visible: boolPtr(true), Height: 1},
		Logs:      config.PanelLayout{Visible: boolPtr(true), Height: 1},
		HelpBar:   config.HelpBarLayout{Visible: boolPtr(true)},
	}
	containers := sampleContainers()
	containers.Alerts = []alert.Alert{{Status: alert.StatusFiring, Severity: alert.SeverityCritical, ServiceName: "web", Message: "high cpu", FiredAt: time.Unix(0, 0)}}

	build := func(t *testing.T, opts Options) *Dashboard {
		d := newTestDashboard(t, opts)
		d.Update(containers)
		return d
	}
	plain := build(t, Options{})
	spelledOut := build(t, Options{Theme: &explicitTheme, Layout: explicit})
	clock := regexpClock()

	for _, size := range [][2]int{{160, 40}, {80, 24}, {100, 31}} {
		for _, page := range []string{"dashboard", "alerts_modal", "error"} {
			plain.pages.SwitchToPage(page)
			spelledOut.pages.SwitchToPage(page)

			a := drawDashboard(t, plain, size[0], size[1])
			b := drawDashboard(t, spelledOut, size[0], size[1])

			if got, want := clock.ReplaceAllString(a.text(), "Updated: HH:MM:SS"), clock.ReplaceAllString(b.text(), "Updated: HH:MM:SS"); got != want {
				t.Errorf("%dx%d %s: text differs:\n%s\n---\n%s", size[0], size[1], page, got, want)
			}
			for i := range a.cells {
				if a.cells[i].Style != b.cells[i].Style {
					t.Errorf("%dx%d %s: style of cell %d differs: %v vs %v", size[0], size[1], page, i, a.cells[i].Style, b.cells[i].Style)
					break
				}
			}
		}
	}
}

// regexpClock matches the clock the resources view prints.
func regexpClock() *regexp.Regexp { return regexp.MustCompile(`Updated: \d\d:\d\d:\d\d`) }
