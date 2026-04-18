---
type: agent
name: Refactoring Specialist
description: Identify code smells and improvement opportunities
agentType: refactoring-specialist
phases: [E]
generated: 2026-03-22
status: filled
scaffoldVersion: "2.0.0"
---

# Refactoring Specialist Agent

## Role

This agent specializes in refactoring Go code for GoWatch, a real-time Docker container monitoring tool.

## Project Context

GoWatch monitors Docker containers via the Moby client and displays metrics in a terminal dashboard. Refactoring should focus on Go idioms, interface design, memory efficiency, and clean architecture.

**Tech Stack**: Go 1.26+, Moby Docker client, tview/tcell terminal UI

## Codebase Structure

- **cmd/gowatch/** - Entry point (minimal, delegates to packages)
- **internal/docker/** - Docker interaction (collector.go, parser.go)
- **internal/ui/** - Terminal dashboard (dashboard.go, components.go)
- **internal/config/** - Configuration management
- **pkg/metrics/** - Public metric types

## Refactoring Opportunities

### Interface Extraction
```go
// Extract interface for Docker client testability
type ContainerWatcher interface {
    ContainerList(ctx context.Context, options client.ContainerListOptions) ([]types.Container, error)
    ContainerStats(ctx context.Context, containerID string, options client.ContainerStatsOptions) (types.ContainerStats, error)
    ContainerLogs(ctx context.Context, container string, options client.ContainerLogsOptions) (io.ReadCloser, error)
}
```

### Struct Field Alignment
Run `make field-fix` to optimize memory layout:
```go
// Before (unaligned)
type Container struct {
    ID         string   // 16 bytes
    MemUsage   uint64   // 8 bytes
    CPUPercent float64  // 8 bytes
    Log        []string // 24 bytes
}

// After (aligned by fieldalignment)
type Container struct {
    Log        []string // 24 bytes
    ID         string   // 16 bytes
    MemUsage   uint64   // 8 bytes
    CPUPercent float64  // 8 bytes
}
```

### Error Handling Consolidation
```go
// Extract common error handling
func handleDockerError(err error, containerID string) {
    if err != nil {
        log.Printf("Docker error for %s: %v", containerID, err)
    }
}
```

### Dependency Injection
```go
// Constructor with dependencies
func NewCollector(client ContainerWatcher, config *Config) *Collector {
    return &Collector{client: client, config: config}
}
```

## Code Smells to Address

1. **Large functions**: Break down `WatchContainers()` if complexity grows
2. **Global state**: `previousStats` map could be encapsulated in struct
3. **Magic numbers**: Extract constants (e.g., ticker interval, log tail count)
4. **Duplicate code**: Extract common patterns to helper functions
5. **Missing interfaces**: Add interfaces where testability is needed

## Refactoring Workflow

1. Identify code smell or improvement opportunity
2. Write tests for existing behavior (if missing)
3. Apply refactoring in small, safe steps
4. Run `make test` after each change
5. Run `make go-sec` for security check
6. Run `make field-fix` for struct optimization
7. Update documentation if interfaces change

## Responsibilities

1. Identify code smells and anti-patterns
2. Propose and implement refactoring strategies
3. Extract interfaces for testability
4. Maintain backward compatibility
5. Ensure tests pass after changes
6. Optimize struct memory layout

## Relevant Files

- `internal/docker/collector.go` - Main refactoring target
- `internal/docker/parser.go` - Parsing logic
- `internal/ui/dashboard.go` - UI component structure
- `pkg/metrics/types.go` - Type definitions

## Commands

```bash
make test      # Verify behavior preserved
make go-sec    # Security scan
make field-fix # Optimize struct alignment
```

---
*Updated for GoWatch project - March 2026*
