//go:build integration
// +build integration

<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package shared

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// K8sErrorType represents specific Kubernetes API error types
type K8sErrorType string

const (
	K8sResourceConflict     K8sErrorType = "resource_conflict"
	K8sPermissionDenied     K8sErrorType = "permission_denied"
	K8sQuotaExceeded        K8sErrorType = "quota_exceeded"
	K8sAPIServerUnavailable K8sErrorType = "api_server_unavailable"
	K8sResourceNotFound     K8sErrorType = "resource_not_found"
	K8sInvalidResource      K8sErrorType = "invalid_resource"
	K8sTooManyRequests      K8sErrorType = "too_many_requests"
	K8sInternalError        K8sErrorType = "internal_error"
)

// EnhancedK8sFakeClient wraps the standard fake client with realistic behaviors
type EnhancedK8sFakeClient struct {
	kubernetes.Interface
	logger            *logrus.Logger
	networkDelay      time.Duration
	errorRate         float64
	resourceTracker   map[string]ResourceState
	operationHistory  []K8sOperation
	simulateTimeouts  bool
	simulateConflicts bool

	// Error injection capabilities
	errorInjection  ErrorInjectionConfig
	activeScenarios map[string]bool
	k8sErrorTypes   []K8sErrorType
}

// ResourceState tracks the state of a Kubernetes resource
type ResourceState struct {
	Name              string
	Namespace         string
	Kind              string
	CreatedAt         time.Time
	LastModified      time.Time
	Status            string
	OperationCount    int
	PendingOperations []string
}

// K8sOperation records operations performed on the cluster
type K8sOperation struct {
	Type      string // CREATE, UPDATE, DELETE, GET, LIST
	Resource  string // pods, deployments, services, etc.
	Namespace string
	Name      string
	Timestamp time.Time
	Duration  time.Duration
	Success   bool
	ErrorType string
}

// NewEnhancedK8sFakeClient creates a new enhanced fake K8s client
func NewEnhancedK8sFakeClient(logger *logrus.Logger) *EnhancedK8sFakeClient {
	fakeClient := fake.NewSimpleClientset()

	return &EnhancedK8sFakeClient{
		Interface:         fakeClient,
		logger:            logger,
		networkDelay:      10 * time.Millisecond, // Default minimal delay
		errorRate:         0.02,                  // 2% error rate by default
		resourceTracker:   make(map[string]ResourceState),
		operationHistory:  make([]K8sOperation, 0),
		simulateTimeouts:  false,
		simulateConflicts: false,
	}
}

// SetNetworkConditions configures network simulation parameters
func (e *EnhancedK8sFakeClient) SetNetworkConditions(delay time.Duration, errorRate float64) {
	e.networkDelay = delay
	e.errorRate = errorRate
}

// EnableTimeoutSimulation turns on timeout simulation
func (e *EnhancedK8sFakeClient) EnableTimeoutSimulation(enabled bool) {
	e.simulateTimeouts = enabled
}

// EnableConflictSimulation turns on resource conflict simulation
func (e *EnhancedK8sFakeClient) EnableConflictSimulation(enabled bool) {
	e.simulateConflicts = enabled
}

// simulateNetworkEffects adds realistic network delays and errors
func (e *EnhancedK8sFakeClient) simulateNetworkEffects(operation string) error {
	// Add network delay
	if e.networkDelay > 0 {
		// Add some randomness to the delay (±50%)
		variation := time.Duration(rand.Float64() * float64(e.networkDelay))
		actualDelay := e.networkDelay + variation - time.Duration(float64(e.networkDelay)*0.5)
		time.Sleep(actualDelay)
	}

	// Simulate timeout
	if e.simulateTimeouts && rand.Float64() < 0.01 { // 1% timeout rate
		return fmt.Errorf("context deadline exceeded")
	}

	// Simulate network errors or K8s API errors
	if rand.Float64() < e.errorRate {
		// Check if we should inject a K8s-specific error
		if shouldInject, k8sErrorType := e.shouldInjectK8sError(); shouldInject {
			return e.createK8sError(k8sErrorType, schema.GroupResource{Resource: "pods"}, "simulated")
		}

		// Otherwise, simulate network errors
		errorTypes := []string{
			"connection refused",
			"no route to host",
			"network is unreachable",
			"temporary failure in name resolution",
		}
		errorType := errorTypes[rand.Intn(len(errorTypes))]
		return fmt.Errorf("network error during %s: %s", operation, errorType)
	}

	return nil
}

