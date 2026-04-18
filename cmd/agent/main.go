package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ldesfontaine/bientot/internal/agent"
	"github.com/ldesfontaine/bientot/internal/modules"
	"github.com/ldesfontaine/bientot/internal/modules/heartbeat"
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

	available := []modules.Module{
		heartbeat.New(),
	}

	a := agent.New(logger, available)
	a.Run(ctx)

	logger.Info("agent exited")
}
