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
package learning

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
)

// RobustFeatureExtractor provides enhanced feature extraction with validation and error recovery
type RobustFeatureExtractor struct {
	*FeatureExtractor
	validator *FeatureValidatorRobust
	log       *logrus.Logger
}

// FeatureValidatorRobust validates extracted features for quality and consistency
type FeatureValidatorRobust struct {
	log             *logrus.Logger
	validationRules map[string]*FeatureValidationRule
}

// FeatureValidationRule defines validation logic for a feature
type FeatureValidationRule struct {
	Name          string                           `json:"name"`
	Type          string                           `json:"type"` // "range", "categorical", "format"
	MinValue      *float64                         `json:"min_value,omitempty"`
	MaxValue      *float64                         `json:"max_value,omitempty"`
	AllowedValues []interface{}                    `json:"allowed_values,omitempty"`
	ValidateFunc  func(interface{}) (bool, string) `json:"-"`
	Required      bool                             `json:"required"`
}

// RobustFeatureExtractionResult contains extraction results with quality metrics
type RobustFeatureExtractionResult struct {
	Features         *shared.WorkflowFeatures       `json:"features"`
	QualityScore     float64                        `json:"quality_score"`
	ValidationErrors []RobustFeatureValidationError `json:"validation_errors"`
	Warnings         []string                       `json:"warnings"`
	Metadata         map[string]interface{}         `json:"metadata"`
}

// RobustFeatureValidationError represents a validation error
type RobustFeatureValidationError struct {
	FeatureName string      `json:"feature_name"`
	Message     string      `json:"message"`
	Severity    string      `json:"severity"` // "error", "warning", "info"
	Value       interface{} `json:"value"`
}

// NewRobustFeatureExtractor creates a new robust feature extractor
func NewRobustFeatureExtractor(baseExtractor *FeatureExtractor, log *logrus.Logger) *RobustFeatureExtractor {
	validator := &FeatureValidatorRobust{
		log:             log,
		validationRules: make(map[string]*FeatureValidationRule),
	}
	validator.initializeValidationRules()

	return &RobustFeatureExtractor{
		FeatureExtractor: baseExtractor,
		validator:        validator,
		log:              log,
	}
}

// ExtractWithValidation extracts features with comprehensive validation
func (rfe *RobustFeatureExtractor) ExtractWithValidation(data *sharedtypes.WorkflowExecutionData) (*RobustFeatureExtractionResult, error) {
	rfe.log.WithField("execution_id", data.ExecutionID).Debug("Starting robust feature extraction")

	result := &RobustFeatureExtractionResult{
		ValidationErrors: make([]RobustFeatureValidationError, 0),
		Warnings:         make([]string, 0),
		Metadata:         make(map[string]interface{}),
	}

	// Pre-validate input data
	if err := rfe.validateInputData(data); err != nil {
		return nil, fmt.Errorf("input data validation failed: %w", err)
	}

	// Extract features using base extractor with error recovery
	features, extractionErrors := rfe.extractWithErrorRecovery(data)
	result.Features = features

	// Add extraction errors to result
	for _, err := range extractionErrors {
		result.ValidationErrors = append(result.ValidationErrors, RobustFeatureValidationError{
			FeatureName: "extraction",
			Message:     err.Error(),
			Severity:    "warning",
		})
	}

	// Validate extracted features
	validationErrors := rfe.validator.ValidateFeatures(features)
	result.ValidationErrors = append(result.ValidationErrors, validationErrors...)

	// Normalize and clean features
	normalizedFeatures, warnings := rfe.normalizeFeatures(features)
	result.Features = normalizedFeatures
	result.Warnings = append(result.Warnings, warnings...)

	// Calculate quality score
	result.QualityScore = rfe.calculateQualityScore(features, result.ValidationErrors)

	// Add metadata
	result.Metadata["extraction_timestamp"] = time.Now()
	result.Metadata["total_features"] = len(rfe.GetFeatureNames())
	result.Metadata["error_count"] = len(result.ValidationErrors)
	result.Metadata["warning_count"] = len(result.Warnings)

	rfe.log.WithFields(logrus.Fields{
		"quality_score":     result.QualityScore,
		"validation_errors": len(result.ValidationErrors),
		"warnings":          len(result.Warnings),
	}).Debug("Robust feature extraction completed")

	return result, nil
}

