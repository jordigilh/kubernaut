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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	contextapierrors "github.com/jordigilh/kubernaut/pkg/contextapi/errors"
	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
	"github.com/jordigilh/kubernaut/pkg/contextapi/query"
	dsclient "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

// mockCache is a simple in-memory cache for testing
type mockCache struct {
	data map[string][]byte
	mu   sync.RWMutex
}

func newMockCache() *mockCache {
	return &mockCache{
		data: make(map[string][]byte),
	}
}

func (m *mockCache) Get(ctx context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if data, ok := m.data[key]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("cache miss")
}

func (m *mockCache) Set(ctx context.Context, key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Serialize to JSON (matching real cache behavior)
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m.data[key] = data
	return nil
}

func (m *mockCache) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *mockCache) Close() error {
	return nil
}

func (m *mockCache) HealthCheck(ctx context.Context) (*cache.HealthStatus, error) {
	return &cache.HealthStatus{Degraded: false, Message: "mock cache"}, nil
}

func (m *mockCache) Stats() cache.Stats {
	return cache.Stats{}
}

// BR-CONTEXT-007: HTTP client for Data Storage Service REST API
// BR-CONTEXT-008: Circuit breaker (3 failures → open for 60s)
// BR-CONTEXT-009: Exponential backoff retry (3 attempts: 100ms, 200ms, 400ms)
// BR-CONTEXT-010: Graceful degradation (Data Storage down → cached data only)

