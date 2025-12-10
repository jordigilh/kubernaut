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
	client client.Client
}

// NewPhaseBasedDeduplicationChecker creates a new phase-based checker
func NewPhaseBasedDeduplicationChecker(k8sClient client.Client) *PhaseBasedDeduplicationChecker {
	return &PhaseBasedDeduplicationChecker{
		client: k8sClient,
	}
}

// ShouldDeduplicate checks if a signal should be deduplicated based on existing RR phase
//
// DD-GATEWAY-011 v1.3: Phase-Based Deduplication Decision
// This method:
// 1. Lists RRs matching the fingerprint label
// 2. Checks if any RR is in a non-terminal phase (including Blocked)
// 3. Returns true (deduplicate) if active RR exists
// 4. Returns false (allow new RR) if no active RR exists
//
// v1.3 SIMPLIFICATION:
// - Gateway does NOT count consecutive failures
// - Gateway does NOT create Blocked RRs
// - Gateway simply checks: "Is there an active RR?" → update dedup, else create new
//
// Business Requirements:
// - BR-GATEWAY-181: Move deduplication tracking to status
// - BR-GATEWAY-185: Redis deprecation path
// - BR-ORCH-042: Consecutive failure blocking (RO responsibility, NOT Gateway)
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - namespace: Namespace to search in
// - fingerprint: Signal fingerprint to match
//
// Returns:
// - bool: true if should deduplicate (in-progress RR exists)
// - *RemediationRequest: existing in-progress RR (nil if none)
// - error: K8s API errors
func (c *PhaseBasedDeduplicationChecker) ShouldDeduplicate(ctx context.Context, namespace, fingerprint string) (bool, *remediationv1alpha1.RemediationRequest, error) {
	// List RRs matching the fingerprint label
	rrList := &remediationv1alpha1.RemediationRequestList{}

	// Use label selector to find RRs with matching fingerprint
	// Label format: kubernaut.ai/signal-fingerprint (truncated to 63 chars for K8s label limit)
	labelFingerprint := fingerprint
	if len(labelFingerprint) > 63 {
		labelFingerprint = labelFingerprint[:63]
	}

	if err := c.client.List(ctx, rrList,
		client.InNamespace(namespace),
		client.MatchingLabels{"kubernaut.ai/signal-fingerprint": labelFingerprint},
	); err != nil {
		return false, nil, err
	}

	// Check each RR for non-terminal phase
	for i := range rrList.Items {
		rr := &rrList.Items[i]

		// Skip if in terminal phase (allow new RR creation)
		if IsTerminalPhase(rr.Status.OverallPhase) {
			continue
		}

		// Found in-progress RR → should deduplicate
		return true, rr, nil
	}

	// No in-progress RR found → allow new RR creation
	return false, nil, nil
}

// IsTerminalPhase checks if a phase is terminal (allows new RR creation)
//
// DD-GATEWAY-011 v1.3: Terminal phase classification
//
// TERMINAL (allow new RR creation):
// - Completed: Remediation succeeded
// - Failed: Remediation failed (including after Blocked→Failed transition)
// - Timeout: Remediation timed out
//
// NON-TERMINAL (deduplicate → update status):
// - Pending, Processing, Analyzing, Approving, Executing, Recovering
// - Blocked: RO holds for cooldown, Gateway updates dedup status (prevents RR flood)
//
// WHITELIST approach (safer than blacklist):
// - Only explicitly terminal phases allow new RR
// - ALL other phases (including Blocked and unknown future phases) are non-terminal
func IsTerminalPhase(phase string) bool {
	switch phase {
	case "Completed", "Failed", "Timeout":
		return true
	default:
		return false
	}
}