// validateInputData validates the input workflow execution data
func (rfe *RobustFeatureExtractor) validateInputData(data *sharedtypes.WorkflowExecutionData) error {
	if data == nil {
		return fmt.Errorf("workflow execution data is nil")
	}

	if data.ExecutionID == "" {
		return fmt.Errorf("execution ID is required")
	}

	if data.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}

	// Validate timestamp is not too far in the future
	if data.Timestamp.After(time.Now().Add(time.Hour)) {
		return fmt.Errorf("timestamp is too far in the future: %v", data.Timestamp)
	}

	// Initialize metadata if nil
	if data.Metadata == nil {
		rfe.log.Debug("Metadata is nil, initializing empty metadata")
		data.Metadata = make(map[string]interface{})
	}

	return nil
}

// extractWithErrorRecovery extracts features with error recovery
func (rfe *RobustFeatureExtractor) extractWithErrorRecovery(data *sharedtypes.WorkflowExecutionData) (*shared.WorkflowFeatures, []error) {
	errors := make([]error, 0)

	// Guideline #14: Use idiomatic patterns - call method directly without embedded field selector
	// Try normal extraction first
	features, err := rfe.Extract(data)
	if err == nil {
		return features, errors
	}

	// If extraction failed, try partial extraction with defaults
	rfe.log.WithError(err).Warn("Normal feature extraction failed, attempting recovery")
	errors = append(errors, err)

	// Initialize features with safe defaults
	features = &shared.WorkflowFeatures{
		CustomMetrics: make(map[string]float64),
		AlertTypes:    make(map[string]int),
		ResourceTypes: make(map[string]int),
	}

	// Attempt individual feature extraction with recovery
	errors = append(errors, rfe.safeExtractBasicFeatures(data, features)...)

	return features, errors
}

// safeExtractBasicFeatures safely extracts basic features with error recovery
func (rfe *RobustFeatureExtractor) safeExtractBasicFeatures(data *sharedtypes.WorkflowExecutionData, features *shared.WorkflowFeatures) []error {
	errors := make([]error, 0)

	// Safe alert feature extraction
	if alertData, exists := data.Metadata["alert"]; exists {
		if alert, ok := alertData.(map[string]interface{}); ok {

			// Validate and set alert features
			features.AlertCount = 1
			if name, ok := alert["name"].(string); ok && name != "" {
				features.AlertTypes = map[string]int{name: 1}
			} else {
				features.AlertTypes = map[string]int{"unknown": 1}
				errors = append(errors, fmt.Errorf("alert name is empty"))
			}

			// Safe severity extraction
			severityStr, _ := alert["severity"].(string)
			severity := strings.ToLower(strings.TrimSpace(severityStr))
			if severity == "" {
				severity = "unknown"
			}

			severityScores := map[string]float64{
				"critical": 1.0,
				"warning":  0.75,
				"info":     0.5,
				"debug":    0.25,
				"unknown":  0.5,
			}

			if score, exists := severityScores[severity]; exists {
				features.SeverityScore = score
			} else {
				features.SeverityScore = 0.5
				errors = append(errors, fmt.Errorf("unknown severity: %s", severity))
			}

			// Resource features
			features.ResourceCount = 1
			features.NamespaceCount = 1
			if resource, ok := alert["resource"].(string); ok && resource != "" {
				features.ResourceTypes = map[string]int{resource: 1}
			} else {
				features.ResourceTypes = map[string]int{"unknown": 1}
			}

			// Custom metrics
			if labels, ok := alert["labels"].(map[string]interface{}); ok {
				features.CustomMetrics["alert_label_count"] = float64(len(labels))
			} else {
				features.CustomMetrics["alert_label_count"] = 0
			}
		}
	} else {
		// No alert data
		features.AlertCount = 0
		features.SeverityScore = 0.0
		features.ResourceCount = 0
		features.NamespaceCount = 0
		features.AlertTypes = make(map[string]int)
		features.ResourceTypes = make(map[string]int)
	}

	// Safe temporal feature extraction
	timestamp := data.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
		errors = append(errors, fmt.Errorf("timestamp is zero, using current time"))
	}

	features.HourOfDay = timestamp.Hour()
	features.DayOfWeek = int(timestamp.Weekday())
	features.IsWeekend = timestamp.Weekday() == time.Saturday || timestamp.Weekday() == time.Sunday
	features.IsBusinessHour = timestamp.Hour() >= 9 && timestamp.Hour() <= 17 && !features.IsWeekend

	// Safe historical features (with defaults)
	features.RecentFailures = 0
	features.AverageSuccessRate = 0.5
	features.LastExecutionTime = 24 * time.Hour

	// Extract from metadata if available
	if histData, exists := data.Metadata["historical_data"]; exists {
		if histMap, ok := histData.(map[string]interface{}); ok {
			if failures, ok := histMap["recent_failures"].(int); ok && failures >= 0 && failures <= 100 {
				features.RecentFailures = failures
			}
			if rate, ok := histMap["average_success_rate"].(float64); ok && rate >= 0.0 && rate <= 1.0 {
				features.AverageSuccessRate = rate
			}
		}
	}

	// Safe complexity features (with defaults)
	features.StepCount = 5
	features.DependencyDepth = 2
	features.ParallelSteps = 1

	// Extract from metadata if available
	if workflowData, exists := data.Metadata["workflow_template"]; exists {
		if workflowMap, ok := workflowData.(map[string]interface{}); ok {
			if stepCount, ok := workflowMap["step_count"].(int); ok && stepCount >= 0 && stepCount <= 100 {
				features.StepCount = stepCount
			}
			if depth, ok := workflowMap["dependency_depth"].(int); ok && depth >= 0 && depth <= 20 {
				features.DependencyDepth = depth
			}
			if parallel, ok := workflowMap["parallel_steps"].(int); ok && parallel >= 0 && parallel <= 20 {
				features.ParallelSteps = parallel
			}
		}
	}

	// Safe environment features (with defaults)
	features.ClusterSize = 10
	features.ClusterLoad = 0.5
	features.ResourcePressure = 0.3

	// Extract from metadata if available
	if envData, exists := data.Metadata["environment_metrics"]; exists {
		if envMap, ok := envData.(map[string]interface{}); ok {
			if size, ok := envMap["cluster_size"].(int); ok && size >= 1 && size <= 10000 {
				features.ClusterSize = size
			}
			if load, ok := envMap["cluster_load"].(float64); ok && load >= 0 && load <= 2.0 {
				features.ClusterLoad = load
			}
			if pressure, ok := envMap["resource_pressure"].(float64); ok && pressure >= 0 && pressure <= 2.0 {
				features.ResourcePressure = pressure
			}
		}
	}

	// Safe resource utilization
	if resourceUsage, exists := data.Metadata["resource_usage"]; exists {
		if usage, ok := resourceUsage.(map[string]interface{}); ok {
			rfe.safeAddResourceUtilizationFromMap(usage, features, &errors)
		}
	}

	return errors
}

