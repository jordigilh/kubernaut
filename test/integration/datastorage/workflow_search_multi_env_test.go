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
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
)

// ========================================
// WORKFLOW SEARCH - MULTI-ENVIRONMENT SUPPORT INTEGRATION TESTS (MODEL 2)
// ========================================
//
// Purpose: Test multi-environment workflow search functionality (Model 2 semantics)
// Authority: BR-STORAGE-040 v2.0 (Multi-Environment Support - Model 2)
// Design Decisions:
//   - DD-WORKFLOW-001 v2.5 (Environment: []string in storage, string in search)
//   - DD-WORKFLOW-004 v2.0 (JSONB ? operator for PostgreSQL array containment)
//
// Model 2 Semantics:
//   - STORAGE: Workflow declares environment: []string{"staging", "production"} (array)
//   - SEARCH: Signal Processing sends environment: "production" (single value)
//   - SQL: labels->'environment' ? 'production' OR labels->'environment' ? '*'
//   - WILDCARD: environment: []string{"*"} matches ALL environments
//
// Test Strategy:
// - Uses REAL PostgreSQL database (not mocks)
// - Validates PostgreSQL JSONB ? operator behavior (array containment)
// - Tests wildcard support (["*"] matches all)
// - Tests validation for empty environment string ("")
//
// ========================================

