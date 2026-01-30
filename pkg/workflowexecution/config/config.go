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

// Package config provides configuration types for the WorkflowExecution controller.
//
// Configuration Structure (ADR-030):
//   - ExecutionConfig: PipelineRun execution settings (namespace, service account, cooldown)
//   - BackoffConfig: Exponential backoff settings (DD-WE-004, BR-WE-012)
//   - AuditConfig: Audit trail settings (BR-WE-005, ADR-032)
//   - ControllerConfig: Controller runtime settings (metrics, health probes, leader election)
//
// All configuration values are validated via Config.Validate() before use.
//
// See: docs/architecture/decisions/ADR-030-service-configuration-management.md
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

// Config holds the complete configuration for the WorkflowExecution controller.
type Config struct {
	Execution  ExecutionConfig  `yaml:"execution" validate:"required"`
	Backoff    BackoffConfig    `yaml:"backoff" validate:"required"`
	Audit      AuditConfig      `yaml:"audit" validate:"required"`
	Controller ControllerConfig `yaml:"controller" validate:"required"`
}

// ExecutionConfig holds settings for Tekton PipelineRun execution.
//
// Business Requirements:
// - BR-WE-003: Execution namespace isolation (DD-WE-002)
// - BR-WE-007: Service account configuration
// - DD-WE-001: Cooldown period for resource locking
type ExecutionConfig struct {
	// Namespace where PipelineRuns are created (DD-WE-002)
	Namespace string `yaml:"namespace" validate:"required"`

	// ServiceAccount for PipelineRuns
	ServiceAccount string `yaml:"service_account" validate:"required"`

	// CooldownPeriod prevents redundant sequential workflows (DD-WE-001)
	CooldownPeriod time.Duration `yaml:"cooldown_period" validate:"required,gt=0"`
}

// BackoffConfig holds exponential backoff settings.
//
// Business Requirements:
// - BR-WE-012: Exponential backoff for pre-execution failures
// - DD-WE-004: Exponential backoff strategy
//
// Formula: Cooldown = BaseCooldown * 2^(min(failures-1, MaxExponent))
// Capped at: MaxCooldown
type BackoffConfig struct {
	// BaseCooldown is the initial cooldown for exponential backoff
	BaseCooldown time.Duration `yaml:"base_cooldown" validate:"required,gt=0"`

	// MaxCooldown caps the exponential backoff
	MaxCooldown time.Duration `yaml:"max_cooldown" validate:"required,gt=0,gtefield=BaseCooldown"`

	// MaxExponent limits exponential growth (e.g., 4 means max multiplier is 2^4 = 16x)
	MaxExponent int `yaml:"max_exponent" validate:"required,gte=1,lte=10"`

	// MaxConsecutiveFailures before auto-failing with ExhaustedRetries
	MaxConsecutiveFailures int `yaml:"max_consecutive_failures" validate:"required,gt=0"`
}

// AuditConfig holds settings for audit trail via Data Storage service.
//
// Business Requirements:
// - BR-WE-005: Audit trail for execution lifecycle
// - ADR-032: Mandatory audit for P0 services
// - DD-AUDIT-003: Audit store configuration
type AuditConfig struct {
	// DataStorageURL is the Data Storage Service URL for audit events (ADR-032: MANDATORY)
	DataStorageURL string `yaml:"datastorage_url" validate:"required,url"`

	// Timeout for audit API calls
	Timeout time.Duration `yaml:"timeout" validate:"required,gt=0"`
}

// ControllerConfig holds controller runtime settings.
//
// Standard controller configuration following DD-005 (Observability Standards).
type ControllerConfig struct {
	// MetricsAddr is the address for Prometheus metrics endpoint
	MetricsAddr string `yaml:"metrics_addr" validate:"required"`

	// HealthProbeAddr is the address for health and readiness probes
	HealthProbeAddr string `yaml:"health_probe_addr" validate:"required"`

	// LeaderElection enables leader election for multi-replica deployments
	LeaderElection bool `yaml:"leader_election"`

	// LeaderElectionID is the unique ID for leader election
	LeaderElectionID string `yaml:"leader_election_id" validate:"required"`
}

