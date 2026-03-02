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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	hgptclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// ========================================
// Test Mocks
// ========================================

// noopAuditClient is a no-op implementation of AuditClientInterface for unit tests.
type noopAuditClient struct{}

func (n *noopAuditClient) RecordAIAgentCall(ctx context.Context, analysis *aianalysisv1.AIAnalysis, endpoint string, statusCode int, durationMs int) {
	// No-op: Unit tests don't need audit recording
}

func (n *noopAuditClient) RecordPhaseTransition(ctx context.Context, analysis *aianalysisv1.AIAnalysis, from, to string) {
	// No-op: Unit tests don't need audit recording
}

func (n *noopAuditClient) RecordAnalysisFailed(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) error {
	// No-op: Unit tests don't need audit recording
	return nil
}

func (n *noopAuditClient) RecordAnalysisComplete(ctx context.Context, analysis *aianalysisv1.AIAnalysis) {
	// No-op: Unit tests don't need audit recording
}

// BR-AA-HAPI-064: Session audit no-ops
func (n *noopAuditClient) RecordAIAgentSubmit(ctx context.Context, analysis *aianalysisv1.AIAnalysis, sessionID string) {
}

func (n *noopAuditClient) RecordAIAgentResult(ctx context.Context, analysis *aianalysisv1.AIAnalysis, investigationTimeMs int64) {
}

func (n *noopAuditClient) RecordAIAgentSessionLost(ctx context.Context, analysis *aianalysisv1.AIAnalysis, generation int32) {
}

// auditClientSpy is a spy implementation that records audit events for validation.
// BR-AUDIT-005 Gap #7: Unit tests validate ErrorDetails structure.
type auditClientSpy struct {
	failedAnalysisEvents []failedAnalysisEvent
}

type failedAnalysisEvent struct {
	analysis *aianalysisv1.AIAnalysis
	err      error
}

func (s *auditClientSpy) RecordAIAgentCall(ctx context.Context, analysis *aianalysisv1.AIAnalysis, endpoint string, statusCode int, durationMs int) {
	// Not tracked in spy for Gap #7 tests
}

func (s *auditClientSpy) RecordPhaseTransition(ctx context.Context, analysis *aianalysisv1.AIAnalysis, from, to string) {
	// Not tracked in spy for Gap #7 tests
}

func (s *auditClientSpy) RecordAnalysisFailed(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) error {
	s.failedAnalysisEvents = append(s.failedAnalysisEvents, failedAnalysisEvent{
		analysis: analysis,
		err:      err,
	})
	return nil
}

func (s *auditClientSpy) RecordAnalysisComplete(ctx context.Context, analysis *aianalysisv1.AIAnalysis) {
	// Not tracked in spy for Gap #7 tests
}

// BR-AA-HAPI-064: Session audit spy methods
func (s *auditClientSpy) RecordAIAgentSubmit(ctx context.Context, analysis *aianalysisv1.AIAnalysis, sessionID string) {
	// Not tracked in spy for Gap #7 tests
}

func (s *auditClientSpy) RecordAIAgentResult(ctx context.Context, analysis *aianalysisv1.AIAnalysis, investigationTimeMs int64) {
	// Not tracked in spy for Gap #7 tests
}

func (s *auditClientSpy) RecordAIAgentSessionLost(ctx context.Context, analysis *aianalysisv1.AIAnalysis, generation int32) {
	// Not tracked in spy for Gap #7 tests
}

func (s *auditClientSpy) getFailedEvents() []failedAnalysisEvent {
	return s.failedAnalysisEvents
}

