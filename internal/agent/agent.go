package agent

import (
	"context"
	"log/slog"
	"time"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
	"github.com/ldesfontaine/bientot/internal/agent/client"
	"github.com/ldesfontaine/bientot/internal/modules"
)

// detectTimeout caps how long a single module's Detect may take at startup.
const detectTimeout = 5 * time.Second

// pushInterval is the single global frequency for pushing collected data.
// At palier 3 all active modules are collected and shipped every tick,
// ignoring per-module Interval(). Per-module scheduling lands at palier 5+.
const pushInterval = 30 * time.Second

// pushTimeout caps how long a single push (collect + HTTP) may take. Keeps a
// stuck push from blocking the loop past the next tick.
const pushTimeout = 10 * time.Second

// Agent runs the active modules on a unified push loop.
type Agent struct {
	modules    []modules.Module
	pushClient *client.Client
	log        *slog.Logger
}

// New filters available modules by calling Detect on each, and returns an Agent
// holding only the ones that reported themselves as runnable. If pushClient is
// non-nil, Run will launch a periodic push loop against the dashboard.
func New(log *slog.Logger, pushClient *client.Client, available []modules.Module) *Agent {
	var active []modules.Module

	for _, m := range available {
		ctx, cancel := context.WithTimeout(context.Background(), detectTimeout)
		err := m.Detect(ctx)
		cancel()

		if err != nil {
			log.Warn("module disabled", "module", m.Name(), "reason", err)
			continue
		}
		log.Info("module enabled", "module", m.Name())
		active = append(active, m)
	}

	return &Agent{modules: active, pushClient: pushClient, log: log}
}

// Run starts the push loop and blocks until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) {
	a.log.Info("agent running", "modules", len(a.modules))

	if len(a.modules) == 0 {
		a.log.Warn("no modules active")
	}

	if a.pushClient != nil {
		go a.pushLoop(ctx)
	}

	<-ctx.Done()
	a.log.Info("agent stopped")
}

func (a *Agent) pushLoop(ctx context.Context) {
	ticker := time.NewTicker(pushInterval)
	defer ticker.Stop()

	a.doPush(ctx)

	for {
		select {
		case <-ctx.Done():
			a.log.Info("push loop stopped")
			return
		case <-ticker.C:
			a.doPush(ctx)
		}
	}
}

func (a *Agent) doPush(ctx context.Context) {
	pushCtx, cancel := context.WithTimeout(ctx, pushTimeout)
	defer cancel()

	var moduleDatas []*bientotv1.ModuleData
	for _, m := range a.modules {
		data, err := m.Collect(pushCtx)
		if err != nil {
			a.log.Warn("module collect failed", "module", m.Name(), "error", err)
			continue
		}
		moduleDatas = append(moduleDatas, client.ToProto(data))
	}

	if len(moduleDatas) == 0 {
		a.log.Debug("no module data to push")
		return
	}

	resp, err := a.pushClient.Push(pushCtx, moduleDatas)
	if err != nil {
		a.log.Warn("push failed", "error", err)
		return
	}

	a.log.Info("push ok",
		"status", resp.Status,
		"modules", resp.AcceptedModules,
		"metrics", resp.AcceptedMetrics,
	)
}
