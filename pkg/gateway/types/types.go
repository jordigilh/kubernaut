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
// classification, priority assignment, and CRD creation).
//
// Design Decision: Minimal fields for fast processing (target: <50ms p95 latency).
// Downstream services (RemediationProcessing, AIAnalysis) enrich with additional context.
type NormalizedSignal struct {
	// Fingerprint is the unique identifier for deduplication
	// Format: SHA256 hash of "alertname:namespace:kind:name"
	// Example: "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456"
	Fingerprint string

	// AlertName is the human-readable signal name
	// Examples: "HighMemoryUsage", "CrashLoopBackOff", "NodeNotReady"
	AlertName string

	// Severity indicates signal criticality
	// Valid values: "critical", "warning", "info"
	// Used for priority assignment (critical + prod → P0)
	Severity string

	// Namespace is the Kubernetes namespace where the signal originated
	// Used for environment classification (prod/staging/dev)
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
	// Examples: "prometheus-alert", "kubernetes-event", "grafana-alert"
	// Stored in RemediationRequest.Spec.SignalType
	SourceType string

	// Source is the adapter name that processed this signal
	// Examples: "prometheus-adapter", "kubernetes-event-adapter"
	// Stored in RemediationRequest.Spec.SignalSource
	Source string

	// RawPayload is the original unprocessed signal payload
	// Stored as []byte in RemediationRequest.Spec.OriginalPayload for audit trail
	RawPayload json.RawMessage

	// Storm Detection Fields (populated by Gateway after storm detection)
	// These fields are set by the server.go ProcessSignal() method after calling StormDetector.Check()

	// IsStorm indicates if this signal is part of a detected alert storm
	IsStorm bool

	// StormType indicates the storm detection method
	// Values: "rate" (frequency-based) or "pattern" (similar alerts)
	StormType string

	// StormWindow is the time window for storm detection (e.g., "5m", "1m")
	StormWindow string

	// AlertCount is the number of alerts in the detected storm
	AlertCount int

	// AffectedResources is a list of affected resources in an aggregated storm
	// Only populated for aggregated storm signals (after aggregation window completes)
	// Format: []string{"namespace:Pod:name", "namespace:Pod:name2", ...}
	AffectedResources []string
}

// ResourceIdentifier identifies the Kubernetes resource affected by a signal
//
// This struct provides a consistent way to reference Kubernetes resources across
// different signal types (alerts may reference pods, events may reference nodes, etc.)
type ResourceIdentifier struct {
	// Kind is the Kubernetes resource type
	// Examples: "Pod", "Deployment", "Node", "StatefulSet", "Service"
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
