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

package aianalysis

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	sharedconfig "github.com/jordigilh/kubernaut/internal/config"
)

// DefaultConfigPath is the standard Kubernetes ConfigMap mount path for this service.
// ADR-030: All services MUST use /etc/{service}/config.yaml as the default.
const DefaultConfigPath = "/etc/aianalysis/config.yaml"

// Config represents the complete AIAnalysis controller configuration.
// ADR-030: Service Configuration Management
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type Config struct {
	// Controller runtime configuration (DD-005)
	Controller sharedconfig.ControllerConfig `yaml:"controller"`

	// DataStorage connectivity (ADR-030: audit trail + workflow catalog)
	DataStorage sharedconfig.DataStorageConfig `yaml:"datastorage"`

	// Rego policy evaluation configuration (BR-AI-012)
	Rego RegoConfig `yaml:"rego"`

	// Logging configuration (Issue #875: config-file-only log level with hot-reload)
	Logging sharedconfig.LoggingConfig `yaml:"logging"`

	// TLSProfile selects the TLS security profile (Old/Intermediate/Modern).
	// Issue #748: OCP-only — set by kubernaut-operator from the cluster APIServer CR.
	TLSProfile string `yaml:"tlsProfile,omitempty"`

	// MaxConcurrentReconciles limits the number of concurrent AIAnalysis
	// reconciliations (controller-runtime's WithOptions, wired via
	// AIAnalysisReconciler.SetupWithManager). #2204 RCA (2026-08-20):
	// cmd/aianalysis/main.go previously called SetupWithManager(mgr) with no
	// explicit worker count, so controller-runtime's implicit default of 1
	// serialized every AIAnalysis reconcile in production/E2E even though
	// KA dispatches the underlying investigations concurrently -- the same
	// bottleneck the integration envtest suite already worked around
	// (test/integration/aianalysis/suite_test.go hardcodes 10), but this is
	// the first time it's wired into the actual production entry point.
	// Mirrors effectivenessmonitor's AssessmentConfig.MaxConcurrentReconciles
	// (ADR-EM-001 §10) precedent. Default: 10. Range: [1, ∞).
	MaxConcurrentReconciles int `yaml:"maxConcurrentReconciles"`

	// Debug holds developer/operator diagnostic toggles (BR-PLATFORM-012,
	// Issue #2275). Defaults to profiling OFF (AC-6).
	Debug sharedconfig.DebugConfig `yaml:"debug"`
}

// #2204 (2026-08-20): AgentConfig (agent.url / agent.timeout /
// agent.sessionPollInterval) removed. It configured an HTTP client to
// Kubernaut Agent (base URL + timeout) and a fixed poll cadence -- both
// vestiges of the pre-AgentSession-CRD design retired by DD-AA-KA-001. AA's
// actual KA channel is creator.AgentSessionCreator talking to the K8s API
// server (watched AgentSession CRDs), which needs neither a base URL/HTTP
// timeout nor a poll interval; nothing in cmd/aianalysis or pkg/aianalysis
// read any of these three fields.

// RegoConfig defines Rego policy evaluation configuration.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type RegoConfig struct {
	// PolicyPath is the file path to the Rego approval policy.
	PolicyPath string `yaml:"policyPath"`

	// ConfidenceThreshold is the operator-configurable auto-approval confidence threshold (#225).
	// When set, passed as input.confidence_threshold to the Rego policy, overriding the
	// policy's built-in default (0.8). Must be in range (0.0, 1.0].
	// nil means "use the Rego policy's built-in default".
	// Stepping stone toward BR-AI-088 (V1.1 rule-based thresholds).
	ConfidenceThreshold *float64 `yaml:"confidenceThreshold,omitempty"`

	// LowConfidenceFloor is the operator-configurable floor for auto-proceeding with a
	// KA-selected workflow (BR-AI-088.4, Issue #1828). During the Investigating phase,
	// when selected_workflow.confidence is below this floor, the ResponseProcessor sets
	// NeedsHumanReview=true (HumanReviewReasonLowConfidence) instead of proceeding to the
	// Analyzing phase. This is a distinct, earlier gate from ConfidenceThreshold above
	// (which tunes Rego's later auto-approval decision, BR-AI-003/#225) — see BR-AI-088.4's
	// note distinguishing the two. Must be in range (0.0, 1.0] when set.
	// nil means "use the built-in 70% floor" (V1.0 global default).
	LowConfidenceFloor *float64 `yaml:"lowConfidenceFloor,omitempty"`
}

// DefaultConfig returns safe defaults for the AIAnalysis controller.
// DD-AUDIT-004: AA-specific buffer defaults (LOW tier: 20K buffer, 1K batch)
// override the shared DefaultDataStorageConfig() values.
func DefaultConfig() *Config {
	ds := sharedconfig.DefaultDataStorageConfig()
	ds.Buffer.BufferSize = 20000
	ds.Buffer.BatchSize = 1000

	return &Config{
		Controller: sharedconfig.ControllerConfig{
			MetricsAddr:      ":9090",
			HealthProbeAddr:  ":8081",
			LeaderElection:   false,
			LeaderElectionID: "aianalysis.kubernaut.ai",
		},
		DataStorage: ds,
		Rego: RegoConfig{
			PolicyPath: "/etc/aianalysis/policies/approval.rego",
		},
		Logging:                 sharedconfig.DefaultLoggingConfig(),
		MaxConcurrentReconciles: 10,
		Debug:                   sharedconfig.DefaultDebugConfig(),
	}
}

// LoadFromFile loads AIAnalysis configuration from YAML file with defaults.
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

// Validate checks AIAnalysis configuration for common issues.
func (c *Config) Validate() error {
	// Issue #875: Logging validation
	if err := c.Logging.Validate(); err != nil {
		return err
	}

	// Validate controller config
	if c.Controller.MetricsAddr == "" {
		return fmt.Errorf("controller.metricsAddr is required")
	}
	if c.Controller.HealthProbeAddr == "" {
		return fmt.Errorf("controller.healthProbeAddr is required")
	}

	// Validate DataStorage config (ADR-030)
	if err := sharedconfig.ValidateDataStorageConfig(&c.DataStorage); err != nil {
		return err
	}

	if c.MaxConcurrentReconciles < 1 {
		return fmt.Errorf("maxConcurrentReconciles must be at least 1, got %d", c.MaxConcurrentReconciles)
	}

	return c.Rego.Validate()
}

// Validate checks the Rego policy configuration.
// Extracted from Config.Validate (gocyclo remediation) so each of the two
// operator-configurable confidence knobs (ConfidenceThreshold, #225;
// LowConfidenceFloor, BR-AI-088.4/#1828) adds one call site here instead of
// two more branches to the top-level function's cyclomatic complexity.
func (r *RegoConfig) Validate() error {
	if r.PolicyPath == "" {
		return fmt.Errorf("rego.policyPath is required")
	}
	if err := validateUnitInterval(r.ConfidenceThreshold, "rego.confidenceThreshold"); err != nil {
		return err
	}
	return validateUnitInterval(r.LowConfidenceFloor, "rego.lowConfidenceFloor")
}

// validateUnitInterval checks that an optional confidence-style field, when
// set, falls in range (0.0, 1.0]. A nil value (operator did not override the
// field) is always valid.
func validateUnitInterval(v *float64, fieldName string) error {
	if v == nil {
		return nil
	}
	if *v <= 0 || *v > 1.0 {
		return fmt.Errorf("%s must be in range (0.0, 1.0], got %v", fieldName, *v)
	}
	return nil
}
