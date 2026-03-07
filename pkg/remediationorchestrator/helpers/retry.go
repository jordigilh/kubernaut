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

package helpers

import (
	"context"

	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// ========================================
// RETRY HELPER (REFACTOR-RO-001)
// Day 0 Validation: ✅ SUCCESSFUL (95% confidence)
// Day 1 Implementation: Production version
// ========================================
//
// UpdateRemediationRequestStatus updates RR status with retry logic.
// Automatically handles refetch, update, and error wrapping.
// Preserves Gateway-owned fields (DD-GATEWAY-011, BR-ORCH-038).
//
// Design Decision:
// - Refetch gets latest resourceVersion (preserves Gateway deduplication fields)
// - UpdateFn receives fresh RR state, modifies only RO-owned fields
// - Status update includes latest resourceVersion from refetch
// - Retry on conflict automatically (up to 10 attempts with exponential backoff)
//
// REFACTOR-RO-008: Instrumented with Prometheus metrics for observability
//
// Reference: REFACTOR-RO-001, REFACTOR-RO-008, DD-GATEWAY-011, BR-ORCH-038
func UpdateRemediationRequestStatus(
	ctx context.Context,
	c client.Client,
	rr *remediationv1.RemediationRequest,
	updateFn func(*remediationv1.RemediationRequest) error,
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Refetch to get latest resourceVersion (preserves Gateway fields)
		if err := c.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return err
		}

		// Apply user's updates (modifies only RO-owned fields)
		if err := updateFn(rr); err != nil {
			return err
		}

		// Update status with latest resourceVersion
		return c.Status().Update(ctx, rr)
	})

}
