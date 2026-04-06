package api

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// escHTML escapes a string for safe inclusion in HTML.
func escHTML(s string) string {
	return html.EscapeString(s)
}

// severityClass returns the CSS class name for a given severity level.
func severityClass(s internal.Severity) string {
	switch s {
	case internal.SeverityCritical:
		return "sev-critical"
	case internal.SeverityWarning:
		return "sev-warning"
	case internal.SeverityInfo:
		return "sev-info"
	case internal.SeverityOK:
		return "sev-ok"
	default:
		return "sev-info"
	}
}

// severityLabel returns a human-friendly label for a severity.
func severityLabel(s internal.Severity) string {
	switch s {
	case internal.SeverityCritical:
		return "Critical"
	case internal.SeverityWarning:
		return "Warning"
	case internal.SeverityInfo:
		return "Info"
	case internal.SeverityOK:
		return "OK"
	default:
		return "Info"
	}
}

// barColor returns the CSS color for a disk-usage bar fill.
func barColor(pct float64) string {
	if pct > 97 {
		return "#dc2626" // red-600
	}
	if pct > 95 {
		return "#ef4444" // red-500
	}
	if pct > 90 {
		return "#f59e0b" // amber-500
	}
	return "#3b82f6" // blue-500
}

// formatAge converts power-on hours to a human string like "4.5 years".
// fmtHours formats power-on hours compactly: "38K" or "1.2K"
func fmtHours(hours int64) string {
	if hours <= 0 {
		return "0"
	}
	if hours < 1000 {
		return fmt.Sprintf("%d", hours)
	}
	return fmt.Sprintf("%.1fK", float64(hours)/1000)
}

func formatAge(hours int64) string {
	if hours <= 0 {
		return "N/A"
	}
	days := float64(hours) / 24.0
	if days < 1 {
		return fmt.Sprintf("%d hrs", hours)
	}
	if days < 365 {
		return fmt.Sprintf("%.0f days", days)
	}
	years := days / 365.25
	return fmt.Sprintf("%.1f years", years)
}

