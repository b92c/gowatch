package config

import (
	"reflect"
	"testing"
	"time"

	"github.com/b92c/gowatch/internal/alert"
	"github.com/b92c/gowatch/pkg/metrics"
)

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

// The theme, layout and keybindings sections are decoded as raw data and
// interpreted by the UI layer (ui.ResolveTheme, ui.BuildKeymap), which is where
// a reserved key such as ctrl+c is refused: this package does not know the keys
// or the actions.
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
