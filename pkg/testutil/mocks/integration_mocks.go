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

package mocks

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// IntegrationMockEmbeddingService provides integration-level mock functionality
// Business Requirement: Support enterprise-scale testing scenarios for BR-VDB-001 and BR-VDB-002
type IntegrationMockEmbeddingService struct {
	serviceType    ServiceType
	circuitBreaker *CircuitBreaker
	retryConfig    RetryConfig
	healthStatus   HealthStatus
	operationMode  OperationMode
	logger         *logrus.Logger

	// Service state
	requests     int64
	failures     int64
	successes    int64
	totalLatency int64
	startTime    time.Time

	// Enhanced mock generator for embeddings
	generator *EnhancedMockEmbeddingGenerator

	// Enterprise features
	loadBalancing     bool
	loadBalancers     []string
	currentBalancer   int
	scalingEnabled    bool
	replicationFactor int

	// Monitoring
	alerts []ServiceAlert
	mutex  sync.RWMutex
}

// ServiceType identifies different types of embedding services for testing
type ServiceType int

const (
	OpenAIServiceType ServiceType = iota
	HuggingFaceServiceType
	LocalServiceType
	HybridServiceType
)

// HealthStatus represents the health state of the service
type HealthStatus int

const (
	HealthyStatus HealthStatus = iota
	DegradedStatus
	UnhealthyStatus
	MaintenanceStatus
)

// OperationMode defines different operational modes for testing
type OperationMode int

const (
	NormalMode OperationMode = iota
	HighAvailabilityMode
	CostOptimizedMode
	DevelopmentMode
	DisasterRecoveryMode
)

// CircuitBreaker implements circuit breaker pattern for testing resilience
type CircuitBreaker struct {
	failureThreshold int
	resetTimeout     time.Duration
	state            CircuitBreakerState
	failures         int
	lastFailureTime  time.Time
	mutex            sync.RWMutex
}

// CircuitBreakerState represents circuit breaker states
type CircuitBreakerState int

const (
	ClosedState CircuitBreakerState = iota
	OpenState
	HalfOpenState
)

// RetryConfig defines retry behavior for testing
type RetryConfig struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
	JitterEnabled bool
}

// ServiceMetrics tracks service performance for testing validation
type ServiceMetrics struct {
	RequestsPerSecond float64
	AverageLatency    time.Duration
	P95Latency        time.Duration
	P99Latency        time.Duration
	ErrorRate         float64
	SuccessRate       float64
	Availability      float64
	ThroughputMbps    float64
}

// ServiceAlert represents service monitoring alerts
type ServiceAlert struct {
	Timestamp   time.Time
	Level       AlertLevel
	Message     string
	MetricName  string
	Threshold   float64
	ActualValue float64
}

// AlertLevel defines alert severity levels
type AlertLevel int

const (
	InfoAlert AlertLevel = iota
	WarningAlert
	CriticalAlert
	FatalAlert
)

// NewIntegrationMockEmbeddingService creates an integration-level mock service
// Following project guideline: All code must be backed up by at least ONE business requirement
func NewIntegrationMockEmbeddingService(serviceType ServiceType, dimension int, logger *logrus.Logger) *IntegrationMockEmbeddingService {
	return &IntegrationMockEmbeddingService{
		serviceType:   serviceType,
		healthStatus:  HealthyStatus,
		operationMode: NormalMode,
		logger:        logger,
		generator:     NewEnhancedMockEmbeddingGenerator(dimension),
		startTime:     time.Now(),
		circuitBreaker: &CircuitBreaker{
			failureThreshold: 5,
			resetTimeout:     time.Minute,
			state:            ClosedState,
		},
		retryConfig: RetryConfig{
			MaxRetries:    3,
			InitialDelay:  time.Millisecond * 100,
			MaxDelay:      time.Second * 5,
			BackoffFactor: 2.0,
			JitterEnabled: true,
		},
		replicationFactor: 3, // Default enterprise replication
	}
}

