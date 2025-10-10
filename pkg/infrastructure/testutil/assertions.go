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
	"math"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/gomega" //nolint:staticcheck
)

// InfrastructureAssertions provides standardized assertion helpers for infrastructure tests
type InfrastructureAssertions struct{}

// NewInfrastructureAssertions creates a new infrastructure assertions helper
func NewInfrastructureAssertions() *InfrastructureAssertions {
	return &InfrastructureAssertions{}
}

// AssertNoError verifies no error occurred with context
func (a *InfrastructureAssertions) AssertNoError(err error, context string) {
	Expect(err).NotTo(HaveOccurred(), "Expected no error in %s, but got: %v", context, err)
}

// AssertErrorContains verifies error contains expected text
func (a *InfrastructureAssertions) AssertErrorContains(err error, expectedText string) {
	Expect(err).To(HaveOccurred(), "Expected an error containing '%s'", expectedText)
	Expect(err.Error()).To(ContainSubstring(expectedText))
}

// AssertInfrastructureConfigValid verifies infrastructure configuration is valid
func (a *InfrastructureAssertions) AssertInfrastructureConfigValid(config *InfrastructureConfig) {
	Expect(config).NotTo(BeNil(), "Metrics config should not be nil")
	Expect(config.Port).To(BeNumerically(">", 0), "Port should be positive")
	Expect(config.Port).To(BeNumerically("<=", 65535), "Port should be valid")

	if config.EnableMetrics {
		Expect(config.MetricsPath).NotTo(BeEmpty(), "Metrics path should not be empty when metrics enabled")
	}

	Expect(config.ServiceName).NotTo(BeEmpty(), "Service name should not be empty")
	Expect(config.Namespace).NotTo(BeEmpty(), "Namespace should not be empty")
}

// AssertHTTPResponseValid verifies HTTP response is valid
func (a *InfrastructureAssertions) AssertHTTPResponseValid(resp *http.Response) {
	Expect(resp).NotTo(BeNil(), "HTTP response should not be nil")
	Expect(resp.StatusCode).To(BeNumerically(">", 0), "Status code should be positive")
	Expect(resp.Header).NotTo(BeNil(), "Response headers should not be nil")
}

// AssertHTTPStatusCode verifies HTTP response has expected status code
func (a *InfrastructureAssertions) AssertHTTPStatusCode(resp *http.Response, expectedCode int) {
	a.AssertHTTPResponseValid(resp)
	Expect(resp.StatusCode).To(Equal(expectedCode),
		"Expected status code %d, got %d", expectedCode, resp.StatusCode)
}

// AssertSuccessfulHTTPResponse verifies HTTP response indicates success
func (a *InfrastructureAssertions) AssertSuccessfulHTTPResponse(resp *http.Response) {
	a.AssertHTTPResponseValid(resp)
	Expect(resp.StatusCode).To(BeNumerically(">=", 200), "Status code should indicate success")
	Expect(resp.StatusCode).To(BeNumerically("<", 300), "Status code should indicate success")
}

// AssertHTTPErrorResponse verifies HTTP response indicates error
func (a *InfrastructureAssertions) AssertHTTPErrorResponse(resp *http.Response) {
	a.AssertHTTPResponseValid(resp)
	Expect(resp.StatusCode).To(BeNumerically(">=", 400), "Status code should indicate error")
}

// AssertMetricsDataValid verifies metrics data is valid
func (a *InfrastructureAssertions) AssertMetricsDataValid(metricsData map[string]float64) {
	Expect(metricsData).NotTo(BeEmpty(), "Metrics data should not be empty")

	for metricName, value := range metricsData {
		Expect(metricName).NotTo(BeEmpty(), "Metric name should not be empty")
		Expect(math.IsNaN(value)).To(BeFalse(), "Metric value should not be NaN")
		Expect(math.IsInf(value, 0)).To(BeFalse(), "Metric value should not be infinite")
	}
}

