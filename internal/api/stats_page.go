package api

import "net/http"

func (s *Server) handleStatsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(statsPageHTML))
}

const statsPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>NAS Doctor — Drive Health</title>
<link rel="icon" href="/icon.png">
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#0f1011;--surface:#191a1b;--elevated:#242526;--text:#f7f8f8;--text2:#8a8f98;--text3:#5a5f6a;--accent:#5e6ad2;--green:#27a644;--amber:#d97706;--red:#dc2626;--border:rgba(255,255,255,0.08);--radius:10px}
body{font-family:'Inter',system-ui,sans-serif;background:var(--bg);color:var(--text);min-height:100vh;-webkit-font-smoothing:antialiased}
.container{max-width:1200px;margin:0 auto;padding:24px}
.header{display:flex;align-items:center;justify-content:space-between;margin-bottom:28px}
.header-left{display:flex;align-items:center;gap:12px;text-decoration:none;color:inherit}
.header-left img{width:28px;height:28px;border-radius:6px}
.header-title{font-size:20px;font-weight:600;letter-spacing:-0.3px}
.header-sub{font-size:13px;color:var(--text2)}
.back{display:inline-flex;align-items:center;gap:4px;font-size:13px;color:var(--text2);text-decoration:none;padding:6px 12px;border:1px solid var(--border);border-radius:var(--radius);transition:all 0.15s}
.back:hover{color:var(--text);border-color:rgba(255,255,255,0.15)}

/* Summary bar */
.summary{display:flex;gap:12px;margin-bottom:24px;flex-wrap:wrap}
.summary-box{background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:14px 18px;flex:1;min-width:100px;text-align:center}
.summary-val{font-size:22px;font-weight:600}
.summary-label{font-size:11px;color:var(--text2);text-transform:uppercase;letter-spacing:0.5px;margin-top:2px}
.val-green{color:var(--green)}.val-amber{color:var(--amber)}.val-red{color:var(--red)}

/* Drive grid */
.drives{display:grid;grid-template-columns:repeat(auto-fill,minmax(340px,1fr));gap:12px;margin-bottom:32px}
.drive{background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);overflow:hidden;cursor:pointer;transition:border-color 0.15s}
.drive:hover{border-color:rgba(255,255,255,0.15)}
.drive-header{display:flex;align-items:center;gap:12px;padding:16px}
.drive-score{width:48px;height:48px;border-radius:50%;display:flex;align-items:center;justify-content:center;font-size:16px;font-weight:700;flex-shrink:0;border:2px solid}
.score-good{border-color:var(--green);color:var(--green);background:rgba(39,166,68,0.08)}
.score-warn{border-color:var(--amber);color:var(--amber);background:rgba(217,119,6,0.08)}
.score-crit{border-color:var(--red);color:var(--red);background:rgba(220,38,38,0.08)}
.drive-info{flex:1;min-width:0}
.drive-name{font-size:14px;font-weight:600;margin-bottom:2px}
.drive-model{font-size:12px;color:var(--text2);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.drive-badges{display:flex;gap:6px;flex-wrap:wrap;margin-top:4px}
.badge{font-size:10px;font-weight:600;padding:2px 8px;border-radius:9999px;text-transform:uppercase;letter-spacing:0.3px}
.badge-ok{background:rgba(39,166,68,0.1);color:var(--green)}
.badge-warn{background:rgba(217,119,6,0.1);color:var(--amber)}
.badge-crit{background:rgba(220,38,38,0.1);color:var(--red)}
.badge-type{background:rgba(94,106,210,0.1);color:var(--accent)}
.drive-stats{display:flex;gap:16px;padding:0 16px 12px;font-size:12px;color:var(--text2)}
.drive-stats strong{color:var(--text)}
.drive-temp-bar{height:3px;background:var(--border);margin:0 16px 16px}
.drive-temp-fill{height:100%;border-radius:2px}

/* Expanded drive detail */
.drive-detail{display:none;border-top:1px solid var(--border);padding:16px}
.drive.expanded .drive-detail{display:block}
.detail-grid{display:grid;grid-template-columns:1fr 1fr;gap:8px 24px;margin-bottom:16px}
.detail-item{display:flex;justify-content:space-between;font-size:12px;padding:4px 0;border-bottom:1px solid rgba(255,255,255,0.04)}
.detail-label{color:var(--text2)}
.detail-value{font-weight:500;font-family:'SF Mono','Cascadia Code',monospace;font-size:11px}
.detail-value.val-bad{color:var(--red)}.detail-value.val-warn{color:var(--amber)}.detail-value.val-ok{color:var(--green)}

/* SMART table */
.smart-section{margin-bottom:32px}
.smart-section-title{font-size:16px;font-weight:600;margin-bottom:12px}
.smart-table{width:100%;background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);overflow:hidden;border-collapse:collapse;font-size:12px}
.smart-table th{background:var(--elevated);color:var(--text2);font-size:10px;font-weight:500;text-transform:uppercase;letter-spacing:0.5px;padding:8px 12px;text-align:left;border-bottom:1px solid var(--border)}
.smart-table td{padding:8px 12px;border-bottom:1px solid rgba(255,255,255,0.04)}
.smart-table tr:last-child td{border-bottom:none}
.smart-table tr:hover td{background:rgba(255,255,255,0.02)}
.mono{font-family:'SF Mono','Cascadia Code',monospace;font-size:11px}

