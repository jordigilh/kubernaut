/*
Copyright 2026 Jordi Gil.

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

package remediationorchestrator

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	sharedconfig "github.com/jordigilh/kubernaut/internal/config"
)

// DefaultConfigPath is the standard Kubernetes ConfigMap mount path for this service.
// ADR-030: All services MUST use /etc/{service}/config.yaml as the default.
const DefaultConfigPath = "/etc/remediationorchestrator/config.yaml"

// Config represents the complete RemediationOrchestrator configuration.
// ADR-030: Service Configuration Management
type Config struct {
	// Controller runtime configuration (DD-005)
	Controller sharedconfig.ControllerConfig `yaml:"controller"`

	// Timeouts for remediation workflow phases (BR-ORCH-027, BR-ORCH-028)
	Timeouts TimeoutsConfig `yaml:"timeouts"`

	// DataStorage connectivity (ADR-030: audit trail + workflow catalog)
	DataStorage sharedconfig.DataStorageConfig `yaml:"datastorage"`

	// EffectivenessAssessment configuration (ADR-EM-001)
	// Controls how the RO creates EffectivenessAssessment CRDs on remediation completion.
	// The RO only sets StabilizationWindow; all other assessment parameters
	// (PrometheusEnabled, AlertManagerEnabled, ValidityWindow) are EM-internal config.
	EA EACreationConfig `yaml:"effectivenessAssessment"`
}

// TimeoutsConfig holds timeout configuration for remediation workflow phases.
// BR-ORCH-027: Global timeout for entire remediation workflow.
// BR-ORCH-028: Per-phase timeouts for SignalProcessing, AIAnalysis, WorkflowExecution.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase.
type TimeoutsConfig struct {
	// Global is the maximum duration for the entire remediation workflow.
	// BR-ORCH-027, AC-027-3. Default: 1h.
	Global time.Duration `yaml:"global"`

	// Processing is the timeout for the SignalProcessing phase.
	// BR-ORCH-028, AC-028-1. Default: 5m.
	Processing time.Duration `yaml:"processing"`

	// Analyzing is the timeout for the AIAnalysis phase.
	// BR-ORCH-028, AC-028-1. Default: 10m.
	Analyzing time.Duration `yaml:"analyzing"`

	// Executing is the timeout for the WorkflowExecution phase.
	// BR-ORCH-028, AC-028-1. Default: 30m.
	Executing time.Duration `yaml:"executing"`

	// AwaitingApproval is the timeout for the AwaitingApproval phase.
	// ADR-040: Maximum duration before an unanswered approval request expires.
	// Default: 15m.
	AwaitingApproval time.Duration `yaml:"awaitingApproval"`
}

// EACreationConfig controls EffectivenessAssessment CRD creation by the RO.
// ADR-EM-001: RO creates EA CRD when RR reaches terminal phase (Completed, Failed, TimedOut).
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type EACreationConfig struct {
	// StabilizationWindow is the duration the EM should wait after remediation
	// before starting assessment checks. Set in the EA spec by the RO.
	// Default: 5m (ADR-EM-001 Section 8). Range: [1s, 1h].
	StabilizationWindow time.Duration `yaml:"stabilizationWindow"`
}

// DefaultConfig returns safe defaults for the RemediationOrchestrator.
func DefaultConfig() *Config {
	return &Config{
		DataStorage: sharedconfig.DefaultDataStorageConfig(),
		Controller: sharedconfig.ControllerConfig{
			MetricsAddr:      ":9090",
			HealthProbeAddr:  ":8081",
			LeaderElection:   false,
			LeaderElectionID: "remediationorchestrator.kubernaut.ai",
		},
		Timeouts: TimeoutsConfig{
			Global:           1 * time.Hour,
			Processing:       5 * time.Minute,
			Analyzing:        10 * time.Minute,
			Executing:        30 * time.Minute,
			AwaitingApproval: 15 * time.Minute,
		},
		EA: EACreationConfig{
			StabilizationWindow: 5 * time.Minute,
		},
	}
}

// LoadFromFile loads configuration from YAML file with defaults.
// ADR-030: Service Configuration Management pattern.
// Graceful degradation: Falls back to defaults if file not found or invalid.
func LoadFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks configuration for common issues.
func (c *Config) Validate() error {
	// Validate DataStorage config (ADR-030)
	if err := sharedconfig.ValidateDataStorageConfig(&c.DataStorage); err != nil {
		return err
	}

	// Validate controller config
	if c.Controller.MetricsAddr == "" {
		return fmt.Errorf("controller.metricsAddr is required")
	}
	if c.Controller.HealthProbeAddr == "" {
		return fmt.Errorf("controller.healthProbeAddr is required")
	}

	// Validate timeouts (BR-ORCH-027, BR-ORCH-028)
	if c.Timeouts.Global <= 0 {
		return fmt.Errorf("timeouts.global must be positive, got %v", c.Timeouts.Global)
	}
	if c.Timeouts.Processing <= 0 {
		return fmt.Errorf("timeouts.processing must be positive, got %v", c.Timeouts.Processing)
	}
	if c.Timeouts.Analyzing <= 0 {
		return fmt.Errorf("timeouts.analyzing must be positive, got %v", c.Timeouts.Analyzing)
	}
	if c.Timeouts.Executing <= 0 {
		return fmt.Errorf("timeouts.executing must be positive, got %v", c.Timeouts.Executing)
	}
	if c.Timeouts.AwaitingApproval <= 0 {
		return fmt.Errorf("timeouts.awaitingApproval must be positive, got %v", c.Timeouts.AwaitingApproval)
	}
	phaseSum := c.Timeouts.Processing + c.Timeouts.Analyzing + c.Timeouts.AwaitingApproval + c.Timeouts.Executing
	if c.Timeouts.Global < phaseSum {
		return fmt.Errorf("timeouts.global (%v) must be >= sum of phase timeouts (%v)", c.Timeouts.Global, phaseSum)
	}

	// Validate EA creation config (ADR-EM-001)
	if c.EA.StabilizationWindow < 1*time.Second {
		return fmt.Errorf("effectivenessAssessment.stabilizationWindow must be at least 1s, got %v", c.EA.StabilizationWindow)
	}
	if c.EA.StabilizationWindow > 1*time.Hour {
		return fmt.Errorf("effectivenessAssessment.stabilizationWindow must not exceed 1h, got %v", c.EA.StabilizationWindow)
	}

	return nil
}
