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
│   ├── config/           # Configuration management (config.go)
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
- **Configuration** (`internal/config/`): Configuration management (currently minimal, zero-config by design).
- **Tracing** (`internal/trace/`): Distributed tracing correlation and export (planned feature).
- **AWS Integration** (`internal/aws/`): Future CloudWatch, XRay, Lambda, CloudFormation monitoring.

## Tech Stack

- **Language**: Go 1.26+
- **Docker Client**: Moby (`github.com/moby/moby`) - direct Docker daemon API via Unix socket
- **Terminal UI**: tcell v2 (low-level) + tview (high-level widgets)
- **Observability**: OpenTelemetry (tracing infrastructure, planned)
- **Build System**: makefile (install, build, run, test, docker-build, go-sec, field-fix)
- **Development**: Docker + Docker Compose for containerized dev environment

## Core Data Flow

1. `main.go` creates Docker client via `client.New(client.FromEnv)`
2. Goroutine ticker (2s interval) calls `docker.WatchContainers()`
3. Collector lists containers, fetches stats and logs for each
4. Parsed data sent to `Dashboard.Update()` via channel
5. Dashboard renders services table, resource panel, and logs view
6. UI re-renders on each update cycle

## Use Cases

- Multi-service monitoring in docker-compose environments
- Performance debugging (CPU/memory spikes)
- Aggregated log troubleshooting across services
- Development environment health checks
- Any containerized application regardless of language

---
*Updated for GoWatch project - March 2026*
