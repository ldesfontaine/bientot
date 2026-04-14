package enrichment

import "time"

// IPIntel contient les données d'enrichissement pour une IP.
type IPIntel struct {
	IP            string    `json:"ip"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	TotalRequests int       `json:"total_requests"`

	// GeoIP (géolocalisation)
	Country string  `json:"country,omitempty"`
	City    string  `json:"city,omitempty"`
	Lat     float64 `json:"lat,omitempty"`
	Lon     float64 `json:"lon,omitempty"`
	ASN     int     `json:"asn,omitempty"`
	ISP     string  `json:"isp,omitempty"`

	// Blocklists (listes de blocage)
	BlocklistsMatched []string `json:"blocklists_matched,omitempty"`

	// Corrélation CrowdSec
	CrowdSecBanned bool   `json:"crowdsec_banned"`
	CrowdSecReason string `json:"crowdsec_reason,omitempty"`

	// Enrichissement API
	AbuseScore     int    `json:"abuse_score,omitempty"`
	GreyNoiseClass string `json:"greynoise_class,omitempty"` // bénin, malveillant, inconnu
	GreyNoiseName  string `json:"greynoise_name,omitempty"`

	// Métadonnées
	EnrichmentSources []string  `json:"enrichment_sources,omitempty"`
	EnrichedAt        time.Time `json:"enriched_at"`
	PriorityScore     int       `json:"priority_score"`
}

// AttackLog représente un événement d'attaque agrégé.
type AttackLog struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Machine   string    `json:"machine"`
	IP        string    `json:"ip"`
	Path      string    `json:"path"`
	Status    int       `json:"status_code"`
	Count     int       `json:"count"`
}

// Pattern représente un schéma de menace détecté.
type Pattern struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`     // scan_distribué, nouveau_chemin, ip_débloquée, rafale
	Severity  string    `json:"severity"` // info, avertissement, critique
	Detail    string    `json:"detail"`
	Resolved  bool      `json:"resolved"`
}

// Provider enrichit une IP via une API externe.
type Provider interface {
	Name() string
	Enrich(ip string) (*ProviderResult, error)
	DailyLimit() int
}

// ProviderResult contient les données retournées par un fournisseur.
type ProviderResult struct {
	Source string            `json:"source"`
	Data   map[string]string `json:"data"`
	Score  int               `json:"score"` // score de risque spécifique au fournisseur (0-100)
}

// GeoResult contient les données de recherche GeoIP.
type GeoResult struct {
	Country string
	City    string
	Lat     float64
	Lon     float64
	ASN     int
	ISP     string
}
