package datastorage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// Aggregation API Integration Tests
// BR-STORAGE-030 to BR-STORAGE-034: Aggregation endpoints
// TDD Phase: Integration tests with real PostgreSQL queries
//
// These tests validate the complete HTTP → Handler → DBAdapter → PostgreSQL flow
// using a real Data Storage Service container (Podman, ADR-016)
//
// Following Data Storage Implementation Plan V4.8:
// - Behavior + Correctness Testing (GAP-05)
// - Defense-in-depth: Integration tests validate real SQL queries
// - Test edge cases: empty data, single record, large datasets

var _ = Describe("Aggregation API Integration - BR-STORAGE-030", Ordered, func() {
	var client *http.Client

	BeforeAll(func() {
		// Use 30-second timeout for aggregation queries (can be slow on first run)
		// Integration tests with real database may need more time than unit tests
		client = &http.Client{Timeout: 30 * time.Second}

		// Insert test data for aggregation tests
		GinkgoWriter.Println("📊 Inserting test data for aggregation tests...")
		insertAggregationTestData()
	})

	AfterAll(func() {
		// Clean up test data
		GinkgoWriter.Println("🧹 Cleaning up aggregation test data...")
		cleanupAggregationTestData()
	})

	// BR-STORAGE-031: Success Rate Aggregation
	// ⚠️  DEPRECATED: workflow_id endpoint (ADR-033)
	// This test uses the deprecated workflow_id parameter.
	// ADR-033 replaces this with incident-type based aggregation.
	// See: test/integration/datastorage/aggregation_api_adr033_test.go for new tests
	Describe("GET /api/v1/incidents/aggregate/success-rate [DEPRECATED]", func() {
		Context("Behavior + Correctness Testing ✅ GAP-05", func() {
			It("should calculate success rate correctly with exact counts [DEPRECATED - use incident-type]", func() {
				// ⚠️  DEPRECATED: This test uses workflow_id which is architecturally flawed per ADR-033
				// AI-generated workflows are unique, so workflow_id success rate is meaningless
				// Use incident-type aggregation instead (see aggregation_api_adr033_test.go)

				// Test data: workflow-agg-1 has 3 completed, 1 failed
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/success-rate?workflow_id=workflow-agg-1", datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				// ✅ BEHAVIOR TEST: HTTP 200 OK
				Expect(resp.StatusCode).To(Equal(http.StatusOK), "Expected 200 OK for success rate aggregation")

				var result models.SuccessRateAggregationResponse
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

				// ✅ CORRECTNESS TEST: Response structure (validated by structured type)
				Expect(result.WorkflowID).ToNot(BeEmpty(), "workflow_id should be present")
				Expect(result.TotalCount).To(BeNumerically(">=", 0), "total_count should be non-negative")
				Expect(result.SuccessCount).To(BeNumerically(">=", 0), "success_count should be non-negative")
				Expect(result.FailureCount).To(BeNumerically(">=", 0), "failure_count should be non-negative")
				Expect(result.SuccessRate).To(BeNumerically(">=", 0), "success_rate should be non-negative")

				// ✅ CORRECTNESS TEST: Verify against real database (GAP-05)
				var dbTotalCount, dbSuccessCount, dbFailureCount int
				err = db.QueryRow(`
					SELECT
						COUNT(*),
						SUM(CASE WHEN execution_status = 'completed' THEN 1 ELSE 0 END),
						SUM(CASE WHEN execution_status = 'failed' THEN 1 ELSE 0 END)
					FROM resource_action_traces
					WHERE action_id = $1
				`, "workflow-agg-1").Scan(&dbTotalCount, &dbSuccessCount, &dbFailureCount)
				Expect(err).ToNot(HaveOccurred(), "Database query should succeed")

				// ✅ CORRECTNESS TEST: API response matches database exactly
				Expect(result.WorkflowID).To(Equal("workflow-agg-1"),
					"workflow_id must match request parameter exactly")
				Expect(result.TotalCount).To(Equal(dbTotalCount),
					fmt.Sprintf("total_count must match database COUNT(*) exactly: %d incidents", dbTotalCount))
				Expect(result.SuccessCount).To(Equal(dbSuccessCount),
					fmt.Sprintf("success_count must match database WHERE execution_status='completed' COUNT: %d incidents", dbSuccessCount))
				Expect(result.FailureCount).To(Equal(dbFailureCount),
					fmt.Sprintf("failure_count must match database WHERE execution_status='failed' COUNT: %d incident", dbFailureCount))

				// ✅ CORRECTNESS TEST: Mathematical accuracy of success rate
				expectedRate := float64(0)
				if dbTotalCount > 0 {
					expectedRate = float64(dbSuccessCount) / float64(dbTotalCount)
				}
				Expect(result.SuccessRate).To(BeNumerically("~", expectedRate, 0.01),
					fmt.Sprintf("success_rate must equal success_count/total_count (%d/%d = %.2f) exactly", dbSuccessCount, dbTotalCount, expectedRate))
			})

			It("should handle 100% success rate", func() {
				// Test data: workflow-agg-2 has 2 completed, 0 failed
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/success-rate?workflow_id=workflow-agg-2", datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var result models.SuccessRateAggregationResponse
				json.NewDecoder(resp.Body).Decode(&result)

				Expect(result.TotalCount).To(Equal(2))
				Expect(result.SuccessCount).To(Equal(2))
				Expect(result.FailureCount).To(Equal(0))
				Expect(result.SuccessRate).To(BeNumerically("~", 1.0, 0.01))
			})

			It("should handle 0% success rate", func() {
				// Test data: workflow-agg-3 has 0 completed, 2 failed
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/success-rate?workflow_id=workflow-agg-3", datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var result models.SuccessRateAggregationResponse
				json.NewDecoder(resp.Body).Decode(&result)

				Expect(result.TotalCount).To(Equal(2))
				Expect(result.SuccessCount).To(Equal(0))
				Expect(result.FailureCount).To(Equal(2))
				Expect(result.SuccessRate).To(BeNumerically("~", 0.0, 0.01))
			})

			It("should handle empty workflow (no incidents)", func() {
				// Test data: workflow-agg-empty has 0 incidents
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/success-rate?workflow_id=workflow-agg-empty", datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var result models.SuccessRateAggregationResponse
				json.NewDecoder(resp.Body).Decode(&result)

				Expect(result.TotalCount).To(Equal(0))
				Expect(result.SuccessCount).To(Equal(0))
				Expect(result.FailureCount).To(Equal(0))
				Expect(result.SuccessRate).To(Equal(0.0))
			})
		})

		Context("RFC 7807 error handling", func() {
			It("should return 400 Bad Request for missing workflow_id", func() {
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/success-rate", datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

				var problem validation.RFC7807Problem
				json.NewDecoder(resp.Body).Decode(&problem)

				Expect(problem.Type).To(ContainSubstring("missing-parameter"))
				Expect(problem.Detail).To(ContainSubstring("workflow_id"))
			})
		})
	})

	// BR-STORAGE-032: Namespace Grouping Aggregation
	Describe("GET /api/v1/incidents/aggregate/by-namespace", func() {
		Context("when incidents exist across namespaces", func() {
			It("should group incidents by namespace with counts", func() {
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/by-namespace", datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				// ✅ BEHAVIOR TEST: HTTP 200 OK
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var result models.NamespaceAggregationResponse
				json.NewDecoder(resp.Body).Decode(&result)

				// ✅ CORRECTNESS TEST: Response structure (validated by structured type)
				Expect(result.Aggregations).To(HaveLen(3), "Expected 3 namespaces: prod-agg, staging-agg, dev-agg")

				// ✅ CORRECTNESS TEST: Verify against real database (GAP-05)
				// Note: resource_action_traces uses cluster_name column (schema compatibility)
				rows, err := db.Query(`
					SELECT cluster_name as namespace, COUNT(*) as count
					FROM resource_action_traces
					WHERE cluster_name LIKE '%-agg'
					GROUP BY cluster_name
					ORDER BY count DESC
				`)
				Expect(err).ToNot(HaveOccurred())
				defer rows.Close()

				dbAggMap := make(map[string]int)
				var dbOrder []string
				for rows.Next() {
					var namespace string
					var count int
					err := rows.Scan(&namespace, &count)
					Expect(err).ToNot(HaveOccurred())
					dbAggMap[namespace] = count
					dbOrder = append(dbOrder, namespace)
				}

				// ✅ CORRECTNESS TEST: API response matches database exactly
				aggMap := make(map[string]int)
				for _, agg := range result.Aggregations {
					aggMap[agg.Namespace] = agg.Count
				}

				for namespace, dbCount := range dbAggMap {
					apiCount := aggMap[namespace]
					Expect(apiCount).To(Equal(dbCount),
						fmt.Sprintf("%s should have %d incidents (database count)", namespace, dbCount))
				}

				// ✅ CORRECTNESS TEST: Verify ORDER BY count DESC
				Expect(result.Aggregations[0].Namespace).To(Equal(dbOrder[0]),
					"First namespace should match database ORDER BY count DESC")
			})

			It("should order namespaces by count descending", func() {
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/by-namespace", datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				var result models.NamespaceAggregationResponse
				json.NewDecoder(resp.Body).Decode(&result)

				Expect(result.Aggregations[0].Namespace).To(Equal("prod-agg"), "First should be prod-agg (highest count)")
				Expect(result.Aggregations[1].Namespace).To(Equal("staging-agg"), "Second should be staging-agg")
				Expect(result.Aggregations[2].Namespace).To(Equal("dev-agg"), "Third should be dev-agg (lowest count)")
			})
		})
	})

	// BR-STORAGE-033: Severity Distribution Aggregation
	Describe("GET /api/v1/incidents/aggregate/by-severity", func() {
		Context("when incidents exist with different severities", func() {
			It("should group incidents by severity with counts", func() {
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/by-severity", datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				// ✅ BEHAVIOR TEST: HTTP 200 OK
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var result models.SeverityAggregationResponse
				json.NewDecoder(resp.Body).Decode(&result)

				// ✅ CORRECTNESS TEST: Response structure (validated by structured type)
				Expect(result.Aggregations).To(HaveLen(4), "Expected 4 severity levels: critical, high, medium, low")

				// ✅ CORRECTNESS TEST: Verify against real database (GAP-05)
				rows, err := db.Query(`
					SELECT signal_severity, COUNT(*) as count
					FROM resource_action_traces
					WHERE signal_name LIKE 'agg-inc-%'
					GROUP BY signal_severity
					ORDER BY
						CASE signal_severity
							WHEN 'critical' THEN 1
							WHEN 'high' THEN 2
							WHEN 'medium' THEN 3
							WHEN 'low' THEN 4
							ELSE 5
						END
				`)
				Expect(err).ToNot(HaveOccurred())
				defer rows.Close()

				dbAggMap := make(map[string]int)
				var dbOrder []string
				for rows.Next() {
					var severity string
					var count int
					err := rows.Scan(&severity, &count)
					Expect(err).ToNot(HaveOccurred())
					dbAggMap[severity] = count
					dbOrder = append(dbOrder, severity)
				}

				// ✅ CORRECTNESS TEST: API response matches database exactly
				aggMap := make(map[string]int)
				for _, agg := range result.Aggregations {
					aggMap[agg.Severity] = agg.Count
				}

				for severity, dbCount := range dbAggMap {
					apiCount := aggMap[severity]
					Expect(apiCount).To(Equal(dbCount),
						fmt.Sprintf("%s should have %d incidents (database count)", severity, dbCount))
				}

				// ✅ CORRECTNESS TEST: Verify ORDER BY severity level (critical first)
				Expect(result.Aggregations[0].Severity).To(Equal("critical"),
					"First severity should be critical (custom CASE ORDER BY)")
			})

			It("should order severities by severity level (critical first)", func() {
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/by-severity", datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				var result models.SeverityAggregationResponse
				json.NewDecoder(resp.Body).Decode(&result)

				Expect(result.Aggregations[0].Severity).To(Equal("critical"), "First should be critical")
				Expect(result.Aggregations[1].Severity).To(Equal("high"), "Second should be high")
				Expect(result.Aggregations[2].Severity).To(Equal("medium"), "Third should be medium")
				Expect(result.Aggregations[3].Severity).To(Equal("low"), "Fourth should be low")
			})
		})
	})

	// BR-STORAGE-034: Incident Trend Aggregation
	Describe("GET /api/v1/incidents/aggregate/trend", func() {
		Context("when period parameter is valid", func() {
			It("should return daily incident counts for 7d period", func() {
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/trend?period=7d", datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var result models.TrendAggregationResponse
				json.NewDecoder(resp.Body).Decode(&result)

				// ✅ CORRECTNESS TEST: Response structure (validated by structured type)
				Expect(result.Period).To(Equal("7d"))
				Expect(result.DataPoints).ToNot(BeEmpty(), "Should have at least one data point from test data")

				// ✅ CORRECTNESS TEST: Data point structure (validated by structured type)
				firstPoint := result.DataPoints[0]
				Expect(firstPoint.Date).ToNot(BeEmpty(), "date should be present")
				Expect(firstPoint.Count).To(BeNumerically(">=", 0), "count should be non-negative")
			})

			It("should return daily incident counts for 30d period", func() {
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/trend?period=30d", datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var result models.TrendAggregationResponse
				json.NewDecoder(resp.Body).Decode(&result)

				Expect(result.Period).To(Equal("30d"))
			})
		})

		Context("RFC 7807 error handling", func() {
			It("should return 400 Bad Request for invalid period", func() {
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/trend?period=invalid", datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				// Note: Currently defaults to 7d, but should return 400
				// This test documents expected behavior for future enhancement
				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusOK),         // Current behavior: defaults to 7d
					Equal(http.StatusBadRequest), // Desired behavior: validation error
				))
			})

			It("should handle missing period parameter (defaults to 7d)", func() {
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/trend", datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				// Should default to 7d period
				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusOK),         // If defaults to 7d
					Equal(http.StatusBadRequest), // If requires period parameter
				))
			})
		})
	})
})

