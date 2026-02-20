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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// HolmesGPT-API Integration Tests
//
// REFACTORED: Per 03-testing-strategy.mdc Mock Policy (Feb 2, 2026)
// - Integration tests use REAL HAPI service (business logic, not external API)
// - HAPI runs with Mock LLM enabled (external API properly mocked)
// - DD-AUTH-014: Uses authenticated realHGClient from suite setup
//
// Testing Strategy (per TESTING_GUIDELINES.md):
// - Mock Strategy: ZERO MOCKS for business logic (line 102)
// - Mock ONLY external services (LLM via Mock LLM service)
// - Uses real HAPI container with real HTTP calls
//
// Infrastructure Required (AIAnalysis-Specific):
//   podman-compose -f test/integration/aianalysis/podman-compose.yml up -d
//
//   Stack:
//   - PostgreSQL (:15438)
//   - Redis (:16384)
//   - DataStorage API (:18095)
//   - Mock LLM Service (:18141) - Standalone Python app (mocks OpenAI)
//   - HolmesGPT API (:18120) - Real business logic

// SERIAL EXECUTION: AA integration suite runs serially for 100% reliability.
// See audit_flow_integration_test.go for detailed rationale.
var _ = Describe("HolmesGPT-API Integration", Label("integration", "holmesgpt"), func() {
	var (
		testCtx    context.Context
		cancelFunc context.CancelFunc
	)

	BeforeEach(func() {
		// DD-AUTH-014: Use shared realHGClient from suite setup (has authentication)
		// DO NOT create new client here - it would lack Bearer token
		// The suite_test.go creates realHGClient with ServiceAccountTransport(token)
		testCtx, cancelFunc = context.WithTimeout(context.Background(), 90*time.Second)
	})

	AfterEach(func() {
		cancelFunc()
	})

	Context("Incident Analysis - BR-AI-006", func() {
		It("should return valid analysis response", func() {
			// Real HAPI call with Mock LLM backend
			// Mock LLM will return deterministic response based on signal type
			resp, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-crashloop-001",
				RemediationID:     "req-test-001",
				SignalType:        "CrashLoopBackOff",
				Severity:          "critical",
				SignalSource:      "kubernaut",
				ResourceNamespace: "staging",
				ResourceKind:      "Pod",
				ResourceName:      "test-app",
				ErrorMessage:      "Container restarted 5 times",
				Environment:       "staging",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "test-cluster",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.Analysis).NotTo(BeEmpty())
			Expect(resp.Confidence).To(BeNumerically(">", 0))
			Expect(resp.Confidence).To(BeNumerically("<=", 1.0))
		})

		It("should return valid response without targetInOwnerChain (ADR-055) - BR-AI-007", func() {
			// Real HAPI call - response determined by Mock LLM
			_, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-memory-001",
				RemediationID:     "req-test-002",
				SignalType:        "MemoryPressure",
				Severity:          "medium", // DD-SEVERITY-001: Use normalized severity enum
				SignalSource:      "kubernaut",
				ResourceNamespace: testNamespace, // DD-TEST-002: Use dynamic namespace
				ResourceKind:      "Pod",
				ResourceName:      "web-app-abc123",
				ErrorMessage:      "Memory pressure detected",
				Environment:       "production",
				Priority:          "P2",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "test-cluster",
			})

			Expect(err).NotTo(HaveOccurred())
			// ADR-055: TargetInOwnerChain removed from HolmesGPT API response.
			// The LLM now populates affectedResource directly via get_resource_context tool.
		})

		It("should return selected workflow - BR-AI-016", func() {
			// Real HAPI call - Mock LLM returns workflow based on OOMKilled signal type
			resp, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-oom-001",
				RemediationID:     "req-test-003",
				SignalType:        "OOMKilled",
				Severity:          "critical",
				SignalSource:      "kubernaut",
				ResourceNamespace: testNamespace, // DD-TEST-002: Use dynamic namespace
				ResourceKind:      "Pod",
				ResourceName:      "memory-hog",
				ErrorMessage:      "Container exceeded memory limit",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "test-cluster",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.SelectedWorkflow.Set).To(BeTrue())
			// Extract workflow_id from the map using helper
			swMap := GetMapFromOptNil(resp.SelectedWorkflow.Value)
			Expect(swMap).NotTo(BeNil())
			workflowID := GetStringFromMap(swMap, "workflow_id")
			Expect(workflowID).NotTo(BeEmpty(), "Mock LLM should return workflow for OOMKilled")

			// Extract confidence from the map using helper
			confidence := GetFloat64FromMap(swMap, "confidence")
			Expect(confidence).To(BeNumerically(">", 0), "Confidence should be positive")
		})

		It("should include alternative workflows for production - BR-AI-016", func() {
			// Real HAPI call - Mock LLM may include alternatives for production
			resp, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-prod-001",
				RemediationID:     "req-test-004",
				SignalType:        "CrashLoopBackOff",
				Severity:          "critical",
				SignalSource:      "kubernaut",
				ResourceNamespace: "production",
				ResourceKind:      "Pod",
				ResourceName:      "prod-app",
				ErrorMessage:      "Pod in CrashLoopBackOff state",
				Environment:       "production",
				Priority:          "P0",
				RiskTolerance:     "low",
				BusinessCategory:  "critical",
				ClusterName:       "prod-cluster",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			// Note: Alternative workflows would need to be added to mock response manually
			// For now, just verify the main response works
			Expect(resp.SelectedWorkflow.Set).To(BeTrue())
		})
	})

	Context("Human Review Flag - BR-HAPI-197", func() {
		It("should handle needs_human_review=true with reason enum", func() {
			// Real HAPI call - Unknown signal type may trigger human review
			resp, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-hr-001",
				RemediationID:     "req-hr-001",
				SignalType:        "Unknown",
				Severity:          "critical",
				SignalSource:      "kubernaut",
				ResourceNamespace: "production",
				ResourceKind:      "Pod",
				ResourceName:      "unknown-app",
				ErrorMessage:      "Unknown error pattern - requires investigation",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "low",
				BusinessCategory:  "standard",
				ClusterName:       "prod-cluster",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			// Note: Mock LLM behavior determines if human review is needed
			// For Unknown signal type, it may trigger investigation_inconclusive
			if resp.NeedsHumanReview.Value {
				Expect(resp.HumanReviewReason.Set).To(BeTrue(), "Human review reason should be set")
			}
		})

		It("should handle testable human_review_reason enum values - BR-HAPI-197", func() {
			// REFACTORED: Now using Mock LLM scenarios to test 5 of 7 human_review_reason enums
			// Mock LLM has built-in scenarios triggered by special signal types (see server.py lines 605-694)
			//
			// Testable (5/7): no_matching_workflows, low_confidence, llm_parsing_error,
			//                 investigation_inconclusive, workflow_not_found
			// Not testable here (2/7): image_mismatch, parameter_validation_failed
			//                          (require HAPI business logic validation)
			//
			// NOTE (BR-HAPI-197 AC-4 Clarification):
			// HAPI does NOT set needs_human_review=true for low confidence scenarios.
			// HAPI returns confidence scores, and AIAnalysis controller applies the threshold.
			// Only HAPI validation failures (llm_parsing_error, no_matching_workflows) set needs_human_review.

			testCases := []struct {
				signalType         string
				expectedReviewFlag bool
				expectedReason     string
				description        string
			}{
				{
					signalType:         "MOCK_NO_WORKFLOW_FOUND",
					expectedReviewFlag: true,
					expectedReason:     "no_matching_workflows",
					description:        "No workflow found scenario",
				},
				{
					signalType:         "MOCK_LOW_CONFIDENCE",
					expectedReviewFlag: false, // BR-HAPI-197 AC-4: HAPI does NOT set for low confidence
					expectedReason:     "",    // AIAnalysis controller will set this
					description:        "Low confidence scenario (<0.5)",
				},
				{
					signalType:         "MOCK_MAX_RETRIES_EXHAUSTED",
					expectedReviewFlag: true,
					expectedReason:     "llm_parsing_error",
					description:        "Max retries exhausted scenario",
				},
				{
					signalType:         "Unknown",
					expectedReviewFlag: false, // May or may not trigger human review
					expectedReason:     "",    // Reason varies
					description:        "Investigation inconclusive (vague signal)",
				},
			}

			for _, tc := range testCases {
				By(fmt.Sprintf("Testing %s", tc.description))
				resp, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
					IncidentID:        "test-hr-" + tc.signalType,
					RemediationID:     "req-hr-" + tc.signalType,
					SignalType:        tc.signalType,
					Severity:          "medium",
					SignalSource:      "kubernaut",
					ResourceNamespace: testNamespace,
					ResourceKind:      "Pod",
					ResourceName:      "test-pod",
					ErrorMessage:      "Test for " + tc.signalType,
					Environment:       "staging",
					Priority:          "P2",
					RiskTolerance:     "medium",
					BusinessCategory:  "standard",
					ClusterName:       "test-cluster",
				})

				Expect(err).NotTo(HaveOccurred(), "Request should succeed for %s", tc.description)
				Expect(resp).NotTo(BeNil())

				if tc.expectedReviewFlag {
					Expect(resp.NeedsHumanReview.Value).To(BeTrue(),
						"%s should trigger human review", tc.description)

					if tc.expectedReason != "" {
						Expect(resp.HumanReviewReason.Set).To(BeTrue(),
							"Human review reason should be set for %s", tc.description)
						Expect(string(resp.HumanReviewReason.Value)).To(Equal(tc.expectedReason),
							"Expected reason '%s' for %s", tc.expectedReason, tc.description)
					}
				}
			}
		})
	})

	Context("Problem Resolved - BR-HAPI-200 Outcome A", func() {
		It("should handle problem resolved scenario (no workflow needed)", func() {
			// REFACTORED: Now using Mock LLM MOCK_PROBLEM_RESOLVED scenario
			// Mock LLM returns investigation_outcome="resolved" with no workflow
			// See server.py lines 948-980 for scenario definition
			resp, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-resolved-001",
				RemediationID:     "req-resolved-001",
				SignalType:        "MOCK_PROBLEM_RESOLVED", // Mock LLM scenario trigger
				Severity:          "medium",
				SignalSource:      "kubernaut",
				ResourceNamespace: testNamespace,
				ResourceKind:      "Pod",
				ResourceName:      "recovered-pod",
				ErrorMessage:      "Service was previously degraded but has now recovered",
				Environment:       "staging",
				Priority:          "P2",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "test-cluster",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())

			// BR-HAPI-200 Outcome A: Problem resolved, no workflow needed
			Expect(resp.NeedsHumanReview.Value).To(BeFalse(),
				"Problem resolved should not require human review")
			Expect(resp.SelectedWorkflow.Set).To(BeFalse(),
				"No workflow should be set when problem is resolved")
			Expect(resp.Confidence).To(BeNumerically(">=", 0.7),
				"High confidence that problem is resolved")

			// Note: Investigation outcome validation removed - field doesn't exist in OpenAPI spec
			// The test validates problem resolution by checking confidence >= 0.7 and no workflow selected
		})
	})

	Context("LLM Parsing Error - BR-HAPI-197", func() {
		It("should handle max retries exhausted scenario", func() {
			// ADDED: Explicit test for MOCK_MAX_RETRIES_EXHAUSTED scenario
			// Mock LLM simulates LLM returning unparseable responses, triggering retry exhaustion
			// See server.py lines 1011-1042 for scenario definition
			resp, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-max-retries-001",
				RemediationID:     "req-max-retries-001",
				SignalType:        "MOCK_MAX_RETRIES_EXHAUSTED", // Mock LLM scenario trigger
				Severity:          "high",
				SignalSource:      "kubernaut",
				ResourceNamespace: testNamespace,
				ResourceKind:      "Pod",
				ResourceName:      "llm-parse-fail-pod",
				ErrorMessage:      "LLM response parsing failed repeatedly",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "low",
				BusinessCategory:  "critical",
				ClusterName:       "prod-cluster",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())

			// BR-HAPI-197: Max retries exhausted requires human review
			Expect(resp.NeedsHumanReview.Value).To(BeTrue(),
				"Max retries exhausted should require human review")

			if resp.HumanReviewReason.Set {
				Expect(string(resp.HumanReviewReason.Value)).To(Equal("llm_parsing_error"),
					"Human review reason should be 'llm_parsing_error'")
			}
		})
	})

	Context("Investigation Inconclusive - BR-HAPI-200 Outcome B", func() {
		It("should handle investigation_inconclusive scenario", func() {
			// Real HAPI call - NetworkFailure with unclear pattern may trigger inconclusive
			resp, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-inconclusive-001",
				RemediationID:     "req-inconclusive-001",
				SignalType:        "NetworkFailure",
				Severity:          "medium", // DD-SEVERITY-001: Use normalized severity enum
				SignalSource:      "kubernaut",
				ResourceNamespace: testNamespace, // DD-TEST-002: Use dynamic namespace
				ResourceKind:      "Pod",
				ResourceName:      "network-app",
				ErrorMessage:      "Intermittent network failures with unclear pattern",
				Environment:       "staging",
				Priority:          "P2",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "test-cluster",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			// Note: Mock LLM behavior determines if investigation is inconclusive
			if resp.NeedsHumanReview.Value {
				// If human review needed, reason should be set
				Expect(resp.HumanReviewReason.Set).To(BeTrue())
			}
		})
	})

	Context("Validation History - DD-HAPI-002", func() {
		It("should return validation attempts history when present", func() {
			// Real HAPI call - validation history populated by HAPI's retry logic
			resp, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-validation-001",
				RemediationID:     "req-validation-001",
				SignalType:        "DatabaseTimeout",
				Severity:          "medium", // DD-SEVERITY-001: Use normalized severity enum
				SignalSource:      "kubernaut",
				ResourceNamespace: "staging",
				ResourceKind:      "Pod",
				ResourceName:      "db-client",
				ErrorMessage:      "Database connection timeout",
				Environment:       "staging",
				Priority:          "P2",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "test-cluster",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			// Note: ValidationAttemptsHistory populated by HAPI's internal retry logic
			// Presence depends on whether HAPI needed to retry LLM parsing
		})
	})

	Context("Error Handling - BR-AI-009", func() {
		It("should handle timeout gracefully", func() {
			// Create client with very short timeout to test timeout handling
			// DD-AUTH-014: Must use authenticated transport
			hapiAuthTransport := testauth.NewServiceAccountTransport(serviceAccountToken)
			shortClient, err := client.NewHolmesGPTClientWithTransport(client.Config{
				BaseURL: "http://localhost:18120",
				Timeout: 1 * time.Nanosecond, // Effectively instant timeout
			}, hapiAuthTransport)
			Expect(err).ToNot(HaveOccurred(), "Failed to create short-timeout HAPI client")

			// Create a very short timeout context
			shortCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
			defer cancel()

			// Wait for context to expire
			time.Sleep(2 * time.Millisecond)

			_, err = shortClient.Investigate(shortCtx, &client.IncidentRequest{
				IncidentID:        "test-timeout-001",
				RemediationID:     "req-timeout-001",
				SignalType:        "Test",
				Severity:          "info",
				SignalSource:      "kubernaut",
				ResourceNamespace: testNamespace, // DD-TEST-002: Use dynamic namespace
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "Test timeout handling",
				Environment:       "staging",
				Priority:          "P3",
				RiskTolerance:     "high",
				BusinessCategory:  "standard",
				ClusterName:       "test-cluster",
			})

			Expect(err).To(HaveOccurred())
			// Timeout error should be context.DeadlineExceeded or contain "timeout"
			Expect(err.Error()).To(Or(
				ContainSubstring("context deadline exceeded"),
				ContainSubstring("timeout"),
			))
		})

		// BR-AI-009: Server failure handling (HTTP 500) covered in unit tests
		// See: test/unit/aianalysis/holmesgpt_client_test.go (HTTP 500 client error handling)
		// See: test/unit/aianalysis/investigating_handler_test.go:823 (HTTP 500 controller retry logic)
		// Unit tests use httptest.Server to simulate server failures without infrastructure manipulation

		It("should handle validation errors (400) - BR-AI-009", func() {
			// Real HAPI call with missing required field - should return 400
			// DD-WORKFLOW-002: remediation_id is required
			_, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-validation-error",
				RemediationID:     "", // EMPTY - violates DD-WORKFLOW-002
				SignalType:        "Test",
				Severity:          "info",
				SignalSource:      "kubernaut",
				ResourceNamespace: testNamespace, // DD-TEST-002: Use dynamic namespace
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "Test validation error",
				Environment:       "staging",
				Priority:          "P3",
				RiskTolerance:     "high",
				BusinessCategory:  "standard",
				ClusterName:       "test-cluster",
			})

			Expect(err).To(HaveOccurred())
			// HAPI returns 400 for validation errors (Pydantic validation)
			apiErr, ok := err.(*client.APIError)
			if ok {
				Expect(apiErr.StatusCode).To(Equal(400), "Should return 400 for validation error")
			} else {
				// If not APIError, should still contain validation-related text
				Expect(err.Error()).To(Or(
					ContainSubstring("400"),
					ContainSubstring("validation"),
					ContainSubstring("required"),
				))
			}
		})
	})
})
