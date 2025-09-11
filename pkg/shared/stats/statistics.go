package stats

import (
	"math"
	"sort"
)

// StatisticalUtils provides common statistical calculations used across the codebase
type StatisticalUtils struct{}

// NewStatisticalUtils creates a new instance of statistical utilities
func NewStatisticalUtils() *StatisticalUtils {
	return &StatisticalUtils{}
}

// CalculateMean calculates the arithmetic mean of a slice of float64 values
func (su *StatisticalUtils) CalculateMean(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// CalculateMedian calculates the median of a slice of float64 values
func (su *StatisticalUtils) CalculateMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}

	// Create a copy and sort it
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// CalculateStandardDeviation calculates the standard deviation of a slice of float64 values
func (su *StatisticalUtils) CalculateStandardDeviation(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}

	mean := su.CalculateMean(values)
	sum := 0.0
	for _, v := range values {
		sum += (v - mean) * (v - mean)
	}
	variance := sum / float64(len(values))
	return math.Sqrt(variance)
}

// CalculateSuccessRate calculates the success rate based on a threshold (default 0.7)
func (su *StatisticalUtils) CalculateSuccessRate(scores []float64, threshold float64) float64 {
	if len(scores) == 0 {
		return 0.0
	}

	if threshold <= 0 {
		threshold = 0.7 // Default success threshold
	}

	successful := 0
	for _, score := range scores {
		if score >= threshold {
			successful++
		}
	}
	return float64(successful) / float64(len(scores))
}

// CalculateConfidenceLevel calculates confidence level based on sample size
func (su *StatisticalUtils) CalculateConfidenceLevel(sampleSize int) float64 {
	// Confidence increases with sample size
	switch {
	case sampleSize >= 1000:
		return 0.95
	case sampleSize >= 100:
		return 0.85
	case sampleSize >= 50:
		return 0.75
	case sampleSize >= 20:
		return 0.65
	default:
		return 0.5
	}
}

// CalculatePercentile calculates the nth percentile of a slice of float64 values
func (su *StatisticalUtils) CalculatePercentile(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0.0
	}

	// Create a copy and sort it
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	rank := percentile / 100.0 * float64(n-1)
	lowerIndex := int(math.Floor(rank))
	upperIndex := int(math.Ceil(rank))

	if lowerIndex == upperIndex {
		return sorted[lowerIndex]
	}

	// Linear interpolation
	weight := rank - float64(lowerIndex)
	return sorted[lowerIndex]*(1-weight) + sorted[upperIndex]*weight
}

// StatisticalSummary represents a statistical summary of data
type StatisticalSummary struct {
	Count             int     `json:"count"`
	Mean              float64 `json:"mean"`
	Median            float64 `json:"median"`
	StandardDeviation float64 `json:"standard_deviation"`
	Min               float64 `json:"min"`
	Max               float64 `json:"max"`
	Percentile25      float64 `json:"percentile_25"`
	Percentile75      float64 `json:"percentile_75"`
	SuccessRate       float64 `json:"success_rate"`
	ConfidenceLevel   float64 `json:"confidence_level"`
}

// CalculateFullSummary calculates a comprehensive statistical summary
func (su *StatisticalUtils) CalculateFullSummary(values []float64, successThreshold float64) *StatisticalSummary {
	if len(values) == 0 {
		return &StatisticalSummary{}
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	return &StatisticalSummary{
		Count:             len(values),
		Mean:              su.CalculateMean(values),
		Median:            su.CalculateMedian(values),
		StandardDeviation: su.CalculateStandardDeviation(values),
		Min:               sorted[0],
		Max:               sorted[len(sorted)-1],
		Percentile25:      su.CalculatePercentile(values, 25),
		Percentile75:      su.CalculatePercentile(values, 75),
		SuccessRate:       su.CalculateSuccessRate(values, successThreshold),
		ConfidenceLevel:   su.CalculateConfidenceLevel(len(values)),
	}
}

// TrendDirection calculates the trend direction of a time series
func (su *StatisticalUtils) TrendDirection(values []float64) string {
	if len(values) < 2 {
		return "insufficient_data"
	}

	// Simple linear regression slope calculation
	n := float64(len(values))
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumX2 := 0.0

	for i, y := range values {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)

	switch {
	case slope > 0.01:
		return "improving"
	case slope < -0.01:
		return "declining"
	default:
		return "stable"
	}
}

// CalculateCorrelation calculates the Pearson correlation coefficient between two datasets
func (su *StatisticalUtils) CalculateCorrelation(x, y []float64) float64 {
	if len(x) != len(y) || len(x) == 0 {
		return 0.0
	}

	meanX := su.CalculateMean(x)
	meanY := su.CalculateMean(y)

	var sumXY, sumX2, sumY2 float64
	for i := 0; i < len(x); i++ {
		dx := x[i] - meanX
		dy := y[i] - meanY
		sumXY += dx * dy
		sumX2 += dx * dx
		sumY2 += dy * dy
	}

	denominator := math.Sqrt(sumX2 * sumY2)
	if denominator == 0 {
		return 0.0
	}

	return sumXY / denominator
}

// Global instance for convenience
var DefaultStatsUtil = NewStatisticalUtils()
