package contextapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/datastorage"
	"github.com/jordigilh/kubernaut/pkg/contextapi/metrics"
	"github.com/jordigilh/kubernaut/pkg/contextapi/query"
	"github.com/jordigilh/kubernaut/pkg/contextapi/server"
	dsmodels "github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// DAY 11 TDD RED: HTTP Handler Unit Tests
// BR-INTEGRATION-008, BR-INTEGRATION-009, BR-INTEGRATION-010
// ========================================
//
// **OBJECTIVE**: Write failing unit tests for HTTP aggregation handlers
//
// **TEST COVERAGE** (10 tests):
// 1. Incident-Type Endpoint (3 tests)
// 2. Playbook Endpoint (3 tests)
// 3. Multi-Dimensional Endpoint (4 tests)
//
// **EXPECTED**: ❌ All tests fail (no HTTP handlers yet)
// ========================================

var _ = Describe("Aggregation HTTP Handlers", func() {
	var (
		testServer      *server.Server
		mockDataStorage *mockDataStorageClient
		mockCache       *mockCacheManager
		logger          *zap.Logger
		httpTestServer  *httptest.Server
	)

	BeforeEach(func() {
		logger = zap.NewNop()
		mockDataStorage = &mockDataStorageClient{}
		mockCache = &mockCacheManager{
			data: make(map[string][]byte),
		}

		// Create AggregationService with mocks
		aggregationService := query.NewAggregationService(
			mockDataStorage,
			mockCache,
			logger,
		)

		// Create test server configuration
		cfg := &server.Config{
			Port:               8080,
			ReadTimeout:        30 * time.Second,
			WriteTimeout:       30 * time.Second,
			DataStorageBaseURL: "http://localhost:8085", // Mock Data Storage Service
		}

		// Create unique metrics registry for each test to avoid duplicate registration
		registry := prometheus.NewRegistry()
		metricsInstance := metrics.NewMetricsWithRegistry("contextapi", "test", registry)

		// Create test server
		var err error
		testServer, err = server.NewServerWithAggregationService(
			"localhost:6379", // Redis address
			logger,
			cfg,
			metricsInstance,    // Use unique metrics instance
			aggregationService, // Inject mock aggregation service
		)
		Expect(err).ToNot(HaveOccurred(), "Server creation should succeed")

		// Create HTTP test server
		httpTestServer = httptest.NewServer(testServer.Handler())
	})

	AfterEach(func() {
		if httpTestServer != nil {
			httpTestServer.Close()
		}
	})

	// ========================================
	// BR-INTEGRATION-008: Incident-Type Success Rate Endpoint
	// ========================================
	Context("GET /api/v1/aggregation/success-rate/incident-type", func() {
		It("should return success rate data for valid incident type", func() {
			// BEHAVIOR: Valid incident_type query returns 200 OK with success rate data
			// CORRECTNESS: Response matches Data Storage Service format

			// Setup mock response
			mockDataStorage.incidentTypeResponse = &dsmodels.IncidentTypeSuccessRateResponse{
				IncidentType:         "pod-oom",
				SuccessRate:          85.0,
				TotalExecutions:      100,
				SuccessfulExecutions: 85,
				FailedExecutions:     15,
				TimeRange:            "7d",
				Confidence:           "high",
				MinSamplesMet:        true,
			}

			// Make HTTP request
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=7d&min_samples=5", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK for valid request")

			// CORRECTNESS: Response body matches expected format
			var result dsmodels.IncidentTypeSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

			// CORRECTNESS: Specific values from mock
			Expect(result.IncidentType).To(Equal("pod-oom"), "Should return requested incident type")
			Expect(result.SuccessRate).To(Equal(85.0), "Should return exact success rate from Data Storage")
			Expect(result.TotalExecutions).To(Equal(100), "Should return exact total executions")
			Expect(result.SuccessfulExecutions).To(Equal(85), "Should return exact successful executions")
			Expect(result.FailedExecutions).To(Equal(15), "Should return exact failed executions")
			Expect(result.TimeRange).To(Equal("7d"), "Should return requested time range")
			Expect(result.Confidence).To(Equal("high"), "Should return confidence level")
		})

		It("should return 400 Bad Request when incident_type is missing", func() {
			// BEHAVIOR: Missing required parameter returns 400 Bad Request
			// CORRECTNESS: RFC 7807 error response with specific error message

			// Make HTTP request without incident_type
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?time_range=7d", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Should return 400 Bad Request for missing incident_type")

			// CORRECTNESS: RFC 7807 error response
			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")
			Expect(errorResp["type"]).To(ContainSubstring("bad-request"), "Should have RFC 7807 type field")
			Expect(errorResp["detail"]).To(ContainSubstring("incident_type"), "Error detail should mention missing parameter")
		})

		It("should return cached data on cache hit", func() {
			// BEHAVIOR: Cache hit returns data without calling Data Storage Service
			// CORRECTNESS: Response matches cached data, Data Storage Service not called

			// Setup cache with data
			cachedData := &dsmodels.IncidentTypeSuccessRateResponse{
				IncidentType:         "pod-oom",
				SuccessRate:          90.0,
				TotalExecutions:      50,
				SuccessfulExecutions: 45,
				FailedExecutions:     5,
				TimeRange:            "7d",
				Confidence:           "high",
				MinSamplesMet:        true,
			}
			cacheKey := "incident_type:pod-oom:7d:5"
			cachedBytes, _ := json.Marshal(cachedData)
			mockCache.data[cacheKey] = cachedBytes

			// Make HTTP request
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=7d&min_samples=5", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK for cached request")

			// CORRECTNESS: Response matches cached data (not mock Data Storage data)
			var result dsmodels.IncidentTypeSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.SuccessRate).To(Equal(90.0), "Should return cached success rate (90.0, not 85.0 from Data Storage)")
			Expect(result.TotalExecutions).To(Equal(50), "Should return cached total executions (50, not 100)")

			// CORRECTNESS: Data Storage Service was NOT called
			Expect(mockDataStorage.incidentTypeCalled).To(BeFalse(), "Data Storage Service should not be called on cache hit")
		})
	})

	// ========================================
	// BR-INTEGRATION-009: Playbook Success Rate Endpoint
	// ========================================
	Context("GET /api/v1/aggregation/success-rate/playbook", func() {
		It("should return playbook success rate for valid playbook_id", func() {
			// BEHAVIOR: Valid playbook_id query returns 200 OK with playbook success rate
			// CORRECTNESS: Response matches Data Storage Service format

			// Setup mock response
			mockDataStorage.playbookResponse = &dsmodels.PlaybookSuccessRateResponse{
				PlaybookID:           "restart-pod-v1",
				PlaybookVersion:      "1.0.0",
				SuccessRate:          92.0,
				TotalExecutions:      200,
				SuccessfulExecutions: 184,
				FailedExecutions:     16,
				TimeRange:            "7d",
				Confidence:           "high",
				MinSamplesMet:        true,
			}

			// Make HTTP request
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/playbook?playbook_id=restart-pod-v1&playbook_version=1.0.0&time_range=7d&min_samples=5", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK for valid playbook request")

			// CORRECTNESS: Response body matches expected format
			var result dsmodels.PlaybookSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

			// CORRECTNESS: Specific values from mock
			Expect(result.PlaybookID).To(Equal("restart-pod-v1"), "Should return requested playbook ID")
			Expect(result.PlaybookVersion).To(Equal("1.0.0"), "Should return requested playbook version")
			Expect(result.SuccessRate).To(Equal(92.0), "Should return exact success rate")
			Expect(result.TotalExecutions).To(Equal(200), "Should return exact total executions")
			Expect(result.SuccessfulExecutions).To(Equal(184), "Should return exact successful executions")
		})

		It("should return 400 Bad Request when playbook_id is missing", func() {
			// BEHAVIOR: Missing required parameter returns 400 Bad Request
			// CORRECTNESS: RFC 7807 error response

			// Make HTTP request without playbook_id
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/playbook?time_range=7d", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Should return 400 Bad Request for missing playbook_id")

			// CORRECTNESS: RFC 7807 error response
			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())
			Expect(errorResp["detail"]).To(ContainSubstring("playbook_id"), "Error detail should mention missing parameter")
		})

		It("should use default values for optional parameters", func() {
			// BEHAVIOR: Optional parameters (time_range, min_samples) use defaults if not provided
			// CORRECTNESS: Handler applies default values correctly

			// Setup mock response
			mockDataStorage.playbookResponse = &dsmodels.PlaybookSuccessRateResponse{
				PlaybookID:           "restart-pod-v1",
				PlaybookVersion:      "",
				SuccessRate:          88.0,
				TotalExecutions:      150,
				SuccessfulExecutions: 132,
				FailedExecutions:     18,
				TimeRange:            "7d", // Default
				Confidence:           "high",
				MinSamplesMet:        true,
			}

			// Make HTTP request with only required parameter
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/playbook?playbook_id=restart-pod-v1", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK with default parameters")

			// CORRECTNESS: Response uses default time_range
			var result dsmodels.PlaybookSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.TimeRange).To(Equal("7d"), "Should use default time_range of 7d")
		})
	})

	// ========================================
	// BR-INTEGRATION-010: Multi-Dimensional Success Rate Endpoint
	// ========================================
	Context("GET /api/v1/aggregation/success-rate/multi-dimensional", func() {
		It("should return multi-dimensional data for all dimensions", func() {
			// BEHAVIOR: All dimensions specified returns combined success rate data
			// CORRECTNESS: Response includes all query dimensions

			// Setup mock response
			mockDataStorage.multiDimensionalResponse = &dsmodels.MultiDimensionalSuccessRateResponse{
				Dimensions: dsmodels.QueryDimensions{
					IncidentType:    "pod-oom",
					PlaybookID:      "restart-pod-v1",
					PlaybookVersion: "1.0.0",
					ActionType:      "restart",
				},
				SuccessRate:          95.0,
				TotalExecutions:      300,
				SuccessfulExecutions: 285,
				FailedExecutions:     15,
				TimeRange:            "7d",
				Confidence:           "high",
				MinSamplesMet:        true,
			}

			// Make HTTP request with all dimensions
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/multi-dimensional?incident_type=pod-oom&playbook_id=restart-pod-v1&playbook_version=1.0.0&action_type=restart&time_range=7d&min_samples=5", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK for multi-dimensional query")

			// CORRECTNESS: Response includes all dimensions
			var result dsmodels.MultiDimensionalSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Dimensions.IncidentType).To(Equal("pod-oom"), "Should include incident_type dimension")
			Expect(result.Dimensions.PlaybookID).To(Equal("restart-pod-v1"), "Should include playbook_id dimension")
			Expect(result.Dimensions.ActionType).To(Equal("restart"), "Should include action_type dimension")
			Expect(result.SuccessRate).To(Equal(95.0), "Should return exact success rate")
		})

		It("should return data for partial dimensions", func() {
			// BEHAVIOR: Partial dimensions (e.g., only incident_type) returns filtered data
			// CORRECTNESS: Response reflects only specified dimensions

			// Setup mock response
			mockDataStorage.multiDimensionalResponse = &dsmodels.MultiDimensionalSuccessRateResponse{
				Dimensions: dsmodels.QueryDimensions{
					IncidentType:    "pod-oom",
					PlaybookID:      "",
					PlaybookVersion: "",
					ActionType:      "",
				},
				SuccessRate:          87.0,
				TotalExecutions:      120,
				SuccessfulExecutions: 104,
				FailedExecutions:     16,
				TimeRange:            "7d",
				Confidence:           "medium",
				MinSamplesMet:        true,
			}

			// Make HTTP request with only incident_type
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/multi-dimensional?incident_type=pod-oom&time_range=7d", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK for partial dimensions")

			// CORRECTNESS: Response includes only specified dimension
			var result dsmodels.MultiDimensionalSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Dimensions.IncidentType).To(Equal("pod-oom"), "Should include incident_type")
			Expect(result.Dimensions.PlaybookID).To(BeEmpty(), "Should not include playbook_id")
			Expect(result.Dimensions.ActionType).To(BeEmpty(), "Should not include action_type")
		})

		It("should return 400 Bad Request when no dimensions are specified", func() {
			// BEHAVIOR: At least one dimension is required
			// CORRECTNESS: RFC 7807 error response

			// Make HTTP request with no dimensions
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/multi-dimensional?time_range=7d", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Should return 400 Bad Request when no dimensions specified")

			// CORRECTNESS: RFC 7807 error response
			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())
			Expect(errorResp["detail"]).To(ContainSubstring("dimension"), "Error detail should mention missing dimensions")
		})

		It("should return 503 Service Unavailable when Data Storage Service times out", func() {
			// BEHAVIOR: Data Storage Service timeout returns 503 Service Unavailable
			// CORRECTNESS: RFC 7807 error response with timeout indication

			// Setup mock to simulate timeout
			mockDataStorage.simulateTimeout = true

			// Make HTTP request
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/multi-dimensional?incident_type=pod-oom&time_range=7d", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 503 Service Unavailable
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable), "Should return 503 Service Unavailable on timeout")

			// CORRECTNESS: RFC 7807 error response
			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())
			Expect(errorResp["type"]).To(ContainSubstring("service-unavailable"), "Should have RFC 7807 type field")
			Expect(errorResp["detail"]).To(Or(
				ContainSubstring("timeout"),
				ContainSubstring("deadline exceeded"),
				ContainSubstring("context"),
			), "Error detail should indicate timeout")
		})
	})
})