// ========================================
// InvestigatingHandler Unit Tests
// BR-AI-007: HolmesGPT-API integration and error handling
// ========================================
//
// ERROR HANDLING BEHAVIOR (BR-AI-009, BR-AI-010):
//
// TRANSIENT ERRORS (Automatic Retry with Exponential Backoff):
//   - 503 Service Unavailable
//   - 429 Too Many Requests
//   - 500 Internal Server Error
//   - 502 Bad Gateway
//   - 504 Gateway Timeout
//   - Connection timeouts (context.DeadlineExceeded)
//   - Connection refused, connection reset
//
// Behavior: Phase stays "Investigating", ConsecutiveFailures incremented,
//           requeue with exponential backoff up to MaxRetries (5)
//
// PERMANENT ERRORS (Immediate Failure):
//   - 401 Unauthorized
//   - 403 Forbidden
//   - 404 Not Found
//   - Unknown errors (fail-safe default)
//
// Behavior: Phase transitions to "Failed", SubReason="PermanentError",
//           no requeue (operator intervention required)
//
// MAX RETRIES EXCEEDED:
//   - After ConsecutiveFailures > MaxRetries (5), transient errors
//     transition to permanent failure with SubReason="MaxRetriesExceeded"
//
// See:
//   - pkg/aianalysis/handlers/error_classifier.go (classification logic)
//   - pkg/shared/backoff/backoff.go (exponential backoff calculation)
//   - docs/handoff/TEAM_ANNOUNCEMENT_SHARED_BACKOFF.md (mandate)
//
// ========================================

