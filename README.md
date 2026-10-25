

# GoWatch

GoWatch is a lightweight, real-time monitoring tool **written in Go** for Docker containers. It provides developers with instant visibility into container resource usage, logs, and system metrics directly in the terminal. Perfect for local development environments, GoWatch works with **any service running in Docker** regardless of the programming language. Its AWS view shows the account your AWS credentials belong to and the accounts of your AWS SSO login; CloudWatch Logs, Lambda, X-Ray and CloudFormation are planned.

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
- **AWS Accounts**: A read-only AWS view (press `a`) that shows the account behind your AWS credentials, found with the standard AWS credential chain, and the accounts and roles of your AWS SSO login; CloudWatch Logs, Lambda, X-Ray and CloudFormation are planned

## Development Environment

GoWatch includes a fully configured Docker development environment:

### Docker Development Container

The project includes a complete Docker setup for development:

- **Dockerfile**: Multi-stage build using the stable Go 1.27 Alpine image and an Alpine runtime
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
- Container state (running, paused or restarting; stopped containers are not listed)
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
- Keyword search and JSON export (see [Searching and exporting logs](#searching-and-exporting-logs))

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

To install GoWatch from source, ensure you have Go 1.27 or later installed:

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
│   ├── aws/              # AWS client: settings, credentials, identity, SSO accounts (CloudWatch, Lambda, X-Ray, CloudFormation planned)
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

Because `./gowatch.yaml` may come from a repository you merely cloned, it is not allowed to set `aws.endpoint_url`, to set `tracing.otlp_receiver.enabled` to `true`, or to set a `tracing.otlp_receiver.address` other than `127.0.0.1`, `[::1]` or `localhost`: GoWatch refuses to start and tells you to move the setting to `~/.gowatch/config.yaml` or to pass the file with `--config`. (`tracing.otlp_receiver.enabled: false` is accepted.) `~/.gowatch/config.yaml` and a `--config` file may set all of them. `aws.profile` and `aws.accounts` are accepted from every file, `./gowatch.yaml` included: they only choose among the credentials and the SSO accounts you already have and cannot redirect traffic or expose anything.

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
  search: ""               # matches service name, container ID or image; also shows the log lines that contain it
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
  profile: ""              # profile of ~/.aws/config; AWS_PROFILE takes priority; empty means default
  refresh_interval: 15s    # how often the poller runs (5s-10m); for now it only calls AWS on discovery passes
  discovery_interval: 60s  # how often the shared files, the identity and the SSO accounts are read again (15s-1h)
  accounts: []             # more accounts to monitor through SSO: "<account id>" or "<account id>/<role>"

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

**OTLP receiver.** When `tracing.otlp_receiver.enabled` is `true`, GoWatch listens on `address` and accepts OTLP/HTTP JSON traces on `/v1/traces`, which then show up in the dashboard. Traces are also built from Docker log lines that carry trace context (a W3C `traceparent`, a `trace_id`/`traceId` field, or a `request_id=` key), with or without the receiver. The receiver can only be turned on from `~/.gowatch/config.yaml` or a `--config` file, not from `./gowatch.yaml`. If the address cannot be bound, GoWatch exits with an error before the dashboard opens.

**AWS.** GoWatch reads AWS through the AWS SDK for Go v2, so it finds credentials the way the AWS CLI does: environment variables, the shared config and credentials files (SSO profiles included), web identity and container credentials. Instance metadata (the role of an EC2 instance) is only tried with `aws.enabled: true`, never with `auto`, so on an EC2 instance with an instance role set `aws.enabled: true`. With `enabled: auto`, the default, the integration starts only when something configures AWS: one of the variables `AWS_ACCESS_KEY_ID`, `AWS_PROFILE`, `AWS_REGION`, `AWS_DEFAULT_REGION`, `AWS_ENDPOINT_URL`, `AWS_WEB_IDENTITY_TOKEN_FILE`, `AWS_CONTAINER_CREDENTIALS_RELATIVE_URI` or `AWS_CONTAINER_CREDENTIALS_FULL_URI`; `aws.profile`, `aws.region`, `aws.endpoint_url` or `aws.accounts` in a config file; or a shared config file (`AWS_CONFIG_FILE`, otherwise `~/.aws/config`) or shared credentials file (`AWS_SHARED_CREDENTIALS_FILE`, otherwise `~/.aws/credentials`) that exists. Without any of them nothing AWS runs, no call is made and the AWS view says `not configured`. `enabled: true` always starts the integration and `enabled: false` never does.

**AWS profile, region and endpoint.** The profile is `AWS_PROFILE`, then `aws.profile`, then `default`. The region is `AWS_REGION`, then `AWS_DEFAULT_REGION`, then `aws.region`, then the region of the profile, then `us-east-1`; once the first pass has loaded the profile, the title of the AWS view shows the region in use. The endpoint is `AWS_ENDPOINT_URL`, then `aws.endpoint_url`, and applies to every AWS client (LocalStack, for example). That includes the SSO portal and OIDC clients, so with an SSO profile the access token of your login is sent to that endpoint: for a stub or LocalStack, point `AWS_PROFILE` or `aws.profile` at a profile without SSO settings; exported keys alone do not help, because the discovery follows the profile.

**AWS refresh.** GoWatch does the AWS work on its own goroutine, so it never delays startup, the dashboard or the Docker refresh. It runs a pass every `aws.refresh_interval` (15s by default), but for now only the discovery passes call AWS: every `aws.discovery_interval` (60s by default) the pass reloads the shared config and credentials files, asks AWS who the credentials belong to and, when the profile has an SSO login, lists the accounts of that login, so keys that you export again, or an `aws sso login` that you run, while GoWatch runs are picked up without a restart. Each call has a 10 second timeout and the SDK does not retry it: a failure is tried again on the next discovery pass.

**AWS view.** Press `a` (the `aws_view` action). The title is ` AWS Cloud Resources (<region>) <status> `, and the status is one of `not configured`, `disabled`, `connecting`, `connected: <account id>` or `error: <problem>`. Once connected, the table starts with an `Account` row for the account of the active credentials: its name (the account ID, or its name in the SSO portal when the login lists it), the status `monitored` and, in the Metrics Summary column, the account ID and the ARN of the caller. With an SSO login, one row per account of the login follows, ordered by name, and the Metrics Summary column adds `roles: <the roles the login can use in the account>` and, for an `aws.accounts` entry that GoWatch monitors, `using <role>`; see the SSO notes below. When there is no row, a single muted row says why: `AWS not configured: set up the AWS CLI or set aws.enabled: true`, `AWS integration is disabled (aws.enabled: false)`, `Connecting to AWS...` or the problem. If a later pass fails, the account row stays and its status shows the problem. A problem with the SSO accounts does not hide a working identity: the title stays `connected` and carries the problem after it, for example `connected: 123456789012; SSO token expired - run aws sso login`. These are the problems GoWatch recognizes, each followed by its hint when there is one (the hint shows in the Status column of the account row or, with no row, in the title and in the single row):

| Problem | Shown as | Hint |
| --- | --- | --- |
| No credential provider found | `no AWS credentials found` | `configure the AWS CLI or set aws.enabled: false` |
| The SSO token is missing or expired, cannot be refreshed, or is rejected by the SSO portal | `SSO token expired` | `run aws sso login` |
| AWS rejects the keys (expired, invalid or wrong signature) | `AWS credentials expired or invalid` | `log in again (for SSO logins, aws sso login and re-export)` |
| The profile's `credential_process` fails or prints something that is not valid JSON (its output is never shown) | `credential_process failed` | `check the credential_process of the profile` |
| The call is not allowed | `access denied: <operation>` | |
| AWS throttles the call (the rest of that pass is skipped) | `throttled by AWS, slowing down` | |
| Timeout or network error | `AWS unreachable: <cause>`, where the cause is `timeout`, `DNS lookup failed` or `network error` | |

A profile that does not exist is shown as `AWS profile "<name>" not found` (hint: `check AWS_PROFILE or aws.profile`), and any other failure as `<step> failed: <first line of the error>`, cut to 120 characters. The SDK runs the `credential_process` of a profile on every discovery pass, with GoWatch's own standard input and standard error, so a program that prompts (for example for MFA) cannot be answered while the dashboard is on screen, and anything it prints to standard error can garble the screen until the next redraw. Prefer exported keys or an SSO profile.

**AWS SSO accounts.** When the profile in use has an SSO login, either `sso_start_url` and `sso_region` in the profile itself or an `sso_session` that points to an `[sso-session]` section, GoWatch lists the accounts of that login in the AWS view. It does so even when the identity comes from keys you exported to `~/.aws/credentials`, which the SDK prefers to the SSO settings of the same profile. On every discovery pass it asks the SSO portal, in the `sso_region` of the profile, for the accounts of the login (at most 100) and for the roles you can use in each (at most 20), with `sso:ListAccounts` and `sso:ListAccountRoles`. The account of the active credentials is `monitored`; every other account of the login is `available`, unless `aws.accounts` asks for it (see below). A profile without an SSO login has only the identity row, apart from the rows of `aws.accounts` entries. The portal sends an e-mail address with each account: GoWatch never reads, keeps or shows it.

**The SSO token.** The calls to the portal carry the access token of your login. For an `sso_session` profile the SDK's token provider gets it: it reads the token cache under `~/.aws/sso/cache/` and, when the token has expired and the cache holds what is needed to refresh it, refreshes it with `sso-oidc:CreateToken` and rewrites its own cache file, as the AWS CLI does. For a legacy profile (`sso_start_url` in the profile) GoWatch reads, and only reads, the one cache file the SDK names for the start URL; it never refreshes it, and a missing, unreadable or expired file is `SSO token expired` with the hint `run aws sso login`. In both forms that problem shows in the title next to `connected` and the accounts listed before stay on screen. Run `aws sso login` and the next discovery pass finds the new token and clears the problem, with no restart. The token is kept in memory only and is never shown or logged.

**Monitoring more accounts.** `aws.accounts` lists the accounts to monitor besides the one of the active credentials, each as `"<account id>"` or `"<account id>/<role>"`, for example `accounts: ["210987654321", "111111111111/ReadOnly"]`. For every entry that the SSO list contains, GoWatch gets credentials for the account and the role with the SDK's SSO provider (`sso:GetRoleCredentials`, with the token of your login) and checks them once; the account is then `monitored`, and the credentials are not requested again while the account and the role stay the same. Without a role in the entry, GoWatch uses the `sso_role_name` of the profile when the login can use it in that account, and otherwise the first role the login can use there, shown as `monitored (first role <role>)`. An entry for the account of the active credentials is ignored, and when an account appears twice the first entry counts. These are the statuses of the account rows:

| Status | Meaning |
| --- | --- |
| `monitored` | The account of the active credentials, or an `aws.accounts` entry whose credentials GoWatch got |
| `monitored (first role <role>)` | An `aws.accounts` entry that names no role, when the login cannot use the `sso_role_name` of the profile in that account: GoWatch used the first role the login can use there |
| `available` | An account of the login that no entry asks for, or an `aws.accounts` entry that a throttled pass did not reach yet (until the next pass) |
| `requires SSO` | An `aws.accounts` entry, but the profile has no SSO login |
| `error: not accessible with this login` | An `aws.accounts` entry that the login does not list; the row shows the account ID |
| `error: role <role> not listed for this account` | The entry names a role that the login cannot use in that account |
| `error: no roles available` | The entry names no role and the login can use none in that account |
| `error: <problem>` | The credentials of the entry could not be got, for example `error: access denied: GetRoleCredentials`, or the SSO accounts could not be read yet, for example `error: SSO token expired - run aws sso login` |

**AWS calls.** The calls GoWatch makes itself are read-only: `sts:GetCallerIdentity` and, for a profile with an SSO login, `sso:ListAccounts`, `sso:ListAccountRoles` and, for the entries of `aws.accounts`, `sso:GetRoleCredentials`. Resolving the credentials may involve the calls of the SDK's own providers, for example for an SSO or an assume-role profile. `sso-oidc:CreateToken` is the call that refreshes the SSO token of an `sso_session` profile: the SDK's token provider makes it when the token has expired and can be refreshed, both for GoWatch's own SSO calls and, with no static keys, in the SDK's default credential chain. The provider rewrites its own cache file under `~/.aws/sso/cache/`, as the AWS CLI does; GoWatch's own code never writes to `~/.aws`. For a profile that uses an `aws login` session the SDK may call `signin:CreateOAuth2Token`. Credentials, tokens and account e-mail addresses never appear in the AWS view or in an error text. CloudWatch Logs are not read yet.

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

**Known limitations.** The log level badges (`[INFO]`, `[WARN]`, `[ERROR]`, `[FATAL]`...) are not drawn: the terminal UI library reads text between square brackets as a color tag, so the badge text never appears in the log lines. That is how GoWatch has always drawn them, and showing them would change the default look, so it has not been changed. As a result, `log_error`, `log_warn`, `log_info`, `log_debug` and `log_fatal_text` have no visible effect yet, and `log_fatal` only colors the background of the rest of a FATAL log line. The roles are kept so that a config file that sets them keeps working if the badges become visible. A related case: the root operation of the recent traces (shown whenever there are traces, from the OTLP receiver or from log lines) is drawn in the terminal's default text color in every theme, so it can be hard to read with the `light` theme in a dark terminal, or with `solarized` or `nord` in a light one.

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
| `search` | Focus the search field (searches services and log text) | `/`, `f` | `/`, `f` |
| `cycle_log_level` | Cycle the minimum log level | `l`, `L` | `l`, `L` |
| `export_logs` | Export the logs on screen to a JSON file in the current directory (not while the search field has the focus) | `e`, `E` | `e`, `E` |
| `clear_filter` | Clear all filters, including the ones from the config file (closes the alerts list first when it is open) | `Esc` | `Esc` |
| `toggle_alerts` | Show or hide the alerts list | `w`, `W` | `w`, `W` |
| `clear_resolved_alerts` | Clear the resolved alerts (works on every screen) | `c`, `C` | `c`, `C` |
| `scroll_up` | Scroll up a line | `↑` | `↑`, `k` |
| `scroll_down` | Scroll down a line | `↓` | `↓`, `j` |
| `page_up` | Scroll up a page | `PgUp` | `PgUp`, `Ctrl+U` |
| `page_down` | Scroll down a page | `PgDn` | `PgDn`, `Ctrl+D` |
| `scroll_top` | Jump to the top | `Home` | `Home`, `g` |
| `scroll_bottom` | Jump to the bottom | `End` | `End`, `G` |

The `vim` preset is the `default` preset plus `j`, `k`, `g`, `G`, `Ctrl+D` and `Ctrl+U`. `Ctrl+D` and `Ctrl+U` move a whole page, the height of the logs panel, not half a page as in vim. There is no `emacs` preset. The AWS view (`aws_view`) shows the identity of the active credentials and the accounts of the SSO login: see the AWS notes under Options.

**Keys.** A key is a single character (`q`, `/`, `?`, `G`; upper and lower case are different keys, so `l` and `L` are listed separately), `ctrl+<letter>` (`ctrl+d`), `space`, or one of `esc` (or `escape`), `enter`, `tab`, `up`, `down`, `pgup`, `pgdn`, `home` and `end`. The names are case-insensitive. Quote the keys in YAML, because characters such as `?`, `#`, `,` and `[` mean something to it: `["?", "#"]`. Action names are written in lower case with underscores (`Quit` also works).

**Rules.** An override replaces all the keys of its action, so listing `quit: ["x"]` makes `q` stop quitting; actions you leave out keep the keys of the preset. Each action needs at least one key, a key can run only one action and a key cannot be repeated within an action. To swap the keys of two actions, override both of them. Any problem (an unknown preset, action or key, a clash, `ctrl+c`...) stops GoWatch at startup with a message that lists the valid choices. The rules are checked together, so the message shows every problem at once.

**Keys that cannot be rebound.** `Ctrl+C` always quits GoWatch, wherever you are, and cannot be bound to an action. `ctrl+h`, `ctrl+i` and `ctrl+m` are not accepted either: a terminal sends the same bytes as for Backspace, Tab and Enter, so GoWatch could not tell them apart (use `tab` and `enter`). The arrow keys, `PgUp`, `PgDn`, `Home` and `End` (and `Ctrl+F` and `Ctrl+B`) are handled by the panel with focus, which scrolls with them in every keymap. They are listed in the scroll actions for the help page and the help bar, and can only be bound to the scroll action that matches what they do (`up` to `scroll_up`, and so on). The native keys scroll whichever panel has the focus (the logs, unless the alerts list or the help page is open). The other keys of the scroll actions, such as `j` and `k` in the `vim` preset, scroll the logs, or the alerts list when it is open, or the help page when that is open. Scrolling the logs with the arrow keys, `PgUp`, `PgDn`, `Home` or `End` (`End` included) stops them from following new lines; only a `scroll_bottom` key that is not one of those (`G` in the `vim` preset) makes them follow the newest line again.

**Search field.** While the search field has the focus, every key is typed into it, `?` and the bound letters included.

**Help page.** `?` (the `help` action) opens a page that lists every action by section with the keys in use, so remapped keys show up as you set them, followed by the keys that always work. It opens over the alerts list too, and goes back to it when it closes. The page closes with the keys of the `help` and `quit` actions and with `Esc`, and quitting does not happen while it is open. No other action runs while it is open (the view cannot change behind it, and the search does not open).

**Help bar.** The bar at the bottom and the title of the alerts list show the first key of each action, so they follow your bindings. With the default keys the bar is the original one with one more entry, `? Help`, between `↑↓ Scroll` and `q Quit`.

### Searching and exporting logs

**Search.** Press `/` or `f` and type. The search is case-insensitive and looks at two things:

- The services table lists the containers whose service name, container ID or image contains the text (and that pass the status filter).
- The logs panel shows every log line of those containers, plus the lines of any other container whose text contains the search text. The minimum log level (`l`) applies to every line. With an empty search nothing changes. `Esc` clears the search and every other filter.

**Export.** Press `e` (or `E`) to write the log lines that are on screen, after the search and the minimum log level, to a JSON file in the directory GoWatch was started from (its current working directory). The file is named `gowatch-logs-YYYYMMDD-HHMMSS.json` (local time); if that name is taken, `-1`, `-2` and so on are added before `.json`, and an existing file is never overwritten. The file can be read by its owner only (mode `0600`). It works whether or not the logs panel is hidden, and from the AWS view too. For about 5 seconds the help bar shows `Logs exported to <file name> (<n> lines)`, or `Log export failed: <reason>`, and then goes back to normal. The key is typed into the search field while that field has the focus, so press `Enter` first (`Esc` also leaves the field, but it clears the filters).

```json
{
  "exported_at": "2026-10-17T15:30:45-03:00",
  "filters": {
    "search": "api",
    "min_log_level": "WARN"
  },
  "count": 2,
  "logs": [
    {
      "service": "api",
      "level": "ERROR",
      "line": "connection refused"
    },
    {
      "service": "worker",
      "level": "WARN",
      "line": "retrying the call to the api"
    }
  ]
}
```

Docker monitoring requires zero configuration. The AWS view uses the standard AWS setup (the `AWS_*` environment variables and the files in `~/.aws`) for credentials, region and endpoint; see the AWS notes under Options.

## Use Cases

GoWatch is ideal for:

- **Local Microservices Development**: Monitor multiple services in docker-compose environments
- **Performance Debugging**: Quickly identify which containers are consuming excessive CPU or memory
- **Log Troubleshooting**: Aggregate logs from all services to debug issues across your stack
- **Development Environment Monitoring**: Keep an eye on resource usage while coding and testing
- **Container Health Checks**: Keep an eye on whether your services are running (stopped containers drop out of the list)

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
