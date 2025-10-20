package retry

import (
	"math"
	"time"
)

// Policy defines retry behavior for failed notifications
type Policy struct {
	config *Config
}

// Config holds retry policy configuration
type Config struct {
	MaxAttempts       int
	BaseBackoff       time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
}

// HTTPError represents an HTTP error with status code
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "HTTP error"
}

// NewPolicy creates a new retry policy
func NewPolicy(config *Config) *Policy {
	return &Policy{
		config: config,
	}
}

// ShouldRetry determines if a delivery should be retried
// Satisfies BR-NOT-052: Automatic Retry
func (p *Policy) ShouldRetry(attemptCount int, err error) bool {
	// Don't retry if max attempts reached
	if attemptCount >= p.config.MaxAttempts {
		return false
	}

	// Don't retry permanent errors
	if !p.IsRetryable(err) {
		return false
	}

	return true
}

// IsRetryable classifies if an error is transient (retryable) or permanent
// Satisfies BR-NOT-052: Automatic Retry (error classification)
func (p *Policy) IsRetryable(err error) bool {
	// Check for HTTP errors
	if httpErr, ok := err.(*HTTPError); ok {
		return isRetryableHTTPStatus(httpErr.StatusCode)
	}

	// Network errors are typically transient
	// In production, would check for specific error types
	return true
}

// NextBackoff calculates the next backoff duration using exponential backoff
// Satisfies BR-NOT-052: Automatic Retry (exponential backoff)
func (p *Policy) NextBackoff(attemptCount int) time.Duration {
	// Exponential backoff: baseBackoff * (multiplier ^ attemptCount)
	backoff := time.Duration(float64(p.config.BaseBackoff) *
		math.Pow(p.config.BackoffMultiplier, float64(attemptCount)))

	// Cap at max backoff
	if backoff > p.config.MaxBackoff {
		return p.config.MaxBackoff
	}

	return backoff
}

// isRetryableHTTPStatus determines if an HTTP status code indicates a retryable error
func isRetryableHTTPStatus(statusCode int) bool {
	// Retryable status codes (transient errors)
	retryableCodes := map[int]bool{
		408: true, // Request Timeout
		429: true, // Too Many Requests (rate limit)
		500: true, // Internal Server Error
		502: true, // Bad Gateway
		503: true, // Service Unavailable
		504: true, // Gateway Timeout
	}

	return retryableCodes[statusCode]
}
