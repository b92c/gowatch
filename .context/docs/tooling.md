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

- **Go 1.27+** - Primary language
- **Docker** - Container runtime for testing and development
- **Docker Compose** - Multi-container orchestration
- **make** - Build automation

## Go Tools (installed via `make install`)

`make install` runs `make install-tools`, which installs these at the versions pinned in the `makefile` (the same ones the CI uses, see [CI/CD](#cicd)):

- **gosec** - Security vulnerability scanner (`go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)`)
- **fieldalignment** - Struct memory optimization (`go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@$(FIELDALIGNMENT_VERSION)`)

## Dependencies

Core libraries from `go.mod`:
- **github.com/moby/moby/client** - Docker daemon API client
- **github.com/moby/moby/api** - Docker API types
- **github.com/rivo/tview** - Terminal UI widgets
- **github.com/gdamore/tcell/v2** - Low-level terminal handling
- **go.opentelemetry.io/otel** - Observability framework (planned)

## Make Commands

```bash
make install         # Install gosec and fieldalignment (pinned versions) and git hooks
make build           # Compile to bin/gowatch
make run             # Run the binary
make test            # Run tests with race detection and coverage
make test-coverage   # Run tests and open HTML coverage report
make docker-build    # Build Docker image
make go-sec          # Security scan with gosec
make field-fix       # Optimize struct field alignment
make clean           # Remove build artifacts
make install-tools   # Install gosec and fieldalignment at the versions pinned in the makefile
make check           # Lint (same steps as the CI) and tests
make lint            # fmt-check, vet, mod-check, field-check and go-sec
make vuln            # govulncheck ./... (install it with make install-govulncheck)
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

The image is built from `.setup/build/Dockerfile` on the stable `golang:1.27-alpine3.23` tag and an `alpine:3.23` runtime, both from the `public.ecr.aws/docker/library` mirror. The build runs `go mod download && go mod verify` and never rewrites `go.mod`. `.dockerignore` keeps `.git`, `bin/`, `coverage.out`, `.superpowers/`, `.vscode/` and `.env` out of the build context. When `go.mod` moves to a new Go release, move the `golang` tag with it.

## CI/CD

How the workflows behave (jobs, release, Dependabot, branch protection) is described in [Development Workflow](./development-workflow.md#cicd). This section covers the pinned versions and how to update them.

### Pinned tool versions

The versions are variables at the top of the "CI CHECKS" block of the `makefile`. The workflows install the tools through `make install-tools` (gosec, fieldalignment) and `make install-govulncheck`, and `make install` calls `make install-tools` too, so the makefile is the only place to change.

| Tool | Variable | Version | Module |
| --- | --- | --- | --- |
| gosec | `GOSEC_VERSION` | v2.30.0 | `github.com/securego/gosec/v2/cmd/gosec` |
| fieldalignment | `FIELDALIGNMENT_VERSION` | v0.52.0 | `golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment` |
| govulncheck | `GOVULNCHECK_VERSION` | v1.9.0 | `golang.org/x/vuln/cmd/govulncheck` |

To update one:

```bash
go list -m -versions golang.org/x/vuln            # list the available versions
GOBIN=$(mktemp -d) go install golang.org/x/vuln/cmd/govulncheck@<new version>   # it must build with Go 1.27
# edit the variable in the makefile, then:
make install-tools install-govulncheck
make check
```

Dependabot does not track these versions, so check them now and then (for example when a tool starts missing new Go language features). To refresh the local copies after a bump, run `make install-tools` again.

### Pinned GitHub Actions

Every action in `.github/workflows/` is pinned by the full commit SHA, with the release tag in a trailing comment (`uses: actions/checkout@<sha> # v7.0.1`). Dependabot opens the pull requests that move both. To pin one by hand:

```bash
gh api repos/<owner>/<repo>/git/ref/tags/<tag> --jq '.object'     # type "commit": the sha is the commit
gh api repos/<owner>/<repo>/git/tags/<sha> --jq '.object.sha'     # type "tag" (annotated): resolve to the commit
gh api repos/<owner>/<repo>/commits/<sha> --jq '.sha'             # confirm the commit exists
```

Check the inputs you pass against the `action.yml` of the pinned version, and lint the workflows with `actionlint` (install it with `go install github.com/rhysd/actionlint/cmd/actionlint@<version>`).

---
*Updated for GoWatch project - March 2026*
