package mtls

import (
	"crypto/tls"
	"os"
	"strings"
	"testing"
)

const (
	testCertPath = "../../../deploy/certs/agent-vps/client.crt"
	testKeyPath  = "../../../deploy/certs/agent-vps/client.key"
	testCAPath   = "../../../deploy/certs/agent-vps/ca-bundle.crt"
)

func TestClientConfig_Success(t *testing.T) {
	if _, err := os.Stat(testCertPath); os.IsNotExist(err) {
		t.Skip("test certs not present, run 'make bootstrap-ca' first")
	}

	cfg, err := ClientConfig(testCertPath, testKeyPath, testCAPath, "dashboard")
	if err != nil {
		t.Fatalf("ClientConfig() error = %v", err)
	}

	if cfg == nil {
		t.Fatal("ClientConfig() returned nil config")
	}

	if cfg.ServerName != "dashboard" {
		t.Errorf("ServerName = %q, want %q", cfg.ServerName, "dashboard")
	}

	if cfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %d, want TLS 1.3", cfg.MinVersion)
	}

	if len(cfg.Certificates) != 1 {
		t.Errorf("len(Certificates) = %d, want 1", len(cfg.Certificates))
	}

	if cfg.RootCAs == nil {
		t.Error("RootCAs is nil")
	}

	if cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify MUST be false")
	}
}

func TestClientConfig_MissingCert(t *testing.T) {
	_, err := ClientConfig("/nonexistent/cert.crt", "/nonexistent/key.key", "/nonexistent/ca.crt", "dashboard")
	if err == nil {
		t.Fatal("ClientConfig() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "load client keypair") {
		t.Errorf("error message = %q, want to contain 'load client keypair'", err.Error())
	}
}

func TestClientConfig_InvalidCA(t *testing.T) {
	if _, err := os.Stat(testCertPath); os.IsNotExist(err) {
		t.Skip("test certs not present, run 'make bootstrap-ca' first")
	}

	emptyCA, err := os.CreateTemp("", "empty-ca-*.crt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(emptyCA.Name())
	emptyCA.Close()

	_, err = ClientConfig(testCertPath, testKeyPath, emptyCA.Name(), "dashboard")
	if err == nil {
		t.Fatal("expected error on empty CA, got nil")
	}
	if !strings.Contains(err.Error(), "no valid PEM") {
		t.Errorf("error = %q, want 'no valid PEM'", err.Error())
	}
}
