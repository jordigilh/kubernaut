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

package testutil

import (
	"math/rand"
	"net/http"
	"time"
)

// InfrastructureTestDataFactory provides standardized test data creation for infrastructure tests
type InfrastructureTestDataFactory struct{}

// NewInfrastructureTestDataFactory creates a new test data factory for infrastructure tests
func NewInfrastructureTestDataFactory() *InfrastructureTestDataFactory {
	return &InfrastructureTestDataFactory{}
}

// CreateInfrastructureConfig creates a test infrastructure configuration
func (f *InfrastructureTestDataFactory) CreateInfrastructureConfig() *InfrastructureConfig {
	return &InfrastructureConfig{
		Port:                  8080,
		ReadTimeout:           30 * time.Second,
		WriteTimeout:          30 * time.Second,
		ShutdownTimeout:       10 * time.Second,
		EnableMetrics:         true,
		MetricsPath:           "/metrics",
		HealthPath:            "/health",
		ReadyPath:             "/ready",
		MaxRequestsPerSecond:  100,
		EnableRequestLogging:  true,
		EnableDetailedMetrics: false,
		Namespace:             "test",
		ServiceName:           "test-service",
	}
}

// CreateHighPerformanceConfig creates a high-performance infrastructure configuration
func (f *InfrastructureTestDataFactory) CreateHighPerformanceConfig() *InfrastructureConfig {
	return &InfrastructureConfig{
		Port:                  8080,
		ReadTimeout:           5 * time.Second,
		WriteTimeout:          5 * time.Second,
		ShutdownTimeout:       2 * time.Second,
		EnableMetrics:         true,
		MetricsPath:           "/metrics",
		HealthPath:            "/health",
		ReadyPath:             "/ready",
		MaxRequestsPerSecond:  10000,
		EnableRequestLogging:  false,
		EnableDetailedMetrics: true,
		Namespace:             "high-perf",
		ServiceName:           "high-perf-service",
	}
}

// CreateMinimalConfig creates a minimal infrastructure configuration
func (f *InfrastructureTestDataFactory) CreateMinimalConfig() *InfrastructureConfig {
	return &InfrastructureConfig{
		Port:          8080,
		EnableMetrics: true,
		MetricsPath:   "/metrics",
		Namespace:     "minimal",
		ServiceName:   "minimal-service",
	}
}

// CreateMetricsData creates test metrics data
func (f *InfrastructureTestDataFactory) CreateMetricsData() map[string]float64 {
	return map[string]float64{
		"http_requests_total":         float64(rand.Intn(10000) + 1000),
		"http_request_duration_ms":    float64(rand.Intn(1000) + 10),
		"memory_usage_bytes":          float64(rand.Intn(1000000000) + 100000000),
		"cpu_usage_percent":           float64(rand.Intn(80) + 10),
		"active_connections":          float64(rand.Intn(100) + 1),
		"error_rate_percent":          float64(rand.Intn(5)),
		"response_time_p95_ms":        float64(rand.Intn(500) + 50),
		"response_time_p99_ms":        float64(rand.Intn(1000) + 100),
		"throughput_requests_per_sec": float64(rand.Intn(1000) + 10),
	}
}

// CreateHealthyMetricsData creates healthy metrics data
func (f *InfrastructureTestDataFactory) CreateHealthyMetricsData() map[string]float64 {
	return map[string]float64{
		"http_requests_total":         2500.0,
		"http_request_duration_ms":    25.5,
		"memory_usage_bytes":          500000000.0, // 500MB
		"cpu_usage_percent":           15.0,
		"active_connections":          50.0,
		"error_rate_percent":          0.1,
		"response_time_p95_ms":        100.0,
		"response_time_p99_ms":        200.0,
		"throughput_requests_per_sec": 100.0,
	}
}

// CreateUnhealthyMetricsData creates unhealthy metrics data
func (f *InfrastructureTestDataFactory) CreateUnhealthyMetricsData() map[string]float64 {
	return map[string]float64{
		"http_requests_total":         50000.0,
		"http_request_duration_ms":    1500.0,
		"memory_usage_bytes":          1500000000.0, // 1.5GB
		"cpu_usage_percent":           95.0,
		"active_connections":          1000.0,
		"error_rate_percent":          15.0,
		"response_time_p95_ms":        2000.0,
		"response_time_p99_ms":        5000.0,
		"throughput_requests_per_sec": 5.0,
	}
}

