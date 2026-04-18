package client

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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

func generateSelfSignedClientCert(t *testing.T) tls.Certificate {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "attacker",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(1 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, priv.Public(), priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	return tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}
}

func TestClient_Ping_NoClientCert(t *testing.T) {
	if _, err := os.Stat(testServerCertPath); os.IsNotExist(err) {
		t.Skip("test certs not present, run 'make bootstrap-ca' first")
	}

	serverTLS, err := mtls.ServerConfig(testServerCertPath, testServerKeyPath, testServerCAPath)
	if err != nil {
		t.Fatalf("build server config: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = serverTLS
	srv.StartTLS()
	defer srv.Close()

	caBytes, err := os.ReadFile(testServerCAPath)
	if err != nil {
		t.Fatalf("read ca: %v", err)
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caBytes)

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    caPool,
				ServerName: "dashboard",
				MinVersion: tls.VersionTLS13,
			},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := httpClient.Get(srv.URL + "/ping")
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected TLS handshake to fail, got successful response")
	}

	if !strings.Contains(err.Error(), "certificate") {
		t.Errorf("error = %q, want to contain 'certificate'", err.Error())
	}

	t.Logf("correctly rejected connection without client cert: %v", err)
}

func TestClient_Ping_WrongCA(t *testing.T) {
	if _, err := os.Stat(testServerCertPath); os.IsNotExist(err) {
		t.Skip("test certs not present, run 'make bootstrap-ca' first")
	}

	serverTLS, err := mtls.ServerConfig(testServerCertPath, testServerKeyPath, testServerCAPath)
	if err != nil {
		t.Fatalf("build server config: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = serverTLS
	srv.StartTLS()
	defer srv.Close()

	attackerCert := generateSelfSignedClientCert(t)

	caBytes, _ := os.ReadFile(testServerCAPath)
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caBytes)

	// GetClientCertificate forces the cert to be sent regardless of the server's
	// acceptable CAs — without it, Go silently drops mismatched certs and the
	// server replies "certificate required" instead of "unknown authority".
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
					return &attackerCert, nil
				},
				RootCAs:    caPool,
				ServerName: "dashboard",
				MinVersion: tls.VersionTLS13,
			},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := httpClient.Get(srv.URL + "/ping")
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected TLS handshake to fail, got successful response")
	}

	msg := err.Error()
	if !strings.Contains(msg, "authority") && !strings.Contains(msg, "unknown") {
		t.Errorf("error = %q, want to indicate unknown CA", msg)
	}

	t.Logf("correctly rejected connection with untrusted cert: %v", err)
}

func TestClient_New_InvalidCert(t *testing.T) {
	_, err := New("https://example.invalid", "/nonexistent/c.crt", "/nonexistent/k.key", "/nonexistent/ca.crt", "dashboard")
	if err == nil {
		t.Fatal("expected error on missing cert, got nil")
	}
}