// simulateResourceConflict checks for resource conflicts
func (e *EnhancedK8sFakeClient) simulateResourceConflict(namespace, name string) error {
	if !e.simulateConflicts {
		return nil
	}

	resourceKey := fmt.Sprintf("%s/%s", namespace, name)
	if state, exists := e.resourceTracker[resourceKey]; exists {
		// Simulate conflict if resource was recently modified
		if time.Since(state.LastModified) < 100*time.Millisecond && rand.Float64() < 0.1 {
			return errors.NewConflict(schema.GroupResource{Resource: "pods"}, name,
				fmt.Errorf("operation cannot be fulfilled on resource %s: the object has been modified", name))
		}
	}

	return nil
}

// trackResourceOperation records an operation in the resource tracker
func (e *EnhancedK8sFakeClient) trackResourceOperation(operation, kind, namespace, name string, success bool, duration time.Duration, errorType string) {
	resourceKey := fmt.Sprintf("%s/%s", namespace, name)

	// Update resource state
	if state, exists := e.resourceTracker[resourceKey]; exists {
		state.LastModified = time.Now()
		state.OperationCount++
		if success {
			state.Status = fmt.Sprintf("%s_completed", operation)
		} else {
			state.Status = fmt.Sprintf("%s_failed", operation)
		}
		e.resourceTracker[resourceKey] = state
	} else if operation == "CREATE" {
		e.resourceTracker[resourceKey] = ResourceState{
			Name:           name,
			Namespace:      namespace,
			Kind:           kind,
			CreatedAt:      time.Now(),
			LastModified:   time.Now(),
			Status:         "created",
			OperationCount: 1,
		}
	}

	// Record in operation history
	op := K8sOperation{
		Type:      operation,
		Resource:  kind,
		Namespace: namespace,
		Name:      name,
		Timestamp: time.Now(),
		Duration:  duration,
		Success:   success,
		ErrorType: errorType,
	}
	e.operationHistory = append(e.operationHistory, op)

	// Log operation
	e.logger.WithFields(logrus.Fields{
		"operation": operation,
		"resource":  kind,
		"namespace": namespace,
		"name":      name,
		"success":   success,
		"duration":  duration,
	}).Debug("Enhanced K8s operation tracked")
}

// GetOperationHistory returns the history of operations performed
func (e *EnhancedK8sFakeClient) GetOperationHistory() []K8sOperation {
	return e.operationHistory
}

// GetResourceStates returns the current state of tracked resources
func (e *EnhancedK8sFakeClient) GetResourceStates() map[string]ResourceState {
	return e.resourceTracker
}

// ClearHistory resets operation history and resource tracking
func (e *EnhancedK8sFakeClient) ClearHistory() {
	e.operationHistory = make([]K8sOperation, 0)
	e.resourceTracker = make(map[string]ResourceState)
}

// CreatePodWithRealisticDelay creates a pod with realistic timing
func (e *EnhancedK8sFakeClient) CreatePodWithRealisticDelay(ctx context.Context, pod *corev1.Pod) (*corev1.Pod, error) {
	startTime := time.Now()

	// Check for network effects
	if err := e.simulateNetworkEffects("CREATE"); err != nil {
		e.trackResourceOperation("CREATE", "pod", pod.Namespace, pod.Name, false, time.Since(startTime), "network")
		return nil, err
	}

	// Check for conflicts
	if err := e.simulateResourceConflict(pod.Namespace, pod.Name); err != nil {
		e.trackResourceOperation("CREATE", "pod", pod.Namespace, pod.Name, false, time.Since(startTime), "conflict")
		return nil, err
	}

	// Perform actual creation
	createdPod, err := e.Interface.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	duration := time.Since(startTime)

	if err != nil {
		e.trackResourceOperation("CREATE", "pod", pod.Namespace, pod.Name, false, duration, "api")
		return nil, err
	}

	// Simulate realistic pod startup timing
	if createdPod.Status.Phase == "" {
		createdPod.Status.Phase = corev1.PodPending
	}

	e.trackResourceOperation("CREATE", "pod", pod.Namespace, pod.Name, true, duration, "")
	return createdPod, nil
}

// DeletePodWithRealisticDelay deletes a pod with realistic timing
func (e *EnhancedK8sFakeClient) DeletePodWithRealisticDelay(ctx context.Context, namespace, name string) error {
	startTime := time.Now()

	// Check for network effects
	if err := e.simulateNetworkEffects("DELETE"); err != nil {
		e.trackResourceOperation("DELETE", "pod", namespace, name, false, time.Since(startTime), "network")
		return err
	}

	// Perform actual deletion
	err := e.Interface.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	duration := time.Since(startTime)

	if err != nil {
		e.trackResourceOperation("DELETE", "pod", namespace, name, false, duration, "api")
		return err
	}

	e.trackResourceOperation("DELETE", "pod", namespace, name, true, duration, "")
	return nil
}

