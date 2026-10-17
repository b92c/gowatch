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
│   ├── ui/               # Terminal UI dashboard (dashboard.go, components.go)
│   ├── config/           # YAML configuration: loading, layering, validation (config.go)
│   ├── trace/            # Distributed tracing (correlator.go, exporter.go)
│   └── aws/              # Future AWS integrations (cloudwatch.go, xray.go, lambda.go)
├── pkg/metrics/          # Metrics types (types.go)
├── docker-compose.yaml   # Docker development environment
└── makefile              # Build and development commands
```

## Key Components

- **Docker Collector** (`internal/docker/collector.go`): Connects to Docker daemon, watches containers, collects stats (CPU, memory), streams logs. Uses Moby client library for direct API access.
- **Stats Parser** (`internal/docker/parser.go`): Parses Docker cgroup statistics and multiplexed log streams.
- **Dashboard** (`internal/ui/dashboard.go`): Interactive terminal UI with services table, resource panel, and aggregated logs view using tcell/tview.
- **Configuration** (`internal/config/`): Loads the optional YAML config (`~/.gowatch/config.yaml`, then `./gowatch.yaml`, or a single `--config` file) on top of `Default()`, decodes strictly (unknown keys are errors) and validates it. With no file it returns exactly the built-in defaults, so GoWatch stays zero-config. It is a leaf package: it may import only the standard library, the YAML decoder, `internal/alert` and `pkg/metrics`, never `internal/docker`, `internal/ui` or `internal/filter`.
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

1. `main.go` parses the flags (`--config`, `--print-default-config`) and calls `config.Load()`; a config error is printed to stderr and the process exits before the TUI opens
2. `main.go` creates Docker client via `client.New(client.FromEnv)` and calls `docker.Configure()` with the log tail, history size, alert rules and AWS options from the config
3. If `tracing.otlp_receiver.enabled` is set, `main.go` starts the OTLP receiver on `docker.GetGlobalTraceStore()` (a bind failure is fatal) and stops it on exit
4. `main.go` creates the dashboard and applies the initial filters with `Dashboard.ApplyInitialFilters()`
5. Goroutine ticker (`refresh_interval`, 2s by default) calls `docker.WatchContainers()`
6. Collector lists containers, fetches stats and logs for each
7. Parsed data sent to `Dashboard.Update()` via channel
8. Dashboard renders services table, resource panel, and logs view
9. UI re-renders on each update cycle

## Use Cases

- Multi-service monitoring in docker-compose environments
- Performance debugging (CPU/memory spikes)
- Aggregated log troubleshooting across services
- Development environment health checks
- Any containerized application regardless of language

---
*Updated for GoWatch project - March 2026*
