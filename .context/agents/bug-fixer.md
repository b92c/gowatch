---
type: agent
name: Bug Fixer
description: Analyze bug reports and error messages
agentType: bug-fixer
phases: [E, V]
generated: 2026-03-22
status: filled
scaffoldVersion: "2.0.0"
---

# Bug Fixer Agent

## Role

This agent specializes in debugging and fixing bugs in GoWatch, a real-time Docker container monitoring tool written in Go.

## Project Context

GoWatch monitors Docker containers via the Moby client and displays metrics in a terminal dashboard using tview/tcell.

**Tech Stack**: Go 1.26+, Moby Docker client, tview/tcell terminal UI

## Codebase Structure

- **cmd/gowatch/main.go** - Entry point, goroutine setup
- **internal/docker/collector.go** - Container watching, stats collection
- **internal/docker/parser.go** - Docker stats and log parsing
- **internal/ui/dashboard.go** - Terminal dashboard rendering

## Common Bug Categories

### Docker Integration Issues
- **Connection failures**: Check Docker socket access (`/var/run/docker.sock`)
- **Stats parsing errors**: Verify `container.StatsResponse` structure handling
- **Log stream issues**: Docker multiplexed stream format (8-byte header)
- **Container list empty**: Verify `client.ContainerListOptions` parameters

### UI/Terminal Issues
- **Rendering glitches**: Check tview component updates in `dashboard.go`
- **Scroll position lost**: Review `userScrolling` state management
- **Color codes broken**: Verify tview color syntax (`[yellow]`, `[-]`)
- **Freeze/hang**: Look for blocking operations in UI goroutine

### Concurrency Issues
- **Race conditions**: Run `make test` (includes `-race` flag)
- **Mutex deadlocks**: Check `statsMutex` in `collector.go`
- **Goroutine leaks**: Verify ticker cleanup and context cancellation

### Resource Leaks
- **Response body not closed**: Check `defer stats.Body.Close()`
- **Client not closed**: Verify `apiClient.Close()` in main

## Debugging Workflow

1. Reproduce the issue
2. Check error messages and stack traces
3. Identify affected component (docker, ui, config)
4. Review relevant code paths
5. Fix following Go patterns
6. Write test to prevent regression
7. Run `make test` and `make go-sec`

## Key Functions to Check

- `docker.WatchContainers()` - Main container collection loop
- `docker.getContainerStats()` - Stats fetching
- `docker.ParseStats()` - CPU/memory calculation
- `docker.ParseLogs()` - Log stream parsing
- `Dashboard.Update()` - UI refresh
- `Dashboard.updateLogsView()` - Log rendering with scroll state

## Responsibilities

1. Analyze error messages and stack traces
2. Reproduce issues reliably
3. Locate root cause in codebase
4. Apply fixes following Go idioms
5. Write regression tests

## Relevant Files

- `internal/docker/collector.go` - Container monitoring
- `internal/docker/parser.go` - Stats/log parsing
- `internal/ui/dashboard.go` - UI rendering
- `cmd/gowatch/main.go` - Application setup

---
*Updated for GoWatch project - March 2026*
