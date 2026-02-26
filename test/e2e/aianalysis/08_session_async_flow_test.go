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

package aianalysis

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Session-Based Async Flow E2E Tests
// Test Plan: docs/testing/BR-AA-HAPI-064/session_based_pull_test_plan_v1.0.md
// Scenario: E2E-AA-064-001
// Business Requirements: BR-AA-HAPI-064.1 through .8
//
// Purpose: Validate that the AA controller completes a full async investigation
// lifecycle (submit -> poll -> result) in a real K8s environment with deployed HAPI.

var _ = Describe("E2E-AA-064: Session-Based Async Flow", Label("e2e", "session", "aa-064"), func() {
	const (
		timeout  = 60 * time.Second       // Allow for session submit + poll + result cycle
		interval = 500 * time.Millisecond // Poll twice per second
	)

	Context("E2E-AA-064-001: AA async submit/poll/result flow (incident)", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-session-async-" + randomSuffix(),
					Namespace: controllerNamespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-remediation-session",
						Namespace: controllerNamespace,
					},
					RemediationID: "e2e-rem-session-001",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-fingerprint-session-001",
							Severity:         "medium",
							SignalName:       "CrashLoopBackOff",
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "session-test-pod",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
					},
				},
			}
		})

		It("should complete full async investigation via session-based pull design", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-AA-064-001
			// Business Outcome: AA controller completes an async investigation in a real K8s environment
			// BR: BR-AA-HAPI-064.1 through .8
			// Flow: AA submits to HAPI (202) -> polls session -> fetches result -> completes analysis

			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			By("Creating AIAnalysis CRD")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for Completed phase (async session flow: submit -> poll -> result -> analysis)")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"))

			By("Verifying InvestigationSession was populated during async flow")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// BR-AA-HAPI-064.4: InvestigationSession tracking
			session := analysis.Status.InvestigationSession
			Expect(session).NotTo(BeNil(), "InvestigationSession must be populated in CRD status")
			// Session ID should be set (from HAPI's 202 response)
			// Note: After result retrieval, the session may be cleared or retained depending on implementation.
			// We verify Generation = 0 (no regeneration needed in happy path)
			Expect(session.Generation).To(Equal(int32(0)),
				"Generation must be 0 (no session loss in happy path)")

			By("Verifying analysis results are complete")
			// Should have workflow selected (same as sync flow)
			Expect(analysis.Status.SelectedWorkflow).NotTo(BeNil(),
				"SelectedWorkflow must be populated after async investigation")

			// Should have completion timestamp
			Expect(analysis.Status.CompletedAt).NotTo(BeZero(),
				"CompletedAt must be set when analysis completes")

			// Staging environment = auto-approve
			Expect(analysis.Status.ApprovalRequired).To(BeFalse(),
				"Staging environment should auto-approve")
		})
	})
})
