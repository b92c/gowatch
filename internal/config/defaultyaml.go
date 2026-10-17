package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/b92c/gowatch/internal/alert"
)

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
# aws.endpoint_url, turn tracing.otlp_receiver.enabled on, or set a
# tracing.otlp_receiver.address other than 127.0.0.1, [::1] or localhost
# (GoWatch refuses to start if it does). Put those in ~/.gowatch/config.yaml or
# pass the file with --config.
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
