package api

// DashboardEmber is the serif typography theme with depth shadows and micro-animations.
var DashboardEmber = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>NAS Doctor — Ember</title>
<link rel="icon" type="image/png" href="/icon.png">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&family=JetBrains+Mono:wght@400;500&family=Literata:ital,wght@0,400;0,500;0,600;1,400;1,500&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { margin: 0; padding: 0; box-sizing: border-box; }

:root {
  --bg: #07080a;
  --surface-100: #101111;
  --surface-200: #1b1c1e;
  --text-primary: #f9f9f9;
  --text-secondary: #cecece;
  --text-tertiary: #9c9c9d;
  --text-dim: #6a6b6c;
  --border: rgba(255, 255, 255, 0.06);
  --border-hover: rgba(255, 255, 255, 0.12);
  --red: #FF6363;
  --blue: #55b3ff;
  --green: #5fc992;
  --yellow: #ffbc33;
  --ease-out: cubic-bezier(0.22, 1, 0.36, 1);
  --font-serif: "Literata", "Georgia", serif;
  --font-sans: "Inter", -apple-system, BlinkMacSystemFont, sans-serif;
  --font-mono: "JetBrains Mono", ui-monospace, "SF Mono", monospace;
  --shadow-card: rgb(27, 28, 30) 0px 0px 0px 1px, rgb(7, 8, 10) 0px 0px 0px 1px inset;
  --shadow-btn: rgba(255, 255, 255, 0.05) 0px 1px 0px 0px inset, rgba(255, 255, 255, 0.1) 0px 0px 0px 1px;
}

html {
  font-family: var(--font-sans);
  font-feature-settings: "calt", "kern", "liga", "ss03";
  font-size: 16px;
  line-height: 1.5;
  color: var(--text-primary);
  background: var(--bg);
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

body {
  min-height: 100vh;
  background: var(--bg);
}

/* ---- Animations ---- */

@keyframes slideUp {
  from { opacity: 0; transform: translateY(12px); }
  to { opacity: 1; transform: translateY(0); }
}

@keyframes valuePulse {
  0% { opacity: 1; }
  50% { opacity: 0.5; }
  100% { opacity: 1; }
}

@keyframes breathe {
  0%, 100% { box-shadow: var(--shadow-btn), 0 0 0 0 rgba(85, 179, 255, 0); }
  50% { box-shadow: var(--shadow-btn), 0 0 12px 2px rgba(85, 179, 255, 0.15); }
}

@keyframes pulse {
  0%, 100% { transform: scale(1); opacity: 1; }
  50% { transform: scale(1.8); opacity: 0; }
}

@keyframes pulseRing {
  0%, 100% { transform: scale(1); opacity: 0.6; }
  50% { transform: scale(1.8); opacity: 0; }
}

@keyframes expandIn {
  from { max-height: 0; opacity: 0; }
  to { max-height: 300px; opacity: 1; }
}

@keyframes collapseOut {
  from { max-height: 300px; opacity: 1; }
  to { max-height: 0; opacity: 0; }
}

@keyframes barFill {
  from { width: 0%; }
}

@keyframes scanRotate {
  0% { filter: hue-rotate(0deg); }
  100% { filter: hue-rotate(360deg); }
}

@keyframes fadeInScan {
  from { opacity: 0; transform: scale(0.96); }
  to { opacity: 1; transform: scale(1); }
}

.section {
  opacity: 0;
  animation: slideUp 500ms var(--ease-out) forwards;
}

.section:nth-child(1)  { animation-delay: 0ms; }
.section:nth-child(2)  { animation-delay: 80ms; }
.section:nth-child(3)  { animation-delay: 160ms; }
.section:nth-child(4)  { animation-delay: 240ms; }
.section:nth-child(5)  { animation-delay: 320ms; }
.section:nth-child(6)  { animation-delay: 400ms; }
.section:nth-child(7)  { animation-delay: 480ms; }
.section:nth-child(8)  { animation-delay: 560ms; }
.section:nth-child(9)  { animation-delay: 640ms; }
.section:nth-child(10) { animation-delay: 720ms; }

.value-pulse {
  animation: valuePulse 400ms ease;
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
  padding: 14px 24px;
  background: rgba(7, 8, 10, 0.85);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border-bottom: 1px solid var(--border);
  position: sticky;
  top: 0;
  z-index: 100;
}

.header-brand {
  font-family: var(--font-serif);
  font-size: 20px;
  font-weight: 500;
  letter-spacing: -0.3px;
  color: var(--text-primary);
  text-decoration: none;
  display: flex;
  align-items: center;
  gap: 10px;
}

.header-brand-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 7px;
  background: linear-gradient(135deg, #55b3ff 0%, #5fc992 100%);
  font-size: 14px;
  line-height: 1;
}

.header-hostname {
  font-family: var(--font-mono);
  font-size: 13px;
  font-weight: 400;
  color: var(--text-tertiary);
  letter-spacing: 0;
}

.header-nav {
  display: flex;
  gap: 4px;
  position: relative;
}

.header-nav a {
  font-family: var(--font-sans);
  font-size: 13px;
  font-weight: 500;
  color: var(--text-dim);
  text-decoration: none;
  padding: 6px 12px;
  border-radius: 6px;
  transition: color 200ms var(--ease-out), background 200ms var(--ease-out);
  position: relative;
}

.header-nav a:hover {
  color: var(--text-secondary);
}

.header-nav a.active {
  color: var(--text-primary);
}

.header-nav a.active::after {
  content: "";
  position: absolute;
  bottom: 0;
  left: 12px;
  right: 12px;
  height: 2px;
  background: var(--blue);
  border-radius: 1px;
  transition: all 300ms var(--ease-out);
}

/* ---- Section spacing ---- */
.section {
  margin-top: 40px;
}

.section-title {
  font-family: var(--font-serif);
  font-size: 24px;
  font-weight: 500;
  letter-spacing: -0.3px;
  color: var(--text-primary);
  margin-bottom: 16px;
}

/* ---- Cards ---- */
.card {
  background: var(--surface-100);
  border-radius: 12px;
  box-shadow: var(--shadow-card);
  padding: 24px;
  transition: transform 200ms var(--ease-out), box-shadow 200ms ease, border-color 200ms ease;
}

.card:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-card), 0 8px 24px rgba(0, 0, 0, 0.3);
}

