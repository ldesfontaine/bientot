package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
)

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func makePush(machineID, nonce string) *bientotv1.PushRequest {
	now := time.Now().UnixNano()
	return &bientotv1.PushRequest{
		V:           1,
		MachineId:   machineID,
		TimestampNs: now,
		Nonce:       nonce,
		Modules: []*bientotv1.ModuleData{
			{
				Module:      "heartbeat",
				TimestampNs: now,
				Metrics: []*bientotv1.Metric{
					{Name: "up", Value: 1.0},
				},
				Metadata: map[string]string{"hostname": "test-host"},
			},
			{
				Module:      "system",
				TimestampNs: now,
				Metrics: []*bientotv1.Metric{
					{Name: "memory_used_bytes", Value: 1_800_000_000},
					{Name: "cpu_user_seconds_total", Value: 12345.67, Labels: map[string]string{"cpu": "0"}},
					{Name: "load_average_1m", Value: 0.42},
				},
			},
		},
	}
}

func TestSavePush_InsertsPush(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	if err := s.SavePush(ctx, makePush("vps", "nonce-1")); err != nil {
		t.Fatalf("SavePush: %v", err)
	}

	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pushes`).Scan(&count)
	if err != nil {
		t.Fatalf("count pushes: %v", err)
	}
	if count != 1 {
		t.Errorf("pushes count = %d, want 1", count)
	}
}

func TestSavePush_InsertsAllMetrics(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	if err := s.SavePush(ctx, makePush("vps", "nonce-2")); err != nil {
		t.Fatalf("SavePush: %v", err)
	}

	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM metrics`).Scan(&count)
	if err != nil {
		t.Fatalf("count metrics: %v", err)
	}
	if count != 4 {
		t.Errorf("metrics count = %d, want 4", count)
	}
}

func TestSavePush_MetricsHaveCorrectFields(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	if err := s.SavePush(ctx, makePush("vps", "nonce-3")); err != nil {
		t.Fatalf("SavePush: %v", err)
	}

	var (
		module   string
		name     string
		value    float64
		labelStr sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT module, name, value, labels FROM metrics WHERE name = 'cpu_user_seconds_total'`,
	).Scan(&module, &name, &value, &labelStr)
	if err != nil {
		t.Fatalf("query metric: %v", err)
	}

	if module != "system" {
		t.Errorf("module = %q, want %q", module, "system")
	}
	if value != 12345.67 {
		t.Errorf("value = %v, want 12345.67", value)
	}
	if !labelStr.Valid {
		t.Error("labels should be non-NULL for cpu metric")
	}
	if labelStr.String != `{"cpu":"0"}` {
		t.Errorf("labels = %q, want %q", labelStr.String, `{"cpu":"0"}`)
	}
}

func TestSavePush_MetricWithoutLabelsIsNull(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	if err := s.SavePush(ctx, makePush("vps", "nonce-4")); err != nil {
		t.Fatalf("SavePush: %v", err)
	}

	var labels sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT labels FROM metrics WHERE name = 'up'`,
	).Scan(&labels)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if labels.Valid {
		t.Errorf("labels for 'up' should be NULL, got %q", labels.String)
	}
}

func TestSavePush_UpsertsAgent_FirstPush(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	if err := s.SavePush(ctx, makePush("vps", "nonce-5")); err != nil {
		t.Fatalf("SavePush: %v", err)
	}

	var firstSeen, lastPush int64
	err := s.db.QueryRowContext(ctx,
		`SELECT first_seen_at, last_push_at FROM agents WHERE machine_id = 'vps'`,
	).Scan(&firstSeen, &lastPush)
	if err != nil {
		t.Fatalf("query agent: %v", err)
	}

	if firstSeen == 0 || lastPush == 0 {
		t.Errorf("timestamps should be set, got first=%d last=%d", firstSeen, lastPush)
	}
	if firstSeen != lastPush {
		t.Errorf("on first push, first_seen_at should equal last_push_at: %d vs %d", firstSeen, lastPush)
	}
}

