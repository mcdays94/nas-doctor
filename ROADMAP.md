# NAS Doctor Feature Gap Roadmap

Last updated: 2026-04-07

This roadmap captures the highest impact feature gaps identified by comparing NAS Doctor with commonly used homelab and self-hosted tools (Netdata, Uptime Kuma, Scrutiny, Beszel, Glances, Prometheus + Alertmanager, Loki, Gatus, and Checkmk), plus repeated requests seen in homelab forums.

## Prioritization and sizing

- `P1` = highest operator impact, should land next.
- `P2` = high value follow-up after P1 is stable.
- `P3` = valuable, but can wait for a later cycle.
- Sizing assumes one maintainer working part-time:
  - `S`: 2-4 days
  - `M`: 1-2 weeks
  - `L`: 2-4 weeks
  - `XL`: 4+ weeks

## Sequenced milestones

| Priority | Milestone | Size | Why now | Tracking issue |
|---|---|---|---|---|
| P1 | Alerts 2.0 core (routing, dedup, quiet hours, maintenance windows) | M | Biggest trust gap vs mature monitoring stacks. | #10 |
| P1 | Alert workflow UX (ack, snooze, timeline, suppression visibility) | M | Needed so alerts become actionable, not noisy. | #11 |
| P1 | Service checks module (HTTP/TCP/DNS/SMB/NFS/container endpoints) | M | Closes major coverage gap vs Uptime Kuma/Gatus. | #12 |
| P2 | Incident timeline + metrics correlation | L | Speeds up root-cause analysis after incidents. | #13 |
| P2 | SMART and array trend intelligence | L | Adds predictive storage maintenance capabilities. | #14 |
| P3 | Team workflow and external integrations expansion | M | Improves multi-user operations and handoffs. | (to be opened) |

Sequencing: #10 is the foundation, then #11 and #12, followed by #13 and #14.

## P1.1 Alerts 2.0 Core (M)

### Scope

- Add policy-based notification routing by severity/category (and optional hostname match).
- Add cooldown and dedup controls to reduce repeated alert spam.
- Add quiet hours and explicit maintenance windows.
- Track persistent alert lifecycle state in DB (open/resolved, with timestamps).

### Acceptance criteria

- Notification policy can target at least one specific severity and category combination.
- Repeated identical findings do not notify more frequently than configured cooldown.
- Quiet hours prevent notifications while still recording alert state changes.
- Maintenance windows suppress delivery but preserve alert event history.

## P1.2 Alert Workflow UX (M)

### Scope

- Add acknowledge and snooze actions for active alerts.
- Add alert timeline/event log per alert instance.
- Surface suppression reason in UI (`acked`, `snoozed`, `quiet_hours`, `maintenance`).

### Acceptance criteria

- User can acknowledge an alert and see who/when acknowledged.
- User can snooze an alert until a chosen timestamp and unsnooze manually.
- Alert detail page (or panel) shows lifecycle events in chronological order.
- Suppressed alerts remain visible with explicit suppression state.

## P1.3 Service Checks Module (M)

### Scope

- Add configurable checks: HTTP(S), TCP port, DNS resolve, SMB/NFS endpoint reachability.
- Add service check findings to existing analysis and notification pipeline.
- Store service-check history for status trend and flapping detection.

### Acceptance criteria

- Failed service checks generate findings with severity mapping.
- Service checks run on schedule and can be triggered manually.
- UI exposes current status and recent check history for each target.
- At least one anti-flap guard exists (consecutive failures or cooldown window).

## P2.1 Incident Timeline and Metrics Correlation (L)

### Scope

- Add incident timeline view combining findings, notifications, and key metric spikes.
- Add comparative windows (before/during/after) for CPU, memory, I/O wait, and disk temperature.
- Improve retention/downsampling for long-range trend readability.

### Acceptance criteria

- Incident timeline can be filtered by time range and severity.
- User can inspect correlated metric changes around a selected alert.
- Long-range charts remain responsive with downsampled history.

## P2.2 SMART and Array Trend Intelligence (L)

### Scope

- Track SMART attribute deltas and degradation velocity over time.
- Add proactive findings for likely near-term disk replacement needs.
- Add parity/scrub/resilver trend summaries and risk markers.

### Acceptance criteria

- Trend engine flags drives with accelerating error patterns.
- Alerts clearly distinguish static bad state vs worsening state.
- UI includes confidence/urgency guidance for replacement planning.

## P3 Team Workflow and Integrations (M)

### Scope

- Add runbook links per finding type.
- Add richer integration templates (Slack/Discord/Teams/Jira payload variants).
- Add incident export bundles (JSON/PDF) with action history.

### Acceptance criteria

- Each critical finding can include an optional runbook reference.
- Integration payloads include alert state and suppression context.
- Export includes findings, timeline, and notification attempts.