.card-static {
  background: var(--surface-100);
  border-radius: 12px;
  box-shadow: var(--shadow-card);
  padding: 24px;
}

/* ---- Badges / Pills ---- */
.badge {
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 6px;
  font-family: var(--font-sans);
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  background: var(--surface-200);
  color: var(--text-secondary);
}

.badge-critical { background: rgba(255, 99, 99, 0.12); color: var(--red); }
.badge-warning  { background: rgba(255, 188, 51, 0.12); color: var(--yellow); }
.badge-info     { background: rgba(85, 179, 255, 0.12); color: var(--blue); }
.badge-ok       { background: rgba(95, 201, 146, 0.12); color: var(--green); }

.pill {
  display: inline-flex;
  align-items: center;
  padding: 5px 14px;
  border-radius: 9999px;
  font-family: var(--font-sans);
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.3px;
  background: var(--surface-200);
  color: var(--text-secondary);
}

.pill-critical { background: rgba(255, 99, 99, 0.12); color: var(--red); }
.pill-warning  { background: rgba(255, 188, 51, 0.12); color: var(--yellow); }
.pill-info     { background: rgba(85, 179, 255, 0.12); color: var(--blue); }
.pill-healthy  { background: rgba(95, 201, 146, 0.12); color: var(--green); }
.pill-neutral  { background: var(--surface-200); color: var(--text-dim); }

/* ---- Health overview ---- */
.health-overview {
  display: flex;
  align-items: center;
  gap: 20px;
  flex-wrap: wrap;
}

.health-status-area {
  display: flex;
  align-items: center;
  gap: 14px;
  flex: 1;
  min-width: 280px;
}

.health-dot-wrap {
  position: relative;
  width: 16px;
  height: 16px;
  flex-shrink: 0;
}

.health-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  position: absolute;
  top: 3px;
  left: 3px;
}

.health-dot-ring {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  position: absolute;
  top: 0;
  left: 0;
}

.health-dot.dot-green       { background: var(--green); }
.health-dot-ring.dot-green  { background: var(--green); animation: pulseRing 2s ease infinite; }
.health-dot.dot-yellow      { background: var(--yellow); }
.health-dot-ring.dot-yellow { background: var(--yellow); animation: pulseRing 1.5s ease infinite; }
.health-dot.dot-red         { background: var(--red); }
.health-dot-ring.dot-red    { background: var(--red); animation: pulseRing 0.8s ease infinite; }

.health-text {
  font-family: var(--font-serif);
  font-size: 22px;
  font-weight: 500;
  letter-spacing: -0.3px;
  color: var(--text-primary);
}

.health-text.text-critical {
  font-weight: 700;
  color: var(--red);
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
  gap: 12px;
}

@media (max-width: 768px) {
  .stats-grid { grid-template-columns: repeat(2, 1fr); }
}

.stat-card {
  background: var(--surface-100);
  border-radius: 12px;
  box-shadow: var(--shadow-card);
  padding: 20px 24px;
  transition: transform 200ms var(--ease-out), box-shadow 200ms ease;
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-card), 0 8px 24px rgba(0, 0, 0, 0.3);
}

.stat-value {
  font-family: var(--font-sans);
  font-size: 28px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.5px;
  color: var(--text-primary);
  line-height: 1.2;
}

