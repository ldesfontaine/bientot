package docker

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

type container struct {
	ID     string   `json:"Id"`
	Names  []string `json:"Names"`
	Image  string   `json:"Image"`
	State  string   `json:"State"`
	Status string   `json:"Status"`
}

// Module collects Docker container metrics.
type Module struct {
	client *http.Client
	host   string // "unix:///var/run/docker.sock" or "tcp://proxy:2375"
}

// New creates a Docker module. host can be a socket path or TCP address.
// If empty, auto-detects from DOCKER_HOST env or default socket.
func New(host string) *Module {
	if host == "" {
		host = os.Getenv("DOCKER_HOST")
	}
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}

	var client *http.Client

	if strings.HasPrefix(host, "unix://") {
		socketPath := strings.TrimPrefix(host, "unix://")
		client = &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
			Timeout: 10 * time.Second,
		}
	} else {
		// TCP (docker-socket-proxy)
		client = &http.Client{Timeout: 10 * time.Second}
	}

	return &Module{client: client, host: host}
}

func (m *Module) Name() string { return "docker" }

func (m *Module) Detect() bool {
	if strings.HasPrefix(m.host, "unix://") {
		socketPath := strings.TrimPrefix(m.host, "unix://")
		_, err := os.Stat(socketPath)
		return err == nil
	}
	// TCP: try a ping
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", m.baseURL()+"/_ping", nil)
	if err != nil {
		return false
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (m *Module) Collect(ctx context.Context) (transport.ModuleData, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", m.baseURL()+"/containers/json?all=true", nil)
	if err != nil {
		return transport.ModuleData{}, fmt.Errorf("creating request: %w", err)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return transport.ModuleData{}, fmt.Errorf("fetching containers: %w", err)
	}
	defer resp.Body.Close()

	var containers []container
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return transport.ModuleData{}, fmt.Errorf("decoding containers: %w", err)
	}

	now := time.Now()
	var metrics []transport.MetricPoint
	var running, stopped int

	for _, c := range containers {
		name := containerName(c)
		labels := map[string]string{"name": name, "id": c.ID[:12], "image": c.Image}

		isRunning := 0.0
		if c.State == "running" {
			isRunning = 1.0
			running++
		} else {
			stopped++
		}

		metrics = append(metrics,
			transport.MetricPoint{Name: "docker_container_running", Value: isRunning, Labels: labels},
			transport.MetricPoint{Name: "docker_container_health", Value: parseHealth(c.Status), Labels: labels},
		)
	}

	metrics = append(metrics,
		transport.MetricPoint{Name: "docker_containers_total", Value: float64(len(containers))},
		transport.MetricPoint{Name: "docker_containers_running", Value: float64(running)},
		transport.MetricPoint{Name: "docker_containers_stopped", Value: float64(stopped)},
	)

	// Software inventory: image tags
	images := make(map[string]string)
	for _, c := range containers {
		if c.State == "running" {
			images[containerName(c)] = c.Image
		}
	}
	metadata := make(map[string]string, len(images))
	for name, image := range images {
		metadata["image_"+name] = image
	}

	return transport.ModuleData{
		Module:    "docker",
		Metrics:   metrics,
		Metadata:  metadata,
		Timestamp: now,
	}, nil
}

func (m *Module) baseURL() string {
	if strings.HasPrefix(m.host, "unix://") {
		return "http://localhost"
	}
	// tcp://host:port -> http://host:port
	return "http://" + strings.TrimPrefix(m.host, "tcp://")
}

func containerName(c container) string {
	if len(c.Names) == 0 {
		return c.ID[:12]
	}
	name := c.Names[0]
	return strings.TrimPrefix(name, "/")
}

func parseHealth(status string) float64 {
	switch {
	case strings.Contains(status, "(healthy)"):
		return 2
	case strings.Contains(status, "(unhealthy)"):
		return 1
	default:
		return 0
	}
}
