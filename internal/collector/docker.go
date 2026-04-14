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

// DockerCollector collecte la santé des conteneurs depuis l'API Docker
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

// NewDockerCollector crée un nouveau collecteur Docker
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
		return nil, fmt.Errorf("création de la requête: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("récupération des conteneurs: %w", err)
	}
	defer resp.Body.Close()

	var containers []dockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("décodage de la réponse: %w", err)
	}

	now := time.Now()
	var metrics []internal.Metric

	for _, container := range containers {
		name := container.Names[0]
		if len(name) > 0 && name[0] == '/' {
			name = name[1:]
		}

		// Statut du conteneur (1 = actif, 0 = arrêté)
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

		// Statut de santé du conteneur
		healthValue := c.parseHealthStatus(container.Status)
		metrics = append(metrics, internal.Metric{
			Name:      "container_health",
			Value:     healthValue,
			Labels:    map[string]string{"name": name, "id": container.ID[:12]},
			Timestamp: now,
			Source:    c.name,
		})
	}

	// Total des conteneurs
	metrics = append(metrics, internal.Metric{
		Name:      "containers_total",
		Value:     float64(len(containers)),
		Timestamp: now,
		Source:    c.name,
	})

	return metrics, nil
}

func (c *DockerCollector) parseHealthStatus(status string) float64 {
	// 2 = sain, 1 = défaillant, 0 = pas de healthcheck
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
