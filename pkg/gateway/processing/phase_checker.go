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
// DD-GATEWAY-011: Phase-Based Deduplication Checker
// 📋 Design Decision: DD-GATEWAY-011 | ✅ Approved Design | Confidence: 90%
// See: docs/architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md
// ========================================
//
// PhaseBasedDeduplicationChecker determines if a signal should be deduplicated
// based on the phase of existing RemediationRequest CRDs.
//
// TERMINAL PHASES (allow new RR):
// - Completed: Remediation succeeded
// - Failed: Remediation failed
// - Cancelled: User cancelled
//
// IN-PROGRESS PHASES (deduplicate):
// - Pending, Processing, and any other non-terminal phase
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
// DD-GATEWAY-011: Phase-Based Deduplication Decision
// This method:
// 1. Lists RRs matching the fingerprint label
// 2. Checks if any RR is in a non-terminal phase
// 3. Returns true (deduplicate) if in-progress RR exists
// 4. Returns false (allow new RR) if no in-progress RR exists
//
// Business Requirements:
// - BR-GATEWAY-184: Check RR phase for deduplication decisions
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
// WHITELIST approach (safer than blacklist):
// - Completed, Failed, Cancelled are terminal
// - ALL other phases (including unknown future phases) are treated as in-progress
func IsTerminalPhase(phase string) bool {
	switch phase {
	case "Completed", "Failed", "Cancelled":
		return true
	default:
		return false
	}
}
