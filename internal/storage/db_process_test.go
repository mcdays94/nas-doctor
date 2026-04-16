package storage

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

func TestDB_ProcessHistory_SaveAndQuery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	procs := []internal.ProcessInfo{
		{PID: 100, User: "root", CPU: 50.0, Mem: 20.0, Command: "/usr/bin/python3 server.py"},
		{PID: 200, User: "www", CPU: 30.0, Mem: 10.0, Command: "nginx: master process"},
		{PID: 300, User: "nobody", CPU: 10.0, Mem: 5.0, Command: "/usr/sbin/sshd"},
	}

	if err := db.SaveProcessStats(procs); err != nil {
		t.Fatalf("SaveProcessStats: %v", err)
	}

	history, err := db.GetProcessHistory(1) // last 1 hour
	if err != nil {
		t.Fatalf("GetProcessHistory: %v", err)
	}

	if len(history) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(history))
	}

	// Should be ordered by name ASC, then timestamp ASC.
	// nginx: < python3 < sshd
	expectedNames := []string{"nginx:", "python3", "sshd"}
	for i, name := range expectedNames {
		if history[i].Name != name {
			t.Errorf("row %d: expected name=%q, got %q", i, name, history[i].Name)
		}
	}

	// Verify specific fields for python3.
	var python ProcessHistoryPoint
	for _, h := range history {
		if h.Name == "python3" {
			python = h
			break
		}
	}
	if python.PID != 100 {
		t.Errorf("expected PID 100, got %d", python.PID)
	}
	if python.User != "root" {
		t.Errorf("expected user root, got %s", python.User)
	}
	if python.CPUPct != 50.0 {
		t.Errorf("expected CPU 50.0, got %f", python.CPUPct)
	}
	if python.MemPct != 20.0 {
		t.Errorf("expected Mem 20.0, got %f", python.MemPct)
	}
	if python.Command != "/usr/bin/python3 server.py" {
		t.Errorf("expected full command preserved, got %s", python.Command)
	}
}

func TestDB_ProcessHistory_SkipsEmptyName(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	procs := []internal.ProcessInfo{
		{PID: 1, User: "root", CPU: 1.0, Mem: 1.0, Command: ""},     // empty command → skip
		{PID: 2, User: "root", CPU: 2.0, Mem: 2.0, Command: "bash"}, // valid
	}

	if err := db.SaveProcessStats(procs); err != nil {
		t.Fatalf("SaveProcessStats: %v", err)
	}

	history, err := db.GetProcessHistory(1)
	if err != nil {
		t.Fatalf("GetProcessHistory: %v", err)
	}

	if len(history) != 1 {
		t.Fatalf("expected 1 row (empty-name skipped), got %d", len(history))
	}
	if history[0].Name != "bash" {
		t.Errorf("expected name=bash, got %s", history[0].Name)
	}
}

func TestDB_ProcessHistory_EmptyInput(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Nil and empty slices should not error.
	if err := db.SaveProcessStats(nil); err != nil {
		t.Fatalf("SaveProcessStats(nil): %v", err)
	}
	if err := db.SaveProcessStats([]internal.ProcessInfo{}); err != nil {
		t.Fatalf("SaveProcessStats([]): %v", err)
	}

	history, err := db.GetProcessHistory(1)
	if err != nil {
		t.Fatalf("GetProcessHistory: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected 0 rows, got %d", len(history))
	}
}

func TestProcessName(t *testing.T) {
	tests := []struct {
		command string
		want    string
	}{
		{"/usr/bin/python3 app.py", "python3"},
		{"nginx: worker process", "nginx:"},
		{"/usr/sbin/sshd -D", "sshd"},
		{"bash", "bash"},
		{"/bin/sh", "sh"},
		{"", ""},
		{"   ", ""},
	}

	for _, tc := range tests {
		got := processName(tc.command)
		if got != tc.want {
			t.Errorf("processName(%q) = %q, want %q", tc.command, got, tc.want)
		}
	}
}
