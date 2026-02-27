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

// DD-EVENT-001: RemediationOrchestrator K8s Event Observability Integration Tests
//
// BR-ORCH-095: K8s Event Observability business requirements
//
// These tests validate event emission in the context of the envtest framework
// with real EventRecorder (k8sManager.GetEventRecorderFor). They use the
// pattern: create CR → wait for target phase → list corev1.Events filtered
// by involvedObject.name → assert expected event reasons.
//
// IMPORTANT: These tests require the full integration environment (CRDs,
// DataStorage, etc.) to run. Structure compiles; execution depends on
// `make test-integration-remediationorchestrator`.
package remediationorchestrator

import (
	"context"
	"fmt"
	"sort"

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
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// listEventsForObject returns corev1.Events for the given object name in the namespace,
// sorted by FirstTimestamp for deterministic ordering.
func listEventsForObjectRO(ctx context.Context, c client.Client, objectName, namespace string) ([]corev1.Event, error) {
	list := &corev1.EventList{}
	if err := c.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	var result []corev1.Event
	for _, e := range list.Items {
		if e.InvolvedObject.Name == objectName {
			result = append(result, e)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].FirstTimestamp.Before(&result[j].FirstTimestamp)
	})
	return result, nil
}

// eventReasonsRO returns the ordered list of event reasons from the given events.
func eventReasonsRO(evts []corev1.Event) []string {
	reasons := make([]string, len(evts))
	for i, e := range evts {
		reasons[i] = e.Reason
	}
	return reasons
}

func containsReasonRO(reasons []string, reason string) bool {
	for _, r := range reasons {
		if r == reason {
			return true
		}
	}
	return false
}

