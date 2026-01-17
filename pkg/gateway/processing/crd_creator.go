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

package processing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
	sharedK8s "github.com/jordigilh/kubernaut/pkg/shared/k8s"
	// DD-GATEWAY-011: sharedtypes import removed (Spec.Deduplication no longer populated)
)

// CRDCreator converts NormalizedSignal to RemediationRequest CRD
//
// This component is responsible for:
// 1. Generating unique CRD names (rr-<fingerprint-prefix>)
// 2. Populating CRD spec from signal data
// 3. Adding required labels for querying/filtering
// 4. Creating CRD in Kubernetes via API
// 5. Recording metrics (success/failure)
//
// CRD naming:
// - Format: rr-<first-16-chars-of-fingerprint>
// - Example: rr-a1b2c3d4e5f6789
// - Reason: Unique, deterministic, short (Kubernetes name limit: 253 chars)
type CRDCreator struct {
	k8sClient         k8s.ClientInterface  // TDD GREEN: Interface supports circuit breaker (BR-GATEWAY-093)
	logger            logr.Logger           // DD-005: logr.Logger for unified logging
	metrics           *metrics.Metrics      // Day 9 Phase 6B Option C1: Centralized metrics
	fallbackNamespace string                // Configurable fallback namespace for CRD creation
	retryConfig       *config.RetrySettings // BR-GATEWAY-111: Retry configuration
	clock             Clock                 // Clock for time-dependent operations (enables fast testing)
}

// NewCRDCreator creates a new CRD creator
// BR-GATEWAY-111: Accepts retry configuration for K8s API retry logic
// DD-005: Uses logr.Logger for unified logging
func NewCRDCreator(k8sClient k8s.ClientInterface, logger logr.Logger, metricsInstance *metrics.Metrics, fallbackNamespace string, retryConfig *config.RetrySettings) *CRDCreator {
	return NewCRDCreatorWithClock(k8sClient, logger, metricsInstance, fallbackNamespace, retryConfig, nil)
}

// NewCRDCreatorWithClock creates a new CRD creator with a custom clock
// This variant enables testing with MockClock for time-dependent behavior
//
// TDD GREEN: Accepts ClientInterface to support circuit breaker (BR-GATEWAY-093)
func NewCRDCreatorWithClock(k8sClient k8s.ClientInterface, logger logr.Logger, metricsInstance *metrics.Metrics, fallbackNamespace string, retryConfig *config.RetrySettings, clock Clock) *CRDCreator {
	// Metrics are mandatory for observability
	if metricsInstance == nil {
		panic("metrics cannot be nil: metrics are mandatory for observability")
	}

	// Fallback namespace validation
	if fallbackNamespace == "" {
		fallbackNamespace = "kubernaut-system" // Safe default
	}

	// Default retry config if not provided
	if retryConfig == nil {
		defaultConfig := config.DefaultRetrySettings()
		retryConfig = &defaultConfig
	}

	// Default clock if not provided
	if clock == nil {
		clock = NewRealClock()
	}

	return &CRDCreator{
		k8sClient:         k8sClient,
		logger:            logger,
		metrics:           metricsInstance,
		fallbackNamespace: fallbackNamespace,
		retryConfig:       retryConfig,
		clock:             clock,
	}
}

