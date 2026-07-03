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
	sharedK8s "github.com/jordigilh/kubernaut/pkg/shared/k8s"
)

// RetryObserver is notified on each failed CRD creation retry attempt.
// BR-GATEWAY-058: Every retry attempt MUST be observed for audit compliance.
// BR-GATEWAY-113: Decouples retry observation from CRD creation logic.
type RetryObserver interface {
	OnRetryAttempt(ctx context.Context, signal *types.NormalizedSignal, attempt int, err error)
}

// CRDCreator converts NormalizedSignal to RemediationRequest CRD
//
// This component is responsible for:
// 1. Generating unique CRD names (rr-{fingerprint[:12]}-{uuid[:8]})
// 2. Populating CRD spec from signal data
// 3. Adding required labels for querying/filtering
// 4. Creating CRD in Kubernetes via API
// 5. Recording metrics (success/failure)
//
// CRD naming (DD-AUDIT-CORRELATION-002):
// - Format: rr-{fingerprint[:12]}-{uuid[:8]}
// - Example: rr-b157a3a9e42f-1c2b5576
// - Reason: Human-readable fingerprint prefix + UUID suffix for zero collision risk
type CRDCreator struct {
	k8sClient           k8s.ClientInterface   // TDD GREEN: Interface supports circuit breaker (BR-GATEWAY-093)
	logger              logr.Logger           // DD-005: logr.Logger for unified logging
	metrics             *metrics.Metrics      // Day 9 Phase 6B Option C1: Centralized metrics
	retryConfig         *config.RetrySettings // BR-GATEWAY-111: Retry configuration
	clock               Clock                 // Clock for time-dependent operations (enables fast testing)
	retryObserver       RetryObserver         // BR-GATEWAY-058: Notified on each retry attempt
	controllerNamespace string                // ADR-057: Namespace where CRDs are created
}

// NewCRDCreator creates a new CRD creator
// BR-GATEWAY-111: Accepts retry configuration for K8s API retry logic
// BR-GATEWAY-058: Accepts RetryObserver for per-attempt audit compliance
// DD-005: Uses logr.Logger for unified logging
func NewCRDCreator(k8sClient k8s.ClientInterface, logger logr.Logger, metricsInstance *metrics.Metrics, retryConfig *config.RetrySettings, retryObserver RetryObserver, controllerNamespace string) *CRDCreator {
	return NewCRDCreatorWithClock(k8sClient, logger, metricsInstance, retryConfig, retryObserver, controllerNamespace, nil)
}

// NewCRDCreatorWithClock creates a new CRD creator with a custom clock
// This variant enables testing with MockClock for time-dependent behavior
//
// TDD GREEN: Accepts ClientInterface to support circuit breaker (BR-GATEWAY-093)
func NewCRDCreatorWithClock(k8sClient k8s.ClientInterface, logger logr.Logger, metricsInstance *metrics.Metrics, retryConfig *config.RetrySettings, retryObserver RetryObserver, controllerNamespace string, clock Clock) *CRDCreator {
	// Metrics are mandatory for observability
	if metricsInstance == nil {
		panic("metrics cannot be nil: metrics are mandatory for observability")
	}

	// BR-GATEWAY-058: Retry observer is mandatory for audit compliance
	if retryObserver == nil {
		panic("retryObserver cannot be nil: retry observation is mandatory per BR-GATEWAY-058")
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
		k8sClient:           k8sClient,
		logger:              logger,
		metrics:             metricsInstance,
		retryConfig:         retryConfig,
		clock:               clock,
		retryObserver:       retryObserver,
		controllerNamespace: controllerNamespace,
	}
}