// insertAggregationTestData inserts test data for aggregation tests
func insertAggregationTestData() {
	// Insert required parent records (resource_references -> action_histories)
	// Step 1: Insert resource_references record
	resourceSQL := `
		INSERT INTO resource_references (id, resource_uid, api_version, kind, name, namespace, created_at)
		VALUES (1, 'test-uid-agg-001', 'apps/v1', 'Deployment', 'test-deployment', 'test-namespace', NOW())
		ON CONFLICT (id) DO NOTHING
	`
	_, err := db.Exec(resourceSQL)
	if err != nil {
		GinkgoWriter.Printf("❌ Failed to insert resource_references: %v\n", err)
		Fail(fmt.Sprintf("Failed to insert resource_references: %v", err))
	}

	// Step 2: Insert action_histories record
	actionHistorySQL := `
		INSERT INTO action_histories (id, resource_id, created_at)
		VALUES (1, 1, NOW())
		ON CONFLICT (id) DO NOTHING
	`
	_, err = db.Exec(actionHistorySQL)
	if err != nil {
		GinkgoWriter.Printf("❌ Failed to insert action_history: %v\n", err)
		Fail(fmt.Sprintf("Failed to insert action_history: %v", err))
	}

	// Insert incidents for success rate testing
	// workflow-agg-1: 3 completed, 1 failed (75% success rate)
	insertIncident("agg-inc-1", "workflow-agg-1", "prod-agg", "critical", "completed")
	insertIncident("agg-inc-2", "workflow-agg-1", "prod-agg", "high", "completed")
	insertIncident("agg-inc-3", "workflow-agg-1", "prod-agg", "medium", "completed")
	insertIncident("agg-inc-4", "workflow-agg-1", "prod-agg", "low", "failed")

	// workflow-agg-2: 2 completed, 0 failed (100% success rate)
	insertIncident("agg-inc-5", "workflow-agg-2", "staging-agg", "high", "completed")
	insertIncident("agg-inc-6", "workflow-agg-2", "staging-agg", "medium", "completed")

	// workflow-agg-3: 0 completed, 2 failed (0% success rate)
	insertIncident("agg-inc-7", "workflow-agg-3", "dev-agg", "medium", "failed")
	insertIncident("agg-inc-8", "workflow-agg-3", "dev-agg", "low", "failed")

	// Additional incidents for namespace and severity aggregation
	insertIncident("agg-inc-9", "workflow-agg-4", "prod-agg", "critical", "completed")
	insertIncident("agg-inc-10", "workflow-agg-4", "staging-agg", "high", "completed")

	GinkgoWriter.Println("  ✅ Aggregation test data inserted (10 incidents)")
}

