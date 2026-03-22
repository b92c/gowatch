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
- **UI Components**: Test data transformation (difficult to test rendering directly)

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
- **internal/ui/** - Data transformation functions (rendering is visual)

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
│   └── dashboard_test.go    # Data transformation tests
└── config/
    ├── config.go
    └── config_test.go       # Configuration tests
```

## Quality Gates

1. All tests must pass (`make test`)
2. No race conditions (`-race` flag enabled)
3. Security scan clean (`make go-sec`)
4. Struct alignment optimized (`make field-fix`)

---
*Updated for GoWatch project - March 2026*
