package web

import (
	"net/http"
	"time"
)

// overviewPageData bundles the data rendered by templates/overview.html.
// For 5.3.1, only sidebar data is real — KPIs come in 5.3.2.
type overviewPageData struct {
	Title   string
	Sidebar *sidebarData
}

// handleOverview renders the per-machine overview page.
// Returns 404 if the machine ID is unknown.
//
// Route: GET /agents/{id}
func (r *Router) handleOverview(w http.ResponseWriter, req *http.Request) {
	machineID := req.PathValue("id")
	if machineID == "" {
		http.Error(w, "missing machine id", http.StatusBadRequest)
		return
	}

	exists, err := r.db.AgentExists(req.Context(), machineID)
	if err != nil {
		r.log.Error("check agent existence failed", "machine_id", machineID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	sidebar, err := r.buildSidebar(req.Context(), machineID, time.Now())
	if err != nil {
		r.log.Error("build sidebar failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data := overviewPageData{
		Title:   "Overview — " + machineID,
		Sidebar: sidebar,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.renderer.Render(w, "overview", data); err != nil {
		r.log.Error("render overview failed", "machine_id", machineID, "error", err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
