package api

import "net/http"

// healthResponse is the body returned by GET /api/health.
type healthResponse struct {
	Status string `json:"status"`
}

// handleHealth is a liveness probe. Returns 200 OK always (no DB check).
// For a readiness probe that verifies storage, add /api/ready later.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.log, http.StatusOK, healthResponse{Status: "ok"})
}
