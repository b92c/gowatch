// Package config loads, layers and validates the GoWatch YAML configuration.
//
// It is a leaf package on purpose: it only depends on the standard library,
// the YAML decoder, internal/alert and pkg/metrics. It must never import
// internal/docker, internal/ui or internal/filter, which depend on the values
// it produces.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/b92c/gowatch/internal/alert"
	"github.com/b92c/gowatch/pkg/metrics"
	"go.yaml.in/yaml/v3"
)

// Built-in defaults. They reproduce the values that used to be hard-coded, so
// running without any config file behaves exactly as before.
const (
	defaultRefreshInterval = 2 * time.Second
	defaultTailLines       = 50
	defaultOTLPAddress     = "127.0.0.1:4318"
)

// Accepted ranges for user supplied values.
const (
	minRefreshInterval = 500 * time.Millisecond
	minTailLines       = 1
	maxTailLines       = 5000
	minHistoryPoints   = 10
	maxHistoryPoints   = 3600
)

// Accepted spellings of aws.enabled.
const (
	awsEnabledAuto  = "auto"
	awsEnabledTrue  = "true"
	awsEnabledFalse = "false"
)

// Severity names accepted in alert rule overrides.
const (
	severityWarn = "warn"
	severityCrit = "crit"
)

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

// AlertsConfig toggles the alerting engine and overrides its built-in rules.
type AlertsConfig struct {
	RuleOverrides RuleOverrides `yaml:"rules"`
	Enabled       bool          `yaml:"enabled"`
}

// AlertRuleOverride changes fields of a built-in alert rule. A nil pointer or
// an empty string means "keep the built-in value".
type AlertRuleOverride struct {
	Threshold       *float64 `yaml:"threshold"`
	Hysteresis      *float64 `yaml:"hysteresis"`
	ConsecutiveHits *int     `yaml:"consecutive_hits"`
	Enabled         *bool    `yaml:"enabled"`
	Severity        string   `yaml:"severity"`
}

// RuleOverrides maps lower-case rule IDs (high_cpu, high_mem, ...) to their
// overrides. When several config layers override the same rule, the layers are
// combined field by field instead of the later one replacing the earlier one.
type RuleOverrides map[string]AlertRuleOverride

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
// the UI layer.
type ThemeConfig struct {
	Colors map[string]string `yaml:"colors"`
	Name   string            `yaml:"name"`
}

// LayoutConfig is the raw layout section. It is decoded here but interpreted
// by the UI layer. A nil Visible means "not configured".
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

// LoadOptions tells Load where to look for configuration files. The
// directories are injectable so tests can use temporary directories.
type LoadOptions struct {
	// ExplicitPath, when set, is the only file read besides the built-in
	// defaults. It must exist.
	ExplicitPath string
	// HomeDir is the directory that holds .gowatch/config.yaml. Empty skips
	// that layer.
	HomeDir string
	// WorkDir is the directory that holds gowatch.yaml. Empty skips that layer.
	// A working directory can be an untrusted checkout, so this layer may not
	// set aws.endpoint_url or a non-loopback tracing.otlp_receiver.address;
	// Load rejects the file if it does.
	WorkDir string
}

