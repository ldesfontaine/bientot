package dashboardsrv

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
	"github.com/ldesfontaine/bientot/internal/shared/crypto"
	"github.com/ldesfontaine/bientot/internal/shared/keys"
)

const (
	testServerCertPath = "../../deploy/certs/dashboard/server.crt"
	testServerKeyPath  = "../../deploy/certs/dashboard/server.key"
	testServerCAPath   = "../../deploy/certs/dashboard/ca-bundle.crt"
	testAgentKeysDir   = "../../deploy/certs/dashboard/agent-keys"

	testClientCertPath = "../../deploy/certs/agent-vps/client.crt"
	testClientKeyPath  = "../../deploy/certs/agent-vps/client.key"
	testClientCAPath   = "../../deploy/certs/agent-vps/ca-bundle.crt"
	testSigningKeyPath = "../../deploy/certs/agent-vps/signing.key"

	testMachineID = "vps"
)

// testSetup starts a dashboardsrv.Server on a random port and returns its URL,
// the agent's signing key, and an mTLS-configured HTTP client.
// Skips the test if the cert/key fixtures are not present.
func testSetup(t *testing.T) (baseURL string, signKey ed25519.PrivateKey, httpClient *http.Client, cleanup func()) {
	t.Helper()

	for _, p := range []string{
		testServerCertPath, testServerKeyPath, testServerCAPath,
		testClientCertPath, testClientKeyPath, testSigningKeyPath,
	} {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Skipf("test fixture missing: %s (run 'make bootstrap-ca' first)", p)
		}
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(logger, addr, testServerCertPath, testServerKeyPath, testServerCAPath, testAgentKeysDir)

	serverCtx, serverCancel := context.WithCancel(context.Background())
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		if err := srv.Run(serverCtx); err != nil && err != http.ErrServerClosed {
			t.Logf("server.Run error: %v", err)
		}
	}()

	// Wait for the server to bind the port (else the first request races the listener).
	ready := false
	for i := 0; i < 20; i++ {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		serverCancel()
		t.Fatalf("server did not start listening on %s", addr)
	}

	clientCert, err := tls.LoadX509KeyPair(testClientCertPath, testClientKeyPath)
	if err != nil {
		serverCancel()
		t.Fatalf("load client keypair: %v", err)
	}
	caBytes, err := os.ReadFile(testClientCAPath)
	if err != nil {
		serverCancel()
		t.Fatalf("read ca: %v", err)
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caBytes)

	httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{clientCert},
				RootCAs:      caPool,
				ServerName:   "dashboard",
				MinVersion:   tls.VersionTLS13,
			},
		},
		Timeout: 5 * time.Second,
	}

	signKey, err = keys.LoadPrivateKey(testSigningKeyPath)
	if err != nil {
		serverCancel()
		t.Fatalf("load signing key: %v", err)
	}

	baseURL = "https://" + addr
	cleanup = func() {
		serverCancel()
		<-serverDone
	}

	return
}

// buildPush builds a valid PushRequest signed with signKey; tweak may mutate
// the request before signing to exercise specific rejection paths.
func buildPush(t *testing.T, signKey ed25519.PrivateKey, tweak func(*bientotv1.PushRequest)) []byte {
	t.Helper()

	req := &bientotv1.PushRequest{
		V:           1,
		MachineId:   testMachineID,
		TimestampNs: time.Now().UnixNano(),
		Nonce:       uuid.NewString(),
		Modules: []*bientotv1.ModuleData{
			{Module: "heartbeat", TimestampNs: time.Now().UnixNano()},
		},
	}

	if tweak != nil {
		tweak(req)
	}

	signed, err := crypto.Sign(req, signKey)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	body, err := proto.Marshal(signed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	return body
}

func postPush(t *testing.T, httpClient *http.Client, baseURL string, body []byte) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/push", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-protobuf")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("http do: %v", err)
	}

	return resp
}

func TestPush_Valid(t *testing.T) {
	baseURL, signKey, httpClient, cleanup := testSetup(t)
	defer cleanup()

	body := buildPush(t, signKey, nil)

	resp := postPush(t, httpClient, baseURL, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("valid push status = %d, want 200. body=%s", resp.StatusCode, string(b))
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var pr bientotv1.PushResponse
	if err := proto.Unmarshal(respBytes, &pr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if pr.Status != "ok" {
		t.Errorf("status = %q, want ok", pr.Status)
	}
}

func TestPush_InvalidVersion(t *testing.T) {
	baseURL, signKey, httpClient, cleanup := testSetup(t)
	defer cleanup()

	body := buildPush(t, signKey, func(req *bientotv1.PushRequest) {
		req.V = 99
	})

	resp := postPush(t, httpClient, baseURL, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d, want 400. body=%s", resp.StatusCode, string(b))
	}
}

func TestPush_StaleTimestamp(t *testing.T) {
	baseURL, signKey, httpClient, cleanup := testSetup(t)
	defer cleanup()

	body := buildPush(t, signKey, func(req *bientotv1.PushRequest) {
		req.TimestampNs = time.Now().Add(-10 * time.Minute).UnixNano()
	})

	resp := postPush(t, httpClient, baseURL, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPush_FutureTimestamp(t *testing.T) {
	baseURL, signKey, httpClient, cleanup := testSetup(t)
	defer cleanup()

	body := buildPush(t, signKey, func(req *bientotv1.PushRequest) {
		req.TimestampNs = time.Now().Add(10 * time.Minute).UnixNano()
	})

	resp := postPush(t, httpClient, baseURL, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPush_ReplayRejected(t *testing.T) {
	baseURL, signKey, httpClient, cleanup := testSetup(t)
	defer cleanup()

	nonce := uuid.NewString()

	body1 := buildPush(t, signKey, func(req *bientotv1.PushRequest) {
		req.Nonce = nonce
	})
	resp1 := postPush(t, httpClient, baseURL, body1)
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first push status = %d, want 200", resp1.StatusCode)
	}

	body2 := buildPush(t, signKey, func(req *bientotv1.PushRequest) {
		req.Nonce = nonce
	})
	resp2 := postPush(t, httpClient, baseURL, body2)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("replay status = %d, want 409", resp2.StatusCode)
	}
}

func TestPush_MachineIDMismatch(t *testing.T) {
	baseURL, signKey, httpClient, cleanup := testSetup(t)
	defer cleanup()

	body := buildPush(t, signKey, func(req *bientotv1.PushRequest) {
		req.MachineId = "attacker"
	})

	resp := postPush(t, httpClient, baseURL, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
}

func TestPush_TamperedMessage(t *testing.T) {
	baseURL, signKey, httpClient, cleanup := testSetup(t)
	defer cleanup()

	req := &bientotv1.PushRequest{
		V:           1,
		MachineId:   testMachineID,
		TimestampNs: time.Now().UnixNano(),
		Nonce:       uuid.NewString(),
		Modules: []*bientotv1.ModuleData{
			{Module: "heartbeat", TimestampNs: time.Now().UnixNano()},
		},
	}

	signed, err := crypto.Sign(req, signKey)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Mutate after signing: signature no longer matches canonical bytes.
	signed.Modules = append(signed.Modules, &bientotv1.ModuleData{
		Module:      "tampered",
		TimestampNs: time.Now().UnixNano(),
	})

	body, err := proto.Marshal(signed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	resp := postPush(t, httpClient, baseURL, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (signature mismatch)", resp.StatusCode)
	}
}