// formatUptime converts seconds to a human string like "30 days" or "2 hours".
func formatUptime(secs int64) string {
	if secs <= 0 {
		return "N/A"
	}
	days := secs / 86400
	hours := (secs % 86400) / 3600
	mins := (secs % 3600) / 60
	if days > 0 {
		return fmt.Sprintf("%d days, %d hrs", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%d hrs, %d min", hours, mins)
	}
	return fmt.Sprintf("%d min", mins)
}

// fmtDurationSecs converts seconds to a duration string for parity checks.
func fmtDurationSecs(secs int64) string {
	if secs <= 0 {
		return "N/A"
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// valClass returns a CSS class for a value based on thresholds.
func valClass(val float64, warnThresh, badThresh float64) string {
	if val >= badThresh {
		return "val-bad"
	}
	if val >= warnThresh {
		return "val-warn"
	}
	return "val-good"
}

// countSeverity counts findings by severity.
func countSeverity(findings []internal.Finding, sev internal.Severity) int {
	n := 0
	for _, f := range findings {
		if f.Severity == sev {
			n++
		}
	}
	return n
}

// groupFindings returns findings grouped and ordered: critical, warning, info, ok.
func groupFindings(findings []internal.Finding) []internal.Finding {
	sorted := make([]internal.Finding, len(findings))
	copy(sorted, findings)
	order := map[internal.Severity]int{
		internal.SeverityCritical: 0,
		internal.SeverityWarning:  1,
		internal.SeverityInfo:     2,
		internal.SeverityOK:       3,
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		return order[sorted[i].Severity] < order[sorted[j].Severity]
	})
	return sorted
}

// healthPassedStr returns "PASSED" or "FAILED".
func healthPassedStr(passed bool) string {
	if passed {
		return "PASSED"
	}
	return "FAILED"
}

// healthClass returns a CSS class for SMART health.
func healthClass(passed bool) string {
	if passed {
		return "val-good"
	}
	return "val-bad"
}

// priorityColor returns the CSS color for an action priority.
func priorityColor(p string) string {
	lower := strings.ToLower(p)
	if strings.Contains(lower, "immediate") {
		return "#dc2626" // red-600
	}
	if strings.Contains(lower, "short") {
		return "#d97706" // amber-600
	}
	if strings.Contains(lower, "medium") {
		return "#2563eb" // blue-600
	}
	return "#475569" // slate-600
}

// arrayStatus derives an overall array status from findings.
func arrayStatus(findings []internal.Finding) string {
	crit := countSeverity(findings, internal.SeverityCritical)
	warn := countSeverity(findings, internal.SeverityWarning)
	if crit > 0 {
		return "Attention Required"
	}
	if warn > 0 {
		return "Warnings Present"
	}
	return "Healthy"
}

// GenerateReport takes a Snapshot and returns a self-contained HTML string
// suitable for browser viewing and Chrome print-to-PDF.
func GenerateReport(snap *internal.Snapshot) string {
	if snap == nil {
		return "<!DOCTYPE html><html><body><p>No snapshot data available.</p></body></html>"
	}

	var b strings.Builder

	// ── HTML head + CSS ──────────────────────────────────────────────
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"UTF-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	b.WriteString("<title>Diagnostic Report — " + escHTML(snap.System.Hostname) + "</title>\n")
	b.WriteString("<link rel=\"preconnect\" href=\"https://fonts.googleapis.com\">\n")
	b.WriteString("<link rel=\"preconnect\" href=\"https://fonts.gstatic.com\" crossorigin>\n")
	b.WriteString("<link href=\"https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&display=swap\" rel=\"stylesheet\">\n")
	b.WriteString("<style>\n")
	writeCSS(&b)
	b.WriteString("</style>\n")
	b.WriteString("</head>\n<body>\n")

	// ── Cover Page ───────────────────────────────────────────────────
	writeCoverPage(&b, snap)

	// ── Table of Contents ────────────────────────────────────────────
	writeTOC(&b, snap)

	// ── System Overview ──────────────────────────────────────────────
	writeSystemOverview(&b, snap)

	// ── Findings ─────────────────────────────────────────────────────
	writeFindings(&b, snap)

	// ── Drive Health & SMART ─────────────────────────────────────────
	writeSMART(&b, snap)

	// ── Docker & Application Analysis ────────────────────────────────
	writeDocker(&b, snap)

	// ── Parity Analysis ──────────────────────────────────────────────
	writeParity(&b, snap)

	// ── Recommended Actions ──────────────────────────────────────────
	writeActions(&b, snap)

	// ── Footer ───────────────────────────────────────────────────────
	writeFooter(&b, snap)

	b.WriteString("</body>\n</html>")
	return b.String()
}

// ─────────────────────────────────────────────────────────────────────
// CSS
// ─────────────────────────────────────────────────────────────────────

func writeCSS(b *strings.Builder) {
	b.WriteString("@import url('https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&display=swap');\n")
	b.WriteString("@page { size: A4; margin: 20mm 18mm 22mm 18mm; }\n")
	b.WriteString("*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }\n")
	b.WriteString(":root {\n")
	b.WriteString("  --red-50: #fef2f2; --red-100: #fee2e2; --red-200: #fecaca; --red-500: #ef4444; --red-600: #dc2626; --red-700: #b91c1c;\n")
	b.WriteString("  --amber-50: #fffbeb; --amber-100: #fef3c7; --amber-200: #fde68a; --amber-500: #f59e0b; --amber-600: #d97706; --amber-700: #b45309;\n")
	b.WriteString("  --green-50: #f0fdf4; --green-100: #dcfce7; --green-500: #22c55e; --green-600: #16a34a;\n")
	b.WriteString("  --blue-50: #eff6ff; --blue-100: #dbeafe; --blue-500: #3b82f6; --blue-600: #2563eb; --blue-700: #1d4ed8;\n")
	b.WriteString("  --slate-50: #f8fafc; --slate-100: #f1f5f9; --slate-200: #e2e8f0; --slate-300: #cbd5e1;\n")
	b.WriteString("  --slate-400: #94a3b8; --slate-500: #64748b; --slate-600: #475569; --slate-700: #334155; --slate-800: #1e293b; --slate-900: #0f172a;\n")
	b.WriteString("}\n")

	// Body
	b.WriteString("body { font-family: 'Inter', system-ui, -apple-system, sans-serif; font-size: 9.5pt; color: var(--slate-800); background: #fff; line-height: 1.6; -webkit-font-smoothing: antialiased; }\n")

	// Cover page
	b.WriteString(".cover { page-break-after: always; padding: 60px 0 40px; }\n")
	b.WriteString(".cover-badge { display: inline-block; font-size: 8pt; font-weight: 700; text-transform: uppercase; letter-spacing: 1.5px; background: var(--red-600); color: #fff; padding: 4px 14px; border-radius: 4px; margin-bottom: 20px; }\n")
	b.WriteString(".cover h1 { font-size: 32pt; font-weight: 800; line-height: 1.15; color: var(--slate-900); margin-bottom: 8px; white-space: pre-line; }\n")
	b.WriteString(".cover-subtitle { font-size: 13pt; font-weight: 400; color: var(--slate-500); margin-bottom: 32px; }\n")
	b.WriteString(".cover-meta { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 16px 32px; padding-top: 24px; border-top: 2px solid var(--slate-200); margin-bottom: 40px; }\n")
	b.WriteString(".cover-meta-item { }\n")
	b.WriteString(".cover-meta-label { font-size: 7pt; font-weight: 700; text-transform: uppercase; letter-spacing: 0.8px; color: var(--slate-400); margin-bottom: 2px; }\n")
	b.WriteString(".cover-meta-value { font-size: 10pt; font-weight: 600; color: var(--slate-800); }\n")

	// Cover summary box
	b.WriteString(".cover-summary { padding: 16px 20px; border-radius: 8px; margin-top: 20px; }\n")
	b.WriteString(".cover-summary-critical { background: var(--red-50); border: 1px solid var(--red-100); border-left: 4px solid var(--red-500); }\n")
	b.WriteString(".cover-summary-warning { background: var(--amber-50); border: 1px solid var(--amber-100); border-left: 4px solid var(--amber-500); }\n")
	b.WriteString(".cover-summary-healthy { background: var(--green-50); border: 1px solid var(--green-100); border-left: 4px solid var(--green-500); }\n")
	b.WriteString(".cover-summary h3 { font-size: 10pt; font-weight: 700; margin-bottom: 8px; }\n")
	b.WriteString(".cover-summary ul { list-style: disc; padding-left: 20px; font-size: 9pt; }\n")
	b.WriteString(".cover-summary li { margin-bottom: 3px; }\n")

	// Section headings
	b.WriteString("h2.section-heading { font-size: 16pt; font-weight: 800; color: var(--slate-900); padding-bottom: 10px; border-bottom: 2px solid var(--slate-200); margin: 40px 0 20px; page-break-after: avoid; }\n")
	b.WriteString("h2.section-heading .section-num { color: var(--blue-600); margin-right: 8px; }\n")
	b.WriteString("h2.section-heading .sev-badge { font-size: 8pt; font-weight: 700; text-transform: uppercase; padding: 2px 8px; border-radius: 4px; margin-left: 12px; vertical-align: middle; }\n")
	b.WriteString(".sev-badge.sev-critical { background: var(--red-100); color: var(--red-700); }\n")
	b.WriteString(".sev-badge.sev-warning { background: var(--amber-100); color: var(--amber-700); }\n")
	b.WriteString(".sev-badge.sev-info { background: var(--blue-100); color: var(--blue-700); }\n")
	b.WriteString(".sev-badge.sev-ok { background: var(--green-100); color: var(--green-600); }\n")

	// Tables
	b.WriteString(".table-wrap { overflow-x: auto; margin-bottom: 20px; }\n")
	b.WriteString("table { width: 100%; border-collapse: collapse; font-size: 8.5pt; table-layout: auto; }\n")
	b.WriteString("thead th { background: var(--slate-800); color: #fff; font-size: 7pt; font-weight: 700; text-transform: uppercase; letter-spacing: 0.6px; padding: 7px 6px; text-align: left; white-space: nowrap; }\n")
	b.WriteString("thead th:first-child { border-radius: 5px 0 0 0; } thead th:last-child { border-radius: 0 5px 0 0; }\n")
	b.WriteString("tbody td { padding: 5px 6px; border-bottom: 1px solid var(--slate-100); }\n")
	b.WriteString("tbody tr:nth-child(even) { background: var(--slate-50); }\n")
	b.WriteString("tbody tr:hover { background: var(--blue-50); }\n")
	b.WriteString(".table-dense { font-size: 7.5pt; }\n")
	b.WriteString(".table-dense td, .table-dense th { padding: 4px 5px; }\n")
	b.WriteString(".td-truncate { max-width: 130px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }\n")

	// Finding cards
	b.WriteString(".finding-card { border-radius: 8px; padding: 16px 18px; margin-bottom: 16px; page-break-inside: avoid; }\n")
	b.WriteString(".finding-card.sev-critical { border: 1px solid var(--red-200); background: linear-gradient(135deg, var(--red-50), #fff); border-left: 4px solid var(--red-500); }\n")
	b.WriteString(".finding-card.sev-warning { border: 1px solid var(--amber-200); background: linear-gradient(135deg, var(--amber-50), #fff); border-left: 4px solid var(--amber-500); }\n")
	b.WriteString(".finding-card.sev-info { border: 1px solid var(--blue-100); background: linear-gradient(135deg, var(--blue-50), #fff); border-left: 4px solid var(--blue-500); }\n")
	b.WriteString(".finding-card.sev-ok { border: 1px solid var(--green-100); background: linear-gradient(135deg, var(--green-50), #fff); border-left: 4px solid var(--green-500); }\n")
	b.WriteString(".finding-title { font-size: 11pt; font-weight: 700; margin-bottom: 6px; }\n")
	b.WriteString(".finding-desc { font-size: 9pt; color: var(--slate-700); margin-bottom: 10px; }\n")
	b.WriteString(".finding-evidence { font-size: 8pt; color: var(--slate-600); margin-bottom: 8px; }\n")
	b.WriteString(".finding-evidence li { margin-bottom: 2px; }\n")
	b.WriteString(".finding-action { font-size: 8.5pt; font-weight: 600; color: var(--slate-800); padding-top: 8px; border-top: 1px solid var(--slate-200); }\n")

	// Stats grid
	b.WriteString(".stats-grid { display: grid; grid-template-columns: 1fr 1fr 1fr 1fr; gap: 10px; margin-bottom: 24px; }\n")
	b.WriteString(".stat-card { background: var(--slate-50); border: 1px solid var(--slate-200); border-radius: 8px; padding: 14px 16px; text-align: center; }\n")
	b.WriteString(".stat-value { font-size: 18pt; font-weight: 800; line-height: 1.2; }\n")
	b.WriteString(".stat-label { font-size: 7pt; font-weight: 700; text-transform: uppercase; letter-spacing: 0.8px; color: var(--slate-500); margin-top: 4px; }\n")

	// Log blocks
	b.WriteString(".log-block { background: var(--slate-900); color: #e2e8f0; border-radius: 6px; padding: 12px; font-family: 'SF Mono', 'Cascadia Code', 'Fira Code', monospace; font-size: 7.5pt; line-height: 1.7; overflow-x: auto; margin-bottom: 16px; }\n")
	b.WriteString(".log-block .log-error { color: #fca5a5; }\n")
	b.WriteString(".log-block .log-warning { color: #fde68a; }\n")

	// Bar charts
	b.WriteString(".bar-chart { margin-bottom: 20px; }\n")
	b.WriteString(".bar-row { display: flex; align-items: center; margin-bottom: 6px; }\n")
	b.WriteString(".bar-label { width: 90px; font-size: 8pt; font-weight: 600; color: var(--slate-700); flex-shrink: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }\n")
	b.WriteString(".bar-track { flex: 1; height: 14px; background: var(--slate-100); border-radius: 3px; overflow: hidden; }\n")
	b.WriteString(".bar-fill { height: 100%; border-radius: 3px; transition: width 0.3s ease; }\n")
	b.WriteString(".bar-value { width: 50px; font-size: 8pt; font-weight: 600; text-align: right; color: var(--slate-600); flex-shrink: 0; margin-left: 8px; }\n")

	// Value color classes
	b.WriteString(".val-bad { color: var(--red-600); font-weight: 700; }\n")
	b.WriteString(".val-warn { color: var(--amber-600); font-weight: 600; }\n")
	b.WriteString(".val-good { color: var(--green-600); font-weight: 600; }\n")

	// TOC
	b.WriteString(".toc { page-break-after: always; padding-top: 40px; }\n")
	b.WriteString(".toc h2 { font-size: 20pt; font-weight: 800; color: var(--slate-900); margin-bottom: 24px; }\n")
	b.WriteString(".toc-list { list-style: none; padding: 0; }\n")
	b.WriteString(".toc-item { display: flex; align-items: center; padding: 10px 0; border-bottom: 1px solid var(--slate-100); font-size: 10pt; }\n")
	b.WriteString(".toc-num { font-weight: 800; color: var(--blue-600); width: 30px; flex-shrink: 0; }\n")
	b.WriteString(".toc-title { flex: 1; font-weight: 600; color: var(--slate-800); }\n")
	b.WriteString(".toc-badge { margin-left: 8px; }\n")

	// Section page break
	b.WriteString(".section { page-break-before: auto; }\n")
	b.WriteString(".section-break { page-break-before: always; }\n")

	// Action table priority
	b.WriteString(".priority-cell { font-weight: 700; }\n")

	// Print rules
	b.WriteString("@media print {\n")
	b.WriteString("  body { -webkit-print-color-adjust: exact; print-color-adjust: exact; }\n")
	b.WriteString("  .finding-card { break-inside: avoid; }\n")
	b.WriteString("  tbody tr:hover { background: inherit; }\n")
	b.WriteString("}\n")
}

// ─────────────────────────────────────────────────────────────────────
// Cover Page
// ─────────────────────────────────────────────────────────────────────

func writeCoverPage(b *strings.Builder, snap *internal.Snapshot) {
	ts := snap.Timestamp.Format("January 2, 2006 at 15:04 MST")
	hostname := escHTML(snap.System.Hostname)
	if hostname == "" {
		hostname = "Unknown Host"
	}

	b.WriteString("<div class=\"cover\">\n")
	b.WriteString("  <div class=\"cover-badge\">Diagnostic Report</div>\n")
	b.WriteString(fmt.Sprintf("  <h1>%s\nHealth Analysis</h1>\n", hostname))
	b.WriteString(fmt.Sprintf("  <div class=\"cover-subtitle\">Automated diagnostic scan &mdash; %s</div>\n", escHTML(ts)))

	// Metadata grid
	hw := escHTML(snap.System.CPUModel)
	if snap.System.CPUCores > 0 {
		hw += fmt.Sprintf(" (%d cores)", snap.System.CPUCores)
	}
	osInfo := escHTML(snap.System.OS)
	if snap.System.Platform != "" {
		osInfo += " / " + escHTML(snap.System.Platform)
	}
	if snap.System.PlatformVer != "" {
		osInfo += " " + escHTML(snap.System.PlatformVer)
	}

	b.WriteString("  <div class=\"cover-meta\">\n")
	writeCoverMetaItem(b, "Server", hostname)
	writeCoverMetaItem(b, "Date", escHTML(snap.Timestamp.Format("2006-01-02")))
	writeCoverMetaItem(b, "Uptime", escHTML(formatUptime(snap.System.UptimeSecs)))
	writeCoverMetaItem(b, "Hardware", hw)
	writeCoverMetaItem(b, "OS / Platform", osInfo)
	writeCoverMetaItem(b, "Array Status", escHTML(arrayStatus(snap.Findings)))
	b.WriteString("  </div>\n")

	// Summary box
	critFindings := []internal.Finding{}
	warnFindings := []internal.Finding{}
	for _, f := range snap.Findings {
		if f.Severity == internal.SeverityCritical {
			critFindings = append(critFindings, f)
		} else if f.Severity == internal.SeverityWarning {
			warnFindings = append(warnFindings, f)
		}
	}

	if len(critFindings) > 0 {
		b.WriteString("  <div class=\"cover-summary cover-summary-critical\">\n")
		b.WriteString(fmt.Sprintf("    <h3>%d Critical Finding(s) Detected</h3>\n", len(critFindings)))
		b.WriteString("    <ul>\n")
		for _, f := range critFindings {
			b.WriteString(fmt.Sprintf("      <li>%s</li>\n", escHTML(f.Title)))
		}
		b.WriteString("    </ul>\n")
		b.WriteString("  </div>\n")
	} else if len(warnFindings) > 0 {
		b.WriteString("  <div class=\"cover-summary cover-summary-warning\">\n")
		b.WriteString(fmt.Sprintf("    <h3>%d Warning(s) Detected</h3>\n", len(warnFindings)))
		b.WriteString("    <ul>\n")
		for _, f := range warnFindings {
			b.WriteString(fmt.Sprintf("      <li>%s</li>\n", escHTML(f.Title)))
		}
		b.WriteString("    </ul>\n")
		b.WriteString("  </div>\n")
	} else {
		b.WriteString("  <div class=\"cover-summary cover-summary-healthy\">\n")
		b.WriteString("    <h3>System Healthy</h3>\n")
		b.WriteString("    <ul><li>No critical issues or warnings detected. All systems operating normally.</li></ul>\n")
		b.WriteString("  </div>\n")
	}

	b.WriteString("</div>\n")
}

func writeCoverMetaItem(b *strings.Builder, label, value string) {
	b.WriteString("    <div class=\"cover-meta-item\">\n")
	b.WriteString(fmt.Sprintf("      <div class=\"cover-meta-label\">%s</div>\n", escHTML(label)))
	b.WriteString(fmt.Sprintf("      <div class=\"cover-meta-value\">%s</div>\n", value))
	b.WriteString("    </div>\n")
}

// ─────────────────────────────────────────────────────────────────────
// Table of Contents
// ─────────────────────────────────────────────────────────────────────

func writeTOC(b *strings.Builder, snap *internal.Snapshot) {
	type tocEntry struct {
		Num      int
		Title    string
		Severity internal.Severity
		Show     bool
	}

	// Determine highest severity across findings
	highestSev := internal.SeverityOK
	for _, f := range snap.Findings {
		if f.Severity == internal.SeverityCritical {
			highestSev = internal.SeverityCritical
			break
		}
		if f.Severity == internal.SeverityWarning && highestSev != internal.SeverityCritical {
			highestSev = internal.SeverityWarning
		}
	}

	entries := []tocEntry{
		{1, "System Overview", internal.SeverityOK, true},
		{2, "Findings", highestSev, len(snap.Findings) > 0},
		{3, "Drive Health & SMART Analysis", internal.SeverityOK, len(snap.SMART) > 0},
		{4, "Docker & Application Analysis", internal.SeverityOK, snap.Docker.Available},
		{5, "Parity Analysis", internal.SeverityOK, snap.Parity != nil && len(snap.Parity.History) > 0},
		{6, "Recommended Actions", internal.SeverityOK, len(snap.Findings) > 0},
	}

	b.WriteString("<div class=\"toc\">\n")
	b.WriteString("  <h2>Table of Contents</h2>\n")
	b.WriteString("  <ol class=\"toc-list\">\n")
	for _, e := range entries {
		if !e.Show {
			continue
		}
		b.WriteString("    <li class=\"toc-item\">\n")
		b.WriteString(fmt.Sprintf("      <span class=\"toc-num\">%d</span>\n", e.Num))
		b.WriteString(fmt.Sprintf("      <span class=\"toc-title\">%s</span>\n", escHTML(e.Title)))
		if e.Severity != internal.SeverityOK {
			b.WriteString(fmt.Sprintf("      <span class=\"toc-badge\"><span class=\"sev-badge %s\">%s</span></span>\n",
				severityClass(e.Severity), escHTML(severityLabel(e.Severity))))
		}
		b.WriteString("    </li>\n")
	}
	b.WriteString("  </ol>\n")
	b.WriteString("</div>\n")
}

// ─────────────────────────────────────────────────────────────────────
// System Overview
// ─────────────────────────────────────────────────────────────────────

func writeSystemOverview(b *strings.Builder, snap *internal.Snapshot) {
	sys := snap.System

	b.WriteString("<div class=\"section\">\n")
	b.WriteString("  <h2 class=\"section-heading\"><span class=\"section-num\">1</span>System Overview</h2>\n")

	// Stats grid
	b.WriteString("  <div class=\"stats-grid\">\n")
	writeStatCard(b, fmt.Sprintf("%d", sys.CPUCores), "CPU Cores", "")
	writeStatCard(b, fmt.Sprintf("%.1f%%", sys.MemPercent), "Memory Usage", valClass(sys.MemPercent, 80, 95))
	writeStatCard(b, fmt.Sprintf("%.1f%%", sys.IOWait), "I/O Wait", valClass(sys.IOWait, 10, 30))
	writeStatCard(b, formatUptime(sys.UptimeSecs), "Uptime", "")
	b.WriteString("  </div>\n")

	// System components table
	b.WriteString("  <table>\n")
	b.WriteString("    <thead><tr><th>Component</th><th>Details</th><th>Status</th></tr></thead>\n")
	b.WriteString("    <tbody>\n")

	cpuStatus := "Normal"
	cpuClass := "val-good"
	if sys.CPUUsage > 90 {
		cpuStatus = "High"
		cpuClass = "val-bad"
	} else if sys.CPUUsage > 70 {
		cpuStatus = "Elevated"
		cpuClass = "val-warn"
	}
	writeComponentRow(b, "CPU", fmt.Sprintf("%s — %.1f%% usage", escHTML(sys.CPUModel), sys.CPUUsage), cpuStatus, cpuClass)

	memDetail := fmt.Sprintf("%d MB / %d MB (%.1f%%)", sys.MemUsedMB, sys.MemTotalMB, sys.MemPercent)
	memStatus := "Normal"
	memClass := "val-good"
	if sys.MemPercent > 95 {
		memStatus = "Critical"
		memClass = "val-bad"
	} else if sys.MemPercent > 80 {
		memStatus = "Elevated"
		memClass = "val-warn"
	}
	writeComponentRow(b, "Memory", memDetail, memStatus, memClass)

	loadDetail := fmt.Sprintf("1m: %.2f / 5m: %.2f / 15m: %.2f", sys.LoadAvg1, sys.LoadAvg5, sys.LoadAvg15)
	loadStatus := "Normal"
	loadClass := "val-good"
	loadThresh := float64(sys.CPUCores)
	if loadThresh == 0 {
		loadThresh = 1
	}
	if sys.LoadAvg1 > loadThresh*2 {
		loadStatus = "Critical"
		loadClass = "val-bad"
	} else if sys.LoadAvg1 > loadThresh {
		loadStatus = "Elevated"
		loadClass = "val-warn"
	}
	writeComponentRow(b, "Load Average", loadDetail, loadStatus, loadClass)

	ioStatus := "Normal"
	ioClass := "val-good"
	if sys.IOWait > 30 {
		ioStatus = "Critical"
		ioClass = "val-bad"
	} else if sys.IOWait > 10 {
		ioStatus = "Elevated"
		ioClass = "val-warn"
	}
	writeComponentRow(b, "I/O Wait", fmt.Sprintf("%.1f%%", sys.IOWait), ioStatus, ioClass)

	// Network interfaces
	netDetail := ""
	for i, iface := range snap.Network.Interfaces {
		if i > 0 {
			netDetail += ", "
		}
		netDetail += escHTML(iface.Name)
		if iface.Speed != "" {
			netDetail += " (" + escHTML(iface.Speed) + ")"
		}
	}
	if netDetail == "" {
		netDetail = "No interfaces detected"
	}
	writeComponentRow(b, "Network", netDetail, "Connected", "val-good")

	writeComponentRow(b, "Uptime", formatUptime(sys.UptimeSecs), "", "")
	writeComponentRow(b, "OS", escHTML(sys.OS)+" "+escHTML(sys.Kernel), "", "")
	if sys.Motherboard != "" {
		writeComponentRow(b, "Motherboard", escHTML(sys.Motherboard), "", "")
	}

	b.WriteString("    </tbody>\n")
	b.WriteString("  </table>\n")

	// Disk space bar chart
	if len(snap.Disks) > 0 {
		b.WriteString("  <h3 style=\"font-size: 11pt; font-weight: 700; margin: 20px 0 12px; color: var(--slate-800);\">Disk Space Utilisation</h3>\n")
		b.WriteString("  <div class=\"bar-chart\">\n")
		for _, d := range snap.Disks {
			label := escHTML(d.Label)
			if label == "" {
				label = escHTML(d.MountPoint)
			}
			if label == "" {
				label = escHTML(d.Device)
			}
			color := barColor(d.UsedPct)
			pctStr := fmt.Sprintf("%.1f%%", d.UsedPct)
			widthPct := d.UsedPct
			if widthPct > 100 {
				widthPct = 100
			}
			b.WriteString("    <div class=\"bar-row\">\n")
			b.WriteString(fmt.Sprintf("      <span class=\"bar-label\">%s</span>\n", label))
			b.WriteString(fmt.Sprintf("      <div class=\"bar-track\"><div class=\"bar-fill\" style=\"width: %.1f%%; background: %s;\"></div></div>\n", widthPct, color))
			b.WriteString(fmt.Sprintf("      <span class=\"bar-value\">%s</span>\n", pctStr))
			b.WriteString("    </div>\n")
		}
		b.WriteString("  </div>\n")
	}

	b.WriteString("</div>\n")
}

func writeStatCard(b *strings.Builder, value, label, valueClass string) {
	cls := ""
	if valueClass != "" {
		cls = " class=\"" + valueClass + "\""
	}
	b.WriteString("    <div class=\"stat-card\">\n")
	b.WriteString(fmt.Sprintf("      <div class=\"stat-value\"%s>%s</div>\n", cls, escHTML(value)))
	b.WriteString(fmt.Sprintf("      <div class=\"stat-label\">%s</div>\n", escHTML(label)))
	b.WriteString("    </div>\n")
}

func writeComponentRow(b *strings.Builder, component, details, status, statusClass string) {
	b.WriteString("      <tr>\n")
	b.WriteString(fmt.Sprintf("        <td><strong>%s</strong></td>\n", escHTML(component)))
	b.WriteString(fmt.Sprintf("        <td>%s</td>\n", details))
	if status != "" {
		b.WriteString(fmt.Sprintf("        <td class=\"%s\">%s</td>\n", statusClass, escHTML(status)))
	} else {
		b.WriteString("        <td>&mdash;</td>\n")
	}
	b.WriteString("      </tr>\n")
}

// ─────────────────────────────────────────────────────────────────────
// Findings
// ─────────────────────────────────────────────────────────────────────

func writeFindings(b *strings.Builder, snap *internal.Snapshot) {
	if len(snap.Findings) == 0 {
		return
	}

	sorted := groupFindings(snap.Findings)

	sectionNum := 2
	b.WriteString("<div class=\"section section-break\">\n")

	// Determine highest severity badge
	highestSev := internal.SeverityOK
	for _, f := range sorted {
		if f.Severity == internal.SeverityCritical {
			highestSev = internal.SeverityCritical
			break
		}
		if f.Severity == internal.SeverityWarning {
			highestSev = internal.SeverityWarning
		}
		if f.Severity == internal.SeverityInfo && highestSev == internal.SeverityOK {
			highestSev = internal.SeverityInfo
		}
	}

	b.WriteString(fmt.Sprintf("  <h2 class=\"section-heading\"><span class=\"section-num\">%d</span>Findings", sectionNum))
	if highestSev != internal.SeverityOK {
		b.WriteString(fmt.Sprintf(" <span class=\"sev-badge %s\">%s</span>",
			severityClass(highestSev), escHTML(severityLabel(highestSev))))
	}
	b.WriteString("</h2>\n")

	for _, f := range sorted {
		cls := severityClass(f.Severity)
		b.WriteString(fmt.Sprintf("  <div class=\"finding-card %s\">\n", cls))
		b.WriteString(fmt.Sprintf("    <div class=\"finding-title\">%s <span class=\"sev-badge %s\">%s</span></div>\n",
			escHTML(f.Title), cls, escHTML(severityLabel(f.Severity))))
		b.WriteString(fmt.Sprintf("    <div class=\"finding-desc\">%s</div>\n", escHTML(f.Description)))

		if len(f.Evidence) > 0 {
			b.WriteString("    <div class=\"finding-evidence\"><strong>Evidence:</strong><ul>\n")
			for _, ev := range f.Evidence {
				b.WriteString(fmt.Sprintf("      <li>%s</li>\n", escHTML(ev)))
			}
			b.WriteString("    </ul></div>\n")
		}

		if f.Action != "" {
			b.WriteString(fmt.Sprintf("    <div class=\"finding-action\">Recommended: %s</div>\n", escHTML(f.Action)))
		}
		b.WriteString("  </div>\n")
	}

	b.WriteString("</div>\n")
}

// ─────────────────────────────────────────────────────────────────────
// SMART Analysis
// ─────────────────────────────────────────────────────────────────────

func writeSMART(b *strings.Builder, snap *internal.Snapshot) {
	if len(snap.SMART) == 0 {
		return
	}

	b.WriteString("<div class=\"section section-break\">\n")
	b.WriteString("  <h2 class=\"section-heading\"><span class=\"section-num\">3</span>Drive Health &amp; SMART Analysis</h2>\n")

	b.WriteString("  <div class=\"table-wrap\"><table class=\"table-dense\">\n")
	b.WriteString("    <thead><tr>")
	b.WriteString("<th>Device</th>")
	b.WriteString("<th>Model</th>")
	b.WriteString("<th>Size</th>")
	b.WriteString("<th>Health</th>")
	b.WriteString("<th>Temp</th>")
	b.WriteString("<th>Realloc</th>")
	b.WriteString("<th>Pending</th>")
	b.WriteString("<th>CRC</th>")
	b.WriteString("<th>Hours</th>")
	b.WriteString("<th>Age</th>")
	b.WriteString("</tr></thead>\n")
	b.WriteString("    <tbody>\n")

	for _, s := range snap.SMART {
		healthStr := healthPassedStr(s.HealthPassed)
		hClass := healthClass(s.HealthPassed)

		tempClass := "val-good"
		if s.Temperature > 55 {
			tempClass = "val-bad"
		} else if s.Temperature > 45 {
			tempClass = "val-warn"
		}

		reallocClass := "val-good"
		if s.Reallocated > 0 {
			reallocClass = "val-bad"
		}

		pendingClass := "val-good"
		if s.Pending > 0 {
			pendingClass = "val-warn"
		}

		crcClass := "val-good"
		if s.UDMACRC > 0 {
			crcClass = "val-warn"
		}

		// Shorten size display
		sizeStr := fmt.Sprintf("%.0fG", s.SizeGB)
		if s.SizeGB >= 1000 {
			sizeStr = fmt.Sprintf("%.0fT", s.SizeGB/1000)
		}

		// Shorten device path
		dev := s.Device
		if len(dev) > 8 {
			dev = dev[5:] // strip /dev/
		}

		b.WriteString("      <tr>")
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(dev)))
		b.WriteString(fmt.Sprintf("<td class=\"td-truncate\" title=\"%s\">%s</td>", escHTML(s.Model), escHTML(s.Model)))
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(sizeStr)))
		b.WriteString(fmt.Sprintf("<td class=\"%s\">%s</td>", hClass, escHTML(healthStr)))
		b.WriteString(fmt.Sprintf("<td class=\"%s\">%d&deg;C</td>", tempClass, s.Temperature))
		b.WriteString(fmt.Sprintf("<td class=\"%s\">%d</td>", reallocClass, s.Reallocated))
		b.WriteString(fmt.Sprintf("<td class=\"%s\">%d</td>", pendingClass, s.Pending))
		b.WriteString(fmt.Sprintf("<td class=\"%s\">%d</td>", crcClass, s.UDMACRC))
		b.WriteString(fmt.Sprintf("<td>%s</td>", fmtHours(s.PowerOnHours)))
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(formatAge(s.PowerOnHours))))
		b.WriteString("</tr>\n")
	}

	b.WriteString("    </tbody>\n")
	b.WriteString("  </table></div>\n")
	b.WriteString("</div>\n")
}

