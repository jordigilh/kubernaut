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

package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int32

const (
	// StateClosed - requests are allowed through
	StateClosed CircuitState = iota
	// StateOpen - requests are immediately failed
	StateOpen
	// StateHalfOpen - limited requests are allowed through to test service recovery
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig contains configuration for circuit breaker behavior
// Business Requirement: BR-EXTERNAL-001 - External API circuit breakers with rate limiting
type CircuitBreakerConfig struct {
	// Failure threshold - number of consecutive failures before opening
	FailureThreshold int `yaml:"failure_threshold" json:"failure_threshold"`

	// Recovery timeout - how long to wait before trying again
	RecoveryTimeout time.Duration `yaml:"recovery_timeout" json:"recovery_timeout"`

	// Success threshold - number of consecutive successes in half-open state to close circuit
	SuccessThreshold int `yaml:"success_threshold" json:"success_threshold"`

	// Request timeout for individual requests
	RequestTimeout time.Duration `yaml:"request_timeout" json:"request_timeout"`

	// Rate limiting configuration
	RequestsPerSecond int `yaml:"requests_per_second" json:"requests_per_second"`
	BurstLimit        int `yaml:"burst_limit" json:"burst_limit"`

	// Health check configuration
	HealthCheckInterval time.Duration `yaml:"health_check_interval" json:"health_check_interval"`
	HealthCheckPath     string        `yaml:"health_check_path" json:"health_check_path"`

	// Monitoring configuration
	EnableMetrics   bool          `yaml:"enable_metrics" json:"enable_metrics"`
	MetricsInterval time.Duration `yaml:"metrics_interval" json:"metrics_interval"`
}

// DefaultCircuitBreakerConfig returns default configuration
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold:    5,
		RecoveryTimeout:     30 * time.Second,
		SuccessThreshold:    3,
		RequestTimeout:      10 * time.Second,
		RequestsPerSecond:   100,
		BurstLimit:          20,
		HealthCheckInterval: 30 * time.Second,
		HealthCheckPath:     "/health",
		EnableMetrics:       true,
		MetricsInterval:     5 * time.Minute,
	}
}

// CircuitBreakerMetrics contains metrics for monitoring circuit breaker behavior
type CircuitBreakerMetrics struct {
	State                CircuitState  `json:"state"`
	TotalRequests        int64         `json:"total_requests"`
	SuccessfulRequests   int64         `json:"successful_requests"`
	FailedRequests       int64         `json:"failed_requests"`
	RejectedRequests     int64         `json:"rejected_requests"`
	ConsecutiveFailures  int32         `json:"consecutive_failures"`
	ConsecutiveSuccesses int32         `json:"consecutive_successes"`
	LastFailureTime      time.Time     `json:"last_failure_time"`
	LastSuccessTime      time.Time     `json:"last_success_time"`
	StateChangedAt       time.Time     `json:"state_changed_at"`
	AverageResponseTime  time.Duration `json:"average_response_time"`
	RateLimitHits        int64         `json:"rate_limit_hits"`
	HealthScore          float64       `json:"health_score"`
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	tokensPerSecond int
	bucketSize      int
	tokens          int64
	lastRefill      int64
	mutex           sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(tokensPerSecond, bucketSize int) *RateLimiter {
	return &RateLimiter{
		tokensPerSecond: tokensPerSecond,
		bucketSize:      bucketSize,
		tokens:          int64(bucketSize),
		lastRefill:      time.Now().UnixNano(),
	}
}

// Allow checks if a request is allowed under rate limiting
func (rl *RateLimiter) Allow() bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now().UnixNano()
	elapsed := now - rl.lastRefill

	// Refill tokens based on elapsed time
	tokensToAdd := (elapsed / int64(time.Second)) * int64(rl.tokensPerSecond)
	rl.tokens += tokensToAdd
	if rl.tokens > int64(rl.bucketSize) {
		rl.tokens = int64(rl.bucketSize)
	}
	rl.lastRefill = now

	// Check if we have tokens
	if rl.tokens > 0 {
		rl.tokens--
		return true
	}

	return false
}