// ========================================
// CRD CREATION RETRY WITH SHARED BACKOFF
// 📋 Shared Utility: pkg/shared/backoff | ✅ Production-Ready | Confidence: 95%
// See: docs/handoff/TEAM_ANNOUNCEMENT_SHARED_BACKOFF.md
// ========================================
//
// createCRDWithRetry implements retry logic with exponential backoff for transient K8s API errors.
// Uses shared backoff utility for consistent retry behavior across all Kubernaut services.
//
// WHY SHARED BACKOFF?
// - ✅ Anti-thundering herd: ±10% jitter prevents simultaneous retries across Gateway pods
// - ✅ Consistent behavior: Matches NT, WE, SP, RO, AA services
// - ✅ Industry best practice: Aligns with Kubernetes ecosystem standards
// - ✅ Centralized maintenance: Bug fixes and improvements in one place
//
// BR-GATEWAY-112: Error Classification (retryable vs non-retryable)
// BR-GATEWAY-113: Exponential Backoff with jitter (shared utility)
// BR-GATEWAY-114: Retry Metrics
// ========================================
func (c *CRDCreator) createCRDWithRetry(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
	startTime := c.clock.Now()

	for attempt := 0; attempt < c.retryConfig.MaxAttempts; attempt++ {
		// Attempt CRD creation
		err := c.k8sClient.CreateRemediationRequest(ctx, rr)

		// Success
		if err == nil {
			// Log success if this was a retry (attempt > 0)
			if attempt > 0 {
				c.logger.Info("CRD creation succeeded after retry",
					"attempt", attempt+1,
					"total_duration", time.Since(startTime),
					"name", rr.Name,
					"namespace", rr.Namespace)
			}
			return nil
		}

		// BR-GATEWAY-CIRCUIT-BREAKER-FIX: Handle "already exists" as idempotent success
		// This prevents circuit breaker from opening due to parallel test execution
		// where multiple requests with the same fingerprint arrive simultaneously.
		if k8serrors.IsAlreadyExists(err) {
			// CRD already exists - this is idempotent success, NOT a failure
			c.logger.Info("CRD already exists (idempotent success)",
				"name", rr.Name,
				"namespace", rr.Namespace,
				"fingerprint", rr.Spec.SignalFingerprint)

			// Optionally fetch the existing CRD to verify fingerprint matches
			existing, getErr := c.k8sClient.GetRemediationRequest(ctx, rr.Namespace, rr.Name)
			if getErr != nil {
				c.logger.Error(getErr, "Failed to fetch existing CRD after AlreadyExists error",
					"name", rr.Name,
					"namespace", rr.Namespace)
				// Still return success - the CRD exists, which is our goal
				return nil
			}

			// Log for debugging: verify fingerprints match
			if existing.Spec.SignalFingerprint != rr.Spec.SignalFingerprint {
				c.logger.Info("Warning: Existing CRD has different fingerprint (hash collision?)",
					"name", rr.Name,
					"namespace", rr.Namespace,
					"expected_fingerprint", rr.Spec.SignalFingerprint,
					"actual_fingerprint", existing.Spec.SignalFingerprint)
			}

		// Return success - CRD exists (idempotent operation)
		return nil
	}

	// BR-GATEWAY-NAMESPACE-FALLBACK: Handle namespace not found by falling back to kubernaut-system
	// Business Outcome: Invalid namespace doesn't block remediation
	// Example scenarios: Namespace deleted after alert fired, cluster-scoped signals (NodeNotReady)
	// Test: test/e2e/gateway/27_error_handling_test.go:224
	if k8serrors.IsNotFound(err) && isNamespaceNotFoundError(err) {
		originalNamespace := rr.Namespace

		c.logger.Info("Namespace not found, falling back to kubernaut-system",
			"original_namespace", originalNamespace,
			"fallback_namespace", "kubernaut-system",
			"crd_name", rr.Name)

		// Update CRD to use kubernaut-system namespace
		rr.Namespace = "kubernaut-system"

		// Add labels to track the fallback
		if rr.Labels == nil {
			rr.Labels = make(map[string]string)
		}
		rr.Labels["kubernaut.ai/cluster-scoped"] = "true"
		rr.Labels["kubernaut.ai/origin-namespace"] = originalNamespace

		// Retry creation in kubernaut-system namespace
		err = c.k8sClient.CreateRemediationRequest(ctx, rr)
		if err == nil {
			c.logger.Info("CRD created successfully in kubernaut-system namespace after fallback",
				"original_namespace", originalNamespace,
				"crd_name", rr.Name)
			return nil
		}

		// If fallback also failed, log and continue to error handling below
		c.logger.Error(err, "CRD creation failed even after kubernaut-system fallback",
			"original_namespace", originalNamespace,
			"fallback_namespace", "kubernaut-system",
			"crd_name", rr.Name)
		// Fall through to normal error handling
	}

	// Get error classification info for metrics
	errorType := getErrorTypeString(err)
	// GAP-10: Simplified error type detection (no external dependencies)
	isRetryable := errorType == "rate_limited" || errorType == "service_unavailable" ||
		errorType == "gateway_timeout" || errorType == "timeout" || errorType == "network_error"

	// Non-retryable error (validation, RBAC, etc.)
	if !isRetryable {
		c.logger.Error(err, "CRD creation failed with non-retryable error",
			"name", rr.Name,
			"namespace", rr.Namespace,
			"error_type", errorType)
		return err
	}

		// Check if this was the last attempt
		if attempt == c.retryConfig.MaxAttempts-1 {
			// Exhausted all retries - return error immediately

			c.logger.Error(err, "CRD creation failed after max retries",
				"max_attempts", c.retryConfig.MaxAttempts,
				"total_duration", time.Since(startTime),
				"name", rr.Name,
				"namespace", rr.Namespace)

			// Wrap error with comprehensive retry context (GAP-10: Enhanced Error Wrapping)
			return &RetryError{
				Attempt:     attempt + 1,
				MaxAttempts: c.retryConfig.MaxAttempts,
				OriginalErr: err,
				ErrorType:   errorType,
				IsRetryable: isRetryable,
			}
		}

		// Calculate backoff using shared utility (with ±10% jitter for anti-thundering herd)
		// Shared backoff utility ensures consistent retry behavior across all Kubernaut services
		backoffConfig := backoff.Config{
			BasePeriod:    c.retryConfig.InitialBackoff,
			MaxPeriod:     c.retryConfig.MaxBackoff,
			Multiplier:    2.0, // Standard exponential (doubles each retry)
			JitterPercent: 10,  // ±10% variance (prevents thundering herd)
		}
		backoffDuration := backoffConfig.Calculate(int32(attempt + 1))

		// Not the last attempt - retry with exponential backoff
		c.logger.Info("CRD creation failed, retrying with shared backoff...",
			"warning", true,
			"attempt", attempt+1,
			"max_attempts", c.retryConfig.MaxAttempts,
			"backoff", backoffDuration,
			"error", err,
			"name", rr.Name,
			"namespace", rr.Namespace)

		// Sleep with backoff (context-aware for graceful shutdown - GAP 6)
		select {
		case <-time.After(backoffDuration):
			// Continue to next attempt
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Defensive: This should never be reached due to the last attempt check above
	// If we reach here, it indicates a logic error in the retry loop
	return fmt.Errorf("retry logic error: loop completed without explicit return (max_attempts=%d)", c.retryConfig.MaxAttempts)
}

// getErrorTypeString returns a human-readable error type for metrics labels
// getErrorTypeString classifies errors by parsing error messages and K8s API status codes.
// GAP-10: Simplified error classification without external dependencies.
// errorPattern defines a pattern to match against error strings
type errorPattern struct {
	patterns []string // Patterns to match (case-insensitive)
	errorType string  // Error type to return if matched
}

// errorPatterns defines the mapping of error patterns to error types
// Ordered by priority (more specific patterns first)
var errorPatterns = []errorPattern{
	// K8s API status errors
	{patterns: []string{"429", "rate limit", "too many requests"}, errorType: "rate_limited"},
	{patterns: []string{"503", "service unavailable", "api server overloaded"}, errorType: "service_unavailable"},
	{patterns: []string{"504", "gateway timeout"}, errorType: "gateway_timeout"},
	{patterns: []string{"400", "bad request"}, errorType: "bad_request"},
	{patterns: []string{"403", "forbidden"}, errorType: "forbidden"},
	{patterns: []string{"422", "unprocessable"}, errorType: "unprocessable_entity"},
	{patterns: []string{"409", "conflict", "already exists"}, errorType: "conflict"},
	// Timeout/network errors
	{patterns: []string{"timeout", "deadline exceeded"}, errorType: "timeout"},
	{patterns: []string{"connection refused", "connection reset"}, errorType: "network_error"},
}

// getErrorTypeString classifies an error into a category for metrics/logging
//
// This function uses pattern matching to classify errors into categories:
// - K8s API errors (rate_limited, service_unavailable, etc.)
// - Network errors (timeout, network_error)
// - Unknown errors (catch-all)
//
// Complexity: Reduced from 23 to <10 using data-driven approach
func getErrorTypeString(err error) string {
	if err == nil {
		return "success"
	}

	// Convert to lowercase for case-insensitive matching
	errStr := strings.ToLower(err.Error())

	// Check each error pattern in priority order
	for _, pattern := range errorPatterns {
		for _, p := range pattern.patterns {
			if strings.Contains(errStr, p) {
				return pattern.errorType
			}
		}
	}

	return "unknown"
}

// CreateRemediationRequest creates a RemediationRequest CRD from a signal
//
// This method:
// 1. Generates CRD name from fingerprint
// 2. Populates metadata (labels, annotations)
// 3. Populates spec (signal data, deduplication info, timestamps)
// 4. Creates CRD in Kubernetes
// 5. Records success/failure metrics
// 6. Logs creation event
//
// CRD structure:
//
//	apiVersion: remediation.kubernaut.ai/v1alpha1
//	kind: RemediationRequest
//	metadata:
//	  name: rr-<fingerprint-prefix>
//	  namespace: <signal-namespace>
//	  labels:
//	    kubernaut.ai/signal-type: prometheus-alert
//	    kubernaut.ai/signal-fingerprint: <full-fingerprint>
//	    kubernaut.ai/severity: critical
//	spec:
//	  signalFingerprint: <fingerprint>
//	  signalName: HighMemoryUsage
//	  severity: critical
//	  environment: prod
//	  priority: P0
//	  signalType: prometheus-alert
//	  targetType: kubernetes
//	  firingTime: <timestamp>
//	  receivedTime: <timestamp>
//	  signalLabels: {alertname: ..., namespace: ..., pod: ...}
//	  signalAnnotations: {summary: ..., description: ...}
//	  originalPayload: <base64-encoded-json>
//	  deduplication:
//	    firstSeen: <timestamp>
//	    lastSeen: <timestamp>
//	    occurrenceCount: 1
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - signal: Normalized signal from adapter
//
// Note: Environment and Priority classification removed from Gateway (2025-12-06)
// These are now owned by Signal Processing service per DD-CATEGORIZATION-001.
// See: docs/handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md
//
// Returns:
// - *RemediationRequest: Created CRD with populated fields
// - error: Kubernetes API errors or validation errors
func (c *CRDCreator) CreateRemediationRequest(
	ctx context.Context,
	signal *types.NormalizedSignal,
) (*remediationv1alpha1.RemediationRequest, error) {
	// GAP-10: Track start time for error duration reporting
	startTime := c.clock.Now()

	// BR-GATEWAY-TARGET-RESOURCE-VALIDATION: Validate resource info (V1.0 Kubernetes-only)
	// Business Outcome: V1.0 only supports Kubernetes signals - reject signals without
	// resource info with HTTP 400 to provide clear feedback to alert sources.
	// This prevents downstream processing failures in SP enrichment, AIAnalysis RCA, and WE remediation.
	if err := c.validateResourceInfo(signal); err != nil {
		c.logger.Info("Signal rejected: missing resource info",
			"fingerprint", signal.Fingerprint,
			"alertName", signal.AlertName,
			"error", err)
		return nil, err
	}

	// DD-AUDIT-CORRELATION-002: UUID-Based CRD Naming for Unique Occurrences
	// Generate CRD name from fingerprint (first 12 chars) + UUID suffix (8 chars)
	// Format: rr-{fingerprint-prefix}-{uuid-suffix}
	// Example: "rr-pod-crash-f8a3b9c2" (human-readable + globally unique)
	// 
	// This ensures:
	// - Each signal occurrence creates a unique CRD (zero collision risk)
	// - Human-readable correlation IDs for debugging
	// - Universal standard: All services use rr.Name as correlation_id
	//
	// Supersedes: DD-015 (timestamp-based naming)
	fingerprintPrefix := signal.Fingerprint
	if len(fingerprintPrefix) > 12 {
		fingerprintPrefix = fingerprintPrefix[:12]
	}
	// DD-AUDIT-CORRELATION-002: UUID suffix guarantees zero collision risk
	shortUUID := uuid.New().String()[:8]
	crdName := fmt.Sprintf("rr-%s-%s", fingerprintPrefix, shortUUID)

	// Build RemediationRequest CRD
	rr := &remediationv1alpha1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crdName,
			Namespace: signal.Namespace,
			Labels: map[string]string{
				// Standard Kubernetes labels for interoperability
				"app.kubernetes.io/managed-by": "gateway-service",
				"app.kubernetes.io/component":  "remediation",

				// Kubernaut-specific labels for filtering and routing
				"kubernaut.ai/signal-type": signal.SourceType,
				// Truncate fingerprint to 63 chars (K8s label value max length)
				"kubernaut.ai/signal-fingerprint": signal.Fingerprint[:min(len(signal.Fingerprint), 63)],
				"kubernaut.ai/severity":           signal.Severity,
				// Note: kubernaut.ai/environment and kubernaut.ai/priority removed (2025-12-06)
				// Signal Processing service now owns classification per DD-CATEGORIZATION-001
			},
		Annotations: map[string]string{
			// Timestamp for audit trail (RFC3339 format)
			"kubernaut.ai/created-at": c.clock.Now().UTC().Format(time.RFC3339),
		},
		},
		Spec: remediationv1alpha1.RemediationRequestSpec{
			// Core signal identification
			SignalFingerprint: signal.Fingerprint,
			SignalName:        signal.AlertName,

			// Classification (environment/priority removed - SP owns these now)
			Severity:     signal.Severity,
			SignalType:   signal.SourceType,
			SignalSource: signal.Source,
			TargetType:   "kubernetes",

			// Target resource identification (BR-GATEWAY-TARGET-RESOURCE)
			// Business Outcome: SignalProcessing and RO can access resource info directly
			// without parsing ProviderData JSON
			TargetResource: c.buildTargetResource(signal),

			// Temporal data
			// Fallback: if FiringTime is zero (not set by adapter), use ReceivedTime
			FiringTime:   metav1.NewTime(c.getFiringTime(signal)),
			ReceivedTime: metav1.NewTime(signal.ReceivedTime),

			// Signal metadata (with K8s label value truncation)
			SignalLabels:      c.truncateLabelValues(signal.Labels),
			SignalAnnotations: c.truncateAnnotationValues(signal.Annotations),

			// Provider-specific data (structured JSON for downstream services)
			ProviderData: c.buildProviderData(signal),

			// Original payload for audit trail
			OriginalPayload: signal.RawPayload,

			// DD-GATEWAY-011: Deduplication REMOVED from Spec (moved to Status)
			// Gateway now owns status.deduplication (initialized by StatusUpdater)
			// Spec.Deduplication kept in API for backwards compatibility but NOT populated
		},
	}

	// Create CRD in Kubernetes with retry logic
	// BR-GATEWAY-112: Retry logic for transient K8s API errors
	if err := c.createCRDWithRetry(ctx, rr); err != nil {
		// Check if CRD already exists (e.g., Redis TTL expired but K8s CRD still exists)
		// This is normal behavior - Redis TTL is shorter than CRD lifecycle
		if strings.Contains(err.Error(), "already exists") {
			c.logger.V(1).Info("RemediationRequest CRD already exists (Redis TTL expired, CRD persists)",
				"name", crdName,
				"namespace", signal.Namespace,
				"fingerprint", signal.Fingerprint,
			)

			// Fetch existing CRD and return it
			// This allows deduplication metadata to be updated in Redis
			//
			// Note: Use retry logic here because in concurrent scenarios, another goroutine
			// may have just created the CRD, and K8s API might not have fully committed it yet.
			// Retry with exponential backoff to allow K8s API time to sync.
			var existing *remediationv1alpha1.RemediationRequest
			var fetchErr error
			maxFetchAttempts := 3
			for attempt := 1; attempt <= maxFetchAttempts; attempt++ {
				existing, fetchErr = c.k8sClient.GetRemediationRequest(ctx, signal.Namespace, crdName)
				if fetchErr == nil {
					break // Successfully fetched
				}

				if attempt < maxFetchAttempts {
					// Exponential backoff: 50ms, 100ms, 200ms
					backoff := time.Duration(50*attempt) * time.Millisecond
					c.logger.V(1).Info("CRD fetch failed, retrying after backoff",
						"name", crdName,
						"attempt", attempt,
						"backoff_ms", backoff.Milliseconds(),
						"error", fetchErr)
					time.Sleep(backoff)
				}
			}

			if fetchErr != nil {
				c.metrics.CRDCreationErrors.WithLabelValues("fetch_existing_failed").Inc()
				// GAP-10: Wrap error with full context
				return nil, NewCRDCreationError(
					signal.Fingerprint,
					signal.Namespace,
					crdName,
					signal.SourceType,
					signal.AlertName,
					maxFetchAttempts,
					startTime,
					fmt.Errorf("CRD exists but failed to fetch after %d attempts: %w", maxFetchAttempts, fetchErr),
				)
			}

			c.logger.Info("Reusing existing RemediationRequest CRD (Redis TTL expired)",
				"name", crdName,
				"namespace", signal.Namespace,
				"fingerprint", signal.Fingerprint,
			)

			return existing, nil
		}

		// Check if namespace doesn't exist - fall back to configured fallback namespace
		// This handles cluster-scoped signals (e.g., NodeNotReady) that don't have a namespace
		if strings.Contains(err.Error(), "namespaces") && strings.Contains(err.Error(), "not found") {
			c.logger.Info("Target namespace not found, creating CRD in fallback namespace",
				"warning", true,
				"original_namespace", signal.Namespace,
				"fallback_namespace", c.fallbackNamespace,
				"crd_name", crdName)

			// Update namespace to configured fallback namespace
			rr.Namespace = c.fallbackNamespace

			// Add labels to preserve origin namespace information for cluster-scoped signals
			rr.Labels["kubernaut.ai/origin-namespace"] = signal.Namespace
			rr.Labels["kubernaut.ai/cluster-scoped"] = "true"

			// Retry creation in fallback namespace with retry logic
			// BR-GATEWAY-112: Retry logic for transient K8s API errors
			if err := c.createCRDWithRetry(ctx, rr); err != nil {
				c.metrics.CRDCreationErrors.WithLabelValues("fallback_failed").Inc()
				// GAP-10: Wrap error with full context
				return nil, NewCRDCreationError(
					signal.Fingerprint,
					c.fallbackNamespace, // Use fallback namespace in error
					crdName,
					signal.SourceType,
					signal.AlertName,
					c.retryConfig.MaxAttempts,
					startTime,
					fmt.Errorf("failed to create CRD in fallback namespace: %w", err),
				)
			}

			// Success with fallback
			c.logger.Info("Created RemediationRequest CRD in fallback namespace (cluster-scoped signal)",
				"name", crdName,
				"namespace", c.fallbackNamespace,
				"fingerprint", signal.Fingerprint,
				"original_ns", signal.Namespace)

			c.metrics.CRDsCreatedTotal.WithLabelValues(signal.SourceType, "fallback_ns").Inc()
			return rr, nil
		}

		// Other errors (not namespace-related, not already-exists)
		c.metrics.CRDCreationErrors.WithLabelValues("k8s_api_error").Inc()

		c.logger.Error(err, "Failed to create RemediationRequest CRD",
			"name", crdName,
			"namespace", signal.Namespace,
			"fingerprint", signal.Fingerprint)

		// GAP-10: Wrap error with full CRD creation context
		return nil, NewCRDCreationError(
			signal.Fingerprint,
			signal.Namespace,
			crdName,
			signal.SourceType,
			signal.AlertName,
			c.retryConfig.MaxAttempts,
			startTime,
			err,
		)
	}

	// Record success metric
	// Note: Metric labels changed from (environment, priority) to (sourceType, "created")
	// since environment/priority classification moved to SP per DD-CATEGORIZATION-001
	c.metrics.CRDsCreatedTotal.WithLabelValues(signal.SourceType, "created").Inc()

	// Log creation event
	c.logger.Info("Created RemediationRequest CRD",
		"name", crdName,
		"namespace", signal.Namespace,
		"fingerprint", signal.Fingerprint,
		"severity", signal.Severity,
		"alertName", signal.AlertName)

	return rr, nil
}

