package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/ldesfontaine/bientot/internal/modules"
	"github.com/ldesfontaine/bientot/internal/transport"
)

// Config holds agent configuration.
type Config struct {
	ServerURL string
	MachineID string
	Token     string // shared secret for HMAC signing

	// Push intervals: hot = frequent, warm = normal, cold = idle
	HotInterval  time.Duration
	WarmInterval time.Duration
	ColdInterval time.Duration
}

// Agent collects metrics from detected modules and pushes them to the server.
type Agent struct {
	cfg     Config
	modules []modules.Module
	client  *http.Client
	logger  *slog.Logger
}

// New creates an agent. It auto-detects which modules are available.
func New(cfg Config, available []modules.Module, logger *slog.Logger) *Agent {
	var detected []modules.Module
	for _, m := range available {
		if m.Detect() {
			logger.Info("module detected", "module", m.Name())
			detected = append(detected, m)
		} else {
			logger.Debug("module not available", "module", m.Name())
		}
	}

	return &Agent{
		cfg:     cfg,
		modules: detected,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Run starts the push loop. It blocks until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) {
	interval := a.cfg.WarmInterval
	if interval == 0 {
		interval = 60 * time.Second
	}

	a.logger.Info("agent started",
		"machine", a.cfg.MachineID,
		"server", a.cfg.ServerURL,
		"interval", interval,
		"modules", len(a.modules),
	)

	// Initial push
	a.push(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("agent stopping")
			return
		case <-ticker.C:
			a.push(ctx)
		}
	}
}

func (a *Agent) push(ctx context.Context) {
	body := transport.Body{
		Modules: a.collectAll(ctx),
	}

	sig, err := transport.Sign(body, a.cfg.Token)
	if err != nil {
		a.logger.Error("failed to sign payload", "error", err)
		return
	}

	payload := transport.Payload{
		MachineID: a.cfg.MachineID,
		Timestamp: time.Now(),
		Nonce:     transport.NewNonce(),
		Signature: sig,
		Body:      body,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		a.logger.Error("failed to marshal payload", "error", err)
		return
	}

	url := a.cfg.ServerURL + "/push"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		a.logger.Error("failed to create request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		a.logger.Error("push failed", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		a.logger.Error("push rejected", "status", resp.StatusCode, "body", string(respBody))
		return
	}

	a.logger.Debug("push ok", "modules", len(body.Modules))
}

func (a *Agent) collectAll(ctx context.Context) []transport.ModuleData {
	var results []transport.ModuleData

	for _, m := range a.modules {
		data, err := m.Collect(ctx)
		if err != nil {
			a.logger.Warn("module collect failed", "module", m.Name(), "error", err)
			results = append(results, transport.ModuleData{
				Module:    m.Name(),
				Error:     fmt.Sprintf("%v", err),
				Timestamp: time.Now(),
			})
			continue
		}
		results = append(results, data)
	}

	return results
}
