---
type: doc
name: testing-strategy
description: Test frameworks, patterns, coverage requirements, and quality gates
category: testing
generated: 2026-03-22
status: filled
scaffoldVersion: "2.0.0"
---

# Testing Strategy

## Framework

- **Go testing package** - Built-in test framework (`testing`)
- Test files: `*_test.go` colocated with source files
- Race detection enabled via `-race` flag

## Running Tests

```bash
make test            # Run all tests with race detection and coverage
make test-coverage   # Run tests and open HTML coverage report
make test-command    # Run raw test command with verbose output
```

The test command runs:
```bash
go test -race -coverpkg=./... -v -coverprofile=coverage.out ./...
```

## Test Patterns

### Unit Tests
- **Docker Collector**: Mock Docker client interface, test container listing and stats parsing
- **Stats Parser**: Test CPU/memory calculation with known input values
- **Log Parser**: Test Docker multiplexed stream parsing
- **UI Components**: Test data transformation, and check what is drawn by rendering the dashboard on a `tcell.SimulationScreen` and reading the cells (see "UI Rendering Tests")
- **Config**: Table-driven tests for layering, validation and `DefaultYAML()`; the temporary config files live in `t.TempDir()`
- **Themes and layout**: Tests for the theme roles, `ResolveTheme()`, `Theme.Tag()`, `planGrid()`/`buildGrid()`; `legacy_look_test.go` compares the dark theme with literals copied from the last version that had the colors in the code

### Integration Tests
- Require running Docker daemon
- Use real containers in isolated test environment
- Test full flow: container discovery → stats collection → UI update

### Mocking Strategies
- Create interfaces for Docker client operations
- Use table-driven tests for parser functions
- Mock `container.StatsResponse` for stats calculations

## Coverage Requirements

Focus coverage on:
- **internal/docker/** - Collector and parser logic (~60%+ coverage target)
- **pkg/metrics/** - Type definitions and any utility functions
- **internal/ui/** - Data transformation functions, themes, layout and the rendered dashboard
- **internal/config/** and **cmd/gowatch/** - Loading, validation and startup errors

## Test File Locations

```
internal/
├── docker/
│   ├── collector.go
│   ├── collector_test.go    # Docker collector tests
│   ├── parser.go
│   └── parser_test.go       # Stats/log parsing tests
├── ui/
│   ├── dashboard.go
│   ├── dashboard_test.go    # Input handling, initial filters, constructors
│   ├── components_test.go   # Sparkline rendering
│   ├── theme.go
│   ├── theme_test.go        # Theme roles, ResolveTheme, Tag, Apply
│   ├── theme_render_test.go # Every theme drawn on a simulated screen
│   ├── layout.go
│   ├── layout_test.go       # Grid plan, hidden panels, focus, keyboard flows
│   ├── legacy_look_test.go  # The dark theme equals the original look
│   └── render_test.go       # Simulated-screen helpers and sample data
├── config/
│   ├── config.go            # Types and Default()
│   ├── load.go              # Layering and strict decoding
│   ├── validate.go          # Validation
│   ├── untrusted.go         # Rules for ./gowatch.yaml
│   ├── alerts.go            # Alert rule overrides
│   ├── defaultyaml.go       # --print-default-config
│   └── *_test.go            # One test file per source file, plus helpers_test.go
├── alert/, aws/, filter/, trace/    # Engine, AWS, filter and tracing tests
cmd/gowatch/
└── main_test.go             # Flags and startup errors (paths that return before the TUI)
pkg/metrics/
└── metrics_test.go
```

## UI Rendering Tests

The TUI cannot be driven by a test, but it can be drawn. `internal/ui/render_test.go` draws a primitive (or the whole dashboard) on a `tcell.NewSimulationScreen("")` without starting the tview event loop, and the tests read the characters and colors of the cells. Rules:

- `Theme.Apply()` changes tview's global `Styles`. Build dashboards with `newTestDashboard` (or call `restoreStyles(t)`), so the styles are restored, and do not use `t.Parallel()` in `internal/ui`.
- The logs view calls `app.Draw()` when it changes, which blocks when no application is running. `newTestDashboard` removes that callback.
- Look panels up by their border titles (`screenshot.titleRow`) instead of plain words, which also appear in table headers and the help bar.

## Quality Gates

1. All tests must pass (`make test`)
2. No race conditions (`-race` flag enabled)
3. Security scan clean (`make go-sec`)
4. Struct alignment optimized (`make field-fix`)

---
*Updated for GoWatch project - March 2026*
