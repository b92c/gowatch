package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/b92c/gowatch/internal/config"
	"github.com/b92c/gowatch/internal/ui"
	"github.com/gdamore/tcell/v2"
)

// The tests only exercise the paths that return before the Docker client and
// the dashboard are created; a valid configuration would start the TUI.

func writeConfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name    string
		wantErr string
		args    []string
		want    cliOptions
	}{
		{name: "no flags"},
		{name: "config path", args: []string{"--config", "/tmp/x.yaml"}, want: cliOptions{configPath: "/tmp/x.yaml"}},
		{name: "single dash works too", args: []string{"-config=/tmp/x.yaml"}, want: cliOptions{configPath: "/tmp/x.yaml"}},
		{name: "print default config", args: []string{"--print-default-config"}, want: cliOptions{printDefaultConfig: true}},
		{name: "theme", args: []string{"--theme", "light"}, want: cliOptions{theme: "light"}},
		{name: "theme with config", args: []string{"--config", "/tmp/x.yaml", "-theme=nord"}, want: cliOptions{configPath: "/tmp/x.yaml", theme: "nord"}},
		{name: "unknown flag", args: []string{"--no-such-flag"}, wantErr: "flag provided but not defined"},
		{name: "theme without a value", args: []string{"--theme"}, wantErr: "needs an argument"},
		{name: "stray argument", args: []string{"extra"}, wantErr: "unexpected arguments"},
		{name: "config without a value", args: []string{"--config"}, wantErr: "needs an argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			got, err := parseFlags(tt.args, &stderr)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("parseFlags(%v) = %+v, want an error", tt.args, got)
				}
				if combined := err.Error() + stderr.String(); !strings.Contains(combined, tt.wantErr) {
					t.Errorf("error output %q does not contain %q", combined, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFlags(%v) error = %v", tt.args, err)
			}
			if got != tt.want {
				t.Errorf("parseFlags(%v) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestRunPrintDefaultConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"--print-default-config"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !bytes.Equal(stdout.Bytes(), config.DefaultYAML()) {
		t.Errorf("stdout differs from config.DefaultYAML():\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"--help"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	for _, want := range []string{"-config", "-print-default-config", "-theme", "dark, light, nord, solarized"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("usage does not mention %s:\n%s", want, stderr.String())
		}
	}
}

func TestRunRejectsBadCommandLine(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"--no-such-flag"}, &stdout, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunConfigErrorsStopStartup(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "typo in a key",
			path: writeConfig(t, dir, "typo.yaml", "refresh_interval: 2s\nlogz:\n  tail_lines: 5\n"),
			want: []string{"typo.yaml", "line 2", "logz"},
		},
		{
			name: "invalid value",
			path: writeConfig(t, dir, "invalid.yaml", "refresh_interval: 100ms\nlogs:\n  tail_lines: 0\n"),
			want: []string{"invalid configuration", "refresh_interval", "logs.tail_lines"},
		},
		{
			name: "unknown alert rule",
			path: writeConfig(t, dir, "rule.yaml", "alerts:\n  rules:\n    hihg_cpu:\n      threshold: 1\n"),
			want: []string{"hihg_cpu", "high_cpu"},
		},
		{
			name: "missing explicit file",
			path: filepath.Join(dir, "missing.yaml"),
			want: []string{"missing.yaml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := run([]string{"--config", tt.path}, &stdout, &stderr)

			if code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if !strings.HasPrefix(stderr.String(), "gowatch: ") {
				t.Errorf("stderr should start with \"gowatch: \", got %q", stderr.String())
			}
			for _, want := range tt.want {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr %q does not contain %q", stderr.String(), want)
				}
			}
		})
	}
}

// Without --config the default locations are read, ./gowatch.yaml included.
func TestRunReadsDefaultLocations(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(work)
	writeConfig(t, work, "gowatch.yaml", "history:\n  max_points: 1\n")

	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (history.max_points: 1 is invalid); stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "history.max_points") || !strings.Contains(stderr.String(), "gowatch.yaml") {
		t.Errorf("stderr = %q, want it to name history.max_points and gowatch.yaml", stderr.String())
	}
}

// ./gowatch.yaml may come from an untrusted checkout, so startup must refuse
// the settings that could redirect AWS calls or start the trace receiver.
func TestRunRefusesRiskySettingsFromWorkingDirectory(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"aws endpoint", "aws:\n  endpoint_url: http://evil.example.com\n", "aws.endpoint_url"},
		{"public OTLP address", "tracing:\n  otlp_receiver:\n    enabled: true\n    address: \"0.0.0.0:4318\"\n", "tracing.otlp_receiver.address"},
		{"enabling the OTLP receiver", "tracing:\n  otlp_receiver:\n    enabled: true\n", "tracing.otlp_receiver.enabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, work := t.TempDir(), t.TempDir()
			t.Setenv("HOME", home)
			t.Chdir(work)
			writeConfig(t, work, "gowatch.yaml", tt.content)

			var stdout, stderr bytes.Buffer
			code := run(nil, &stdout, &stderr)

			if code != 1 {
				t.Fatalf("exit code = %d, want 1; stderr: %s", code, stderr.String())
			}
			for _, want := range []string{tt.want, "gowatch.yaml", "~/.gowatch/config.yaml", "--config"} {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr %q does not contain %q", stderr.String(), want)
				}
			}
		})
	}
}

