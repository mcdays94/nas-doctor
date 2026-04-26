// Package storage handles SQLite persistence for snapshots, findings, and config.
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database.
type DB struct {
	db     *sql.DB
	path   string
	logger *slog.Logger
}

// Open creates or opens the SQLite database at the given path.
func Open(path string, logger *slog.Logger) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_timeout=5000", path)
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Set pragmas for performance
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA cache_size=-20000", // 20MB cache
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := sqldb.Exec(pragma); err != nil {
			return nil, fmt.Errorf("set pragma: %w", err)
		}
	}

	d := &DB{db: sqldb, path: path, logger: logger}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
}

// DataDir returns the directory containing the database file.
func (d *DB) DataDir() string {
	return filepath.Dir(d.path)
}

// Close closes the database.
func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS snapshots (
			id TEXT PRIMARY KEY,
			timestamp DATETIME NOT NULL,
			duration_seconds REAL NOT NULL,
			data JSON NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS findings (
			id TEXT NOT NULL,
			snapshot_id TEXT NOT NULL REFERENCES snapshots(id),
			severity TEXT NOT NULL,
			category TEXT NOT NULL,
			title TEXT NOT NULL,
			data JSON NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (snapshot_id, id)
		)`,
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value JSON NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS alerts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			finding_id TEXT NOT NULL,
			snapshot_id TEXT NOT NULL,
			severity TEXT NOT NULL,
			title TEXT NOT NULL,
			notified_at DATETIME,
			resolved_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_snapshots_timestamp ON snapshots(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_findings_severity ON findings(severity)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_resolved ON alerts(resolved_at)`,

		// --- SMART history ---
		`CREATE TABLE IF NOT EXISTS smart_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
			device TEXT NOT NULL,
			serial TEXT NOT NULL,
			model TEXT NOT NULL,
			temperature INTEGER,
			reallocated INTEGER DEFAULT 0,
			pending INTEGER DEFAULT 0,
			offline_uncorrectable INTEGER DEFAULT 0,
			udma_crc INTEGER DEFAULT 0,
			command_timeout INTEGER DEFAULT 0,
			power_on_hours INTEGER DEFAULT 0,
			health_passed BOOLEAN DEFAULT 1,
			timestamp DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_smart_history_serial ON smart_history(serial, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_smart_history_device ON smart_history(device, timestamp DESC)`,

		// --- System history ---
		`CREATE TABLE IF NOT EXISTS system_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
			cpu_usage REAL,
			mem_percent REAL,
			io_wait REAL,
			load_avg_1 REAL,
			load_avg_5 REAL,
			load_avg_15 REAL,
			uptime_seconds INTEGER,
			timestamp DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_system_history_ts ON system_history(timestamp DESC)`,

		// --- GPU history ---
		`CREATE TABLE IF NOT EXISTS gpu_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
			gpu_index INTEGER NOT NULL,
			name TEXT,
			vendor TEXT,
			usage_pct REAL,
			mem_used_mb REAL,
			mem_total_mb REAL,
			mem_pct REAL,
			temperature INTEGER,
			power_watts REAL,
			fan_pct REAL,
			encoder_pct REAL,
			decoder_pct REAL,
			timestamp DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gpu_history_ts ON gpu_history(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_gpu_history_gpu ON gpu_history(gpu_index, timestamp DESC)`,

		// --- Container stats history ---
		// snapshot_id is intentionally NOT a REFERENCES FK: SaveContainerStats
		// is called from the lightweight 5-minute collection loop with a
		// synthetic "cstats-<ms>" ID that has no matching snapshots row.
		// PruneSnapshots uses explicit DELETE (see PR #151 precedent) to
		// clean up scan-captured entries, so the FK is not needed.
		`CREATE TABLE IF NOT EXISTS container_stats_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id TEXT NOT NULL,
			container_id TEXT NOT NULL,
			name TEXT NOT NULL,
			image TEXT,
			cpu_pct REAL,
			mem_mb REAL,
			mem_pct REAL,
			net_in_bytes REAL,
			net_out_bytes REAL,
			block_read_bytes REAL,
			block_write_bytes REAL,
			timestamp DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_container_stats_ts ON container_stats_history(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_container_stats_name ON container_stats_history(name, timestamp DESC)`,

		// --- Speed test history ---
		// See note on container_stats_history: snapshot_id is NOT a FK for
		// the same reason. SaveSpeedTest gets "speedtest-<ts>" synthetic IDs
		// from the scheduler when the test runs outside a scan.
		// engine is the speed-test engine that produced the row. Closed
		// set: 'speedtest_go' (the showwin/speedtest-go primary path
		// introduced in PRD #283 / issue #284) or 'ookla_cli' (the
		// bundled Ookla CLI fallback). Default 'ookla_cli' back-fills
		// pre-#284 rows so the historical chart can mark the
		// engine-switchover point on upgrade.
		`CREATE TABLE IF NOT EXISTS speedtest_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id TEXT NOT NULL,
			download_mbps REAL,
			upload_mbps REAL,
			latency_ms REAL,
			jitter_ms REAL,
			server_name TEXT,
			isp TEXT,
			timestamp DATETIME NOT NULL,
			engine TEXT NOT NULL DEFAULT 'ookla_cli',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_speedtest_ts ON speedtest_history(timestamp DESC)`,

		// --- Speed test per-sample telemetry (PRD #283 / issue #286) ---
		// Slice 3 of the live-progress PRD. Captures the per-sample
		// throughput stream emitted by the runner during a test, bulk-
		// inserted in one transaction at completion (see scheduler's
		// handleSpeedTestResult). One row per sample emitted by the
		// engine, ~30 rows for a typical 30-60s test.
		//
		//   test_id      → speedtest_history.id (the parent row)
		//   sample_index → 0-based monotonic counter assigned by the
		//                  bulk-insert; preserves emission order on
		//                  read so the mini-chart renders left-to-right.
		//   phase        → "latency" / "download" / "upload"
		//   ts           → wall-clock time the sample was emitted.
		//   mbps         → throughput Mbps (zero for latency-phase).
		//   latency_ms   → ping latency ms (zero for throughput-phase).
		//
		// FK ON DELETE CASCADE means samples are pruned automatically
		// whenever the parent speedtest_history row is pruned by the
		// retention loop — no separate retention knob needed.
		`CREATE TABLE IF NOT EXISTS speedtest_samples (
			test_id INTEGER NOT NULL,
			sample_index INTEGER NOT NULL,
			phase TEXT NOT NULL,
			ts TIMESTAMP NOT NULL,
			mbps REAL,
			latency_ms REAL,
			PRIMARY KEY (test_id, sample_index),
			FOREIGN KEY (test_id) REFERENCES speedtest_history(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS speedtest_samples_test_id ON speedtest_samples(test_id)`,

		// --- Last speed-test attempt (single-row table, see issue #210) ---
		// Records the outcome of the most recent speed-test run — success,
		// failure, pending, or disabled. Consumed by the scheduled
		// type=speed service check (option B, reads attempt state + latest
		// speedtest_history row instead of running Ookla per-check) and by
		// the dashboard widget to render "Running initial speed test…"
		// when no history has been produced yet. id=1 is enforced so the
		// table never grows beyond a single row.
		`CREATE TABLE IF NOT EXISTS speedtest_attempt (
			id INTEGER PRIMARY KEY,
			timestamp DATETIME NOT NULL,
			status TEXT NOT NULL,
			error_msg TEXT
		)`,

		// --- Notification log ---
		`CREATE TABLE IF NOT EXISTS notification_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			webhook_name TEXT NOT NULL,
			webhook_type TEXT NOT NULL,
			status TEXT NOT NULL,
			findings_count INTEGER,
			error_message TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_notification_log_ts ON notification_log(created_at DESC)`,

		// --- Notification dedup state ---
		`CREATE TABLE IF NOT EXISTS notification_state (
			fingerprint TEXT NOT NULL,
			route_key TEXT NOT NULL,
			last_sent_unix INTEGER,
			last_status TEXT,
			PRIMARY KEY (fingerprint, route_key)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_notification_state_route ON notification_state(route_key)`,

		// --- Alert lifecycle events ---
		`CREATE TABLE IF NOT EXISTS alert_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			alert_id INTEGER NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
			event_type TEXT NOT NULL,
			actor TEXT NOT NULL DEFAULT 'system',
			data JSON,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_events_alert ON alert_events(alert_id, created_at DESC)`,

		// --- Service checks ---
		`CREATE TABLE IF NOT EXISTS service_checks_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			check_key TEXT NOT NULL,
			name TEXT NOT NULL,
			check_type TEXT NOT NULL,
			target TEXT NOT NULL,
			status TEXT NOT NULL,
			response_ms INTEGER,
			error_message TEXT,
			consecutive_failures INTEGER NOT NULL DEFAULT 0,
			failure_threshold INTEGER NOT NULL DEFAULT 1,
			failure_severity TEXT NOT NULL DEFAULT 'warning',
			checked_at DATETIME NOT NULL,
			details_json TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_service_checks_key_ts ON service_checks_history(check_key, checked_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_service_checks_ts ON service_checks_history(checked_at DESC)`,
		// --- Process history ---
		`CREATE TABLE IF NOT EXISTS process_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id TEXT NOT NULL,
			pid INTEGER NOT NULL,
			user TEXT,
			name TEXT NOT NULL,
			command TEXT,
			container_name TEXT DEFAULT '',
			container_id TEXT DEFAULT '',
			cpu_pct REAL,
			mem_pct REAL,
			timestamp DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_process_history_ts ON process_history(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_process_history_name ON process_history(name, container_name, timestamp DESC)`,

		// Disk usage history for capacity forecasting
		`CREATE TABLE IF NOT EXISTS disk_usage_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			mount_point TEXT NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			device TEXT NOT NULL DEFAULT '',
			total_gb REAL NOT NULL,
			used_gb REAL NOT NULL,
			free_gb REAL NOT NULL,
			used_pct REAL NOT NULL,
			timestamp DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_disk_usage_mount_ts ON disk_usage_history(mount_point, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_disk_usage_ts ON disk_usage_history(timestamp DESC)`,

		// --- Drive maintenance events (issue #130) ---
		// Per-slot chronological timeline of events for a drive. Two types:
		//   "note"        — user-entered freeform content. is_auto=0, mutable.
		//   "replacement" — system-detected when a slot's serial changes
		//                   between scans. Content is JSON {old_serial,
		//                   old_model, new_serial, new_model}. is_auto=1,
		//                   immutable (Update/Delete return 403).
		// slot_key is the ArraySlot on Unraid (stable across replacements)
		// or the Serial on platforms without physical-slot identity.
		`CREATE TABLE IF NOT EXISTS drive_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			slot_key TEXT NOT NULL,
			platform TEXT NOT NULL DEFAULT '',
			event_type TEXT NOT NULL,
			event_time DATETIME NOT NULL,
			content TEXT NOT NULL DEFAULT '',
			is_auto INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_drive_events_slot_ts ON drive_events(slot_key, event_time DESC)`,

		// --- Drive slot state (issue #130, replacement detection) ---
		// Tracks the last-observed serial+model per slot_key so the SMART
		// collector can detect when a slot's physical drive changes.
		// Separate from smart_history because the detection logic should
		// be snapshot-agnostic (it compares the current cycle against the
		// persisted last-observed state, not against an arbitrary slice
		// of history rows). A single row per slot; UPSERT semantics.
		`CREATE TABLE IF NOT EXISTS drive_slot_state (
			slot_key TEXT PRIMARY KEY,
			serial TEXT NOT NULL,
			model TEXT NOT NULL DEFAULT '',
			platform TEXT NOT NULL DEFAULT '',
			observed_at DATETIME NOT NULL
		)`,
	}

	for _, m := range migrations {
		if _, err := d.db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, m)
		}
	}

	if err := d.ensureColumn("alerts", "fingerprint", "TEXT"); err != nil {
		return fmt.Errorf("ensure alerts.fingerprint: %w", err)
	}
	if err := d.ensureColumn("alerts", "acknowledged_at", "DATETIME"); err != nil {
		return fmt.Errorf("ensure alerts.acknowledged_at: %w", err)
	}
	if err := d.ensureColumn("alerts", "acknowledged_by", "TEXT"); err != nil {
		return fmt.Errorf("ensure alerts.acknowledged_by: %w", err)
	}
	if err := d.ensureColumn("alerts", "snoozed_until", "DATETIME"); err != nil {
		return fmt.Errorf("ensure alerts.snoozed_until: %w", err)
	}
	// details_json holds a per-type diagnostic JSON blob (HTTP status_code,
	// DNS records, Ping rtt_ms, TCP resolved_address, failure_stage, …)
	// persisted by the scheduler so the /service-checks log UI can render
	// the same rich context the Test button already shows. Legacy rows
	// pre-dating this column read back as NULL → Details == nil. A single
	// TEXT column (vs per-key columns) keeps schema churn zero when new
	// detail keys get added. See issue #182.
	if err := d.ensureColumn("service_checks_history", "details_json", "TEXT"); err != nil {
		return fmt.Errorf("ensure service_checks_history.details_json: %w", err)
	}
	// speedtest_history_id links a type=speed service-check log row to
	// the speedtest_history row that produced it. Lets the
	// /service-checks expanded-log-row UI fetch per-sample data via
	// /api/v1/speedtest/samples/{id} without timestamp-fuzzy matching.
	// NULL on legacy rows + non-speed types — the UI renders the
	// "no per-sample data available" empty state in that case. Issue
	// #286 / PRD #283 slice 3.
	if err := d.ensureColumn("service_checks_history", "speedtest_history_id", "INTEGER"); err != nil {
		return fmt.Errorf("ensure service_checks_history.speedtest_history_id: %w", err)
	}
	if _, err := d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_alerts_fingerprint_open ON alerts(fingerprint, resolved_at)`); err != nil {
		return fmt.Errorf("create alerts fingerprint index: %w", err)
	}
	if _, err := d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_alerts_ack ON alerts(acknowledged_at)`); err != nil {
		return fmt.Errorf("create alerts ack index: %w", err)
	}
	if _, err := d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_alerts_snooze ON alerts(snoozed_until)`); err != nil {
		return fmt.Errorf("create alerts snooze index: %w", err)
	}

	// Issue #155: drop the snapshots(id) FK from container_stats_history and
	// speedtest_history for existing databases. The CREATE TABLE statements
	// above no longer define the FK for fresh installs, but existing DBs
	// keep the old constraint until we rebuild the tables. SQLite has no
	// ALTER TABLE ... DROP CONSTRAINT, so we rename → create-fresh → copy
	// → drop guarded by a PRAGMA foreign_key_list probe (idempotent).
	if err := d.dropSnapshotFKIfPresent("container_stats_history"); err != nil {
		return fmt.Errorf("drop FK on container_stats_history: %w", err)
	}
	if err := d.dropSnapshotFKIfPresent("speedtest_history"); err != nil {
		return fmt.Errorf("drop FK on speedtest_history: %w", err)
	}

	// PRD #283 / issue #284: speedtest_history.engine is purely
	// additive. Existing DBs predating #284 get the column added via
	// ALTER TABLE with the default 'ookla_cli' which back-fills every
	// pre-switchover row at one stroke. The CREATE TABLE statement
	// above already declares the column for fresh installs, so this
	// no-ops on first launch of a brand-new install.
	if err := d.ensureColumn("speedtest_history", "engine", "TEXT NOT NULL DEFAULT 'ookla_cli'"); err != nil {
		return fmt.Errorf("ensure speedtest_history.engine: %w", err)
	}

	return nil
}

// dropSnapshotFKIfPresent rebuilds the given table without its
// `snapshot_id REFERENCES snapshots(id)` foreign key if one is currently
// defined. No-op on fresh installs (where the CREATE TABLE above already
// omits the FK) and on second calls. Data is preserved by copying rows
// from the renamed old table before dropping it.
func (d *DB) dropSnapshotFKIfPresent(table string) error {
	// Probe for an FK that targets the snapshots table.
	rows, err := d.db.Query(fmt.Sprintf(`PRAGMA foreign_key_list(%s)`, table))
	if err != nil {
		return fmt.Errorf("pragma foreign_key_list(%s): %w", table, err)
	}
	hasSnapshotFK := false
	for rows.Next() {
		var (
			id, seq         int
			tableRef        string
			from, to        string
			onUpdate, onDel string
			match           string
		)
		if err := rows.Scan(&id, &seq, &tableRef, &from, &to, &onUpdate, &onDel, &match); err != nil {
			rows.Close()
			return fmt.Errorf("scan foreign_key_list(%s): %w", table, err)
		}
		if tableRef == "snapshots" && from == "snapshot_id" {
			hasSnapshotFK = true
		}
	}
	rows.Close()
	if !hasSnapshotFK {
		return nil
	}

	// Per-table rebuild. We do NOT parameterise the CREATE with a helper —
	// explicit SQL per table keeps the migration greppable and removes any
	// doubt about column lists drifting from the migration section above.
	switch table {
	case "container_stats_history":
		return d.rebuildTable(
			table,
			`CREATE TABLE container_stats_history (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				snapshot_id TEXT NOT NULL,
				container_id TEXT NOT NULL,
				name TEXT NOT NULL,
				image TEXT,
				cpu_pct REAL,
				mem_mb REAL,
				mem_pct REAL,
				net_in_bytes REAL,
				net_out_bytes REAL,
				block_read_bytes REAL,
				block_write_bytes REAL,
				timestamp DATETIME NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
			[]string{
				`CREATE INDEX IF NOT EXISTS idx_container_stats_ts ON container_stats_history(timestamp DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_container_stats_name ON container_stats_history(name, timestamp DESC)`,
			},
			`id, snapshot_id, container_id, name, image, cpu_pct, mem_mb, mem_pct, net_in_bytes, net_out_bytes, block_read_bytes, block_write_bytes, timestamp, created_at`,
		)
	case "speedtest_history":
		// engine column included so the rebuild-old-FK path (issue #155
		// hangover from before the engine column was added in #284)
		// preserves the new column. The COALESCE on the SELECT side is
		// required when the source table predates #284 — old DBs being
		// rebuilt for the first time AFTER #284 ships will rebuild
		// straight from a no-engine table.
		return d.rebuildTable(
			table,
			`CREATE TABLE speedtest_history (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				snapshot_id TEXT NOT NULL,
				download_mbps REAL,
				upload_mbps REAL,
				latency_ms REAL,
				jitter_ms REAL,
				server_name TEXT,
				isp TEXT,
				timestamp DATETIME NOT NULL,
				engine TEXT NOT NULL DEFAULT 'ookla_cli',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
			[]string{
				`CREATE INDEX IF NOT EXISTS idx_speedtest_ts ON speedtest_history(timestamp DESC)`,
			},
			`id, snapshot_id, download_mbps, upload_mbps, latency_ms, jitter_ms, server_name, isp, timestamp, created_at`,
		)
	}
	return fmt.Errorf("dropSnapshotFKIfPresent: no rebuild recipe for table %q", table)
}

// rebuildTable implements the SQLite FK-dropping recipe for a single table:
// rename old → create new → copy rows → drop old → recreate indexes. All
// steps run in a single transaction so a failure rolls back cleanly.
func (d *DB) rebuildTable(table, createSQL string, indexSQLs []string, copyCols string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(fmt.Sprintf(`ALTER TABLE %s RENAME TO %s_old_fk`, table, table)); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	if _, err := tx.Exec(createSQL); err != nil {
		return fmt.Errorf("create new: %w", err)
	}
	if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO %s (%s) SELECT %s FROM %s_old_fk`, table, copyCols, copyCols, table)); err != nil {
		return fmt.Errorf("copy rows: %w", err)
	}
	if _, err := tx.Exec(fmt.Sprintf(`DROP TABLE %s_old_fk`, table)); err != nil {
		return fmt.Errorf("drop old: %w", err)
	}
	for _, ix := range indexSQLs {
		if _, err := tx.Exec(ix); err != nil {
			return fmt.Errorf("recreate index: %w", err)
		}
	}
	return tx.Commit()
}

