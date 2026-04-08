package api

import "net/http"

// SharedCSS is the unified design system served at /css/shared.css.
// All subpages (alerts, settings, stats, fleet, disk_detail) link to this
// instead of duplicating theme tokens and component styles.
const SharedCSS = `
/* ============================================================
   NAS Doctor — Shared Design System
   Single source of truth for tokens, typography, and components.
   Served at /css/shared.css
   ============================================================ */

/* ── Reset ──────────────────────────────────────────────────── */
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}

/* ── Design Tokens ──────────────────────────────────────────── */

/* Midnight (default) */
:root, body.theme-midnight {
  --bg:#0f1011; --surface:#191a1b; --elevated:#242526;
  --text:#f7f8f8; --text2:#8a8f98; --text3:#5a5f6a;
  --accent:#5e6ad2; --accent-hover:#7170ff;
  --green:#27a644; --amber:#d97706; --red:#dc2626;
  --border:rgba(255,255,255,0.08); --border-hover:rgba(255,255,255,0.15);
  --radius:8px;
  --input-bg:rgba(255,255,255,0.04);
  --card-bg:var(--surface); --card-border:var(--border); --card-shadow:none;
  --btn-hover-bg:rgba(255,255,255,0.06);
  --font-sans:'Inter',system-ui,-apple-system,sans-serif;
  --font-mono:'SF Mono',ui-monospace,monospace;
  --font-serif:'Inter',system-ui,sans-serif;
  --disabled-opacity:0.4;
}

/* Clean */
body.theme-clean {
  --bg:#ffffff; --surface:#ffffff; --elevated:#fafafa;
  --text:#171717; --text2:#808080; --text3:#b3b3b3;
  --accent:#171717; --accent-hover:#404040;
  --green:#16a34a; --amber:#d97706; --red:#dc2626;
  --border:rgba(0,0,0,0.08); --border-hover:rgba(0,0,0,0.2);
  --radius:8px;
  --input-bg:rgba(0,0,0,0.03);
  --card-bg:#ffffff; --card-border:rgba(0,0,0,0.08); --card-shadow:0 1px 3px rgba(0,0,0,0.04);
  --btn-hover-bg:rgba(0,0,0,0.04);
  --font-sans:'Inter',-apple-system,BlinkMacSystemFont,sans-serif;
  --font-mono:'SF Mono',ui-monospace,monospace;
  --font-serif:'Inter',-apple-system,sans-serif;
  --disabled-opacity:0.5;
}

/* Ember */
body.theme-ember {
  --bg:#07080a; --surface:#101111; --elevated:#1a1b1c;
  --text:#f9f9f9; --text2:#9c9c9d; --text3:#5a5b5c;
  --accent:#55b3ff; --accent-hover:#7ec8ff;
  --green:#5fc992; --amber:#FACC15; --red:#FF6363;
  --border:rgba(255,255,255,0.06); --border-hover:rgba(255,255,255,0.12);
  --radius:8px;
  --input-bg:rgba(255,255,255,0.04);
  --card-bg:var(--surface); --card-border:var(--border); --card-shadow:none;
  --btn-hover-bg:rgba(255,255,255,0.06);
  --font-sans:'Inter',-apple-system,BlinkMacSystemFont,sans-serif;
  --font-mono:'JetBrains Mono',ui-monospace,'SF Mono',monospace;
  --font-serif:'Literata','Georgia',serif;
  --disabled-opacity:0.5;
}

/* ── Base ───────────────────────────────────────────────────── */
html {
  background:var(--bg); color:var(--text);
  font-family:var(--font-sans); font-size:14px; line-height:1.5;
  -webkit-font-smoothing:antialiased;
}
body {
  min-height:100vh; padding:24px;
  transition:background 0.2s ease,color 0.2s ease;
}
a { color:var(--accent); text-decoration:none }
a:hover { color:var(--accent-hover) }

/* ── Typography — Ember overrides ───────────────────────────── */
body.theme-ember .card-title,
body.theme-ember .page-title,
body.theme-ember .header-title,
body.theme-ember .summary-val,
body.theme-ember .drive-name,
body.theme-ember .drive-title,
body.theme-ember .card-name,
body.theme-ember .empty-title,
body.theme-ember .section-title { font-family:var(--font-serif) }

body.theme-ember .log-table td,
body.theme-ember .status-badge,
body.theme-ember .badge,
body.theme-ember .detail-value,
body.theme-ember .mono,
body.theme-ember .stat-val,
body.theme-ember .card-hostname,
body.theme-ember .attr-raw,
body.theme-ember .attr-value,
body.theme-ember code { font-family:var(--font-mono) }

/* ── Layout ─────────────────────────────────────────────────── */
.container { max-width:1100px; margin:0 auto }

/* ── Header ─────────────────────────────────────────────────── */
.header {
  display:flex; align-items:center; justify-content:space-between;
  padding:16px 0; margin-bottom:24px; border-bottom:1px solid var(--border);
}
.header-left { display:flex; align-items:center; gap:16px }
.logo {
  display:flex; align-items:center; gap:8px;
  font-size:20px; font-weight:600; letter-spacing:-0.5px; color:var(--text);
}
.logo img { width:24px; height:24px; border-radius:4px }
.page-title { font-size:20px; font-weight:600; color:var(--text2) }
.header-sub { font-size:13px; color:var(--text2) }

/* ── Navigation ─────────────────────────────────────────────── */
.nav-links { display:flex; gap:4px; align-items:center }
.nav-link {
  padding:6px 12px; border-radius:var(--radius);
  font-size:12px; font-weight:500; color:var(--text2);
  border:1px solid transparent; transition:all 0.15s ease;
  text-decoration:none; display:inline-flex; align-items:center;
}
.nav-link:hover {
  color:var(--text); background:var(--btn-hover-bg);
  border-color:var(--border);
}
.nav-link.active {
  color:var(--text); background:var(--btn-hover-bg);
  border-color:var(--border);
}

/* ── Cards ──────────────────────────────────────────────────── */
.card {
  background:var(--card-bg); border:1px solid var(--card-border);
  box-shadow:var(--card-shadow);
  border-radius:12px; padding:24px; margin-bottom:20px;
  transition:background 0.2s ease, border-color 0.2s ease;
  position:relative;
}
.card-header {
  display:flex; align-items:center; justify-content:space-between;
  margin-bottom:4px;
}
.card-title { font-size:16px; font-weight:600 }
.card-desc { font-size:13px; color:var(--text2); margin-bottom:20px }

/* ── Buttons ────────────────────────────────────────────────── */
.btn {
  display:inline-flex; align-items:center; gap:6px;
  padding:8px 16px; border-radius:var(--radius);
  font-size:13px; font-weight:600; font-family:inherit;
  cursor:pointer; border:1px solid transparent;
  transition:all 0.15s; text-decoration:none;
}
.btn-primary { background:var(--accent); color:#fff; border-color:var(--accent) }
.btn-primary:hover { background:var(--accent-hover) }
.btn-secondary {
  background:var(--btn-hover-bg); color:var(--text2);
  border-color:var(--border);
}
.btn-secondary:hover {
  color:var(--text); border-color:var(--border-hover);
  background:var(--btn-hover-bg);
}
.btn-danger {
  background:rgba(220,38,38,0.1); color:var(--red);
  border-color:rgba(220,38,38,0.2);
}
.btn-danger:hover { background:rgba(220,38,38,0.18) }
.btn-success {
  background:rgba(39,166,68,0.1); color:var(--green);
  border-color:rgba(39,166,68,0.2);
}
.btn:disabled { opacity:var(--disabled-opacity); cursor:not-allowed }
.btn-sm { padding:5px 10px; font-size:12px }

/* ── Forms ──────────────────────────────────────────────────── */
label {
  display:block; font-size:12px; font-weight:600; color:var(--text2);
  text-transform:uppercase; letter-spacing:0.5px; margin-bottom:6px;
}
select,
input[type="text"],
input[type="url"],
input[type="password"],
input[type="number"] {
  width:100%; padding:8px 12px;
  background:var(--input-bg); border:1px solid var(--border);
  border-radius:var(--radius); color:var(--text);
  font-size:14px; font-family:inherit; outline:none;
  transition:border 0.15s;
}
select:focus,input:focus { border-color:var(--accent) }
select {
  cursor:pointer; -webkit-appearance:none; appearance:none;
  background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' fill='%238a8f98'%3E%3Cpath d='M6 8.5L1 3.5h10z'/%3E%3C/svg%3E");
  background-repeat:no-repeat; background-position:right 10px center;
}
input:disabled,select:disabled { opacity:var(--disabled-opacity); cursor:not-allowed }
.form-row { display:grid; grid-template-columns:1fr 1fr; gap:16px; margin-bottom:16px }
.form-group { margin-bottom:16px }

/* ── Toggle ─────────────────────────────────────────────────── */
.toggle-wrap { display:flex; align-items:center; gap:10px; cursor:pointer }
.toggle {
  position:relative; width:40px; height:22px;
  background:rgba(255,255,255,0.1); border-radius:11px;
  transition:background 0.2s; flex-shrink:0;
}
.toggle.on { background:var(--accent) }
.toggle-knob {
  position:absolute; top:2px; left:2px; width:18px; height:18px;
  background:#fff; border-radius:50%; transition:left 0.2s;
}
.toggle.on .toggle-knob { left:20px }
.toggle-label { font-size:13px; color:var(--text2) }
body.theme-clean .toggle { background:rgba(0,0,0,0.1) }
body.theme-clean .toggle.on { background:var(--accent) }

/* ── Tables ─────────────────────────────────────────────────── */
.log-table { width:100%; border-collapse:collapse }
.log-table th {
  text-align:left; font-size:11px; font-weight:600; color:var(--text2);
  text-transform:uppercase; letter-spacing:0.5px;
  padding:8px 12px; border-bottom:1px solid var(--border);
}
.log-table td {
  padding:8px 12px; font-size:13px; color:var(--text);
  border-bottom:1px solid rgba(255,255,255,0.04); white-space:nowrap;
}
.log-table tr:hover td { background:var(--btn-hover-bg) }
.log-empty { text-align:center; padding:32px; color:var(--text2); font-size:13px }
.overflow-x { overflow-x:auto }

body.theme-clean .log-table th { border-bottom-color:rgba(0,0,0,0.08) }
body.theme-clean .log-table td { border-bottom-color:rgba(0,0,0,0.04) }
body.theme-clean .log-table tr:hover td { background:rgba(0,0,0,0.02) }

/* ── Status Badges ──────────────────────────────────────────── */
.status-badge { font-size:11px; font-weight:600; padding:2px 8px; border-radius:4px }
.status-success { color:var(--green); background:rgba(39,166,68,0.12) }
.status-failed  { color:var(--red);   background:rgba(220,38,38,0.12) }
.status-up      { color:var(--green); background:rgba(39,166,68,0.12) }
.status-down    { color:var(--red);   background:rgba(220,38,38,0.12) }
.status-open    { color:var(--text);  background:rgba(255,255,255,0.08) }
.status-acknowledged { color:#f59e0b; background:rgba(245,158,11,0.12) }
.status-snoozed { color:#3b82f6; background:rgba(59,130,246,0.12) }
.status-resolved { color:var(--green); background:rgba(39,166,68,0.12) }

body.theme-clean .status-open { background:rgba(0,0,0,0.06) }

/* ── Badges ─────────────────────────────────────────────────── */
.badge {
  display:inline-block; font-size:10px; font-weight:700;
  text-transform:uppercase; letter-spacing:0.5px;
  padding:2px 8px; border-radius:4px; margin-left:8px;
}
.badge-discord { background:rgba(88,101,242,0.15); color:#5865f2 }
.badge-slack   { background:rgba(74,21,75,0.15);   color:#e01e5a }
.badge-gotify  { background:rgba(16,185,129,0.15); color:#10b981 }
.badge-ntfy    { background:rgba(59,130,246,0.15); color:#3b82f6 }
.badge-generic { background:rgba(255,255,255,0.08); color:var(--text2) }
body.theme-clean .badge-generic { background:rgba(0,0,0,0.06) }

/* ── Pills (service check types) ────────────────────────────── */
.pill {
  display:inline-block; font-size:10px; font-weight:700;
  text-transform:uppercase; letter-spacing:0.5px;
  padding:2px 8px; border-radius:999px;
}
.pill-http { background:rgba(59,130,246,0.14); color:#60a5fa }
.pill-tcp  { background:rgba(16,185,129,0.14); color:#34d399 }
.pill-dns  { background:rgba(236,72,153,0.14); color:#f472b6 }
.pill-smb,.pill-nfs { background:rgba(245,158,11,0.14); color:#fbbf24 }

/* ── Webhook Items ──────────────────────────────────────────── */
.webhook-item {
  display:flex; align-items:center; gap:12px;
  padding:12px 16px; border:1px solid var(--border);
  border-radius:var(--radius); margin-bottom:8px;
  background:var(--input-bg); transition:background 0.15s;
}
.webhook-item:hover { background:var(--btn-hover-bg) }
.webhook-info { flex:1; min-width:0 }
.webhook-name { font-size:14px; font-weight:600; color:var(--text) }
.webhook-url {
  font-size:12px; color:var(--text2);
  overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:300px;
}
.webhook-actions { display:flex; align-items:center; gap:8px; flex-shrink:0 }
.webhook-form {
  border:1px solid var(--accent); border-radius:var(--radius);
  padding:20px; margin-top:12px;
  background:rgba(94,106,210,0.04); display:none;
}
.webhook-form.visible { display:block }
.webhook-form-actions { display:flex; gap:8px; margin-top:16px }

/* ── Mini List Items ────────────────────────────────────────── */
.mini-list-item {
  display:flex; align-items:center; gap:10px;
  padding:10px 12px; border:1px solid var(--border);
  border-radius:var(--radius); margin-bottom:8px; background:var(--input-bg);
}
.mini-list-main { flex:1; min-width:0 }
.mini-list-title { font-size:13px; font-weight:600; color:var(--text) }
.mini-list-sub {
  font-size:12px; color:var(--text2);
  overflow:hidden; text-overflow:ellipsis; white-space:nowrap;
}
.inline-muted { font-size:11px; color:var(--text2) }

/* ── Chart Card ─────────────────────────────────────────────── */
.chart-card {
  background:var(--input-bg); border:1px solid var(--border);
  border-radius:var(--radius); padding:10px;
}
.chart-label { font-size:12px; color:var(--text2); margin-bottom:6px }

/* ── Toasts ─────────────────────────────────────────────────── */
.toast-container {
  position:fixed; top:20px; right:20px; z-index:9999;
  display:flex; flex-direction:column; gap:8px;
}
.toast {
  padding:10px 18px; border-radius:var(--radius);
  font-size:13px; font-weight:500; color:#fff;
  animation:toast-in 0.25s ease; pointer-events:none;
}
.toast-success { background:var(--green) }
.toast-error   { background:var(--red) }
.toast-info    { background:var(--accent) }
@keyframes toast-in { from{opacity:0;transform:translateY(-8px)} to{opacity:1;transform:translateY(0)} }
@keyframes toast-out { from{opacity:1} to{opacity:0;transform:translateY(-8px)} }

/* ── Coming Soon ────────────────────────────────────────────── */
.coming-soon { position:relative }
.coming-soon::after {
  content:"Coming Soon"; position:absolute; top:12px; right:12px;
  font-size:11px; font-weight:700; text-transform:uppercase; letter-spacing:0.5px;
  color:var(--accent); background:rgba(94,106,210,0.12);
  padding:3px 10px; border-radius:4px;
}
.coming-soon .form-group { opacity:0.4; pointer-events:none }
body.theme-ember .coming-soon::after { background:rgba(85,179,255,0.12) }

/* ── Sticky Save Bar ────────────────────────────────────────── */
.save-bar {
  position:fixed; bottom:0; left:0; right:0; z-index:100;
  background:var(--surface); border-top:1px solid var(--border);
  padding:12px 24px; transform:translateY(100%);
  transition:transform 0.25s ease; box-shadow:0 -4px 20px rgba(0,0,0,0.3);
}
.save-bar.visible { transform:translateY(0) }
.save-bar-inner {
  max-width:1100px; margin:0 auto;
  display:flex; align-items:center; justify-content:space-between;
}
.save-bar-text { font-size:13px; color:var(--text2) }

/* ── Drag & Drop ────────────────────────────────────────────── */
.card.dragging { opacity:0.5; transform:scale(0.98) }
.card-controls { display:flex; align-items:center; gap:6px }
.card.collapsed .card-body { display:none }
.card.collapsed .card-desc { display:none }
.card.collapsed { padding-bottom:16px }
.drag-handle {
  cursor:grab; padding:4px; color:var(--text2); opacity:0.5;
  transition:opacity 0.15s; display:flex; align-items:center;
}
.drag-handle:hover { opacity:1 }
.drag-handle:active { cursor:grabbing }
.drag-handle svg { pointer-events:none }
.collapse-btn {
  cursor:pointer; padding:4px; color:var(--text2); opacity:0.5;
  transition:all 0.15s; display:flex; align-items:center;
  background:none; border:none;
}
.collapse-btn:hover { opacity:1 }
.collapse-btn svg { transition:transform 0.2s ease; pointer-events:none }
.card.collapsed .collapse-btn svg { transform:rotate(-90deg) }
.drag-placeholder {
  border:2px dashed var(--accent); border-radius:12px;
  margin-bottom:20px; opacity:0.4; background:rgba(94,106,210,0.04);
}

/* ── Customize Panel ────────────────────────────────────────── */
.customize-btn {
  padding:6px 14px; border-radius:var(--radius);
  font-size:12px; font-weight:500; color:var(--text2);
  border:1px solid var(--border); background:transparent;
  cursor:pointer; transition:all 0.15s;
  display:inline-flex; align-items:center; gap:6px;
}
.customize-btn:hover { color:var(--text); border-color:var(--border-hover); background:var(--btn-hover-bg) }
.customize-overlay { position:fixed; top:0; left:0; right:0; bottom:0; z-index:99; display:none }
.customize-overlay.visible { display:block }
.customize-panel {
  position:fixed; top:50%; left:50%; transform:translate(-50%,-50%); z-index:100;
  background:var(--surface); border:1px solid var(--border); border-radius:12px;
  padding:24px; width:380px; max-height:80vh; overflow-y:auto;
  box-shadow:0 20px 60px rgba(0,0,0,0.4); display:none;
}
.customize-panel.visible { display:block }
.customize-panel-title { font-size:16px; font-weight:600; margin-bottom:4px }
.customize-panel-desc { font-size:12px; color:var(--text2); margin-bottom:16px }
.customize-item {
  display:flex; align-items:center; gap:10px;
  padding:10px 8px; border-bottom:1px solid rgba(255,255,255,0.04);
  cursor:grab; user-select:none;
}
.customize-item:last-child { border-bottom:none }
.customize-item:hover { background:var(--btn-hover-bg); border-radius:var(--radius) }
.customize-item .grip { color:var(--text2); opacity:0.4; flex-shrink:0 }
.customize-item .item-name { flex:1; font-size:13px; font-weight:500 }
.customize-item .toggle { transform:scale(0.85) }

body.theme-clean .customize-panel { box-shadow:0 8px 24px rgba(0,0,0,0.12) }
body.theme-clean .customize-item { border-bottom-color:rgba(0,0,0,0.06) }
body.theme-clean .customize-item:hover { background:rgba(0,0,0,0.02) }

/* ── Summary Bar ────────────────────────────────────────────── */
.summary-bar { display:flex; gap:12px; flex-wrap:wrap; margin-bottom:20px }
.summary-stat {
  display:flex; align-items:center; gap:8px;
  padding:10px 16px; background:var(--surface);
  border:1px solid var(--border); border-radius:var(--radius);
  font-size:13px; font-weight:500; min-width:100px;
}
.summary-stat .stat-count { font-size:20px; font-weight:700 }
.summary-stat.critical .stat-count { color:var(--red) }
.summary-stat.warning .stat-count  { color:var(--amber) }
.summary-stat.healthy .stat-count  { color:var(--green) }
.summary-stat.info .stat-count     { color:var(--accent) }

/* ── Responsive ─────────────────────────────────────────────── */
/* ── Sort Bar ───────────────────────────────────────────────── */
.sort-bar {
  display:inline-flex; gap:1px; padding:2px; border-radius:6px;
  background:var(--input-bg); border:1px solid var(--border);
  opacity:0; transition:opacity 0.2s ease;
}
.section-block:hover .sort-bar { opacity:1 }
.sort-pill {
  padding:4px 10px; border:none; border-radius:4px;
  font-size:10px; font-weight:600; font-family:inherit;
  color:var(--text3); background:transparent; cursor:pointer;
  transition:all 0.12s; text-transform:uppercase; letter-spacing:0.3px;
}
.sort-pill:hover { color:var(--text2); background:var(--btn-hover-bg) }
.sort-pill.active { color:var(--text); background:var(--surface); box-shadow:0 1px 3px rgba(0,0,0,0.15) }
body.theme-clean .sort-pill.active { box-shadow:0 1px 2px rgba(0,0,0,0.06) }
body.theme-clean .sort-pill { color:#b3b3b3 }
body.theme-clean .sort-pill:hover { color:#808080 }
body.theme-clean .sort-pill.active { color:#171717 }

/* ── Swipe-to-dismiss (findings) ────────────────────────────── */
.swipe-dismiss-bg {
  position:absolute; top:0; right:0; bottom:0; width:100%;
  display:flex; align-items:center; justify-content:flex-end; padding-right:20px;
  background:rgba(220,38,38,0.12); color:var(--red);
  font-size:11px; font-weight:600; text-transform:uppercase; letter-spacing:0.5px;
  opacity:0; pointer-events:none; z-index:-1; border-radius:inherit;
}

/* ── Drive Health Summary ───────────────────────────────────── */
.health-summary {
  display:inline-flex; gap:10px; align-items:center;
  font-size:11px; color:var(--text2); margin-left:12px;
}
.health-summary span { display:flex; align-items:center; gap:3px }
.health-summary .dot { width:6px; height:6px; border-radius:50%; flex-shrink:0 }
.health-summary .dot-ok { background:var(--green) }
.health-summary .dot-warn { background:var(--amber) }
.health-summary .dot-crit { background:var(--red) }

@media(max-width:768px) {
  .form-row { grid-template-columns:1fr }
  .header { flex-direction:column; gap:12px; align-items:flex-start }
  .nav-links { flex-wrap:wrap }
  .summary-bar { flex-direction:column }
  .sort-bar { flex-wrap:wrap }
}
`

func serveSharedCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write([]byte(SharedCSS))
}
