---
type: doc
name: tooling
description: Scripts, IDE settings, automation, and developer productivity tips
category: tooling
generated: 2026-03-22
status: filled
scaffoldVersion: "2.0.0"
---

# Tooling

## Required Tools

- **Go 1.25+** - Primary language
- **Docker** - Container runtime for testing and development
- **Docker Compose** - Multi-container orchestration
- **make** - Build automation

## Go Tools (installed via `make install`)

- **gosec** - Security vulnerability scanner (`go install github.com/securego/gosec/v2/cmd/gosec@latest`)
- **fieldalignment** - Struct memory optimization (`go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest`)

## Dependencies

Core libraries from `go.mod`:
- **github.com/moby/moby/client** - Docker daemon API client
- **github.com/moby/moby/api** - Docker API types
- **github.com/rivo/tview** - Terminal UI widgets
- **github.com/gdamore/tcell/v2** - Low-level terminal handling
- **go.opentelemetry.io/otel** - Observability framework (planned)

## Make Commands

```bash
make install         # Install gosec, fieldalignment, and git hooks
make build           # Compile to bin/gowatch
make run             # Run the binary
make test            # Run tests with race detection and coverage
make test-coverage   # Run tests and open HTML coverage report
make docker-build    # Build Docker image
make go-sec          # Security scan with gosec
make field-fix       # Optimize struct field alignment
make clean           # Remove build artifacts
```

## IDE Configuration

Recommended VS Code extensions:
- **Go** (official Go team extension)
- **gopls** (Go language server, auto-installed)
- **Error Lens** (inline error display)

Recommended settings (`.vscode/settings.json`):
```json
{
  "go.useLanguageServer": true,
  "go.lintTool": "golangci-lint",
  "go.formatTool": "gofmt",
  "editor.formatOnSave": true,
  "[go]": {
    "editor.codeActionsOnSave": {
      "source.organizeImports": true
    }
  }
}
```

## Git Hooks

Pre-commit hooks installed via `make install-hooks`:
- Located in `.setup/build/config/pre-commit`
- Runs quality checks before each commit

## Docker Development

```bash
# Build and run in Docker with socket access
make docker-build
docker compose up

# The container mounts /var/run/docker.sock for container monitoring
```

---
*Updated for GoWatch project - March 2026*
