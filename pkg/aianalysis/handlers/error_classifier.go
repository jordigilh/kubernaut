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

// Package handlers provides error classification and retry logic for AIAnalysis.
//
// Business Requirements:
// - BR-AI-009: Error classification and handling
// - BR-AI-010: Retry logic for transient failures
//
// Design Decision: DD-AIANALYSIS-006 - Error Classification Pattern
// Error classification follows these principles:
// - ✅ Classify by cause (authentication, network, transient, permanent)
// - ✅ Determine retry strategy based on classification
// - ✅ Track error patterns for monitoring and alerting
// - ✅ Provide clear error messages for debugging
package handlers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-logr/logr"
	hgptclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
)

// ErrorType represents the classification of an error
type ErrorType string

const (
	// ErrorTypeAuthentication indicates authentication failure
	ErrorTypeAuthentication ErrorType = "Authentication"

	// ErrorTypeAuthorization indicates authorization/permission failure
	ErrorTypeAuthorization ErrorType = "Authorization"

	// ErrorTypeConfiguration indicates configuration error (e.g., 404 Not Found)
	ErrorTypeConfiguration ErrorType = "Configuration"

	// ErrorTypeRateLimit indicates rate limiting
	ErrorTypeRateLimit ErrorType = "RateLimit"

	// ErrorTypeTransient indicates temporary/transient failure
	ErrorTypeTransient ErrorType = "Transient"

	// ErrorTypeTimeout indicates request timeout
	ErrorTypeTimeout ErrorType = "Timeout"

	// ErrorTypeNetwork indicates network connectivity error
	ErrorTypeNetwork ErrorType = "Network"

	// ErrorTypePermanent indicates permanent failure (bad request, etc.)
	ErrorTypePermanent ErrorType = "Permanent"

	// ErrorTypeSessionLost indicates HAPI session was lost (404 on poll)
	// BR-AA-HAPI-064.5: Triggers session regeneration, not standard retry
	ErrorTypeSessionLost ErrorType = "SessionLost"
)

// ErrorClassification contains error analysis results
type ErrorClassification struct {
	// ErrorType is the classified error type
	ErrorType ErrorType

	// IsRetryable indicates if the error should be retried
	IsRetryable bool

	// ShouldAlert indicates if this error requires immediate attention
	ShouldAlert bool

	// RetryAfter suggests when to retry (0 = immediate, -1 = never)
	RetryAfter time.Duration

	// Message provides human-readable error description
	Message string

	// OriginalError is the underlying error
	OriginalError error
}

// ErrorClassifier handles error classification and retry strategies
//
// Design Decision: DD-SHARED-001 - Shared Backoff Library
// Uses pkg/shared/backoff for consistent exponential backoff with jitter
// across all services (AIAnalysis, Notification, WorkflowExecution, etc.)
type ErrorClassifier struct {
	log logr.Logger

	// Backoff configuration (DD-SHARED-001 compliant)
	backoffConfig backoff.Config
	maxRetries    int
}

// NewErrorClassifier creates a new ErrorClassifier with default configuration
//
// Default Configuration (DD-SHARED-001 compliant):
// - Base period: 1 second (faster than standard 30s for API errors)
// - Max period: 5 minutes
// - Multiplier: 2.0 (standard exponential)
// - Jitter: ±10% (anti-thundering herd protection)
// - Max retries: 5
//
// Exponential backoff sequence:
// - Attempt 1: ~1s (0.9-1.1s with jitter)
// - Attempt 2: ~2s (1.8-2.2s with jitter)
// - Attempt 3: ~4s (3.6-4.4s with jitter)
// - Attempt 4: ~8s (7.2-8.8s with jitter)
// - Attempt 5: ~16s (14.4-17.6s with jitter)
// - Attempt 6+: 300s (5 minutes max)
func NewErrorClassifier(logger logr.Logger) *ErrorClassifier {
	return &ErrorClassifier{
		log: logger,
		backoffConfig: backoff.Config{
			BasePeriod:    1 * time.Second,  // Fast initial retry for transient API errors
			MaxPeriod:     5 * time.Minute,  // Cap at 5 minutes
			Multiplier:    2.0,              // Standard exponential (power-of-2)
			JitterPercent: 10,               // ±10% variance (production anti-thundering herd)
		},
		maxRetries: 5,
	}
}

