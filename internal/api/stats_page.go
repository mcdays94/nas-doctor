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
<title>NAS Doctor — Stats</title>
<link rel="icon" href="/icon.png">
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#0f1011;--surface:#191a1b;--text:#f7f8f8;--text2:#8a8f98;--text3:#5a5f6a;--accent:#5e6ad2;--green:#27a644;--amber:#d97706;--red:#dc2626;--border:rgba(255,255,255,0.08);--radius:10px}
body{font-family:'Inter',system-ui,sans-serif;background:var(--bg);color:var(--text);min-height:100vh;-webkit-font-smoothing:antialiased}
.container{max-width:1200px;margin:0 auto;padding:24px}
.header{display:flex;align-items:center;justify-content:space-between;margin-bottom:28px}
.header-left{display:flex;align-items:center;gap:12px;text-decoration:none;color:inherit}
.header-left img{width:28px;height:28px;border-radius:6px}
.header-title{font-size:20px;font-weight:600;letter-spacing:-0.3px}
.header-sub{font-size:13px;color:var(--text2)}
.back{display:inline-flex;align-items:center;gap:4px;font-size:13px;color:var(--text2);text-decoration:none;padding:6px 12px;border:1px solid var(--border);border-radius:var(--radius);transition:all 0.15s}
.back:hover{color:var(--text);border-color:rgba(255,255,255,0.15)}
.section{margin-bottom:32px}
.section-title{font-size:15px;font-weight:600;margin-bottom:12px;color:var(--text)}
.chart-card{background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:20px;margin-bottom:16px}
.chart-card canvas{width:100%;height:200px;display:block}
.chart-row{display:grid;grid-template-columns:1fr 1fr;gap:16px}
@media(max-width:900px){.chart-row{grid-template-columns:1fr}}
.chart-label{font-size:12px;color:var(--text2);margin-bottom:8px}
.stat-row{display:flex;gap:16px;margin-bottom:16px;flex-wrap:wrap}
.stat-box{background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:16px 20px;flex:1;min-width:120px;text-align:center}
.stat-value{font-size:24px;font-weight:600;color:var(--text)}
.stat-label{font-size:11px;color:var(--text2);text-transform:uppercase;letter-spacing:0.5px;margin-top:4px}
.drives-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:12px}
.drive-card{background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:16px}
.drive-name{font-size:13px;font-weight:600;margin-bottom:2px}
.drive-model{font-size:11px;color:var(--text3);margin-bottom:8px}
.drive-card canvas{width:100%;height:80px;display:block}
.loading{text-align:center;padding:40px;color:var(--text2)}
</style>
</head>
<body>
<div class="container">
  <div class="header">
    <a href="/" class="header-left">
      <img src="/icon.png" alt="">
      <div>
        <div class="header-title">Stats & Trends</div>
        <div class="header-sub">Historical system and drive metrics</div>
      </div>
    </a>
    <div style="display:flex;gap:8px">
      <a href="/" class="back">&larr; Dashboard</a>
    </div>
  </div>
  <div id="app"><div class="loading">Loading data...</div></div>
