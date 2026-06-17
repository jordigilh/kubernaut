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

// Issue #1449 / FedRAMP IR-5, AU-12: Proves operator_escalation can be persisted
// to the CRD status via the Kubernetes API server (envtest with real CRD validation).
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
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("Escalation Status Write (#1449)", Label("integration", "escalation"), func() {
	const (
		timeout  = 30 * time.Second
		interval = 500 * time.Millisecond
	)

	Context("IT-AA-1449-001: operator_escalation CRD status write (IR-5, AU-12)", func() {
		var analysis *aianalysisv1.AIAnalysis

		BeforeEach(func() {
			rrName := helpers.UniqueTestName("test-escalation-rr")
			analysis = &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      helpers.UniqueTestName("escalation-test"),
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
							Fingerprint:      "test-fingerprint-escalation",
							Severity:         "critical",
							SignalName:       "OperatorEscalation",
							Environment:      "production",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: testNamespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
					},
				},
			}
		})

		It("IT-AA-1449-001: persists humanReviewReason=operator_escalation to CRD status via API server", func() {
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Creating AIAnalysis CRD")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Writing status with humanReviewReason=operator_escalation")
			Eventually(func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
					return err
				}
				now := metav1.Now()
				analysis.Status.Phase = aianalysis.PhaseFailed
				analysis.Status.NeedsHumanReview = true
				analysis.Status.HumanReviewReason = "operator_escalation"
				analysis.Status.Reason = aianalysisv1.ReasonWorkflowResolutionFailed
				analysis.Status.SubReason = "OperatorEscalation"
				analysis.Status.CompletedAt = &now
				return k8sClient.Status().Update(ctx, analysis)
			}, timeout, interval).Should(Succeed(),
				"IR-5/AU-12: API server must accept operator_escalation in CRD status (proves enum fix)")

			By("Verifying the field persisted after re-read")
			var persisted aianalysisv1.AIAnalysis
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &persisted)).To(Succeed())

			Expect(persisted.Status.HumanReviewReason).To(Equal("operator_escalation"),
				"IR-5: operator_escalation must be persisted — this was the root cause of #1449")
			Expect(persisted.Status.NeedsHumanReview).To(BeTrue(),
				"IR-5: NeedsHumanReview must be persisted for escalation routing")
			Expect(string(persisted.Status.Phase)).To(Equal("Failed"),
				"AU-12: Phase=Failed must be persisted for audit trail")
			Expect(persisted.Status.SubReason).To(Equal("OperatorEscalation"),
				"AU-12: SubReason must be persisted for structured audit reporting")
		})
	})
})
