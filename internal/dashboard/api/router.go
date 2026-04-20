package api

import (
	"net/http"
	"time"
)

// buildRouter constructs the HTTP mux and wires all handlers.
// Route registration happens here so adding endpoints is a single-file change.
func (s *Server) buildRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/agents", s.handleListAgents)
	mux.HandleFunc("GET /api/agents/{id}/metrics", s.handleGetLatestMetrics)
	mux.HandleFunc("GET /api/agents/{id}/metric-points", s.handleGetMetricPoints)

	return s.withLogging(mux)
}

// withLogging logs every request at INFO level: method, path, status, duration.
func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.log.Info("api request",
			"method", r.Method,
			"path", r.URL.Path,
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

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
