package veille

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ldesfontaine/bientot/internal"
	"github.com/ldesfontaine/bientot/internal/storage"
)

// SyncerConfig configures the veille-secu sync loop.
type SyncerConfig struct {
	PollInterval   time.Duration
	SyncTools      bool     // if true, push inventory to veille-secu
	SeverityFilter []string // only process these severities
}

// Syncer periodically fetches veille-secu alerts and correlates with inventory.
type Syncer struct {
	client   *Client
	store    *storage.SQLiteStorage
	cfg      SyncerConfig
	logger   *slog.Logger
	onMatch  func(internal.VulnMatch) // callback for new matches (alerting)
}

// NewSyncer creates a veille-secu syncer.
func NewSyncer(client *Client, store *storage.SQLiteStorage, cfg SyncerConfig, logger *slog.Logger) *Syncer {
	return &Syncer{
		client: client,
		store:  store,
		cfg:    cfg,
		logger: logger,
	}
}

// OnMatch sets a callback fired when a new CVE match is found.
func (s *Syncer) OnMatch(fn func(internal.VulnMatch)) {
	s.onMatch = fn
}

// Run starts the sync loop. Blocks until ctx is cancelled.
func (s *Syncer) Run(ctx context.Context) {
	interval := s.cfg.PollInterval
	if interval == 0 {
		interval = 15 * time.Minute
	}

	s.logger.Info("veille syncer started", "interval", interval, "sync_tools", s.cfg.SyncTools)

	// Initial sync
	s.sync(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("veille syncer stopping")
			return
		case <-ticker.C:
			s.sync(ctx)
		}
	}
}

func (s *Syncer) sync(ctx context.Context) {
	// Check health first
	if err := s.client.Health(); err != nil {
		s.logger.Warn("veille-secu unreachable, skipping sync", "error", err)
		s.store.InsertSyncLog(ctx, 0, 0, "unreachable")
		return
	}

	// Fetch new alerts
	alerts, err := s.client.FetchAlerts("new", s.cfg.SeverityFilter, 200)
	if err != nil {
		s.logger.Error("failed to fetch veille alerts", "error", err)
		s.store.InsertSyncLog(ctx, 0, 0, fmt.Sprintf("error: %v", err))
		return
	}

	s.logger.Debug("veille alerts fetched", "count", len(alerts))

	// Correlate with software inventory
	matchCount := 0
	for _, alert := range alerts {
		matches := s.correlate(ctx, alert)
		matchCount += len(matches)
	}

	s.store.InsertSyncLog(ctx, len(alerts), matchCount, "ok")
	s.logger.Info("veille sync complete", "alerts", len(alerts), "matches", matchCount)

	// Optionally sync tools back to veille-secu
	if s.cfg.SyncTools {
		s.syncToolsToVeille(ctx)
	}
}

// correlate checks if an alert's matched_tools appear in software_inventory.
func (s *Syncer) correlate(ctx context.Context, alert Alert) []internal.VulnMatch {
	var matches []internal.VulnMatch

	for _, toolName := range alert.MatchedTools {
		// Find all machines with this software
		items, err := s.store.FindSoftwareByName(ctx, toolName)
		if err != nil {
			s.logger.Warn("software lookup failed", "tool", toolName, "error", err)
			continue
		}

		for _, item := range items {
			confidence := determineConfidence(alert, item)

			match := internal.VulnMatch{
				CVEID:            alert.CVEID,
				Severity:         alert.Severity,
				CVSSScore:        alert.CVSSScore,
				Title:            alert.Title,
				Link:             alert.Link,
				MatchedSoftware:  toolName,
				Machine:          item.Machine,
				InstalledVersion: item.Version,
				Confidence:       confidence,
				VeilleAlertID:    alert.ID,
				CISAKEV:          isCISAKEV(alert),
				FirstSeen:        time.Now(),
			}

			if err := s.store.UpsertVulnMatch(ctx, &match); err != nil {
				s.logger.Warn("vuln match upsert failed", "cve", alert.CVEID, "machine", item.Machine, "error", err)
				continue
			}

			matches = append(matches, match)

			if s.onMatch != nil {
				s.onMatch(match)
			}
		}
	}

	return matches
}

// determineConfidence classifies the match quality.
func determineConfidence(alert Alert, item internal.SoftwareItem) string {
	// Check if the CVE description mentions a specific version range
	desc := strings.ToLower(alert.Title + " " + alert.Description)
	version := strings.ToLower(item.Version)

	// If version is explicitly mentioned in the alert text → confirmed
	if version != "" && version != "latest" && strings.Contains(desc, version) {
		return "confirmed"
	}

	// If version is very old (heuristic: "latest" or empty) → likely
	if version == "latest" || version == "" {
		return "likely"
	}

	// Check for "before X.Y.Z" patterns — if our version is mentioned,
	// the alert probably applies to versions before a fixed version
	if strings.Contains(desc, "before") || strings.Contains(desc, "prior to") {
		return "likely"
	}

	// Default: tool matched but no version confirmation
	return "likely"
}

// isCISAKEV checks if the alert comes from CISA KEV source.
func isCISAKEV(alert Alert) bool {
	return alert.SourceID == "cisa-kev" || strings.Contains(strings.ToLower(alert.SourceName), "kev")
}

// syncToolsToVeille pushes the bientot software inventory to veille-secu as tools.
func (s *Syncer) syncToolsToVeille(ctx context.Context) {
	items, err := s.store.QuerySoftware(ctx, "")
	if err != nil {
		s.logger.Warn("failed to query software inventory for sync", "error", err)
		return
	}

	// Deduplicate by name (only push unique software names)
	seen := make(map[string]bool)
	synced := 0

	for _, item := range items {
		if seen[item.Name] {
			continue
		}
		seen[item.Name] = true

		tool := Tool{
			Name:     item.Name,
			Keywords: []string{item.Name},
			Version:  item.Version,
			Source:   "bientot-auto",
		}

		if err := s.client.AddTool(tool); err != nil {
			s.logger.Debug("tool sync failed", "name", item.Name, "error", err)
			continue
		}
		synced++
	}

	if synced > 0 {
		s.logger.Info("tools synced to veille-secu", "count", synced)
	}
}
