package api

// DashboardMidnight is the dark precision dashboard theme.
var DashboardMidnight = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>NAS Doctor — Midnight</title>
<link rel="icon" type="image/png" href="/icon.png">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg-base:#08090a;--bg-panel:#0f1011;--bg-elevated:#191a1b;
  --brand:#5e6ad2;--accent:#7170ff;--hover:#828fff;
  --border:rgba(255,255,255,0.05);--border-hover:rgba(255,255,255,0.08);
  --btn-bg:rgba(255,255,255,0.02);--btn-bg-hover:rgba(255,255,255,0.05);
  --text-primary:#f7f8f8;--text-secondary:#d0d6e0;--text-tertiary:#8a8f98;--text-quaternary:#62666d;
  --green:#10b981;--green-bg:rgba(16,185,129,0.1);
  --amber:#d97706;--amber-bg:rgba(217,119,6,0.1);
  --red:#dc2626;--red-bg:rgba(220,38,38,0.1);
  --radius:8px;--sp:8px;
}
html{background:var(--bg-base);color:var(--text-primary);font-family:'Inter',system-ui,-apple-system,sans-serif;font-feature-settings:"cv01","ss03";font-size:14px;line-height:1.5;-webkit-font-smoothing:antialiased;-moz-osx-font-smoothing:grayscale}
body{min-height:100vh;padding:calc(var(--sp)*3)}
a{color:var(--accent);text-decoration:none}
a:hover{color:var(--hover)}

.container{max-width:1200px;margin:0 auto}

/* Header */
.header{display:flex;align-items:center;justify-content:space-between;padding:calc(var(--sp)*2) 0;margin-bottom:0;border-bottom:1px solid var(--border)}
.header-left{display:flex;align-items:center;gap:calc(var(--sp)*2)}
.logo{display:flex;align-items:center;gap:var(--sp);font-size:20px;font-weight:600;letter-spacing:-0.5px;color:var(--text-primary)}
.logo-emoji{font-size:24px}
.hostname{font-size:13px;color:var(--text-tertiary);font-weight:400;padding:4px 10px;background:var(--btn-bg);border:1px solid var(--border);border-radius:var(--radius)}
.header-right{display:flex;align-items:center;gap:16px}
.theme-switcher{display:flex;align-items:center;gap:2px;background:var(--bg-elevated);border:1px solid var(--border);border-radius:var(--radius);padding:2px}
.theme-switcher a{padding:5px 10px;border-radius:6px;font-size:11px;font-weight:500;color:var(--text-quaternary);text-decoration:none;transition:all 0.15s ease;line-height:1.3}
.theme-switcher a:hover{color:var(--text-tertiary)}
.theme-switcher a.active{color:var(--text-primary);background:var(--btn-bg-hover);box-shadow:0 1px 2px rgba(0,0,0,0.3)}
.nav-links{display:flex;gap:4px}
.nav-link{padding:6px 12px;border-radius:var(--radius);font-size:12px;font-weight:500;color:var(--text-tertiary);border:1px solid transparent;transition:all 0.15s ease;text-decoration:none}
.nav-link:hover{color:var(--text-secondary);background:var(--btn-bg-hover);border-color:var(--border-hover)}

/* Top bar — compact health + stats */
.top-bar{display:flex;align-items:center;justify-content:space-between;padding:10px 0;border-bottom:1px solid var(--border);gap:calc(var(--sp)*2);min-height:48px;max-height:48px}
.top-bar-health{display:flex;align-items:center;gap:calc(var(--sp)*1.5);flex-shrink:0}
.health-pill{display:flex;align-items:center;gap:8px;padding:4px 14px;border-radius:20px;font-size:13px;font-weight:600}
.health-pill.healthy{background:var(--green-bg);color:var(--green)}
.health-pill.warning{background:var(--amber-bg);color:var(--amber)}
.health-pill.critical{background:var(--red-bg);color:var(--red)}
.health-pill-dot{width:8px;height:8px;border-radius:50%;flex-shrink:0}
.health-pill.healthy .health-pill-dot{background:var(--green)}
.health-pill.warning .health-pill-dot{background:var(--amber)}
.health-pill.critical .health-pill-dot{background:var(--red)}
.health-badges{display:flex;align-items:center;gap:6px;margin-left:4px}
.health-badge{font-size:11px;font-weight:600;padding:2px 7px;border-radius:10px;line-height:1.3}
.health-badge.badge-red{color:var(--red);background:var(--red-bg)}
.health-badge.badge-amber{color:var(--amber);background:var(--amber-bg)}
.health-badge.badge-brand{color:var(--accent);background:rgba(94,106,210,0.1)}
.top-bar-stats{display:flex;align-items:center;gap:calc(var(--sp)*3)}
.stat-item{display:flex;align-items:baseline;gap:6px;font-size:13px;white-space:nowrap}
.stat-item-label{color:var(--text-quaternary);font-weight:500;font-size:12px}
.stat-item-value{font-weight:600;font-size:14px;letter-spacing:-0.3px}
@media(max-width:700px){
  .top-bar{flex-wrap:wrap;max-height:none;padding:8px 0}
  .top-bar-stats{flex-wrap:wrap;gap:calc(var(--sp)*2)}
}

/* Scan bar — compact */
.scan-bar{display:flex;align-items:center;justify-content:space-between;padding:8px 0;margin-bottom:calc(var(--sp)*2);gap:calc(var(--sp)*2);border-bottom:1px solid var(--border)}
.scan-info{font-size:12px;color:var(--text-quaternary)}
.scan-info strong{color:var(--text-tertiary);font-weight:500}

