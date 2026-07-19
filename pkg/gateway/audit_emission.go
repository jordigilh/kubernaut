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

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/circuitbreaker" // BR-GATEWAY-093: Circuit breaker detection

	"github.com/jordigilh/kubernaut/pkg/audit"                       // DD-AUDIT-003: Audit integration
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client" // Ogen generated audit types
	sharedaudit "github.com/jordigilh/kubernaut/pkg/shared/audit"    // BR-AUDIT-005 Gap #7: Standardized error details
	"github.com/jordigilh/kubernaut/pkg/shared/auth"                 // BR-GATEWAY-036/037: Shared auth middleware

	// ADR-052 Addendum 001: Exponential backoff with jitter
	// Issue #753: Dedicated health server
	// Issue #756: FileWatcher for cert rotation
	// Issue #493/#678: Conditional TLS

	// BR-GATEWAY-190: Lease resources for distributed locking

	// BR-GATEWAY-036/037: K8s clientset for TokenReview/SAR

	// ADR-068: Federated scope checking factory

	// BR-109: Request ID middleware

	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	// BR-HTTP-015: Shared CORS library
	// DD-005: Shared sanitization library
	// BR-SCOPE-002: Resource scope management
)

// =============================================================================
// DD-AUDIT-003: Audit Event Emission (P0 Compliance)
// =============================================================================

// extractRRReconstructionFields sanitizes signal fields for audit event storage
//
// ========================================
// RR RECONSTRUCTION FIELD SANITIZATION (REFACTOR PHASE)
// BR-AUDIT-005: Ensure PostgreSQL JSONB compatibility
// ========================================
//
// WHY THIS HELPER?
// - ✅ Eliminates code duplication (used by signal.received AND signal.deduplicated)
// - ✅ PostgreSQL JSONB prefers empty maps over nil values
// - ✅ Graceful handling of synthetic signals without RawPayload
// - ✅ Consistent nil handling across all Gateway audit events
//
// RETURNS:
// - labels: non-nil map[string]string (empty map if nil)
// - annotations: non-nil map[string]string (empty map if nil)
// - originalPayload: interface{} (nil if signal.RawPayload is nil)
// ========================================
func extractRRReconstructionFields(signal *types.NormalizedSignal) (
	labels map[string]string,
	annotations map[string]string,
	originalPayload map[string]interface{},
) {
	// Gap #2: Signal labels (ensure non-nil for JSONB)
	labels = signal.Labels
	if labels == nil {
		labels = make(map[string]string)
	}

	// Gap #3: Signal annotations (ensure non-nil for JSONB)
	annotations = signal.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Gap #1: Original payload (nil OK for synthetic signals)
	if len(signal.RawPayload) > 0 {
		// Unmarshal json.RawMessage to map[string]interface{}
		if err := json.Unmarshal(signal.RawPayload, &originalPayload); err != nil {
			// If unmarshal fails, leave originalPayload nil (defensive)
			originalPayload = nil
		}
	}

	return labels, annotations, originalPayload
}

// EmitConfigReloadAudit emits 'gateway.config.reloaded' (reloadErr == nil) or
// 'gateway.config.rejected' (reloadErr != nil) for a hot-reloadable Gateway
// component (GAP-11, Issue #1505 — SOC2 CC7.2 change management, FedRAMP AU-12).
//
// component identifies which hot-reloadable setting was affected, e.g.
// "log_level" (cmd/gateway/main.go log-level FileWatcher) or "ca_cert"
// (sharedtls.StartCAFileWatcher). Exported for use from cmd/gateway/main.go,
// which owns both FileWatcher lifecycles.
func (s *Server) EmitConfigReloadAudit(ctx context.Context, component string, reloadErr error) {
	if s.auditStore == nil {
		// ❌ CRITICAL: This should NEVER happen if init is fixed (ADR-032 §2)
		s.logger.Error(fmt.Errorf("AuditStore is nil"), "CRITICAL: Cannot record audit event (ADR-032 §1.5 violation)")
		return
	}

	event := audit.NewAuditEventRequest()
	audit.SetEventCategory(event, CategoryGateway)
	audit.SetActor(event, "service", "gateway")
	audit.SetResource(event, "Config", component)
	audit.SetCorrelationID(event, fmt.Sprintf("config-reload-%s-%d", component, time.Now().UnixNano()))

	if reloadErr != nil {
		audit.SetEventType(event, EventTypeConfigRejected)
		audit.SetEventAction(event, ActionRejected)
		audit.SetEventOutcome(event, audit.OutcomeFailure)
		payload := api.GatewayConfigRejectedPayload{
			EventType:       api.GatewayConfigRejectedPayloadEventTypeGatewayConfigRejected,
			Component:       component,
			RejectionReason: reloadErr.Error(),
		}
		event.EventData = api.NewGatewayConfigRejectedPayloadAuditEventRequestEventData(payload)
	} else {
		audit.SetEventType(event, EventTypeConfigReloaded)
		audit.SetEventAction(event, ActionReloaded)
		audit.SetEventOutcome(event, audit.OutcomeSuccess)
		payload := api.GatewayConfigReloadedPayload{
			EventType: api.GatewayConfigReloadedPayloadEventTypeGatewayConfigReloaded,
			Component: component,
		}
		event.EventData = api.NewGatewayConfigReloadedPayloadAuditEventRequestEventData(payload)
	}

	// Fire-and-forget: StoreAudit is non-blocking per DD-AUDIT-002
	if err := s.auditStore.StoreAudit(ctx, event); err != nil {
		s.logger.Info("DD-AUDIT-003: Failed to emit config reload audit event",
			"error", err, "component", component)
	}
}