// DefaultConfig returns the default WorkflowExecution controller configuration.
//
// Defaults align with:
// - DD-WE-001: 5-minute cooldown period
// - DD-WE-002: kubernaut-workflows namespace
// - DD-WE-004: 1-minute base cooldown, 10-minute max, 5 max failures
// - DD-005: :9090 metrics, :8081 health probes
func DefaultConfig() *Config {
	return &Config{
		Execution: ExecutionConfig{
			Namespace:      "kubernaut-workflows",
			ServiceAccount: "kubernaut-workflow-runner",
			CooldownPeriod: 5 * time.Minute,
		},
		Backoff: BackoffConfig{
			BaseCooldown:           60 * time.Second,
			MaxCooldown:            10 * time.Minute,
			MaxExponent:            4,
			MaxConsecutiveFailures: 5,
		},
		Audit: AuditConfig{
			DataStorageURL: "http://data-storage-service:8080", // DD-AUTH-011: Standard service name (with hyphen)
			Timeout:        10 * time.Second,
		},
		Controller: ControllerConfig{
			MetricsAddr:      ":9090",
			HealthProbeAddr:  ":8081",
			LeaderElection:   false,
			LeaderElectionID: "workflowexecution.kubernaut.ai",
		},
	}
}

// LoadFromFile loads configuration from a YAML file.
//
// If the file doesn't exist or can't be parsed, returns error.
// Recommended usage: Load file if exists, otherwise use DefaultConfig().
//
// See: ADR-030 Section 4 (Configuration Loading)
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults for any missing values
	if cfg.Execution.Namespace == "" {
		cfg.Execution.Namespace = "kubernaut-workflows"
	}
	if cfg.Execution.ServiceAccount == "" {
		cfg.Execution.ServiceAccount = "kubernaut-workflow-runner"
	}
	if cfg.Execution.CooldownPeriod == 0 {
		cfg.Execution.CooldownPeriod = 5 * time.Minute
	}
	if cfg.Backoff.BaseCooldown == 0 {
		cfg.Backoff.BaseCooldown = 60 * time.Second
	}
	if cfg.Backoff.MaxCooldown == 0 {
		cfg.Backoff.MaxCooldown = 10 * time.Minute
	}
	if cfg.Backoff.MaxExponent == 0 {
		cfg.Backoff.MaxExponent = 4
	}
	if cfg.Backoff.MaxConsecutiveFailures == 0 {
		cfg.Backoff.MaxConsecutiveFailures = 5
	}
	if cfg.Audit.DataStorageURL == "" {
		cfg.Audit.DataStorageURL = "http://data-storage-service:8080" // DD-AUTH-011: Standard service name (with hyphen)
	}
	if cfg.Audit.Timeout == 0 {
		cfg.Audit.Timeout = 10 * time.Second
	}
	if cfg.Controller.MetricsAddr == "" {
		cfg.Controller.MetricsAddr = ":9090"
	}
	if cfg.Controller.HealthProbeAddr == "" {
		cfg.Controller.HealthProbeAddr = ":8081"
	}
	if cfg.Controller.LeaderElectionID == "" {
		cfg.Controller.LeaderElectionID = "workflowexecution.kubernaut.ai"
	}

	return &cfg, nil
}

// Validate checks if the configuration is valid.
//
// Validation Rules:
// - Execution namespace must not be empty
// - Cooldown periods must be positive
// - Backoff exponent must be reasonable (1-10)
// - Max consecutive failures must be positive
// - Data Storage URL must not be empty
//
// Returns error if validation fails.
// Validate validates the complete configuration using go-playground/validator (ADR-046).
//
// Validation rules enforce business requirements and design decisions via struct tags:
// - BR-WE-003: Execution namespace must be set (`validate:"required"`)
// - BR-WE-007: Service account must be specified (`validate:"required"`)
// - DD-WE-001: Cooldown period must be positive (`validate:"gt=0"`)
// - DD-WE-004: Backoff settings must be valid (`validate:"gte=1,lte=10"`, `gtefield`)
// - BR-WE-005, ADR-032: Audit configuration must be complete (`validate:"required,url"`)
// - DD-005: Controller observability settings must be present (`validate:"required"`)
//
// See: docs/architecture/decisions/ADR-046-struct-validation-standard.md
//
// Returns validator.ValidationErrors if validation fails.
func (c *Config) Validate() error {
	validate := validator.New()
	return validate.Struct(c)
}
