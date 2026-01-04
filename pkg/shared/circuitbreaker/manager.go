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

	"github.com/sony/gobreaker"
)

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
//	manager := circuitbreaker.NewManager(gobreaker.Settings{
//	    MaxRequests: 2,
//	    Timeout:     30 * time.Second,
//	    ReadyToTrip: func(counts gobreaker.Counts) bool {
//	        return counts.ConsecutiveFailures >= 3
//	    },
//	})
//	_, err := manager.Execute("slack", func() (interface{}, error) {
//	    return nil, slackService.Deliver(ctx, notification)
//	})
//
// Gateway Service (single K8s API):
//
//	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{...})
//	_, err := cb.Execute(func() (interface{}, error) {
//	    return nil, k8sClient.Create(ctx, crd)
//	})
type Manager struct {
	breakers map[string]*gobreaker.CircuitBreaker
	settings gobreaker.Settings
	mu       sync.RWMutex
}

// NewManager creates a circuit breaker manager with shared settings
//
// Parameters:
// - settings: Base gobreaker.Settings applied to all channels
//   Note: settings.Name will be overridden per channel
func NewManager(settings gobreaker.Settings) *Manager {
	return &Manager{
		breakers: make(map[string]*gobreaker.CircuitBreaker),
		settings: settings,
	}
}

// Execute runs a function with circuit breaker protection for a specific channel
//
// This is the recommended method as it automatically tracks success/failure
// and handles state transitions.
//
// Parameters:
// - channel: Resource identifier (e.g., "slack", "console", "k8s-api")
// - fn: Function to execute with circuit breaker protection
//
// Returns:
// - result: Function return value (or nil if error)
// - error: gobreaker.ErrOpenState if circuit is open, or function error
//
// Example:
//
//	result, err := manager.Execute("slack", func() (interface{}, error) {
//	    return slackService.Deliver(ctx, notification)
//	})
//	if err == gobreaker.ErrOpenState {
//	    // Circuit breaker is open, fail fast
//	}
func (m *Manager) Execute(channel string, fn func() (interface{}, error)) (interface{}, error) {
	cb := m.getOrCreate(channel)
	return cb.Execute(fn)
}

// AllowRequest checks if circuit breaker allows requests for a channel
//
// This method is provided for backward compatibility with existing code
// that manually checks circuit breaker state before operations.
//
// Returns:
// - true: Circuit is Closed or HalfOpen (allow requests)
// - false: Circuit is Open (reject requests)
//
// Example:
//
//	if !manager.AllowRequest("slack") {
//	    return fmt.Errorf("slack circuit breaker is open")
//	}
func (m *Manager) AllowRequest(channel string) bool {
	cb := m.getOrCreate(channel)
	return cb.State() != gobreaker.StateOpen
}

// State returns the current state of a channel's circuit breaker
//
// Returns:
// - gobreaker.StateClosed: Normal operation (0)
// - gobreaker.StateHalfOpen: Testing recovery (1)
// - gobreaker.StateOpen: Blocking requests (2)
//
// Example:
//
//	state := manager.State("slack")
//	switch state {
//	case gobreaker.StateClosed:
//	    // Normal operation
//	case gobreaker.StateOpen:
//	    // Failing fast
//	case gobreaker.StateHalfOpen:
//	    // Testing recovery
//	}
func (m *Manager) State(channel string) gobreaker.State {
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
func (m *Manager) getOrCreate(channel string) *gobreaker.CircuitBreaker {
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
	cb := gobreaker.NewCircuitBreaker(settings)
	m.breakers[channel] = cb
	return cb
}