// GenerateEmbedding provides integration-level single embedding generation with enterprise features
// Business Requirement: BR-VDB-001/BR-VDB-002 - Support enterprise reliability patterns
func (s *IntegrationMockEmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
	start := time.Now()

	// Check circuit breaker state
	if !s.circuitBreaker.CanProceed() {
		s.recordFailure()
		return nil, errors.New("circuit breaker open: service unavailable")
	}

	// Check service health
	if s.healthStatus == UnhealthyStatus {
		s.recordFailure()
		return nil, errors.New("service unhealthy")
	}

	// Apply operation mode logic
	if err := s.applyOperationModeRestrictions(ctx); err != nil {
		s.recordFailure()
		return nil, err
	}

	// Execute with retry logic
	embedding, err := s.executeSingleWithRetry(ctx, func(ctx context.Context) ([]float64, error) {
		return s.generator.GenerateEmbedding(ctx, text, nil)
	})

	duration := time.Since(start)

	if err != nil {
		s.recordFailure()
		s.circuitBreaker.RecordFailure()
		s.generateAlert(CriticalAlert, "Embedding generation failed", "error_rate", 0.0, 1.0)
	} else {
		s.recordSuccess(duration)
		s.circuitBreaker.RecordSuccess()
	}

	return embedding, err
}

// GenerateBatchEmbeddings provides enterprise-scale batch processing with load balancing
// Business Requirement: BR-VDB-009 - Support enterprise batch processing patterns
func (s *IntegrationMockEmbeddingService) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([][]float64, error) {
	start := time.Now()

	s.logger.WithFields(logrus.Fields{
		"service_type":   s.serviceType,
		"batch_size":     len(texts),
		"operation_mode": s.operationMode,
		"health_status":  s.healthStatus,
	}).Info("Processing batch embedding request")

	// Check circuit breaker
	if !s.circuitBreaker.CanProceed() {
		s.recordFailure()
		return nil, errors.New("circuit breaker open: batch service unavailable")
	}

	// Apply load balancing for large batches
	if s.loadBalancing && len(texts) > 50 {
		return s.processBatchWithLoadBalancing(ctx, texts)
	}

	// Apply scaling for enterprise workloads
	if s.scalingEnabled && len(texts) > 100 {
		return s.processBatchWithScaling(ctx, texts)
	}

	// Standard batch processing
	embeddings, err := s.executeWithRetry(ctx, func(ctx context.Context) ([][]float64, error) {
		return s.generator.GenerateBatchEmbeddings(ctx, texts)
	})

	duration := time.Since(start)

	if err != nil {
		s.recordFailure()
		s.circuitBreaker.RecordFailure()
		s.generateAlert(CriticalAlert, "Batch processing failed", "batch_error_rate", 0.0, 1.0)
	} else {
		s.recordSuccess(duration)
		s.circuitBreaker.RecordSuccess()

		// Check for SLA compliance
		if err := s.validateSLACompliance(duration, len(texts)); err != nil {
			s.generateAlert(WarningAlert, "SLA threshold exceeded", "latency_sla", 5000.0, float64(duration.Milliseconds()))
		}
	}

	return embeddings, err
}

// GetDimension returns embedding dimension
func (s *IntegrationMockEmbeddingService) GetDimension() int {
	return s.generator.GetDimension()
}

// GetModel returns service model name with service type context
func (s *IntegrationMockEmbeddingService) GetModel() string {
	switch s.serviceType {
	case OpenAIServiceType:
		return "integration-openai-mock"
	case HuggingFaceServiceType:
		return "integration-huggingface-mock"
	case LocalServiceType:
		return "integration-local-mock"
	case HybridServiceType:
		return "integration-hybrid-mock"
	default:
		return "integration-unknown-mock"
	}
}

