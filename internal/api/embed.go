package api

import "embed"

//go:embed templates/*
var templateFS embed.FS

// Dashboard theme HTML loaded from embedded templates.
var (
	DashboardMidnight string
	DashboardClean    string
)

// Page HTML loaded from embedded templates.
var (
	statsPageHTML          string
	fleetPageHTML          string
	alertsPageHTML         string
	serviceChecksPageHTML  string
	parityPageHTML         string
	replacementPlannerHTML string
	SettingsPage           string
	DiskDetailPage         string
)

func init() {
	read := func(name string) string {
		data, err := templateFS.ReadFile("templates/" + name)
		if err != nil {
			panic("embedded template missing: " + name + ": " + err.Error())
		}
		return string(data)
	}

	DashboardMidnight = read("midnight.html")
	DashboardClean = read("clean.html")
	statsPageHTML = read("stats.html")
	fleetPageHTML = read("fleet.html")
	alertsPageHTML = read("alerts.html")
	serviceChecksPageHTML = read("service_checks.html")
	parityPageHTML = read("parity.html")
	replacementPlannerHTML = read("replacement_planner.html")
	SettingsPage = read("settings.html")
	DiskDetailPage = read("disk_detail.html")
}
