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
	"regexp"
	"sort"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/gateway/middleware"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// rfc1123LabelRegexp validates a K8s name/namespace against RFC 1123 label rules.
// Max 253 chars, lowercase alphanumeric + hyphens, must start and end with alphanumeric.
// Intentionally lowercase-only: Prometheus label values like "StatefulSet" (kind names)
// are not valid K8s resource names and are correctly rejected — extractTargetResource
// uses label values as resource *names* for API lookups, not as kind identifiers.
var rfc1123LabelRegexp = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]{0,251}[a-z0-9])?$`)

// PrometheusReservedLabels contains Prometheus-standard label keys that must be
// excluded from dynamic Kubernetes kind resolution. These labels carry scrape
// metadata (job name, instance endpoint, etc.) or location context, not
// Kubernetes resource identifiers. Without filtering, "job" maps to batch/v1
// Job and "namespace" maps to core/v1 Namespace via API discovery, causing
// signals to be dropped or directed to wrong targets. Issues #1045, #1067.
//
// Reference: https://prometheus.io/docs/concepts/jobs_instances/
var PrometheusReservedLabels = map[string]bool{
	"job":       true, // Scrape target name (e.g. "kube-state-metrics")
	"service":   true, // ServiceMonitor target (e.g. "kube-prometheus-stack-kube-state-metrics")
	"instance":  true, // Scrape endpoint (e.g. "10.0.1.45:9090")
	"endpoint":  true, // Port name on ServiceMonitor (e.g. "http")
	"container": true, // Container name (e.g. "payment-api")
	"namespace": true, // Location context, not a resource target (#1067)
}

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
	readerFactory readerFactory // BR-INTEGRATION-065: optional, for remote owner chain resolution
	logger        logr.Logger

	ownerResolutionMetric *prometheus.CounterVec // {kind, outcome} — optional (#1029)
	parseDroppedMetric    *prometheus.CounterVec // {reason} — optional (#1032)
}

// readerFactory abstracts fleet.ReaderFactory to avoid importing pkg/fleet.
// Structurally compatible with fleet.ReaderFactory.
type readerFactory interface {
	ReaderFor(ctx context.Context, clusterID string) (client.Reader, error)
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

// SetOwnerResolutionMetric injects the counter that tracks owner resolution outcomes.
func (a *PrometheusAdapter) SetOwnerResolutionMetric(c *prometheus.CounterVec) {
	a.ownerResolutionMetric = c
}

// SetParseDroppedMetric injects the counter that tracks alerts dropped during batch parsing.
func (a *PrometheusAdapter) SetParseDroppedMetric(c *prometheus.CounterVec) {
	a.parseDroppedMetric = c
}

// SetReaderFactory injects a fleet.ReaderFactory for remote owner chain resolution.
// When set and a signal carries a non-empty clusterID, Parse/ParseBatch construct
// a remote K8sOwnerResolver backed by the reader for that cluster.
func (a *PrometheusAdapter) SetReaderFactory(rf readerFactory) {
	a.readerFactory = rf
}

// resolverForCluster returns the appropriate owner resolver for the given clusterID.
// For local signals (empty clusterID) or when no readerFactory is configured,
// it returns the local resolver. For remote signals, it constructs a
// K8sOwnerResolver backed by a remote client.Reader from the factory.
// On error, it falls back to the local resolver with a logged warning.
func (a *PrometheusAdapter) resolverForCluster(ctx context.Context, clusterID string) types.OwnerResolver {
	if clusterID == "" || a.readerFactory == nil {
		return a.ownerResolver
	}
	reader, err := a.readerFactory.ReaderFor(ctx, clusterID)
	if err != nil {
		a.logger.Error(err, "Failed to obtain remote reader, falling back to local resolver",
			"cluster", clusterID)
		return a.ownerResolver
	}
	return NewK8sOwnerResolver(reader, a.logger)
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
		resource := a.extractAlertResource(ctx, alert)

		// BR-INTEGRATION-065: Extract cluster label from commonLabels (Thanos federation)
		clusterID := webhook.CommonLabels[types.ClusterLabelKey]
		resolver := a.resolverForCluster(ctx, clusterID)

		fingerprint, resolvedResource, err := a.resolveAlertForParse(ctx, i, resource, clusterID, resolver)
		if err != nil {
			lastErr = err
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
			ClusterID:    clusterID,
			SignalName:    alert.Labels["alertname"],
			Severity:     severity,
			Namespace:    resolvedResource.Namespace,
			Resource:     resolvedResource,
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
		signal, ok := a.parseOneAlertInBatch(ctx, i, alert, webhook, rawData)
		if ok {
			signals = append(signals, signal)
		}
	}

	if len(signals) == 0 {
		return nil, fmt.Errorf("all %d alert(s) in batch failed owner resolution", len(webhook.Alerts))
	}

	return signals, nil
}

// parseOneAlertInBatch converts a single AlertManager alert into a
// NormalizedSignal, independent of the rest of the batch (#1032). Returns
// ok=false when owner resolution fails for this alert, so the caller can skip
// it without affecting other alerts in the batch. Extracted from ParseBatch
// to keep its cognitive complexity low.
// extractAlertResource resolves the target ResourceIdentifier (kind/name/namespace)
// for a single alert. Extracted from parseOneAlertInBatch (funlen).
func (a *PrometheusAdapter) extractAlertResource(ctx context.Context, alert AlertManagerAlert) types.ResourceIdentifier {
	ns, nsFound := extractNamespace(alert.Labels)
	kind, name := extractTargetResource(ctx, alert.Labels, ns, a.registry)
	if !nsFound {
		if a.registry != nil && !a.registry.IsNamespacedKind(kind) {
			ns = ""
		} else {
			ns = "default"
		}
	}
	return types.ResourceIdentifier{
		Kind:      kind,
		Name:      name,
		Namespace: ns,
	}
}

// resolveAlertForParse resolves the owner-chain fingerprint for a single
// alert during Parse's "first successfully-resolved alert wins" scan (#451).
// A non-nil error means the caller should skip this alert (e.g., stale alert
// from a deleted pod) and continue to the next one. Extracted from Parse
// (funlen). Distinct from resolveAlertFingerprint (used by ParseBatch, #1032)
// because Parse tracks lastErr instead of a parseDroppedMetric.
func (a *PrometheusAdapter) resolveAlertForParse(
	ctx context.Context,
	alertIndex int,
	resource types.ResourceIdentifier,
	clusterID string,
	resolver types.OwnerResolver,
) (fingerprint string, resolvedResource types.ResourceIdentifier, err error) {
	fingerprint, resolvedResource, err = types.ResolveFingerprintWithCluster(ctx, clusterID, resolver, resource, a.logger)
	if err != nil {
		a.logger.Info("Skipping stale alert in batch",
			"alert", alertIndex, "resource", resource.String(), "error", err)
		if a.ownerResolutionMetric != nil {
			a.ownerResolutionMetric.WithLabelValues(resource.Kind, "failed").Inc()
		}
		return "", types.ResourceIdentifier{}, err
	}
	if a.ownerResolutionMetric != nil {
		a.ownerResolutionMetric.WithLabelValues(resolvedResource.Kind, "success").Inc()
	}
	return fingerprint, resolvedResource, nil
}

// resolveAlertFingerprint resolves the owner-chain fingerprint for a single
// alert's target resource, recording success/failure metrics. ok=false means
// the caller must skip this alert (owner resolution failed) without treating
// it as a batch-level error. Extracted from parseOneAlertInBatch (funlen).
func (a *PrometheusAdapter) resolveAlertFingerprint(
	ctx context.Context,
	alertIndex int,
	resource types.ResourceIdentifier,
	clusterID string,
	resolver types.OwnerResolver,
) (fingerprint string, resolvedResource types.ResourceIdentifier, ok bool) {
	fingerprint, resolvedResource, err := types.ResolveFingerprintWithCluster(ctx, clusterID, resolver, resource, a.logger)
	if err != nil {
		a.logger.Info("Alert failed owner resolution in batch, skipping",
			"alert", alertIndex, "resource", resource.String(), "error", err)
		if a.ownerResolutionMetric != nil {
			a.ownerResolutionMetric.WithLabelValues(resource.Kind, "failed").Inc()
		}
		if a.parseDroppedMetric != nil {
			a.parseDroppedMetric.WithLabelValues("owner_resolution_failed").Inc()
		}
		return "", types.ResourceIdentifier{}, false
	}
	if a.ownerResolutionMetric != nil {
		a.ownerResolutionMetric.WithLabelValues(resolvedResource.Kind, "success").Inc()
	}
	return fingerprint, resolvedResource, true
}

func (a *PrometheusAdapter) parseOneAlertInBatch(
	ctx context.Context,
	alertIndex int,
	alert AlertManagerAlert,
	webhook AlertManagerWebhook,
	rawData []byte,
) (*types.NormalizedSignal, bool) {
	resource := a.extractAlertResource(ctx, alert)

	// BR-INTEGRATION-065: Extract cluster label from commonLabels (Thanos federation)
	clusterID := webhook.CommonLabels[types.ClusterLabelKey]
	resolver := a.resolverForCluster(ctx, clusterID)

	fingerprint, resolvedResource, ok := a.resolveAlertFingerprint(ctx, alertIndex, resource, clusterID, resolver)
	if !ok {
		return nil, false
	}

	labels := MergeLabels(alert.Labels, webhook.CommonLabels)
	annotations := MergeAnnotations(alert.Annotations, webhook.CommonAnnotations)

	severity := alert.Labels["severity"]
	if severity == "" {
		severity = "unknown"
	}

	return &types.NormalizedSignal{
		Fingerprint:  fingerprint,
		ClusterID:    clusterID,
		SignalName:   alert.Labels["alertname"],
		Severity:     severity,
		Namespace:    resolvedResource.Namespace,
		Resource:     resolvedResource,
		Labels:       labels,
		Annotations:  annotations,
		FiringTime:   alert.StartsAt,
		ReceivedTime: time.Now(),
		SourceType:   SourceTypePrometheusAlert,
		Source:       a.GetSourceService(),
		RawPayload:   rawData,
	}, true
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
// - Node "worker-node-1" (no owner) → SHA256(:Node:worker-node-1)
//
// Examples (without OwnerResolver):
// - SHA256(prod:Pod:payment-api-789) — resource from labels, no alertname
//
// Why alertname is excluded:
// - LLM investigates resource state, not signal type
// - Multiple alertnames (KubePodCrashLooping, KubePodNotReady) for the same
//   resource produce redundant RemediationRequests
// - RCA outcome is independent of which alert triggered it

// extractTargetResource determines the Kubernetes target resource (kind + name)
// from Prometheus alert labels using multi-candidate scoring backed by the
// APIResourceRegistry (#1029).
//
// Each label key is matched against discovered APIResource.SingularName values,
// and the candidate with the highest-priority tier (lowest number) wins.
// Prometheus-reserved labels (job, service, instance, endpoint, container) are
// excluded before lookup to prevent scrape metadata from being misinterpreted
// as Kubernetes resource identifiers (#1045).
//
// A non-nil registry is required. Production code always provides one via
// NewAPIResourceRegistry; tests should use NewTestAPIResourceRegistry.
func extractTargetResource(ctx context.Context, labels map[string]string, namespace string, registry *APIResourceRegistry) (kind, name string) {
	if registry == nil {
		return "Unknown", "unknown"
	}

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
		if PrometheusReservedLabels[labelKey] {
			continue
		}
		if !isValidK8sName(labelVal) {
			continue
		}
		k := registry.LabelToKind(labelKey)
		if k == "" {
			continue
		}
		tier := registry.TierForKind(k)
		candidates = append(candidates, candidate{kind: k, name: labelVal, tier: tier})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].tier != candidates[j].tier {
			return candidates[i].tier < candidates[j].tier
		}
		return candidates[i].kind < candidates[j].kind
	})

	for _, c := range candidates {
		gvr, ok := registry.KindToGVR(c.kind)
		if !ok {
			continue
		}
		if registry.CheckExistence(ctx, gvr, namespace, c.name) {
			return c.kind, c.name
		}
	}

	if len(candidates) > 0 {
		return candidates[0].kind, candidates[0].name
	}
	return "Unknown", "unknown"
}

// extractNamespace gets the Kubernetes namespace from labels.
//
// Extraction order (#1029 update — exported_namespace takes precedence):
// 1. exported_namespace label (for federated Prometheus — points to the real namespace)
// 2. namespace label
// 3. ("", false) — caller decides based on resource kind scope (#1371)
//
// In federated setups, the "namespace" label often points to the federation
// namespace (e.g., "monitoring"), while "exported_namespace" contains the
// actual workload namespace (e.g., "production").
func extractNamespace(labels map[string]string) (string, bool) {
	if ns, ok := labels["exported_namespace"]; ok && ns != "" && isValidK8sName(ns) {
		return ns, true
	}
	if ns, ok := labels["namespace"]; ok && isValidK8sName(ns) {
		return ns, true
	}
	return "", false
}

// isValidK8sName returns true if s conforms to RFC 1123 label naming rules
// used by Kubernetes for resource names and namespaces.
func isValidK8sName(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	return rfc1123LabelRegexp.MatchString(s)
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
