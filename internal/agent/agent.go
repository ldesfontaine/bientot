package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/ldesfontaine/bientot/internal/agent/client"
	"github.com/ldesfontaine/bientot/internal/modules"
)

// detectTimeout caps how long a single module's Detect may take at startup.
const detectTimeout = 5 * time.Second

// pingInterval is how often the agent pings the dashboard. Independent of
// module intervals: it's a liveness check of the mTLS channel, not a metric.
const pingInterval = 30 * time.Second

// pingTimeout caps how long a single ping may take. Keeps a stuck ping from
// blocking the loop past the next tick.
const pingTimeout = 10 * time.Second

// Agent runs the active modules on their respective intervals.
type Agent struct {
	modules []modules.Module
	pinger  *client.Client
	log     *slog.Logger
}

// New filters available modules by calling Detect on each, and returns an Agent
// holding only the ones that reported themselves as runnable. If pinger is
// non-nil, Run will also launch a periodic ping loop against the dashboard.
func New(log *slog.Logger, pinger *client.Client, available []modules.Module) *Agent {
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

	return &Agent{modules: active, pinger: pinger, log: log}
}

// Run starts one goroutine per active module and blocks until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) {
	a.log.Info("agent running", "modules", len(a.modules))

	if len(a.modules) == 0 {
		a.log.Warn("no modules active")
	}

	for _, m := range a.modules {
		go a.runModule(ctx, m)
	}

	if a.pinger != nil {
		go a.pingLoop(ctx)
	}

	<-ctx.Done()
	a.log.Info("agent stopped")
}

func (a *Agent) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	a.ping(ctx)

	for {
		select {
		case <-ctx.Done():
			a.log.Info("ping loop stopped")
			return
		case <-ticker.C:
			a.ping(ctx)
		}
	}
}

func (a *Agent) ping(ctx context.Context) {
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	resp, err := a.pinger.Ping(pingCtx)
	if err != nil {
		a.log.Warn("ping failed", "error", err)
		return
	}
	a.log.Info("ping ok", "client_cn", resp.ClientCN, "from", resp.From)
}

func (a *Agent) runModule(ctx context.Context, m modules.Module) {
	ticker := time.NewTicker(m.Interval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.log.Info("module stopped", "module", m.Name())
			return
		case <-ticker.C:
			a.collect(ctx, m)
		}
	}
}

func (a *Agent) collect(ctx context.Context, m modules.Module) {
	data, err := m.Collect(ctx)
	if err != nil {
		a.log.Warn("collect failed", "module", m.Name(), "error", err)
		return
	}

	a.log.Info("collected",
		"module", m.Name(),
		"metrics", len(data.Metrics),
		"metadata", data.Metadata,
	)
}
