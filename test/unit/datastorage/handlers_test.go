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
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/pkg/datastorage/mocks"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// PagedResponse represents a generic paged API response
// Used for testing list endpoints with data and pagination fields
// Replaces map[string]interface{} for type safety (IMPLEMENTATION_PLAN_V4.9 #21)
type PagedResponse struct {
	Data       []interface{}          `json:"data"`
	Pagination map[string]interface{} `json:"pagination"` // Generic pagination metadata
}

// ========================================
// REST API HANDLERS (BR-STORAGE-021 to BR-STORAGE-028)
// TESTING PRINCIPLE: Behavior + Correctness (Implementation Plan V4.9)
// ========================================
var _ = Describe("REST API Handlers - BR-STORAGE-021, BR-STORAGE-024", func() {
	var (
		handler *server.Handler
		mockDB  *mocks.MockDB
		req     *http.Request
		rec     *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		mockDB = mocks.NewMockDB()
		handler = server.NewHandler(mockDB)
		rec = httptest.NewRecorder()
	})

	// BR-STORAGE-021: REST API read endpoints
	Describe("ListIncidents", func() {
		// BEHAVIOR: Handler returns HTTP 200 with paginated incident list for valid filters
		// CORRECTNESS: Response contains non-nil data array and pagination metadata
		It("should return paginated incidents with valid namespace filter", func() {
			// ARRANGE: Request with valid namespace filter
			req = httptest.NewRequest("GET", "/api/v1/incidents?namespace=prod", nil)

			// ACT: Call handler
			handler.ListIncidents(rec, req)

			// CORRECTNESS: HTTP 200 OK status
			Expect(rec.Code).To(Equal(http.StatusOK), "Handler should return 200 OK for valid request")

			// CORRECTNESS: Response is valid JSON
			var response PagedResponse
			err := json.Unmarshal(rec.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

			// CORRECTNESS: Response has data array (not nil, may be empty)
			Expect(response.Data).ToNot(BeNil(), "Response.Data should be non-nil array")

			// CORRECTNESS: Response has pagination metadata
			Expect(response.Pagination).ToNot(BeNil(), "Response.Pagination should be non-nil object")
			Expect(response.Pagination).To(HaveKey("limit"), "Pagination should include limit")
			Expect(response.Pagination).To(HaveKey("offset"), "Pagination should include offset")
		})

		It("should return empty array when no incidents found", func() {
			req = httptest.NewRequest("GET", "/api/v1/incidents?namespace=nonexistent", nil)

			handler.ListIncidents(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))

			var response PagedResponse
			err := json.Unmarshal(rec.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred())

			Expect(response.Data).To(HaveLen(0))
		})

		// BEHAVIOR: Handler returns empty array when no incidents match filter
		// CORRECTNESS: HTTP 200 OK with zero-length data array
		It("should return empty array when no incidents found", func() {
			// ARRANGE: Request with non-existent namespace
			req = httptest.NewRequest("GET", "/api/v1/incidents?namespace=nonexistent", nil)

			// ACT: Call handler
			handler.ListIncidents(rec, req)

			// CORRECTNESS: HTTP 200 OK (empty result is not an error)
			Expect(rec.Code).To(Equal(http.StatusOK), "Handler should return 200 OK even for empty results")

			// CORRECTNESS: Response is valid JSON
			var response PagedResponse
			err := json.Unmarshal(rec.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

			// CORRECTNESS: Data array is empty (not nil)
			Expect(response.Data).To(HaveLen(0), "Response.Data should be empty array for no matches")
		})

		// BEHAVIOR: Handler rejects invalid limit with RFC 7807 error
		// CORRECTNESS: HTTP 400 with complete RFC 7807 problem details
		It("should return RFC 7807 error for invalid limit parameter", func() {
			// ARRANGE: Request with invalid limit (exceeds maximum)
			req = httptest.NewRequest("GET", "/api/v1/incidents?limit=9999", nil)

			// ACT: Call handler
			handler.ListIncidents(rec, req)

			// CORRECTNESS: HTTP 400 Bad Request
			Expect(rec.Code).To(Equal(http.StatusBadRequest), "Invalid limit should return 400 Bad Request")

			// CORRECTNESS: Response is RFC 7807 problem detail
			var problemDetail validation.RFC7807Problem
			err := json.Unmarshal(rec.Body.Bytes(), &problemDetail)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid RFC 7807 JSON")

			// CORRECTNESS: RFC 7807 required fields are populated
			Expect(problemDetail.Type).ToNot(BeEmpty(), "RFC 7807 Type field is required")
			Expect(problemDetail.Type).To(ContainSubstring("invalid-limit"), "Type should identify limit validation error")
			Expect(problemDetail.Title).ToNot(BeEmpty(), "RFC 7807 Title field is required")
			Expect(problemDetail.Status).To(Equal(400), "RFC 7807 Status should match HTTP status")
			Expect(problemDetail.Detail).ToNot(BeEmpty(), "RFC 7807 Detail field is required")
			Expect(problemDetail.Detail).To(ContainSubstring("limit"), "Detail should mention limit parameter")
		})

		DescribeTable("should validate query parameters",
			func(queryString string, expectedStatus int, expectedErrorType string) {
				req = httptest.NewRequest("GET", "/api/v1/incidents?"+queryString, nil)

				handler.ListIncidents(rec, req)

				Expect(rec.Code).To(Equal(expectedStatus))
				if expectedStatus != http.StatusOK {
					var problem validation.RFC7807Problem
					err := json.Unmarshal(rec.Body.Bytes(), &problem)
					Expect(err).ToNot(HaveOccurred())
					Expect(problem.Type).To(ContainSubstring(expectedErrorType))
				}
			},
			Entry("negative limit", "limit=-1", http.StatusBadRequest, "invalid-limit"),
			Entry("zero limit", "limit=0", http.StatusBadRequest, "invalid-limit"),
			Entry("limit too large", "limit=10000", http.StatusBadRequest, "invalid-limit"),
			Entry("negative offset", "offset=-1", http.StatusBadRequest, "invalid-offset"),
			Entry("invalid severity", "severity=INVALID_VALUE", http.StatusBadRequest, "invalid-severity"),
		)

		// BR-STORAGE-027: Large result sets
		It("should handle large result sets efficiently", func() {
			// Mock 10,000 records
			mockDB.SetRecordCount(10000)
			req = httptest.NewRequest("GET", "/api/v1/incidents?limit=1000", nil)

			start := time.Now()
			handler.ListIncidents(rec, req)
			duration := time.Since(start)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(duration).To(BeNumerically("<", 500*time.Millisecond)) // Performance target: < 500ms
		})
	})

	// BR-STORAGE-021: Get single incident endpoint
	Describe("GetIncident", func() {
		// BEHAVIOR: Handler returns single incident by ID
		// CORRECTNESS: HTTP 200 with complete incident details
		It("should return incident with all required fields populated", func() {
			// ARRANGE: Request for specific incident ID
			req = httptest.NewRequest("GET", "/api/v1/incidents/123", nil)

			// ACT: Call handler
			handler.GetIncident(rec, req)

			// CORRECTNESS: HTTP 200 OK
			Expect(rec.Code).To(Equal(http.StatusOK), "Handler should return 200 OK for valid ID")

			// CORRECTNESS: Response is valid JSON with audit event structure
			var response repository.AuditEvent
			err := json.Unmarshal(rec.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

			// CORRECTNESS: EventID is populated (not zero UUID)
			Expect(response.EventID).ToNot(Equal(uuid.Nil), "EventID should be populated")

			// CORRECTNESS: Required fields are populated (not empty)
			Expect(response.ResourceNamespace).ToNot(BeEmpty(), "ResourceNamespace is a required field")
			Expect(response.EventAction).ToNot(BeEmpty(), "EventAction is a required field")
		})

		// BR-STORAGE-024: RFC 7807 for not found
		It("should return RFC 7807 error for non-existent incident", func() {
			req = httptest.NewRequest("GET", "/api/v1/incidents/999999", nil)

			handler.GetIncident(rec, req)

			Expect(rec.Code).To(Equal(http.StatusNotFound))

			var problemDetail validation.RFC7807Problem
			err := json.Unmarshal(rec.Body.Bytes(), &problemDetail)
			Expect(err).ToNot(HaveOccurred())

			Expect(problemDetail.Type).To(ContainSubstring("not-found"))
			Expect(problemDetail.Status).To(Equal(404))
		})

		It("should return RFC 7807 error for invalid ID format", func() {
			req = httptest.NewRequest("GET", "/api/v1/incidents/invalid-id", nil)

			handler.GetIncident(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))

			var problemDetail validation.RFC7807Problem
			err := json.Unmarshal(rec.Body.Bytes(), &problemDetail)
			Expect(err).ToNot(HaveOccurred())

			Expect(problemDetail.Type).To(ContainSubstring("invalid-id"))
		})
	})

	// BR-STORAGE-025: SQL injection protection at handler level
	Describe("security validation", func() {
		DescribeTable("should sanitize malicious input",
			func(parameter, value string) {
				// BR-STORAGE-025: URL-encode malicious input
				encodedValue := url.QueryEscape(value)
				req = httptest.NewRequest("GET", "/api/v1/incidents?"+parameter+"="+encodedValue, nil)

				handler.ListIncidents(rec, req)

				// Should not crash, should return safe response
				Expect(rec.Code).To(BeNumerically(">=", 200))
				Expect(rec.Code).To(BeNumerically("<", 500))
			},
			Entry("SQL injection in namespace", "namespace", "'; DROP TABLE resource_action_traces--"),
			Entry("SQL injection in severity", "severity", "' OR '1'='1"),
			Entry("SQL injection in cluster", "cluster", "' UNION SELECT * FROM users--"),
		)
	})
})