// GetStatistics returns usage statistics for the enhanced client
func (e *EnhancedK8sFakeClient) GetStatistics() map[string]interface{} {
	totalOps := len(e.operationHistory)
	successfulOps := 0
	var totalDuration time.Duration
	errorBreakdown := make(map[string]int)
	operationBreakdown := make(map[string]int)

	for _, op := range e.operationHistory {
		if op.Success {
			successfulOps++
		} else {
			errorBreakdown[op.ErrorType]++
		}
		totalDuration += op.Duration
		operationBreakdown[op.Type]++
	}

	stats := map[string]interface{}{
		"total_operations":      totalOps,
		"successful_operations": successfulOps,
		"success_rate":          0.0,
		"average_latency_ms":    0.0,
		"error_breakdown":       errorBreakdown,
		"operation_breakdown":   operationBreakdown,
		"tracked_resources":     len(e.resourceTracker),
	}

	if totalOps > 0 {
		stats["success_rate"] = float64(successfulOps) / float64(totalOps)
		stats["average_latency_ms"] = float64(totalDuration) / float64(totalOps) / float64(time.Millisecond)
	}

	return stats
}

// Additional K8s error types for enhanced fake client (extending existing types)
const (
	K8sServiceUnavailable K8sErrorType = "service_unavailable"
	K8sTimeout            K8sErrorType = "timeout"
	K8sResourceExists     K8sErrorType = "resource_exists"
	K8sValidationError    K8sErrorType = "validation_error"
)

// Enhanced K8s Error Injection Methods as specified in TODO requirements

// SimulateAPIServerError simulates specific API server error conditions
func (e *EnhancedK8sFakeClient) SimulateAPIServerError(errorType K8sErrorType) error {
	if !e.errorInjection.Enabled {
		return fmt.Errorf("error injection is not enabled")
	}

	// Add this error type to active scenarios
	e.k8sErrorTypes = append(e.k8sErrorTypes, errorType)

	e.logger.WithFields(logrus.Fields{
		"error_type": errorType,
	}).Info("Simulating Kubernetes API server error")

	return nil
}

// ConfigureErrorScenario configures K8s client for specific error scenario
func (e *EnhancedK8sFakeClient) ConfigureErrorScenario(scenario ErrorScenario) error {
	if !e.errorInjection.Enabled {
		e.errorInjection.Enabled = true
	}

	// Mark scenario as active
	e.activeScenarios[scenario.Name] = true

	// Configure based on scenario category
	switch scenario.Category {
	case K8sAPIError:
		e.simulateTimeouts = true
		e.errorRate = 0.8 // High error rate for API errors

		// Add specific K8s error types based on scenario description
		switch scenario.Name {
		case "k8s_api_rate_limit":
			e.k8sErrorTypes = append(e.k8sErrorTypes, K8sQuotaExceeded)
		case "resource_conflicts":
			e.k8sErrorTypes = append(e.k8sErrorTypes, K8sResourceConflict)
			e.simulateConflicts = true
		case "permission_denied":
			e.k8sErrorTypes = append(e.k8sErrorTypes, K8sPermissionDenied)
		case "api_server_down":
			e.k8sErrorTypes = append(e.k8sErrorTypes, K8sAPIServerUnavailable, K8sServiceUnavailable)
		default:
			e.k8sErrorTypes = append(e.k8sErrorTypes, K8sInternalError)
		}

	case NetworkError:
		e.networkDelay = scenario.Duration
		e.simulateTimeouts = true
		e.k8sErrorTypes = append(e.k8sErrorTypes, K8sTimeout, K8sAPIServerUnavailable)

	case TimeoutError:
		e.networkDelay = scenario.Duration
		e.simulateTimeouts = true
		e.k8sErrorTypes = append(e.k8sErrorTypes, K8sTimeout)

	default:
		e.errorRate = 0.5 // Moderate error rate for generic scenarios
		e.k8sErrorTypes = append(e.k8sErrorTypes, K8sInternalError)
	}

	// Schedule automatic recovery
	if scenario.RecoveryTime > 0 {
		go func() {
			time.Sleep(scenario.RecoveryTime)
			e.recoverFromScenario(scenario.Name)
		}()
	}

	e.logger.WithFields(logrus.Fields{
		"scenario":          scenario.Name,
		"category":          scenario.Category,
		"duration":          scenario.Duration,
		"recovery_time":     scenario.RecoveryTime,
		"error_rate":        e.errorRate,
		"simulate_timeouts": e.simulateTimeouts,
		"k8s_error_types":   e.k8sErrorTypes,
	}).Info("Configured K8s error scenario")

	return nil
}

