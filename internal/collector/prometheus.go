package collector

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// PrometheusCollector scrape les endpoints de métriques au format Prometheus
type PrometheusCollector struct {
	name     string
	url      string
	interval time.Duration
	client   *http.Client
}

// NewPrometheusCollector crée un nouveau collecteur Prometheus
func NewPrometheusCollector(name, url string, interval time.Duration) *PrometheusCollector {
	return &PrometheusCollector{
		name:     name,
		url:      url,
		interval: interval,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *PrometheusCollector) Name() string     { return c.name }
func (c *PrometheusCollector) Type() string     { return "prometheus" }
func (c *PrometheusCollector) Interval() time.Duration { return c.interval }

func (c *PrometheusCollector) Collect(ctx context.Context) ([]internal.Metric, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.url, nil)
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

	return c.parse(resp.Body, time.Now())
}

func (c *PrometheusCollector) parse(r interface{ Read([]byte) (int, error) }, ts time.Time) ([]internal.Metric, error) {
	var metrics []internal.Metric
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		metric, err := c.parseLine(line, ts)
		if err != nil {
			continue // ignore les lignes malformées
		}
		metrics = append(metrics, metric)
	}

	return metrics, scanner.Err()
}

func (c *PrometheusCollector) parseLine(line string, ts time.Time) (internal.Metric, error) {
	// Analyse : metric_name{label="value"} 123.45
	// ou :     metric_name 123.45

	var name string
	var labels map[string]string
	var valueStr string

	if idx := strings.Index(line, "{"); idx != -1 {
		name = line[:idx]
		endIdx := strings.Index(line, "}")
		if endIdx == -1 {
			return internal.Metric{}, fmt.Errorf("labels malformés")
		}
		labels = parseLabels(line[idx+1 : endIdx])
		valueStr = strings.TrimSpace(line[endIdx+1:])
	} else {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return internal.Metric{}, fmt.Errorf("ligne malformée")
		}
		name = parts[0]
		valueStr = parts[1]
	}

	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return internal.Metric{}, fmt.Errorf("analyse de la valeur: %w", err)
	}

	return internal.Metric{
		Name:      name,
		Value:     value,
		Labels:    labels,
		Timestamp: ts,
		Source:    c.name,
	}, nil
}

func parseLabels(s string) map[string]string {
	labels := make(map[string]string)
	pairs := strings.Split(s, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			val := strings.Trim(strings.TrimSpace(kv[1]), "\"")
			labels[key] = val
		}
	}
	return labels
}