// emitSignalReceivedAudit emits 'gateway.signal.received' audit event (BR-GATEWAY-190)
// This is called when a NEW signal is received and RR is created
func (s *Server) emitSignalReceivedAudit(ctx context.Context, signal *types.NormalizedSignal, rrName, rrNamespace string) {
	if s.auditStore == nil {
		// ❌ CRITICAL: This should NEVER happen if init is fixed (ADR-032 §2)
		s.logger.Error(fmt.Errorf("AuditStore is nil"), "CRITICAL: Cannot record audit event (ADR-032 §1.5 violation)")
		return
	}

	// Use OpenAPI helper functions (DD-AUDIT-002 V2.0.1)
	event := audit.NewAuditEventRequest()
	audit.SetEventType(event, EventTypeSignalReceived)
	audit.SetEventCategory(event, CategoryGateway)
	audit.SetEventAction(event, ActionReceived)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	// FedRAMP AU-3: Enrich actor with authenticated K8s identity when available
	if user := auth.GetUserFromContext(ctx); user != "" {
		audit.SetActor(event, "authenticated-service", user)
	} else {
		audit.SetActor(event, "external", signal.Source)
	}
	audit.SetResource(event, "Signal", signal.Fingerprint)
	audit.SetCorrelationID(event, rrName) // Use RR name as correlation
	audit.SetNamespace(event, signal.Namespace)
	// DD-AUDIT-003 v2.2: Fleet cluster provenance (CC8.1)
	if signal.ClusterID != "" {
		audit.SetClusterID(event, signal.ClusterID)
	}

	// Event data with Gateway-specific fields + RR reconstruction fields
	//
	// ========================================
	// BR-AUDIT-005: RR Reconstruction Fields (DD-AUDIT-004)
	// ========================================
	// SOC2 Compliance: Gaps #1-3 for RemediationRequest reconstruction
	// - Gap #1: original_payload (full signal payload for RR.Spec.OriginalPayload)
	// - Gap #2: signal_labels (for RR.Spec.SignalLabels)
	// - Gap #3: signal_annotations (for RR.Spec.SignalAnnotations)
	// ========================================

	// Extract RR reconstruction fields with defensive nil handling (REFACTOR phase)
	labels, annotations, originalPayload := extractRRReconstructionFields(signal)

	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.GatewayAuditPayload{
		EventType: EventTypeSignalReceived, // Required for discriminator

		// Gateway-Specific Metadata
		SignalType:  toGatewayAuditPayloadSignalType(signal.SourceType),
		SignalName:  signal.SignalName,
		Namespace:   signal.Namespace,
		Fingerprint: signal.Fingerprint,
	}

	// RR Reconstruction Fields (Root Level per DD-AUDIT-004)
	if originalPayload != nil {
		payload.OriginalPayload.SetTo(convertMapToJxRaw(originalPayload)) // Gap #1
	}
	if labels != nil {
		payload.SignalLabels.SetTo(labels) // Gap #2
	}
	if annotations != nil {
		payload.SignalAnnotations.SetTo(annotations) // Gap #3
	}

	// Optional fields
	payload.Severity = toGatewayAuditPayloadSeverity(signal.Severity) // Pass through raw severity (DD-SEVERITY-001)
	payload.ResourceKind.SetTo(signal.Resource.Kind)
	payload.ResourceName.SetTo(signal.Resource.Name)
	payload.RemediationRequest.SetTo(fmt.Sprintf("%s/%s", rrNamespace, rrName))
	payload.DeduplicationStatus.SetTo(toGatewayAuditPayloadDeduplicationStatus("new"))

	event.EventData = api.NewAuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData(payload)

	// Fire-and-forget: StoreAudit is non-blocking per DD-AUDIT-002
	// FedRAMP SC-8 / AU-9: Tamper-evidence for audit records is enforced by the
	// downstream Data Storage service and SIEM/WORM backend, not at this emission point.
	if err := s.auditStore.StoreAudit(ctx, event); err != nil {
		s.logger.Info("DD-AUDIT-003: Failed to emit signal.received audit event",
			"error", err, "fingerprint", signal.Fingerprint)
	}
}

