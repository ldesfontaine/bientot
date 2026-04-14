package alerter

import (
	"fmt"
	"strings"

	"github.com/ldesfontaine/bientot/internal"
)

// Operator définit les opérateurs de comparaison
type Operator string

const (
	OpGreater      Operator = ">"
	OpGreaterEqual Operator = ">="
	OpLess         Operator = "<"
	OpLessEqual    Operator = "<="
	OpEqual        Operator = "=="
	OpNotEqual     Operator = "!="
)

// Rule définit une règle d'alerte
type Rule struct {
	Name       string
	MetricName string
	Labels     map[string]string
	Operator   Operator
	Threshold  float64
	Severity   internal.Severity
	Message    string
}

// Evaluate vérifie si la condition de la règle est remplie
func (r Rule) Evaluate(value float64) bool {
	switch r.Operator {
	case OpGreater:
		return value > r.Threshold
	case OpGreaterEqual:
		return value >= r.Threshold
	case OpLess:
		return value < r.Threshold
	case OpLessEqual:
		return value <= r.Threshold
	case OpEqual:
		return value == r.Threshold
	case OpNotEqual:
		return value != r.Threshold
	default:
		return false
	}
}

// FormatMessage formate le message d'alerte avec les données de métrique
func (r Rule) FormatMessage(value float64, labels map[string]string) string {
	msg := r.Message
	msg = strings.ReplaceAll(msg, "{{ .Value }}", fmt.Sprintf("%.2f", value))

	for k, v := range labels {
		msg = strings.ReplaceAll(msg, fmt.Sprintf("{{ .Labels.%s }}", k), v)
	}

	return msg
}

// ParseOperator analyse une chaîne d'opérateur
func ParseOperator(s string) (Operator, error) {
	switch s {
	case ">":
		return OpGreater, nil
	case ">=":
		return OpGreaterEqual, nil
	case "<":
		return OpLess, nil
	case "<=":
		return OpLessEqual, nil
	case "==":
		return OpEqual, nil
	case "!=":
		return OpNotEqual, nil
	default:
		return "", fmt.Errorf("opérateur inconnu: %s", s)
	}
}
