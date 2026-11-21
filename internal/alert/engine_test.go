package alert

import (
	"testing"
	"time"

	"github.com/b92c/gowatch/pkg/metrics"
)

func TestAlertEngineConsecutiveHits(t *testing.T) {
	engine := NewAlertEngine(nil)

	host := metrics.HostInfo{
		CPUCount: 1,
		MemTotal: 1024 * 1024 * 1024,
	}

	c := ContainerEvaluatorData{
		ID:         "container-1",
		Service:    "web",
		State:      "running",
		CPUPercent: 90.0,
		MemUsage:   100 * 1024 * 1024,
	}

	alerts := engine.Evaluate([]ContainerEvaluatorData{c}, host)
	if len(alerts) != 0 {
		t.Fatalf("expected 0 active alerts on hit 1, got %d", len(alerts))
	}

	alerts = engine.Evaluate([]ContainerEvaluatorData{c}, host)
	if len(alerts) != 0 {
		t.Fatalf("expected 0 active alerts on hit 2, got %d", len(alerts))
	}

	alerts = engine.Evaluate([]ContainerEvaluatorData{c}, host)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 active alert on hit 3, got %d", len(alerts))
	}
	if alerts[0].Status != StatusFiring {
		t.Fatalf("expected status FIRING, got %s", alerts[0].Status)
	}
	if alerts[0].RuleID != "HIGH_CPU" {
		t.Fatalf("expected rule HIGH_CPU, got %s", alerts[0].RuleID)
	}
}

func TestAlertEngineHysteresis(t *testing.T) {
	engine := NewAlertEngine(nil)
	host := metrics.HostInfo{CPUCount: 1, MemTotal: 1000}

	c := ContainerEvaluatorData{
		ID:         "c1",
		Service:    "db",
		State:      "running",
		CPUPercent: 90.0,
	}

	for i := 0; i < 3; i++ {
		engine.Evaluate([]ContainerEvaluatorData{c}, host)
	}

	alerts := engine.GetActiveAlertsSnapshot()
	if len(alerts) != 1 || alerts[0].Status != StatusFiring {
		t.Fatalf("alert should be FIRING")
	}

	c.CPUPercent = 80.0
	engine.Evaluate([]ContainerEvaluatorData{c}, host)

	alerts = engine.GetActiveAlertsSnapshot()
	if len(alerts) != 1 || alerts[0].Status != StatusFiring {
		t.Fatalf("alert should STILL be FIRING due to hysteresis (val=80%%, hyst=75%%)")
	}

	c.CPUPercent = 70.0
	engine.Evaluate([]ContainerEvaluatorData{c}, host)

	alerts = engine.GetActiveAlertsSnapshot()
	if len(alerts) != 1 || alerts[0].Status != StatusResolved {
		t.Fatalf("alert should be RESOLVED after dropping below hysteresis threshold")
	}
}

func TestAlertEngineSafeMemoryLimit(t *testing.T) {
	engine := NewAlertEngine(nil)

	host := metrics.HostInfo{CPUCount: 1, MemTotal: 0}
	c := ContainerEvaluatorData{
		ID:       "c-zero-mem",
		MemUsage: 5000,
	}

	alerts := engine.Evaluate([]ContainerEvaluatorData{c}, host)
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts for zero mem total host fallback, got %d", len(alerts))
	}
}

func TestAlertEngineStaleContainerCleanup(t *testing.T) {
	engine := NewAlertEngine(nil)
	host := metrics.HostInfo{CPUCount: 1, MemTotal: 1000}

	c := ContainerEvaluatorData{
		ID:        "c-ephemeral",
		OOMEvents: 1,
	}

	engine.Evaluate([]ContainerEvaluatorData{c}, host)

	alerts := engine.GetActiveAlertsSnapshot()
	if len(alerts) != 1 || alerts[0].Status != StatusFiring {
		t.Fatalf("expected OOM alert firing")
	}

	engine.Evaluate([]ContainerEvaluatorData{}, host)

	alerts = engine.GetActiveAlertsSnapshot()
	if len(alerts) != 1 || alerts[0].Status != StatusResolved {
		t.Fatalf("expected stale container alert to be auto-resolved")
	}
}

func TestAlertEngineConcurrency(t *testing.T) {
	engine := NewAlertEngine(nil)
	host := metrics.HostInfo{CPUCount: 2, MemTotal: 8000}

	done := make(chan bool)
	go func() {
		for i := 0; i < 50; i++ {
			engine.Evaluate([]ContainerEvaluatorData{
				{ID: "c1", CPUPercent: 95.0},
			}, host)
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			_ = engine.GetActiveAlertsSnapshot()
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	<-done
	<-done
}

func TestAlertEngineDiskIOAndLogPattern(t *testing.T) {
	rules := []AlertRule{
		{
			ID:                  "HIGH_DISK_IO",
			Metric:              "disk_io_mb",
			Threshold:           100.0,
			HysteresisThreshold: 50.0,
			ConsecutiveHits:     1,
			Severity:            SeverityWarning,
			Description:         "Disk I/O exceeds 100MB",
		},
		{
			ID:                  "LOG_PATTERN",
			Metric:              "log_pattern",
			Threshold:           1.0,
			HysteresisThreshold: 0.0,
			ConsecutiveHits:     1,
			Severity:            SeverityCritical,
			Pattern:             "panic|fatal error",
			Description:         "Log pattern matched",
		},
	}

	engine := NewAlertEngine(rules)
	var notified []Alert
	engine.SetNotifyFunc(func(a Alert) {
		notified = append(notified, a)
	})

	host := metrics.HostInfo{CPUCount: 2, MemTotal: 1 << 30}
	const mb = 1024 * 1024

	alerts := engine.Evaluate([]ContainerEvaluatorData{{
		ID:             "c1",
		Service:        "api",
		State:          "running",
		DiskReadBytes:  60 * mb,
		DiskWriteBytes: 50 * mb,
		Logs:           []string{"INFO ok", "FATAL ERROR: db down"},
	}}, host)

	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts))
	}
	if len(notified) != 2 {
		t.Fatalf("expected 2 notifications on firing, got %d", len(notified))
	}

	stats := engine.GetStats()
	if stats.TotalFired != 2 || stats.Firing != 2 || stats.Critical != 1 || stats.Warning != 1 {
		t.Errorf("unexpected stats: %+v", stats)
	}

	ack, ok := engine.AcknowledgeFirstFiring()
	if !ok || ack.Status != StatusAcknowledged {
		t.Fatalf("AcknowledgeFirstFiring() = (%+v, %v)", ack, ok)
	}
	if !engine.ResolveAlert(ack.ID) {
		t.Errorf("ResolveAlert(%q) returned false", ack.ID)
	}

	stats = engine.GetStats()
	if stats.Resolved != 1 || stats.Firing != 1 {
		t.Errorf("unexpected stats after resolve: %+v", stats)
	}
	if len(notified) != 3 || notified[2].Status != StatusResolved {
		t.Errorf("expected resolution notification, got %+v", notified)
	}
}