// CreateRemediationRequest creates a RemediationRequest CRD from a signal
//
// This method:
// 1. Generates CRD name from fingerprint
// 2. Populates metadata (labels, annotations)
// 3. Populates spec (signal data, timestamps)
// 4. Creates CRD in Kubernetes
// 5. Records success/failure metrics
// 6. Logs creation event
//
// CRD structure:
//
//	apiVersion: remediation.kubernaut.ai/v1alpha1
//	kind: RemediationRequest
//	metadata:
//	  name: rr-<fingerprint[:12]>-<uuid[:8]>
//	  namespace: <controller-namespace>
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
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - signal: Normalized signal from adapter
//
// Note: Environment and Priority classification removed from Gateway (2025-12-06)
// These are now owned by Signal Processing service per DD-CATEGORIZATION-001.
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
			"signal_name", signal.SignalName,
			"error", err)
		return nil, err
	}

	crdName := generateCRDName(signal.Fingerprint)
	rr := c.buildRemediationRequestCRD(crdName, signal)

	// Create CRD in Kubernetes with retry logic
	// BR-GATEWAY-112: Retry logic for transient K8s API errors
	if err := c.createCRDWithRetry(ctx, rr, signal); err != nil {
		// Check if CRD already exists (e.g., Redis TTL expired but K8s CRD still exists)
		// This is normal behavior - Redis TTL is shorter than CRD lifecycle
		if k8serrors.IsAlreadyExists(err) {
			return c.recoverExistingCRDOnConflict(ctx, crdName, signal, startTime)
		}
		return nil, c.buildCreationFailureError(err, crdName, signal, startTime)
	}

	// Record success metric
	// Note: Metric labels changed from (environment, priority) to (sourceType, "created")
	// since environment/priority classification moved to SP per DD-CATEGORIZATION-001
	c.metrics.CRDsCreatedTotal.WithLabelValues(signal.Source, "created").Inc()

	// Log creation event
	c.logger.Info("Created RemediationRequest CRD",
		"name", crdName,
		"namespace", c.controllerNamespace,
		"fingerprint", signal.Fingerprint,
		"severity", signal.Severity,
		"signal_name", signal.SignalName)

	return rr, nil
}

// generateCRDName builds a unique CRD name per DD-AUDIT-CORRELATION-002:
// rr-{fingerprint-prefix[:12]}-{uuid-suffix[:8]}. The fingerprint prefix gives
// human-readable correlation for debugging; the UUID suffix guarantees zero
// collision risk across occurrences of the same signal. Supersedes DD-015
// (timestamp-based naming). Extracted from CreateRemediationRequest (funlen).
func generateCRDName(fingerprint string) string {
	fingerprintPrefix := fingerprint
	if len(fingerprintPrefix) > 12 {
		fingerprintPrefix = fingerprintPrefix[:12]
	}
	shortUUID := uuid.New().String()[:8]
	return fmt.Sprintf("rr-%s-%s", fingerprintPrefix, shortUUID)
}

// buildRemediationRequestCRD constructs the RemediationRequest object (not
// yet persisted) from a normalized signal. ADR-057: created in the
// controller namespace, not the signal's namespace. Extracted from
// CreateRemediationRequest (funlen).
func (c *CRDCreator) buildRemediationRequestCRD(crdName string, signal *types.NormalizedSignal) *remediationv1alpha1.RemediationRequest {
	return &remediationv1alpha1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crdName,
			Namespace: c.controllerNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "gateway-service",
				"app.kubernetes.io/component":  "remediation",
			},
			Annotations: map[string]string{
				// Timestamp for audit trail (RFC3339 format)
				"kubernaut.ai/created-at": c.clock.Now().UTC().Format(time.RFC3339),
			},
		},
		Spec: remediationv1alpha1.RemediationRequestSpec{
			// Core signal identification
			SignalFingerprint: signal.Fingerprint,
			SignalName:        signal.SignalName,

			// Classification (environment/priority removed - SP owns these now)
			Severity:     signal.Severity,
			SignalType:   signal.SourceType,
			SignalSource: signal.Source,
			TargetType:   "kubernetes",

			// BR-INTEGRATION-065: Multi-cluster federation
			ClusterID: signal.ClusterID,

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
			// Issue #96: string type eliminates unnecessary base64 encoding layer
			ProviderData: string(c.buildProviderData(signal)),

			// Original payload for audit trail
			// Issue #96: string type preserves raw JSON text without base64 encoding
			OriginalPayload: string(signal.RawPayload),

			// DD-GATEWAY-011: Deduplication lives in Status (initialized by StatusUpdater)
		},
	}
}

