package netbird

import (
	"context"
	"net"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// Module vérifie la connectivité mesh NetBird en faisant un TCP-dial vers l'IP du pair.
// Pas besoin d'accès CLI — fonctionne depuis l'intérieur d'un conteneur.
type Module struct {
	peerIP   string // IP NetBird du pair distant (ex. "100.64.0.2")
	peerPort string // Port à dialer sur le pair (ex. "443")
}

func New(peerIP, peerPort string) *Module {
	if peerPort == "" {
		peerPort = "443"
	}
	return &Module{peerIP: peerIP, peerPort: peerPort}
}

func (m *Module) Name() string { return "netbird" }

func (m *Module) Detect() bool {
	return m.peerIP != ""
}

func (m *Module) Collect(_ context.Context) (transport.ModuleData, error) {
	now := time.Now()

	conn, err := net.DialTimeout("tcp", m.peerIP+":"+m.peerPort, 3*time.Second)
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
