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
	"crypto/sha256"
	"fmt"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DistributedLockManager manages K8s Lease-based distributed locks for multi-replica Gateway safety.
//
// Business Requirement: BR-GATEWAY-190 (Multi-Replica Deduplication Safety)
// Design Decision: ADR-052 (K8s Lease-Based Distributed Locking Pattern)
// API Client Choice: Uses apiReader (non-cached client) for immediate consistency
//
// Purpose: Prevents duplicate RemediationRequest CRD creation when multiple Gateway pods
// process the same signal concurrently (fixes GW-DEDUP-002 race condition).
//
// Pattern: Uses Kubernetes coordination.k8s.io/v1 Lease resources for distributed locking.
// This ensures only 1 pod can acquire a lock for a given fingerprint at a time.
//
// WHY apiReader (Non-Cached Client)?
// - ✅ Immediate Consistency: No cache sync delay (5-50ms race window eliminated)
// - ✅ Correctness: Distributed locking requires guaranteed freshness
// - ✅ Production-Ready: K8s leader-election uses direct API calls for Lease operations
//
// API Server Impact: Acceptable at production scale
// - Normal load (1 signal/sec): 3 API req/sec (negligible)
// - Peak load (8 signals/sec): 24 API req/sec (low)
// - Design target (1000 signals/sec): 3000 API req/sec (30-60% of K8s API capacity)
//
// See: docs/services/stateless/gateway-service/GW_API_SERVER_IMPACT_ANALYSIS_DISTRIBUTED_LOCKING_JAN18_2026.md
//
// Lock Lifecycle:
//  1. AcquireLock(fingerprint) - Creates or claims Lease
//  2. Process signal (create CRD)
//  3. ReleaseLock(fingerprint) - Deletes Lease
//
// Fault Tolerance: Expired leases (from crashed pods) can be taken over after 30s.
type DistributedLockManager struct {
	client    client.Client // Uses apiReader (non-cached) in production for immediate consistency
	namespace string
	holderID  string
}

const (
	// LockDurationSeconds defines how long a lock is valid before it expires.
	// If a pod crashes without releasing, another pod can take over after 30s.
	LockDurationSeconds = 30
)

// NewDistributedLockManager creates a new distributed lock manager.
//
// Parameters:
//   - client: K8s client for Lease operations (MUST be apiReader/non-cached for immediate consistency)
//   - namespace: Namespace where Leases will be created (typically "kubernaut-system")
//   - holderID: Unique identifier for this pod (typically POD_NAME from env)
//
// IMPORTANT: Pass apiReader (non-cached client) to avoid race conditions.
// Cached clients have 5-50ms sync delay, allowing duplicate lock acquisitions.
//
// Example (Production):
//
//	lockManager := NewDistributedLockManager(apiReader, "kubernaut-system", "gateway-pod-1")
//
// Example (Testing):
//
//	lockManager := NewDistributedLockManager(k8sClient, "default", "test-pod")
func NewDistributedLockManager(k8sClient client.Client, namespace, holderID string) *DistributedLockManager {
	return &DistributedLockManager{
		client:    k8sClient,
		namespace: namespace,
		holderID:  holderID,
	}
}

