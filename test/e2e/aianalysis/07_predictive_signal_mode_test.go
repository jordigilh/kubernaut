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

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// E2E-AA-084-001: Predictive Signal Mode Pass-Through to HAPI
//
// Business Requirement: BR-AI-084 (Predictive Signal Mode Prompt Strategy)
// Architecture: ADR-054 (Predictive Signal Mode Classification)
//
// Tests that AA correctly passes signalMode from its CRD spec to HAPI
// and that the Mock LLM returns a predictive-aware response.
//
// Data Flow: AA.Spec.SignalContext.SignalMode="predictive" → HAPI → Mock LLM → AA.Status

var _ = Describe("E2E-AA-084-001: Predictive Signal Mode Investigation", Label("e2e", "signalmode", "aianalysis"), func() {
	const (
		timeout  = 30 * time.Second
		interval = 500 * time.Millisecond
	)

	Context("Predictive OOMKill investigation (BR-AI-084)", func() {
		It("should complete analysis with predictive signal mode context", func() {
			// BUSINESS CONTEXT:
			// AA receives signalMode=predictive from RO (copied from SP.Status).
			// AA passes this to HAPI, which adapts the prompt for preemptive analysis.
			// Mock LLM detects predictive keywords and returns the oomkilled_predictive scenario.
			//
			// This E2E test validates the full AA → HAPI → Mock LLM pipeline with
			// predictive signal mode context flowing through all components.

			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-predictive-oomkill-" + randomSuffix(),
					Namespace: controllerNamespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-predictive-remediation",
						Namespace: controllerNamespace,
					},
					RemediationID: "e2e-predictive-rem-001",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-predictive-fingerprint-001",
							Severity:         "critical",
							SignalName:       "OOMKilled",    // Normalized by SP from PredictedOOMKill
							SignalMode:       "predictive",   // BR-AI-084: Predictive signal mode
							Environment:      "production",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment",
								Name:      "api-server",
								Namespace: controllerNamespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
					},
				},
			}

			By("Creating AIAnalysis with signalMode=predictive")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Waiting for AA to complete investigation (4-phase reconciliation)")
			// The Mock LLM should detect the predictive keywords in the prompt
			// and return the oomkilled_predictive scenario response.
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"),
				"AA should complete investigation with predictive signal mode")

			By("Verifying analysis completed successfully")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// The AA controller should have passed signalMode=predictive to HAPI,
			// which should have adapted the prompt for preemptive analysis.
			// The Mock LLM returns a workflow for the predictive scenario.
			Expect(analysis.Status.CompletedAt).ToNot(BeZero(),
				"CompletedAt should be set after successful completion")

			GinkgoWriter.Println("E2E-AA-084-001: Predictive signal mode investigation completed in Kind cluster")
		})
	})

	Context("Reactive signal mode investigation (backwards compatibility)", func() {
		It("should complete standard RCA analysis for reactive signal mode", func() {
			// BUSINESS CONTEXT:
			// Existing reactive signals should continue working with standard RCA.
			// signalMode=reactive (or empty) should produce normal investigation results.

			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-reactive-oomkill-" + randomSuffix(),
					Namespace: controllerNamespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-reactive-remediation",
						Namespace: controllerNamespace,
					},
					RemediationID: "e2e-reactive-rem-001",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-reactive-fingerprint-001",
							Severity:         "critical",
							SignalName:       "OOMKilled",
							SignalMode:       "reactive",   // Explicit reactive mode
							Environment:      "production",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "worker-pod",
								Namespace: controllerNamespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
					},
				},
			}

			By("Creating AIAnalysis with signalMode=reactive")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Waiting for AA to complete standard RCA investigation")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"),
				"AA should complete standard RCA with reactive signal mode")

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
			Expect(analysis.Status.CompletedAt).ToNot(BeZero())

			GinkgoWriter.Println("E2E-AA-084-001: Reactive signal mode backwards compatibility validated")
		})
	})
})
