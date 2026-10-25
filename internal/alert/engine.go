package alert

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/b92c/gowatch/pkg/metrics"
)

// ContainerEvaluatorData encapsulates container properties needed for evaluation
type ContainerEvaluatorData struct {
	ID         string
	Service    string
	State      string
	CPUPercent float64
	MemUsage   uint64
	OOMEvents  uint64
}

type AlertEngine struct {
	activeAlerts map[string]*Alert
	hitCounters  map[string]int
	rules        []AlertRule
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

func (ae *AlertEngine) Evaluate(containers []ContainerEvaluatorData, host metrics.HostInfo) []Alert {
	ae.mu.Lock()
	defer ae.mu.Unlock()

	activeContainerIDs := make(map[string]bool)
	for _, c := range containers {
		activeContainerIDs[c.ID] = true
	}

	for key, alert := range ae.activeAlerts {
		if !activeContainerIDs[alert.ContainerID] {
			if alert.Status == StatusFiring || alert.Status == StatusAcknowledged {
				now := time.Now()
				alert.Status = StatusResolved
				alert.ResolvedAt = &now
				alert.Message = "Container terminated (stale alert auto-resolved)"
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
						ae.activeAlerts[key] = &Alert{
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
					}
				} else {
					ae.hitCounters[key] = 0
				}
			}
		}
	}

	return ae.getSnapshotLocked()
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

	default:
		return 0, false
	}
}

func (ae *AlertEngine) isBelowHysteresis(c ContainerEvaluatorData, host metrics.HostInfo, rule AlertRule, currentVal float64) bool {
	switch rule.Metric {
	case "cpu_percent":
		return currentVal < rule.HysteresisThreshold
	case "mem_percent":
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
		alertCopy := *alert
		if alert.ResolvedAt != nil {
			t := *alert.ResolvedAt
			alertCopy.ResolvedAt = &t
		}
		snapshot = append(snapshot, alertCopy)
	}
	return snapshot
}
