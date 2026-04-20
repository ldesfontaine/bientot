package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	dashboardsrv "github.com/ldesfontaine/bientot/internal/dashboard"
	"github.com/ldesfontaine/bientot/internal/dashboard/api"
	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
	"github.com/ldesfontaine/bientot/internal/dashboard/web"
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

	apiRouter := api.NewRouter(logger, db, api.Config{
		OfflineThreshold: time.Duration(offlineThresholdSec) * time.Second,
	})

	devMode := getEnvBool("DASHBOARD_DEV_MODE", false)
	webRouter, err := web.NewRouter(logger, db, web.Config{DevMode: devMode})
	if err != nil {
		logger.Error("failed to init web router", "error", err)
		os.Exit(1)
	}
	logger.Info("web router initialized", "devMode", devMode)

	// Single mux serves both API (/api/*) and web (/*).
	// /api/ must be registered before / to take precedence.
	mainMux := http.NewServeMux()
	mainMux.Handle("/api/", apiRouter.BuildHandler())
	mainMux.Handle("/", webRouter.BuildHandler())

	httpSrv := &http.Server{
		Addr:              webAddr,
		Handler:           mainMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	group, groupCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		return mtlsServer.Run(groupCtx)
	})

	group.Go(func() error {
		logger.Info("http server listening", "addr", webAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})

	group.Go(func() error {
		<-groupCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
		return nil
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

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
