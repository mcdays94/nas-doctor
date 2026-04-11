package api

import (
	"encoding/json"
	"net/http"

	"github.com/mcdays94/nas-doctor/internal/analyzer"
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(replacementPlannerHTML))
}
