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

package gateway

// ========================================
// ProcessSignal orchestration, split out of signal_ingestion.go (Wave 6
// GREEN: file-size remediation for the Go Anti-Pattern Audit). The HTTP-layer
// adapter registration/handler wiring stays in signal_ingestion.go; this file
// holds the core per-signal business pipeline: scope validation → distributed
// lock (BR-GATEWAY-190) → deduplication → CRD creation.
// ========================================

import (
	"context"
	"fmt"
	"time"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/middleware"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff" // ADR-052 Addendum 001: Exponential backoff with jitter
)

// ProcessSignal implements adapters.SignalProcessor interface.
//
// Main signal processing pipeline orchestrator, called by adapter handlers.
// TDD REFACTOR: Simplified by extracting helper methods.
//
// Pipeline stages:
//  1. Scope validation → validateScope() rejects unmanaged resources
//  2. Optional distributed lock (DD-GATEWAY-013) for multi-replica safety
//  3. Deduplication check → K8s status lookup (DD-GATEWAY-011); if duplicate,
//     update status.deduplication on the existing RemediationRequest and return HTTP 202
//  4. CRD creation → createRemediationRequestCRD() for new signals; return HTTP 201
//
// Note: Environment classification and Priority assignment removed (2025-12-06).
// These are now owned by Signal Processing service per DD-CATEGORIZATION-001.
//
// Performance (order-of-magnitude; varies by cluster and API load):
// - New signal: p95 often ~50-80ms — K8s dedup check, CRD creation (Kubernetes API).
// - Duplicate: p95 often lower — K8s dedup check and status patch; no new CRD.
func (s *Server) ProcessSignal(ctx context.Context, signal *types.NormalizedSignal) (*ProcessingResponse, error) {
	start := time.Now()
	logger := middleware.GetLogger(ctx)

	// Record ingestion metric (environment label removed - SP owns classification)
	s.metricsInstance.AlertsReceivedTotal.WithLabelValues(signal.Source, signal.Severity).Inc()

	// BR-SCOPE-002: Validate resource is within Kubernaut's management scope
	if rejection, err := s.validateScope(ctx, signal); err != nil {
		return nil, err
	} else if rejection != nil {
		return rejection, nil
	}

	// BR-GATEWAY-190: Acquire distributed lock for multi-replica safety
	// DD-GATEWAY-013: K8s Lease-based distributed locking pattern
	// ADR-052 Addendum 001 (Jan 2026): Exponential backoff with jitter (anti-thundering herd)
	if s.lockManager != nil {
		releaseLock, duplicateResponse, lockErr := s.acquireDistributedLockWithRetry(ctx, signal)
		if duplicateResponse != nil || lockErr != nil {
			return duplicateResponse, lockErr
		}
		defer releaseLock()
	}

	// 1. Deduplication check (DD-GATEWAY-011: K8s status-based, NOT Redis)
	// BR-GATEWAY-185: Redis deprecation - use PhaseBasedDeduplicationChecker
	// Issue #195: Use controllerNamespace — RRs live in controller NS per ADR-057
	shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(ctx, s.controllerNamespace, signal.Fingerprint)
	if err != nil {
		logger.Error(err, "Deduplication check failed",
			"fingerprint", signal.Fingerprint)
		return nil, fmt.Errorf("deduplication check failed: %w", err)
	}

	if shouldDeduplicate && existingRR != nil {
		occurrenceCount := int32(1)
		if existingRR.Status.Deduplication != nil {
			occurrenceCount = existingRR.Status.Deduplication.OccurrenceCount
		}
		logger.V(1).Info("Duplicate signal detected (K8s status-based)",
			"fingerprint", signal.Fingerprint,
			"existingRR", existingRR.Name,
			"phase", existingRR.Status.OverallPhase,
			"occurrenceCount", occurrenceCount)
		// Reuses the same dedup-status-update + metrics + audit logic as the
		// lock-contention retry path (handleDuplicateSignal), since both are
		// the identical "duplicate found" business outcome (BR-GATEWAY-190/185).
		return s.handleDuplicateSignal(ctx, signal, existingRR)
	}

	// 2. CRD creation pipeline
	return s.createRemediationRequestCRD(ctx, signal, start)
}

