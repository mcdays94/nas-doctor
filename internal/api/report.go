package api

import (
	"fmt"
	"html"
	"sort"
	"strings"

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
	b.WriteString("@page { size: A4; margin: 22mm 20mm 28mm 20mm; }\n")
	b.WriteString("*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }\n")

	// Clean theme palette — matching app's Clean theme + CF Workers design system
	b.WriteString(":root {\n")
	b.WriteString("  --text: #171717; --text-muted: #4d4d4d; --text-subtle: #808080; --text-faint: #b3b3b3;\n")
	b.WriteString("  --bg: #ffffff; --bg-card: #ffffff; --bg-hover: #fafafa;\n")
	b.WriteString("  --border: rgba(0,0,0,0.08); --border-solid: #e5e5e5;\n")
	b.WriteString("  --red: #dc2626; --red-bg: rgba(220,38,38,0.06); --red-border: rgba(220,38,38,0.15);\n")
	b.WriteString("  --amber: #d97706; --amber-bg: rgba(217,119,6,0.06); --amber-border: rgba(217,119,6,0.15);\n")
	b.WriteString("  --green: #16a34a; --green-bg: rgba(22,163,74,0.06); --green-border: rgba(22,163,74,0.15);\n")
	b.WriteString("  --blue: #0072f5; --blue-bg: rgba(0,114,245,0.06); --blue-border: rgba(0,114,245,0.15);\n")
	b.WriteString("}\n")

	// Body — Clean theme
	b.WriteString("body { font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; font-size: 9.5pt; color: var(--text); background: var(--bg); line-height: 1.65; -webkit-font-smoothing: antialiased; }\n")

	// Page numbers (print)
	b.WriteString("@page { @bottom-center { font-family: 'Inter', sans-serif; font-size: 7.5pt; color: var(--text-subtle); content: 'Page ' counter(page) ' of ' counter(pages); } }\n")

	// Dashed separator utility (CF Workers style)
	b.WriteString(".dashed-sep { height: 1px; background-image: linear-gradient(to right, var(--border-solid) 50%, transparent 50%); background-size: 10px 1px; background-repeat: repeat-x; }\n")

	// ── Cover page ──
	b.WriteString(".cover { page-break-after: always; padding: 80px 0 40px; }\n")
	b.WriteString(".cover-badge { display: inline-block; font-size: 8pt; font-weight: 600; text-transform: uppercase; letter-spacing: 1.5px; background: var(--text); color: #fff; padding: 5px 16px; border-radius: 6px; margin-bottom: 28px; }\n")
	b.WriteString(".cover h1 { font-size: 34pt; font-weight: 500; line-height: 1.15; color: var(--text); margin-bottom: 12px; white-space: pre-line; letter-spacing: -0.5px; }\n")
	b.WriteString(".cover-subtitle { font-size: 12pt; font-weight: 400; color: var(--text-muted); margin-bottom: 40px; line-height: 1.5; }\n")
	// Dashed top border on meta grid (CF Workers style)
	b.WriteString(".cover-meta { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 18px 40px; padding-top: 28px; margin-bottom: 48px; position: relative; }\n")
	b.WriteString(".cover-meta::before { content: ''; position: absolute; top: 0; left: 0; right: 0; height: 1px; background-image: linear-gradient(to right, var(--border-solid) 50%, transparent 50%); background-size: 10px 1px; background-repeat: repeat-x; }\n")
	b.WriteString(".cover-meta-item { }\n")
	b.WriteString(".cover-meta-label { font-size: 7pt; font-weight: 600; text-transform: uppercase; letter-spacing: 1px; color: var(--text-subtle); margin-bottom: 4px; }\n")
	b.WriteString(".cover-meta-value { font-size: 10.5pt; font-weight: 500; color: var(--text); }\n")

	// Cover summary box — Clean theme card style with severity left border
	b.WriteString(".cover-summary { padding: 20px 24px; border-radius: 8px; margin-top: 24px; position: relative; }\n")
	b.WriteString(".cover-summary-critical { background: var(--bg); box-shadow: 0 0 0 1px var(--red-border); border-left: 4px solid var(--red); }\n")
	b.WriteString(".cover-summary-warning { background: var(--bg); box-shadow: 0 0 0 1px var(--amber-border); border-left: 4px solid var(--amber); }\n")
	b.WriteString(".cover-summary-healthy { background: var(--bg); box-shadow: 0 0 0 1px var(--green-border); border-left: 4px solid var(--green); }\n")
	b.WriteString(".cover-summary-critical h3 { color: var(--red); }\n")
	b.WriteString(".cover-summary-warning h3 { color: var(--amber); }\n")
	b.WriteString(".cover-summary-healthy h3 { color: var(--green); }\n")
	b.WriteString(".cover-summary h3 { font-size: 11pt; font-weight: 600; margin-bottom: 10px; }\n")
	b.WriteString(".cover-summary ul { list-style: none; padding-left: 0; font-size: 9.5pt; line-height: 1.7; }\n")
	b.WriteString(".cover-summary li { margin-bottom: 4px; padding-left: 16px; position: relative; color: var(--text-muted); }\n")
	b.WriteString(".cover-summary-critical li::before { content: ''; position: absolute; left: 0; top: 8px; width: 6px; height: 6px; border-radius: 50%; background: var(--red); }\n")
	b.WriteString(".cover-summary-warning li::before { content: ''; position: absolute; left: 0; top: 8px; width: 6px; height: 6px; border-radius: 50%; background: var(--amber); }\n")
	b.WriteString(".cover-summary-healthy li::before { content: ''; position: absolute; left: 0; top: 8px; width: 6px; height: 6px; border-radius: 50%; background: var(--green); }\n")

	// ── Section headings — Clean theme: lighter weight, dashed bottom border ──
	b.WriteString("h2.section-heading { font-size: 18pt; font-weight: 500; color: var(--text); padding-bottom: 14px; margin: 44px 0 24px; page-break-after: avoid; letter-spacing: -0.3px; position: relative; }\n")
	b.WriteString("h2.section-heading::after { content: ''; position: absolute; bottom: 0; left: 0; right: 0; height: 1px; background-image: linear-gradient(to right, var(--border-solid) 50%, transparent 50%); background-size: 10px 1px; background-repeat: repeat-x; }\n")
	b.WriteString("h2.section-heading .section-num { color: var(--text); margin-right: 6px; font-weight: 600; }\n")
	// Severity badges — pill style matching Clean theme
	b.WriteString("h2.section-heading .sev-badge { font-size: 7.5pt; font-weight: 600; text-transform: uppercase; padding: 2px 10px; border-radius: 9999px; margin-left: 14px; vertical-align: middle; letter-spacing: 0.4px; }\n")
	b.WriteString(".sev-badge.sev-critical { background: var(--red-bg); color: var(--red); }\n")
	b.WriteString(".sev-badge.sev-warning { background: var(--amber-bg); color: var(--amber); }\n")
	b.WriteString(".sev-badge.sev-info { background: var(--blue-bg); color: var(--blue); }\n")
	b.WriteString(".sev-badge.sev-ok { background: var(--green-bg); color: var(--green); }\n")

	// Sub-headings
	b.WriteString("h3.sub-heading { font-size: 12pt; font-weight: 600; color: var(--text); margin: 24px 0 14px; }\n")

	// ── Tables — Clean theme style: light header, box-shadow border, rounded ──
	b.WriteString(".table-wrap { margin-bottom: 24px; border-radius: 8px; overflow: hidden; box-shadow: 0 0 0 1px var(--border); }\n")
	b.WriteString("table { width: 100%; border-collapse: collapse; font-size: 9pt; table-layout: auto; }\n")
	b.WriteString("thead th { background: var(--bg-hover); color: var(--text-subtle); font-size: 7.5pt; font-weight: 500; text-transform: uppercase; letter-spacing: 0.5px; padding: 10px 12px; text-align: left; white-space: nowrap; border-bottom: 1px solid var(--border); }\n")
	b.WriteString("tbody td { padding: 10px 12px; border-bottom: 1px solid rgba(0,0,0,0.04); font-size: 9pt; color: var(--text); }\n")
	b.WriteString("tbody tr:last-child td { border-bottom: none; }\n")
	b.WriteString(".table-dense { font-size: 8pt; }\n")
	b.WriteString(".table-dense td, .table-dense th { padding: 8px 10px; }\n")
	b.WriteString(".td-truncate { max-width: 160px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }\n")

	// ── Finding cards — Clean theme: white bg, box-shadow outline, corner brackets, severity left border ──
	b.WriteString(".finding-card { border-radius: 8px; padding: 18px 22px; margin-bottom: 16px; page-break-inside: avoid; background: var(--bg); box-shadow: 0 0 0 1px var(--border); position: relative; }\n")
	b.WriteString(".finding-card.sev-critical { border-left: 4px solid var(--red); }\n")
	b.WriteString(".finding-card.sev-warning { border-left: 4px solid var(--amber); }\n")
	b.WriteString(".finding-card.sev-info { border-left: 4px solid var(--blue); }\n")
	b.WriteString(".finding-card.sev-ok { border-left: 4px solid var(--green); }\n")
	// Corner brackets (CF Workers / Clean theme decorative element)
	b.WriteString(".finding-card::before, .finding-card::after { content: ''; position: absolute; width: 7px; height: 7px; border-radius: 1.5px; background: var(--bg-hover); border: 1px solid var(--border-solid); }\n")
	b.WriteString(".finding-card::before { top: -3px; right: -3px; }\n")
	b.WriteString(".finding-card::after { bottom: -3px; right: -3px; }\n")
	b.WriteString(".finding-title { font-size: 11pt; font-weight: 600; margin-bottom: 6px; color: var(--text); }\n")
	b.WriteString(".finding-desc { font-size: 9.5pt; color: var(--text-muted); margin-bottom: 12px; line-height: 1.6; }\n")
	b.WriteString(".finding-evidence { font-size: 8.5pt; color: var(--text-subtle); margin-bottom: 10px; }\n")
	b.WriteString(".finding-evidence ul { padding-left: 18px; }\n")
	b.WriteString(".finding-evidence li { margin-bottom: 3px; }\n")
	b.WriteString(".finding-action { font-size: 9pt; font-weight: 500; color: var(--text-muted); padding-top: 10px; border-top: 1px solid rgba(0,0,0,0.06); }\n")

	// ── Stats grid — Clean theme card style ──
	b.WriteString(".stats-grid { display: grid; grid-template-columns: 1fr 1fr 1fr 1fr; gap: 12px; margin-bottom: 28px; }\n")
	b.WriteString(".stat-card { background: var(--bg); border-radius: 10px; padding: 18px 16px; text-align: center; box-shadow: 0 0 0 1px var(--border); }\n")
	b.WriteString(".stat-value { font-size: 20pt; font-weight: 600; line-height: 1.2; color: var(--text); }\n")
	b.WriteString(".stat-label { font-size: 7pt; font-weight: 500; text-transform: uppercase; letter-spacing: 0.8px; color: var(--text-subtle); margin-top: 6px; }\n")

	// Log blocks
	b.WriteString(".log-block { background: #1a1a1a; color: #e0e0e0; border-radius: 8px; padding: 14px 16px; font-family: 'SF Mono', 'Cascadia Code', 'Fira Code', monospace; font-size: 7.5pt; line-height: 1.8; overflow-x: auto; margin-bottom: 18px; }\n")
	b.WriteString(".log-block .log-error { color: #fca5a5; }\n")
	b.WriteString(".log-block .log-warning { color: #fde68a; }\n")

	// ── Bar charts — Clean style with rounded pill bars ──
	b.WriteString(".bar-chart { margin-bottom: 24px; }\n")
	b.WriteString(".bar-row { display: flex; align-items: center; margin-bottom: 8px; }\n")
	b.WriteString(".bar-label { width: 100px; font-size: 8.5pt; font-weight: 500; color: var(--text); flex-shrink: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }\n")
	b.WriteString(".bar-track { flex: 1; height: 8px; background: #ebebeb; border-radius: 9999px; overflow: hidden; }\n")
	b.WriteString(".bar-fill { height: 100%; border-radius: 9999px; }\n")
	b.WriteString(".bar-value { width: 50px; font-size: 8.5pt; font-weight: 600; text-align: right; color: var(--text-muted); flex-shrink: 0; margin-left: 10px; }\n")

	// Value color classes
	b.WriteString(".val-bad { color: var(--red); font-weight: 600; }\n")
	b.WriteString(".val-warn { color: var(--amber); font-weight: 500; }\n")
	b.WriteString(".val-good { color: var(--green); font-weight: 500; }\n")

	// ── TOC — dashed separators (CF Workers style) ──
	b.WriteString(".toc { page-break-after: always; padding-top: 48px; }\n")
	b.WriteString(".toc h2 { font-size: 22pt; font-weight: 500; color: var(--text); margin-bottom: 28px; padding-bottom: 14px; position: relative; }\n")
	b.WriteString(".toc h2::after { content: ''; position: absolute; bottom: 0; left: 0; right: 0; height: 1px; background-image: linear-gradient(to right, var(--border-solid) 50%, transparent 50%); background-size: 10px 1px; background-repeat: repeat-x; }\n")
	b.WriteString(".toc-list { list-style: none; padding: 0; }\n")
	b.WriteString(".toc-item { display: flex; align-items: center; padding: 14px 0; font-size: 10.5pt; position: relative; }\n")
	b.WriteString(".toc-item + .toc-item::before { content: ''; position: absolute; top: 0; left: 0; right: 0; height: 1px; background-image: linear-gradient(to right, var(--border-solid) 50%, transparent 50%); background-size: 10px 1px; background-repeat: repeat-x; }\n")
	b.WriteString(".toc-num { font-weight: 500; color: var(--text); width: 30px; flex-shrink: 0; }\n")
	b.WriteString(".toc-title { flex: 1; font-weight: 500; color: var(--text-muted); }\n")
	b.WriteString(".toc-badge { margin-left: 10px; }\n")

	// Section page break
	b.WriteString(".section { page-break-before: auto; }\n")
	b.WriteString(".section-break { page-break-before: always; }\n")

	// Action table priority
	b.WriteString(".priority-cell { font-weight: 600; }\n")

	// Callout box (CF Workers blockquote style)
	b.WriteString(".callout { background: var(--bg); border-left: 3px solid var(--text); border-radius: 0 8px 8px 0; padding: 14px 18px; margin: 16px 0; font-size: 9pt; color: var(--text-muted); line-height: 1.6; box-shadow: 0 0 0 1px var(--border); }\n")
	b.WriteString(".callout-warn { border-left-color: var(--amber); }\n")
	b.WriteString(".callout-danger { border-left-color: var(--red); }\n")

	// Print rules
	b.WriteString("@media print {\n")
	b.WriteString("  body { -webkit-print-color-adjust: exact; print-color-adjust: exact; }\n")
	b.WriteString("  .finding-card { break-inside: avoid; }\n")
	b.WriteString("  .table-wrap { break-inside: avoid; }\n")
	b.WriteString("  .stats-grid { break-inside: avoid; }\n")
	b.WriteString("  .section-break { break-before: page; }\n")
	b.WriteString("}\n")
}