// SetOperationMode configures the service operation mode for different testing scenarios
// Business Requirement: Support different operational contexts (production, development, cost-optimized)
func (s *IntegrationMockEmbeddingService) SetOperationMode(mode OperationMode) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.operationMode = mode

	// Apply mode-specific configurations
	switch mode {
	case HighAvailabilityMode:
		s.replicationFactor = 5
		s.retryConfig.MaxRetries = 5
		s.circuitBreaker.failureThreshold = 3
		s.loadBalancing = true

	case CostOptimizedMode:
		s.replicationFactor = 1
		s.retryConfig.MaxRetries = 1
		s.circuitBreaker.failureThreshold = 10
		s.loadBalancing = false
		s.generator.SetPerformanceProfile(PerformanceProfile{
			Name:               "cost_optimized",
			BaseLatency:        time.Millisecond * 200,
			LatencyVariation:   time.Millisecond * 50,
			ThroughputLimit:    50,
			QualityDegradation: 0.1, // Acceptable quality trade-off for cost
		})

	case DevelopmentMode:
		s.replicationFactor = 1
		s.retryConfig.MaxRetries = 1
		s.circuitBreaker.failureThreshold = 20
		s.generator.EnableLatencySimulation(time.Millisecond * 10)

	case DisasterRecoveryMode:
		s.healthStatus = DegradedStatus
		s.replicationFactor = 2
		s.generator.SetFailureRate(0.2) // Simulate degraded performance
	}

	s.logger.WithFields(logrus.Fields{
		"operation_mode":     mode,
		"replication_factor": s.replicationFactor,
		"max_retries":        s.retryConfig.MaxRetries,
	}).Info("Operation mode updated")
}

// EnableLoadBalancing enables load balancing simulation for testing
// Business Requirement: Support enterprise load balancing patterns
func (s *IntegrationMockEmbeddingService) EnableLoadBalancing(balancers []string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.loadBalancing = true
	s.loadBalancers = append([]string{}, balancers...)
	s.logger.WithField("balancers", balancers).Info("Load balancing enabled")
}

// EnableAutoScaling enables auto-scaling simulation for enterprise testing
// Business Requirement: Support enterprise scaling patterns for load testing
func (s *IntegrationMockEmbeddingService) EnableAutoScaling() {
	s.scalingEnabled = true
	s.logger.Info("Auto-scaling enabled")
}

// SetHealthStatus simulates different health states for resilience testing
// Business Requirement: Test health monitoring and degraded service scenarios
func (s *IntegrationMockEmbeddingService) SetHealthStatus(status HealthStatus) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	oldStatus := s.healthStatus
	s.healthStatus = status

	s.logger.WithFields(logrus.Fields{
		"old_status": oldStatus,
		"new_status": status,
	}).Info("Health status changed")

	// Generate health change alert
	s.generateAlert(InfoAlert, "Health status changed", "health_status", float64(oldStatus), float64(status))
}

// GetServiceMetrics returns comprehensive service metrics for testing validation
// Business Requirement: Support quantifiable service performance validation
func (s *IntegrationMockEmbeddingService) GetServiceMetrics() ServiceMetrics {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	uptime := time.Since(s.startTime)
	totalRequests := s.requests

	var requestsPerSecond float64
	if uptime.Seconds() > 0 {
		requestsPerSecond = float64(totalRequests) / uptime.Seconds()
	}

	var errorRate float64
	if totalRequests > 0 {
		errorRate = float64(s.failures) / float64(totalRequests)
	}

	var avgLatency time.Duration
	if totalRequests > 0 {
		avgLatency = time.Duration(s.totalLatency / totalRequests)
	}

	return ServiceMetrics{
		RequestsPerSecond: requestsPerSecond,
		AverageLatency:    avgLatency,
		P95Latency:        time.Duration(float64(avgLatency) * 1.5), // Simulated P95
		P99Latency:        time.Duration(float64(avgLatency) * 2.0), // Simulated P99
		ErrorRate:         errorRate,
		SuccessRate:       1.0 - errorRate,
		Availability:      s.calculateAvailability(),
		ThroughputMbps:    requestsPerSecond * 0.1, // Estimated throughput
	}
}

