# GoWatch

GoWatch is a lightweight, real-time monitoring tool **written in Go** for Docker containers. It provides developers with instant visibility into container resource usage, logs, and system metrics directly in the terminal. Perfect for local development environments, GoWatch works with **any service running in Docker** regardless of the programming language, with planned AWS Serverless support for monitoring CloudWatch, XRay, Lambda, and CloudFormation resources.

## Features

- **Universal Container Monitoring**: Monitor CPU, memory, and resource usage for **any service in Docker** (Node.js, Python, Java, Go, PHP, etc.)
- **Real-time Log Aggregation**: Stream and view container logs from all services in one place
- **Live Resource Tracking**: Track CPU percentage, memory usage, and system metrics with automatic updates
- **Docker Integration**: Direct Docker daemon integration using the Moby client library
- **Terminal UI Dashboard**: Beautiful, interactive terminal interface built with tcell and tview
- **Zero Configuration**: Auto-detects running containers and requires no setup
- **Themes and Layout**: Optional YAML config with four built-in color themes (dark, light, solarized, nord), per-color overrides and a configurable panel layout
- **Keyboard Shortcuts**: Remappable keys, a `vim` preset and an interactive help page (press `?`)
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

Press `?` in the dashboard to see every keyboard shortcut (`?`, `Esc` or `q` closes the page). The keys can be changed, see [Keybindings](#keybindings).

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
| `--theme <name>` | Use a built-in color theme (`dark`, `light`, `solarized` or `nord`). Replaces `theme.name` from the config file. |
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

theme:
  name: dark               # dark | light | solarized | nord (--theme replaces it)
  # colors:                # override single colors by role, see "Themes" below
  #   accent: "#ff8800"

layout:                    # panels from top to bottom, see "Layout" below
  services:
    height: 1              # relative height, 1-100 (0 or omitted means 1)
  resources:
    visible: true
    height: 1
  logs:
    visible: true
    height: 1
  help_bar:
    visible: true

keybindings:               # keyboard shortcuts, see "Keybindings" below
  preset: default          # default | vim
  # bindings:              # replaces ALL the keys of an action
  #   quit: ["q", "x"]
  #   scroll_down: ["down", "j"]
```

**Alert rules.** The built-in rules are `high_cpu`, `high_mem`, `oom_killed` and `container_down`. Under `alerts.rules` you can change `threshold`, `hysteresis`, `consecutive_hits` and `severity` of a rule, or drop it with `enabled: false`. `threshold` and `hysteresis` must be finite numbers >= 0, and `hysteresis` must not exceed `threshold`; if you change `threshold` and leave `hysteresis` out, the built-in hysteresis is lowered to match when needed. Write each rule ID once per file (`high_cpu` and `HIGH_CPU` count as the same rule). If the same rule appears in both config files, the two entries are combined field by field.

**OTLP receiver.** When `tracing.otlp_receiver.enabled` is `true`, GoWatch listens on `address` and accepts OTLP/HTTP JSON traces on `/v1/traces`, which then show up in the dashboard. The receiver can only be turned on from `~/.gowatch/config.yaml` or a `--config` file, not from `./gowatch.yaml`. If the address cannot be bound, GoWatch exits with an error before the dashboard opens.

**AWS.** `aws.region` and `aws.endpoint_url` are used when the corresponding environment variables are not set (`AWS_REGION`, then `AWS_DEFAULT_REGION`, then the config file, then `us-east-1`; `AWS_ENDPOINT_URL` before the config file). With `enabled: auto` the AWS integration is on when `AWS_ACCESS_KEY_ID`, `AWS_PROFILE`, `AWS_REGION` or `AWS_ENDPOINT_URL` is set, or when the config file sets `region` or `endpoint_url`. `enabled: true` or `enabled: false` always wins.

**Themes.** `theme.name` picks one of the built-in themes: `dark` (the default, and the look GoWatch has always had), `light`, `solarized` (Solarized Dark) or `nord`. The `--theme` flag replaces the name from the file. Names are case-insensitive, and an unknown name stops GoWatch at startup with the list of valid ones. The light, solarized and nord themes use true-color values, so they look best in a terminal with true-color support.

`theme.colors` overrides single colors on top of the chosen theme. The key is a role and the value is a color name known to tcell (such as `yellow`, `navy`, `aqua` or `silver`; `cyan` and `magenta` also work) or `#rrggbb`. Quote the `#rrggbb` form in YAML, because `#` starts a comment. Roles are written exactly as listed; an unknown role or an unreadable color stops GoWatch at startup. The colors apply on top of the theme even when it comes from `--theme`.

| Role | What it colors |
| --- | --- |
| `background` | Background of every panel |
| `text` | Regular text |
| `muted` | Secondary text, such as timestamps and counters |
| `accent` | Table headers, labels and service names in the logs |
| `border` | Panel borders and table grid lines |
| `title` | Panel titles |
| `info` | Informational columns, such as the image name |
| `subheading` | Sub-headings, such as "Recent Traces:" |
| `faint` | Least important text, such as the Docker retry notice |
| `status_ok`, `status_warn`, `status_error` | Container states, alert counts, alert status and severity |
| `help_bar_bg`, `help_key`, `help_text` | Help bar background, keys and descriptions (the key hints in the alerts modal use the same colors) |
| `log_fatal` | Currently colors only the background of the rest of a FATAL log line (see the known limitation below) |
| `log_fatal_text`, `log_error`, `log_warn`, `log_info`, `log_debug` | Reserved for the log level badges (DEBUG and TRACE share `log_debug`). No visible effect yet (see the known limitation below) |
| `field_bg`, `field_hint` | Search field background and placeholder text |

The colors that tell services apart in the logs come from the theme and cannot be overridden one by one.

**Known limitations.** The log level badges (`[INFO]`, `[WARN]`, `[ERROR]`, `[FATAL]`...) are not drawn: the terminal UI library reads text between square brackets as a color tag, so the badge text never appears in the log lines. That is how GoWatch has always drawn them, and showing them would change the default look, so it has not been changed. As a result, `log_error`, `log_warn`, `log_info`, `log_debug` and `log_fatal_text` have no visible effect yet, and `log_fatal` only colors the background of the rest of a FATAL log line. The roles are kept so that a config file that sets them keeps working if the badges become visible. A related case: the root operation of the recent traces (shown only when the OTLP receiver is on) is drawn in the terminal's default text color in every theme, so it can be hard to read with the `light` theme in a dark terminal, or with `solarized` or `nord` in a light one.

**Layout.** `layout` describes the panels from top to bottom: the services table, system resources, the search field, the logs and the help bar. `height` is the share of the free space a panel gets compared to the others, from 1 to 100: with every height at 1 the panels are equal, and `height: 2` gives a panel twice the space. `0` or no value counts as 1. The search field (3 rows) and the help bar (1 row) have a fixed height. `visible: false` removes the system resources, the logs or the help bar from the screen; the services table and the search field are always shown. With the logs hidden, the focus starts on the services table and goes back to it after `Esc`, `Enter` or closing the alerts modal. The layout is read at startup; panels cannot be toggled while GoWatch runs.

### Keybindings

The keys are the ones GoWatch has always had, plus `?` for the help page. `keybindings.preset` picks the starting keys and `keybindings.bindings` changes single actions:

```yaml
keybindings:
  preset: vim              # default | vim
  bindings:
    quit: ["q", "x"]       # replaces ALL the keys of the action
    search: ["/"]          # with no "f", the help bar no longer offers a filter key
```

| Action | What it does | `default` keys | `vim` keys |
| --- | --- | --- | --- |
| `help` | Show or hide the help page | `?` | `?` |
| `quit` | Quit GoWatch | `q` | `q` |
| `docker_view` | Show the Docker services | `d`, `D` | `d`, `D` |
| `aws_view` | Show the AWS resources | `a`, `A` | `a`, `A` |
| `search` | Focus the search field | `/`, `f` | `/`, `f` |
| `cycle_log_level` | Cycle the minimum log level | `l`, `L` | `l`, `L` |
| `clear_filter` | Clear all filters, including the ones from the config file (closes the alerts list first when it is open) | `Esc` | `Esc` |
| `toggle_alerts` | Show or hide the alerts list | `w`, `W` | `w`, `W` |
| `clear_resolved_alerts` | Clear the resolved alerts (works on every screen) | `c`, `C` | `c`, `C` |
| `scroll_up` | Scroll up a line | `↑` | `↑`, `k` |
| `scroll_down` | Scroll down a line | `↓` | `↓`, `j` |
| `page_up` | Scroll up a page | `PgUp` | `PgUp`, `Ctrl+U` |
| `page_down` | Scroll down a page | `PgDn` | `PgDn`, `Ctrl+D` |
| `scroll_top` | Jump to the top | `Home` | `Home`, `g` |
| `scroll_bottom` | Jump to the bottom | `End` | `End`, `G` |

The `vim` preset is the `default` preset plus `j`, `k`, `g`, `G`, `Ctrl+D` and `Ctrl+U`. `Ctrl+D` and `Ctrl+U` move a whole page, the height of the logs panel, not half a page as in vim. There is no `emacs` preset.

**Keys.** A key is a single character (`q`, `/`, `?`, `G`; upper and lower case are different keys, so `l` and `L` are listed separately), `ctrl+<letter>` (`ctrl+d`), `space`, or one of `esc` (or `escape`), `enter`, `tab`, `up`, `down`, `pgup`, `pgdn`, `home` and `end`. The names are case-insensitive. Quote the keys in YAML, because characters such as `?`, `#`, `,` and `[` mean something to it: `["?", "#"]`. Action names are written in lower case with underscores (`Quit` also works).

**Rules.** An override replaces all the keys of its action, so listing `quit: ["x"]` makes `q` stop quitting; actions you leave out keep the keys of the preset. Each action needs at least one key, a key can run only one action and a key cannot be repeated within an action. To swap the keys of two actions, override both of them. Any problem (an unknown preset, action or key, a clash, `ctrl+c`...) stops GoWatch at startup with a message that lists the valid choices. The rules are checked together, so the message shows every problem at once.

**Keys that cannot be rebound.** `Ctrl+C` always quits GoWatch, wherever you are, and cannot be bound to an action. `ctrl+h`, `ctrl+i` and `ctrl+m` are not accepted either: a terminal sends the same bytes as for Backspace, Tab and Enter, so GoWatch could not tell them apart (use `tab` and `enter`). The arrow keys, `PgUp`, `PgDn`, `Home` and `End` (and `Ctrl+F` and `Ctrl+B`) are handled by the panel with focus, which scrolls with them in every keymap. They are listed in the scroll actions for the help page and the help bar, and can only be bound to the scroll action that matches what they do (`up` to `scroll_up`, and so on). The native keys scroll whichever panel has the focus (the logs, unless the alerts list or the help page is open). The other keys of the scroll actions, such as `j` and `k` in the `vim` preset, scroll the logs, or the alerts list when it is open, or the help page when that is open. Scrolling the logs with the arrow keys, `PgUp`, `PgDn`, `Home` or `End` (`End` included) stops them from following new lines; only a `scroll_bottom` key that is not one of those (`G` in the `vim` preset) makes them follow the newest line again.

**Search field.** While the search field has the focus, every key is typed into it, `?` and the bound letters included.

**Help page.** `?` (the `help` action) opens a page that lists every action by section with the keys in use, so remapped keys show up as you set them, followed by the keys that always work. It opens over the alerts list too, and goes back to it when it closes. The page closes with the keys of the `help` and `quit` actions and with `Esc`, and quitting does not happen while it is open. No other action runs while it is open (the view cannot change behind it, and the search does not open).

**Help bar.** The bar at the bottom and the title of the alerts list show the first key of each action, so they follow your bindings. With the default keys the bar is the original one with one more entry, `? Help`, between `↑↓ Scroll` and `q Quit`.

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
