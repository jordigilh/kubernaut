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

package routing

import (
	"context"
	"fmt"
	"sort"
	"time"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Engine is the interface for routing decision logic.
// Allows mocking in unit tests while using real implementation in integration tests.
type Engine interface {
	CheckBlockingConditions(ctx context.Context, rr *remediationv1.RemediationRequest, workflowID string) (*BlockingCondition, error)
	Config() Config
	CalculateExponentialBackoff(consecutiveFailures int32) time.Duration
}

// RoutingEngine makes routing decisions for RemediationRequests.
// It determines if an RR should proceed to workflow execution or be blocked.
//
// Reference: DD-RO-002 (Centralized Routing Responsibility)
type RoutingEngine struct {
	client    client.Client
	apiReader client.Reader // DD-STATUS-001: Cache-bypassed reader for fresh routing queries
	namespace string
	config    Config
}

// Config holds configuration for routing decisions.
type Config struct {
	// ConsecutiveFailureThreshold is the number of consecutive failures
	// before an RR is blocked. Default: 3 (from BR-ORCH-042)
	ConsecutiveFailureThreshold int

	// ConsecutiveFailureCooldown is the duration to block after hitting
	// the consecutive failure threshold. Default: 1 hour (from BR-ORCH-042)
	ConsecutiveFailureCooldown int64 // seconds

	// RecentlyRemediatedCooldown is the duration to wait after a successful
	// remediation before allowing another remediation on the same target+workflow.
	// Default: 5 minutes (from DD-WE-001)
	RecentlyRemediatedCooldown int64 // seconds

	// ========================================
	// EXPONENTIAL BACKOFF (DD-WE-004, V1.0)
	// ========================================

	// ExponentialBackoffBase is the base cooldown period for exponential backoff.
	// Default: 60 seconds (1 minute)
	// Formula: min(Base × 2^(failures-1), Max)
	// Reference: DD-WE-004 lines 66-89
	ExponentialBackoffBase int64 // seconds

	// ExponentialBackoffMax is the maximum cooldown period for exponential backoff.
	// Default: 600 seconds (10 minutes)
	// Prevents exceeding RemediationRequest timeout (60 minutes)
	// Reference: DD-WE-004 line 70
	ExponentialBackoffMax int64 // seconds

	// ExponentialBackoffMaxExponent caps the exponential calculation.
	// Default: 4 (2^4 = 16x multiplier)
	// Prevents overflow and aligns with MaxCooldown
	// Reference: DD-WE-004 line 71
	ExponentialBackoffMaxExponent int
}

// NewRoutingEngine creates a new RoutingEngine with the given client and config.
// DD-STATUS-001: Accepts apiReader for cache-bypassed routing queries
func NewRoutingEngine(client client.Client, apiReader client.Reader, namespace string, config Config) *RoutingEngine {
	return &RoutingEngine{
		client:    client,
		apiReader: apiReader,
		namespace: namespace,
		config:    config,
	}
}

// Config returns the routing engine's configuration.
// Used by reconciler to access threshold values for exponential backoff integration.
func (r *RoutingEngine) Config() Config {
	return r.config
}

// CheckBlockingConditions checks all blocking scenarios in priority order.
// Returns a BlockingCondition if any blocking condition is found, or nil if
// the RR can proceed to workflow execution.
//
// Priority Order (checked sequentially, first match wins):
// 1. ConsecutiveFailures (BR-ORCH-042) - highest priority
// 2. DuplicateInProgress (DD-RO-002-ADDENDUM) - prevents RR flood
// 3. ResourceBusy (DD-RO-002) - protects target resources
// 4. RecentlyRemediated (DD-RO-002 Check 4) - workflow-specific cooldown
// 5. ExponentialBackoff (DD-WE-004) - graduated retry
//
// Parameters:
//   - workflowID: The workflow ID from AIAnalysis.Status.SelectedWorkflow.WorkflowID
//     Used for workflow-specific cooldown in Check 4 (RecentlyRemediated).
//
// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
func (r *RoutingEngine) CheckBlockingConditions(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	workflowID string,
) (*BlockingCondition, error) {
	// Check 1: Consecutive failures (highest priority)
	if blocked := r.CheckConsecutiveFailures(ctx, rr); blocked != nil {
		return blocked, nil
	}

	// Check 2: Duplicate in progress (critical for Gateway deduplication)
	blocked, err := r.CheckDuplicateInProgress(ctx, rr)
	if err != nil {
		return nil, fmt.Errorf("failed to check duplicate: %w", err)
	}
	if blocked != nil {
		return blocked, nil
	}

	// Check 3: Resource busy
	blocked, err = r.CheckResourceBusy(ctx, rr)
	if err != nil {
		return nil, fmt.Errorf("failed to check resource lock: %w", err)
	}
	if blocked != nil {
		return blocked, nil
	}

	// Check 4: Recently remediated (workflow-specific cooldown per DD-RO-002)
	blocked, err = r.CheckRecentlyRemediated(ctx, rr, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to check recent remediation: %w", err)
	}
	if blocked != nil {
		return blocked, nil
	}

	// Check 5: Exponential backoff
	if blocked := r.CheckExponentialBackoff(ctx, rr); blocked != nil {
		return blocked, nil
	}

	// No blocking conditions found - can proceed
	return nil, nil
}

// CheckConsecutiveFailures checks if the RR is blocked due to consecutive failures.
// Blocks when consecutive failures >= threshold.
//
// BlockReason: "ConsecutiveFailures"
// RequeueAfter: ConsecutiveFailureCooldown (default 1 hour)
//
// Reference: BR-ORCH-042 (Consecutive Failure Blocking)
//
// IMPLEMENTATION NOTE: Queries history of RRs with same SignalFingerprint to count
// consecutive failures. The incoming RR's ConsecutiveFailureCount is always 0 for new RRs.
func (r *RoutingEngine) CheckConsecutiveFailures(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
) *BlockingCondition {
	logger := log.FromContext(ctx)

	// Query for all RemediationRequests with the same fingerprint in the SAME NAMESPACE
	// BR-ORCH-042: Consecutive failure blocking MUST be namespace-scoped (multi-tenant isolation)
	list := &remediationv1.RemediationRequestList{}
	err := r.client.List(ctx, list,
		client.InNamespace(rr.Namespace), // MULTI-TENANT SAFETY: Isolate by namespace
		client.MatchingFields{
			"spec.signalFingerprint": rr.Spec.SignalFingerprint,
		})
	if err != nil {
		// Log error but don't block on query failure
		logger.Error(err, "Failed to query RRs by fingerprint", "fingerprint", rr.Spec.SignalFingerprint)
		return nil
	}

	logger.Info("CheckConsecutiveFailures query results",
		"incomingRR", rr.Name,
		"fingerprint", rr.Spec.SignalFingerprint,
		"queriedRRs", len(list.Items),
		"threshold", r.config.ConsecutiveFailureThreshold)

	// Sort ALL RRs by creation timestamp (newest first)
	// We need to check consecutive failures from most recent
	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].CreationTimestamp.After(list.Items[j].CreationTimestamp.Time)
	})

	// Log all RRs for debugging
	for i, item := range list.Items {
		logger.Info("RR in history",
			"index", i,
			"name", item.Name,
			"phase", item.Status.OverallPhase,
			"createdAt", item.CreationTimestamp.Format("15:04:05"),
			"isIncoming", item.UID == rr.UID)
	}

	// Count consecutive failures from most recent RRs
	// Stop counting when we hit a non-Failed RR (success breaks the consecutive chain)
	consecutiveFailures := 0
	for _, item := range list.Items {
		// Skip the incoming RR itself (it's not failed yet)
		if item.UID == rr.UID {
			logger.Info("Skipping incoming RR", "name", item.Name)
			continue
		}

		// Count both Failed and Blocked RRs as failures
		// Blocked RRs indicate the signal was blocked due to consecutive failures
		if item.Status.OverallPhase == remediationv1.PhaseFailed ||
		   item.Status.OverallPhase == remediationv1.PhaseBlocked {
			consecutiveFailures++
			logger.Info("Counted failed/blocked RR", "name", item.Name, "phase", item.Status.OverallPhase, "consecutiveFailures", consecutiveFailures)
		} else if item.Status.OverallPhase == remediationv1.PhaseCompleted {
			// Found a successful RR - consecutive chain is broken
			logger.Info("Found completed RR - breaking chain", "name", item.Name, "consecutiveFailures", consecutiveFailures)
			break
		} else {
			logger.Info("Ignoring non-terminal RR", "name", item.Name, "phase", item.Status.OverallPhase)
		}
		// Ignore RRs in other phases (Pending, Processing, etc.) - they're not terminal yet
	}

	logger.Info("CheckConsecutiveFailures result",
		"consecutiveFailures", consecutiveFailures,
		"threshold", r.config.ConsecutiveFailureThreshold,
		"willBlock", consecutiveFailures >= r.config.ConsecutiveFailureThreshold)

	// Check if threshold exceeded
	if consecutiveFailures < r.config.ConsecutiveFailureThreshold {
		return nil // Not blocked
	}

	// Calculate when cooldown expires
	cooldownDuration := time.Duration(r.config.ConsecutiveFailureCooldown) * time.Second
	blockedUntil := time.Now().Add(cooldownDuration)

	return &BlockingCondition{
		Blocked:      true,
		Reason:       string(remediationv1.BlockReasonConsecutiveFailures),
		Message:      fmt.Sprintf("%d consecutive failures. Cooldown expires: %s", consecutiveFailures, blockedUntil.Format(time.RFC3339)),
		RequeueAfter: cooldownDuration,
		BlockedUntil: &blockedUntil,
	}
}

