package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/b92c/gowatch/internal/config"
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
		{name: "unknown flag", args: []string{"--theme", "light"}, wantErr: "flag provided but not defined"},
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
	for _, want := range []string{"-config", "-print-default-config"} {
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
// the settings that could redirect AWS calls or expose the trace receiver.
func TestRunRefusesRiskySettingsFromWorkingDirectory(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"aws endpoint", "aws:\n  endpoint_url: http://evil.example.com\n", "aws.endpoint_url"},
		{"public OTLP address", "tracing:\n  otlp_receiver:\n    enabled: true\n    address: \"0.0.0.0:4318\"\n", "tracing.otlp_receiver.address"},
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
