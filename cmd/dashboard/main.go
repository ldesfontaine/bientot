package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

const version = "0.1.0-dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("dashboard starting", "version", version)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		slog.Info("shutdown signal received")
		cancel()
	}()

	<-ctx.Done()
	slog.Info("dashboard stopped")
}
