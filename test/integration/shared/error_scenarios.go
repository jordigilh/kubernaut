//go:build integration
// +build integration

package shared

import (
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ErrorScenario defines a specific error testing scenario with comprehensive configuration
type ErrorScenario struct {
	Name             string                 `json:"name"`
	Category         ErrorCategory          `json:"category"`
	Description      string                 `json:"description"`
	TriggerCondition string                 `json:"trigger_condition"` // When to trigger the error
	Duration         time.Duration          `json:"duration"`          // How long error persists
	RecoveryTime     time.Duration          `json:"recovery_time"`     // Time to recover
	ExpectedBehavior string                 `json:"expected_behavior"` // Expected system behavior
	InjectionPoint   string                 `json:"injection_point"`   // Where to inject the error
	Cascading        []string               `json:"cascading"`         // Related cascade failures
	Severity         ErrorSeverity          `json:"severity"`
	RecoveryActions  []RecoveryAction       `json:"recovery_actions"`
	Tags             []string               `json:"tags"`
	Prerequisites    []string               `json:"prerequisites"`
	Metrics          map[string]interface{} `json:"metrics"` // Expected metric changes
}

// ErrorScenarioConfig controls scenario execution behavior
type ErrorScenarioConfig struct {
	MaxConcurrentScenarios int           `json:"max_concurrent"`
	GlobalTimeout          time.Duration `json:"global_timeout"`
	RetryAttempts          int           `json:"retry_attempts"`
	LogLevel               string        `json:"log_level"`
	EnableMetrics          bool          `json:"enable_metrics"`
	EnableCascading        bool          `json:"enable_cascading"`
}

// PredefinedErrorScenarios contains comprehensive realistic error scenarios
var PredefinedErrorScenarios = map[string]ErrorScenario{
	// Network-related scenarios
	"transient_network_failure": {
		Name:             "Transient Network Failure",
		Category:         NetworkError,
		Description:      "Simulates temporary network connectivity loss",
		TriggerCondition: "after_5_successful_requests",
		Duration:         30 * time.Second,
		RecoveryTime:     5 * time.Second,
		ExpectedBehavior: "retry_with_exponential_backoff",
		InjectionPoint:   "slm_request",
		Cascading:        []string{"timeout_cascade", "circuit_breaker_activation"},
		Severity:         SeverityMedium,
		RecoveryActions:  []RecoveryAction{RetryWithBackoff, FallbackToCache},
		Tags:             []string{"network", "transient", "recoverable"},
		Prerequisites:    []string{"network_connectivity"},
		Metrics: map[string]interface{}{
			"expected_retry_count":   3,
			"expected_recovery_time": "5s",
			"success_rate_after":     0.95,
		},
	},

	"dns_resolution_failure": {
		Name:             "DNS Resolution Failure",
		Category:         NetworkError,
		Description:      "Simulates DNS resolution failures for external services",
		TriggerCondition: "random_10_percent",
		Duration:         15 * time.Second,
		RecoveryTime:     3 * time.Second,
		ExpectedBehavior: "retry_with_backoff_then_fallback",
		InjectionPoint:   "dns_lookup",
		Cascading:        []string{"service_discovery_failure"},
		Severity:         SeverityMedium,
		RecoveryActions:  []RecoveryAction{RetryWithBackoff, FallbackToCache},
		Tags:             []string{"network", "dns", "infrastructure"},
		Prerequisites:    []string{"external_dns_dependency"},
		Metrics: map[string]interface{}{
			"dns_failure_rate": 0.1,
			"fallback_usage":   "increased",
		},
	},

	// Database-related scenarios
	"database_connection_loss": {
		Name:             "Database Connection Loss",
		Category:         DatabaseError,
		Description:      "Simulates complete loss of database connectivity",
		TriggerCondition: "random_5_percent",
		Duration:         2 * time.Minute,
		RecoveryTime:     10 * time.Second,
		ExpectedBehavior: "graceful_degradation_with_cache_fallback",
		InjectionPoint:   "database_connection",
		Cascading:        []string{"readonly_mode_activation", "cache_pressure_increase"},
		Severity:         SeverityCritical,
		RecoveryActions:  []RecoveryAction{RetryWithBackoff, FallbackToCache, GracefulDegradation},
		Tags:             []string{"database", "critical", "persistence"},
		Prerequisites:    []string{"database_connectivity"},
		Metrics: map[string]interface{}{
			"cache_hit_rate_increase": 0.8,
			"readonly_operations":     "100%",
			"error_rate":              0.05,
		},
	},

	"database_transaction_deadlock": {
		Name:             "Database Transaction Deadlock",
		Category:         DatabaseError,
		Description:      "Simulates database deadlocks during concurrent operations",
		TriggerCondition: "concurrent_write_operations",
		Duration:         5 * time.Second,
		RecoveryTime:     1 * time.Second,
		ExpectedBehavior: "automatic_retry_with_jitter",
		InjectionPoint:   "transaction_commit",
		Cascading:        []string{"transaction_queue_buildup"},
		Severity:         SeverityMedium,
		RecoveryActions:  []RecoveryAction{RetryWithBackoff},
		Tags:             []string{"database", "concurrency", "transient"},
		Prerequisites:    []string{"concurrent_access"},
		Metrics: map[string]interface{}{
			"deadlock_retry_success_rate":  0.9,
			"transaction_latency_increase": "20%",
		},
	},

	// Kubernetes API scenarios
	"k8s_api_rate_limit": {
		Name:             "Kubernetes API Rate Limiting",
		Category:         RateLimitError,
		Description:      "Simulates Kubernetes API server rate limiting",
		TriggerCondition: "burst_requests_exceed_limit",
		Duration:         1 * time.Minute,
		RecoveryTime:     15 * time.Second,
		ExpectedBehavior: "exponential_backoff_with_jitter",
		InjectionPoint:   "k8s_api_call",
		Cascading:        []string{"operation_queuing", "delayed_reconciliation"},
		Severity:         SeverityMedium,
		RecoveryActions:  []RecoveryAction{RetryWithBackoff},
		Tags:             []string{"kubernetes", "rate_limit", "api"},
		Prerequisites:    []string{"k8s_api_access"},
		Metrics: map[string]interface{}{
			"request_queue_depth": "increased",
			"api_latency":         "2x_normal",
			"retry_backoff_time":  "exponential",
		},
	},

	"k8s_resource_conflict": {
		Name:             "Kubernetes Resource Conflict",
		Category:         K8sAPIError,
		Description:      "Simulates resource version conflicts during updates",
		TriggerCondition: "concurrent_resource_updates",
		Duration:         10 * time.Second,
		RecoveryTime:     2 * time.Second,
		ExpectedBehavior: "refresh_and_retry",
		InjectionPoint:   "resource_update",
		Cascading:        []string{"stale_resource_cache"},
		Severity:         SeverityLow,
		RecoveryActions:  []RecoveryAction{RetryImmediate},
		Tags:             []string{"kubernetes", "conflict", "optimistic_locking"},
		Prerequisites:    []string{"resource_updates_enabled"},
		Metrics: map[string]interface{}{
			"conflict_resolution_time": "2s",
			"successful_retry_rate":    0.95,
		},
	},

	// SLM service scenarios
	"slm_service_degradation": {
		Name:             "SLM Service Degradation",
		Category:         SLMError,
		Description:      "Simulates SLM service responding slowly or partially",
		TriggerCondition: "high_load_detected",
		Duration:         45 * time.Second,
		RecoveryTime:     20 * time.Second,
		ExpectedBehavior: "fallback_to_cached_recommendations",
		InjectionPoint:   "slm_analysis_request",
		Cascading:        []string{"confidence_score_reduction", "simpler_action_selection"},
		Severity:         SeverityHigh,
		RecoveryActions:  []RecoveryAction{RetryWithBackoff, FallbackToCache, GracefulDegradation},
		Tags:             []string{"slm", "performance", "ml_service"},
		Prerequisites:    []string{"slm_service_availability"},
		Metrics: map[string]interface{}{
			"response_time_increase": "300%",
			"cache_fallback_rate":    0.7,
			"confidence_degradation": 0.1,
		},
	},

	"slm_model_unavailable": {
		Name:             "SLM Model Unavailable",
		Category:         SLMError,
		Description:      "Simulates specific ML model being unavailable",
		TriggerCondition: "model_loading_failure",
		Duration:         2 * time.Minute,
		RecoveryTime:     30 * time.Second,
		ExpectedBehavior: "fallback_to_alternative_model",
		InjectionPoint:   "model_inference",
		Cascading:        []string{"model_switching", "performance_degradation"},
		Severity:         SeverityMedium,
		RecoveryActions:  []RecoveryAction{FallbackToCache, GracefulDegradation},
		Tags:             []string{"slm", "model", "availability"},
		Prerequisites:    []string{"multiple_models_available"},
		Metrics: map[string]interface{}{
			"model_switch_time":    "5s",
			"fallback_model_usage": "100%",
			"accuracy_impact":      "minimal",
		},
	},

	// Circuit breaker scenarios
	"circuit_breaker_activation": {
		Name:             "Circuit Breaker Activation",
		Category:         CircuitBreakerError,
		Description:      "Simulates circuit breaker opening due to repeated failures",
		TriggerCondition: "failure_threshold_exceeded",
		Duration:         1 * time.Minute,
		RecoveryTime:     30 * time.Second,
		ExpectedBehavior: "fail_fast_with_fallback",
		InjectionPoint:   "service_health_check",
		Cascading:        []string{"cascading_circuit_breakers", "service_isolation"},
		Severity:         SeverityHigh,
		RecoveryActions:  []RecoveryAction{CircuitBreakerOpen, FallbackToCache},
		Tags:             []string{"circuit_breaker", "resilience", "fail_fast"},
		Prerequisites:    []string{"circuit_breaker_enabled"},
		Metrics: map[string]interface{}{
			"circuit_open_duration": "30s",
			"fallback_success_rate": 0.9,
			"recovery_attempts":     3,
		},
	},

	// Complex cascade scenarios
	"memory_pressure_cascade": {
		Name:             "Memory Pressure Cascade Failure",
		Category:         ResourceError,
		Description:      "Simulates memory pressure leading to cascading failures",
		TriggerCondition: "memory_usage_above_85_percent",
		Duration:         3 * time.Minute,
		RecoveryTime:     45 * time.Second,
		ExpectedBehavior: "progressive_graceful_degradation",
		InjectionPoint:   "memory_allocation",
		Cascading:        []string{"oom_kills", "pod_evictions", "service_degradation"},
		Severity:         SeverityCritical,
		RecoveryActions:  []RecoveryAction{GracefulDegradation, EscalateToOperator},
		Tags:             []string{"resource", "memory", "cascade", "critical"},
		Prerequisites:    []string{"resource_monitoring"},
		Metrics: map[string]interface{}{
			"memory_usage_peak":       "95%",
			"pod_restart_count":       "increased",
			"degradation_timeline":    "progressive",
			"recovery_priority_order": []string{"critical", "high", "medium"},
		},
	},

	"multi_service_cascade": {
		Name:             "Multi-Service Cascade Failure",
		Category:         TransientError,
		Description:      "Simulates cascade failure across multiple services",
		TriggerCondition: "primary_service_failure",
		Duration:         5 * time.Minute,
		RecoveryTime:     2 * time.Minute,
		ExpectedBehavior: "controlled_cascade_with_circuit_breakers",
		InjectionPoint:   "service_dependency_chain",
		Cascading:        []string{"dependent_service_failures", "timeout_propagation", "queue_buildup"},
		Severity:         SeverityCritical,
		RecoveryActions:  []RecoveryAction{CircuitBreakerOpen, GracefulDegradation, EscalateToOperator},
		Tags:             []string{"cascade", "multi_service", "dependency", "critical"},
		Prerequisites:    []string{"service_dependency_mapping"},
		Metrics: map[string]interface{}{
			"services_affected":        3,
			"cascade_propagation_time": "30s",
			"isolation_effectiveness":  0.8,
			"recovery_order":           []string{"database", "api", "frontend"},
		},
	},
}

// ErrorScenarioManager manages execution of error scenarios
type ErrorScenarioManager struct {
	config           ErrorScenarioConfig
	activeScenarios  map[string]*ErrorScenarioExecution
	logger           *logrus.Logger
	metrics          *ErrorMetrics
	injectionTargets map[string]ErrorInjector
}

// ErrorScenarioExecution tracks an active error scenario
type ErrorScenarioExecution struct {
	Scenario         ErrorScenario
	StartTime        time.Time
	EndTime          *time.Time
	Status           ScenarioStatus
	ErrorsInjected   int
	RecoveryAttempts int
	CascadeTriggered []string
	Metrics          map[string]interface{}
}

// ScenarioStatus represents the current status of a scenario
type ScenarioStatus string

const (
	ScenarioStatusPending    ScenarioStatus = "pending"
	ScenarioStatusActive     ScenarioStatus = "active"
	ScenarioStatusRecovering ScenarioStatus = "recovering"
	ScenarioStatusCompleted  ScenarioStatus = "completed"
	ScenarioStatusFailed     ScenarioStatus = "failed"
	ScenarioStatusCanceled   ScenarioStatus = "canceled"
)

// ErrorInjector interface for components that can inject errors
type ErrorInjector interface {
	InjectError(scenario ErrorScenario) error
	RecoverFromError(scenario ErrorScenario) error
	GetInjectionCapabilities() []string
}

// NewErrorScenarioManager creates a new error scenario manager
func NewErrorScenarioManager(logger *logrus.Logger, config ErrorScenarioConfig) *ErrorScenarioManager {
	return &ErrorScenarioManager{
		config:           config,
		activeScenarios:  make(map[string]*ErrorScenarioExecution),
		logger:           logger,
		metrics:          NewErrorMetrics(),
		injectionTargets: make(map[string]ErrorInjector),
	}
}

// RegisterInjectionTarget registers a component that can inject errors
func (esm *ErrorScenarioManager) RegisterInjectionTarget(name string, injector ErrorInjector) {
	esm.injectionTargets[name] = injector
	esm.logger.WithFields(logrus.Fields{
		"target":       name,
		"capabilities": injector.GetInjectionCapabilities(),
	}).Info("Registered error injection target")
}

// ExecuteScenario executes a specific error scenario
func (esm *ErrorScenarioManager) ExecuteScenario(scenarioName string) (*ErrorScenarioExecution, error) {
	scenario, exists := PredefinedErrorScenarios[scenarioName]
	if !exists {
		return nil, fmt.Errorf("scenario '%s' not found", scenarioName)
	}

	// Check if max concurrent scenarios would be exceeded
	if len(esm.activeScenarios) >= esm.config.MaxConcurrentScenarios {
		return nil, fmt.Errorf("maximum concurrent scenarios (%d) exceeded", esm.config.MaxConcurrentScenarios)
	}

	execution := &ErrorScenarioExecution{
		Scenario:  scenario,
		StartTime: time.Now(),
		Status:    ScenarioStatusPending,
		Metrics:   make(map[string]interface{}),
	}

	esm.activeScenarios[scenarioName] = execution

	esm.logger.WithFields(logrus.Fields{
		"scenario":  scenarioName,
		"duration":  scenario.Duration,
		"injection": scenario.InjectionPoint,
		"severity":  scenario.Severity,
	}).Info("Starting error scenario execution")

	go esm.runScenario(execution)

	return execution, nil
}

// runScenario executes the scenario in a goroutine
func (esm *ErrorScenarioManager) runScenario(execution *ErrorScenarioExecution) {
	defer func() {
		if r := recover(); r != nil {
			execution.Status = ScenarioStatusFailed
			esm.logger.WithFields(logrus.Fields{
				"scenario": execution.Scenario.Name,
				"panic":    r,
			}).Error("Scenario execution panicked")
		}
		delete(esm.activeScenarios, execution.Scenario.Name)
	}()

	scenario := execution.Scenario

	// Check prerequisites
	if err := esm.checkPrerequisites(scenario); err != nil {
		execution.Status = ScenarioStatusFailed
		esm.logger.WithError(err).Error("Scenario prerequisites not met")
		return
	}

	// Find appropriate injection target
	injector, err := esm.findInjectionTarget(scenario.InjectionPoint)
	if err != nil {
		execution.Status = ScenarioStatusFailed
		esm.logger.WithError(err).Error("No suitable injection target found")
		return
	}

	execution.Status = ScenarioStatusActive

	// Inject the error
	if err := injector.InjectError(scenario); err != nil {
		execution.Status = ScenarioStatusFailed
		esm.logger.WithError(err).Error("Failed to inject error")
		return
	}

	execution.ErrorsInjected++
	esm.logger.WithField("scenario", scenario.Name).Info("Error injection successful")

	// Wait for scenario duration
	time.Sleep(scenario.Duration)

	// Trigger cascading failures if enabled
	if esm.config.EnableCascading && len(scenario.Cascading) > 0 {
		esm.triggerCascadingFailures(execution)
	}

	// Begin recovery phase
	execution.Status = ScenarioStatusRecovering
	esm.logger.WithField("scenario", scenario.Name).Info("Beginning scenario recovery")

	if err := injector.RecoverFromError(scenario); err != nil {
		esm.logger.WithError(err).Warning("Error recovery failed, waiting for natural recovery")
	}

	// Wait for recovery time
	time.Sleep(scenario.RecoveryTime)

	// Mark completion
	now := time.Now()
	execution.EndTime = &now
	execution.Status = ScenarioStatusCompleted

	totalDuration := execution.EndTime.Sub(execution.StartTime)
	esm.logger.WithFields(logrus.Fields{
		"scenario":          scenario.Name,
		"total_duration":    totalDuration,
		"errors_injected":   execution.ErrorsInjected,
		"cascade_triggered": len(execution.CascadeTriggered),
	}).Info("Scenario execution completed")

	// Record metrics
	if esm.config.EnableMetrics {
		esm.recordScenarioMetrics(execution)
	}
}

// checkPrerequisites verifies scenario prerequisites are met
func (esm *ErrorScenarioManager) checkPrerequisites(scenario ErrorScenario) error {
	for _, prereq := range scenario.Prerequisites {
		// This is a placeholder - in real implementation you'd check actual prerequisites
		esm.logger.WithFields(logrus.Fields{
			"prerequisite": prereq,
			"scenario":     scenario.Name,
		}).Debug("Checking prerequisite")
	}
	return nil
}

// findInjectionTarget finds appropriate injector for injection point
func (esm *ErrorScenarioManager) findInjectionTarget(injectionPoint string) (ErrorInjector, error) {
	for name, injector := range esm.injectionTargets {
		capabilities := injector.GetInjectionCapabilities()
		for _, capability := range capabilities {
			if strings.Contains(injectionPoint, capability) || capability == "all" {
				esm.logger.WithFields(logrus.Fields{
					"target":     name,
					"capability": capability,
					"injection":  injectionPoint,
				}).Debug("Found matching injection target")
				return injector, nil
			}
		}
	}
	return nil, fmt.Errorf("no injection target found for point: %s", injectionPoint)
}

// triggerCascadingFailures triggers related cascade failures
func (esm *ErrorScenarioManager) triggerCascadingFailures(execution *ErrorScenarioExecution) {
	for _, cascadeName := range execution.Scenario.Cascading {
		if cascadeScenario, exists := PredefinedErrorScenarios[cascadeName]; exists {
			esm.logger.WithFields(logrus.Fields{
				"parent":  execution.Scenario.Name,
				"cascade": cascadeName,
			}).Info("Triggering cascading failure")

			// Execute cascade with reduced duration
			cascadeScenario.Duration = cascadeScenario.Duration / 2
			go esm.executeCascadeScenario(cascadeScenario)

			execution.CascadeTriggered = append(execution.CascadeTriggered, cascadeName)
		}
	}
}

// executeCascadeScenario executes a cascading scenario
func (esm *ErrorScenarioManager) executeCascadeScenario(scenario ErrorScenario) {
	// Simplified cascade execution - would use similar logic to runScenario
	esm.logger.WithField("cascade", scenario.Name).Info("Executing cascade scenario")
}

// recordScenarioMetrics records metrics for completed scenario
func (esm *ErrorScenarioManager) recordScenarioMetrics(execution *ErrorScenarioExecution) {
	// Record scenario completion in metrics
	esm.metrics.RecordScenarioExecution(execution)
}

// GetActiveScenarios returns currently active scenarios
func (esm *ErrorScenarioManager) GetActiveScenarios() map[string]*ErrorScenarioExecution {
	return esm.activeScenarios
}

// StopScenario stops an active scenario
func (esm *ErrorScenarioManager) StopScenario(scenarioName string) error {
	if execution, exists := esm.activeScenarios[scenarioName]; exists {
		execution.Status = ScenarioStatusCanceled
		now := time.Now()
		execution.EndTime = &now

		esm.logger.WithField("scenario", scenarioName).Info("Scenario stopped")
		delete(esm.activeScenarios, scenarioName)
		return nil
	}
	return fmt.Errorf("scenario '%s' not found or not active", scenarioName)
}

// StopAllScenarios stops all active scenarios
func (esm *ErrorScenarioManager) StopAllScenarios() {
	for scenarioName := range esm.activeScenarios {
		esm.StopScenario(scenarioName)
	}
}

// GetScenarioMetrics returns metrics for a specific scenario type
func (esm *ErrorScenarioManager) GetScenarioMetrics() *ErrorMetrics {
	return esm.metrics
}

// Utility functions for scenario creation and customization

// CreateCustomScenario creates a custom error scenario
func CreateCustomScenario(name, description string, category ErrorCategory) ErrorScenario {
	return ErrorScenario{
		Name:             name,
		Category:         category,
		Description:      description,
		Duration:         30 * time.Second,
		RecoveryTime:     10 * time.Second,
		ExpectedBehavior: "retry_with_backoff",
		Severity:         SeverityMedium,
		RecoveryActions:  []RecoveryAction{RetryWithBackoff},
		Tags:             []string{"custom"},
		Prerequisites:    []string{},
		Metrics:          make(map[string]interface{}),
	}
}

// GetScenariosByTag returns scenarios matching specific tags
func GetScenariosByTag(tag string) map[string]ErrorScenario {
	matching := make(map[string]ErrorScenario)
	for name, scenario := range PredefinedErrorScenarios {
		for _, scenarioTag := range scenario.Tags {
			if scenarioTag == tag {
				matching[name] = scenario
				break
			}
		}
	}
	return matching
}

// GetScenariosByCategory returns scenarios of specific category
func GetScenariosByCategory(category ErrorCategory) map[string]ErrorScenario {
	matching := make(map[string]ErrorScenario)
	for name, scenario := range PredefinedErrorScenarios {
		if scenario.Category == category {
			matching[name] = scenario
		}
	}
	return matching
}

// GetScenariosBySeverity returns scenarios of specific severity
func GetScenariosBySeverity(severity ErrorSeverity) map[string]ErrorScenario {
	matching := make(map[string]ErrorScenario)
	for name, scenario := range PredefinedErrorScenarios {
		if scenario.Severity == severity {
			matching[name] = scenario
		}
	}
	return matching
}
