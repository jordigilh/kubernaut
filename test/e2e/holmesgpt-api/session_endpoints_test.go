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

package holmesgptapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	hapiclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// Session-Based Endpoint E2E Tests (BR-AA-HAPI-064)
// Test Plan: docs/testing/BR-AA-HAPI-064/session_based_pull_test_plan_v1.0.md
// Scenarios: E2E-HAPI-064-001 through E2E-HAPI-064-012
// Business Requirements: BR-AA-HAPI-064.1 through .9
//
// Purpose: Validate all HAPI session REST API endpoints with realistic business outcomes.
// These tests exercise the async submit/poll/result pattern using raw session endpoints.
//
// REST API Endpoints under test:
//   POST /api/v1/incident/analyze          → 202 + session_id
//   GET  /api/v1/incident/session/{id}     → session status
//   GET  /api/v1/incident/session/{id}/result → incident result

var _ = Describe("E2E-HAPI-064: Session-Based Endpoints", Label("e2e", "hapi", "session", "aa-064"), func() {

	// sessionClient is initialized in SynchronizedBeforeSuite (suite_test.go)
	// It provides async submit/poll/result methods for session-based HAPI endpoints.

	// =====================================================================
	// INCIDENT SESSION ENDPOINTS
	// =====================================================================

	Context("Incident Session: Happy path scenarios", func() {

		It("E2E-HAPI-064-001: Incident submit/poll/result for CrashLoopBackOff", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-064-001
			// Business Outcome: Standard CrashLoopBackOff signal produces confident recommendation via async session
			// BR: BR-AA-HAPI-064.1, .2, .3
			// Endpoints: POST /incident/analyze, GET /incident/session/{id}, GET /incident/session/{id}/result

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-session-001",
				RemediationID:     "test-rem-session-001",
				SignalName:        "CrashLoopBackOff",
				Severity:          "high",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "session-pod-001",
				ErrorMessage:      "Container restarting repeatedly",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT STEP 1: Submit investigation (202 Accepted)
			// ========================================
			By("Submitting incident investigation via session endpoint")
			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "POST /incident/analyze should return 202 with session_id")
			Expect(sessionID).ToNot(BeEmpty(), "session_id must be non-empty")

			// ========================================
			// ACT STEP 2: Poll session until completed
			// ========================================
			By("Polling session status until investigation completes")
			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				return status.Status
			}, 30*time.Second, 1*time.Second).Should(Equal("completed"),
				"Session should reach 'completed' status within 30s")

			// ========================================
			// ACT STEP 3: Retrieve result
			// ========================================
			By("Retrieving incident investigation result")
			result, err := sessionClient.GetSessionResult(ctx, sessionID)
			Expect(err).ToNot(HaveOccurred(), "GET /incident/session/{id}/result should succeed")

			// ========================================
			// ASSERT: Business outcome validation
			// ========================================
			// BEHAVIOR: Confident recommendation provided
			Expect(result.IncidentID).To(Equal("test-session-001"),
				"incident_id must match request")
			Expect(result.NeedsHumanReview.Value).To(BeFalse(),
				"needs_human_review must be false for confident CrashLoopBackOff recommendation")
			Expect(result.Confidence).To(BeNumerically("~", 0.88, 0.05),
				"Mock LLM 'crashloop' scenario returns confidence = 0.88 ± 0.05")
			Expect(len(result.Analysis) > 0).To(BeTrue(),
				"analysis field must be present")

			// BUSINESS IMPACT: AIAnalysis creates WorkflowExecution automatically
		})

		It("E2E-HAPI-064-002: Incident submit/poll/result for OOMKilled", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-064-002
			// Business Outcome: OOMKilled signal type produces confident workflow recommendation via async session
			// BR: BR-AA-HAPI-064.1, .2, .3

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-session-002",
				RemediationID:     "test-rem-session-002",
				SignalName:        "OOMKilled",
				Severity:          "high",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "session-pod-002",
				ErrorMessage:      "Container memory limit exceeded",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT: Full session lifecycle
			// ========================================
			By("Submitting OOMKilled incident via session endpoint")
			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(sessionID)).To(BeNumerically(">", 0),
				"SubmitInvestigation should return a non-empty session ID")

			By("Polling session status until completed")
			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				return status.Status
			}, 30*time.Second, 1*time.Second).Should(Equal("completed"))

			By("Retrieving result")
			result, err := sessionClient.GetSessionResult(ctx, sessionID)
			Expect(err).ToNot(HaveOccurred())

			// ========================================
			// ASSERT
			// ========================================
			Expect(result.NeedsHumanReview.Value).To(BeFalse(),
				"needs_human_review must be false for confident OOMKilled recommendation")
			Expect(result.Confidence).To(BeNumerically("~", 0.88, 0.05),
				"Mock LLM 'oomkilled' scenario returns confidence = 0.88 ± 0.05")

			// BUSINESS IMPACT: Memory-related workflows auto-selected and executed
		})
	})

	Context("Incident Session: Human review scenarios", func() {

		It("E2E-HAPI-064-003: No workflow found via session", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-064-003
			// Business Outcome: MOCK_NO_WORKFLOW_FOUND escalates to human review via session flow
			// BR: BR-AA-HAPI-064.1, BR-HAPI-197

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-session-003",
				RemediationID:     "test-rem-session-003",
				SignalName:        "MOCK_NO_WORKFLOW_FOUND",
				Severity:          "high",
				SignalSource:      "prometheus",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "session-pod-003",
				ErrorMessage:      "No automation available",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT
			// ========================================
			By("Submitting investigation for signal with no matching workflow")
			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			By("Polling until session completes")
			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				return status.Status
			}, 30*time.Second, 1*time.Second).Should(Equal("completed"))

			By("Retrieving result")
			result, err := sessionClient.GetSessionResult(ctx, sessionID)
			Expect(err).ToNot(HaveOccurred())

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Human review required
			Expect(result.NeedsHumanReview.Value).To(BeTrue(),
				"needs_human_review must be true when no workflow found")
			Expect(result.HumanReviewReason.Value).To(Equal(hapiclient.HumanReviewReasonNoMatchingWorkflows),
				"human_review_reason must indicate no matching workflows")
			Expect(result.SelectedWorkflow.Value).To(BeNil(),
				"selected_workflow must be nil when no workflow found")

			// CORRECTNESS: Zero confidence
			Expect(result.Confidence).To(BeNumerically("==", 0.0),
				"confidence must be 0.0 when no automation possible")

			// BUSINESS IMPACT: Operator notified for manual intervention
		})

		It("E2E-HAPI-064-004: Low confidence via session", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-064-004
			// Business Outcome: MOCK_LOW_CONFIDENCE returns low-confidence recommendation for AA to evaluate
			// BR: BR-AA-HAPI-064.1, BR-HAPI-197

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-session-004",
				RemediationID:     "test-rem-session-004",
				SignalName:        "MOCK_LOW_CONFIDENCE",
				Severity:          "high",
				SignalSource:      "prometheus",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "session-pod-004",
				ErrorMessage:      "Uncertain root cause",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT
			// ========================================
			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				return status.Status
			}, 30*time.Second, 1*time.Second).Should(Equal("completed"))

			result, err := sessionClient.GetSessionResult(ctx, sessionID)
			Expect(err).ToNot(HaveOccurred())

			// ========================================
			// ASSERT
			// ========================================
			// BR-HAPI-197 + BR-HAPI-198: HAPI returns confidence but does NOT enforce thresholds
			// AIAnalysis owns the threshold logic
			Expect(result.NeedsHumanReview.Value).To(BeFalse(),
				"HAPI should NOT set needs_human_review based on confidence thresholds (BR-HAPI-197)")
			Expect(result.SelectedWorkflow.Set).To(BeTrue(),
				"selected_workflow must be present even with low confidence")
			Expect(result.Confidence).To(BeNumerically("<", 0.5),
				"confidence < 0.5 signals low confidence to AIAnalysis for threshold evaluation")
			Expect(result.AlternativeWorkflows).ToNot(BeEmpty(),
				"alternative_workflows help AIAnalysis when confidence is low")

			// BUSINESS IMPACT: AIAnalysis applies 70% threshold, sees <0.70, sets needs_human_review=true
		})

		It("E2E-HAPI-064-005: Max retries exhausted via session", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-064-005
			// Business Outcome: MOCK_MAX_RETRIES_EXHAUSTED returns complete validation history for debugging
			// BR: BR-AA-HAPI-064.1, BR-HAPI-197

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-session-005",
				RemediationID:     "test-rem-session-005",
				SignalName:        "MOCK_MAX_RETRIES_EXHAUSTED",
				Severity:          "high",
				SignalSource:      "prometheus",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "session-pod-005",
				ErrorMessage:      "Validation failed",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT
			// ========================================
			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				return status.Status
			}, 30*time.Second, 1*time.Second).Should(Equal("completed"))

			result, err := sessionClient.GetSessionResult(ctx, sessionID)
			Expect(err).ToNot(HaveOccurred())

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: AI gave up after max retries
			Expect(result.NeedsHumanReview.Value).To(BeTrue(),
				"needs_human_review must be true when max retries exhausted")
			Expect(result.HumanReviewReason.Value).To(Equal(hapiclient.HumanReviewReasonLlmParsingError),
				"human_review_reason must indicate LLM parsing error")
			Expect(result.SelectedWorkflow.Set).To(BeFalse(),
				"selected_workflow must be null when parsing failed")

			// CORRECTNESS: Complete validation history for debugging
			Expect(result.ValidationAttemptsHistory).ToNot(BeEmpty(),
				"validation_attempts_history must be present for debugging")
			Expect(len(result.ValidationAttemptsHistory)).To(Equal(3),
				"MOCK_MAX_RETRIES_EXHAUSTED triggers exactly 3 validation attempts")

			for i, attempt := range result.ValidationAttemptsHistory {
				Expect(attempt.Attempt).To(Equal(i + 1), "attempt number must be sequential")
				Expect(attempt.IsValid).To(BeFalse(), "is_valid must be false for failed validation")
				Expect(attempt.Errors).ToNot(BeEmpty(), "errors must be present for failed validation")
				Expect(attempt.Timestamp).ToNot(BeEmpty(), "timestamp must be present")
			}

			// BUSINESS IMPACT: Operator sees why AI failed, can debug or manually intervene
		})
	})

	Context("Incident Session: Status transitions", func() {

		It("E2E-HAPI-064-006: Session status transitions observable during investigation", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-064-006
			// Business Outcome: Session status is queryable and reaches terminal state
			// BR: BR-AA-HAPI-064.2

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-session-006",
				RemediationID:     "test-rem-session-006",
				SignalName:        "CrashLoopBackOff",
				Severity:          "medium",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "session-pod-006",
				ErrorMessage:      "Container restarting",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT
			// ========================================
			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// Poll and collect observed statuses
			observedStatuses := map[string]bool{}
			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				observedStatuses[status.Status] = true
				return status.Status
			}, 30*time.Second, 200*time.Millisecond).Should(Equal("completed"))

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Session reaches terminal "completed" state
			Expect(observedStatuses).To(HaveKey("completed"),
				"'completed' status must be observed")

			// Note: Due to Mock LLM speed, intermediate states (pending, investigating)
			// may or may not be observed. The key contract is that the session reaches
			// a terminal state. In production with real LLM latency, intermediate
			// states would be observable.

			// BUSINESS IMPACT: AA controller can reliably poll and detect completion
		})
	})

	// =====================================================================
	// SESSION ERROR HANDLING
	// =====================================================================

	Context("Session Error Handling", func() {

		It("E2E-HAPI-064-011: Poll non-existent session returns error", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-064-011
			// Business Outcome: Invalid session IDs return clear errors (404)
			// BR: BR-AA-HAPI-064.2, BR-AA-HAPI-064.5

			// ========================================
			// ARRANGE: Use a fabricated UUID that doesn't exist
			// ========================================
			fakeSessionID := uuid.New().String()

			// ========================================
			// ACT
			// ========================================
			_, err := sessionClient.PollSession(ctx, fakeSessionID)

			// ========================================
			// ASSERT
			// ========================================
			Expect(err).To(HaveOccurred(),
				"Polling a non-existent session must return an error")

			// CORRECTNESS: Error should indicate session not found
			apiErr, ok := err.(*hapiclient.APIError)
			Expect(ok).To(BeTrue(), "Error should be an APIError type")
			Expect(apiErr.StatusCode).To(Equal(http.StatusNotFound),
				"Non-existent session should return HTTP 404")

			// BUSINESS IMPACT: AA controller detects session loss and triggers regeneration (BR-AA-HAPI-064.5)
		})

		It("E2E-HAPI-064-012: Result for non-existent session returns error", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-064-012
			// Business Outcome: Result retrieval for invalid sessions returns clear errors
			// BR: BR-AA-HAPI-064.3

			// ========================================
			// ARRANGE: Use a fabricated UUID that doesn't exist
			// ========================================
			fakeSessionID := uuid.New().String()

			// ========================================
			// ACT
			// ========================================
			_, err := sessionClient.GetSessionResult(ctx, fakeSessionID)

			// ========================================
			// ASSERT
			// ========================================
			Expect(err).To(HaveOccurred(),
				"Getting result for a non-existent session must return an error")

			apiErr, ok := err.(*hapiclient.APIError)
			Expect(ok).To(BeTrue(), "Error should be an APIError type")
			Expect(apiErr.StatusCode).To(Equal(http.StatusNotFound),
				"Non-existent session result should return HTTP 404")

			// BUSINESS IMPACT: Clear error handling prevents AA controller from hanging
		})
	})
})
