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

	// Routing engine configuration (DD-RO-002, BR-ORCH-042, DD-WE-004, Issue #214)
	// Controls blocking thresholds, cooldowns, and backoff for the routing engine.
	// Falls back to DefaultConfig() defaults when omitted from YAML.
	Routing RoutingConfig `yaml:"routing"`

	// AsyncPropagation configures delays for async-managed targets (GitOps, operator CRDs).
	// The RO uses these values to compute HashComputeAfter when creating EA CRDs.
	// DD-EM-004 v2.0, BR-RO-103.3, BR-RO-103.4, Issue #253
	AsyncPropagation AsyncPropagationConfig `yaml:"asyncPropagation"`
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

// RoutingConfig holds configuration for the routing engine's blocking decisions.
// DD-RO-002: Centralized Routing Responsibility.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase.
type RoutingConfig struct {
	// ConsecutiveFailureThreshold is the number of consecutive Failed/Blocked RRs
	// for the same fingerprint before blocking new RRs.
	// BR-ORCH-042. Default: 3.
	ConsecutiveFailureThreshold int `yaml:"consecutiveFailureThreshold"`

	// ConsecutiveFailureCooldown is how long to block after hitting the threshold.
	// BR-ORCH-042. Default: 1h.
	ConsecutiveFailureCooldown time.Duration `yaml:"consecutiveFailureCooldown"`

	// RecentlyRemediatedCooldown is the minimum interval between successful
	// remediations on the same target+workflow.
	// DD-WE-001. Default: 5m.
	RecentlyRemediatedCooldown time.Duration `yaml:"recentlyRemediatedCooldown"`

	// ExponentialBackoffBase is the base cooldown for exponential backoff.
	// Formula: min(Base * 2^(failures-1), Max).
	// DD-WE-004. Default: 1m.
	ExponentialBackoffBase time.Duration `yaml:"exponentialBackoffBase"`

	// ExponentialBackoffMax is the maximum cooldown for exponential backoff.
	// DD-WE-004. Default: 10m.
	ExponentialBackoffMax time.Duration `yaml:"exponentialBackoffMax"`

	// ExponentialBackoffMaxExponent caps the exponential calculation (2^N multiplier).
	// DD-WE-004. Default: 4.
	ExponentialBackoffMaxExponent int `yaml:"exponentialBackoffMaxExponent"`

	// ScopeBackoffBase is the initial backoff for unmanaged resource blocking.
	// ADR-053, BR-SCOPE-010. Default: 5s.
	ScopeBackoffBase time.Duration `yaml:"scopeBackoffBase"`

	// ScopeBackoffMax is the maximum backoff for unmanaged resource blocking.
	// ADR-053, BR-SCOPE-010. Default: 5m.
	ScopeBackoffMax time.Duration `yaml:"scopeBackoffMax"`

	// IneffectiveChainThreshold is the number of consecutive ineffective remediations
	// (hash chain match or spec_drift) required to trigger escalation.
	// Issue #214, BR-ORCH-042.5. Default: 3.
	IneffectiveChainThreshold int `yaml:"ineffectiveChainThreshold"`

	// RecurrenceCountThreshold is the total number of remediation entries within
	// the time window required to trigger the safety-net escalation (Layer 3).
	// Issue #214, BR-ORCH-042.5. Default: 5.
	RecurrenceCountThreshold int `yaml:"recurrenceCountThreshold"`

	// IneffectiveTimeWindow is the lookback window for both hash chain and safety net.
	// Issue #214, BR-ORCH-042.5. Default: 4h.
	IneffectiveTimeWindow time.Duration `yaml:"ineffectiveTimeWindow"`
}

// AsyncPropagationConfig configures delays for async-managed remediation targets.
// These delays account for the time between a remediation completing and the
// actual spec change appearing on the target resource.
// DD-EM-004 v2.0, BR-RO-103.3, BR-RO-103.4, Issue #253, Issue #277
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase.
type AsyncPropagationConfig struct {
	// GitOpsSyncDelay is the expected time for a GitOps tool (ArgoCD/Flux)
	// to sync a change to the cluster. Default: 3m. Range: [0, 30m].
	// Zero disables this stage (for environments with instant sync).
	GitOpsSyncDelay time.Duration `yaml:"gitOpsSyncDelay"`

	// OperatorReconcileDelay is the expected time for a Kubernetes operator
	// to reconcile after its CR is updated. Default: 1m. Range: [0, 30m].
	// Zero disables this stage.
	OperatorReconcileDelay time.Duration `yaml:"operatorReconcileDelay"`

	// ProactiveAlertDelay is the additional delay for alert resolution checks
	// on proactive (predictive) signals. Default: 5m.
	// Applied by the RO when ai.Spec.AnalysisRequest.SignalMode == "proactive",
	// causing the EM to defer AlertManager checks by this duration beyond
	// the StabilizationWindow.
	// Reference: Issue #277, BR-EM-009
	ProactiveAlertDelay time.Duration `yaml:"proactiveAlertDelay"`
}

