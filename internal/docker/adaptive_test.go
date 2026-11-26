package docker

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/b92c/gowatch/internal/alert"
)

func TestAdaptivePollerDisabled(t *testing.T) {
	base := 2 * time.Second
	poller := NewAdaptivePoller(base, 10*time.Second, false)

	containers := Containers{
		C: []Container{{ID: "c1", State: "running"}},
	}
	for range 10 {
		got := poller.NextInterval(5*time.Second, containers)
		if got != base {
			t.Fatalf("NextInterval() = %s, want %s when disabled", got, base)
		}
	}
}

func TestAdaptivePollerSanitizesIntervals(t *testing.T) {
	poller := NewAdaptivePoller(0, 500*time.Millisecond, true)
	if poller.baseInterval != defaultAdaptiveBaseInterval {
		t.Errorf("baseInterval = %s, want %s", poller.baseInterval, defaultAdaptiveBaseInterval)
	}
	if poller.maxInterval != defaultAdaptiveBaseInterval {
		t.Errorf("maxInterval = %s, want %s", poller.maxInterval, defaultAdaptiveBaseInterval)
	}
}

func TestAdaptivePollerScaleUpUnderHighLoadAndCap(t *testing.T) {
	base := 2 * time.Second
	maxInt := 5 * time.Second
	poller := NewAdaptivePoller(base, maxInt, true)

	containers := Containers{
		C: []Container{{ID: "c1", State: "running"}},
	}

	// First tick records initial container state and snaps to base.
	if got := poller.NextInterval(100*time.Millisecond, containers); got != base {
		t.Fatalf("first tick = %s, want %s", got, base)
	}

	// elapsed > currentInterval/2 triggers 50% scale-up: 2s -> 3s.
	if got := poller.NextInterval(1500*time.Millisecond, containers); got != 3*time.Second {
		t.Fatalf("second tick = %s, want 3s", got)
	}

	// Next high-load tick: 3s -> 4.5s.
	if got := poller.NextInterval(2*time.Second, containers); got != 4500*time.Millisecond {
		t.Fatalf("third tick = %s, want 4.5s", got)
	}

	// Next high-load tick is capped at maxInterval (5s).
	if got := poller.NextInterval(3*time.Second, containers); got != maxInt {
		t.Fatalf("fourth tick = %s, want %s", got, maxInt)
	}
}

func TestAdaptivePollerScaleUpIdleManyContainers(t *testing.T) {
	base := 2 * time.Second
	maxInt := 10 * time.Second
	poller := NewAdaptivePoller(base, maxInt, true)

	many := make([]Container, 50)
	for i := range many {
		many[i] = Container{ID: fmt.Sprintf("c-%d", i), State: "running"}
	}
	containers := Containers{C: many}

	// Tick 0: state change from empty -> 50 containers.
	if got := poller.NextInterval(10*time.Millisecond, containers); got != base {
		t.Fatalf("initial tick = %s, want %s", got, base)
	}

	// Idle ticks 1 and 2 stay at base.
	for i := 1; i <= 2; i++ {
		if got := poller.NextInterval(10*time.Millisecond, containers); got != base {
			t.Fatalf("idle tick %d = %s, want %s", i, got, base)
		}
	}

	// Idle tick 3 with >= 50 containers scales up by 50%: 2s -> 3s.
	if got := poller.NextInterval(10*time.Millisecond, containers); got != 3*time.Second {
		t.Fatalf("idle tick 3 = %s, want 3s", got)
	}
}

func TestAdaptivePollerSnapBackOnStateChangeOrAlert(t *testing.T) {
	base := 2 * time.Second
	poller := NewAdaptivePoller(base, 10*time.Second, true)

	containers := Containers{
		C: []Container{{ID: "c1", State: "running"}},
	}

	// Prime initial state, then scale up via high elapsed.
	poller.NextInterval(10*time.Millisecond, containers)
	if got := poller.NextInterval(1500*time.Millisecond, containers); got != 3*time.Second {
		t.Fatalf("scaled tick = %s, want 3s", got)
	}

	// Container state change snaps back to baseInterval.
	changed := Containers{
		C: []Container{{ID: "c1", State: "exited"}},
	}
	if got := poller.NextInterval(1500*time.Millisecond, changed); got != base {
		t.Fatalf("state change tick = %s, want %s", got, base)
	}

	// Scale up again.
	if got := poller.NextInterval(1500*time.Millisecond, changed); got != 3*time.Second {
		t.Fatalf("re-scaled tick = %s, want 3s", got)
	}

	// New firing alert snaps back to baseInterval.
	withAlert := Containers{
		C:      []Container{{ID: "c1", State: "exited"}},
		Alerts: []alert.Alert{{RuleID: "HIGH_CPU", Status: alert.StatusFiring}},
	}
	if got := poller.NextInterval(1500*time.Millisecond, withAlert); got != base {
		t.Fatalf("alert tick = %s, want %s", got, base)
	}
}

func TestNewPooledHTTPClient(t *testing.T) {
	tests := []struct {
		name           string
		maxConcurrency int
		wantPoolSize   int
	}{
		{name: "small concurrency floors at 32", maxConcurrency: 4, wantPoolSize: 32},
		{name: "default concurrency 16 gives 32", maxConcurrency: 16, wantPoolSize: 32},
		{name: "large concurrency scales 2x", maxConcurrency: 24, wantPoolSize: 48},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewPooledHTTPClient(tt.maxConcurrency)
			tr, ok := client.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("Transport type = %T, want *http.Transport", client.Transport)
			}
			if tr.MaxIdleConns != tt.wantPoolSize {
				t.Errorf("MaxIdleConns = %d, want %d", tr.MaxIdleConns, tt.wantPoolSize)
			}
			if tr.MaxIdleConnsPerHost != tt.wantPoolSize {
				t.Errorf("MaxIdleConnsPerHost = %d, want %d", tr.MaxIdleConnsPerHost, tt.wantPoolSize)
			}
			if tr.MaxConnsPerHost != tt.wantPoolSize {
				t.Errorf("MaxConnsPerHost = %d, want %d", tr.MaxConnsPerHost, tt.wantPoolSize)
			}
			if tr.IdleConnTimeout != 90*time.Second {
				t.Errorf("IdleConnTimeout = %s, want 90s", tr.IdleConnTimeout)
			}
		})
	}
}
