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
)

// E2E-AA-607-001: Not-Actionable Alert (Outcome D) — Confidence Gate Regression
//
// Issue: #607
// Business Requirement: BR-AI-001 (Complete reconciliation lifecycle)
//
// When the LLM determines an alert is not actionable (e.g., orphaned PVCs from
// completed batch jobs), the AA controller must complete as WorkflowNotNeeded/
// NotActionable regardless of confidence value. Before #607, a confidence gate
// (>= 0.7) blocked this path when the LLM omitted or returned low confidence.
//
// This E2E test validates the full pipeline:
//   AA → KA → Mock LLM (actionable=false, confidence=0.0) → KA (floor to 0.8) → AA (Completed)
//
// Regression gate for the 433 team migrating to KA.

var _ = Describe("E2E-AA-607: Not-Actionable Confidence Gate", Label("e2e", "not-actionable", "aianalysis"), func() {
	const (
		timeout  = 30 * time.Second
		interval = 500 * time.Millisecond
	)

	Context("Orphaned PVC alert — not actionable (#607)", func() {
		var analysis *aianalysisv1.AIAnalysis

		BeforeEach(func() {
			_ = createTestNamespace("e2e-607-not-actionable")
			analysis = &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-not-actionable-607-" + randomSuffix(),
					Namespace: controllerNamespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-remediation-607",
						Namespace: controllerNamespace,
					},
					RemediationID: "e2e-rem-607-not-actionable",
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint:      "e2e-fingerprint-607",
							Severity:         "low",
							SignalName:       "MOCK_NOT_ACTIONABLE",
							Environment:      "staging",
							BusinessPriority: "P3",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "PersistentVolumeClaim",
								Name:      "batch-job-pvc-expired",
								Namespace: "production",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []aianalysisv1.AnalysisType{
							aianalysisv1.AnalysisTypeInvestigation,
							aianalysisv1.AnalysisTypeRootCause,
							aianalysisv1.AnalysisTypeWorkflowSelection,
						},
					},
				},
			}
		})

		It("should complete as WorkflowNotNeeded/NotActionable - E2E-AA-607-001", func() {
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Creating AIAnalysis with MOCK_NOT_ACTIONABLE signal")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for AA to complete (not fail) — #607 fix removes confidence gate on actionable=false")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"),
				"#607: actionable=false must route to Completed, not Failed")

			By("Verifying Outcome D status fields")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			Expect(analysis.Status.Reason).To(Equal(aianalysisv1.ReasonWorkflowNotNeeded),
				"#607: Reason must be WorkflowNotNeeded for not-actionable alerts")
			Expect(analysis.Status.SubReason).To(Equal("NotActionable"),
				"#607: SubReason must be NotActionable (distinct from ProblemResolved)")
			Expect(analysis.Status.NeedsHumanReview).To(BeFalse(),
				"#607: Benign alerts should not require human review")
			Expect(analysis.Status.Actionability).To(Equal(aianalysis.ActionabilityNotActionable),
				"#607: Actionability must be NotActionable for benign conditions")

			By("Verifying RCA is preserved for audit trail")
			Expect(analysis.Status.RootCauseAnalysis).NotTo(BeNil(),
				"RCA should be populated even for not-actionable alerts")
			Expect(analysis.Status.RootCauseAnalysis.Summary).NotTo(BeEmpty())

			By("Verifying no workflow was selected")
			Expect(analysis.Status.SelectedWorkflow).To(BeNil(),
				"No workflow should be selected for not-actionable alerts")

			By("Verifying completion metadata")
			Expect(analysis.Status.CompletedAt).NotTo(BeZero())
			// Uses >= 0 because mock LLM responds instantly; sub-second analyses truncate to 0
			Expect(analysis.Status.TotalAnalysisTime).To(BeNumerically(">=", 0))
		})
	})
})
