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

package notification

import (
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sony/gobreaker"

	"github.com/jordigilh/kubernaut/pkg/notification/retry"
	"github.com/jordigilh/kubernaut/pkg/shared/circuitbreaker"
)

// BR-NOT-052: Retry Policy - BEHAVIOR & CORRECTNESS Testing
// BR-NOT-061: Circuit Breaker - BEHAVIOR & CORRECTNESS Testing
//
// FOCUS: Test WHAT the system does (behavior), NOT HOW it does it (implementation)
// BEHAVIOR: Does it retry? Does it block requests? Does it apply backoff?
// CORRECTNESS: Are retries timed correctly? Are errors classified properly?

var _ = Describe("BR-NOT-052: Retry Policy", func() {
	var policy *retry.Policy

	BeforeEach(func() {
		policy = retry.NewPolicy(&retry.Config{
			MaxAttempts:       5,
			BaseBackoff:       30 * time.Second,
			MaxBackoff:        480 * time.Second,
			BackoffMultiplier: 2.0,
		})
	})

	// ==============================================
	// CATEGORY 1: Error Classification (BEHAVIOR)
	// BR-NOT-052: System must distinguish transient vs permanent errors
	// ==============================================

	Context("Error Classification - BEHAVIOR", func() {
		// TABLE-DRIVEN: Error classification determines retry behavior
		DescribeTable("should classify errors for retry decisions (BR-NOT-052: Error classification)",
			func(err error, expectedRetryable bool, businessReason string) {
				// BEHAVIOR: Error classification determines if retry happens
				retryable := policy.IsRetryable(err)
				Expect(retryable).To(Equal(expectedRetryable), businessReason)
			},
			// TRANSIENT ERRORS: Retry because issue might resolve
			Entry("network timeout - transient infrastructure issue",
				errors.New("network timeout"),
				true,
				"Network timeouts are transient - retry may succeed after network recovers"),
			Entry("503 service unavailable - service temporarily down",
				&retry.HTTPError{StatusCode: 503},
				true,
				"503 means service temporarily unavailable - retry when service recovers"),
			Entry("500 internal error - possible temporary server issue",
				&retry.HTTPError{StatusCode: 500},
				true,
				"500 errors may be temporary - retry may succeed"),
			Entry("502 bad gateway - upstream service issue",
				&retry.HTTPError{StatusCode: 502},
				true,
				"502 indicates upstream problem - retry when upstream recovers"),
			Entry("504 gateway timeout - upstream timeout",
				&retry.HTTPError{StatusCode: 504},
				true,
				"504 indicates timeout - retry may succeed with more time"),
			Entry("429 rate limit - temporary rate limiting",
				&retry.HTTPError{StatusCode: 429},
				true,
				"429 means rate limited - retry after backoff period"),
			Entry("408 request timeout - client timeout",
				&retry.HTTPError{StatusCode: 408},
				true,
				"408 indicates timeout - retry may succeed"),

			// PERMANENT ERRORS: Don't retry because issue won't resolve
			Entry("401 unauthorized - invalid credentials",
				&retry.HTTPError{StatusCode: 401},
				false,
				"401 means wrong credentials - retrying won't fix authentication"),
			Entry("403 forbidden - insufficient permissions",
				&retry.HTTPError{StatusCode: 403},
				false,
				"403 means insufficient permissions - retrying won't grant access"),
			Entry("404 not found - resource doesn't exist",
				&retry.HTTPError{StatusCode: 404},
				false,
				"404 means resource not found - retrying won't create resource"),
			Entry("400 bad request - invalid request format",
				&retry.HTTPError{StatusCode: 400},
				false,
				"400 means malformed request - retrying won't fix bad input"),
			Entry("422 unprocessable - validation failure",
				&retry.HTTPError{StatusCode: 422},
				false,
				"422 means validation failed - retrying won't change input"),
		)
	})

	// ==============================================
	// CATEGORY 2: Retry Behavior (BEHAVIOR & CORRECTNESS)
	// BR-NOT-052: System must retry with exponential backoff
	// ==============================================

	Context("Retry Behavior - BEHAVIOR & CORRECTNESS", func() {
		It("should allow retries up to max attempts for transient errors (BR-NOT-052: Retry attempts)", func() {
			// BEHAVIOR: Transient errors trigger retry attempts up to max
			transientError := errors.New("network timeout")

			for attempt := 0; attempt < 5; attempt++ {
				shouldRetry := policy.ShouldRetry(attempt, transientError)

				// BEHAVIOR VALIDATION: Each attempt is allowed
				Expect(shouldRetry).To(BeTrue(),
					"Should allow retry attempt %d for transient errors", attempt)
			}
		})

		It("should stop retrying after max attempts (BR-NOT-052: Max attempts enforcement)", func() {
			// BEHAVIOR: System stops retrying after exhausting max attempts
			// BUSINESS CONTEXT: Prevents infinite retry loops wasting resources

			transientError := errors.New("network timeout")

			// BEHAVIOR VALIDATION: After max attempts, no more retries
			shouldRetry := policy.ShouldRetry(5, transientError)
			Expect(shouldRetry).To(BeFalse(),
				"Should stop retrying after %d attempts to prevent resource exhaustion", 5)
		})

		It("should not retry permanent errors at all (BR-NOT-052: Permanent error handling)", func() {
			// BEHAVIOR: Permanent errors stop immediately without retries
			// BUSINESS CONTEXT: Don't waste time retrying errors that won't fix themselves

			authError := &retry.HTTPError{StatusCode: 401}

			// BEHAVIOR VALIDATION: Permanent errors don't trigger any retries
			for attempt := 0; attempt < 5; attempt++ {
				shouldRetry := policy.ShouldRetry(attempt, authError)
				Expect(shouldRetry).To(BeFalse(),
					"Should not retry permanent errors (attempt %d)", attempt)
			}

			// CORRECTNESS: Verify error classification is consistent
			Expect(policy.IsRetryable(authError)).To(BeFalse(),
				"Permanent errors must be consistently classified")
		})
	})

	// ==============================================
	// CATEGORY 3: Exponential Backoff (CORRECTNESS)
	// BR-NOT-052: Backoff must increase exponentially
	// ==============================================

	Context("Exponential Backoff - CORRECTNESS", func() {
		It("should calculate exponentially increasing backoff durations (BR-NOT-052: Exponential backoff)", func() {
			// CORRECTNESS: Backoff increases exponentially (base * multiplier^attempt)
			// BUSINESS CONTEXT: Gives failing service time to recover

			expectedBackoffs := []time.Duration{
				30 * time.Second,  // attempt 0: base
				60 * time.Second,  // attempt 1: base * 2
				120 * time.Second, // attempt 2: base * 4
				240 * time.Second, // attempt 3: base * 8
				480 * time.Second, // attempt 4: capped at max
			}

			for i, expected := range expectedBackoffs {
				actual := policy.NextBackoff(i)

				// CORRECTNESS VALIDATION: Each backoff matches exponential formula
				Expect(actual).To(Equal(expected),
					"Backoff for attempt %d should follow exponential pattern", i)
			}
		})

		It("should respect maximum backoff cap (BR-NOT-052: Backoff ceiling)", func() {
			// CORRECTNESS: Backoff never exceeds maximum (prevents excessive delays)
			// BUSINESS CONTEXT: Balances retry attempts with reasonable response times

			// Attempt 5 would be 960s (30 * 2^5), but capped at 480s
			backoff := policy.NextBackoff(5)

			// CORRECTNESS VALIDATION: Backoff capped at configured maximum
			Expect(backoff).To(Equal(480*time.Second),
				"Backoff should never exceed configured maximum")

			// Verify cap is consistently applied
			backoff6 := policy.NextBackoff(6)
			Expect(backoff6).To(Equal(480*time.Second),
				"Backoff cap should apply to all attempts beyond threshold")
		})

		It("should calculate correct backoff delays for retry attempts (BR-NOT-052: Backoff calculation)", func() {
			// CORRECTNESS: Retry policy calculates correct exponential backoff delays
			// BUSINESS CONTEXT: Ensures service has time to recover between retries
			// NOTE: Testing calculation logic directly (no sleep) - faster and equally valid

			policy := retry.NewPolicy(&retry.Config{
				MaxAttempts:       3,
				BaseBackoff:       50 * time.Millisecond,
				MaxBackoff:        500 * time.Millisecond,
				BackoffMultiplier: 2.0,
			})

			// Verify backoff calculation for each attempt
			backoff0 := policy.NextBackoff(0)
			backoff1 := policy.NextBackoff(1)
			backoff2 := policy.NextBackoff(2)

			// CORRECTNESS VALIDATION: Verify exponential delay pattern
			Expect(backoff0).To(Equal(50*time.Millisecond),
				"First backoff should be base delay (50ms)")
			Expect(backoff1).To(Equal(100*time.Millisecond),
				"Second backoff should be 2x base (100ms)")
			Expect(backoff2).To(Equal(200*time.Millisecond),
				"Third backoff should be 4x base (200ms)")

			// Verify exponential growth
			Expect(backoff1).To(BeNumerically(">", backoff0),
				"Each backoff must be longer than previous (exponential growth)")
			Expect(backoff2).To(BeNumerically(">", backoff1),
				"Each backoff must be longer than previous (exponential growth)")

			// Verify multiplier relationship
			Expect(backoff1).To(Equal(backoff0*2),
				"Backoff should grow by configured multiplier (2.0)")
			Expect(backoff2).To(Equal(backoff1*2),
				"Backoff should grow by configured multiplier (2.0)")
		})
	})
})