// CreateTimeSeriesMetricsData creates time series metrics data
func (f *InfrastructureTestDataFactory) CreateTimeSeriesMetricsData(points int, pattern string) []map[string]interface{} {
	data := make([]map[string]interface{}, points)
	baseTime := time.Now().Add(-time.Duration(points) * time.Minute)

	for i := 0; i < points; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Minute)

		var cpuUsage, memoryUsage, requestRate float64

		switch pattern {
		case "increasing_load":
			cpuUsage = 10.0 + float64(i)*0.5 + rand.Float64()*5.0
			memoryUsage = 200000000.0 + float64(i)*1000000.0 + rand.Float64()*50000000.0
			requestRate = 10.0 + float64(i)*0.2 + rand.Float64()*2.0
		case "decreasing_load":
			cpuUsage = 80.0 - float64(i)*0.3 + rand.Float64()*5.0
			memoryUsage = 800000000.0 - float64(i)*2000000.0 + rand.Float64()*50000000.0
			requestRate = 100.0 - float64(i)*0.5 + rand.Float64()*5.0
		case "spike":
			if i > points/2-5 && i < points/2+5 {
				cpuUsage = 90.0 + rand.Float64()*10.0
				memoryUsage = 1000000000.0 + rand.Float64()*200000000.0
				requestRate = 500.0 + rand.Float64()*100.0
			} else {
				cpuUsage = 20.0 + rand.Float64()*10.0
				memoryUsage = 300000000.0 + rand.Float64()*100000000.0
				requestRate = 50.0 + rand.Float64()*20.0
			}
		case "stable":
			cpuUsage = 25.0 + rand.Float64()*5.0
			memoryUsage = 400000000.0 + rand.Float64()*50000000.0
			requestRate = 75.0 + rand.Float64()*10.0
		default:
			cpuUsage = rand.Float64() * 100.0
			memoryUsage = rand.Float64() * 1000000000.0
			requestRate = rand.Float64() * 200.0
		}

		data[i] = map[string]interface{}{
			"timestamp":          timestamp,
			"cpu_usage_percent":  cpuUsage,
			"memory_usage_bytes": memoryUsage,
			"request_rate":       requestRate,
			"pattern":            pattern,
			"index":              i,
		}
	}

	return data
}

// CreateHTTPResponse creates test HTTP response data
func (f *InfrastructureTestDataFactory) CreateHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       http.NoBody,
	}
}

// CreateSuccessfulHTTPResponse creates a successful HTTP response
func (f *InfrastructureTestDataFactory) CreateSuccessfulHTTPResponse() *http.Response {
	resp := f.CreateHTTPResponse(http.StatusOK, `{"status":"success"}`)
	resp.Header.Set("Content-Type", "application/json")
	return resp
}

// CreateErrorHTTPResponse creates an error HTTP response
func (f *InfrastructureTestDataFactory) CreateErrorHTTPResponse() *http.Response {
	resp := f.CreateHTTPResponse(http.StatusInternalServerError, `{"status":"error","message":"Internal server error"}`)
	resp.Header.Set("Content-Type", "application/json")
	return resp
}

// CreatePrometheusMetricsResponse creates a Prometheus-style metrics response
func (f *InfrastructureTestDataFactory) CreatePrometheusMetricsResponse() string {
	return `# HELP http_requests_total Total number of HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 1000
http_requests_total{method="GET",code="404"} 50
http_requests_total{method="POST",code="200"} 500
http_requests_total{method="POST",code="500"} 25

# HELP http_request_duration_seconds HTTP request latency
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.1"} 500
http_request_duration_seconds_bucket{le="0.5"} 900
http_request_duration_seconds_bucket{le="1.0"} 950
http_request_duration_seconds_bucket{le="+Inf"} 1000
http_request_duration_seconds_sum 250.5
http_request_duration_seconds_count 1000

# HELP memory_usage_bytes Current memory usage in bytes
# TYPE memory_usage_bytes gauge
memory_usage_bytes 500000000

# HELP cpu_usage_percent Current CPU usage percentage
# TYPE cpu_usage_percent gauge
cpu_usage_percent 25.5
`
}