</div>
<script src="/js/charts.js"></script>
<script>
(function() {
  function esc(s) { var d = document.createElement("div"); d.textContent = s || ""; return d.innerHTML; }

  function render(data) {
    var app = document.getElementById("app");
    var sys = data.system || [];
    var disks = data.disks || [];
    var h = "";

    // Current stats summary
    if (sys.length > 0) {
      var latest = sys[sys.length - 1];
      h += '<div class="stat-row">';
      h += '<div class="stat-box"><div class="stat-value">' + (latest.cpu_usage || 0).toFixed(1) + '%</div><div class="stat-label">CPU Now</div></div>';
      h += '<div class="stat-box"><div class="stat-value">' + (latest.mem_percent || 0).toFixed(1) + '%</div><div class="stat-label">Memory Now</div></div>';
      h += '<div class="stat-box"><div class="stat-value">' + (latest.io_wait || 0).toFixed(1) + '%</div><div class="stat-label">I/O Wait Now</div></div>';
      h += '<div class="stat-box"><div class="stat-value">' + sys.length + '</div><div class="stat-label">Data Points</div></div>';
      h += '</div>';
    }

    // System charts
    if (sys.length >= 2) {
      h += '<div class="section">';
      h += '<div class="section-title">System Metrics</div>';
      h += '<div class="chart-row">';

      h += '<div class="chart-card">';
      h += '<div class="chart-label">CPU Usage (%)</div>';
      h += '<canvas id="chart-cpu" height="200"></canvas>';
      h += '</div>';

      h += '<div class="chart-card">';
      h += '<div class="chart-label">Memory Usage (%)</div>';
      h += '<canvas id="chart-mem" height="200"></canvas>';
      h += '</div>';

      h += '</div>';

      h += '<div class="chart-row">';

      h += '<div class="chart-card">';
      h += '<div class="chart-label">I/O Wait (%)</div>';
      h += '<canvas id="chart-io" height="200"></canvas>';
      h += '</div>';

      h += '<div class="chart-card">';
      h += '<div class="chart-label">Load Average (1 min)</div>';
      h += '<canvas id="chart-load" height="200"></canvas>';
      h += '</div>';

      h += '</div>';
      h += '</div>';
    }

    // Drive temperature charts
    if (disks.length > 0) {
      h += '<div class="section">';
      h += '<div class="section-title">Drive Temperatures</div>';
      h += '<div class="drives-grid">';
      for (var i = 0; i < disks.length; i++) {
        var dk = disks[i];
        if (!dk.temps || dk.temps.length < 2) continue;
        var temps = dk.temps.map(function(p) { return p.temp; });
        var maxT = Math.max.apply(null, temps);
        var minT = Math.min.apply(null, temps);
        var curT = temps[temps.length - 1];
        h += '<div class="drive-card">';
        h += '<div class="drive-name">' + esc(dk.device) + ' &mdash; ' + curT + '&deg;C</div>';
        h += '<div class="drive-model">' + esc(dk.model) + ' &middot; ' + minT + '&ndash;' + maxT + '&deg;C range</div>';
        h += '<canvas id="chart-temp-' + i + '" height="80"></canvas>';
        h += '</div>';
      }
      h += '</div>';
      h += '</div>';
    }

    // SMART attribute trends
    var hasSMARTTrends = false;
    for (var s = 0; s < disks.length; s++) {
      var d = disks[s];
      if (d.reallocated && d.reallocated.some(function(v){ return v > 0; })) { hasSMARTTrends = true; break; }
      if (d.crc && d.crc.some(function(v){ return v > 0; })) { hasSMARTTrends = true; break; }
    }
    if (hasSMARTTrends) {
      h += '<div class="section">';
      h += '<div class="section-title">SMART Attribute Trends</div>';
      h += '<div class="drives-grid">';
      for (var j = 0; j < disks.length; j++) {
        var dd = disks[j];
        var hasData = false;
        if (dd.reallocated && dd.reallocated.some(function(v){ return v > 0; })) hasData = true;
        if (dd.crc && dd.crc.some(function(v){ return v > 0; })) hasData = true;
        if (dd.pending && dd.pending.some(function(v){ return v > 0; })) hasData = true;
        if (!hasData) continue;
        h += '<div class="drive-card">';
        h += '<div class="drive-name">' + esc(dd.device) + '</div>';
        h += '<div class="drive-model">' + esc(dd.model) + '</div>';
        h += '<canvas id="chart-smart-' + j + '" height="80"></canvas>';
        h += '</div>';
      }
      h += '</div>';
      h += '</div>';
    }

    if (!sys.length && !disks.length) {
      h = '<div class="loading">No historical data yet. Run a few scans to build up trends.</div>';
    }

    app.innerHTML = h;

    // Draw charts after DOM update
    if (sys.length >= 2 && window.NasChart) {
      var cpuD = sys.map(function(p) { return p.cpu_usage; });
      var memD = sys.map(function(p) { return p.mem_percent; });
      var ioD = sys.map(function(p) { return p.io_wait; });
      var loadD = sys.map(function(p) { return p.load_1; });
      var labels = sys.map(function(p, i) { return i; });

      try { NasChart.area("chart-cpu", { data: cpuD, labels: labels, color: "#5e6ad2", fillAlpha: 0.15, yMin: 0, yMax: 100 }); } catch(e) {}
      try { NasChart.area("chart-mem", { data: memD, labels: labels, color: "#7170ff", fillAlpha: 0.15, yMin: 0, yMax: 100 }); } catch(e) {}
      try { NasChart.area("chart-io", { data: ioD, labels: labels, color: "#f59e0b", fillAlpha: 0.15, yMin: 0 }); } catch(e) {}
      try { NasChart.line("chart-load", { data: loadD, labels: labels, color: "#22c55e" }); } catch(e) {}
    }

    // Drive temperature charts
    for (var t = 0; t < disks.length; t++) {
      var dsk = disks[t];
      if (!dsk.temps || dsk.temps.length < 2) continue;
      var tData = dsk.temps.map(function(p) { return p.temp; });
      var tLabels = dsk.temps.map(function(p, i) { return i; });
      var maxTemp = Math.max.apply(null, tData);
      var tColor = maxTemp >= 55 ? "#ef4444" : maxTemp >= 45 ? "#f59e0b" : "#22c55e";
      try { NasChart.area("chart-temp-" + t, { data: tData, labels: tLabels, color: tColor, fillAlpha: 0.1 }); } catch(e) {}
    }

    // SMART attribute trend charts
    for (var k = 0; k < disks.length; k++) {
      var sd = disks[k];
      var el = document.getElementById("chart-smart-" + k);
      if (!el) continue;
      // Draw reallocated as red line, CRC as amber, pending as purple
      var sLabels = [];
      if (sd.reallocated) sLabels = sd.reallocated.map(function(v, i) { return i; });
      if (sd.reallocated && sd.reallocated.some(function(v){ return v > 0; })) {
        try { NasChart.line("chart-smart-" + k, { data: sd.reallocated, labels: sLabels, color: "#ef4444" }); } catch(e) {}
      } else if (sd.crc && sd.crc.some(function(v){ return v > 0; })) {
        try { NasChart.line("chart-smart-" + k, { data: sd.crc, labels: sLabels, color: "#f59e0b" }); } catch(e) {}
      }
    }
  }

  fetch("/api/v1/sparklines")
    .then(function(r) { return r.json(); })
    .then(function(data) { render(data); })
    .catch(function() {
      document.getElementById("app").innerHTML = '<div class="loading">Failed to load data.</div>';
    });
})();
</script>
</body>
</html>`
