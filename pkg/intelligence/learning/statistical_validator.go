package learning

import (
	"fmt"
	"math"
	"sort"
	"time"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/stat"
)

// PatternDiscoveryConfig provides configuration for pattern discovery
type PatternDiscoveryConfig struct {
	MinExecutionsForPattern int `yaml:"min_executions_for_pattern" default:"10"`
	MaxHistoryDays          int `yaml:"max_history_days" default:"30"`
}

// StatisticalValidator provides validation for statistical assumptions in ML models
type StatisticalValidator struct {
	log    *logrus.Logger
	config *PatternDiscoveryConfig
}

// StatisticalAssumptionResult contains the result of statistical assumption validation
type StatisticalAssumptionResult struct {
	IsValid               bool               `json:"is_valid"`
	Assumptions           []*AssumptionCheck `json:"assumptions"`
	Recommendations       []string           `json:"recommendations"`
	SampleSizeAdequate    bool               `json:"sample_size_adequate"`
	MinRecommendedSamples int                `json:"min_recommended_samples"`
	DataQualityScore      float64            `json:"data_quality_score"`
	TemporalConsistency   float64            `json:"temporal_consistency"`
	OverallReliability    float64            `json:"overall_reliability"`
}

// AssumptionCheck represents a single statistical assumption check
type AssumptionCheck struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Passed      bool    `json:"passed"`
	Score       float64 `json:"score"`
	PValue      float64 `json:"p_value,omitempty"`
	Statistic   float64 `json:"statistic,omitempty"`
	Threshold   float64 `json:"threshold"`
	Details     string  `json:"details"`
}

// StatisticalConfidenceInterval represents a statistical confidence interval
type StatisticalConfidenceInterval struct {
	Lower           float64 `json:"lower"`
	Upper           float64 `json:"upper"`
	ConfidenceLevel float64 `json:"confidence_level"`
	IsReliable      bool    `json:"is_reliable"`
	SampleSize      int     `json:"sample_size"`
	Method          string  `json:"method"`
}

// ReliabilityAssessment assesses the reliability of ML predictions
type ReliabilityAssessment struct {
	IsReliable          bool     `json:"is_reliable"`
	ReliabilityScore    float64  `json:"reliability_score"`
	RecommendedMinSize  int      `json:"recommended_min_size"`
	ActualSize          int      `json:"actual_size"`
	DataQuality         float64  `json:"data_quality"`
	TemporalStability   float64  `json:"temporal_stability"`
	StatisticalValidity float64  `json:"statistical_validity"`
	Recommendations     []string `json:"recommendations"`
}

// NewStatisticalValidator creates a new statistical validator
func NewStatisticalValidator(config *PatternDiscoveryConfig, log *logrus.Logger) *StatisticalValidator {
	return &StatisticalValidator{
		config: config,
		log:    log,
	}
}

// ValidateStatisticalAssumptions validates key statistical assumptions for ML models
func (sv *StatisticalValidator) ValidateStatisticalAssumptions(data []*sharedtypes.WorkflowExecutionData) *StatisticalAssumptionResult {
	result := &StatisticalAssumptionResult{
		IsValid:            true,
		Assumptions:        make([]*AssumptionCheck, 0),
		Recommendations:    make([]string, 0),
		SampleSizeAdequate: len(data) >= sv.config.MinExecutionsForPattern*2,
	}

	// 1. Check sample size adequacy
	sampleSizeCheck := sv.checkSampleSizeAdequacy(len(data))
	result.Assumptions = append(result.Assumptions, sampleSizeCheck)
	if !sampleSizeCheck.Passed {
		result.IsValid = false
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Increase sample size to at least %d for reliable analysis", int(sampleSizeCheck.Threshold)))
	}

	// 2. Check data distribution normality (for success rates)
	normalityCheck := sv.checkNormality(data)
	result.Assumptions = append(result.Assumptions, normalityCheck)
	if !normalityCheck.Passed {
		result.Recommendations = append(result.Recommendations,
			"Data distribution is non-normal; consider using non-parametric methods")
	}

	// 3. Check temporal independence
	independenceCheck := sv.checkTemporalIndependence(data)
	result.Assumptions = append(result.Assumptions, independenceCheck)
	if !independenceCheck.Passed {
		result.IsValid = false
		result.Recommendations = append(result.Recommendations,
			"Strong temporal correlation detected; consider time series methods")
	}

	// 4. Check variance homogeneity
	varianceCheck := sv.checkVarianceHomogeneity(data)
	result.Assumptions = append(result.Assumptions, varianceCheck)
	if !varianceCheck.Passed {
		result.Recommendations = append(result.Recommendations,
			"Unequal variance detected; consider variance stabilization")
	}

	// 5. Check for outliers
	outlierCheck := sv.checkForOutliers(data)
	result.Assumptions = append(result.Assumptions, outlierCheck)
	if !outlierCheck.Passed {
		result.Recommendations = append(result.Recommendations,
			"Significant outliers detected; consider robust estimation methods")
	}

	// Calculate overall scores
	result.DataQualityScore = sv.calculateDataQuality(data)
	result.TemporalConsistency = independenceCheck.Score
	result.MinRecommendedSamples = int(sampleSizeCheck.Threshold)

	// Calculate overall reliability
	totalScore := 0.0
	for _, assumption := range result.Assumptions {
		if assumption.Passed {
			totalScore += assumption.Score
		} else {
			totalScore += assumption.Score * 0.5 // Penalty for failed assumptions
		}
	}
	result.OverallReliability = totalScore / float64(len(result.Assumptions))

	sv.log.WithFields(logrus.Fields{
		"sample_size":         len(data),
		"assumptions_passed":  sv.countPassedAssumptions(result.Assumptions),
		"overall_reliability": result.OverallReliability,
		"is_valid":            result.IsValid,
	}).Info("Statistical assumption validation completed")

	return result
}

