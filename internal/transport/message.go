package transport

import "time"

// Payload is the envelope sent by agents to the server.
// The server validates MachineID against the token, checks Timestamp freshness,
// rejects duplicate Nonce, and verifies Signature (HMAC-SHA256 over Body).
type Payload struct {
	MachineID string    `json:"machine_id"`
	Timestamp time.Time `json:"timestamp"`
	Nonce     string    `json:"nonce"`
	Signature string    `json:"signature"` // hex-encoded HMAC-SHA256
	Body      Body      `json:"body"`
}

// Body carries the actual data collected by the agent.
type Body struct {
	Modules []ModuleData `json:"modules"`
}

// ModuleData holds metrics from a single agent module.
type ModuleData struct {
	Module    string            `json:"module"`
	Metrics   []MetricPoint     `json:"metrics"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Error     string            `json:"error,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// MetricPoint is a single metric collected by a module.
type MetricPoint struct {
	Name   string            `json:"name"`
	Value  float64           `json:"value"`
	Labels map[string]string `json:"labels,omitempty"`
}
