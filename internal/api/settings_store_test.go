package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/fleet"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

func putSettingsJSON(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	srv.handleUpdateSettings(rr, httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader(body)))
	return rr
}

func getSettingsJSON(t *testing.T, srv *Server) map[string]any {
	t.Helper()
	rr := httptest.NewRecorder()
	srv.handleGetSettings(rr, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings = %d: %s", rr.Code, rr.Body.String())
	}
	var m map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestUpdateSettings_MergesInsteadOfReplacing is the regression test for
// "any save from the settings page resets everything the page did not
// send". A save now only changes what it mentions.
func TestUpdateSettings_MergesInsteadOfReplacing(t *testing.T) {
	srv := newSettingsTestServer()

	seed := `{
		"scan_interval": "6h",
		"chart_range_hours": 24,
		"section_heights": {"docker": 420},
		"section_order": {"cols": ["findings", "docker"]},
		"dismissed_findings": ["Aging Drive: /dev/sdb"],
		"cost_per_tb": 19.5,
		"retention": {"snapshot_days": 30, "max_db_size_mb": 200, "notify_log_days": 7},
		"backup": {"enabled": false, "path": "/mnt/backups", "keep_count": 9, "interval_hours": 24},
		"notifications": {
			"default_cooldown_sec": 3600,
			"quiet_hours": {"enabled": true, "timezone": "Europe/Lisbon", "start_hhmm": "23:30", "end_hhmm": "06:15"},
			"webhooks": [{"name": "hook", "url": "http://127.0.0.1:9/x", "type": "generic", "enabled": true}]
		}
	}`
	if rr := putSettingsJSON(t, srv, seed); rr.Code != http.StatusOK {
		t.Fatalf("seed PUT = %d: %s", rr.Code, rr.Body.String())
	}

	// A client that only knows about one section toggle.
	if rr := putSettingsJSON(t, srv, `{"sections": {"gpu": false}}`); rr.Code != http.StatusOK {
		t.Fatalf("partial PUT = %d: %s", rr.Code, rr.Body.String())
	}

	got := srv.getSettings()
	if got.Sections.GPU {
		t.Error("the field that was sent (sections.gpu=false) was not applied")
	}
	if !got.Sections.Findings || !got.Sections.Docker {
		t.Error("sibling section toggles were reset by a partial sections object")
	}
	if got.ScanInterval != "6h" {
		t.Errorf("scan_interval = %q, want 6h", got.ScanInterval)
	}
	if got.ChartRangeHours != 24 {
		t.Errorf("chart_range_hours = %d, want 24", got.ChartRangeHours)
	}
	if got.SectionHeights["docker"] != 420 || len(got.SectionOrder["cols"]) != 2 {
		t.Errorf("dashboard layout lost: heights=%v order=%v", got.SectionHeights, got.SectionOrder)
	}
	if len(got.DismissedFindings) != 1 {
		t.Errorf("dismissed_findings = %v, want 1 entry", got.DismissedFindings)
	}
	if got.CostPerTB != 19.5 {
		t.Errorf("cost_per_tb = %v, want 19.5", got.CostPerTB)
	}
	if got.Retention != (RetentionSettings{SnapshotDays: 30, MaxDBSizeMB: 200, NotifyLogDays: 7}) {
		t.Errorf("retention reset: %+v", got.Retention)
	}
	if got.Backup.Enabled || got.Backup.Path != "/mnt/backups" || got.Backup.KeepCount != 9 || got.Backup.IntervalH != 24 {
		t.Errorf("backup reset: %+v", got.Backup)
	}
	qh := got.Notifications.QuietHours
	if !qh.Enabled || qh.Timezone != "Europe/Lisbon" || qh.StartHHMM != "23:30" || qh.EndHHMM != "06:15" {
		t.Errorf("quiet hours reset: %+v", qh)
	}
	if got.Notifications.DefaultCooldownSec != 3600 {
		t.Errorf("cooldown = %d, want 3600", got.Notifications.DefaultCooldownSec)
	}
	if len(got.Notifications.Webhooks) != 1 {
		t.Errorf("webhooks = %d, want 1", len(got.Notifications.Webhooks))
	}
}

// TestUpdateSettings_ArraysReplaceWhole: lists are not merged element by
// element — sending a list is how a client removes entries from it.
func TestUpdateSettings_ArraysReplaceWhole(t *testing.T) {
	srv := newSettingsTestServer()
	two := `{"notifications":{"webhooks":[
		{"name":"a","url":"http://127.0.0.1:9/a","type":"generic","enabled":true,"headers":{"X-A":"1"}},
		{"name":"b","url":"http://127.0.0.1:9/b","type":"generic","enabled":true}]}}`
	if rr := putSettingsJSON(t, srv, two); rr.Code != http.StatusOK {
		t.Fatalf("PUT = %d: %s", rr.Code, rr.Body.String())
	}
	one := `{"notifications":{"webhooks":[{"name":"b","url":"http://127.0.0.1:9/b","type":"generic","enabled":true}]}}`
	if rr := putSettingsJSON(t, srv, one); rr.Code != http.StatusOK {
		t.Fatalf("PUT = %d: %s", rr.Code, rr.Body.String())
	}
	whs := srv.getSettings().Notifications.Webhooks
	if len(whs) != 1 || whs[0].Name != "b" {
		t.Fatalf("webhooks = %+v, want just b", whs)
	}
	if len(whs[0].Headers) != 0 {
		t.Errorf("webhook b inherited headers from the entry it replaced: %v", whs[0].Headers)
	}
	// An explicit empty list clears it.
	if rr := putSettingsJSON(t, srv, `{"notifications":{"webhooks":[]}}`); rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}
	if n := len(srv.getSettings().Notifications.Webhooks); n != 0 {
		t.Errorf("webhooks after [] = %d, want 0", n)
	}
}

