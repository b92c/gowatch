---
type: doc
name: project-roadmap
description: Implementation roadmap for GoWatch features and enhancements
category: planning
status: active
scaffoldVersion: "2.0.0"
---

# GoWatch Implementation Roadmap

## Executive Summary

GoWatch is transitioning from MVP (core Docker monitoring + TUI dashboard) to a comprehensive monitoring platform. This roadmap outlines 7 implementation phases spanning core enhancements, configuration, distributed tracing, AWS integration, alerting, performance optimization, and container management.

**Timeline**: 12-18 months for full roadmap completion. Phases 1-2 recommended for near-term focus. What is done and what is not is in Current State below.

---

## Current State

Status checked against the code in October 2026. Legend:

- ✅ done
- 🟡 partial, or only the structure exists (the note under the phase or item says which)
- ⚪ not started: nothing usable yet (the note says when placeholder code exists)

**MVP ✅**

- ✅ Real-time Docker container monitoring (CPU, memory)
- ✅ Terminal UI dashboard with tcell/tview
- ✅ Aggregated container logs
- ✅ System resources panel
- ✅ Zero-configuration auto-detection
- ✅ Docker Compose dev environment

**By phase** (the gaps are listed under each phase and item below)

- Phase 1 🟡: P1.2 is done; P1.1, P1.3 and P1.4 have gaps (no log export, no min/max/average display, no status or label filter while running).
- Phase 2 ✅: done, except the Emacs keybinding mode.
- Phase 3 🟡: P3.1 is done; traces from log lines and from the optional OTLP receiver show in the System Resources panel. There is no flamegraph, no log-to-span link and nothing calls the exporter.
- Phase 4 🟡 (structure only): the AWS code returns fixed data and nothing calls it, so the AWS view is always empty.
- Phase 5 🟡: P5.1 works in part (CPU, memory, OOM and restarting containers) and the alerts list works; P5.2 has not started; P5.3 cannot acknowledge alerts or show statistics.
- Phases 6 and 7 ⚪: not started.

---

## Phase 1: Core Monitoring Enhancements (Q2-Q3 2026) 🟡

Phase 1 is partly done: P1.2 is complete, and P1.1, P1.3 and P1.4 each have gaps, listed under the item.

**P1.1 - Advanced Container Filtering & Search** 🟡
- Search containers by name, ID, or image
- Filter by status and docker labels
- Persistent filter state

Status: the search field (`/` or `f`) filters containers by service name, ID or image, and `l` cycles the minimum log level. Initial filters (`filters.search`, `filters.status`, `filters.min_log_level`) come from the config file and are applied at startup; nothing is written to disk while GoWatch runs. Missing: a way to filter by status or by docker label while it runs (the status filter can only be set in the config file, and the label filter in `internal/filter` is not reachable from the UI or the config), and saving filters between runs.

**P1.2 - Extended Metrics Collection** ✅
- Network metrics (bytes/packets in/out)
- Disk I/O metrics
- Process count and OOM tracking

Status: network bytes and packets, disk read/write bytes and operations, current PIDs and an OOM counter are collected and shown as columns of the services table. The OOM column reads the memory failure counter of the container stats (`Failcnt`). The disk operation counts come from a cgroup v1 field, so they show 0 on cgroup v2 hosts.

**P1.3 - Historical Data & Trending** 🟡
- Metrics history buffer (5 min - 1 hour)
- Sparkline charts for CPU/memory trends
- Min/max/average metrics display

Status: the history buffer keeps `history.max_points` samples per container (default 60, which is 2 minutes at the default 2s refresh; accepted range 10-3600). The CPU and memory sparklines are in the services table, and each one draws only the latest 10 samples. Missing: the min/max/average display (`metrics.FormatStatsSummary` exists and is tested, but nothing in the UI calls it).

**P1.4 - Log Management Enhancements** 🟡
- Log filtering by container/keyword
- Log level parsing
- Log export functionality

Status: log lines are parsed into levels (trace to fatal), `l` filters the logs by minimum level, and the search field narrows the logs to the matching containers. Missing: searching for a keyword inside the log text, and log export (nothing writes logs to a file).

---

## Phase 2: Configuration & Customization (Q3 2026) ✅

Phase 2 is complete, except for the Emacs keybinding mode (see P2.3).

**P2.1 - Configuration File Support** ✅
- YAML config format (`go.yaml.in/yaml/v3`, strict: unknown keys are errors)
- Paths: ~/.gowatch/config.yaml, ./gowatch.yaml (later files override earlier ones key by key); `--config <file>` replaces both
- Settings: refresh interval, log tail, history size, initial filters, alert rule overrides, OTLP receiver, AWS
- `--print-default-config` prints a commented example
- Theme and layout settings landed with P2.2; keybinding settings with P2.3

