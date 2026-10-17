package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// The helpers in this file draw primitives on a simulated terminal, so the
// tests can inspect what the user would see: the characters and their colors.
// Nothing here starts the tview event loop.
//
// tview.Styles is global state that themes change. Tests that build a
// dashboard with newTestDashboard get it restored afterwards, and none of them
// run in parallel.

// screenshot is a drawn frame.
type screenshot struct {
	cells  []tcell.SimCell
	width  int
	height int
}

// drawPrimitive draws p over a width x height terminal.
func drawPrimitive(t *testing.T, p tview.Primitive, width, height int) screenshot {
	t.Helper()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("simulation screen: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(width, height)

	p.SetRect(0, 0, width, height)
	p.Draw(screen)
	screen.Show()

	cells, w, h := screen.GetContents()
	return screenshot{cells: cells, width: w, height: h}
}

// drawDashboard draws the dashboard page of d.
func drawDashboard(t *testing.T, d *Dashboard, width, height int) screenshot {
	t.Helper()
	return drawPrimitive(t, d.pages, width, height)
}

func (s screenshot) cell(x, y int) tcell.SimCell { return s.cells[y*s.width+x] }

// row returns row y with one rune per terminal cell.
func (s screenshot) row(y int) []rune {
	runes := make([]rune, s.width)
	for x := range runes {
		runes[x] = ' '
		if c := s.cell(x, y); len(c.Runes) > 0 {
			runes[x] = c.Runes[0]
		}
	}
	return runes
}

// text is every row of the frame, one per line.
func (s screenshot) text() string {
	lines := make([]string, s.height)
	for y := range lines {
		lines[y] = string(s.row(y))
	}
	return strings.Join(lines, "\n")
}

// find returns the position of the first cell of the first occurrence of text.
func (s screenshot) find(text string) (x, y int, ok bool) {
	needle := []rune(text)
	for y := 0; y < s.height; y++ {
		row := s.row(y)
		for x := 0; x+len(needle) <= len(row); x++ {
			if string(row[x:x+len(needle)]) == text {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// titleRow returns the row of the top border that carries title, or -1. A
// title sits between border characters (single lines, or double lines on the
// panel that has the focus), which tells it apart from the same words used
// elsewhere, such as a table column called "Logs".
func (s screenshot) titleRow(title string) int {
	for y := 0; y < s.height; y++ {
		row := string(s.row(y))
		for _, before := range []string{"┌", "╔", "─", "═"} {
			for _, after := range []string{"─", "═", "┐", "╗"} {
				if strings.Contains(row, before+title+after) {
					return y
				}
			}
		}
	}
	return -1
}

// colorsOf returns the foreground and background of the first character of the
// first occurrence of text, failing the test when text is not on screen.
func (s screenshot) colorsOf(t *testing.T, text string) (fg, bg tcell.Color) {
	t.Helper()
	x, y, ok := s.find(text)
	if !ok {
		t.Fatalf("%q is not on screen:\n%s", text, s.text())
	}
	fg, bg, _ = s.cell(x, y).Style.Decompose()
	return fg, bg
}

// colorsOnRowOf is colorsOf for the occurrence of text on the row that contains
// anchor.
func (s screenshot) colorsOnRowOf(t *testing.T, anchor, text string) (fg, bg tcell.Color) {
	t.Helper()
	_, y, ok := s.find(anchor)
	if !ok {
		t.Fatalf("%q is not on screen:\n%s", anchor, s.text())
	}
	row, n := s.row(y), len([]rune(text))
	for x := 0; x+n <= len(row); x++ {
		if string(row[x:x+n]) == text {
			fg, bg, _ = s.cell(x, y).Style.Decompose()
			return fg, bg
		}
	}
	t.Fatalf("%q is not on the row of %q:\n%s", text, anchor, string(row))
	return fg, bg
}

// restoreStyles puts tview.Styles back after the test.
func restoreStyles(t *testing.T) {
	t.Helper()
	saved := tview.Styles
	t.Cleanup(func() { tview.Styles = saved })
}

// newTestDashboard builds a dashboard without the "changed" callback of the
// logs view, which would otherwise ask an application that is not running to
// redraw.
func newTestDashboard(t *testing.T, opts Options) *Dashboard {
	t.Helper()
	restoreStyles(t)
	d := NewDashboardWithOptions(opts)
	d.logsView.SetChangedFunc(nil)
	return d
}

// sampleContainers is a small, fixed set of containers that exercises every
// colored part of the dashboard.
func sampleContainers() docker.Containers {
	return docker.Containers{
		Host: metrics.HostInfo{CPUCount: 8, MemTotal: 16 << 30, MemFree: 4 << 30},
		C: []docker.Container{
			{ID: "aaaaaaaaaaaaaaaaaaaa", Service: "web", Image: "nginx:latest", State: "running", CPUPercent: 12.5, MemUsage: 100 << 20},
			{ID: "bbbbbbbbbbbbbbbbbbbb", Service: "db", Image: "postgres:16", State: "exited", OOMEvents: 2},
		},
		FlatLogs: []docker.FormattedLog{
			{Service: "web", Line: "listening", Level: metrics.LogLevelInfo},
			{Service: "db", Line: "disk full", Level: metrics.LogLevelFatal},
			{Service: "web", Line: "slow query", Level: metrics.LogLevelWarn},
		},
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
