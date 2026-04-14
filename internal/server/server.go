package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
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

// AgentToken associe un machine_id à son secret partagé.
type AgentToken struct {
	MachineID string
	Token     string
}

// Config contient la configuration du serveur.
type Config struct {
	DashboardAddr string // e.g. "0.0.0.0:3001"
	AgentAddr     string // e.g. "0.0.0.0:3002"
	DBPath        string
	RetentionDays int
	Agents        []AgentToken
}

// Server est le serveur central bientot avec dual-listen.
type Server struct {
	cfg          Config
	store        storage.Storage
	tokens       map[string]string // machine_id -> token
	nonces       *transport.NonceCache
	alerter      *alerter.Alerter          // nil si aucune règle configurée
	pipeline     *enrichment.Pipeline      // nil si enrichissement désactivé
	enrichStore  *storage.SQLiteStorage    // nil si enrichissement désactivé
	sse          *SSEBroker
	cmdChannel   *CommandChannel    // nil si canal de commandes désactivé
	veilleSyncer     *veille.Syncer     // nil si veille-secu désactivé
	scanTracker      *scanTracker       // suivi des scans CVE reçus
	scanStaleMu      sync.Mutex
	scanStaleAlerted map[string]bool   // alertes de staleness déjà émises
	services         *serviceStore
	logger           *slog.Logger
}

// New crée une instance du serveur.
func New(cfg Config, store storage.Storage, logger *slog.Logger) *Server {
	tokens := make(map[string]string, len(cfg.Agents))
	for _, a := range cfg.Agents {
		tokens[a.MachineID] = a.Token
	}

	return &Server{
		cfg:               cfg,
		store:             store,
		tokens:            tokens,
		nonces:            transport.NewNonceCache(),
		sse:               NewSSEBroker(),
		scanTracker:       newScanTracker(),
		scanStaleAlerted:  make(map[string]bool),
		services:          newServiceStore(),
		logger:            logger,
	}
}

// SetAlerter attache un alerter au serveur.
func (s *Server) SetAlerter(a *alerter.Alerter) {
	s.alerter = a

	// Publication des événements d'alerte via SSE
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

// SetEnrichment attache le pipeline d'enrichissement au serveur.
func (s *Server) SetEnrichment(p *enrichment.Pipeline, store *storage.SQLiteStorage) {
	s.pipeline = p
	s.enrichStore = store
}

// EnableCommandChannel active le canal de commandes agent.
func (s *Server) EnableCommandChannel() {
	s.cmdChannel = NewCommandChannel(s.logger)
	s.logger.Info("canal de commandes activé")
}

// SetVeilleSyncer attache le syncer veille-secu.
func (s *Server) SetVeilleSyncer(syncer *veille.Syncer) {
	s.veilleSyncer = syncer

	// Publication des correspondances CVE via SSE + déclenchement d'alertes pour critical+KEV
	syncer.OnMatch(func(match internal.VulnMatch) {
		s.sse.Publish(SSEEvent{
			Type: "vuln_match",
			Data: match,
		})
	})
}

// Run démarre les deux listeners. Bloque jusqu'à l'annulation du ctx.
func (s *Server) Run(ctx context.Context) error {
	// Serveur dashboard (:3001) — exposé via Traefik
	dashboardRouter := s.dashboardRouter()
	dashboardSrv := &http.Server{
		Addr:         s.cfg.DashboardAddr,
		Handler:      dashboardRouter,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Serveur agent (:3002) — mesh direct, PAS via Traefik
	agentRouter := s.agentRouter()
	agentSrv := &http.Server{
		Addr:         s.cfg.AgentAddr,
		Handler:      agentRouter,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 2)

	go func() {
		s.logger.Info("dashboard en écoute", "addr", s.cfg.DashboardAddr)
		if err := dashboardSrv.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	go func() {
		s.logger.Info("endpoint agent en écoute", "addr", s.cfg.AgentAddr)
		if err := agentSrv.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Démarrage de la boucle d'évaluation des alertes si configuré
	if s.alerter != nil {
		go s.runAlertLoop(ctx)
	}

	// Démarrage de la synchronisation veille-secu si configuré
	if s.veilleSyncer != nil {
		go s.veilleSyncer.Run(ctx)
	}

	select {
	case <-ctx.Done():
		s.logger.Info("arrêt du serveur")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		dashboardSrv.Shutdown(shutdownCtx)
		agentSrv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

// runAlertLoop évalue les règles d'alerte à intervalle fixe.
func (s *Server) runAlertLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	s.logger.Info("alerter démarré", "interval", "30s")

	// Évaluation initiale
	if err := s.alerter.Evaluate(ctx); err != nil {
		s.logger.Error("échec de l'évaluation initiale des alertes", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.alerter.Evaluate(ctx); err != nil {
				s.logger.Error("échec de l'évaluation des alertes", "error", err)
			}
			s.checkScanStaleness()
		}
	}
}

// dashboardRouter sert l'interface web et l'API sur :3001.
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

		// Endpoints alertes
		r.Get("/alerts", s.handleAlerts)
		r.Get("/alerts/active", s.handleActiveAlerts)
		r.Post("/alerts/{alertID}/ack", s.handleAckAlert)

		// Endpoints threat intel
		r.Get("/threats", s.handleThreats)
		r.Get("/threats/attackers", s.handleAttackers)
		r.Get("/threats/patterns", s.handlePatterns)
		r.Get("/threats/unblocked", s.handleUnblocked)
		r.Get("/threats/budget", s.handleBudget)

		// Endpoints vulnérabilités / CVE
		r.Get("/vulns", s.handleVulns)
		r.Get("/vulns/active", s.handleActiveVulns)
		r.Get("/vulns/inventory", s.handleInventory)
		r.Get("/vulns/sync", s.handleSyncLogs)
		r.Patch("/vulns/{id}/dismiss", s.handleDismissVuln)
		r.Patch("/vulns/{id}/resolve", s.handleResolveVuln)

		// Découverte de services
		r.Get("/services", s.handleServices)

		// Événements SSE en temps réel
		r.Get("/events", s.handleSSE)

		// Canal de commandes (dashboard → agent)
		r.Post("/commands", s.handleSendCommand)
		r.Get("/commands/agents", s.handleConnectedAgents)
	})

	return r
}

// agentRouter gère les push des agents sur :3002.
func (s *Server) agentRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Post("/push", s.handlePush)
	r.Post("/scan/ingest", s.handleScanIngest)
	r.Get("/health", s.handleHealth)

	// Canal de commandes WebSocket (opt-in)
	r.Get("/ws", s.handleAgentWS)

	return r
}
