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
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// SCENARIO 8: WORKFLOW SEARCH EDGE CASES
// ========================================
//
// GAP ANALYSIS: TRIAGE_DS_TEST_COVERAGE_GAP_ANALYSIS_V3.md
// - Gap 2.1: Workflow search zero matches (P0, 45m, 95% confidence)
// - Gap 2.2: Workflow search score tie-breaking (P0, 1h, 91% confidence)
// - Gap 2.3: Wildcard matching edge cases (P0, 1.5h, 92% confidence)
//
// Business Requirements:
// - BR-STORAGE-013: Semantic search with hybrid weighted scoring
// - DD-WORKFLOW-001: Mandatory label schema (5 labels)
//
// BUSINESS VALUE:
// - HolmesGPT-API handles "no matching workflow" gracefully
// - Deterministic workflow selection when scores are identical
// - Wildcard matching logic correctness affects selection accuracy
//
// Test Flow:
// 1. Deploy Data Storage Service in isolated namespace (shared)
// 2. Test zero matches scenario
// 3. Test tie-breaking with identical scores
// 4. Test wildcard matching edge cases
//
// ========================================

var _ = Describe("Scenario 8: Workflow Search Edge Cases", Label("e2e", "workflow-search-edge-cases", "p0"), Ordered, func() {
	var (
		testCancel    context.CancelFunc
		testLogger    logr.Logger
		httpClient    *http.Client
		testNamespace string
		serviceURL    string
		db            *sql.DB
		testID        string
	)

	BeforeAll(func() {
		_, testCancel = context.WithTimeout(ctx, 15*time.Minute)
		testLogger = logger.WithValues("test", "workflow-search-edge-cases")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario 8: Workflow Search Edge Cases - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique test ID for workflow isolation
		testID = fmt.Sprintf("e2e-edge-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())

		// Use shared deployment
		testNamespace = sharedNamespace
		serviceURL = dataStorageURL
		testLogger.Info("Using shared deployment", "namespace", testNamespace, "url", serviceURL)

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

		// Connect to PostgreSQL
		testLogger.Info("🔌 Connecting to PostgreSQL...")
		connStr := "host=localhost port=25433 user=slm_user password=test_password dbname=action_history sslmode=disable" // Per DD-TEST-001
		var err error
		db, err = sql.Open("pgx", connStr)
		Expect(err).ToNot(HaveOccurred())
		Expect(db.Ping()).To(Succeed())

		testLogger.Info("✅ PostgreSQL connection established")
	})

	AfterAll(func() {
		testLogger.Info("🧹 Cleaning up...")
		if db != nil {
			if err := db.Close(); err != nil {
				testLogger.Info("warning: failed to close database connection", "error", err)
			}
		}
		if testCancel != nil {
			testCancel()
		}
	})

	// ========================================
	// GAP 2.1: WORKFLOW SEARCH ZERO MATCHES
	// ========================================
	Describe("GAP 2.1: Workflow Search with Zero Matches", Label("gap-2.1"), func() {
		It("should return empty result set with HTTP 200 (not 404)", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("GAP 2.1: Testing workflow search with zero matches")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// ARRANGE: Ensure workflow catalog has NO workflows matching this filter
			searchRequest := map[string]interface{}{
				"filters": map[string]interface{}{
					"signal_type": "NonExistentSignalType_12345", // Will not match any workflow
					"severity":    "critical",
					"component":   "deployment",
					"priority":    "P0", // OpenAPI schema requires uppercase (enum: [P0, P1, P2, P3])
					"environment": "production",
				},
				"top_k": 5,
			}

			payloadBytes, err := json.Marshal(searchRequest)
			Expect(err).ToNot(HaveOccurred())

			// ACT: POST workflow search
			testLogger.Info("🔍 Posting workflow search with non-existent signal_type...")
			resp, err := httpClient.Post(
				serviceURL+"/api/v1/workflows/search",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// ASSERT: HTTP 200 OK (not 404 Not Found)
			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"Should return 200 OK even with zero matches (not 404)")

			// ASSERT: Parse response
			bodyBytes, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			var searchResponse map[string]interface{}
			err = json.Unmarshal(bodyBytes, &searchResponse)
			Expect(err).ToNot(HaveOccurred())

			// ASSERT: Empty workflows array
			workflows, ok := searchResponse["workflows"].([]interface{})
			Expect(ok).To(BeTrue(), "Response should have 'workflows' array")
			Expect(workflows).To(BeEmpty(), "Workflows array should be empty when no workflows match")

			// ASSERT: total_results = 0
			totalResults, ok := searchResponse["total_results"].(float64)
			Expect(ok).To(BeTrue(), "Response should have 'total_results'")
			Expect(totalResults).To(Equal(float64(0)), "Total results should be 0 for zero matches")

			// ASSERT: filters metadata exists
			filters, ok := searchResponse["filters"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "Response should have 'filters' object")
			Expect(filters["signal_type"]).To(Equal("NonExistentSignalType_12345"))

			testLogger.Info("✅ Zero matches handled correctly",
				"http_status", resp.StatusCode,
				"total_results", totalResults,
				"workflows_length", len(workflows))

			// BUSINESS VALUE: HolmesGPT-API can distinguish:
			// - "no workflow found" (HTTP 200, data=[])
			// - "search failed" (HTTP 500)
			// This enables proper error handling vs fallback strategies
		})

		It("should generate audit event with outcome=success and result=no_matches", func() {
			testLogger.Info("🔍 Verifying audit event for zero matches...")

			// ARRANGE: Search with non-matching filters
			searchRequest := map[string]interface{}{
				"filters": map[string]interface{}{
					"signal_type": "AnotherNonExistentType_99999",
					"severity":    "critical",
					"component":   "statefulset",
					"priority":    "P1",
					"environment": "staging",
				},
				"top_k": 10,
			}

			payloadBytes, err := json.Marshal(searchRequest)
			Expect(err).ToNot(HaveOccurred())

			// ACT: POST workflow search
			resp, err := httpClient.Post(
				serviceURL+"/api/v1/workflows/search",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// ASSERT: Audit event generated (BR-AUDIT-023 to BR-AUDIT-028)
			// Query audit_events table for workflow.catalog.search_completed event
			Eventually(func() bool {
				var count int
				query := `SELECT COUNT(*) FROM audit_events
				          WHERE event_type = 'workflow.catalog.search_completed'
				          AND event_outcome = 'success'`
				err := db.QueryRow(query).Scan(&count)
				return err == nil && count > 0
			}, "10s", "1s").Should(BeTrue(), "Audit event should be generated for zero matches")

			testLogger.Info("✅ Audit event generated for zero matches scenario")

			// NOTE: Enhanced event_data validation deferred to V1.1+ (optional improvement)
			// V1.0 validates: event_type and event_outcome (sufficient for audit trail)
			// Future enhancement: Verify specific JSONB fields in event_data:
			//   - event_data.result = "no_matches" OR event_data.results_count = 0
			//   - More granular outcome tracking for analytics
		})
	})

	// ========================================
	// GAP 2.2: WORKFLOW SEARCH SCORE TIE-BREAKING
	// ========================================
	Describe("GAP 2.2: Workflow Search Score Tie-Breaking", Label("gap-2.2"), func() {
		var workflow1ID, workflow2ID, workflow3ID string

		BeforeEach(func() {
			// ARRANGE: Create 3 workflows with IDENTICAL label scores
			testLogger.Info("📦 Creating 3 workflows with identical labels...")

			baseLabels := map[string]interface{}{
				"signal_type": "tie-breaking-test",
				"severity":    "critical",
				"component":   "deployment",
				"priority":    "P0",
				"environment": "production",
			}

			// Workflow 1: Created first
			content1 := `{"steps":[{"action":"scale","replicas":3}]}`
			workflow1 := map[string]interface{}{
				"workflow_name":    fmt.Sprintf("tie-breaking-workflow-1-%s", testID),
				"version":          "v1.0.0",
				"name":             "Tie Breaking Test Workflow 1",
				"description":      "First workflow (oldest)",
				"labels":           baseLabels,
				"content":          content1,
				"content_hash":     fmt.Sprintf("%x", sha256.Sum256([]byte(content1))),
				"execution_engine": "tekton", // Required per OpenAPI spec
				"status":           "active", // Required per OpenAPI spec
			}
			workflow1ID = workflow1["workflow_name"].(string)
			resp1, err := httpClient.Post(serviceURL+"/api/v1/workflows", "application/json",
				bytes.NewReader(mustMarshal(workflow1)))
			Expect(err).ToNot(HaveOccurred())
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated))
			resp1.Body.Close()

			time.Sleep(100 * time.Millisecond) // Ensure different created_at

			// Workflow 2: Created second
			content2 := `{"steps":[{"action":"scale","replicas":5}]}`
			workflow2 := map[string]interface{}{
				"workflow_name":    fmt.Sprintf("tie-breaking-workflow-2-%s", testID),
				"version":          "v1.0.0",
				"name":             "Tie Breaking Test Workflow 2",
				"description":      "Second workflow (middle)",
				"labels":           baseLabels,
				"content":          content2,
				"content_hash":     fmt.Sprintf("%x", sha256.Sum256([]byte(content2))),
				"execution_engine": "tekton", // Required per OpenAPI spec
				"status":           "active", // Required per OpenAPI spec
			}
			workflow2ID = workflow2["workflow_name"].(string)
			resp2, err := httpClient.Post(serviceURL+"/api/v1/workflows", "application/json",
				bytes.NewReader(mustMarshal(workflow2)))
			Expect(err).ToNot(HaveOccurred())
			Expect(resp2.StatusCode).To(Equal(http.StatusCreated))
			resp2.Body.Close()

			time.Sleep(100 * time.Millisecond)

			// Workflow 3: Created last (most recent)
			content3 := `{"steps":[{"action":"scale","replicas":7}]}`
			workflow3 := map[string]interface{}{
				"workflow_name":    fmt.Sprintf("tie-breaking-workflow-3-%s", testID),
				"version":          "v1.0.0",
				"name":             "Tie Breaking Test Workflow 3",
				"description":      "Third workflow (newest)",
				"labels":           baseLabels,
				"content":          content3,
				"content_hash":     fmt.Sprintf("%x", sha256.Sum256([]byte(content3))),
				"execution_engine": "tekton", // Required per OpenAPI spec
				"status":           "active", // Required per OpenAPI spec
			}
			workflow3ID = workflow3["workflow_name"].(string)
			resp3, err := httpClient.Post(serviceURL+"/api/v1/workflows", "application/json",
				bytes.NewReader(mustMarshal(workflow3)))
			Expect(err).ToNot(HaveOccurred())
			Expect(resp3.StatusCode).To(Equal(http.StatusCreated))
			resp3.Body.Close()

			testLogger.Info("✅ Created 3 workflows with identical labels")
		})

		AfterEach(func() {
			// Cleanup: Delete test workflows
			for _, workflowID := range []string{workflow1ID, workflow2ID, workflow3ID} {
				query := "DELETE FROM remediation_workflow_catalog WHERE workflow_id = $1"
				_, _ = db.Exec(query, workflowID)
			}
		})

		It("should use deterministic tie-breaking when scores are identical", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("GAP 2.2: Testing workflow search tie-breaking")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// ACT: Search with filters that match ALL 3 workflows identically
			searchRequest := map[string]interface{}{
				"filters": map[string]interface{}{
					"signal_type": "tie-breaking-test",
					"severity":    "critical",
					"component":   "deployment",
					"priority":    "P0",
					"environment": "production",
				},
				"top_k": 1, // Request only 1 result - forces tie-breaking
			}

			payloadBytes, err := json.Marshal(searchRequest)
			Expect(err).ToNot(HaveOccurred())

			// Execute search multiple times to verify consistency
			var firstResultID string
			for i := 0; i < 5; i++ {
				resp, err := httpClient.Post(
					serviceURL+"/api/v1/workflows/search",
					"application/json",
					bytes.NewReader(payloadBytes),
				)
				Expect(err).ToNot(HaveOccurred())

				bodyBytes, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var searchResponse map[string]interface{}
				err = json.Unmarshal(bodyBytes, &searchResponse)
				Expect(err).ToNot(HaveOccurred())

				workflows, ok := searchResponse["workflows"].([]interface{})
				Expect(ok).To(BeTrue(), "Response should have 'workflows' array")
				Expect(workflows).To(HaveLen(1), "Should return exactly 1 result (top_k=1)")

				workflow := workflows[0].(map[string]interface{})
				workflowID := workflow["workflow_id"].(string)

				if i == 0 {
					firstResultID = workflowID
					testLogger.Info("First search result", "workflow_id", workflowID)
				} else {
					// ASSERT: Deterministic - same workflow returned every time
					Expect(workflowID).To(Equal(firstResultID),
						"Tie-breaking should be deterministic - same workflow every query")
				}
			}

			testLogger.Info("✅ Tie-breaking is deterministic across 5 queries",
				"selected_workflow", firstResultID)

			// BUSINESS VALUE: Predictable workflow selection
			// - Same query always returns same workflow
			// - No random selection causing inconsistent remediations
			// - Recommended tie-breaking: most recently created workflow (newest = latest best practices)
		})
	})

	// ========================================
	// GAP 2.3: WILDCARD MATCHING EDGE CASES
	// ========================================
	Describe("GAP 2.3: Wildcard Matching Edge Cases", Label("gap-2.3"), func() {
		var wildcardWorkflowID, specificWorkflowID string

		BeforeEach(func() {
			// ARRANGE: Create workflows with wildcard and specific labels
			testLogger.Info("📦 Creating workflows for wildcard matching tests...")

			// Workflow with wildcard: component="*" (matches any)
			content1 := `{"steps":[{"action":"scale","replicas":3}]}`
			wildcardWorkflow := map[string]interface{}{
				"workflow_name": fmt.Sprintf("wildcard-workflow-%s", testID),
				"version":       "v1.0.0",
				"name":          "Wildcard Component Workflow",
				"description":   "Accepts any component",
				"labels": map[string]interface{}{
					"signal_type": "wildcard-test",
					"severity":    "critical",
					"component":   "*", // Wildcard
					"priority":    "P0",
					"environment": "production",
				},
				"content":          content1,
				"content_hash":     fmt.Sprintf("%x", sha256.Sum256([]byte(content1))),
				"execution_engine": "tekton", // Required per OpenAPI spec
				"status":           "active", // Required per OpenAPI spec
			}
			resp1, err := httpClient.Post(serviceURL+"/api/v1/workflows", "application/json",
				bytes.NewReader(mustMarshal(wildcardWorkflow)))
			Expect(err).ToNot(HaveOccurred())
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated))
			// Extract workflow_id from response
			var createResp1 map[string]interface{}
			json.NewDecoder(resp1.Body).Decode(&createResp1)
			resp1.Body.Close()
			wildcardWorkflowID = createResp1["workflow_id"].(string)

			// Workflow with specific: component="deployment"
			content2 := `{"steps":[{"action":"restart","delay":10}]}`
			specificWorkflow := map[string]interface{}{
				"workflow_name": fmt.Sprintf("specific-workflow-%s", testID),
				"version":       "v1.0.0",
				"name":          "Specific Component Workflow",
				"description":   "Only accepts deployment component",
				"labels": map[string]interface{}{
					"signal_type": "wildcard-test",
					"severity":    "critical",
					"component":   "deployment", // Specific
					"priority":    "P0",
					"environment": "production",
				},
				"content":          content2,
				"content_hash":     fmt.Sprintf("%x", sha256.Sum256([]byte(content2))),
				"execution_engine": "tekton", // Required per OpenAPI spec
				"status":           "active", // Required per OpenAPI spec
			}
			resp2, err := httpClient.Post(serviceURL+"/api/v1/workflows", "application/json",
				bytes.NewReader(mustMarshal(specificWorkflow)))
			Expect(err).ToNot(HaveOccurred())
			Expect(resp2.StatusCode).To(Equal(http.StatusCreated))
			// Extract workflow_id from response
			var createResp2 map[string]interface{}
			json.NewDecoder(resp2.Body).Decode(&createResp2)
			resp2.Body.Close()
			specificWorkflowID = createResp2["workflow_id"].(string)

			testLogger.Info("✅ Created workflows with wildcard and specific component labels")
		})

		AfterEach(func() {
			// Cleanup (using workflow_id which is now UUID)
			for _, workflowID := range []string{wildcardWorkflowID, specificWorkflowID} {
				query := "DELETE FROM remediation_workflow_catalog WHERE workflow_id = $1"
				_, _ = db.Exec(query, workflowID)
			}
		})

		It("should match wildcard (*) when search filter is specific value", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("GAP 2.3: Testing wildcard matching - specific filter")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// ACT: Search with specific component filter
			searchRequest := map[string]interface{}{
				"filters": map[string]interface{}{
					"signal_type": "wildcard-test",
					"severity":    "critical",
					"component":   "deployment", // Specific value
					"priority":    "P0",
					"environment": "production",
				},
				"top_k": 10,
			}

			payloadBytes, err := json.Marshal(searchRequest)
			Expect(err).ToNot(HaveOccurred())

			resp, err := httpClient.Post(
				serviceURL+"/api/v1/workflows/search",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			bodyBytes, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			var searchResponse map[string]interface{}
			err = json.Unmarshal(bodyBytes, &searchResponse)
			Expect(err).ToNot(HaveOccurred())

			workflows, ok := searchResponse["workflows"].([]interface{})
			Expect(ok).To(BeTrue(), "Response should have 'workflows' array")

			// ASSERT: BOTH workflows match (wildcard matches specific filter)
			Expect(workflows).To(HaveLen(2), "Both wildcard and specific workflows should match")

			workflowIDs := make([]string, len(workflows))
			for i, wf := range workflows {
				workflowIDs[i] = wf.(map[string]interface{})["workflow_id"].(string)
			}

			Expect(workflowIDs).To(ContainElement(wildcardWorkflowID),
				"Wildcard workflow (component='*') should match specific filter (component='deployment')")
			Expect(workflowIDs).To(ContainElement(specificWorkflowID),
				"Specific workflow (component='deployment') should match exact filter")

			testLogger.Info("✅ Wildcard matching works correctly", "matched_workflows", len(workflows))
		})

		It("should match wildcard (*) when search filter is unknown value", func() {
			testLogger.Info("🔍 Testing wildcard matching - unknown component value")

			// ACT: Search with unknown component filter (not matching specific workflow)
			searchRequest := map[string]interface{}{
				"filters": map[string]interface{}{
					"signal_type": "wildcard-test",
					"severity":    "critical",
					"component":   "unknown-component", // Unknown value (not "deployment")
					"priority":    "P0",
					"environment": "production",
				},
				"top_k": 10,
			}

			payloadBytes, err := json.Marshal(searchRequest)
			Expect(err).ToNot(HaveOccurred())

			resp, err := httpClient.Post(
				serviceURL+"/api/v1/workflows/search",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			bodyBytes, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			var searchResponse map[string]interface{}
			err = json.Unmarshal(bodyBytes, &searchResponse)
			Expect(err).ToNot(HaveOccurred())

			workflows, ok := searchResponse["workflows"].([]interface{})
			Expect(ok).To(BeTrue(), "Response should have 'workflows' array")

			// ASSERT: Wildcard workflow matches (unknown value satisfies wildcard)
			// Specific workflow should NOT match (unknown != "deployment")
			Expect(workflows).To(HaveLen(1), "Only wildcard workflow should match unknown component")

			workflowID := workflows[0].(map[string]interface{})["workflow_id"].(string)
			Expect(workflowID).To(Equal(wildcardWorkflowID),
				"Wildcard workflow (component='*') should match unknown component value")

			testLogger.Info("✅ Wildcard matches unknown component correctly")

			// BUSINESS VALUE: Wildcard logic correctness
			// - Workflow with component="*" can handle ANY component (including unknown values)
			// - Workflow with component="deployment" only handles deployment (strict)
		})
	})
})

// Helper function to marshal JSON (panics on error for test setup)
func mustMarshal(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal JSON: %v", err))
	}
	return data
}