// ─────────────────────────────────────────────────────────────────────
// Docker & Application Analysis
// ─────────────────────────────────────────────────────────────────────

func writeDocker(b *strings.Builder, snap *internal.Snapshot) {
	if !snap.Docker.Available {
		return
	}

	b.WriteString("<div class=\"section section-break\">\n")
	b.WriteString("  <h2 class=\"section-heading\"><span class=\"section-num\">4</span>Docker &amp; Application Analysis</h2>\n")

	// Container table
	if len(snap.Docker.Containers) > 0 {
		b.WriteString("  <h3 style=\"font-size: 11pt; font-weight: 700; margin: 0 0 12px; color: var(--slate-800);\">Containers</h3>\n")
		b.WriteString("  <table>\n")
		b.WriteString("    <thead><tr><th>Name</th><th>Image</th><th>Status</th><th>CPU %</th><th>Memory (MB)</th></tr></thead>\n")
		b.WriteString("    <tbody>\n")
		for _, c := range snap.Docker.Containers {
			stateClass := "val-good"
			if c.State != "running" {
				stateClass = "val-warn"
			}
			b.WriteString("      <tr>")
			b.WriteString(fmt.Sprintf("<td><strong>%s</strong></td>", escHTML(c.Name)))
			b.WriteString(fmt.Sprintf("<td style=\"font-size: 7.5pt;\">%s</td>", escHTML(c.Image)))
			b.WriteString(fmt.Sprintf("<td class=\"%s\">%s</td>", stateClass, escHTML(c.Status)))
			b.WriteString(fmt.Sprintf("<td>%.1f</td>", c.CPU))
			b.WriteString(fmt.Sprintf("<td>%.1f</td>", c.MemMB))
			b.WriteString("</tr>\n")
		}
		b.WriteString("    </tbody>\n")
		b.WriteString("  </table>\n")
	}

	// Top processes table
	if len(snap.System.TopProcesses) > 0 {
		b.WriteString("  <h3 style=\"font-size: 11pt; font-weight: 700; margin: 20px 0 12px; color: var(--slate-800);\">Top Processes</h3>\n")
		b.WriteString("  <table>\n")
		b.WriteString("    <thead><tr><th>Process</th><th>CPU %</th><th>Mem %</th><th>User</th></tr></thead>\n")
		b.WriteString("    <tbody>\n")
		for _, p := range snap.System.TopProcesses {
			b.WriteString("      <tr>")
			b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(p.Command)))
			b.WriteString(fmt.Sprintf("<td>%.1f</td>", p.CPU))
			b.WriteString(fmt.Sprintf("<td>%.1f</td>", p.Mem))
			b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(p.User)))
			b.WriteString("</tr>\n")
		}
		b.WriteString("    </tbody>\n")
		b.WriteString("  </table>\n")
	}

	b.WriteString("</div>\n")
}

