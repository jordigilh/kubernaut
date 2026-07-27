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

package processing_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	gatewayPodCrashed = "gateway-pod-crashed"
)

// ========================================
// TDD RED PHASE: Unit Tests for DistributedLockManager
// Business Requirements: BR-GATEWAY-190 (Multi-Replica Deduplication Safety)
// Design Decision: ADR-052 (K8s Lease-Based Distributed Locking)
// ========================================
//
// These tests define the business contract for DistributedLockManager.
// They will FAIL until the implementation is complete (TDD GREEN phase).
//
// Test Coverage:
// - Lock acquisition (new lease, expired lease, idempotent)
// - Lock release (success, idempotent)
// - Lock contention (held by another pod)
// - K8s API errors (permission denied, API unavailable)
// - Edge cases (long fingerprints, lease name truncation)
//
// Testing Strategy:
// - Use fake K8s client for unit tests (no real K8s cluster)
// - Test business logic, not K8s API behavior
// - Mock external dependencies only (K8s API is mocked via fake client)
// ========================================

var _ = Describe("DistributedLockManager", func() {
	var (
		ctx         context.Context
		k8sClient   client.Client
		scheme      *runtime.Scheme
		lockManager *processing.DistributedLockManager
		namespace   string
		holderID    string
		fingerprint string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "kubernaut-system"
		holderID = "gateway-pod-1"
		fingerprint = "cb639afcefc1341a46b82d7cfdbb022195e8848acb6bc3e70e9917dd02353966" // 64-char SHA256

		// Create scheme with Lease type
		scheme = runtime.NewScheme()
		Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())

		// Create fake K8s client
		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		// Create lock manager
		lockManager = processing.NewDistributedLockManager(k8sClient, namespace, holderID)
	})

	Describe("NewDistributedLockManager", func() {
		It("should create a new lock manager with provided parameters", func() {
			// BR-GATEWAY-190: Lock manager must be initialized with K8s client, namespace, and pod identity
			Expect(lockManager).NotTo(BeNil(),
				"Lock manager should be initialized")
		})
	})

	Describe("AcquireLock", func() {
		Context("when lease doesn't exist", func() {
			It("should create lease and acquire lock successfully", func() {
				// TDD: Define expected behavior for new lease acquisition
				// BR-GATEWAY-190: First request for a fingerprint should acquire lock

				// When: Attempt to acquire lock
				acquired, err := lockManager.AcquireLock(ctx, fingerprint)

				// Then: Lock should be acquired
				Expect(err).NotTo(HaveOccurred(),
					"Lock acquisition should succeed when lease doesn't exist")
				Expect(acquired).To(BeTrue(),
					"Lock should be acquired when lease doesn't exist")

				// And: Lease should exist in K8s
				leaseName := "gw-lock-" + fingerprint[:16]
				lease := &coordinationv1.Lease{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Namespace: namespace,
					Name:      leaseName,
				}, lease)
				Expect(err).NotTo(HaveOccurred(),
					"Lease should be created in K8s")
				Expect(*lease.Spec.HolderIdentity).To(Equal(holderID),
					"Lease holder should be our pod")
			})
		})

		Context("when lease exists and held by us", func() {
			BeforeEach(func() {
				// Given: Lease exists and is held by us
				leaseName := "gw-lock-" + fingerprint[:16]
				leaseDuration := int32(30)
				now := metav1.NowMicro()

				lease := &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      leaseName,
						Namespace: namespace,
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity:       &holderID,
						LeaseDurationSeconds: &leaseDuration,
						RenewTime:            &now,
					},
				}
				Expect(k8sClient.Create(ctx, lease)).To(Succeed())
			})

			It("should return true (idempotent)", func() {
				// TDD: Define idempotent behavior
				// BR-GATEWAY-190: Re-acquiring our own lock should succeed (safe to call multiple times)

				// When: Attempt to acquire lock we already hold
				acquired, err := lockManager.AcquireLock(ctx, fingerprint)

				// Then: Should succeed (idempotent)
				Expect(err).NotTo(HaveOccurred(),
					"Re-acquiring our own lock should not error")
				Expect(acquired).To(BeTrue(),
					"Re-acquiring our own lock should return true (idempotent)")
			})
		})

		Context("when lease exists and held by another pod", func() {
			BeforeEach(func() {
				// Given: Lease exists and is held by another pod
				leaseName := "gw-lock-" + fingerprint[:16]
				leaseDuration := int32(30)
				now := metav1.NowMicro()
				otherPodID := "gateway-pod-2"

				lease := &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      leaseName,
						Namespace: namespace,
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity:       &otherPodID,
						LeaseDurationSeconds: &leaseDuration,
						RenewTime:            &now,
					},
				}
				Expect(k8sClient.Create(ctx, lease)).To(Succeed())
			})

			It("should return false (lock contention)", func() {
				// TDD: Define lock contention behavior
				// BR-GATEWAY-190: Cannot acquire lock held by another pod (not an error)

				// When: Attempt to acquire lock held by another pod
				acquired, err := lockManager.AcquireLock(ctx, fingerprint)

				// Then: Should not acquire lock (contention)
				Expect(err).NotTo(HaveOccurred(),
					"Lock contention is not an error, just means lock not acquired")
				Expect(acquired).To(BeFalse(),
					"Lock should not be acquired when held by another pod")
			})
		})

		Context("when lease exists and is expired", func() {
			BeforeEach(func() {
				// Given: Lease exists but expired (held by crashed pod)
				leaseName := "gw-lock-" + fingerprint[:16]
				leaseDuration := int32(30)
				// Set renew time to 35 seconds ago (expired)
				expiredTime := metav1.NewMicroTime(time.Now().Add(-35 * time.Second))
				otherPodID := gatewayPodCrashed

				lease := &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      leaseName,
						Namespace: namespace,
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity:       &otherPodID,
						LeaseDurationSeconds: &leaseDuration,
						RenewTime:            &expiredTime,
					},
				}
				Expect(k8sClient.Create(ctx, lease)).To(Succeed())
			})

			It("should take over expired lease and acquire lock", func() {
				// TDD: Define expired lease takeover behavior
				// BR-GATEWAY-190: Expired leases from crashed pods should be taken over

				// When: Attempt to acquire expired lease
				acquired, err := lockManager.AcquireLock(ctx, fingerprint)

				// Then: Should acquire lock by taking over expired lease
				Expect(err).NotTo(HaveOccurred(),
					"Taking over expired lease should succeed")
				Expect(acquired).To(BeTrue(),
					"Lock should be acquired by taking over expired lease")

				// And: Lease holder should now be us
				leaseName := "gw-lock-" + fingerprint[:16]
				lease := &coordinationv1.Lease{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Namespace: namespace,
					Name:      leaseName,
				}, lease)
				Expect(err).NotTo(HaveOccurred())
				Expect(*lease.Spec.HolderIdentity).To(Equal(holderID),
					"Lease holder should be updated to our pod after takeover")
			})
		})

		Context("when Get fails with a non-NotFound API error", func() {
			It("should propagate the error instead of treating it as a missing lease", func() {
				// BR-GATEWAY-190: Wave 6 RED-phase characterization test.
				// Branch-check discovered this K8s API error path (line 133) had 0%
				// UT+IT coverage. Distinguishing "lease truly absent" (NotFound, create
				// path) from "API unavailable" (any other error) is safety-critical:
				// silently treating an API outage as "no lease" would let two pods
				// both believe they hold the lock.
				errClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							return apierrors.NewServiceUnavailable("etcd unavailable")
						},
					}).
					Build()
				erroringManager := processing.NewDistributedLockManager(errClient, namespace, holderID)

				acquired, err := erroringManager.AcquireLock(ctx, fingerprint)

				Expect(err).To(HaveOccurred(),
					"BR-GATEWAY-190: API errors must propagate, not be swallowed as 'lease absent'")
				Expect(err.Error()).To(ContainSubstring("failed to check for existing lease"))
				Expect(acquired).To(BeFalse(),
					"Lock must not be reported as acquired when the existence check failed")
			})
		})

		Context("when Create races with another pod (AlreadyExists)", func() {
			It("should treat the race as contention, not an error", func() {
				// BR-GATEWAY-190 / ADR-052: Two Gateway pods may both observe "lease
				// absent" and race to Create it. The loser must see AlreadyExists and
				// report ordinary contention (acquired=false, err=nil) so its retry
				// loop backs off and re-checks, rather than surfacing a false failure.
				raceClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
							return apierrors.NewAlreadyExists(schema.GroupResource{Resource: "leases"}, "gw-lock-race")
						},
					}).
					Build()
				racingManager := processing.NewDistributedLockManager(raceClient, namespace, holderID)

				acquired, err := racingManager.AcquireLock(ctx, fingerprint)

				Expect(err).NotTo(HaveOccurred(),
					"BR-GATEWAY-190: Create-race (AlreadyExists) is contention, not a hard error")
				Expect(acquired).To(BeFalse(),
					"Losing the create race means the lock was not acquired")
			})
		})

		Context("when Update races with another pod during expired-lease takeover (Conflict)", func() {
			It("should treat the takeover race as contention, not an error", func() {
				// BR-GATEWAY-190 / ADR-052: Two pods may both observe the same expired
				// lease and race to take it over. The loser's Update must fail with
				// Conflict; this must resolve to ordinary contention (acquired=false,
				// err=nil), not a propagated error that would abort the caller's flow.
				conflictClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
							return apierrors.NewConflict(schema.GroupResource{Resource: "leases"}, "gw-lock-race", errors.New("resource version mismatch"))
						},
					}).
					Build()

				leaseName := "gw-lock-" + fingerprint[:16]
				leaseDuration := int32(30)
				expiredTime := metav1.NewMicroTime(time.Now().Add(-35 * time.Second))
				otherPodID := gatewayPodCrashed
				lease := &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      leaseName,
						Namespace: namespace,
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity:       &otherPodID,
						LeaseDurationSeconds: &leaseDuration,
						RenewTime:            &expiredTime,
					},
				}
				Expect(conflictClient.Create(ctx, lease)).To(Succeed())

				conflictManager := processing.NewDistributedLockManager(conflictClient, namespace, holderID)

				acquired, err := conflictManager.AcquireLock(ctx, fingerprint)

				Expect(err).NotTo(HaveOccurred(),
					"BR-GATEWAY-190: Takeover-race (Conflict) is contention, not a hard error")
				Expect(acquired).To(BeFalse(),
					"Losing the takeover race means the lock was not acquired")
			})
		})

		Context("when Create fails with a non-AlreadyExists API error", func() {
			It("should propagate the error", func() {
				// BR-GATEWAY-190: Distinguish a genuine API failure (e.g. quota,
				// permission denied) from ordinary create-race contention. A hard
				// failure must surface as an error, not be silently swallowed as
				// "lock not acquired".
				errClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
							return apierrors.NewForbidden(schema.GroupResource{Resource: "leases"}, "gw-lock", errors.New("quota exceeded"))
						},
					}).
					Build()
				erroringManager := processing.NewDistributedLockManager(errClient, namespace, holderID)

				acquired, err := erroringManager.AcquireLock(ctx, fingerprint)

				Expect(err).To(HaveOccurred(),
					"BR-GATEWAY-190: Non-race Create failures must propagate as errors")
				Expect(err.Error()).To(ContainSubstring("failed to create lease"))
				Expect(acquired).To(BeFalse())
			})
		})

		Context("when Update fails with a non-Conflict API error during takeover", func() {
			It("should propagate the error", func() {
				// BR-GATEWAY-190: Distinguish a genuine API failure during expired-lease
				// takeover from ordinary update-race contention. A hard failure must
				// surface as an error so the caller's retry/backoff logic can react.
				errClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
							return apierrors.NewForbidden(schema.GroupResource{Resource: "leases"}, "gw-lock", errors.New("quota exceeded"))
						},
					}).
					Build()

				leaseName := "gw-lock-" + fingerprint[:16]
				leaseDuration := int32(30)
				expiredTime := metav1.NewMicroTime(time.Now().Add(-35 * time.Second))
				otherPodID := gatewayPodCrashed
				lease := &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      leaseName,
						Namespace: namespace,
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity:       &otherPodID,
						LeaseDurationSeconds: &leaseDuration,
						RenewTime:            &expiredTime,
					},
				}
				Expect(errClient.Create(ctx, lease)).To(Succeed())

				erroringManager := processing.NewDistributedLockManager(errClient, namespace, holderID)

				acquired, err := erroringManager.AcquireLock(ctx, fingerprint)

				Expect(err).To(HaveOccurred(),
					"BR-GATEWAY-190: Non-race Update failures during takeover must propagate as errors")
				Expect(err.Error()).To(ContainSubstring("failed to take over expired lease"))
				Expect(acquired).To(BeFalse())
			})
		})

		Context("when fingerprint is short (edge case)", func() {
			It("should handle short fingerprints gracefully", func() {
				// TDD: Define edge case behavior for short fingerprints
				// BR-GATEWAY-190: Short fingerprints should be hashed to ensure uniqueness

				shortFingerprint := "abc123" // Only 6 chars

				// When: Attempt to acquire lock with short fingerprint
				acquired, err := lockManager.AcquireLock(ctx, shortFingerprint)

				// Then: Should succeed (fingerprint hashed)
				Expect(err).NotTo(HaveOccurred(),
					"Short fingerprints should be handled gracefully")
				Expect(acquired).To(BeTrue(),
					"Lock should be acquired with short fingerprint")

				// And: Lease name should be K8s-compatible (<=63 chars)
				leaseList := &coordinationv1.LeaseList{}
				err = k8sClient.List(ctx, leaseList, client.InNamespace(namespace))
				Expect(err).NotTo(HaveOccurred())
				Expect(len(leaseList.Items)).To(Equal(1))
				Expect(len(leaseList.Items[0].Name)).To(BeNumerically("<=", 63),
					"Lease name must comply with K8s 63-char limit")
			})
		})
	})

	Describe("ReleaseLock", func() {
		BeforeEach(func() {
			// Given: Lock is acquired
			acquired, err := lockManager.AcquireLock(ctx, fingerprint)
			Expect(err).NotTo(HaveOccurred())
			Expect(acquired).To(BeTrue())
		})

		It("should release lock by deleting lease", func() {
			// TDD: Define lock release behavior
			// BR-GATEWAY-190: Lock must be released after processing completes

			// When: Release lock
			err := lockManager.ReleaseLock(ctx, fingerprint)

			// Then: Should succeed
			Expect(err).NotTo(HaveOccurred(),
				"Lock release should succeed")

			// And: Lease should be deleted
			leaseName := "gw-lock-" + fingerprint[:16]
			lease := &coordinationv1.Lease{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      leaseName,
			}, lease)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(),
				"Lease should be deleted after lock release")
		})

		It("should be idempotent (safe to call multiple times)", func() {
			// TDD: Define idempotent release behavior
			// BR-GATEWAY-190: Releasing lock twice should not error (safe for defer)

			// When: Release lock twice
			err1 := lockManager.ReleaseLock(ctx, fingerprint)
			err2 := lockManager.ReleaseLock(ctx, fingerprint)

			// Then: Both should succeed (idempotent)
			Expect(err1).NotTo(HaveOccurred(),
				"First release should succeed")
			Expect(err2).NotTo(HaveOccurred(),
				"Second release should succeed (idempotent)")
		})
	})

	Describe("Business Scenario: Concurrent Requests", func() {
		It("should allow only one pod to acquire lock at a time", func() {
			// TDD: Define core business behavior for GW-DEDUP-002 fix
			// BR-GATEWAY-190: Only 1 Gateway pod should process a signal at a time

			// Given: Two lock managers for different pods
			lockManager1 := processing.NewDistributedLockManager(k8sClient, namespace, "gateway-pod-1")
			lockManager2 := processing.NewDistributedLockManager(k8sClient, namespace, "gateway-pod-2")

			// When: Both pods try to acquire lock simultaneously
			acquired1, err1 := lockManager1.AcquireLock(ctx, fingerprint)
			acquired2, err2 := lockManager2.AcquireLock(ctx, fingerprint)

			// Then: Only one should acquire lock
			Expect(err1).NotTo(HaveOccurred())
			Expect(err2).NotTo(HaveOccurred())
			Expect(acquired1 != acquired2).To(BeTrue(),
				"Only one pod should acquire lock (mutual exclusion)")

			// And: The pod that didn't acquire lock should be able to retry after release
			if acquired1 {
				Expect(lockManager1.ReleaseLock(ctx, fingerprint)).To(Succeed())
				acquired2Retry, err := lockManager2.AcquireLock(ctx, fingerprint)
				Expect(err).NotTo(HaveOccurred())
				Expect(acquired2Retry).To(BeTrue(),
					"Second pod should acquire lock after first pod releases")
			} else {
				Expect(lockManager2.ReleaseLock(ctx, fingerprint)).To(Succeed())
				acquired1Retry, err := lockManager1.AcquireLock(ctx, fingerprint)
				Expect(err).NotTo(HaveOccurred())
				Expect(acquired1Retry).To(BeTrue(),
					"First pod should acquire lock after second pod releases")
			}
		})
	})

	Describe("GW-C1: Holder-aware lock release (#1356)", func() {
		It("UT-GW-1356-001: should not delete lease if held by another pod", func() {
			// Given: Pod 1 acquires the lock
			lockManager1 := processing.NewDistributedLockManager(k8sClient, namespace, "gateway-pod-1")
			acquired, err := lockManager1.AcquireLock(ctx, fingerprint)
			Expect(err).NotTo(HaveOccurred())
			Expect(acquired).To(BeTrue())

			// And: Pod 1's lock expires and Pod 2 takes over
			leaseName := "gw-lock-" + fingerprint[:16]
			lease := &coordinationv1.Lease{}
			err = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: leaseName}, lease)
			Expect(err).NotTo(HaveOccurred())
			expiredTime := metav1.NewMicroTime(time.Now().Add(-35 * time.Second))
			lease.Spec.RenewTime = &expiredTime
			Expect(k8sClient.Update(ctx, lease)).To(Succeed())

			lockManager2 := processing.NewDistributedLockManager(k8sClient, namespace, "gateway-pod-2")
			acquired2, err := lockManager2.AcquireLock(ctx, fingerprint)
			Expect(err).NotTo(HaveOccurred())
			Expect(acquired2).To(BeTrue())

			// When: Pod 1 (stale owner) tries to release
			err = lockManager1.ReleaseLock(ctx, fingerprint)

			// Then: Should not return error (idempotent/safe for defer)
			Expect(err).NotTo(HaveOccurred())

			// And: Lease should still exist (owned by Pod 2)
			err = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: leaseName}, lease)
			Expect(err).NotTo(HaveOccurred(), "Lease should NOT be deleted")
			Expect(*lease.Spec.HolderIdentity).To(Equal("gateway-pod-2"))
		})

		It("UT-GW-1356-002: should delete lease if held by current pod", func() {
			// Given: Pod 1 acquires the lock
			acquired, err := lockManager.AcquireLock(ctx, fingerprint)
			Expect(err).NotTo(HaveOccurred())
			Expect(acquired).To(BeTrue())

			// When: Same pod releases it
			err = lockManager.ReleaseLock(ctx, fingerprint)
			Expect(err).NotTo(HaveOccurred())

			// Then: Lease is deleted
			leaseName := "gw-lock-" + fingerprint[:16]
			lease := &coordinationv1.Lease{}
			err = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: leaseName}, lease)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})

	Describe("Business Scenario: Pod Crash Recovery", func() {
		It("should allow new pod to take over after lease expires", func() {
			// TDD: Define fault-tolerance behavior
			// BR-GATEWAY-190: If pod crashes, lease should expire and allow recovery

			// Given: Pod 1 acquires lock
			lockManager1 := processing.NewDistributedLockManager(k8sClient, namespace, "gateway-pod-1")
			acquired, err := lockManager1.AcquireLock(ctx, fingerprint)
			Expect(err).NotTo(HaveOccurred())
			Expect(acquired).To(BeTrue())

			// And: Pod 1 crashes (doesn't release lock)
			// Simulate by manually setting lease to expired
			leaseName := "gw-lock-" + fingerprint[:16]
			lease := &coordinationv1.Lease{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      leaseName,
			}, lease)
			Expect(err).NotTo(HaveOccurred())

			// Set renew time to 35 seconds ago (expired)
			expiredTime := metav1.NewMicroTime(time.Now().Add(-35 * time.Second))
			lease.Spec.RenewTime = &expiredTime
			Expect(k8sClient.Update(ctx, lease)).To(Succeed())

			// When: Pod 2 tries to acquire lock
			lockManager2 := processing.NewDistributedLockManager(k8sClient, namespace, "gateway-pod-2")
			acquired2, err := lockManager2.AcquireLock(ctx, fingerprint)

			// Then: Pod 2 should acquire lock by taking over expired lease
			Expect(err).NotTo(HaveOccurred(),
				"Pod 2 should successfully take over expired lease")
			Expect(acquired2).To(BeTrue(),
				"Pod 2 should acquire lock after Pod 1 crashed (fault-tolerance)")
		})
	})
})
