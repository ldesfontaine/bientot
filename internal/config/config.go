package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ldesfontaine/bientot/internal"
	"github.com/ldesfontaine/bientot/internal/alerter"
)

// Config holds the application configuration
type Config struct {
	Targets TargetsConfig
	Alerts  AlertsConfig
}

// TargetsConfig holds targets.yml content
type TargetsConfig struct {
	Collectors CollectorsConfig `yaml:"collectors"`
}

type CollectorsConfig struct {
	Prometheus []PrometheusTarget `yaml:"prometheus"`
	CrowdSec   []CrowdSecTarget   `yaml:"crowdsec"`
	JSONFile   []JSONFileTarget   `yaml:"json_file"`
	Docker     DockerConfig       `yaml:"docker"`
	ZFS        ZFSConfig          `yaml:"zfs"`
}

type PrometheusTarget struct {
	Name     string        `yaml:"name"`
	URL      string        `yaml:"url"`
	Interval time.Duration `yaml:"interval"`
}

type CrowdSecTarget struct {
	Name     string        `yaml:"name"`
	URL      string        `yaml:"url"`
	Interval time.Duration `yaml:"interval"`
}

type JSONFileTarget struct {
	Name     string        `yaml:"name"`
	Path     string        `yaml:"path"`
	Interval time.Duration `yaml:"interval"`
}

type DockerConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Socket   string        `yaml:"socket"`
	Interval time.Duration `yaml:"interval"`
}

type ZFSConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Pools    []string      `yaml:"pools"`
	Interval time.Duration `yaml:"interval"`
}

// AlertsConfig holds alerts.yml content
type AlertsConfig struct {
	Alerts    []AlertRule      `yaml:"alerts"`
	Notifiers []NotifierConfig `yaml:"notifiers"`
}

type AlertRule struct {
	Name     string `yaml:"name"`
	Expr     string `yaml:"expr"`
	Severity string `yaml:"severity"`
	Message  string `yaml:"message"`
}

type NotifierConfig struct {
	Type           string   `yaml:"type"`
	URL            string   `yaml:"url"`
	Topic          string   `yaml:"topic"`
	SeverityFilter []string `yaml:"severity_filter"`
	Headers        map[string]string `yaml:"headers,omitempty"`
}

// LoadTargets loads targets.yml with environment variable expansion
func LoadTargets(path string) (*TargetsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	expanded := os.ExpandEnv(string(data))

	var config TargetsConfig
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadAlerts loads alerts.yml
func LoadAlerts(path string) (*AlertsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var config AlertsConfig
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// ParseRules converts AlertRule configs to alerter.Rule
func ParseRules(configs []AlertRule) ([]alerter.Rule, error) {
	var rules []alerter.Rule

	for _, cfg := range configs {
		rule, err := parseRule(cfg)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

func parseRule(cfg AlertRule) (alerter.Rule, error) {
	// Parse expression
	expr, err := alerter.ParseExpression(cfg.Expr)
	if err != nil {
		return alerter.Rule{}, fmt.Errorf("parsing rule %s: %w", cfg.Name, err)
	}

	var severity internal.Severity
	switch cfg.Severity {
	case "critical":
		severity = internal.SeverityCritical
	case "warning":
		severity = internal.SeverityWarning
	default:
		severity = internal.SeverityInfo
	}

	return expr.ToRule(cfg.Name, severity, cfg.Message), nil
}
