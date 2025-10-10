//go:build integration
// +build integration

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
package shared

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ErrorCategory defines common error categories for systematic testing
type ErrorCategory string

const (
	NetworkError    ErrorCategory = "network"
	TimeoutError    ErrorCategory = "timeout"
	AuthError       ErrorCategory = "authentication"
	RateLimitError  ErrorCategory = "rate_limit"
	ResourceError   ErrorCategory = "resource"
	ValidationError ErrorCategory = "validation"
	ConfigError     ErrorCategory = "configuration"
	DatabaseError   ErrorCategory = "database"
	K8sAPIError     ErrorCategory = "k8s_api"
	SLMError        ErrorCategory = "slm_service"
)

// ErrorSeverity represents the severity level of errors
type ErrorSeverity string

const (
	SeverityLow      ErrorSeverity = "low"
	SeverityMedium   ErrorSeverity = "medium"
	SeverityHigh     ErrorSeverity = "high"
	SeverityCritical ErrorSeverity = "critical"
)

// StandardizedError provides a comprehensive error structure for consistent testing
type StandardizedError struct {
	Category      ErrorCategory
	Code          string
	Message       string
	Retryable     bool
	Recovery      []string // Suggested recovery actions
	Context       map[string]interface{}
	Severity      ErrorSeverity
	Component     string
	MaxRetries    int
	RetryBackoff  time.Duration
	Timestamp     time.Time
	RequestID     string
	CorrelationID string
}

// NewStandardizedError creates a new standardized error
func NewStandardizedError(category ErrorCategory, code, message string) *StandardizedError {
	return &StandardizedError{
		Category:  category,
		Code:      code,
		Message:   message,
		Context:   make(map[string]interface{}),
		Timestamp: time.Now(),
		Severity:  SeverityMedium, // Default severity
	}
}

// WithSeverity sets the error severity
func (e *StandardizedError) WithSeverity(severity ErrorSeverity) *StandardizedError {
	e.Severity = severity
	return e
}

// WithComponent sets the component that generated the error
func (e *StandardizedError) WithComponent(component string) *StandardizedError {
	e.Component = component
	return e
}

// WithRetryConfig configures retry behavior
func (e *StandardizedError) WithRetryConfig(retryable bool, maxRetries int, backoff time.Duration) *StandardizedError {
	e.Retryable = retryable
	e.MaxRetries = maxRetries
	e.RetryBackoff = backoff
	return e
}

// WithContext adds context information
func (e *StandardizedError) WithContext(key string, value interface{}) *StandardizedError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// WithRequestID sets the request ID
func (e *StandardizedError) WithRequestID(requestID string) *StandardizedError {
	e.RequestID = requestID
	return e
}

// WithCorrelationID sets the correlation ID
func (e *StandardizedError) WithCorrelationID(correlationID string) *StandardizedError {
	e.CorrelationID = correlationID
	return e
}

// WithRecovery adds recovery suggestions
func (e *StandardizedError) WithRecovery(actions ...string) *StandardizedError {
	e.Recovery = append(e.Recovery, actions...)
	return e
}

// Error implements the error interface
func (e *StandardizedError) Error() string {
	return fmt.Sprintf("[%s:%s] %s (component: %s)", e.Category, e.Code, e.Message, e.Component)
}

// IsRetryable returns whether the error is retryable
func (e *StandardizedError) IsRetryable() bool {
	return e.Retryable
}

// GetRetryDelay returns the suggested retry delay
func (e *StandardizedError) GetRetryDelay() time.Duration {
	return e.RetryBackoff
}

// GetSeverity returns the error severity
func (e *StandardizedError) GetSeverity() ErrorSeverity {
	return e.Severity
}

