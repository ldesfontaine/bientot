package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleAlerts(w http.ResponseWriter, _ *http.Request) {
	if s.alerter == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"alerts": []interface{}{}})
		return
	}

	alerts := s.alerter.ActiveAlerts()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"alerts": alerts})
}

func (s *Server) handleActiveAlerts(w http.ResponseWriter, _ *http.Request) {
	if s.alerter == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"alerts": []interface{}{}, "count": 0})
		return
	}

	alerts := s.alerter.ActiveAlerts()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"alerts": alerts,
		"count":  len(alerts),
	})
}

func (s *Server) handleAckAlert(w http.ResponseWriter, r *http.Request) {
	if s.alerter == nil {
		http.Error(w, "alerter non configuré", http.StatusServiceUnavailable)
		return
	}

	alertID := chi.URLParam(r, "alertID")
	if s.alerter.Acknowledge(alertID) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "acknowledged"})
	} else {
		http.Error(w, "alerte introuvable", http.StatusNotFound)
	}
}
