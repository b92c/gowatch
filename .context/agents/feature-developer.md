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

- **cmd/gowatch/main.go** - Entry point, creates Docker client and dashboard
- **internal/docker/collector.go** - Container watching, stats and logs collection
- **internal/docker/parser.go** - Docker stats and log stream parsing
- **internal/ui/dashboard.go** - Terminal dashboard with services table, resources, logs
- **internal/ui/components.go** - Reusable UI components
- **internal/config/** - Configuration management
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
- `internal/config/config.go` - Add configuration options
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
