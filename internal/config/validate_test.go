package config

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/b92c/gowatch/pkg/metrics"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr []string // substrings; empty means the config must be valid
	}{
		{"defaults", func(*Config) {}, nil},

		{"refresh at the minimum", func(c *Config) { c.RefreshInterval = 500 * time.Millisecond }, nil},
		{"refresh below the minimum", func(c *Config) { c.RefreshInterval = 499 * time.Millisecond }, []string{"refresh_interval"}},
		{"refresh zero", func(c *Config) { c.RefreshInterval = 0 }, []string{"refresh_interval"}},

		{"tail at the minimum", func(c *Config) { c.Logs.TailLines = 1 }, nil},
		{"tail at the maximum", func(c *Config) { c.Logs.TailLines = 5000 }, nil},
		{"tail zero", func(c *Config) { c.Logs.TailLines = 0 }, []string{"logs.tail_lines"}},
		{"tail above the maximum", func(c *Config) { c.Logs.TailLines = 5001 }, []string{"logs.tail_lines"}},

		{"history at the minimum", func(c *Config) { c.History.MaxPoints = 10 }, nil},
		{"history at the maximum", func(c *Config) { c.History.MaxPoints = 3600 }, nil},
		{"history below the minimum", func(c *Config) { c.History.MaxPoints = 9 }, []string{"history.max_points"}},
		{"history above the maximum", func(c *Config) { c.History.MaxPoints = 3601 }, []string{"history.max_points"}},

		{"log level empty", func(c *Config) { c.Filters.MinLogLevelName = "" }, nil},
		{"log level info", func(c *Config) { c.Filters.MinLogLevelName = "info" }, nil},
		{"log level is case-insensitive", func(c *Config) { c.Filters.MinLogLevelName = "WARN" }, nil},
		{"log level error", func(c *Config) { c.Filters.MinLogLevelName = "error" }, nil},
		{"log level unknown name", func(c *Config) { c.Filters.MinLogLevelName = "verbose" }, []string{"filters.min_log_level", "verbose"}},

		{"aws enabled auto", func(c *Config) { c.AWS.Enabled = "auto" }, nil},
		{"aws enabled empty", func(c *Config) { c.AWS.Enabled = "" }, nil},
		{"aws enabled true", func(c *Config) { c.AWS.Enabled = "true" }, nil},
		{"aws enabled false", func(c *Config) { c.AWS.Enabled = "false" }, nil},
		{"aws enabled garbage", func(c *Config) { c.AWS.Enabled = "maybe" }, []string{"aws.enabled", "maybe"}},

		{"known rule id", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"high_cpu": {}}
		}, nil},
		{"unknown rule id", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"hihg_cpu": {}}
		}, []string{"alerts.rules.hihg_cpu", "unknown rule ID", "high_cpu"}},

		{"hysteresis equal to threshold", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"high_cpu": {Threshold: floatPtr(70), Hysteresis: floatPtr(70)}}
		}, nil},
		{"hysteresis above explicit threshold", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"high_cpu": {Threshold: floatPtr(70), Hysteresis: floatPtr(80)}}
		}, []string{"alerts.rules.high_cpu.hysteresis", "must not exceed threshold"}},
		{"hysteresis above the built-in threshold", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"high_cpu": {Hysteresis: floatPtr(86)}}
		}, []string{"alerts.rules.high_cpu.hysteresis"}},
		{"threshold below the built-in hysteresis is fine", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"high_cpu": {Threshold: floatPtr(1)}}
		}, nil},

		{"negative threshold", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"oom_killed": {Threshold: floatPtr(-1)}}
		}, []string{"alerts.rules.oom_killed.threshold", "finite number >= 0"}},
		{"negative hysteresis", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"high_cpu": {Hysteresis: floatPtr(-5)}}
		}, []string{"alerts.rules.high_cpu.hysteresis", "finite number >= 0"}},
		{"NaN threshold", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"high_cpu": {Threshold: floatPtr(math.NaN())}}
		}, []string{"alerts.rules.high_cpu.threshold", "finite number >= 0"}},
		{"infinite threshold", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"high_cpu": {Threshold: floatPtr(math.Inf(1))}}
		}, []string{"alerts.rules.high_cpu.threshold", "finite number >= 0"}},
		{"zero threshold is fine", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"oom_killed": {Threshold: floatPtr(0), Hysteresis: floatPtr(0)}}
		}, nil},

		{"consecutive hits of one", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"high_cpu": {ConsecutiveHits: intPtr(1)}}
		}, nil},
		{"consecutive hits of zero", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"high_cpu": {ConsecutiveHits: intPtr(0)}}
		}, []string{"alerts.rules.high_cpu.consecutive_hits"}},

		{"severity warn", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"high_cpu": {Severity: "warn"}}
		}, nil},
		{"severity crit", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"high_cpu": {Severity: "crit"}}
		}, nil},
		{"severity unknown", func(c *Config) {
			c.Alerts.RuleOverrides = RuleOverrides{"high_cpu": {Severity: "info"}}
		}, []string{"alerts.rules.high_cpu.severity", "info"}},

		{"every problem is reported at once", func(c *Config) {
			c.RefreshInterval = time.Millisecond
			c.Logs.TailLines = 0
			c.History.MaxPoints = 1
		}, []string{"refresh_interval", "logs.tail_lines", "history.max_points"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want an error mentioning %v", tt.wantErr)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestFiltersMinLogLevel(t *testing.T) {
	tests := []struct {
		in   string
		want metrics.LogLevel
	}{
		{"", metrics.LogLevelUnknown},
		{"info", metrics.LogLevelInfo},
		{"INFO", metrics.LogLevelInfo},
		{" warn ", metrics.LogLevelWarn},
		{"error", metrics.LogLevelError},
		{"verbose", metrics.LogLevelUnknown}, // invalid values are rejected by Validate, never filtered on
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := (FiltersConfig{MinLogLevelName: tt.in}).MinLogLevel(); got != tt.want {
				t.Errorf("MinLogLevel(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestAWSEnabledOverride(t *testing.T) {
	// A nil want means "auto".
	tests := []struct {
		want *bool
		name string
		yaml string
		fail bool
	}{
		{name: "omitted is auto", yaml: "refresh_interval: 2s\n"},
		{name: "auto unquoted", yaml: "aws:\n  enabled: auto\n"},
		{name: "auto quoted", yaml: "aws:\n  enabled: \"auto\"\n"},
		{name: "true unquoted", yaml: "aws:\n  enabled: true\n", want: boolPtr(true)},
		{name: "true quoted", yaml: "aws:\n  enabled: \"true\"\n", want: boolPtr(true)},
		{name: "false unquoted", yaml: "aws:\n  enabled: false\n", want: boolPtr(false)},
		{name: "false quoted", yaml: "aws:\n  enabled: \"false\"\n", want: boolPtr(false)},
		{name: "mixed case", yaml: "aws:\n  enabled: TRUE\n", want: boolPtr(true)},
		{name: "garbage", yaml: "aws:\n  enabled: maybe\n", fail: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadYAML(t, tt.yaml)
			if tt.fail {
				if err == nil || !strings.Contains(err.Error(), "aws.enabled") {
					t.Fatalf("Load() error = %v, want an aws.enabled error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			got := cfg.AWS.EnabledOverride()
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("EnabledOverride() = %v, want nil (auto)", *got)
			case tt.want != nil && (got == nil || *got != *tt.want):
				t.Errorf("EnabledOverride() = %v, want %v", got, *tt.want)
			}
		})
	}
}
