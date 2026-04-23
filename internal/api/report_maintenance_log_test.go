package api

import (
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// TestReportExport_IncludesMaintenanceLog verifies the Diagnostic Report
// includes per-drive maintenance log sections when events exist, and
// that replacement events are expanded into a human-readable one-liner
// rather than raw JSON.
func TestReportExport_IncludesMaintenanceLog(t *testing.T) {
	ts := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	snap := &internal.Snapshot{
		ID:        "snap-1",
		Timestamp: ts,
		System:    internal.SystemInfo{Hostname: "testhost", Platform: "unraid"},
		SMART: []internal.SMARTInfo{
			{Device: "/dev/sdb", Model: "WDC 4TB", Serial: "WD-NEW", ArraySlot: "disk1", HealthPassed: true, SizeGB: 4000},
			{Device: "/dev/sdc", Model: "Seagate 6TB", Serial: "ST-1", ArraySlot: "disk2", HealthPassed: true, SizeGB: 6000},
		},
	}
	sparks := ReportSparklines{
		DriveEventsBySlot: map[string][]storage.DriveEvent{
			"disk1": {
				{
					ID:        2,
					SlotKey:   "disk1",
					Platform:  "unraid",
					EventType: "note",
					EventTime: ts,
					Content:   "SATA cable replaced",
					IsAuto:    false,
				},
				{
					ID:        1,
					SlotKey:   "disk1",
					Platform:  "unraid",
					EventType: "replacement",
					EventTime: ts.Add(-24 * time.Hour),
					Content:   `{"old_serial":"WD-OLD","old_model":"WDC 2TB","new_serial":"WD-NEW","new_model":"WDC 4TB"}`,
					IsAuto:    true,
				},
			},
			// disk2 deliberately has no events — should NOT appear in the log section.
		},
	}

	htmlOut := GenerateReport(snap, sparks)

	// Section heading.
	if !strings.Contains(htmlOut, "Maintenance Log") {
		t.Errorf("report missing 'Maintenance Log' heading")
	}
	// disk1 title + manual note content present.
	if !strings.Contains(htmlOut, "disk1") || !strings.Contains(htmlOut, "WDC 4TB") {
		t.Errorf("report missing disk1/WDC 4TB title block")
	}
	if !strings.Contains(htmlOut, "SATA cable replaced") {
		t.Errorf("report missing manual note content")
	}
	// Replacement event rendered as human-readable line, NOT raw JSON.
	if strings.Contains(htmlOut, `"old_serial"`) {
		t.Errorf("report shows raw JSON for replacement event; should parse to human-readable line")
	}
	if !strings.Contains(htmlOut, "Drive replaced") {
		t.Errorf("report missing 'Drive replaced' summary for replacement event")
	}
	if !strings.Contains(htmlOut, "WD-OLD") || !strings.Contains(htmlOut, "WD-NEW") {
		t.Errorf("report missing old/new serial in replacement summary")
	}
	// [system] marker on auto events.
	if !strings.Contains(htmlOut, "[system]") {
		t.Errorf("report missing [system] marker on auto replacement event")
	}
	// Timestamps formatted (not zero).
	if !strings.Contains(htmlOut, "2026-04-15 10:00") {
		t.Errorf("report missing formatted timestamp; got: want substring 2026-04-15 10:00")
	}
	// disk2 should not appear in a Maintenance Log entry — it has no events.
	// We verify this by splitting the HTML at the Maintenance Log heading.
	idx := strings.Index(htmlOut, "Maintenance Log")
	if idx < 0 {
		t.Fatal("Maintenance Log heading missing")
	}
	afterHeading := htmlOut[idx:]
	// Cut off after the next </div> … </div> closing the SMART section,
	// to avoid false positives from disk2 being mentioned in the main
	// SMART table above OR in the Docker/other sections below.
	if end := strings.Index(afterHeading, "</div>\n</div>\n"); end > 0 {
		afterHeading = afterHeading[:end]
	}
	if strings.Contains(afterHeading, "disk2") {
		t.Errorf("report Maintenance Log unexpectedly includes disk2 (which has no events)")
	}
}

// TestReportExport_NoMaintenanceLog_WhenNoEvents ensures the section is
// omitted entirely if no drives have events. The report should not
// render an empty heading.
func TestReportExport_NoMaintenanceLog_WhenNoEvents(t *testing.T) {
	snap := &internal.Snapshot{
		ID:        "snap-1",
		Timestamp: time.Now(),
		System:    internal.SystemInfo{Hostname: "testhost", Platform: "unraid"},
		SMART: []internal.SMARTInfo{
			{Device: "/dev/sdb", Model: "WDC", Serial: "S1", ArraySlot: "disk1"},
		},
	}
	sparks := ReportSparklines{}
	htmlOut := GenerateReport(snap, sparks)
	// Drive Health & SMART Analysis section should still render its heading.
	if !strings.Contains(htmlOut, "SMART Analysis") {
		t.Errorf("report missing SMART Analysis section")
	}
	// But no Maintenance Log heading when no events.
	if strings.Contains(htmlOut, "Maintenance Log") {
		t.Errorf("report contains empty Maintenance Log section when no events exist")
	}
}
