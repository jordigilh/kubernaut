<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package testutil

import (
	"fmt"
	"net/http"
	"time"
)

// SharedTestDataFactory provides standardized test data creation for shared package tests
type SharedTestDataFactory struct{}

// NewSharedTestDataFactory creates a new test data factory for shared package tests
func NewSharedTestDataFactory() *SharedTestDataFactory {
	return &SharedTestDataFactory{}
}

// CreateTestError creates a standard test error
func (f *SharedTestDataFactory) CreateTestError(message string) error {
	return fmt.Errorf("%s", message)
}

// CreateTimeoutError creates a timeout error
func (f *SharedTestDataFactory) CreateTimeoutError() error {
	return fmt.Errorf("operation timed out after 30s")
}

// CreateNetworkError creates a network error
func (f *SharedTestDataFactory) CreateNetworkError() error {
	return fmt.Errorf("network connection failed: connection refused")
}

// CreateValidationError creates a validation error
func (f *SharedTestDataFactory) CreateValidationError(field string) error {
	return fmt.Errorf("validation failed for field '%s': required field is missing", field)
}

// CreateHTTPClientConfig creates a standard HTTP client configuration
func (f *SharedTestDataFactory) CreateHTTPClientConfig() map[string]interface{} {
	return map[string]interface{}{
		"timeout":                  30 * time.Second,
		"max_retries":              3,
		"disable_ssl_verification": false,
		"max_idle_conns":           10,
		"idle_conn_timeout":        90 * time.Second,
		"tls_handshake_timeout":    10 * time.Second,
		"response_header_timeout":  10 * time.Second,
	}
}

// CreateCustomHTTPClientConfig creates an HTTP client config with custom values
func (f *SharedTestDataFactory) CreateCustomHTTPClientConfig(timeout time.Duration, maxRetries int) map[string]interface{} {
	config := f.CreateHTTPClientConfig()
	config["timeout"] = timeout
	config["max_retries"] = maxRetries
	return config
}

// CreateTestURLs creates a set of test URLs for HTTP testing
func (f *SharedTestDataFactory) CreateTestURLs() []string {
	return []string{
		"https://api.kubernaut.io/v1/health",
		"https://service.test.com/status",
		"http://localhost:8080/metrics",
		"https://monitoring.internal/ping",
	}
}

// CreateHTTPHeaders creates common HTTP headers for testing
func (f *SharedTestDataFactory) CreateHTTPHeaders() http.Header {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	headers.Set("User-Agent", "test-client/1.0")
	headers.Set("X-Request-ID", "test-request-123")
	return headers
}

// CreateLogFields creates standard logging fields for testing
func (f *SharedTestDataFactory) CreateLogFields() map[string]interface{} {
	return map[string]interface{}{
		"timestamp":  time.Now(),
		"level":      "info",
		"message":    "Test log message",
		"component":  "test-component",
		"operation":  "test-operation",
		"request_id": "test-req-123",
		"user_id":    "test-user-456",
		"trace_id":   "test-trace-789",
	}
}

// CreateErrorLogFields creates error-specific logging fields
func (f *SharedTestDataFactory) CreateErrorLogFields(err error) map[string]interface{} {
	fields := f.CreateLogFields()
	fields["level"] = "error"
	fields["error"] = err.Error()
	fields["error_type"] = fmt.Sprintf("%T", err)
	fields["stack_trace"] = "test stack trace"
	return fields
}

// CreateContextualLogFields creates logging fields with additional context
func (f *SharedTestDataFactory) CreateContextualLogFields(context map[string]interface{}) map[string]interface{} {
	fields := f.CreateLogFields()
	for key, value := range context {
		fields[key] = value
	}
	return fields
}

// CreateTestStatistics creates sample statistical data for testing
func (f *SharedTestDataFactory) CreateTestStatistics() map[string]float64 {
	return map[string]float64{
		"count":         100.0,
		"mean":          50.5,
		"median":        50.0,
		"stddev":        28.87,
		"variance":      833.25,
		"min":           1.0,
		"max":           100.0,
		"percentile_50": 50.0,
		"percentile_90": 90.0,
		"percentile_95": 95.0,
		"percentile_99": 99.0,
	}
}

