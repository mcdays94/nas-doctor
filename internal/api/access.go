package api

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Access model
//
// An instance is in one of three modes, decided by two settings (the API
// key and access.require_sign_in):
//
//	open       no API key. Everything is reachable, as it always has been.
//	key-for-API  a key is set, sign-in is off. The key is asked of exactly
//	           the routes it guarded before sign-in existed (status,
//	           snapshots, scan, report, live speed test, Duplicacy test),
//	           with the same same-page exemption. The dashboard, Settings
//	           and /metrics stay open.
//	sign-in    a key is set and sign-in is on. Every route needs the key
//	           or a signed-in browser (auth.go).
//
// The middle mode exists so that upgrading never changes how an install
// behaves. An install that already had a key was told "the dashboard UI is
// not affected", and its owner may have generated that key once for fleet
// polling and never written it down. It keeps the old rules until someone
// turns sign-in on from Settings, where the key is shown first.

// AccessSettings controls who is asked for the API key.
type AccessSettings struct {
	// RequireSignIn puts every page and endpoint behind the API key; a
	// browser signs in once. It has no effect while no key is set.
	RequireSignIn bool `json:"require_sign_in"`
	// SessionDays is how long a browser stays signed in, counted from
	// its last request. 0 means until it signs out.
	SessionDays int `json:"session_days"`
	// TrustLocalNetwork lets requests that demonstrably come from a
	// private address in without the key. Only used with RequireSignIn.
	TrustLocalNetwork bool `json:"trust_local_network"`
	// LocalHosts are extra host names under which NAS Doctor may be
	// opened for TrustLocalNetwork to apply (see hostIsOurs). Names that
	// can only exist on a private network need not be listed.
	LocalHosts []string `json:"local_hosts,omitempty"`
	// NoticeDismissed records that the dashboard notice shown to
	// installs that upgraded with a key has been dismissed.
	NoticeDismissed bool `json:"notice_dismissed,omitempty"`
}

// MaxLocalHosts bounds AccessSettings.LocalHosts.
const MaxLocalHosts = 10

const (
	// DefaultSessionDays is how long a browser stays signed in unless
	// changed in Settings. Sessions slide: the period restarts whenever
	// the browser is used.
	DefaultSessionDays = 365
	// MaxSessionDays bounds the setting; 0 (no expiry) is also allowed.
	MaxSessionDays = 3650
)

type accessMode int

const (
	accessOpen accessMode = iota
	accessKeyForAPI
	accessSignIn
)

func (m accessMode) String() string {
	switch m {
	case accessKeyForAPI:
		return "api_key"
	case accessSignIn:
		return "sign_in"
	}
	return "open"
}

func (st Settings) accessMode() accessMode {
	switch {
	case st.APIKey == "":
		return accessOpen
	case st.Access.RequireSignIn:
		return accessSignIn
	}
	return accessKeyForAPI
}

// sessionLifetime is how long after its last use a session stays valid.
// Zero means it does not expire.
func (st Settings) sessionLifetime() time.Duration {
	if st.Access.SessionDays <= 0 {
		return 0
	}
	return time.Duration(st.Access.SessionDays) * 24 * time.Hour
}

// ---------- Key-for-API mode: the rules from before sign-in existed ----------

// keyGuardedRoute reports whether path is one of the routes the API key
// guarded before sign-in existed. The list is frozen: it is what installs
// that have not turned sign-in on were promised.
func keyGuardedRoute(path string) bool {
	switch path {
	case "/api/v1/status",
		"/api/v1/snapshot/latest",
		"/api/v1/snapshots",
		"/api/v1/scan",
		"/api/v1/report",
		"/api/v1/speedtest/run",
		"/api/v1/backup-monitor/duplicacy/test":
		return true
	}
	for _, prefix := range []string{
		"/api/v1/snapshot/",
		"/api/v1/speedtest/stream/",
		"/api/v1/speedtest/cancel/",
		"/api/v1/speedtest/samples/",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// requestHasKey reports whether r presents the API key itself (not a
// browser session).
func requestHasKey(r *http.Request, apiKey string) bool {
	if apiKey == "" {
		return false
	}
	if tok := bearerToken(r); tok != "" && secureEqual(tok, apiKey) {
		return true
	}
	if q := r.URL.Query().Get("api_key"); q != "" && secureEqual(q, apiKey) {
		return true
	}
	return false
}

// samePageExemption is the rule that let the web UI call key-guarded
// routes before sign-in existed: a request whose Referer names this host.
// Any client can send that header, which is why sign-in replaces it; it is
// kept, unchanged, for installs that have not turned sign-in on, because
// their dashboard depends on it.
func samePageExemption(r *http.Request) bool {
	referer := r.Header.Get("Referer")
	return referer != "" && (strings.Contains(referer, r.Host) || strings.Contains(referer, "localhost"))
}

// ---------- Who is connecting ----------

type peerAddrKey struct{}

// capturePeerAddr remembers the address of the socket the request arrived
// on. It must run before chi's RealIP, which replaces RemoteAddr with
// whatever a forwarding header claims.
func capturePeerAddr(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), peerAddrKey{}, r.RemoteAddr)))
	})
}

