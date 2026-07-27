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

package transport

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/sony/gobreaker/v2"
)

// CircuitBreakerConfig controls the circuit breaker behavior.
type CircuitBreakerConfig struct {
	// Enabled activates circuit breaker protection.
	Enabled bool

	// Name identifies this breaker in logs and metrics.
	Name string

	// MaxRequests is the number of requests allowed in half-open state.
	MaxRequests uint32

	// Interval is the cyclic period of the closed state for clearing
	// internal counts. If 0, internal counts are never cleared.
	Interval time.Duration

	// Timeout is the period of the open state, after which the state
	// transitions to half-open.
	Timeout time.Duration

	// FailureThreshold is the minimum number of requests before
	// ReadyToTrip evaluates the failure ratio.
	FailureThreshold uint32

	// FailureRatio is the failure ratio (0.0–1.0) that triggers the
	// circuit to open when FailureThreshold requests have been made.
	FailureRatio float64

	// Logger for state change events. Zero-value disables logging.
	Logger logr.Logger

	// OnStateChange is called when the circuit breaker transitions state.
	// Use for metrics integration (e.g., Prometheus gauge).
	OnStateChange func(name string, from, to gobreaker.State)
}

// DefaultCircuitBreakerConfig returns production-ready defaults matching
// the gateway K8s API circuit breaker (BR-GATEWAY-093).
func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Enabled:          false,
		Name:             name,
		MaxRequests:      3,
		Interval:         10 * time.Second,
		Timeout:          30 * time.Second,
		FailureThreshold: 10,
		FailureRatio:     0.5,
	}
}

// CircuitBreakerTransport wraps an http.RoundTripper with gobreaker
// circuit breaker protection. When the breaker is open, requests fail
// immediately with gobreaker.ErrOpenState (fail-fast).
//
// Intended placement in the transport chain:
//
//	CircuitBreakerTransport -> RetryTransport -> Auth -> TLS/base
//
// This ensures that when the circuit is open, no retries are wasted.
type CircuitBreakerTransport struct {
	next http.RoundTripper
	cb   *gobreaker.CircuitBreaker[*http.Response]
}

// NewCircuitBreakerTransport creates a CircuitBreakerTransport wrapping
// next. If cfg.Enabled is false, returns next directly (no-op passthrough).
func NewCircuitBreakerTransport(next http.RoundTripper, cfg CircuitBreakerConfig) http.RoundTripper {
	if !cfg.Enabled {
		return next
	}

	failureThreshold := cfg.FailureThreshold
	if failureThreshold == 0 {
		failureThreshold = 10
	}
	failureRatio := cfg.FailureRatio
	if failureRatio <= 0 || failureRatio > 1 {
		failureRatio = 0.5
	}

	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			ratio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= failureThreshold && ratio >= failureRatio
		},
	}

	if cfg.OnStateChange != nil {
		settings.OnStateChange = cfg.OnStateChange
	} else if cfg.Logger.GetSink() != nil {
		logger := cfg.Logger
		settings.OnStateChange = func(name string, from, to gobreaker.State) {
			logger.Info("circuit breaker state change",
				"name", name,
				"from", from.String(),
				"to", to.String())
		}
	}

	return &CircuitBreakerTransport{
		next: next,
		// nolint:bodyclose // false positive: NewCircuitBreaker[T any](st Settings)
		// *CircuitBreaker[T] is a generic constructor with no HTTP semantics;
		// bodyclose misparses the *http.Response type parameter as an HTTP call
		// site. RoundTrip below always returns either (resp, nil) or (nil, err)
		// to its caller, never both, so the caller's normal Body-close obligation
		// applies unchanged (Issue #1546 Tier 2).
		cb: gobreaker.NewCircuitBreaker[*http.Response](settings),
	}
}

// RoundTrip executes the HTTP request through the circuit breaker.
// Returns gobreaker.ErrOpenState when the circuit is open.
func (t *CircuitBreakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.cb.Execute(func() (*http.Response, error) {
		resp, err := t.next.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		if isCircuitBreakerFailure(resp.StatusCode) {
			return resp, fmt.Errorf("circuit breaker: server returned %d", resp.StatusCode)
		}
		return resp, nil
	})

	if err != nil {
		if resp != nil {
			return resp, nil
		}
		return nil, err
	}

	return resp, nil
}

// State returns the current circuit breaker state (for observability).
func (t *CircuitBreakerTransport) State() gobreaker.State {
	return t.cb.State()
}

// isCircuitBreakerFailure returns true for HTTP status codes that should
// count as circuit breaker failures. Aligns with RetryTransport's
// retryable statuses to ensure consistent failure classification.
func isCircuitBreakerFailure(code int) bool {
	return code == http.StatusBadGateway ||
		code == http.StatusServiceUnavailable ||
		code == http.StatusGatewayTimeout
}
