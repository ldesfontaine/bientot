package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ldesfontaine/bientot/internal/agent"
	"github.com/ldesfontaine/bientot/internal/agent/client"
	"github.com/ldesfontaine/bientot/internal/modules"
	"github.com/ldesfontaine/bientot/internal/modules/heartbeat"
	"github.com/ldesfontaine/bientot/internal/shared/keys"
)

const version = "0.1.0-dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	logger.Info("agent starting", "version", version)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		logger.Info("shutdown signal received")
		cancel()
	}()

	dashboardURL := getEnv("DASHBOARD_URL", "https://echo-server:8443")
	certPath := getEnv("AGENT_CERT", "/etc/bientot/certs/client.crt")
	keyPath := getEnv("AGENT_KEY", "/etc/bientot/certs/client.key")
	caPath := getEnv("AGENT_CA_BUNDLE", "/etc/bientot/certs/ca-bundle.crt")
	serverName := getEnv("DASHBOARD_SERVER_NAME", "dashboard")
	signingKeyPath := getEnv("AGENT_SIGNING_KEY", "/etc/bientot/keys/signing.key")
	machineID := getEnv("BIENTOT_MACHINE_ID", "vps")

	signKey, err := keys.LoadPrivateKey(signingKeyPath)
	if err != nil {
		logger.Error("failed to load signing key", "error", err)
		os.Exit(1)
	}

	pushClient, err := client.New(dashboardURL, certPath, keyPath, caPath, serverName, signKey, machineID)
	if err != nil {
		logger.Error("failed to init dashboard client", "error", err)
		os.Exit(1)
	}

	available := []modules.Module{
		heartbeat.New(),
	}

	a := agent.New(logger, pushClient, available)
	a.Run(ctx)

	logger.Info("agent exited")
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
