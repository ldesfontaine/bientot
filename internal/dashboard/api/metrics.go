package api

import (
	"net/http"
	"sort"

	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
)

// metricDTO is the JSON representation of a single metric's latest value.
type metricDTO struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Module    string            `json:"module"`
	Labels    map[string]string `json:"labels"`
	Timestamp int64             `json:"timestamp"` // Unix milliseconds
}

// handleGetLatestMetrics returns the latest value of every metric for an agent.
// Returns 404 if the agent doesn't exist, [] if it exists but has no metrics.
// Metrics are sorted by name for a deterministic response.
//
// Route: GET /api/agents/{id}/metrics
func (s *Server) handleGetLatestMetrics(w http.ResponseWriter, r *http.Request) {
	machineID := r.PathValue("id")
	if machineID == "" {
		writeError(w, s.log, http.StatusBadRequest, "missing agent id")
		return
	}

	exists, err := s.db.AgentExists(r.Context(), machineID)
	if err != nil {
		s.log.Error("check agent existence failed", "machine_id", machineID, "error", err)
		writeError(w, s.log, http.StatusInternalServerError, "internal error")
		return
	}
	if !exists {
		writeError(w, s.log, http.StatusNotFound, "agent not found")
		return
	}

	metrics, err := s.db.GetLatestMetrics(r.Context(), machineID)
	if err != nil {
		s.log.Error("get latest metrics failed", "machine_id", machineID, "error", err)
		writeError(w, s.log, http.StatusInternalServerError, "failed to fetch metrics")
		return
	}

	dtos := make([]metricDTO, 0, len(metrics))
	for _, m := range metrics {
		dtos = append(dtos, toMetricDTO(m))
	}

	sort.Slice(dtos, func(i, j int) bool {
		return dtos[i].Name < dtos[j].Name
	})

	writeJSON(w, s.log, http.StatusOK, dtos)
}

// toMetricDTO converts a storage.Metric into its JSON-friendly DTO.
// Labels are always a non-nil map (empty if source has none) so the
// frontend can assume labels is always an object, never null.
func toMetricDTO(m storage.Metric) metricDTO {
	labels := m.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	return metricDTO{
		Name:      m.Name,
		Value:     m.Value,
		Module:    m.Module,
		Labels:    labels,
		Timestamp: timestampNsToMillis(m.TimestampNs),
	}
}

// timestampNsToMillis converts Unix nanoseconds (storage) to Unix
// milliseconds (JSON API). Reused across the api package.
func timestampNsToMillis(ns int64) int64 {
	return ns / 1_000_000
}
