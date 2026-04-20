package storage

import (
	"context"
	"testing"
	"time"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
)

func savedPush(t *testing.T, s *Storage, machineID, nonce string, metrics []*bientotv1.Metric, atNs int64) {
	t.Helper()
	req := &bientotv1.PushRequest{
		V:           1,
		MachineId:   machineID,
		TimestampNs: atNs,
		Nonce:       nonce,
		Modules: []*bientotv1.ModuleData{
			{
				Module:      "system",
				TimestampNs: atNs,
				Metrics:     metrics,
			},
		},
	}
	if err := s.SavePush(context.Background(), req); err != nil {
		t.Fatalf("SavePush: %v", err)
	}
}

// ─── ListAgents ────────────────────────────────────────

func TestListAgents_Empty(t *testing.T) {
	s := newTestStorage(t)

	agents, err := s.ListAgents(context.Background())
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if agents == nil {
		t.Error("ListAgents returned nil, want empty slice")
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestListAgents_TwoAgentsOrdered(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now().UnixNano()

	savedPush(t, s, "pi", "p1", []*bientotv1.Metric{{Name: "up", Value: 1}}, now)
	savedPush(t, s, "vps", "v1", []*bientotv1.Metric{{Name: "up", Value: 1}}, now)

	agents, err := s.ListAgents(context.Background())
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}

	if agents[0].MachineID != "pi" || agents[1].MachineID != "vps" {
		t.Errorf("order = [%q, %q], want [pi, vps]",
			agents[0].MachineID, agents[1].MachineID)
	}
}

func TestListAgents_TimestampsReturned(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now().UnixNano()

	savedPush(t, s, "vps", "n1", []*bientotv1.Metric{{Name: "up", Value: 1}}, now)

	agents, _ := s.ListAgents(context.Background())
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	a := agents[0]
	if a.FirstSeenAt.IsZero() {
		t.Error("FirstSeenAt is zero")
	}
	if a.LastPushAt.IsZero() {
		t.Error("LastPushAt is zero")
	}
}

// ─── GetLatestMetrics ──────────────────────────────────

func TestGetLatestMetrics_Empty(t *testing.T) {
	s := newTestStorage(t)

	metrics, err := s.GetLatestMetrics(context.Background(), "vps")
	if err != nil {
		t.Fatalf("GetLatestMetrics: %v", err)
	}
	if metrics == nil {
		t.Error("returned nil, want empty map")
	}
	if len(metrics) != 0 {
		t.Errorf("expected 0 metrics, got %d", len(metrics))
	}
}

func TestGetLatestMetrics_ReturnsLatestValue(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	t0 := time.Now().UnixNano()
	t1 := t0 + int64(time.Second)

	savedPush(t, s, "vps", "p1",
		[]*bientotv1.Metric{{Name: "cpu_user_seconds_total", Value: 10}},
		t0,
	)
	savedPush(t, s, "vps", "p2",
		[]*bientotv1.Metric{{Name: "cpu_user_seconds_total", Value: 20}},
		t1,
	)

	metrics, err := s.GetLatestMetrics(ctx, "vps")
	if err != nil {
		t.Fatalf("GetLatestMetrics: %v", err)
	}

	m, ok := metrics["cpu_user_seconds_total"]
	if !ok {
		t.Fatal("cpu metric not found")
	}
	if m.Value != 20 {
		t.Errorf("value = %v, want 20 (latest)", m.Value)
	}
}

func TestGetLatestMetrics_MultipleMetrics(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	now := time.Now().UnixNano()
	savedPush(t, s, "vps", "n1",
		[]*bientotv1.Metric{
			{Name: "cpu", Value: 42},
			{Name: "memory", Value: 1024},
			{Name: "load", Value: 0.5},
		},
		now,
	)

	metrics, _ := s.GetLatestMetrics(ctx, "vps")

	if len(metrics) != 3 {
		t.Errorf("expected 3 metrics, got %d", len(metrics))
	}
	if metrics["cpu"].Value != 42 {
		t.Errorf("cpu = %v, want 42", metrics["cpu"].Value)
	}
	if metrics["memory"].Value != 1024 {
		t.Errorf("memory = %v, want 1024", metrics["memory"].Value)
	}
}

func TestGetLatestMetrics_LabelsDeserialized(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	savedPush(t, s, "vps", "n1",
		[]*bientotv1.Metric{
			{Name: "cpu", Value: 42, Labels: map[string]string{"cpu": "0", "mode": "user"}},
		},
		time.Now().UnixNano(),
	)

	metrics, _ := s.GetLatestMetrics(ctx, "vps")
	m := metrics["cpu"]

	if len(m.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(m.Labels))
	}
	if m.Labels["cpu"] != "0" || m.Labels["mode"] != "user" {
		t.Errorf("labels = %v, want {cpu:0, mode:user}", m.Labels)
	}
}

func TestGetLatestMetrics_NoLabels(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	savedPush(t, s, "vps", "n1",
		[]*bientotv1.Metric{
			{Name: "up", Value: 1},
		},
		time.Now().UnixNano(),
	)

	metrics, _ := s.GetLatestMetrics(ctx, "vps")
	m := metrics["up"]

	if len(m.Labels) != 0 {
		t.Errorf("expected no labels, got %v", m.Labels)
	}
}

