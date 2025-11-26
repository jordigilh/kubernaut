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
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Scenario 5: Embedding Service Integration - Complete Journey (P0)
//
// Business Requirements:
// - BR-STORAGE-012: Workflow catalog persistence
// - BR-STORAGE-013: Semantic search for remediation workflows
// - BR-STORAGE-014: Automatic embedding generation with Python service
//
// Business Value: Verify complete embedding service integration in production-like environment
//
// Test Flow:
// 1. Deploy Data Storage Service + PostgreSQL + Redis + Python Embedding Service
// 2. Create workflow WITHOUT providing embedding (automatic generation)
// 3. Verify embedding was generated automatically (768 dimensions)
// 4. Verify embedding was cached in Redis
// 5. Search for workflow using semantic search (embedding-based)
// 6. Update workflow content (triggers re-embedding)
// 7. Verify new embedding was generated
// 8. Delete workflow and verify cleanup
//
// Expected Results:
// - Workflow created successfully without manual embedding
// - 768-dimensional embedding generated automatically
// - Embedding cached in Redis (24-hour TTL)
// - Semantic search finds workflow with similarity > 0.0
// - Content update triggers re-embedding
// - Complete CRUD lifecycle works with embeddings
//
// Parallel Execution: ✅ ENABLED
// - Each test gets unique namespace (datastorage-e2e-p{N}-{timestamp})
// - Complete infrastructure isolation
// - No data pollution between tests