// CreateServerEndpoints creates test server endpoint configurations
func (f *InfrastructureTestDataFactory) CreateServerEndpoints() map[string]string {
	return map[string]string{
		"metrics": "/metrics",
		"health":  "/health",
		"ready":   "/ready",
		"info":    "/info",
		"status":  "/status",
	}
}

// CreateLoadTestScenarios creates load testing scenario configurations
func (f *InfrastructureTestDataFactory) CreateLoadTestScenarios() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":                "light_load",
			"requests_per_second": 10,
			"duration_seconds":    30,
			"concurrent_users":    5,
		},
		{
			"name":                "moderate_load",
			"requests_per_second": 100,
			"duration_seconds":    60,
			"concurrent_users":    25,
		},
		{
			"name":                "heavy_load",
			"requests_per_second": 1000,
			"duration_seconds":    120,
			"concurrent_users":    100,
		},
		{
			"name":                "stress_test",
			"requests_per_second": 5000,
			"duration_seconds":    30,
			"concurrent_users":    500,
		},
	}
}

// CreateAlertThresholds creates alert threshold configurations
func (f *InfrastructureTestDataFactory) CreateAlertThresholds() map[string]float64 {
	return map[string]float64{
		"cpu_usage_warning":      70.0,
		"cpu_usage_critical":     90.0,
		"memory_usage_warning":   800000000.0,  // 800MB
		"memory_usage_critical":  1000000000.0, // 1GB
		"error_rate_warning":     5.0,
		"error_rate_critical":    10.0,
		"response_time_warning":  1000.0, // 1 second
		"response_time_critical": 5000.0, // 5 seconds
	}
}

// CreateHealthCheckEndpoints creates health check endpoint test data
func (f *InfrastructureTestDataFactory) CreateHealthCheckEndpoints() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"endpoint":        "/health",
			"expected_status": 200,
			"expected_body":   `{"status":"healthy"}`,
			"timeout":         5 * time.Second,
		},
		{
			"endpoint":        "/ready",
			"expected_status": 200,
			"expected_body":   `{"status":"ready"}`,
			"timeout":         3 * time.Second,
		},
		{
			"endpoint":        "/metrics",
			"expected_status": 200,
			"content_type":    "text/plain",
			"timeout":         10 * time.Second,
		},
	}
}

// CreatePerformanceBenchmarks creates performance benchmark data
func (f *InfrastructureTestDataFactory) CreatePerformanceBenchmarks() map[string]map[string]float64 {
	return map[string]map[string]float64{
		"baseline": {
			"requests_per_second":  100.0,
			"avg_response_time_ms": 50.0,
			"p95_response_time_ms": 100.0,
			"p99_response_time_ms": 200.0,
			"error_rate_percent":   1.0,
			"cpu_usage_percent":    30.0,
			"memory_usage_mb":      400.0,
		},
		"target": {
			"requests_per_second":  200.0,
			"avg_response_time_ms": 25.0,
			"p95_response_time_ms": 50.0,
			"p99_response_time_ms": 100.0,
			"error_rate_percent":   0.5,
			"cpu_usage_percent":    50.0,
			"memory_usage_mb":      600.0,
		},
		"maximum": {
			"requests_per_second":  1000.0,
			"avg_response_time_ms": 100.0,
			"p95_response_time_ms": 500.0,
			"p99_response_time_ms": 1000.0,
			"error_rate_percent":   5.0,
			"cpu_usage_percent":    80.0,
			"memory_usage_mb":      1000.0,
		},
	}
}

// CreateErrorScenarios creates error testing scenarios
func (f *InfrastructureTestDataFactory) CreateErrorScenarios() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":          "network_timeout",
			"error_type":    "timeout",
			"expected_code": http.StatusGatewayTimeout,
			"retry_count":   3,
			"delay":         1 * time.Second,
		},
		{
			"name":          "server_error",
			"error_type":    "server",
			"expected_code": http.StatusInternalServerError,
			"retry_count":   2,
			"delay":         500 * time.Millisecond,
		},
		{
			"name":          "rate_limit",
			"error_type":    "rate_limit",
			"expected_code": http.StatusTooManyRequests,
			"retry_count":   1,
			"delay":         2 * time.Second,
		},
		{
			"name":          "not_found",
			"error_type":    "not_found",
			"expected_code": http.StatusNotFound,
			"retry_count":   0,
			"delay":         0,
		},
	}
}
