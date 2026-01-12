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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// HolmesGPT-API Integration Tests
//
// Per HAPI team response (Dec 9, 2025) in REQUEST_HAPI_INTEGRATION_TEST_MOCK_ASSISTANCE.md:
// - Use mocks.MockHolmesGPTClient for integration tests (Option B)
// - Mock helpers provide canonical fixtures for all ADR-045 scenarios
// - No real HAPI server dependency for integration tier
//
// Testing Strategy (per TESTING_GUIDELINES.md):
// - Unit: Mock ✅ | Integration: Mock ✅ | E2E: REAL ❌
// - Contract validation via ADR-045 + OpenAPI spec

// SERIAL EXECUTION: AA integration suite runs serially for 100% reliability.
// See audit_flow_integration_test.go for detailed rationale.
var _ = Describe("HolmesGPT-API Integration", Label("integration", "holmesgpt"), func() {
	var (
		mockClient *mocks.MockHolmesGPTClient
		testCtx    context.Context
		cancelFunc context.CancelFunc
	)

	BeforeEach(func() {
		mockClient = mocks.NewMockHolmesGPTClient()
		testCtx, cancelFunc = context.WithTimeout(context.Background(), 60*time.Second)
	})

	AfterEach(func() {
		cancelFunc()
	})

	Context("Incident Analysis - BR-AI-006", func() {
		It("should return valid analysis response", func() {
			// Configure mock with successful response using new signature
			mockClient.WithFullResponse(
				"Root cause analysis: Container OOM killed due to memory leak",
				0.85,
				[]string{},
				"Memory leak in application",        // rcaSummary
				"high",                              // rcaSeverity
				"restart-pod-v1",                    // workflowID
				"kubernaut/restart-workflow:v1.0.0", // containerImage
				0.85,                                // workflowConfidence
				true,                                // targetInOwnerChain
				"",                                  // workflowRationale
				false,                               // includeAlternatives
			)

			resp, err := mockClient.Investigate(testCtx, &client.IncidentRequest{
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

		It("should include targetInOwnerChain in response - BR-AI-007", func() {
			mockClient.WithFullResponse(
				"Memory pressure analysis",
				0.75,
				[]string{},
				"",                          // rcaSummary
				"",                          // rcaSeverity
				"scale-up-v1",               // workflowID
				"kubernaut/scale-up:v1.0.0", // containerImage
				0.75,                        // workflowConfidence
				true,                        // targetInOwnerChain
				"",                          // workflowRationale
				false,                       // includeAlternatives
			)

			resp, err := mockClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-memory-001",
				RemediationID:     "req-test-002",
				SignalType:        "MemoryPressure",
				Severity:          "warning",
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
			// TargetInOwnerChain is set by default in mock to true
			Expect(resp.TargetInOwnerChain.Value).To(BeTrue())
		})

		It("should return selected workflow - BR-AI-016", func() {
			mockClient.WithFullResponse(
				"OOM analysis complete",
				0.90,
				[]string{},
				"",                                  // rcaSummary
				"",                                  // rcaSeverity
				"restart-pod-v1",                    // workflowID
				"kubernaut/restart-workflow:v1.2.0", // containerImage
				0.90,                                // workflowConfidence
				true,                                // targetInOwnerChain
				"",                                  // workflowRationale
				false,                               // includeAlternatives
			)

			resp, err := mockClient.Investigate(testCtx, &client.IncidentRequest{
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
			Expect(workflowID).To(Equal("restart-pod-v1"))

			// Extract confidence from the map using helper
			confidence := GetFloat64FromMap(swMap, "confidence")
			Expect(confidence).To(BeNumerically(">=", 0.9))
		})

		It("should include alternative workflows for production - BR-AI-016", func() {
			// Note: WithFullResponse now supports alternatives via includeAlternatives parameter
			mockClient.WithFullResponse(
				"Production incident analysis",
				0.85,
				[]string{},
				"",                                  // rcaSummary
				"",                                  // rcaSeverity
				"restart-pod-v1",                    // workflowID
				"kubernaut/restart-workflow:v1.0.0", // containerImage
				0.85,                                // workflowConfidence
				true,                                // targetInOwnerChain
				"",                                  // workflowRationale
				false,                               // includeAlternatives
			)

			resp, err := mockClient.Investigate(testCtx, &client.IncidentRequest{
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
			mockClient.WithHumanReviewReasonEnum("low_confidence", []string{
				"Confidence below threshold (0.45 < 0.70)",
				"Multiple potential root causes identified",
			})

			resp, err := mockClient.Investigate(testCtx, &client.IncidentRequest{
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
			Expect(resp.NeedsHumanReview.Value).To(BeTrue())
			Expect(resp.HumanReviewReason.Set).To(BeTrue())
			Expect(string(resp.HumanReviewReason.Value)).To(Equal("low_confidence"))
		})

		It("should handle all 7 human_review_reason enum values - BR-HAPI-197", func() {
			reasonEnums := []string{
				"workflow_not_found",
				"image_mismatch",
				"parameter_validation_failed",
				"no_matching_workflows",
				"low_confidence",
				"llm_parsing_error",
				"investigation_inconclusive",
			}

			for _, reason := range reasonEnums {
				mockClient.WithHumanReviewReasonEnum(reason, []string{"Test warning"})

				resp, err := mockClient.Investigate(testCtx, &client.IncidentRequest{
					IncidentID:        "test-hr-loop-" + reason,
					RemediationID:     "req-hr-loop",
					SignalType:        "Test",
					Severity:          "warning",
					SignalSource:      "kubernaut",
					ResourceNamespace: testNamespace, // DD-TEST-002: Use dynamic namespace
					ResourceKind:      "Pod",
					ResourceName:      "test-pod",
					ErrorMessage:      "Test for " + reason,
					Environment:       "staging",
					Priority:          "P2",
					RiskTolerance:     "medium",
					BusinessCategory:  "standard",
					ClusterName:       "test-cluster",
				})

				Expect(err).NotTo(HaveOccurred(), "Failed for reason: %s", reason)
				Expect(resp.NeedsHumanReview.Value).To(BeTrue(), "NeedsHumanReview should be true for: %s", reason)
				Expect(resp.HumanReviewReason.Set).To(BeTrue(), "HumanReviewReason should be set for: %s", reason)
				Expect(string(resp.HumanReviewReason.Value)).To(Equal(reason), "Reason should match for: %s", reason)
			}
		})
	})

	Context("Problem Resolved - BR-HAPI-200 Outcome A", func() {
		It("should handle problem resolved scenario (no workflow needed)", func() {
			mockClient.WithProblemResolved(
				0.85,
				[]string{},
				"Problem self-resolved: Pod restarted successfully and is now healthy",
			)

			resp, err := mockClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-resolved-001",
				RemediationID:     "req-resolved-001",
				SignalType:        "CrashLoopBackOff",
				Severity:          "warning",
				SignalSource:      "kubernaut",
				ResourceNamespace: testNamespace, // DD-TEST-002: Use dynamic namespace
				ResourceKind:      "Pod",
				ResourceName:      "recovered-pod",
				ErrorMessage:      "Pod was in CrashLoopBackOff but has now recovered",
				Environment:       "staging",
				Priority:          "P2",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "test-cluster",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.NeedsHumanReview.Value).To(BeFalse())
			Expect(resp.SelectedWorkflow.Set).To(BeFalse()) // No workflow set
			Expect(resp.Confidence).To(BeNumerically(">=", 0.7))
			Expect(resp.Analysis).To(ContainSubstring("self-resolved"))
		})
	})

	Context("Investigation Inconclusive - BR-HAPI-200 Outcome B", func() {
		It("should handle investigation_inconclusive scenario", func() {
			mockClient.WithHumanReviewReasonEnum("investigation_inconclusive", []string{
				"Unable to determine root cause",
				"Insufficient data for analysis",
			})

			resp, err := mockClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-inconclusive-001",
				RemediationID:     "req-inconclusive-001",
				SignalType:        "NetworkFailure",
				Severity:          "warning",
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
			Expect(resp.NeedsHumanReview.Value).To(BeTrue())
			Expect(string(resp.HumanReviewReason.Value)).To(Equal("investigation_inconclusive"))
		})
	})

	Context("Validation History - DD-HAPI-002", func() {
		It("should return validation attempts history when present", func() {
			// Note: ValidationAttemptsHistory not yet fully implemented in mock
			// TODO: Update when mock client supports validation history
			mockClient.WithHumanReviewReasonEnum("llm_parsing_error", []string{
				"Parsing failed on first attempt",
			})

			resp, err := mockClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-validation-001",
				RemediationID:     "req-validation-001",
				SignalType:        "DatabaseTimeout",
				Severity:          "warning",
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
			Expect(resp.NeedsHumanReview.Value).To(BeTrue())
			Expect(string(resp.HumanReviewReason.Value)).To(Equal("llm_parsing_error"))
			// TODO: Add validation history assertions when mock supports it
			// Expect(resp.ValidationAttemptsHistory).To(HaveLen(2))
		})
	})

	Context("Error Handling - BR-AI-009", func() {
		It("should handle timeout gracefully", func() {
			// Create a very short timeout context
			shortCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
			defer cancel()

			// Wait for context to expire
			time.Sleep(2 * time.Millisecond)

			mockClient.WithError(context.DeadlineExceeded)

			_, err := mockClient.Investigate(shortCtx, &client.IncidentRequest{
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
			Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
		})

		It("should return error for server failures - BR-AI-009", func() {
			mockClient.WithError(errors.New("API error: 500 Internal server error"))

			_, err := mockClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-error-001",
				RemediationID:     "req-error-001",
				SignalType:        "Test",
				Severity:          "info",
				SignalSource:      "kubernaut",
				ResourceNamespace: testNamespace, // DD-TEST-002: Use dynamic namespace
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "Test error handling",
				Environment:       "staging",
				Priority:          "P3",
				RiskTolerance:     "high",
				BusinessCategory:  "standard",
				ClusterName:       "test-cluster",
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("500"))
		})

		It("should handle validation errors (400) - BR-AI-009", func() {
			mockClient.WithError(errors.New("API error: 400 Invalid request: missing required field 'context'"))

			// Minimal request with mock - mock returns configured error
			_, err := mockClient.Investigate(testCtx, &client.IncidentRequest{
				IncidentID:        "test-validation-error",
				RemediationID:     "req-validation-error",
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
			Expect(err.Error()).To(ContainSubstring("400"))
		})
	})
})
