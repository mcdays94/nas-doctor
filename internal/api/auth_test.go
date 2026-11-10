package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcdays94/nas-doctor/internal/storage"
)

const testAPIKey = "nd-test-key-0123456789"

// newAuthTestServer returns a Server on a FakeStore. When key is non-empty
// it is stored as the instance API key.
func newAuthTestServer(t *testing.T, key string) *Server {
	t.Helper()
	srv := &Server{
		store:     storage.NewFakeStore(),
		logger:    slog.Default(),
		version:   "test",
		startTime: time.Now(),
	}
	if key != "" {
		if _, err := srv.updateSettings(func(s *Settings) error { s.APIKey = key; return nil }); err != nil {
			t.Fatalf("seed api key: %v", err)
		}
	}
	return srv
}

var routeParamRE = regexp.MustCompile(`\{[^}]+\}`)

// concretePath turns a chi route pattern into a requestable path.
func concretePath(pattern string) string {
	p := routeParamRE.ReplaceAllStringFunc(pattern, func(m string) string {
		switch m {
		case "{theme}":
			return "clean"
		case "{name}":
			return "icon3"
		}
		return "1"
	})
	return strings.TrimSuffix(p, "/*")
}

// TestAuth_EveryRouteRequiresTheKey walks the real router. With a key set,
// every route outside the public allow-list must refuse a request that
// carries no proof of the key — and none of the allow-listed routes may.
//
// This is the regression test for the API key protecting only the handful
// of routes that happened to be registered inside one sub-router, while
// settings (including the key itself), backups, alerts and the rest stayed
// open.
func TestAuth_EveryRouteRequiresTheKey(t *testing.T) {
	srv := newAuthTestServer(t, testAPIKey)
	router := srv.Router()

	routes, ok := router.(chi.Routes)
	if !ok {
		t.Fatal("Router() does not expose chi.Routes")
	}

	seen := 0
	err := chi.Walk(routes, func(method, pattern string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		seen++
		path := concretePath(pattern)
		var body *bytes.Reader
		if method == http.MethodGet || method == http.MethodHead {
			body = bytes.NewReader(nil)
		} else {
			body = bytes.NewReader([]byte(`{}`))
		}
		req := httptest.NewRequest(method, path, body)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		refused := rr.Code == http.StatusUnauthorized ||
			(rr.Code == http.StatusFound && strings.HasPrefix(rr.Header().Get("Location"), "/login"))

		if isPublicPath(path) {
			// The sign-in endpoint answers 401 itself for a wrong key;
			// that is its job, not the gate refusing the route.
			if refused && path != "/api/v1/auth/login" {
				t.Errorf("%s %s is on the public list but was refused (%d)", method, pattern, rr.Code)
			}
			return nil
		}
		if !refused {
			t.Errorf("%s %s answered %d without the API key; want 401 or a redirect to /login", method, pattern, rr.Code)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if seen < 60 {
		t.Fatalf("walked only %d routes; the router changed shape and this test may be vacuous", seen)
	}
}

// TestAuth_BearerOpensEveryRoute is the other half: holding the key must
// get past the gate on every route (whatever the handler then answers).
func TestAuth_BearerOpensEveryRoute(t *testing.T) {
	srv := newAuthTestServer(t, testAPIKey)
	router := srv.Router()
	routes := router.(chi.Routes)

	_ = chi.Walk(routes, func(method, pattern string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method != http.MethodGet {
			return nil // side-effect free subset is enough to prove the gate opens
		}
		if strings.Contains(pattern, "/stream/") {
			return nil // SSE: would hold the connection
		}
		req := httptest.NewRequest(method, concretePath(pattern), nil)
		req.Header.Set("Authorization", "Bearer "+testAPIKey)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code == http.StatusUnauthorized {
			t.Errorf("%s %s refused a valid Bearer key", method, pattern)
		}
		if rr.Code == http.StatusFound && strings.HasPrefix(rr.Header().Get("Location"), "/login") {
			t.Errorf("%s %s sent a valid Bearer key to /login", method, pattern)
		}
		return nil
	})
}

func TestAuth_NoKeyMeansOpen(t *testing.T) {
	srv := newAuthTestServer(t, "")
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("no key configured: GET /api/v1/settings = %d, want 200", rr.Code)
	}
}

// TestAuth_SettingsDoNotLeakTheKey is the concrete case from the audit:
// with a key set, the settings endpoint answered without it and returned
// the key in the body.
func TestAuth_SettingsDoNotLeakTheKey(t *testing.T) {
	srv := newAuthTestServer(t, testAPIKey)
	router := srv.Router()

	for _, method := range []string{http.MethodGet, http.MethodPut} {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(method, "/api/v1/settings", strings.NewReader(`{"api_key":""}`)))
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s /api/v1/settings without key = %d, want 401", method, rr.Code)
		}
		if strings.Contains(rr.Body.String(), testAPIKey) {
			t.Errorf("%s /api/v1/settings leaked the API key to an unauthenticated caller", method)
		}
	}
	if got := srv.getSettings().APIKey; got != testAPIKey {
		t.Fatalf("unauthenticated PUT changed the key to %q", got)
	}
}

