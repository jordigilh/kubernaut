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
package patterns

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
)

// PatternConfidenceValidatorSimple provides empirical validation for pattern confidence calculations
// Business Requirement: BR-PATTERN-003 - Pattern confidence validation and accuracy tracking
type PatternConfidenceValidatorSimple struct {
	log               *logrus.Logger
	config            *PatternDiscoveryConfig
	validationHistory map[string]*SimpleConfidenceValidationHistory
}

// SimpleConfidenceValidationHistory tracks historical validation results for patterns
type SimpleConfidenceValidationHistory struct {
	PatternID             string                              `json:"pattern_id"`
	PatternType           shared.PatternType                  `json:"pattern_type"`
	ValidationResults     []*SimpleConfidenceValidationResult `json:"validation_results"`
	LastValidated         time.Time                           `json:"last_validated"`
	TotalValidations      int                                 `json:"total_validations"`
	SuccessfulPredictions int                                 `json:"successful_predictions"`
}

// SimpleConfidenceValidationResult represents a single validation result
type SimpleConfidenceValidationResult struct {
	ValidationID        string    `json:"validation_id"`
	Timestamp           time.Time `json:"timestamp"`
	PredictedConfidence float64   `json:"predicted_confidence"`
	ActualOutcome       bool      `json:"actual_outcome"`
	ValidationMethod    string    `json:"validation_method"`
	SampleSize          int       `json:"sample_size"`
	CalibrationError    float64   `json:"calibration_error"`
	Reliability         float64   `json:"reliability"`
}

// SimpleConfidenceValidationReport provides comprehensive validation results
type SimpleConfidenceValidationReport struct {
	PatternID            string    `json:"pattern_id"`
	ValidationTimestamp  time.Time `json:"validation_timestamp"`
	OverallAccuracy      float64   `json:"overall_accuracy"`
	CalibrationError     float64   `json:"calibration_error"`
	Reliability          float64   `json:"reliability"`
	Recommendations      []string  `json:"recommendations"`
	ConfidenceAdjustment float64   `json:"confidence_adjustment"`
	QualityGrade         string    `json:"quality_grade"`
}

// SimpleCalibrationMetrics provides basic metrics for confidence calibration
type SimpleCalibrationMetrics struct {
	Brier            float64                  `json:"brier_score"`
	Reliability      float64                  `json:"reliability"`
	Resolution       float64                  `json:"resolution"`
	Uncertainty      float64                  `json:"uncertainty"`
	CalibrationCurve []SimpleCalibrationPoint `json:"calibration_curve"`
	OverConfidence   float64                  `json:"over_confidence"`
	UnderConfidence  float64                  `json:"under_confidence"`
}

// SimpleCalibrationPoint represents a point on the calibration curve
type SimpleCalibrationPoint struct {
	PredictedProbability float64 `json:"predicted_probability"`
	ObservedFrequency    float64 `json:"observed_frequency"`
	Count                int     `json:"count"`
	ConfidenceInterval   Range   `json:"confidence_interval"`
}

// NewPatternConfidenceValidatorSimple creates a new confidence validator
func NewPatternConfidenceValidatorSimple(config *PatternDiscoveryConfig, log *logrus.Logger) *PatternConfidenceValidatorSimple {
	return &PatternConfidenceValidatorSimple{
		log:               log,
		config:            config,
		validationHistory: make(map[string]*SimpleConfidenceValidationHistory),
	}
}

