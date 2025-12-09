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

package audit

import (
	"errors"
	"fmt"
)

// ========================================
// AUDIT ERROR TYPES (GAP-11)
// 📋 Design Decision: DD-AUDIT-002 | BR-AUDIT-001
// Authority: ADR-032 "No Audit Loss"
// ========================================
//
// These error types enable differentiation between:
// - Client errors (4xx): Invalid request, should NOT retry
// - Server errors (5xx): Server failure, SHOULD retry
// - Network errors: Connection failure, SHOULD retry
//
// This supports the BufferedAuditStore retry logic:
// - 4xx errors → Don't retry, move to DLQ with "invalid" reason
// - 5xx errors → Retry with exponential backoff
// - Network errors → Retry with exponential backoff
//
// Defense-in-Depth Testing:
// - Unit tests: test/unit/audit/http_client_test.go
// - Unit tests: test/unit/audit/errors_test.go
// ========================================

// RetryableError interface allows checking if an error should trigger retry
// This is the primary interface used by BufferedAuditStore to determine retry behavior
type RetryableError interface {
	error
	IsRetryable() bool
}

// HTTPError represents an HTTP error from the Data Storage Service
// It includes the status code to enable differentiation between 4xx and 5xx errors
type HTTPError struct {
	StatusCode int
	Message    string
}

// Error implements the error interface
func (e *HTTPError) Error() string {
	return fmt.Sprintf("Data Storage Service returned status %d: %s", e.StatusCode, e.Message)
}

// IsRetryable returns true for 5xx errors (server errors)
// 4xx errors (client errors) are NOT retryable - they indicate invalid data
// GAP-11: Error differentiation for retry logic
func (e *HTTPError) IsRetryable() bool {
	return e.StatusCode >= 500 && e.StatusCode < 600
}

// Is4xxError returns true for client errors (400-499)
// Used to identify validation failures that should go to DLQ without retry
func (e *HTTPError) Is4xxError() bool {
	return e.StatusCode >= 400 && e.StatusCode < 500
}

// Is5xxError returns true for server errors (500-599)
// Used to identify server failures that should trigger retry
func (e *HTTPError) Is5xxError() bool {
	return e.StatusCode >= 500 && e.StatusCode < 600
}

// NetworkError represents a network-level error (connection failure, timeout)
// Network errors are always retryable
type NetworkError struct {
	Underlying error
}

// Error implements the error interface
func (e *NetworkError) Error() string {
	return fmt.Sprintf("network error: %v", e.Underlying)
}

// IsRetryable returns true - network errors should always be retried
func (e *NetworkError) IsRetryable() bool {
	return true
}

// Unwrap returns the underlying error
func (e *NetworkError) Unwrap() error {
	return e.Underlying
}

// MarshalError represents a JSON marshaling error
// Marshal errors are NOT retryable - they indicate code bugs or data issues
type MarshalError struct {
	Underlying error
}

// Error implements the error interface
func (e *MarshalError) Error() string {
	return fmt.Sprintf("failed to marshal audit event: %v", e.Underlying)
}

// IsRetryable returns false - marshal errors cannot be fixed by retry
func (e *MarshalError) IsRetryable() bool {
	return false
}

// Unwrap returns the underlying error
func (e *MarshalError) Unwrap() error {
	return e.Underlying
}

// NewHTTPError creates a new HTTPError with the given status code and message
func NewHTTPError(statusCode int, message string) *HTTPError {
	return &HTTPError{
		StatusCode: statusCode,
		Message:    message,
	}
}

// NewNetworkError creates a new NetworkError wrapping the underlying error
func NewNetworkError(err error) *NetworkError {
	return &NetworkError{
		Underlying: err,
	}
}

// NewMarshalError creates a new MarshalError wrapping the underlying error
func NewMarshalError(err error) *MarshalError {
	return &MarshalError{
		Underlying: err,
	}
}

// IsRetryable checks if an error should trigger retry
// This is the main entry point for retry logic to use
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check if error implements RetryableError interface
	var retryable RetryableError
	if errors.As(err, &retryable) {
		return retryable.IsRetryable()
	}

	// Default: unknown errors are retryable (fail-safe)
	return true
}

// Is4xxError checks if an error is a 4xx HTTP error
func Is4xxError(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Is4xxError()
	}
	return false
}

// Is5xxError checks if an error is a 5xx HTTP error
func Is5xxError(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Is5xxError()
	}
	return false
}

