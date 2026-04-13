package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"strings"

	"github.com/ldesfontaine/bientot/internal/agent"
	"github.com/ldesfontaine/bientot/internal/modules"
	"github.com/ldesfontaine/bientot/internal/modules/backup"
	"github.com/ldesfontaine/bientot/internal/modules/certs"
	"github.com/ldesfontaine/bientot/internal/modules/crowdsec"
	"github.com/ldesfontaine/bientot/internal/modules/docker"
	gitmod "github.com/ldesfontaine/bientot/internal/modules/git"
	"github.com/ldesfontaine/bientot/internal/modules/netbird"
	"github.com/ldesfontaine/bientot/internal/modules/system"
	"github.com/ldesfontaine/bientot/internal/modules/traefik"
	"github.com/ldesfontaine/bientot/internal/modules/zfs"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(getEnv("LOG_LEVEL", "info")),
	}))
	slog.SetDefault(logger)

	cfg := agent.Config{
		ServerURL: requireEnv("BIENTOT_SERVER_URL"),
		MachineID: requireEnv("BIENTOT_MACHINE_ID"),
		Token:     requireEnv("BIENTOT_TOKEN"),

		HotInterval:  parseDuration(getEnv("PUSH_HOT", "10s")),
		WarmInterval: parseDuration(getEnv("PUSH_WARM", "1m")),
		ColdInterval: parseDuration(getEnv("PUSH_COLD", "5m")),
	}

	// All available modules — agent auto-detects which ones work on this machine
	available := []modules.Module{
		system.New(),
		docker.New(getEnv("DOCKER_HOST", "")),
		zfs.New(splitList(getEnv("ZFS_POOLS", ""))),
		crowdsec.New(getEnv("CROWDSEC_URL", "")),
		netbird.New(),
		traefik.New(getEnv("TRAEFIK_API_URL", ""), getEnv("DOCKER_SOCKET", "")),
		backup.New(getEnv("BACKUP_STATUS_DIR", "")),
		certs.New(splitList(getEnv("CERT_DOMAINS", ""))),
		gitmod.New(splitList(getEnv("GIT_REPOS", ""))),
	}

	a := agent.New(cfg, available, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
	}()

	// Command channel (opt-in, disabled by default)
	if getEnv("COMMAND_CHANNEL", "") == "true" {
		logger.Info("command channel enabled")
		go a.RunCommandChannel(ctx, a.DefaultCommandHandler())
	}

	a.Run(ctx)
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

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		slog.Error("invalid duration", "value", s, "error", err)
		os.Exit(1)
	}
	return d
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	var items []string
	for _, item := range strings.Split(s, ",") {
		if v := strings.TrimSpace(item); v != "" {
			items = append(items, v)
		}
	}
	return items
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
