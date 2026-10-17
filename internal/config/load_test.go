package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

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
