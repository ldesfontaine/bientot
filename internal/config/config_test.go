package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad_Valid(t *testing.T) {
	yaml := `
machine_id: vps
dashboard:
  url: https://dashboard:8443
  server_name: dashboard
  cert: /etc/bientot/certs/client.crt
  key: /etc/bientot/certs/client.key
  ca_bundle: /etc/bientot/certs/ca-bundle.crt
signing_key: /etc/bientot/keys/signing.key
push_interval: 45s
modules:
  - type: heartbeat
    enabled: true
  - type: system
    enabled: true
    config:
      node_exporter_url: http://node-exporter:9100
`

	path := writeTempFile(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.MachineID != "vps" {
		t.Errorf("MachineID = %q, want vps", cfg.MachineID)
	}
	if cfg.PushInterval != 45*time.Second {
		t.Errorf("PushInterval = %v, want 45s", cfg.PushInterval)
	}
	if len(cfg.Modules) != 2 {
		t.Fatalf("Modules count = %d, want 2", len(cfg.Modules))
	}
	if cfg.Modules[1].Type != "system" {
		t.Errorf("Modules[1].Type = %q, want system", cfg.Modules[1].Type)
	}
	if cfg.Modules[1].Config["node_exporter_url"] != "http://node-exporter:9100" {
		t.Errorf("system node_exporter_url not parsed correctly: %v", cfg.Modules[1].Config)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	path := writeTempFile(t, "not: valid: yaml: :")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestLoad_MissingMachineID(t *testing.T) {
	yaml := `
dashboard:
  url: https://dashboard:8443
  cert: /a
  key: /b
  ca_bundle: /c
signing_key: /d
`
	path := writeTempFile(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "machine_id") {
		t.Errorf("error should mention machine_id, got: %v", err)
	}
}

func TestLoad_DefaultPushInterval(t *testing.T) {
	yaml := `
machine_id: vps
dashboard:
  url: https://dashboard:8443
  cert: /a
  key: /b
  ca_bundle: /c
signing_key: /d
modules: []
`
	path := writeTempFile(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.PushInterval != 30*time.Second {
		t.Errorf("default PushInterval = %v, want 30s", cfg.PushInterval)
	}
	if cfg.Dashboard.ServerName != "dashboard" {
		t.Errorf("default ServerName = %q, want dashboard", cfg.Dashboard.ServerName)
	}
}

func TestLoad_PushIntervalTooSmall(t *testing.T) {
	yaml := `
machine_id: vps
dashboard:
  url: https://dashboard:8443
  cert: /a
  key: /b
  ca_bundle: /c
signing_key: /d
push_interval: 30
`
	path := writeTempFile(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for unitless push_interval, got nil")
	}
}

func TestLoad_ModuleMissingType(t *testing.T) {
	yaml := `
machine_id: vps
dashboard:
  url: https://dashboard:8443
  cert: /a
  key: /b
  ca_bundle: /c
signing_key: /d
modules:
  - enabled: true
`
	path := writeTempFile(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for module missing type, got nil")
	}
	if !strings.Contains(err.Error(), "type is required") {
		t.Errorf("error should mention type, got: %v", err)
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}