// ClassifyError determines the error type and retry strategy
//
// Business Requirement: BR-AI-009
// Classifies errors into categories for appropriate handling:
// - Authentication (401): Alert required, no retry
// - Authorization (403): Alert required, no retry
// - Configuration (404): Alert required, no retry
// - Rate Limit (429): Retry with backoff, no alert
// - Transient (5xx): Retry with backoff, no alert
// - Timeout: Retry with backoff, no alert
// - Network: Retry with backoff, no alert
// - Permanent (4xx): No retry, alert
func (ec *ErrorClassifier) ClassifyError(err error) ErrorClassification {
	if err == nil {
		return ErrorClassification{
			ErrorType:     ErrorTypePermanent,
			IsRetryable:   false,
			ShouldAlert:   false,
			RetryAfter:    -1,
			Message:       "no error",
			OriginalError: nil,
		}
	}

	// Check for timeout errors first (highest priority)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrorClassification{
			ErrorType:     ErrorTypeTimeout,
			IsRetryable:   true,
			ShouldAlert:   false,
			RetryAfter:    ec.backoffConfig.BasePeriod,
			Message:       "Request timed out or was canceled",
			OriginalError: err,
		}
	}

	// Check for DNS resolution errors (before generic net.Error)
	// *net.DNSError implements net.Error, so check this first for better classification
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return ErrorClassification{
			ErrorType:     ErrorTypeNetwork,
			IsRetryable:   true,
			ShouldAlert:   true, // DNS errors might indicate configuration issues
			RetryAfter:    ec.backoffConfig.BasePeriod,
			Message:       fmt.Sprintf("DNS resolution failed: %s", dnsErr.Name),
			OriginalError: err,
		}
	}

	// Check for network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrorClassification{
				ErrorType:     ErrorTypeTimeout,
				IsRetryable:   true,
				ShouldAlert:   false,
				RetryAfter:    ec.backoffConfig.BasePeriod,
				Message:       "Network timeout",
				OriginalError: err,
			}
		}
		return ErrorClassification{
			ErrorType:     ErrorTypeNetwork,
			IsRetryable:   true,
			ShouldAlert:   false,
			RetryAfter:    ec.backoffConfig.BasePeriod,
			Message:       "Network connectivity error",
			OriginalError: err,
		}
	}

	// Check for API errors (HTTP status codes)
	var apiErr *hgptclient.APIError
	if errors.As(err, &apiErr) {
		return ec.classifyHTTPError(apiErr)
	}

	// Check for connection refused errors
	if strings.Contains(err.Error(), "connection refused") {
		return ErrorClassification{
			ErrorType:     ErrorTypeNetwork,
			IsRetryable:   true,
			ShouldAlert:   true, // Service unavailable
			RetryAfter:    ec.backoffConfig.BasePeriod,
			Message:       "Connection refused - service unavailable",
			OriginalError: err,
		}
	}

	// Default: permanent error (unknown)
	return ErrorClassification{
		ErrorType:     ErrorTypePermanent,
		IsRetryable:   false,
		ShouldAlert:   true,
		RetryAfter:    -1,
		Message:       fmt.Sprintf("Unknown error: %v", err),
		OriginalError: err,
	}
}

