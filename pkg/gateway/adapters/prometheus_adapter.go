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

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/pkg/gateway/middleware"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// PrometheusAdapter handles Prometheus AlertManager webhook format
//
// This adapter parses AlertManager webhook payloads and converts them to
// Gateway's unified NormalizedSignal format.
//
// Supported webhook format: AlertManager v4 (current stable)
// Endpoint: /api/v1/signals/prometheus
//
// Key responsibilities:
// 1. Parse AlertManager JSON payload
// 2. Extract alert fields (labels, annotations, timestamps)
// 3. Generate fingerprint for deduplication (owner-chain based, alertname excluded)
// 4. Determine severity, namespace, resource
// 5. Convert to NormalizedSignal format
//
// Deduplication strategy (Issue #63):
// - alertname is EXCLUDED from fingerprint (LLM investigates resource state, not signal type)
// - OwnerResolver resolves Pod→Deployment for consistent fingerprinting across pod restarts
// - Fallback to resource-level fingerprint (without alertname) when resolution fails
type PrometheusAdapter struct {
	ownerResolver OwnerResolver
}

// NewPrometheusAdapter creates a new Prometheus adapter.
// Optionally accepts an OwnerResolver for owner-chain-based fingerprinting.
// When provided, Pod-level alerts are fingerprinted at the Deployment level.
// When not provided, resource-level fingerprinting is used (alertname still excluded).
func NewPrometheusAdapter(ownerResolver ...OwnerResolver) *PrometheusAdapter {
	adapter := &PrometheusAdapter{}
	if len(ownerResolver) > 0 && ownerResolver[0] != nil {
		adapter.ownerResolver = ownerResolver[0]
	}
	return adapter
}

// Name returns the adapter identifier
func (a *PrometheusAdapter) Name() string {
	return "prometheus"
}

// GetRoute returns the HTTP route for this adapter
func (a *PrometheusAdapter) GetRoute() string {
	return "/api/v1/signals/prometheus"
}

// ReplayValidator returns hybrid replay prevention middleware (BR-GATEWAY-074).
//
// Strategy: Header-first with body-fallback (AlertManagerFreshnessValidator).
//   - If X-Timestamp header is present → strict header-based validation
//     (for direct API clients, tests, external integrations).
//   - If X-Timestamp header is absent → body-based validation using
//     alerts[].startsAt (for real AlertManager webhooks that cannot set
//     custom HTTP headers).
//
// Design Decision (Feb 2026):
// AlertManager's standard webhook format does NOT support custom HTTP headers.
// The previous header-only approach (TimestampValidator) rejected all legitimate
// AlertManager webhooks with "missing timestamp header". This hybrid strategy
// maintains strict validation for direct callers while enabling real AlertManager
// deployments.
func (a *PrometheusAdapter) ReplayValidator(tolerance time.Duration) func(http.Handler) http.Handler {
	return middleware.AlertManagerFreshnessValidator(tolerance)
}

// GetSourceService returns the monitoring system name (BR-GATEWAY-027)
//
// Returns "prometheus" (the monitoring system) instead of "prometheus-adapter" (the adapter name).
// The LLM uses this to select appropriate investigation tools:
// - signal_source="prometheus" → LLM uses Prometheus queries for investigation
//
// This is the SOURCE MONITORING SYSTEM, not the adapter implementation name.
func (a *PrometheusAdapter) GetSourceService() string {
	return "prometheus"
}

// GetSourceType returns the signal type identifier (BR-GATEWAY-027)
//
// Returns "alert" (normalized signal type) per OpenAPI enum validation and authoritative documentation.
// Used for metrics, logging, signal classification, and audit events.
func (a *PrometheusAdapter) GetSourceType() string {
	return SourceTypePrometheusAlert // Prometheus AlertManager signals
}

