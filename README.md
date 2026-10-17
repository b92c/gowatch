# GoWatch

GoWatch is a lightweight, real-time monitoring tool **written in Go** for Docker containers. It provides developers with instant visibility into container resource usage, logs, and system metrics directly in the terminal. Perfect for local development environments, GoWatch works with **any service running in Docker** regardless of the programming language, with planned AWS Serverless support for monitoring CloudWatch, XRay, Lambda, and CloudFormation resources.

## Features

- **Universal Container Monitoring**: Monitor CPU, memory, and resource usage for **any service in Docker** (Node.js, Python, Java, Go, PHP, etc.)
- **Real-time Log Aggregation**: Stream and view container logs from all services in one place
- **Live Resource Tracking**: Track CPU percentage, memory usage, and system metrics with automatic updates
- **Docker Integration**: Direct Docker daemon integration using the Moby client library
- **Terminal UI Dashboard**: Beautiful, interactive terminal interface built with tcell and tview
- **Zero Configuration**: Auto-detects running containers and requires no setup
- **Development Focused**: Perfect for docker-compose environments and local microservice development
- **Lightweight**: Minimal resource footprint, built in pure Go
- **Future AWS Integration**: Planned support for AWS services like CloudWatch, XRay, Lambda, and CloudFormation

## Development Environment

GoWatch includes a fully configured Docker development environment:

### Docker Development Container

The project includes a complete Docker setup for development:

- **Dockerfile**: Multi-stage build using Go tip-alpine and Alpine runtime
- **Docker Compose**: Pre-configured service with volume mounts and Docker socket access
- **Development Tools**: Includes security scanning and field alignment tools

### Quick Start with Docker

```bash
# Build the Docker development environment
make docker-build

# Run GoWatch in Docker to monitor your services
docker compose up
```

## User Interface

GoWatch provides a real-time terminal dashboard with three main sections:

**Docker Services Table** - View all running containers with:
- Container status (Running, Stopped, etc.)
- Container image information
- CPU usage percentage
- Memory consumption (MB)
- Log line count

**System Resources Panel** - Monitor host system:
- Total CPU cores
- Total memory
- Available free memory
- Last update timestamp

**Logs Section** - Aggregate logs from all containers:
- Color-coded output
- Real-time log streaming
- Service name identification

### Dashboard Example

![GoWatch Dashboard](.assets/gowatch-dash.gif)