// AssertHealthyMetrics verifies metrics indicate healthy system
func (a *InfrastructureAssertions) AssertHealthyMetrics(metricsData map[string]float64) {
	a.AssertMetricsDataValid(metricsData)

	// Check CPU usage is reasonable
	if cpuUsage, exists := metricsData["cpu_usage_percent"]; exists {
		Expect(cpuUsage).To(BeNumerically(">=", 0), "CPU usage should be non-negative")
		Expect(cpuUsage).To(BeNumerically("<=", 100), "CPU usage should not exceed 100%")
		Expect(cpuUsage).To(BeNumerically("<", 80), "CPU usage should be healthy (< 80%)")
	}

	// Check error rate is low
	if errorRate, exists := metricsData["error_rate_percent"]; exists {
		Expect(errorRate).To(BeNumerically(">=", 0), "Error rate should be non-negative")
		Expect(errorRate).To(BeNumerically("<", 5), "Error rate should be healthy (< 5%)")
	}

	// Check response times are reasonable
	if responseTime, exists := metricsData["response_time_p95_ms"]; exists {
		Expect(responseTime).To(BeNumerically(">", 0), "Response time should be positive")
		Expect(responseTime).To(BeNumerically("<", 1000), "P95 response time should be healthy (< 1s)")
	}
}

// AssertUnhealthyMetrics verifies metrics indicate unhealthy system
func (a *InfrastructureAssertions) AssertUnhealthyMetrics(metricsData map[string]float64) {
	a.AssertMetricsDataValid(metricsData)

	unhealthyIndicators := 0

	// Check for high CPU usage
	if cpuUsage, exists := metricsData["cpu_usage_percent"]; exists && cpuUsage > 85 {
		unhealthyIndicators++
	}

	// Check for high error rate
	if errorRate, exists := metricsData["error_rate_percent"]; exists && errorRate > 10 {
		unhealthyIndicators++
	}

	// Check for high response times
	if responseTime, exists := metricsData["response_time_p95_ms"]; exists && responseTime > 2000 {
		unhealthyIndicators++
	}

	// Check for high memory usage
	if memoryUsage, exists := metricsData["memory_usage_bytes"]; exists && memoryUsage > 1000000000 { // 1GB
		unhealthyIndicators++
	}

	Expect(unhealthyIndicators).To(BeNumerically(">", 0),
		"Should have at least one unhealthy indicator in metrics")
}

// AssertTimeSeriesDataValid verifies time series data
func (a *InfrastructureAssertions) AssertTimeSeriesDataValid(data []map[string]interface{}, expectedPattern string) {
	Expect(data).NotTo(BeEmpty(), "Time series data should not be empty")

	// Verify each point
	var prevTimestamp time.Time
	for i, point := range data {
		timestamp, ok := point["timestamp"].(time.Time)
		Expect(ok).To(BeTrue(), "Point %d should have valid timestamp", i)
		Expect(timestamp).NotTo(BeZero(), "Point %d timestamp should be set", i)

		if i > 0 {
			Expect(timestamp).To(BeTemporally(">=", prevTimestamp), "Timestamps should be ordered")
		}
		prevTimestamp = timestamp

		// Verify numeric values are valid
		for key, value := range point {
			if key == "timestamp" || key == "pattern" || key == "index" {
				continue
			}

			if floatVal, ok := value.(float64); ok {
				Expect(math.IsNaN(floatVal)).To(BeFalse(), "Point %d %s should not be NaN", i, key)
				Expect(math.IsInf(floatVal, 0)).To(BeFalse(), "Point %d %s should not be infinite", i, key)
			}
		}
	}

	// Verify pattern characteristics if specified
	if expectedPattern != "" && len(data) > 5 {
		switch expectedPattern {
		case "increasing_load":
			a.assertIncreasingPattern(data, "cpu_usage_percent")
		case "decreasing_load":
			a.assertDecreasingPattern(data, "cpu_usage_percent")
		case "spike":
			a.assertSpikePattern(data, "cpu_usage_percent")
		case "stable":
			a.assertStablePattern(data, "cpu_usage_percent")
		}
	}
}

