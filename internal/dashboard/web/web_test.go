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

func TestStatic_CSSServed(t *testing.T) {
	r := newTestRouter(t)

	rec := doRequest(t, r, http.MethodGet, "/static/css/app.css")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/css") {
		t.Errorf("Content-Type = %q, want text/css", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "--bg-0") {
		t.Error("CSS should contain token --bg-0")
	}
	if !strings.Contains(body, "Geist") {
		t.Error("CSS should reference Geist font")
	}
}

func TestStatic_FontsServed(t *testing.T) {
	r := newTestRouter(t)

	fonts := []string{
		"/static/fonts/Geist-Variable.woff2",
		"/static/fonts/GeistMono-Variable.woff2",
	}
	for _, f := range fonts {
		rec := doRequest(t, r, http.MethodGet, f)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", f, rec.Code)
		}
		if rec.Body.Len() < 10_000 {
			t.Errorf("%s: body too small (%d bytes), expected valid WOFF2", f, rec.Body.Len())
		}
	}
}

func TestHome_UsesTokens(t *testing.T) {
	r := newTestRouter(t)

	rec := doRequest(t, r, http.MethodGet, "/")
	body := rec.Body.String()

	if strings.Contains(body, "tailwindcss") || strings.Contains(body, "jsdelivr") {
		t.Error("HTML should not reference Tailwind/jsdelivr anymore")
	}
	if !strings.Contains(body, `href="/static/css/app.css"`) {
		t.Error("HTML should link /static/css/app.css")
	}
	classes := []string{"app-shell", "sidebar", "brand", "nav-item", "card"}
	for _, c := range classes {
		if !strings.Contains(body, c) {
			t.Errorf("HTML should use class %q", c)
		}
	}
}
