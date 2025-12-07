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

// Package timeout provides timeout detection for RemediationOrchestrator.
// Reference: BR-ORCH-027 (global timeout), BR-ORCH-028 (per-phase timeout)
package timeout

import (
	"time"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator"
)

// TimeoutResult contains information about a detected timeout.
type TimeoutResult struct {
	TimedOut      bool
	TimedOutPhase string // "global", "Processing", "Analyzing", "Executing"
	Elapsed       time.Duration
}

// Detector detects phase and global timeouts.
// Reference: BR-ORCH-027 (global timeout), BR-ORCH-028 (per-phase timeout)
type Detector struct {
	config remediationorchestrator.OrchestratorConfig
}

// NewDetector creates a new timeout detector.
func NewDetector(config remediationorchestrator.OrchestratorConfig) *Detector {
	return &Detector{config: config}
}

// Terminal phases that should skip timeout checks
var terminalPhases = map[string]bool{
	"Completed": true,
	"Failed":    true,
	"Timeout":   true,
	"Skipped":   true,
}

// CheckTimeout checks if global or phase timeout has been exceeded.
// Global timeout (BR-ORCH-027) is checked first, then per-phase (BR-ORCH-028).
// Returns TimeoutResult with details about the timeout, or TimedOut=false if no timeout.
func (d *Detector) CheckTimeout(rr *remediationv1.RemediationRequest) TimeoutResult {
	currentPhase := rr.Status.OverallPhase

	// Skip if terminal state
	if d.IsTerminalPhase(currentPhase) {
		return TimeoutResult{TimedOut: false}
	}

	// Check global timeout first (BR-ORCH-027)
	if result := d.CheckGlobalTimeout(rr); result.TimedOut {
		return result
	}

	// Check per-phase timeout (BR-ORCH-028)
	return d.CheckPhaseTimeout(rr)
}

// CheckGlobalTimeout checks if global timeout has been exceeded (BR-ORCH-027).
func (d *Detector) CheckGlobalTimeout(rr *remediationv1.RemediationRequest) TimeoutResult {
	elapsed := time.Since(rr.CreationTimestamp.Time)

	// Get global timeout from config or per-remediation override
	globalTimeout := d.config.Timeouts.Global
	if rr.Spec.TimeoutConfig != nil && rr.Spec.TimeoutConfig.OverallWorkflowTimeout.Duration > 0 {
		globalTimeout = rr.Spec.TimeoutConfig.OverallWorkflowTimeout.Duration
	}

	if elapsed > globalTimeout {
		return TimeoutResult{
			TimedOut:      true,
			TimedOutPhase: "global",
			Elapsed:       elapsed,
		}
	}

	return TimeoutResult{TimedOut: false}
}

// CheckPhaseTimeout checks if current phase has timed out (BR-ORCH-028).
func (d *Detector) CheckPhaseTimeout(rr *remediationv1.RemediationRequest) TimeoutResult {
	currentPhase := rr.Status.OverallPhase

	// Get phase start time based on current phase
	var phaseStartTime *time.Time
	switch currentPhase {
	case "Processing":
		if rr.Status.ProcessingStartTime != nil {
			t := rr.Status.ProcessingStartTime.Time
			phaseStartTime = &t
		}
	case "Analyzing", "AwaitingApproval":
		if rr.Status.AnalyzingStartTime != nil {
			t := rr.Status.AnalyzingStartTime.Time
			phaseStartTime = &t
		}
	case "Executing":
		if rr.Status.ExecutingStartTime != nil {
			t := rr.Status.ExecutingStartTime.Time
			phaseStartTime = &t
		}
	}

	if phaseStartTime == nil {
		return TimeoutResult{TimedOut: false}
	}

	// Get timeout for current phase (with per-remediation override)
	timeout := d.GetPhaseTimeout(rr, currentPhase)
	elapsed := time.Since(*phaseStartTime)

	if elapsed > timeout {
		return TimeoutResult{
			TimedOut:      true,
			TimedOutPhase: currentPhase,
			Elapsed:       elapsed,
		}
	}

	return TimeoutResult{TimedOut: false}
}

// GetPhaseTimeout returns the configured timeout for a phase.
// Checks per-remediation override first, then falls back to global config.
// Reference: BR-ORCH-028
func (d *Detector) GetPhaseTimeout(rr *remediationv1.RemediationRequest, phase string) time.Duration {
	// Check per-remediation override first
	if rr.Spec.TimeoutConfig != nil {
		switch phase {
		case "Processing":
			if rr.Spec.TimeoutConfig.RemediationProcessingTimeout.Duration > 0 {
				return rr.Spec.TimeoutConfig.RemediationProcessingTimeout.Duration
			}
		case "Analyzing", "AwaitingApproval":
			if rr.Spec.TimeoutConfig.AIAnalysisTimeout.Duration > 0 {
				return rr.Spec.TimeoutConfig.AIAnalysisTimeout.Duration
			}
		case "Executing":
			if rr.Spec.TimeoutConfig.WorkflowExecutionTimeout.Duration > 0 {
				return rr.Spec.TimeoutConfig.WorkflowExecutionTimeout.Duration
			}
		}
	}

	// Fall back to global config defaults
	switch phase {
	case "Processing":
		return d.config.Timeouts.Processing
	case "Analyzing", "AwaitingApproval":
		return d.config.Timeouts.Analyzing
	case "Executing":
		return d.config.Timeouts.Executing
	default:
		return d.config.Timeouts.Global
	}
}

// IsTerminalPhase checks if the phase is terminal (no timeout check needed).
func (d *Detector) IsTerminalPhase(phase string) bool {
	return terminalPhases[phase]
}

