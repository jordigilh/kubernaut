package datastorage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// ADR-033 HTTP API INTEGRATION TESTS - TDD RED PHASE
// 📋 Authority: IMPLEMENTATION_PLAN_V5.0.md Day 15
// 📋 Testing Strategy: Behavior + Correctness with REAL PostgreSQL + HTTP API
// ========================================
//
// This file tests ADR-033 multi-dimensional success tracking HTTP endpoints
// against a REAL Data Storage Service (Podman container) with real PostgreSQL.
//
// INTEGRATION TEST STRATEGY:
// - Use REAL HTTP client to call Data Storage Service API
// - Insert test data directly into PostgreSQL database
// - Execute HTTP GET requests to aggregation endpoints
// - Verify API responses against direct database queries
// - Validate Behavior (HTTP status, response structure)
// - Validate Correctness (exact counts, mathematical accuracy)
// - Clean up test data after each test
//
// Business Requirements:
// - BR-STORAGE-031-01: Incident-Type Success Rate API
// - BR-STORAGE-031-02: Playbook Success Rate API
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
		client *http.Client
	)

	BeforeAll(func() {
		// Use 30-second timeout for HTTP requests
		client = &http.Client{Timeout: 30 * time.Second}

		GinkgoWriter.Println("📊 ADR-033 Integration Tests: HTTP API + PostgreSQL")

		// Create parent records required for foreign key constraints
		// Step 1: Insert resource_references record
		resourceSQL := `
			INSERT INTO resource_references (id, resource_uid, api_version, kind, name, namespace, created_at)
			VALUES (999, 'test-uid-adr033-001', 'apps/v1', 'Deployment', 'test-deployment-adr033', 'test-namespace', NOW())
			ON CONFLICT (id) DO NOTHING
		`
		_, err := db.Exec(resourceSQL)
		Expect(err).ToNot(HaveOccurred())

		// Step 2: Insert action_histories record
		actionHistorySQL := `
			INSERT INTO action_histories (id, resource_id, created_at)
			VALUES (999, 999, NOW())
			ON CONFLICT (id) DO NOTHING
		`
		_, err = db.Exec(actionHistorySQL)
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Println("  ✅ Parent records created (resource_references id=999, action_histories id=999)")
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
	// PRIMARY DIMENSION: Track which playbooks work for specific problems
	// ========================================
	Describe("GET /api/v1/success-rate/incident-type", func() {
		Context("TC-ADR033-01: Basic incident-type success rate calculation", func() {
			It("should calculate incident-type success rate correctly with exact counts", func() {
				incidentType := "integration-test-pod-oom-killer"

				// BEHAVIOR: API calculates success rate from real database
				GinkgoWriter.Printf("  📝 Testing incident-type: %s\n", incidentType)

				// Setup: 8 successes, 2 failures = 80% success rate
				for i := 0; i < 8; i++ {
					insertADR033ActionTrace(incidentType, "completed", "pod-oom-recovery", "v1.0", true, false, false)
				}
				for i := 0; i < 2; i++ {
					insertADR033ActionTrace(incidentType, "failed", "pod-oom-recovery", "v1.0", true, false, false)
				}

				// Execute HTTP request
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					datastorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

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
				err = db.QueryRow(`
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
					insertADR033ActionTrace(incidentType, "completed", "test-playbook", "v1.0", true, false, false)
				}

				resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					datastorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var result models.IncidentTypeSuccessRateResponse
				json.NewDecoder(resp.Body).Decode(&result)

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
					insertADR033ActionTrace(incidentType, "completed", "test-playbook", "v1.0", true, false, false)
				}

				resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					datastorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				var result models.IncidentTypeSuccessRateResponse
				json.NewDecoder(resp.Body).Decode(&result)

				// CORRECTNESS: Confidence should be 'low' (10 samples in 5-19 range)
				Expect(result.TotalExecutions).To(Equal(10))
				Expect(result.Confidence).To(Equal("low"),
					"Confidence should be 'low' (10 samples in 5-19 range)")
			})

			It("should return 'medium' confidence for 20-99 samples", func() {
				incidentType := "integration-test-medium-confidence"

				// Setup: 50 samples (20-99 range = medium confidence)
				for i := 0; i < 50; i++ {
					insertADR033ActionTrace(incidentType, "completed", "test-playbook", "v1.0", true, false, false)
				}

				resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					datastorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				var result models.IncidentTypeSuccessRateResponse
				json.NewDecoder(resp.Body).Decode(&result)

				// CORRECTNESS: Confidence should be 'medium' (50 samples in 20-99 range)
				Expect(result.TotalExecutions).To(Equal(50))
				Expect(result.Confidence).To(Equal("medium"),
					"Confidence should be 'medium' (50 samples in 20-99 range)")
			})

			It("should return 'high' confidence for 100+ samples", func() {
				incidentType := "integration-test-high-confidence"

				// Setup: 150 samples (100+ = high confidence)
				for i := 0; i < 150; i++ {
					insertADR033ActionTrace(incidentType, "completed", "test-playbook", "v1.0", true, false, false)
				}

				resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					datastorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				var result models.IncidentTypeSuccessRateResponse
				json.NewDecoder(resp.Body).Decode(&result)

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
					insertADR033ActionTrace(incidentType, "completed", "test-playbook", "v1.0", true, false, false)
				}

				// Old data (8 days ago) - should be excluded
				_, err := db.Exec(`
					INSERT INTO resource_action_traces (
						action_id, action_type, action_timestamp, execution_status,
						resource_type, resource_name, resource_namespace,
						model_used, model_confidence,
						incident_type, playbook_id, playbook_version,
						ai_selected_playbook
					) VALUES (
						gen_random_uuid()::text, 'increase_memory', NOW() - INTERVAL '8 days', 'completed',
						'pod', 'test-pod', 'default',
						'gpt-4', 0.95,
						$1, 'test-playbook', 'v1.0',
						true
					)
				`, incidentType)
				Expect(err).ToNot(HaveOccurred())

				resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=1",
					datastorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				var result models.IncidentTypeSuccessRateResponse
				json.NewDecoder(resp.Body).Decode(&result)

				// CORRECTNESS: Should only count recent data (5 records, not 6)
				Expect(result.TotalExecutions).To(Equal(5),
					"Should only count data within 7 days (not 8-day-old record)")
			})
		})

		Context("TC-ADR033-04: Edge cases", func() {
			It("should handle zero data gracefully", func() {
				incidentType := "integration-test-no-data"

				resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					datastorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				// BEHAVIOR: Should return 200 OK even with no data
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var result models.IncidentTypeSuccessRateResponse
				json.NewDecoder(resp.Body).Decode(&result)

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
					insertADR033ActionTrace(incidentType, "completed", "test-playbook", "v1.0", true, false, false)
				}

				resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					datastorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				var result models.IncidentTypeSuccessRateResponse
				json.NewDecoder(resp.Body).Decode(&result)

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
					insertADR033ActionTrace(incidentType, "failed", "test-playbook", "v1.0", true, false, false)
				}

				resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5",
					datastorageURL, incidentType))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				var result models.IncidentTypeSuccessRateResponse
				json.NewDecoder(resp.Body).Decode(&result)

				// CORRECTNESS: 0% success rate
				Expect(result.TotalExecutions).To(Equal(10))
				Expect(result.SuccessfulExecutions).To(Equal(0))
				Expect(result.FailedExecutions).To(Equal(10))
				Expect(result.SuccessRate).To(BeNumerically("~", 0.0, 0.01))
			})
		})

		Context("TC-ADR033-05: Error handling", func() {
			It("should return 400 Bad Request for missing incident_type", func() {
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?time_range=7d",
					datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				// BEHAVIOR: Should return 400 Bad Request
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Should return 400 Bad Request for missing incident_type")
			})

			It("should return 400 Bad Request for invalid time_range", func() {
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=test&time_range=invalid",
					datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				// BEHAVIOR: Should return 400 Bad Request
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Should return 400 Bad Request for invalid time_range")
			})
		})
	})

	// ========================================
	// BR-STORAGE-031-02: Playbook Success Rate API
	// SECONDARY DIMENSION: Track which playbooks are most effective
	// ========================================
	Describe("GET /api/v1/success-rate/playbook", func() {
		Context("TC-ADR033-06: Basic playbook success rate calculation", func() {
			It("should calculate playbook success rate correctly with exact counts", func() {
				playbookID := "integration-test-pod-restart"
				playbookVersion := "v1.2.3"

				// BEHAVIOR: API calculates playbook success rate from real database
				GinkgoWriter.Printf("  📝 Testing playbook: %s@%s\n", playbookID, playbookVersion)

				// Setup: 7 successes, 3 failures = 70% success rate
				for i := 0; i < 7; i++ {
					insertADR033ActionTrace("pod-crash", "completed", playbookID, playbookVersion, true, false, false)
				}
				for i := 0; i < 3; i++ {
					insertADR033ActionTrace("pod-crash", "failed", playbookID, playbookVersion, true, false, false)
				}

				// Execute HTTP request
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/playbook?playbook_id=%s&playbook_version=%s&time_range=7d&min_samples=5",
					datastorageURL, playbookID, playbookVersion))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				// BEHAVIOR: HTTP 200 OK
				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"API should return 200 OK for valid playbook request")

				// BEHAVIOR: Response is valid JSON
				var result models.PlaybookSuccessRateResponse
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred(),
					"Response should be valid JSON")

				// CORRECTNESS: Response structure
				Expect(result.PlaybookID).To(Equal(playbookID),
					"Response should contain requested playbook ID")
				Expect(result.PlaybookVersion).To(Equal(playbookVersion),
					"Response should contain requested playbook version")
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

		Context("TC-ADR033-07: Playbook version filtering", func() {
			It("should filter by specific playbook version", func() {
				playbookID := "integration-test-version-filter"

				// Setup: Different versions with different success rates
				// v1.0: 5 successes
				for i := 0; i < 5; i++ {
					insertADR033ActionTrace("test-incident", "completed", playbookID, "v1.0", true, false, false)
				}
				// v2.0: 3 successes
				for i := 0; i < 3; i++ {
					insertADR033ActionTrace("test-incident", "completed", playbookID, "v2.0", true, false, false)
				}

				// Query for v1.0 only
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/playbook?playbook_id=%s&playbook_version=v1.0&time_range=7d&min_samples=1",
					datastorageURL, playbookID))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				var result models.PlaybookSuccessRateResponse
				json.NewDecoder(resp.Body).Decode(&result)

				// CORRECTNESS: Should only count v1.0 (5 records, not 8)
				Expect(result.TotalExecutions).To(Equal(5),
					"Should only count v1.0 executions (not v2.0)")
				Expect(result.PlaybookVersion).To(Equal("v1.0"))
			})
		})

		Context("TC-ADR033-08: Error handling", func() {
			It("should return 400 Bad Request for missing playbook_id", func() {
				resp, err := client.Get(fmt.Sprintf("%s/api/v1/success-rate/playbook?time_range=7d",
					datastorageURL))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				// BEHAVIOR: Should return 400 Bad Request
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Should return 400 Bad Request for missing playbook_id")
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
	incidentType string,
	executionStatus string,
	playbookID string,
	playbookVersion string,
	aiSelectedPlaybook bool,
	aiChainedPlaybooks bool,
	aiManualEscalation bool,
) {
	query := `
		INSERT INTO resource_action_traces (
			action_history_id,
			action_id, action_type, action_timestamp, execution_status,
			signal_name, signal_severity,
			model_used, model_confidence,
			incident_type, alert_name, incident_severity,
			playbook_id, playbook_version, playbook_step_number, playbook_execution_id,
			ai_selected_playbook, ai_chained_playbooks, ai_manual_escalation
		) VALUES (
			999,
			gen_random_uuid()::text, 'increase_memory', NOW(), $1,
			'TestSignal', 'warning',
			'gpt-4', 0.95,
			$2, 'TestAlert', 'warning',
			$3, $4, 1, gen_random_uuid()::text,
			$5, $6, $7
		)
	`
	_, err := db.Exec(query,
		executionStatus, incidentType,
		playbookID, playbookVersion,
		aiSelectedPlaybook, aiChainedPlaybooks, aiManualEscalation,
	)
	Expect(err).ToNot(HaveOccurred())
}

// cleanupADR033TestData removes all test data from resource_action_traces
func cleanupADR033TestData() {
	_, err := db.Exec("DELETE FROM resource_action_traces WHERE incident_type LIKE 'integration-test-%'")
	Expect(err).ToNot(HaveOccurred())
}

