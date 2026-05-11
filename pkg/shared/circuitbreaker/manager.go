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

package circuitbreaker

import (
	"sync"
	"time"

	"github.com/sony/gobreaker/v2"
)

// State represents the circuit breaker state. Re-exported so callers
// never need to import gobreaker directly.
type State = gobreaker.State

const (
	StateClosed   = gobreaker.StateClosed
	StateHalfOpen = gobreaker.StateHalfOpen
	StateOpen     = gobreaker.StateOpen
)

// ErrOpenState is returned when the circuit breaker is open and rejects
// the call. Re-exported so callers never need to import gobreaker.
var ErrOpenState = gobreaker.ErrOpenState

// ManagerConfig controls the circuit breaker Manager behavior.
// Mirrors transport.CircuitBreakerConfig so that callers never import
// gobreaker directly.
type ManagerConfig struct {
	// MaxRequests is the number of requests allowed in half-open state.
	MaxRequests uint32

	// Interval is the cyclic period of the closed state for clearing
	// internal counts. If 0, internal counts are never cleared.
	Interval time.Duration

	// Timeout is the period of the open state, after which the state
	// transitions to half-open.
	Timeout time.Duration

	// ConsecutiveFailureThreshold trips the circuit when consecutive
	// failures reach this count. 0 means never trip.
	ConsecutiveFailureThreshold uint32

	// OnStateChange is called when any channel's breaker transitions state.
	OnStateChange func(name string, from, to State)
}

// Manager manages multiple circuit breakers (one per resource/channel)
//
// Business Requirements:
// - BR-NOT-055: Graceful Degradation (per-channel isolation for Notification)
// - BR-GATEWAY-XXX: K8s API protection (single-resource for Gateway)
//
// Design Pattern:
// This wrapper provides a clean abstraction for managing multiple circuit breakers,
// enabling per-channel isolation (Slack, console, webhooks) while using the
// battle-tested github.com/sony/gobreaker library.
//
// Usage Examples:
//
// Notification Service (multi-channel):
//
//	manager := circuitbreaker.NewManager(circuitbreaker.ManagerConfig{
//	    MaxRequests:                 2,
//	    Timeout:                    30 * time.Second,
//	    ConsecutiveFailureThreshold: 3,
//	})
//	_, err := manager.Execute("slack", func() (interface{}, error) {
//	    return nil, slackService.Deliver(ctx, notification)
//	})
type Manager struct {
	breakers map[string]*gobreaker.CircuitBreaker[any]
	settings gobreaker.Settings
	mu       sync.RWMutex
}

// NewManager creates a circuit breaker manager with shared settings.
// The ManagerConfig is translated to gobreaker.Settings internally so
// that callers never depend on gobreaker types.
func NewManager(cfg ManagerConfig) *Manager {
	settings := gobreaker.Settings{
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
	}

	if cfg.ConsecutiveFailureThreshold > 0 {
		threshold := cfg.ConsecutiveFailureThreshold
		settings.ReadyToTrip = func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= threshold
		}
	}

	if cfg.OnStateChange != nil {
		settings.OnStateChange = cfg.OnStateChange
	}

	return &Manager{
		breakers: make(map[string]*gobreaker.CircuitBreaker[any]),
		settings: settings,
	}
}

// Execute runs a function with circuit breaker protection for a specific channel.
//
// Returns ErrOpenState when the circuit is open (fail-fast).
func (m *Manager) Execute(channel string, fn func() (interface{}, error)) (interface{}, error) {
	cb := m.getOrCreate(channel)
	return cb.Execute(fn)
}

// AllowRequest checks if circuit breaker allows requests for a channel.
// Returns false when the circuit is Open (reject requests).
func (m *Manager) AllowRequest(channel string) bool {
	cb := m.getOrCreate(channel)
	return cb.State() != StateOpen
}

// State returns the current state of a channel's circuit breaker
// (StateClosed, StateHalfOpen, or StateOpen).
func (m *Manager) State(channel string) State {
	cb := m.getOrCreate(channel)
	return cb.State()
}

// RecordSuccess manually records a success for a channel
//
// NOTE: This method is provided for backward compatibility.
// Prefer using Execute() which automatically tracks success/failure.
//
// This is a no-op for gobreaker as success is tracked automatically
// via Execute(). Kept for API compatibility with existing code.
func (m *Manager) RecordSuccess(channel string) {
	// gobreaker tracks success automatically via Execute()
	// This method exists for API compatibility with manual tracking patterns
	// No-op: success is recorded when Execute() returns nil error
}

// RecordFailure manually records a failure for a channel
//
// NOTE: This method is provided for backward compatibility.
// Prefer using Execute() which automatically tracks success/failure.
//
// This is a no-op for gobreaker as failures are tracked automatically
// via Execute(). Kept for API compatibility with existing code.
func (m *Manager) RecordFailure(channel string) {
	// gobreaker tracks failures automatically via Execute()
	// This method exists for API compatibility with manual tracking patterns
	// No-op: failure is recorded when Execute() returns error
}

// getOrCreate returns existing or creates new circuit breaker for channel
//
// Thread-safe: Uses read lock for fast path (breaker exists),
// write lock only for slow path (create new breaker).
//
// Note: Caller does not need to hold lock, this method handles locking internally.
func (m *Manager) getOrCreate(channel string) *gobreaker.CircuitBreaker[any] {
	// Fast path: breaker already exists
	m.mu.RLock()
	if cb, exists := m.breakers[channel]; exists {
		m.mu.RUnlock()
		return cb
	}
	m.mu.RUnlock()

	// Slow path: create new breaker
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have created it)
	if cb, exists := m.breakers[channel]; exists {
		return cb
	}

	// Create channel-specific settings (override name)
	settings := m.settings
	settings.Name = channel

	// Create and store circuit breaker for this channel
	cb := gobreaker.NewCircuitBreaker[any](settings)
	m.breakers[channel] = cb
	return cb
}

