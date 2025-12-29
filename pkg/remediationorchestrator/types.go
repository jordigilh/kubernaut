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

// Package remediationorchestrator provides the central coordinator for the Kubernaut
// remediation lifecycle. It orchestrates SignalProcessing, AIAnalysis, WorkflowExecution,
// and NotificationRequest CRDs.
//
// Business Requirements:
// - BR-ORCH-025: Phase state transitions
// - BR-ORCH-026: Status aggregation
// - BR-ORCH-027: Global timeout handling
// - BR-ORCH-028: Per-phase timeout handling
// - BR-ORCH-001: Approval notification creation
package remediationorchestrator

import (
	"time"
)

// PhaseTimeouts defines configurable timeouts per phase.
//
// Business Requirements:
// - BR-ORCH-027: Global timeout for entire remediation
// - BR-ORCH-028: Per-phase timeouts for individual phases
type PhaseTimeouts struct {
	// Processing timeout for SignalProcessing phase (default: 5 minutes)
	Processing time.Duration

	// Analyzing timeout for AIAnalysis phase (default: 10 minutes)
	Analyzing time.Duration

	// Executing timeout for WorkflowExecution phase (default: 30 minutes)
	Executing time.Duration

	// Global timeout for entire remediation (default: 60 minutes)
	// Reference: BR-ORCH-027
	Global time.Duration
}

// DefaultPhaseTimeouts returns sensible defaults for phase timeouts.
func DefaultPhaseTimeouts() PhaseTimeouts {
	return PhaseTimeouts{
		Processing: 5 * time.Minute,
		Analyzing:  10 * time.Minute,
		Executing:  30 * time.Minute,
		Global:     60 * time.Minute,
	}
}

// OrchestratorConfig holds controller configuration.
type OrchestratorConfig struct {
	// Timeouts for each phase
	Timeouts PhaseTimeouts

	// RetentionPeriod after completion (default: 24h)
	RetentionPeriod time.Duration

	// MaxConcurrentReconciles limits parallel reconciliations
	MaxConcurrentReconciles int

	// EnableMetrics enables Prometheus metrics
	EnableMetrics bool
}

// DefaultConfig returns sensible defaults for the orchestrator.
func DefaultConfig() OrchestratorConfig {
	return OrchestratorConfig{
		Timeouts:                DefaultPhaseTimeouts(),
		RetentionPeriod:         24 * time.Hour,
		MaxConcurrentReconciles: 10,
		EnableMetrics:           true,
	}
}
