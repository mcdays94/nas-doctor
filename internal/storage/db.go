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
	}

	for _, m := range migrations {
		if _, err := d.db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, m)
		}
	}

	if err := d.ensureColumn("alerts", "fingerprint", "TEXT"); err != nil {
		return fmt.Errorf("ensure alerts.fingerprint: %w", err)
	}
	if _, err := d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_alerts_fingerprint_open ON alerts(fingerprint, resolved_at)`); err != nil {
		return fmt.Errorf("create alerts fingerprint index: %w", err)
	}

	return nil
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

// AlertStateFinding is the minimum finding shape required to track alert lifecycle.
type AlertStateFinding struct {
	Fingerprint string
	FindingID   string
	Severity    string
	Title       string
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

		if _, err := tx.Exec(
			`INSERT INTO alerts (finding_id, snapshot_id, severity, title, fingerprint)
			 VALUES (?, ?, ?, ?, ?)`,
			f.FindingID, snapshotID, f.Severity, f.Title, fp,
		); err != nil {
			return err
		}
	}

	for _, id := range openByFingerprint {
		if _, err := tx.Exec("UPDATE alerts SET resolved_at = ? WHERE id = ?", now, id); err != nil {
			return err
		}
	}

	return tx.Commit()
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
	FileSizeMB     float64 `json:"file_size_mb"`
	SnapshotCount  int     `json:"snapshot_count"`
	SMARTRows      int     `json:"smart_history_rows"`
	SystemRows     int     `json:"system_history_rows"`
	FindingRows    int     `json:"finding_rows"`
	NotifyLogRows  int     `json:"notify_log_rows"`
	AlertRows      int     `json:"alert_rows"`
	OldestSnapshot string  `json:"oldest_snapshot,omitempty"`
	NewestSnapshot string  `json:"newest_snapshot,omitempty"`
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
		for _, table := range []string{"smart_history", "system_history"} {
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
