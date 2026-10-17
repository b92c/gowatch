package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/b92c/gowatch/internal/alert"
	"github.com/b92c/gowatch/pkg/metrics"
	"go.yaml.in/yaml/v3"
)

func TestDefaultYAML(t *testing.T) {
	out := DefaultYAML()

	t.Run("documents only the supported sections", func(t *testing.T) {
		var doc map[string]any
		if err := yaml.Unmarshal(out, &doc); err != nil {
			t.Fatalf("DefaultYAML is not valid YAML: %v", err)
		}
		got := make([]string, 0, len(doc))
		for k := range doc {
			got = append(got, k)
		}
		want := []string{"refresh_interval", "logs", "history", "filters", "alerts", "tracing", "aws"}
		if len(got) != len(want) {
			t.Fatalf("sections = %v, want %v", got, want)
		}
		for _, k := range want {
			if _, ok := doc[k]; !ok {
				t.Errorf("section %q missing from DefaultYAML", k)
			}
		}
	})

	t.Run("is commented", func(t *testing.T) {
		if !strings.HasPrefix(string(out), "#") || strings.Count(string(out), "\n#") < 5 {
			t.Errorf("DefaultYAML should be a commented example:\n%s", out)
		}
	})

	t.Run("loads strictly and changes nothing", func(t *testing.T) {
		cfg, err := loadYAML(t, string(out))
		if err != nil {
			t.Fatalf("DefaultYAML does not load: %v\n%s", err, out)
		}

		if got, want := cfg.Alerts.Rules(), alert.DefaultRules(); !reflect.DeepEqual(got, want) {
			t.Errorf("rules from DefaultYAML = %+v, want the built-in rules %+v", got, want)
		}
		if n := len(cfg.Alerts.RuleOverrides); n != len(alert.DefaultRules()) {
			t.Errorf("DefaultYAML lists %d rules, want one per built-in rule (%d)", n, len(alert.DefaultRules()))
		}

		// Normalize the two spellings that legitimately differ from Default().
		cfg.Alerts.RuleOverrides = nil
		if len(cfg.Filters.Status) == 0 {
			cfg.Filters.Status = nil
		}
		if want := Default(); !reflect.DeepEqual(*cfg, want) {
			t.Errorf("config from DefaultYAML = %+v, want Default() %+v", *cfg, want)
		}
	})

	t.Run("is deterministic", func(t *testing.T) {
		if string(DefaultYAML()) != string(out) {
			t.Error("DefaultYAML output changed between calls")
		}
	})
}

// The generated example must stay usable: editing only a rule's threshold in
// the printed file has to load, and the rule has to fire at the new threshold.
func TestDefaultYAMLThresholdEditFiresTheRule(t *testing.T) {
	out := string(DefaultYAML())
	const before = "    high_cpu:\n      threshold: 85\n"
	if !strings.Contains(out, before) {
		t.Fatalf("DefaultYAML no longer has the high_cpu block this test edits:\n%s", out)
	}
	edited := strings.Replace(out, before, "    high_cpu:\n      threshold: 1\n", 1)

	cfg, err := loadYAML(t, edited)
	if err != nil {
		t.Fatalf("the printed example with only high_cpu.threshold changed does not load: %v", err)
	}

	engine := alert.NewAlertEngine(cfg.Alerts.Rules())
	host := metrics.HostInfo{CPUCount: 1, MemTotal: 1 << 30}
	container := []alert.ContainerEvaluatorData{{ID: "c1", Service: "web", State: "running", CPUPercent: 5}}
	var alerts []alert.Alert
	for range 3 { // consecutive_hits is 3
		alerts = engine.Evaluate(container, host)
	}
	if len(alerts) != 1 || alerts[0].RuleID != "HIGH_CPU" || alerts[0].Status != alert.StatusFiring {
		t.Errorf("alerts = %+v, want one firing HIGH_CPU alert at the 1%% threshold", alerts)
	}
}
