package ui

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// These tests change the working directory, so they do not run in parallel.

func TestLiveExportKeyWritesAFileAndShowsAMessage(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	l := startLive(t, Options{}, 150, 40)
	l.do(func() { l.d.messageDelay = time.Hour }) // the timer never fires here; the restore is called by hand
	l.update(logsOf(3))

	l.pressRune('e')

	files := exportFilesIn(t, dir)
	if len(files) != 1 {
		t.Fatalf("want one export file in %s, got %v", dir, files)
	}
	doc, _ := readExport(t, files[0])
	if doc["count"] != float64(3) {
		t.Errorf("count = %v, want 3", doc["count"])
	}
	name := files[0][strings.LastIndex(files[0], string(os.PathSeparator))+1:]
	if want := "Logs exported to " + name + " (3 lines)"; !strings.Contains(l.shotOnLoop().text(), want) {
		t.Fatalf("the message %q is not on the screen:\n%s", want, l.shotOnLoop().text())
	}

	// The normal help bar comes back.
	l.do(func() { l.d.restoreBars(l.d.messageSeq) })
	l.do(func() {
		if got := l.d.helpBar.GetText(false); !strings.Contains(got, "Quit") || strings.Contains(got, "Logs exported") {
			t.Errorf("the help bar was not restored: %q", got)
		}
	})

	// E exports too, and the name does not collide within the same second.
	l.pressRune('E')
	if got := exportFilesIn(t, dir); len(got) != 2 {
		t.Errorf("want two files, got %v", got)
	}
}

func TestLiveExportFailureShowsTheError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	if err := os.Chmod(dir, 0o500); err != nil { // #nosec G302 -- a directory that is searchable but not writable
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) // #nosec G302 -- lets the test directory be removed

	l := startLive(t, Options{}, 150, 40)
	l.update(logsOf(1))
	l.pressRune('e')
	if !strings.Contains(l.shot().text(), "Log export failed: ") {
		t.Errorf("the failure is not shown:\n%s", l.shot().text())
	}
	if got := exportFilesIn(t, dir); len(got) != 0 {
		t.Errorf("files = %v", got)
	}
}

func TestLiveNewerMessageSurvivesTheOlderTimer(t *testing.T) {
	l := startLive(t, Options{}, 150, 40)
	var oldSeq uint64
	l.do(func() {
		l.d.showMessage("first message")
		oldSeq = l.d.messageSeq
		l.d.messageDelay = time.Hour // no real timer fires during the test
		l.d.showMessage("second message")
		// What the first timer does when it fires late: nothing.
		l.d.restoreBars(oldSeq)
		if got := l.d.helpBar.GetText(false); got != "second message" {
			t.Errorf("an older timer wiped the newer message: %q", got)
		}
		l.d.restoreBars(l.d.messageSeq)
		if got := l.d.helpBar.GetText(false); got == "second message" {
			t.Errorf("the current message was not restored")
		}
	})
}

func TestLiveMessageTimerRestoresTheBars(t *testing.T) {
	l := startLive(t, Options{}, 150, 40)
	l.do(func() {
		l.d.messageDelay = 50 * time.Millisecond
		l.d.showMessage("hello")
	})
	if !l.shotHas("↑↓ Scroll | ? Help | q Quit", liveTimeout) {
		t.Errorf("the help bar was not restored:\n%s", l.shotOnLoop().text())
	}
}

func TestLiveExportMessageShowsOnTheDaemonErrorPage(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	l := startLive(t, Options{}, 150, 40)
	l.do(func() { l.d.messageDelay = time.Hour })
	l.update(logsOf(2))
	l.d.ShowDaemonError(errors.New("daemon is down"))
	l.do(func() {})
	l.pressRune('e')
	if got := exportFilesIn(t, dir); len(got) != 1 {
		t.Fatalf("files = %v", got)
	}
	if text := l.shotOnLoop().text(); !strings.Contains(text, "Logs exported to") {
		t.Fatalf("the message is not on the error page:\n%s", text)
	}

	// The error page bar is restored, not the dashboard one.
	l.do(func() {
		l.d.restoreBars(l.d.messageSeq)
		got := l.d.errorHelpBar.GetText(false)
		if strings.Contains(got, "Logs exported") || !strings.Contains(got, "Waiting for Docker daemon...") {
			t.Errorf("the error bar was not restored: %q", got)
		}
		if got := l.d.helpBar.GetText(false); strings.Contains(got, "Logs exported") {
			t.Errorf("stale message on the help bar: %q", got)
		}
	})
}

func TestStopStopsTheMessageTimer(t *testing.T) {
	restoreStyles(t)
	d := NewDashboard()
	queued := make(chan struct{}, 4)
	d.queueUpdate = func(func()) { queued <- struct{}{} }
	d.messageDelay = time.Hour
	d.showMessage("x")
	seq := d.messageSeq

	if !d.messageTimerFired(seq) {
		t.Fatal("before Stop the timer should queue the restore")
	}
	<-queued

	d.Stop()
	if d.messageTimerFired(seq) {
		t.Error("a timer that fires after Stop must not queue anything")
	}
	select {
	case <-queued:
		t.Error("an update was queued after Stop")
	default:
	}
	if !d.stopped.Load() {
		t.Error("Stop did not mark the dashboard as stopped")
	}
}

func TestQuitActionGoesThroughStop(t *testing.T) {
	restoreStyles(t)
	d := NewDashboard()
	d.runAction(ActionQuit)
	if !d.stopped.Load() {
		t.Error("the quit action must go through Stop")
	}
}

func TestLiveExportKeyIsTypedInTheSearchField(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	l := startLive(t, Options{}, 150, 40)
	l.update(logsOf(2))
	l.pressRune('/')
	l.typeText("eEe")

	if got := l.d.searchField.GetText(); got != "eEe" {
		t.Errorf("search text = %q, want eEe", got)
	}
	if got := exportFilesIn(t, dir); len(got) != 0 {
		t.Errorf("exported while typing: %v", got)
	}
}

func TestLiveExportIgnoresAWSViewAndHiddenLogs(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	l := startLive(t, Options{Layout: oracleLayouts["logs hidden"]}, 150, 40)
	l.update(logsOf(4))
	l.pressRune('a') // AWS view
	l.update(logsOf(4))
	l.pressRune('e')

	files := exportFilesIn(t, dir)
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	if doc, _ := readExport(t, files[0]); doc["count"] != float64(4) {
		t.Errorf("count = %v, want 4", doc["count"])
	}
}