func TestGetLatestMetrics_IsolatedByAgent(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	now := time.Now().UnixNano()
	savedPush(t, s, "vps", "v1", []*bientotv1.Metric{{Name: "cpu", Value: 42}}, now)
	savedPush(t, s, "pi", "p1", []*bientotv1.Metric{{Name: "cpu", Value: 99}}, now)

	vpsMetrics, _ := s.GetLatestMetrics(ctx, "vps")
	piMetrics, _ := s.GetLatestMetrics(ctx, "pi")

	if vpsMetrics["cpu"].Value != 42 {
		t.Errorf("vps cpu = %v, want 42", vpsMetrics["cpu"].Value)
	}
	if piMetrics["cpu"].Value != 99 {
		t.Errorf("pi cpu = %v, want 99", piMetrics["cpu"].Value)
	}
}

// ─── GetMetricPoints ───────────────────────────────────

func TestGetMetricPoints_Empty(t *testing.T) {
	s := newTestStorage(t)

	now := time.Now()
	points, err := s.GetMetricPoints(
		context.Background(),
		"vps", "cpu",
		now.Add(-1*time.Hour), now,
	)
	if err != nil {
		t.Fatalf("GetMetricPoints: %v", err)
	}
	if points == nil {
		t.Error("returned nil, want empty slice")
	}
	if len(points) != 0 {
		t.Errorf("expected 0 points, got %d", len(points))
	}
}

func TestGetMetricPoints_MultiplePointsOrdered(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	base := time.Now().Add(-10 * time.Minute).UnixNano()
	savedPush(t, s, "vps", "p2", []*bientotv1.Metric{{Name: "cpu", Value: 20}}, base+int64(2*time.Minute))
	savedPush(t, s, "vps", "p3", []*bientotv1.Metric{{Name: "cpu", Value: 30}}, base+int64(4*time.Minute))
	savedPush(t, s, "vps", "p1", []*bientotv1.Metric{{Name: "cpu", Value: 10}}, base)

	points, err := s.GetMetricPoints(
		ctx, "vps", "cpu",
		time.Unix(0, base-int64(time.Second)),
		time.Unix(0, base+int64(10*time.Minute)),
	)
	if err != nil {
		t.Fatalf("GetMetricPoints: %v", err)
	}

	if len(points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(points))
	}

	if points[0].Value != 10 || points[1].Value != 20 || points[2].Value != 30 {
		t.Errorf("values = [%v, %v, %v], want [10, 20, 30]",
			points[0].Value, points[1].Value, points[2].Value)
	}

	if points[0].TimestampNs >= points[1].TimestampNs {
		t.Error("timestamps not ascending")
	}
}

func TestGetMetricPoints_HalfOpenInterval(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	t0 := time.Now().UnixNano()
	t1 := t0 + int64(time.Minute)
	t2 := t0 + int64(2*time.Minute)

	savedPush(t, s, "vps", "a", []*bientotv1.Metric{{Name: "cpu", Value: 1}}, t0)
	savedPush(t, s, "vps", "b", []*bientotv1.Metric{{Name: "cpu", Value: 2}}, t1)
	savedPush(t, s, "vps", "c", []*bientotv1.Metric{{Name: "cpu", Value: 3}}, t2)

	points, _ := s.GetMetricPoints(ctx, "vps", "cpu", time.Unix(0, t0), time.Unix(0, t2))

	if len(points) != 2 {
		t.Errorf("expected 2 points for [t0, t2), got %d", len(points))
	}
	if points[0].Value != 1 || points[1].Value != 2 {
		t.Errorf("values = [%v, %v], want [1, 2]", points[0].Value, points[1].Value)
	}
}

func TestGetMetricPoints_FilteredByMetricName(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	now := time.Now().UnixNano()
	savedPush(t, s, "vps", "n1",
		[]*bientotv1.Metric{
			{Name: "cpu", Value: 10},
			{Name: "memory", Value: 1024},
		},
		now,
	)

	points, _ := s.GetMetricPoints(
		ctx, "vps", "cpu",
		time.Unix(0, now-int64(time.Second)),
		time.Unix(0, now+int64(time.Second)),
	)

	if len(points) != 1 {
		t.Errorf("expected only cpu, got %d points", len(points))
	}
	if points[0].Value != 10 {
		t.Errorf("value = %v, want 10", points[0].Value)
	}
}

// ─── ListModulesForAgent ─────────────────────────────────

func TestListModulesForAgent_Empty(t *testing.T) {
	s := newTestStorage(t)

	modules, err := s.ListModulesForAgent(context.Background(), "vps")
	if err != nil {
		t.Fatalf("ListModulesForAgent: %v", err)
	}
	if modules == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(modules) != 0 {
		t.Errorf("expected 0 modules, got %d", len(modules))
	}
}