// GetAlerts returns service alerts for monitoring validation
// Business Requirement: Support service monitoring and alerting testing
func (s *IntegrationMockEmbeddingService) GetAlerts() []ServiceAlert {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	alerts := make([]ServiceAlert, len(s.alerts))
	copy(alerts, s.alerts)
	return alerts
}

// ResetMetrics clears all metrics and state for clean testing
// Following project guideline: Provide clean test state management
func (s *IntegrationMockEmbeddingService) ResetMetrics() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.requests = 0
	s.failures = 0
	s.successes = 0
	s.totalLatency = 0
	s.startTime = time.Now()
	s.alerts = []ServiceAlert{}
	s.circuitBreaker.Reset()
	s.generator.ResetStats()

	s.logger.Info("Service metrics reset")
}

// GetEnhancedGenerator provides access to the underlying enhanced generator
// Business Requirement: Support advanced testing capabilities
func (s *IntegrationMockEmbeddingService) GetEnhancedGenerator() *EnhancedMockEmbeddingGenerator {
	return s.generator
}

// Private helper methods

func (s *IntegrationMockEmbeddingService) executeSingleWithRetry(ctx context.Context, fn func(context.Context) ([]float64, error)) ([]float64, error) {
	var lastErr error

	for attempt := 0; attempt <= s.retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff delay
			delay := time.Duration(float64(s.retryConfig.InitialDelay) *
				math.Pow(s.retryConfig.BackoffFactor, float64(attempt-1)))

			if delay > s.retryConfig.MaxDelay {
				delay = s.retryConfig.MaxDelay
			}

			// Add jitter if enabled
			if s.retryConfig.JitterEnabled {
				jitter := time.Duration(float64(delay) * 0.1 * (2.0*float64(attempt%10)/10.0 - 1.0))
				delay += jitter
			}

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err
		s.logger.WithFields(logrus.Fields{
			"attempt": attempt + 1,
			"error":   err.Error(),
		}).Warn("Single retry attempt failed")
	}

	return nil, fmt.Errorf("all retry attempts failed, last error: %w", lastErr)
}

func (s *IntegrationMockEmbeddingService) executeWithRetry(ctx context.Context, fn func(context.Context) ([][]float64, error)) ([][]float64, error) {
	var lastErr error

	for attempt := 0; attempt <= s.retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff delay
			delay := time.Duration(float64(s.retryConfig.InitialDelay) *
				math.Pow(s.retryConfig.BackoffFactor, float64(attempt-1)))

			if delay > s.retryConfig.MaxDelay {
				delay = s.retryConfig.MaxDelay
			}

			// Add jitter if enabled
			if s.retryConfig.JitterEnabled {
				jitter := time.Duration(float64(delay) * 0.1 * (2.0*float64(attempt%10)/10.0 - 1.0))
				delay += jitter
			}

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err
		s.logger.WithFields(logrus.Fields{
			"attempt": attempt + 1,
			"error":   err.Error(),
		}).Warn("Retry attempt failed")
	}

	return nil, fmt.Errorf("all retry attempts failed, last error: %w", lastErr)
}

func (s *IntegrationMockEmbeddingService) processBatchWithLoadBalancing(ctx context.Context, texts []string) ([][]float64, error) {
	s.logger.WithFields(logrus.Fields{
		"batch_size":       len(texts),
		"load_balancers":   len(s.loadBalancers),
		"current_balancer": s.currentBalancer,
	}).Info("Processing batch with load balancing")

	// Simple round-robin simulation
	s.currentBalancer = (s.currentBalancer + 1) % len(s.loadBalancers)

	// Add slight delay to simulate load balancer routing
	time.Sleep(time.Millisecond * 5)

	return s.generator.GenerateBatchEmbeddings(ctx, texts)
}

