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

package errors

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// EnhancedErrorHandler provides actionable error messages and user-friendly feedback
// This consolidates error handling patterns from multiple locations:
// - pkg/orchestration/execution/enhanced_error_handling.go
// - internal/errors/errors.go
// - existing pkg/shared/errors/errors.go functionality
type EnhancedErrorHandler struct {
	log *logrus.Logger
}

// EnhancedError represents a comprehensive error with actionable suggestions
// This replaces and consolidates:
// - PatternEngineError from orchestration/execution
// - AppError from internal/errors
// - OperationError from shared/errors
type EnhancedError struct {
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	UserMessage string            `json:"user_message"`
	Suggestions []string          `json:"suggestions"`
	Context     map[string]string `json:"context"`
	Severity    ErrorSeverity     `json:"severity"`
	Category    ErrorCategory     `json:"category"`
	HelpURL     string            `json:"help_url,omitempty"`
	QuickFixes  []QuickFix        `json:"quick_fixes,omitempty"`
	StatusCode  int               `json:"status_code"`
	Cause       error             `json:"-"`

	// Legacy compatibility fields
	Operation string `json:"operation,omitempty"` // For OperationError compatibility
	Component string `json:"component,omitempty"` // For OperationError compatibility
	Resource  string `json:"resource,omitempty"`  // For OperationError compatibility
	Details   string `json:"details,omitempty"`   // For AppError compatibility
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
	ErrorCategoryDatabase      ErrorCategory = "database"
	ErrorCategoryNetwork       ErrorCategory = "network"
	ErrorCategoryAuth          ErrorCategory = "auth"
	ErrorCategoryNotFound      ErrorCategory = "not_found"
	ErrorCategoryConflict      ErrorCategory = "conflict"
	ErrorCategoryInternal      ErrorCategory = "internal"
	ErrorCategoryTimeout       ErrorCategory = "timeout"
	ErrorCategoryRateLimit     ErrorCategory = "rate_limit"
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
func (ee *EnhancedError) Error() string {
	return ee.Message
}

// ConvertToEnhancedError converts various error types to EnhancedError
// This consolidates error conversion patterns found across multiple packages
func ConvertToEnhancedError(err error, category ErrorCategory, message string) *EnhancedError {
	if err == nil {
		return nil
	}

	// If already an EnhancedError, just wrap it
	if enhanced, ok := err.(*EnhancedError); ok {
		return &EnhancedError{
			Code:        enhanced.Code,
			Category:    category,
			Message:     message,
			UserMessage: enhanced.UserMessage,
			Suggestions: enhanced.Suggestions,
			Context:     enhanced.Context,
			Severity:    enhanced.Severity,
			HelpURL:     enhanced.HelpURL,
			QuickFixes:  enhanced.QuickFixes,
			StatusCode:  enhanced.StatusCode,
			Cause:       enhanced,
			Operation:   enhanced.Operation,
			Component:   enhanced.Component,
			Resource:    enhanced.Resource,
			Details:     enhanced.Details,
		}
	}

	// Convert other error types
	return Wrap(err, category, message)
}

// Unwrap returns the underlying cause for error unwrapping
func (ee *EnhancedError) Unwrap() error {
	return ee.Cause
}

// WrapError enhances a generic error with actionable information
func (eeh *EnhancedErrorHandler) WrapError(err error, context map[string]string) *EnhancedError {
	if err == nil {
		return nil
	}

	errorMsg := err.Error()
	enhancedError := &EnhancedError{
		Message:    errorMsg,
		Context:    context,
		StatusCode: 500, // Default to internal server error
		Cause:      err,
	}

	// Analyze error and provide enhanced information
	eeh.analyzeAndEnhanceError(enhancedError)

	return enhancedError
}

// New creates a new EnhancedError with specified category and message
func New(category ErrorCategory, message string) *EnhancedError {
	return &EnhancedError{
		Code:       string(category) + "_ERROR",
		Message:    message,
		Category:   category,
		Severity:   ErrorSeverityError,
		StatusCode: getStatusCodeForCategory(category),
	}
}

// Wrap wraps an existing error with additional context
func Wrap(err error, category ErrorCategory, message string) *EnhancedError {
	return &EnhancedError{
		Code:       string(category) + "_ERROR",
		Message:    message,
		Cause:      err,
		Category:   category,
		Severity:   ErrorSeverityError,
		StatusCode: getStatusCodeForCategory(category),
	}
}

// Wrapf wraps an existing error with formatted message
func Wrapf(err error, category ErrorCategory, format string, args ...interface{}) *EnhancedError {
	if err == nil {
		return nil
	}
	return &EnhancedError{
		Code:       string(category) + "_ERROR",
		Message:    fmt.Sprintf(format, args...),
		Cause:      err,
		Category:   category,
		Severity:   ErrorSeverityError,
		StatusCode: getStatusCodeForCategory(category),
	}
}

// Legacy compatibility functions

// FailedTo creates a standardized "failed to" error message
// Maintains compatibility with existing pkg/shared/errors functionality
func FailedTo(action string, cause error) error {
	if cause == nil {
		return New(ErrorCategorySystem, fmt.Sprintf("failed to %s", action))
	}
	return Wrap(cause, ErrorCategorySystem, fmt.Sprintf("failed to %s", action))
}

// FailedToWithDetails creates a detailed error with component and resource context
// Maintains compatibility with existing OperationError functionality
func FailedToWithDetails(action, component, resource string, cause error) error {
	enhanced := &EnhancedError{
		Code:       "OPERATION_FAILED",
		Message:    fmt.Sprintf("failed to %s", action),
		Operation:  action,
		Component:  component,
		Resource:   resource,
		Category:   ErrorCategorySystem,
		Severity:   ErrorSeverityError,
		StatusCode: 500,
		Cause:      cause,
		Context: map[string]string{
			"operation": action,
			"component": component,
			"resource":  resource,
		},
	}
	return enhanced
}

// DatabaseError creates a standardized database operation error
func DatabaseError(operation string, cause error) error {
	return Wrap(cause, ErrorCategoryDatabase, fmt.Sprintf("database operation failed: %s", operation))
}

// NetworkError creates a standardized network operation error
func NetworkError(operation string, endpoint string, cause error) error {
	enhanced := Wrap(cause, ErrorCategoryNetwork, fmt.Sprintf("network operation failed: %s", operation))
	if enhanced.Context == nil {
		enhanced.Context = make(map[string]string)
	}
	enhanced.Context["endpoint"] = endpoint
	return enhanced
}

// Helper functions

func getStatusCodeForCategory(category ErrorCategory) int {
	switch category {
	case ErrorCategoryValidation:
		return 400
	case ErrorCategoryAuth:
		return 401
	case ErrorCategoryNotFound:
		return 404
	case ErrorCategoryConflict:
		return 409
	case ErrorCategoryTimeout:
		return 408
	case ErrorCategoryRateLimit:
		return 429
	case ErrorCategoryDependency, ErrorCategoryNetwork:
		return 503
	default:
		return 500
	}
}

func (eeh *EnhancedErrorHandler) analyzeAndEnhanceError(enhancedError *EnhancedError) {
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

	case strings.Contains(msg, "database") || strings.Contains(msg, "sql"):
		enhancedError.Category = ErrorCategoryDatabase
		enhancedError.Code = "DATABASE_ERROR"
		enhancedError.UserMessage = "Database operation failed"
		enhancedError.Suggestions = []string{
			"Check database connectivity and credentials",
			"Verify database schema and permissions",
			"Check for database locks or resource constraints",
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
	enhancedError.StatusCode = getStatusCodeForCategory(enhancedError.Category)
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

// ErrorFormatter provides different error formatting options
type ErrorFormatter struct {
	handler *EnhancedErrorHandler
}

// NewErrorFormatter creates a new error formatter
func NewErrorFormatter(handler *EnhancedErrorHandler) *ErrorFormatter {
	return &ErrorFormatter{handler: handler}
}

// FormatForUser formats error for end-user display
func (ef *ErrorFormatter) FormatForUser(err *EnhancedError) string {
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
func (ef *ErrorFormatter) FormatForLog(err *EnhancedError) logrus.Fields {
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

// Backward compatibility removed - use EnhancedError directly
