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

// handlePush valide et stocke un payload agent.
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB max
	if err != nil {
		http.Error(w, "erreur de lecture", http.StatusBadRequest)
		return
	}

	var payload transport.Payload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "JSON invalide", http.StatusBadRequest)
		return
	}

	// Vérification que le machine_id a un token enregistré
	token, ok := s.tokens[payload.MachineID]
	if !ok {
		s.logger.Warn("machine_id inconnu", "machine_id", payload.MachineID)
		http.Error(w, "non autorisé", http.StatusUnauthorized)
		return
	}

	// Anti-rejeu : vérification timestamp + nonce
	if err := s.nonces.Check(payload.Timestamp, payload.Nonce); err != nil {
		s.logger.Warn("rejeu rejeté", "machine_id", payload.MachineID, "error", err)
		http.Error(w, fmt.Sprintf("replay: %v", err), http.StatusForbidden)
		return
	}

	// Vérification de la signature HMAC
	if err := transport.Verify(payload.Body, token, payload.Signature); err != nil {
		s.logger.Warn("signature invalide", "machine_id", payload.MachineID)
		http.Error(w, "signature incorrecte", http.StatusForbidden)
		return
	}

	// Conversion et stockage des métriques
	metrics := payloadToMetrics(payload)
	if err := s.store.Write(r.Context(), metrics); err != nil {
		s.logger.Error("échec de l'écriture en stockage", "error", err)
		http.Error(w, "erreur de stockage", http.StatusInternalServerError)
		return
	}

	s.logger.Debug("push accepté",
		"machine_id", payload.MachineID,
		"modules", len(payload.Body.Modules),
		"metrics", len(metrics),
	)

	// Extraction de l'inventaire logiciel depuis les métadonnées des modules
	if s.enrichStore != nil {
		s.extractSoftwareInventory(r.Context(), payload)
	}

	// Mise à jour des services découverts depuis les labels bientot.service.*
	s.services.update(payload.MachineID, payload)

	// Publication de l'événement push aux clients SSE
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

// payloadToMetrics convertit un payload transport en métriques stockables.
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

// lastPushTime trace le dernier push de chaque machine (pour l'affichage du statut).
var lastPush = struct {
	times map[string]time.Time
}{
	times: make(map[string]time.Time),
}

// extractSoftwareInventory analyse les métadonnées des modules pour les entrées logicielles.
// Le module Docker envoie des métadonnées comme "image_traefik" = "traefik:3.1.2"
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
					s.logger.Warn("échec de l'upsert de l'inventaire logiciel",
						"machine", p.MachineID, "name", name, "error", err)
				}
			}
		}
	}
}

// parseImageTag sépare "traefik:3.1.2" en ("traefik", "3.1.2").
// Gère les préfixes de registre comme "ghcr.io/org/name:tag".
func parseImageTag(image string) (string, string) {
	// Suppression du préfixe de registre (tout avant le dernier /)
	if idx := strings.LastIndex(image, "/"); idx >= 0 {
		image = image[idx+1:]
	}

	// Séparation nom:version
	parts := strings.SplitN(image, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], "latest"
}