// ─────────────────────────────────────────────────────────────────────
// Cover Page
// ─────────────────────────────────────────────────────────────────────

func writeCoverPage(b *strings.Builder, snap *internal.Snapshot) {
	ts := snap.Timestamp.Format("2 January 2006")
	hostname := escHTML(snap.System.Hostname)
	if hostname == "" {
		hostname = "Unknown Host"
	}

	// Derive a subtitle from findings
	subtitle := "Automated diagnostic scan and health analysis."
	critCount := countSeverity(snap.Findings, internal.SeverityCritical)
	warnCount := countSeverity(snap.Findings, internal.SeverityWarning)
	if critCount > 0 || warnCount > 0 {
		subtitle = fmt.Sprintf("Diagnostic scan identified %d critical and %d warning findings requiring attention.", critCount, warnCount)
	}

	b.WriteString("<div class=\"cover\">\n")
	b.WriteString("  <div class=\"cover-badge\">Diagnostic Report</div>\n")
	b.WriteString(fmt.Sprintf("  <h1>%s\nHealth Analysis</h1>\n", hostname))
	b.WriteString(fmt.Sprintf("  <div class=\"cover-subtitle\">%s</div>\n", escHTML(subtitle)))

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
	writeCoverMetaItem(b, "Date", escHTML(ts))
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
	b.WriteString("  <h2>Contents</h2>\n")
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
	b.WriteString("  <div class=\"table-wrap\"><table>\n")
	b.WriteString("    <thead><tr><th>Component</th><th>Detail</th><th>Status</th></tr></thead>\n")
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
	b.WriteString("  </table></div>\n")

	// Disk space bar chart
	if len(snap.Disks) > 0 {
		b.WriteString("  <h3 class=\"sub-heading\">Disk Space Utilisation</h3>\n")
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

	b.WriteString("  <div class=\"table-wrap\"><table>\n")
	b.WriteString("    <thead><tr>")
	b.WriteString("<th>Dev</th>")
	b.WriteString("<th>Disk</th>")
	b.WriteString("<th>Model</th>")
	b.WriteString("<th>Size</th>")
	b.WriteString("<th>Hours</th>")
	b.WriteString("<th>Age</th>")
	b.WriteString("<th>Health</th>")
	b.WriteString("<th>Concerns</th>")
	b.WriteString("</tr></thead>\n")
	b.WriteString("    <tbody>\n")

	for _, s := range snap.SMART {
		healthStr := healthPassedStr(s.HealthPassed)
		hClass := healthClass(s.HealthPassed)

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

		// Disk slot
		slot := s.ArraySlot
		if slot == "" {
			slot = "\u2014"
		}

		// Build concerns string
		var concerns []string
		if s.UDMACRC > 0 {
			concerns = append(concerns, fmt.Sprintf("%d CRC error(s)", s.UDMACRC))
		}
		if s.CommandTimeout > 5 {
			concerns = append(concerns, fmt.Sprintf("%d cmd timeouts", s.CommandTimeout))
		}
		if s.Temperature > 55 {
			concerns = append(concerns, fmt.Sprintf("Max %d\u00b0C (!)", s.Temperature))
		} else if s.Temperature > 45 {
			concerns = append(concerns, fmt.Sprintf("Max %d\u00b0C", s.Temperature))
		}
		if s.Reallocated > 0 {
			concerns = append(concerns, fmt.Sprintf("%d realloc", s.Reallocated))
		}
		if s.Pending > 0 {
			concerns = append(concerns, fmt.Sprintf("%d pending", s.Pending))
		}
		if s.PowerOnHours > 55000 {
			concerns = append(concerns, "Very old")
		}
		concernStr := "\u2014"
		concernClass := ""
		if len(concerns) > 0 {
			concernStr = strings.Join(concerns, ", ")
			concernClass = "val-warn"
			// Elevate to bad if there are critical issues
			if s.UDMACRC > 5 || s.Reallocated > 0 || s.Temperature > 55 || s.CommandTimeout > 10 {
				concernClass = "val-bad"
			}
		}

		// Health with caveats
		if s.HealthPassed && len(concerns) > 0 {
			healthStr = "PASSED*"
		}

		b.WriteString("      <tr>")
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(dev)))
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(slot)))
		b.WriteString(fmt.Sprintf("<td class=\"td-truncate\" title=\"%s\">%s</td>", escHTML(s.Model), escHTML(s.Model)))
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(sizeStr)))
		b.WriteString(fmt.Sprintf("<td>%s</td>", fmtHours(s.PowerOnHours)))
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(formatAge(s.PowerOnHours))))
		b.WriteString(fmt.Sprintf("<td class=\"%s\"><strong>%s</strong></td>", hClass, escHTML(healthStr)))
		b.WriteString(fmt.Sprintf("<td class=\"%s\">%s</td>", concernClass, escHTML(concernStr)))
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
		b.WriteString("  <h3 class=\"sub-heading\" style=\"margin-top: 0;\">Running Containers</h3>\n")
		b.WriteString("  <div class=\"table-wrap\"><table>\n")
		b.WriteString("    <thead><tr><th>Container</th><th>Status</th><th>Uptime</th></tr></thead>\n")
		b.WriteString("    <tbody>\n")
		for _, c := range snap.Docker.Containers {
			stateClass := "val-good"
			if c.State != "running" {
				stateClass = "val-warn"
			}
			b.WriteString("      <tr>")
			b.WriteString(fmt.Sprintf("<td><strong>%s</strong></td>", escHTML(c.Name)))
			b.WriteString(fmt.Sprintf("<td class=\"%s\">%s</td>", stateClass, escHTML(c.Status)))
			b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(c.Uptime)))
			b.WriteString("</tr>\n")
		}
		b.WriteString("    </tbody>\n")
		b.WriteString("  </table></div>\n")
	}

	// Top processes table
	if len(snap.System.TopProcesses) > 0 {
		b.WriteString("  <h3 class=\"sub-heading\">Top Processes by Resource Usage</h3>\n")
		b.WriteString("  <div class=\"table-wrap\"><table>\n")
		b.WriteString("    <thead><tr><th>Process</th><th>CPU %</th><th>RAM</th><th>Notes</th></tr></thead>\n")
		b.WriteString("    <tbody>\n")
		for _, p := range snap.System.TopProcesses {
			cpuClass := "val-good"
			if p.CPU > 10 {
				cpuClass = "val-warn"
			}
			if p.CPU > 50 {
				cpuClass = "val-bad"
			}
			b.WriteString("      <tr>")
			b.WriteString(fmt.Sprintf("<td><strong>%s</strong></td>", escHTML(p.Command)))
			b.WriteString(fmt.Sprintf("<td class=\"%s\"><strong>%.1f%%</strong></td>", cpuClass, p.CPU))
			b.WriteString(fmt.Sprintf("<td>%.1f%%</td>", p.Mem))
			b.WriteString(fmt.Sprintf("<td style=\"color: var(--slate-500);\">%s</td>", escHTML(p.User)))
			b.WriteString("</tr>\n")
		}
		b.WriteString("    </tbody>\n")
		b.WriteString("  </table></div>\n")
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

	b.WriteString("  <div class=\"table-wrap\"><table>\n")
	b.WriteString("    <thead><tr><th>Period</th><th>Duration</th><th>Speed</th><th>Errors</th><th>Action</th></tr></thead>\n")
	b.WriteString("    <tbody>\n")
	for _, p := range snap.Parity.History {
		errClass := "val-good"
		if p.Errors > 0 {
			errClass = "val-bad"
		}
		speedClass := ""
		if p.SpeedMBs > 0 && p.SpeedMBs < 30 {
			speedClass = "val-bad"
		} else if p.SpeedMBs > 0 && p.SpeedMBs < 60 {
			speedClass = "val-warn"
		}
		b.WriteString("      <tr>")
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(p.Date)))
		b.WriteString(fmt.Sprintf("<td><strong>%s</strong></td>", escHTML(fmtDurationSecs(p.Duration))))
		b.WriteString(fmt.Sprintf("<td class=\"%s\">%.1f MB/s</td>", speedClass, p.SpeedMBs))
		b.WriteString(fmt.Sprintf("<td class=\"%s\">%d</td>", errClass, p.Errors))
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(p.Action)))
		b.WriteString("</tr>\n")
	}
	b.WriteString("    </tbody>\n")
	b.WriteString("  </table></div>\n")

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

	b.WriteString("  <div class=\"table-wrap\"><table>\n")
	b.WriteString("    <thead><tr><th>Action</th><th>Priority</th><th>Estimated Impact</th><th>Cost</th></tr></thead>\n")
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
		b.WriteString(fmt.Sprintf("<td class=\"priority-cell\" style=\"color: %s;\"><strong>%s</strong></td>", pColor, escHTML(priority)))
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(impact)))
		b.WriteString(fmt.Sprintf("<td>%s</td>", escHTML(cost)))
		b.WriteString("</tr>\n")
	}
	b.WriteString("    </tbody>\n")
	b.WriteString("  </table></div>\n")

	b.WriteString("</div>\n")
}

