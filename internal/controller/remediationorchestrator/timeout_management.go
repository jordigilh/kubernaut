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

package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// populateTimeoutDefaults initializes rr.Status.TimeoutConfig from controller
// defaults on first reconcile. No-op if TimeoutConfig is already set.
//
// Parameters:
// - ctx: Context for logging
// - rr: RemediationRequest to populate (modified in-place)
//
// Returns:
// - bool: true if TimeoutConfig was populated, false if already initialized
func (r *Reconciler) populateTimeoutDefaults(ctx context.Context, rr *remediationv1.RemediationRequest) bool {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name, "namespace", rr.Namespace)

	// Only initialize if status.timeoutConfig is nil (first reconcile)
	if rr.Status.TimeoutConfig != nil {
		logger.V(2).Info("TimeoutConfig already initialized, skipping",
			"global", rr.Status.TimeoutConfig.Global,
			"processing", rr.Status.TimeoutConfig.Processing,
			"analyzing", rr.Status.TimeoutConfig.Analyzing,
			"executing", rr.Status.TimeoutConfig.Executing)
		return false // Already initialized, preserve existing values
	}

	// REFACTOR: Validate controller timeouts before applying
	// This prevents configuration errors from propagating to RRs
	if err := r.validateControllerTimeouts(); err != nil {
		logger.Error(err, "Controller timeout configuration invalid, using safe defaults")
		// Fallback to safe defaults if controller config is invalid
		rr.Status.TimeoutConfig = r.getSafeDefaultTimeouts()
		return true
	}

	// Set defaults from controller config
	rr.Status.TimeoutConfig = &remediationv1.TimeoutConfig{
		Global:     &metav1.Duration{Duration: r.timeouts.Global},
		Processing: &metav1.Duration{Duration: r.timeouts.Processing},
		Analyzing:  &metav1.Duration{Duration: r.timeouts.Analyzing},
		Executing:  &metav1.Duration{Duration: r.timeouts.Executing},
	}

	logger.Info("Populated timeout defaults in status.timeoutConfig",
		"global", r.timeouts.Global,
		"processing", r.timeouts.Processing,
		"analyzing", r.timeouts.Analyzing,
		"executing", r.timeouts.Executing)

	return true
}

// validateControllerTimeouts validates that controller-level timeout configuration is sane.
// REFACTOR enhancement: Prevents invalid configuration from affecting RRs.
//
// Validation rules:
// - All timeouts must be positive (>0)
// - Per-phase timeouts should not exceed global timeout
// - Global timeout should be at least 1 minute
//
// Returns:
// - error: Non-nil if validation fails
func (r *Reconciler) validateControllerTimeouts() error {
	if r.timeouts.Global <= 0 {
		return fmt.Errorf("global timeout must be positive, got %v", r.timeouts.Global)
	}
	if r.timeouts.Global < 1*time.Minute {
		return fmt.Errorf("global timeout too short (%v), must be at least 1 minute", r.timeouts.Global)
	}
	if r.timeouts.Processing <= 0 {
		return fmt.Errorf("processing timeout must be positive, got %v", r.timeouts.Processing)
	}
	if r.timeouts.Analyzing <= 0 {
		return fmt.Errorf("analyzing timeout must be positive, got %v", r.timeouts.Analyzing)
	}
	if r.timeouts.Executing <= 0 {
		return fmt.Errorf("executing timeout must be positive, got %v", r.timeouts.Executing)
	}

	if r.timeouts.MaxAnalyzing <= 0 {
		return fmt.Errorf("maxAnalyzing timeout must be positive, got %v", r.timeouts.MaxAnalyzing)
	}
	if r.timeouts.MaxAnalyzing < r.timeouts.Analyzing {
		return fmt.Errorf("maxAnalyzing timeout (%v) must be >= analyzing timeout (%v)", r.timeouts.MaxAnalyzing, r.timeouts.Analyzing)
	}

	// Warn if per-phase timeouts exceed global (not fatal, but suspicious)
	if r.timeouts.Processing > r.timeouts.Global {
		return fmt.Errorf("processing timeout (%v) exceeds global timeout (%v)", r.timeouts.Processing, r.timeouts.Global)
	}
	if r.timeouts.Analyzing > r.timeouts.Global {
		return fmt.Errorf("analyzing timeout (%v) exceeds global timeout (%v)", r.timeouts.Analyzing, r.timeouts.Global)
	}
	if r.timeouts.Executing > r.timeouts.Global {
		return fmt.Errorf("executing timeout (%v) exceeds global timeout (%v)", r.timeouts.Executing, r.timeouts.Global)
	}
	if r.timeouts.MaxAnalyzing > r.timeouts.Global {
		return fmt.Errorf("maxAnalyzing timeout (%v) exceeds global timeout (%v)", r.timeouts.MaxAnalyzing, r.timeouts.Global)
	}

	return nil
}