// ========================================
// Mock Implementations
// ========================================

// mockDataStorageClient mocks the Data Storage HTTP client
type mockDataStorageClient struct {
	incidentTypeResponse     *dsmodels.IncidentTypeSuccessRateResponse
	playbookResponse         *dsmodels.PlaybookSuccessRateResponse
	multiDimensionalResponse *dsmodels.MultiDimensionalSuccessRateResponse
	incidentTypeCalled       bool
	playbookCalled           bool
	multiDimensionalCalled   bool
	simulateTimeout          bool
}

func (m *mockDataStorageClient) GetSuccessRateByIncidentType(ctx context.Context, incidentType, timeRange string, minSamples int) (*dsmodels.IncidentTypeSuccessRateResponse, error) {
	m.incidentTypeCalled = true
	if m.simulateTimeout {
		return nil, context.DeadlineExceeded
	}
	if m.incidentTypeResponse == nil {
		return nil, fmt.Errorf("no mock response configured")
	}
	return m.incidentTypeResponse, nil
}

func (m *mockDataStorageClient) GetSuccessRateByPlaybook(ctx context.Context, playbookID, playbookVersion, timeRange string, minSamples int) (*dsmodels.PlaybookSuccessRateResponse, error) {
	m.playbookCalled = true
	if m.simulateTimeout {
		return nil, context.DeadlineExceeded
	}
	if m.playbookResponse == nil {
		return nil, fmt.Errorf("no mock response configured")
	}
	return m.playbookResponse, nil
}

