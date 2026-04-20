package web

import (
	"net/http"
)

// handleHome routes to the appropriate landing page:
//   - If no agents exist, render the empty-state page
//   - Otherwise redirect to the first agent's overview (alphabetical)
//
// Route: GET /
func (r *Router) handleHome(w http.ResponseWriter, req *http.Request) {
	firstID, err := r.firstMachineID(req.Context())
	if err != nil {
		r.log.Error("load first machine failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if firstID == "" {
		r.renderNoAgents(w, req)
		return
	}

	http.Redirect(w, req, "/agents/"+firstID, http.StatusSeeOther)
}

// renderNoAgents renders the empty-state page shown when the dashboard
// has never seen a push.
func (r *Router) renderNoAgents(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := struct {
		Title string
	}{Title: "No agents"}

	if err := r.renderer.RenderStandalone(w, "no_agents", data); err != nil {
		r.log.Error("render no_agents failed", "error", err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
