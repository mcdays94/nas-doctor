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
  --shadow-ring: rgba(255, 255, 255, 0.06) 0px 0px 0px 1px, rgba(0, 0, 0, 0.4) 0px 2px 6px, rgba(0, 0, 0, 0.2) 0px 8px 24px;
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

@keyframes pulseSlow {
  0%, 100% { box-shadow: 0 0 0 0 rgba(255, 255, 255, 0); }
  50% { box-shadow: 0 0 10px 2px rgba(255, 255, 255, 0.15); }
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
  max-width: 1280px;
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

.header-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.theme-switcher {
  display: flex;
  align-items: center;
  gap: 2px;
  background: var(--surface-200);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 2px;
}

.theme-switcher a {
  font-family: var(--font-sans);
  padding: 5px 10px;
  border-radius: 6px;
  font-size: 11px;
  font-weight: 500;
  color: var(--text-dim);
  text-decoration: none;
  transition: all 200ms var(--ease-out);
  line-height: 1.3;
}

.theme-switcher a:hover {
  color: var(--text-tertiary);
}

.theme-switcher a.active {
  color: var(--text-primary);
  background: var(--surface-100);
  box-shadow: 0 1px 3px rgba(0,0,0,0.4);
}

.nav-links {
  display: flex;
  gap: 4px;
}

.nav-link {
  font-family: var(--font-sans);
  font-size: 13px;
  font-weight: 500;
  color: var(--text-dim);
  text-decoration: none;
  padding: 6px 12px;
  border-radius: 6px;
  transition: color 200ms var(--ease-out);
}

.nav-link:hover {
  color: var(--text-secondary);
}

/* ---- Compact Top Bar (health + stats combined) ---- */
.top-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 10px 20px;
  background: var(--surface-100);
  border-radius: 12px;
  box-shadow: var(--shadow-ring);
  max-height: 52px;
  overflow: hidden;
  flex-wrap: nowrap;
}

.top-bar-health {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.top-bar-dot-wrap {
  position: relative;
  width: 8px;
  height: 8px;
  flex-shrink: 0;
}

.top-bar-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  position: absolute;
  top: 0;
  left: 0;
}

.top-bar-dot-ring {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  position: absolute;
  top: 0;
  left: 0;
}

.top-bar-dot.dot-green       { background: var(--green); }
.top-bar-dot-ring.dot-green  { background: var(--green); animation: pulseRing 2s ease infinite; }
.top-bar-dot.dot-yellow      { background: var(--yellow); }
.top-bar-dot-ring.dot-yellow { background: var(--yellow); animation: pulseRing 1.5s ease infinite; }
.top-bar-dot.dot-red         { background: var(--red); }
.top-bar-dot-ring.dot-red    { background: var(--red); animation: pulseRing 0.8s ease infinite; }

.top-bar-label {
  font-family: var(--font-serif);
  font-size: 14px;
  font-weight: 500;
  color: var(--text-primary);
  white-space: nowrap;
  letter-spacing: -0.2px;
}

.top-bar-label.text-critical {
  font-weight: 600;
  color: var(--red);
}

.top-bar-sep {
  width: 1px;
  height: 24px;
  background: var(--border);
  flex-shrink: 0;
}

.top-bar-pills {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}

.top-bar-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-family: var(--font-sans);
  font-size: 12px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  padding: 2px 8px;
  border-radius: 6px;
  white-space: nowrap;
}

.top-bar-pill-critical { background: rgba(255, 99, 99, 0.12); color: var(--red); }
.top-bar-pill-warning  { background: rgba(255, 188, 51, 0.12); color: var(--yellow); }
.top-bar-pill-info     { background: rgba(85, 179, 255, 0.12); color: var(--blue); }
.top-bar-pill-ok       { background: rgba(95, 201, 146, 0.12); color: var(--green); }

.top-bar-stats {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-left: auto;
  flex-shrink: 0;
}

.top-bar-stat {
  font-family: var(--font-sans);
  font-size: 13px;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
  color: var(--text-secondary);
  white-space: nowrap;
}

