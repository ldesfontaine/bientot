package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// WebhookNotifier sends alerts via generic HTTP webhook
type WebhookNotifier struct {
	name       string
	url        string
	headers    map[string]string
	severities []internal.Severity
	client     *http.Client
}

// WebhookConfig holds webhook configuration
type WebhookConfig struct {
	Name       string
	URL        string
	Headers    map[string]string
	Severities []internal.Severity
}

// NewWebhookNotifier creates a new webhook notifier
func NewWebhookNotifier(config WebhookConfig) *WebhookNotifier {
	return &WebhookNotifier{
		name:       config.Name,
		url:        config.URL,
		headers:    config.Headers,
		severities: config.Severities,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (n *WebhookNotifier) Name() string { return n.name }
func (n *WebhookNotifier) Type() string { return "webhook" }

func (n *WebhookNotifier) SupportsSeverity(severity internal.Severity) bool {
	for _, s := range n.severities {
		if s == severity {
			return true
		}
	}
	return false
}

func (n *WebhookNotifier) Send(alert internal.Alert) error {
	payload := map[string]interface{}{
		"id":       alert.ID,
		"name":     alert.Name,
		"severity": alert.Severity,
		"message":  alert.Message,
		"labels":   alert.Labels,
		"value":    alert.Value,
		"fired_at": alert.FiredAt.Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	req, err := http.NewRequest("POST", n.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range n.headers {
		req.Header.Set(k, v)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return nil
}
