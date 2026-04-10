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

// Integration tests for Issue #190: Dedup Result Propagation (Cross-WE Watch).
// These tests validate that the RO controller:
//   - Detects DeduplicatedByWE on the RR after the WE handler sets it
//   - Fetches the original WFE and propagates its terminal result
//   - Handles dangling references (original WFE deleted)
//   - Does NOT increment ConsecutiveFailureCount for inherited failures
//
// Test Strategy:
//   - Real envtest with RO controller running
//   - Manually drive RR through SP → AI → Executing
//   - Set WFE to Failed/Deduplicated with DeduplicatedBy
//   - Create original WFE in cluster and verify propagation
//
// Defense-in-Depth:
//   - Unit tests: Mock client, isolated handler/reconciler logic (dedup_handler_test.go)
//   - Integration tests: Real K8s API with reconciler loop (this file)

package remediationorchestrator

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// driveToExecuting drives an RR through SP → AI → Executing by manually completing child CRDs.
// Returns the RR in Executing phase with a WFE created by the controller.
func driveToExecuting(ns, rrName string) *remediationv1.RemediationRequest {
	rr := createRemediationRequest(ns, rrName)

	Eventually(func() remediationv1.RemediationPhase {
		_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
		return rr.Status.OverallPhase
	}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

	spName := fmt.Sprintf("sp-%s", rr.Name)
	sp := &signalprocessingv1.SignalProcessing{}
	Eventually(func() error {
		return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
	}, timeout, interval).Should(Succeed())
	Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

	Eventually(func() remediationv1.RemediationPhase {
		_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
		return rr.Status.OverallPhase
	}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

	aiName := fmt.Sprintf("ai-%s", rr.Name)
	ai := &aianalysisv1.AIAnalysis{}
	Eventually(func() error {
		return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, ai)
	}, timeout, interval).Should(Succeed())
	ai.Status.Phase = aianalysisv1.PhaseCompleted
	ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
		WorkflowID:      "wf-restart-pods",
		Version:         "v1.0.0",
		ExecutionBundle: "test-image:latest",
		Confidence:      0.95,
	}
	ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
		Summary:    "Dedup test RCA",
		Severity:   "critical",
		SignalType: "alert",
		RemediationTarget: &aianalysisv1.RemediationTarget{
			Kind:      "Deployment",
			Name:      "test-app",
			Namespace: ns,
		},
	}
	now := metav1.Now()
	ai.Status.CompletedAt = &now
	Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

	Eventually(func() remediationv1.RemediationPhase {
		_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
		return rr.Status.OverallPhase
	}, timeout, interval).Should(Equal(remediationv1.PhaseExecuting))

	return rr
}

