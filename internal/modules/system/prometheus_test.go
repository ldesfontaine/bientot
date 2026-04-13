package system

import (
	"strings"
	"testing"
)

func TestParsePrometheusMetrics(t *testing.T) {
	input := `# HELP node_cpu_seconds_total CPU time
# TYPE node_cpu_seconds_total counter
node_cpu_seconds_total{cpu="0",mode="idle"} 123456.78
node_cpu_seconds_total{cpu="0",mode="user"} 5432.1
node_memory_MemTotal_bytes 8.589934592e+09
node_memory_MemAvailable_bytes 4294967296
node_load1 1.5
node_load5 1.2
node_load15 0.8
node_uname_info{machine="x86_64",nodename="server1",release="6.1.0",sysname="Linux"} 1
node_filesystem_size_bytes{device="/dev/sda1",fstype="ext4",mountpoint="/"} 5e+10
node_filesystem_avail_bytes{device="/dev/sda1",fstype="ext4",mountpoint="/"} 2e+10
node_hwmon_temp_celsius{chip="coretemp",sensor="temp1"} 45.0
node_zfs_zpool_state{zpool="tank",state="online"} 1
node_zfs_zpool_allocated_bytes{zpool="tank"} 1e+12
node_zfs_zpool_size_bytes{zpool="tank"} 2e+12
`

	metrics := parsePrometheusMetrics(strings.NewReader(input))

	// Build lookup by name+labels for easier assertions
	type key struct {
		name string
		cpu  string
		mode string
	}
	byName := make(map[string][]promMetric)
	for _, m := range metrics {
		byName[m.Name] = append(byName[m.Name], m)
	}

	// CPU metrics parsed
	cpuMetrics := byName["node_cpu_seconds_total"]
	if len(cpuMetrics) != 2 {
		t.Fatalf("expected 2 cpu metrics, got %d", len(cpuMetrics))
	}

	// Memory
	if ms := byName["node_memory_MemTotal_bytes"]; len(ms) != 1 || ms[0].Value != 8.589934592e+09 {
		t.Errorf("MemTotal: got %v", ms)
	}
	if ms := byName["node_memory_MemAvailable_bytes"]; len(ms) != 1 || ms[0].Value != 4294967296 {
		t.Errorf("MemAvailable: got %v", ms)
	}

	// Load
	if ms := byName["node_load1"]; len(ms) != 1 || ms[0].Value != 1.5 {
		t.Errorf("load1: got %v", ms)
	}

	// Uname labels
	uname := byName["node_uname_info"]
	if len(uname) != 1 {
		t.Fatalf("expected 1 uname metric, got %d", len(uname))
	}
	if uname[0].Labels["machine"] != "x86_64" {
		t.Errorf("uname machine: got %q", uname[0].Labels["machine"])
	}
	if uname[0].Labels["sysname"] != "Linux" {
		t.Errorf("uname sysname: got %q", uname[0].Labels["sysname"])
	}

	// Filesystem
	fs := byName["node_filesystem_size_bytes"]
	if len(fs) != 1 || fs[0].Labels["mountpoint"] != "/" {
		t.Errorf("filesystem: got %v", fs)
	}

	// Temperature
	temp := byName["node_hwmon_temp_celsius"]
	if len(temp) != 1 || temp[0].Value != 45.0 {
		t.Errorf("temperature: got %v", temp)
	}

	// ZFS
	zpoolState := byName["node_zfs_zpool_state"]
	if len(zpoolState) != 1 || zpoolState[0].Labels["zpool"] != "tank" {
		t.Errorf("zpool state: got %v", zpoolState)
	}
}

func TestParseLabels(t *testing.T) {
	tests := []struct {
		input string
		want  map[string]string
	}{
		{`cpu="0",mode="idle"`, map[string]string{"cpu": "0", "mode": "idle"}},
		{`device="/dev/sda1"`, map[string]string{"device": "/dev/sda1"}},
		{``, map[string]string{}},
	}

	for _, tt := range tests {
		got := parseLabels(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseLabels(%q): got %v, want %v", tt.input, got, tt.want)
			continue
		}
		for k, v := range tt.want {
			if got[k] != v {
				t.Errorf("parseLabels(%q)[%q] = %q, want %q", tt.input, k, got[k], v)
			}
		}
	}
}

func TestExtractCPU(t *testing.T) {
	all := []promMetric{
		{Name: "node_cpu_seconds_total", Labels: map[string]string{"cpu": "0", "mode": "user"}, Value: 100},
		{Name: "node_cpu_seconds_total", Labels: map[string]string{"cpu": "0", "mode": "system"}, Value: 50},
		{Name: "node_cpu_seconds_total", Labels: map[string]string{"cpu": "0", "mode": "idle"}, Value: 800},
		{Name: "node_cpu_seconds_total", Labels: map[string]string{"cpu": "0", "mode": "iowait"}, Value: 50},
	}

	metrics := extractCPU(all)
	if len(metrics) != 5 {
		t.Fatalf("expected 5 cpu metrics, got %d", len(metrics))
	}

	// total = 100+50+800+50 = 1000
	// usage = (1000-800-50)/1000 * 100 = 15%
	if metrics[0].Name != "system_cpu_usage_percent" || metrics[0].Value != 15.0 {
		t.Errorf("cpu usage: got %v", metrics[0])
	}
}

func TestExtractZFS(t *testing.T) {
	all := []promMetric{
		{Name: "node_zfs_zpool_state", Labels: map[string]string{"zpool": "tank", "state": "online"}, Value: 1},
		{Name: "node_zfs_zpool_state", Labels: map[string]string{"zpool": "tank", "state": "degraded"}, Value: 0},
		{Name: "node_zfs_zpool_allocated_bytes", Labels: map[string]string{"zpool": "tank"}, Value: 500e9},
		{Name: "node_zfs_zpool_size_bytes", Labels: map[string]string{"zpool": "tank"}, Value: 1000e9},
	}

	metrics := extractZFS(all)

	// Should have: health, used_bytes, size_bytes, available_bytes, used_percent
	byName := make(map[string]float64)
	for _, m := range metrics {
		byName[m.Name] = m.Value
	}

	if byName["zfs_pool_health"] != 2 {
		t.Errorf("zfs health: got %v, want 2 (online)", byName["zfs_pool_health"])
	}
	if byName["zfs_pool_used_bytes"] != 500e9 {
		t.Errorf("zfs used: got %v", byName["zfs_pool_used_bytes"])
	}
	if byName["zfs_pool_available_bytes"] != 500e9 {
		t.Errorf("zfs available: got %v", byName["zfs_pool_available_bytes"])
	}
	if byName["zfs_pool_used_percent"] != 50 {
		t.Errorf("zfs used_percent: got %v", byName["zfs_pool_used_percent"])
	}
}
