package utils

import (
	"time"
)

// **REFACTOR PHASE**: Extracted common metrics patterns for code quality improvement

// MetricsCollector provides common metrics collection functionality
type MetricsCollector struct {
	startTime time.Time
	metrics   map[string]interface{}
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		startTime: time.Now(),
		metrics:   make(map[string]interface{}),
	}
}

// RecordMetric records a metric value
func (mc *MetricsCollector) RecordMetric(name string, value interface{}) {
	mc.metrics[name] = value
}

// GetProcessingTime returns the processing time since creation
func (mc *MetricsCollector) GetProcessingTime() time.Duration {
	return time.Since(mc.startTime)
}

// GetMetrics returns all recorded metrics
func (mc *MetricsCollector) GetMetrics() map[string]interface{} {
	// Add processing time to metrics
	mc.metrics["processing_time"] = mc.GetProcessingTime()
	mc.metrics["performance_tier"] = ClassifyPerformance(mc.GetProcessingTime())
	return mc.metrics
}

// CalculateCacheHitRate calculates cache hit rate
// **PERFORMANCE OPTIMIZATION**: Common cache hit rate calculation
func CalculateCacheHitRate(hits, total int) float64 {
	if total == 0 {
		return 0.0
	}
	return float64(hits) / float64(total)
}

// CalculateSuccessRate calculates success rate
// **ARCHITECTURE IMPROVEMENT**: Common success rate calculation
func CalculateSuccessRate(successful, total int) float64 {
	if total == 0 {
		return 0.0
	}
	return float64(successful) / float64(total)
}

// CalculateErrorRate calculates error rate
func CalculateErrorRate(errors, total int) float64 {
	if total == 0 {
		return 0.0
	}
	return float64(errors) / float64(total)
}

// ProcessingMetrics represents common processing metrics
type ProcessingMetrics struct {
	StartTime       time.Time     `json:"start_time"`
	ProcessingTime  time.Duration `json:"processing_time"`
	SuccessRate     float64       `json:"success_rate"`
	ErrorRate       float64       `json:"error_rate"`
	CacheHitRate    float64       `json:"cache_hit_rate"`
	PerformanceTier string        `json:"performance_tier"`
	BusinessValue   float64       `json:"business_value"`
}

// CalculateProcessingMetrics calculates comprehensive processing metrics
// **CODE QUALITY**: Common metrics calculation pattern
func CalculateProcessingMetrics(startTime time.Time, successful, errors, cacheHits, total int) *ProcessingMetrics {
	processingTime := time.Since(startTime)

	return &ProcessingMetrics{
		StartTime:       startTime,
		ProcessingTime:  processingTime,
		SuccessRate:     CalculateSuccessRate(successful, total),
		ErrorRate:       CalculateErrorRate(errors, total),
		CacheHitRate:    CalculateCacheHitRate(cacheHits, total),
		PerformanceTier: string(ClassifyPerformance(processingTime)),
		BusinessValue: CalculateBusinessValue(
			1.0-CalculateErrorRate(errors, total),            // Cost savings from low error rate
			CalculateSuccessRate(successful, total),          // Quality score
			1.0-float64(processingTime)/float64(time.Second), // Efficiency score (inverse of time)
		),
	}
}

