package infrastructure

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
)

// ============================================================================
// Prometheus Mock Server for Integration Tests (Tier 2)
// ============================================================================
//
// Provides a configurable httptest.NewServer that responds to Prometheus API
// endpoints with canned responses. Used in EM integration tests to simulate
// Prometheus query results without requiring a real Prometheus instance.
//
// This is a documented exception per TESTING_GUIDELINES.md v2.6.0 Section 4a:
// real Prometheus contract validation is deferred to E2E (Tier 3) tests.
//
// Each Ginkgo process gets its own mock server on an ephemeral OS port,
// so no port allocation or collision management is needed.
//
// Supported endpoints:
//   - GET /api/v1/query     (instant query)
//   - GET /api/v1/query_range (range query)
//   - GET /-/ready          (readiness check)
//   - GET /-/healthy        (health check)
//
// References:
//   - ADR-EM-001: EM integration architecture
//   - TESTING_GUIDELINES.md v2.6.0 Section 4a: Prom/AM mocking policy
// ============================================================================

// PromQueryResult represents a single result entry in a Prometheus query response.
type PromQueryResult struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value,omitempty"`  // [timestamp, value] for instant query
	Values [][]interface{}   `json:"values,omitempty"` // [[timestamp, value], ...] for range query
}

// PromQueryResponse represents the full Prometheus query API response.
type PromQueryResponse struct {
	Status string `json:"status"` // "success" or "error"
	Data   struct {
		ResultType string            `json:"resultType"` // "vector", "matrix", "scalar", "string"
		Result     []PromQueryResult `json:"result"`
	} `json:"data"`
	ErrorType string `json:"errorType,omitempty"`
	Error     string `json:"error,omitempty"`
}

// MockPrometheusConfig configures the mock Prometheus server behavior.
type MockPrometheusConfig struct {
	// QueryResponse is returned for all /api/v1/query requests.
	// If nil, an empty vector result is returned.
	QueryResponse *PromQueryResponse

	// QueryRangeResponse is returned for all /api/v1/query_range requests.
	// If nil, an empty matrix result is returned.
	QueryRangeResponse *PromQueryResponse

	// QueryHandler overrides the default handler for /api/v1/query.
	// When set, QueryResponse is ignored.
	QueryHandler http.HandlerFunc

	// QueryRangeHandler overrides the default handler for /api/v1/query_range.
	// When set, QueryRangeResponse is ignored.
	QueryRangeHandler http.HandlerFunc

	// Ready controls the response of the /-/ready endpoint.
	// When false, the endpoint returns 503.
	Ready bool

	// Healthy controls the response of the /-/healthy endpoint.
	// When false, the endpoint returns 503.
	Healthy bool
}

// MockPrometheus wraps an httptest.Server with configuration methods.
type MockPrometheus struct {
	Server *httptest.Server
	mu     sync.Mutex
	config MockPrometheusConfig

	// RequestLog tracks all requests received by the mock.
	RequestLog []MockHTTPRequest
}

// MockHTTPRequest records a received HTTP request for assertion.
type MockHTTPRequest struct {
	Method string
	Path   string
	Query  map[string][]string
}

