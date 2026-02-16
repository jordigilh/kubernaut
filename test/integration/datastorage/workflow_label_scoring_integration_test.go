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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
)

// ========================================
// WORKFLOW LABEL SCORING INTEGRATION TESTS
// ========================================
//
// Purpose: Validate that label weights are correctly applied in workflow search scoring
//
// Authority:
// - DD-WORKFLOW-004 v1.5 (Fixed DetectedLabel Weights)
// - pkg/datastorage/repository/workflow/search.go (lines 417-513)
//
// Business Requirements:
// - BR-STORAGE-013: Semantic search with hybrid weighted scoring
// - BR-WORKFLOW-003: Workflow matching accuracy
//
// Test Strategy:
// - Uses REAL PostgreSQL database (not mocks)
// - Creates workflows with different DetectedLabels
// - Validates boost/penalty values in search results
// - Tests exact weight values (0.10, 0.05, 0.02)
//
// Coverage Gap Addressed:
// This file addresses the gap identified in DS_WEIGHTS_TEST_COVERAGE_ANALYSIS_DEC_17_2025.md
// where NO integration tests validated that label weights are correctly applied in scoring.
//
// Defense-in-Depth Strategy:
// - Unit tests: Validate constants (DELETED - tested dead code)
// - Integration tests (this file): Validate weights applied in SQL scoring
// - E2E tests: Validate complete workflow selection behavior
//
// ========================================

