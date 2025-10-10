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

package vector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// RetryConfig holds configuration for retry behavior
type RetryConfig struct {
	MaxAttempts       int           `yaml:"max_attempts"`       // Maximum number of retry attempts
	InitialDelay      time.Duration `yaml:"initial_delay"`      // Initial delay between retries
	MaxDelay          time.Duration `yaml:"max_delay"`          // Maximum delay between retries
	BackoffMultiplier float64       `yaml:"backoff_multiplier"` // Multiplier for exponential backoff
	Jitter            bool          `yaml:"jitter"`             // Whether to add jitter to delays
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          5 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            true,
	}
}

// DatabaseRetryConfig returns retry configuration optimized for database operations
func DatabaseRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:       5,
		InitialDelay:      250 * time.Millisecond,
		MaxDelay:          10 * time.Second,
		BackoffMultiplier: 1.5,
		Jitter:            true,
	}
}

// RetryableError represents an error that can be retried
type RetryableError struct {
	Err       error
	Retryable bool
	Reason    string
}

func (r *RetryableError) Error() string {
	return fmt.Sprintf("retryable=%v reason=%s: %v", r.Retryable, r.Reason, r.Err)
}

func (r *RetryableError) Unwrap() error {
	return r.Err
}

// IsRetryableError checks if an error should be retried
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for explicit retryable error
	var retryableErr *RetryableError
	if errors.As(err, &retryableErr) {
		return retryableErr.Retryable
	}

	// Check for common database errors that are retryable
	switch {
	case errors.Is(err, sql.ErrConnDone):
		return true
	case errors.Is(err, context.DeadlineExceeded):
		return true
	case errors.Is(err, context.Canceled):
		return false // Don't retry canceled contexts
	default:
		// Check error message for common retryable database errors
		errMsg := err.Error()
		return isRetryableDatabaseError(errMsg)
	}
}

// isRetryableDatabaseError checks if a database error message indicates a retryable condition
func isRetryableDatabaseError(errMsg string) bool {
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"connection timeout",
		"temporary failure",
		"too many connections",
		"deadlock detected",
		"lock timeout",
		"serialization failure",
		"could not serialize access",
		"connection lost",
		"server closed the connection",
		"broken pipe",
		"i/o timeout",
		"network is unreachable",
		"no route to host",
	}

	for _, pattern := range retryablePatterns {
		if containsIgnoreCase(errMsg, pattern) {
			return true
		}
	}

	return false
}

// containsIgnoreCase checks if a string contains a substring (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				containsIgnoreCaseHelper(s, substr)))
}

func containsIgnoreCaseHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c1 := s[i+j]
			c2 := substr[j]
			// Simple case conversion (works for ASCII)
			if c1 >= 'A' && c1 <= 'Z' {
				c1 += 32
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 += 32
			}
			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// RetryableOperation represents an operation that can be retried
type RetryableOperation[T any] func(ctx context.Context, attempt int) (T, error)

// Retrier handles retry logic for operations
type Retrier struct {
	config RetryConfig
	logger *logrus.Logger
}

// NewRetrier creates a new retrier with the given configuration
func NewRetrier(config RetryConfig, logger *logrus.Logger) *Retrier {
	if logger == nil {
		logger = logrus.New()
	}

	return &Retrier{
		config: config,
		logger: logger,
	}
}

// Execute runs an operation with retry logic
func (r *Retrier) Execute(ctx context.Context, operation RetryableOperation[any]) (any, error) {
	return r.ExecuteWithType(ctx, operation)
}

