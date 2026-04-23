package api

import "net/http"

func (s *Server) handleStatsPage(w http.ResponseWriter, r *http.Request) {
	s.servePage(w, statsPageHTML)
}
