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

package processing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// ========================================
// DD-GATEWAY-011 v1.3: Phase-Based Deduplication Checker
// 📋 Design Decision: DD-GATEWAY-011 | ✅ Approved Design | Confidence: 95%
// See: docs/architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md
// ========================================
//
// PhaseBasedDeduplicationChecker determines if a signal should be deduplicated
// based on the phase of existing RemediationRequest CRDs.
//
// v1.3 CHANGES (2025-12-10):
// - Gateway does NOT count consecutive failures (moved to RO per BR-ORCH-042)
// - Gateway does NOT create RR with phase=Blocked
// - Blocked is non-terminal (RO owns blocking logic with cooldown)
//
// TERMINAL PHASES (allow new RR creation):
// - Completed: Remediation succeeded
// - Failed: Remediation failed (including after cooldown)
// - Timeout: Remediation timed out
//
// NON-TERMINAL PHASES (deduplicate → update status):
// - Pending, Processing, Analyzing, Approving, Executing, Recovering
// - Blocked: RO holds signal for cooldown, Gateway updates dedup status
// ========================================

// PhaseBasedDeduplicationChecker checks for existing in-progress RRs by fingerprint
type PhaseBasedDeduplicationChecker struct {
	client         client.Reader // Changed to Reader (can be apiReader or ctrlClient)
	cooldownPeriod time.Duration
}

// NewPhaseBasedDeduplicationChecker creates a new phase-based checker.
// DD-GATEWAY-011: Accepts client.Reader to allow apiReader (cache-bypassed) for race-free deduplication.
// cooldownPeriod controls how long after a successful remediation new signals are suppressed.
// Set to 0 to disable post-completion cooldown.
func NewPhaseBasedDeduplicationChecker(k8sClient client.Reader, cooldownPeriod time.Duration) *PhaseBasedDeduplicationChecker {
	return &PhaseBasedDeduplicationChecker{
		client:         k8sClient,
		cooldownPeriod: cooldownPeriod,
	}
}

