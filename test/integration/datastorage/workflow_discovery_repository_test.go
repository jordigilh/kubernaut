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
	"crypto/sha256"
	"fmt"

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
// Test IDs: IT-DS-017-001-001 through IT-DS-017-001-006
//
// Strategy: TDD RED phase - tests written FIRST, implementation follows
// Infrastructure: Real PostgreSQL (same as workflow_repository_integration_test.go)
//
// ========================================

// Serial: These tests TRUNCATE remediation_workflow_catalog and assert exact counts.
// Must not run concurrently with other tests that write to the workflow catalog.
var _ = Describe("Workflow Discovery Repository Integration Tests", Serial, func() {
	var (
		workflowRepo *workflow.Repository
		testID       string
	)

	BeforeEach(func() {
		workflowRepo = workflow.NewRepository(db, logger)
		testID = generateTestID()

		// Clean up workflow catalog for test isolation
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE remediation_workflow_catalog")
		Expect(err).ToNot(HaveOccurred(), "Workflow catalog truncation should succeed")

		// Seed action_type_taxonomy (may already exist from migration 025)
		// Use ON CONFLICT DO NOTHING for idempotency
		// DD-WORKFLOW-016 V1.0: Use DD types (IncreaseMemoryLimits, RestartDeployment, RollbackDeployment)
		seedSQL := `
			INSERT INTO action_type_taxonomy (action_type, description) VALUES
				('ScaleReplicas', '{"what": "Horizontally scale a workload", "when_to_use": "Insufficient capacity", "preconditions": "Evidence of increased load"}'),
				('RestartPod', '{"what": "Kill and recreate pods", "when_to_use": "Transient runtime state issue", "preconditions": "Evidence issue is transient"}'),
				('IncreaseMemoryLimits', '{"what": "Increase memory limits on containers", "when_to_use": "OOM kills from low limits", "preconditions": "Stable memory pattern"}'),
				('RestartDeployment', '{"what": "Rolling restart of workload", "when_to_use": "Workload-wide state issue", "preconditions": "Multiple pods affected"}'),
				('RollbackDeployment', '{"what": "Revert to previous revision", "when_to_use": "Recent deployment regression", "preconditions": "Previous healthy revision exists"}')
			ON CONFLICT (action_type) DO NOTHING
		`
		_, err = db.ExecContext(ctx, seedSQL)
		Expect(err).ToNot(HaveOccurred(), "Taxonomy seeding should succeed")
	})

	AfterEach(func() {
		if db != nil {
			_, _ = db.ExecContext(ctx,
				"DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE $1",
				fmt.Sprintf("wf-disc-%s%%", testID))
		}
	})

	// ========================================
	// HELPER: Create a test workflow with action_type
	// ========================================
	createTestWorkflow := func(name, version, actionType, severity, component, environment, priority, status string) *models.RemediationWorkflow {
		content := fmt.Sprintf("apiVersion: v1\nkind: Workflow\nname: %s", name)
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

		labels := models.MandatoryLabels{
			SignalName:  "OOMKilled",
			Severity:    []string{severity},
			Component:   component,
			Environment: []string{environment},
			Priority:    priority,
		}

		wf := &models.RemediationWorkflow{
			WorkflowName:    fmt.Sprintf("wf-disc-%s-%s", testID, name),
			Version:         version,
			Name:            name,
			Description: models.StructuredDescription{
				What:      fmt.Sprintf("Test workflow %s for discovery", name),
				WhenToUse: "Testing",
			},
			Content:         content,
			ContentHash:     hash,
			Labels:          labels,
			ExecutionEngine: models.ExecutionEngineTekton,
			Status:          status,
			IsLatestVersion: true,
			ActionType:      actionType,
		}

		err := workflowRepo.Create(ctx, wf)
		Expect(err).ToNot(HaveOccurred(), "Workflow creation should succeed for %s", name)
		Expect(wf.WorkflowID).ToNot(BeEmpty(), "Workflow ID should be generated")

		return wf
	}

	// ========================================
	// IT-DS-017-001-001: ListActions -- active status filter
	// ========================================
	Describe("ListActions", func() {
		Context("IT-DS-017-001-001: active status filter", func() {
			It("should return only action types that have active workflows", func() {
				// Arrange: 2 active ScaleReplicas, 1 disabled RestartPod
				createTestWorkflow("scale-1", "v1.0.0", "ScaleReplicas", "critical", "pod", "production", "P0", "active")
				createTestWorkflow("scale-2", "v1.0.0", "ScaleReplicas", "high", "deployment", "staging", "P1", "active")
				createTestWorkflow("restart-disabled", "v1.0.0", "RestartPod", "critical", "pod", "production", "P0", "disabled")

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
		// GAP-WF-3: ListActions -- returns only latest version counts
		// ========================================
		Context("GAP-WF-3: ListActions returns only latest version counts", func() {
			It("should count only workflows with is_latest_version=true", func() {
				// Arrange: Two versions of same workflow - only latest should be counted
				createTestWorkflow("scale-v1", "v1.0.0", "ScaleReplicas", "critical", "pod", "production", "P0", "active")
				createTestWorkflow("scale-v1", "v1.1.0", "ScaleReplicas", "critical", "pod", "production", "P0", "active")

				filters := &models.WorkflowDiscoveryFilters{}

				// Act
				result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

				// Assert: ScaleReplicas should have workflow_count=1 (latest only), not 2
				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1))
				Expect(result).To(HaveLen(1))
				Expect(result[0].ActionType).To(Equal("ScaleReplicas"))
				Expect(result[0].WorkflowCount).To(Equal(1), "Should count only latest version")
			})
		})

		// ========================================
		// IT-DS-017-001-002: ListActions -- pagination
		// ========================================
		Context("IT-DS-017-001-002: pagination returns correct slice", func() {
			It("should paginate action types correctly", func() {
				// Arrange: Create workflows spanning 5 action types (all active) - DD-WORKFLOW-016 V1.0
				createTestWorkflow("scale-wf", "v1.0.0", "ScaleReplicas", "critical", "pod", "production", "P0", "active")
				createTestWorkflow("restart-wf", "v1.0.0", "RestartPod", "critical", "pod", "production", "P0", "active")
				createTestWorkflow("rollback-wf", "v1.0.0", "RollbackDeployment", "critical", "pod", "production", "P0", "active")
				createTestWorkflow("memory-wf", "v1.0.0", "IncreaseMemoryLimits", "critical", "pod", "production", "P0", "active")
				createTestWorkflow("restart-deploy-wf", "v1.0.0", "RestartDeployment", "critical", "pod", "production", "P0", "active")

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
				createTestWorkflow("scale-conservative", "v1.0.0", "ScaleReplicas", "critical", "pod", "production", "P0", "active")
				createTestWorkflow("scale-aggressive", "v1.0.0", "ScaleReplicas", "high", "deployment", "staging", "P1", "active")
				createTestWorkflow("restart-simple", "v1.0.0", "RestartPod", "critical", "pod", "production", "P0", "active")

				// Act: Filter for ScaleReplicas + severity=critical
				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "pod",
					Environment: "production",
					Priority:    "P0",
				}
				results, totalCount, err := workflowRepo.ListWorkflowsByActionType(ctx, "ScaleReplicas", filters, 0, 10)

				// Assert: Only scale-conservative matches (action_type=ScaleReplicas AND severity=critical)
				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1))
				Expect(results).To(HaveLen(1))
				Expect(results[0].Name).To(Equal("scale-conservative"))
			})
		})

		// ========================================
		// GAP-WF-3: ListWorkflowsByActionType -- returns only latest versions
		// ========================================
		Context("GAP-WF-3: returns only latest workflow versions", func() {
			It("should return only workflows with is_latest_version=true", func() {
				// Arrange: Create two versions of same workflow (v1.0.0, v1.1.0)
				// Create() sets v1.0.0 is_latest_version=true, then v1.1.0 creation flips v1.0.0 to false
				createTestWorkflow("scale-v1", "v1.0.0", "ScaleReplicas", "critical", "pod", "production", "P0", "active")
				createTestWorkflow("scale-v1", "v1.1.0", "ScaleReplicas", "critical", "pod", "production", "P0", "active")

				// Act
				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "pod",
					Environment: "production",
					Priority:    "P0",
				}
				results, totalCount, err := workflowRepo.ListWorkflowsByActionType(ctx, "ScaleReplicas", filters, 0, 10)

				// Assert: Only one workflow (latest v1.1.0), not two
				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1), "Discovery should return only latest version")
				Expect(results).To(HaveLen(1))
				Expect(results[0].Version).To(Equal("v1.1.0"))
				Expect(results[0].IsLatestVersion).To(BeTrue())
			})
		})

		// ========================================
		// IT-DS-017-001-004: ListWorkflowsByActionType -- excludes disabled workflows
		// ========================================
		Context("IT-DS-017-001-004: excludes disabled workflows", func() {
			It("should not return disabled workflows", func() {
				// Arrange: One active, one disabled -- same action_type
				createTestWorkflow("scale-active", "v1.0.0", "ScaleReplicas", "critical", "pod", "production", "P0", "active")
				createTestWorkflow("scale-disabled", "v1.0.0", "ScaleReplicas", "critical", "pod", "production", "P0", "disabled")

				// Act
				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "pod",
					Environment: "production",
					Priority:    "P0",
				}
				results, totalCount, err := workflowRepo.ListWorkflowsByActionType(ctx, "ScaleReplicas", filters, 0, 10)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1))
				Expect(results).To(HaveLen(1))
				Expect(results[0].Name).To(Equal("scale-active"))
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
				wf := createTestWorkflow("scale-match", "v1.0.0", "ScaleReplicas", "critical", "pod", "production", "P0", "active")

				// Act
				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "pod",
					Environment: "production",
					Priority:    "P0",
				}
				result, err := workflowRepo.GetWorkflowWithContextFilters(ctx, wf.WorkflowID, filters)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.WorkflowID).To(Equal(wf.WorkflowID))
				Expect(result.ActionType).To(Equal("ScaleReplicas"))
			})
		})

		// ========================================
		// IT-DS-017-001-006: GetWorkflowWithContextFilters -- context mismatch returns nil
		// ========================================
		Context("IT-DS-017-001-006: context mismatch returns nil (security gate)", func() {
			It("should return nil when context filters do not match", func() {
				// Arrange: workflow with severity=critical, environment=production
				wf := createTestWorkflow("scale-mismatch", "v1.0.0", "ScaleReplicas", "critical", "pod", "production", "P0", "active")

				// Act: Query with mismatching context (severity=high, environment=staging)
				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "high",
					Component:   "deployment",
					Environment: "staging",
					Priority:    "P1",
				}
				result, err := workflowRepo.GetWorkflowWithContextFilters(ctx, wf.WorkflowID, filters)

				// Assert: Security gate -- no workflow returned
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeNil(), "Security gate should prevent returning mismatched workflow")
			})
		})
	})
})