var _ = Describe("RemediationOrchestrator K8s Event Observability (DD-EVENT-001, BR-ORCH-095)", Label("integration", "events"), func() {

	Context("IT-RO-095-01: Happy path event trail (auto-approve)", func() {
		It("should emit RemediationCreated, PhaseTransition, RemediationCompleted when lifecycle completes", func() {
			namespace := createTestNamespace("ro-events-happy")
			defer deleteTestNamespace(namespace)

			rrName := fmt.Sprintf("rr-events-happy-%s", uuid.New().String()[:13])
			rr := createRemediationRequest(namespace, rrName)

			By("Waiting for SignalProcessing to be created")
			spName := fmt.Sprintf("sp-%s", rrName)
			Eventually(func() error {
				sp := &signalprocessingv1.SignalProcessing{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
			}, timeout, interval).Should(Succeed())

			By("Simulating SignalProcessing completion")
			Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

			By("Waiting for AIAnalysis to be created")
			aiName := fmt.Sprintf("ai-%s", rrName)
			Eventually(func() error {
				ai := &aianalysisv1.AIAnalysis{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, ai)
			}, timeout, interval).Should(Succeed())

			By("Simulating AIAnalysis completion (high confidence, no approval)")
			ai := &aianalysisv1.AIAnalysis{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, ai)).To(Succeed())
			ai.Status.Phase = "Completed"
			ai.Status.ApprovalRequired = false
			ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:     "wf-restart-pods",
				Version:        "v1.0.0",
				Confidence:     0.95,
				ExecutionBundle: "kubernaut/workflows:latest",
				Rationale:      "High confidence auto-approve",
			}
			// DD-HAPI-006: AffectedResource is required for routing to WorkflowExecution
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary:    "OOM kill detected",
				Severity:   "critical",
				SignalType: "alert",
				AffectedResource: &aianalysisv1.AffectedResource{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: namespace,
				},
			}
			now := metav1.Now()
			ai.Status.CompletedAt = &now
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			By("Waiting for Executing phase (WorkflowExecution created)")
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseExecuting))

			By("Simulating WorkflowExecution completion")
			weName := fmt.Sprintf("we-%s", rrName)
			we := &workflowexecutionv1.WorkflowExecution{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: weName, Namespace: ROControllerNamespace}, we)
			}, timeout, interval).Should(Succeed())
			we.Status.Phase = workflowexecutionv1.PhaseCompleted
			completionTime := metav1.Now()
			we.Status.CompletionTime = &completionTime
			Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

			By("Waiting for Completed phase")
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted))

			By("Listing events and asserting RemediationCreated, PhaseTransition, RemediationCompleted")
			var evts []corev1.Event
			Eventually(func() bool {
				var err error
				evts, err = listEventsForObjectRO(ctx, k8sClient, rrName, ROControllerNamespace)
				if err != nil {
					return false
				}
				reasons := eventReasonsRO(evts)
				return containsReasonRO(reasons, events.EventReasonRemediationCreated) &&
					containsReasonRO(reasons, events.EventReasonPhaseTransition) &&
					containsReasonRO(reasons, events.EventReasonRemediationCompleted)
			}, timeout, interval).Should(BeTrue())

			reasons := eventReasonsRO(evts)
			Expect(containsReasonRO(reasons, events.EventReasonRemediationCreated)).To(BeTrue())
			Expect(containsReasonRO(reasons, events.EventReasonPhaseTransition)).To(BeTrue())
			Expect(containsReasonRO(reasons, events.EventReasonRemediationCompleted)).To(BeTrue())
		})
	})

	Context("IT-RO-095-02: Approval flow event trail", func() {
		It("should emit RemediationCreated, ApprovalRequired, ApprovalGranted, RemediationCompleted when approval flow completes", func() {
			namespace := createTestNamespace("ro-events-approval")
			defer deleteTestNamespace(namespace)

			rrName := fmt.Sprintf("rr-events-appr-%s", uuid.New().String()[:13])
			_ = createRemediationRequest(namespace, rrName)
			rr := &remediationv1.RemediationRequest{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, rr)).To(Succeed())

			By("Progressing through SP phase")
			spName := fmt.Sprintf("sp-%s", rrName)
			Eventually(func() error {
				sp := &signalprocessingv1.SignalProcessing{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
			}, timeout, interval).Should(Succeed())
			Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

			By("Progressing through AI with approval required")
			aiName := fmt.Sprintf("ai-%s", rrName)
			Eventually(func() error {
				ai := &aianalysisv1.AIAnalysis{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, ai)
			}, timeout, interval).Should(Succeed())

			ai := &aianalysisv1.AIAnalysis{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, ai)).To(Succeed())
			ai.Status.Phase = "Completed"
			ai.Status.ApprovalRequired = true
			ai.Status.ApprovalReason = "Confidence below threshold"
			ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:     "wf-restart-pods",
				Version:        "v1.0.0",
				Confidence:     0.70,
				ExecutionBundle: "kubernaut/workflows:latest",
				Rationale:      "Restart recommended",
			}
			now := metav1.Now()
			ai.Status.CompletedAt = &now
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			By("Waiting for AwaitingApproval phase")
			Eventually(func() string {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return string(rr.Status.OverallPhase)
			}, timeout, interval).Should(Equal("AwaitingApproval"))

			By("Approving the RemediationApprovalRequest")
			rarName := fmt.Sprintf("rar-%s", rrName)
			rar := &remediationv1.RemediationApprovalRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)
			}, timeout, interval).Should(Succeed())
			rar.Status.Decision = remediationv1.ApprovalDecisionApproved
			rar.Status.DecidedBy = "test-admin@kubernaut.ai"
			rar.Status.DecisionMessage = "Approved for event test"
			decidedAt := metav1.Now()
			rar.Status.DecidedAt = &decidedAt
			Expect(k8sClient.Status().Update(ctx, rar)).To(Succeed())

			By("Waiting for Executing phase")
			Eventually(func() string {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return string(rr.Status.OverallPhase)
			}, timeout, interval).Should(Equal("Executing"))

			By("Simulating WorkflowExecution completion")
			weName := fmt.Sprintf("we-%s", rrName)
			we := &workflowexecutionv1.WorkflowExecution{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: weName, Namespace: ROControllerNamespace}, we)
			}, timeout, interval).Should(Succeed())
			we.Status.Phase = workflowexecutionv1.PhaseCompleted
			completionTime := metav1.Now()
			we.Status.CompletionTime = &completionTime
			Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

			By("Waiting for Completed phase")
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted))

			By("Listing events and asserting ApprovalRequired, ApprovalGranted, RemediationCompleted")
			var evts []corev1.Event
			Eventually(func() bool {
				var err error
				evts, err = listEventsForObjectRO(ctx, k8sClient, rrName, ROControllerNamespace)
				if err != nil {
					return false
				}
				reasons := eventReasonsRO(evts)
				return containsReasonRO(reasons, events.EventReasonRemediationCreated) &&
					containsReasonRO(reasons, events.EventReasonApprovalRequired) &&
					containsReasonRO(reasons, events.EventReasonApprovalGranted) &&
					containsReasonRO(reasons, events.EventReasonRemediationCompleted)
			}, timeout, interval).Should(BeTrue())

			reasons := eventReasonsRO(evts)
			Expect(containsReasonRO(reasons, events.EventReasonRemediationCreated)).To(BeTrue())
			Expect(containsReasonRO(reasons, events.EventReasonApprovalRequired)).To(BeTrue())
			Expect(containsReasonRO(reasons, events.EventReasonApprovalGranted)).To(BeTrue())
			Expect(containsReasonRO(reasons, events.EventReasonRemediationCompleted)).To(BeTrue())
		})
	})

	// IT-RO-095-03: Timeout event trail — NOT FEASIBLE in envtest.
	// CreationTimestamp is immutable (set by K8s API server), so we cannot create
	// an RR with an expired StartTime. Covered by unit tests (timeout_detector_test.go).
	// Ref: docs/handoff/RO_TIMEOUT_TESTS_TRIAGE_DEC_24_2025.md

	Context("IT-RO-095-04: Consecutive failure blocking event trail", func() {
		It("should emit RemediationCreated and ConsecutiveFailureBlocked when target at threshold", func() {
			namespace := createTestNamespace("ro-events-consecutive")
			defer deleteTestNamespace(namespace)

			fingerprint := GenerateTestFingerprint(namespace, "events-blocked")

			// Create and fail 3 consecutive RRs with same fingerprint
			for i := 1; i <= 3; i++ {
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("rr-cf-events-%d", i),
						Namespace: ROControllerNamespace,
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: fingerprint,
						SignalName:        "test-signal",
						Severity:          "critical",
						SignalType:        "test-type",
						TargetType:        "kubernetes",
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: namespace,
						},
						FiringTime:   metav1.Now(),
						ReceivedTime: metav1.Now(),
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: metav1.Now(),
							LastOccurrence:  metav1.Now(),
							OccurrenceCount: 1,
						},
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				Eventually(func() remediationv1.RemediationPhase {
					_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
					return rr.Status.OverallPhase
				}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

				spName := fmt.Sprintf("sp-%s", rr.Name)
				sp := &signalprocessingv1.SignalProcessing{}
				Eventually(func() error {
					return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: spName, Namespace: ROControllerNamespace}, sp)
				}, timeout, interval).Should(Succeed())
				sp.Status.Phase = signalprocessingv1.PhaseFailed
				sp.Status.Error = "Simulated failure for consecutive failure event test"
				Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

				Eventually(func() bool {
					_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
					return rr.Status.OverallPhase == remediationv1.PhaseFailed || rr.Status.OverallPhase == remediationv1.PhaseBlocked
				}, timeout, interval).Should(BeTrue())
			}

			// Create 4th RR - should be Blocked
			rr4Name := "rr-cf-events-4"
			rr4 := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rr4Name,
					Namespace: ROControllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "test-signal",
					Severity:          "critical",
					SignalType:        "test-type",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: namespace,
					},
					FiringTime:   metav1.Now(),
					ReceivedTime: metav1.Now(),
					Deduplication: sharedtypes.DeduplicationInfo{
						FirstOccurrence: metav1.Now(),
						LastOccurrence:  metav1.Now(),
						OccurrenceCount: 1,
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr4)).To(Succeed())

			By("Waiting for RR4 to transition to Blocked")
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr4), rr4)
				return rr4.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseBlocked))

			By("Listing events and asserting RemediationCreated, ConsecutiveFailureBlocked")
			var evts []corev1.Event
			Eventually(func() bool {
				var err error
				evts, err = listEventsForObjectRO(ctx, k8sClient, rr4Name, ROControllerNamespace)
				if err != nil {
					return false
				}
				reasons := eventReasonsRO(evts)
				return containsReasonRO(reasons, events.EventReasonRemediationCreated) &&
					containsReasonRO(reasons, events.EventReasonConsecutiveFailureBlocked)
			}, timeout, interval).Should(BeTrue())

			reasons := eventReasonsRO(evts)
			Expect(containsReasonRO(reasons, events.EventReasonRemediationCreated)).To(BeTrue())
			Expect(containsReasonRO(reasons, events.EventReasonConsecutiveFailureBlocked)).To(BeTrue())
		})
	})
})