// getSafeDefaultTimeouts returns safe fallback timeout values.
// Used when controller configuration is invalid.
// REFACTOR enhancement: Ensures system never operates with zero timeouts.
//
// Returns:
// - *remediationv1.TimeoutConfig: Safe default configuration
func (r *Reconciler) getSafeDefaultTimeouts() *remediationv1.TimeoutConfig {
	return &remediationv1.TimeoutConfig{
		Global:     &metav1.Duration{Duration: 1 * time.Hour},
		Processing: &metav1.Duration{Duration: 5 * time.Minute},
		Analyzing:  &metav1.Duration{Duration: 10 * time.Minute},
		Executing:  &metav1.Duration{Duration: 30 * time.Minute},
	}
}

// safeFormatTime safely formats a time, returning "N/A" if nil.
func safeFormatTime(t *metav1.Time) string {
	if t == nil {
		return "N/A"
	}
	return t.Format(time.RFC3339)
}

// getEffectiveGlobalTimeout returns the effective global timeout for a remediation.
// Checks for per-RR override in spec.timeoutConfig.global (AC-027-4).
// Falls back to controller-level default if not overridden.
func (r *Reconciler) getEffectiveGlobalTimeout(rr *remediationv1.RemediationRequest) time.Duration {
	if rr.Status.TimeoutConfig != nil && rr.Status.TimeoutConfig.Global != nil {
		return rr.Status.TimeoutConfig.Global.Duration
	}
	return r.timeouts.Global
}

// getEffectivePhaseTimeout returns the effective timeout for a specific phase.
// Checks for per-RR override in spec.timeoutConfig (AC-028-5).
// Falls back to controller-level default if not overridden.
func (r *Reconciler) getEffectivePhaseTimeout(rr *remediationv1.RemediationRequest, phase remediationv1.RemediationPhase) time.Duration {
	if rr.Status.TimeoutConfig != nil {
		switch phase {
		case remediationv1.PhaseProcessing:
			if rr.Status.TimeoutConfig.Processing != nil {
				return rr.Status.TimeoutConfig.Processing.Duration
			}
		case remediationv1.PhaseAnalyzing:
			if rr.Status.TimeoutConfig.Analyzing != nil {
				return rr.Status.TimeoutConfig.Analyzing.Duration
			}
		case remediationv1.PhaseExecuting:
			if rr.Status.TimeoutConfig.Executing != nil {
				return rr.Status.TimeoutConfig.Executing.Duration
			}
		}
	}

	// Fall back to controller-level defaults
	switch phase {
	case remediationv1.PhaseProcessing:
		return r.timeouts.Processing
	case remediationv1.PhaseAnalyzing:
		return r.timeouts.Analyzing
	case remediationv1.PhaseExecuting:
		return r.timeouts.Executing
	default:
		// For phases without specific timeouts, return 0 (no timeout)
		return 0
	}
}

