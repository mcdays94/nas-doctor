package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
)

func postBorgTest(t *testing.T, srv *Server, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	rr := httptest.NewRecorder()
	srv.handleTestBorgMonitor(rr, httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/borg/test", strings.NewReader(string(data))))
	return rr
}

// TestBorgTestEndpoint_RefusesArbitraryBinariesAndOptionInjection is the
// regression test for the Test button running whatever binary_path named
// and pasting ssh_key_path into BORG_RSH unchecked.
func TestBorgTestEndpoint_RefusesArbitraryBinariesAndOptionInjection(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "payload.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+filepath.Join(dir, "pwned")+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	notExec := filepath.Join(dir, "borg-notexec")
	if err := os.WriteFile(notExec, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		body map[string]any
	}{
		{"executable that is not borg", map[string]any{"repo_path": "/r", "binary_path": script}},
		{"shell", map[string]any{"repo_path": "/r", "binary_path": "/bin/sh"}},
		{"relative binary", map[string]any{"repo_path": "/r", "binary_path": "borg-custom"}},
		{"borg-named but not executable", map[string]any{"repo_path": "/r", "binary_path": notExec}},
		{"missing binary", map[string]any{"repo_path": "/r", "binary_path": filepath.Join(dir, "borg-missing")}},
		{"ssh option injection", map[string]any{"repo_path": "ssh://u@h/./r", "ssh_key_path": "/dev/null -o ProxyCommand=touch /tmp/x"}},
		{"ssh key is a directory", map[string]any{"repo_path": "ssh://u@h/./r", "ssh_key_path": dir}},
		{"relative ssh key", map[string]any{"repo_path": "ssh://u@h/./r", "ssh_key_path": "id_ed25519"}},
		{"repo path that is an option", map[string]any{"repo_path": "--rsh=touch /tmp/x"}},
	}
	for _, tc := range cases {
		srv := newTestServer(t)
		// A runner that would succeed: the request must be refused before it.
		srv.borgRunner = collector.NewExecBorgRunner()
		rr := postBorgTest(t, srv, tc.body)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (%s)", tc.name, rr.Code, rr.Body.String())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
		t.Fatal("the endpoint executed a caller-supplied script")
	}
}

func TestValidateBorgRepo_AcceptsLegitimateEntries(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "borg-1.4")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(dir, "my keys", "id_ed25519")
	if err := os.MkdirAll(filepath.Dir(key), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	ok := []BorgExternalRepo{
		{Enabled: true, RepoPath: "/mnt/user/appdata/borg/main"},
		{Enabled: true, RepoPath: "ssh://borg@host:2222/./repo", SSHKeyPath: key, BinaryPath: bin, PassphraseEnv: "BORG_MAIN"},
		{Enabled: false},
	}
	for i, r := range ok {
		if err := validateBorgRepo(r); err != nil {
			t.Errorf("entry %d rejected: %v", i, err)
		}
	}
}

// TestNormalizeServiceCheck_RejectsOptionLikeTargets: ping, mtr and the
// resolver receive these on their command line.
func TestNormalizeServiceCheck_RejectsOptionLikeTargets(t *testing.T) {
	for _, c := range []internal.ServiceCheckConfig{
		{Name: "p", Type: "ping", Target: "-F/etc/passwd"},
		{Name: "t", Type: "traceroute", Target: "--filename=/etc/passwd"},
		{Name: "d", Type: "dns", Target: "example.com", DNSServer: "-x"},
	} {
		c := c
		if err := normalizeServiceCheckConfig(&c); err == nil {
			t.Errorf("%s check with target %q / dns_server %q was accepted", c.Type, c.Target, c.DNSServer)
		}
	}
	good := internal.ServiceCheckConfig{Name: "p", Type: "ping", Target: "192.168.1.1"}
	if err := normalizeServiceCheckConfig(&good); err != nil {
		t.Errorf("ordinary ping target rejected: %v", err)
	}
}

// TestTestWebhook_ReportsFailure: the endpoint used to answer "sent"
// whatever happened.
func TestTestWebhook_ReportsFailure(t *testing.T) {
	secretPath := "/services/T000/B000/SECRETTOKEN"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(strings.Repeat("internal detail ", 400)))
	}))
	defer upstream.Close()

	srv := newTestServer(t)
	body := `{"name":"hook","type":"generic","url":"` + upstream.URL + secretPath + `"}`
	rr := httptest.NewRecorder()
	srv.handleTestWebhook(rr, httptest.NewRequest(http.MethodPost, "/api/v1/settings/test-webhook", strings.NewReader(body)))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("failed delivery reported as %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "failed" || !strings.Contains(resp["error"], "403") {
		t.Errorf("response = %v, want status failed mentioning 403", resp)
	}
	if len(resp["error"]) > 400 {
		t.Errorf("error carries %d bytes of the upstream body, want it truncated", len(resp["error"]))
	}

	// What reaches the notification log must not carry the URL path.
	logs, err := srv.store.GetNotificationLog(10)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range logs {
		if strings.Contains(l.ErrorMessage, "SECRETTOKEN") {
			t.Errorf("notification log stores the webhook URL: %q", l.ErrorMessage)
		}
		if len(l.ErrorMessage) > 400 {
			t.Errorf("notification log stores %d bytes of upstream body", len(l.ErrorMessage))
		}
	}

	// A webhook that accepts the request reports success.
	okUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer okUp.Close()
	rr = httptest.NewRecorder()
	srv.handleTestWebhook(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"hook","type":"generic","url":"`+okUp.URL+`"}`)))
	if rr.Code != http.StatusOK {
		t.Errorf("successful delivery = %d: %s", rr.Code, rr.Body.String())
	}
}

// TestReport_EscapesZFSAndUPSStrings: these values come from zpool and
// from upsc/apcaccess, which can be a remote daemon.
func TestReport_EscapesZFSAndUPSStrings(t *testing.T) {
	const payload = `<script>alert(1)</script>`
	snap := &internal.Snapshot{
		System: internal.SystemInfo{Hostname: "h"},
		ZFS: &internal.ZFSInfo{Available: true, Pools: []internal.ZPool{{
			Name: "tank", State: payload, Status: payload, ScanType: payload, ScanDate: payload,
		}}},
		UPS: &internal.UPSInfo{Available: true, Name: "ups", StatusHuman: payload, LastTransfer: payload},
	}
	html := GenerateReport(snap, ReportSparklines{})
	if strings.Contains(html, payload) {
		t.Error("report contains an unescaped ZFS/UPS string")
	}
	if !strings.Contains(html, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Error("expected the escaped form of the test string in the report (did the fields stop rendering?)")
	}
}
