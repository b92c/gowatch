---
type: doc
name: development-workflow
description: Day-to-day engineering processes, branching, and contribution guidelines
category: workflow
generated: 2026-03-22
status: filled
scaffoldVersion: "2.0.0"
---

# Development Workflow

## Prerequisites

- Go 1.26+ installed
- Docker and Docker Compose
- Access to Docker daemon socket (`/var/run/docker.sock`)

## Quick Start

```bash
# Clone and setup
git clone https://github.com/b92c/gowatch.git
cd gowatch
make install      # Install dependencies and git hooks

# Build and run
make build        # Build binary to bin/gowatch
make run          # Run the application

# Or use Docker
make docker-build # Build Docker image
docker compose up # Run in container with Docker socket access
```

## Branching Strategy

- `main` - Production-ready code
- `fix/*` - Bug fixes
- `feat/*` - New features
- `chore/*` - Maintenance tasks

## Development Process

1. Create a branch from `main`
2. Implement changes following Go conventions
3. Run tests: `make test`
4. Run security scan: `make go-sec`
5. Fix struct alignment: `make field-fix`
6. Create PR for review

## Key Make Commands

```bash
make install         # Install dependencies (gosec, fieldalignment) and git hooks
make build           # Build binary to bin/gowatch
make run             # Run the application
make test            # Run tests with coverage
make test-coverage   # Run tests and open HTML coverage report
make docker-build    # Build Docker development image
make go-sec          # Security vulnerability scan with gosec
make field-fix       # Fix struct field alignment for memory optimization
make clean           # Remove build artifacts
```

## Docker Development Environment

The project includes a complete Docker setup:

```yaml
# docker-compose.yaml mounts Docker socket for container access
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

This allows GoWatch running in a container to monitor other containers on the host.

## Code Organization

- **cmd/gowatch/**: Application entry point only - keep minimal
- **internal/docker/**: Docker interaction logic (not exported)
- **internal/ui/**: Terminal UI components (not exported)
- **internal/config/**: Configuration (not exported)
- **internal/trace/**: Tracing functionality (not exported)
- **internal/aws/**: AWS integrations (not exported)
- **pkg/metrics/**: Public metrics types (can be imported by external packages)

## Git Hooks

Pre-commit hooks are installed via `make install-hooks`:
- Located in `.setup/build/config/pre-commit`
- Runs security and quality checks before commit

---
*Updated for GoWatch project - March 2026*
