package api

// DashboardClean is the light minimal dashboard theme.
var DashboardClean = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>NAS Doctor — Clean</title>
<link rel="icon" type="image/png" href="/icon.png">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { margin: 0; padding: 0; box-sizing: border-box; }

html {
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  font-size: 16px;
  line-height: 1.5;
  color: #171717;
  background: #ffffff;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

body {
  min-height: 100vh;
  background: #ffffff;
}

/* ---- Layout ---- */
.wrapper {
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 24px 80px;
}

/* ---- Header ---- */
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 24px;
  background: #ffffff;
  box-shadow: 0px 0px 0px 1px rgba(0,0,0,0.08);
  position: sticky;
  top: 0;
  z-index: 100;
}

.header-brand {
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.32px;
  color: #171717;
  text-decoration: none;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.theme-switcher {
  display: flex;
  align-items: center;
  gap: 2px;
  background: #f5f5f5;
  border-radius: 8px;
  padding: 2px;
}

.theme-switcher a {
  padding: 4px 10px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 500;
  color: #999999;
  text-decoration: none;
  transition: all 0.15s;
  line-height: 1.4;
}

.theme-switcher a:hover {
  color: #666666;
}

.theme-switcher a.active {
  color: #171717;
  background: #ffffff;
  box-shadow: 0 1px 3px rgba(0,0,0,0.08);
}

.nav-links {
  display: flex;
  gap: 6px;
}

.nav-link {
  font-size: 13px;
  font-weight: 500;
  color: #666666;
  text-decoration: none;
  padding: 4px 10px;
  border-radius: 6px;
  transition: color 0.15s;
}

.nav-link:hover {
  color: #171717;
}

/* ---- Compact top bar (health + stats) ---- */
.top-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 20px;
  margin-top: 20px;
  background: #ffffff;
  border-radius: 10px;
  box-shadow: 0px 0px 0px 1px rgba(0,0,0,0.08);
  min-height: 52px;
  max-height: 60px;
  gap: 16px;
  flex-wrap: nowrap;
  overflow: hidden;
}

.top-bar-left {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.top-bar-divider {
  width: 1px;
  height: 24px;
  background: rgba(0,0,0,0.1);
  flex-shrink: 0;
}

.top-bar-right {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 400;
  color: #4d4d4d;
  flex-shrink: 0;
  white-space: nowrap;
}

.top-bar-right .stat-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.top-bar-right .stat-label {
  color: #808080;
  font-size: 12px;
  font-weight: 500;
}

.top-bar-right .stat-val {
  color: #171717;
  font-size: 13px;
  font-weight: 600;
}

.top-bar-right .dot-sep {
  color: #d4d4d4;
  margin: 0 4px;
  font-size: 10px;
}

/* ---- Pills / Badges ---- */
.pill {
  display: inline-flex;
  align-items: center;
  padding: 3px 10px;
  border-radius: 9999px;
  font-size: 12px;
  font-weight: 600;
  line-height: 1.4;
  white-space: nowrap;
}

.pill-healthy {
  background: #f0fdf4;
  color: #16a34a;
}

.pill-warning {
  background: #fffbeb;
  color: #d97706;
}

.pill-critical {
  background: #fef2f2;
  color: #dc2626;
}

.pill-info {
  background: #ebf5ff;
  color: #0068d6;
}

.pill-neutral {
  background: #fafafa;
  color: #4d4d4d;
}

/* ---- Scan bar ---- */
.scan-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  margin-top: 16px;
  padding: 8px 0;
}

.scan-info {
  font-size: 13px;
  color: #808080;
  font-weight: 400;
}

/* ---- Two-column grid ---- */
.two-col {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
  margin-top: 24px;
}

