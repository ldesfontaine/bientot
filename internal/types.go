package internal

import "time"

// Metric represents a single metric data point
type Metric struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Source    string            `json:"source"`
}

// Point represents a time-series data point for queries
type Point struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Min       float64   `json:"min,omitempty"`
	Max       float64   `json:"max,omitempty"`
	Avg       float64   `json:"avg,omitempty"`
}

// Alert represents an active or historical alert
type Alert struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Severity  Severity          `json:"severity"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels,omitempty"`
	Value     float64           `json:"value"`
	FiredAt   time.Time         `json:"fired_at"`
	ResolvedAt *time.Time       `json:"resolved_at,omitempty"`
	Acknowledged bool           `json:"acknowledged"`
}

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// Resolution defines time-series granularity
type Resolution string

const (
	ResolutionRaw    Resolution = "raw"    // 30s
	Resolution5Min   Resolution = "5min"   // 5 minutes
	ResolutionHourly Resolution = "hourly" // 1 hour
)

// HealthStatus represents system health
type HealthStatus string

const (
	HealthOK       HealthStatus = "ok"
	HealthWarning  HealthStatus = "warning"
	HealthCritical HealthStatus = "critical"
	HealthUnknown  HealthStatus = "unknown"
)

// Status represents the global system status
type Status struct {
	Health      HealthStatus     `json:"health"`
	Collectors  []CollectorStatus `json:"collectors"`
	AlertsActive int             `json:"alerts_active"`
	LastScrape  time.Time        `json:"last_scrape"`
	Uptime      time.Duration    `json:"uptime"`
}

type CollectorStatus struct {
	Name      string       `json:"name"`
	Type      string       `json:"type"`
	Health    HealthStatus `json:"health"`
	LastRun   time.Time    `json:"last_run"`
	Error     string       `json:"error,omitempty"`
}
