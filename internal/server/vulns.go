package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleVulns(w http.ResponseWriter, r *http.Request) {
	if s.enrichStore == nil {
		http.Error(w, "veille not configured", http.StatusServiceUnavailable)
		return
	}

	machine := r.URL.Query().Get("machine")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	matches, err := s.enrichStore.QueryVulnMatches(r.Context(), machine, false, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"vulns": matches, "count": len(matches)})
}

func (s *Server) handleActiveVulns(w http.ResponseWriter, r *http.Request) {
	if s.enrichStore == nil {
		http.Error(w, "veille not configured", http.StatusServiceUnavailable)
		return
	}

	machine := r.URL.Query().Get("machine")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	matches, err := s.enrichStore.QueryVulnMatches(r.Context(), machine, true, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"vulns": matches, "count": len(matches)})
}

func (s *Server) handleInventory(w http.ResponseWriter, r *http.Request) {
	if s.enrichStore == nil {
		http.Error(w, "veille not configured", http.StatusServiceUnavailable)
		return
	}

	machine := r.URL.Query().Get("machine")

	items, err := s.enrichStore.QuerySoftware(r.Context(), machine)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"inventory": items, "count": len(items)})
}

func (s *Server) handleDismissVuln(w http.ResponseWriter, r *http.Request) {
	if s.enrichStore == nil {
		http.Error(w, "veille not configured", http.StatusServiceUnavailable)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := s.enrichStore.DismissVuln(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "dismissed"})
}

func (s *Server) handleResolveVuln(w http.ResponseWriter, r *http.Request) {
	if s.enrichStore == nil {
		http.Error(w, "veille not configured", http.StatusServiceUnavailable)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := s.enrichStore.ResolveVuln(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "resolved"})
}

func (s *Server) handleSyncLogs(w http.ResponseWriter, r *http.Request) {
	if s.enrichStore == nil {
		http.Error(w, "veille not configured", http.StatusServiceUnavailable)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	logs, err := s.enrichStore.QuerySyncLogs(r.Context(), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"sync_logs": logs, "count": len(logs)})
}