// TestUpdateSettings_LayoutMapsReplaceWhole: the dashboard layout maps are
// owned whole by the client, so a key left out is a key removed.
func TestUpdateSettings_LayoutMapsReplaceWhole(t *testing.T) {
	srv := newSettingsTestServer()
	putSettingsJSON(t, srv, `{"section_heights":{"docker":300,"zfs":200}}`)
	putSettingsJSON(t, srv, `{"section_heights":{"docker":310}}`)
	h := srv.getSettings().SectionHeights
	if len(h) != 1 || h["docker"] != 310 {
		t.Errorf("section_heights = %v, want only docker=310", h)
	}
}

func TestUpdateSettings_RejectsNonObjectBodies(t *testing.T) {
	srv := newSettingsTestServer()
	putSettingsJSON(t, srv, `{"api_key":"nd-keep-me","scan_interval":"6h"}`)

	for _, body := range []string{`null`, `[]`, `"x"`, `42`, ``, `{"a":1} trailing`} {
		if rr := putSettingsJSON(t, srv, body); rr.Code != http.StatusBadRequest {
			t.Errorf("PUT %q = %d, want 400", body, rr.Code)
		}
	}
	// An empty object is a no-op, not a wipe.
	if rr := putSettingsJSON(t, srv, `{}`); rr.Code != http.StatusOK {
		t.Errorf("PUT {} = %d, want 200", rr.Code)
	}
	got := srv.getSettings()
	if got.APIKey != "nd-keep-me" || got.ScanInterval != "6h" {
		t.Errorf("settings changed by rejected or empty bodies: key=%q interval=%q", got.APIKey, got.ScanInterval)
	}
}

func TestUpdateSettings_VersionIsServerOwned(t *testing.T) {
	srv := newSettingsTestServer()
	if rr := putSettingsJSON(t, srv, `{"settings_version": 0}`); rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}
	if v := srv.getSettings().SettingsVersion; v != currentSettingsVersion {
		t.Errorf("settings_version = %d, want %d", v, currentSettingsVersion)
	}
}

