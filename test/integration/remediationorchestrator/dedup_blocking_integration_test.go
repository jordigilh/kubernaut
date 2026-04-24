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
// Test Strategy (revised per TOCTOU fix):
//   - Real envtest with RO controller running
//   - Use the NATURAL routing flow: create original (active), then duplicate
//     with the same fingerprint so the routing engine blocks it automatically
//   - Inject terminal status on the original AFTER the duplicate is blocked
//   - Verify the duplicate inherits the correct outcome
//
// This eliminates the TOCTOU race from manual Blocked status injection, where
// the controller's cached-client status updates could overwrite the injected phase.
//
// Defense-in-Depth:
//   - Unit tests: Mock client, isolated recheckDuplicateBlock logic (dedup_blocking_test.go)
//   - Integration tests: Real K8s API with reconciler loop (this file)

package remediationorchestrator

import (
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

// waitForProcessing waits until the RR reaches Processing via apiReader.
func waitForProcessing(name string) {
	key := types.NamespacedName{Name: name, Namespace: ROControllerNamespace}
	Eventually(func() remediationv1.RemediationPhase {
		fetched := &remediationv1.RemediationRequest{}
		if err := k8sManager.GetAPIReader().Get(ctx, key, fetched); err != nil {
			return ""
		}
		return fetched.Status.OverallPhase
	}, timeout, 25*time.Millisecond).Should(Equal(remediationv1.PhaseProcessing),
		"RO should reach Processing for %s/%s", ROControllerNamespace, name)
}

// waitForBlocked waits until the RR is naturally blocked by the routing engine
// as DuplicateInProgress, confirming the controller set the status (not the test).
func waitForBlocked(name string) {
	key := types.NamespacedName{Name: name, Namespace: ROControllerNamespace}
	Eventually(func() remediationv1.BlockReason {
		fetched := &remediationv1.RemediationRequest{}
		if err := k8sManager.GetAPIReader().Get(ctx, key, fetched); err != nil {
			return ""
		}
		if fetched.Status.OverallPhase != remediationv1.PhaseBlocked {
			return ""
		}
		return fetched.Status.BlockReason
	}, timeout, 25*time.Millisecond).Should(Equal(remediationv1.BlockReasonDuplicateInProgress),
		"RO should naturally block %s/%s as DuplicateInProgress", ROControllerNamespace, name)
}

// injectTerminalStatus injects a terminal phase on an RR that is already in Processing.
// Uses retry-on-conflict to handle concurrent controller updates.
func injectTerminalStatus(name string, phase remediationv1.RemediationPhase) {
	key := types.NamespacedName{Name: name, Namespace: ROControllerNamespace}
	now := metav1.Now()
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
	GinkgoWriter.Printf("✅ Injected terminal status: %s/%s → %s\n", ROControllerNamespace, name, phase)
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
		fingerprint := GenerateTestFingerprint(ns, "614-001")
		originalName := fmt.Sprintf("a-orig-614-001-%s", uuid.New().String()[:8])
		dupName := fmt.Sprintf("z-dup-614-001-%s", uuid.New().String()[:8])

		By("Creating the original RR and waiting for it to reach Processing (active)")
		createRemediationRequestWithFingerprint(ns, originalName, fingerprint)
		waitForProcessing(originalName)

		By("Creating the duplicate RR with the same fingerprint — routing blocks it naturally")
		dupRR := createRemediationRequestWithFingerprint(ns, dupName, fingerprint)
		waitForBlocked(dupName)

		By("Injecting Completed status on the original RR")
		injectTerminalStatus(originalName, remediationv1.PhaseCompleted)

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
		fingerprint := GenerateTestFingerprint(ns, "614-002")
		originalName := fmt.Sprintf("a-orig-614-002-%s", uuid.New().String()[:8])
		dupName := fmt.Sprintf("z-dup-614-002-%s", uuid.New().String()[:8])

		By("Creating the original RR and waiting for it to reach Processing (active)")
		createRemediationRequestWithFingerprint(ns, originalName, fingerprint)
		waitForProcessing(originalName)

		By("Creating the duplicate RR with the same fingerprint — routing blocks it naturally")
		dupRR := createRemediationRequestWithFingerprint(ns, dupName, fingerprint)
		waitForBlocked(dupName)

		By("Injecting Failed status on the original RR")
		injectTerminalStatus(originalName, remediationv1.PhaseFailed)

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
		fingerprint := GenerateTestFingerprint(ns, "614-003")
		originalName := fmt.Sprintf("a-orig-614-003-%s", uuid.New().String()[:8])
		dupName := fmt.Sprintf("z-dup-614-003-%s", uuid.New().String()[:8])

		By("Creating the original RR and waiting for it to reach Processing (active)")
		createRemediationRequestWithFingerprint(ns, originalName, fingerprint)
		waitForProcessing(originalName)

		By("Creating the duplicate RR with the same fingerprint — routing blocks it naturally")
		dupRR := createRemediationRequestWithFingerprint(ns, dupName, fingerprint)
		waitForBlocked(dupName)

		By("Injecting Failed status on the original RR")
		injectTerminalStatus(originalName, remediationv1.PhaseFailed)

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
		newRRName := fmt.Sprintf("z-new-614-003-%s", uuid.New().String()[:8])
		newRR := createRemediationRequestWithFingerprint(ns, newRRName, fingerprint)

		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(newRR), newRR)
			return newRR.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing),
			"Behavior: new RR must proceed to Processing, NOT be blocked (inherited failures excluded from count)")
	})
})