// acquireDistributedLockWithRetry implements BR-GATEWAY-190's multi-replica
// safety guarantee: retry lock acquisition with exponential backoff+jitter
// (ADR-052 Addendum 001), re-checking deduplication on every contended retry
// in case a competing pod created the RemediationRequest while we waited.
// Extracted from ProcessSignal to keep its cognitive complexity low.
//
// Returns exactly one of:
//   - (releaseFunc, nil, nil): lock acquired: caller must `defer releaseFunc()`
//     and continue the normal dedup+CRD-creation pipeline.
//   - (nil, response, nil): a competing pod already created the RR during
//     contention; caller must return this duplicate response immediately.
//   - (nil, nil, err): lock acquisition failed permanently (API error or
//     bounded-retry timeout exceeded); caller must return this error.
func (s *Server) acquireDistributedLockWithRetry(ctx context.Context, signal *types.NormalizedSignal) (func(), *ProcessingResponse, error) {
	logger := middleware.GetLogger(ctx)
	const maxRetries = 10 // 10 retries = ~2.5s total wait with exponential backoff

	// Configure shared backoff with jitter (pkg/shared/backoff)
	// ADR-052 Addendum 001: Use production-proven backoff from Notification v3.1
	backoffConfig := backoff.Config{
		BasePeriod:    100 * time.Millisecond, // Start at 100ms (proven in production)
		MaxPeriod:     1 * time.Second,        // Cap at 1s (faster than 30s lease expiry)
		Multiplier:    2.0,                    // Standard exponential (100ms → 200ms → 400ms → 800ms)
		JitterPercent: 10,                     // ±10% jitter (prevents thundering herd)
	}

	// Iterative retry loop with exponential backoff (replaces unbounded recursion)
	// ADR-052 Addendum 001: Prevents stack overflow risk from recursive retry
	for attempt := int32(1); attempt <= maxRetries; attempt++ {
		acquired, err := s.lockManager.AcquireLock(ctx, signal.Fingerprint)
		if err != nil {
			return nil, nil, fmt.Errorf("distributed lock acquisition failed: %w", err)
		}

		if acquired {
			// Lock acquired - exit retry loop and proceed with normal flow
			break
		}

		// Lock held by another Gateway pod
		logger.V(1).Info("Lock contention, retrying with exponential backoff",
			"attempt", attempt,
			"maxRetries", maxRetries,
			"fingerprint", signal.Fingerprint)

		// Check if we've exhausted all retries (early return for failure case)
		if attempt >= maxRetries {
			// Max retries exceeded - fail immediately
			return nil, nil, fmt.Errorf("lock acquisition timeout after %d attempts (fingerprint: %s)",
				maxRetries, signal.Fingerprint)
		}

		// Exponential backoff with jitter (shared implementation)
		backoffDuration := backoffConfig.Calculate(attempt)
		logger.V(2).Info("Backing off before retry",
			"backoff", backoffDuration,
			"attempt", attempt,
			"fingerprint", signal.Fingerprint)

		time.Sleep(backoffDuration)

		// Retry deduplication check (other pod may have created RR by now)
		// Issue #195: Use controllerNamespace — RRs live in controller NS per ADR-057
		shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(ctx, s.controllerNamespace, signal.Fingerprint)
		if err != nil {
			return nil, nil, fmt.Errorf("deduplication check failed after lock contention: %w", err)
		}

		if shouldDeduplicate && existingRR != nil {
			// BR-GATEWAY-190: Another pod created RR during lock contention
			// Handle deduplication and return early (no need to continue retry loop)
			response, dupErr := s.handleDuplicateSignal(ctx, signal, existingRR)
			return nil, response, dupErr
		}

		// Still no RR - continue to next retry attempt
	}

	// Lock acquired successfully - caller must release it after the operation
	releaseLock := func() {
		if err := s.lockManager.ReleaseLock(ctx, signal.Fingerprint); err != nil {
			logger.Error(err, "Failed to release distributed lock", "fingerprint", signal.Fingerprint)
		}
	}
	return releaseLock, nil, nil
}