// AssertServerPerformance verifies server performance metrics
func (a *InfrastructureAssertions) AssertServerPerformance(metrics map[string]float64, benchmarks map[string]float64) {
	a.AssertMetricsDataValid(metrics)

	// Compare against benchmarks
	for benchmarkName, benchmarkValue := range benchmarks {
		if actualValue, exists := metrics[benchmarkName]; exists {
			switch benchmarkName {
			case "requests_per_second":
				Expect(actualValue).To(BeNumerically(">=", benchmarkValue*0.8),
					"Request rate should be within 20% of benchmark")
			case "avg_response_time_ms", "p95_response_time_ms", "p99_response_time_ms":
				Expect(actualValue).To(BeNumerically("<=", benchmarkValue*1.2),
					"Response time should be within 20% of benchmark")
			case "error_rate_percent":
				Expect(actualValue).To(BeNumerically("<=", benchmarkValue*1.5),
					"Error rate should be within 50% of benchmark")
			case "cpu_usage_percent":
				Expect(actualValue).To(BeNumerically("<=", benchmarkValue*1.3),
					"CPU usage should be within 30% of benchmark")
			}
		}
	}
}

// AssertLoadTestResults verifies load test results
func (a *InfrastructureAssertions) AssertLoadTestResults(results map[string]interface{}) {
	Expect(results).NotTo(BeEmpty(), "Load test results should not be empty")

	// Verify required fields
	Expect(results).To(HaveKey("requests_sent"), "Results should include requests sent")
	Expect(results).To(HaveKey("requests_successful"), "Results should include successful requests")
	Expect(results).To(HaveKey("avg_response_time"), "Results should include average response time")

	requestsSent, ok := results["requests_sent"].(int)
	Expect(ok).To(BeTrue(), "Requests sent should be integer")
	Expect(requestsSent).To(BeNumerically(">", 0), "Should have sent requests")

	requestsSuccessful, ok := results["requests_successful"].(int)
	Expect(ok).To(BeTrue(), "Requests successful should be integer")
	Expect(requestsSuccessful).To(BeNumerically(">=", 0), "Successful requests should be non-negative")
	Expect(requestsSuccessful).To(BeNumerically("<=", requestsSent), "Successful requests should not exceed sent")
}

// AssertAlertThresholds verifies alert thresholds are reasonable
func (a *InfrastructureAssertions) AssertAlertThresholds(thresholds map[string]float64) {
	Expect(thresholds).NotTo(BeEmpty(), "Alert thresholds should not be empty")

	for thresholdName, value := range thresholds {
		Expect(thresholdName).NotTo(BeEmpty(), "Threshold name should not be empty")
		Expect(value).To(BeNumerically(">", 0), "Threshold value should be positive")

		// Verify warning thresholds are lower than critical thresholds
		if strings.Contains(thresholdName, "_warning") {
			criticalName := strings.Replace(thresholdName, "_warning", "_critical", 1)
			if criticalValue, exists := thresholds[criticalName]; exists {
				Expect(value).To(BeNumerically("<", criticalValue),
					"Warning threshold should be lower than critical threshold")
			}
		}
	}
}

// AssertHealthCheckEndpoint verifies health check endpoint functionality
func (a *InfrastructureAssertions) AssertHealthCheckEndpoint(endpoint map[string]interface{}) {
	Expect(endpoint).To(HaveKey("endpoint"), "Should have endpoint path")
	Expect(endpoint).To(HaveKey("expected_status"), "Should have expected status")

	endpointPath, ok := endpoint["endpoint"].(string)
	Expect(ok).To(BeTrue(), "Endpoint path should be string")
	Expect(endpointPath).To(HavePrefix("/"), "Endpoint path should start with /")

	expectedStatus, ok := endpoint["expected_status"].(int)
	Expect(ok).To(BeTrue(), "Expected status should be integer")
	Expect(expectedStatus).To(BeNumerically(">=", 200), "Expected status should be valid HTTP code")
	Expect(expectedStatus).To(BeNumerically("<", 600), "Expected status should be valid HTTP code")
}

