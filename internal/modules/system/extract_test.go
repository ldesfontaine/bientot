package system

import (
	"testing"
	"time"

	"github.com/ldesfontaine/bientot/internal/modules"
	"github.com/ldesfontaine/bientot/internal/shared/promparse"
)

var testNow = time.Unix(2_000_000_000, 0)

func findMetric(metrics []modules.Metric, name string) *modules.Metric {
	for i := range metrics {
		if metrics[i].Name == name {
			return &metrics[i]
		}
	}
	return nil
}

func TestExtract_AllMetrics(t *testing.T) {
	samples := []promparse.Sample{
		{Name: "node_memory_MemTotal_bytes", Value: 8_000_000_000},
		{Name: "node_memory_MemAvailable_bytes", Value: 4_000_000_000},
		{Name: "node_memory_MemFree_bytes", Value: 2_000_000_000},
		{Name: "node_memory_SwapTotal_bytes", Value: 1_000_000_000},
		{Name: "node_memory_SwapFree_bytes", Value: 800_000_000},
		{Name: "node_load1", Value: 0.75},
		{Name: "node_load5", Value: 1.25},
		{Name: "node_load15", Value: 1.80},
		{Name: "node_cpu_seconds_total", Labels: map[string]string{"cpu": "0", "mode": "user"}, Value: 100},
		{Name: "node_cpu_seconds_total", Labels: map[string]string{"cpu": "0", "mode": "system"}, Value: 50},
		{Name: "node_cpu_seconds_total", Labels: map[string]string{"cpu": "0", "mode": "idle"}, Value: 9000},
		{Name: "node_cpu_seconds_total", Labels: map[string]string{"cpu": "0", "mode": "iowait"}, Value: 10},
		{Name: "node_cpu_seconds_total", Labels: map[string]string{"cpu": "1", "mode": "user"}, Value: 200},
		{Name: "node_cpu_seconds_total", Labels: map[string]string{"cpu": "1", "mode": "system"}, Value: 75},
		{Name: "node_cpu_seconds_total", Labels: map[string]string{"cpu": "1", "mode": "idle"}, Value: 8500},
		{Name: "node_cpu_seconds_total", Labels: map[string]string{"cpu": "1", "mode": "iowait"}, Value: 15},
		{Name: "node_filesystem_size_bytes", Labels: map[string]string{"mountpoint": "/"}, Value: 500_000_000_000},
		{Name: "node_filesystem_size_bytes", Labels: map[string]string{"mountpoint": "/boot"}, Value: 1_000_000_000},
		{Name: "node_filesystem_avail_bytes", Labels: map[string]string{"mountpoint": "/"}, Value: 200_000_000_000},
		{Name: "node_filesystem_avail_bytes", Labels: map[string]string{"mountpoint": "/boot"}, Value: 500_000_000},
		{Name: "node_boot_time_seconds", Value: float64(testNow.Unix() - 3600)},
	}

	got := Extract(samples, testNow)

	if len(got) != 16 {
		t.Errorf("got %d metrics, want 16. Got: %+v", len(got), got)
	}

	checks := map[string]float64{
		"memory_total_bytes":       8_000_000_000,
		"load_average_1m":          0.75,
		"load_average_5m":          1.25,
		"load_average_15m":         1.80,
		"cpu_user_seconds_total":   300,
		"cpu_system_seconds_total": 125,
		"cpu_idle_seconds_total":   17500,
		"cpu_iowait_seconds_total": 25,
		"cpu_cores":                2,
		"filesystem_size_bytes":    500_000_000_000,
		"filesystem_avail_bytes":   200_000_000_000,
		"uptime_seconds":           3600,
	}

	for name, want := range checks {
		m := findMetric(got, name)
		if m == nil {
			t.Errorf("metric %q missing", name)
			continue
		}
		if m.Value != want {
			t.Errorf("metric %q = %v, want %v", name, m.Value, want)
		}
	}
}

func TestExtract_Partial(t *testing.T) {
	samples := []promparse.Sample{
		{Name: "node_memory_MemTotal_bytes", Value: 8_000_000_000},
		{Name: "node_load1", Value: 0.5},
	}

	got := Extract(samples, testNow)

	if len(got) != 2 {
		t.Errorf("got %d metrics, want 2", len(got))
	}
}

func TestExtract_Empty(t *testing.T) {
	got := Extract(nil, testNow)
	if len(got) != 0 {
		t.Errorf("got %d metrics, want 0", len(got))
	}
}

func TestExtract_IgnoresNonRootFilesystem(t *testing.T) {
	samples := []promparse.Sample{
		{Name: "node_filesystem_size_bytes", Labels: map[string]string{"mountpoint": "/boot"}, Value: 500_000_000},
	}

	got := Extract(samples, testNow)

	if findMetric(got, "filesystem_size_bytes") != nil {
		t.Error("filesystem_size_bytes should be absent when no root mountpoint")
	}
}
