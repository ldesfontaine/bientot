package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	dashboardsrv "github.com/ldesfontaine/bientot/internal/dashboard"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	logger.Info("dashboard starting",
		"version", version,
		"commit", commit,
		"built", date,
	)

	addr := getEnv("DASHBOARD_ADDR", ":8443")
	cert := getEnv("DASHBOARD_CERT", "/etc/bientot/certs/server.crt")
	key := getEnv("DASHBOARD_KEY", "/etc/bientot/certs/server.key")
	ca := getEnv("DASHBOARD_CA_BUNDLE", "/etc/bientot/certs/ca-bundle.crt")
	agentKeys := getEnv("DASHBOARD_AGENT_KEYS", "/etc/bientot/agent-keys")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		logger.Info("shutdown signal received")
		cancel()
	}()

	s := dashboardsrv.New(logger, addr, cert, key, ca, agentKeys)
	if err := s.Run(ctx); err != nil {
		logger.Error("dashboard failed", "err", err)
		os.Exit(1)
	}

	logger.Info("dashboard exited")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