// CreateSampleDataSet creates a sample dataset for statistical testing
func (f *SharedTestDataFactory) CreateSampleDataSet(size int) []float64 {
	data := make([]float64, size)
	for i := 0; i < size; i++ {
		data[i] = float64(i + 1) // Simple sequence: 1, 2, 3, ..., size
	}
	return data
}

// CreateNormalDistributionData creates normally distributed sample data
func (f *SharedTestDataFactory) CreateNormalDistributionData() []float64 {
	// Approximate normal distribution for testing
	return []float64{
		45.2, 48.7, 52.3, 49.1, 51.8, 47.6, 53.2, 50.4, 49.9, 52.7,
		48.3, 51.5, 47.9, 53.8, 49.6, 52.1, 48.8, 50.7, 49.3, 51.2,
		47.4, 52.9, 48.1, 51.9, 50.2, 49.5, 53.1, 47.8, 52.4, 50.8,
	}
}

// CreateSkewedDistributionData creates skewed sample data for testing
func (f *SharedTestDataFactory) CreateSkewedDistributionData() []float64 {
	return []float64{
		10, 12, 15, 18, 20, 22, 25, 28, 30, 35,
		40, 45, 55, 60, 75, 80, 95, 100, 120, 150,
	}
}

// CreateEmptyDataSet creates an empty dataset for edge case testing
func (f *SharedTestDataFactory) CreateEmptyDataSet() []float64 {
	return []float64{}
}

// CreateSingleElementDataSet creates a dataset with one element
func (f *SharedTestDataFactory) CreateSingleElementDataSet(value float64) []float64 {
	return []float64{value}
}

// CreateTestConfigMap creates a configuration map for testing
func (f *SharedTestDataFactory) CreateTestConfigMap() map[string]interface{} {
	return map[string]interface{}{
		"service_name":    "test-service",
		"port":            8080,
		"debug":           true,
		"timeout":         "30s",
		"max_connections": 100,
		"allowed_hosts":   []string{"localhost", "127.0.0.1", "::1"},
		"features": map[string]bool{
			"feature_a": true,
			"feature_b": false,
			"feature_c": true,
		},
	}
}

// CreateTestDurations creates various durations for timeout testing
func (f *SharedTestDataFactory) CreateTestDurations() []time.Duration {
	return []time.Duration{
		1 * time.Millisecond,
		10 * time.Millisecond,
		100 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
		10 * time.Second,
		30 * time.Second,
		1 * time.Minute,
		5 * time.Minute,
	}
}

// CreateTestTimestamps creates a series of timestamps for temporal testing
func (f *SharedTestDataFactory) CreateTestTimestamps() []time.Time {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	return []time.Time{
		base,
		base.Add(1 * time.Hour),
		base.Add(24 * time.Hour),
		base.Add(7 * 24 * time.Hour),
		base.Add(30 * 24 * time.Hour),
	}
}

// CreateTestJSONStrings creates various JSON strings for parsing tests
func (f *SharedTestDataFactory) CreateTestJSONStrings() []string {
	return []string{
		`{"name": "test", "value": 123}`,
		`[1, 2, 3, 4, 5]`,
		`null`,
		`true`,
		`false`,
		`"simple string"`,
		`{"nested": {"object": {"value": "deep"}}}`,
		`{"array": [{"id": 1}, {"id": 2}]}`,
	}
}

// CreateInvalidJSONStrings creates invalid JSON strings for error testing
func (f *SharedTestDataFactory) CreateInvalidJSONStrings() []string {
	return []string{
		`{invalid json}`,
		`{"missing": quote}`,
		`{trailing comma,}`,
		`{"unmatched": bracket]`,
		`{missing closing brace`,
		`invalid`,
		``,
	}
}
