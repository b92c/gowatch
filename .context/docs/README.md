# Documentation Index

Welcome to the GoWatch repository knowledge base. Start with the project overview, then dive into specific guides as needed.

## Core Guides
- [Project Overview](./project-overview.md) - Architecture, components, and tech stack
- [Development Workflow](./development-workflow.md) - Branching, make commands, Docker setup
- [Testing Strategy](./testing-strategy.md) - Go testing patterns and coverage
- [Tooling & Productivity Guide](./tooling.md) - Go tools, IDE config, dependencies

## Repository Snapshot
- `cmd/gowatch/` — Application entry point (main.go)
- `internal/docker/` — Docker daemon interaction, container monitoring
- `internal/ui/` — Terminal dashboard using tcell/tview
- `internal/config/` — Configuration management
- `internal/trace/` — Distributed tracing (planned)
- `internal/aws/` — AWS integrations (CloudWatch, XRay, Lambda - planned)
- `pkg/metrics/` — Public metrics types
- `docker-compose.yaml` — Docker development environment
- `makefile` — Build and development commands
- `AGENTS.md` — AI context references

## Document Map
| Guide | File | Primary Inputs |
| --- | --- | --- |
| Project Overview | `project-overview.md` | README, go.mod, codebase structure |
| Development Workflow | `development-workflow.md` | makefile, Docker setup, branching rules |
| Testing Strategy | `testing-strategy.md` | Go test patterns, coverage commands |
| Tooling & Productivity Guide | `tooling.md` | Go tools, IDE configs, dependencies |

## Quick Reference

### Key Commands
```bash
make install      # Setup dev environment
make build        # Build binary
make run          # Run gowatch
make test         # Run tests with coverage
make docker-build # Build Docker image
```

### Tech Stack
- **Language**: Go 1.25+
- **Docker Client**: Moby (github.com/moby/moby)
- **Terminal UI**: tview + tcell
- **Observability**: OpenTelemetry (planned)
