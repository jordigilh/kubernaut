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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ========================================
// V1.0 Centralized Routing Integration Tests (Day 5)
// DD-RO-002: Centralized Routing Responsibility
// ========================================
//
// These tests validate RO's centralized routing logic with real Kubernetes API.
// They verify that RO correctly blocks RemediationRequest creation based on:
// - Workflow cooldown (RecentlyRemediated)
// - Signal cooldown (DuplicateInProgress)
// - Resource lock (ResourceBusy)
//
// Reference: V1.0_CENTRALIZED_ROUTING_IMPLEMENTATION_PLAN.md (Day 5, Task 3.2)
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Routing logic in pkg/remediationorchestrator/routing/
// - Integration tests (>50%): RO routing + K8s API interaction (this file)
// - E2E tests (10-15%): Full signal → remediation flow
//
// ========================================
// Phase 1 Integration Test Pattern
// ========================================
// These tests follow Phase 1 integration strategy where:
// - ONLY the RO controller runs (no child controllers: SP, AA, WE)
// - Tests manually create and control child CRD phases
// - This validates RO orchestration logic in isolation
//
// Reference: RO_PHASE1_INTEGRATION_STRATEGY_IMPLEMENTED_DEC_19_2025.md
// Helper functions: createSignalProcessingCRD, createAIAnalysisCRD, createWorkflowExecutionCRD