// ─────────────────────────────────────────────────────────────────────
// Footer
// ─────────────────────────────────────────────────────────────────────

func writeFooter(b *strings.Builder, snap *internal.Snapshot) {
	b.WriteString("<div class=\"section\" style=\"margin-top: 48px; padding-top: 24px; border-top: 3px solid var(--slate-200);\">\n")
	b.WriteString("  <p style=\"font-size: 8.5pt; color: var(--slate-500); font-style: italic; margin-bottom: 16px; line-height: 1.6;\">")
	b.WriteString(fmt.Sprintf("This report was generated from a read-only diagnostic session on %s. No changes were made to the system.",
		escHTML(snap.Timestamp.Format("2 January 2006"))))
	b.WriteString("</p>\n")

	// Data sources
	sources := []string{}
	if len(snap.SMART) > 0 {
		sources = append(sources, "SMART")
	}
	if len(snap.Logs.DmesgErrors) > 0 || len(snap.Logs.SyslogErrors) > 0 {
		sources = append(sources, "dmesg", "syslog")
	}
	sources = append(sources, "/proc/meminfo", "/proc/stat")
	if snap.Docker.Available {
		sources = append(sources, "docker")
	}
	if snap.Parity != nil && len(snap.Parity.History) > 0 {
		sources = append(sources, "parity-checks.log")
	}

	b.WriteString(fmt.Sprintf("  <p style=\"font-size: 8pt; color: var(--slate-400); text-align: center;\">Data sources: %s.</p>\n",
		escHTML(strings.Join(sources, ", "))))

	b.WriteString("</div>\n")
}
