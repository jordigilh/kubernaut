package contextapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// RED PHASE: These tests define the business outcome
// DD-004: RFC 7807 Error Response Standard
// BR-CONTEXT-009: Consistent error responses across all Context API endpoints
//
// Business Requirement: ALL HTTP error responses (4xx, 5xx) MUST use RFC 7807 format
//
// Expected Behavior:
// 1. Error responses MUST have Content-Type: application/problem+json
// 2. Error responses MUST include required fields: type, title, detail, status, instance
// 3. Error type URIs MUST follow format: https://kubernaut.io/errors/{error-type}
// 4. Success responses (2xx) MUST use application/json (NOT RFC 7807)

var _ = Describe("RFC 7807 Error Response Compliance - RED PHASE", func() {
	var (
		testServer *httptest.Server
	)

	BeforeEach(func() {
		// Create test server for HTTP API testing
		testServer, _ = createTestServer()
		Expect(testServer).ToNot(BeNil())

		// Setup test data
		_, err := SetupTestData(sqlxDB, 5)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}

		// Clean up test data (Data Storage schema)
		_, err := db.ExecContext(ctx, `
			DELETE FROM resource_action_traces WHERE action_id LIKE 'test-%' OR action_id LIKE 'rr-%';
			DELETE FROM action_histories WHERE id IN (
				SELECT ah.id FROM action_histories ah
				JOIN resource_references rr ON ah.resource_id = rr.id
				WHERE rr.resource_uid LIKE 'test-uid-%'
			);
			DELETE FROM resource_references WHERE resource_uid LIKE 'test-uid-%';
		`)
		if err != nil {
			GinkgoWriter.Printf("⚠️  Test data cleanup warning: %v\n", err)
		}
	})

	Context("Business Requirement: 400 Bad Request errors", func() {
		It("MUST return RFC 7807 format for invalid limit parameter", func() {
			// Business Scenario: User provides invalid query parameter
			// Expected: RFC 7807 error response with validation-error type

			url := fmt.Sprintf("%s/api/v1/incidents?limit=200", testServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Business Outcome 1: Correct HTTP status
			Expect(resp.StatusCode).To(Equal(400))

			// Business Outcome 2: RFC 7807 Content-Type
			contentType := resp.Header.Get("Content-Type")
			Expect(contentType).To(Equal("application/problem+json"))

			// Business Outcome 3: Parse RFC 7807 structure
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			var errorResp map[string]interface{}
			err = json.Unmarshal(body, &errorResp)
			Expect(err).ToNot(HaveOccurred())

			// Business Outcome 4: Required RFC 7807 fields
			Expect(errorResp).To(HaveKey("type"))
			Expect(errorResp).To(HaveKey("title"))
			Expect(errorResp).To(HaveKey("detail"))
			Expect(errorResp).To(HaveKey("status"))
			Expect(errorResp).To(HaveKey("instance"))

			// Business Outcome 5: Error type URI format
			typeURI := errorResp["type"].(string)
			Expect(typeURI).To(Equal("https://kubernaut.io/errors/validation-error"))

			// Business Outcome 6: Meaningful error title
			title := errorResp["title"].(string)
			Expect(title).To(Equal("Bad Request"))

			// Business Outcome 7: Specific error detail
			detail := errorResp["detail"].(string)
			Expect(detail).To(ContainSubstring("limit"))

			// Business Outcome 8: Status code in body matches header
			status := errorResp["status"].(float64)
			Expect(int(status)).To(Equal(400))

			// Business Outcome 9: Instance points to request path
			instance := errorResp["instance"].(string)
			Expect(instance).To(Equal("/api/v1/incidents"))

			// Business Outcome 10: Request ID for tracing (extension member)
			Expect(errorResp).To(HaveKey("request_id"))
			Expect(errorResp["request_id"]).ToNot(BeEmpty())
		})

		It("MUST return RFC 7807 format for invalid incident ID", func() {
			url := fmt.Sprintf("%s/api/v1/incidents/invalid", testServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(400))
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())

			Expect(errorResp["type"]).To(Equal("https://kubernaut.io/errors/validation-error"))
			Expect(errorResp["detail"]).To(ContainSubstring("Invalid incident ID"))
		})

		It("MUST return RFC 7807 format for missing workflow_id", func() {
			url := fmt.Sprintf("%s/api/v1/aggregations/success-rate", testServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(400))
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())

			Expect(errorResp["type"]).To(Equal("https://kubernaut.io/errors/validation-error"))
			Expect(errorResp["detail"]).To(ContainSubstring("workflow_id"))
		})

		It("MUST return RFC 7807 format for invalid days parameter", func() {
			url := fmt.Sprintf("%s/api/v1/aggregations/trend?days=500", testServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(400))
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())

			Expect(errorResp["type"]).To(Equal("https://kubernaut.io/errors/validation-error"))
			Expect(errorResp["detail"]).To(ContainSubstring("days"))
		})
	})

	Context("Business Requirement: 404 Not Found errors", func() {
		It("MUST return RFC 7807 format for non-existent incident", func() {
			url := fmt.Sprintf("%s/api/v1/incidents/99999", testServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(404))
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())

			Expect(errorResp["type"]).To(Equal("https://kubernaut.io/errors/not-found"))
			Expect(errorResp["title"]).To(Equal("Not Found"))
			Expect(errorResp["detail"]).To(ContainSubstring("not found"))
			Expect(errorResp["status"]).To(Equal(float64(404)))
			Expect(errorResp["instance"]).To(ContainSubstring("/incidents/99999"))
		})
	})

	Context("Business Requirement: Success responses MUST NOT use RFC 7807", func() {
		It("MUST use application/json for successful responses (NOT RFC 7807)", func() {
			// Business Scenario: User makes valid request
			// Expected: Normal JSON response (NOT RFC 7807 format)

			// Test data already inserted in BeforeEach

			url := fmt.Sprintf("%s/api/v1/incidents?limit=10", testServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			// Business Outcome: Success uses application/json (NOT application/problem+json)
			contentType := resp.Header.Get("Content-Type")
			Expect(contentType).To(Equal("application/json"))

			// Parse as normal JSON
			var successResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&successResp)
			Expect(err).ToNot(HaveOccurred())

			// Business Outcome: Should NOT have RFC 7807 fields
			_, hasType := successResp["type"]
			_, hasTitle := successResp["title"]
			Expect(hasType).To(BeFalse(), "success response must not have 'type' field")
			Expect(hasTitle).To(BeFalse(), "success response must not have 'title' field")

			// Business Outcome: Should have normal success fields
			Expect(successResp).To(HaveKey("incidents"))
			Expect(successResp).To(HaveKey("total"))
		})
	})

	Context("Business Requirement: Error type URI format", func() {
		It("MUST follow https://kubernaut.io/errors/{error-type} format", func() {
			url := fmt.Sprintf("%s/api/v1/incidents?limit=200", testServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())

			// Business Outcome: URI format validation
			typeURI := errorResp["type"].(string)
			Expect(typeURI).To(HavePrefix("https://kubernaut.io/errors/"))
			Expect(typeURI).To(MatchRegexp(`^https://kubernaut\.io/errors/[a-z-]+$`))
		})
	})

	Context("Business Requirement: Request ID tracing", func() {
		It("MUST include request_id extension member for tracing", func() {
			url := fmt.Sprintf("%s/api/v1/incidents?limit=200", testServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())

			// Business Outcome: Tracing capability via request ID
			Expect(errorResp).To(HaveKey("request_id"))
			requestID := errorResp["request_id"]
			Expect(requestID).ToNot(BeNil())
			Expect(requestID).ToNot(BeEmpty())
		})
	})

	Context("Business Requirement: Instance field accuracy", func() {
		It("MUST set instance to the specific request path", func() {
			url := fmt.Sprintf("%s/api/v1/incidents/invalid", testServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())

			// Business Outcome: Instance identifies specific occurrence
			instance := errorResp["instance"].(string)
			Expect(instance).To(Equal("/api/v1/incidents/invalid"))
		})
	})
})
