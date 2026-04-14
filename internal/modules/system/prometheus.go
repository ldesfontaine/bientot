package system

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// promMetric représente une seule ligne de métrique Prometheus analysée.
type promMetric struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// parsePrometheusMetrics analyse le format texte d'exposition Prometheus.
// return toutes les lignes de métriques en slice (supporte les labels et doublons).
func parsePrometheusMetrics(body io.Reader) []promMetric {
	var result []promMetric
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		m, ok := parseLine(line)
		if !ok {
			continue
		}
		result = append(result, m)
	}
	return result
}

// parseLine analyse une seule ligne de métrique Prometheus.
// Format : metric_name{label="value",...} value [timestamp]
func parseLine(line string) (promMetric, bool) {
	var m promMetric

	// Séparation des labels du nom
	braceOpen := strings.IndexByte(line, '{')
	if braceOpen >= 0 {
		m.Name = line[:braceOpen]
		braceClose := strings.IndexByte(line[braceOpen:], '}')
		if braceClose < 0 {
			return m, false
		}
		braceClose += braceOpen
		m.Labels = parseLabels(line[braceOpen+1 : braceClose])
		line = strings.TrimSpace(line[braceClose+1:])
	} else {
		// Pas de labels : "metric_name value [timestamp]"
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return m, false
		}
		m.Name = parts[0]
		line = parts[1]
	}

	// Analyse de la valeur (premier champ restant)
	valStr := strings.Fields(line)[0]
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return m, false
	}
	m.Value = val
	return m, true
}

// parseLabels analyse label="value",label2="value2"
func parseLabels(s string) map[string]string {
	labels := make(map[string]string)
	for s != "" {
		// Trouver la clé
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			break
		}
		key := s[:eq]
		s = s[eq+1:]

		// La valeur est entre guillemets
		if len(s) == 0 || s[0] != '"' {
			break
		}
		s = s[1:] // skip opening quote
		end := strings.IndexByte(s, '"')
		if end < 0 {
			break
		}
		labels[key] = s[:end]
		s = s[end+1:]

		// Ignorer la virgule
		if len(s) > 0 && s[0] == ',' {
			s = s[1:]
		}
	}
	return labels
}
