package web

import (
	"net/http"
	"strings"
	"testing"
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
