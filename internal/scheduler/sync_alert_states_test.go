package scheduler

import (
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/notifier"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// SyncAlertStates turns a snapshot's findings into open alerts. Demo mode
// relies on it because it saves its snapshot without running a scan.
func TestSyncAlertStates_OpensAnAlertPerFinding(t *testing.T) {
	db, err := storage.Open(t.TempDir()+"/test.db", silentTestLogger())
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	defer db.Close()
	s := New(collector.New(internal.HostPaths{}, silentTestLogger()), db, &notifier.Notifier{}, nil, silentTestLogger(), 30*time.Minute)

	now := time.Now()
	snap := &internal.Snapshot{ID: "snap-1", Timestamp: now, Findings: []internal.Finding{
		{ID: "f1", Severity: internal.SeverityCritical, Category: internal.CategorySMART, Title: "Reallocated Sectors on /dev/sde"},
		{ID: "f2", Severity: internal.SeverityWarning, Category: internal.CategoryDisk, Title: "Low Disk Space: /mnt/disk3 (96%)"},
	}}
	if err := db.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	s.SyncAlertStates(snap)

	alerts, err := db.ListAlerts("open", 10, now)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("got %d open alerts; want 2", len(alerts))
	}
}
