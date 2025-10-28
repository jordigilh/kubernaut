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

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
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
	k8sClient *k8s.Client
	logger    *zap.Logger
	metrics   *metrics.Metrics // Day 9 Phase 6B Option C1: Centralized metrics
}

// NewCRDCreator creates a new CRD creator
func NewCRDCreator(k8sClient *k8s.Client, logger *zap.Logger, metricsInstance *metrics.Metrics) *CRDCreator {
	return &CRDCreator{
		k8sClient: k8sClient,
		logger:    logger,
		metrics:   metricsInstance,
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
//	apiVersion: remediation.kubernaut.io/v1alpha1
//	kind: RemediationRequest
//	metadata:
//	  name: rr-<fingerprint-prefix>
//	  namespace: <signal-namespace>
//	  labels:
//	    kubernaut.io/signal-type: prometheus-alert
//	    kubernaut.io/signal-fingerprint: <full-fingerprint>
//	    kubernaut.io/severity: critical
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
// - priority: Assigned priority (P0/P1/P2) from PriorityEngine
// - environment: Classified environment (prod/staging/dev) from EnvironmentClassifier
//
// Returns:
// - *RemediationRequest: Created CRD with populated fields
// - error: Kubernetes API errors or validation errors
func (c *CRDCreator) CreateRemediationRequest(
	ctx context.Context,
	signal *types.NormalizedSignal,
	priority string,
	environment string,
) (*remediationv1alpha1.RemediationRequest, error) {
	// Generate CRD name from fingerprint (first 16 chars)
	// Example: rr-a1b2c3d4e5f6789
	crdName := fmt.Sprintf("rr-%s", signal.Fingerprint[:16])

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
				"kubernaut.io/signal-type": signal.SourceType,
				// Truncate fingerprint to 63 chars (K8s label value max length)
				"kubernaut.io/signal-fingerprint": signal.Fingerprint[:min(len(signal.Fingerprint), 63)],
				"kubernaut.io/severity":           signal.Severity,
				"kubernaut.io/environment":        environment,
				"kubernaut.io/priority":           priority,
			},
			Annotations: map[string]string{
				// Timestamp for audit trail (RFC3339 format)
				"kubernaut.io/created-at": time.Now().UTC().Format(time.RFC3339),
			},
		},
		Spec: remediationv1alpha1.RemediationRequestSpec{
			// Core signal identification
			SignalFingerprint: signal.Fingerprint,
			SignalName:        signal.AlertName,

			// Classification
			Severity:     signal.Severity,
			Environment:  environment,
			Priority:     priority,
			SignalType:   signal.SourceType,
			SignalSource: signal.Source,
			TargetType:   "kubernetes",

			// Temporal data
			// Fallback: if FiringTime is zero (not set by adapter), use ReceivedTime
			FiringTime:   metav1.NewTime(c.getFiringTime(signal)),
			ReceivedTime: metav1.NewTime(signal.ReceivedTime),

			// Signal metadata
			SignalLabels:      signal.Labels,
			SignalAnnotations: signal.Annotations,

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
			Deduplication: remediationv1alpha1.DeduplicationInfo{
				FirstSeen:       metav1.NewTime(signal.ReceivedTime),
				LastSeen:        metav1.NewTime(signal.ReceivedTime),
				OccurrenceCount: 1,
			},
		},
	}

	// Create CRD in Kubernetes
	if err := c.k8sClient.CreateRemediationRequest(ctx, rr); err != nil {
		// Check if CRD already exists (e.g., Redis TTL expired but K8s CRD still exists)
		// This is normal behavior - Redis TTL is shorter than CRD lifecycle
		if strings.Contains(err.Error(), "already exists") {
			c.logger.Debug("RemediationRequest CRD already exists (Redis TTL expired, CRD persists)",
				zap.String("name", crdName),
				zap.String("namespace", signal.Namespace),
				zap.String("fingerprint", signal.Fingerprint),
			)

			// Fetch existing CRD and return it
			// This allows deduplication metadata to be updated in Redis
			existing, err := c.k8sClient.GetRemediationRequest(ctx, signal.Namespace, crdName)
			if err != nil {
				c.metrics.CRDCreationErrors.WithLabelValues("fetch_existing_failed").Inc()
				return nil, fmt.Errorf("CRD exists but failed to fetch: %w", err)
			}

			c.logger.Info("Reusing existing RemediationRequest CRD (Redis TTL expired)",
				zap.String("name", crdName),
				zap.String("namespace", signal.Namespace),
				zap.String("fingerprint", signal.Fingerprint),
			)

			return existing, nil
		}

		// Check if namespace doesn't exist - fall back to default namespace
		if strings.Contains(err.Error(), "namespaces") && strings.Contains(err.Error(), "not found") {
			c.logger.Warn("Target namespace not found, creating CRD in default namespace as fallback",
				zap.String("original_namespace", signal.Namespace),
				zap.String("fallback_namespace", "default"),
				zap.String("crd_name", crdName))

			// Update namespace to default
			rr.Namespace = "default"

			// Retry creation in default namespace
			if err := c.k8sClient.CreateRemediationRequest(ctx, rr); err != nil {
				c.metrics.CRDCreationErrors.WithLabelValues("fallback_failed").Inc()
				return nil, fmt.Errorf("failed to create CRD in fallback namespace: %w", err)
			}

			// Success with fallback
			c.logger.Info("Created RemediationRequest CRD in default namespace (fallback)",
				zap.String("name", crdName),
				zap.String("namespace", "default"),
				zap.String("fingerprint", signal.Fingerprint),
				zap.String("original_ns", signal.Namespace))

			c.metrics.CRDsCreatedTotal.WithLabelValues(environment, priority).Inc()
			return rr, nil
		}

		// Other errors (not namespace-related, not already-exists)
		c.metrics.CRDCreationErrors.WithLabelValues("k8s_api_error").Inc()

		c.logger.Error("Failed to create RemediationRequest CRD",
			zap.String("name", crdName),
			zap.String("namespace", signal.Namespace),
			zap.String("fingerprint", signal.Fingerprint),
			zap.Error(err))

		return nil, fmt.Errorf("failed to create RemediationRequest CRD: %w", err)
	}

	// Record success metric
	c.metrics.CRDsCreatedTotal.WithLabelValues(environment, priority).Inc()

	// Log creation event
	c.logger.Info("Created RemediationRequest CRD",
		zap.String("name", crdName),
		zap.String("namespace", signal.Namespace),
		zap.String("fingerprint", signal.Fingerprint),
		zap.String("severity", signal.Severity),
		zap.String("environment", environment),
		zap.String("priority", priority),
		zap.String("alertName", signal.AlertName))

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
		c.logger.Debug("FiringTime not set by adapter, using ReceivedTime as fallback",
			zap.String("fingerprint", signal.Fingerprint),
			zap.String("alert_name", signal.AlertName))
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
	providerData := map[string]interface{}{
		"namespace": signal.Namespace,
		"resource": map[string]string{
			"kind":      signal.Resource.Kind,
			"name":      signal.Resource.Name,
			"namespace": signal.Resource.Namespace,
		},
		"labels": signal.Labels,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(providerData)
	if err != nil {
		c.logger.Warn("Failed to marshal provider data, using empty",
			zap.String("fingerprint", signal.Fingerprint),
			zap.Error(err))
		return []byte("{}")
	}

	return jsonData
}

// CreateStormCRD creates a new RemediationRequest CRD for storm aggregation
//
// Parameters:
// - signal: The first signal that triggered storm detection
// - windowID: Storm window ID for tracking aggregated resources
//
// Returns the created CRD or error
func (c *CRDCreator) CreateStormCRD(ctx context.Context, signal *types.NormalizedSignal, windowID string) (*remediationv1alpha1.RemediationRequest, error) {
	// Create base CRD
	rr, err := c.CreateRemediationRequest(ctx, signal, "high", "ai")
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
