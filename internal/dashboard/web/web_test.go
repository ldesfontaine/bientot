package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestRouter(t *testing.T) *Router {
	t.Helper()
	return &Router{
		db:  nil,
		log: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
}

func doRequest(t *testing.T, r *Router, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	r.BuildHandler().ServeHTTP(rec, req)
	return rec
}

func TestHome_Returns200(t *testing.T) {
	r := newTestRouter(t)

	rec := doRequest(t, r, http.MethodGet, "/")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestHome_ContentTypeText(t *testing.T) {
	r := newTestRouter(t)

	rec := doRequest(t, r, http.MethodGet, "/")

	ct := rec.Header().Get("Content-Type")
	if ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
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
