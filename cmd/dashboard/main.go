package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	dashboardsrv "github.com/ldesfontaine/bientot/internal/dashboard"
	"github.com/ldesfontaine/bientot/internal/dashboard/api"
	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
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
	dbPath := getEnv("BIENTOT_DB_PATH", "/data/dashboard.db")
	webAddr := getEnv("DASHBOARD_WEB_ADDR", "0.0.0.0:8080")
	offlineThresholdSec := getEnvInt("OFFLINE_THRESHOLD_SECONDS", 120)

	db, err := storage.Open(dbPath)
	if err != nil {
		logger.Error("failed to open storage", "path", dbPath, "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("error closing storage", "error", err)
		}
	}()
	logger.Info("storage opened", "path", dbPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		logger.Info("shutdown signal received")
		cancel()
	}()

	mtlsServer := dashboardsrv.New(logger, addr, cert, key, ca, agentKeys, db)
	apiServer := api.New(logger, db, api.Config{
		Addr:             webAddr,
		OfflineThreshold: time.Duration(offlineThresholdSec) * time.Second,
	})

	group, groupCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		return mtlsServer.Run(groupCtx)
	})

	group.Go(func() error {
		return apiServer.Run(groupCtx)
	})

	if err := group.Wait(); err != nil {
		logger.Error("server exited with error", "error", err)
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

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
