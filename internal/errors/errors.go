package errors

import (
	"fmt"
	"net/http"
	"strings"
)

// ErrorType represents the category of error
type ErrorType string

const (
	ErrorTypeValidation ErrorType = "validation"
	ErrorTypeDatabase   ErrorType = "database"
	ErrorTypeNetwork    ErrorType = "network"
	ErrorTypeAuth       ErrorType = "auth"
	ErrorTypeNotFound   ErrorType = "not_found"
	ErrorTypeConflict   ErrorType = "conflict"
	ErrorTypeInternal   ErrorType = "internal"
	ErrorTypeTimeout    ErrorType = "timeout"
	ErrorTypeRateLimit  ErrorType = "rate_limit"
)

// AppError represents a structured application error
type AppError struct {
	Type       ErrorType `json:"type"`
	Message    string    `json:"message"`
	Details    string    `json:"details,omitempty"`
	StatusCode int       `json:"status_code"`
	Cause      error     `json:"-"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Type, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// Unwrap returns the underlying error
func (e *AppError) Unwrap() error {
	return e.Cause
}

// New creates a new AppError
func New(errorType ErrorType, message string) *AppError {
	return &AppError{
		Type:       errorType,
		Message:    message,
		StatusCode: getStatusCode(errorType),
	}
}

// Wrap wraps an existing error with additional context
func Wrap(err error, errorType ErrorType, message string) *AppError {
	return &AppError{
		Type:       errorType,
		Message:    message,
		Cause:      err,
		StatusCode: getStatusCode(errorType),
	}
}

// Wrapf wraps an existing error with formatted message
func Wrapf(err error, errorType ErrorType, format string, args ...interface{}) *AppError {
	return &AppError{
		Type:       errorType,
		Message:    fmt.Sprintf(format, args...),
		Cause:      err,
		StatusCode: getStatusCode(errorType),
	}
}

// WithDetails adds details to an AppError
func (e *AppError) WithDetails(details string) *AppError {
	e.Details = details
	return e
}

// WithDetailsf adds formatted details to an AppError
func (e *AppError) WithDetailsf(format string, args ...interface{}) *AppError {
	e.Details = fmt.Sprintf(format, args...)
	return e
}

// getStatusCode maps error types to HTTP status codes
func getStatusCode(errorType ErrorType) int {
	switch errorType {
	case ErrorTypeValidation:
		return http.StatusBadRequest
	case ErrorTypeAuth:
		return http.StatusUnauthorized
	case ErrorTypeNotFound:
		return http.StatusNotFound
	case ErrorTypeConflict:
		return http.StatusConflict
	case ErrorTypeTimeout:
		return http.StatusRequestTimeout
	case ErrorTypeRateLimit:
		return http.StatusTooManyRequests
	case ErrorTypeDatabase, ErrorTypeNetwork, ErrorTypeInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// Predefined error constructors for common scenarios

// NewValidationError creates a validation error
func NewValidationError(message string) *AppError {
	return New(ErrorTypeValidation, message)
}

// NewDatabaseError creates a database error
func NewDatabaseError(operation string, err error) *AppError {
	return Wrapf(err, ErrorTypeDatabase, "database operation failed: %s", operation)
}

// NewNotFoundError creates a not found error
func NewNotFoundError(resource string) *AppError {
	return New(ErrorTypeNotFound, fmt.Sprintf("%s not found", resource))
}

// NewAuthError creates an authentication error
func NewAuthError(message string) *AppError {
	return New(ErrorTypeAuth, message)
}

// NewTimeoutError creates a timeout error
func NewTimeoutError(operation string) *AppError {
	return New(ErrorTypeTimeout, fmt.Sprintf("operation timed out: %s", operation))
}

// IsType checks if an error is of a specific type
func IsType(err error, errorType ErrorType) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Type == errorType
	}
	return false
}

// GetType returns the error type, or ErrorTypeInternal if not an AppError
func GetType(err error) ErrorType {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Type
	}
	return ErrorTypeInternal
}

// GetStatusCode returns the HTTP status code for an error
func GetStatusCode(err error) int {
	if appErr, ok := err.(*AppError); ok {
		return appErr.StatusCode
	}
	return http.StatusInternalServerError
}

// ErrorMessages contains common error messages to ensure consistency
var ErrorMessages = struct {
	ResourceNotFound       string
	InvalidInput           string
	DatabaseUnavailable    string
	AuthenticationFailed   string
	PermissionDenied       string
	RateLimitExceeded      string
	OperationTimeout       string
	ConcurrentModification string
}{
	ResourceNotFound:       "The requested resource was not found",
	InvalidInput:           "The provided input is invalid",
	DatabaseUnavailable:    "Database is temporarily unavailable",
	AuthenticationFailed:   "Authentication failed",
	PermissionDenied:       "Permission denied",
	RateLimitExceeded:      "Rate limit exceeded, please try again later",
	OperationTimeout:       "Operation timed out",
	ConcurrentModification: "Resource was modified by another process",
}

// SafeErrorMessage returns a user-safe error message
func SafeErrorMessage(err error) string {
	if appErr, ok := err.(*AppError); ok {
		switch appErr.Type {
		case ErrorTypeValidation:
			return appErr.Message
		case ErrorTypeNotFound:
			return ErrorMessages.ResourceNotFound
		case ErrorTypeAuth:
			return ErrorMessages.AuthenticationFailed
		case ErrorTypeTimeout:
			return ErrorMessages.OperationTimeout
		case ErrorTypeRateLimit:
			return ErrorMessages.RateLimitExceeded
		case ErrorTypeConflict:
			return ErrorMessages.ConcurrentModification
		default:
			return "An internal error occurred"
		}
	}
	return "An unexpected error occurred"
}

// LogFields returns structured fields for logging
func LogFields(err error) map[string]interface{} {
	fields := map[string]interface{}{
		"error": err.Error(),
	}

	if appErr, ok := err.(*AppError); ok {
		fields["error_type"] = string(appErr.Type)
		fields["status_code"] = appErr.StatusCode
		if appErr.Details != "" {
			fields["error_details"] = appErr.Details
		}
		if appErr.Cause != nil {
			fields["underlying_error"] = appErr.Cause.Error()
		}
	}

	return fields
}

// Chain creates a chain of errors for better context
func Chain(errors ...error) error {
	if len(errors) == 0 {
		return nil
	}

	// Filter out nil errors
	var validErrors []error
	for _, err := range errors {
		if err != nil {
			validErrors = append(validErrors, err)
		}
	}

	if len(validErrors) == 0 {
		return nil
	}
	if len(validErrors) == 1 {
		return validErrors[0]
	}

	// Create a chain of error messages
	var messages []string
	for _, err := range validErrors {
		messages = append(messages, err.Error())
	}

	return &AppError{
		Type:       ErrorTypeInternal,
		Message:    strings.Join(messages, " -> "),
		StatusCode: http.StatusInternalServerError,
	}
}