// CircuitBreaker implements circuit breaker pattern with rate limiting
// Business Requirement: BR-EXTERNAL-001 - External API circuit breakers and rate limiting
type CircuitBreaker struct {
	name        string
	config      *CircuitBreakerConfig
	client      *http.Client
	logger      *logrus.Logger
	rateLimiter *RateLimiter

	// State management
	state                int32 // CircuitState stored as int32 for atomic operations
	consecutiveFailures  int32
	consecutiveSuccesses int32
	stateChangedAt       int64 // Unix timestamp

	// Metrics (atomic counters for thread safety)
	totalRequests      int64
	successfulRequests int64
	failedRequests     int64
	rejectedRequests   int64
	rateLimitHits      int64
	totalResponseTime  int64 // Nanoseconds
	lastFailureTime    int64 // Unix timestamp
	lastSuccessTime    int64 // Unix timestamp

	// Monitoring
	metricsTimer *time.Timer
	healthTimer  *time.Timer
	stopChannel  chan struct{}
	wg           sync.WaitGroup
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(name string, config *CircuitBreakerConfig, client *http.Client, logger *logrus.Logger) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}

	cb := &CircuitBreaker{
		name:           name,
		config:         config,
		client:         client,
		logger:         logger,
		rateLimiter:    NewRateLimiter(config.RequestsPerSecond, config.BurstLimit),
		state:          int32(StateClosed),
		stateChangedAt: time.Now().Unix(),
		stopChannel:    make(chan struct{}),
	}

	// Start monitoring if enabled
	if config.EnableMetrics {
		cb.startMonitoring()
	}

	logger.WithFields(logrus.Fields{
		"circuit_breaker":     name,
		"failure_threshold":   config.FailureThreshold,
		"recovery_timeout":    config.RecoveryTimeout,
		"requests_per_second": config.RequestsPerSecond,
	}).Info("Circuit breaker initialized")

	return cb
}

// Do executes an HTTP request through the circuit breaker
func (cb *CircuitBreaker) Do(req *http.Request) (*http.Response, error) {
	// Check rate limiting first
	if !cb.rateLimiter.Allow() {
		atomic.AddInt64(&cb.rateLimitHits, 1)
		atomic.AddInt64(&cb.rejectedRequests, 1)
		cb.logger.WithField("circuit_breaker", cb.name).Debug("Request rejected due to rate limiting")
		return nil, errors.New("rate limit exceeded")
	}

	// Check circuit breaker state
	currentState := CircuitState(atomic.LoadInt32(&cb.state))
	switch currentState {
	case StateOpen:
		// Check if recovery timeout has passed
		stateChangedAt := time.Unix(atomic.LoadInt64(&cb.stateChangedAt), 0)
		if time.Since(stateChangedAt) < cb.config.RecoveryTimeout {
			atomic.AddInt64(&cb.rejectedRequests, 1)
			return nil, fmt.Errorf("circuit breaker %s is open", cb.name)
		}
		// Transition to half-open
		if atomic.CompareAndSwapInt32(&cb.state, int32(StateOpen), int32(StateHalfOpen)) {
			atomic.StoreInt64(&cb.stateChangedAt, time.Now().Unix())
			atomic.StoreInt32(&cb.consecutiveSuccesses, 0)
			cb.logger.WithField("circuit_breaker", cb.name).Info("Circuit breaker transitioned to half-open")
		}
	case StateHalfOpen:
		// Limit concurrent requests in half-open state
		// For simplicity, we'll allow the request through but track it carefully
	case StateClosed:
		// Normal operation
	}

	// Execute the request with timeout
	ctx, cancel := context.WithTimeout(req.Context(), cb.config.RequestTimeout)
	defer cancel()
	req = req.WithContext(ctx)

	startTime := time.Now()
	atomic.AddInt64(&cb.totalRequests, 1)

	resp, err := cb.client.Do(req)
	responseTime := time.Since(startTime)

	// Update response time metrics
	atomic.AddInt64(&cb.totalResponseTime, responseTime.Nanoseconds())

	// Handle response
	if err != nil || (resp != nil && resp.StatusCode >= 500) {
		cb.onFailure()
		if err != nil {
			return nil, fmt.Errorf("circuit breaker %s request failed: %w", cb.name, err)
		}
	} else {
		cb.onSuccess()
	}

	return resp, err
}

