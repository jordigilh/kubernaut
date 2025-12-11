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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvector/pgvector-go"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// V1.0 SCORING TESTS - DD-WORKFLOW-004 v2.0
// ========================================
// Authority: DD-WORKFLOW-004 v2.0 (Hybrid Weighted Label Scoring)
// Business Requirement: BR-STORAGE-013 (Semantic Search API)
//
// V1.0 Decision (DD-WORKFLOW-004 v2.0):
//   - NO boost/penalty logic in V1.0
//   - confidence = base_similarity (cosine similarity only)
//   - LabelBoost = 0.0 (always)
//   - LabelPenalty = 0.0 (always)
//   - FinalScore = BaseSimilarity
//
// V2.0+ Roadmap: Customer-configurable label weights (deferred)
//
// TDD Phase: RED → GREEN → REFACTOR
// ========================================

var _ = Describe("V1.0 Scoring per DD-WORKFLOW-004 v2.0", Serial, Label("v1-scoring", "dd-workflow-004"), func() {
	var (
		workflowRepo *repository.WorkflowRepository
		testCtx      context.Context
		testID       string
	)

	BeforeEach(func() {
		// Serial tests must use public schema
		usePublicSchema()

		testCtx = context.Background()
		testID = generateTestID()

		// BR-STORAGE-014: Pass embedding client for automatic embedding generation
		workflowRepo = repository.NewWorkflowRepository(db, logger, embeddingClient)
	})

	// ========================================
	// TDD TEST 1: LabelBoost = 0.0 Always
	// ========================================
	// DD-WORKFLOW-004 v2.0: V1.0 has no boost logic
	Context("LabelBoost is always 0.0 (DD-WORKFLOW-004 v2.0)", func() {
		It("should return LabelBoost = 0.0 for workflows with matching DetectedLabels", func() {
			// ARRANGE: Create workflow with GitOps detected labels
			labels, _ := json.Marshal(map[string]interface{}{
				"signal_type": "OOMKilled",
				"severity":    "critical",
				"component":   "pod",
				"environment": "production",
				"priority":    "P0",
			})

			detectedLabels, _ := json.Marshal(map[string]interface{}{
				"git_ops_managed": true,
				"git_ops_tool":    "argocd", // Would have gotten boost in v1.1
			})

			embedding := pgvector.NewVector(make([]float32, 768))
			for i := range embedding.Slice() {
				embedding.Slice()[i] = 0.8
			}

			workflow := &models.RemediationWorkflow{
				WorkflowName:         "v1-scoring-boost-test-" + testID,
				Version:              "v1.0.0",
				Name:                 "GitOps OOM Recovery",
				Description:          "Recovery workflow with GitOps labels",
				Content:              "apiVersion: tekton.dev/v1beta1",
				ContentHash:          "hash-boost-" + testID,
				Labels:               labels,
				DetectedLabels:       detectedLabels,
				Embedding:            &embedding,
				Status:               "active",
				IsLatestVersion:      true,
				TotalExecutions:      0,
				SuccessfulExecutions: 0,
			}

			err := workflowRepo.Create(testCtx, workflow)
			Expect(err).ToNot(HaveOccurred(), "Workflow creation should succeed")

			// ACT: Search with matching DetectedLabels filter
			queryEmbedding := pgvector.NewVector(make([]float32, 768))
			for i := range queryEmbedding.Slice() {
				queryEmbedding.Slice()[i] = 0.8
			}

			gitOpsManaged := true
			gitOpsTool := "argocd"
			request := &models.WorkflowSearchRequest{
				Query:     "OOM recovery",
				Embedding: &queryEmbedding,
				TopK:      10,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "OOMKilled",
					Severity:    "critical",
					Component:   "pod",
					Environment: "production",
					Priority:    "P0",
					DetectedLabels: &models.DetectedLabels{
						GitOpsManaged: &gitOpsManaged,
						GitOpsTool:    &gitOpsTool, // Would have boosted in v1.1
					},
				},
			}

			response, err := workflowRepo.SearchByEmbedding(testCtx, request)

			// ASSERT: LabelBoost = 0.0 per DD-WORKFLOW-004 v2.0
			Expect(err).ToNot(HaveOccurred(), "Search should succeed")
			Expect(response).ToNot(BeNil())
			Expect(response.Workflows).ToNot(BeEmpty(), "Should return at least one workflow")

			// V1.0 SCORING ASSERTION: LabelBoost = 0.0
			firstWorkflow := response.Workflows[0]
			Expect(firstWorkflow.LabelBoost).To(Equal(0.0),
				"DD-WORKFLOW-004 v2.0: LabelBoost must be 0.0 in V1.0 (no boost logic)")
		})
	})

	// ========================================
	// TDD TEST 2: LabelPenalty = 0.0 Always
	// ========================================
	// DD-WORKFLOW-004 v2.0: V1.0 has no penalty logic
	Context("LabelPenalty is always 0.0 (DD-WORKFLOW-004 v2.0)", func() {
		It("should return LabelPenalty = 0.0 for workflows with conflicting DetectedLabels", func() {
			// ARRANGE: Create workflow with Flux (would conflict with ArgoCD search)
			labels, _ := json.Marshal(map[string]interface{}{
				"signal_type": "MemoryLeak",
				"severity":    "high",
				"component":   "deployment",
				"environment": "staging",
				"priority":    "P1",
			})

			detectedLabels, _ := json.Marshal(map[string]interface{}{
				"git_ops_managed": true,
				"git_ops_tool":    "flux", // Would have gotten penalty in v1.1 if searching for argocd
			})

			embedding := pgvector.NewVector(make([]float32, 768))
			for i := range embedding.Slice() {
				embedding.Slice()[i] = 0.7
			}

			workflow := &models.RemediationWorkflow{
				WorkflowName:         "v1-scoring-penalty-test-" + testID,
				Version:              "v1.0.0",
				Name:                 "Flux Memory Recovery",
				Description:          "Recovery workflow with Flux labels",
				Content:              "apiVersion: tekton.dev/v1beta1",
				ContentHash:          "hash-penalty-" + testID,
				Labels:               labels,
				DetectedLabels:       detectedLabels,
				Embedding:            &embedding,
				Status:               "active",
				IsLatestVersion:      true,
				TotalExecutions:      0,
				SuccessfulExecutions: 0,
			}

			err := workflowRepo.Create(testCtx, workflow)
			Expect(err).ToNot(HaveOccurred(), "Workflow creation should succeed")

			// ACT: Search with NO DetectedLabels filter (so Flux workflow is returned)
			queryEmbedding := pgvector.NewVector(make([]float32, 768))
			for i := range queryEmbedding.Slice() {
				queryEmbedding.Slice()[i] = 0.7
			}

			request := &models.WorkflowSearchRequest{
				Query:     "Memory recovery",
				Embedding: &queryEmbedding,
				TopK:      10,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "MemoryLeak",
					Severity:    "high",
					Component:   "deployment",
					Environment: "staging",
					Priority:    "P1",
					// No DetectedLabels filter - workflow should still have LabelPenalty = 0.0
				},
			}

			response, err := workflowRepo.SearchByEmbedding(testCtx, request)

			// ASSERT: LabelPenalty = 0.0 per DD-WORKFLOW-004 v2.0
			Expect(err).ToNot(HaveOccurred(), "Search should succeed")
			Expect(response).ToNot(BeNil())
			Expect(response.Workflows).ToNot(BeEmpty(), "Should return at least one workflow")

			// V1.0 SCORING ASSERTION: LabelPenalty = 0.0
			firstWorkflow := response.Workflows[0]
			Expect(firstWorkflow.LabelPenalty).To(Equal(0.0),
				"DD-WORKFLOW-004 v2.0: LabelPenalty must be 0.0 in V1.0 (no penalty logic)")
		})
	})

	// ========================================
	// TDD TEST 3: FinalScore = BaseSimilarity
	// ========================================
	// DD-WORKFLOW-004 v2.0: V1.0 scoring = base similarity only
	Context("FinalScore equals BaseSimilarity (DD-WORKFLOW-004 v2.0)", func() {
		It("should have FinalScore equal to BaseSimilarity for all workflows", func() {
			// ARRANGE: Create a simple workflow
			labels, _ := json.Marshal(map[string]interface{}{
				"signal_type": "CrashLoopBackOff",
				"severity":    "medium",
				"component":   "pod",
				"environment": "development",
				"priority":    "P2",
			})

			embedding := pgvector.NewVector(make([]float32, 768))
			for i := range embedding.Slice() {
				embedding.Slice()[i] = 0.6
			}

			workflow := &models.RemediationWorkflow{
				WorkflowName:         "v1-scoring-final-test-" + testID,
				Version:              "v1.0.0",
				Name:                 "CrashLoop Recovery",
				Description:          "Recovery workflow for testing final score",
				Content:              "apiVersion: tekton.dev/v1beta1",
				ContentHash:          "hash-final-" + testID,
				Labels:               labels,
				Embedding:            &embedding,
				Status:               "active",
				IsLatestVersion:      true,
				TotalExecutions:      0,
				SuccessfulExecutions: 0,
			}

			err := workflowRepo.Create(testCtx, workflow)
			Expect(err).ToNot(HaveOccurred(), "Workflow creation should succeed")

			// ACT: Search for the workflow
			queryEmbedding := pgvector.NewVector(make([]float32, 768))
			for i := range queryEmbedding.Slice() {
				queryEmbedding.Slice()[i] = 0.6
			}

			request := &models.WorkflowSearchRequest{
				Query:     "CrashLoop recovery",
				Embedding: &queryEmbedding,
				TopK:      10,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "CrashLoopBackOff",
					Severity:    "medium",
					Component:   "pod",
					Environment: "development",
					Priority:    "P2",
				},
			}

			response, err := workflowRepo.SearchByEmbedding(testCtx, request)

			// ASSERT: FinalScore = BaseSimilarity per DD-WORKFLOW-004 v2.0
			Expect(err).ToNot(HaveOccurred(), "Search should succeed")
			Expect(response).ToNot(BeNil())
			Expect(response.Workflows).ToNot(BeEmpty(), "Should return at least one workflow")

			// V1.0 SCORING ASSERTION: FinalScore = BaseSimilarity
			firstWorkflow := response.Workflows[0]
			Expect(firstWorkflow.FinalScore).To(Equal(firstWorkflow.BaseSimilarity),
				"DD-WORKFLOW-004 v2.0: FinalScore must equal BaseSimilarity in V1.0 (no boost/penalty)")
		})
	})

	// ========================================
	// TDD TEST 4: Confidence = BaseSimilarity
	// ========================================
	// DD-WORKFLOW-004 v2.0: confidence = base_similarity (API contract)
	Context("Confidence equals BaseSimilarity (DD-WORKFLOW-004 v2.0)", func() {
		It("should have Confidence equal to BaseSimilarity for API contract compliance", func() {
			// ARRANGE: Create a workflow
			labels, _ := json.Marshal(map[string]interface{}{
				"signal_type": "NodeNotReady",
				"severity":    "critical",
				"component":   "node",
				"environment": "production",
				"priority":    "P0",
			})

			embedding := pgvector.NewVector(make([]float32, 768))
			for i := range embedding.Slice() {
				embedding.Slice()[i] = 0.9
			}

			workflow := &models.RemediationWorkflow{
				WorkflowName:         "v1-scoring-confidence-test-" + testID,
				Version:              "v1.0.0",
				Name:                 "Node Recovery",
				Description:          "Recovery workflow for testing confidence",
				Content:              "apiVersion: tekton.dev/v1beta1",
				ContentHash:          "hash-confidence-" + testID,
				Labels:               labels,
				Embedding:            &embedding,
				Status:               "active",
				IsLatestVersion:      true,
				TotalExecutions:      0,
				SuccessfulExecutions: 0,
			}

			err := workflowRepo.Create(testCtx, workflow)
			Expect(err).ToNot(HaveOccurred(), "Workflow creation should succeed")

			// ACT: Search for the workflow
			queryEmbedding := pgvector.NewVector(make([]float32, 768))
			for i := range queryEmbedding.Slice() {
				queryEmbedding.Slice()[i] = 0.9
			}

			request := &models.WorkflowSearchRequest{
				Query:     "Node recovery",
				Embedding: &queryEmbedding,
				TopK:      10,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "NodeNotReady",
					Severity:    "critical",
					Component:   "node",
					Environment: "production",
					Priority:    "P0",
				},
			}

			response, err := workflowRepo.SearchByEmbedding(testCtx, request)

			// ASSERT: Confidence = BaseSimilarity per DD-WORKFLOW-004 v2.0
			Expect(err).ToNot(HaveOccurred(), "Search should succeed")
			Expect(response).ToNot(BeNil())
			Expect(response.Workflows).ToNot(BeEmpty(), "Should return at least one workflow")

			// V1.0 SCORING ASSERTION: Confidence = BaseSimilarity (API contract)
			firstWorkflow := response.Workflows[0]
			Expect(firstWorkflow.Confidence).To(Equal(firstWorkflow.BaseSimilarity),
				"DD-WORKFLOW-004 v2.0: Confidence must equal BaseSimilarity in V1.0 (API contract)")
		})
	})

	// ========================================
	// TDD TEST 5: Multiple Workflows Ranking
	// ========================================
	// DD-WORKFLOW-004 v2.0: Ranking is purely by BaseSimilarity
	Context("Multiple workflows ranking (DD-WORKFLOW-004 v2.0)", func() {
		It("should rank workflows purely by BaseSimilarity, not by labels", func() {
			// ARRANGE: Create 3 workflows with DIFFERENT similarity scores but SAME labels
			// Workflow A: Low embedding match (0.3)
			// Workflow B: High embedding match (0.9)
			// Workflow C: Medium embedding match (0.6)
			// Expected order: B, C, A (by similarity, not by creation order or labels)

			labels, _ := json.Marshal(map[string]interface{}{
				"signal_type": "ResourceQuota",
				"severity":    "high",
				"component":   "namespace",
				"environment": "production",
				"priority":    "P1",
			})

			// Workflow A: Low similarity
			embeddingA := pgvector.NewVector(make([]float32, 768))
			for i := range embeddingA.Slice() {
				embeddingA.Slice()[i] = 0.3
			}
			workflowA := &models.RemediationWorkflow{
				WorkflowName:    "v1-ranking-low-" + testID,
				Version:         "v1.0.0",
				Name:            "Resource Quota Recovery A",
				Description:     "Low similarity workflow",
				Content:         "apiVersion: tekton.dev/v1beta1",
				ContentHash:     "hash-rank-a-" + testID,
				Labels:          labels,
				Embedding:       &embeddingA,
				Status:          "active",
				IsLatestVersion: true,
			}

			// Workflow B: High similarity
			embeddingB := pgvector.NewVector(make([]float32, 768))
			for i := range embeddingB.Slice() {
				embeddingB.Slice()[i] = 0.9
			}
			workflowB := &models.RemediationWorkflow{
				WorkflowName:    "v1-ranking-high-" + testID,
				Version:         "v1.0.0",
				Name:            "Resource Quota Recovery B",
				Description:     "High similarity workflow",
				Content:         "apiVersion: tekton.dev/v1beta1",
				ContentHash:     "hash-rank-b-" + testID,
				Labels:          labels,
				Embedding:       &embeddingB,
				Status:          "active",
				IsLatestVersion: true,
			}

			// Workflow C: Medium similarity
			embeddingC := pgvector.NewVector(make([]float32, 768))
			for i := range embeddingC.Slice() {
				embeddingC.Slice()[i] = 0.6
			}
			workflowC := &models.RemediationWorkflow{
				WorkflowName:    "v1-ranking-medium-" + testID,
				Version:         "v1.0.0",
				Name:            "Resource Quota Recovery C",
				Description:     "Medium similarity workflow",
				Content:         "apiVersion: tekton.dev/v1beta1",
				ContentHash:     "hash-rank-c-" + testID,
				Labels:          labels,
				Embedding:       &embeddingC,
				Status:          "active",
				IsLatestVersion: true,
			}

			// Create in non-sorted order (A, B, C)
			Expect(workflowRepo.Create(testCtx, workflowA)).To(Succeed())
			Expect(workflowRepo.Create(testCtx, workflowB)).To(Succeed())
			Expect(workflowRepo.Create(testCtx, workflowC)).To(Succeed())

			// ACT: Search with embedding that matches B best (0.9)
			queryEmbedding := pgvector.NewVector(make([]float32, 768))
			for i := range queryEmbedding.Slice() {
				queryEmbedding.Slice()[i] = 0.9
			}

			request := &models.WorkflowSearchRequest{
				Query:     "Resource quota recovery",
				Embedding: &queryEmbedding,
				TopK:      10,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "ResourceQuota",
					Severity:    "high",
					Component:   "namespace",
					Environment: "production",
					Priority:    "P1",
				},
			}

			response, err := workflowRepo.SearchByEmbedding(testCtx, request)

			// ASSERT: Order is B, C, A (by similarity descending)
			Expect(err).ToNot(HaveOccurred())
			Expect(response.Workflows).To(HaveLen(3), "Should return all 3 workflows")

			// V1.0 SCORING ASSERTION: Ranking by BaseSimilarity only
			Expect(response.Workflows[0].BaseSimilarity).To(BeNumerically(">=", response.Workflows[1].BaseSimilarity),
				"First workflow should have highest BaseSimilarity")
			Expect(response.Workflows[1].BaseSimilarity).To(BeNumerically(">=", response.Workflows[2].BaseSimilarity),
				"Second workflow should have higher BaseSimilarity than third")

			// All have 0.0 boost/penalty
			for _, wf := range response.Workflows {
				Expect(wf.LabelBoost).To(Equal(0.0), "LabelBoost must be 0.0 for all workflows")
				Expect(wf.LabelPenalty).To(Equal(0.0), "LabelPenalty must be 0.0 for all workflows")
			}
		})
	})

	// ========================================
	// TDD TEST 6: All Severity Levels
	// ========================================
	// DD-WORKFLOW-004 v2.0: No implicit boosting for severity
	Context("All severity levels have same scoring behavior (DD-WORKFLOW-004 v2.0)", func() {
		It("should not boost critical severity over low severity workflows", func() {
			// ARRANGE: Create workflows with different severities but SAME embedding
			embedding := pgvector.NewVector(make([]float32, 768))
			for i := range embedding.Slice() {
				embedding.Slice()[i] = 0.75
			}

			severities := []string{"critical", "high", "medium", "low"}
			for _, sev := range severities {
				labels, _ := json.Marshal(map[string]interface{}{
					"signal_type": "GenericError",
					"severity":    sev,
					"component":   "service",
					"environment": "production",
					"priority":    "P2",
				})

				workflow := &models.RemediationWorkflow{
					WorkflowName:    "v1-sev-" + sev + "-" + testID,
					Version:         "v1.0.0",
					Name:            "Recovery for " + sev,
					Description:     "Workflow with " + sev + " severity",
					Content:         "apiVersion: tekton.dev/v1beta1",
					ContentHash:     "hash-sev-" + sev + "-" + testID,
					Labels:          labels,
					Embedding:       &embedding,
					Status:          "active",
					IsLatestVersion: true,
				}

				Expect(workflowRepo.Create(testCtx, workflow)).To(Succeed())
			}

			// ACT: Search for each severity
			for _, sev := range severities {
				queryEmbedding := pgvector.NewVector(make([]float32, 768))
				for i := range queryEmbedding.Slice() {
					queryEmbedding.Slice()[i] = 0.75
				}

				request := &models.WorkflowSearchRequest{
					Query:     "Recovery",
					Embedding: &queryEmbedding,
					TopK:      1,
					Filters: &models.WorkflowSearchFilters{
						SignalType:  "GenericError",
						Severity:    sev,
						Component:   "service",
						Environment: "production",
						Priority:    "P2",
					},
				}

				response, err := workflowRepo.SearchByEmbedding(testCtx, request)

				// ASSERT: All severities have same scoring behavior
				Expect(err).ToNot(HaveOccurred())
				Expect(response.Workflows).ToNot(BeEmpty(), "Should find workflow for severity: "+sev)

				wf := response.Workflows[0]
				Expect(wf.LabelBoost).To(Equal(0.0),
					"DD-WORKFLOW-004 v2.0: LabelBoost must be 0.0 for severity: "+sev)
				Expect(wf.LabelPenalty).To(Equal(0.0),
					"DD-WORKFLOW-004 v2.0: LabelPenalty must be 0.0 for severity: "+sev)
				Expect(wf.FinalScore).To(Equal(wf.BaseSimilarity),
					"DD-WORKFLOW-004 v2.0: FinalScore must equal BaseSimilarity for severity: "+sev)
			}
		})
	})

	// ========================================
	// TDD TEST 7: Perfect Similarity Boundary
	// ========================================
	// DD-WORKFLOW-004 v2.0: Edge case for max similarity (1.0)
	Context("Perfect similarity boundary case (DD-WORKFLOW-004 v2.0)", func() {
		It("should handle perfect similarity (1.0) correctly with no boost", func() {
			// ARRANGE: Create workflow with specific embedding
			embedding := pgvector.NewVector(make([]float32, 768))
			for i := range embedding.Slice() {
				embedding.Slice()[i] = 1.0 / float32(768) // Normalized vector
			}

			labels, _ := json.Marshal(map[string]interface{}{
				"signal_type": "PerfectMatch",
				"severity":    "critical",
				"component":   "test",
				"environment": "production",
				"priority":    "P0",
			})

			workflow := &models.RemediationWorkflow{
				WorkflowName:    "v1-perfect-sim-" + testID,
				Version:         "v1.0.0",
				Name:            "Perfect Similarity Test",
				Description:     "Workflow for perfect similarity edge case",
				Content:         "apiVersion: tekton.dev/v1beta1",
				ContentHash:     "hash-perfect-" + testID,
				Labels:          labels,
				Embedding:       &embedding,
				Status:          "active",
				IsLatestVersion: true,
			}

			Expect(workflowRepo.Create(testCtx, workflow)).To(Succeed())

			// ACT: Search with IDENTICAL embedding (should give ~1.0 similarity)
			queryEmbedding := pgvector.NewVector(make([]float32, 768))
			for i := range queryEmbedding.Slice() {
				queryEmbedding.Slice()[i] = 1.0 / float32(768)
			}

			request := &models.WorkflowSearchRequest{
				Query:     "Perfect match",
				Embedding: &queryEmbedding,
				TopK:      1,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "PerfectMatch",
					Severity:    "critical",
					Component:   "test",
					Environment: "production",
					Priority:    "P0",
				},
			}

			response, err := workflowRepo.SearchByEmbedding(testCtx, request)

			// ASSERT: Perfect similarity should have 0.0 boost/penalty
			Expect(err).ToNot(HaveOccurred())
			Expect(response.Workflows).ToNot(BeEmpty())

			wf := response.Workflows[0]
			Expect(wf.BaseSimilarity).To(BeNumerically(">=", 0.99),
				"Identical embeddings should have near-perfect similarity")
			Expect(wf.LabelBoost).To(Equal(0.0),
				"DD-WORKFLOW-004 v2.0: LabelBoost must be 0.0 even for perfect similarity")
			Expect(wf.LabelPenalty).To(Equal(0.0),
				"DD-WORKFLOW-004 v2.0: LabelPenalty must be 0.0 even for perfect similarity")
			Expect(wf.FinalScore).To(Equal(wf.BaseSimilarity),
				"DD-WORKFLOW-004 v2.0: FinalScore must equal BaseSimilarity even for perfect similarity")
		})
	})

	// ========================================
	// TDD TEST 8: Near-Zero Similarity Boundary
	// ========================================
	// DD-WORKFLOW-004 v2.0: Edge case for min similarity
	Context("Near-zero similarity boundary case (DD-WORKFLOW-004 v2.0)", func() {
		It("should handle near-zero similarity correctly with no penalty", func() {
			// ARRANGE: Create workflow with specific embedding
			embedding := pgvector.NewVector(make([]float32, 768))
			for i := range embedding.Slice() {
				embedding.Slice()[i] = 0.99 // High values
			}

			labels, _ := json.Marshal(map[string]interface{}{
				"signal_type": "LowMatch",
				"severity":    "low",
				"component":   "test",
				"environment": "development",
				"priority":    "P3",
			})

			workflow := &models.RemediationWorkflow{
				WorkflowName:    "v1-low-sim-" + testID,
				Version:         "v1.0.0",
				Name:            "Low Similarity Test",
				Description:     "Workflow for low similarity edge case",
				Content:         "apiVersion: tekton.dev/v1beta1",
				ContentHash:     "hash-low-" + testID,
				Labels:          labels,
				Embedding:       &embedding,
				Status:          "active",
				IsLatestVersion: true,
			}

			Expect(workflowRepo.Create(testCtx, workflow)).To(Succeed())

			// ACT: Search with OPPOSITE embedding (should give low similarity)
			queryEmbedding := pgvector.NewVector(make([]float32, 768))
			for i := range queryEmbedding.Slice() {
				queryEmbedding.Slice()[i] = 0.01 // Low values
			}

			request := &models.WorkflowSearchRequest{
				Query:     "Low match",
				Embedding: &queryEmbedding,
				TopK:      1,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "LowMatch",
					Severity:    "low",
					Component:   "test",
					Environment: "development",
					Priority:    "P3",
				},
			}

			response, err := workflowRepo.SearchByEmbedding(testCtx, request)

			// ASSERT: Low similarity should still have 0.0 boost/penalty
			Expect(err).ToNot(HaveOccurred())
			Expect(response.Workflows).ToNot(BeEmpty())

			wf := response.Workflows[0]
			Expect(wf.BaseSimilarity).To(BeNumerically("<", 0.5),
				"Opposite embeddings should have low similarity")
			Expect(wf.LabelBoost).To(Equal(0.0),
				"DD-WORKFLOW-004 v2.0: LabelBoost must be 0.0 even for low similarity")
			Expect(wf.LabelPenalty).To(Equal(0.0),
				"DD-WORKFLOW-004 v2.0: LabelPenalty must be 0.0 even for low similarity")
			Expect(wf.FinalScore).To(Equal(wf.BaseSimilarity),
				"DD-WORKFLOW-004 v2.0: FinalScore must equal BaseSimilarity even for low similarity")
		})
	})

	// ========================================
	// TDD TEST 9: Multiple DetectedLabels
	// ========================================
	// DD-WORKFLOW-004 v2.0: Even with many potential boost sources, all = 0.0
	Context("Multiple DetectedLabels have no effect (DD-WORKFLOW-004 v2.0)", func() {
		It("should have LabelBoost = 0.0 even with many matching DetectedLabels", func() {
			// ARRANGE: Create workflow with MANY DetectedLabels
			labels, _ := json.Marshal(map[string]interface{}{
				"signal_type": "ComplexSignal",
				"severity":    "critical",
				"component":   "statefulset",
				"environment": "production",
				"priority":    "P0",
			})

			// Many DetectedLabels that would have boosted in v1.1
			detectedLabels, _ := json.Marshal(map[string]interface{}{
				"git_ops_managed":    true,
				"git_ops_tool":       "argocd",
				"resource_type":      "kubernetes",
				"cloud_provider":     "aws",
				"monitoring_enabled": true,
				"auto_scaling":       true,
				"ha_enabled":         true,
			})

			embedding := pgvector.NewVector(make([]float32, 768))
			for i := range embedding.Slice() {
				embedding.Slice()[i] = 0.85
			}

			workflow := &models.RemediationWorkflow{
				WorkflowName:    "v1-many-labels-" + testID,
				Version:         "v1.0.0",
				Name:            "Multi-Label Workflow",
				Description:     "Workflow with many DetectedLabels",
				Content:         "apiVersion: tekton.dev/v1beta1",
				ContentHash:     "hash-many-" + testID,
				Labels:          labels,
				DetectedLabels:  detectedLabels,
				Embedding:       &embedding,
				Status:          "active",
				IsLatestVersion: true,
			}

			Expect(workflowRepo.Create(testCtx, workflow)).To(Succeed())

			// ACT: Search with matching DetectedLabels filter
			queryEmbedding := pgvector.NewVector(make([]float32, 768))
			for i := range queryEmbedding.Slice() {
				queryEmbedding.Slice()[i] = 0.85
			}

			gitOpsManaged := true
			gitOpsTool := "argocd"
			request := &models.WorkflowSearchRequest{
				Query:     "Complex signal recovery",
				Embedding: &queryEmbedding,
				TopK:      1,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "ComplexSignal",
					Severity:    "critical",
					Component:   "statefulset",
					Environment: "production",
					Priority:    "P0",
					DetectedLabels: &models.DetectedLabels{
						GitOpsManaged: &gitOpsManaged,
						GitOpsTool:    &gitOpsTool,
					},
				},
			}

			response, err := workflowRepo.SearchByEmbedding(testCtx, request)

			// ASSERT: Still 0.0 boost/penalty despite many matching labels
			Expect(err).ToNot(HaveOccurred())
			Expect(response.Workflows).ToNot(BeEmpty())

			wf := response.Workflows[0]
			Expect(wf.LabelBoost).To(Equal(0.0),
				"DD-WORKFLOW-004 v2.0: LabelBoost must be 0.0 even with many matching DetectedLabels")
			Expect(wf.LabelPenalty).To(Equal(0.0),
				"DD-WORKFLOW-004 v2.0: LabelPenalty must be 0.0 even with many matching DetectedLabels")
		})
	})

	// ========================================
	// TDD TEST 10: CustomLabels are Ignored
	// ========================================
	// DD-WORKFLOW-004 v2.0: CustomLabels have no effect in V1.0
	Context("CustomLabels have no effect on scoring (DD-WORKFLOW-004 v2.0)", func() {
		It("should have LabelBoost = 0.0 regardless of CustomLabels", func() {
			// ARRANGE: Create workflow with CustomLabels
			labels, _ := json.Marshal(map[string]interface{}{
				"signal_type": "CustomSignal",
				"severity":    "high",
				"component":   "deployment",
				"environment": "staging",
				"priority":    "P1",
			})

			// CustomLabels (customer-defined via Rego policies)
			customLabels, _ := json.Marshal(map[string]interface{}{
				"team":           "platform",
				"cost_center":    "engineering",
				"compliance":     "pci-dss",
				"data_class":     "confidential",
				"sla_tier":       "platinum",
				"region":         "us-east-1",
				"business_unit":  "core-services",
			})

			embedding := pgvector.NewVector(make([]float32, 768))
			for i := range embedding.Slice() {
				embedding.Slice()[i] = 0.78
			}

			workflow := &models.RemediationWorkflow{
				WorkflowName:    "v1-custom-labels-" + testID,
				Version:         "v1.0.0",
				Name:            "Custom Labels Workflow",
				Description:     "Workflow with CustomLabels",
				Content:         "apiVersion: tekton.dev/v1beta1",
				ContentHash:     "hash-custom-" + testID,
				Labels:          labels,
				CustomLabels:    customLabels,
				Embedding:       &embedding,
				Status:          "active",
				IsLatestVersion: true,
			}

			Expect(workflowRepo.Create(testCtx, workflow)).To(Succeed())

			// ACT: Search
			queryEmbedding := pgvector.NewVector(make([]float32, 768))
			for i := range queryEmbedding.Slice() {
				queryEmbedding.Slice()[i] = 0.78
			}

			request := &models.WorkflowSearchRequest{
				Query:     "Custom signal recovery",
				Embedding: &queryEmbedding,
				TopK:      1,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "CustomSignal",
					Severity:    "high",
					Component:   "deployment",
					Environment: "staging",
					Priority:    "P1",
				},
			}

			response, err := workflowRepo.SearchByEmbedding(testCtx, request)

			// ASSERT: CustomLabels have no effect on scoring
			Expect(err).ToNot(HaveOccurred())
			Expect(response.Workflows).ToNot(BeEmpty())

			wf := response.Workflows[0]
			Expect(wf.LabelBoost).To(Equal(0.0),
				"DD-WORKFLOW-004 v2.0: LabelBoost must be 0.0 regardless of CustomLabels")
			Expect(wf.LabelPenalty).To(Equal(0.0),
				"DD-WORKFLOW-004 v2.0: LabelPenalty must be 0.0 regardless of CustomLabels")
			Expect(wf.FinalScore).To(Equal(wf.BaseSimilarity),
				"DD-WORKFLOW-004 v2.0: FinalScore must equal BaseSimilarity regardless of CustomLabels")
		})
	})

	// ========================================
	// TDD TEST 11: Empty Results
	// ========================================
	// DD-WORKFLOW-004 v2.0: No error on no matches
	Context("Empty results handling (DD-WORKFLOW-004 v2.0)", func() {
		It("should return empty results without error when no workflows match", func() {
			// ACT: Search for non-existent signal type
			queryEmbedding := pgvector.NewVector(make([]float32, 768))
			for i := range queryEmbedding.Slice() {
				queryEmbedding.Slice()[i] = 0.5
			}

			request := &models.WorkflowSearchRequest{
				Query:     "NonExistent recovery",
				Embedding: &queryEmbedding,
				TopK:      10,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "NonExistentSignalType_" + testID, // Unique non-existent signal
					Severity:    "critical",
					Component:   "pod",
					Environment: "production",
					Priority:    "P0",
				},
			}

			response, err := workflowRepo.SearchByEmbedding(testCtx, request)

			// ASSERT: Empty results, no error
			Expect(err).ToNot(HaveOccurred(), "Search should not error on no matches")
			Expect(response).ToNot(BeNil(), "Response should not be nil")
			Expect(response.Workflows).To(BeEmpty(), "Should return empty slice, not nil")
		})
	})

	// ========================================
	// TDD TEST 12: All Environments Equal
	// ========================================
	// DD-WORKFLOW-004 v2.0: No environment-based boosting
	Context("All environments are treated equally (DD-WORKFLOW-004 v2.0)", func() {
		It("should not boost production over development environments", func() {
			// ARRANGE: Create workflows with different environments but SAME embedding
			embedding := pgvector.NewVector(make([]float32, 768))
			for i := range embedding.Slice() {
				embedding.Slice()[i] = 0.82
			}

			environments := []string{"production", "staging", "development", "qa", "sandbox"}
			for _, env := range environments {
				labels, _ := json.Marshal(map[string]interface{}{
					"signal_type": "EnvTest",
					"severity":    "medium",
					"component":   "service",
					"environment": env,
					"priority":    "P2",
				})

				workflow := &models.RemediationWorkflow{
					WorkflowName:    "v1-env-" + env + "-" + testID,
					Version:         "v1.0.0",
					Name:            "Recovery for " + env,
					Description:     "Workflow for " + env + " environment",
					Content:         "apiVersion: tekton.dev/v1beta1",
					ContentHash:     "hash-env-" + env + "-" + testID,
					Labels:          labels,
					Embedding:       &embedding,
					Status:          "active",
					IsLatestVersion: true,
				}

				Expect(workflowRepo.Create(testCtx, workflow)).To(Succeed())
			}

			// ACT: Search for each environment
			var scores []float64
			for _, env := range environments {
				queryEmbedding := pgvector.NewVector(make([]float32, 768))
				for i := range queryEmbedding.Slice() {
					queryEmbedding.Slice()[i] = 0.82
				}

				request := &models.WorkflowSearchRequest{
					Query:     "Env recovery",
					Embedding: &queryEmbedding,
					TopK:      1,
					Filters: &models.WorkflowSearchFilters{
						SignalType:  "EnvTest",
						Severity:    "medium",
						Component:   "service",
						Environment: env,
						Priority:    "P2",
					},
				}

				response, err := workflowRepo.SearchByEmbedding(testCtx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.Workflows).ToNot(BeEmpty())

				wf := response.Workflows[0]
				scores = append(scores, wf.FinalScore)

				// V1.0 SCORING ASSERTION: All environments have same behavior
				Expect(wf.LabelBoost).To(Equal(0.0),
					"DD-WORKFLOW-004 v2.0: LabelBoost must be 0.0 for environment: "+env)
				Expect(wf.LabelPenalty).To(Equal(0.0),
					"DD-WORKFLOW-004 v2.0: LabelPenalty must be 0.0 for environment: "+env)
			}

			// ASSERT: All environments have same FinalScore (since same embedding)
			for i := 1; i < len(scores); i++ {
				Expect(scores[i]).To(BeNumerically("~", scores[0], 0.001),
					"All environments should have same FinalScore with same embedding")
			}
		})
	})
})