/* Section titles */
.section-title{font-size:13px;font-weight:600;color:var(--text-tertiary);text-transform:uppercase;letter-spacing:0.5px;margin-bottom:calc(var(--sp)*1.5);margin-top:0}

/* Two-column layout */
.two-col{display:grid;grid-template-columns:1fr 1fr;gap:16px}
@media(max-width:900px){.two-col{grid-template-columns:1fr}}
.col-left{min-width:0;display:flex;flex-direction:column;min-height:0}
.col-right{min-width:0;display:flex;flex-direction:column;gap:calc(var(--sp)*3)}

/* Findings — compact */
.findings-list{display:flex;flex-direction:column;gap:6px;flex:1;overflow-y:auto;min-height:0;scrollbar-width:thin;scrollbar-color:var(--border-hover) transparent}
.findings-list::-webkit-scrollbar{width:5px}
.findings-list::-webkit-scrollbar-track{background:transparent}
.findings-list::-webkit-scrollbar-thumb{background:var(--border-hover);border-radius:3px}
.findings-list::-webkit-scrollbar-thumb:hover{background:rgba(255,255,255,0.12)}
.finding{border:1px solid rgba(255,255,255,0.06);border-radius:8px;padding:12px;transition:all 200ms ease}
.finding:hover{border-color:rgba(255,255,255,0.10)}
.finding-critical{background:rgba(220,38,38,0.06)}
.finding-critical:hover{background:rgba(220,38,38,0.10)}
.finding-warning{background:rgba(217,119,6,0.06)}
.finding-warning:hover{background:rgba(217,119,6,0.10)}
.finding-info{background:rgba(94,106,210,0.06)}
.finding-info:hover{background:rgba(94,106,210,0.10)}
.finding-ok{background:rgba(16,185,129,0.06)}
.finding-ok:hover{background:rgba(16,185,129,0.10)}
.sev-dot{display:inline-block;width:7px;height:7px;border-radius:50%;margin-right:6px;vertical-align:middle;flex-shrink:0}
.sev-dot-critical{background:#dc2626}
.sev-dot-warning{background:#d97706}
.sev-dot-info{background:#5e6ad2}
.sev-dot-ok{background:#10b981}
.finding{cursor:pointer;transition:background 0.15s ease}
.finding-title{font-size:13px;font-weight:600;color:var(--text-primary)}
.finding-desc{font-size:12px;color:var(--text-secondary);line-height:1.45}
.finding-expandable{overflow:hidden;max-height:0;opacity:0;transition:max-height 0.3s ease,opacity 0.3s ease}
.finding.active .finding-expandable{max-height:500px;opacity:1}
.finding-details{margin-top:8px;padding-top:8px;border-top:1px solid var(--border);display:flex;flex-direction:column;gap:6px}
.finding-detail-row{display:flex;gap:8px;font-size:12px}
.finding-detail-label{font-weight:600;color:var(--text-tertiary);min-width:60px;font-size:11px;text-transform:uppercase;letter-spacing:0.3px;padding-top:1px}
.finding-detail-value{color:var(--text-secondary);line-height:1.45}
.finding-detail-value.val-accent{color:var(--accent)}
.finding-detail-value.val-italic{font-style:italic}
.finding-evidence-list{list-style:none;display:flex;flex-direction:column;gap:2px;font-family:'SF Mono',monospace;font-size:11px;color:var(--text-tertiary)}
.finding-meta{display:flex;flex-wrap:wrap;gap:calc(var(--sp)*1.5);font-size:11px;color:var(--text-quaternary);margin-top:6px}
.finding-meta span{display:flex;align-items:center;gap:3px}
.finding-meta strong{color:var(--text-tertiary);font-weight:500}
.finding-tag{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:0.5px;padding:2px 7px;border-radius:4px}
.finding-tag.sev-critical{color:var(--red);background:var(--red-bg)}
.finding-tag.sev-warning{color:var(--amber);background:var(--amber-bg)}
.finding-tag.sev-info{color:var(--accent);background:rgba(94,106,210,0.1)}
.finding-tag.sev-ok{color:var(--green);background:var(--green-bg)}

/* Tables */
.table-wrap{background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);overflow:hidden}
table{width:100%;border-collapse:collapse}
thead{background:var(--bg-elevated)}
th{font-size:11px;font-weight:600;color:var(--text-tertiary);text-transform:uppercase;letter-spacing:0.4px;padding:calc(var(--sp)*1.5) calc(var(--sp)*2);text-align:left;border-bottom:1px solid var(--border)}
td{font-size:13px;color:var(--text-secondary);padding:calc(var(--sp)*1.5) calc(var(--sp)*2);border-bottom:1px solid var(--border)}
tr:last-child td{border-bottom:none}
tbody tr:nth-child(even){background:rgba(255,255,255,0.01)}
tbody tr:hover{background:rgba(255,255,255,0.03)}
.td-healthy{color:var(--green)}
.td-warn{color:var(--amber)}
.td-crit{color:var(--red)}

/* Disk bars — compact */
.disk-list{display:flex;flex-direction:column;gap:8px}
.disk-item{background:var(--bg-panel);border:1px solid var(--border);border-radius:var(--radius);padding:8px 12px}
.disk-header{display:flex;justify-content:space-between;align-items:center;margin-bottom:4px}
.disk-name{font-size:12px;font-weight:600;color:var(--text-primary)}
.disk-info{font-size:11px;color:var(--text-quaternary)}
.disk-bar-bg{height:8px;background:var(--bg-elevated);border-radius:4px;overflow:hidden}
.disk-bar-fill{height:100%;border-radius:4px;transition:width 0.5s ease}

/* Ghost button */
.ghost-btn{display:inline-flex;align-items:center;gap:var(--sp);padding:6px 14px;background:var(--btn-bg);border:1px solid var(--border-hover);border-radius:var(--radius);color:var(--text-secondary);font-family:inherit;font-size:12px;font-weight:500;cursor:pointer;transition:all 0.15s ease}
.ghost-btn:hover{background:var(--btn-bg-hover);border-color:rgba(255,255,255,0.12);color:var(--text-primary)}
.ghost-btn:active{transform:scale(0.98)}
.ghost-btn:disabled{opacity:0.4;cursor:not-allowed;transform:none}
.ghost-btn.scanning{color:var(--accent)}

/* Empty state */
.empty{text-align:center;padding:calc(var(--sp)*6);color:var(--text-quaternary);font-size:13px}
.empty-icon{font-size:28px;margin-bottom:calc(var(--sp)*1.5);opacity:0.5}

/* Status dot */
.status-dot{display:inline-block;width:7px;height:7px;border-radius:50%;margin-right:4px}
.status-dot.running{background:var(--green)}
.status-dot.exited{background:var(--red)}
.status-dot.paused{background:var(--amber)}

/* Utility */
.text-green{color:var(--green)}.text-amber{color:var(--amber)}.text-red{color:var(--red)}.text-brand{color:var(--accent)}

/* Fade in animation */
@keyframes fadeIn{from{opacity:0;transform:translateY(4px)}to{opacity:1;transform:translateY(0)}}
.fade-in{animation:fadeIn 0.3s ease both}

/* Loading spinner */
.spinner{display:inline-block;width:14px;height:14px;border:2px solid var(--border-hover);border-top-color:var(--accent);border-radius:50%;animation:spin 0.6s linear infinite}
@keyframes spin{to{transform:rotate(360deg)}}
</style>
</head>
<body>
<div class="container" id="app">
  <div class="empty" style="padding:80px 0">
    <div class="spinner" style="width:24px;height:24px;margin:0 auto 16px"></div>
    <div>Loading dashboard...</div>
  </div>
</div>

<script src="/js/charts.js"></script>
<script>
(function(){
  "use strict";
  var cachedSnapshot = null;

  var REFRESH_MS = 30000;
  var refreshTimer = null;
  var statusData = null;
  var snapshotData = null;
  var scanInProgress = false;

  function esc(s) {
    if (!s && s !== 0) return "";
    var d = document.createElement("div");
    d.appendChild(document.createTextNode(String(s)));
    return d.innerHTML;
  }

  function fetchJSON(url) {
    return fetch(url).then(function(r) {
      if (!r.ok) throw new Error(r.status + " " + r.statusText);
      return r.json();
    });
  }

  function loadAll() {
    return Promise.all([
      fetchJSON("/api/v1/status").then(function(d) { statusData = d; }).catch(function() { statusData = null; }),
      fetchJSON("/api/v1/snapshot/latest").then(function(d) { snapshotData = d; }).catch(function() { snapshotData = null; })
    ]).then(render);
  }

  function startRefresh() {
    if (refreshTimer) clearInterval(refreshTimer);
    refreshTimer = setInterval(loadAll, REFRESH_MS);
  }

  function triggerScan() {
    if (scanInProgress) return;
    scanInProgress = true;
    render();
    fetch("/api/v1/scan", { method: "POST" })
      .then(function(r) { return r.json(); })
      .then(function() {
        setTimeout(function() {
          scanInProgress = false;
          loadAll();
        }, 5000);
      })
      .catch(function() {
        scanInProgress = false;
        render();
      });
  }

  function colorForPct(pct) {
    if (pct >= 90) return "var(--red)";
    if (pct >= 75) return "var(--amber)";
    return "var(--green)";
  }

  function classForPct(pct) {
    if (pct >= 90) return "text-red";
    if (pct >= 75) return "text-amber";
    return "text-green";
  }

  function formatUptime(s) {
    if (!s) return "N/A";
    if (typeof s === "string") return s;
    var days = Math.floor(s / 86400);
    var hours = Math.floor((s % 86400) / 3600);
    if (days > 0) return days + "d " + hours + "h";
    var mins = Math.floor((s % 3600) / 60);
    return hours + "h " + mins + "m";
  }

  function render() {
    var st = statusData;
    var sn = snapshotData;
    cachedSnapshot = sn;
    var hostname = (st && st.hostname) ? st.hostname : (sn && sn.system ? sn.system.hostname : "Unknown");
    var health = (st && st.overall_health) ? st.overall_health : "healthy";
    var critCount = st ? (st.critical_count || 0) : 0;
    var warnCount = st ? (st.warning_count || 0) : 0;
    var infoCount = st ? (st.info_count || 0) : 0;
    var lastScan = (st && st.last_scan) ? new Date(st.last_scan).toLocaleString() : "Never";
    var sys = sn ? sn.system : null;
    var healthLabel = esc(health.charAt(0).toUpperCase() + health.slice(1));

    var h = "";

    // Header
    h += '<header class="header fade-in">';
    h += '<div class="header-left">';
    h += '<a href="/" class="logo" style="text-decoration:none;color:inherit"><img src="/icon.png" alt="" style="width:24px;height:24px;border-radius:4px;vertical-align:middle;margin-right:8px;">NAS Doctor</a>';
    h += '<span class="hostname">' + esc(hostname) + '</span>';
    h += '</div>';
    h += '<div class="header-right">';
    h += '<div class="nav-links">';
    h += '<a href="/api/v1/report" class="nav-link" target="_blank">Export Report</a>';
    h += '<a href="/stats" class="nav-link">Stats</a>';
    h += '<a href="https://github.com/mcdays94/nas-doctor" class="nav-link" target="_blank" title="GitHub"><svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" style="vertical-align:middle"><path d="M12 0C5.37 0 0 5.37 0 12c0 5.3 3.44 9.8 8.2 11.39.6.11.82-.26.82-.58v-2.03c-3.34.73-4.04-1.61-4.04-1.61-.55-1.39-1.34-1.76-1.34-1.76-1.09-.75.08-.73.08-.73 1.2.08 1.84 1.24 1.84 1.24 1.07 1.84 2.81 1.31 3.5 1 .1-.78.42-1.31.76-1.61-2.67-.3-5.47-1.33-5.47-5.93 0-1.31.47-2.38 1.24-3.22-.13-.3-.54-1.52.12-3.18 0 0 1.01-.32 3.3 1.23a11.5 11.5 0 0 1 6.02 0c2.28-1.55 3.29-1.23 3.29-1.23.66 1.66.25 2.88.12 3.18.77.84 1.24 1.91 1.24 3.22 0 4.61-2.81 5.63-5.48 5.92.43.37.81 1.1.81 2.22v3.29c0 .32.22.7.82.58C20.56 21.8 24 17.3 24 12c0-6.63-5.37-12-12-12z"/></svg></a>';
    h += '<a href="/settings" class="nav-link" title="Settings"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="vertical-align:middle"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg></a>';
    h += '</div>';
    h += '</div>';
    h += '</header>';

    // Top bar — compact health pill + inline stats
    h += '<div class="top-bar fade-in">';
    h += '<div class="top-bar-health">';
    h += '<div class="health-pill ' + esc(health) + '">';
    h += '<span class="health-pill-dot"></span>';
    h += healthLabel;
    h += '</div>';
    h += '<div class="health-badges">';
    if (critCount > 0) h += '<span class="health-badge badge-red">' + critCount + '&#x1F534;</span>';
    if (warnCount > 0) h += '<span class="health-badge badge-amber">' + warnCount + '&#x1F7E1;</span>';
    if (infoCount > 0) h += '<span class="health-badge badge-brand">' + infoCount + '&#x1F535;</span>';
    h += '</div>';
    h += '</div>';
    h += '<div class="top-bar-stats">';
    if (sys) {
      var cpu = sys.cpu_usage_percent || 0;
      var mem = sys.mem_percent || 0;
      var io = sys.io_wait_percent || 0;
      var uptime = formatUptime(sys.uptime_seconds || (st ? st.uptime : null));
      h += '<div class="stat-item"><span class="stat-item-label">CPU</span><span class="stat-item-value ' + classForPct(cpu) + '">' + cpu.toFixed(1) + '%</span><canvas id="spark-cpu" width="48" height="20" style="margin-left:4px;vertical-align:middle"></canvas></div>';
      h += '<div class="stat-item"><span class="stat-item-label">Mem</span><span class="stat-item-value ' + classForPct(mem) + '">' + mem.toFixed(1) + '%</span><canvas id="spark-mem" width="48" height="20" style="margin-left:4px;vertical-align:middle"></canvas></div>';
      h += '<div class="stat-item"><span class="stat-item-label">I/O</span><span class="stat-item-value ' + classForPct(io > 20 ? 90 : io > 10 ? 75 : 0) + '">' + io.toFixed(1) + '%</span><canvas id="spark-io" width="48" height="20" style="margin-left:4px;vertical-align:middle"></canvas></div>';
      h += '<div class="stat-item"><span class="stat-item-label">Up</span><span class="stat-item-value" style="color:var(--text-primary)">' + esc(uptime) + '</span></div>';
    }
    h += '</div>';
    h += '</div>';

    // OS version banner (always shows platform + version, highlights if update available)
    var upd = sn ? sn.update : null;
    var sysPlat = (sn && sn.system) ? sn.system.platform : null;
    var sysVer = (sn && sn.system) ? sn.system.platform_version : null;
    if (sysPlat && sysVer) {
      var hasBanner = upd && upd.update_available;
      var bgStyle = hasBanner ? 'background:rgba(94,106,210,0.08);border:1px solid rgba(94,106,210,0.2)' : 'background:var(--bg-elevated);border:1px solid var(--border)';
      h += '<div class="fade-in" style="display:flex;align-items:center;gap:8px;padding:8px 12px;' + bgStyle + ';border-radius:var(--radius);margin-bottom:8px;font-size:12px">';
      h += '<span style="color:var(--text-primary);font-weight:600">' + esc(sysPlat) + '</span>';
      h += '<span style="color:var(--text-tertiary)">v' + esc(sysVer) + '</span>';
      if (hasBanner) {
        h += '<span style="color:var(--accent);font-weight:600;margin-left:8px">Update → ' + esc(upd.latest_version) + '</span>';
        if (upd.release_url) h += '<a href="' + esc(upd.release_url) + '" target="_blank" style="color:var(--accent);text-decoration:underline;margin-left:auto">Release notes</a>';
      }
      h += '</div>';
    }

    // Scan bar — compact
    h += '<div class="scan-bar fade-in">';
    h += '<div class="scan-info">Last scan: <strong>' + esc(lastScan) + '</strong></div>';
    if (scanInProgress || (st && st.scan_running)) {
      h += '<button class="ghost-btn scanning" disabled><span class="spinner"></span> Scanning...</button>';
    } else {
      h += '<button class="ghost-btn" onclick="window._triggerScan()">Run Scan</button>';
    }
    h += '</div>';

    // Two-column layout — sections auto-distributed by height
    h += '<div class="two-col fade-in" id="two-col">';
    h += '<div class="col-left" id="col-left"></div>';
    h += '<div class="col-right" id="col-right"></div>';
    h += '</div>';

    // Hidden staging area for sections — JS will measure and distribute
    h += '<div id="section-staging" style="position:absolute;visibility:hidden;width:50%;left:-9999px">';

    // ======= Section: Findings =======
    h += '<div class="section-block" data-section="findings">';
    var findings = sn ? (sn.findings || []) : [];
    h += '<div class="section-title">Findings (' + findings.length + ')</div>';
    if (findings.length === 0) {
      h += '<div class="empty"><div class="empty-icon">&#9989;</div>No findings yet. Run a scan to check your NAS health.</div>';
    } else {
      h += '<div class="findings-list">';
      var sevOrder = { critical: 0, warning: 1, info: 2, ok: 3 };
      findings.sort(function(a, b) { return (sevOrder[a.severity] || 9) - (sevOrder[b.severity] || 9); });
      // Filter dismissed findings
      var dismissed = (statusData && statusData.dismissed_findings) ? statusData.dismissed_findings : [];
      var visibleFindings = findings.filter(function(f) { return dismissed.indexOf(f.title) === -1; });
      for (var fi = 0; fi < visibleFindings.length; fi++) {
        var f = visibleFindings[fi];
        var sev = esc(f.severity);
        h += '<div class="finding finding-' + sev + '" onclick="window._toggleFinding(this)">';
        h += '<div style="display:flex;align-items:center;gap:6px;margin-bottom:2px">';
        h += '<span class="sev-dot sev-dot-' + sev + '"></span>';
        h += '<span class="finding-tag sev-' + sev + '">' + sev + '</span>';
        h += '<span class="finding-title">' + esc(f.title) + '</span>';
        h += '</div>';
        h += '<div class="finding-expandable">';
        h += '<div class="finding-details">';
        h += '<div class="finding-desc">' + esc(f.description) + '</div>';
        if (f.evidence && f.evidence.length > 0) {
          h += '<div class="finding-detail-row"><div class="finding-detail-label">Evidence</div><div class="finding-detail-value"><ul class="finding-evidence-list">';
          for (var ei = 0; ei < f.evidence.length; ei++) {
            h += '<li>' + esc(f.evidence[ei]) + '</li>';
          }
          h += '</ul></div></div>';
        }
        if (f.action) h += '<div class="finding-detail-row"><div class="finding-detail-label">Action</div><div class="finding-detail-value val-accent">' + esc(f.action) + '</div></div>';
        if (f.impact) h += '<div class="finding-detail-row"><div class="finding-detail-label">Impact</div><div class="finding-detail-value val-italic">' + esc(f.impact) + '</div></div>';
        h += '<div class="finding-meta">';
        if (f.detected_at) h += '<span><strong>Detected:</strong> ' + new Date(f.detected_at).toLocaleString() + '</span>';
        if (f.priority) h += '<span><strong>Priority:</strong> ' + esc(f.priority) + '</span>';
        if (f.cost) h += '<span><strong>Cost:</strong> ' + esc(f.cost) + '</span>';
        if (f.category) h += '<span><strong>Category:</strong> ' + esc(f.category) + '</span>';
        h += '<span style="margin-left:auto"><a href="#" onclick="event.stopPropagation();window._dismissFinding(\'' + esc(f.title).replace(/'/g, "\\'") + '\');return false" style="font-size:11px;color:var(--text-quaternary);text-decoration:none">Dismiss</a></span>';
        h += '</div>';
        h += '</div>';
        h += '</div>';
        h += '</div>';
      }
      h += '</div>';
    }
    h += '</div>'; // end section-block findings

    // ======= Section: Disk Space =======
    // ======= Section: Drives (SMART-centric like Scrutiny) =======
    h += '<div class="section-block" data-section="drives">';
    var smart = sn ? (sn.smart || []) : [];
    var disks = sn ? (sn.disks || []) : [];
    if (smart.length > 0 || disks.length > 0) {
      h += '<div>';
      h += '<div class="section-title">Drives (' + (smart.length || disks.length) + ')</div>';

      if (smart.length > 0) {
        for (var si = 0; si < smart.length; si++) {
          var s = smart[si];
          var healthDot = s.health_passed ? "running" : "exited";
          var tempClass = (s.temperature_c || 0) >= 55 ? "td-crit" : (s.temperature_c || 0) >= 45 ? "td-warn" : "td-healthy";
          var sizeStr = s.size_gb >= 1000 ? (s.size_gb / 1000).toFixed(1) + ' TB' : (s.size_gb || 0).toFixed(0) + ' GB';
          var ageStr = s.power_on_hours > 8766 ? (s.power_on_hours / 8766).toFixed(1) + 'y' : (s.power_on_hours || 0).toLocaleString() + 'h';
          var slotLabel = s.array_slot || '';

          h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:10px 12px;margin-bottom:6px;cursor:pointer" onclick="window.location=\'/disk/' + encodeURIComponent(s.serial || '') + '\'">';
          h += '<div style="display:flex;align-items:center;gap:10px;flex-wrap:wrap">';
          h += '<span class="status-dot ' + healthDot + '"></span>';
          h += '<span style="font-weight:600;font-size:13px;min-width:55px">' + esc(s.device) + '</span>';
          if (slotLabel) h += '<span style="font-size:11px;color:var(--text-quaternary);background:var(--bg-elevated);padding:1px 6px;border-radius:4px">' + esc(slotLabel) + '</span>';
          h += '<span style="font-size:12px;color:var(--text-tertiary);flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(s.model) + '</span>';
          h += '<span style="font-size:12px;color:var(--text-tertiary)">' + sizeStr + '</span>';
          h += '<span class="' + tempClass + '" style="font-size:12px;font-weight:600">' + (s.temperature_c || 0) + '&deg;C</span>';
          h += '<span style="font-size:11px;color:var(--text-quaternary)">' + ageStr + '</span>';
          h += '<canvas id="spark-temp-' + si + '" width="60" height="20" style="flex-shrink:0"></canvas>';
          if (!s.health_passed) h += '<span style="font-size:10px;font-weight:600;color:var(--red);background:rgba(220,38,38,0.1);padding:1px 6px;border-radius:9999px">FAILED</span>';
          if (s.reallocated_sectors > 0) h += '<span style="font-size:10px;color:var(--amber);background:rgba(217,119,6,0.1);padding:1px 6px;border-radius:9999px">' + s.reallocated_sectors + ' realloc</span>';
          if (s.pending_sectors > 0) h += '<span style="font-size:10px;color:var(--red);background:rgba(220,38,38,0.1);padding:1px 6px;border-radius:9999px">' + s.pending_sectors + ' pending</span>';
          if (s.udma_crc_errors > 0) h += '<span style="font-size:10px;color:var(--amber);background:rgba(217,119,6,0.1);padding:1px 6px;border-radius:9999px">' + s.udma_crc_errors + ' CRC</span>';
          h += '</div>';
          h += '</div>';
        }
      }

      // Storage volumes (disk space from df)
      if (disks.length > 0) {
        h += '<div style="margin-top:10px;font-size:11px;color:var(--text-quaternary);text-transform:uppercase;letter-spacing:0.5px;margin-bottom:4px">Storage</div>';
        for (var di = 0; di < disks.length; di++) {
          var dk = disks[di];
          var pct = dk.used_percent || 0;
          h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:8px 12px;margin-bottom:4px">';
          h += '<div style="display:flex;justify-content:space-between;font-size:12px;margin-bottom:2px">';
          h += '<span style="font-weight:500">' + esc(dk.label || dk.mount_point) + '</span>';
          h += '<span style="color:var(--text-tertiary)">' + (dk.used_gb || 0).toFixed(0) + ' / ' + (dk.total_gb || 0).toFixed(0) + ' GB (' + pct.toFixed(0) + '%)</span>';
          h += '</div>';
          h += '<div class="disk-bar-bg" style="height:4px"><div class="disk-bar-fill" style="height:4px;width:' + pct.toFixed(1) + '%;background:' + colorForPct(pct) + '"></div></div>';
          h += '</div>';
        }
      }
      h += '</div>';
    }
    h += '</div>'; // end section-block drives

    // ======= Section: Docker =======
    h += '<div class="section-block" data-section="docker">';
    var docker = sn ? sn.docker : null;
    if (docker && docker.available && docker.containers && docker.containers.length > 0) {
      var containers = docker.containers;
      h += '<div>';
      h += '<div class="section-title">Docker Containers (' + containers.length + ')</div>';
      h += '<div class="table-wrap">';
      h += '<table><thead><tr>';
      h += '<th>Name</th><th>Image</th><th>Status</th><th>CPU</th><th>Memory</th><th>Uptime</th>';
      h += '</tr></thead><tbody>';
      for (var ci = 0; ci < containers.length; ci++) {
        var c = containers[ci];
        var stateClass = c.state === "running" ? "running" : (c.state === "paused" ? "paused" : "exited");
        h += '<tr>';
        h += '<td><span class="status-dot ' + stateClass + '"></span>' + esc(c.name) + '</td>';
        h += '<td style="color:var(--text-quaternary);max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(c.image) + '</td>';
        h += '<td>' + esc(c.status) + '</td>';
        h += '<td>' + (c.cpu_percent || 0).toFixed(1) + '%</td>';
        h += '<td>' + (c.mem_mb || 0).toFixed(0) + ' MB (' + (c.mem_percent || 0).toFixed(1) + '%)</td>';
        h += '<td>' + esc(c.uptime || "N/A") + '</td>';
        h += '</tr>';
      }
      h += '</tbody></table></div>';
      h += '</div>';
    }

    h += '</div>'; // end section-block docker

    // ======= Section: ZFS Pools =======
    h += '<div class="section-block" data-section="zfs">';
    var zfs = sn ? sn.zfs : null;
    if (zfs && zfs.available && zfs.pools && zfs.pools.length > 0) {
      h += '<div>';
      h += '<div class="section-title">ZFS Pools</div>';
      for (var zi = 0; zi < zfs.pools.length; zi++) {
        var zp = zfs.pools[zi];
        var poolStateClass = zp.state === "ONLINE" ? "td-healthy" : zp.state === "DEGRADED" ? "td-warn" : "td-crit";
        h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:12px;margin-bottom:8px">';
        h += '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">';
        h += '<span style="font-weight:600;font-size:13px;color:var(--text-primary)">' + esc(zp.name) + '</span>';
        h += '<span class="' + poolStateClass + '" style="font-weight:600;font-size:11px;text-transform:uppercase">' + esc(zp.state) + '</span>';
        h += '</div>';
        h += '<div style="font-size:12px;color:var(--text-tertiary);margin-bottom:6px">';
        h += esc((zp.used_gb || 0).toFixed(0)) + ' / ' + esc((zp.total_gb || 0).toFixed(0)) + ' GB (' + (zp.used_percent || 0).toFixed(0) + '%) &middot; Frag: ' + (zp.fragmentation_percent || 0) + '%';
        h += '</div>';
        if (zp.scan_type && zp.scan_type !== "none") {
          h += '<div style="font-size:11px;color:var(--text-quaternary)">Last ' + esc(zp.scan_type) + ': ' + (zp.scan_errors || 0) + ' errors</div>';
        }
        // VDev tree
        if (zp.vdevs && zp.vdevs.length > 0) {
          h += '<div style="margin-top:8px;font-size:11px;font-family:monospace;color:var(--text-tertiary)">';
          for (var vi = 0; vi < zp.vdevs.length; vi++) {
            var vd = zp.vdevs[vi];
            var vdClass = vd.state === "ONLINE" ? "td-healthy" : vd.state === "DEGRADED" ? "td-warn" : "td-crit";
            h += '<div>' + esc(vd.name) + ' <span class="' + vdClass + '">' + esc(vd.state) + '</span></div>';
            if (vd.children) {
              for (var ci = 0; ci < vd.children.length; ci++) {
                var ch = vd.children[ci];
                var chClass = ch.state === "ONLINE" ? "td-healthy" : ch.state === "DEGRADED" ? "td-warn" : "td-crit";
                h += '<div style="padding-left:16px">' + esc(ch.name) + ' <span class="' + chClass + '">' + esc(ch.state) + '</span></div>';
              }
            }
          }
          h += '</div>';
        }
        h += '</div>';
      }
      // ARC stats
      if (zfs.arc) {
        h += '<div style="font-size:11px;color:var(--text-quaternary);margin-top:6px">ARC: ' + (zfs.arc.size_mb / 1024).toFixed(1) + ' GB / ' + (zfs.arc.max_size_mb / 1024).toFixed(1) + ' GB &middot; Hit rate: ' + (zfs.arc.hit_rate_percent || 0).toFixed(1) + '%</div>';
      }
      h += '</div>';
    }

    h += '</div>'; // end section-block zfs

    // ======= Section: UPS =======
    h += '<div class="section-block" data-section="ups">';
    var ups = sn ? sn.ups : null;
    if (ups && ups.available) {
      h += '<div>';
      h += '<div class="section-title">UPS / Power</div>';
      var upsStateClass = ups.on_battery ? "td-crit" : "td-healthy";
      h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:12px">';
      h += '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px">';
      h += '<span style="font-weight:600;font-size:13px;color:var(--text-primary)">' + esc(ups.name || ups.model) + '</span>';
      h += '<span class="' + upsStateClass + '" style="font-weight:600;font-size:11px;text-transform:uppercase">' + esc(ups.status_human) + '</span>';
      h += '</div>';
      h += '<div style="display:flex;gap:16px;font-size:12px;color:var(--text-tertiary);flex-wrap:wrap">';
      h += '<span>Battery: <strong style="color:var(--text-primary)">' + (ups.battery_percent || 0).toFixed(0) + '%</strong></span>';
      h += '<span>Load: <strong style="color:var(--text-primary)">' + (ups.load_percent || 0).toFixed(0) + '%</strong></span>';
      h += '<span>Runtime: <strong style="color:var(--text-primary)">' + (ups.runtime_minutes || 0).toFixed(0) + ' min</strong></span>';
      if (ups.wattage_watts > 0) h += '<span>Power: <strong style="color:var(--text-primary)">' + (ups.wattage_watts || 0).toFixed(0) + 'W / ' + (ups.nominal_watts || 0).toFixed(0) + 'W</strong></span>';
      if (ups.input_voltage > 0) h += '<span>Input: ' + (ups.input_voltage || 0).toFixed(0) + 'V</span>';
      h += '</div>';
      if (ups.last_transfer) h += '<div style="font-size:11px;color:var(--text-quaternary);margin-top:4px">Last transfer: ' + esc(ups.last_transfer) + '</div>';
      h += '</div>';
      h += '</div>';
    }
    h += '</div>'; // end section-block ups

    // ======= Section: Network =======
    h += '<div class="section-block" data-section="network">';
    var net = sn ? sn.network : null;
    if (net && net.interfaces && net.interfaces.length > 0) {
      h += '<div>';
      h += '<div class="section-title">Network</div>';
      h += '<table style="width:100%;font-size:12px;border-collapse:collapse">';
      h += '<tr style="color:var(--text-quaternary);font-size:10px;text-transform:uppercase;letter-spacing:0.5px">';
      h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Interface</th>';
      h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">State</th>';
      h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Speed</th>';
      h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">MTU</th>';
      h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">IP</th></tr>';
      for (var ni = 0; ni < net.interfaces.length; ni++) {
        var iface = net.interfaces[ni];
        var stateColor = iface.state === "UP" ? "td-healthy" : "td-warn";
        h += '<tr><td style="padding:5px 8px;border-bottom:1px solid var(--border)">' + esc(iface.name) + '</td>';
        h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)" class="' + stateColor + '">' + esc(iface.state) + '</td>';
        h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)">' + esc(iface.speed || "—") + '</td>';
        h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)">' + (iface.mtu || 0) + '</td>';
        h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)">' + esc(iface.ipv4 || "—") + '</td></tr>';
      }
      h += '</table>';
      h += '</div>';
    }
    h += '</div>'; // end section-block network

    // ======= Section: Parity =======
    h += '<div class="section-block" data-section="parity">';
    var parity = sn ? sn.parity : null;
    if (parity && parity.history && parity.history.length > 0) {
      h += '<div>';
      h += '<div class="section-title">Parity History</div>';
      h += '<div style="font-size:12px;color:var(--text-tertiary);margin-bottom:8px">Status: ' + esc(parity.status || "idle") + '</div>';
      h += '<div style="max-height:200px;overflow-y:auto;scrollbar-width:thin">';
      h += '<table style="width:100%;font-size:12px;border-collapse:collapse">';
      h += '<tr style="color:var(--text-quaternary);font-size:10px;text-transform:uppercase;letter-spacing:0.5px">';
      h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Date</th>';
      h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Duration</th>';
      h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Speed</th>';
      h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Errors</th></tr>';
      for (var pi = 0; pi < parity.history.length; pi++) {
        var pc = parity.history[pi];
        var errClass = pc.errors > 0 ? "td-crit" : "td-healthy";
        h += '<tr><td style="padding:5px 8px;border-bottom:1px solid var(--border)">' + esc(pc.date) + '</td>';
        h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)">' + esc(pc.duration_human || "—") + '</td>';
        h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)">' + (pc.speed_mb_s || 0).toFixed(1) + ' MB/s</td>';
        h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)" class="' + errClass + '">' + (pc.errors || 0) + '</td></tr>';
      }
      h += '</table>';
      h += '</div>'; // scroll container
      h += '</div>';
    } else if (parity && parity.status) {
      h += '<div>';
      h += '<div class="section-title">Parity</div>';
      h += '<div style="font-size:12px;color:var(--text-tertiary)">Status: ' + esc(parity.status) + ' &middot; No parity check history found</div>';
      h += '</div>';
    }
    h += '</div>'; // end section-block parity

    h += '</div>'; // end section-staging

    // Footer
    h += '<div style="text-align:center;padding:' + 'calc(var(--sp)*4) 0;color:var(--text-quaternary);font-size:12px" class="fade-in">';
    h += 'NAS Doctor &middot; Auto-refreshes every 30s';
    h += '</div>';

    document.getElementById("app").innerHTML = h;

    // Auto-distribute sections across two columns for balanced height
    distributeSections();

    // Render sparklines after DOM is updated
    renderSparklines();
  }

  function renderSparklines() {
    fetch("/api/v1/sparklines")
      .then(function(r) { return r.json(); })
      .then(function(data) {
        // System sparklines
        if (data.system && data.system.length >= 2 && window.NasChart) {
          var cpuData = data.system.map(function(p) { return p.cpu_usage; });
          var memData = data.system.map(function(p) { return p.mem_percent; });
          var ioData = data.system.map(function(p) { return p.io_wait; });
          try { NasChart.sparkline("spark-cpu", { data: cpuData, color: "#5e6ad2", width: 48, height: 20 }); } catch(e) {}
          try { NasChart.sparkline("spark-mem", { data: memData, color: "#7170ff", width: 48, height: 20 }); } catch(e) {}
          try { NasChart.sparkline("spark-io", { data: ioData, color: "#f59e0b", width: 48, height: 20 }); } catch(e) {}
        }
        // SMART temperature sparklines
        if (data.disks && window.NasChart) {
          var smart = cachedSnapshot ? (cachedSnapshot.smart || []) : [];
          for (var i = 0; i < smart.length; i++) {
            var serial = smart[i].serial || "";
            var diskData = null;
            for (var d = 0; d < data.disks.length; d++) {
              if (data.disks[d].serial === serial) { diskData = data.disks[d]; break; }
            }
            if (diskData && diskData.temps && diskData.temps.length >= 2) {
              var temps = diskData.temps.map(function(p) { return p.temp; });
              var maxT = Math.max.apply(null, temps);
              var color = maxT >= 55 ? "#ef4444" : maxT >= 45 ? "#f59e0b" : "#22c55e";
              try { NasChart.sparkline("spark-temp-" + i, { data: temps, color: color, width: 70, height: 24 }); } catch(e) {}
            }
          }
        }
      })
      .catch(function() {});
  }

  function distributeSections() {
    var staging = document.getElementById("section-staging");
    var colL = document.getElementById("col-left");
    var colR = document.getElementById("col-right");
    if (!staging || !colL || !colR) return;

    // Get section visibility from status response
    var sec = (statusData && statusData.sections) ? statusData.sections : {};
    var sectionMap = {
      "findings": sec.findings !== false,
      "drives": sec.disk_space !== false || sec.smart !== false,
      "docker": sec.docker !== false,
      "zfs": sec.zfs !== false,
      "ups": sec.ups !== false,
      "network": sec.network !== false,
      "parity": sec.parity !== false
    };

    var blocks = staging.querySelectorAll(".section-block");
    if (blocks.length === 0) return;

    var items = [];
    for (var i = 0; i < blocks.length; i++) {
      var name = blocks[i].getAttribute("data-section");
      if (sectionMap[name] === false) continue; // skip hidden sections
      if (blocks[i].offsetHeight < 10) continue; // skip empty sections
      items.push({ el: blocks[i], h: blocks[i].offsetHeight });
    }

    var leftH = 0, rightH = 0;
    for (var j = 0; j < items.length; j++) {
      if (leftH <= rightH) {
        colL.appendChild(items[j].el);
        leftH += items[j].h;
      } else {
        colR.appendChild(items[j].el);
        rightH += items[j].h;
      }
    }

    staging.parentNode.removeChild(staging);
  }

  window._dismissFinding = function(title) {
    fetch("/api/v1/findings/dismiss", { method: "POST", headers: {"Content-Type":"application/json"}, body: JSON.stringify({title: title}) })
      .then(function() { loadAll(); })
      .catch(function() {});
  };

  window._toggleFinding = function(el) {
    var all = document.querySelectorAll(".finding");
    for (var i = 0; i < all.length; i++) {
      if (all[i] !== el) all[i].classList.remove("active");
    }
    el.classList.toggle("active");
  };

  window._triggerScan = triggerScan;

  // Initial load
  loadAll().then(startRefresh);
})();
</script>
</body>
</html>`