// ─────────────────────────────────────────────────────────────────────
// Parity Analysis
// ─────────────────────────────────────────────────────────────────────

func writeParity(b *strings.Builder, snap *internal.Snapshot) {
	if snap.Parity == nil || len(snap.Parity.History) == 0 {
		return
	}

	b.WriteString("<div class=\"section section-break\">\n")
	b.WriteString("  <h2 class=\"section-heading\"><span class=\"section-num\">5</span>Parity Analysis</h2>\n")

	b.WriteString("  <table>\n")
	b.WriteString("    <thead><tr><th>Date</th><th>Duration</th><th>Speed (MB/s)</th><th>Errors</th><th>Action</th></tr></thead>\n")
	b.WriteString("    <tbody>\n")
	for _, p := range snap.Parity.History {
		errClass := "val-good"
		if p.Errors > 0 {
			errClass = "val-bad"
		}
		b.WriteString("      <tr>")
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(p.Date)))
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(fmtDurationSecs(p.Duration))))
		b.WriteString(fmt.Sprintf("<td>%.1f</td>", p.SpeedMBs))
		b.WriteString(fmt.Sprintf("<td class=\"%s\">%d</td>", errClass, p.Errors))
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(p.Action)))
		b.WriteString("</tr>\n")
	}
	b.WriteString("    </tbody>\n")
	b.WriteString("  </table>\n")

	b.WriteString("</div>\n")
}

