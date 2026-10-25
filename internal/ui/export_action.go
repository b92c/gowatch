package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/internal/filter"
	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/rivo/tview"
)

// defaultExportMessageDelay is how long the result of an export replaces the
// help bar.
const defaultExportMessageDelay = 5 * time.Second

// shownSnapshot is what an update put on screen: the log lines that passed the
// filters, and the filters that let them through, so that an export describes
// exactly the lines it writes even if a filter changed since.
type shownSnapshot struct {
	search   string
	logs     []docker.FormattedLog
	minLevel metrics.LogLevel
}

// rememberShown keeps a copy of the log lines that passed the filters in the
// last update, with the search text and minimum level used: they are what
// export_logs writes. Update runs on the collector's goroutine and the export
// on the application's, hence the lock.
func (d *Dashboard) rememberShown(fs filter.FilterState, logs []docker.FormattedLog) {
	snap := shownSnapshot{logs: slices.Clone(logs), search: fs.SearchText, minLevel: fs.MinLogLevel}
	d.shownMu.Lock()
	d.shown = snap
	d.shownMu.Unlock()
}

func (d *Dashboard) shownCopy() shownSnapshot {
	d.shownMu.Lock()
	defer d.shownMu.Unlock()
	snap := d.shown
	snap.logs = slices.Clone(snap.logs)
	return snap
}

// exportLogs writes the log lines that pass the filters to a JSON file in the
// current working directory and tells the user how it went. It runs on the
// application's goroutine, from the input handler.
func (d *Dashboard) exportLogs() {
	snap := d.shownCopy()
	logs := snap.logs

	dir, err := os.Getwd()
	var path string
	if err == nil {
		path, err = writeLogsExport(dir, time.Now(), snap.search, snap.minLevel, logs)
	}
	if err != nil {
		d.showMessage(d.theme.Tag(d.theme.StatusError) + tview.Escape("Log export failed: "+err.Error()) + "[-]")
		return
	}
	d.showMessage(d.theme.Tag(d.theme.StatusOK) + tview.Escape(fmt.Sprintf("Logs exported to %s (%d lines)", filepath.Base(path), len(logs))) + "[-]")
}

// showMessage replaces the text of the help bar, and of the bar of the "Docker
// daemon offline" page, with text (already formatted for tview) and puts the
// normal bars back after the message delay. A newer message cancels the timer of
// the previous one, and a timer that already fired for an older message does
// nothing. It runs on the application's goroutine.
func (d *Dashboard) showMessage(text string) {
	d.helpBar.SetText(text)
	d.errorHelpBar.SetText(text)

	d.messageMu.Lock()
	defer d.messageMu.Unlock()
	if d.messageTimer != nil {
		d.messageTimer.Stop()
	}
	d.messageSeq++
	seq := d.messageSeq

	delay := d.messageDelay
	if delay <= 0 {
		delay = defaultExportMessageDelay
	}
	d.messageTimer = time.AfterFunc(delay, func() { d.messageTimerFired(seq) })
}

// messageTimerFired is what the timer of message seq does: ask the application
// to put the normal bars back. It reports whether it did. QueueUpdateDraw waits
// for the event loop, which is gone after Stop, so the call is made from a
// goroutine of its own: the timer's goroutine never blocks, and after Stop
// nothing is queued at all.
func (d *Dashboard) messageTimerFired(seq uint64) bool {
	if d.stopped.Load() {
		return false
	}
	queue := d.queueUpdate
	if queue == nil {
		queue = func(f func()) { d.app.QueueUpdateDraw(f) }
	}
	go queue(func() { d.restoreBars(seq) })
	return true
}

// restoreBars puts the normal text back on both bars, unless a newer message
// than the one numbered seq has replaced it since.
func (d *Dashboard) restoreBars(seq uint64) {
	d.messageMu.Lock()
	current := d.messageSeq == seq
	d.messageMu.Unlock()
	if !current {
		return
	}
	d.helpBar.SetText(helpBarText(d.theme, d.keymap))
	d.errorHelpBar.SetText(errorHelpBarText(d.theme, d.keymap))
}
