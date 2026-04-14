package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// CrowdSecCollector collecte les métriques depuis l'API CrowdSec
type CrowdSecCollector struct {
	name     string
	url      string
	interval time.Duration
	client   *http.Client
}

type crowdsecMetrics struct {
	Decisions struct {
		Total  int `json:"total"`
		Active int `json:"active"`
	} `json:"decisions"`
	Alerts struct {
		Total int `json:"total"`
	} `json:"alerts"`
	Parsers struct {
		Total   int `json:"total"`
		Parsed  int `json:"parsed"`
		Unparsed int `json:"unparsed"`
	} `json:"parsers"`
	Buckets struct {
		Total int `json:"total"`
	} `json:"buckets"`
}

// NewCrowdSecCollector crée un nouveau collecteur CrowdSec
func NewCrowdSecCollector(name, url string, interval time.Duration) *CrowdSecCollector {
	return &CrowdSecCollector{
		name:     name,
		url:      url,
		interval: interval,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *CrowdSecCollector) Name() string           { return c.name }
func (c *CrowdSecCollector) Type() string           { return "crowdsec" }
func (c *CrowdSecCollector) Interval() time.Duration { return c.interval }

func (c *CrowdSecCollector) Collect(ctx context.Context) ([]internal.Metric, error) {
	now := time.Now()
	var metrics []internal.Metric

	// Récupération des métriques depuis l'API CrowdSec
	metricsData, err := c.fetchMetrics(ctx)
	if err != nil {
		return nil, err
	}

	// Décisions
	metrics = append(metrics, internal.Metric{
		Name:      "crowdsec_decisions_total",
		Value:     float64(metricsData.Decisions.Total),
		Timestamp: now,
		Source:    c.name,
	})
	metrics = append(metrics, internal.Metric{
		Name:      "crowdsec_decisions_active",
		Value:     float64(metricsData.Decisions.Active),
		Timestamp: now,
		Source:    c.name,
	})

	// Alertes
	metrics = append(metrics, internal.Metric{
		Name:      "crowdsec_alerts_total",
		Value:     float64(metricsData.Alerts.Total),
		Timestamp: now,
		Source:    c.name,
	})

	// Parseurs
	metrics = append(metrics, internal.Metric{
		Name:      "crowdsec_parsed_total",
		Value:     float64(metricsData.Parsers.Parsed),
		Timestamp: now,
		Source:    c.name,
	})
	metrics = append(metrics, internal.Metric{
		Name:      "crowdsec_unparsed_total",
		Value:     float64(metricsData.Parsers.Unparsed),
		Timestamp: now,
		Source:    c.name,
	})

	// Buckets (seaux)
	metrics = append(metrics, internal.Metric{
		Name:      "crowdsec_buckets_total",
		Value:     float64(metricsData.Buckets.Total),
		Timestamp: now,
		Source:    c.name,
	})

	// Récupération des décisions actives (bans)
	bans, err := c.fetchDecisions(ctx)
	if err == nil {
		metrics = append(metrics, internal.Metric{
			Name:      "crowdsec_bans_active",
			Value:     float64(bans),
			Timestamp: now,
			Source:    c.name,
		})
	}

	return metrics, nil
}

func (c *CrowdSecCollector) fetchMetrics(ctx context.Context) (*crowdsecMetrics, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.url+"/v1/metrics", nil)
	if err != nil {
		return nil, fmt.Errorf("création de la requête: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("récupération des métriques: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("statut inattendu: %d", resp.StatusCode)
	}

	var metrics crowdsecMetrics
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		return nil, fmt.Errorf("décodage de la réponse: %w", err)
	}

	return &metrics, nil
}

func (c *CrowdSecCollector) fetchDecisions(ctx context.Context) (int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.url+"/v1/decisions", nil)
	if err != nil {
		return 0, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("statut inattendu: %d", resp.StatusCode)
	}

	var decisions []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&decisions); err != nil {
		return 0, err
	}

	return len(decisions), nil
}
