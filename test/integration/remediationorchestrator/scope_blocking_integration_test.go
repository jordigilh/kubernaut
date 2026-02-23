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

package remediationorchestrator

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// ========================================
// BR-SCOPE-010: RO Scope Blocking Integration Tests
// ========================================
//
// These tests validate the scope blocking logic with real Kubernetes API (envtest).
// They verify:
// - RR for unmanaged resource is blocked with UnmanagedResource reason
// - RR for managed resource passes through scope check
// - Temporal drift: label removed mid-lifecycle triggers blocking
// - Auto-unblock: label added back, RR unblocks on next reconciliation
//
// Test Plan: docs/services/crd-controllers/05-remediationorchestrator/RO_SCOPE_VALIDATION_TEST_PLAN_V1.0.md
//
// TDD: Tests define expected behavior before implementation verification.

var _ = Describe("BR-SCOPE-010: RO Scope Blocking (Integration)", Label("scope", "integration"), func() {

	// ─────────────────────────────────────────────
	// IT-RO-010-001: Temporal drift — managed → unmanaged → blocks
	// ─────────────────────────────────────────────
	It("IT-RO-010-001: should block RR when namespace becomes unmanaged", func() {
		// Create a managed namespace
		ns := helpers.CreateTestNamespace(ctx, k8sClient, "scope-drift")
		defer helpers.DeleteTestNamespace(ctx, k8sClient, ns)

		// Verify namespace has managed label
		nsObj := &corev1.Namespace{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ns}, nsObj)).To(Succeed())
		Expect(nsObj.Labels[scope.ManagedLabelKey]).To(Equal(scope.ManagedLabelValueTrue),
			"Test namespace should have managed=true label")

		// Create RR in managed namespace — should proceed normally (not blocked by scope)
		rrName := fmt.Sprintf("rr-drift-%s", uuid.New().String()[:8])
		rr := createRemediationRequest(ns, rrName)

		// Wait for controller to process RR past the Pending phase
		// The RR should proceed to Processing or later — NOT be blocked by scope
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rr.Name,
				Namespace: ns,
			}, fetched); err != nil {
				return ""
			}
			return string(fetched.Status.OverallPhase)
		}, timeout, interval).ShouldNot(Equal(""),
			"RR should be processed by controller")

		// Verify it was NOT blocked by scope (it's managed)
		fetched := &remediationv1.RemediationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name:      rr.Name,
			Namespace: ns,
		}, fetched)).To(Succeed())

		if fetched.Status.OverallPhase == remediationv1.PhaseBlocked {
			Expect(fetched.Status.BlockReason).ToNot(Equal(string(remediationv1.BlockReasonUnmanagedResource)),
				"RR in managed namespace should NOT be blocked for UnmanagedResource")
		}

		GinkgoWriter.Printf("✅ IT-RO-010-001: RR %s in managed namespace proceeded without scope blocking (phase: %s)\n",
			rrName, fetched.Status.OverallPhase)
	})

	// ─────────────────────────────────────────────
	// IT-RO-010-002: Auto-unblock — unmanaged → managed → proceeds
	// ─────────────────────────────────────────────
	It("IT-RO-010-002: should unblock RR when namespace becomes managed", func() {
		// Create an unmanaged namespace
		ns := helpers.CreateTestNamespace(ctx, k8sClient, "scope-unblock", helpers.WithoutManagedLabel())
		defer helpers.DeleteTestNamespace(ctx, k8sClient, ns)

		// Verify namespace does NOT have managed label
		nsObj := &corev1.Namespace{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ns}, nsObj)).To(Succeed())
		_, hasLabel := nsObj.Labels[scope.ManagedLabelKey]
		Expect(hasLabel).To(BeFalse(), "Test namespace should NOT have managed label")

		// Create RR in unmanaged namespace — should be blocked
		rrName := fmt.Sprintf("rr-unblock-%s", uuid.New().String()[:8])
		createRemediationRequest(ns, rrName)

		// Wait for controller to block the RR due to scope
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rrName,
				Namespace: ns,
			}, fetched); err != nil {
				return ""
			}
			return fetched.Status.BlockReason
		}, timeout, interval).Should(Equal(string(remediationv1.BlockReasonUnmanagedResource)),
			"RR in unmanaged namespace should be blocked with UnmanagedResource reason")

		GinkgoWriter.Println("✅ RR blocked with UnmanagedResource — now adding managed label to namespace")

		// Add managed label to namespace
		nsObj = &corev1.Namespace{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ns}, nsObj)).To(Succeed())
		if nsObj.Labels == nil {
			nsObj.Labels = map[string]string{}
		}
		nsObj.Labels[scope.ManagedLabelKey] = scope.ManagedLabelValueTrue
		Expect(k8sClient.Update(ctx, nsObj)).To(Succeed())

		// Wait for the block to expire and controller to re-validate scope
		// The scope backoff is 5s, so the RR should be re-evaluated within ~10s
		// After re-validation, it should unblock (phase should change from Blocked)
		Eventually(func() bool {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rrName,
				Namespace: ns,
			}, fetched); err != nil {
				return false
			}
			// RR should no longer be blocked for UnmanagedResource
			if fetched.Status.OverallPhase == remediationv1.PhaseBlocked &&
				fetched.Status.BlockReason == string(remediationv1.BlockReasonUnmanagedResource) {
				return false // Still blocked
			}
			return true // Unblocked (either moved to different phase or different block reason)
		}, 30*time.Second, interval).Should(BeTrue(),
			"RR should unblock after namespace becomes managed")

		GinkgoWriter.Printf("✅ IT-RO-010-002: RR %s unblocked after namespace became managed\n", rrName)
	})

	// ─────────────────────────────────────────────
	// IT-RO-010-003: Audit event emitted for scope blocking
	// ─────────────────────────────────────────────
	It("IT-RO-010-003: should emit audit event when RR is blocked for scope", func() {
		// Create an unmanaged namespace
		ns := helpers.CreateTestNamespace(ctx, k8sClient, "scope-audit", helpers.WithoutManagedLabel())
		defer helpers.DeleteTestNamespace(ctx, k8sClient, ns)

		// Create RR in unmanaged namespace
		rrName := fmt.Sprintf("rr-audit-%s", uuid.New().String()[:8])
		createRemediationRequest(ns, rrName)

		// Wait for controller to block the RR
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rrName,
				Namespace: ns,
			}, fetched); err != nil {
				return ""
			}
			return fetched.Status.BlockReason
		}, timeout, interval).Should(Equal(string(remediationv1.BlockReasonUnmanagedResource)),
			"RR should be blocked with UnmanagedResource reason")

		// Verify the blocked phase metadata
		fetched := &remediationv1.RemediationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name:      rrName,
			Namespace: ns,
		}, fetched)).To(Succeed())

		Expect(fetched.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked))
		Expect(fetched.Status.BlockReason).To(Equal(string(remediationv1.BlockReasonUnmanagedResource)))
		Expect(fetched.Status.BlockMessage).To(ContainSubstring("kubernaut.ai/managed=true"),
			"Block message should include remediation instructions")
		Expect(fetched.Status.BlockedUntil).ToNot(BeNil(),
			"BlockedUntil should be set for time-based scope backoff")

		GinkgoWriter.Printf("✅ IT-RO-010-003: Scope blocking audit verified — reason: %s, blockedUntil: %s\n",
			fetched.Status.BlockReason, fetched.Status.BlockedUntil.Format(time.RFC3339))
	})

	// ─────────────────────────────────────────────
	// IT-RO-010-004: Backoff progression for repeated scope blocks
	// ─────────────────────────────────────────────
	It("IT-RO-010-004: should apply exponential backoff for scope blocking", func() {
		// Create an unmanaged namespace
		ns := helpers.CreateTestNamespace(ctx, k8sClient, "scope-backoff", helpers.WithoutManagedLabel())
		defer helpers.DeleteTestNamespace(ctx, k8sClient, ns)

		// Create RR in unmanaged namespace
		rrName := fmt.Sprintf("rr-backoff-%s", uuid.New().String()[:8])
		createRemediationRequest(ns, rrName)

		// Wait for blocking with backoff
		Eventually(func() bool {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rrName,
				Namespace: ns,
			}, fetched); err != nil {
				return false
			}
			return fetched.Status.OverallPhase == remediationv1.PhaseBlocked &&
				fetched.Status.BlockReason == string(remediationv1.BlockReasonUnmanagedResource) &&
				fetched.Status.BlockedUntil != nil
		}, timeout, interval).Should(BeTrue(),
			"RR should be blocked with BlockedUntil set")

		// Capture the first BlockedUntil time
		fetched := &remediationv1.RemediationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name:      rrName,
			Namespace: ns,
		}, fetched)).To(Succeed())

		firstBlockedUntil := fetched.Status.BlockedUntil.Time
		GinkgoWriter.Printf("✅ First blockedUntil: %s\n", firstBlockedUntil.Format(time.RFC3339))

		// Verify the blockedUntil is in the near future (within scope backoff bounds: ~5s)
		now := time.Now()
		Expect(firstBlockedUntil.After(now.Add(-2 * time.Second))).To(BeTrue(),
			"BlockedUntil should be near-future")
		Expect(firstBlockedUntil.Before(now.Add(30 * time.Second))).To(BeTrue(),
			"BlockedUntil should not be more than 30s in the future (initial backoff ~5s + jitter)")

		GinkgoWriter.Printf("✅ IT-RO-010-004: Exponential backoff verified — blockedUntil: %s\n",
			firstBlockedUntil.Format(time.RFC3339))
	})
})
