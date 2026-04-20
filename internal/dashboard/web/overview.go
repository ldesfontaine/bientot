package web

import (
	"net/http"
	"time"
)

// overviewPageData bundles everything templates/overview.html needs.
type overviewPageData struct {
	Title       string
	MachineID   string
	Sidebar     *sidebarData
	KPIs        overviewKPIs
	ModuleCards []moduleCard
}

// handleOverview renders the per-machine overview page.
// Route: GET /agents/{id}
func (r *Router) handleOverview(w http.ResponseWriter, req *http.Request) {
	machineID := req.PathValue("id")
	if machineID == "" {
		http.Error(w, "missing machine id", http.StatusBadRequest)
		return
	}

	ctx := req.Context()

	exists, err := r.db.AgentExists(ctx, machineID)
	if err != nil {
		r.log.Error("check agent existence failed", "machine_id", machineID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	now := time.Now()

	sidebar, err := r.buildSidebar(ctx, machineID, now)
	if err != nil {
		r.log.Error("build sidebar failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	metrics, err := r.db.GetLatestMetrics(ctx, machineID)
	if err != nil {
		r.log.Error("load metrics failed", "machine_id", machineID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	lastPushAt := findLastPush(sidebar, machineID)
	kpis := buildKPIs(metrics, now, lastPushAt)

	activeModules, err := r.db.ListModulesForAgent(ctx, machineID)
	if err != nil {
		r.log.Error("list modules failed", "machine_id", machineID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	moduleCards := buildModuleCards(activeModules, now)

	data := overviewPageData{
		Title:       "Overview — " + machineID,
		MachineID:   machineID,
		Sidebar:     sidebar,
		KPIs:        kpis,
		ModuleCards: moduleCards,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.renderer.Render(w, "overview", data); err != nil {
		r.log.Error("render overview failed", "machine_id", machineID, "error", err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// findLastPush reads LastPushAt directly from the already-loaded sidebar
// data, avoiding a second DB query.
func findLastPush(sidebar *sidebarData, machineID string) time.Time {
	for _, m := range sidebar.Machines {
		if m.ID == machineID {
			return m.LastPushAt
		}
	}
	return time.Time{}
}
