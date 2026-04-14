package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ldesfontaine/bientot/internal"
	"github.com/ldesfontaine/bientot/internal/alerter"
	"github.com/ldesfontaine/bientot/internal/config"
	"github.com/ldesfontaine/bientot/internal/enrichment"
	"github.com/ldesfontaine/bientot/internal/enrichment/providers"
	"github.com/ldesfontaine/bientot/internal/notifier"
	"github.com/ldesfontaine/bientot/internal/server"
	"github.com/ldesfontaine/bientot/internal/storage"
	"github.com/ldesfontaine/bientot/internal/veille"
)

func main() {
	// Healthcheck subcommand: HTTP GET on dashboard /health endpoint
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		addr := getEnv("DASHBOARD_ADDR", "0.0.0.0:3001")
		// Extract port from addr for localhost check
		port := addr
		if i := strings.LastIndex(addr, ":"); i >= 0 {
			port = addr[i+1:]
		}
		resp, err := http.Get("http://localhost:" + port + "/health")
		if err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "healthcheck: status %d\n", resp.StatusCode)
			os.Exit(1)
		}
		fmt.Println("OK")
		os.Exit(0)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(getEnv("LOG_LEVEL", "info")),
	}))
	slog.SetDefault(logger)

	// Parse agent tokens from env: BIENTOT_AGENTS="machine1:token1,machine2:token2"
	agents := parseAgentTokens(requireEnv("BIENTOT_AGENTS"))
	if len(agents) == 0 {
		logger.Error("no agent tokens configured")
		os.Exit(1)
	}

	storeCfg := storage.Config{
		DBPath:        getEnv("DB_PATH", "/data/bientot.db"),
		RetentionDays: 90,
	}
	store, err := storage.NewSQLiteStorage(storeCfg)
	if err != nil {
		logger.Error("failed to initialize storage", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	cfg := server.Config{
		DashboardAddr: getEnv("DASHBOARD_ADDR", "0.0.0.0:3001"),
		AgentAddr:     getEnv("AGENT_ADDR", "0.0.0.0:3002"),
		DBPath:        storeCfg.DBPath,
		RetentionDays: storeCfg.RetentionDays,
		Agents:        agents,
	}

	srv := server.New(cfg, store, logger)

	// Wire alerter + notifier if alerts.yml exists
	alertsPath := getEnv("ALERTS_CONFIG", "config/alerts.yml")
	if alertsCfg, err := config.LoadAlerts(alertsPath); err != nil {
		logger.Warn("alerts config not loaded, alerting disabled", "path", alertsPath, "error", err)
	} else {
		registry := notifier.NewRegistry()

		// Register notifiers from config
		for _, nc := range alertsCfg.Notifiers {
			switch nc.Type {
			case "ntfy":
				if nc.URL != "" && nc.Topic != "" {
					severities := make([]internal.Severity, 0, len(nc.SeverityFilter))
					for _, s := range nc.SeverityFilter {
						severities = append(severities, internal.Severity(s))
					}
					registry.Register(notifier.NewNtfyNotifier(notifier.NtfyConfig{
						Name:       "ntfy",
						URL:        nc.URL,
						Topic:      nc.Topic,
						Token:      nc.Token,
						Severities: severities,
					}))
					logger.Info("ntfy notifier registered", "url", nc.URL, "topic", nc.Topic)
				}
			case "webhook":
				if nc.URL != "" {
					severities := make([]internal.Severity, 0, len(nc.SeverityFilter))
					for _, s := range nc.SeverityFilter {
						severities = append(severities, internal.Severity(s))
					}
					registry.Register(notifier.NewWebhookNotifier(notifier.WebhookConfig{
						Name:       "webhook",
						URL:        nc.URL,
						Headers:    nc.Headers,
						Severities: severities,
					}))
					logger.Info("webhook notifier registered", "url", nc.URL)
				}
			default:
				logger.Warn("unknown notifier type", "type", nc.Type)
			}
		}

		// Parse rules and create alerter
		rules, err := config.ParseRules(alertsCfg.Alerts)
		if err != nil {
			logger.Error("failed to parse alert rules", "error", err)
		} else {
			a := alerter.New(rules, store, registry, logger)
			srv.SetAlerter(a)
			logger.Info("alerter enabled", "rules", len(rules), "notifiers", len(registry.All()))
		}
	}

	// Wire enrichment pipeline if config exists
	enrichPath := getEnv("ENRICHMENT_CONFIG", "config/enrichment.yml")
	if enrichCfg, err := config.LoadEnrichment(enrichPath); err != nil {
		logger.Warn("enrichment config not loaded, enrichment disabled", "path", enrichPath, "error", err)
	} else if enrichCfg.Enabled {
		pipeCfg := enrichment.PipelineConfig{
			GeoIPPath: enrichCfg.GeoIP.DBPath,
		}

		// Convert blocklist sources
		for _, src := range enrichCfg.Blocklists.Sources {
			pipeCfg.BlocklistSources = append(pipeCfg.BlocklistSources, enrichment.BlocklistSource{
				Name:   src.Name,
				URL:    src.URL,
				Format: src.Format,
			})
		}

		// Configure providers + budget limits
		budgetLimits := make(map[string]int)
		for name, pc := range enrichCfg.Providers {
			if !pc.Enabled || pc.APIKey == "" {
				continue
			}
			budgetLimits[name] = pc.DailyLimit
			switch name {
			case "abuseipdb":
				pipeCfg.Providers = append(pipeCfg.Providers, providers.NewAbuseIPDB(pc.APIKey, pc.DailyLimit))
			case "greynoise":
				pipeCfg.Providers = append(pipeCfg.Providers, providers.NewGreyNoise(pc.APIKey, pc.DailyLimit))
			case "crowdsec_cti":
				pipeCfg.Providers = append(pipeCfg.Providers, providers.NewCrowdSecCTI(pc.APIKey, pc.DailyLimit))
			}
		}
		pipeCfg.BudgetLimits = budgetLimits

		pipeline, err := enrichment.NewPipeline(pipeCfg, store, logger)
		if err != nil {
			logger.Error("failed to initialize enrichment pipeline", "error", err)
		} else {
			srv.SetEnrichment(pipeline, store)
			logger.Info("enrichment pipeline enabled",
				"geoip", pipeCfg.GeoIPPath != "",
				"blocklists", len(pipeCfg.BlocklistSources),
				"providers", len(pipeCfg.Providers),
			)
		}
	}

	// Wire veille-secu sync if configured
	veillePath := getEnv("VEILLE_CONFIG", "config/veille.yml")
	if veilleCfg, err := config.LoadVeille(veillePath); err != nil {
		logger.Warn("veille config not loaded, CVE correlation disabled", "path", veillePath, "error", err)
	} else if veilleCfg.Enabled && veilleCfg.URL != "" {
		veilleClient := veille.NewClient(veilleCfg.URL, veilleCfg.Token)

		pollInterval := 15 * time.Minute
		if veilleCfg.PollInterval != "" {
			if d, err := time.ParseDuration(veilleCfg.PollInterval); err == nil {
				pollInterval = d
			}
		}

		syncer := veille.NewSyncer(veilleClient, store, veille.SyncerConfig{
			PollInterval:   pollInterval,
			SyncTools:      veilleCfg.SyncTools,
			SeverityFilter: veilleCfg.SeverityFilter,
		}, logger)

		srv.SetVeilleSyncer(syncer)
		logger.Info("veille-secu integration enabled",
			"url", veilleCfg.URL,
			"poll_interval", pollInterval,
			"sync_tools", veilleCfg.SyncTools,
		)
	}

	// Command channel (opt-in)
	if getEnv("COMMAND_CHANNEL", "") == "true" {
		srv.EnableCommandChannel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
	}()

	if err := srv.Run(ctx); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

// parseAgentTokens parses "machine1:token1,machine2:token2" into AgentToken slice.
func parseAgentTokens(s string) []server.AgentToken {
	var agents []server.AgentToken
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			continue
		}
		agents = append(agents, server.AgentToken{
			MachineID: strings.TrimSpace(parts[0]),
			Token:     strings.TrimSpace(parts[1]),
		})
	}
	return agents
}

func requireEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		slog.Error("required environment variable not set", "key", key)
		os.Exit(1)
	}
	return val
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