func TestRunThemeErrorsStopStartup(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "unknown theme from the flag",
			args: []string{"--theme", "nope"},
			want: []string{`no built-in theme named "nope"`, "dark, light, nord, solarized"},
		},
		{
			name: "unknown theme in the file",
			args: []string{"--config", writeConfig(t, dir, "theme.yaml", "theme:\n  name: neon\n")},
			want: []string{`no built-in theme named "neon"`, "dark, light, nord, solarized"},
		},
		{
			name: "the flag replaces a valid theme in the file",
			args: []string{"--theme", "nope", "--config", writeConfig(t, dir, "valid.yaml", "theme:\n  name: light\n")},
			want: []string{`no built-in theme named "nope"`},
		},
		{
			name: "invalid color",
			args: []string{"--config", writeConfig(t, dir, "color.yaml", "theme:\n  colors:\n    accent: not-a-color\n")},
			want: []string{"theme.colors.accent", `"not-a-color"`},
		},
		{
			name: "unquoted hex color",
			args: []string{"--config", writeConfig(t, dir, "hex.yaml", "theme:\n  colors:\n    accent: #ff8800\n")},
			want: []string{"theme.colors.accent", "quote #rrggbb"},
		},
		{
			name: "unknown color role",
			args: []string{"--config", writeConfig(t, dir, "role.yaml", "theme:\n  colors:\n    acent: red\n")},
			want: []string{"theme.colors.acent", "unknown color role", "accent"},
		},
		{
			name: "negative layout height",
			args: []string{"--config", writeConfig(t, dir, "height.yaml", "layout:\n  resources:\n    height: -1\n")},
			want: []string{"layout.resources.height", "got -1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := run(tt.args, &stdout, &stderr)

			if code != 1 {
				t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if !strings.HasPrefix(stderr.String(), "gowatch: ") {
				t.Errorf("stderr should start with \"gowatch: \", got %q", stderr.String())
			}
			for _, want := range tt.want {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr %q does not contain %q", stderr.String(), want)
				}
			}
		})
	}
}

func TestThemeFor(t *testing.T) {
	light := ui.BuiltinThemes()["light"]

	t.Run("no flag keeps the configured theme", func(t *testing.T) {
		got, err := themeFor(config.ThemeConfig{Name: "light"}, "")
		if err != nil || got.Background != light.Background {
			t.Errorf("themeFor() = background %v, error %v, want the light theme", got.Background, err)
		}
	})
	t.Run("the flag replaces the configured name", func(t *testing.T) {
		got, err := themeFor(config.ThemeConfig{Name: "neon"}, "light")
		if err != nil || got.Background != light.Background {
			t.Errorf("themeFor() = background %v, error %v, want the light theme", got.Background, err)
		}
	})
	t.Run("color overrides from the file apply on top of the flag theme", func(t *testing.T) {
		got, err := themeFor(config.ThemeConfig{Name: "dark", Colors: map[string]string{"accent": "#123456"}}, "light")
		if err != nil {
			t.Fatalf("themeFor() error = %v", err)
		}
		if got.Background != light.Background || got.Text != light.Text {
			t.Errorf("themeFor() = %+v, want the light theme", got)
		}
		if want := tcell.NewHexColor(0x123456); got.Accent != want {
			t.Errorf("accent = %v, want the file's %v", got.Accent, want)
		}
	})
	t.Run("an unknown flag value is an error", func(t *testing.T) {
		if _, err := themeFor(config.ThemeConfig{}, "nope"); err == nil {
			t.Error("themeFor() with an unknown theme returned no error")
		}
	})
}

// The printed example has to document every theme and color role, load
// strictly, and describe the built-in look: nothing changes when it is used.
func TestPrintedDefaultConfigCoversThemeAndLayout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--print-default-config"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
	printed := stdout.String()

	for _, want := range append(ui.ThemeNames(), ui.ColorRoleNames()...) {
		if !strings.Contains(printed, want) {
			t.Errorf("--print-default-config does not mention %q", want)
		}
	}

	path := writeConfig(t, t.TempDir(), "printed.yaml", printed)
	cfg, _, err := config.Load(config.LoadOptions{ExplicitPath: path})
	if err != nil {
		t.Fatalf("the printed configuration does not load: %v", err)
	}
	got, err := ui.ResolveTheme(cfg.Theme)
	if err != nil {
		t.Fatalf("the printed theme does not resolve: %v", err)
	}
	if want := ui.BuiltinThemes()[config.DefaultThemeName]; !reflect.DeepEqual(got, want) {
		t.Errorf("printed theme = %+v, want the built-in %s theme %+v", got, config.DefaultThemeName, want)
	}
}
