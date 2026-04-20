package web

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderer_ProdMode_ParsesAtStartup(t *testing.T) {
	_, err := newRenderer(false)
	if err != nil {
		t.Fatalf("newRenderer(false): %v", err)
	}
}

func TestRenderer_DevMode_ParsesLazily(t *testing.T) {
	_, err := newRenderer(true)
	if err != nil {
		t.Fatalf("newRenderer(true): %v", err)
	}
}

func TestRenderer_RendersOverview(t *testing.T) {
	r, err := newRenderer(false)
	if err != nil {
		t.Fatalf("newRenderer: %v", err)
	}

	var buf bytes.Buffer
	data := overviewPageData{
		Title: "Overview — vps",
		Sidebar: &sidebarData{
			CurrentMachineID: "vps",
			Machines:         []sidebarMachine{{ID: "vps", Status: "online"}},
			Version:          "test",
		},
	}
	if err := r.Render(&buf, "overview", data); err != nil {
		t.Fatalf("Render: %v", err)
	}

	body := buf.String()
	if !strings.Contains(body, "Overview — vps") {
		t.Error("body should contain Title")
	}
	if !strings.Contains(body, "vps") {
		t.Error("body should contain machine ID in sidebar")
	}
}

func TestRenderer_RendersStandalone(t *testing.T) {
	r, err := newRenderer(false)
	if err != nil {
		t.Fatalf("newRenderer: %v", err)
	}

	var buf bytes.Buffer
	data := struct{ Title string }{Title: "No agents"}
	if err := r.RenderStandalone(&buf, "no_agents", data); err != nil {
		t.Fatalf("RenderStandalone: %v", err)
	}

	body := buf.String()
	if !strings.Contains(body, "No agents yet") {
		t.Error("body should contain empty-state title")
	}
	if strings.Contains(body, "app-shell") {
		t.Error("standalone should not include the layout's app-shell")
	}
}

func TestRenderer_UnknownTemplate(t *testing.T) {
	r, err := newRenderer(false)
	if err != nil {
		t.Fatalf("newRenderer: %v", err)
	}

	var buf bytes.Buffer
	err = r.Render(&buf, "nonexistent", nil)
	if err == nil {
		t.Error("expected error for unknown template, got nil")
	}
}
