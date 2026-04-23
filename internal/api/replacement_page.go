package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/mcdays94/nas-doctor/internal/analyzer"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

func (s *Server) handleReplacementPlan(w http.ResponseWriter, r *http.Request) {
	// Get latest snapshot for SMART data
	snap, err := s.store.GetLatestSnapshot()
	if err != nil || snap == nil {
		writeJSON(w, http.StatusOK, analyzer.ReplacementPlan{})
		return
	}

	// Get cost_per_tb from settings
	var costPerTB float64
	raw, err := s.store.GetConfig("settings")
	if err == nil && raw != "" {
		var settings Settings
		if json.Unmarshal([]byte(raw), &settings) == nil {
			costPerTB = settings.CostPerTB
		}
	}

	plan := analyzer.BuildReplacementPlan(snap.SMART, costPerTB)
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleReplacementPlannerPage(w http.ResponseWriter, r *http.Request) {
	s.servePage(w, replacementPlannerHTML)
}

func (s *Server) handleCapacityForecast(w http.ResponseWriter, r *http.Request) {
	series, err := s.store.GetDiskUsageHistory(500)
	if err != nil {
		writeJSON(w, http.StatusOK, analyzer.CapacityReport{})
		return
	}
	report := analyzer.BuildCapacityReport(series)
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleDiskUsageHistory(w http.ResponseWriter, r *http.Request) {
	limit := 500
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	series, err := s.store.GetDiskUsageHistory(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if series == nil {
		series = []storage.DiskUsageSeries{}
	}
	writeJSON(w, http.StatusOK, series)
}
