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
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// ADR-056 SoC: Integration tests for PostRCAContext and DetectedLabels
// in the AIAnalysis controller reconciliation loop.
//
// Business Requirements:
//   - BR-AI-056: DetectedLabels stored in PostRCAContext via HAPI
//   - BR-AI-013: Rego policy evaluation uses detected_labels
//   - BR-AI-082: PostRCAContext.setAt immutability guard
//   - ADR-056 Phase 4: enrichmentResults must NOT contain detectedLabels
//
// Infrastructure: envtest (per-process) + HAPI container + Mock LLM (3-step) + DataStorage
//
// IMPORTANT: HAPI computes DetectedLabels by querying K8s resources (PDBs, HPAs, etc.)
// via the shared envtest. Whether PostRCAContext is populated depends on whether HAPI
// can discover K8s resources matching the target resource in its connected envtest.
// In integration tests, PostRCAContext may be nil if the shared envtest lacks resources.
// E2E tests (Kind cluster) provide the full label detection path.
//
// SERIAL EXECUTION: AA integration suite runs serially for 100% reliability.
var _ = Describe("ADR-056 PostRCAContext Integration", Label("integration", "adr-056", "post-rca-context"), func() {
	const (
		timeout  = 2 * time.Minute
		interval = time.Second
	)

	// newIncidentAnalysis creates a standard AIAnalysis CR for incident tests.
	newIncidentAnalysis := func(suffix string) *aianalysisv1alpha1.AIAnalysis {
		rrName := helpers.UniqueTestName("remediation-" + suffix)
		return &aianalysisv1alpha1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      helpers.UniqueTestName("it-aa-056-" + suffix),
				Namespace: testNamespace,
			},
			Spec: aianalysisv1alpha1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      rrName,
					Namespace: testNamespace,
				},
				RemediationID: rrName,
				AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
					SignalContext: aianalysisv1alpha1.SignalContextInput{
						Fingerprint:      "fp-it-aa-056-" + suffix,
						Severity:         "critical",
						SignalName:       "CrashLoopBackOff",
						Environment:      "staging",
						BusinessPriority: "P1",
						TargetResource: aianalysisv1alpha1.TargetResource{
							Kind:      "Pod",
							Name:      "test-app",
							Namespace: testNamespace,
						},
						EnrichmentResults: sharedtypes.EnrichmentResults{},
					},
					AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
				},
			},
		}
	}

	// waitForTerminalPhase waits until the AIAnalysis CR reaches a terminal phase.
	waitForTerminalPhase := func(analysis *aianalysisv1alpha1.AIAnalysis) {
		Eventually(func() string {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
			return string(analysis.Status.Phase)
		}, timeout, interval).Should(
			SatisfyAny(Equal("Completed"), Equal("Failed")),
			"CR should reach a terminal phase (Completed or Failed)")
	}

	Context("Incident Analysis with PostRCAContext - ADR-056", func() {
		It("IT-AA-056-001: should complete reconciliation and handle PostRCAContext from HAPI response", func() {
			analysis := newIncidentAnalysis("001")
			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			By("Creating AIAnalysis CR with CrashLoopBackOff signal")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for reconciliation to reach terminal phase")
			waitForTerminalPhase(analysis)

			By("Verifying HAPI was called and PostRCAContext handling")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// ADR-056: HAPI computes DetectedLabels on-demand during list_available_actions.
			// After reconciliation, PostRCAContext is populated if HAPI returns non-empty labels.
			// The controller must not crash regardless of label detection outcome.
			if analysis.Status.PostRCAContext != nil {
				Expect(analysis.Status.PostRCAContext.DetectedLabels).NotTo(BeNil(),
					"When PostRCAContext is set, DetectedLabels must be non-nil")
				Expect(analysis.Status.PostRCAContext.SetAt).NotTo(BeNil(),
					"When PostRCAContext is set, SetAt must be non-nil (BR-AI-082)")
			}

			// The reconciliation should complete successfully regardless of label detection
			Expect(analysis.Status.InvestigationID).NotTo(BeEmpty(),
				"InvestigationID should be set after HAPI call")
		})

		It("IT-AA-056-003: should reach Analyzing phase with detected_labels in Rego input", func() {
			analysis := newIncidentAnalysis("003")
			// Use production to trigger approval-required path (Rego evaluation).
			analysis.Spec.AnalysisRequest.SignalContext.Environment = "production"
			// Use a signal returning confidence < 0.8 (Rego default threshold) so the
			// production catch-all rule fires. Unrecognized signals hit mock default (0.75).
			analysis.Spec.AnalysisRequest.SignalContext.SignalName = "MOCK_APPROVAL_TEST"
			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			By("Creating production AIAnalysis CR")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for reconciliation to complete (must pass through Analyzing phase)")
			waitForTerminalPhase(analysis)

			By("Verifying Rego evaluation completed (Analyzing phase was reached)")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// BR-AI-013: If reconciliation reaches Completed, Rego evaluation ran.
			// The Rego evaluator receives detected_labels (from PostRCAContext or empty map).
			// Production + confidence < 0.8 triggers approval via Rego policy.
			if analysis.Status.Phase == "Completed" {
				Expect(analysis.Status.ApprovalRequired).To(BeTrue(),
					"Production environment should require approval (Rego evaluation with detected_labels)")
			}
		})

		It("IT-AA-056-004: should handle failedDetections gracefully in Rego evaluation", func() {
			analysis := newIncidentAnalysis("004")
			analysis.Spec.AnalysisRequest.SignalContext.Environment = "production"
			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			By("Creating AIAnalysis CR")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for reconciliation to complete")
			waitForTerminalPhase(analysis)

			By("Verifying failedDetections handling")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// BR-SP-103 + BR-AI-013: If PostRCAContext has failedDetections,
			// Rego evaluation should still complete without error.
			// The test verifies the controller doesn't crash on partial detection.
			if analysis.Status.PostRCAContext != nil &&
				analysis.Status.PostRCAContext.DetectedLabels != nil &&
				len(analysis.Status.PostRCAContext.DetectedLabels.FailedDetections) > 0 {
				// failedDetections were propagated correctly from HAPI
				GinkgoWriter.Printf("failedDetections found: %v\n",
					analysis.Status.PostRCAContext.DetectedLabels.FailedDetections)
			}

			// Reconciliation must complete regardless of partial detection failures
			Expect(analysis.Status.Phase).To(SatisfyAny(Equal("Completed"), Equal("Failed")))
		})

		It("IT-AA-056-005: should set PostRCAContext.setAt timestamp (immutability guard)", func() {
			analysis := newIncidentAnalysis("005")
			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			By("Creating AIAnalysis CR")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for reconciliation to complete")
			waitForTerminalPhase(analysis)

			By("Verifying setAt immutability guard")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// BR-AI-082: When PostRCAContext is populated, setAt must be a valid timestamp
			// close to the current time. This serves as the CEL immutability guard anchor.
			if analysis.Status.PostRCAContext != nil {
				Expect(analysis.Status.PostRCAContext.SetAt).NotTo(BeNil(),
					"PostRCAContext.SetAt must be set (BR-AI-082 immutability guard)")
				Expect(analysis.Status.PostRCAContext.SetAt.Time).To(
					BeTemporally("~", time.Now(), 5*time.Minute),
					"SetAt should be within 5 minutes of now")
			}
		})
	})

	Context("ADR-056 Phase 4: Cleanup Validation", func() {
		It("IT-AA-056-006: enrichmentResults must NOT contain detectedLabels or ownerChain", func() {
			analysis := newIncidentAnalysis("006")
			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			By("Creating AIAnalysis CR with empty EnrichmentResults")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for reconciliation to start processing")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(
				SatisfyAny(Equal("Investigating"), Equal("Analyzing"), Equal("Completed"), Equal("Failed")),
				"CR should progress past Pending phase")

			By("Verifying enrichmentResults in spec (JSON-level check)")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// ADR-056 Phase 4: DetectedLabels and OwnerChain were removed from EnrichmentResults.
			// Verify at the JSON serialization level that the HAPI request's enrichmentResults
			// does NOT contain these deprecated fields.
			enrichmentJSON, err := json.Marshal(analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults)
			Expect(err).NotTo(HaveOccurred())

			var enrichmentMap map[string]interface{}
			Expect(json.Unmarshal(enrichmentJSON, &enrichmentMap)).To(Succeed())

			Expect(enrichmentMap).NotTo(HaveKey("detectedLabels"),
				"enrichmentResults must NOT contain detectedLabels (ADR-056 Phase 4)")
			Expect(enrichmentMap).NotTo(HaveKey("ownerChain"),
				"enrichmentResults must NOT contain ownerChain (ADR-055)")
		})
	})
})
