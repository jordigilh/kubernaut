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
	"encoding/json"
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// ADR-033 HTTP API INTEGRATION TESTS - TDD RED PHASE
// 📋 Authority: IMPLEMENTATION_PLAN_V5.3.md Day 15
// 📋 Testing Strategy: Behavior + Correctness with REAL PostgreSQL + HTTP API
// ========================================
//
// This file tests ADR-033 multi-dimensional success tracking HTTP endpoints
// against a REAL Data Storage Service (Podman container) with real PostgreSQL.
//
// INTEGRATION TEST STRATEGY:
// - Use REAL HTTP HTTPClient to call Data Storage Service API
// - Insert test data directly into PostgreSQL database
// - Execute HTTP GET requests to aggregation endpoints
// - Verify API responses against direct database queries
// - Validate Behavior (HTTP status, response structure)
// - Validate Correctness (exact counts, mathematical accuracy)
// - Clean up test data after each test
//
// Business Requirements:
// - BR-STORAGE-031-01: Incident-Type Success Rate API
// - BR-STORAGE-031-02: Workflow Success Rate API
// - BR-STORAGE-031-04: AI Execution Mode Tracking
// - BR-STORAGE-031-05: Multi-Dimensional Success Rate API
//
// TDD PHASES:
// - RED: Write tests first (this file)
// - GREEN: Verify handlers return correct data (Day 14 complete)
// - REFACTOR: Optimize queries if needed (Day 16)
//
// ========================================