// checkPhaseTimeouts checks if the current phase has exceeded its timeout.
// Returns error if phase timeout detected and transition to TimedOut phase succeeds.
// Reference: BR-ORCH-028 (Per-phase timeouts), AC-028-2
func (r *Reconciler) checkPhaseTimeouts(ctx context.Context, rr *remediationv1.RemediationRequest) error {
	logger := log.FromContext(ctx)

	currentPhase := rr.Status.OverallPhase
	var phaseStartTime *metav1.Time

	// Get phase start time based on current phase
	switch currentPhase {
	case remediationv1.PhaseProcessing:
		phaseStartTime = rr.Status.ProcessingStartTime
	case remediationv1.PhaseAnalyzing:
		phaseStartTime = rr.Status.AnalyzingStartTime
	case remediationv1.PhaseExecuting:
		phaseStartTime = rr.Status.ExecutingStartTime
	default:
		// Phase doesn't have specific timeout
		return nil
	}

	// No phase start time set yet, skip check
	if phaseStartTime == nil {
		return nil
	}

	// Get effective timeout for this phase
	phaseTimeout := r.getEffectivePhaseTimeout(rr, currentPhase)
	if phaseTimeout == 0 {
		// No timeout configured for this phase
		return nil
	}

	// DD-INTERACTIVE-002: Extend Analyzing timeout when interactive session is active
	if currentPhase == remediationv1.PhaseAnalyzing {
		phaseTimeout = r.applyInteractiveTimeoutExtension(ctx, rr, phaseTimeout)
	}

	// Check if phase has exceeded timeout
	timeSincePhaseStart := time.Since(phaseStartTime.Time)
	if timeSincePhaseStart > phaseTimeout {
		logger.Info("RemediationRequest exceeded per-phase timeout",
			"phase", currentPhase,
			"timeSincePhaseStart", timeSincePhaseStart,
			"phaseTimeout", phaseTimeout,
			"phaseStartTime", phaseStartTime.Time,
			"overridden", rr.Status.TimeoutConfig != nil)
		return r.handlePhaseTimeout(ctx, rr, currentPhase, phaseTimeout)
	}

	return nil
}

// applyInteractiveTimeoutExtension checks if the associated AIAnalysis has an active
// interactive session and extends the timeout to MaxAnalyzing if so.
// Falls back gracefully to the original timeout on AA fetch errors.
// Reference: DD-INTERACTIVE-002 (dynamic timeout extension)
func (r *Reconciler) applyInteractiveTimeoutExtension(ctx context.Context, rr *remediationv1.RemediationRequest, defaultTimeout time.Duration) time.Duration {
	if rr.Status.AIAnalysisRef == nil {
		return defaultTimeout
	}

	logger := log.FromContext(ctx)
	ai := &aianalysisv1.AIAnalysis{}
	key := client.ObjectKey{
		Name:      rr.Status.AIAnalysisRef.Name,
		Namespace: rr.Status.AIAnalysisRef.Namespace,
	}
	if key.Namespace == "" {
		key.Namespace = rr.Namespace
	}

	if err := r.client.Get(ctx, key, ai); err != nil {
		logger.V(1).Info("Failed to fetch AIAnalysis for interactive timeout check, using default",
			"aiAnalysis", key, "error", err)
		return defaultTimeout
	}

	if ai.Status.InteractiveSession == nil {
		return defaultTimeout
	}

	// Active session: StartedAt set, CompletedAt nil
	if ai.Status.InteractiveSession.StartedAt != nil && ai.Status.InteractiveSession.CompletedAt == nil {
		extended := r.timeouts.MaxAnalyzing
		if extended > defaultTimeout {
			logger.Info("Extending Analyzing timeout for active interactive session",
				"sessionID", ai.Status.InteractiveSession.SessionID,
				"actingUser", ai.Status.InteractiveSession.ActingUser,
				"defaultTimeout", defaultTimeout,
				"extendedTimeout", extended)
			return extended
		}
	}

	return defaultTimeout
}

