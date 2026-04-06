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
.header{display:flex;align-items:center;justify-content:space-between;padding:calc(var(--sp)*2) 0;margin-bottom:calc(var(--sp)*4);border-bottom:1px solid var(--border)}
.header-left{display:flex;align-items:center;gap:calc(var(--sp)*2)}
.logo{display:flex;align-items:center;gap:var(--sp);font-size:20px;font-weight:600;letter-spacing:-0.5px;color:var(--text-primary)}
.logo-emoji{font-size:24px}
.hostname{font-size:13px;color:var(--text-tertiary);font-weight:400;padding:4px 10px;background:var(--btn-bg);border:1px solid var(--border);border-radius:var(--radius)}
.theme-nav{display:flex;gap:4px}
.theme-link{padding:6px 12px;border-radius:var(--radius);font-size:12px;font-weight:500;color:var(--text-tertiary);border:1px solid transparent;transition:all 0.15s ease}
.theme-link:hover{color:var(--text-secondary);background:var(--btn-bg-hover);border-color:var(--border-hover)}
.theme-link.active{color:var(--accent);background:rgba(94,106,210,0.08);border-color:rgba(94,106,210,0.2)}

/* Section titles */
.section-title{font-size:13px;font-weight:600;color:var(--text-tertiary);text-transform:uppercase;letter-spacing:0.5px;margin-bottom:calc(var(--sp)*2);margin-top:calc(var(--sp)*4)}
.section-title:first-of-type{margin-top:0}

/* Health overview card */
.health-card{background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:calc(var(--sp)*4);margin-bottom:calc(var(--sp)*4);display:flex;align-items:center;justify-content:space-between;gap:calc(var(--sp)*4)}
.health-main{display:flex;align-items:center;gap:calc(var(--sp)*3)}
.health-dot{width:48px;height:48px;border-radius:50%;display:flex;align-items:center;justify-content:center;font-size:24px;flex-shrink:0}
.health-dot.healthy{background:var(--green-bg);color:var(--green)}
.health-dot.warning{background:var(--amber-bg);color:var(--amber)}
.health-dot.critical{background:var(--red-bg);color:var(--red)}
.health-label{font-size:32px;font-weight:600;letter-spacing:-1px}
.health-label.healthy{color:var(--green)}
.health-label.warning{color:var(--amber)}
.health-label.critical{color:var(--red)}
.health-sub{font-size:13px;color:var(--text-tertiary);margin-top:2px}
.health-counts{display:flex;gap:calc(var(--sp)*3)}
.health-count{text-align:center}
.health-count-num{font-size:24px;font-weight:600;letter-spacing:-0.5px}
.health-count-label{font-size:11px;color:var(--text-quaternary);text-transform:uppercase;letter-spacing:0.3px;margin-top:2px}

/* Stat cards grid */
.stats-grid{display:grid;grid-template-columns:repeat(4,1fr);gap:calc(var(--sp)*2);margin-bottom:calc(var(--sp)*4)}
@media(max-width:900px){.stats-grid{grid-template-columns:repeat(2,1fr)}}
@media(max-width:500px){.stats-grid{grid-template-columns:1fr}}
.stat-card{background:var(--btn-bg);border:1px solid var(--border-hover);border-radius:var(--radius);padding:calc(var(--sp)*2.5)}
.stat-label{font-size:12px;font-weight:500;color:var(--text-quaternary);text-transform:uppercase;letter-spacing:0.3px;margin-bottom:var(--sp)}
.stat-value{font-size:28px;font-weight:600;letter-spacing:-1px}
.stat-sub{font-size:12px;color:var(--text-quaternary);margin-top:4px}

