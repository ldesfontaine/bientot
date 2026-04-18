package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ldesfontaine/bientot/internal/agent"
	"github.com/ldesfontaine/bientot/internal/agent/client"
	"github.com/ldesfontaine/bientot/internal/config"
	"github.com/ldesfontaine/bientot/internal/modules"
	_ "github.com/ldesfontaine/bientot/internal/modules/registry"
	"github.com/ldesfontaine/bientot/internal/shared/keys"
)

const version = "0.2.0-dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	logger.Info("agent starting", "version", version)

	configPath := getEnv("BIENTOT_CONFIG", "/etc/bientot/agent.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("failed to load config", "path", configPath, "error", err)
		os.Exit(1)
	}
	logger.Info("config loaded",
		"path", configPath,
		"machine_id", cfg.MachineID,
		"modules_declared", len(cfg.Modules),
		"push_interval", cfg.PushInterval,
	)

	signKey, err := keys.LoadPrivateKey(cfg.SigningKey)
	if err != nil {
		logger.Error("failed to load signing key", "path", cfg.SigningKey, "error", err)
		os.Exit(1)
	}

	pushClient, err := client.New(
		cfg.Dashboard.URL,
		cfg.Dashboard.Cert,
		cfg.Dashboard.Key,
		cfg.Dashboard.CABundle,
		cfg.Dashboard.ServerName,
		signKey,
		cfg.MachineID,
	)
	if err != nil {
		logger.Error("failed to init dashboard client", "error", err)
		os.Exit(1)
	}

	moduleConfigs := make([]modules.ModuleConfig, len(cfg.Modules))
	for i, mc := range cfg.Modules {
		moduleConfigs[i] = modules.ModuleConfig{
			Type:    mc.Type,
			Enabled: mc.Enabled,
			Config:  mc.Config,
		}
	}
	builtModules, err := modules.Build(moduleConfigs, logger)
	if err != nil {
		logger.Error("failed to build modules", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		logger.Info("shutdown signal received")
		cancel()
	}()

	a := agent.New(logger, pushClient, builtModules, cfg.PushInterval)
	a.Run(ctx)

	logger.Info("agent exited")
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
