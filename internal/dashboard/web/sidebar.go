package web

import (
	"context"
	"time"
)

// sidebarMachine is the representation of one machine in the sidebar.
type sidebarMachine struct {
	ID     string
	Status string // "online" | "offline"
}

// sidebarData is the common set of data every authenticated page needs
// to render its sidebar. Built by buildSidebar from storage.
type sidebarData struct {
	CurrentMachineID string
	Machines         []sidebarMachine
	Version          string
}

// buildSidebar loads the data needed to render the sidebar for a given
// currently-selected machine. currentID may be empty on pages where no
// machine is in scope.
//
// Status mirrors the rule used by the API handler:
// now - lastPushAt > offlineThreshold => offline.
func (r *Router) buildSidebar(ctx context.Context, currentID string, now time.Time) (*sidebarData, error) {
	agents, err := r.db.ListAgents(ctx)
	if err != nil {
		return nil, err
	}

	machines := make([]sidebarMachine, 0, len(agents))
	for _, a := range agents {
		status := "online"
		if now.Sub(a.LastPushAt) > r.offlineThreshold {
			status = "offline"
		}
		machines = append(machines, sidebarMachine{
			ID:     a.MachineID,
			Status: status,
		})
	}

	return &sidebarData{
		CurrentMachineID: currentID,
		Machines:         machines,
		Version:          r.version,
	}, nil
}

// firstMachineID returns the alphabetically-first machine ID (matching
// ListAgents order) or "" if no agents exist.
func (r *Router) firstMachineID(ctx context.Context) (string, error) {
	agents, err := r.db.ListAgents(ctx)
	if err != nil {
		return "", err
	}
	if len(agents) == 0 {
		return "", nil
	}
	return agents[0].MachineID, nil
}
