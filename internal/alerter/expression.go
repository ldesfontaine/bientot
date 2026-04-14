package alerter

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ldesfontaine/bientot/internal"
)

// Expression représente une expression d'alerte analysée
type Expression struct {
	MetricName string
	Labels     map[string]string
	Operator   Operator
	Threshold  float64
}

var exprRegex = regexp.MustCompile(`^(\w+)(?:\{([^}]*)\})?\s*(>=|<=|==|!=|>|<)\s*(.+)$`)

// ParseExpression analyse une expression comme "disk_used_percent > 90"
func ParseExpression(expr string) (*Expression, error) {
	expr = strings.TrimSpace(expr)

	matches := exprRegex.FindStringSubmatch(expr)
	if matches == nil {
		return nil, fmt.Errorf("format d'expression invalide: %s", expr)
	}

	metricName := matches[1]
	labelsStr := matches[2]
	opStr := matches[3]
	thresholdStr := matches[4]

	// Analyse de l'opérateur
	op, err := ParseOperator(opStr)
	if err != nil {
		return nil, err
	}

	// Analyse du seuil
	threshold, err := strconv.ParseFloat(strings.TrimSpace(thresholdStr), 64)
	if err != nil {
		return nil, fmt.Errorf("seuil invalide: %s", thresholdStr)
	}

	// Analyse des labels si présents
	labels := make(map[string]string)
	if labelsStr != "" {
		pairs := strings.Split(labelsStr, ",")
		for _, pair := range pairs {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) == 2 {
				key := strings.TrimSpace(kv[0])
				val := strings.Trim(strings.TrimSpace(kv[1]), "\"'")
				labels[key] = val
			}
		}
	}

	return &Expression{
		MetricName: metricName,
		Labels:     labels,
		Operator:   op,
		Threshold:  threshold,
	}, nil
}

// ToRule convertit une Expression en Rule
func (e *Expression) ToRule(name string, severity internal.Severity, message string) Rule {
	return Rule{
		Name:       name,
		MetricName: e.MetricName,
		Labels:     e.Labels,
		Operator:   e.Operator,
		Threshold:  e.Threshold,
		Severity:   severity,
		Message:    message,
	}
}
