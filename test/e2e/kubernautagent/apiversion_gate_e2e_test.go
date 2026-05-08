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

package kubernautagent

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/agentclient"
)

// apiVersion Validation Gate E2E Tests — Issue #1044
// Test Plan: docs/tests/1044/E2E_TEST_PLAN.md
// Scenarios: E2E-KA-1044-001 through E2E-KA-1044-002
// Business Requirements: BR-AI-1044
//
// These tests validate that the apiVersionValidationGate correctly escalates
// to human review when the LLM omits api_version for a Kind that exists in
// multiple API groups (CRD kind collision). The mock LLM scenario
// "ambiguous_kind" always returns an empty APIVersion, so the gate exhausts
// its retries and sets HumanReviewNeeded=true — the security-critical path
// that prevents incorrect RBAC grants.
//
// Infrastructure: Kind cluster with two CRDs (TestWidget in
// alpha.kubernaut-test.ai/v1 and beta.kubernaut-test.ai/v1),
// Mock LLM with ambiguous_kind scenario, full KA pipeline.

var _ = Describe("E2E-KA-1044: apiVersion Validation Gate", Label("e2e", "ka", "apiversion-gate", "1044"), func() {

	Context("BR-AI-1044: Gate exhaustion with ambiguous CRD kind", func() {

		It("E2E-KA-1044-001: Pod signal with RCA targeting ambiguous TestWidget triggers human review", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-1044-001
			// Business Outcome: When the LLM targets an ambiguous Kind (TestWidget)
			//   without api_version and exhausts retries, the gate escalates to
			//   human review, preventing incorrect RBAC grants.
			// BR: BR-AI-1044 AC3
			// Risk Mitigation: R1 (CRD kind collision → wrong GVK → wrong RBAC)

			// ========================================
			// ARRANGE: Pod signal that triggers ambiguous_kind scenario
			// ========================================
			req := &agentclient.IncidentRequest{
				IncidentID:        "e2e-1044-001-pod-ambiguous",
				RemediationID:     "req-e2e-1044-001",
				SignalName:        "MOCK_AMBIGUOUS_KIND",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "mock_ambiguous_kind: TestWidget misconfiguration detected",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "infrastructure",
				ClusterName:       "kubernaut-agent-e2e",
			}

			// ========================================
			// ACT: Full investigation pipeline
			// ========================================
			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred(), "investigation API call should succeed")
			Expect(result).NotTo(BeNil(), "response should not be nil")

			// ========================================
			// ASSERT: Gate exhaustion → human review
			// ========================================
			Expect(result.NeedsHumanReview.Value).To(BeTrue(),
				"needs_human_review must be true when gate exhausts retries for ambiguous kind without api_version")

			hrReason, ok := result.HumanReviewReason.Get()
			Expect(ok).To(BeTrue(), "human_review_reason must be set")
			Expect(hrReason).To(Equal(agentclient.HumanReviewReasonRcaIncomplete),
				"human_review_reason should be rca_incomplete (gate could not resolve ambiguous kind)")

			Expect(result.SelectedWorkflow.Value).To(BeNil(),
				"selected_workflow must be nil when gate escalates to human review")

			Expect(result.Warnings).NotTo(BeEmpty(),
				"warnings should be present when gate exhausts retries for ambiguous kind")
		})

		It("E2E-KA-1044-002: TestWidget signal directly triggers gate exhaustion and human review", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-1044-002
			// Business Outcome: Same gate behavior when the signal resource itself
			//   is the ambiguous kind — confirms gate fires regardless of whether
			//   the ambiguity is in the signal or the RCA target.
			// BR: BR-AI-1044 AC3
			// Risk Mitigation: R1

			// ========================================
			// ARRANGE: TestWidget signal (ambiguous kind as signal resource)
			// ========================================
			req := &agentclient.IncidentRequest{
				IncidentID:        "e2e-1044-002-widget-direct",
				RemediationID:     "req-e2e-1044-002",
				SignalName:        "MOCK_AMBIGUOUS_KIND",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "TestWidget",
				ResourceName:      "test-widget-instance",
				ErrorMessage:      "mock_ambiguous_kind: TestWidget spec misconfiguration",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "infrastructure",
				ClusterName:       "kubernaut-agent-e2e",
			}

			// ========================================
			// ACT
			// ========================================
			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred(), "investigation API call should succeed")
			Expect(result).NotTo(BeNil(), "response should not be nil")

			// ========================================
			// ASSERT: Same gate exhaustion behavior
			// ========================================
			Expect(result.NeedsHumanReview.Value).To(BeTrue(),
				"needs_human_review must be true for ambiguous TestWidget kind without api_version")

			hrReason, ok := result.HumanReviewReason.Get()
			Expect(ok).To(BeTrue(), "human_review_reason must be set")
			Expect(hrReason).To(Equal(agentclient.HumanReviewReasonRcaIncomplete),
				"human_review_reason should be rca_incomplete")

			Expect(result.Warnings).NotTo(BeEmpty(),
				"warnings should be present when gate exhausts retries")
		})
	})
})
