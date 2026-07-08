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

package llm

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
)

var defaultRetryBackoff = backoff.Config{
	BasePeriod:    500 * time.Millisecond,
	MaxPeriod:     5 * time.Second,
	Multiplier:    2.0,
	JitterPercent: 20,
}

// ResolveRetryBackoff returns params.RetryBackoff if set, otherwise the
// package default (500ms base, 5s max, 2x multiplier, 20% jitter). Shared
// by ChatWithParams and the streaming retry path (chatOrStream, #1612) so
// both use one source of truth for the default policy.
func ResolveRetryBackoff(params RuntimeParams) backoff.Config {
	if params.RetryBackoff != nil {
		return *params.RetryBackoff
	}
	return defaultRetryBackoff
}

// ResolveMaxAttempts returns the total number of attempts (1 initial +
// MaxRetries), floored at 1. Shared by ChatWithParams and chatOrStream.
func ResolveMaxAttempts(params RuntimeParams) int {
	maxAttempts := 1 + params.MaxRetries
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	return maxAttempts
}

// AttemptResult is the outcome of a single attempt passed to
// RetryWithBackoff. SafeToRetry tells the retry loop whether another
// attempt may be made after this one fails — it is the caller's own AND of
// every concern that matters at its call site: for ChatWithParams that's
// error classification alone (llm.IsRetryable, #1585); for the streaming
// path (chatOrStream) it's error classification AND "no stream callback
// has fired yet this attempt" (#1612), since StreamChat's callback has
// real side effects (event-sink emission) that must never be duplicated
// by a retry. SafeToRetry is ignored when Err is nil.
type AttemptResult[T any] struct {
	Value       T
	Err         error
	SafeToRetry bool
}

// RetryWithBackoff runs attempt up to maxAttempts times, sleeping with
// exponential backoff (bo) between attempts. It stops early — without
// consuming the remaining budget — when an attempt reports
// SafeToRetry=false, and aborts immediately if ctx is cancelled while
// sleeping between attempts. attempt receives the zero-based attempt index.
func RetryWithBackoff[T any](ctx context.Context, maxAttempts int, bo backoff.Config, attempt func(attemptIndex int) AttemptResult[T]) (T, error) {
	var lastErr error
	var zero T

	for i := 0; i < maxAttempts; i++ {
		result := attempt(i)
		if result.Err == nil {
			return result.Value, nil
		}
		lastErr = result.Err

		if !result.SafeToRetry {
			break
		}

		if i < maxAttempts-1 {
			delay := bo.Calculate(int32(i + 1))
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return zero, ctx.Err()
			case <-timer.C:
			}
		}
	}

	return zero, lastErr
}

// nonRetryableError marks a permanent (non-transient) LLM API error so
// ChatWithParams and the streaming retry path (#1612) can fail fast
// instead of consuming the retry budget on an error that will never
// succeed on retry (#1585). Provider adapters (anthropicfamily, openai)
// classify their own SDK-specific errors at the translation boundary they
// already own and call MarkNonRetryable — the generic retry machinery in
// this file never needs to know about provider-specific error shapes
// (DD-HAPI-019 Framework Isolation).
type nonRetryableError struct {
	err error
}

func (e *nonRetryableError) Error() string { return e.err.Error() }
func (e *nonRetryableError) Unwrap() error { return e.err }

// MarkNonRetryable wraps err so IsRetryable(err) reports false, while
// preserving err's message and identity for errors.Is/errors.As and any
// further fmt.Errorf("...: %w", ...) wrapping an adapter applies on top.
// Returns nil for a nil err.
func MarkNonRetryable(err error) error {
	if err == nil {
		return nil
	}
	return &nonRetryableError{err: err}
}

// IsRetryable reports whether err should be retried by ChatWithParams or
// the streaming retry path. Defaults to true for any error not explicitly
// marked via MarkNonRetryable — an unrecognized error shape is assumed
// transient (fail-safe posture per #1585 AC3: this is a targeted
// classification fix, not a rewrite of the retry policy's default
// posture). Returns false for a nil err (nothing to retry).
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var nre *nonRetryableError
	return !errors.As(err, &nre)
}

// IsNonRetryableHTTPStatus reports whether an HTTP status code represents a
// permanent client-side failure that will never succeed on retry (bad
// request, auth/authz failures, not-found) as opposed to a transient one
// (429 rate limit, 5xx server errors, and anything else, which default to
// retryable). Used by provider adapters to classify their SDK's typed
// errors (#1585).
func IsNonRetryableHTTPStatus(code int) bool {
	switch code {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return true
	default:
		return false
	}
}

// ChatWithParams wraps a Client.Chat call, injecting runtime parameters
// (temperature, timeout, retries) from the hot-reloadable LLM config.
// Each attempt gets a fresh context timeout; the timeout is cancelled
// immediately after Chat returns to avoid resource leaks.
// Retries use exponential backoff, respect parent context cancellation,
// and fail fast on errors an adapter has classified as non-retryable
// (#1585) instead of consuming the full retry budget.
func ChatWithParams(ctx context.Context, client Client, req ChatRequest, params RuntimeParams) (ChatResponse, error) {
	temp := params.Temperature
	req.Options.Temperature = &temp

	bo := ResolveRetryBackoff(params)
	maxAttempts := ResolveMaxAttempts(params)

	return RetryWithBackoff(ctx, maxAttempts, bo, func(int) AttemptResult[ChatResponse] {
		chatCtx := ctx
		var chatCancel context.CancelFunc
		if params.TimeoutSeconds > 0 {
			chatCtx, chatCancel = context.WithTimeout(ctx, time.Duration(params.TimeoutSeconds)*time.Second)
		}

		resp, err := client.Chat(chatCtx, req)

		if chatCancel != nil {
			chatCancel()
		}

		return AttemptResult[ChatResponse]{Value: resp, Err: err, SafeToRetry: IsRetryable(err)}
	})
}
