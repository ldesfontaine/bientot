package modules

import (
	"context"
	"time"
)

// Module is the interface every agent collector module must implement.
type Module interface {
	// Name returns the unique identifier of the module (e.g. "heartbeat", "system").
	Name() string

	// Detect returns nil if the module can run on this machine, otherwise
	// an error describing why its prerequisites are not met.
	Detect(ctx context.Context) error

	// Collect gathers the module's data. The returned error must be real —
	// callers log and surface it, there is no silent swallowing.
	Collect(ctx context.Context) (*Data, error)

	// Interval returns the recommended collection period for this module.
	Interval() time.Duration
}

// Data is the payload produced by a single Collect call.
type Data struct {
	Module    string
	Metrics   []Metric
	Metadata  map[string]string
	Software  []SoftwareItem
	RawEvents []RawEvent
	Timestamp time.Time
}

// Metric is a single numeric data point with Prometheus-style labels.
type Metric struct {
	Name   string
	Value  float64
	Labels map[string]string
}

// SoftwareItem describes one installed piece of software detected by a module.
// Source indicates where the detection came from (e.g. "docker", "apt", "binary").
type SoftwareItem struct {
	Name    string
	Version string
	Source  string
}

// RawEvent is an untyped event forwarded by a module for future CTI processing.
// Fields is intentionally loose — each module populates what is relevant.
type RawEvent struct {
	Source    string
	Timestamp time.Time
	Fields    map[string]any
}
