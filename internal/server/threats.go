package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) handleThreats(w http.ResponseWriter, r *http.Request) {
	if s.enrichStore == nil {
		http.Error(w, "enrichment not configured", http.StatusServiceUnavailable)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}

	intels, err := s.enrichStore.QueryIPIntel(r.Context(), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"threats": intels, "count": len(intels)})
}

func (s *Server) handleAttackers(w http.ResponseWriter, r *http.Request) {
	if s.enrichStore == nil {
		http.Error(w, "enrichment not configured", http.StatusServiceUnavailable)
		return
	}

	since := time.Now().Add(-24 * time.Hour)
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	logs, err := s.enrichStore.QueryAttackLogs(r.Context(), since, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"attackers": logs, "count": len(logs)})
}

func (s *Server) handlePatterns(w http.ResponseWriter, r *http.Request) {
	if s.enrichStore == nil {
		http.Error(w, "enrichment not configured", http.StatusServiceUnavailable)
		return
	}

	unresolvedOnly := r.URL.Query().Get("unresolved") == "true"
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	patterns, err := s.enrichStore.QueryPatterns(r.Context(), unresolvedOnly, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"patterns": patterns, "count": len(patterns)})
}

func (s *Server) handleUnblocked(w http.ResponseWriter, r *http.Request) {
	if s.enrichStore == nil {
		http.Error(w, "enrichment not configured", http.StatusServiceUnavailable)
		return
	}

	minScore, _ := strconv.Atoi(r.URL.Query().Get("min_score"))
	if minScore <= 0 {
		minScore = 50
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	intels, err := s.enrichStore.UnblockedHighRisk(r.Context(), minScore, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"unblocked": intels, "count": len(intels)})
}

func (s *Server) handleBudget(w http.ResponseWriter, _ *http.Request) {
	if s.pipeline == nil {
		http.Error(w, "enrichment not configured", http.StatusServiceUnavailable)
		return
	}

	status := s.pipeline.BudgetStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"budget": status})
}
