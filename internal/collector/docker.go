package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// DockerCollector collects container health from Docker API
type DockerCollector struct {
	name       string
	socketPath string
	interval   time.Duration
	client     *http.Client
}

type dockerContainer struct {
	ID     string `json:"Id"`
	Names  []string `json:"Names"`
	State  string `json:"State"`
	Status string `json:"Status"`
}

// NewDockerCollector creates a new Docker collector
func NewDockerCollector(name, socketPath string, interval time.Duration) *DockerCollector {
	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}

	return &DockerCollector{
		name:       name,
		socketPath: socketPath,
		interval:   interval,
		client: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
	}
}

func (c *DockerCollector) Name() string           { return c.name }
func (c *DockerCollector) Type() string           { return "docker" }
func (c *DockerCollector) Interval() time.Duration { return c.interval }

func (c *DockerCollector) Collect(ctx context.Context) ([]internal.Metric, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost/containers/json?all=true", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching containers: %w", err)
	}
	defer resp.Body.Close()

	var containers []dockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	now := time.Now()
	var metrics []internal.Metric

	for _, container := range containers {
		name := container.Names[0]
		if len(name) > 0 && name[0] == '/' {
			name = name[1:]
		}

		// Container running status (1 = running, 0 = stopped)
		running := 0.0
		if container.State == "running" {
			running = 1.0
		}
		metrics = append(metrics, internal.Metric{
			Name:      "container_running",
			Value:     running,
			Labels:    map[string]string{"name": name, "id": container.ID[:12]},
			Timestamp: now,
			Source:    c.name,
		})

		// Container health status
		healthValue := c.parseHealthStatus(container.Status)
		metrics = append(metrics, internal.Metric{
			Name:      "container_health",
			Value:     healthValue,
			Labels:    map[string]string{"name": name, "id": container.ID[:12]},
			Timestamp: now,
			Source:    c.name,
		})
	}

	// Total containers
	metrics = append(metrics, internal.Metric{
		Name:      "containers_total",
		Value:     float64(len(containers)),
		Timestamp: now,
		Source:    c.name,
	})

	return metrics, nil
}

func (c *DockerCollector) parseHealthStatus(status string) float64 {
	// 2 = healthy, 1 = unhealthy, 0 = no healthcheck
	switch {
	case contains(status, "(healthy)"):
		return 2
	case contains(status, "(unhealthy)"):
		return 1
	default:
		return 0
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
