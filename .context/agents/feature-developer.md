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

- **cmd/gowatch/main.go** - Entry point: parses flags, loads the config, resolves the theme, configures the collector, starts the optional OTLP receiver, creates Docker client and dashboard
- **internal/docker/collector.go** - Container watching, stats and logs collection
- **internal/docker/parser.go** - Docker stats and log stream parsing
- **internal/ui/dashboard.go** - Terminal dashboard with services table, resources, logs
- **internal/ui/theme.go** - `Theme` (one color per semantic role), the built-in themes, `ResolveTheme()`, `Theme.Tag()` and `Theme.Apply()`
- **internal/ui/layout.go** - `planGrid()`/`buildGrid()`: the main grid from the layout config, hidden panels and focus fallback
- **internal/ui/texts.go** - Themed texts: help bars, alerts modal title and the Docker-offline page
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

`main` runs in this order: flags -> `config.Load()` (exits with a message on stderr if the config is invalid) -> `ui.ResolveTheme()` (the `--theme` flag replaces `theme.name`; an unknown theme, role or color also exits with a message) -> Docker client -> `docker.Configure()` (log tail, history size, alert rules, AWS options) -> OTLP receiver if enabled -> `ui.NewDashboardWithOptions(ui.Options{Theme, Layout})` + `ApplyInitialFilters()` -> ticker with `refresh_interval`.

When adding a user-tunable option:

1. Add the field (with a `yaml` tag) to the structs in `internal/config/config.go`, its built-in value to `Default()`, its checks to `Validate()` (`validate.go`) and its documented line to `DefaultYAML()` (`defaultyaml.go`).
2. Pass it to the consuming package through an `Options` struct (see `docker.Options`, `aws.Options`); a zero value must mean "keep the built-in default".
3. Keep `internal/config` a leaf package: only the standard library, the YAML decoder, `internal/alert` and `pkg/metrics`. Never import `internal/docker`, `internal/ui` or `internal/filter` from it.
4. Add table-driven tests, update the README "Configuration" section and keep the no-config-file behavior byte-for-byte identical to the defaults.
5. `./gowatch.yaml` may come from an untrusted checkout. A setting that could redirect network traffic, or expose or start a listener, must not be honored from that layer: extend `Config.untrustedSettings` in `internal/config/untrusted.go`.
6. Beware of `make field-fix`: it reorders struct fields, so use keyed fields in struct literals (including test tables).

## UI Colors and Layout

- Never write a color in `internal/ui` as a literal (`tcell.ColorYellow`, `[yellow]` in a tag). Take it from the dashboard's `Theme` (`d.theme.Accent`, `d.theme.Tag(d.theme.StatusError)`), so every built-in theme and every `theme.colors` override reaches the screen.
- A new color means a new role: add the field to `Theme`, an entry to `colorRoles` (its name is what users write under `theme.colors`), a value in all four built-in themes, the role to the table in the README and to the role list in `DefaultYAML()`. The `dark` theme must keep reproducing the original colors, which `legacy_look_test.go` checks against literals copied from commit `0a4c422`.
- `Theme.Apply()` changes tview's global styles and must run before the primitives are created; `NewDashboardWithOptions` does that. Tests that build dashboards with other themes call `restoreStyles(t)` (or use `newTestDashboard`) and do not run in parallel.
- The main grid is built by `buildGrid()` from `planGrid()`. Hidden panels are not added to the grid and `Update()` skips them, so focus must never be set on a primitive that is not on screen: use `Dashboard.mainPanel()` (the logs, or the services table when the logs are hidden).
- To check what a change looks like, draw the dashboard on a `tcell.SimulationScreen` with the helpers in `internal/ui/render_test.go` and read the cells (characters and colors).
- tview reads a tag such as `[INFO]` or `[q]` as a color name, so those texts do not show up on screen. Escape literal brackets with `tview.Escape` in new code.
- Because of that, the log level badges are not drawn, and the roles `log_error`, `log_warn`, `log_info`, `log_debug` and `log_fatal_text` have no visible effect (`log_fatal` only colors the background of the rest of a FATAL line). Do not document them as coloring badges. Escaping the badges would change the default look and needs the owner's approval.
- The `dark` theme holds two marker values in `Subheading` and `ServicePalette` (`colorLegacyCyan`, `colorLegacyMagenta`) that are not real tcell colors. Use those roles only to build tags (`Tag`, `colorName`), never with `SetTextColor`; `TestLegacyMarkersAreOnlyUsedToBuildTags` checks it.

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
- `internal/ui/dashboard.go` - Add new UI panels or views (colors from the theme, panels placed by `layout.go`)
- `internal/ui/theme.go` - Add color roles and built-in themes
- `internal/config/` - Add configuration options (see "Configuration and Startup Flow"): types and `Default()` in `config.go`, layering in `load.go`, checks in `validate.go`, untrusted-layer rules in `untrusted.go`, alert rules in `alerts.go`, `DefaultYAML()` in `defaultyaml.go`
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