func TestUpdateSettings_ScanIntervalFloor(t *testing.T) {
	srv := newSettingsTestServer()
	for _, v := range []string{"1s", "29s"} {
		if rr := putSettingsJSON(t, srv, fmt.Sprintf(`{"scan_interval":%q}`, v)); rr.Code != http.StatusBadRequest {
			t.Errorf("scan_interval %s = %d, want 400", v, rr.Code)
		}
	}
	if rr := putSettingsJSON(t, srv, `{"scan_interval":"30s"}`); rr.Code != http.StatusOK {
		t.Errorf("scan_interval 30s = %d, want 200: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateSettings_RetentionClampsToFloor(t *testing.T) {
	srv := newSettingsTestServer()
	if rr := putSettingsJSON(t, srv, `{"retention":{"snapshot_days":5,"max_db_size_mb":40,"notify_log_days":0}}`); rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}
	got := srv.getSettings().Retention
	want := RetentionSettings{SnapshotDays: 7, MaxDBSizeMB: 50, NotifyLogDays: 30}
	if got != want {
		t.Errorf("retention = %+v, want %+v (below-floor values go to the floor, unset to the default)", got, want)
	}
}

// TestSettings_SpeedTestSectionCanBeHidden: the page sends, and the
// dashboard reads, sections.speedtest. The server used to store it under a
// different name and drop the toggle.
func TestSettings_SpeedTestSectionCanBeHidden(t *testing.T) {
	srv := newSettingsTestServer()
	if !srv.getSettings().Sections.SpeedTest {
		t.Fatal("speed test section should default to visible")
	}
	if rr := putSettingsJSON(t, srv, `{"sections":{"speedtest":false}}`); rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}
	sec := getSettingsJSON(t, srv)["sections"].(map[string]any)
	if v, ok := sec["speedtest"].(bool); !ok || v {
		t.Errorf("sections.speedtest = %v, want false", sec["speedtest"])
	}

	// A blob written by an older version carries the meaningless
	// "speed_test": false. It must not hide the section.
	old := newSettingsTestServer()
	_ = old.store.SetConfig(settingsConfigKey, `{"settings_version":3,"sections":{"findings":true,"speed_test":false}}`)
	if !old.getSettings().Sections.SpeedTest {
		t.Error("legacy speed_test:false hid the Speed Test section after upgrade")
	}
}

// TestGetSettings_SectionsAreTakenAtTheirWord: hiding the six core
// sections used to make GET /settings return (and the page then save) the
// defaults for every section.
func TestGetSettings_SectionsAreTakenAtTheirWord(t *testing.T) {
	srv := newSettingsTestServer()
	body := `{"sections":{"findings":false,"disk_space":false,"smart":false,"docker":false,"zfs":false,"ups":false,"gpu":true,"dash_columns":4}}`
	if rr := putSettingsJSON(t, srv, body); rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}
	sec := getSettingsJSON(t, srv)["sections"].(map[string]any)
	for _, k := range []string{"findings", "disk_space", "smart", "docker", "zfs", "ups"} {
		if sec[k] != false {
			t.Errorf("sections.%s = %v, want false", k, sec[k])
		}
	}
	if sec["gpu"] != true || sec["dash_columns"].(float64) != 4 {
		t.Errorf("other section settings were reset: gpu=%v dash_columns=%v", sec["gpu"], sec["dash_columns"])
	}
}

// TestMigrateSettings_DefaultCheckDoesNotLeak: decoding a stored list on
// top of the seeded "Internet Speed" check used to leave the seed's
// interval and margin in the user's first check.
func TestMigrateSettings_DefaultCheckDoesNotLeak(t *testing.T) {
	raw := `{"settings_version":3,"service_checks":{"checks":[{"name":"My line","type":"speed","target":"speedtest","enabled":true,"contracted_down_mbps":500}]}}`
	got := migrateSettings([]byte(raw), defaultSettings())
	if len(got.ServiceChecks.Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(got.ServiceChecks.Checks))
	}
	c := got.ServiceChecks.Checks[0]
	if c.MarginPct != 0 {
		t.Errorf("margin_pct = %v, want 0 (leaked from the default check)", c.MarginPct)
	}
	if c.IntervalSec != 0 {
		t.Errorf("interval_sec = %v, want 0 (leaked from the default check)", c.IntervalSec)
	}

	// With nothing stored for service checks, the seed still applies.
	fresh := migrateSettings([]byte(`{"settings_version":3}`), defaultSettings())
	if len(fresh.ServiceChecks.Checks) != 1 || fresh.ServiceChecks.Checks[0].Name != "Internet Speed" {
		t.Errorf("default check missing when the blob has no service_checks: %+v", fresh.ServiceChecks.Checks)
	}
	// An explicit empty list means the user removed it.
	empty := migrateSettings([]byte(`{"settings_version":3,"service_checks":{"checks":[]}}`), defaultSettings())
	if len(empty.ServiceChecks.Checks) != 0 {
		t.Errorf("explicit empty check list was replaced by the default: %+v", empty.ServiceChecks.Checks)
	}
}

