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
│   ├── ui/               # Terminal UI: dashboard.go, components.go, theme.go (themes), layout.go (panel grid), texts.go (themed texts)
│   ├── config/           # YAML configuration: config.go (types, Default), load.go, validate.go, untrusted.go, alerts.go, defaultyaml.go
│   ├── trace/            # Distributed tracing (correlator.go, exporter.go)
│   └── aws/              # Future AWS integrations (cloudwatch.go, xray.go, lambda.go)
├── pkg/metrics/          # Metrics types (types.go)
├── docker-compose.yaml   # Docker development environment
└── makefile              # Build and development commands
```

## Key Components

- **Docker Collector** (`internal/docker/collector.go`): Connects to Docker daemon, watches containers, collects stats (CPU, memory), streams logs. Uses Moby client library for direct API access.
- **Stats Parser** (`internal/docker/parser.go`): Parses Docker cgroup statistics and multiplexed log streams.
- **Dashboard** (`internal/ui/dashboard.go`): Interactive terminal UI with services table, resource panel, and aggregated logs view using tcell/tview. `NewDashboardWithOptions(ui.Options{Theme, Layout})` builds it from a resolved theme and the layout config; `NewDashboard()` is the same with the built-in dark theme and default layout.
- **Themes** (`internal/ui/theme.go`): A `Theme` holds one color per semantic role (text, accent, status colors, help bar, search field, log level...) plus the service palette. The log level roles (`log_*`) have little visible effect yet because tview does not draw the level badges. `BuiltinThemes()` has `dark`, `light`, `solarized` and `nord`; `dark` reproduces the original colors exactly. `ResolveTheme()` applies the `theme.colors` overrides and reports an unknown theme, role or color. `Theme.Tag()` turns a color into a tview tag and `Theme.Apply()` sets tview's global styles; the dashboard reads every color from its theme and never from literals.
- **Layout** (`internal/ui/layout.go`): `planGrid()`/`buildGrid()` build the main grid from the layout config: services, resources, the 3-row search field, logs and the 1-row help bar. Heights are proportional weights; resources, logs and the help bar can be hidden and then leave the grid. The focus returns to the logs, or to the services table when the logs are hidden.
- **Configuration** (`internal/config/`): Loads the optional YAML config (`~/.gowatch/config.yaml`, then `./gowatch.yaml`, or a single `--config` file) on top of `Default()`, decodes strictly (unknown keys are errors) and validates it. The `theme` section is decoded here as raw data and checked by `ui.ResolveTheme`; the layout heights are checked by `Validate()`. With no file it returns exactly the built-in defaults, so GoWatch stays zero-config. It is a leaf package: it may import only the standard library, the YAML decoder, `internal/alert` and `pkg/metrics`, never `internal/docker`, `internal/ui` or `internal/filter`.
- **Tracing** (`internal/trace/`): Distributed tracing correlation and export (planned feature).
- **AWS Integration** (`internal/aws/`): Future CloudWatch, XRay, Lambda, CloudFormation monitoring.

## Tech Stack

- **Language**: Go 1.26+
- **Docker Client**: Moby (`github.com/moby/moby`) - direct Docker daemon API via Unix socket
- **Terminal UI**: tcell v2 (low-level) + tview (high-level widgets)
- **Observability**: OpenTelemetry (tracing infrastructure, planned)
- **Configuration**: YAML via `go.yaml.in/yaml/v3`
- **Build System**: makefile (install, build, run, test, docker-build, go-sec, field-fix)
- **Development**: Docker + Docker Compose for containerized dev environment

## Core Data Flow

1. `main.go` parses the flags (`--config`, `--theme`, `--print-default-config`) and calls `config.Load()`; a config error is printed to stderr and the process exits before the TUI opens
2. `main.go` resolves the theme with `ui.ResolveTheme()` (the `--theme` flag replaces `theme.name`); an unknown theme, role or color is a startup error too
3. `main.go` creates Docker client via `client.New(client.FromEnv)` and calls `docker.Configure()` with the log tail, history size, alert rules and AWS options from the config
4. If `tracing.otlp_receiver.enabled` is set, `main.go` starts the OTLP receiver on `docker.GetGlobalTraceStore()` (a bind failure is fatal) and stops it on exit
5. `main.go` creates the dashboard with `ui.NewDashboardWithOptions()` (theme and layout) and applies the initial filters with `Dashboard.ApplyInitialFilters()`
6. Goroutine ticker (`refresh_interval`, 2s by default) calls `docker.WatchContainers()`
7. Collector lists containers, fetches stats and logs for each
8. Parsed data sent to `Dashboard.Update()` via channel
9. Dashboard renders services table, resource panel, and logs view (hidden panels are not updated)
10. UI re-renders on each update cycle

## Use Cases

- Multi-service monitoring in docker-compose environments
- Performance debugging (CPU/memory spikes)
- Aggregated log troubleshooting across services
- Development environment health checks
- Any containerized application regardless of language

---
*Updated for GoWatch project - March 2026*