// emitSignalDeduplicatedAudit emits 'gateway.signal.deduplicated' audit event (BR-GATEWAY-191)
// This is called when a DUPLICATE signal is detected
func (s *Server) emitSignalDeduplicatedAudit(ctx context.Context, signal *types.NormalizedSignal, rrName, rrNamespace string, occurrenceCount int32) {
	if s.auditStore == nil {
		// ❌ CRITICAL: This should NEVER happen if init is fixed (ADR-032 §2)
		s.logger.Error(fmt.Errorf("AuditStore is nil"), "CRITICAL: Cannot record audit event (ADR-032 §1.5 violation)")
		return
	}

	// Use OpenAPI helper functions (DD-AUDIT-002 V2.0.1)
	event := audit.NewAuditEventRequest()
	audit.SetEventType(event, EventTypeSignalDeduplicated)
	audit.SetEventCategory(event, CategoryGateway)
	audit.SetEventAction(event, ActionDeduplicated)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	// FedRAMP AU-3: Enrich actor with authenticated K8s identity when available
	if user := auth.GetUserFromContext(ctx); user != "" {
		audit.SetActor(event, "authenticated-service", user)
	} else {
		audit.SetActor(event, "external", signal.Source)
	}
	audit.SetResource(event, "Signal", signal.Fingerprint)
	audit.SetCorrelationID(event, rrName)
	audit.SetNamespace(event, signal.Namespace)
	// DD-AUDIT-003 v2.2: Fleet cluster provenance (CC8.1)
	if signal.ClusterID != "" {
		audit.SetClusterID(event, signal.ClusterID)
	}

	// Event data with RR reconstruction fields (same as signal.received for consistency)
	// Extract RR reconstruction fields with defensive nil handling (REFACTOR phase)
	labels, annotations, originalPayload := extractRRReconstructionFields(signal)

	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.GatewayAuditPayload{
		EventType: EventTypeSignalDeduplicated, // Required for discriminator

		// Gateway-Specific Metadata
		SignalType:  toGatewayAuditPayloadSignalType(signal.SourceType),
		SignalName:  signal.SignalName,
		Namespace:   signal.Namespace,
		Fingerprint: signal.Fingerprint,
	}
	payload.OccurrenceCount.SetTo(occurrenceCount)

	// RR Reconstruction Fields (Root Level per DD-AUDIT-004)
	if originalPayload != nil {
		payload.OriginalPayload.SetTo(convertMapToJxRaw(originalPayload)) // Gap #1
	}
	if labels != nil {
		payload.SignalLabels.SetTo(labels) // Gap #2
	}
	if annotations != nil {
		payload.SignalAnnotations.SetTo(annotations) // Gap #3
	}

	// Optional fields
	payload.RemediationRequest.SetTo(fmt.Sprintf("%s/%s", rrNamespace, rrName))
	payload.DeduplicationStatus.SetTo(toGatewayAuditPayloadDeduplicationStatus("duplicate"))

	event.EventData = api.NewAuditEventRequestEventDataGatewaySignalDeduplicatedAuditEventRequestEventData(payload)

	if err := s.auditStore.StoreAudit(ctx, event); err != nil {
		s.logger.Info("DD-AUDIT-003: Failed to emit signal.deduplicated audit event",
			"error", err, "fingerprint", signal.Fingerprint)
	}
}

