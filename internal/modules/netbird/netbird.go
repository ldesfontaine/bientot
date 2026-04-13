package netbird

import (
	"context"
	"net"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// Module checks NetBird mesh connectivity by TCP-dialing the peer IP.
// No CLI access needed — works from inside a container.
type Module struct {
	peerIP string // NetBird IP of the remote peer (e.g. "100.64.0.2")
}

func New(peerIP string) *Module {
	return &Module{peerIP: peerIP}
}

func (m *Module) Name() string { return "netbird" }

func (m *Module) Detect() bool {
	return m.peerIP != ""
}

func (m *Module) Collect(_ context.Context) (transport.ModuleData, error) {
	now := time.Now()

	conn, err := net.DialTimeout("tcp", m.peerIP+":9100", 3*time.Second)
	meshUp := 0.0
	if err == nil {
		meshUp = 1.0
		conn.Close()
	}

	metrics := []transport.MetricPoint{
		{Name: "netbird_connected", Value: meshUp},
	}

	metadata := map[string]string{
		"peer_ip": m.peerIP,
	}

	return transport.ModuleData{
		Module:    "netbird",
		Metrics:   metrics,
		Metadata:  metadata,
		Timestamp: now,
	}, nil
}
