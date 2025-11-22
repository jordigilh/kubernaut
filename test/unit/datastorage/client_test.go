package datastorage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

// func TestDataStorageClient(t *testing.T) {
// 	RegisterFailHandler(Fail)
// 	RunSpecs(t, "...")
// }

var _ = Describe("DataStorageClient", func() {
	var (
		server   *httptest.Server
		dsClient *client.DataStorageClient
		ctx      context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Context("NewDataStorageClient", func() {
		// BEHAVIOR: Client constructor creates functional client with default configuration
		// CORRECTNESS: Client is non-nil and can make successful API calls
		It("should create functional client with default values", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data": [], "pagination": {"total": 0, "limit": 100, "offset": 0, "has_more": false}}`))
			}))

			// ARRANGE + ACT: Create client with default config
			dsClient = client.NewDataStorageClient(client.Config{
				BaseURL: server.URL,
			})

			// CORRECTNESS: Client is created successfully
			Expect(dsClient).ToNot(BeNil(), "Client should be created successfully")

			// CORRECTNESS: Client can make API calls (validate functionality)
			result, err := dsClient.ListIncidents(ctx, nil)
			Expect(err).ToNot(HaveOccurred(), "Client should successfully make API calls")
			Expect(result).ToNot(BeNil(), "API response should be non-nil")
		})

		// BEHAVIOR: Client respects custom timeout and max connections configuration
		// CORRECTNESS: Client is created with custom config and is functional
		It("should create functional client with custom timeout and max connections", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data": [], "pagination": {"total": 0, "limit": 100, "offset": 0, "has_more": false}}`))
			}))

			// ARRANGE + ACT: Create client with custom config
			dsClient = client.NewDataStorageClient(client.Config{
				BaseURL:        server.URL,
				Timeout:        10 * time.Second,
				MaxConnections: 50,
			})

			// CORRECTNESS: Client is created successfully
			Expect(dsClient).ToNot(BeNil(), "Client should be created with custom config")

			// CORRECTNESS: Client can make API calls (validate functionality)
			result, err := dsClient.ListIncidents(ctx, nil)
			Expect(err).ToNot(HaveOccurred(), "Client should successfully make API calls")
			Expect(result).ToNot(BeNil(), "API response should be non-nil")
		})
	})

	Context("ListIncidents", func() {
		// BEHAVIOR: ListIncidents makes GET request with proper headers
		// CORRECTNESS: X-Request-ID header is UUID format, User-Agent is correct
		It("should successfully list incidents with proper headers", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v1/incidents"))

				// CORRECTNESS: X-Request-ID is UUID format (not just non-empty)
				requestID := r.Header.Get("X-Request-ID")
				Expect(requestID).ToNot(BeEmpty(), "X-Request-ID should be present")
				Expect(requestID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`),
					"X-Request-ID should be valid UUID format")
				Expect(r.Header.Get("User-Agent")).To(ContainSubstring("kubernaut-context-api"))

				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
					"data": [
						{
							"id": 123,
							"alert_name": "test-alert",
							"alert_severity": "critical",
							"action_type": "scale",
							"action_timestamp": "2025-11-02T10:30:00Z",
							"model_used": "gpt-4",
							"model_confidence": 0.95,
							"execution_status": "completed"
						}
					],
					"pagination": {
						"total": 1,
						"limit": 100,
						"offset": 0,
						"has_more": false
					}
				}`))
			}))

			dsClient = client.NewDataStorageClient(client.Config{
				BaseURL: server.URL,
			})

			result, err := dsClient.ListIncidents(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Incidents).To(HaveLen(1))
			Expect(result.Total).To(Equal(1))
			Expect(result.Incidents[0].Id).To(Equal(int64(123)))
			Expect(result.Incidents[0].AlertName).To(Equal("test-alert"))
			Expect(result.Incidents[0].AlertSeverity).To(Equal(client.IncidentAlertSeverityCritical))
		})

		It("should handle RFC 7807 errors", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{
					"type": "https://kubernaut.io/errors/invalid-filter",
					"title": "Invalid Filter Parameter",
					"status": 400,
					"detail": "The 'severity' filter value must be one of: low, medium, high, critical"
				}`))
			}))

			dsClient = client.NewDataStorageClient(client.Config{
				BaseURL: server.URL,
			})

			_, err := dsClient.ListIncidents(ctx, map[string]string{"severity": "invalid"})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Invalid Filter Parameter"))
		})
	})

	Context("GetIncidentByID", func() {
		It("should successfully get incident by ID", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v1/incidents/123"))

				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
					"id": 123,
					"alert_name": "test-alert",
					"alert_severity": "critical",
					"action_type": "scale",
					"action_timestamp": "2025-11-02T10:30:00Z",
					"model_used": "gpt-4",
					"model_confidence": 0.95,
					"execution_status": "completed"
				}`))
			}))

			dsClient = client.NewDataStorageClient(client.Config{
				BaseURL: server.URL,
			})

			// ACT: Get incident by ID
			incident, err := dsClient.GetIncidentByID(ctx, 123)

			// CORRECTNESS: Request succeeds
			Expect(err).ToNot(HaveOccurred(), "GetIncidentByID should succeed")

			// CORRECTNESS: Incident is non-nil with all expected fields
			Expect(incident).ToNot(BeNil(), "Incident should be non-nil")
			Expect(incident.Id).To(Equal(int64(123)), "Incident ID should match requested ID")
			Expect(incident.AlertName).To(Equal("test-alert"), "Signal name should match response")
		})

		It("should return nil for non-existent incident", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{
					"type": "about:blank",
					"title": "Incident Not Found",
					"status": 404
				}`))
			}))

			dsClient = client.NewDataStorageClient(client.Config{
				BaseURL: server.URL,
			})

			incident, err := dsClient.GetIncidentByID(ctx, 999)

			Expect(err).ToNot(HaveOccurred())
			Expect(incident).To(BeNil())
		})
	})
})
