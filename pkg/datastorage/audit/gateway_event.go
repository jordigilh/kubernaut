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

package audit

// GatewayEventData represents Gateway service event_data structure.
//
// Gateway Service creates audit events for:
// - Signal ingestion (Prometheus AlertManager, Kubernetes Events)
// - Deduplication decisions
// - Storm detection
// - Environment classification
// - Priority assignment
//
// Business Requirement: BR-STORAGE-033-004
type GatewayEventData struct {
	SignalType          string            `json:"signal_type"`                    // "prometheus" or "kubernetes"
	SignalName          string            `json:"signal_name,omitempty"`          // Human-readable signal name (e.g., KubePodCrashLooping)
	Fingerprint         string            `json:"fingerprint,omitempty"`          // Signal fingerprint
	Namespace           string            `json:"namespace,omitempty"`            // K8s namespace
	ResourceType        string            `json:"resource_type,omitempty"`        // "pod", "node", etc.
	ResourceName        string            `json:"resource_name,omitempty"`        // Resource identifier
	Severity            string            `json:"severity,omitempty"`             // "critical", "warning", "info"
	Priority            string            `json:"priority,omitempty"`             // "P0", "P1", "P2", "P3"
	Environment         string            `json:"environment,omitempty"`          // "production", "staging", "dev"
	DeduplicationStatus string            `json:"deduplication_status,omitempty"` // "new", "duplicate", "storm"
	StormDetected       bool              `json:"storm_detected"`                 // Storm flag
	StormID             string            `json:"storm_id,omitempty"`             // Storm identifier
	Labels              map[string]string `json:"labels,omitempty"`               // Additional labels
	SourcePayload       string            `json:"source_payload,omitempty"`       // Base64 original payload
}

// GatewayEventBuilder builds Gateway-specific event data.
//
// Usage:
//
//	eventData, err := audit.NewGatewayEvent("signal.received").
//	    WithSignalType("prometheus").
//	    WithSignalName("HighMemoryUsage").
//	    WithFingerprint("sha256:abc123").
//	    WithNamespace("production").
//	    WithResource("pod", "api-server-123").
//	    WithSeverity("critical").
//	    WithPriority("P0").
//	    WithEnvironment("production").
//	    WithDeduplicationStatus("new").
//	    Build()
//
// Business Requirement: BR-STORAGE-033-005
type GatewayEventBuilder struct {
	*BaseEventBuilder
	gatewayData GatewayEventData
}

// NewGatewayEvent creates a new Gateway event builder.
//
// Parameters:
// - eventType: Specific Gateway event type (e.g., "signal.received", "signal.deduplicated")
//
// Example:
//
//	builder := audit.NewGatewayEvent("signal.received")
func NewGatewayEvent(eventType string) *GatewayEventBuilder {
	return &GatewayEventBuilder{
		BaseEventBuilder: NewEventBuilder("gateway", eventType),
		gatewayData:      GatewayEventData{},
	}
}

// WithSignalType sets the signal type (prometheus/kubernetes).
//
// Valid values:
// - "prometheus": Prometheus AlertManager webhook
// - "kubernetes": Kubernetes Event API
//
// Example:
//
//	builder.WithSignalType("prometheus")
//
// Business Requirement: BR-STORAGE-033-005
func (b *GatewayEventBuilder) WithSignalType(signalType string) *GatewayEventBuilder {
	b.gatewayData.SignalType = signalType
	return b
}

// WithSignalName sets the human-readable signal name.
//
// Examples: "KubePodCrashLooping", "HighMemoryUsage", "NodeNotReady"
//
// Example:
//
//	builder.WithSignalName("HighMemoryUsage")
func (b *GatewayEventBuilder) WithSignalName(signalName string) *GatewayEventBuilder {
	b.gatewayData.SignalName = signalName
	return b
}

// WithFingerprint sets the signal fingerprint.
//
// Fingerprint is used for deduplication.
//
// Example:
//
//	builder.WithFingerprint("sha256:abc123def456")
func (b *GatewayEventBuilder) WithFingerprint(fingerprint string) *GatewayEventBuilder {
	b.gatewayData.Fingerprint = fingerprint
	return b
}

