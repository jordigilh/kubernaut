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

package aianalysis

import (
	"context"
	"errors"
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	hgptclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// ========================================
// ERROR CLASSIFIER UNIT TESTS
// ========================================
//
// Business Requirements:
// - BR-AI-009: Error classification and handling
// - BR-AI-010: Retry logic for transient failures
//
// Test Tier: UNIT (70%+ coverage)
// Testing Pattern: Mock ALL external dependencies (HAPI client is mocked)
//
// Test Plan Mapping:
// - AA-UNIT-ERR-001 to AA-UNIT-ERR-016 (16 error classification tests)
//   Moved from integration tier per assessment: "no test logic in production code"
//   See: docs/handoff/AA_MOCK_HAPI_ERROR_INJECTION_ASSESSMENT_DEC_24_2025.md
//
// TDD Approach:
// - RED: Test fails because error classifier doesn't classify correctly
// - GREEN: Test passes with correct classification
// - REFACTOR: Enhance classification logic for edge cases
//
// ========================================

var _ = Describe("ErrorClassifier", func() {
	var (
		errorClassifier *handlers.ErrorClassifier
		logger          logr.Logger
	)

	BeforeEach(func() {
		logger = logr.Discard() // Test logger (silent)
		errorClassifier = handlers.NewErrorClassifier(logger)
		Expect(errorClassifier).ToNot(BeNil(), "ErrorClassifier should be created successfully")
	})

	// ========================================
	// HTTP ERROR CLASSIFICATION (12 tests)
	// BR-AI-009: Classify HTTP errors by status code
	// ========================================

	Context("HTTP Error Classification - BR-AI-009", func() {
		// ========================================
		// AA-UNIT-ERR-001: 401 Unauthorized → Authentication Error
		// ========================================
		It("should classify 401 as Authentication Error", func() {
			// Arrange
			apiErr := &hgptclient.APIError{
				StatusCode: 401,
				Message:    "Unauthorized",
			}

			// Act
			classification := errorClassifier.ClassifyError(apiErr)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypeAuthentication),
				"401 errors indicate missing or invalid authentication credentials")
			Expect(classification.IsRetryable).To(BeFalse(),
				"Authentication errors require fixing credentials, not retry")
			Expect(classification.ShouldAlert).To(BeTrue(),
				"Authentication failures require immediate attention")
			Expect(classification.RetryAfter).To(Equal(time.Duration(-1)),
				"No retry delay for non-retryable errors")
			Expect(classification.Message).To(ContainSubstring("Authentication failed"),
				"Error message should indicate authentication failure")
		})

		// ========================================
		// AA-UNIT-ERR-002: 403 Forbidden → Authorization Error
		// ========================================
		It("should classify 403 as Authorization Error", func() {
			// Arrange
			apiErr := &hgptclient.APIError{
				StatusCode: 403,
				Message:    "Forbidden",
			}

			// Act
			classification := errorClassifier.ClassifyError(apiErr)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypeAuthorization),
				"403 errors indicate insufficient permissions")
			Expect(classification.IsRetryable).To(BeFalse(),
				"Authorization errors require permission changes, not retry")
			Expect(classification.ShouldAlert).To(BeTrue(),
				"Authorization failures require immediate attention")
			Expect(classification.Message).To(ContainSubstring("Authorization failed"))
		})

		// ========================================
		// AA-UNIT-ERR-003: 404 Not Found → Configuration Error
		// ========================================
		It("should classify 404 as Configuration Error", func() {
			// Arrange
			apiErr := &hgptclient.APIError{
				StatusCode: 404,
				Message:    "Endpoint not found",
			}

			// Act
			classification := errorClassifier.ClassifyError(apiErr)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypeConfiguration),
				"404 errors indicate incorrect endpoint configuration")
			Expect(classification.IsRetryable).To(BeFalse(),
				"Configuration errors require fixing configuration, not retry")
			Expect(classification.ShouldAlert).To(BeTrue(),
				"Configuration errors require immediate attention")
			Expect(classification.Message).To(ContainSubstring("Resource not found"))
		})

		// ========================================
		// AA-UNIT-ERR-004: 429 Too Many Requests → Rate Limit Error
		// ========================================
		It("should classify 429 as Rate Limit Error", func() {
			// Arrange
			apiErr := &hgptclient.APIError{
				StatusCode: 429,
				Message:    "Too many requests",
			}

			// Act
			classification := errorClassifier.ClassifyError(apiErr)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypeRateLimit),
				"429 errors indicate rate limiting")
			Expect(classification.IsRetryable).To(BeTrue(),
				"Rate limit errors are transient and retryable")
			Expect(classification.ShouldAlert).To(BeFalse(),
				"Rate limiting is expected behavior, no alert needed")
			Expect(classification.RetryAfter).To(BeNumerically(">", 0),
				"Should provide retry delay for rate limit errors")
			Expect(classification.Message).To(ContainSubstring("Rate limit exceeded"))
		})

		// ========================================
		// AA-UNIT-ERR-008: 500 Internal Server Error → Transient Error
		// ========================================
		It("should classify 500 as Transient Error", func() {
			// Arrange
			apiErr := &hgptclient.APIError{
				StatusCode: 500,
				Message:    "Internal server error",
			}

			// Act
			classification := errorClassifier.ClassifyError(apiErr)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypeTransient),
				"500 errors are typically transient server issues")
			Expect(classification.IsRetryable).To(BeTrue(),
				"Transient errors should be retried")
			Expect(classification.ShouldAlert).To(BeFalse(),
				"Transient errors are expected and handled by retry logic")
			Expect(classification.RetryAfter).To(BeNumerically(">", 0),
				"Should provide retry delay for transient errors")
			Expect(classification.Message).To(ContainSubstring("Server error (HTTP 500)"))
		})

		// ========================================
		// AA-UNIT-ERR-009: 502 Bad Gateway → Transient Error
		// ========================================
		It("should classify 502 as Transient Error", func() {
			// Arrange
			apiErr := &hgptclient.APIError{
				StatusCode: 502,
				Message:    "Bad gateway",
			}

			// Act
			classification := errorClassifier.ClassifyError(apiErr)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypeTransient))
			Expect(classification.IsRetryable).To(BeTrue())
			Expect(classification.ShouldAlert).To(BeFalse())
			Expect(classification.Message).To(ContainSubstring("Server error (HTTP 502)"))
		})

		// ========================================
		// AA-UNIT-ERR-010: 503 Service Unavailable → Transient Error
		// ========================================
		It("should classify 503 as Transient Error", func() {
			// Arrange
			apiErr := &hgptclient.APIError{
				StatusCode: 503,
				Message:    "Service unavailable",
			}

			// Act
			classification := errorClassifier.ClassifyError(apiErr)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypeTransient))
			Expect(classification.IsRetryable).To(BeTrue())
			Expect(classification.ShouldAlert).To(BeFalse())
			Expect(classification.Message).To(ContainSubstring("Server error (HTTP 503)"))
		})

		// ========================================
		// AA-UNIT-ERR-011: 504 Gateway Timeout → Transient Error
		// ========================================
		It("should classify 504 as Transient Error", func() {
			// Arrange
			apiErr := &hgptclient.APIError{
				StatusCode: 504,
				Message:    "Gateway timeout",
			}

			// Act
			classification := errorClassifier.ClassifyError(apiErr)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypeTransient))
			Expect(classification.IsRetryable).To(BeTrue())
			Expect(classification.ShouldAlert).To(BeFalse())
			Expect(classification.Message).To(ContainSubstring("Server error (HTTP 504)"))
		})

		// ========================================
		// AA-UNIT-ERR-005: 400 Bad Request → Permanent Error
		// ========================================
		It("should classify 400 as Permanent Error", func() {
			// Arrange
			apiErr := &hgptclient.APIError{
				StatusCode: 400,
				Message:    "Bad request - invalid parameter",
			}

			// Act
			classification := errorClassifier.ClassifyError(apiErr)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypePermanent),
				"400 errors indicate invalid request that won't succeed on retry")
			Expect(classification.IsRetryable).To(BeFalse(),
				"Permanent errors should not be retried")
			Expect(classification.ShouldAlert).To(BeTrue(),
				"Permanent errors indicate code bugs requiring attention")
			Expect(classification.Message).To(ContainSubstring("Client error (HTTP 400)"))
		})

		// ========================================
		// AA-UNIT-ERR-006: 422 Unprocessable Entity → Permanent Error
		// ========================================
		It("should classify 422 as Permanent Error", func() {
			// Arrange
			apiErr := &hgptclient.APIError{
				StatusCode: 422,
				Message:    "Validation failed",
			}

			// Act
			classification := errorClassifier.ClassifyError(apiErr)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypePermanent))
			Expect(classification.IsRetryable).To(BeFalse())
			Expect(classification.ShouldAlert).To(BeTrue())
			Expect(classification.Message).To(ContainSubstring("Client error (HTTP 422)"))
		})

		// ========================================
		// AA-UNIT-ERR-007: 418 I'm a teapot → Transient Error (unknown code)
		// ========================================
		It("should classify unknown HTTP status codes as Transient with alert", func() {
			// Arrange
			apiErr := &hgptclient.APIError{
				StatusCode: 418,
				Message:    "I'm a teapot",
			}

			// Act
			classification := errorClassifier.ClassifyError(apiErr)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypeTransient),
				"Unknown status codes should be treated as transient to allow recovery")
			Expect(classification.IsRetryable).To(BeTrue())
			Expect(classification.ShouldAlert).To(BeTrue(),
				"Unknown status codes should alert for investigation")
			Expect(classification.Message).To(ContainSubstring("Unknown HTTP status 418"))
		})
	})

	// ========================================
	// NETWORK ERROR CLASSIFICATION (3 tests)
	// BR-AI-009: Classify network-level errors
	// ========================================

	Context("Network Error Classification - BR-AI-009", func() {
		// ========================================
		// AA-UNIT-ERR-012: context.DeadlineExceeded → Timeout Error
		// ========================================
		It("should classify context.DeadlineExceeded as Timeout Error", func() {
			// Arrange
			err := context.DeadlineExceeded

			// Act
			classification := errorClassifier.ClassifyError(err)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypeTimeout),
				"context.DeadlineExceeded indicates request timeout")
			Expect(classification.IsRetryable).To(BeTrue(),
				"Timeouts are transient and retryable")
			Expect(classification.ShouldAlert).To(BeFalse(),
				"Occasional timeouts are expected")
			Expect(classification.RetryAfter).To(BeNumerically(">", 0),
				"Should provide retry delay for timeout errors")
			Expect(classification.Message).To(ContainSubstring("timed out or was canceled"))
		})

		// ========================================
		// AA-UNIT-ERR-013: connection refused → Network Error
		// ========================================
		It("should classify connection refused as Network Error", func() {
			// Arrange
			err := errors.New("dial tcp 127.0.0.1:8080: connect: connection refused")

			// Act
			classification := errorClassifier.ClassifyError(err)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypeNetwork),
				"Connection refused indicates service unavailable")
			Expect(classification.IsRetryable).To(BeTrue(),
				"Service may become available after retry")
			Expect(classification.ShouldAlert).To(BeTrue(),
				"Connection refused indicates service down, requires alert")
			Expect(classification.Message).To(ContainSubstring("Connection refused"))
		})

		// ========================================
		// AA-UNIT-ERR-014: DNS resolution failure → Network Error
		// ========================================
		It("should classify DNS resolution failure as Network Error", func() {
			// Arrange
			dnsErr := &net.DNSError{
				Err:        "no such host",
				Name:       "nonexistent.example.com",
				IsNotFound: true,
			}

			// Act
			classification := errorClassifier.ClassifyError(dnsErr)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypeNetwork),
				"DNS resolution failure is a network-level error")
			Expect(classification.IsRetryable).To(BeTrue(),
				"DNS issues may be transient")
			Expect(classification.ShouldAlert).To(BeTrue(),
				"DNS failures may indicate configuration issues requiring attention")
			Expect(classification.Message).To(ContainSubstring("DNS resolution failed"))
			Expect(classification.Message).To(ContainSubstring("nonexistent.example.com"))
		})

		// ========================================
		// AA-UNIT-ERR-015: Network timeout → Timeout Error
		// ========================================
		It("should classify network timeout as Timeout Error", func() {
			// Arrange
			netErr := &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: &timeoutError{},
			}

			// Act
			classification := errorClassifier.ClassifyError(netErr)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypeTimeout),
				"Network timeouts should be classified as Timeout")
			Expect(classification.IsRetryable).To(BeTrue())
			Expect(classification.ShouldAlert).To(BeFalse())
			Expect(classification.Message).To(ContainSubstring("Network timeout"))
		})

		// ========================================
		// AA-UNIT-ERR-016: Generic network error → Network Error
		// ========================================
		It("should classify generic network error as Network Error", func() {
			// Arrange
			netErr := &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: errors.New("network unreachable"),
			}

			// Act
			classification := errorClassifier.ClassifyError(netErr)

			// Assert
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypeNetwork))
			Expect(classification.IsRetryable).To(BeTrue())
			Expect(classification.ShouldAlert).To(BeFalse())
			Expect(classification.Message).To(ContainSubstring("Network connectivity error"))
		})
	})

	// ========================================
	// RETRY STRATEGY TESTS (BR-AI-010)
	// ========================================

	Context("Retry Strategy - BR-AI-010", func() {
		// ========================================
		// AA-UNIT-ERR-017: GetRetryDelay() exponential backoff calculation
		// ========================================
		It("should calculate exponential backoff delays correctly with jitter", func() {
			// BR-AI-010: Exponential backoff formula verification
			// Design Decision: DD-SHARED-001 - Shared Backoff Library
			// Formula: min(BasePeriod * (Multiplier ^ attemptCount), MaxPeriod) ± jitter
			// Defaults: BasePeriod=1s, Multiplier=2.0, MaxPeriod=5min, Jitter=±10%

			By("Attempt 0: ~1s (0.9-1.1s with ±10% jitter)")
			delay0 := errorClassifier.GetRetryDelay(0)
			Expect(delay0).To(BeNumerically(">=", 900*time.Millisecond),
				"Delay should be at least 1s - 10% = 0.9s")
			Expect(delay0).To(BeNumerically("<=", 1100*time.Millisecond),
				"Delay should be at most 1s + 10% = 1.1s")

			By("Attempt 1: ~2s (1.8-2.2s with ±10% jitter)")
			delay1 := errorClassifier.GetRetryDelay(1)
			Expect(delay1).To(BeNumerically(">=", 1800*time.Millisecond),
				"Delay should be at least 2s - 10% = 1.8s")
			Expect(delay1).To(BeNumerically("<=", 2200*time.Millisecond),
				"Delay should be at most 2s + 10% = 2.2s")

			By("Attempt 2: ~4s (3.6-4.4s with ±10% jitter)")
			delay2 := errorClassifier.GetRetryDelay(2)
			Expect(delay2).To(BeNumerically(">=", 3600*time.Millisecond),
				"Delay should be at least 4s - 10% = 3.6s")
			Expect(delay2).To(BeNumerically("<=", 4400*time.Millisecond),
				"Delay should be at most 4s + 10% = 4.4s")

			By("Attempt 3: ~8s (7.2-8.8s with ±10% jitter)")
			delay3 := errorClassifier.GetRetryDelay(3)
			Expect(delay3).To(BeNumerically(">=", 7200*time.Millisecond),
				"Delay should be at least 8s - 10% = 7.2s")
			Expect(delay3).To(BeNumerically("<=", 8800*time.Millisecond),
				"Delay should be at most 8s + 10% = 8.8s")

			By("Attempt 4: ~16s (14.4-17.6s with ±10% jitter)")
			delay4 := errorClassifier.GetRetryDelay(4)
			Expect(delay4).To(BeNumerically(">=", 14400*time.Millisecond),
				"Delay should be at least 16s - 10% = 14.4s")
			Expect(delay4).To(BeNumerically("<=", 17600*time.Millisecond),
				"Delay should be at most 16s + 10% = 17.6s")

			By("Attempt 10: capped at ~5 minutes with jitter (270-330s)")
			delay10 := errorClassifier.GetRetryDelay(10)
			Expect(delay10).To(BeNumerically(">=", 270*time.Second),
				"Delay should be at least 5min - 10% = 4.5min = 270s")
			Expect(delay10).To(BeNumerically("<=", 330*time.Second),
				"Delay should be capped at 5min + 10% = 5.5min = 330s (but actually capped at MaxPeriod)")

			// Business Outcome: Anti-thundering herd protection
			// When 100 AIAnalysis instances fail simultaneously:
			// - WITHOUT jitter: All 100 retry at EXACTLY 1s → API overload
			// - WITH ±10% jitter: 100 retry spread over 0.9-1.1s → manageable load
		})

		It("should handle negative attempt counts gracefully", func() {
			delay := errorClassifier.GetRetryDelay(-1)
			// Negative attempts should be treated as attempt 0 (1s base) with ±10% jitter
			expectedMin := time.Duration(float64(1*time.Second) * 0.9) // 0.9s
			expectedMax := time.Duration(float64(1*time.Second) * 1.1) // 1.1s
			Expect(delay).To(BeNumerically(">=", expectedMin),
				"Delay should be >= 0.9s (1s - 10% jitter)")
			Expect(delay).To(BeNumerically("<=", expectedMax),
				"Delay should be <= 1.1s (1s + 10% jitter)")
		})

		It("should determine retryability correctly", func() {
			By("Retryable error (transient)")
			transientClassification := handlers.ErrorClassification{
				IsRetryable: true,
			}
			Expect(errorClassifier.IsRetryable(transientClassification)).To(BeTrue())

			By("Non-retryable error (permanent)")
			permanentClassification := handlers.ErrorClassification{
				IsRetryable: false,
			}
			Expect(errorClassifier.IsRetryable(permanentClassification)).To(BeFalse())
		})

		It("should respect max retries limit", func() {
			retryableClassification := handlers.ErrorClassification{
				IsRetryable: true,
			}

			By("Attempt 0: should retry")
			Expect(errorClassifier.ShouldRetry(retryableClassification, 0)).To(BeTrue())

			By("Attempt 4: should retry (< maxRetries=5)")
			Expect(errorClassifier.ShouldRetry(retryableClassification, 4)).To(BeTrue())

			By("Attempt 5: should NOT retry (>= maxRetries=5)")
			Expect(errorClassifier.ShouldRetry(retryableClassification, 5)).To(BeFalse())

			By("Attempt 10: should NOT retry (>> maxRetries)")
			Expect(errorClassifier.ShouldRetry(retryableClassification, 10)).To(BeFalse())
		})

		It("should never retry non-retryable errors regardless of attempt count", func() {
			nonRetryableClassification := handlers.ErrorClassification{
				IsRetryable: false,
			}

			Expect(errorClassifier.ShouldRetry(nonRetryableClassification, 0)).To(BeFalse())
			Expect(errorClassifier.ShouldRetry(nonRetryableClassification, 3)).To(BeFalse())
		})
	})

	// ========================================
	// EDGE CASES
	// ========================================

	Context("Edge Cases", func() {
		It("should handle nil error gracefully", func() {
			classification := errorClassifier.ClassifyError(nil)
			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypePermanent))
			Expect(classification.IsRetryable).To(BeFalse())
			Expect(classification.Message).To(ContainSubstring("no error"))
		})

		It("should handle unknown error types", func() {
			unknownErr := errors.New("completely unknown error type")
			classification := errorClassifier.ClassifyError(unknownErr)

			Expect(classification.ErrorType).To(Equal(handlers.ErrorTypePermanent),
				"Unknown errors should be classified as permanent by default")
			Expect(classification.IsRetryable).To(BeFalse())
			Expect(classification.ShouldAlert).To(BeTrue(),
				"Unknown errors should alert for investigation")
		})
	})
})

// ========================================
// TEST HELPERS
// ========================================

// timeoutError implements net.Error for testing timeout scenarios
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }
