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
//   - (BackoffConfig removed in V1.0 per DD-RO-002 Phase 3)
//   - DataStorageConfig: Data Storage connectivity (BR-WE-005, ADR-030)
//   - ControllerConfig: Controller runtime settings (metrics, health probes, leader election)
//
// All configuration values are validated via Config.Validate() before use.
//
// See: docs/architecture/decisions/ADR-030-service-configuration-management.md
package config

import (
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"

	sharedconfig "github.com/jordigilh/kubernaut/internal/config"
)

// DefaultConfigPath is the standard Kubernetes ConfigMap mount path for this service.
// ADR-030: All services MUST use /etc/{service}/config.yaml as the default.
const DefaultConfigPath = "/etc/workflowexecution/config.yaml"

// Config holds the complete configuration for the WorkflowExecution controller.
// Issue #99: BackoffConfig removed (DD-RO-002 Phase 3 -- RO handles all routing/backoff)
type Config struct {
	Execution   ExecutionConfig              `yaml:"execution" validate:"required"`
	DataStorage sharedconfig.DataStorageConfig `yaml:"datastorage"`
	Controller  ControllerConfig             `yaml:"controller" validate:"required"`
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
	ServiceAccount string `yaml:"serviceAccount" validate:"required"`

	// CooldownPeriod prevents redundant sequential workflows (DD-WE-001)
	CooldownPeriod time.Duration `yaml:"cooldownPeriod" validate:"required,gt=0"`
}

// ControllerConfig holds controller runtime settings.
//
// Standard controller configuration following DD-005 (Observability Standards).
type ControllerConfig struct {
	// MetricsAddr is the address for Prometheus metrics endpoint
	MetricsAddr string `yaml:"metricsAddr" validate:"required"`

	// HealthProbeAddr is the address for health and readiness probes
	HealthProbeAddr string `yaml:"healthProbeAddr" validate:"required"`

	// LeaderElection enables leader election for multi-replica deployments
	LeaderElection bool `yaml:"leaderElection"`

	// LeaderElectionID is the unique ID for leader election
	LeaderElectionID string `yaml:"leaderElectionId" validate:"required"`
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
		DataStorage: sharedconfig.DefaultDataStorageConfig(),
		Controller: ControllerConfig{
			MetricsAddr:      ":9090",
			HealthProbeAddr:  ":8081",
			LeaderElection:   false,
			LeaderElectionID: "workflowexecution.kubernaut.ai",
		},
	}
}

// LoadFromFile loads configuration from a YAML file with defaults.
// ADR-030: Service Configuration Management pattern.
// Graceful degradation: Falls back to defaults if file not found or invalid.
func LoadFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, nil
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return cfg, nil
	}

	return cfg, nil
}

// Validate checks if the configuration is valid.
//
// Validation Rules:
// - Execution namespace must not be empty
// - Cooldown periods must be positive
// - (Backoff validation removed in V1.0 per DD-RO-002 Phase 3)
// - Data Storage URL must not be empty
//
// Returns error if validation fails.
// Validate validates the complete configuration using go-playground/validator (ADR-046).
//
// Validation rules enforce business requirements and design decisions via struct tags:
// - BR-WE-003: Execution namespace must be set (`validate:"required"`)
// - BR-WE-007: Service account must be specified (`validate:"required"`)
// - DD-WE-001: Cooldown period must be positive (`validate:"gt=0"`)
// - (DD-WE-004: Backoff validation removed in V1.0 per DD-RO-002 Phase 3)
// - BR-WE-005, ADR-032: Audit configuration must be complete (`validate:"required,url"`)
// - DD-005: Controller observability settings must be present (`validate:"required"`)
//
// See: docs/architecture/decisions/ADR-046-struct-validation-standard.md
//
// Returns validator.ValidationErrors if validation fails.
func (c *Config) Validate() error {
	validate := validator.New()
	if err := validate.Struct(c); err != nil {
		return err
	}
	// DataStorage uses shared validation (ADR-030)
	return sharedconfig.ValidateDataStorageConfig(&c.DataStorage)
}
