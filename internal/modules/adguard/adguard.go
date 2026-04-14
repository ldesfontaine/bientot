package adguard

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

type statusResp struct {
	Version           string `json:"version"`
	Running           bool   `json:"running"`
	ProtectionEnabled bool   `json:"protection_enabled"`
}

type statsResp struct {
	NumDNSQueries           int     `json:"num_dns_queries"`
	NumBlockedFiltering     int     `json:"num_blocked_filtering"`
	NumReplacedSafebrowsing int     `json:"num_replaced_safebrowsing"`
	NumReplacedParental     int     `json:"num_replaced_parental"`
	AvgProcessingTime       float64 `json:"avg_processing_time"`
}

// Module collecte les métriques AdGuard Home via son API REST.
// Actif uniquement quand ADGUARD_URL est défini (Pi). Ignoré sur VPS.
type Module struct {
	url      string // e.g. "http://adguard:3000"
	user     string
	password string
	client   *http.Client
}

func New(url, user, password string) *Module {
	return &Module{
		url:      url,
		user:     user,
		password: password,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (m *Module) Name() string { return "adguard" }

func (m *Module) Detect() bool {
	if m.url == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", m.url+"/control/status", nil)
	if err != nil {
		return false
	}
	m.setAuth(req)
	resp, err := m.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (m *Module) Collect(ctx context.Context) (transport.ModuleData, error) {
	now := time.Now()

	status, err := m.fetchStatus(ctx)
	if err != nil {
		slog.Warn("adguard status unreachable", "error", err)
		return transport.ModuleData{
			Module:    "adguard",
			Metrics:   []transport.MetricPoint{},
			Timestamp: now,
		}, nil
	}

	stats, err := m.fetchStats(ctx)
	if err != nil {
		slog.Warn("adguard stats unreachable", "error", err)
		return transport.ModuleData{
			Module:    "adguard",
			Metrics:   []transport.MetricPoint{},
			Timestamp: now,
		}, nil
	}

	protection := 0.0
	if status.ProtectionEnabled {
		protection = 1.0
	}

	totalBlocked := stats.NumBlockedFiltering + stats.NumReplacedSafebrowsing + stats.NumReplacedParental
	blockRatio := 0.0
	if stats.NumDNSQueries > 0 {
		blockRatio = float64(totalBlocked) / float64(stats.NumDNSQueries) * 100
	}

	metrics := []transport.MetricPoint{
		{Name: "adguard_protection_enabled", Value: protection},
		{Name: "adguard_queries_total", Value: float64(stats.NumDNSQueries)},
		{Name: "adguard_queries_blocked", Value: float64(totalBlocked)},
		{Name: "adguard_block_ratio", Value: blockRatio},
		{Name: "adguard_avg_processing_time", Value: stats.AvgProcessingTime},
	}

	metadata := map[string]string{
		"version": status.Version,
	}

	return transport.ModuleData{
		Module:    "adguard",
		Metrics:   metrics,
		Metadata:  metadata,
		Timestamp: now,
	}, nil
}

func (m *Module) setAuth(req *http.Request) {
	if m.user != "" {
		req.SetBasicAuth(m.user, m.password)
	}
}

func (m *Module) fetchStatus(ctx context.Context) (*statusResp, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", m.url+"/control/status", nil)
	if err != nil {
		return nil, err
	}
	m.setAuth(req)
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("statut inattendu : %d", resp.StatusCode)
	}
	var data statusResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (m *Module) fetchStats(ctx context.Context) (*statsResp, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", m.url+"/control/stats", nil)
	if err != nil {
		return nil, err
	}
	m.setAuth(req)
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("statut inattendu : %d", resp.StatusCode)
	}
	var data statsResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
}
