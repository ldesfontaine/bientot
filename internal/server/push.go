package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ldesfontaine/bientot/internal"
	"github.com/ldesfontaine/bientot/internal/transport"
)

// handlePush validates and stores an agent payload.
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB max
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	var payload transport.Payload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate machine_id has a registered token
	token, ok := s.tokens[payload.MachineID]
	if !ok {
		s.logger.Warn("unknown machine_id", "machine_id", payload.MachineID)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Anti-replay: check timestamp + nonce
	if err := s.nonces.Check(payload.Timestamp, payload.Nonce); err != nil {
		s.logger.Warn("replay rejected", "machine_id", payload.MachineID, "error", err)
		http.Error(w, fmt.Sprintf("replay: %v", err), http.StatusForbidden)
		return
	}

	// Verify HMAC signature
	if err := transport.Verify(payload.Body, token, payload.Signature); err != nil {
		s.logger.Warn("signature invalid", "machine_id", payload.MachineID)
		http.Error(w, "signature mismatch", http.StatusForbidden)
		return
	}

	// Convert and store metrics
	metrics := payloadToMetrics(payload)
	if err := s.store.Write(r.Context(), metrics); err != nil {
		s.logger.Error("store write failed", "error", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	s.logger.Debug("push accepted",
		"machine_id", payload.MachineID,
		"modules", len(payload.Body.Modules),
		"metrics", len(metrics),
	)

	// Extract software inventory from module metadata
	if s.enrichStore != nil {
		s.extractSoftwareInventory(r.Context(), payload)
	}

	// Update discovered services from bientot.service.* labels
	s.services.update(payload.MachineID, payload)

	// Publish push event to SSE clients
	s.sse.Publish(SSEEvent{
		Type: "push",
		Data: map[string]interface{}{
			"machine_id": payload.MachineID,
			"modules":    len(payload.Body.Modules),
			"metrics":    len(metrics),
		},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// payloadToMetrics converts a transport payload into storable Metrics.
func payloadToMetrics(p transport.Payload) []internal.Metric {
	var metrics []internal.Metric

	for _, mod := range p.Body.Modules {
		if mod.Error != "" {
			continue
		}
		for _, pt := range mod.Metrics {
			labels := make(map[string]string, len(pt.Labels)+2)
			for k, v := range pt.Labels {
				labels[k] = v
			}
			labels["machine_id"] = p.MachineID
			labels["module"] = mod.Module

			metrics = append(metrics, internal.Metric{
				Name:      pt.Name,
				Value:     pt.Value,
				Labels:    labels,
				Timestamp: mod.Timestamp,
				Source:    p.MachineID,
			})
		}
	}

	return metrics
}

// lastPushTime tracks when each machine last pushed (for status display).
var lastPush = struct {
	times map[string]time.Time
}{
	times: make(map[string]time.Time),
}

// extractSoftwareInventory parses module metadata for software entries.
// Docker module sends metadata like "image_traefik" = "traefik:3.1.2"
func (s *Server) extractSoftwareInventory(ctx context.Context, p transport.Payload) {
	for _, mod := range p.Body.Modules {
		if mod.Error != "" || len(mod.Metadata) == 0 {
			continue
		}

		switch mod.Module {
		case "docker":
			for key, value := range mod.Metadata {
				if !strings.HasPrefix(key, "image_") {
					continue
				}
				containerName := strings.TrimPrefix(key, "image_")
				name, version := parseImageTag(value)

				item := &internal.SoftwareItem{
					Machine:   p.MachineID,
					Name:      name,
					Version:   version,
					Source:    "docker",
					Container: containerName,
				}
				if err := s.enrichStore.UpsertSoftware(ctx, item); err != nil {
					s.logger.Warn("software inventory upsert failed",
						"machine", p.MachineID, "name", name, "error", err)
				}
			}
		}
	}
}

// parseImageTag splits "traefik:3.1.2" into ("traefik", "3.1.2").
// Handles registry prefixes like "ghcr.io/org/name:tag".
func parseImageTag(image string) (string, string) {
	// Remove registry prefix (anything before last /)
	if idx := strings.LastIndex(image, "/"); idx >= 0 {
		image = image[idx+1:]
	}

	// Split name:version
	parts := strings.SplitN(image, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], "latest"
}
