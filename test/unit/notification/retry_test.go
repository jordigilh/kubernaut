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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/notification/retry"
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

		It("should apply exponential backoff in retry loop (BR-NOT-052: Backoff application)", func() {
			// CORRECTNESS: Retry loop actually applies calculated backoff delays
			// BUSINESS CONTEXT: Ensures service has time to recover between retries

			// Use faster backoff for test speed
			fastPolicy := retry.NewPolicy(&retry.Config{
				MaxAttempts:       3,
				BaseBackoff:       50 * time.Millisecond,
				MaxBackoff:        500 * time.Millisecond,
				BackoffMultiplier: 2.0,
			})

			attemptTimes := []time.Time{}
			transientError := errors.New("network timeout")

			// Simulate retry loop (as controller would do)
			for attempt := 0; attempt < 3; attempt++ {
				attemptTimes = append(attemptTimes, time.Now())

				if !fastPolicy.ShouldRetry(attempt, transientError) {
					break
				}

				// Apply backoff between retries
				backoff := fastPolicy.NextBackoff(attempt)
				time.Sleep(backoff)
			}

			// CORRECTNESS VALIDATION: Verify exponential delay pattern
			Expect(attemptTimes).To(HaveLen(3), "Should execute 3 retry attempts")

			delay1 := attemptTimes[1].Sub(attemptTimes[0])
			delay2 := attemptTimes[2].Sub(attemptTimes[1])

			// Validate exponential growth (with tolerance for timing variations)
			Expect(delay1).To(BeNumerically("~", 50*time.Millisecond, 30*time.Millisecond),
				"First backoff should be ~50ms (base delay)")
			Expect(delay2).To(BeNumerically("~", 100*time.Millisecond, 30*time.Millisecond),
				"Second backoff should be ~100ms (2x base)")
			Expect(delay2).To(BeNumerically(">", delay1),
				"Second delay must be longer than first (exponential growth)")
		})
	})
})

// ==============================================
// BR-NOT-061: Circuit Breaker - BEHAVIOR & CORRECTNESS
// ==============================================

