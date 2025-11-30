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

// Scenario 6: Workflow Search Audit Trail (P0)
//
// Business Requirements:
// - BR-AUDIT-023: Audit event generation in Data Storage Service
// - BR-AUDIT-024: Asynchronous non-blocking audit (ADR-038)
// - BR-AUDIT-025: Query metadata capture
// - BR-AUDIT-026: Scoring capture (V1.0: confidence only)
// - BR-AUDIT-027: Workflow metadata capture
// - BR-AUDIT-028: Search metadata capture
//
// Business Value: Verify workflow search generates audit trail for compliance and debugging
//
// ARCHITECTURE: Uses SHARED deployment pattern (like Gateway E2E tests)
// - Services deployed ONCE in SynchronizedBeforeSuite
// - All tests share the same infrastructure via NodePort
// - No kubectl port-forward needed - eliminates instability
// - Uses dataStorageURL and postgresURL from suite
//
// Test Flow:
// 1. Use SHARED Data Storage Service (deployed in SynchronizedBeforeSuite)
// 2. Seed workflow catalog with test workflow
// 3. Perform workflow search with remediation_id
// 4. Query audit_events table to verify audit event was created
// 5. Validate audit event contains correct metadata
//
// Expected Results:
// - Audit event created with event_type = 'workflow.catalog.search_completed'
// - Audit event contains correlation_id = remediation_id
// - Audit event contains query metadata (text, filters, top_k)
// - Audit event contains scoring data (confidence)
// - Audit event contains workflow metadata (workflow_id, version, title)