// recoverExistingCRDOnConflict handles the AlreadyExists race (e.g., Redis TTL
// expired but the K8s CRD still exists — normal behavior since Redis TTL is
// shorter than CRD lifecycle): fetch and return the existing CRD instead of
// treating this as a failure. Retries the fetch up to 3 times with linear
// backoff (50ms/100ms/200ms) to absorb any residual read-after-write lag.
// Extracted from CreateRemediationRequest (funlen).
func (c *CRDCreator) recoverExistingCRDOnConflict(
	ctx context.Context,
	crdName string,
	signal *types.NormalizedSignal,
	startTime time.Time,
) (*remediationv1alpha1.RemediationRequest, error) {
	c.logger.V(1).Info("RemediationRequest CRD already exists (Redis TTL expired, CRD persists)",
		"name", crdName,
		"namespace", c.controllerNamespace,
		"fingerprint", signal.Fingerprint,
	)

	var existing *remediationv1alpha1.RemediationRequest
	var fetchErr error
	const maxFetchAttempts = 3
	for attempt := 1; attempt <= maxFetchAttempts; attempt++ {
		existing, fetchErr = c.k8sClient.GetRemediationRequest(ctx, c.controllerNamespace, crdName)
		if fetchErr == nil {
			break // Successfully fetched
		}

		if attempt < maxFetchAttempts {
			// Exponential backoff: 50ms, 100ms, 200ms
			backoffDelay := time.Duration(50*attempt) * time.Millisecond
			c.logger.V(1).Info("CRD fetch failed, retrying after backoff",
				"name", crdName,
				"attempt", attempt,
				"backoff_ms", backoffDelay.Milliseconds(),
				"error", fetchErr)
			time.Sleep(backoffDelay)
		}
	}

	if fetchErr != nil {
		c.metrics.CRDCreationErrors.WithLabelValues("fetch_existing_failed").Inc()
		return nil, NewCRDCreationError(CRDCreationErrorParams{
			Fingerprint: signal.Fingerprint,
			Namespace:   c.controllerNamespace,
			CRDName:     crdName,
			SignalType:  signal.SourceType,
			SignalName:  signal.SignalName,
			Attempts:    maxFetchAttempts,
			StartTime:   startTime,
			Err:         fmt.Errorf("CRD exists but failed to fetch after %d attempts: %w", maxFetchAttempts, fetchErr),
		})
	}

	c.logger.Info("Reusing existing RemediationRequest CRD (Redis TTL expired)",
		"name", crdName,
		"namespace", c.controllerNamespace,
		"fingerprint", signal.Fingerprint,
	)

	return existing, nil
}

// buildCreationFailureError records metrics/logs and builds the structured
// error for a genuine (non-AlreadyExists) CRD creation failure.
//
// Note: Namespace-not-found errors are no longer handled via fallback.
// ADR-053 scope validation rejects signals to unmanaged namespaces upstream,
// so namespace-not-found during CRD creation is now a genuine error.
// See: DD-GATEWAY-007 (DEPRECATED February 2026)
// Extracted from CreateRemediationRequest (funlen).
func (c *CRDCreator) buildCreationFailureError(
	err error,
	crdName string,
	signal *types.NormalizedSignal,
	startTime time.Time,
) error {
	c.metrics.CRDCreationErrors.WithLabelValues("k8s_api_error").Inc()

	c.logger.Error(err, "Failed to create RemediationRequest CRD",
		"name", crdName,
		"namespace", c.controllerNamespace,
		"fingerprint", signal.Fingerprint)

	return NewCRDCreationError(CRDCreationErrorParams{
		Fingerprint: signal.Fingerprint,
		Namespace:   c.controllerNamespace,
		CRDName:     crdName,
		SignalType:  signal.SourceType,
		SignalName:  signal.SignalName,
		Attempts:    c.retryConfig.MaxAttempts,
		StartTime:   startTime,
		Err:         err,
	})
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
			"signal_name", signal.SignalName)
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

	if signal.Resource.Kind == "" || strings.EqualFold(signal.Resource.Kind, "unknown") {
		missingFields = append(missingFields, "Kind")
	}
	if signal.Resource.Name == "" || strings.EqualFold(signal.Resource.Name, "unknown") {
		missingFields = append(missingFields, "Name")
	}

	if len(missingFields) > 0 {
		return fmt.Errorf("resource validation failed: missing or unresolved fields [%s] - V1.0 requires valid Kubernetes resource info",
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

// Note: createCRDWithRetry and its error-classification/retry-handler methods
// (handleAlreadyExistsError, shouldRetryError, logSuccessAfterRetry,
// wrapRetryExhaustedError, waitWithBackoff, getErrorTypeString) live in
// crd_creator_retry.go (Wave 6 GREEN: file-size remediation).