// Load builds the configuration from the built-in defaults and the config files,
// in increasing priority: ~/.gowatch/config.yaml, then ./gowatch.yaml. With
// ExplicitPath only that file is read. Each file is decoded over the result of
// the previous layer, so a key a file leaves out keeps its earlier value.
// Lists are replaced as a whole, maps are merged by key.
//
// Missing files in the default locations are skipped. Unknown keys, malformed
// YAML and invalid values are errors. Load returns the files it read, in the
// order they were applied.
func Load(opts LoadOptions) (*Config, []string, error) {
	cfg := Default()

	var sources []source
	if opts.ExplicitPath != "" {
		sources = []source{{path: opts.ExplicitPath, required: true}}
	} else {
		if opts.HomeDir != "" {
			sources = append(sources, source{path: filepath.Join(opts.HomeDir, ".gowatch", "config.yaml")})
		}
		if opts.WorkDir != "" {
			sources = append(sources, source{path: filepath.Join(opts.WorkDir, "gowatch.yaml"), untrusted: true})
		}
	}

	var loaded []string
	for _, src := range sources {
		found, err := cfg.applyFile(src)
		if err != nil {
			return nil, nil, err
		}
		if found {
			loaded = append(loaded, filepath.Clean(src.path))
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid configuration (%s):\n%w", describeSources(loaded), err)
	}
	return &cfg, loaded, nil
}

// source is one config file layer.
type source struct {
	path string
	// required makes a missing file an error (the --config file).
	required bool
	// untrusted marks the working-directory file, which may come from a cloned
	// repository and so may not set the values in untrustedSettings.
	untrusted bool
}

// applyFile decodes one config file over c. It reports whether the file
// existed; a missing file is only an error when the source is required.
func (c *Config) applyFile(src source) (bool, error) {
	// The path is chosen by the local user (--config flag or the fixed default
	// locations), so it is cleaned but not restricted to a directory.
	clean := filepath.Clean(src.path)
	data, err := os.ReadFile(clean)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && !src.required {
			return false, nil
		}
		return false, fmt.Errorf("reading config file: %w", err)
	}

	if src.untrusted {
		// Decode into a fresh Config first: only what this file itself sets is
		// non-zero there, whatever earlier layers contributed.
		var probe Config
		if err := decodeStrict(clean, data, &probe); err != nil {
			return true, err
		}
		if err := probe.untrustedSettings(clean); err != nil {
			return true, err
		}
	}
	return true, decodeStrict(clean, data, c)
}

// decodeStrict decodes the single YAML document in data into c, rejecting
// unknown keys and any further document.
func decodeStrict(path string, data []byte, c *Config) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil {
		if errors.Is(err, io.EOF) { // empty or comment-only file
			return nil
		}
		return decodeError(path, err)
	}

	// Without this check a second document would be silently ignored, typos
	// included.
	var extra yaml.Node
	switch err := dec.Decode(&extra); {
	case errors.Is(err, io.EOF):
		return nil
	case err != nil:
		return decodeError(path, err)
	case isEmptyDocument(&extra):
		return nil
	default:
		return fmt.Errorf("%s: line %d: only one YAML document per file is supported, remove the extra \"---\" section", path, firstLine(&extra))
	}
}

// isEmptyDocument reports whether n is a document with no content, such as a
// trailing "---".
func isEmptyDocument(n *yaml.Node) bool {
	if n.Kind != yaml.DocumentNode || len(n.Content) == 0 {
		return true
	}
	body := n.Content[0]
	return body.Kind == yaml.ScalarNode && body.Tag == "!!null"
}

func firstLine(n *yaml.Node) int {
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return n.Content[0].Line
	}
	return n.Line
}

// untrustedSettings lists the settings the working-directory file may not set,
// because a repository you merely cloned could otherwise use them to redirect
// AWS calls or to expose the trace receiver on the network. They are still
// accepted from ~/.gowatch/config.yaml and from --config.
func (c *Config) untrustedSettings(path string) error {
	const advice = "move it to ~/.gowatch/config.yaml or pass the file with --config"

	var errs []error
	if c.AWS.EndpointURL != "" {
		errs = append(errs, fmt.Errorf("%s: aws.endpoint_url is not honored from the working-directory gowatch.yaml (it could come from an untrusted checkout); %s", path, advice))
	}
	if addr := c.Tracing.OTLPReceiver.Address; addr != "" && !isLoopbackAddress(addr) {
		errs = append(errs, fmt.Errorf("%s: tracing.otlp_receiver.address %q is not honored from the working-directory gowatch.yaml unless it is 127.0.0.1, [::1] or localhost; %s", path, addr, advice))
	}
	return errors.Join(errs...)
}

// isLoopbackAddress reports whether a host:port address names only the local
// machine. Anything it cannot positively identify, such as an empty host, a
// wildcard address or a malformed value, counts as not loopback.
func isLoopbackAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	switch strings.ToLower(host) {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
}

// decodeError prefixes every YAML problem with the file it came from. The YAML
// library already includes the line number in each message.
func decodeError(path string, err error) error {
	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) {
		errs := make([]error, 0, len(typeErr.Errors))
		for _, msg := range typeErr.Errors {
			errs = append(errs, fmt.Errorf("%s: %s", path, msg))
		}
		return errors.Join(errs...)
	}
	return fmt.Errorf("%s: %w", path, err)
}