// CalculateConfidenceInterval calculates Wilson score confidence interval
func (sv *StatisticalValidator) CalculateConfidenceInterval(successes, total int, confidenceLevel float64) *StatisticalConfidenceInterval {
	if total < 5 {
		return &StatisticalConfidenceInterval{
			Lower:           0.0,
			Upper:           1.0,
			ConfidenceLevel: confidenceLevel,
			IsReliable:      false,
			SampleSize:      total,
			Method:          "insufficient_data",
		}
	}

	p := float64(successes) / float64(total)
	n := float64(total)

	// Get z-score for confidence level
	z := sv.getZScore(confidenceLevel)

	// Wilson score interval (more robust than normal approximation)
	denominator := 1 + (z*z)/n
	center := p + (z*z)/(2*n)
	halfWidth := z * math.Sqrt(p*(1-p)/n+(z*z)/(4*n*n))

	lower := (center - halfWidth/denominator) / denominator
	upper := (center + halfWidth/denominator) / denominator

	// Ensure bounds
	lower = math.Max(0.0, lower)
	upper = math.Min(1.0, upper)

	return &StatisticalConfidenceInterval{
		Lower:           lower,
		Upper:           upper,
		ConfidenceLevel: confidenceLevel,
		IsReliable:      total >= 20, // Rule of thumb: need at least 20 samples
		SampleSize:      total,
		Method:          "wilson_score",
	}
}

// AssessReliability provides a comprehensive reliability assessment
func (sv *StatisticalValidator) AssessReliability(data []*sharedtypes.WorkflowExecutionData) *ReliabilityAssessment {
	assessment := &ReliabilityAssessment{
		ActualSize:      len(data),
		Recommendations: make([]string, 0),
	}

	// Minimum recommended size based on statistical power analysis
	assessment.RecommendedMinSize = sv.calculateMinSampleSize(0.8, 0.05) // 80% power, 5% alpha

	// Check if we have sufficient data
	assessment.IsReliable = len(data) >= assessment.RecommendedMinSize

	// Assess data quality
	assessment.DataQuality = sv.calculateDataQuality(data)

	// Assess temporal stability
	assessment.TemporalStability = sv.calculateTemporalStability(data)

	// Validate statistical assumptions
	assumptions := sv.ValidateStatisticalAssumptions(data)
	assessment.StatisticalValidity = assumptions.OverallReliability

	// Calculate overall reliability score
	weights := map[string]float64{
		"sample_size":          0.3,
		"data_quality":         0.3,
		"temporal_stability":   0.2,
		"statistical_validity": 0.2,
	}

	sampleSizeScore := math.Min(1.0, float64(len(data))/float64(assessment.RecommendedMinSize))

	assessment.ReliabilityScore =
		weights["sample_size"]*sampleSizeScore +
			weights["data_quality"]*assessment.DataQuality +
			weights["temporal_stability"]*assessment.TemporalStability +
			weights["statistical_validity"]*assessment.StatisticalValidity

	// Generate recommendations
	if !assessment.IsReliable {
		assessment.Recommendations = append(assessment.Recommendations,
			fmt.Sprintf("Increase sample size from %d to at least %d", len(data), assessment.RecommendedMinSize))
	}

	if assessment.DataQuality < 0.7 {
		assessment.Recommendations = append(assessment.Recommendations,
			"Improve data quality by reducing missing values and inconsistencies")
	}

	if assessment.TemporalStability < 0.6 {
		assessment.Recommendations = append(assessment.Recommendations,
			"Data shows temporal instability; consider shorter analysis windows")
	}

	if assessment.StatisticalValidity < 0.7 {
		assessment.Recommendations = append(assessment.Recommendations,
			"Statistical assumptions violated; consider alternative methods")
	}

	return assessment
}

