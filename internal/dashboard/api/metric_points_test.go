package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
)

func savePushAtTimestamp(t *testing.T, db *storage.Storage, machineID, nonce, metricName string, value float64, ts time.Time) {
	t.Helper()
	req := &bientotv1.PushRequest{
		V:           1,
		MachineId:   machineID,
		TimestampNs: ts.UnixNano(),
		Nonce:       nonce,
		Modules: []*bientotv1.ModuleData{
			{
				Module:      "system",
				TimestampNs: ts.UnixNano(),
				Metrics:     []*bientotv1.Metric{{Name: metricName, Value: value}},
			},
		},
	}
	if err := db.SavePush(context.Background(), req); err != nil {
		t.Fatalf("SavePush: %v", err)
	}
}

// ─── Validation des paramètres ───────────────────────────

func TestGetMetricPoints_MissingName(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)
	savePushAtTimestamp(t, db, "vps", "n1", "cpu", 1, time.Now())

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metric-points")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestGetMetricPoints_AgentNotFound(t *testing.T) {
	s, _ := newTestServerWithDB(t, 2*time.Minute)

	rec := doRequest(t, s, http.MethodGet, "/api/agents/nope/metric-points?name=cpu")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestGetMetricPoints_BadRange(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)
	savePushAtTimestamp(t, db, "vps", "n1", "cpu", 1, time.Now())

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metric-points?name=cpu&range=invalid")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestGetMetricPoints_PartialAbsolute(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)
	savePushAtTimestamp(t, db, "vps", "n1", "cpu", 1, time.Now())

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metric-points?name=cpu&start=1000")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// ─── Happy path ──────────────────────────────────────────

func TestGetMetricPoints_AgentExistsNoPoints(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)

	savePushAtTimestamp(t, db, "vps", "n1", "memory", 100, time.Now())

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metric-points?name=cpu&range=1h")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var resp metricPointsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Points) != 0 {
		t.Errorf("points count = %d, want 0", len(resp.Points))
	}
	if resp.Points == nil {
		t.Error("points should be empty array, not null")
	}
}

func TestGetMetricPoints_ReturnsAllPointsInRange(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)

	now := time.Now()
	for i := 0; i < 5; i++ {
		ts := now.Add(time.Duration(-i*5) * time.Minute)
		savePushAtTimestamp(t, db, "vps", fmt.Sprintf("n%d", i), "cpu", float64(i*10), ts)
	}

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metric-points?name=cpu&range=1h")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var resp metricPointsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Points) != 5 {
		t.Errorf("points count = %d, want 5", len(resp.Points))
	}
}

func TestGetMetricPoints_PointsSortedAscending(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)

	now := time.Now()
	savePushAtTimestamp(t, db, "vps", "n3", "cpu", 30, now.Add(-3*time.Minute))
	savePushAtTimestamp(t, db, "vps", "n1", "cpu", 10, now.Add(-1*time.Minute))
	savePushAtTimestamp(t, db, "vps", "n2", "cpu", 20, now.Add(-2*time.Minute))

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metric-points?name=cpu&range=1h")

	var resp metricPointsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Points) != 3 {
		t.Fatalf("count = %d", len(resp.Points))
	}

	for i := 1; i < len(resp.Points); i++ {
		if resp.Points[i].T <= resp.Points[i-1].T {
			t.Errorf("points not sorted at index %d: %d <= %d",
				i, resp.Points[i].T, resp.Points[i-1].T)
		}
	}
}

func TestGetMetricPoints_RangeFiltering(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)

	now := time.Now()
	savePushAtTimestamp(t, db, "vps", "recent", "cpu", 1, now.Add(-30*time.Minute))
	savePushAtTimestamp(t, db, "vps", "old", "cpu", 99, now.Add(-2*time.Hour))

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metric-points?name=cpu&range=1h")

	var resp metricPointsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Points) != 1 {
		t.Errorf("count = %d, want 1 (range=1h excludes the 2h-old point)", len(resp.Points))
	}
	if resp.Points[0].V != 1 {
		t.Errorf("got value %v, want 1 (recent point)", resp.Points[0].V)
	}
}