func describeSources(files []string) string {
	if len(files) == 0 {
		return "built-in defaults"
	}
	return strings.Join(files, ", ")
}

// Validate checks every value and reports all problems at once.
func (c *Config) Validate() error {
	var errs []error

	if c.RefreshInterval < minRefreshInterval {
		errs = append(errs, fmt.Errorf("refresh_interval must be at least %s, got %s", minRefreshInterval, c.RefreshInterval))
	}
	if c.Logs.TailLines < minTailLines || c.Logs.TailLines > maxTailLines {
		errs = append(errs, fmt.Errorf("logs.tail_lines must be between %d and %d, got %d", minTailLines, maxTailLines, c.Logs.TailLines))
	}
	if c.History.MaxPoints < minHistoryPoints || c.History.MaxPoints > maxHistoryPoints {
		errs = append(errs, fmt.Errorf("history.max_points must be between %d and %d, got %d", minHistoryPoints, maxHistoryPoints, c.History.MaxPoints))
	}
	if _, ok := parseMinLogLevel(c.Filters.MinLogLevelName); !ok {
		errs = append(errs, fmt.Errorf("filters.min_log_level must be info, warn or error (or empty), got %q", c.Filters.MinLogLevelName))
	}
	if !validAWSEnabled(c.AWS.Enabled) {
		errs = append(errs, fmt.Errorf("aws.enabled must be auto, true or false, got %q", c.AWS.Enabled))
	}
	errs = append(errs, c.Alerts.validate()...)

	return errors.Join(errs...)
}