// countingStore counts settings writes.
type countingStore struct {
	storage.Store
	sets atomic.Int64
}

func (c *countingStore) SetConfig(key, value string) error {
	c.sets.Add(1)
	return c.Store.SetConfig(key, value)
}

// TestGetSettings_DoesNotWrite: reads used to rewrite the blob every time.
func TestGetSettings_DoesNotWrite(t *testing.T) {
	cs := &countingStore{Store: storage.NewFakeStore()}
	srv := newSettingsTestServer()
	srv.store = cs
	if rr := putSettingsJSON(t, srv, `{"scan_interval":"6h"}`); rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}
	base := cs.sets.Load()

	for i := 0; i < 50; i++ {
		_ = srv.getSettings()
	}
	rr := httptest.NewRecorder()
	srv.handleGetSettings(rr, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	rr = httptest.NewRecorder()
	srv.handleStatus(rr, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))

	if n := cs.sets.Load() - base; n != 0 {
		t.Errorf("reads performed %d settings writes, want 0", n)
	}
}

// TestGetSettings_MigratesOnce: a blob from an older version is rewritten
// exactly once, then reads stop writing.
func TestGetSettings_MigratesOnce(t *testing.T) {
	cs := &countingStore{Store: storage.NewFakeStore()}
	_ = cs.Store.SetConfig(settingsConfigKey, `{"settings_version":1,"scan_interval":"2h","wake_drives_for_smart":true}`)
	srv := newSettingsTestServer()
	srv.store = cs

	first := srv.getSettings()
	if first.SettingsVersion != currentSettingsVersion || !first.AdvancedScans.SMART.WakeDrives || first.ScanInterval != "2h" {
		t.Fatalf("migration result wrong: %+v", first)
	}
	for i := 0; i < 10; i++ {
		_ = srv.getSettings()
	}
	if n := cs.sets.Load(); n != 1 {
		t.Errorf("migration wrote %d times, want exactly 1", n)
	}
}

// TestSettings_ConcurrentReadersDoNotUndoSaves is the lost-update test.
// Readers hammer the read paths while writers save distinct values; every
// acknowledged save must be what a subsequent read sees.
func TestSettings_ConcurrentReadersDoNotUndoSaves(t *testing.T) {
	srv := newSettingsTestServer()
	if rr := putSettingsJSON(t, srv, `{"scan_interval":"6h"}`); rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				rr := httptest.NewRecorder()
				srv.handleStatus(rr, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
				_ = srv.getSettings()
			}
		}()
	}

	lost := 0
	for i := 0; i < 300; i++ {
		cost := float64(i + 1)
		if rr := putSettingsJSON(t, srv, fmt.Sprintf(`{"cost_per_tb":%v}`, cost)); rr.Code != http.StatusOK {
			t.Fatalf("PUT %d = %d", i, rr.Code)
		}
		if got := srv.getSettings().CostPerTB; got != cost {
			lost++
		}
	}
	close(stop)
	readers.Wait()
	if lost > 0 {
		t.Errorf("%d of 300 acknowledged saves were undone by concurrent readers", lost)
	}
}