var _ = Describe("ADR-033 HTTP API Integration Tests - Multi-Dimensional Success Tracking", Ordered, func() {
	var (
		adr033HistoryID int64 // Auto-generated history ID for test data
	)
	// DD-AUTH-014: HTTPClient is provided by suite setup (global authenticated client)

	BeforeAll(func() {
		// CRITICAL: API tests MUST use public schema
		// Rationale: The in-process HTTP API server (testServer) uses public schema,
		// not parallel process schemas. If tests insert data into test_process_X
		// schemas, the API won't find the data and tests will fail.
		// This is NOT a parallel execution issue - it's an API server architecture decision.

		// DD-AUTH-014: HTTPClient is now provided by suite setup with ServiceAccount auth
		// Note: HTTPClient has 10s timeout by default, sufficient for aggregation queries

		GinkgoWriter.Println("📊 ADR-033 Integration Tests: HTTP API + PostgreSQL")

		// Create parent records required for foreign key constraints
		// Step 1: Insert resource_references record (let PostgreSQL auto-generate id)
		var adr033ResourceID int64
		resourceSQL := `
			INSERT INTO resource_references (resource_uid, api_version, kind, name, namespace, created_at)
			VALUES ('test-uid-adr033-001', 'apps/v1', 'Deployment', 'test-deployment-adr033', 'test-namespace', NOW())
			RETURNING id
		`
		err := testDB.QueryRow(resourceSQL).Scan(&adr033ResourceID)
		Expect(err).ToNot(HaveOccurred())

		// Step 2: Insert action_histories record (let PostgreSQL auto-generate id)
		actionHistorySQL := `
			INSERT INTO action_histories (resource_id, created_at)
			VALUES ($1, NOW())
			RETURNING id
		`
		err = testDB.QueryRow(actionHistorySQL, adr033ResourceID).Scan(&adr033HistoryID)
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Printf("  ✅ Parent records created (resource_references id=%d, action_histories id=%d)\n", adr033ResourceID, adr033HistoryID)
	})

	BeforeEach(func() {
		// Clean up test data before each test
		cleanupADR033TestData()
	})

	AfterEach(func() {
		// Clean up test data after each test
		cleanupADR033TestData()
	})

	// ========================================
	// BR-STORAGE-031-01: Incident-Type Success Rate API
	// PRIMARY DIMENSION: Track which workflows work for specific problems
	// ========================================
	Describe("GET /api/v1/success-rate/incident-type", func() {
		Context("TC-ADR033-01: Basic incident-type success rate calculation", func() {
			It("should calculate incident-type success rate correctly with exact counts", func() {
				incidentType := "integration-test-pod-oom-killer"

				// BEHAVIOR: API calculates success rate from real database
				GinkgoWriter.Printf("  📝 Testing incident-type: %s\n", incidentType)

				// Setup: 8 successes, 2 failures = 80% success rate
				for i := 0; i < 8; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "completed", "pod-oom-recovery", "v1.0", true, false, false)
				}
				for i := 0; i < 2; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "failed", "pod-oom-recovery", "v1.0", true, false, false)
				}

				// Execute HTTP request
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					dataStorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// BEHAVIOR: HTTP 200 OK
				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"API should return 200 OK for valid incident-type request")

				// BEHAVIOR: Response is valid JSON
				var result models.IncidentTypeSuccessRateResponse
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred(),
					"Response should be valid JSON")

				// CORRECTNESS: Response structure
				Expect(result.IncidentType).To(Equal(incidentType),
					"Response should contain requested incident type")
				Expect(result.TimeRange).To(Equal("7d"),
					"Response should contain requested time range")

				// CORRECTNESS: Exact count validation (8 successes + 2 failures = 10 total)
				Expect(result.TotalExecutions).To(Equal(10),
					"Total executions should be exactly 10 (8 successes + 2 failures)")
				Expect(result.SuccessfulExecutions).To(Equal(8),
					"Successful executions should be exactly 8")
				Expect(result.FailedExecutions).To(Equal(2),
					"Failed executions should be exactly 2")

				// CORRECTNESS: Mathematical accuracy (8/10 = 80%)
				Expect(result.SuccessRate).To(BeNumerically("~", 80.0, 0.01),
					"Success rate should be 80% (8/10)")

				// CORRECTNESS: Confidence level calculation
				// Per BR-STORAGE-031-01: low (5-19), medium (20-99), high (100+)
				Expect(result.MinSamplesMet).To(BeTrue(),
					"Min samples met should be true (10 >= 5)")
				Expect(result.Confidence).To(Equal("low"),
					"Confidence should be 'low' (10 samples < 20)")

				// CORRECTNESS: Validate against direct database query
				var dbTotal, dbSuccess, dbFailed int
				err = testDB.QueryRow(`
					SELECT
						COUNT(*),
						SUM(CASE WHEN execution_status = 'completed' THEN 1 ELSE 0 END),
						SUM(CASE WHEN execution_status = 'failed' THEN 1 ELSE 0 END)
					FROM resource_action_traces
					WHERE incident_type = $1
						AND action_timestamp >= NOW() - INTERVAL '7 days'
				`, incidentType).Scan(&dbTotal, &dbSuccess, &dbFailed)
				Expect(err).ToNot(HaveOccurred(),
					"Database query should succeed")

				// CORRECTNESS: API response matches database exactly
				Expect(result.TotalExecutions).To(Equal(dbTotal),
					fmt.Sprintf("API total_executions must match database COUNT(*): %d", dbTotal))
				Expect(result.SuccessfulExecutions).To(Equal(dbSuccess),
					fmt.Sprintf("API successful_executions must match database completed count: %d", dbSuccess))
				Expect(result.FailedExecutions).To(Equal(dbFailed),
					fmt.Sprintf("API failed_executions must match database failed count: %d", dbFailed))
			})
		})

		Context("TC-ADR033-02: Confidence level calculation", func() {
			It("should return 'insufficient_data' for < 5 samples", func() {
				incidentType := "integration-test-low-sample"

				// Setup: Only 3 samples (below min_samples=5)
				for i := 0; i < 3; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "completed", "test-workflow", "v1.0", true, false, false)
				}

				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					dataStorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var result models.IncidentTypeSuccessRateResponse
				Expect(json.NewDecoder(resp.Body).Decode(&result)).ToNot(HaveOccurred())

				// CORRECTNESS: Confidence should be 'insufficient_data' (< 5 samples)
				Expect(result.TotalExecutions).To(Equal(3))
				Expect(result.MinSamplesMet).To(BeFalse(),
					"Min samples met should be false (3 < 5)")
				Expect(result.Confidence).To(Equal("insufficient_data"),
					"Confidence should be 'insufficient_data' (< 5 samples)")
			})

			It("should return 'low' confidence for 5-19 samples", func() {
				incidentType := "integration-test-low-confidence"

				// Setup: 10 samples (5-19 range = low confidence)
				for i := 0; i < 10; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "completed", "test-workflow", "v1.0", true, false, false)
				}

				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					dataStorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				var result models.IncidentTypeSuccessRateResponse
				Expect(json.NewDecoder(resp.Body).Decode(&result)).ToNot(HaveOccurred())

				// CORRECTNESS: Confidence should be 'low' (10 samples in 5-19 range)
				Expect(result.TotalExecutions).To(Equal(10))
				Expect(result.Confidence).To(Equal("low"),
					"Confidence should be 'low' (10 samples in 5-19 range)")
			})

			It("should return 'medium' confidence for 20-99 samples", func() {
				incidentType := "integration-test-medium-confidence"

				// Setup: 50 samples (20-99 range = medium confidence)
				for i := 0; i < 50; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "completed", "test-workflow", "v1.0", true, false, false)
				}

				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					dataStorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				var result models.IncidentTypeSuccessRateResponse
				Expect(json.NewDecoder(resp.Body).Decode(&result)).ToNot(HaveOccurred())

				// CORRECTNESS: Confidence should be 'medium' (50 samples in 20-99 range)
				Expect(result.TotalExecutions).To(Equal(50))
				Expect(result.Confidence).To(Equal("medium"),
					"Confidence should be 'medium' (50 samples in 20-99 range)")
			})

			It("should return 'high' confidence for 100+ samples", func() {
				incidentType := "integration-test-high-confidence"

				// Setup: 150 samples (100+ = high confidence)
				for i := 0; i < 150; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "completed", "test-workflow", "v1.0", true, false, false)
				}

				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					dataStorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				var result models.IncidentTypeSuccessRateResponse
				Expect(json.NewDecoder(resp.Body).Decode(&result)).ToNot(HaveOccurred())

				// CORRECTNESS: Confidence should be 'high' (150 samples >= 100)
				Expect(result.TotalExecutions).To(Equal(150))
				Expect(result.Confidence).To(Equal("high"),
					"Confidence should be 'high' (150 samples >= 100)")
			})
		})

		Context("TC-ADR033-03: Time range filtering", func() {
			It("should filter by time range correctly (7d)", func() {
				incidentType := "integration-test-time-filter"

				// Setup: Insert data at different times
				// Recent data (within 7 days)
				for i := 0; i < 5; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "completed", "test-workflow", "v1.0", true, false, false)
				}

				// Old data (8 days ago) - should be excluded
				_, err := testDB.Exec(`
					INSERT INTO resource_action_traces (
						action_history_id,
						action_id, action_type, action_timestamp, execution_status,
						signal_name, signal_severity,
						model_used, model_confidence,
						incident_type, alert_name, incident_severity,
						workflow_id, workflow_version, workflow_step_number, workflow_execution_id,
						ai_selected_workflow
					) VALUES (
						$1,
						gen_random_uuid()::text, 'increase_memory', NOW() - INTERVAL '8 days', 'completed',
						'TestSignal', 'warning',
						'gpt-4', 0.95,
						$2, 'TestAlert', 'warning',
						'test-workflow', 'v1.0', 1, gen_random_uuid()::text,
						true
					)
				`, adr033HistoryID, incidentType)
				Expect(err).ToNot(HaveOccurred())

				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=1",
					dataStorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				var result models.IncidentTypeSuccessRateResponse
				_ = json.NewDecoder(resp.Body).Decode(&result)

				// CORRECTNESS: Should only count recent data (5 records, not 6)
				Expect(result.TotalExecutions).To(Equal(5),
					"Should only count data within 7 days (not 8-day-old record)")
			})
		})

		Context("TC-ADR033-04: Edge cases", func() {
			It("should handle zero data gracefully", func() {
				incidentType := "integration-test-no-data"

				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					dataStorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// BEHAVIOR: Should return 200 OK even with no data
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var result models.IncidentTypeSuccessRateResponse
				_ = json.NewDecoder(resp.Body).Decode(&result)

				// CORRECTNESS: Zero values for no data
				Expect(result.TotalExecutions).To(Equal(0))
				Expect(result.SuccessfulExecutions).To(Equal(0))
				Expect(result.FailedExecutions).To(Equal(0))
				Expect(result.SuccessRate).To(Equal(0.0))
				Expect(result.Confidence).To(Equal("insufficient_data"))
			})

			It("should handle 100% success rate", func() {
				incidentType := "integration-test-perfect-success"

				// Setup: All successes
				for i := 0; i < 10; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "completed", "test-workflow", "v1.0", true, false, false)
				}

				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					dataStorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				var result models.IncidentTypeSuccessRateResponse
				_ = json.NewDecoder(resp.Body).Decode(&result)

				// CORRECTNESS: 100% success rate
				Expect(result.TotalExecutions).To(Equal(10))
				Expect(result.SuccessfulExecutions).To(Equal(10))
				Expect(result.FailedExecutions).To(Equal(0))
				Expect(result.SuccessRate).To(BeNumerically("~", 100.0, 0.01))
			})

			It("should handle 0% success rate (all failures)", func() {
				incidentType := "integration-test-all-failures"

				// Setup: All failures
				for i := 0; i < 10; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "failed", "test-workflow", "v1.0", true, false, false)
				}

				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					dataStorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				var result models.IncidentTypeSuccessRateResponse
				_ = json.NewDecoder(resp.Body).Decode(&result)

				// CORRECTNESS: 0% success rate
				Expect(result.TotalExecutions).To(Equal(10))
				Expect(result.SuccessfulExecutions).To(Equal(0))
				Expect(result.FailedExecutions).To(Equal(10))
				Expect(result.SuccessRate).To(BeNumerically("~", 0.0, 0.01))
			})
		})

		Context("TC-ADR033-05: Error handling", func() {
			It("should return 400 Bad Request for missing incident_type", func() {
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?time_range=7d",
					dataStorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// BEHAVIOR: Should return 400 Bad Request
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Should return 400 Bad Request for missing incident_type")
			})

			It("should return 400 Bad Request for invalid time_range", func() {
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=test&time_range=invalid",
					dataStorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// BEHAVIOR: Should return 400 Bad Request
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Should return 400 Bad Request for invalid time_range")
			})
		})
	})

	// ========================================
	// BR-STORAGE-031-02: Workflow Success Rate API
	// SECONDARY DIMENSION: Track which workflows are most effective
	// ========================================
	Describe("GET /api/v1/success-rate/workflow", func() {
		Context("TC-ADR033-06: Basic workflow success rate calculation", func() {
			It("should calculate workflow success rate correctly with exact counts", func() {
				workflowID := "integration-test-pod-restart"
				workflowVersion := "v1.2.3"

				// BEHAVIOR: API calculates workflow success rate from real database
				GinkgoWriter.Printf("  📝 Testing workflow: %s@%s\n", workflowID, workflowVersion)

				// Setup: 7 successes, 3 failures = 70% success rate
				for i := 0; i < 7; i++ {
					insertADR033ActionTrace(adr033HistoryID, "pod-crash", "completed", workflowID, workflowVersion, true, false, false)
				}
				for i := 0; i < 3; i++ {
					insertADR033ActionTrace(adr033HistoryID, "pod-crash", "failed", workflowID, workflowVersion, true, false, false)
				}

				// Execute HTTP request
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/workflow?workflow_id=%s&workflow_version=%s&time_range=7d&min_samples=5",
					dataStorageURL, workflowID, workflowVersion))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// BEHAVIOR: HTTP 200 OK
				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"API should return 200 OK for valid workflow request")

				// BEHAVIOR: Response is valid JSON
				var result models.WorkflowSuccessRateResponse
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred(),
					"Response should be valid JSON")

				// CORRECTNESS: Response structure
				Expect(result.WorkflowID).To(Equal(workflowID),
					"Response should contain requested workflow ID")
				Expect(result.WorkflowVersion).To(Equal(workflowVersion),
					"Response should contain requested workflow version")
				Expect(result.TimeRange).To(Equal("7d"),
					"Response should contain requested time range")

				// CORRECTNESS: Exact count validation (7 successes + 3 failures = 10 total)
				Expect(result.TotalExecutions).To(Equal(10),
					"Total executions should be exactly 10 (7 successes + 3 failures)")
				Expect(result.SuccessfulExecutions).To(Equal(7),
					"Successful executions should be exactly 7")
				Expect(result.FailedExecutions).To(Equal(3),
					"Failed executions should be exactly 3")

				// CORRECTNESS: Mathematical accuracy (7/10 = 70%)
				Expect(result.SuccessRate).To(BeNumerically("~", 70.0, 0.01),
					"Success rate should be 70% (7/10)")

				// CORRECTNESS: Confidence level calculation
				Expect(result.MinSamplesMet).To(BeTrue(),
					"Min samples met should be true (10 >= 5)")
				Expect(result.Confidence).To(Equal("low"),
					"Confidence should be 'low' (10 samples < 20)")
			})
		})

		Context("TC-ADR033-07: Workflow version filtering", func() {
			It("should filter by specific workflow version", func() {
				workflowID := "integration-test-version-filter"

				// Setup: Different versions with different success rates
				// v1.0: 5 successes
				for i := 0; i < 5; i++ {
					insertADR033ActionTrace(adr033HistoryID, "test-incident", "completed", workflowID, "v1.0", true, false, false)
				}
				// v2.0: 3 successes
				for i := 0; i < 3; i++ {
					insertADR033ActionTrace(adr033HistoryID, "test-incident", "completed", workflowID, "v2.0", true, false, false)
				}

				// Query for v1.0 only
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/workflow?workflow_id=%s&workflow_version=v1.0&time_range=7d&min_samples=1",
					dataStorageURL, workflowID))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				var result models.WorkflowSuccessRateResponse
				_ = json.NewDecoder(resp.Body).Decode(&result)

				// CORRECTNESS: Should only count v1.0 (5 records, not 8)
				Expect(result.TotalExecutions).To(Equal(5),
					"Should only count v1.0 executions (not v2.0)")
				Expect(result.WorkflowVersion).To(Equal("v1.0"))
			})
		})

		Context("TC-ADR033-08: Error handling", func() {
			It("should return 400 Bad Request for missing workflow_id", func() {
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/workflow?time_range=7d",
					dataStorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// BEHAVIOR: Should return 400 Bad Request
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Should return 400 Bad Request for missing workflow_id")
			})
		})
	})

	// ========================================
	// TC-ADR033-09: AI Execution Mode Distribution (ADR-033 Hybrid Model)
	// BR-STORAGE-031-10: Track AI execution mode distribution
	// ========================================
	Describe("TC-ADR033-09: AI Execution Mode Distribution", func() {
		Context("when querying incident-type with AI execution mode data", func() {
			It("should track AI execution mode distribution (90-9-1 hybrid model)", func() {
				incidentType := "integration-test-ai-mode-distribution"

				// Setup: ADR-033 Hybrid Model distribution
				// 90% catalog selections
				for i := 0; i < 90; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "completed", "catalog-workflow", "v1.0", true, false, false)
				}
				// 9% chained workflows
				for i := 0; i < 9; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "completed", "chained-workflow", "v1.0", false, true, false)
				}
				// 1% manual escalation
				insertADR033ActionTrace(adr033HistoryID, incidentType, "failed", "manual-escalation", "v1.0", false, false, true)

				// ACT: Query incident-type success rate
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=1",
					dataStorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// ASSERT: HTTP 200 OK
				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"Handler should return 200 OK for AI mode query")

				var result models.IncidentTypeSuccessRateResponse
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred(),
					"Response should be valid JSON")

				// BEHAVIOR: Returns AI execution mode statistics
				Expect(result.AIExecutionMode).ToNot(BeNil(),
					"AI execution mode should be present in response")

				// CORRECTNESS: Distribution matches hybrid model (90-9-1)
				Expect(result.AIExecutionMode.CatalogSelected).To(Equal(90),
					"Catalog selection should be 90% (90 out of 100)")
				Expect(result.AIExecutionMode.Chained).To(Equal(9),
					"Chained workflows should be 9% (9 out of 100)")
				Expect(result.AIExecutionMode.ManualEscalation).To(Equal(1),
					"Manual escalation should be 1% (1 out of 100)")

				// CORRECTNESS: Total executions
				Expect(result.TotalExecutions).To(Equal(100),
					"Total executions should be 100 (90 + 9 + 1)")

				// CORRECTNESS: Success rate (99% - only manual escalation failed)
				Expect(result.SuccessRate).To(BeNumerically("~", 99.0, 0.1),
					"Success rate should be 99% (99 completed out of 100)")
			})

			It("should handle 100% catalog selection (no chaining or escalation)", func() {
				incidentType := "integration-test-catalog-only"

				// Setup: 100% catalog selections
				for i := 0; i < 50; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "completed", "catalog-workflow", "v1.0", true, false, false)
				}

				// ACT: Query incident-type success rate
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=1",
					dataStorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				var result models.IncidentTypeSuccessRateResponse
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: 100% catalog selection
				Expect(result.AIExecutionMode.CatalogSelected).To(Equal(50),
					"All executions should be catalog selections")
				Expect(result.AIExecutionMode.Chained).To(Equal(0),
					"No chained workflows")
				Expect(result.AIExecutionMode.ManualEscalation).To(Equal(0),
					"No manual escalations")
			})

			It("should handle mixed AI execution modes with failures", func() {
				incidentType := "integration-test-mixed-ai-modes"

				// Setup: Mixed modes with some failures
				// 10 catalog selections (8 success, 2 failure)
				for i := 0; i < 8; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "completed", "catalog-workflow", "v1.0", true, false, false)
				}
				for i := 0; i < 2; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "failed", "catalog-workflow", "v1.0", true, false, false)
				}
				// 5 chained workflows (all success)
				for i := 0; i < 5; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "completed", "chained-workflow", "v1.0", false, true, false)
				}
				// 2 manual escalations (both failure)
				for i := 0; i < 2; i++ {
					insertADR033ActionTrace(adr033HistoryID, incidentType, "failed", "manual-escalation", "v1.0", false, false, true)
				}

				// ACT: Query incident-type success rate
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=1",
					dataStorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				var result models.IncidentTypeSuccessRateResponse
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: AI execution mode counts
				Expect(result.AIExecutionMode.CatalogSelected).To(Equal(10),
					"10 catalog selections (8 success + 2 failure)")
				Expect(result.AIExecutionMode.Chained).To(Equal(5),
					"5 chained workflows (all success)")
				Expect(result.AIExecutionMode.ManualEscalation).To(Equal(2),
					"2 manual escalations (both failure)")

				// CORRECTNESS: Total and success counts
				Expect(result.TotalExecutions).To(Equal(17),
					"Total: 10 + 5 + 2 = 17")
				Expect(result.SuccessfulExecutions).To(Equal(13),
					"Successful: 8 + 5 + 0 = 13")
				Expect(result.FailedExecutions).To(Equal(4),
					"Failed: 2 + 0 + 2 = 4")

				// CORRECTNESS: Success rate (13/17 ≈ 76.47%)
				Expect(result.SuccessRate).To(BeNumerically("~", 76.47, 0.1),
					"Success rate: 13/17 = 76.47%")
			})
		})
	})

	// ========================================
	// BR-STORAGE-031-05: Multi-Dimensional Success Rate API
	// TDD RED Phase: Integration tests for /api/v1/success-rate/multi-dimensional
	// ========================================
	Describe("GET /api/v1/success-rate/multi-dimensional", func() {
		BeforeEach(func() {
			cleanupADR033TestData()
		})

		AfterEach(func() {
			cleanupADR033TestData()
		})

		Context("with all three dimensions (incident_type + workflow + action_type)", func() {
			It("should return aggregated data filtered by all dimensions", func() {
				// ARRANGE: Insert test data with specific incident_type, workflow, and action_type
				// Insert 10 completed actions for specific combination
				for i := 0; i < 10; i++ {
					insertADR033ActionTrace(
						adr033HistoryID,
						"integration-test-pod-oom",
						"completed",
						"pod-oom-recovery",
						"v1.2",
						true, false, false,
					)
				}
				// Insert 2 failed actions for same combination
				for i := 0; i < 2; i++ {
					insertADR033ActionTrace(
						adr033HistoryID,
						"integration-test-pod-oom",
						"failed",
						"pod-oom-recovery",
						"v1.2",
						true, false, false,
					)
				}
				// Insert noise data with different combination (should be filtered out)
				insertADR033ActionTrace(adr033HistoryID, "integration-test-other", "completed", "other-workflow", "v2.0", true, false, false)

				// ACT: Query multi-dimensional endpoint with all 3 dimensions
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/multi-dimensional?incident_type=integration-test-pod-oom&workflow_id=pod-oom-recovery&workflow_version=v1.2&action_type=increase_memory&time_range=1h", dataStorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// ASSERT: HTTP 200 OK
				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"Multi-dimensional endpoint should return 200 OK")

				// ASSERT: Parse response
				var result models.MultiDimensionalSuccessRateResponse
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred(),
					"Response should be valid JSON")

				// CORRECTNESS: Validate dimensions echo back
				Expect(result.Dimensions.IncidentType).To(Equal("integration-test-pod-oom"))
				Expect(result.Dimensions.WorkflowID).To(Equal("pod-oom-recovery"))
				Expect(result.Dimensions.WorkflowVersion).To(Equal("v1.2"))
				Expect(result.Dimensions.ActionType).To(Equal("increase_memory"))

				// CORRECTNESS: Validate counts (10 completed + 2 failed = 12 total)
				Expect(result.TotalExecutions).To(Equal(12))
				Expect(result.SuccessfulExecutions).To(Equal(10))
				Expect(result.FailedExecutions).To(Equal(2))

				// CORRECTNESS: Validate success rate (10/12 = 83.33%)
				Expect(result.SuccessRate).To(BeNumerically("~", 83.33, 0.1),
					"Success rate should be 10/12 = 83.33%")

				// BEHAVIOR: Validate confidence (12 samples = low confidence)
				Expect(result.Confidence).To(Equal("low"))
			})
		})

		Context("with partial dimensions (incident_type + workflow only)", func() {
			It("should aggregate across all action_types for given incident and workflow", func() {
				// ARRANGE: Insert data for incident_type + workflow with MULTIPLE action_types
				// Pod OOM recovery workflow - increase_memory action
				for i := 0; i < 8; i++ {
					query := `
						INSERT INTO resource_action_traces (
							action_history_id, action_id, action_type, action_timestamp, execution_status,
							signal_name, signal_severity, model_used, model_confidence,
							incident_type, alert_name, incident_severity,
							workflow_id, workflow_version, workflow_step_number, workflow_execution_id,
							ai_selected_workflow, ai_chained_workflows, ai_manual_escalation
						) VALUES (
							$1, gen_random_uuid()::text, 'increase_memory', NOW(), 'completed',
							'TestSignal', 'warning', 'gpt-4', 0.95,
							'integration-test-pod-oom', 'TestAlert', 'warning',
							'pod-oom-recovery', 'v1.2', 1, gen_random_uuid()::text,
							true, false, false
						)
					`
					_, err := testDB.Exec(query, adr033HistoryID)
					Expect(err).ToNot(HaveOccurred())
				}
				// Pod OOM recovery workflow - restart_pod action
				for i := 0; i < 5; i++ {
					query := `
						INSERT INTO resource_action_traces (
							action_history_id, action_id, action_type, action_timestamp, execution_status,
							signal_name, signal_severity, model_used, model_confidence,
							incident_type, alert_name, incident_severity,
							workflow_id, workflow_version, workflow_step_number, workflow_execution_id,
							ai_selected_workflow, ai_chained_workflows, ai_manual_escalation
						) VALUES (
							$1, gen_random_uuid()::text, 'restart_pod', NOW(), 'completed',
							'TestSignal', 'warning', 'gpt-4', 0.95,
							'integration-test-pod-oom', 'TestAlert', 'warning',
							'pod-oom-recovery', 'v1.2', 2, gen_random_uuid()::text,
							true, false, false
						)
					`
					_, err := testDB.Exec(query, adr033HistoryID)
					Expect(err).ToNot(HaveOccurred())
				}

				// ACT: Query without action_type (should aggregate both actions)
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/multi-dimensional?incident_type=integration-test-pod-oom&workflow_id=pod-oom-recovery&workflow_version=v1.2&time_range=1h", dataStorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// ASSERT: HTTP 200 OK
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				// ASSERT: Parse response
				var result models.MultiDimensionalSuccessRateResponse
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: Total should be 8 + 5 = 13 actions across both types
				Expect(result.TotalExecutions).To(Equal(13),
					"Should aggregate across all action_types when not specified")
				Expect(result.SuccessfulExecutions).To(Equal(13))

				// CORRECTNESS: action_type dimension should be empty
				Expect(result.Dimensions.ActionType).To(BeEmpty(),
					"action_type should be empty when not filtered")
			})
		})

		Context("with single dimension (incident_type only)", func() {
			It("should aggregate across all workflows and action_types", func() {
				// ARRANGE: Insert data for single incident_type with MULTIPLE workflows
				// Workflow A
				for i := 0; i < 6; i++ {
					insertADR033ActionTrace(adr033HistoryID, "integration-test-disk-full", "completed", "disk-cleanup", "v1.0", true, false, false)
				}
				// Workflow B
				for i := 0; i < 4; i++ {
					insertADR033ActionTrace(adr033HistoryID, "integration-test-disk-full", "completed", "expand-volume", "v2.1", true, false, false)
				}

				// ACT: Query with only incident_type
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/multi-dimensional?incident_type=integration-test-disk-full&time_range=1h", dataStorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// ASSERT: HTTP 200 OK
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				// ASSERT: Parse response
				var result models.MultiDimensionalSuccessRateResponse
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: Total should be 6 + 4 = 10 across all workflows
				Expect(result.TotalExecutions).To(Equal(10),
					"Should aggregate across all workflows when not specified")

				// CORRECTNESS: workflow dimensions should be empty
				Expect(result.Dimensions.WorkflowID).To(BeEmpty())
				Expect(result.Dimensions.WorkflowVersion).To(BeEmpty())
			})
		})

		Context("validation errors", func() {
			It("should return 400 Bad Request when workflow_version without workflow_id", func() {
				// ACT: Query with invalid parameters
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/multi-dimensional?incident_type=test&workflow_version=v1.0&time_range=7d", dataStorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// ASSERT: HTTP 400 Bad Request
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Should return 400 for workflow_version without workflow_id")

				// ASSERT: RFC 7807 Problem Details
				var problem map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&problem)
				Expect(err).ToNot(HaveOccurred())
				Expect(problem["type"]).To(ContainSubstring("validation-error"))
				Expect(problem["detail"]).To(ContainSubstring("workflow_version requires workflow_id"))
			})

			It("should return 400 Bad Request when no dimensions are specified", func() {
				// ACT: Query with no dimension filters (only time_range)
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/multi-dimensional?time_range=7d", dataStorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// ASSERT: HTTP 400 Bad Request
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Should return 400 when no dimensions are specified")

				// CORRECTNESS: RFC 7807 error response
				var problem map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&problem)
				Expect(err).ToNot(HaveOccurred())
				Expect(problem["type"]).To(ContainSubstring("validation-error"))
				Expect(problem["detail"]).To(ContainSubstring("at least one dimension filter"))
			})

			It("should return 400 Bad Request for invalid time_range", func() {
				// ACT: Query with invalid time_range
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/multi-dimensional?incident_type=test&time_range=invalid", dataStorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// ASSERT: HTTP 400 Bad Request
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

				// ASSERT: Error message
				var problem map[string]interface{}
				_ = json.NewDecoder(resp.Body).Decode(&problem)
				Expect(problem["detail"]).To(ContainSubstring("time_range"))
			})
		})

		Context("defaults", func() {
			It("should default to 7d time_range when not specified", func() {
				// ARRANGE: Insert test data
				insertADR033ActionTrace(adr033HistoryID, "integration-test-defaults", "completed", "test-workflow", "v1.0", true, false, false)

				// ACT: Query without time_range
				resp, err := HTTPClient.Get(fmt.Sprintf("%s/api/v1/success-rate/multi-dimensional?incident_type=integration-test-defaults", dataStorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// ASSERT: Response shows default time_range
				var result models.MultiDimensionalSuccessRateResponse
				_ = json.NewDecoder(resp.Body).Decode(&result)
				Expect(result.TimeRange).To(Equal("7d"),
					"time_range should default to 7d")
			})
		})
	})
})