The interactive dashboard updates every 2 seconds by default (configurable, see [Configuration](#configuration)), providing live visibility into your containerized services.

## Installation

### Quick Install (Recommended)

Install GoWatch with a single command — no Go or Git required:

**Linux / macOS:**

```bash
curl -sSL https://raw.githubusercontent.com/b92c/gowatch/main/.installer/install.sh | bash
```

**Install a specific version:**

```bash
curl -sSL https://raw.githubusercontent.com/b92c/gowatch/main/.installer/install.sh | bash -s -- v1.0.0
```

**Custom install directory:**

```bash
GOWATCH_INSTALL_DIR=$HOME/.local/bin curl -sSL https://raw.githubusercontent.com/b92c/gowatch/main/.installer/install.sh | bash
```

**Windows (Git Bash / MSYS2):**

```bash
curl -sSL https://raw.githubusercontent.com/b92c/gowatch/main/.installer/install.sh | bash
```

### Uninstall

```bash
curl -sSL https://raw.githubusercontent.com/b92c/gowatch/main/.installer/uninstall.sh | bash
```

### Manual Installation

To install GoWatch from source, ensure you have Go 1.26 or later installed:

```bash
git clone https://github.com/b92c/gowatch.git
cd gowatch
make install
make run
```

## Build Commands

```bash
# Install dependencies and hooks
make install

# Build the binary
make build

# Run the application
make run

# Build Docker image
make docker-build

# Run tests
make test

# Security scan
make go-sec

# Fix struct field alignment
make field-fix
```

## Project Structure

```
gowatch/
├── cmd/gowatch/          # Application entry point
├── internal/
│   ├── aws/              # Future AWS integrations (XRay, CloudWatch, Lambda, CloudFormation)
│   ├── docker/           # Docker monitoring and collection
│   ├── trace/            # Distributed tracing functionality
│   ├── ui/               # Terminal UI components and dashboard
│   └── config/           # YAML configuration loading and validation
├── pkg/metrics/          # Metrics types and definitions
├── docker-compose.yaml   # Docker development environment
├── Dockerfile           # Multi-stage build configuration
└── makefile            # Build and development commands
```

## Configuration

GoWatch works out of the box: with no configuration file it detects your running containers, refreshes every 2 seconds, tails the last 50 log lines per container, keeps 60 history points and runs the built-in alert rules. A configuration file is only needed to change those defaults.

### Config files and precedence

Settings are layered, from lowest to highest priority:

1. Built-in defaults
2. `~/.gowatch/config.yaml`
3. `./gowatch.yaml` (in the directory where you start GoWatch)
4. Environment variables (AWS only, see below)

Each file only overrides the keys it sets; everything else keeps the value from the previous layer. Lists (such as `filters.status`) are replaced as a whole, while maps (such as `alerts.rules`) are merged by key. A file that does not exist is skipped. With `--config <file>`, only the built-in defaults and that file are used, and the file must exist.

Because `./gowatch.yaml` may come from a repository you merely cloned, it is not allowed to set `aws.endpoint_url`, to set `tracing.otlp_receiver.enabled` to `true`, or to set a `tracing.otlp_receiver.address` other than `127.0.0.1`, `[::1]` or `localhost`: GoWatch refuses to start and tells you to move the setting to `~/.gowatch/config.yaml` or to pass the file with `--config`. (`tracing.otlp_receiver.enabled: false` is accepted.) `~/.gowatch/config.yaml` and a `--config` file may set all of them.

Unknown keys, malformed YAML (including a second `---` document) and out-of-range values stop GoWatch at startup with a message on stderr (including the file and line for YAML problems), so a typo never goes unnoticed.

### Command-line flags

| Flag | Description |
| --- | --- |
| `--config <file>` | Use this file instead of `~/.gowatch/config.yaml` and `./gowatch.yaml`. |
| `--print-default-config` | Print a commented example configuration to stdout and exit. |

The easiest way to start is:

```bash
gowatch --print-default-config > gowatch.yaml
```

The generated file contains every default value, so using it unchanged behaves exactly like having no file at all.

### Options

```yaml
refresh_interval: 2s       # how often containers are polled (minimum 500ms)

logs:
  tail_lines: 50           # log lines fetched per container (1-5000)

history:
  max_points: 60           # samples kept for the CPU/memory sparklines (10-3600)

filters:                   # applied at startup; Esc in the dashboard clears all of them
  search: ""               # matches service name, container ID or image
  min_log_level: ""        # info | warn | error (empty shows every level)
  status: []               # container states to show, for example [running, exited]

alerts:
  enabled: true            # false turns the alerting engine off
  rules:                   # override built-in rules by ID; omitted fields keep their value
    high_cpu:
      threshold: 85
      # hysteresis: 75     # optional; must not exceed threshold
      consecutive_hits: 3
      severity: warn       # warn | crit
      enabled: true

tracing:
  otlp_receiver:
    enabled: false         # off by default
    address: "127.0.0.1:4318"

aws:
  region: ""
  endpoint_url: ""
  enabled: auto            # auto | true | false
```

**Alert rules.** The built-in rules are `high_cpu`, `high_mem`, `oom_killed` and `container_down`. Under `alerts.rules` you can change `threshold`, `hysteresis`, `consecutive_hits` and `severity` of a rule, or drop it with `enabled: false`. `threshold` and `hysteresis` must be finite numbers >= 0, and `hysteresis` must not exceed `threshold`; if you change `threshold` and leave `hysteresis` out, the built-in hysteresis is lowered to match when needed. Write each rule ID once per file (`high_cpu` and `HIGH_CPU` count as the same rule). If the same rule appears in both config files, the two entries are combined field by field.

**OTLP receiver.** When `tracing.otlp_receiver.enabled` is `true`, GoWatch listens on `address` and accepts OTLP/HTTP JSON traces on `/v1/traces`, which then show up in the dashboard. The receiver can only be turned on from `~/.gowatch/config.yaml` or a `--config` file, not from `./gowatch.yaml`. If the address cannot be bound, GoWatch exits with an error before the dashboard opens.

**AWS.** `aws.region` and `aws.endpoint_url` are used when the corresponding environment variables are not set (`AWS_REGION`, then `AWS_DEFAULT_REGION`, then the config file, then `us-east-1`; `AWS_ENDPOINT_URL` before the config file). With `enabled: auto` the AWS integration is on when `AWS_ACCESS_KEY_ID`, `AWS_PROFILE`, `AWS_REGION` or `AWS_ENDPOINT_URL` is set, or when the config file sets `region` or `endpoint_url`. `enabled: true` or `enabled: false` always wins.

## Use Cases

GoWatch is ideal for:

- **Local Microservices Development**: Monitor multiple services in docker-compose environments
- **Performance Debugging**: Quickly identify which containers are consuming excessive CPU or memory
- **Log Troubleshooting**: Aggregate logs from all services to debug issues across your stack
- **Development Environment Monitoring**: Keep an eye on resource usage while coding and testing
- **Container Health Checks**: Ensure all your services are running and healthy

## Supported Technologies

Since GoWatch monitors any Docker container, you can use it with services written in:

- **Go** (Go applications)
- **Python** (Django, FastAPI, Flask, etc.)
- **Node.js** (Express, NestJS, etc.)
- **Java** (Spring Boot, Quarkus, etc.)
- **PHP** (Laravel, Symfony, etc.)
- **Ruby** (Rails, Sinatra, etc.)
- **C#** (.NET, ASP.NET Core)
- **Rust** (Actix, Rocket, etc.)
- Any other language/framework running in Docker containers

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests and security checks
5. Submit a pull request

## License

MIT
