// Package config loads and validates .hive/config.yaml.
package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Team struct {
	Name     string `yaml:"name"`
	RepoURL  string `yaml:"repo_url"`
	RepoPath string `yaml:"repo_path"`
}

type Config struct {
	WorkspaceSlug         string `yaml:"workspace_slug"`
	MaxWorkers            int    `yaml:"max_workers"`
	TickIntervalSeconds   int    `yaml:"tick_interval_seconds"`
	ManagerTimeoutSeconds int    `yaml:"manager_timeout_seconds"`
	Teams                 []Team `yaml:"teams"`
}

// Load reads, parses, validates, and applies defaults to a config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	cfg.applyDefaults()
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.WorkspaceSlug == "" {
		return errors.New("workspace_slug is required")
	}
	for i, t := range c.Teams {
		if t.Name == "" {
			return fmt.Errorf("teams[%d].name is required", i)
		}
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.MaxWorkers == 0 {
		c.MaxWorkers = 3
	}
	if c.TickIntervalSeconds == 0 {
		c.TickIntervalSeconds = 60
	}
	if c.ManagerTimeoutSeconds == 0 {
		c.ManagerTimeoutSeconds = 480
	}
}