// emitCRDCreatedAudit emits 'gateway.crd.created' audit event (DD-AUDIT-003)
// This is called when a RemediationRequest CRD is successfully created
func (s *Server) emitCRDCreatedAudit(ctx context.Context, signal *types.NormalizedSignal, rrName, rrNamespace string) {
	if s.auditStore == nil {
		// ❌ CRITICAL: This should NEVER happen if init is fixed (ADR-032 §2)
		s.logger.Error(fmt.Errorf("AuditStore is nil"), "CRITICAL: Cannot record audit event (ADR-032 §1.5 violation)")
		return
	}

	// Use OpenAPI helper functions (DD-AUDIT-002 V2.0.1)
	event := audit.NewAuditEventRequest()
	audit.SetEventType(event, EventTypeCRDCreated)
	audit.SetEventCategory(event, CategoryGateway)
	audit.SetEventAction(event, ActionCreated)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "gateway", "crd-creator") // Gateway's CRD creator component
	audit.SetResource(event, "RemediationRequest", fmt.Sprintf("%s/%s", rrNamespace, rrName))
	audit.SetCorrelationID(event, rrName) // Use RR name as correlation
	audit.SetNamespace(event, signal.Namespace)
	// DD-AUDIT-003 v2.2: Fleet cluster provenance (CC8.1)
	if signal.ClusterID != "" {
		audit.SetClusterID(event, signal.ClusterID)
	}

	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.GatewayAuditPayload{
		EventType: EventTypeCRDCreated, // Required for discriminator

		SignalType:  toGatewayAuditPayloadSignalType(signal.SourceType),
		SignalName:  signal.SignalName,
		Namespace:   signal.Namespace,
		Fingerprint: signal.Fingerprint,
	}

	// Optional fields
	payload.Severity = toGatewayAuditPayloadSeverity(signal.Severity) // Pass through raw severity (DD-SEVERITY-001)
	payload.ResourceKind.SetTo(signal.Resource.Kind)
	payload.ResourceName.SetTo(signal.Resource.Name)
	payload.RemediationRequest.SetTo(fmt.Sprintf("%s/%s", rrNamespace, rrName))
	payload.OccurrenceCount.SetTo(1) // BR-GATEWAY-056: New CRD always has OccurrenceCount=1

	event.EventData = api.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(payload)

	// Fire-and-forget: StoreAudit is non-blocking per DD-AUDIT-002
	if err := s.auditStore.StoreAudit(ctx, event); err != nil {
		s.logger.Info("DD-AUDIT-003: Failed to emit crd.created audit event",
			"error", err, "rrName", rrName)
	}
}

// retryAuditObserver implements processing.RetryObserver by emitting
// a gateway.crd.failed audit event for each intermediate retry attempt.
// BR-GATEWAY-058: Every retry attempt MUST generate an audit event.
type retryAuditObserver struct {
	server *Server
}

func (o *retryAuditObserver) OnRetryAttempt(ctx context.Context, signal *types.NormalizedSignal, attempt int, err error) {
	o.server.emitCRDCreationFailedAudit(ctx, signal, err)
}

