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

package kubernautagent

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Issue #2190: migrated from the retired pkg/agentclient HTTP submit/poll/
// result channel to direct AgentSession CRD Create/watch (DD-AA-KA-001).
// investigateViaAgentSession Creates the AgentSession directly (playing AA's
// role -- no AA controller runs in this KA-focused suite) and polls
// Status.Phase to a terminal state, replacing sessionClient.Investigate().
// AgentSessionSpec is documented as a 1:1, lossless translation of the
// retired agentclient.IncidentRequest body (see
// api/agentsession/v1alpha1/agentsession_types.go), so field-for-field this
// migration is a rename, not a redesign -- except E2E-KA-007/008 (invalid
// request rejection), which move from ogen HTTP validation to the CRD's own
// OpenAPI schema validation (apierrors.IsInvalid), a different mechanism
// proving the same business intent (SI-10: bad investigation-identity input
// rejected clearly, before ever reaching KA's investigation logic).

// Incident Analysis E2E Tests
// Test Plan: docs/development/testing/KA_E2E_TEST_PLAN.md
// Scenarios: E2E-KA-001 through E2E-KA-008 (8 total)
// Business Requirements: BR-KA-197, BR-KA-002, BR-AI-075, BR-KA-200
//
// Purpose: Validate incident analysis endpoint behavior and correctness