var _ = Describe("BR-NOT-061: Circuit Breaker", func() {
	var breaker *retry.CircuitBreaker

	BeforeEach(func() {
		breaker = retry.NewCircuitBreaker(&retry.CircuitBreakerConfig{
			FailureThreshold: 5,
			SuccessThreshold: 2,
			Timeout:          60 * time.Second,
		})
	})

	// ==============================================
	// CATEGORY 1: Request Blocking (BEHAVIOR)
	// BR-NOT-061: Circuit breaker prevents cascade failures
	// ==============================================

	Context("Request Blocking - BEHAVIOR", func() {
		It("should block requests after failure threshold (BR-NOT-061: Cascade failure prevention)", func() {
			// BEHAVIOR: Circuit breaker blocks requests to protect failing service
			// BUSINESS CONTEXT: Prevents overwhelming unhealthy service with more requests

			// Record failures up to threshold - 1
			for i := 0; i < 4; i++ {
				breaker.RecordFailure("slack")

				// BEHAVIOR VALIDATION: Requests still allowed before threshold
				Expect(breaker.AllowRequest("slack")).To(BeTrue(),
					"Should allow requests before reaching threshold (failure %d)", i)
			}

			// 5th failure triggers circuit breaker
			breaker.RecordFailure("slack")

			// BEHAVIOR VALIDATION: Circuit breaker now blocks requests
			Expect(breaker.AllowRequest("slack")).To(BeFalse(),
				"Should block requests after reaching failure threshold to prevent cascade failures")
		})

		It("should allow limited requests in recovery mode (BR-NOT-061: Service recovery)", func() {
			// BEHAVIOR: Circuit breaker allows probes to test if service recovered
			// BUSINESS CONTEXT: Enables service recovery without overwhelming it

			// Trigger circuit breaker (open state)
			for i := 0; i < 5; i++ {
				breaker.RecordFailure("slack")
			}
			Expect(breaker.AllowRequest("slack")).To(BeFalse(),
				"Circuit should block requests when open")

			// Transition to half-open (recovery mode)
			breaker.TryReset("slack")

			// BEHAVIOR VALIDATION: Half-open allows probe requests
			Expect(breaker.AllowRequest("slack")).To(BeTrue(),
				"Should allow probe requests in half-open state to test service recovery")
		})

		It("should resume normal operation after successful recovery (BR-NOT-061: Circuit closure)", func() {
			// BEHAVIOR: Circuit breaker closes after service proves it's healthy
			// BUSINESS CONTEXT: Returns to normal operation once service recovers

			// Open circuit
			for i := 0; i < 5; i++ {
				breaker.RecordFailure("slack")
			}
			breaker.TryReset("slack") // Transition to half-open

			// Record successful probe requests
			breaker.RecordSuccess("slack")
			breaker.RecordSuccess("slack")

			// BEHAVIOR VALIDATION: Circuit allows all requests after recovery
			Expect(breaker.AllowRequest("slack")).To(BeTrue(),
				"Should allow all requests after successful recovery (circuit closed)")

			// CORRECTNESS: Verify circuit stays closed for subsequent requests
			for i := 0; i < 10; i++ {
				Expect(breaker.AllowRequest("slack")).To(BeTrue(),
					"Circuit should remain closed for normal operation (request %d)", i)
			}
		})
	})

	// ==============================================
	// CATEGORY 2: Channel Isolation (CORRECTNESS)
	// BR-NOT-061: Circuit breaker per channel
	// ==============================================

	Context("Channel Isolation - CORRECTNESS", func() {
		It("should maintain independent circuit states per channel (BR-NOT-061: Channel isolation)", func() {
			// CORRECTNESS: Each channel has independent circuit breaker
			// BUSINESS CONTEXT: Failure in Slack doesn't affect Console delivery

			// Fail Slack channel repeatedly
			for i := 0; i < 5; i++ {
				breaker.RecordFailure("slack")
			}

			// CORRECTNESS VALIDATION: Slack blocked, Console still works
			Expect(breaker.AllowRequest("slack")).To(BeFalse(),
				"Slack circuit should be open after failures")
			Expect(breaker.AllowRequest("console")).To(BeTrue(),
				"Console circuit should remain closed (independent from Slack)")
			Expect(breaker.AllowRequest("email")).To(BeTrue(),
				"Email circuit should remain closed (independent from Slack)")
		})
	})

	// ==============================================
	// CATEGORY 3: Failure Recovery (CORRECTNESS)
	// BR-NOT-061: Circuit breaker recovery behavior
	// ==============================================

	Context("Failure Recovery - CORRECTNESS", func() {
		It("should reset failure count after success in normal operation (BR-NOT-061: Failure count reset)", func() {
			// CORRECTNESS: Success in closed state resets failure count
			// BUSINESS CONTEXT: Occasional failures don't trigger circuit breaker

			// Record some failures (not enough to open)
			breaker.RecordFailure("slack")
			breaker.RecordFailure("slack")

			// Success should reset failure count
			breaker.RecordSuccess("slack")

			// CORRECTNESS VALIDATION: Should need full threshold again
			for i := 0; i < 4; i++ {
				breaker.RecordFailure("slack")
				Expect(breaker.AllowRequest("slack")).To(BeTrue(),
					"Should still allow requests (failure count was reset, now at %d)", i+1)
			}

			// 5th failure (after reset) opens circuit
			breaker.RecordFailure("slack")
			Expect(breaker.AllowRequest("slack")).To(BeFalse(),
				"Should open circuit after reaching threshold from reset point")
		})

		It("should reopen circuit on failure during recovery (BR-NOT-061: Recovery failure handling)", func() {
			// CORRECTNESS: Failure during half-open immediately reopens circuit
			// BUSINESS CONTEXT: Service not fully recovered - give it more time

			// Open circuit
			for i := 0; i < 5; i++ {
				breaker.RecordFailure("slack")
			}
			breaker.TryReset("slack") // Half-open

			Expect(breaker.AllowRequest("slack")).To(BeTrue(),
				"Should allow probe in half-open state")

			// Probe fails - service not recovered
			breaker.RecordFailure("slack")

			// CORRECTNESS VALIDATION: Circuit reopens immediately
			Expect(breaker.AllowRequest("slack")).To(BeFalse(),
				"Should reopen circuit after failed recovery probe")
		})

		It("should require all successes in recovery before closing (BR-NOT-061: Success threshold)", func() {
			// CORRECTNESS: Circuit requires multiple successful probes before closing
			// BUSINESS CONTEXT: Ensures service is stable, not just temporarily responding

			// Open circuit and enter half-open
			for i := 0; i < 5; i++ {
				breaker.RecordFailure("slack")
			}
			breaker.TryReset("slack")

			// One success not enough (threshold is 2)
			breaker.RecordSuccess("slack")
			Expect(breaker.AllowRequest("slack")).To(BeTrue(),
				"Should still be in half-open after 1 success (threshold is 2)")

			// Second success closes circuit
			breaker.RecordSuccess("slack")

			// CORRECTNESS VALIDATION: Circuit fully closed after meeting threshold
			Expect(breaker.AllowRequest("slack")).To(BeTrue(),
				"Should close circuit after meeting success threshold")

			// Verify circuit stays closed
			for i := 0; i < 5; i++ {
				Expect(breaker.AllowRequest("slack")).To(BeTrue(),
					"Circuit should remain closed after successful recovery")
			}
		})
	})
})
