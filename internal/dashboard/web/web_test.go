package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestRouter(t *testing.T) *Router {
	t.Helper()
	r, err := NewRouter(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		nil,
		Config{DevMode: false},
	)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return r
}

func doRequest(t *testing.T, r *Router, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	r.BuildHandler().ServeHTTP(rec, req)
	return rec
}

func TestHome_ReturnsHTML(t *testing.T) {
	r := newTestRouter(t)

	rec := doRequest(t, r, http.MethodGet, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("response should contain DOCTYPE")
	}
	if !strings.Contains(body, "Bientôt") {
		t.Error("response should contain 'Bientôt' (title)")
	}
	if !strings.Contains(body, "2d 14h 22m") {
		t.Error("response should contain formatted duration from fmtDuration")
	}
}

func TestHome_ContainsSidebar(t *testing.T) {
	r := newTestRouter(t)

	rec := doRequest(t, r, http.MethodGet, "/")
	body := rec.Body.String()

	checks := []string{
		"Machine",
		"vps",
		"Overview",
		"System",
		"Docker",
		"Config",
	}
	for _, s := range checks {
		if !strings.Contains(body, s) {
			t.Errorf("response should contain %q", s)
		}
	}
}

func TestStatic_HTMXServed(t *testing.T) {
	r := newTestRouter(t)

	rec := doRequest(t, r, http.MethodGet, "/static/htmx.min.js")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	if rec.Body.Len() < 1000 {
		t.Errorf("htmx.min.js seems too small: %d bytes", rec.Body.Len())
	}
}

func TestStatic_UnknownFile_404(t *testing.T) {
	r := newTestRouter(t)

	rec := doRequest(t, r, http.MethodGet, "/static/does-not-exist.js")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
