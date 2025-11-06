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

package contextapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Structured response types (no unstructured data - project anti-pattern)

// SuccessRateResponse represents the response from the success rate endpoint
type SuccessRateResponse struct {
	IncidentType         string  `json:"incident_type"`
	TimeRange            string  `json:"time_range"`
	TotalExecutions      int     `json:"total_executions"`
	SuccessfulExecutions int     `json:"successful_executions"`
	FailedExecutions     int     `json:"failed_executions"`
	SuccessRate          float64 `json:"success_rate"`
	Confidence           string  `json:"confidence"`
	MinSamplesMet        bool    `json:"min_samples_met"`
}

// Test 1: End-to-End Aggregation Flow
// Validates the complete flow: PostgreSQL → Data Storage Service → Context API
//
// Flow:
// 1. Seed Data Storage with test data (via REST API)
// 2. AI Client queries Context API for incident-type success rate
// 3. Context API → Data Storage Service → PostgreSQL
// 4. Response returned to AI Client with correct aggregation
//
// Related: Day 12.2 - Test 1: E2E Aggregation Flow

var _ = Describe("E2E: Aggregation Flow", Ordered, func() {
	// Test data helpers
	var seedTestData = func() error {
		// Seed 3 pod-oom incidents: 2 successful, 1 failed
		// Use direct database inserts (matches integration test pattern)
		
		// Create parent records first (to satisfy foreign key constraints)
		// 1. Create resource_references record
		_, err := db.Exec(`
			INSERT INTO resource_references (id, resource_type, namespace, name, cluster_name)
			VALUES (999, 'Pod', 'e2e-test', 'test-pod', 'e2e-cluster')
			ON CONFLICT (id) DO NOTHING
		`)
		if err != nil {
			return fmt.Errorf("failed to insert resource_references: %w", err)
		}

		// 2. Create action_histories record
		_, err = db.Exec(`
			INSERT INTO action_histories (id, resource_id)
			VALUES (999, 999)
			ON CONFLICT (id) DO NOTHING
		`)
		if err != nil {
			return fmt.Errorf("failed to insert action_histories: %w", err)
		}

		// Insert 2 successful executions
		for i := 0; i < 2; i++ {
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
					gen_random_uuid()::text, 'restart-pod', NOW(), 'completed',
					'pod-oom', 'warning',
					'gpt-4', 0.95,
					'pod-oom', 'pod-oom-e2e', 'warning',
					'playbook-restart-v1', '1.0.0', 1, gen_random_uuid()::text,
					true, false, false
				)
			`
			_, err := db.Exec(query)
			if err != nil {
				return fmt.Errorf("failed to insert successful incident: %w", err)
			}
		}

		// Insert 1 failed execution
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
				gen_random_uuid()::text, 'restart-pod', NOW(), 'failed',
				'pod-oom', 'warning',
				'gpt-4', 0.95,
				'pod-oom', 'pod-oom-e2e', 'warning',
				'playbook-restart-v1', '1.0.0', 1, gen_random_uuid()::text,
				true, false, false
			)
		`
		_, err = db.Exec(query)
		if err != nil {
			return fmt.Errorf("failed to insert failed incident: %w", err)
		}

		// Wait for data to be persisted
		time.Sleep(500 * time.Millisecond)
		return nil
	}

	BeforeEach(func() {
		// Seed test data before each test
		err := seedTestData()
		Expect(err).ToNot(HaveOccurred(), "Test data seeding should succeed")
	})

	It("should complete end-to-end aggregation flow", func() {
		// BEHAVIOR: AI client queries Context API for incident-type success rate
		// CORRECTNESS: Returns accurate aggregation from Data Storage → PostgreSQL

		url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom", contextAPIBaseURL)
		resp, err := http.Get(url)
		Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
		defer resp.Body.Close()

		// BEHAVIOR: Returns HTTP 200 OK
		Expect(resp.StatusCode).To(Equal(http.StatusOK), "E2E flow should succeed")

		// CORRECTNESS: Response contains accurate aggregation (using structured type)
		var result SuccessRateResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

		// Validate specific business values (not null testing)
		Expect(result.IncidentType).To(Equal("pod-oom"), "Incident type should match query")
		Expect(result.TotalExecutions).To(BeNumerically(">=", 3), "Should aggregate at least 3 seeded incidents")
		Expect(result.SuccessfulExecutions).To(BeNumerically(">=", 2), "Should count at least 2 successful executions")

		// Success rate should be approximately 66.67% (2/3)
		Expect(result.SuccessRate).To(BeNumerically(">=", 60), "Success rate should be >= 60%")
		Expect(result.SuccessRate).To(BeNumerically("<=", 70), "Success rate should be <= 70%")

		// Confidence should be a valid level
		Expect([]string{"low", "medium", "high", "insufficient_data"}).To(ContainElement(result.Confidence),
			"Confidence should be a valid level")

		GinkgoWriter.Printf("✅ E2E Aggregation Flow: %d executions, %.2f%% success rate, %s confidence\n",
			result.TotalExecutions,
			result.SuccessRate,
			result.Confidence)
	})

	It("should handle non-existent incident type gracefully", func() {
		// BEHAVIOR: Query for incident type with no data
		// CORRECTNESS: Returns 200 OK with zero values and insufficient_data confidence

		url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=nonexistent-e2e-test", contextAPIBaseURL)
		resp, err := http.Get(url)
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		// BEHAVIOR: Returns 200 OK (no data is valid state)
		Expect(resp.StatusCode).To(Equal(http.StatusOK), "Non-existent incident type should return 200 OK")

		// CORRECTNESS: Response indicates no data (using structured type)
		var result SuccessRateResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		Expect(err).ToNot(HaveOccurred())

		Expect(result.TotalExecutions).To(Equal(0), "Total executions should be 0")
		Expect(result.SuccessRate).To(Equal(0.0), "Success rate should be 0")
		Expect(result.Confidence).To(Equal("insufficient_data"), "Confidence should be insufficient_data")
	})

	It("should validate all 4 services are working together", func() {
		// BEHAVIOR: Verify each service in the chain is operational
		// CORRECTNESS: All services respond with healthy status

		// 1. PostgreSQL (via Data Storage health check)
		resp, err := http.Get(dataStorageBaseURL + "/health")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK), "Data Storage Service should be healthy")
		resp.Body.Close()

		// 2. Data Storage Service (direct check)
		resp, err = http.Get(dataStorageBaseURL + "/api/v1/notification-audit?limit=1")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK), "Data Storage API should be accessible")
		resp.Body.Close()

		// 3. Context API (health check)
		resp, err = http.Get(contextAPIBaseURL + "/health")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK), "Context API should be healthy")
		resp.Body.Close()

		// 4. Context API (aggregation endpoint)
		url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom", contextAPIBaseURL)
		resp, err = http.Get(url)
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK), "Context API aggregation should work")
		resp.Body.Close()

		GinkgoWriter.Println("✅ All 4 services operational: PostgreSQL → Data Storage → Context API")
	})
})
