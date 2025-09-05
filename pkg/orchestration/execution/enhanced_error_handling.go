package orchestration

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// EnhancedErrorHandler provides actionable error messages and user-friendly feedback
type EnhancedErrorHandler struct {
	log *logrus.Logger
}

// PatternEngineError represents an enhanced error with actionable suggestions
type PatternEngineError struct {
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	UserMessage string            `json:"user_message"`
	Suggestions []string          `json:"suggestions"`
	Context     map[string]string `json:"context"`
	Severity    ErrorSeverity     `json:"severity"`
	Category    ErrorCategory     `json:"category"`
	HelpURL     string            `json:"help_url,omitempty"`
	QuickFixes  []QuickFix        `json:"quick_fixes,omitempty"`
}

// ErrorSeverity defines error severity levels
type ErrorSeverity string

const (
	ErrorSeverityInfo     ErrorSeverity = "info"
	ErrorSeverityWarning  ErrorSeverity = "warning"
	ErrorSeverityError    ErrorSeverity = "error"
	ErrorSeverityCritical ErrorSeverity = "critical"
)

// ErrorCategory defines error categories for better organization
type ErrorCategory string

const (
	ErrorCategoryConfiguration ErrorCategory = "configuration"
	ErrorCategoryData          ErrorCategory = "data"
	ErrorCategoryModel         ErrorCategory = "model"
	ErrorCategoryDependency    ErrorCategory = "dependency"
	ErrorCategoryValidation    ErrorCategory = "validation"
	ErrorCategorySystem        ErrorCategory = "system"
)

// QuickFix represents an automated fix suggestion
type QuickFix struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Action      string            `json:"action"`
	Parameters  map[string]string `json:"parameters,omitempty"`
	Risk        QuickFixRisk      `json:"risk"`
}

// QuickFixRisk defines risk levels for quick fixes
type QuickFixRisk string

const (
	QuickFixRiskSafe   QuickFixRisk = "safe"
	QuickFixRiskLow    QuickFixRisk = "low"
	QuickFixRiskMedium QuickFixRisk = "medium"
	QuickFixRiskHigh   QuickFixRisk = "high"
)

// NewEnhancedErrorHandler creates a new enhanced error handler
func NewEnhancedErrorHandler(log *logrus.Logger) *EnhancedErrorHandler {
	return &EnhancedErrorHandler{
		log: log,
	}
}

// Error implements the error interface
func (pee *PatternEngineError) Error() string {
	return pee.Message
}

// WrapError enhances a generic error with actionable information
func (eeh *EnhancedErrorHandler) WrapError(err error, context map[string]string) *PatternEngineError {
	if err == nil {
		return nil
	}

	errorMsg := err.Error()
	enhancedError := &PatternEngineError{
		Message: errorMsg,
		Context: context,
	}

	// Analyze error and provide enhanced information
	eeh.analyzeAndEnhanceError(enhancedError)

	return enhancedError
}

// CreateConfigurationError creates an enhanced configuration error
func (eeh *EnhancedErrorHandler) CreateConfigurationError(field, value, issue string) *PatternEngineError {
	return &PatternEngineError{
		Code:        "CONFIG_INVALID",
		Message:     fmt.Sprintf("Invalid configuration for %s: %s", field, issue),
		UserMessage: fmt.Sprintf("Configuration problem with %s", field),
		Category:    ErrorCategoryConfiguration,
		Severity:    ErrorSeverityError,
		Context: map[string]string{
			"field": field,
			"value": value,
			"issue": issue,
		},
		Suggestions: eeh.getConfigurationSuggestions(field, value, issue),
		QuickFixes:  eeh.getConfigurationQuickFixes(field, value, issue),
		HelpURL:     fmt.Sprintf("https://docs.example.com/config/%s", strings.ToLower(field)),
	}
}

