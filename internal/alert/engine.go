package alert

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/b92c/gowatch/pkg/metrics"
)

// ContainerEvaluatorData encapsulates container properties needed for evaluation
type ContainerEvaluatorData struct {
	ID             string
	Service        string
	State          string
	Logs           []string
	CPUPercent     float64
	MemUsage       uint64
	OOMEvents      uint64
	DiskReadBytes  uint64
	DiskWriteBytes uint64
}

type AlertEngine struct {
	activeAlerts map[string]*Alert
	hitCounters  map[string]int
	notifyFn     func(Alert)
	rules        []AlertRule
	totalFired   int
	mu           sync.RWMutex
}

func NewAlertEngine(rules []AlertRule) *AlertEngine {
	if len(rules) == 0 {
		rules = DefaultRules()
	}
	return &AlertEngine{
		rules:        rules,
		activeAlerts: make(map[string]*Alert),
		hitCounters:  make(map[string]int),
	}
}

func (ae *AlertEngine) SetRules(rules []AlertRule) {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	ae.rules = rules
}

// SetNotifyFunc registers a callback invoked on alert firing or resolution.
func (ae *AlertEngine) SetNotifyFunc(fn func(Alert)) {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	ae.notifyFn = fn
}

func (ae *AlertEngine) Evaluate(containers []ContainerEvaluatorData, host metrics.HostInfo) []Alert {
	ae.mu.Lock()

	activeContainerIDs := make(map[string]bool)
	for _, c := range containers {
		activeContainerIDs[c.ID] = true
	}

	var toNotify []Alert

	for key, a := range ae.activeAlerts {
		if !activeContainerIDs[a.ContainerID] {
			if a.Status == StatusFiring || a.Status == StatusAcknowledged {
				now := time.Now()
				a.Status = StatusResolved
				a.ResolvedAt = &now
				a.Message = "Container terminated (stale alert auto-resolved)"
				toNotify = append(toNotify, copyAlert(a))
			}
			delete(ae.hitCounters, key)
		}
	}

	now := time.Now()
	for _, c := range containers {
		serviceName := c.Service
		if serviceName == "" {
			if len(c.ID) >= 12 {
				serviceName = c.ID[:12]
			} else {
				serviceName = c.ID
			}
		}

		for _, rule := range ae.rules {
			key := c.ID + ":" + rule.ID
			val, hit := ae.evaluateRule(c, host, rule)

			if hit {
				ae.hitCounters[key]++
				hits := ae.hitCounters[key]

				if hits >= rule.ConsecutiveHits {
					existing, exists := ae.activeAlerts[key]
					if !exists || existing.Status == StatusResolved {
						newAlert := &Alert{
							ID:          key + ":" + fmt.Sprintf("%d", now.UnixNano()),
							ContainerID: c.ID,
							ServiceName: serviceName,
							RuleID:      rule.ID,
							Message:     fmt.Sprintf("%s: %.1f (threshold: %.1f)", rule.Description, val, rule.Threshold),
							Value:       val,
							Severity:    rule.Severity,
							Status:      StatusFiring,
							FiredAt:     now,
						}
						ae.activeAlerts[key] = newAlert
						ae.totalFired++
						toNotify = append(toNotify, copyAlert(newAlert))
					} else if existing.Status == StatusFiring {
						existing.Value = val
						existing.Message = fmt.Sprintf("%s: %.1f (threshold: %.1f)", rule.Description, val, rule.Threshold)
					}
				}
			} else {
				existing, exists := ae.activeAlerts[key]
				if exists && (existing.Status == StatusFiring || existing.Status == StatusAcknowledged) {
					isBelowHysteresis := ae.isBelowHysteresis(c, host, rule, val)
					if isBelowHysteresis {
						existing.Status = StatusResolved
						existing.ResolvedAt = &now
						ae.hitCounters[key] = 0
						toNotify = append(toNotify, copyAlert(existing))
					}
				} else {
					ae.hitCounters[key] = 0
				}
			}
		}
	}

	snapshot := ae.getSnapshotLocked()
	notifyFn := ae.notifyFn
	ae.mu.Unlock()

	if notifyFn != nil {
		for _, a := range toNotify {
			notifyFn(a)
		}
	}

	return snapshot
}

