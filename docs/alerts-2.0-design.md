# Alerts 2.0 Design

Status: Draft  
Owner: NAS Doctor  
Related roadmap item: `P1.1 Alerts 2.0 core` in `ROADMAP.md`

## Problem

Current notification behavior sends findings directly from each scan run to all enabled webhooks that match `min_level`. This works, but it lacks the controls operators expect in production-style monitoring:

- no policy routing beyond `min_level`
- no dedup/cooldown per alert signature
- no maintenance windows or quiet hours
- no alert-level acknowledge/snooze workflow
- limited event history for why a notification was or was not sent

## Goals

- Deliver fewer, higher-signal notifications.
- Persist alert lifecycle state (`open`, `acknowledged`, `snoozed`, `resolved`).
- Add policy-based routing and suppression controls.
- Keep backward compatibility with existing settings and webhooks.
- Reuse current notifier integrations (Discord, Slack, Gotify, ntfy, generic).

## Non-goals (for this phase)

- Multi-user auth/permissions model.
- PagerDuty/Opsgenie-specific providers.
- Full rule engine DSL.

## Current behavior summary

- Scheduler runs `collector -> analyzer -> storage -> notifier` on each interval.
- Notifier filters by webhook `min_level` and sends immediately.
- Notification attempts are logged in `notification_log`.
- `alerts` table exists but is not a full alert lifecycle model.

## Proposed architecture

Add an alert lifecycle and policy layer between analysis and webhook dispatch.

```text
Analyzer findings
  -> fingerprint + lifecycle sync
  -> suppression evaluation (dismissed, ack, snooze, quiet hours, maintenance)
  -> policy routing
  -> dedup/cooldown gate
  -> notifier dispatch
  -> event + delivery logging
```

### 1) Alert fingerprinting

Generate a stable fingerprint per finding using deterministic fields:

- `category`
- `title`
- `related_disk` (if present)
- optional normalized evidence key (small subset only)

This allows the system to recognize the same logical alert across scans.

### 2) Alert lifecycle synchronization

Per scan:

- Upsert active fingerprints as `open` alerts.
- If an `open` fingerprint is missing in current scan, mark `resolved`.
- Track `first_seen_at`, `last_seen_at`, and occurrence count.

### 3) Suppression evaluation

Evaluate in this order:

1. dismissed finding
2. snoozed alert (until timestamp)
3. maintenance window
4. quiet hours
5. acked alert re-notify policy (optional reminder interval)

Suppression never deletes alert state; it only gates delivery.

### 4) Policy routing

Route candidate alerts through ordered policies:

- match: severity/category/hostname (simple exact or list match)
- target: one webhook name
- controls: cooldown, batching window

If no policy matches, fallback to legacy behavior using webhook `min_level`.

### 5) Delivery and event logging

- Delivery attempts continue in `notification_log`.
- Lifecycle actions and decisions are stored in a new `alert_events` table.

## Data model changes

### Extend `alerts` table

Add columns:

- `fingerprint TEXT`
- `status TEXT NOT NULL DEFAULT 'open'` (`open|acknowledged|snoozed|resolved`)
- `first_seen_at DATETIME`
- `last_seen_at DATETIME`
- `occurrences INTEGER NOT NULL DEFAULT 1`
- `acknowledged_at DATETIME`
- `acknowledged_by TEXT`
- `snoozed_until DATETIME`
- `last_notified_at DATETIME`
- `suppression_reason TEXT`

Indexes:

- `idx_alerts_fingerprint_status (fingerprint, status)`
- `idx_alerts_last_seen (last_seen_at DESC)`

### New table: `alert_events`

