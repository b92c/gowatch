package ui

import (
	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func NewServiceListTable() *tview.Table {
	table := tview.NewTable().
		SetBorders(true).
		SetFixed(1, 0)
	table.SetBorder(true).SetTitle(" Services ").SetTitleAlign(tview.AlignLeft)
	return table
}

func NewResourceStatsView() *tview.TextView {
	view := tview.NewTextView().
		SetDynamicColors(true)
	view.SetBorder(true).SetTitle(" Resources ").SetTitleAlign(tview.AlignLeft)
	return view
}

func NewLogsView() *tview.TextView {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	view.SetBorder(true).SetTitle(" Logs ").SetTitleAlign(tview.AlignLeft)
	return view
}

func NewStatusBar() *tview.TextView {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
	view.SetBackgroundColor(tcell.ColorDarkBlue)
	return view
}

// RenderSparkline creates a small text-based chart using unicode block characters
func RenderSparkline(values []metrics.MetricPoint, maxPoints int) string {
	blocks := []rune{' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	if len(values) == 0 {
		runes := make([]rune, maxPoints)
		for i := 0; i < maxPoints; i++ {
			runes[i] = ' '
		}
		return string(runes)
	}

	if len(values) > maxPoints {
		values = values[len(values)-maxPoints:]
	}

	maxVal := 0.00001
	for _, v := range values {
		if v.Value > maxVal {
			maxVal = v.Value
		}
	}

	runes := make([]rune, 0, maxPoints)

	paddingCount := maxPoints - len(values)
	for i := 0; i < paddingCount; i++ {
		runes = append(runes, ' ')
	}

	for _, v := range values {
		idx := int((v.Value / maxVal) * float64(len(blocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		runes = append(runes, blocks[idx])
	}

	return string(runes)
}

func FormatStatsSummary(values []metrics.MetricPoint, unit string) string {
	return metrics.FormatStatsSummary(values, unit)
}