// safeAddResourceUtilizationFromMap safely adds resource utilization metrics from map data
func (rfe *RobustFeatureExtractor) safeAddResourceUtilizationFromMap(usage map[string]interface{}, features *shared.WorkflowFeatures, errors *[]error) {
	addUtilization := func(key string, value float64) {
		if value >= 0.0 && value <= 2.0 {
			features.CustomMetrics[key] = value
		} else {
			*errors = append(*errors, fmt.Errorf("invalid %s utilization: %f", key, value))
			features.CustomMetrics[key] = 0.5 // Safe default
		}
	}

	if cpu, ok := usage["cpu_usage"].(float64); ok {
		addUtilization("cpu_utilization", cpu)
	}
	if memory, ok := usage["memory_usage"].(float64); ok {
		addUtilization("memory_utilization", memory)
	}
	if network, ok := usage["network_usage"].(float64); ok {
		addUtilization("network_utilization", network)
	}
	if storage, ok := usage["storage_usage"].(float64); ok {
		addUtilization("storage_utilization", storage)
	}
}

// normalizeFeatures normalizes features and applies bounds checking
func (rfe *RobustFeatureExtractor) normalizeFeatures(features *shared.WorkflowFeatures) (*shared.WorkflowFeatures, []string) {
	warnings := make([]string, 0)

	if features == nil {
		return features, append(warnings, "features is nil, skipping normalization")
	}

	// Create a copy to avoid modifying the original
	normalized := *features
	normalized.CustomMetrics = make(map[string]float64)
	for k, v := range features.CustomMetrics {
		normalized.CustomMetrics[k] = v
	}

	// Normalize severity score
	if features.SeverityScore < 0 || features.SeverityScore > 1 {
		normalized.SeverityScore = math.Max(0, math.Min(1, features.SeverityScore))
		warnings = append(warnings, fmt.Sprintf("severity_score clamped to [0,1]: %f", features.SeverityScore))
	}

	// Normalize cluster load
	if features.ClusterLoad < 0 || features.ClusterLoad > 2 {
		normalized.ClusterLoad = math.Max(0, math.Min(2, features.ClusterLoad))
		warnings = append(warnings, fmt.Sprintf("cluster_load clamped to [0,2]: %f", features.ClusterLoad))
	}

	// Clean custom metrics
	for key, value := range normalized.CustomMetrics {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			normalized.CustomMetrics[key] = 0.0
			warnings = append(warnings, fmt.Sprintf("custom metric %s was NaN/Inf, set to 0", key))
		}
	}

	return &normalized, warnings
}

