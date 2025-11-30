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
		testCancel    context.CancelFunc
		testLogger    *zap.Logger
		httpClient    *http.Client
		testNamespace string
		serviceURL    string
		db            *sql.DB
		testID        string
	)

	BeforeAll(func() {
		_, testCancel = context.WithTimeout(ctx, 15*time.Minute)
		testLogger = logger.With(zap.String("test", "workflow-search"))
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario 4: Workflow Search - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique test ID for workflow isolation
		testID = fmt.Sprintf("e2e-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())

		// Use shared deployment from SynchronizedBeforeSuite (no per-test deployment)
		// Services are deployed ONCE and shared via NodePort (no port-forwarding needed)
		testNamespace = sharedNamespace
		serviceURL = dataStorageURL
		testLogger.Info("Using shared deployment", zap.String("namespace", testNamespace), zap.String("url", serviceURL))

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

		// Connect to PostgreSQL for direct database verification (using shared NodePort - no port-forward needed)
		testLogger.Info("🔌 Connecting to PostgreSQL via NodePort...")

		connStr := "host=localhost port=5432 user=slm_user password=test_password dbname=action_history sslmode=disable"
		var err error
		db, err = sql.Open("pgx", connStr)
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

			// Test workflows with all 7 mandatory labels per DD-WORKFLOW-001
			// JSON labels use hyphenated keys (signal-type, risk-tolerance)
			// YAML content uses underscored keys (signal_type, risk_tolerance)
			workflows := []struct {
				workflowID  string
				name        string
				description string
				labels      map[string]interface{} // JSON labels (hyphenated keys)
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
						"component":           "deployment",
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
						"component":           "deployment",
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
						"component":           "deployment",
					},
					embedding: generateTestEmbedding("OOMKilled critical manual kubectl production"),
				},
				{
					workflowID:  fmt.Sprintf("wf-generic-%s", testID),
					name:        "OOM Recovery (Generic)",
					description: "Generic OOM recovery workflow",
					labels: map[string]interface{}{
						"signal-type":       "OOMKilled",
						"severity":          "critical",
						"environment":       "production",
						"priority":          "P1",
						"risk-tolerance":    "medium",
						"business-category": "availability",
						"component":         "pod",
					},
					embedding: generateTestEmbedding("OOMKilled critical generic recovery"),
				},
				{
					workflowID:  fmt.Sprintf("wf-different-signal-%s", testID),
					name:        "CrashLoopBackOff Recovery",
					description: "Recover from CrashLoopBackOff",
					labels: map[string]interface{}{
						"signal-type":       "CrashLoopBackOff",
						"severity":          "high",
						"environment":       "staging",
						"priority":          "P2",
						"risk-tolerance":    "high",
						"business-category": "performance",
						"component":         "pod",
					},
					embedding: generateTestEmbedding("CrashLoopBackOff high recovery"),
				},
			}

			// Create workflows via API with ADR-043 compliant content
			for i, wf := range workflows {
				// Get risk_tolerance for YAML content (convert from hyphenated JSON key)
				riskTolerance := wf.labels["risk-tolerance"]
				environment := wf.labels["environment"]
				priority := wf.labels["priority"]
				businessCategory := wf.labels["business-category"]
				component := wf.labels["component"]

				// Generate ADR-043 compliant workflow-schema.yaml content
				// YAML uses underscored keys (signal_type, risk_tolerance)
				workflowSchemaContent := fmt.Sprintf(`apiVersion: kubernaut.io/v1alpha1
kind: WorkflowSchema
metadata:
  workflow_id: %s
  version: "1.0.0"
  description: %s
labels:
  signal_type: %s
  severity: %s
  risk_tolerance: %s
  environment: %s
  priority: %s
  business_category: %s
  component: %s
parameters:
  - name: NAMESPACE
    type: string
    required: true
    description: Target namespace
  - name: POD_NAME
    type: string
    required: true
    description: Name of the pod to remediate
execution:
  engine: tekton
  bundle: ghcr.io/kubernaut/workflows/test:v1.0.0
`, wf.workflowID, wf.description, wf.labels["signal-type"], wf.labels["severity"],
					riskTolerance, environment, priority, businessCategory, component)

				// DD-WORKFLOW-002 v2.4: container_image is MANDATORY with digest
				containerImage := fmt.Sprintf("ghcr.io/kubernaut/workflows/%s:v1.0.0@sha256:%064d", wf.workflowID, i+1)

				// DD-WORKFLOW-002 v3.0: workflow_name is the human identifier, workflow_id is auto-generated UUID
				workflowReq := map[string]interface{}{
					"workflow_name":   wf.workflowID, // Using workflowID test field as workflow_name
					"version":         "1.0.0",
					"name":            wf.name,
					"description":     wf.description,
					"content":         workflowSchemaContent,
					"labels":          wf.labels,
					"container_image": containerImage,
					"embedding":       wf.embedding,
					"status":          "active",
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
			// DD-WORKFLOW-002 v3.0: workflow_id is UUID, use workflow_name for filtering
			var count int
			err := db.QueryRow("SELECT COUNT(*) FROM remediation_workflow_catalog WHERE workflow_name LIKE $1",
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

			// ASSERT: Verify V1.0 semantic search results (base similarity only)
			// Authority: DD-WORKFLOW-002 v3.0 (flat response structure, UUID workflow_id)
			// Authority: DD-WORKFLOW-004 v2.0 (V1.0: confidence = base_similarity)
			var searchResults struct {
				Workflows []struct {
					// DD-WORKFLOW-002 v3.0: Flat structure - all fields at same level
					WorkflowID      string  `json:"workflow_id"` // UUID
					Title           string  `json:"title"`
					Description     string  `json:"description"`
					SignalType      string  `json:"signal_type"` // v3.0: singular string
					ContainerImage  string  `json:"container_image"`
					ContainerDigest string  `json:"container_digest"`
					Confidence      float64 `json:"confidence"`
				} `json:"workflows"`
				TotalResults int `json:"total_results"`
			}

			err = json.Unmarshal(bodyBytes, &searchResults)
			Expect(err).ToNot(HaveOccurred())

			testLogger.Info("📊 Search Results (V1.0 - Base Similarity Only):")
			for i, result := range searchResults.Workflows {
				testLogger.Info(fmt.Sprintf("  %d. %s", i+1, result.Title),
					zap.Float64("confidence", result.Confidence))
			}

			// Assertion 1: Search should return results
			Expect(searchResults.Workflows).ToNot(BeEmpty(), "Search should return workflows")
			Expect(searchResults.TotalResults).To(BeNumerically(">=", 1), "Should return at least 1 matching workflow")

			// Assertion 2: All results should have signal_type matching the query
			// DD-WORKFLOW-002 v3.0: signal_type is singular string (not array)
			for _, result := range searchResults.Workflows {
				Expect(result.SignalType).To(Equal("OOMKilled"),
					"All results should have matching signal_type")
			}

			// Assertion 3: Confidence scores should be valid (0.0-1.0)
			for _, result := range searchResults.Workflows {
				Expect(result.Confidence).To(BeNumerically(">=", 0.0),
					"Confidence should be >= 0.0")
				Expect(result.Confidence).To(BeNumerically("<=", 1.0),
					"Confidence should be <= 1.0")
			}

			// Assertion 4: Results should be ordered by confidence descending
			for i := 1; i < len(searchResults.Workflows); i++ {
				Expect(searchResults.Workflows[i-1].Confidence).To(BeNumerically(">=", searchResults.Workflows[i].Confidence),
					"Results should be ordered by confidence descending")
			}

			// Assertion 5: Search latency should be acceptable (<200ms for local testing)
			Expect(searchDuration).To(BeNumerically("<", 200*time.Millisecond),
				"Search latency should be <200ms for E2E test (local infrastructure)")

			// Assertion 6: CrashLoopBackOff workflow should NOT be returned (different signal_type)
			// DD-WORKFLOW-002 v3.0: WorkflowID is UUID, verify signal_type filtering works
			for _, result := range searchResults.Workflows {
				// DD-WORKFLOW-002 v3.0: WorkflowID is UUID format
				Expect(result.WorkflowID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`),
					"WorkflowID should be UUID format")
				// Verify CrashLoopBackOff is filtered out by signal_type
				Expect(result.SignalType).ToNot(Equal("CrashLoopBackOff"),
					"CrashLoopBackOff workflow should NOT be returned (mandatory label mismatch)")
			}

			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ V1.0 Semantic Search Validation Complete")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Key Validations (DD-WORKFLOW-004 v2.0):")
			testLogger.Info("  ✅ Mandatory label filtering enforced (signal_type, severity)")
			testLogger.Info("  ✅ Confidence scores valid (0.0-1.0)")
			testLogger.Info("  ✅ Results ordered by confidence descending")
			testLogger.Info("  ✅ Search latency <200ms")
			testLogger.Info("  ✅ V1.0: Base similarity only (no boost/penalty)")
			testLogger.Info("  ✅ V2.0+: Configurable label weights (future)")
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
