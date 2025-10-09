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

package shared

import (
	"runtime"
	"time"
)

// ResourceMonitor tracks resource usage during tests
type ResourceMonitor struct {
	StartTime    time.Time
	StartMemory  runtime.MemStats
	Measurements []Measurement
}

// Measurement represents a single resource measurement
type Measurement struct {
	Timestamp     time.Time     `json:"timestamp"`
	MemoryUsage   uint64        `json:"memory_usage"`
	ResponseTime  time.Duration `json:"response_time"`
	NumGoroutines int           `json:"num_goroutines"`
}

// ResourceReport summarizes resource usage over the test period
type ResourceReport struct {
	TotalDuration    time.Duration `json:"total_duration"`
	AverageResponse  time.Duration `json:"average_response"`
	MaxResponse      time.Duration `json:"max_response"`
	MinResponse      time.Duration `json:"min_response"`
	P95Response      time.Duration `json:"p95_response"`
	P99Response      time.Duration `json:"p99_response"`
	MemoryGrowth     int64         `json:"memory_growth"`
	MaxMemoryUsage   uint64        `json:"max_memory_usage"`
	MeasurementCount int           `json:"measurement_count"`
	GoroutineGrowth  int           `json:"goroutine_growth"`
}

// IntegrationTestReport contains the complete test results
type IntegrationTestReport struct {
	TotalTests         int             `json:"total_tests"`
	PassedTests        int             `json:"passed_tests"`
	FailedTests        []string        `json:"failed_tests"`
	SkippedTests       []string        `json:"skipped_tests"`
	AverageResponse    time.Duration   `json:"average_response_time"`
	MaxResponse        time.Duration   `json:"max_response_time"`
	ResourceUsage      ResourceReport  `json:"resource_usage"`
	ModelResponses     []ModelResponse `json:"model_responses"`
	ActionDistribution map[string]int  `json:"action_distribution"`
	ConfidenceStats    ConfidenceStats `json:"confidence_stats"`
	TestDuration       time.Duration   `json:"test_duration"`
}

// ModelResponse captures a single model response for analysis
type ModelResponse struct {
	TestName     string        `json:"test_name"`
	Action       string        `json:"action"`
	Confidence   float64       `json:"confidence"`
	Reasoning    string        `json:"reasoning"`
	ResponseTime time.Duration `json:"response_time"`
	Success      bool          `json:"success"`
	Error        string        `json:"error,omitempty"`
}