**P2.2 - Theme & UI Customization** ✅
- Built-in themes (dark, light, solarized, nord); `dark` is the original look, and it stays the default
- `--theme <name>` flag and `theme.name`; per-role color overrides under `theme.colors` (color name or `#rrggbb`)
- Basic layout customization: show or hide the resources panel, the logs and the help bar, and set relative panel heights under `layout`

**P2.3 - Keyboard Shortcuts & Help System** ✅ (except the Emacs mode)
- Configurable keybindings: `keybindings.preset` and `keybindings.bindings` in the config file, checked at startup by `ui.BuildKeymap` (unknown preset, action or key, a key on two actions and `ctrl+c`, which always quits, are errors)
- `default` preset (the keys GoWatch has always had, plus `?` for the help page) and `vim` preset (`j`/`k`, `g`/`G`, `Ctrl+D`/`Ctrl+U` on top of `default`)
- Interactive help page on `?`, generated from the active keymap; the help bar and the alerts list title show the keys in use
- Not done: the Emacs mode, which was left out on purpose. `preset: emacs` is a startup error that lists `default` and `vim`

**Numbering of the older commits.** Some commit messages use labels that differ from the phase numbers of this document: `feat: implement distributed tracing ... (P2.0)` is Phase 3 here, `feat: implement aws serverless ... (P3.0)` is Phase 4, and `feat: implement alerting engine ...`, which has no label, is P5.1. The commits tagged P2.1, P2.2 and P2.3 are the Phase 2 items of this document.

---

## Phase 3: Distributed Tracing (Q4 2026) 🟡

Phase 3 is partly done. Traces reach the dashboard from two sources: the log correlator, which is always on and reads trace context from the Docker log lines, and the OTLP receiver, which is off by default (`tracing.otlp_receiver.enabled`). The OpenTelemetry SDK is not used (`go.opentelemetry.io/otel` is only an indirect dependency).

**P3.1 - OpenTelemetry Integration** ✅
- Implement trace correlator
- Extract trace context from logs
- Trace exporter base

Status: the correlator reads trace context from log lines (W3C `traceparent`, JSON `trace_id`/`span_id`, logfmt `trace_id=`/`span_id=`, and `request_id=`) and keeps the traces in memory (up to 500 traces; a trace older than 5 minutes is dropped when the next span arrives). `main` starts the OTLP receiver when `tracing.otlp_receiver.enabled` is true; it accepts OTLP/HTTP JSON on `/v1/traces`. The exporter base type exists, but nothing calls it (see P3.3).

**P3.2 - Trace Visualization** 🟡
- Active traces UI panel
- Trace flamegraph display
- Log-to-span linking

Status: the System Resources panel shows how many traces are active, how many have errors, and the two most recent ones (trace ID, span count, duration, root service and operation). Missing: a flamegraph, log-to-span linking, and a panel of its own for traces.

**P3.3 - Trace Export** ⚪
- OTLP, Jaeger exporters
- Trace sampling configuration

Status: only placeholder code exists. `trace.Exporter` posts a trace to a URL as GoWatch's own JSON (not the OTLP format). It is covered by a test, but no code outside the tests creates or calls it, and no setting configures it. Missing: an exporter that is wired in, an OTLP-format exporter, a Jaeger exporter and sampling configuration. The OTLP receiver is the inbound side only.

---

## Phase 4: AWS Serverless Integration (Q1-Q2 2027) 🟡 (structure only)

Only the structure exists, and none of it is connected to a data source:

- `internal/aws` has a state manager (`AWSClientManager`) and four collectors (CloudWatch, Lambda, X-Ray, CloudFormation). The collectors return fixed sample data, for example the Lambda functions `gowatch-auth-handler` and `gowatch-api-processor`.
- Nothing outside the tests calls the collectors, `SetResources` or `SetConnected`, and `aws-sdk-go-v2` is not in `go.mod`, so GoWatch makes no AWS calls.
- The AWS view (`a`) builds its table from the resources in that state, which is never filled, so the table is always empty (the region shows in its title).
- The `aws.*` settings and the `AWS_*` variables only set the `Enabled`, `Region` and `EndpointURL` fields of that state. The UI reads only `Region`.

**P4.1 - CloudWatch Logs Integration** ⚪
- CloudWatch Log Groups monitoring
- Aggregated log streaming with Docker logs

Status: only placeholder code exists. `CloudWatchCollector` returns fixed `Invocations`, `Errors` and `DurationAvg` values for any group name. No log group is read and nothing is streamed into the logs panel.

**P4.2 - Lambda Monitoring** ⚪
- Display Lambda metrics (invocations, duration, errors)
- Lambda logs from CloudWatch

Status: only placeholder code exists. `LambdaCollector.ListFunctions` returns two fixed functions. No metrics or logs are fetched.

**P4.3 - XRay Integration** ⚪
- Distributed trace fetching
- Service map display
- Trace correlation

Status: only placeholder code exists. `XRayCollector.FetchServiceGraph` returns one fixed service (`api-gateway`). No trace is fetched, and there is no service map and no correlation.