// ========================================
// HELPER FUNCTIONS FOR ADR-033 INTEGRATION TESTS
// ========================================

// insertADR033ActionTrace inserts a test action trace with ADR-033 fields
// Uses the actual schema columns from 001_initial_schema.sql + 011_rename_alert_to_signal.sql + 012_adr033_multidimensional_tracking.sql
func insertADR033ActionTrace(
	actionHistoryID int64,
	incidentType string,
	executionStatus string,
	workflowID string,
	workflowVersion string,
	aiSelectedWorkflow bool,
	aiChainedWorkflows bool,
	aiManualEscalation bool,
) {
	query := `
		INSERT INTO resource_action_traces (
			action_history_id,
			action_id, action_type, action_timestamp, execution_status,
			signal_name, signal_severity,
			model_used, model_confidence,
			incident_type, alert_name, incident_severity,
			workflow_id, workflow_version, workflow_step_number, workflow_execution_id,
		ai_selected_workflow, ai_chained_workflows, ai_manual_escalation
	) VALUES (
		$1,
		gen_random_uuid()::text, 'increase_memory', NOW(), $2,
			'TestSignal', 'warning',
			'gpt-4', 0.95,
			$3, 'TestAlert', 'warning',
			$4, $5, 1, gen_random_uuid()::text,
			$6, $7, $8
		)
	`
	_, err := testDB.Exec(query,
		actionHistoryID, executionStatus, incidentType,
		workflowID, workflowVersion,
		aiSelectedWorkflow, aiChainedWorkflows, aiManualEscalation,
	)
	Expect(err).ToNot(HaveOccurred())
}

// cleanupADR033TestData removes all test data from resource_action_traces
// Note: Does NOT delete parent records (resource_references, action_histories) created in BeforeAll
func cleanupADR033TestData() {
	_, err := testDB.Exec("DELETE FROM resource_action_traces WHERE incident_type LIKE 'integration-test-%'")
	Expect(err).ToNot(HaveOccurred())
}
