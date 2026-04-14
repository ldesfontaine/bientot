package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// ServiceInfo représente un service découvert via les labels Docker.
type ServiceInfo struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Icon    string `json:"icon"`
	Public  bool   `json:"public"`
	Status  string `json:"status"`  // running, exited, ...
	Health  string `json:"health"`  // healthy, unhealthy, starting, none
	Machine string `json:"machine"` // machine_id source
}

// serviceStore contient les services découverts par machine, mis à jour à chaque push.
type serviceStore struct {
	mu       sync.RWMutex
	byMachine map[string][]ServiceInfo // machine_id -> services
	lastSeen  map[string]time.Time     // machine_id -> dernier push
}

func newServiceStore() *serviceStore {
	return &serviceStore{
		byMachine: make(map[string][]ServiceInfo),
		lastSeen:  make(map[string]time.Time),
	}
}

// update remplace tous les services d'une machine depuis un payload de push.
func (ss *serviceStore) update(machineID string, payload transport.Payload) {
	var services []ServiceInfo

	for _, mod := range payload.Body.Modules {
		if mod.Module != "docker" || mod.Error != "" || len(mod.Metadata) == 0 {
			continue
		}

		// Collecte des noms de services depuis les entrées svc_<container>_name
		containers := make(map[string]bool)
		for key := range mod.Metadata {
			if strings.HasPrefix(key, "svc_") && strings.HasSuffix(key, "_name") {
				cName := key[4 : len(key)-5] // strip "svc_" and "_name"
				containers[cName] = true
			}
		}

		for cName := range containers {
			prefix := "svc_" + cName + "_"
			svc := ServiceInfo{
				Name:    mod.Metadata[prefix+"name"],
				URL:     mod.Metadata[prefix+"url"],
				Icon:    mod.Metadata[prefix+"icon"],
				Status:  mod.Metadata[prefix+"status"],
				Health:  mod.Metadata[prefix+"health"],
				Machine: machineID,
				Public:  mod.Metadata[prefix+"public"] == "true",
			}
			services = append(services, svc)
		}
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.byMachine[machineID] = services
	ss.lastSeen[machineID] = time.Now()
}

// all return tous les services découverts sur toutes les machines.
func (ss *serviceStore) all() []ServiceInfo {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	var all []ServiceInfo
	for _, svcs := range ss.byMachine {
		all = append(all, svcs...)
	}
	return all
}

// handleServices return tous les services découverts en JSON.
func (s *Server) handleServices(w http.ResponseWriter, _ *http.Request) {
	services := s.services.all()
	if services == nil {
		services = []ServiceInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"services": services,
	})
}
