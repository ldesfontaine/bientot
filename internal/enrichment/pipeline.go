package enrichment

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Pipeline orchestrates IP enrichment: dedup → score → local → API → store.
type Pipeline struct {
	geoip      *GeoIP             // nil = disabled
	blocklists *BlocklistChecker   // nil = disabled
	providers  []Provider
	budget     *BudgetTracker
	store      Store
	cache      *dedupCache
	logger     *slog.Logger
}

// Store is the interface for enrichment persistence.
type Store interface {
	UpsertIPIntel(ctx context.Context, intel *IPIntel) error
	GetIPIntel(ctx context.Context, ip string) (*IPIntel, error)
	InsertAttackLog(ctx context.Context, log *AttackLog) error
	InsertPattern(ctx context.Context, pattern *Pattern) error
	QueryIPIntel(ctx context.Context, limit int) ([]IPIntel, error)
	QueryAttackLogs(ctx context.Context, since time.Time, limit int) ([]AttackLog, error)
	QueryPatterns(ctx context.Context, unresolvedOnly bool, limit int) ([]Pattern, error)
	BudgetStatus(ctx context.Context) (map[string]map[string]int, error)
}

// PipelineConfig holds pipeline configuration.
type PipelineConfig struct {
	GeoIPPath        string
	BlocklistSources []BlocklistSource
	Providers        []Provider
	BudgetLimits     map[string]int
}

// NewPipeline creates a fully configured enrichment pipeline.
func NewPipeline(cfg PipelineConfig, store Store, logger *slog.Logger) (*Pipeline, error) {
	p := &Pipeline{
		providers: cfg.Providers,
		store:     store,
		cache:     newDedupCache(24 * time.Hour),
		logger:    logger,
	}

	// GeoIP (optional)
	if cfg.GeoIPPath != "" {
		geoip, err := NewGeoIP(cfg.GeoIPPath)
		if err != nil {
			logger.Warn("geoip disabled", "error", err)
		} else {
			p.geoip = geoip
			logger.Info("geoip enabled", "db", cfg.GeoIPPath)
		}
	}

	// Blocklists (optional)
	if len(cfg.BlocklistSources) > 0 {
		p.blocklists = NewBlocklistChecker(cfg.BlocklistSources, logger)
		if err := p.blocklists.Load(); err != nil {
			logger.Warn("blocklists initial load failed", "error", err)
		}
		logger.Info("blocklists enabled", "sources", len(cfg.BlocklistSources), "entries", p.blocklists.Count())
	}

	// Budget
	if len(cfg.BudgetLimits) > 0 {
		p.budget = NewBudgetTracker(cfg.BudgetLimits)
	}

	return p, nil
}

// EnrichIP runs the full pipeline for a single IP.
func (p *Pipeline) EnrichIP(ctx context.Context, ip string, reqCount int, paths []string, crowdsecBanned bool) (*IPIntel, error) {
	// Dedup: skip if already enriched in the last 24h
	if p.cache.seen(ip) {
		return p.store.GetIPIntel(ctx, ip)
	}
	p.cache.mark(ip)

	intel := &IPIntel{
		IP:             ip,
		FirstSeen:      time.Now(),
		LastSeen:       time.Now(),
		TotalRequests:  reqCount,
		CrowdSecBanned: crowdsecBanned,
		EnrichedAt:     time.Now(),
	}

	// Check if we already have data
	existing, _ := p.store.GetIPIntel(ctx, ip)
	if existing != nil {
		intel.FirstSeen = existing.FirstSeen
		intel.TotalRequests = existing.TotalRequests + reqCount
	}

	// Local enrichment (always, free)
	p.enrichLocal(ip, intel)

	// Score
	inBlocklist := len(intel.BlocklistsMatched) > 0
	intel.PriorityScore = ScoreIP(reqCount, paths, inBlocklist, crowdsecBanned)

	// API enrichment (only if score > 0 and budget available)
	if intel.PriorityScore > 0 {
		p.enrichAPI(ip, intel)
	}

	// Persist
	if err := p.store.UpsertIPIntel(ctx, intel); err != nil {
		return nil, err
	}

	return intel, nil
}

// enrichLocal runs GeoIP + blocklists + CrowdSec correlation.
func (p *Pipeline) enrichLocal(ip string, intel *IPIntel) {
	// GeoIP
	if p.geoip != nil {
		if geo, err := p.geoip.Lookup(ip); err == nil {
			intel.Country = geo.Country
			intel.City = geo.City
			intel.Lat = geo.Lat
			intel.Lon = geo.Lon
			intel.ASN = geo.ASN
			intel.ISP = geo.ISP
			intel.EnrichmentSources = append(intel.EnrichmentSources, "geoip")
		}
	}

	// Blocklists
	if p.blocklists != nil {
		matched := p.blocklists.Check(ip)
		if len(matched) > 0 {
			intel.BlocklistsMatched = matched
			intel.EnrichmentSources = append(intel.EnrichmentSources, "blocklist")
		}
	}
}

// enrichAPI queries external providers respecting budget.
func (p *Pipeline) enrichAPI(ip string, intel *IPIntel) {
	for _, prov := range p.providers {
		if p.budget != nil && !p.budget.CanSpend(prov.Name()) {
			continue
		}

		result, err := prov.Enrich(ip)
		if err != nil {
			p.logger.Warn("provider enrichment failed", "provider", prov.Name(), "ip", ip, "error", err)
			continue
		}

		if p.budget != nil {
			p.budget.Spend(prov.Name())
		}

		// Apply provider results
		switch prov.Name() {
		case "abuseipdb":
			intel.AbuseScore = result.Score
		case "greynoise":
			intel.GreyNoiseClass = result.Data["classification"]
			intel.GreyNoiseName = result.Data["name"]
		case "crowdsec_cti":
			// CrowdSec CTI data supplements local CrowdSec data
			if result.Data["reputation"] == "malicious" && !intel.CrowdSecBanned {
				intel.CrowdSecReason = "cti:" + result.Data["behaviors"]
			}
		}

		intel.EnrichmentSources = append(intel.EnrichmentSources, prov.Name())
	}
}

// StartBlocklistRefresh starts periodic blocklist updates.
func (p *Pipeline) StartBlocklistRefresh(ctx context.Context, interval time.Duration) {
	if p.blocklists != nil {
		go p.blocklists.StartAutoRefresh(ctx, interval)
	}
}

// BudgetStatus returns current API budget state.
func (p *Pipeline) BudgetStatus() map[string]map[string]int {
	if p.budget == nil {
		return nil
	}
	return p.budget.Status()
}

// Close releases pipeline resources.
func (p *Pipeline) Close() error {
	if p.geoip != nil {
		return p.geoip.Close()
	}
	return nil
}

// dedupCache tracks recently enriched IPs to avoid redundant work.
type dedupCache struct {
	mu  sync.Mutex
	ips map[string]time.Time
	ttl time.Duration
}

func newDedupCache(ttl time.Duration) *dedupCache {
	return &dedupCache{
		ips: make(map[string]time.Time),
		ttl: ttl,
	}
}

func (c *dedupCache) seen(ip string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if t, ok := c.ips[ip]; ok && time.Since(t) < c.ttl {
		return true
	}
	return false
}

func (c *dedupCache) mark(ip string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ips[ip] = time.Now()
}
