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

// Issue #2019 / #2020 / FedRAMP AU-2, AU-3, AC-6, CM-3: Proves decision_expired
// can be persisted to the CRD status via the Kubernetes API server (envtest
// with real CRD validation), and that the discovered-but-unconfirmed
// SelectedWorkflow is preserved alongside it -- without ever setting
// ApprovalRequired/auto-executing on the human's behalf.
package aianalysis

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// decisionExpiredWorkflowSnapshot builds a minimal but schema-valid
// WorkflowSnapshot (WorkflowName/ActionType/Version/ExecutionBundle are all
// +kubebuilder:validation:Required per DD-WORKFLOW-018) for the
// discovered-but-unconfirmed workflow preserved by the #2019/#2020 fix.
func decisionExpiredWorkflowSnapshot() sharedtypes.WorkflowSnapshot {
	return sharedtypes.WorkflowSnapshot{
		WorkflowID:      "wf-recommended-2020",
		WorkflowName:    "restart-crashlooping-pod",
		ActionType:      "RestartPod",
		Version:         "v1.0.0",
		ExecutionBundle: "ghcr.io/kubernaut/restart-pod:v1.0",
	}
}

var _ = Describe("Decision Expired Status Write (#2019/#2020)", Label("integration", "decision-expired"), func() {
	const (
		timeout  = 30 * time.Second
		interval = 500 * time.Millisecond
	)

	Context("IT-AA-2020-001: decision_expired CRD status write preserves the discovered workflow (AU-2, AU-3, AC-6, CM-3)", func() {
		var analysis *aianalysisv1.AIAnalysis

		BeforeEach(func() {
			rrName := helpers.UniqueTestName("test-decision-expired-rr")
			analysis = &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      helpers.UniqueTestName("decision-expired-test"),
					Namespace: testNamespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      rrName,
						Namespace: testNamespace,
					},
					RemediationID: rrName,
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint:      "test-fingerprint-decision-expired",
							Severity:         "critical",
							SignalName:       "KubePodCrashLooping",
							Environment:      "production",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Deployment",
								Name:      "worker",
								Namespace: testNamespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
					},
				},
			}
		})

		It("IT-AA-2020-001: persists humanReviewReason=decision_expired and the discovered SelectedWorkflow, without approval", func() {
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Creating AIAnalysis CRD")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Writing status with humanReviewReason=decision_expired and a preserved SelectedWorkflow")
			Eventually(func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
					return err
				}
				now := metav1.Now()
				analysis.Status.Phase = aianalysis.PhaseFailed
				analysis.Status.NeedsHumanReview = true
				analysis.Status.HumanReviewReason = katypes.HumanReviewReasonDecisionExpired
				analysis.Status.Reason = aianalysisv1.ReasonWorkflowResolutionFailed
				analysis.Status.SubReason = "DecisionExpired"
				analysis.Status.CompletedAt = &now
				// #2019/#2020: the discovered-but-unconfirmed workflow must be
				// preserved for audit/retroactive human action -- but
				// ApprovalRequired stays unset/false, since the human never
				// actually confirmed it (AC-6/CM-3: no silent auto-approval of
				// an action nobody signed off on).
				analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
					WorkflowSnapshot: decisionExpiredWorkflowSnapshot(),
					Confidence:       0.9,
					Rationale:        "restart the crashlooping pod",
				}
				return k8sClient.Status().Update(ctx, analysis)
			}, timeout, interval).Should(Succeed(),
				"AU-2/AU-3: API server must accept decision_expired in CRD status (proves the CRD/OpenAPI enum widening)")

			By("Verifying the field persisted after re-read")
			var persisted aianalysisv1.AIAnalysis
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &persisted)).To(Succeed())

			Expect(persisted.Status.HumanReviewReason).To(Equal(katypes.HumanReviewReasonDecisionExpired),
				"AU-2/AU-3: decision_expired must be persisted -- this is the #2019/#2020 audit-of-record fix")
			Expect(persisted.Status.NeedsHumanReview).To(BeTrue(),
				"AC-6/CM-3: NeedsHumanReview must stay true -- a presented-but-unanswered decision is never auto-approved")
			Expect(persisted.Status.Phase).To(Equal("Failed"),
				"AU-2/AU-3: Phase=Failed must be persisted for audit trail")
			Expect(persisted.Status.SubReason).To(Equal("DecisionExpired"),
				"AU-2/AU-3: SubReason must be persisted for structured audit reporting/metrics")
			Expect(persisted.Status.SelectedWorkflow).NotTo(BeNil(),
				"AU-2/AU-3: the discovered workflow recommendation must survive -- this is the actual bug #2019/#2020 fixes "+
					"(previously silently discarded as has_workflow:false)")
			Expect(persisted.Status.SelectedWorkflow.WorkflowID).To(Equal("wf-recommended-2020"))
			Expect(persisted.Status.ApprovalRequired).To(BeFalse(),
				"AC-6/CM-3: a decision_expired outcome must never be marked as approved -- no execution without an explicit human decision")
		})
	})
})