func (a AlertsConfig) validate() []error {
	defaults := defaultRulesByKey()
	known := strings.Join(sortedKeys(defaults), ", ")

	ids := make([]string, 0, len(a.RuleOverrides))
	for id := range a.RuleOverrides {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var errs []error
	for _, id := range ids {
		base, isBuiltin := defaults[id]
		if !isBuiltin {
			errs = append(errs, fmt.Errorf("alerts.rules.%s: unknown rule ID (known: %s)", id, known))
			continue
		}

		o := a.RuleOverrides[id]
		if o.Severity != "" {
			if _, ok := parseSeverity(o.Severity); !ok {
				errs = append(errs, fmt.Errorf("alerts.rules.%s.severity must be %s or %s, got %q", id, severityWarn, severityCrit, o.Severity))
			}
		}
		if o.ConsecutiveHits != nil && *o.ConsecutiveHits < 1 {
			errs = append(errs, fmt.Errorf("alerts.rules.%s.consecutive_hits must be at least 1, got %d", id, *o.ConsecutiveHits))
		}
		validNumbers := true
		for _, field := range []struct {
			value *float64
			name  string
		}{{value: o.Threshold, name: "threshold"}, {value: o.Hysteresis, name: "hysteresis"}} {
			if field.value != nil && (math.IsNaN(*field.value) || math.IsInf(*field.value, 0) || *field.value < 0) {
				errs = append(errs, fmt.Errorf("alerts.rules.%s.%s must be a finite number >= 0, got %g", id, field.name, *field.value))
				validNumbers = false
			}
		}
		// Only an explicit hysteresis is checked. When just the threshold is
		// overridden, Rules lowers the built-in hysteresis to match.
		if o.Hysteresis != nil && validNumbers {
			threshold := base.Threshold
			if o.Threshold != nil {
				threshold = *o.Threshold
			}
			if *o.Hysteresis > threshold {
				errs = append(errs, fmt.Errorf("alerts.rules.%s.hysteresis (%g) must not exceed threshold (%g)", id, *o.Hysteresis, threshold))
			}
		}
	}
	return errs
}

// Rules returns the alert rules to run: the built-in rules with the overrides
// applied. A rule with enabled: false is dropped, and alerts.enabled: false
// yields an empty, non-nil list.
func (a AlertsConfig) Rules() []alert.AlertRule {
	if !a.Enabled {
		return []alert.AlertRule{}
	}

	builtin := alert.DefaultRules()
	rules := make([]alert.AlertRule, 0, len(builtin))
	for _, rule := range builtin {
		o, overridden := a.RuleOverrides[ruleKey(rule.ID)]
		if !overridden {
			rules = append(rules, rule)
			continue
		}
		if o.Enabled != nil && !*o.Enabled {
			continue
		}
		rules = append(rules, o.apply(rule))
	}
	return rules
}

// apply returns rule with the override's fields applied.
func (o AlertRuleOverride) apply(rule alert.AlertRule) alert.AlertRule {
	if o.Threshold != nil {
		rule.Description = strings.Replace(rule.Description, formatPercent(rule.Threshold), formatPercent(*o.Threshold), 1)
		rule.Threshold = *o.Threshold
	}
	if o.Hysteresis != nil {
		rule.HysteresisThreshold = *o.Hysteresis
	} else if rule.HysteresisThreshold > rule.Threshold {
		rule.HysteresisThreshold = rule.Threshold
	}
	if o.ConsecutiveHits != nil {
		rule.ConsecutiveHits = *o.ConsecutiveHits
	}
	if sev, ok := parseSeverity(o.Severity); ok {
		rule.Severity = sev
	}
	return rule
}

// merge returns o with every field that in sets taking the value from in.
func (o AlertRuleOverride) merge(in AlertRuleOverride) AlertRuleOverride {
	if in.Threshold != nil {
		o.Threshold = in.Threshold
	}
	if in.Hysteresis != nil {
		o.Hysteresis = in.Hysteresis
	}
	if in.ConsecutiveHits != nil {
		o.ConsecutiveHits = in.ConsecutiveHits
	}
	if in.Enabled != nil {
		o.Enabled = in.Enabled
	}
	if in.Severity != "" {
		o.Severity = in.Severity
	}
	return o
}

// UnmarshalYAML merges the decoded rules into the ones already present, so
// that a later config file only replaces the fields it sets. It uses the
// callback form of the interface because that is the one that keeps the
// decoder's strict unknown-key checking.
func (r *RuleOverrides) UnmarshalYAML(unmarshal func(any) error) error {
	var incoming map[string]AlertRuleOverride
	if err := unmarshal(&incoming); err != nil {
		return err
	}

	// Two spellings of one rule (high_cpu and HIGH_CPU) in the same file would
	// be merged in random map order, so reject them.
	ids := make([]string, 0, len(incoming))
	for id := range incoming {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	spelled := make(map[string]string, len(ids))
	for _, id := range ids {
		key := ruleKey(id)
		if first, dup := spelled[key]; dup {
			return fmt.Errorf("alerts.rules: rule %q is defined twice (%q and %q)", key, first, id)
		}
		spelled[key] = id
	}

	if *r == nil {
		*r = make(RuleOverrides, len(incoming))
	}
	for id, o := range incoming {
		key := ruleKey(id)
		(*r)[key] = (*r)[key].merge(o)
	}
	return nil
}

// ruleKey is the canonical map key for a rule ID: the config files spell IDs
// in lower case (high_cpu) while the alert package uses upper case (HIGH_CPU).
func ruleKey(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func defaultRulesByKey() map[string]alert.AlertRule {
	rules := make(map[string]alert.AlertRule)
	for _, rule := range alert.DefaultRules() {
		rules[ruleKey(rule.ID)] = rule
	}
	return rules
}

func sortedKeys(m map[string]alert.AlertRule) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func parseSeverity(s string) (alert.Severity, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case severityWarn:
		return alert.SeverityWarning, true
	case severityCrit:
		return alert.SeverityCritical, true
	}
	return "", false
}

func severityName(s alert.Severity) string {
	if s == alert.SeverityCritical {
		return severityCrit
	}
	return severityWarn
}

// formatPercent renders a threshold the way the built-in rule descriptions do
// ("85%"), so a description can follow an overridden threshold.
func formatPercent(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64) + "%"
}

// MinLogLevel returns the configured minimum log level, or
// metrics.LogLevelUnknown (no filtering) when it is empty or invalid.
func (f FiltersConfig) MinLogLevel() metrics.LogLevel {
	level, _ := parseMinLogLevel(f.MinLogLevelName)
	return level
}

func parseMinLogLevel(s string) (metrics.LogLevel, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return metrics.LogLevelUnknown, true
	case "info":
		return metrics.LogLevelInfo, true
	case "warn":
		return metrics.LogLevelWarn, true
	case "error":
		return metrics.LogLevelError, true
	}
	return metrics.LogLevelUnknown, false
}