// onSuccess handles successful requests
func (cb *CircuitBreaker) onSuccess() {
	atomic.AddInt64(&cb.successfulRequests, 1)
	atomic.StoreInt64(&cb.lastSuccessTime, time.Now().Unix())
	atomic.StoreInt32(&cb.consecutiveFailures, 0)

	currentState := CircuitState(atomic.LoadInt32(&cb.state))
	if currentState == StateHalfOpen {
		successes := atomic.AddInt32(&cb.consecutiveSuccesses, 1)
		if int(successes) >= cb.config.SuccessThreshold {
			// Close the circuit
			if atomic.CompareAndSwapInt32(&cb.state, int32(StateHalfOpen), int32(StateClosed)) {
				atomic.StoreInt64(&cb.stateChangedAt, time.Now().Unix())
				atomic.StoreInt32(&cb.consecutiveSuccesses, 0)
				cb.logger.WithField("circuit_breaker", cb.name).Info("Circuit breaker closed after successful recovery")
			}
		}
	}
}

// onFailure handles failed requests
func (cb *CircuitBreaker) onFailure() {
	atomic.AddInt64(&cb.failedRequests, 1)
	atomic.StoreInt64(&cb.lastFailureTime, time.Now().Unix())
	atomic.StoreInt32(&cb.consecutiveSuccesses, 0)

	failures := atomic.AddInt32(&cb.consecutiveFailures, 1)
	currentState := CircuitState(atomic.LoadInt32(&cb.state))

	if currentState == StateClosed && int(failures) >= cb.config.FailureThreshold {
		// Open the circuit
		if atomic.CompareAndSwapInt32(&cb.state, int32(StateClosed), int32(StateOpen)) {
			atomic.StoreInt64(&cb.stateChangedAt, time.Now().Unix())
			cb.logger.WithFields(logrus.Fields{
				"circuit_breaker":      cb.name,
				"consecutive_failures": failures,
			}).Warn("Circuit breaker opened due to consecutive failures")
		}
	} else if currentState == StateHalfOpen {
		// Go back to open on any failure in half-open state
		if atomic.CompareAndSwapInt32(&cb.state, int32(StateHalfOpen), int32(StateOpen)) {
			atomic.StoreInt64(&cb.stateChangedAt, time.Now().Unix())
			atomic.StoreInt32(&cb.consecutiveFailures, 1)
			cb.logger.WithField("circuit_breaker", cb.name).Warn("Circuit breaker reopened after failure in half-open state")
		}
	}
}

// GetMetrics returns current circuit breaker metrics
func (cb *CircuitBreaker) GetMetrics() CircuitBreakerMetrics {
	totalReqs := atomic.LoadInt64(&cb.totalRequests)
	totalTime := atomic.LoadInt64(&cb.totalResponseTime)

	var avgResponseTime time.Duration
	if totalReqs > 0 {
		avgResponseTime = time.Duration(totalTime / totalReqs)
	}

	// Calculate health score
	successRate := float64(0)
	if totalReqs > 0 {
		successRate = float64(atomic.LoadInt64(&cb.successfulRequests)) / float64(totalReqs)
	}

	// Health score based on success rate and circuit state
	healthScore := successRate
	if totalReqs == 0 {
		// No requests yet, consider it healthy if closed
		if CircuitState(atomic.LoadInt32(&cb.state)) == StateClosed {
			healthScore = 1.0
		} else {
			healthScore = 0.0
		}
	} else {
		if CircuitState(atomic.LoadInt32(&cb.state)) == StateOpen {
			healthScore *= 0.1 // Heavily penalize open circuit
		} else if CircuitState(atomic.LoadInt32(&cb.state)) == StateHalfOpen {
			healthScore *= 0.5 // Moderately penalize half-open circuit
		}
	}

	return CircuitBreakerMetrics{
		State:                CircuitState(atomic.LoadInt32(&cb.state)),
		TotalRequests:        totalReqs,
		SuccessfulRequests:   atomic.LoadInt64(&cb.successfulRequests),
		FailedRequests:       atomic.LoadInt64(&cb.failedRequests),
		RejectedRequests:     atomic.LoadInt64(&cb.rejectedRequests),
		ConsecutiveFailures:  atomic.LoadInt32(&cb.consecutiveFailures),
		ConsecutiveSuccesses: atomic.LoadInt32(&cb.consecutiveSuccesses),
		LastFailureTime:      time.Unix(atomic.LoadInt64(&cb.lastFailureTime), 0),
		LastSuccessTime:      time.Unix(atomic.LoadInt64(&cb.lastSuccessTime), 0),
		StateChangedAt:       time.Unix(atomic.LoadInt64(&cb.stateChangedAt), 0),
		AverageResponseTime:  avgResponseTime,
		RateLimitHits:        atomic.LoadInt64(&cb.rateLimitHits),
		HealthScore:          healthScore,
	}
}

