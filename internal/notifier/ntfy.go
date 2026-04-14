package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// NtfyNotifier envoie les alertes via Ntfy
type NtfyNotifier struct {
	name       string
	url        string
	topic      string
	token      string
	severities []internal.Severity
	client     *http.Client
}

// NtfyConfig contient la configuration Ntfy
type NtfyConfig struct {
	Name       string
	URL        string
	Topic      string
	Token      string
	Severities []internal.Severity
}

// NewNtfyNotifier crée un nouveau notifier Ntfy
func NewNtfyNotifier(config NtfyConfig) *NtfyNotifier {
	return &NtfyNotifier{
		name:       config.Name,
		url:        config.URL,
		topic:      config.Topic,
		token:      config.Token,
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

	// Priorité selon la sévérité
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
		return fmt.Errorf("sérialisation du payload ntfy: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("création de la requête ntfy: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Authentification Bearer si un token est configuré
	if n.token != "" {
		req.Header.Set("Authorization", "Bearer "+n.token)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("envoi de la notification ntfy: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("statut inattendu ntfy: %d", resp.StatusCode)
	}

	return nil
}

// severityTags return les tags emoji Ntfy selon la sévérité
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