```sql
CREATE TABLE IF NOT EXISTS alert_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  alert_id INTEGER NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  actor TEXT NOT NULL DEFAULT 'system',
  data JSON,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

Suggested `event_type` values:

- `opened`
- `reopened`
- `resolved`
- `acknowledged`
- `unacknowledged`
- `snoozed`
- `unsnoozed`
- `suppressed`
- `notified`
- `notify_failed`

### New table: `notification_state`

Tracks dedup/cooldown state per route and fingerprint.

```sql
CREATE TABLE IF NOT EXISTS notification_state (
  fingerprint TEXT NOT NULL,
  route_key TEXT NOT NULL,
  last_sent_at DATETIME,
  last_status TEXT,
  PRIMARY KEY (fingerprint, route_key)
);
```

## Settings schema extensions

Extend `SettingsNotifications` in `internal/api/api_extended.go`:

- `policies []AlertPolicy`
- `quiet_hours QuietHours`
- `maintenance_windows []MaintenanceWindow`

Suggested shapes:

```go
type AlertPolicy struct {
  Name          string
  Enabled       bool
  MinSeverity   internal.Severity
  Categories    []internal.Category
  Hostnames     []string
  WebhookName   string
  CooldownSec   int
  BatchWindowSec int
}

type QuietHours struct {
  Enabled   bool
  Timezone  string
  StartHHMM string
  EndHHMM   string
}

type MaintenanceWindow struct {
  Name      string
  Enabled   bool
  StartISO  string
  EndISO    string
  Hostnames []string
}
```

Backward compatibility:

- If `policies` is empty, use existing webhook `min_level` behavior.
- Existing saved settings continue to load with zero-value defaults.

## API changes

Add endpoints:

- `GET /api/v1/alerts?status=open|acknowledged|snoozed|resolved`
- `GET /api/v1/alerts/{id}`
- `GET /api/v1/alerts/{id}/events`
- `POST /api/v1/alerts/{id}/ack`
- `POST /api/v1/alerts/{id}/unack`
- `POST /api/v1/alerts/{id}/snooze` with `{ "until": "RFC3339" }`
- `POST /api/v1/alerts/{id}/unsnooze`

Existing endpoint updates:

- `PUT /api/v1/settings` accepts the new notification fields.
- `GET /api/v1/settings` returns defaults for new fields when missing.

## Scheduler integration

Add a new internal step after `Analyze` and before `NotifyFindings`:

1. Build active alert set from findings (excluding dismissed titles).
2. Sync lifecycle state in DB.
3. Build delivery plan from policies.
4. Apply dedup/cooldown using `notification_state`.
5. Dispatch grouped payloads through existing notifier.
6. Write `alert_events` and `notification_log` records.

## UI plan

Add an Alerts panel/page:

- list filters: `open`, `acknowledged`, `snoozed`, `resolved`
- row actions: `ack`, `snooze`, `unsnooze`
- detail drawer/page: event timeline and suppression reason
- badges indicating why alert was suppressed

## Migration plan

1. Add schema migration helpers that can conditionally add missing columns in SQLite.
2. Add new tables (`alert_events`, `notification_state`).
3. Keep old `alerts` rows readable; backfill `fingerprint` lazily on next sighting.
4. Release with fallback path to legacy `min_level` routing when no policy is configured.

## Testing plan

- Unit tests:
  - fingerprint stability
  - policy matching and precedence
  - quiet-hours and maintenance window logic
  - dedup/cooldown gates
- Storage integration tests:
  - migrations from old DB shape
  - alert lifecycle transitions across snapshots
- API tests:
  - ack/snooze endpoints
  - settings backward compatibility
- End-to-end:
  - fake webhook receiver validates dedup and suppression behavior

## Rollout phases

1. Phase A: DB + API + scheduler lifecycle (no UI actions yet).
2. Phase B: UI actions (`ack`, `snooze`) and alert event timeline.
3. Phase C: policy editor UX and migration of users from simple `min_level` setup.

## Definition of done

- Alert spam is reduced with dedup/cooldown defaults.
- Operators can acknowledge and snooze alerts safely.
- Maintenance windows and quiet hours behave predictably.
- Existing webhook setups continue to work without reconfiguration.