// Parse converts AlertManager webhook payload to NormalizedSignal
//
// AlertManager webhook format:
//
//	{
//	  "alerts": [{
//	    "labels": {"alertname": "...", "severity": "...", "namespace": "...", "pod": "..."},
//	    "annotations": {"summary": "...", "description": "..."},
//	    "startsAt": "2025-10-09T10:00:00Z",
//	    "endsAt": "0001-01-01T00:00:00Z"
//	  }],
//	  "commonLabels": {"cluster": "..."},
//	  "commonAnnotations": {}
//	}
//
// Processing steps:
// 1. Unmarshal JSON payload
// 2. Extract first alert (AlertManager groups multiple alerts, Gateway processes one at a time)
// 3. Determine resource (Pod, Deployment, Node) from labels
// 4. Generate fingerprint: SHA256(namespace:ownerKind:ownerName) — alertname excluded (Issue #63)
// 5. Merge alert labels with common labels
// 6. Convert to NormalizedSignal
//
// Returns:
// - *NormalizedSignal: Unified format for Gateway processing
// - error: Parse errors (invalid JSON, missing required fields)
func (a *PrometheusAdapter) Parse(ctx context.Context, rawData []byte) (*types.NormalizedSignal, error) {
	// Validate payload size (prevent DoS attacks)
	// Reasonable alert payloads are <100KB; anything >100KB is suspicious
	const maxPayloadSize = 100 * 1024 // 100KB
	if len(rawData) > maxPayloadSize {
		return nil, fmt.Errorf("payload too large: %d bytes (max %d bytes)", len(rawData), maxPayloadSize)
	}

	var webhook AlertManagerWebhook
	if err := json.Unmarshal(rawData, &webhook); err != nil {
		// Return user-friendly error for malformed JSON
		return nil, fmt.Errorf("malformed JSON payload: %w", err)
	}

	if len(webhook.Alerts) == 0 {
		return nil, errors.New("no alerts in webhook payload")
	}

	// Process first alert (AlertManager may send multiple alerts in one webhook)
	// Gateway processes one signal at a time for simpler deduplication and CRD creation
	alert := webhook.Alerts[0]

	// Extract resource from labels
	resource := types.ResourceIdentifier{
		Kind:      extractResourceKind(alert.Labels),
		Name:      extractResourceName(alert.Labels),
		Namespace: extractNamespace(alert.Labels),
	}

	// Generate fingerprint for deduplication (Issue #63: alertname excluded)
	// LLM investigates resource state, not signal type — multiple alertnames for
	// the same resource are redundant work.
	// With OwnerResolver: SHA256(namespace:ownerKind:ownerName) e.g., SHA256(prod:Deployment:payment-api)
	// Without OwnerResolver: SHA256(namespace:kind:name) e.g., SHA256(prod:Pod:payment-api-789)
	var fingerprint string
	if a.ownerResolver != nil {
		ownerKind, ownerName, err := a.ownerResolver.ResolveTopLevelOwner(
			ctx, resource.Namespace, resource.Kind, resource.Name)
		if err == nil && ownerKind != "" && ownerName != "" {
			fingerprint = types.CalculateOwnerFingerprint(types.ResourceIdentifier{
				Namespace: resource.Namespace,
				Kind:      ownerKind,
				Name:      ownerName,
			})
		} else {
			// Fallback: resource-level fingerprint (alertname excluded)
			fingerprint = types.CalculateOwnerFingerprint(resource)
		}
	} else {
		// No OwnerResolver: resource-level fingerprint (alertname excluded)
		fingerprint = types.CalculateOwnerFingerprint(resource)
	}

	// Merge alert-specific labels with common labels
	// Common labels are shared across all alerts in the webhook group
	labels := MergeLabels(alert.Labels, webhook.CommonLabels)
	annotations := MergeAnnotations(alert.Annotations, webhook.CommonAnnotations)

	// BR-GATEWAY-111: Pass through severity without transformation
	// Gateway acts as "dumb pipe" - extract and preserve, never transform
	// Examples: "Sev1" → "Sev1", "P0" → "P0", "critical" → "critical"
	// SignalProcessing Rego will normalize via BR-SP-105
	severity := alert.Labels["severity"]
	if severity == "" {
		severity = "unknown" // Only default if missing entirely (not policy determination)
	}

	return &types.NormalizedSignal{
		Fingerprint:  fingerprint,
		SignalName:    alert.Labels["alertname"],
		Severity:     severity, // BR-GATEWAY-111: Preserve external severity value
		Namespace:    resource.Namespace,
		Resource:     resource,
		Labels:       labels,
		Annotations:  annotations,
		FiringTime:   alert.StartsAt,
		ReceivedTime: time.Now(),
		SourceType:   SourceTypePrometheusAlert, // ✅ Adapter constant (BR-GATEWAY-027)
		Source:       a.GetSourceService(),      // BR-GATEWAY-027: Use monitoring system name, not adapter name
		RawPayload:   rawData,
	}, nil
}