// ValidatePatternConfidence performs comprehensive confidence validation
func (pcv *PatternConfidenceValidatorSimple) ValidatePatternConfidence(ctx context.Context, pattern *shared.DiscoveredPattern, historicalData []*sharedtypes.WorkflowExecutionData) (*SimpleConfidenceValidationReport, error) {
	pcv.log.WithFields(logrus.Fields{
		"pattern_id":      pattern.ID,
		"pattern_type":    pattern.Type,
		"raw_confidence":  pattern.Confidence,
		"historical_data": len(historicalData),
	}).Info("Starting empirical confidence validation")

	if len(historicalData) < 10 {
		return nil, fmt.Errorf("insufficient historical data for validation: %d samples", len(historicalData))
	}

	report := &SimpleConfidenceValidationReport{
		PatternID:           pattern.ID,
		ValidationTimestamp: time.Now(),
		Recommendations:     make([]string, 0),
	}

	// Step 1: Calculate basic calibration metrics
	calibrationMetrics, err := pcv.calculateBasicCalibrationMetrics(pattern, historicalData)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate calibration metrics: %w", err)
	}

	report.CalibrationError = calibrationMetrics.Reliability
	report.Reliability = calibrationMetrics.Reliability

	// Step 2: Calculate overall accuracy
	report.OverallAccuracy = pcv.calculatePatternAccuracy(pattern, historicalData)

	// Step 3: Calculate confidence adjustment
	report.ConfidenceAdjustment = pcv.calculateConfidenceAdjustment(pattern, calibrationMetrics)

	// Step 4: Generate quality grade and recommendations
	report.QualityGrade = pcv.calculateQualityGrade(report)
	report.Recommendations = pcv.generateRecommendations(report)

	// Step 5: Update validation history
	pcv.updateValidationHistory(pattern.ID, report)

	pcv.log.WithFields(logrus.Fields{
		"pattern_id":          pattern.ID,
		"original_confidence": pattern.Confidence,
		"adjusted_confidence": pattern.Confidence + report.ConfidenceAdjustment,
		"calibration_error":   report.CalibrationError,
		"quality_grade":       report.QualityGrade,
	}).Info("Confidence validation completed")

	return report, nil
}

// calculateBasicCalibrationMetrics calculates simplified calibration metrics
func (pcv *PatternConfidenceValidatorSimple) calculateBasicCalibrationMetrics(pattern *shared.DiscoveredPattern, data []*sharedtypes.WorkflowExecutionData) (*SimpleCalibrationMetrics, error) {
	// Group predictions by confidence bins
	binCount := 10
	bins := make([][]bool, binCount)
	binConfidences := make([]float64, binCount)

	for i := 0; i < binCount; i++ {
		bins[i] = make([]bool, 0)
		binConfidences[i] = (float64(i) + 0.5) / float64(binCount)
	}

	// Classify each execution into confidence bins
	totalSamples := 0
	totalBrier := 0.0
	overallSuccessCount := 0

	for _, execData := range data {
		if pcv.patternApplies(pattern, execData) {
			predicted := pattern.Confidence
			actual := execData.Success

			if actual {
				overallSuccessCount++
			}
			totalSamples++

			binIndex := int(predicted * float64(binCount))
			if binIndex >= binCount {
				binIndex = binCount - 1
			}
			if binIndex < 0 {
				binIndex = 0
			}

			bins[binIndex] = append(bins[binIndex], actual)

			// Calculate Brier score
			actualFloat := 0.0
			if actual {
				actualFloat = 1.0
			}
			totalBrier += (predicted - actualFloat) * (predicted - actualFloat)
		}
	}

	if totalSamples == 0 {
		return nil, fmt.Errorf("no applicable samples found for pattern")
	}

	overallMean := float64(overallSuccessCount) / float64(totalSamples)

	// Calculate calibration curve and reliability
	calibrationCurve := make([]SimpleCalibrationPoint, 0)
	totalReliability := 0.0
	totalResolution := 0.0

	for i, bin := range bins {
		if len(bin) == 0 {
			continue
		}

		predicted := binConfidences[i]
		observed := pcv.calculateBinSuccessRate(bin)
		count := len(bin)

		// Calculate confidence interval (simplified)
		ci := pcv.calculateSimpleConfidenceInterval(observed, count)

		point := SimpleCalibrationPoint{
			PredictedProbability: predicted,
			ObservedFrequency:    observed,
			Count:                count,
			ConfidenceInterval:   ci,
		}
		calibrationCurve = append(calibrationCurve, point)

		// Accumulate reliability (calibration error) and resolution
		totalReliability += float64(count) * (predicted - observed) * (predicted - observed)
		totalResolution += float64(count) * (observed - overallMean) * (observed - overallMean)
	}

	metrics := &SimpleCalibrationMetrics{
		Brier:            totalBrier / float64(totalSamples),
		Reliability:      math.Sqrt(totalReliability / float64(totalSamples)),
		Resolution:       totalResolution / float64(totalSamples),
		Uncertainty:      overallMean * (1 - overallMean),
		CalibrationCurve: calibrationCurve,
	}

	// Calculate over/under confidence
	metrics.OverConfidence, metrics.UnderConfidence = pcv.calculateConfidenceBias(calibrationCurve)

	return metrics, nil
}

