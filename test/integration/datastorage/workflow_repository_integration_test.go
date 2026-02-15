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
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
)

// ========================================
// WORKFLOW CATALOG REPOSITORY INTEGRATION TESTS
// ========================================
//
// Purpose: Test Workflow Catalog Repository against REAL PostgreSQL database
// to catch schema mismatches and field mapping bugs.
//
// Authority: DD-STORAGE-008 v2.0 (Workflow Catalog Schema)
// Business Requirements:
// - BR-STORAGE-012: Workflow catalog persistence
// - BR-WORKFLOW-001: Remediation workflow storage
//
// Test Strategy:
// - Uses REAL PostgreSQL database (not mocks)
// - Validates composite PK (workflow_name, version) constraints
// - Tests JSONB labels serialization/deserialization
// - Tests version management (is_latest_version flag)
// - Tests lifecycle status management
// - Tests success metrics tracking
//
// Coverage Gap Addressed:
// This file addresses the gap identified in TEST_COVERAGE_GAP_WORKFLOW_CATALOG.md
// where missing integration tests could allow field mapping bugs similar to
// the audit events bug (missing version, namespace, cluster_name).
//
// Defense-in-Depth Strategy:
// - Integration tests (this file): Catch schema/field mapping bugs with real DB
// - E2E tests (REST API): Validate complete business flows
//
// ========================================

