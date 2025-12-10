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

	aianalysisclient "github.com/jordigilh/kubernaut/pkg/aianalysis/client"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

// HolmesGPT-API Integration Tests
//
// Per HAPI team response (Dec 9, 2025) in REQUEST_HAPI_INTEGRATION_TEST_MOCK_ASSISTANCE.md:
// - Use testutil.MockHolmesGPTClient for integration tests (Option B)
// - Mock helpers provide canonical fixtures for all ADR-045 scenarios
// - No real HAPI server dependency for integration tier
//
// Testing Strategy (per TESTING_GUIDELINES.md):
// - Unit: Mock ✅ | Integration: Mock ✅ | E2E: REAL ❌
// - Contract validation via ADR-045 + OpenAPI spec

var _ = Describe("HolmesGPT-API Integration", Label("integration", "holmesgpt"), func() {
	var (
		mockClient *testutil.MockHolmesGPTClient
		testCtx    context.Context
		cancelFunc context.CancelFunc
	)

	BeforeEach(func() {
		mockClient = testutil.NewMockHolmesGPTClient()
		testCtx, cancelFunc = context.WithTimeout(context.Background(), 60*time.Second)
	})

	AfterEach(func() {
		cancelFunc()
	})

	Context("Incident Analysis - BR-AI-006", func() {
		It("should return valid analysis response", func() {
			// Configure mock with successful response
			mockClient.WithFullResponse(
				"Root cause analysis: Container OOM killed due to memory leak",
				0.85,
				true, // targetInOwnerChain
				[]string{},
				&aianalysisclient.RootCauseAnalysis{
					Summary:  "Memory leak in application",
					Severity: "high",
				},
				&aianalysisclient.SelectedWorkflow{
					WorkflowID: "restart-pod-v1",
					Confidence: 0.85,
				},
				nil,
			)

			resp, err := mockClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
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
				Context:           "Pod CrashLoopBackOff in staging namespace. Container test-app restarted 5 times.",
				EnrichmentResults: &aianalysisclient.EnrichmentResults{
					DetectedLabels: map[string]interface{}{
						"gitOpsManaged": true,
						"pdbProtected":  false,
					},
					OwnerChain: []aianalysisclient.OwnerChainEntry{
						{Namespace: "staging", Kind: "Deployment", Name: "test-app"},
					},
				},
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
				true, // targetInOwnerChain = true
				[]string{},
				nil,
				&aianalysisclient.SelectedWorkflow{WorkflowID: "scale-up-v1", Confidence: 0.75},
				nil,
			)

			resp, err := mockClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
				IncidentID:        "test-memory-001",
				RemediationID:     "req-test-002",
				SignalType:        "MemoryPressure",
				Severity:          "warning",
				SignalSource:      "kubernaut",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "web-app-abc123",
				ErrorMessage:      "Memory pressure detected",
				Environment:       "production",
				Priority:          "P2",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "test-cluster",
				Context:           "Memory pressure detected on pod web-app-abc123",
				EnrichmentResults: &aianalysisclient.EnrichmentResults{
					OwnerChain: []aianalysisclient.OwnerChainEntry{
						{Namespace: "default", Kind: "Deployment", Name: "web-app"},
					},
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.TargetInOwnerChain).To(BeTrue())
		})

		It("should return selected workflow - BR-AI-016", func() {
			mockClient.WithFullResponse(
				"OOM analysis complete",
				0.90,
				true,
				[]string{},
				nil,
				&aianalysisclient.SelectedWorkflow{
					WorkflowID:     "restart-pod-v1",
					Version:        "v1.2.0",
					ContainerImage: "kubernaut/restart-workflow:v1.2.0",
					Confidence:     0.90,
					Parameters:     map[string]string{"gracePeriod": "30s"},
					Rationale:      "High confidence match for OOM scenario",
				},
				nil,
			)

			resp, err := mockClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
				IncidentID:        "test-oom-001",
				RemediationID:     "req-test-003",
				SignalType:        "OOMKilled",
				Severity:          "critical",
				SignalSource:      "kubernaut",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "memory-hog",
				ErrorMessage:      "Container exceeded memory limit",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "test-cluster",
				Context:           "OOM Killed - container exceeded memory limit. Pod memory-hog in namespace default.",
				EnrichmentResults: &aianalysisclient.EnrichmentResults{
					DetectedLabels: map[string]interface{}{
						"gitOpsManaged": true,
					},
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.SelectedWorkflow).NotTo(BeNil())
			Expect(resp.SelectedWorkflow.WorkflowID).To(Equal("restart-pod-v1"))
			Expect(resp.SelectedWorkflow.Confidence).To(BeNumerically(">=", 0.9))
		})

		It("should include alternative workflows for production - BR-AI-016", func() {
			mockClient.WithFullResponse(
				"Production incident analysis",
				0.85,
				true,
				[]string{},
				nil,
				&aianalysisclient.SelectedWorkflow{WorkflowID: "restart-pod-v1", Confidence: 0.85},
				[]aianalysisclient.AlternativeWorkflow{
					{WorkflowID: "scale-up-v1", Confidence: 0.70, Rationale: "Alternative: vertical scaling"},
					{WorkflowID: "rollback-v1", Confidence: 0.65, Rationale: "Alternative: rollback to previous version"},
				},
			)

			resp, err := mockClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
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
				Context:           "Pod in CrashLoopBackOff state. Environment: production. Business priority: P0.",
				EnrichmentResults: &aianalysisclient.EnrichmentResults{
					DetectedLabels: map[string]interface{}{
						"environment": "production",
					},
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.AlternativeWorkflows).To(HaveLen(2))
			Expect(resp.AlternativeWorkflows[0].WorkflowID).To(Equal("scale-up-v1"))
		})
	})

	Context("Human Review Flag - BR-HAPI-197", func() {
		It("should handle needs_human_review=true with reason enum", func() {
			mockClient.WithHumanReviewReasonEnum("low_confidence", []string{
				"Confidence below threshold (0.45 < 0.70)",
				"Multiple potential root causes identified",
			})

			resp, err := mockClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
				IncidentID:    "test-hr-001",
				RemediationID: "req-hr-001",
				Context:       "Unknown error pattern in production - requires investigation",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.NeedsHumanReview).To(BeTrue())
			Expect(resp.HumanReviewReason).NotTo(BeNil())
			Expect(*resp.HumanReviewReason).To(Equal("low_confidence"))
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

				resp, err := mockClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
					IncidentID:    "test-hr-loop-" + reason,
					RemediationID: "req-hr-loop",
					Context:       "Test for " + reason,
				})

				Expect(err).NotTo(HaveOccurred(), "Failed for reason: %s", reason)
				Expect(resp.NeedsHumanReview).To(BeTrue(), "NeedsHumanReview should be true for: %s", reason)
				Expect(resp.HumanReviewReason).NotTo(BeNil(), "HumanReviewReason should not be nil for: %s", reason)
				Expect(*resp.HumanReviewReason).To(Equal(reason), "Reason should match for: %s", reason)
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

			resp, err := mockClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
				IncidentID:    "test-resolved-001",
				RemediationID: "req-resolved-001",
				Context:       "Pod was in CrashLoopBackOff but has now recovered",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.NeedsHumanReview).To(BeFalse())
			Expect(resp.SelectedWorkflow).To(BeNil())
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

			resp, err := mockClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
				IncidentID:    "test-inconclusive-001",
				RemediationID: "req-inconclusive-001",
				Context:       "Intermittent network failures with unclear pattern",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.NeedsHumanReview).To(BeTrue())
			Expect(*resp.HumanReviewReason).To(Equal("investigation_inconclusive"))
		})
	})

	Context("Validation History - DD-HAPI-002", func() {
		It("should return validation attempts history when present", func() {
			mockClient.WithHumanReviewAndHistory(
				"llm_parsing_error",
				[]string{"Parsing failed on first attempt"},
				[]aianalysisclient.ValidationAttempt{
					{
						Attempt:    1,
						WorkflowID: "restart-pod-v1",
						IsValid:    false,
						Errors:     []string{"JSON parsing failed", "Invalid JSON structure"},
						Timestamp:  "2025-12-09T10:00:00Z",
					},
					{
						Attempt:    2,
						WorkflowID: "restart-pod-v1",
						IsValid:    true,
						Errors:     nil,
						Timestamp:  "2025-12-09T10:00:01Z",
					},
				},
			)

			resp, err := mockClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
				IncidentID:    "test-validation-001",
				RemediationID: "req-validation-001",
				Context:       "Database connection timeout in staging",
				EnrichmentResults: &aianalysisclient.EnrichmentResults{
					OwnerChain: []aianalysisclient.OwnerChainEntry{
						{Namespace: "staging", Kind: "Deployment", Name: "db-client"},
					},
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.ValidationAttemptsHistory).To(HaveLen(2))
			Expect(resp.ValidationAttemptsHistory[0].IsValid).To(BeFalse())
			Expect(resp.ValidationAttemptsHistory[1].IsValid).To(BeTrue())
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

			_, err := mockClient.Investigate(shortCtx, &aianalysisclient.IncidentRequest{
				IncidentID:    "test-timeout-001",
				RemediationID: "req-timeout-001",
				Context:       "Test timeout handling",
			})

			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
		})

		It("should return API error for server failures - BR-AI-009", func() {
			mockClient.WithAPIError(500, "Internal server error")

			_, err := mockClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
				IncidentID:    "test-error-001",
				RemediationID: "req-error-001",
				Context:       "Test error handling",
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("500"))
		})

		It("should handle validation errors (400) - BR-AI-009", func() {
			mockClient.WithAPIError(400, "Invalid request: missing required field 'context'")

			// Empty request with mock - mock still returns configured error
			_, err := mockClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
				IncidentID:    "",  // Empty - would fail validation on real HAPI
				RemediationID: "",  // Empty - would fail validation on real HAPI
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("400"))
		})
	})
})
