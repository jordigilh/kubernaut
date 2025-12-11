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
})

