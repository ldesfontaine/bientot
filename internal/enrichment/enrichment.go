package enrichment

import "time"

// IPIntel holds enrichment data for a single IP.
type IPIntel struct {
	IP            string    `json:"ip"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	TotalRequests int       `json:"total_requests"`

	// GeoIP
	Country string  `json:"country,omitempty"`
	City    string  `json:"city,omitempty"`
	Lat     float64 `json:"lat,omitempty"`
	Lon     float64 `json:"lon,omitempty"`
	ASN     int     `json:"asn,omitempty"`
	ISP     string  `json:"isp,omitempty"`

	// Blocklists
	BlocklistsMatched []string `json:"blocklists_matched,omitempty"`

	// CrowdSec correlation
	CrowdSecBanned bool   `json:"crowdsec_banned"`
	CrowdSecReason string `json:"crowdsec_reason,omitempty"`

	// API enrichment
	AbuseScore     int    `json:"abuse_score,omitempty"`
	GreyNoiseClass string `json:"greynoise_class,omitempty"` // benign, malicious, unknown
	GreyNoiseName  string `json:"greynoise_name,omitempty"`

	// Meta
	EnrichmentSources []string  `json:"enrichment_sources,omitempty"`
	EnrichedAt        time.Time `json:"enriched_at"`
	PriorityScore     int       `json:"priority_score"`
}

// AttackLog represents an aggregated attack event.
type AttackLog struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Machine   string    `json:"machine"`
	IP        string    `json:"ip"`
	Path      string    `json:"path"`
	Status    int       `json:"status_code"`
	Count     int       `json:"count"`
}

// Pattern represents a detected threat pattern.
type Pattern struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`     // scan_distributed, new_path, ip_unblocked, burst
	Severity  string    `json:"severity"` // info, warning, critical
	Detail    string    `json:"detail"`
	Resolved  bool      `json:"resolved"`
}

// Provider enriches a single IP via an external API.
type Provider interface {
	Name() string
	Enrich(ip string) (*ProviderResult, error)
	DailyLimit() int
}

// ProviderResult holds data returned by a provider.
type ProviderResult struct {
	Source string            `json:"source"`
	Data   map[string]string `json:"data"`
	Score  int               `json:"score"` // provider-specific risk score (0-100)
}

// GeoResult holds GeoIP lookup data.
type GeoResult struct {
	Country string
	City    string
	Lat     float64
	Lon     float64
	ASN     int
	ISP     string
}