func (m *mockDataStorageClient) GetSuccessRateMultiDimensional(ctx context.Context, query *datastorage.MultiDimensionalQuery) (*dsmodels.MultiDimensionalSuccessRateResponse, error) {
	m.multiDimensionalCalled = true
	if m.simulateTimeout {
		return nil, context.DeadlineExceeded
	}
	if m.multiDimensionalResponse == nil {
		return nil, fmt.Errorf("no mock response configured")
	}
	return m.multiDimensionalResponse, nil
}

// mockCacheManager mocks the cache manager
type mockCacheManager struct {
	data map[string][]byte
}

func (m *mockCacheManager) Get(ctx context.Context, key string) ([]byte, error) {
	if data, ok := m.data[key]; ok {
		return data, nil
	}
	return nil, nil
}

func (m *mockCacheManager) Set(ctx context.Context, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m.data[key] = data
	return nil
}

func (m *mockCacheManager) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockCacheManager) Clear(ctx context.Context) error {
	m.data = make(map[string][]byte)
	return nil
}

func (m *mockCacheManager) Stats() cache.Stats {
	return cache.Stats{
		HitsL1:      0,
		HitsL2:      0,
		Misses:      0,
		Sets:        0,
		Evictions:   0,
		Errors:      0,
		TotalSize:   0,
		MaxSize:     1000,
		RedisStatus: "available",
	}
}

func (m *mockCacheManager) HealthCheck(ctx context.Context) (*cache.HealthStatus, error) {
	return &cache.HealthStatus{
		Degraded: false,
		Message:  "OK",
	}, nil
}

func (m *mockCacheManager) Close() error {
	return nil
}