// Validate checks if the parsed signal meets minimum requirements
//
// Validation rules:
// - Fingerprint must be non-empty (64-char SHA256 hex string)
// - AlertName must be non-empty
// - Severity must be non-empty string (any value accepted - BR-GATEWAY-111)
// - Namespace can be empty for cluster-scoped alerts (e.g., node alerts)
//
// BR-GATEWAY-111: Gateway does NOT enforce severity enum validation
// SignalProcessing Rego will normalize any value via BR-SP-105
//
// Returns:
// - error: Validation errors with descriptive messages
func (a *PrometheusAdapter) Validate(signal *types.NormalizedSignal) error {
	if signal.Fingerprint == "" {
		return errors.New("fingerprint is required")
	}
	if signal.SignalName == "" {
		return errors.New("alertName is required")
	}
	// BR-GATEWAY-111: Accept ANY severity value (no enum restriction)
	// Examples: "Sev1", "P0", "critical", "HIGH" all valid
	if signal.Severity == "" {
		return errors.New("severity is required (cannot be empty)")
	}
	// Note: Namespace can be empty for cluster-scoped alerts (e.g., node alerts)
	return nil
}

// GetMetadata returns adapter information
func (a *PrometheusAdapter) GetMetadata() AdapterMetadata {
	return AdapterMetadata{
		Name:                  "prometheus",
		Version:               "1.0",
		Description:           "Handles Prometheus AlertManager webhook notifications",
		SupportedContentTypes: []string{"application/json"},
		RequiredHeaders:       []string{}, // No required headers (optional: X-Prometheus-External-URL)
	}
}

// Fingerprint generation: SHA256 hash for deduplication (Issue #63)
//
// Fingerprint format: SHA256(namespace:ownerKind:ownerName) — alertname EXCLUDED
//
// Examples (with OwnerResolver):
// - Pod "payment-api-789" owned by Deployment "payment-api" → SHA256(prod:Deployment:payment-api)
// - Node "worker-node-1" (no owner) → SHA256(default:Node:worker-node-1)
//
// Examples (without OwnerResolver):
// - SHA256(prod:Pod:payment-api-789) — resource from labels, no alertname
//
// Why alertname is excluded:
// - LLM investigates resource state, not signal type
// - Multiple alertnames (KubePodCrashLooping, KubePodNotReady) for the same
//   resource produce redundant RemediationRequests
// - RCA outcome is independent of which alert triggered it

// extractResourceKind determines Kubernetes resource type from labels.
//
// BR-GATEWAY-184: Specific resource labels are checked before "pod" because
// kube-state-metrics resource-level alerts (kube_hpa_*, kube_deployment_*, etc.)
// include a "pod" label pointing to the metrics exporter, not the affected resource.
//
// Detection order (first match wins):
//  1. horizontalpodautoscaler → "HorizontalPodAutoscaler"
//  2. poddisruptionbudget    → "PodDisruptionBudget"
//  3. persistentvolumeclaim  → "PersistentVolumeClaim"
//  4. deployment             → "Deployment"
//  5. statefulset            → "StatefulSet"
//  6. daemonset              → "DaemonSet"
//  7. node                   → "Node"
//  8. service                → "Service"
//  9. job                    → "Job"
//  10. cronjob               → "CronJob"
//  11. pod                   → "Pod" (last: correct for pod-level metrics like kube_pod_*)
//  12. default               → "Unknown"
func extractResourceKind(labels map[string]string) string {
	if _, ok := labels["horizontalpodautoscaler"]; ok {
		return "HorizontalPodAutoscaler"
	}
	if _, ok := labels["poddisruptionbudget"]; ok {
		return "PodDisruptionBudget"
	}
	if _, ok := labels["persistentvolumeclaim"]; ok {
		return "PersistentVolumeClaim"
	}
	if _, ok := labels["deployment"]; ok {
		return "Deployment"
	}
	if _, ok := labels["statefulset"]; ok {
		return "StatefulSet"
	}
	if _, ok := labels["daemonset"]; ok {
		return "DaemonSet"
	}
	if _, ok := labels["node"]; ok {
		return "Node"
	}
	if _, ok := labels["service"]; ok {
		return "Service"
	}
	if _, ok := labels["job"]; ok {
		return "Job"
	}
	if _, ok := labels["cronjob"]; ok {
		return "CronJob"
	}
	if _, ok := labels["pod"]; ok {
		return "Pod"
	}
	return "Unknown"
}

