package config

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/b92c/gowatch/internal/alert"
	"github.com/b92c/gowatch/pkg/metrics"
	"go.yaml.in/yaml/v3"
)

func boolPtr(v bool) *bool             { return &v }
func floatPtr(v float64) *float64      { return &v }
func intPtr(v int) *int                { return &v }
func globalConfigPath(h string) string { return filepath.Join(h, ".gowatch", "config.yaml") }
func localConfigPath(w string) string  { return filepath.Join(w, "gowatch.yaml") }

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// loadYAML loads a single config file through the explicit-path layer.
func loadYAML(t *testing.T, content string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, content)
	cfg, _, err := Load(LoadOptions{ExplicitPath: path})
	return cfg, err
}

// withDefaults returns the built-in configuration after applying mutate.
func withDefaults(mutate func(*Config)) Config {
	cfg := Default()
	mutate(&cfg)
	return cfg
}

func TestDefaultMatchesLegacyValues(t *testing.T) {
	cfg := Default()

	if cfg.RefreshInterval != 2*time.Second {
		t.Errorf("RefreshInterval = %s, want 2s (the former hard-coded ticker)", cfg.RefreshInterval)
	}
	if cfg.Logs.TailLines != 50 {
		t.Errorf("TailLines = %d, want 50 (the former hard-coded tail)", cfg.Logs.TailLines)
	}
	if cfg.History.MaxPoints != 60 {
		t.Errorf("MaxPoints = %d, want 60 (the former metrics.MaxHistoryPoints)", cfg.History.MaxPoints)
	}
	if !cfg.Alerts.Enabled {
		t.Error("alerts must be enabled by default")
	}
	if got, want := cfg.Alerts.Rules(), alert.DefaultRules(); !reflect.DeepEqual(got, want) {
		t.Errorf("default rules = %+v, want alert.DefaultRules() %+v", got, want)
	}
	if cfg.Tracing.OTLPReceiver.Enabled {
		t.Error("the OTLP receiver must be off by default")
	}
	if cfg.Tracing.OTLPReceiver.Address != "127.0.0.1:4318" {
		t.Errorf("OTLP address = %q, want 127.0.0.1:4318", cfg.Tracing.OTLPReceiver.Address)
	}
	if cfg.AWS.EnabledOverride() != nil {
		t.Error("aws.enabled must default to auto (nil override), so environment detection stays in charge")
	}
	if cfg.AWS.Region != "" || cfg.AWS.EndpointURL != "" {
		t.Errorf("AWS region/endpoint must default to empty, got %q / %q", cfg.AWS.Region, cfg.AWS.EndpointURL)
	}
	if cfg.Filters.Search != "" || len(cfg.Filters.Status) != 0 || cfg.Filters.MinLogLevel() != metrics.LogLevelUnknown {
		t.Errorf("filters must default to none, got %+v", cfg.Filters)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Default() must be valid: %v", err)
	}
}

func TestLoadWithoutFilesReturnsDefault(t *testing.T) {
	tests := []struct {
		opts func(t *testing.T) LoadOptions
		name string
	}{
		{name: "empty home and work dirs", opts: func(t *testing.T) LoadOptions {
			return LoadOptions{HomeDir: t.TempDir(), WorkDir: t.TempDir()}
		}},
		{name: "no directories at all", opts: func(t *testing.T) LoadOptions { return LoadOptions{} }},
		{name: "home dir without .gowatch folder", opts: func(t *testing.T) LoadOptions {
			return LoadOptions{HomeDir: filepath.Join(t.TempDir(), "missing")}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, files, err := Load(tt.opts(t))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if want := Default(); !reflect.DeepEqual(*cfg, want) {
				t.Errorf("Load() = %+v, want Default() %+v", *cfg, want)
			}
			if len(files) != 0 {
				t.Errorf("files = %v, want none", files)
			}
		})
	}
}