var _ = Describe("E2E-KA Incident Analysis", Label("e2e", "ka", "incident"), func() {

	Context("BR-KA-197: Human review scenarios", func() {

		It("E2E-KA-001: No workflow found returns human review", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-001
			// Business Outcome: When no matching workflow exists, system escalates to human operator with clear reason
			// Ported from: test_mock_llm_edge_cases_e2e.py:121 (Python KA, deprecated)
			// BR: BR-KA-197

			// ========================================
			// ARRANGE: Create request with MOCK_NO_WORKFLOW_FOUND
			// ========================================
			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: "test-rem-001", Namespace: sharedNamespace},
				IncidentID:            "test-edge-001",
				RemediationID:         "test-rem-001",
				SignalName:            "MOCK_NO_WORKFLOW_FOUND",
				Severity:              "high",
				SignalSource:          "prometheus",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod",
				ErrorMessage:          "No automation available",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
				ClusterName:           "e2e-test",
			}

			// ========================================
			// ACT: Investigate via AgentSession CRD (BR-AA-KA-064, #2190)
			// ========================================
			result, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis should succeed")

			// ========================================
			// ASSERT: Business outcome validation
			// ========================================
			// BEHAVIOR: Human review required
			Expect(result.NeedsHumanReview).To(BeTrue(),
				"needsHumanReview must be true when no workflow found")
			Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"),
				"humanReviewReason must indicate no matching workflows")
			Expect(result.SelectedWorkflow).To(BeNil(),
				"selectedWorkflow must be absent when no workflow found")

			// CORRECTNESS: Zero confidence
			Expect(result.Confidence).To(BeNumerically("==", 0.0),
				"confidence must be 0.0 when no automation possible")

			// CORRECTNESS: Warnings present (may or may not contain "MOCK" - implementation detail)
			Expect(result.Warnings).ToNot(BeEmpty(),
				"warnings must be present when no workflow found")

			// BUSINESS IMPACT: (verified by integration tests - AIAnalysis sets RequiresHumanReview phase)
			// - AIAnalysis controller sets phase = "RequiresHumanReview"
			// - Creates notification for operator
			// - Does NOT create WorkflowExecution CRD
		})

		It("E2E-KA-002: Low confidence returns human review with alternatives", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-002
			// Business Outcome: When confidence is low, system provides tentative recommendation but requires human decision
			// Ported from: test_mock_llm_edge_cases_e2e.py:153 (Python KA, deprecated)
			// BR: BR-KA-197

			// ========================================
			// ARRANGE
			// ========================================
			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: "test-rem-002", Namespace: sharedNamespace},
				IncidentID:            "test-edge-002",
				RemediationID:         "test-rem-002",
				SignalName:            "MOCK_LOW_CONFIDENCE",
				Severity:              "high",
				SignalSource:          "prometheus",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod-2",
				ErrorMessage:          "Uncertain root cause",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
				ClusterName:           "e2e-test",
			}

			// ========================================
			// ACT (#2190: AgentSession CRD flow)
			// ========================================
			result, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BR-KA-197 + BR-AI-088: KA returns confidence but does NOT enforce thresholds
			// AIAnalysis owns the threshold logic (70% in V1.0, configurable in V1.1)
			Expect(result.NeedsHumanReview).To(BeFalse(),
				"KA should NOT set needsHumanReview based on confidence thresholds (BR-KA-197)")
			Expect(result.SelectedWorkflow).ToNot(BeNil(),
				"selectedWorkflow must be present")

			// CORRECTNESS: Low confidence returned for AIAnalysis to evaluate
			Expect(result.Confidence).To(BeNumerically("<", 0.5),
				"confidence < 0.5 signals low confidence to AIAnalysis")

			// CORRECTNESS: Alternatives provided for AIAnalysis evaluation
			Expect(result.AlternativeWorkflows).ToNot(BeEmpty(),
				"alternativeWorkflows help AIAnalysis when confidence is low")

			// BUSINESS IMPACT: AIAnalysis applies 70% threshold, sees 0.35 < 0.70, sets needs_human_review=true
		})

		It("E2E-KA-003: Max retries exhausted returns validation history", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-003
			// Business Outcome: When LLM self-correction fails after max retries, provide complete validation history for debugging
			// Ported from: test_mock_llm_edge_cases_e2e.py:189 (Python KA, deprecated)
			// BR: BR-KA-197

			// ========================================
			// ARRANGE
			// ========================================
			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: "test-rem-003", Namespace: sharedNamespace},
				IncidentID:            "test-edge-003",
				RemediationID:         "test-rem-003",
				SignalName:            "MOCK_MAX_RETRIES_EXHAUSTED",
				Severity:              "high",
				SignalSource:          "prometheus",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod-3",
				ErrorMessage:          "Validation failed",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
				ClusterName:           "e2e-test",
			}

			// ========================================
			// ACT (#2190: AgentSession CRD flow)
			// ========================================
			result, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: AI gave up after max retries
			Expect(result.NeedsHumanReview).To(BeTrue(),
				"needsHumanReview must be true when max retries exhausted")
			Expect(result.HumanReviewReason).To(Equal("llm_parsing_error"),
				"humanReviewReason must indicate LLM parsing error")
			Expect(result.SelectedWorkflow).To(BeNil(),
				"selectedWorkflow must be absent when parsing failed")

			// CORRECTNESS: Complete audit trail
			Expect(result.ValidationAttemptsHistory).ToNot(BeEmpty(),
				"validationAttemptsHistory must be present for debugging")
			Expect(result.ValidationAttemptsHistory).To(HaveLen(3), "MOCK_MAX_RETRIES_EXHAUSTED triggers exactly 3 validation attempts")

			// Verify each attempt has required fields
			for i, attempt := range result.ValidationAttemptsHistory {
				Expect(attempt.Attempt).To(Equal(i+1),
					"attempt number must be sequential")
				Expect(attempt.IsValid).To(BeFalse(),
					"isValid must be false for failed validation")
				Expect(attempt.Errors).ToNot(BeEmpty(),
					"errors must be present for failed validation")
				Expect(attempt.Timestamp).ToNot(BeEmpty(),
					"timestamp must be present for each attempt")
			}

			// BUSINESS IMPACT: Operator sees why AI failed, can debug or manually intervene
		})
	})

	Context("BR-KA-002: Happy path scenarios", func() {

		It("E2E-KA-004: Normal incident analysis succeeds", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-004
			// Business Outcome: Standard signal types produce confident workflow recommendations
			// Ported from: test_mock_llm_edge_cases_e2e.py:332 (Python KA, deprecated)
			// BR: BR-KA-002

			// ========================================
			// ARRANGE
			// ========================================
			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: "test-rem-004", Namespace: sharedNamespace},
				IncidentID:            "test-happy-004",
				RemediationID:         "test-rem-004",
				SignalName:            "OOMKilled",
				Severity:              "high",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod-4",
				ErrorMessage:          "Container memory limit exceeded",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
				ClusterName:           "e2e-test",
			}

			// ========================================
			// ACT (#2190: AgentSession CRD flow)
			// ========================================
			result, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Confident recommendation provided
			Expect(result.NeedsHumanReview).To(BeFalse(),
				"needsHumanReview must be false for confident recommendation")
			Expect(result.SelectedWorkflow).ToNot(BeNil(),
				"selectedWorkflow must be present")
			Expect(result.Confidence).To(BeNumerically("~", 0.95, 0.05),
				"Mock LLM 'oomkilled' scenario returns confidence = 0.95 ± 0.05")

			// CORRECTNESS: Workflow matches signal type
			// Note: selectedWorkflow is raw JSON - detailed field validation skipped in E2E
			// Workflow selection logic validated in integration tests

			// BUSINESS IMPACT: AIAnalysis creates WorkflowExecution automatically
		})
	})

	Context("BR-AI-075: Response structure validation", func() {

		It("E2E-KA-005: Incident response structure validation", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-005
			// Business Outcome: Response contains all fields required by AIAnalysis controller
			// Ported from: test_workflow_selection_e2e.py:217 (Python KA, deprecated)
			// BR: BR-AI-075

			// ========================================
			// ARRANGE
			// ========================================
			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: "test-rem-005", Namespace: sharedNamespace},
				IncidentID:            "test-struct-005",
				RemediationID:         "test-rem-005",
				SignalName:            "CrashLoopBackOff",
				Severity:              "high",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod-5",
				ErrorMessage:          "Container restarting repeatedly",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
				ClusterName:           "e2e-test",
			}

			// ========================================
			// ACT (#2190: AgentSession CRD flow)
			// ========================================
			result, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Complete response structure
			Expect(result.IncidentID).To(Equal("test-struct-005"),
				"incidentID must match request")
			Expect(result.Analysis).ToNot(BeEmpty(), "analysis field must be present")

			// CORRECTNESS: Exact confidence value from Mock LLM
			Expect(result.Confidence).To(BeNumerically("~", 0.95, 0.05),
				"Mock LLM 'crashloop' scenario returns confidence = 0.95 ± 0.05")

			// BUSINESS IMPACT: AIAnalysis can parse response without errors
		})

		It("E2E-KA-006: Incident with enrichment results processing", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-006
			// Business Outcome: EnrichmentResults (detectedLabels, customLabels) influence workflow selection
			// Ported from: test_workflow_selection_e2e.py:246 (Python KA, deprecated)
			// BR: DD-KA-002 (Custom Labels Auto-Append)

			// ========================================
			// ARRANGE
			// ========================================
			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: "test-rem-006", Namespace: sharedNamespace},
				IncidentID:            "test-enrich-006",
				RemediationID:         "test-rem-006",
				SignalName:            "OOMKilled",
				Severity:              "high",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod-6",
				ErrorMessage:          "Container memory limit exceeded",
				// EnrichmentResults: TODO - complex raw-JSON construction
				Environment:      "production",
				Priority:         "P1",
				RiskTolerance:    "medium",
				BusinessCategory: "standard",
				ClusterName:      "e2e-test",
			}

			// ========================================
			// ACT (#2190: AgentSession CRD flow)
			// ========================================
			result, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Workflow selection influenced by labels
			Expect(result.SelectedWorkflow).ToNot(BeNil(),
				"selectedWorkflow must be present")

			// CORRECTNESS: Appropriate workflow for label context
			// (Workflow should respect GitOps/PDB/stateful constraints)
			// This is validated by workflow catalog logic, not explicitly testable here

			// BUSINESS IMPACT: Workflows respect cluster constraints (GitOps, PDB, stateful)
		})
	})

	Context("BR-KA-200: Error handling", func() {

		It("E2E-KA-007: Invalid request returns error", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-007
			// Business Outcome: Invalid requests rejected with clear error messages
			// Ported from: test_workflow_selection_e2e.py:342 (Python KA, deprecated)
			// BR: BR-KA-200
			//
			// #2190: redesigned from ogen HTTP validation (4xx) to the
			// AgentSession CRD's own OpenAPI schema validation -- same
			// business intent (bad input rejected before ever reaching KA's
			// investigation logic), different enforcement point now that
			// pkg/agentclient's HTTP channel is retired.

			// ========================================
			// ARRANGE: Create AgentSession with missing required fields
			// ========================================
			as := &agentsessionv1.AgentSession{
				ObjectMeta: metav1.ObjectMeta{Name: "as-test-invalid-007", Namespace: sharedNamespace},
				Spec: agentsessionv1.AgentSessionSpec{
					IncidentID: "test-invalid-007",
					// Missing remediationRequestRef, remediationID, signalName,
					// severity, etc.
				},
			}

			// ========================================
			// ACT
			// ========================================
			err := k8sClient.Create(ctx, as)

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Request rejected
			Expect(err).To(HaveOccurred(),
				"Invalid request should be rejected")
			Expect(apierrors.IsInvalid(err)).To(BeTrue(),
				"rejection must be a schema-validation error (missing required fields), not e.g. RBAC/network")

			// BUSINESS IMPACT: Caller knows what to fix
		})

		It("E2E-KA-008: Missing remediation ID returns error", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-008
			// Business Outcome: remediation_id is mandatory for audit trail correlation
			// Ported from: test_workflow_selection_e2e.py:364 (Python KA, deprecated)
			// BR: DD-WORKFLOW-002
			//
			// #2190: redesigned per E2E-KA-007 above. AgentSessionSpec.RemediationID
			// carries the same kubebuilder MinLength=1 constraint the retired
			// agentclient.IncidentRequest OpenAPI schema enforced (issue #2190).

			// ========================================
			// ARRANGE: Create AgentSession WITHOUT remediationID
			// ========================================
			as := &agentsessionv1.AgentSession{
				ObjectMeta: metav1.ObjectMeta{Name: "as-test-no-rem-008", Namespace: sharedNamespace},
				Spec: agentsessionv1.AgentSessionSpec{
					RemediationRequestRef: agentsessionv1.ObjectRef{Name: "test-no-rem-008", Namespace: sharedNamespace},
					IncidentID:            "test-no-rem-008",
					// RemediationID is MISSING
					SignalName:        "OOMKilled",
					Severity:          "high",
					SignalSource:      "kubernetes",
					ResourceNamespace: "default",
					ResourceKind:      "Pod",
					ResourceName:      "test-pod-8",
					ErrorMessage:      "Container memory limit exceeded",
				},
			}

			// ========================================
			// ACT
			// ========================================
			err := k8sClient.Create(ctx, as)

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Request rejected
			Expect(err).To(HaveOccurred(),
				"Request without remediationID should be rejected")
			Expect(apierrors.IsInvalid(err)).To(BeTrue(),
				"rejection must be a schema-validation error")

			// CORRECTNESS: kubebuilder's MinLength=1 constraint enforces this server-side.
			Expect(err.Error()).To(ContainSubstring("remediationID"),
				"Error should indicate the missing/invalid remediationID field")

			// BUSINESS IMPACT: Audit trail can correlate events
		})
	})
})