// CheckDuplicateInProgress checks if this RR is a duplicate of an active RR.
// Finds active (non-terminal) RRs with the same SignalFingerprint.
//
// BlockReason: "DuplicateInProgress"
// RequeueAfter: 30 seconds (to check if original completes)
//
// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
func (r *RoutingEngine) CheckDuplicateInProgress(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
) (*BlockingCondition, error) {
	// Find active RR with same fingerprint (excluding self)
	// Multi-tenant isolation: Pass rr.Namespace to scope search to current namespace
	originalRR, err := r.FindActiveRRForFingerprint(ctx, rr.Namespace, rr.Spec.SignalFingerprint, rr.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check for duplicate: %w", err)
	}

	if originalRR == nil {
		return nil, nil // Not a duplicate
	}

	// This is a duplicate - block it
	return &BlockingCondition{
		Blocked:      true,
		Reason:       string(remediationv1.BlockReasonDuplicateInProgress),
		Message:      fmt.Sprintf("Duplicate of active remediation %s. Will inherit outcome when original completes.", originalRR.Name),
		RequeueAfter: 30 * time.Second,
		DuplicateOf:  originalRR.Name,
	}, nil
}

// CheckResourceBusy checks if another WorkflowExecution is running on the same target.
// Blocks when an active (Running phase) WFE exists for the same TargetResource.
//
// BlockReason: "ResourceBusy"
// RequeueAfter: 30 seconds (to check if WFE completes)
//
// Reference: DD-RO-002 (Centralized Routing), DD-WE-001 (Resource Locking)
func (r *RoutingEngine) CheckResourceBusy(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
) (*BlockingCondition, error) {
	// Get target resource string representation
	targetResourceStr := rr.Spec.TargetResource.String()

	// Find active WFE for the same target resource
	activeWFE, err := r.FindActiveWFEForTarget(ctx, targetResourceStr)
	if err != nil {
		return nil, fmt.Errorf("failed to check resource lock: %w", err)
	}

	if activeWFE == nil {
		return nil, nil // Resource not busy
	}

	// Resource is busy - block this RR
	return &BlockingCondition{
		Blocked:                   true,
		Reason:                    string(remediationv1.BlockReasonResourceBusy),
		Message:                   fmt.Sprintf("Another workflow (%s) is running on the same target resource. Waiting for completion.", activeWFE.Name),
		RequeueAfter:              30 * time.Second,
		BlockingWorkflowExecution: activeWFE.Name,
	}, nil
}

