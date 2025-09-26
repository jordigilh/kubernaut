package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// LLMHealthMonitor implements HealthMonitor interface for LLM health monitoring
// BR-HEALTH-001: MUST implement comprehensive health checks for all components
// BR-HEALTH-002: MUST provide liveness and readiness probes for Kubernetes
// BR-HEALTH-003: MUST monitor external dependency health and availability
// BR-HEALTH-016: MUST track system availability and uptime metrics
// BR-REL-011: MUST maintain monitoring accuracy >99% for critical metrics
type LLMHealthMonitor struct {
	llmClient     llm.Client
	logger        *logrus.Logger
	monitoringCtx context.Context
	cancelFunc    context.CancelFunc
	isMonitoring  bool
	mu            sync.RWMutex

	// Health tracking state - using structured types following guidelines
	healthStatus types.HealthStatus
	dependencies map[string]*types.DependencyStatus
	probeResults map[string]*types.ProbeResult

	// Configuration from local-llm.yaml heartbeat section
	checkInterval    time.Duration
	failureThreshold int
	healthyThreshold int
	timeout          time.Duration
	healthPrompt     string

	// Metrics tracking
	consecutiveFailures int
	consecutivePasses   int
	startTime           time.Time

	// Enhanced metrics following BR-METRICS-020 through BR-METRICS-039
	enhancedMetrics *metrics.EnhancedHealthMetrics
}

// NewLLMHealthMonitor creates a new LLM health monitor following business requirements
// Integrates with existing monitoring infrastructure and uses shared types
func NewLLMHealthMonitor(llmClient llm.Client, logger *logrus.Logger) *LLMHealthMonitor {
	return NewLLMHealthMonitorWithMetrics(llmClient, logger, nil)
}

