// Package promparse parses the Prometheus text exposition format.
// Spec: https://prometheus.io/docs/instrumenting/exposition_formats/
package promparse

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Sample is one parsed metric data point.
type Sample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// Parse reads Prometheus text format from r and returns all samples.
// Lines starting with # (HELP, TYPE) and empty lines are skipped.
// Returns a descriptive error on malformed lines.
func Parse(r io.Reader) ([]Sample, error) {
	var samples []Sample
	scanner := bufio.NewScanner(r)

	// Some node_exporter lines (many labels) can exceed the default 64 KB.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		sample, err := parseLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		samples = append(samples, sample)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	return samples, nil
}

// parseLine parses one metric line.
// Format examples:
//
//	node_memory_MemTotal_bytes 8.3886848e+09
//	node_cpu_seconds_total{cpu="0",mode="idle"} 123456.78
//	node_scrape_duration{} 0.01
func parseLine(line string) (Sample, error) {
	var name string
	var rest string

	if idx := strings.IndexAny(line, "{ "); idx >= 0 {
		name = line[:idx]
		rest = line[idx:]
	} else {
		return Sample{}, fmt.Errorf("no separator after metric name: %q", line)
	}

	labels := make(map[string]string)
	if strings.HasPrefix(rest, "{") {
		closeIdx := strings.Index(rest, "}")
		if closeIdx < 0 {
			return Sample{}, fmt.Errorf("unclosed label set: %q", line)
		}

		labelStr := rest[1:closeIdx]
		rest = rest[closeIdx+1:]

		if labelStr != "" {
			parsed, err := parseLabels(labelStr)
			if err != nil {
				return Sample{}, fmt.Errorf("labels: %w", err)
			}
			labels = parsed
		}
	}

	rest = strings.TrimSpace(rest)
	if rest == "" {
		return Sample{}, fmt.Errorf("missing value: %q", line)
	}

	// Optional trailing timestamp is ignored (format: "value timestamp").
	fields := strings.Fields(rest)
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return Sample{}, fmt.Errorf("parse value %q: %w", fields[0], err)
	}

	return Sample{Name: name, Labels: labels, Value: value}, nil
}

// parseLabels parses: cpu="0",mode="idle"
// Does NOT handle escaped quotes in values (unlikely in node_exporter output).
func parseLabels(s string) (map[string]string, error) {
	labels := make(map[string]string)

	for _, pair := range splitTopLevel(s, ',') {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		eq := strings.Index(pair, "=")
		if eq < 0 {
			return nil, fmt.Errorf("no = in label pair %q", pair)
		}

		key := strings.TrimSpace(pair[:eq])
		val := strings.TrimSpace(pair[eq+1:])

		if len(val) < 2 || val[0] != '"' || val[len(val)-1] != '"' {
			return nil, fmt.Errorf("label value not quoted: %q", val)
		}
		val = val[1 : len(val)-1]

		labels[key] = val
	}

	return labels, nil
}

// splitTopLevel splits s on sep, ignoring separators inside double-quoted strings.
// Label values may legitimately contain the separator, e.g. {path="/a,b"}.
func splitTopLevel(s string, sep rune) []string {
	var parts []string
	var current strings.Builder
	inQuote := false

	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			current.WriteRune(r)
		case r == sep && !inQuote:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}
