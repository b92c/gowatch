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

**Timeline**: 12-18 months for full roadmap completion. Phases 1-2 recommended for near-term focus.

---

## Current State (MVP ✅)

- ✅ Real-time Docker container monitoring (CPU, memory)
- ✅ Terminal UI dashboard with tcell/tview
- ✅ Aggregated container logs
- ✅ System resources panel
- ✅ Zero-configuration auto-detection
- ✅ Docker Compose dev environment

---

## Phase 1: Core Monitoring Enhancements (Q2-Q3 2026)

**P1.1 - Advanced Container Filtering & Search**
- Search containers by name, ID, or image
- Filter by status and docker labels
- Persistent filter state

**P1.2 - Extended Metrics Collection**
- Network metrics (bytes/packets in/out)
- Disk I/O metrics
- Process count and OOM tracking

**P1.3 - Historical Data & Trending**
- Metrics history buffer (5 min - 1 hour)
- Sparkline charts for CPU/memory trends
- Min/max/average metrics display

**P1.4 - Log Management Enhancements**
- Log filtering by container/keyword
- Log level parsing
- Log export functionality

---

## Phase 2: Configuration & Customization (Q3 2026)

**P2.1 - Configuration File Support**
- YAML/TOML config format
- Paths: ~/.gowatch/config.yaml, ./gowatch.yaml
- Settings: update interval, log buffer, filters, shortcuts

**P2.2 - Theme & UI Customization**
- Built-in themes (dark, light, solarized, nord)
- Per-element color customization
- Layout customization

**P2.3 - Keyboard Shortcuts & Help System**
- Vim/Emacs/default modes
- Configurable keybindings
- Interactive help menu

---

## Phase 3: Distributed Tracing (Q4 2026)

**P3.1 - OpenTelemetry Integration**
- Implement trace correlator
- Extract trace context from logs
- Trace exporter base

**P3.2 - Trace Visualization**
- Active traces UI panel
- Trace flamegraph display
- Log-to-span linking

**P3.3 - Trace Export**
- OTLP, Jaeger exporters
- Trace sampling configuration

---

## Phase 4: AWS Serverless Integration (Q1-Q2 2027)

**P4.1 - CloudWatch Logs Integration**
- CloudWatch Log Groups monitoring
- Aggregated log streaming with Docker logs

**P4.2 - Lambda Monitoring**
- Display Lambda metrics (invocations, duration, errors)
- Lambda logs from CloudWatch

**P4.3 - XRay Integration**
- Distributed trace fetching
- Service map display
- Trace correlation

**P4.4 - CloudFormation Monitoring**
- Stack status monitoring
- Resource tracking
- Event timeline

---

## Phase 5: Alerting & Notifications (Q2-Q3 2027)

**P5.1 - Alert System**
- Resource thresholds (CPU, memory, disk I/O)
- Event-based alerts (crash, restart, OOM)
- Log pattern matching

**P5.2 - Notification Channels**
- Terminal, desktop, webhooks
- Slack, Teams, Discord
- Email, PagerDuty, AWS SNS/SQS

**P5.3 - Alert Management**
- Alert history viewer
- Acknowledge/resolve alerts
- Statistics dashboard

---

## Phase 6: Performance & Advanced Features (Q3-Q4 2027)

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

## Phase 7: Container Management (Q4 2027+)

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

**Phases 1-2** (Months 1-5): Immediate focus for MVP+ completeness
**Phase 3** (Months 6-7): Optional for APM integration
**Phases 4-7** (Months 8-18): Extended ecosystem in priority order

---

## Key Dependencies

| Phase | Library | Purpose |
|-------|---------|---------|
| P1.3 | asciigraph | Charts |
| P3 | go.opentelemetry.io/otel | Tracing |
| P4 | aws-sdk-go-v2 | AWS |
| P5 | notify-go | Notifications |
| P6.4 | prometheus/client_golang | Metrics |

---

**Last Updated**: April 2026
**Next Review**: July 2026