// handlePhaseTimeout handles phase timeout by transitioning to TimedOut phase.
// Creates notification for phase-specific escalation.
// Reference: BR-ORCH-028 (Per-phase timeouts), AC-028-4
func (r *Reconciler) handlePhaseTimeout(ctx context.Context, rr *remediationv1.RemediationRequest, phase remediationv1.RemediationPhase, timeout time.Duration) error {
	logger := log.FromContext(ctx)

	// Update status to TimedOut with phase-specific metadata
	// REFACTOR-RO-001: Using retry helper
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		// Transition to TimedOut phase
		rr.Status.OverallPhase = remediationv1.PhaseTimedOut
		rr.Status.Message = fmt.Sprintf("Phase %s exceeded timeout of %s", phase, timeout)
		rr.Status.TimeoutTime = &metav1.Time{Time: time.Now()}
		rr.Status.TimeoutPhase = &phase
		rr.Status.CompletedAt = &metav1.Time{Time: time.Now()}
		return nil
	})

	if err != nil {
		logger.Error(err, "Failed to update status on phase timeout")
		return err
	}

	// Record timeout metric (BR-ORCH-044)
	if r.Metrics != nil {
		r.Metrics.TimeoutsTotal.WithLabelValues(rr.Namespace, string(phase)).Inc()
	}

	// Per DD-AUDIT-003: Emit timeout event (lifecycle.completed with outcome=failure)
	if rr.Status.StartTime != nil {
		durationMs := time.Since(rr.Status.StartTime.Time).Milliseconds()
		r.emitTimeoutAudit(ctx, rr, "phase", string(phase), durationMs)
	}

	// DD-EVENT-001: Emit K8s event for phase timeout (BR-ORCH-095)
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonRemediationTimeout,
			fmt.Sprintf("Phase %s exceeded timeout of %s", phase, timeout))
	}

	logger.Info("RemediationRequest transitioned to TimedOut due to phase timeout",
		"phase", phase,
		"timeout", timeout)

	// Create phase-specific timeout notification (non-blocking)
	r.createPhaseTimeoutNotification(ctx, rr, phase, timeout)

	// Issue #240: EA is NOT created on phase timeout. See transitionToVerifying.

	return nil
}

// getConsecutiveFailureThreshold returns the configured threshold from the routing engine.
func (r *Reconciler) getConsecutiveFailureThreshold() int {
	return r.routingEngine.Config().ConsecutiveFailureThreshold
}

// validateTimeoutConfig validates the timeout configuration in RemediationRequest.Status.TimeoutConfig.
// BR-AUDIT-005 Gap #7: Validates that all timeouts are non-negative.
// Gap #8: TimeoutConfig moved from Spec to Status for operator mutability.
// Returns error with ERR_INVALID_TIMEOUT_CONFIG code if validation fails.
func (r *Reconciler) validateTimeoutConfig(rr *remediationv1.RemediationRequest) error {
	if rr.Status.TimeoutConfig == nil {
		return nil // No custom timeout config, use defaults
	}

	// Validate Global timeout
	if rr.Status.TimeoutConfig.Global != nil && rr.Status.TimeoutConfig.Global.Duration < 0 {
		return fmt.Errorf("global timeout cannot be negative (got: %v): %w", rr.Status.TimeoutConfig.Global.Duration, roaudit.ErrInvalidTimeoutConfig)
	}

	// Validate Processing timeout
	if rr.Status.TimeoutConfig.Processing != nil && rr.Status.TimeoutConfig.Processing.Duration < 0 {
		return fmt.Errorf("processing timeout cannot be negative (got: %v): %w", rr.Status.TimeoutConfig.Processing.Duration, roaudit.ErrInvalidTimeoutConfig)
	}

	// Validate Analyzing timeout
	if rr.Status.TimeoutConfig.Analyzing != nil && rr.Status.TimeoutConfig.Analyzing.Duration < 0 {
		return fmt.Errorf("analyzing timeout cannot be negative (got: %v): %w", rr.Status.TimeoutConfig.Analyzing.Duration, roaudit.ErrInvalidTimeoutConfig)
	}

	// Validate Executing timeout
	if rr.Status.TimeoutConfig.Executing != nil && rr.Status.TimeoutConfig.Executing.Duration < 0 {
		return fmt.Errorf("executing timeout cannot be negative (got: %v): %w", rr.Status.TimeoutConfig.Executing.Duration, roaudit.ErrInvalidTimeoutConfig)
	}

	return nil
}