// Private helper methods

func (sv *StatisticalValidator) checkSampleSizeAdequacy(sampleSize int) *AssumptionCheck {
	minSize := sv.config.MinExecutionsForPattern * 2 // At least 2x minimum for ML
	threshold := float64(minSize)

	passed := sampleSize >= minSize
	score := math.Min(1.0, float64(sampleSize)/threshold)

	return &AssumptionCheck{
		Name:        "sample_size_adequacy",
		Description: "Adequate sample size for reliable ML training",
		Passed:      passed,
		Score:       score,
		Threshold:   threshold,
		Details:     fmt.Sprintf("Sample size: %d, Required: %d", sampleSize, minSize),
	}
}

func (sv *StatisticalValidator) checkNormality(data []*sharedtypes.WorkflowExecutionData) *AssumptionCheck {
	if len(data) < 8 {
		return &AssumptionCheck{
			Name:        "normality",
			Description: "Data distribution normality",
			Passed:      false,
			Score:       0.5,
			Details:     "Insufficient data for normality test",
		}
	}

	// Extract success rates over time windows
	values := sv.extractSuccessRatesOverTime(data, time.Hour*6)

	if len(values) < 3 {
		return &AssumptionCheck{
			Name:        "normality",
			Description: "Data distribution normality",
			Passed:      true, // Assume normal if too few windows
			Score:       0.7,
			Details:     "Too few time windows for normality assessment",
		}
	}

	// Simplified Shapiro-Wilk like test
	mean := stat.Mean(values, nil)
	variance := stat.Variance(values, nil)

	// Calculate skewness and kurtosis
	skewness := sv.calculateSkewness(values, mean, variance)
	kurtosis := sv.calculateKurtosis(values, mean, variance)

	// Rough normality assessment
	normalityScore := 1.0 - (math.Abs(skewness)/3.0 + math.Abs(kurtosis-3.0)/4.0)
	normalityScore = math.Max(0.0, normalityScore)

	passed := normalityScore > 0.6 && math.Abs(skewness) < 2.0 && math.Abs(kurtosis-3.0) < 3.0

	return &AssumptionCheck{
		Name:        "normality",
		Description: "Data distribution normality",
		Passed:      passed,
		Score:       normalityScore,
		Statistic:   skewness,
		Threshold:   2.0,
		Details:     fmt.Sprintf("Skewness: %.3f, Kurtosis: %.3f", skewness, kurtosis),
	}
}

func (sv *StatisticalValidator) checkTemporalIndependence(data []*sharedtypes.WorkflowExecutionData) *AssumptionCheck {
	if len(data) < 10 {
		return &AssumptionCheck{
			Name:        "temporal_independence",
			Description: "Temporal independence of observations",
			Passed:      true,
			Score:       0.8,
			Details:     "Insufficient data for autocorrelation test",
		}
	}

	// Calculate autocorrelation at lag 1
	successValues := make([]float64, len(data))
	for i, d := range data {
		if d.Success {
			successValues[i] = 1.0
		} else {
			successValues[i] = 0.0
		}
	}

	// Simple lag-1 autocorrelation
	autocorr := sv.calculateAutocorrelation(successValues, 1)

	// Critical value for independence (approximate)
	criticalValue := 1.96 / math.Sqrt(float64(len(data))) // 95% confidence

	passed := math.Abs(autocorr) < criticalValue
	score := 1.0 - math.Min(1.0, math.Abs(autocorr)/0.5) // Penalize high correlation

	return &AssumptionCheck{
		Name:        "temporal_independence",
		Description: "Temporal independence of observations",
		Passed:      passed,
		Score:       score,
		Statistic:   autocorr,
		Threshold:   criticalValue,
		Details:     fmt.Sprintf("Lag-1 autocorrelation: %.3f, Critical: %.3f", autocorr, criticalValue),
	}
}

