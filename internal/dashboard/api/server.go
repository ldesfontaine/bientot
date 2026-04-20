// Package api serves the dashboard's read-only JSON API on a separate
// HTTP listener (no TLS) intended to be accessed from a trusted network
// (e.g. NetBird, Tailscale, WireGuard).
//
// This is distinct from the mTLS agent-ingestion server in the parent
// package: API consumers are humans/browsers, not agents.
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
)

// Server is the HTTP server exposing the dashboard's read API.
// It wraps an *http.Server and keeps a reference to the shared storage handle.
type Server struct {
	addr             string
	db               *storage.Storage
	log              *slog.Logger
	srv              *http.Server
	offlineThreshold time.Duration
}

// Config bundles the parameters needed to build an api.Server.
type Config struct {
	Addr             string
	OfflineThreshold time.Duration
}

// New returns a Server that will listen on cfg.Addr when Run is called.
// The db handle must be opened by the caller and remains owned by the caller.
func New(log *slog.Logger, db *storage.Storage, cfg Config) *Server {
	return &Server{
		addr:             cfg.Addr,
		db:               db,
		log:              log,
		offlineThreshold: cfg.OfflineThreshold,
	}
}

// Run starts listening and blocks until ctx is cancelled. Returns the first
// non-ErrServerClosed error encountered.
func (s *Server) Run(ctx context.Context) error {
	mux := s.buildRouter()

	s.srv = &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	shutdownDone := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			s.log.Error("api shutdown error", "error", err)
		}
		close(shutdownDone)
	}()

	s.log.Info("api server listening", "addr", s.addr)
	if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("api listen: %w", err)
	}

	<-shutdownDone
	s.log.Info("api server stopped")
	return nil
}
