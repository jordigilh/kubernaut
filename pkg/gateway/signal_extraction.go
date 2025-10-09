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

package gateway

// signal_extraction.go provides helper functions to extract structured metadata
// from provider-specific signal payloads (Prometheus, Kubernetes events, etc.)
//
// These functions support Phase 1 enhancement: populating signalLabels and
// signalAnnotations fields in RemediationRequest CRD for downstream processing
// without requiring payload parsing.

import (
	"fmt"
)

// ExtractLabels extracts labels from an alert payload
// For Prometheus alerts: returns alert.Labels map
// For Kubernetes events: would extract event-specific labels
// For AWS CloudWatch: would extract alarm tags as labels
//
// Phase 1: Supports Prometheus alerts only (V1 scope)
// Future: Extend for other signal types (Kubernetes events, CloudWatch alarms)
func ExtractLabels(alert *AlertManagerAlert) map[string]string {
	if alert == nil {
		return make(map[string]string)
	}

	// For Prometheus alerts, labels are directly available
	// Deep copy to prevent modification of original
	labels := make(map[string]string, len(alert.Labels))
	for k, v := range alert.Labels {
		labels[k] = v
	}

	return labels
}

// ExtractAnnotations extracts annotations from an alert payload
// For Prometheus alerts: returns alert.Annotations map
// For Kubernetes events: would extract event message/reason as annotations
// For AWS CloudWatch: would extract alarm description as annotations
//
// Phase 1: Supports Prometheus alerts only (V1 scope)
// Future: Extend for other signal types
func ExtractAnnotations(alert *AlertManagerAlert) map[string]string {
	if alert == nil {
		return make(map[string]string)
	}

	// For Prometheus alerts, annotations are directly available
	// Deep copy to prevent modification of original
	annotations := make(map[string]string, len(alert.Annotations))
	for k, v := range alert.Annotations {
		annotations[k] = v
	}

	return annotations
}

// extractLabelsWithFallback extracts labels with fallback defaults
// Used when labels map is nil or missing critical keys
//
// Ensures RemediationRequest always has minimum required labels:
// - alertname (signal identifier)
// - namespace (target namespace for Kubernetes signals)
// - severity (signal severity level)
func extractLabelsWithFallback(alert *AlertManagerAlert) map[string]string {
	labels := ExtractLabels(alert)

	// Fallback: Ensure alertname exists
	if _, ok := labels["alertname"]; !ok {
		labels["alertname"] = "unknown"
	}

	// Fallback: Ensure namespace exists for Kubernetes signals
	if _, ok := labels["namespace"]; !ok {
		// Check alternative namespace fields
		if ns, ok := labels["exported_namespace"]; ok {
			labels["namespace"] = ns
		} else {
			labels["namespace"] = "default"
		}
	}

	// Fallback: Ensure severity exists
	if _, ok := labels["severity"]; !ok {
		labels["severity"] = "info"
	}

	return labels
}

// extractAnnotationsWithFallback extracts annotations with fallback defaults
// Used when annotations map is nil or missing critical keys
//
// Ensures RemediationRequest always has minimum required annotations:
// - summary (brief description)
// - description (detailed description)
func extractAnnotationsWithFallback(alert *AlertManagerAlert) map[string]string {
	annotations := ExtractAnnotations(alert)

	// Fallback: Ensure summary exists
	if _, ok := annotations["summary"]; !ok {
		// Generate summary from alertname if missing
		if alertName, ok := alert.Labels["alertname"]; ok {
			annotations["summary"] = fmt.Sprintf("Alert: %s", alertName)
		} else {
			annotations["summary"] = "Unknown alert"
		}
	}

	// Fallback: Ensure description exists
	if _, ok := annotations["description"]; !ok {
		// Use summary as description if missing
		if summary, ok := annotations["summary"]; ok {
			annotations["description"] = summary
		} else {
			annotations["description"] = "No description available"
		}
	}

	return annotations
}

// SanitizeLabels sanitizes label keys and values to ensure Kubernetes compliance
// Label keys must be valid DNS subdomain names (RFC 1123)
// Label values must be valid label values (63 chars max, alphanumeric + dash/underscore/dot)
//
// Used to ensure extracted labels meet Kubernetes label requirements
func SanitizeLabels(labels map[string]string) map[string]string {
	sanitized := make(map[string]string, len(labels))

	for k, v := range labels {
		// Skip empty keys
		if k == "" {
			continue
		}

		// Truncate long values (Kubernetes label value max: 63 chars)
		if len(v) > 63 {
			v = v[:63]
		}

		sanitized[k] = v
	}

	return sanitized
}

// SanitizeAnnotations sanitizes annotation keys and values
// Annotations have more relaxed requirements than labels:
// - Keys: valid DNS subdomain prefix + optional DNS subdomain name
// - Values: no size limit in API, but practical limit for storage
//
// Used to ensure extracted annotations are safe for storage
func SanitizeAnnotations(annotations map[string]string) map[string]string {
	sanitized := make(map[string]string, len(annotations))

	for k, v := range annotations {
		// Skip empty keys
		if k == "" {
			continue
		}

		// Practical size limit for annotations: 256KB per annotation
		// (Kubernetes etcd has ~1.5MB total resource size limit)
		const maxAnnotationSize = 256 * 1024
		if len(v) > maxAnnotationSize {
			v = v[:maxAnnotationSize] + "... (truncated)"
		}

		sanitized[k] = v
	}

	return sanitized
}

// MergeLabels merges multiple label maps with priority (later maps override earlier)
// Used to combine signal labels with common labels from AlertManager webhook
//
// Example: Merge alert-specific labels with group labels from webhook
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
// Used to combine signal annotations with common annotations from AlertManager webhook
func MergeAnnotations(annotationMaps ...map[string]string) map[string]string {
	merged := make(map[string]string)

	for _, annotations := range annotationMaps {
		for k, v := range annotations {
			merged[k] = v
		}
	}

	return merged
}

// ExtractSignalMetadata extracts both labels and annotations from an alert
// with fallback defaults and sanitization
//
// This is the main function Gateway Service should use to populate
// RemediationRequest.Spec.SignalLabels and SignalAnnotations fields
//
// Returns:
// - labels: Sanitized labels with fallback defaults
// - annotations: Sanitized annotations with fallback defaults
func ExtractSignalMetadata(alert *AlertManagerAlert, webhook *AlertManagerWebhook) (labels, annotations map[string]string) {
	// Extract alert-specific labels and annotations
	alertLabels := extractLabelsWithFallback(alert)
	alertAnnotations := extractAnnotationsWithFallback(alert)

	// Merge with common labels/annotations from webhook (if available)
	if webhook != nil {
		alertLabels = MergeLabels(webhook.CommonLabels, webhook.GroupLabels, alertLabels)
		alertAnnotations = MergeAnnotations(webhook.CommonAnnotations, alertAnnotations)
	}

	// Sanitize for Kubernetes compliance
	labels = SanitizeLabels(alertLabels)
	annotations = SanitizeAnnotations(alertAnnotations)

	return labels, annotations
}
