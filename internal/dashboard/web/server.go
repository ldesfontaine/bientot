// Package web serves the dashboard's human-facing HTML pages.
// It shares the same HTTP listener as the api package: both are mounted
// together by the caller (cmd/dashboard/main.go).
package web

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
)

// Router wraps the dependencies needed to serve web pages.
type Router struct {
	db  *storage.Storage
	log *slog.Logger
}

// Config bundles the parameters needed to build the web router.
// Empty for now — extended in future features as needed.
type Config struct{}

// NewRouter returns a Router holding the shared dependencies.
func NewRouter(log *slog.Logger, db *storage.Storage, _ Config) *Router {
	return &Router{
		db:  db,
		log: log,
	}
}

//go:embed static
var staticFS embed.FS

// BuildHandler constructs and returns the http.Handler for all web routes.
//
// Mount the result at "/" in the parent mux. API routes under "/api/" must be
// registered on the parent mux BEFORE "/" to take precedence.
func (r *Router) BuildHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", r.handleHome)

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		r.log.Error("build static subFS failed", "error", err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	return r.withLogging(mux)
}

// withLogging mirrors the api package's middleware. Could be extracted to a
// shared internal/dashboard/httplog package later if duplication grows.
func (r *Router) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, req)
		r.log.Info("web request",
			"method", req.Method,
			"path", req.URL.Path,
			"status", rec.status,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}