var _ = Describe("V1.0 Centralized Routing Integration (DD-RO-002)", func() {

	// ========================================
	// Test 2: Workflow Cooldown Prevents WE Creation
	// Reference: V1.0 Plan Day 5, Scenario 2
	// BR-RO-XXX: RecentlyRemediated blocking
	// ========================================
	Describe("Workflow Cooldown Blocking (RecentlyRemediated)", func() {

		// Phase 2 test moved to E2E suite: test/e2e/remediationorchestrator/routing_cooldown_e2e_test.go

		It("should transition from Blocked to Failed when cooldown period has expired", func() {
			// Create unique namespace for this test
			ns := createTestNamespace("routing-cooldown-expired")
			defer deleteTestNamespace(ns)

			GinkgoWriter.Println("📋 Testing cooldown expiry by setting BlockedUntil in the past")

			// Use unique fingerprint for this test (must be valid 64-char hex: ^[a-f0-9]{64}$)
			fingerprint := "aaaa0000bbbb1111cccc2222dddd3333eeee4444ffff5555aaaa6666bbbb7777"

			// Create RR that will be blocked
			rr := createRemediationRequestWithFingerprint(ns, "rr-cooldown-expired", fingerprint)

		// Wait for controller to fully process the RR (reach Processing phase)
		// RC-11 pattern: Wait for Processing + ObservedGeneration to prevent race
		// where controller overwrites our manually set Blocked phase
		Eventually(func() bool {
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: ns}, rr); err != nil {
				return false
			}
			return rr.Status.OverallPhase == remediationv1.PhaseProcessing &&
				rr.Status.ObservedGeneration == rr.Generation
		}, timeout, interval).Should(BeTrue(),
			"RR should reach Processing phase with ObservedGeneration matching Generation")

		// Manually set RR to Blocked state with BlockedUntil in the PAST
		// This simulates a cooldown that has already expired
		pastTime := metav1.NewTime(time.Now().Add(-10 * time.Second))
		Eventually(func() error {
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: ns}, rr); err != nil {
				return err
			}

			rr.Status.OverallPhase = remediationv1.PhaseBlocked
			rr.Status.BlockedUntil = &pastTime
			rr.Status.BlockReason = string(remediationv1.BlockReasonConsecutiveFailures)
			rr.Status.Message = "Blocked due to consecutive failures (test scenario)"
			rr.Status.ObservedGeneration = rr.Generation

			return k8sClient.Status().Update(ctx, rr)
		}, timeout, interval).Should(Succeed())

		// Verify RR transitions from Blocked → Failed after controller detects expired cooldown
		// BR-ORCH-042: Controller checks BlockedUntil on each reconcile
		Eventually(func(g Gomega) {
			g.Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: ns}, rr)).To(Succeed())
			g.Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed))
		}, timeout, interval).Should(Succeed(),
			"RR should transition to Failed when BlockedUntil is in the past")

			// Verify failure details indicate cooldown expiry
			Expect(rr.Status.FailurePhase).To(And(
				Not(BeNil()),
				HaveValue(Equal("blocked")),
			))
			Expect(rr.Status.FailureReason).To(And(
				Not(BeNil()),
				HaveValue(ContainSubstring("cooldown expired")),
			))

			GinkgoWriter.Println("✅ TEST PASSED: Cooldown expiry correctly triggered transition to Failed")
		})
	})

	// ========================================
	// Test 1: Signal Cooldown Prevents SP Creation
	// Reference: V1.0 Plan Day 5, Scenario 1
	// BR-GATEWAY-XXX: DuplicateInProgress blocking
	// ========================================
	Describe("Signal Cooldown Blocking (DuplicateInProgress)", func() {

		It("should block duplicate RR when active RR exists with same fingerprint", FlakeAttempts(3), func() {
			// FlakeAttempts(3): Timing-sensitive test with concurrent CRD operations - retry up to 3 times in CI
			// Create unique namespace for this test
			ns := createTestNamespace("routing-signal-cooldown")
			defer deleteTestNamespace(ns)

			// ========================================
			// SETUP: First RR is active (Pending/Processing/Analyzing/Executing)
			// ========================================

			GinkgoWriter.Println("📋 Creating first RemediationRequest (RR1) with specific fingerprint...")

			// Use a specific fingerprint for this test
			fingerprint := "c1d2e3f4a5b6c1d2e3f4a5b6c1d2e3f4a5b6c1d2e3f4a5b6c1d2e3f4a5b6c1d2"
			rr1 := createRemediationRequestWithFingerprint(ns, "rr-signal-1", fingerprint)

			// Wait for RR1 to be initialized (any non-empty phase)
			GinkgoWriter.Println("⏳ Waiting for RR1 to be initialized...")
			Eventually(func() string {
				rr := &remediationv1.RemediationRequest{}
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rr1.Name, Namespace: ns}, rr)
				if err != nil {
					return ""
				}
				return string(rr.Status.OverallPhase)
			}, timeout, interval).ShouldNot(BeEmpty(), "RR1 should be initialized")

			GinkgoWriter.Println("✅ RR1 is active and in progress")

			// ========================================
			// TEST: Second RR with SAME fingerprint should be blocked
			// ========================================

			GinkgoWriter.Println("")
			GinkgoWriter.Println("📋 Creating second RemediationRequest (RR2) with SAME fingerprint - should be BLOCKED...")

			// Create second RR with SAME fingerprint
			rr2 := createRemediationRequestWithFingerprint(ns, "rr-signal-2", fingerprint)

			// ========================================
			// VERIFY: RR2 should transition to Blocked (NOT create SP)
			// ========================================

			GinkgoWriter.Println("⏳ Waiting for RR2 to transition to Blocked phase...")
			Eventually(func() string {
				rr := &remediationv1.RemediationRequest{}
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rr2.Name, Namespace: ns}, rr)
				if err != nil {
					return ""
				}
				return string(rr.Status.OverallPhase)
			}, timeout, interval).Should(Equal("Blocked"), "RR2 should be blocked due to duplicate in progress")

			// Verify BlockReason is DuplicateInProgress
			GinkgoWriter.Println("✅ Verifying BlockReason is DuplicateInProgress...")
			rr2Updated := &remediationv1.RemediationRequest{}
			err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rr2.Name, Namespace: ns}, rr2Updated)
			Expect(err).ToNot(HaveOccurred())
			Expect(rr2Updated.Status.BlockReason).To(Equal("DuplicateInProgress"),
				"BlockReason should be DuplicateInProgress")
			Expect(rr2Updated.Status.BlockMessage).To(ContainSubstring("Duplicate"),
				"BlockMessage should mention duplicate")
			Expect(rr2Updated.Status.DuplicateOf).To(Equal(rr1.Name),
				"Should reference the original RR")

			GinkgoWriter.Println("✅ RR2 blocked correctly with DuplicateInProgress")

			// Verify NO SignalProcessing was created for RR2
			GinkgoWriter.Println("✅ Verifying NO SignalProcessing created for blocked RR2...")
			Consistently(func() bool {
				sp := &signalprocessingv1.SignalProcessing{}
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      rr2.Name + "-sp",
					Namespace: ns,
				}, sp)
				return apierrors.IsNotFound(err)
			}, 5*time.Second, interval).Should(BeTrue(),
				"SP should NOT exist for blocked RR2")

			GinkgoWriter.Println("✅ TEST PASSED: Signal cooldown correctly prevented SP creation")
		})

		It("should allow RR when original RR completes (no longer active)", func() {
			// Create unique namespace for this test
			ns := createTestNamespace("routing-signal-after-complete")
			defer deleteTestNamespace(ns)

			// Use a specific fingerprint for this test
			fingerprint := "d1e2f3a4b5c6d1e2f3a4b5c6d1e2f3a4b5c6d1e2f3a4b5c6d1e2f3a4b5c6d1e2"

		GinkgoWriter.Println("📋 Creating first RemediationRequest (RR1)...")
		rr1 := createRemediationRequestWithFingerprint(ns, "rr-signal-complete-1", fingerprint)

		// Wait for RR1 to transition to Processing (RO creates child SP)
		GinkgoWriter.Println("⏳ Waiting for RR1 to reach Processing phase...")
		Eventually(func() string {
			rr := &remediationv1.RemediationRequest{}
			err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rr1.Name, Namespace: ns}, rr)
			if err != nil {
				return ""
			}
			return string(rr.Status.OverallPhase)
		}, timeout, interval).Should(Equal("Processing"), "RR1 should reach Processing phase")

		// Phase 1 Pattern: Manually force RR to Completed by deleting child CRDs
		// This simulates terminal phase without needing to complete full orchestration pipeline
		// (no child controllers running - SP, AI, WE)

		GinkgoWriter.Println("✅ Simulating RR1 completion by deleting child CRDs (Phase 1: manual control)...")

		// Delete SignalProcessing CRD if it exists
		spName := "sp-rr-signal-complete-1"
		sp := &signalprocessingv1.SignalProcessing{}
		err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ns}, sp)
		if err == nil {
			GinkgoWriter.Println("🗑️  Deleting SP CRD to unblock RR1...")
			Expect(k8sClient.Delete(ctx, sp)).To(Succeed())
		}

		// Manually set RR1 to Completed (without child CRDs, RO won't override)
		GinkgoWriter.Println("✅ Manually setting RR1 to Completed...")
		Eventually(func() error {
			rr := &remediationv1.RemediationRequest{}
			err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rr1.Name, Namespace: ns}, rr)
			if err != nil {
				return err
			}
			rr.Status.OverallPhase = "Completed"
			return k8sClient.Status().Update(ctx, rr)
		}, timeout, interval).Should(Succeed(), "RR1 should be manually marked as Completed")

		// FIX: Explicitly wait for RR1 to be fully completed with ObservedGeneration
		// This ensures the controller has observed the completion before we create RR2
		// Prevents cache lag from causing RR2 to see stale RR1 status
		GinkgoWriter.Println("⏳ Waiting for RR1 completion to be observed by controller...")
		Eventually(func() bool {
			rr := &remediationv1.RemediationRequest{}
			err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rr1.Name, Namespace: ns}, rr)
			if err != nil {
				return false
			}
			// Controller must have observed the completion (ObservedGeneration == Generation)
			// AND status must be Completed
			return rr.Status.OverallPhase == "Completed" &&
				rr.Status.ObservedGeneration == rr.Generation
		}, timeout, interval).Should(BeTrue(), "RR1 should be fully completed with ObservedGeneration == Generation")

		GinkgoWriter.Println("✅ RR1 completed and observed by controller")

		// Create second RR with SAME fingerprint (should NOT be blocked now)
		GinkgoWriter.Println("📋 Creating second RemediationRequest (RR2) with same fingerprint...")
		rr2 := createRemediationRequestWithFingerprint(ns, "rr-signal-complete-2", fingerprint)

		// Verify RR2 is NOT blocked (should proceed to Pending/Processing)
		// FIX: Increased timeout to 120s for parallel execution environment (12 processes)
		// Parallel execution can be slower due to resource contention
		GinkgoWriter.Println("⏳ Waiting for RR2 to proceed (not blocked)...")
		Eventually(func() bool {
			rr := &remediationv1.RemediationRequest{}
			err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rr2.Name, Namespace: ns}, rr)
			if err != nil {
				return false
			}
			phase := string(rr.Status.OverallPhase)
			// Should be Pending or Processing, NOT Blocked
			return phase == "Pending" || phase == "Processing" || phase == "Analyzing"
		}, 120*time.Second, interval).Should(BeTrue(), "RR2 should proceed (original RR is no longer active)")

			GinkgoWriter.Println("✅ TEST PASSED: RR allowed after original completed")
		})
	})
})

