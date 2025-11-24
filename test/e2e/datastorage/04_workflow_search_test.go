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

// Scenario 4: Workflow Search with Hybrid Weighted Scoring (P0)
//
// Business Requirements:
// - BR-STORAGE-012: Workflow catalog persistence
// - BR-STORAGE-013: Semantic search with hybrid weighted scoring
//
// Business Value: Verify workflow search selects correct workflow using hybrid scoring
//
// Test Flow:
// 1. Deploy Data Storage Service in isolated namespace
// 2. Seed workflow catalog with 5 test workflows (various labels)
// 3. Simulate HolmesGPT API calling workflow search endpoint
// 4. Verify hybrid weighted scoring selects correct workflow
// 5. Validate boost/penalty calculations in results
//
// Expected Results:
// - Workflow search returns results ranked by hybrid score
// - GitOps workflow ranked higher than manual workflow (boost applied)
// - Mandatory labels (signal_type, severity) strictly enforced
// - Search latency <200ms (p95, local testing)
//
// Parallel Execution: ✅ ENABLED
// - Each test gets unique namespace (datastorage-e2e-p{N}-{timestamp})
// - Complete infrastructure isolation
// - No data pollution between tests

var _ = Describe("Scenario 4: Workflow Search with Hybrid Weighted Scoring", Label("e2e", "workflow-search", "p0"), Ordered, func() {
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
		testCtx, testCancel = context.WithTimeout(ctx, 15*time.Minute)
		testLogger = logger.With(zap.String("test", "workflow-search"))
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario 4: Workflow Search - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique test ID for workflow isolation
		testID = fmt.Sprintf("e2e-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())

		// Generate unique namespace for this test (parallel execution)
		testNamespace = generateUniqueNamespace()
		testLogger.Info("Deploying test services...", zap.String("namespace", testNamespace))

		// Deploy PostgreSQL, Redis, and Data Storage Service
		err := infrastructure.DeployDataStorageTestServices(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

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
		}, "2m", "5s").Should(Succeed())

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

	Context("when searching for workflows with hybrid weighted scoring", func() {
		It("should select correct workflow using hybrid scoring (BR-STORAGE-013)", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Test: Workflow Search with Hybrid Weighted Scoring")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// ARRANGE: Seed workflow catalog with 5 test workflows
			testLogger.Info("📦 Seeding workflow catalog with test workflows...")

			workflows := []struct {
				workflowID  string
				name        string
				description string
				labels      map[string]interface{}
				embedding   []float64
			}{
				{
					workflowID:  fmt.Sprintf("wf-gitops-argocd-%s", testID),
					name:        "OOM Recovery with GitOps (ArgoCD)",
					description: "Recover from OOMKilled using GitOps with ArgoCD",
					labels: map[string]interface{}{
						"signal-type":         "OOMKilled",
						"severity":            "critical",
						"resource-management": "gitops",
						"gitops-tool":         "argocd",
						"environment":         "production",
						"business-category":   "revenue-critical",
						"priority":            "P0",
						"risk-tolerance":      "low",
					},
					embedding: generateTestEmbedding("OOMKilled critical gitops argocd production"),
				},
				{
					workflowID:  fmt.Sprintf("wf-gitops-flux-%s", testID),
					name:        "OOM Recovery with GitOps (Flux)",
					description: "Recover from OOMKilled using GitOps with Flux",
					labels: map[string]interface{}{
						"signal-type":         "OOMKilled",
						"severity":            "critical",
						"resource-management": "gitops",
						"gitops-tool":         "flux",
						"environment":         "production",
						"business-category":   "revenue-critical",
						"priority":            "P0",
						"risk-tolerance":      "low",
					},
					embedding: generateTestEmbedding("OOMKilled critical gitops flux production"),
				},
				{
					workflowID:  fmt.Sprintf("wf-manual-%s", testID),
					name:        "OOM Recovery with Manual Intervention",
					description: "Recover from OOMKilled using manual kubectl commands",
					labels: map[string]interface{}{
						"signal-type":         "OOMKilled",
						"severity":            "critical",
						"resource-management": "manual",
						"environment":         "production",
						"business-category":   "revenue-critical",
						"priority":            "P0",
						"risk-tolerance":      "low",
					},
					embedding: generateTestEmbedding("OOMKilled critical manual kubectl production"),
				},
				{
					workflowID:  fmt.Sprintf("wf-generic-%s", testID),
					name:        "OOM Recovery (Generic)",
					description: "Generic OOM recovery workflow",
					labels: map[string]interface{}{
						"signal-type": "OOMKilled",
						"severity":    "critical",
					},
					embedding: generateTestEmbedding("OOMKilled critical generic recovery"),
				},
				{
					workflowID:  fmt.Sprintf("wf-different-signal-%s", testID),
					name:        "CrashLoopBackOff Recovery",
					description: "Recover from CrashLoopBackOff",
					labels: map[string]interface{}{
						"signal-type": "CrashLoopBackOff",
						"severity":    "high",
					},
					embedding: generateTestEmbedding("CrashLoopBackOff high recovery"),
				},
			}

			// Create workflows via API
			for i, wf := range workflows {
				workflowReq := map[string]interface{}{
					"workflow_id":       wf.workflowID,
					"version":           "1.0.0",
					"name":              wf.name,
					"description":       wf.description,
					"content":           fmt.Sprintf("# Workflow content for %s", wf.name),
					"labels":            wf.labels,
					"embedding":         wf.embedding,
					"status":            "active",
					"is_latest_version": true,
				}

				reqBody, err := json.Marshal(workflowReq)
				Expect(err).ToNot(HaveOccurred())

				resp, err := httpClient.Post(
					serviceURL+"/api/v1/workflows",
					"application/json",
					bytes.NewBuffer(reqBody),
				)
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				bodyBytes, _ := io.ReadAll(resp.Body)
				Expect(resp.StatusCode).To(Equal(http.StatusCreated),
					fmt.Sprintf("Failed to create workflow %d: %s", i+1, string(bodyBytes)))

				testLogger.Info(fmt.Sprintf("✅ Created workflow %d/%d", i+1, len(workflows)),
					zap.String("workflow_id", wf.workflowID))
			}

			// Verify workflows were created in database
			var count int
			err := db.QueryRow("SELECT COUNT(*) FROM remediation_workflow_catalog WHERE workflow_id LIKE $1",
				fmt.Sprintf("%%-%s", testID)).Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(5), "All 5 workflows should be created")

			testLogger.Info("✅ All workflows created successfully")

			// ACT: Search for OOMKilled workflows with GitOps + ArgoCD preference
			testLogger.Info("🔍 Searching for workflows with hybrid weighted scoring...")
			testLogger.Info("   Query: 'OOMKilled critical with GitOps ArgoCD'")
			testLogger.Info("   Filters: signal_type=OOMKilled, severity=critical, resource_management=gitops, gitops_tool=argocd")

			searchReq := map[string]interface{}{
				"query":     "OOMKilled critical with GitOps ArgoCD",
				"embedding": generateTestEmbedding("OOMKilled critical gitops argocd production"),
				"filters": map[string]interface{}{
					"signal_type":         "OOMKilled",
					"severity":            "critical",
					"resource_management": "gitops",
					"gitops_tool":         "argocd",
				},
				"top_k": 5,
			}

			reqBody, err := json.Marshal(searchReq)
			Expect(err).ToNot(HaveOccurred())

			start := time.Now()
			resp, err := httpClient.Post(
				serviceURL+"/api/v1/workflows/search",
				"application/json",
				bytes.NewBuffer(reqBody),
			)
			searchDuration := time.Since(start)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			bodyBytes, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK), fmt.Sprintf("Search failed: %s", string(bodyBytes)))

			testLogger.Info("✅ Search completed", zap.Duration("duration", searchDuration))

			// ASSERT: Verify hybrid weighted scoring results
			var searchResults struct {
				Workflows []struct {
					Workflow struct {
						WorkflowID  string                 `json:"workflow_id"`
						Name        string                 `json:"name"`
						Description string                 `json:"description"`
						Labels      map[string]interface{} `json:"labels"`
					} `json:"workflow"`
					SimilarityScore float64 `json:"similarity_score"`
					BoostScore      float64 `json:"boost_score"`
					PenaltyScore    float64 `json:"penalty_score"`
					FinalScore      float64 `json:"final_score"`
					Rank            int     `json:"rank"`
				} `json:"workflows"`
				TotalCount int `json:"total_count"`
			}

			err = json.Unmarshal(bodyBytes, &searchResults)
			Expect(err).ToNot(HaveOccurred())

			testLogger.Info("📊 Search Results:")
			for i, result := range searchResults.Workflows {
				testLogger.Info(fmt.Sprintf("  %d. %s", i+1, result.Workflow.Name),
					zap.Float64("final_score", result.FinalScore),
					zap.Float64("boost_score", result.BoostScore),
					zap.Float64("penalty_score", result.PenaltyScore),
					zap.Float64("similarity_score", result.SimilarityScore))
			}

			// Assertion 1: Search should return results
			Expect(searchResults.Workflows).ToNot(BeEmpty(), "Search should return workflows")
			Expect(searchResults.TotalCount).To(BeNumerically(">=", 3), "Should return at least 3 matching workflows")

			// Assertion 2: Top result should be GitOps + ArgoCD workflow (highest boost)
			topWorkflow := searchResults.Workflows[0]
			Expect(topWorkflow.Workflow.WorkflowID).To(Equal(workflows[0].workflowID),
				"GitOps + ArgoCD workflow should be ranked #1 due to boost")

			// Assertion 3: GitOps + ArgoCD workflow should have boost applied
			Expect(topWorkflow.BoostScore).To(BeNumerically(">", 0),
				"GitOps + ArgoCD workflow should have boost score > 0")
			Expect(topWorkflow.BoostScore).To(BeNumerically(">=", 0.15),
				"Expected boost: resource_management=gitops (+0.10) + gitops_tool=argocd (+0.05) = 0.15")

			// Assertion 4: GitOps + ArgoCD workflow should have no penalty
			Expect(topWorkflow.PenaltyScore).To(Equal(0.0),
				"GitOps + ArgoCD workflow should have no penalty")

			// Assertion 5: Manual workflow should have penalty applied (if returned)
			manualWorkflowFound := false
			for _, result := range searchResults.Workflows {
				if result.Workflow.WorkflowID == workflows[2].workflowID {
					manualWorkflowFound = true
					Expect(result.PenaltyScore).To(BeNumerically(">", 0),
						"Manual workflow should have penalty score > 0")
					Expect(result.PenaltyScore).To(BeNumerically(">=", 0.10),
						"Expected penalty: resource_management=manual (-0.10)")
					Expect(result.FinalScore).To(BeNumerically("<", topWorkflow.FinalScore),
						"Manual workflow should be ranked lower than GitOps workflow")
					break
				}
			}
			if manualWorkflowFound {
				testLogger.Info("✅ Manual workflow penalty validated")
			}

			// Assertion 6: Final scores should be capped at 1.0
			for _, result := range searchResults.Workflows {
				Expect(result.FinalScore).To(BeNumerically("<=", 1.0),
					"Final score should be capped at 1.0")
			}

			// Assertion 7: Results should be ordered by final_score descending
			for i := 1; i < len(searchResults.Workflows); i++ {
				Expect(searchResults.Workflows[i-1].FinalScore).To(BeNumerically(">=", searchResults.Workflows[i].FinalScore),
					"Results should be ordered by final_score descending")
			}

			// Assertion 8: Search latency should be acceptable (<200ms for local testing)
			Expect(searchDuration).To(BeNumerically("<", 200*time.Millisecond),
				"Search latency should be <200ms for E2E test (local infrastructure)")

			// Assertion 9: CrashLoopBackOff workflow should NOT be returned (different signal_type)
			for _, result := range searchResults.Workflows {
				Expect(result.Workflow.WorkflowID).ToNot(Equal(workflows[4].workflowID),
					"CrashLoopBackOff workflow should NOT be returned (mandatory label mismatch)")
			}

			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Hybrid Weighted Scoring Validation Complete")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Key Validations:")
			testLogger.Info("  ✅ GitOps + ArgoCD workflow ranked #1 (highest boost)")
			testLogger.Info("  ✅ Boost score applied correctly (≥0.15)")
			testLogger.Info("  ✅ Penalty score applied to manual workflow (≥0.10)")
			testLogger.Info("  ✅ Final scores capped at 1.0")
			testLogger.Info("  ✅ Results ordered by final_score descending")
			testLogger.Info("  ✅ Search latency <200ms")
			testLogger.Info("  ✅ Mandatory label filtering enforced (signal_type, severity)")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})
	})
})

// generateTestEmbedding creates a simple test embedding based on text
// In production, this would be generated by the embedding service
func generateTestEmbedding(text string) []float64 {
	// Generate a deterministic 384-dimensional embedding
	// For testing, we use a simple hash-based approach
	embedding := make([]float64, 384)
	hash := 0
	for _, c := range text {
		hash = (hash*31 + int(c)) % 1000
	}

	// Fill embedding with deterministic values based on hash
	for i := range embedding {
		embedding[i] = float64((hash+i)%100) / 100.0
	}

	return embedding
}
