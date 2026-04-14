package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ldesfontaine/bientot/internal"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "ok",
		"machines": len(s.tokens),
	})
}

func (s *Server) handleMachines(w http.ResponseWriter, _ *http.Request) {
	machines := make([]string, 0, len(s.tokens))
	for id := range s.tokens {
		machines = append(machines, id)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"machines": machines,
	})
}

func (s *Server) handleMachineMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	machineID := chi.URLParam(r, "machineID")

	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		}
	}

	// Récupération de tous les noms de métriques, puis filtrage par machine_id via les labels
	names, err := s.store.List(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result := make(map[string]interface{})
	for _, name := range names {
		metric, err := s.store.QueryLatest(ctx, name, map[string]string{"machine_id": machineID})
		if err == nil && metric != nil {
			points, _ := s.store.Query(ctx, name, from, to, internal.ResolutionRaw)
			result[name] = map[string]interface{}{
				"latest": metric,
				"points": points,
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleListMetrics(w http.ResponseWriter, r *http.Request) {
	names, err := s.store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{"metrics": names})
}

func (s *Server) handleQueryMetric(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := chi.URLParam(r, "name")

	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()
	resolution := internal.ResolutionRaw

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		}
	}
	if resStr := r.URL.Query().Get("resolution"); resStr != "" {
		resolution = internal.Resolution(resStr)
	}

	points, err := s.store.Query(ctx, name, from, to, resolution)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"metric": name,
		"from":   from,
		"to":     to,
		"points": points,
	})
}

func (s *Server) handleLatestMetric(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := chi.URLParam(r, "name")

	metric, err := s.store.QueryLatest(ctx, name, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if metric == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metric)
}
