package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
)

func TestOverview_AgentNotFound(t *testing.T) {
	r, _ := newTestRouterWithDB(t)

	rec := doRequest(t, r, http.MethodGet, "/agents/nonexistent")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestOverview_SingleAgent_NoDropdown(t *testing.T) {
	r, db := newTestRouterWithDB(t)
	savePushSimple(t, db, "vps", "n1")

	rec := doRequest(t, r, http.MethodGet, "/agents/vps")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	body := rec.Body.String()

	if strings.Contains(body, "<details") {
		t.Error("with 1 machine, sidebar should not render <details> dropdown")
	}
	if !strings.Contains(body, "vps") {
		t.Error("sidebar should show machine id 'vps'")
	}
	if !strings.Contains(body, "nav-item active") {
		t.Error("overview should have an active nav item")
	}
}

func TestOverview_MultipleAgents_ShowsDropdown(t *testing.T) {
	r, db := newTestRouterWithDB(t)
	savePushSimple(t, db, "vps", "n1")
	savePushSimple(t, db, "pi", "n2")

	rec := doRequest(t, r, http.MethodGet, "/agents/vps")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	body := rec.Body.String()

	if !strings.Contains(body, "<details") {
		t.Error("with 2+ machines, sidebar should render <details> dropdown")
	}
	if !strings.Contains(body, "vps") {
		t.Error("sidebar should mention 'vps'")
	}
	if !strings.Contains(body, "pi") {
		t.Error("sidebar should mention 'pi'")
	}
	if !strings.Contains(body, `href="/agents/pi"`) {
		t.Error("dropdown should have link to /agents/pi")
	}
}

func TestOverview_DisabledNavItems(t *testing.T) {
	r, db := newTestRouterWithDB(t)
	savePushSimple(t, db, "vps", "n1")

	rec := doRequest(t, r, http.MethodGet, "/agents/vps")

	body := rec.Body.String()

	if !strings.Contains(body, `aria-disabled="true"`) {
		t.Error("modules nav items should have aria-disabled")
	}
	if !strings.Contains(body, "System") || !strings.Contains(body, "Docker") || !strings.Contains(body, "Certs") {
		t.Error("three module items expected in the sidebar")
	}
}

func TestOverview_TitleIncludesMachineID(t *testing.T) {
	r, db := newTestRouterWithDB(t)
	savePushSimple(t, db, "vps", "n1")

	rec := doRequest(t, r, http.MethodGet, "/agents/vps")

	body := rec.Body.String()

	if !strings.Contains(body, "<title>Overview — vps — Bientôt</title>") {
		t.Error("title with machine id not found")
	}
}

func TestOverview_KPIsRendered(t *testing.T) {
	r, db := newTestRouterWithDB(t)
	saveMetricsPush(t, db, "vps", "n1", map[string]float64{
		"uptime_seconds":         90000,
		"load_average_1m":        0.42,
		"memory_total_bytes":     4_000_000_000,
		"memory_available_bytes": 2_200_000_000,
		"filesystem_size_bytes":  30_000_000_000,
		"filesystem_avail_bytes": 11_400_000_000,
	})

	rec := doRequest(t, r, http.MethodGet, "/agents/vps")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	body := rec.Body.String()

	for _, label := range []string{"Uptime", "Load 1m", "Memory", "Disk /", "Containers", "Last push"} {
		if !strings.Contains(body, label) {
			t.Errorf("overview should contain KPI label %q", label)
		}
	}

	if !strings.Contains(body, "0.42") {
		t.Error("overview should display load value 0.42")
	}
	if !strings.Contains(body, "45%") {
		t.Error("overview should display memory 45%")
	}
}

func TestOverview_MissingMetricsShowDashes(t *testing.T) {
	r, db := newTestRouterWithDB(t)
	savePushSimple(t, db, "vps", "n1") // heartbeat only, no system metrics

	rec := doRequest(t, r, http.MethodGet, "/agents/vps")
	body := rec.Body.String()

	if !strings.Contains(body, "kpi-missing") {
		t.Error("missing KPIs should render with .kpi-missing class")
	}
}

// saveMetricsPush saves a push with multiple system metrics in one shot.
func saveMetricsPush(t *testing.T, db *storage.Storage, machineID, nonce string, metrics map[string]float64) {
	t.Helper()
	ts := time.Now().UnixNano()
	metricList := make([]*bientotv1.Metric, 0, len(metrics))
	for name, value := range metrics {
		metricList = append(metricList, &bientotv1.Metric{Name: name, Value: value})
	}
	req := &bientotv1.PushRequest{
		V:           1,
		MachineId:   machineID,
		TimestampNs: ts,
		Nonce:       nonce,
		Modules: []*bientotv1.ModuleData{
			{Module: "system", TimestampNs: ts, Metrics: metricList},
		},
	}
	if err := db.SavePush(context.Background(), req); err != nil {
		t.Fatalf("SavePush: %v", err)
	}
}
