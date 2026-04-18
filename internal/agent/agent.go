package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/ldesfontaine/bientot/internal/modules"
)

// detectTimeout caps how long a single module's Detect may take at startup.
const detectTimeout = 5 * time.Second

// Agent runs the active modules on their respective intervals.
type Agent struct {
	modules []modules.Module
	log     *slog.Logger
}

// New filters available modules by calling Detect on each, and returns an Agent
// holding only the ones that reported themselves as runnable.
func New(log *slog.Logger, available []modules.Module) *Agent {
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

	return &Agent{modules: active, log: log}
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

	<-ctx.Done()
	a.log.Info("agent stopped")
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