var _ = Describe("Workflow Search - Multi-Environment Support (Model 2)", Label("integration", "datastorage", "BR-STORAGE-040"), func() {
	var (
		ctx          context.Context
		workflowRepo *workflow.Repository
		testID       string
	)

	BeforeEach(func() {
		ctx = context.Background()
		workflowRepo = workflow.NewRepository(db, logger)
		testID = generateTestID()
	})

	AfterEach(func() {
		By("Cleaning up test workflows")
		// Clean up only this test's workflows (scoped by testID for parallel safety)
		_, err := db.ExecContext(ctx, "DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE $1",
			fmt.Sprintf("multi-env-v2-test-%%-%s", testID))
		Expect(err).ToNot(HaveOccurred())
	})

	Context("UT-DS-040-V2-001: Multi-Env Workflow - Single Environment Search Match", func() {
		It("should find workflow when stored array CONTAINS searched single value", func() {
			By("Seeding workflow with environment=['staging', 'production'] (array)")
			owner := "test-team"
			maintainer := "test@example.com"
		workflow := &models.RemediationWorkflow{
			WorkflowName:    "multi-env-v2-test-match-v1-" + testID,
			ActionType:      "ScaleReplicas", // Required: fk_workflow_action_type (migration 025)
			Version:         "1.0.0",
			Name:            "Multi-Env Workflow (Match)",
			Description:     models.StructuredDescription{What: "Workflow works in staging AND production", WhenToUse: "Testing"},
				Owner:           &owner,
				Maintainer:      &maintainer,
				Content:         "echo 'test'",
				ContentHash:     "abc123",
				ExecutionEngine: models.ExecutionEngineTekton,
				Labels: models.MandatoryLabels{
					SignalName:  "TestSignal-" + testID,
					Severity:    []string{"high"},
					Component:   "pod",
					Environment: []string{"staging", "production"}, // Workflow stored with array
					Priority:    "P1",
				},
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "active",
				IsLatestVersion: true, // Required for search to find this workflow
			}
			err := workflowRepo.Create(ctx, workflow)
			Expect(err).ToNot(HaveOccurred())
			Expect(workflow.WorkflowID).ToNot(BeEmpty(), "Workflow ID should be generated")

			By("Querying with single environment: 'production'")
			searchReq := &models.WorkflowSearchRequest{
				Filters: &models.WorkflowSearchFilters{
					SignalName:  "TestSignal-" + testID,
					Severity:    "high",
					Component:   "pod",
					Environment: "production", // Search: single string from Signal Processing
					Priority:    "P1",
				},
				TopK: 5,
			}

			By("Expecting workflow to be found (array contains 'production')")
			result, err := workflowRepo.SearchByLabels(ctx, searchReq)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Workflows).To(HaveLen(1), "Should find exactly 1 workflow")
			Expect(result.Workflows[0].WorkflowID).To(Equal(workflow.WorkflowID))
		})
	})

	Context("UT-DS-040-V2-002: Multi-Env Workflow - No Match", func() {
		It("should NOT find workflow when stored array does NOT contain searched value", func() {
			By("Seeding workflow with environment=['staging'] (array)")
			owner := "test-team"
			maintainer := "test@example.com"
		workflow := &models.RemediationWorkflow{
			WorkflowName:    "multi-env-v2-test-nomatch-v1-" + testID,
			ActionType:      "ScaleReplicas",
			Version:         "1.0.0",
			Name:            "Staging-Only Workflow",
			Description:     models.StructuredDescription{What: "Workflow only for staging", WhenToUse: "Testing"},
				Owner:           &owner,
				Maintainer:      &maintainer,
				Content:         "echo 'test'",
				ContentHash:     "abc124",
				ExecutionEngine: models.ExecutionEngineTekton,
				Labels: models.MandatoryLabels{
					SignalName:  "TestSignal-" + testID,
					Severity:    []string{"high"},
					Component:   "pod",
					Environment: []string{"staging"}, // Only staging
					Priority:    "P1",
				},
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "active",
				IsLatestVersion: true, // Required for search
			}
			err := workflowRepo.Create(ctx, workflow)
			Expect(err).ToNot(HaveOccurred())

			By("Querying with environment: 'production'")
			searchReq := &models.WorkflowSearchRequest{
				Filters: &models.WorkflowSearchFilters{
					SignalName:  "TestSignal-" + testID,
					Severity:    "high",
					Component:   "pod",
					Environment: "production", // Search for production (NOT in array)
					Priority:    "P1",
				},
				TopK: 5,
			}

			By("Expecting NO workflows to be found (array does NOT contain 'production')")
			result, err := workflowRepo.SearchByLabels(ctx, searchReq)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Workflows).To(BeEmpty(), "Should return empty results")
		})
	})

	Context("UT-DS-040-V2-003: Wildcard Workflow - Matches ALL Environments", func() {
		It("should find wildcard workflow for ANY environment search", func() {
			By("Seeding workflow with environment=['*'] (wildcard)")
			owner := "test-team"
			maintainer := "test@example.com"
		workflow := &models.RemediationWorkflow{
			WorkflowName:    "multi-env-v2-test-wildcard-v1-" + testID,
			ActionType:      "ScaleReplicas",
			Version:         "1.0.0",
			Name:            "Universal Workflow",
			Description:     models.StructuredDescription{What: "Workflow works in ALL environments", WhenToUse: "Testing"},
				Owner:           &owner,
				Maintainer:      &maintainer,
				Content:         "echo 'wildcard'",
				ContentHash:     "abc125",
				ExecutionEngine: models.ExecutionEngineTekton,
				Labels: models.MandatoryLabels{
					SignalName:  "TestSignal-" + testID,
					Severity:    []string{"high"},
					Component:   "pod",
					Environment: []string{"*"}, // Wildcard: matches ALL
					Priority:    "P1",
				},
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "active",
				IsLatestVersion: true, // Required for search
			}
			err := workflowRepo.Create(ctx, workflow)
			Expect(err).ToNot(HaveOccurred())

			By("Querying with environment: 'production'")
			searchReq1 := &models.WorkflowSearchRequest{
				Filters: &models.WorkflowSearchFilters{
					SignalName:  "TestSignal-" + testID,
					Severity:    "high",
					Component:   "pod",
					Environment: "production",
					Priority:    "P1",
				},
				TopK: 5,
			}

			By("Expecting wildcard workflow to be found")
			result1, err := workflowRepo.SearchByLabels(ctx, searchReq1)
			Expect(err).ToNot(HaveOccurred())
			Expect(result1.Workflows).To(HaveLen(1), "Should find wildcard workflow")
			Expect(result1.Workflows[0].WorkflowID).To(Equal(workflow.WorkflowID))

			By("Querying with environment: 'staging' (different environment)")
			searchReq2 := &models.WorkflowSearchRequest{
				Filters: &models.WorkflowSearchFilters{
					SignalName:  "TestSignal-" + testID,
					Severity:    "high",
					Component:   "pod",
					Environment: "staging",
					Priority:    "P1",
				},
				TopK: 5,
			}

			By("Expecting wildcard workflow to ALSO be found")
			result2, err := workflowRepo.SearchByLabels(ctx, searchReq2)
			Expect(err).ToNot(HaveOccurred())
			Expect(result2.Workflows).To(HaveLen(1), "Should find wildcard workflow for staging too")
			Expect(result2.Workflows[0].WorkflowID).To(Equal(workflow.WorkflowID))
		})
	})

	Context("UT-DS-040-V2-004: Validation - Empty Environment String", func() {
		It("should reject empty environment string", func() {
			By("Creating search request with empty environment string")
			searchReq := &models.WorkflowSearchRequest{
				Filters: &models.WorkflowSearchFilters{
					SignalName:  "TestSignal-" + testID,
					Severity:    "high",
					Component:   "pod",
					Environment: "", // Empty string (INVALID)
					Priority:    "P1",
				},
				TopK: 5,
			}

			By("Expecting validation error")
			result, err := workflowRepo.SearchByLabels(ctx, searchReq)
			Expect(err).To(HaveOccurred(), "Should return error for empty environment string")
			Expect(err.Error()).To(ContainSubstring("environment"), "Error message should mention environment")
			Expect(result).To(BeNil())
		})
	})

	Context("UT-DS-040-V2-005: Multiple Workflows - Different Environments", func() {
		It("should return only workflows where stored array contains searched value", func() {
			By("Seeding workflow 1 with environment=['production']")
			owner := "test-team"
			maintainer := "test@example.com"
		workflow1 := &models.RemediationWorkflow{
			WorkflowName:    "multi-env-v2-test-prod-v1-" + testID,
			ActionType:      "ScaleReplicas",
			Version:         "1.0.0",
			Name:            "Production Workflow",
			Description:     models.StructuredDescription{What: "Production environment only", WhenToUse: "Testing"},
				Owner:           &owner,
				Maintainer:      &maintainer,
				Content:         "echo 'prod'",
				ContentHash:     "abc126",
				ExecutionEngine: models.ExecutionEngineTekton,
				Labels: models.MandatoryLabels{
					SignalName:  "TestSignal-" + testID,
					Severity:    []string{"high"},
					Component:   "pod",
					Environment: []string{"production"},
					Priority:    "P1",
				},
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "active",
				IsLatestVersion: true, // Required for search
			}
			err := workflowRepo.Create(ctx, workflow1)
			Expect(err).ToNot(HaveOccurred())

			By("Seeding workflow 2 with environment=['staging']")
		workflow2 := &models.RemediationWorkflow{
			WorkflowName:    "multi-env-v2-test-staging-v1-" + testID,
			ActionType:      "ScaleReplicas",
			Version:         "1.0.0",
			Name:            "Staging Workflow",
			Description:     models.StructuredDescription{What: "Staging environment only", WhenToUse: "Testing"},
				Owner:           &owner,
				Maintainer:      &maintainer,
				Content:         "echo 'staging'",
				ContentHash:     "abc127",
				ExecutionEngine: models.ExecutionEngineTekton,
				Labels: models.MandatoryLabels{
					SignalName:  "TestSignal-" + testID,
					Severity:    []string{"high"},
					Component:   "pod",
					Environment: []string{"staging"},
					Priority:    "P1",
				},
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "active",
				IsLatestVersion: true, // Required for search
			}
			err = workflowRepo.Create(ctx, workflow2)
			Expect(err).ToNot(HaveOccurred())

			By("Seeding workflow 3 with environment=['staging', 'production']")
		workflow3 := &models.RemediationWorkflow{
			WorkflowName:    "multi-env-v2-test-both-v1-" + testID,
			ActionType:      "ScaleReplicas",
			Version:         "1.0.0",
			Name:            "Multi-Environment Workflow",
			Description:     models.StructuredDescription{What: "Works in both staging AND production", WhenToUse: "Testing"},
				Owner:           &owner,
				Maintainer:      &maintainer,
				Content:         "echo 'both'",
				ContentHash:     "abc128",
				ExecutionEngine: models.ExecutionEngineTekton,
				Labels: models.MandatoryLabels{
					SignalName:  "TestSignal-" + testID,
					Severity:    []string{"high"},
					Component:   "pod",
					Environment: []string{"staging", "production"},
					Priority:    "P1",
				},
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "active",
				IsLatestVersion: true, // Required for search
			}
			err = workflowRepo.Create(ctx, workflow3)
			Expect(err).ToNot(HaveOccurred())

			By("Querying with environment: 'production'")
			searchReq := &models.WorkflowSearchRequest{
				Filters: &models.WorkflowSearchFilters{
					SignalName:  "TestSignal-" + testID,
					Severity:    "high",
					Component:   "pod",
					Environment: "production", // Single value
					Priority:    "P1",
				},
				TopK: 5,
			}

			By("Expecting workflows 1 and 3 (both contain 'production'), NOT 2")
			result, err := workflowRepo.SearchByLabels(ctx, searchReq)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Workflows).To(HaveLen(2), "Should find exactly 2 workflows")

			workflowIDs := []string{result.Workflows[0].WorkflowID, result.Workflows[1].WorkflowID}
			Expect(workflowIDs).To(ContainElements(workflow1.WorkflowID, workflow3.WorkflowID))
			Expect(workflowIDs).ToNot(ContainElement(workflow2.WorkflowID), "Should NOT find staging-only workflow")
		})
	})

	Context("UT-DS-040-V2-006: Single-Environment Workflow - Exact Match", func() {
		It("should find workflow when stored array has single matching value", func() {
			By("Seeding workflow with environment=['development'] (single value in array)")
			owner := "test-team"
			maintainer := "test@example.com"
		workflow := &models.RemediationWorkflow{
			WorkflowName:    "multi-env-v2-test-dev-v1-" + testID,
			ActionType:      "ScaleReplicas",
			Version:         "1.0.0",
			Name:            "Development Workflow",
			Description:     models.StructuredDescription{What: "Development environment only", WhenToUse: "Testing"},
				Owner:           &owner,
				Maintainer:      &maintainer,
				Content:         "echo 'dev'",
				ContentHash:     "abc129",
				ExecutionEngine: models.ExecutionEngineTekton,
				Labels: models.MandatoryLabels{
					SignalName:  "TestSignal-" + testID,
					Severity:    []string{"high"},
					Component:   "pod",
					Environment: []string{"development"}, // Single value in array
					Priority:    "P1",
				},
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "active",
				IsLatestVersion: true, // Required for search
			}
			err := workflowRepo.Create(ctx, workflow)
			Expect(err).ToNot(HaveOccurred())

			By("Querying with environment: 'development'")
			searchReq := &models.WorkflowSearchRequest{
				Filters: &models.WorkflowSearchFilters{
					SignalName:  "TestSignal-" + testID,
					Severity:    "high",
					Component:   "pod",
					Environment: "development",
					Priority:    "P1",
				},
				TopK: 5,
			}

			By("Expecting workflow to be found")
			result, err := workflowRepo.SearchByLabels(ctx, searchReq)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Workflows).To(HaveLen(1), "Should find exactly 1 workflow")
			Expect(result.Workflows[0].WorkflowID).To(Equal(workflow.WorkflowID))
		})
	})
})