var _ = Describe("Scenario 6: Workflow Search Audit Trail", Label("e2e", "workflow-search-audit", "p0"), Ordered, func() {
	var (
		testLogger logr.Logger
		httpClient *http.Client
		db         *sql.DB
		testID     string
	)

	BeforeAll(func() {
		testLogger = logger.WithValues("test", "workflow-search-audit")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario 6: Workflow Search Audit Trail - Using SHARED Services")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Using SHARED services (deployed in SynchronizedBeforeSuite)",
			"dataStorageURL", dataStorageURL,
			"postgresURL", postgresURL)

		// Generate unique test ID for workflow isolation within shared namespace
		testID = fmt.Sprintf("audit-e2e-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())

		// Connect to PostgreSQL for direct database verification via NodePort
		testLogger.Info("🔌 Connecting to PostgreSQL via NodePort...")
		var err error
		// Use NodePort URL (localhost:5432 mapped from NodePort 30432)
		dbConnStr := "host=localhost port=5432 user=slm_user password=test_password dbname=action_history sslmode=disable"
		db, err = sql.Open("pgx", dbConnStr)
		Expect(err).ToNot(HaveOccurred())
		Expect(db.Ping()).To(Succeed())

		testLogger.Info("✅ PostgreSQL connection established via NodePort")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("🧹 Cleaning up audit test resources...")

		// Close database connection
		if db != nil {
			if err := db.Close(); err != nil {
				testLogger.Info("warning: failed to close database connection", "error", err)
			}
		}

		testLogger.Info("✅ Audit test cleanup complete")
	})

	Context("when performing workflow search with remediation_id", func() {
		It("should generate audit event with complete metadata (BR-AUDIT-023 through BR-AUDIT-028)", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Test: Workflow Search Audit Trail Generation")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// ARRANGE: Seed workflow catalog with test workflow
			testLogger.Info("📦 Seeding workflow catalog with test workflow...")

			workflowID := fmt.Sprintf("wf-audit-test-%s", testID)

			// ADR-043 compliant workflow-schema.yaml content
			// YAML uses underscored keys (signal_type, risk_tolerance)
			// DD-WORKFLOW-001: All 7 mandatory labels required
			workflowSchemaContent := fmt.Sprintf(`apiVersion: kubernaut.io/v1alpha1
kind: WorkflowSchema
metadata:
  workflow_id: %s
  version: "1.0.0"
  description: Recover from OOMKilled using kubectl rollout restart
labels:
  signal_type: OOMKilled
  severity: critical
  environment: production
  priority: p0
  risk_tolerance: low
  business_category: revenue-critical
  component: deployment
parameters:
  - name: DEPLOYMENT_NAME
    type: string
    required: true
    description: Name of the deployment to restart
  - name: NAMESPACE
    type: string
    required: true
    description: Namespace of the deployment
execution:
  engine: tekton
  bundle: ghcr.io/kubernaut/workflows/oom-recovery:v1.0.0
`, workflowID)

			// DD-WORKFLOW-002 v2.4: container_image is MANDATORY with digest
			containerImage := fmt.Sprintf("ghcr.io/kubernaut/workflows/oom-recovery:v1.0.0@sha256:%064d", 1)

			workflow := map[string]interface{}{
				"workflow_name": workflowID, // DD-WORKFLOW-002 v3.0: workflow_name is the human identifier
				"version":       "1.0.0",
				"name":          "OOM Recovery Workflow",
				"description":   "Recover from OOMKilled using kubectl rollout restart",
				"owner":         "platform-team",
				"maintainer":    "oncall@example.com",
				"content":       workflowSchemaContent,
				// JSON labels use hyphenated keys (signal_type, risk_tolerance)
				"labels": map[string]interface{}{
					"signal_type":       "OOMKilled",
					"severity":          "critical",
					"environment":       "production",
					"priority":          "P0",
					"risk_tolerance":    "low",
					"business_category": "revenue-critical",
					"component":         "deployment",
				},
				"container_image": containerImage,
			}

			workflowJSON, err := json.Marshal(workflow)
			Expect(err).ToNot(HaveOccurred())

			// Create workflow via API (using shared NodePort URL)
			resp, err := httpClient.Post(
				dataStorageURL+"/api/v1/workflows",
				"application/json",
				bytes.NewBuffer(workflowJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				testLogger.Info("Failed to create workflow",
					"status", resp.StatusCode,
					"body", string(body))
			}
			Expect(resp.StatusCode).To(SatisfyAny(Equal(http.StatusCreated), Equal(http.StatusOK)),
				"Workflow creation should succeed")

			testLogger.Info("✅ Test workflow created", "workflow_id", workflowID)

			// ACT: Perform workflow search with remediation_id
			testLogger.Info("🔍 Performing workflow search with remediation_id...")

			remediationID := fmt.Sprintf("rem-%s", testID)
			searchRequest := map[string]interface{}{
				"query":          "OOMKilled critical memory increase",
				"remediation_id": remediationID,
				"filters": map[string]interface{}{
					"signal_type": "OOMKilled",
					"severity":    "critical",
				},
				"top_k":          5,
				"min_similarity": 0.5,
			}

			searchJSON, err := json.Marshal(searchRequest)
			Expect(err).ToNot(HaveOccurred())

			searchStart := time.Now()
			resp, err = httpClient.Post(
				dataStorageURL+"/api/v1/workflows/search",
				"application/json",
				bytes.NewBuffer(searchJSON),
			)
			searchDuration := time.Since(searchStart)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				fmt.Sprintf("Search should succeed, got: %s", string(body)))

			testLogger.Info("✅ Workflow search completed",
				"duration", searchDuration,
				"remediation_id", remediationID)

			// Wait for async audit buffer to flush (ADR-038)
			// The buffered audit store flushes every 100ms or on buffer full
			testLogger.Info("⏳ Waiting for async audit buffer to flush...")
			time.Sleep(500 * time.Millisecond)

			// ASSERT: Query audit_events table to verify audit event was created
			testLogger.Info("🔍 Querying audit_events table...")

			var (
				eventID       string
				eventType     string
				eventCategory string
				eventAction   string
				eventOutcome  string
				actorType     string
				actorID       string
				resourceType  string
				correlationID string
				eventData     []byte
			)

			// Query for the audit event with our remediation_id
			query := `
				SELECT
					event_id,
					event_type,
					event_category,
					event_action,
					event_outcome,
					actor_type,
					actor_id,
					resource_type,
					correlation_id,
					event_data
				FROM audit_events
				WHERE correlation_id = $1
				  AND event_type = 'workflow.catalog.search_completed'
				ORDER BY event_timestamp DESC
				LIMIT 1
			`

			// Use Eventually to handle async audit write timing
			Eventually(func() error {
				return db.QueryRow(query, remediationID).Scan(
					&eventID,
					&eventType,
					&eventCategory,
					&eventAction,
					&eventOutcome,
					&actorType,
					&actorID,
					&resourceType,
					&correlationID,
					&eventData,
				)
			}, "5s", "500ms").Should(Succeed(), "Audit event should be created within 5 seconds")

			testLogger.Info("✅ Audit event found",
				"event_id", eventID,
				"event_type", eventType,
				"correlation_id", correlationID)

			// Assertion 1: Verify event classification (BR-AUDIT-023)
			Expect(eventType).To(Equal("workflow.catalog.search_completed"),
				"Event type should be 'workflow.catalog.search_completed'")
			Expect(eventCategory).To(Equal("workflow"),
				"Event category should be 'workflow'")
			Expect(eventAction).To(Equal("search_completed"),
				"Event action should be 'search_completed'")
			Expect(eventOutcome).To(Equal("success"),
				"Event outcome should be 'success'")

			// Assertion 2: Verify actor information
			Expect(actorType).To(Equal("service"),
				"Actor type should be 'service'")
			Expect(actorID).To(Equal("datastorage"),
				"Actor ID should be 'datastorage'")

			// Assertion 3: Verify resource information
			Expect(resourceType).To(Equal("workflow_catalog"),
				"Resource type should be 'workflow_catalog'")

			// Assertion 4: Verify correlation_id matches remediation_id
			Expect(correlationID).To(Equal(remediationID),
				"Correlation ID should match remediation_id")

			// Assertion 5: Verify event_data contains expected metadata (BR-AUDIT-025 through BR-AUDIT-028)
			var eventDataMap map[string]interface{}
			err = json.Unmarshal(eventData, &eventDataMap)
			Expect(err).ToNot(HaveOccurred(), "event_data should be valid JSON")

			// Verify query metadata (BR-AUDIT-025)
			queryData, ok := eventDataMap["query"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "event_data should contain 'query' object")
			Expect(queryData["text"]).To(Equal("OOMKilled critical memory increase"),
				"Query text should match search request")
			Expect(queryData["top_k"]).To(BeNumerically("==", 5),
				"Query top_k should match search request")

			// Verify results metadata (BR-AUDIT-027)
			resultsData, ok := eventDataMap["results"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "event_data should contain 'results' object")
			Expect(resultsData["total_found"]).To(BeNumerically(">=", 1),
				"Results should contain at least 1 workflow")

			// Verify workflow scoring data (BR-AUDIT-026)
			workflows, ok := resultsData["workflows"].([]interface{})
			Expect(ok).To(BeTrue(), "results should contain 'workflows' array")
			Expect(workflows).ToNot(BeEmpty(), "workflows array should not be empty")

			firstWorkflow := workflows[0].(map[string]interface{})
			Expect(firstWorkflow["workflow_id"]).ToNot(BeEmpty(),
				"Workflow should have workflow_id")

			// V1.0: confidence only (DD-WORKFLOW-004 v2.0)
			scoring, ok := firstWorkflow["scoring"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "workflow should contain 'scoring' object")
			Expect(scoring["confidence"]).To(BeNumerically(">=", 0.5),
				"Confidence should be >= min_similarity threshold")

			// Verify search metadata (BR-AUDIT-028)
			searchMetadata, ok := eventDataMap["search_metadata"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "event_data should contain 'search_metadata' object")
			// Note: duration_ms may be 0 for sub-millisecond searches (Milliseconds() truncates)
			// The important thing is that the field exists and is a valid number >= 0
			Expect(searchMetadata["duration_ms"]).To(BeNumerically(">=", 0),
				"Search duration should be recorded (may be 0 for sub-millisecond searches)")

			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Workflow Search Audit Trail Validation Complete")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Key Validations:")
			testLogger.Info("  ✅ Audit event created with correct event_type")
			testLogger.Info("  ✅ Correlation_id matches remediation_id")
			testLogger.Info("  ✅ Query metadata captured (BR-AUDIT-025)")
			testLogger.Info("  ✅ Scoring data captured - V1.0 confidence only (BR-AUDIT-026)")
			testLogger.Info("  ✅ Workflow metadata captured (BR-AUDIT-027)")
			testLogger.Info("  ✅ Search metadata captured (BR-AUDIT-028)")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})

		It("should not block search response when audit write is slow (BR-AUDIT-024)", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Test: Async Audit Non-Blocking Behavior")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// ACT: Perform multiple rapid searches to test async behavior
			testLogger.Info("🔍 Performing rapid workflow searches...")

			var totalDuration time.Duration
			numSearches := 5

			for i := 0; i < numSearches; i++ {
				remediationID := fmt.Sprintf("rem-async-%s-%d", testID, i)
				searchRequest := map[string]interface{}{
					"query":          "OOMKilled critical",
					"remediation_id": remediationID,
					"filters": map[string]interface{}{
						"signal_type": "OOMKilled",
						"severity":    "critical",
					},
					"top_k": 3,
				}

				searchJSON, err := json.Marshal(searchRequest)
				Expect(err).ToNot(HaveOccurred())

				start := time.Now()
				resp, err := httpClient.Post(
					dataStorageURL+"/api/v1/workflows/search",
					"application/json",
					bytes.NewBuffer(searchJSON),
				)
				duration := time.Since(start)
				totalDuration += duration

				Expect(err).ToNot(HaveOccurred())
				resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				testLogger.Info(fmt.Sprintf("  Search %d completed", i+1),
					"duration", duration)
			}

			avgDuration := totalDuration / time.Duration(numSearches)

			// ASSERT: Average search latency should be <200ms (async audit should not add significant latency)
			// Per BR-AUDIT-024: Audit writes use buffered async pattern, search latency < 50ms impact
			Expect(avgDuration).To(BeNumerically("<", 200*time.Millisecond),
				"Average search latency should be <200ms (async audit should not block)")

			testLogger.Info("✅ Async audit behavior validated",
				"avg_latency", avgDuration,
				"num_searches", numSearches)
		})
	})
})
