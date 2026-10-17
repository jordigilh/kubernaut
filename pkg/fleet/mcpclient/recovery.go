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

package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrSessionClosed is returned when a reconnect or provider lookup races with
// the owner's close fence. It is intentionally stable for callers that need
// to distinguish shutdown from transient gateway failure.
var ErrSessionClosed = errors.New("mcp session closed")

// TransportFailureKind is the server-owned classification of a transport
// boundary failure. The classifier is intentionally separate from BridgeTool
// so replay policy never has to parse SDK or HTTP error strings.
type TransportFailureKind string

const (
	FailureSessionReset   TransportFailureKind = "session_reset"
	FailureHTTPTransient  TransportFailureKind = "http_transient"
	FailureUnauthorized   TransportFailureKind = "unauthorized"
	FailureCallerCanceled TransportFailureKind = "caller_canceled"
	FailurePermanent      TransportFailureKind = "permanent"
	FailureUnknown        TransportFailureKind = "unknown"
)

// RecoveryAction is the bounded operation permitted after a transport
// failure. Semantic MCP tool errors are never passed to this classifier.
type RecoveryAction string

const (
	ActionReconnectAndRetryRead RecoveryAction = "reconnect_and_retry_read"
	ActionRetryRead             RecoveryAction = "retry_read"
	ActionFail                  RecoveryAction = "fail"
)

// RetryObservation is the dependency-free recovery telemetry contract. The
// shared MCP package deliberately does not know about Prometheus or the KA
// audit package; composition roots adapt this bounded observation to those
// sinks. The request context is passed to the observer separately so it can
// recover correlation metadata without storing a context in telemetry.
type RetryObservation struct {
	Source       string
	ToolClass    string
	ClusterID    string
	LogicalTool  string
	Attempt      int
	Outcome      string
	FailureClass TransportFailureKind
	FailureCode  string
}

const (
	RetryOutcomeRetry     = "retry"
	RetryOutcomeSuccess   = "success"
	RetryOutcomeFailed    = "failed"
	RetryOutcomeExhausted = "exhausted"
)

// RetryObserver receives bounded recovery observations. Implementations must
// be non-blocking and must not make recovery depend on an audit or metrics
// backend being available.
type RetryObserver interface {
	ObserveRetry(context.Context, RetryObservation)
}

// RetryObserverFunc adapts a context-aware function to RetryObserver.
type RetryObserverFunc func(context.Context, RetryObservation)

func (f RetryObserverFunc) ObserveRetry(ctx context.Context, observation RetryObservation) {
	if f != nil {
		f(ctx, observation)
	}
}

// TransportFailure carries the normalized failure metadata used by recovery,
// audit, and metrics adapters. Cause remains available to callers through
// errors.Is/errors.As; its text must not be persisted as an unbounded label.
type TransportFailure struct {
	Kind         TransportFailureKind
	HTTPStatus   int
	RetryAfter   time.Duration
	SessionState string
	FailureCode  string
	Cause        error
}

func (f *TransportFailure) Error() string {
	if f == nil {
		return "transport failure"
	}
	if f.Cause == nil {
		return string(f.Kind)
	}
	return fmt.Sprintf("%s: %v", f.Kind, f.Cause)
}

// Unwrap exposes the original SDK/network error without making it part of the
// bounded classification contract.
func (f *TransportFailure) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.Cause
}

// Action returns the safe recovery operation for the normalized failure.
func (f *TransportFailure) Action() RecoveryAction {
	if f == nil {
		return ActionFail
	}
	switch f.Kind {
	case FailureSessionReset, FailureUnauthorized:
		return ActionReconnectAndRetryRead
	case FailureHTTPTransient:
		return ActionRetryRead
	default:
		return ActionFail
	}
}

// HTTPStatusCode is the narrow optional error seam used by HTTP transports.
// The MCP SDK does not expose the response status in every version, so the
// boundary may provide it without coupling this package to a concrete client.
type HTTPStatusCode interface {
	HTTPStatusCode() int
}

// RetryAfterProvider is the optional bounded retry hint exposed by an HTTP
// transport adapter.
type RetryAfterProvider interface {
	RetryAfterDuration() time.Duration
}

