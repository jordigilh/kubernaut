package errors

import (
	"fmt"
	"strings"
)

// OperationError represents a structured error with operation context
type OperationError struct {
	Operation string
	Component string
	Resource  string
	Cause     error
}

// Error implements the error interface
func (e *OperationError) Error() string {
	var parts []string

	if e.Operation != "" {
		parts = append(parts, fmt.Sprintf("failed to %s", e.Operation))
	}

	if e.Component != "" {
		parts = append(parts, fmt.Sprintf("component: %s", e.Component))
	}

	if e.Resource != "" {
		parts = append(parts, fmt.Sprintf("resource: %s", e.Resource))
	}

	if e.Cause != nil {
		parts = append(parts, fmt.Sprintf("cause: %v", e.Cause))
	}

	return strings.Join(parts, ", ")
}

// Unwrap returns the underlying cause for error unwrapping
func (e *OperationError) Unwrap() error {
	return e.Cause
}

// FailedTo creates a standardized "failed to" error message
// This consolidates the common pattern: fmt.Errorf("failed to [action]: %w", err)
func FailedTo(action string, cause error) error {
	if cause == nil {
		return fmt.Errorf("failed to %s", action)
	}
	return fmt.Errorf("failed to %s: %w", action, cause)
}

// FailedToWithDetails creates a detailed error with component and resource context
func FailedToWithDetails(action, component, resource string, cause error) error {
	return &OperationError{
		Operation: action,
		Component: component,
		Resource:  resource,
		Cause:     cause,
	}
}

// Wrapf wraps an error with a formatted message (similar to pkg/errors)
func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s: %w", msg, err)
}

// DatabaseError creates a standardized database operation error
func DatabaseError(operation string, cause error) error {
	return FailedToWithDetails(operation, "database", "", cause)
}

// NetworkError creates a standardized network operation error
func NetworkError(operation string, endpoint string, cause error) error {
	return FailedToWithDetails(operation, "network", endpoint, cause)
}

// ValidationError creates a standardized validation error
func ValidationError(field string, reason string) error {
	return fmt.Errorf("validation failed for field %s: %s", field, reason)
}

// ConfigurationError creates a standardized configuration error
func ConfigurationError(setting string, reason string) error {
	return fmt.Errorf("configuration error for setting %s: %s", setting, reason)
}

// TimeoutError creates a standardized timeout error
func TimeoutError(operation string, duration interface{}) error {
	return fmt.Errorf("timeout while %s after %v", operation, duration)
}

// AuthenticationError creates a standardized authentication error
func AuthenticationError(reason string) error {
	return fmt.Errorf("authentication failed: %s", reason)
}

// AuthorizationError creates a standardized authorization error
func AuthorizationError(action, resource string) error {
	return fmt.Errorf("authorization failed: insufficient permissions to %s %s", action, resource)
}

// ParseError creates a standardized parsing error
func ParseError(content, format string, cause error) error {
	return FailedToWithDetails(fmt.Sprintf("parse %s as %s", content, format), "parser", "", cause)
}

// IsTemporary checks if an error is temporary (can be retried)
func IsTemporary(err error) bool {
	type temporary interface {
		Temporary() bool
	}

	if temp, ok := err.(temporary); ok {
		return temp.Temporary()
	}
	return false
}

// IsRetryable checks if an error is retryable based on common patterns
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	if IsTemporary(err) {
		return true
	}

	errStr := strings.ToLower(err.Error())
	retryablePatterns := []string{
		"timeout",
		"connection refused",
		"connection reset",
		"temporary failure",
		"service unavailable",
		"too many requests",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// Chain combines multiple errors into a single error message
func Chain(errors ...error) error {
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

	var messages []string
	for _, err := range validErrors {
		messages = append(messages, err.Error())
	}

	return fmt.Errorf("multiple errors: %s", strings.Join(messages, "; "))
}