// TestAuth_RefererDoesNotBypass pins the removal of the "Referer contains
// our host or the word localhost" exemption.
func TestAuth_RefererDoesNotBypass(t *testing.T) {
	srv := newAuthTestServer(t, testAPIKey)
	router := srv.Router()
	for _, ref := range []string{
		"http://localhost/",
		"https://evil.example/?localhost",
		"http://example.com/",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		req.Host = "example.com"
		req.Header.Set("Referer", ref)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Referer %q: got %d, want 401", ref, rr.Code)
		}
	}
}

func TestAuth_QueryParamKeyStillWorks(t *testing.T) {
	srv := newAuthTestServer(t, testAPIKey)
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/status?api_key="+testAPIKey, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("?api_key= = %d, want 200", rr.Code)
	}
}

func TestAuth_PagesRedirectToLogin(t *testing.T) {
	srv := newAuthTestServer(t, testAPIKey)
	router := srv.Router()

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/settings?tab=x", nil))
	if rr.Code != http.StatusFound {
		t.Fatalf("GET /settings = %d, want 302", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login?next=%2Fsettings%3Ftab%3Dx" {
		t.Errorf("Location = %q", loc)
	}

	// The sign-in page and what it needs are reachable.
	for _, p := range []string{"/login", "/css/shared.css", "/icon.png", "/api/v1/health"} {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		if rr.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", p, rr.Code)
		}
	}
}

func loginRequest(key string) *http.Request {
	body, _ := json.Marshal(map[string]string{"api_key": key})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func sessionCookieFrom(rr *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rr.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	return nil
}

func TestAuth_LoginIssuesSessionCookie(t *testing.T) {
	srv := newAuthTestServer(t, testAPIKey)
	router := srv.Router()

	// Wrong key: 401, no cookie.
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, loginRequest("nope"))
	if rr.Code != http.StatusUnauthorized || sessionCookieFrom(rr) != nil {
		t.Fatalf("wrong key: code=%d cookie=%v", rr.Code, sessionCookieFrom(rr))
	}

	// Right key: cookie with the properties the browser needs.
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, loginRequest(testAPIKey))
	if rr.Code != http.StatusOK {
		t.Fatalf("login = %d: %s", rr.Code, rr.Body.String())
	}
	c := sessionCookieFrom(rr)
	if c == nil {
		t.Fatal("no session cookie set")
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		t.Errorf("cookie flags: HttpOnly=%v SameSite=%v Path=%q", c.HttpOnly, c.SameSite, c.Path)
	}
	if strings.Contains(c.Value, testAPIKey) {
		t.Error("session cookie contains the API key itself")
	}

	// The cookie opens the API and the pages.
	for _, p := range []string{"/api/v1/status", "/settings"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.AddCookie(c)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("GET %s with session = %d, want 200", p, rr.Code)
		}
	}

	// Rotating the key ends the session.
	if _, err := srv.updateSettings(func(s *Settings) error { s.APIKey = "nd-rotated"; return nil }); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.AddCookie(c)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("old session after key rotation = %d, want 401", rr.Code)
	}
}

