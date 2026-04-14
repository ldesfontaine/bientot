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

// Config contient la configuration de l'agent.
type Config struct {
	ServerURL string
	MachineID string
	Token     string // secret partagé pour la signature HMAC

	// Intervalles de push : hot = fréquent, warm = normal, cold = idle
	HotInterval  time.Duration
	WarmInterval time.Duration
	ColdInterval time.Duration
}

// Agent collecte les métriques depuis les modules détectés et les push au serveur.
type Agent struct {
	cfg     Config
	modules []modules.Module
	client  *http.Client
	logger  *slog.Logger
}

// New crée un agent. Il auto-détecte quels modules sont disponibles.
func New(cfg Config, available []modules.Module, logger *slog.Logger) *Agent {
	var detected []modules.Module
	for _, m := range available {
		if m.Detect() {
			logger.Info("module détecté", "module", m.Name())
			detected = append(detected, m)
		} else {
			logger.Debug("module non disponible", "module", m.Name())
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

// Run démarre la boucle de push. Bloque jusqu'à l'annulation du ctx.
func (a *Agent) Run(ctx context.Context) {
	interval := a.cfg.WarmInterval
	if interval == 0 {
		interval = 60 * time.Second
	}

	a.logger.Info("agent démarré",
		"machine", a.cfg.MachineID,
		"server", a.cfg.ServerURL,
		"interval", interval,
		"modules", len(a.modules),
	)

	// Push initial
	a.push(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("arrêt de l'agent")
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
		a.logger.Error("échec de la signature du payload", "error", err)
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
		a.logger.Error("échec de la sérialisation du payload", "error", err)
		return
	}

	url := a.cfg.ServerURL + "/push"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		a.logger.Error("échec de la création de la requête", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		a.logger.Error("échec du push", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		a.logger.Error("push rejeté", "status", resp.StatusCode, "body", string(respBody))
		return
	}

	a.logger.Debug("push ok", "modules", len(body.Modules))
}

func (a *Agent) collectAll(ctx context.Context) []transport.ModuleData {
	var results []transport.ModuleData

	for _, m := range a.modules {
		data, err := m.Collect(ctx)
		if err != nil {
			a.logger.Warn("échec de la collecte du module", "module", m.Name(), "error", err)
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