// handleDuplicateSignal handles the case where another pod created a RemediationRequest during lock contention
// TDD REFACTOR: Extracted from ProcessSignal lock retry loop for clarity and testability
//
// BR-GATEWAY-190: Multi-replica deduplication safety
// ADR-052 Addendum 001: This helper is called when exponential backoff retry discovers
// that another Gateway pod successfully acquired the lock and created the RR.
//
// Business Outcome:
//   - Updates occurrence count for deduplication tracking
//   - Records metrics for alert deduplication monitoring
//   - Emits audit event for compliance and debugging
//   - Returns early from retry loop (no need to continue retrying)
//
// Returns:
//   - *ProcessingResponse: Duplicate response with existing RR reference
//   - error: Non-nil if status update or audit emission fails critically
func (s *Server) handleDuplicateSignal(ctx context.Context, signal *types.NormalizedSignal, existingRR *remediationv1alpha1.RemediationRequest) (*ProcessingResponse, error) {
	logger := middleware.GetLogger(ctx)

	// Update occurrence count for deduplication tracking
	if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR); err != nil {
		// Non-critical: Log and continue (deduplication still succeeded)
		logger.Info("Failed to update deduplication status after lock contention",
			"error", err,
			"fingerprint", signal.Fingerprint,
			"rr", existingRR.Name)
	}

	// Get updated occurrence count for metrics and audit
	occurrenceCount := int32(1)
	if existingRR.Status.Deduplication != nil {
		occurrenceCount = existingRR.Status.Deduplication.OccurrenceCount
	}

	// Record metrics for monitoring dashboard
	s.metricsInstance.AlertsDeduplicatedTotal.WithLabelValues(signal.SignalName).Inc()

	// Emit audit event for compliance (DD-AUDIT-003)
	s.emitSignalDeduplicatedAudit(ctx, signal, existingRR.Name, existingRR.Namespace, occurrenceCount)

	// Return duplicate response (early exit from retry loop)
	return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
}

// createRemediationRequestCRD handles the CRD creation pipeline
// TDD REFACTOR: Extracted from ProcessSignal for clarity
// Business Outcome: Consistent CRD creation (BR-004)
//
// Note: Environment, Priority, and RemediationPath removed from Gateway (2025-12-06)
// Signal Processing service now owns classification and path decision
// per DD-CATEGORIZATION-001 and DD-WORKFLOW-001 (risk_tolerance in CustomLabels)
func (s *Server) createRemediationRequestCRD(ctx context.Context, signal *types.NormalizedSignal, start time.Time) (*ProcessingResponse, error) {
	logger := middleware.GetLogger(ctx)

	// Create RemediationRequest CRD (classification and path moved to SP)
	rr, err := s.crdCreator.CreateRemediationRequest(ctx, signal)
	if err != nil {
		logger.Error(err, "Failed to create RemediationRequest CRD",
			"fingerprint", signal.Fingerprint)

		// DD-AUDIT-003: Emit crd.creation_failed audit event (DD-AUDIT-003)
		// Fire-and-forget: audit failures don't affect business logic
		s.emitCRDCreationFailedAudit(ctx, signal, err)

		return nil, fmt.Errorf("failed to create RemediationRequest CRD: %w", err)
	}

	// DD-GATEWAY-011: Initialize status.deduplication for NEW CRD
	// Gateway owns status.deduplication per DD-GATEWAY-011
	// Must initialize immediately after creation (OccurrenceCount=1, FirstSeenAt=now)
	if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, rr); err != nil {
		logger.Info("Failed to initialize deduplication status (DD-GATEWAY-011)",
			"error", err,
			"fingerprint", signal.Fingerprint,
			"rr", rr.Name)
		// Non-fatal: CRD exists, status update can be retried by RO or next duplicate
	}

	// DD-GATEWAY-011: Redis deduplication storage DEPRECATED
	// Deduplication now uses K8s status-based lookup (phaseChecker.ShouldDeduplicate)
	// and status updates (statusUpdater.UpdateDeduplicationStatus)
	// Redis is no longer used for deduplication state

	// DD-AUDIT-003: Emit signal.received audit event (BR-GATEWAY-190)
	// Fire-and-forget: audit failures don't affect business logic
	s.emitSignalReceivedAudit(ctx, signal, rr.Name, rr.Namespace)

	// DD-AUDIT-003: Emit crd.created audit event (DD-AUDIT-003)
	// Fire-and-forget: audit failures don't affect business logic
	s.emitCRDCreatedAudit(ctx, signal, rr.Name, rr.Namespace)

	// Record processing duration
	duration := time.Since(start)
	logger.Info("Signal processed successfully",
		"fingerprint", signal.Fingerprint,
		"crdName", rr.Name,
		"duration_ms", duration.Milliseconds())

	return NewCRDCreatedResponse(signal.Fingerprint, rr.Name, rr.Namespace), nil
}
