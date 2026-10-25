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

- Go 1.27+ installed
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
6. Run the same checks as the CI before pushing: `make check` (see [CI/CD](#cicd))
7. Create PR for review

## Key Make Commands

```bash
make install         # Install dependencies (gosec and fieldalignment, pinned versions) and git hooks
make build           # Build binary to bin/gowatch
make run             # Run the application
make test            # Run tests with coverage
make test-coverage   # Run tests and open HTML coverage report
make docker-build    # Build Docker development image
make go-sec          # Security vulnerability scan with gosec
make field-fix       # Fix struct field alignment for memory optimization
make clean           # Remove build artifacts
make install-tools   # Install gosec and fieldalignment at the versions the CI uses
make check           # Same lint steps as the CI, then the tests
make lint            # Only the lint steps (fmt-check, vet, mod-check, field-check, go-sec)
make fmt-check       # Fail if gofmt would change any file
make vet             # go vet ./...
make mod-check       # Fail if go.mod/go.sum are not tidy (go mod tidy -diff)
make field-check     # Fail if fieldalignment reports anything
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

## CI/CD

The pipeline lives in `.github/workflows/`. Tool versions and how to update them are in [Tooling](./tooling.md#cicd).

### CI (`build.yml`)

Runs on every push to `main`, on every pull request targeting `main`, and when the Release workflow calls it (`workflow_call`). A new push to the same pull request cancels the older run. Outside pull requests every run has its own concurrency group, so runs on `main` and runs started by a release are never cancelled and never replace a run that is waiting. The workflow only has `contents: read`.

| Job | What it does |
| --- | --- |
| `Lint` | `go mod verify`, `gofmt -l .` (must print nothing), `go vet ./...`, `go mod tidy -diff` (go.mod/go.sum must be tidy), `fieldalignment ./...` (must print nothing) and `gosec ./...` |
| `Test (ubuntu-latest)`, `Test (macos-latest)` | `make test-command` (race detector and coverage), then `make build`. Only the Ubuntu run uploads `coverage.out` to Codecov, and a missing token (pull requests from forks) never fails the job |
| `Cross build` | Compiles the same six targets as the release (linux, darwin and windows, each for amd64 and arm64) with `CGO_ENABLED=0`, `-trimpath` and `-ldflags="-s -w"`. Nothing is uploaded; it only makes a build break fail the pull request instead of the tag |
| `Docker build` | Builds `.setup/build/Dockerfile` with Buildx and the GitHub Actions cache. Nothing is pushed |

Windows is only covered by the cross build: the tests do not run there.

### Run the CI checks locally

```bash
make install-tools   # once: gosec and fieldalignment at the pinned versions
make check           # lint (same steps as the Lint job) and the tests
make lint            # only the lint steps
```

The `Lint` job runs the same make targets one by one (`fmt-check`, `vet`, `mod-check`, `field-check`, `go-sec`), so a failure there reproduces with the target of the same name. The tools are installed in `$(go env GOPATH)/bin`, which must be on your `PATH`. They have to be built with the Go version of the module (1.27): a tool built with an older Go cannot analyze it, so reinstall with `make install-tools` if you see type-checking errors.

### Release (`release.yml`)

Pushing a tag that starts with `v` runs the release:

1. `Validate tag` fails early unless the tag matches `^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$` (for example `v1.4.0` or `v1.4.0-rc.1`).
2. `Verify` calls the CI workflow, so nothing is published unless lint, tests, cross build and Docker build pass.
3. `release` builds the six binaries (`CGO_ENABLED=0`, `-trimpath`, `-ldflags="-s -w"`) and uploads each one as a workflow artifact.
4. `publish` is the only job with `contents: write`. It checks that all six assets exist, writes `CHECKSUMS.txt`, runs `sha256sum -c CHECKSUMS.txt` and creates the GitHub Release with generated notes.

A tag with a suffix after the version (anything with a `-`) is published as a pre-release, so it is not "latest" and the installer ignores it.

The installer (`.installer/install.sh`) depends on the asset names `gowatch_<os>_<arch>` (with `.exe` on Windows) and on `CHECKSUMS.txt` with one `<sha256>  <asset name>` line per asset. Do not change either without changing the installer. The binary does not embed its version yet, so the tag is the only place the version exists.

### Security workflow (`security.yml`)

Runs `govulncheck ./...` every Monday at 06:17 UTC, on demand (`workflow_dispatch`) and on pull requests that change `go.mod` or `go.sum`. It is a separate workflow on purpose: a vulnerability published later for the standard library or a dependency would otherwise fail unrelated pull requests.

### Dependabot (`.github/dependabot.yml`)

Weekly updates (Mondays) for `gomod` (`/`), `github-actions` (`/`) and `docker` (`/.setup/build`). Minor and patch updates of each ecosystem arrive as one grouped pull request; major updates arrive one by one. Commits use the `chore(deps)` prefix (`ci(deps)` for actions), with at most 5 open pull requests for Go modules and actions and 3 for Docker.

### Branch protection (suggestion)

Repository settings are not managed by files in this repository. Enabling branch protection on `main` that requires the CI checks `Lint`, `Test (ubuntu-latest)`, `Test (macos-latest)`, `Cross build` and `Docker build` to pass before merging would make the pipeline enforce what it checks.

---
*Updated for GoWatch project - March 2026*
