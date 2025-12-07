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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/client"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

// BR-AI-007: InvestigatingHandler tests
var _ = Describe("InvestigatingHandler", func() {
	var (
		handler    *handlers.InvestigatingHandler
		mockClient *testutil.MockHolmesGPTClient
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = testutil.NewMockHolmesGPTClient()
		handler = handlers.NewInvestigatingHandler(mockClient, ctrl.Log.WithName("test"))
	})

	// Helper to create valid AIAnalysis
	createTestAnalysis := func() *aianalysisv1.AIAnalysis {
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-analysis",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Kind:      "RemediationRequest",
					Name:      "test-rr",
					Namespace: "default",
				},
				RemediationID: "test-remediation-001",
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      "test-fingerprint",
						Severity:         "warning",
						SignalType:       "OOMKilled",
						Environment:      "production",
						BusinessPriority: "P0",
						TargetResource: aianalysisv1.TargetResource{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
					AnalysisTypes: []string{"investigation"},
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: aianalysis.PhaseInvestigating,
			},
		}
	}

	Describe("Handle", func() {
		// BR-AI-007: Process HolmesGPT response
		// NOTE: To proceed to Analyzing phase, HAPI MUST return a SelectedWorkflow
		// If no workflow is returned with high confidence, it triggers "Resolved" (BR-HAPI-200)
		Context("with successful API response including workflow", func() {
			BeforeEach(func() {
				mockClient.WithFullResponse(
					"Root cause identified: OOM",
					0.9,
					true,
					[]string{},
					nil, // no RCA needed for this test
					&client.SelectedWorkflow{
						WorkflowID:     "wf-restart-pod",
						ContainerImage: "kubernaut.io/workflows/restart:v1.0.0",
						Confidence:     0.9,
						Rationale:      "Selected for OOM recovery",
					},
					nil, // no alternatives
				)
			})

			// BR-AI-007: Business outcome - investigation completes and proceeds to analysis
			It("should complete investigation and proceed to policy analysis", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// Business outcome: Investigation completes successfully
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseAnalyzing), "Should proceed to Analyzing phase")
				// Business outcome: Data quality indicator captured for policy evaluation
				Expect(analysis.Status.TargetInOwnerChain).NotTo(BeNil(), "Data quality indicator should be captured")
				Expect(*analysis.Status.TargetInOwnerChain).To(BeTrue(), "Target should be found in owner chain")
			})
		})

		// BR-AI-008: Handle warnings
		// NOTE: To proceed to Analyzing phase, HAPI MUST return a SelectedWorkflow
		Context("with warnings in response including workflow", func() {
			BeforeEach(func() {
				mockClient.WithFullResponse(
					"Analysis with warnings",
					0.7,
					false,
					[]string{"High memory pressure", "Node scheduling delayed"},
					nil, // no RCA
					&client.SelectedWorkflow{
						WorkflowID:     "wf-scale-deployment",
						ContainerImage: "kubernaut.io/workflows/scale:v1.0.0",
						Confidence:     0.7,
						Rationale:      "Scale to handle memory pressure",
					},
					nil, // no alternatives
				)
			})

			It("should capture warnings in status", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Warnings).To(HaveLen(2))
				Expect(analysis.Status.Warnings).To(ContainElement("High memory pressure"))
				Expect(analysis.Status.Warnings).To(ContainElement("Node scheduling delayed"))
			})

			It("should set targetInOwnerChain to false", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.TargetInOwnerChain).NotTo(BeNil())
				Expect(*analysis.Status.TargetInOwnerChain).To(BeFalse())
			})
		})

		// BR-AI-008: v1.5 Response Fields - RootCauseAnalysis capture
		Context("with full v1.5 response (RCA, Workflow, Alternatives)", func() {
			BeforeEach(func() {
				mockClient.WithFullResponse(
					"Full analysis with all fields",  // analysis
					0.92,                             // confidence
					true,                             // targetInChain
					[]string{"Memory pressure high"}, // warnings
					&client.RootCauseAnalysis{
						Summary:             "OOM caused by memory leak",
						Severity:            "high",
						ContributingFactors: []string{"Memory leak in container"},
					}, // rca
					&client.SelectedWorkflow{
						WorkflowID:     "wf-restart-pod",
						Version:        "1.0.0",
						ContainerImage: "kubernaut.io/workflows/restart:v1.0.0",
						Confidence:     0.92,
						Rationale:      "Selected for OOM recovery",
						Parameters:     map[string]string{"NAMESPACE": "default"},
					}, // selectedWorkflow
					[]client.AlternativeWorkflow{
						{
							WorkflowID:     "wf-scale-deployment",
							ContainerImage: "kubernaut.io/workflows/scale:v1.0.0",
							Confidence:     0.75,
							Rationale:      "Consider scaling if restart fails",
						},
					}, // alternatives
				)
			})

			// BR-AI-008: RootCauseAnalysis capture
			It("should capture RootCauseAnalysis in status", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.RootCauseAnalysis).NotTo(BeNil())
				Expect(analysis.Status.RootCauseAnalysis.Summary).To(Equal("OOM caused by memory leak"))
				Expect(analysis.Status.RootCauseAnalysis.Severity).To(Equal("high"))
				Expect(analysis.Status.RootCause).To(Equal("OOM caused by memory leak"))
			})

			// BR-AI-008: SelectedWorkflow capture (DD-CONTRACT-002)
			It("should capture SelectedWorkflow in status", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.SelectedWorkflow).NotTo(BeNil())
				Expect(analysis.Status.SelectedWorkflow.WorkflowID).To(Equal("wf-restart-pod"))
				Expect(analysis.Status.SelectedWorkflow.ContainerImage).To(Equal("kubernaut.io/workflows/restart:v1.0.0"))
				Expect(analysis.Status.SelectedWorkflow.Confidence).To(BeNumerically("~", 0.92, 0.01))
				Expect(analysis.Status.SelectedWorkflow.Rationale).To(Equal("Selected for OOM recovery"))
			})

			// BR-AI-008: AlternativeWorkflows capture (Q12 - for audit/context only)
			It("should capture AlternativeWorkflows in status for operator context", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.AlternativeWorkflows).To(HaveLen(1))
				Expect(analysis.Status.AlternativeWorkflows[0].WorkflowID).To(Equal("wf-scale-deployment"))
				Expect(analysis.Status.AlternativeWorkflows[0].Confidence).To(BeNumerically("~", 0.75, 0.01))
				Expect(analysis.Status.AlternativeWorkflows[0].Rationale).To(ContainSubstring("scaling"))
			})
		})

		// BR-AI-009: Business outcome - transient errors allow investigation to recover
		// BR-AI-010: Business outcome - permanent errors fail fast to prevent infinite loops
		// Using DescribeTable per 03-testing-strategy.mdc for reduced duplication
		DescribeTable("handles API errors appropriately for business continuity",
			func(statusCode int, shouldRetry bool, expectedPhase string) {
				mockClient.WithAPIError(statusCode, "API Error")
				analysis := createTestAnalysis()

				result, err := handler.Handle(ctx, analysis)

				if shouldRetry {
					// Business outcome: Transient errors allow recovery (service may come back)
					Expect(err).To(HaveOccurred(), "Transient error should signal retry")
					Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Should schedule retry")
					Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating), "Should stay in Investigating for retry")
				} else {
					// Business outcome: Permanent errors fail fast (don't waste resources)
					Expect(err).NotTo(HaveOccurred())
					Expect(analysis.Status.Phase).To(Equal(expectedPhase), "Should transition to failed state")
				}
			},
			// BR-AI-009: Transient errors - system should recover automatically
			Entry("503 - service unavailable: allow recovery", 503, true, aianalysis.PhaseInvestigating),
			Entry("429 - rate limited: backoff and retry", 429, true, aianalysis.PhaseInvestigating),
			Entry("502 - gateway error: allow recovery", 502, true, aianalysis.PhaseInvestigating),
			Entry("504 - timeout: allow recovery", 504, true, aianalysis.PhaseInvestigating),

			// BR-AI-010: Permanent errors - fail immediately, don't retry
			Entry("401 - unauthorized: misconfigured credentials", 401, false, aianalysis.PhaseFailed),
			Entry("400 - bad request: invalid analysis request", 400, false, aianalysis.PhaseFailed),
			Entry("403 - forbidden: access denied", 403, false, aianalysis.PhaseFailed),
			Entry("404 - not found: resource doesn't exist", 404, false, aianalysis.PhaseFailed),
		)

		// BR-AI-009: Business outcome - prevent infinite retry loops
		Context("when transient errors persist beyond retry limit", func() {
			BeforeEach(func() {
				mockClient.WithAPIError(503, "Service Unavailable")
			})

			It("should fail gracefully after exhausting retry budget", func() {
				analysis := createTestAnalysis()
				// Simulate max retries already reached
				analysis.Annotations = map[string]string{
					handlers.RetryCountAnnotation: "5",
				}

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// Business outcome: Analysis fails gracefully (doesn't hang indefinitely)
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed), "Should fail after exhausting retries")
				Expect(analysis.Status.Message).To(ContainSubstring("max retries"), "Should explain retry exhaustion")
			})
		})

		// Test mock call tracking
		Context("mock call verification", func() {
			It("should track API calls", func() {
				analysis := createTestAnalysis()

				Expect(mockClient.AssertNotCalled()).To(Succeed())

				_, _ = handler.Handle(ctx, analysis)

				Expect(mockClient.AssertCalled(1)).To(Succeed())
			})
		})

		// ========================================
		// BR-HAPI-197: Human Review Required Handling
		// When HolmesGPT-API returns needs_human_review=true, AIAnalysis MUST:
		// - NOT proceed to Analyzing phase
		// - Fail with structured reason (WorkflowResolutionFailed + SubReason)
		// - Preserve partial response for operator context
		// ========================================

		Context("when HolmesGPT-API returns needs_human_review=true", func() {
			// BR-HAPI-197: Preferred method - use human_review_reason enum
			DescribeTable("should map human_review_reason enum to SubReason",
				func(humanReviewReason string, expectedSubReason string) {
					mockClient.WithHumanReviewReasonEnum(humanReviewReason, []string{"test warning"})
					analysis := createTestAnalysis()

					_, err := handler.Handle(ctx, analysis)

					Expect(err).NotTo(HaveOccurred())
					Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed), "Should fail immediately")
					Expect(analysis.Status.Reason).To(Equal("WorkflowResolutionFailed"), "Should use umbrella reason")
					Expect(analysis.Status.SubReason).To(Equal(expectedSubReason), "Should map enum to SubReason")
				},
				Entry("workflow_not_found → WorkflowNotFound", "workflow_not_found", "WorkflowNotFound"),
				Entry("image_mismatch → ImageMismatch", "image_mismatch", "ImageMismatch"),
				Entry("parameter_validation_failed → ParameterValidationFailed", "parameter_validation_failed", "ParameterValidationFailed"),
				Entry("no_matching_workflows → NoMatchingWorkflows", "no_matching_workflows", "NoMatchingWorkflows"),
				Entry("low_confidence → LowConfidence", "low_confidence", "LowConfidence"),
				Entry("llm_parsing_error → LLMParsingError", "llm_parsing_error", "LLMParsingError"),
				// BR-HAPI-200: New investigation outcome
				Entry("investigation_inconclusive → InvestigationInconclusive", "investigation_inconclusive", "InvestigationInconclusive"),
			)

			// BR-HAPI-197: Backward compatibility - fallback to warning parsing
			// Business Value: Operators can diagnose failures even with older HAPI versions
			DescribeTable("should fallback to warning parsing when enum is nil",
				func(warnings []string, expectedSubReason string) {
					mockClient.WithHumanReviewRequired(warnings)
					analysis := createTestAnalysis()

					_, err := handler.Handle(ctx, analysis)

					Expect(err).NotTo(HaveOccurred())
					Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
					Expect(analysis.Status.Reason).To(Equal("WorkflowResolutionFailed"))
					Expect(analysis.Status.SubReason).To(Equal(expectedSubReason))
				},
				// WorkflowNotFound patterns
				Entry("'not found' in warning → WorkflowNotFound",
					[]string{"Workflow 'restart-pod-v1' not found in catalog"}, "WorkflowNotFound"),
				Entry("'does not exist' in warning → WorkflowNotFound",
					[]string{"Workflow does not exist in catalog"}, "WorkflowNotFound"),
				// NoMatchingWorkflows patterns
				Entry("'no workflows matched' in warning → NoMatchingWorkflows",
					[]string{"No workflows matched the incident criteria"}, "NoMatchingWorkflows"),
				Entry("'no matching' in warning → NoMatchingWorkflows",
					[]string{"No matching workflow for OOMKilled signal"}, "NoMatchingWorkflows"),
				// LowConfidence patterns
				Entry("'confidence below' in warning → LowConfidence",
					[]string{"Confidence (0.55) below threshold (0.70)"}, "LowConfidence"),
				// ParameterValidationFailed patterns
				Entry("'parameter validation' in warning → ParameterValidationFailed",
					[]string{"Parameter validation failed for workflow"}, "ParameterValidationFailed"),
				Entry("'missing required' in warning → ParameterValidationFailed",
					[]string{"Missing required parameter: namespace"}, "ParameterValidationFailed"),
				// ImageMismatch patterns
				Entry("'image mismatch' in warning → ImageMismatch",
					[]string{"Image mismatch: expected v1.0.0, got v2.0.0"}, "ImageMismatch"),
				Entry("'container image' in warning → ImageMismatch",
					[]string{"Container image validation failed"}, "ImageMismatch"),
				// LLMParsingError patterns
				Entry("'parse' in warning → LLMParsingError",
					[]string{"Failed to parse LLM response"}, "LLMParsingError"),
				Entry("'invalid json' in warning → LLMParsingError",
					[]string{"Invalid JSON in LLM output"}, "LLMParsingError"),
				// Default case (unknown warning)
				Entry("unknown warning → WorkflowNotFound (default)",
					[]string{"Some completely unexpected error"}, "WorkflowNotFound"),
			)

			// BR-HAPI-197: Unknown enum value handling
			// Business Value: System handles new HAPI enum values gracefully
			It("should default to WorkflowNotFound for unknown human_review_reason enum", func() {
				mockClient.WithHumanReviewReasonEnum("some_future_enum_value", []string{"Unknown reason"})
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
				Expect(analysis.Status.SubReason).To(Equal("WorkflowNotFound"), "Should default to WorkflowNotFound for unknown enum")
			})

			// BR-HAPI-197.4: Preserve partial response for operator context
			It("should preserve partial workflow and RCA for operator context", func() {
				reason := "parameter_validation_failed"
				mockClient.WithHumanReviewRequiredWithPartialResponse(
					&reason,
					[]string{"Parameter validation failed: Missing 'namespace'"},
					&client.SelectedWorkflow{
						WorkflowID:     "invalid-workflow",
						ContainerImage: "kubernaut.io/workflows/restart:v1.0.0",
						Confidence:     0.85,
						Rationale:      "AI attempted this workflow",
					},
				)
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
				// Partial workflow preserved
				Expect(analysis.Status.SelectedWorkflow).NotTo(BeNil(), "Partial workflow should be preserved")
				Expect(analysis.Status.SelectedWorkflow.WorkflowID).To(Equal("invalid-workflow"))
				// RCA preserved
				Expect(analysis.Status.RootCauseAnalysis).NotTo(BeNil(), "RCA should be preserved")
				// Message contains warnings
				Expect(analysis.Status.Message).To(ContainSubstring("Parameter validation failed"))
			})

			// BR-HAPI-197.6: MUST NOT proceed to Analyzing
			It("should NOT proceed to Analyzing phase (terminal failure)", func() {
				mockClient.WithHumanReviewReasonEnum("workflow_not_found", []string{"Workflow not found"})
				analysis := createTestAnalysis()

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed), "Should be Failed, not Analyzing")
				Expect(result.Requeue).To(BeFalse(), "Should NOT requeue (terminal)")
				Expect(result.RequeueAfter).To(BeZero(), "Should NOT schedule requeue")
			})

			// ========================================
			// DD-HAPI-002 v1.4: ValidationAttemptsHistory Support
			// HAPI retries up to 3 times with LLM self-correction
			// validation_attempts_history provides audit trail
			// ========================================

			// DD-HAPI-002 v1.4: Store validation attempts history for audit
			It("should store validation attempts history for audit/debugging", func() {
				mockClient.WithHumanReviewAndHistory(
					"parameter_validation_failed",
					[]string{"Workflow validation failed after 3 attempts"},
					[]client.ValidationAttempt{
						{Attempt: 1, WorkflowID: "bad-workflow-1", IsValid: false, Errors: []string{"Workflow not found"}, Timestamp: "2025-12-06T10:00:00Z"},
						{Attempt: 2, WorkflowID: "restart-pod", IsValid: false, Errors: []string{"Image mismatch"}, Timestamp: "2025-12-06T10:00:05Z"},
						{Attempt: 3, WorkflowID: "restart-pod", IsValid: false, Errors: []string{"Missing parameter: namespace"}, Timestamp: "2025-12-06T10:00:10Z"},
					},
				)
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
				Expect(analysis.Status.ValidationAttemptsHistory).To(HaveLen(3), "Should store all 3 validation attempts")

				// Verify attempt details
				Expect(analysis.Status.ValidationAttemptsHistory[0].Attempt).To(Equal(1))
				Expect(analysis.Status.ValidationAttemptsHistory[0].WorkflowID).To(Equal("bad-workflow-1"))
				Expect(analysis.Status.ValidationAttemptsHistory[0].Errors).To(ContainElement("Workflow not found"))

				Expect(analysis.Status.ValidationAttemptsHistory[2].Attempt).To(Equal(3))
				Expect(analysis.Status.ValidationAttemptsHistory[2].Errors).To(ContainElement("Missing parameter: namespace"))
			})

			// DD-HAPI-002 v1.4: Build detailed message from validation attempts
			It("should build operator-friendly message from validation attempts history", func() {
				mockClient.WithHumanReviewAndHistory(
					"llm_parsing_error",
					[]string{"LLM parsing failed"},
					testutil.NewMockValidationAttempts([]string{
						"Invalid JSON response",
						"Missing workflow_id field",
						"Schema validation failed",
					}),
				)
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// Message should contain attempt details for operator notification
				Expect(analysis.Status.Message).To(ContainSubstring("Attempt 1"))
				Expect(analysis.Status.Message).To(ContainSubstring("Invalid JSON response"))
				Expect(analysis.Status.Message).To(ContainSubstring("Attempt 3"))
				Expect(analysis.Status.Message).To(ContainSubstring("Schema validation failed"))
			})

			// DD-HAPI-002 v1.4: Parse timestamps correctly
			It("should parse validation attempt timestamps", func() {
				mockClient.WithHumanReviewAndHistory(
					"workflow_not_found",
					[]string{"Workflow not found"},
					[]client.ValidationAttempt{
						{Attempt: 1, WorkflowID: "test", IsValid: false, Errors: []string{"error"}, Timestamp: "2025-12-06T10:00:00Z"},
					},
				)
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ValidationAttemptsHistory).To(HaveLen(1))
				Expect(analysis.Status.ValidationAttemptsHistory[0].Timestamp.IsZero()).To(BeFalse(), "Timestamp should be parsed")
			})

			// DD-HAPI-002 v1.4: Handle empty validation history (backward compatibility)
			It("should fallback to warnings when validation history is empty", func() {
				mockClient.WithHumanReviewReasonEnum("low_confidence", []string{"Confidence too low"})
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ValidationAttemptsHistory).To(BeEmpty(), "No history when HAPI doesn't provide it")
				Expect(analysis.Status.Message).To(Equal("Confidence too low"), "Should use warnings for message")
			})

			// DD-HAPI-002 v1.4: Handle malformed timestamp gracefully
			// Business Value: System doesn't crash on bad HAPI data, provides fallback
			It("should fallback to current time when timestamp is malformed", func() {
				mockClient.WithHumanReviewAndHistory(
					"workflow_not_found",
					[]string{"Workflow not found"},
					[]client.ValidationAttempt{
						{Attempt: 1, WorkflowID: "test", IsValid: false, Errors: []string{"error"}, Timestamp: "not-a-valid-timestamp"},
					},
				)
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ValidationAttemptsHistory).To(HaveLen(1))
				// Timestamp should be set to current time (fallback), not zero
				Expect(analysis.Status.ValidationAttemptsHistory[0].Timestamp.IsZero()).To(BeFalse(), "Should use current time as fallback")
			})
		})
	})

	// ========================================
	// BR-HAPI-200 OUTCOME A: PROBLEM SELF-RESOLVED
	// When needs_human_review=false AND selected_workflow=null AND confidence >= 0.7
	// This represents incidents that self-healed (e.g., OOMKilled pod restarted)
	// ========================================
	Describe("Problem Resolved Handling (BR-HAPI-200)", func() {
		// BR-HAPI-200: Core behavior - problem resolved with high confidence
		Context("when problem is confidently resolved", func() {
			BeforeEach(func() {
				mockClient.WithProblemResolved(
					0.92, // High confidence
					[]string{"Problem self-resolved - no remediation required"},
					"Investigated OOMKilled signal. Pod 'myapp' recovered automatically. Status: Running, memory at 45% of limit.",
				)
			})

			// BR-HAPI-200: Business outcome - no workflow execution, mark complete
			It("should complete without workflow execution", func() {
				analysis := createTestAnalysis()

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// Business outcome: Analysis completes successfully (no remediation needed)
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted), "Should be Completed (not Analyzing)")
				Expect(analysis.Status.Reason).To(Equal("WorkflowNotNeeded"), "Should indicate no workflow needed")
				Expect(analysis.Status.SubReason).To(Equal("ProblemResolved"), "Should specify problem self-resolved")
				// Terminal state - no requeue
				Expect(result.Requeue).To(BeFalse(), "Should NOT requeue (terminal success)")
				Expect(result.RequeueAfter).To(BeZero(), "Should NOT schedule requeue")
			})

			// BR-HAPI-200: Message should contain investigation summary
			It("should capture investigation summary in message", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Message).To(ContainSubstring("OOMKilled"))
				Expect(analysis.Status.Message).To(ContainSubstring("recovered automatically"))
			})

			// BR-HAPI-200: Warnings should be preserved
			It("should preserve warnings for audit trail", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Warnings).To(HaveLen(1))
				Expect(analysis.Status.Warnings).To(ContainElement("Problem self-resolved - no remediation required"))
			})
		})

		// BR-HAPI-200: Preserve RCA even when no workflow needed
		Context("when problem resolved with RCA available", func() {
			BeforeEach(func() {
				mockClient.WithProblemResolvedAndRCA(
					0.85,
					[]string{"Transient failure - recovered"},
					"Memory pressure subsided after pod restart",
					&client.RootCauseAnalysis{
						Summary:             "OOM caused by temporary memory spike",
						Severity:            "low",
						ContributingFactors: []string{"Temporary memory pressure", "Pod restarted automatically"},
					},
				)
			})

			// BR-HAPI-200: RCA preserved for audit/learning
			It("should preserve RCA for audit/learning even when no workflow executed", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted))
				Expect(analysis.Status.RootCauseAnalysis).NotTo(BeNil(), "RCA should be preserved")
				Expect(analysis.Status.RootCauseAnalysis.Summary).To(ContainSubstring("memory spike"))
				Expect(analysis.Status.RootCauseAnalysis.ContributingFactors).To(HaveLen(2))
			})
		})

		// BR-HAPI-200: Confidence threshold boundary tests
		DescribeTable("should respect confidence threshold of 0.7",
			func(confidence float64, shouldBeResolved bool) {
				if shouldBeResolved {
					mockClient.WithProblemResolved(confidence, []string{"Resolved"}, "Problem resolved")
				} else {
					// Low confidence without workflow = should NOT trigger resolved
					// This goes to normal flow (Analyzing) since no human review needed
					mockClient.WithFullResponse(
						"Low confidence analysis",
						confidence,
						true,
						[]string{},
						nil, // no RCA
						&client.SelectedWorkflow{ // Must have workflow to proceed to Analyzing
							WorkflowID:     "wf-test",
							ContainerImage: "test:v1",
							Confidence:     confidence,
							Rationale:      "Test workflow",
						},
						nil, // no alternatives
					)
				}
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				if shouldBeResolved {
					Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted), "Should be Completed (resolved)")
					Expect(analysis.Status.Reason).To(Equal("WorkflowNotNeeded"))
				} else {
					Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseAnalyzing), "Should proceed to Analyzing (has workflow)")
				}
			},
			// At threshold: problem is confidently resolved
			Entry("confidence = 0.70 (at threshold) → resolved", 0.70, true),
			Entry("confidence = 0.71 (above threshold) → resolved", 0.71, true),
			Entry("confidence = 0.85 (well above) → resolved", 0.85, true),
			Entry("confidence = 0.99 (near certainty) → resolved", 0.99, true),
			// Below threshold with workflow → proceeds to Analyzing (not resolved)
			Entry("confidence = 0.69 (below threshold) with workflow → Analyzing", 0.69, false),
			Entry("confidence = 0.50 (low) with workflow → Analyzing", 0.50, false),
		)

		// BR-HAPI-200: Fallback message when analysis is empty
		Context("when analysis text is empty", func() {
			It("should use warnings for message when analysis is empty", func() {
				mockClient.WithProblemResolved(0.92, []string{"Pod recovered"}, "") // Empty analysis
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted))
				Expect(analysis.Status.Message).To(Equal("Pod recovered"))
			})

			It("should use default message when both analysis and warnings are empty", func() {
				mockClient.WithProblemResolved(0.92, []string{}, "") // Empty both
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted))
				Expect(analysis.Status.Message).To(Equal("Problem self-resolved. No remediation required."))
			})
		})

		// BR-HAPI-200: Retry count reset on success
		Context("retry count management", func() {
			It("should reset retry count on successful resolution", func() {
				mockClient.WithProblemResolved(0.92, []string{"Resolved"}, "Problem resolved")
				analysis := createTestAnalysis()
				// Simulate previous retries
				analysis.Annotations = map[string]string{
					handlers.RetryCountAnnotation: "3",
				}

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted))
				// Retry count should be reset
				Expect(analysis.Annotations[handlers.RetryCountAnnotation]).To(Equal("0"))
			})
		})
	})

	// ========================================
	// RETRY MECHANISM EDGE CASES
	// ========================================
	Describe("Retry Mechanism", func() {
		// BR-AI-021: Exponential backoff for transient errors
		// Business Value: System retries intelligently without overwhelming HAPI

		Context("getRetryCount edge cases", func() {
			// Business Value: Handler correctly reads retry state from annotations
			It("should handle nil annotations gracefully (treats as 0 retries)", func() {
				analysis := createTestAnalysis()
				analysis.Annotations = nil

				// Using 503 (transient error) to trigger retry path
				mockClient.WithAPIError(503, "Service Unavailable")

				_, err := handler.Handle(ctx, analysis)

				// Should requeue (transient error triggers retry) - error indicates requeue
				Expect(err).To(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating), "Should stay in Investigating")
				// Should have created annotations and set retry count to 1
				Expect(analysis.Annotations).NotTo(BeNil())
				Expect(analysis.Annotations["aianalysis.kubernaut.ai/retry-count"]).To(Equal("1"))
			})

			// Business Value: Handle corrupted annotation data gracefully
			It("should handle malformed retry count annotation (treats as 0)", func() {
				analysis := createTestAnalysis()
				analysis.Annotations = map[string]string{
					"aianalysis.kubernaut.ai/retry-count": "not-a-number",
				}

				mockClient.WithAPIError(503, "Service Unavailable")

				_, err := handler.Handle(ctx, analysis)

				// Should requeue (transient error triggers retry)
				Expect(err).To(HaveOccurred())
				// Should behave as if retry count was 0, now incremented to 1
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating))
				Expect(analysis.Annotations["aianalysis.kubernaut.ai/retry-count"]).To(Equal("1"))
			})

			// Business Value: Correctly increments retry count on transient failures
			It("should increment retry count on transient error", func() {
				analysis := createTestAnalysis()
				analysis.Annotations = map[string]string{
					"aianalysis.kubernaut.ai/retry-count": "2",
				}

				mockClient.WithAPIError(503, "Service Unavailable")

				_, err := handler.Handle(ctx, analysis)

				Expect(err).To(HaveOccurred())
				// Retry count should be incremented from 2 to 3
				Expect(analysis.Annotations["aianalysis.kubernaut.ai/retry-count"]).To(Equal("3"))
			})
		})
	})
})