func peerAddr(r *http.Request) string {
	if v, ok := r.Context().Value(peerAddrKey{}).(string); ok && v != "" {
		return v
	}
	return r.RemoteAddr
}

// clientView is how a request looks from here: where the connection came
// from, what any proxy in between says about the visitor, and whether that
// adds up to "on the local network".
type clientView struct {
	Address   string   `json:"address"`
	Forwarded []string `json:"forwarded_for,omitempty"`
	// Host is the name NAS Doctor was opened under.
	Host  string `json:"host,omitempty"`
	Local bool   `json:"local"`
	// Reason is a short code the Settings page turns into a sentence:
	// private, loopback, public, gateway, forwarded_public,
	// forwarded_unknown, proxy, http10, host, unknown.
	Reason string `json:"reason"`
}

// privateOnlySuffixes are name endings that cannot be registered in the
// public DNS, so a request addressed to one was not steered here by an
// outside site.
var privateOnlySuffixes = []string{
	".localhost", ".local", ".lan", ".home", ".home.arpa", ".internal",
	".localdomain", ".intranet", ".private", ".corp",
}

// normalizeHost lower-cases a Host header value and strips the port.
func normalizeHost(hostport string) string {
	h := strings.ToLower(strings.TrimSpace(hostport))
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(h, "["), "]"), ".")
}

// hostIsOurs reports whether host is a name this instance is known to be
// reached by: a private address, a name that only exists on private
// networks, or one the user listed.
//
// This is what stops DNS rebinding. A hostile page can point its own
// domain at the NAS and have a visitor's browser make requests that arrive
// from a local address, carrying any header the page likes. It cannot
// change the Host its browser sends, and that Host is its own domain.
func hostIsOurs(host string, listed []string) bool {
	h := normalizeHost(host)
	if h == "" {
		return false
	}
	if a, ok := parseAddr(h); ok {
		return a.IsLoopback() || isPrivateAddr(a)
	}
	if h == "localhost" || !strings.Contains(h, ".") {
		return true
	}
	for _, suffix := range privateOnlySuffixes {
		if strings.HasSuffix(h, suffix) {
			return true
		}
	}
	for _, l := range listed {
		if normalizeHost(l) == h {
			return true
		}
	}
	return false
}

// normalizeLocalHosts validates and tidies AccessSettings.LocalHosts.
func normalizeLocalHosts(in []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		// Checked before the port is stripped: "http://x/y" would
		// otherwise be read as host "http".
		if strings.ContainsAny(raw, " /\\@?#\"'<>") || hasControlChars(raw) || len(raw) > 260 {
			return nil, fmt.Errorf("access.local_hosts: %q is not a host name", raw)
		}
		h := normalizeHost(raw)
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	if len(out) > MaxLocalHosts {
		return nil, fmt.Errorf("access.local_hosts: at most %d names", MaxLocalHosts)
	}
	return out, nil
}

// clientAddressHeaders are the headers a proxy uses to pass on the
// visitor's address. Every address in any of them must be private for a
// request to count as local.
var clientAddressHeaders = []string{
	"X-Forwarded-For", "X-Real-Ip", "Cf-Connecting-Ip", "True-Client-Ip",
	"X-Client-Ip", "Fly-Client-Ip", "X-Original-Forwarded-For",
}

// proxyMarkerHeaders show that a proxy handled the request even when it
// did not pass on the visitor's address. X-Forwarded-Host is checked
// against the names we answer to as well (hostIsOurs).
var proxyMarkerHeaders = []string{
	"Via", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-Port",
	"X-Forwarded-Server", "Cf-Ray", "Cdn-Loop",
}

func isPrivateAddr(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsPrivate() || a.IsLinkLocalUnicast()
}

func parseAddr(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Addr{}, false
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap(), true
	}
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	if i := strings.IndexByte(s, '%'); i >= 0 { // zone
		s = s[:i]
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}

