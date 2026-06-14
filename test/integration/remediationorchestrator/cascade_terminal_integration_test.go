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

package remediationorchestrator

import (
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ============================================================================
// CASCADE TERMINAL INTEGRATION TESTS (#1421)
//
// FedRAMP Control Objectives:
//   IR-4 (Incident Handling): When an operator cancels a remediation through the
//     Console, ALL downstream child CRDs (SP, AI, WE) MUST transition to a
//     terminal state. This ensures automated incident response mechanisms do not
//     continue operating after human-initiated cancellation.
//   AC-6 (Least Privilege): Active child CRDs (AI sessions, workflow PipelineRuns)
//     hold elevated cluster access. Cascade termination revokes these promptly.
//   AU-12 (Audit Generation): The cascade must produce observable state transitions
//     in the K8s API (status patches) that are auditable via standard watch mechanisms.
//
// Test Strategy:
//   These tests exercise the REAL RO controller reconcile loop via envtest.
//   RR is patched to Cancelled externally (simulating Console action), then
//   we verify child CRD status transitions via Eventually assertions.
// ============================================================================

var _ = Describe("Cascade Terminal to Children (#1421) [IR-4, AC-6, AU-12]", Label("integration", "cascade"), func() {

	var (
		namespace string
	)

	BeforeEach(func() {
		namespace = createTestNamespace("ro-cascade")
	})

	AfterEach(func() {
		deleteTestNamespace(namespace)
	})

	// IT-RO-1421-001: RR cancelled → AI transitions to Failed with ParentCancelled
	// Exercises the production RO reconcile loop → cascadeTerminalToChildren → AI status patch.
	// FedRAMP IR-4(1): Automated response terminates active investigation when parent is cancelled.
	It("IT-RO-1421-001: RR cancelled via status patch → AI transitions to Failed [IR-4(1)]", func() {
		rrName := fmt.Sprintf("rr-cascade-%s", uuid.New().String()[:8])
		aiName := fmt.Sprintf("ai-%s", rrName)
		spName := fmt.Sprintf("sp-%s", rrName)

		By("Creating an RR already in terminal (Cancelled) phase with child refs")
		now := metav1.Now()
		rr := createRemediationRequest(namespace, rrName)
		Expect(rr.UID).ToNot(BeEmpty(), "createRemediationRequest should populate UID after Create")

		By("Creating an AIAnalysis in Investigating phase (child of RR)")
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aiName,
				Namespace: ROControllerNamespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: remediationv1.GroupVersion.String(),
						Kind:       "RemediationRequest",
						Name:       rrName,
						UID:        rr.UID,
						Controller: ptrBool(true),
					},
				},
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       rrName,
					Namespace:  ROControllerNamespace,
				},
				RemediationID: rrName,
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
						Severity:         "critical",
						SignalName:       "TestHighMemoryAlert",
						Environment:      "test",
						BusinessPriority: "P1",
					},
					AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ai)).To(Succeed())

		By("Setting AI status to Investigating")
		ai.Status.Phase = aianalysisv1.PhaseInvestigating
		ai.Status.StartedAt = &now
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		By("Creating a SignalProcessing in Enriching phase (child of RR)")
		sp := &signalprocessingv1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      spName,
				Namespace: ROControllerNamespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: remediationv1.GroupVersion.String(),
						Kind:       "RemediationRequest",
						Name:       rrName,
						UID:        rr.UID,
						Controller: ptrBool(true),
					},
				},
			},
			Spec: signalprocessingv1.SignalProcessingSpec{
				RemediationRequestRef: signalprocessingv1.ObjectReference{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       rrName,
					Namespace:  ROControllerNamespace,
				},
			},
		}
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())

		sp.Status.Phase = signalprocessingv1.PhaseEnriching
		Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

		By("Patching RR status to Cancelled with child refs (simulating Console cancellation)")
		Eventually(func(g Gomega) {
			fetched := &remediationv1.RemediationRequest{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), fetched)).To(Succeed())

			fetched.Status.OverallPhase = remediationv1.PhaseCancelled
			fetched.Status.StartTime = &now
			fetched.Status.AIAnalysisRef = &corev1.ObjectReference{
				APIVersion: aianalysisv1.GroupVersion.String(),
				Kind:       "AIAnalysis",
				Name:       aiName,
				Namespace:  ROControllerNamespace,
			}
			fetched.Status.SignalProcessingRef = &corev1.ObjectReference{
				APIVersion: signalprocessingv1.GroupVersion.String(),
				Kind:       "SignalProcessing",
				Name:       spName,
				Namespace:  ROControllerNamespace,
			}
			g.Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		By("Verifying AI transitions to Failed with ParentCancelled reason [IR-4(1)]")
		Eventually(func(g Gomega) {
			fetchedAI := &aianalysisv1.AIAnalysis{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, fetchedAI)).To(Succeed())
			g.Expect(fetchedAI.Status.Phase).To(Equal(aianalysisv1.PhaseFailed),
				"AI must transition to Failed when parent RR is Cancelled")
			g.Expect(fetchedAI.Status.Reason).To(Equal(aianalysisv1.ReasonParentCancelled),
				"Reason must be ParentCancelled for audit trail [AU-12]")
			g.Expect(fetchedAI.Status.Message).To(ContainSubstring("terminal phase"),
				"Message must indicate parent termination for operator visibility")
			g.Expect(fetchedAI.Status.CompletedAt).ToNot(BeNil(),
				"CompletedAt must be set for lifecycle completeness")
		}, timeout, interval).Should(Succeed())

		By("Verifying SP transitions to Failed [AC-6]")
		Eventually(func(g Gomega) {
			fetchedSP := &signalprocessingv1.SignalProcessing{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, fetchedSP)).To(Succeed())
			g.Expect(fetchedSP.Status.Phase).To(Equal(signalprocessingv1.PhaseFailed),
				"SP must transition to Failed when parent RR is Cancelled")
		}, timeout, interval).Should(Succeed())
	})

	// IT-RO-1421-002: Idempotent cascade — already-terminal children not overwritten
	// FedRAMP CM-3: Configuration integrity — repeated reconciles must not corrupt state.
	It("IT-RO-1421-002: already-terminal AI is not overwritten on cascade [CM-3]", func() {
		rrName := fmt.Sprintf("rr-idem-%s", uuid.New().String()[:8])
		aiName := fmt.Sprintf("ai-%s", rrName)

		By("Creating RR")
		now := metav1.Now()
		rr := createRemediationRequest(namespace, rrName)
		Expect(rr.UID).ToNot(BeEmpty(), "createRemediationRequest should populate UID after Create")

		By("Creating AI already in Completed state")
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aiName,
				Namespace: ROControllerNamespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: remediationv1.GroupVersion.String(),
						Kind:       "RemediationRequest",
						Name:       rrName,
						UID:        rr.UID,
						Controller: ptrBool(true),
					},
				},
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       rrName,
					Namespace:  ROControllerNamespace,
				},
				RemediationID: rrName,
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
						Severity:         "critical",
						SignalName:       "TestHighMemoryAlert",
						Environment:      "test",
						BusinessPriority: "P1",
					},
					AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ai)).To(Succeed())
		ai.Status.Phase = aianalysisv1.PhaseCompleted
		ai.Status.Reason = aianalysisv1.ReasonAnalysisCompleted
		ai.Status.CompletedAt = &now
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		By("Patching RR to Cancelled with AI ref")
		Eventually(func(g Gomega) {
			fetched := &remediationv1.RemediationRequest{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), fetched)).To(Succeed())
			fetched.Status.OverallPhase = remediationv1.PhaseCancelled
			fetched.Status.StartTime = &now
			fetched.Status.AIAnalysisRef = &corev1.ObjectReference{
				APIVersion: aianalysisv1.GroupVersion.String(),
				Kind:       "AIAnalysis",
				Name:       aiName,
				Namespace:  ROControllerNamespace,
			}
			g.Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		By("Verifying AI remains Completed (not overwritten) [CM-3]")
		Consistently(func(g Gomega) {
			fetchedAI := &aianalysisv1.AIAnalysis{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, fetchedAI)).To(Succeed())
			g.Expect(fetchedAI.Status.Phase).To(Equal(aianalysisv1.PhaseCompleted),
				"Already-terminal AI must NOT be overwritten")
			g.Expect(fetchedAI.Status.Reason).To(Equal(aianalysisv1.ReasonAnalysisCompleted),
				"Original reason must be preserved")
		}, "5s", interval).Should(Succeed())
	})

	// IT-RO-1421-003: Cascade to WorkflowExecution
	// FedRAMP AC-6: Active PipelineRun with elevated RBAC must be terminated.
	It("IT-RO-1421-003: WE transitions to Failed when RR is Cancelled [AC-6]", func() {
		rrName := fmt.Sprintf("rr-we-%s", uuid.New().String()[:8])
		weName := fmt.Sprintf("we-%s", rrName)

		By("Creating RR")
		now := metav1.Now()
		rr := createRemediationRequest(namespace, rrName)
		Expect(rr.UID).ToNot(BeEmpty(), "createRemediationRequest should populate UID after Create")

		By("Creating WE in Running phase")
		we := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      weName,
				Namespace: ROControllerNamespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: remediationv1.GroupVersion.String(),
						Kind:       "RemediationRequest",
						Name:       rrName,
						UID:        rr.UID,
						Controller: ptrBool(true),
					},
				},
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       rrName,
					Namespace:  ROControllerNamespace,
				},
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					WorkflowID:      "restart-deployment",
					Version:         "v1",
					ExecutionBundle: "ghcr.io/kubernaut/workflows/restart:v1",
				},
				TargetResource: fmt.Sprintf("%s/deployment/test-app", ROControllerNamespace),
			},
		}
		Expect(k8sClient.Create(ctx, we)).To(Succeed())
		we.Status.Phase = workflowexecutionv1.PhaseRunning
		Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

		By("Patching RR to Cancelled with WE ref")
		Eventually(func(g Gomega) {
			fetched := &remediationv1.RemediationRequest{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), fetched)).To(Succeed())
			fetched.Status.OverallPhase = remediationv1.PhaseCancelled
			fetched.Status.StartTime = &now
			fetched.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				APIVersion: workflowexecutionv1.GroupVersion.String(),
				Kind:       "WorkflowExecution",
				Name:       weName,
				Namespace:  ROControllerNamespace,
			}
			g.Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		By("Verifying WE transitions to Failed [AC-6]")
		Eventually(func(g Gomega) {
			fetchedWE := &workflowexecutionv1.WorkflowExecution{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, fetchedWE)).To(Succeed())
			g.Expect(fetchedWE.Status.Phase).To(Equal(workflowexecutionv1.PhaseFailed),
				"WE must transition to Failed when parent RR is Cancelled")
			g.Expect(fetchedWE.Status.FailureReason).To(ContainSubstring("terminal phase"),
				"FailureReason must indicate parent termination [AU-12]")
		}, timeout, interval).Should(Succeed())
	})
})

func ptrBool(b bool) *bool {
	return &b
}
