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

//go:build integration

package llm_integration

import (
	"runtime"
	"sync"
	"time"
)

// =============================================================================
// SHARED PERFORMANCE MONITORING UTILITIES
// =============================================================================

// PerformanceTracker consolidates performance monitoring for all AI tests
type PerformanceTracker struct {
	metricsMap map[string]*TestMetrics
	mutex      sync.RWMutex
}

type TestMetrics struct {
	Name         string
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	MemoryBefore uint64
	MemoryAfter  uint64
	MemoryDelta  uint64
	Success      bool
	ErrorCount   int
}

// NewPerformanceTracker creates a consolidated performance tracker
func NewPerformanceTracker() *PerformanceTracker {
	return &PerformanceTracker{
		metricsMap: make(map[string]*TestMetrics),
	}
}

// StartTracking begins performance tracking for a test
func (pt *PerformanceTracker) StartTracking(testName string) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	pt.metricsMap[testName] = &TestMetrics{
		Name:         testName,
		StartTime:    time.Now(),
		MemoryBefore: m.Alloc,
	}
}

// EndTracking completes performance tracking for a test
func (pt *PerformanceTracker) EndTracking(testName string, success bool) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	metric, exists := pt.metricsMap[testName]
	if !exists {
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	metric.EndTime = time.Now()
	metric.Duration = metric.EndTime.Sub(metric.StartTime)
	metric.MemoryAfter = m.Alloc
	metric.MemoryDelta = metric.MemoryAfter - metric.MemoryBefore
	metric.Success = success
}

// RecordError increments error count for a test
func (pt *PerformanceTracker) RecordError(testName string) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	if metric, exists := pt.metricsMap[testName]; exists {
		metric.ErrorCount++
	}
}

// GetMetrics returns metrics for a specific test
func (pt *PerformanceTracker) GetMetrics(testName string) *TestMetrics {
	pt.mutex.RLock()
	defer pt.mutex.RUnlock()

	if metric, exists := pt.metricsMap[testName]; exists {
		// Return a copy to prevent race conditions
		return &TestMetrics{
			Name:         metric.Name,
			StartTime:    metric.StartTime,
			EndTime:      metric.EndTime,
			Duration:     metric.Duration,
			MemoryBefore: metric.MemoryBefore,
			MemoryAfter:  metric.MemoryAfter,
			MemoryDelta:  metric.MemoryDelta,
			Success:      metric.Success,
			ErrorCount:   metric.ErrorCount,
		}
	}
	return nil
}

// GenerateReport creates a comprehensive performance report
func (pt *PerformanceTracker) GenerateReport() map[string]interface{} {
	pt.mutex.RLock()
	defer pt.mutex.RUnlock()

	totalTests := len(pt.metricsMap)
	successfulTests := 0
	totalDuration := time.Duration(0)
	totalMemoryUsed := uint64(0)
	totalErrors := 0

	testDetails := make([]map[string]interface{}, 0, totalTests)

	for _, metric := range pt.metricsMap {
		if metric.Success {
			successfulTests++
		}
		totalDuration += metric.Duration
		totalMemoryUsed += metric.MemoryDelta
		totalErrors += metric.ErrorCount

		testDetails = append(testDetails, map[string]interface{}{
			"name":         metric.Name,
			"duration_ms":  metric.Duration.Milliseconds(),
			"memory_delta": metric.MemoryDelta,
			"success":      metric.Success,
			"error_count":  metric.ErrorCount,
		})
	}

	var avgDuration time.Duration
	if totalTests > 0 {
		avgDuration = totalDuration / time.Duration(totalTests)
	}

	return map[string]interface{}{
		"summary": map[string]interface{}{
			"total_tests":      totalTests,
			"successful_tests": successfulTests,
			"success_rate":     float64(successfulTests) / float64(totalTests),
			"total_duration":   totalDuration,
			"avg_duration":     avgDuration,
			"total_memory_mb":  float64(totalMemoryUsed) / (1024 * 1024),
			"total_errors":     totalErrors,
		},
		"test_details": testDetails,
	}
}

// Reset clears all metrics
func (pt *PerformanceTracker) Reset() {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	pt.metricsMap = make(map[string]*TestMetrics)
}

// =============================================================================
// LEGACY COMPATIBILITY (for existing code)
// =============================================================================

// Note: MemoryProfiler kept in performance_test.go for performance-specific functionality

// NewMemoryProfiler - see performance_test.go

// MemoryProfiler methods - see performance_test.go

// Note: LatencyTracker kept in performance_test.go for performance-specific functionality

// NewLatencyTracker - see performance_test.go

// LatencyTracker methods - see performance_test.go

// generatePerformanceReport - see performance_test.go for performance-specific implementation
