// Package delivery provides shared error types for notification delivery
package delivery

// RetryableError indicates an error that can be retried with backoff
// This distinguishes temporary failures (network, permissions, rate limits)
// from permanent failures (invalid URLs, authentication errors, TLS issues)
//
// BR-NOT-055: Retry logic with permanent error classification
type RetryableError struct {
	err error
}

// NewRetryableError creates a new retryable error
func NewRetryableError(err error) *RetryableError {
	return &RetryableError{err: err}
}

// Error implements the error interface
func (e *RetryableError) Error() string {
	return e.err.Error()
}

// Unwrap implements error unwrapping for errors.Is/As
func (e *RetryableError) Unwrap() error {
	return e.err
}

// IsRetryableError checks if an error is retryable
func IsRetryableError(err error) bool {
	_, ok := err.(*RetryableError)
	return ok
}


