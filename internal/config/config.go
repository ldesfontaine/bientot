// Package config loads the agent configuration from a YAML file.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root of agent.yaml.
type Config struct {
	MachineID    string          `yaml:"machine_id"`
	Dashboard    DashboardConfig `yaml:"dashboard"`
	SigningKey   string          `yaml:"signing_key"`
	PushInterval time.Duration   `yaml:"push_interval"`
	Modules      []ModuleConfig  `yaml:"modules"`
}

// DashboardConfig holds everything needed to talk to the dashboard.
type DashboardConfig struct {
	URL        string `yaml:"url"`
	ServerName string `yaml:"server_name"`
	Cert       string `yaml:"cert"`
	Key        string `yaml:"key"`
	CABundle   string `yaml:"ca_bundle"`
}

// ModuleConfig declares one module in the config file.
// Config is a free-form map whose keys depend on the module type.
type ModuleConfig struct {
	Type    string                 `yaml:"type"`
	Enabled bool                   `yaml:"enabled"`
	Config  map[string]interface{} `yaml:"config"`
}

// Load reads and parses a YAML config file from path.
// Returns an error if the file is missing, malformed, or has missing required fields.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}

	cfg.applyDefaults()

	return &cfg, nil
}

// Validate returns an error if required fields are missing or malformed.
func (c *Config) Validate() error {
	if c.MachineID == "" {
		return fmt.Errorf("machine_id is required")
	}
	if c.Dashboard.URL == "" {
		return fmt.Errorf("dashboard.url is required")
	}
	if c.Dashboard.Cert == "" || c.Dashboard.Key == "" || c.Dashboard.CABundle == "" {
		return fmt.Errorf("dashboard.cert, key and ca_bundle are required")
	}
	if c.SigningKey == "" {
		return fmt.Errorf("signing_key is required")
	}
	if c.PushInterval != 0 && c.PushInterval < time.Second {
		return fmt.Errorf("push_interval must be >= 1s (got %v) — did you forget the unit?", c.PushInterval)
	}

	for i, m := range c.Modules {
		if m.Type == "" {
			return fmt.Errorf("modules[%d]: type is required", i)
		}
	}

	return nil
}

// applyDefaults fills in sane defaults for optional fields.
func (c *Config) applyDefaults() {
	if c.PushInterval == 0 {
		c.PushInterval = 30 * time.Second
	}
	if c.Dashboard.ServerName == "" {
		c.Dashboard.ServerName = "dashboard"
	}
}
