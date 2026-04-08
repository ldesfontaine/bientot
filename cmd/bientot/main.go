package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ldesfontaine/bientot/internal"
	"github.com/ldesfontaine/bientot/internal/alerter"
	"github.com/ldesfontaine/bientot/internal/api"
	"github.com/ldesfontaine/bientot/internal/collector"
	"github.com/ldesfontaine/bientot/internal/config"
	"github.com/ldesfontaine/bientot/internal/notifier"
	"github.com/ldesfontaine/bientot/internal/storage"
	"github.com/ldesfontaine/bientot/internal/web"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	targetsPath := getEnv("TARGETS_PATH", "config/targets.yml")
	alertsPath := getEnv("ALERTS_PATH", "config/alerts.yml")

	targets, err := config.LoadTargets(targetsPath)
	if err != nil {
		logger.Error("failed to load targets", "error", err)
		os.Exit(1)
	}

	alertsCfg, err := config.LoadAlerts(alertsPath)
	if err != nil {
		logger.Error("failed to load alerts", "error", err)
		os.Exit(1)
	}

	// Initialize storage
	storeCfg := storage.Config{
		DBPath:        getEnv("DB_PATH", "/data/metrics.db"),
		RetentionDays: 90,
	}
	store, err := storage.NewSQLiteStorage(storeCfg)
	if err != nil {
		logger.Error("failed to initialize storage", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Initialize collectors
	collectors := collector.NewRegistry()
	registerCollectors(collectors, targets)

	// Initialize notifiers
	notifiers := notifier.NewRegistry()
	registerNotifiers(notifiers, alertsCfg)

	// Initialize alerter
	rules, _ := config.ParseRules(alertsCfg.Alerts)
	alert := alerter.New(rules, store, notifiers, logger)

	// Start scraper loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runScrapeLoop(ctx, collectors, store, logger)
	go runAlertLoop(ctx, alert, logger)
	go runMaintenanceLoop(ctx, store, logger)

	// Initialize API
	apiHandler := api.New(store, alert)

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.Handle("/", web.Handler())
	mux.Handle("/health", apiHandler.Router())
	mux.Handle("/api/", apiHandler.Router())

	port := getEnv("WEB_PORT", "3001")
	bind := getEnv("WEB_BIND", "0.0.0.0")
	addr := bind + ":" + port

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		logger.Info("shutting down...")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)
	}()

	logger.Info("starting server", "addr", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func registerCollectors(registry *collector.Registry, cfg *config.TargetsConfig) {
	// Prometheus collectors
	for _, target := range cfg.Collectors.Prometheus {
		c := collector.NewPrometheusCollector(target.Name, target.URL, target.Interval)
		registry.Register(c)
	}

	// CrowdSec collectors
	for _, target := range cfg.Collectors.CrowdSec {
		c := collector.NewCrowdSecCollector(target.Name, target.URL, target.Interval)
		registry.Register(c)
	}

	// Docker collector
	if cfg.Collectors.Docker.Enabled {
		c := collector.NewDockerCollector("docker", cfg.Collectors.Docker.Socket, cfg.Collectors.Docker.Interval)
		registry.Register(c)
	}

	// ZFS collector
	if cfg.Collectors.ZFS.Enabled {
		c := collector.NewZFSCollector("zfs", cfg.Collectors.ZFS.Pools, cfg.Collectors.ZFS.Interval)
		registry.Register(c)
	}

	// JSON file collectors
	for _, target := range cfg.Collectors.JSONFile {
		c := collector.NewJSONFileCollector(target.Name, target.Path, target.Interval)
		registry.Register(c)
	}
}

func registerNotifiers(registry *notifier.Registry, cfg *config.AlertsConfig) {
	for _, n := range cfg.Notifiers {
		switch n.Type {
		case "ntfy":
			severities := parseSeverities(n.SeverityFilter)
			notif := notifier.NewNtfyNotifier(notifier.NtfyConfig{
				Name:       "ntfy",
				URL:        n.URL,
				Topic:      n.Topic,
				Severities: severities,
			})
			registry.Register(notif)
		case "webhook":
			severities := parseSeverities(n.SeverityFilter)
			notif := notifier.NewWebhookNotifier(notifier.WebhookConfig{
				Name:       "webhook",
				URL:        n.URL,
				Headers:    n.Headers,
				Severities: severities,
			})
			registry.Register(notif)
		}
	}
}

func parseSeverities(filters []string) []internal.Severity {
	var severities []internal.Severity
	for _, f := range filters {
		switch f {
		case "critical":
			severities = append(severities, internal.SeverityCritical)
		case "warning":
			severities = append(severities, internal.SeverityWarning)
		case "info":
			severities = append(severities, internal.SeverityInfo)
		}
	}
	return severities
}

func runScrapeLoop(ctx context.Context, collectors *collector.Registry, store storage.Storage, logger *slog.Logger) {
	// Launch one goroutine per collector with its own interval
	for _, c := range collectors.All() {
		go runCollector(ctx, c, store, logger)
	}
	<-ctx.Done()
}

func runCollector(ctx context.Context, c collector.Collector, store storage.Storage, logger *slog.Logger) {
	interval := c.Interval()
	if interval <= 0 {
		interval = 30 * time.Second
	}

	// Initial scrape
	scrapeOne(ctx, c, store, logger)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scrapeOne(ctx, c, store, logger)
		}
	}
}

func scrapeOne(ctx context.Context, c collector.Collector, store storage.Storage, logger *slog.Logger) {
	metrics, err := c.Collect(ctx)
	if err != nil {
		logger.Error("scrape failed", "collector", c.Name(), "error", err)
		return
	}

	if err := store.Write(ctx, metrics); err != nil {
		logger.Error("write failed", "collector", c.Name(), "error", err)
	}

	logger.Debug("scraped", "collector", c.Name(), "metrics", len(metrics))
}

func runAlertLoop(ctx context.Context, alert *alerter.Alerter, logger *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := alert.Evaluate(ctx); err != nil {
				logger.Error("alert evaluation failed", "error", err)
			}
		}
	}
}

func runMaintenanceLoop(ctx context.Context, store storage.Storage, logger *slog.Logger) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := store.Downsample(ctx); err != nil {
				logger.Error("downsampling failed", "error", err)
			}
			if err := store.Cleanup(ctx); err != nil {
				logger.Error("cleanup failed", "error", err)
			}
		}
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
