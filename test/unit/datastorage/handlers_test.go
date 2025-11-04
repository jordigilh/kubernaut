package datastorage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/mocks"
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
		It("should return incidents with valid filters", func() {
			req = httptest.NewRequest("GET", "/api/v1/incidents?namespace=prod", nil)

			handler.ListIncidents(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))

		var response PagedResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		Expect(err).ToNot(HaveOccurred())

		// Validate response structure (guaranteed by structured type)
		Expect(response.Data).ToNot(BeNil())
		Expect(response.Pagination).ToNot(BeNil())
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

		// BR-STORAGE-024: RFC 7807 error responses
		It("should return RFC 7807 error for invalid limit", func() {
			req = httptest.NewRequest("GET", "/api/v1/incidents?limit=9999", nil)

			handler.ListIncidents(rec, req)

		Expect(rec.Code).To(Equal(http.StatusBadRequest))

		var problemDetail validation.RFC7807Problem
		err := json.Unmarshal(rec.Body.Bytes(), &problemDetail)
		Expect(err).ToNot(HaveOccurred())

		// RFC 7807 required fields (guaranteed by structured type)
		Expect(problemDetail.Type).ToNot(BeEmpty())
		Expect(problemDetail.Title).ToNot(BeEmpty())
		Expect(problemDetail.Status).To(Equal(400))
		Expect(problemDetail.Detail).ToNot(BeEmpty())
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
		It("should return incident by ID", func() {
			req = httptest.NewRequest("GET", "/api/v1/incidents/123", nil)

			handler.GetIncident(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))

		// Response is a RemediationAudit - just verify it's valid JSON with expected structure
		var response struct {
			ID         int64  `json:"id"`
			Namespace  string `json:"namespace"`
			ActionType string `json:"action_type"`
		}
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		Expect(err).ToNot(HaveOccurred())

		Expect(response.ID).To(BeNumerically(">", 0))
		Expect(response.Namespace).ToNot(BeEmpty())
		Expect(response.ActionType).ToNot(BeEmpty())
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