// Helper methods

func (pcv *PatternConfidenceValidatorSimple) patternApplies(pattern *shared.DiscoveredPattern, execData *sharedtypes.WorkflowExecutionData) bool {
	// Simplified pattern applicability check
	// Since Alert and AlertPatterns fields don't exist in current implementation,
	// we'll use a basic pattern matching approach based on pattern type and metadata

	// Check if pattern type matches execution context
	if patternType, ok := execData.Metadata["pattern_type"]; ok {
		if patternTypeStr, ok := patternType.(string); ok && patternTypeStr == pattern.Type {
			return true
		}
	}

	// Default: assume pattern applies (conservative approach for validation)
	return true
}

func (pcv *PatternConfidenceValidatorSimple) calculateBinSuccessRate(outcomes []bool) float64 {
	if len(outcomes) == 0 {
		return 0.0
	}

	successful := 0
	for _, outcome := range outcomes {
		if outcome {
			successful++
		}
	}

	return float64(successful) / float64(len(outcomes))
}

func (pcv *PatternConfidenceValidatorSimple) calculateSimpleConfidenceInterval(p float64, n int) Range {
	if n == 0 {
		return Range{Min: 0, Max: 1}
	}

	// Simple normal approximation
	z := 1.96 // 95% confidence
	se := math.Sqrt(p * (1 - p) / float64(n))
	margin := z * se

	return Range{
		Min: math.Max(0, p-margin),
		Max: math.Min(1, p+margin),
	}
}

func (pcv *PatternConfidenceValidatorSimple) calculateConfidenceBias(curve []SimpleCalibrationPoint) (float64, float64) {
	overConfidence := 0.0
	underConfidence := 0.0
	totalWeight := 0.0

	for _, point := range curve {
		weight := float64(point.Count)
		diff := point.PredictedProbability - point.ObservedFrequency

		if diff > 0 {
			overConfidence += weight * diff
		} else {
			underConfidence += weight * (-diff)
		}

		totalWeight += weight
	}

	if totalWeight > 0 {
		overConfidence /= totalWeight
		underConfidence /= totalWeight
	}

	return overConfidence, underConfidence
}

func (pcv *PatternConfidenceValidatorSimple) calculatePatternAccuracy(pattern *shared.DiscoveredPattern, data []*sharedtypes.WorkflowExecutionData) float64 {
	if len(data) == 0 {
		return 0.0
	}

	correct := 0
	total := 0

	for _, execData := range data {
		if pcv.patternApplies(pattern, execData) {
			total++
			// Simple threshold-based prediction
			predicted := pattern.Confidence > 0.5
			actual := execData.Success

			if predicted == actual {
				correct++
			}
		}
	}

	if total == 0 {
		return 0.0
	}

	return float64(correct) / float64(total)
}

