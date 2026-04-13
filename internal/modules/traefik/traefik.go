package traefik

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// Traefik API types (v3)
type router struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Service string `json:"service"`
	Rule    string `json:"rule"`
}

type service struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Type   string `json:"type"`
}

type entrypoint struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// Module collects Traefik router/service status via the API.
// Detects Traefik via Docker (image traefik:*) or direct API endpoint.
type Module struct {
	apiURL       string // e.g. "http://localhost:8080" (Traefik API port)
	dockerSocket string
	client       *http.Client
}

func New(apiURL, dockerSocket string) *Module {
	if dockerSocket == "" {
		dockerSocket = "/var/run/docker.sock"
	}
	return &Module{
		apiURL:       apiURL,
		dockerSocket: dockerSocket,
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

func (m *Module) Name() string { return "traefik" }

func (m *Module) Detect() bool {
	// Method 1: direct API check
	if m.apiURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", m.apiURL+"/api/overview", nil)
		if req != nil {
			resp, err := m.client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return true
				}
			}
		}
	}

	// Method 2: check for traefik container via Docker socket
	if _, err := os.Stat(m.dockerSocket); err == nil {
		return m.detectViaDocker()
	}
	return false
}

func (m *Module) detectViaDocker() bool {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", m.dockerSocket)
			},
		},
		Timeout: 3 * time.Second,
	}

	resp, err := client.Get("http://localhost/containers/json?filters={\"ancestor\":[\"traefik\"]}")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var containers []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return false
	}
	return len(containers) > 0
}

func (m *Module) Collect(ctx context.Context) (transport.ModuleData, error) {
	if m.apiURL == "" {
		return transport.ModuleData{
			Module:    "traefik",
			Error:     "no API URL configured",
			Timestamp: time.Now(),
		}, nil
	}

	now := time.Now()
	var metrics []transport.MetricPoint

	// Routers
	routers, err := fetchJSON[[]router](ctx, m.client, m.apiURL+"/api/http/routers")
	if err == nil {
		var enabled, errored int
		for _, r := range routers {
			if r.Status == "enabled" {
				enabled++
			} else {
				errored++
			}
		}
		metrics = append(metrics,
			transport.MetricPoint{Name: "traefik_routers_total", Value: float64(len(routers))},
			transport.MetricPoint{Name: "traefik_routers_enabled", Value: float64(enabled)},
			transport.MetricPoint{Name: "traefik_routers_errored", Value: float64(errored)},
		)
	}

	// Services
	services, err := fetchJSON[[]service](ctx, m.client, m.apiURL+"/api/http/services")
	if err == nil {
		metrics = append(metrics,
			transport.MetricPoint{Name: "traefik_services_total", Value: float64(len(services))},
		)
	}

	// Entrypoints
	eps, err := fetchJSON[[]entrypoint](ctx, m.client, m.apiURL+"/api/entrypoints")
	if err == nil {
		metrics = append(metrics,
			transport.MetricPoint{Name: "traefik_entrypoints_total", Value: float64(len(eps))},
		)
		epNames := make([]string, 0, len(eps))
		for _, ep := range eps {
			epNames = append(epNames, ep.Name)
		}
		metadata := map[string]string{"entrypoints": strings.Join(epNames, ",")}
		return transport.ModuleData{
			Module:    "traefik",
			Metrics:   metrics,
			Metadata:  metadata,
			Timestamp: now,
		}, nil
	}

	return transport.ModuleData{
		Module:    "traefik",
		Metrics:   metrics,
		Timestamp: now,
	}, nil
}

func fetchJSON[T any](ctx context.Context, client *http.Client, url string) (T, error) {
	var zero T
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return zero, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return zero, fmt.Errorf("status %d", resp.StatusCode)
	}
	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return zero, err
	}
	return result, nil
}