// TestSettings_ConcurrentWritersKeepEachOthersChanges: single-field
// handlers and the full save must not overwrite one another with stale
// copies.
func TestSettings_ConcurrentWritersKeepEachOthersChanges(t *testing.T) {
	srv := newSettingsTestServer()
	const n = 40
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"title":"finding-%d"}`, i)
			rr := httptest.NewRecorder()
			srv.handleDismissFinding(rr, httptest.NewRequest(http.MethodPost, "/api/v1/findings/dismiss", strings.NewReader(body)))
		}(i)
		go func(i int) {
			defer wg.Done()
			putSettingsJSON(t, srv, fmt.Sprintf(`{"cost_per_tb":%d}`, i+1))
		}(i)
	}
	wg.Wait()
	if got := len(srv.getSettings().DismissedFindings); got != n {
		t.Errorf("dismissed findings = %d, want %d (some were overwritten by a concurrent save)", got, n)
	}
}

// TestUpdateSettings_RefusesToWriteWhenReadFails: a failed read must not
// lead to defaults being written over the real settings.
func TestUpdateSettings_RefusesToWriteWhenReadFails(t *testing.T) {
	srv := newSettingsTestServer()
	putSettingsJSON(t, srv, `{"api_key":"nd-real","scan_interval":"6h"}`)
	good := srv.store
	cs := &countingStore{Store: failingConfigStore{good}}
	srv.store = cs

	if rr := putSettingsJSON(t, srv, `{"theme":"clean"}`); rr.Code != http.StatusServiceUnavailable {
		t.Errorf("PUT with unreadable settings = %d, want 503", rr.Code)
	}
	rr := httptest.NewRecorder()
	srv.handleGetSettings(rr, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("GET with unreadable settings = %d, want 503 (not defaults)", rr.Code)
	}
	rr = httptest.NewRecorder()
	srv.handleDismissFinding(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"title":"t"}`)))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("dismiss with unreadable settings = %d, want 500", rr.Code)
	}
	if n := cs.sets.Load(); n != 0 {
		t.Errorf("%d writes happened while settings were unreadable", n)
	}

	srv.store = good
	if got := srv.getSettings(); got.APIKey != "nd-real" || got.ScanInterval != "6h" {
		t.Errorf("stored settings were damaged: %+v", got)
	}
}

// TestUpdateSettings_KeyChangeKeepsTheBrowserSignedIn: the browser that
// sets or rotates the key receives a session for the new key; revoking the
// key clears it.
func TestUpdateSettings_KeyChangeKeepsTheBrowserSignedIn(t *testing.T) {
	srv := newSettingsTestServer()

	rr := putSettingsJSON(t, srv, `{"api_key":"nd-new-key"}`)
	c := sessionCookieFrom(rr)
	if c == nil || !validSessionToken("nd-new-key", c.Value, time.Now()) {
		t.Fatalf("setting a key did not issue a valid session cookie: %v", c)
	}

	// An unrelated save does not touch the cookie.
	if c := sessionCookieFrom(putSettingsJSON(t, srv, `{"theme":"clean"}`)); c != nil {
		t.Errorf("unrelated save set a session cookie: %v", c)
	}

	rr = putSettingsJSON(t, srv, `{"api_key":""}`)
	c = sessionCookieFrom(rr)
	if c == nil || c.MaxAge >= 0 {
		t.Errorf("revoking the key did not clear the session cookie: %v", c)
	}
}

func TestMergeWebhooks(t *testing.T) {
	saved := []internal.WebhookConfig{{Name: "discord", URL: "http://a"}}
	static := []internal.WebhookConfig{{Name: "env-webhook", URL: "http://b"}, {Name: "Discord", URL: "http://c"}}
	got := MergeWebhooks(saved, static)
	if len(got) != 2 || got[0].Name != "discord" || got[1].Name != "env-webhook" {
		t.Errorf("MergeWebhooks = %+v; want saved first, then static entries whose name is free", got)
	}
	if got := MergeWebhooks(nil, static[:1]); len(got) != 1 {
		t.Errorf("static webhook dropped when nothing is saved: %+v", got)
	}
	if got := MergeWebhooks(saved, nil); len(got) != 1 {
		t.Errorf("saved webhooks changed with no static ones: %+v", got)
	}
}