// ToMap converts the error to a map for logging/serialization
func (e *StandardizedError) ToMap() map[string]interface{} {
	result := map[string]interface{}{
		"category":  string(e.Category),
		"code":      e.Code,
		"message":   e.Message,
		"retryable": e.Retryable,
		"severity":  string(e.Severity),
		"component": e.Component,
		"timestamp": e.Timestamp,
	}

	if e.MaxRetries > 0 {
		result["max_retries"] = e.MaxRetries
	}

	if e.RetryBackoff > 0 {
		result["retry_backoff"] = e.RetryBackoff.String()
	}

	if e.RequestID != "" {
		result["request_id"] = e.RequestID
	}

	if e.CorrelationID != "" {
		result["correlation_id"] = e.CorrelationID
	}

	if len(e.Recovery) > 0 {
		result["recovery_actions"] = e.Recovery
	}

	if len(e.Context) > 0 {
		result["context"] = e.Context
	}

	return result
}

// Utility functions for error category detection

// IsNetworkError checks if an error indicates network issues
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}
	errorStr := strings.ToLower(err.Error())
	networkPatterns := []string{
		"connection refused",
		"connection reset",
		"no route to host",
		"network is unreachable",
		"timeout",
		"dns",
	}

	for _, pattern := range networkPatterns {
		if strings.Contains(errorStr, pattern) {
			return true
		}
	}
	return false
}

// IsTimeoutError checks if an error indicates a timeout
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	errorStr := strings.ToLower(err.Error())
	timeoutPatterns := []string{
		"timeout",
		"deadline exceeded",
		"context canceled",
		"context deadline exceeded",
	}

	for _, pattern := range timeoutPatterns {
		if strings.Contains(errorStr, pattern) {
			return true
		}
	}
	return false
}

// IsRetryableError determines if an error should be retried
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for common retryable patterns
	retryablePatterns := []string{
		"connection refused",
		"timeout",
		"temporary failure",
		"service unavailable",
		"too many requests",
		"rate limit",
		"circuit breaker",
	}

	errorStr := strings.ToLower(err.Error())
	for _, pattern := range retryablePatterns {
		if strings.Contains(errorStr, pattern) {
			return true
		}
	}

	return false
}

// CategorizeError attempts to categorize a generic error
func CategorizeError(err error) ErrorCategory {
	if err == nil {
		return ValidationError
	}

	errorStr := strings.ToLower(err.Error())

	// Network-related errors
	if IsNetworkError(err) {
		return NetworkError
	}

	// Timeout-related errors
	if IsTimeoutError(err) {
		return TimeoutError
	}

	// Authentication errors
	authPatterns := []string{"unauthorized", "forbidden", "permission denied", "access denied"}
	for _, pattern := range authPatterns {
		if strings.Contains(errorStr, pattern) {
			return AuthError
		}
	}

	// Rate limiting errors
	rateLimitPatterns := []string{"rate limit", "too many requests", "quota exceeded"}
	for _, pattern := range rateLimitPatterns {
		if strings.Contains(errorStr, pattern) {
			return RateLimitError
		}
	}

	// Database errors
	databasePatterns := []string{"database", "connection", "sql", "transaction", "deadlock"}
	for _, pattern := range databasePatterns {
		if strings.Contains(errorStr, pattern) {
			return DatabaseError
		}
	}

	// Kubernetes API errors
	k8sPatterns := []string{"kubernetes", "k8s", "api server", "etcd", "pod", "deployment"}
	for _, pattern := range k8sPatterns {
		if strings.Contains(errorStr, pattern) {
			return K8sAPIError
		}
	}

	// SLM/ML service errors
	slmPatterns := []string{"model", "inference", "slm", "llm", "ai service"}
	for _, pattern := range slmPatterns {
		if strings.Contains(errorStr, pattern) {
			return SLMError
		}
	}

	// Default to validation error for unrecognized patterns
	return ValidationError
}

// ValidateErrorPattern checks if an error matches expected patterns
func ValidateErrorPattern(err error, expectedPattern string) bool {
	if err == nil {
		return expectedPattern == ""
	}

	matched, regexErr := regexp.MatchString(expectedPattern, err.Error())
	if regexErr != nil {
		// If regex is invalid, fall back to simple string matching
		return strings.Contains(strings.ToLower(err.Error()), strings.ToLower(expectedPattern))
	}

	return matched
}
