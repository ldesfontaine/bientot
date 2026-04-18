// Package system collects host machine metrics by scraping a node_exporter instance.
package system

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ldesfontaine/bientot/internal/modules"
)

// Module scrapes a node_exporter endpoint for host metrics.
type Module struct {
	url    string
	client *http.Client
}

// New returns a system module targeting the given node_exporter base URL
// (e.g. "http://node-exporter:9100"). An empty URL is accepted — Detect
// will then fail, keeping the module inactive.
func New(url string) *Module {
	return &Module{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name implements modules.Module.
func (m *Module) Name() string { return "system" }

// Detect implements modules.Module. Returns nil if node_exporter responds
// 200 on its /metrics endpoint.
func (m *Module) Detect(ctx context.Context) error {
	if m.url == "" {
		return fmt.Errorf("NODE_EXPORTER_URL not set")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.url+"/metrics", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("node_exporter unreachable at %s: %w", m.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("node_exporter returned %d", resp.StatusCode)
	}

	return nil
}

// Collect implements modules.Module. For palier 4.1 it returns a minimal
// data point proving the module is alive; real parsing lands in 4.2.
func (m *Module) Collect(_ context.Context) (*modules.Data, error) {
	hostname, _ := os.Hostname()
	return &modules.Data{
		Module:    m.Name(),
		Metrics:   []modules.Metric{{Name: "system_up", Value: 1}},
		Metadata:  map[string]string{"hostname": hostname, "scrape_target": m.url},
		Timestamp: time.Now(),
	}, nil
}

// Interval implements modules.Module.
func (m *Module) Interval() time.Duration { return 30 * time.Second }

// Compile-time check that *Module implements modules.Module.
var _ modules.Module = (*Module)(nil)

// Factory creates a system module from a config map.
// Required config key:
//   - node_exporter_url (string): URL of the node_exporter /metrics endpoint.
func Factory(cfg map[string]interface{}) (modules.Module, error) {
	url, _ := cfg["node_exporter_url"].(string)
	if url == "" {
		return nil, fmt.Errorf("system: node_exporter_url is required in config")
	}
	return New(url), nil
}

func init() {
	modules.Register("system", Factory)
}