// forwardedClients collects every visitor address a proxy passed on. ok is
// false when a value could not be read as an address ("unknown", an
// obfuscated identifier), in which case the visitor cannot be placed.
func forwardedClients(h http.Header) (addrs []netip.Addr, raw []string, ok bool) {
	ok = true
	add := func(v string) {
		v = strings.Trim(strings.TrimSpace(v), `"`)
		if v == "" {
			return
		}
		raw = append(raw, v)
		if a, good := parseAddr(v); good {
			addrs = append(addrs, a)
		} else {
			ok = false
		}
	}
	for _, name := range clientAddressHeaders {
		for _, line := range h.Values(name) {
			for _, part := range strings.Split(line, ",") {
				add(part)
			}
		}
	}
	// RFC 7239: Forwarded: for=192.0.2.60;proto=http, for="[2001:db8::1]:4711"
	for _, line := range h.Values("Forwarded") {
		for _, hop := range strings.Split(line, ",") {
			for _, pair := range strings.Split(hop, ";") {
				k, v, found := strings.Cut(strings.TrimSpace(pair), "=")
				if found && strings.EqualFold(k, "for") {
					add(v)
				}
			}
		}
	}
	return addrs, raw, ok
}

// describeClient works out whether r comes from the local network.
//
// It is deliberately one-sided. Forwarding headers are never used to make
// a request local, only to rule it out, and anything ambiguous counts as
// not local: the price of a wrong "no" is typing the key once, the price of
// a wrong "yes" is an open door.
func describeClient(r *http.Request, listedHosts []string) clientView {
	view := clientView{Address: peerAddr(r), Host: normalizeHost(r.Host)}
	peer, ok := parseAddr(view.Address)
	if !ok {
		view.Reason = "unknown"
		return view
	}
	view.Address = peer.String()
	forwarded, raw, parsed := forwardedClients(r.Header)
	view.Forwarded = raw

	switch {
	case !peer.IsLoopback() && !isPrivateAddr(peer):
		view.Reason = "public"
		return view
	case !parsed:
		view.Reason = "forwarded_unknown"
		return view
	}
	for _, a := range forwarded {
		if !a.IsLoopback() && !isPrivateAddr(a) {
			view.Reason = "forwarded_public"
			return view
		}
	}
	if len(forwarded) == 0 {
		for _, name := range proxyMarkerHeaders {
			if r.Header.Get(name) != "" {
				// A proxy is in the path and did not say who the
				// visitor is.
				view.Reason = "proxy"
				return view
			}
		}
		// nginx talks HTTP/1.0 to its upstream unless told otherwise,
		// and no browser does. A bare proxy_pass with no forwarding
		// headers is otherwise indistinguishable from a direct visit.
		if r.ProtoMajor == 1 && r.ProtoMinor == 0 {
			view.Reason = "http10"
			return view
		}
	}
	// The default gateway is never a visitor's own address. Seeing it
	// means the real address was rewritten on the way in: Docker's
	// userland proxy, rootless networking, or a router that masquerades
	// forwarded ports. Every visitor then looks the same, wherever they
	// are, and headers from such a peer prove nothing.
	if !peer.IsLoopback() {
		for _, gw := range defaultGateways() {
			if gw == peer {
				view.Reason = "gateway"
				return view
			}
		}
	}
	// Addressed to a name we do not know as ours: see hostIsOurs. Both
	// the Host we received and the one a proxy says the visitor used must
	// pass, since a page can add the second header itself.
	hosts := append([]string{r.Host}, r.Header.Values("X-Forwarded-Host")...)
	for _, hv := range hosts {
		for _, h := range strings.Split(hv, ",") {
			if !hostIsOurs(h, listedHosts) {
				view.Host = normalizeHost(h)
				view.Reason = "host"
				return view
			}
		}
	}
	view.Local = true
	if peer.IsLoopback() && len(forwarded) == 0 {
		view.Reason = "loopback"
	} else {
		view.Reason = "private"
	}
	return view
}

// defaultGateways returns this machine's IPv4 default gateways. It is a
// variable so tests can substitute it.
var defaultGateways = cachedDefaultGateways

var gatewayCache struct {
	sync.Mutex
	at    time.Time
	addrs []netip.Addr
}

func cachedDefaultGateways() []netip.Addr {
	gatewayCache.Lock()
	defer gatewayCache.Unlock()
	if !gatewayCache.at.IsZero() && time.Since(gatewayCache.at) < 5*time.Minute {
		return gatewayCache.addrs
	}
	gatewayCache.at = time.Now()
	gatewayCache.addrs = nil
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return nil // not Linux, or /proc unavailable: no gateway rule
	}
	defer f.Close()
	gatewayCache.addrs = parseDefaultGateways(f)
	return gatewayCache.addrs
}

