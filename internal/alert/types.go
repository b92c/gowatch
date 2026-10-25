package alert

import (
	"time"
)

type Severity string

const (
	SeverityWarning  Severity = "WARN"
	SeverityCritical Severity = "CRIT"
)

type AlertStatus string

const (
	StatusFiring       AlertStatus = "FIRING"
	StatusResolved     AlertStatus = "RESOLVED"
	StatusAcknowledged AlertStatus = "ACKNOWLEDGED"
)

type AlertRule struct {
	ID                  string
	Metric              string
	Severity            Severity
	Description         string
	Threshold           float64
	HysteresisThreshold float64
	ConsecutiveHits     int
}

type Alert struct {
	FiredAt     time.Time
	ResolvedAt  *time.Time
	ID          string
	ContainerID string
	ServiceName string
	RuleID      string
	Message     string
	Severity    Severity
	Status      AlertStatus
	Value       float64
}

func DefaultRules() []AlertRule {
	return []AlertRule{
		{
			ID:                  "HIGH_CPU",
			Metric:              "cpu_percent",
			Threshold:           85.0,
			HysteresisThreshold: 75.0,
			ConsecutiveHits:     3,
			Severity:            SeverityWarning,
			Description:         "CPU usage exceeds 85%",
		},
		{
			ID:                  "HIGH_MEM",
			Metric:              "mem_percent",
			Threshold:           90.0,
			HysteresisThreshold: 80.0,
			ConsecutiveHits:     3,
			Severity:            SeverityCritical,
			Description:         "Memory usage exceeds 90%",
		},
		{
			ID:                  "OOM_KILLED",
			Metric:              "oom_events",
			Threshold:           0.0,
			HysteresisThreshold: 0.0,
			ConsecutiveHits:     1,
			Severity:            SeverityCritical,
			Description:         "Container experienced an Out Of Memory (OOM) event",
		},
		{
			ID:                  "CONTAINER_DOWN",
			Metric:              "container_state",
			Threshold:           0.0,
			HysteresisThreshold: 0.0,
			ConsecutiveHits:     1,
			Severity:            SeverityCritical,
			Description:         "Container is in unhealthy/restarting/dead state",
		},
	}
}