.stat-label {
  font-family: var(--font-sans);
  font-size: 12px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.8px;
  color: var(--text-dim);
  margin-top: 6px;
}

/* ---- Findings ---- */
@keyframes pulseSlow {
  0%, 100% { box-shadow: 0 0 0 0 rgba(255, 255, 255, 0); }
  50% { box-shadow: 0 0 10px 2px rgba(255, 255, 255, 0.15); }
}

.findings-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.finding-card {
  position: relative;
  background: var(--surface-100);
  border-radius: 12px;
  padding: 20px 20px 16px 56px;
  margin-bottom: 0;
  box-shadow: rgb(27, 28, 30) 0px 0px 0px 1px, rgb(7, 8, 10) 0px 0px 0px 1px inset;
  cursor: pointer;
  transition: all 200ms cubic-bezier(0.22, 1, 0.36, 1);
  overflow: hidden;
}

.finding-card::before {
  content: "";
  position: absolute;
  top: 0;
  left: 0;
  width: 120px;
  height: 120px;
  border-radius: 50%;
  filter: blur(40px);
  opacity: 0.08;
  pointer-events: none;
  transition: opacity 200ms cubic-bezier(0.22, 1, 0.36, 1);
}

.finding-card:hover::before { opacity: 0.15; }
.finding-card:hover {
  transform: translateY(-2px);
  box-shadow: rgb(27, 28, 30) 0px 0px 0px 1px, rgb(7, 8, 10) 0px 0px 0px 1px inset, rgba(0, 0, 0, 0.3) 0px 8px 24px -4px;
}

.finding-card.active::before { opacity: 0.15; }
.finding-card.active {
  box-shadow: rgb(27, 28, 30) 0px 0px 0px 1px, rgb(7, 8, 10) 0px 0px 0px 1px inset, rgba(0, 0, 0, 0.3) 0px 8px 24px -4px;
}

