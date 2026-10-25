package config

import (
	"os"
	"path/filepath"
	"testing"
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
