package storage

import (
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// TestSaveServiceCheckResult_PersistsDetailsJSON — a scheduled check run
// that attaches a per-type Details map (HTTP status_code, DNS records, …)
// must round-trip through SQLite so the /service-checks log UI can render
// the same diagnostic context that the Test button already shows. See
// issue #182.
func TestSaveServiceCheckResult_PersistsDetailsJSON(t *testing.T) {
	db := newTestDB(t)

	now := time.Now().UTC()
	recs := []string{"1.2.3.4", "5.6.7.8"}
	results := []internal.ServiceCheckResult{{
		Key:              "svc-http",
		Name:             "HTTP With Details",
		Type:             "http",
		Target:           "https://example.com/health",
		Status:           "down",
		ResponseMS:       42,
		FailureThreshold: 1,
		FailureSeverity:  internal.SeverityWarning,
		CheckedAt:        now.Format(time.RFC3339),
		Details: map[string]any{
			"status_code":   404,
			"content_type":  "text/html; charset=utf-8",
			"body_bytes":    int64(1234),
			"final_url":     "https://example.com/health",
			"failure_stage": "http_status",
		},
	}, {
		Key:              "svc-dns",
		Name:             "DNS With Records",
		Type:             "dns",
		Target:           "example.com",
		Status:           "up",
		ResponseMS:       7,
		FailureThreshold: 1,
		FailureSeverity:  internal.SeverityWarning,
		CheckedAt:        now.Add(time.Second).Format(time.RFC3339),
		Details: map[string]any{
			"query_host": "example.com",
			"dns_server": "1.1.1.1:53",
			"records":    recs,
		},
	}}

	if err := db.SaveServiceCheckResults(results); err != nil {
		t.Fatalf("SaveServiceCheckResults: %v", err)
	}

	// ListLatestServiceChecks must echo back the Details map for both rows.
	entries, err := db.ListLatestServiceChecks(100)
	if err != nil {
		t.Fatalf("ListLatestServiceChecks: %v", err)
	}
	byKey := make(map[string]ServiceCheckEntry, len(entries))
	for _, e := range entries {
		byKey[e.Key] = e
	}

	httpEntry, ok := byKey["svc-http"]
	if !ok {
		t.Fatalf("svc-http not in ListLatestServiceChecks result; got keys %v", mapKeys(byKey))
	}
	if httpEntry.Details == nil {
		t.Fatalf("svc-http: expected Details populated, got nil (the persistence path still strips the map)")
	}
	// JSON round-trip turns numbers into float64 by default — assert on
	// the concrete JSON representation rather than int.
	if code, _ := asFloat(httpEntry.Details["status_code"]); code != 404 {
		t.Errorf("svc-http: expected status_code=404, got %v (%T)", httpEntry.Details["status_code"], httpEntry.Details["status_code"])
	}
	if stage, _ := httpEntry.Details["failure_stage"].(string); stage != "http_status" {
		t.Errorf("svc-http: expected failure_stage=http_status, got %v", httpEntry.Details["failure_stage"])
	}

	dnsEntry, ok := byKey["svc-dns"]
	if !ok {
		t.Fatalf("svc-dns missing from list; got keys %v", mapKeys(byKey))
	}
	if dnsEntry.Details == nil {
		t.Fatalf("svc-dns: expected Details populated, got nil")
	}
	// DNS records round-trip as []interface{} through encoding/json.
	records, ok := dnsEntry.Details["records"].([]interface{})
	if !ok {
		t.Fatalf("svc-dns: records should be []interface{}, got %T: %v", dnsEntry.Details["records"], dnsEntry.Details["records"])
	}
	if len(records) != 2 || records[0].(string) != "1.2.3.4" {
		t.Fatalf("svc-dns: expected [1.2.3.4 5.6.7.8], got %v", records)
	}

	// GetServiceCheckHistory must also return Details for a per-key query.
	hist, err := db.GetServiceCheckHistory("svc-http", 10)
	if err != nil {
		t.Fatalf("GetServiceCheckHistory: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("expected 1 history row for svc-http, got %d", len(hist))
	}
	if hist[0].Details == nil {
		t.Fatalf("GetServiceCheckHistory: expected Details populated, got nil")
	}
	if code, _ := asFloat(hist[0].Details["status_code"]); code != 404 {
		t.Errorf("GetServiceCheckHistory: expected status_code=404, got %v", hist[0].Details["status_code"])
	}
}

// TestSaveServiceCheckResult_NilDetailsIsOK — legacy code paths (and the
// pre-migration world) produce results with no Details map at all. Those
// must still persist cleanly and round-trip as nil / empty so the log UI
// doesn't choke.
func TestSaveServiceCheckResult_NilDetailsIsOK(t *testing.T) {
	db := newTestDB(t)

	now := time.Now().UTC()
	results := []internal.ServiceCheckResult{{
		Key:              "svc-plain",
		Name:             "No Details",
		Type:             "tcp",
		Target:           "127.0.0.1:22",
		Status:           "up",
		ResponseMS:       1,
		FailureThreshold: 1,
		FailureSeverity:  internal.SeverityWarning,
		CheckedAt:        now.Format(time.RFC3339),
		Details:          nil,
	}}
	if err := db.SaveServiceCheckResults(results); err != nil {
		t.Fatalf("SaveServiceCheckResults: %v", err)
	}
	entries, err := db.ListLatestServiceChecks(100)
	if err != nil {
		t.Fatalf("ListLatestServiceChecks: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Details != nil {
		t.Errorf("expected Details nil for row saved without details, got %v", entries[0].Details)
	}
}

// TestMigration_AddsDetailsJsonColumn — Open on an existing database
// without the details_json column must transparently add it and leave
// historical rows with a NULL value (rendered as nil Details on read).
func TestMigration_AddsDetailsJsonColumn(t *testing.T) {
	db := newTestDB(t)

	// Simulate a pre-migration state: the fresh newTestDB has the column.
	// Drop it (SQLite 3.35+) to recreate a pre-migration shape, then
	// re-run migrations to prove the ensureColumn path is idempotent and
	// actually re-adds the column on a DB missing it.
	if _, err := db.db.Exec(`ALTER TABLE service_checks_history DROP COLUMN details_json`); err != nil {
		// Older SQLite builds without DROP COLUMN support — the test
		// still serves its purpose on fresh runs (ensureColumn no-ops),
		// but we can't assert the re-add path. Skip rather than fail.
		t.Skipf("SQLite does not support DROP COLUMN on this build: %v", err)
	}
	if columnExists(t, db, "service_checks_history", "details_json") {
		t.Fatalf("precondition: details_json should have been dropped")
	}

	// Seed a legacy row while the column is absent.
	if _, err := db.db.Exec(
		`INSERT INTO service_checks_history (check_key, name, check_type, target, status, response_ms, error_message, consecutive_failures, failure_threshold, failure_severity, checked_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"legacy", "Legacy Row", "http", "http://x", "up", 1, "", 0, 1, "warning", time.Now().UTC(),
	); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	// Re-run migrations. migrate() is the internal helper the constructor
	// calls; re-running it on an already-open DB must be a no-op for
	// everything except the ensureColumn lines, which should add the
	// dropped details_json column back.
	if err := db.migrate(); err != nil {
		t.Fatalf("migrate (re-run): %v", err)
	}
	if !columnExists(t, db, "service_checks_history", "details_json") {
		t.Fatalf("runMigrations did not re-add details_json column")
	}
	// Legacy row must still be readable and have nil Details.
	entries, err := db.ListLatestServiceChecks(100)
	if err != nil {
		t.Fatalf("ListLatestServiceChecks: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 legacy entry, got %d", len(entries))
	}
	if entries[0].Details != nil {
		t.Errorf("expected legacy row Details nil after migration, got %v", entries[0].Details)
	}
}

// ── helpers ────────────────────────────────────────────────────────────

func mapKeys(m map[string]ServiceCheckEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// asFloat normalises JSON-decoded numerics: encoding/json produces float64
// for generic map[string]any, so the assertion can't assume int.
func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

func columnExists(t *testing.T, db *DB, table, column string) bool {
	t.Helper()
	rows, err := db.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid         int
			name, ctype string
			notnull, pk int
			dflt        any
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}
