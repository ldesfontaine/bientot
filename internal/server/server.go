package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ldesfontaine/bientot/internal"
	"github.com/ldesfontaine/bientot/internal/alerter"
	"github.com/ldesfontaine/bientot/internal/enrichment"
	"github.com/ldesfontaine/bientot/internal/storage"
	"github.com/ldesfontaine/bientot/internal/transport"
	"github.com/ldesfontaine/bientot/internal/veille"
)

// AgentToken maps a machine_id to its shared secret.
type AgentToken struct {
	MachineID string
	Token     string
}

// Config holds server configuration.
type Config struct {
	DashboardAddr string // e.g. "0.0.0.0:3001"
	AgentAddr     string // e.g. "0.0.0.0:3002"
	DBPath        string
	RetentionDays int
	Agents        []AgentToken
}

// Server is the central bientot server with dual-listen.
type Server struct {
	cfg          Config
	store        storage.Storage
	tokens       map[string]string // machine_id -> token
	nonces       *transport.NonceCache
	alerter      *alerter.Alerter          // nil if no rules configured
	pipeline     *enrichment.Pipeline      // nil if enrichment disabled
	enrichStore  *storage.SQLiteStorage    // nil if enrichment disabled
	sse          *SSEBroker
	cmdChannel   *CommandChannel    // nil if command channel disabled
	veilleSyncer *veille.Syncer     // nil if veille-secu disabled
	services     *serviceStore
	logger       *slog.Logger
}

// New creates a server instance.
func New(cfg Config, store storage.Storage, logger *slog.Logger) *Server {
	tokens := make(map[string]string, len(cfg.Agents))
	for _, a := range cfg.Agents {
		tokens[a.MachineID] = a.Token
	}

	return &Server{
		cfg:      cfg,
		store:    store,
		tokens:   tokens,
		nonces:   transport.NewNonceCache(),
		sse:      NewSSEBroker(),
		services: newServiceStore(),
		logger:   logger,
	}
}

// SetAlerter attaches an alerter to the server.
func (s *Server) SetAlerter(a *alerter.Alerter) {
	s.alerter = a

	// Publish alert events to SSE
	a.OnAlert(func(alert internal.Alert, resolved bool) {
		eventType := "alert_fired"
		if resolved {
			eventType = "alert_resolved"
		}
		s.sse.Publish(SSEEvent{
			Type: eventType,
			Data: alert,
		})
	})
}

// SetEnrichment attaches the enrichment pipeline to the server.
func (s *Server) SetEnrichment(p *enrichment.Pipeline, store *storage.SQLiteStorage) {
	s.pipeline = p
	s.enrichStore = store
}

// EnableCommandChannel activates the agent command channel.
func (s *Server) EnableCommandChannel() {
	s.cmdChannel = NewCommandChannel(s.logger)
	s.logger.Info("command channel enabled")
}

// SetVeilleSyncer attaches the veille-secu syncer.
func (s *Server) SetVeilleSyncer(syncer *veille.Syncer) {
	s.veilleSyncer = syncer

	// Publish CVE matches to SSE + trigger alerts for critical+KEV
	syncer.OnMatch(func(match internal.VulnMatch) {
		s.sse.Publish(SSEEvent{
			Type: "vuln_match",
			Data: match,
		})
	})
}

// Run starts both listeners. Blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	// Dashboard server (:3001) — exposed via Traefik
	dashboardRouter := s.dashboardRouter()
	dashboardSrv := &http.Server{
		Addr:         s.cfg.DashboardAddr,
		Handler:      dashboardRouter,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Agent server (:3002) — mesh direct, NOT via Traefik
	agentRouter := s.agentRouter()
	agentSrv := &http.Server{
		Addr:         s.cfg.AgentAddr,
		Handler:      agentRouter,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 2)

	go func() {
		s.logger.Info("dashboard listening", "addr", s.cfg.DashboardAddr)
		if err := dashboardSrv.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	go func() {
		s.logger.Info("agent endpoint listening", "addr", s.cfg.AgentAddr)
		if err := agentSrv.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Start alerter eval loop if configured
	if s.alerter != nil {
		go s.runAlertLoop(ctx)
	}

	// Start veille-secu sync if configured
	if s.veilleSyncer != nil {
		go s.veilleSyncer.Run(ctx)
	}

	select {
	case <-ctx.Done():
		s.logger.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		dashboardSrv.Shutdown(shutdownCtx)
		agentSrv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

// runAlertLoop evaluates alert rules on a fixed interval.
func (s *Server) runAlertLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	s.logger.Info("alerter started", "interval", "30s")

	// Initial evaluation
	if err := s.alerter.Evaluate(ctx); err != nil {
		s.logger.Error("initial alert evaluation failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.alerter.Evaluate(ctx); err != nil {
				s.logger.Error("alert evaluation failed", "error", err)
			}
		}
	}
}

// dashboardRouter serves the web UI and API on :3001.
func (s *Server) dashboardRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	r.Get("/health", s.handleHealth)

	r.Route("/api", func(r chi.Router) {
		r.Get("/status", s.handleStatus)
		r.Get("/machines", s.handleMachines)
		r.Get("/machines/{machineID}/metrics", s.handleMachineMetrics)
		r.Get("/metrics", s.handleListMetrics)
		r.Get("/metrics/{name}", s.handleQueryMetric)
		r.Get("/metrics/{name}/latest", s.handleLatestMetric)

		// Alert endpoints
		r.Get("/alerts", s.handleAlerts)
		r.Get("/alerts/active", s.handleActiveAlerts)
		r.Post("/alerts/{alertID}/ack", s.handleAckAlert)

		// Threat intel endpoints
		r.Get("/threats", s.handleThreats)
		r.Get("/threats/attackers", s.handleAttackers)
		r.Get("/threats/patterns", s.handlePatterns)
		r.Get("/threats/unblocked", s.handleUnblocked)
		r.Get("/threats/budget", s.handleBudget)

		// Vulnerability / CVE endpoints
		r.Get("/vulns", s.handleVulns)
		r.Get("/vulns/active", s.handleActiveVulns)
		r.Get("/vulns/inventory", s.handleInventory)
		r.Get("/vulns/sync", s.handleSyncLogs)
		r.Patch("/vulns/{id}/dismiss", s.handleDismissVuln)
		r.Patch("/vulns/{id}/resolve", s.handleResolveVuln)

		// Service discovery
		r.Get("/services", s.handleServices)

		// SSE real-time events
		r.Get("/events", s.handleSSE)

		// Command channel (dashboard → agent)
		r.Post("/commands", s.handleSendCommand)
		r.Get("/commands/agents", s.handleConnectedAgents)
	})

	return r
}

// agentRouter handles push from agents on :3002.
func (s *Server) agentRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Post("/push", s.handlePush)
	r.Get("/health", s.handleHealth)

	// Command channel WebSocket (opt-in)
	r.Get("/ws", s.handleAgentWS)

	return r
}