func TestLoadLayering(t *testing.T) {
	tests := []struct {
		name   string
		global string
		local  string
		want   Config
	}{
		{
			name:   "global only",
			global: "refresh_interval: 5s\nlogs:\n  tail_lines: 10\n",
			want: withDefaults(func(c *Config) {
				c.RefreshInterval = 5 * time.Second
				c.Logs.TailLines = 10
			}),
		},
		{
			name:  "local only",
			local: "history:\n  max_points: 120\n",
			want:  withDefaults(func(c *Config) { c.History.MaxPoints = 120 }),
		},
		{
			name:   "local overrides global field by field",
			global: "refresh_interval: 5s\nlogs:\n  tail_lines: 10\nhistory:\n  max_points: 120\n",
			local:  "logs:\n  tail_lines: 20\n",
			want: withDefaults(func(c *Config) {
				c.RefreshInterval = 5 * time.Second
				c.Logs.TailLines = 20
				c.History.MaxPoints = 120
			}),
		},
		{
			name:   "local wins on the same field",
			global: "refresh_interval: 5s\n",
			local:  "refresh_interval: 3s\n",
			want:   withDefaults(func(c *Config) { c.RefreshInterval = 3 * time.Second }),
		},
		{
			name:   "a key left out keeps the earlier value even if the section appears",
			global: "alerts:\n  enabled: false\n",
			local:  "alerts:\n  rules:\n    high_cpu:\n      threshold: 95\n",
			want: withDefaults(func(c *Config) {
				c.Alerts.Enabled = false
				c.Alerts.RuleOverrides = RuleOverrides{"high_cpu": {Threshold: floatPtr(95)}}
			}),
		},
		{
			name:   "lists are replaced, not merged",
			global: "filters:\n  status: [running, exited]\n  search: web\n",
			local:  "filters:\n  status: [paused]\n",
			want: withDefaults(func(c *Config) {
				c.Filters.Status = []string{"paused"}
				c.Filters.Search = "web"
			}),
		},
		{
			name:   "maps are merged by key and their lists replaced",
			global: "theme:\n  name: nord\n  colors:\n    accent: \"#111111\"\n    border: \"#222222\"\nkeybindings:\n  bindings:\n    quit: [q, ctrl+c]\n    search: [\"/\"]\n",
			local:  "theme:\n  colors:\n    accent: \"#333333\"\nkeybindings:\n  bindings:\n    quit: [x]\n",
			want: withDefaults(func(c *Config) {
				c.Theme = ThemeConfig{Name: "nord", Colors: map[string]string{"accent": "#333333", "border": "#222222"}}
				c.Keybindings.Bindings = map[string][]string{"quit": {"x"}, "search": {"/"}}
			}),
		},
		{
			name:   "rule overrides from two files combine field by field",
			global: "alerts:\n  rules:\n    high_cpu:\n      threshold: 90\n      hysteresis: 70\n    high_mem:\n      severity: warn\n",
			local:  "alerts:\n  rules:\n    HIGH_CPU:\n      severity: crit\n      threshold: 95\n",
			want: withDefaults(func(c *Config) {
				c.Alerts.RuleOverrides = RuleOverrides{
					"high_cpu": {Threshold: floatPtr(95), Hysteresis: floatPtr(70), Severity: "crit"},
					"high_mem": {Severity: "warn"},
				}
			}),
		},
		{
			name:   "optional booleans keep an earlier false",
			global: "layout:\n  resources:\n    visible: false\n",
			local:  "layout:\n  resources:\n    height: 3\n  help_bar:\n    visible: true\n",
			want: withDefaults(func(c *Config) {
				c.Layout.Resources = PanelLayout{Visible: boolPtr(false), Height: 3}
				c.Layout.HelpBar = HelpBarLayout{Visible: boolPtr(true)}
			}),
		},
		{
			name:   "comment-only and blank files change nothing",
			global: "# nothing to see here\n",
			local:  "\n",
			want:   Default(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, work := t.TempDir(), t.TempDir()
			var wantFiles []string
			if tt.global != "" {
				writeFile(t, globalConfigPath(home), tt.global)
				wantFiles = append(wantFiles, globalConfigPath(home))
			}
			if tt.local != "" {
				writeFile(t, localConfigPath(work), tt.local)
				wantFiles = append(wantFiles, localConfigPath(work))
			}

			cfg, files, err := Load(LoadOptions{HomeDir: home, WorkDir: work})
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !reflect.DeepEqual(*cfg, tt.want) {
				t.Errorf("Load() =\n%+v\nwant\n%+v", *cfg, tt.want)
			}
			if !reflect.DeepEqual(files, wantFiles) {
				t.Errorf("files = %v, want %v (global first, local second)", files, wantFiles)
			}
		})
	}
}