// parseDefaultGateways reads the kernel routing table (/proc/net/route)
// and returns the gateway of every default route.
func parseDefaultGateways(r io.Reader) []netip.Addr {
	var out []netip.Addr
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		// Iface Destination Gateway Flags ...
		if len(f) < 3 || f[1] != "00000000" {
			continue
		}
		b, err := hex.DecodeString(f[2])
		if err != nil || len(b) != 4 {
			continue
		}
		// The kernel prints the address in host byte order, which is
		// little-endian on every platform NAS Doctor ships for.
		gw := netip.AddrFrom4([4]byte{b[3], b[2], b[1], b[0]})
		if gw.IsUnspecified() {
			continue
		}
		out = append(out, gw)
	}
	return out
}

// ---------- Cross-origin reads ----------

// crossOriginReadable reports whether another site's page may read path
// through a visitor's browser on an install without sign-in.
//
// Every response used to carry Access-Control-Allow-Origin: *, for every
// method. That let any web page the user happened to visit read the
// settings (keys and webhook URLs included) and rewrite them. Reading
// monitoring data stays possible, because a status badge on someone's own
// start page is a legitimate use that predates this change. Configuration
// is not readable, and nothing is writable.
func crossOriginReadable(path string) bool {
	if path == "/metrics" {
		return true
	}
	if !strings.HasPrefix(path, "/api/v1/") {
		return false
	}
	for _, private := range []string{"/api/v1/settings", "/api/v1/fleet", "/api/v1/backup", "/api/v1/auth", "/api/v1/access"} {
		if path == private || strings.HasPrefix(path, private+"/") {
			return false
		}
	}
	return true
}

func isCrossOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return true
	}
	return !sameHost(u.Host, r.Host) && !sameHost(u.Host, r.Header.Get("X-Forwarded-Host"))
}

// crossOriginReads answers cross-origin GETs of monitoring data on
// installs without sign-in. With sign-in on, no CORS headers are sent.
func (s *Server) crossOriginReads(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isCrossOrigin(r) || !crossOriginReadable(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		method := r.Method
		preflight := method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
		if preflight {
			method = r.Header.Get("Access-Control-Request-Method")
		}
		if method != http.MethodGet && method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		settings, err := s.loadSettings()
		if err != nil || settings.accessMode() == accessSignIn {
			next.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Add("Vary", "Origin")
		h.Set("Access-Control-Allow-Origin", "*")
		if preflight {
			h.Set("Access-Control-Allow-Methods", method)
			h.Set("Access-Control-Allow-Headers", "Accept, Content-Type")
			h.Set("Access-Control-Max-Age", "300")
			w.WriteHeader(http.StatusOK)
			return
		}
		h.Set("Access-Control-Expose-Headers", "Link")
		next.ServeHTTP(w, r)
	})
}

// ---------- Who connects without the key ----------

// keylessClient is something other than a person's browser that used this
// instance without presenting the API key. Before sign-in is turned on the
// list answers "what will need the key?"; afterwards it shows what is
// being turned away.
type keylessClient struct {
	Address  string    `json:"address"`
	Agent    string    `json:"agent"`
	Path     string    `json:"path"`
	Embedded bool      `json:"embedded,omitempty"` // loaded inside another site's page
	Local    bool      `json:"local"`
	Refused  bool      `json:"refused"`
	Count    int       `json:"count"`
	LastSeen time.Time `json:"last_seen"`
}

const maxKeylessClients = 50