/* Chart area */
.chart-card{background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:16px;margin-bottom:16px}
.chart-label{font-size:12px;color:var(--text2);margin-bottom:8px}
.chart-card canvas{width:100%;display:block}

/* System section (secondary) */
.system-section{margin-top:32px;padding-top:24px;border-top:1px solid var(--border)}
.system-section-title{font-size:14px;font-weight:600;color:var(--text2);margin-bottom:12px;text-transform:uppercase;letter-spacing:0.5px}
.chart-row{display:grid;grid-template-columns:1fr 1fr;gap:12px}
@media(max-width:800px){.chart-row{grid-template-columns:1fr}}
.loading{text-align:center;padding:40px;color:var(--text2)}
</style>
</head>
<body>
<div class="container">
  <div class="header">
    <a href="/" class="header-left">
      <img src="/icon.png" alt="">
      <div>
        <div class="header-title">Drive Health & Stats</div>
        <div class="header-sub">SMART analysis, temperature trends, and system metrics</div>
      </div>
    </a>
    <a href="/" class="back">&larr; Dashboard</a>
  </div>
  <div id="app"><div class="loading">Loading...</div></div>
</div>
<script src="/js/charts.js"></script>
<script>
(function() {
  var snapshot = null;
  var sparklines = null;

  function esc(s) { var d = document.createElement("div"); d.textContent = s || ""; return d.innerHTML; }

  function healthScore(d) {
    var score = 100;
    if (!d.health_passed) score -= 50;
    if (d.reallocated_sectors > 0) { score -= Math.min(36, d.reallocated_sectors > 100 ? 36 : d.reallocated_sectors > 20 ? 24 : d.reallocated_sectors > 5 ? 15 : 8); }
    if (d.pending_sectors > 0) { score -= Math.min(38, d.pending_sectors > 20 ? 38 : d.pending_sectors > 5 ? 20 : 10); }
    if (d.udma_crc_errors > 0) { score -= (d.udma_crc_errors > 200 ? 10 : d.udma_crc_errors > 50 ? 6 : 3); }
    if (d.command_timeout > 0) { score -= (d.command_timeout > 100 ? 15 : d.command_timeout > 25 ? 8 : 3); }
    var t = d.temperature_c || 0;
    if (t > 55) score -= 20; else if (t > 50) score -= 12; else if (t > 45) score -= 4;
    var h = d.power_on_hours || 0;
    if (h > 61000) score -= 8; else if (h > 44000) score -= 4; else if (h > 35000) score -= 2;
    return Math.max(0, score);
  }

  function scoreClass(s) { return s >= 80 ? "score-good" : s >= 50 ? "score-warn" : "score-crit"; }
  function valClass(v, warnAt, critAt) { if (v >= critAt) return "val-bad"; if (v >= warnAt) return "val-warn"; return "val-ok"; }

  function formatHours(h) {
    if (!h) return "0h";
    var y = Math.floor(h / 8766); var d = Math.floor((h % 8766) / 24);
    if (y > 0) return y + "y " + d + "d";
    return Math.floor(h / 24) + "d " + (h % 24) + "h";
  }

  function formatSize(gb) {
    if (!gb) return "?";
    if (gb >= 1000) return (gb / 1000).toFixed(1) + " TB";
    return gb.toFixed(0) + " GB";
  }

  function render() {
    var app = document.getElementById("app");
    var smart = (snapshot && snapshot.smart) ? snapshot.smart : [];
    if (!smart.length) { app.innerHTML = '<div class="loading">No SMART data available. Run a scan first.</div>'; return; }

    var h = "";

    // Summary
    var totalDrives = smart.length;
    var healthyDrives = smart.filter(function(d) { return healthScore(d) >= 80; }).length;
    var warningDrives = smart.filter(function(d) { var s = healthScore(d); return s >= 50 && s < 80; }).length;
    var critDrives = smart.filter(function(d) { return healthScore(d) < 50; }).length;
    var totalTB = smart.reduce(function(sum, d) { return sum + (d.size_gb || 0); }, 0) / 1000;
    var avgTemp = smart.reduce(function(sum, d) { return sum + (d.temperature_c || 0); }, 0) / smart.length;

    h += '<div class="summary">';
    h += '<div class="summary-box"><div class="summary-val">' + totalDrives + '</div><div class="summary-label">Drives</div></div>';
    h += '<div class="summary-box"><div class="summary-val val-green">' + healthyDrives + '</div><div class="summary-label">Healthy</div></div>';
    if (warningDrives > 0) h += '<div class="summary-box"><div class="summary-val val-amber">' + warningDrives + '</div><div class="summary-label">Warning</div></div>';
    if (critDrives > 0) h += '<div class="summary-box"><div class="summary-val val-red">' + critDrives + '</div><div class="summary-label">Critical</div></div>';
    h += '<div class="summary-box"><div class="summary-val">' + totalTB.toFixed(1) + ' TB</div><div class="summary-label">Total Storage</div></div>';
    h += '<div class="summary-box"><div class="summary-val">' + avgTemp.toFixed(0) + '&deg;C</div><div class="summary-label">Avg Temp</div></div>';
    h += '</div>';

    // Drive grid
    h += '<div class="drives">';
    var sorted = smart.slice().sort(function(a, b) { return healthScore(a) - healthScore(b); }); // worst first
    for (var i = 0; i < sorted.length; i++) {
      var d = sorted[i];
      var score = healthScore(d);
      var sc = scoreClass(score);
      var tempPct = Math.min(100, ((d.temperature_c || 30) - 20) / 50 * 100);
      var tempColor = (d.temperature_c || 0) >= 55 ? "var(--red)" : (d.temperature_c || 0) >= 45 ? "var(--amber)" : "var(--green)";

      h += '<div class="drive" data-idx="' + i + '" onclick="window._toggleDrive(this)">';
      h += '<div class="drive-header">';
      h += '<div class="drive-score ' + sc + '">' + score + '</div>';
      h += '<div class="drive-info">';
      h += '<div class="drive-name">' + esc(d.device) + (d.array_slot ? ' <span style="color:var(--text3)">(' + esc(d.array_slot) + ')</span>' : '') + '</div>';
      h += '<div class="drive-model">' + esc(d.model) + '</div>';
      h += '<div class="drive-badges">';
      h += '<span class="badge badge-type">' + esc(d.disk_type || "hdd") + '</span>';
      h += '<span class="badge badge-type">' + formatSize(d.size_gb) + '</span>';
      if (!d.health_passed) h += '<span class="badge badge-crit">SMART FAILED</span>';
      if (d.reallocated_sectors > 0) h += '<span class="badge badge-warn">' + d.reallocated_sectors + ' realloc</span>';
      if (d.pending_sectors > 0) h += '<span class="badge badge-crit">' + d.pending_sectors + ' pending</span>';
      if (d.udma_crc_errors > 0) h += '<span class="badge badge-warn">' + d.udma_crc_errors + ' CRC</span>';
      h += '</div>';
      h += '</div>';
      h += '</div>';

      h += '<div class="drive-stats">';
      h += '<span>Temp: <strong>' + (d.temperature_c || 0) + '&deg;C</strong></span>';
      h += '<span>Age: <strong>' + formatHours(d.power_on_hours) + '</strong></span>';
      h += '<span>S/N: <strong style="font-family:monospace;font-size:11px">' + esc(d.serial) + '</strong></span>';
      h += '</div>';

      h += '<div class="drive-temp-bar"><div class="drive-temp-fill" style="width:' + tempPct + '%;background:' + tempColor + '"></div></div>';

      // Expanded detail (hidden by default)
      h += '<div class="drive-detail">';

      // Temperature chart
      h += '<div style="margin-bottom:16px"><div style="font-size:12px;color:var(--text2);margin-bottom:6px">Temperature History</div>';
      h += '<canvas id="temp-chart-' + i + '" height="60"></canvas></div>';

      // Full SMART attributes table
      h += '<div class="detail-grid">';
      h += '<div class="detail-item"><span class="detail-label">Model</span><span class="detail-value">' + esc(d.model) + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">Serial</span><span class="detail-value mono">' + esc(d.serial) + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">Firmware</span><span class="detail-value mono">' + esc(d.firmware) + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">Capacity</span><span class="detail-value">' + formatSize(d.size_gb) + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">Type</span><span class="detail-value">' + esc(d.disk_type || "HDD") + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">ATA Port</span><span class="detail-value mono">' + esc(d.ata_port) + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">Array Slot</span><span class="detail-value">' + esc(d.array_slot || "—") + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">SMART Health</span><span class="detail-value ' + (d.health_passed ? "val-ok" : "val-bad") + '">' + (d.health_passed ? "PASSED" : "FAILED") + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">Power-On Hours</span><span class="detail-value">' + (d.power_on_hours || 0).toLocaleString() + ' h (' + formatHours(d.power_on_hours) + ')</span></div>';
      h += '<div class="detail-item"><span class="detail-label">Temperature</span><span class="detail-value ' + valClass(d.temperature_c || 0, 45, 55) + '">' + (d.temperature_c || 0) + '&deg;C (max: ' + (d.temperature_max_c || 0) + '&deg;C)</span></div>';
      h += '<div class="detail-item"><span class="detail-label">Reallocated Sectors</span><span class="detail-value ' + valClass(d.reallocated_sectors || 0, 1, 20) + '">' + (d.reallocated_sectors || 0) + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">Pending Sectors</span><span class="detail-value ' + valClass(d.pending_sectors || 0, 1, 5) + '">' + (d.pending_sectors || 0) + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">Offline Uncorrectable</span><span class="detail-value ' + valClass(d.offline_uncorrectable || 0, 1, 5) + '">' + (d.offline_uncorrectable || 0) + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">UDMA CRC Errors</span><span class="detail-value ' + valClass(d.udma_crc_errors || 0, 1, 100) + '">' + (d.udma_crc_errors || 0) + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">Command Timeout</span><span class="detail-value ' + valClass(d.command_timeout || 0, 6, 100) + '">' + (d.command_timeout || 0) + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">Spin Retry Count</span><span class="detail-value ' + valClass(d.spin_retry_count || 0, 1, 5) + '">' + (d.spin_retry_count || 0) + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">Raw Read Error Rate</span><span class="detail-value mono">' + (d.raw_read_error_rate || 0) + '</span></div>';
      h += '<div class="detail-item"><span class="detail-label">Seek Error Rate</span><span class="detail-value mono">' + (d.seek_error_rate || 0) + '</span></div>';
      h += '</div>';

      h += '<div style="text-align:right;margin-top:8px"><a href="/disk/' + encodeURIComponent(d.serial || '') + '" style="font-size:12px;color:var(--accent)">Full disk detail page &rarr;</a></div>';
      h += '</div>'; // drive-detail

      h += '</div>'; // drive
    }
    h += '</div>';

    // SMART comparison table across all drives
    h += '<div class="smart-section">';
    h += '<div class="smart-section-title">SMART Comparison — All Drives</div>';
    h += '<div style="overflow-x:auto"><table class="smart-table">';
    h += '<thead><tr><th>Device</th><th>Model</th><th>Health</th><th>Score</th><th>Temp</th><th>Age</th><th>Realloc</th><th>Pending</th><th>CRC</th><th>Cmd TO</th><th>Offline</th></tr></thead>';
    h += '<tbody>';
    for (var j = 0; j < sorted.length; j++) {
      var dd = sorted[j];
      var sc2 = healthScore(dd);
      var scCls = sc2 >= 80 ? "val-ok" : sc2 >= 50 ? "val-warn" : "val-bad";
      h += '<tr>';
      h += '<td><strong>' + esc(dd.device) + '</strong></td>';
      h += '<td style="max-width:180px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(dd.model) + '</td>';
      h += '<td class="' + (dd.health_passed ? "val-ok" : "val-bad") + '">' + (dd.health_passed ? "PASS" : "FAIL") + '</td>';
      h += '<td class="' + scCls + '" style="font-weight:600">' + sc2 + '</td>';
      h += '<td class="' + valClass(dd.temperature_c || 0, 45, 55) + '">' + (dd.temperature_c || 0) + '&deg;</td>';
      h += '<td>' + formatHours(dd.power_on_hours) + '</td>';
      h += '<td class="' + valClass(dd.reallocated_sectors || 0, 1, 20) + '">' + (dd.reallocated_sectors || 0) + '</td>';
      h += '<td class="' + valClass(dd.pending_sectors || 0, 1, 5) + '">' + (dd.pending_sectors || 0) + '</td>';
      h += '<td class="' + valClass(dd.udma_crc_errors || 0, 1, 100) + '">' + (dd.udma_crc_errors || 0) + '</td>';
      h += '<td class="' + valClass(dd.command_timeout || 0, 6, 100) + '">' + (dd.command_timeout || 0) + '</td>';
      h += '<td class="' + valClass(dd.offline_uncorrectable || 0, 1, 5) + '">' + (dd.offline_uncorrectable || 0) + '</td>';
      h += '</tr>';
    }
    h += '</tbody></table></div>';
    h += '</div>';

    // System metrics (secondary, at the bottom)
    var sys = (sparklines && sparklines.system) ? sparklines.system : [];
    if (sys.length >= 2) {
      h += '<div class="system-section">';
      h += '<div class="system-section-title">System Metrics History</div>';
      h += '<div class="chart-row">';
      h += '<div class="chart-card"><div class="chart-label">CPU Usage (%)</div><canvas id="chart-cpu" height="120"></canvas></div>';
      h += '<div class="chart-card"><div class="chart-label">Memory Usage (%)</div><canvas id="chart-mem" height="120"></canvas></div>';
      h += '</div>';
      h += '<div class="chart-row">';
      h += '<div class="chart-card"><div class="chart-label">I/O Wait (%)</div><canvas id="chart-io" height="120"></canvas></div>';
      h += '<div class="chart-card"><div class="chart-label">Load Average</div><canvas id="chart-load" height="120"></canvas></div>';
      h += '</div>';
      h += '</div>';
    }

    app.innerHTML = h;
    drawCharts();
  }

  function drawCharts() {
    if (!window.NasChart) return;
    var sys = (sparklines && sparklines.system) ? sparklines.system : [];
    var disks = (sparklines && sparklines.disks) ? sparklines.disks : [];
    var smart = (snapshot && snapshot.smart) ? snapshot.smart.slice().sort(function(a, b) { return healthScore(a) - healthScore(b); }) : [];

    // System charts
    if (sys.length >= 2) {
      var labels = sys.map(function(p, i) { return i; });
      try { NasChart.area("chart-cpu", { data: sys.map(function(p){return p.cpu_usage}), labels: labels, color: "#5e6ad2", fillAlpha: 0.12, yMin: 0, yMax: 100 }); } catch(e) {}
      try { NasChart.area("chart-mem", { data: sys.map(function(p){return p.mem_percent}), labels: labels, color: "#7170ff", fillAlpha: 0.12, yMin: 0, yMax: 100 }); } catch(e) {}
      try { NasChart.area("chart-io", { data: sys.map(function(p){return p.io_wait}), labels: labels, color: "#f59e0b", fillAlpha: 0.12, yMin: 0 }); } catch(e) {}
      try { NasChart.line("chart-load", { data: sys.map(function(p){return p.load_1}), labels: labels, color: "#22c55e" }); } catch(e) {}
    }

    // Per-drive temp charts (for expanded drives)
    for (var i = 0; i < smart.length; i++) {
      var serial = smart[i].serial || "";
      var diskData = null;
      for (var d = 0; d < disks.length; d++) {
        if (disks[d].serial === serial) { diskData = disks[d]; break; }
      }
      if (diskData && diskData.temps && diskData.temps.length >= 2) {
        var temps = diskData.temps.map(function(p){ return p.temp; });
        var maxT = Math.max.apply(null, temps);
        var clr = maxT >= 55 ? "#ef4444" : maxT >= 45 ? "#f59e0b" : "#22c55e";
        try { NasChart.area("temp-chart-" + i, { data: temps, labels: temps.map(function(v,k){return k}), color: clr, fillAlpha: 0.1 }); } catch(e) {}
      }
    }
  }

  window._toggleDrive = function(el) {
    el.classList.toggle("expanded");
    // Redraw chart if expanded (canvas may not have been visible before)
    if (el.classList.contains("expanded")) {
      setTimeout(drawCharts, 50);
    }
  };

  // Load data
  Promise.all([
    fetch("/api/v1/snapshot/latest").then(function(r){return r.json()}).catch(function(){return null}),
    fetch("/api/v1/sparklines").then(function(r){return r.json()}).catch(function(){return null})
  ]).then(function(results) {
    snapshot = results[0];
    sparklines = results[1];
    render();
  });
})();
</script>
</body>
</html>`
