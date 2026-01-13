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
//
// DD-STATUS-001: Cache-Bypassed Reads (Adopted from RO)
// - Uses apiReader for refetches to bypass controller-runtime cache
// - Prevents "not found" errors when reading immediately after CRD creation
// - Ensures fresh reads directly from K8s API server for optimistic locking
// ========================================

// StatusUpdater handles status updates to RemediationRequest CRDs
type StatusUpdater struct {
	client    client.Client
	apiReader client.Reader // DD-STATUS-001: Cache-bypassed reads for fresh status
}

// NewStatusUpdater creates a new status updater
// apiReader bypasses controller-runtime cache for optimistic locking refetches (DD-STATUS-001)
func NewStatusUpdater(k8sClient client.Client, apiReader client.Reader) *StatusUpdater {
	return &StatusUpdater{
		client:    k8sClient,
		apiReader: apiReader,
	}
}

// GatewayRetryBackoff is the retry configuration for Gateway status updates.
// DD-GATEWAY-011: Optimized for Gateway latency requirements (P95 <50ms)
var GatewayRetryBackoff = retry.DefaultBackoff

// UpdateDeduplicationStatus updates the status.deduplication fields for a duplicate signal
//
// DD-GATEWAY-011: Status-Based Deduplication
// This method:
// 1. Refetches RR to get latest resourceVersion (using apiReader for cache-bypassed read)
// 2. Updates ONLY status.deduplication (Gateway-owned)
// 3. Uses Status().Update() to update status subresource
// 4. Retries on conflict (optimistic concurrency)
//
// DD-STATUS-001: Uses apiReader to bypass controller-runtime cache
// This prevents "not found" errors when Gateway reads immediately after CRD creation,
// as the cached client may not have synced yet. Direct API server reads are always fresh.
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
		// DD-STATUS-001: Use apiReader to bypass controller-runtime cache for fresh read
		// This prevents "not found" errors immediately after CRD creation
		if err := u.apiReader.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
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
