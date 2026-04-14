package internal

import "time"

// Metric représente un point de donnée métrique
type Metric struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Source    string            `json:"source"`
}

// Point représente un point de données temporelles pour les requêtes
type Point struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Min       float64   `json:"min,omitempty"`
	Max       float64   `json:"max,omitempty"`
	Avg       float64   `json:"avg,omitempty"`
}

// Alert représente une alerte active ou historique
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

// Resolution définit la granularité des séries temporelles
type Resolution string

const (
	ResolutionRaw    Resolution = "raw"    // 30s
	Resolution5Min   Resolution = "5min"   // 5 minutes
	ResolutionHourly Resolution = "hourly" // 1 heure
)

// HealthStatus représente l'état de santé du système
type HealthStatus string

const (
	HealthOK       HealthStatus = "ok"
	HealthWarning  HealthStatus = "warning"
	HealthCritical HealthStatus = "critical"
	HealthUnknown  HealthStatus = "unknown"
)

// LogEntry représente une entrée de log structurée provenant de n'importe quelle source
type LogEntry struct {
	Timestamp time.Time      `json:"timestamp"`
	Source    string         `json:"source"`   // "ssh", "nftables", "ufw", "docker", "crowdsec"
	Machine  string         `json:"machine"`
	Severity string         `json:"severity"` // "info", "warning", "error", "critical"
	Message  string         `json:"message"`  // ligne brute, tronquée à 500 caractères
	Parsed   map[string]any `json:"parsed"`   // champs extraits (IP, port, user, action, container...)
}

// LogStats contient les compteurs de logs par source et sévérité
type LogStats struct {
	BySource   map[string]int `json:"by_source"`
	BySeverity map[string]int `json:"by_severity"`
}

// SoftwareItem représente un logiciel détecté sur une machine.
type SoftwareItem struct {
	ID        int64     `json:"id"`
	Machine   string    `json:"machine"`
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Source    string    `json:"source"`    // "docker", "binaire", "paquet"
	Container string    `json:"container,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// VulnMatch représente une CVE correspondant à un logiciel installé.
type VulnMatch struct {
	ID               int64      `json:"id"`
	CVEID            string     `json:"cve_id"`
	Severity         string     `json:"severity"`
	CVSSScore        float64    `json:"cvss_score"`
	Title            string     `json:"title"`
	Link             string     `json:"link"`
	MatchedSoftware  string     `json:"matched_software"`
	Machine          string     `json:"machine"`
	InstalledVersion string     `json:"installed_version"`
	Confidence       string     `json:"confidence"` // confirmé, probable, obsolète
	VeilleAlertID    int64      `json:"veille_alert_id"`
	CISAKEV          bool       `json:"cisa_kev"`
	FirstSeen        time.Time  `json:"first_seen"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
	Dismissed        bool       `json:"dismissed"`
}

// Status représente le statut global du système
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
