package alerter

import (
	"testing"

	"github.com/ldesfontaine/bientot/internal"
)

func TestParseExpression(t *testing.T) {
	tests := []struct {
		name      string
		expr      string
		wantName  string
		wantOp    Operator
		wantThres float64
		wantErr   bool
	}{
		{
			name:      "simple greater than",
			expr:      "disk_used_percent > 90",
			wantName:  "disk_used_percent",
			wantOp:    OpGreater,
			wantThres: 90,
			wantErr:   false,
		},
		{
			name:      "greater or equal",
			expr:      "memory_usage >= 85.5",
			wantName:  "memory_usage",
			wantOp:    OpGreaterEqual,
			wantThres: 85.5,
			wantErr:   false,
		},
		{
			name:      "less than",
			expr:      "container_health < 2",
			wantName:  "container_health",
			wantOp:    OpLess,
			wantThres: 2,
			wantErr:   false,
		},
		{
			name:      "equals",
			expr:      "container_running == 0",
			wantName:  "container_running",
			wantOp:    OpEqual,
			wantThres: 0,
			wantErr:   false,
		},
		{
			name:      "not equals",
			expr:      "zfs_pool_health != 2",
			wantName:  "zfs_pool_health",
			wantOp:    OpNotEqual,
			wantThres: 2,
			wantErr:   false,
		},
		{
			name:    "invalid expression",
			expr:    "invalid",
			wantErr: true,
		},
		{
			name:    "invalid operator",
			expr:    "metric ?? 10",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseExpression(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseExpression() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if got.MetricName != tt.wantName {
				t.Errorf("MetricName = %v, want %v", got.MetricName, tt.wantName)
			}
			if got.Operator != tt.wantOp {
				t.Errorf("Operator = %v, want %v", got.Operator, tt.wantOp)
			}
			if got.Threshold != tt.wantThres {
				t.Errorf("Threshold = %v, want %v", got.Threshold, tt.wantThres)
			}
		})
	}
}

func TestParseExpressionWithLabels(t *testing.T) {
	expr := `container_health{name="nginx"} == 1`
	got, err := ParseExpression(expr)
	if err != nil {
		t.Fatalf("ParseExpression() error = %v", err)
	}

	if got.MetricName != "container_health" {
		t.Errorf("MetricName = %v, want container_health", got.MetricName)
	}
	if got.Labels["name"] != "nginx" {
		t.Errorf("Labels[name] = %v, want nginx", got.Labels["name"])
	}
	if got.Operator != OpEqual {
		t.Errorf("Operator = %v, want ==", got.Operator)
	}
	if got.Threshold != 1 {
		t.Errorf("Threshold = %v, want 1", got.Threshold)
	}
}

func TestRuleEvaluate(t *testing.T) {
	tests := []struct {
		name     string
		rule     Rule
		value    float64
		expected bool
	}{
		{
			name:     "above threshold",
			rule:     Rule{Operator: OpGreater, Threshold: 90},
			value:    95,
			expected: true,
		},
		{
			name:     "at threshold (greater)",
			rule:     Rule{Operator: OpGreater, Threshold: 90},
			value:    90,
			expected: false,
		},
		{
			name:     "below threshold",
			rule:     Rule{Operator: OpGreater, Threshold: 90},
			value:    85,
			expected: false,
		},
		{
			name:     "at threshold (greater or equal)",
			rule:     Rule{Operator: OpGreaterEqual, Threshold: 90},
			value:    90,
			expected: true,
		},
		{
			name:     "equals true",
			rule:     Rule{Operator: OpEqual, Threshold: 0},
			value:    0,
			expected: true,
		},
		{
			name:     "equals false",
			rule:     Rule{Operator: OpEqual, Threshold: 0},
			value:    1,
			expected: false,
		},
		{
			name:     "not equals true",
			rule:     Rule{Operator: OpNotEqual, Threshold: 2},
			value:    1,
			expected: true,
		},
		{
			name:     "less than true",
			rule:     Rule{Operator: OpLess, Threshold: 2},
			value:    1,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rule.Evaluate(tt.value)
			if got != tt.expected {
				t.Errorf("Evaluate(%v) = %v, want %v", tt.value, got, tt.expected)
			}
		})
	}
}

func TestRuleFormatMessage(t *testing.T) {
	rule := Rule{
		Message: "Disk usage critical: {{ .Value }}% on {{ .Labels.device }}",
	}
	labels := map[string]string{"device": "/dev/sda1"}

	got := rule.FormatMessage(95.5, labels)
	want := "Disk usage critical: 95.50% on /dev/sda1"

	if got != want {
		t.Errorf("FormatMessage() = %v, want %v", got, want)
	}
}

func TestExpressionToRule(t *testing.T) {
	expr := &Expression{
		MetricName: "disk_used_percent",
		Labels:     map[string]string{"mount": "/"},
		Operator:   OpGreater,
		Threshold:  90,
	}

	rule := expr.ToRule("disk_critical", internal.SeverityCritical, "Disk is full!")

	if rule.Name != "disk_critical" {
		t.Errorf("Name = %v, want disk_critical", rule.Name)
	}
	if rule.MetricName != "disk_used_percent" {
		t.Errorf("MetricName = %v, want disk_used_percent", rule.MetricName)
	}
	if rule.Severity != internal.SeverityCritical {
		t.Errorf("Severity = %v, want critical", rule.Severity)
	}
	if rule.Threshold != 90 {
		t.Errorf("Threshold = %v, want 90", rule.Threshold)
	}
}