**P4.4 - CloudFormation Monitoring** ⚪
- Stack status monitoring
- Resource tracking
- Event timeline

Status: only placeholder code exists. `CloudFormationCollector.ListStacks` returns one fixed stack (`gowatch-dev-stack`). There is no stack status monitoring, resource tracking or event timeline.

---

## Phase 5: Alerting & Notifications (Q2-Q3 2027) 🟡

Phase 5 is partly done: P5.1 works in part (CPU, memory, OOM and restarting containers) and the alerts list works, P5.2 has not started, and P5.3 has gaps.

**P5.1 - Alert System** 🟡
- Resource thresholds (CPU, memory, disk I/O)
- Event-based alerts (crash, restart, OOM)
- Log pattern matching

Status: rules for CPU and memory thresholds (with hysteresis and consecutive samples), for OOM events and for containers in the `restarting`, `dead` or `exited` state run on every refresh, and the System Resources panel shows the counts. The container list is requested without the "all" option, so stopped containers are not listed and the container-state rule in practice sees only restarting ones. The four built-in rules can be tuned under `alerts.rules` (threshold, hysteresis, consecutive hits, severity, on or off), but new rules cannot be defined. Missing: disk I/O thresholds and log pattern matching.

**P5.2 - Notification Channels** ⚪
- Terminal, desktop, webhooks
- Slack, Teams, Discord
- Email, PagerDuty, AWS SNS/SQS

Status: not started. Alerts are shown only inside the dashboard (the summary in the System Resources panel and the alerts list on `w`); there is no desktop, webhook, chat, email or queue notification.

**P5.3 - Alert Management** 🟡
- Alert history viewer
- Acknowledge/resolve alerts
- Statistics dashboard

Status: the alerts list (`w`) shows the active alerts and the resolved ones until `c` clears them; it lives in memory for the session. `AlertStatus` has an `ACKNOWLEDGED` value and `AlertEngine.AcknowledgeAlert` exists, but nothing calls it, so alerts cannot be acknowledged from the UI. An alert resolves only on its own, when its value recovers past the hysteresis threshold or the container is gone. Missing: acknowledge and resolve actions, and statistics.

---

## Phase 6: Performance & Advanced Features (Q3-Q4 2027) ⚪

Not started: nothing in the code implements these items.

**P6.1 - Performance Optimization**
- Adaptive Docker polling
- Connection pooling
- Memory management for 100+ containers

**P6.2 - Multi-host Monitoring**
- Multiple Docker daemons
- SSH tunnel support
- Unified dashboard

**P6.3 - Audit & Logging**
- Application audit trail
- Debug logging
- Self-metrics

**P6.4 - Export & Reporting**
- Dashboard snapshots (JSON/YAML)
- HTML reports
- REST API and Prometheus endpoint

---

## Phase 7: Container Management (Q4 2027+) ⚪

Not started: GoWatch only reads from Docker (container list, stats, logs and host info), and nothing in the code implements these items.

**P7.1 - Container Control**
- Start/stop/restart from UI
- Execute commands
- Interactive shell

**P7.2 - Resource Management**
- CPU/memory limits adjustment
- Network/volume viewing

**P7.3 - Image Management**
- Build/pull/remove images
- Layer tracking

---

## Implementation Strategy

Original plan, kept as written:

**Phases 1-2** (Months 1-5): Immediate focus for MVP+ completeness
**Phase 3** (Months 6-7): Optional for APM integration
**Phases 4-7** (Months 8-18): Extended ecosystem in priority order

Where things stand (October 2026). This list is status only and does not change the plan above; the quarters in the phase titles and the months above are those of the original plan and have not been revised.

- **Done**: Phase 2 (except the Emacs mode), P1.2 and P3.1.
- **Partly done**: P1.1, P1.3 and P1.4 (Phase 1), P3.2 (Phase 3), P5.1 and P5.3 (Phase 5). What is missing is listed under each item.
- **Structure only**: Phase 4. The AWS view, its state and the `aws` settings exist, but P4.1-P4.4 have only placeholder code (collectors that return fixed data and are not called), so no real AWS data is read yet.
- **Not started**: P3.3 (only placeholder code exists: the exporter is not called), P5.2, Phase 6 and Phase 7.

---

## Key Dependencies

| Phase | Library | Purpose |
|-------|---------|---------|
| P1.3 | asciigraph | Charts |
| P3 | go.opentelemetry.io/otel | Tracing |
| P4 | aws-sdk-go-v2 | AWS |
| P5 | notify-go | Notifications |
| P6.4 | prometheus/client_golang | Metrics |

None of these libraries is a direct dependency yet. The sparklines are drawn by `ui.RenderSparkline`, the OTLP receiver is written in the repository, and `go.opentelemetry.io/otel` is only an indirect dependency in `go.mod`.

---

**Last Updated**: October 2026
**Next Review**: January 2027