.top-bar-stat-label {
  color: var(--text-dim);
  font-weight: 400;
  margin-right: 3px;
}

.top-bar-stat-value {
  font-weight: 600;
  color: var(--text-primary);
}

.top-bar-mid-dot {
  color: var(--text-dim);
  font-size: 11px;
  flex-shrink: 0;
}

@media (max-width: 900px) {
  .top-bar {
    flex-wrap: wrap;
    max-height: none;
    gap: 8px;
    padding: 12px 16px;
  }
  .top-bar-stats {
    margin-left: 0;
    flex-wrap: wrap;
  }
}

/* ---- Section spacing ---- */
.section {
  margin-top: 16px;
}

.section-title {
  font-family: var(--font-serif);
  font-size: 18px;
  font-weight: 500;
  letter-spacing: -0.2px;
  color: var(--text-primary);
  margin-bottom: 12px;
}

/* ---- Two Column Grid ---- */
.two-col {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

@media (max-width: 900px) {
  .two-col {
    grid-template-columns: 1fr;
  }
}

.col-left {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
  min-height: 0;
}

.col-right {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
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
  padding: 20px;
}

/* ---- Badges / Pills ---- */
.badge {
  display: inline-flex;
  align-items: center;
  padding: 3px 8px;
  border-radius: 5px;
  font-family: var(--font-sans);
  font-size: 11px;
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

/* ---- Scan bar (compact) ---- */
.scan-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.scan-info {
  font-family: var(--font-sans);
  font-size: 12px;
  color: var(--text-dim);
  font-weight: 400;
}

.scan-info strong {
  color: var(--yellow);
  font-weight: 500;
}

/* ---- Findings (compact) ---- */
.findings-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  scrollbar-width: thin;
  scrollbar-color: var(--surface-300) transparent;
}
.findings-list::-webkit-scrollbar { width: 5px; }
.findings-list::-webkit-scrollbar-track { background: transparent; }
.findings-list::-webkit-scrollbar-thumb { background: var(--surface-300); border-radius: 3px; }
.findings-list::-webkit-scrollbar-thumb:hover { background: var(--text-dim); }

.finding-card {
  position: relative;
  background: var(--surface-100);
  border-radius: 10px;
  padding: 14px 14px 12px 48px;
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
  width: 100px;
  height: 100px;
  border-radius: 50%;
  filter: blur(36px);
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
  top: 13px;
  left: 12px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
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
  gap: 8px;
  margin-bottom: 4px;
}

.finding-title {
  font-family: var(--font-serif);
  font-size: 15px;
  font-weight: 500;
  color: var(--text-primary);
  letter-spacing: -0.2px;
  line-height: 1.3;
}

.finding-category {
  font-family: var(--font-sans);
  font-size: 10px;
  font-weight: 500;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 1px;
}

.finding-desc {
  font-family: var(--font-serif);
  font-size: 13px;
  font-weight: 400;
  font-style: italic;
  color: var(--text-secondary);
  line-height: 1.5;
  margin-top: 2px;
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
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.finding-detail-row {
  display: flex;
  gap: 8px;
}

.finding-detail-label {
  font-family: var(--font-sans);
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--text-dim);
  min-width: 64px;
  padding-top: 2px;
}

.finding-detail-value {
  font-family: var(--font-serif);
  font-size: 13px;
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
  gap: 3px;
}

.finding-evidence-item {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-tertiary);
  padding: 3px 6px;
  background: rgba(255, 255, 255, 0.02);
  border-radius: 4px;
  line-height: 1.4;
}

.finding-meta {
  display: flex;
  gap: 6px;
  margin-top: 8px;
  flex-wrap: wrap;
}

.finding-meta-tag {
  font-family: var(--font-sans);
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  padding: 2px 6px;
  border-radius: 5px;
  background: var(--surface-200);
  color: var(--text-dim);
}

