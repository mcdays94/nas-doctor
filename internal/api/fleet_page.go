package api

import "net/http"

func (s *Server) handleFleetPage(w http.ResponseWriter, r *http.Request) {
	s.servePage(w, fleetPageHTML)
}