func (sv *StatisticalValidator) checkVarianceHomogeneity(data []*sharedtypes.WorkflowExecutionData) *AssumptionCheck {
	// Group data by success/failure and check variance equality
	successTimes := make([]float64, 0)
	failureTimes := make([]float64, 0)

	for _, d := range data {
		duration := d.Duration.Seconds()
		if d.Success {
			successTimes = append(successTimes, duration)
		} else {
			failureTimes = append(failureTimes, duration)
		}
	}

	if len(successTimes) < 3 || len(failureTimes) < 3 {
		return &AssumptionCheck{
			Name:        "variance_homogeneity",
			Description: "Equal variance between groups",
			Passed:      true,
			Score:       0.8,
			Details:     "Insufficient data for variance test",
		}
	}

	successVar := stat.Variance(successTimes, nil)
	failureVar := stat.Variance(failureTimes, nil)

	// F-ratio test (simplified)
	fRatio := math.Max(successVar, failureVar) / math.Min(successVar, failureVar)

	// Approximate critical value for F-test
	criticalF := 2.5 // Approximate for moderate sample sizes

	passed := fRatio < criticalF
	score := 1.0 - math.Min(1.0, (fRatio-1.0)/4.0) // Normalize score

	return &AssumptionCheck{
		Name:        "variance_homogeneity",
		Description: "Equal variance between success/failure groups",
		Passed:      passed,
		Score:       score,
		Statistic:   fRatio,
		Threshold:   criticalF,
		Details:     fmt.Sprintf("F-ratio: %.3f, Success var: %.3f, Failure var: %.3f", fRatio, successVar, failureVar),
	}
}

func (sv *StatisticalValidator) checkForOutliers(data []*sharedtypes.WorkflowExecutionData) *AssumptionCheck {
	if len(data) < 10 {
		return &AssumptionCheck{
			Name:        "outlier_detection",
			Description: "Absence of extreme outliers",
			Passed:      true,
			Score:       0.9,
			Details:     "Insufficient data for outlier detection",
		}
	}

	// Extract execution durations
	durations := make([]float64, 0)
	for _, d := range data {
		durations = append(durations, d.Duration.Seconds())
	}

	if len(durations) < 5 {
		return &AssumptionCheck{
			Name:        "outlier_detection",
			Description: "Absence of extreme outliers",
			Passed:      true,
			Score:       0.9,
			Details:     "Insufficient duration data",
		}
	}

	// IQR method for outlier detection
	sort.Float64s(durations)
	q1 := sv.percentile(durations, 25)
	q3 := sv.percentile(durations, 75)
	iqr := q3 - q1

	lowerBound := q1 - 1.5*iqr
	upperBound := q3 + 1.5*iqr

	outlierCount := 0
	for _, d := range durations {
		if d < lowerBound || d > upperBound {
			outlierCount++
		}
	}

	outlierRate := float64(outlierCount) / float64(len(durations))
	passed := outlierRate < 0.1                   // Less than 10% outliers
	score := 1.0 - math.Min(1.0, outlierRate*5.0) // Penalize high outlier rates

	return &AssumptionCheck{
		Name:        "outlier_detection",
		Description: "Absence of extreme outliers",
		Passed:      passed,
		Score:       score,
		Statistic:   outlierRate,
		Threshold:   0.1,
		Details:     fmt.Sprintf("Outlier rate: %.1f%% (%d/%d)", outlierRate*100, outlierCount, len(durations)),
	}
}

// Additional helper methods

func (sv *StatisticalValidator) calculateDataQuality(data []*sharedtypes.WorkflowExecutionData) float64 {
	if len(data) == 0 {
		return 0.0
	}

	qualityScore := 0.0
	validCount := 0

	for _, d := range data {
		score := 0.0

		// Check for required fields
		if d.ExecutionID != "" {
			score += 0.25
		}
		if !d.Timestamp.IsZero() {
			score += 0.25
		}
		if d.WorkflowID != "" {
			score += 0.25
		}
		if d.Duration > 0 {
			score += 0.25
		}

		qualityScore += score
		validCount++
	}

	if validCount == 0 {
		return 0.0
	}

	return qualityScore / float64(validCount)
}

func (sv *StatisticalValidator) calculateTemporalStability(data []*sharedtypes.WorkflowExecutionData) float64 {
	if len(data) < 6 {
		return 0.5 // Neutral score for insufficient data
	}

	// Calculate success rates in time windows
	windowSize := time.Hour * 12
	windows := sv.extractSuccessRatesOverTime(data, windowSize)

	if len(windows) < 3 {
		return 0.7 // Assume stable if few windows
	}

	// Calculate coefficient of variation (CV)
	mean := stat.Mean(windows, nil)
	variance := stat.Variance(windows, nil)

	if mean == 0 {
		return 0.0
	}

	cv := math.Sqrt(variance) / mean

	// Convert CV to stability score (lower CV = higher stability)
	stability := 1.0 / (1.0 + cv*2.0)
	return math.Min(1.0, stability)
}