// IsHealthy returns true if the circuit breaker is in a healthy state
func (cb *CircuitBreaker) IsHealthy() bool {
	metrics := cb.GetMetrics()
	return metrics.State != StateOpen && metrics.HealthScore > 0.8
}

// GetState returns the current circuit breaker state
func (cb *CircuitBreaker) GetState() CircuitState {
	return CircuitState(atomic.LoadInt32(&cb.state))
}

// Reset resets the circuit breaker to closed state (for testing/admin purposes)
func (cb *CircuitBreaker) Reset() {
	atomic.StoreInt32(&cb.state, int32(StateClosed))
	atomic.StoreInt32(&cb.consecutiveFailures, 0)
	atomic.StoreInt32(&cb.consecutiveSuccesses, 0)
	atomic.StoreInt64(&cb.stateChangedAt, time.Now().Unix())

	// Reset failure counters for testing
	atomic.StoreInt64(&cb.failedRequests, 0)
	atomic.StoreInt64(&cb.successfulRequests, 0)
	atomic.StoreInt64(&cb.totalRequests, 0)
	atomic.StoreInt64(&cb.totalResponseTime, 0)

	cb.logger.WithField("circuit_breaker", cb.name).Info("Circuit breaker reset to closed state")
}

// Stop stops the circuit breaker monitoring
func (cb *CircuitBreaker) Stop() {
	close(cb.stopChannel)
	if cb.metricsTimer != nil {
		cb.metricsTimer.Stop()
	}
	if cb.healthTimer != nil {
		cb.healthTimer.Stop()
	}
	cb.wg.Wait()

	cb.logger.WithField("circuit_breaker", cb.name).Info("Circuit breaker stopped")
}

// startMonitoring starts background monitoring goroutines
func (cb *CircuitBreaker) startMonitoring() {
	// Metrics logging
	if cb.config.MetricsInterval > 0 {
		cb.wg.Add(1)
		go func() {
			defer cb.wg.Done()
			ticker := time.NewTicker(cb.config.MetricsInterval)
			defer ticker.Stop()

			for {
				select {
				case <-cb.stopChannel:
					return
				case <-ticker.C:
					cb.logMetrics()
				}
			}
		}()
	}

	// Health check (optional feature for future enhancement)
	if cb.config.HealthCheckInterval > 0 && cb.config.HealthCheckPath != "" {
		cb.wg.Add(1)
		go func() {
			defer cb.wg.Done()
			ticker := time.NewTicker(cb.config.HealthCheckInterval)
			defer ticker.Stop()

			for {
				select {
				case <-cb.stopChannel:
					return
				case <-ticker.C:
					cb.performHealthCheck()
				}
			}
		}()
	}
}

// logMetrics logs current metrics
func (cb *CircuitBreaker) logMetrics() {
	metrics := cb.GetMetrics()

	cb.logger.WithFields(logrus.Fields{
		"circuit_breaker":      cb.name,
		"state":                metrics.State.String(),
		"total_requests":       metrics.TotalRequests,
		"successful_requests":  metrics.SuccessfulRequests,
		"failed_requests":      metrics.FailedRequests,
		"rejected_requests":    metrics.RejectedRequests,
		"rate_limit_hits":      metrics.RateLimitHits,
		"consecutive_failures": metrics.ConsecutiveFailures,
		"avg_response_time_ms": metrics.AverageResponseTime.Milliseconds(),
		"health_score":         metrics.HealthScore,
	}).Info("Circuit breaker metrics")
}

// performHealthCheck performs a health check (placeholder for future implementation)
func (cb *CircuitBreaker) performHealthCheck() {
	// This would implement actual health checking logic
	// For now, it's a placeholder for future enhancement
	cb.logger.WithField("circuit_breaker", cb.name).Debug("Health check performed")
}