// AcquireLock attempts to acquire a distributed lock for the given fingerprint.
//
// Returns:
//   - (true, nil) if lock was acquired
//   - (false, nil) if lock is held by another pod (contention - not an error)
//   - (false, err) if K8s API error occurred
//
// Lock Acquisition Flow:
//  1. Try to Get existing Lease
//  2. If not found, Create new Lease (acquire lock)
//  3. If found and held by us, return true (idempotent)
//  4. If found and held by another pod, check if expired
//  5. If expired, Update Lease to take over
//  6. If not expired, return false (contention)
//
// Example:
//
//	acquired, err := lockManager.AcquireLock(ctx, signal.Fingerprint)
//	if err != nil {
//	    return fmt.Errorf("lock acquisition failed: %w", err)
//	}
//	if !acquired {
//	    // Lock held by another pod - retry or return
//	    return nil
//	}
//	defer lockManager.ReleaseLock(ctx, signal.Fingerprint)
func (m *DistributedLockManager) AcquireLock(ctx context.Context, fingerprint string) (bool, error) {
	leaseName := generateLeaseName(fingerprint)

	lease := &coordinationv1.Lease{}
	err := m.client.Get(ctx, client.ObjectKey{
		Namespace: m.namespace,
		Name:      leaseName,
	}, lease)

	if err != nil {
		if !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to check for existing lease: %w", err)
		}

		// Lease doesn't exist - create new lease and acquire lock
		now := metav1.NowMicro()
		leaseDuration := int32(LockDurationSeconds)

		lease = &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      leaseName,
				Namespace: m.namespace,
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       &m.holderID,
				LeaseDurationSeconds: &leaseDuration,
				AcquireTime:          &now,
				RenewTime:            &now,
			},
		}

		if err := m.client.Create(ctx, lease); err != nil {
			if apierrors.IsAlreadyExists(err) {
				// Race condition: another pod created lease between Get and Create
				// This is contention, not an error
				return false, nil
			}
			return false, fmt.Errorf("failed to create lease: %w", err)
		}

		return true, nil
	}

	// Lease exists - check if we own it
	if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity == m.holderID {
		// We already own this lease (idempotent)
		return true, nil
	}

	// Lease held by another pod - check if expired
	if lease.Spec.RenewTime != nil && lease.Spec.LeaseDurationSeconds != nil {
		expiry := lease.Spec.RenewTime.Add(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second)
		if time.Now().After(expiry) {
			// Lease expired - take it over
			now := metav1.NowMicro()
			lease.Spec.HolderIdentity = &m.holderID
			lease.Spec.RenewTime = &now

			if err := m.client.Update(ctx, lease); err != nil {
				if apierrors.IsConflict(err) {
					// Race condition: another pod updated lease between Get and Update
					// This is contention, not an error
					return false, nil
				}
				return false, fmt.Errorf("failed to take over expired lease: %w", err)
			}

			return true, nil
		}
	}

	// Lease held by another pod and not expired - lock contention
	return false, nil
}

// ReleaseLock releases the distributed lock by deleting the Lease.
//
// This method is idempotent - it's safe to call multiple times (e.g., in defer).
// If the Lease doesn't exist (already deleted), no error is returned.
//
// Returns:
//   - nil on success
//   - error if K8s API error occurred (excluding NotFound)
//
// Example:
//
//	defer func() {
//	    if err := lockManager.ReleaseLock(ctx, signal.Fingerprint); err != nil {
//	        logger.Error(err, "Failed to release lock")
//	    }
//	}()
func (m *DistributedLockManager) ReleaseLock(ctx context.Context, fingerprint string) error {
	leaseName := generateLeaseName(fingerprint)

	lease := &coordinationv1.Lease{}
	lease.Namespace = m.namespace
	lease.Name = leaseName

	err := m.client.Delete(ctx, lease)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete lease: %w", err)
	}

	return nil
}

// generateLeaseName creates a K8s-compliant lease name from a fingerprint.
//
// Lease Name Format: "gw-lock-{hash-prefix}"
// - Uses first 16 chars of fingerprint (or hash if fingerprint is short)
// - Complies with K8s 63-char limit
// - Ensures uniqueness across different fingerprints
//
// Example:
//
//	fingerprint := "cb639afcefc1341a46b82d7cfdbb022195e8848acb6bc3e70e9917dd02353966"
//	leaseName := generateLeaseName(fingerprint) // "gw-lock-cb639afcefc1341a"
func generateLeaseName(fingerprint string) string {
	// For short fingerprints, hash them to ensure uniqueness
	if len(fingerprint) < 16 {
		hash := sha256.Sum256([]byte(fingerprint))
		fingerprint = fmt.Sprintf("%x", hash)
	}

	// Use first 16 chars of fingerprint as lease name suffix
	// "gw-lock-" (8 chars) + 16 chars = 24 chars total (well under 63-char K8s limit)
	return fmt.Sprintf("gw-lock-%s", fingerprint[:16])
}
