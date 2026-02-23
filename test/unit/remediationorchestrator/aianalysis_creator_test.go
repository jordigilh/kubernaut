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

// Package remediationorchestrator contains unit tests for the Remediation Orchestrator.
package remediationorchestrator

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("AIAnalysisCreator", func() {
	var (
		scheme *runtime.Scheme
		ctx    context.Context
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = signalprocessingv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
		ctx = context.Background()
	})

	Describe("NewAIAnalysisCreator", func() {
		It("should return a non-nil creator to enable BR-ORCH-025 data pass-through", func() {
			// Arrange
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Act
			aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)

			// Assert
			Expect(aiCreator).ToNot(BeNil())
		})
	})

	Describe("Create", func() {
		Context("BR-ORCH-025: Data pass-through from RemediationRequest and SignalProcessing", func() {
			It("should generate deterministic name in format 'ai-{rr.Name}' for reliable tracking", func() {
				// Arrange - use testutil factories
				completedSP := helpers.NewCompletedSignalProcessing("sp-test-remediation", "default")
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
					WithStatusSubresource(completedSP).Build()
				aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default")

				// Act
				name, err := aiCreator.Create(ctx, rr, completedSP)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("ai-test-remediation"))
			})

			It("should be idempotent - return existing name on retry without creating duplicate", func() {
				// Arrange - pre-create the AIAnalysis
				existingAI := &aianalysisv1.AIAnalysis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ai-test-remediation",
						Namespace: "default",
					},
				}
				completedSP := helpers.NewCompletedSignalProcessing("sp-test-remediation", "default")
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).
					WithObjects(existingAI, completedSP).
					WithStatusSubresource(completedSP).Build()
				aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default")

				// Act
				name, err := aiCreator.Create(ctx, rr, completedSP)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("ai-test-remediation"))
			})

			It("should build correct AIAnalysis spec with signal context and enrichment data", func() {
			// Arrange - use testutil factories with custom options
			// NOTE: Environment, Priority, and Severity now come from SP.Status, not RR.Spec
			// (per NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md and DD-SEVERITY-001)
			completedSP := helpers.NewCompletedSignalProcessing("sp-test-remediation", "default")
			// Override SP status to have custom environment/priority/severity for test
			completedSP.Status.EnvironmentClassification.Environment = "production"
			completedSP.Status.PriorityAssignment.Priority = "P0"
			completedSP.Status.Severity = "critical" // DD-SEVERITY-001: Normalized severity from SP
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
				WithStatusSubresource(completedSP).Build()
			aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default", helpers.RemediationRequestOpts{
				Severity:   "sev1", // External severity (not used by AIAnalysis per DD-SEVERITY-001)
				SignalType: "kubernetes-event",
			})

				// Act
				name, err := aiCreator.Create(ctx, rr, completedSP)

				// Assert
				Expect(err).ToNot(HaveOccurred())

				// Fetch created AI and verify spec
				createdAI := &aianalysisv1.AIAnalysis{}
				err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
				Expect(err).ToNot(HaveOccurred())

			// Verify RemediationRequestRef
			Expect(createdAI.Spec.RemediationRequestRef.Name).To(Equal(rr.Name))
			Expect(createdAI.Spec.RemediationRequestRef.Kind).To(Equal("RemediationRequest"))

			// Verify RemediationID uses RR.Name per DD-AUDIT-CORRELATION-001
			// (NOT rr.UID - that was the old inconsistent pattern)
			Expect(createdAI.Spec.RemediationID).To(Equal(rr.Name))

			// Verify SignalContext
			// Fingerprint comes from RR.Spec
			Expect(createdAI.Spec.AnalysisRequest.SignalContext.Fingerprint).To(Equal(rr.Spec.SignalFingerprint))
			// BR-SP-106: SignalType now comes from SP.Status (normalized by signal mode classifier)
			Expect(createdAI.Spec.AnalysisRequest.SignalContext.SignalType).To(Equal(completedSP.Status.SignalType))
			// DD-SEVERITY-001: Severity, Environment, and Priority come from SP.Status (normalized)
			Expect(createdAI.Spec.AnalysisRequest.SignalContext.Severity).To(Equal(completedSP.Status.Severity))
			Expect(createdAI.Spec.AnalysisRequest.SignalContext.Environment).To(Equal(completedSP.Status.EnvironmentClassification.Environment))
			Expect(createdAI.Spec.AnalysisRequest.SignalContext.BusinessPriority).To(Equal(completedSP.Status.PriorityAssignment.Priority))

				// Verify TargetResource
				Expect(createdAI.Spec.AnalysisRequest.SignalContext.TargetResource.Kind).To(Equal(rr.Spec.TargetResource.Kind))
				Expect(createdAI.Spec.AnalysisRequest.SignalContext.TargetResource.Name).To(Equal(rr.Spec.TargetResource.Name))
				Expect(createdAI.Spec.AnalysisRequest.SignalContext.TargetResource.Namespace).To(Equal(rr.Spec.TargetResource.Namespace))

				// Verify AnalysisTypes
				Expect(createdAI.Spec.AnalysisRequest.AnalysisTypes).To(ContainElements("investigation", "root-cause", "workflow-selection"))

				// Verify recovery fields (should be false for initial analysis)
				Expect(createdAI.Spec.IsRecoveryAttempt).To(BeFalse())
				Expect(createdAI.Spec.RecoveryAttemptNumber).To(Equal(0))
			})

			It("should pass through enrichment results from SignalProcessing.Status", func() {
				// Arrange - use testutil factory with KubernetesContext
				completedSP := helpers.NewSignalProcessing("sp-test-remediation", "default", helpers.SignalProcessingOpts{
					Phase: signalprocessingv1.PhaseCompleted,
					KubernetesContext: &signalprocessingv1.KubernetesContext{
						Namespace: &signalprocessingv1.NamespaceContext{
							Name: "default",
							Labels: map[string]string{
								"kubernetes.io/metadata.name": "default",
								"environment":                 "production",
							},
						},
					},
				})
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
					WithStatusSubresource(completedSP).Build()
				aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default")

				// Act
				name, err := aiCreator.Create(ctx, rr, completedSP)

				// Assert
				Expect(err).ToNot(HaveOccurred())

				// Fetch created AI and verify enrichment results
				createdAI := &aianalysisv1.AIAnalysis{}
				err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
				Expect(err).ToNot(HaveOccurred())

				// Verify EnrichmentResults.KubernetesContext is populated
				Expect(createdAI.Spec.AnalysisRequest.SignalContext.EnrichmentResults.KubernetesContext).ToNot(BeNil())
				// Issue #113: sharedtypes.KubernetesContext has Namespace as *NamespaceContext
				Expect(createdAI.Spec.AnalysisRequest.SignalContext.EnrichmentResults.KubernetesContext.Namespace).ToNot(BeNil())
				Expect(createdAI.Spec.AnalysisRequest.SignalContext.EnrichmentResults.KubernetesContext.Namespace.Name).To(Equal("default"))
				Expect(createdAI.Spec.AnalysisRequest.SignalContext.EnrichmentResults.KubernetesContext.Namespace.Labels).To(
					HaveKeyWithValue("environment", "production"))
			})

			It("should use normalized severity from SignalProcessing.Status.Severity (DD-SEVERITY-001)", func() {
				// DD-SEVERITY-001: AIAnalysis uses normalized severity from SignalProcessing Rego policy
				// BR-SP-105: Severity Determination via Rego Policy
				// Arrange - RR has external severity "Sev1", SP has normalized severity "critical"
				completedSP := helpers.NewSignalProcessing("sp-test-remediation", "default", helpers.SignalProcessingOpts{
					Phase: signalprocessingv1.PhaseCompleted,
				})
				// Set normalized severity in SP status (determined by SignalProcessing Rego policy)
				completedSP.Status.Severity = "critical"
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
					WithStatusSubresource(completedSP).Build()
				aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)

				// RR has external (customer-specific) severity "Sev1"
				rr := helpers.NewRemediationRequest("test-remediation", "default", helpers.RemediationRequestOpts{
					Severity: "Sev1", // External severity (customer scheme: Sev1-4)
				})

				// Act
				name, err := aiCreator.Create(ctx, rr, completedSP)

				// Assert
				Expect(err).ToNot(HaveOccurred())

				// Fetch created AIAnalysis and verify it uses normalized severity
				createdAI := &aianalysisv1.AIAnalysis{}
				err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
				Expect(err).ToNot(HaveOccurred())

				// DD-SEVERITY-001: AIAnalysis MUST use normalized severity from SP.Status.Severity
				// (not external severity from RR.Spec.Severity)
				Expect(createdAI.Spec.AnalysisRequest.SignalContext.Severity).To(Equal("critical"),
					"AIAnalysis should use normalized severity from SP.Status.Severity (DD-SEVERITY-001)")

				// Verify RR still has external severity (for notifications/operator messages)
				Expect(rr.Spec.Severity).To(Equal("Sev1"),
					"RemediationRequest should preserve external severity for operator-facing messages")
			})
		})

		Context("BR-ORCH-031: Cascade deletion via owner references", func() {
			It("should set owner reference for automatic cleanup when RemediationRequest is deleted", func() {
				// Arrange - use testutil factories
				completedSP := helpers.NewCompletedSignalProcessing("sp-test-remediation", "default")
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
					WithStatusSubresource(completedSP).Build()
				aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default")

				// Act
				name, err := aiCreator.Create(ctx, rr, completedSP)

				// Assert
				Expect(err).ToNot(HaveOccurred())

				// Fetch the created AIAnalysis
				createdAI := &aianalysisv1.AIAnalysis{}
				err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
				Expect(err).ToNot(HaveOccurred())

				// Verify owner reference
				Expect(createdAI.OwnerReferences).To(HaveLen(1))
				Expect(createdAI.OwnerReferences[0].Name).To(Equal(rr.Name))
				Expect(createdAI.OwnerReferences[0].UID).To(Equal(rr.UID))
			})
		})

		// BR-ORCH-025: Edge cases for data pass-through
		Context("BR-ORCH-025: Edge cases for enrichment data pass-through", func() {
			It("should handle SignalProcessing with nil KubernetesContext gracefully", func() {
				// Arrange - SP completed but without KubernetesContext (edge case)
				completedSP := helpers.NewSignalProcessing("sp-test-remediation", "default", helpers.SignalProcessingOpts{
					Phase:             signalprocessingv1.PhaseCompleted,
					KubernetesContext: nil, // Explicitly nil
				})
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
					WithStatusSubresource(completedSP).Build()
				aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default")

				// Act
				name, err := aiCreator.Create(ctx, rr, completedSP)

				// Assert - should succeed, AIAnalysis created without enrichment
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("ai-test-remediation"))

				// Verify AIAnalysis was created
				createdAI := &aianalysisv1.AIAnalysis{}
				err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
				Expect(err).ToNot(HaveOccurred())
				// EnrichmentResults.KubernetesContext should be nil when SP has no context
				Expect(createdAI.Spec.AnalysisRequest.SignalContext.EnrichmentResults.KubernetesContext).To(BeNil())
			})

			It("should not propagate OwnerChain from SP to AIAnalysis (ADR-055)", func() {
				// ADR-055: OwnerChain is no longer propagated from SP to AIAnalysis.
				// HAPI resolves its own chain post-RCA via get_resource_context tool.
				completedSP := helpers.NewSignalProcessing("sp-test-remediation", "default", helpers.SignalProcessingOpts{
					Phase: signalprocessingv1.PhaseCompleted,
					KubernetesContext: &signalprocessingv1.KubernetesContext{
						Namespace: &signalprocessingv1.NamespaceContext{
							Labels: map[string]string{"env": "test"},
						},
					},
				})
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
					WithStatusSubresource(completedSP).Build()
				aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default")

				// Act
				name, err := aiCreator.Create(ctx, rr, completedSP)

				// Assert
				Expect(err).ToNot(HaveOccurred())

				createdAI := &aianalysisv1.AIAnalysis{}
				err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
				Expect(err).ToNot(HaveOccurred())

				Expect(createdAI.Spec.RemediationRequestRef.Name).To(Equal(rr.Name))
			})

			It("should handle SignalProcessing with partial/incomplete enrichment data", func() {
				// Arrange - SP with only some enrichment fields populated (simulates partial failure)
				completedSP := helpers.NewSignalProcessing("sp-test-remediation", "default", helpers.SignalProcessingOpts{
					Phase:             signalprocessingv1.PhaseCompleted,
					KubernetesContext: &signalprocessingv1.KubernetesContext{
						// Namespace nil - empty enrichment
					},
				})
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
					WithStatusSubresource(completedSP).Build()
				aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default")

				// Act
				name, err := aiCreator.Create(ctx, rr, completedSP)

				// Assert - should succeed, partial data is better than no data
				Expect(err).ToNot(HaveOccurred())

				createdAI := &aianalysisv1.AIAnalysis{}
				err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
				Expect(err).ToNot(HaveOccurred())

				// AIAnalysis should be created even with incomplete enrichment
				Expect(createdAI.Spec.RemediationRequestRef.Name).To(Equal(rr.Name))
			})
		})

		// BR-ORCH-025: Error handling ensures failures are propagated correctly
		Context("BR-ORCH-025: Error handling for infrastructure failures", func() {
			DescribeTable("should return appropriate error when client operations fail",
				func(errorType string, interceptFunc interceptor.Funcs, expectedError string) {
					// Arrange
					completedSP := helpers.NewCompletedSignalProcessing("sp-test-remediation", "default")
					fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
						WithInterceptorFuncs(interceptFunc).Build()
					aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
					rr := helpers.NewRemediationRequest("test-remediation", "default")

					// Act
					_, err := aiCreator.Create(ctx, rr, completedSP)

					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedError))
				},
				Entry("Get fails with non-NotFound error - propagates to allow RO to mark RR as Failed",
					"get_error",
					interceptor.Funcs{
						Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							// Return error for AIAnalysis Get
							if _, ok := obj.(*aianalysisv1.AIAnalysis); ok {
								return fmt.Errorf("network error")
							}
							return c.Get(ctx, key, obj, opts...)
						},
					},
					"failed to check existing AIAnalysis",
				),
				Entry("Create fails with API server error - propagates to allow RO to mark RR as Failed",
					"create_error",
					interceptor.Funcs{
						Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
							return fmt.Errorf("API server unavailable")
						},
					},
					"failed to create AIAnalysis",
				),
			)
		})

		// BR-SP-106 / BR-AI-084: Predictive Signal Mode
		Context("BR-SP-106: Signal mode propagation from SP to AA", func() {
			It("UT-RO-106-001: should copy SignalMode from SP status to AA spec", func() {
				// Arrange: SP has predictive signal mode
				completedSP := helpers.NewCompletedSignalProcessing("sp-test-remediation", "default")
				completedSP.Status.SignalMode = "predictive"
				completedSP.Status.SignalType = "OOMKilled"             // normalized
				completedSP.Status.OriginalSignalType = "PredictedOOMKill" // preserved for audit

				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
					WithStatusSubresource(completedSP).Build()
				aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default", helpers.RemediationRequestOpts{
					SignalType: "PredictedOOMKill", // Raw type from Gateway
				})

				// Act
				name, err := aiCreator.Create(ctx, rr, completedSP)

				// Assert
				Expect(err).ToNot(HaveOccurred())

				createdAI := &aianalysisv1.AIAnalysis{}
				err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
				Expect(err).ToNot(HaveOccurred())

				// SignalMode should be copied from SP status
				Expect(createdAI.Spec.AnalysisRequest.SignalContext.SignalMode).To(Equal("predictive"))
			})

			It("UT-RO-106-002: should read SignalType from SP status (not RR spec)", func() {
				// Arrange: SP has normalized signal type
				completedSP := helpers.NewCompletedSignalProcessing("sp-test-remediation", "default")
				completedSP.Status.SignalMode = "predictive"
				completedSP.Status.SignalType = "OOMKilled"             // Normalized by SP
				completedSP.Status.OriginalSignalType = "PredictedOOMKill"

				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
					WithStatusSubresource(completedSP).Build()
				aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default", helpers.RemediationRequestOpts{
					SignalType: "PredictedOOMKill", // Raw type from Gateway (should NOT be used)
				})

				// Act
				name, err := aiCreator.Create(ctx, rr, completedSP)

				// Assert
				Expect(err).ToNot(HaveOccurred())

				createdAI := &aianalysisv1.AIAnalysis{}
				err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
				Expect(err).ToNot(HaveOccurred())

				// SignalType should come from SP status (normalized), NOT from RR spec
				Expect(createdAI.Spec.AnalysisRequest.SignalContext.SignalType).To(Equal("OOMKilled"))
				Expect(createdAI.Spec.AnalysisRequest.SignalContext.SignalType).ToNot(Equal("PredictedOOMKill"))
			})

			It("should default to reactive signal mode for reactive signals", func() {
				// Arrange: SP has reactive signal mode
				completedSP := helpers.NewCompletedSignalProcessing("sp-test-remediation", "default")
				// NewCompletedSignalProcessing defaults to reactive/prometheus

				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
					WithStatusSubresource(completedSP).Build()
				aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default")

				// Act
				name, err := aiCreator.Create(ctx, rr, completedSP)

				// Assert
				Expect(err).ToNot(HaveOccurred())

				createdAI := &aianalysisv1.AIAnalysis{}
				err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
				Expect(err).ToNot(HaveOccurred())

				Expect(createdAI.Spec.AnalysisRequest.SignalContext.SignalMode).To(Equal("reactive"))
				// SignalType for reactive comes from SP status (which mirrors Spec.Signal.Type)
				Expect(createdAI.Spec.AnalysisRequest.SignalContext.SignalType).To(Equal("prometheus"))
			})
		})
	})

	Context("CustomLabels pass-through (BR-SP-102)", func() {
		It("should copy CustomLabels from SP KubernetesContext to AIAnalysis EnrichmentResults", func() {
			completedSP := helpers.NewCompletedSignalProcessing("sp-customlabels", "default")
			completedSP.Status.KubernetesContext.CustomLabels = map[string][]string{
				"team":        {"platform"},
				"tier":        {"critical"},
				"cost-center": {"eng-42"},
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
				WithStatusSubresource(completedSP).Build()
			aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-customlabels", "default")

			name, err := aiCreator.Create(ctx, rr, completedSP)

			Expect(err).ToNot(HaveOccurred())
			createdAI := &aianalysisv1.AIAnalysis{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
			Expect(err).ToNot(HaveOccurred())

			Expect(createdAI.Spec.AnalysisRequest.SignalContext.EnrichmentResults.KubernetesContext).ToNot(BeNil())
			Expect(createdAI.Spec.AnalysisRequest.SignalContext.EnrichmentResults.KubernetesContext.CustomLabels).To(HaveKeyWithValue("team", []string{"platform"}))
			Expect(createdAI.Spec.AnalysisRequest.SignalContext.EnrichmentResults.KubernetesContext.CustomLabels).To(HaveKeyWithValue("tier", []string{"critical"}))
			Expect(createdAI.Spec.AnalysisRequest.SignalContext.EnrichmentResults.KubernetesContext.CustomLabels).To(HaveKeyWithValue("cost-center", []string{"eng-42"}))
		})

		It("should not set CustomLabels when SP has no CustomLabels", func() {
			completedSP := helpers.NewCompletedSignalProcessing("sp-no-labels", "default")

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
				WithStatusSubresource(completedSP).Build()
			aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-no-labels", "default")

			name, err := aiCreator.Create(ctx, rr, completedSP)

			Expect(err).ToNot(HaveOccurred())
			createdAI := &aianalysisv1.AIAnalysis{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
			Expect(err).ToNot(HaveOccurred())

			er := createdAI.Spec.AnalysisRequest.SignalContext.EnrichmentResults
			Expect(er.KubernetesContext == nil || len(er.KubernetesContext.CustomLabels) == 0).To(BeTrue(),
				"CustomLabels should be empty when SP has no CustomLabels")
		})
	})

	Context("BusinessClassification pass-through (BR-SP-002, BR-SP-080, BR-SP-081)", func() {
		It("should copy BusinessClassification from SP status to AIAnalysis EnrichmentResults", func() {
			completedSP := helpers.NewCompletedSignalProcessing("sp-bizclass", "default")
			completedSP.Status.BusinessClassification = &signalprocessingv1.BusinessClassification{
				BusinessUnit:   "payments",
				ServiceOwner:   "team-checkout",
				Criticality:    "critical",
				SLARequirement: "platinum",
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
				WithStatusSubresource(completedSP).Build()
			aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-bizclass", "default")

			name, err := aiCreator.Create(ctx, rr, completedSP)

			Expect(err).ToNot(HaveOccurred())
			createdAI := &aianalysisv1.AIAnalysis{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
			Expect(err).ToNot(HaveOccurred())

			bc := createdAI.Spec.AnalysisRequest.SignalContext.EnrichmentResults.BusinessClassification
			Expect(bc).ToNot(BeNil())
			Expect(bc.BusinessUnit).To(Equal("payments"))
			Expect(bc.ServiceOwner).To(Equal("team-checkout"))
			Expect(bc.Criticality).To(Equal("critical"))
			Expect(bc.SLARequirement).To(Equal("platinum"))
		})

		It("should not set BusinessClassification when SP has none", func() {
			completedSP := helpers.NewCompletedSignalProcessing("sp-no-bizclass", "default")

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
				WithStatusSubresource(completedSP).Build()
			aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-no-bizclass", "default")

			name, err := aiCreator.Create(ctx, rr, completedSP)

			Expect(err).ToNot(HaveOccurred())
			createdAI := &aianalysisv1.AIAnalysis{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
			Expect(err).ToNot(HaveOccurred())

			Expect(createdAI.Spec.AnalysisRequest.SignalContext.EnrichmentResults.BusinessClassification).To(BeNil())
		})

		It("should handle partial BusinessClassification fields", func() {
			completedSP := helpers.NewCompletedSignalProcessing("sp-partial-bizclass", "default")
			completedSP.Status.BusinessClassification = &signalprocessingv1.BusinessClassification{
				Criticality: "high",
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedSP).
				WithStatusSubresource(completedSP).Build()
			aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-partial-bizclass", "default")

			name, err := aiCreator.Create(ctx, rr, completedSP)

			Expect(err).ToNot(HaveOccurred())
			createdAI := &aianalysisv1.AIAnalysis{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdAI)
			Expect(err).ToNot(HaveOccurred())

			bc := createdAI.Spec.AnalysisRequest.SignalContext.EnrichmentResults.BusinessClassification
			Expect(bc).ToNot(BeNil())
			Expect(bc.Criticality).To(Equal("high"))
			Expect(bc.BusinessUnit).To(BeEmpty())
			Expect(bc.ServiceOwner).To(BeEmpty())
			Expect(bc.SLARequirement).To(BeEmpty())
		})
	})
})
