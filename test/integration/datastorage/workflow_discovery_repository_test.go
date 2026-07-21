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

package datastorage

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
)

// ========================================
// WORKFLOW DISCOVERY REPOSITORY INTEGRATION TESTS
// ========================================
//
// Authority: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing)
// Authority: DD-HAPI-017 (Three-Step Workflow Discovery Integration)
// Business Requirement: BR-HAPI-017-001 (Three-Step Tool Implementation)
//
// Test Plan: docs/testing/DD-HAPI-017/TEST_PLAN.md
// Test IDs: IT-DS-017-001-001, IT-DS-017-001-003, IT-DS-017-001-005/006,
// IT-DS-464-001 through IT-DS-464-006, IT-DS-522-001 through IT-DS-522-003
//
// #1661 Phase F: migrated off workflowRepo.Create (Postgres, zero production
// callers post-Phase-B) to seedWorkflowCRD -- DD-WORKFLOW-018 (etcd sole
// source of truth).
//
// Two categories of scenario were removed as obsolete rather than migrated:
//
//   - IT-DS-017-001-002 (pagination) survives unchanged in spirit but note the
//     historical "GAP-WF-3" scenarios ("ListActions/ListWorkflowsByActionType
//     returns only latest version counts") were deleted: they depended on
//     creating two coexisting rows for the same workflow_name at different
//     versions and asserting only the "latest" is counted. DD-WORKFLOW-018
//     makes metadata.name the workflow's sole identity -- there is no
//     "version" dimension left to disambiguate, so the scenario has no
//     CRD-native equivalent (there is only ever one live object per name).
//
//   - IT-DS-017-001-004 ("excludes disabled workflows") was deleted: a
//     RemediationWorkflow CRD is Always Active once admitted (mirrors the
//     E2E-DS-017-001-002 deletion precedent) -- infrastructure.
//     SeedWorkflowContentViaDirectCRDCreation always status-patches
//     CatalogStatusActive, the same as AuthWebhook's real admission path, so
//     there is no way to construct a "Disabled" workflow to exercise the
//     exclusion in the first place. IT-DS-017-001-001 below was adjusted to
//     drop its disabled-workflow leg for the same reason.
// ========================================

