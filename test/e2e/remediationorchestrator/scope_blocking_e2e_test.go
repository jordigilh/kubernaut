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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// ========================================
// BR-SCOPE-010: RO Scope Blocking E2E Tests
// ========================================
//
// E2E tests validate scope blocking with the real RO controller running
// in a Kind cluster. The controller processes RRs and applies scope
// validation as Check #1 in the blocking pipeline (ADR-053).
//
// Test Plan: docs/services/crd-controllers/05-remediationorchestrator/RO_SCOPE_VALIDATION_TEST_PLAN_V1.0.md
//
// Defense-in-depth overlap:
// - Unit tests: CheckUnmanagedResource() isolation (test/unit/remediationorchestrator/routing/)
// - Integration tests: Scope blocking with envtest (test/integration/remediationorchestrator/)
// - E2E tests (this file): Full controller behavior in Kind cluster

var _ = Describe("BR-SCOPE-010: RO Scope Blocking E2E", Label("e2e", "scope"), func() {

	// ─────────────────────────────────────────────
	// E2E-RO-010-001: Unmanaged namespace blocks RR
	// ─────────────────────────────────────────────
	It("E2E-RO-010-001: should block RR for target in unmanaged namespace", func() {
		By("Creating an unmanaged namespace (no kubernaut.ai/managed label)")
		unmanagedNS := helpers.CreateTestNamespaceAndWait(k8sClient, "scope-e2e-unmgd",
			helpers.WithoutManagedLabel())
		defer helpers.DeleteTestNamespace(ctx, k8sClient, unmanagedNS)

		By("Verifying namespace does NOT have managed label")
		nsObj := &corev1.Namespace{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: unmanagedNS},
		}), nsObj)).To(Succeed())
		_, hasLabel := nsObj.Labels[scope.ManagedLabelKey]
		Expect(hasLabel).To(BeFalse(), "Namespace should NOT have managed label")

		By("Creating a RemediationRequest targeting a resource in the unmanaged namespace")
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-scope-unmanaged-e2e",
				Namespace: unmanagedNS,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "e2e010001a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
				SignalName:        "HighCPUUsage",
				Severity:          "critical",
				SignalType:        "prometheus",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: unmanagedNS,
				},
				FiringTime:   now,
				ReceivedTime: now,
				Deduplication: sharedtypes.DeduplicationInfo{
					FirstOccurrence: now,
					LastOccurrence:  now,
					OccurrenceCount: 1,
				},
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		By("Waiting for the RO controller to block the RR with UnmanagedResource reason")
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := apiReader.Get(ctx, client.ObjectKeyFromObject(rr), fetched); err != nil {
				return ""
			}
			return fetched.Status.BlockReason
		}, timeout, interval).Should(Equal(string(remediationv1.BlockReasonUnmanagedResource)),
			"RR should be blocked with UnmanagedResource reason by the RO controller")

		By("Verifying blocked phase metadata")
		fetched := &remediationv1.RemediationRequest{}
		Expect(apiReader.Get(ctx, client.ObjectKeyFromObject(rr), fetched)).To(Succeed())
		Expect(fetched.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked))
		Expect(fetched.Status.BlockMessage).To(ContainSubstring("kubernaut.ai/managed=true"),
			"Block message should include remediation instructions")
		Expect(fetched.Status.BlockedUntil).ToNot(BeNil(),
			"BlockedUntil should be set for scope backoff")

		GinkgoWriter.Printf("✅ E2E-RO-010-001: RR blocked — reason: %s, message: %s\n",
			fetched.Status.BlockReason, fetched.Status.BlockMessage)
	})

	// ─────────────────────────────────────────────
	// E2E-RO-010-002: Managed namespace allows RR to proceed
	// ─────────────────────────────────────────────
	It("E2E-RO-010-002: should allow RR to proceed for target in managed namespace", func() {
		By("Creating a managed namespace (with kubernaut.ai/managed=true)")
		managedNS := createTestNamespace("scope-e2e-mgd")
		defer deleteTestNamespace(managedNS)

		By("Creating a RemediationRequest targeting a resource in the managed namespace")
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-scope-managed-e2e",
				Namespace: managedNS,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "e2e010002b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5",
				SignalName:        "HighCPUUsage",
				Severity:          "critical",
				SignalType:        "prometheus",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: managedNS,
				},
				FiringTime:   now,
				ReceivedTime: now,
				Deduplication: sharedtypes.DeduplicationInfo{
					FirstOccurrence: now,
					LastOccurrence:  now,
					OccurrenceCount: 1,
				},
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		By("Waiting for the RO controller to process the RR (should NOT be blocked for scope)")
		// The controller should pick it up and transition it past Pending
		// It should NOT be blocked with UnmanagedResource reason
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := apiReader.Get(ctx, client.ObjectKeyFromObject(rr), fetched); err != nil {
				return ""
			}
			return string(fetched.Status.OverallPhase)
		}, timeout, interval).ShouldNot(BeEmpty(),
			"RR should be processed by the controller")

		// Verify it was not blocked for scope reasons
		fetched := &remediationv1.RemediationRequest{}
		Expect(apiReader.Get(ctx, client.ObjectKeyFromObject(rr), fetched)).To(Succeed())

		if fetched.Status.OverallPhase == remediationv1.PhaseBlocked {
			Expect(fetched.Status.BlockReason).ToNot(Equal(string(remediationv1.BlockReasonUnmanagedResource)),
				"RR in managed namespace should NOT be blocked for UnmanagedResource")
		}

		GinkgoWriter.Printf("✅ E2E-RO-010-002: RR in managed namespace proceeded — phase: %s\n",
			fetched.Status.OverallPhase)
	})

	// ─────────────────────────────────────────────
	// E2E-RO-010-003: Auto-unblock after namespace becomes managed
	// ─────────────────────────────────────────────
	It("E2E-RO-010-003: should auto-unblock RR when namespace becomes managed", func() {
		By("Creating an unmanaged namespace")
		ns := helpers.CreateTestNamespaceAndWait(k8sClient, "scope-e2e-unblock",
			helpers.WithoutManagedLabel())
		defer helpers.DeleteTestNamespace(ctx, k8sClient, ns)

		By("Creating a RemediationRequest in unmanaged namespace")
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-scope-unblock-e2e",
				Namespace: ns,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "e2e010003c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6",
				SignalName:        "HighCPUUsage",
				Severity:          "critical",
				SignalType:        "prometheus",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: ns,
				},
				FiringTime:   now,
				ReceivedTime: now,
				Deduplication: sharedtypes.DeduplicationInfo{
					FirstOccurrence: now,
					LastOccurrence:  now,
					OccurrenceCount: 1,
				},
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		By("Waiting for RR to be blocked with UnmanagedResource")
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := apiReader.Get(ctx, client.ObjectKeyFromObject(rr), fetched); err != nil {
				return ""
			}
			return fetched.Status.BlockReason
		}, timeout, interval).Should(Equal(string(remediationv1.BlockReasonUnmanagedResource)))

		GinkgoWriter.Println("✅ RR blocked — now adding managed label to namespace")

		By("Adding kubernaut.ai/managed=true label to namespace")
		nsObj := &corev1.Namespace{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		}), nsObj)).To(Succeed())
		if nsObj.Labels == nil {
			nsObj.Labels = map[string]string{}
		}
		nsObj.Labels[scope.ManagedLabelKey] = scope.ManagedLabelValueTrue
		Expect(k8sClient.Update(ctx, nsObj)).To(Succeed())

		By("Waiting for RR to auto-unblock after scope backoff expires")
		// Scope backoff starts at 5s. The controller should re-evaluate scope
		// after the block expires, find the namespace is now managed, and unblock.
		Eventually(func() bool {
			fetched := &remediationv1.RemediationRequest{}
			if err := apiReader.Get(ctx, client.ObjectKeyFromObject(rr), fetched); err != nil {
				return false
			}
			// Check if no longer blocked for UnmanagedResource
			if fetched.Status.OverallPhase == remediationv1.PhaseBlocked &&
				fetched.Status.BlockReason == string(remediationv1.BlockReasonUnmanagedResource) {
				return false
			}
			return true
		}, 60*time.Second, interval).Should(BeTrue(),
			"RR should auto-unblock after namespace becomes managed")

		// Verify final state
		fetched := &remediationv1.RemediationRequest{}
		Expect(apiReader.Get(ctx, client.ObjectKeyFromObject(rr), fetched)).To(Succeed())
		GinkgoWriter.Printf("✅ E2E-RO-010-003: RR auto-unblocked — final phase: %s, blockReason: %s\n",
			fetched.Status.OverallPhase, fetched.Status.BlockReason)
	})
})
