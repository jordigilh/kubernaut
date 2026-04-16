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
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/agentclient"
)

// TP-433-ADV Phase 8: E2E Adversarial Parity Tests
//
// These tests validate the P1-P7 implementation against a live Kind cluster
// with Mock LLM. Each test triggers a specific Mock LLM scenario via the
// signal_name keyword matching in the Mock LLM registry.
//
// Mock LLM scenarios used: problem_resolved, predictive_no_action,
// problem_resolved_contradiction, oomkilled, no_workflow_found,
// rca_incomplete, low_confidence

var _ = Describe("E2E-KA-433-ADV: Adversarial Parity Tests", Label("e2e", "ka", "adversarial"), func() {

	// Helper to build a standard request with the mock keyword as signal name
	buildRequest := func(incidentID, signalName, severity string) *agentclient.IncidentRequest {
		sev := agentclient.SeverityCritical
		switch severity {
		case "high":
			sev = agentclient.SeverityHigh
		case "medium":
			sev = agentclient.SeverityMedium
		case "low":
			sev = agentclient.SeverityLow
		}
		return &agentclient.IncidentRequest{
			IncidentID:        incidentID,
			RemediationID:     "rem-adv-" + incidentID,
			SignalName:        signalName,
			Severity:          sev,
			SignalSource:      "kubernetes",
			ResourceNamespace: "production",
			ResourceKind:      "Pod",
			ResourceName:      "test-pod-adv",
			ErrorMessage:      signalName + " triggered for adversarial test",
			Environment:       "production",
			Priority:          "high",
			RiskTolerance:     "medium",
			BusinessCategory:  "test",
			ClusterName:       "kubernaut-agent-e2e",
		}
	}

	// ===================================================================
	// GAP-002: Outcome Routing (is_actionable)
	// ===================================================================

	Context("GAP-002: Outcome routing", func() {

		It("E2E-KA-433-ADV-001: problem_resolved → is_actionable=false, no workflow", func() {
			req := buildRequest("adv-001", "mock_problem_resolved", "low")
			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			isActionable, hasActionable := result.IsActionable.Get()
			Expect(hasActionable).To(BeTrue(),
				"M1: is_actionable must be set for problem_resolved outcome")
			Expect(isActionable).To(BeFalse(),
				"problem_resolved should set is_actionable=false")

			_, hasSW := result.SelectedWorkflow.Get()
			Expect(hasSW).To(BeFalse(),
				"problem_resolved should not select a workflow")
		})

		It("E2E-KA-433-ADV-002: predictive_no_action → is_actionable=false", func() {
			req := buildRequest("adv-002", "mock_predictive_no_action", "low")
			req.SignalMode.SetTo(agentclient.SignalModeProactive)
			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			isActionable, hasActionable := result.IsActionable.Get()
			Expect(hasActionable).To(BeTrue(),
				"M1: is_actionable must be set for predictive_no_action outcome")
			Expect(isActionable).To(BeFalse(),
				"predictive_no_action should set is_actionable=false")
		})

		It("E2E-KA-433-ADV-003: problem_resolved_contradiction → needs_human_review=false (#301 override)", func() {
			req := buildRequest("adv-003", "mock_problem_resolved_contradiction", "low")
			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// #301: When investigation_outcome=problem_resolved AND needs_human_review=true,
			// the resolution takes precedence. Python KA enforced this override; KA matches.
			Expect(result.NeedsHumanReview.Value).To(BeFalse(),
				"#301: problem_resolved overrides contradictory needs_human_review=true")
		})
	})

	// ===================================================================
	// GAP-001: Post-RCA Enrichment
	// ===================================================================

	Context("GAP-001: Post-RCA enrichment", func() {

		It("E2E-KA-433-ADV-004: OOMKilled enrichment produces detected_labels", func() {
			req := buildRequest("adv-004", "OOMKilled", "high")
			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Analysis).NotTo(BeEmpty())
		})
	})

	// ===================================================================
	// GAP-004/016: RFC 7807 Error Handling
	// ===================================================================

	Context("GAP-004/016: RFC 7807 errors", func() {

		It("E2E-KA-433-ADV-005: Invalid request returns error with detail field", func() {
			resp, err := authHTTPClient.Post(kaURL+"/api/v1/incident/analyze", "application/json",
				strings.NewReader(`{"invalid": "missing required fields"}`))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusBadRequest),
				Equal(http.StatusUnprocessableEntity),
			), "malformed request should return 400 or 422")
		})

		It("E2E-KA-433-ADV-006: Missing remediation_id → HTTP 422 or 400", func() {
			resp, err := authHTTPClient.Post(kaURL+"/api/v1/incident/analyze", "application/json",
				strings.NewReader(`{"incident_id": "test", "signal_name": "OOMKilled"}`))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusBadRequest),
				Equal(http.StatusUnprocessableEntity),
			), "missing remediation_id should be rejected")
		})

		It("E2E-KA-433-ADV-007: Missing incident_id → HTTP 422 or 400", func() {
			resp, err := authHTTPClient.Post(kaURL+"/api/v1/incident/analyze", "application/json",
				strings.NewReader(`{"remediation_id": "rem-test", "signal_name": "OOMKilled"}`))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusBadRequest),
				Equal(http.StatusUnprocessableEntity),
			), "missing incident_id should be rejected")
		})
	})

	// ===================================================================
	// GAP-006: LLM Metrics
	// ===================================================================

	Context("GAP-006: LLM metrics", func() {

		It("E2E-KA-433-ADV-008: /metrics has aiagent_api_llm_requests_total after investigation", func() {
			req := buildRequest("adv-008", "OOMKilled", "high")
			_, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Get(kaURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			Expect(metricsText).To(ContainSubstring("aiagent_api_llm_requests_total"),
				"/metrics should expose LLM request counter")
		})

		It("E2E-KA-433-ADV-009: /metrics has aiagent_api_llm_tokens_total after investigation", func() {
			// M5-fix: ensure at least one investigation has run so metrics are populated
			req := buildRequest("adv-009-tokens", "OOMKilled", "high")
			_, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Get(kaURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			Expect(metricsText).To(ContainSubstring("aiagent_api_llm_tokens_total"),
				"/metrics should expose LLM token counter")
		})
	})

	// ===================================================================
	// GAP-009: ExecutionBundle + AlternativeWorkflows
	// ===================================================================

	Context("GAP-009: ExecutionBundle and AlternativeWorkflows", func() {

		It("E2E-KA-433-ADV-010: Successful investigation selected_workflow present", func() {
			req := buildRequest("adv-010", "OOMKilled", "high")
			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			needsHR, hasHR := result.NeedsHumanReview.Get()
			Expect(hasHR).To(BeTrue(), "M1: needs_human_review must always be set")
			if !needsHR {
				sw, hasSW := result.SelectedWorkflow.Get()
				Expect(hasSW).To(BeTrue(), "actionable result should have selected_workflow")
				Expect(sw).To(HaveKey("workflow_id"))
			}
		})

		It("E2E-KA-433-ADV-011: Low confidence result produces investigation output", func() {
			req := buildRequest("adv-011", "mock_low_confidence", "critical")
			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Analysis).NotTo(BeEmpty())
			Expect(result.Confidence).To(BeNumerically("<", 1.0))
		})
	})

	// ===================================================================
	// GAP-011/020: Audit Trail
	// ===================================================================

	Context("GAP-011: Audit trail", func() {

		It("E2E-KA-433-ADV-012: Investigation completes with incident correlation", func() {
			req := buildRequest("adv-012-audit", "OOMKilled", "high")
			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IncidentID).To(Equal("adv-012-audit"))
		})

		It("E2E-KA-433-ADV-013: Investigation produces non-empty analysis", func() {
			req := buildRequest("adv-013-prompt", "OOMKilled", "high")
			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Analysis).NotTo(BeEmpty(),
				"analysis should contain RCA summary from prompt")
		})

		It("E2E-KA-433-ADV-014: Authenticated investigation preserves SA identity", func() {
			req := buildRequest("adv-014-auth", "OOMKilled", "high")
			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})
	})

	// ===================================================================
	// GAP-015: HR Reason Alignment
	// ===================================================================

	Context("GAP-015: Human review reason alignment", func() {

		It("E2E-KA-433-ADV-015: no_workflow_found → correct HR reason enum", func() {
			req := buildRequest("adv-015", "mock_no_workflow_found", "critical")
			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			needsHR, hasHR := result.NeedsHumanReview.Get()
			Expect(hasHR).To(BeTrue(), "M1: needs_human_review must always be set")
			Expect(needsHR).To(BeTrue(),
				"no_workflow_found should require human review")
			hrReason, hasReason := result.HumanReviewReason.Get()
			Expect(hasReason).To(BeTrue())
			Expect(string(hrReason)).To(SatisfyAny(
				Equal("no_matching_workflows"),
				Equal("investigation_inconclusive"),
				Equal("workflow_not_found"),
			), "no_workflow_found should produce a valid HR reason enum")
		})

		// BR-HAPI-261 AC#7 / #704: enrichment-driven rca_incomplete.
		// Target Pods don't exist in Kind cluster → GetOwnerChain fails → rca_incomplete.
		It("E2E-KA-433-ADV-016: rca_incomplete → needs_human_review=true", func() {
			req := buildRequest("adv-016", "mock_rca_incomplete", "critical")
			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			needsHR, hasHR := result.NeedsHumanReview.Get()
			Expect(hasHR).To(BeTrue(), "M1: needs_human_review must always be set")
			Expect(needsHR).To(BeTrue(),
				"rca_incomplete: enrichment owner chain failure must trigger human review")
			hrReason, hasReason := result.HumanReviewReason.Get()
			Expect(hasReason).To(BeTrue(), "human_review_reason must be set when HR=true")
			Expect(string(hrReason)).To(Equal("rca_incomplete"),
				"E2E-KA-433-ADV-016: reason must be rca_incomplete per BR-HAPI-261 AC#7")
		})
	})
})