func TestFormatScanInterval(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Minute:              "30m",
		6 * time.Hour:                 "6h",
		90 * time.Minute:              "1h30m",
		45 * time.Second:              "45s",
		24 * time.Hour:                "24h",
		time.Hour + 30*time.Second:    "1h0m30s",
		2*time.Minute + 5*time.Second: "2m5s",
	}
	for d, want := range cases {
		got := FormatScanInterval(d)
		if got != want {
			t.Errorf("FormatScanInterval(%v) = %q, want %q", d, got, want)
		}
		if back, err := time.ParseDuration(got); err != nil || back != d {
			t.Errorf("FormatScanInterval(%v) = %q does not parse back", d, got)
		}
	}
}

// TestLoadSettings_BootIntervalIsTheDefault: NAS_DOCTOR_INTERVAL is what a
// fresh install shows and runs, until the user saves something else.
func TestLoadSettings_BootIntervalIsTheDefault(t *testing.T) {
	store := storage.NewFakeStore()

	s, found, err := LoadSettings(store, 6*time.Hour)
	if err != nil || found {
		t.Fatalf("fresh store: found=%v err=%v", found, err)
	}
	if s.ScanInterval != "6h" {
		t.Errorf("fresh install scan_interval = %q, want 6h", s.ScanInterval)
	}
	if len(s.ServiceChecks.Checks) != 1 || len(s.SpeedTestSchedule) != 1 {
		t.Errorf("fresh install is missing the default check or schedule: %+v / %v", s.ServiceChecks.Checks, s.SpeedTestSchedule)
	}

	srv := newSettingsTestServer()
	srv.store = store
	srv.SetBootScanInterval(6 * time.Hour)
	if got := getSettingsJSON(t, srv)["scan_interval"]; got != "6h" {
		t.Errorf("GET settings on a fresh install shows %v, want 6h", got)
	}
	// Saving something unrelated keeps it.
	putSettingsJSON(t, srv, `{"theme":"clean"}`)
	if got := srv.getSettings().ScanInterval; got != "6h" {
		t.Errorf("after an unrelated save scan_interval = %q, want 6h", got)
	}
	// An explicit choice wins from then on.
	putSettingsJSON(t, srv, `{"scan_interval":"1h"}`)
	s, found, _ = LoadSettings(store, 6*time.Hour)
	if !found || s.ScanInterval != "1h" {
		t.Errorf("stored choice not honoured: found=%v interval=%q", found, s.ScanInterval)
	}
}

// TestFleetStatus_DoesNotExposePeerCredentials covers GET /api/v1/fleet:
// the status feed the fleet page renders must not carry the API keys and
// auth headers used to poll each peer, while the editable server list
// still does.
func TestFleetStatus_DoesNotExposePeerCredentials(t *testing.T) {
	peer := internal.RemoteServer{
		ID: "a", Name: "Backup NAS", URL: "http://10.0.0.2:8060", Enabled: true,
		APIKey:  "nd-PEER-KEY",
		Headers: map[string]string{"CF-Access-Client-Secret": "cfsecret"},
	}
	mgr := fleet.New(slog.Default())
	mgr.SetServers([]internal.RemoteServer{peer})
	mgr.InjectStatuses([]internal.RemoteServerStatus{{Server: peer, Online: true, Hostname: "backup-nas"}})

	srv := newSettingsTestServer()
	srv.fleet = mgr

	rr := httptest.NewRecorder()
	srv.handleFleetStatus(rr, httptest.NewRequest(http.MethodGet, "/api/v1/fleet", nil))
	body := rr.Body.String()
	if strings.Contains(body, "nd-PEER-KEY") || strings.Contains(body, "cfsecret") {
		t.Fatalf("GET /api/v1/fleet exposes peer credentials: %s", body)
	}
	if !strings.Contains(body, "Backup NAS") || !strings.Contains(body, "backup-nas") {
		t.Errorf("status feed lost the fields the page needs: %s", body)
	}

	// The manager's own record is untouched (the poller still needs it).
	if got := mgr.GetServers(); len(got) != 1 || got[0].APIKey != "nd-PEER-KEY" {
		t.Errorf("sanitising the response altered the manager's server list: %+v", got)
	}
	if st := mgr.GetStatuses(); st[0].Server.APIKey != "nd-PEER-KEY" {
		t.Error("sanitising the response altered the manager's stored status")
	}
}