// ─────────────────────────────────────────────────────────────────────
// Recommended Actions
// ─────────────────────────────────────────────────────────────────────

func writeActions(b *strings.Builder, snap *internal.Snapshot) {
	// Collect findings that have actions
	var actionFindings []internal.Finding
	for _, f := range snap.Findings {
		if f.Action != "" {
			actionFindings = append(actionFindings, f)
		}
	}
	if len(actionFindings) == 0 {
		return
	}

	// Sort by severity (critical first)
	actionFindings = groupFindings(actionFindings)

	b.WriteString("<div class=\"section section-break\">\n")
	b.WriteString("  <h2 class=\"section-heading\"><span class=\"section-num\">6</span>Recommended Actions</h2>\n")

	b.WriteString("  <table>\n")
	b.WriteString("    <thead><tr><th>Action</th><th>Priority</th><th>Impact</th><th>Est. Cost</th></tr></thead>\n")
	b.WriteString("    <tbody>\n")
	for _, f := range actionFindings {
		priority := f.Priority
		if priority == "" {
			// Derive from severity
			switch f.Severity {
			case internal.SeverityCritical:
				priority = "Immediate"
			case internal.SeverityWarning:
				priority = "Short-term"
			default:
				priority = "Medium-term"
			}
		}
		impact := f.Impact
		if impact == "" {
			impact = "\u2014"
		}
		cost := f.Cost
		if cost == "" {
			cost = "\u2014"
		}
		pColor := priorityColor(priority)
		b.WriteString("      <tr>")
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(f.Action)))
		b.WriteString(fmt.Sprintf("<td class=\"priority-cell\" style=\"color: %s;\">%s</td>", pColor, escHTML(priority)))
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(impact)))
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(cost)))
		b.WriteString("</tr>\n")
	}
	b.WriteString("    </tbody>\n")
	b.WriteString("  </table>\n")

	b.WriteString("</div>\n")
}

