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
  max-width: 1080px;
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

.header-hostname {
  font-size: 14px;
  font-weight: 500;
  color: #4d4d4d;
}

.header-nav {
  display: flex;
  gap: 8px;
}

.header-nav a {
  font-size: 13px;
  font-weight: 500;
  color: #666666;
  text-decoration: none;
  padding: 4px 10px;
  border-radius: 6px;
  transition: color 0.15s;
}

.header-nav a:hover {
  color: #171717;
}

.header-nav a.active {
  color: #171717;
  text-decoration: underline;
  text-underline-offset: 4px;
}

/* ---- Section spacing ---- */
.section {
  margin-top: 48px;
}

.section-title {
  font-size: 14px;
  font-weight: 500;
  color: #808080;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 16px;
}

/* ---- Cards ---- */
.card {
  background: #ffffff;
  border-radius: 8px;
  box-shadow: 0px 0px 0px 1px rgba(0,0,0,0.08);
  padding: 24px;
}

.card-elevated {
  background: #ffffff;
  border-radius: 8px;
  box-shadow: rgba(0,0,0,0.08) 0px 0px 0px 1px, rgba(0,0,0,0.04) 0px 2px 2px, #fafafa 0px 0px 0px 1px;
  padding: 24px;
}

/* ---- Pills / Badges ---- */
.pill {
  display: inline-flex;
  align-items: center;
  padding: 4px 12px;
  border-radius: 9999px;
  font-size: 13px;
  font-weight: 500;
  line-height: 1.4;
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

/* ---- Health overview ---- */
.health-overview {
  display: flex;
  align-items: center;
  gap: 24px;
  flex-wrap: wrap;
}

.health-status-label {
  font-size: 32px;
  font-weight: 600;
  letter-spacing: -1.28px;
  color: #171717;
}

.health-pills {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

/* ---- Stats grid ---- */
.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
}

@media (max-width: 768px) {
  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

.stat-value {
  font-size: 24px;
  font-weight: 600;
  letter-spacing: -0.96px;
  color: #171717;
  line-height: 1.2;
}

.stat-label {
  font-size: 13px;
  font-weight: 400;
  color: #666666;
  margin-top: 4px;
}

/* ---- Findings ---- */
.findings-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.finding-card {
  position: relative;
  background: #ffffff;
  border-radius: 8px;
  border: none;
  padding: 20px 24px;
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
  width: 8px;
  height: 8px;
  border-radius: 1.5px;
  background: #fafafa;
  border: 1px solid #ebebeb;
  transition: border-style 0.2s ease;
  z-index: 1;
}
.finding-card::before { top: -4px; left: -4px; }
.finding-card::after  { top: -4px; right: -4px; }
.finding-card .corner-bl { bottom: -4px; left: -4px; }
.finding-card .corner-br { bottom: -4px; right: -4px; }

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
  padding: 2px 10px;
  border-radius: 9999px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  line-height: 1.6;
}
.sev-pill.sev-critical { background: rgba(220,38,38,0.08); color: #dc2626; }
.sev-pill.sev-warning  { background: rgba(217,119,6,0.08); color: #d97706; }
.sev-pill.sev-info     { background: rgba(0,114,245,0.08); color: #0072f5; }
.sev-pill.sev-ok       { background: rgba(22,163,74,0.08); color: #16a34a; }

/* ---- Category tag ---- */
.cat-tag {
  font-size: 12px;
  font-weight: 500;
  color: #808080;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.finding-title {
  font-size: 16px;
  font-weight: 600;
  color: #171717;
  letter-spacing: -0.32px;
}

.finding-desc {
  font-size: 15px;
  font-weight: 400;
  color: #4d4d4d;
  line-height: 1.6;
  margin-top: 4px;
}

.finding-meta {
  display: flex;
  gap: 16px;
  margin-top: 12px;
  flex-wrap: wrap;
}

.finding-meta-item {
  font-size: 13px;
  color: #666666;
}

.finding-meta-item strong {
  font-weight: 500;
  color: #4d4d4d;
}

/* ---- Disk bars ---- */
.disk-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.disk-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.disk-info {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
}

.disk-label {
  font-size: 14px;
  font-weight: 500;
  color: #171717;
}

.disk-detail {
  font-size: 13px;
  font-weight: 400;
  color: #666666;
}

.disk-bar-track {
  width: 100%;
  height: 8px;
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
  font-size: 12px;
  font-weight: 500;
  color: #808080;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  padding: 10px 16px;
  text-align: left;
  border-bottom: 1px solid rgba(0,0,0,0.08);
}

.table-container tbody td {
  font-size: 14px;
  font-weight: 400;
  color: #171717;
  padding: 12px 16px;
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
  padding: 8px 16px;
  font-size: 14px;
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

/* ---- Scan bar ---- */
.scan-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.scan-info {
  font-size: 13px;
  color: #808080;
  font-weight: 400;
}

/* ---- Empty states ---- */
.empty-state {
  text-align: center;
  padding: 48px 24px;
  color: #808080;
  font-size: 14px;
  font-weight: 400;
}

/* ---- Status dot ---- */
.status-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 6px;
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
  <span class="header-hostname" id="hostname">-</span>
  <nav class="header-nav">
    <a href="/">Midnight</a>
    <a href="/theme/clean" class="active">Clean</a>
    <a href="/theme/ember">Ember</a>
    <a href="/api/v1/report" target="_blank">Export Report</a>
    <a href="/settings">Settings</a>
  </nav>
</header>

<div class="wrapper">
  <div id="app">
    <div class="loading" id="loadingState">Loading diagnostics...</div>
  </div>
</div>

<script>
(function() {
  "use strict";

  var REFRESH_INTERVAL = 30000;
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

    // ---- Health overview ----
    html += '<div class="section">';
    html += '<div class="card">';
    html += '<div class="health-overview">';
    html += '<div class="health-status-label">' + escapeHTML(healthLabel(status.overall_health)) + '</div>';
    html += '<span class="pill ' + pillClass(status.overall_health) + '">' + escapeHTML(healthLabel(status.overall_health)) + '</span>';
    html += '<div class="health-pills">';
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
    html += '</div>';
    html += '</div>';
    html += '</div>';

    // ---- Scan bar ----
    html += '<div class="section">';
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
    html += '</div>';

    // ---- System stats ----
    if (snapshot && snapshot.system) {
      var sys = snapshot.system;

      html += '<div class="section">';
      html += '<div class="section-title">System</div>';
      html += '<div class="stats-grid">';

      // CPU
      html += '<div class="card-elevated">';
      html += '<div class="stat-value">' + formatPct(sys.cpu_usage_percent) + '</div>';
      html += '<div class="stat-label">CPU Usage</div>';
      html += '</div>';

      // Memory
      html += '<div class="card-elevated">';
      html += '<div class="stat-value">' + formatPct(sys.mem_percent) + '</div>';
      html += '<div class="stat-label">Memory (' + (sys.mem_used_mb || 0) + ' / ' + (sys.mem_total_mb || 0) + ' MB)</div>';
      html += '</div>';

      // IO Wait
      html += '<div class="card-elevated">';
      html += '<div class="stat-value">' + formatPct(sys.io_wait_percent) + '</div>';
      html += '<div class="stat-label">I/O Wait</div>';
      html += '</div>';

      // Uptime
      html += '<div class="card-elevated">';
      html += '<div class="stat-value">' + escapeHTML(status.uptime || "-") + '</div>';
      html += '<div class="stat-label">Uptime</div>';
      html += '</div>';

      html += '</div>';
      html += '</div>';
    }

    // ---- Findings ----
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
        html += '<div class="finding-card ' + severityClass(f.severity) + '">';
        html += '<div class="corner-bl"></div>';
        html += '<div class="corner-br"></div>';
        html += '<div style="display:flex;align-items:center;gap:8px;margin-bottom:8px;">';
        html += '<span class="sev-pill sev-' + sevKey + '">' + escapeHTML(f.severity) + '</span>';
        html += '<span class="cat-tag">' + escapeHTML(f.category) + '</span>';
        html += '</div>';
        html += '<div class="finding-title">' + escapeHTML(f.title) + '</div>';
        html += '<div class="finding-desc">' + escapeHTML(f.description) + '</div>';
        if (f.action || f.impact || f.priority) {
          html += '<div class="finding-meta">';
          if (f.action) html += '<div class="finding-meta-item"><strong>Action:</strong> ' + escapeHTML(f.action) + '</div>';
          if (f.impact) html += '<div class="finding-meta-item"><strong>Impact:</strong> ' + escapeHTML(f.impact) + '</div>';
          if (f.priority) html += '<div class="finding-meta-item"><strong>Priority:</strong> ' + escapeHTML(f.priority) + '</div>';
          html += '</div>';
        }
        html += '</div>';
      }
      html += '</div>';
    } else {
      html += '<div class="card"><div class="empty-state">No findings to display. Run a scan to check your system.</div></div>';
    }
    html += '</div>';

    // ---- Disk space ----
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
    html += '</div>';

    // ---- SMART health ----
    html += '<div class="section">';
    html += '<div class="section-title">SMART Health</div>';
    if (snapshot && snapshot.smart && snapshot.smart.length > 0) {
      html += '<div class="table-container">';
      html += '<table>';
      html += '<thead><tr>';
      html += '<th>Device</th><th>Model</th><th>Health</th><th>Temp</th><th>Power-On</th><th>Reallocated</th><th>Pending</th>';
      html += '</tr></thead>';
      html += '<tbody>';
      for (var s = 0; s < snapshot.smart.length; s++) {
        var sm = snapshot.smart[s];
        var healthOk = sm.health_passed;
        var hrs = sm.power_on_hours;
        var hrsStr = hrs != null ? (hrs > 8760 ? (hrs / 8760).toFixed(1) + "y" : hrs + "h") : "-";
        html += '<tr>';
        html += '<td>' + escapeHTML(sm.device) + '</td>';
        html += '<td>' + escapeHTML(sm.model) + '</td>';
        html += '<td><span class="status-dot ' + (healthOk ? "green" : "red") + '"></span>' + (healthOk ? "Passed" : "Failed") + '</td>';
        html += '<td>' + (sm.temperature_c != null ? sm.temperature_c + "&deg;C" : "-") + '</td>';
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
    html += '</div>';

    // ---- Docker containers ----
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
        html += '<td style="color:#666666;font-size:13px">' + escapeHTML(ct.image) + '</td>';
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
    html += '</div>';

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