// CreateDataValidationError creates an enhanced data validation error
func (eeh *EnhancedErrorHandler) CreateDataValidationError(dataType, validation, details string) *PatternEngineError {
	return &PatternEngineError{
		Code:        "DATA_VALIDATION_FAILED",
		Message:     fmt.Sprintf("Data validation failed for %s: %s", dataType, validation),
		UserMessage: fmt.Sprintf("The %s data doesn't meet quality requirements", dataType),
		Category:    ErrorCategoryData,
		Severity:    ErrorSeverityWarning,
		Context: map[string]string{
			"data_type":  dataType,
			"validation": validation,
			"details":    details,
		},
		Suggestions: eeh.getDataValidationSuggestions(dataType, validation),
		QuickFixes:  eeh.getDataValidationQuickFixes(dataType, validation),
		HelpURL:     "https://docs.example.com/data-validation",
	}
}

// CreateModelError creates an enhanced model-related error
func (eeh *EnhancedErrorHandler) CreateModelError(operation, model, reason string) *PatternEngineError {
	return &PatternEngineError{
		Code:        "MODEL_OPERATION_FAILED",
		Message:     fmt.Sprintf("Model operation failed: %s on %s - %s", operation, model, reason),
		UserMessage: fmt.Sprintf("Machine learning model '%s' encountered an issue", model),
		Category:    ErrorCategoryModel,
		Severity:    ErrorSeverityError,
		Context: map[string]string{
			"operation": operation,
			"model":     model,
			"reason":    reason,
		},
		Suggestions: eeh.getModelErrorSuggestions(operation, model, reason),
		QuickFixes:  eeh.getModelErrorQuickFixes(operation, model, reason),
		HelpURL:     "https://docs.example.com/models/troubleshooting",
	}
}

// CreateDependencyError creates an enhanced dependency error
func (eeh *EnhancedErrorHandler) CreateDependencyError(dependency, operation, status string) *PatternEngineError {
	severity := ErrorSeverityError
	if status == "degraded" {
		severity = ErrorSeverityWarning
	}

	return &PatternEngineError{
		Code:        "DEPENDENCY_UNAVAILABLE",
		Message:     fmt.Sprintf("Dependency %s is %s during %s", dependency, status, operation),
		UserMessage: fmt.Sprintf("External service '%s' is currently %s", dependency, status),
		Category:    ErrorCategoryDependency,
		Severity:    severity,
		Context: map[string]string{
			"dependency": dependency,
			"operation":  operation,
			"status":     status,
		},
		Suggestions: eeh.getDependencyErrorSuggestions(dependency, status),
		QuickFixes:  eeh.getDependencyErrorQuickFixes(dependency, status),
		HelpURL:     "https://docs.example.com/dependencies/troubleshooting",
	}
}

// Private helper methods for error analysis and enhancement

func (eeh *EnhancedErrorHandler) analyzeAndEnhanceError(enhancedError *PatternEngineError) {
	msg := strings.ToLower(enhancedError.Message)

	switch {
	case strings.Contains(msg, "config"):
		enhancedError.Category = ErrorCategoryConfiguration
		enhancedError.Code = "CONFIG_ERROR"
		enhancedError.UserMessage = "Configuration issue detected"
		enhancedError.Suggestions = []string{
			"Check your configuration file for syntax errors",
			"Verify all required configuration fields are set",
			"Validate configuration values are within acceptable ranges",
		}

	case strings.Contains(msg, "validation") || strings.Contains(msg, "invalid"):
		enhancedError.Category = ErrorCategoryValidation
		enhancedError.Code = "VALIDATION_ERROR"
		enhancedError.UserMessage = "Input validation failed"
		enhancedError.Suggestions = []string{
			"Check input data format and structure",
			"Ensure all required fields are provided",
			"Verify data types match expected schema",
		}

	case strings.Contains(msg, "connection") || strings.Contains(msg, "network"):
		enhancedError.Category = ErrorCategoryDependency
		enhancedError.Code = "CONNECTION_ERROR"
		enhancedError.UserMessage = "Network or connection issue"
		enhancedError.Suggestions = []string{
			"Check network connectivity to external services",
			"Verify service endpoints are correct and accessible",
			"Check if services are running and healthy",
		}

	case strings.Contains(msg, "model") || strings.Contains(msg, "prediction"):
		enhancedError.Category = ErrorCategoryModel
		enhancedError.Code = "MODEL_ERROR"
		enhancedError.UserMessage = "Machine learning model issue"
		enhancedError.Suggestions = []string{
			"Check if model is properly trained and loaded",
			"Verify input features match model expectations",
			"Consider retraining model with recent data",
		}

	default:
		enhancedError.Category = ErrorCategorySystem
		enhancedError.Code = "SYSTEM_ERROR"
		enhancedError.UserMessage = "System error occurred"
		enhancedError.Suggestions = []string{
			"Check system logs for more details",
			"Verify system resources are available",
			"Try the operation again after a brief wait",
		}
	}

	enhancedError.Severity = eeh.determineSeverity(enhancedError.Message)
}

