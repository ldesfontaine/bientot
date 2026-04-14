package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ldesfontaine/bientot/internal/agent"
	"github.com/ldesfontaine/bientot/internal/modules"
	"github.com/ldesfontaine/bientot/internal/modules/adguard"
	"github.com/ldesfontaine/bientot/internal/modules/backup"
	"github.com/ldesfontaine/bientot/internal/modules/certs"
	"github.com/ldesfontaine/bientot/internal/modules/crowdsec"
	"github.com/ldesfontaine/bientot/internal/modules/docker"
	gitmod "github.com/ldesfontaine/bientot/internal/modules/git"
	"github.com/ldesfontaine/bientot/internal/modules/netbird"
	"github.com/ldesfontaine/bientot/internal/modules/system"
	"github.com/ldesfontaine/bientot/internal/modules/traefik"
)

func main() {
	// Sous-commande healthcheck : vérifie si le processus agent tourne
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		pidFile := getEnv("PID_FILE", "/tmp/bientot-agent.pid")
		data, err := os.ReadFile(pidFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck : impossible de lire le fichier pid : %v\n", err)
			os.Exit(1)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck : pid invalide : %v\n", err)
			os.Exit(1)
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck : processus introuvable : %v\n", err)
			os.Exit(1)
		}
		// Signal 0 vérifie si le processus existe sans le tuer
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck : processus %d non actif : %v\n", pid, err)
			os.Exit(1)
		}
		fmt.Println("OK")
		os.Exit(0)
	}

	// Écriture du fichier PID pour le healthcheck
	pidFile := getEnv("PID_FILE", "/tmp/bientot-agent.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		slog.Error("échec de l'écriture du fichier pid", "error", err)
	}

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

	// Tous les modules disponibles — l'agent auto-détecte ceux qui fonctionnent sur cette machine
	available := []modules.Module{
		system.New(getEnv("NODE_EXPORTER_URL", "")),
		docker.New(getEnv("DOCKER_HOST", "")),
		crowdsec.New(getEnv("CROWDSEC_URL", ""), getEnv("CROWDSEC_API_KEY", "")),
		adguard.New(getEnv("ADGUARD_URL", ""), getEnv("ADGUARD_USER", ""), getEnv("ADGUARD_PASSWORD", "")),
		netbird.New(getEnv("NETBIRD_PEER_IP", ""), getEnv("NETBIRD_PEER_PORT", "")),
		traefik.New(getEnv("TRAEFIK_API_URL", ""), getEnv("DOCKER_SOCKET", "")),
		backup.New(getEnv("BACKUP_STATUS_DIR", "/status")),
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
		logger.Info("signal d'arrêt reçu")
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
