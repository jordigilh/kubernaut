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

package monitoring

import (
	"math"
	"sort"
	"time"
)

// MetricAggregationResult represents the result of metric aggregation calculations
type MetricAggregationResult struct {
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	StdDev float64 `json:"std_dev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Count  int     `json:"count"`
	Sum    float64 `json:"sum"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
}

// ThresholdAnalysisResult represents threshold evaluation results
type ThresholdAnalysisResult struct {
	IsViolated        bool    `json:"is_violated"`
	ViolationSeverity string  `json:"violation_severity"`
	ViolationLevel    float64 `json:"violation_level"`
	ThresholdType     string  `json:"threshold_type"`
	RecommendedAction string  `json:"recommended_action"`
	ConfidenceScore   float64 `json:"confidence_score"`
}

// PerformanceScoreResult represents performance calculation results
type PerformanceScoreResult struct {
	OverallScore        float64            `json:"overall_score"`
	ComponentScores     map[string]float64 `json:"component_scores"`
	PerformanceGrade    string             `json:"performance_grade"`
	ImprovementAreas    []string           `json:"improvement_areas"`
	BenchmarkComparison float64            `json:"benchmark_comparison"`
}

// Monitoring Algorithm Functions (BR-MON-001 to BR-MON-015)
// These functions implement algorithmic logic for metrics processing and analysis

// AggregateMetrics performs statistical aggregation of metric values
// BR-MON-001: Metric aggregation algorithms
func AggregateMetrics(values []float64) *MetricAggregationResult {
	if len(values) == 0 {
		return &MetricAggregationResult{}
	}

	// Sort values for percentile calculations
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	// Calculate basic statistics
	sum := 0.0
	for _, v := range values {
		sum += v
	}

	mean := sum / float64(len(values))
	min := sorted[0]
	max := sorted[len(sorted)-1]

	// Calculate median
	var median float64
	n := len(sorted)
	if n%2 == 0 {
		median = (sorted[n/2-1] + sorted[n/2]) / 2
	} else {
		median = sorted[n/2]
	}

	// Calculate standard deviation
	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(len(values)))

	// Calculate percentiles using linear interpolation
	p95 := calculatePercentile(sorted, 0.95)
	p99 := calculatePercentile(sorted, 0.99)

	return &MetricAggregationResult{
		Mean:   mean,
		Median: median,
		StdDev: stdDev,
		Min:    min,
		Max:    max,
		Count:  len(values),
		Sum:    sum,
		P95:    p95,
		P99:    p99,
	}
}

// EvaluateThreshold analyzes metric values against defined thresholds
// BR-MON-002: Threshold evaluation logic
func EvaluateThreshold(currentValue, warningThreshold, criticalThreshold float64, thresholdType string) *ThresholdAnalysisResult {
	var isViolated bool
	var severity string
	var violationLevel float64
	var recommendedAction string
	var confidenceScore float64

	switch thresholdType {
	case "upper":
		if currentValue >= criticalThreshold {
			isViolated = true
			severity = "critical"
			violationLevel = (currentValue - criticalThreshold) / criticalThreshold
			recommendedAction = "immediate_intervention"
			confidenceScore = 0.95
		} else if currentValue >= warningThreshold {
			isViolated = true
			severity = "warning"
			violationLevel = (currentValue - warningThreshold) / warningThreshold
			recommendedAction = "monitor_closely"
			confidenceScore = 0.85
		} else {
			violationLevel = currentValue / warningThreshold
			recommendedAction = "continue_monitoring"
			confidenceScore = 0.75
		}
	case "lower":
		if currentValue <= criticalThreshold {
			isViolated = true
			severity = "critical"
			violationLevel = (criticalThreshold - currentValue) / criticalThreshold
			recommendedAction = "immediate_intervention"
			confidenceScore = 0.95
		} else if currentValue <= warningThreshold {
			isViolated = true
			severity = "warning"
			violationLevel = (warningThreshold - currentValue) / warningThreshold
			recommendedAction = "monitor_closely"
			confidenceScore = 0.85
		} else {
			violationLevel = currentValue / warningThreshold
			recommendedAction = "continue_monitoring"
			confidenceScore = 0.75
		}
	}

	return &ThresholdAnalysisResult{
		IsViolated:        isViolated,
		ViolationSeverity: severity,
		ViolationLevel:    violationLevel,
		ThresholdType:     thresholdType,
		RecommendedAction: recommendedAction,
		ConfidenceScore:   confidenceScore,
	}
}

// CalculatePerformanceScore computes performance scores based on multiple metrics
// BR-MON-003: Performance calculation algorithms
func CalculatePerformanceScore(metrics map[string]float64, weights map[string]float64, benchmarks map[string]float64) *PerformanceScoreResult {
	if len(metrics) == 0 {
		return &PerformanceScoreResult{OverallScore: 0.0}
	}

	componentScores := make(map[string]float64)
	totalWeightedScore := 0.0
	totalWeight := 0.0
	improvementAreas := []string{}

	// Calculate component scores
	for metric, value := range metrics {
		weight := 1.0
		if w, exists := weights[metric]; exists {
			weight = w
		}

		benchmark := 100.0 // Default benchmark
		if b, exists := benchmarks[metric]; exists {
			benchmark = b
		}

		// Normalize score to 0-100 scale
		score := (value / benchmark) * 100
		if score > 100 {
			score = 100
		}

		componentScores[metric] = score
		totalWeightedScore += score * weight
		totalWeight += weight

		// Identify improvement areas (scores below 70)
		if score < 70 {
			improvementAreas = append(improvementAreas, metric)
		}
	}

	// Calculate overall score
	overallScore := totalWeightedScore / totalWeight
	if overallScore > 100 {
		overallScore = 100
	}

	// Determine performance grade
	var grade string
	switch {
	case overallScore >= 90:
		grade = "excellent"
	case overallScore >= 80:
		grade = "good"
	case overallScore >= 70:
		grade = "satisfactory"
	case overallScore >= 60:
		grade = "needs_improvement"
	default:
		grade = "poor"
	}

	// Calculate benchmark comparison (average deviation from benchmarks)
	benchmarkComparison := 0.0
	benchmarkCount := 0
	for metric, value := range metrics {
		if benchmark, exists := benchmarks[metric]; exists {
			deviation := ((value - benchmark) / benchmark) * 100
			benchmarkComparison += deviation
			benchmarkCount++
		}
	}
	if benchmarkCount > 0 {
		benchmarkComparison = benchmarkComparison / float64(benchmarkCount)
	}

	return &PerformanceScoreResult{
		OverallScore:        overallScore,
		ComponentScores:     componentScores,
		PerformanceGrade:    grade,
		ImprovementAreas:    improvementAreas,
		BenchmarkComparison: benchmarkComparison,
	}
}