// CheckRecentlyRemediated checks if the same workflow+target was recently executed.
// Blocks when a completed WFE for the same target and workflow exists within cooldown.
//
// BlockReason: "RecentlyRemediated"
// RequeueAfter: Remaining cooldown duration
//
// Parameters:
//   - workflowID: The workflow ID from AIAnalysis.Status.SelectedWorkflow.WorkflowID
//     This is passed by the reconciler after AIAnalysis completes.
//     Per DD-RO-002 Check 4: Only blocks if SAME workflow was recently executed.
//
// Reference: DD-RO-002 Check 4 (Workflow Cooldown), DD-WE-001 (Cooldown Prevent Redundant Execution)
func (r *RoutingEngine) CheckRecentlyRemediated(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	workflowID string,
) (*BlockingCondition, error) {
	// Get target resource string representation
	targetResourceStr := rr.Spec.TargetResource.String()

	// DD-RO-002 Check 4: Workflow-specific cooldown
	// Only block if the SAME workflow was recently executed on the same target.
	// Different workflow on same target should NOT be blocked (different remediation approach).
	recentWFE, err := r.FindRecentCompletedWFE(
		ctx,
		targetResourceStr,
		workflowID, // Pass workflow ID for workflow-specific matching
		r.config.RecentlyRemediatedCooldown,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to check recent remediation: %w", err)
	}

	if recentWFE == nil {
		return nil, nil // No recent remediation
	}

	// Calculate remaining cooldown
	cooldownDuration := time.Duration(r.config.RecentlyRemediatedCooldown) * time.Second
	timeSinceCompletion := time.Since(recentWFE.Status.CompletionTime.Time)
	remainingCooldown := cooldownDuration - timeSinceCompletion
	blockedUntil := time.Now().Add(remainingCooldown)

	return &BlockingCondition{
		Blocked:      true,
		Reason:       string(remediationv1.BlockReasonRecentlyRemediated),
		Message:      fmt.Sprintf("Target was remediated recently (%s). Cooldown expires: %s", recentWFE.Name, blockedUntil.Format(time.RFC3339)),
		RequeueAfter: remainingCooldown,
		BlockedUntil: &blockedUntil,
	}, nil
}

