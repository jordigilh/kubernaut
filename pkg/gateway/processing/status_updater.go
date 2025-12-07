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

	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// ========================================
// DD-GATEWAY-011: Status Updater
// 📋 Design Decision: DD-GATEWAY-011 | ✅ Approved Design | Confidence: 90%
// See: docs/architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md
// ========================================
//
// StatusUpdater handles updates to RemediationRequest STATUS fields
// (not Spec, which is immutable after creation).
//
// OWNERSHIP (per DD-GATEWAY-011):
// - Gateway OWNS: status.deduplication, status.stormAggregation
// - RO OWNS: status.overallPhase, status.*Ref, status.timestamps
//
// CONFLICT HANDLING:
// - Uses retry.RetryOnConflict pattern for optimistic concurrency
// - Refetches RR before each update attempt to get latest resourceVersion
// ========================================

// StatusUpdater handles status updates to RemediationRequest CRDs
type StatusUpdater struct {
	client client.Client
}

// NewStatusUpdater creates a new status updater
func NewStatusUpdater(k8sClient client.Client) *StatusUpdater {
	return &StatusUpdater{
		client: k8sClient,
	}
}

// UpdateDeduplicationStatus updates the status.deduplication fields for a duplicate signal
//
// DD-GATEWAY-011: Status-Based Deduplication
// This method:
// 1. Refetches RR to get latest resourceVersion
// 2. Updates ONLY status.deduplication (Gateway-owned)
// 3. Uses Status().Update() to update status subresource
// 4. Retries on conflict (optimistic concurrency)
//
// Business Requirements:
// - BR-GATEWAY-181: Move deduplication tracking from spec to status
// - BR-GATEWAY-183: Implement optimistic concurrency for status updates
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - rr: RemediationRequest to update
//
// Returns:
// - error: K8s API errors (not found, timeout, etc.)
func (u *StatusUpdater) UpdateDeduplicationStatus(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
	// DD-GATEWAY-011: RED phase - return not implemented error
	// This will be implemented in Day 2 (GREEN phase)
	return fmt.Errorf("UpdateDeduplicationStatus not implemented yet (DD-GATEWAY-011 Day 1 RED phase)")
}

// UpdateStormAggregationStatus updates the status.stormAggregation fields
//
// DD-GATEWAY-011 + DD-GATEWAY-008 v2.0: Async Storm Aggregation
// This method updates storm tracking in RR status, enabling Redis deprecation.
//
// Business Requirements:
// - BR-GATEWAY-182: Move storm aggregation from Redis to status
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - rr: RemediationRequest to update
// - isThresholdReached: Whether storm threshold has been reached
//
// Returns:
// - error: K8s API errors
func (u *StatusUpdater) UpdateStormAggregationStatus(ctx context.Context, rr *remediationv1alpha1.RemediationRequest, isThresholdReached bool) error {
	// DD-GATEWAY-011: RED phase - return not implemented error
	// This will be implemented in Day 3 (GREEN phase)
	return fmt.Errorf("UpdateStormAggregationStatus not implemented yet (DD-GATEWAY-011 Day 3)")
}