.col-left {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.col-left > .section {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

@media (max-width: 900px) {
  .two-col {
    grid-template-columns: 1fr;
  }
  .top-bar {
    flex-wrap: wrap;
    max-height: none;
  }
  .col-left > .section {
    max-height: 600px;
  }
}

/* ---- Section spacing ---- */
.section {
  margin-top: 28px;
}

.section:first-child {
  margin-top: 0;
}

.section-title {
  font-size: 12px;
  font-weight: 500;
  color: #808080;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 12px;
}

/* ---- Cards ---- */
.card {
  background: #ffffff;
  border-radius: 8px;
  box-shadow: 0px 0px 0px 1px rgba(0,0,0,0.08);
  padding: 16px;
}

/* ---- Findings ---- */
.findings-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  scrollbar-width: thin;
  scrollbar-color: rgba(0,0,0,0.15) transparent;
}
.findings-list::-webkit-scrollbar { width: 5px; }
.findings-list::-webkit-scrollbar-track { background: transparent; }
.findings-list::-webkit-scrollbar-thumb { background: rgba(0,0,0,0.15); border-radius: 3px; }
.findings-list::-webkit-scrollbar-thumb:hover { background: rgba(0,0,0,0.25); }

.finding-card {
  position: relative;
  background: #ffffff;
  border-radius: 8px;
  border: none;
  padding: 14px 16px;
  transition: box-shadow 0.2s ease;
  box-shadow: 0px 0px 0px 1px rgba(0,0,0,0.08);
}

/* Corner bracket base (all 4 corners) */
.finding-card::before,
.finding-card::after,
.finding-card .corner-bl,
.finding-card .corner-br {
  content: '';
  position: absolute;
  width: 7px;
  height: 7px;
  border-radius: 1.5px;
  background: #fafafa;
  border: 1px solid #ebebeb;
  transition: border-style 0.2s ease;
  z-index: 1;
}
.finding-card::before { top: -3px; left: -3px; }
.finding-card::after  { top: -3px; right: -3px; }
.finding-card .corner-bl { bottom: -3px; left: -3px; }
.finding-card .corner-br { bottom: -3px; right: -3px; }

/* Hover: dashed corners */
.finding-card:hover::before,
.finding-card:hover::after,
.finding-card:hover .corner-bl,
.finding-card:hover .corner-br {
  border-style: dashed;
}

/* ---- Severity: Critical ---- */
.finding-card.severity-critical {
  box-shadow: 0px 0px 0px 1px rgba(220,38,38,0.2);
}
.finding-card.severity-critical:hover {
  box-shadow: 0px 0px 0px 1px rgba(220,38,38,0.3), 0px 4px 12px rgba(220,38,38,0.06);
}
.finding-card.severity-critical::before,
.finding-card.severity-critical::after,
.finding-card.severity-critical .corner-bl,
.finding-card.severity-critical .corner-br {
  border-color: #dc2626;
}

/* ---- Severity: Warning ---- */
.finding-card.severity-warning {
  box-shadow: 0px 0px 0px 1px rgba(217,119,6,0.2);
}
.finding-card.severity-warning:hover {
  box-shadow: 0px 0px 0px 1px rgba(217,119,6,0.3), 0px 4px 12px rgba(217,119,6,0.06);
}
.finding-card.severity-warning::before,
.finding-card.severity-warning::after,
.finding-card.severity-warning .corner-bl,
.finding-card.severity-warning .corner-br {
  border-color: #d97706;
}

/* ---- Severity: Info ---- */
.finding-card.severity-info {
  box-shadow: 0px 0px 0px 1px rgba(0,114,245,0.2);
}
.finding-card.severity-info:hover {
  box-shadow: 0px 0px 0px 1px rgba(0,114,245,0.3), 0px 4px 12px rgba(0,114,245,0.06);
}
.finding-card.severity-info::before,
.finding-card.severity-info::after,
.finding-card.severity-info .corner-bl,
.finding-card.severity-info .corner-br {
  border-color: #0072f5;
}

/* ---- Severity: OK ---- */
.finding-card.severity-ok {
  box-shadow: 0px 0px 0px 1px rgba(22,163,74,0.2);
}
.finding-card.severity-ok:hover {
  box-shadow: 0px 0px 0px 1px rgba(22,163,74,0.3), 0px 4px 12px rgba(22,163,74,0.06);
}
.finding-card.severity-ok::before,
.finding-card.severity-ok::after,
.finding-card.severity-ok .corner-bl,
.finding-card.severity-ok .corner-br {
  border-color: #16a34a;
}

/* ---- Severity pill badge ---- */
.sev-pill {
  display: inline-flex;
  align-items: center;
  padding: 1px 8px;
  border-radius: 9999px;
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.4px;
  line-height: 1.6;
}
.sev-pill.sev-critical { background: rgba(220,38,38,0.08); color: #dc2626; }
.sev-pill.sev-warning  { background: rgba(217,119,6,0.08); color: #d97706; }
.sev-pill.sev-info     { background: rgba(0,114,245,0.08); color: #0072f5; }
.sev-pill.sev-ok       { background: rgba(22,163,74,0.08); color: #16a34a; }

/* ---- Category tag ---- */
.cat-tag {
  font-size: 11px;
  font-weight: 500;
  color: #808080;
  text-transform: uppercase;
  letter-spacing: 0.4px;
}

.finding-title {
  font-size: 14px;
  font-weight: 600;
  color: #171717;
  letter-spacing: -0.2px;
}

.finding-card { cursor: pointer; }

.finding-desc {
  font-size: 13px;
  font-weight: 400;
  color: #4d4d4d;
  line-height: 1.5;
  margin-top: 2px;
}

.finding-expandable {
  overflow: hidden;
  max-height: 0;
  opacity: 0;
  transition: max-height 0.3s ease, opacity 0.3s ease;
}

.finding-card.active .finding-expandable {
  max-height: 500px;
  opacity: 1;
}

.finding-detail-row {
  display: flex;
  gap: 8px;
  font-size: 13px;
  margin-top: 4px;
}

.finding-detail-label {
  font-weight: 600;
  color: #808080;
  min-width: 60px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.3px;
  padding-top: 2px;
}

.finding-detail-value {
  color: #4d4d4d;
  line-height: 1.5;
}

.finding-detail-value.val-accent { color: #171717; font-weight: 500; }
.finding-detail-value.val-italic { font-style: italic; }

.finding-evidence-list {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-family: 'SF Mono', monospace;
  font-size: 11px;
  color: #808080;
}

.finding-details {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid rgba(0,0,0,0.06);
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.finding-meta {
  display: flex;
  gap: 12px;
  margin-top: 8px;
  flex-wrap: wrap;
}

.finding-meta-item {
  font-size: 12px;
  color: #666666;
}

.finding-meta-item strong {
  font-weight: 500;
  color: #4d4d4d;
}

/* ---- Disk bars (compact) ---- */
.disk-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.disk-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.disk-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.disk-label {
  font-size: 13px;
  font-weight: 500;
  color: #171717;
}

.disk-detail {
  font-size: 12px;
  font-weight: 400;
  color: #666666;
}

.disk-bar-track {
  width: 100%;
  height: 6px;
  background: #ebebeb;
  border-radius: 9999px;
  overflow: hidden;
}

.disk-bar-fill {
  height: 100%;
  border-radius: 9999px;
  transition: width 0.5s ease;
}

.disk-bar-fill.color-green { background: #16a34a; }
.disk-bar-fill.color-amber { background: #d97706; }
.disk-bar-fill.color-red   { background: #dc2626; }

/* ---- Tables ---- */
.table-container {
  background: #ffffff;
  border-radius: 8px;
  box-shadow: 0px 0px 0px 1px rgba(0,0,0,0.08);
  overflow: hidden;
}

.table-container table {
  width: 100%;
  border-collapse: collapse;
}

.table-container thead th {
  background: #fafafa;
  font-size: 11px;
  font-weight: 500;
  color: #808080;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  padding: 8px 12px;
  text-align: left;
  border-bottom: 1px solid rgba(0,0,0,0.08);
}

.table-container tbody td {
  font-size: 13px;
  font-weight: 400;
  color: #171717;
  padding: 8px 12px;
  border-bottom: 1px solid rgba(0,0,0,0.04);
}

.table-container tbody tr:last-child td {
  border-bottom: none;
}

/* ---- Buttons ---- */
.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  font-size: 13px;
  font-weight: 500;
  font-family: inherit;
  border-radius: 6px;
  border: none;
  cursor: pointer;
  transition: opacity 0.15s;
}

.btn:hover {
  opacity: 0.85;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-primary {
  background: #171717;
  color: #ffffff;
}

.btn-outline {
  background: #ffffff;
  color: #171717;
  box-shadow: 0px 0px 0px 1px rgba(0,0,0,0.15);
}

/* ---- Empty states ---- */
.empty-state {
  text-align: center;
  padding: 32px 16px;
  color: #808080;
  font-size: 13px;
  font-weight: 400;
}

/* ---- Status dot ---- */
.status-dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  margin-right: 5px;
}

.status-dot.green  { background: #16a34a; }
.status-dot.amber  { background: #d97706; }
.status-dot.red    { background: #dc2626; }
.status-dot.gray   { background: #808080; }

/* ---- Loading ---- */
.loading {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 80px 24px;
  color: #808080;
  font-size: 14px;
}

/* ---- Refresh indicator ---- */
.refresh-bar {
  position: fixed;
  top: 0;
  left: 0;
  height: 2px;
  background: #171717;
  transition: width 0.3s ease;
  z-index: 200;
}
</style>
</head>
<body>

<div class="refresh-bar" id="refreshBar" style="width: 0%"></div>

<header class="header">
  <a href="/" class="header-brand"><img src="/icon.png" alt="" style="width:22px;height:22px;border-radius:4px;vertical-align:middle;margin-right:6px;">NAS Doctor</a>
  <div class="header-right">
    <div class="theme-switcher">
      <a href="/">Midnight</a>
      <a href="/theme/clean" class="active">Clean</a>
      <a href="/theme/ember">Ember</a>
    </div>
    <div class="nav-links">
      <a href="/api/v1/report" class="nav-link" target="_blank">Export Report</a>
      <a href="/stats" class="nav-link">Stats</a>
      <a href="https://github.com/mcdays94/nas-doctor" class="nav-link" target="_blank" title="GitHub"><svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" style="vertical-align:middle"><path d="M12 0C5.37 0 0 5.37 0 12c0 5.3 3.44 9.8 8.2 11.39.6.11.82-.26.82-.58v-2.03c-3.34.73-4.04-1.61-4.04-1.61-.55-1.39-1.34-1.76-1.34-1.76-1.09-.75.08-.73.08-.73 1.2.08 1.84 1.24 1.84 1.24 1.07 1.84 2.81 1.31 3.5 1 .1-.78.42-1.31.76-1.61-2.67-.3-5.47-1.33-5.47-5.93 0-1.31.47-2.38 1.24-3.22-.13-.3-.54-1.52.12-3.18 0 0 1.01-.32 3.3 1.23a11.5 11.5 0 0 1 6.02 0c2.28-1.55 3.29-1.23 3.29-1.23.66 1.66.25 2.88.12 3.18.77.84 1.24 1.91 1.24 3.22 0 4.61-2.81 5.63-5.48 5.92.43.37.81 1.1.81 2.22v3.29c0 .32.22.7.82.58C20.56 21.8 24 17.3 24 12c0-6.63-5.37-12-12-12z"/></svg></a>
      <a href="/settings" class="nav-link" title="Settings"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="vertical-align:middle"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg></a>
    </div>
  </div>
</header>

<div class="wrapper">
  <div id="app">
    <div class="loading" id="loadingState">Loading diagnostics...</div>
  </div>
</div>

<script src="/js/charts.js"></script>
<script>
(function() {
  "use strict";

  var REFRESH_INTERVAL = 30000;
  var _cachedStatus = null;
  var refreshTimer = null;
  var refreshCountdown = null;

  function escapeHTML(str) {
    if (!str) return "";
    var div = document.createElement("div");
    div.appendChild(document.createTextNode(str));
    return div.innerHTML;
  }

  function formatPct(n) {
    if (n == null) return "-";
    return n.toFixed(1) + "%";
  }

  function diskBarColor(pct) {
    if (pct >= 90) return "red";
    if (pct >= 75) return "amber";
    return "green";
  }

  function formatGB(gb) {
    if (gb == null) return "-";
    if (gb >= 1000) return (gb / 1000).toFixed(2) + " TB";
    return gb.toFixed(1) + " GB";
  }

  function severityClass(sev) {
    if (sev === "critical") return "severity-critical";
    if (sev === "warning") return "severity-warning";
    if (sev === "info") return "severity-info";
    return "severity-ok";
  }

  function pillClass(sev) {
    if (sev === "critical") return "pill-critical";
    if (sev === "warning") return "pill-warning";
    if (sev === "healthy") return "pill-healthy";
    return "pill-info";
  }

  function healthLabel(h) {
    if (h === "critical") return "Critical";
    if (h === "warning") return "Warning";
    return "Healthy";
  }

  function containerStatusDot(state) {
    if (state === "running") return "green";
    if (state === "exited") return "red";
    return "gray";
  }

  function fetchJSON(url) {
    return fetch(url).then(function(res) {
      if (!res.ok) throw new Error("HTTP " + res.status);
      return res.json();
    });
  }

  function renderDashboard(status, snapshot) {
    var html = "";
    var sys = (snapshot && snapshot.system) ? snapshot.system : null;

    // ---- Compact top bar: health + stats ----
    html += '<div class="top-bar">';

    // Left side: hostname + health pill + finding count pills
    html += '<div class="top-bar-left">';
    if (status.hostname) {
      html += '<span style="font-size:13px;font-weight:600;color:#171717;letter-spacing:-0.2px" id="hostname">' + escapeHTML(status.hostname) + '</span>';
      html += '<div class="top-bar-divider"></div>';
    }
    html += '<span class="pill ' + pillClass(status.overall_health) + '">' + escapeHTML(healthLabel(status.overall_health)) + '</span>';
    if (status.critical_count > 0) {
      html += '<span class="pill pill-critical">' + status.critical_count + ' Critical</span>';
    }
    if (status.warning_count > 0) {
      html += '<span class="pill pill-warning">' + status.warning_count + ' Warning</span>';
    }
    if (status.info_count > 0) {
      html += '<span class="pill pill-info">' + status.info_count + ' Info</span>';
    }
    if (status.critical_count === 0 && status.warning_count === 0 && status.info_count === 0) {
      html += '<span class="pill pill-neutral">No findings</span>';
    }
    html += '</div>';

    // Divider
    html += '<div class="top-bar-divider"></div>';

    // Right side: system stats inline
    html += '<div class="top-bar-right">';
    if (sys) {
      html += '<span class="stat-item"><span class="stat-label">CPU</span> <span class="stat-val">' + formatPct(sys.cpu_usage_percent) + '</span><canvas id="spark-cpu" width="40" height="18" style="margin-left:3px;vertical-align:middle"></canvas></span>';
      html += '<span class="dot-sep">&middot;</span>';
      html += '<span class="stat-item"><span class="stat-label">Mem</span> <span class="stat-val">' + formatPct(sys.mem_percent) + '</span><canvas id="spark-mem" width="40" height="18" style="margin-left:3px;vertical-align:middle"></canvas></span>';
      html += '<span class="dot-sep">&middot;</span>';
      html += '<span class="stat-item"><span class="stat-label">I/O</span> <span class="stat-val">' + formatPct(sys.io_wait_percent) + '</span></span>';
      html += '<span class="dot-sep">&middot;</span>';
      html += '<span class="stat-item"><span class="stat-label">Up</span> <span class="stat-val">' + escapeHTML(status.uptime || "-") + '</span></span>';
    } else {
      html += '<span style="color:#808080;font-size:12px">No system data</span>';
    }
    html += '</div>';

    html += '</div>';

    // ---- Scan bar ----
    html += '<div class="scan-bar">';
    html += '<div class="scan-info">';
    if (status.last_scan) {
      html += 'Last scan: ' + escapeHTML(new Date(status.last_scan).toLocaleString());
    } else {
      html += 'No scans yet';
    }
    if (status.scan_running) {
      html += ' &middot; <strong style="color:#d97706">Scan in progress...</strong>';
    }
    html += '</div>';
    html += '<button class="btn btn-primary" id="scanBtn" onclick="window._triggerScan()"';
    if (status.scan_running) html += ' disabled';
    html += '>Run Scan</button>';
    html += '</div>';

    // ---- Two-column grid ----
    // OS version banner
    var sysP = (snapshot && snapshot.system) ? snapshot.system.platform : null;
    var sysV = (snapshot && snapshot.system) ? snapshot.system.platform_version : null;
    if (sysP && sysV) {
      var hasUpd = snapshot.update && snapshot.update.update_available;
      html += '<div style="display:flex;align-items:center;gap:8px;padding:10px 14px;background:#f5f5f5;border:1px solid rgba(0,0,0,0.08);border-radius:8px;margin-bottom:12px;font-size:13px">';
      html += '<span style="color:#171717;font-weight:600">' + escapeHTML(sysP) + '</span>';
      html += '<span style="color:#808080">v' + escapeHTML(sysV) + '</span>';
      if (hasUpd) {
        html += '<span style="color:#171717;font-weight:600;margin-left:8px">Update → ' + escapeHTML(snapshot.update.latest_version) + '</span>';
        if (snapshot.update.release_url) html += '<a href="' + escapeHTML(snapshot.update.release_url) + '" target="_blank" style="color:#171717;text-decoration:underline;margin-left:auto">Release notes</a>';
      }
      html += '</div>';
    }

    html += '<div class="two-col" id="two-col">';
    html += '<div class="col-left" id="col-left"></div>';
    html += '<div class="col-right" id="col-right"></div>';
    html += '</div>';

    html += '<div id="section-staging" style="position:absolute;visibility:hidden;width:50%;left:-9999px">';

    // ==== Section: Findings ====
    html += '<div class="section-block" data-section="findings">';
    html += '<div class="section">';
    html += '<div class="section-title">Findings</div>';
    if (snapshot && snapshot.findings && snapshot.findings.length > 0) {
      html += '<div class="findings-list">';
      var findings = snapshot.findings.slice().sort(function(a, b) {
        var order = { critical: 0, warning: 1, info: 2, ok: 3 };
        return (order[a.severity] || 3) - (order[b.severity] || 3);
      });
      for (var i = 0; i < findings.length; i++) {
        var f = findings[i];
        var sevKey = f.severity === "critical" ? "critical" : f.severity === "warning" ? "warning" : f.severity === "info" ? "info" : "ok";
        html += '<div class="finding-card ' + severityClass(f.severity) + '" onclick="window._toggleFinding(this)">';
        html += '<div class="corner-bl"></div>';
        html += '<div class="corner-br"></div>';
        html += '<div style="display:flex;align-items:center;gap:6px;margin-bottom:2px;">';
        html += '<span class="sev-pill sev-' + sevKey + '">' + escapeHTML(f.severity) + '</span>';
        html += '<span class="cat-tag">' + escapeHTML(f.category) + '</span>';
        html += '</div>';
        html += '<div class="finding-title">' + escapeHTML(f.title) + '</div>';
        html += '<div class="finding-expandable">';
        html += '<div class="finding-details">';
        html += '<div class="finding-desc">' + escapeHTML(f.description) + '</div>';
        if (f.evidence && f.evidence.length > 0) {
          html += '<div class="finding-detail-row"><div class="finding-detail-label">Evidence</div><div class="finding-detail-value"><ul class="finding-evidence-list">';
          for (var ei = 0; ei < f.evidence.length; ei++) {
            html += '<li>' + escapeHTML(f.evidence[ei]) + '</li>';
          }
          html += '</ul></div></div>';
        }
        if (f.action) html += '<div class="finding-detail-row"><div class="finding-detail-label">Action</div><div class="finding-detail-value val-accent">' + escapeHTML(f.action) + '</div></div>';
        if (f.impact) html += '<div class="finding-detail-row"><div class="finding-detail-label">Impact</div><div class="finding-detail-value val-italic">' + escapeHTML(f.impact) + '</div></div>';
        if (f.priority || f.cost) {
          html += '<div class="finding-meta">';
          if (f.priority) html += '<div class="finding-meta-item"><strong>Priority:</strong> ' + escapeHTML(f.priority) + '</div>';
          if (f.cost) html += '<div class="finding-meta-item"><strong>Cost:</strong> ' + escapeHTML(f.cost) + '</div>';
          html += '</div>';
        }
        html += '</div>';
        html += '</div>';
        html += '</div>';
      }
      html += '</div>';
    } else {
      html += '<div class="card"><div class="empty-state">No findings to display. Run a scan to check your system.</div></div>';
    }
    html += '</div>';
    html += '</div>'; // end section-block findings

    // ==== Section: Disk Space ====
    html += '<div class="section-block" data-section="disk-space">';
    html += '<div class="section">';
    html += '<div class="section-title">Disk Space</div>';
    if (snapshot && snapshot.disks && snapshot.disks.length > 0) {
      html += '<div class="card">';
      html += '<div class="disk-list">';
      for (var d = 0; d < snapshot.disks.length; d++) {
        var disk = snapshot.disks[d];
        var pct = disk.used_percent || 0;
        var col = diskBarColor(pct);
        html += '<div class="disk-item">';
        html += '<div class="disk-info">';
        html += '<span class="disk-label">' + escapeHTML(disk.label || disk.mount_point || disk.device) + '</span>';
        html += '<span class="disk-detail">' + formatGB(disk.used_gb) + ' / ' + formatGB(disk.total_gb) + ' (' + formatPct(pct) + ')</span>';
        html += '</div>';
        html += '<div class="disk-bar-track"><div class="disk-bar-fill color-' + col + '" style="width:' + pct.toFixed(1) + '%"></div></div>';
        html += '</div>';
      }
      html += '</div>';
      html += '</div>';
    } else {
      html += '<div class="card"><div class="empty-state">No disk data available.</div></div>';
    }
    html += '</div>'; // end section
    html += '</div>'; // end section-block disk-space

    // ==== Section: SMART Health ====
    html += '<div class="section-block" data-section="smart">';
    html += '<div class="section">';
    html += '<div class="section-title">SMART Health</div>';
    if (snapshot && snapshot.smart && snapshot.smart.length > 0) {
      html += '<div class="table-container">';
      html += '<table>';
      html += '<thead><tr>';
      html += '<th>Device</th><th>Model</th><th>Health</th><th>Temp</th><th style="width:70px">Trend</th><th>Power-On</th><th>Realloc</th><th>Pending</th>';
      html += '</tr></thead>';
      html += '<tbody>';
      for (var s = 0; s < snapshot.smart.length; s++) {
        var sm = snapshot.smart[s];
        var healthOk = sm.health_passed;
        var hrs = sm.power_on_hours;
        var hrsStr = hrs != null ? (hrs > 8760 ? (hrs / 8760).toFixed(1) + "y" : hrs + "h") : "-";
        html += '<tr style="cursor:pointer" onclick="window.location=\'/disk/' + encodeURIComponent(sm.serial || '') + '\'">';
        html += '<td>' + escapeHTML(sm.device) + '</td>';
        html += '<td>' + escapeHTML(sm.model) + '</td>';
        html += '<td><span class="status-dot ' + (healthOk ? "green" : "red") + '"></span>' + (healthOk ? "Passed" : "Failed") + '</td>';
        html += '<td>' + (sm.temperature_c != null ? sm.temperature_c + "&deg;C" : "-") + '</td>';
        html += '<td><canvas id="spark-temp-' + s + '" width="60" height="20"></canvas></td>';
        html += '<td>' + hrsStr + '</td>';
        html += '<td>' + (sm.reallocated_sectors != null ? sm.reallocated_sectors : "-") + '</td>';
        html += '<td>' + (sm.pending_sectors != null ? sm.pending_sectors : "-") + '</td>';
        html += '</tr>';
      }
      html += '</tbody>';
      html += '</table>';
      html += '</div>';
    } else {
      html += '<div class="card"><div class="empty-state">No SMART data available.</div></div>';
    }
    html += '</div>'; // end section
    html += '</div>'; // end section-block smart

    // ==== Section: Docker ====
    html += '<div class="section-block" data-section="docker">';
    html += '<div class="section">';
    html += '<div class="section-title">Docker Containers</div>';
    if (snapshot && snapshot.docker && snapshot.docker.available && snapshot.docker.containers && snapshot.docker.containers.length > 0) {
      html += '<div class="table-container">';
      html += '<table>';
      html += '<thead><tr>';
      html += '<th>Name</th><th>Image</th><th>State</th><th>CPU</th><th>Memory</th><th>Uptime</th>';
      html += '</tr></thead>';
      html += '<tbody>';
      for (var c = 0; c < snapshot.docker.containers.length; c++) {
        var ct = snapshot.docker.containers[c];
        html += '<tr>';
        html += '<td style="font-weight:500">' + escapeHTML(ct.name) + '</td>';
        html += '<td style="color:#666666;font-size:12px">' + escapeHTML(ct.image) + '</td>';
        html += '<td><span class="status-dot ' + containerStatusDot(ct.state) + '"></span>' + escapeHTML(ct.state) + '</td>';
        html += '<td>' + formatPct(ct.cpu_percent) + '</td>';
        html += '<td>' + (ct.mem_mb != null ? ct.mem_mb.toFixed(1) + " MB" : "-") + '</td>';
        html += '<td style="color:#666666">' + escapeHTML(ct.uptime || "-") + '</td>';
        html += '</tr>';
      }
      html += '</tbody>';
      html += '</table>';
      html += '</div>';
    } else {
      html += '<div class="card"><div class="empty-state">No Docker containers found or Docker not available.</div></div>';
    }
    html += '</div>'; // end section
    html += '</div>'; // end section-block docker

    // ==== Section: ZFS Pools ====
    html += '<div class="section-block" data-section="zfs">';
    if (snapshot && snapshot.zfs && snapshot.zfs.available && snapshot.zfs.pools && snapshot.zfs.pools.length > 0) {
      html += '<div class="section">';
      html += '<div class="section-title">ZFS Pools</div>';
      for (var zi = 0; zi < snapshot.zfs.pools.length; zi++) {
        var zp = snapshot.zfs.pools[zi];
        var stDot = zp.state === 'ONLINE' ? 'green' : zp.state === 'DEGRADED' ? 'amber' : 'red';
        html += '<div class="card" style="margin-bottom:8px">';
        html += '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px">';
        html += '<span style="font-weight:600;font-size:14px;color:#171717">' + escapeHTML(zp.name) + '</span>';
        html += '<span class="status-dot ' + stDot + '"></span> <span style="font-size:12px;font-weight:500">' + escapeHTML(zp.state) + '</span>';
        html += '</div>';
        html += '<div style="font-size:13px;color:#4d4d4d">' + (zp.used_gb || 0).toFixed(0) + ' / ' + (zp.total_gb || 0).toFixed(0) + ' GB (' + (zp.used_percent || 0).toFixed(0) + '%)</div>';
        if (zp.scan_type && zp.scan_type !== 'none') {
          html += '<div style="font-size:12px;color:#808080;margin-top:4px">Last ' + escapeHTML(zp.scan_type) + ': ' + (zp.scan_errors || 0) + ' errors</div>';
        }
        if (zp.vdevs && zp.vdevs.length > 0) {
          html += '<div style="margin-top:6px;font-size:11px;font-family:monospace;color:#808080">';
          for (var vi = 0; vi < zp.vdevs.length; vi++) {
            var vd = zp.vdevs[vi];
            html += '<div>' + escapeHTML(vd.name) + ' <span style="color:' + (vd.state === 'ONLINE' ? '#16a34a' : vd.state === 'DEGRADED' ? '#d97706' : '#dc2626') + '">' + escapeHTML(vd.state) + '</span></div>';
            if (vd.children) {
              for (var ci = 0; ci < vd.children.length; ci++) {
                var ch = vd.children[ci];
                html += '<div style="padding-left:14px">' + escapeHTML(ch.name) + ' <span style="color:' + (ch.state === 'ONLINE' ? '#16a34a' : ch.state === 'DEGRADED' ? '#d97706' : '#dc2626') + '">' + escapeHTML(ch.state) + '</span></div>';
              }
            }
          }
          html += '</div>';
        }
        html += '</div>';
      }
      if (snapshot.zfs.arc) {
        html += '<div style="font-size:11px;color:#808080;margin-top:4px">ARC: ' + (snapshot.zfs.arc.size_mb / 1024).toFixed(1) + ' GB / ' + (snapshot.zfs.arc.max_size_mb / 1024).toFixed(1) + ' GB · Hit rate: ' + (snapshot.zfs.arc.hit_rate_percent || 0).toFixed(1) + '%</div>';
      }
      html += '</div>';
    }

    html += '</div>'; // end section-block zfs

    // ==== Section: UPS ====
    html += '<div class="section-block" data-section="ups">';
    if (snapshot && snapshot.ups && snapshot.ups.available) {
      var ups = snapshot.ups;
      html += '<div class="section">';
      html += '<div class="section-title">UPS / Power</div>';
      html += '<div class="card">';
      var upsDot = ups.on_battery ? 'red' : 'green';
      html += '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px">';
      html += '<span style="font-weight:600;font-size:14px">' + escapeHTML(ups.name || ups.model) + '</span>';
      html += '<span><span class="status-dot ' + upsDot + '"></span> ' + escapeHTML(ups.status_human) + '</span>';
      html += '</div>';
      html += '<div style="display:flex;gap:16px;font-size:13px;color:#4d4d4d;flex-wrap:wrap">';
      html += '<span>Battery: <strong>' + (ups.battery_percent || 0).toFixed(0) + '%</strong></span>';
      html += '<span>Load: <strong>' + (ups.load_percent || 0).toFixed(0) + '%</strong></span>';
      html += '<span>Runtime: <strong>' + (ups.runtime_minutes || 0).toFixed(0) + ' min</strong></span>';
      if (ups.wattage_watts > 0) html += '<span>' + (ups.wattage_watts || 0).toFixed(0) + 'W / ' + (ups.nominal_watts || 0).toFixed(0) + 'W</span>';
      html += '</div>';
      if (ups.last_transfer) html += '<div style="font-size:12px;color:#808080;margin-top:4px">Last transfer: ' + escapeHTML(ups.last_transfer) + '</div>';
      html += '</div>';
      html += '</div>';
    }
    html += '</div>'; // end section-block ups

    html += '</div>'; // end section-staging

    return html;
  }

  function loadData() {
    var bar = document.getElementById("refreshBar");
    if (bar) bar.style.width = "30%";

    Promise.all([
      fetchJSON("/api/v1/status"),
      fetchJSON("/api/v1/snapshot/latest").catch(function() { return null; })
    ]).then(function(results) {
      var status = results[0];
      var snapshot = results[1];
      _cachedStatus = status;

      if (bar) bar.style.width = "100%";
      setTimeout(function() { if (bar) bar.style.width = "0%"; }, 400);

      // Update hostname
      var hostnameEl = document.getElementById("hostname");
      if (hostnameEl && status.hostname) {
        hostnameEl.textContent = status.hostname;
      }

      // Update page title
      if (status.hostname) {
        document.title = "NAS Doctor - " + status.hostname;
      }

      // Render
      var app = document.getElementById("app");
      if (app) {
        app.innerHTML = renderDashboard(status, snapshot);
        _distributeSections();
        _renderSparklines(snapshot);
      }
    }).catch(function(err) {
      var bar2 = document.getElementById("refreshBar");
      if (bar2) bar2.style.width = "0%";

      var app2 = document.getElementById("app");
      if (app2) {
        var loadEl = document.getElementById("loadingState");
        if (loadEl) {
          loadEl.textContent = "Failed to load data. Retrying...";
        }
      }
      console.error("Failed to load dashboard data:", err);
    });
  }

  function _distributeSections() {
    var staging = document.getElementById("section-staging");
    var colL = document.getElementById("col-left");
    var colR = document.getElementById("col-right");
    if (!staging || !colL || !colR) return;
    var sec = (_cachedStatus && _cachedStatus.sections) ? _cachedStatus.sections : {};
    var sectionMap = { "findings": sec.findings !== false, "disk-space": sec.disk_space !== false, "smart": sec.smart !== false, "docker": sec.docker !== false, "zfs": sec.zfs !== false, "ups": sec.ups !== false };
    var blocks = staging.querySelectorAll(".section-block");
    if (blocks.length === 0) return;
    var items = [];
    for (var i = 0; i < blocks.length; i++) {
      var name = blocks[i].getAttribute("data-section");
      if (sectionMap[name] === false) continue;
      if (blocks[i].offsetHeight < 10) continue;
      items.push({ el: blocks[i], h: blocks[i].offsetHeight });
    }
    var leftH = 0, rightH = 0;
    for (var j = 0; j < items.length; j++) {
      if (leftH <= rightH) { colL.appendChild(items[j].el); leftH += items[j].h; }
      else { colR.appendChild(items[j].el); rightH += items[j].h; }
    }
    staging.parentNode.removeChild(staging);
  }

  function _renderSparklines(snapshot) {
    fetch("/api/v1/sparklines")
      .then(function(r) { return r.json(); })
      .then(function(data) {
        if (data.system && data.system.length >= 2 && window.NasChart) {
          var cpuD = data.system.map(function(p) { return p.cpu_usage; });
          var memD = data.system.map(function(p) { return p.mem_percent; });
          try { NasChart.sparkline("spark-cpu", { data: cpuD, color: "#171717", width: 40, height: 18 }); } catch(e) {}
          try { NasChart.sparkline("spark-mem", { data: memD, color: "#171717", width: 40, height: 18 }); } catch(e) {}
        }
        if (data.disks && snapshot && snapshot.smart && window.NasChart) {
          for (var i = 0; i < snapshot.smart.length; i++) {
            var serial = snapshot.smart[i].serial || "";
            var dd = null;
            for (var d = 0; d < data.disks.length; d++) {
              if (data.disks[d].serial === serial) { dd = data.disks[d]; break; }
            }
            if (dd && dd.temps && dd.temps.length >= 2) {
              var temps = dd.temps.map(function(p) { return p.temp; });
              var mx = Math.max.apply(null, temps);
              var clr = mx >= 55 ? "#dc2626" : mx >= 45 ? "#d97706" : "#16a34a";
              try { NasChart.sparkline("spark-temp-" + i, { data: temps, color: clr, width: 60, height: 20 }); } catch(e) {}
            }
          }
        }
      }).catch(function() {});
  }

  window._toggleFinding = function(el) {
    var all = document.querySelectorAll(".finding-card");
    for (var i = 0; i < all.length; i++) {
      if (all[i] !== el) all[i].classList.remove("active");
    }
    el.classList.toggle("active");
  };

  window._triggerScan = function() {
    var btn = document.getElementById("scanBtn");
    if (btn) btn.disabled = true;

    fetch("/api/v1/scan", { method: "POST" }).then(function(res) {
      if (!res.ok) return res.json().then(function(d) { throw new Error(d.error || "Scan failed"); });
      // Refresh shortly after triggering
      setTimeout(loadData, 2000);
    }).catch(function(err) {
      alert("Scan error: " + err.message);
      if (btn) btn.disabled = false;
    });
  };

  // Initial load
  loadData();

  // Auto-refresh
  refreshTimer = setInterval(loadData, REFRESH_INTERVAL);
})();
</script>
</body>
</html>`
