package client

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
	"github.com/ldesfontaine/bientot/internal/shared/keys"
	"github.com/ldesfontaine/bientot/internal/shared/mtls"
)

const (
	testCertPath       = "../../../deploy/certs/agent-vps/client.crt"
	testKeyPath        = "../../../deploy/certs/agent-vps/client.key"
	testCAPath         = "../../../deploy/certs/agent-vps/ca-bundle.crt"
	testSigningKeyPath = "../../../deploy/certs/agent-vps/signing.key"

	testServerCertPath = "../../../deploy/certs/dashboard/server.crt"
	testServerKeyPath  = "../../../deploy/certs/dashboard/server.key"
	testServerCAPath   = "../../../deploy/certs/dashboard/ca-bundle.crt"
)

// testNew is a helper that loads the local signing key (or generates a throwaway
// one if missing) and calls New with the canonical machineID. Keeps test bodies
// focused on the behavior under test rather than wiring.
func testNew(t *testing.T, url string) (*Client, error) {
	t.Helper()
	var signKey ed25519.PrivateKey
	if _, err := os.Stat(testSigningKeyPath); err == nil {
		k, err := keys.LoadPrivateKey(testSigningKeyPath)
		if err != nil {
			t.Fatalf("load signing key: %v", err)
		}
		signKey = k
	} else {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate signing key: %v", err)
		}
		signKey = priv
	}
	return New(url, testCertPath, testKeyPath, testCAPath, "dashboard", signKey, "vps")
}

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

	c, err := testNew(t, srv.URL)
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
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	_, err = New("https://example.invalid", "/nonexistent/c.crt", "/nonexistent/k.key", "/nonexistent/ca.crt", "dashboard", priv, "vps")
	if err == nil {
		t.Fatal("expected error on missing cert, got nil")
	}
}

func TestClient_Push(t *testing.T) {
	if _, err := os.Stat(testCertPath); os.IsNotExist(err) {
		t.Skip("test certs not present, run 'make bootstrap-ca' first")
	}
	if _, err := os.Stat(testSigningKeyPath); os.IsNotExist(err) {
		t.Skip("signing key not present, run 'make bootstrap-keys' first")
	}

	serverTLS, err := mtls.ServerConfig(testServerCertPath, testServerKeyPath, testServerCAPath)
	if err != nil {
		t.Fatalf("build server config: %v", err)
	}

	var (
		gotContentType string
		gotReq         bientotv1.PushRequest
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/push", func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		if err := proto.Unmarshal(body, &gotReq); err != nil {
			t.Errorf("server unmarshal: %v", err)
		}
		resp := &bientotv1.PushResponse{
			Status:          "ok",
			AcceptedModules: int32(len(gotReq.Modules)),
			AcceptedMetrics: 1,
		}
		respBytes, _ := proto.Marshal(resp)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(respBytes)
	})

	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = serverTLS
	srv.StartTLS()
	defer srv.Close()

	c, err := testNew(t, srv.URL)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	moduleDatas := []*bientotv1.ModuleData{
		{Module: "heartbeat", TimestampNs: time.Now().UnixNano()},
	}

	resp, err := c.Push(context.Background(), moduleDatas)
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("Status = %q, want ok", resp.Status)
	}
	if resp.AcceptedModules != 1 {
		t.Errorf("AcceptedModules = %d, want 1", resp.AcceptedModules)
	}

	if gotContentType != "application/x-protobuf" {
		t.Errorf("Content-Type = %q, want application/x-protobuf", gotContentType)
	}
	if gotReq.V != 1 {
		t.Errorf("req.V = %d, want 1", gotReq.V)
	}
	if gotReq.MachineId != "vps" {
		t.Errorf("req.MachineId = %q, want vps", gotReq.MachineId)
	}
	if gotReq.Nonce == "" {
		t.Error("req.Nonce is empty")
	}
	if len(gotReq.Signature) == 0 {
		t.Error("req.Signature is empty")
	}
	if len(gotReq.Modules) != 1 || gotReq.Modules[0].Module != "heartbeat" {
		t.Errorf("req.Modules = %+v, want 1 heartbeat entry", gotReq.Modules)
	}
}

func TestClient_Push_ServerError(t *testing.T) {
	if _, err := os.Stat(testCertPath); os.IsNotExist(err) {
		t.Skip("test certs not present, run 'make bootstrap-ca' first")
	}

	serverTLS, err := mtls.ServerConfig(testServerCertPath, testServerKeyPath, testServerCAPath)
	if err != nil {
		t.Fatalf("build server config: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/push", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad signature", http.StatusForbidden)
	})

	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = serverTLS
	srv.StartTLS()
	defer srv.Close()

	c, err := testNew(t, srv.URL)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = c.Push(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error on 403 response, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want to contain '403'", err.Error())
	}
}