// ========================================
// IT-RO-WE001: Target Resource Casing Preservation (Issue #203)
// Test Plan: docs/testing/COOLDOWN_GW_RO/TEST_PLAN.md
//
// BUSINESS VALUE:
// Validates the reconciler preserves AffectedResource.Kind casing
// when building the targetResource string for routing checks. The
// routing engine uses a field-indexed lookup on WFE.spec.targetResource,
// so a case mismatch silently bypasses the RecentlyRemediated cooldown.
// ========================================
var _ = Describe("Target Resource Casing Preservation (Issue #203)", func() {

	It("IT-RO-WE001-001: should block with RecentlyRemediated when Kind casing matches WFE", func() {
		ns := createTestNamespace("ro-we001-001")
		defer deleteTestNamespace(ns)

		By("Creating a RemediationRequest")
		rr := createRemediationRequest(ns, "rr-we001-001")

		By("Driving RR to Processing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		By("Completing SignalProcessing")
		spName := fmt.Sprintf("sp-%s", rr.Name)
		sp := &signalprocessingv1.SignalProcessing{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ns}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(ns, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

		By("Waiting for Analyzing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

		By("Pre-creating a completed WFE with uppercase Kind in spec.targetResource")
		// The targetResource format is "namespace/Kind/name" (preserving Kind casing).
		// Before the fix (issue #203), the reconciler lowercased the first letter of Kind,
		// producing "namespace/deployment/name" which would NOT match this WFE.
		targetResource := ns + "/Deployment/test-app"
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "wfe-recently-completed",
				Namespace: ns,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      "rr-previous",
					Namespace: ns,
				},
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					WorkflowID:      "wf-restart-pods",
					Version:         "v1.0.0",
					ExecutionBundle: "test-image:latest",
				},
				TargetResource:  targetResource,
				ExecutionEngine: "job",
			},
		}
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
		recentCompletion := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		wfe.Status.Phase = workflowexecutionv1.PhaseCompleted
		wfe.Status.CompletionTime = &recentCompletion
		Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

		By("Completing AIAnalysis with AffectedResource.Kind = 'Deployment' (uppercase)")
		aiName := fmt.Sprintf("ai-%s", rr.Name)
		ai := &aianalysisv1.AIAnalysis{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ns}, ai)
		}, timeout, interval).Should(Succeed())
		ai.Status.Phase = aianalysisv1.PhaseCompleted
		ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:      "wf-restart-pods",
			Version:         "v1.0.0",
			ExecutionBundle: "test-image:latest",
			Confidence:      0.95,
		}
		ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:    "OOM kill detected",
			Severity:   "critical",
			SignalType: "alert",
			AffectedResource: &aianalysisv1.AffectedResource{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: ns,
			},
		}
		aiCompletedAt := metav1.Now()
		ai.Status.CompletedAt = &aiCompletedAt
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		By("Asserting RR transitions to Blocked with RecentlyRemediated reason")
		// BUSINESS OUTCOME: The routing engine finds the pre-created WFE because
		// the reconciler now preserves "Deployment" (not "deployment") when building
		// the targetResource query. This prevents redundant remediation on a target
		// that was just successfully fixed.
		Eventually(func(g Gomega) {
			g.Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
			g.Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked))
			g.Expect(rr.Status.BlockReason).To(Equal(string(remediationv1.BlockReasonRecentlyRemediated)))
		}, timeout, interval).Should(Succeed(),
			"RR must be blocked as RecentlyRemediated when Kind casing matches WFE")

		GinkgoWriter.Println("TEST PASSED: Casing-preserved targetResource matched WFE (RecentlyRemediated)")
	})
})

// Note: Helper functions (createRemediationRequestWithFingerprint, simulateFailedPhase)
// are defined in blocking_integration_test.go and shared across all integration tests.
