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
// 3. Generate fingerprint for deduplication
// 4. Determine severity, namespace, resource
// 5. Convert to NormalizedSignal format
type PrometheusAdapter struct{}

// NewPrometheusAdapter creates a new Prometheus adapter
func NewPrometheusAdapter() *PrometheusAdapter {
	return &PrometheusAdapter{}
}

// Name returns the adapter identifier
func (a *PrometheusAdapter) Name() string {
	return "prometheus"
}

// GetRoute returns the HTTP route for this adapter
func (a *PrometheusAdapter) GetRoute() string {
	return "/api/v1/signals/prometheus"
}

// ReplayValidator returns header-based replay prevention middleware (BR-GATEWAY-074).
// Prometheus AlertManager webhook sources include X-Timestamp header with
// a fresh Unix epoch timestamp on each request.
func (a *PrometheusAdapter) ReplayValidator(tolerance time.Duration) func(http.Handler) http.Handler {
	return middleware.TimestampValidator(tolerance)
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
// Returns "prometheus-alert" per OpenAPI enum validation and authoritative documentation.
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
// 4. Generate fingerprint: SHA256(alertname:namespace:kind:name)
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

	// Generate fingerprint for deduplication (shared utility)
	// Format: SHA256(alertname:namespace:kind:name)
	// Example: "HighMemoryUsage:prod-payment-service:Pod:payment-api-789"
	fingerprint := types.CalculateFingerprint(alert.Labels["alertname"], resource)

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
		AlertName:    alert.Labels["alertname"],
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
	if signal.AlertName == "" {
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

// calculateFingerprint generates SHA256 hash for deduplication
//
// Fingerprint format: SHA256(alertname:namespace:kind:name)
//
// Examples:
// - "HighMemoryUsage:prod-payment-service:Pod:payment-api-789"
// - "NodeNotReady:default:Node:worker-node-1"
// - "DeploymentReplicasUnavailable:prod-api:Deployment:api-server"
//
// Why SHA256:
// - Collision probability: 2^-256 (astronomically unlikely)
// - Fixed 64-character hex string (easy storage, indexing)
// - Fast: ~1-2μs per hash (negligible overhead)
//
// Returns:
// - string: 64-character hex string (e.g., "a1b2c3d4e5f6...")

// extractResourceKind determines Kubernetes resource type from labels
//
// Detection order (first match wins):
// 1. pod label → "Pod"
// 2. deployment label → "Deployment"
// 3. statefulset label → "StatefulSet"
// 4. daemonset label → "DaemonSet"
// 5. node label → "Node"
// 6. service label → "Service"
// 7. default → "Unknown"
//
// Returns:
// - string: Kubernetes resource kind
func extractResourceKind(labels map[string]string) string {
	if _, ok := labels["pod"]; ok {
		return "Pod"
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
	return "Unknown"
}

// extractResourceName gets the resource instance name from labels
//
// Extraction order (first match wins):
// 1. pod label
// 2. deployment label
// 3. statefulset label
// 4. daemonset label
// 5. node label
// 6. service label
// 7. default → "unknown"
//
// Returns:
// - string: Resource name or "unknown"
func extractResourceName(labels map[string]string) string {
	if pod, ok := labels["pod"]; ok {
		return pod
	}
	if deployment, ok := labels["deployment"]; ok {
		return deployment
	}
	if statefulset, ok := labels["statefulset"]; ok {
		return statefulset
	}
	if daemonset, ok := labels["daemonset"]; ok {
		return daemonset
	}
	if node, ok := labels["node"]; ok {
		return node
	}
	if service, ok := labels["service"]; ok {
		return service
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