// ClassifyTransportError converts an SDK/network/HTTP failure into a typed
// failure. Caller cancellation takes precedence over every transport class.
func ClassifyTransportError(err error) *TransportFailure {
	if err == nil {
		return nil
	}

	failure := &TransportFailure{Cause: err, FailureCode: "transport_unknown"}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		failure.Kind = FailureCallerCanceled
		failure.FailureCode = "caller_canceled"
		return failure
	}

	if status, ok := httpStatusFromError(err); ok {
		failure.HTTPStatus = status
		if retryAfter, ok := retryAfterFromError(err); ok {
			failure.RetryAfter = retryAfter
		}
		switch status {
		case http.StatusUnauthorized:
			failure.Kind = FailureUnauthorized
			failure.FailureCode = "http_401"
		case http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			failure.Kind = FailureHTTPTransient
			failure.FailureCode = fmt.Sprintf("http_%d", status)
		case http.StatusBadRequest,
			http.StatusForbidden,
			http.StatusNotFound,
			http.StatusUnprocessableEntity:
			failure.Kind = FailurePermanent
			failure.FailureCode = fmt.Sprintf("http_%d", status)
		default:
			failure.Kind = FailureUnknown
			failure.FailureCode = "http_unknown"
		}
		return failure
	}

	switch {
	case errors.Is(err, mcp.ErrConnectionClosed),
		errors.Is(err, mcp.ErrSessionMissing),
		errors.Is(err, io.EOF),
		errors.Is(err, io.ErrUnexpectedEOF),
		errors.Is(err, io.ErrNoProgress),
		errors.Is(err, net.ErrClosed),
		errors.Is(err, syscall.ECONNRESET),
		errors.Is(err, syscall.EPIPE):
		failure.Kind = FailureSessionReset
		failure.FailureCode = "session_reset"
	case errors.Is(err, syscall.ECONNREFUSED):
		failure.Kind = FailureSessionReset
		failure.FailureCode = "connection_refused"
	default:
		var opErr *net.OpError
		var netErr net.Error
		switch {
		case errors.As(err, &opErr):
			failure.Kind = FailureSessionReset
			failure.FailureCode = "network_error"
		case errors.As(err, &netErr) && netErr.Timeout():
			failure.Kind = FailureSessionReset
			failure.FailureCode = "transport_timeout"
		default:
			failure.Kind = FailureUnknown
		}
	}
	return failure
}

// httpStatusFromError recognizes status-bearing adapter errors and the fixed
// status text emitted by the MCP SDK. The recognition happens once at the
// transport boundary; recovery code never scans arbitrary error text.
func httpStatusFromError(err error) (int, bool) {
	var statusErr HTTPStatusCode
	if errors.As(err, &statusErr) {
		status := statusErr.HTTPStatusCode()
		if status >= 100 && status <= 599 {
			return status, true
		}
	}

	message := err.Error()
	for _, status := range []int{
		http.StatusUnauthorized,
		http.StatusBadRequest,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusUnprocessableEntity,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		statusText := http.StatusText(status)
		if strings.Contains(message, statusText) {
			return status, true
		}
	}
	return 0, false
}

func retryAfterFromError(err error) (time.Duration, bool) {
	var retryAfter RetryAfterProvider
	if !errors.As(err, &retryAfter) {
		return 0, false
	}
	delay := retryAfter.RetryAfterDuration()
	if delay < 0 {
		return 0, false
	}
	return delay, true
}

// ReadRecoveryPolicy owns transport-call recovery defaults. It is deliberately
// separate from FleetResilienceConfig, which controls startup/readiness
// connection behavior rather than per-call business retries.
type ReadRecoveryPolicy struct {
	MaxAttempts    int
	MaxElapsed     time.Duration
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	RetryAfterCap  time.Duration
	Jitter         func(attempt int, base time.Duration) time.Duration
	Sleep          func(context.Context, time.Duration) error
}

// DefaultReadRecoveryPolicy returns the bounded read policy required by
// DD-FLEET-009. MaxAttempts includes the initial call.
func DefaultReadRecoveryPolicy() ReadRecoveryPolicy {
	return ReadRecoveryPolicy{
		MaxAttempts:    3,
		MaxElapsed:     10 * time.Second,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
		RetryAfterCap:  5 * time.Second,
		Jitter:         func(_ int, base time.Duration) time.Duration { return base },
		Sleep:          sleepWithContext,
	}
}

func (p ReadRecoveryPolicy) normalized() ReadRecoveryPolicy {
	defaults := DefaultReadRecoveryPolicy()
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = defaults.MaxAttempts
	}
	if p.MaxElapsed <= 0 {
		p.MaxElapsed = defaults.MaxElapsed
	}
	if p.InitialBackoff <= 0 {
		p.InitialBackoff = defaults.InitialBackoff
	}
	if p.MaxBackoff <= 0 {
		p.MaxBackoff = defaults.MaxBackoff
	}
	if p.RetryAfterCap <= 0 {
		p.RetryAfterCap = defaults.RetryAfterCap
	}
	if p.Jitter == nil {
		p.Jitter = defaults.Jitter
	}
	if p.Sleep == nil {
		p.Sleep = defaults.Sleep
	}
	return p
}

func (p ReadRecoveryPolicy) delay(attempt int, failure *TransportFailure) time.Duration {
	p = p.normalized()
	if attempt < 1 {
		attempt = 1
	}
	backoff := p.InitialBackoff
	for i := 1; i < attempt && backoff < p.MaxBackoff; i++ {
		if backoff > p.MaxBackoff/2 {
			backoff = p.MaxBackoff
			break
		}
		backoff *= 2
	}
	if backoff > p.MaxBackoff {
		backoff = p.MaxBackoff
	}
	if failure != nil && failure.RetryAfter > backoff {
		retryAfter := failure.RetryAfter
		if retryAfter > p.RetryAfterCap {
			retryAfter = p.RetryAfterCap
		}
		if retryAfter > backoff {
			backoff = retryAfter
		}
	}
	return p.Jitter(attempt, backoff)
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func boundedRecoveryContext(parent context.Context, maxElapsed time.Duration) (context.Context, context.CancelFunc) {
	if maxElapsed <= 0 {
		return parent, func() {}
	}
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) <= maxElapsed {
		return parent, func() {}
	}
	return context.WithTimeout(parent, maxElapsed)
}