// getFiringTime returns the firing time for a signal, with fallback to ReceivedTime
//
// If the adapter didn't set FiringTime (zero value), we use ReceivedTime instead.
// This ensures the CRD always has a valid timestamp for the firingTime field.
//
// Rationale:
// - Some monitoring systems (tests, simple adapters) don't provide explicit firing timestamps
// - ReceivedTime is always set (Gateway sets it on ingestion)
// - For deduplication/TTL purposes, ReceivedTime is an acceptable fallback
func (c *CRDCreator) getFiringTime(signal *types.NormalizedSignal) time.Time {
	if signal.FiringTime.IsZero() {
		c.logger.V(1).Info("FiringTime not set by adapter, using ReceivedTime as fallback",
			"fingerprint", signal.Fingerprint,
			"alert_name", signal.AlertName)
		return signal.ReceivedTime
	}
	return signal.FiringTime
}

// buildProviderData constructs provider-specific data in JSON format
//
// For Kubernetes signals, this creates a structured JSON with:
// - namespace: Target namespace for remediation
// - resource: Target resource (kind, name, namespace)
// - labels: Signal labels for additional context
//
// Downstream services (AI Analysis, Workflow Execution) parse this to determine:
// - WHICH resource to remediate (kubectl target)
// - WHERE to remediate (namespace, cluster)
// - Additional context for decision-making
func (c *CRDCreator) buildProviderData(signal *types.NormalizedSignal) []byte {
	// Construct provider-specific data structure
	// NOTE: Resource info is NOT included here - it's in spec.TargetResource
	// per API Contract Alignment (BR-GATEWAY-TARGET-RESOURCE)
	providerData := map[string]interface{}{
		"namespace": signal.Namespace,
		"labels":    signal.Labels,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(providerData)
	if err != nil {
		c.logger.Info("Failed to marshal provider data, using empty",
			"warning", true,
			"fingerprint", signal.Fingerprint,
			"error", err)
		return []byte("{}")
	}

	return jsonData
}

// validateResourceInfo validates that signal has required resource info for V1.0 (Kubernetes-only)
// Business Outcome: V1.0 only supports Kubernetes signals. Signals without resource info indicate
// configuration issues at the alert source and should be rejected with clear HTTP 400 feedback.
//
// Per RO team API Contract Alignment (Q3 response):
// - V1.0 is Kubernetes-only
// - Malformed alerts missing resource labels indicate configuration issues at source
// - Rejecting early provides clear feedback to alert authors
// - "Unknown" kind would break downstream processing (SP enrichment, AIAnalysis RCA, WE remediation)
//
// Validation Rules:
// - Resource.Kind is REQUIRED (e.g., "Pod", "Deployment", "Node")
// - Resource.Name is REQUIRED (e.g., "payment-api-789", "worker-node-1")
// - Resource.Namespace may be empty for cluster-scoped resources (e.g., Node, ClusterRole)
//
// Metrics: Increments gateway_signals_rejected_total{reason="..."} on validation failure
func (c *CRDCreator) validateResourceInfo(signal *types.NormalizedSignal) error {
	var missingFields []string

	if signal.Resource.Kind == "" {
		missingFields = append(missingFields, "Kind")
	}
	if signal.Resource.Name == "" {
		missingFields = append(missingFields, "Name")
	}

	if len(missingFields) > 0 {
		return fmt.Errorf("resource validation failed: missing required fields [%s] - V1.0 requires valid Kubernetes resource info",
			strings.Join(missingFields, ", "))
	}

	return nil
}

// buildTargetResource constructs ResourceIdentifier from NormalizedSignal.Resource
// Business Outcome: SignalProcessing and RO can access resource info directly
// without parsing ProviderData JSON
//
// TargetResource is REQUIRED (per API_CONTRACT_TRIAGE.md GAP-C1-03)
// Validation is performed by validateResourceInfo() before this function is called.
func (c *CRDCreator) buildTargetResource(signal *types.NormalizedSignal) remediationv1alpha1.ResourceIdentifier {
	// Note: Validation already performed by validateResourceInfo()
	// Kind and Name are guaranteed to be non-empty at this point
	return remediationv1alpha1.ResourceIdentifier{
		Kind:      signal.Resource.Kind,
		Name:      signal.Resource.Name,
		Namespace: signal.Resource.Namespace,
	}
}

// truncateLabelValues truncates label values to comply with K8s 63 character limit.
// Uses shared K8s validation utility for consistent behavior across services.
// See: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
func (c *CRDCreator) truncateLabelValues(labels map[string]string) map[string]string {
	return sharedK8s.TruncateMapValues(labels, sharedK8s.MaxLabelValueLength)
}

// truncateAnnotationValues truncates annotation values to comply with K8s 256KB limit.
// Uses shared K8s validation utility for consistent behavior across services.
// See: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/#syntax-and-character-set
func (c *CRDCreator) truncateAnnotationValues(annotations map[string]string) map[string]string {
	return sharedK8s.TruncateMapValues(annotations, sharedK8s.MaxAnnotationValueLength)
}

// isNamespaceNotFoundError checks if an error is specifically about a namespace not being found
// (as opposed to a CRD not being found)
//
// Example error message: "namespaces \"does-not-exist-123\" not found"
//
// BR-GATEWAY-NAMESPACE-FALLBACK: Used to detect when to fallback to kubernaut-system namespace
// Test: test/e2e/gateway/27_error_handling_test.go:224
func isNamespaceNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	// Check if error message contains "namespaces" and "not found"
	// This distinguishes namespace errors from RemediationRequest not found errors
	return strings.Contains(errMsg, "namespaces") && strings.Contains(errMsg, "not found")
}