// NewLLMHealthMonitorWithMetrics creates a new LLM health monitor with optional enhanced metrics
// This allows for custom registries in testing scenarios
func NewLLMHealthMonitorWithMetrics(llmClient llm.Client, logger *logrus.Logger, enhancedMetrics *metrics.EnhancedHealthMetrics) *LLMHealthMonitor {
	now := time.Now()

	monitor := &LLMHealthMonitor{
		llmClient:        llmClient,
		logger:           logger,
		dependencies:     make(map[string]*types.DependencyStatus),
		probeResults:     make(map[string]*types.ProbeResult),
		checkInterval:    30 * time.Second,                             // From config/local-llm.yaml heartbeat.check_interval
		failureThreshold: 3,                                            // From config/local-llm.yaml heartbeat.failure_threshold
		healthyThreshold: 2,                                            // From config/local-llm.yaml heartbeat.healthy_threshold
		timeout:          10 * time.Second,                             // From config/local-llm.yaml heartbeat.timeout
		healthPrompt:     "System health check. Respond with: HEALTHY", // From config
		startTime:        now,

		// Enhanced metrics will be set below

		// Initialize health status using shared types and BaseEntity
		healthStatus: types.HealthStatus{
			BaseEntity: types.BaseEntity{
				ID:          "llm-health-monitor",
				Name:        "LLM Health Monitor",
				Description: "20B+ Model Health Monitoring",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			BaseTimestampedResult: types.BaseTimestampedResult{
				Success:   true,
				StartTime: now,
				EndTime:   now,
				Duration:  0,
			},
			IsHealthy:       true,
			ComponentType:   "llm-20b",
			ServiceEndpoint: llmClient.GetEndpoint(), // Set from LLM client config following BR-HEALTH-001
			ResponseTime:    0,
			HealthMetrics: types.HealthMetrics{
				UptimePercentage: 100.0,
				TotalUptime:      0,
				AccuracyRate:     100.0,
			},
			ProbeResults: make(map[string]types.ProbeResult),
		},
	}

	// Initialize enhanced metrics - use provided instance or create new one for production
	if enhancedMetrics != nil {
		monitor.enhancedMetrics = enhancedMetrics
	} else {
		// For production use, create enhanced metrics with default registry
		// In testing, pass a custom registry to avoid conflicts
		monitor.enhancedMetrics = metrics.NewEnhancedHealthMetrics(prometheus.DefaultRegisterer.(*prometheus.Registry))
	}

	// BR-METRICS-036: Record initial model parameter count
	if llmClient.GetMinParameterCount() > 0 {
		monitor.enhancedMetrics.RecordModelParameterCount(llmClient.GetModel(), float64(llmClient.GetMinParameterCount()))
	}

	return monitor
}

// GetHealthStatus returns comprehensive health status for the LLM component
// BR-HEALTH-001: MUST implement comprehensive health checks for all components
func (m *LLMHealthMonitor) GetHealthStatus(ctx context.Context) (*types.HealthStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update timestamp using BaseEntity pattern
	now := time.Now()
	m.healthStatus.UpdatedAt = now
	m.healthStatus.EndTime = now

	// Perform comprehensive health checks to capture detailed error information
	// BR-HEALTH-001: MUST implement comprehensive health checks for all components
	// Following guideline #27: Strong business validations with descriptive error messages
	isHealthy := true
	var healthCheckError error

	// Perform liveness check to capture actual error details
	livenessErr := m.llmClient.LivenessCheck(ctx)
	if livenessErr != nil {
		isHealthy = false
		healthCheckError = livenessErr
	}

	// Perform readiness check if liveness passed
	if isHealthy {
		readinessErr := m.llmClient.ReadinessCheck(ctx)
		if readinessErr != nil {
			isHealthy = false
			healthCheckError = readinessErr
		}
	}

	// Update health status with detailed error information
	if !isHealthy && healthCheckError != nil {
		m.healthStatus.IsHealthy = false
		m.healthStatus.Success = false
		// Capture the actual error details for strong business validation
		m.healthStatus.Error = healthCheckError.Error()
	} else {
		m.healthStatus.IsHealthy = true
		m.healthStatus.Success = true
		m.healthStatus.Error = ""
	}

	// Calculate uptime metrics - BR-HEALTH-016: Must track system availability and uptime metrics
	totalDuration := now.Sub(m.startTime)
	if totalDuration > 0 {
		uptime := totalDuration - m.healthStatus.HealthMetrics.TotalDowntime
		m.healthStatus.HealthMetrics.TotalUptime = uptime
		m.healthStatus.HealthMetrics.UptimePercentage = float64(uptime) / float64(totalDuration) * 100
	}

	// Record metrics for monitoring infrastructure integration
	if m.healthStatus.IsHealthy {
		metrics.RecordSLMAPICall("llm-health-check")
		// BR-METRICS-020: Record health status using enhanced metrics
		m.enhancedMetrics.RecordHealthStatus(&m.healthStatus)
		// BR-METRICS-022: Record successful health check
		m.enhancedMetrics.RecordHealthCheck(m.healthStatus.ComponentType, "success")
	} else {
		metrics.RecordSLMAPIError("llm-health-check", "health_check_failed")
		// BR-METRICS-020: Record unhealthy status using enhanced metrics
		m.enhancedMetrics.RecordHealthStatus(&m.healthStatus)
		// BR-METRICS-022: Record failed health check
		m.enhancedMetrics.RecordHealthCheck(m.healthStatus.ComponentType, "failure")
	}

	// BR-METRICS-023: Record consecutive failures
	m.enhancedMetrics.RecordConsecutiveFailures(m.healthStatus.ComponentType, m.consecutiveFailures)

	// BR-METRICS-024: Record uptime percentage
	m.enhancedMetrics.RecordUptimePercentage(m.healthStatus.ComponentType, m.healthStatus.HealthMetrics.UptimePercentage)

	// BR-METRICS-035: Record monitoring accuracy for BR-REL-011 compliance
	m.enhancedMetrics.RecordMonitoringAccuracy("llm-health-monitor", m.healthStatus.HealthMetrics.AccuracyRate)

	// Deep copy to prevent concurrent modification
	status := m.healthStatus
	return &status, nil
}

// PerformLivenessProbe checks if the LLM component is alive (Kubernetes liveness)
// BR-HEALTH-002: MUST provide liveness and readiness probes for Kubernetes
func (m *LLMHealthMonitor) PerformLivenessProbe(ctx context.Context) (*types.ProbeResult, error) {
	startTime := time.Now()

	// Perform basic connectivity check
	isHealthy := true
	var probeError error

	// Use LLM client for health check with timeout
	healthCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	_, err := m.llmClient.ChatCompletion(healthCtx, m.healthPrompt)
	if err != nil {
		isHealthy = false
		probeError = err
		m.logger.WithError(err).Warn("LLM liveness probe failed")
	}

	responseTime := time.Since(startTime)

	// Update probe tracking
	m.mu.Lock()
	if isHealthy {
		m.consecutivePasses++
		m.consecutiveFailures = 0
	} else {
		m.consecutiveFailures++
		m.consecutivePasses = 0
	}

	result := &types.ProbeResult{
		ProbeType:           "liveness",
		IsHealthy:           isHealthy,
		ComponentID:         m.healthStatus.ComponentType, // Use consistent component type
		ResponseTime:        responseTime,
		LastCheckTime:       time.Now(),
		ConsecutivePasses:   m.consecutivePasses,
		ConsecutiveFailures: m.consecutiveFailures,
	}

	// Store probe result
	m.probeResults["liveness"] = result
	m.healthStatus.ProbeResults["liveness"] = *result
	m.mu.Unlock()

	// Record probe metrics using enhanced metrics
	if isHealthy {
		metrics.RecordSLMAnalysis(responseTime)
		// BR-METRICS-025: Record liveness probe duration
		m.enhancedMetrics.RecordProbeDuration("liveness", result.ComponentID, responseTime)
	} else {
		metrics.RecordSLMAPIError("liveness-probe", "probe_failed")
		// BR-METRICS-025: Record failed liveness probe duration
		m.enhancedMetrics.RecordProbeDuration("liveness", result.ComponentID, responseTime)
	}

	if probeError != nil {
		return result, fmt.Errorf("liveness probe failed: %w", probeError)
	}

	return result, nil
}

// PerformReadinessProbe checks if the LLM component is ready to serve traffic
// BR-HEALTH-002: MUST provide liveness and readiness probes for Kubernetes
func (m *LLMHealthMonitor) PerformReadinessProbe(ctx context.Context) (*types.ProbeResult, error) {
	startTime := time.Now()

	// Perform readiness check - more comprehensive than liveness
	isReady := true
	var probeError error

	// Use LLM client for readiness check with timeout
	readinessCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	// Test with a simple prompt to verify model is responsive
	response, err := m.llmClient.ChatCompletion(readinessCtx, "Test readiness: respond with 'READY'")
	if err != nil || len(response) == 0 {
		isReady = false
		probeError = fmt.Errorf("readiness check failed: %w", err)
		m.logger.WithError(err).Warn("LLM readiness probe failed")
	}

	responseTime := time.Since(startTime)

	// Update probe tracking
	m.mu.Lock()
	result := &types.ProbeResult{
		ProbeType:           "readiness",
		IsHealthy:           isReady,
		ComponentID:         m.healthStatus.ComponentType, // Use consistent component type
		ResponseTime:        responseTime,
		LastCheckTime:       time.Now(),
		ConsecutivePasses:   m.consecutivePasses,
		ConsecutiveFailures: m.consecutiveFailures,
	}

	// Store probe result
	m.probeResults["readiness"] = result
	m.healthStatus.ProbeResults["readiness"] = *result
	m.mu.Unlock()

	// Record readiness metrics using enhanced metrics
	if isReady {
		metrics.RecordSLMAnalysis(responseTime)
		// BR-METRICS-026: Record readiness probe duration
		m.enhancedMetrics.RecordProbeDuration("readiness", result.ComponentID, responseTime)
	} else {
		metrics.RecordSLMAPIError("readiness-probe", "probe_failed")
		// BR-METRICS-026: Record failed readiness probe duration
		m.enhancedMetrics.RecordProbeDuration("readiness", result.ComponentID, responseTime)
	}

	if probeError != nil {
		return result, fmt.Errorf("readiness probe failed: %w", probeError)
	}

	return result, nil
}

// GetDependencyStatus monitors external dependency health and availability
// BR-HEALTH-003: MUST monitor external dependency health and availability
func (m *LLMHealthMonitor) GetDependencyStatus(ctx context.Context, dependencyName string) (*types.DependencyStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check if we have this dependency tracked
	if status, exists := m.dependencies[dependencyName]; exists {
		// Update timestamp
		status.UpdatedAt = time.Now()
		status.LastCheckTime = time.Now()
		return status, nil
	}

	// Create new dependency status for 20B+ LLM service
	now := time.Now()
	dependencyStatus := &types.DependencyStatus{
		BaseEntity: types.BaseEntity{
			ID:          fmt.Sprintf("dependency-%s", dependencyName),
			Name:        dependencyName,
			Description: "External 20B+ LLM Model Dependency",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		IsAvailable:    m.healthStatus.IsHealthy,
		DependencyType: "external_llm",
		Endpoint:       m.healthStatus.ServiceEndpoint,
		Criticality:    "critical",
		LastCheckTime:  now,
		HealthMetrics:  m.healthStatus.HealthMetrics,
		Configuration: map[string]string{
			"model_type":        "20b_parameter_model",
			"provider":          "enterprise",
			"check_interval":    m.checkInterval.String(),
			"failure_threshold": fmt.Sprintf("%d", m.failureThreshold),
		},
	}

	// Store dependency status
	m.dependencies[dependencyName] = dependencyStatus

	// BR-METRICS-030: Record dependency status using enhanced metrics
	m.enhancedMetrics.RecordDependencyStatus(dependencyName, dependencyStatus.Criticality, dependencyStatus.IsAvailable)

	return dependencyStatus, nil
}

// StartHealthMonitoring begins continuous health monitoring
// BR-HEALTH-016: MUST track system availability and uptime metrics
func (m *LLMHealthMonitor) StartHealthMonitoring(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isMonitoring {
		return fmt.Errorf("health monitoring is already running")
	}

	m.monitoringCtx, m.cancelFunc = context.WithCancel(ctx)
	m.isMonitoring = true

	// Start monitoring goroutine following the heartbeat configuration
	go m.monitoringLoop()

	m.logger.Info("LLM health monitoring started")
	return nil
}

// StopHealthMonitoring stops continuous health monitoring
func (m *LLMHealthMonitor) StopHealthMonitoring(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isMonitoring {
		return fmt.Errorf("health monitoring is not running")
	}

	if m.cancelFunc != nil {
		m.cancelFunc()
	}

	m.isMonitoring = false
	m.logger.Info("LLM health monitoring stopped")
	return nil
}

// monitoringLoop performs continuous health monitoring following BR requirements
func (m *LLMHealthMonitor) monitoringLoop() {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.monitoringCtx.Done():
			return
		case <-ticker.C:
			m.performHealthCheck()
		}
	}
}

