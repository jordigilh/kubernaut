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
	"sort"
	"time"

	"github.com/go-logr/logr"
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
	ownerResolver types.OwnerResolver
	registry      *APIResourceRegistry
	logger        logr.Logger
}

// NewPrometheusAdapter creates a new Prometheus adapter.
//
// Parameters:
//   - ownerResolver: If non-nil, resolves Pod→Deployment for owner-chain-based fingerprinting.
//   - registry: If non-nil, provides discovery-backed label-to-kind resolution and
//     tier-based scoring for multi-candidate selection. Replaces static resourceCandidates
//     and LabelFilter (#1029). When nil, falls back to "Unknown" resource kind.
func NewPrometheusAdapter(ownerResolver types.OwnerResolver, registry *APIResourceRegistry, opts ...logr.Logger) *PrometheusAdapter {
	l := logr.Discard()
	if len(opts) > 0 {
		l = opts[0]
	}
	return &PrometheusAdapter{
		ownerResolver: ownerResolver,
		registry:      registry,
		logger:        l.WithName("prometheus-adapter"),
	}
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
// 2. Iterate alerts, skip stale ones where owner resolution fails (Issue #451)
// 3. Determine resource (Pod, Deployment, Node) from labels
// 4. Generate fingerprint: SHA256(namespace:ownerKind:ownerName) — alertname excluded (Issue #63)
// 5. Merge alert labels with common labels
// 6. Convert to NormalizedSignal
//
// Returns:
// - *NormalizedSignal: Unified format for Gateway processing
// - error: Parse errors (invalid JSON, missing required fields, all alerts stale)
func (a *PrometheusAdapter) Parse(ctx context.Context, rawData []byte) (*types.NormalizedSignal, error) {
	const maxPayloadSize = 100 * 1024 // 100KB
	if len(rawData) > maxPayloadSize {
		return nil, fmt.Errorf("payload too large: %d bytes (max %d bytes)", len(rawData), maxPayloadSize)
	}

	var webhook AlertManagerWebhook
	if err := json.Unmarshal(rawData, &webhook); err != nil {
		return nil, fmt.Errorf("malformed JSON payload: %w", err)
	}

	if len(webhook.Alerts) == 0 {
		return nil, errors.New("no alerts in webhook payload")
	}

	// Issue #451: Iterate alerts and use the first one that resolves successfully.
	// AlertManager groups multiple alerts by namespace; stale alerts (deleted pods
	// from previous scenario runs) must be skipped rather than dropping the entire batch.
	var lastErr error
	for i, alert := range webhook.Alerts {
		ns := extractNamespace(alert.Labels)
		kind, name := extractTargetResource(ctx, alert.Labels, ns, a.registry)
		resource := types.ResourceIdentifier{
			Kind:      kind,
			Name:      name,
			Namespace: ns,
		}

		fingerprint, err := types.ResolveFingerprint(ctx, a.ownerResolver, resource, a.logger)
		if err != nil {
			lastErr = err
			a.logger.Info("Skipping stale alert in batch",
				"alert", i, "resource", resource.String(), "error", err)
			continue
		}

		labels := MergeLabels(alert.Labels, webhook.CommonLabels)
		annotations := MergeAnnotations(alert.Annotations, webhook.CommonAnnotations)

		severity := alert.Labels["severity"]
		if severity == "" {
			severity = "unknown"
		}

		return &types.NormalizedSignal{
			Fingerprint:  fingerprint,
			SignalName:    alert.Labels["alertname"],
			Severity:     severity,
			Namespace:    resource.Namespace,
			Resource:     resource,
			Labels:       labels,
			Annotations:  annotations,
			FiringTime:   alert.StartsAt,
			ReceivedTime: time.Now(),
			SourceType:   SourceTypePrometheusAlert,
			Source:       a.GetSourceService(),
			RawPayload:   rawData,
		}, nil
	}

	return nil, fmt.Errorf("dropping signal: all %d alert(s) in batch failed owner resolution; last error: %w", len(webhook.Alerts), lastErr)
}

// ParseBatch converts an AlertManager webhook payload into independent signals,
// one per alert in the batch (#1032). Unlike Parse which returns the first
// successfully resolved alert, ParseBatch processes every alert independently
// so that one alert's resolution failure does not affect other alerts in the batch.
func (a *PrometheusAdapter) ParseBatch(ctx context.Context, rawData []byte) ([]*types.NormalizedSignal, error) {
	const maxPayloadSize = 100 * 1024
	if len(rawData) > maxPayloadSize {
		return nil, fmt.Errorf("payload too large: %d bytes (max %d bytes)", len(rawData), maxPayloadSize)
	}

	var webhook AlertManagerWebhook
	if err := json.Unmarshal(rawData, &webhook); err != nil {
		return nil, fmt.Errorf("malformed JSON payload: %w", err)
	}

	if len(webhook.Alerts) == 0 {
		return nil, errors.New("no alerts in webhook payload")
	}

	var signals []*types.NormalizedSignal
	for i, alert := range webhook.Alerts {
		ns := extractNamespace(alert.Labels)
		kind, name := extractTargetResource(ctx, alert.Labels, ns, a.registry)
		resource := types.ResourceIdentifier{
			Kind:      kind,
			Name:      name,
			Namespace: ns,
		}

		fingerprint, err := types.ResolveFingerprint(ctx, a.ownerResolver, resource, a.logger)
		if err != nil {
			a.logger.Info("Alert failed owner resolution in batch, skipping",
				"alert", i, "resource", resource.String(), "error", err)
			continue
		}

		labels := MergeLabels(alert.Labels, webhook.CommonLabels)
		annotations := MergeAnnotations(alert.Annotations, webhook.CommonAnnotations)

		severity := alert.Labels["severity"]
		if severity == "" {
			severity = "unknown"
		}

		signals = append(signals, &types.NormalizedSignal{
			Fingerprint:  fingerprint,
			SignalName:   alert.Labels["alertname"],
			Severity:     severity,
			Namespace:    resource.Namespace,
			Resource:     resource,
			Labels:       labels,
			Annotations:  annotations,
			FiringTime:   alert.StartsAt,
			ReceivedTime: time.Now(),
			SourceType:   SourceTypePrometheusAlert,
			Source:       a.GetSourceService(),
			RawPayload:   rawData,
		})
	}

	if len(signals) == 0 {
		return nil, fmt.Errorf("all %d alert(s) in batch failed owner resolution", len(webhook.Alerts))
	}

	return signals, nil
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

// resourceCandidate maps a Prometheus alert label key to the Kubernetes resource
// kind it represents. Used by extractTargetResource for priority-ordered scanning.
//
// Deprecated: Superseded by APIResourceRegistry dynamic discovery (#1029).
// Retained as a nil-registry fallback for tests that do not inject a fake discovery client.
// Will be removed in a dedicated cleanup PR once all tests use fake discovery.
type resourceCandidate struct {
	labelKey string
	kind     string
}

// Deprecated: See resourceCandidate deprecation notice above.
var resourceCandidates = []resourceCandidate{
	{"horizontalpodautoscaler", "HorizontalPodAutoscaler"},
	{"poddisruptionbudget", "PodDisruptionBudget"},
	{"persistentvolumeclaim", "PersistentVolumeClaim"},
	{"deployment", "Deployment"},
	{"statefulset", "StatefulSet"},
	{"daemonset", "DaemonSet"},
	{"replicaset", "ReplicaSet"},
	{"node", "Node"},
	{"service", "Service"},
	{"job_name", "Job"},
	{"cronjob", "CronJob"},
	{"pod", "Pod"},
}

// extractTargetResource determines the Kubernetes target resource (kind + name)
// from Prometheus alert labels using multi-candidate scoring backed by the
// APIResourceRegistry (#1029).
//
// When a registry is provided, each label key is matched against discovered
// APIResource.SingularName values, and the candidate with the highest-priority
// tier (lowest number) wins. This replaces the old static resourceCandidates
// list and LabelFilter.
//
// When registry is nil (tests or pre-discovery startup), falls back to the
// static resourceCandidates list for backward compatibility.
func extractTargetResource(ctx context.Context, labels map[string]string, namespace string, registry *APIResourceRegistry) (kind, name string) {
	if registry != nil {
		type candidate struct {
			kind string
			name string
			tier int
		}
		var candidates []candidate
		for labelKey, labelVal := range labels {
			if labelVal == "" {
				continue
			}
			k := registry.LabelToKind(labelKey)
			if k == "" {
				continue
			}
			tier := registry.TierForKind(k)
			candidates = append(candidates, candidate{kind: k, name: labelVal, tier: tier})
		}

		// Sort by tier (ascending), then by kind name (lexicographic) for determinism
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].tier != candidates[j].tier {
				return candidates[i].tier < candidates[j].tier
			}
			return candidates[i].kind < candidates[j].kind
		})

		// Return the first candidate that actually exists (if existence checking is available)
		for _, c := range candidates {
			gvr, ok := registry.KindToGVR(c.kind)
			if !ok {
				continue
			}
			if registry.CheckExistence(ctx, gvr, namespace, c.name) {
				return c.kind, c.name
			}
		}

		// If no candidate passed existence check, return the best-ranked one
		if len(candidates) > 0 {
			return candidates[0].kind, candidates[0].name
		}
		return "Unknown", "unknown"
	}

	for _, c := range resourceCandidates {
		if v, ok := labels[c.labelKey]; ok {
			return c.kind, v
		}
	}
	return "Unknown", "unknown"
}

// extractNamespace gets the Kubernetes namespace from labels.
//
// Extraction order (#1029 update — exported_namespace takes precedence):
// 1. exported_namespace label (for federated Prometheus — points to the real namespace)
// 2. namespace label
// 3. default → "default"
//
// In federated setups, the "namespace" label often points to the federation
// namespace (e.g., "monitoring"), while "exported_namespace" contains the
// actual workload namespace (e.g., "production").
func extractNamespace(labels map[string]string) string {
	if ns, ok := labels["exported_namespace"]; ok && ns != "" {
		return ns
	}
	if ns, ok := labels["namespace"]; ok {
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
