package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

func TestDataStorageClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Storage Client Test Suite")
}

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
		It("should create client with default values", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data": [], "pagination": {"total": 0, "limit": 100, "offset": 0, "has_more": false}}`))
			}))

			dsClient = client.NewDataStorageClient(client.Config{
				BaseURL: server.URL,
			})

			Expect(dsClient).ToNot(BeNil())
		})

		It("should use custom timeout and max connections", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data": [], "pagination": {"total": 0, "limit": 100, "offset": 0, "has_more": false}}`))
			}))

			dsClient = client.NewDataStorageClient(client.Config{
				BaseURL:        server.URL,
				Timeout:        10 * time.Second,
				MaxConnections: 50,
			})

			Expect(dsClient).ToNot(BeNil())
		})
	})

	Context("ListIncidents", func() {
		It("should successfully list incidents", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v1/incidents"))
				Expect(r.Header.Get("X-Request-ID")).ToNot(BeEmpty())
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

			incident, err := dsClient.GetIncidentByID(ctx, 123)

			Expect(err).ToNot(HaveOccurred())
			Expect(incident).ToNot(BeNil())
			Expect(incident.Id).To(Equal(int64(123)))
			Expect(incident.AlertName).To(Equal("test-alert"))
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