// emitCRDCreationFailedAudit emits 'gateway.crd.failed' audit event (DD-AUDIT-003)
// This is called when RemediationRequest CRD creation fails
//
// GW-INT-AUD-019 Enhancement (BR-GATEWAY-093):
// Detects circuit breaker state and includes it in error details for audit trail compliance
//
// BR-GATEWAY-058-A (Enhanced Correlation ID Pattern):
// Uses human-readable correlation ID (alertname:namespace:kind:name) instead of SHA256 hash
// for better operator experience and pattern matching capabilities.
// Fingerprint (SHA256) remains in payload for deduplication queries.
func (s *Server) emitCRDCreationFailedAudit(ctx context.Context, signal *types.NormalizedSignal, err error) {
	if s.auditStore == nil {
		// ❌ CRITICAL: This should NEVER happen if init is fixed (ADR-032 §2)
		s.logger.Error(fmt.Errorf("AuditStore is nil"), "CRITICAL: Cannot record audit event (ADR-032 §1.5 violation)")
		return
	}

	// Use OpenAPI helper functions (DD-AUDIT-002 V2.0.1)
	event := audit.NewAuditEventRequest()
	audit.SetEventType(event, EventTypeCRDFailed)
	audit.SetEventCategory(event, CategoryGateway)
	audit.SetEventAction(event, ActionFailed)
	audit.SetEventOutcome(event, audit.OutcomeFailure)
	audit.SetActor(event, "gateway", "crd-creator")

	// BR-GATEWAY-058-A: Use human-readable correlation ID
	// Format: "alertname:namespace:kind:name" (e.g., "HighMemoryUsage:prod:Pod:api-789")
	// Benefit: SRE can immediately understand what triggered the failure
	// Fingerprint (SHA256) still available in payload for deduplication
	correlationID := constructReadableCorrelationID(signal)
	audit.SetResource(event, "RemediationRequest", correlationID)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, signal.Namespace)
	// DD-AUDIT-003 v2.2: Fleet cluster provenance (CC8.1)
	if signal.ClusterID != "" {
		audit.SetClusterID(event, signal.ClusterID)
	}

	// BR-AUDIT-005 Gap #7: Standardized error_details
	// GW-INT-AUD-019 (BR-GATEWAY-093): Detect circuit breaker errors for audit compliance
	var errorDetails *sharedaudit.ErrorDetails
	if errors.Is(err, circuitbreaker.ErrOpenState) {
		// Circuit breaker is open - create specialized error details
		// BR-GATEWAY-093: Circuit breaker for K8s API
		errorDetails = sharedaudit.NewErrorDetails(
			"gateway",
			"ERR_CIRCUIT_BREAKER_OPEN",
			"K8s API circuit breaker is open (fail-fast mode) - preventing cascade failure",
			true, // Retry possible once circuit breaker closes
		)
		s.logger.Info("Circuit breaker prevented K8s API request",
			"fingerprint", signal.Fingerprint,
			"circuit_breaker_state", "open")
	} else {
		// Standard K8s error handling
		errorDetails = sharedaudit.NewErrorDetailsFromK8sError("gateway", err)
	}

	// Convert shared ErrorDetails to api.ErrorDetails
	apiErrorDetails := toAPIErrorDetails(errorDetails)

	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.GatewayAuditPayload{
		EventType: EventTypeCRDFailed, // Required for discriminator

		SignalType:  toGatewayAuditPayloadSignalType(signal.SourceType),
		SignalName:  signal.SignalName,
		Namespace:   signal.Namespace,
		Fingerprint: signal.Fingerprint,
	}

	// Optional fields
	payload.Severity = toGatewayAuditPayloadSeverity(signal.Severity) // Pass through raw severity (DD-SEVERITY-001)
	payload.ResourceKind.SetTo(signal.Resource.Kind)
	payload.ResourceName.SetTo(signal.Resource.Name)
	payload.ErrorDetails.SetTo(apiErrorDetails) // Gap #7: Standardized error_details for SOC2 compliance

	event.EventData = api.NewAuditEventRequestEventDataGatewayCrdFailedAuditEventRequestEventData(payload)

	// Fire-and-forget: StoreAudit is non-blocking per DD-AUDIT-002
	if storeErr := s.auditStore.StoreAudit(ctx, event); storeErr != nil {
		s.logger.Info("DD-AUDIT-003: Failed to emit crd.creation_failed audit event",
			"error", storeErr, "fingerprint", signal.Fingerprint)
	}
}

// constructReadableCorrelationID creates human-readable correlation ID for failed CRD creation
//
// BR-GATEWAY-058-A: Enhanced Correlation ID Pattern
//
// Format: "alertname:namespace:kind:name"
// Examples:
//   - "HighMemoryUsage:prod-payment-service:Pod:payment-api-789"
//   - "NodeNotReady:default:Node:worker-node-1"
//   - "DeploymentReplicasUnavailable:prod-api:Deployment:api-server"
//
// Benefits:
//   - Human-readable: SRE can immediately identify the alert and resource
//   - Pattern matching: Query all failures for specific alert or namespace
//   - Consistency: Aligns with industry standards (OpenTelemetry semantic conventions)
//
// Fingerprint (SHA256) remains in GatewayAuditPayload.fingerprint for deduplication queries.
//
// Returns:
//   - string: Human-readable correlation ID (30-150 chars depending on names)
func constructReadableCorrelationID(signal *types.NormalizedSignal) string {
	return fmt.Sprintf("%s:%s:%s:%s",
		signal.SignalName,
		signal.Namespace,
		signal.Resource.Kind,
		signal.Resource.Name,
	)
}
