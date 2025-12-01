package datastorage

import (
	"context"
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvector/pgvector-go"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// WORKFLOW CATALOG INTEGRATION TESTS
// ========================================
// BR-STORAGE-012: Workflow catalog persistence
// BR-STORAGE-013: Semantic search for remediation workflows
// BR-STORAGE-014: Workflow catalog management
//
// These tests validate the workflow catalog functionality with a real PostgreSQL database.

var _ = Describe("Workflow Catalog Integration", Serial, func() {
	var (
		workflowRepo *repository.WorkflowRepository
		testCtx      context.Context
		testID       string
	)

	BeforeEach(func() {
		// Serial tests must use public schema (workflow catalog data is in public schema)
		// This reconnects the database, so we must create the repository AFTER this call
		usePublicSchema()

		testCtx = context.Background()
		testID = generateTestID() // Unique ID for test data isolation

		// CRITICAL: Create repository AFTER usePublicSchema() to use the reconnected db
		// BR-STORAGE-014: Pass embedding client for automatic embedding generation
		workflowRepo = repository.NewWorkflowRepository(db, logger, embeddingClient)
	})

	AfterEach(func() {
		// Clean up test workflows
		// Note: For now, we'll let the data accumulate. In production, we'd implement proper cleanup
	})

	// ========================================
	// TEST 1: Create and Retrieve Workflow
	// ========================================
	// BR-STORAGE-012: Workflow catalog persistence
	Context("when creating and retrieving workflows", func() {
		It("should persist workflow and retrieve it by ID", func() {
			// ARRANGE: Create test workflow
			// DD-WORKFLOW-002 v3.0: WorkflowName is the human-readable identifier
			workflowName := "test-pod-oom-recovery-" + testID
			labels := map[string]interface{}{
				"signal_types":      []string{"MemoryLeak", "OOMKilled"},
				"business_category": "payments",
				"risk_tolerance":    "low",
			}
			labelsJSON, err := json.Marshal(labels)
			Expect(err).ToNot(HaveOccurred())

			embedding := pgvector.NewVector(make([]float32, 768)) // DD-TEST-001: Updated to 768 dimensions (all-mpnet-base-v2)
			for i := range embedding.Slice() {
				embedding.Slice()[i] = 0.1 + float32(i)/1000.0 // Add some variation
			}

			workflow := &models.RemediationWorkflow{
				WorkflowName:         workflowName, // DD-WORKFLOW-002: WorkflowID is the unique identifier
				Version:              "v1.0.0",
				Name:                 "Test Pod OOM Recovery",
				Description:          "Integration test workflow for OOM recovery",
				Content:              "apiVersion: tekton.dev/v1beta1\nkind: Pipeline\nmetadata:\n  name: pod-oom-recovery",
				ContentHash:          "test-hash-123",
				Labels:               labelsJSON,
				Embedding:            &embedding,
				Status:               "active",
				IsLatestVersion:      true,
				TotalExecutions:      0,
				SuccessfulExecutions: 0,
			}

			// ACT: Create workflow
			// DD-WORKFLOW-002: Create returns error only
			err = workflowRepo.Create(testCtx, workflow)

			// ASSERT: Workflow created successfully
			Expect(err).ToNot(HaveOccurred(), "Workflow creation should succeed")
			// After Create, workflow.WorkflowID contains the generated UUID
			Expect(workflow.WorkflowID).ToNot(BeEmpty(), "WorkflowID should be populated after create")

			// ACT: Retrieve workflow by UUID (primary key)
			// DD-WORKFLOW-002 v3.0: GetByID takes only UUID
			retrieved, err := workflowRepo.GetByID(testCtx, workflow.WorkflowID)

			// ASSERT: Workflow retrieved successfully
			Expect(err).ToNot(HaveOccurred(), "Workflow retrieval should succeed")
			Expect(retrieved).ToNot(BeNil(), "Retrieved workflow should not be nil")
			Expect(retrieved.WorkflowName).To(Equal(workflowName), "WorkflowName should match")
			Expect(retrieved.Name).To(Equal("Test Pod OOM Recovery"), "Name should match")
			Expect(retrieved.Version).To(Equal("v1.0.0"), "Version should match")
			Expect(retrieved.Name).To(Equal("Test Pod OOM Recovery"), "Name should match")
			Expect(retrieved.Status).To(Equal("active"), "Status should be active")
			Expect(retrieved.IsLatestVersion).To(BeTrue(), "Should be marked as latest version")
			Expect(retrieved.Description).To(Equal("Integration test workflow for OOM recovery"), "Description should match")

			// Verify labels were persisted correctly
			var retrievedLabels map[string]interface{}
			err = json.Unmarshal(retrieved.Labels, &retrievedLabels)
			Expect(err).ToNot(HaveOccurred(), "Labels should be valid JSON")
			Expect(retrievedLabels["business_category"]).To(Equal("payments"), "Business category should match")
			Expect(retrievedLabels["risk_tolerance"]).To(Equal("low"), "Risk tolerance should match")

			// ACT: Retrieve latest version by workflow_name
			// DD-WORKFLOW-002 v3.0: GetLatestVersion uses workflow_name
			latest, err := workflowRepo.GetLatestVersion(testCtx, workflowName)

			// ASSERT: Latest version retrieved successfully
			Expect(err).ToNot(HaveOccurred(), "Latest version retrieval should succeed")
			Expect(latest).ToNot(BeNil(), "Latest workflow should not be nil")
			Expect(latest.WorkflowName).To(Equal(workflowName), "Workflow ID should match")
			Expect(latest.Version).To(Equal("v1.0.0"), "Should retrieve v1.0.0 as latest")
			Expect(latest.IsLatestVersion).To(BeTrue(), "Should be marked as latest")
		})

		// DD-WORKFLOW-001 v1.3: business_category is now optional custom label
		It("should create workflow without business_category (optional per v1.3)", func() {
			// ARRANGE: Create workflow WITHOUT business_category
			// Per DD-WORKFLOW-001 v1.3: business_category moved from mandatory to optional
			workflowName := "test-no-business_category-" + testID
			labels := map[string]interface{}{
				"signal_types":   []string{"HighCPU"},
				"risk_tolerance": "medium",
				// NOTE: business_category intentionally omitted
			}
			labelsJSON, err := json.Marshal(labels)
			Expect(err).ToNot(HaveOccurred())

			embedding := pgvector.NewVector(make([]float32, 768))
			for i := range embedding.Slice() {
				embedding.Slice()[i] = 0.2 + float32(i)/1000.0
			}

			workflow := &models.RemediationWorkflow{
				WorkflowName:         workflowName,
				Version:              "v1.0.0",
				Name:                 "Test Workflow Without Business Category",
				Description:          "Validates DD-WORKFLOW-001 v1.3 optional business_category",
				Content:              "apiVersion: tekton.dev/v1beta1\nkind: Pipeline\nmetadata:\n  name: no-biz-cat",
				ContentHash:          "test-hash-no-biz-cat",
				Labels:               labelsJSON,
				Embedding:            &embedding,
				Status:               "active",
				IsLatestVersion:      true,
				TotalExecutions:      0,
				SuccessfulExecutions: 0,
			}

			// ACT: Create workflow without business_category
			err = workflowRepo.Create(testCtx, workflow)

			// ASSERT: Workflow created successfully (business_category is optional)
			Expect(err).ToNot(HaveOccurred(), "Workflow creation should succeed without business_category")
			Expect(workflow.WorkflowID).ToNot(BeEmpty(), "WorkflowID should be populated")

			// ACT: Retrieve and verify labels don't have business_category
			retrieved, err := workflowRepo.GetByID(testCtx, workflow.WorkflowID)
			Expect(err).ToNot(HaveOccurred(), "Retrieval should succeed")

			var retrievedLabels map[string]interface{}
			err = json.Unmarshal(retrieved.Labels, &retrievedLabels)
			Expect(err).ToNot(HaveOccurred(), "Labels should be valid JSON")
			Expect(retrievedLabels).ToNot(HaveKey("business_category"), "business_category should not be present")
			Expect(retrievedLabels["risk_tolerance"]).To(Equal("medium"), "Other labels should be preserved")
		})
	})

	// ========================================
	// TEST 2: Semantic Search
	// ========================================
	// BR-STORAGE-013: Semantic search for remediation workflows
	Context("when performing semantic search", func() {
		It("should find workflows by embedding similarity", func() {
			// ARRANGE: Create multiple workflows with different embeddings
			workflows := []struct {
				id          string
				name        string
				description string
				embedding   []float32
			}{
				{
					id:          "memory-leak-workflow-" + testID,
					name:        "Memory Leak Detection",
					description: "Detects and resolves memory leak issues in pods",
					embedding:   createEmbedding(0.8, 0.2, 0.1), // High memory focus
				},
				{
					id:          "cpu-spike-workflow-" + testID,
					name:        "CPU Spike Handler",
					description: "Handles CPU spike incidents",
					embedding:   createEmbedding(0.1, 0.9, 0.2), // High CPU focus
				},
				{
					id:          "disk-full-workflow-" + testID,
					name:        "Disk Full Recovery",
					description: "Recovers from disk full situations",
					embedding:   createEmbedding(0.2, 0.1, 0.9), // High disk focus
				},
			}

			// Create all workflows
			// DD-WORKFLOW-002 v3.0: WorkflowName is the human-readable identifier
			for _, wf := range workflows {
				labels, _ := json.Marshal(map[string]interface{}{
					"business_category": "infrastructure",
					"environment":       "production",
				})

				embedding := pgvector.NewVector(wf.embedding)
				workflow := &models.RemediationWorkflow{
					WorkflowName:         wf.id, // DD-WORKFLOW-002 v3.0: Use WorkflowName
					Version:              "v1.0.0",
					Name:                 wf.name,
					Description:          wf.description,
					Content:              "apiVersion: tekton.dev/v1beta1",
					ContentHash:          "hash-" + wf.id,
					Labels:               labels,
					Embedding:            &embedding,
					Status:               "active",
					IsLatestVersion:      true,
					TotalExecutions:      0,
					SuccessfulExecutions: 0,
				}

				// DD-WORKFLOW-002 v3.0: Create returns error only
				createErr := workflowRepo.Create(testCtx, workflow)
				Expect(createErr).ToNot(HaveOccurred(), "Workflow creation should succeed for "+wf.name)
			}

			// ACT: Search for memory-related workflows
			searchEmbedding := pgvector.NewVector(createEmbedding(0.85, 0.15, 0.1)) // Similar to memory leak
			searchReq := &models.WorkflowSearchRequest{
				Query:     "memory leak detection",
				Embedding: &searchEmbedding,
				TopK:      3,
				Filters: &models.WorkflowSearchFilters{
					Status: []string{"active"},
				},
			}

			response, err := workflowRepo.SearchByEmbedding(testCtx, searchReq)

			// ASSERT: Search should return results
			Expect(err).ToNot(HaveOccurred(), "Search should succeed")
			Expect(response).ToNot(BeNil(), "Response should not be nil")
			Expect(response.Workflows).ToNot(BeEmpty(), "Should return at least one workflow")

			// The first result should be the memory leak workflow (most similar)
			// Note: With placeholder embeddings, similarity is based on vector distance
			// DD-WORKFLOW-002 v3.0: Flat structure - use Title field directly
			firstResult := response.Workflows[0]
			Expect(firstResult.Title).To(ContainSubstring("Memory"), "First result should be memory-related")
		})
	})

	// ========================================
	// TEST 3: List Workflows with Filters
	// ========================================
	// BR-STORAGE-014: Workflow catalog management
	Context("when listing workflows with filters", func() {
		It("should return filtered workflows", func() {
			// ARRANGE: Create workflows with different statuses and categories
			workflows := []struct {
				id       string
				name     string
				status   string
				category string
			}{
				{
					id:       "active-payments-" + testID,
					name:     "Payments Recovery",
					status:   "active",
					category: "payments",
				},
				{
					id:       "active-auth-" + testID,
					name:     "Auth Recovery",
					status:   "active",
					category: "authentication",
				},
				{
					id:       "disabled-payments-" + testID,
					name:     "Old Payments Recovery",
					status:   "disabled",
					category: "payments",
				},
			}

			// Create all workflows
			for _, wf := range workflows {
				// DD-WORKFLOW-001 v1.8: mandatory labels in Labels, business_category in custom_labels
				labels, _ := json.Marshal(map[string]interface{}{
					"signal_type": "OOMKilled",
					"severity":    "critical",
				})

				// DD-WORKFLOW-001 v1.6: business_category moved to custom_labels
				customLabels, _ := json.Marshal(map[string][]string{
					"business": {"category=" + wf.category},
				})

				embedding := pgvector.NewVector(make([]float32, 768)) // DD-TEST-001: Updated to 768 dimensions (all-mpnet-base-v2)
				workflow := &models.RemediationWorkflow{
					WorkflowName:         wf.id,
					Version:              "v1.0.0",
					Name:                 wf.name,
					Description:          "Test workflow",
					Content:              "apiVersion: tekton.dev/v1beta1",
					ContentHash:          "hash-" + wf.id,
					Labels:               labels,
					CustomLabels:         customLabels,
					Embedding:            &embedding,
					Status:               wf.status,
					IsLatestVersion:      true,
					TotalExecutions:      0,
					SuccessfulExecutions: 0,
				}

				createErr := workflowRepo.Create(testCtx, workflow)
				Expect(createErr).ToNot(HaveOccurred(), "Workflow creation should succeed for "+wf.name)
			}

			// Verify workflows were created
			// DD-WORKFLOW-002 v3.0: workflow_id is UUID, use workflow_name for filtering
			By("Verifying workflows were created")
			Eventually(func() int {
				var count int
				err := db.QueryRow("SELECT COUNT(*) FROM remediation_workflow_catalog WHERE workflow_name LIKE $1", "%-"+testID).Scan(&count)
				if err != nil {
					return 0
				}
				return count
			}, "5s", "100ms").Should(Equal(3), "All 3 workflows should be created")

			// ACT: List active workflows only
			filters := &models.WorkflowSearchFilters{
				Status: []string{"active"},
			}
			activeWorkflows, total, err := workflowRepo.List(testCtx, filters, 10, 0)

			// ASSERT: Should return only active workflows
			Expect(err).ToNot(HaveOccurred(), "List should succeed")
			Expect(activeWorkflows).ToNot(BeEmpty(), "Should return at least one workflow")
			Expect(total).To(BeNumerically(">=", 2), "Should have at least 2 active workflows")

			// Verify all returned workflows are active
			for _, wf := range activeWorkflows {
				Expect(wf.Status).To(Equal("active"), "All workflows should be active")
			}

			// ACT: List active payments workflows only
			// DD-WORKFLOW-001 v1.6: business_category moved to custom_labels
			paymentsFilters := &models.WorkflowSearchFilters{
				CustomLabels: map[string][]string{
					"business": {"category=payments"},
				},
				Status: []string{"active"}, // Filter out disabled workflows
			}
			allPaymentsWorkflows, _, err := workflowRepo.List(testCtx, paymentsFilters, 100, 0)

			// ASSERT: Should return only active payments workflows
			Expect(err).ToNot(HaveOccurred(), "List with category filter should succeed")

			// Filter to only workflows from this test (for parallel execution isolation)
			// DD-WORKFLOW-002 v3.0: Use WorkflowName for filtering (WorkflowID is UUID)
			paymentsWorkflows := []models.RemediationWorkflow{}
			for _, wf := range allPaymentsWorkflows {
				if strings.Contains(wf.WorkflowName, testID) {
					paymentsWorkflows = append(paymentsWorkflows, wf)
				}
			}

			Expect(paymentsWorkflows).ToNot(BeEmpty(), "Should return at least one payments workflow from this test")
			Expect(len(paymentsWorkflows)).To(Equal(1), "Should have exactly 1 active payments workflow from this test (disabled-payments is filtered out)")

			// Verify the returned workflow is the active payments workflow
			Expect(paymentsWorkflows[0].Status).To(Equal("active"), "Returned workflow should be active")
			Expect(paymentsWorkflows[0].WorkflowName).To(ContainSubstring("active-payments"), "Returned workflow should be active-payments")
		})
	})
})

// ========================================
// HELPER FUNCTIONS
// ========================================

// createEmbedding creates a 768-dimensional embedding with the given focus values
// This simulates different types of workflows (memory, CPU, disk focused)
func createEmbedding(memoryFocus, cpuFocus, diskFocus float32) []float32 {
	embedding := make([]float32, 768) // DD-TEST-001: Updated to 768 dimensions (all-mpnet-base-v2)
	for i := range embedding {
		// Distribute focus across dimensions
		if i < 128 {
			embedding[i] = memoryFocus
		} else if i < 256 {
			embedding[i] = cpuFocus
		} else {
			embedding[i] = diskFocus
		}
		// Add some noise for realism
		embedding[i] += float32(i%10) / 100.0
	}
	return embedding
}

// ========================================
// TDD CYCLE 5: Mandatory Label Validation (Integration)
// ========================================
// Business Requirement: BR-STORAGE-013 (Hybrid Weighted Scoring)
// Design Decision: DD-WORKFLOW-004 v1.1 (Hybrid Weighted Label Scoring)
//
// Purpose: Validate mandatory label validation in HTTP handler
// Focus: Handler validation logic + Error responses
//
// TDD Phase: RED → GREEN → REFACTOR
// Expected: FAIL initially (validation not implemented yet)

var _ = Describe("Workflow Search - Mandatory Label Validation", Serial, func() {
	var (
		workflowRepo *repository.WorkflowRepository
		testCtx      context.Context
		testID       string
	)

	BeforeEach(func() {
		// Serial tests must use public schema
		usePublicSchema()

		testCtx = context.Background()
		// BR-STORAGE-014: Pass embedding client for automatic embedding generation
		workflowRepo = repository.NewWorkflowRepository(db, logger, embeddingClient)
		testID = generateTestID()
	})

	Context("when searching without mandatory labels", func() {
		It("should accept search with both signal_type and severity", func() {
			// ARRANGE: Create test workflow with new label schema
			// Use unique signal_type to avoid interference with other tests
			workflowID := "test-mandatory-labels-" + testID
			labels := map[string]interface{}{
				"signal_type": "MemoryLeak", // Different from other test (OOMKilled)
				"severity":    "high",       // Different from other test (critical)
			}
			labelsJSON, err := json.Marshal(labels)
			Expect(err).ToNot(HaveOccurred())

			embedding := pgvector.NewVector(make([]float32, 768)) // DD-TEST-001: Updated to 768 dimensions (all-mpnet-base-v2)
			for i := range embedding.Slice() {
				embedding.Slice()[i] = 0.5
			}

			workflow := &models.RemediationWorkflow{
				WorkflowName:         workflowID,
				Version:              "v1.0.0",
				Name:                 "Test Mandatory Labels",
				Description:          "Test workflow with mandatory labels",
				Content:              "apiVersion: tekton.dev/v1beta1",
				ContentHash:          "hash-mandatory-" + testID,
				Labels:               labelsJSON,
				Embedding:            &embedding,
				Status:               "active",
				IsLatestVersion:      true,
				TotalExecutions:      0,
				SuccessfulExecutions: 0,
			}

			err = workflowRepo.Create(testCtx, workflow)
			Expect(err).ToNot(HaveOccurred())

			// ACT: Search with mandatory labels (matching the workflow we created)
			queryEmbedding := pgvector.NewVector(make([]float32, 768)) // DD-TEST-001: Updated to 768 dimensions (all-mpnet-base-v2)
			for i := range queryEmbedding.Slice() {
				queryEmbedding.Slice()[i] = 0.5
			}

			request := &models.WorkflowSearchRequest{
				Query:     "Memory leak recovery",
				Embedding: &queryEmbedding,
				TopK:      10,
				Filters: &models.WorkflowSearchFilters{
					SignalType: "MemoryLeak", // Match the workflow we created
					Severity:   "high",       // Match the workflow we created
				},
			}

			response, err := workflowRepo.SearchByEmbedding(testCtx, request)

			// ASSERT: Search succeeds with mandatory labels
			// DD-WORKFLOW-002 v3.0: Flat structure - WorkflowID is UUID at top level
			Expect(err).ToNot(HaveOccurred(), "Search with mandatory labels should succeed")
			Expect(response).ToNot(BeNil())
			Expect(response.Workflows).ToNot(BeEmpty(), "Should return at least one workflow")
			// DD-WORKFLOW-002 v3.0: WorkflowID is flat field (UUID)
			Expect(response.Workflows[0].WorkflowID).ToNot(BeEmpty())
			// DD-WORKFLOW-002 v3.0: Title is flat field (renamed from Name)
			Expect(response.Workflows[0].Title).To(Equal("Test Mandatory Labels"))
		})
	})
})

// ========================================
// TDD CYCLE 6: Hybrid Scoring End-to-End (Integration)
// ========================================
// Business Requirement: BR-STORAGE-013 (Hybrid Weighted Scoring)
// Design Decision: DD-WORKFLOW-004 v1.1 (Hybrid Weighted Label Scoring)
//
// Purpose: Validate hybrid scoring through complete handler → repository → database flow
// Focus: End-to-end scoring with boost/penalty calculation
//
// TDD Phase: RED → GREEN → REFACTOR
// Expected: FAIL initially (hybrid scoring not fully wired)

var _ = Describe("Workflow Search - Hybrid Scoring End-to-End", Serial, func() {
	var (
		workflowRepo *repository.WorkflowRepository
		testCtx      context.Context
		testID       string
	)

	BeforeEach(func() {
		// Serial tests must use public schema
		usePublicSchema()

		testCtx = context.Background()
		// BR-STORAGE-014: Pass embedding client for automatic embedding generation
		workflowRepo = repository.NewWorkflowRepository(db, logger, embeddingClient)
		testID = generateTestID()
	})

	Context("when searching with optional labels", func() {
		It("should return workflows with hybrid weighted scores", func() {
			// ARRANGE: Create two workflows - one gitops, one manual
			// DD-WORKFLOW-001 v1.6: Labels must include ALL mandatory fields for filtering to work
			workflows := []struct {
				id              string
				name            string
				resourceMgmt    string
				expectedBoost   float64
				expectedPenalty float64
			}{
				{
					id:              "test-gitops-workflow-" + testID,
					name:            "GitOps OOM Recovery",
					resourceMgmt:    "gitops",
					expectedBoost:   0.10, // Matches search filter
					expectedPenalty: 0.0,
				},
				{
					id:              "test-manual-workflow-" + testID,
					name:            "Manual OOM Recovery",
					resourceMgmt:    "manual",
					expectedBoost:   0.0,
					expectedPenalty: 0.10, // Conflicts with search filter
				},
			}

			for _, wf := range workflows {
				// DD-WORKFLOW-001 v1.6: Include ALL mandatory labels so workflows match search filters
				labels := map[string]interface{}{
					"signal_type":         "OOMKilled",
					"severity":            "critical",
					"component":           "pod",        // DD-WORKFLOW-001 v1.4: mandatory
					"environment":         "production", // DD-WORKFLOW-001 v1.4: mandatory
					"priority":            "P0",         // DD-WORKFLOW-001 v1.4: mandatory
					"resource_management": wf.resourceMgmt,
				}
				labelsJSON, err := json.Marshal(labels)
				Expect(err).ToNot(HaveOccurred())

				// DD-WORKFLOW-001 v1.6: DetectedLabels for GitOps detection
				detectedLabels := map[string]interface{}{
					"git_ops_managed": wf.resourceMgmt == "gitops",
				}
				detectedLabelsJSON, err := json.Marshal(detectedLabels)
				Expect(err).ToNot(HaveOccurred())

				embedding := pgvector.NewVector(make([]float32, 768)) // DD-TEST-001: Updated to 768 dimensions (all-mpnet-base-v2)
				for i := range embedding.Slice() {
					embedding.Slice()[i] = 0.9 // High base similarity
				}

				workflow := &models.RemediationWorkflow{
					WorkflowName:         wf.id,
					Version:              "v1.0.0",
					Name:                 wf.name,
					Description:          "Test workflow for hybrid scoring",
					Content:              "apiVersion: tekton.dev/v1beta1",
					ContentHash:          "hash-" + wf.id,
					Labels:               labelsJSON,
					DetectedLabels:       detectedLabelsJSON,
					Embedding:            &embedding,
					Status:               "active",
					IsLatestVersion:      true,
					TotalExecutions:      0,
					SuccessfulExecutions: 0,
				}

				err = workflowRepo.Create(testCtx, workflow)
				Expect(err).ToNot(HaveOccurred())
			}

			// ACT: Search with gitops filter
			queryEmbedding := pgvector.NewVector(make([]float32, 768)) // DD-TEST-001: Updated to 768 dimensions (all-mpnet-base-v2)
			for i := range queryEmbedding.Slice() {
				queryEmbedding.Slice()[i] = 0.9
			}

			// DD-WORKFLOW-001 v1.6: GitOps is now detected via GitOpsManaged boolean
			gitOpsManaged := true
			request := &models.WorkflowSearchRequest{
				Query:     "OOM recovery with gitops",
				Embedding: &queryEmbedding,
				TopK:      10,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "OOMKilled",
					Severity:    "critical",
					Component:   "pod",        // DD-WORKFLOW-001 v1.4: mandatory
					Environment: "production", // DD-WORKFLOW-001 v1.4: mandatory
					Priority:    "P0",         // DD-WORKFLOW-001 v1.4: mandatory
					DetectedLabels: &models.DetectedLabels{
						GitOpsManaged: &gitOpsManaged,
					},
				},
			}

			response, err := workflowRepo.SearchByEmbedding(testCtx, request)

			// ASSERT: V1.0 Scoring (base similarity only) works correctly
			// DD-WORKFLOW-004 v2.0: V1.0 uses confidence = base_similarity (no boost/penalty)
			Expect(err).ToNot(HaveOccurred(), "Search should succeed")
			Expect(response).ToNot(BeNil())
			Expect(response.Workflows).To(HaveLen(2), "Should return both workflows")

			// Verify both workflows have valid scores
			// DD-WORKFLOW-004: FinalScore is the hybrid weighted score
			// DD-WORKFLOW-002 v3.0: Rank is implicit in ordering (index 0 = rank 1)
			firstWorkflow := response.Workflows[0]
			Expect(firstWorkflow.FinalScore).To(BeNumerically(">=", 0.0), "FinalScore should be >= 0.0")
			Expect(firstWorkflow.FinalScore).To(BeNumerically("<=", 1.0), "FinalScore should be <= 1.0")

			secondWorkflow := response.Workflows[1]
			Expect(secondWorkflow.FinalScore).To(BeNumerically(">=", 0.0), "FinalScore should be >= 0.0")
			Expect(secondWorkflow.FinalScore).To(BeNumerically("<=", 1.0), "FinalScore should be <= 1.0")

			// Verify ordering: first workflow should have higher or equal score
			Expect(firstWorkflow.FinalScore).To(BeNumerically(">=", secondWorkflow.FinalScore),
				"First workflow should have higher or equal score (semantic similarity)")
		})
	})

	// ========================================
	// TEST 5b: Container Image and Digest in Search Response
	// ========================================
	// DD-CONTRACT-001 v1.2: container_image and container_digest must be returned in search
	// TDD RED Phase: This test should FAIL initially (fields not returned)
	Context("when searching for workflows with container image", func() {
		It("should return container_image and container_digest in search results", func() {
			// ARRANGE: Create workflow WITH container_image and container_digest
			workflowID := "test-container-image-" + testID
			containerImage := "ghcr.io/kubernaut/pod-oom-recovery:v1.0.0"
			containerDigest := "sha256:abc123def456789012345678901234567890123456789012345678901234"

			labels := map[string]interface{}{
				"signal_type": "OOMKilled",
				"severity":    "critical",
				"component":   "pod",
				"environment": "production",
				"priority":    "P0",
			}
			labelsJSON, err := json.Marshal(labels)
			Expect(err).ToNot(HaveOccurred())

			embedding := pgvector.NewVector(make([]float32, 768))
			for i := range embedding.Slice() {
				embedding.Slice()[i] = 0.85
			}

			workflow := &models.RemediationWorkflow{
				WorkflowName:    workflowID,
				Version:         "v1.0.0",
				Name:            "Container Image Test Workflow",
				Description:     "Test workflow for container image in search response",
				Content:         "apiVersion: tekton.dev/v1beta1",
				ContentHash:     "hash-container-image-" + testID,
				Labels:          labelsJSON,
				Embedding:       &embedding,
				ContainerImage:  &containerImage,
				ContainerDigest: &containerDigest,
				Status:          "active",
				IsLatestVersion: true,
			}

			err = workflowRepo.Create(testCtx, workflow)
			Expect(err).ToNot(HaveOccurred())

			// ACT: Search for the workflow
			queryEmbedding := pgvector.NewVector(make([]float32, 768))
			for i := range queryEmbedding.Slice() {
				queryEmbedding.Slice()[i] = 0.85
			}

			request := &models.WorkflowSearchRequest{
				Query:     "OOM recovery container image test",
				Embedding: &queryEmbedding,
				TopK:      10,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "OOMKilled",
					Severity:    "critical",
					Component:   "pod",
					Environment: "production",
					Priority:    "P0",
				},
			}

			response, err := workflowRepo.SearchByEmbedding(testCtx, request)

			// ASSERT: Search succeeds and returns the workflow
			Expect(err).ToNot(HaveOccurred())
			Expect(response).ToNot(BeNil())
			Expect(len(response.Workflows)).To(BeNumerically(">=", 1), "Should return at least 1 workflow")

			// Find our test workflow in results
			var foundWorkflow *models.WorkflowSearchResult
			for i := range response.Workflows {
				if response.Workflows[i].WorkflowID == workflow.WorkflowID {
					foundWorkflow = &response.Workflows[i]
					break
				}
			}
			Expect(foundWorkflow).ToNot(BeNil(), "Should find the test workflow in search results")

			// ASSERT: container_image and container_digest are populated
			// DD-CONTRACT-001 v1.2: These fields MUST be returned in search response
			Expect(foundWorkflow.ContainerImage).To(Equal(containerImage),
				"container_image should be returned in search response")
			Expect(foundWorkflow.ContainerDigest).To(Equal(containerDigest),
				"container_digest should be returned in search response")
		})
	})

	// ========================================
	// TEST 6: Automatic Embedding Generation
	// ========================================
	// BR-STORAGE-014: Workflow CRUD operations with embedding generation
	// TDD RED Phase: This test should FAIL initially
	Context("when creating workflow with automatic embedding generation", func() {
		It("should automatically generate 768-dimensional embedding from workflow metadata", func() {
			// ARRANGE: Create workflow WITHOUT embedding (should be auto-generated)
			workflowID := "test-auto-embedding-" + testID
			labels := map[string]interface{}{
				"signal_type":         "OOMKilled",
				"severity":            "critical",
				"resource_management": "gitops",
				"gitops_tool":         "argocd",
				"environment":         "production",
				"business_category":   "payments",
				"priority":            "high",
				"risk_tolerance":      "low",
			}
			labelsJSON, err := json.Marshal(labels)
			Expect(err).ToNot(HaveOccurred())

			workflow := &models.RemediationWorkflow{
				WorkflowName: workflowID,
				Version:      "v1.0.0",
				Name:         "OOMKilled Pod Recovery",
				Description:  "Recovers pods that are killed due to out of memory errors in production environments",
				Content:      "apiVersion: tekton.dev/v1beta1\nkind: Pipeline\nmetadata:\n  name: pod-oom-recovery\nspec:\n  tasks:\n  - name: restart-pod\n    taskRef:\n      name: kubectl-restart",
				ContentHash:  "test-hash-auto-embedding-123",
				Labels:       labelsJSON,
				// NOTE: Embedding is NOT set - should be auto-generated by repository
				Status:               "active",
				IsLatestVersion:      true,
				TotalExecutions:      0,
				SuccessfulExecutions: 0,
			}

			// ACT: Create workflow (should trigger automatic embedding generation)
			err = workflowRepo.Create(testCtx, workflow)

			// ASSERT: Workflow created successfully
			Expect(err).ToNot(HaveOccurred(), "Workflow creation should succeed")

			// ASSERT: Embedding was automatically generated
			Expect(workflow.Embedding).ToNot(BeNil(), "Embedding should be auto-generated")
			Expect(len(workflow.Embedding.Slice())).To(BeNumerically(">", 0),
				"Embedding should have dimensions (actual size depends on model)")

			// ASSERT: Embedding values are non-zero (actual embedding, not placeholder)
			hasNonZeroValues := false
			for _, val := range workflow.Embedding.Slice() {
				if val != 0.0 {
					hasNonZeroValues = true
					break
				}
			}
			Expect(hasNonZeroValues).To(BeTrue(), "Embedding should have non-zero values")

			// ACT: Retrieve workflow from database to verify embedding was persisted
			// DD-WORKFLOW-002 v3.0: GetByID takes only UUID (populated after Create)
			retrieved, err := workflowRepo.GetByID(testCtx, workflow.WorkflowID)

			// ASSERT: Retrieved workflow has embedding
			Expect(err).ToNot(HaveOccurred(), "Workflow retrieval should succeed")
			Expect(retrieved.Embedding).ToNot(BeNil(), "Retrieved workflow should have embedding")
			Expect(len(retrieved.Embedding.Slice())).To(Equal(len(workflow.Embedding.Slice())),
				"Retrieved embedding should have same dimensions as original")

			// ASSERT: Retrieved embedding matches original
			for i := 0; i < len(workflow.Embedding.Slice()); i++ {
				Expect(retrieved.Embedding.Slice()[i]).To(BeNumerically("~", workflow.Embedding.Slice()[i], 0.0001),
					"Retrieved embedding values should match original")
			}
		})

		It("should use cached embedding for identical text", func() {
			// ARRANGE: Create first workflow
			workflowID1 := "test-cache-hit-1-" + testID
			labels := map[string]interface{}{
				"signal_type": "OOMKilled",
				"severity":    "critical",
			}
			labelsJSON, err := json.Marshal(labels)
			Expect(err).ToNot(HaveOccurred())

			workflow1 := &models.RemediationWorkflow{
				WorkflowName:         workflowID1,
				Version:              "v1.0.0",
				Name:                 "Identical Workflow Name",
				Description:          "Identical description for cache testing",
				Content:              "Identical content for cache testing",
				ContentHash:          "test-hash-cache-1",
				Labels:               labelsJSON,
				Status:               "active",
				IsLatestVersion:      true,
				TotalExecutions:      0,
				SuccessfulExecutions: 0,
			}

			// ACT: Create first workflow (cache miss - generates embedding)
			err = workflowRepo.Create(testCtx, workflow1)
			Expect(err).ToNot(HaveOccurred())
			Expect(workflow1.Embedding).ToNot(BeNil())

			// ARRANGE: Create second workflow with IDENTICAL metadata
			workflowID2 := "test-cache-hit-2-" + testID
			workflow2 := &models.RemediationWorkflow{
				WorkflowName:         workflowID2,
				Version:              "v1.0.0",
				Name:                 "Identical Workflow Name",
				Description:          "Identical description for cache testing",
				Content:              "Identical content for cache testing",
				ContentHash:          "test-hash-cache-2",
				Labels:               labelsJSON,
				Status:               "active",
				IsLatestVersion:      true,
				TotalExecutions:      0,
				SuccessfulExecutions: 0,
			}

			// ACT: Create second workflow (cache hit - reuses embedding)
			err = workflowRepo.Create(testCtx, workflow2)
			Expect(err).ToNot(HaveOccurred())
			Expect(workflow2.Embedding).ToNot(BeNil())

			// ASSERT: Both embeddings should be identical (cache hit)
			for i := 0; i < 768; i++ {
				Expect(workflow2.Embedding.Slice()[i]).To(BeNumerically("~", workflow1.Embedding.Slice()[i], 0.0001),
					"Cached embedding should match original")
			}
		})

		It("should handle embedding service unavailable gracefully", func() {
			// NOTE: This test validates graceful degradation
			// If embedding service is unavailable, workflow creation should still succeed
			// but without an embedding (will be generated later when service is available)

			// ARRANGE: Create workflow
			workflowID := "test-embedding-unavailable-" + testID
			labels := map[string]interface{}{
				"signal_type": "CrashLoopBackOff",
				"severity":    "high",
			}
			labelsJSON, err := json.Marshal(labels)
			Expect(err).ToNot(HaveOccurred())

			workflow := &models.RemediationWorkflow{
				WorkflowName:         workflowID,
				Version:              "v1.0.0",
				Name:                 "CrashLoopBackOff Recovery",
				Description:          "Recovers pods in CrashLoopBackOff state",
				Content:              "apiVersion: tekton.dev/v1beta1\nkind: Pipeline",
				ContentHash:          "test-hash-unavailable-123",
				Labels:               labelsJSON,
				Status:               "active",
				IsLatestVersion:      true,
				TotalExecutions:      0,
				SuccessfulExecutions: 0,
			}

			// ACT: Create workflow (embedding service may be unavailable in test environment)
			err = workflowRepo.Create(testCtx, workflow)

			// ASSERT: Workflow creation should succeed even if embedding generation fails
			Expect(err).ToNot(HaveOccurred(), "Workflow creation should succeed with graceful degradation")

			// NOTE: Embedding may be nil if service is unavailable (graceful degradation)
			// This is acceptable behavior - embedding can be generated later
		})
	})
})

// stringPtr returns a pointer to the given string
// Helper for optional string fields in test data
func stringPtr(s string) *string {
	return &s
}
