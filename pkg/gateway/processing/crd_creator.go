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
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
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
	k8sClient         *k8s.Client
	logger            logr.Logger           // DD-005: logr.Logger for unified logging
	metrics           *metrics.Metrics      // Day 9 Phase 6B Option C1: Centralized metrics
	fallbackNamespace string                // Configurable fallback namespace for CRD creation
	retryConfig       *config.RetrySettings // BR-GATEWAY-111: Retry configuration
}

// NewCRDCreator creates a new CRD creator
// BR-GATEWAY-111: Accepts retry configuration for K8s API retry logic
// DD-005: Uses logr.Logger for unified logging
func NewCRDCreator(k8sClient *k8s.Client, logger logr.Logger, metricsInstance *metrics.Metrics, fallbackNamespace string, retryConfig *config.RetrySettings) *CRDCreator {
	// If metrics is nil (e.g., unit tests), create a test-isolated metrics instance
	if metricsInstance == nil {
		// Use custom registry to avoid "duplicate metrics collector registration" in tests
		registry := prometheus.NewRegistry()
		metricsInstance = metrics.NewMetricsWithRegistry(registry)
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

	return &CRDCreator{
		k8sClient:         k8sClient,
		logger:            logger,
		metrics:           metricsInstance,
		fallbackNamespace: fallbackNamespace,
		retryConfig:       retryConfig,
	}
}

// createCRDWithRetry implements retry logic with exponential backoff for transient K8s API errors.
// BR-GATEWAY-112: Error Classification (retryable vs non-retryable)
// BR-GATEWAY-113: Exponential Backoff
// BR-GATEWAY-114: Retry Metrics
func (c *CRDCreator) createCRDWithRetry(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
	backoff := c.retryConfig.InitialBackoff
	startTime := time.Now()

	for attempt := 0; attempt < c.retryConfig.MaxAttempts; attempt++ {
		// Attempt CRD creation
		err := c.k8sClient.CreateRemediationRequest(ctx, rr)

		// Success
		if err == nil {
			// Record success metrics if this was a retry (attempt > 0)
			if attempt > 0 {
				errorType := "success" // Success after retry
				attemptStr := fmt.Sprintf("%d", attempt+1)

				// Record successful retry
				c.metrics.RetrySuccessTotal.WithLabelValues(errorType, attemptStr).Inc()

				// Record retry duration
				duration := time.Since(startTime).Seconds()
				c.metrics.RetryDuration.WithLabelValues(errorType).Observe(duration)

				c.logger.Info("CRD creation succeeded after retry",
					"attempt", attempt+1,
					"total_duration", time.Since(startTime),
					"name", rr.Name,
					"namespace", rr.Namespace)
			}
			return nil
		}

		// Get error classification info for metrics
		errorType := getErrorTypeString(err)
		statusCode := fmt.Sprintf("%d", GetErrorCode(err))

		// Non-retryable error (validation, RBAC, etc.)
		if IsNonRetryableError(err) {
			c.logger.Error(err, "CRD creation failed with non-retryable error",
				"name", rr.Name,
				"namespace", rr.Namespace)
			return err
		}

		// Check if error is retryable (Reliability-First: always retry transient errors)
		if !IsRetryableError(err) {
			c.logger.Error(err, "CRD creation failed with non-retryable error",
				"name", rr.Name,
				"namespace", rr.Namespace)
			return err
		}

		// Record retry attempt metric
		c.metrics.RetryAttemptsTotal.WithLabelValues(errorType, statusCode).Inc()

		// Check if this was the last attempt
		if attempt == c.retryConfig.MaxAttempts-1 {
			// Exhausted all retries - record metrics and return error immediately
			c.metrics.RetryExhaustedTotal.WithLabelValues(errorType, statusCode).Inc()

			// Record retry duration
			duration := time.Since(startTime).Seconds()
			c.metrics.RetryDuration.WithLabelValues(errorType).Observe(duration)

			c.logger.Error(err, "CRD creation failed after max retries",
				"max_attempts", c.retryConfig.MaxAttempts,
				"total_duration", time.Since(startTime),
				"name", rr.Name,
				"namespace", rr.Namespace)

			// Wrap error with retry context (GAP 10: Error Wrapping)
			return &RetryError{
				Attempt:      attempt + 1,
				MaxAttempts:  c.retryConfig.MaxAttempts,
				OriginalErr:  err,
				ErrorType:    "retryable",
				ErrorCode:    GetErrorCode(err),
				ErrorMessage: GetErrorMessage(err),
			}
		}

		// Not the last attempt - retry with exponential backoff
		c.logger.Info("CRD creation failed, retrying...",
			"warning", true,
			"attempt", attempt+1,
			"max_attempts", c.retryConfig.MaxAttempts,
			"backoff", backoff,
			"error", err,
			"name", rr.Name,
			"namespace", rr.Namespace)

		// Sleep with backoff (context-aware for graceful shutdown - GAP 6)
		select {
		case <-time.After(backoff):
			// Continue to next attempt
		case <-ctx.Done():
			return ctx.Err()
		}

		// Exponential backoff (double each time, capped at max)
		backoff *= 2
		if backoff > c.retryConfig.MaxBackoff {
			backoff = c.retryConfig.MaxBackoff
		}
	}

	// Defensive: This should never be reached due to the last attempt check above
	// If we reach here, it indicates a logic error in the retry loop
	return fmt.Errorf("retry logic error: loop completed without explicit return (max_attempts=%d)", c.retryConfig.MaxAttempts)
}

// getErrorTypeString returns a human-readable error type for metrics labels
func getErrorTypeString(err error) string {
	if err == nil {
		return "success"
	}

	statusCode := GetErrorCode(err)
	switch statusCode {
	case 429:
		return "rate_limited"
	case 503:
		return "service_unavailable"
	case 504:
		return "gateway_timeout"
	case 400:
		return "bad_request"
	case 403:
		return "forbidden"
	case 422:
		return "unprocessable_entity"
	case 409:
		return "conflict"
	default:
		errStr := err.Error()
		if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
			return "timeout"
		}
		if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "connection reset") {
			return "network_error"
		}
		return "unknown"
	}
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

	// DD-015: Timestamp-Based CRD Naming for Unique Occurrences
	// Generate CRD name from fingerprint (first 12 chars) + Unix timestamp
	// Example: rr-bd773c9f25ac-1731868032
	// This ensures each signal occurrence creates a unique CRD, even if the
	// same problem reoccurs after a previous remediation completed.
	// See: docs/architecture/decisions/DD-015-timestamp-based-crd-naming.md
	fingerprintPrefix := signal.Fingerprint
	if len(fingerprintPrefix) > 12 {
		fingerprintPrefix = fingerprintPrefix[:12]
	}
	timestamp := time.Now().Unix()
	crdName := fmt.Sprintf("rr-%s-%d", fingerprintPrefix, timestamp)

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
				"kubernaut.ai/created-at": time.Now().UTC().Format(time.RFC3339),
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

			// Storm detection (populated if storm detected)
			IsStorm:           signal.IsStorm,
			StormType:         signal.StormType,
			StormWindow:       signal.StormWindow,
			StormAlertCount:   signal.AlertCount,
			AffectedResources: signal.AffectedResources,

			// Deduplication metadata (initial values)
			// Uses shared type field names (per RO team API Contract Alignment)
			Deduplication: sharedtypes.DeduplicationInfo{
				FirstOccurrence: metav1.NewTime(signal.ReceivedTime),
				LastOccurrence:  metav1.NewTime(signal.ReceivedTime),
				OccurrenceCount: 1,
			},
		},
	}

	// BR-GATEWAY-016: Add storm label if this is an aggregated storm CRD
	// This enables filtering storm CRDs: kubectl get remediationrequests -l kubernaut.ai/storm=true
	if signal.IsStorm {
		rr.Labels["kubernaut.ai/storm"] = "true"
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
			existing, err := c.k8sClient.GetRemediationRequest(ctx, signal.Namespace, crdName)
			if err != nil {
				c.metrics.CRDCreationErrors.WithLabelValues("fetch_existing_failed").Inc()
				return nil, fmt.Errorf("CRD exists but failed to fetch: %w", err)
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
				return nil, fmt.Errorf("failed to create CRD in fallback namespace: %w", err)
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

		return nil, fmt.Errorf("failed to create RemediationRequest CRD: %w", err)
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
		// Record metric for rejected signals (S2: helps alert source authors identify misconfigured alerts)
		if c.metrics != nil && c.metrics.SignalsRejectedTotal != nil {
			reason := "missing_resource_info"
			if len(missingFields) == 1 {
				reason = "missing_resource_" + strings.ToLower(missingFields[0])
			}
			c.metrics.SignalsRejectedTotal.WithLabelValues(reason).Inc()
		}

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

// CreateStormCRD creates a new RemediationRequest CRD for storm aggregation
//
// Parameters:
// - signal: The first signal that triggered storm detection
// - windowID: Storm window ID for tracking aggregated resources
//
// Returns the created CRD or error
func (c *CRDCreator) CreateStormCRD(ctx context.Context, signal *types.NormalizedSignal, windowID string) (*remediationv1alpha1.RemediationRequest, error) {
	// Create base CRD (environment/priority classification removed - SP owns these)
	rr, err := c.CreateRemediationRequest(ctx, signal)
	if err != nil {
		return nil, fmt.Errorf("failed to create storm CRD: %w", err)
	}

	// Storm aggregation metadata is stored in Redis, not in CRD spec
	// The windowID is used to track aggregated resources in Redis
	// This keeps CRD size minimal and avoids etcd size limits

	return rr, nil
}

// UpdateStormCRD updates an existing storm CRD with new alert count
//
// Parameters:
// - crd: The existing storm CRD to update
// - alertCount: New total alert count
//
// # Returns error if update fails
//
// Note: Storm aggregation metadata (alert count, resources) is stored in Redis,
// not in the CRD spec. This method is a no-op for now, as updates happen in Redis.
// The CRD itself remains unchanged to avoid etcd size limits.
func (c *CRDCreator) UpdateStormCRD(ctx context.Context, crd *remediationv1alpha1.RemediationRequest, alertCount int) error {
	// Storm aggregation metadata is stored in Redis, not CRD spec
	// This keeps CRD size minimal and avoids etcd size limits
	// The alert count is tracked in Redis using the storm window ID

	// No CRD update needed - metadata is in Redis
	return nil
}

// truncateLabelValues truncates label values to comply with K8s 63 character limit
// K8s label values must be <= 63 characters
// See: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
func (c *CRDCreator) truncateLabelValues(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}

	truncated := make(map[string]string, len(labels))
	for key, value := range labels {
		if len(value) > 63 {
			truncated[key] = value[:63]
		} else {
			truncated[key] = value
		}
	}
	return truncated
}

// truncateAnnotationValues truncates annotation values to comply with K8s 256KB limit
// K8s annotation values must be < 256KB (262144 bytes)
// We truncate to 262000 bytes to leave room for metadata overhead
// See: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/#syntax-and-character-set
func (c *CRDCreator) truncateAnnotationValues(annotations map[string]string) map[string]string {
	if annotations == nil {
		return nil
	}

	const maxAnnotationSize = 262000 // Slightly less than 256KB to leave room for overhead
	truncated := make(map[string]string, len(annotations))
	for key, value := range annotations {
		if len(value) > maxAnnotationSize {
			truncated[key] = value[:maxAnnotationSize]
		} else {
			truncated[key] = value
		}
	}
	return truncated
}