func (d *DB) ensureColumn(table, column, definition string) error {
	rows, err := d.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if strings.EqualFold(name, column) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = d.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

// SaveSnapshot stores a complete diagnostic snapshot.
func (d *DB) SaveSnapshot(snap *internal.Snapshot) error {
	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"INSERT INTO snapshots (id, timestamp, duration_seconds, data) VALUES (?, ?, ?, ?)",
		snap.ID, snap.Timestamp, snap.Duration, string(data),
	)
	if err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}

	// Store findings individually for efficient querying
	for _, f := range snap.Findings {
		fData, _ := json.Marshal(f)
		_, err = tx.Exec(
			"INSERT INTO findings (id, snapshot_id, severity, category, title, data) VALUES (?, ?, ?, ?, ?, ?)",
			f.ID, snap.ID, f.Severity, f.Category, f.Title, string(fData),
		)
		if err != nil {
			return fmt.Errorf("insert finding: %w", err)
		}
	}

	// Store SMART history for trend analysis
	if err := d.saveSMARTHistory(tx, snap); err != nil {
		return fmt.Errorf("save smart history: %w", err)
	}

	// Store system metrics history
	if err := d.saveSystemHistory(tx, snap); err != nil {
		return fmt.Errorf("save system history: %w", err)
	}

	// Store disk usage history for capacity forecasting
	if err := d.saveDiskUsageHistory(tx, snap); err != nil {
		return fmt.Errorf("save disk usage history: %w", err)
	}

	// Store GPU history
	if err := d.saveGPUHistory(tx, snap); err != nil {
		return fmt.Errorf("save gpu history: %w", err)
	}

	// Store container stats history
	if err := d.saveContainerHistory(tx, snap); err != nil {
		return fmt.Errorf("save container history: %w", err)
	}

	// Store speed test history
	if err := d.saveSpeedTestHistory(tx, snap); err != nil {
		return fmt.Errorf("save speedtest history: %w", err)
	}

	return tx.Commit()
}

