package api

import "net/http"

// healthResponse is the body returned by GET /api/health.
type healthResponse struct {
	Status string `json:"status"`
}

// handleHealth is a liveness probe. Returns 200 OK always (no DB check).
// For a readiness probe that verifies storage, add /api/ready later.
func (r *Router) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, r.log, http.StatusOK, healthResponse{Status: "ok"})
}
