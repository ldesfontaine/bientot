// Package api serves the dashboard's read-only JSON API.
//
// The package is stateless at the HTTP-server level: it exposes
// a Router that builds an http.Handler, and the caller is responsible
// for running the *http.Server (typically cmd/dashboard/main.go).
//
// API consumers are humans/browsers, not agents — agent ingestion lives
// in the parent package over mTLS.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
)

// Router wraps the dependencies needed to serve API endpoints.
// It does not own an HTTP server: the caller mounts BuildHandler() into
// its own *http.Server.
type Router struct {
	db               *storage.Storage
	log              *slog.Logger
	offlineThreshold time.Duration
}

// Config bundles the parameters needed to build the API router.
type Config struct {
	OfflineThreshold time.Duration
}

// NewRouter returns a Router holding the shared dependencies.
func NewRouter(log *slog.Logger, db *storage.Storage, cfg Config) *Router {
	return &Router{
		db:               db,
		log:              log,
		offlineThreshold: cfg.OfflineThreshold,
	}
}

// BuildHandler constructs and returns the http.Handler for all API routes.
// The result is wrapped with request-logging middleware.
func (r *Router) BuildHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", r.handleHealth)
	mux.HandleFunc("GET /api/agents", r.handleListAgents)
	mux.HandleFunc("GET /api/agents/{id}/metrics", r.handleGetLatestMetrics)
	mux.HandleFunc("GET /api/agents/{id}/metric-points", r.handleGetMetricPoints)

	return r.withLogging(mux)
}

// withLogging logs every request at INFO level: method, path, status, duration.
func (r *Router) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, req)
		r.log.Info("api request",
			"method", req.Method,
			"path", req.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// statusRecorder wraps http.ResponseWriter to capture the status code
// written via WriteHeader (the stdlib doesn't expose it).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}
