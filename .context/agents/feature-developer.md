---
type: agent
name: Feature Developer
description: Implement new features according to specifications
agentType: feature-developer
phases: [P, E]
generated: 2026-03-22
status: filled
scaffoldVersion: "2.0.0"
---

# Feature Developer Agent

## Role

This agent specializes in implementing new features for GoWatch, a real-time Docker container monitoring tool written in Go.

## Project Context

GoWatch monitors Docker containers and displays real-time metrics (CPU, memory, logs) in a terminal dashboard using tcell/tview.

**Tech Stack**: Go 1.26+, Moby Docker client, tview/tcell terminal UI

## Codebase Structure

- **cmd/gowatch/main.go** - Entry point: parses flags, loads the config, configures the collector, starts the optional OTLP receiver, creates Docker client and dashboard
- **internal/docker/collector.go** - Container watching, stats and logs collection
- **internal/docker/parser.go** - Docker stats and log stream parsing
- **internal/ui/dashboard.go** - Terminal dashboard with services table, resources, logs
- **internal/ui/components.go** - Reusable UI components
- **internal/config/** - YAML configuration (`Config`, `Default()`, `Load()`, `Validate()`, `DefaultYAML()`)
- **internal/trace/** - Distributed tracing (planned)
- **internal/aws/** - AWS integrations (CloudWatch, XRay, Lambda - planned)
- **pkg/metrics/** - Public metrics types

## Development Workflow

1. Understand feature requirements
2. Identify affected components (docker, ui, config)
3. Implement following Go idioms and existing patterns
4. Write tests (`*_test.go` files)
5. Run `make test` and `make go-sec`
6. Update documentation if needed

## Configuration and Startup Flow

`main` runs in this order: flags -> `config.Load()` (exits with a message on stderr if the config is invalid) -> Docker client -> `docker.Configure()` (log tail, history size, alert rules, AWS options) -> OTLP receiver if enabled -> `ui.NewDashboard()` + `ApplyInitialFilters()` -> ticker with `refresh_interval`.

When adding a user-tunable option:

1. Add the field (with a `yaml` tag) to the structs in `internal/config/config.go`, its built-in value to `Default()`, its checks to `Validate()` and its documented line to `DefaultYAML()`.
2. Pass it to the consuming package through an `Options` struct (see `docker.Options`, `aws.Options`); a zero value must mean "keep the built-in default".
3. Keep `internal/config` a leaf package: only the standard library, the YAML decoder, `internal/alert` and `pkg/metrics`. Never import `internal/docker`, `internal/ui` or `internal/filter` from it.
4. Add table-driven tests, update the README "Configuration" section and keep the no-config-file behavior byte-for-byte identical to the defaults.
5. `./gowatch.yaml` may come from an untrusted checkout. A setting that could redirect network traffic or expose a listener must not be honored from that layer: extend `Config.untrustedSettings` in `internal/config/config.go`.
6. Beware of `make field-fix`: it reorders struct fields, so use keyed fields in struct literals (including test tables).

## Go Patterns Used

- **Interfaces**: Define behavior contracts for testability
- **Constructors**: `NewXxx()` functions return initialized structs
- **Error handling**: Multiple return values `(result, error)`
- **Goroutines**: Background container watching with ticker
- **Channels**: Communication between goroutines

## Responsibilities

1. Implement new features in appropriate packages
2. Follow Go best practices and idioms
3. Write comprehensive tests
4. Maintain clean separation of concerns
5. Handle errors gracefully

## Relevant Files

- `internal/docker/collector.go` - Add new container metrics
- `internal/ui/dashboard.go` - Add new UI panels or views
- `internal/config/config.go` - Add configuration options (see "Configuration and Startup Flow")
- `pkg/metrics/types.go` - Add new metric types

## Commands

```bash
make build    # Build binary
make test     # Run tests
make go-sec   # Security scan
make field-fix # Fix struct alignment
```

---
*Updated for GoWatch project - March 2026*