// extractResourceName gets the resource instance name from labels.
//
// BR-GATEWAY-184: Mirrors extractResourceKind priority order. Specific resource
// labels are checked before "pod" to avoid returning the kube-state-metrics
// exporter pod name instead of the actual affected resource name.
//
// Extraction order (first match wins):
//  1. horizontalpodautoscaler
//  2. poddisruptionbudget
//  3. persistentvolumeclaim
//  4. deployment
//  5. statefulset
//  6. daemonset
//  7. node
//  8. service
//  9. job
//  10. cronjob
//  11. pod (last)
//  12. default → "unknown"
func extractResourceName(labels map[string]string) string {
	if v, ok := labels["horizontalpodautoscaler"]; ok {
		return v
	}
	if v, ok := labels["poddisruptionbudget"]; ok {
		return v
	}
	if v, ok := labels["persistentvolumeclaim"]; ok {
		return v
	}
	if v, ok := labels["deployment"]; ok {
		return v
	}
	if v, ok := labels["statefulset"]; ok {
		return v
	}
	if v, ok := labels["daemonset"]; ok {
		return v
	}
	if v, ok := labels["node"]; ok {
		return v
	}
	if v, ok := labels["service"]; ok {
		return v
	}
	if v, ok := labels["job"]; ok {
		return v
	}
	if v, ok := labels["cronjob"]; ok {
		return v
	}
	if v, ok := labels["pod"]; ok {
		return v
	}
	return "unknown"
}

// extractNamespace gets the Kubernetes namespace from labels
//
// Extraction order:
// 1. namespace label
// 2. exported_namespace label (for federated Prometheus)
// 3. default → "default"
//
// Returns:
// - string: Namespace name or "default"
func extractNamespace(labels map[string]string) string {
	if ns, ok := labels["namespace"]; ok {
		return ns
	}
	if ns, ok := labels["exported_namespace"]; ok {
		return ns
	}
	return "default"
}

// MergeLabels merges multiple label maps with priority (later maps override earlier)
//
// Use case: Merge alert-specific labels with AlertManager common labels
// Example:
//
//	alert.Labels = {"alertname": "HighMemory", "pod": "api-789"}
//	webhook.CommonLabels = {"cluster": "prod", "region": "us-west"}
//	result = {"alertname": "HighMemory", "pod": "api-789", "cluster": "prod", "region": "us-west"}
//
// Returns:
// - map[string]string: Merged labels
func MergeLabels(labelMaps ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, labels := range labelMaps {
		for k, v := range labels {
			merged[k] = v
		}
	}
	return merged
}

// MergeAnnotations merges multiple annotation maps with priority
//
// Use case: Merge alert-specific annotations with AlertManager common annotations
//
// Returns:
// - map[string]string: Merged annotations
func MergeAnnotations(annotationMaps ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, annotations := range annotationMaps {
		for k, v := range annotations {
			merged[k] = v
		}
	}
	return merged
}

// AlertManagerWebhook represents the AlertManager webhook payload structure
//
// This struct follows AlertManager v4 webhook format (current stable).
// Refer to: https://prometheus.io/docs/alerting/latest/configuration/#webhook_config
type AlertManagerWebhook struct {
	// Version is the AlertManager version (e.g., "4")
	Version string `json:"version"`

	// GroupKey is the unique identifier for the alert group
	GroupKey string `json:"groupKey"`

	// TruncatedAlerts is the number of alerts omitted from this webhook
	TruncatedAlerts int `json:"truncatedAlerts"`

	// Status is the alert group status ("firing" or "resolved")
	Status string `json:"status"`

	// Receiver is the AlertManager receiver name that sent this webhook
	Receiver string `json:"receiver"`

	// GroupLabels are the labels that define the alert group
	GroupLabels map[string]string `json:"groupLabels"`

	// CommonLabels are labels shared by all alerts in the group
	CommonLabels map[string]string `json:"commonLabels"`

	// CommonAnnotations are annotations shared by all alerts in the group
	CommonAnnotations map[string]string `json:"commonAnnotations"`

	// ExternalURL is the AlertManager external URL
	ExternalURL string `json:"externalURL"`

	// Alerts is the array of individual alerts in this webhook
	Alerts []AlertManagerAlert `json:"alerts"`
}

// AlertManagerAlert represents a single alert from AlertManager
type AlertManagerAlert struct {
	// Status is the alert status ("firing" or "resolved")
	Status string `json:"status"`

	// Labels contains alert-specific labels
	// Common labels: alertname, severity, namespace, pod, container, node, etc.
	Labels map[string]string `json:"labels"`

	// Annotations contains alert-specific annotations
	// Common annotations: summary, description, runbook_url, dashboard_url
	Annotations map[string]string `json:"annotations"`

	// StartsAt is when the alert started firing
	StartsAt time.Time `json:"startsAt"`

	// EndsAt is when the alert resolved (or 0001-01-01 if still firing)
	EndsAt time.Time `json:"endsAt"`

	// GeneratorURL is the Prometheus expression browser URL for this alert
	GeneratorURL string `json:"generatorURL"`

	// Fingerprint is the AlertManager-generated fingerprint (different from Gateway fingerprint)
	Fingerprint string `json:"fingerprint"`
}
