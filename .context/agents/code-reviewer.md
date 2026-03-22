---
type: agent
name: Code Reviewer
description: Review code changes for quality, style, and best practices
agentType: code-reviewer
phases: [R, V]
generated: 2026-03-22
status: filled
scaffoldVersion: "2.0.0"
---

# Code Reviewer Agent

## Role

This agent specializes in reviewing Go code for GoWatch, a real-time Docker container monitoring tool.

## Project Context

GoWatch monitors Docker containers and displays real-time metrics in a terminal dashboard. Code reviews should focus on Go idioms, Docker integration correctness, and UI performance.

**Tech Stack**: Go 1.25+, Moby Docker client, tview/tcell terminal UI

## Codebase Structure

- **cmd/gowatch/** - Entry point
- **internal/docker/** - Docker daemon interaction
- **internal/ui/** - Terminal dashboard
- **internal/config/** - Configuration
- **pkg/metrics/** - Public types

## Review Checklist

### Go Idioms
1. Error handling: `if err != nil` patterns, no ignored errors
2. Named return values used appropriately
3. Interfaces defined where behavior is abstracted
4. Constructors follow `NewXxx() *Xxx` pattern
5. Package names are lowercase, no underscores

### Concurrency
1. Goroutine leaks prevented (defer close, context cancellation)
2. Mutex usage is correct (RWMutex where appropriate)
3. Channel operations are non-blocking or properly managed
4. Race conditions avoided (run `make test` with `-race`)

### Docker Integration
1. Docker client errors handled gracefully
2. Resources (response bodies) properly closed with `defer`
3. Container stats parsing handles edge cases
4. Log stream parsing handles Docker multiplexed format

### UI/Terminal
1. tview components properly initialized
2. Color codes used correctly (`[yellow]`, `[-]`)
3. Scroll state preserved during updates
4. No blocking operations on UI thread

### Security
1. No hardcoded credentials
2. Docker socket access is necessary
3. Input validation where applicable

## Responsibilities

1. Review PRs for Go code quality
2. Check adherence to project patterns
3. Verify test coverage
4. Run `make go-sec` for security issues
5. Suggest improvements

## Relevant Files

- `internal/docker/collector.go` - Container monitoring logic
- `internal/docker/parser.go` - Stats and log parsing
- `internal/ui/dashboard.go` - Terminal UI
- `*_test.go` - Test files

---
*Updated for GoWatch project - March 2026*