// WithNamespace sets the Kubernetes namespace.
//
// Example:
//
//	builder.WithNamespace("production")
func (b *GatewayEventBuilder) WithNamespace(namespace string) *GatewayEventBuilder {
	b.gatewayData.Namespace = namespace
	return b
}

// WithResource sets the resource type and name.
//
// Parameters:
// - resourceType: Resource kind (e.g., "pod", "node", "deployment")
// - resourceName: Resource identifier (e.g., "api-server-123")
//
// Example:
//
//	builder.WithResource("pod", "api-server-xyz-123")
func (b *GatewayEventBuilder) WithResource(resourceType, resourceName string) *GatewayEventBuilder {
	b.gatewayData.ResourceType = resourceType
	b.gatewayData.ResourceName = resourceName
	return b
}

// WithSeverity sets the severity level.
//
// Common values: "critical", "warning", "info"
//
// Example:
//
//	builder.WithSeverity("critical")
func (b *GatewayEventBuilder) WithSeverity(severity string) *GatewayEventBuilder {
	b.gatewayData.Severity = severity
	return b
}

// WithPriority sets the priority level.
//
// Priority levels: "P0" (critical), "P1" (high), "P2" (medium), "P3" (low)
//
// Example:
//
//	builder.WithPriority("P0")
func (b *GatewayEventBuilder) WithPriority(priority string) *GatewayEventBuilder {
	b.gatewayData.Priority = priority
	return b
}

// WithEnvironment sets the environment classification.
//
// Common values: "production", "staging", "development"
//
// Example:
//
//	builder.WithEnvironment("production")
func (b *GatewayEventBuilder) WithEnvironment(environment string) *GatewayEventBuilder {
	b.gatewayData.Environment = environment
	return b
}

// WithDeduplicationStatus sets the deduplication status.
//
// Valid values:
// - "new": First occurrence of signal
// - "duplicate": Signal is a duplicate
// - "storm": Signal is part of a detected storm
//
// Example:
//
//	builder.WithDeduplicationStatus("duplicate")
//
// Business Requirement: BR-STORAGE-033-006
func (b *GatewayEventBuilder) WithDeduplicationStatus(status string) *GatewayEventBuilder {
	b.gatewayData.DeduplicationStatus = status
	return b
}

// WithStorm marks event as part of a storm.
//
// Parameters:
// - stormID: Unique storm identifier (e.g., "storm-2025-11-18-001")
//
// Example:
//
//	builder.WithStorm("storm-2025-11-18-001")
//
// Business Requirement: BR-STORAGE-033-006
func (b *GatewayEventBuilder) WithStorm(stormID string) *GatewayEventBuilder {
	b.gatewayData.StormDetected = true
	b.gatewayData.StormID = stormID
	return b
}

// WithLabels adds additional labels.
//
// Labels provide flexible metadata for signal context.
//
// Example:
//
//	builder.WithLabels(map[string]string{
//	    "app": "api-server",
//	    "tier": "backend",
//	    "cluster": "prod-us-west-2",
//	})
func (b *GatewayEventBuilder) WithLabels(labels map[string]string) *GatewayEventBuilder {
	b.gatewayData.Labels = labels
	return b
}

// WithSourcePayload stores the original payload (base64 encoded).
//
// Use this to preserve the original signal payload for debugging.
//
// Example:
//
//	encodedPayload := base64.StdEncoding.EncodeToString(originalPayload)
//	builder.WithSourcePayload(encodedPayload)
func (b *GatewayEventBuilder) WithSourcePayload(payload string) *GatewayEventBuilder {
	b.gatewayData.SourcePayload = payload
	return b
}

// Build constructs the final event_data JSONB.
//
// Returns:
// - map[string]interface{}: JSONB-ready event data
// - error: JSON marshaling error (should not occur for valid inputs)
//
// Example:
//
//	eventData, err := builder.Build()
//	if err != nil {
//	    return fmt.Errorf("failed to build Gateway event: %w", err)
//	}
func (b *GatewayEventBuilder) Build() (map[string]interface{}, error) {
	// Add Gateway-specific data to base event
	b.WithCustomField("gateway", b.gatewayData)
	return b.BaseEventBuilder.Build()
}