var _ = Describe("Workflow Catalog Repository Integration Tests", func() {
	var (
		workflowRepo *workflow.Repository
		testID       string
	)

	BeforeEach(func() {
		// Create repository with real database (shared public schema)
		workflowRepo = workflow.NewRepository(db, logger)

		// Generate unique test ID for isolation
		testID = generateTestID()

		// DS-FLAKY-007: Process-scoped cleanup instead of global TRUNCATE.
		// TRUNCATE is a global DDL operation that wipes ALL rows from the table,
		// including rows created by OTHER parallel ginkgo processes sharing the
		// same database. This causes race conditions:
		//   - Process A creates a workflow, Process B's BeforeEach TRUNCATEs → A's SELECT finds nothing
		//   - Process A inserts row #1, Process B TRUNCATEs, Process A inserts row #2 → no unique violation
		// Fix: Use process-scoped DELETE to only clean up rows created by THIS process.
		processPrefix := fmt.Sprintf("wf-repo-test-%d-%%", GinkgoParallelProcess())
		result, err := db.ExecContext(ctx,
			"DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE $1",
			processPrefix)
		Expect(err).ToNot(HaveOccurred(), "Process-scoped cleanup should succeed")

		rowsDeleted, _ := result.RowsAffected()
		GinkgoWriter.Printf("✅ Deleted %d workflow(s) in process-scoped cleanup (process %d)\n",
			rowsDeleted, GinkgoParallelProcess())
	})

	AfterEach(func() {
		// Clean up test data
		if db != nil {
			_, _ = db.ExecContext(ctx,
				"DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE $1",
				fmt.Sprintf("wf-repo-%s%%", testID))
		}
	})

	// ========================================
	// CREATE METHOD TESTS - COMPOSITE PK VALIDATION
	// ========================================
	Describe("Create", func() {
		Context("with valid workflow and all required fields", func() {
			It("should persist workflow with structured labels and composite PK", func() {
				// ARRANGE: Create test workflow per DD-STORAGE-008 schema
				workflowName := fmt.Sprintf("wf-repo-%s-test", testID)
				content := `{"steps":[{"action":"scale","replicas":3}]}`
				contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

				// V1.0: Use structured MandatoryLabels
				labels := models.MandatoryLabels{
					SignalType:  "prometheus",
					Severity:    models.StringOrSlice{"critical"},
					Component:   "kube-apiserver",
					Priority:    "P0",
					Environment: []string{"production"},
				}

				testWorkflow := &models.RemediationWorkflow{
					WorkflowName:    workflowName,
					Version:         "v1.0.0",
					Name:            "Test Workflow",
					Description:     models.StructuredDescription{What: "Integration test workflow", WhenToUse: "Testing"},
					Content:         content,
					ContentHash:     contentHash,
					Labels:          labels,
					CustomLabels:    models.CustomLabels{},
					DetectedLabels:  models.DetectedLabels{},
					Status:          "active",
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
					ActionType:      "ScaleReplicas",
				}

				// ACT: Create workflow
				err := workflowRepo.Create(ctx, testWorkflow)

				// ASSERT: Create succeeds
				Expect(err).ToNot(HaveOccurred(), "Create should succeed")

				// ASSERT: Verify workflow persisted with correct composite PK
				// Use Eventually to handle transaction commit delays (DS-FLAKY-006 fix)
				var (
					dbWorkflowName, dbVersion, dbName, dbDescription, dbContent, dbContentHash, dbStatus, dbExecutionEngine string
					dbLabels                                                                                                []byte // JSONB
					dbIsLatestVersion                                                                                       bool
					dbCreatedAt, dbUpdatedAt                                                                                time.Time
				)

				Eventually(func() error {
					row := db.QueryRowContext(ctx, `
						SELECT workflow_name, version, name, description, content, content_hash,
						       labels, status, execution_engine, is_latest_version, created_at, updated_at
						FROM remediation_workflow_catalog
						WHERE workflow_name = $1 AND version = $2
					`, workflowName, "v1.0.0")

					return row.Scan(
						&dbWorkflowName,
						&dbVersion,
						&dbName,
						&dbDescription,
						&dbContent,
						&dbContentHash,
						&dbLabels,
						&dbStatus,
						&dbExecutionEngine,
						&dbIsLatestVersion,
					&dbCreatedAt,
					&dbUpdatedAt,
				)
			}, 10*time.Second, 200*time.Millisecond).Should(Succeed(), "Should retrieve workflow from database within 10 seconds (CI-safe)")

				// CRITICAL ASSERTIONS: Verify composite PK and all fields
				Expect(dbWorkflowName).To(Equal(workflowName), "workflow_name should match")
				Expect(dbVersion).To(Equal("v1.0.0"), "version should match")
				Expect(dbName).To(Equal("Test Workflow"))
				// Description is now StructuredDescription JSONB (BR-WORKFLOW-004, migration 026)
			var parsedDesc models.StructuredDescription
			Expect(json.Unmarshal([]byte(dbDescription), &parsedDesc)).To(Succeed())
			Expect(parsedDesc.What).To(Equal("Integration test workflow"))
				Expect(dbContent).To(ContainSubstring("scale"))
				Expect(dbContentHash).To(Equal(contentHash))
				Expect(dbStatus).To(Equal("active"))
				Expect(dbExecutionEngine).To(Equal("argo-workflows"))
				Expect(dbIsLatestVersion).To(BeTrue(), "is_latest_version should be true")
				Expect(dbCreatedAt).ToNot(BeZero())
				Expect(dbUpdatedAt).ToNot(BeZero())

			// CRITICAL: Verify JSONB labels persisted correctly
			Expect(dbLabels).ToNot(BeEmpty(), "Labels should be persisted as JSONB")
			// DD-WORKFLOW-001 v2.5: environment is now []string, use map[string]interface{}
			var persistedLabels map[string]interface{}
			err = json.Unmarshal(dbLabels, &persistedLabels)
			Expect(err).ToNot(HaveOccurred())
			Expect(persistedLabels).To(HaveKeyWithValue("signalType", "prometheus"))
			Expect(persistedLabels).To(HaveKeyWithValue("severity", "critical"))
			// Verify environment is an array
			Expect(persistedLabels["environment"]).To(BeAssignableToTypeOf([]interface{}{}))
			envArray := persistedLabels["environment"].([]interface{})
			Expect(envArray).To(ContainElement("production"))
			})
		})

		Context("with duplicate composite PK (workflow_name, version)", func() {
			It("should return unique constraint violation error", func() {
				// ARRANGE: Create first workflow
				workflowName := fmt.Sprintf("wf-repo-%s-duplicate", testID)
				content := `{"steps":[]}`
				contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

				// V1.0: Use structured MandatoryLabels
				labels := models.MandatoryLabels{
					SignalType:  "test",
					Severity:    models.StringOrSlice{"low"},
					Component:   "test",
					Priority:    "P3",
					Environment: []string{"test"},
				}

				testWorkflow := &models.RemediationWorkflow{
					WorkflowName:    workflowName,
					Version:         "v1.0.0",
					Name:            "Original Workflow",
					Description:     models.StructuredDescription{What: "First workflow", WhenToUse: "Testing"},
					Content:         content,
					ContentHash:     contentHash,
					Labels:          labels,
					CustomLabels:    models.CustomLabels{},
					DetectedLabels:  models.DetectedLabels{},
					Status:          "active",
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
					ActionType:      "ScaleReplicas",
				}

				err := workflowRepo.Create(ctx, testWorkflow)
				Expect(err).ToNot(HaveOccurred())

				// ACT: Attempt to create duplicate (same workflow_name + version)
				duplicateWorkflow := &models.RemediationWorkflow{
					WorkflowName:    workflowName, // Same name
					Version:         "v1.0.0",     // Same version
					Name:            "Duplicate Workflow",
					Description:     models.StructuredDescription{What: "Duplicate workflow", WhenToUse: "Testing"},
					Content:         content,
					ContentHash:     contentHash,
					Labels:          labels,
					CustomLabels:    models.CustomLabels{},
					DetectedLabels:  models.DetectedLabels{},
					Status:          "active",
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
					ActionType:      "ScaleReplicas",
				}

				err = workflowRepo.Create(ctx, duplicateWorkflow)

				// ASSERT: Should fail with composite PK violation
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("duplicate key"), "Should be unique constraint violation")
			})
		})
	})

	// ========================================
	// GET METHODS TESTS - VERSION MANAGEMENT
	// ========================================
	Describe("GetByNameAndVersion", func() {
		var workflowName string

		BeforeEach(func() {
			// Insert test workflow
			workflowName = fmt.Sprintf("wf-repo-%s-get", testID)
			content := `{"steps":[]}`
			contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

			// V1.0: Use structured MandatoryLabels
			labels := models.MandatoryLabels{
				SignalType:  "test",
				Severity:    models.StringOrSlice{"low"},
				Component:   "test",
				Priority:    "P3",
				Environment: []string{"test"},
			}

			testWorkflow := &models.RemediationWorkflow{
				WorkflowName:    workflowName,
				Version:         "v1.0.0",
				Name:            "Test Workflow Get",
				Description:     models.StructuredDescription{What: "Test workflow for Get method", WhenToUse: "Testing"},
				Content:         content,
				ContentHash:     contentHash,
				Labels:          labels,
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "active",
				ExecutionEngine: "argo-workflows",
				IsLatestVersion: true,
				ActionType:      "ScaleReplicas",
			}

			err := workflowRepo.Create(ctx, testWorkflow)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("with existing workflow", func() {
			It("should retrieve workflow with all fields including JSONB labels", func() {
				// ACT: Get workflow by name and version
				retrievedWorkflow, err := workflowRepo.GetByNameAndVersion(ctx, workflowName, "v1.0.0")

				// ASSERT: Get succeeds
				Expect(err).ToNot(HaveOccurred())
				Expect(retrievedWorkflow).ToNot(BeNil())

				// ASSERT: All fields populated correctly
				Expect(retrievedWorkflow.WorkflowName).To(Equal(workflowName))
				Expect(retrievedWorkflow.Version).To(Equal("v1.0.0"))
				Expect(retrievedWorkflow.Name).To(Equal("Test Workflow Get"))
				// Description is now StructuredDescription (BR-WORKFLOW-004, migration 026)
			Expect(retrievedWorkflow.Description.What).To(Equal("Test workflow for Get method"))
				Expect(retrievedWorkflow.Status).To(Equal("active"))
				Expect(retrievedWorkflow.ExecutionEngine).To(Equal(models.ExecutionEngine("argo-workflows")))
				Expect(retrievedWorkflow.IsLatestVersion).To(BeTrue())
				Expect(retrievedWorkflow.CreatedAt).ToNot(BeZero())
				Expect(retrievedWorkflow.UpdatedAt).ToNot(BeZero())

			// CRITICAL: Verify structured labels deserialized correctly
			Expect(retrievedWorkflow.Labels.SignalType).To(Equal("test"))
			Expect(retrievedWorkflow.Labels.Severity).To(Equal(models.StringOrSlice{"low"}))
			Expect(retrievedWorkflow.Labels.Component).To(Equal("test"))
			Expect(retrievedWorkflow.Labels.Priority).To(Equal("P3"))
			// DD-WORKFLOW-001 v2.5: Environment is now []string
			Expect(retrievedWorkflow.Labels.Environment).To(Equal([]string{"test"}))
			})
		})
	})

	// ========================================
	// LIST METHOD TESTS - FILTERING & PAGINATION
	// ========================================
	// ARCHITECTURAL NOTE: Serial Execution Required for List tests
	// List tests with nil filters verify exact workflow counts.
	// Serial execution ensures no concurrent test creates/deletes workflows.
	Describe("List", Serial, func() {
		var createdWorkflowNames []string

		BeforeEach(func() {
			createdWorkflowNames = []string{} // Reset for each test

			// Clean slate: delete ALL workflows so List(nil) returns exact counts.
			// Safe because Serial guarantees no concurrent access.
			_, _ = db.ExecContext(ctx, `DELETE FROM remediation_workflow_catalog`)

			// Insert multiple test workflows with different statuses
			workflows := []struct {
				name    string
				version string
				status  string
			}{
				{fmt.Sprintf("wf-repo-%s-list-1", testID), "v1.0.0", "active"},
				{fmt.Sprintf("wf-repo-%s-list-2", testID), "v1.0.0", "disabled"},
				{fmt.Sprintf("wf-repo-%s-list-3", testID), "v1.0.0", "active"},
			}

			for _, wf := range workflows {
				content := `{"steps":[]}`
				contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

				// V1.0: Use structured MandatoryLabels
				labels := models.MandatoryLabels{
					SignalType:  "test",
					Severity:    models.StringOrSlice{"low"},
					Component:   "test",
					Priority:    "P3",
					Environment: []string{"test"},
				}

				testWorkflow := &models.RemediationWorkflow{
					WorkflowName:    wf.name,
					Version:         wf.version,
					Name:            wf.name,
					Description:     models.StructuredDescription{What: "Test workflow", WhenToUse: "Testing"},
					Content:         content,
					ContentHash:     contentHash,
					Labels:          labels,
					CustomLabels:    models.CustomLabels{},
					DetectedLabels:  models.DetectedLabels{},
					Status:          wf.status,
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
					ActionType:      "ScaleReplicas",
				}

				err := workflowRepo.Create(ctx, testWorkflow)
				Expect(err).ToNot(HaveOccurred())
				createdWorkflowNames = append(createdWorkflowNames, wf.name)
			}
		})

		AfterEach(func() {
			// Cleanup: Delete test workflows created in this test
			for _, workflowName := range createdWorkflowNames {
				_, _ = db.ExecContext(ctx, `DELETE FROM remediation_workflow_catalog WHERE workflow_name = $1`, workflowName)
			}
		})

		Context("with no filters", func() {
			It("should return all workflows with all fields", func() {
				// ACT: List workflows
				workflows, total, err := workflowRepo.List(ctx, nil, 50, 0)

				// ASSERT: List succeeds
				Expect(err).ToNot(HaveOccurred())
				Expect(workflows).To(HaveLen(3))
				Expect(total).To(Equal(3))

				// ASSERT: All fields populated for each workflow
				for _, wf := range workflows {
					Expect(wf.WorkflowName).ToNot(BeEmpty())
					Expect(wf.Version).ToNot(BeEmpty())
					Expect(wf.Name).ToNot(BeEmpty())
					Expect(wf.Labels).ToNot(BeNil())
					Expect(wf.CreatedAt).ToNot(BeZero())
					Expect(wf.UpdatedAt).ToNot(BeZero())
				}
			})
		})

		Context("with status filter", func() {
			It("should filter workflows by status", func() {
				// ACT: List only active workflows
				filters := &models.WorkflowSearchFilters{
					Status: []string{"active"},
				}
				workflows, total, err := workflowRepo.List(ctx, filters, 50, 0)

				// ASSERT: Only active workflows returned
				Expect(err).ToNot(HaveOccurred())
				Expect(workflows).To(HaveLen(2), "Should return 2 active workflows")
				Expect(total).To(Equal(2))

				for _, wf := range workflows {
					Expect(wf.Status).To(Equal("active"))
				}
			})
		})

		Context("with pagination", func() {
			It("should apply limit and offset correctly", func() {
				// ACT: List with pagination (limit=2, offset=1)
				workflows, total, err := workflowRepo.List(ctx, nil, 2, 1)

				// ASSERT: Pagination applied correctly
				Expect(err).ToNot(HaveOccurred())
				Expect(workflows).To(HaveLen(2), "Should return 2 workflows (limit=2)")
				Expect(total).To(Equal(3), "Total should be 3 (all workflows)")
			})
		})

		Context("with workflow_name filter", func() {
			It("should filter workflows by exact workflow name match", func() {
				// ARRANGE: Specific workflow name we want to find
				targetWorkflowName := fmt.Sprintf("wf-repo-%s-list-1", testID)

				// ACT: List workflows filtered by workflow_name
				// Authority: DD-API-001 (OpenAPI client mandatory - workflow_name filter added Jan 2026)
				// Used for test idempotency and workflow lookup by human-readable name
				filters := &models.WorkflowSearchFilters{
					WorkflowName: targetWorkflowName,
				}
				workflows, total, err := workflowRepo.List(ctx, filters, 50, 0)

				// ASSERT: Only the specific workflow returned
				Expect(err).ToNot(HaveOccurred())
				Expect(workflows).To(HaveLen(1), "Should return exactly 1 workflow matching the name")
				Expect(total).To(Equal(1), "Total should be 1")
				Expect(workflows[0].WorkflowName).To(Equal(targetWorkflowName))
			})

			It("should return empty result for non-existent workflow name", func() {
				// ACT: List workflows with non-existent workflow_name
				filters := &models.WorkflowSearchFilters{
					WorkflowName: "non-existent-workflow-name",
				}
				workflows, total, err := workflowRepo.List(ctx, filters, 50, 0)

				// ASSERT: Empty result set
				Expect(err).ToNot(HaveOccurred())
				Expect(workflows).To(HaveLen(0), "Should return no workflows")
				Expect(total).To(Equal(0), "Total should be 0")
			})
		})
	})

	// ========================================
	// UPDATE STATUS TESTS - LIFECYCLE MANAGEMENT
	// ========================================
	Describe("UpdateStatus", func() {
		var workflowName string
		var workflowID string

		BeforeEach(func() {
			// Insert active workflow
			workflowName = fmt.Sprintf("wf-repo-%s-update", testID)
			content := `{"steps":[]}`
			contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

			// V1.0: Use structured MandatoryLabels
			labels := models.MandatoryLabels{
				SignalType:  "test",
				Severity:    models.StringOrSlice{"low"},
				Component:   "test",
				Priority:    "P3",
				Environment: []string{"test"},
			}

			testWorkflow := &models.RemediationWorkflow{
				WorkflowName:    workflowName,
				Version:         "v1.0.0",
				Name:            "Workflow to Update",
				Description:     models.StructuredDescription{What: "Test workflow for status update", WhenToUse: "Testing"},
				Content:         content,
				ContentHash:     contentHash,
				Labels:          labels,
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "active",
				ExecutionEngine: "argo-workflows",
				IsLatestVersion: true,
				ActionType:      "ScaleReplicas",
			}

			err := workflowRepo.Create(ctx, testWorkflow)
			Expect(err).ToNot(HaveOccurred())

			// Store the generated workflow_id for use in tests
			workflowID = testWorkflow.WorkflowID
		})

		Context("with valid status change (active → disabled)", func() {
			It("should update status with reason and metadata", func() {
				// ACT: Update status to disabled (use workflow_id UUID, not workflow_name)
				err := workflowRepo.UpdateStatus(ctx, workflowID, "v1.0.0", "disabled", "Test disable reason", "test-user")

				// ASSERT: Update succeeds
				Expect(err).ToNot(HaveOccurred())

				// ASSERT: Verify status updated in database
				var dbStatus string
				var dbDisabledBy, dbDisabledReason *string
				var dbDisabledAt *time.Time

				err = db.QueryRowContext(ctx, `
					SELECT status, disabled_at, disabled_by, disabled_reason
					FROM remediation_workflow_catalog
					WHERE workflow_name = $1 AND version = $2
				`, workflowName, "v1.0.0").Scan(&dbStatus, &dbDisabledAt, &dbDisabledBy, &dbDisabledReason)

				Expect(err).ToNot(HaveOccurred())
				Expect(dbStatus).To(Equal("disabled"), "Status should be disabled")
				Expect(dbDisabledAt).ToNot(BeNil(), "disabled_at should be set")
				Expect(dbDisabledBy).ToNot(BeNil(), "disabled_by should be set")
				Expect(*dbDisabledBy).To(Equal("test-user"))
				Expect(dbDisabledReason).ToNot(BeNil(), "disabled_reason should be set")
				Expect(*dbDisabledReason).To(Equal("Test disable reason"))
			})
		})
	})
})