/* Findings — Luminance-Stepped Status Tiles */
.findings-list{display:flex;flex-direction:column;gap:calc(var(--sp)*1);margin-bottom:calc(var(--sp)*4)}
.finding{border:1px solid rgba(255,255,255,0.06);border-radius:8px;padding:16px;margin-bottom:8px;transition:all 200ms ease}
.finding:hover{border-color:rgba(255,255,255,0.10)}
.finding-critical{background:rgba(220,38,38,0.06)}
.finding-critical:hover{background:rgba(220,38,38,0.10)}
.finding-warning{background:rgba(217,119,6,0.06)}
.finding-warning:hover{background:rgba(217,119,6,0.10)}
.finding-info{background:rgba(94,106,210,0.06)}
.finding-info:hover{background:rgba(94,106,210,0.10)}
.finding-ok{background:rgba(16,185,129,0.06)}
.finding-ok:hover{background:rgba(16,185,129,0.10)}
.sev-dot{display:inline-block;width:8px;height:8px;border-radius:50%;margin-right:8px;vertical-align:middle;flex-shrink:0}
.sev-dot-critical{background:#dc2626}
.sev-dot-warning{background:#d97706}
.sev-dot-info{background:#5e6ad2}
.sev-dot-ok{background:#10b981}
.finding-title{font-size:14px;font-weight:600;color:var(--text-primary)}
.finding-desc{font-size:13px;color:var(--text-secondary);margin-bottom:8px;line-height:1.5}
.finding-action{font-size:13px;color:var(--accent);margin-bottom:8px}
.finding-meta{display:flex;flex-wrap:wrap;gap:calc(var(--sp)*2);font-size:12px;color:var(--text-quaternary)}
.finding-meta span{display:flex;align-items:center;gap:4px}
.finding-meta strong{color:var(--text-tertiary);font-weight:500}
.finding-tag{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:0.5px;padding:2px 8px;border-radius:4px}
.finding-tag.sev-critical{color:var(--red);background:var(--red-bg)}
.finding-tag.sev-warning{color:var(--amber);background:var(--amber-bg)}
.finding-tag.sev-info{color:var(--accent);background:rgba(94,106,210,0.1)}
.finding-tag.sev-ok{color:var(--green);background:var(--green-bg)}

/* Tables */
.table-wrap{background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);overflow:hidden;margin-bottom:calc(var(--sp)*4)}
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

/* Disk bars */
.disk-list{display:flex;flex-direction:column;gap:calc(var(--sp)*1.5);margin-bottom:calc(var(--sp)*4)}
.disk-item{background:var(--bg-panel);border:1px solid var(--border);border-radius:var(--radius);padding:calc(var(--sp)*2)}
.disk-header{display:flex;justify-content:space-between;align-items:baseline;margin-bottom:var(--sp)}
.disk-name{font-size:13px;font-weight:600;color:var(--text-primary)}
.disk-info{font-size:12px;color:var(--text-quaternary)}
.disk-bar-bg{height:6px;background:var(--bg-elevated);border-radius:3px;overflow:hidden}
.disk-bar-fill{height:100%;border-radius:3px;transition:width 0.5s ease}

/* Ghost button */
.ghost-btn{display:inline-flex;align-items:center;gap:var(--sp);padding:8px 16px;background:var(--btn-bg);border:1px solid var(--border-hover);border-radius:var(--radius);color:var(--text-secondary);font-family:inherit;font-size:13px;font-weight:500;cursor:pointer;transition:all 0.15s ease}
.ghost-btn:hover{background:var(--btn-bg-hover);border-color:rgba(255,255,255,0.12);color:var(--text-primary)}
.ghost-btn:active{transform:scale(0.98)}
.ghost-btn:disabled{opacity:0.4;cursor:not-allowed;transform:none}
.ghost-btn.scanning{color:var(--accent)}

/* Scan bar */
.scan-bar{display:flex;align-items:center;justify-content:space-between;margin-bottom:calc(var(--sp)*4);gap:calc(var(--sp)*2)}
.scan-info{font-size:12px;color:var(--text-quaternary)}
.scan-info strong{color:var(--text-tertiary);font-weight:500}

/* Empty state */
.empty{text-align:center;padding:calc(var(--sp)*8);color:var(--text-quaternary);font-size:14px}
.empty-icon{font-size:32px;margin-bottom:calc(var(--sp)*2);opacity:0.5}

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

<script>
(function(){
  "use strict";

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

  function healthIcon(h) {
    if (h === "critical") return "&#10060;";
    if (h === "warning") return "&#9888;&#65039;";
    return "&#10003;";
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
    var hostname = (st && st.hostname) ? st.hostname : (sn && sn.system ? sn.system.hostname : "Unknown");
    var health = (st && st.overall_health) ? st.overall_health : "healthy";
    var critCount = st ? (st.critical_count || 0) : 0;
    var warnCount = st ? (st.warning_count || 0) : 0;
    var infoCount = st ? (st.info_count || 0) : 0;
    var lastScan = (st && st.last_scan) ? new Date(st.last_scan).toLocaleString() : "Never";
    var sys = sn ? sn.system : null;

    var h = "";

    // Header
    h += '<header class="header fade-in">';
    h += '<div class="header-left">';
    h += '<div class="logo"><img src="/icon.png" alt="" style="width:24px;height:24px;border-radius:4px;vertical-align:middle;margin-right:8px;">NAS Doctor</div>';
    h += '<span class="hostname">' + esc(hostname) + '</span>';
    h += '</div>';
    h += '<nav class="theme-nav">';
    h += '<a href="/" class="theme-link active">Midnight</a>';
    h += '<a href="/theme/clean" class="theme-link">Clean</a>';
    h += '<a href="/theme/ember" class="theme-link">Ember</a>';
    h += '<a href="/api/v1/report" class="theme-link" target="_blank">Export Report</a>';
    h += '<a href="/settings" class="theme-link">Settings</a>';
    h += '</nav>';
    h += '</header>';

    // Scan bar
    h += '<div class="scan-bar fade-in">';
    h += '<div class="scan-info">Last scan: <strong>' + esc(lastScan) + '</strong></div>';
    if (scanInProgress || (st && st.scan_running)) {
      h += '<button class="ghost-btn scanning" disabled><span class="spinner"></span> Scanning...</button>';
    } else {
      h += '<button class="ghost-btn" onclick="window._triggerScan()">Run Scan</button>';
    }
    h += '</div>';

    // Health overview
    h += '<div class="health-card fade-in">';
    h += '<div class="health-main">';
    h += '<div class="health-dot ' + esc(health) + '">' + healthIcon(health) + '</div>';
    h += '<div>';
    h += '<div class="health-label ' + esc(health) + '">' + esc(health.charAt(0).toUpperCase() + health.slice(1)) + '</div>';
    h += '<div class="health-sub">Overall system health</div>';
    h += '</div>';
    h += '</div>';
    h += '<div class="health-counts">';
    h += '<div class="health-count"><div class="health-count-num text-red">' + critCount + '</div><div class="health-count-label">Critical</div></div>';
    h += '<div class="health-count"><div class="health-count-num text-amber">' + warnCount + '</div><div class="health-count-label">Warning</div></div>';
    h += '<div class="health-count"><div class="health-count-num text-brand">' + infoCount + '</div><div class="health-count-label">Info</div></div>';
    h += '</div>';
    h += '</div>';

    // System stats
    if (sys) {
      h += '<div class="section-title fade-in">System Stats</div>';
      h += '<div class="stats-grid fade-in">';

      // CPU
      var cpu = sys.cpu_usage_percent || 0;
      h += '<div class="stat-card">';
      h += '<div class="stat-label">CPU Usage</div>';
      h += '<div class="stat-value ' + classForPct(cpu) + '">' + cpu.toFixed(1) + '%</div>';
      h += '<div class="stat-sub">' + esc(sys.cpu_model || "N/A") + ' &middot; ' + (sys.cpu_cores || 0) + ' cores</div>';
      h += '</div>';

      // Memory
      var mem = sys.mem_percent || 0;
      h += '<div class="stat-card">';
      h += '<div class="stat-label">Memory</div>';
      h += '<div class="stat-value ' + classForPct(mem) + '">' + mem.toFixed(1) + '%</div>';
      h += '<div class="stat-sub">' + (sys.mem_used_mb || 0) + ' / ' + (sys.mem_total_mb || 0) + ' MB</div>';
      h += '</div>';

      // IO Wait
      var io = sys.io_wait_percent || 0;
      h += '<div class="stat-card">';
      h += '<div class="stat-label">I/O Wait</div>';
      h += '<div class="stat-value ' + classForPct(io > 20 ? 90 : io > 10 ? 75 : 0) + '">' + io.toFixed(1) + '%</div>';
      h += '<div class="stat-sub">Load ' + ((sys.load_avg_1 || 0).toFixed(2)) + ' / ' + ((sys.load_avg_5 || 0).toFixed(2)) + ' / ' + ((sys.load_avg_15 || 0).toFixed(2)) + '</div>';
      h += '</div>';

      // Uptime
      h += '<div class="stat-card">';
      h += '<div class="stat-label">Uptime</div>';
      h += '<div class="stat-value" style="color:var(--text-primary)">' + esc(formatUptime(sys.uptime_seconds || (st ? st.uptime : null))) + '</div>';
      h += '<div class="stat-sub">' + esc(sys.platform || "") + ' ' + esc(sys.platform_version || "") + '</div>';
      h += '</div>';

      h += '</div>';
    }

    // Findings
    var findings = sn ? (sn.findings || []) : [];
    h += '<div class="section-title fade-in">Findings (' + findings.length + ')</div>';
    if (findings.length === 0) {
      h += '<div class="empty fade-in"><div class="empty-icon">&#9989;</div>No findings yet. Run a scan to check your NAS health.</div>';
    } else {
      h += '<div class="findings-list fade-in">';
      // Sort: critical first, then warning, then info, then ok
      var sevOrder = { critical: 0, warning: 1, info: 2, ok: 3 };
      findings.sort(function(a, b) { return (sevOrder[a.severity] || 9) - (sevOrder[b.severity] || 9); });
      for (var fi = 0; fi < findings.length; fi++) {
        var f = findings[fi];
        var sev = esc(f.severity);
        h += '<div class="finding finding-' + sev + '">';
        h += '<div style="display:flex;align-items:center;margin-bottom:8px">';
        h += '<span class="sev-dot sev-dot-' + sev + '"></span>';
        h += '<span class="finding-title">' + esc(f.title) + '</span>';
        h += '</div>';
        h += '<div class="finding-desc">' + esc(f.description) + '</div>';
        if (f.action) h += '<div class="finding-action">' + esc(f.action) + '</div>';
        h += '<div class="finding-meta">';
        h += '<span class="finding-tag sev-' + sev + '">' + sev + '</span>';
        if (f.priority) h += '<span><strong>Priority:</strong> ' + esc(f.priority) + '</span>';
        if (f.cost) h += '<span><strong>Cost:</strong> ' + esc(f.cost) + '</span>';
        if (f.category) h += '<span><strong>Category:</strong> ' + esc(f.category) + '</span>';
        h += '</div>';
        h += '</div>';
      }
      h += '</div>';
    }

    // Disk space
    var disks = sn ? (sn.disks || []) : [];
    if (disks.length > 0) {
      h += '<div class="section-title fade-in">Disk Space</div>';
      h += '<div class="disk-list fade-in">';
      for (var di = 0; di < disks.length; di++) {
        var d = disks[di];
        var pct = d.used_percent || 0;
        h += '<div class="disk-item">';
        h += '<div class="disk-header">';
        h += '<span class="disk-name">' + esc(d.label || d.mount_point || d.device) + '</span>';
        h += '<span class="disk-info">' + (d.used_gb || 0).toFixed(1) + ' / ' + (d.total_gb || 0).toFixed(1) + ' GB (' + pct.toFixed(1) + '%)</span>';
        h += '</div>';
        h += '<div class="disk-bar-bg"><div class="disk-bar-fill" style="width:' + pct.toFixed(1) + '%;background:' + colorForPct(pct) + '"></div></div>';
        h += '</div>';
      }
      h += '</div>';
    }

    // SMART health table
    var smart = sn ? (sn.smart || []) : [];
    if (smart.length > 0) {
      h += '<div class="section-title fade-in">SMART Health</div>';
      h += '<div class="table-wrap fade-in">';
      h += '<table><thead><tr>';
      h += '<th>Device</th><th>Model</th><th>Health</th><th>Temp</th><th>Reallocated</th><th>Pending</th><th>UDMA CRC</th><th>Power Hours</th>';
      h += '</tr></thead><tbody>';
      for (var si = 0; si < smart.length; si++) {
        var s = smart[si];
        var healthClass = s.health_passed ? "td-healthy" : "td-crit";
        var healthText = s.health_passed ? "PASSED" : "FAILED";
        var tempClass = (s.temperature_c || 0) >= 55 ? "td-crit" : (s.temperature_c || 0) >= 45 ? "td-warn" : "td-healthy";
        var reallocClass = (s.reallocated_sectors || 0) > 0 ? "td-crit" : "td-healthy";
        var pendClass = (s.pending_sectors || 0) > 0 ? "td-warn" : "td-healthy";
        var crcClass = (s.udma_crc_errors || 0) > 0 ? "td-warn" : "td-healthy";
        h += '<tr style="cursor:pointer" onclick="window.location=\'/disk/' + encodeURIComponent(s.serial || '') + '\'">';
        h += '<td>' + esc(s.device) + '</td>';
        h += '<td>' + esc(s.model) + '</td>';
        h += '<td class="' + healthClass + '">' + healthText + '</td>';
        h += '<td class="' + tempClass + '">' + (s.temperature_c || 0) + '&deg;C</td>';
        h += '<td class="' + reallocClass + '">' + (s.reallocated_sectors || 0) + '</td>';
        h += '<td class="' + pendClass + '">' + (s.pending_sectors || 0) + '</td>';
        h += '<td class="' + crcClass + '">' + (s.udma_crc_errors || 0) + '</td>';
        h += '<td>' + (s.power_on_hours || 0).toLocaleString() + 'h</td>';
        h += '</tr>';
      }
      h += '</tbody></table></div>';
    }

    // Docker containers
    var docker = sn ? sn.docker : null;
    if (docker && docker.available && docker.containers && docker.containers.length > 0) {
      var containers = docker.containers;
      h += '<div class="section-title fade-in">Docker Containers (' + containers.length + ')</div>';
      h += '<div class="table-wrap fade-in">';
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
    }

    // Footer
    h += '<div style="text-align:center;padding:' + 'calc(var(--sp)*4) 0;color:var(--text-quaternary);font-size:12px" class="fade-in">';
    h += 'NAS Doctor &middot; Auto-refreshes every 30s';
    h += '</div>';

    document.getElementById("app").innerHTML = h;
  }

  window._triggerScan = triggerScan;

  // Initial load
  loadAll().then(startRefresh);
})();
</script>
</body>
</html>`
