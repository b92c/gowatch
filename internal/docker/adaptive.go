package docker

import (
	"net/http"
	"time"

	"github.com/b92c/gowatch/internal/alert"
)

const (
	defaultAdaptiveBaseInterval = 2 * time.Second
	minHTTPPoolSize             = 32
	httpIdleConnTimeout         = 90 * time.Second
	idleTickScaleThreshold      = 5
	largeIdleTickScaleThreshold = 3
	largeContainerThreshold     = 50
)

// AdaptivePoller adjusts the polling interval based on container state stability,
// active firing alerts, and collection duration.
type AdaptivePoller struct {
	lastStates      map[string]string
	baseInterval    time.Duration
	maxInterval     time.Duration
	currentInterval time.Duration
	idleTicks       int
	lastFiringCount int
	enabled         bool
}

// NewAdaptivePoller creates an AdaptivePoller with sanitized interval bounds.
func NewAdaptivePoller(baseInterval, maxInterval time.Duration, enabled bool) *AdaptivePoller {
	if baseInterval <= 0 {
		baseInterval = defaultAdaptiveBaseInterval
	}
	if maxInterval < baseInterval {
		maxInterval = baseInterval
	}
	return &AdaptivePoller{
		lastStates:      make(map[string]string),
		baseInterval:    baseInterval,
		maxInterval:     maxInterval,
		currentInterval: baseInterval,
		enabled:         enabled,
	}
}

// NextInterval returns the next polling interval given the last collection's
// elapsed duration and resulting container snapshot.
func (p *AdaptivePoller) NextInterval(elapsed time.Duration, containers Containers) time.Duration {
	if !p.enabled {
		return p.baseInterval
	}

	firingCount := 0
	for _, a := range containers.Alerts {
		if a.Status == alert.StatusFiring {
			firingCount++
		}
	}

	stateChanged := len(containers.C) != len(p.lastStates)
	newStates := make(map[string]string, len(containers.C))
	for _, c := range containers.C {
		newStates[c.ID] = c.State
		if !stateChanged {
			if prev, ok := p.lastStates[c.ID]; !ok || prev != c.State {
				stateChanged = true
			}
		}
	}

	alertsIncreased := firingCount > p.lastFiringCount
	p.lastStates = newStates
	p.lastFiringCount = firingCount

	if stateChanged || alertsIncreased {
		p.idleTicks = 0
		p.currentInterval = p.baseInterval
		return p.currentInterval
	}

	p.idleTicks++
	if elapsed > p.currentInterval/2 ||
		(len(containers.C) >= largeContainerThreshold && p.idleTicks >= largeIdleTickScaleThreshold) ||
		p.idleTicks >= idleTickScaleThreshold {
		p.currentInterval = min(p.currentInterval+p.currentInterval/2, p.maxInterval)
	}

	return p.currentInterval
}

// NewPooledHTTPClient returns an HTTP client configured with a connection pool
// sized for concurrent Docker API requests.
func NewPooledHTTPClient(maxConcurrency int) *http.Client {
	poolSize := max(maxConcurrency*2, minHTTPPoolSize)
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        poolSize,
			MaxIdleConnsPerHost: poolSize,
			MaxConnsPerHost:     poolSize,
			IdleConnTimeout:     httpIdleConnTimeout,
		},
	}
}