func (ae *AlertEngine) evaluateRule(c ContainerEvaluatorData, host metrics.HostInfo, rule AlertRule) (float64, bool) {
	switch rule.Metric {
	case "cpu_percent":
		cpuCount := float64(host.CPUCount)
		if cpuCount <= 0 {
			cpuCount = 1
		}
		normalizedCPU := c.CPUPercent / cpuCount
		return normalizedCPU, normalizedCPU >= rule.Threshold

	case "mem_percent":
		memLimit := host.MemTotal
		if memLimit == 0 {
			return 0, false
		}
		memPercent := (float64(c.MemUsage) / float64(memLimit)) * 100.0
		return memPercent, memPercent >= rule.Threshold

	case "oom_events":
		return float64(c.OOMEvents), c.OOMEvents > uint64(rule.Threshold)

	case "container_state":
		state := strings.ToLower(c.State)
		isDown := state == "restarting" || state == "dead" || state == "exited"
		val := 0.0
		if isDown {
			val = 1.0
		}
		return val, isDown

	case "disk_io_mb":
		diskMB := float64(c.DiskReadBytes+c.DiskWriteBytes) / (1024.0 * 1024.0)
		return diskMB, diskMB >= rule.Threshold

	case "log_pattern":
		if rule.Pattern == "" {
			return 0, false
		}
		matches := countLogPatternMatches(c.Logs, rule.Pattern)
		return float64(matches), float64(matches) >= rule.Threshold

	default:
		return 0, false
	}
}

func countLogPatternMatches(logs []string, pattern string) int {
	re, err := regexp.Compile("(?i)" + pattern)
	count := 0
	if err == nil {
		for _, line := range logs {
			if re.MatchString(line) {
				count++
			}
		}
		return count
	}
	lowerPat := strings.ToLower(pattern)
	for _, line := range logs {
		if strings.Contains(strings.ToLower(line), lowerPat) {
			count++
		}
	}
	return count
}

func (ae *AlertEngine) isBelowHysteresis(c ContainerEvaluatorData, host metrics.HostInfo, rule AlertRule, currentVal float64) bool {
	switch rule.Metric {
	case "cpu_percent", "mem_percent", "disk_io_mb":
		return currentVal < rule.HysteresisThreshold
	default:
		return currentVal <= rule.HysteresisThreshold
	}
}

func (ae *AlertEngine) AcknowledgeAlert(alertID string) {
	ae.mu.Lock()
	defer ae.mu.Unlock()

	for _, alert := range ae.activeAlerts {
		if alert.ID == alertID && alert.Status == StatusFiring {
			alert.Status = StatusAcknowledged
			break
		}
	}
}

// AcknowledgeFirstFiring marks the first firing alert as acknowledged.
func (ae *AlertEngine) AcknowledgeFirstFiring() (Alert, bool) {
	ae.mu.Lock()
	defer ae.mu.Unlock()

	for _, a := range ae.activeAlerts {
		if a.Status == StatusFiring {
			a.Status = StatusAcknowledged
			return copyAlert(a), true
		}
	}
	return Alert{}, false
}

// ResolveAlert manually resolves an active or acknowledged alert.
func (ae *AlertEngine) ResolveAlert(alertID string) bool {
	ae.mu.Lock()
	var resolved *Alert
	for key, a := range ae.activeAlerts {
		if a.ID == alertID && (a.Status == StatusFiring || a.Status == StatusAcknowledged) {
			now := time.Now()
			a.Status = StatusResolved
			a.ResolvedAt = &now
			ae.hitCounters[key] = 0
			cp := copyAlert(a)
			resolved = &cp
			break
		}
	}
	notifyFn := ae.notifyFn
	ae.mu.Unlock()

	if resolved != nil && notifyFn != nil {
		notifyFn(*resolved)
	}
	return resolved != nil
}

// GetStats returns summary counts for alerts in the current session.
func (ae *AlertEngine) GetStats() AlertStats {
	ae.mu.RLock()
	defer ae.mu.RUnlock()

	stats := AlertStats{TotalFired: ae.totalFired}
	for _, a := range ae.activeAlerts {
		switch a.Status {
		case StatusFiring:
			stats.Firing++
		case StatusAcknowledged:
			stats.Acknowledged++
		case StatusResolved:
			stats.Resolved++
		}
		switch a.Severity {
		case SeverityCritical:
			stats.Critical++
		case SeverityWarning:
			stats.Warning++
		}
	}
	return stats
}

func (ae *AlertEngine) ClearResolvedAlerts() {
	ae.mu.Lock()
	defer ae.mu.Unlock()

	for key, alert := range ae.activeAlerts {
		if alert.Status == StatusResolved {
			delete(ae.activeAlerts, key)
			delete(ae.hitCounters, key)
		}
	}
}

func (ae *AlertEngine) GetActiveAlertsSnapshot() []Alert {
	ae.mu.RLock()
	defer ae.mu.RUnlock()
	return ae.getSnapshotLocked()
}

func (ae *AlertEngine) getSnapshotLocked() []Alert {
	snapshot := make([]Alert, 0, len(ae.activeAlerts))
	for _, alert := range ae.activeAlerts {
		snapshot = append(snapshot, copyAlert(alert))
	}
	return snapshot
}

func copyAlert(alert *Alert) Alert {
	alertCopy := *alert
	if alert.ResolvedAt != nil {
		t := *alert.ResolvedAt
		alertCopy.ResolvedAt = &t
	}
	return alertCopy
}
