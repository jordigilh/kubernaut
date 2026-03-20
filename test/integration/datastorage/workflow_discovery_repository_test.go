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
			Severity:    []string{severity},
			Component:   component,
			Environment: []string{environment},
			Priority:    priority,
		}

		wf := &models.RemediationWorkflow{
			WorkflowName:    fmt.Sprintf("wf-disc-%s-%s", testID, name),
			Version:         version,
			SchemaVersion:   "1.0",
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
	// protocol against real PostgreSQL JSONB operators.
	// ========================================

	createTestWorkflowWithArrayLabels := func(name, version, actionType string, severity []string, component string, environment []string, priority, status string) *models.RemediationWorkflow {
		content := fmt.Sprintf("apiVersion: v1\nkind: Workflow\nname: %s", name)
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

		labels := models.MandatoryLabels{
			Severity:    severity,
			Component:   component,
			Environment: environment,
			Priority:    priority,
		}

		wf := &models.RemediationWorkflow{
			WorkflowName:  fmt.Sprintf("wf-disc-%s-%s", testID, name),
			Version:       version,
			SchemaVersion: "1.0",
			Name:          name,
			Description: models.StructuredDescription{
				What:      fmt.Sprintf("Test workflow %s for wildcard discovery", name),
				WhenToUse: "Testing wildcard label matching",
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

	Describe("ListActions - Wildcard Labels (#464)", func() {
		// ========================================
		// IT-DS-464-001: ListActions matches wildcard component + priority
		// ========================================
		Context("IT-DS-464-001: wildcard component + priority", func() {
			It("should match a workflow with component='*' and priority='*' when queried with specific values", func() {
				createTestWorkflowWithArrayLabels("wc-comp-pri", "v1.0.0", "ScaleReplicas",
					[]string{"critical"}, "*", []string{"production"}, "*", "active")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "Pod",
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
				createTestWorkflowWithArrayLabels("wc-all", "v1.0.0", "ScaleReplicas",
					[]string{"*"}, "*", []string{"*"}, "*", "active")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "high",
					Component:   "Deployment",
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
				createTestWorkflowWithArrayLabels("wc-step2", "v1.0.0", "ScaleReplicas",
					[]string{"critical"}, "*", []string{"production"}, "*", "active")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "Pod",
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
				wf := createTestWorkflowWithArrayLabels("wc-gate", "v1.0.0", "ScaleReplicas",
					[]string{"critical"}, "*", []string{"production"}, "*", "active")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "Pod",
					Environment: "production",
					Priority:    "P1",
				}
				result, err := workflowRepo.GetWorkflowWithContextFilters(ctx, wf.WorkflowID, filters)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil(), "IT-DS-464-004: security gate must not reject wildcard-labeled workflow")
				Expect(result.WorkflowID).To(Equal(wf.WorkflowID))
			})
		})
	})

	// ========================================
	// IT-DS-464-005: Demo scenario — mixed wildcards + exact labels
	// ========================================
	Describe("Demo Scenario - Mixed Wildcards (#464)", func() {
		Context("IT-DS-464-005: exact demo scenario from issue #464", func() {
			It("should match a workflow with mixed wildcard and exact labels", func() {
				createTestWorkflowWithArrayLabels("wc-demo", "v1.0.0", "IncreaseMemoryLimits",
					[]string{"critical", "high"}, "*", []string{"production", "staging", "*"}, "*", "active")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "Pod",
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
				createTestWorkflowWithArrayLabels("wc-sev", "v1.0.0", "RestartPod",
					[]string{"*"}, "pod", []string{"production"}, "P0", "active")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "pod",
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
})
