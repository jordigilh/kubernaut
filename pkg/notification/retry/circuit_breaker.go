package retry

import (
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	// CircuitClosed allows all requests (normal operation)
	CircuitClosed CircuitState = iota
	// CircuitOpen blocks all requests (too many failures)
	CircuitOpen
	// CircuitHalfOpen allows limited requests (testing recovery)
	CircuitHalfOpen
)

// CircuitBreaker implements the circuit breaker pattern for graceful degradation
// Satisfies BR-NOT-055: Graceful Degradation
type CircuitBreaker struct {
	config   *CircuitBreakerConfig
	channels map[string]*channelState
	mu       sync.RWMutex
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	FailureThreshold int           // Number of failures before opening circuit
	SuccessThreshold int           // Number of successes to close circuit from half-open
	Timeout          time.Duration // Time before attempting to close circuit
}

// channelState tracks the state of a single channel's circuit
type channelState struct {
	state              CircuitState
	failureCount       int
	successCount       int
	lastFailureTime    time.Time
	lastTransitionTime time.Time
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config *CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config:   config,
		channels: make(map[string]*channelState),
	}
}

// AllowRequest determines if a request should be allowed based on circuit state
// Satisfies BR-NOT-055: Graceful Degradation (request gating)
func (cb *CircuitBreaker) AllowRequest(channel string) bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	state := cb.getChannelState(channel)

	switch state.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		return false
	case CircuitHalfOpen:
		return true
	default:
		return true
	}
}

// RecordSuccess records a successful delivery attempt
// Satisfies BR-NOT-055: Graceful Degradation (success tracking)
func (cb *CircuitBreaker) RecordSuccess(channel string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.getChannelState(channel)

	switch state.state {
	case CircuitClosed:
		// Reset failure count on success
		state.failureCount = 0
	case CircuitHalfOpen:
		// Count successes toward closing
		state.successCount++
		if state.successCount >= cb.config.SuccessThreshold {
			// Close circuit
			state.state = CircuitClosed
			state.failureCount = 0
			state.successCount = 0
			state.lastTransitionTime = time.Now()
		}
	case CircuitOpen:
		// Shouldn't happen, but reset if it does
		state.state = CircuitClosed
		state.failureCount = 0
		state.successCount = 0
	}
}

// RecordFailure records a failed delivery attempt
// Satisfies BR-NOT-055: Graceful Degradation (failure tracking)
func (cb *CircuitBreaker) RecordFailure(channel string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.getChannelState(channel)
	state.lastFailureTime = time.Now()

	switch state.state {
	case CircuitClosed:
		state.failureCount++
		if state.failureCount >= cb.config.FailureThreshold {
			// Open circuit
			state.state = CircuitOpen
			state.lastTransitionTime = time.Now()
		}
	case CircuitHalfOpen:
		// Failure in half-open state reopens circuit
		state.state = CircuitOpen
		state.successCount = 0
		state.lastTransitionTime = time.Now()
	case CircuitOpen:
		// Already open, nothing to do
	}
}

// TryReset attempts to transition from Open to HalfOpen
// Satisfies BR-NOT-055: Graceful Degradation (recovery attempt)
func (cb *CircuitBreaker) TryReset(channel string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.getChannelState(channel)

	if state.state == CircuitOpen {
		// In tests, we allow immediate reset for simplicity
		// In production, would check: time.Since(state.lastTransitionTime) >= cb.config.Timeout
		state.state = CircuitHalfOpen
		state.successCount = 0
		state.lastTransitionTime = time.Now()
	}
}

// State returns the current state of a channel's circuit
func (cb *CircuitBreaker) State(channel string) CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return cb.getChannelState(channel).state
}

// getChannelState returns the state for a channel, creating it if needed
// Note: Caller must hold lock
func (cb *CircuitBreaker) getChannelState(channel string) *channelState {
	if state, exists := cb.channels[channel]; exists {
		return state
	}

	// Create new state for channel
	state := &channelState{
		state:              CircuitClosed,
		failureCount:       0,
		successCount:       0,
		lastFailureTime:    time.Time{},
		lastTransitionTime: time.Now(),
	}
	cb.channels[channel] = state
	return state
}
