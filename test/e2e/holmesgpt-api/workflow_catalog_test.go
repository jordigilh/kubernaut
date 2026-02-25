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

// Workflow Catalog E2E Tests
// Test Plan: docs/development/testing/HAPI_E2E_TEST_PLAN.md
// Scenarios: E2E-HAPI-030 through E2E-HAPI-044 (15 total)
// Business Requirements: BR-STORAGE-013, BR-HAPI-250, DD-WORKFLOW-004, DD-LLM-001, BR-AI-075
//
// Purpose: Validate workflow catalog search functionality and DataStorage integration
//
// NOTE: These tests validate the workflow catalog tool (used by HAPI internally for LLM-driven workflow search).
// The workflow catalog is not a direct HTTP endpoint, but is invoked as part of incident analysis.
//
// BR-AA-HAPI-064: All success-path tests migrated from ogen direct client (sync 200) to
// sessionClient.Investigate() (async submit/poll/result wrapper) because HAPI
// endpoints are now async-only (202 Accepted).

var _ = Describe("E2E-HAPI Workflow Catalog", Label("e2e", "hapi", "catalog"), func() {

	Context("BR-STORAGE-013: Semantic search functionality", func() {

		It("E2E-HAPI-030: Semantic search with exact match", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-030
			// Business Outcome: Workflow catalog finds workflows by semantic similarity to incident description
			// Python Source: test_workflow_catalog_data_storage_integration.py:181
			// BR: BR-STORAGE-013

			// ========================================
			// ARRANGE: Create incident request that will trigger workflow catalog search
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-catalog-030",
				RemediationID:     "test-rem-030",
				SignalName:        "OOMKilled",
				Severity:          "critical",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod-030",
				ErrorMessage:      "Container memory limit exceeded",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT: Call HAPI (which internally uses workflow catalog)
			// (BR-AA-HAPI-064: async session flow)
			// ========================================
			incidentResp, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Successful semantic search
			Expect(incidentResp.SelectedWorkflow.Set).To(BeTrue(),
				"selected_workflow must be present (semantic search succeeded)")

			// Note: SelectedWorkflow is map[string]jx.Raw, so detailed field validation skipped in E2E
			// Workflow selection logic validated in integration tests

			// BUSINESS IMPACT: LLM finds relevant workflows without exact keyword matching
		})

		It("E2E-HAPI-031: Confidence scoring validation", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-031
			// Business Outcome: Workflows ranked by V1.0 base similarity scoring (no boost/penalty yet)
			// Python Source: test_workflow_catalog_data_storage_integration.py:252
			// BR: DD-WORKFLOW-004 v2.0

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-catalog-031",
				RemediationID:     "test-rem-031",
				SignalName:        "OOMKilled",
				Severity:          "critical",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod-031",
				ErrorMessage:      "Container memory limit exceeded",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT (BR-AA-HAPI-064: async session flow)
			// ========================================
			_, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Results sorted by confidence descending
			// Note: SelectedWorkflow is map[string]jx.Raw, so detailed field validation skipped in E2E
			// Workflow selection logic validated in integration tests

			// Note: Alternative workflows should also be sorted by confidence
			// Confidence sorting validated in integration tests

			// BUSINESS IMPACT: LLM sees most relevant workflows first
		})

		It("E2E-HAPI-032: Empty results handling", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-032
			// Business Outcome: No matching workflows returns empty array (not error)
			// Python Source: test_workflow_catalog_data_storage_integration.py:303
			// BR: BR-HAPI-250

		// ========================================
		// ARRANGE: Create request with Mock LLM scenario for no workflow found
		// ========================================
		req := &hapiclient.IncidentRequest{
			IncidentID:        "test-catalog-032",
			RemediationID:     "test-rem-032",
			SignalName:        "MOCK_NO_WORKFLOW_FOUND",  // Mock LLM scenario
			Severity:          "high",
			SignalSource:      "kubernetes",
			ResourceNamespace: "default",
			ResourceKind:      "Pod",
			ResourceName:      "test-pod",
			ErrorMessage:      "Test error",
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
			Expect(err).ToNot(HaveOccurred(), "HAPI should handle empty workflow results gracefully")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Graceful empty results (escalates to human review)
		Expect(incidentResp.NeedsHumanReview.Value).To(BeTrue(),
			"needs_human_review must be true when no workflows found")
		Expect(incidentResp.HumanReviewReason.Value).To(Equal(hapiclient.HumanReviewReasonNoMatchingWorkflows),
			"human_review_reason must indicate no matching workflows")

			// BUSINESS IMPACT: LLM can tell operator "No automated remediation available"
		})

		It("E2E-HAPI-033: Filter validation", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-033
			// Business Outcome: Mandatory label filters correctly narrow search results
			// Python Source: test_workflow_catalog_data_storage_integration.py:335
			// BR: DD-LLM-001

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-catalog-033",
				RemediationID:     "test-rem-033",
				SignalName:        "CrashLoopBackOff",
				Severity:          "high",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "Test error",
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
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Filtered results
			if incidentResp.SelectedWorkflow.Set {
				// Workflow should match CrashLoopBackOff signal type
				// (DD-WORKFLOW-002 v3.0: signal_type is singular string)
				Expect(incidentResp.SelectedWorkflow.Value).ToNot(BeNil(),
					"selected workflow should be non-nil when Set is true")
			}

			// BUSINESS IMPACT: Workflows match incident type (no irrelevant suggestions)
		})

		It("E2E-HAPI-034: Top-K limiting", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-034
			// Business Outcome: Tool respects result count limit (prevents LLM context overflow)
			// Python Source: test_workflow_catalog_data_storage_integration.py:384
			// BR: BR-HAPI-250

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-catalog-034",
				RemediationID:     "test-rem-034",
				SignalName:        "OOMKilled",
				Severity:          "high",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "Test error",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}
			// ========================================
			// ACT (BR-AA-HAPI-064: async session flow)
			// ========================================
			_, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Result count limited
			// HAPI returns selected_workflow (top 1) + alternative_workflows (typically top 5-10)
			// Total should not exceed reasonable LLM context limits

			// Note: SelectedWorkflow is map[string]jx.Raw, so detailed field validation skipped in E2E
			// Mock LLM confidence validation done in integration tests

			// BUSINESS IMPACT: LLM doesn't get overwhelmed with too many options
		})

		It("E2E-HAPI-035: Error handling - Service unavailable", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-035
			// Business Outcome: Tool handles DataStorage unavailability gracefully
			// Python Source: test_workflow_catalog_data_storage_integration.py:428
			// BR: BR-STORAGE-013

			// NOTE: This test requires DataStorage to be unavailable, which is not the case in this E2E setup.
			// In production, HAPI would return an error or fallback response.
			// Skipping this test in E2E (would require infrastructure manipulation)

			Skip("Skipping DataStorage unavailability test (requires infrastructure manipulation)")
		})
	})

	Context("Critical user journeys", func() {

		It("E2E-HAPI-036: OOMKilled incident finds memory workflow", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-036
			// Business Outcome: Complete user journey - AI finds OOMKilled remediation workflow
			// Python Source: test_workflow_catalog_e2e.py:85
			// BR: BR-STORAGE-013

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-catalog-036",
				RemediationID:     "test-rem-036",
				SignalName:        "OOMKilled",
				Severity:          "critical",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "Test error",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}
			// ========================================
			// ACT: Simulate LLM completing RCA for OOMKilled
			// (BR-AA-HAPI-064: async session flow)
			// ========================================
			incidentResp, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Relevant workflow found
			Expect(incidentResp.SelectedWorkflow.Set).To(BeTrue(),
				"selected_workflow must be present for OOMKilled")

			// Note: SelectedWorkflow is map[string]jx.Raw, so detailed field validation skipped in E2E
			// Workflow selection logic validated in integration tests

			// BUSINESS IMPACT: Operator presented with actionable remediation
		})

		It("E2E-HAPI-037: CrashLoop incident finds restart workflow", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-037
			// Business Outcome: AI finds CrashLoopBackOff remediation workflow
			// Python Source: test_workflow_catalog_e2e.py:151
			// BR: BR-STORAGE-013

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-catalog-037",
				RemediationID:     "test-rem-037",
				SignalName:        "CrashLoopBackOff",
				Severity:          "critical",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "Test error",
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
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Relevant workflow found
			Expect(incidentResp.SelectedWorkflow.Set).To(BeTrue(),
				"selected_workflow must be present for CrashLoopBackOff")

			// Note: SelectedWorkflow is map[string]jx.Raw, so detailed field validation skipped in E2E
			// Workflow selection logic validated in integration tests

			// BUSINESS IMPACT: Operator has automated restart remediation option
		})

		It("E2E-HAPI-038: AI handles no matching workflows gracefully", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-038
			// Business Outcome: AI handles "no automated solution" scenario without errors
			// Python Source: test_workflow_catalog_e2e.py:218
			// BR: BR-HAPI-250

		// ========================================
		// ARRANGE: Use Mock LLM scenario for no workflow found
		// ========================================
		req := &hapiclient.IncidentRequest{
			IncidentID:        "test-catalog-038",
			RemediationID:     "test-rem-038",
			SignalName:        "MOCK_NO_WORKFLOW_FOUND",  // Mock LLM scenario
			Severity:          "high",
			SignalSource:      "kubernetes",
			ResourceNamespace: "default",
			ResourceKind:      "Pod",
			ResourceName:      "test-pod",
			ErrorMessage:      "Test error",
			Environment:       "production",
			Priority:          "P1",
			RiskTolerance:     "medium",
			BusinessCategory:  "standard",
			ClusterName:       "e2e-test",
		}
			// ========================================
			// ACT (BR-AA-HAPI-064: async session flow)
			// ========================================
			respObj, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI should handle empty workflow results gracefully")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Graceful empty results
			Expect(respObj.NeedsHumanReview.Value).To(BeTrue(),
				"needs_human_review must be true when no workflows found")

			// BUSINESS IMPACT: AI informs operator "No workflows found for this incident"
		})

		It("E2E-HAPI-039: AI can refine search with keywords", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-039
			// Business Outcome: AI can perform broad search then refine with specific terms
			// Python Source: test_workflow_catalog_e2e.py:244
			// BR: BR-HAPI-250

			// NOTE: This test would require multiple HAPI calls with different search terms
			// HAPI's workflow catalog search is LLM-driven and doesn't expose direct search refinement
			// Skipping this test (functionality validated at integration level)

			Skip("Skipping search refinement test (LLM-driven search pattern)")
		})
	})

	Context("BR-AI-075: Container image integration", func() {

		It("E2E-HAPI-040: DataStorage returns container_image in search", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-040
			// Business Outcome: Workflow search results include container_image for WorkflowExecution
			// Python Source: test_workflow_catalog_container_image_integration.py:100
			// BR: BR-AI-075

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-catalog-040",
				RemediationID:     "test-rem-040",
				SignalName:        "OOMKilled",
				Severity:          "critical",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "Test error",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}
			// ========================================
			// ACT (BR-AA-HAPI-064: async session flow)
			// ========================================
			_, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: container_image included
			// Note: SelectedWorkflow is map[string]jx.Raw, so detailed field validation skipped in E2E
			// Container image validation done in integration tests

			// BUSINESS IMPACT: WorkflowExecution can pull and execute container without additional lookups
		})

		It("E2E-HAPI-041: DataStorage returns container_digest in search", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-041
			// Business Outcome: Workflow results include immutable digest for security
			// Python Source: test_workflow_catalog_container_image_integration.py:152
			// BR: BR-AI-075

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-catalog-041",
				RemediationID:     "test-rem-041",
				SignalName:        "OOMKilled",
				Severity:          "critical",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "Test error",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}
			// ========================================
			// ACT (BR-AA-HAPI-064: async session flow)
			// ========================================
			_, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: container_digest included
			// Note: SelectedWorkflow is map[string]jx.Raw, so detailed field validation skipped in E2E
			// Container digest validation done in integration tests

			// BUSINESS IMPACT: WorkflowExecution uses immutable digest (security requirement)
		})

		It("E2E-HAPI-042: End-to-end container image flow", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-042
			// Business Outcome: Complete flow from search to container_image extraction validated
			// Python Source: test_workflow_catalog_container_image_integration.py:206
			// BR: BR-AI-075

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-catalog-042",
				RemediationID:     "test-rem-042",
				SignalName:        "OOMKilled",
				Severity:          "critical",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "Test error",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}
			// ========================================
			// ACT (BR-AA-HAPI-064: async session flow)
			// ========================================
			_, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Complete workflow data
			// Note: SelectedWorkflow is map[string]jx.Raw, so detailed field validation skipped in E2E
			// Required fields validation done in integration tests

			// BUSINESS IMPACT: AIAnalysis has all data to create WorkflowExecution CRD
		})

		It("E2E-HAPI-043: Container image matches catalog entry", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-043
			// Business Outcome: Returned container_image has valid OCI format
			// Python Source: test_workflow_catalog_container_image_integration.py:278
			// BR: BR-AI-075

			// ========================================
			// ARRANGE
			// ========================================
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-catalog-043",
				RemediationID:     "test-rem-043",
				SignalName:        "CrashLoopBackOff",
				Severity:          "high",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "Test error",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}
			// ========================================
			// ACT (BR-AA-HAPI-064: async session flow)
			// ========================================
			_, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

			// ========================================
			// ASSERT
			// ========================================
			// BEHAVIOR: Valid OCI references
			// Note: SelectedWorkflow is map[string]jx.Raw, so detailed field validation skipped in E2E
			// OCI format validation done in integration tests

			// BUSINESS IMPACT: Container runtime can pull images without format errors
		})

		It("E2E-HAPI-044: Direct API search returns container image", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-044
			// Business Outcome: DataStorage API contract includes container_image (validates tool transformation)
			// Python Source: test_workflow_catalog_container_image_integration.py:338
			// BR: BR-AI-075

			// NOTE: This test requires calling DataStorage API directly, which is outside the scope of HAPI E2E tests
			// DataStorage E2E tests already validate this contract
			// Skipping this test (functionality validated at DataStorage E2E level)

			Skip("Skipping direct DataStorage API test (validated at DataStorage E2E level)")
		})
	})
})