// performHealthCheck executes comprehensive health check following business requirements
func (m *LLMHealthMonitor) performHealthCheck() {
	ctx, cancel := context.WithTimeout(m.monitoringCtx, m.timeout)
	defer cancel()

	startTime := time.Now()
	isHealthy := true
	var lastError error

	// Perform liveness and readiness probes
	_, livenessErr := m.PerformLivenessProbe(ctx)
	_, readinessErr := m.PerformReadinessProbe(ctx)

	if livenessErr != nil || readinessErr != nil {
		isHealthy = false
		if livenessErr != nil {
			lastError = livenessErr
		} else {
			lastError = readinessErr
		}
	}

	responseTime := time.Since(startTime)

	// BR-METRICS-021: Record health check duration using enhanced metrics
	m.enhancedMetrics.RecordHealthCheckDuration(m.healthStatus.ComponentType, responseTime)

	// Update health status using shared types
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.healthStatus.BaseEntity.UpdatedAt = now
	m.healthStatus.BaseTimestampedResult.EndTime = now
	m.healthStatus.BaseTimestampedResult.Duration = responseTime
	m.healthStatus.ResponseTime = responseTime

	if isHealthy {
		m.healthStatus.IsHealthy = true
		m.healthStatus.Success = true
		m.healthStatus.BaseTimestampedResult.Error = ""
		m.consecutivePasses++
		m.consecutiveFailures = 0

		// Update recovery time if we were previously unhealthy
		if m.healthStatus.HealthMetrics.LastFailureTime.After(m.healthStatus.HealthMetrics.LastRecoveryTime) {
			m.healthStatus.HealthMetrics.LastRecoveryTime = now
		}

	} else {
		m.consecutiveFailures++
		m.consecutivePasses = 0

		// Only mark as unhealthy after failure threshold
		if m.consecutiveFailures >= m.failureThreshold {
			m.healthStatus.IsHealthy = false
			m.healthStatus.Success = false
			if lastError != nil {
				m.healthStatus.BaseTimestampedResult.Error = lastError.Error()
			}
			m.healthStatus.HealthMetrics.FailureCount++
			m.healthStatus.HealthMetrics.LastFailureTime = now
		}
	}

	// BR-REL-011: Track monitoring accuracy >99%
	totalChecks := m.healthStatus.HealthMetrics.FailureCount + m.consecutivePasses
	if totalChecks > 0 {
		accuracyRate := float64(m.consecutivePasses) / float64(totalChecks) * 100
		m.healthStatus.HealthMetrics.AccuracyRate = accuracyRate
	}

	// Log health status changes following guidelines (always log errors)
	if !isHealthy {
		m.logger.WithError(lastError).
			WithField("consecutive_failures", m.consecutiveFailures).
			WithField("failure_threshold", m.failureThreshold).
			Error("LLM health check failed")
	}
}
