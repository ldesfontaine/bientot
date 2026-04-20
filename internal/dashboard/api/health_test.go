package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestServer returns a Server with a nil storage (handlers that don't touch
// the DB still work) and a silent logger, for unit testing handlers.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{
		db:               nil,
		log:              slog.New(slog.NewJSONHandler(io.Discard, nil)),
		offlineThreshold: 2 * time.Minute,
	}
}

// doRequest performs an in-process request against the server's router
// and returns the recorded response.
func doRequest(t *testing.T, s *Server, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	s.buildRouter().ServeHTTP(rec, req)
	return rec
}

func TestHealth_ReturnsOK(t *testing.T) {
	s := newTestServer(t)

	rec := doRequest(t, s, http.MethodGet, "/api/health")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}

	var body healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
}

func TestHealth_WrongMethod(t *testing.T) {
	s := newTestServer(t)

	rec := doRequest(t, s, http.MethodPost, "/api/health")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/health: status = %d, want 405", rec.Code)
	}
}

func TestHealth_UnknownRoute(t *testing.T) {
	s := newTestServer(t)

	rec := doRequest(t, s, http.MethodGet, "/api/does-not-exist")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