// EnabledOverride maps aws.enabled to a tri-state: nil for "auto" (detect from
// the environment), otherwise a pointer to the forced value.
func (a AWSConfig) EnabledOverride() *bool {
	var forced bool
	switch strings.ToLower(strings.TrimSpace(a.Enabled)) {
	case awsEnabledTrue:
		forced = true
	case awsEnabledFalse:
		forced = false
	default:
		return nil
	}
	return &forced
}

func validAWSEnabled(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", awsEnabledAuto, awsEnabledTrue, awsEnabledFalse:
		return true
	}
	return false
}

// DefaultYAML returns the commented example configuration printed by
// --print-default-config. Every value is the built-in default, so using it as
// a config file changes nothing. It only documents the sections that are
// currently supported.
func DefaultYAML() []byte {
	def := Default()
	var b strings.Builder

	b.WriteString(`# GoWatch configuration.
#
# GoWatch works without any configuration file. When files exist they are
# applied in this order, each one overriding only the keys it sets:
#   1. built-in defaults
#   2. ~/.gowatch/config.yaml
#   3. ./gowatch.yaml
# AWS_* environment variables take priority over the aws section.
# With --config <file>, only the built-in defaults and that file are used.
#
# ./gowatch.yaml may come from a repository you merely cloned, so it cannot set
# aws.endpoint_url or a tracing.otlp_receiver.address other than 127.0.0.1,
# [::1] or localhost (GoWatch refuses to start if it does). Put those in
# ~/.gowatch/config.yaml or pass the file with --config.
#
# Unknown keys and invalid values stop GoWatch at startup with a message.

# How often containers are polled. Go duration syntax (500ms, 2s, 1m); minimum 500ms.
`)
	fmt.Fprintf(&b, "refresh_interval: %s\n", def.RefreshInterval)

	b.WriteString("\nlogs:\n  # Log lines fetched per container on every refresh (1-5000).\n")
	fmt.Fprintf(&b, "  tail_lines: %d\n", def.Logs.TailLines)

	b.WriteString("\nhistory:\n  # Samples kept per container for the CPU and memory sparklines (10-3600).\n")
	fmt.Fprintf(&b, "  max_points: %d\n", def.History.MaxPoints)

	b.WriteString(`
# Filters applied when GoWatch starts. Esc in the dashboard clears all of them.
filters:
  search: ""          # matches service name, container ID or image
  min_log_level: ""   # info | warn | error (empty shows every level)
  status: []          # container states to show, for example [running, exited]

alerts:
`)
	fmt.Fprintf(&b, "  enabled: %-5t # false turns the alerting engine off\n", def.Alerts.Enabled)
	b.WriteString(`  # Override the built-in rules by ID. Fields you leave out keep the built-in
  # value; set enabled: false to drop a rule. severity is warn or crit.
  # hysteresis must not exceed threshold: if you change threshold and leave
  # hysteresis commented out, it is lowered to match when needed.
  rules:
`)
	for _, rule := range alert.DefaultRules() {
		fmt.Fprintf(&b, "    %s:\n", ruleKey(rule.ID))
		fmt.Fprintf(&b, "      threshold: %s\n", strconv.FormatFloat(rule.Threshold, 'f', -1, 64))
		fmt.Fprintf(&b, "      # hysteresis: %s\n", strconv.FormatFloat(rule.HysteresisThreshold, 'f', -1, 64))
		fmt.Fprintf(&b, "      consecutive_hits: %d\n", rule.ConsecutiveHits)
		fmt.Fprintf(&b, "      severity: %s\n", severityName(rule.Severity))
		b.WriteString("      enabled: true\n")
	}

	b.WriteString("\ntracing:\n  otlp_receiver:\n")
	fmt.Fprintf(&b, "    enabled: %t\n", def.Tracing.OTLPReceiver.Enabled)
	fmt.Fprintf(&b, "    address: %q   # accepts OTLP/HTTP JSON on /v1/traces\n", def.Tracing.OTLPReceiver.Address)

	b.WriteString(`
aws:
  region: ""          # AWS_REGION / AWS_DEFAULT_REGION take priority; falls back to us-east-1
  endpoint_url: ""    # AWS_ENDPOINT_URL takes priority (LocalStack, for example)
`)
	fmt.Fprintf(&b, "  enabled: %s       # auto | true | false; auto detects the AWS_* environment\n", def.AWS.Enabled)

	return []byte(b.String())
}