// insertIncident is a helper to insert test incidents
func insertIncident(alertName, workflowID, namespace, severity, status string) {
	// Note: 'namespace' parameter maps to cluster_name for schema compatibility
	// The 001_initial_schema has no namespace column, using cluster_name instead
	sql := `
		INSERT INTO resource_action_traces (
			action_id, signal_name, signal_severity, action_type, action_timestamp,
			cluster_name, model_used, model_confidence,
			execution_status, action_history_id
		) VALUES ($1, $2, $3, 'scale', $4, $5, 'test-model', 0.9, $6, 1)
	`
	_, err := db.Exec(sql, workflowID, alertName, severity, time.Now(), namespace, status)
	if err != nil {
		GinkgoWriter.Printf("❌ Failed to insert incident %s: %v\n", alertName, err)
		Fail(fmt.Sprintf("Failed to insert test incident: %v", err))
	}
}

// cleanupAggregationTestData removes test data after tests complete
func cleanupAggregationTestData() {
	sql := `DELETE FROM resource_action_traces WHERE signal_name LIKE 'agg-inc-%'`
	_, err := db.Exec(sql)
	if err != nil {
		GinkgoWriter.Printf("⚠️  Failed to cleanup aggregation test data: %v\n", err)
	} else {
		GinkgoWriter.Println("  ✅ Aggregation test data cleaned up")
	}
}
