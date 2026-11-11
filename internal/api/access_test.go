package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcdays94/nas-doctor/internal/storage"
)

// upgradedServer is an install whose settings were saved by a version that
// had no sign-in: the blob carries no "access" object.
func upgradedServer(t *testing.T, blob string) *Server {
	t.Helper()
	store := storage.NewFakeStore()
	if blob != "" {
		if err := store.SetConfig(settingsConfigKey, blob); err != nil {
			t.Fatal(err)
		}
	}
	return &Server{store: store, logger: slog.Default(), version: "test", startTime: time.Now()}
}

const oldBlobWithKey = `{"settings_version":3,"scan_interval":"30m","theme":"midnight","api_key":"nd-old-key"}`

func do(router http.Handler, method, path string, mut func(*http.Request)) *httptest.ResponseRecorder {
	var body *bytes.Reader
	if method == http.MethodGet || method == http.MethodHead {
		body = bytes.NewReader(nil)
	} else {
		body = bytes.NewReader([]byte(`{}`))
	}
	req := httptest.NewRequest(method, path, body)
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set("Content-Type", "application/json")
	}
	if mut != nil {
		mut(req)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func refused(rr *httptest.ResponseRecorder) bool {
	return rr.Code == http.StatusUnauthorized ||
		(rr.Code == http.StatusFound && strings.HasPrefix(rr.Header().Get("Location"), "/login"))
}

// TestUpgrade_InstallWithKeyBehavesAsBefore is the promise to existing
// users: an install that already had an API key is not put behind a
// sign-in page by upgrading. Every route behaves as it did, including the
// handful the key guarded and the way the web UI got past them.
func TestUpgrade_InstallWithKeyBehavesAsBefore(t *testing.T) {
	srv := upgradedServer(t, oldBlobWithKey)
	router := srv.Router()

	if mode := srv.getSettings().accessMode(); mode != accessKeyForAPI {
		t.Fatalf("mode after upgrade = %v, want the key-for-API rules", mode)
	}

	// Pages and configuration open without the key, as they did.
	for _, path := range []string{"/", "/settings", "/alerts", "/fleet", "/stats", "/api/v1/settings", "/api/v1/disks", "/api/v1/alerts"} {
		if rr := do(router, http.MethodGet, path, nil); refused(rr) {
			t.Errorf("GET %s was refused (%d -> %q) on an upgraded install", path, rr.Code, rr.Header().Get("Location"))
		}
	}
	// Nobody is sent to a sign-in page, and asking for it leads home.
	if rr := do(router, http.MethodGet, "/login", nil); rr.Code != http.StatusFound || rr.Header().Get("Location") != "/" {
		t.Errorf("GET /login = %d -> %q, want a redirect to /", rr.Code, rr.Header().Get("Location"))
	}

	// The routes the key guarded still want it...
	rr := do(router, http.MethodGet, "/api/v1/status", nil)
	if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "invalid or missing API key") {
		t.Errorf("status without the key = %d %s, want the 401 it always gave", rr.Code, rr.Body.String())
	}
	// ...and accept it the ways they always did.
	for name, mut := range map[string]func(*http.Request){
		"bearer":      func(r *http.Request) { r.Header.Set("Authorization", "Bearer nd-old-key") },
		"query param": func(r *http.Request) { r.URL.RawQuery = "api_key=nd-old-key" },
		"same page":   func(r *http.Request) { r.Host = "nas.lan:8060"; r.Header.Set("Referer", "http://nas.lan:8060/") },
		"localhost":   func(r *http.Request) { r.Header.Set("Referer", "http://localhost:8060/settings") },
	} {
		if rr := do(router, http.MethodGet, "/api/v1/status", mut); rr.Code != http.StatusOK {
			t.Errorf("status with %s = %d, want 200", name, rr.Code)
		}
	}
	if rr := do(router, http.MethodGet, "/api/v1/status", func(r *http.Request) {
		r.Host = "nas.lan:8060"
		r.Header.Set("Referer", "https://elsewhere.example/")
	}); rr.Code != http.StatusUnauthorized {
		t.Errorf("status from another site's page = %d, want 401", rr.Code)
	}
	if rr := do(router, http.MethodGet, "/api/v1/status", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer wrong")
	}); rr.Code != http.StatusUnauthorized {
		t.Errorf("status with a wrong key = %d, want 401", rr.Code)
	}
}