func TestGetMetricPoints_ResponseStructure(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)
	savePushAtTimestamp(t, db, "vps", "n1", "cpu", 42, time.Now())

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metric-points?name=cpu&range=1h")

	var resp metricPointsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Name != "cpu" {
		t.Errorf("name = %q, want cpu", resp.Name)
	}
	if resp.MachineID != "vps" {
		t.Errorf("machineId = %q, want vps", resp.MachineID)
	}
	if resp.Start == 0 || resp.End == 0 {
		t.Errorf("start/end should be non-zero: start=%d end=%d", resp.Start, resp.End)
	}
	if resp.End <= resp.Start {
		t.Errorf("end (%d) should be > start (%d)", resp.End, resp.Start)
	}
}

func TestGetMetricPoints_DefaultRangeIs24h(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)
	savePushAtTimestamp(t, db, "vps", "n1", "cpu", 1, time.Now())

	rec := doRequest(t, s, http.MethodGet, "/api/agents/vps/metric-points?name=cpu")

	var resp metricPointsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rangeMs := resp.End - resp.Start
	expectedMs := int64(24 * time.Hour / time.Millisecond)
	if abs(rangeMs-expectedMs) > 1000 {
		t.Errorf("default range = %d ms, want ~%d ms (24h)", rangeMs, expectedMs)
	}
}

func TestGetMetricPoints_AbsoluteMode(t *testing.T) {
	s, db := newTestServerWithDB(t, 2*time.Minute)

	pushTime := time.Now().Add(-2 * time.Hour)
	savePushAtTimestamp(t, db, "vps", "n1", "cpu", 42, pushTime)

	startMs := pushTime.Add(-1 * time.Minute).UnixMilli()
	endMs := pushTime.Add(1 * time.Minute).UnixMilli()
	url := fmt.Sprintf("/api/agents/vps/metric-points?name=cpu&start=%d&end=%d", startMs, endMs)

	rec := doRequest(t, s, http.MethodGet, url)

	var resp metricPointsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Points) != 1 {
		t.Errorf("count = %d, want 1", len(resp.Points))
	}
	if resp.Start != startMs || resp.End != endMs {
		t.Errorf("start/end mismatch: got [%d, %d], want [%d, %d]",
			resp.Start, resp.End, startMs, endMs)
	}
}

// ─── parseTimeRange (unit) ───────────────────────────────

func TestParseTimeRange_Default(t *testing.T) {
	now := time.Date(2026, 4, 20, 14, 0, 0, 0, time.UTC)
	q := map[string][]string{}

	start, end, err := parseTimeRange(q, now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !end.Equal(now) {
		t.Errorf("end = %v, want %v", end, now)
	}
	if !start.Equal(now.Add(-24 * time.Hour)) {
		t.Errorf("start = %v, want now-24h", start)
	}
}

func TestParseTimeRange_Relative(t *testing.T) {
	now := time.Date(2026, 4, 20, 14, 0, 0, 0, time.UTC)
	q := map[string][]string{"range": {"3h"}}

	start, end, err := parseTimeRange(q, now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !start.Equal(now.Add(-3 * time.Hour)) {
		t.Errorf("start = %v, want now-3h", start)
	}
	if !end.Equal(now) {
		t.Errorf("end = %v, want now", end)
	}
}

func TestParseTimeRange_Absolute(t *testing.T) {
	now := time.Date(2026, 4, 20, 14, 0, 0, 0, time.UTC)
	q := map[string][]string{
		"start": {"1776000000000"},
		"end":   {"1776100000000"},
	}

	start, end, err := parseTimeRange(q, now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if start.UnixMilli() != 1776000000000 {
		t.Errorf("start ms = %d", start.UnixMilli())
	}
	if end.UnixMilli() != 1776100000000 {
		t.Errorf("end ms = %d", end.UnixMilli())
	}
}

func TestParseTimeRange_Errors(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name string
		q    map[string][]string
	}{
		{"invalid_duration", map[string][]string{"range": {"xyz"}}},
		{"negative_duration", map[string][]string{"range": {"-1h"}}},
		{"start_only", map[string][]string{"start": {"1000"}}},
		{"end_only", map[string][]string{"end": {"1000"}}},
		{"start_after_end", map[string][]string{"start": {"2000"}, "end": {"1000"}}},
		{"non_numeric_start", map[string][]string{"start": {"abc"}, "end": {"1000"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseTimeRange(tc.q, now)
			if err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
