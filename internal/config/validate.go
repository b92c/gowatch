package config

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/b92c/gowatch/pkg/metrics"
)

// Accepted ranges for user supplied values.
const (
	minRefreshInterval = 500 * time.Millisecond
	minTailLines       = 1
	maxTailLines       = 5000
	minHistoryPoints   = 10
	maxHistoryPoints   = 3600
	maxLayoutHeight    = 100
	minMaxConcurrency  = 1
	maxMaxConcurrency  = 128
)

// Accepted spellings of aws.enabled.
const (
	awsEnabledAuto  = "auto"
	awsEnabledTrue  = "true"
	awsEnabledFalse = "false"
)

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
	errs = append(errs, c.Tracing.validate()...)
	errs = append(errs, c.Layout.validate()...)
	errs = append(errs, c.Performance.validate(c.RefreshInterval)...)

	return errors.Join(errs...)
}

func (p PerformanceConfig) validate(refreshInterval time.Duration) []error {
	var errs []error
	if p.MaxConcurrency < minMaxConcurrency || p.MaxConcurrency > maxMaxConcurrency {
		errs = append(errs, fmt.Errorf("performance.max_concurrency must be between %d and %d, got %d", minMaxConcurrency, maxMaxConcurrency, p.MaxConcurrency))
	}
	if p.MaxRefreshInterval < refreshInterval {
		errs = append(errs, fmt.Errorf("performance.max_refresh_interval (%s) must be at least refresh_interval (%s)", p.MaxRefreshInterval, refreshInterval))
	}
	return errs
}

func (t TracingConfig) validate() []error {
	var errs []error
	if t.SamplingRate != nil && (math.IsNaN(*t.SamplingRate) || *t.SamplingRate < 0 || *t.SamplingRate > 1) {
		errs = append(errs, fmt.Errorf("tracing.sampling_rate must be between 0 and 1, got %g", *t.SamplingRate))
	}
	if t.Exporter.Format != "" {
		switch strings.ToLower(strings.TrimSpace(t.Exporter.Format)) {
		case "otlp", "jaeger", "gowatch":
		default:
			errs = append(errs, fmt.Errorf("tracing.exporter.format must be otlp, jaeger or gowatch, got %q", t.Exporter.Format))
		}
	}
	if t.Exporter.Enabled && strings.TrimSpace(t.Exporter.Endpoint) == "" {
		errs = append(errs, errors.New("tracing.exporter.endpoint must be set when tracing.exporter.enabled is true"))
	}
	return errs
}

// validate checks the panel heights. They are relative weights: 0 means "not
// set" and counts as 1, so only negative or absurdly large values are errors.
// The upper bound keeps the sum of the weights far from integer overflow.
func (l LayoutConfig) validate() []error {
	var errs []error
	for _, panel := range []struct {
		name   string
		height int
	}{
		{name: "services", height: l.Services.Height},
		{name: "resources", height: l.Resources.Height},
		{name: "logs", height: l.Logs.Height},
	} {
		if panel.height < 0 || panel.height > maxLayoutHeight {
			errs = append(errs, fmt.Errorf("layout.%s.height must be between 1 and %d (0 or omitted means 1), got %d", panel.name, maxLayoutHeight, panel.height))
		}
	}
	return errs
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
		if o.Pattern != nil && *o.Pattern != "" {
			if _, err := regexp.Compile(*o.Pattern); err != nil {
				errs = append(errs, fmt.Errorf("alerts.rules.%s.pattern is not a valid regular expression: %w", id, err))
			}
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
	if a.Notifications.SMTP.Host != "" {
		if a.Notifications.SMTP.Port < 1 || a.Notifications.SMTP.Port > 65535 {
			errs = append(errs, fmt.Errorf("alerts.notifications.smtp.port must be between 1 and 65535, got %d", a.Notifications.SMTP.Port))
		}
		if len(a.Notifications.SMTP.To) == 0 {
			errs = append(errs, errors.New("alerts.notifications.smtp.to must list at least one recipient"))
		}
	}
	return errs
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
