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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	hapiclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// ========================================
// E2E-HAPI-017: Three-Step Workflow Discovery (DD-HAPI-017)
// ========================================
//
// Business Requirements:
//   - BR-HAPI-017-001: Three-step tool implementation
//   - BR-HAPI-017-004: Recovery validation loop with three-step protocol
//
// Design Decisions:
//   - DD-HAPI-017: Three-Step Workflow Discovery Integration
//   - DD-WORKFLOW-016: Action-Type Workflow Catalog Indexing
//
// Test Strategy:
//   The three-step discovery protocol (list_available_actions → list_workflows → get_workflow)
//   is transparent to API callers. The HAPI Python toolset handles the multi-turn tool call
//   loop internally. Mock LLM is programmed to follow the three-step sequence when it detects
//   the discovery tools in the available tools list.
//
//   These tests verify that the full stack (HAPI → Mock LLM → DS) works with the new protocol
//   by exercising incident and recovery flows that trigger workflow discovery.

var _ = Describe("E2E-HAPI-017: Three-Step Workflow Discovery", Label("e2e", "hapi", "discovery", "three-step"), func() {

	Context("BR-HAPI-017-001: Incident flow with three-step discovery", func() {

		It("E2E-HAPI-017-001-001: Incident analysis uses three-step discovery to select workflow", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-017-001-001
			// Business Outcome: Full incident analysis uses three-step discovery with Mock LLM.
			//   Mock LLM calls list_available_actions → list_workflows → get_workflow,
			//   HAPI returns a valid investigation result with selected workflow.
			// BR: BR-HAPI-017-001
			// Phase: 11 (DD-HAPI-017 Implementation Plan)

			// ========================================
			// ARRANGE
			// ========================================
			// OOMKilled signal triggers the "oomkilled" Mock LLM scenario.
			// Mock LLM three-step flow:
			//   Step 1: list_available_actions → DS returns action types including "IncreaseMemoryLimits"
			//   Step 2: list_workflows(action_type="IncreaseMemoryLimits") → DS returns workflows
			//   Step 3: get_workflow(workflow_id=<oomkill-increase-memory-v1 UUID>) → DS returns full detail
			//   Step 4: Final analysis with selected_workflow
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-discovery-017-001",
				RemediationID:     "test-rem-017-001",
				SignalName:        "OOMKilled",
				Severity:          "critical",
				SignalSource:      "prometheus",
				ResourceNamespace: "production",
				ResourceKind:      "Pod",
				ResourceName:      "api-server-abc123",
				ErrorMessage:      "Container memory limit exceeded - testing three-step discovery",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT (BR-AA-HAPI-064: async session flow)
			// ========================================
			incidentResp, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis should succeed with three-step discovery")

			// ========================================
			// ASSERT
			// ========================================

			// BEHAVIOR: Workflow selected via three-step discovery
			Expect(incidentResp.SelectedWorkflow.Set).To(BeTrue(),
				"selected_workflow must be present — three-step discovery should find oomkill-increase-memory-v1")

			// CORRECTNESS: Confident recommendation (Mock LLM oomkilled scenario returns 0.95)
			Expect(incidentResp.Confidence).To(BeNumerically("~", 0.95, 0.10),
				"Confidence should be ~0.95 for OOMKilled scenario via three-step discovery")

			// BEHAVIOR: No human review needed for confident recommendation
			Expect(incidentResp.NeedsHumanReview.Value).To(BeFalse(),
				"needs_human_review must be false when three-step discovery finds a confident workflow")

			// CORRECTNESS: Analysis contains RCA
			Expect(incidentResp.Analysis).ToNot(BeEmpty(),
				"Analysis text should be populated from Mock LLM final response")

			logger.Info("✅ E2E-HAPI-017-001-001: Incident three-step discovery PASSED",
				"incident_id", incidentResp.IncidentID,
				"confidence", incidentResp.Confidence,
				"selected_workflow_set", incidentResp.SelectedWorkflow.Set)
		})

		It("E2E-HAPI-017-001-001b: CrashLoop incident also uses three-step discovery", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-017-001-001b (variant)
			// Business Outcome: Different signal type also works with three-step discovery.
			// BR: BR-HAPI-017-001

			// ========================================
			// ARRANGE
			// ========================================
			// CrashLoopBackOff triggers the "crashloop" Mock LLM scenario.
			// Three-step: list_available_actions → list_workflows(RestartDeployment) → get_workflow
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-discovery-017-001b",
				RemediationID:     "test-rem-017-001b",
				SignalName:        "CrashLoopBackOff",
				Severity:          "high",
				SignalSource:      "kubernetes",
				ResourceNamespace: "staging",
				ResourceKind:      "Pod",
				ResourceName:      "worker-pod-xyz",
				ErrorMessage:      "Container failing due to config error - testing three-step variant",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT
			// ========================================
			incidentResp, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis should succeed for CrashLoop via three-step")

			// ========================================
			// ASSERT
			// ========================================
			Expect(incidentResp.SelectedWorkflow.Set).To(BeTrue(),
				"selected_workflow must be present for CrashLoop via three-step discovery")
			Expect(incidentResp.Confidence).To(BeNumerically("~", 0.88, 0.10),
				"Confidence should be ~0.88 for CrashLoop scenario")

			logger.Info("✅ E2E-HAPI-017-001-001b: CrashLoop three-step discovery PASSED",
				"incident_id", incidentResp.IncidentID,
				"confidence", incidentResp.Confidence)
		})
	})

	Context("BR-HAPI-017-001: Recovery flow with three-step discovery", func() {

		// Helper to create previous execution context for recovery tests
		createPreviousExecution := func(failedWorkflowID, failureReason string) hapiclient.PreviousExecution {
			return hapiclient.PreviousExecution{
				WorkflowExecutionRef: "workflow-exec-" + failedWorkflowID,
				OriginalRca: hapiclient.OriginalRCA{
					Summary:             "Initial OOMKilled RCA — memory limit exceeded",
					SignalName:          "OOMKilled",
					Severity:            "critical",
					ContributingFactors: []string{"memory_exhaustion", "traffic_spike"},
				},
				SelectedWorkflow: hapiclient.SelectedWorkflowSummary{
					WorkflowID:     failedWorkflowID,
					Version:        "1.0.0",
					ExecutionBundle: "ghcr.io/kubernaut/workflows/oomkill-increase-memory:v1.0.0",
					Rationale:      "Selected for initial remediation via three-step discovery",
				},
				Failure: hapiclient.ExecutionFailure{
					FailedStepIndex: 0,
					FailedStepName:  "increase-memory-step",
					Reason:          failureReason,
					Message:         "Workflow execution failed: " + failureReason,
					FailedAt:        "2026-02-12T10:00:00Z",
					ExecutionTime:   "1m45s",
				},
			}
		}

		It("E2E-HAPI-017-001-002: Recovery analysis uses three-step discovery for alternative workflow", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-017-001-002
			// Business Outcome: Recovery analysis uses three-step discovery to find
			//   alternative workflow after initial attempt failed.
			// BR: BR-HAPI-017-001, BR-AI-080, BR-AI-081

			// ========================================
			// ARRANGE
			// ========================================
			prevExec := createPreviousExecution("workflow-oomkill-failed", "resource_limit_too_low")

			req := &hapiclient.RecoveryRequest{
				IncidentID:            "test-recovery-017-002",
				RemediationID:         "test-rem-017-002",
				SignalName:            hapiclient.NewOptNilString("OOMKilled"),
				Severity:              hapiclient.NewOptNilSeverity(hapiclient.SeverityCritical),
				IsRecoveryAttempt:     hapiclient.NewOptBool(true),
				RecoveryAttemptNumber: hapiclient.NewOptNilInt(2),
				PreviousExecution:     hapiclient.NewOptNilPreviousExecution(prevExec),
			}

			// ========================================
			// ACT
			// ========================================
			recoveryResp, err := sessionClient.InvestigateRecovery(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI recovery analysis should succeed with three-step discovery")

			// ========================================
			// ASSERT
			// ========================================

			// BEHAVIOR: Recovery finds alternative workflow
			Expect(recoveryResp.IncidentID).To(Equal("test-recovery-017-002"))
			Expect(recoveryResp.CanRecover).To(BeTrue(),
				"Recovery should be possible — three-step discovery finds alternative workflow")

			// BEHAVIOR: Recovery analysis present
			Expect(recoveryResp.RecoveryAnalysis.Set).To(BeTrue(),
				"recovery_analysis must be present (BR-AI-081)")

			// CORRECTNESS: Selected workflow from three-step discovery
			Expect(recoveryResp.SelectedWorkflow.Set).To(BeTrue(),
				"selected_workflow must be present — three-step discovery finds alternative")
			Expect(recoveryResp.AnalysisConfidence).To(BeNumerically("~", 0.85, 0.10),
				"Recovery confidence should be ~0.85 for alternative workflow")

			logger.Info("✅ E2E-HAPI-017-001-002: Recovery three-step discovery PASSED",
				"incident_id", recoveryResp.IncidentID,
				"can_recover", recoveryResp.CanRecover,
				"confidence", recoveryResp.AnalysisConfidence)
		})
	})
})