// saveSMARTHistory inserts a row per drive into smart_history within the given transaction.
func (d *DB) saveSMARTHistory(tx *sql.Tx, snap *internal.Snapshot) error {
	for _, s := range snap.SMART {
		_, err := tx.Exec(
			`INSERT INTO smart_history (snapshot_id, device, serial, model, temperature, reallocated, pending, offline_uncorrectable, udma_crc, command_timeout, power_on_hours, health_passed, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			snap.ID, s.Device, s.Serial, s.Model, s.Temperature, s.Reallocated, s.Pending, s.Offline, s.UDMACRC, s.CommandTimeout, s.PowerOnHours, s.HealthPassed, snap.Timestamp,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// saveSystemHistory inserts a single system metrics row within the given transaction.
func (d *DB) saveSystemHistory(tx *sql.Tx, snap *internal.Snapshot) error {
	_, err := tx.Exec(
		`INSERT INTO system_history (snapshot_id, cpu_usage, mem_percent, io_wait, load_avg_1, load_avg_5, load_avg_15, uptime_seconds, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.System.CPUUsage, snap.System.MemPercent, snap.System.IOWait,
		snap.System.LoadAvg1, snap.System.LoadAvg5, snap.System.LoadAvg15, snap.System.UptimeSecs, snap.Timestamp,
	)
	return err
}

func (d *DB) saveDiskUsageHistory(tx *sql.Tx, snap *internal.Snapshot) error {
	for _, disk := range snap.Disks {
		if disk.MountPoint == "" || disk.TotalGB <= 1 || disk.MountPoint[0] != '/' {
			continue // skip virtual/empty/non-absolute mounts
		}
		_, err := tx.Exec(
			`INSERT INTO disk_usage_history (mount_point, label, device, total_gb, used_gb, free_gb, used_pct, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			disk.MountPoint, disk.Label, disk.Device,
			disk.TotalGB, disk.UsedGB, disk.FreeGB, disk.UsedPct, snap.Timestamp,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) saveGPUHistory(tx *sql.Tx, snap *internal.Snapshot) error {
	if snap.GPU == nil || !snap.GPU.Available {
		return nil
	}
	for _, g := range snap.GPU.GPUs {
		_, err := tx.Exec(
			`INSERT INTO gpu_history (snapshot_id, gpu_index, name, vendor, usage_pct, mem_used_mb, mem_total_mb, mem_pct, temperature, power_watts, fan_pct, encoder_pct, decoder_pct, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			snap.ID, g.Index, g.Name, g.Vendor,
			g.UsagePct, g.MemUsedMB, g.MemTotalMB, g.MemPct,
			g.Temperature, g.PowerW, g.FanPct,
			g.EncoderPct, g.DecoderPct, snap.Timestamp,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) saveContainerHistory(tx *sql.Tx, snap *internal.Snapshot) error {
	if !snap.Docker.Available {
		return nil
	}
	for _, c := range snap.Docker.Containers {
		if c.State != "running" {
			continue // only track running containers
		}
		_, err := tx.Exec(
			`INSERT INTO container_stats_history (snapshot_id, container_id, name, image, cpu_pct, mem_mb, mem_pct, net_in_bytes, net_out_bytes, block_read_bytes, block_write_bytes, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			snap.ID, c.ID, c.Name, c.Image,
			c.CPU, c.MemMB, c.MemPct,
			c.NetIn, c.NetOut, c.BlockRead, c.BlockWrite,
			snap.Timestamp,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// SaveContainerStats saves a standalone container stats snapshot (not tied to a full scan).
// Used by the lightweight container stats collection loop.
func (d *DB) SaveContainerStats(docker *internal.DockerInfo) error {
	if docker == nil || !docker.Available {
		return nil
	}
	now := time.Now()
	snapshotID := fmt.Sprintf("cstats-%d", now.UnixMilli())
	for _, c := range docker.Containers {
		if c.State != "running" {
			continue
		}
		_, err := d.db.Exec(
			`INSERT INTO container_stats_history (snapshot_id, container_id, name, image, cpu_pct, mem_mb, mem_pct, net_in_bytes, net_out_bytes, block_read_bytes, block_write_bytes, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			snapshotID, c.ID, c.Name, c.Image,
			c.CPU, c.MemMB, c.MemPct,
			c.NetIn, c.NetOut, c.BlockRead, c.BlockWrite,
			now,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// ContainerHistoryPoint represents a single time-series data point for a container.
type ContainerHistoryPoint struct {
	Timestamp  time.Time `json:"timestamp"`
	Name       string    `json:"name"`
	Image      string    `json:"image"`
	CPUPct     float64   `json:"cpu_percent"`
	MemMB      float64   `json:"mem_mb"`
	MemPct     float64   `json:"mem_percent"`
	NetIn      float64   `json:"net_in_bytes"`
	NetOut     float64   `json:"net_out_bytes"`
	BlockRead  float64   `json:"block_read_bytes"`
	BlockWrite float64   `json:"block_write_bytes"`
}

// GetAvgTempDuringRange returns the average and max temperature across all drives
// for a given time range. Used to compute array temps during parity checks.
func (d *DB) GetAvgTempDuringRange(start, end time.Time) (avg float64, max float64, err error) {
	row := d.db.QueryRow(
		`SELECT COALESCE(AVG(temperature), 0), COALESCE(MAX(temperature), 0)
		 FROM smart_history
		 WHERE timestamp BETWEEN ? AND ? AND temperature > 0`,
		start, end,
	)
	err = row.Scan(&avg, &max)
	return
}

// GetContainerHistory returns container stats history for the given time range.
func (d *DB) GetContainerHistory(hours int) ([]ContainerHistoryPoint, error) {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	rows, err := d.db.Query(
		`SELECT timestamp, name, image, cpu_pct, mem_mb, mem_pct, net_in_bytes, net_out_bytes, block_read_bytes, block_write_bytes
		 FROM container_stats_history
		 WHERE timestamp >= ?
		 ORDER BY name ASC, timestamp ASC`,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []ContainerHistoryPoint
	for rows.Next() {
		var p ContainerHistoryPoint
		if err := rows.Scan(&p.Timestamp, &p.Name, &p.Image, &p.CPUPct, &p.MemMB, &p.MemPct, &p.NetIn, &p.NetOut, &p.BlockRead, &p.BlockWrite); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// ProcessHistoryPoint represents a single time-series data point for a process.
type ProcessHistoryPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	PID           int       `json:"pid"`
	User          string    `json:"user"`
	Name          string    `json:"name"`
	Command       string    `json:"command"`
	ContainerName string    `json:"container_name"`
	ContainerID   string    `json:"container_id"`
	CPUPct        float64   `json:"cpu_percent"`
	MemPct        float64   `json:"mem_percent"`
}

// processName extracts a short process name from a full command string.
// e.g. "/usr/bin/python3 app.py" → "python3", "nginx: worker" → "nginx:"
func processName(command string) string {
	if command == "" {
		return ""
	}
	// Take the first field (the executable path/name).
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	exe := fields[0]
	// Strip leading path: /usr/bin/python3 → python3
	if idx := strings.LastIndex(exe, "/"); idx >= 0 && idx < len(exe)-1 {
		exe = exe[idx+1:]
	}
	return exe
}

// SaveProcessStats saves a standalone process stats snapshot at the current time.
func (d *DB) SaveProcessStats(procs []internal.ProcessInfo) error {
	return d.SaveProcessStatsAt(procs, time.Now())
}

// SaveProcessStatsAt saves a standalone process stats snapshot at the given timestamp.
// Used by the lightweight process stats collection loop (similar to SaveContainerStats)
// and by demo mode for seeding historical data.
func (d *DB) SaveProcessStatsAt(procs []internal.ProcessInfo, ts time.Time) error {
	if len(procs) == 0 {
		return nil
	}
	snapshotID := fmt.Sprintf("pstats-%d", ts.UnixMilli())
	for _, p := range procs {
		name := processName(p.Command)
		if name == "" {
			continue // skip processes with empty name
		}
		_, err := d.db.Exec(
			`INSERT INTO process_history (snapshot_id, pid, user, name, command, container_name, container_id, cpu_pct, mem_pct, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			snapshotID, p.PID, p.User, name, p.Command, p.ContainerName, p.ContainerID, p.CPU, p.Mem, ts,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetProcessHistory returns process history for the given time range.
func (d *DB) GetProcessHistory(hours int) ([]ProcessHistoryPoint, error) {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	rows, err := d.db.Query(
		`SELECT timestamp, pid, COALESCE(user, ''), name, COALESCE(command, ''), COALESCE(container_name, ''), COALESCE(container_id, ''), cpu_pct, mem_pct
		 FROM process_history
		 WHERE timestamp >= ?
		 ORDER BY name ASC, container_name ASC, timestamp ASC`,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []ProcessHistoryPoint
	for rows.Next() {
		var p ProcessHistoryPoint
		if err := rows.Scan(&p.Timestamp, &p.PID, &p.User, &p.Name, &p.Command, &p.ContainerName, &p.ContainerID, &p.CPUPct, &p.MemPct); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

func (d *DB) saveSpeedTestHistory(tx *sql.Tx, snap *internal.Snapshot) error {
	if snap.SpeedTest == nil || !snap.SpeedTest.Available || snap.SpeedTest.Latest == nil {
		return nil
	}
	r := snap.SpeedTest.Latest
	_, err := tx.Exec(
		`INSERT INTO speedtest_history (snapshot_id, download_mbps, upload_mbps, latency_ms, jitter_ms, server_name, isp, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.ID, r.DownloadMbps, r.UploadMbps, r.LatencyMs, r.JitterMs, r.ServerName, r.ISP, snap.Timestamp,
	)
	return err
}

// SpeedTestHistoryPoint represents a single speed test history data point.
type SpeedTestHistoryPoint struct {
	// ID is the speedtest_history.id primary key. Surfaced for callers
	// that need to correlate per-sample data (speedtest_samples.test_id)
	// to the parent history row, notably the type=speed service-check
	// dispatch which stamps service_checks_history.speedtest_history_id
	// so the /service-checks expanded-log mini-chart can fetch
	// /api/v1/speedtest/samples/{id}. Zero for fake/test points where
	// the row was never persisted via SaveSpeedTestReturningID.
	// Issue #286.
	ID           int64     `json:"id,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	DownloadMbps float64   `json:"download_mbps"`
	UploadMbps   float64   `json:"upload_mbps"`
	LatencyMs    float64   `json:"latency_ms"`
	JitterMs     float64   `json:"jitter_ms"`
	ServerName   string    `json:"server_name"`
	ISP          string    `json:"isp"`
	// Engine identifies the speed-test engine that produced the row
	// ("speedtest_go" or "ookla_cli"). Pre-#284 rows back-fill via
	// the column default. See PRD #283 user story 15.
	Engine string `json:"engine,omitempty"`
}

// SpeedTestSample is a single per-tick datum captured during a speed
// test, persisted into speedtest_samples after the test completes.
// Mirrors collector.SpeedTestSample (the in-memory wire shape) plus
// the parent test_id linkage. PRD #283 / issue #286.
type SpeedTestSample struct {
	// SampleIndex is the 0-based emission-order counter. Stored as
	// the PK alongside test_id so re-inserts are caught by SQLite
	// rather than producing duplicate rows.
	SampleIndex int       `json:"sample_index"`
	Phase       string    `json:"phase"`
	Timestamp   time.Time `json:"ts"`
	Mbps        float64   `json:"mbps"`
	LatencyMs   float64   `json:"latency_ms"`
}

// SaveSpeedTest inserts a single speed test result row. The engine
// field falls back to "ookla_cli" via the column default when the
// caller didn't stamp result.Engine — this preserves backwards
// compatibility for any callsite that pre-dates issue #284.
func (d *DB) SaveSpeedTest(snapshotID string, result *internal.SpeedTestResult) error {
	_, err := d.SaveSpeedTestReturningID(snapshotID, result)
	return err
}

// SaveSpeedTestReturningID is the same insert as SaveSpeedTest but
// returns the new speedtest_history.id so the caller can wire it to
// the per-sample rows in speedtest_samples (FK target). Returns
// (0, nil) when result is nil so the empty-result path stays a no-op.
// Added for PRD #283 slice 3 / issue #286 — the runner-driven path
// in scheduler.handleSpeedTestResult needs the ID to bulk-insert
// in-memory samples buffered by the LiveTest.
func (d *DB) SaveSpeedTestReturningID(snapshotID string, result *internal.SpeedTestResult) (int64, error) {
	if result == nil {
		return 0, nil
	}
	engine := result.Engine
	if engine == "" {
		engine = internal.SpeedTestEngineOoklaCLI
	}
	res, err := d.db.Exec(
		`INSERT INTO speedtest_history (snapshot_id, download_mbps, upload_mbps, latency_ms, jitter_ms, server_name, isp, timestamp, engine)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshotID, result.DownloadMbps, result.UploadMbps, result.LatencyMs, result.JitterMs, result.ServerName, result.ISP, result.Timestamp, engine,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// InsertSpeedTestSamples bulk-inserts the per-sample buffer for a
// completed test in a single transaction. The parent speedtest_history
// row MUST already exist (FK target) — callers should write the
// history row via SaveSpeedTestReturningID first, then pass that ID
// here. Re-inserting with the same (test_id, sample_index) violates
// the PK and surfaces as an error so callers can detect double-insert
// bugs. Empty samples slice is a no-op (returns nil). Issue #286.
func (d *DB) InsertSpeedTestSamples(testID int64, samples []SpeedTestSample) error {
	if len(samples) == 0 {
		return nil
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(
		`INSERT INTO speedtest_samples (test_id, sample_index, phase, ts, mbps, latency_ms)
		 VALUES (?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, s := range samples {
		ts := s.Timestamp
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		if _, err := stmt.Exec(testID, s.SampleIndex, s.Phase, ts.UTC(), s.Mbps, s.LatencyMs); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetSpeedTestSamples returns every sample row for the given test_id,
// ordered by sample_index ascending so the mini-chart renders the
// throughput evolution left-to-right exactly as it was emitted.
// Returns an empty slice (NOT an error) for test_ids with no samples
// row — pre-#286 tests legitimately have no samples and the UI
// renders an empty-state hint in that case. Issue #286.
func (d *DB) GetSpeedTestSamples(testID int64) ([]SpeedTestSample, error) {
	rows, err := d.db.Query(
		`SELECT sample_index, phase, ts, COALESCE(mbps, 0), COALESCE(latency_ms, 0)
		 FROM speedtest_samples
		 WHERE test_id = ?
		 ORDER BY sample_index ASC`,
		testID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SpeedTestSample, 0)
	for rows.Next() {
		var s SpeedTestSample
		if err := rows.Scan(&s.SampleIndex, &s.Phase, &s.Timestamp, &s.Mbps, &s.LatencyMs); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetLatestSpeedTestHistoryID returns the speedtest_history.id of the
// most-recent row, or (0, false, nil) if no history rows exist yet.
// Used by the type=speed scheduled service check to stamp the new
// service_checks_history.speedtest_history_id column so the expanded
// log row can link to the parent test. Issue #286.
func (d *DB) GetLatestSpeedTestHistoryID() (int64, bool, error) {
	row := d.db.QueryRow(
		`SELECT id FROM speedtest_history ORDER BY id DESC LIMIT 1`,
	)
	var id int64
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return 0, false, nil
		}
		return 0, false, err
	}
	return id, true, nil
}

// GetSpeedTestHistory returns speed test history for the given time range.
func (d *DB) GetSpeedTestHistory(hours int) ([]SpeedTestHistoryPoint, error) {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	rows, err := d.db.Query(
		`SELECT id, timestamp, download_mbps, upload_mbps, latency_ms, jitter_ms, COALESCE(server_name, ''), COALESCE(isp, ''), COALESCE(engine, 'ookla_cli')
		 FROM speedtest_history
		 WHERE timestamp >= ?
		 ORDER BY timestamp ASC`,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []SpeedTestHistoryPoint
	for rows.Next() {
		var p SpeedTestHistoryPoint
		if err := rows.Scan(&p.ID, &p.Timestamp, &p.DownloadMbps, &p.UploadMbps, &p.LatencyMs, &p.JitterMs, &p.ServerName, &p.ISP, &p.Engine); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// LastSpeedTestAttempt records the outcome of the most recent speed-test
// run. The scheduled type=speed service check reads this (plus the latest
// speedtest_history row) to determine health, and the dashboard widget
// reads it to render "Running initial speed test…" on fresh installs. See
// issue #210.
type LastSpeedTestAttempt struct {
	Timestamp time.Time `json:"timestamp"`
	// Status values: "success", "failed", "pending", "disabled".
	// "success" — the run produced a speedtest_history row.
	// "failed" — Ookla errored or returned zero throughput.
	// "pending" — scheduler set this before invoking the runner;
	//             cleared on outcome. Used by the widget to render
	//             "Running initial speed test…" pre-first-result.
	// "disabled" — the speed-test interval is the disabled sentinel (#180).
	Status   string `json:"status"`
	ErrorMsg string `json:"error_msg,omitempty"`
}

// SaveSpeedTestAttempt upserts the current speed-test attempt state into
// the single-row speedtest_attempt table (id=1).
func (d *DB) SaveSpeedTestAttempt(att LastSpeedTestAttempt) error {
	_, err := d.db.Exec(
		`INSERT INTO speedtest_attempt (id, timestamp, status, error_msg)
		 VALUES (1, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   timestamp = excluded.timestamp,
		   status = excluded.status,
		   error_msg = excluded.error_msg`,
		att.Timestamp.UTC(), att.Status, att.ErrorMsg,
	)
	return err
}

// GetLastSpeedTestAttempt returns the current speed-test attempt state
// or (nil, nil) if nothing has been recorded yet (fresh install pre-first
// scheduler tick).
func (d *DB) GetLastSpeedTestAttempt() (*LastSpeedTestAttempt, error) {
	row := d.db.QueryRow(
		`SELECT timestamp, status, COALESCE(error_msg, '')
		 FROM speedtest_attempt
		 WHERE id = 1`,
	)
	var att LastSpeedTestAttempt
	if err := row.Scan(&att.Timestamp, &att.Status, &att.ErrorMsg); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &att, nil
}

// GPUHistoryPoint represents a single time-series data point for a GPU.
type GPUHistoryPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	GPUIndex    int       `json:"gpu_index"`
	Name        string    `json:"name"`
	Vendor      string    `json:"vendor"`
	UsagePct    float64   `json:"usage_percent"`
	MemPct      float64   `json:"mem_percent"`
	Temperature int       `json:"temperature_c"`
	PowerW      float64   `json:"power_watts"`
	EncoderPct  float64   `json:"encoder_percent"`
	DecoderPct  float64   `json:"decoder_percent"`
}

// GetGPUHistory returns GPU history for the given time range.
func (d *DB) GetGPUHistory(hours int) ([]GPUHistoryPoint, error) {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	rows, err := d.db.Query(
		`SELECT timestamp, gpu_index, name, vendor, usage_pct, mem_pct, temperature, power_watts, encoder_pct, decoder_pct
		 FROM gpu_history
		 WHERE timestamp >= ?
		 ORDER BY gpu_index ASC, timestamp ASC`,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []GPUHistoryPoint
	for rows.Next() {
		var p GPUHistoryPoint
		if err := rows.Scan(&p.Timestamp, &p.GPUIndex, &p.Name, &p.Vendor, &p.UsagePct, &p.MemPct, &p.Temperature, &p.PowerW, &p.EncoderPct, &p.DecoderPct); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// DiskUsagePoint is a single historical data point for capacity forecasting.
type DiskUsagePoint struct {
	Timestamp string  `json:"timestamp"`
	UsedGB    float64 `json:"used_gb"`
	TotalGB   float64 `json:"total_gb"`
	UsedPct   float64 `json:"used_pct"`
}

// DiskUsageSeries holds usage history for one mount point.
type DiskUsageSeries struct {
	MountPoint string           `json:"mount_point"`
	Label      string           `json:"label"`
	Device     string           `json:"device"`
	TotalGB    float64          `json:"total_gb"`
	CurrentPct float64          `json:"current_pct"`
	Points     []DiskUsagePoint `json:"points"`
}

// GetDiskUsageHistory returns usage history grouped by mount point.
func (d *DB) GetDiskUsageHistory(limit int) ([]DiskUsageSeries, error) {
	// Get unique mount points
	mpRows, err := d.db.Query(
		`SELECT mount_point, label, device, total_gb FROM disk_usage_history
		 GROUP BY mount_point ORDER BY mount_point`)
	if err != nil {
		return nil, err
	}
	defer mpRows.Close()

	var series []DiskUsageSeries
	for mpRows.Next() {
		var ds DiskUsageSeries
		if err := mpRows.Scan(&ds.MountPoint, &ds.Label, &ds.Device, &ds.TotalGB); err != nil {
			continue
		}
		series = append(series, ds)
	}

	// For each mount point, get last N data points
	for i := range series {
		rows, err := d.db.Query(
			`SELECT timestamp, used_gb, total_gb, used_pct FROM disk_usage_history
			 WHERE mount_point = ? ORDER BY timestamp DESC LIMIT ?`,
			series[i].MountPoint, limit,
		)
		if err != nil {
			continue
		}
		var points []DiskUsagePoint
		for rows.Next() {
			var p DiskUsagePoint
			if err := rows.Scan(&p.Timestamp, &p.UsedGB, &p.TotalGB, &p.UsedPct); err != nil {
				continue
			}
			points = append(points, p)
		}
		rows.Close()
		// Reverse to chronological order
		for l, r := 0, len(points)-1; l < r; l, r = l+1, r-1 {
			points[l], points[r] = points[r], points[l]
		}
		series[i].Points = points
		if len(points) > 0 {
			series[i].CurrentPct = points[len(points)-1].UsedPct
			series[i].TotalGB = points[len(points)-1].TotalGB
		}
	}

	return series, nil
}

// GetLatestSnapshot returns the most recent snapshot.
func (d *DB) GetLatestSnapshot() (*internal.Snapshot, error) {
	row := d.db.QueryRow("SELECT data FROM snapshots ORDER BY timestamp DESC LIMIT 1")
	var data string
	if err := row.Scan(&data); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	var snap internal.Snapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// GetSnapshot returns a snapshot by ID.
func (d *DB) GetSnapshot(id string) (*internal.Snapshot, error) {
	row := d.db.QueryRow("SELECT data FROM snapshots WHERE id = ?", id)
	var data string
	if err := row.Scan(&data); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	var snap internal.Snapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// SnapshotSummary is a lightweight view of a snapshot for listing.
type SnapshotSummary struct {
	ID            string    `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	Duration      float64   `json:"duration_seconds"`
	CriticalCount int       `json:"critical_count"`
	WarningCount  int       `json:"warning_count"`
	InfoCount     int       `json:"info_count"`
}

// ListSnapshots returns summaries of recent snapshots.
func (d *DB) ListSnapshots(limit int) ([]SnapshotSummary, error) {
	rows, err := d.db.Query(`
		SELECT 
			s.id, s.timestamp, s.duration_seconds,
			COALESCE(SUM(CASE WHEN f.severity = 'critical' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN f.severity = 'warning' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN f.severity = 'info' THEN 1 ELSE 0 END), 0)
		FROM snapshots s
		LEFT JOIN findings f ON f.snapshot_id = s.id
		GROUP BY s.id
		ORDER BY s.timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []SnapshotSummary
	for rows.Next() {
		var s SnapshotSummary
		if err := rows.Scan(&s.ID, &s.Timestamp, &s.Duration, &s.CriticalCount, &s.WarningCount, &s.InfoCount); err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}
	return summaries, nil
}

// AlertStateFinding is the minimum finding shape required to track alert lifecycle.
type AlertStateFinding struct {
	Fingerprint string
	FindingID   string
	Severity    string
	Title       string
}

// AlertStatus describes the current lifecycle state of an alert.
type AlertStatus string

const (
	AlertStatusOpen         AlertStatus = "open"
	AlertStatusAcknowledged AlertStatus = "acknowledged"
	AlertStatusSnoozed      AlertStatus = "snoozed"
	AlertStatusResolved     AlertStatus = "resolved"
)

// AlertRecord is the API-facing representation of an alert row.
type AlertRecord struct {
	ID                int64       `json:"id"`
	FindingID         string      `json:"finding_id"`
	SnapshotID        string      `json:"snapshot_id"`
	Fingerprint       string      `json:"fingerprint,omitempty"`
	Severity          string      `json:"severity"`
	Title             string      `json:"title"`
	Status            AlertStatus `json:"status"`
	SuppressionReason string      `json:"suppression_reason,omitempty"`
	NotifiedAt        string      `json:"notified_at,omitempty"`
	ResolvedAt        string      `json:"resolved_at,omitempty"`
	AcknowledgedAt    string      `json:"acknowledged_at,omitempty"`
	AcknowledgedBy    string      `json:"acknowledged_by,omitempty"`
	SnoozedUntil      string      `json:"snoozed_until,omitempty"`
	CreatedAt         string      `json:"created_at"`
}

// AlertEvent is a lifecycle event tied to an alert.
type AlertEvent struct {
	ID        int64  `json:"id"`
	AlertID   int64  `json:"alert_id"`
	EventType string `json:"event_type"`
	Actor     string `json:"actor"`
	Data      any    `json:"data,omitempty"`
	CreatedAt string `json:"created_at"`
}

// SyncAlertStates updates open/resolved alert state based on current findings.
func (d *DB) SyncAlertStates(snapshotID string, findings []AlertStateFinding, now time.Time) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	active := make(map[string]AlertStateFinding, len(findings))
	for _, f := range findings {
		if f.Fingerprint == "" {
			continue
		}
		if _, exists := active[f.Fingerprint]; !exists {
			active[f.Fingerprint] = f
		}
	}

	rows, err := tx.Query(`
		SELECT id, fingerprint
		FROM alerts
		WHERE resolved_at IS NULL AND COALESCE(fingerprint, '') != ''
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	openByFingerprint := make(map[string]int64)
	var duplicateIDs []int64
	for rows.Next() {
		var id int64
		var fp string
		if err := rows.Scan(&id, &fp); err != nil {
			return err
		}
		if _, exists := openByFingerprint[fp]; exists {
			duplicateIDs = append(duplicateIDs, id)
			continue
		}
		openByFingerprint[fp] = id
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range duplicateIDs {
		if _, err := tx.Exec("UPDATE alerts SET resolved_at = ? WHERE id = ?", now, id); err != nil {
			return err
		}
		if err := d.appendAlertEventTx(tx, id, "resolved", "system", map[string]any{"reason": "deduplicated_open_rows"}, now); err != nil {
			return err
		}
	}

	for fp, f := range active {
		if id, exists := openByFingerprint[fp]; exists {
			if _, err := tx.Exec(
				"UPDATE alerts SET finding_id = ?, snapshot_id = ?, severity = ?, title = ? WHERE id = ?",
				f.FindingID, snapshotID, f.Severity, f.Title, id,
			); err != nil {
				return err
			}
			delete(openByFingerprint, fp)
			continue
		}

		result, err := tx.Exec(
			`INSERT INTO alerts (finding_id, snapshot_id, severity, title, fingerprint)
			 VALUES (?, ?, ?, ?, ?)`,
			f.FindingID, snapshotID, f.Severity, f.Title, fp,
		)
		if err != nil {
			return err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		if err := d.appendAlertEventTx(tx, id, "opened", "system", nil, now); err != nil {
			return err
		}
	}

	for fp, id := range openByFingerprint {
		if _, err := tx.Exec("UPDATE alerts SET resolved_at = ? WHERE id = ?", now, id); err != nil {
			return err
		}
		if err := d.appendAlertEventTx(tx, id, "resolved", "system", map[string]any{"fingerprint": fp}, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListAlerts returns recent alerts, optionally filtered by status.
func (d *DB) ListAlerts(status string, limit int, now time.Time) ([]AlertRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := d.db.Query(
		`SELECT id, finding_id, snapshot_id, COALESCE(fingerprint, ''), severity, title,
				notified_at, resolved_at, acknowledged_at, COALESCE(acknowledged_by, ''), snoozed_until, created_at
		 FROM alerts
		 ORDER BY created_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	status = strings.ToLower(strings.TrimSpace(status))
	var out []AlertRecord
	for rows.Next() {
		raw, err := scanAlertRow(rows)
		if err != nil {
			return nil, err
		}
		record := buildAlertRecord(raw, now)
		if status != "" && string(record.Status) != status {
			continue
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// GetAlert fetches a single alert by numeric ID.
func (d *DB) GetAlert(id int64, now time.Time) (*AlertRecord, error) {
	row := d.db.QueryRow(
		`SELECT id, finding_id, snapshot_id, COALESCE(fingerprint, ''), severity, title,
				notified_at, resolved_at, acknowledged_at, COALESCE(acknowledged_by, ''), snoozed_until, created_at
		 FROM alerts WHERE id = ?`,
		id,
	)
	raw, err := scanAlertRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	record := buildAlertRecord(raw, now)
	return &record, nil
}

// GetAlertEvents returns alert lifecycle events in reverse chronological order.
func (d *DB) GetAlertEvents(alertID int64, limit int) ([]AlertEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := d.db.Query(
		`SELECT id, alert_id, event_type, actor, COALESCE(data, ''), created_at
		 FROM alert_events
		 WHERE alert_id = ?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		alertID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []AlertEvent
	for rows.Next() {
		var ev AlertEvent
		var dataRaw string
		var createdAt time.Time
		if err := rows.Scan(&ev.ID, &ev.AlertID, &ev.EventType, &ev.Actor, &dataRaw, &createdAt); err != nil {
			return nil, err
		}
		ev.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		if dataRaw != "" {
			var parsed any
			if err := json.Unmarshal([]byte(dataRaw), &parsed); err == nil {
				ev.Data = parsed
			} else {
				ev.Data = dataRaw
			}
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// AcknowledgeAlert marks an unresolved alert as acknowledged.
func (d *DB) AcknowledgeAlert(id int64, actor string, now time.Time) (bool, error) {
	if actor == "" {
		actor = "manual"
	}
	tx, err := d.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`UPDATE alerts
		 SET acknowledged_at = ?, acknowledged_by = ?, snoozed_until = NULL
		 WHERE id = ? AND resolved_at IS NULL`,
		now, actor, id,
	)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return false, nil
	}
	if err := d.appendAlertEventTx(tx, id, "acknowledged", actor, nil, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// UnacknowledgeAlert removes acknowledgement state from an unresolved alert.
func (d *DB) UnacknowledgeAlert(id int64, actor string, now time.Time) (bool, error) {
	if actor == "" {
		actor = "manual"
	}
	tx, err := d.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`UPDATE alerts
		 SET acknowledged_at = NULL, acknowledged_by = NULL
		 WHERE id = ? AND resolved_at IS NULL`,
		id,
	)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return false, nil
	}
	if err := d.appendAlertEventTx(tx, id, "unacknowledged", actor, nil, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// SnoozeAlert suppresses notifications for an unresolved alert until the given time.
func (d *DB) SnoozeAlert(id int64, until time.Time, actor string, now time.Time) (bool, error) {
	if actor == "" {
		actor = "manual"
	}
	tx, err := d.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`UPDATE alerts
		 SET snoozed_until = ?, acknowledged_at = NULL, acknowledged_by = NULL
		 WHERE id = ? AND resolved_at IS NULL`,
		until, id,
	)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return false, nil
	}
	if err := d.appendAlertEventTx(tx, id, "snoozed", actor, map[string]any{"until": until.UTC().Format(time.RFC3339)}, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// UnsnoozeAlert removes snooze state from an unresolved alert.
func (d *DB) UnsnoozeAlert(id int64, actor string, now time.Time) (bool, error) {
	if actor == "" {
		actor = "manual"
	}
	tx, err := d.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`UPDATE alerts
		 SET snoozed_until = NULL
		 WHERE id = ? AND resolved_at IS NULL`,
		id,
	)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return false, nil
	}
	if err := d.appendAlertEventTx(tx, id, "unsnoozed", actor, nil, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// IsAlertSuppressed reports whether an unresolved alert fingerprint is suppressed.
func (d *DB) IsAlertSuppressed(fingerprint string, now time.Time) (bool, string, error) {
	if strings.TrimSpace(fingerprint) == "" {
		return false, "", nil
	}

	row := d.db.QueryRow(
		`SELECT acknowledged_at, snoozed_until
		 FROM alerts
		 WHERE fingerprint = ? AND resolved_at IS NULL
		 ORDER BY id DESC
		 LIMIT 1`,
		fingerprint,
	)
	var acknowledgedAt sql.NullTime
	var snoozedUntil sql.NullTime
	if err := row.Scan(&acknowledgedAt, &snoozedUntil); err != nil {
		if err == sql.ErrNoRows {
			return false, "", nil
		}
		return false, "", err
	}

	if snoozedUntil.Valid && snoozedUntil.Time.After(now) {
		return true, "snoozed", nil
	}
	if acknowledgedAt.Valid {
		return true, "acknowledged", nil
	}
	return false, "", nil
}

type alertRowScanner interface {
	Scan(dest ...any) error
}

type alertRow struct {
	ID             int64
	FindingID      string
	SnapshotID     string
	Fingerprint    string
	Severity       string
	Title          string
	NotifiedAt     sql.NullTime
	ResolvedAt     sql.NullTime
	AcknowledgedAt sql.NullTime
	AcknowledgedBy string
	SnoozedUntil   sql.NullTime
	CreatedAt      time.Time
}

func scanAlertRow(s alertRowScanner) (alertRow, error) {
	var row alertRow
	err := s.Scan(
		&row.ID,
		&row.FindingID,
		&row.SnapshotID,
		&row.Fingerprint,
		&row.Severity,
		&row.Title,
		&row.NotifiedAt,
		&row.ResolvedAt,
		&row.AcknowledgedAt,
		&row.AcknowledgedBy,
		&row.SnoozedUntil,
		&row.CreatedAt,
	)
	return row, err
}

func buildAlertRecord(row alertRow, now time.Time) AlertRecord {
	status, reason := alertStatus(row.ResolvedAt, row.AcknowledgedAt, row.SnoozedUntil, now)
	return AlertRecord{
		ID:                row.ID,
		FindingID:         row.FindingID,
		SnapshotID:        row.SnapshotID,
		Fingerprint:       row.Fingerprint,
		Severity:          row.Severity,
		Title:             row.Title,
		Status:            status,
		SuppressionReason: reason,
		NotifiedAt:        nullTimeRFC3339(row.NotifiedAt),
		ResolvedAt:        nullTimeRFC3339(row.ResolvedAt),
		AcknowledgedAt:    nullTimeRFC3339(row.AcknowledgedAt),
		AcknowledgedBy:    row.AcknowledgedBy,
		SnoozedUntil:      nullTimeRFC3339(row.SnoozedUntil),
		CreatedAt:         row.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func alertStatus(resolvedAt, acknowledgedAt, snoozedUntil sql.NullTime, now time.Time) (AlertStatus, string) {
	if resolvedAt.Valid {
		return AlertStatusResolved, ""
	}
	if snoozedUntil.Valid && snoozedUntil.Time.After(now) {
		return AlertStatusSnoozed, "snoozed"
	}
	if acknowledgedAt.Valid {
		return AlertStatusAcknowledged, "acknowledged"
	}
	return AlertStatusOpen, ""
}

func nullTimeRFC3339(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

func (d *DB) appendAlertEventTx(tx *sql.Tx, alertID int64, eventType, actor string, data any, at time.Time) error {
	if actor == "" {
		actor = "system"
	}
	var payload any
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		payload = string(b)
	}
	_, err := tx.Exec(
		`INSERT INTO alert_events (alert_id, event_type, actor, data, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		alertID, eventType, actor, payload, at,
	)
	return err
}

// MarkAlertsNotifiedByFingerprint updates open alerts as notified.
func (d *DB) MarkAlertsNotifiedByFingerprint(fingerprints []string, notifiedAt time.Time) error {
	fps := uniqueStrings(fingerprints)
	if len(fps) == 0 {
		return nil
	}

	placeholders := make([]string, len(fps))
	args := make([]any, 0, len(fps)+1)
	args = append(args, notifiedAt)
	for i, fp := range fps {
		placeholders[i] = "?"
		args = append(args, fp)
	}

	query := fmt.Sprintf(
		"UPDATE alerts SET notified_at = ? WHERE resolved_at IS NULL AND fingerprint IN (%s)",
		strings.Join(placeholders, ","),
	)
	_, err := d.db.Exec(query, args...)
	return err
}

// CanSendNotification reports whether a notification is allowed for route+fingerprint based on cooldown.
func (d *DB) CanSendNotification(fingerprint, routeKey string, cooldown time.Duration, now time.Time) (bool, error) {
	if fingerprint == "" || routeKey == "" || cooldown <= 0 {
		return true, nil
	}

	row := d.db.QueryRow(
		"SELECT last_sent_unix FROM notification_state WHERE fingerprint = ? AND route_key = ?",
		fingerprint, routeKey,
	)
	var lastSent sql.NullInt64
	if err := row.Scan(&lastSent); err != nil {
		if err == sql.ErrNoRows {
			return true, nil
		}
		return false, err
	}
	if !lastSent.Valid || lastSent.Int64 <= 0 {
		return true, nil
	}

	return now.Unix()-lastSent.Int64 >= int64(cooldown.Seconds()), nil
}

// SaveNotificationState records the last delivery timestamp/status for route+fingerprint.
func (d *DB) SaveNotificationState(fingerprint, routeKey, status string, sentAt time.Time) error {
	if fingerprint == "" || routeKey == "" {
		return nil
	}
	_, err := d.db.Exec(
		`INSERT INTO notification_state (fingerprint, route_key, last_sent_unix, last_status)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(fingerprint, route_key)
		 DO UPDATE SET last_sent_unix = excluded.last_sent_unix, last_status = excluded.last_status`,
		fingerprint, routeKey, sentAt.Unix(), status,
	)
	return err
}

// ServiceCheckState is the latest status snapshot for a service check key.
type ServiceCheckState struct {
	Status              string
	ConsecutiveFailures int
	CheckedAt           time.Time
}

// ServiceCheckEntry represents a stored service check result.
type ServiceCheckEntry struct {
	Key                 string `json:"key"`
	Name                string `json:"name"`
	Type                string `json:"type"`
	Target              string `json:"target"`
	Status              string `json:"status"`
	ResponseMS          int64  `json:"response_ms"`
	Error               string `json:"error,omitempty"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	FailureThreshold    int    `json:"failure_threshold"`
	FailureSeverity     string `json:"failure_severity"`
	CheckedAt           string `json:"checked_at"`

	// Details carries the per-check-type diagnostic map (HTTP status_code,
	// DNS records, Ping rtt_ms, TCP resolved_address, failure_stage, …)
	// deserialised from the details_json column. nil on legacy rows
	// written before the column existed, or for check types that produce
	// no extra context. See issue #182.
	Details map[string]any `json:"details,omitempty"`

	// SpeedTestHistoryID links a type=speed log row to the
	// speedtest_history row that produced it, so the
	// /service-checks expanded-log mini-chart can fetch
	// /api/v1/speedtest/samples/{id}. Zero on legacy rows + non-speed
	// types — the UI renders the "no per-sample data available" empty
	// state in that case. PRD #283 slice 3 / issue #286.
	SpeedTestHistoryID int64 `json:"speedtest_history_id,omitempty"`
}

// SaveServiceCheckResults appends service check run results to history.
func (d *DB) SaveServiceCheckResults(results []internal.ServiceCheckResult) error {
	if len(results) == 0 {
		return nil
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, result := range results {
		if strings.TrimSpace(result.Key) == "" {
			continue
		}
		checkedAt := time.Now().UTC()
		if parsed, err := time.Parse(time.RFC3339, result.CheckedAt); err == nil {
			checkedAt = parsed.UTC()
		}
		// Serialise the per-type Details map. A nil or empty map writes
		// SQL NULL so legacy readers and the pre-migration world stay
		// unaffected. We do NOT fail the whole transaction on encoding
		// errors — the core row still contains status/ms/error so losing
		// details gracefully is preferable to losing the whole check
		// history for a run. See issue #182.
		var detailsPayload any // sql.NullString wrapped via any; nil = SQL NULL
		if len(result.Details) > 0 {
			if buf, err := json.Marshal(result.Details); err == nil {
				detailsPayload = string(buf)
			} else {
				d.logger.Warn("service check details_json marshal failed; row saved without details",
					"check", result.Name, "error", err)
			}
		}
		// speedtest_history_id is non-zero only for type=speed rows
		// where the dispatch managed to resolve the parent test ID
		// (issue #286). NULL otherwise — UI treats NULL as "legacy
		// row, no per-sample mini-chart available".
		var speedHistID any // nil → SQL NULL
		if result.SpeedTestHistoryID > 0 {
			speedHistID = result.SpeedTestHistoryID
		}
		_, err := tx.Exec(
			`INSERT INTO service_checks_history (
				check_key, name, check_type, target, status, response_ms, error_message,
				consecutive_failures, failure_threshold, failure_severity, checked_at, details_json,
				speedtest_history_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			result.Key,
			result.Name,
			result.Type,
			result.Target,
			result.Status,
			result.ResponseMS,
			result.Error,
			result.ConsecutiveFailures,
			result.FailureThreshold,
			result.FailureSeverity,
			checkedAt,
			detailsPayload,
			speedHistID,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetLatestServiceCheckState returns the most recent known state for a service check key.
func (d *DB) GetLatestServiceCheckState(checkKey string) (ServiceCheckState, bool, error) {
	var state ServiceCheckState
	if strings.TrimSpace(checkKey) == "" {
		return state, false, nil
	}
	row := d.db.QueryRow(
		`SELECT status, consecutive_failures, checked_at
		 FROM service_checks_history
		 WHERE check_key = ?
		 ORDER BY checked_at DESC
		 LIMIT 1`,
		checkKey,
	)
	if err := row.Scan(&state.Status, &state.ConsecutiveFailures, &state.CheckedAt); err != nil {
		if err == sql.ErrNoRows {
			return ServiceCheckState{}, false, nil
		}
		return ServiceCheckState{}, false, err
	}
	return state, true, nil
}

// ListLatestServiceChecks returns latest status per service check key.
func (d *DB) ListLatestServiceChecks(limit int) ([]ServiceCheckEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := d.db.Query(
		`SELECT check_key, name, check_type, target, status,
				COALESCE(response_ms, 0), COALESCE(error_message, ''),
				COALESCE(consecutive_failures, 0), COALESCE(failure_threshold, 1), COALESCE(failure_severity, 'warning'),
				checked_at, details_json, speedtest_history_id
		 FROM service_checks_history
		 WHERE id IN (
			SELECT MAX(id) FROM service_checks_history GROUP BY check_key
		 )
		 ORDER BY checked_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ServiceCheckEntry
	for rows.Next() {
		var e ServiceCheckEntry
		var checkedAt time.Time
		var detailsJSON sql.NullString
		var speedHistID sql.NullInt64
		if err := rows.Scan(
			&e.Key,
			&e.Name,
			&e.Type,
			&e.Target,
			&e.Status,
			&e.ResponseMS,
			&e.Error,
			&e.ConsecutiveFailures,
			&e.FailureThreshold,
			&e.FailureSeverity,
			&checkedAt,
			&detailsJSON,
			&speedHistID,
		); err != nil {
			return nil, err
		}
		e.CheckedAt = checkedAt.UTC().Format(time.RFC3339)
		e.Details = decodeDetailsJSON(d.logger, detailsJSON, e.Key)
		if speedHistID.Valid {
			e.SpeedTestHistoryID = speedHistID.Int64
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetServiceCheckHistory returns recent history for a specific check key.
func (d *DB) GetServiceCheckHistory(checkKey string, limit int) ([]ServiceCheckEntry, error) {
	if strings.TrimSpace(checkKey) == "" {
		return []ServiceCheckEntry{}, nil
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := d.db.Query(
		`SELECT check_key, name, check_type, target, status,
				COALESCE(response_ms, 0), COALESCE(error_message, ''),
				COALESCE(consecutive_failures, 0), COALESCE(failure_threshold, 1), COALESCE(failure_severity, 'warning'),
				checked_at, details_json, speedtest_history_id
		 FROM service_checks_history
		 WHERE check_key = ?
		 ORDER BY checked_at DESC
		 LIMIT ?`,
		checkKey,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ServiceCheckEntry
	for rows.Next() {
		var e ServiceCheckEntry
		var checkedAt time.Time
		var detailsJSON sql.NullString
		var speedHistID sql.NullInt64
		if err := rows.Scan(
			&e.Key,
			&e.Name,
			&e.Type,
			&e.Target,
			&e.Status,
			&e.ResponseMS,
			&e.Error,
			&e.ConsecutiveFailures,
			&e.FailureThreshold,
			&e.FailureSeverity,
			&checkedAt,
			&detailsJSON,
			&speedHistID,
		); err != nil {
			return nil, err
		}
		e.CheckedAt = checkedAt.UTC().Format(time.RFC3339)
		e.Details = decodeDetailsJSON(d.logger, detailsJSON, e.Key)
		if speedHistID.Valid {
			e.SpeedTestHistoryID = speedHistID.Int64
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// decodeDetailsJSON deserialises the details_json column from a history
// row. Returns nil on SQL NULL, empty strings, or malformed JSON — the
// log UI treats nil as "no extra context to render" rather than an error.
// Corrupted rows are logged at warn level but never break list/history
// queries, since the core row is still valuable. See issue #182.
func decodeDetailsJSON(logger *slog.Logger, raw sql.NullString, checkKey string) map[string]any {
	if !raw.Valid {
		return nil
	}
	s := strings.TrimSpace(raw.String)
	if s == "" || s == "null" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		if logger != nil {
			logger.Warn("service check details_json unmarshal failed; row returned without details",
				"check_key", checkKey, "error", err)
		}
		return nil
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// PruneServiceCheckHistory deletes service check history rows older than the given duration.
func (d *DB) PruneServiceCheckHistory(olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := d.db.Exec("DELETE FROM service_checks_history WHERE checked_at < ?", cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// DeleteServiceCheckByKey removes all history for a specific service check key.
func (d *DB) DeleteServiceCheckByKey(key string) (int, error) {
	result, err := d.db.Exec("DELETE FROM service_checks_history WHERE check_key = ?", key)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// DeleteServiceChecksNotIn removes all history rows whose check_key is NOT in
// the provided keepKeys slice. This is how orphaned history is cleaned up
// after a service check is removed from the configuration. Passing a nil or
// empty slice deletes every row in service_checks_history.
func (d *DB) DeleteServiceChecksNotIn(keepKeys []string) (int, error) {
	if len(keepKeys) == 0 {
		result, err := d.db.Exec("DELETE FROM service_checks_history")
		if err != nil {
			return 0, err
		}
		n, _ := result.RowsAffected()
		return int(n), nil
	}

	// Build placeholder list (?, ?, ?) and args slice.
	placeholders := make([]string, len(keepKeys))
	args := make([]interface{}, len(keepKeys))
	for i, k := range keepKeys {
		placeholders[i] = "?"
		args[i] = k
	}
	query := "DELETE FROM service_checks_history WHERE check_key NOT IN (" +
		strings.Join(placeholders, ",") + ")"
	result, err := d.db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// GetFindingHistory returns how a specific finding category has changed over time.
func (d *DB) GetFindingHistory(category string, limit int) ([]internal.Finding, error) {
	rows, err := d.db.Query(`
		SELECT f.data 
		FROM findings f
		JOIN snapshots s ON s.id = f.snapshot_id
		WHERE f.category = ?
		ORDER BY s.timestamp DESC
		LIMIT ?
	`, category, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var findings []internal.Finding
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var f internal.Finding
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			continue
		}
		findings = append(findings, f)
	}
	return findings, nil
}

// GetConfig retrieves a config value by key.
func (d *DB) GetConfig(key string) (string, error) {
	row := d.db.QueryRow("SELECT value FROM config WHERE key = ?", key)
	var val string
	if err := row.Scan(&val); err != nil {
		return "", err
	}
	return val, nil
}

// SetConfig stores a config value.
func (d *DB) SetConfig(key, value string) error {
	_, err := d.db.Exec(
		"INSERT INTO config (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP) ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = CURRENT_TIMESTAMP",
		key, value, value,
	)
	return err
}

// PruneSnapshots deletes snapshots older than the given duration, keeping at least `keepMin`.
// Associated smart_history and system_history rows are also pruned (via CASCADE or explicit DELETE).
func (d *DB) PruneSnapshots(olderThan time.Duration, keepMin int) (int, error) {
	cutoff := time.Now().Add(-olderThan)

	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Identify snapshot IDs that will be deleted
	pruneQuery := `
		SELECT id FROM snapshots
		WHERE id NOT IN (
			SELECT id FROM snapshots ORDER BY timestamp DESC LIMIT ?
		) AND timestamp < ?
	`

	// Explicitly delete from history tables for the snapshots being pruned,
	// in case foreign_keys or CASCADE is not fully honoured at runtime.
	//
	// NOTE: disk_usage_history is intentionally NOT in this list — it has no
	// snapshot_id column (capacity-forecast rows outlive snapshot pruning).
	// It is pruned independently via PruneDiskUsageHistory. Including it here
	// previously caused the whole transaction to roll back on every run
	// ("no such column: snapshot_id"), making snapshot+history pruning a no-op.
	for _, table := range []string{"smart_history", "system_history", "gpu_history", "container_stats_history", "speedtest_history", "process_history"} {
		_, err := tx.Exec(fmt.Sprintf(
			`DELETE FROM %s WHERE snapshot_id IN (%s)`, table, pruneQuery,
		), keepMin, cutoff)
		if err != nil {
			return 0, fmt.Errorf("prune %s: %w", table, err)
		}
	}

	// Delete the snapshots themselves (findings cascade via FK or are orphaned)
	result, err := tx.Exec(`
		DELETE FROM snapshots 
		WHERE id NOT IN (
			SELECT id FROM snapshots ORDER BY timestamp DESC LIMIT ?
		) AND timestamp < ?
	`, keepMin, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune snapshots: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	affected, _ := result.RowsAffected()
	return int(affected), nil
}

// ---------- Disk history ----------

// DiskHistoryPoint represents a single SMART data point for trend graphs.
type DiskHistoryPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	Temperature int       `json:"temperature"`
	Reallocated int64     `json:"reallocated"`
	Pending     int64     `json:"pending"`
	UDMACRC     int64     `json:"udma_crc"`
	CmdTimeout  int64     `json:"command_timeout"`
	PowerHours  int64     `json:"power_on_hours"`
	Health      bool      `json:"health_passed"`
}

// GetDiskHistory returns historical SMART data for a specific drive by serial number.
func (d *DB) GetDiskHistory(serial string, limit int) ([]DiskHistoryPoint, error) {
	rows, err := d.db.Query(
		`SELECT timestamp, temperature, reallocated, pending, udma_crc, command_timeout, power_on_hours, health_passed
		 FROM (
			SELECT timestamp, temperature, reallocated, pending, udma_crc, command_timeout, power_on_hours, health_passed
			FROM smart_history
			WHERE serial = ?
			ORDER BY timestamp DESC
			LIMIT ?
		 ) recent
		 ORDER BY timestamp ASC`,
		serial, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []DiskHistoryPoint
	for rows.Next() {
		var p DiskHistoryPoint
		if err := rows.Scan(&p.Timestamp, &p.Temperature, &p.Reallocated, &p.Pending, &p.UDMACRC, &p.CmdTimeout, &p.PowerHours, &p.Health); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetDiskHistoryInRange returns historical SMART data for a specific drive
// whose timestamp falls within the last `window` duration. Returned rows are
// ordered by timestamp ASC.
//
// Unlike GetDiskHistory (which caps by row count), this filters by time —
// which is what the /disk/<serial> charts need so the x-axis density stays
// legible regardless of scan frequency or retention depth. See issue #166.
func (d *DB) GetDiskHistoryInRange(serial string, window time.Duration) ([]DiskHistoryPoint, error) {
	cutoff := time.Now().UTC().Add(-window)
	rows, err := d.db.Query(
		`SELECT timestamp, temperature, reallocated, pending, udma_crc, command_timeout, power_on_hours, health_passed
		 FROM smart_history
		 WHERE serial = ? AND timestamp >= ?
		 ORDER BY timestamp ASC`,
		serial, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []DiskHistoryPoint
	for rows.Next() {
		var p DiskHistoryPoint
		if err := rows.Scan(&p.Timestamp, &p.Temperature, &p.Reallocated, &p.Pending, &p.UDMACRC, &p.CmdTimeout, &p.PowerHours, &p.Health); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetLastSMARTCollectedAt returns the most recent smart_history.timestamp
// for the given device and a found flag. Used by the scheduler's
// StaleSMARTChecker (issue #238) to decide whether a drive in standby is
// overdue for a SMART read.
//
// Returns:
//   - (ts, true, nil)          — device has at least one smart_history row
//   - (time.Time{}, false, nil) — device has no smart_history rows (new drive)
//   - (time.Time{}, false, err) — query failed
//
// Uses idx_smart_history_device (device, timestamp DESC) so the
// ORDER BY ... LIMIT 1 form is an index seek, not a table scan. We
// prefer that over MAX() because the modernc.org/sqlite driver only
// applies timestamp affinity to bare column scans — MAX() returns a
// string that would fail to scan into a time.Time.
func (d *DB) GetLastSMARTCollectedAt(device string) (time.Time, bool, error) {
	var ts time.Time
	err := d.db.QueryRow(
		`SELECT timestamp FROM smart_history
		 WHERE device = ?
		 ORDER BY timestamp DESC
		 LIMIT 1`,
		device,
	).Scan(&ts)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return ts, true, nil
}

// ---------- System history ----------

// SystemHistoryPoint represents a single system metrics data point.
type SystemHistoryPoint struct {
	Timestamp  time.Time `json:"timestamp"`
	CPUUsage   float64   `json:"cpu_usage"`
	MemPercent float64   `json:"mem_percent"`
	IOWait     float64   `json:"io_wait"`
	LoadAvg1   float64   `json:"load_avg_1"`
	LoadAvg5   float64   `json:"load_avg_5"`
}

// GetSystemHistory returns historical system metrics.
func (d *DB) GetSystemHistory(limit int) ([]SystemHistoryPoint, error) {
	rows, err := d.db.Query(
		`SELECT timestamp, cpu_usage, mem_percent, io_wait, load_avg_1, load_avg_5
		 FROM system_history
		 ORDER BY timestamp ASC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []SystemHistoryPoint
	for rows.Next() {
		var p SystemHistoryPoint
		if err := rows.Scan(&p.Timestamp, &p.CPUUsage, &p.MemPercent, &p.IOWait, &p.LoadAvg1, &p.LoadAvg5); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// NumericPoint represents a timestamped numeric value series.
type NumericPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// GetSystemHistoryRange returns system metrics between start and end, downsampled to maxPoints when needed.
func (d *DB) GetSystemHistoryRange(start, end time.Time, maxPoints int) ([]SystemHistoryPoint, error) {
	if end.Before(start) {
		start, end = end, start
	}
	rows, err := d.db.Query(
		`SELECT timestamp, cpu_usage, mem_percent, io_wait, load_avg_1, load_avg_5
		 FROM system_history
		 WHERE timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp ASC`,
		start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []SystemHistoryPoint
	for rows.Next() {
		var p SystemHistoryPoint
		if err := rows.Scan(&p.Timestamp, &p.CPUUsage, &p.MemPercent, &p.IOWait, &p.LoadAvg1, &p.LoadAvg5); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return downsampleSystemHistory(points, maxPoints), nil
}

// GetAverageDiskTemperatureRange returns average disk temperature over time in a range, downsampled to maxPoints.
func (d *DB) GetAverageDiskTemperatureRange(start, end time.Time, maxPoints int) ([]NumericPoint, error) {
	if end.Before(start) {
		start, end = end, start
	}
	rows, err := d.db.Query(
		`SELECT timestamp, AVG(COALESCE(temperature, 0))
		 FROM smart_history
		 WHERE timestamp >= ? AND timestamp <= ?
		 GROUP BY timestamp
		 ORDER BY timestamp ASC`,
		start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []NumericPoint
	for rows.Next() {
		var p NumericPoint
		if err := rows.Scan(&p.Timestamp, &p.Value); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return downsampleNumeric(points, maxPoints), nil
}

func downsampleSystemHistory(points []SystemHistoryPoint, maxPoints int) []SystemHistoryPoint {
	if maxPoints <= 0 || len(points) <= maxPoints {
		return points
	}
	step := float64(len(points)-1) / float64(maxPoints-1)
	out := make([]SystemHistoryPoint, 0, maxPoints)
	for i := 0; i < maxPoints; i++ {
		idx := int(float64(i) * step)
		if idx >= len(points) {
			idx = len(points) - 1
		}
		out = append(out, points[idx])
	}
	return out
}

func downsampleNumeric(points []NumericPoint, maxPoints int) []NumericPoint {
	if maxPoints <= 0 || len(points) <= maxPoints {
		return points
	}
	step := float64(len(points)-1) / float64(maxPoints-1)
	out := make([]NumericPoint, 0, maxPoints)
	for i := 0; i < maxPoints; i++ {
		idx := int(float64(i) * step)
		if idx >= len(points) {
			idx = len(points) - 1
		}
		out = append(out, points[idx])
	}
	return out
}

// ---------- Disk listing ----------

// DiskSummary provides a high-level view of each unique disk seen in SMART history.
type DiskSummary struct {
	Device     string `json:"device"`
	Serial     string `json:"serial"`
	Model      string `json:"model"`
	LastTemp   int    `json:"last_temperature"`
	LastHealth bool   `json:"last_health_passed"`
	PowerHours int64  `json:"power_on_hours"`
	DataPoints int    `json:"data_points"`
}

// ListDisks returns a summary for each unique disk seen in smart_history.
func (d *DB) ListDisks() ([]DiskSummary, error) {
	rows, err := d.db.Query(`
		SELECT
			latest.device,
			latest.serial,
			latest.model,
			latest.temperature,
			latest.health_passed,
			latest.power_on_hours,
			counts.data_points
		FROM (
			SELECT sh.*
			FROM smart_history sh
			INNER JOIN (
				SELECT serial, MAX(timestamp) AS max_ts
				FROM smart_history
				GROUP BY serial
			) grp ON sh.serial = grp.serial AND sh.timestamp = grp.max_ts
		) latest
		JOIN (
			SELECT serial, COUNT(*) AS data_points
			FROM smart_history
			GROUP BY serial
		) counts ON latest.serial = counts.serial
		ORDER BY latest.device
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var disks []DiskSummary
	for rows.Next() {
		var ds DiskSummary
		if err := rows.Scan(&ds.Device, &ds.Serial, &ds.Model, &ds.LastTemp, &ds.LastHealth, &ds.PowerHours, &ds.DataPoints); err != nil {
			return nil, err
		}
		disks = append(disks, ds)
	}
	return disks, rows.Err()
}

// ---------- Data lifecycle / pruning ----------

// PruneNotificationLog deletes notification log entries older than the given duration.
func (d *DB) PruneNotificationLog(olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := d.db.Exec("DELETE FROM notification_log WHERE created_at < ?", cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// PruneAlerts deletes resolved alerts older than the given duration.
func (d *DB) PruneAlerts(olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := d.db.Exec("DELETE FROM alerts WHERE created_at < ? AND resolved_at IS NOT NULL", cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// PruneOrphanedFindings deletes findings whose snapshot_id no longer exists.
func (d *DB) PruneOrphanedFindings() (int, error) {
	result, err := d.db.Exec("DELETE FROM findings WHERE snapshot_id NOT IN (SELECT id FROM snapshots)")
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// Vacuum runs SQLite VACUUM to reclaim disk space after pruning.
func (d *DB) Vacuum() error {
	_, err := d.db.Exec("VACUUM")
	return err
}

// DBStats returns size information about the database.
type DBStats struct {
	FileSizeMB       float64 `json:"file_size_mb"`
	SnapshotCount    int     `json:"snapshot_count"`
	SMARTRows        int     `json:"smart_history_rows"`
	SystemRows       int     `json:"system_history_rows"`
	FindingRows      int     `json:"finding_rows"`
	NotifyLogRows    int     `json:"notify_log_rows"`
	ServiceCheckRows int     `json:"service_check_rows"`
	AlertRows        int     `json:"alert_rows"`
	OldestSnapshot   string  `json:"oldest_snapshot,omitempty"`
	NewestSnapshot   string  `json:"newest_snapshot,omitempty"`
}

// GetDBStats returns statistics about the database contents.
func (d *DB) GetDBStats() (*DBStats, error) {
	stats := &DBStats{}

	// Row counts
	for _, q := range []struct {
		query string
		dest  *int
	}{
		{"SELECT COUNT(*) FROM snapshots", &stats.SnapshotCount},
		{"SELECT COUNT(*) FROM smart_history", &stats.SMARTRows},
		{"SELECT COUNT(*) FROM system_history", &stats.SystemRows},
		{"SELECT COUNT(*) FROM findings", &stats.FindingRows},
		{"SELECT COUNT(*) FROM notification_log", &stats.NotifyLogRows},
		{"SELECT COUNT(*) FROM service_checks_history", &stats.ServiceCheckRows},
		{"SELECT COUNT(*) FROM alerts", &stats.AlertRows},
	} {
		if err := d.db.QueryRow(q.query).Scan(q.dest); err != nil {
			return nil, err
		}
	}

	// DB file size via page_count * page_size
	var pageCount, pageSize int64
	d.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	d.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	stats.FileSizeMB = float64(pageCount*pageSize) / (1024 * 1024)

	// Oldest and newest snapshot timestamps
	d.db.QueryRow("SELECT COALESCE(MIN(timestamp), '') FROM snapshots").Scan(&stats.OldestSnapshot)
	d.db.QueryRow("SELECT COALESCE(MAX(timestamp), '') FROM snapshots").Scan(&stats.NewestSnapshot)

	return stats, nil
}

// PruneDiskUsageHistory removes disk_usage_history rows whose timestamp is
// strictly before cutoff. Returns the number of rows deleted.
//
// disk_usage_history is snapshot-independent (no snapshot_id column): it's
// keyed by mount_point + timestamp so capacity-forecast data survives
// snapshot pruning. It has its own retention policy, managed independently
// of PruneSnapshots.
func (d *DB) PruneDiskUsageHistory(cutoff time.Time) (int64, error) {
	res, err := d.db.Exec(`DELETE FROM disk_usage_history WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune disk_usage_history: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// PruneToSizeMB aggressively deletes the oldest snapshots until the DB is under the target size.
// Returns the number of snapshots deleted.
func (d *DB) PruneToSizeMB(targetMB float64) (int, error) {
	totalPruned := 0
	for i := 0; i < 20; i++ { // max 20 iterations to avoid infinite loop
		stats, err := d.GetDBStats()
		if err != nil {
			return totalPruned, err
		}
		if stats.FileSizeMB <= targetMB || stats.SnapshotCount <= 5 {
			break // under target or at minimum
		}
		// Delete the oldest 10% of snapshots (at least 5)
		batchSize := stats.SnapshotCount / 10
		if batchSize < 5 {
			batchSize = 5
		}
		// Delete oldest batch
		_, err = d.db.Exec(`
			DELETE FROM findings WHERE snapshot_id IN (
				SELECT id FROM snapshots ORDER BY timestamp ASC LIMIT ?
			)`, batchSize)
		if err != nil {
			return totalPruned, err
		}
		// NOTE: disk_usage_history excluded — no snapshot_id column; see PruneSnapshots.
		for _, table := range []string{"smart_history", "system_history", "gpu_history", "container_stats_history", "speedtest_history", "process_history"} {
			d.db.Exec(fmt.Sprintf(`DELETE FROM %s WHERE snapshot_id IN (
				SELECT id FROM snapshots ORDER BY timestamp ASC LIMIT ?
			)`, table), batchSize)
		}
		result, err := d.db.Exec("DELETE FROM snapshots WHERE id IN (SELECT id FROM snapshots ORDER BY timestamp ASC LIMIT ?)", batchSize)
		if err != nil {
			return totalPruned, err
		}
		n, _ := result.RowsAffected()
		totalPruned += int(n)
		if n == 0 {
			break
		}
	}
	if totalPruned > 0 {
		d.Vacuum()
	}
	return totalPruned, nil
}

// ---------- Bulk sparkline data ----------

// SparklinePoint is a compact data point for sparkline rendering.
type SparklinePoint struct {
	Timestamp   time.Time `json:"t"`
	Temperature int       `json:"temp"`
}

// DiskSparklines holds condensed history for a single drive.
type DiskSparklines struct {
	Serial      string           `json:"serial"`
	Model       string           `json:"model"`
	Device      string           `json:"device"`
	Temps       []SparklinePoint `json:"temps"`
	Reallocated []int64          `json:"reallocated"`
	Pending     []int64          `json:"pending"`
	CRC         []int64          `json:"crc"`
}

// GetAllDiskSparklines returns condensed SMART history for all drives (last N points each).
func (d *DB) GetAllDiskSparklines(pointsPerDisk int) ([]DiskSparklines, error) {
	// Get unique serials
	serials, err := d.db.Query("SELECT DISTINCT serial, model, device FROM smart_history GROUP BY serial ORDER BY device")
	if err != nil {
		return nil, err
	}
	defer serials.Close()

	var results []DiskSparklines
	for serials.Next() {
		var ds DiskSparklines
		if err := serials.Scan(&ds.Serial, &ds.Model, &ds.Device); err != nil {
			continue
		}

		rows, err := d.db.Query(
			`SELECT timestamp, temperature, reallocated, pending, udma_crc
			 FROM smart_history WHERE serial = ?
			 ORDER BY timestamp DESC LIMIT ?`,
			ds.Serial, pointsPerDisk,
		)
		if err != nil {
			continue
		}

		// Collect in reverse (DESC), then flip
		var temps []SparklinePoint
		var realloc, pending, crc []int64
		for rows.Next() {
			var sp SparklinePoint
			var r, p, c int64
			if err := rows.Scan(&sp.Timestamp, &sp.Temperature, &r, &p, &c); err != nil {
				continue
			}
			temps = append(temps, sp)
			realloc = append(realloc, r)
			pending = append(pending, p)
			crc = append(crc, c)
		}
		rows.Close()

		// Reverse to ASC order
		for i, j := 0, len(temps)-1; i < j; i, j = i+1, j-1 {
			temps[i], temps[j] = temps[j], temps[i]
			realloc[i], realloc[j] = realloc[j], realloc[i]
			pending[i], pending[j] = pending[j], pending[i]
			crc[i], crc[j] = crc[j], crc[i]
		}

		ds.Temps = temps
		ds.Reallocated = realloc
		ds.Pending = pending
		ds.CRC = crc
		results = append(results, ds)
	}
	return results, nil
}

// ---------- System history sparkline ----------

// GetSystemSparkline returns condensed system metrics for sparkline rendering.
func (d *DB) GetSystemSparkline(limit int) ([]SystemHistoryPoint, error) {
	return d.GetSystemHistory(limit)
}

// ---------- Notification log ----------

// NotificationLogEntry represents a single notification delivery attempt.
type NotificationLogEntry struct {
	ID            int       `json:"id"`
	WebhookName   string    `json:"webhook_name"`
	WebhookType   string    `json:"webhook_type"`
	Status        string    `json:"status"`
	FindingsCount int       `json:"findings_count"`
	ErrorMessage  string    `json:"error_message,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// SaveNotificationLog records the result of a webhook notification attempt.
func (d *DB) SaveNotificationLog(name, webhookType, status string, findingsCount int, errMsg string) error {
	_, err := d.db.Exec(
		`INSERT INTO notification_log (webhook_name, webhook_type, status, findings_count, error_message)
		 VALUES (?, ?, ?, ?, ?)`,
		name, webhookType, status, findingsCount, errMsg,
	)
	return err
}

// GetNotificationLog returns recent notification log entries.
func (d *DB) GetNotificationLog(limit int) ([]NotificationLogEntry, error) {
	rows, err := d.db.Query(
		`SELECT id, webhook_name, webhook_type, status, findings_count, COALESCE(error_message, ''), created_at
		 FROM notification_log
		 ORDER BY created_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []NotificationLogEntry
	for rows.Next() {
		var e NotificationLogEntry
		if err := rows.Scan(&e.ID, &e.WebhookName, &e.WebhookType, &e.Status, &e.FindingsCount, &e.ErrorMessage, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetNotificationLogRange returns notification attempts within a time range, newest first.
func (d *DB) GetNotificationLogRange(start, end time.Time, limit int) ([]NotificationLogEntry, error) {
	if end.Before(start) {
		start, end = end, start
	}
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	rows, err := d.db.Query(
		`SELECT id, webhook_name, webhook_type, status, findings_count, COALESCE(error_message, ''), created_at
		 FROM notification_log
		 WHERE created_at >= ? AND created_at <= ?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		start, end, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []NotificationLogEntry
	for rows.Next() {
		var e NotificationLogEntry
		if err := rows.Scan(&e.ID, &e.WebhookName, &e.WebhookType, &e.Status, &e.FindingsCount, &e.ErrorMessage, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
