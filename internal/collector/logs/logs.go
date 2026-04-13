package logs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ldesfontaine/bientot/internal"
	"github.com/ldesfontaine/bientot/internal/storage"
)

const maxEntriesPerCollect = 200

// LogsCollector implements the collector.Collector interface.
// It auto-detects available log sources and collects structured log entries,
// writing them to storage and returning summary metrics.
type LogsCollector struct {
	machine  string
	interval time.Duration
	sources  []Source
	store    storage.Storage
	logger   *slog.Logger
}

// Config holds configuration for the logs collector.
type Config struct {
	Enabled      bool          `yaml:"enabled"`
	Machine      string        `yaml:"machine"`
	Interval     time.Duration `yaml:"interval"`
	DockerSocket string        `yaml:"docker_socket"`
	CrowdSecURL  string        `yaml:"crowdsec_url"`
}

// New creates a LogsCollector. It probes all possible sources
// and keeps only those that are available on this machine.
func New(cfg Config, store storage.Storage, logger *slog.Logger) *LogsCollector {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Minute
	}
	if cfg.Machine == "" {
		cfg.Machine = "unknown"
	}

	candidates := buildCandidates(cfg)

	var active []Source
	for _, s := range candidates {
		if s.Available() {
			logger.Info("logs source detected", "source", s.Name())
			active = append(active, s)
		} else {
			logger.Debug("logs source not available", "source", s.Name())
		}
	}

	return &LogsCollector{
		machine:  cfg.Machine,
		interval: cfg.Interval,
		sources:  active,
		store:    store,
		logger:   logger,
	}
}

func buildCandidates(cfg Config) []Source {
	var candidates []Source

	// Journald sources
	candidates = append(candidates,
		NewJournaldSSHSource(),
		NewJournaldNftablesSource(),
	)

	// UFW: prefer journald, fall back to file
	ufwJournald := NewJournaldUFWSource()
	if ufwJournald.Available() {
		candidates = append(candidates, ufwJournald)
	} else {
		candidates = append(candidates, NewFileUFWSource())
	}

	// Docker
	if cfg.DockerSocket != "" {
		candidates = append(candidates, NewDockerSource(cfg.DockerSocket))
	}

	// CrowdSec
	if cfg.CrowdSecURL != "" {
		candidates = append(candidates, NewCrowdSecSource(cfg.CrowdSecURL))
	}

	return candidates
}

// --- collector.Collector interface ---

func (c *LogsCollector) Name() string            { return "logs" }
func (c *LogsCollector) Type() string            { return "logs" }
func (c *LogsCollector) Interval() time.Duration { return c.interval }

// Collect gathers log entries from all active sources, writes them to storage,
// and returns summary metrics (entry counts by source).
func (c *LogsCollector) Collect(ctx context.Context) ([]internal.Metric, error) {
	var allEntries []internal.LogEntry

	for _, src := range c.sources {
		entries, err := src.Collect(ctx, c.machine)
		if err != nil {
			c.logger.Warn("log source collect failed", "source", src.Name(), "error", err)
			continue
		}
		allEntries = append(allEntries, entries...)
	}

	// Cap at maxEntriesPerCollect (keep most recent)
	if len(allEntries) > maxEntriesPerCollect {
		allEntries = allEntries[len(allEntries)-maxEntriesPerCollect:]
	}

	// Write to storage
	if len(allEntries) > 0 {
		if err := c.store.InsertLogs(ctx, allEntries); err != nil {
			return nil, fmt.Errorf("storing log entries: %w", err)
		}
	}

	// Build summary metrics
	now := time.Now()
	counts := map[string]int{}
	severityCounts := map[string]int{}
	for _, e := range allEntries {
		counts[e.Source]++
		severityCounts[e.Severity]++
	}

	var metrics []internal.Metric
	metrics = append(metrics, internal.Metric{
		Name:      "log_entries_total",
		Value:     float64(len(allEntries)),
		Timestamp: now,
		Source:    "logs",
	})

	for source, count := range counts {
		metrics = append(metrics, internal.Metric{
			Name:      "log_entries_by_source",
			Value:     float64(count),
			Labels:    map[string]string{"source": source},
			Timestamp: now,
			Source:    "logs",
		})
	}

	for sev, count := range severityCounts {
		metrics = append(metrics, internal.Metric{
			Name:      "log_entries_by_severity",
			Value:     float64(count),
			Labels:    map[string]string{"severity": sev},
			Timestamp: now,
			Source:    "logs",
		})
	}

	c.logger.Debug("logs collected", "entries", len(allEntries), "sources", len(c.sources))
	return metrics, nil
}

// SourceNames returns the names of active sources (for health/status).
func (c *LogsCollector) SourceNames() []string {
	names := make([]string, len(c.sources))
	for i, s := range c.sources {
		names[i] = s.Name()
	}
	return names
}
