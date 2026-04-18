package heartbeat

import (
	"context"
	"testing"
	"time"
)

func TestModule_Name(t *testing.T) {
	m := New()
	got := m.Name()
	want := "heartbeat"
	if got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestModule_Detect(t *testing.T) {
	m := New()
	if err := m.Detect(context.Background()); err != nil {
		t.Errorf("Detect() = %v, want nil", err)
	}
}

func TestModule_Collect(t *testing.T) {
	m := New()

	before := time.Now()
	data, err := m.Collect(context.Background())

	if err != nil {
		t.Fatalf("Collect() error = %v, want nil", err)
	}
	if data == nil {
		t.Fatal("Collect() returned nil data")
	}

	if data.Module != "heartbeat" {
		t.Errorf("data.Module = %q, want %q", data.Module, "heartbeat")
	}

	if len(data.Metrics) != 1 {
		t.Fatalf("len(Metrics) = %d, want 1", len(data.Metrics))
	}
	if data.Metrics[0].Name != "up" {
		t.Errorf("Metrics[0].Name = %q, want %q", data.Metrics[0].Name, "up")
	}
	if data.Metrics[0].Value != 1 {
		t.Errorf("Metrics[0].Value = %v, want 1", data.Metrics[0].Value)
	}

	if data.Metadata["hostname"] == "" {
		t.Error(`Metadata["hostname"] is empty`)
	}

	if data.Timestamp.Before(before) {
		t.Errorf("Timestamp = %v, want >= %v", data.Timestamp, before)
	}
}

func TestFactory(t *testing.T) {
	m, err := Factory(nil)
	if err != nil {
		t.Fatalf("Factory: %v", err)
	}
	if m.Name() != "heartbeat" {
		t.Errorf("Name = %q, want heartbeat", m.Name())
	}
}
