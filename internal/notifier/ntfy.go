package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// NtfyNotifier sends alerts via Ntfy
type NtfyNotifier struct {
	name       string
	url        string
	topic      string
	severities []internal.Severity
	client     *http.Client
}

// NtfyConfig holds Ntfy configuration
type NtfyConfig struct {
	Name       string
	URL        string
	Topic      string
	Severities []internal.Severity
}

// NewNtfyNotifier creates a new Ntfy notifier
func NewNtfyNotifier(config NtfyConfig) *NtfyNotifier {
	return &NtfyNotifier{
		name:       config.Name,
		url:        config.URL,
		topic:      config.Topic,
		severities: config.Severities,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (n *NtfyNotifier) Name() string { return n.name }
func (n *NtfyNotifier) Type() string { return "ntfy" }

func (n *NtfyNotifier) SupportsSeverity(severity internal.Severity) bool {
	for _, s := range n.severities {
		if s == severity {
			return true
		}
	}
	return false
}

func (n *NtfyNotifier) Send(alert internal.Alert) error {
	endpoint := fmt.Sprintf("%s/%s", n.url, n.topic)

	payload := map[string]interface{}{
		"topic":   n.topic,
		"title":   fmt.Sprintf("[%s] %s", alert.Severity, alert.Name),
		"message": alert.Message,
		"tags":    n.severityTags(alert.Severity),
	}

	// Add priority based on severity
	switch alert.Severity {
	case internal.SeverityCritical:
		payload["priority"] = 5
	case internal.SeverityWarning:
		payload["priority"] = 4
	default:
		payload["priority"] = 3
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return nil
}

func (n *NtfyNotifier) severityTags(severity internal.Severity) []string {
	switch severity {
	case internal.SeverityCritical:
		return []string{"rotating_light", "skull"}
	case internal.SeverityWarning:
		return []string{"warning"}
	default:
		return []string{"information_source"}
	}
}