func (pcv *PatternConfidenceValidatorSimple) calculateConfidenceAdjustment(pattern *shared.DiscoveredPattern, metrics *SimpleCalibrationMetrics) float64 {
	// Simple adjustment based on calibration error
	adjustment := 0.0

	// If overconfident, reduce confidence
	if metrics.OverConfidence > 0.1 {
		adjustment = -metrics.OverConfidence * 0.5
	}

	// If underconfident, increase confidence (but less aggressively)
	if metrics.UnderConfidence > 0.1 {
		adjustment = metrics.UnderConfidence * 0.3
	}

	// Apply pattern-specific adjustments based on pattern maturity
	if pattern != nil {
		// More mature patterns (higher frequency) get smaller adjustments
		if pattern.Frequency > 100 {
			adjustment *= 0.5 // Reduce adjustment for well-established patterns
		} else if pattern.Frequency < 10 {
			adjustment *= 1.5 // Increase adjustment for new patterns
		}

		// Adjust based on pattern confidence - very low confidence patterns need more correction
		if pattern.Confidence < 0.3 {
			adjustment *= 1.2
		}
	}

	// Cap adjustments to reasonable range
	return math.Max(-0.3, math.Min(0.2, adjustment))
}

func (pcv *PatternConfidenceValidatorSimple) calculateQualityGrade(report *SimpleConfidenceValidationReport) string {
	score := 0.0

	// Accuracy component (50%)
	if report.OverallAccuracy > 0.8 {
		score += 0.5
	} else if report.OverallAccuracy > 0.6 {
		score += 0.5 * (report.OverallAccuracy - 0.6) / 0.2
	}

	// Calibration component (50%)
	maxCalibrationError := 0.3
	calibrationScore := math.Max(0, (maxCalibrationError-report.CalibrationError)/maxCalibrationError)
	score += 0.5 * calibrationScore

	if score >= 0.9 {
		return "A"
	} else if score >= 0.8 {
		return "B"
	} else if score >= 0.7 {
		return "C"
	} else if score >= 0.6 {
		return "D"
	}
	return "F"
}

func (pcv *PatternConfidenceValidatorSimple) generateRecommendations(report *SimpleConfidenceValidationReport) []string {
	recommendations := make([]string, 0)

	if report.CalibrationError > 0.15 {
		recommendations = append(recommendations, "High calibration error detected. Consider recalibrating confidence scores.")
	}

	if report.OverallAccuracy < 0.7 {
		recommendations = append(recommendations, "Low prediction accuracy. Review pattern definition and feature extraction.")
	}

	if report.QualityGrade == "D" || report.QualityGrade == "F" {
		recommendations = append(recommendations, "Poor quality pattern. Consider collecting more data or refining pattern criteria.")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Pattern confidence appears reasonably well-calibrated.")
	}

	return recommendations
}

func (pcv *PatternConfidenceValidatorSimple) updateValidationHistory(patternID string, report *SimpleConfidenceValidationReport) {
	history, exists := pcv.validationHistory[patternID]
	if !exists {
		history = &SimpleConfidenceValidationHistory{
			PatternID:         patternID,
			ValidationResults: make([]*SimpleConfidenceValidationResult, 0),
		}
		pcv.validationHistory[patternID] = history
	}

	result := &SimpleConfidenceValidationResult{
		ValidationID:        fmt.Sprintf("val-%d", time.Now().Unix()),
		Timestamp:           report.ValidationTimestamp,
		PredictedConfidence: 0.0, // Would be filled from original pattern
		ActualOutcome:       report.OverallAccuracy > 0.5,
		ValidationMethod:    "empirical",
		CalibrationError:    report.CalibrationError,
		Reliability:         report.Reliability,
	}

	history.ValidationResults = append(history.ValidationResults, result)
	history.LastValidated = report.ValidationTimestamp
	history.TotalValidations++

	if result.ActualOutcome {
		history.SuccessfulPredictions++
	}
}

// GetValidationHistory returns the validation history for a pattern
func (pcv *PatternConfidenceValidatorSimple) GetValidationHistory(patternID string) *SimpleConfidenceValidationHistory {
	return pcv.validationHistory[patternID]
}

// GetAllValidationHistory returns all validation history
func (pcv *PatternConfidenceValidatorSimple) GetAllValidationHistory() map[string]*SimpleConfidenceValidationHistory {
	return pcv.validationHistory
}
