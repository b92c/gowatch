// Package config loads, layers and validates the GoWatch YAML configuration.
//
// It is a leaf package on purpose: it only depends on the standard library,
// the YAML decoder, internal/alert and pkg/metrics. It must never import
// internal/docker, internal/ui or internal/filter, which depend on the values
// it produces.
package config

import (
	"time"

	"github.com/b92c/gowatch/pkg/metrics"
)

// Built-in defaults. They reproduce the values that used to be hard-coded, so
// running without any config file behaves exactly as before.
const (
	defaultRefreshInterval = 2 * time.Second
	defaultTailLines       = 50
	defaultOTLPAddress     = "127.0.0.1:4318"
)

// DefaultThemeName is the theme used when theme.name is empty. The UI package
// defines the built-in themes; the name lives here so the example configuration
// can mention it without importing the UI.
const DefaultThemeName = "dark"

// Config is the complete GoWatch configuration.
type Config struct {
	Layout          LayoutConfig      `yaml:"layout"`
	Filters         FiltersConfig     `yaml:"filters"`
	AWS             AWSConfig         `yaml:"aws"`
	Keybindings     KeybindingsConfig `yaml:"keybindings"`
	Theme           ThemeConfig       `yaml:"theme"`
	Alerts          AlertsConfig      `yaml:"alerts"`
	Tracing         TracingConfig     `yaml:"tracing"`
	Logs            LogsConfig        `yaml:"logs"`
	History         HistoryConfig     `yaml:"history"`
	RefreshInterval time.Duration     `yaml:"refresh_interval"`
}

// LogsConfig controls log collection.
type LogsConfig struct {
	TailLines int `yaml:"tail_lines"`
}

// HistoryConfig controls how many samples are kept for the sparklines.
type HistoryConfig struct {
	MaxPoints int `yaml:"max_points"`
}

// FiltersConfig holds the filters applied when the dashboard starts.
type FiltersConfig struct {
	Search          string   `yaml:"search"`
	MinLogLevelName string   `yaml:"min_log_level"`
	Status          []string `yaml:"status"`
}

// TracingConfig groups the tracing settings.
type TracingConfig struct {
	OTLPReceiver OTLPReceiverConfig `yaml:"otlp_receiver"`
}

// OTLPReceiverConfig controls the embedded OTLP/HTTP trace receiver.
type OTLPReceiverConfig struct {
	Address string `yaml:"address"`
	Enabled bool   `yaml:"enabled"`
}

// AWSConfig holds the AWS integration settings. Enabled is one of "auto",
// "true" or "false"; AWS_* environment variables take priority over the other
// fields.
type AWSConfig struct {
	Region      string `yaml:"region"`
	EndpointURL string `yaml:"endpoint_url"`
	Enabled     string `yaml:"enabled"`
}

// ThemeConfig is the raw theme section. It is decoded here but interpreted by
// the UI layer, which checks the theme name and the color roles and values.
type ThemeConfig struct {
	Colors map[string]string `yaml:"colors"`
	Name   string            `yaml:"name"`
}

// LayoutConfig is the raw layout section. It is decoded here but interpreted
// by the UI layer. A nil Visible means "not configured", which shows the panel.
// A Height is a relative weight; 0 means "not configured", which counts as 1.
// Validate checks the heights.
type LayoutConfig struct {
	HelpBar   HelpBarLayout  `yaml:"help_bar"`
	Resources PanelLayout    `yaml:"resources"`
	Logs      PanelLayout    `yaml:"logs"`
	Services  ServicesLayout `yaml:"services"`
}

// PanelLayout is the layout of a panel that can be hidden and resized.
type PanelLayout struct {
	Visible *bool `yaml:"visible"`
	Height  int   `yaml:"height"`
}

// ServicesLayout is the layout of the services table, which is always shown.
type ServicesLayout struct {
	Height int `yaml:"height"`
}

// HelpBarLayout is the layout of the help bar.
type HelpBarLayout struct {
	Visible *bool `yaml:"visible"`
}

// KeybindingsConfig is the raw keybindings section. It is decoded here but
// interpreted by the UI layer.
type KeybindingsConfig struct {
	Bindings map[string][]string `yaml:"bindings"`
	Preset   string              `yaml:"preset"`
}

// Default returns the built-in configuration, equal to the behavior GoWatch had
// before configuration files existed. The theme, layout and keybindings
// sections are left empty: empty means "built-in".
func Default() Config {
	return Config{
		RefreshInterval: defaultRefreshInterval,
		Logs:            LogsConfig{TailLines: defaultTailLines},
		History:         HistoryConfig{MaxPoints: metrics.MaxHistoryPoints},
		Alerts:          AlertsConfig{Enabled: true},
		Tracing:         TracingConfig{OTLPReceiver: OTLPReceiverConfig{Address: defaultOTLPAddress}},
		AWS:             AWSConfig{Enabled: awsEnabledAuto},
	}
}
