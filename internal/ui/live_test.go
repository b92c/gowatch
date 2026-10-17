package ui

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/gdamore/tcell/v2"
)

// liveTimeout bounds every wait of the tests that run the real tview
// application.
const liveTimeout = 5 * time.Second

// liveDashboard is a dashboard running in the real tview.Application, on a
// simulated terminal: keys are injected on the screen, go through the
// application's input loop (the capture, then the primitive with focus) and the
// result is read back from the screen cells. This is as close as a test gets to
// a person at the keyboard.
type liveDashboard struct {
	t       *testing.T
	d       *Dashboard
	screen  tcell.SimulationScreen
	exited  chan struct{}
	runErr  error
	handled atomic.Int64
}

// startLive runs a dashboard with opts on a width x height terminal. The
// dashboard is stopped when the test ends.
func startLive(t *testing.T, opts Options, width, height int) *liveDashboard {
	t.Helper()
	restoreStyles(t)

	d := NewDashboardWithOptions(opts)
	// The logs view asks the application to redraw whenever its text changes; the
	// tests update from inside the event loop, where that request would wait for
	// the very loop that is making it.
	d.logsView.SetChangedFunc(nil)

	screen := tcell.NewSimulationScreen("")
	d.app.SetScreen(screen)
	screen.SetSize(width, height)

	l := &liveDashboard{t: t, d: d, screen: screen, exited: make(chan struct{})}
	go func() {
		l.runErr = d.Run()
		close(l.exited)
	}()
	t.Cleanup(l.stop)

	// A queued update runs once the loop is up. Count the keys the loop has
	// finished with by wrapping the capture Run installed.
	l.do(func() {
		inner := d.app.GetInputCapture()
		d.app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
			out := inner(ev)
			l.handled.Add(1)
			return out
		})
	})
	return l
}

// do runs f on the application's goroutine and waits for it, so that f can touch
// the dashboard without racing the event loop. It must not be used once the
// dashboard has quit.
func (l *liveDashboard) do(f func()) {
	l.t.Helper()
	done := make(chan struct{})
	go func() {
		l.d.app.QueueUpdate(f)
		close(done)
	}()
	select {
	case <-done:
	case <-l.exited:
		l.t.Fatal("the dashboard quit")
	case <-time.After(liveTimeout):
		l.t.Fatal("timed out waiting for the application loop")
	}
}

// press injects a key and waits until the application has handled it and drawn
// the result.
func (l *liveDashboard) press(key tcell.Key, r rune, mod tcell.ModMask) {
	l.t.Helper()
	want := l.handled.Load() + 1
	l.screen.InjectKey(key, r, mod)
	deadline := time.Now().Add(liveTimeout)
	for l.handled.Load() < want {
		if time.Now().After(deadline) {
			l.t.Fatalf("the application did not handle the key %v %q", key, r)
		}
		select {
		case <-l.exited:
			l.t.Fatalf("the dashboard quit on the key %v %q", key, r)
		case <-time.After(time.Millisecond):
		}
	}
	l.do(func() {}) // the loop is past the key, redraw included
}

func (l *liveDashboard) pressRune(r rune) { l.t.Helper(); l.press(tcell.KeyRune, r, tcell.ModNone) }

func (l *liveDashboard) pressKey(k tcell.Key) {
	l.t.Helper()
	mod := tcell.ModNone
	if k >= tcell.KeyCtrlA && k <= tcell.KeyCtrlZ {
		mod = tcell.ModCtrl
	}
	l.press(k, 0, mod)
}

// typeText types each rune of s.
func (l *liveDashboard) typeText(s string) {
	l.t.Helper()
	for _, r := range s {
		l.pressRune(r)
	}
}

// pressAndQuit injects a key that should end the application and waits for it.
func (l *liveDashboard) pressAndQuit(key tcell.Key, r rune, mod tcell.ModMask) {
	l.t.Helper()
	l.screen.InjectKey(key, r, mod)
	if !l.waitForExit() {
		l.t.Errorf("the dashboard did not quit on the key %v %q", key, r)
	}
}

func (l *liveDashboard) waitForExit() bool {
	select {
	case <-l.exited:
		return true
	case <-time.After(liveTimeout):
		return false
	}
}

// running reports whether the application loop is still alive.
func (l *liveDashboard) running() bool {
	select {
	case <-l.exited:
		return false
	default:
		return true
	}
}

func (l *liveDashboard) stop() {
	l.d.app.Stop()
	if !l.waitForExit() {
		l.t.Error("the dashboard did not stop")
	}
}

// shot copies what is on the screen right now.
func (l *liveDashboard) shot() screenshot {
	cells, w, h := l.screen.GetContents()
	return screenshot{cells: append([]tcell.SimCell(nil), cells...), width: w, height: h}
}

// update gives the dashboard new containers and redraws, as a tick of the
// collector does.
func (l *liveDashboard) update(c docker.Containers) {
	l.t.Helper()
	done := make(chan struct{})
	go func() {
		l.d.app.QueueUpdateDraw(func() { l.d.Update(c) })
		close(done)
	}()
	select {
	case <-done:
	case <-l.exited:
		l.t.Fatal("the dashboard quit")
	case <-time.After(liveTimeout):
		l.t.Fatal("timed out waiting for the application loop")
	}
}

// logsOf is the sample containers with n log lines, "line 001" to "line n".
func logsOf(n int) docker.Containers {
	c := sampleContainers()
	c.FlatLogs = nil
	for i := 1; i <= n; i++ {
		c.FlatLogs = append(c.FlatLogs, docker.FormattedLog{Service: "web", Line: fmt.Sprintf("line %03d", i), Level: metrics.LogLevelInfo})
	}
	return c
}
