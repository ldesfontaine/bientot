package api

import (
	"net/http"
	"time"

	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
)

// agentDTO is the JSON representation of an agent in API responses.
// All timestamps are Unix milliseconds (JavaScript-friendly).
type agentDTO struct {
	MachineID   string `json:"machineId"`
	FirstSeenAt int64  `json:"firstSeenAt"`
	LastPushAt  int64  `json:"lastPushAt"`
	Status      string `json:"status"`
}

const (
	statusOnline  = "online"
	statusOffline = "offline"
)

// handleListAgents returns all known agents with their computed online/offline
// status based on OFFLINE_THRESHOLD_SECONDS. Returns [] (not null) if empty.
func (r *Router) handleListAgents(w http.ResponseWriter, req *http.Request) {
	agents, err := r.db.ListAgents(req.Context())
	if err != nil {
		r.log.Error("list agents failed", "error", err)
		writeError(w, r.log, http.StatusInternalServerError, "failed to list agents")
		return
	}

	now := time.Now()
	dtos := make([]agentDTO, 0, len(agents))
	for _, a := range agents {
		dtos = append(dtos, toDTO(a, now, r.offlineThreshold))
	}

	writeJSON(w, r.log, http.StatusOK, dtos)
}

// toDTO converts a storage.Agent into the JSON-friendly agentDTO.
// Status uses a strict inequality (now - lastPush > threshold → offline)
// so an agent at exactly the threshold boundary stays online — avoids
// flip-flopping at the edge when real-world drift is in play.
func toDTO(a storage.Agent, now time.Time, threshold time.Duration) agentDTO {
	status := statusOnline
	if now.Sub(a.LastPushAt) > threshold {
		status = statusOffline
	}
	return agentDTO{
		MachineID:   a.MachineID,
		FirstSeenAt: a.FirstSeenAt.UnixMilli(),
		LastPushAt:  a.LastPushAt.UnixMilli(),
		Status:      status,
	}
}
