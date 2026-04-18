package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ldesfontaine/bientot/internal/shared/mtls"
)

const (
	testCertPath = "../../../deploy/certs/agent-vps/client.crt"
	testKeyPath  = "../../../deploy/certs/agent-vps/client.key"
	testCAPath   = "../../../deploy/certs/agent-vps/ca-bundle.crt"

	testServerCertPath = "../../../deploy/certs/dashboard/server.crt"
	testServerKeyPath  = "../../../deploy/certs/dashboard/server.key"
	testServerCAPath   = "../../../deploy/certs/dashboard/ca-bundle.crt"
)

func TestClient_Ping(t *testing.T) {
	if _, err := os.Stat(testCertPath); os.IsNotExist(err) {
		t.Skip("test certs not present, run 'make bootstrap-ca' first")
	}

	serverTLS, err := mtls.ServerConfig(testServerCertPath, testServerKeyPath, testServerCAPath)
	if err != nil {
		t.Fatalf("build server config: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		cn := ""
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			cn = r.TLS.PeerCertificates[0].Subject.CommonName
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"from":      "test-echo",
			"client_cn": cn,
		})
	})

	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = serverTLS
	srv.StartTLS()
	defer srv.Close()

	c, err := New(srv.URL, testCertPath, testKeyPath, testCAPath, "dashboard")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	resp, err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	if resp.From != "test-echo" {
		t.Errorf("From = %q, want %q", resp.From, "test-echo")
	}
	if resp.ClientCN != "vps" {
		t.Errorf("ClientCN = %q, want %q", resp.ClientCN, "vps")
	}
}

func TestClient_New_InvalidCert(t *testing.T) {
	_, err := New("https://example.invalid", "/nonexistent/c.crt", "/nonexistent/k.key", "/nonexistent/ca.crt", "dashboard")
	if err == nil {
		t.Fatal("expected error on missing cert, got nil")
	}
}
