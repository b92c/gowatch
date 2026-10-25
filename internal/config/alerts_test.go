package config

import (
	"reflect"
	"testing"

	"github.com/b92c/gowatch/internal/alert"
)

func TestAlertsRules(t *testing.T) {
	byID := func(rules []alert.AlertRule, id string) alert.AlertRule {
		t.Helper()
		for _, r := range rules {
			if r.ID == id {
				return r
			}
		}
		t.Fatalf("rule %s not found in %+v", id, rules)
		return alert.AlertRule{}
	}

	t.Run("no overrides returns the built-in rules", func(t *testing.T) {
		got := AlertsConfig{Enabled: true}.Rules()
		if !reflect.DeepEqual(got, alert.DefaultRules()) {
			t.Errorf("Rules() = %+v, want alert.DefaultRules()", got)
		}
	})

	t.Run("threshold only keeps the rest and follows the description", func(t *testing.T) {
		cfg, err := loadYAML(t, "alerts:\n  rules:\n    high_cpu:\n      threshold: 1\n")
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		rules := cfg.Alerts.Rules()
		got := byID(rules, "HIGH_CPU")
		want := alert.AlertRule{
			ID: "HIGH_CPU", Metric: "cpu_percent", Severity: alert.SeverityWarning,
			Description: "CPU usage exceeds 1%", Threshold: 1,
			HysteresisThreshold: 1, // lowered from 75 so it never exceeds the threshold
			ConsecutiveHits:     3,
		}
		if got != want {
			t.Errorf("HIGH_CPU = %+v, want %+v", got, want)
		}
		if len(rules) != 4 {
			t.Errorf("len(rules) = %d, want 4 (overrides never add or drop rules)", len(rules))
		}
		for _, id := range []string{"HIGH_MEM", "OOM_KILLED", "CONTAINER_DOWN"} {
			if byID(rules, id) != byID(alert.DefaultRules(), id) {
				t.Errorf("%s changed although it was not overridden", id)
			}
		}
	})

	t.Run("all fields", func(t *testing.T) {
		cfg, err := loadYAML(t, `
alerts:
  rules:
    high_mem:
      threshold: 70
      hysteresis: 60
      consecutive_hits: 5
      severity: warn
`)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		got := byID(cfg.Alerts.Rules(), "HIGH_MEM")
		want := alert.AlertRule{
			ID: "HIGH_MEM", Metric: "mem_percent", Severity: alert.SeverityWarning,
			Description: "Memory usage exceeds 70%", Threshold: 70,
			HysteresisThreshold: 60, ConsecutiveHits: 5,
		}
		if got != want {
			t.Errorf("HIGH_MEM = %+v, want %+v", got, want)
		}
	})

	t.Run("severity crit", func(t *testing.T) {
		cfg, err := loadYAML(t, "alerts:\n  rules:\n    high_cpu:\n      severity: crit\n")
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got := byID(cfg.Alerts.Rules(), "HIGH_CPU").Severity; got != alert.SeverityCritical {
			t.Errorf("severity = %s, want %s", got, alert.SeverityCritical)
		}
	})

	t.Run("rule IDs are matched case-insensitively", func(t *testing.T) {
		cfg, err := loadYAML(t, "alerts:\n  rules:\n    HIGH_CPU:\n      consecutive_hits: 9\n")
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got := byID(cfg.Alerts.Rules(), "HIGH_CPU").ConsecutiveHits; got != 9 {
			t.Errorf("ConsecutiveHits = %d, want 9", got)
		}
	})

	t.Run("a disabled rule is dropped and the others keep their order", func(t *testing.T) {
		cfg, err := loadYAML(t, "alerts:\n  rules:\n    high_mem:\n      enabled: false\n    high_cpu:\n      enabled: true\n")
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		var ids []string
		for _, r := range cfg.Alerts.Rules() {
			ids = append(ids, r.ID)
		}
		if want := []string{"HIGH_CPU", "OOM_KILLED", "CONTAINER_DOWN"}; !reflect.DeepEqual(ids, want) {
			t.Errorf("rule IDs = %v, want %v", ids, want)
		}
	})

	t.Run("alerts disabled yields an empty non-nil list", func(t *testing.T) {
		cfg, err := loadYAML(t, "alerts:\n  enabled: false\n")
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		rules := cfg.Alerts.Rules()
		if rules == nil || len(rules) != 0 {
			t.Errorf("Rules() = %#v, want an empty non-nil slice (nil would mean \"keep the defaults\")", rules)
		}
	})

	t.Run("Rules does not mutate the built-in rules", func(t *testing.T) {
		AlertsConfig{Enabled: true, RuleOverrides: RuleOverrides{"high_cpu": {Threshold: floatPtr(1)}}}.Rules()
		if got := byID(alert.DefaultRules(), "HIGH_CPU").Threshold; got != 85 {
			t.Errorf("built-in threshold = %v, want 85", got)
		}
	})
}
