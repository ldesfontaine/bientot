package system

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ldesfontaine/bientot/internal/modules"
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

const fakeNodeExporterOutput = `# HELP node_memory_MemTotal_bytes Memory information field MemTotal_bytes.
# TYPE node_memory_MemTotal_bytes gauge
node_memory_MemTotal_bytes 1.677721600e+10
# HELP node_memory_MemAvailable_bytes Memory information field MemAvailable_bytes.
# TYPE node_memory_MemAvailable_bytes gauge
node_memory_MemAvailable_bytes 8.388608e+09
# HELP node_memory_MemFree_bytes Memory information field MemFree_bytes.
# TYPE node_memory_MemFree_bytes gauge
node_memory_MemFree_bytes 4.194304e+09
# HELP node_memory_SwapTotal_bytes Memory information field SwapTotal_bytes.
# TYPE node_memory_SwapTotal_bytes gauge
node_memory_SwapTotal_bytes 2.147483648e+09
# HELP node_memory_SwapFree_bytes Memory information field SwapFree_bytes.
# TYPE node_memory_SwapFree_bytes gauge
node_memory_SwapFree_bytes 1.073741824e+09
# HELP node_load1 1m load average.
# TYPE node_load1 gauge
node_load1 0.42
# HELP node_cpu_seconds_total Seconds the CPUs spent in each mode.
# TYPE node_cpu_seconds_total counter
node_cpu_seconds_total{cpu="0",mode="idle"} 123456.78
node_cpu_seconds_total{cpu="0",mode="user"} 3456.12
node_cpu_seconds_total{cpu="0",mode="system"} 890.34
node_cpu_seconds_total{cpu="0",mode="iowait"} 45.67
node_cpu_seconds_total{cpu="1",mode="idle"} 120000.00
node_cpu_seconds_total{cpu="1",mode="user"} 3100.50
node_cpu_seconds_total{cpu="1",mode="system"} 780.20
node_cpu_seconds_total{cpu="1",mode="iowait"} 40.00
# HELP node_filesystem_size_bytes Filesystem size in bytes.
# TYPE node_filesystem_size_bytes gauge
node_filesystem_size_bytes{device="/dev/sda1",fstype="ext4",mountpoint="/"} 5.4e+11
node_filesystem_size_bytes{device="/dev/sda2",fstype="ext4",mountpoint="/boot"} 1.0e+09
# HELP node_filesystem_avail_bytes Filesystem space available to non-root users in bytes.
# TYPE node_filesystem_avail_bytes gauge
node_filesystem_avail_bytes{device="/dev/sda1",fstype="ext4",mountpoint="/"} 3.0e+11
node_filesystem_avail_bytes{device="/dev/sda2",fstype="ext4",mountpoint="/boot"} 5.0e+08
# HELP node_boot_time_seconds Node boot time, in unixtime.
# TYPE node_boot_time_seconds gauge
node_boot_time_seconds 1.70e+09
`

func TestModule_Collect_Real(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(fakeNodeExporterOutput))
	}))
	defer srv.Close()

	m := New(srv.URL)
	data, err := m.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if data.Module != "system" {
		t.Errorf("Module = %q, want system", data.Module)
	}

	if len(data.Metrics) != 14 {
		t.Errorf("got %d metrics, want 14. Got: %+v", len(data.Metrics), metricNames(data.Metrics))
	}

	checks := map[string]float64{
		"memory_total_bytes": 1.677721600e+10,
		"load_average_1m":    0.42,
		"cpu_cores":          2,
	}

	for name, want := range checks {
		found := false
		for _, metric := range data.Metrics {
			if metric.Name == name {
				found = true
				if metric.Value != want {
					t.Errorf("metric %q = %v, want %v", name, metric.Value, want)
				}
				break
			}
		}
		if !found {
			t.Errorf("metric %q missing", name)
		}
	}

	if data.Metadata["hostname"] == "" {
		t.Error("hostname metadata empty")
	}
	if data.Metadata["scrape_target"] != srv.URL {
		t.Errorf("scrape_target = %q, want %q", data.Metadata["scrape_target"], srv.URL)
	}
}

func metricNames(metrics []modules.Metric) []string {
	names := make([]string, len(metrics))
	for i, m := range metrics {
		names[i] = m.Name
	}
	return names
}

func TestFactory_Valid(t *testing.T) {
	cfg := map[string]interface{}{
		"node_exporter_url": "http://test:9100",
	}
	m, err := Factory(cfg)
	if err != nil {
		t.Fatalf("Factory: %v", err)
	}
	if m.Name() != "system" {
		t.Errorf("Name = %q, want system", m.Name())
	}
}

func TestFactory_MissingURL(t *testing.T) {
	if _, err := Factory(nil); err == nil {
		t.Fatal("expected error for missing url, got nil")
	}
	if _, err := Factory(map[string]interface{}{"node_exporter_url": ""}); err == nil {
		t.Fatal("expected error for empty url, got nil")
	}
}
