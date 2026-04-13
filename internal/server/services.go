package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// ServiceInfo represents a service discovered via Docker labels.
type ServiceInfo struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Icon    string `json:"icon"`
	Public  bool   `json:"public"`
	Status  string `json:"status"`  // running, exited, ...
	Health  string `json:"health"`  // healthy, unhealthy, starting, none
	Machine string `json:"machine"` // machine_id source
}

// serviceStore holds discovered services per machine, updated on each push.
type serviceStore struct {
	mu       sync.RWMutex
	byMachine map[string][]ServiceInfo // machine_id -> services
	lastSeen  map[string]time.Time     // machine_id -> last push
}

func newServiceStore() *serviceStore {
	return &serviceStore{
		byMachine: make(map[string][]ServiceInfo),
		lastSeen:  make(map[string]time.Time),
	}
}

// update replaces all services for a machine from a push payload.
func (ss *serviceStore) update(machineID string, payload transport.Payload) {
	var services []ServiceInfo

	for _, mod := range payload.Body.Modules {
		if mod.Module != "docker" || mod.Error != "" || len(mod.Metadata) == 0 {
			continue
		}

		// Collect service names from svc_<container>_name entries
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

// all returns all discovered services across machines.
func (ss *serviceStore) all() []ServiceInfo {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	var all []ServiceInfo
	for _, svcs := range ss.byMachine {
		all = append(all, svcs...)
	}
	return all
}

// handleServices returns all discovered services as JSON.
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