// AnalyzeTrend performs trend analysis on time-series metric data
// BR-MON-004: Trend analysis algorithms
func AnalyzeTrend(values []float64, timestamps []time.Time) *TrendAnalysisResult {
	if len(values) == 0 || len(values) != len(timestamps) {
		return &TrendAnalysisResult{
			Direction: "unknown",
			Slope:     0.0,
			Strength:  0.0,
		}
	}

	// Convert timestamps to float64 for linear regression
	x := make([]float64, len(timestamps))
	for i, t := range timestamps {
		x[i] = float64(t.Unix())
	}

	// Calculate linear regression slope
	n := float64(len(values))
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumXX := 0.0

	for i := 0; i < len(values); i++ {
		sumX += x[i]
		sumY += values[i]
		sumXY += x[i] * values[i]
		sumXX += x[i] * x[i]
	}

	slope := (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)

	// Calculate correlation coefficient (R)
	meanX := sumX / n
	meanY := sumY / n

	numerator := 0.0
	denomX := 0.0
	denomY := 0.0

	for i := 0; i < len(values); i++ {
		dx := x[i] - meanX
		dy := values[i] - meanY
		numerator += dx * dy
		denomX += dx * dx
		denomY += dy * dy
	}

	correlation := numerator / math.Sqrt(denomX*denomY)
	strength := math.Abs(correlation)

	// Determine trend direction
	var direction string
	if math.Abs(slope) < 1e-10 {
		direction = "stable"
	} else if slope > 0 {
		direction = "increasing"
	} else {
		direction = "decreasing"
	}

	// Calculate volatility
	volatility := 0.0
	if len(values) > 1 {
		for i := 1; i < len(values); i++ {
			change := math.Abs(values[i] - values[i-1])
			volatility += change
		}
		volatility = volatility / float64(len(values)-1)
	}

	return &TrendAnalysisResult{
		Direction:   direction,
		Slope:       slope,
		Strength:    strength,
		Correlation: correlation,
		Volatility:  volatility,
	}
}

// TrendAnalysisResult represents trend analysis results
type TrendAnalysisResult struct {
	Direction   string  `json:"direction"`
	Slope       float64 `json:"slope"`
	Strength    float64 `json:"strength"`
	Correlation float64 `json:"correlation"`
	Volatility  float64 `json:"volatility"`
}

// DetectAnomalies identifies anomalous metric values using statistical methods
// BR-MON-005: Anomaly detection algorithms
func DetectAnomalies(values []float64, sensitivityThreshold float64) *AnomalyDetectionResult {
	if len(values) < 3 {
		return &AnomalyDetectionResult{
			AnomalousIndices: []int{},
			AnomalyScores:    []float64{},
		}
	}

	aggregation := AggregateMetrics(values)
	mean := aggregation.Mean
	stdDev := aggregation.StdDev

	var anomalousIndices []int
	var anomalyScores []float64

	// Use Z-score method for anomaly detection
	for i, value := range values {
		zScore := math.Abs((value - mean) / stdDev)
		anomalyScore := zScore / 3.0 // Normalize to 0-1 scale

		if zScore > sensitivityThreshold {
			anomalousIndices = append(anomalousIndices, i)
		}
		anomalyScores = append(anomalyScores, anomalyScore)
	}

	return &AnomalyDetectionResult{
		AnomalousIndices: anomalousIndices,
		AnomalyScores:    anomalyScores,
		Mean:             mean,
		StdDev:           stdDev,
		Threshold:        sensitivityThreshold,
	}
}

// AnomalyDetectionResult represents anomaly detection results
type AnomalyDetectionResult struct {
	AnomalousIndices []int     `json:"anomalous_indices"`
	AnomalyScores    []float64 `json:"anomaly_scores"`
	Mean             float64   `json:"mean"`
	StdDev           float64   `json:"std_dev"`
	Threshold        float64   `json:"threshold"`
}

// calculatePercentile calculates the percentile value using linear interpolation
func calculatePercentile(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0.0
	}
	if len(sortedValues) == 1 {
		return sortedValues[0]
	}

	// Calculate the index for the percentile
	index := percentile * float64(len(sortedValues)-1)
	lowerIndex := int(math.Floor(index))
	upperIndex := int(math.Ceil(index))

	if lowerIndex == upperIndex {
		return sortedValues[lowerIndex]
	}

	// Linear interpolation between the two adjacent values
	weight := index - float64(lowerIndex)
	return sortedValues[lowerIndex]*(1-weight) + sortedValues[upperIndex]*weight
}
