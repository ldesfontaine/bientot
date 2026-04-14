package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ldesfontaine/bientot/internal"
	"github.com/ldesfontaine/bientot/internal/alerter"
)

// Config contient la configuration de l'application
type Config struct {
	Targets TargetsConfig
	Alerts  AlertsConfig
}

// TargetsConfig contient le contenu de targets.yml
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

// AlertsConfig contient le contenu de alerts.yml
type AlertsConfig struct {
	Alerts    []AlertRule      `yaml:"alerts"`
	Notifiers []NotifierConfig `yaml:"notifiers"`
}

// EnrichmentConfig contient la section enrichissement de la config serveur
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
	Type           string            `yaml:"type"`
	URL            string            `yaml:"url"`
	Topic          string            `yaml:"topic"`
	Token          string            `yaml:"token,omitempty"`
	SeverityFilter []string          `yaml:"severity_filter"`
	Headers        map[string]string `yaml:"headers,omitempty"`
}

// VeilleConfig contient la configuration d'intégration veille-secu.
type VeilleConfig struct {
	Enabled        bool     `yaml:"enabled"`
	URL            string   `yaml:"url"`
	Token          string   `yaml:"token"`
	PollInterval   string   `yaml:"poll_interval"`
	SyncTools      bool     `yaml:"sync_tools"`
	SeverityFilter []string `yaml:"severity_filter"`
}

// LoadVeille charge la config veille-secu depuis le YAML avec expansion des variables d'environnement.
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

	// La variable d'environnement VEILLE_ENABLED surcharge le champ enabled du YAML
	if v := os.Getenv("VEILLE_ENABLED"); v != "" {
		wrapper.Veille.Enabled = v == "true" || v == "1"
	}

	return &wrapper.Veille, nil
}

// LoadEnrichment charge la config d'enrichissement depuis le YAML avec expansion des variables d'environnement.
func LoadEnrichment(path string) (*EnrichmentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	expanded := os.ExpandEnv(string(data))

	// La section enrichissement peut être à la racine ou imbriquée
	var wrapper struct {
		Enrichment EnrichmentConfig `yaml:"enrichment"`
	}
	if err := yaml.Unmarshal([]byte(expanded), &wrapper); err != nil {
		return nil, err
	}

	return &wrapper.Enrichment, nil
}

// LoadTargets charge targets.yml avec expansion des variables d'environnement
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

// LoadAlerts charge alerts.yml
func LoadAlerts(path string) (*AlertsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Expansion des variables d'environnement
	expanded := os.ExpandEnv(string(data))

	var config AlertsConfig
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// ParseRules convertit les configs AlertRule en alerter.Rule
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
	// Analyse de l'expression
	expr, err := alerter.ParseExpression(cfg.Expr)
	if err != nil {
		return alerter.Rule{}, fmt.Errorf("analyse de la règle %s: %w", cfg.Name, err)
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
