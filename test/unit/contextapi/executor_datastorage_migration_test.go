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
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
	"github.com/jordigilh/kubernaut/pkg/contextapi/query"
	dsclient "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

// BR-CONTEXT-007: HTTP client for Data Storage Service REST API
// BR-CONTEXT-008: Circuit breaker (3 failures → open for 60s)
// BR-CONTEXT-009: Exponential backoff retry (3 attempts: 100ms, 200ms, 400ms)
// BR-CONTEXT-010: Graceful degradation (Data Storage down → cached data only)

var _ = Describe("CachedExecutor - Data Storage Service Migration", func() {
	var (
		ctx              context.Context
		mockDataStore    *httptest.Server
		dsClient         *dsclient.DataStorageClient
		executor         *query.CachedExecutor
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if mockDataStore != nil {
			mockDataStore.Close()
		}
	})

	// ===========================================
	// BR-CONTEXT-007: HTTP Client Integration
	// ===========================================

	Context("when querying via Data Storage Service", func() {
		It("should use Data Storage REST API instead of direct PostgreSQL", func() {
			// Mock Data Storage Service
			mockDataStore = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v1/incidents"))
				Expect(r.Method).To(Equal("GET"))

				// Verify query parameters
				query := r.URL.Query()
				Expect(query.Get("limit")).To(Equal("100"))
				Expect(query.Get("offset")).To(Equal("0"))

				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
					"data": [
						{
							"id": 1,
							"alert_name": "HighMemoryUsage",
							"alert_severity": "critical",
							"action_type": "scale",
							"action_timestamp": "2025-11-01T10:00:00Z",
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

			// Create Data Storage client
			dsClient = dsclient.NewDataStorageClient(dsclient.Config{
				BaseURL: mockDataStore.URL,
				Timeout: 5 * time.Second,
			})

			// Create executor with Data Storage client
			executor = query.NewCachedExecutorWithDataStorage(dsClient)

			// Execute query
			params := &models.ListIncidentsParams{
				Limit:  100,
				Offset: 0,
			}

			incidents, total, err := executor.ListIncidents(ctx, params)

			// BR-CONTEXT-007: Should successfully query via REST API
			Expect(err).ToNot(HaveOccurred())
			Expect(incidents).To(HaveLen(1))
			Expect(total).To(Equal(1))
			Expect(incidents[0].Name).To(Equal("HighMemoryUsage"))
		})

		It("should pass namespace filters to Data Storage API", func() {
			namespace := "production"

			mockDataStore = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query()
				Expect(query.Get("namespace")).To(Equal("production"))

				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
					"data": [],
					"pagination": {"total": 0, "limit": 100, "offset": 0, "has_more": false}
				}`))
			}))

			dsClient = dsclient.NewDataStorageClient(dsclient.Config{BaseURL: mockDataStore.URL})
			executor = query.NewCachedExecutorWithDataStorage(dsClient)

			params := &models.ListIncidentsParams{
				Namespace: &namespace,
				Limit:     100,
			}

			_, _, err := executor.ListIncidents(ctx, params)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should pass severity filters to Data Storage API", func() {
			severity := "critical"

			mockDataStore = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query()
				Expect(query.Get("severity")).To(Equal("critical"))

				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
					"data": [],
					"pagination": {"total": 0, "limit": 100, "offset": 0, "has_more": false}
				}`))
			}))

			dsClient = dsclient.NewDataStorageClient(dsclient.Config{BaseURL: mockDataStore.URL})
			executor = query.NewCachedExecutorWithDataStorage(dsClient)

			params := &models.ListIncidentsParams{
				Severity: &severity,
				Limit:    100,
			}

			_, _, err := executor.ListIncidents(ctx, params)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should get total count from API response pagination metadata", func() {
			mockDataStore = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
					"data": [
						{"id": 1, "alert_name": "test1", "alert_severity": "info", "action_type": "log", "action_timestamp": "2025-11-01T10:00:00Z", "model_used": "gpt-4", "model_confidence": 0.5, "execution_status": "completed"}
					],
					"pagination": {
						"total": 1500,
						"limit": 100,
						"offset": 0,
						"has_more": true
					}
				}`))
			}))

			dsClient = dsclient.NewDataStorageClient(dsclient.Config{BaseURL: mockDataStore.URL})
			executor = query.NewCachedExecutorWithDataStorage(dsClient)

			params := &models.ListIncidentsParams{Limit: 100}

			incidents, total, err := executor.ListIncidents(ctx, params)

			Expect(err).ToNot(HaveOccurred())
			Expect(incidents).To(HaveLen(1))
			Expect(total).To(Equal(1500)) // From pagination.total, not len(data)
		})
	})

	// ===========================================
	// BR-CONTEXT-008: Circuit Breaker
	// ===========================================

	Context("when Data Storage Service fails repeatedly", func() {
		It("should open circuit breaker after 3 consecutive failures", func() {
			failureCount := 0

			mockDataStore = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				failureCount++
				w.WriteHeader(http.StatusInternalServerError)
			}))

			dsClient = dsclient.NewDataStorageClient(dsclient.Config{
				BaseURL: mockDataStore.URL,
				Timeout: 1 * time.Second,
			})

			executor = query.NewCachedExecutorWithDataStorage(dsClient)
			params := &models.ListIncidentsParams{Limit: 100}

			// First 3 requests should hit the service
			for i := 0; i < 3; i++ {
				_, _, err := executor.ListIncidents(ctx, params)
				Expect(err).To(HaveOccurred())
			}

			Expect(failureCount).To(Equal(3))

			// 4th request should be rejected by circuit breaker
			_, _, err := executor.ListIncidents(ctx, params)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("circuit breaker open"))

			// Failure count should still be 3 (circuit breaker prevented 4th call)
			Expect(failureCount).To(Equal(3))
		})

		It("should close circuit breaker after timeout expires", func() {
			Skip("Implementation detail: circuit breaker timeout testing")
		})
	})

	// ===========================================
	// BR-CONTEXT-009: Exponential Backoff Retry
	// ===========================================

	Context("when Data Storage Service returns transient errors", func() {
		It("should retry with exponential backoff (100ms, 200ms, 400ms)", func() {
			attemptTimes := []time.Time{}

			mockDataStore = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attemptTimes = append(attemptTimes, time.Now())

				// Fail first 2 attempts, succeed on 3rd
				if len(attemptTimes) < 3 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}

				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
					"data": [],
					"pagination": {"total": 0, "limit": 100, "offset": 0, "has_more": false}
				}`))
			}))

			dsClient = dsclient.NewDataStorageClient(dsclient.Config{BaseURL: mockDataStore.URL})
			executor = query.NewCachedExecutorWithDataStorage(dsClient)

			params := &models.ListIncidentsParams{Limit: 100}

			_, _, err := executor.ListIncidents(ctx, params)
			Expect(err).ToNot(HaveOccurred())

			// Verify exponential backoff timing
			Expect(attemptTimes).To(HaveLen(3))

			delay1 := attemptTimes[1].Sub(attemptTimes[0])
			delay2 := attemptTimes[2].Sub(attemptTimes[1])

			Expect(delay1).To(BeNumerically(">=", 100*time.Millisecond))
			Expect(delay1).To(BeNumerically("<", 150*time.Millisecond))

			Expect(delay2).To(BeNumerically(">=", 200*time.Millisecond))
			Expect(delay2).To(BeNumerically("<", 250*time.Millisecond))
		})

		It("should give up after 3 retry attempts", func() {
			attemptCount := 0

			mockDataStore = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attemptCount++
				w.WriteHeader(http.StatusServiceUnavailable)
			}))

			dsClient = dsclient.NewDataStorageClient(dsclient.Config{BaseURL: mockDataStore.URL})
			executor = query.NewCachedExecutorWithDataStorage(dsClient)

			params := &models.ListIncidentsParams{Limit: 100}

			_, _, err := executor.ListIncidents(ctx, params)

			Expect(err).To(HaveOccurred())
			Expect(attemptCount).To(Equal(3)) // Initial + 2 retries = 3 total
		})
	})

	// ===========================================
	// BR-CONTEXT-010: Graceful Degradation
	// ===========================================

	Context("when Data Storage Service is completely unavailable", func() {
		It("should return cached data when service is down", func() {
			// First request succeeds and populates cache
			mockDataStore = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
					"data": [
						{"id": 999, "alert_name": "cached-alert", "alert_severity": "info", "action_type": "log", "action_timestamp": "2025-11-01T10:00:00Z", "model_used": "gpt-4", "model_confidence": 0.5, "execution_status": "completed"}
					],
					"pagination": {"total": 1, "limit": 100, "offset": 0, "has_more": false}
				}`))
			}))

			dsClient = dsclient.NewDataStorageClient(dsclient.Config{BaseURL: mockDataStore.URL})
			executor = query.NewCachedExecutorWithDataStorage(dsClient)

			params := &models.ListIncidentsParams{Limit: 100}

			// First call - populates cache
			incidents1, _, err1 := executor.ListIncidents(ctx, params)
			Expect(err1).ToNot(HaveOccurred())
			Expect(incidents1).To(HaveLen(1))

			// Close server to simulate unavailability
			mockDataStore.Close()
			mockDataStore = nil

			// Second call - should return cached data
			incidents2, _, err2 := executor.ListIncidents(ctx, params)
			Expect(err2).ToNot(HaveOccurred())
			Expect(incidents2).To(HaveLen(1))
			Expect(incidents2[0].ID).To(Equal(int64(999)))
		})

		It("should return error when cache is empty and service unavailable", func() {
			// Create client pointing to non-existent service
			dsClient = dsclient.NewDataStorageClient(dsclient.Config{
				BaseURL: "http://localhost:9999",
				Timeout: 100 * time.Millisecond,
			})

			executor = query.NewCachedExecutorWithDataStorage(dsClient)
			params := &models.ListIncidentsParams{Limit: 100}

			_, _, err := executor.ListIncidents(ctx, params)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("connection refused"),
				ContainSubstring("circuit breaker open"),
			))
		})
	})

	// ===========================================
	// RFC 7807 Error Handling
	// ===========================================

	Context("when Data Storage Service returns RFC 7807 errors", func() {
		It("should parse and propagate RFC 7807 error details", func() {
			mockDataStore = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{
					"type": "https://kubernaut.io/errors/invalid-pagination",
					"title": "Invalid Pagination Parameters",
					"status": 400,
					"detail": "Limit must be between 1 and 1000"
				}`))
			}))

			dsClient = dsclient.NewDataStorageClient(dsclient.Config{BaseURL: mockDataStore.URL})
			executor = query.NewCachedExecutorWithDataStorage(dsClient)

			params := &models.ListIncidentsParams{Limit: 5000} // Invalid

			_, _, err := executor.ListIncidents(ctx, params)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Invalid Pagination Parameters"))
		})
	})
})

