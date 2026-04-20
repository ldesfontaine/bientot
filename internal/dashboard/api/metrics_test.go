package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"testing"
	"time"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
)

func savePushMulti(t *testing.T, db *storage.Storage, machineID, nonce string, metrics []*bientotv1.Metric) {
	t.Helper()
	ts := time.Now().UnixNano()
	req := &bientotv1.PushRequest{
		V:           1,
		MachineId:   machineID,
		TimestampNs: ts,
		Nonce:       nonce,
		Modules: []*bientotv1.ModuleData{
			{Module: "system", TimestampNs: ts, Metrics: metrics},
		},
	}
	if err := db.SavePush(context.Background(), req); err != nil {
		t.Fatalf("SavePush: %v", err)
	}
}

// ─── 404 — Agent inexistant ──────────────────────────────

func TestGetLatestMetrics_AgentNotFound(t *testing.T) {
	s, _ := newTestServerWithDB(t, 2*time.Minute)

	rec := doRequest(t, s, http.MethodGet, "/api/agents/nonexistent/metrics")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error == "" {
		t.Error("error message should not be empty")
	}
}

// ─── 200 — Agent existe mais sans métriques ──────────────

func TestGetLatestMetrics_AgentExistsNoMetrics(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)

	ts := time.Now().UnixNano()
	req := &bientotv1.PushRequest{
		V:           1,
		MachineId:   "vps",
		TimestampNs: ts,
		Nonce:       "n1",
	}
	if err := db.SavePush(context.Background(), req); err != nil {
		t.Fatalf("SavePush: %v", err)
	}

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metrics")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "[]\n" {
		t.Errorf("body = %q, want []", body)
	}
}

// ─── 200 — Agent avec métriques ──────────────────────────

func TestGetLatestMetrics_ReturnsAllMetrics(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)

	savePushMulti(t, db, "vps", "n1", []*bientotv1.Metric{
		{Name: "cpu", Value: 42},
		{Name: "memory", Value: 1024},
		{Name: "up", Value: 1},
	})

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metrics")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var metrics []metricDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &metrics); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(metrics) != 3 {
		t.Fatalf("count = %d, want 3", len(metrics))
	}
}

func TestGetLatestMetrics_SortedAlphabetically(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)

	savePushMulti(t, db, "vps", "n1", []*bientotv1.Metric{
		{Name: "zeta", Value: 1},
		{Name: "alpha", Value: 1},
		{Name: "middle", Value: 1},
	})

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metrics")

	var metrics []metricDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &metrics); err != nil {
		t.Fatalf("decode: %v", err)
	}

	want := []string{"alpha", "middle", "zeta"}
	for i, w := range want {
		if metrics[i].Name != w {
			t.Errorf("metrics[%d].name = %q, want %q", i, metrics[i].Name, w)
		}
	}
}

func TestGetLatestMetrics_ReturnsLatestValue(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)

	savePushMulti(t, db, "vps", "n1", []*bientotv1.Metric{{Name: "cpu", Value: 10}})
	savePushMulti(t, db, "vps", "n2", []*bientotv1.Metric{{Name: "cpu", Value: 99}})

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metrics")

	var metrics []metricDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &metrics); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(metrics) != 1 {
		t.Fatalf("count = %d, want 1", len(metrics))
	}
	if metrics[0].Value != 99 {
		t.Errorf("value = %v, want 99 (latest)", metrics[0].Value)
	}
}

func TestGetLatestMetrics_LabelsNeverNull(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)

	savePushMulti(t, db, "vps", "n1", []*bientotv1.Metric{
		{Name: "up", Value: 1},
	})

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metrics")

	// Parse as raw JSON to verify the wire-level text is `{}` not `null`.
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}

	labels, ok := raw[0]["labels"]
	if !ok {
		t.Fatal("labels field missing")
	}
	if string(labels) != "{}" {
		t.Errorf("labels = %s, want {} (empty object, not null)", labels)
	}
}

func TestGetLatestMetrics_TimestampInMs(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)

	savePushMulti(t, db, "vps", "n1", []*bientotv1.Metric{{Name: "up", Value: 1}})

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metrics")

	var metrics []metricDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &metrics); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Timestamp should be a plausible Unix milli (> 10^12 since year 2001).
	if metrics[0].Timestamp < 1_000_000_000_000 {
		t.Errorf("timestamp = %d, should be Unix ms", metrics[0].Timestamp)
	}
}

// ─── toMetricDTO (unit tests) ────────────────────────────

func TestToMetricDTO_EmptyLabels(t *testing.T) {
	m := storage.Metric{
		Name:        "cpu",
		Value:       42,
		Module:      "system",
		Labels:      nil,
		TimestampNs: 1_776_699_707_263_000_000,
	}

	dto := toMetricDTO(m)

	if dto.Labels == nil {
		t.Error("labels should never be nil")
	}
	if len(dto.Labels) != 0 {
		t.Errorf("labels = %v, want empty map", dto.Labels)
	}
}

func TestToMetricDTO_WithLabels(t *testing.T) {
	m := storage.Metric{
		Name:        "cpu",
		Value:       42,
		Module:      "system",
		Labels:      map[string]string{"cpu": "0", "mode": "user"},
		TimestampNs: 1_776_699_707_263_000_000,
	}

	dto := toMetricDTO(m)

	if len(dto.Labels) != 2 {
		t.Fatalf("len(labels) = %d, want 2", len(dto.Labels))
	}
	if dto.Labels["cpu"] != "0" {
		t.Errorf("labels[cpu] = %q, want 0", dto.Labels["cpu"])
	}
}

func TestToMetricDTO_TimestampConversion(t *testing.T) {
	m := storage.Metric{
		TimestampNs: 1_776_699_707_263_000_000,
	}

	dto := toMetricDTO(m)

	expected := int64(1_776_699_707_263)
	if dto.Timestamp != expected {
		t.Errorf("timestamp = %d, want %d", dto.Timestamp, expected)
	}
}

// ─── Deterministic ordering across iterations ────────────

func TestGetLatestMetrics_DeterministicOrdering(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)

	savePushMulti(t, db, "vps", "n1", []*bientotv1.Metric{
		{Name: "b_metric", Value: 1},
		{Name: "a_metric", Value: 1},
		{Name: "c_metric", Value: 1},
	})

	var reference []string

	for i := 0; i < 10; i++ {
		rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metrics")
		var metrics []metricDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &metrics); err != nil {
			t.Fatalf("decode: %v", err)
		}

		names := make([]string, len(metrics))
		for j, m := range metrics {
			names[j] = m.Name
		}

		if i == 0 {
			reference = names
			sorted := append([]string(nil), names...)
			sort.Strings(sorted)
			for k := range names {
				if names[k] != sorted[k] {
					t.Fatalf("iteration 0: not alphabetical: %v", names)
				}
			}
		} else {
			for j := range names {
				if names[j] != reference[j] {
					t.Errorf("iteration %d: names[%d] = %q, want %q", i, j, names[j], reference[j])
				}
			}
		}
	}
}
