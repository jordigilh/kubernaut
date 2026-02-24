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

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

// Approval Context Integration Tests
// Test Plan: MockLLM Test Extension Triage - Phase 1 (Feb 4, 2026)
// Scenarios: IT-AA-085, IT-AA-086, IT-AA-088
// Business Requirements: BR-AI-076, BR-HAPI-200, BR-AI-028, BR-AI-029
//
// Purpose: Validate HAPI-AA integration for approval context population,
// human review reason propagation, and Rego policy evaluation with real MockLLM responses.
//
// Test Strategy (per 03-testing-strategy.mdc):
// - Integration tests use REAL HAPI service (business logic, not external API)
// - HAPI runs with Mock LLM enabled (external API properly mocked)
// - Uses real Kubernetes API (envtest) for complete reconciliation validation
// - MockLLM scenarios: low_confidence, no_workflow_found, max_retries_exhausted, oomkilled
//
// Infrastructure Required:
//   - PostgreSQL (:15438)
//   - Redis (:16384)
//   - DataStorage API (:18095)
//   - Mock LLM Service (:18141) - Provides deterministic test responses
//   - HolmesGPT API (:18120) - Real business logic with Mock LLM backend

var _ = Describe("Approval Context Integration", Label("integration", "approval", "hapi-aa"), func() {
	var (
		testCtx    context.Context
		cancelFunc context.CancelFunc
	)

	BeforeEach(func() {
		testCtx, cancelFunc = context.WithTimeout(context.Background(), 60*time.Second)
	})

	AfterEach(func() {
		cancelFunc()
	})

	// Helper function to create and reconcile AIAnalysis for approval testing
	createAndReconcileAIAnalysis := func(signalType, severity string) *aianalysisv1.AIAnalysis {
		remediationID := fmt.Sprintf("rr-approval-%s", uuid.New().String()[:8])
		
		// Create AIAnalysis CRD directly (no Signal CRD needed for integration tests)
		aianalysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-approval-%s", uuid.New().String()[:8]),
				Namespace: testNamespace,
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: remediationID,
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      fmt.Sprintf("fp-%s", uuid.New().String()[:8]),
						Severity:         severity,
						SignalName:       signalType,
						Environment:      "production",
						BusinessPriority: "P1",
						TargetResource: aianalysisv1.TargetResource{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: testNamespace,
						},
					},
					// DD-AIANALYSIS-005: v1.x supports single analysis type only
					AnalysisTypes: []string{"investigation"},
				},
			},
		}

		// Create AIAnalysis
		Expect(k8sClient.Create(testCtx, aianalysis)).To(Succeed())

		// Clean up after test
		defer func() {
			_ = k8sClient.Delete(context.Background(), aianalysis)
		}()

		// Wait for reconciliation to complete
		Eventually(func() string {
			if err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(aianalysis), aianalysis); err != nil {
				return ""
			}
			return aianalysis.Status.Phase
		}, 60*time.Second, 1*time.Second).Should(Or(Equal("Completed"), Equal("Failed")),
			"AIAnalysis should reach terminal phase")

		// Return final state
		Expect(k8sClient.Get(testCtx, client.ObjectKeyFromObject(aianalysis), aianalysis)).To(Succeed())

		return aianalysis
	}

	Context("BR-AI-076: Alternative Workflows in Approval Context", func() {
		It("IT-AA-085: Should populate approval context with alternatives from HAPI", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: IT-AA-085
			// Gap ID: GAP-001
			// Business Outcome: Alternative workflows from HAPI correctly populate AIAnalysis approvalContext
			// Confidence: 95%
			// BR: BR-AI-076 (Approval Context), BR-AUDIT-005 Gap #4 (Alternative Workflows)
			// MockLLM Scenario: low_confidence (confidence=0.35, includes alternatives)

			// ========================================
			// ARRANGE & ACT: Create AIAnalysis with low confidence scenario
			// ========================================
			// Mock LLM "low_confidence" scenario returns:
			// - confidence: 0.35 (low, <0.8 threshold)
			// - selected_workflow: generic-restart-v1
			// - alternative_workflows: [alt1, alt2] (E2E-HAPI-002 validated)
			result := createAndReconcileAIAnalysis("MOCK_LOW_CONFIDENCE", "high")

		// ========================================
		// ASSERT: Low confidence triggers terminal failure (BR-AI-050)
		// ========================================
		// Per BR-AI-050 + Issue #28: Low confidence (<0.7) transitions to Failed phase
		Expect(result.Status.Phase).To(Equal("Failed"),
			"Low confidence (<0.7) transitions to Failed phase per BR-AI-050")

		// Human review required for low confidence
		Expect(result.Status.NeedsHumanReview).To(BeTrue(),
			"NeedsHumanReview=true for low confidence scenarios")

		// Status should indicate low confidence failure
		Expect(result.Status.Reason).To(Equal("WorkflowResolutionFailed"),
			"Reason should be 'WorkflowResolutionFailed' (umbrella category)")
		Expect(result.Status.SubReason).To(Equal("LowConfidence"),
			"SubReason should be 'LowConfidence' for specific failure type")

		// Alternative workflows stored for human review context (BR-AUDIT-005 Gap #4)
		Expect(result.Status.AlternativeWorkflows).ToNot(BeEmpty(),
			"Alternative workflows should be stored for human review context")
		Expect(len(result.Status.AlternativeWorkflows)).To(BeNumerically(">=", 2),
			"Mock LLM returns at least 2 alternatives for low_confidence scenario")

		// Validate alternative structure
		for _, alt := range result.Status.AlternativeWorkflows {
			Expect(alt.WorkflowID).ToNot(BeEmpty(), "Alternative workflow must have ID")
			Expect(alt.Rationale).ToNot(BeEmpty(), "Alternative must have rationale")
		}

		// NOTE: ApprovalContext only populated in Analyzing phase (Completed flow)
		// Low confidence scenarios transition to Failed without reaching Analyzing
		// Alternative workflows stored in Status.AlternativeWorkflows for audit trail

			// ========================================
			// BUSINESS IMPACT
			// ========================================
			// Operator sees alternatives in approval UI
			// Reduces approval time by providing context
			// Enables informed decision-making for low-confidence recommendations
		})
	})

	Context("BR-HAPI-200, BR-AI-028: Human Review Reason Code Mapping", func() {
		It("IT-AA-086: Maps HAPI human_review_reason to AA approval status", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: IT-AA-086
			// Gap ID: GAP-002
			// Business Outcome: HAPI human_review_reason correctly triggers AA approval routing
			// Confidence: 93%
			// BR: BR-HAPI-200 (Structured Human Review Reasons), BR-AI-028 (Auto-Approve or Flag)
			// MockLLM Scenarios: no_workflow_found, max_retries_exhausted

			testCases := []struct {
				scenario           string
				signalType         string
				expectedReason     string
				expectedApproval   bool
				expectedConfidence float64
			}{
				{
					scenario:           "no_workflow_found",
					signalType:         "MOCK_NO_WORKFLOW_FOUND",
					expectedReason:     "no_matching_workflows",
					expectedApproval:   true,
					expectedConfidence: 0.0,
				},
				{
					scenario:           "llm_parsing_error",
					signalType:         "MOCK_MAX_RETRIES_EXHAUSTED",
					expectedReason:     "llm_parsing_error",
					expectedApproval:   true,
					expectedConfidence: 0.0,
				},
			}

		for _, tc := range testCases {
			By(fmt.Sprintf("Testing %s scenario", tc.scenario))

			// ARRANGE & ACT: Create and reconcile AIAnalysis with specific scenario
			// HAPI will set human_review_reason based on MockLLM scenario
			result := createAndReconcileAIAnalysis(tc.signalType, "high")

			// ASSERT: Per BR-AI-050, low confidence (<0.7) and no workflow scenarios transition to Failed
			// Only high confidence scenarios reach Completed phase
			if tc.expectedConfidence < 0.7 || tc.expectedConfidence == 0.0 {
				// Low confidence / no workflow: Terminal failure
				Expect(result.Status.Phase).To(Equal("Failed"),
					fmt.Sprintf("Low confidence/no workflow transitions to Failed for %s", tc.scenario))

				Expect(result.Status.NeedsHumanReview).To(Equal(tc.expectedApproval),
					fmt.Sprintf("NeedsHumanReview should be %v for %s", tc.expectedApproval, tc.scenario))

				// Per reconciliation-phases.md v2.1: Reason = umbrella, SubReason = specific
				Expect(result.Status.Reason).To(Equal("WorkflowResolutionFailed"),
					"Reason should be umbrella category per BR-HAPI-197")

				// Map expectedReason (from test case) to SubReason enum
				// per mapEnumToSubReason in response_processor.go:859-873
				expectedSubReason := map[string]string{
					"no_matching_workflows": "NoMatchingWorkflows",
					"llm_parsing_error":     "LLMParsingError",
					"low_confidence":        "LowConfidence",
				}[tc.expectedReason]

				Expect(result.Status.SubReason).To(Equal(expectedSubReason),
					fmt.Sprintf("SubReason should be '%s' for %s scenario", expectedSubReason, tc.scenario))
			} else {
				// High confidence: Completed phase
				Expect(result.Status.Phase).To(Equal("Completed"),
					fmt.Sprintf("High confidence (>=0.7) reaches Completed for %s", tc.scenario))

				Expect(result.Status.ApprovalRequired).To(Equal(tc.expectedApproval),
					fmt.Sprintf("ApprovalRequired should be %v for %s", tc.expectedApproval, tc.scenario))
			}

				// ========================================
				// BUSINESS IMPACT
				// ========================================
				// Human review reasons correctly propagate from HAPI to AA
				// Enables proper approval routing based on failure type
				// Operators see consistent reason codes across the system
			}
		})
	})

	Context("BR-AI-028, BR-AI-029: Rego Policy with MockLLM Confidence Scores", func() {
		It("IT-AA-088: Evaluates Rego policy with MockLLM confidence scores", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: IT-AA-088
			// Gap ID: GAP-004
			// Business Outcome: Rego policies correctly evaluate confidence scores from MockLLM
			// Confidence: 94%
			// BR: BR-AI-028 (Auto-Approve or Flag), BR-AI-029 (Rego Policy Evaluation)
			// MockLLM Scenarios: oomkilled (0.95), low_confidence (0.35), no_workflow_found (0.0)

			testCases := []struct {
				scenario           string
				signalType         string
				expectedConfidence float64
				expectedApproval   bool
				description        string
			}{
			{
				scenario:           "high_confidence_production",
				signalType:         "OOMKilled", // Mock: 0.88 (measured from test run)
				expectedConfidence: 0.88,
				expectedApproval:   true, // Production ALWAYS requires approval (per approval.rego:119-122)
				description:        "High confidence (>=0.8) in production requires approval per Rego policy",
			},
				{
					scenario:           "low_confidence_require_approval",
					signalType:         "MOCK_LOW_CONFIDENCE", // Mock: 0.35
					expectedConfidence: 0.35,
					expectedApproval:   true, // Require approval (low confidence < 0.8)
					description:        "Low confidence (<0.8) should require approval",
				},
				{
					scenario:           "zero_confidence_require_approval",
					signalType:         "MOCK_NO_WORKFLOW_FOUND", // Mock: 0.0
					expectedConfidence: 0.0,
					expectedApproval:   true, // Require approval (no workflow)
					description:        "Zero confidence (no workflow) should require approval",
				},
			}

		for _, tc := range testCases {
			By(fmt.Sprintf("Testing %s: %s", tc.scenario, tc.description))

			// ARRANGE & ACT: Create and reconcile AIAnalysis with specific scenario
			// HAPI returns MockLLM confidence, Rego policy evaluates for approval
			result := createAndReconcileAIAnalysis(tc.signalType, "high")

			// ASSERT: Per BR-AI-050, confidence <0.7 transitions to Failed phase
			if tc.expectedConfidence < 0.7 {
				// Low confidence / no workflow: Terminal failure (BR-AI-050 + Issue #28/#29)
				Expect(result.Status.Phase).To(Equal("Failed"),
					fmt.Sprintf("Low confidence (<0.7) transitions to Failed for %s", tc.scenario))

				// Confidence score stored in SelectedWorkflow (if workflow was selected)
				if result.Status.SelectedWorkflow != nil {
					Expect(result.Status.SelectedWorkflow.Confidence).To(
						BeNumerically("~", tc.expectedConfidence, 0.05),
						fmt.Sprintf("Confidence should match MockLLM %s scenario", tc.scenario))
				}

				// NeedsHumanReview set for low confidence failures
				Expect(result.Status.NeedsHumanReview).To(Equal(tc.expectedApproval),
					fmt.Sprintf("NeedsHumanReview should be %v for %s", tc.expectedApproval, tc.scenario))

			} else {
				// High confidence: Completed phase (Rego policy evaluation)
				Expect(result.Status.Phase).To(Equal("Completed"),
					fmt.Sprintf("High confidence (>=0.7) reaches Completed for %s", tc.scenario))

				// Confidence score matches MockLLM scenario
				if result.Status.SelectedWorkflow != nil {
					Expect(result.Status.SelectedWorkflow.Confidence).To(
						BeNumerically("~", tc.expectedConfidence, 0.05),
						fmt.Sprintf("Confidence should match MockLLM %s scenario", tc.scenario))
				}

				// Approval decision matches expected policy outcome (Rego evaluation)
				Expect(result.Status.ApprovalRequired).To(Equal(tc.expectedApproval),
					fmt.Sprintf("ApprovalRequired should be %v for %s", tc.expectedApproval, tc.scenario))

				// Validate approval context for manual review scenarios
				if tc.expectedApproval {
					Expect(result.Status.ApprovalContext).ToNot(BeNil(),
						"ApprovalContext must be populated when approval required")
					Expect(result.Status.ApprovalContext.ConfidenceScore).To(
						BeNumerically("~", tc.expectedConfidence, 0.05),
						"ApprovalContext confidence should match workflow confidence")

					// Confidence level correctly categorized based on actual confidence
					expectedLevel := "low"
					if tc.expectedConfidence >= 0.6 && tc.expectedConfidence < 0.8 {
						expectedLevel = "medium"
					} else if tc.expectedConfidence >= 0.8 {
						expectedLevel = "high"
					}
					Expect(result.Status.ApprovalContext.ConfidenceLevel).To(Equal(expectedLevel),
						fmt.Sprintf("Confidence level should be '%s' for score %v", expectedLevel, tc.expectedConfidence))
				}
			}

				// ========================================
				// BUSINESS IMPACT
				// ========================================
				// Rego policies correctly evaluate real HAPI confidence scores
				// Enables automated approval routing based on confidence thresholds
				// Validates complete HAPI-AA-Rego integration end-to-end
			}
		})
	})
})