// Helper to create executor with mock cache
func createTestExecutor(dsClient *dsclient.DataStorageClient) *query.CachedExecutor {
	mockCache := newMockCache()

	cfg := &query.DataStorageExecutorConfig{
		DSClient: dsClient,
		Cache:    mockCache,
	}

	executor, err := query.NewCachedExecutorWithDataStorage(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(executor).ToNot(BeNil())

	return executor
}

var _ = Describe("CachedExecutor - Data Storage Service Migration", func() {
	var (
		ctx           context.Context
		mockDataStore *httptest.Server
		dsClient      *dsclient.DataStorageClient
		executor      *query.CachedExecutor
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
						"signal_name": "HighMemoryUsage",
						"signal_severity": "critical",
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
			executor = createTestExecutor(dsClient)

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

		// 🔴 RED: P1 Field Mapping Completeness - Validate ALL 15+ fields mapped correctly
		It("should map ALL Data Storage API fields to Context API model correctly", func() {
			// Setup: Mock server with COMPLETE Data Storage API response
			mockDataStore = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				// ⭐ Complete Data Storage API response with all available fields
				_, _ = w.Write([]byte(`{
					"data": [
						{
							"id": 42,
							"signal_name": "HighCPUUsage",
							"signal_fingerprint": "fp-abc123",
							"signal_severity": "critical",
							"namespace": "production",
							"target_resource": "deployment/api-server",
							"cluster_name": "prod-us-east-1",
							"environment": "production",
							"action_type": "scale",
							"action_timestamp": "2025-11-01T15:30:00Z",
							"start_time": "2025-11-01T15:30:00Z",
							"end_time": "2025-11-01T15:35:00Z",
							"duration": 300000,
							"model_used": "gpt-4",
							"model_confidence": 0.95,
							"execution_status": "completed",
							"remediation_request_id": "req-xyz-789",
							"metadata": "{\"replicas\":5,\"reason\":\"high CPU\"}",
							"error_message": null
						}
					],
					"pagination": {"total": 1, "limit": 100, "offset": 0}
				}`))
			}))

			dsClient = dsclient.NewDataStorageClient(dsclient.Config{BaseURL: mockDataStore.URL})
			executor = createTestExecutor(dsClient)
			params := &models.ListIncidentsParams{Limit: 100}

			// Execute query
			incidents, total, err := executor.ListIncidents(ctx, params)

			// ⭐ BEHAVIOR: Query should succeed
			Expect(err).ToNot(HaveOccurred())
			Expect(incidents).To(HaveLen(1))
			Expect(total).To(Equal(1))

			// ⭐⭐ CORRECTNESS: Validate ALL fields are mapped correctly
			incident := incidents[0]

			// Primary identification fields
			Expect(incident.ID).To(Equal(int64(42)),
				"ID should be mapped from Data Storage 'id' field")
			Expect(incident.Name).To(Equal("HighCPUUsage"),
				"Name should be mapped from Data Storage 'alert_name' field")
			Expect(incident.AlertFingerprint).To(Equal("fp-abc123"),
				"AlertFingerprint should be mapped from Data Storage 'alert_fingerprint' field")
			Expect(incident.RemediationRequestID).To(Equal("req-xyz-789"),
				"RemediationRequestID should be mapped from Data Storage 'remediation_request_id' field")

			// Kubernetes context fields
			Expect(incident.Namespace).To(Equal("production"),
				"Namespace should be mapped from Data Storage 'namespace' field")
			Expect(incident.TargetResource).To(Equal("deployment/api-server"),
				"TargetResource should be mapped from Data Storage 'target_resource' field")
			Expect(incident.ClusterName).To(Equal("prod-us-east-1"),
				"ClusterName should be mapped from Data Storage 'cluster_name' field")
			Expect(incident.Environment).To(Equal("production"),
				"Environment should be mapped from Data Storage 'environment' field")

			// Status and severity fields
			Expect(incident.Severity).To(Equal("critical"),
				"Severity should be mapped from Data Storage 'alert_severity' field")
			Expect(incident.ActionType).To(Equal("scale"),
				"ActionType should be mapped from Data Storage 'action_type' field")
			Expect(incident.Status).To(Equal("completed"),
				"Status should be mapped from Data Storage 'execution_status' field")
			Expect(incident.Phase).To(Equal("completed"),
				"Phase should be derived from Data Storage 'execution_status' field")

			// Timing fields
			Expect(incident.StartTime).ToNot(BeNil(),
				"StartTime should be mapped from Data Storage 'action_timestamp' field")
			Expect(incident.EndTime).ToNot(BeNil(),
				"EndTime should be mapped from Data Storage 'end_time' field")
			Expect(incident.Duration).ToNot(BeNil(),
				"Duration should be mapped from Data Storage 'duration' field")
			Expect(*incident.Duration).To(Equal(int64(300000)),
				"Duration value should be 300000 milliseconds")

			// Metadata field
			Expect(incident.Metadata).To(Equal("{\"replicas\":5,\"reason\":\"high CPU\"}"),
				"Metadata should be mapped from Data Storage 'metadata' field")

			// Null/optional field handling
			Expect(incident.ErrorMessage).To(BeNil(),
				"ErrorMessage should be nil when Data Storage 'error_message' is null")
		})

		// 🔄 REFACTOR: Table-driven filter tests (reduces duplication from 50 lines to 30 lines)
		type filterTestCase struct {
			filterName  string
			queryParam  string
			filterValue string
			paramsFunc  func(string) *models.ListIncidentsParams
		}

		DescribeTable("Filter parameter passing to Data Storage API",
			func(tc filterTestCase) {
				mockDataStore = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					query := r.URL.Query()
					Expect(query.Get(tc.queryParam)).To(Equal(tc.filterValue),
						"should pass %s filter to Data Storage API", tc.filterName)

					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{
						"data": [],
						"pagination": {"total": 0, "limit": 100, "offset": 0}
					}`))
				}))

				dsClient = dsclient.NewDataStorageClient(dsclient.Config{BaseURL: mockDataStore.URL})
				executor = createTestExecutor(dsClient)

				params := tc.paramsFunc(tc.filterValue)
				_, _, err := executor.ListIncidents(ctx, params)
				Expect(err).ToNot(HaveOccurred())
			},
			Entry("namespace filter", filterTestCase{
				filterName:  "namespace",
				queryParam:  "namespace",
				filterValue: "production",
				paramsFunc: func(val string) *models.ListIncidentsParams {
					return &models.ListIncidentsParams{Namespace: &val, Limit: 100}
				},
			}),
			Entry("severity filter", filterTestCase{
				filterName:  "severity",
				queryParam:  "severity",
				filterValue: "critical",
				paramsFunc: func(val string) *models.ListIncidentsParams {
					return &models.ListIncidentsParams{Severity: &val, Limit: 100}
				},
			}),
		)

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
			executor = createTestExecutor(dsClient)

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

			executor = createTestExecutor(dsClient)
			params := &models.ListIncidentsParams{Limit: 100}

			// First 3 requests should hit the service (each with 3 retry attempts = 9 total HTTP calls)
			for i := 0; i < 3; i++ {
				_, _, err := executor.ListIncidents(ctx, params)
				Expect(err).To(HaveOccurred())
			}

			Expect(failureCount).To(Equal(9)) // 3 requests × 3 retry attempts each

			// 4th request should be rejected by circuit breaker
			_, _, err := executor.ListIncidents(ctx, params)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("circuit breaker open"))

			// Failure count should still be 9 (circuit breaker prevented 4th call from hitting server)
			Expect(failureCount).To(Equal(9))
		})

		It("should close circuit breaker after timeout expires and allow requests through", func() {
			// Track HTTP calls
			callCount := 0
			var callTimes []time.Time

			mockDataStore = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				callTimes = append(callTimes, time.Now())

				// Fail first 3 requests (9 calls with retries)
				if callCount <= 9 {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				// Succeed after circuit recovery
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
					"data": [],
					"pagination": {"total": 0, "limit": 100, "offset": 0}
				}`))
			}))

			// Create client with SHORT timeout for testing (2 seconds instead of 60)
			dsClient = dsclient.NewDataStorageClient(dsclient.Config{
				BaseURL: mockDataStore.URL,
				Timeout: 1 * time.Second,
			})

			// Create executor with test-specific short circuit breaker timeout
			mockCache := newMockCache()
			cfg := &query.DataStorageExecutorConfig{
				DSClient:                dsClient,
				Cache:                   mockCache,
				CircuitBreakerThreshold: 3,               // Open after 3 failures
				CircuitBreakerTimeout:   2 * time.Second, // ⭐ TEST: 2s timeout (not 60s)
			}

			executor, err := query.NewCachedExecutorWithDataStorage(cfg)
			Expect(err).ToNot(HaveOccurred())

			params := &models.ListIncidentsParams{Limit: 100}

			// Step 1: Trigger circuit breaker open (3 failures = 9 HTTP calls with retries)
			for i := 0; i < 3; i++ {
				_, _, err := executor.ListIncidents(ctx, params)
				Expect(err).To(HaveOccurred())
			}

			// ⭐ CORRECTNESS: Verify exactly 9 calls (3 requests × 3 retries)
			Expect(callCount).To(Equal(9), "should have 9 calls (3 requests × 3 retries)")

			// Step 2: Verify circuit is open (4th request rejected WITHOUT hitting server)
			_, _, err = executor.ListIncidents(ctx, params)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("circuit breaker open"))
			Expect(callCount).To(Equal(9), "circuit breaker should prevent call (still 9)")

			// Step 3: Wait for circuit breaker timeout (2 seconds)
			time.Sleep(2100 * time.Millisecond) // Slightly more than timeout

			// Step 4: ⭐⭐ CRITICAL TEST: Circuit should close (half-open), allow test request
			beforeRecoveryCount := callCount
			_, _, err = executor.ListIncidents(ctx, params)

			// ⭐⭐ CORRECTNESS: Request should succeed (circuit recovered)
			Expect(err).ToNot(HaveOccurred(), "circuit breaker should have closed after timeout")

			// ⭐⭐ CORRECTNESS: HTTP call should have gone through (call count increased)
			Expect(callCount).To(BeNumerically(">", beforeRecoveryCount),
				"circuit breaker recovery should allow HTTP call through")

			// Step 5: Verify subsequent requests continue to succeed (circuit fully closed)
			_, _, err = executor.ListIncidents(ctx, params)
			Expect(err).ToNot(HaveOccurred(), "subsequent requests should succeed after recovery")
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
			executor = createTestExecutor(dsClient)

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
			executor = createTestExecutor(dsClient)

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
			executor = createTestExecutor(dsClient)

			params := &models.ListIncidentsParams{Limit: 100}

			// First call - populates cache
			incidents1, _, err1 := executor.ListIncidents(ctx, params)
			Expect(err1).ToNot(HaveOccurred())
			Expect(incidents1).To(HaveLen(1))

			// Wait for async cache population to complete
			time.Sleep(100 * time.Millisecond)

			// Close server to simulate unavailability
			mockDataStore.Close()
			mockDataStore = nil

			// Second call - should return cached data
			incidents2, _, err2 := executor.ListIncidents(ctx, params)
			Expect(err2).ToNot(HaveOccurred())
			Expect(incidents2).To(HaveLen(1))
			Expect(incidents2[0].ID).To(Equal(int64(999)))
		})

		// 🔴 RED: P0 Cache Content Validation - Test ALL fields for correctness
		It("should return cached data with ALL fields accurate (not just ID)", func() {
			// Setup: Create mock server with comprehensive incident data
			mockDataStore = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			// ⭐ Use Data Storage API field names (will be converted by convertIncidentToModel)
			_, _ = w.Write([]byte(`{
				"data": [
					{
						"id": 42,
						"signal_name": "HighMemoryUsage",
						"signal_severity": "critical",
						"namespace": "production",
						"target_resource": "deployment/api-server-7d9f8b",
						"cluster_name": "prod-us-east-1",
						"environment": "production",
						"action_type": "scale",
						"action_timestamp": "2025-11-01T15:30:00Z",
						"model_used": "gpt-4",
						"model_confidence": 0.95,
						"execution_status": "completed",
						"error_message": null,
						"start_time": "2025-11-01T15:30:00Z",
						"signal_fingerprint": "abc123",
						"remediation_request_id": "req-xyz-789"
					}
				],
				"pagination": {"total": 1, "limit": 100, "offset": 0}
			}`))
			}))

			dsClient = dsclient.NewDataStorageClient(dsclient.Config{BaseURL: mockDataStore.URL})
			executor = createTestExecutor(dsClient)
			params := &models.ListIncidentsParams{Limit: 100}

			// Step 1: First call - populates cache from Data Storage Service
			incidents1, total1, err1 := executor.ListIncidents(ctx, params)
			Expect(err1).ToNot(HaveOccurred())
			Expect(incidents1).To(HaveLen(1))
			Expect(total1).To(Equal(1))

			// Wait for async cache population to complete
			time.Sleep(150 * time.Millisecond)

			// Step 2: Close server to force cache-only operation
			mockDataStore.Close()
			mockDataStore = nil

			// Step 3: Second call - retrieve from cache
			cachedIncidents, cachedTotal, err2 := executor.ListIncidents(ctx, params)

			// ⭐ BEHAVIOR: Cache hit should succeed
			Expect(err2).ToNot(HaveOccurred())
			Expect(cachedIncidents).To(HaveLen(1), "cache should return 1 incident")
			Expect(cachedTotal).To(Equal(1), "cache should preserve total count")

			// ⭐⭐ CORRECTNESS: Validate ALL critical fields (not just ID)
			incident := cachedIncidents[0]

			// Core identifiers
			Expect(incident.ID).To(Equal(int64(42)),
				"cache should preserve incident ID")
			Expect(incident.Name).To(Equal("HighMemoryUsage"),
				"cache should preserve alert name")
			Expect(incident.AlertFingerprint).To(Equal("abc123"),
				"cache should preserve alert fingerprint")
			Expect(incident.RemediationRequestID).To(Equal("req-xyz-789"),
				"cache should preserve remediation request ID")

			// Kubernetes context
			Expect(incident.Namespace).To(Equal("production"),
				"cache should preserve namespace")
			Expect(incident.TargetResource).To(Equal("deployment/api-server-7d9f8b"),
				"cache should preserve target resource")
			Expect(incident.ClusterName).To(Equal("prod-us-east-1"),
				"cache should preserve cluster name")
			Expect(incident.Environment).To(Equal("production"),
				"cache should preserve environment")

			// Severity and action
			Expect(incident.Severity).To(Equal("critical"),
				"cache should preserve severity")
			Expect(incident.ActionType).To(Equal("scale"),
				"cache should preserve action type")
			Expect(incident.Status).To(Equal("completed"),
				"cache should preserve status")
			Expect(incident.Phase).To(Equal("completed"),
				"cache should preserve phase")

			// Timestamps (StartTime is critical for chronological ordering)
			Expect(incident.StartTime).ToNot(BeNil(),
				"cache should preserve start time (action_timestamp)")

			// Null/optional field handling
			if incident.ErrorMessage != nil {
				Expect(*incident.ErrorMessage).To(BeEmpty(),
					"cache should preserve null error_message as nil or empty")
			}
		})

		It("should return error when cache is empty and service unavailable", func() {
			// Create client pointing to non-existent service
			dsClient = dsclient.NewDataStorageClient(dsclient.Config{
				BaseURL: "http://localhost:9999",
				Timeout: 100 * time.Millisecond,
			})

			executor = createTestExecutor(dsClient)
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
		It("should parse and propagate RFC 7807 error details with structured fields", func() {
			mockDataStore = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{
					"type": "https://api.kubernaut.io/problems/invalid-pagination",
					"title": "Invalid Pagination Parameters",
					"status": 400,
					"detail": "Limit must be between 1 and 1000",
					"instance": "/api/v1/incidents?limit=5000"
				}`))
			}))

			dsClient = dsclient.NewDataStorageClient(dsclient.Config{BaseURL: mockDataStore.URL})
			executor = createTestExecutor(dsClient)

			params := &models.ListIncidentsParams{Limit: 5000} // Invalid

			_, _, err := executor.ListIncidents(ctx, params)

			// ⭐ BEHAVIOR: Error should occur
			Expect(err).To(HaveOccurred())

			// ⭐⭐ CORRECTNESS: Error should be structured RFC 7807 error
			rfc7807Err, ok := contextapierrors.IsRFC7807Error(err)
			Expect(ok).To(BeTrue(), "error should be RFC7807Error type")
			Expect(rfc7807Err).ToNot(BeNil())

			// ⭐⭐ CORRECTNESS: Validate all RFC 7807 fields are preserved
			Expect(rfc7807Err.Type).To(Equal("https://api.kubernaut.io/problems/invalid-pagination"),
				"RFC 7807 type field should be preserved")
			Expect(rfc7807Err.Title).To(Equal("Invalid Pagination Parameters"),
				"RFC 7807 title field should be preserved")
			Expect(rfc7807Err.Detail).To(Equal("Limit must be between 1 and 1000"),
				"RFC 7807 detail field should be preserved")
			Expect(rfc7807Err.Status).To(Equal(400),
				"RFC 7807 status field should be preserved")
			Expect(rfc7807Err.Instance).To(Equal("/api/v1/incidents?limit=5000"),
				"RFC 7807 instance field should be preserved")

			// ⭐ BEHAVIOR: Error message should be readable
			Expect(err.Error()).To(ContainSubstring("Invalid Pagination Parameters"))
			Expect(err.Error()).To(ContainSubstring("Limit must be between 1 and 1000"))
		})
	})
})
