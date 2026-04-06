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
/* Midnight (default) */
:root,body.theme-midnight{--bg:#0f1011;--surface:#191a1b;--elevated:#242526;--text:#f7f8f8;--text2:#8a8f98;--text3:#5a5f6a;--accent:#5e6ad2;--green:#27a644;--amber:#d97706;--red:#dc2626;--border:rgba(255,255,255,0.08);--radius:10px}
/* Clean */
body.theme-clean{--bg:#ffffff;--surface:#ffffff;--elevated:#fafafa;--text:#171717;--text2:#808080;--text3:#b3b3b3;--accent:#171717;--green:#16a34a;--amber:#d97706;--red:#dc2626;--border:rgba(0,0,0,0.08)}
body.theme-clean .drive,.theme-clean .summary-box,.theme-clean .chart-card,.theme-clean .smart-table{box-shadow:0 0 0 1px rgba(0,0,0,0.08)}
body.theme-clean .drive:hover{box-shadow:0 0 0 1px rgba(0,0,0,0.15)}
body.theme-clean .back{border-color:rgba(0,0,0,0.12);color:#808080}
body.theme-clean .back:hover{color:#171717;border-color:rgba(0,0,0,0.2)}
/* Ember */
body.theme-ember{--bg:#07080a;--surface:#101111;--elevated:#1a1b1c;--text:#f9f9f9;--text2:#9c9c9d;--text3:#5a5b5c;--accent:#55b3ff;--green:#5fc992;--amber:#FACC15;--red:#FF6363;--border:rgba(255,255,255,0.06)}
body{font-family:'Inter',system-ui,sans-serif;background:var(--bg);color:var(--text);min-height:100vh;-webkit-font-smoothing:antialiased;transition:background 0.2s,color 0.2s}
.container{max-width:1200px;margin:0 auto;padding:24px}
.header{display:flex;align-items:center;justify-content:space-between;margin-bottom:28px}
.header-left{display:flex;align-items:center;gap:12px;text-decoration:none;color:inherit}
.header-left img{width:28px;height:28px;border-radius:6px}
.header-title{font-size:20px;font-weight:600;letter-spacing:-0.3px}
.header-sub{font-size:13px;color:var(--text2)}
.back{display:inline-flex;align-items:center;gap:4px;font-size:13px;color:var(--text2);text-decoration:none;padding:6px 12px;border:1px solid var(--border);border-radius:var(--radius);transition:all 0.15s}
.back:hover{color:var(--text);border-color:rgba(255,255,255,0.15)}
.summary{display:flex;gap:12px;margin-bottom:24px;flex-wrap:wrap}
.summary-box{background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:14px 18px;flex:1;min-width:100px;text-align:center}
.summary-val{font-size:22px;font-weight:600}
.summary-label{font-size:11px;color:var(--text2);text-transform:uppercase;letter-spacing:0.5px;margin-top:2px}
.val-green{color:var(--green)}.val-amber{color:var(--amber)}.val-red{color:var(--red)}
/* Drive list — single column to avoid row-stretch bug */
.drives{display:flex;flex-direction:column;gap:12px;margin-bottom:32px}
.drive{background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);overflow:hidden;transition:border-color 0.15s}
.drive-clickable{cursor:pointer}
.drive-clickable:hover{border-color:rgba(255,255,255,0.15)}
.drive-row{display:flex;align-items:center;gap:12px;padding:14px 16px}
.drive-score{width:44px;height:44px;border-radius:50%;display:flex;align-items:center;justify-content:center;font-size:15px;font-weight:700;flex-shrink:0;border:2px solid}
.score-good{border-color:var(--green);color:var(--green);background:rgba(39,166,68,0.08)}
.score-warn{border-color:var(--amber);color:var(--amber);background:rgba(217,119,6,0.08)}
.score-crit{border-color:var(--red);color:var(--red);background:rgba(220,38,38,0.08)}
.drive-info{flex:1;min-width:0}
.drive-name{font-size:14px;font-weight:600}
.drive-model{font-size:12px;color:var(--text2);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.drive-meta{display:flex;gap:12px;font-size:12px;color:var(--text2);margin-top:3px;flex-wrap:wrap}
.drive-meta strong{color:var(--text)}
.drive-badges{display:flex;gap:6px;flex-shrink:0;flex-wrap:wrap}
.badge{font-size:10px;font-weight:600;padding:2px 8px;border-radius:9999px;text-transform:uppercase;letter-spacing:0.3px}
.badge-ok{background:rgba(39,166,68,0.1);color:var(--green)}
.badge-warn{background:rgba(217,119,6,0.1);color:var(--amber)}
.badge-crit{background:rgba(220,38,38,0.1);color:var(--red)}
.badge-type{background:rgba(94,106,210,0.1);color:var(--accent)}
.drive-chevron{color:var(--text3);font-size:18px;transition:transform 0.2s;flex-shrink:0}
.drive.expanded .drive-chevron{transform:rotate(90deg)}
/* Expanded detail */
.drive-detail{max-height:0;overflow:hidden;transition:max-height 0.35s ease}
.drive.expanded .drive-detail{max-height:2000px}
.drive-detail-inner{border-top:1px solid var(--border);padding:16px}
.detail-grid{display:grid;grid-template-columns:1fr 1fr;gap:6px 24px;margin-bottom:16px}
@media(max-width:700px){.detail-grid{grid-template-columns:1fr}}
.detail-item{display:flex;justify-content:space-between;font-size:12px;padding:5px 0;border-bottom:1px solid rgba(255,255,255,0.04)}
.detail-label{color:var(--text2)}
.detail-value{font-weight:500;font-family:'SF Mono','Cascadia Code',monospace;font-size:11px}
.detail-value.val-bad{color:var(--red)}.detail-value.val-warn{color:var(--amber)}.detail-value.val-ok{color:var(--green)}
/* Backblaze info box */
.bb-box{background:var(--elevated);border:1px solid var(--border);border-radius:var(--radius);padding:12px 14px;margin-bottom:14px;font-size:12px;line-height:1.6}
.bb-title{font-weight:600;color:var(--text);margin-bottom:4px;font-size:13px}
.bb-row{display:flex;justify-content:space-between;padding:2px 0}
.bb-label{color:var(--text2)}
.bb-value{font-weight:600}
/* SMART table */
.smart-section{margin-bottom:32px}
.smart-section-title{font-size:16px;font-weight:600;margin-bottom:12px}
.smart-table{width:100%;background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);overflow:hidden;border-collapse:collapse;font-size:12px}
.smart-table th{background:var(--elevated);color:var(--text2);font-size:10px;font-weight:500;text-transform:uppercase;letter-spacing:0.5px;padding:8px 12px;text-align:left;border-bottom:1px solid var(--border)}
.smart-table td{padding:8px 12px;border-bottom:1px solid rgba(255,255,255,0.04)}
.smart-table tr:last-child td{border-bottom:none}
.smart-table tr:hover td{background:rgba(255,255,255,0.02)}
.mono{font-family:'SF Mono','Cascadia Code',monospace;font-size:11px}
.chart-card{background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:16px;margin-bottom:12px}
.chart-label{font-size:12px;color:var(--text2);margin-bottom:8px}
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
        <div class="header-sub">SMART analysis powered by Backblaze failure data (Q4-2025)</div>
      </div>
    </a>
    <a href="/" class="back">&larr; Dashboard</a>
  </div>
  <div id="app"><div class="loading">Loading...</div></div>
</div>
<script src="/js/charts.js"></script>
<script>
(function() {
  var snapshot = null, sparklines = null, expandedIdx = -1;
  function esc(s) { var d = document.createElement("div"); d.textContent = s || ""; return d.innerHTML; }

  /* ── Backblaze thresholds (mirrors backblaze_thresholds.go) ── */
  function bbReallocTier(n) {
    if (n <= 0) return null;
    if (n <= 4) return { mult: 2.5, label: "Elevated risk" };
    if (n <= 19) return { mult: 5.0, label: "High risk" };
    if (n <= 99) return { mult: 8.0, label: "Very high risk" };
    return { mult: 12.0, label: "Extreme risk" };
  }
  function bbPendingTier(n) {
    if (n <= 0) return null;
    if (n <= 4) return { mult: 4.0, label: "Active media degradation" };
    if (n <= 19) return { mult: 8.0, label: "Significant media failure" };
    return { mult: 15.0, label: "Severe media failure" };
  }
  function bbAgeTier(h) {
    if (h <= 13000) return { mult: 1.5, label: "Infant period (burn-in)", bracket: "0–1.5 years" };
    if (h <= 35000) return { mult: 1.0, label: "Prime operating years", bracket: "1.5–4 years" };
    if (h <= 44000) return { mult: 1.3, label: "Entering higher-risk bracket", bracket: "4–5 years" };
    if (h <= 61000) return { mult: 1.8, label: "Failure rate rising", bracket: "5–7 years" };
    return { mult: 2.5, label: "Plan replacement", bracket: "7+ years" };
  }
  function bbTempTier(t) {
    if (t <= 34) return { mult: 1.0, label: "Optimal" };
    if (t <= 39) return { mult: 1.0, label: "Normal" };
    if (t <= 44) return { mult: 1.5, label: "Warm" };
    if (t <= 49) return { mult: 2.0, label: "Elevated" };
    if (t <= 54) return { mult: 3.0, label: "Hot" };
    if (t <= 59) return { mult: 5.0, label: "Dangerous" };
    return { mult: 10.0, label: "Critical thermal" };
  }
  function estimatedLife(h) {
    /* Average HDD life per Backblaze: ~5-6 years. Median failure around 50,000 hours. */
    var avgLifeH = 50000;
    var remaining = Math.max(0, avgLifeH - h);
    var remainY = remaining / 8766;
    return { avgYears: 5.7, remainingYears: remainY, percentUsed: Math.min(100, (h / avgLifeH) * 100) };
  }

  function healthScore(d) {
    var score = 100;
    if (!d.health_passed) score -= 50;
    if (d.reallocated_sectors > 0) { var t = bbReallocTier(d.reallocated_sectors); score -= Math.min(36, t.mult * 3); }
    if (d.pending_sectors > 0) { var t = bbPendingTier(d.pending_sectors); score -= Math.min(38, t.mult * 2.5); }
    if (d.udma_crc_errors > 0) score -= (d.udma_crc_errors > 200 ? 10 : d.udma_crc_errors > 50 ? 6 : 3);
    if (d.command_timeout > 0) score -= (d.command_timeout > 100 ? 15 : d.command_timeout > 25 ? 8 : 3);
    var tt = bbTempTier(d.temperature_c || 0); score -= (tt.mult - 1) * 4;
    var at = bbAgeTier(d.power_on_hours || 0); score -= (at.mult - 1) * 5;
    return Math.max(0, Math.round(score));
  }
  function scoreClass(s) { return s >= 80 ? "score-good" : s >= 50 ? "score-warn" : "score-crit"; }
  function valClass(v, w, c) { return v >= c ? "val-bad" : v >= w ? "val-warn" : "val-ok"; }
  function formatHours(h) { if (!h) return "0h"; var y = Math.floor(h/8766), d = Math.floor((h%8766)/24); return y > 0 ? y+"y "+d+"d" : Math.floor(h/24)+"d "+(h%24)+"h"; }
  function formatSize(gb) { return !gb ? "?" : gb >= 1000 ? (gb/1000).toFixed(1)+" TB" : gb.toFixed(0)+" GB"; }

  function render() {
    var app = document.getElementById("app");
    var smart = (snapshot && snapshot.smart) ? snapshot.smart : [];
    if (!smart.length) { app.innerHTML = '<div class="loading">No SMART data available. Run a scan first.</div>'; return; }
    var sorted = smart.slice().sort(function(a,b){ return healthScore(a) - healthScore(b); });
    var h = "";
    // Summary
    var total = smart.length, healthy = 0, warn = 0, crit = 0;
    for (var x = 0; x < smart.length; x++) { var sc = healthScore(smart[x]); if (sc >= 80) healthy++; else if (sc >= 50) warn++; else crit++; }
    var totalTB = smart.reduce(function(s,d){return s+(d.size_gb||0)},0)/1000;
    var avgTemp = smart.reduce(function(s,d){return s+(d.temperature_c||0)},0)/smart.length;
    h += '<div class="summary">';
    h += '<div class="summary-box"><div class="summary-val">'+total+'</div><div class="summary-label">Drives</div></div>';
    h += '<div class="summary-box"><div class="summary-val val-green">'+healthy+'</div><div class="summary-label">Healthy</div></div>';
    if (warn) h += '<div class="summary-box"><div class="summary-val val-amber">'+warn+'</div><div class="summary-label">Warning</div></div>';
    if (crit) h += '<div class="summary-box"><div class="summary-val val-red">'+crit+'</div><div class="summary-label">Critical</div></div>';
    h += '<div class="summary-box"><div class="summary-val">'+totalTB.toFixed(1)+' TB</div><div class="summary-label">Total Storage</div></div>';
    h += '<div class="summary-box"><div class="summary-val">'+avgTemp.toFixed(0)+'&deg;C</div><div class="summary-label">Avg Temp</div></div>';
    h += '</div>';
    // Drive list (single column — no row-stretch)
    h += '<div class="drives">';
    for (var i = 0; i < sorted.length; i++) {
      var d = sorted[i], score = healthScore(d), scCls = scoreClass(score);
      var isExp = (i === expandedIdx);
      h += '<div class="drive drive-clickable'+(isExp?" expanded":"")+'" data-idx="'+i+'">';
      h += '<div class="drive-row" onclick="window._toggleDrive('+i+')">';
      h += '<div class="drive-score '+scCls+'">'+score+'</div>';
      h += '<div class="drive-info">';
      h += '<div class="drive-name">'+esc(d.device)+(d.array_slot?' <span style="color:var(--text3)">('+esc(d.array_slot)+')</span>':'')+'</div>';
      h += '<div class="drive-model">'+esc(d.model)+'</div>';
      h += '<div class="drive-meta">';
      h += '<span>'+formatSize(d.size_gb)+'</span>';
      h += '<span>'+esc(d.disk_type||"hdd")+'</span>';
      h += '<span>'+(d.temperature_c||0)+'&deg;C</span>';
      h += '<span>'+formatHours(d.power_on_hours)+'</span>';
      h += '</div>';
      h += '</div>';
      h += '<div class="drive-badges">';
      if (!d.health_passed) h += '<span class="badge badge-crit">FAILED</span>';
      if (d.reallocated_sectors > 0) h += '<span class="badge badge-warn">'+d.reallocated_sectors+' realloc</span>';
      if (d.pending_sectors > 0) h += '<span class="badge badge-crit">'+d.pending_sectors+' pending</span>';
      if (d.udma_crc_errors > 0) h += '<span class="badge badge-warn">'+d.udma_crc_errors+' CRC</span>';
      h += '</div>';
      h += '<span class="drive-chevron">&#x276F;</span>';
      h += '</div>';
      // Detail panel
      h += '<div class="drive-detail"><div class="drive-detail-inner">';
      // Backblaze risk assessment
      var age = bbAgeTier(d.power_on_hours||0);
      var temp = bbTempTier(d.temperature_c||0);
      var life = estimatedLife(d.power_on_hours||0);
      h += '<div class="bb-box">';
      h += '<div class="bb-title">Backblaze Risk Assessment (Q4-2025, 337k+ drives)</div>';
      h += '<div class="bb-row"><span class="bb-label">Health Score</span><span class="bb-value '+scCls.replace("score-","val-")+'">'+score+' / 100</span></div>';
      h += '<div class="bb-row"><span class="bb-label">Age Bracket</span><span class="bb-value">'+age.bracket+' — '+age.label+'</span></div>';
      h += '<div class="bb-row"><span class="bb-label">Age Failure Multiplier</span><span class="bb-value">'+age.mult.toFixed(1)+'x baseline</span></div>';
      h += '<div class="bb-row"><span class="bb-label">Temperature Rating</span><span class="bb-value">'+temp.label+' ('+temp.mult.toFixed(1)+'x failure rate)</span></div>';
      h += '<div class="bb-row"><span class="bb-label">Estimated Life Used</span><span class="bb-value">'+life.percentUsed.toFixed(0)+'% (avg HDD life: ~'+life.avgYears+' years)</span></div>';
      h += '<div class="bb-row"><span class="bb-label">Est. Remaining Life</span><span class="bb-value '+(life.remainingYears < 1 ? "val-red" : life.remainingYears < 2 ? "val-amber" : "")+'">~'+life.remainingYears.toFixed(1)+' years at current usage</span></div>';
      var rt = bbReallocTier(d.reallocated_sectors);
      if (rt) h += '<div class="bb-row"><span class="bb-label">Reallocated Sector Risk</span><span class="bb-value val-warn">'+rt.label+' ('+rt.mult+'x failure rate)</span></div>';
      var pt = bbPendingTier(d.pending_sectors);
      if (pt) h += '<div class="bb-row"><span class="bb-label">Pending Sector Risk</span><span class="bb-value val-red">'+pt.label+' ('+pt.mult+'x failure rate)</span></div>';
      h += '</div>';
      // Temp chart
      h += '<div style="margin-bottom:14px"><div style="font-size:12px;color:var(--text2);margin-bottom:6px">Temperature History</div>';
      h += '<canvas id="temp-chart-'+i+'" width="600" height="60" style="width:100%;height:60px"></canvas></div>';
      // SMART attributes
      h += '<div class="detail-grid">';
      var attrs = [
        ["Model", esc(d.model), ""],
        ["Serial", esc(d.serial), "mono"],
        ["Firmware", esc(d.firmware), "mono"],
        ["Capacity", formatSize(d.size_gb), ""],
        ["Type", esc(d.disk_type||"HDD"), ""],
        ["ATA Port", esc(d.ata_port), "mono"],
        ["Array Slot", esc(d.array_slot||"—"), ""],
        ["SMART Health", d.health_passed?"PASSED":"FAILED", d.health_passed?"val-ok":"val-bad"],
        ["Power-On Hours", (d.power_on_hours||0).toLocaleString()+" h ("+formatHours(d.power_on_hours)+")", ""],
        ["Temperature", (d.temperature_c||0)+"°C (max: "+(d.temperature_max_c||0)+"°C)", valClass(d.temperature_c||0,45,55)],
        ["Reallocated Sectors", (d.reallocated_sectors||0)+"", valClass(d.reallocated_sectors||0,1,20)],
        ["Pending Sectors", (d.pending_sectors||0)+"", valClass(d.pending_sectors||0,1,5)],
        ["Offline Uncorrectable", (d.offline_uncorrectable||0)+"", valClass(d.offline_uncorrectable||0,1,5)],
        ["UDMA CRC Errors", (d.udma_crc_errors||0)+"", valClass(d.udma_crc_errors||0,1,100)],
        ["Command Timeout", (d.command_timeout||0)+"", valClass(d.command_timeout||0,6,100)],
        ["Spin Retry Count", (d.spin_retry_count||0)+"", valClass(d.spin_retry_count||0,1,5)],
        ["Raw Read Error Rate", (d.raw_read_error_rate||0)+"", "mono"],
        ["Seek Error Rate", (d.seek_error_rate||0)+"", "mono"]
      ];
      for (var a = 0; a < attrs.length; a++) {
        h += '<div class="detail-item"><span class="detail-label">'+attrs[a][0]+'</span><span class="detail-value '+attrs[a][2]+'">'+attrs[a][1]+'</span></div>';
      }
      h += '</div>';
      h += '<div style="text-align:right"><a href="/disk/'+encodeURIComponent(d.serial||'')+'" style="font-size:12px;color:var(--accent)">Full disk detail page &rarr;</a></div>';
      h += '</div></div>'; // detail-inner, detail
      h += '</div>'; // drive
    }
    h += '</div>';
    // SMART comparison table
    h += '<div class="smart-section"><div class="smart-section-title">SMART Comparison — All Drives</div>';
    h += '<div style="overflow-x:auto"><table class="smart-table">';
    h += '<thead><tr><th>Device</th><th>Model</th><th>Health</th><th>Score</th><th>Temp</th><th>Age</th><th>Realloc</th><th>Pending</th><th>CRC</th><th>Cmd TO</th><th>Offline</th></tr></thead><tbody>';
    for (var j = 0; j < sorted.length; j++) {
      var dd = sorted[j], s2 = healthScore(dd), sCls = s2>=80?"val-ok":s2>=50?"val-warn":"val-bad";
      h += '<tr><td><strong>'+esc(dd.device)+'</strong></td>';
      h += '<td style="max-width:160px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">'+esc(dd.model)+'</td>';
      h += '<td class="'+(dd.health_passed?"val-ok":"val-bad")+'">'+(dd.health_passed?"PASS":"FAIL")+'</td>';
      h += '<td class="'+sCls+'" style="font-weight:600">'+s2+'</td>';
      h += '<td class="'+valClass(dd.temperature_c||0,45,55)+'">'+(dd.temperature_c||0)+'°</td>';
      h += '<td>'+formatHours(dd.power_on_hours)+'</td>';
      h += '<td class="'+valClass(dd.reallocated_sectors||0,1,20)+'">'+(dd.reallocated_sectors||0)+'</td>';
      h += '<td class="'+valClass(dd.pending_sectors||0,1,5)+'">'+(dd.pending_sectors||0)+'</td>';
      h += '<td class="'+valClass(dd.udma_crc_errors||0,1,100)+'">'+(dd.udma_crc_errors||0)+'</td>';
      h += '<td class="'+valClass(dd.command_timeout||0,6,100)+'">'+(dd.command_timeout||0)+'</td>';
      h += '<td class="'+valClass(dd.offline_uncorrectable||0,1,5)+'">'+(dd.offline_uncorrectable||0)+'</td></tr>';
    }
    h += '</tbody></table></div></div>';
    // System metrics
    var sys = (sparklines && sparklines.system) ? sparklines.system : [];
    // Limit to last 7 days worth (max 168 points)
    if (sys.length > 168) sys = sys.slice(sys.length - 168);
    if (sys.length >= 1) {
      h += '<div class="system-section"><div class="system-section-title">System Metrics (last '+sys.length+' data points)</div>';
      h += '<div class="chart-row">';
      h += '<div class="chart-card"><div class="chart-label">CPU Usage (%)</div><canvas id="chart-cpu" width="600" height="140" style="width:100%;height:140px"></canvas></div>';
      h += '<div class="chart-card"><div class="chart-label">Memory Usage (%)</div><canvas id="chart-mem" width="600" height="140" style="width:100%;height:140px"></canvas></div>';
      h += '</div><div class="chart-row">';
      h += '<div class="chart-card"><div class="chart-label">I/O Wait (%)</div><canvas id="chart-io" width="600" height="140" style="width:100%;height:140px"></canvas></div>';
      h += '<div class="chart-card"><div class="chart-label">Load Average</div><canvas id="chart-load" width="600" height="140" style="width:100%;height:140px"></canvas></div>';
      h += '</div></div>';
    }
    app.innerHTML = h;
    drawCharts();
  }

  function drawCharts() {
    if (!window.NasChart) return;
    var sys = (sparklines && sparklines.system) ? sparklines.system : [];
    if (sys.length > 168) sys = sys.slice(sys.length - 168);
    var disks = (sparklines && sparklines.disks) ? sparklines.disks : [];
    var smart = (snapshot && snapshot.smart) ? snapshot.smart.slice().sort(function(a,b){return healthScore(a)-healthScore(b)}) : [];
    if (sys.length >= 1) {
      // Duplicate single point so charts can draw a line
      if (sys.length === 1) sys = [sys[0], sys[0]];
      var lb = sys.map(function(p,i){return i});
      try { NasChart.area("chart-cpu",{data:sys.map(function(p){return p.cpu_usage}),labels:lb,color:"#5e6ad2",fillAlpha:0.12,yMin:0}); } catch(e){}
      try { NasChart.area("chart-mem",{data:sys.map(function(p){return p.mem_percent}),labels:lb,color:"#7170ff",fillAlpha:0.12,yMin:0}); } catch(e){}
      try { NasChart.area("chart-io",{data:sys.map(function(p){return p.io_wait}),labels:lb,color:"#f59e0b",fillAlpha:0.12,yMin:0}); } catch(e){}
      try { NasChart.line("chart-load",{data:sys.map(function(p){return p.load_1}),labels:lb,color:"#22c55e"}); } catch(e){}
    }
    for (var i = 0; i < smart.length; i++) {
      var el = document.getElementById("temp-chart-"+i);
      if (!el || !el.offsetParent) continue; // skip hidden canvases
      var serial = smart[i].serial||"";
      for (var d = 0; d < disks.length; d++) {
        if (disks[d].serial === serial && disks[d].temps && disks[d].temps.length >= 2) {
          var temps = disks[d].temps.map(function(p){return p.temp});
          var mx = Math.max.apply(null,temps);
          var clr = mx>=55?"#ef4444":mx>=45?"#f59e0b":"#22c55e";
          try { NasChart.area("temp-chart-"+i,{data:temps,labels:temps.map(function(v,k){return k}),color:clr,fillAlpha:0.1}); } catch(e){}
          break;
        }
      }
    }
  }

  window._toggleDrive = function(idx) {
    expandedIdx = (expandedIdx === idx) ? -1 : idx;
    render(); // re-render to collapse others and expand target
  };

  /* Apply theme */
  (function(){
    try {
      var t = localStorage.getItem("nas-doctor-theme");
      if (t === "clean" || t === "ember") document.body.classList.add("theme-"+t);
    } catch(e) {}
    fetch("/api/v1/settings").then(function(r){return r.json()}).then(function(d){
      if (d.theme === "clean" || d.theme === "ember") {
        document.body.className = "theme-"+d.theme;
      }
      try { localStorage.setItem("nas-doctor-theme", d.theme); } catch(e){}
    }).catch(function(){});
  })();

  Promise.all([
    fetch("/api/v1/snapshot/latest").then(function(r){return r.json()}).catch(function(){return null}),
    fetch("/api/v1/sparklines").then(function(r){return r.json()}).catch(function(){return null})
  ]).then(function(r){ snapshot=r[0]; sparklines=r[1]; render(); });
})();
</script>
</body>
</html>`