// ComputePropagationDelay returns the total propagation delay for a given target
// based on detection results. The delays compound additively when both flags are true.
// Returns 0 for sync targets (neither GitOps nor CRD).
// DD-EM-004 v2.0, BR-RO-103.5
func (a *AsyncPropagationConfig) ComputePropagationDelay(isGitOps, isCRD bool) time.Duration {
	var total time.Duration
	if isGitOps {
		total += a.GitOpsSyncDelay
	}
	if isCRD {
		total += a.OperatorReconcileDelay
	}
	return total
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
		AsyncPropagation: AsyncPropagationConfig{
			GitOpsSyncDelay:        3 * time.Minute,
			OperatorReconcileDelay: 1 * time.Minute,
			ProactiveAlertDelay:    5 * time.Minute,
		},
		Routing: RoutingConfig{
			ConsecutiveFailureThreshold:  3,
			ConsecutiveFailureCooldown:   1 * time.Hour,
			RecentlyRemediatedCooldown:   5 * time.Minute,
			ExponentialBackoffBase:       1 * time.Minute,
			ExponentialBackoffMax:        10 * time.Minute,
			ExponentialBackoffMaxExponent: 4,
			ScopeBackoffBase:            5 * time.Second,
			ScopeBackoffMax:             5 * time.Minute,
			IneffectiveChainThreshold:   3,
			RecurrenceCountThreshold:    5,
			IneffectiveTimeWindow:       4 * time.Hour,
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

	// Validate async propagation config (DD-EM-004 v2.0, BR-RO-103.4, Issue #253)
	if c.AsyncPropagation.GitOpsSyncDelay < 0 {
		return fmt.Errorf("asyncPropagation.gitOpsSyncDelay must be >= 0, got %v", c.AsyncPropagation.GitOpsSyncDelay)
	}
	if c.AsyncPropagation.OperatorReconcileDelay < 0 {
		return fmt.Errorf("asyncPropagation.operatorReconcileDelay must be >= 0, got %v", c.AsyncPropagation.OperatorReconcileDelay)
	}
	if c.AsyncPropagation.ProactiveAlertDelay < 0 {
		return fmt.Errorf("asyncPropagation.proactiveAlertDelay must be >= 0, got %v", c.AsyncPropagation.ProactiveAlertDelay)
	}

	// Validate routing config (DD-RO-002, BR-ORCH-042, DD-WE-004, Issue #214)
	if c.Routing.ConsecutiveFailureThreshold < 1 {
		return fmt.Errorf("routing.consecutiveFailureThreshold must be >= 1, got %d", c.Routing.ConsecutiveFailureThreshold)
	}
	if c.Routing.ConsecutiveFailureCooldown <= 0 {
		return fmt.Errorf("routing.consecutiveFailureCooldown must be positive, got %v", c.Routing.ConsecutiveFailureCooldown)
	}
	if c.Routing.RecentlyRemediatedCooldown <= 0 {
		return fmt.Errorf("routing.recentlyRemediatedCooldown must be positive, got %v", c.Routing.RecentlyRemediatedCooldown)
	}
	if c.Routing.ExponentialBackoffBase <= 0 {
		return fmt.Errorf("routing.exponentialBackoffBase must be positive, got %v", c.Routing.ExponentialBackoffBase)
	}
	if c.Routing.ExponentialBackoffMax <= 0 {
		return fmt.Errorf("routing.exponentialBackoffMax must be positive, got %v", c.Routing.ExponentialBackoffMax)
	}
	if c.Routing.ExponentialBackoffMax < c.Routing.ExponentialBackoffBase {
		return fmt.Errorf("routing.exponentialBackoffMax (%v) must be >= exponentialBackoffBase (%v)", c.Routing.ExponentialBackoffMax, c.Routing.ExponentialBackoffBase)
	}
	if c.Routing.ExponentialBackoffMaxExponent < 1 {
		return fmt.Errorf("routing.exponentialBackoffMaxExponent must be >= 1, got %d", c.Routing.ExponentialBackoffMaxExponent)
	}
	if c.Routing.ScopeBackoffBase <= 0 {
		return fmt.Errorf("routing.scopeBackoffBase must be positive, got %v", c.Routing.ScopeBackoffBase)
	}
	if c.Routing.ScopeBackoffMax <= 0 {
		return fmt.Errorf("routing.scopeBackoffMax must be positive, got %v", c.Routing.ScopeBackoffMax)
	}
	if c.Routing.IneffectiveChainThreshold < 1 {
		return fmt.Errorf("routing.ineffectiveChainThreshold must be >= 1, got %d", c.Routing.IneffectiveChainThreshold)
	}
	if c.Routing.RecurrenceCountThreshold < 1 {
		return fmt.Errorf("routing.recurrenceCountThreshold must be >= 1, got %d", c.Routing.RecurrenceCountThreshold)
	}
	if c.Routing.IneffectiveTimeWindow <= 0 {
		return fmt.Errorf("routing.ineffectiveTimeWindow must be positive, got %v", c.Routing.IneffectiveTimeWindow)
	}

	return nil
}