// classifyHTTPError classifies HTTP status code errors
func (ec *ErrorClassifier) classifyHTTPError(apiErr *hgptclient.APIError) ErrorClassification {
	switch apiErr.StatusCode {
	// 401 Unauthorized - Authentication error
	case 401:
		return ErrorClassification{
			ErrorType:     ErrorTypeAuthentication,
			IsRetryable:   false,
			ShouldAlert:   true,
			RetryAfter:    -1,
			Message:       fmt.Sprintf("Authentication failed: %s", apiErr.Message),
			OriginalError: apiErr,
		}

	// 403 Forbidden - Authorization error
	case 403:
		return ErrorClassification{
			ErrorType:     ErrorTypeAuthorization,
			IsRetryable:   false,
			ShouldAlert:   true,
			RetryAfter:    -1,
			Message:       fmt.Sprintf("Authorization failed: %s", apiErr.Message),
			OriginalError: apiErr,
		}

	// 404 Not Found - Configuration error
	case 404:
		return ErrorClassification{
			ErrorType:     ErrorTypeConfiguration,
			IsRetryable:   false,
			ShouldAlert:   true,
			RetryAfter:    -1,
			Message:       fmt.Sprintf("Resource not found: %s", apiErr.Message),
			OriginalError: apiErr,
		}

	// 422 Unprocessable Entity - Validation error (permanent)
	case 422:
		return ErrorClassification{
			ErrorType:     ErrorTypePermanent,
			IsRetryable:   false,
			ShouldAlert:   true,
			RetryAfter:    -1,
			Message:       fmt.Sprintf("Client error (HTTP 422): %s", apiErr.Message),
			OriginalError: apiErr,
		}

	// 429 Too Many Requests - Rate limit error
	case 429:
		return ErrorClassification{
			ErrorType:     ErrorTypeRateLimit,
			IsRetryable:   true,
			ShouldAlert:   false, // Rate limiting is expected behavior
			RetryAfter:    ec.backoffConfig.BasePeriod * 2, // Wait longer for rate limits
			Message:       fmt.Sprintf("Rate limit exceeded: %s", apiErr.Message),
			OriginalError: apiErr,
		}

	// 5xx Server Errors - Transient errors
	case 500, 502, 503, 504:
		return ErrorClassification{
			ErrorType:     ErrorTypeTransient,
			IsRetryable:   true,
			ShouldAlert:   false, // Transient errors are expected
			RetryAfter:    ec.backoffConfig.BasePeriod,
			Message:       fmt.Sprintf("Server error (HTTP %d): %s", apiErr.StatusCode, apiErr.Message),
			OriginalError: apiErr,
		}

	// 400 Bad Request - Permanent error (invalid request data)
	case 400:
		return ErrorClassification{
			ErrorType:     ErrorTypePermanent,
			IsRetryable:   false,
			ShouldAlert:   true,
			RetryAfter:    -1,
			Message:       fmt.Sprintf("Client error (HTTP 400): %s", apiErr.Message),
			OriginalError: apiErr,
		}

	// 4xx Client Errors (other than explicitly handled) - Treat as Transient with alert
	// Rationale: Unknown 4xx codes might be temporary issues or misconfigurations
	// that could be resolved, so give them a chance to recover
	default:
		// All unknown status codes (4xx, 5xx, or other) - treat as transient
		return ErrorClassification{
			ErrorType:     ErrorTypeTransient,
			IsRetryable:   true,
			ShouldAlert:   true, // Alert for investigation
			RetryAfter:    ec.backoffConfig.BasePeriod,
			Message:       fmt.Sprintf("Unknown HTTP status %d: %s", apiErr.StatusCode, apiErr.Message),
			OriginalError: apiErr,
		}
	}
}

// IsRetryable checks if an error should be retried
// Business Requirement: BR-AI-010
func (ec *ErrorClassifier) IsRetryable(classification ErrorClassification) bool {
	return classification.IsRetryable
}

// GetRetryDelay calculates exponential backoff delay using shared backoff library
//
// Business Requirement: BR-AI-010
// Design Decision: DD-SHARED-001 - Shared Backoff Library
//
// Uses pkg/shared/backoff for consistent exponential backoff with jitter.
// Formula: min(BasePeriod * (Multiplier ^ attemptCount), MaxPeriod) ± jitter
//
// Example with defaults (BasePeriod=1s, Multiplier=2.0, MaxPeriod=5min, Jitter=±10%):
// - Attempt 0: ~1s (0.9-1.1s with jitter)
// - Attempt 1: ~2s (1.8-2.2s with jitter)
// - Attempt 2: ~4s (3.6-4.4s with jitter)
// - Attempt 3: ~8s (7.2-8.8s with jitter)
// - Attempt 4: ~16s (14.4-17.6s with jitter)
// - Attempt 5: ~32s (28.8-35.2s with jitter)
// - ...
// - Attempt 9+: ~300s (5 minutes max, 270-330s with jitter)
//
// Jitter Benefits:
// - Prevents thundering herd when multiple AIAnalysis fail simultaneously
// - Distributes retry load over time for better API stability
// - Battle-tested in Notification v3.1 (0 issues in 6 months)
func (ec *ErrorClassifier) GetRetryDelay(attemptCount int) time.Duration {
	if attemptCount < 0 {
		attemptCount = 0
	}

	// Use shared backoff library (DD-SHARED-001 compliant)
	// Note: attemptCount is 0-based in our usage, backoff.Calculate expects 1-based
	return ec.backoffConfig.Calculate(int32(attemptCount + 1))
}

// GetMaxRetries returns the maximum number of retry attempts
func (ec *ErrorClassifier) GetMaxRetries() int {
	return ec.maxRetries
}

// ShouldRetry determines if retry should be attempted based on attempt count
// Returns true if attemptCount < maxRetries and error is retryable
func (ec *ErrorClassifier) ShouldRetry(classification ErrorClassification, attemptCount int) bool {
	if !classification.IsRetryable {
		return false
	}

	if attemptCount >= ec.maxRetries {
		ec.log.Info("Max retries reached", "attemptCount", attemptCount, "maxRetries", ec.maxRetries)
		return false
	}

	return true
}
