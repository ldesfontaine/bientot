package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ldesfontaine/bientot/internal/echoserver"
)

const version = "0.1.0-dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	logger.Info("echo-server starting", "version", version)

	addr := getEnv("ECHO_ADDR", ":8443")
	cert := getEnv("ECHO_CERT", "/etc/bientot/certs/server.crt")
	key := getEnv("ECHO_KEY", "/etc/bientot/certs/server.key")
	ca := getEnv("ECHO_CA_BUNDLE", "/etc/bientot/certs/ca-bundle.crt")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		logger.Info("shutdown signal received")
		cancel()
	}()

	s := echoserver.New(logger, addr, cert, key, ca)
	if err := s.Run(ctx); err != nil {
		logger.Error("echo-server failed", "err", err)
		os.Exit(1)
	}

	logger.Info("echo-server exited")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
