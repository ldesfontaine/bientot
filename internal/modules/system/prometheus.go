package system

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// promMetric represents a single parsed Prometheus metric line.
type promMetric struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// parsePrometheusMetrics parses Prometheus text exposition format.
// Returns all metric lines as a slice (supports labels and duplicates).
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

// parseLine parses a single Prometheus metric line.
// Format: metric_name{label="value",...} value [timestamp]
func parseLine(line string) (promMetric, bool) {
	var m promMetric

	// Split labels from name
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
		// No labels: "metric_name value [timestamp]"
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return m, false
		}
		m.Name = parts[0]
		line = parts[1]
	}

	// Parse value (first field of remaining)
	valStr := strings.Fields(line)[0]
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return m, false
	}
	m.Value = val
	return m, true
}

// parseLabels parses label="value",label2="value2"
func parseLabels(s string) map[string]string {
	labels := make(map[string]string)
	for s != "" {
		// Find key
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			break
		}
		key := s[:eq]
		s = s[eq+1:]

		// Value is quoted
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

		// Skip comma
		if len(s) > 0 && s[0] == ',' {
			s = s[1:]
		}
	}
	return labels
}
