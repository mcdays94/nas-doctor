package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

// TestSettingsDefault_DockerHiddenContainersIsEmpty verifies that a fresh
// defaultSettings() has no hidden containers — i.e. the current "show
// everything" behavior is preserved for new installs and upgraders alike.
// Issue #204.
func TestSettingsDefault_DockerHiddenContainersIsEmpty(t *testing.T) {
	d := defaultSettings()
	if len(d.DockerHiddenContainers) != 0 {
		t.Errorf("defaultSettings().DockerHiddenContainers = %v; want empty (nil or []) — hiding must be opt-in (#204)", d.DockerHiddenContainers)
	}
}

// TestSettings_DockerHiddenContainers_RoundTrip exercises the PUT/GET cycle
// for the new field against the real settings handlers, guarding against
// JSON tag typos.
func TestSettings_DockerHiddenContainers_RoundTrip(t *testing.T) {
	srv := newSettingsTestServer()

	putBody := map[string]interface{}{
		"scan_interval":            "30m",
		"theme":                    "midnight",
		"docker_hidden_containers": []string{"watchtower", "portainer-agent"},
	}
	buf, _ := json.Marshal(putBody)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.handleUpdateSettings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/v1/settings returned %d: %s", rec.Code, rec.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rec2 := httptest.NewRecorder()
	srv.handleGetSettings(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/settings returned %d", rec2.Code)
	}
	body, _ := io.ReadAll(rec2.Body)
	var got Settings
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("parse GET response: %v", err)
	}
	want := []string{"watchtower", "portainer-agent"}
	if !reflect.DeepEqual(got.DockerHiddenContainers, want) {
		t.Errorf("DockerHiddenContainers round-trip failed: got %v, want %v", got.DockerHiddenContainers, want)
	}
}

// TestHandleLatestSnapshot_FiltersHiddenDockerContainers verifies that the
// snapshot endpoint strips hidden containers from DockerInfo.Containers
// before serializing the response. Filtering server-side keeps hidden
// container names off the wire entirely.
//
// Scope boundary (see issue #204): only the Docker section's container
// list is filtered. Top Processes container attribution is explicitly
// preserved — see TestHandleLatestSnapshot_DoesNotFilterTopProcessesContainerAttribution.
func TestHandleLatestSnapshot_FiltersHiddenDockerContainers(t *testing.T) {
	srv := newSettingsTestServer()

	// Configure hidden containers.
	settings := defaultSettings()
	settings.DockerHiddenContainers = []string{"watchtower", "portainer-agent"}
	data, _ := json.Marshal(settings)
	if err := srv.store.SetConfig(settingsConfigKey, string(data)); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	// Seed a snapshot with 5 containers, 2 of which should be hidden.
	snap := &internal.Snapshot{
		ID: "test-snap",
		Docker: internal.DockerInfo{
			Available: true,
			Containers: []internal.ContainerInfo{
				{ID: "a", Name: "plex", State: "running"},
				{ID: "b", Name: "sonarr", State: "running"},
				{ID: "c", Name: "watchtower", State: "running"},
				{ID: "d", Name: "portainer-agent", State: "running"},
				{ID: "e", Name: "radarr", State: "running"},
			},
		},
	}
	if err := srv.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot/latest", nil)
	rec := httptest.NewRecorder()
	srv.handleLatestSnapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/snapshot/latest returned %d: %s", rec.Code, rec.Body.String())
	}
	var got internal.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}

	names := make([]string, 0, len(got.Docker.Containers))
	for _, c := range got.Docker.Containers {
		names = append(names, c.Name)
	}
	for _, hidden := range []string{"watchtower", "portainer-agent"} {
		for _, n := range names {
			if n == hidden {
				t.Errorf("hidden container %q leaked into snapshot response: %v", hidden, names)
			}
		}
	}
	wantVisible := map[string]bool{"plex": true, "sonarr": true, "radarr": true}
	if len(names) != len(wantVisible) {
		t.Errorf("expected %d visible containers, got %d: %v", len(wantVisible), len(names), names)
	}
	for _, n := range names {
		if !wantVisible[n] {
			t.Errorf("unexpected container in response: %q", n)
		}
	}
	if got.Docker.HiddenCount != 2 {
		t.Errorf("DockerInfo.HiddenCount = %d; want 2", got.Docker.HiddenCount)
	}
}

