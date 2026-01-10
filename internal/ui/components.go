package ui

import (
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