func TestListModulesForAgent_MultipleModules(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()
	now := time.Now().UnixNano()

	req := &bientotv1.PushRequest{
		V:           1,
		MachineId:   "vps",
		TimestampNs: now,
		Nonce:       "n1",
		Modules: []*bientotv1.ModuleData{
			{
				Module:      "heartbeat",
				TimestampNs: now,
				Metrics:     []*bientotv1.Metric{{Name: "up", Value: 1}},
			},
			{
				Module:      "system",
				TimestampNs: now,
				Metrics: []*bientotv1.Metric{
					{Name: "cpu", Value: 42},
					{Name: "memory", Value: 1024},
					{Name: "disk", Value: 512},
				},
			},
		},
	}
	if err := s.SavePush(ctx, req); err != nil {
		t.Fatalf("SavePush: %v", err)
	}

	modules, err := s.ListModulesForAgent(ctx, "vps")
	if err != nil {
		t.Fatalf("ListModulesForAgent: %v", err)
	}

	if len(modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(modules))
	}

	if modules[0].Module != "heartbeat" {
		t.Errorf("modules[0] = %q, want heartbeat", modules[0].Module)
	}
	if modules[0].MetricCount != 1 {
		t.Errorf("heartbeat metric count = %d, want 1", modules[0].MetricCount)
	}

	if modules[1].Module != "system" {
		t.Errorf("modules[1] = %q, want system", modules[1].Module)
	}
	if modules[1].MetricCount != 3 {
		t.Errorf("system metric count = %d, want 3", modules[1].MetricCount)
	}
}

func TestListModulesForAgent_CountsDistinctNames(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()
	now := time.Now().UnixNano()

	for i, nonce := range []string{"n1", "n2"} {
		req := &bientotv1.PushRequest{
			V:           1,
			MachineId:   "vps",
			TimestampNs: now + int64(i)*int64(time.Second),
			Nonce:       nonce,
			Modules: []*bientotv1.ModuleData{
				{
					Module:      "system",
					TimestampNs: now + int64(i)*int64(time.Second),
					Metrics:     []*bientotv1.Metric{{Name: "cpu", Value: 42}},
				},
			},
		}
		if err := s.SavePush(ctx, req); err != nil {
			t.Fatalf("SavePush: %v", err)
		}
	}

	modules, _ := s.ListModulesForAgent(ctx, "vps")
	if len(modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(modules))
	}
	if modules[0].MetricCount != 1 {
		t.Errorf("distinct metric count = %d, want 1", modules[0].MetricCount)
	}
}

func TestListModulesForAgent_IsolatedByAgent(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()
	now := time.Now().UnixNano()

	savedPush(t, s, "vps", "v1", []*bientotv1.Metric{{Name: "cpu", Value: 1}}, now)
	savedPush(t, s, "pi", "p1", []*bientotv1.Metric{{Name: "cpu", Value: 2}}, now)

	vpsModules, _ := s.ListModulesForAgent(ctx, "vps")
	if len(vpsModules) != 1 {
		t.Errorf("vps modules = %d, want 1", len(vpsModules))
	}

	piModules, _ := s.ListModulesForAgent(ctx, "pi")
	if len(piModules) != 1 {
		t.Errorf("pi modules = %d, want 1", len(piModules))
	}
}

// ─── AgentExists ─────────────────────────────────────────

func TestAgentExists_NotFound(t *testing.T) {
	s := newTestStorage(t)

	exists, err := s.AgentExists(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("AgentExists: %v", err)
	}
	if exists {
		t.Error("expected false for nonexistent agent, got true")
	}
}

func TestAgentExists_Found(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now().UnixNano()

	savedPush(t, s, "vps", "n1", []*bientotv1.Metric{{Name: "up", Value: 1}}, now)

	exists, err := s.AgentExists(context.Background(), "vps")
	if err != nil {
		t.Fatalf("AgentExists: %v", err)
	}
	if !exists {
		t.Error("expected true for existing agent, got false")
	}
}

func TestAgentExists_CaseSensitive(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now().UnixNano()

	savedPush(t, s, "vps", "n1", []*bientotv1.Metric{{Name: "up", Value: 1}}, now)

	exists, err := s.AgentExists(context.Background(), "VPS")
	if err != nil {
		t.Fatalf("AgentExists: %v", err)
	}
	if exists {
		t.Error("expected false for case-different ID, got true")
	}
}

func TestGetMetricPoints_FilteredByAgent(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	now := time.Now().UnixNano()
	savedPush(t, s, "vps", "n1", []*bientotv1.Metric{{Name: "cpu", Value: 10}}, now)
	savedPush(t, s, "pi", "n2", []*bientotv1.Metric{{Name: "cpu", Value: 99}}, now)

	points, _ := s.GetMetricPoints(
		ctx, "vps", "cpu",
		time.Unix(0, now-int64(time.Second)),
		time.Unix(0, now+int64(time.Second)),
	)

	if len(points) != 1 {
		t.Fatalf("expected 1 point (vps only), got %d", len(points))
	}
	if points[0].Value != 10 {
		t.Errorf("value = %v, want 10 (vps)", points[0].Value)
	}
}