// CheckExponentialBackoff checks if the RR is in an exponential backoff window.
// Blocks when NextAllowedExecution is set and in the future.
//
// BlockReason: "ExponentialBackoff"
// RequeueAfter: Time until NextAllowedExecution
//
// Reference: DD-WE-004 (Exponential Backoff Cooldown)
// ========================================
// EXPONENTIAL BACKOFF (DD-WE-004, V1.0)
// 📋 Design Decision: DD-WE-004 | ✅ Approved Design | Confidence: 90%
// See: docs/architecture/decisions/DD-WE-004-exponential-backoff-cooldown.md
// ========================================
//
// CheckExponentialBackoff checks if the RR is blocked due to exponential backoff.
// Blocks when NextAllowedExecution is set and in the future.
//
// BlockReason: "ExponentialBackoff"
// RequeueAfter: time until NextAllowedExecution
//
// WHY DD-WE-004?
// - ✅ Progressive retry delays (1m → 10m) prevent remediation storms
// - ✅ Complements consecutive failure blocking with adaptive timing
// - ✅ Reduces cluster load during persistent infrastructure issues
//
// Reference: DD-WE-004 (Exponential Backoff Cooldown)
// ========================================
func (r *RoutingEngine) CheckExponentialBackoff(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
) *BlockingCondition {
	logger := log.FromContext(ctx)

	// No backoff configured
	if rr.Status.NextAllowedExecution == nil {
		return nil
	}

	now := time.Now()
	nextAllowed := rr.Status.NextAllowedExecution.Time

	// Backoff expired - can proceed
	if nextAllowed.Before(now) || nextAllowed.Equal(now) {
		logger.V(1).Info("Exponential backoff expired, allowing execution",
			"nextAllowedExecution", nextAllowed,
			"now", now)
		return nil
	}

	// Backoff still active - block
	requeueAfter := nextAllowed.Sub(now)
	logger.Info("Blocking due to exponential backoff",
		"nextAllowedExecution", nextAllowed.Format(time.RFC3339),
		"requeueAfter", requeueAfter,
		"consecutiveFailures", rr.Status.ConsecutiveFailureCount)

	return &BlockingCondition{
		Blocked:      true,
		Reason:       string(remediationv1.BlockReasonExponentialBackoff),
		Message:      fmt.Sprintf("Exponential backoff active. Next execution allowed at %s (in %s)", nextAllowed.Format(time.RFC3339), requeueAfter.Round(time.Second)),
		RequeueAfter: requeueAfter,
		BlockedUntil: &nextAllowed,
	}
}