/* ---- Disk bars (compact) ---- */
.disk-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.disk-item {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.disk-info {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
}

.disk-label {
  font-family: var(--font-sans);
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.disk-device {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-dim);
  margin-left: 6px;
}

.disk-detail {
  font-family: var(--font-mono);
  font-size: 12px;
  font-weight: 400;
  color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
}

.disk-bar-track {
  width: 100%;
  height: 8px;
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
  font-size: 10px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 1px;
  color: var(--text-dim);
  padding: 10px 12px;
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
  font-size: 13px;
  font-weight: 400;
  color: var(--text-primary);
  padding: 9px 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.03);
}

.table-wrap tbody tr:last-child td {
  border-bottom: none;
}

td.mono {
  font-family: var(--font-mono);
  font-size: 12px;
}

/* ---- Status dot ---- */
.status-dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  margin-right: 5px;
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
  font-size: 13px;
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
  padding: 8px 20px;
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

/* ---- Empty state ---- */
.empty-state {
  text-align: center;
  padding: 32px 16px;
  font-family: var(--font-serif);
  font-style: italic;
  color: var(--text-dim);
  font-size: 14px;
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
  padding: 24px 24px;
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
  <div class="header-right">
    <div class="theme-switcher">
      <a href="/">Midnight</a>
      <a href="/theme/clean">Clean</a>
      <a href="/theme/ember" class="active">Ember</a>
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
    <div class="loading" id="loadingState">Awaiting first diagnostic scan...</div>
  </div>
</div>

<script src="/js/charts.js"></script>
<script>
(function() {
  "use strict";

  var REFRESH_INTERVAL = 30000;
  var refreshTimer = null;
  var activeFindingId = null;
  var _cachedStatus = null;
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

  function healthLabelShort(h) {
    if (h === "critical") return "Critical";
    if (h === "warning") return "Warning";
    return "Healthy";
  }

  function healthDesc(h) {
    if (h === "critical") return "Your server has critical issues";
    if (h === "warning") return "Attention needed";
    return "All systems healthy";
  }

  function healthTextClass(h) {
    if (h === "critical") return " text-critical";
    return "";
  }

  function fmtUptime(u) {
    if (!u) return "-";
    var m = u.match(/(\d+)d/);
    if (m) return m[1] + "d";
    m = u.match(/(\d+)h/);
    if (m) return m[1] + "h";
    return u;
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
    var sys = (snapshot && snapshot.system) ? snapshot.system : null;

    /* ---- Compact Top Bar: hostname + health + pills + inline stats ---- */
    html += "<div class=\"section\">";
    html += "<div class=\"top-bar\">";

    /* Hostname */
    if (status.hostname) {
      html += "<span id=\"hostname\" style=\"font-family:var(--font-mono);font-size:13px;font-weight:500;color:var(--text-secondary);letter-spacing:-0.2px\">" + esc(status.hostname) + "</span>";
      html += "<div class=\"top-bar-sep\"></div>";
    }

    /* Health dot + label */
    html += "<div class=\"top-bar-health\">";
    html += "<div class=\"top-bar-dot-wrap\">";
    html += "<div class=\"top-bar-dot-ring " + healthDotClass(status.overall_health) + "\"></div>";
    html += "<div class=\"top-bar-dot " + healthDotClass(status.overall_health) + "\"></div>";
    html += "</div>";
    html += "<span class=\"top-bar-label" + healthTextClass(status.overall_health) + "\">" + esc(healthLabelShort(status.overall_health)) + " &mdash; " + esc(healthDesc(status.overall_health)) + "</span>";
    html += "</div>";

    html += "<div class=\"top-bar-sep\"></div>";

    /* Severity pills */
    html += "<div class=\"top-bar-pills\">";
    if (status.critical_count > 0) {
      html += "<span class=\"top-bar-pill top-bar-pill-critical\">" + status.critical_count + " \u25cf</span>";
    }
    if (status.warning_count > 0) {
      html += "<span class=\"top-bar-pill top-bar-pill-warning\">" + status.warning_count + " \u25cf</span>";
    }
    if (status.info_count > 0) {
      html += "<span class=\"top-bar-pill top-bar-pill-info\">" + status.info_count + " \u25cf</span>";
    }
    if (status.critical_count === 0 && status.warning_count === 0 && status.info_count === 0) {
      html += "<span class=\"top-bar-pill top-bar-pill-ok\">0 findings</span>";
    }
    html += "</div>";

    html += "<div class=\"top-bar-sep\"></div>";

    /* Inline stats */
    html += "<div class=\"top-bar-stats\">";
    if (sys) {
      html += "<span class=\"top-bar-stat\"><span class=\"top-bar-stat-label\">CPU</span><span class=\"top-bar-stat-value\" data-counter=\"cpu\" data-target=\"" + (sys.cpu_usage_percent || 0).toFixed(1) + "\" data-suffix=\"%\">0%</span><canvas id=\"spark-cpu\" width=\"44\" height=\"18\" style=\"margin-left:4px;vertical-align:middle\"></canvas></span>";
      html += "<span class=\"top-bar-mid-dot\">&middot;</span>";
      html += "<span class=\"top-bar-stat\"><span class=\"top-bar-stat-label\">Mem</span><span class=\"top-bar-stat-value\" data-counter=\"mem\" data-target=\"" + (sys.mem_percent || 0).toFixed(1) + "\" data-suffix=\"%\">0%</span><canvas id=\"spark-mem\" width=\"44\" height=\"18\" style=\"margin-left:4px;vertical-align:middle\"></canvas></span>";
      html += "<span class=\"top-bar-mid-dot\">&middot;</span>";
      html += "<span class=\"top-bar-stat\"><span class=\"top-bar-stat-label\">I/O</span><span class=\"top-bar-stat-value\" data-counter=\"io\" data-target=\"" + (sys.io_wait_percent || 0).toFixed(1) + "\" data-suffix=\"%\">0%</span></span>";
      html += "<span class=\"top-bar-mid-dot\">&middot;</span>";
      html += "<span class=\"top-bar-stat\"><span class=\"top-bar-stat-value\" data-counter=\"uptime\">" + esc(fmtUptime(status.uptime)) + "</span></span>";
    } else {
      html += "<span class=\"top-bar-stat\" style=\"color:var(--text-dim)\">No system data</span>";
    }
    html += "</div>";

    html += "</div>"; /* .top-bar */
    html += "</div>"; /* .section */

    /* ---- Scan bar (compact) ---- */
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

    /* ---- Two-Column Grid ---- */
    /* OS version banner */
    var sysP = (snapshot && snapshot.system) ? snapshot.system.platform : null;
    var sysV = (snapshot && snapshot.system) ? snapshot.system.platform_version : null;
    if (sysP && sysV) {
      var hasUpd = snapshot && snapshot.update && snapshot.update.update_available;
      var bgS = hasUpd ? "background:rgba(85,179,255,0.06);border:1px solid rgba(85,179,255,0.15)" : "background:var(--surface-200);border:1px solid var(--border)";
      html += "<div style=\"display:flex;align-items:center;gap:8px;padding:10px 14px;" + bgS + ";border-radius:8px;margin-bottom:12px;font-size:12px\">";
      html += "<span style=\"color:var(--text-primary);font-weight:600\">" + esc(sysP) + "</span>";
      html += "<span style=\"color:var(--text-tertiary)\">v" + esc(sysV) + "</span>";
      if (hasUpd) {
        html += "<span style=\"color:var(--blue);font-weight:600;margin-left:8px\">Update → " + esc(snapshot.update.latest_version) + "</span>";
        if (snapshot.update.release_url) html += "<a href=\"" + esc(snapshot.update.release_url) + "\" target=\"_blank\" style=\"color:var(--blue);text-decoration:underline;margin-left:auto\">Release notes</a>";
      }
      html += "</div>";
    }

    html += "<div class=\"two-col\" id=\"two-col\">";
    html += "<div class=\"col-left\" id=\"col-left\"></div>";
    html += "<div class=\"col-right\" id=\"col-right\"></div>";
    html += "</div>";

    html += "<div id=\"section-staging\" style=\"position:absolute;visibility:hidden;width:50%;left:-9999px\">";

    /* ==== Section: Findings ==== */
    html += "<div class=\"section-block\" data-section=\"findings\">";
    html += "<div class=\"section\" style=\"margin-top:0\">";
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

        html += "</div>"; /* .finding-card */
      }
      html += "</div>"; /* .findings-list */
    } else {
      html += "<div class=\"card-static\"><div class=\"empty-state\">No findings to display. Run a scan to check your system.</div></div>";
    }
    html += "</div>"; /* .section */
    html += "</div>"; /* section-block findings */

    /* ==== Section: Disk Space ==== */
    html += "<div class=\"section-block\" data-section=\"disk-space\">";
    html += "<div class=\"section\" style=\"margin-top:0\">";
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
        html += "<span class=\"disk-detail\">" + fmtPct(pct) + "</span>";
        html += "</div>";
        html += "<div class=\"disk-bar-track\"><div class=\"disk-bar-fill c-" + col + "\" style=\"width:" + pct.toFixed(1) + "%;animation-delay:" + (d * 60) + "ms\"></div></div>";
        html += "</div>";
      }
      html += "</div>";
      html += "</div>";
    } else {
      html += "<div class=\"card-static\"><div class=\"empty-state\">No disk data available.</div></div>";
    }
    html += "</div>"; /* section */
    html += "</div>"; /* section-block disk-space */

    /* ==== Section: SMART Health ==== */
    html += "<div class=\"section-block\" data-section=\"smart\">";
    html += "<div class=\"section\" style=\"margin-top:0\">";
    html += "<div class=\"section-title\">SMART Health</div>";
    if (snapshot && snapshot.smart && snapshot.smart.length > 0) {
      html += "<div class=\"table-wrap\">";
      html += "<table>";
      html += "<thead><tr>";
      html += "<th>Device</th><th>Health</th><th>Temp</th><th style=\"width:70px\">Trend</th><th>Power-On</th><th>Realloc</th><th>Pending</th>";
      html += "</tr></thead>";
      html += "<tbody>";
      for (var s = 0; s < snapshot.smart.length; s++) {
        var sm = snapshot.smart[s];
        var healthOk = sm.health_passed;
        var hrs = sm.power_on_hours;
        var hrsStr = hrs != null ? (hrs > 8760 ? (hrs / 8760).toFixed(1) + "y" : hrs + "h") : "-";
        html += "<tr style=\"cursor:pointer\" onclick=\"window.location='/disk/" + encodeURIComponent(sm.serial || '') + "'\" title=\"" + esc(sm.model || sm.device) + "\">";
        html += "<td class=\"mono\">" + esc(sm.device) + "</td>";
        html += "<td><span class=\"status-dot " + (healthOk ? "s-green" : "s-red") + "\"></span>" + (healthOk ? "OK" : "<strong style=\"color:#FF6363\">FAIL</strong>") + "</td>";
        html += "<td class=\"mono\" style=\"" + tempColor(sm.temperature_c) + "\">" + (sm.temperature_c != null ? sm.temperature_c + "&deg;" : "-") + "</td>";
        html += "<td><canvas id=\"spark-temp-" + s + "\" width=\"60\" height=\"20\"></canvas></td>";
        html += "<td class=\"mono\" style=\"" + powerColor(hrs) + "\">" + hrsStr + "</td>";
        html += "<td class=\"mono\">" + (sm.reallocated_sectors != null ? sm.reallocated_sectors : "-") + "</td>";
        html += "<td class=\"mono\">" + (sm.pending_sectors != null ? sm.pending_sectors : "-") + "</td>";
        html += "</tr>";
      }
      html += "</tbody>";
      html += "</table>";
      html += "</div>";
    } else {
      html += "<div class=\"card-static\"><div class=\"empty-state\">No SMART data available.</div></div>";
    }
    html += "</div>"; /* section */
    html += "</div>"; /* section-block smart */

    /* ==== Section: Docker ==== */
    html += "<div class=\"section-block\" data-section=\"docker\">";
    html += "<div class=\"section\" style=\"margin-top:0\">";
    html += "<div class=\"section-title\">Docker</div>";
    if (snapshot && snapshot.docker && snapshot.docker.available && snapshot.docker.containers && snapshot.docker.containers.length > 0) {
      html += "<div class=\"table-wrap\">";
      html += "<table>";
      html += "<thead><tr>";
      html += "<th>Name</th><th>State</th><th>CPU</th><th>Mem</th>";
      html += "</tr></thead>";
      html += "<tbody>";
      for (var c = 0; c < snapshot.docker.containers.length; c++) {
        var ct = snapshot.docker.containers[c];
        html += "<tr title=\"" + esc(ct.image || "") + "\">";
        html += "<td style=\"font-weight:500;font-size:12px\">" + esc(ct.name) + "</td>";
        html += "<td style=\"" + containerColor(ct.state) + ";font-size:12px\"><span class=\"status-dot " + containerDot(ct.state) + "\"></span>" + esc(ct.state) + "</td>";
        html += "<td class=\"mono\">" + fmtPct(ct.cpu_percent) + "</td>";
        html += "<td class=\"mono\">" + (ct.mem_mb != null ? ct.mem_mb.toFixed(0) + "M" : "-") + "</td>";
        html += "</tr>";
      }
      html += "</tbody>";
      html += "</table>";
      html += "</div>";
    } else {
      html += "<div class=\"card-static\"><div class=\"empty-state\">No Docker containers found.</div></div>";
    }
    html += "</div>"; /* section */
    html += "</div>"; /* section-block docker */

    /* ==== Section: ZFS Pools ==== */
    html += "<div class=\"section-block\" data-section=\"zfs\">";
    if (snapshot && snapshot.zfs && snapshot.zfs.available && snapshot.zfs.pools && snapshot.zfs.pools.length > 0) {
      html += "<div class=\"section\" style=\"margin-top:0\">";
      html += "<div class=\"section-title\">ZFS Pools</div>";
      for (var zi = 0; zi < snapshot.zfs.pools.length; zi++) {
        var zp = snapshot.zfs.pools[zi];
        var sDot = zp.state === "ONLINE" ? "s-green" : zp.state === "DEGRADED" ? "s-amber" : "s-red";
        html += "<div class=\"card-static\" style=\"margin-bottom:8px;padding:12px\">";
        html += "<div style=\"display:flex;justify-content:space-between;align-items:center;margin-bottom:6px\">";
        html += "<span style=\"font-family:var(--font-mono);font-weight:500;font-size:14px\">" + esc(zp.name) + "</span>";
        html += "<span class=\"status-dot " + sDot + "\"></span><span class=\"mono\" style=\"font-size:11px\">" + esc(zp.state) + "</span>";
        html += "</div>";
        html += "<div style=\"font-size:12px;color:var(--text-tertiary)\">" + (zp.used_gb || 0).toFixed(0) + " / " + (zp.total_gb || 0).toFixed(0) + " GB (" + (zp.used_percent || 0).toFixed(0) + "%)</div>";
        if (zp.scan_type && zp.scan_type !== "none") {
          html += "<div style=\"font-size:11px;color:var(--text-dim);margin-top:4px\">Last " + esc(zp.scan_type) + ": " + (zp.scan_errors || 0) + " errors</div>";
        }
        if (zp.vdevs && zp.vdevs.length > 0) {
          html += "<div style=\"margin-top:6px;font-size:11px;font-family:var(--font-mono);color:var(--text-dim)\">";
          for (var vi = 0; vi < zp.vdevs.length; vi++) {
            var vd = zp.vdevs[vi];
            var vClr = vd.state === "ONLINE" ? "var(--green)" : vd.state === "DEGRADED" ? "#FACC15" : "var(--red)";
            html += "<div>" + esc(vd.name) + " <span style=\"color:" + vClr + "\">" + esc(vd.state) + "</span></div>";
            if (vd.children) {
              for (var ci = 0; ci < vd.children.length; ci++) {
                var ch = vd.children[ci];
                var cClr = ch.state === "ONLINE" ? "var(--green)" : ch.state === "DEGRADED" ? "#FACC15" : "var(--red)";
                html += "<div style=\"padding-left:14px\">" + esc(ch.name) + " <span style=\"color:" + cClr + "\">" + esc(ch.state) + "</span></div>";
              }
            }
          }
          html += "</div>";
        }
        html += "</div>";
      }
      if (snapshot.zfs.arc) {
        html += "<div style=\"font-size:11px;color:var(--text-dim);margin-top:4px\">ARC: " + (snapshot.zfs.arc.size_mb / 1024).toFixed(1) + " GB / " + (snapshot.zfs.arc.max_size_mb / 1024).toFixed(1) + " GB &middot; Hit rate: " + (snapshot.zfs.arc.hit_rate_percent || 0).toFixed(1) + "%</div>";
      }
      html += "</div>";
    }

    html += "</div>"; /* section-block zfs */

    /* ==== Section: UPS ==== */
    html += "<div class=\"section-block\" data-section=\"ups\">";
    if (snapshot && snapshot.ups && snapshot.ups.available) {
      var ups = snapshot.ups;
      html += "<div class=\"section\" style=\"margin-top:0\">";
      html += "<div class=\"section-title\">UPS / Power</div>";
      var upsDot = ups.on_battery ? "s-red" : "s-green";
      html += "<div class=\"card-static\" style=\"padding:12px\">";
      html += "<div style=\"display:flex;justify-content:space-between;align-items:center;margin-bottom:6px\">";
      html += "<span style=\"font-family:var(--font-mono);font-weight:500;font-size:14px\">" + esc(ups.name || ups.model) + "</span>";
      html += "<span class=\"status-dot " + upsDot + "\"></span><span class=\"mono\" style=\"font-size:11px\">" + esc(ups.status_human) + "</span>";
      html += "</div>";
      html += "<div style=\"display:flex;gap:14px;font-size:12px;color:var(--text-tertiary);flex-wrap:wrap\">";
      html += "<span>Battery: <strong style=\"color:var(--text-primary)\">" + (ups.battery_percent || 0).toFixed(0) + "%</strong></span>";
      html += "<span>Load: <strong style=\"color:var(--text-primary)\">" + (ups.load_percent || 0).toFixed(0) + "%</strong></span>";
      html += "<span>Runtime: <strong style=\"color:var(--text-primary)\">" + (ups.runtime_minutes || 0).toFixed(0) + " min</strong></span>";
      if (ups.wattage_watts > 0) html += "<span>" + (ups.wattage_watts || 0).toFixed(0) + "W / " + (ups.nominal_watts || 0).toFixed(0) + "W</span>";
      html += "</div>";
      if (ups.last_transfer) html += "<div style=\"font-size:11px;color:var(--text-dim);margin-top:4px\">Last transfer: " + esc(ups.last_transfer) + "</div>";
      html += "</div>";
      html += "</div>";
    }
    html += "</div>"; /* section-block ups */

    html += "</div>"; /* section-staging */

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
      _cachedStatus = status;
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
        _distributeSections();
        _renderSparklines(snapshot);
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
          try { NasChart.sparkline("spark-cpu", { data: cpuD, color: "#55b3ff", width: 44, height: 18 }); } catch(e) {}
          try { NasChart.sparkline("spark-mem", { data: memD, color: "#5fc992", width: 44, height: 18 }); } catch(e) {}
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
              var clr = mx >= 55 ? "#FF6363" : mx >= 45 ? "#FACC15" : "#5fc992";
              try { NasChart.sparkline("spark-temp-" + i, { data: temps, color: clr, width: 60, height: 20 }); } catch(e) {}
            }
          }
        }
      }).catch(function() {});
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
