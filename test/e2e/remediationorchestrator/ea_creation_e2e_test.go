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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// E2E-RO-EA-001: EffectivenessAssessment CRD Creation on Completed Remediation
//
// Business Requirement: BR-EM-001 (Effectiveness Assessment), ADR-EM-001
// Architecture: BR-ORCH-031 (Cascade Deletion)
//
// Tests that RO creates an EffectivenessAssessment CRD when a RemediationRequest
// transitions to Completed phase after successful workflow execution.
//
// Full lifecycle: RR → SP → AA → WE → Completed → EA CRD
// Pattern: Manual child CRD status updates (no child controllers deployed)

var _ = Describe("E2E-RO-EA-001: EA Creation on Completion", Label("e2e", "ea", "remediationorchestrator"), func() {
	var (
		testNS string
	)

	BeforeEach(func() {
		testNS = createTestNamespace("ro-ea-e2e")
	})

	AfterEach(func() {
		deleteTestNamespace(testNS)
	})

	It("should create EffectivenessAssessment CRD when remediation completes successfully", func() {
		By("1. Creating RemediationRequest")
		rrName := "rr-ea-e2e-" + uuid.New().String()[:8]
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rrName,
				Namespace: controllerNamespace,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: func() string {
					h := sha256.Sum256([]byte(uuid.New().String()))
					return hex.EncodeToString(h[:])
				}(),
				SignalName: "HighCPU",
				Severity:   "critical",
				SignalType: "alert",
				TargetType: "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "test-app-ea",
					Namespace: testNS,
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
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

		By("2. Waiting for RO to create SignalProcessing CRD")
		var sp *signalprocessingv1.SignalProcessing
		Eventually(func() bool {
			spList := &signalprocessingv1.SignalProcessingList{}
			_ = k8sClient.List(ctx, spList, client.InNamespace(controllerNamespace))
			for i := range spList.Items {
				if len(spList.Items[i].OwnerReferences) > 0 &&
					spList.Items[i].OwnerReferences[0].Kind == "RemediationRequest" &&
					spList.Items[i].OwnerReferences[0].Name == rrName {
					sp = &spList.Items[i]
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "SignalProcessing should be created by RO")

		By("3. Manually updating SP status to Completed")
		sp.Status.Phase = signalprocessingv1.PhaseCompleted
		sp.Status.Severity = "critical"
		sp.Status.SignalMode = "reactive"
		sp.Status.SignalName = sp.Spec.Signal.Name // Issue #166: Use signal name, not type ("alert")
		sp.Status.EnvironmentClassification = &signalprocessingv1.EnvironmentClassification{
			Environment:  "production",
			Source:       "namespace-labels",
			ClassifiedAt: metav1.Now(),
		}
		sp.Status.PriorityAssignment = &signalprocessingv1.PriorityAssignment{
			Priority:   "P1",
			Source:     "rego-policy",
			AssignedAt: metav1.Now(),
		}
		Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

		By("4. Waiting for RO to create AIAnalysis CRD")
		var analysis *aianalysisv1.AIAnalysis
		Eventually(func() bool {
			analysisList := &aianalysisv1.AIAnalysisList{}
			_ = k8sClient.List(ctx, analysisList, client.InNamespace(controllerNamespace))
			for i := range analysisList.Items {
				if len(analysisList.Items[i].OwnerReferences) > 0 &&
					analysisList.Items[i].OwnerReferences[0].Kind == "RemediationRequest" &&
					analysisList.Items[i].OwnerReferences[0].Name == rrName {
					analysis = &analysisList.Items[i]
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "AIAnalysis should be created by RO")

		By("5. Manually updating AIAnalysis status to Completed with SelectedWorkflow")
		analysis.Status.Phase = aianalysisv1.PhaseCompleted
		analysis.Status.Reason = "AnalysisCompleted"
		analysis.Status.Message = "Workflow recommended: restart-deployment-v1"
		analysis.Status.RootCause = "CPU throttling due to resource limits"
		analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:     "restart-deployment-v1",
			Version:        "1.0.0",
			ExecutionBundle: "quay.io/kubernaut/restart-deployment:v1",
			Confidence:     0.92,
			Rationale:      "High confidence match for CPU remediation",
		}
		// DD-HAPI-006: AffectedResource is required for routing to WorkflowExecution
		analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:    "CPU throttling due to resource limits",
			Severity:   "critical",
			SignalType: "alert",
			AffectedResource: &aianalysisv1.AffectedResource{
				Kind:      "Deployment",
				Name:      "test-app-ea",
				Namespace: testNS,
			},
		}
		Expect(k8sClient.Status().Update(ctx, analysis)).To(Succeed())

		By("6. Waiting for RO to create WorkflowExecution CRD")
		var we *workflowexecutionv1.WorkflowExecution
		Eventually(func() bool {
			weList := &workflowexecutionv1.WorkflowExecutionList{}
			_ = k8sClient.List(ctx, weList, client.InNamespace(controllerNamespace))
			for i := range weList.Items {
				if len(weList.Items[i].OwnerReferences) > 0 &&
					weList.Items[i].OwnerReferences[0].Kind == "RemediationRequest" &&
					weList.Items[i].OwnerReferences[0].Name == rrName {
					we = &weList.Items[i]
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "WorkflowExecution should be created by RO")

		By("7. Manually updating WorkflowExecution status to Completed")
		we.Status.Phase = workflowexecutionv1.PhaseCompleted
		Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

		By("8. Waiting for RemediationRequest to transition to Completed")
		updatedRR := &remediationv1.RemediationRequest{}
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)
			return updatedRR.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted),
			"RemediationRequest should transition to Completed after WE completes")

		// Re-fetch RR for latest status (RO may update multiple times)
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())

		// E2E-RO-163-001: Phase timestamps validation
		Expect(updatedRR.Status.ProcessingStartTime).NotTo(BeNil())
		Expect(updatedRR.Status.AnalyzingStartTime).NotTo(BeNil())
		Expect(updatedRR.Status.ExecutingStartTime).NotTo(BeNil())
		Expect(updatedRR.Status.AnalyzingStartTime.Time).To(BeTemporally(">=", updatedRR.Status.ProcessingStartTime.Time))
		Expect(updatedRR.Status.ExecutingStartTime.Time).To(BeTemporally(">=", updatedRR.Status.AnalyzingStartTime.Time))

		// E2E-RO-163-006: Outcome validation
		Expect(updatedRR.Status.Outcome).To(Equal("Remediated"))

		// E2E-RO-163-007: NotificationDelivered condition
		// RO creates NR on completion; simulate delivery by updating NR to Sent
		By("8b. Waiting for NR child and simulating successful delivery")
		var nr *notificationv1.NotificationRequest
		Eventually(func() bool {
			nrList := &notificationv1.NotificationRequestList{}
			_ = k8sClient.List(ctx, nrList, client.InNamespace(controllerNamespace))
			for i := range nrList.Items {
				if nrList.Items[i].Spec.RemediationRequestRef != nil &&
					nrList.Items[i].Spec.RemediationRequestRef.Name == rrName {
					nr = &nrList.Items[i]
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "NotificationRequest should be created by RO after completion")

		// Simulate successful delivery: NR phase Sent triggers NotificationDelivered=True
		nr.Status.Phase = notificationv1.NotificationPhaseSent
		Expect(k8sClient.Status().Update(ctx, nr)).To(Succeed())

		// RO Owns NR; status update triggers reconcile; notification handler sets NotificationDelivered
		Eventually(func() bool {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)
			cond := meta.FindStatusCondition(updatedRR.Status.Conditions, "NotificationDelivered")
			return cond != nil && cond.Status == metav1.ConditionTrue
		}, timeout, interval).Should(BeTrue(), "NotificationDelivered condition should be True after NR Sent")

		// Re-fetch for final assertions
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())

		// E2E-RO-163-007: All RR conditions (table-driven)
		expectedConditions := []string{
			"SignalProcessingReady", "SignalProcessingComplete",
			"AIAnalysisReady", "AIAnalysisComplete",
			"WorkflowExecutionReady", "WorkflowExecutionComplete",
			"Ready",
			"NotificationDelivered",
		}
		for _, condType := range expectedConditions {
			Expect(updatedRR.Status.Conditions).To(ContainElement(
				And(HaveField("Type", condType), HaveField("Status", metav1.ConditionTrue)),
			), "Condition %s should be True", condType)
		}

		By("9. Waiting for RO to create EffectivenessAssessment CRD (ADR-EM-001)")
		eaName := fmt.Sprintf("ea-%s", rrName)
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: eaName, Namespace: controllerNamespace}, ea)
		}, timeout, interval).Should(Succeed(), "EffectivenessAssessment should be created after RR completion")

		By("10. Validating EA spec fields")
		Expect(ea.Spec.CorrelationID).To(Equal(rrName),
			"EA correlation ID should match RR name")
		Expect(ea.Spec.RemediationTarget.Kind).To(Equal("Deployment"))
		Expect(ea.Spec.RemediationTarget.Name).To(Equal("test-app-ea"))
		Expect(ea.Spec.RemediationTarget.Namespace).To(Equal(testNS))
		Expect(ea.Spec.Config.StabilizationWindow.Duration).To(BeNumerically(">", 0),
			"Stabilization window should be set from RO config")

		By("11. Validating EA spec fields")
		Expect(ea.Spec.RemediationRequestPhase).To(Equal("Completed"))

		By("12. Validating owner reference for cascade deletion (BR-ORCH-031)")
		Expect(ea.OwnerReferences).To(HaveLen(1))
		Expect(ea.OwnerReferences[0].Name).To(Equal(rrName))
		Expect(ea.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))

		GinkgoWriter.Printf("E2E-RO-EA-001: EffectivenessAssessment '%s' validated in Kind cluster\n", eaName)
	})

	// E2E-RO-163-005: Failure post-mortem scenario
	// Tests that RR transitions to Failed with correct FailurePhase, FailureReason, ConsecutiveFailureCount
	// when WorkflowExecution fails during execution.
	Context("E2E-RO-163-005: Failure post-mortem", func() {
		It("should set FailurePhase, FailureReason, ConsecutiveFailureCount when WE fails", func() {
			By("1. Creating RemediationRequest")
			rrName := "rr-fail-e2e-" + uuid.New().String()[:8]
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rrName,
					Namespace: controllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: func() string {
						h := sha256.Sum256([]byte(uuid.New().String()))
						return hex.EncodeToString(h[:])
					}(),
					SignalName: "HighCPU",
					Severity:   "critical",
					SignalType: "alert",
					TargetType: "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-app-fail",
						Namespace: testNS,
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
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

			By("2. Waiting for RO to create SignalProcessing CRD")
			var sp *signalprocessingv1.SignalProcessing
			Eventually(func() bool {
				spList := &signalprocessingv1.SignalProcessingList{}
				_ = k8sClient.List(ctx, spList, client.InNamespace(controllerNamespace))
				for i := range spList.Items {
					if len(spList.Items[i].OwnerReferences) > 0 &&
						spList.Items[i].OwnerReferences[0].Kind == "RemediationRequest" &&
						spList.Items[i].OwnerReferences[0].Name == rrName {
						sp = &spList.Items[i]
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "SignalProcessing should be created by RO")

			By("3. Manually updating SP status to Completed")
			sp.Status.Phase = signalprocessingv1.PhaseCompleted
			sp.Status.Severity = "critical"
			sp.Status.SignalMode = "reactive"
			sp.Status.SignalName = sp.Spec.Signal.Name // Issue #166: Use signal name, not type ("alert")
			sp.Status.EnvironmentClassification = &signalprocessingv1.EnvironmentClassification{
				Environment:  "production",
				Source:       "namespace-labels",
				ClassifiedAt: metav1.Now(),
			}
			sp.Status.PriorityAssignment = &signalprocessingv1.PriorityAssignment{
				Priority:   "P1",
				Source:     "rego-policy",
				AssignedAt: metav1.Now(),
			}
			Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

			By("4. Waiting for RO to create AIAnalysis CRD")
			var analysis *aianalysisv1.AIAnalysis
			Eventually(func() bool {
				analysisList := &aianalysisv1.AIAnalysisList{}
				_ = k8sClient.List(ctx, analysisList, client.InNamespace(controllerNamespace))
				for i := range analysisList.Items {
					if len(analysisList.Items[i].OwnerReferences) > 0 &&
						analysisList.Items[i].OwnerReferences[0].Kind == "RemediationRequest" &&
						analysisList.Items[i].OwnerReferences[0].Name == rrName {
						analysis = &analysisList.Items[i]
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "AIAnalysis should be created by RO")

			By("5. Manually updating AIAnalysis status to Completed with SelectedWorkflow")
			analysis.Status.Phase = aianalysisv1.PhaseCompleted
			analysis.Status.Reason = "AnalysisCompleted"
			analysis.Status.Message = "Workflow recommended: restart-deployment-v1"
			analysis.Status.RootCause = "CPU throttling due to resource limits"
			analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:      "restart-deployment-v1",
				Version:        "1.0.0",
				ExecutionBundle: "quay.io/kubernaut/restart-deployment:v1",
				Confidence:      0.92,
				Rationale:       "High confidence match for CPU remediation",
			}
			// DD-HAPI-006: AffectedResource is required for routing to WorkflowExecution
			analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary:    "CPU throttling due to resource limits",
				Severity:   "critical",
				SignalType: "alert",
				AffectedResource: &aianalysisv1.AffectedResource{
					Kind:      "Deployment",
					Name:      "test-app-fail",
					Namespace: testNS,
				},
			}
			Expect(k8sClient.Status().Update(ctx, analysis)).To(Succeed())

			By("6. Waiting for RO to create WorkflowExecution CRD")
			var we *workflowexecutionv1.WorkflowExecution
			Eventually(func() bool {
				weList := &workflowexecutionv1.WorkflowExecutionList{}
				_ = k8sClient.List(ctx, weList, client.InNamespace(controllerNamespace))
				for i := range weList.Items {
					if len(weList.Items[i].OwnerReferences) > 0 &&
						weList.Items[i].OwnerReferences[0].Kind == "RemediationRequest" &&
						weList.Items[i].OwnerReferences[0].Name == rrName {
						we = &weList.Items[i]
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "WorkflowExecution should be created by RO")

			By("7. Manually updating WorkflowExecution status to Failed with FailureDetails")
			we.Status.Phase = workflowexecutionv1.PhaseFailed
			we.Status.FailureReason = "Tekton PipelineRun failed"
			we.Status.FailureDetails = &workflowexecutionv1.FailureDetails{
				FailedTaskIndex:              0,
				FailedTaskName:               "restart-deployment",
				Reason:                       workflowexecutionv1.FailureReasonTaskFailed,
				Message:                      "Container step failed with exit code 1",
				FailedAt:                     metav1.Now(),
				ExecutionTimeBeforeFailure:   "30s",
				NaturalLanguageSummary:      "Workflow failed during deployment restart step",
				WasExecutionFailure:         true,
			}
			Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

			By("8. Waiting for RemediationRequest to transition to Failed")
			updatedRR := &remediationv1.RemediationRequest{}
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)
				return updatedRR.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseFailed),
				"RemediationRequest should transition to Failed after WE fails")

			// Re-fetch RR for latest status
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())

			// E2E-RO-163-005: Failure post-mortem assertions
			Expect(updatedRR.Status.FailurePhase).NotTo(BeNil())
			Expect(*updatedRR.Status.FailurePhase).To(Equal("workflow_execution"),
				"FailurePhase should be workflow_execution (lowercase snake_case)")
			Expect(updatedRR.Status.FailureReason).NotTo(BeNil())
			Expect(*updatedRR.Status.FailureReason).NotTo(BeEmpty(),
				"FailureReason should contain the WE failure reason")
			Expect(updatedRR.Status.ConsecutiveFailureCount).To(Equal(int32(1)),
				"ConsecutiveFailureCount should be 1 for first failure")
			Expect(updatedRR.Status.NextAllowedExecution).NotTo(BeNil(),
				"NextAllowedExecution should be set via exponential backoff on first failure")

			GinkgoWriter.Printf("E2E-RO-163-005: Failure post-mortem validated — FailurePhase=%s, ConsecutiveFailureCount=%d, NextAllowedExecution=%s\n",
				*updatedRR.Status.FailurePhase, updatedRR.Status.ConsecutiveFailureCount, updatedRR.Status.NextAllowedExecution.Format(time.RFC3339))
		})
	})
})
