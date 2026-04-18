package system

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestModule_Name(t *testing.T) {
	m := New("http://test")
	if m.Name() != "system" {
		t.Errorf("Name() = %q, want %q", m.Name(), "system")
	}
}

func TestModule_Interval(t *testing.T) {
	m := New("http://test")
	if m.Interval() != 30*time.Second {
		t.Errorf("Interval() = %v, want 30s", m.Interval())
	}
}

func TestModule_Detect_EmptyURL(t *testing.T) {
	m := New("")
	if err := m.Detect(context.Background()); err == nil {
		t.Fatal("expected error with empty URL, got nil")
	}
}

func TestModule_Detect_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	m := New(srv.URL)
	if err := m.Detect(context.Background()); err != nil {
		t.Errorf("Detect() error = %v, want nil", err)
	}
}

func TestModule_Detect_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := New(srv.URL)
	if err := m.Detect(context.Background()); err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}

func TestModule_Detect_ServerDown(t *testing.T) {
	m := New("http://127.0.0.1:65500")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := m.Detect(ctx); err == nil {
		t.Fatal("expected error with down server, got nil")
	}
}

func TestModule_Collect_Minimal(t *testing.T) {
	m := New("http://test")
	data, err := m.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if data.Module != "system" {
		t.Errorf("Module = %q, want system", data.Module)
	}
	if len(data.Metrics) != 1 || data.Metrics[0].Name != "system_up" || data.Metrics[0].Value != 1 {
		t.Errorf("expected single metric system_up=1, got %v", data.Metrics)
	}
	if data.Metadata["hostname"] == "" {
		t.Error("hostname metadata is empty")
	}
	if data.Metadata["scrape_target"] != "http://test" {
		t.Errorf("scrape_target = %q, want http://test", data.Metadata["scrape_target"])
	}
}
