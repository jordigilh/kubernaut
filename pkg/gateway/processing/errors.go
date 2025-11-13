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

package processing

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// RetryError wraps an error with retry context for better debugging.
// GAP 10: Error Wrapping
// BR-GATEWAY-112: Error Classification
type RetryError struct {
	Attempt      int    // Current attempt number (1-based)
	MaxAttempts  int    // Maximum retry attempts configured
	OriginalErr  error  // The original error that caused retry exhaustion
	ErrorType    string // "retryable" or "non-retryable"
	ErrorCode    int    // HTTP status code (if applicable)
	ErrorMessage string // Human-readable error message
}

// Error implements the error interface.
func (e *RetryError) Error() string {
	if e.ErrorCode > 0 {
		return fmt.Sprintf("retry exhausted after %d/%d attempts (error type: %s, HTTP %d): %s",
			e.Attempt, e.MaxAttempts, e.ErrorType, e.ErrorCode, e.ErrorMessage)
	}
	return fmt.Sprintf("retry exhausted after %d/%d attempts (error type: %s): %s",
		e.Attempt, e.MaxAttempts, e.ErrorType, e.ErrorMessage)
}

// Unwrap returns the original error for error chain inspection.
func (e *RetryError) Unwrap() error {
	return e.OriginalErr
}

// IsRetryableError determines if a K8s API error should be retried.
// BR-GATEWAY-112: Error Classification (retryable errors)
// Reliability-First Design: Always retry transient errors
//
// Retryable errors are transient failures that may succeed on retry:
// - HTTP 429: Too Many Requests (rate limiting) - ALWAYS RETRY
// - HTTP 503: Service Unavailable (API server overload) - ALWAYS RETRY
// - HTTP 504: Gateway Timeout (API server slow response) - ALWAYS RETRY
// - Timeout errors: Network latency, deadline exceeded - ALWAYS RETRY
// - Connection errors: Connection refused, connection reset - ALWAYS RETRY
//
// Returns true if the error should be retried (no configuration needed).
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for K8s API status errors
	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		statusCode := statusErr.ErrStatus.Code

		// HTTP 429 - Too Many Requests (rate limiting) - ALWAYS RETRY
		if statusCode == http.StatusTooManyRequests {
			return true
		}

		// HTTP 503 - Service Unavailable (API server overload) - ALWAYS RETRY
		if statusCode == http.StatusServiceUnavailable {
			return true
		}

		// HTTP 504 - Gateway Timeout (API server slow response) - ALWAYS RETRY
		if statusCode == http.StatusGatewayTimeout {
			return true
		}
	}

	// Check for timeout errors (string matching as fallback) - ALWAYS RETRY
	errStr := err.Error()
	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "context deadline exceeded") {
		return true
	}

	// Check for temporary network errors - ALWAYS RETRY
	// These are transient and may succeed on retry
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "broken pipe") {
		return true
	}

	return false
}

// IsNonRetryableError determines if an error should never be retried.
// BR-GATEWAY-112: Error Classification (non-retryable errors)
//
// Non-retryable errors are permanent failures that will not succeed on retry:
// - HTTP 400: Bad Request (validation error - fix the request)
// - HTTP 403: Forbidden (RBAC error - fix permissions)
// - HTTP 422: Unprocessable Entity (schema validation - fix the CRD)
// - HTTP 409: Conflict (already exists - idempotent, not an error)
// - HTTP 404: Not Found (namespace doesn't exist - use fallback)
//
// Returns true if the error should NOT be retried.
func IsNonRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		statusCode := statusErr.ErrStatus.Code

		// HTTP 400 - Bad Request (validation error)
		if statusCode == http.StatusBadRequest {
			return true
		}

		// HTTP 403 - Forbidden (RBAC error)
		if statusCode == http.StatusForbidden {
			return true
		}

		// HTTP 422 - Unprocessable Entity (schema validation)
		if statusCode == http.StatusUnprocessableEntity {
			return true
		}

		// HTTP 409 - Conflict (already exists)
		if statusCode == http.StatusConflict {
			return true
		}

		// HTTP 404 - Not Found (namespace doesn't exist)
		// Note: This is handled separately with fallback namespace logic
		if statusCode == http.StatusNotFound {
			return true
		}
	}

	return false
}

// GetErrorCode extracts the HTTP status code from a K8s API error.
// Returns 0 if the error is not a K8s API status error.
func GetErrorCode(err error) int {
	if err == nil {
		return 0
	}

	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		return int(statusErr.ErrStatus.Code)
	}

	return 0
}

// GetErrorMessage extracts a human-readable error message.
func GetErrorMessage(err error) string {
	if err == nil {
		return ""
	}

	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		return statusErr.ErrStatus.Message
	}

	return err.Error()
}
