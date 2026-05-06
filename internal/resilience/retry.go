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
}

// RetryTransport wraps an http.RoundTripper with retry logic for transient failures.
// Non-replayable request bodies (Body != nil && GetBody == nil) are not retried.
type RetryTransport struct {
	next   http.RoundTripper
	config RetryConfig
}

// NewRetryTransport creates a RetryTransport wrapping next.
func NewRetryTransport(next http.RoundTripper, config RetryConfig) *RetryTransport {
	if config.MaxAttempts < 1 {
		config.MaxAttempts = 1
	}
	return &RetryTransport{next: next, config: config}
}

// RoundTrip executes the request with retry logic.
func (rt *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}

	canReplay := req.Body == nil || req.Body == http.NoBody || req.GetBody != nil

	var lastResp *http.Response
	var lastErr error

	for attempt := 1; attempt <= rt.config.MaxAttempts; attempt++ {
		if attempt > 1 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = body
		}

		resp, err := rt.next.RoundTrip(req)

		if err == nil && !rt.isRetryableStatus(resp.StatusCode) {
			return resp, nil
		}

		if err != nil {
			if !isRetryableError(err) {
				return nil, err
			}
			lastErr = err
		} else {
			drainAndClose(resp.Body)
			lastResp = resp
			lastErr = nil
		}

		if !canReplay {
			if lastErr != nil {
				return nil, lastErr
			}
			return lastResp, nil
		}

		if attempt < rt.config.MaxAttempts {
			if rt.config.RetryCounter != nil {
				rt.config.RetryCounter.WithLabelValues(
					rt.config.DependencyName,
					fmt.Sprintf("%d", attempt+1),
				).Inc()
			}

			delay := rt.calculateBackoff(attempt)
			if err2 := sleepWithContext(req.Context(), delay); err2 != nil {
				return nil, err2
			}
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return lastResp, nil
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
	jitter := backoff * 0.2 * (rand.Float64() - 0.5)
	return time.Duration(backoff + jitter)
}

func isRetryableError(err error) bool {
	return errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF)
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