// BR-AI-007: InvestigatingHandler tests
var _ = Describe("InvestigatingHandler", func() {
	var (
		handler    *handlers.InvestigatingHandler
		mockClient *mocks.MockHolmesGPTClient
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = mocks.NewMockHolmesGPTClient()
		// Create mock audit client (noop for unit tests) and metrics
		mockAuditClient := &noopAuditClient{}
		testMetrics := metrics.NewMetrics()
		handler = handlers.NewInvestigatingHandler(mockClient, ctrl.Log.WithName("test"), testMetrics, mockAuditClient)
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
						SignalName:       "OOMKilled",
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
					[]string{},
					"", // rcaSummary
					"", // rcaSeverity
					"wf-restart-pod",
					"kubernaut.io/workflows/restart:v1.0.0",
					0.9,
					"",    // workflowRationale
					false, // includeAlternatives
				)
			})

			// BR-AI-007: Business outcome - investigation completes and proceeds to analysis
			It("should complete investigation and proceed to policy analysis", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// Business outcome: Investigation completes successfully
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseAnalyzing), "Should proceed to Analyzing phase")
			})
		})

		// BR-AI-008: Handle warnings
		// NOTE: To proceed to Analyzing phase, HAPI MUST return a SelectedWorkflow
		Context("with warnings in response including workflow", func() {
			BeforeEach(func() {
				mockClient.WithFullResponse(
					"Analysis with warnings",
					0.7,
					[]string{"High memory pressure", "Node scheduling delayed"},
					"",                                    // rcaSummary
					"",                                    // rcaSeverity
					"wf-scale-deployment",                 // workflowID
					"kubernaut.io/workflows/scale:v1.0.0", // executionBundle
					0.7,                                   // workflowConfidence
					"",                                    // workflowRationale
					false,                                 // includeAlternatives
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
		})

		// BR-AI-008: v1.5 Response Fields - RootCauseAnalysis capture
		Context("with full v1.5 response (RCA, Workflow, Alternatives)", func() {
			BeforeEach(func() {
				mockClient.WithFullResponse(
					"Full analysis with all fields",         // analysis
					0.92,                                    // confidence
					[]string{"Memory pressure high"},        // warnings
					"OOM caused by memory leak",             // rcaSummary
					"high",                                  // rcaSeverity
					"wf-restart-pod",                        // workflowID
					"kubernaut.io/workflows/restart:v1.0.0", // executionBundle
					0.92,                                    // workflowConfidence
					"Selected for OOM recovery",             // workflowRationale
					true,                                    // includeAlternatives
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
				Expect(analysis.Status.SelectedWorkflow.ExecutionBundle).To(Equal("kubernaut.io/workflows/restart:v1.0.0"))
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

		// BR-AI-010: Permanent API error handling moved to integration tier
		// NOTE: Permanent error tests moved to test/integration/aianalysis/
		// These require real infrastructure to validate the complete error handling flow

		// BR-AI-009: Business outcome - prevent infinite retry loops
		Context("when transient errors persist beyond retry limit", func() {
			var auditSpy *auditClientSpy

			BeforeEach(func() {
				mockClient.WithError(&hgptclient.APIError{StatusCode: 503, Message: "Service Unavailable"})
				// Use audit spy to capture failure events for Gap #7 validation
				auditSpy = &auditClientSpy{}
				testMetrics := metrics.NewMetrics()
				handler = handlers.NewInvestigatingHandler(mockClient, ctrl.Log.WithName("test"), testMetrics, auditSpy)
			})

			It("should fail gracefully after exhausting retry budget", func() {
				analysis := createTestAnalysis()
				// Simulate max retries already reached (ConsecutiveFailures = 5)
				// Next error will increment to 6, which exceeds MaxRetries (5)
				analysis.Status.ConsecutiveFailures = 5

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// Business outcome: Analysis fails gracefully after max retries (doesn't hang indefinitely)
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed), "Should fail after exhausting retries")
				Expect(analysis.Status.Message).To(ContainSubstring("exceeded max retries"), "Should explain max retries exceeded")
				Expect(analysis.Status.SubReason).To(Equal("MaxRetriesExceeded"), "Should set SubReason to MaxRetriesExceeded")
				Expect(analysis.Status.ConsecutiveFailures).To(Equal(int32(6)), "Should increment failure count before failing")
				Expect(result.RequeueAfter).To(Equal(time.Duration(0)), "Should not requeue after max retries")

				// BR-AUDIT-005 Gap #7: Validate error audit with standardized ErrorDetails
				failedEvents := auditSpy.getFailedEvents()
				Expect(failedEvents).To(HaveLen(1), "Should record exactly 1 failure audit event")
				Expect(failedEvents[0].analysis.Name).To(Equal(analysis.Name))
				Expect(failedEvents[0].err).ToNot(BeNil(), "Should capture the error that caused failure")
				Expect(failedEvents[0].err.Error()).To(ContainSubstring("Service Unavailable"), "Should capture upstream error message")
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
					reason,
					[]string{"Parameter validation failed: Missing 'namespace'"},
					"invalid-workflow",
					"kubernaut.io/workflows/restart:v1.0.0",
					"AI attempted this workflow",
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
				Expect(result.RequeueAfter).To(BeZero(), "Should NOT requeue (terminal)")
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
					[]map[string]interface{}{
						{"attempt": 1, "workflow_id": "bad-workflow-1", "is_valid": false, "errors": []string{"Workflow not found"}, "timestamp": "2025-12-06T10:00:00Z"},
						{"attempt": 2, "workflow_id": "restart-pod", "is_valid": false, "errors": []string{"Image mismatch"}, "timestamp": "2025-12-06T10:00:05Z"},
						{"attempt": 3, "workflow_id": "restart-pod", "is_valid": false, "errors": []string{"Missing parameter: namespace"}, "timestamp": "2025-12-06T10:00:10Z"},
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
					mocks.NewMockValidationAttempts([]string{
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
					[]map[string]interface{}{
						{"attempt": 1, "workflow_id": "test", "is_valid": false, "errors": []string{"error"}, "timestamp": "2025-12-06T10:00:00Z"},
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
					[]map[string]interface{}{
						{"attempt": 1, "workflow_id": "test", "is_valid": false, "errors": []string{"error"}, "timestamp": "not-a-valid-timestamp"},
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
	Describe("InvestigatingHandler.HandleProblemResolved", func() {
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
				Expect(result.RequeueAfter).To(BeZero(), "Should NOT requeue (terminal success)")
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

		// BR-HAPI-200 + #208: RCA with contributing factors → human review (not resolved)
		Context("when problem resolved with RCA available", func() {
			BeforeEach(func() {
				mockClient.WithProblemResolvedAndRCA(
					0.85,
					[]string{"Transient failure - recovered"},
					"Memory pressure subsided after pod restart",
					"OOM caused by temporary memory spike",
					"low",
				)
			})

			// #208: RCA with contributing factors indicates a real analyzed problem.
			// Even if transient, the operator should be notified rather than silently closed.
			It("should escalate to human review and preserve RCA", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
					"#208: RCA with contributing factors should not be treated as resolved")
				Expect(analysis.Status.NeedsHumanReview).To(BeTrue())
				Expect(analysis.Status.RootCauseAnalysis).NotTo(BeNil(), "RCA should be preserved")
				Expect(analysis.Status.RootCauseAnalysis.Summary).To(ContainSubstring("memory spike"))
				Expect(analysis.Status.RootCauseAnalysis.ContributingFactors).To(HaveLen(2))
			})
		})

		// #208: Real problem identified but no workflow → should escalate to human review
		Context("when LLM identifies real problem with RCA but no workflow (#208)", func() {
			BeforeEach(func() {
				mockClient.WithProblemResolvedAndRCA(
					0.85,
					[]string{},
					"Orphaned PVCs from completed batch jobs that were not properly cleaned up",
					"Orphaned PVCs consuming disk space",
					"low",
				)
			})

			It("should escalate to human review instead of NoActionRequired", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// #208: Real problem with contributing factors should NOT be treated as "resolved"
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
					"Real problem with RCA should fail (not complete as NoActionRequired)")
				Expect(analysis.Status.NeedsHumanReview).To(BeTrue(),
					"Real problem without workflow should escalate to human review")
				Expect(analysis.Status.RootCauseAnalysis).NotTo(BeNil(),
					"RCA should be preserved for operator context")
			})
		})

		// BR-HAPI-200: Confidence threshold boundary tests
		DescribeTable("should respect confidence threshold of 0.7",
			func(confidence float64, shouldBeResolved bool) {
				if shouldBeResolved {
					mockClient.WithProblemResolved(confidence, []string{"Resolved"}, "Problem resolved")
				} else {
					// BR-AI-050: Low confidence (<0.7) with workflow → terminal failure (Failed phase)
					// These scenarios test that low confidence is correctly treated as a failure
					mockClient.WithFullResponse(
						"Low confidence analysis",
						confidence,
						[]string{},
						"", // rcaSummary
						"", // rcaSeverity
						"wf-test",
						"test:v1",
						confidence,
						"",    // workflowRationale
						false, // includeAlternatives
					)
				}
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				if shouldBeResolved {
					Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted), "Should be Completed (resolved)")
					Expect(analysis.Status.Reason).To(Equal("WorkflowNotNeeded"))
				} else {
					// BR-AI-050: Confidence <0.7 is terminal failure (reconciliation-phases.md v2.1)
					Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed), "Should fail for confidence <0.7 (BR-AI-050)")
					Expect(analysis.Status.Reason).To(Equal("WorkflowResolutionFailed"), "Umbrella category per structured taxonomy")
					Expect(analysis.Status.SubReason).To(Equal("LowConfidence"), "Specific cause per reconciliation-phases.md v2.1:334")
				}
			},
			// At threshold: problem is confidently resolved
			Entry("confidence = 0.70 (at threshold) → resolved", 0.70, true),
			Entry("confidence = 0.71 (above threshold) → resolved", 0.71, true),
			Entry("confidence = 0.85 (well above) → resolved", 0.85, true),
			Entry("confidence = 0.99 (near certainty) → resolved", 0.99, true),
			// BR-AI-050: Below threshold (<0.7) is terminal failure, even with workflow
			// These tests verify correct transition to Failed for low confidence scenarios
			Entry("confidence = 0.69 (below threshold) → Failed (BR-AI-050)", 0.69, false),
			Entry("confidence = 0.50 (low confidence) → Failed (BR-AI-050)", 0.50, false),
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

		// ========================================
		// ERROR CLASSIFICATION TESTS (BR-AI-009, BR-AI-010)
		// Tests for transient vs permanent error classification and retry behavior
		// See: pkg/aianalysis/handlers/error_classifier.go
		// ========================================
		Context("Error Classification - BR-AI-009 & BR-AI-010", func() {
			It("should classify 503 Service Unavailable as transient and retry", func() {
				analysis := createTestAnalysis()
				mockClient.WithError(&hgptclient.APIError{StatusCode: 503, Message: "Service Unavailable"})

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating), "503 errors are transient")
				Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Should retry with backoff")
				Expect(analysis.Status.ConsecutiveFailures).To(Equal(int32(1)), "Counter incremented")
			})

			It("should classify 429 Too Many Requests as transient and retry", func() {
				analysis := createTestAnalysis()
				mockClient.WithError(&hgptclient.APIError{StatusCode: 429, Message: "Too Many Requests"})

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating), "429 errors are transient")
				Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Should retry with backoff")
			})

			It("should classify 500 Internal Server Error as transient and retry", func() {
				analysis := createTestAnalysis()
				mockClient.WithError(&hgptclient.APIError{StatusCode: 500, Message: "Internal Server Error"})

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating), "500 errors are transient")
				Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Should retry with backoff")
			})

			// NOTE: Permanent error classification tests (401, 403, unknown) moved to integration tier
			// These require real infrastructure to validate the complete error handling and phase transition flow
			// See test/integration/aianalysis/ for comprehensive permanent error coverage
		})

		Context("Exponential Backoff Behavior - BR-AI-009", func() {
			It("should increase backoff duration with each retry attempt", func() {
				By("First attempt")
				analysis := createTestAnalysis()
				mockClient.WithError(&hgptclient.APIError{StatusCode: 503, Message: "Service Unavailable"})
				result1, _ := handler.Handle(ctx, analysis)
				backoff1 := result1.RequeueAfter

				By("Second attempt")
				mockClient.WithError(&hgptclient.APIError{StatusCode: 503, Message: "Service Unavailable"})
				result2, _ := handler.Handle(ctx, analysis)
				backoff2 := result2.RequeueAfter

				By("Third attempt")
				mockClient.WithError(&hgptclient.APIError{StatusCode: 503, Message: "Service Unavailable"})
				result3, _ := handler.Handle(ctx, analysis)
				backoff3 := result3.RequeueAfter

				Expect(backoff2).To(BeNumerically(">", backoff1), "Backoff increases on second attempt")
				Expect(backoff3).To(BeNumerically(">", backoff2), "Backoff increases on third attempt")
			})

			It("should reset ConsecutiveFailures to 0 on successful API call", func() {
				analysis := createTestAnalysis()
				analysis.Status.ConsecutiveFailures = 3

				// Simulate successful API call with complete response
				mockClient.WithFullResponse(
					"Analysis complete",
					0.85,
					[]string{},
					"Memory leak detected",    // rcaSummary
					"high",                    // rcaSeverity
					"restart-pod-v1",          // workflowID
					"kubernaut/restart:v1",    // executionBundle
					0.90,                      // workflowConfidence
					"Pod restart recommended", // workflowRationale
					false,                     // includeAlternatives
				)

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ConsecutiveFailures).To(Equal(int32(0)), "Counter reset on success")
			})
		})

		Context("transient error retry behavior", func() {
			// Business Value: Handler retries transient errors with exponential backoff
			It("should retry transient errors starting from ConsecutiveFailures=0", func() {
				analysis := createTestAnalysis()
				analysis.Status.ConsecutiveFailures = 0

				// Using transient error (503) to trigger retry path
				mockClient.WithError(&hgptclient.APIError{StatusCode: 503, Message: "Service Unavailable"})

				result, err := handler.Handle(ctx, analysis)

				// Business outcome: Transient errors trigger retry with backoff
				Expect(err).NotTo(HaveOccurred(), "No error returned, status updated")
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating), "Phase stays Investigating during retry")
				Expect(analysis.Status.ConsecutiveFailures).To(Equal(int32(1)), "Failure counter incremented")
				Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Backoff duration set for retry")
				Expect(analysis.Status.Message).To(ContainSubstring("Transient error"), "Status indicates transient error")
			})

			// Business Value: Handle initial state gracefully (ConsecutiveFailures not set)
			It("should handle ConsecutiveFailures=0 gracefully for first transient error", func() {
				analysis := createTestAnalysis()
				// ConsecutiveFailures defaults to 0

				mockClient.WithError(&hgptclient.APIError{StatusCode: 503, Message: "Service Unavailable"})

				result, err := handler.Handle(ctx, analysis)

				// Should retry (transient error classification)
				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating), "Phase stays Investigating")
				Expect(analysis.Status.ConsecutiveFailures).To(Equal(int32(1)), "Counter incremented from 0 to 1")
				Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Backoff set")
			})

			// Business Value: Correctly increments retry count on transient failures
			It("should increment retry count on subsequent transient errors", func() {
				analysis := createTestAnalysis()
				analysis.Status.ConsecutiveFailures = 2

				mockClient.WithError(&hgptclient.APIError{StatusCode: 503, Message: "Service Unavailable"})

				result, err := handler.Handle(ctx, analysis)

				// Should retry (not yet at max retries)
				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating), "Phase stays Investigating")
				Expect(analysis.Status.ConsecutiveFailures).To(Equal(int32(3)), "Counter incremented from 2 to 3")
				Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Backoff increases with attempt count")
				Expect(analysis.Status.Message).To(ContainSubstring("attempt 3/5"), "Status shows progress toward max retries")
			})
		})
	})
})
