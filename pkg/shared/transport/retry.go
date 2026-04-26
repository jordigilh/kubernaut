/*
Copyright 2026 Jordi Gil.

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

// Package transport provides HTTP transport middleware for inter-service
// communication resilience. The primary type is RetryTransport, an
// http.RoundTripper that retries failed requests on transient errors
// (connection reset, EOF, HTTP 502/503/504) with exponential backoff.
//
// Issue #853: Inter-service HTTP clients lack retry/circuit-breaker.
package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"syscall"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
)

// RetryConfig controls the retry behavior of RetryTransport.
type RetryConfig struct {
	// MaxAttempts is the total number of attempts (1 = no retries).
	MaxAttempts int

	// Backoff configures exponential backoff between retries.
	Backoff backoff.Config

	// Logger for retry events. If zero-value, retries are silent.
	Logger logr.Logger
}

// DefaultRetryConfig returns production-ready defaults:
//   - 3 attempts (1 initial + 2 retries)
//   - 100ms base, 2x multiplier, 1s max, 20% jitter
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		Backoff: backoff.Config{
			BasePeriod:    100 * time.Millisecond,
			MaxPeriod:     1 * time.Second,
			Multiplier:    2.0,
			JitterPercent: 20,
		},
	}
}

// RetryTransport wraps an http.RoundTripper and retries on transient failures.
// It implements http.RoundTripper.
//
// Retryable conditions:
//   - Connection errors: ECONNRESET, ECONNREFUSED, io.EOF, io.ErrUnexpectedEOF
//   - HTTP status codes: 502 (Bad Gateway), 503 (Service Unavailable), 504 (Gateway Timeout)
//
// Non-retryable conditions:
//   - HTTP 1xx–4xx responses (including 400, 404, etc.)
//   - HTTP 500, 501 (non-transient server errors)
//   - Requests with body but nil GetBody (body cannot be replayed safely)
//   - Context cancellation or deadline exceeded
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
	// Fast path: if context is already done, don't attempt.
	if err := req.Context().Err(); err != nil {
		return nil, err
	}

	// If request has a body but GetBody is nil, the body cannot be replayed.
	// Attempt once; if it fails with a retryable error, return immediately.
	canReplay := req.Body == nil || req.GetBody != nil

	var lastResp *http.Response
	var lastErr error

	logger := rt.config.Logger

	for attempt := 1; attempt <= rt.config.MaxAttempts; attempt++ {
		if attempt > 1 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = body
		}

		resp, err := rt.next.RoundTrip(req)

		if err == nil && !isRetryableStatus(resp.StatusCode) {
			return resp, nil
		}

		if err != nil {
			if !isRetryableError(err) {
				return nil, err
			}
			lastErr = err
			if logger.GetSink() != nil {
				logger.V(1).Info("retryable connection error",
					"attempt", attempt, "max", rt.config.MaxAttempts,
					"url", req.URL.String(), "error", err)
			}
		} else {
			drainAndClose(resp.Body)
			lastResp = resp
			lastErr = nil
			if logger.GetSink() != nil {
				logger.V(1).Info("retryable HTTP status",
					"attempt", attempt, "max", rt.config.MaxAttempts,
					"url", req.URL.String(), "status", resp.StatusCode)
			}
		}

		if !canReplay {
			if lastErr != nil {
				return nil, lastErr
			}
			return lastResp, nil
		}

		if attempt < rt.config.MaxAttempts {
			delay := rt.config.Backoff.Calculate(int32(attempt))
			if err := sleepWithContext(req.Context(), delay); err != nil {
				return nil, err
			}
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return lastResp, nil
}

func isRetryableStatus(code int) bool {
	return code == http.StatusBadGateway ||
		code == http.StatusServiceUnavailable ||
		code == http.StatusGatewayTimeout
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
