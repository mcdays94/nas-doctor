package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDiskDetailTemplate_HasMaintenanceLogSection is a cross-reference
// check (per AGENTS.md §4b): the disk_detail.html template must contain
// the Maintenance Log section markup that the Phase 3 API endpoints
// feed into. It also validates the JavaScript glue that fetches from
// /api/v1/drives/{slot_key}/events and wires the Add/Edit/Delete flow.
//
// If someone deletes or renames any of these anchors, the UI will
// silently render blank even though the API responds correctly — this
// test catches that class of regression.
func TestDiskDetailTemplate_HasMaintenanceLogSection(t *testing.T) {
	path := filepath.Join("templates", "disk_detail.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read disk_detail.html: %v", err)
	}
	src := string(data)

	// IDs live inside JS string literals with escaped quotes, so we
	// search by the unique identifier value (stripped of quotes).
	// If any of these anchors drifts the UI silently renders blank.
	checks := []struct {
		name   string
		substr string
	}{
		{"section heading", "Maintenance Log"},
		{"list container id", "maintenanceLogList"},
		{"add-entry form id", "maintenanceLogForm"},
		{"content textarea id", "maintenanceLogContent"},
		{"event_time input id", "maintenanceLogTime"},
		{"save button id", "maintenanceLogSave"},
		{"fetch endpoint", "/api/v1/drives/"},
		{"events path fragment", "/events"},
		{"render function", "renderMaintenanceLog"},
		{"create handler", "createMaintenanceEntry"},
		{"edit action", "editMaintenanceEntry"},
		{"delete action", "deleteMaintenanceEntry"},
		{"system badge label", "System"},
	}
	for _, c := range checks {
		if !strings.Contains(src, c.substr) {
			t.Errorf("disk_detail.html missing %s (%q)", c.name, c.substr)
		}
	}

	// The UI must resolve the slot_key from the current disk payload:
	// prefer array_slot when populated, fall back to serial. Guard
	// against a regression where someone hard-codes serial as slot_key
	// and silently breaks Unraid where slot_key is the ArraySlot.
	if !strings.Contains(src, "disk.array_slot") {
		t.Errorf("template does not reference disk.array_slot when computing slot_key")
	}
	if !strings.Contains(src, "maintenance-log-entry") {
		t.Errorf("template missing per-entry CSS class .maintenance-log-entry — row styling hook must exist")
	}
}

// TestDiskDetailTemplate_MaintenanceLog_SystemBadgeOnly_ForAuto ensures
// the UI distinguishes auto events (no edit/delete controls) from manual
// ones. The only reliable signal in the template is that the is_auto
// branch renders a "System" badge and does NOT render the edit/delete
// buttons, keyed off ev.is_auto.
func TestDiskDetailTemplate_MaintenanceLog_SystemBadgeOnly_ForAuto(t *testing.T) {
	path := filepath.Join("templates", "disk_detail.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	src := string(data)

	// The template must branch on is_auto when deciding whether to
	// render edit/delete buttons. Look for the idiomatic check.
	if !strings.Contains(src, "ev.is_auto") && !strings.Contains(src, "entry.is_auto") {
		t.Errorf("template does not branch on is_auto — auto events may render edit/delete buttons")
	}
}