func TestSessionToken_ExpiryAndTamper(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tok := newSessionToken(testAPIKey, now.Add(time.Hour))
	if !validSessionToken(testAPIKey, tok, now) {
		t.Fatal("fresh token rejected")
	}
	if validSessionToken(testAPIKey, tok, now.Add(2*time.Hour)) {
		t.Error("expired token accepted")
	}
	if validSessionToken("other-key", tok, now) {
		t.Error("token accepted for a different key")
	}
	// Extending the expiry without re-signing must fail.
	_, mac, _ := strings.Cut(tok, ".")
	forged := "9999999999." + mac
	if validSessionToken(testAPIKey, forged, now) {
		t.Error("token with a forged expiry accepted")
	}
	for _, bad := range []string{"", ".", "abc", "123.", ".abc"} {
		if validSessionToken(testAPIKey, bad, now) {
			t.Errorf("malformed token %q accepted", bad)
		}
	}
	if validSessionToken("", newSessionToken("", now.Add(time.Hour)), now) {
		t.Error("token accepted when no key is configured")
	}
}

func TestAuth_LoginIsRateLimited(t *testing.T) {
	srv := newAuthTestServer(t, testAPIKey)
	router := srv.Router()
	var last int
	for i := 0; i < loginMaxFailsPerIP+1; i++ {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, loginRequest("wrong"))
		last = rr.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("after %d failures got %d, want 429", loginMaxFailsPerIP+1, last)
	}
	// Even the right key is refused until the window passes.
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, loginRequest(testAPIKey))
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("correct key during lockout = %d, want 429", rr.Code)
	}
	// The window rolls.
	srv.loginLimiter.nowFunc = func() time.Time { return time.Now().Add(loginWindowLen + time.Second) }
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, loginRequest(testAPIKey))
	if rr.Code != http.StatusOK {
		t.Errorf("after the window = %d, want 200", rr.Code)
	}
}

func TestSafeNextPath(t *testing.T) {
	cases := map[string]string{
		"":                     "/",
		"/":                    "/",
		"/settings":            "/settings",
		"/disk/ABC?range=7d":   "/disk/ABC?range=7d",
		"//evil.example":       "/",
		"/\\evil.example":      "/",
		"https://evil.example": "/",
		"javascript:alert(1)":  "/",
		"/login?next=/":        "/",
	}
	for in, want := range cases {
		if got := safeNextPath(in); got != want {
			t.Errorf("safeNextPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSameOriginGuard covers the cross-site request protection that
// replaces the wildcard CORS policy.
func TestSameOriginGuard(t *testing.T) {
	srv := newAuthTestServer(t, "") // no key: the guard is the only defence
	router := srv.Router()

	post := func(mut func(*http.Request)) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/findings/dismiss", strings.NewReader(`{"title":"x"}`))
		req.Host = "nas.local:8060"
		mut(req)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr.Code
	}

	if c := post(func(r *http.Request) {}); c != http.StatusOK {
		t.Errorf("no Origin (curl, scripts) = %d, want 200", c)
	}
	if c := post(func(r *http.Request) { r.Header.Set("Origin", "http://nas.local:8060") }); c != http.StatusOK {
		t.Errorf("same Origin = %d, want 200", c)
	}
	if c := post(func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }); c != http.StatusForbidden {
		t.Errorf("foreign Origin = %d, want 403", c)
	}
	if c := post(func(r *http.Request) { r.Header.Set("Origin", "null") }); c != http.StatusForbidden {
		t.Errorf("null Origin = %d, want 403", c)
	}
	if c := post(func(r *http.Request) { r.Header.Set("Origin", "http://nas.local:9999") }); c != http.StatusForbidden {
		t.Errorf("same host, other port = %d, want 403", c)
	}
	if c := post(func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }); c != http.StatusForbidden {
		t.Errorf("Sec-Fetch-Site: cross-site = %d, want 403", c)
	}
	// Behind a TLS-terminating proxy the browser says same-origin even
	// though the Host header we see is the upstream's.
	if c := post(func(r *http.Request) {
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("Origin", "https://nas.example.com")
	}); c != http.StatusOK {
		t.Errorf("Sec-Fetch-Site: same-origin = %d, want 200", c)
	}
	if c := post(func(r *http.Request) {
		r.Header.Set("Origin", "http://nas.example.com")
		r.Header.Set("X-Forwarded-Host", "nas.example.com")
	}); c != http.StatusOK {
		t.Errorf("Origin matching X-Forwarded-Host = %d, want 200", c)
	}

	// Reads are not blocked by the guard (the browser's same-origin policy
	// already keeps the response from a foreign page).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Header.Set("Origin", "https://evil.example")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("cross-origin GET = %d, want 200", rr.Code)
	}
}

