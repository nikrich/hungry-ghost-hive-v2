// Package config loads and validates .hive/config.yaml.
package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Team struct {
	Name        string `yaml:"name"`
	RepoURL     string `yaml:"repo_url"`
	RepoPath    string `yaml:"repo_path"`
	TestCommand string `yaml:"test_command"` // Phase 2.C — QA runs this; default "go test ./..."
}

type Config struct {
	WorkspaceSlug         string  `yaml:"workspace_slug"`
	MaxWorkers            int     `yaml:"max_workers"`
	MaxQA                 int     `yaml:"max_qa"` // Phase 2.C — concurrent QA slots
	TickIntervalSeconds   int     `yaml:"tick_interval_seconds"`
	ManagerTimeoutSeconds int     `yaml:"manager_timeout_seconds"`
	IdleBackoffMaxSeconds int     `yaml:"idle_backoff_max_seconds"` // cap for exponential backoff on consecutive idle ticks; set ≤ tick_interval_seconds to disable
	IdleBackoffFactor     float64 `yaml:"idle_backoff_factor"`      // multiplier per consecutive idle tick
	// Per-tier model selection. Manager reads these from config.yaml and passes
	// them to `claude --print --model <id>` when spawning workers, so trivial
	// 1-2pt stories run on Haiku instead of the operator's default Opus/Sonnet.
	ModelForJunior       string `yaml:"model_for_junior"`
	ModelForIntermediate string `yaml:"model_for_intermediate"`
	ModelForSenior       string `yaml:"model_for_senior"`
	Teams                []Team `yaml:"teams"`
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
	if c.MaxQA == 0 {
		c.MaxQA = 2
	}
	if c.TickIntervalSeconds == 0 {
		c.TickIntervalSeconds = 60
	}
	if c.ManagerTimeoutSeconds == 0 {
		c.ManagerTimeoutSeconds = 480
	}
	if c.IdleBackoffMaxSeconds == 0 {
		c.IdleBackoffMaxSeconds = 600
	}
	if c.IdleBackoffFactor == 0 {
		c.IdleBackoffFactor = 2.0
	}
	if c.ModelForJunior == "" {
		c.ModelForJunior = "claude-haiku-4-5-20251001"
	}
	if c.ModelForIntermediate == "" {
		c.ModelForIntermediate = "claude-sonnet-4-6"
	}
	if c.ModelForSenior == "" {
		c.ModelForSenior = "claude-opus-4-7"
	}
	for i := range c.Teams {
		if c.Teams[i].TestCommand == "" {
			c.Teams[i].TestCommand = "go test ./..."
		}
	}
}