.finding-card.sev-critical::before { background: #FF6363; }
.finding-card.sev-warning::before  { background: #ffbc33; }
.finding-card.sev-info::before     { background: #55b3ff; }
.finding-card.sev-ok::before       { background: #5fc992; }

.sev-icon {
  position: absolute;
  top: 16px;
  left: 16px;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  font-weight: 700;
  color: white;
  z-index: 1;
}

.sev-icon-critical { background: #FF6363; animation: pulseSlow 2s ease-in-out infinite; }
.sev-icon-warning  { background: #ffbc33; color: #101111; }
.sev-icon-info     { background: #55b3ff; }
.sev-icon-ok       { background: #5fc992; color: #101111; }

.finding-top {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}

.finding-title {
  font-family: var(--font-serif);
  font-size: 18px;
  font-weight: 500;
  color: var(--text-primary);
  letter-spacing: -0.2px;
}

.finding-category {
  font-family: var(--font-sans);
  font-size: 11px;
  font-weight: 500;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 1px;
}

.finding-desc {
  font-family: var(--font-serif);
  font-size: 15px;
  font-weight: 400;
  font-style: italic;
  color: var(--text-secondary);
  line-height: 1.6;
  margin-top: 4px;
}

.finding-expandable {
  overflow: hidden;
  max-height: 0;
  opacity: 0;
  transition: max-height 300ms var(--ease-out), opacity 300ms var(--ease-out);
}

.finding-card.active .finding-expandable {
  max-height: 300px;
  opacity: 1;
}

.finding-details {
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.finding-detail-row {
  display: flex;
  gap: 8px;
}

.finding-detail-label {
  font-family: var(--font-sans);
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--text-dim);
  min-width: 80px;
  padding-top: 2px;
}

.finding-detail-value {
  font-family: var(--font-serif);
  font-size: 14px;
  font-weight: 400;
  color: var(--text-secondary);
  line-height: 1.5;
}

.finding-detail-value.italic {
  font-style: italic;
}

.finding-evidence-list {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.finding-evidence-item {
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--text-tertiary);
  padding: 4px 8px;
  background: rgba(255, 255, 255, 0.02);
  border-radius: 4px;
  line-height: 1.4;
}

.finding-meta {
  display: flex;
  gap: 8px;
  margin-top: 12px;
  flex-wrap: wrap;
}

.finding-meta-tag {
  font-family: var(--font-sans);
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  padding: 3px 8px;
  border-radius: 6px;
  background: var(--surface-200);
  color: var(--text-dim);
}

/* ---- Disk bars ---- */
.disk-list {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.disk-item {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.disk-info {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
}

.disk-label {
  font-family: var(--font-sans);
  font-size: 14px;
  font-weight: 500;
  color: var(--text-primary);
}

.disk-device {
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--text-dim);
  margin-left: 8px;
}

.disk-detail {
  font-family: var(--font-mono);
  font-size: 13px;
  font-weight: 400;
  color: var(--text-tertiary);
}

.disk-bar-track {
  width: 100%;
  height: 6px;
  background: var(--surface-200);
  border-radius: 9999px;
  overflow: hidden;
}

.disk-bar-fill {
  height: 100%;
  border-radius: 9999px;
  animation: barFill 800ms var(--ease-out) forwards;
}

.disk-bar-fill.c-green  { background: var(--green); }
.disk-bar-fill.c-yellow { background: var(--yellow); }
.disk-bar-fill.c-red    { background: var(--red); }

/* ---- Tables ---- */
.table-wrap {
  background: var(--surface-100);
  border-radius: 12px;
  box-shadow: var(--shadow-card);
  overflow: hidden;
}

.table-wrap table {
  width: 100%;
  border-collapse: collapse;
}

.table-wrap thead th {
  font-family: var(--font-sans);
  font-size: 11px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 1px;
  color: var(--text-dim);
  padding: 12px 16px;
  text-align: left;
  background: rgba(255, 255, 255, 0.02);
  border-bottom: 1px solid var(--border);
}

.table-wrap tbody tr {
  transition: transform 200ms var(--ease-out), background 200ms ease;
  border-left: 2px solid transparent;
}

.table-wrap tbody tr:hover {
  background: rgba(255, 255, 255, 0.02);
  transform: translateX(2px);
  border-left-color: var(--blue);
}

.table-wrap tbody td {
  font-family: var(--font-sans);
  font-size: 14px;
  font-weight: 400;
  color: var(--text-primary);
  padding: 11px 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.03);
}

.table-wrap tbody tr:last-child td {
  border-bottom: none;
}

td.mono {
  font-family: var(--font-mono);
  font-size: 13px;
}

/* ---- Status dot ---- */
.status-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 6px;
  vertical-align: middle;
}

.status-dot.s-green  { background: var(--green); }
.status-dot.s-yellow { background: var(--yellow); }
.status-dot.s-red    { background: var(--red); }
.status-dot.s-gray   { background: var(--text-dim); }

/* ---- Buttons ---- */
.btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-family: var(--font-sans);
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.3px;
  border: none;
  cursor: pointer;
  transition: opacity 200ms ease, transform 200ms var(--ease-out);
}

.btn:hover {
  opacity: 0.9;
  transform: translateY(-1px);
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  transform: none;
}

.btn-primary {
  background: var(--surface-200);
  color: var(--text-primary);
  padding: 10px 24px;
  border-radius: 9999px;
  box-shadow: var(--shadow-btn);
  animation: breathe 2s ease infinite;
}

.btn-primary:disabled {
  animation: none;
}

.btn-primary.scanning {
  animation: scanRotate 2s linear infinite;
  background: linear-gradient(135deg, var(--blue), var(--green));
  color: var(--bg);
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
  font-family: var(--font-sans);
  font-size: 13px;
  color: var(--text-dim);
  font-weight: 400;
}

.scan-info strong {
  color: var(--yellow);
  font-weight: 500;
}

/* ---- Empty state ---- */
.empty-state {
  text-align: center;
  padding: 48px 24px;
  font-family: var(--font-serif);
  font-style: italic;
  color: var(--text-dim);
  font-size: 15px;
  font-weight: 400;
}

/* ---- Loading ---- */
.loading {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 80px 24px;
  font-family: var(--font-serif);
  font-style: italic;
  color: var(--text-dim);
  font-size: 16px;
}

/* ---- Footer ---- */
.footer {
  text-align: center;
  padding: 32px 24px;
  font-family: var(--font-sans);
  font-size: 12px;
  color: var(--text-dim);
  letter-spacing: 0.3px;
}

.footer span {
  color: var(--text-tertiary);
}

/* ---- Scrollbar ---- */
::-webkit-scrollbar {
  width: 6px;
}

::-webkit-scrollbar-track {
  background: transparent;
}

::-webkit-scrollbar-thumb {
  background: var(--surface-200);
  border-radius: 3px;
}

::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.1);
}

/* ---- Selection ---- */
::selection {
  background: rgba(85, 179, 255, 0.25);
  color: var(--text-primary);
}
</style>
</head>
<body>

<header class="header">
  <a href="/" class="header-brand">
    <img src="/icon.png" alt="" style="width:26px;height:26px;border-radius:5px;vertical-align:middle;margin-right:8px;">
    NAS Doctor
  </a>
  <span class="header-hostname" id="hostname">-</span>
  <nav class="header-nav">
    <a href="/">Midnight</a>
    <a href="/theme/clean">Clean</a>
    <a href="/theme/ember" class="active">Ember</a>
    <a href="/api/v1/report" target="_blank">Export Report</a>
    <a href="/settings">Settings</a>
  </nav>
</header>

<div class="wrapper">
  <div id="app">
    <div class="loading" id="loadingState">Awaiting first diagnostic scan...</div>
  </div>
</div>

<script>
(function() {
  "use strict";

  var REFRESH_INTERVAL = 30000;
  var refreshTimer = null;
  var activeFindingId = null;
  var prevStatValues = {};

  function esc(s) {
    if (!s) return "";
    var d = document.createElement("div");
    d.appendChild(document.createTextNode(s));
    return d.innerHTML;
  }

  function fmtPct(n) {
    if (n == null) return "-";
    return n.toFixed(1) + "%";
  }

  function fmtGB(gb) {
    if (gb == null) return "-";
    if (gb >= 1000) return (gb / 1000).toFixed(2) + " TB";
    return gb.toFixed(1) + " GB";
  }

  function diskBarColor(pct) {
    if (pct > 95) return "red";
    if (pct >= 80) return "yellow";
    return "green";
  }

  function tempColor(c) {
    if (c == null) return "";
    if (c >= 50) return "color:" + getComputedStyle(document.documentElement).getPropertyValue("--red").trim();
    if (c >= 40) return "color:" + getComputedStyle(document.documentElement).getPropertyValue("--yellow").trim();
    return "color:" + getComputedStyle(document.documentElement).getPropertyValue("--green").trim();
  }

  function powerColor(h) {
    if (h == null) return "";
    if (h > 50000) return "color:#FF6363";
    if (h >= 30000) return "color:#ffbc33";
    return "";
  }

  function containerDot(state) {
    if (state === "running") return "s-green";
    if (state === "exited") return "s-red";
    return "s-gray";
  }

  function containerColor(state) {
    if (state === "running") return "color:#5fc992";
    if (state === "exited") return "color:#FF6363";
    return "";
  }

  function sevBadgeClass(sev) {
    if (sev === "critical") return "badge-critical";
    if (sev === "warning") return "badge-warning";
    if (sev === "info") return "badge-info";
    return "badge-ok";
  }

  function sevCardClass(sev) {
    if (sev === "critical") return "sev-critical";
    if (sev === "warning") return "sev-warning";
    if (sev === "info") return "sev-info";
    return "sev-ok";
  }

  function sevIconSymbol(sev) {
    if (sev === "critical") return "\u2715";
    if (sev === "warning") return "!";
    if (sev === "info") return "i";
    return "\u2713";
  }

  function healthDotClass(h) {
    if (h === "critical") return "dot-red";
    if (h === "warning") return "dot-yellow";
    return "dot-green";
  }

  function healthLabel(h) {
    if (h === "critical") return "Your server has critical issues";
    if (h === "warning") return "Attention needed";
    return "All systems healthy";
  }

  function healthTextClass(h) {
    if (h === "critical") return " text-critical";
    return "";
  }

  function fetchJSON(url) {
    return fetch(url).then(function(res) {
      if (!res.ok) throw new Error("HTTP " + res.status);
      return res.json();
    });
  }

  /* Number counting animation */
  function animateCounter(el, target, suffix, duration) {
    if (!el) return;
    var start = 0;
    var startTime = null;
    var isFloat = String(target).indexOf(".") !== -1;
    suffix = suffix || "";
    duration = duration || 600;

    function step(ts) {
      if (!startTime) startTime = ts;
      var progress = Math.min((ts - startTime) / duration, 1);
      var ease = 1 - Math.pow(1 - progress, 3);
      var current = start + (target - start) * ease;
      el.textContent = (isFloat ? current.toFixed(1) : Math.round(current)) + suffix;
      if (progress < 1) requestAnimationFrame(step);
    }
    requestAnimationFrame(step);
  }

  function renderDashboard(status, snapshot) {
    var html = "";

    /* ---- Health Overview ---- */
    html += "<div class=\"section\">";
    html += "<div class=\"card-static\">";
    html += "<div class=\"health-overview\">";
    html += "<div class=\"health-status-area\">";
    html += "<div class=\"health-dot-wrap\">";
    html += "<div class=\"health-dot-ring " + healthDotClass(status.overall_health) + "\"></div>";
    html += "<div class=\"health-dot " + healthDotClass(status.overall_health) + "\"></div>";
    html += "</div>";
    html += "<div class=\"health-text" + healthTextClass(status.overall_health) + "\">" + esc(healthLabel(status.overall_health)) + "</div>";
    html += "</div>";
    html += "<div class=\"health-pills\">";
    if (status.critical_count > 0) {
      html += "<span class=\"pill pill-critical\">" + status.critical_count + " Critical</span>";
    }
    if (status.warning_count > 0) {
      html += "<span class=\"pill pill-warning\">" + status.warning_count + " Warning</span>";
    }
    if (status.info_count > 0) {
      html += "<span class=\"pill pill-info\">" + status.info_count + " Info</span>";
    }
    if (status.critical_count === 0 && status.warning_count === 0 && status.info_count === 0) {
      html += "<span class=\"pill pill-neutral\">No findings</span>";
    }
    html += "</div>";
    html += "</div>";
    html += "</div>";
    html += "</div>";

    /* ---- Scan bar ---- */
    html += "<div class=\"section\">";
    html += "<div class=\"scan-bar\">";
    html += "<div class=\"scan-info\">";
    if (status.last_scan) {
      html += "Last scan: " + esc(new Date(status.last_scan).toLocaleString());
    } else {
      html += "No scans yet";
    }
    if (status.scan_running) {
      html += " &middot; <strong>Scan in progress...</strong>";
    }
    html += "</div>";
    html += "<button class=\"btn btn-primary" + (status.scan_running ? " scanning" : "") + "\" id=\"scanBtn\" onclick=\"window._triggerScan()\"";
    if (status.scan_running) html += " disabled";
    html += ">" + (status.scan_running ? "Scanning..." : "Run Scan") + "</button>";
    html += "</div>";
    html += "</div>";

    /* ---- System Stats ---- */
    if (snapshot && snapshot.system) {
      var sys = snapshot.system;
      html += "<div class=\"section\">";
      html += "<div class=\"section-title\">System</div>";
      html += "<div class=\"stats-grid\">";

      html += "<div class=\"stat-card\">";
      html += "<div class=\"stat-value\" data-counter=\"cpu\" data-target=\"" + (sys.cpu_usage_percent || 0).toFixed(1) + "\" data-suffix=\"%\">0%</div>";
      html += "<div class=\"stat-label\">CPU Usage</div>";
      html += "</div>";

      html += "<div class=\"stat-card\">";
      html += "<div class=\"stat-value\" data-counter=\"mem\" data-target=\"" + (sys.mem_percent || 0).toFixed(1) + "\" data-suffix=\"%\">0%</div>";
      var memDetail = (sys.mem_used_mb || 0) + " / " + (sys.mem_total_mb || 0) + " MB";
      html += "<div class=\"stat-label\">Memory (" + esc(memDetail) + ")</div>";
      html += "</div>";

      html += "<div class=\"stat-card\">";
      html += "<div class=\"stat-value\" data-counter=\"io\" data-target=\"" + (sys.io_wait_percent || 0).toFixed(1) + "\" data-suffix=\"%\">0%</div>";
      html += "<div class=\"stat-label\">I/O Wait</div>";
      html += "</div>";

      html += "<div class=\"stat-card\">";
      html += "<div class=\"stat-value\" data-counter=\"uptime\">" + esc(status.uptime || "-") + "</div>";
      html += "<div class=\"stat-label\">Uptime</div>";
      html += "</div>";

      html += "</div>";
      html += "</div>";
    }

    /* ---- Findings ---- */
    html += "<div class=\"section\">";
    html += "<div class=\"section-title\">Findings</div>";
    if (snapshot && snapshot.findings && snapshot.findings.length > 0) {
      html += "<div class=\"findings-list\">";
      var findings = snapshot.findings.slice().sort(function(a, b) {
        var order = { critical: 0, warning: 1, info: 2, ok: 3 };
        return (order[a.severity] || 3) - (order[b.severity] || 3);
      });
      for (var i = 0; i < findings.length; i++) {
        var f = findings[i];
        var fid = "finding-" + i;
        var isActive = activeFindingId === fid;
        html += "<div class=\"finding-card " + sevCardClass(f.severity) + (isActive ? " active" : "") + "\" data-fid=\"" + fid + "\" onclick=\"window._toggleFinding(this)\">";

        html += "<div class=\"sev-icon sev-icon-" + esc(f.severity) + "\">" + sevIconSymbol(f.severity) + "</div>";

        html += "<div class=\"finding-top\">";
        html += "<span class=\"badge " + sevBadgeClass(f.severity) + "\">" + esc(f.severity) + "</span>";
        html += "<span class=\"finding-category\">" + esc(f.category) + "</span>";
        html += "</div>";

        html += "<div class=\"finding-title\">" + esc(f.title) + "</div>";
        html += "<div class=\"finding-desc\">" + esc(f.description) + "</div>";

        /* Expandable details */
        html += "<div class=\"finding-expandable\">";
        html += "<div class=\"finding-details\">";
        if (f.evidence && f.evidence.length > 0) {
          html += "<div class=\"finding-detail-row\">";
          html += "<div class=\"finding-detail-label\">Evidence</div>";
          html += "<div class=\"finding-detail-value\"><ul class=\"finding-evidence-list\">";
          for (var e = 0; e < f.evidence.length; e++) {
            html += "<li class=\"finding-evidence-item\">" + esc(f.evidence[e]) + "</li>";
          }
          html += "</ul></div>";
          html += "</div>";
        }
        if (f.action) {
          html += "<div class=\"finding-detail-row\">";
          html += "<div class=\"finding-detail-label\">Action</div>";
          html += "<div class=\"finding-detail-value\">" + esc(f.action) + "</div>";
          html += "</div>";
        }
        if (f.impact) {
          html += "<div class=\"finding-detail-row\">";
          html += "<div class=\"finding-detail-label\">Impact</div>";
          html += "<div class=\"finding-detail-value italic\">" + esc(f.impact) + "</div>";
          html += "</div>";
        }
        html += "</div>";
        html += "</div>";

        /* Meta tags */
        if (f.priority || f.cost) {
          html += "<div class=\"finding-meta\">";
          if (f.priority) html += "<span class=\"finding-meta-tag\">" + esc(f.priority) + "</span>";
          if (f.cost) html += "<span class=\"finding-meta-tag\">" + esc(f.cost) + "</span>";
          html += "</div>";
        }

        html += "</div>";
      }
      html += "</div>";
    } else {
      html += "<div class=\"card-static\"><div class=\"empty-state\">No findings to display. Run a scan to check your system.</div></div>";
    }
    html += "</div>";

    /* ---- Disk Space ---- */
    html += "<div class=\"section\">";
    html += "<div class=\"section-title\">Disk Space</div>";
    if (snapshot && snapshot.disks && snapshot.disks.length > 0) {
      html += "<div class=\"card-static\">";
      html += "<div class=\"disk-list\">";
      for (var d = 0; d < snapshot.disks.length; d++) {
        var disk = snapshot.disks[d];
        var pct = disk.used_percent || 0;
        var col = diskBarColor(pct);
        html += "<div class=\"disk-item\">";
        html += "<div class=\"disk-info\">";
        html += "<span class=\"disk-label\">" + esc(disk.label || disk.mount_point || disk.device);
        if (disk.device) html += "<span class=\"disk-device\">" + esc(disk.device) + "</span>";
        html += "</span>";
        html += "<span class=\"disk-detail\">" + fmtGB(disk.used_gb) + " / " + fmtGB(disk.total_gb) + " (" + fmtPct(pct) + ")</span>";
        html += "</div>";
        html += "<div class=\"disk-bar-track\"><div class=\"disk-bar-fill c-" + col + "\" style=\"width:" + pct.toFixed(1) + "%;animation-delay:" + (d * 60) + "ms\"></div></div>";
        html += "</div>";
      }
      html += "</div>";
      html += "</div>";
    } else {
      html += "<div class=\"card-static\"><div class=\"empty-state\">No disk data available.</div></div>";
    }
    html += "</div>";

    /* ---- SMART Health ---- */
    html += "<div class=\"section\">";
    html += "<div class=\"section-title\">SMART Health</div>";
    if (snapshot && snapshot.smart && snapshot.smart.length > 0) {
      html += "<div class=\"table-wrap\">";
      html += "<table>";
      html += "<thead><tr>";
      html += "<th>Device</th><th>Model</th><th>Health</th><th>Temp</th><th>Power-On</th><th>Reallocated</th><th>Pending</th><th>UDMA CRC</th>";
      html += "</tr></thead>";
      html += "<tbody>";
      for (var s = 0; s < snapshot.smart.length; s++) {
        var sm = snapshot.smart[s];
        var healthOk = sm.health_passed;
        var hrs = sm.power_on_hours;
        var hrsStr = hrs != null ? (hrs > 8760 ? (hrs / 8760).toFixed(1) + "y" : hrs + "h") : "-";
        html += "<tr>";
        html += "<td class=\"mono\">" + esc(sm.device) + "</td>";
        html += "<td>" + esc(sm.model) + "</td>";
        html += "<td><span class=\"status-dot " + (healthOk ? "s-green" : "s-red") + "\"></span>" + (healthOk ? "Passed" : "<strong style=\"color:#FF6363\">FAILED</strong>") + "</td>";
        html += "<td class=\"mono\" style=\"" + tempColor(sm.temperature_c) + "\">" + (sm.temperature_c != null ? sm.temperature_c + "&deg;C" : "-") + "</td>";
        html += "<td class=\"mono\" style=\"" + powerColor(hrs) + "\">" + hrsStr + "</td>";
        html += "<td class=\"mono\">" + (sm.reallocated_sectors != null ? sm.reallocated_sectors : "-") + "</td>";
        html += "<td class=\"mono\">" + (sm.pending_sectors != null ? sm.pending_sectors : "-") + "</td>";
        html += "<td class=\"mono\">" + (sm.udma_crc_errors != null ? sm.udma_crc_errors : "-") + "</td>";
        html += "</tr>";
      }
      html += "</tbody>";
      html += "</table>";
      html += "</div>";
    } else {
      html += "<div class=\"card-static\"><div class=\"empty-state\">No SMART data available.</div></div>";
    }
    html += "</div>";

    /* ---- Docker Containers ---- */
    html += "<div class=\"section\">";
    html += "<div class=\"section-title\">Docker Containers</div>";
    if (snapshot && snapshot.docker && snapshot.docker.available && snapshot.docker.containers && snapshot.docker.containers.length > 0) {
      html += "<div class=\"table-wrap\">";
      html += "<table>";
      html += "<thead><tr>";
      html += "<th>Name</th><th>Image</th><th>State</th><th>CPU</th><th>Memory</th><th>Uptime</th>";
      html += "</tr></thead>";
      html += "<tbody>";
      for (var c = 0; c < snapshot.docker.containers.length; c++) {
        var ct = snapshot.docker.containers[c];
        html += "<tr>";
        html += "<td style=\"font-weight:500\">" + esc(ct.name) + "</td>";
        html += "<td style=\"color:#9c9c9d;font-size:13px\">" + esc(ct.image) + "</td>";
        html += "<td style=\"" + containerColor(ct.state) + "\"><span class=\"status-dot " + containerDot(ct.state) + "\"></span>" + esc(ct.state) + "</td>";
        html += "<td class=\"mono\">" + fmtPct(ct.cpu_percent) + "</td>";
        html += "<td class=\"mono\">" + (ct.mem_mb != null ? ct.mem_mb.toFixed(1) + " MB" : "-") + "</td>";
        html += "<td style=\"color:#6a6b6c\">" + esc(ct.uptime || "-") + "</td>";
        html += "</tr>";
      }
      html += "</tbody>";
      html += "</table>";
      html += "</div>";
    } else {
      html += "<div class=\"card-static\"><div class=\"empty-state\">No Docker containers found or Docker not available.</div></div>";
    }
    html += "</div>";

    /* ---- Footer ---- */
    html += "<div class=\"section\">";
    html += "<div class=\"footer\">NAS Doctor &middot; <span>Auto-refreshes every 30s</span></div>";
    html += "</div>";

    return html;
  }

  function postRender(snapshot) {
    /* Animate stat counters */
    var counters = document.querySelectorAll("[data-counter]");
    for (var i = 0; i < counters.length; i++) {
      var el = counters[i];
      var key = el.getAttribute("data-counter");
      var target = parseFloat(el.getAttribute("data-target"));
      var suffix = el.getAttribute("data-suffix") || "";
      if (key === "uptime") continue;
      if (!isNaN(target)) {
        var prev = prevStatValues[key];
        if (prev !== undefined && prev !== target) {
          el.classList.add("value-pulse");
          setTimeout((function(e) { return function() { e.classList.remove("value-pulse"); }; })(el), 400);
        }
        prevStatValues[key] = target;
        animateCounter(el, target, suffix, 600);
      }
    }
  }

  /* Finding toggle */
  window._toggleFinding = function(cardEl) {
    var fid = cardEl.getAttribute("data-fid");
    var allCards = document.querySelectorAll(".finding-card");
    for (var i = 0; i < allCards.length; i++) {
      if (allCards[i] !== cardEl) {
        allCards[i].classList.remove("active");
      }
    }
    var isNowActive = cardEl.classList.toggle("active");
    activeFindingId = isNowActive ? fid : null;
  };

  function loadData() {
    Promise.all([
      fetchJSON("/api/v1/status"),
      fetchJSON("/api/v1/snapshot/latest").catch(function() { return null; })
    ]).then(function(results) {
      var status = results[0];
      var snapshot = results[1];

      var hostnameEl = document.getElementById("hostname");
      if (hostnameEl && status.hostname) {
        hostnameEl.textContent = status.hostname;
      }

      if (status.hostname) {
        document.title = "NAS Doctor - " + status.hostname;
      }

      var app = document.getElementById("app");
      if (app) {
        app.innerHTML = renderDashboard(status, snapshot);
        postRender(snapshot);
      }
    }).catch(function(err) {
      console.error("Failed to load dashboard data:", err);
      var app2 = document.getElementById("app");
      if (app2) {
        var ld = document.getElementById("loadingState");
        if (ld) {
          ld.textContent = "Failed to load data. Retrying...";
        }
      }
    });
  }

  window._triggerScan = function() {
    var btn = document.getElementById("scanBtn");
    if (btn) {
      btn.disabled = true;
      btn.classList.add("scanning");
      btn.textContent = "Scanning...";
    }
    fetch("/api/v1/scan", { method: "POST" }).then(function(res) {
      if (!res.ok) return res.json().then(function(d) { throw new Error(d.error || "Scan failed"); });
      setTimeout(loadData, 2000);
    }).catch(function(err) {
      alert("Scan error: " + err.message);
      if (btn) {
        btn.disabled = false;
        btn.classList.remove("scanning");
        btn.textContent = "Run Scan";
      }
    });
  };

  loadData();
  refreshTimer = setInterval(loadData, REFRESH_INTERVAL);
})();
</script>
</body>
</html>`
