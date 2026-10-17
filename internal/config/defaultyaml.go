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

	b.WriteString(`
# Color theme: dark (the original look), light, solarized or nord.
# --theme <name> on the command line overrides the name.
theme:
`)
	fmt.Fprintf(&b, "  name: %s\n", DefaultThemeName)
	b.WriteString(`  # Override single colors by role. A color is a name (yellow, navy, aqua...) or
  # #rrggbb; quote the #rrggbb form. The roles are:
  #   background, text, muted, accent, border, title, info, subheading, faint
  #   status_ok, status_warn, status_error
  #   help_bar_bg, help_key, help_text
  #   log_fatal, log_fatal_text, log_error, log_warn, log_info, log_debug
  #     The log level badges are not drawn (the UI library reads [INFO] and the
  #     like as color tags), so for now log_fatal only colors the background of
  #     the rest of a FATAL line and the other five have no visible effect.
  #   field_bg, field_hint (the search field and its placeholder)
  # colors:
  #   accent: "#ff8800"
  #   status_error: red

# Panel layout, from top to bottom: services, resources, the search field,
# logs and the help bar. height is the share of the free space a panel gets
# relative to the others, from 1 to 100 (1 = equal, 2 = twice as much; 0 or
# omitted means 1).
# visible: false removes a panel from the screen. The services table and the
# search field are always shown; without the logs the focus starts in the
# services table.
layout:
  services:
    height: 1
  resources:
    visible: true
    height: 1
  logs:
    visible: true
    height: 1
  help_bar:
    visible: true

# Keyboard shortcuts. preset picks the starting keys: default (the keys GoWatch
# has always had, plus ? for the help page) or vim (default plus j/k to scroll,
# g/G for the top and the bottom, ctrl+d/ctrl+u to page). bindings replaces ALL
# the keys of an action; an action you leave out keeps the keys of the preset.
# Press ? in the dashboard to see the keys in use.
#
# A key is a single character (q, /, ?, G; upper and lower case are different
# keys), ctrl+<letter>, or one of: esc, enter, tab, up, down, pgup, pgdn, home,
# end, space. Quote the keys in YAML: "?", "/", "#"...
#   - ctrl+c always quits GoWatch and cannot be bound. ctrl+h, ctrl+i and ctrl+m
#     are the Backspace, Tab and Enter keys in a terminal and are not accepted.
#   - up, down, pgup, pgdn, home, end (and ctrl+f, ctrl+b) always scroll the
#     panel with focus, so they can only be bound to the scroll action that
#     matches what they do.
#   - Scrolling with up, down, pgup, pgdn, home or end stops the logs from
#     following new lines, end included. A scroll_bottom key that is not one of
#     them (G in the vim preset) makes the logs follow the newest line again.
#   - A key can run only one action. To swap the keys of two actions, set both.
#
# Actions, with their keys in the default preset:
#   help                   ?         show or hide the help page
#   quit                   q         quit GoWatch
#   docker_view            d, D      show the Docker services
#   aws_view               a, A      show the AWS resources
#   search                 /, f      focus the search field
#   cycle_log_level        l, L      cycle the minimum log level
#   clear_filter           esc       clear all filters (closes the alerts list first)
#   toggle_alerts          w, W      show or hide the alerts list
#   clear_resolved_alerts  c, C      clear the resolved alerts
#   scroll_up              up        scroll up a line
#   scroll_down            down      scroll down a line
#   page_up                pgup      scroll up a page
#   page_down              pgdn      scroll down a page
#   scroll_top             home      jump to the top
#   scroll_bottom          end       jump to the bottom
keybindings:
  preset: default
  # bindings:
  #   quit: ["q", "x"]
  #   scroll_down: ["down", "j"]
  #   scroll_up: ["up", "k"]
`)

	return []byte(b.String())
}