// AssertPositiveNumber verifies number is positive
func (a *InfrastructureAssertions) AssertPositiveNumber(value float64, description string) {
	Expect(value).To(BeNumerically(">", 0), "%s should be positive, got %.2f", description, value)
}

// AssertNonNegativeNumber verifies number is non-negative
func (a *InfrastructureAssertions) AssertNonNegativeNumber(value float64, description string) {
	Expect(value).To(BeNumerically(">=", 0), "%s should be non-negative, got %.2f", description, value)
}

// AssertPercentageRange verifies value is in percentage range [0, 100]
func (a *InfrastructureAssertions) AssertPercentageRange(value float64, metricName string) {
	Expect(value).To(BeNumerically(">=", 0), "%s should be >= 0, got %.2f", metricName, value)
	Expect(value).To(BeNumerically("<=", 100), "%s should be <= 100, got %.2f", metricName, value)
}

// AssertResponseTimeReasonable verifies response time is reasonable
func (a *InfrastructureAssertions) AssertResponseTimeReasonable(responseTime float64, maxAcceptable float64) {
	Expect(responseTime).To(BeNumerically(">", 0), "Response time should be positive")
	Expect(responseTime).To(BeNumerically("<=", maxAcceptable),
		"Response time %.2fms should be <= %.2fms", responseTime, maxAcceptable)
}

// Helper methods for pattern analysis
func (a *InfrastructureAssertions) assertIncreasingPattern(data []map[string]interface{}, metricKey string) {
	increases := 0
	for i := 1; i < len(data); i++ {
		current := data[i][metricKey].(float64)
		previous := data[i-1][metricKey].(float64)
		if current > previous {
			increases++
		}
	}

	expectedIncreases := int(float64(len(data)-1) * 0.6) // Allow 40% noise
	Expect(increases).To(BeNumerically(">=", expectedIncreases),
		"Increasing pattern: expected >= %d increases in %s, got %d",
		expectedIncreases, metricKey, increases)
}

func (a *InfrastructureAssertions) assertDecreasingPattern(data []map[string]interface{}, metricKey string) {
	decreases := 0
	for i := 1; i < len(data); i++ {
		current := data[i][metricKey].(float64)
		previous := data[i-1][metricKey].(float64)
		if current < previous {
			decreases++
		}
	}

	expectedDecreases := int(float64(len(data)-1) * 0.6)
	Expect(decreases).To(BeNumerically(">=", expectedDecreases),
		"Decreasing pattern: expected >= %d decreases in %s, got %d",
		expectedDecreases, metricKey, decreases)
}

func (a *InfrastructureAssertions) assertSpikePattern(data []map[string]interface{}, metricKey string) {
	// Find the maximum value and ensure it's significantly higher than average
	var values []float64
	maxValue := 0.0

	for _, point := range data {
		value := point[metricKey].(float64)
		values = append(values, value)
		if value > maxValue {
			maxValue = value
		}
	}

	// Calculate average
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	average := sum / float64(len(values))

	// Spike should be at least 2x the average
	Expect(maxValue).To(BeNumerically(">=", average*2),
		"Spike pattern: max value %.2f should be >= 2x average %.2f", maxValue, average)
}

func (a *InfrastructureAssertions) assertStablePattern(data []map[string]interface{}, metricKey string) {
	// Calculate standard deviation - should be relatively low for stable pattern
	var values []float64
	sum := 0.0

	for _, point := range data {
		value := point[metricKey].(float64)
		values = append(values, value)
		sum += value
	}

	mean := sum / float64(len(values))
	variance := 0.0
	for _, value := range values {
		variance += (value - mean) * (value - mean)
	}
	variance /= float64(len(values))
	stdDev := math.Sqrt(variance)

	// Standard deviation should be less than 20% of mean for stable pattern
	maxStdDev := mean * 0.2
	Expect(stdDev).To(BeNumerically("<=", maxStdDev),
		"Stable pattern: standard deviation %.2f should be <= %.2f (20%% of mean %.2f)",
		stdDev, maxStdDev, mean)
}