// InjectTransientFailures injects temporary failures for the specified duration
func (e *EnhancedK8sFakeClient) InjectTransientFailures(duration time.Duration) {
	e.errorInjection.Enabled = true
	e.errorRate = 0.3 // 30% failure rate
	e.simulateTimeouts = true

	// Add common transient error types
	e.k8sErrorTypes = []K8sErrorType{
		K8sServiceUnavailable,
		K8sTimeout,
		K8sInternalError,
	}

	e.logger.WithFields(logrus.Fields{
		"duration":    duration,
		"error_rate":  e.errorRate,
		"error_types": e.k8sErrorTypes,
	}).Info("Injecting transient failures into K8s client")

	// Auto-recovery after duration
	go func() {
		time.Sleep(duration)
		e.ResetErrorState()
	}()
}

// ResetErrorState resets all error injection state
func (e *EnhancedK8sFakeClient) ResetErrorState() {
	e.errorInjection.Enabled = false
	e.errorRate = 0.0
	e.simulateTimeouts = false
	e.simulateConflicts = false
	e.networkDelay = 0
	e.k8sErrorTypes = []K8sErrorType{}
	e.activeScenarios = make(map[string]bool)

	e.logger.Info("Reset K8s error injection state")
}

// GetActiveScenarios returns currently active error scenarios
func (e *EnhancedK8sFakeClient) GetActiveScenarios() map[string]bool {
	scenariosCopy := make(map[string]bool)
	for name, active := range e.activeScenarios {
		scenariosCopy[name] = active
	}
	return scenariosCopy
}

// Internal helper methods for error injection

// recoverFromScenario recovers from a specific error scenario
func (e *EnhancedK8sFakeClient) recoverFromScenario(scenarioName string) {
	delete(e.activeScenarios, scenarioName)

	// If no active scenarios remain, reset to normal operation
	if len(e.activeScenarios) == 0 {
		e.errorRate = 0.0
		e.simulateTimeouts = false
		e.simulateConflicts = false
		e.networkDelay = 0
		e.k8sErrorTypes = []K8sErrorType{}
	}

	e.logger.WithField("scenario", scenarioName).Info("Recovered from K8s error scenario")
}

// shouldInjectK8sError determines if a K8s error should be injected
func (e *EnhancedK8sFakeClient) shouldInjectK8sError() (bool, K8sErrorType) {
	if !e.errorInjection.Enabled || e.errorRate == 0 {
		return false, ""
	}

	// Simple probability check
	if rand.Float64() < e.errorRate {
		if len(e.k8sErrorTypes) > 0 {
			// Select random error type
			errorType := e.k8sErrorTypes[rand.Intn(len(e.k8sErrorTypes))]
			return true, errorType
		}
		return true, K8sInternalError // Default error type
	}

	return false, ""
}

// createK8sError creates a Kubernetes API error of the specified type
func (e *EnhancedK8sFakeClient) createK8sError(errorType K8sErrorType, resource schema.GroupResource, name string) error {
	switch errorType {
	case K8sResourceConflict:
		return errors.NewConflict(resource, name, fmt.Errorf("simulated resource conflict"))
	case K8sPermissionDenied:
		return errors.NewForbidden(resource, name, fmt.Errorf("simulated permission denied"))
	case K8sQuotaExceeded:
		return errors.NewForbidden(resource, name, fmt.Errorf("simulated quota exceeded"))
	case K8sAPIServerUnavailable:
		return errors.NewServiceUnavailable("simulated API server unavailable")
	case K8sResourceNotFound:
		return errors.NewNotFound(resource, name)
	case K8sServiceUnavailable:
		return errors.NewServiceUnavailable("simulated service unavailable")
	case K8sTimeout:
		return errors.NewTimeoutError("simulated timeout", 0)
	case K8sResourceExists:
		return errors.NewAlreadyExists(resource, name)
	case K8sValidationError:
		return errors.NewBadRequest("simulated validation error")
	default:
		return errors.NewInternalError(fmt.Errorf("simulated internal error"))
	}
}