// ShouldDeduplicate checks if a signal should be deduplicated based on existing RR phase
//
// DD-GATEWAY-011 v1.3: Phase-Based Deduplication Decision
// This method:
// 1. Lists RRs matching the fingerprint via field selector (BR-GATEWAY-185 v1.1)
// 2. Checks if any RR is in a non-terminal phase (including Blocked)
// 3. Returns true (deduplicate) if active RR exists
// 4. Returns false (allow new RR) if no active RR exists
//
// v1.3 SIMPLIFICATION:
// - Gateway does NOT count consecutive failures
// - Gateway does NOT create Blocked RRs
// - Gateway simply checks: "Is there an active RR?" → update dedup, else create new
//
// BR-GATEWAY-185 v1.1: Use spec.signalFingerprint field selector instead of labels
// - Labels are mutable and truncated to 63 chars (data loss risk)
// - spec.signalFingerprint is immutable and supports full 64-char SHA256
//
// Business Requirements:
// - BR-GATEWAY-181: Move deduplication tracking to status
// - BR-GATEWAY-185: Field selector for fingerprint lookup (v1.1)
// - BR-ORCH-042: Consecutive failure blocking (RO responsibility, NOT Gateway)
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - namespace: Namespace to search in
// - fingerprint: Signal fingerprint to match (full 64-char SHA256)
//
// Returns:
// - bool: true if should deduplicate (in-progress RR exists)
// - *RemediationRequest: existing in-progress RR (nil if none)
// - error: K8s API errors
func (c *PhaseBasedDeduplicationChecker) ShouldDeduplicate(ctx context.Context, namespace, fingerprint string) (bool, *remediationv1alpha1.RemediationRequest, error) {
	// List RRs matching the fingerprint via field selector (BR-GATEWAY-185 v1.1)
	// NO truncation - uses full 64-char SHA256 fingerprint
	rrList := &remediationv1alpha1.RemediationRequestList{}

	err := c.client.List(ctx, rrList,
		client.InNamespace(namespace),
		client.MatchingFields{"spec.signalFingerprint": fingerprint},
	)

	// FALLBACK: If field selector not supported (e.g., in tests without field index),
	// list all RRs in namespace and filter in-memory
	// This is less efficient but ensures tests work without cached client setup
	if err != nil && (strings.Contains(err.Error(), "field label not supported") || strings.Contains(err.Error(), "field selector")) {
		// Fall back to listing all RRs and filtering in-memory
		if err := c.client.List(ctx, rrList, client.InNamespace(namespace)); err != nil {
			return false, nil, fmt.Errorf("deduplication check failed: %w", err)
		}

		// Filter by fingerprint in-memory
		filteredItems := []remediationv1alpha1.RemediationRequest{}
		for i := range rrList.Items {
			if rrList.Items[i].Spec.SignalFingerprint == fingerprint {
				filteredItems = append(filteredItems, rrList.Items[i])
			}
		}
		rrList.Items = filteredItems
	} else if err != nil {
		return false, nil, fmt.Errorf("deduplication check failed: %w", err)
	}

	// Check each RR: non-terminal phases always deduplicate, Completed RRs within
	// cooldown also deduplicate to prevent stale signals reaching the LLM pipeline.
	var mostRecentCooldownRR *remediationv1alpha1.RemediationRequest

	for i := range rrList.Items {
		rr := &rrList.Items[i]

		if !IsTerminalPhase(rr.Status.OverallPhase) {
			return true, rr, nil
		}

		// Post-completion cooldown: only Completed (not Failed/TimedOut/Skipped/Cancelled)
		if c.cooldownPeriod > 0 &&
			rr.Status.OverallPhase == remediationv1alpha1.PhaseCompleted &&
			rr.Status.CompletedAt != nil &&
			time.Since(rr.Status.CompletedAt.Time) < c.cooldownPeriod {
			if mostRecentCooldownRR == nil ||
				rr.Status.CompletedAt.Time.After(mostRecentCooldownRR.Status.CompletedAt.Time) {
				mostRecentCooldownRR = rr
			}
		}
	}

	if mostRecentCooldownRR != nil {
		return true, mostRecentCooldownRR, nil
	}

	return false, nil, nil
}

// IsTerminalPhase checks if a RemediationRequest phase is terminal.
// Terminal phases allow new RR creation for the same signal fingerprint.
//
// DD-GATEWAY-011 v1.3: Terminal phase classification
// DD-GATEWAY-009: Cancelled state handling (allows retry)
//
// TERMINAL (allow new RR creation):
// - Completed: Remediation succeeded
// - Failed: Remediation failed (including after Blocked→Failed transition)
// - TimedOut: Remediation timed out
// - Skipped: Remediation was not needed (per BR-ORCH-032)
// - Cancelled: Remediation was manually cancelled (per DD-GATEWAY-009, allows retry)
//
// NON-TERMINAL (deduplicate → update status):
// - Pending, Processing, Analyzing, AwaitingApproval, Executing
// - Blocked: RO holds for cooldown, Gateway updates dedup status (prevents RR flood)
//
// WHITELIST approach (safer than blacklist):
// - Only explicitly terminal phases allow new RR
// - ALL other phases (including Blocked and unknown future phases) are non-terminal
//
// Phase values per api/remediation/v1alpha1/remediationrequest_types.go:
// - Terminal: Completed, Failed, TimedOut, Skipped, Cancelled
// - Non-Terminal: Pending, Processing, Analyzing, AwaitingApproval, Executing, Blocked
//
// 🏛️ Compliance: BR-COMMON-001 (Phase Format), Viceversa Pattern (Cross-Service Consumption)
// See: docs/handoff/TEAM_NOTIFICATION_GATEWAY_PHASE_COMPLIANCE.md
func IsTerminalPhase(phase remediationv1alpha1.RemediationPhase) bool {
	switch phase {
	case remediationv1alpha1.PhaseCompleted,
		remediationv1alpha1.PhaseFailed,
		remediationv1alpha1.PhaseTimedOut,
		remediationv1alpha1.PhaseSkipped,
		remediationv1alpha1.PhaseCancelled:
		return true
	default:
		return false
	}
}