func TestLoadExplicitPathIgnoresDefaultLocations(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	writeFile(t, globalConfigPath(home), "refresh_interval: 9s\nhistory:\n  max_points: 99\n")
	writeFile(t, localConfigPath(work), "logs:\n  tail_lines: 99\n")
	explicit := filepath.Join(t.TempDir(), "custom.yaml")
	writeFile(t, explicit, "logs:\n  tail_lines: 7\n")

	cfg, files, err := Load(LoadOptions{ExplicitPath: explicit, HomeDir: home, WorkDir: work})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := withDefaults(func(c *Config) { c.Logs.TailLines = 7 })
	if !reflect.DeepEqual(*cfg, want) {
		t.Errorf("Load() = %+v, want only defaults + the explicit file %+v", *cfg, want)
	}
	if !reflect.DeepEqual(files, []string{explicit}) {
		t.Errorf("files = %v, want [%s]", files, explicit)
	}
}

func TestLoadExplicitPathMustExist(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.yaml")
	_, _, err := Load(LoadOptions{ExplicitPath: missing})
	if err == nil {
		t.Fatal("Load() with a missing --config file must fail")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error %q does not mention %s", err, missing)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "unknown top-level key reports the line",
			content: "refresh_interval: 2s\nlogz:\n  tail_lines: 5\n",
			want:    []string{"line 2", "logz"},
		},
		{
			name:    "unknown nested key reports the line",
			content: "logs:\n  tail_line: 5\n",
			want:    []string{"line 2", "tail_line"},
		},
		{
			name:    "typo inside a rule override is still rejected",
			content: "alerts:\n  rules:\n    high_cpu:\n      treshold: 1\n",
			want:    []string{"line 4", "treshold"},
		},
		{
			name:    "wrong value type reports the line",
			content: "history:\n  max_points: lots\n",
			want:    []string{"line 2", "lots"},
		},
		{
			name:    "invalid duration",
			content: "refresh_interval: soon\n",
			want:    []string{"line 1"},
		},
		{
			name:    "malformed yaml",
			content: "logs: [\n",
			want:    []string{"yaml"},
		},
		{
			name:    "a second YAML document is not silently ignored",
			content: "history:\n  max_points: 20\n---\nlogz: 1\n",
			want:    []string{"line 4", "only one YAML document"},
		},
		{
			name:    "the same rule written in two spellings",
			content: "alerts:\n  rules:\n    high_cpu:\n      threshold: 1\n    HIGH_CPU:\n      threshold: 2\n",
			want:    []string{"high_cpu", "defined twice", "HIGH_CPU"},
		},
		{
			name:    "validation failure names the key and the file",
			content: "refresh_interval: 100ms\n",
			want:    []string{"invalid configuration", "refresh_interval"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			path := globalConfigPath(home)
			writeFile(t, path, tt.content)

			cfg, files, err := Load(LoadOptions{HomeDir: home})
			if err == nil {
				t.Fatalf("Load() = %+v, want an error", *cfg)
			}
			if cfg != nil || files != nil {
				t.Errorf("on error Load must return nil config and files, got %v / %v", cfg, files)
			}
			for _, want := range append(tt.want, path) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

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

func TestAWSRegionAndEndpointAreLoaded(t *testing.T) {
	cfg, err := loadYAML(t, "aws:\n  region: eu-west-1\n  endpoint_url: http://localhost:4566\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AWS.Region != "eu-west-1" || cfg.AWS.EndpointURL != "http://localhost:4566" {
		t.Errorf("AWS = %+v", cfg.AWS)
	}
}

func TestTracingAndFiltersAreLoaded(t *testing.T) {
	cfg, err := loadYAML(t, `
tracing:
  otlp_receiver:
    enabled: true
    address: "0.0.0.0:4319"
filters:
  search: api
  min_log_level: warn
  status: [running]
`)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Tracing.OTLPReceiver.Enabled || cfg.Tracing.OTLPReceiver.Address != "0.0.0.0:4319" {
		t.Errorf("Tracing = %+v", cfg.Tracing)
	}
	if cfg.Filters.Search != "api" || cfg.Filters.MinLogLevel() != metrics.LogLevelWarn || !reflect.DeepEqual(cfg.Filters.Status, []string{"running"}) {
		t.Errorf("Filters = %+v", cfg.Filters)
	}
}

// The theme, layout and keybindings sections are interpreted by later
// features; for now they only have to be accepted and kept as raw data.
func TestFutureSectionsAreKeptAsRawData(t *testing.T) {
	cfg, err := loadYAML(t, `
theme:
  name: solarized
  colors:
    accent: "#ff8800"
layout:
  resources: { visible: false, height: 2 }
  logs:      { height: 3 }
  services:  { height: 4 }
  help_bar:  { visible: false }
keybindings:
  preset: vim
  bindings:
    quit: ["q", "ctrl+c"]
`)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Theme.Name != "solarized" || cfg.Theme.Colors["accent"] != "#ff8800" {
		t.Errorf("Theme = %+v", cfg.Theme)
	}
	wantLayout := LayoutConfig{
		Resources: PanelLayout{Visible: boolPtr(false), Height: 2},
		Logs:      PanelLayout{Height: 3},
		Services:  ServicesLayout{Height: 4},
		HelpBar:   HelpBarLayout{Visible: boolPtr(false)},
	}
	if !reflect.DeepEqual(cfg.Layout, wantLayout) {
		t.Errorf("Layout = %+v, want %+v", cfg.Layout, wantLayout)
	}
	if cfg.Layout.Logs.Visible != nil {
		t.Error("an absent visible must stay nil to differ from an explicit false")
	}
	if cfg.Keybindings.Preset != "vim" || !reflect.DeepEqual(cfg.Keybindings.Bindings["quit"], []string{"q", "ctrl+c"}) {
		t.Errorf("Keybindings = %+v", cfg.Keybindings)
	}
}

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

// A trailing document separator leaves no second document behind.
func TestLoadAcceptsTrailingDocumentSeparator(t *testing.T) {
	cfg, err := loadYAML(t, "history:\n  max_points: 20\n---\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.History.MaxPoints != 20 {
		t.Errorf("MaxPoints = %d, want 20", cfg.History.MaxPoints)
	}
}

// ./gowatch.yaml may come from a cloned, untrusted repository, so it cannot
// set aws.endpoint_url or a non-loopback OTLP address. The same values are
// fine in ~/.gowatch/config.yaml and in a --config file.
func TestLoadRestrictsWorkingDirectoryLayer(t *testing.T) {
	const (
		endpoint = "aws:\n  endpoint_url: http://169.254.169.254:4566\n"
		hint1    = "~/.gowatch/config.yaml"
		hint2    = "--config"
	)
	otlp := func(addr string) string {
		return "tracing:\n  otlp_receiver:\n    enabled: true\n    address: \"" + addr + "\"\n"
	}

	// layer is "home", "work" or "explicit". An empty wantAddress means the
	// built-in default address; an empty wantErr means the file is accepted.
	tests := []struct {
		name        string
		layer       string
		content     string
		wantAddress string
		wantErr     []string
	}{
		{name: "endpoint from the working directory is rejected", layer: "work", content: endpoint,
			wantErr: []string{"aws.endpoint_url", hint1, hint2}},
		{name: "endpoint from the home config is accepted", layer: "home", content: endpoint},
		{name: "endpoint from --config is accepted", layer: "explicit", content: endpoint},
		{name: "an empty endpoint in the working directory is harmless", layer: "work", content: "aws:\n  endpoint_url: \"\"\n"},

		{name: "wildcard address from the working directory is rejected", layer: "work", content: otlp("0.0.0.0:4318"),
			wantErr: []string{"tracing.otlp_receiver.address", "0.0.0.0:4318", hint1, hint2}},
		{name: "empty host from the working directory is rejected", layer: "work", content: otlp(":4318"),
			wantErr: []string{"tracing.otlp_receiver.address", hint1}},
		{name: "LAN address from the working directory is rejected", layer: "work", content: otlp("192.168.1.5:4318"),
			wantErr: []string{"tracing.otlp_receiver.address"}},
		{name: "hostname from the working directory is rejected", layer: "work", content: otlp("collector.example.com:4318"),
			wantErr: []string{"tracing.otlp_receiver.address"}},
		{name: "other 127.x addresses are not on the allow list", layer: "work", content: otlp("127.0.0.2:4318"),
			wantErr: []string{"tracing.otlp_receiver.address"}},
		{name: "an address without a port is rejected", layer: "work", content: otlp("127.0.0.1"),
			wantErr: []string{"tracing.otlp_receiver.address"}},
		{name: "IPv6 wildcard from the working directory is rejected", layer: "work", content: otlp("[::]:4318"),
			wantErr: []string{"tracing.otlp_receiver.address"}},

		{name: "127.0.0.1 from the working directory is accepted", layer: "work", content: otlp("127.0.0.1:4319"), wantAddress: "127.0.0.1:4319"},
		{name: "IPv6 loopback from the working directory is accepted", layer: "work", content: otlp("[::1]:4318"), wantAddress: "[::1]:4318"},
		{name: "localhost from the working directory is accepted", layer: "work", content: otlp("localhost:4318"), wantAddress: "localhost:4318"},
		{name: "localhost is case-insensitive", layer: "work", content: otlp("LocalHost:4318"), wantAddress: "LocalHost:4318"},
		{name: "non-loopback address from the home config is accepted", layer: "home", content: otlp("0.0.0.0:4318"), wantAddress: "0.0.0.0:4318"},
		{name: "non-loopback address from --config is accepted", layer: "explicit", content: otlp("0.0.0.0:4318"), wantAddress: "0.0.0.0:4318"},

		{name: "both violations are reported together", layer: "work", content: endpoint + otlp("0.0.0.0:4318"),
			wantErr: []string{"aws.endpoint_url", "tracing.otlp_receiver.address"}},
		{name: "other settings in the working directory still apply", layer: "work", content: "logs:\n  tail_lines: 9\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, work := t.TempDir(), t.TempDir()
			opts := LoadOptions{HomeDir: home, WorkDir: work}
			var path string
			switch tt.layer {
			case "home":
				path = globalConfigPath(home)
			case "work":
				path = localConfigPath(work)
			case "explicit":
				path = filepath.Join(t.TempDir(), "custom.yaml")
				opts.ExplicitPath = path
			}
			writeFile(t, path, tt.content)

			cfg, _, err := Load(opts)

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("Load() = %+v, want an error", *cfg)
				}
				for _, want := range append(tt.wantErr, path) {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			wantAddress := tt.wantAddress
			if wantAddress == "" {
				wantAddress = Default().Tracing.OTLPReceiver.Address
			}
			if got := cfg.Tracing.OTLPReceiver.Address; got != wantAddress {
				t.Errorf("OTLP address = %q, want %q", got, wantAddress)
			}
			if strings.Contains(tt.content, "endpoint_url: http") && cfg.AWS.EndpointURL == "" {
				t.Error("an accepted endpoint_url must end up in the config")
			}
		})
	}
}

// The restriction looks at what the working-directory file sets itself, not at
// the merged result: a trusted layer's endpoint survives, and a working
// directory file that leaves the key out is fine.
func TestWorkingDirectoryLayerKeepsTrustedValues(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	writeFile(t, globalConfigPath(home), "aws:\n  endpoint_url: http://localstack:4566\ntracing:\n  otlp_receiver:\n    address: \"0.0.0.0:4318\"\n")
	writeFile(t, localConfigPath(work), "logs:\n  tail_lines: 9\n")

	cfg, _, err := Load(LoadOptions{HomeDir: home, WorkDir: work})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AWS.EndpointURL != "http://localstack:4566" || cfg.Tracing.OTLPReceiver.Address != "0.0.0.0:4318" || cfg.Logs.TailLines != 9 {
		t.Errorf("merged config = %+v", *cfg)
	}
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
