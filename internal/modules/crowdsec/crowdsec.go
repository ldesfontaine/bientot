package crowdsec

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

type metricsResp struct {
	Decisions struct {
		Total  int `json:"total"`
		Active int `json:"active"`
	} `json:"decisions"`
	Alerts struct {
		Total int `json:"total"`
	} `json:"alerts"`
	Parsers struct {
		Parsed   int `json:"parsed"`
		Unparsed int `json:"unparsed"`
	} `json:"parsers"`
	Buckets struct {
		Total int `json:"total"`
	} `json:"buckets"`
}

// Module collecte les métriques CrowdSec depuis les endpoints HTTP LAPI.
// Actif uniquement quand CROWDSEC_URL est défini (VPS). Ignoré sur Pi.
type Module struct {
	url    string // e.g. "http://crowdsec:8080"
	client *http.Client
}

func New(url string) *Module {
	return &Module{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (m *Module) Name() string { return "crowdsec" }

func (m *Module) Detect() bool {
	if m.url == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", m.url+"/v1/metrics", nil)
	if err != nil {
		return false
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (m *Module) Collect(ctx context.Context) (transport.ModuleData, error) {
	now := time.Now()

	apiMetrics, err := m.fetchMetrics(ctx)
	if err != nil {
		slog.Warn("crowdsec metrics unreachable", "error", err)
		return transport.ModuleData{
			Module:    "crowdsec",
			Metrics:   []transport.MetricPoint{},
			Timestamp: now,
		}, nil
	}

	metrics := []transport.MetricPoint{
		{Name: "crowdsec_decisions_total", Value: float64(apiMetrics.Decisions.Total)},
		{Name: "crowdsec_decisions_active", Value: float64(apiMetrics.Decisions.Active)},
		{Name: "crowdsec_alerts_total", Value: float64(apiMetrics.Alerts.Total)},
		{Name: "crowdsec_parsed_total", Value: float64(apiMetrics.Parsers.Parsed)},
		{Name: "crowdsec_unparsed_total", Value: float64(apiMetrics.Parsers.Unparsed)},
		{Name: "crowdsec_buckets_total", Value: float64(apiMetrics.Buckets.Total)},
	}

	bans, err := m.fetchDecisionCount(ctx)
	if err == nil {
		metrics = append(metrics, transport.MetricPoint{
			Name: "crowdsec_bans_active", Value: float64(bans),
		})
	}

	return transport.ModuleData{
		Module:    "crowdsec",
		Metrics:   metrics,
		Timestamp: now,
	}, nil
}

func (m *Module) fetchMetrics(ctx context.Context) (*metricsResp, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", m.url+"/v1/metrics", nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("statut inattendu : %d", resp.StatusCode)
	}
	var data metricsResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (m *Module) fetchDecisionCount(ctx context.Context) (int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", m.url+"/v1/decisions", nil)
	if err != nil {
		return 0, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("statut inattendu : %d", resp.StatusCode)
	}
	var decisions []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&decisions); err != nil {
		return 0, err
	}
	return len(decisions), nil
}
