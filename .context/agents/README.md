# Agent Handbook

This directory contains ready-to-customize playbooks for AI agents collaborating on the GoWatch repository.

## Project Context

**GoWatch** is a real-time Docker container monitoring tool written in Go. It provides terminal UI visibility into container resource usage, logs, and system metrics.

## Available Agents
- [Code Reviewer](./code-reviewer.md) — Review Go code for quality, idioms, and Docker integration patterns
- [Bug Fixer](./bug-fixer.md) — Debug Docker collector, UI rendering, and goroutine issues
- [Feature Developer](./feature-developer.md) — Implement new monitoring features and UI components
- [Refactoring Specialist](./refactoring-specialist.md) — Optimize Go code, improve interfaces, fix struct alignment
- [Test Writer](./test-writer.md) — Write Go tests for Docker collector, parser, and UI logic
- [Documentation Writer](./documentation-writer.md) — Maintain README, code comments, and context docs
- [Performance Optimizer](./performance-optimizer.md) — Optimize Docker API calls, UI rendering, memory usage

## How To Use These Playbooks
1. Pick the agent that matches your task.
2. Review the agent's responsibilities and relevant files.
3. Follow Go conventions and project patterns.
4. Run `make test` and `make go-sec` before completing.

## Tech Stack Reference
- **Language**: Go 1.26+
- **Docker Client**: Moby (`github.com/moby/moby`)
- **Terminal UI**: tview + tcell
- **Build**: makefile

## Related Resources
- [Documentation Index](../docs/README.md)
- [Agent Knowledge Base](../../AGENTS.md)
- [Project README](../../README.md)
