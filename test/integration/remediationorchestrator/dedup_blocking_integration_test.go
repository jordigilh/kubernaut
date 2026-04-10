/*
Copyright 2026 Jordi Gil.

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

// Integration tests for Issue #614: RO-level DuplicateInProgress Outcome Inheritance.
// These tests validate that the RO controller inherits outcomes from original RRs
// when a duplicate-blocked RR's original reaches terminal phase.
//
// Test Strategy:
//   - Real envtest with RO controller running
//   - Create RR, manually set status to Blocked/DuplicateInProgress
//   - Create original RR in various terminal states
//   - Verify the duplicate RR inherits the correct outcome
//
// Defense-in-Depth:
//   - Unit tests: Mock client, isolated recheckDuplicateBlock logic (dedup_blocking_test.go)
//   - Integration tests: Real K8s API with reconciler loop (this file)

package remediationorchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// createBlockedDuplicateRR creates an RR and manually sets its status to
// Blocked/DuplicateInProgress, referencing the given original RR name.
// Waits for the RO controller's initial reconcile (Pending/Processing) so
// status updates use a fresh resourceVersion and do not race with init;
// uses retry-on-conflict for the Blocked status injection.
func createBlockedDuplicateRR(ns, name, duplicateOf string) *remediationv1.RemediationRequest {
	now := metav1.Now()
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ROControllerNamespace,
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: func() string {
				h := sha256.Sum256([]byte(uuid.New().String()))
				return hex.EncodeToString(h[:])
			}(),
			SignalName: "TestDupBlockAlert",
			Severity:   "critical",
			SignalType: "alert",
			TargetType: "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: ns,
			},
			FiringTime:   now,
			ReceivedTime: now,
		},
	}
	Expect(k8sClient.Create(ctx, rr)).To(Succeed())

	key := types.NamespacedName{Name: name, Namespace: ROControllerNamespace}
	Eventually(func() remediationv1.RemediationPhase {
		fetched := &remediationv1.RemediationRequest{}
		if err := k8sManager.GetAPIReader().Get(ctx, key, fetched); err != nil {
			return ""
		}
		return fetched.Status.OverallPhase
	}, timeout, 25*time.Millisecond).Should(Or(Equal(remediationv1.PhasePending), Equal(remediationv1.PhaseProcessing)),
		"RO should initialize %s/%s before injecting Blocked status", ROControllerNamespace, name)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fetched := &remediationv1.RemediationRequest{}
		if err := k8sClient.Get(ctx, key, fetched); err != nil {
			return err
		}
		fetched.Status.OverallPhase = remediationv1.PhaseBlocked
		fetched.Status.BlockReason = remediationv1.BlockReasonDuplicateInProgress
		fetched.Status.BlockMessage = fmt.Sprintf("Duplicate of active remediation %s", duplicateOf)
		fetched.Status.DuplicateOf = duplicateOf
		fetched.Status.StartTime = &now
		fetched.Status.ObservedGeneration = fetched.Generation
		return k8sClient.Status().Update(ctx, fetched)
	})
	Expect(err).To(Succeed())

	GinkgoWriter.Printf("✅ Created Blocked/DuplicateInProgress RR: %s (duplicateOf: %s)\n", name, duplicateOf)
	return rr
}

// createTerminalRR creates an RR with a given terminal phase.
// Waits for the RO controller's initial reconcile (Pending) so status updates use a
// fresh resourceVersion and do not race with init; uses retry-on-conflict for updates.
func createTerminalRR(ns, name string, phase remediationv1.RemediationPhase) *remediationv1.RemediationRequest {
	now := metav1.Now()
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ROControllerNamespace,
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: func() string {
				h := sha256.Sum256([]byte(uuid.New().String()))
				return hex.EncodeToString(h[:])
			}(),
			SignalName: "TestOriginalAlert",
			Severity:   "critical",
			SignalType: "alert",
			TargetType: "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: ns,
			},
			FiringTime:   now,
			ReceivedTime: now,
		},
	}
	Expect(k8sClient.Create(ctx, rr)).To(Succeed())

	key := types.NamespacedName{Name: name, Namespace: ROControllerNamespace}
	// Poll faster than suite interval: RO requeues after ~100ms and may advance Pending→Processing quickly.
	// Accept either phase: we only need the controller to have touched status (fresh RV) before our terminal write.
	Eventually(func() remediationv1.RemediationPhase {
		fetched := &remediationv1.RemediationRequest{}
		if err := k8sManager.GetAPIReader().Get(ctx, key, fetched); err != nil {
			return ""
		}
		return fetched.Status.OverallPhase
	}, timeout, 25*time.Millisecond).Should(Or(Equal(remediationv1.PhasePending), Equal(remediationv1.PhaseProcessing)),
		"RO should initialize %s/%s (Pending or Processing) before injecting terminal status", ROControllerNamespace, name)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fetched := &remediationv1.RemediationRequest{}
		if err := k8sClient.Get(ctx, key, fetched); err != nil {
			return err
		}
		fetched.Status.OverallPhase = phase
		if phase == remediationv1.PhaseCompleted {
			fetched.Status.Outcome = "Remediated"
			fetched.Status.CompletedAt = &now
		} else if phase == remediationv1.PhaseFailed {
			failPhase := remediationv1.FailurePhaseWorkflowExecution
			failReason := "original failure"
			fetched.Status.FailurePhase = &failPhase
			fetched.Status.FailureReason = &failReason
			fetched.Status.CompletedAt = &now
		}
		fetched.Status.ObservedGeneration = fetched.Generation
		return k8sClient.Status().Update(ctx, fetched)
	})
	Expect(err).To(Succeed())

	GinkgoWriter.Printf("✅ Created terminal RR: %s (phase: %s)\n", name, phase)
	return rr
}

var _ = Describe("Issue #614: DuplicateInProgress Outcome Inheritance Integration", Label("integration", "dedup-blocking"), func() {
	var ns string

	BeforeEach(func() {
		ns = createTestNamespace("dedup-block")
	})

	AfterEach(func() {
		deleteTestNamespace(ns)
	})

	It("IT-RO-614-001: Blocked/DuplicateInProgress RR inherits Completed from original RR", func() {
		originalName := fmt.Sprintf("original-614-001-%s", uuid.New().String()[:8])
		dupName := fmt.Sprintf("dup-614-001-%s", uuid.New().String()[:8])

		By("Creating the original RR in Completed state")
		createTerminalRR(ns, originalName, remediationv1.PhaseCompleted)

		By("Creating the duplicate RR in Blocked/DuplicateInProgress state")
		dupRR := createBlockedDuplicateRR(ns, dupName, originalName)

		By("Waiting for the duplicate to inherit Completed")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(dupRR), dupRR)
			return dupRR.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted))

		Expect(dupRR.Status.Outcome).To(Equal("Remediated"),
			"Behavior: outcome must be Remediated (lineage tracked via DuplicateOf + K8s events)")
		Expect(dupRR.Status.CompletedAt).NotTo(BeNil(),
			"Behavior: CompletedAt must be set for terminal transition")
	})

	It("IT-RO-614-002: Blocked/DuplicateInProgress RR inherits Failed from original RR with FailurePhaseDeduplicated", func() {
		originalName := fmt.Sprintf("original-614-002-%s", uuid.New().String()[:8])
		dupName := fmt.Sprintf("dup-614-002-%s", uuid.New().String()[:8])

		By("Creating the original RR in Failed state")
		createTerminalRR(ns, originalName, remediationv1.PhaseFailed)

		By("Creating the duplicate RR in Blocked/DuplicateInProgress state")
		dupRR := createBlockedDuplicateRR(ns, dupName, originalName)

		By("Waiting for the duplicate to inherit Failed")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(dupRR), dupRR)
			return dupRR.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))

		Expect(dupRR.Status.FailurePhase).NotTo(BeNil(),
			"Behavior: FailurePhase must be set for inherited failures")
		Expect(*dupRR.Status.FailurePhase).To(Equal(remediationv1.FailurePhaseDeduplicated),
			"Behavior: FailurePhase must be Deduplicated (not the original's failure phase)")
		Expect(dupRR.Status.FailureReason).NotTo(BeNil())
		Expect(*dupRR.Status.FailureReason).To(ContainSubstring(originalName),
			"Behavior: FailureReason must reference the original RR for traceability")
		Expect(dupRR.Status.ConsecutiveFailureCount).To(Equal(int32(0)),
			"Behavior: inherited failures must NOT increment ConsecutiveFailureCount")
	})

	It("IT-RO-614-003: inherited failure from RR-level dedup is excluded from countConsecutiveFailures", func() {
		originalName := fmt.Sprintf("original-614-003-%s", uuid.New().String()[:8])
		dupName := fmt.Sprintf("dup-614-003-%s", uuid.New().String()[:8])

		By("Creating the original RR in Failed state")
		createTerminalRR(ns, originalName, remediationv1.PhaseFailed)

		By("Creating the duplicate RR in Blocked/DuplicateInProgress state")
		dupRR := createBlockedDuplicateRR(ns, dupName, originalName)

		By("Waiting for the duplicate to inherit Failed")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(dupRR), dupRR)
			return dupRR.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))

		By("Verifying FailurePhase=Deduplicated marks this for exclusion from countConsecutiveFailures")
		Expect(dupRR.Status.FailurePhase).NotTo(BeNil())
		Expect(*dupRR.Status.FailurePhase).To(Equal(remediationv1.FailurePhaseDeduplicated),
			"Invariant: FailurePhase=Deduplicated is the marker that countConsecutiveFailures skips")

		By("Creating a new RR with same fingerprint to verify it is NOT blocked")
		newRRName := fmt.Sprintf("new-614-003-%s", uuid.New().String()[:8])
		newRR := createRemediationRequestWithFingerprint(ns, newRRName, dupRR.Spec.SignalFingerprint)

		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(newRR), newRR)
			return newRR.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing),
			"Behavior: new RR must proceed to Processing, NOT be blocked (inherited failures excluded from count)")
	})
})
