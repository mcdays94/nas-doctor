// Package storage handles SQLite persistence for snapshots, findings, and config.
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database.
type DB struct {
	db     *sql.DB
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

	d := &DB{db: sqldb, logger: logger}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
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
	}

	for _, m := range migrations {
		if _, err := d.db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, m)
		}
	}
	return nil
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
	for _, table := range []string{"smart_history", "system_history"} {
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
		 FROM smart_history
		 WHERE serial = ?
		 ORDER BY timestamp ASC
		 LIMIT ?`,
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
