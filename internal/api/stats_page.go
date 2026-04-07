package api

import "net/http"

func (s *Server) handleStatsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(statsPageHTML))
}
