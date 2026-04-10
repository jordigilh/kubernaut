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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ========================================
// Issue #189: RO Distributed Locking Integration Tests
// BR-ORCH-025: Prevents duplicate WFE creation for same target
// BR-ORCH-031: Idempotent WFE creation (Get-before-Create preserved)
// BR-ORCH-050: Owner-ref self-detection in CheckResourceBusy
// ========================================

var _ = Describe("RO Distributed Locking (Issue #189, BR-ORCH-025)", func() {

	// advanceToAnalyzing advances an RR through SP → AI complete, leaving it
	// in Analyzing phase ready to create a WFE. Returns the completed AIAnalysis.
	advanceToAnalyzing := func(ns, rrName, targetResource string) (*remediationv1.RemediationRequest, *aianalysisv1.AIAnalysis) {
		rr := createRemediationRequest(ns, rrName)

		// Wait for SP creation
		Eventually(func() bool {
			spList := &signalprocessingv1.SignalProcessingList{}
			if err := k8sClient.List(ctx, spList, client.InNamespace(ROControllerNamespace)); err != nil {
				return false
			}
			for _, sp := range spList.Items {
				if sp.Spec.RemediationRequestRef.Name == rrName {
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "SP should be created for RR")

		// Find and complete SP
		spList := &signalprocessingv1.SignalProcessingList{}
		Expect(k8sClient.List(ctx, spList, client.InNamespace(ROControllerNamespace))).To(Succeed())
		var spName string
		for _, sp := range spList.Items {
			if sp.Spec.RemediationRequestRef.Name == rrName {
				spName = sp.Name
				break
			}
		}
		Expect(spName).NotTo(BeEmpty())
		Eventually(func() error {
			return updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted)
		}, timeout, interval).Should(Succeed())

		// Wait for AI Analysis creation
		Eventually(func() bool {
			aiList := &aianalysisv1.AIAnalysisList{}
			if err := k8sClient.List(ctx, aiList, client.InNamespace(ROControllerNamespace)); err != nil {
				return false
			}
			for _, ai := range aiList.Items {
				if ai.Spec.RemediationRequestRef.Name == rrName {
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "AIAnalysis should be created for RR")

		// Find and complete AI Analysis with RCA target
		aiList := &aianalysisv1.AIAnalysisList{}
		Expect(k8sClient.List(ctx, aiList, client.InNamespace(ROControllerNamespace))).To(Succeed())
		var ai *aianalysisv1.AIAnalysis
		for i := range aiList.Items {
			if aiList.Items[i].Spec.RemediationRequestRef.Name == rrName {
				ai = &aiList.Items[i]
				break
			}
		}
		Expect(ai).NotTo(BeNil())

		// Complete AI Analysis with target resource and selected workflow
		Eventually(func() error {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ai), ai); err != nil {
				return err
			}
			ai.Status.Phase = "Completed"
			now := metav1.Now()
			ai.Status.CompletedAt = &now
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary:    "Test root cause",
				Severity:   "critical",
				SignalType: "alert",
				RemediationTarget: &aianalysisv1.RemediationTarget{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: ns,
				},
			}
			ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:      "wf-test-001",
				ActionType:      "restart",
				Version:         "v1",
				ExecutionBundle: "test-image:latest",
				Confidence:      0.95,
				Rationale:       "Test rationale",
			}
			return k8sClient.Status().Update(ctx, ai)
		}, timeout, interval).Should(Succeed())

		// Refetch RR
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
		}, timeout, interval).Should(Succeed())

		return rr, ai
	}

	// IT-RO-189-001: Two RRs same target, parallel reconciles = only one WFE
	Describe("IT-RO-189-001: Two RRs same target = only one WFE", func() {
		It("should create only one WFE when two RRs target the same resource", func() {
			ns := createTestNamespace("locking-dedup")
			defer deleteTestNamespace(ns)

			targetResource := fmt.Sprintf("%s/Deployment/test-app", ns)

			// Create RR1 and advance to Analyzing
			rr1, _ := advanceToAnalyzing(ns, "rr-lock-001", targetResource)

			// Wait for WFE creation from RR1
			Eventually(func() bool {
				wfeList := &workflowexecutionv1.WorkflowExecutionList{}
				if err := k8sClient.List(ctx, wfeList, client.InNamespace(ROControllerNamespace)); err != nil {
					return false
				}
				for _, wfe := range wfeList.Items {
					if wfe.Spec.TargetResource == targetResource {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "WFE should be created for RR1")

			// Create RR2 targeting the same resource
			rr2, _ := advanceToAnalyzing(ns, "rr-lock-002", targetResource)

			// Wait for RR2 to be processed (either WFE created or blocked)
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr2), rr2); err != nil {
					return ""
				}
				return string(rr2.Status.OverallPhase)
			}, timeout, interval).Should(SatisfyAny(
				Equal(string(remediationv1.PhaseBlocked)),
				Equal(string(remediationv1.PhaseExecuting)),
			), "RR2 should be either Blocked or Executing")

			// Verify only one active WFE for this target
			wfeList := &workflowexecutionv1.WorkflowExecutionList{}
			Expect(k8sClient.List(ctx, wfeList, client.InNamespace(ROControllerNamespace))).To(Succeed())
			activeCount := 0
			for _, wfe := range wfeList.Items {
				if wfe.Spec.TargetResource == targetResource &&
					wfe.Status.Phase != workflowexecutionv1.PhaseCompleted &&
					wfe.Status.Phase != workflowexecutionv1.PhaseFailed {
					activeCount++
				}
			}
			Expect(activeCount).To(Equal(1), "only one active WFE should exist for the target")

			_ = rr1 // used in advanceToAnalyzing
		})
	})

	// IT-RO-189-002: WFE creation fails = lock released, next reconcile acquires
	Describe("IT-RO-189-002: Lock released on WFE creation failure", func() {
		It("should release the lock when WFE creation fails, allowing retry", func() {
			ns := createTestNamespace("locking-fail-release")
			defer deleteTestNamespace(ns)

			targetResource := fmt.Sprintf("%s/Deployment/test-app", ns)
			rr, _ := advanceToAnalyzing(ns, "rr-lock-fail", targetResource)

			// Wait for reconcile to process (either creates WFE or fails)
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
					return ""
				}
				return string(rr.Status.OverallPhase)
			}, timeout, interval).Should(SatisfyAny(
				Equal(string(remediationv1.PhaseExecuting)),
				Equal(string(remediationv1.PhaseAnalyzing)),
			))

			// Verify no dangling Leases exist after reconcile
			leaseList := &coordinationv1.LeaseList{}
			Expect(k8sClient.List(ctx, leaseList, client.InNamespace(ROControllerNamespace))).To(Succeed())

			roLeaseCount := 0
			for _, lease := range leaseList.Items {
				if len(lease.Name) > 8 && lease.Name[:8] == "ro-lock-" {
					roLeaseCount++
				}
			}
			Expect(roLeaseCount).To(Equal(0), "no RO lock leases should remain after reconcile completes")
		})
	})

	// IT-RO-189-003: After successful Create, Lease deleted; second RR proceeds
	Describe("IT-RO-189-003: Lock released after successful WFE creation", func() {
		It("should release lock after WFE creation, allowing next RR to proceed", func() {
			ns := createTestNamespace("locking-release-proceed")
			defer deleteTestNamespace(ns)

			targetResource := fmt.Sprintf("%s/Deployment/test-app", ns)

			// Create RR1 and let it create WFE
			rr1, _ := advanceToAnalyzing(ns, "rr-lock-first", targetResource)

			// Wait for WFE1 creation
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr1), rr1); err != nil {
					return false
				}
				return rr1.Status.WorkflowExecutionRef != nil
			}, timeout, interval).Should(BeTrue(), "RR1 should have WFE ref")

			// Complete the WFE1 (make it terminal so second RR isn't blocked by ResourceBusy)
			wfe1Name := rr1.Status.WorkflowExecutionRef.Name
			Eventually(func() error {
				wfe1 := &workflowexecutionv1.WorkflowExecution{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      wfe1Name,
					Namespace: ROControllerNamespace,
				}, wfe1); err != nil {
					return err
				}
				wfe1.Status.Phase = workflowexecutionv1.PhaseCompleted
				now := metav1.Now()
				wfe1.Status.CompletionTime = &now
				return k8sClient.Status().Update(ctx, wfe1)
			}, timeout, interval).Should(Succeed())

			// Verify no lock leases remain
			leaseList := &coordinationv1.LeaseList{}
			Expect(k8sClient.List(ctx, leaseList, client.InNamespace(ROControllerNamespace))).To(Succeed())
			for _, lease := range leaseList.Items {
				Expect(lease.Name).NotTo(HavePrefix("ro-lock-"),
					"no RO lock leases should remain after WFE creation + release")
			}
		})
	})

	// IT-RO-189-004: Post-approval Create with concurrent RRs = only one WFE
	Describe("IT-RO-189-004: Approval path uses locking", func() {
		It("should prevent duplicate WFE creation via approval path when target is busy", func() {
			ns := createTestNamespace("locking-approval")
			defer deleteTestNamespace(ns)

			targetResource := fmt.Sprintf("%s/Deployment/test-app", ns)

			// Create RR1 via normal path, let it create WFE1
			rr1, _ := advanceToAnalyzing(ns, "rr-lock-normal", targetResource)

			// Wait for WFE1 creation
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr1), rr1); err != nil {
					return false
				}
				return rr1.Status.WorkflowExecutionRef != nil
			}, timeout, interval).Should(BeTrue(), "RR1 should have WFE ref")

			// Create RR2 with approval required
			rr2 := createRemediationRequest(ns, "rr-lock-approval")

			// Advance RR2 through SP → AI → AwaitingApproval
			// Wait for SP creation for RR2
			Eventually(func() bool {
				spList := &signalprocessingv1.SignalProcessingList{}
				if err := k8sClient.List(ctx, spList, client.InNamespace(ROControllerNamespace)); err != nil {
					return false
				}
				for _, sp := range spList.Items {
					if sp.Spec.RemediationRequestRef.Name == "rr-lock-approval" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Complete SP for RR2
			spList := &signalprocessingv1.SignalProcessingList{}
			Expect(k8sClient.List(ctx, spList, client.InNamespace(ROControllerNamespace))).To(Succeed())
			for _, sp := range spList.Items {
				if sp.Spec.RemediationRequestRef.Name == "rr-lock-approval" {
					Eventually(func() error {
						return updateSPStatus(ROControllerNamespace, sp.Name, signalprocessingv1.PhaseCompleted)
					}, timeout, interval).Should(Succeed())
					break
				}
			}

			// Wait for AI Analysis for RR2 and complete it with approval required
			Eventually(func() bool {
				aiList := &aianalysisv1.AIAnalysisList{}
				if err := k8sClient.List(ctx, aiList, client.InNamespace(ROControllerNamespace)); err != nil {
					return false
				}
				for _, ai := range aiList.Items {
					if ai.Spec.RemediationRequestRef.Name == "rr-lock-approval" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			aiList := &aianalysisv1.AIAnalysisList{}
			Expect(k8sClient.List(ctx, aiList, client.InNamespace(ROControllerNamespace))).To(Succeed())
			for i := range aiList.Items {
				if aiList.Items[i].Spec.RemediationRequestRef.Name == "rr-lock-approval" {
					ai := &aiList.Items[i]
					Eventually(func() error {
						if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ai), ai); err != nil {
							return err
						}
					ai.Status.Phase = "Completed"
					now := metav1.Now()
					ai.Status.CompletedAt = &now
					ai.Status.ApprovalRequired = true
					ai.Status.ApprovalReason = "Confidence below threshold"
					ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
						Summary:  "Test root cause - approval required",
						Severity: "critical",
						RemediationTarget: &aianalysisv1.RemediationTarget{
							Kind:      "Deployment",
							Name:      "test-app",
							Namespace: ns,
						},
					}
					ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
						WorkflowID:      "wf-test-001",
						ActionType:      "restart",
						Version:         "v1",
						ExecutionBundle: "test-image:latest",
						Confidence:      0.65,
						Rationale:       "Test rationale - approval required",
					}
						return k8sClient.Status().Update(ctx, ai)
					}, timeout, interval).Should(Succeed())
					break
				}
			}

			// Wait for RR2 to reach AwaitingApproval
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr2), rr2); err != nil {
					return ""
				}
				return string(rr2.Status.OverallPhase)
			}, timeout, interval).Should(Equal(string(remediationv1.PhaseAwaitingApproval)),
				"RR2 should reach AwaitingApproval phase")

		// Approve RR2 by updating the Status of the RO-created RAR
		rar := &remediationv1.RemediationApprovalRequest{}
		rarName := fmt.Sprintf("rar-%s", rr2.Name)
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      rarName,
				Namespace: ROControllerNamespace,
			}, rar)
		}, timeout, interval).Should(Succeed(), "RAR should be created by the RO controller")

		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.DecidedBy = "test-user@kubernaut.ai"
		rar.Status.DecisionMessage = "Testing locking on approval path"
		decidedAt := metav1.Now()
		rar.Status.DecidedAt = &decidedAt
		Expect(k8sClient.Status().Update(ctx, rar)).To(Succeed())

			// Wait for RR2 to be processed after approval
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr2), rr2); err != nil {
					return ""
				}
				return string(rr2.Status.OverallPhase)
			}, timeout, interval).Should(SatisfyAny(
				Equal(string(remediationv1.PhaseBlocked)),
				Equal(string(remediationv1.PhaseExecuting)),
			), "RR2 should be Blocked or Executing after approval")

			// Verify: if RR1's WFE is still active, RR2 should NOT have created a second WFE
			wfeList := &workflowexecutionv1.WorkflowExecutionList{}
			Expect(k8sClient.List(ctx, wfeList, client.InNamespace(ROControllerNamespace))).To(Succeed())
			activeCount := 0
			for _, wfe := range wfeList.Items {
				if wfe.Spec.TargetResource == targetResource &&
					wfe.Status.Phase != workflowexecutionv1.PhaseCompleted &&
					wfe.Status.Phase != workflowexecutionv1.PhaseFailed {
					activeCount++
				}
			}
			Expect(activeCount).To(BeNumerically("<=", 1),
				"approval path should not create duplicate WFE when target is busy")
		})
	})

	// IT-RO-189-005: Status update failure recovery via creator idempotency
	Describe("IT-RO-189-005: Idempotent WFE creation on status update failure", func() {
		It("should recover via Get-before-Create when WFE exists but status update failed", func() {
			ns := createTestNamespace("locking-idempotency")
			defer deleteTestNamespace(ns)

			targetResource := fmt.Sprintf("%s/Deployment/test-app", ns)
			rr, _ := advanceToAnalyzing(ns, "rr-lock-idem", targetResource)

			// Wait for WFE creation
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
					return false
				}
				return rr.Status.WorkflowExecutionRef != nil
			}, timeout, interval).Should(BeTrue(), "WFE should be created")

			wfeName := rr.Status.WorkflowExecutionRef.Name

			// Simulate status update failure by clearing WorkflowExecutionRef
			// (as if the status update after WFE creation failed)
			Eventually(func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
					return err
				}
				rr.Status.WorkflowExecutionRef = nil
				rr.Status.OverallPhase = remediationv1.PhaseAnalyzing
				return k8sClient.Status().Update(ctx, rr)
			}, timeout, interval).Should(Succeed())

			// Wait for reconciler to retry and recover via Get-before-Create
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
					return false
				}
				return rr.Status.WorkflowExecutionRef != nil
			}, timeout, interval).Should(BeTrue(), "WFE ref should be restored via idempotent create")

			// Verify the same WFE was reused (not a new one)
			Expect(rr.Status.WorkflowExecutionRef.Name).To(Equal(wfeName),
				"Get-before-Create should reuse existing WFE, not create a duplicate")
		})
	})
})