// NewMockPrometheus creates a new mock Prometheus httptest server.
//
// Usage:
//
//	mock := NewMockPrometheus(MockPrometheusConfig{
//	    QueryResponse: &PromQueryResponse{...},
//	    Ready: true,
//	    Healthy: true,
//	})
//	defer mock.Close()
//	// Use mock.URL() as the Prometheus endpoint
func NewMockPrometheus(config MockPrometheusConfig) *MockPrometheus {
	mp := &MockPrometheus{
		config: config,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/query", func(w http.ResponseWriter, r *http.Request) {
		mp.logRequest(r)
		mp.mu.Lock()
		handler := mp.config.QueryHandler
		response := mp.config.QueryResponse
		mp.mu.Unlock()

		if handler != nil {
			handler(w, r)
			return
		}

		resp := response
		if resp == nil {
			resp = &PromQueryResponse{
				Status: "success",
			}
			resp.Data.ResultType = "vector"
			resp.Data.Result = []PromQueryResult{}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/api/v1/query_range", func(w http.ResponseWriter, r *http.Request) {
		mp.logRequest(r)
		mp.mu.Lock()
		handler := mp.config.QueryRangeHandler
		response := mp.config.QueryRangeResponse
		mp.mu.Unlock()

		if handler != nil {
			handler(w, r)
			return
		}

		resp := response
		if resp == nil {
			resp = &PromQueryResponse{
				Status: "success",
			}
			resp.Data.ResultType = "matrix"
			resp.Data.Result = []PromQueryResult{}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/-/ready", func(w http.ResponseWriter, r *http.Request) {
		mp.logRequest(r)
		mp.mu.Lock()
		ready := mp.config.Ready
		mp.mu.Unlock()

		if ready {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, "Prometheus Server is Ready.\n")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, "Prometheus Server is Not Ready.\n")
		}
	})

	mux.HandleFunc("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
		mp.logRequest(r)
		mp.mu.Lock()
		healthy := mp.config.Healthy
		mp.mu.Unlock()

		if healthy {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, "Prometheus Server is Healthy.\n")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, "Prometheus Server is Not Healthy.\n")
		}
	})

	mp.Server = httptest.NewServer(mux)
	return mp
}

// URL returns the base URL of the mock Prometheus server.
func (mp *MockPrometheus) URL() string {
	return mp.Server.URL
}

// Close shuts down the mock server.
func (mp *MockPrometheus) Close() {
	mp.Server.Close()
}

// SetQueryResponse updates the canned response for /api/v1/query at runtime.
func (mp *MockPrometheus) SetQueryResponse(resp *PromQueryResponse) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.config.QueryResponse = resp
}

// SetQueryRangeResponse updates the canned response for /api/v1/query_range at runtime.
func (mp *MockPrometheus) SetQueryRangeResponse(resp *PromQueryResponse) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.config.QueryRangeResponse = resp
}

// SetReady updates the readiness state of the mock.
func (mp *MockPrometheus) SetReady(ready bool) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.config.Ready = ready
}

// SetHealthy updates the health state of the mock.
func (mp *MockPrometheus) SetHealthy(healthy bool) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.config.Healthy = healthy
}

// GetRequestLog returns a copy of all recorded requests.
func (mp *MockPrometheus) GetRequestLog() []MockHTTPRequest {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	log := make([]MockHTTPRequest, len(mp.RequestLog))
	copy(log, mp.RequestLog)
	return log
}

// ResetRequestLog clears the request log.
func (mp *MockPrometheus) ResetRequestLog() {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.RequestLog = nil
}

func (mp *MockPrometheus) logRequest(r *http.Request) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.RequestLog = append(mp.RequestLog, MockHTTPRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.Query(),
	})
}

// ============================================================================
// Helper constructors for common Prometheus mock responses
// ============================================================================

// NewPromVectorResponse creates a Prometheus instant query response with a single vector result.
func NewPromVectorResponse(metric map[string]string, value float64, timestamp float64) *PromQueryResponse {
	resp := &PromQueryResponse{
		Status: "success",
	}
	resp.Data.ResultType = "vector"
	resp.Data.Result = []PromQueryResult{
		{
			Metric: metric,
			Value:  []interface{}{timestamp, fmt.Sprintf("%f", value)},
		},
	}
	return resp
}

// NewPromEmptyVectorResponse creates a Prometheus instant query response with no results.
func NewPromEmptyVectorResponse() *PromQueryResponse {
	resp := &PromQueryResponse{
		Status: "success",
	}
	resp.Data.ResultType = "vector"
	resp.Data.Result = []PromQueryResult{}
	return resp
}

// NewPromErrorResponse creates a Prometheus error response.
func NewPromErrorResponse(errorType, message string) *PromQueryResponse {
	return &PromQueryResponse{
		Status:    "error",
		ErrorType: errorType,
		Error:     message,
	}
}