// CalculateExponentialBackoff calculates the cooldown duration based on consecutive failures.
//
// Formula: Cooldown = min(Base × 2^(failures-1), Max)
//
// Examples (Base=1min, Max=10min):
//   - 1 failure:  1min × 2^0 = 1min
//   - 2 failures: 1min × 2^1 = 2min
//   - 3 failures: 1min × 2^2 = 4min
//   - 4 failures: 1min × 2^3 = 8min
//   - 5+ failures: capped at 10min
//
// ========================================
// EXPONENTIAL BACKOFF (DD-SHARED-001)
// 📋 Design Decision: DD-SHARED-001 | ✅ Adopted Shared Library | Confidence: 100%
// See: docs/architecture/decisions/DD-SHARED-001-shared-backoff-library.md
// ========================================
//
// Uses shared exponential backoff library (pkg/shared/backoff) with ±10% jitter.
//
// WHY DD-SHARED-001?
// - ✅ Single source of truth: Consistent formula across all services
// - ✅ Battle-tested: 24 comprehensive unit tests, 100% passing
// - ✅ Maintainable: Bug fixes in one place benefit all services
// - ✅ Anti-thundering herd: Jitter prevents simultaneous retries in HA deployment (2+ replicas)
//
// WHY JITTER?
// RO runs with 2+ replicas (leader election, HA). Without jitter, multiple RRs with
// consecutive failures would retry simultaneously after cooldown, creating load spikes
// on downstream services (AIAnalysis, WorkflowExecution). 10% jitter distributes
// retries over time (~48s window for 8min backoff).
//
// Reference: DD-WE-004 (Exponential Backoff Cooldown), DD-SHARED-001 (Shared Backoff Library)
// ========================================
func (r *RoutingEngine) CalculateExponentialBackoff(consecutiveFailures int32) time.Duration {
	if consecutiveFailures <= 0 {
		return 0 // No failures, no backoff
	}

	// Use shared backoff library (DD-SHARED-001)
	// Configuration for production HA deployment (2+ replicas with leader election)
	config := backoff.Config{
		BasePeriod:    time.Duration(r.config.ExponentialBackoffBase) * time.Second,
		MaxPeriod:     time.Duration(r.config.ExponentialBackoffMax) * time.Second,
		Multiplier:    2.0,  // Standard exponential (power-of-2)
		JitterPercent: 10,   // ±10% variance prevents thundering herd in HA deployment
	}

	// Note: MaxExponent capping is handled by MaxPeriod in shared library
	// Jitter distributes retry attempts over time, preventing load spikes on downstream services
	return config.Calculate(consecutiveFailures)
}

// FindActiveRRForFingerprint finds an active (non-terminal) RR with the given fingerprint.
// Returns the first active RR found, or nil if none exist.
// Excludes the RR with excludeName (to avoid self-matching).
//
// Uses field index on spec.signalFingerprint for O(1) lookup (configured in Day 1).
//
// Reference: BR-ORCH-042 (Fingerprint Field Index)
// Multi-Tenant Isolation: Scoped to namespace parameter for tenant isolation (BR-ORCH-042)
func (r *RoutingEngine) FindActiveRRForFingerprint(
	ctx context.Context,
	namespace string,
	fingerprint string,
	excludeName string,
) (*remediationv1.RemediationRequest, error) {
	logger := log.FromContext(ctx)

	// List all RRs with matching fingerprint using field index
	// NOTE: Must use cached client for field index queries (indexes not available on APIReader)
	// CRITICAL: Use namespace parameter (not r.namespace) for multi-tenant isolation
	rrList := &remediationv1.RemediationRequestList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingFields{"spec.signalFingerprint": fingerprint},
	}

	if err := r.client.List(ctx, rrList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list RemediationRequests by fingerprint: %w", err)
	}

	// Find first active (non-terminal) RR, excluding self
	// DD-STATUS-001: Refetch each candidate with APIReader to get fresh status
	for i := range rrList.Items {
		rr := &rrList.Items[i]
		if rr.Name == excludeName {
			continue // Skip self
		}

		// Refetch with APIReader to bypass cache and get fresh status
		// This prevents false "DuplicateInProgress" blocks due to stale status in cache
		freshRR := &remediationv1.RemediationRequest{}
		if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(rr), freshRR); err != nil {
			logger.Error(err, "Failed to refetch RR status", "rr", rr.Name)
			// Fall back to cached status if refetch fails
			freshRR = rr
		}

		if !IsTerminalPhase(freshRR.Status.OverallPhase) {
			logger.V(1).Info("Found active RR with fingerprint",
				"rr", freshRR.Name,
				"fingerprint", fingerprint,
				"phase", freshRR.Status.OverallPhase)
			return freshRR, nil
		}
	}

	return nil, nil // No active duplicate found
}

