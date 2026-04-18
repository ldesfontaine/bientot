package promparse

import (
	"strings"
	"testing"
)

func TestParse_SimpleMetric(t *testing.T) {
	input := `node_memory_MemTotal_bytes 8.3886848e+09`
	samples, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("got %d samples, want 1", len(samples))
	}
	s := samples[0]
	if s.Name != "node_memory_MemTotal_bytes" {
		t.Errorf("Name = %q", s.Name)
	}
	if s.Value != 8.3886848e+09 {
		t.Errorf("Value = %v", s.Value)
	}
	if len(s.Labels) != 0 {
		t.Errorf("Labels = %v, want empty", s.Labels)
	}
}

func TestParse_WithLabels(t *testing.T) {
	input := `node_cpu_seconds_total{cpu="0",mode="idle"} 123456.78`
	samples, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	s := samples[0]
	if s.Labels["cpu"] != "0" || s.Labels["mode"] != "idle" {
		t.Errorf("Labels = %v", s.Labels)
	}
}

func TestParse_SkipsComments(t *testing.T) {
	input := `# HELP foo help text
# TYPE foo gauge
foo 42
# random comment
bar 3.14`
	samples, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(samples) != 2 {
		t.Errorf("got %d samples, want 2", len(samples))
	}
}

func TestParse_EmptyLabels(t *testing.T) {
	input := `metric{} 1.0`
	samples, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(samples[0].Labels) != 0 {
		t.Errorf("expected empty labels, got %v", samples[0].Labels)
	}
}

func TestParse_MalformedLine(t *testing.T) {
	input := `this_is_not_valid`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestParse_MultipleLines(t *testing.T) {
	input := `
node_cpu_seconds_total{cpu="0",mode="idle"} 100
node_cpu_seconds_total{cpu="0",mode="user"} 50
node_cpu_seconds_total{cpu="1",mode="idle"} 200

node_memory_MemTotal_bytes 8000000000
`
	samples, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(samples) != 4 {
		t.Errorf("got %d samples, want 4", len(samples))
	}
}

func TestParse_CommaInLabelValue(t *testing.T) {
	input := `metric{path="/a,b,c"} 1.0`
	samples, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if samples[0].Labels["path"] != "/a,b,c" {
		t.Errorf("path label = %q, want %q", samples[0].Labels["path"], "/a,b,c")
	}
}

func TestParse_ScientificNotation(t *testing.T) {
	input := `metric 1.234e+10`
	samples, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if samples[0].Value != 1.234e+10 {
		t.Errorf("Value = %v, want 1.234e+10", samples[0].Value)
	}
}

func TestParse_IgnoresTimestamp(t *testing.T) {
	input := `metric 42 1625097600000`
	samples, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if samples[0].Value != 42 {
		t.Errorf("Value = %v", samples[0].Value)
	}
}

func TestParse_RealNodeExporterSnippet(t *testing.T) {
	input := `# HELP go_gc_duration_seconds A summary of GC pause durations.
# TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 1.2345e-05
go_gc_duration_seconds{quantile="0.25"} 4.5678e-05
# HELP node_memory_MemTotal_bytes Memory information field MemTotal_bytes.
# TYPE node_memory_MemTotal_bytes gauge
node_memory_MemTotal_bytes 1.677721600e+10
# HELP node_filesystem_size_bytes Filesystem size in bytes.
# TYPE node_filesystem_size_bytes gauge
node_filesystem_size_bytes{device="/dev/sda1",fstype="ext4",mountpoint="/"} 5.4e+11
`
	samples, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(samples) != 4 {
		t.Errorf("got %d samples, want 4", len(samples))
	}
	found := false
	for _, s := range samples {
		if s.Name == "node_filesystem_size_bytes" {
			found = true
			if len(s.Labels) != 3 {
				t.Errorf("filesystem labels = %v, want 3 labels", s.Labels)
			}
			if s.Labels["mountpoint"] != "/" {
				t.Errorf("mountpoint = %q, want %q", s.Labels["mountpoint"], "/")
			}
		}
	}
	if !found {
		t.Error("node_filesystem_size_bytes not found")
	}
}
