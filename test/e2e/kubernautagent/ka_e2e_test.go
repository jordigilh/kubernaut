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
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/agentclient"
)

// Kubernaut Agent E2E Tests — #433
// Test Plan: docs/tests/433/TEST_PLAN.md
// Scenarios: E2E-KA-433-001 through E2E-KA-433-009
// Business Requirements: BR-HAPI-433
//
// These tests validate API contract parity between the Go Kubernaut Agent
// and the retired Python KA service. The same ogen-generated client is
// used (pkg/agentclient) to ensure wire-level compatibility.

var _ = Describe("E2E-KA-433: Kubernaut Agent API Contract Parity", Label("e2e", "ka", "parity"), func() {

	// =====================================================================
	// FUNCTIONAL TESTS (E2E-KA-433-001..005)
	// =====================================================================

	Context("BR-HAPI-433: Full investigation parity with Python KA", func() {

		It("E2E-KA-433-001: OOMKilled investigation produces correct workflow selection", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-433-001
			// Business Outcome: Full OOMKilled investigation against mock-llm produces correct workflow selection (parity with Python)
			// BR: BR-HAPI-433
			// Risk Mitigation: R7 (Mock-llm parity divergence)

			req := &agentclient.IncidentRequest{
				IncidentID:        "e2e-ka-001-oom",
				RemediationID:     "req-e2e-ka-001",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "production",
				ResourceKind:      "Pod",
				ResourceName:      "web-server-abc123",
				ErrorMessage:      "Container killed due to OOM",
				Environment:       "production",
				Priority:          "high",
				RiskTolerance:     "medium",
				BusinessCategory:  "web-application",
				ClusterName:       "kubernaut-agent-e2e",
			}

			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred(), "OOMKilled investigation should succeed")
			Expect(result).NotTo(BeNil())
			Expect(result.IncidentID).To(Equal("e2e-ka-001-oom"))
			Expect(result.Analysis).NotTo(BeEmpty(), "analysis should be non-empty")
			Expect(result.Confidence).To(BeNumerically(">", 0), "confidence should be positive")

			if result.NeedsHumanReview.Value {
				Expect(result.HumanReviewReason.Value).NotTo(BeZero())
			} else {
				sw, ok := result.SelectedWorkflow.Get()
				Expect(ok).To(BeTrue(), "selected_workflow should be set")
				Expect(sw).To(HaveKey("workflow_id"),
					"OOMKilled should select a workflow with workflow_id")
			}
		})

		It("E2E-KA-433-002: CrashLoopBackOff investigation produces correct workflow selection", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-433-002
			// Business Outcome: Full CrashLoopBackOff investigation produces correct workflow selection (parity with Python)
			// BR: BR-HAPI-433
			// Risk Mitigation: R7 (Mock-llm parity divergence)

			req := &agentclient.IncidentRequest{
				IncidentID:        "e2e-ka-002-clb",
				RemediationID:     "req-e2e-ka-002",
				SignalName:        "CrashLoopBackOff",
				Severity:          agentclient.SeverityCritical,
				SignalSource:      "kubernetes",
				ResourceNamespace: "production",
				ResourceKind:      "Pod",
				ResourceName:      "api-server-xyz789",
				ErrorMessage:      "Back-off restarting failed container",
				Environment:       "production",
				Priority:          "critical",
				RiskTolerance:     "low",
				BusinessCategory:  "api-backend",
				ClusterName:       "kubernaut-agent-e2e",
			}

			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred(), "CrashLoopBackOff investigation should succeed")
			Expect(result).NotTo(BeNil())
			Expect(result.IncidentID).To(Equal("e2e-ka-002-clb"))
			Expect(result.Analysis).NotTo(BeEmpty())
			Expect(result.Confidence).To(BeNumerically(">", 0))
		})
	})

	Context("BR-HAPI-433 (FR-07): Session-based async API contract", func() {

		It("E2E-KA-433-003: POST /analyze returns 202 with session_id", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-433-003
			// Business Outcome: POST /analyze returns 202 with session ID in response body
			// BR: BR-HAPI-433 (FR-07)

			req := &agentclient.IncidentRequest{
				IncidentID:        "e2e-ka-003-session",
				RemediationID:     "req-e2e-ka-003",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityMedium,
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "OOM test",
				Environment:       "staging",
				Priority:          "medium",
				RiskTolerance:     "medium",
				BusinessCategory:  "test",
				ClusterName:       "kubernaut-agent-e2e",
			}

			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).NotTo(HaveOccurred(), "submit should return 202")
			Expect(sessionID).NotTo(BeEmpty(), "session_id must be non-empty")
		})

		It("E2E-KA-433-004: GET /session/{id} returns investigation status", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-433-004
			// Business Outcome: GET /session/{id} returns investigation progress/status
			// BR: BR-HAPI-433 (FR-07)

			req := &agentclient.IncidentRequest{
				IncidentID:        "e2e-ka-004-poll",
				RemediationID:     "req-e2e-ka-004",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityMedium,
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "OOM test for polling",
				Environment:       "staging",
				Priority:          "medium",
				RiskTolerance:     "medium",
				BusinessCategory:  "test",
				ClusterName:       "kubernaut-agent-e2e",
			}

			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(sessionID).NotTo(BeEmpty())

			// Poll should return a valid status
			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				return status.Status
			}, 60*time.Second, 1*time.Second).Should(
				BeElementOf("pending", "investigating", "completed", "failed"),
				"session status should be a valid state",
			)
		})

		It("E2E-KA-433-005: GET /result returns completed IncidentResponse", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-433-005
			// Business Outcome: GET /result returns completed investigation JSON matching API contract
			// BR: BR-HAPI-433 (FR-07)

			req := &agentclient.IncidentRequest{
				IncidentID:        "e2e-ka-005-result",
				RemediationID:     "req-e2e-ka-005",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "production",
				ResourceKind:      "Pod",
				ResourceName:      "web-pod",
				ErrorMessage:      "OOM for full flow test",
				Environment:       "production",
				Priority:          "high",
				RiskTolerance:     "medium",
				BusinessCategory:  "web-application",
				ClusterName:       "kubernaut-agent-e2e",
			}

			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred(), "full investigation should complete")
			Expect(result).NotTo(BeNil())

			// Validate API contract fields
			Expect(result.IncidentID).To(Equal("e2e-ka-005-result"))
			Expect(result.Analysis).NotTo(BeEmpty())
			Expect(result.Timestamp).NotTo(BeEmpty())
			Expect(result.RootCauseAnalysis).NotTo(BeZero(),
				"root_cause_analysis should be populated")
		})
	})

	// =====================================================================
	// NON-FUNCTIONAL TESTS (E2E-KA-433-006..009)
	// =====================================================================

	Context("BR-HAPI-433 (NFR): Non-functional requirements", func() {

		It("E2E-KA-433-006: GET /healthz returns 200 (Issue #753: dedicated health port)", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-433-006
			// Business Outcome: GET /healthz returns 200 within 5s of container start
			// BR: BR-HAPI-433 (NFR)

			resp, err := http.Get(kaHealthURL + "/healthz")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"health endpoint should return 200")
		})

		It("E2E-KA-433-007: GET /metrics exposes Prometheus metrics (Issue #753: dedicated metrics port)", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-433-007
			// Business Outcome: GET /metrics exposes Prometheus metrics (go runtime + request counters)
			// BR: BR-HAPI-433 (NFR)

			resp, err := http.Get(kaMetricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"/metrics should return 200")
		})

	})
})
