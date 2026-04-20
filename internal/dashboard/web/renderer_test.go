package web

import (
	"bytes"
	"strings"
	"testing"
	"time"
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

func TestRenderer_RendersHome(t *testing.T) {
	r, err := newRenderer(false)
	if err != nil {
		t.Fatalf("newRenderer: %v", err)
	}

	var buf bytes.Buffer
	data := homePageData{
		Title:      "Test",
		Subtitle:   "Sub",
		UptimeFake: 3 * time.Hour,
		RenderedAt: "12:00:00",
	}
	if err := r.Render(&buf, "home", data); err != nil {
		t.Fatalf("Render: %v", err)
	}

	body := buf.String()
	if !strings.Contains(body, "Test") {
		t.Error("body should contain Title")
	}
	if !strings.Contains(body, "3h 00m") {
		t.Error("body should contain formatted UptimeFake")
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