func (eeh *EnhancedErrorHandler) determineSeverity(message string) ErrorSeverity {
	msg := strings.ToLower(message)

	switch {
	case strings.Contains(msg, "critical") || strings.Contains(msg, "fatal"):
		return ErrorSeverityCritical
	case strings.Contains(msg, "error") || strings.Contains(msg, "failed"):
		return ErrorSeverityError
	case strings.Contains(msg, "warning") || strings.Contains(msg, "deprecated"):
		return ErrorSeverityWarning
	default:
		return ErrorSeverityInfo
	}
}

func (eeh *EnhancedErrorHandler) getConfigurationSuggestions(field, value, issue string) []string {
	suggestions := []string{
		fmt.Sprintf("Review the %s configuration field", field),
		"Check configuration documentation for valid values",
		"Use configuration validation tools",
	}

	switch field {
	case "MinExecutionsForPattern":
		suggestions = append(suggestions, "Set value between 5-100 based on your data volume")
	case "SimilarityThreshold":
		suggestions = append(suggestions, "Use values between 0.7-0.95 for best results")
	case "PredictionConfidence":
		suggestions = append(suggestions, "Recommended range is 0.6-0.9 for production")
	}

	return suggestions
}

func (eeh *EnhancedErrorHandler) getConfigurationQuickFixes(field, value, issue string) []QuickFix {
	fixes := []QuickFix{}

	switch field {
	case "MinExecutionsForPattern":
		fixes = append(fixes, QuickFix{
			Title:       "Set Recommended Value",
			Description: "Set MinExecutionsForPattern to recommended default of 10",
			Action:      "set_config_value",
			Parameters:  map[string]string{"field": field, "value": "10"},
			Risk:        QuickFixRiskSafe,
		})
	case "SimilarityThreshold":
		fixes = append(fixes, QuickFix{
			Title:       "Use Balanced Threshold",
			Description: "Set SimilarityThreshold to balanced value of 0.85",
			Action:      "set_config_value",
			Parameters:  map[string]string{"field": field, "value": "0.85"},
			Risk:        QuickFixRiskSafe,
		})
	}

	return fixes
}

func (eeh *EnhancedErrorHandler) getDataValidationSuggestions(dataType, validation string) []string {
	return []string{
		fmt.Sprintf("Check %s data format and completeness", dataType),
		"Ensure data meets minimum quality requirements",
		"Consider data cleaning or preprocessing",
		"Verify data source is reliable and up-to-date",
	}
}

func (eeh *EnhancedErrorHandler) getDataValidationQuickFixes(dataType, validation string) []QuickFix {
	return []QuickFix{
		{
			Title:       "Skip Validation",
			Description: "Temporarily skip validation (not recommended for production)",
			Action:      "skip_validation",
			Parameters:  map[string]string{"data_type": dataType},
			Risk:        QuickFixRiskHigh,
		},
		{
			Title:       "Use Fallback Data",
			Description: "Use cached or default data for this analysis",
			Action:      "use_fallback_data",
			Parameters:  map[string]string{"data_type": dataType},
			Risk:        QuickFixRiskMedium,
		},
	}
}

func (eeh *EnhancedErrorHandler) getModelErrorSuggestions(operation, model, reason string) []string {
	suggestions := []string{
		fmt.Sprintf("Check %s model status and health", model),
		"Verify model input data format and quality",
		"Consider model retraining or updates",
	}

	if strings.Contains(reason, "overfitting") {
		suggestions = append(suggestions,
			"Implement regularization techniques",
			"Increase training data diversity",
			"Use cross-validation")
	}

	if strings.Contains(reason, "accuracy") {
		suggestions = append(suggestions,
			"Review training data quality",
			"Consider feature engineering",
			"Evaluate model hyperparameters")
	}

	return suggestions
}