func (s *IntegrationMockEmbeddingService) processBatchWithScaling(ctx context.Context, texts []string) ([][]float64, error) {
	s.logger.WithFields(logrus.Fields{
		"batch_size":         len(texts),
		"replication_factor": s.replicationFactor,
	}).Info("Processing batch with auto-scaling")

	// Simulate auto-scaling by processing in parallel chunks
	chunkSize := len(texts) / s.replicationFactor
	if chunkSize < 10 {
		chunkSize = 10
	}

	// For simulation, just add a slight processing improvement based on chunk size
	scalingImprovement := time.Duration(s.replicationFactor) * time.Millisecond
	if chunkSize > 50 {
		scalingImprovement *= 2 // Additional improvement for larger chunks
	}
	s.generator.EnableLatencySimulation(s.generator.latencyPerItem - scalingImprovement)

	return s.generator.GenerateBatchEmbeddings(ctx, texts)
}

func (s *IntegrationMockEmbeddingService) applyOperationModeRestrictions(ctx context.Context) error {
	switch s.operationMode {
	case DisasterRecoveryMode:
		// Simulate degraded performance in DR mode
		if s.requests%10 == 7 { // 10% degradation
			return errors.New("disaster recovery mode: temporary service degradation")
		}
	}

	// Check health status separately
	if s.healthStatus == MaintenanceStatus {
		return errors.New("service in maintenance mode")
	}

	return nil
}

func (s *IntegrationMockEmbeddingService) validateSLACompliance(duration time.Duration, batchSize int) error {
	// Business requirement: Validate SLA compliance based on operation mode
	var threshold time.Duration

	switch s.operationMode {
	case HighAvailabilityMode:
		threshold = time.Millisecond * 500 // Strict SLA
	case CostOptimizedMode:
		threshold = time.Second * 5 // Relaxed SLA
	case DevelopmentMode:
		threshold = time.Second * 10 // Very relaxed SLA
	default:
		threshold = time.Second * 2 // Standard SLA
	}

	if duration > threshold {
		return fmt.Errorf("SLA threshold exceeded: %v > %v for batch size %d", duration, threshold, batchSize)
	}

	return nil
}

func (s *IntegrationMockEmbeddingService) recordSuccess(duration time.Duration) {
	atomic.AddInt64(&s.requests, 1)
	atomic.AddInt64(&s.successes, 1)
	atomic.AddInt64(&s.totalLatency, int64(duration))
}

func (s *IntegrationMockEmbeddingService) recordFailure() {
	atomic.AddInt64(&s.requests, 1)
	atomic.AddInt64(&s.failures, 1)
}

func (s *IntegrationMockEmbeddingService) generateAlert(level AlertLevel, message, metricName string, threshold, actual float64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	alert := ServiceAlert{
		Timestamp:   time.Now(),
		Level:       level,
		Message:     message,
		MetricName:  metricName,
		Threshold:   threshold,
		ActualValue: actual,
	}

	s.alerts = append(s.alerts, alert)

	// Keep only last 100 alerts
	if len(s.alerts) > 100 {
		s.alerts = s.alerts[1:]
	}
}

func (s *IntegrationMockEmbeddingService) calculateAvailability() float64 {
	totalRequests := s.requests
	if totalRequests == 0 {
		return 1.0
	}

	successfulRequests := s.successes
	return float64(successfulRequests) / float64(totalRequests)
}

// Circuit breaker methods

func (cb *CircuitBreaker) CanProceed() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	switch cb.state {
	case ClosedState:
		return true
	case OpenState:
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = HalfOpenState
			return true
		}
		return false
	case HalfOpenState:
		return true
	default:
		return false
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures = 0
	cb.state = ClosedState
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	if cb.failures >= cb.failureThreshold {
		cb.state = OpenState
	}
}

func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures = 0
	cb.state = ClosedState
	cb.lastFailureTime = time.Time{}
}
