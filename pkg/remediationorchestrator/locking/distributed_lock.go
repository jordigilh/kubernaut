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

// Package locking provides distributed lock management for the Remediation Orchestrator.
//
// BR-ORCH-025: Prevents duplicate WorkflowExecution creation when multiple RO pods
// or concurrent reconciles target the same resource.
//
// Pattern: Kubernetes coordination.k8s.io/v1 Lease-based locking, adapted from
// Gateway's DistributedLockManager (pkg/gateway/processing/distributed_lock.go)
// with RO-specific prefix and target-resource keying.
package locking

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// LockDurationSeconds defines how long a lock is valid before it expires.
	// If a pod crashes without releasing, another pod can take over after this period.
	LockDurationSeconds = 30
)

// DistributedLockManager manages K8s Lease-based distributed locks for RO WFE creation safety.
type DistributedLockManager struct {
	client    client.Client
	namespace string
	holderID  string
}

// NewDistributedLockManager creates a new distributed lock manager for RO.
//
// Parameters:
//   - k8sClient: K8s client for Lease operations (prefer apiReader/non-cached for consistency)
//   - namespace: Namespace where Leases will be created (typically "kubernaut-system")
//   - holderID: Unique identifier for this pod (typically POD_NAME from env)
func NewDistributedLockManager(k8sClient client.Client, namespace, holderID string) *DistributedLockManager {
	return &DistributedLockManager{
		client:    k8sClient,
		namespace: namespace,
		holderID:  holderID,
	}
}

// AcquireLock attempts to acquire a distributed lock for the given target resource.
//
// Returns:
//   - (true, nil) if lock was acquired
//   - (false, nil) if lock is held by another pod (contention)
//   - (false, err) if K8s API error occurred
func (m *DistributedLockManager) AcquireLock(ctx context.Context, targetResource string) (bool, error) {
	logger := log.FromContext(ctx)
	leaseName := GenerateLeaseName(targetResource)

	lease := &coordinationv1.Lease{}
	err := m.client.Get(ctx, client.ObjectKey{
		Namespace: m.namespace,
		Name:      leaseName,
	}, lease)

	if err != nil {
		if !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to check for existing lease: %w", err)
		}

		// Lease doesn't exist — create new lease and acquire lock
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
				return false, nil
			}
			return false, fmt.Errorf("failed to create lease: %w", err)
		}

		return true, nil
	}

	// Lease exists — check if we own it (idempotent re-acquire)
	if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity == m.holderID {
		logger.V(2).Info("Re-acquired own lease (idempotent)", "target", targetResource, "lease", leaseName)
		return true, nil
	}

	// Lease held by another pod — check if expired
	if lease.Spec.RenewTime != nil && lease.Spec.LeaseDurationSeconds != nil {
		expiry := lease.Spec.RenewTime.Add(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second)
		if time.Now().After(expiry) {
			now := metav1.NowMicro()
			lease.Spec.HolderIdentity = &m.holderID
			lease.Spec.RenewTime = &now

			if err := m.client.Update(ctx, lease); err != nil {
				if apierrors.IsConflict(err) {
					return false, nil
				}
				return false, fmt.Errorf("failed to take over expired lease: %w", err)
			}

			return true, nil
		}
	}

	// Lease held by another pod and not expired (or nil RenewTime = ambiguous)
	holder := "<unknown>"
	if lease.Spec.HolderIdentity != nil {
		holder = *lease.Spec.HolderIdentity
	}
	logger.V(1).Info("Lock contention — lease held by another pod",
		"target", targetResource, "lease", leaseName, "holder", holder)
	return false, nil
}

// ReleaseLock releases the distributed lock for the given target resource.
// Idempotent: safe to call multiple times (e.g., in defer).
func (m *DistributedLockManager) ReleaseLock(ctx context.Context, targetResource string) error {
	leaseName := GenerateLeaseName(targetResource)

	lease := &coordinationv1.Lease{}
	lease.Namespace = m.namespace
	lease.Name = leaseName

	err := m.client.Delete(ctx, lease)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete lease: %w", err)
	}

	return nil
}

// GenerateLeaseName creates a K8s-compliant lease name from a target resource string.
// Uses "ro-lock-" prefix + first 16 chars of SHA256 hash for DNS compliance and uniqueness.
func GenerateLeaseName(targetResource string) string {
	hash := sha256.Sum256([]byte(targetResource))
	hashHex := fmt.Sprintf("%x", hash)
	return fmt.Sprintf("ro-lock-%s", hashHex[:16])
}