// ExecuteWithType runs a typed operation with retry logic
func (r *Retrier) ExecuteWithType(ctx context.Context, operation RetryableOperation[any]) (any, error) {
	var result any
	var lastErr error

	maxAttempts := r.config.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1 // Ensure at least one attempt
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Check context before each attempt
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Execute the operation
		result, err := operation(ctx, attempt)
		if err == nil {
			// Success!
			if attempt > 1 {
				r.logger.WithFields(logrus.Fields{
					"attempt":        attempt,
					"total_attempts": maxAttempts,
				}).Info("Operation succeeded after retry")
			}
			return result, nil
		}

		lastErr = err

		// Check if we should retry
		if !IsRetryableError(err) {
			r.logger.WithFields(logrus.Fields{
				"attempt": attempt,
				"error":   err.Error(),
			}).Debug("Error is not retryable, giving up")
			return result, fmt.Errorf("operation failed with non-retryable error: %w", err)
		}

		// Don't wait after the last attempt
		if attempt >= maxAttempts {
			break
		}

		// Calculate delay
		delay := r.calculateDelay(attempt)

		r.logger.WithFields(logrus.Fields{
			"attempt":        attempt,
			"total_attempts": maxAttempts,
			"delay_ms":       delay.Milliseconds(),
			"error":          err.Error(),
		}).Warn("Operation failed, retrying after delay")

		// Wait with context cancellation support
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	r.logger.WithFields(logrus.Fields{
		"total_attempts": maxAttempts,
		"final_error":    lastErr.Error(),
	}).Error("Operation failed after all retry attempts")

	return result, fmt.Errorf("operation failed after %d attempts: %w", maxAttempts, lastErr)
}

// calculateDelay calculates the delay for the given attempt using exponential backoff
func (r *Retrier) calculateDelay(attempt int) time.Duration {
	// Start with initial delay
	delay := r.config.InitialDelay

	// Apply exponential backoff
	for i := 1; i < attempt; i++ {
		delay = time.Duration(float64(delay) * r.config.BackoffMultiplier)
	}

	// Cap at maximum delay
	if delay > r.config.MaxDelay {
		delay = r.config.MaxDelay
	}

	// Add jitter if enabled
	if r.config.Jitter {
		jitterAmount := float64(delay) * 0.1 // 10% jitter
		jitter := time.Duration(float64(time.Now().UnixNano()%int64(jitterAmount*2)) - jitterAmount)
		delay += jitter

		// Ensure delay is never negative
		if delay < 0 {
			delay = r.config.InitialDelay
		}
	}

	return delay
}

// DatabaseRetrier is a specialized retrier for database operations
type DatabaseRetrier struct {
	*Retrier
}

// NewDatabaseRetrier creates a retrier optimized for database operations
func NewDatabaseRetrier(logger *logrus.Logger) *DatabaseRetrier {
	config := DatabaseRetryConfig()
	return &DatabaseRetrier{
		Retrier: NewRetrier(config, logger),
	}
}

// ExecuteDBOperation executes a database operation with appropriate retry logic
func (dr *DatabaseRetrier) ExecuteDBOperation(ctx context.Context, operationName string, operation RetryableOperation[any]) (any, error) {
	dr.logger.WithField("operation", operationName).Debug("Starting database operation with retry support")

	result, err := dr.Execute(ctx, operation)

	if err != nil {
		dr.logger.WithFields(logrus.Fields{
			"operation": operationName,
			"error":     err.Error(),
		}).Error("Database operation failed after all retries")
	} else {
		dr.logger.WithField("operation", operationName).Debug("Database operation completed successfully")
	}

	return result, err
}

// WrapRetryableError wraps an error to make it explicitly retryable or non-retryable
func WrapRetryableError(err error, retryable bool, reason string) error {
	if err == nil {
		return nil
	}

	return &RetryableError{
		Err:       err,
		Retryable: retryable,
		Reason:    reason,
	}
}

// RetryIfNeeded provides a simple way to add retry logic to existing functions
func RetryIfNeeded(ctx context.Context, config RetryConfig, logger *logrus.Logger, operation func() error) error {
	retrier := NewRetrier(config, logger)

	_, err := retrier.Execute(ctx, func(ctx context.Context, attempt int) (any, error) {
		return nil, operation()
	})

	return err
}