func (sv *StatisticalValidator) calculateMinSampleSize(power, alpha float64) int {
	// Simplified sample size calculation for proportion test
	// Using Cohen's conventions for small, medium, large effect sizes

	effectSize := 0.2 // Small effect size (Cohen's h)
	zAlpha := sv.getZScore(1.0 - alpha/2.0)
	zBeta := sv.getZScore(power)

	// Guideline #14: Optimize math operations - expand math.Pow(x, 2) to x*x for performance
	n := (zAlpha + zBeta) * (zAlpha + zBeta) / (effectSize * effectSize)

	// Minimum practical size
	return int(math.Max(30, math.Ceil(n)))
}

// Statistical utility functions

func (sv *StatisticalValidator) getZScore(confidenceLevel float64) float64 {
	switch confidenceLevel {
	case 0.90:
		return 1.645
	case 0.95:
		return 1.96
	case 0.99:
		return 2.576
	case 0.80:
		return 1.282
	default:
		return 1.96 // Default to 95%
	}
}

func (sv *StatisticalValidator) extractSuccessRatesOverTime(data []*sharedtypes.WorkflowExecutionData, windowSize time.Duration) []float64 {
	if len(data) == 0 {
		return []float64{}
	}

	// Sort by timestamp
	sortedData := make([]*sharedtypes.WorkflowExecutionData, len(data))
	copy(sortedData, data)
	sort.Slice(sortedData, func(i, j int) bool {
		return sortedData[i].Timestamp.Before(sortedData[j].Timestamp)
	})

	rates := make([]float64, 0)
	windowStart := sortedData[0].Timestamp

	for {
		windowEnd := windowStart.Add(windowSize)
		successCount := 0
		totalCount := 0

		for _, d := range sortedData {
			if d.Timestamp.After(windowEnd) {
				break
			}
			if !d.Timestamp.Before(windowStart) {
				totalCount++
				if d.Success {
					successCount++
				}
			}
		}

		if totalCount > 0 {
			rates = append(rates, float64(successCount)/float64(totalCount))
			windowStart = windowEnd
		} else {
			break
		}

		// Prevent infinite loop
		if windowStart.After(sortedData[len(sortedData)-1].Timestamp) {
			break
		}
	}

	return rates
}

func (sv *StatisticalValidator) calculateSkewness(values []float64, mean, variance float64) float64 {
	if len(values) < 3 || variance == 0 {
		return 0.0
	}

	sum := 0.0
	stdDev := math.Sqrt(variance)

	for _, v := range values {
		// Guideline #14: Optimize math operations - manual cube calculation for clarity
		normalized := (v - mean) / stdDev
		sum += normalized * normalized * normalized
	}

	return sum / float64(len(values))
}

func (sv *StatisticalValidator) calculateKurtosis(values []float64, mean, variance float64) float64 {
	if len(values) < 4 || variance == 0 {
		return 3.0 // Normal kurtosis
	}

	sum := 0.0
	stdDev := math.Sqrt(variance)

	for _, v := range values {
		sum += math.Pow((v-mean)/stdDev, 4)
	}

	return sum / float64(len(values))
}

func (sv *StatisticalValidator) calculateAutocorrelation(values []float64, lag int) float64 {
	if len(values) <= lag {
		return 0.0
	}

	n := len(values) - lag
	mean := stat.Mean(values, nil)

	numerator := 0.0
	denominator := 0.0

	for i := 0; i < n; i++ {
		numerator += (values[i] - mean) * (values[i+lag] - mean)
	}

	for i := 0; i < len(values); i++ {
		denominator += (values[i] - mean) * (values[i] - mean)
	}

	if denominator == 0 {
		return 0.0
	}

	return numerator / denominator
}

func (sv *StatisticalValidator) percentile(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0.0
	}

	index := percentile / 100.0 * float64(len(sortedValues)-1)
	lower := int(index)
	upper := lower + 1

	if upper >= len(sortedValues) {
		return sortedValues[len(sortedValues)-1]
	}

	weight := index - float64(lower)
	return sortedValues[lower]*(1-weight) + sortedValues[upper]*weight
}

func (sv *StatisticalValidator) countPassedAssumptions(assumptions []*AssumptionCheck) int {
	count := 0
	for _, assumption := range assumptions {
		if assumption.Passed {
			count++
		}
	}
	return count
}
