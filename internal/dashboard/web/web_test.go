package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
)

// newTestRouterWithDB constructs a Router backed by a fresh temp SQLite DB.
func newTestRouterWithDB(t *testing.T) (*Router, *storage.Storage) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	r, err := NewRouter(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		db,
		Config{
			DevMode:          false,
			OfflineThreshold: 2 * time.Minute,
			Version:          "0.0.0-test",
		},
	)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return r, db
}

// newTestRouter returns a Router without DB. Useful for handlers that don't
// touch storage (e.g. static files), but most tests should use the DB-backed
// version.
func newTestRouter(t *testing.T) *Router {
	t.Helper()
	r, err := NewRouter(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		nil,
		Config{DevMode: false, Version: "0.0.0-test"},
	)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return r
}

// savePushSimple is a helper for tests that need at least one agent.
func savePushSimple(t *testing.T, db *storage.Storage, machineID, nonce string) {
	t.Helper()
	ts := time.Now().UnixNano()
	req := &bientotv1.PushRequest{
		V:           1,
		MachineId:   machineID,
		TimestampNs: ts,
		Nonce:       nonce,
		Modules: []*bientotv1.ModuleData{
			{Module: "heartbeat", TimestampNs: ts, Metrics: []*bientotv1.Metric{{Name: "up", Value: 1}}},
		},
	}
	if err := db.SavePush(context.Background(), req); err != nil {
		t.Fatalf("SavePush: %v", err)
	}
}

func doRequest(t *testing.T, r *Router, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	r.BuildHandler().ServeHTTP(rec, req)
	return rec
}

// ─── Home: redirect / empty ─────────────────────────────

func TestHome_NoAgents_RendersEmptyState(t *testing.T) {
	r, _ := newTestRouterWithDB(t)

	rec := doRequest(t, r, http.MethodGet, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "No agents yet") {
		t.Error("body should contain empty-state title")
	}
	if strings.Contains(body, "app-shell") {
		t.Error("empty-state should not include sidebar layout")
	}
}

func TestHome_WithAgents_RedirectsToFirst(t *testing.T) {
	r, db := newTestRouterWithDB(t)
	savePushSimple(t, db, "vps", "n1")
	savePushSimple(t, db, "pi", "n2")
	savePushSimple(t, db, "laptop", "n3")

	rec := doRequest(t, r, http.MethodGet, "/")

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (See Other)", rec.Code)
	}

	loc := rec.Header().Get("Location")
	// Alphabetical first: laptop < pi < vps
	if loc != "/agents/laptop" {
		t.Errorf("Location = %q, want /agents/laptop", loc)
	}
}

// ─── Static assets ───────────────────────────────────────

func TestStatic_CSSServed(t *testing.T) {
	r := newTestRouter(t)

	rec := doRequest(t, r, http.MethodGet, "/static/css/app.css")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "--bg-0") {
		t.Error("CSS should contain token --bg-0")
	}
}

func TestStatic_HTMXServed(t *testing.T) {
	r := newTestRouter(t)

	rec := doRequest(t, r, http.MethodGet, "/static/htmx.min.js")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
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
			t.Errorf("%s: body too small (%d bytes)", f, rec.Body.Len())
		}
	}
}

func TestStatic_UnknownFile_404(t *testing.T) {
	r := newTestRouter(t)

	rec := doRequest(t, r, http.MethodGet, "/static/does-not-exist.js")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
