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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
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

// GatewayRetryBackoff is the retry configuration for Gateway status updates.
// DD-GATEWAY-011: Optimized for Gateway latency requirements (P95 <50ms)
var GatewayRetryBackoff = retry.DefaultBackoff

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
	return retry.RetryOnConflict(GatewayRetryBackoff, func() error {
		// Refetch to get latest resourceVersion
		if err := u.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return err
		}

		// Update ONLY Gateway-owned status.deduplication fields
		now := metav1.Now()
		if rr.Status.Deduplication == nil {
			// Initialize deduplication status on first update
			rr.Status.Deduplication = &remediationv1alpha1.DeduplicationStatus{
				FirstSeenAt:     &now,
				OccurrenceCount: 1,
			}
		} else {
			// Increment occurrence count for duplicate
			rr.Status.Deduplication.OccurrenceCount++
		}
		rr.Status.Deduplication.LastSeenAt = &now

		// Use Status().Update() to update only the status subresource
		return u.client.Status().Update(ctx, rr)
	})
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
// - threshold: Storm threshold from config (default: 5)
//
// Returns:
// - error: K8s API errors
func (u *StatusUpdater) UpdateStormAggregationStatus(ctx context.Context, rr *remediationv1alpha1.RemediationRequest, isThresholdReached bool, threshold int32) error {
	// DD-GATEWAY-011 Day 3: RED phase stub - return not implemented error
	// This will be implemented following strict TDD after tests are written
	return fmt.Errorf("UpdateStormAggregationStatus not implemented yet (DD-GATEWAY-011 Day 3 RED phase)")
}
