package api

import "net/http"

func (s *Server) handleFleetPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(fleetPageHTML))
}

const fleetPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>NAS Doctor — Fleet</title>
<link rel="icon" href="/icon.png">
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#0f1011;--surface:#191a1b;--text:#f7f8f8;--text2:#8a8f98;--text3:#5a5f6a;--accent:#5e6ad2;--green:#27a644;--amber:#d97706;--red:#dc2626;--border:rgba(255,255,255,0.08);--radius:10px}
body{font-family:'Inter',system-ui,sans-serif;background:var(--bg);color:var(--text);min-height:100vh;-webkit-font-smoothing:antialiased}
.container{max-width:1100px;margin:0 auto;padding:24px}
.header{display:flex;align-items:center;justify-content:space-between;margin-bottom:32px}
.header-left{display:flex;align-items:center;gap:12px}
.header-left img{width:28px;height:28px;border-radius:6px}
.header-title{font-size:20px;font-weight:600;letter-spacing:-0.3px}
.header-sub{font-size:13px;color:var(--text2)}
.back{display:inline-flex;align-items:center;gap:4px;font-size:13px;color:var(--text2);text-decoration:none;padding:6px 12px;border:1px solid var(--border);border-radius:var(--radius);transition:all 0.15s}
.back:hover{color:var(--text);border-color:rgba(255,255,255,0.15)}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(300px,1fr));gap:16px}
.card{background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:20px;transition:border-color 0.15s}
.card:hover{border-color:rgba(255,255,255,0.12)}
.card-header{display:flex;justify-content:space-between;align-items:center;margin-bottom:12px}
.card-name{font-size:15px;font-weight:600}
.card-hostname{font-size:12px;color:var(--text2);margin-top:2px}
.health-dot{width:10px;height:10px;border-radius:50%;flex-shrink:0}
.health-dot.healthy{background:var(--green)}
.health-dot.warning{background:var(--amber)}
.health-dot.critical{background:var(--red)}
.health-dot.offline{background:var(--text3)}
.stats{display:flex;gap:16px;margin-top:12px;font-size:12px;color:var(--text2)}
.stat-val{font-weight:600;color:var(--text)}
.findings-row{display:flex;gap:10px;margin-top:10px}
.pill{font-size:11px;font-weight:600;padding:2px 8px;border-radius:9999px}
.pill-crit{background:rgba(220,38,38,0.1);color:var(--red)}
.pill-warn{background:rgba(217,119,6,0.1);color:var(--amber)}
.pill-info{background:rgba(94,106,210,0.1);color:var(--accent)}
.error-msg{font-size:12px;color:var(--red);margin-top:8px}
.empty{text-align:center;padding:60px 20px;color:var(--text2)}
.empty-title{font-size:16px;font-weight:600;color:var(--text);margin-bottom:8px}
.add-btn{display:inline-flex;align-items:center;gap:6px;padding:8px 18px;background:var(--accent);color:#fff;border:none;border-radius:9999px;font-family:inherit;font-size:13px;font-weight:500;cursor:pointer;text-decoration:none;transition:background 0.15s}
.add-btn:hover{background:#7170ff}
a.card-link{text-decoration:none;color:inherit;display:block}
</style>
</head>
<body>
<div class="container">
  <div class="header">
    <a href="/" class="header-left" style="text-decoration:none;color:inherit">
      <img src="/icon.png" alt="">
      <div>
        <div class="header-title">Fleet Overview</div>
        <div class="header-sub">Monitor all your NAS Doctor instances</div>
      </div>
    </a>
    <div style="display:flex;gap:8px">
      <a href="/" class="back">&larr; Dashboard</a>
      <a href="/settings" class="back">Settings</a>
    </div>
  </div>
  <div id="app"></div>
</div>
<script>
(function() {
  function esc(s) { var d = document.createElement("div"); d.textContent = s || ""; return d.innerHTML; }

  function render(statuses) {
    var app = document.getElementById("app");
    if (!statuses || statuses.length === 0) {
      app.innerHTML = '<div class="empty"><div class="empty-title">No remote servers configured</div><p>Add remote NAS Doctor instances in <a href="/settings" style="color:#5e6ad2">Settings</a> to monitor your fleet from here.</p></div>';
      return;
    }

    var h = '<div class="grid">';
    for (var i = 0; i < statuses.length; i++) {
      var s = statuses[i];
      var healthClass = s.online ? (s.overall_health || "healthy") : "offline";
      var healthLabel = s.online ? (s.overall_health || "unknown") : "offline";
      var url = s.server.url || "#";

      h += '<a class="card-link" href="' + esc(url) + '" target="_blank">';
      h += '<div class="card">';
      h += '<div class="card-header">';
      h += '<div><div class="card-name">' + esc(s.server.name || s.hostname || "Unknown") + '</div>';
      if (s.hostname && s.hostname !== s.server.name) h += '<div class="card-hostname">' + esc(s.hostname) + '</div>';
      h += '</div>';
      h += '<div class="health-dot ' + healthClass + '" title="' + esc(healthLabel) + '"></div>';
      h += '</div>';

      if (s.online) {
        h += '<div class="stats">';
        h += '<span>Platform: <span class="stat-val">' + esc(s.platform) + '</span></span>';
        h += '<span>Uptime: <span class="stat-val">' + esc(s.uptime) + '</span></span>';
        h += '</div>';
        if (s.critical_count > 0 || s.warning_count > 0 || s.info_count > 0) {
          h += '<div class="findings-row">';
          if (s.critical_count > 0) h += '<span class="pill pill-crit">' + s.critical_count + ' critical</span>';
          if (s.warning_count > 0) h += '<span class="pill pill-warn">' + s.warning_count + ' warning</span>';
          if (s.info_count > 0) h += '<span class="pill pill-info">' + s.info_count + ' info</span>';
          h += '</div>';
        }
      } else {
        h += '<div class="error-msg">' + esc(s.error || "Offline") + '</div>';
      }

      h += '</div></a>';
    }
    h += '</div>';
    app.innerHTML = h;
  }

  function load() {
    fetch("/api/v1/fleet")
      .then(function(r) { return r.json(); })
      .then(function(data) { render(data); })
      .catch(function() { render([]); });
  }

  load();
  setInterval(load, 60000);
})();
</script>
</body>
</html>`
