package api

// DashboardJS is a shared vanilla JavaScript module that provides the dashboard
// rendering engine for NAS Doctor. It extracts the duplicated inline JavaScript
// from the two dashboard theme templates (midnight, clean) into a single
// reusable module served at /js/dashboard.js.
//
// It exposes a global NasDashboard object with:
//   - NasDashboard.util.*      — shared utility functions (esc, fmtBytes, etc.)
//   - NasDashboard.polling.*   — status polling and snapshot fetching
//   - NasDashboard.sections.*  — per-section HTML renderers
//   - NasDashboard.init(cfg)   — initializer that wires everything together
//
// Each theme template only needs to include this script and call NasDashboard.init()
// with a theme config object. The theme template retains its own HTML structure and CSS.
var DashboardJS = `
(function(){
"use strict";

/* ================================================================
   NasDashboard — Shared dashboard rendering engine
   ================================================================ */

/* ── Utilities ─────────────────────────────────────────────────── */
var util = {};

util.esc = function(s) {
  if (!s && s !== 0) return "";
  var d = document.createElement("div");
  d.appendChild(document.createTextNode(String(s)));
  return d.innerHTML;
};

util.fmtBytes = function(b) {
  if (!b || b <= 0) return "0 B";
  var units = ["B", "KB", "MB", "GB", "TB"];
  var i = 0;
  while (b >= 1024 && i < units.length - 1) { b /= 1024; i++; }
  return b.toFixed(i === 0 ? 0 : 1) + " " + units[i];
};

util.fmtUptime = function(s) {
  if (!s) return "N/A";
  if (typeof s === "string") return s;
  var days = Math.floor(s / 86400);
  var hours = Math.floor((s % 86400) / 3600);
  if (days > 0) return days + "d " + hours + "h";
  var mins = Math.floor((s % 3600) / 60);
  return hours + "h " + mins + "m";
};

util.fmtPercent = function(n) {
  if (n == null) return "0%";
  return n.toFixed(1) + "%";
};

util.fmtGB = function(gb) {
  if (!gb) return "0 GB";
  if (gb >= 1000) return (gb / 1000).toFixed(1) + " TB";
  return gb.toFixed(0) + " GB";
};

util.fetchJSON = function(url) {
  return fetch(url).then(function(r) {
    if (!r.ok) throw new Error(r.status + " " + r.statusText);
    return r.json();
  });
};

util.colorForPct = function(pct) {
  if (pct >= 90) return "var(--red)";
  if (pct >= 75) return "var(--amber)";
  return "var(--green)";
};

util.classForPct = function(pct) {
  if (pct >= 90) return "text-red";
  if (pct >= 75) return "text-amber";
  return "text-green";
};

util.severityClass = function(sev) {
  if (sev === "critical") return "td-crit";
  if (sev === "warning") return "td-warn";
  if (sev === "info") return "text-brand";
  return "td-healthy";
};

/* ── Polling ───────────────────────────────────────────────────── */
var polling = {};
var _pollTimer = null;
var _statusData = null;
var _snapshotData = null;
var _scanInProgress = false;
var _lastScanTime = null;
var _wasScanRunning = false;
var _lastFetchTime = 0;
var _chartRange = 1;
var _renderFn = null;
var _onRenderComplete = null;
var POLL_MS = 60000;
var SCAN_POLL_MS = 15000;

polling.statusData = function() { return _statusData; };
polling.snapshotData = function() { return _snapshotData; };
polling.scanInProgress = function() { return _scanInProgress; };
polling.chartRange = function() { return _chartRange; };
polling.setChartRange = function(h) { _chartRange = h; };
polling.lastFetchTime = function() { return _lastFetchTime; };

polling._pollStatus = function() {
  util.fetchJSON("/api/v1/status").then(function(d) {
    _statusData = d;
    var scanTime = d.last_scan || null;
    var scanRunning = !!d.scan_running;

    if (scanTime && scanTime !== _lastScanTime && _lastScanTime !== null) {
      _lastScanTime = scanTime;
      util.fetchJSON("/api/v1/snapshot/latest").then(function(snap) {
        _snapshotData = snap;
        if (_renderFn) _renderFn();
      }).catch(function() {});
    } else {
      if (!_lastScanTime) _lastScanTime = scanTime;
    }

    if (scanRunning !== _wasScanRunning) {
      var wasRunning = _wasScanRunning;
      _wasScanRunning = scanRunning;
      _scanInProgress = scanRunning;
      polling._startPoll();
      if (wasRunning && !scanRunning) {
        _lastScanTime = scanTime;
        util.fetchJSON("/api/v1/snapshot/latest").then(function(snap) {
          _snapshotData = snap;
          if (_renderFn) _renderFn();
        }).catch(function() {});
      } else if (_renderFn) {
        _renderFn();
      }
    }
  }).catch(function() {});
  _lastFetchTime = Date.now();
};

polling._startPoll = function() {
  if (_pollTimer) clearInterval(_pollTimer);
  var ms = _wasScanRunning ? SCAN_POLL_MS : POLL_MS;
  _pollTimer = setInterval(polling._pollStatus, ms);
};

polling.loadAll = function() {
  return Promise.all([
    util.fetchJSON("/api/v1/status").then(function(d) { _statusData = d; }).catch(function() { _statusData = null; }),
    util.fetchJSON("/api/v1/snapshot/latest").then(function(d) { _snapshotData = d; }).catch(function() { _snapshotData = null; })
  ]).then(function() {
    if (_renderFn) _renderFn();
    _lastFetchTime = Date.now();
    var dot = document.getElementById("refresh-dot");
    if (dot) {
      dot.classList.remove("pulse");
      void dot.offsetWidth;
      dot.classList.add("pulse");
    }
  });
};

polling.triggerScan = function() {
  if (_scanInProgress) return;
  _scanInProgress = true;
  if (_renderFn) _renderFn();
  fetch("/api/v1/scan", { method: "POST" })
    .then(function(r) { return r.json(); })
    .then(function() {
      setTimeout(function() {
        _scanInProgress = false;
        polling.loadAll();
      }, 5000);
    })
    .catch(function() {
      _scanInProgress = false;
      if (_renderFn) _renderFn();
    });
};

polling.saveChartRange = function(hours) {
  _chartRange = hours;
  fetch("/api/v1/settings/chart-range", { method: "PUT", headers: {"Content-Type":"application/json"}, body: JSON.stringify({hours: hours}) }).catch(function() {});
};

/* ── Section Renderers ─────────────────────────────────────────── */
var sections = {};

/* Helper: render a container metrics card */
sections._renderContainerCard = function(cm, idx) {
  var esc = util.esc;
  var fmtBytes = util.fmtBytes;
  var h = '';
  var cpuClass = cm.cpu_percent > 200 ? 'td-crit' : cm.cpu_percent > 80 ? 'td-warn' : 'td-healthy';
  var memClass = cm.mem_percent > 95 ? 'td-crit' : cm.mem_percent > 80 ? 'td-warn' : 'td-healthy';
  h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:12px;margin-bottom:6px">';
  h += '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px">';
  h += '<span style="font-weight:600;font-size:13px;color:var(--text-primary)">' + esc(cm.name) + '</span>';
  h += '<span style="font-size:11px;color:var(--text-quaternary);max-width:140px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(cm.image) + '</span>';
  h += '</div>';
  h += '<div style="display:flex;gap:16px;font-size:12px;color:var(--text-tertiary);flex-wrap:wrap;margin-bottom:8px">';
  h += '<span class="' + cpuClass + '">CPU: <strong>' + (cm.cpu_percent || 0).toFixed(1) + '%</strong></span>';
  h += '<span class="' + memClass + '">Mem: <strong>' + (cm.mem_mb || 0).toFixed(0) + ' MB (' + (cm.mem_percent || 0).toFixed(1) + '%)</strong></span>';
  if (cm.net_in_bytes > 0 || cm.net_out_bytes > 0) h += '<span>Net: <strong style="color:var(--text-primary)">' + fmtBytes(cm.net_in_bytes) + ' / ' + fmtBytes(cm.net_out_bytes) + '</strong></span>';
  if (cm.block_read_bytes > 0 || cm.block_write_bytes > 0) h += '<span>Disk: <strong style="color:var(--text-primary)">' + fmtBytes(cm.block_read_bytes) + ' / ' + fmtBytes(cm.block_write_bytes) + '</strong></span>';
  h += '</div>';
  h += '<canvas id="cmetrics-chart-' + idx + '" data-container="' + esc(cm.name) + '" style="width:100%;height:60px"></canvas>';
  h += '</div>';
  return h;
};

/* Helper: render range buttons for chart sections */
sections._rangeButtons = function(cls, onclickFn, chartRange) {
  var h = '<div class="' + cls + '-range-btns" style="display:flex;gap:4px">';
  var ranges = [{h:1,l:"1H"},{h:24,l:"1D"},{h:168,l:"1W"}];
  for (var i = 0; i < ranges.length; i++) {
    var rr = ranges[i];
    var active = chartRange === rr.h;
    h += '<button class="' + cls + '-range-btn' + (active ? ' active' : '') + '" data-hours="' + rr.h + '" onclick="' + onclickFn + '(' + rr.h + ',true)" style="font-size:10px;padding:2px 8px;border-radius:4px;border:1px solid var(--border);background:' + (active ? 'var(--bg-elevated)' : 'transparent') + ';color:' + (active ? 'var(--text-secondary)' : 'var(--text-tertiary)') + ';cursor:pointer">' + rr.l + '</button>';
  }
  h += '</div>';
  return h;
};

/* ── Section: Findings ───────────────────────────────────────── */
sections.findings = function(sn, st) {
  var esc = util.esc;
  var h = '';
  h += '<div class="section-block" data-section="findings">';
  var findings = sn ? (sn.findings || []) : [];
  h += '<div class="section-title">Findings (' + findings.length + ')</div>';
  if (findings.length === 0) {
    h += '<div class="empty"><div class="empty-icon">&#9989;</div>No findings yet. Run a scan to check your NAS health.</div>';
  } else {
    var dismissed = (st && st.dismissed_findings) ? st.dismissed_findings : [];
    var visibleFindings = findings.filter(function(f) { return dismissed.indexOf(f.title) === -1; });
    var sortPref = (window.NasSort ? NasSort.getPrefs().findings : null) || "severity";
    if (window.NasSort) NasSort.sortFindings(visibleFindings, sortPref);
    else visibleFindings.sort(function(a, b) { return ({ critical:0,warning:1,info:2,ok:3 }[a.severity]||9) - ({ critical:0,warning:1,info:2,ok:3 }[b.severity]||9); });

    var sevCounts = { critical: 0, warning: 0, info: 0 };
    for (var fc = 0; fc < visibleFindings.length; fc++) { var sv = visibleFindings[fc].severity; if (sevCounts[sv] !== undefined) sevCounts[sv]++; }
    var activeFilters = window._findingSevFilters || {};
    var hasFilter = activeFilters.critical || activeFilters.warning || activeFilters.info;
    h += '<div style="display:flex;align-items:center;gap:6px;margin-bottom:8px;flex-wrap:wrap">';
    function sevBadge(sev, count, color, bg) {
      if (count === 0) return '';
      var isActive = activeFilters[sev];
      var style = 'font-size:10px;font-weight:600;padding:2px 8px;border-radius:4px;cursor:pointer;transition:all 0.12s;border:1px solid transparent;';
      if (isActive) style += 'color:#fff;background:' + color + ';border-color:' + color + ';';
      else style += 'color:' + color + ';background:' + bg + ';';
      return '<span style="' + style + '" onclick="window._toggleSevFilter(\'' + sev + '\')">' + count + ' ' + sev + '</span>';
    }
    h += sevBadge('critical', sevCounts.critical, 'var(--red)', 'var(--red-bg)');
    h += sevBadge('warning', sevCounts.warning, 'var(--amber)', 'var(--amber-bg)');
    h += sevBadge('info', sevCounts.info, 'var(--accent)', 'rgba(94,106,210,0.1)');
    if (hasFilter) h += '<span style="font-size:10px;color:var(--text-quaternary);cursor:pointer;padding:2px 6px" onclick="window._clearSevFilters()">&times; Clear</span>';
    h += '<div id="findings-sort-mount" style="margin-left:auto"></div>';
    h += '</div>';
    if (hasFilter) {
      visibleFindings = visibleFindings.filter(function(f) { return activeFilters[f.severity]; });
    }
    h += '<div class="findings-list">';
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
  h += '</div>';
  return h;
};

/* ── Section: Drives ─────────────────────────────────────────── */
sections.drives = function(sn, st) {
  var esc = util.esc;
  var colorForPct = util.colorForPct;
  var h = '';
  h += '<div class="section-block" data-section="drives">';
  var smart = sn ? (sn.smart || []) : [];
  var disks = sn ? (sn.disks || []) : [];
  if (smart.length > 0 || disks.length > 0) {
    h += '<div>';
    var healthOk = 0, healthWarn = 0, healthCrit = 0;
    for (var hc = 0; hc < smart.length; hc++) {
      if (!smart[hc].health_passed) healthCrit++;
      else if (smart[hc].data_available === false) healthWarn++;
      else if ((smart[hc].temperature_c || 0) >= 50 || smart[hc].reallocated_sectors > 0 || smart[hc].pending_sectors > 0) healthWarn++;
      else healthOk++;
    }
    h += '<div class="section-title" style="display:flex;align-items:center;gap:12px">Drives (' + (smart.length || disks.length) + ')';
    h += '<span class="health-summary" style="display:inline-flex;gap:8px;font-size:11px;color:var(--text-quaternary);font-weight:400;text-transform:none;letter-spacing:0">';
    if (healthOk > 0) h += '<span style="display:flex;align-items:center;gap:3px"><span style="width:6px;height:6px;border-radius:50%;background:var(--green)"></span>' + healthOk + ' ok</span>';
    if (healthWarn > 0) h += '<span style="display:flex;align-items:center;gap:3px"><span style="width:6px;height:6px;border-radius:50%;background:var(--amber)"></span>' + healthWarn + ' warn</span>';
    if (healthCrit > 0) h += '<span style="display:flex;align-items:center;gap:3px"><span style="width:6px;height:6px;border-radius:50%;background:var(--red)"></span>' + healthCrit + ' fail</span>';
    h += '</span></div>';

    var driveSortPref = (window.NasSort ? NasSort.getPrefs().drives : null) || "device";
    if (window.NasSort) NasSort.sortDrives(smart, driveSortPref);
    h += '<div id="drives-sort-mount" style="margin-bottom:8px"></div>';

    var slotToDrive = {};
    if (smart && smart.length) {
      for (var si2 = 0; si2 < smart.length; si2++) {
        var sd = smart[si2];
        if (sd.array_slot) slotToDrive[sd.array_slot] = sd;
      }
    }
    var deviceToStorage = {};
    for (var dm = 0; dm < disks.length; dm++) {
      var dmp = disks[dm].mount_point || "";
      if (dmp.indexOf("/mnt/disk") === 0) {
        var sn2 = dmp.replace("/mnt/disk", "");
        var sdx = slotToDrive["disk" + sn2];
        if (sdx) deviceToStorage[sdx.device] = disks[dm];
      } else if (dmp.indexOf("/mnt/cache") === 0 && slotToDrive["cache"]) {
        deviceToStorage[slotToDrive["cache"].device] = disks[dm];
      } else if (dmp.indexOf("/volume") === 0) {
        for (var sx = 0; sx < smart.length; sx++) {
          if (!deviceToStorage[smart[sx].device] && Math.abs((smart[sx].size_gb || 0) - (disks[dm].total_gb || 0)) < 100) {
            deviceToStorage[smart[sx].device] = disks[dm];
            break;
          }
        }
      }
    }

    var mergedView = (st && st.sections && st.sections.merged_drives);

    if (smart.length > 0) {
      for (var si = 0; si < smart.length; si++) {
        var s = smart[si];
        var noData = s.data_available === false;
        var healthDot = noData ? "unknown" : (s.health_passed ? "running" : "exited");
        var tempClass = noData ? "td-unknown" : ((s.temperature_c || 0) >= 55 ? "td-crit" : (s.temperature_c || 0) >= 45 ? "td-warn" : "td-healthy");
        var sizeStr = s.size_gb >= 1000 ? (s.size_gb / 1000).toFixed(1) + ' TB' : (s.size_gb || 0).toFixed(0) + ' GB';
        var ageStr = noData ? '\u2014' : (s.power_on_hours > 8766 ? (s.power_on_hours / 8766).toFixed(1) + 'y' : (s.power_on_hours || 0).toLocaleString() + 'h');
        var slotLabel = s.array_slot || '';

        h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:10px 12px;margin-bottom:6px;cursor:pointer" onclick="window.location=\'/disk/' + encodeURIComponent(s.serial || '') + '\'">';
        h += '<div style="display:flex;align-items:center;gap:10px;flex-wrap:wrap">';
        h += '<span class="status-dot ' + healthDot + '"></span>';
        h += '<span style="font-weight:600;font-size:13px;min-width:55px">' + esc(s.device) + '</span>';
        if (slotLabel) h += '<span style="font-size:11px;color:var(--text-quaternary);background:var(--bg-elevated);padding:1px 6px;border-radius:4px">' + esc(slotLabel) + '</span>';
        h += '<span style="font-size:12px;color:var(--text-tertiary);flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(s.model) + '</span>';
        h += '<span style="font-size:12px;color:var(--text-tertiary)">' + sizeStr + '</span>';
        h += '<span class="' + tempClass + '" style="font-size:12px;font-weight:600">' + (noData ? '\u2014' : ((s.temperature_c || 0) + '&deg;C')) + '</span>';
        h += '<span style="font-size:11px;color:var(--text-quaternary)">' + ageStr + '</span>';
        h += '<canvas id="spark-temp-' + si + '" width="60" height="20" style="flex-shrink:0"></canvas>';
        if (noData) h += '<span style="font-size:10px;font-weight:600;color:var(--amber);background:rgba(217,119,6,0.1);padding:1px 6px;border-radius:9999px">NO DATA</span>';
        else if (!s.health_passed) h += '<span style="font-size:10px;font-weight:600;color:var(--red);background:rgba(220,38,38,0.1);padding:1px 6px;border-radius:9999px">FAILED</span>';
        if (s.reallocated_sectors > 0) h += '<span style="font-size:10px;color:var(--amber);background:rgba(217,119,6,0.1);padding:1px 6px;border-radius:9999px">' + s.reallocated_sectors + ' realloc</span>';
        if (s.pending_sectors > 0) h += '<span style="font-size:10px;color:var(--red);background:rgba(220,38,38,0.1);padding:1px 6px;border-radius:9999px">' + s.pending_sectors + ' pending</span>';
        if (s.udma_crc_errors > 0) h += '<span style="font-size:10px;color:var(--amber);background:rgba(217,119,6,0.1);padding:1px 6px;border-radius:9999px">' + s.udma_crc_errors + ' CRC</span>';
        h += '</div>';

        if (mergedView && deviceToStorage[s.device]) {
          var mdk = deviceToStorage[s.device];
          var mpct = mdk.used_percent || 0;
          h += '<div style="margin-top:6px;display:flex;align-items:center;gap:8px">';
          h += '<div style="flex:1"><div class="disk-bar-bg" style="height:4px"><div class="disk-bar-fill" style="height:4px;width:' + mpct.toFixed(1) + '%;background:' + colorForPct(mpct) + '"></div></div></div>';
          h += '<span style="font-size:11px;color:var(--text-quaternary);white-space:nowrap">' + (mdk.used_gb || 0).toFixed(0) + '/' + (mdk.total_gb || 0).toFixed(0) + ' GB (' + mpct.toFixed(0) + '%)</span>';
          h += '</div>';
        }

        h += '</div>';
      }
    }

    var usedInMerge = {};
    if (mergedView) {
      for (var mk in deviceToStorage) { usedInMerge[deviceToStorage[mk].mount_point] = true; }
    }
    var unmatchedDisks = disks.filter(function(d) { return !mergedView || !usedInMerge[d.mount_point]; });
    if (unmatchedDisks.length > 0) {
      h += '<div style="margin-top:10px;font-size:11px;color:var(--text-quaternary);text-transform:uppercase;letter-spacing:0.5px;margin-bottom:4px">Storage</div>';
      for (var di = 0; di < unmatchedDisks.length; di++) {
        var dk = unmatchedDisks[di];
        var pct = dk.used_percent || 0;
        var storageLabel = dk.label || dk.mount_point;
        var mp = dk.mount_point || "";
        if (mp.indexOf("/mnt/disk") === 0) {
          var slotNum = mp.replace("/mnt/disk", "");
          var slotDrive = slotToDrive["disk" + slotNum];
          if (slotDrive) storageLabel = esc(slotDrive.device || "") + " \u2014 " + esc(slotDrive.model || "");
        } else if (mp.indexOf("/mnt/cache") === 0 && slotToDrive["cache"]) {
          var cacheDrive = slotToDrive["cache"];
          storageLabel = esc(cacheDrive.device || "") + " \u2014 " + esc(cacheDrive.model || "");
        }
        h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:8px 12px;margin-bottom:4px">';
        h += '<div style="display:flex;justify-content:space-between;font-size:12px;margin-bottom:2px">';
        h += '<span style="font-weight:500">' + storageLabel + '</span>';
        h += '<span style="color:var(--text-tertiary)">' + (dk.used_gb || 0).toFixed(0) + ' / ' + (dk.total_gb || 0).toFixed(0) + ' GB (' + pct.toFixed(0) + '%)</span>';
        h += '</div>';
        h += '<div class="disk-bar-bg" style="height:4px"><div class="disk-bar-fill" style="height:4px;width:' + pct.toFixed(1) + '%;background:' + colorForPct(pct) + '"></div></div>';
        h += '</div>';
      }
    }
    h += '</div>';
  }
  h += '</div>';
  return h;
};

/* ── Section: Docker ─────────────────────────────────────────── */
sections.docker = function(sn, st) {
  var esc = util.esc;
  var h = '';
  h += '<div class="section-block" data-section="docker">';
  var docker = sn ? sn.docker : null;
  var mergedContainers = (st && st.sections && st.sections.merged_containers);
  if (docker && docker.available && docker.containers && docker.containers.length > 0) {
    var containers = docker.containers;
    var runningCsMerged = containers.filter(function(c) { return c.state === "running"; });
    var stoppedCs = containers.filter(function(c) { return c.state !== "running"; });
    h += '<div>';
    if (mergedContainers && runningCsMerged.length > 0) {
      h += '<div class="section-title" style="display:flex;align-items:center;justify-content:space-between">Docker Containers (' + containers.length + ')';
      h += sections._rangeButtons("cmetrics", "loadContainerChart", _chartRange);
      h += '</div>';
      for (var cmi = 0; cmi < runningCsMerged.length; cmi++) {
        h += sections._renderContainerCard(runningCsMerged[cmi], cmi);
      }
      if (stoppedCs.length > 0) {
        h += '<div style="font-size:11px;color:var(--text-quaternary);margin-top:8px">' + stoppedCs.length + ' stopped: ';
        h += stoppedCs.map(function(c) { return esc(c.name); }).join(', ');
        h += '</div>';
      }
    } else {
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
        h += '<td style="color:var(--text-quaternary);max-width:140px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(c.image) + '</td>';
        h += '<td>' + esc(c.status) + '</td>';
        h += '<td>' + (c.cpu_percent || 0).toFixed(1) + '%</td>';
        h += '<td>' + (c.mem_mb || 0).toFixed(0) + ' MB (' + (c.mem_percent || 0).toFixed(1) + '%)</td>';
        h += '<td>' + esc(c.uptime || "N/A") + '</td>';
        h += '</tr>';
      }
      h += '</tbody></table></div>';
    }
    h += '</div>';
  }
  h += '</div>';
  return h;
};

/* ── Section: Container Metrics (standalone) ─────────────────── */
sections.containerMetrics = function(sn, st) {
  var h = '';
  h += '<div class="section-block" data-section="container_metrics">';
  var mergedContainers = (st && st.sections && st.sections.merged_containers);
  if (!mergedContainers) {
    var dockerInfo = sn ? sn.docker : null;
    if (dockerInfo && dockerInfo.available && dockerInfo.containers) {
      var runningCs = dockerInfo.containers.filter(function(c) { return c.state === "running"; });
      if (runningCs.length > 0) {
        h += '<div>';
        h += '<div class="section-title" style="display:flex;align-items:center;justify-content:space-between">Container Metrics (' + runningCs.length + ')';
        h += sections._rangeButtons("cmetrics", "loadContainerChart", _chartRange);
        h += '</div>';
        for (var cmi2 = 0; cmi2 < runningCs.length; cmi2++) {
          h += sections._renderContainerCard(runningCs[cmi2], cmi2);
        }
        h += '</div>';
      }
    }
  }
  h += '</div>';
  return h;
};

/* ── Section: ZFS ────────────────────────────────────────────── */
sections.zfs = function(sn) {
  var esc = util.esc;
  var h = '';
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
      if (zp.vdevs && zp.vdevs.length > 0) {
        h += '<div style="margin-top:8px;font-size:11px;font-family:monospace;color:var(--text-tertiary)">';
        for (var vi = 0; vi < zp.vdevs.length; vi++) {
          var vd = zp.vdevs[vi];
          var vdClass = vd.state === "ONLINE" ? "td-healthy" : vd.state === "DEGRADED" ? "td-warn" : "td-crit";
          h += '<div>' + esc(vd.name) + ' <span class="' + vdClass + '">' + esc(vd.state) + '</span></div>';
          if (vd.children) {
            for (var vci = 0; vci < vd.children.length; vci++) {
              var ch = vd.children[vci];
              var chClass = ch.state === "ONLINE" ? "td-healthy" : ch.state === "DEGRADED" ? "td-warn" : "td-crit";
              h += '<div style="padding-left:16px">' + esc(ch.name) + ' <span class="' + chClass + '">' + esc(ch.state) + '</span></div>';
            }
          }
        }
        h += '</div>';
      }
      h += '</div>';
    }
    if (zfs.arc) {
      h += '<div style="font-size:11px;color:var(--text-quaternary);margin-top:6px">ARC: ' + (zfs.arc.size_mb / 1024).toFixed(1) + ' GB / ' + (zfs.arc.max_size_mb / 1024).toFixed(1) + ' GB &middot; Hit rate: ' + (zfs.arc.hit_rate_percent || 0).toFixed(1) + '%</div>';
    }
    h += '</div>';
  }
  h += '</div>';
  return h;
};

/* ── Section: GPU ────────────────────────────────────────────── */
sections.gpu = function(sn) {
  var esc = util.esc;
  var h = '';
  h += '<div class="section-block" data-section="gpu">';
  var gpuInfo = sn ? sn.gpu : null;
  if (gpuInfo && gpuInfo.available && gpuInfo.gpus && gpuInfo.gpus.length > 0) {
    h += '<div>';
    h += '<div class="section-title" style="display:flex;align-items:center;justify-content:space-between">GPU' + (gpuInfo.gpus.length > 1 ? 's (' + gpuInfo.gpus.length + ')' : '');
    h += sections._rangeButtons("gpu", "loadGPUChart", _chartRange);
    h += '</div>';
    for (var gi = 0; gi < gpuInfo.gpus.length; gi++) {
      var g = gpuInfo.gpus[gi];
      var gName = g.name || (g.vendor + ' GPU ' + g.index);
      var tempClass = g.temperature_c >= 95 ? 'td-crit' : g.temperature_c >= 85 ? 'td-warn' : 'td-healthy';
      h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:12px;margin-bottom:6px">';
      h += '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px">';
      h += '<span style="font-weight:600;font-size:13px;color:var(--text-primary)">' + esc(gName) + '</span>';
      h += '<span style="font-size:11px;color:var(--text-quaternary);text-transform:uppercase">' + esc(g.vendor) + (g.driver ? ' \u00b7 ' + esc(g.driver) : '') + '</span>';
      h += '</div>';
      h += '<div style="display:flex;gap:16px;font-size:12px;color:var(--text-tertiary);flex-wrap:wrap;margin-bottom:8px">';
      h += '<span>GPU: <strong style="color:var(--text-primary)">' + (g.usage_percent || 0).toFixed(0) + '%</strong></span>';
      h += '<span class="' + tempClass + '">Temp: <strong>' + (g.temperature_c || 0) + '\u00b0C</strong></span>';
      if (g.mem_total_mb > 0) h += '<span>VRAM: <strong style="color:var(--text-primary)">' + (g.mem_used_mb || 0).toFixed(0) + ' / ' + (g.mem_total_mb || 0).toFixed(0) + ' MB</strong></span>';
      if (g.power_watts > 0) h += '<span>Power: <strong style="color:var(--text-primary)">' + (g.power_watts || 0).toFixed(0) + 'W' + (g.power_max_watts > 0 ? ' / ' + (g.power_max_watts).toFixed(0) + 'W' : '') + '</strong></span>';
      if (g.fan_percent > 0) h += '<span>Fan: ' + (g.fan_percent || 0).toFixed(0) + '%</span>';
      if (g.encoder_percent > 0 || g.decoder_percent > 0) h += '<span>Enc/Dec: ' + (g.encoder_percent || 0).toFixed(0) + '% / ' + (g.decoder_percent || 0).toFixed(0) + '%</span>';
      h += '</div>';
      h += '<canvas id="gpu-chart-' + gi + '" style="width:100%;height:60px"></canvas>';
      h += '</div>';
    }
    h += '</div>';
  }
  h += '</div>';
  return h;
};

/* ── Section: UPS ────────────────────────────────────────────── */
sections.ups = function(sn) {
  var esc = util.esc;
  var h = '';
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
  h += '</div>';
  return h;
};

/* ── Section: Backup ─────────────────────────────────────────── */
sections.backup = function(sn) {
  var esc = util.esc;
  var fmtBytes = util.fmtBytes;
  var h = '';
  h += '<div class="section-block" data-section="backup">';
  var backup = sn ? sn.backup : null;
  if (backup && backup.available && backup.jobs && backup.jobs.length > 0) {
    h += '<div>';
    h += '<div class="section-title">Backup Jobs (' + backup.jobs.length + ')</div>';
    for (var bi = 0; bi < backup.jobs.length; bi++) {
      var bj = backup.jobs[bi];
      var statusClass = bj.status === 'ok' ? 'td-healthy' : bj.status === 'warning' ? 'td-warn' : 'td-crit';
      var provLabel = bj.provider.toUpperCase();
      h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:12px;margin-bottom:6px">';
      h += '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px">';
      h += '<div style="display:flex;align-items:center;gap:8px">';
      h += '<span style="font-weight:600;font-size:13px;color:var(--text-primary)">' + esc(bj.name) + '</span>';
      h += '<span style="font-size:10px;padding:2px 6px;border-radius:4px;background:var(--bg-elevated);color:var(--text-tertiary)">' + provLabel + '</span>';
      h += '</div>';
      h += '<span class="' + statusClass + '" style="font-size:12px;font-weight:600">' + bj.status.toUpperCase() + '</span>';
      h += '</div>';
      h += '<div style="display:flex;gap:16px;font-size:12px;color:var(--text-tertiary);flex-wrap:wrap">';
      if (bj.snapshot_count) h += '<span>Snapshots: <strong style="color:var(--text-primary)">' + bj.snapshot_count + '</strong></span>';
      if (bj.size_bytes > 0) h += '<span>Size: <strong style="color:var(--text-primary)">' + fmtBytes(bj.size_bytes) + '</strong></span>';
      if (bj.last_success) { var age = Math.round((Date.now() - new Date(bj.last_success).getTime()) / 3600000); h += '<span>Last: <strong style="color:var(--text-primary)">' + (age < 1 ? '<1h ago' : age + 'h ago') + '</strong></span>'; }
      if (bj.encrypted) h += '<span style="color:var(--text-quaternary)">Encrypted</span>';
      h += '</div>';
      h += '</div>';
    }
    h += '</div>';
  }
  h += '</div>';
  return h;
};

/* ── Section: Network ────────────────────────────────────────── */
sections.network = function(sn) {
  var esc = util.esc;
  var h = '';
  h += '<div class="section-block" data-section="network">';
  var net = sn ? sn.network : null;
  if (net && net.interfaces && net.interfaces.length > 0) {
    h += '<div>';
    h += '<div class="section-title">Network</div>';
    h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);overflow-x:auto">';
    h += '<table style="width:100%;font-size:12px;border-collapse:collapse">';
    h += '<tr style="color:var(--text-quaternary);font-size:10px;text-transform:uppercase;letter-spacing:0.5px">';
    h += '<th style="text-align:left;padding:8px 12px;border-bottom:1px solid var(--border)">Interface</th>';
    h += '<th style="text-align:left;padding:8px 12px;border-bottom:1px solid var(--border)">State</th>';
    h += '<th style="text-align:left;padding:8px 12px;border-bottom:1px solid var(--border)">Speed</th>';
    h += '<th style="text-align:left;padding:8px 12px;border-bottom:1px solid var(--border)">MTU</th>';
    h += '<th style="text-align:left;padding:8px 12px;border-bottom:1px solid var(--border)">IP</th></tr>';
    for (var ni = 0; ni < net.interfaces.length; ni++) {
      var iface = net.interfaces[ni];
      var stateColor = iface.state === "UP" ? "td-healthy" : "td-warn";
      h += '<tr><td style="padding:6px 12px;border-bottom:1px solid var(--border)">' + esc(iface.name) + '</td>';
      h += '<td style="padding:6px 12px;border-bottom:1px solid var(--border)" class="' + stateColor + '">' + esc(iface.state) + '</td>';
      h += '<td style="padding:6px 12px;border-bottom:1px solid var(--border)">' + esc(iface.speed || "\u2014") + '</td>';
      h += '<td style="padding:6px 12px;border-bottom:1px solid var(--border)">' + (iface.mtu || 0) + '</td>';
      h += '<td style="padding:6px 12px;border-bottom:1px solid var(--border)">' + esc(iface.ipv4 || "\u2014") + '</td></tr>';
    }
    h += '</table>';
    h += '</div>';
    h += '</div>';
  }
  h += '</div>';
  return h;
};

/* ── Section: Speed Test ─────────────────────────────────────── */
sections.speedtest = function(sn) {
  var esc = util.esc;
  var h = '';
  h += '<div class="section-block" data-section="speedtest">';
  var spd = sn ? sn.speed_test : null;
  if (spd && spd.available && spd.latest) {
    var r = spd.latest;
    h += '<div>';
    h += '<div class="section-title" style="display:flex;align-items:center;justify-content:space-between">Speed Test';
    h += sections._rangeButtons("st", "loadSpeedTestChart", _chartRange);
    h += '</div>';
    h += '<div style="display:flex;gap:16px;font-size:13px;color:var(--text-tertiary);flex-wrap:wrap;margin-bottom:12px">';
    h += '<span>Download: <strong style="color:var(--text-primary);font-size:15px">' + r.download_mbps.toFixed(0) + ' Mbps</strong></span>';
    h += '<span>Upload: <strong style="color:var(--text-primary);font-size:15px">' + r.upload_mbps.toFixed(0) + ' Mbps</strong></span>';
    h += '<span>Latency: <strong style="color:var(--text-primary)">' + r.latency_ms.toFixed(1) + ' ms</strong></span>';
    if (r.jitter_ms) h += '<span>Jitter: <strong style="color:var(--text-primary)">' + r.jitter_ms.toFixed(1) + ' ms</strong></span>';
    h += '</div>';
    h += '<div style="font-size:11px;color:var(--text-quaternary);margin-bottom:8px">';
    if (r.server_name) h += 'Server: ' + esc(r.server_name) + ' &middot; ';
    if (r.isp) h += 'ISP: ' + esc(r.isp);
    h += '</div>';
    h += '<canvas id="speedtest-chart" style="width:100%;height:80px"></canvas>';
    h += '</div>';
  }
  h += '</div>';
  return h;
};

/* ── Section: Service Checks ─────────────────────────────────── */
sections.serviceChecks = function(sn) {
  var esc = util.esc;
  var h = '';
  h += '<div class="section-block" data-section="services">';
  var svcChecks = sn ? (sn.service_checks || []) : [];
  if (svcChecks && svcChecks.length > 0) {
    h += '<div>';
    h += '<div class="section-title">Service Checks (' + svcChecks.length + ')</div>';
    h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);overflow:hidden">';
    h += '<table style="width:100%;font-size:12px;border-collapse:collapse">';
    h += '<tr style="color:var(--text-quaternary);font-size:10px;text-transform:uppercase;letter-spacing:0.5px">';
    h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Name</th>';
    h += '<th style="text-align:center;padding:6px 8px;border-bottom:1px solid var(--border)">Health</th>';
    h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Type</th>';
    h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Status</th>';
    h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Latency</th>';
    h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Failures</th>';
    h += '</tr>';
    for (var svi = 0; svi < svcChecks.length; svi++) {
      var sc = svcChecks[svi];
      var scStatus = (sc.status || '').toLowerCase();
      var scClass = scStatus === 'up' ? 'td-healthy' : 'td-crit';
      var fails = sc.consecutive_failures || 0;
      var dots = ''; for (var di = 0; di < 8; di++) { var isRed = di >= (8 - fails); dots += '<span style="display:inline-block;width:4px;height:12px;border-radius:1px;margin-right:1px;background:' + (isRed ? 'var(--red)' : 'var(--green)') + '"></span>'; }
      h += '<tr style="cursor:pointer" data-sc-filter="' + esc(sc.name || '') + '">';
      h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)"><div style="font-weight:500">' + esc(sc.name || 'Service') + '</div><div style="font-size:11px;color:var(--text-quaternary)">' + esc(sc.target || '') + '</div></td>';
      h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border);text-align:center"><div style="display:inline-flex;align-items:center;gap:0">' + dots + '</div></td>';
      h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)">' + esc((sc.type || '').toUpperCase()) + '</td>';
      h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)" class="' + scClass + '">' + esc(sc.status || 'unknown') + '</td>';
      h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)">' + ((sc.response_ms != null) ? (sc.response_ms + ' ms') : '\u2014') + '</td>';
      h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)">' + (sc.consecutive_failures || 0) + ' / ' + (sc.failure_threshold || 1) + '</td>';
      h += '</tr>';
    }
    h += '</table>';
    h += '</div>';
    h += '</div>';
  }
  h += '</div>';
  return h;
};

/* ── Section: Tunnels ────────────────────────────────────────── */
sections.tunnels = function(sn) {
  var esc = util.esc;
  var h = '';
  h += '<div class="section-block" data-section="tunnels">';
  var tunnels = sn ? sn.tunnels : null;
  if (tunnels && (tunnels.cloudflared || tunnels.tailscale)) {
    h += '<div>';
    h += '<div class="section-title">Tunnels</div>';
    h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:12px">';
    if (tunnels.cloudflared && tunnels.cloudflared.tunnels && tunnels.cloudflared.tunnels.length > 0) {
      h += '<div style="font-size:11px;color:var(--text-quaternary);margin-bottom:6px;text-transform:uppercase;letter-spacing:0.5px">Cloudflared ' + esc(tunnels.cloudflared.version || '') + '</div>';
      for (var ti = 0; ti < tunnels.cloudflared.tunnels.length; ti++) {
        var ct = tunnels.cloudflared.tunnels[ti];
        var ctColor = ct.status === 'healthy' ? 'var(--green)' : ct.status === 'degraded' ? 'var(--amber)' : 'var(--red)';
        h += '<div style="display:flex;align-items:center;gap:8px;padding:6px 0;border-bottom:1px solid var(--border)">';
        h += '<span style="width:8px;height:8px;border-radius:50%;background:' + ctColor + ';flex-shrink:0"></span>';
        h += '<span style="font-weight:600;font-size:13px">' + esc(ct.name) + '</span>';
        h += '<span style="font-size:11px;color:var(--text-tertiary)">' + ct.connections + ' conn</span>';
        if (ct.routes && ct.routes.length > 0) h += '<span style="font-size:11px;color:var(--text-quaternary)">' + esc(ct.routes.join(', ')) + '</span>';
        h += '</div>';
      }
    }
    if (tunnels.tailscale && tunnels.tailscale.self) {
      var ts = tunnels.tailscale;
      h += '<div style="font-size:11px;color:var(--text-quaternary);margin:12px 0 6px;text-transform:uppercase;letter-spacing:0.5px">Tailscale ' + esc(ts.version || '') + (ts.tailnet_name ? ' &middot; ' + esc(ts.tailnet_name) : '') + '</div>';
      var allNodes = [ts.self].concat(ts.peers || []);
      for (var ni = 0; ni < allNodes.length; ni++) {
        var nd = allNodes[ni];
        var ndColor = nd.online ? 'var(--green)' : 'var(--text-quaternary)';
        h += '<div style="display:flex;align-items:center;gap:8px;padding:5px 0;border-bottom:1px solid var(--border);font-size:12px">';
        h += '<span style="width:8px;height:8px;border-radius:50%;background:' + ndColor + ';flex-shrink:0"></span>';
        h += '<span style="font-weight:600;min-width:80px">' + esc(nd.name) + '</span>';
        h += '<span style="font-family:var(--font-mono);font-size:11px;color:var(--text-tertiary);min-width:90px">' + esc(nd.ip || '') + '</span>';
        h += '<span style="font-size:11px;color:var(--text-quaternary)">' + esc(nd.os || '') + '</span>';
        if (ni === 0) h += '<span style="font-size:10px;padding:1px 6px;border-radius:999px;background:rgba(94,106,210,0.15);color:var(--accent)">self</span>';
        if (nd.exit_node) h += '<span style="font-size:10px;padding:1px 6px;border-radius:999px;background:rgba(217,119,6,0.15);color:var(--amber)">exit node</span>';
        h += '</div>';
      }
    }
    h += '</div>';
    h += '</div>';
  }
  h += '</div>';
  return h;
};

/* ── Section: Proxmox ────────────────────────────────────────── */
sections.proxmox = function(sn) {
  var esc = util.esc;
  var h = '';
  h += '<div class="section-block" data-section="proxmox">';
  var pve = sn ? sn.proxmox : null;
  if (pve) {
    h += '<div>';
    var pveTitle = pve.alias ? esc(pve.alias) : 'Proxmox VE';
    h += '<div class="section-title">' + pveTitle + (pve.cluster_name ? ' &middot; ' + esc(pve.cluster_name) : '') + (pve.version ? ' <span style="font-size:11px;color:var(--text-quaternary);font-weight:400">v' + esc(pve.version) + '</span>' : '') + '</div>';
    if (pve.error) {
      h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:12px;color:var(--red);font-size:12px">' + esc(pve.error) + '</div>';
    } else if (!pve.connected) {
      h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:12px;color:var(--text-tertiary);font-size:12px">Not connected. Configure API credentials in Settings.</div>';
    } else {
      if (pve.nodes && pve.nodes.length > 0) {
        h += '<div style="display:flex;gap:8px;flex-wrap:wrap;margin-bottom:12px">';
        for (var ni = 0; ni < pve.nodes.length; ni++) {
          var nd = pve.nodes[ni];
          var ndOnline = nd.status === 'online';
          var memPct = nd.mem_total > 0 ? (nd.mem_used / nd.mem_total * 100) : 0;
          var uptStr = nd.uptime > 86400 ? Math.floor(nd.uptime/86400) + 'd' : Math.floor(nd.uptime/3600) + 'h';
          h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:10px 14px;flex:1;min-width:200px">';
          h += '<div style="display:flex;align-items:center;gap:6px;margin-bottom:6px">';
          h += '<span style="width:8px;height:8px;border-radius:50%;background:' + (ndOnline ? 'var(--green)' : 'var(--red)') + '"></span>';
          h += '<span style="font-weight:600;font-size:13px">' + esc(nd.name) + '</span>';
          h += '<span style="font-size:11px;color:var(--text-quaternary);margin-left:auto">' + uptStr + '</span>';
          h += '</div>';
          h += '<div style="display:flex;gap:12px;font-size:11px;color:var(--text-tertiary)">';
          h += '<span>CPU ' + (nd.cpu_usage * 100).toFixed(0) + '%</span>';
          h += '<span>Mem ' + memPct.toFixed(0) + '%</span>';
          h += '<span>' + nd.cpu_cores + ' cores</span>';
          h += '</div>';
          if (nd.cpu_model) h += '<div style="font-size:10px;color:var(--text-quaternary);margin-top:4px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(nd.cpu_model) + '</div>';
          h += '</div>';
        }
        h += '</div>';
      }
      if (pve.guests && pve.guests.length > 0) {
        h += '<div style="font-size:11px;color:var(--text-quaternary);text-transform:uppercase;letter-spacing:0.5px;margin-bottom:6px">Guests (' + pve.guests.length + ')</div>';
        h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);overflow:hidden">';
        h += '<table style="width:100%;font-size:12px;border-collapse:collapse">';
        h += '<tr style="color:var(--text-quaternary);font-size:10px;text-transform:uppercase;letter-spacing:0.5px">';
        h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">ID</th>';
        h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Name</th>';
        h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Type</th>';
        h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Node</th>';
        h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Status</th>';
        h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">CPU</th>';
        h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Memory</th>';
        h += '</tr>';
        for (var gi = 0; gi < pve.guests.length; gi++) {
          var g = pve.guests[gi];
          var gRunning = g.status === 'running';
          var gMemPct = g.mem_max > 0 ? (g.mem_used / g.mem_max * 100).toFixed(0) + '%' : '\u2014';
          var gMemStr = g.mem_max > 0 ? (g.mem_max / 1073741824).toFixed(1) + ' GB' : '\u2014';
          var gType = g.type === 'qemu' ? 'VM' : 'LXC';
          h += '<tr>';
          h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border);font-family:var(--font-mono);font-size:11px">' + g.vmid + '</td>';
          h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border);font-weight:500">' + esc(g.name) + '</td>';
          h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)"><span style="font-size:10px;padding:1px 6px;border-radius:999px;background:' + (g.type === 'qemu' ? 'rgba(94,106,210,0.12);color:var(--accent)' : 'rgba(245,158,11,0.12);color:var(--amber)') + '">' + gType + '</span></td>';
          h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border);font-size:11px;color:var(--text-tertiary)">' + esc(g.node) + '</td>';
          h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)"><span style="color:' + (gRunning ? 'var(--green)' : 'var(--text-quaternary)') + ';font-weight:600">' + esc(g.status) + '</span></td>';
          h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border);font-size:11px">' + (gRunning ? (g.cpu_usage * 100).toFixed(0) + '%' : '\u2014') + '</td>';
          h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border);font-size:11px">' + (gRunning ? gMemPct + ' / ' + gMemStr : gMemStr) + '</td>';
          h += '</tr>';
        }
        h += '</table></div>';
      }
      if (pve.storage && pve.storage.length > 0) {
        h += '<div style="font-size:11px;color:var(--text-quaternary);text-transform:uppercase;letter-spacing:0.5px;margin:12px 0 6px">Storage Pools (' + pve.storage.length + ')</div>';
        h += '<div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:6px">';
        for (var si = 0; si < pve.storage.length; si++) {
          var pst = pve.storage[si];
          var stPct = pst.used_pct || 0;
          var stColor = stPct >= 90 ? 'var(--red)' : stPct >= 75 ? 'var(--amber)' : 'var(--green)';
          var stTotal = pst.total > 1099511627776 ? (pst.total / 1099511627776).toFixed(1) + ' TB' : (pst.total / 1073741824).toFixed(0) + ' GB';
          h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:var(--radius);padding:8px 10px">';
          h += '<div style="display:flex;justify-content:space-between;font-size:12px;margin-bottom:4px"><span style="font-weight:500">' + esc(pst.storage) + '</span><span style="color:var(--text-quaternary);font-size:10px">' + esc(pst.node) + ' &middot; ' + esc(pst.type) + '</span></div>';
          h += '<div style="height:4px;background:var(--border);border-radius:2px;overflow:hidden"><div style="height:100%;width:' + stPct.toFixed(1) + '%;background:' + stColor + ';border-radius:2px"></div></div>';
          h += '<div style="display:flex;justify-content:space-between;font-size:10px;color:var(--text-quaternary);margin-top:3px"><span>' + stPct.toFixed(0) + '% used</span><span>' + stTotal + '</span></div>';
          h += '</div>';
        }
        h += '</div>';
      }
    }
    h += '</div>';
  }
  h += '</div>';
  return h;
};

/* ── Section: Kubernetes ─────────────────────────────────────── */
sections.kubernetes = function(sn) {
  var esc = util.esc;
  var h = '';
  var k8s = sn ? sn.kubernetes : null;
  var k8sConnected = k8s && k8s.connected && !k8s.error;
  var k8sTitle = k8s ? (k8s.alias ? esc(k8s.alias) : 'K8s') : 'K8s';
  var k8sBadge = (k8s && k8s.platform ? ' <span style="font-size:10px;padding:1px 6px;border-radius:999px;background:rgba(94,106,210,0.12);color:var(--accent)">' + esc(k8s.platform) + '</span>' : '');

  // K8s Nodes
  h += '<div class="section-block" data-section="k8s_nodes">';
  if (k8sConnected && k8s.nodes && k8s.nodes.length > 0) {
    h += '<div>';
    h += '<div class="section-title">' + k8sTitle + ' Nodes' + k8sBadge + (k8s.version ? ' <span style="font-size:11px;color:var(--text-quaternary);font-weight:400">' + esc(k8s.version) + '</span>' : '') + '</div>';
    h += '<div style="display:flex;gap:8px;flex-wrap:wrap">';
    for (var ki = 0; ki < k8s.nodes.length; ki++) {
      var kn = k8s.nodes[ki]; var knReady = kn.status === 'Ready';
      h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:10px 14px;flex:1;min-width:200px">';
      h += '<div style="display:flex;align-items:center;gap:6px;margin-bottom:6px"><span style="width:8px;height:8px;border-radius:50%;background:' + (knReady ? 'var(--green)' : 'var(--red)') + '"></span><span style="font-weight:600;font-size:13px">' + esc(kn.name) + '</span>';
      if (kn.roles) h += '<span style="font-size:10px;padding:1px 6px;border-radius:999px;background:rgba(94,106,210,0.1);color:var(--accent)">' + esc(kn.roles) + '</span>';
      h += '</div>';
      h += '<div style="font-size:11px;color:var(--text-tertiary)">' + kn.cpu_cores + ' cores &middot; ' + (kn.mem_total > 0 ? (kn.mem_total/1073741824).toFixed(0) + ' GB RAM' : '?') + ' &middot; ' + kn.pod_count + '/' + kn.pod_capacity + ' pods</div>';
      if (kn.disk_total > 0) { var du=kn.disk_total-kn.disk_allocatable,dp=du/kn.disk_total*100,dc=dp>=90?'var(--red)':dp>=75?'var(--amber)':'var(--green)',ds=kn.disk_total>=1073741824?(kn.disk_total/1073741824).toFixed(0)+' GB':(kn.disk_total/1048576).toFixed(0)+' MB'; h += '<div style="margin-top:4px"><div style="display:flex;justify-content:space-between;font-size:10px;color:var(--text-quaternary);margin-bottom:2px"><span>Disk</span><span>'+dp.toFixed(0)+'% of '+ds+'</span></div><div style="height:3px;background:var(--border);border-radius:2px;overflow:hidden"><div style="height:100%;width:'+dp.toFixed(1)+'%;background:'+dc+'"></div></div></div>'; }
      if (kn.conditions && kn.conditions.length > 0) h += '<div style="font-size:10px;color:var(--amber);margin-top:4px">' + esc(kn.conditions.join(', ')) + '</div>';
      h += '</div>';
    }
    h += '</div></div>';
  } else if (k8s && k8s.error) {
    h += '<div><div class="section-title">' + k8sTitle + ' Nodes</div><div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:12px;color:var(--red);font-size:12px">' + esc(k8s.error) + '</div></div>';
  }
  h += '</div>';

  // K8s Problem Pods
  var _k8sRunning = [], _k8sProblems = [], _k8sByNode = {};
  if (k8sConnected && k8s.pods && k8s.pods.length > 0) {
    _k8sRunning = k8s.pods.filter(function(p){ return p.status === 'Running'; });
    _k8sProblems = k8s.pods.filter(function(p){ return p.status !== 'Running' && p.status !== 'Succeeded'; });
    for (var pi = 0; pi < _k8sRunning.length; pi++) { var nd2 = _k8sRunning[pi].node || 'unassigned'; if (!_k8sByNode[nd2]) _k8sByNode[nd2] = []; _k8sByNode[nd2].push(_k8sRunning[pi]); }
  }

  h += '<div class="section-block" data-section="k8s_problems">';
  if (_k8sProblems.length > 0) {
    h += '<div>';
    h += '<div class="section-title">' + k8sTitle + ' Problem Pods (' + _k8sProblems.length + ')</div>';
    h += '<div style="background:rgba(220,38,38,0.08);border:1px solid rgba(220,38,38,0.2);border-radius:calc(var(--radius)*1.5);padding:8px 12px">';
    for (var pp = 0; pp < _k8sProblems.length; pp++) { var prb = _k8sProblems[pp]; h += '<div style="display:flex;align-items:center;gap:8px;padding:3px 0;font-size:12px"><span style="color:var(--red);font-weight:600">'+esc(prb.status)+'</span><span style="font-weight:500">'+esc(prb.name)+'</span><span style="color:var(--text-quaternary);font-size:11px">'+esc(prb.namespace)+'</span>'+(prb.restarts>0?'<span style="color:var(--amber);font-size:11px">'+prb.restarts+' restarts</span>':'')+'</div>'; }
    h += '</div></div>';
  }
  h += '</div>';

  // K8s per-node workloads
  var _k8sNodeNames = Object.keys(_k8sByNode).sort();
  for (var nni = 0; nni < _k8sNodeNames.length; nni++) {
    var nodeName = _k8sNodeNames[nni]; var nodePods = _k8sByNode[nodeName];
    var nodeInfo = null; for (var fi = 0; fi < (k8s ? k8s.nodes||[] : []).length; fi++) { if (k8s.nodes[fi].name === nodeName) { nodeInfo = k8s.nodes[fi]; break; } }
    h += '<div class="section-block" data-section="k8s_node_' + nni + '">';
    h += '<div>';
    h += '<div class="section-title">' + esc(nodeName) + ' <span style="font-size:11px;color:var(--text-quaternary);font-weight:400">' + nodePods.length + ' pods</span>';
    if (nodeInfo&&nodeInfo.roles) h+=' <span style="font-size:10px;padding:1px 6px;border-radius:999px;background:rgba(94,106,210,0.1);color:var(--accent)">'+esc(nodeInfo.roles)+'</span>';
    h += '</div>';
    h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:10px 12px">';
    if (nodeInfo&&nodeInfo.disk_total>0){var ndu=nodeInfo.disk_total-nodeInfo.disk_allocatable,ndp=ndu/nodeInfo.disk_total*100,ndc=ndp>=90?'var(--red)':ndp>=75?'var(--amber)':'var(--green)';h+='<div style="display:flex;align-items:center;gap:6px;margin-bottom:8px;font-size:10px;color:var(--text-quaternary)"><span>Disk '+ndp.toFixed(0)+'%</span><div style="flex:1;height:3px;background:var(--border);border-radius:2px;overflow:hidden"><div style="height:100%;width:'+ndp.toFixed(1)+'%;background:'+ndc+'"></div></div><span>'+(nodeInfo.disk_total>=1073741824?(nodeInfo.disk_total/1073741824).toFixed(0)+'G':(nodeInfo.disk_total/1048576).toFixed(0)+'M')+'</span></div>';}
    var nsPods = {}; for (var npi = 0; npi < nodePods.length; npi++) { var ns = nodePods[npi].namespace; if (!nsPods[ns]) nsPods[ns] = []; nsPods[ns].push(nodePods[npi]); }
    var nsKeys = Object.keys(nsPods).sort();
    for (var nsk = 0; nsk < nsKeys.length; nsk++) {
      h += '<div style="margin-bottom:6px"><div style="font-size:10px;color:var(--text-quaternary);text-transform:uppercase;letter-spacing:0.3px;margin-bottom:2px">' + esc(nsKeys[nsk]) + '</div>';
      for (var npj = 0; npj < nsPods[nsKeys[nsk]].length; npj++) { var pod = nsPods[nsKeys[nsk]][npj]; h += '<div style="display:flex;align-items:center;gap:6px;padding:2px 0;font-size:11px"><span style="width:6px;height:6px;border-radius:50%;background:var(--green);flex-shrink:0"></span><span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1">'+esc(pod.name)+'</span><span style="color:var(--text-quaternary);font-size:10px">'+esc(pod.ready)+'</span>'+(pod.restarts>0?'<span style="color:var(--amber);font-size:10px">'+pod.restarts+'r</span>':'')+'</div>'; }
      h += '</div>';
    }
    h += '</div></div>';
    h += '</div>';
  }

  // K8s Deployments
  h += '<div class="section-block" data-section="k8s_deployments">';
  if (k8sConnected && k8s.deployments && k8s.deployments.length > 0) {
    var unhealthyDeps = k8s.deployments.filter(function(d){ return d.unavailable > 0; });
    h += '<div>';
    h += '<div class="section-title">' + k8sTitle + ' Deployments (' + k8s.deployments.length + ')' + (unhealthyDeps.length > 0 ? ' <span style="color:var(--red)">' + unhealthyDeps.length + ' unhealthy</span>' : '') + '</div>';
    h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);overflow:hidden;max-height:250px;overflow-y:auto;scrollbar-width:thin">';
    h += '<table style="width:100%;font-size:12px;border-collapse:collapse"><tr style="color:var(--text-quaternary);font-size:10px;text-transform:uppercase;letter-spacing:0.5px;position:sticky;top:0;background:var(--bg-panel)"><th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Name</th><th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">NS</th><th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Ready</th><th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Strategy</th></tr>';
    for (var di = 0; di < k8s.deployments.length; di++) { var dep = k8s.deployments[di]; var dOk = dep.unavailable === 0 && dep.ready_replicas >= dep.replicas;
      h += '<tr><td style="padding:5px 8px;border-bottom:1px solid var(--border);font-weight:500;color:'+(dOk?'inherit':'var(--red)')+'">'+esc(dep.name)+'</td><td style="padding:5px 8px;border-bottom:1px solid var(--border);font-size:11px;color:var(--text-tertiary)">'+esc(dep.namespace)+'</td><td style="padding:5px 8px;border-bottom:1px solid var(--border);color:'+(dOk?'var(--green)':'var(--red)')+'">'+dep.ready_replicas+'/'+dep.replicas+'</td><td style="padding:5px 8px;border-bottom:1px solid var(--border);font-size:11px;color:var(--text-quaternary)">'+esc(dep.strategy||'')+'</td></tr>';
    }
    h += '</table></div></div>';
  }
  h += '</div>';

  // K8s Events
  h += '<div class="section-block" data-section="k8s_events">';
  if (k8sConnected && k8s.events && k8s.events.length > 0) {
    h += '<div>';
    h += '<div class="section-title">' + k8sTitle + ' Events (' + k8s.events.length + ' warnings)</div>';
    h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:8px;max-height:200px;overflow-y:auto;scrollbar-width:thin">';
    for (var ei = 0; ei < Math.min(k8s.events.length, 15); ei++) { var ev = k8s.events[ei];
      h += '<div style="font-size:11px;padding:4px 0;border-bottom:1px solid var(--border)"><span style="color:var(--amber);font-weight:600">' + esc(ev.reason) + '</span> <span style="color:var(--text-quaternary)">' + esc(ev.object) + '</span> <span style="color:var(--text-tertiary)">' + esc(ev.message).substring(0, 100) + '</span></div>';
    }
    h += '</div></div>';
  }
  h += '</div>';

  return h;
};

/* ── Section: Processes ───────────────────────────────────────── */
sections.processes = function(sn) {
  var esc = util.esc;
  var h = '';
  h += '<div class="section-block" data-section="processes">';
  var procs = (sn && sn.system && sn.system.top_processes) ? sn.system.top_processes : [];
  if (procs.length > 0) {
    var showCount = Math.min(procs.length, 10);
    h += '<div>';
    h += '<div class="section-title" style="display:flex;align-items:center;justify-content:space-between"><span>Top Processes (' + procs.length + ')</span><a href="/stats#process-history" style="font-size:11px;color:var(--text-quaternary);text-decoration:none;font-weight:400;margin-left:16px;white-space:nowrap">View history &rarr;</a></div>';
    h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);overflow:hidden">';
    h += '<table style="width:100%;font-size:12px;border-collapse:collapse">';
    h += '<tr style="color:var(--text-quaternary);font-size:10px;text-transform:uppercase;letter-spacing:0.5px">';
    h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border);width:28px">#</th>';
    h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Process</th>';
    h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">Container</th>';
    h += '<th style="text-align:right;padding:6px 8px;border-bottom:1px solid var(--border)">CPU%</th>';
    h += '<th style="text-align:right;padding:6px 8px;border-bottom:1px solid var(--border)">Mem%</th>';
    h += '<th style="text-align:left;padding:6px 8px;border-bottom:1px solid var(--border)">User</th>';
    h += '</tr>';
    for (var pi = 0; pi < showCount; pi++) {
      var p = procs[pi];
      var cpuClass = p.cpu_percent >= 50 ? 'td-crit' : p.cpu_percent >= 20 ? 'td-warn' : '';
      var memClass = p.mem_percent >= 50 ? 'td-crit' : p.mem_percent >= 20 ? 'td-warn' : '';
      var containerTag = '';
      if (p.container_name) {
        containerTag = '<span style="display:inline-block;font-size:10px;padding:1px 6px;border-radius:999px;background:rgba(94,106,210,0.15);color:var(--accent);white-space:nowrap">' + esc(p.container_name) + '</span>';
      } else {
        containerTag = '<span style="display:inline-block;font-size:10px;padding:1px 6px;border-radius:999px;background:rgba(128,128,128,0.12);color:var(--text-quaternary);white-space:nowrap">host</span>';
      }
      var cmdDisplay = esc(p.command || '');
      if (cmdDisplay.length > 40) cmdDisplay = cmdDisplay.substring(0, 40) + '\u2026';
      var procName = (p.command || '').split('/').pop().split(' ')[0];
      var statsUrl = '/stats?process=' + encodeURIComponent(procName) + '&container=' + encodeURIComponent(p.container_name || '') + '#process-history';
      h += '<tr style="cursor:pointer;transition:background 0.15s" onmouseover="this.style.background=\'rgba(94,106,210,0.08)\'" onmouseout="this.style.background=\'transparent\'" onclick="window.location.href=\'' + statsUrl + '\'">';
      h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border);color:var(--text-quaternary)">' + (pi + 1) + '</td>';
      h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)"><div style="font-weight:500;max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title="' + esc(p.command || '') + '">' + cmdDisplay + '</div></td>';
      h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border)">' + containerTag + '</td>';
      h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border);text-align:right;font-family:var(--font-mono);font-size:11px" class="' + cpuClass + '">' + (p.cpu_percent || 0).toFixed(1) + '</td>';
      h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border);text-align:right;font-family:var(--font-mono);font-size:11px" class="' + memClass + '">' + (p.mem_percent || 0).toFixed(1) + '</td>';
      h += '<td style="padding:5px 8px;border-bottom:1px solid var(--border);color:var(--text-tertiary);font-size:11px">' + esc(p.user || '') + '</td>';
      h += '</tr>';
    }
    h += '</table>';
    h += '</div>';
    if (procs.length > showCount) {
      h += '<div style="text-align:center;padding:6px 0;font-size:11px;color:var(--text-quaternary)">' + (procs.length - showCount) + ' more processes not shown</div>';
    }
    h += '</div>';
  }
  h += '</div>';
  return h;
};

/* ── Section: Parity ─────────────────────────────────────────── */
sections.parity = function(sn) {
  var esc = util.esc;
  var h = '';
  h += '<div class="section-block" data-section="parity">';
  var parity = sn ? sn.parity : null;
  if (parity && parity.history && parity.history.length > 0) {
    h += '<div>';
    h += '<div class="section-title" style="display:flex;align-items:center;justify-content:space-between"><span>Parity History</span><a href="/parity" style="font-size:11px;color:var(--text-quaternary);text-decoration:none;font-weight:400;margin-left:16px;white-space:nowrap">View all &rarr;</a></div>';
    h += '<div style="font-size:12px;color:var(--text-tertiary);margin-bottom:8px">Status: ' + esc(parity.status || "idle") + '</div>';
    h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:12px">';
    h += '<div style="display:flex;gap:8px;overflow-x:auto;padding-bottom:6px;scrollbar-width:thin">';
    var sorted = parity.history.slice().sort(function(a,b){ return (b.date||"").localeCompare(a.date||""); });
    for (var pi = 0; pi < sorted.length; pi++) {
      var pc = sorted[pi];
      var hasErr = pc.errors > 0;
      var dur = pc.duration_seconds > 0 ? (pc.duration_seconds >= 3600 ? Math.floor(pc.duration_seconds/3600)+'h '+Math.round((pc.duration_seconds%3600)/60)+'m' : Math.round(pc.duration_seconds/60)+'m') : '\u2014';
      h += '<a href="/parity" style="text-decoration:none;flex-shrink:0">';
      h += '<div style="background:var(--bg-panel);border:1px solid '+(hasErr?'var(--red)':'var(--border)')+';border-radius:10px;padding:8px 14px;min-width:120px;cursor:pointer;transition:border-color 0.15s">';
      h += '<div style="font-size:12px;font-weight:600;color:var(--text-primary)">' + esc(pc.date) + '</div>';
      h += '<div style="font-size:11px;color:var(--text-tertiary);margin-top:2px">' + dur + ' &middot; ' + (pc.speed_mb_s||0).toFixed(0) + ' MB/s</div>';
      h += '<div style="font-size:11px;margin-top:2px;font-weight:600;color:'+(hasErr?'var(--red)':'var(--green)')+'">'+( hasErr ? pc.errors+' errors' : 'Clean' )+'</div>';
      h += '</div></a>';
    }
    h += '</div>';
    h += '</div>';
    h += '</div>';
  } else if (parity && parity.status) {
    h += '<div>';
    h += '<div class="section-title">Parity</div>';
    h += '<div style="background:var(--bg-panel);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:12px;font-size:12px;color:var(--text-tertiary)">Status: ' + esc(parity.status) + ' &middot; No parity check history found</div>';
    h += '</div>';
  }
  h += '</div>';
  return h;
};

/* ── Chart Loaders ───────────────────────────────────────────── */
var charts = {};

charts.loadGPU = function(hours, save) {
  if (save) { polling.saveChartRange(hours); charts.loadContainers(hours); charts.loadSpeedTest(hours); }
  var btns = document.querySelectorAll(".gpu-range-btn");
  for (var b = 0; b < btns.length; b++) {
    var btn = btns[b];
    if (parseInt(btn.getAttribute("data-hours")) === hours) {
      btn.style.background = "var(--bg-elevated)"; btn.style.color = "var(--text-secondary)"; btn.classList.add("active");
    } else {
      btn.style.background = "transparent"; btn.style.color = "var(--text-tertiary)"; btn.classList.remove("active");
    }
  }
  fetch("/api/v1/history/gpu?hours=" + hours)
    .then(function(r) { return r.json(); })
    .then(function(points) {
      if (!points || !points.length || !window.NasChart) return;
      var byGPU = {};
      for (var i = 0; i < points.length; i++) { var p = points[i]; if (!byGPU[p.gpu_index]) byGPU[p.gpu_index] = []; byGPU[p.gpu_index].push(p); }
      for (var idx in byGPU) {
        var canvasId = "gpu-chart-" + idx;
        var el = document.getElementById(canvasId);
        if (!el) continue;
        var data = byGPU[idx];
        var usageData = data.map(function(p) { return p.usage_percent; });
        var labels = data.map(function(p) { var d = new Date(p.timestamp); if (hours <= 1) return d.getHours() + ":" + ("0" + d.getMinutes()).slice(-2); if (hours <= 24) return d.getHours() + ":00"; return (d.getMonth()+1) + "/" + d.getDate(); });
        try { NasChart.area(canvasId, { datasets: [{ data: usageData, color: "#8b5cf6", label: "GPU %" }], labels: labels, yMax: 100, width: el.offsetWidth || 400, height: 60, showDots: false, margins: { top: 4, bottom: 16, left: 30, right: 8 } }); } catch(e) {}
      }
    })
    .catch(function() {});
};

charts.loadContainers = function(hours, save) {
  if (save) { polling.saveChartRange(hours); charts.loadGPU(hours); charts.loadSpeedTest(hours); }
  var btns = document.querySelectorAll(".cmetrics-range-btn");
  for (var b = 0; b < btns.length; b++) {
    var btn = btns[b];
    if (parseInt(btn.getAttribute("data-hours")) === hours) {
      btn.style.background = "var(--bg-elevated)"; btn.style.color = "var(--text-secondary)"; btn.classList.add("active");
    } else {
      btn.style.background = "transparent"; btn.style.color = "var(--text-tertiary)"; btn.classList.remove("active");
    }
  }
  fetch("/api/v1/history/containers?hours=" + hours)
    .then(function(r) { return r.json(); })
    .then(function(points) {
      if (!points || !points.length || !window.NasChart) return;
      var byName = {};
      for (var i = 0; i < points.length; i++) { var p = points[i]; if (!byName[p.name]) byName[p.name] = []; byName[p.name].push(p); }
      var canvases = document.querySelectorAll("[id^='cmetrics-chart-']");
      for (var ci = 0; ci < canvases.length; ci++) {
        var el = canvases[ci];
        var cname = el.getAttribute("data-container");
        if (!cname || !byName[cname]) continue;
        var data = byName[cname];
        var cpuData = data.map(function(p) { return p.cpu_percent; });
        var memData = data.map(function(p) { return p.mem_mb; });
        var labels = data.map(function(p) { var d = new Date(p.timestamp); if (hours <= 1) return d.getHours() + ":" + ("0" + d.getMinutes()).slice(-2); if (hours <= 24) return d.getHours() + ":00"; return (d.getMonth()+1) + "/" + d.getDate(); });
        try { NasChart.area(el.id, { datasets: [{ data: cpuData, color: "#3b82f6", label: "CPU %" }, { data: memData, color: "#8b5cf6", label: "Mem MB" }], labels: labels, width: el.offsetWidth || 400, height: 60, showDots: false, margins: { top: 4, bottom: 16, left: 30, right: 8 } }); } catch(e) {}
      }
    })
    .catch(function() {});
};

charts.loadSpeedTest = function(hours, save) {
  if (save) { polling.saveChartRange(hours); charts.loadGPU(hours); charts.loadContainers(hours); }
  var btns = document.querySelectorAll(".st-range-btn");
  for (var b = 0; b < btns.length; b++) {
    var btn = btns[b];
    if (parseInt(btn.getAttribute("data-hours")) === hours) {
      btn.style.background = "var(--bg-elevated)"; btn.style.color = "var(--text-secondary)";
    } else {
      btn.style.background = "transparent"; btn.style.color = "var(--text-tertiary)";
    }
  }
  fetch("/api/v1/history/speedtest?hours=" + hours)
    .then(function(r) { return r.json(); })
    .then(function(points) {
      if (!points || !points.length || !window.NasChart) return;
      var dlData = points.map(function(p) { return p.download_mbps; });
      var ulData = points.map(function(p) { return p.upload_mbps; });
      var labels = points.map(function(p) { var d = new Date(p.timestamp); if (hours <= 1) return d.getHours() + ":" + ("0" + d.getMinutes()).slice(-2); if (hours <= 24) return d.getHours() + ":00"; return (d.getMonth()+1) + "/" + d.getDate(); });
      try { NasChart.area("speedtest-chart", { datasets: [{ data: dlData, color: "#3b82f6", label: "Download" }, { data: ulData, color: "#8b5cf6", label: "Upload" }], labels: labels, width: document.getElementById("speedtest-chart").offsetWidth || 400, height: 80, showDots: true, margins: { top: 4, bottom: 16, left: 40, right: 8 } }); } catch(e) {}
    }).catch(function() {});
};

charts.loadSparklines = function(snapshot) {
  fetch("/api/v1/sparklines")
    .then(function(r) { return r.json(); })
    .then(function(data) {
      if (data.system && data.system.length >= 2 && window.NasChart) {
        var cpuData = data.system.map(function(p) { return p.cpu_usage; });
        var memData = data.system.map(function(p) { return p.mem_percent; });
        var ioData = data.system.map(function(p) { return p.io_wait; });
        try { NasChart.sparkline("spark-cpu", { data: cpuData, color: "#5e6ad2", width: 48, height: 20 }); } catch(e) {}
        try { NasChart.sparkline("spark-mem", { data: memData, color: "#7170ff", width: 48, height: 20 }); } catch(e) {}
        try { NasChart.sparkline("spark-io", { data: ioData, color: "#f59e0b", width: 48, height: 20 }); } catch(e) {}
      }
      if (data.disks && window.NasChart) {
        var smart = snapshot ? (snapshot.smart || []) : [];
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
  _chartRange = (_statusData && _statusData.chart_range_hours) || 1;
  charts.loadGPU(_chartRange);
  charts.loadContainers(_chartRange);
  charts.loadSpeedTest(_chartRange);
};

/* ── NasScrollFade ───────────────────────────────────────────── */
var NasScrollFade = {
  _done: [],
  _heights: {},
  _saveTimer: null,
  init: function() {
    for (var i = 0; i < this._done.length; i++) { try { this._done[i](); } catch(e) {} }
    this._done = [];
    this._heights = (_statusData && _statusData.section_heights) ? JSON.parse(JSON.stringify(_statusData.section_heights)) : {};
    var candidates = document.querySelectorAll('.table-wrap, .table-container, [style*="overflow-x:auto"], [style*="overflow-x: auto"]');
    for (var j = 0; j < candidates.length; j++) this._setupH(candidates[j]);
    var blocks = document.querySelectorAll('.section-block');
    for (var k = 0; k < blocks.length; k++) {
      if (blocks[k].offsetHeight > 60) this._setupV(blocks[k]);
    }
  },
  _bg: function(el) {
    for (var n = el; n && n !== document.body; n = n.parentElement) {
      var bg = getComputedStyle(n).backgroundColor;
      if (bg && bg !== 'rgba(0, 0, 0, 0)' && bg !== 'transparent') return bg;
    }
    return getComputedStyle(document.body).backgroundColor || '#0a0a1a';
  },
  _saveHeights: function() {
    var self = this;
    if (self._saveTimer) clearTimeout(self._saveTimer);
    self._saveTimer = setTimeout(function() {
      fetch("/api/v1/settings/section-heights", { method: "PUT", headers: {"Content-Type":"application/json"}, body: JSON.stringify(self._heights) }).catch(function() {});
    }, 500);
  },
  _setupH: function(scrollEl) {
    if (scrollEl._sfH) return; scrollEl._sfH = true;
    var hasBg = getComputedStyle(scrollEl).backgroundColor;
    var container = (hasBg && hasBg !== 'rgba(0, 0, 0, 0)' && hasBg !== 'transparent') ? scrollEl : scrollEl.parentElement;
    var bg = this._bg(container);
    container.style.position = 'relative';
    var fL = document.createElement('div'); fL.className = 'sf sf-h sf-l';
    var fR = document.createElement('div'); fR.className = 'sf sf-h sf-r';
    fL.style.background = 'linear-gradient(to right, ' + bg + ', transparent)';
    fR.style.background = 'linear-gradient(to left, ' + bg + ', transparent)';
    container.appendChild(fL); container.appendChild(fR);
    var update = function() {
      fL.classList.toggle('show', scrollEl.scrollLeft > 2);
      fR.classList.toggle('show', scrollEl.scrollWidth - scrollEl.clientWidth - scrollEl.scrollLeft > 2);
    };
    scrollEl.addEventListener('scroll', update, { passive: true });
    update();
    if (window.ResizeObserver) new ResizeObserver(update).observe(scrollEl);
    this._done.push(function() { fL.remove(); fR.remove(); });
  },
  _setupV: function(block) {
    if (block._sfV) return; block._sfV = true;
    var self = this;
    var key = block.getAttribute('data-section') || ('blk-' + Math.random().toString(36).slice(2, 8));
    var bg = this._bg(block);
    var fB = document.createElement('div'); fB.className = 'sf sf-v sf-b';
    fB.style.background = 'linear-gradient(to top, ' + bg + ', transparent)';
    block.appendChild(fB);
    var handle = document.createElement('div'); handle.className = 'sb-handle';
    block.appendChild(handle);
    if (self._heights[key]) {
      block.classList.add('sb-resized');
      block.style.height = self._heights[key] + 'px';
      block.style.overflow = 'hidden';
    }
    var startY, startH;
    function onMove(e) {
      var dy = (e.touches ? e.touches[0].clientY : e.clientY) - startY;
      block.style.height = Math.max(60, startH + dy) + 'px';
    }
    function onUp() {
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
      document.removeEventListener('touchmove', onMove);
      document.removeEventListener('touchend', onUp);
      document.body.style.userSelect = '';
      document.body.style.cursor = '';
      var h = parseInt(block.style.height);
      if (h > 0) { self._heights[key] = h; } else { delete self._heights[key]; }
      self._saveHeights();
      updateFade();
    }
    function onDown(e) {
      e.preventDefault();
      startY = e.touches ? e.touches[0].clientY : e.clientY;
      startH = block.offsetHeight;
      if (!block.classList.contains('sb-resized')) {
        block.classList.add('sb-resized');
        block.style.height = startH + 'px';
        block.style.overflow = 'hidden';
      }
      document.body.style.userSelect = 'none';
      document.body.style.cursor = 'ns-resize';
      document.addEventListener('mousemove', onMove);
      document.addEventListener('mouseup', onUp);
      document.addEventListener('touchmove', onMove, { passive: false });
      document.addEventListener('touchend', onUp);
    }
    handle.addEventListener('mousedown', onDown);
    handle.addEventListener('touchstart', onDown, { passive: false });
    handle.addEventListener('dblclick', function() {
      block.classList.remove('sb-resized');
      block.style.height = ''; block.style.overflow = '';
      delete self._heights[key];
      self._saveHeights();
      updateFade();
    });
    function updateFade() {
      var overflows = block.classList.contains('sb-resized') && block.scrollHeight > block.clientHeight + 4;
      fB.classList.toggle('show', overflows);
      if (overflows) block.style.overflow = 'auto';
    }
    block.addEventListener('scroll', function() { updateFade(); }, { passive: true });
    if (window.ResizeObserver) new ResizeObserver(updateFade).observe(block);
    updateFade();
    this._done.push(function() { fB.remove(); handle.remove(); block.classList.remove('sb-resized'); block.style.height = ''; block.style.overflow = ''; });
  }
};

/* ── Section Distribution ────────────────────────────────────── */
function distributeSections() {
  var staging = document.getElementById("section-staging");
  var colL = document.getElementById("col-left");
  var colR = document.getElementById("col-right");
  if (!staging || !colL || !colR) return;

  var sec = (_statusData && _statusData.sections) ? _statusData.sections : {};
  var numCols = sec.dash_columns || 2;
  if (numCols < 1) numCols = 2;
  var container = document.querySelector(".container");
  var twoCol = document.getElementById("two-col");
  if (numCols >= 3 && container) container.classList.add("dash-wide");
  else if (container) container.classList.remove("dash-wide");
  if (twoCol) twoCol.style.gridTemplateColumns = "repeat(" + numCols + ", 1fr)";

  var allCols = [colL, colR];
  for (var ci = 3; ci <= numCols; ci++) {
    var extraCol = document.getElementById("col-" + ci);
    if (extraCol) allCols.push(extraCol);
  }
  var sectionMap = {
    "findings": sec.findings !== false,
    "drives": sec.disk_space !== false || sec.smart !== false,
    "docker": sec.docker !== false,
    "container_metrics": sec.container_metrics !== false,
    "zfs": sec.zfs !== false,
    "gpu": sec.gpu !== false,
    "ups": sec.ups !== false,
    "backup": sec.backup !== false,
    "services": true,
    "processes": sec.processes !== false,
    "network": sec.network !== false,
    "speedtest": sec.speedtest !== false,
    "tunnels": sec.tunnels !== false,
    "proxmox": sec.proxmox !== false,
    "kubernetes": sec.kubernetes !== false,
    "k8s_nodes": sec.kubernetes !== false,
    "k8s_problems": sec.kubernetes !== false,
    "k8s_deployments": sec.kubernetes !== false,
    "k8s_events": sec.kubernetes !== false,
    "parity": sec.parity !== false
  };

  var blocks = staging.querySelectorAll(".section-block");
  if (blocks.length === 0) return;

  var blockMap = {};
  var visibleItems = [];
  for (var i = 0; i < blocks.length; i++) {
    var name = blocks[i].getAttribute("data-section");
    var resolved = sectionMap[name];
    if (resolved === undefined && name && name.indexOf("k8s_node_") === 0) resolved = sec.kubernetes !== false;
    if (resolved === false) continue;
    if (blocks[i].offsetHeight < 10) continue;
    blockMap[name] = blocks[i];
    visibleItems.push({ el: blocks[i], name: name, h: blocks[i].offsetHeight });
  }

  window._serverSectionOrder = (_statusData && _statusData.section_order) || null;

  if (!window.NasDrag || !NasDrag.applySavedOrder(blockMap, visibleItems, allCols)) {
    var colHeights = allCols.map(function() { return 0; });
    for (var j = 0; j < visibleItems.length; j++) {
      var minIdx = 0;
      for (var k = 1; k < colHeights.length; k++) {
        if (colHeights[k] < colHeights[minIdx]) minIdx = k;
      }
      allCols[minIdx].appendChild(visibleItems[j].el);
      colHeights[minIdx] += visibleItems[j].h;
    }
  }

  staging.parentNode.removeChild(staging);

  if (window.NasDrag) NasDrag.init();
  if (window.NasSwipe) NasSwipe.init();
  initSortBars();
  NasScrollFade.init();
  document.addEventListener("click", function(e) {
    var row = e.target.closest("[data-sc-filter]");
    if (row) window.location.href = "/service-checks?filter=" + encodeURIComponent(row.getAttribute("data-sc-filter"));
  });
}

/* ── Sort Bars ───────────────────────────────────────────────── */
function initSortBars() {
  if (!window.NasSort) return;
  var prefs = NasSort.getPrefs();

  var findingsMount = document.getElementById("findings-sort-mount");
  if (findingsMount) {
    NasSort.renderSortBar({
      container: findingsMount,
      options: [
        { key: "severity", label: "Severity" },
        { key: "date", label: "Newest" },
        { key: "category", label: "Category" }
      ],
      active: prefs.findings || "severity",
      onSort: function(key) { prefs.findings = key; NasSort.savePrefs(prefs); polling.loadAll(); }
    });
  }

  var drivesMount = document.getElementById("drives-sort-mount");
  if (drivesMount) {
    NasSort.renderSortBar({
      container: drivesMount,
      options: [
        { key: "device", label: "Device" },
        { key: "temp", label: "Temp" },
        { key: "age", label: "Age" },
        { key: "size", label: "Size" },
        { key: "health", label: "Health" }
      ],
      active: prefs.drives || "device",
      onSort: function(key) { prefs.drives = key; NasSort.savePrefs(prefs); polling.loadAll(); }
    });
  }
}

/* ── Global event bindings ───────────────────────────────────── */
function setupGlobals() {
  // Severity filters
  try { window._findingSevFilters = JSON.parse(localStorage.getItem("nas-doctor-sev-filters")) || {}; } catch(e) { window._findingSevFilters = {}; }
  window._toggleSevFilter = function(sev) {
    window._findingSevFilters[sev] = !window._findingSevFilters[sev];
    if (!window._findingSevFilters[sev]) delete window._findingSevFilters[sev];
    try { localStorage.setItem("nas-doctor-sev-filters", JSON.stringify(window._findingSevFilters)); } catch(e) {}
    if (_renderFn) _renderFn();
  };
  window._clearSevFilters = function() {
    window._findingSevFilters = {};
    try { localStorage.removeItem("nas-doctor-sev-filters"); } catch(e) {}
    if (_renderFn) _renderFn();
  };

  window._dismissFinding = function(title, skipReload) {
    fetch("/api/v1/findings/dismiss", { method: "POST", headers: {"Content-Type":"application/json"}, body: JSON.stringify({title: title}) })
      .then(function() { if (!skipReload) polling.loadAll(); })
      .catch(function() {});
  };

  window._toggleFinding = function(el) {
    var all = document.querySelectorAll(".finding");
    for (var i = 0; i < all.length; i++) {
      if (all[i] !== el) all[i].classList.remove("active");
    }
    el.classList.toggle("active");
  };

  window._triggerScan = function() { polling.triggerScan(); };

  // Global chart loaders (called from onclick attributes in range buttons)
  window.loadGPUChart = function(hours, save) { charts.loadGPU(hours, save); };
  window.loadContainerChart = function(hours, save) { charts.loadContainers(hours, save); };
  window.loadSpeedTestChart = function(hours, save) { charts.loadSpeedTest(hours, save); };

  // Refresh indicator timer
  setInterval(function() {
    var el = document.getElementById("refresh-ago");
    if (!el || !_lastFetchTime) return;
    var secs = Math.round((Date.now() - _lastFetchTime) / 1000);
    if (secs < 5) el.textContent = "just now";
    else if (secs < 60) el.textContent = secs + "s ago";
    else el.textContent = Math.floor(secs / 60) + "m ago";
  }, 1000);
}

/* ── Public API ──────────────────────────────────────────────── */
window.NasDashboard = {
  util: util,
  polling: polling,
  sections: sections,
  charts: charts,
  NasScrollFade: NasScrollFade,
  distributeSections: distributeSections,
  initSortBars: initSortBars,

  init: function(cfg) {
    cfg = cfg || {};
    setupGlobals();

    // Store the render function
    _renderFn = cfg.render || null;
    _onRenderComplete = cfg.onRenderComplete || null;

    // Load and start polling
    polling.loadAll().then(function() {
      polling._startPoll();
    });
  }
};

})();
`
