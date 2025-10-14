package notification_test

import (
	"errors"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/notification/retry"
)

func TestRetry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Retry Policy Suite")
}

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

	// ⭐ TABLE-DRIVEN: Retry decision based on error type (Option B: 12 error classification tests)
	DescribeTable("should determine if error is retryable",
		func(err error, expectedRetryable bool, description string) {
			retryable := policy.IsRetryable(err)
			Expect(retryable).To(Equal(expectedRetryable), description)
		},
		// Transient errors (retryable)
		Entry("transient network error", errors.New("network timeout"), true, "network timeouts are retryable"),
		Entry("503 service unavailable", &retry.HTTPError{StatusCode: 503}, true, "503 errors are retryable"),
		Entry("500 internal error", &retry.HTTPError{StatusCode: 500}, true, "500 errors are retryable"),
		Entry("502 bad gateway", &retry.HTTPError{StatusCode: 502}, true, "502 errors are retryable"),
		Entry("504 gateway timeout", &retry.HTTPError{StatusCode: 504}, true, "504 errors are retryable"),
		Entry("429 rate limit", &retry.HTTPError{StatusCode: 429}, true, "rate limits are retryable"),
		Entry("408 request timeout", &retry.HTTPError{StatusCode: 408}, true, "408 errors are retryable"),

		// Permanent errors (not retryable)
		Entry("401 unauthorized", &retry.HTTPError{StatusCode: 401}, false, "401 errors are permanent"),
		Entry("403 forbidden", &retry.HTTPError{StatusCode: 403}, false, "403 errors are permanent"),
		Entry("404 not found", &retry.HTTPError{StatusCode: 404}, false, "404 errors are permanent"),
		Entry("400 bad request", &retry.HTTPError{StatusCode: 400}, false, "400 errors are permanent"),
		Entry("422 unprocessable", &retry.HTTPError{StatusCode: 422}, false, "422 errors are permanent"),
	)

	Context("max attempts enforcement", func() {
		It("should allow retries up to max attempts", func() {
			for attempt := 0; attempt < 5; attempt++ {
				shouldRetry := policy.ShouldRetry(attempt, errors.New("transient"))
				Expect(shouldRetry).To(BeTrue(), "attempt %d should be allowed", attempt)
			}
		})

		It("should stop retrying after max attempts", func() {
			shouldRetry := policy.ShouldRetry(5, errors.New("transient"))
			Expect(shouldRetry).To(BeFalse(), "should stop after 5 attempts")
		})

		It("should not retry permanent errors", func() {
			err := &retry.HTTPError{StatusCode: 401}
			shouldRetry := policy.ShouldRetry(0, err)
			Expect(shouldRetry).To(BeFalse(), "permanent errors should not retry")
		})
	})

	Context("backoff calculation", func() {
		// Note: CalculateBackoff function already tested in Day 2
		// This verifies integration with Policy
		It("should calculate correct backoff durations", func() {
			backoffs := []time.Duration{
				30 * time.Second,  // attempt 0
				60 * time.Second,  // attempt 1
				120 * time.Second, // attempt 2
				240 * time.Second, // attempt 3
				480 * time.Second, // attempt 4 (capped)
			}

			for i, expected := range backoffs {
				actual := policy.NextBackoff(i)
				Expect(actual).To(Equal(expected), "backoff for attempt %d", i)
			}
		})

		It("should respect max backoff cap", func() {
			// Attempt 5 would be 960s, but capped at 480s
			backoff := policy.NextBackoff(5)
			Expect(backoff).To(Equal(480 * time.Second))
		})
	})
})

var _ = Describe("BR-NOT-055: Circuit Breaker", func() {
	var breaker *retry.CircuitBreaker

	BeforeEach(func() {
		breaker = retry.NewCircuitBreaker(&retry.CircuitBreakerConfig{
			FailureThreshold: 5,
			SuccessThreshold: 2,
			Timeout:          60 * time.Second,
		})
	})

	It("should open circuit after consecutive failures", func() {
		// Record 4 failures (circuit still closed)
		for i := 0; i < 4; i++ {
			breaker.RecordFailure("slack")
			Expect(breaker.State("slack")).To(Equal(retry.CircuitClosed))
		}

		// 5th failure opens the circuit
		breaker.RecordFailure("slack")
		Expect(breaker.State("slack")).To(Equal(retry.CircuitOpen))
	})

	It("should allow attempts when circuit is half-open", func() {
		// Open the circuit
		for i := 0; i < 5; i++ {
			breaker.RecordFailure("slack")
		}
		Expect(breaker.State("slack")).To(Equal(retry.CircuitOpen))

		// Try to reset (moves to half-open)
		breaker.TryReset("slack")

		Expect(breaker.State("slack")).To(Equal(retry.CircuitHalfOpen))
		Expect(breaker.AllowRequest("slack")).To(BeTrue())
	})

	It("should close circuit after success threshold", func() {
		// Open circuit
		for i := 0; i < 5; i++ {
			breaker.RecordFailure("slack")
		}
		breaker.TryReset("slack")
		Expect(breaker.State("slack")).To(Equal(retry.CircuitHalfOpen))

		// Record 2 successes (threshold)
		breaker.RecordSuccess("slack")
		breaker.RecordSuccess("slack")

		Expect(breaker.State("slack")).To(Equal(retry.CircuitClosed))
	})

	It("should maintain separate states per channel", func() {
		// Fail Slack
		for i := 0; i < 5; i++ {
			breaker.RecordFailure("slack")
		}

		// Console remains closed
		Expect(breaker.State("slack")).To(Equal(retry.CircuitOpen))
		Expect(breaker.State("console")).To(Equal(retry.CircuitClosed))
	})

	It("should reset failure count on success in closed state", func() {
		// Record some failures (not enough to open)
		breaker.RecordFailure("slack")
		breaker.RecordFailure("slack")

		// Success should reset count
		breaker.RecordSuccess("slack")

		// Should need 5 failures again to open
		for i := 0; i < 4; i++ {
			breaker.RecordFailure("slack")
			Expect(breaker.State("slack")).To(Equal(retry.CircuitClosed))
		}
	})

	It("should reject requests when circuit is open", func() {
		// Open the circuit
		for i := 0; i < 5; i++ {
			breaker.RecordFailure("slack")
		}

		Expect(breaker.AllowRequest("slack")).To(BeFalse())
	})
})
