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

package types

import (
	"encoding/json"
	"time"
)

// NormalizedSignal is the internal representation after adapter parsing
//
// This struct represents the unified format for all signal types (Prometheus alerts,
// Kubernetes events, etc.) after adapter-specific parsing. It contains only the
// data needed for Gateway's processing pipeline (deduplication, storm detection,
// and CRD creation).
//
// Note: Classification and priority assignment removed from Gateway (2025-12-06).
// Signal Processing service now owns these per DD-CATEGORIZATION-001.
//
// Design Decision: Minimal fields for fast processing (target: <50ms p95 latency).
// Downstream services (SignalProcessing, AIAnalysis) enrich with additional context.
type NormalizedSignal struct {
	// Fingerprint is the unique identifier for deduplication
	// Format: SHA256 hash of "alertname:namespace:kind:name"
	// Example: "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456"
	Fingerprint string

	// SignalName is the human-readable signal name
	// Examples: "HighMemoryUsage", "CrashLoopBackOff", "NodeNotReady"
	SignalName string

	// Severity indicates signal criticality
	// Valid values: "critical", "warning", "info"
	// Passed to Signal Processing for priority assignment
	Severity string

	// Namespace is the Kubernetes namespace where the signal originated
	// Passed to Signal Processing for environment classification
	Namespace string

	// Resource identifies the Kubernetes resource affected by the signal
	Resource ResourceIdentifier

	// Labels contains source-specific labels (e.g., Prometheus alert labels)
	// Stored in RemediationRequest.Spec.SignalLabels for downstream processing
	Labels map[string]string

	// Annotations contains source-specific annotations (e.g., descriptions, runbooks)
	// Stored in RemediationRequest.Spec.SignalAnnotations for downstream processing
	Annotations map[string]string

	// FiringTime is when the signal first started firing (from upstream source)
	FiringTime time.Time

	// ReceivedTime is when Gateway received the signal
	// Used for deduplication window calculations (5-minute TTL)
	ReceivedTime time.Time

	// SourceType indicates the signal source
	// Examples: "alert" (normalized signal type for all adapters)
	// Stored in RemediationRequest.Spec.SignalType
	SourceType string

	// Source is the adapter name that processed this signal
	// Examples: "prometheus-adapter", "kubernetes-event-adapter"
	// Stored in RemediationRequest.Spec.SignalSource
	Source string

	// RawPayload is the original unprocessed signal payload
	// Stored as []byte in RemediationRequest.Spec.OriginalPayload for audit trail
	RawPayload json.RawMessage
}

// ResourceIdentifier identifies the Kubernetes resource affected by a signal
//
// This struct provides a consistent way to reference Kubernetes resources across
// different signal types (alerts may reference pods, events may reference nodes, etc.)
type ResourceIdentifier struct {
	// Kind is the Kubernetes resource type
	// Examples: "Pod", "Deployment", "StatefulSet", "DaemonSet", "Node", "Service",
	//           "HorizontalPodAutoscaler", "PodDisruptionBudget", "PersistentVolumeClaim",
	//           "Job", "CronJob"
	Kind string

	// Name is the resource instance name
	// Examples: "payment-api-789", "web-app-deployment", "node-1"
	Name string

	// Namespace is the resource's namespace (empty for cluster-scoped resources like Node)
	Namespace string
}

// String returns a human-readable representation of the resource
// Format: "namespace:Kind:name" or "Kind:name" for cluster-scoped resources
func (r ResourceIdentifier) String() string {
	if r.Namespace == "" {
		return r.Kind + ":" + r.Name
	}
	return r.Namespace + ":" + r.Kind + ":" + r.Name
}
