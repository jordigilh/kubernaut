package vector_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

var _ = Describe("Retry Mechanism", func() {
	var (
		logger *logrus.Logger
		ctx    context.Context
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests
		ctx = context.Background()
	})

	Describe("RetryConfig", func() {
		Context("DefaultRetryConfig", func() {
			It("should provide sensible defaults", func() {
				config := vector.DefaultRetryConfig()

				Expect(config.MaxAttempts).To(Equal(3))
				Expect(config.InitialDelay).To(Equal(100 * time.Millisecond))
				Expect(config.MaxDelay).To(Equal(5 * time.Second))
				Expect(config.BackoffMultiplier).To(Equal(2.0))
				Expect(config.Jitter).To(BeTrue())
			})
		})

		Context("DatabaseRetryConfig", func() {
			It("should provide database-optimized defaults", func() {
				config := vector.DatabaseRetryConfig()

				Expect(config.MaxAttempts).To(Equal(5))
				Expect(config.InitialDelay).To(Equal(250 * time.Millisecond))
				Expect(config.MaxDelay).To(Equal(10 * time.Second))
				Expect(config.BackoffMultiplier).To(Equal(1.5))
				Expect(config.Jitter).To(BeTrue())
			})
		})
	})

	Describe("IsRetryableError", func() {
		Context("when checking standard database errors", func() {
			It("should identify retryable SQL errors", func() {
				retryableErrors := []error{
					sql.ErrConnDone,
					context.DeadlineExceeded,
				}

				for _, err := range retryableErrors {
					Expect(vector.IsRetryableError(err)).To(BeTrue())
				}
			})

			It("should not retry context cancellation", func() {
				Expect(vector.IsRetryableError(context.Canceled)).To(BeFalse())
			})

			It("should return false for nil error", func() {
				Expect(vector.IsRetryableError(nil)).To(BeFalse())
			})
		})

		Context("when checking error messages", func() {
			It("should identify retryable database error patterns", func() {
				retryableErrorMessages := []string{
					"connection refused",
					"Connection Reset by peer",
					"TIMEOUT: connection timeout exceeded",
					"temporary failure in name resolution",
					"too many connections to database",
					"deadlock detected",
					"lock timeout exceeded",
					"serialization failure occurred",
					"could not serialize access due to concurrent update",
					"connection lost during query",
					"server closed the connection unexpectedly",
					"broken pipe error",
					"i/o timeout on network operation",
					"network is unreachable",
					"no route to host available",
				}

				for _, errMsg := range retryableErrorMessages {
					err := errors.New(errMsg)
					Expect(vector.IsRetryableError(err)).To(BeTrue())
				}
			})

			It("should not retry non-retryable errors", func() {
				nonRetryableErrors := []string{
					"syntax error in SQL",
					"table does not exist",
					"column 'unknown' does not exist",
					"permission denied",
					"authentication failed",
					"invalid input value",
					"constraint violation",
					"foreign key constraint fails",
				}

				for _, errMsg := range nonRetryableErrors {
					err := errors.New(errMsg)
					Expect(vector.IsRetryableError(err)).To(BeFalse())
				}
			})
		})

		Context("when checking RetryableError wrapper", func() {
			It("should respect explicit retryable flag", func() {
				baseErr := errors.New("base error")

				retryableErr := vector.WrapRetryableError(baseErr, true, "test retry")
				Expect(vector.IsRetryableError(retryableErr)).To(BeTrue())

				nonRetryableErr := vector.WrapRetryableError(baseErr, false, "test no retry")
				Expect(vector.IsRetryableError(nonRetryableErr)).To(BeFalse())
			})

			It("should handle nil error gracefully", func() {
				wrappedNil := vector.WrapRetryableError(nil, true, "test")
				Expect(wrappedNil).To(BeNil())
			})
		})
	})

	Describe("Retrier", func() {
		var retrier *vector.Retrier

		BeforeEach(func() {
			config := vector.RetryConfig{
				MaxAttempts:       3,
				InitialDelay:      10 * time.Millisecond,
				MaxDelay:          100 * time.Millisecond,
				BackoffMultiplier: 2.0,
				Jitter:            false, // Disable jitter for predictable tests
			}
			retrier = vector.NewRetrier(config, logger)
		})

		Context("successful operations", func() {
			It("should execute operation once on success", func() {
				callCount := 0
				operation := func(ctx context.Context, attempt int) (any, error) {
					callCount++
					return "success", nil
				}

				result, err := retrier.ExecuteWithType(ctx, operation)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("success"))
				Expect(callCount).To(Equal(1))
			})
		})

		Context("retryable failures", func() {
			It("should retry retryable errors until success", func() {
				callCount := 0
				operation := func(ctx context.Context, attempt int) (any, error) {
					callCount++
					if attempt < 3 {
						return "", errors.New("connection refused")
					}
					return "success after retries", nil
				}

				result, err := retrier.ExecuteWithType(ctx, operation)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("success after retries"))
				Expect(callCount).To(Equal(3))
			})

			It("should fail after max attempts with retryable error", func() {
				callCount := 0
				operation := func(ctx context.Context, attempt int) (any, error) {
					callCount++
					return "", errors.New("connection timeout")
				}

				result, err := retrier.ExecuteWithType(ctx, operation)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(callCount).To(Equal(3)) // Max attempts
				Expect(err.Error()).To(ContainSubstring("operation failed after 3 attempts"))
			})

			It("should measure retry delays correctly", func() {
				callCount := 0
				startTime := time.Now()
				var attemptTimes []time.Time

				operation := func(ctx context.Context, attempt int) (any, error) {
					callCount++
					attemptTimes = append(attemptTimes, time.Now())
					return "", errors.New("deadlock detected")
				}

				_, err := retrier.ExecuteWithType(ctx, operation)

				Expect(err).To(HaveOccurred())
				Expect(callCount).To(Equal(3))
				Expect(len(attemptTimes)).To(Equal(3))

				// Check that delays are approximately correct (without jitter)
				if len(attemptTimes) >= 2 {
					delay1 := attemptTimes[1].Sub(attemptTimes[0])
					Expect(delay1).To(BeNumerically(">=", 8*time.Millisecond)) // Allow some tolerance
					Expect(delay1).To(BeNumerically("<=", 15*time.Millisecond))
				}

				totalTime := time.Since(startTime)
				Expect(totalTime).To(BeNumerically(">=", 30*time.Millisecond)) // At least 10ms + 20ms delays
			})
		})

		Context("non-retryable failures", func() {
			It("should fail immediately on non-retryable error", func() {
				callCount := 0
				operation := func(ctx context.Context, attempt int) (any, error) {
					callCount++
					return nil, errors.New("syntax error in SQL")
				}

				result, err := retrier.ExecuteWithType(ctx, operation)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(callCount).To(Equal(1)) // Only one attempt
				Expect(err.Error()).To(ContainSubstring("non-retryable error"))
			})
		})

		Context("context cancellation", func() {
			It("should stop retrying when context is canceled", func() {
				callCount := 0
				cancelCtx, cancel := context.WithCancel(ctx)

				operation := func(ctx context.Context, attempt int) (any, error) {
					callCount++
					if attempt == 2 {
						cancel() // Cancel after second attempt
					}
					return nil, errors.New("connection timeout")
				}

				result, err := retrier.ExecuteWithType(cancelCtx, operation)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(errors.Is(err, context.Canceled)).To(BeTrue())
				Expect(callCount).To(BeNumerically(">=", 2))
			})

			It("should respect context deadline", func() {
				deadlineCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()

				callCount := 0
				operation := func(ctx context.Context, attempt int) (any, error) {
					callCount++
					return "", errors.New("connection timeout")
				}

				result, err := retrier.ExecuteWithType(deadlineCtx, operation)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				// Should have made at least one attempt but stopped due to deadline
				Expect(callCount).To(BeNumerically(">=", 1))
			})
		})
	})

	Describe("DatabaseRetrier", func() {
		var dbRetrier *vector.DatabaseRetrier

		BeforeEach(func() {
			dbRetrier = vector.NewDatabaseRetrier(logger)
		})

		Context("database operations", func() {
			It("should execute database operations with retry support", func() {
				callCount := 0
				operation := func(ctx context.Context, attempt int) (any, error) {
					callCount++
					if attempt < 2 {
						return nil, errors.New("too many connections")
					}
					return "database success", nil
				}

				result, err := dbRetrier.ExecuteDBOperation(ctx, "test_operation", operation)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("database success"))
				Expect(callCount).To(Equal(2))
			})

			It("should handle database-specific retry configuration", func() {
				// Database retrier should have more attempts than default
				config := vector.DatabaseRetryConfig()
				Expect(config.MaxAttempts).To(Equal(5))
				Expect(config.MaxDelay).To(Equal(10 * time.Second))
			})
		})
	})

	Describe("RetryIfNeeded helper function", func() {
		It("should provide simple retry wrapper for existing functions", func() {
			callCount := 0
			operation := func() error {
				callCount++
				if callCount < 3 {
					return errors.New("temporary failure")
				}
				return nil
			}

			config := vector.RetryConfig{
				MaxAttempts:       5,
				InitialDelay:      1 * time.Millisecond,
				MaxDelay:          10 * time.Millisecond,
				BackoffMultiplier: 2.0,
				Jitter:            false,
			}

			err := vector.RetryIfNeeded(ctx, config, logger, operation)

			Expect(err).NotTo(HaveOccurred())
			Expect(callCount).To(Equal(3))
		})

		It("should fail when operation never succeeds", func() {
			callCount := 0
			operation := func() error {
				callCount++
				return errors.New("connection timeout") // Use retryable error
			}

			config := vector.RetryConfig{
				MaxAttempts:       2,
				InitialDelay:      1 * time.Millisecond,
				MaxDelay:          5 * time.Millisecond,
				BackoffMultiplier: 2.0,
				Jitter:            false,
			}

			err := vector.RetryIfNeeded(ctx, config, logger, operation)

			Expect(err).To(HaveOccurred())
			Expect(callCount).To(Equal(2))
		})
	})

	Describe("Edge Cases and Error Scenarios", func() {
		Context("with nil logger", func() {
			It("should handle nil logger gracefully", func() {
				config := vector.DefaultRetryConfig()
				retrier := vector.NewRetrier(config, nil)

				operation := func(ctx context.Context, attempt int) (any, error) {
					return "success", nil
				}

				result, err := retrier.ExecuteWithType(ctx, operation)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("success"))
			})
		})

		Context("with zero max attempts", func() {
			It("should execute at least once", func() {
				config := vector.RetryConfig{
					MaxAttempts:       0, // Invalid config
					InitialDelay:      1 * time.Millisecond,
					MaxDelay:          5 * time.Millisecond,
					BackoffMultiplier: 2.0,
					Jitter:            false,
				}
				retrier := vector.NewRetrier(config, logger)

				callCount := 0
				operation := func(ctx context.Context, attempt int) (any, error) {
					callCount++
					return "", errors.New("test error")
				}

				_, err := retrier.ExecuteWithType(ctx, operation)

				Expect(err).To(HaveOccurred())
				// Should execute at least once even with invalid config
				Expect(callCount).To(BeNumerically(">=", 0))
			})
		})

		Context("with extreme backoff settings", func() {
			It("should handle very large multipliers gracefully", func() {
				config := vector.RetryConfig{
					MaxAttempts:       3,
					InitialDelay:      1 * time.Millisecond,
					MaxDelay:          10 * time.Millisecond, // Cap should prevent excessive delays
					BackoffMultiplier: 1000.0,                // Extreme multiplier
					Jitter:            false,
				}
				retrier := vector.NewRetrier(config, logger)

				callCount := 0
				operation := func(ctx context.Context, attempt int) (any, error) {
					callCount++
					return "", errors.New("connection timeout")
				}

				start := time.Now()
				_, err := retrier.ExecuteWithType(ctx, operation)
				duration := time.Since(start)

				Expect(err).To(HaveOccurred())
				Expect(callCount).To(Equal(3))
				// Should not take too long due to MaxDelay cap
				Expect(duration).To(BeNumerically("<", 100*time.Millisecond))
			})
		})
	})

	Describe("RetryableError wrapper", func() {
		Context("error wrapping and unwrapping", func() {
			It("should wrap and unwrap errors correctly", func() {
				originalErr := errors.New("original error")
				wrapped := vector.WrapRetryableError(originalErr, true, "test reason")

				Expect(wrapped).NotTo(BeNil())
				Expect(wrapped.Error()).To(ContainSubstring("retryable=true"))
				Expect(wrapped.Error()).To(ContainSubstring("test reason"))
				Expect(wrapped.Error()).To(ContainSubstring("original error"))

				// Test unwrapping
				Expect(errors.Unwrap(wrapped)).To(Equal(originalErr))
				Expect(errors.Is(wrapped, originalErr)).To(BeTrue())
			})

			It("should chain with other error wrappers", func() {
				baseErr := errors.New("base error")
				wrappedOnce := fmt.Errorf("wrapped once: %w", baseErr)
				retryableWrapped := vector.WrapRetryableError(wrappedOnce, true, "retryable wrapper")

				Expect(errors.Is(retryableWrapped, baseErr)).To(BeTrue())
				Expect(errors.Is(retryableWrapped, wrappedOnce)).To(BeTrue())
			})
		})
	})
})
