package netbird

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

type peerDetail struct {
	FQDN             string `json:"fqdn"`
	IP               string `json:"netbirdIp"`
	Status           string `json:"status"`
	LastStatusUpdate string `json:"lastStatusUpdate"`
	ConnectionType   string `json:"connectionType"`
	Latency          int64  `json:"latency"` // nanoseconds
}

type peersInfo struct {
	Total     int          `json:"total"`
	Connected int          `json:"connected"`
	Details   []peerDetail `json:"details"`
}

type netbirdStatus struct {
	Peers         peersInfo `json:"peers"`
	DaemonStatus  string    `json:"daemonStatus"`
	CLIVersion    string    `json:"cliVersion"`
	DaemonVersion string    `json:"daemonVersion"`
}

// Module collects NetBird mesh status via the CLI.
type Module struct{}

func New() *Module { return &Module{} }

func (m *Module) Name() string { return "netbird" }

func (m *Module) Detect() bool {
	_, err := exec.LookPath("netbird")
	return err == nil
}

func (m *Module) Collect(ctx context.Context) (transport.ModuleData, error) {
	now := time.Now()

	cmd := exec.CommandContext(ctx, "netbird", "status", "--json")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return transport.ModuleData{
			Module:    "netbird",
			Error:     err.Error(),
			Timestamp: now,
		}, nil
	}

	var status netbirdStatus
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		return transport.ModuleData{}, err
	}

	selfConnected := 0.0
	if strings.EqualFold(status.DaemonStatus, "connected") {
		selfConnected = 1.0
	}

	disconnected := status.Peers.Total - status.Peers.Connected

	metrics := []transport.MetricPoint{
		{Name: "netbird_connected", Value: selfConnected},
		{Name: "netbird_peers_total", Value: float64(status.Peers.Total)},
		{Name: "netbird_peers_connected", Value: float64(status.Peers.Connected)},
		{Name: "netbird_peers_disconnected", Value: float64(disconnected)},
	}

	// Per-peer metrics
	for _, peer := range status.Peers.Details {
		labels := map[string]string{"fqdn": peer.FQDN, "ip": peer.IP}
		peerConnected := 0.0
		if strings.EqualFold(peer.Status, "connected") {
			peerConnected = 1.0
		}
		metrics = append(metrics,
			transport.MetricPoint{Name: "netbird_peer_connected", Value: peerConnected, Labels: labels},
		)
		if peer.Latency > 0 {
			latencyMs := float64(peer.Latency) / 1e6
			metrics = append(metrics,
				transport.MetricPoint{Name: "netbird_peer_latency_ms", Value: latencyMs, Labels: labels},
			)
		}
	}

	// Build metadata from first peer's own IP (if available)
	metadata := map[string]string{
		"status":  status.DaemonStatus,
		"version": status.DaemonVersion,
	}
	if len(status.Peers.Details) > 0 {
		metadata["fqdn"] = status.Peers.Details[0].FQDN
	}

	return transport.ModuleData{
		Module:    "netbird",
		Metrics:   metrics,
		Metadata:  metadata,
		Timestamp: now,
	}, nil
}