// TestHandleLatestSnapshot_NoHiddenContainersPreservesAllContainers is the
// regression guard: when DockerHiddenContainers is empty (the default), the
// full container list must survive the snapshot endpoint unchanged and
// HiddenCount must be zero.
func TestHandleLatestSnapshot_NoHiddenContainersPreservesAllContainers(t *testing.T) {
	srv := newSettingsTestServer()

	snap := &internal.Snapshot{
		ID: "test-snap",
		Docker: internal.DockerInfo{
			Available: true,
			Containers: []internal.ContainerInfo{
				{ID: "a", Name: "plex", State: "running"},
				{ID: "b", Name: "sonarr", State: "running"},
				{ID: "c", Name: "watchtower", State: "running"},
			},
		},
	}
	if err := srv.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot/latest", nil)
	rec := httptest.NewRecorder()
	srv.handleLatestSnapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/snapshot/latest returned %d: %s", rec.Code, rec.Body.String())
	}
	var got internal.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	if len(got.Docker.Containers) != 3 {
		t.Errorf("expected 3 containers when no hiding configured, got %d", len(got.Docker.Containers))
	}
	if got.Docker.HiddenCount != 0 {
		t.Errorf("DockerInfo.HiddenCount = %d; want 0 when no hiding configured", got.Docker.HiddenCount)
	}
}

// TestHandleLatestSnapshot_DoesNotFilterTopProcessesContainerAttribution
// is the scope-boundary guard for issue #204. Even when a container is
// in DockerHiddenContainers, any TopProcess attributed to that container
// must STILL carry the container name in the response.
//
// Rationale: users want to know WHICH container is chewing CPU even when
// they've hidden the container-list tile for scroll-length reasons. The
// hiding is a rendering preference, not a data-suppression directive.
func TestHandleLatestSnapshot_DoesNotFilterTopProcessesContainerAttribution(t *testing.T) {
	srv := newSettingsTestServer()

	settings := defaultSettings()
	settings.DockerHiddenContainers = []string{"watchtower"}
	data, _ := json.Marshal(settings)
	if err := srv.store.SetConfig(settingsConfigKey, string(data)); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	snap := &internal.Snapshot{
		ID: "test-snap",
		System: internal.SystemInfo{
			TopProcesses: []internal.ProcessInfo{
				{PID: 100, Command: "watchtower", ContainerName: "watchtower", ContainerID: "abc", CPU: 42.0},
				{PID: 200, Command: "plexmediaserver", ContainerName: "plex", ContainerID: "def", CPU: 30.0},
			},
		},
		Docker: internal.DockerInfo{Available: true},
	}
	if err := srv.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot/latest", nil)
	rec := httptest.NewRecorder()
	srv.handleLatestSnapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/snapshot/latest returned %d", rec.Code)
	}
	var got internal.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	if len(got.System.TopProcesses) != 2 {
		t.Fatalf("expected 2 top processes, got %d", len(got.System.TopProcesses))
	}
	found := false
	for _, p := range got.System.TopProcesses {
		if p.PID == 100 {
			found = true
			if p.ContainerName != "watchtower" {
				t.Errorf("hidden container attribution was stripped from top process: ContainerName=%q, want %q (scope boundary violation: filter must NOT apply to Top Processes)", p.ContainerName, "watchtower")
			}
		}
	}
	if !found {
		t.Error("top process with PID 100 (attributed to hidden container 'watchtower') was dropped from response; it should still appear")
	}
}

// TestDockerInfo_HiddenCountJSONField verifies the new HiddenCount field
// on DockerInfo serializes as docker_hidden_count so the frontend can
// render "Containers (N shown, M hidden)" headers.
func TestDockerInfo_HiddenCountJSONField(t *testing.T) {
	d := internal.DockerInfo{HiddenCount: 3}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	v, ok := m["hidden_count"]
	if !ok {
		t.Fatalf("DockerInfo JSON missing hidden_count; got keys %v", keysOf(m))
	}
	if fv, ok := v.(float64); !ok || int(fv) != 3 {
		t.Errorf("hidden_count = %v; want 3", v)
	}
}

func keysOf(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestSettingsHTMLIncludesDockerHiddenContainersCheckboxList verifies the
// settings page template ships the Advanced-section checkbox-list UI for
// docker_hidden_containers with the load/save wiring. v0.9.6 UX rework:
// the old comma-separated text input was replaced with a checkbox list
// populated from live containers + ghost entries for stored-but-not-
// running names. Cross-reference test: any future refactor that renames
// one side without the other will break this.
func TestSettingsHTMLIncludesDockerHiddenContainersCheckboxList(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	content := string(data)

	checks := []struct {
		name   string
		substr string
	}{
		// Container div that JS populates with checkboxes (replaces old text input).
		{"checkbox list container", `id="docker-hidden-containers-list"`},
		// Lives inside the Advanced card (regression: not inside Dashboard Sections).
		{"advanced card anchor", `id="card-advanced"`},
		// Load path reads the JSON field and hands off to the renderer.
		{"load binds field", `data.docker_hidden_containers`},
		{"load calls renderer", `loadDockerHiddenContainersCheckboxes(hidden)`},
		// Save payload emits the JSON field via the collector.
		{"save sends field", `docker_hidden_containers: collectCheckedHiddenContainers()`},
		// Helper text should mention tick-boxes (not commas) and scope.
		{"helper mentions ticking", `Tick the containers`},
		{"helper mentions docker section", `Docker Containers section`},
		// The key JS helpers must exist by their stable names.
		{"renderer defined", `function renderDockerHiddenContainersCheckboxes`},
		{"collector defined", `function collectCheckedHiddenContainers`},
		// Checkbox class used to collect checked state at save time.
		{"checkbox class", `docker-hidden-checkbox`},
		// Ghost-entry label for stored names not currently running.
		{"ghost label", `not running`},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(content, tc.substr) {
				t.Errorf("settings.html missing %q — expected substring: %q", tc.name, tc.substr)
			}
		})
	}
}

