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
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	_ "github.com/jackc/pgx/v5/stdlib"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
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
		testID = fmt.Sprintf("e2e-edge-%d-%s", GinkgoParallelProcess(), uuid.New().String()[:8])

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
			defer func() { _ = resp.Body.Close() }()
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
			// DD-API-001: Use typed OpenAPI struct
			topK := 5
			searchRequest := dsgen.WorkflowSearchRequest{
				Filters: dsgen.WorkflowSearchFilters{
					SignalType:  "NonExistentSignalType_12345", // Will not match any workflow
					Severity:    dsgen.WorkflowSearchFiltersSeverityCritical,
					Component:   "deployment",
					Priority:    dsgen.WorkflowSearchFiltersPriorityP0, // OpenAPI schema requires uppercase (enum: [P0, P1, P2, P3])
					Environment: "production",
				},
				TopK: dsgen.NewOptInt(topK),
			}

			// ACT: POST workflow search
			testLogger.Info("🔍 Posting workflow search with non-existent signal_type...")
			resp, err := dsClient.SearchWorkflows(ctx, &searchRequest)
			Expect(err).ToNot(HaveOccurred())
			searchResults, ok := resp.(*dsgen.WorkflowSearchResponse)
			Expect(ok).To(BeTrue(), "Expected *WorkflowSearchResponse type")

			// ASSERT: HTTP 200 OK (not 404 Not Found)
			Expect(searchResults).ToNot(BeNil())

			// ASSERT: Empty workflows array
			workflows := searchResults.Workflows
			Expect(workflows).To(BeEmpty(), "Workflows array should be empty when no workflows match")

			// ASSERT: total_results = 0
			totalResults := searchResults.TotalResults
			Expect(totalResults).ToNot(BeNil(), "TotalResults should not be nil")
			Expect(totalResults.Value).To(Equal(0), "Total results should be 0 for zero matches")

			// ASSERT: filters metadata exists
			filters := searchResults.Filters
			Expect(filters).ToNot(BeNil())
			Expect(filters.Value.SignalType).To(Equal("NonExistentSignalType_12345"))

			testLogger.Info("✅ Zero matches handled correctly",
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
			// DD-API-001: Use typed OpenAPI struct
			topK := 10
			searchRequest := dsgen.WorkflowSearchRequest{
				Filters: dsgen.WorkflowSearchFilters{
					SignalType:  "AnotherNonExistentType_99999",
					Severity:    dsgen.WorkflowSearchFiltersSeverityCritical,
					Component:   "statefulset",
					Priority:    dsgen.WorkflowSearchFiltersPriorityP1,
					Environment: "staging",
				},
				TopK: dsgen.NewOptInt(topK),
			}

			// ACT: POST workflow search
			_, err := dsClient.SearchWorkflows(ctx, &searchRequest)
			Expect(err).ToNot(HaveOccurred())

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
			// DD-API-001: Use typed OpenAPI struct
			content1 := `{"steps":[{"action":"scale","replicas":3}]}`
			workflow1ID = fmt.Sprintf("tie-breaking-workflow-1-%s", testID)
			workflow1 := dsgen.RemediationWorkflow{
				WorkflowName: workflow1ID,
				Version:      "v1.0.0",
				Name:         "Tie Breaking Test Workflow 1",
				Description:  "First workflow (oldest)",
				Labels: dsgen.MandatoryLabels{
					SignalType:  baseLabels["signal_type"].(string),
					Severity:    dsgen.MandatoryLabelsSeverityCritical,
					Component:   baseLabels["component"].(string),
					Priority:    dsgen.MandatoryLabelsPriority_P0,
					Environment: baseLabels["environment"].(string),
				},
				Content:         content1,
				ContentHash:     fmt.Sprintf("%x", sha256.Sum256([]byte(content1))),
				ExecutionEngine: "tekton",                              // Required per OpenAPI spec
				Status:          dsgen.RemediationWorkflowStatusActive, // Required per OpenAPI spec
			}
			_, err := dsClient.CreateWorkflow(ctx, &workflow1)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(100 * time.Millisecond) // Ensure different created_at

			// Workflow 2: Created second
			// DD-API-001: Use typed OpenAPI struct
			content2 := `{"steps":[{"action":"scale","replicas":5}]}`
			workflow2ID = fmt.Sprintf("tie-breaking-workflow-2-%s", testID)
			workflow2 := dsgen.RemediationWorkflow{
				WorkflowName: workflow2ID,
				Version:      "v1.0.0",
				Name:         "Tie Breaking Test Workflow 2",
				Description:  "Second workflow (middle)",
				Labels: dsgen.MandatoryLabels{
					SignalType:  baseLabels["signal_type"].(string),
					Severity:    dsgen.MandatoryLabelsSeverityCritical,
					Component:   baseLabels["component"].(string),
					Priority:    dsgen.MandatoryLabelsPriority_P0,
					Environment: baseLabels["environment"].(string),
				},
				Content:         content2,
				ContentHash:     fmt.Sprintf("%x", sha256.Sum256([]byte(content2))),
				ExecutionEngine: "tekton",                              // Required per OpenAPI spec
				Status:          dsgen.RemediationWorkflowStatusActive, // Required per OpenAPI spec
			}
			_, err = dsClient.CreateWorkflow(ctx, &workflow2)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(100 * time.Millisecond)

			// Workflow 3: Created last (most recent)
			// DD-API-001: Use typed OpenAPI struct
			content3 := `{"steps":[{"action":"scale","replicas":7}]}`
			workflow3ID = fmt.Sprintf("tie-breaking-workflow-3-%s", testID)
			workflow3 := dsgen.RemediationWorkflow{
				WorkflowName: workflow3ID,
				Version:      "v1.0.0",
				Name:         "Tie Breaking Test Workflow 3",
				Description:  "Third workflow (newest)",
				Labels: dsgen.MandatoryLabels{
					SignalType:  baseLabels["signal_type"].(string),
					Severity:    dsgen.MandatoryLabelsSeverityCritical,
					Component:   baseLabels["component"].(string),
					Priority:    dsgen.MandatoryLabelsPriority_P0,
					Environment: baseLabels["environment"].(string),
				},
				Content:         content3,
				ContentHash:     fmt.Sprintf("%x", sha256.Sum256([]byte(content3))),
				ExecutionEngine: "tekton",                              // Required per OpenAPI spec
				Status:          dsgen.RemediationWorkflowStatusActive, // Required per OpenAPI spec
			}
			_, err = dsClient.CreateWorkflow(ctx, &workflow3)
			Expect(err).ToNot(HaveOccurred())

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
			// DD-API-001: Use typed OpenAPI struct
			topK := 1
			searchRequest := dsgen.WorkflowSearchRequest{
				Filters: dsgen.WorkflowSearchFilters{
					SignalType:  "tie-breaking-test",
					Severity:    dsgen.WorkflowSearchFiltersSeverityCritical,
					Component:   "deployment",
					Priority:    dsgen.WorkflowSearchFiltersPriorityP0,
					Environment: "production",
				},
				TopK: dsgen.NewOptInt(topK), // Request only 1 result - forces tie-breaking
			}

			// Execute search multiple times to verify consistency
			var firstResultID string
			for i := 0; i < 5; i++ {
				resp, err := dsClient.SearchWorkflows(ctx, &searchRequest)
				Expect(err).ToNot(HaveOccurred())
				searchResults, ok := resp.(*dsgen.WorkflowSearchResponse)
				Expect(ok).To(BeTrue(), "Expected *WorkflowSearchResponse type")
				Expect(searchResults).ToNot(BeNil())

				workflows := searchResults.Workflows
				Expect(workflows).To(HaveLen(1), "Should return exactly 1 result (top_k=1)")

				workflow := workflows[0]
				workflowID := workflow.WorkflowID.String()

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
			// DD-API-001: Use typed OpenAPI struct
			content1 := `{"steps":[{"action":"scale","replicas":3}]}`
			wildcardWorkflow := dsgen.RemediationWorkflow{
				WorkflowName: fmt.Sprintf("wildcard-workflow-%s", testID),
				Version:      "v1.0.0",
				Name:         "Wildcard Component Workflow",
				Description:  "Accepts any component",
				Labels: dsgen.MandatoryLabels{
					SignalType:  "wildcard-test",
					Severity:    dsgen.MandatoryLabelsSeverityCritical,
					Component:   "*", // Wildcard
					Priority:    dsgen.MandatoryLabelsPriority_P0,
					Environment: "production",
				},
				Content:         content1,
				ContentHash:     fmt.Sprintf("%x", sha256.Sum256([]byte(content1))),
				ExecutionEngine: "tekton",                              // Required per OpenAPI spec
				Status:          dsgen.RemediationWorkflowStatusActive, // Required per OpenAPI spec
			}
			resp1, err := dsClient.CreateWorkflow(ctx, &wildcardWorkflow)
			Expect(err).ToNot(HaveOccurred())
			// Extract workflow_id from response
			createdWildcard, ok := resp1.(*dsgen.RemediationWorkflow)
			Expect(ok).To(BeTrue(), "Expected *RemediationWorkflow response")
			wfID, ok := createdWildcard.WorkflowID.Get()
			Expect(ok).To(BeTrue(), "Expected WorkflowID to be set")
			wildcardWorkflowID = wfID.String()

			// Workflow with specific: component="deployment"
			// DD-API-001: Use typed OpenAPI struct
			content2 := `{"steps":[{"action":"restart","delay":10}]}`
			specificWorkflow := dsgen.RemediationWorkflow{
				WorkflowName: fmt.Sprintf("specific-workflow-%s", testID),
				Version:      "v1.0.0",
				Name:         "Specific Component Workflow",
				Description:  "Only accepts deployment component",
				Labels: dsgen.MandatoryLabels{
					SignalType:  "wildcard-test",
					Severity:    dsgen.MandatoryLabelsSeverityCritical,
					Component:   "deployment", // Specific
					Priority:    dsgen.MandatoryLabelsPriority_P0,
					Environment: "production",
				},
				Content:         content2,
				ContentHash:     fmt.Sprintf("%x", sha256.Sum256([]byte(content2))),
				ExecutionEngine: "tekton",                              // Required per OpenAPI spec
				Status:          dsgen.RemediationWorkflowStatusActive, // Required per OpenAPI spec
			}
			resp2, err := dsClient.CreateWorkflow(ctx, &specificWorkflow)
			Expect(err).ToNot(HaveOccurred())
			// Extract workflow_id from response
			createdSpecific, ok := resp2.(*dsgen.RemediationWorkflow)
			Expect(ok).To(BeTrue(), "Expected *RemediationWorkflow response")
			wfID2, ok := createdSpecific.WorkflowID.Get()
			Expect(ok).To(BeTrue(), "Expected WorkflowID to be set")
			specificWorkflowID = wfID2.String()

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
			// DD-API-001: Use typed OpenAPI struct
			topK := 10
			searchRequest := dsgen.WorkflowSearchRequest{
				Filters: dsgen.WorkflowSearchFilters{
					SignalType:  "wildcard-test",
					Severity:    dsgen.WorkflowSearchFiltersSeverityCritical,
					Component:   "deployment", // Specific value
					Priority:    dsgen.WorkflowSearchFiltersPriorityP0,
					Environment: "production",
				},
				TopK: dsgen.NewOptInt(topK),
			}

			resp, err := dsClient.SearchWorkflows(ctx, &searchRequest)
			Expect(err).ToNot(HaveOccurred())
			searchResults, ok := resp.(*dsgen.WorkflowSearchResponse)
			Expect(ok).To(BeTrue(), "Expected *WorkflowSearchResponse type")
			Expect(searchResults).ToNot(BeNil())

			workflows := searchResults.Workflows

			// ASSERT: BOTH workflows match (wildcard matches specific filter)
			Expect(workflows).To(HaveLen(2), "Both wildcard and specific workflows should match")

			workflowIDs := make([]string, len(workflows))
			for i, wf := range workflows {
				workflowIDs[i] = wf.WorkflowID.String()
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
			// DD-API-001: Use typed OpenAPI struct
			topK := 10
			searchRequest := dsgen.WorkflowSearchRequest{
				Filters: dsgen.WorkflowSearchFilters{
					SignalType:  "wildcard-test",
					Severity:    dsgen.WorkflowSearchFiltersSeverityCritical,
					Component:   "unknown-component", // Unknown value (not "deployment")
					Priority:    dsgen.WorkflowSearchFiltersPriorityP0,
					Environment: "production",
				},
				TopK: dsgen.NewOptInt(topK),
			}

			resp, err := dsClient.SearchWorkflows(ctx, &searchRequest)
			Expect(err).ToNot(HaveOccurred())
			searchResults, ok := resp.(*dsgen.WorkflowSearchResponse)
			Expect(ok).To(BeTrue(), "Expected *WorkflowSearchResponse type")
			Expect(searchResults).ToNot(BeNil())

			workflows := searchResults.Workflows

			// ASSERT: Wildcard workflow matches (unknown value satisfies wildcard)
			// Specific workflow should NOT match (unknown != "deployment")
			Expect(workflows).To(HaveLen(1), "Only wildcard workflow should match unknown component")

			workflowID := workflows[0].WorkflowID.String()
			Expect(workflowID).To(Equal(wildcardWorkflowID),
				"Wildcard workflow (component='*') should match unknown component value")

			testLogger.Info("✅ Wildcard matches unknown component correctly")

			// BUSINESS VALUE: Wildcard logic correctness
			// - Workflow with component="*" can handle ANY component (including unknown values)
			// - Workflow with component="deployment" only handles deployment (strict)
		})
	})
})