// keylessTracker keeps a small in-memory list of keylessClients. It is
// not persisted: the list restarts with the process, and Since says how
// far back it reaches.
type keylessTracker struct {
	mu      sync.Mutex
	since   time.Time
	entries map[string]*keylessClient
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// pathGroup shortens a path to its first three segments so one client
// reading many disks or snapshots is one entry.
func pathGroup(p string) string {
	parts := strings.SplitN(strings.TrimPrefix(p, "/"), "/", 4)
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return clip("/"+strings.Join(parts, "/"), 80)
}

// observe records r when it comes from a script, a scraper or an embedded
// frame rather than from someone browsing NAS Doctor directly.
func (t *keylessTracker) observe(r *http.Request, settings Settings, refused bool) {
	agent := strings.TrimSpace(r.Header.Get("User-Agent"))
	embedded := false
	switch r.Header.Get("Sec-Fetch-Dest") {
	case "iframe", "frame", "embed", "object":
		embedded = true
	}
	if !embedded && strings.Contains(agent, "Mozilla/") {
		return // a person's browser; it can sign in
	}
	if embedded {
		agent = "embedded page"
		if u, err := url.Parse(r.Header.Get("Referer")); err == nil && u.Host != "" {
			agent = "embedded in " + u.Host
		}
	} else if agent == "" {
		agent = "unnamed client"
	} else if i := strings.IndexAny(agent, " ("); i > 0 {
		agent = agent[:i] // "Prometheus/2.53.0", "curl/8.5.0"
	}
	agent = clip(agent, 60)
	view := describeClient(r, settings.Access.LocalHosts)
	address := clientIP(r)
	path := pathGroup(r.URL.Path)
	key := address + "|" + agent + "|" + path

	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.entries == nil {
		t.entries = make(map[string]*keylessClient)
		if t.since.IsZero() {
			t.since = now
		}
	}
	e := t.entries[key]
	if e == nil {
		if len(t.entries) >= maxKeylessClients {
			var oldest string
			for k, v := range t.entries {
				if oldest == "" || v.LastSeen.Before(t.entries[oldest].LastSeen) {
					oldest = k
				}
			}
			delete(t.entries, oldest)
		}
		e = &keylessClient{Address: clip(address, 64), Agent: agent, Path: path, Embedded: embedded}
		t.entries[key] = e
	}
	e.Count++
	e.LastSeen = now
	e.Local = view.Local
	e.Refused = refused
}

func (t *keylessTracker) list() (since time.Time, out []keylessClient) {
	t.mu.Lock()
	defer t.mu.Unlock()
	out = make([]keylessClient, 0, len(t.entries))
	for _, e := range t.entries {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	return t.since, out
}

// ---------- Access status ----------

type accessStatus struct {
	Mode              string          `json:"mode"`
	KeySet            bool            `json:"key_set"`
	RequireSignIn     bool            `json:"require_sign_in"`
	SessionDays       int             `json:"session_days"`
	TrustLocalNetwork bool            `json:"trust_local_network"`
	LocalHosts        []string        `json:"local_hosts,omitempty"`
	SignedInVia       string          `json:"signed_in_via,omitempty"`
	Client            clientView      `json:"client"`
	KeylessClients    []keylessClient `json:"keyless_clients"`
	TrackingSince     time.Time       `json:"tracking_since"`
}

// handleAccessStatus describes how this instance is protected and how the
// caller looks from here. The Settings page uses it for the Access card.
// GET /api/v1/access
func (s *Server) handleAccessStatus(w http.ResponseWriter, r *http.Request) {
	settings, err := s.loadSettings()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "settings unavailable"})
		return
	}
	since, clients := s.keyless.list()
	if since.IsZero() {
		since = s.startTime
	}
	st := accessStatus{
		Mode:              settings.accessMode().String(),
		KeySet:            settings.APIKey != "",
		RequireSignIn:     settings.Access.RequireSignIn,
		SessionDays:       settings.Access.SessionDays,
		TrustLocalNetwork: settings.Access.TrustLocalNetwork,
		LocalHosts:        settings.Access.LocalHosts,
		Client:            describeClient(r, settings.Access.LocalHosts),
		KeylessClients:    clients,
		TrackingSince:     since,
	}
	if settings.accessMode() == accessSignIn {
		st.SignedInVia = signedInVia(r, settings)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, st)
}

// handleDismissAccessNotice hides the dashboard notice about sign-in.
// POST /api/v1/access/dismiss-notice
func (s *Server) handleDismissAccessNotice(w http.ResponseWriter, r *http.Request) {
	if _, err := s.updateSettings(func(st *Settings) error {
		st.Access.NoticeDismissed = true
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "dismissed"})
}

// accessNoticeDue reports whether the dashboard should tell the user that
// sign-in exists: only on an install whose key guards the API alone, and
// only until dismissed.
func accessNoticeDue(st Settings) bool {
	return st.accessMode() == accessKeyForAPI && !st.Access.NoticeDismissed
}

// ShowAPIKey returns the stored API key. It backs the -show-api-key
// command-line flag: whoever controls the host can already read the
// database, and reading the key back is kinder than resetting it.
func ShowAPIKey(store storage.ConfigStore) (string, error) {
	settings, _, err := LoadSettings(store, 0)
	if err != nil {
		return "", err
	}
	return settings.APIKey, nil
}
