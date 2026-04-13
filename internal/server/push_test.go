package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// mockStorage implements storage.Storage for testing push validation only.
type mockStorage struct{}

func (m *mockStorage) Write(_ interface{}, _ interface{}) error                    { return nil }
func (m *mockStorage) Query(_ interface{}, _ string, _, _ time.Time, _ interface{}) (interface{}, error) {
	return nil, nil
}
func (m *mockStorage) QueryLatest(_ interface{}, _ string, _ map[string]string) (interface{}, error) {
	return nil, nil
}
func (m *mockStorage) List(_ interface{}) ([]string, error)          { return nil, nil }
func (m *mockStorage) Downsample(_ interface{}) error                 { return nil }
func (m *mockStorage) Cleanup(_ interface{}) error                    { return nil }
func (m *mockStorage) Close() error                                   { return nil }
func (m *mockStorage) InsertLogs(_ interface{}, _ interface{}) error  { return nil }
func (m *mockStorage) QueryLogs(_ interface{}, _, _, _ string, _ time.Time, _ int) (interface{}, error) {
	return nil, nil
}
func (m *mockStorage) QueryLogStats(_ interface{}) (interface{}, error) { return nil, nil }
func (m *mockStorage) PurgeLogs(_ interface{}, _ time.Duration) error   { return nil }

func TestHandlePush_ValidPayload(t *testing.T) {
	token := "test-secret"
	machineID := "test-machine"

	body := transport.Body{
		Modules: []transport.ModuleData{
			{
				Module:    "system",
				Metrics:   []transport.MetricPoint{{Name: "cpu", Value: 42.0}},
				Timestamp: time.Now(),
			},
		},
	}

	sig, err := transport.Sign(body, token)
	if err != nil {
		t.Fatal(err)
	}

	payload := transport.Payload{
		MachineID: machineID,
		Timestamp: time.Now(),
		Nonce:     transport.NewNonce(),
		Signature: sig,
		Body:      body,
	}

	data, _ := json.Marshal(payload)

	// We test the validation logic directly via payloadToMetrics
	metrics := payloadToMetrics(payload)
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if metrics[0].Name != "cpu" {
		t.Fatalf("expected metric name 'cpu', got %q", metrics[0].Name)
	}
	if metrics[0].Labels["machine_id"] != machineID {
		t.Fatalf("expected machine_id label %q, got %q", machineID, metrics[0].Labels["machine_id"])
	}

	// Verify the JSON is valid
	var decoded transport.Payload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("payload JSON roundtrip failed: %v", err)
	}
}

func TestHandlePush_UnknownMachine(t *testing.T) {
	srv := newTestServer("known-machine", "secret")

	body := transport.Body{}
	sig, _ := transport.Sign(body, "secret")
	payload := transport.Payload{
		MachineID: "unknown-machine",
		Timestamp: time.Now(),
		Nonce:     transport.NewNonce(),
		Signature: sig,
		Body:      body,
	}

	data, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/push", bytes.NewReader(data))
	w := httptest.NewRecorder()

	srv.agentRouter().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandlePush_BadSignature(t *testing.T) {
	srv := newTestServer("machine1", "correct-secret")

	body := transport.Body{}
	sig, _ := transport.Sign(body, "wrong-secret")
	payload := transport.Payload{
		MachineID: "machine1",
		Timestamp: time.Now(),
		Nonce:     transport.NewNonce(),
		Signature: sig,
		Body:      body,
	}

	data, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/push", bytes.NewReader(data))
	w := httptest.NewRecorder()

	srv.agentRouter().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func newTestServer(machineID, token string) *Server {
	return New(
		Config{
			Agents: []AgentToken{{MachineID: machineID, Token: token}},
		},
		nil, // storage not needed for auth tests
		slog.Default(),
	)
}