// TestNoCORSHeaders pins the removal of Access-Control-Allow-Origin: *.
func TestNoCORSHeaders(t *testing.T) {
	srv := newAuthTestServer(t, "")
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req.Header.Set("Origin", "https://evil.example")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if v := rr.Header().Get("Access-Control-Allow-Origin"); v != "" {
		t.Errorf("Access-Control-Allow-Origin = %q on a GET, want none", v)
	}

	pre := httptest.NewRequest(http.MethodOptions, "/api/v1/settings", nil)
	pre.Header.Set("Origin", "https://evil.example")
	pre.Header.Set("Access-Control-Request-Method", "PUT")
	pre.Header.Set("Access-Control-Request-Headers", "content-type")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, pre)
	if v := rr.Header().Get("Access-Control-Allow-Origin"); v != "" {
		t.Errorf("preflight granted Access-Control-Allow-Origin = %q", v)
	}
	if v := rr.Header().Get("Access-Control-Allow-Methods"); v != "" {
		t.Errorf("preflight granted methods %q", v)
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv := newAuthTestServer(t, "")
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/settings", nil))
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options: nosniff")
	}
	csp := rr.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "connect-src 'self'", "object-src 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP %q missing %q", csp, want)
		}
	}
}

// failingConfigStore makes every settings read fail.
type failingConfigStore struct{ storage.Store }

func (failingConfigStore) GetConfig(string) (string, error) {
	return "", errors.New("database is locked")
}

// TestAuth_FailsClosedWhenSettingsUnreadable: an unreadable settings row
// must not be treated as "no key configured".
func TestAuth_FailsClosedWhenSettingsUnreadable(t *testing.T) {
	srv := newAuthTestServer(t, testAPIKey)
	srv.store = failingConfigStore{srv.store}
	router := srv.Router()

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status with unreadable settings = %d, want 503", rr.Code)
	}
	// The health probe keeps working.
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("health = %d, want 200", rr.Code)
	}
}

// TestRequestBodyLimit: oversized bodies are rejected, not stored.
func TestRequestBodyLimit(t *testing.T) {
	srv := newAuthTestServer(t, "")
	router := srv.Router()

	big := `{"cols":["` + strings.Repeat("a", requestBodyLimit+1024) + `"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/section-order", strings.NewReader(big))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("oversized section-order = %d, want 400", rr.Code)
	}
	if len(srv.getSettings().SectionOrder) != 0 {
		t.Error("oversized layout was stored")
	}

	// A well-formed layout is accepted; junk ids are not.
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/v1/settings/section-order", strings.NewReader(`{"cols":["findings","docker"]}`)))
	if rr.Code != http.StatusOK {
		t.Errorf("valid layout = %d: %s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/v1/settings/section-order", strings.NewReader(`{"cols":["<script>"]}`)))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("layout with markup in an id = %d, want 400", rr.Code)
	}
}

// TestThemeURLDoesNotChangeSettings: GET /theme/{theme} is a preview only.
func TestThemeURLDoesNotChangeSettings(t *testing.T) {
	srv := newAuthTestServer(t, "")
	before := srv.getSettings().Theme
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/theme/clean", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /theme/clean = %d, want 200", rr.Code)
	}
	if after := srv.getSettings().Theme; after != before {
		t.Errorf("GET /theme/clean changed the saved theme from %q to %q", before, after)
	}
}
