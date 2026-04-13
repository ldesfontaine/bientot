package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetect_NoDir(t *testing.T) {
	m := New("")
	if m.Detect() {
		t.Fatal("should not detect with empty dir")
	}
}

func TestDetect_ValidDir(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if !m.Detect() {
		t.Fatal("should detect valid directory")
	}
}

func TestCollect_ParsesJSON(t *testing.T) {
	dir := t.TempDir()
	data := `{"type":"vps","timestamp":"2025-01-15T10:00:00Z","success":true,"size_bytes":1024}`
	if err := os.WriteFile(filepath.Join(dir, "vps-latest.json"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	m := New(dir)
	result, err := m.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if result.Module != "backup" {
		t.Fatalf("expected module 'backup', got %q", result.Module)
	}

	// Should have success, size_bytes, age_hours metrics
	names := make(map[string]bool)
	for _, metric := range result.Metrics {
		names[metric.Name] = true
	}

	for _, expected := range []string{"backup_success", "backup_size_bytes", "backup_age_hours"} {
		if !names[expected] {
			t.Errorf("missing metric %q", expected)
		}
	}

	// Check success = 1
	for _, metric := range result.Metrics {
		if metric.Name == "backup_success" && metric.Value != 1.0 {
			t.Errorf("backup_success should be 1.0, got %f", metric.Value)
		}
	}
}

func TestCollect_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	result, err := m.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Metrics) != 0 {
		t.Fatalf("expected 0 metrics for empty dir, got %d", len(result.Metrics))
	}
}