// Serial: several scenarios below assert exact global counts (ListActions
// totalCount/WorkflowCount) with no per-spec prefix-scoping of results, unlike
// e.g. workflow_detected_labels_cnv_test.go's filterOurs pattern. Must not run
// concurrently (across any parallel process) with other specs that create
// RemediationWorkflow CRDs in the shared workflowCRDNamespace.
var _ = Describe("Workflow Discovery Repository Integration Tests", Serial, func() {
	var (
		workflowRepo *workflow.Repository
		testID       string
	)

	BeforeEach(func() {
		workflowRepo = newCachedWorkflowRepo()
		testID = generateTestID()
	})

	// ========================================
	// HELPER: Create a test workflow with action_type
	// ========================================
	createTestWorkflow := func(name, actionType, severity, component, environment, priority string) string {
		return seedWorkflowCRD(workflowCRDSpec{
			Name:        testID + "-" + name,
			ActionType:  actionType,
			Severity:    []string{severity},
			Component:   []string{component},
			Environment: []string{environment},
			Priority:    priority,
		})
	}

	// ========================================
	// IT-DS-017-001-001: ListActions -- active status filter
	// ========================================
	Describe("ListActions", func() {
		Context("IT-DS-017-001-001: active status filter", func() {
			It("should return only action types that have active workflows", func() {
				// Arrange: 2 active ScaleReplicas workflows (see file header: the
				// disabled-workflow leg was dropped -- CRD-native seeding can only
				// produce Active workflows, matching production reality).
				createTestWorkflow("scale-1", "ScaleReplicas", "critical", "v1/Pod", "production", "P0")
				createTestWorkflow("scale-2", "ScaleReplicas", "high", "apps/v1/Deployment", "staging", "P1")

				// Act: List available actions (no specific context filters to get all)
				filters := &models.WorkflowDiscoveryFilters{}
				result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1), "Only ScaleReplicas should have active workflows")
				Expect(result).To(HaveLen(1))
				Expect(result[0].ActionType).To(Equal("ScaleReplicas"))
				Expect(result[0].WorkflowCount).To(Equal(2))
			})
		})

		// ========================================
		// IT-DS-017-001-002: ListActions -- pagination
		// ========================================
		Context("IT-DS-017-001-002: pagination returns correct slice", func() {
			It("should paginate action types correctly", func() {
				// Arrange: Create workflows spanning 5 action types (all active) - DD-WORKFLOW-016 V1.0
				createTestWorkflow("scale-wf", "ScaleReplicas", "critical", "v1/Pod", "production", "P0")
				createTestWorkflow("restart-wf", "RestartPod", "critical", "v1/Pod", "production", "P0")
				createTestWorkflow("rollback-wf", "RollbackDeployment", "critical", "v1/Pod", "production", "P0")
				createTestWorkflow("memory-wf", "IncreaseMemoryLimits", "critical", "v1/Pod", "production", "P0")
				createTestWorkflow("restart-deploy-wf", "RestartDeployment", "critical", "v1/Pod", "production", "P0")

				filters := &models.WorkflowDiscoveryFilters{}

				// Act: First page (3 items)
				result1, totalCount1, err := workflowRepo.ListActions(ctx, filters, 0, 3)
				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount1).To(Equal(5))
				Expect(result1).To(HaveLen(3))

				// Act: Second page (remaining 2 items)
				result2, totalCount2, err := workflowRepo.ListActions(ctx, filters, 3, 3)
				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount2).To(Equal(5))
				Expect(result2).To(HaveLen(2))

				// Verify no overlap between pages
				page1Types := make(map[string]bool)
				for _, entry := range result1 {
					page1Types[entry.ActionType] = true
				}
				for _, entry := range result2 {
					Expect(page1Types).ToNot(HaveKey(entry.ActionType), "Pages should not overlap")
				}
			})
		})
	})

	// ========================================
	// IT-DS-017-001-003: ListWorkflowsByActionType -- filters by action_type + context
	// ========================================
	Describe("ListWorkflowsByActionType", func() {
		Context("IT-DS-017-001-003: filters by action_type AND signal context", func() {
			It("should return only workflows matching both action_type and context filters", func() {
				// Arrange
				createTestWorkflow("scale-conservative", "ScaleReplicas", "critical", "v1/Pod", "production", "P0")
				createTestWorkflow("scale-aggressive", "ScaleReplicas", "high", "apps/v1/Deployment", "staging", "P1")
				createTestWorkflow("restart-simple", "RestartPod", "critical", "v1/Pod", "production", "P0")

				// Act: Filter for ScaleReplicas + severity=critical
				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "v1/Pod",
					Environment: "production",
					Priority:    "P0",
				}
				results, totalCount, err := workflowRepo.ListWorkflowsByActionType(ctx, "ScaleReplicas", filters, 0, 10)

				// Assert: Only scale-conservative matches (action_type=ScaleReplicas AND severity=critical)
				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1))
				Expect(results).To(HaveLen(1))
				Expect(results[0].Name).To(Equal(testID + "-scale-conservative"))
			})
		})
	})

	// ========================================
	// IT-DS-017-001-005: GetWorkflowWithContextFilters -- context match
	// ========================================
	Describe("GetWorkflowWithContextFilters", func() {
		Context("IT-DS-017-001-005: context match returns workflow", func() {
			It("should return workflow when context filters match", func() {
				// Arrange
				workflowID := createTestWorkflow("scale-match", "ScaleReplicas", "critical", "v1/Pod", "production", "P0")

				// Act
				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "v1/Pod",
					Environment: "production",
					Priority:    "P0",
				}
				result, err := workflowRepo.GetWorkflowWithContextFilters(ctx, workflowID, filters)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(result.WorkflowID).To(Equal(workflowID))
				Expect(result.ActionType).To(Equal("ScaleReplicas"))
			})
		})

		// ========================================
		// IT-DS-017-001-006: GetWorkflowWithContextFilters -- context mismatch returns nil
		// ========================================
		Context("IT-DS-017-001-006: context mismatch returns nil (security gate)", func() {
			It("should return workflow.ErrNotFound when context filters do not match", func() {
				// Arrange: workflow with severity=critical, environment=production
				workflowID := createTestWorkflow("scale-mismatch", "ScaleReplicas", "critical", "v1/Pod", "production", "P0")

				// Act: Query with mismatching context (severity=high, environment=staging)
				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "high",
					Component:   "apps/v1/Deployment",
					Environment: "staging",
					Priority:    "P1",
				}
				result, err := workflowRepo.GetWorkflowWithContextFilters(ctx, workflowID, filters)

				// Assert: Security gate -- no workflow returned. Issue #1674: the
				// repository signals "not found OR filtered out" via the
				// workflow.ErrNotFound sentinel instead of the ambiguous (nil, nil)
				// return; the two cases are still deliberately not distinguished
				// from each other to prevent information leakage (DD-WORKFLOW-016).
				Expect(errors.Is(err, workflow.ErrNotFound)).To(BeTrue(), "Security gate should surface ErrNotFound for mismatched workflow")
				Expect(result).To(BeNil(), "Security gate should prevent returning mismatched workflow")
			})
		})
	})

	// ========================================
	// ISSUE #464: WILDCARD MANDATORY LABEL MATCHING
	// ========================================
	//
	// Authority: DD-WORKFLOW-001 v2.8, DD-WORKFLOW-016 v2.1
	// Business Requirement: BR-HAPI-017-001
	// Test Plan: docs/tests/464/TEST_PLAN.md
	// Test IDs: IT-DS-464-001 through IT-DS-464-006
	//
	// These tests validate that workflows using wildcard ("*") values in
	// mandatory labels are correctly matched by the three-step discovery
	// protocol against the cache-backed discovery path.
	// ========================================

	createTestWorkflowWithArrayLabels := func(name, actionType string, severity []string, component string, environment []string, priority string) string {
		return seedWorkflowCRD(workflowCRDSpec{
			Name:        testID + "-" + name,
			ActionType:  actionType,
			Severity:    severity,
			Component:   []string{component},
			Environment: environment,
			Priority:    priority,
		})
	}

	Describe("ListActions - Wildcard Labels (#464)", func() {
		// ========================================
		// IT-DS-464-001: ListActions matches wildcard component + priority
		// ========================================
		Context("IT-DS-464-001: wildcard component + priority", func() {
			It("should match a workflow with component='*' and priority='*' when queried with specific values", func() {
				createTestWorkflowWithArrayLabels("wc-comp-pri", "ScaleReplicas",
					[]string{"critical"}, "*", []string{"production"}, "*")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "v1/Pod",
					Environment: "production",
					Priority:    "P1",
				}
				result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1), "IT-DS-464-001: wildcard component/priority workflow must be discovered")
				Expect(result).To(HaveLen(1))
				Expect(result[0].ActionType).To(Equal("ScaleReplicas"))
				Expect(result[0].WorkflowCount).To(BeNumerically(">=", 1))
			})
		})

		// ========================================
		// IT-DS-464-002: ListActions matches all-wildcard workflow
		// ========================================
		Context("IT-DS-464-002: all-wildcard mandatory labels", func() {
			It("should match a fully wildcarded workflow for any combination of filter values", func() {
				createTestWorkflowWithArrayLabels("wc-all", "ScaleReplicas",
					[]string{"*"}, "*", []string{"*"}, "*")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "high",
					Component:   "apps/v1/Deployment",
					Environment: "staging",
					Priority:    "P3",
				}
				result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1), "IT-DS-464-002: all-wildcard workflow must be discoverable with any filter values")
				Expect(result).To(HaveLen(1))
				Expect(result[0].ActionType).To(Equal("ScaleReplicas"))
			})
		})
	})

	// ========================================
	// IT-DS-464-003: ListWorkflowsByActionType returns wildcard-labeled workflow
	// ========================================
	Describe("ListWorkflowsByActionType - Wildcard Labels (#464)", func() {
		Context("IT-DS-464-003: wildcard labels in Step 2 discovery", func() {
			It("should return a wildcard-labeled workflow when queried with specific filter values", func() {
				createTestWorkflowWithArrayLabels("wc-step2", "ScaleReplicas",
					[]string{"critical"}, "*", []string{"production"}, "*")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "v1/Pod",
					Environment: "production",
					Priority:    "P1",
				}
				results, totalCount, err := workflowRepo.ListWorkflowsByActionType(ctx, "ScaleReplicas", filters, 0, 10)

				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1), "IT-DS-464-003: wildcard workflow must appear in Step 2 results")
				Expect(results).To(HaveLen(1))
				Expect(results[0].ActionType).To(Equal("ScaleReplicas"))
			})
		})
	})

	// ========================================
	// IT-DS-464-004: GetWorkflowWithContextFilters passes security gate for wildcard workflow
	// ========================================
	Describe("GetWorkflowWithContextFilters - Wildcard Labels (#464)", func() {
		Context("IT-DS-464-004: security gate passes for wildcard workflow", func() {
			It("should return the workflow (not nil) when wildcard labels match the query context", func() {
				workflowID := createTestWorkflowWithArrayLabels("wc-gate", "ScaleReplicas",
					[]string{"critical"}, "*", []string{"production"}, "*")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "v1/Pod",
					Environment: "production",
					Priority:    "P1",
				}
				result, err := workflowRepo.GetWorkflowWithContextFilters(ctx, workflowID, filters)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil(), "IT-DS-464-004: security gate must not reject wildcard-labeled workflow")
				Expect(result.WorkflowID).To(Equal(workflowID))
			})
		})
	})

	// ========================================
	// IT-DS-464-005: Demo scenario — mixed wildcards + exact labels
	// ========================================
	Describe("Demo Scenario - Mixed Wildcards (#464)", func() {
		Context("IT-DS-464-005: exact demo scenario from issue #464", func() {
			It("should match a workflow with mixed wildcard and exact labels", func() {
				createTestWorkflowWithArrayLabels("wc-demo", "IncreaseMemoryLimits",
					[]string{"critical", "high"}, "*", []string{"production", "staging", "*"}, "*")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "v1/Pod",
					Environment: "staging",
					Priority:    "P1",
				}
				result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1), "IT-DS-464-005: the demo scenario workflow must be discovered (was 0 in the bug)")
				Expect(result).To(HaveLen(1))
				Expect(result[0].ActionType).To(Equal("IncreaseMemoryLimits"))
			})
		})
	})

	// ========================================
	// IT-DS-464-006: Severity wildcard in JSONB array matches via ? operator
	// ========================================
	Describe("Severity Wildcard JSONB Matching (#464)", func() {
		Context("IT-DS-464-006: severity ['*'] matches via PostgreSQL JSONB ? operator", func() {
			It("should match severity=['*'] when queried with severity=critical", func() {
				createTestWorkflowWithArrayLabels("wc-sev", "RestartPod",
					[]string{"*"}, "v1/Pod", []string{"production"}, "P0")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "v1/Pod",
					Environment: "production",
					Priority:    "P0",
				}
				result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1), "IT-DS-464-006: severity ['*'] must match query severity=critical via JSONB ? operator")
				Expect(result).To(HaveLen(1))
				Expect(result[0].ActionType).To(Equal("RestartPod"))
			})
		})
	})

	// ========================================
	// Issue #522: Wildcard labels return 0 results
	// ========================================
	// Authority: Issue #522 (list_available_actions returns 0 when workflow labels use wildcards)
	// Reproduction: Exact label values and query filters from the bug report.
	// The workflow uses mixed exact + wildcard labels; the query uses values that
	// should match via wildcard on component, environment, and priority.
	// ========================================

	Describe("ListActions - Issue #522 Reproduction", func() {
		Context("IT-DS-522-001: mixed exact severity + wildcard component/environment/priority", func() {
			It("should match a workflow with severity=[critical,high], component='*', environment=['*'], priority='*'", func() {
				createTestWorkflowWithArrayLabels("522-emptydir", "IncreaseMemoryLimits",
					[]string{"critical", "high"}, "*", []string{"*"}, "*")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "high",
					Component:   "v1/Node",
					Environment: "unknown",
					Priority:    "P3",
				}
				result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1), "IT-DS-522-001: workflow with wildcard component/environment/priority must match specific query values")
				Expect(result).To(HaveLen(1))
				Expect(result[0].ActionType).To(Equal("IncreaseMemoryLimits"))
				Expect(result[0].WorkflowCount).To(BeNumerically(">=", 1))
			})
		})

		Context("IT-DS-522-002: all-wildcard labels with 'unknown' environment", func() {
			It("should match a fully wildcarded workflow when environment=unknown", func() {
				createTestWorkflowWithArrayLabels("522-allwild", "IncreaseMemoryLimits",
					[]string{"*"}, "*", []string{"*"}, "*")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "high",
					Component:   "v1/Node",
					Environment: "unknown",
					Priority:    "P3",
				}
				result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1), "IT-DS-522-002: all-wildcard workflow must match when environment=unknown")
				Expect(result).To(HaveLen(1))
			})
		})

		Context("IT-DS-522-003: ListWorkflowsByActionType with wildcard labels", func() {
			It("should return the wildcard workflow in Step 2 discovery", func() {
				createTestWorkflowWithArrayLabels("522-step2", "IncreaseMemoryLimits",
					[]string{"critical", "high"}, "*", []string{"*"}, "*")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "high",
					Component:   "v1/Node",
					Environment: "unknown",
					Priority:    "P3",
				}
				results, totalCount, err := workflowRepo.ListWorkflowsByActionType(ctx, "IncreaseMemoryLimits", filters, 0, 10)

				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1), "IT-DS-522-003: wildcard workflow must appear in Step 2 discovery")
				Expect(results).To(HaveLen(1))
				Expect(results[0].ActionType).To(Equal("IncreaseMemoryLimits"))
			})
		})
	})
})
