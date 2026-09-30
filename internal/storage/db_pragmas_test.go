package storage

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// Open used to set its pragmas with one-off Exec calls, which only configure
// the pooled connection that runs them. Every other connection ran with
// busy_timeout=0 and foreign keys off, so a write that met another
// connection's write lock failed at once with "database is locked
// (SQLITE_BUSY)" and was dropped. These tests hold two connections at once
// so the pool has to open a second one.

func twoConns(t *testing.T, db *DB) (*sql.Conn, *sql.Conn) {
	t.Helper()
	ctx := context.Background()
	c1, err := db.db.Conn(ctx)
	if err != nil {
		t.Fatalf("first connection: %v", err)
	}
	t.Cleanup(func() { c1.Close() })
	c2, err := db.db.Conn(ctx)
	if err != nil {
		t.Fatalf("second connection: %v", err)
	}
	t.Cleanup(func() { c2.Close() })
	return c1, c2
}

func TestOpen_PragmasApplyToEveryConnection(t *testing.T) {
	db := newTestDB(t)
	c1, c2 := twoConns(t, db)
	ctx := context.Background()

	for i, c := range []*sql.Conn{c1, c2} {
		var busy, fk, sync int
		var mode string
		for _, p := range []struct {
			pragma string
			dst    any
		}{
			{"busy_timeout", &busy},
			{"foreign_keys", &fk},
			{"synchronous", &sync},
			{"journal_mode", &mode},
		} {
			if err := c.QueryRowContext(ctx, "PRAGMA "+p.pragma).Scan(p.dst); err != nil {
				t.Fatalf("connection %d: PRAGMA %s: %v", i+1, p.pragma, err)
			}
		}
		if busy != 5000 || fk != 1 || sync != 1 || mode != "wal" {
			t.Errorf("connection %d: busy_timeout=%d foreign_keys=%d synchronous=%d journal_mode=%s; want 5000, 1, 1 (NORMAL), wal",
				i+1, busy, fk, sync, mode)
		}
	}
}

// The production symptom: a write on one connection while another holds the
// write lock has to wait for the lock, not fail with SQLITE_BUSY.
func TestOpen_SecondConnectionWaitsForWriteLock(t *testing.T) {
	db := newTestDB(t)
	holder, writer := twoConns(t, db)
	ctx := context.Background()

	if _, err := holder.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("BEGIN IMMEDIATE: %v", err)
	}
	if _, err := holder.ExecContext(ctx, `INSERT INTO config (key, value) VALUES ('lock-holder', '{}')`); err != nil {
		t.Fatalf("insert on lock holder: %v", err)
	}
	committed := make(chan error, 1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		_, err := holder.ExecContext(ctx, "COMMIT")
		committed <- err
	}()

	if _, err := writer.ExecContext(ctx, `INSERT INTO config (key, value) VALUES ('waiter', '{}')`); err != nil {
		t.Errorf("write on the second connection failed instead of waiting for the lock: %v", err)
	}
	if err := <-committed; err != nil {
		t.Fatalf("COMMIT: %v", err)
	}
}
