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

// Phases that skip standard timeout checks (terminal phases + phases with their own timeout mechanism).
// Terminal phases have no further processing; Blocked has its own cooldown; Verifying has VerificationDeadline.
var skipTimeoutPhases = map[string]bool{
	string(remediationv1.PhaseCompleted): true,
	string(remediationv1.PhaseFailed):    true,
	string(remediationv1.PhaseTimedOut):  true,
	string(remediationv1.PhaseSkipped):   true,
	// Blocked is NON-terminal but has its own cooldown mechanism (BR-ORCH-042)
	string(remediationv1.PhaseBlocked): true,
	// Verifying is NON-terminal but has its own deadline via VerificationDeadline (#280)
	string(remediationv1.PhaseVerifying): true,
}

// CheckTimeout checks if global or phase timeout has been exceeded.
// Global timeout (BR-ORCH-027) is checked first, then per-phase (BR-ORCH-028).
// Returns TimeoutResult with details about the timeout, or TimedOut=false if no timeout.
func (d *Detector) CheckTimeout(rr *remediationv1.RemediationRequest) TimeoutResult {
	currentPhase := rr.Status.OverallPhase

	// Skip if phase has its own timeout mechanism or is terminal
	if d.SkipTimeoutCheck(string(currentPhase)) {
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

	// Get global timeout from config or per-remediation override (AC-027-4)
	globalTimeout := d.config.Timeouts.Global
	if rr.Status.TimeoutConfig != nil && rr.Status.TimeoutConfig.Global != nil && rr.Status.TimeoutConfig.Global.Duration > 0 {
		globalTimeout = rr.Status.TimeoutConfig.Global.Duration
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
	case remediationv1.PhaseProcessing:
		if rr.Status.ProcessingStartTime != nil {
			t := rr.Status.ProcessingStartTime.Time
			phaseStartTime = &t
		}
	case remediationv1.PhaseAnalyzing, remediationv1.PhaseAwaitingApproval:
		if rr.Status.AnalyzingStartTime != nil {
			t := rr.Status.AnalyzingStartTime.Time
			phaseStartTime = &t
		}
	case remediationv1.PhaseExecuting:
		if rr.Status.ExecutingStartTime != nil {
			t := rr.Status.ExecutingStartTime.Time
			phaseStartTime = &t
		}
	}

	if phaseStartTime == nil {
		return TimeoutResult{TimedOut: false}
	}

	// Get timeout for current phase (with per-remediation override)
	timeout := d.GetPhaseTimeout(rr, string(currentPhase))
	elapsed := time.Since(*phaseStartTime)

	if elapsed > timeout {
		return TimeoutResult{
			TimedOut:      true,
			TimedOutPhase: string(currentPhase),
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
	if rr.Status.TimeoutConfig != nil {
		switch phase {
		case string(remediationv1.PhaseProcessing):
			if rr.Status.TimeoutConfig.Processing != nil && rr.Status.TimeoutConfig.Processing.Duration > 0 {
				return rr.Status.TimeoutConfig.Processing.Duration
			}
		case string(remediationv1.PhaseAnalyzing), string(remediationv1.PhaseAwaitingApproval):
			if rr.Status.TimeoutConfig.Analyzing != nil && rr.Status.TimeoutConfig.Analyzing.Duration > 0 {
				return rr.Status.TimeoutConfig.Analyzing.Duration
			}
		case string(remediationv1.PhaseExecuting):
			if rr.Status.TimeoutConfig.Executing != nil && rr.Status.TimeoutConfig.Executing.Duration > 0 {
				return rr.Status.TimeoutConfig.Executing.Duration
			}
		}
	}

	// Fall back to global config defaults
	switch phase {
	case string(remediationv1.PhaseProcessing):
		return d.config.Timeouts.Processing
	case string(remediationv1.PhaseAnalyzing), string(remediationv1.PhaseAwaitingApproval):
		return d.config.Timeouts.Analyzing
	case string(remediationv1.PhaseExecuting):
		return d.config.Timeouts.Executing
	default:
		return d.config.Timeouts.Global
	}
}

// SkipTimeoutCheck returns true if the phase should skip standard timeout detection.
// This includes terminal phases and phases with their own timeout mechanism (Blocked, Verifying).
func (d *Detector) SkipTimeoutCheck(phase string) bool {
	return skipTimeoutPhases[phase]
}