// TestUpgrade_KeyGuardsExactlyTheOldRoutes walks the real router on an
// upgraded install: the key is asked for on the routes it guarded before
// sign-in existed, and on no others.
func TestUpgrade_KeyGuardsExactlyTheOldRoutes(t *testing.T) {
	srv := upgradedServer(t, oldBlobWithKey)
	router := srv.Router()

	// The set as registered behind the key in the last release without
	// sign-in.
	guarded := map[string]bool{
		"GET /api/v1/status":                         true,
		"GET /api/v1/snapshot/latest":                true,
		"GET /api/v1/snapshot/{id}":                  true,
		"GET /api/v1/snapshots":                      true,
		"POST /api/v1/scan":                          true,
		"GET /api/v1/report":                         true,
		"POST /api/v1/speedtest/run":                 true,
		"GET /api/v1/speedtest/stream/{test_id}":     true,
		"POST /api/v1/speedtest/cancel/{test_id}":    true,
		"GET /api/v1/speedtest/samples/{test_id}":    true,
		"POST /api/v1/backup-monitor/duplicacy/test": true,
	}
	found := 0
	err := chi.Walk(router.(chi.Routes), func(method, pattern string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		rr := do(router, method, concretePath(pattern), nil)
		want := guarded[method+" "+pattern]
		if want {
			found++
		}
		if got := refused(rr); got != want {
			t.Errorf("%s %s without the key: refused=%v (status %d), want refused=%v", method, pattern, got, rr.Code, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found != len(guarded) {
		t.Errorf("found %d of the %d historically guarded routes; one was renamed or removed", found, len(guarded))
	}
}

// TestUpgrade_SavingSettingsDoesNotTurnSignInOn: the old rules are not a
// one-request courtesy. They survive saves, and a save hands out no
// session.
func TestUpgrade_SavingSettingsDoesNotTurnSignInOn(t *testing.T) {
	srv := upgradedServer(t, oldBlobWithKey)

	rr := putSettingsJSON(t, srv, `{"theme":"clean"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("save = %d: %s", rr.Code, rr.Body.String())
	}
	if c := sessionCookieFrom(rr); c != nil {
		t.Errorf("an unrelated save on an upgraded install set a session cookie")
	}
	raw, _ := srv.store.GetConfig(settingsConfigKey)
	if !strings.Contains(raw, `"require_sign_in":false`) {
		t.Errorf("the choice was not stored explicitly: %s", raw)
	}
	// Replacing the key (as the Generate button does) keeps the rules too.
	if rr := putSettingsJSON(t, srv, `{"api_key":"nd-replaced"}`); rr.Code != http.StatusOK {
		t.Fatalf("replace key = %d", rr.Code)
	}
	if mode := srv.getSettings().accessMode(); mode != accessKeyForAPI {
		t.Errorf("mode after replacing the key = %v, want the key-for-API rules", mode)
	}
	if rr := do(srv.Router(), http.MethodGet, "/settings", nil); refused(rr) {
		t.Errorf("Settings was put behind sign-in by a save")
	}
}

// TestUpgrade_InstallWithoutKeyIsStillOpen: nothing changes for the common
// case either, and a key set later is a decision made in the new Settings
// page, where it means what it says.
func TestUpgrade_InstallWithoutKeyIsStillOpen(t *testing.T) {
	for name, blob := range map[string]string{
		"saved settings, no key": `{"settings_version":3,"scan_interval":"30m","theme":"midnight"}`,
		"never saved":            "",
	} {
		srv := upgradedServer(t, blob)
		router := srv.Router()
		for _, path := range []string{"/", "/settings", "/api/v1/settings", "/api/v1/status"} {
			if rr := do(router, http.MethodGet, path, nil); refused(rr) {
				t.Errorf("%s: GET %s refused on an install with no key", name, path)
			}
		}
		if !srv.getSettings().Access.RequireSignIn {
			t.Errorf("%s: a key set from now on would not be protected by sign-in", name)
		}
	}
}

// TestSignIn_IsAnExplicitSwitch walks an upgraded install through turning
// sign-in on and off again.
func TestSignIn_IsAnExplicitSwitch(t *testing.T) {
	srv := upgradedServer(t, oldBlobWithKey)
	router := srv.Router()

	rr := putSettingsJSON(t, srv, `{"access":{"require_sign_in":true}}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("turn on = %d: %s", rr.Code, rr.Body.String())
	}
	cookie := sessionCookieFrom(rr)
	if cookie == nil {
		t.Fatal("the browser that turned sign-in on was not signed in")
	}
	if got := srv.getSettings(); got.accessMode() != accessSignIn || got.Access.SessionDays != DefaultSessionDays {
		t.Fatalf("after turning on: mode %v, session days %d", got.accessMode(), got.Access.SessionDays)
	}

	if rr := do(router, http.MethodGet, "/settings", nil); !refused(rr) {
		t.Errorf("Settings without a session = %d, want the sign-in page", rr.Code)
	}
	if rr := do(router, http.MethodGet, "/api/v1/settings", nil); rr.Code != http.StatusUnauthorized {
		t.Errorf("settings API without the key = %d, want 401", rr.Code)
	}
	// The same-page exemption is gone with the old rules.
	if rr := do(router, http.MethodGet, "/api/v1/status", func(r *http.Request) {
		r.Header.Set("Referer", "http://localhost:8060/")
	}); rr.Code != http.StatusUnauthorized {
		t.Errorf("status with only a Referer = %d, want 401 once sign-in is on", rr.Code)
	}
	withCookie := func(r *http.Request) { r.AddCookie(cookie) }
	if rr := do(router, http.MethodGet, "/settings", withCookie); rr.Code != http.StatusOK {
		t.Errorf("Settings with the session = %d, want 200", rr.Code)
	}
	// Fleet peers and scripts that hold the key carry on.
	if rr := do(router, http.MethodGet, "/api/v1/status", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer nd-old-key")
	}); rr.Code != http.StatusOK {
		t.Errorf("status with the key = %d, want 200", rr.Code)
	}

	// Off again: back to the old rules, and the session is dropped.
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader(`{"access":{"require_sign_in":false}}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	off := httptest.NewRecorder()
	router.ServeHTTP(off, req)
	if off.Code != http.StatusOK {
		t.Fatalf("turn off = %d: %s", off.Code, off.Body.String())
	}
	if c := sessionCookieFrom(off); c == nil || c.MaxAge >= 0 {
		t.Errorf("turning sign-in off did not clear the session cookie: %+v", c)
	}
	if rr := do(router, http.MethodGet, "/settings", nil); refused(rr) {
		t.Errorf("Settings still behind sign-in after turning it off")
	}
}

// TestSession_SlidesAndFollowsTheSetting drives sessions through the
// router: the cookie is reissued as it is used, and the configured length
// applies to sessions that already exist.
func TestSession_SlidesAndFollowsTheSetting(t *testing.T) {
	srv := newAuthTestServer(t, testAPIKey)
	router := srv.Router()
	withToken := func(issued time.Time) func(*http.Request) {
		return func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: newSessionToken(testAPIKey, issued)})
		}
	}
	day := 24 * time.Hour

	// A session used a moment ago is left alone; an older one is renewed.
	if rr := do(router, http.MethodGet, "/api/v1/settings", withToken(time.Now().Add(-10*time.Minute))); rr.Code != http.StatusOK || sessionCookieFrom(rr) != nil {
		t.Errorf("recent session: status %d, reissued=%v; want 200 and no new cookie", rr.Code, sessionCookieFrom(rr) != nil)
	}
	rr := do(router, http.MethodGet, "/api/v1/settings", withToken(time.Now().Add(-3*time.Hour)))
	c := sessionCookieFrom(rr)
	if rr.Code != http.StatusOK || c == nil {
		t.Fatalf("3-hour-old session: status %d, reissued=%v; want 200 and a fresh cookie", rr.Code, c != nil)
	}
	if issued, ok := sessionIssuedAt(testAPIKey, c.Value); !ok || time.Since(issued) > time.Minute {
		t.Errorf("reissued cookie is not fresh: %v %v", issued, ok)
	}
	if want := int((DefaultSessionDays * day) / time.Second); c.MaxAge != want {
		t.Errorf("cookie Max-Age = %d, want %d (the default session length)", c.MaxAge, want)
	}

	// 200 days idle is fine by default...
	if rr := do(router, http.MethodGet, "/api/v1/settings", withToken(time.Now().Add(-200*day))); rr.Code != http.StatusOK {
		t.Errorf("session idle 200 days with the default length = %d, want 200", rr.Code)
	}
	// ...shortening the setting ends it without anyone signing out...
	if _, err := srv.updateSettings(func(s *Settings) error { s.Access.SessionDays = 30; return nil }); err != nil {
		t.Fatal(err)
	}
	if rr := do(router, http.MethodGet, "/api/v1/settings", withToken(time.Now().Add(-200*day))); rr.Code != http.StatusUnauthorized {
		t.Errorf("session idle 200 days with a 30-day length = %d, want 401", rr.Code)
	}
	// ...and "never" keeps even a very old one, with the longest cookie a
	// browser will hold.
	if _, err := srv.updateSettings(func(s *Settings) error { s.Access.SessionDays = 0; return nil }); err != nil {
		t.Fatal(err)
	}
	rr = do(router, http.MethodGet, "/api/v1/settings", withToken(time.Now().Add(-900*day)))
	if rr.Code != http.StatusOK {
		t.Fatalf("session idle 900 days with no expiry = %d, want 200", rr.Code)
	}
	if c := sessionCookieFrom(rr); c == nil || c.MaxAge != int(sessionCookieMaxAge/time.Second) {
		t.Errorf("never-expiring session cookie: %+v", c)
	}
}

func TestUpdateSettings_SessionDaysBounds(t *testing.T) {
	srv := newSettingsTestServer()
	for _, bad := range []string{`{"access":{"session_days":-1}}`, `{"access":{"session_days":99999}}`} {
		if rr := putSettingsJSON(t, srv, bad); rr.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", bad, rr.Code)
		}
	}
	for _, good := range []string{`{"access":{"session_days":0}}`, `{"access":{"session_days":1}}`, `{"access":{"session_days":3650}}`} {
		if rr := putSettingsJSON(t, srv, good); rr.Code != http.StatusOK {
			t.Errorf("%s = %d: %s", good, rr.Code, rr.Body.String())
		}
	}
	// A partial access patch leaves the other access settings alone.
	if _, err := srv.updateSettings(func(s *Settings) error { s.Access.TrustLocalNetwork = true; return nil }); err != nil {
		t.Fatal(err)
	}
	putSettingsJSON(t, srv, `{"access":{"session_days":7}}`)
	if got := srv.getSettings().Access; !got.TrustLocalNetwork || !got.RequireSignIn || got.SessionDays != 7 {
		t.Errorf("partial access patch changed other fields: %+v", got)
	}
}

func withGateways(t *testing.T, addrs ...string) {
	t.Helper()
	prev := defaultGateways
	var gws []netip.Addr
	for _, a := range addrs {
		gws = append(gws, netip.MustParseAddr(a))
	}
	defaultGateways = func() []netip.Addr { return gws }
	t.Cleanup(func() { defaultGateways = prev })
}

func TestDescribeClient(t *testing.T) {
	withGateways(t, "172.17.0.1")
	cases := []struct {
		name   string
		peer   string
		proto  string
		host   string
		header map[string]string
		local  bool
		reason string
	}{
		{name: "opened by IP address", peer: "192.168.1.23:1", host: "192.168.1.10:8060", local: true, reason: "private"},
		{name: "opened by IPv6 address", peer: "192.168.1.23:1", host: "[fd12:3456::1]:8060", local: true, reason: "private"},
		{name: "opened by short name", peer: "192.168.1.23:1", host: "tower:8060", local: true, reason: "private"},
		{name: "opened by mDNS name", peer: "192.168.1.23:1", host: "Tower.local", local: true, reason: "private"},
		{name: "opened as localhost", peer: "127.0.0.1:1", host: "localhost:8060", local: true, reason: "loopback"},
		{name: "own domain, listed", peer: "192.168.1.23:1", host: "nas.example.com", local: true, reason: "private"},
		{name: "own domain, listed, other case and port", peer: "192.168.1.23:1", host: "NAS.Example.com:443", local: true, reason: "private"},
		{name: "DNS rebinding: someone else's domain pointed at the NAS", peer: "192.168.1.23:1", host: "attacker.example:8060", reason: "host"},
		{name: "rebinding page adds its own forwarding headers", peer: "192.168.1.23:1", host: "attacker.example", header: map[string]string{"X-Forwarded-For": "192.168.1.5", "X-Forwarded-Host": "tower.local"}, reason: "host"},
		{name: "proxy reports an unknown public name", peer: "172.18.0.3:1", host: "tower.local", header: map[string]string{"X-Forwarded-For": "192.168.1.5", "X-Forwarded-Host": "attacker.example"}, reason: "host"},
		{name: "opened by a public IP address", peer: "192.168.1.23:1", host: "203.0.113.5:8060", reason: "host"},
		{name: "LAN device", peer: "192.168.1.23:50000", local: true, reason: "private"},
		{name: "10/8", peer: "10.0.0.7:1", local: true, reason: "private"},
		{name: "the host itself", peer: "127.0.0.1:50000", local: true, reason: "loopback"},
		{name: "IPv6 loopback", peer: "[::1]:50000", local: true, reason: "loopback"},
		{name: "IPv6 unique local", peer: "[fd12:3456::9]:1", local: true, reason: "private"},
		{name: "IPv4-mapped LAN", peer: "[::ffff:192.168.1.23]:1", local: true, reason: "private"},
		{name: "internet", peer: "203.0.113.9:443", reason: "public"},
		{name: "global IPv6", peer: "[2001:db8::1]:1", reason: "public"},
		{name: "carrier NAT range is not private", peer: "100.64.3.2:1", reason: "public"},
		{name: "container gateway", peer: "172.17.0.1:40000", reason: "gateway"},
		{name: "gateway cannot vouch for anyone", peer: "172.17.0.1:40000", header: map[string]string{"X-Forwarded-For": "192.168.1.5"}, reason: "gateway"},
		{name: "proxy, LAN visitor", peer: "172.18.0.3:1", header: map[string]string{"X-Forwarded-For": "192.168.1.23", "X-Forwarded-Proto": "https"}, local: true, reason: "private"},
		{name: "proxy, internet visitor", peer: "172.18.0.3:1", header: map[string]string{"X-Forwarded-For": "203.0.113.9"}, reason: "forwarded_public"},
		{name: "visitor claims a LAN address, proxy appends the real one", peer: "172.18.0.3:1", header: map[string]string{"X-Forwarded-For": "192.168.1.5, 203.0.113.9"}, reason: "forwarded_public"},
		{name: "X-Real-IP", peer: "127.0.0.1:1", header: map[string]string{"X-Real-Ip": "198.51.100.4"}, reason: "forwarded_public"},
		{name: "Cloudflare", peer: "127.0.0.1:1", header: map[string]string{"Cf-Connecting-Ip": "198.51.100.4"}, reason: "forwarded_public"},
		{name: "RFC 7239", peer: "10.0.0.2:1", header: map[string]string{"Forwarded": `for="[2001:db8::7]:4711";proto=https`}, reason: "forwarded_public"},
		{name: "RFC 7239 private", peer: "10.0.0.2:1", header: map[string]string{"Forwarded": `for=192.168.1.9;proto=https`}, local: true, reason: "private"},
		{name: "unreadable forwarded value", peer: "10.0.0.2:1", header: map[string]string{"X-Forwarded-For": "unknown"}, reason: "forwarded_unknown"},
		{name: "proxy that hides the visitor", peer: "10.0.0.2:1", header: map[string]string{"X-Forwarded-Proto": "https"}, reason: "proxy"},
		{name: "Via only", peer: "127.0.0.1:1", header: map[string]string{"Via": "1.1 varnish"}, reason: "proxy"},
		{name: "bare nginx proxy_pass", peer: "127.0.0.1:1", proto: "HTTP/1.0", reason: "http10"},
		{name: "public peer claiming to be local", peer: "203.0.113.9:1", header: map[string]string{"X-Forwarded-For": "192.168.1.5"}, reason: "public"},
		{name: "unparseable peer", peer: "@", reason: "unknown"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = c.peer
		req.Host = "nas.lan:8060"
		if c.host != "" {
			req.Host = c.host
		}
		if c.proto == "HTTP/1.0" {
			req.Proto, req.ProtoMajor, req.ProtoMinor = "HTTP/1.0", 1, 0
		}
		for k, v := range c.header {
			req.Header.Set(k, v)
		}
		got := describeClient(req, []string{"nas.example.com"})
		if got.Local != c.local || got.Reason != c.reason {
			t.Errorf("%s: local=%v reason=%q, want local=%v reason=%q", c.name, got.Local, got.Reason, c.local, c.reason)
		}
	}
}

func TestParseDefaultGateways(t *testing.T) {
	table := "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t010011AC\t0003\t0\t0\t0\t00000000\t0\t0\t0\n" +
		"eth0\t000011AC\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0\n" +
		"br0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n" +
		"junk line\n"
	got := parseDefaultGateways(strings.NewReader(table))
	if len(got) != 2 || got[0].String() != "172.17.0.1" || got[1].String() != "192.168.1.1" {
		t.Errorf("gateways = %v, want [172.17.0.1 192.168.1.1]", got)
	}
}

// TestTrustLocalNetwork goes through the router, where chi's RealIP has
// already replaced RemoteAddr with whatever X-Forwarded-For claims. The
// decision must rest on the socket's own address.
func TestTrustLocalNetwork(t *testing.T) {
	withGateways(t, "172.17.0.1")
	srv := newAuthTestServer(t, testAPIKey)
	router := srv.Router()
	from := func(peer string, hdr ...string) func(*http.Request) {
		return func(r *http.Request) {
			r.RemoteAddr = peer
			r.Host = "192.168.1.10:8060"
			for i := 0; i+1 < len(hdr); i += 2 {
				r.Header.Set(hdr[i], hdr[i+1])
			}
		}
	}

	// Off by default: being on the LAN is not enough.
	if rr := do(router, http.MethodGet, "/api/v1/settings", from("192.168.1.23:5000")); rr.Code != http.StatusUnauthorized {
		t.Fatalf("LAN request with the option off = %d, want 401", rr.Code)
	}
	if _, err := srv.updateSettings(func(s *Settings) error { s.Access.TrustLocalNetwork = true; return nil }); err != nil {
		t.Fatal(err)
	}

	allowed := map[string]func(*http.Request){
		"LAN device":              from("192.168.1.23:5000"),
		"the host":                from("127.0.0.1:5000"),
		"LAN visitor via a proxy": from("172.18.0.3:5000", "X-Forwarded-For", "192.168.1.23"),
	}
	for name, mut := range allowed {
		if rr := do(router, http.MethodGet, "/api/v1/settings", mut); rr.Code != http.StatusOK {
			t.Errorf("%s = %d, want 200 without the key", name, rr.Code)
		}
		if rr := do(router, http.MethodGet, "/settings", mut); rr.Code != http.StatusOK {
			t.Errorf("%s: Settings page = %d, want 200 without signing in", name, rr.Code)
		}
	}
	turnedAway := map[string]func(*http.Request){
		"internet":                              from("203.0.113.9:5000"),
		"internet claiming a LAN address":       from("203.0.113.9:5000", "X-Forwarded-For", "192.168.1.23"),
		"internet claiming a LAN address (XRI)": from("203.0.113.9:5000", "X-Real-Ip", "192.168.1.23"),
		"internet visitor via a proxy":          from("172.18.0.3:5000", "X-Forwarded-For", "203.0.113.9"),
		"spoofed and real address":              from("172.18.0.3:5000", "X-Forwarded-For", "192.168.1.5, 203.0.113.9"),
		"container gateway":                     from("172.17.0.1:5000"),
		"proxy that hides the visitor":          from("127.0.0.1:5000", "X-Forwarded-Proto", "https"),
		"page that pointed its domain at the NAS": func(r *http.Request) {
			r.RemoteAddr = "192.168.1.23:5000"
			r.Host = "attacker.example:8060"
			r.Header.Set("X-Forwarded-For", "192.168.1.23")
		},
	}
	for name, mut := range turnedAway {
		if rr := do(router, http.MethodGet, "/api/v1/settings", mut); rr.Code != http.StatusUnauthorized {
			t.Errorf("%s = %d, want 401", name, rr.Code)
		}
	}
	// The key still works from anywhere.
	if rr := do(router, http.MethodGet, "/api/v1/settings", func(r *http.Request) {
		r.RemoteAddr = "203.0.113.9:5000"
		r.Header.Set("Authorization", "Bearer "+testAPIKey)
	}); rr.Code != http.StatusOK {
		t.Errorf("internet request with the key = %d, want 200", rr.Code)
	}
}

// TestCrossOriginReads: another site's page may read monitoring data on an
// install without sign-in, as it always could. It may not read
// configuration or change anything, and with sign-in on it gets nothing.
func TestCrossOriginReads(t *testing.T) {
	cross := func(method, path, preflightFor string) func(http.Handler) *httptest.ResponseRecorder {
		return func(router http.Handler) *httptest.ResponseRecorder {
			req := httptest.NewRequest(method, path, nil)
			req.Host = "nas.lan:8060"
			req.Header.Set("Origin", "https://start.example")
			if preflightFor != "" {
				req.Header.Set("Access-Control-Request-Method", preflightFor)
				req.Header.Set("Access-Control-Request-Headers", "content-type")
			}
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			return rr
		}
	}
	acao := func(rr *httptest.ResponseRecorder) string { return rr.Header().Get("Access-Control-Allow-Origin") }

	open := newAuthTestServer(t, "").Router()
	if rr := cross(http.MethodGet, "/api/v1/status", "")(open); rr.Code != http.StatusOK || acao(rr) != "*" {
		t.Errorf("open install, GET status cross-origin: %d, ACAO %q; want 200 and *", rr.Code, acao(rr))
	}
	if rr := cross(http.MethodOptions, "/api/v1/status", "GET")(open); rr.Code != http.StatusOK || acao(rr) != "*" || rr.Header().Get("Access-Control-Allow-Methods") != "GET" {
		t.Errorf("open install, preflight for GET status: %d, ACAO %q, methods %q", rr.Code, acao(rr), rr.Header().Get("Access-Control-Allow-Methods"))
	}
	for _, path := range []string{"/api/v1/settings", "/api/v1/fleet/servers", "/api/v1/backup", "/api/v1/access", "/settings", "/"} {
		if rr := cross(http.MethodGet, path, "")(open); acao(rr) != "" {
			t.Errorf("open install: %s is readable by another site's page (ACAO %q)", path, acao(rr))
		}
	}
	for _, m := range []string{"PUT", "POST", "DELETE"} {
		for _, path := range []string{"/api/v1/settings", "/api/v1/scan", "/api/v1/status"} {
			rr := cross(http.MethodOptions, path, m)(open)
			if acao(rr) != "" || rr.Header().Get("Access-Control-Allow-Methods") != "" {
				t.Errorf("open install: preflight for %s %s was granted", m, path)
			}
		}
	}
	// A page on the same host is not cross-origin and gets no CORS header.
	same := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	same.Host = "nas.lan:8060"
	same.Header.Set("Origin", "http://nas.lan:8060")
	rr := httptest.NewRecorder()
	open.ServeHTTP(rr, same)
	if acao(rr) != "" {
		t.Errorf("same-origin request got ACAO %q", acao(rr))
	}

	upgraded := upgradedServer(t, oldBlobWithKey).Router()
	if rr := cross(http.MethodGet, "/api/v1/disks", "")(upgraded); acao(rr) != "*" {
		t.Errorf("upgraded install, GET disks cross-origin: ACAO %q, want *", acao(rr))
	}
	if rr := cross(http.MethodGet, "/api/v1/settings", "")(upgraded); acao(rr) != "" {
		t.Errorf("upgraded install: settings readable cross-origin")
	}

	protected := newAuthTestServer(t, testAPIKey).Router()
	if rr := cross(http.MethodGet, "/api/v1/status", "")(protected); acao(rr) != "" {
		t.Errorf("sign-in on: ACAO %q, want none", acao(rr))
	}
	if rr := cross(http.MethodOptions, "/api/v1/status", "GET")(protected); acao(rr) != "" {
		t.Errorf("sign-in on: preflight granted")
	}
}

// TestKeylessClients: what uses the instance without the key is listed for
// the Settings page, so turning sign-in on is not a leap in the dark.
func TestKeylessClients(t *testing.T) {
	srv := upgradedServer(t, oldBlobWithKey)
	router := srv.Router()
	agent := func(ua string, more ...func(*http.Request)) func(*http.Request) {
		return func(r *http.Request) {
			r.Header.Set("User-Agent", ua)
			r.RemoteAddr = "192.168.1.5:40000"
			r.Host = "tower.local:8060"
			for _, m := range more {
				m(r)
			}
		}
	}
	do(router, http.MethodGet, "/api/v1/disks", agent("Prometheus/2.53.0"))
	do(router, http.MethodGet, "/api/v1/disks/ABC123", agent("Prometheus/2.53.0"))
	do(router, http.MethodGet, "/api/v1/alerts", agent("python-requests/2.32 (linux)"))
	// A person's browser can sign in; it is not listed.
	do(router, http.MethodGet, "/settings", agent("Mozilla/5.0 (X11; Linux x86_64) Firefox/131.0"))
	// Something that already sends the key will keep working; not listed.
	do(router, http.MethodGet, "/api/v1/sparklines", agent("curl/8.5.0", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer nd-old-key")
	}))
	// A page framed by another site cannot sign in there; listed.
	do(router, http.MethodGet, "/", agent("Mozilla/5.0 (Macintosh) Safari/605", func(r *http.Request) {
		r.Header.Set("Sec-Fetch-Dest", "iframe")
		r.Header.Set("Referer", "https://home.example/dashboard")
	}))

	status := func() accessStatus {
		rr := do(router, http.MethodGet, "/api/v1/access", agent("Mozilla/5.0"))
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /api/v1/access = %d: %s", rr.Code, rr.Body.String())
		}
		var st accessStatus
		if err := json.Unmarshal(rr.Body.Bytes(), &st); err != nil {
			t.Fatal(err)
		}
		return st
	}
	st := status()
	if st.Mode != "api_key" || !st.KeySet || st.RequireSignIn {
		t.Errorf("status = %+v", st)
	}
	if st.Client.Address != "192.168.1.5" || !st.Client.Local {
		t.Errorf("client view = %+v, want the LAN address, local", st.Client)
	}
	byAgent := map[string]keylessClient{}
	for _, c := range st.KeylessClients {
		byAgent[c.Agent+" "+c.Path] = c
	}
	if c, ok := byAgent["Prometheus/2.53.0 /api/v1/disks"]; !ok || c.Count != 2 || c.Refused || !c.Local {
		t.Errorf("Prometheus entry = %+v (present %v); want 2 requests grouped, local, not refused", c, ok)
	}
	if _, ok := byAgent["python-requests/2.32 /api/v1/alerts"]; !ok {
		t.Errorf("script not listed: %v", byAgent)
	}
	if c, ok := byAgent["embedded in home.example /"]; !ok || !c.Embedded {
		t.Errorf("embedded page not listed: %v", byAgent)
	}
	if len(st.KeylessClients) != 3 {
		t.Errorf("listed %d clients, want 3: %+v", len(st.KeylessClients), st.KeylessClients)
	}

	// With sign-in on, the same scraper is turned away and shown as such.
	if _, err := srv.updateSettings(func(s *Settings) error { s.Access.RequireSignIn = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if rr := do(router, http.MethodGet, "/api/v1/disks", agent("Prometheus/2.53.0")); rr.Code != http.StatusUnauthorized {
		t.Fatalf("scraper after sign-in was turned on = %d, want 401", rr.Code)
	}
	since, list := srv.keyless.list()
	if since.IsZero() {
		t.Error("tracking start time not recorded")
	}
	var prom *keylessClient
	for i := range list {
		if list[i].Agent == "Prometheus/2.53.0" && list[i].Path == "/api/v1/disks" {
			prom = &list[i]
		}
	}
	if prom == nil || !prom.Refused || prom.Count != 3 {
		t.Errorf("scraper entry after sign-in = %+v, want refused with 3 requests", prom)
	}

	// The list is bounded.
	for i := 0; i < maxKeylessClients*2; i++ {
		do(router, http.MethodGet, "/api/v1/disks", agent(fmt.Sprintf("bot-%d/1.0", i)))
	}
	if _, list := srv.keyless.list(); len(list) > maxKeylessClients {
		t.Errorf("list grew to %d entries, cap is %d", len(list), maxKeylessClients)
	}
}

// TestAccessNotice: an upgraded install with a key is told once, on the
// dashboard, that sign-in exists. Nobody else is.
func TestAccessNotice(t *testing.T) {
	notice := func(srv *Server) bool {
		rr := do(srv.Router(), http.MethodGet, "/api/v1/status", func(r *http.Request) {
			r.Header.Set("Referer", "http://localhost/")
		})
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d", rr.Code)
		}
		var body struct {
			AccessNotice bool `json:"access_notice"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &body)
		return body.AccessNotice
	}
	if notice(newAuthTestServer(t, "")) {
		t.Error("notice shown on an install with no key")
	}
	srv := upgradedServer(t, oldBlobWithKey)
	if !notice(srv) {
		t.Fatal("notice not shown on an upgraded install with a key")
	}
	if rr := do(srv.Router(), http.MethodPost, "/api/v1/access/dismiss-notice", nil); rr.Code != http.StatusOK {
		t.Fatalf("dismiss = %d: %s", rr.Code, rr.Body.String())
	}
	if notice(srv) {
		t.Error("notice still shown after it was dismissed")
	}
	if srv.getSettings().accessMode() != accessKeyForAPI {
		t.Error("dismissing the notice changed the access mode")
	}
}

func TestShowAPIKey(t *testing.T) {
	store := storage.NewFakeStore()
	if key, err := ShowAPIKey(store); err != nil || key != "" {
		t.Errorf("no settings: %q, %v", key, err)
	}
	_ = store.SetConfig(settingsConfigKey, oldBlobWithKey)
	if key, err := ShowAPIKey(store); err != nil || key != "nd-old-key" {
		t.Errorf("stored key: %q, %v", key, err)
	}
}

// TestLocalHosts: the names NAS Doctor answers to for the local-network
// option are tidied on save, bounded, and dropped with the option.
func TestLocalHosts(t *testing.T) {
	srv := newSettingsTestServer()
	rr := putSettingsJSON(t, srv, `{"access":{"trust_local_network":true,"local_hosts":["NAS.Example.com:443","nas.example.com"," tower.lan "]}}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("save = %d: %s", rr.Code, rr.Body.String())
	}
	if got := srv.getSettings().Access.LocalHosts; len(got) != 2 || got[0] != "nas.example.com" || got[1] != "tower.lan" {
		t.Errorf("stored names = %v, want [nas.example.com tower.lan]", got)
	}
	if rr := putSettingsJSON(t, srv, `{"access":{"local_hosts":["http://x/y"]}}`); rr.Code != http.StatusBadRequest {
		t.Errorf("a URL as a host name = %d, want 400", rr.Code)
	}
	many := make([]string, MaxLocalHosts+1)
	for i := range many {
		many[i] = fmt.Sprintf("h%d.example.com", i)
	}
	body, _ := json.Marshal(map[string]any{"access": map[string]any{"local_hosts": many}})
	if rr := putSettingsJSON(t, srv, string(body)); rr.Code != http.StatusBadRequest {
		t.Errorf("%d names = %d, want 400", len(many), rr.Code)
	}
	// Switching the option off forgets the names.
	putSettingsJSON(t, srv, `{"access":{"trust_local_network":false}}`)
	if got := srv.getSettings().Access.LocalHosts; len(got) != 0 {
		t.Errorf("names kept after the option was switched off: %v", got)
	}
}
