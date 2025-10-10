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

package context

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// ContextUtils provides common context creation and timeout patterns
// used across multiple services. This consolidates the repeated
// context.WithTimeout(ctx, timeout) patterns found in 15+ files.

// TimeoutConfig holds timeout configuration for different operations
type TimeoutConfig struct {
	DefaultTimeout    time.Duration `yaml:"default_timeout" default:"30s"`
	ShortTimeout      time.Duration `yaml:"short_timeout" default:"5s"`
	MediumTimeout     time.Duration `yaml:"medium_timeout" default:"30s"`
	LongTimeout       time.Duration `yaml:"long_timeout" default:"5m"`
	DatabaseTimeout   time.Duration `yaml:"database_timeout" default:"10s"`
	NetworkTimeout    time.Duration `yaml:"network_timeout" default:"30s"`
	ProcessingTimeout time.Duration `yaml:"processing_timeout" default:"2m"`
}

// DefaultTimeoutConfig returns default timeout configuration
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		DefaultTimeout:    30 * time.Second,
		ShortTimeout:      5 * time.Second,
		MediumTimeout:     30 * time.Second,
		LongTimeout:       5 * time.Minute,
		DatabaseTimeout:   10 * time.Second,
		NetworkTimeout:    30 * time.Second,
		ProcessingTimeout: 2 * time.Minute,
	}
}

// WithTimeout creates a context with timeout and returns both context and cancel function
// This is the most commonly used pattern across the codebase.
func WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 30 * time.Second // Default fallback
	}
	return context.WithTimeout(parent, timeout)
}

// WithDefaultTimeout creates a context with the default timeout (30 seconds)
func WithDefaultTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return WithTimeout(parent, DefaultTimeoutConfig().DefaultTimeout)
}

// WithShortTimeout creates a context with short timeout (5 seconds) for quick operations
func WithShortTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return WithTimeout(parent, DefaultTimeoutConfig().ShortTimeout)
}

// WithLongTimeout creates a context with long timeout (5 minutes) for lengthy operations
func WithLongTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return WithTimeout(parent, DefaultTimeoutConfig().LongTimeout)
}

// WithDatabaseTimeout creates a context with database-appropriate timeout
func WithDatabaseTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return WithTimeout(parent, DefaultTimeoutConfig().DatabaseTimeout)
}

// WithNetworkTimeout creates a context with network-appropriate timeout
func WithNetworkTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return WithTimeout(parent, DefaultTimeoutConfig().NetworkTimeout)
}

// WithProcessingTimeout creates a context with processing-appropriate timeout
func WithProcessingTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return WithTimeout(parent, DefaultTimeoutConfig().ProcessingTimeout)
}

// ExecuteWithTimeout executes a function with a timeout context
// This consolidates the common pattern of creating context, executing, and handling timeout errors
func ExecuteWithTimeout[T any](
	parent context.Context,
	timeout time.Duration,
	operation func(ctx context.Context) (T, error),
	logger *logrus.Logger,
	operationName string,
) (T, error) {
	var zero T

	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	start := time.Now()

	if logger != nil {
		logger.WithFields(logrus.Fields{
			"operation": operationName,
			"timeout":   timeout,
		}).Debug("Starting operation with timeout")
	}

	result, err := operation(ctx)
	duration := time.Since(start)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			if logger != nil {
				logger.WithFields(logrus.Fields{
					"operation": operationName,
					"timeout":   timeout,
					"duration":  duration,
				}).Warn("Operation timed out")
			}
			return zero, fmt.Errorf("operation '%s' timed out after %v", operationName, timeout)
		}

		if logger != nil {
			logger.WithError(err).WithFields(logrus.Fields{
				"operation": operationName,
				"duration":  duration,
			}).Error("Operation failed")
		}
		return zero, err
	}

	if logger != nil {
		logger.WithFields(logrus.Fields{
			"operation": operationName,
			"duration":  duration,
		}).Debug("Operation completed successfully")
	}

	return result, nil
}

// ExecuteWithDefaultTimeout executes a function with default timeout
func ExecuteWithDefaultTimeout[T any](
	parent context.Context,
	operation func(ctx context.Context) (T, error),
	logger *logrus.Logger,
	operationName string,
) (T, error) {
	return ExecuteWithTimeout(parent, DefaultTimeoutConfig().DefaultTimeout, operation, logger, operationName)
}

// ExecuteWithRetryAndTimeout executes a function with retries and timeout
// This consolidates retry + timeout patterns found across multiple services
func ExecuteWithRetryAndTimeout[T any](
	parent context.Context,
	timeout time.Duration,
	maxRetries int,
	retryBackoff time.Duration,
	operation func(ctx context.Context) (T, error),
	isRetryableError func(error) bool,
	logger *logrus.Logger,
	operationName string,
) (T, error) {
	var zero T
	var lastErr error

	if maxRetries <= 0 {
		maxRetries = 3
	}
	if retryBackoff <= 0 {
		retryBackoff = time.Second
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err := ExecuteWithTimeout(parent, timeout, operation, logger,
			fmt.Sprintf("%s (attempt %d/%d)", operationName, attempt+1, maxRetries+1))

		if err == nil {
			return result, nil
		}

		lastErr = err

		// Don't retry on the last attempt
		if attempt == maxRetries {
			break
		}

		// Check if error is retryable
		if isRetryableError != nil && !isRetryableError(err) {
			if logger != nil {
				logger.WithError(err).WithField("operation", operationName).
					Debug("Error is not retryable, stopping retry attempts")
			}
			break
		}

		if logger != nil {
			logger.WithError(err).WithFields(logrus.Fields{
				"operation": operationName,
				"attempt":   attempt + 1,
				"backoff":   retryBackoff,
			}).Warn("Operation failed, retrying after backoff")
		}

		// Wait before retry
		select {
		case <-parent.Done():
			return zero, parent.Err()
		case <-time.After(retryBackoff):
			// Continue to next retry
		}
	}

	return zero, fmt.Errorf("operation '%s' failed after %d attempts: %w", operationName, maxRetries+1, lastErr)
}

// ContextValidator provides utilities for context validation
type ContextValidator struct {
	logger *logrus.Logger
}

// NewContextValidator creates a new context validator
func NewContextValidator(logger *logrus.Logger) *ContextValidator {
	return &ContextValidator{logger: logger}
}

// ValidateContext checks if context is valid and not cancelled
func (cv *ContextValidator) ValidateContext(ctx context.Context, operationName string) error {
	if ctx == nil {
		return fmt.Errorf("context is nil for operation: %s", operationName)
	}

	select {
	case <-ctx.Done():
		if cv.logger != nil {
			cv.logger.WithFields(logrus.Fields{
				"operation": operationName,
				"error":     ctx.Err(),
			}).Warn("Context is already cancelled")
		}
		return fmt.Errorf("context cancelled for operation '%s': %w", operationName, ctx.Err())
	default:
		return nil
	}
}

// GetRemainingTimeout returns the remaining timeout for a context
func (cv *ContextValidator) GetRemainingTimeout(ctx context.Context) (time.Duration, bool) {
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		return remaining, true
	}
	return 0, false
}
