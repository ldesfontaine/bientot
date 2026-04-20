package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
)

// newTestRouterWithDB returns a Router backed by a real (temp) SQLite DB.
// Unlike newTestRouter (health_test.go), this one is for handlers that
// need to read/write actual data.
func newTestRouterWithDB(t *testing.T, threshold time.Duration) (*Router, *storage.Storage) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	rt := &Router{
		db:               db,
		log:              slog.New(slog.NewJSONHandler(io.Discard, nil)),
		offlineThreshold: threshold,
	}
	return rt, db
}

// savePushAtTime saves a minimal push for machineID at the given timestamp.
func savePushAtTime(t *testing.T, db *storage.Storage, machineID, nonce string, ts time.Time) {
	t.Helper()
	req := &bientotv1.PushRequest{
		V:           1,
		MachineId:   machineID,
		TimestampNs: ts.UnixNano(),
		Nonce:       nonce,
		Modules: []*bientotv1.ModuleData{
			{
				Module:      "heartbeat",
				TimestampNs: ts.UnixNano(),
				Metrics:     []*bientotv1.Metric{{Name: "up", Value: 1}},
			},
		},
	}
	if err := db.SavePush(context.Background(), req); err != nil {
		t.Fatalf("save push: %v", err)
	}
}

// ─── handleListAgents ────────────────────────────────────

func TestListAgents_Empty(t *testing.T) {
	r, _ := newTestRouterWithDB(t, 2*time.Minute)

	rec := doRequest(t, r, http.MethodGet, "/api/agents")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if body != "[]\n" {
		t.Errorf("body = %q, want [] (empty array, not null)", body)
	}
}

func TestListAgents_OneOnline(t *testing.T) {
	r, db := newTestRouterWithDB(t, 2*time.Minute)

	savePushAtTime(t, db, "vps", "n1", time.Now())

	rec := doRequest(t, r, http.MethodGet, "/api/agents")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var agents []agentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &agents); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("count = %d, want 1", len(agents))
	}
	if agents[0].MachineID != "vps" {
		t.Errorf("machineId = %q, want vps", agents[0].MachineID)
	}
	if agents[0].Status != statusOnline {
		t.Errorf("status = %q, want online", agents[0].Status)
	}
	if agents[0].FirstSeenAt == 0 {
		t.Error("firstSeenAt should not be 0")
	}
	if agents[0].LastPushAt == 0 {
		t.Error("lastPushAt should not be 0")
	}
}

func TestListAgents_SortedAlphabetically(t *testing.T) {
	r, db := newTestRouterWithDB(t, 2*time.Minute)

	now := time.Now()
	savePushAtTime(t, db, "vps", "v1", now)
	savePushAtTime(t, db, "pi", "p1", now)
	savePushAtTime(t, db, "laptop", "l1", now)

	rec := doRequest(t, r, http.MethodGet, "/api/agents")

	var agents []agentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &agents); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(agents) != 3 {
		t.Fatalf("count = %d, want 3", len(agents))
	}
	want := []string{"laptop", "pi", "vps"}
	for i, w := range want {
		if agents[i].MachineID != w {
			t.Errorf("agents[%d] = %q, want %q", i, agents[i].MachineID, w)
		}
	}
}

func TestListAgents_ContentTypeIsJSON(t *testing.T) {
	r, db := newTestRouterWithDB(t, 2*time.Minute)
	savePushAtTime(t, db, "vps", "n1", time.Now())

	rec := doRequest(t, r, http.MethodGet, "/api/agents")

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
}

// ─── toDTO (unit test of status logic) ───────────────────

func TestToDTO_Online(t *testing.T) {
	now := time.Now()
	agent := storage.Agent{
		MachineID:   "vps",
		FirstSeenAt: now.Add(-1 * time.Hour),
		LastPushAt:  now.Add(-30 * time.Second),
	}

	dto := toDTO(agent, now, 2*time.Minute)

	if dto.Status != statusOnline {
		t.Errorf("status = %q, want online", dto.Status)
	}
}

func TestToDTO_Offline(t *testing.T) {
	now := time.Now()
	agent := storage.Agent{
		MachineID:   "vps",
		FirstSeenAt: now.Add(-1 * time.Hour),
		LastPushAt:  now.Add(-5 * time.Minute),
	}

	dto := toDTO(agent, now, 2*time.Minute)

	if dto.Status != statusOffline {
		t.Errorf("status = %q, want offline", dto.Status)
	}
}

func TestToDTO_ExactlyAtThreshold(t *testing.T) {
	now := time.Now()
	agent := storage.Agent{
		MachineID:   "vps",
		FirstSeenAt: now.Add(-1 * time.Hour),
		LastPushAt:  now.Add(-2 * time.Minute),
	}

	dto := toDTO(agent, now, 2*time.Minute)

	if dto.Status != statusOnline {
		t.Errorf("at exact threshold, status = %q, want online (strict >)", dto.Status)
	}
}

func TestToDTO_TimestampsInMilliseconds(t *testing.T) {
	fixedTime := time.Date(2026, 4, 20, 14, 0, 0, 0, time.UTC)
	agent := storage.Agent{
		MachineID:   "vps",
		FirstSeenAt: fixedTime,
		LastPushAt:  fixedTime,
	}

	dto := toDTO(agent, fixedTime, 2*time.Minute)

	expected := fixedTime.UnixMilli()
	if dto.FirstSeenAt != expected {
		t.Errorf("firstSeenAt = %d, want %d", dto.FirstSeenAt, expected)
	}
	if dto.LastPushAt != expected {
		t.Errorf("lastPushAt = %d, want %d", dto.LastPushAt, expected)
	}
}

// ─── status computation with stale agent (integration) ───

func TestListAgents_OfflineAgent(t *testing.T) {
	r, db := newTestRouterWithDB(t, 2*time.Minute)

	savePushAtTime(t, db, "vps", "n1", time.Now())

	// Force threshold impossibly strict to make the fresh agent count as offline.
	r.offlineThreshold = -1 * time.Second

	rec := doRequest(t, r, http.MethodGet, "/api/agents")

	var agents []agentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &agents); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("count = %d", len(agents))
	}
	if agents[0].Status != statusOffline {
		t.Errorf("status = %q, want offline (threshold: %v)", agents[0].Status, r.offlineThreshold)
	}
}