// FindActiveWFEForTarget finds an active (Running phase) WFE for the given target resource.
// Returns the first running WFE found, or nil if none exist.
//
// Uses field index on spec.targetResource for O(1) lookup (configured in Day 1).
//
// Reference: DD-RO-002 (Target Resource Field Index)
func (r *RoutingEngine) FindActiveWFEForTarget(
	ctx context.Context,
	targetResource string,
) (*workflowexecutionv1.WorkflowExecution, error) {
	logger := log.FromContext(ctx)

	// List all WFEs with matching target resource using field index
	// NOTE: Must use cached client for field index queries (indexes not available on APIReader)
	wfeList := &workflowexecutionv1.WorkflowExecutionList{}
	listOpts := []client.ListOption{
		client.InNamespace(r.namespace),
		client.MatchingFields{"spec.targetResource": targetResource},
	}

	if err := r.client.List(ctx, wfeList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list WorkflowExecutions by target: %w", err)
	}

	// Find first active (non-terminal) WFE
	// DD-STATUS-001: Refetch each candidate with APIReader to get fresh status
	for i := range wfeList.Items {
		wfe := &wfeList.Items[i]

		// Refetch with APIReader to bypass cache and get fresh status
		// This prevents false "ResourceBusy" blocks due to stale status in cache
		freshWFE := &workflowexecutionv1.WorkflowExecution{}
		if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(wfe), freshWFE); err != nil {
			logger.Error(err, "Failed to refetch WFE status", "wfe", wfe.Name)
			// Fall back to cached status if refetch fails
			freshWFE = wfe
		}

		// Check if phase is not terminal (Running, Pending, etc.)
		// V1.0: Only Completed and Failed are terminal phases
		if freshWFE.Status.Phase != workflowexecutionv1.PhaseCompleted &&
			freshWFE.Status.Phase != workflowexecutionv1.PhaseFailed {
			logger.V(1).Info("Found active WFE for target",
				"wfe", freshWFE.Name,
				"target", targetResource,
				"phase", freshWFE.Status.Phase)
			return freshWFE, nil
		}
	}

	return nil, nil // No active WFE found
}

// FindRecentCompletedWFE finds the most recent completed WFE for the given
// target resource and workflow ID, within the cooldown period.
// Returns the WFE if found within cooldown, or nil if none exist or outside cooldown.
//
// Uses field index on spec.targetResource for O(1) lookup (configured in Day 1).
//
// Reference: DD-WE-001 (Recently Remediated Cooldown)
func (r *RoutingEngine) FindRecentCompletedWFE(
	ctx context.Context,
	targetResource string,
	workflowID string,
	cooldownDuration int64,
) (*workflowexecutionv1.WorkflowExecution, error) {
	logger := log.FromContext(ctx)
	cooldown := time.Duration(cooldownDuration) * time.Second

	// List all WFEs with matching target resource using field index
	wfeList := &workflowexecutionv1.WorkflowExecutionList{}
	listOpts := []client.ListOption{
		client.InNamespace(r.namespace),
		client.MatchingFields{"spec.targetResource": targetResource},
	}

	if err := r.client.List(ctx, wfeList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list WorkflowExecutions for cooldown check: %w", err)
	}

	var mostRecentCompleted *workflowexecutionv1.WorkflowExecution
	now := time.Now()

	// Find most recent completed WFE matching workflow ID within cooldown
	for i := range wfeList.Items {
		wfe := &wfeList.Items[i]

		// Filter: Only completed phase
		if wfe.Status.Phase != workflowexecutionv1.PhaseCompleted {
			continue
		}

		// Filter: Must have CompletionTime timestamp
		if wfe.Status.CompletionTime == nil {
			logger.V(1).Info("Skipping completed WFE with nil CompletionTime",
				"wfe", wfe.Name)
			continue
		}

		// Filter: Must match workflow ID (if specified)
		if workflowID != "" && wfe.Spec.WorkflowRef.WorkflowID != workflowID {
			continue
		}

		// Filter: Must be within cooldown period
		timeSinceCompletion := now.Sub(wfe.Status.CompletionTime.Time)
		if timeSinceCompletion >= cooldown {
			continue // Outside cooldown window
		}

		// Track most recent
		if mostRecentCompleted == nil ||
			wfe.Status.CompletionTime.After(mostRecentCompleted.Status.CompletionTime.Time) {
			mostRecentCompleted = wfe
		}
	}

	return mostRecentCompleted, nil
}
