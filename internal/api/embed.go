package api

import "embed"

//go:embed templates/*
var templateFS embed.FS

// Dashboard theme HTML loaded from embedded templates.
var (
	DashboardMidnight string
	DashboardClean    string
	DashboardEmber    string
)

// Page HTML loaded from embedded templates.
var (
	statsPageHTML  string
	fleetPageHTML  string
	alertsPageHTML string
	SettingsPage   string
	DiskDetailPage string
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
	DashboardEmber = read("ember.html")
	statsPageHTML = read("stats.html")
	fleetPageHTML = read("fleet.html")
	alertsPageHTML = read("alerts.html")
	SettingsPage = read("settings.html")
	DiskDetailPage = read("disk_detail.html")
}