// calculateQualityScore calculates a quality score for the extracted features
func (rfe *RobustFeatureExtractor) calculateQualityScore(features *shared.WorkflowFeatures, validationErrors []RobustFeatureValidationError) float64 {
	if features == nil {
		return 0.0
	}

	score := 1.0

	// Penalize validation errors
	for _, err := range validationErrors {
		switch err.Severity {
		case "error":
			score -= 0.2
		case "warning":
			score -= 0.05
		}
	}

	// Check feature completeness
	if features.AlertCount == 0 && len(features.AlertTypes) == 0 {
		score -= 0.1
	}
	if features.ResourceCount == 0 {
		score -= 0.1
	}
	if len(features.CustomMetrics) == 0 {
		score -= 0.05
	}

	return math.Max(0.0, math.Min(1.0, score))
}

// Validator methods

func (fv *FeatureValidatorRobust) initializeValidationRules() {
	fv.validationRules = map[string]*FeatureValidationRule{
		"alert_count": {
			Name:     "alert_count",
			Type:     "range",
			MinValue: floatPtr(0),
			MaxValue: floatPtr(1000),
			Required: true,
		},
		"severity_score": {
			Name:     "severity_score",
			Type:     "range",
			MinValue: floatPtr(0),
			MaxValue: floatPtr(1),
			Required: true,
		},
		"hour_of_day": {
			Name:     "hour_of_day",
			Type:     "range",
			MinValue: floatPtr(0),
			MaxValue: floatPtr(23),
			Required: true,
		},
		"day_of_week": {
			Name:     "day_of_week",
			Type:     "range",
			MinValue: floatPtr(0),
			MaxValue: floatPtr(6),
			Required: true,
		},
	}
}

func (fv *FeatureValidatorRobust) ValidateFeatures(features *shared.WorkflowFeatures) []RobustFeatureValidationError {
	errors := make([]RobustFeatureValidationError, 0)

	if features == nil {
		errors = append(errors, RobustFeatureValidationError{
			FeatureName: "features",
			Message:     "features object is nil",
			Severity:    "error",
		})
		return errors
	}

	// Validate individual features
	errors = append(errors, fv.validateNumericFeature("alert_count", float64(features.AlertCount))...)
	errors = append(errors, fv.validateNumericFeature("severity_score", features.SeverityScore)...)
	errors = append(errors, fv.validateNumericFeature("hour_of_day", float64(features.HourOfDay))...)
	errors = append(errors, fv.validateNumericFeature("day_of_week", float64(features.DayOfWeek))...)

	// Validate custom metrics
	for key, value := range features.CustomMetrics {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			errors = append(errors, RobustFeatureValidationError{
				FeatureName: key,
				Message:     "value is NaN or Inf",
				Severity:    "error",
				Value:       value,
			})
		}
	}

	return errors
}

func (fv *FeatureValidatorRobust) validateNumericFeature(name string, value float64) []RobustFeatureValidationError {
	errors := make([]RobustFeatureValidationError, 0)

	rule, exists := fv.validationRules[name]
	if !exists {
		return errors
	}

	// Check for NaN or Inf
	if math.IsNaN(value) || math.IsInf(value, 0) {
		errors = append(errors, RobustFeatureValidationError{
			FeatureName: name,
			Message:     "value is NaN or Inf",
			Severity:    "error",
			Value:       value,
		})
		return errors
	}

	// Check range
	if rule.MinValue != nil && value < *rule.MinValue {
		errors = append(errors, RobustFeatureValidationError{
			FeatureName: name,
			Message:     fmt.Sprintf("value %f is below minimum %f", value, *rule.MinValue),
			Severity:    "warning",
			Value:       value,
		})
	}

	if rule.MaxValue != nil && value > *rule.MaxValue {
		errors = append(errors, RobustFeatureValidationError{
			FeatureName: name,
			Message:     fmt.Sprintf("value %f is above maximum %f", value, *rule.MaxValue),
			Severity:    "warning",
			Value:       value,
		})
	}

	return errors
}

// Helper function
func floatPtr(f float64) *float64 {
	return &f
}