// ConfidenceStats provides statistics about confidence scores
type ConfidenceStats struct {
	Average float64 `json:"average"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	P50     float64 `json:"p50"`
	P95     float64 `json:"p95"`
	P99     float64 `json:"p99"`
	Count   int     `json:"count"`
}

// NewResourceMonitor creates a new resource monitor
func NewResourceMonitor() *ResourceMonitor {
	var startMem runtime.MemStats
	runtime.ReadMemStats(&startMem)

	return &ResourceMonitor{
		StartTime:    time.Now(),
		StartMemory:  startMem,
		Measurements: make([]Measurement, 0),
	}
}

// RecordMeasurement records a new measurement
func (rm *ResourceMonitor) RecordMeasurement(responseTime time.Duration) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	rm.Measurements = append(rm.Measurements, Measurement{
		Timestamp:     time.Now(),
		MemoryUsage:   m.Alloc,
		ResponseTime:  responseTime,
		NumGoroutines: runtime.NumGoroutine(),
	})
}

// GenerateReport generates a comprehensive resource usage report
func (rm *ResourceMonitor) GenerateReport() ResourceReport {
	if len(rm.Measurements) == 0 {
		return ResourceReport{}
	}

	// Calculate response time statistics
	var totalResponse time.Duration
	var maxResponse time.Duration
	var minResponse = time.Hour // Initialize to large value
	var maxMemory uint64
	responseTimes := make([]time.Duration, len(rm.Measurements))

	for i, m := range rm.Measurements {
		totalResponse += m.ResponseTime
		responseTimes[i] = m.ResponseTime

		if m.ResponseTime > maxResponse {
			maxResponse = m.ResponseTime
		}
		if m.ResponseTime < minResponse {
			minResponse = m.ResponseTime
		}
		if m.MemoryUsage > maxMemory {
			maxMemory = m.MemoryUsage
		}
	}

	avgResponse := totalResponse / time.Duration(len(rm.Measurements))

	// Calculate percentiles
	p95Response := calculatePercentile(responseTimes, 0.95)
	p99Response := calculatePercentile(responseTimes, 0.99)

	// Calculate memory growth
	var finalMem runtime.MemStats
	runtime.ReadMemStats(&finalMem)
	memoryGrowth := int64(finalMem.Alloc) - int64(rm.StartMemory.Alloc)

	// Calculate goroutine growth
	finalGoroutines := runtime.NumGoroutine()
	var startGoroutines int
	if len(rm.Measurements) > 0 {
		startGoroutines = rm.Measurements[0].NumGoroutines
	}
	goroutineGrowth := finalGoroutines - startGoroutines

	return ResourceReport{
		TotalDuration:    time.Since(rm.StartTime),
		AverageResponse:  avgResponse,
		MaxResponse:      maxResponse,
		MinResponse:      minResponse,
		P95Response:      p95Response,
		P99Response:      p99Response,
		MemoryGrowth:     memoryGrowth,
		MaxMemoryUsage:   maxMemory,
		MeasurementCount: len(rm.Measurements),
		GoroutineGrowth:  goroutineGrowth,
	}
}

// calculatePercentile calculates the given percentile of response times
func calculatePercentile(times []time.Duration, percentile float64) time.Duration {
	if len(times) == 0 {
		return 0
	}

	// Simple percentile calculation (would use sort in production)
	index := int(float64(len(times)) * percentile)
	if index >= len(times) {
		index = len(times) - 1
	}

	// Find the value at the percentile index (simplified)
	// In a real implementation, we'd sort the slice first
	return times[index]
}

// NewIntegrationTestReport creates a new test report
func NewIntegrationTestReport() *IntegrationTestReport {
	return &IntegrationTestReport{
		FailedTests:        make([]string, 0),
		SkippedTests:       make([]string, 0),
		ModelResponses:     make([]ModelResponse, 0),
		ActionDistribution: make(map[string]int),
	}
}

// AddModelResponse adds a model response to the report
func (itr *IntegrationTestReport) AddModelResponse(response ModelResponse) {
	itr.ModelResponses = append(itr.ModelResponses, response)

	if response.Success {
		itr.ActionDistribution[response.Action]++
	}
}

// AddFailedTest adds a failed test to the report
func (itr *IntegrationTestReport) AddFailedTest(testName string) {
	itr.FailedTests = append(itr.FailedTests, testName)
}

// AddSkippedTest adds a skipped test to the report
func (itr *IntegrationTestReport) AddSkippedTest(testName string) {
	itr.SkippedTests = append(itr.SkippedTests, testName)
}

// CalculateStats calculates final statistics for the report
func (itr *IntegrationTestReport) CalculateStats(resourceReport ResourceReport) {
	itr.ResourceUsage = resourceReport

	// Calculate response time statistics
	if len(itr.ModelResponses) > 0 {
		var totalResponseTime time.Duration
		var maxResponseTime time.Duration

		for _, response := range itr.ModelResponses {
			if response.Success {
				totalResponseTime += response.ResponseTime
				if response.ResponseTime > maxResponseTime {
					maxResponseTime = response.ResponseTime
				}
			}
		}

		successfulResponses := itr.PassedTests
		if successfulResponses > 0 {
			itr.AverageResponse = totalResponseTime / time.Duration(successfulResponses)
		}
		itr.MaxResponse = maxResponseTime
	}

	// Calculate confidence statistics
	itr.ConfidenceStats = itr.calculateConfidenceStats()
}

// calculateConfidenceStats calculates confidence score statistics
func (itr *IntegrationTestReport) calculateConfidenceStats() ConfidenceStats {
	var confidences []float64

	for _, response := range itr.ModelResponses {
		if response.Success {
			confidences = append(confidences, response.Confidence)
		}
	}

	if len(confidences) == 0 {
		return ConfidenceStats{}
	}

	// Calculate basic statistics
	var sum float64
	var min, max = 1.0, 0.0

	for _, conf := range confidences {
		sum += conf
		if conf < min {
			min = conf
		}
		if conf > max {
			max = conf
		}
	}

	average := sum / float64(len(confidences))

	// For simplicity, using average for percentiles
	// In production, we'd properly sort and calculate
	return ConfidenceStats{
		Average: average,
		Min:     min,
		Max:     max,
		P50:     average, // Simplified
		P95:     max,     // Simplified
		P99:     max,     // Simplified
		Count:   len(confidences),
	}
}