// ==============================================
// BR-NOT-061: Circuit Breaker Manager - BEHAVIOR & CORRECTNESS
// ==============================================
//
// NOTE: These unit tests validate the Manager wrapper behavior.
// The underlying github.com/sony/gobreaker library is battle-tested and covered by its own tests.
// Integration tests cover end-to-end circuit breaker behavior (performance_concurrent_test.go).
//
// Unit Test Scope:
// 1. Manager wrapper API (Execute, AllowRequest, State)
// 2. Per-channel isolation (multiple channels don't interfere)
// 3. Manager creation and configuration
//
// Integration Test Scope (see test/integration/notification/performance_concurrent_test.go):
// 1. Request blocking after threshold failures (cascade failure prevention)
// 2. Service recovery and circuit closure
// 3. Half-open state and probing behavior
var _ = Describe("BR-NOT-061: Circuit Breaker Manager", func() {
	var manager *circuitbreaker.Manager

	BeforeEach(func() {
		// Create manager with test-friendly settings
		manager = circuitbreaker.NewManager(gobreaker.Settings{
			MaxRequests: 2,
			Interval:    10 * time.Second,
			Timeout:     1 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= 3
			},
		})
	})

	Context("Manager Creation", func() {
		It("should create manager with shared settings", func() {
			// BEHAVIOR: Manager initializes successfully
			Expect(manager).ToNot(BeNil())
		})

		It("should allow initial requests (circuit starts closed)", func() {
			// BEHAVIOR: New channels start in closed state (normal operation)
			Expect(manager.AllowRequest("slack")).To(BeTrue(),
				"New circuit should allow requests initially")
			Expect(manager.State("slack")).To(Equal(gobreaker.StateClosed),
				"New circuit should start in Closed state")
		})
	})

	Context("Per-Channel Isolation", func() {
		It("should maintain independent circuit states per channel", func() {
			// BEHAVIOR: Each channel has its own circuit breaker state
			// BUSINESS VALUE: Slack failure doesn't affect console delivery

			// Fail slack channel 3 times to open circuit
			for i := 0; i < 3; i++ {
				_, err := manager.Execute("slack", func() (interface{}, error) {
					return nil, fmt.Errorf("slack failure %d", i+1)
				})
				Expect(err).To(HaveOccurred())
			}

			// CORRECTNESS: Slack circuit should be open
			Expect(manager.AllowRequest("slack")).To(BeFalse(),
				"Slack circuit should be open after 3 failures")

			// CORRECTNESS: Console circuit should still be closed
			Expect(manager.AllowRequest("console")).To(BeTrue(),
				"Console circuit should remain closed (independent from slack)")
			Expect(manager.State("console")).To(Equal(gobreaker.StateClosed),
				"Console circuit should be in Closed state")
		})

		It("should allow success on one channel while another is open", func() {
			// Open slack circuit
			for i := 0; i < 3; i++ {
				_, _ = manager.Execute("slack", func() (interface{}, error) {
					return nil, fmt.Errorf("slack failure")
				})
			}

			// BEHAVIOR: Console can still succeed
			_, err := manager.Execute("console", func() (interface{}, error) {
				return "success", nil
			})
			Expect(err).ToNot(HaveOccurred(),
				"Console delivery should succeed even when slack circuit is open")
		})
	})

	Context("Execute Method", func() {
		It("should return function result on success", func() {
			// BEHAVIOR: Execute returns function's return value
			result, err := manager.Execute("slack", func() (interface{}, error) {
				return "delivered", nil
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal("delivered"))
		})

		It("should return function error on failure", func() {
			// BEHAVIOR: Execute propagates function errors
			expectedErr := fmt.Errorf("delivery failed")
			_, err := manager.Execute("slack", func() (interface{}, error) {
				return nil, expectedErr
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("delivery failed"))
		})

		It("should return ErrOpenState when circuit is open", func() {
			// Open the circuit
			for i := 0; i < 3; i++ {
				_, _ = manager.Execute("slack", func() (interface{}, error) {
					return nil, fmt.Errorf("failure")
				})
			}

			// BEHAVIOR: Execute returns gobreaker.ErrOpenState
			_, err := manager.Execute("slack", func() (interface{}, error) {
				Fail("Function should not be called when circuit is open")
				return nil, nil
			})
			Expect(err).To(Equal(gobreaker.ErrOpenState),
				"Should return ErrOpenState when circuit is open")
		})
	})

	Context("State Method", func() {
		It("should return Closed state initially", func() {
			state := manager.State("new-channel")
			Expect(state).To(Equal(gobreaker.StateClosed))
		})

		It("should return Open state after threshold failures", func() {
			// Trigger circuit open
			for i := 0; i < 3; i++ {
				_, _ = manager.Execute("slack", func() (interface{}, error) {
					return nil, fmt.Errorf("failure")
				})
			}

			state := manager.State("slack")
			Expect(state).To(Equal(gobreaker.StateOpen))
		})
	})

	Context("AllowRequest Method", func() {
		It("should return true for closed circuit", func() {
			Expect(manager.AllowRequest("slack")).To(BeTrue())
		})

		It("should return false for open circuit", func() {
			// Open circuit
			for i := 0; i < 3; i++ {
				_, _ = manager.Execute("slack", func() (interface{}, error) {
					return nil, fmt.Errorf("failure")
				})
			}

			Expect(manager.AllowRequest("slack")).To(BeFalse())
		})
	})

	Context("Backward Compatibility Methods", func() {
		It("RecordSuccess should be no-op (backward compatibility)", func() {
			// BEHAVIOR: RecordSuccess exists for API compatibility
			// but is a no-op (gobreaker tracks via Execute)
			manager.RecordSuccess("slack")
			// No assertion - just verify it doesn't panic
		})

		It("RecordFailure should be no-op (backward compatibility)", func() {
			// BEHAVIOR: RecordFailure exists for API compatibility
			// but is a no-op (gobreaker tracks via Execute)
			manager.RecordFailure("slack")
			// No assertion - just verify it doesn't panic
		})
	})
})
