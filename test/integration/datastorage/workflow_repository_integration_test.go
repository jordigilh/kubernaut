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
		// CRITICAL: Use public schema FIRST before any cleanup/operations
		// remediation_workflow_catalog is NOT schema-isolated - all workflows are in public schema
		// Without this, cleanup queries wrong schema and leaves data contamination
		usePublicSchema()

		// Create repository with real database
		workflowRepo = workflow.NewRepository(db, logger)

		// Generate unique test ID for isolation
		testID = generateTestID()

		// This ensures no leftover data from previous test runs
		// Clean up ALL test workflows including:
		// - wf-repo-% (workflow repository tests)
		// - wf-scoring-% (scoring tests)
		// - bulk-import-test-% (bulk import tests)
		// Use TRUNCATE for complete cleanup to avoid LIKE pattern mismatches
		result, err := db.ExecContext(ctx, "TRUNCATE TABLE remediation_workflow_catalog")
		Expect(err).ToNot(HaveOccurred(), "Global cleanup should succeed")

		rowsDeleted, _ := result.RowsAffected()
		GinkgoWriter.Printf("✅ Deleted %d workflow(s) in global cleanup (TRUNCATE)\n", rowsDeleted)
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
		BeforeEach(func() {
			// CRITICAL: Use public schema - remediation_workflow_catalog is NOT schema-isolated
			// Without this, workflow created in test_process_N schema won't be found by Get/List
			usePublicSchema()
		})

		Context("with valid workflow and all required fields", func() {
			It("should persist workflow with structured labels and composite PK", func() {
				// ARRANGE: Create test workflow per DD-STORAGE-008 schema
				workflowName := fmt.Sprintf("wf-repo-%s-test", testID)
				content := `{"steps":[{"action":"scale","replicas":3}]}`
				contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

				// V1.0: Use structured MandatoryLabels
				labels := models.MandatoryLabels{
					SignalType:  "prometheus",
					Severity:    "critical",
					Component:   "kube-apiserver",
					Priority:    "P0",
					Environment: "production",
				}

				testWorkflow := &models.RemediationWorkflow{
					WorkflowName:    workflowName,
					Version:         "v1.0.0",
					Name:            "Test Workflow",
					Description:     "Integration test workflow",
					Content:         content,
					ContentHash:     contentHash,
					Labels:          labels,
					CustomLabels:    models.CustomLabels{},
					DetectedLabels:  models.DetectedLabels{},
					Status:          "active",
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
				}

				// ACT: Create workflow
				err := workflowRepo.Create(ctx, testWorkflow)

				// ASSERT: Create succeeds
				Expect(err).ToNot(HaveOccurred(), "Create should succeed")

				// ASSERT: Verify workflow persisted with correct composite PK
				var (
					dbWorkflowName, dbVersion, dbName, dbDescription, dbContent, dbContentHash, dbStatus, dbExecutionEngine string
					dbLabels                                                                                                []byte // JSONB
					dbIsLatestVersion                                                                                       bool
					dbCreatedAt, dbUpdatedAt                                                                                time.Time
				)

				row := db.QueryRowContext(ctx, `
					SELECT workflow_name, version, name, description, content, content_hash,
					       labels, status, execution_engine, is_latest_version, created_at, updated_at
					FROM remediation_workflow_catalog
					WHERE workflow_name = $1 AND version = $2
				`, workflowName, "v1.0.0")

				err = row.Scan(
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

				Expect(err).ToNot(HaveOccurred(), "Should retrieve workflow from database")

				// CRITICAL ASSERTIONS: Verify composite PK and all fields
				Expect(dbWorkflowName).To(Equal(workflowName), "workflow_name should match")
				Expect(dbVersion).To(Equal("v1.0.0"), "version should match")
				Expect(dbName).To(Equal("Test Workflow"))
				Expect(dbDescription).To(Equal("Integration test workflow"))
				Expect(dbContent).To(ContainSubstring("scale"))
				Expect(dbContentHash).To(Equal(contentHash))
				Expect(dbStatus).To(Equal("active"))
				Expect(dbExecutionEngine).To(Equal("argo-workflows"))
				Expect(dbIsLatestVersion).To(BeTrue(), "is_latest_version should be true")
				Expect(dbCreatedAt).ToNot(BeZero())
				Expect(dbUpdatedAt).ToNot(BeZero())

				// CRITICAL: Verify JSONB labels persisted correctly
				Expect(dbLabels).ToNot(BeEmpty(), "Labels should be persisted as JSONB")
				var persistedLabels map[string]string
				err = json.Unmarshal(dbLabels, &persistedLabels)
				Expect(err).ToNot(HaveOccurred())
				Expect(persistedLabels).To(HaveKeyWithValue("signal_type", "prometheus"))
				Expect(persistedLabels).To(HaveKeyWithValue("severity", "critical"))
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
					Severity:    "low",
					Component:   "test",
					Priority:    "P3",
					Environment: "test",
				}

				testWorkflow := &models.RemediationWorkflow{
					WorkflowName:    workflowName,
					Version:         "v1.0.0",
					Name:            "Original Workflow",
					Description:     "First workflow",
					Content:         content,
					ContentHash:     contentHash,
					Labels:          labels,
					CustomLabels:    models.CustomLabels{},
					DetectedLabels:  models.DetectedLabels{},
					Status:          "active",
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
				}

				err := workflowRepo.Create(ctx, testWorkflow)
				Expect(err).ToNot(HaveOccurred())

				// ACT: Attempt to create duplicate (same workflow_name + version)
				duplicateWorkflow := &models.RemediationWorkflow{
					WorkflowName:    workflowName, // Same name
					Version:         "v1.0.0",     // Same version
					Name:            "Duplicate Workflow",
					Description:     "Duplicate workflow",
					Content:         content,
					ContentHash:     contentHash,
					Labels:          labels,
					CustomLabels:    models.CustomLabels{},
					DetectedLabels:  models.DetectedLabels{},
					Status:          "active",
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
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
				Severity:    "low",
				Component:   "test",
				Priority:    "P3",
				Environment: "test",
			}

			testWorkflow := &models.RemediationWorkflow{
				WorkflowName:    workflowName,
				Version:         "v1.0.0",
				Name:            "Test Workflow Get",
				Description:     "Test workflow for Get method",
				Content:         content,
				ContentHash:     contentHash,
				Labels:          labels,
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "active",
				ExecutionEngine: "argo-workflows",
				IsLatestVersion: true,
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
				Expect(retrievedWorkflow.Description).To(Equal("Test workflow for Get method"))
				Expect(retrievedWorkflow.Status).To(Equal("active"))
				Expect(retrievedWorkflow.ExecutionEngine).To(Equal(models.ExecutionEngine("argo-workflows")))
				Expect(retrievedWorkflow.IsLatestVersion).To(BeTrue())
				Expect(retrievedWorkflow.CreatedAt).ToNot(BeZero())
				Expect(retrievedWorkflow.UpdatedAt).ToNot(BeZero())

				// CRITICAL: Verify structured labels deserialized correctly
				Expect(retrievedWorkflow.Labels.SignalType).To(Equal("test"))
				Expect(retrievedWorkflow.Labels.Severity).To(Equal("low"))
				Expect(retrievedWorkflow.Labels.Component).To(Equal("test"))
				Expect(retrievedWorkflow.Labels.Priority).To(Equal("P3"))
				Expect(retrievedWorkflow.Labels.Environment).To(Equal("test"))
			})
		})
	})

	// ========================================
	// LIST METHOD TESTS - FILTERING & PAGINATION
	// ========================================
	// ARCHITECTURAL NOTE: Serial Execution Required for List tests
	// remediation_workflow_catalog is a global table shared by ALL test processes.
	// List tests verify exact workflow counts (e.g., Expect(len).To(Equal(3)))
	// Parallel execution causes data contamination:
	// - Process 1: TRUNCATE → Create 3 workflows → List (expect 3)
	// - Process 2: Create 200 bulk workflows (during P1's test) → P1 finds 203 ❌
	// Decision: Serial execution prevents cross-process data contamination for count tests
	Describe("List", Serial, func() {
		var createdWorkflowNames []string

		BeforeEach(func() {
			// CRITICAL: Use public schema for workflow catalog tests
			// remediation_workflow_catalog is NOT schema-isolated - all parallel processes
			// share the same table. Without usePublicSchema(), each process sees different
			// data, causing cleanup to be ineffective and tests to see contaminated data.
			usePublicSchema()

			createdWorkflowNames = []string{} // Reset for each test

			// Cleanup any leftover test workflows from previous runs (data pollution fix)
			// V1.0 FIX: Correct SQL LIKE pattern - use single % for wildcard
			_, _ = db.ExecContext(ctx, `DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE $1`, "wf-repo-%-list-%")

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
					Severity:    "low",
					Component:   "test",
					Priority:    "P3",
					Environment: "test",
				}

				testWorkflow := &models.RemediationWorkflow{
					WorkflowName:    wf.name,
					Version:         wf.version,
					Name:            wf.name,
					Description:     "Test workflow",
					Content:         content,
					ContentHash:     contentHash,
					Labels:          labels,
					CustomLabels:    models.CustomLabels{},
					DetectedLabels:  models.DetectedLabels{},
					Status:          wf.status,
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
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
				Severity:    "low",
				Component:   "test",
				Priority:    "P3",
				Environment: "test",
			}

			testWorkflow := &models.RemediationWorkflow{
				WorkflowName:    workflowName,
				Version:         "v1.0.0",
				Name:            "Workflow to Update",
				Description:     "Test workflow for status update",
				Content:         content,
				ContentHash:     contentHash,
				Labels:          labels,
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "active",
				ExecutionEngine: "argo-workflows",
				IsLatestVersion: true,
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
