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

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
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
				Namespace: testNS,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: func() string {
					h := sha256.Sum256([]byte(uuid.New().String()))
					return hex.EncodeToString(h[:])
				}(),
				SignalName: "HighCPU",
				Severity:   "critical",
				SignalType: "prometheus",
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

		By("2. Waiting for RO to create SignalProcessing CRD")
		var sp *signalprocessingv1.SignalProcessing
		Eventually(func() bool {
			spList := &signalprocessingv1.SignalProcessingList{}
			_ = k8sClient.List(ctx, spList, client.InNamespace(testNS))
			if len(spList.Items) == 0 {
				return false
			}
			sp = &spList.Items[0]
			return true
		}, timeout, interval).Should(BeTrue(), "SignalProcessing should be created by RO")

		By("3. Manually updating SP status to Completed")
		sp.Status.Phase = signalprocessingv1.PhaseCompleted
		sp.Status.Severity = "critical"
		sp.Status.SignalMode = "reactive"
		sp.Status.SignalType = "prometheus"
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
			_ = k8sClient.List(ctx, analysisList, client.InNamespace(testNS))
			if len(analysisList.Items) == 0 {
				return false
			}
			analysis = &analysisList.Items[0]
			return true
		}, timeout, interval).Should(BeTrue(), "AIAnalysis should be created by RO")

		By("5. Manually updating AIAnalysis status to Completed with SelectedWorkflow")
		analysis.Status.Phase = aianalysisv1.PhaseCompleted
		analysis.Status.Reason = "AnalysisCompleted"
		analysis.Status.Message = "Workflow recommended: restart-deployment-v1"
		analysis.Status.RootCause = "CPU throttling due to resource limits"
		analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:     "restart-deployment-v1",
			Version:        "1.0.0",
			ContainerImage: "quay.io/kubernaut/restart-deployment:v1",
			Confidence:     0.92,
			Rationale:      "High confidence match for CPU remediation",
		}
		Expect(k8sClient.Status().Update(ctx, analysis)).To(Succeed())

		By("6. Waiting for RO to create WorkflowExecution CRD")
		var we *workflowexecutionv1.WorkflowExecution
		Eventually(func() bool {
			weList := &workflowexecutionv1.WorkflowExecutionList{}
			_ = k8sClient.List(ctx, weList, client.InNamespace(testNS))
			if len(weList.Items) == 0 {
				return false
			}
			we = &weList.Items[0]
			return true
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

		By("9. Waiting for RO to create EffectivenessAssessment CRD (ADR-EM-001)")
		eaName := fmt.Sprintf("ea-%s", rrName)
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: eaName, Namespace: testNS}, ea)
		}, timeout, interval).Should(Succeed(), "EffectivenessAssessment should be created after RR completion")

		By("10. Validating EA spec fields")
		Expect(ea.Spec.CorrelationID).To(Equal(rrName),
			"EA correlation ID should match RR name")
		Expect(ea.Spec.TargetResource.Kind).To(Equal("Deployment"))
		Expect(ea.Spec.TargetResource.Name).To(Equal("test-app-ea"))
		Expect(ea.Spec.TargetResource.Namespace).To(Equal(testNS))
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
})
