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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Structured response types (no unstructured data - project anti-pattern)

// NotificationAuditRequest represents the request body for creating a notification audit
type NotificationAuditRequest struct {
	SignalName        string `json:"signal_name"`
	SignalFingerprint string `json:"signal_fingerprint"`
	Namespace         string `json:"namespace"`
	ActionType        string `json:"action_type"`
	ActionStatus      string `json:"action_status"`
	IncidentType      string `json:"incident_type"`
	PlaybookID        string `json:"playbook_id"`
	PlaybookVersion   string `json:"playbook_version"`
	AIExecutionMode   string `json:"ai_execution_mode"`
	ExecutedAt        string `json:"executed_at"`
}

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
		// Seed 3 pod-oom incidents: 2 successful, 1 failed (using structured types)
		now := time.Now().Format(time.RFC3339)
		incidents := []NotificationAuditRequest{
			{
				SignalName:        "pod-oom",
				SignalFingerprint: "pod-oom-e2e-001",
				Namespace:         "e2e-test",
				ActionType:        "restart-pod",
				ActionStatus:      "success",
				IncidentType:      "pod-oom",
				PlaybookID:        "playbook-restart-v1",
				PlaybookVersion:   "1.0.0",
				AIExecutionMode:   "catalog",
				ExecutedAt:        now,
			},
			{
				SignalName:        "pod-oom",
				SignalFingerprint: "pod-oom-e2e-002",
				Namespace:         "e2e-test",
				ActionType:        "restart-pod",
				ActionStatus:      "success",
				IncidentType:      "pod-oom",
				PlaybookID:        "playbook-restart-v1",
				PlaybookVersion:   "1.0.0",
				AIExecutionMode:   "catalog",
				ExecutedAt:        now,
			},
			{
				SignalName:        "pod-oom",
				SignalFingerprint: "pod-oom-e2e-003",
				Namespace:         "e2e-test",
				ActionType:        "restart-pod",
				ActionStatus:      "failure",
				IncidentType:      "pod-oom",
				PlaybookID:        "playbook-restart-v1",
				PlaybookVersion:   "1.0.0",
				AIExecutionMode:   "catalog",
				ExecutedAt:        now,
			},
		}

		for _, incident := range incidents {
			body, err := json.Marshal(incident)
			if err != nil {
				return fmt.Errorf("failed to marshal incident: %w", err)
			}

			resp, err := http.Post(
				dataStorageBaseURL+"/api/v1/audit/notifications",
				"application/json",
				bytes.NewReader(body),
			)
			if err != nil {
				return fmt.Errorf("failed to POST incident: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
				return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			}
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
