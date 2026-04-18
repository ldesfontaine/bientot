// Package client provides the HTTP client used by the agent to talk to the dashboard.
package client

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
	"github.com/ldesfontaine/bientot/internal/shared/crypto"
	"github.com/ldesfontaine/bientot/internal/shared/mtls"
)

const protocolVersion = 1

// Client is a mTLS HTTP client targeting a single dashboard base URL.
type Client struct {
	baseURL   string
	http      *http.Client
	signKey   ed25519.PrivateKey
	machineID string
}

// PingResponse is the JSON body returned by the dashboard /ping endpoint.
type PingResponse struct {
	From     string `json:"from"`
	ClientCN string `json:"client_cn"`
}

// New builds a Client with a mTLS transport. serverName must match a SAN of
// the server certificate — NOT necessarily the DNS host in baseURL.
//
// signKey and machineID are required for Push; they are intrinsic properties
// of the agent identity, not per-request parameters.
func New(
	baseURL, certPath, keyPath, caPath, serverName string,
	signKey ed25519.PrivateKey,
	machineID string,
) (*Client, error) {
	tlsConfig, err := mtls.ClientConfig(certPath, keyPath, caPath, serverName)
	if err != nil {
		return nil, fmt.Errorf("build tls config: %w", err)
	}

	transport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
		},
		signKey:   signKey,
		machineID: machineID,
	}, nil
}

// Ping issues a GET {baseURL}/ping and decodes the JSON response.
// Kept for healthcheck and manual debugging — not used by the normal push loop.
func (c *Client) Ping(ctx context.Context) (*PingResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/ping", nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var pr PingResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &pr, nil
}

// Push builds a signed PushRequest containing the given module data, marshals
// it as protobuf, and POSTs to {baseURL}/v1/push.
func (c *Client) Push(ctx context.Context, modules []*bientotv1.ModuleData) (*bientotv1.PushResponse, error) {
	req := &bientotv1.PushRequest{
		V:           protocolVersion,
		MachineId:   c.machineID,
		TimestampNs: time.Now().UnixNano(),
		Nonce:       uuid.NewString(),
		Modules:     modules,
	}

	signed, err := crypto.Sign(req, c.signKey)
	if err != nil {
		return nil, fmt.Errorf("sign request: %w", err)
	}

	body, err := proto.Marshal(signed)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.baseURL+"/v1/push",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-protobuf")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("push rejected: status=%d body=%s", resp.StatusCode, string(respBytes))
	}

	var pr bientotv1.PushResponse
	if err := proto.Unmarshal(respBytes, &pr); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &pr, nil
}
