---
type: agent
name: Documentation Writer
description: Create clear, comprehensive documentation
agentType: documentation-writer
phases: [P, C]
generated: 2026-03-22
status: filled
scaffoldVersion: "2.0.0"
---

# Documentation Writer Agent

## Role

This agent specializes in documentation for GoWatch, a real-time Docker container monitoring tool written in Go.

## Project Context

GoWatch monitors Docker containers via the Moby client and displays metrics in a terminal dashboard. Documentation should help users install, run, and understand the project, while helping developers contribute effectively.

**Tech Stack**: Go 1.26+, Moby Docker client, tview/tcell terminal UI

## Codebase Structure

- **cmd/gowatch/** - Entry point
- **internal/docker/** - Docker interaction
- **internal/ui/** - Terminal dashboard
- **internal/config/** - Configuration
- **pkg/metrics/** - Public types
- **.context/docs/** - Project documentation
- **.context/agents/** - Agent playbooks

## Documentation Types

### User Documentation
- **README.md** - Installation, quick start, features
- **Usage examples** - Common use cases
- **Configuration** - Any config options (currently zero-config)

### Developer Documentation
- **.context/docs/project-overview.md** - Architecture and components
- **.context/docs/development-workflow.md** - Dev process, commands
- **.context/docs/testing-strategy.md** - Test patterns
- **.context/docs/tooling.md** - Tools and IDE setup
- **Code comments** - Complex logic explanation

### Agent Documentation
- **.context/agents/*.md** - AI agent playbooks
- Task-specific guidance and patterns

## Go Documentation Standards

### Package Comments
```go
// Package docker provides Docker container monitoring functionality.
// It connects to the Docker daemon and collects stats, logs, and metadata.
package docker
```

### Function Comments
```go
// WatchContainers retrieves all running containers and their metrics.
// It returns a Containers struct with container info, logs, and host stats.
// Returns an error if the Docker daemon is unreachable.
func WatchContainers(ctx context.Context, apiClient *client.Client) (Containers, error) {
```

### Inline Comments
```go
// Parse payload size from bytes 4-7 (big-endian uint32)
size := int(header[4])<<24 | int(header[5])<<16 | int(header[6])<<8 | int(header[7])
```

## Documentation Workflow

1. Identify documentation needs (new feature, API change, user question)
2. Write clear, concise content
3. Include code examples where helpful
4. Update related docs (README, .context files)
5. Verify links and cross-references
6. Run `go doc` to check package documentation

## Responsibilities

1. Keep README.md accurate and helpful
2. Maintain .context documentation
3. Add code comments for complex logic
4. Document APIs and interfaces
5. Update agent playbooks when patterns change

## Relevant Files

- `README.md` - Main project documentation
- `.context/docs/` - Project context docs
- `.context/agents/` - Agent playbooks
- `AGENTS.md` - AI context references
- `internal/**/*.go` - Code comments

---
*Updated for GoWatch project - March 2026*