var _ = Describe("Workflow Label Scoring Integration Tests", func() {
	var (
		workflowRepo *workflow.Repository
		testID       string
	)

	BeforeEach(func() {
		// Create repository with real database (shared public schema)
		workflowRepo = workflow.NewRepository(db, logger)

		// Generate unique test ID for isolation
		// All test data uses testID-scoped names, so parallel processes don't collide
		testID = generateTestID()
	})

	AfterEach(func() {
		// Clean up test data
		if db != nil {
			_, _ = db.ExecContext(ctx,
				"DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE $1",
				fmt.Sprintf("wf-scoring-%s%%", testID))
		}
	})

	// ========================================
	// TEST 1: GitOps Weight (0.10 boost)
	// ========================================
	// BR-STORAGE-013: GitOps workflows should be ranked higher
	// Weight: 0.10 (high-impact)
	Describe("GitOps DetectedLabel Weight", func() {
		Context("when searching for GitOps workflows", func() {
			It("should apply 0.10 boost for GitOps-managed workflows", FlakeAttempts(3), func() {
				// ARRANGE: Create 2 workflows - one GitOps, one manual
				// Both have identical mandatory labels to isolate DetectedLabel impact
				content := `{"steps":[{"action":"scale","replicas":3}]}`
				contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

			gitopsWorkflow := &models.RemediationWorkflow{
				WorkflowName: fmt.Sprintf("wf-scoring-%s-gitops", testID),
				ActionType:   "ScaleReplicas", // Required: fk_workflow_action_type (migration 025)
				Version:      "v1.0",
				Name:         "GitOps Workflow",
				Description:  models.StructuredDescription{What: "Workflow managed by GitOps", WhenToUse: "Testing"},
					Content:      content,
					ContentHash:  contentHash,
					Labels: models.MandatoryLabels{
						SignalType:  "OOMKilled",
						Severity:    []string{"critical"},
						Component:   "pod",
						Environment: []string{"production"},
						Priority:    "P0",
					},
					CustomLabels: models.CustomLabels{}, // ✅ Empty map (NOT NULL constraint)
					DetectedLabels: models.DetectedLabels{
						GitOpsManaged: true, // ✅ +0.10 boost expected
					},
					Status:          "active",
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
				}

			manualWorkflow := &models.RemediationWorkflow{
				WorkflowName: fmt.Sprintf("wf-scoring-%s-manual", testID),
				ActionType:   "ScaleReplicas",
				Version:      "v1.0",
				Name:         "Manual Workflow",
				Description:  models.StructuredDescription{What: "Workflow without GitOps", WhenToUse: "Testing"},
					Content:      content,
					ContentHash:  contentHash,
					Labels: models.MandatoryLabels{
						SignalType:  "OOMKilled",
						Severity:    []string{"critical"},
						Component:   "pod",
						Environment: []string{"production"},
						Priority:    "P0",
					},
					CustomLabels: models.CustomLabels{}, // ✅ Empty map (NOT NULL constraint)
					DetectedLabels: models.DetectedLabels{
						GitOpsManaged: false, // ❌ No boost
					},
					Status:          "active",
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
				}

				// Persist workflows
				err := workflowRepo.Create(ctx, gitopsWorkflow)
				Expect(err).ToNot(HaveOccurred(), "GitOps workflow should be created")
				err = workflowRepo.Create(ctx, manualWorkflow)
				Expect(err).ToNot(HaveOccurred(), "Manual workflow should be created")

				// ACT: Search for workflows with GitOps requirement
				searchRequest := &models.WorkflowSearchRequest{
					Filters: &models.WorkflowSearchFilters{
						SignalType:  "OOMKilled",
						Severity:    "critical",
						Component:   "pod",
						Environment: "production",
						Priority:    "P0",
						DetectedLabels: models.DetectedLabels{
							GitOpsManaged: true, // Search wants GitOps
						},
					},
					TopK: 10,
				}

				// DS-FLAKY-004 FIX: Handle async workflow indexing/search - filter by workflow name to avoid parallel test pollution
				// NOTE: Parallel tests may create other workflows, so filter by WorkflowName (includes testID)
				var response *models.WorkflowSearchResponse
				var gitopsResult, manualResult *models.WorkflowSearchResult
				Eventually(func() bool {
					var err error
					response, err = workflowRepo.SearchByLabels(ctx, searchRequest)
					if err != nil {
						return false
					}

					// Filter to find OUR test workflows (by name which includes testID)
					gitopsResult = nil
					manualResult = nil
					for i := range response.Workflows {
						if response.Workflows[i].Title == gitopsWorkflow.Name {
							gitopsResult = &response.Workflows[i]
						}
						if response.Workflows[i].Title == manualWorkflow.Name {
							manualResult = &response.Workflows[i]
						}
					}

					// Success when both our workflows are found
					return gitopsResult != nil && manualResult != nil
				}, 10*time.Second, 200*time.Millisecond).Should(BeTrue(), "Both test workflows should be searchable (DS-FLAKY-006: increased timeout for parallel test contention)")

				// ASSERT: Found both our test workflows
				Expect(gitopsResult).ToNot(BeNil(), "GitOps workflow should be in results")
				Expect(manualResult).ToNot(BeNil(), "Manual workflow should be in results")

				// BUSINESS VALUE ASSERTIONS:
				// 1. GitOps workflow should have higher LabelBoost
				Expect(gitopsResult.LabelBoost).To(Equal(0.10),
					"GitOps workflow should have 0.10 boost (DD-WORKFLOW-004 v1.5)")

				Expect(manualResult.LabelBoost).To(Equal(0.0),
					"Manual workflow should have 0.0 boost (no matching labels)")

				// 2. GitOps workflow should be ranked first
				Expect(gitopsResult.Rank).To(BeNumerically("<", manualResult.Rank),
					"GitOps workflow should be ranked higher than manual workflow")

				// 3. GitOps workflow should have higher or equal final score (may be capped at 1.0)
				Expect(gitopsResult.FinalScore).To(BeNumerically(">=", manualResult.FinalScore),
					"GitOps workflow final score should be >= manual workflow (boost applied before capping)")

				// 4. If base similarity is high (near 1.0), final scores may both be capped at 1.0
				// The LabelBoost field (checked above) is the authoritative indicator of boost application
			})
		})
	})

	// ========================================
	// TEST 2: PDB Weight (0.05 boost)
	// ========================================
	// BR-STORAGE-013: Workflows with PDB protection should be ranked higher
	// Weight: 0.05 (medium-impact)
	Describe("PDB DetectedLabel Weight", func() {
		Context("when searching for PDB-protected workflows", func() {
			It("should apply 0.05 boost for PDB-protected workflows", func() {
				// ARRANGE: Create 2 workflows - one with PDB, one without
				content := `{"steps":[{"action":"scale","replicas":3}]}`
				contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
			pdbWorkflow := &models.RemediationWorkflow{
				WorkflowName: fmt.Sprintf("wf-scoring-%s-pdb", testID),
				ActionType:   "ScaleReplicas",
				Version:      "v1.0",
				Name:         "PDB-Protected Workflow",
				Description:  models.StructuredDescription{What: "Workflow with PodDisruptionBudget protection", WhenToUse: "Testing"},
					Labels: models.MandatoryLabels{
						SignalType:  "HighMemoryUsage",
						Severity:    []string{"high"},
						Component:   "deployment",
						Environment: []string{"production"},
						Priority:    "P1",
					},
					CustomLabels: models.CustomLabels{
						"test_run_id": {testID}, // 🎯 TEST ISOLATION: Unique ID per parallel test
					},
					DetectedLabels: models.DetectedLabels{
						PDBProtected: true, // ✅ +0.05 boost expected
					},
					Status:          "active",
					Content:         content,
					ContentHash:     contentHash,
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
				}

			noPdbWorkflow := &models.RemediationWorkflow{
				WorkflowName: fmt.Sprintf("wf-scoring-%s-nopdb", testID),
				ActionType:   "ScaleReplicas",
				Version:      "v1.0",
				Name:         "No PDB Workflow",
				Description:  models.StructuredDescription{What: "Workflow without PDB protection", WhenToUse: "Testing"},
					Labels: models.MandatoryLabels{
						SignalType:  "HighMemoryUsage",
						Severity:    []string{"high"},
						Component:   "deployment",
						Environment: []string{"production"},
						Priority:    "P1",
					},
					CustomLabels: models.CustomLabels{
						"test_run_id": {testID}, // 🎯 TEST ISOLATION: Unique ID per parallel test
					},
					DetectedLabels: models.DetectedLabels{
						PDBProtected: false, // ❌ No boost
					},
					Status:          "active",
					Content:         content,
					ContentHash:     contentHash,
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
				}

				// Persist workflows
				err := workflowRepo.Create(ctx, pdbWorkflow)
				Expect(err).ToNot(HaveOccurred())
				err = workflowRepo.Create(ctx, noPdbWorkflow)
				Expect(err).ToNot(HaveOccurred())

				// ACT: Search for PDB-protected workflows
				// NOTE: Don't add test_run_id to search filters - it would add +0.05 boost and pollute assertions
				searchRequest := &models.WorkflowSearchRequest{
					Filters: &models.WorkflowSearchFilters{
						SignalType:  "HighMemoryUsage",
						Severity:    "high",
						Component:   "deployment",
						Environment: "production",
						Priority:    "P1",
						DetectedLabels: models.DetectedLabels{
							PDBProtected: true, // Search wants PDB protection
						},
					},
					TopK: 10,
				}

				// Handle async workflow indexing/search - allow time for workflows to become searchable
				// NOTE: Parallel tests may create other workflows, so filter by WorkflowName (includes testID)
				var response *models.WorkflowSearchResponse
				var pdbResult, noPdbResult *models.WorkflowSearchResult
				Eventually(func() bool {
					var err error
					response, err = workflowRepo.SearchByLabels(ctx, searchRequest)
					if err != nil {
						return false
					}

					// Filter to find OUR test workflows (by name which includes testID)
					pdbResult = nil
					noPdbResult = nil
					for i := range response.Workflows {
						if response.Workflows[i].Title == pdbWorkflow.Name {
							pdbResult = &response.Workflows[i]
						}
						if response.Workflows[i].Title == noPdbWorkflow.Name {
							noPdbResult = &response.Workflows[i]
						}
					}

					// Success when both our workflows are found
					return pdbResult != nil && noPdbResult != nil
				}, 10*time.Second, 200*time.Millisecond).Should(BeTrue(), "Both test workflows should be searchable (DS-FLAKY-006: increased timeout for parallel test contention)")

				// ASSERT: Found both our test workflows
				Expect(pdbResult).ToNot(BeNil(), "PDB workflow should be found")
				Expect(noPdbResult).ToNot(BeNil(), "No-PDB workflow should be found")

				// BUSINESS VALUE ASSERTIONS:
				Expect(pdbResult.LabelBoost).To(Equal(0.05),
					"PDB-protected workflow should have 0.05 boost (DD-WORKFLOW-004 v1.5)")

				Expect(noPdbResult.LabelBoost).To(Equal(0.0),
					"Non-PDB workflow should have 0.0 boost")

				Expect(pdbResult.FinalScore).To(BeNumerically(">", noPdbResult.FinalScore),
					"PDB-protected workflow should have higher score")
			})
		})
	})

	// ========================================
	// TEST 3: GitOps Penalty (-0.10)
	// ========================================
	// BR-STORAGE-013: Workflows that don't match GitOps requirement should be penalized
	// Penalty: -0.10 (high-impact mismatch)
	Describe("GitOps DetectedLabel Penalty", func() {
		Context("when signal requires GitOps but workflow is manual", func() {
			It("should apply -0.10 penalty for GitOps mismatch", func() {
				// ARRANGE: Create manual workflow
				content := `{"steps":[{"action":"scale","replicas":3}]}`
				contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
			manualWorkflow := &models.RemediationWorkflow{
				WorkflowName: fmt.Sprintf("wf-scoring-%s-manual-penalty", testID),
				ActionType:   "ScaleReplicas",
				Version:      "v1.0",
				Name:         "Manual Workflow (GitOps Required)",
				Description:  models.StructuredDescription{What: "Manual workflow when GitOps is required", WhenToUse: "Testing"},
					Labels: models.MandatoryLabels{
						SignalType:  "DatabaseConnectionLeak",
						Severity:    []string{"critical"},
						Component:   "deployment",
						Environment: []string{"production"},
						Priority:    "P0",
					},
					CustomLabels: models.CustomLabels{
						"test_run_id": {testID}, // 🎯 TEST ISOLATION: Unique ID per parallel test
					},
					DetectedLabels: models.DetectedLabels{
						GitOpsManaged: false, // ❌ Mismatch: signal wants GitOps
					},
					Status:          "active",
					Content:         content,
					ContentHash:     contentHash,
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
				}

				err := workflowRepo.Create(ctx, manualWorkflow)
				Expect(err).ToNot(HaveOccurred())

				// ACT: Search with GitOps requirement
				// NOTE: Don't add test_run_id to avoid +0.05 boost pollution
				searchRequest := &models.WorkflowSearchRequest{
					Filters: &models.WorkflowSearchFilters{
						SignalType:  "DatabaseConnectionLeak",
						Severity:    "critical",
						Component:   "deployment",
						Environment: "production",
						Priority:    "P0",
						DetectedLabels: models.DetectedLabels{
							GitOpsManaged: true, // ⚠️ Signal REQUIRES GitOps
						},
					},
					TopK: 10,
				}

				// Handle async workflow indexing/search - allow time for workflow to become searchable
				// NOTE: Parallel tests may create other workflows, so filter by WorkflowName (includes testID)
				var response *models.WorkflowSearchResponse
				var result *models.WorkflowSearchResult
				Eventually(func() bool {
					var err error
					response, err = workflowRepo.SearchByLabels(ctx, searchRequest)
					if err != nil {
						return false
					}

					// Filter to find OUR test workflow (by name which includes testID)
					result = nil
					for i := range response.Workflows {
						if response.Workflows[i].Title == manualWorkflow.Name {
							result = &response.Workflows[i]
							break
						}
					}

					return result != nil
				}, 20*time.Second, 200*time.Millisecond).Should(BeTrue(), "Manual workflow should be searchable (DS-FLAKY-006: increased timeout to 20s for CI environment resource contention)")

				// ASSERT: Found our test workflow
				Expect(result).ToNot(BeNil(), "Manual workflow should be found")

				// BUSINESS VALUE ASSERTIONS:
				Expect(result.LabelPenalty).To(Equal(0.10),
					"Manual workflow should have 0.10 penalty for GitOps mismatch (DD-WORKFLOW-004 v1.5)")

				Expect(result.LabelBoost).To(Equal(0.0),
					"Manual workflow should have no boost")

				// Final score should be reduced by penalty
				// Since penalty is subtracted, a lower final score is expected
				Expect(result.FinalScore).To(BeNumerically("<", 0.95),
					"Final score should be reduced due to penalty")
			})
		})
	})

	// ========================================
	// TEST 4: Custom Label Boost (0.05 per key)
	// ========================================
	// BR-STORAGE-013: Workflows matching custom labels should get boost
	// Weight: 0.05 per custom label key (up to 10 keys = 0.50 max)
	Describe("Custom Label Boost", func() {
		Context("when workflows have matching custom labels", func() {
			It("should apply 0.05 boost per matching custom label key", FlakeAttempts(3), func() {
				// ARRANGE: Create workflows with different custom labels
				content := `{"steps":[{"action":"scale","replicas":3}]}`
				contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
			twoLabelsWorkflow := &models.RemediationWorkflow{
				WorkflowName: fmt.Sprintf("wf-scoring-%s-custom2", testID),
				ActionType:   "ScaleReplicas",
				Version:      "v1.0",
				Name:         "Workflow with 2 Custom Labels",
				Description:  models.StructuredDescription{What: "Testing custom label scoring", WhenToUse: "Testing"},
					Content:      content,
					ContentHash:  contentHash,
					Labels: models.MandatoryLabels{
						SignalType:  "CPUThrottling",
						Severity:    []string{"medium"},
						Component:   "pod",
						Environment: []string{"staging"},
						Priority:    "P2",
					},
					CustomLabels: models.CustomLabels{
						"team":       []string{"payments"},
						"constraint": []string{"cost-sensitive"},
					},
					Status:          "active",
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
				}

			oneLabelsWorkflow := &models.RemediationWorkflow{
				WorkflowName: fmt.Sprintf("wf-scoring-%s-custom1", testID),
				ActionType:   "ScaleReplicas",
				Version:      "v1.0",
				Name:         "Workflow with 1 Custom Label",
				Description:  models.StructuredDescription{What: "Testing custom label scoring", WhenToUse: "Testing"},
					Content:      content,
					ContentHash:  contentHash,
					Labels: models.MandatoryLabels{
						SignalType:  "CPUThrottling",
						Severity:    []string{"medium"},
						Component:   "pod",
						Environment: []string{"staging"},
						Priority:    "P2",
					},
					CustomLabels: models.CustomLabels{
						"team": []string{"payments"},
					},
					Status:          "active",
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
				}

				err := workflowRepo.Create(ctx, twoLabelsWorkflow)
				Expect(err).ToNot(HaveOccurred())
				err = workflowRepo.Create(ctx, oneLabelsWorkflow)
				Expect(err).ToNot(HaveOccurred())

				// ACT: Search with custom labels
				searchRequest := &models.WorkflowSearchRequest{
					Filters: &models.WorkflowSearchFilters{
						SignalType:  "CPUThrottling",
						Severity:    "medium",
						Component:   "pod",
						Environment: "staging",
						Priority:    "P2",
						CustomLabels: models.CustomLabels{
							"team":       []string{"payments"},
							"constraint": []string{"cost-sensitive"},
						},
					},
					TopK: 10,
				}

				// DS-FLAKY-005 FIX: Handle async workflow indexing/search - filter by workflow name to avoid parallel test pollution
				// NOTE: Parallel tests may create other workflows, so filter by WorkflowName (includes testID)
				var response *models.WorkflowSearchResponse
				var twoLabelsResult, oneLabelsResult *models.WorkflowSearchResult
				Eventually(func() bool {
					var err error
					response, err = workflowRepo.SearchByLabels(ctx, searchRequest)
					if err != nil {
						return false
					}

					// Filter to find OUR test workflows (by name which includes testID)
					twoLabelsResult = nil
					oneLabelsResult = nil
					for i := range response.Workflows {
						if response.Workflows[i].Title == twoLabelsWorkflow.Name {
							twoLabelsResult = &response.Workflows[i]
						}
						if response.Workflows[i].Title == oneLabelsWorkflow.Name {
							oneLabelsResult = &response.Workflows[i]
						}
					}

					// Success when both our workflows are found
					return twoLabelsResult != nil && oneLabelsResult != nil
				}, 10*time.Second, 200*time.Millisecond).Should(BeTrue(), "Both test workflows should be searchable (DS-FLAKY-006: increased timeout for parallel test contention)")

				// ASSERT: Found both our test workflows
				Expect(twoLabelsResult).ToNot(BeNil(), "Workflow with 2 custom labels should be found")
				Expect(oneLabelsResult).ToNot(BeNil(), "Workflow with 1 custom label should be found")

				// BUSINESS VALUE ASSERTIONS:
				// Workflow with 2 matching custom label keys should have 0.10 boost (2 * 0.05)
				Expect(twoLabelsResult.LabelBoost).To(BeNumerically(">=", 0.09),
					"Workflow with 2 custom labels should have ~0.10 boost (2 * 0.05)")

				// Workflow with 1 matching custom label key should have 0.05 boost
				Expect(oneLabelsResult.LabelBoost).To(BeNumerically(">=", 0.04),
					"Workflow with 1 custom label should have ~0.05 boost")

				// More custom labels = higher score
				Expect(twoLabelsResult.FinalScore).To(BeNumerically(">", oneLabelsResult.FinalScore),
					"Workflow with more matching custom labels should rank higher")
			})
		})
	})

	// ========================================
	// TEST 5: Wildcard Matching (Half Boost)
	// ========================================
	// BR-STORAGE-013: Wildcard matches should give half the boost
	// Weight: 0.05 for exact, 0.025 for wildcard (half of 0.05)
	Describe("Wildcard DetectedLabel Matching", func() {
		Context("when searching with wildcard service mesh requirement", func() {
			It("should apply half boost (0.025) for wildcard matches", func() {
				// ARRANGE: Create 2 workflows - one with specific mesh, one without
				content := `{"steps":[{"action":"scale","replicas":3}]}`
				contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
			istioWorkflow := &models.RemediationWorkflow{
				WorkflowName: fmt.Sprintf("wf-scoring-%s-istio", testID),
				ActionType:   "ScaleReplicas",
				Version:      "v1.0",
				Name:         "Istio Service Mesh Workflow",
				Description:  models.StructuredDescription{What: "Workflow for Istio service mesh", WhenToUse: "Testing"},
					Labels: models.MandatoryLabels{
						SignalType:  "NetworkLatency",
						Severity:    []string{"high"},
						Component:   "service",
						Environment: []string{"production"},
						Priority:    "P1",
					},
					CustomLabels: models.CustomLabels{
						"test_run_id": {testID}, // 🎯 TEST ISOLATION: Unique ID per parallel test
					},
					DetectedLabels: models.DetectedLabels{
						ServiceMesh: "istio", // Exact match: +0.05
					},
					Status:          "active",
					Content:         content,
					ContentHash:     contentHash,
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
				}

			noMeshWorkflow := &models.RemediationWorkflow{
				WorkflowName: fmt.Sprintf("wf-scoring-%s-nomesh", testID),
				ActionType:   "ScaleReplicas",
				Version:      "v1.0",
				Name:         "No Service Mesh Workflow",
				Description:  models.StructuredDescription{What: "Workflow without service mesh", WhenToUse: "Testing"},
					Labels: models.MandatoryLabels{
						SignalType:  "NetworkLatency",
						Severity:    []string{"high"},
						Component:   "service",
						Environment: []string{"production"},
						Priority:    "P1",
					},
					CustomLabels: models.CustomLabels{
						"test_run_id": {testID}, // 🎯 TEST ISOLATION: Unique ID per parallel test
					},
					DetectedLabels: models.DetectedLabels{
						ServiceMesh: "", // No mesh
					},
					Status:          "active",
					Content:         content,
					ContentHash:     contentHash,
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
				}

				err := workflowRepo.Create(ctx, istioWorkflow)
				Expect(err).ToNot(HaveOccurred())
				err = workflowRepo.Create(ctx, noMeshWorkflow)
				Expect(err).ToNot(HaveOccurred())

				// ACT: Search with wildcard service mesh (any mesh accepted)
				// NOTE: Don't add test_run_id to avoid +0.05 boost pollution
				searchRequest := &models.WorkflowSearchRequest{
					Filters: &models.WorkflowSearchFilters{
						SignalType:  "NetworkLatency",
						Severity:    "high",
						Component:   "service",
						Environment: "production",
						Priority:    "P1",
						DetectedLabels: models.DetectedLabels{
							ServiceMesh: "*", // ⚠️ Wildcard: ANY service mesh
						},
					},
					TopK: 10,
				}

				// Handle async workflow indexing/search - allow time for workflows to become searchable
				// NOTE: Parallel tests may create other workflows, so filter by WorkflowName (includes testID)
				var response *models.WorkflowSearchResponse
				var istioResult, noMeshResult *models.WorkflowSearchResult
				Eventually(func() bool {
					var err error
					response, err = workflowRepo.SearchByLabels(ctx, searchRequest)
					if err != nil {
						return false
					}

					// Filter to find OUR test workflows (by name which includes testID)
					istioResult = nil
					noMeshResult = nil
					for i := range response.Workflows {
						if response.Workflows[i].Title == istioWorkflow.Name {
							istioResult = &response.Workflows[i]
						}
						if response.Workflows[i].Title == noMeshWorkflow.Name {
							noMeshResult = &response.Workflows[i]
						}
					}

					return istioResult != nil && noMeshResult != nil
				}, 10*time.Second, 200*time.Millisecond).Should(BeTrue(), "Both test workflows should be searchable (DS-FLAKY-006: increased timeout for parallel test contention)")

				// ASSERT: Found both our test workflows
				Expect(istioResult).ToNot(BeNil(), "Istio workflow should be found")
				Expect(noMeshResult).ToNot(BeNil(), "No-mesh workflow should be found")

				// BUSINESS VALUE ASSERTIONS:
				// Wildcard match should give ~0.025 boost (half of 0.05)
				Expect(istioResult.LabelBoost).To(BeNumerically(">=", 0.02),
					"Wildcard match should give at least 0.02 boost (half of 0.05)")
				Expect(istioResult.LabelBoost).To(BeNumerically("<=", 0.03),
					"Wildcard match should give at most 0.03 boost (half of 0.05)")

				Expect(noMeshResult.LabelBoost).To(Equal(0.0),
					"No mesh workflow should have 0.0 boost")

				Expect(istioResult.FinalScore).To(BeNumerically(">", noMeshResult.FinalScore),
					"Workflow with service mesh should rank higher even with wildcard match")
			})
		})

		Context("when searching with exact service mesh requirement", func() {
			It("should apply full boost (0.05) for exact matches", func() {
				// ARRANGE: Reuse workflows from wildcard test
				content := `{"steps":[{"action":"scale","replicas":3}]}`
				contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
			istioWorkflow := &models.RemediationWorkflow{
				WorkflowName: fmt.Sprintf("wf-scoring-%s-istio-exact", testID),
				ActionType:   "ScaleReplicas",
				Version:      "v1.0",
				Name:         "Istio Service Mesh Workflow",
				Description:  models.StructuredDescription{What: "Workflow for Istio service mesh", WhenToUse: "Testing"},
					Labels: models.MandatoryLabels{
						SignalType:  "NetworkLatency",
						Severity:    []string{"high"},
						Component:   "service",
						Environment: []string{"production"},
						Priority:    "P1",
					},
					CustomLabels: models.CustomLabels{
						"test_run_id": {testID}, // 🎯 TEST ISOLATION: Unique ID per parallel test
					},
					DetectedLabels: models.DetectedLabels{
						ServiceMesh: "istio", // Exact match
					},
					Status:          "active",
					Content:         content,
					ContentHash:     contentHash,
					ExecutionEngine: "argo-workflows",
					IsLatestVersion: true,
				}

				err := workflowRepo.Create(ctx, istioWorkflow)
				Expect(err).ToNot(HaveOccurred())

				// ACT: Search with EXACT service mesh requirement
				// NOTE: Don't add test_run_id to avoid +0.05 boost pollution
				searchRequest := &models.WorkflowSearchRequest{
					Filters: &models.WorkflowSearchFilters{
						SignalType:  "NetworkLatency",
						Severity:    "high",
						Component:   "service",
						Environment: "production",
						Priority:    "P1",
						DetectedLabels: models.DetectedLabels{
							ServiceMesh: "istio", // ✅ Exact match: istio
						},
					},
					TopK: 10,
				}

				// Handle async workflow indexing/search - allow time for workflow to become searchable
				// NOTE: Parallel tests may create other workflows, so filter by WorkflowName (includes testID)
				var response *models.WorkflowSearchResponse
				var result *models.WorkflowSearchResult
				Eventually(func() bool {
					var err error
					response, err = workflowRepo.SearchByLabels(ctx, searchRequest)
					if err != nil {
						return false
					}

					// Filter to find OUR test workflow (by name which includes testID)
					result = nil
					for i := range response.Workflows {
						if response.Workflows[i].Title == istioWorkflow.Name {
							result = &response.Workflows[i]
							break
						}
					}

					return result != nil
				}, 10*time.Second, 200*time.Millisecond).Should(BeTrue(), "Test workflow should be searchable (DS-FLAKY-006: increased timeout for parallel test contention)")

				// ASSERT: Found our test workflow
				Expect(result).ToNot(BeNil(), "Istio workflow should be found")

				// BUSINESS VALUE ASSERTIONS:
				Expect(result.LabelBoost).To(Equal(0.05),
					"Exact service mesh match should give full 0.05 boost (DD-WORKFLOW-004 v1.5)")

				// Final score depends on base similarity (mandatory labels) + boost
				// Just verify boost was applied correctly (checked above)
			})
		})
	})
})