func TestSavePush_UpsertsAgent_SecondPushKeepsFirstSeen(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	if err := s.SavePush(ctx, makePush("vps", "nonce-6a")); err != nil {
		t.Fatalf("first SavePush: %v", err)
	}

	var firstSeenAfterFirst int64
	s.db.QueryRowContext(ctx,
		`SELECT first_seen_at FROM agents WHERE machine_id = 'vps'`,
	).Scan(&firstSeenAfterFirst)

	time.Sleep(10 * time.Millisecond)

	if err := s.SavePush(ctx, makePush("vps", "nonce-6b")); err != nil {
		t.Fatalf("second SavePush: %v", err)
	}

	var firstSeenAfterSecond, lastPushAfterSecond int64
	s.db.QueryRowContext(ctx,
		`SELECT first_seen_at, last_push_at FROM agents WHERE machine_id = 'vps'`,
	).Scan(&firstSeenAfterSecond, &lastPushAfterSecond)

	if firstSeenAfterFirst != firstSeenAfterSecond {
		t.Errorf("first_seen_at changed: was %d, now %d", firstSeenAfterFirst, firstSeenAfterSecond)
	}
	if lastPushAfterSecond <= firstSeenAfterSecond {
		t.Errorf("last_push_at should advance: first_seen=%d last=%d", firstSeenAfterSecond, lastPushAfterSecond)
	}
}

func TestSavePush_TwoAgentsIsolated(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	if err := s.SavePush(ctx, makePush("vps", "nonce-7a")); err != nil {
		t.Fatalf("vps SavePush: %v", err)
	}
	if err := s.SavePush(ctx, makePush("pi", "nonce-7b")); err != nil {
		t.Fatalf("pi SavePush: %v", err)
	}

	var count int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agents`).Scan(&count)
	if count != 2 {
		t.Errorf("agents count = %d, want 2", count)
	}

	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM metrics`).Scan(&count)
	if count != 8 {
		t.Errorf("metrics count = %d, want 8", count)
	}
}

func TestSavePush_DuplicateNonceFails(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	if err := s.SavePush(ctx, makePush("vps", "same-nonce")); err != nil {
		t.Fatalf("first SavePush: %v", err)
	}

	err := s.SavePush(ctx, makePush("vps", "same-nonce"))
	if err == nil {
		t.Fatal("second SavePush with same nonce should fail, got nil")
	}
}

func TestSavePush_RollbackOnMetricFailure(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	if err := s.SavePush(ctx, makePush("vps", "nonce-good")); err != nil {
		t.Fatalf("good SavePush: %v", err)
	}

	failingPush := makePush("vps", "nonce-good")
	if err := s.SavePush(ctx, failingPush); err == nil {
		t.Fatal("expected error")
	}

	var count int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM metrics`).Scan(&count)
	if count != 4 {
		t.Errorf("metrics count after rollback = %d, want 4 (no pollution)", count)
	}
}

func TestSavePush_NilRequest(t *testing.T) {
	s := newTestStorage(t)

	err := s.SavePush(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request, got nil")
	}
}

func TestSavePush_EmptyModules(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	req := &bientotv1.PushRequest{
		V:           1,
		MachineId:   "empty-agent",
		TimestampNs: time.Now().UnixNano(),
		Nonce:       "empty-nonce",
	}

	if err := s.SavePush(ctx, req); err != nil {
		t.Fatalf("SavePush with no modules: %v", err)
	}

	var pushCount, metricCount int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pushes`).Scan(&pushCount)
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM metrics`).Scan(&metricCount)

	if pushCount != 1 {
		t.Errorf("pushes count = %d, want 1", pushCount)
	}
	if metricCount != 0 {
		t.Errorf("metrics count = %d, want 0", metricCount)
	}
}

func TestSavePush_RawPayloadRoundtrips(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	original := makePush("vps", "nonce-roundtrip")
	if err := s.SavePush(ctx, original); err != nil {
		t.Fatalf("SavePush: %v", err)
	}

	var rawBytes []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT raw_payload FROM pushes WHERE nonce = 'nonce-roundtrip'`,
	).Scan(&rawBytes)
	if err != nil {
		t.Fatalf("read raw_payload: %v", err)
	}

	var decoded bientotv1.PushRequest
	if err := proto.Unmarshal(rawBytes, &decoded); err != nil {
		t.Fatalf("unmarshal raw_payload: %v", err)
	}
	if decoded.MachineId != original.MachineId {
		t.Errorf("machine_id roundtrip mismatch: %q vs %q", decoded.MachineId, original.MachineId)
	}
	if decoded.Nonce != original.Nonce {
		t.Errorf("nonce roundtrip mismatch: %q vs %q", decoded.Nonce, original.Nonce)
	}
	if len(decoded.Modules) != len(original.Modules) {
		t.Errorf("modules count roundtrip: %d vs %d", len(decoded.Modules), len(original.Modules))
	}
}
