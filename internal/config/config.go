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
	Logs       LogsConfig         `yaml:"logs"`
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

type LogsConfig struct {
	Enabled      bool          `yaml:"enabled"`
	Machine      string        `yaml:"machine"`
	Interval     time.Duration `yaml:"interval"`
	DockerSocket string        `yaml:"docker_socket"`
	CrowdSecURL  string        `yaml:"crowdsec_url"`
}

// AlertsConfig holds alerts.yml content
type AlertsConfig struct {
	Alerts    []AlertRule      `yaml:"alerts"`
	Notifiers []NotifierConfig `yaml:"notifiers"`
}

// EnrichmentConfig holds enrichment section of server config
type EnrichmentConfig struct {
	Enabled    bool                       `yaml:"enabled"`
	GeoIP      GeoIPConfig                `yaml:"geoip"`
	Blocklists BlocklistsConfig           `yaml:"blocklists"`
	Providers  map[string]ProviderConfig  `yaml:"providers"`
}

type GeoIPConfig struct {
	DBPath     string `yaml:"db_path"`
	AutoUpdate bool   `yaml:"auto_update"`
	LicenseKey string `yaml:"license_key"`
}

type BlocklistsConfig struct {
	UpdateInterval string              `yaml:"update_interval"`
	Sources        []BlocklistSource   `yaml:"sources"`
}

type BlocklistSource struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url"`
	Format string `yaml:"format"`
}

type ProviderConfig struct {
	Enabled    bool   `yaml:"enabled"`
	APIKey     string `yaml:"api_key"`
	DailyLimit int    `yaml:"daily_limit"`
	Priority   int    `yaml:"priority"`
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

// VeilleConfig holds veille-secu integration config.
type VeilleConfig struct {
	Enabled        bool     `yaml:"enabled"`
	URL            string   `yaml:"url"`
	Token          string   `yaml:"token"`
	PollInterval   string   `yaml:"poll_interval"`
	SyncTools      bool     `yaml:"sync_tools"`
	SeverityFilter []string `yaml:"severity_filter"`
}

// LoadVeille loads veille-secu config from YAML with env expansion.
func LoadVeille(path string) (*VeilleConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	expanded := os.ExpandEnv(string(data))

	var wrapper struct {
		Veille VeilleConfig `yaml:"veille"`
	}
	if err := yaml.Unmarshal([]byte(expanded), &wrapper); err != nil {
		return nil, err
	}

	return &wrapper.Veille, nil
}

// LoadEnrichment loads enrichment config from YAML with env expansion.
func LoadEnrichment(path string) (*EnrichmentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	expanded := os.ExpandEnv(string(data))

	// Wrap: the enrichment section may be at root or nested
	var wrapper struct {
		Enrichment EnrichmentConfig `yaml:"enrichment"`
	}
	if err := yaml.Unmarshal([]byte(expanded), &wrapper); err != nil {
		return nil, err
	}

	return &wrapper.Enrichment, nil
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
