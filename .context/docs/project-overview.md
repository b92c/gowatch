---
type: doc
name: project-overview
description: High-level overview of the project, its purpose, and key components
category: overview
generated: 2026-03-22
status: filled
scaffoldVersion: "2.0.0"
---

# Project Overview

## Summary

GoWatch is a lightweight, real-time monitoring tool written in Go for Docker containers. It provides developers with instant visibility into container resource usage, logs, and system metrics directly in the terminal. Perfect for local development environments, GoWatch works with any service running in Docker regardless of the programming language.

## Architecture

GoWatch follows the standard Go project layout:

```
gowatch/
├── cmd/gowatch/          # Application entry point (main.go)
├── internal/
│   ├── docker/           # Docker daemon interaction (collector.go, parser.go)
│   ├── ui/               # Terminal UI: dashboard.go, components.go, theme.go (themes), layout.go (panel grid), texts.go (themed texts), keymap.go (keys and presets), input.go (key handling), help.go (help page), aws_view.go (AWS table)
│   ├── config/           # YAML configuration: config.go (types, Default), load.go, validate.go, untrusted.go, alerts.go, defaultyaml.go
│   ├── trace/            # Distributed tracing: types.go (trace store), correlator.go (log correlator), exporter.go (OTLP receiver, exporter)
│   └── aws/              # AWS client: client.go (manager and state), settings.go, errors.go, sdk.go, poller.go, accounts.go, sso.go
├── pkg/metrics/          # Metrics types (types.go)
├── docker-compose.yaml   # Docker development environment
└── makefile              # Build and development commands
```

## Key Components

- **Docker Collector** (`internal/docker/collector.go`): Connects to Docker daemon, watches containers, collects stats (CPU, memory), streams logs. Uses Moby client library for direct API access.
- **Stats Parser** (`internal/docker/parser.go`): Parses Docker cgroup statistics and multiplexed log streams.
- **Dashboard** (`internal/ui/dashboard.go`): Interactive terminal UI with services table, resource panel, and aggregated logs view using tcell/tview. `NewDashboardWithOptions(ui.Options{Theme, Keymap, Layout})` builds it from a resolved theme, a keymap and the layout config; `NewDashboard()` is the same with the built-in dark theme, the default keymap and the default layout.
- **Themes** (`internal/ui/theme.go`): A `Theme` holds one color per semantic role (text, accent, status colors, help bar, search field, log level...) plus the service palette. The log level roles (`log_*`) have little visible effect yet because tview does not draw the level badges. `BuiltinThemes()` has `dark`, `light`, `solarized` and `nord`; `dark` reproduces the original colors exactly. `ResolveTheme()` applies the `theme.colors` overrides and reports an unknown theme, role or color. `Theme.Tag()` turns a color into a tview tag and `Theme.Apply()` sets tview's global styles; the dashboard reads every color from its theme and never from literals.
- **Keybindings** (`internal/ui/keymap.go`): An `Action` is something the user can do (`quit`, `search`, `toggle_alerts`, `scroll_down`, `help`...), each with a group and a description that feed the help page. `ParseKey()` reads a key as written in the config (`q`, `ctrl+d`, `pgup`...) into a `KeyCombo`; the presets are `default` (the keys GoWatch has always had, plus `?`) and `vim`. `BuildKeymap()` applies the `keybindings` section on top of a preset, replacing all the keys of an overridden action, and reports every problem at once (unknown preset, action or key, a key on two actions, `ctrl+c`, which is reserved, scroll keys given to the wrong action). `Keymap.Lookup()` finds the action of a key event, ignoring modifiers as the dashboard always did. The arrows, PgUp/PgDn and Home/End are never taken over: they stay with the panel that has focus.
- **Input and help** (`internal/ui/input.go`, `internal/ui/help.go`): `handleInput` is the application-wide input capture. While the search field has focus every key is typed into it; otherwise the keymap gives the action and `runAction` does it. Scroll actions on keys that are not native scroll keys (`j`/`k` in the `vim` preset) act on the logs view, on the alerts list when it is open, or on the help page. `?` opens the `help` page (a `tview.Table` built from the active keymap, drawn on top of the other pages); while it is open no other action runs and `?`, Esc and the quit keys close it. The help bar, the alerts list title and the error page bar (`texts.go`) are also built from the keymap, with `tview.Escape` around every key name.
- **Layout** (`internal/ui/layout.go`): `planGrid()`/`buildGrid()` build the main grid from the layout config: services, resources, the 3-row search field, logs and the 1-row help bar. Heights are proportional weights; resources, logs and the help bar can be hidden and then leave the grid. The focus returns to the logs, or to the services table when the logs are hidden.
- **Configuration** (`internal/config/`): Loads the optional YAML config (`~/.gowatch/config.yaml`, then `./gowatch.yaml`, or a single `--config` file) on top of `Default()`, decodes strictly (unknown keys are errors) and validates it. The `theme` and `keybindings` sections are decoded here as raw data and checked by `ui.ResolveTheme` and `ui.BuildKeymap`; the layout heights are checked by `Validate()`. With no file it returns exactly the built-in defaults, so GoWatch stays zero-config. It is a leaf package: it may import only the standard library, the YAML decoder, `internal/alert` and `pkg/metrics`, never `internal/docker`, `internal/ui` or `internal/filter`.
- **Tracing** (`internal/trace/`): An in-memory trace store (up to 500 traces; a trace older than 5 minutes is dropped when the next span arrives), a correlator that reads trace context from the Docker log lines on every refresh, and an OTLP/HTTP JSON receiver that `main` starts when `tracing.otlp_receiver.enabled` is true. The traces show in the System Resources panel (count, error count and the two most recent). The `Exporter` type exists, but nothing outside the tests calls it; there is no flamegraph and no log-to-span link.
- **AWS Integration** (`internal/aws/`): A read-only client built on the AWS SDK for Go v2. `AWSClientManager` resolves the profile, region, endpoint and whether AWS is configured when it is built, from the environment and the metadata of the shared files only. A poller goroutine that `main` starts does every AWS call, each with a 10 second timeout: it loads the SDK configuration, resolves the credentials and calls `sts:GetCallerIdentity` and, for a profile with an SSO login, lists the accounts and roles of the login and gets credentials for the accounts named in `aws.accounts`, then publishes a snapshot that `GetState()` returns as a copy. SDK errors become short texts with hints. The AWS view (`a`) shows the status in its title and a row for the account of the active credentials followed by one row per account of the SSO login, or one row that explains why there is none. CloudWatch Logs are not read yet.

