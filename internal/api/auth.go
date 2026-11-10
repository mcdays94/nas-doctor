package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Authentication model
//
// When no API key is configured the instance is open, as it always has
// been. When a key is configured, EVERY route requires it except the small
// allow-list in isPublicPath (health probe, the sign-in page and the static
// assets that page needs).
//
// A request proves it holds the key in one of three ways:
//
//   - Authorization: Bearer <key>   — API clients, fleet polling, Prometheus
//   - ?api_key=<key>                — kept for existing integrations
//   - the nd_session cookie         — the browser UI, issued by /login
//
// The cookie replaces the old "Referer contains our host" exemption, which
// any client could satisfy by sending a header. The cookie value is a
// stateless HMAC over an expiry time, keyed by the API key itself: sessions
// survive restarts, need no server-side table, and every session dies the
// moment the key is rotated or revoked.

const (
	sessionCookieName = "nd_session"
	sessionTTL        = 30 * 24 * time.Hour
	// requestBodyLimit caps every request body. Nothing the UI or the
	// documented API sends comes close; an unbounded body used to be
	// persisted verbatim by a few handlers.
	requestBodyLimit = 1 << 20
)

// secureEqual compares two secrets in constant time without leaking their
// lengths.
func secureEqual(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

func sessionMAC(apiKey string, exp int64) string {
	m := hmac.New(sha256.New, []byte(apiKey))
	m.Write([]byte("nas-doctor-session-v1|" + strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(m.Sum(nil))
}

// newSessionToken returns a token valid until exp for the given API key.
func newSessionToken(apiKey string, exp time.Time) string {
	e := exp.Unix()
	return strconv.FormatInt(e, 10) + "." + sessionMAC(apiKey, e)
}

// validSessionToken reports whether token was issued for apiKey and has not
// expired.
func validSessionToken(apiKey, token string, now time.Time) bool {
	if apiKey == "" {
		return false
	}
	expStr, mac, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || now.Unix() >= exp {
		return false
	}
	return hmac.Equal([]byte(mac), []byte(sessionMAC(apiKey, exp)))
}

// requestIsHTTPS reports whether the client reached us over TLS, directly
// or through a reverse proxy that says so.
func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, apiKey string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    newSessionToken(apiKey, time.Now().Add(sessionTTL)),
		Path:     "/",
		MaxAge:   int(sessionTTL / time.Second),
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		// Lax, not Strict: a link to the dashboard from a chat notification
		// should not bounce a signed-in user through the sign-in page. Lax
		// cookies are never sent on cross-site POST/PUT/DELETE, and every
		// state-changing route is non-GET.
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// bearerToken extracts the token from an Authorization: Bearer header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// requestAuthorized reports whether r carries proof of the API key.
func requestAuthorized(r *http.Request, apiKey string) bool {
	if apiKey == "" {
		return true
	}
	if tok := bearerToken(r); tok != "" && secureEqual(tok, apiKey) {
		return true
	}
	if q := r.URL.Query().Get("api_key"); q != "" && secureEqual(q, apiKey) {
		return true
	}
	if c, err := r.Cookie(sessionCookieName); err == nil && validSessionToken(apiKey, c.Value, time.Now()) {
		return true
	}
	return false
}

// isPublicPath lists the routes that stay reachable without the key.
func isPublicPath(path string) bool {
	switch path {
	case "/api/v1/health", // Docker HEALTHCHECK, Kubernetes probes, fleet discovery
		"/login", "/api/v1/auth/login", "/api/v1/auth/logout",
		"/css/shared.css", "/icon.png", "/favicon.png": // what the sign-in page renders with
		return true
	}
	return strings.HasPrefix(path, "/icons/")
}

// isPageRequest reports whether an unauthenticated request should be sent
// to the sign-in page (a person navigating) rather than answered with a
// JSON 401 (a script or API client).
func isPageRequest(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	p := r.URL.Path
	for _, prefix := range []string{"/api/", "/js/", "/css/"} {
		if strings.HasPrefix(p, prefix) {
			return false
		}
	}
	return p != "/metrics"
}

// authMiddleware enforces the API key on every route when one is set.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		settings, err := s.loadSettings()
		if err != nil {
			// Fail closed. Treating an unreadable settings row as "no key
			// configured" would switch authentication off exactly when the
			// database is in trouble.
			s.logger.Error("auth: cannot read settings", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "settings unavailable"})
			return
		}
		if requestAuthorized(r, settings.APIKey) {
			next.ServeHTTP(w, r)
			return
		}
		if isPageRequest(r) {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="nas-doctor"`)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or missing API key"})
	})
}

// apiKeyMiddleware is the historical name for authMiddleware, kept so
// route-level wrappers and tests that reference it keep working. The whole
// router is now behind authMiddleware, so wrapping a route again is a no-op.
func (s *Server) apiKeyMiddleware(next http.Handler) http.Handler {
	return s.authMiddleware(next)
}

// sameHost compares two host[:port] values case-insensitively.
func sameHost(a, b string) bool {
	return a != "" && strings.EqualFold(a, b)
}

// sameOriginGuard rejects state-changing requests that a browser sent from
// another site. It is what stops any web page the user happens to visit
// from POSTing to the NAS on their behalf — which matters most on installs
// with no API key, where there is no cookie to withhold.
//
// Non-browser clients (curl, the fleet poller, Prometheus) send neither
// Origin nor Sec-Fetch-Site and pass untouched. A request that presents the
// API key as a Bearer token is an explicit API client and also passes.
func (s *Server) sameOriginGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if bearerToken(r) != "" {
			// Browsers cannot attach an Authorization header cross-site
			// without a CORS preflight, which this server never grants.
			next.ServeHTTP(w, r)
			return
		}
		switch r.Header.Get("Sec-Fetch-Site") {
		case "same-origin", "none":
			next.ServeHTTP(w, r)
			return
		case "cross-site", "same-site":
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-origin request refused"})
			return
		}
		// Sec-Fetch-Site is only sent in secure contexts. On plain-HTTP LAN
		// access fall back to comparing Origin with the host we were
		// addressed as (or the host a reverse proxy says the client used).
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		u, err := url.Parse(origin)
		if err == nil && (sameHost(u.Host, r.Host) || sameHost(u.Host, r.Header.Get("X-Forwarded-Host"))) {
			next.ServeHTTP(w, r)
			return
		}
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-origin request refused"})
	})
}

// contentSecurityPolicy allows what the embedded UI actually loads: its own
// scripts and styles (inline handlers included — the templates rely on
// them), Google Fonts, and same-origin API calls. It gives injected markup
// nowhere to send data with fetch or image requests.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline'; " +
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
	"font-src 'self' data: https://fonts.gstatic.com; " +
	"img-src 'self' data: blob:; " +
	"connect-src 'self'; " +
	"object-src 'none'; base-uri 'self'; form-action 'self'"

// securityHeaders sets response headers that apply to every route.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		next.ServeHTTP(w, r)
	})
}

// limitRequestBody caps every request body at requestBodyLimit.
func (s *Server) limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Body != http.NoBody {
			r.Body = http.MaxBytesReader(w, r.Body, requestBodyLimit)
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- Sign-in ----------

// loginLimiter slows key guessing through the sign-in form: a handful of
// failures per client address, plus a global ceiling so rotating the
// (spoofable) forwarded address does not help.
type loginLimiter struct {
	mu      sync.Mutex
	perIP   map[string]*loginWindow
	global  loginWindow
	nowFunc func() time.Time
}

type loginWindow struct {
	start time.Time
	fails int
}

const (
	loginWindowLen      = 5 * time.Minute
	loginMaxFailsPerIP  = 10
	loginMaxFailsGlobal = 100
)

func (l *loginLimiter) now() time.Time {
	if l.nowFunc != nil {
		return l.nowFunc()
	}
	return time.Now()
}

func (w *loginWindow) roll(now time.Time) {
	if now.Sub(w.start) >= loginWindowLen {
		w.start, w.fails = now, 0
	}
}

// blocked reports whether ip has used up its attempts for this window.
func (l *loginLimiter) blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.global.roll(now)
	if l.global.fails >= loginMaxFailsGlobal {
		return true
	}
	if w := l.perIP[ip]; w != nil {
		w.roll(now)
		return w.fails >= loginMaxFailsPerIP
	}
	return false
}

func (l *loginLimiter) recordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.global.roll(now)
	l.global.fails++
	if l.perIP == nil {
		l.perIP = make(map[string]*loginWindow)
	}
	// Opportunistic cleanup so the map cannot grow without bound.
	if len(l.perIP) > 1024 {
		for k, w := range l.perIP {
			if now.Sub(w.start) >= loginWindowLen {
				delete(l.perIP, k)
			}
		}
	}
	w := l.perIP[ip]
	if w == nil {
		w = &loginWindow{start: now}
		l.perIP[ip] = w
	}
	w.roll(now)
	w.fails++
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// handleLogin exchanges the API key for a session cookie.
// POST /api/v1/auth/login {"api_key":"..."}
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	settings, err := s.loadSettings()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "settings unavailable"})
		return
	}
	if settings.APIKey == "" {
		// Nothing to sign in to; let the page carry on to the dashboard.
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "auth_required": false})
		return
	}
	ip := clientIP(r)
	if s.loginLimiter.blocked(ip) {
		w.Header().Set("Retry-After", strconv.Itoa(int(loginWindowLen/time.Second)))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts, try again in a few minutes"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !secureEqual(strings.TrimSpace(req.APIKey), settings.APIKey) {
		s.loginLimiter.recordFailure(ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "that key is not correct"})
		return
	}
	setSessionCookie(w, r, settings.APIKey)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "auth_required": true})
}

// handleLogout clears the session cookie.
// POST /api/v1/auth/logout
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "signed out"})
}

// handleLoginPage serves the sign-in form. When no key is configured, or
// the visitor is already signed in, it goes straight to the dashboard.
// GET /login
func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if settings, err := s.loadSettings(); err == nil && requestAuthorized(r, settings.APIKey) {
		http.Redirect(w, r, safeNextPath(r.URL.Query().Get("next")), http.StatusFound)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	s.servePage(w, loginPageHTML)
}

// safeNextPath returns next when it is a local path, otherwise "/". It
// keeps the sign-in redirect from being used to bounce people to another
// site.
func safeNextPath(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	if strings.HasPrefix(next, "/login") {
		return "/"
	}
	return next
}
