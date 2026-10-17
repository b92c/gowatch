package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/b92c/gowatch/internal/alert"
)

// Severity names accepted in alert rule overrides.
const (
	severityWarn = "warn"
	severityCrit = "crit"
)

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