## Tech Stack

- **Language**: Go 1.27+
- **Docker Client**: Moby (`github.com/moby/moby`) - direct Docker daemon API via Unix socket
- **Terminal UI**: tcell v2 (low-level) + tview (high-level widgets)
- **AWS**: AWS SDK for Go v2 (`github.com/aws/aws-sdk-go-v2`, a direct dependency: `config`, `credentials`, `feature/ec2/imds`, `service/sso`, `service/ssooidc`, `service/sts`); read-only, the identity of the active credentials and the accounts of the SSO login for now
- **Observability**: OpenTelemetry (planned: the OTLP receiver is written in the repository and the OpenTelemetry SDK is only an indirect dependency)
- **Configuration**: YAML via `go.yaml.in/yaml/v3`
- **Build System**: makefile (install, build, run, test, docker-build, go-sec, field-fix)
- **Development**: Docker + Docker Compose for containerized dev environment

## Core Data Flow

1. `main.go` parses the flags (`--config`, `--theme`, `--print-default-config`) and calls `config.Load()`; a config error is printed to stderr and the process exits before the TUI opens
2. `main.go` resolves the theme with `ui.ResolveTheme()` (the `--theme` flag replaces `theme.name`); an unknown theme, role or color is a startup error too
3. `main.go` builds the keymap with `ui.BuildKeymap()`; an unknown preset, action or key, a clash between actions or `ctrl+c` is a startup error (`gowatch: keybindings...` on stderr)
4. `main.go` creates Docker client via `client.New(client.FromEnv)` and calls `docker.Configure()` with the log tail, history size, alert rules and AWS options from the config; it then starts the AWS poller, which returns at once and does nothing unless AWS is enabled, and stops it on exit
5. If `tracing.otlp_receiver.enabled` is set, `main.go` starts the OTLP receiver on `docker.GetGlobalTraceStore()` (a bind failure is fatal) and stops it on exit
6. `main.go` creates the dashboard with `ui.NewDashboardWithOptions()` (theme, keymap and layout) and applies the initial filters with `Dashboard.ApplyInitialFilters()`
7. Goroutine ticker (`refresh_interval`, 2s by default) calls `docker.WatchContainers()`
8. Collector lists containers, fetches stats and logs for each
9. The ticker goroutine passes the parsed data to `Dashboard.Update()` directly (no channel)
10. Dashboard renders services table, resource panel, and logs view (hidden panels are not updated)
11. UI re-renders on each update cycle; keys go through `Dashboard.handleInput` and the keymap

## Use Cases

- Multi-service monitoring in docker-compose environments
- Performance debugging (CPU/memory spikes)
- Aggregated log troubleshooting across services
- Development environment health checks
- Any containerized application regardless of language

---
*Updated for GoWatch project - March 2026*
