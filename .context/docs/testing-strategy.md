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
- **Keybindings**: `ParseKey()`, the presets, `BuildKeymap()` (every error, aggregated and in a fixed order), the texts built from the keymap, the help page and `handleInput` with remapped keys; `input_oracle_test.go` proves that the default keymap behaves like the original `handleInput` (see "Keyboard Tests")

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
│   ├── keymap.go
│   ├── keymap_test.go       # ParseKey, presets, BuildKeymap and its errors
│   ├── input.go
│   ├── input_test.go        # handleInput with remapped keys, scrolling, the help page
│   ├── input_oracle_test.go # The default keymap equals the original handleInput
│   ├── help.go
│   ├── help_test.go         # The help page content and colors
│   ├── texts_test.go        # Help bar, alerts title and error bar built from the keymap
│   ├── live_test.go         # startLive(): the real application on a simulated screen
│   ├── interaction_test.go  # Keyboard flows on the running application
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
├── main_test.go             # Flags and startup errors (paths that return before the TUI)
└── keybindings_test.go      # Keybinding errors at startup, layering, the printed example
pkg/metrics/
└── metrics_test.go
```

## UI Rendering Tests

The TUI cannot be driven by a test, but it can be drawn. `internal/ui/render_test.go` draws a primitive (or the whole dashboard) on a `tcell.NewSimulationScreen("")` without starting the tview event loop, and the tests read the characters and colors of the cells. Rules:

- `Theme.Apply()` changes tview's global `Styles`. Build dashboards with `newTestDashboard` (or call `restoreStyles(t)`), so the styles are restored, and do not use `t.Parallel()` in `internal/ui`.
- The logs view calls `app.Draw()` when it changes, which blocks when no application is running. `newTestDashboard` removes that callback.
- Look panels up by their border titles (`screenshot.titleRow`) instead of plain words, which also appear in table headers and the help bar.

## Keyboard Tests

- **Oracle** (`input_oracle_test.go`): `legacyHandleInput` is a verbatim copy of `handleInput` from commit `155d4ff`, the last version with the keys written in the code. The test sends the same events to it and to the real `handleInput` (default keymap) in every state (main panel, alerts modal, search field focused, daemon error page; two layouts; both views) and compares what each returns and the state it leaves: the view, the modal, the filters, the focus, the front page, the scroll position, the resolved alerts and whether the application quit. The only difference allowed is `?`, which opens the help page and is tested on its own. Do not edit the copy or loosen the comparison.
- **Running application** (`live_test.go`, `interaction_test.go`): `startLive()` runs the real `tview.Application` on a `tcell.SimulationScreen`, injects keys (`InjectKey`), waits until the input capture has handled each one and reads the screen cells. Use `liveDashboard.do()` to read or change the dashboard from the application goroutine, and `update()` to give it containers as a collector tick would. Do not call `do()` or `press()` after the dashboard quit: use `pressAndQuit()` and wait for the exit.
- **Unit level**: `handleInput` is called directly on a dashboard that has a simulated screen attached (`newKeyedDashboard`), so a quit shows as the screen being finalized (its size becomes 0x0).
- Without a drawn frame a grid does not report the focus of its panels (`Grid.HasFocus` counts only the items it drew), so tests that deliver keys through the root draw first.

## Quality Gates

1. All tests must pass (`make test`)
2. No race conditions (`-race` flag enabled)
3. Security scan clean (`make go-sec`)
4. Struct alignment optimized (`make field-fix`)

---
*Updated for GoWatch project - March 2026*
