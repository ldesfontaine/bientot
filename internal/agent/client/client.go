// Package client provides the HTTP client used by the agent to talk to the dashboard.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ldesfontaine/bientot/internal/shared/mtls"
)

// Client is a mTLS HTTP client targeting a single dashboard base URL.
type Client struct {
	baseURL string
	http    *http.Client
}

// PingResponse is the JSON body returned by the dashboard /ping endpoint.
type PingResponse struct {
	From     string `json:"from"`
	ClientCN string `json:"client_cn"`
}

// New builds a Client with a mTLS transport. serverName must match a SAN of
// the server certificate — NOT necessarily the DNS host in baseURL.
func New(baseURL, certPath, keyPath, caPath, serverName string) (*Client, error) {
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
	}, nil
}

// Ping issues a GET {baseURL}/ping and decodes the JSON response.
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