// TestSettingsHTMLDoesNotShipCommaTextInput is a negative test for the
// UX rework. The old text input id and the parse helper must NOT be
// present — if they reappear, it means a stale copy was restored and
// we'd be shipping the worse UX by accident.
func TestSettingsHTMLDoesNotShipCommaTextInput(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	content := string(data)

	forbidden := []string{
		// Old text input id (the one people used to type commas into).
		// The NEW id is docker-hidden-containers-list — checked in the
		// positive test above. This guard catches a regression to the
		// old input element specifically.
		`id="docker-hidden-containers"`,
		// Old parse helper; replaced by collectCheckedHiddenContainers.
		`function parseHiddenContainerList`,
		// Old placeholder copy hinting at comma-separated input.
		`Comma-separated container names`,
	}
	for _, substr := range forbidden {
		if strings.Contains(content, substr) {
			t.Errorf("settings.html still contains legacy comma-input artifact: %q", substr)
		}
	}
}

// TestDashboardJS_DockerSectionHeaderShowsHiddenCount verifies the client-
// side Docker section renderer reads hidden_count and renders the
// "(N shown, M hidden)" header format.
func TestDashboardJS_DockerSectionHeaderShowsHiddenCount(t *testing.T) {
	js := DashboardJS
	checks := []struct {
		name   string
		substr string
	}{
		// Renderer must read hidden_count off the docker payload.
		{"reads hidden_count", "hidden_count"},
		// And include both words in the header format.
		{"header format — hidden word", "hidden"},
		{"header format — shown word", "shown"},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(js, tc.substr) {
				t.Errorf("DashboardJS missing %q — expected substring: %q", tc.name, tc.substr)
			}
		})
	}
}

// (Removed: TestSettingsHTML_DockerHiddenContainers_ParseHelperHandlesWhitespace)
//
// The v0.9.6 UX rework replaced the comma-separated text input with a
// checkbox list, so parseHiddenContainerList no longer exists. Its
// positive coverage is now split across:
//   - TestSettingsHTMLIncludesDockerHiddenContainersCheckboxList
//     (checkbox list + renderer + collector are all present)
//   - TestSettingsHTMLDoesNotShipCommaTextInput (negative guard —
//     the parse helper must NOT come back)

// TestSettings_DockerHiddenContainers_EmptyArrayDoesNotFilter verifies that
// an explicitly-empty list (as opposed to nil) behaves identically to nil:
// nothing is filtered, HiddenCount is zero. Guards against a future
// refactor that conflates nil vs [] in an "is hiding configured" check.
func TestSettings_DockerHiddenContainers_EmptyArrayDoesNotFilter(t *testing.T) {
	srv := newSettingsTestServer()

	settings := defaultSettings()
	settings.DockerHiddenContainers = []string{} // explicit empty, not nil
	data, _ := json.Marshal(settings)
	if err := srv.store.SetConfig(settingsConfigKey, string(data)); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	snap := &internal.Snapshot{
		ID: "test-snap",
		Docker: internal.DockerInfo{
			Available: true,
			Containers: []internal.ContainerInfo{
				{ID: "a", Name: "plex", State: "running"},
				{ID: "b", Name: "watchtower", State: "running"},
			},
		},
	}
	if err := srv.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot/latest", nil)
	rec := httptest.NewRecorder()
	srv.handleLatestSnapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot returned %d", rec.Code)
	}
	var got internal.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	if len(got.Docker.Containers) != 2 {
		t.Errorf("empty DockerHiddenContainers should not filter; got %d containers, want 2", len(got.Docker.Containers))
	}
	if got.Docker.HiddenCount != 0 {
		t.Errorf("HiddenCount = %d; want 0 for empty hidden list", got.Docker.HiddenCount)
	}
}
