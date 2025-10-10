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

package context

import (
	"sync"
	"time"
)

// PerformanceMetrics tracks and analyzes LLM performance metrics
// Supports BR-CONTEXT-040: Key performance indicators tracking
type PerformanceMetrics struct {
	responseQualityHistory []float64
	responseTimeHistory    []time.Duration
	tokenUsageHistory      []int
	contextSizeHistory     []int
	timestampHistory       []time.Time
	mu                     sync.RWMutex
	maxHistorySize         int
}

// MetricsSummary provides statistical summary of performance metrics
type MetricsSummary struct {
	AverageQuality      float64       `json:"average_quality"`
	AverageResponseTime time.Duration `json:"average_response_time"`
	AverageTokenUsage   float64       `json:"average_token_usage"`
	QualityTrend        string        `json:"quality_trend"`
	ResponseTimeTrend   string        `json:"response_time_trend"`
	TokenUsageTrend     string        `json:"token_usage_trend"`
	SampleCount         int           `json:"sample_count"`
	TimeRange           TimeRange     `json:"time_range"`
}

// TimeRange represents a time period for metrics analysis
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// NewPerformanceMetrics creates a new performance metrics tracker
func NewPerformanceMetrics() *PerformanceMetrics {
	return &PerformanceMetrics{
		responseQualityHistory: make([]float64, 0),
		responseTimeHistory:    make([]time.Duration, 0),
		tokenUsageHistory:      make([]int, 0),
		contextSizeHistory:     make([]int, 0),
		timestampHistory:       make([]time.Time, 0),
		maxHistorySize:         1000, // Keep last 1000 measurements
	}
}

// RecordPerformance records a new performance measurement
func (pm *PerformanceMetrics) RecordPerformance(quality float64, responseTime time.Duration, tokenUsage int, contextSize int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Add new measurements
	pm.responseQualityHistory = append(pm.responseQualityHistory, quality)
	pm.responseTimeHistory = append(pm.responseTimeHistory, responseTime)
	pm.tokenUsageHistory = append(pm.tokenUsageHistory, tokenUsage)
	pm.contextSizeHistory = append(pm.contextSizeHistory, contextSize)
	pm.timestampHistory = append(pm.timestampHistory, time.Now())

	// Maintain maximum history size
	if len(pm.responseQualityHistory) > pm.maxHistorySize {
		pm.responseQualityHistory = pm.responseQualityHistory[1:]
		pm.responseTimeHistory = pm.responseTimeHistory[1:]
		pm.tokenUsageHistory = pm.tokenUsageHistory[1:]
		pm.contextSizeHistory = pm.contextSizeHistory[1:]
		pm.timestampHistory = pm.timestampHistory[1:]
	}
}

// GetSummary returns a statistical summary of recent performance
func (pm *PerformanceMetrics) GetSummary(samples int) *MetricsSummary {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if len(pm.responseQualityHistory) == 0 {
		return &MetricsSummary{
			SampleCount: 0,
		}
	}

	// Determine sample range
	totalSamples := len(pm.responseQualityHistory)
	if samples <= 0 || samples > totalSamples {
		samples = totalSamples
	}

	startIdx := totalSamples - samples

	// Calculate averages
	avgQuality := pm.calculateAverage(pm.responseQualityHistory[startIdx:])
	avgResponseTime := pm.calculateAverageTime(pm.responseTimeHistory[startIdx:])
	avgTokenUsage := pm.calculateAverageInt(pm.tokenUsageHistory[startIdx:])

	// Calculate trends
	qualityTrend := pm.calculateTrend(pm.responseQualityHistory[startIdx:])
	responseTrend := pm.calculateTimeTrend(pm.responseTimeHistory[startIdx:])
	tokenTrend := pm.calculateIntTrend(pm.tokenUsageHistory[startIdx:])

	// Time range
	timeRange := TimeRange{
		Start: pm.timestampHistory[startIdx],
		End:   pm.timestampHistory[totalSamples-1],
	}

	return &MetricsSummary{
		AverageQuality:      avgQuality,
		AverageResponseTime: avgResponseTime,
		AverageTokenUsage:   avgTokenUsage,
		QualityTrend:        qualityTrend,
		ResponseTimeTrend:   responseTrend,
		TokenUsageTrend:     tokenTrend,
		SampleCount:         samples,
		TimeRange:           timeRange,
	}
}

// GetCorrelationAnalysis analyzes correlation between context size and performance
func (pm *PerformanceMetrics) GetCorrelationAnalysis() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if len(pm.responseQualityHistory) < 10 {
		return map[string]interface{}{
			"insufficient_data": true,
			"sample_count":      len(pm.responseQualityHistory),
		}
	}

	// Calculate correlations
	qualityContextCorr := pm.calculateCorrelation(pm.contextSizeHistory, pm.responseQualityHistory)
	timeContextCorr := pm.calculateTimeContextCorrelation(pm.contextSizeHistory, pm.responseTimeHistory)
	tokenContextCorr := pm.calculateIntContextCorrelation(pm.contextSizeHistory, pm.tokenUsageHistory)

	return map[string]interface{}{
		"quality_context_correlation": qualityContextCorr,
		"time_context_correlation":    timeContextCorr,
		"token_context_correlation":   tokenContextCorr,
		"sample_count":                len(pm.responseQualityHistory),
		"analysis_confidence":         pm.calculateCorrelationConfidence(),
	}
}

