package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// JSONFileCollector lit les métriques depuis des fichiers JSON (ex: statut de sauvegarde)
type JSONFileCollector struct {
	name     string
	path     string
	interval time.Duration
}

// NewJSONFileCollector crée un nouveau collecteur de fichier JSON
func NewJSONFileCollector(name, path string, interval time.Duration) *JSONFileCollector {
	return &JSONFileCollector{
		name:     name,
		path:     path,
		interval: interval,
	}
}

func (c *JSONFileCollector) Name() string           { return c.name }
func (c *JSONFileCollector) Type() string           { return "json_file" }
func (c *JSONFileCollector) Interval() time.Duration { return c.interval }

func (c *JSONFileCollector) Collect(ctx context.Context) ([]internal.Metric, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return nil, fmt.Errorf("lecture du fichier: %w", err)
	}

	var content map[string]interface{}
	if err := json.Unmarshal(data, &content); err != nil {
		return nil, fmt.Errorf("analyse du JSON: %w", err)
	}

	now := time.Now()
	metrics := c.extractMetrics(content, "", now)

	// Ajout du temps de modification du fichier comme métrique
	info, err := os.Stat(c.path)
	if err == nil {
		age := now.Sub(info.ModTime())
		metrics = append(metrics, internal.Metric{
			Name:      "json_file_age_seconds",
			Value:     age.Seconds(),
			Labels:    map[string]string{"file": c.name},
			Timestamp: now,
			Source:    c.name,
		})
	}

	return metrics, nil
}

func (c *JSONFileCollector) extractMetrics(data map[string]interface{}, prefix string, ts time.Time) []internal.Metric {
	var metrics []internal.Metric

	for key, value := range data {
		metricName := key
		if prefix != "" {
			metricName = prefix + "_" + key
		}

		switch v := value.(type) {
		case float64:
			metrics = append(metrics, internal.Metric{
				Name:      metricName,
				Value:     v,
				Labels:    map[string]string{"file": c.name},
				Timestamp: ts,
				Source:    c.name,
			})
		case bool:
			val := 0.0
			if v {
				val = 1.0
			}
			metrics = append(metrics, internal.Metric{
				Name:      metricName,
				Value:     val,
				Labels:    map[string]string{"file": c.name},
				Timestamp: ts,
				Source:    c.name,
			})
		case string:
			// Tentative d'analyse comme timestamp pour calculer l'âge
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				age := ts.Sub(t)
				metrics = append(metrics, internal.Metric{
					Name:      metricName + "_age_hours",
					Value:     age.Hours(),
					Labels:    map[string]string{"file": c.name},
					Timestamp: ts,
					Source:    c.name,
				})
			}
		case map[string]interface{}:
			// Récursion dans les objets imbriqués
			nested := c.extractMetrics(v, metricName, ts)
			metrics = append(metrics, nested...)
		}
	}

	return metrics
}
