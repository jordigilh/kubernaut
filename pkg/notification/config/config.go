/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ========================================
// NOTIFICATION SERVICE CONFIGURATION (ADR-030)
// 📋 Pattern: DataStorage/Gateway config pattern
// Authority: config/notification.yaml (source of truth)
// Reference: pkg/datastorage/config/config.go
// ========================================
//
// ADR-030 Configuration Management:
// 1. Load from YAML file (ConfigMap in Kubernetes)
// 2. Override with environment variables (secrets only)
// 3. Validate configuration before startup
//
// Configuration Hierarchy (highest to lowest priority):
// 1. Environment variables (secrets only - SLACK_WEBHOOK_URL)
// 2. YAML file (everything else)
// 3. Defaults (fallback values)
// ========================================

// Config is the top-level configuration for Notification service.
// Organized by Single Responsibility Principle for better maintainability.
type Config struct {
	// Controller configuration (leader election, metrics, health probes)
	Controller ControllerSettings `yaml:"controller"`

	// Delivery channel configuration
	Delivery DeliverySettings `yaml:"delivery"`

	// Infrastructure dependencies (audit, external services)
	Infrastructure InfrastructureSettings `yaml:"infrastructure"`
}

// ControllerSettings contains Kubernetes controller configuration.
// Single Responsibility: Controller runtime behavior
type ControllerSettings struct {
	MetricsAddr      string `yaml:"metricsAddr"`      // Default: ":9090"
	HealthProbeAddr  string `yaml:"healthProbeAddr"`  // Default: ":8081"
	LeaderElection   bool   `yaml:"leaderElection"`   // Default: false
	LeaderElectionID string `yaml:"leaderElectionId"` // Default: "notification.kubernaut.ai"
}

// DeliverySettings contains delivery channel configuration.
// Single Responsibility: Notification delivery behavior
type DeliverySettings struct {
	Console ConsoleSettings `yaml:"console"`
	File    FileSettings    `yaml:"file"`
	Log     LogSettings     `yaml:"log"`
	Slack   SlackSettings   `yaml:"slack"`
}

// ConsoleSettings contains console delivery configuration.
type ConsoleSettings struct {
	Enabled bool `yaml:"enabled"` // Default: true
}

// FileSettings contains file delivery configuration.
// DD-NOT-006: File delivery for audit trails and E2E testing
type FileSettings struct {
	OutputDir string        `yaml:"outputDir"` // Required when ChannelFile used
	Format    string        `yaml:"format"`    // Default: "json"
	Timeout   time.Duration `yaml:"timeout"`   // Default: 5s
}

// LogSettings contains structured log delivery configuration.
// DD-NOT-006: Structured log delivery (JSON Lines to stdout)
type LogSettings struct {
	Enabled bool   `yaml:"enabled"` // Default: false
	Format  string `yaml:"format"`  // Default: "json"
}

// SlackSettings contains Slack webhook configuration.
type SlackSettings struct {
	WebhookURL string        `yaml:"webhookUrl"` // From env or config (secret)
	Timeout    time.Duration `yaml:"timeout"`    // Default: 10s
}

// InfrastructureSettings contains external dependency configuration.
// Single Responsibility: Infrastructure connections
type InfrastructureSettings struct {
	// ADR-032: Data Storage URL for audit event emission (MANDATORY for P0 services)
	// Example: "http://data-storage-service.kubernaut-system.svc.cluster.local:8080"
	DataStorageURL string `yaml:"dataStorageUrl"`
}

// LoadFromFile loads configuration from YAML file.
// This is the primary configuration source (ADR-030).
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Apply defaults for missing values
	cfg.applyDefaults()

	return &cfg, nil
}

// LoadFromEnv overrides configuration with environment variables.
// ADR-030: Only secrets should come from environment variables, never functional config.
func (c *Config) LoadFromEnv() {
	// Secrets only (NEVER in ConfigMap)
	if webhookURL := os.Getenv("SLACK_WEBHOOK_URL"); webhookURL != "" {
		c.Delivery.Slack.WebhookURL = webhookURL
	}
}

// Validate validates configuration.
// ADR-030: Fail fast on invalid configuration.
func (c *Config) Validate() error {
	// ADR-032: Data Storage URL is MANDATORY for audit
	if c.Infrastructure.DataStorageURL == "" {
		return fmt.Errorf("infrastructure.data_storage_url is required (ADR-032)")
	}

	// Controller validation
	if c.Controller.MetricsAddr == "" {
		return fmt.Errorf("controller.metrics_addr cannot be empty")
	}
	if c.Controller.HealthProbeAddr == "" {
		return fmt.Errorf("controller.health_probe_addr cannot be empty")
	}
	if c.Controller.LeaderElectionID == "" {
		return fmt.Errorf("controller.leader_election_id cannot be empty")
	}

	return nil
}

// applyDefaults sets default values for missing configuration.
func (c *Config) applyDefaults() {
	// Controller defaults
	if c.Controller.MetricsAddr == "" {
		c.Controller.MetricsAddr = ":9090"
	}
	if c.Controller.HealthProbeAddr == "" {
		c.Controller.HealthProbeAddr = ":8081"
	}
	if c.Controller.LeaderElectionID == "" {
		c.Controller.LeaderElectionID = "notification.kubernaut.ai"
	}

	// Delivery defaults
	// Console enabled by default (for local development)
	if !c.Delivery.Console.Enabled {
		c.Delivery.Console.Enabled = true
	}

	// File format default
	if c.Delivery.File.Format == "" {
		c.Delivery.File.Format = "json"
	}
	if c.Delivery.File.Timeout == 0 {
		c.Delivery.File.Timeout = 5 * time.Second
	}

	// Log format default
	if c.Delivery.Log.Format == "" {
		c.Delivery.Log.Format = "json"
	}

	// Slack timeout default
	if c.Delivery.Slack.Timeout == 0 {
		c.Delivery.Slack.Timeout = 10 * time.Second
	}
}

// DefaultConfig returns default configuration.
// Used for local development and testing when config file not provided.
func DefaultConfig() *Config {
	cfg := &Config{
		Controller: ControllerSettings{
			MetricsAddr:      ":9090",
			HealthProbeAddr:  ":8081",
			LeaderElection:   false,
			LeaderElectionID: "notification.kubernaut.ai",
		},
		Delivery: DeliverySettings{
			Console: ConsoleSettings{Enabled: true},
			File: FileSettings{
				Format:  "json",
				Timeout: 5 * time.Second,
			},
			Log: LogSettings{
				Enabled: false,
				Format:  "json",
			},
			Slack: SlackSettings{
				Timeout: 10 * time.Second,
			},
		},
		Infrastructure: InfrastructureSettings{
			// Will be set from config file or env
			DataStorageURL: "",
		},
	}

	return cfg
}