// Helper methods for calculations

func (pm *PerformanceMetrics) calculateAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func (pm *PerformanceMetrics) calculateAverageTime(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}

	sum := time.Duration(0)
	for _, v := range values {
		sum += v
	}
	return sum / time.Duration(len(values))
}

func (pm *PerformanceMetrics) calculateAverageInt(values []int) float64 {
	if len(values) == 0 {
		return 0.0
	}

	sum := 0
	for _, v := range values {
		sum += v
	}
	return float64(sum) / float64(len(values))
}

func (pm *PerformanceMetrics) calculateTrend(values []float64) string {
	if len(values) < 2 {
		return "insufficient_data"
	}

	// Simple trend calculation: compare first half with second half
	mid := len(values) / 2
	firstHalf := pm.calculateAverage(values[:mid])
	secondHalf := pm.calculateAverage(values[mid:])

	diff := secondHalf - firstHalf

	if diff > 0.05 {
		return "improving"
	} else if diff < -0.05 {
		return "declining"
	} else {
		return "stable"
	}
}

func (pm *PerformanceMetrics) calculateTimeTrend(values []time.Duration) string {
	if len(values) < 2 {
		return "insufficient_data"
	}

	// Convert to float64 for calculation
	floatValues := make([]float64, len(values))
	for i, v := range values {
		floatValues[i] = float64(v.Milliseconds())
	}

	mid := len(floatValues) / 2
	firstHalf := pm.calculateAverage(floatValues[:mid])
	secondHalf := pm.calculateAverage(floatValues[mid:])

	diff := secondHalf - firstHalf
	threshold := firstHalf * 0.1 // 10% change threshold

	if diff > threshold {
		return "increasing"
	} else if diff < -threshold {
		return "decreasing"
	} else {
		return "stable"
	}
}

func (pm *PerformanceMetrics) calculateIntTrend(values []int) string {
	if len(values) < 2 {
		return "insufficient_data"
	}

	// Convert to float64 for calculation
	floatValues := make([]float64, len(values))
	for i, v := range values {
		floatValues[i] = float64(v)
	}

	mid := len(floatValues) / 2
	firstHalf := pm.calculateAverage(floatValues[:mid])
	secondHalf := pm.calculateAverage(floatValues[mid:])

	diff := secondHalf - firstHalf
	threshold := firstHalf * 0.1 // 10% change threshold

	if diff > threshold {
		return "increasing"
	} else if diff < -threshold {
		return "decreasing"
	} else {
		return "stable"
	}
}

func (pm *PerformanceMetrics) calculateCorrelation(x []int, y []float64) float64 {
	if len(x) != len(y) || len(x) < 2 {
		return 0.0
	}

	// Convert x to float64
	xFloat := make([]float64, len(x))
	for i, v := range x {
		xFloat[i] = float64(v)
	}

	// Calculate means
	xMean := pm.calculateAverage(xFloat)
	yMean := pm.calculateAverage(y)

	// Calculate correlation coefficient
	numerator := 0.0
	xVar := 0.0
	yVar := 0.0

	for i := 0; i < len(x); i++ {
		xDiff := xFloat[i] - xMean
		yDiff := y[i] - yMean
		numerator += xDiff * yDiff
		xVar += xDiff * xDiff
		yVar += yDiff * yDiff
	}

	if xVar == 0 || yVar == 0 {
		return 0.0
	}

	return numerator / (sqrt(xVar) * sqrt(yVar))
}

func (pm *PerformanceMetrics) calculateTimeContextCorrelation(contextSize []int, responseTime []time.Duration) float64 {
	// Convert time to float64 (milliseconds)
	timeFloat := make([]float64, len(responseTime))
	for i, t := range responseTime {
		timeFloat[i] = float64(t.Milliseconds())
	}

	return pm.calculateCorrelation(contextSize, timeFloat)
}

func (pm *PerformanceMetrics) calculateIntContextCorrelation(contextSize []int, values []int) float64 {
	// Convert values to float64
	valuesFloat := make([]float64, len(values))
	for i, v := range values {
		valuesFloat[i] = float64(v)
	}

	return pm.calculateCorrelation(contextSize, valuesFloat)
}

func (pm *PerformanceMetrics) calculateCorrelationConfidence() float64 {
	sampleCount := len(pm.responseQualityHistory)

	if sampleCount < 10 {
		return 0.1
	} else if sampleCount < 50 {
		return 0.5
	} else if sampleCount < 100 {
		return 0.7
	} else {
		return 0.9
	}
}

// Simple square root implementation
func sqrt(x float64) float64 {
	if x == 0 {
		return 0
	}

	// Newton's method for square root
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}