var _ = Describe("Issue #190: Dedup Result Propagation Integration", Label("integration", "dedup"), func() {
	var ns string

	BeforeEach(func() {
		ns = createTestNamespace("dedup-prop")
	})

	AfterEach(func() {
		deleteTestNamespace(ns)
	})

	It("IT-RO-190-001: RR inherits Completed from original WFE after dedup", func() {
		rr := driveToExecuting(ns, "rr-dedup-001")

		originalWFEName := "original-wfe-it-001"
		By("Creating the original WFE as Completed before any dedup reference (avoids RO NotFound → Failed race)")
		originalWFE := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      originalWFEName,
				Namespace: ROControllerNamespace,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					WorkflowID: "wf-restart-pods",
					Version:    "v1.0.0",
				},
				TargetResource: ns + "/deployment/test-app",
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{
				Phase: workflowexecutionv1.PhaseCompleted,
			},
		}
		Expect(k8sClient.Create(ctx, originalWFE)).To(Succeed())
		completionTime := metav1.Now()
		originalWFE.Status.CompletionTime = &completionTime
		Expect(k8sClient.Status().Update(ctx, originalWFE)).To(Succeed())

		By("Marking the WFE as Failed/Deduplicated")
		weName := fmt.Sprintf("we-%s", rr.Name)
		we := &workflowexecutionv1.WorkflowExecution{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
		}, timeout, interval).Should(Succeed())

		we.Status.Phase = workflowexecutionv1.PhaseFailed
		we.Status.FailureDetails = &workflowexecutionv1.FailureDetails{
			Reason:   workflowexecutionv1.FailureReasonDeduplicated,
			FailedAt: metav1.Now(),
		}
		we.Status.DeduplicatedBy = originalWFEName
		Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

		By("Waiting for DeduplicatedByWE to be set on the RR")
		Eventually(func() string {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.DeduplicatedByWE
		}, timeout, interval).Should(Equal(originalWFEName))

		By("Verifying RR inherits Completed")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted))

		Expect(rr.Status.Outcome).To(Equal("Remediated"),
			"Behavior: outcome must be Remediated (lineage tracked via DeduplicatedByWE + K8s events)")
		Expect(rr.Status.CompletedAt).NotTo(BeNil())
	})

	It("IT-RO-190-002: RR inherits Failed from original WFE with FailurePhaseDeduplicated", func() {
		rr := driveToExecuting(ns, "rr-dedup-002")

		By("Marking the WFE as Failed/Deduplicated")
		weName := fmt.Sprintf("we-%s", rr.Name)
		we := &workflowexecutionv1.WorkflowExecution{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
		}, timeout, interval).Should(Succeed())

		originalWFEName := "original-wfe-it-002"
		we.Status.Phase = workflowexecutionv1.PhaseFailed
		we.Status.FailureDetails = &workflowexecutionv1.FailureDetails{
			Reason:   workflowexecutionv1.FailureReasonDeduplicated,
			FailedAt: metav1.Now(),
		}
		we.Status.DeduplicatedBy = originalWFEName
		Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

		By("Waiting for DeduplicatedByWE to be set on the RR")
		Eventually(func() string {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.DeduplicatedByWE
		}, timeout, interval).Should(Equal(originalWFEName))

		By("Creating the original WFE as Failed")
		originalWFE := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      originalWFEName,
				Namespace: ROControllerNamespace,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					WorkflowID: "wf-restart-pods",
					Version:    "v1.0.0",
				},
				TargetResource: ns + "/deployment/test-app",
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{
				Phase: workflowexecutionv1.PhaseFailed,
			},
		}
		Expect(k8sClient.Create(ctx, originalWFE)).To(Succeed())
		originalWFE.Status.FailureReason = "OOM killed"
		Expect(k8sClient.Status().Update(ctx, originalWFE)).To(Succeed())

		By("Verifying RR inherits Failed with FailurePhaseDeduplicated")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))

		Expect(rr.Status.FailurePhase).NotTo(BeNil())
		Expect(*rr.Status.FailurePhase).To(Equal(remediationv1.FailurePhaseDeduplicated))
		Expect(rr.Status.CompletedAt).NotTo(BeNil())
	})

	It("IT-RO-190-003: RR fails when original WFE is deleted (dangling reference)", func() {
		rr := driveToExecuting(ns, "rr-dedup-003")

		By("Marking the WFE as Failed/Deduplicated pointing to non-existent WFE")
		weName := fmt.Sprintf("we-%s", rr.Name)
		we := &workflowexecutionv1.WorkflowExecution{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
		}, timeout, interval).Should(Succeed())

		we.Status.Phase = workflowexecutionv1.PhaseFailed
		we.Status.FailureDetails = &workflowexecutionv1.FailureDetails{
			Reason:   workflowexecutionv1.FailureReasonDeduplicated,
			FailedAt: metav1.Now(),
		}
		we.Status.DeduplicatedBy = "deleted-wfe-it-003"
		Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

		By("Waiting for RR to fail with FailurePhaseDeduplicated (dangling reference)")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))

		Expect(rr.Status.FailurePhase).NotTo(BeNil())
		Expect(*rr.Status.FailurePhase).To(Equal(remediationv1.FailurePhaseDeduplicated))
	})

	It("IT-RO-190-004: RR stays Executing while original WFE is Running, then inherits Completed", func() {
		rr := driveToExecuting(ns, "rr-dedup-004")

		originalWFEName := "original-wfe-it-004"
		By("Creating the original WFE as Running before any dedup reference (avoids RO NotFound → Failed race)")
		originalWFE := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      originalWFEName,
				Namespace: ROControllerNamespace,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					WorkflowID: "wf-restart-pods",
					Version:    "v1.0.0",
				},
				TargetResource: ns + "/deployment/test-app",
			},
		}
		Expect(k8sClient.Create(ctx, originalWFE)).To(Succeed())
		originalWFE.Status.Phase = workflowexecutionv1.PhaseRunning
		Expect(k8sClient.Status().Update(ctx, originalWFE)).To(Succeed())

		By("Marking the WFE as Failed/Deduplicated")
		weName := fmt.Sprintf("we-%s", rr.Name)
		we := &workflowexecutionv1.WorkflowExecution{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
		}, timeout, interval).Should(Succeed())

		we.Status.Phase = workflowexecutionv1.PhaseFailed
		we.Status.FailureDetails = &workflowexecutionv1.FailureDetails{
			Reason:   workflowexecutionv1.FailureReasonDeduplicated,
			FailedAt: metav1.Now(),
		}
		we.Status.DeduplicatedBy = originalWFEName
		Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

		By("Waiting for DeduplicatedByWE to be set")
		Eventually(func() string {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.DeduplicatedByWE
		}, timeout, interval).Should(Equal(originalWFEName))

		By("Verifying RR stays Executing while original is Running")
		Consistently(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, 5*time.Second, interval).Should(Equal(remediationv1.PhaseExecuting))

		By("Completing the original WFE")
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(originalWFE), originalWFE)
		}, timeout, interval).Should(Succeed())
		originalWFE.Status.Phase = workflowexecutionv1.PhaseCompleted
		completionTime := metav1.Now()
		originalWFE.Status.CompletionTime = &completionTime
		Expect(k8sClient.Status().Update(ctx, originalWFE)).To(Succeed())

		By("Verifying RR eventually inherits Completed")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted))

		Expect(rr.Status.Outcome).To(Equal("Remediated"))
	})

	It("IT-RO-190-006: Inherited failure does NOT increment ConsecutiveFailureCount", func() {
		rr := driveToExecuting(ns, "rr-dedup-006")

		By("Marking the WFE as Failed/Deduplicated pointing to non-existent WFE (fast failure path)")
		weName := fmt.Sprintf("we-%s", rr.Name)
		we := &workflowexecutionv1.WorkflowExecution{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
		}, timeout, interval).Should(Succeed())

		we.Status.Phase = workflowexecutionv1.PhaseFailed
		we.Status.FailureDetails = &workflowexecutionv1.FailureDetails{
			Reason:   workflowexecutionv1.FailureReasonDeduplicated,
			FailedAt: metav1.Now(),
		}
		we.Status.DeduplicatedBy = "nonexistent-wfe-it-006"
		Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

		By("Waiting for RR to fail with FailurePhaseDeduplicated")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))

		By("Verifying ConsecutiveFailureCount was NOT incremented")
		Expect(rr.Status.ConsecutiveFailureCount).To(Equal(int32(0)),
			"Behavior: inherited failures must NOT increment consecutive failure count (Phase 5)")
	})
})
