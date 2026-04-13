package veille

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Alert represents a veille-secu alert.
type Alert struct {
	ID           int64     `json:"id"`
	SourceID     string    `json:"source_id"`
	SourceName   string    `json:"source_name"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Link         string    `json:"link"`
	CVEID        string    `json:"cve_id"`
	CVSSScore    float64   `json:"cvss_score"`
	Severity     string    `json:"severity"`
	MatchedTools []string  `json:"matched_tools"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Notified     bool      `json:"notified"`
}

// Tool represents a software tool in veille-secu.
type Tool struct {
	ID       string   `json:"id,omitempty"`
	Name     string   `json:"name"`
	Keywords []string `json:"keywords"`
	Version  string   `json:"version,omitempty"`
	CPE      string   `json:"cpe,omitempty"`
	Source   string   `json:"source"`
}

// Client queries the veille-secu API.
type Client struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewClient creates a veille-secu API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

// Health checks if veille-secu is reachable.
func (c *Client) Health() error {
	resp, err := c.doGet("/health")
	if err != nil {
		return fmt.Errorf("veille-secu unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("veille-secu health: status %d", resp.StatusCode)
	}
	return nil
}

// FetchAlerts retrieves alerts, optionally filtered by status and severity.
func (c *Client) FetchAlerts(status string, severities []string, limit int) ([]Alert, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/alerts", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	q := req.URL.Query()
	if status != "" {
		q.Set("status", status)
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	for _, sev := range severities {
		q.Add("severity", sev)
	}
	req.URL.RawQuery = q.Encode()
	c.setAuth(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching alerts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("veille-secu alerts: status %d", resp.StatusCode)
	}

	var wrapper struct {
		Alerts []Alert `json:"alerts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decoding alerts: %w", err)
	}

	return wrapper.Alerts, nil
}

// AddTool registers a software tool in veille-secu for matching.
func (c *Client) AddTool(tool Tool) error {
	data, err := json.Marshal(tool)
	if err != nil {
		return fmt.Errorf("marshaling tool: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+"/api/tools", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuth(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("adding tool: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("veille-secu add tool: status %d", resp.StatusCode)
	}

	return nil
}

// FetchTools lists all watched tools.
func (c *Client) FetchTools() ([]Tool, error) {
	resp, err := c.doGet("/api/tools")
	if err != nil {
		return nil, fmt.Errorf("fetching tools: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("veille-secu tools: status %d", resp.StatusCode)
	}

	var tools []Tool
	if err := json.NewDecoder(resp.Body).Decode(&tools); err != nil {
		return nil, fmt.Errorf("decoding tools: %w", err)
	}

	return tools, nil
}

func (c *Client) doGet(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	return c.client.Do(req)
}

func (c *Client) setAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}
