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

package holmesgptapi

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	hapiclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// BR-AA-HAPI-064: All success-path tests migrated from ogen direct client (sync 200) to
// sessionClient.Investigate() (async submit/poll/result wrapper) because HAPI
// endpoints are now async-only (202 Accepted).

// Predictive Signal Mode E2E Tests
// Test Plan: docs/development/testing/HAPI_E2E_TEST_PLAN.md (Category G)
// Scenarios: E2E-HAPI-055 through E2E-HAPI-057 (3 total)
// Business Requirements: BR-AI-084 (Predictive Signal Mode Prompt Strategy)
// Architecture: ADR-054 (Predictive Signal Mode Classification)
//
// Purpose: Validate HAPI correctly adapts the investigation prompt for predictive
// signal mode and returns appropriate predictive-aware analysis results.
//
// Mock LLM Scenarios:
//   - oomkilled_predictive: Triggered by predictive keywords + "oomkilled" in prompt
//   - Standard oomkilled: Triggered by "oomkilled" without predictive keywords

var _ = Describe("E2E-HAPI-084: Predictive Signal Mode Investigation", Label("e2e", "hapi", "signalmode"), func() {

	Context("BR-AI-084: Predictive signal mode prompt adaptation", func() {

		It("E2E-HAPI-055: Predictive OOMKill returns predictive-aware analysis", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-055
			// Business Outcome: When signal_mode=predictive, HAPI adapts its 5-phase investigation
			//   prompt to perform preemptive analysis. Mock LLM detects predictive keywords and
			//   returns the oomkilled_predictive scenario with prevention-focused root cause.
			// Mock LLM Scenario: oomkilled_predictive (server.py:226)
			// BR: BR-AI-084, ADR-054

			// ========================================
			// ARRANGE: Create request with signal_mode=predictive
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-predictive-055",
				RemediationID:     "test-rem-predictive-055",
				SignalType:        "OOMKilled", // Normalized by SP from PredictedOOMKill (ADR-054)
				Severity:          "critical",
				SignalSource:      "prometheus",
				ResourceNamespace: "production",
				ResourceKind:      "Pod",
				ResourceName:      "api-server-abc123",
				ErrorMessage:      "Predicted memory exhaustion based on trend analysis",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}
			// BR-AI-084: Set signal_mode to predictive
			req.SignalMode = hapiclient.NewOptNilSignalMode(
				hapiclient.SignalModePredictive,
			)

			// ========================================
			// ACT: Call HAPI incident analysis endpoint (BR-AA-HAPI-064: async session flow)
			// ========================================
			incidentResp, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

			// ========================================
			// ASSERT: Business outcome validation
			// ========================================
			// BEHAVIOR: Analysis should complete successfully
			Expect(len(incidentResp.Analysis) > 0).To(BeTrue(),
				"Predictive analysis should produce non-empty analysis text")

			// CORRECTNESS: Mock LLM oomkilled_predictive scenario returns confidence = 0.88
			Expect(incidentResp.Confidence).To(BeNumerically("~", 0.88, 0.05),
				"Mock LLM 'oomkilled_predictive' scenario returns confidence = 0.88 ± 0.05 (server.py:233)")

			// CORRECTNESS: Workflow should be selected (oomkill-increase-memory-v1)
			Expect(incidentResp.SelectedWorkflow.Set).To(BeTrue(),
				"selected_workflow must be set for predictive OOMKill scenario")
			Expect(incidentResp.SelectedWorkflow.Null).To(BeFalse(),
				"selected_workflow must not be null for predictive OOMKill scenario")

			// CORRECTNESS: Analysis should reference prediction/prevention language
			// The Mock LLM oomkilled_predictive root_cause contains:
			// "Predicted OOMKill based on memory utilization trend analysis (predict_linear).
			//  Current memory usage is 85% of limit and growing at 50MB/min.
			//  Preemptive action recommended..."
			Expect(incidentResp.Analysis).To(
				SatisfyAny(
					ContainSubstring("Predict"),
					ContainSubstring("predict"),
					ContainSubstring("Preemptive"),
					ContainSubstring("preemptive"),
					ContainSubstring("trend"),
				),
				"Analysis should reference predictive/preemptive language (not standard RCA)")

			// BUSINESS IMPACT: AIAnalysis controller adapts phase handling for predictive signals
			// - Uses same workflow catalog as reactive (SP normalizes signal type)
			// - Prompt includes predictive investigation strategy
			// - Audit trail records signal_mode=predictive
		})
	})

	Context("BR-AI-084: Reactive signal mode (backwards compatibility)", func() {

		It("E2E-HAPI-056: Reactive signal mode returns standard RCA analysis", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-056
			// Business Outcome: Existing reactive requests continue working unchanged.
			//   signal_mode=reactive produces standard RCA results without predictive language.
			// Mock LLM Scenario: oomkilled (standard reactive scenario)
			// BR: BR-AI-084 (backwards compatibility)

			// ========================================
			// ARRANGE: Create request with explicit reactive signal mode
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-reactive-056",
				RemediationID:     "test-rem-reactive-056",
				SignalType:        "OOMKilled",
				Severity:          "critical",
				SignalSource:      "prometheus",
				ResourceNamespace: "production",
				ResourceKind:      "Pod",
				ResourceName:      "worker-pod-def456",
				ErrorMessage:      "Container killed due to OOM",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}
			// Set explicit reactive mode
			req.SignalMode = hapiclient.NewOptNilSignalMode(
				hapiclient.SignalModeReactive,
			)

			// ========================================
			// ACT: Call HAPI incident analysis endpoint (BR-AA-HAPI-064: async session flow)
			// ========================================
			incidentResp, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

			// ========================================
			// ASSERT: Business outcome validation
			// ========================================
			// BEHAVIOR: Standard reactive response
			Expect(len(incidentResp.Analysis) > 0).To(BeTrue(),
				"Reactive analysis should produce non-empty analysis text")

			// CORRECTNESS: Standard oomkilled scenario confidence
			Expect(incidentResp.Confidence).To(BeNumerically(">", 0.0),
				"Reactive analysis should have positive confidence")

			// CORRECTNESS: Workflow should be selected (standard oomkilled scenario)
			Expect(incidentResp.SelectedWorkflow.Set).To(BeTrue(),
				"selected_workflow must be set for reactive OOMKilled scenario")

			// BUSINESS IMPACT: Existing reactive flow unchanged by ADR-054
		})

		It("E2E-HAPI-057: Missing signal mode defaults to reactive behavior", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-057
			// Business Outcome: Requests without signal_mode should default to reactive behavior.
			//   Ensures backwards compatibility with pre-ADR-054 clients.
			// Mock LLM Scenario: oomkilled (standard reactive scenario - no predictive keywords in prompt)
			// BR: BR-AI-084 (default behavior)

			// ========================================
			// ARRANGE: Request without setting signal_mode
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-default-057",
				RemediationID:     "test-rem-default-057",
				SignalType:        "OOMKilled",
				Severity:          "critical",
				SignalSource:      "prometheus",
				ResourceNamespace: "production",
				ResourceKind:      "Pod",
				ResourceName:      "app-pod-ghi789",
				ErrorMessage:      "Container exceeded memory limit",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}
			// signal_mode intentionally NOT set — defaults to reactive

			// ========================================
			// ACT: Call HAPI incident analysis endpoint (BR-AA-HAPI-064: async session flow)
			// ========================================
			incidentResp, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

			// ========================================
			// ASSERT: Business outcome validation
			// ========================================
			// BEHAVIOR: Should behave same as explicit reactive mode
			Expect(len(incidentResp.Analysis) > 0).To(BeTrue(),
				"Default (no signal_mode) should produce non-empty analysis like reactive")

			// CORRECTNESS: Positive confidence (standard scenario)
			Expect(incidentResp.Confidence).To(BeNumerically(">", 0.0),
				"Default analysis should have positive confidence")

			// CORRECTNESS: Workflow should be selected (standard oomkilled scenario)
			Expect(incidentResp.SelectedWorkflow.Set).To(BeTrue(),
				"selected_workflow must be set for default (reactive) OOMKilled scenario")

			// BUSINESS IMPACT: Pre-ADR-054 clients continue working without modification
		})
	})
})