// ─────────────────────────────────────────────────────────────────────
// Footer
// ─────────────────────────────────────────────────────────────────────

func writeFooter(b *strings.Builder, snap *internal.Snapshot) {
	b.WriteString("<div class=\"section\" style=\"margin-top: 40px; padding-top: 20px; border-top: 2px solid var(--slate-200);\">\n")
	b.WriteString("  <p style=\"font-size: 8pt; color: var(--slate-500); font-style: italic; margin-bottom: 12px;\">")
	b.WriteString("This report was generated from an automated diagnostic scan. No changes were made to the system.")
	b.WriteString("</p>\n")

	// Data sources
	sources := []string{"System metrics (CPU, memory, load, I/O)"}
	if len(snap.Disks) > 0 {
		sources = append(sources, "Disk space utilisation")
	}
	if len(snap.SMART) > 0 {
		sources = append(sources, "S.M.A.R.T. drive health data")
	}
	if snap.Docker.Available {
		sources = append(sources, "Docker container metrics")
	}
	if len(snap.Network.Interfaces) > 0 {
		sources = append(sources, "Network interface status")
	}
	if snap.Parity != nil && len(snap.Parity.History) > 0 {
		sources = append(sources, "Parity check history")
	}
	if len(snap.Logs.DmesgErrors) > 0 || len(snap.Logs.SyslogErrors) > 0 {
		sources = append(sources, "System log analysis (dmesg, syslog)")
	}

	b.WriteString("  <p style=\"font-size: 7.5pt; color: var(--slate-400); margin-bottom: 4px;\"><strong>Data Sources:</strong></p>\n")
	b.WriteString("  <ul style=\"font-size: 7.5pt; color: var(--slate-400); padding-left: 18px; margin-bottom: 12px;\">\n")
	for _, s := range sources {
		b.WriteString(fmt.Sprintf("    <li>%s</li>\n", escHTML(s)))
	}
	b.WriteString("  </ul>\n")

	b.WriteString(fmt.Sprintf("  <p style=\"font-size: 7pt; color: var(--slate-400);\">Generated by NAS Doctor on %s. Scan duration: %.1fs.</p>\n",
		escHTML(snap.Timestamp.Format(time.RFC3339)), snap.Duration))

	b.WriteString("</div>\n")
}
