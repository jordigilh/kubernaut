package resilience

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// RetryConfig controls retry behavior for the RetryTransport.
type RetryConfig struct {
	MaxAttempts       int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	RetryableStatuses []int
	RetryCounter      *prometheus.CounterVec
	DependencyName    string
	// IdempotentOnly, when true, restricts retries to idempotent methods
	// (GET, HEAD, OPTIONS, PUT, DELETE). POST/PATCH are not retried regardless
	// of whether GetBody is set, unless this is false.
	IdempotentOnly bool
}

// RetryTransport wraps an http.RoundTripper with retry logic for transient failures.
// Non-replayable request bodies (Body != nil && GetBody == nil) are not retried.
// When IdempotentOnly is set, non-idempotent methods (POST, PATCH) are never retried.
type RetryTransport struct {
	next   http.RoundTripper
	config RetryConfig
}

// NewRetryTransport creates a RetryTransport wrapping next.
func NewRetryTransport(next http.RoundTripper, config *RetryConfig) *RetryTransport {
	if config.MaxAttempts < 1 {
		config.MaxAttempts = 1
	}
	return &RetryTransport{next: next, config: *config}
}

// RoundTrip executes the request with retry logic.
func (rt *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}

	canReplay := req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
	if rt.config.IdempotentOnly && !isIdempotent(req.Method) {
		canReplay = false
	}

	var lastErr error

	for attempt := 1; attempt <= rt.config.MaxAttempts; attempt++ {
		if err := rewindRequestBody(req, attempt); err != nil {
			return nil, err
		}

		resp, retryable, err := rt.attemptRoundTrip(req)
		if !retryable {
			return resp, err
		}
		lastErr = err

		if !canReplay {
			return nil, lastErr
		}

		if attempt < rt.config.MaxAttempts {
			if err2 := rt.waitBeforeRetry(req, attempt); err2 != nil {
				return nil, err2
			}
		}
	}

	return nil, lastErr
}

// rewindRequestBody re-obtains a fresh, replayable request body via
// req.GetBody before a retry attempt (a no-op on the first attempt or when
// the request has no body).
func rewindRequestBody(req *http.Request, attempt int) error {
	if attempt <= 1 || req.GetBody == nil {
		return nil
	}
	body, err := req.GetBody()
	if err != nil {
		return err
	}
	req.Body = body
	return nil
}

// attemptRoundTrip performs a single RoundTrip attempt. retryable is false
// when the caller should return (resp, err) immediately (success, or a
// non-retryable failure); when true, err holds the retryable failure reason
// (network error or retryable HTTP status) for the caller to track as
// lastErr.
func (rt *RetryTransport) attemptRoundTrip(req *http.Request) (resp *http.Response, retryable bool, err error) {
	resp, err = rt.next.RoundTrip(req)

	if err == nil && !rt.isRetryableStatus(resp.StatusCode) {
		return resp, false, nil
	}
	if err != nil {
		if !isRetryableError(err) {
			return nil, false, err
		}
		return nil, true, err
	}
	drainAndClose(resp.Body)
	return nil, true, fmt.Errorf("downstream returned retryable status %d", resp.StatusCode)
}

// waitBeforeRetry records the retry-attempt metric and sleeps for the
// backoff duration before the next attempt, respecting req's context.
func (rt *RetryTransport) waitBeforeRetry(req *http.Request, attempt int) error {
	if rt.config.RetryCounter != nil {
		rt.config.RetryCounter.WithLabelValues(
			rt.config.DependencyName,
			fmt.Sprintf("%d", attempt+1),
		).Inc()
	}

	delay := rt.calculateBackoff(attempt)
	return sleepWithContext(req.Context(), delay)
}

func (rt *RetryTransport) isRetryableStatus(code int) bool {
	for _, s := range rt.config.RetryableStatuses {
		if code == s {
			return true
		}
	}
	return false
}

func (rt *RetryTransport) calculateBackoff(attempt int) time.Duration {
	backoff := float64(rt.config.InitialBackoff) * math.Pow(2, float64(attempt-1))
	if backoff > float64(rt.config.MaxBackoff) {
		backoff = float64(rt.config.MaxBackoff)
	}
	jitter := backoff * 0.2 * (rand.Float64() - 0.5) // #nosec G404 -- jitter does not require cryptographic randomness
	return time.Duration(backoff + jitter)
}

func isRetryableError(err error) bool {
	return errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