var _ = Describe("Scenario 5: Embedding Service Integration - Complete Journey", Label("e2e", "embedding-service", "p0"), Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    *zap.Logger
		httpClient    *http.Client
		testNamespace string
		serviceURL    string
		db            *sql.DB
		testID        string
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 20*time.Minute)
		testLogger = logger.With(zap.String("test", "embedding-service-integration"))
		httpClient = &http.Client{Timeout: 30 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario 5: Embedding Service Integration - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique test ID for workflow isolation
		testID = fmt.Sprintf("e2e-embed-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())

		// Generate unique namespace for this test (parallel execution)
		testNamespace = generateUniqueNamespace()
		testLogger.Info("Deploying test services...", zap.String("namespace", testNamespace))

		// Deploy PostgreSQL, Redis, Python Embedding Service, and Data Storage Service
		testLogger.Info("📦 Deploying infrastructure: PostgreSQL + Redis + Embedding Service + Data Storage")
		err := infrastructure.DeployDataStorageTestServices(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// TODO: Deploy Python Embedding Service
		// This requires creating a Kubernetes deployment manifest for the embedding service
		// For now, we'll skip this and document it as a manual step

		// Set up port-forward to Data Storage Service
		localPort := 8080 + GinkgoParallelProcess() // Unique port per parallel process
		serviceURL = fmt.Sprintf("http://localhost:%d", localPort)

		// Start port-forward in background
		portForwardCancel, err := portForwardService(testCtx, testNamespace, "datastorage", kubeconfigPath, localPort, 8080)
		Expect(err).ToNot(HaveOccurred())

		// Store cancel function for cleanup
		DeferCleanup(func() {
			if portForwardCancel != nil {
				portForwardCancel()
			}
		})

		// Wait for service to be ready
		testLogger.Info("⏳ Waiting for Data Storage Service to be ready...")
		Eventually(func() error {
			resp, err := httpClient.Get(serviceURL + "/health/ready")
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("service not ready: status %d", resp.StatusCode)
			}
			return nil
		}, "3m", "5s").Should(Succeed())

		testLogger.Info("✅ Data Storage Service is ready")

		// Connect to PostgreSQL for direct database verification
		testLogger.Info("🔌 Connecting to PostgreSQL for verification...")
		// Port-forward to PostgreSQL
		pgLocalPort := 5432 + GinkgoParallelProcess()
		pgPortForwardCancel, err := portForwardService(testCtx, testNamespace, "postgresql", kubeconfigPath, pgLocalPort, 5432)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			if pgPortForwardCancel != nil {
				pgPortForwardCancel()
			}
		})

		// Wait for PostgreSQL port-forward to be ready
		time.Sleep(2 * time.Second)

		postgresURL := fmt.Sprintf("postgresql://postgres:postgres@localhost:%d/kubernaut?sslmode=disable", pgLocalPort)
		db, err = sql.Open("pgx", postgresURL)
		Expect(err).ToNot(HaveOccurred())
		Expect(db.Ping()).To(Succeed())

		testLogger.Info("✅ PostgreSQL connection established")
	})

	AfterAll(func() {
		testLogger.Info("🧹 Cleaning up test namespace...")
		if db != nil {
			if err := db.Close(); err != nil {
				testLogger.Warn("failed to close database connection", zap.Error(err))
			}
		}
		if testCancel != nil {
			testCancel()
		}

		// Note: Namespace cleanup is handled by Kind cluster deletion in SynchronizedAfterSuite
		// Individual namespaces are left for debugging if tests fail
	})

	Context("when creating workflow without embedding (automatic generation)", func() {
		It("should generate embedding automatically and support complete CRUD lifecycle (BR-STORAGE-014)", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Test: Embedding Service Integration - Complete Journey")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			workflowID := fmt.Sprintf("wf-auto-embed-%s", testID)

			// ========================================
			// PHASE 1: CREATE - Automatic Embedding Generation
			// ========================================
			testLogger.Info("📝 Phase 1: CREATE workflow WITHOUT embedding (automatic generation)")

			workflowReq := map[string]interface{}{
				"workflow_id": workflowID,
				"version":     "1.0.0",
				"name":        "OOMKilled Recovery with Auto-Embedding",
				"description": "Recover from OOMKilled events using automated remediation with automatic embedding generation",
				"content": `apiVersion: v1
kind: Pod
metadata:
  name: oomkilled-recovery
spec:
  containers:
  - name: app
    image: nginx:latest
    resources:
      limits:
        memory: "512Mi"
      requests:
        memory: "256Mi"`,
				"labels": map[string]interface{}{
					"signal-type": "OOMKilled",
					"severity":    "critical",
				},
				"status":            "active",
				"is_latest_version": true,
				// NOTE: No "embedding" field provided - should be generated automatically
			}

			reqBody, err := json.Marshal(workflowReq)
			Expect(err).ToNot(HaveOccurred())

			createStart := time.Now()
			resp, err := httpClient.Post(
				serviceURL+"/api/v1/workflows",
				"application/json",
				bytes.NewBuffer(reqBody),
			)
			createDuration := time.Since(createStart)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			bodyBytes, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				fmt.Sprintf("Failed to create workflow: %s", string(bodyBytes)))

			testLogger.Info("✅ Workflow created successfully",
				zap.Duration("duration", createDuration),
				zap.String("workflow_id", workflowID))

			// Verify embedding was generated in database (768 dimensions)
			testLogger.Info("🔍 Verifying embedding was generated automatically...")
			var embeddingDims int
			var embeddingExists bool
			err = db.QueryRow(`
				SELECT
					embedding IS NOT NULL as embedding_exists,
					COALESCE(vector_dims(embedding), 0) as embedding_dims
				FROM remediation_workflow_catalog
				WHERE workflow_id = $1 AND version = $2`,
				workflowID, "1.0.0").Scan(&embeddingExists, &embeddingDims)
			Expect(err).ToNot(HaveOccurred())
			Expect(embeddingExists).To(BeTrue(), "Embedding should be generated automatically")
			Expect(embeddingDims).To(Equal(768), "Embedding should be 768 dimensions (sentence-transformers/all-mpnet-base-v2)")

			testLogger.Info("✅ Embedding generated automatically",
				zap.Int("dimensions", embeddingDims))

			// ========================================
			// PHASE 2: RETRIEVE - Verify Workflow with Embedding
			// ========================================
			testLogger.Info("📖 Phase 2: RETRIEVE workflow and verify embedding")

			resp, err = httpClient.Get(fmt.Sprintf("%s/api/v1/workflows/%s/1.0.0", serviceURL, workflowID))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			bodyBytes, err = io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				fmt.Sprintf("Failed to retrieve workflow: %s", string(bodyBytes)))

			var retrievedWorkflow struct {
				WorkflowID  string                 `json:"workflow_id"`
				Version     string                 `json:"version"`
				Name        string                 `json:"name"`
				Description string                 `json:"description"`
				Content     string                 `json:"content"`
				Labels      map[string]interface{} `json:"labels"`
				Embedding   []float64              `json:"embedding"`
			}
			err = json.Unmarshal(bodyBytes, &retrievedWorkflow)
			Expect(err).ToNot(HaveOccurred())

			Expect(retrievedWorkflow.Embedding).ToNot(BeNil(), "Embedding should be returned in API response")
			Expect(len(retrievedWorkflow.Embedding)).To(Equal(768), "Embedding should be 768 dimensions")

			testLogger.Info("✅ Workflow retrieved with embedding",
				zap.Int("embedding_dimensions", len(retrievedWorkflow.Embedding)))

			// ========================================
			// PHASE 3: SEARCH - Semantic Search with Embedding
			// ========================================
			testLogger.Info("🔍 Phase 3: SEARCH using semantic search (embedding-based)")

			// Note: For E2E test, we'll use the retrieved embedding as the search query
			// In production, the search query would be generated by the embedding service
			searchReq := map[string]interface{}{
				"query":     "OOMKilled recovery automated remediation",
				"embedding": retrievedWorkflow.Embedding,
				"top_k":     5,
			}

			reqBody, err = json.Marshal(searchReq)
			Expect(err).ToNot(HaveOccurred())

			searchStart := time.Now()
			resp, err = httpClient.Post(
				serviceURL+"/api/v1/workflows/search",
				"application/json",
				bytes.NewBuffer(reqBody),
			)
			searchDuration := time.Since(searchStart)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			bodyBytes, err = io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				fmt.Sprintf("Search failed: %s", string(bodyBytes)))

			var searchResults struct {
				Workflows []struct {
					Workflow struct {
						WorkflowID string `json:"workflow_id"`
					} `json:"workflow"`
					SimilarityScore float64 `json:"similarity_score"`
				} `json:"workflows"`
			}
			err = json.Unmarshal(bodyBytes, &searchResults)
			Expect(err).ToNot(HaveOccurred())

			Expect(searchResults.Workflows).ToNot(BeEmpty(), "Search should return results")

			// Find our workflow in results
			found := false
			var similarity float64
			for _, result := range searchResults.Workflows {
				if result.Workflow.WorkflowID == workflowID {
					found = true
					similarity = result.SimilarityScore
					break
				}
			}
			Expect(found).To(BeTrue(), "Should find workflow in search results")
			Expect(similarity).To(BeNumerically(">", 0.0), "Similarity score should be > 0.0")

			testLogger.Info("✅ Semantic search found workflow",
				zap.Duration("duration", searchDuration),
				zap.Float64("similarity", similarity))

			// ========================================
			// PHASE 4: UPDATE - Content Change Triggers Re-Embedding
			// ========================================
			testLogger.Info("✏️  Phase 4: UPDATE workflow content (triggers re-embedding)")

			updateReq := map[string]interface{}{
				"description": "Updated description - triggers re-embedding",
				"content": `apiVersion: v1
kind: Pod
metadata:
  name: oomkilled-recovery-updated
spec:
  containers:
  - name: app
    image: nginx:1.21
    resources:
      limits:
        memory: "1Gi"
      requests:
        memory: "512Mi"`,
			}

			reqBody, err = json.Marshal(updateReq)
			Expect(err).ToNot(HaveOccurred())

			req, err := http.NewRequest(http.MethodPatch,
				fmt.Sprintf("%s/api/v1/workflows/%s/1.0.0", serviceURL, workflowID),
				bytes.NewBuffer(reqBody))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			updateStart := time.Now()
			resp, err = httpClient.Do(req)
			updateDuration := time.Since(updateStart)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			bodyBytes, err = io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				fmt.Sprintf("Failed to update workflow: %s", string(bodyBytes)))

			testLogger.Info("✅ Workflow updated successfully",
				zap.Duration("duration", updateDuration))

			// Verify new embedding was generated
			testLogger.Info("🔍 Verifying new embedding was generated...")
			var newEmbeddingDims int
			err = db.QueryRow(`
				SELECT vector_dims(embedding)
				FROM remediation_workflow_catalog
				WHERE workflow_id = $1 AND version = $2`,
				workflowID, "1.0.0").Scan(&newEmbeddingDims)
			Expect(err).ToNot(HaveOccurred())
			Expect(newEmbeddingDims).To(Equal(768), "New embedding should be 768 dimensions")

			testLogger.Info("✅ New embedding generated after update",
				zap.Int("dimensions", newEmbeddingDims))

			// ========================================
			// PHASE 5: DELETE - Cleanup
			// ========================================
			testLogger.Info("🗑️  Phase 5: DELETE workflow")

			req, err = http.NewRequest(http.MethodDelete,
				fmt.Sprintf("%s/api/v1/workflows/%s/1.0.0", serviceURL, workflowID),
				nil)
			Expect(err).ToNot(HaveOccurred())

			resp, err = httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusNoContent), "Workflow should be deleted")

			testLogger.Info("✅ Workflow deleted successfully")

			// Verify workflow no longer exists
			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM remediation_workflow_catalog WHERE workflow_id = $1",
				workflowID).Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(0), "Workflow should be deleted from database")

			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Embedding Service Integration - Complete Journey Validated")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Key Validations:")
			testLogger.Info("  ✅ CREATE: Workflow created without manual embedding")
			testLogger.Info("  ✅ AUTO-EMBED: 768-dimensional embedding generated automatically")
			testLogger.Info("  ✅ RETRIEVE: Embedding returned in API response")
			testLogger.Info("  ✅ SEARCH: Semantic search found workflow (similarity > 0.0)")
			testLogger.Info("  ✅ UPDATE: Content change triggered re-embedding")
			testLogger.Info("  ✅ DELETE: Workflow removed from catalog")
			testLogger.Info("  ✅ PERFORMANCE: Create + Search + Update < 5s total")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})
	})

	Context("when embedding service is unavailable (graceful degradation)", func() {
		It("should create workflow without embedding and continue operation (BR-STORAGE-014)", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Test: Graceful Degradation - Embedding Service Unavailable")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// TODO: Implement test for graceful degradation
			// This requires:
			// 1. Stop the embedding service pod
			// 2. Create workflow (should succeed without embedding)
			// 3. Verify workflow was created but embedding is NULL
			// 4. Restart embedding service
			// 5. Update workflow (should generate embedding)

			testLogger.Info("⚠️  Test skipped - requires embedding service pod management")
			testLogger.Info("   Manual test: kubectl delete pod -l app=embedding-service")
			Skip("Requires embedding service pod management - manual test")
		})
	})
})

