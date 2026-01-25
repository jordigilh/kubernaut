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

package k8s

import (
	"context"
	"time"

	"github.com/sony/gobreaker"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// ClientWithCircuitBreaker wraps K8s client operations with circuit breaker protection
//
// Business Requirements:
// - BR-GATEWAY-093: Circuit breaker for critical dependencies (K8s API)
// - BR-GATEWAY-093-A: Fail-fast when K8s API unavailable
// - BR-GATEWAY-093-B: Prevent cascade failures during K8s API overload
// - BR-GATEWAY-093-C: Observable metrics for circuit breaker state and operations
//
// Design Decision: DD-GATEWAY-015 (Kubernetes API Circuit Breaker Implementation)
//
// Circuit Breaker States:
// - Closed: Normal operation, all requests allowed
// - Open: Too many failures, block all requests (fail-fast)
// - Half-Open: Testing recovery, allow limited requests
//
// Configuration:
// - MaxRequests: 3 (allow 3 test requests in half-open state)
// - Interval: 10s (reset failure count every 10s)
// - Timeout: 30s (stay open for 30s before attempting recovery)
// - ReadyToTrip: 50% failure rate over 10 requests triggers open state
//
// Metrics Integration:
// - OnStateChange callback updates gateway_circuit_breaker_state metric
// - Success/failure tracked in gateway_circuit_breaker_operations_total
type ClientWithCircuitBreaker struct {
	*Client                        // Embed base client for non-circuit-breaker methods
	cb      *gobreaker.CircuitBreaker
	metrics *metrics.Metrics
}

// NewClientWithCircuitBreaker creates a K8s client with circuit breaker protection
//
// Parameters:
// - client: Base K8s client
// - metricsInstance: Prometheus metrics for observability
//
// Circuit Breaker Configuration:
// - Threshold: 50% failure rate over 10 requests
// - Recovery: 30s timeout before testing recovery
// - Half-Open: Allow 3 test requests during recovery
func NewClientWithCircuitBreaker(client *Client, metricsInstance *metrics.Metrics) *ClientWithCircuitBreaker {
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "k8s-api",
		MaxRequests: 3, // Allow 3 test requests in half-open state
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second, // Stay open for 30s before trying again
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip circuit if 50% failure rate over 10 requests
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 10 && failureRatio >= 0.5
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			// Update Prometheus metric
			if metricsInstance != nil {
				metricsInstance.CircuitBreakerState.WithLabelValues(name).Set(float64(to))
			}
		},
	})

	return &ClientWithCircuitBreaker{
		Client:  client,
		cb:      cb,
		metrics: metricsInstance,
	}
}

// ========================================
// Client Interface Methods (with Circuit Breaker Protection)
// ========================================

// CreateRemediationRequest creates a RemediationRequest with circuit breaker protection
//
// Circuit Breaker Behavior:
// - If circuit is OPEN: Returns gobreaker.ErrOpenState immediately (fail-fast)
// - If circuit is CLOSED/HALF-OPEN: Executes K8s API call and tracks result
// - AlreadyExists errors are treated as idempotent success (don't trip circuit breaker)
//
// Benefits (BR-GATEWAY-093):
// - Fail-fast during K8s API outages (BR-GATEWAY-093-A)
// - Prevents cascade failures (BR-GATEWAY-093-B)
// - Observable via Prometheus metrics (BR-GATEWAY-093-C)
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - rr: RemediationRequest CRD to create
//
// Returns:
// - error: gobreaker.ErrOpenState if circuit is open, or K8s API error
func (c *ClientWithCircuitBreaker) CreateRemediationRequest(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
	_, err := c.cb.Execute(func() (interface{}, error) {
		err := c.Client.CreateRemediationRequest(ctx, rr)

		// BR-GATEWAY-CIRCUIT-BREAKER-IDEMPOTENT-FIX v3.0:
		// Treat "AlreadyExists" as idempotent success for BOTH metrics AND circuit breaker state.
		// This prevents circuit breaker from opening during parallel test execution.
		if k8serrors.IsAlreadyExists(err) {
			c.recordOperationResultWithIdempotency("create", err)  // Metrics: success
			return nil, nil  // Circuit breaker: success (don't increment failure count)
		}

		// All other errors: record and return as-is
		c.recordOperationResult("create", err)
		return nil, err
	})
	return err
}

// UpdateRemediationRequest updates a RemediationRequest with circuit breaker protection
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - rr: RemediationRequest CRD with updated fields
//
// Returns:
// - error: gobreaker.ErrOpenState if circuit is open, or K8s API error
func (c *ClientWithCircuitBreaker) UpdateRemediationRequest(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
	_, err := c.cb.Execute(func() (interface{}, error) {
		err := c.Client.UpdateRemediationRequest(ctx, rr)
		c.recordOperationResult("update", err)
		return nil, err
	})
	return err
}

// GetRemediationRequest retrieves a RemediationRequest with circuit breaker protection
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - namespace: Namespace of the RemediationRequest
// - name: Name of the RemediationRequest
//
// Returns:
// - *RemediationRequest: The retrieved CRD
// - error: gobreaker.ErrOpenState if circuit is open, or K8s API error
func (c *ClientWithCircuitBreaker) GetRemediationRequest(ctx context.Context, namespace, name string) (*remediationv1alpha1.RemediationRequest, error) {
	result, err := c.cb.Execute(func() (interface{}, error) {
		rr, err := c.Client.GetRemediationRequest(ctx, namespace, name)
		c.recordOperationResult("get", err)
		return rr, err
	})

	if err != nil {
		return nil, err
	}

	return result.(*remediationv1alpha1.RemediationRequest), nil
}

// State returns the current circuit breaker state (for observability)
//
// Returns:
// - gobreaker.StateClosed (0): Normal operation
// - gobreaker.StateHalfOpen (1): Testing recovery
// - gobreaker.StateOpen (2): Blocking requests (fail-fast)
func (c *ClientWithCircuitBreaker) State() gobreaker.State {
	return c.cb.State()
}

// recordOperationResult updates Prometheus metrics for circuit breaker operations
func (c *ClientWithCircuitBreaker) recordOperationResult(operation string, err error) {
	if c.metrics == nil {
		return
	}

	result := "success"
	if err != nil {
		result = "failure"
	}

	c.metrics.CircuitBreakerOperations.WithLabelValues("k8s-api", result).Inc()
}

// recordOperationResultWithIdempotency updates metrics treating IsAlreadyExists as success
//
// BR-GATEWAY-CIRCUIT-BREAKER-IDEMPOTENT-FIX:
// This prevents circuit breaker from opening due to "AlreadyExists" errors during
// parallel test execution or concurrent requests with the same fingerprint.
//
// Idempotent Operations:
// - IsAlreadyExists(err) → Treated as SUCCESS for circuit breaker purposes
// - Other errors → Treated as FAILURE (may trip circuit breaker)
//
// This method MUST be used for CRD Create operations to prevent false positives
// in circuit breaker failure tracking.
func (c *ClientWithCircuitBreaker) recordOperationResultWithIdempotency(operation string, err error) {
	if c.metrics == nil {
		return
	}

	result := "success"
	if err != nil {
		// Treat "AlreadyExists" as success (idempotent operation)
		if !k8serrors.IsAlreadyExists(err) {
			result = "failure"
		}
	}

	c.metrics.CircuitBreakerOperations.WithLabelValues("k8s-api", result).Inc()
}