func (eeh *EnhancedErrorHandler) getModelErrorQuickFixes(operation, model, reason string) []QuickFix {
	fixes := []QuickFix{
		{
			Title:       "Use Fallback Model",
			Description: "Switch to backup model for this operation",
			Action:      "use_fallback_model",
			Parameters:  map[string]string{"model": model, "operation": operation},
			Risk:        QuickFixRiskLow,
		},
	}

	if strings.Contains(reason, "training") {
		fixes = append(fixes, QuickFix{
			Title:       "Retrain Model",
			Description: "Automatically retrain model with latest data",
			Action:      "retrain_model",
			Parameters:  map[string]string{"model": model},
			Risk:        QuickFixRiskMedium,
		})
	}

	return fixes
}

func (eeh *EnhancedErrorHandler) getDependencyErrorSuggestions(dependency, status string) []string {
	suggestions := []string{
		fmt.Sprintf("Check %s service health and connectivity", dependency),
		"Verify network connectivity and firewall rules",
		"Review service logs for additional details",
	}

	if status == "unavailable" {
		suggestions = append(suggestions,
			"Consider using fallback mechanisms if available",
			"Check service restart or scaling options")
	}

	return suggestions
}

func (eeh *EnhancedErrorHandler) getDependencyErrorQuickFixes(dependency, status string) []QuickFix {
	fixes := []QuickFix{
		{
			Title:       "Retry Operation",
			Description: "Retry the operation with exponential backoff",
			Action:      "retry_operation",
			Parameters:  map[string]string{"dependency": dependency},
			Risk:        QuickFixRiskSafe,
		},
	}

	if status == "degraded" {
		fixes = append(fixes, QuickFix{
			Title:       "Use Fallback Service",
			Description: "Switch to fallback implementation",
			Action:      "use_fallback",
			Parameters:  map[string]string{"dependency": dependency},
			Risk:        QuickFixRiskLow,
		})
	}

	return fixes
}

// ErrorFormatter provides different error formatting options
type ErrorFormatter struct {
	handler *EnhancedErrorHandler
}

// NewErrorFormatter creates a new error formatter
func NewErrorFormatter(handler *EnhancedErrorHandler) *ErrorFormatter {
	return &ErrorFormatter{handler: handler}
}

// FormatForUser formats error for end-user display
func (ef *ErrorFormatter) FormatForUser(err *PatternEngineError) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("❌ %s\n\n", err.UserMessage))

	if len(err.Suggestions) > 0 {
		builder.WriteString("💡 Suggestions:\n")
		for _, suggestion := range err.Suggestions {
			builder.WriteString(fmt.Sprintf("  • %s\n", suggestion))
		}
		builder.WriteString("\n")
	}

	if len(err.QuickFixes) > 0 {
		builder.WriteString("🔧 Quick Fixes:\n")
		for _, fix := range err.QuickFixes {
			riskIcon := ef.getRiskIcon(fix.Risk)
			builder.WriteString(fmt.Sprintf("  %s %s - %s\n", riskIcon, fix.Title, fix.Description))
		}
		builder.WriteString("\n")
	}

	if err.HelpURL != "" {
		builder.WriteString(fmt.Sprintf("📚 More help: %s\n", err.HelpURL))
	}

	return builder.String()
}

// FormatForLog formats error for logging
func (ef *ErrorFormatter) FormatForLog(err *PatternEngineError) logrus.Fields {
	fields := logrus.Fields{
		"error_code":     err.Code,
		"error_category": err.Category,
		"error_severity": err.Severity,
		"message":        err.Message,
		"user_message":   err.UserMessage,
	}

	if len(err.Context) > 0 {
		for key, value := range err.Context {
			fields[fmt.Sprintf("context_%s", key)] = value
		}
	}

	return fields
}

func (ef *ErrorFormatter) getRiskIcon(risk QuickFixRisk) string {
	switch risk {
	case QuickFixRiskSafe:
		return "✅"
	case QuickFixRiskLow:
		return "🟢"
	case QuickFixRiskMedium:
		return "🟡"
	case QuickFixRiskHigh:
		return "🔴"
	default:
		return "❓"
	}
}
