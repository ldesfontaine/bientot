package transport

import "time"

// Payload est l'enveloppe envoyée par les agents au serveur.
// Le serveur valide MachineID par rapport au token, vérifie la fraîcheur du Timestamp,
// rejette les Nonce dupliqués, et vérifie la Signature (HMAC-SHA256 sur Body).
type Payload struct {
	MachineID string    `json:"machine_id"`
	Timestamp time.Time `json:"timestamp"`
	Nonce     string    `json:"nonce"`
	Signature string    `json:"signature"` // hex-encoded HMAC-SHA256
	Body      Body      `json:"body"`
}

// Body contient les données collectées par l'agent.
type Body struct {
	Modules []ModuleData `json:"modules"`
}

// ModuleData contient les métriques d'un module agent.
type ModuleData struct {
	Module    string            `json:"module"`
	Metrics   []MetricPoint     `json:"metrics"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Error     string            `json:"error,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// MetricPoint est une métrique collectée par un module.
type MetricPoint struct {
	Name   string            `json:"name"`
	Value  float64           `json:"value"`
	Labels map[string]string `json:"labels,omitempty"`
}
