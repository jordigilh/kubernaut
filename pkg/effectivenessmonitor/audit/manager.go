/*
Copyright 2026 Jordi Gil.

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

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	pkgaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	emtypes "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

// ServiceName is the canonical service identifier for audit events.
// Follows naming convention: "<service>-controller"
const ServiceName = "effectivenessmonitor-controller"

// Event category for EM audit events (ADR-034 v1.7: Service-level category).
const CategoryEffectiveness = "effectiveness"

// Event actions for EM audit events (per DD-AUDIT-003).
const (
	ActionAssessed  = "assessed"
	ActionComputed  = "computed"
	ActionScheduled = "scheduled"
)

// componentEventTypeMapping maps EM component types to their audit event type
// and the ogen constructor for AuditEventRequestEventData.
type componentEventConfig struct {
	eventType      string
	payloadType    ogenclient.EffectivenessAssessmentAuditPayloadEventType
	component      ogenclient.EffectivenessAssessmentAuditPayloadComponent
	action         string
	newEventDataFn func(ogenclient.EffectivenessAssessmentAuditPayload) ogenclient.AuditEventRequestEventData
}

// componentConfigs maps component type strings to their event configuration.
var componentConfigs = map[string]componentEventConfig{
	"health": {
		eventType:      string(emtypes.AuditHealthAssessed),
		payloadType:    ogenclient.EffectivenessAssessmentAuditPayloadEventTypeEffectivenessHealthAssessed,
		component:      ogenclient.EffectivenessAssessmentAuditPayloadComponentHealth,
		action:         ActionAssessed,
		newEventDataFn: ogenclient.NewAuditEventRequestEventDataEffectivenessHealthAssessedAuditEventRequestEventData,
	},
	"hash": {
		eventType:      string(emtypes.AuditHashComputed),
		payloadType:    ogenclient.EffectivenessAssessmentAuditPayloadEventTypeEffectivenessHashComputed,
		component:      ogenclient.EffectivenessAssessmentAuditPayloadComponentHash,
		action:         ActionComputed,
		newEventDataFn: ogenclient.NewAuditEventRequestEventDataEffectivenessHashComputedAuditEventRequestEventData,
	},
	"alert": {
		eventType:      string(emtypes.AuditAlertAssessed),
		payloadType:    ogenclient.EffectivenessAssessmentAuditPayloadEventTypeEffectivenessAlertAssessed,
		component:      ogenclient.EffectivenessAssessmentAuditPayloadComponentAlert,
		action:         ActionAssessed,
		newEventDataFn: ogenclient.NewAuditEventRequestEventDataEffectivenessAlertAssessedAuditEventRequestEventData,
	},
	"metrics": {
		eventType:      string(emtypes.AuditMetricsAssessed),
		payloadType:    ogenclient.EffectivenessAssessmentAuditPayloadEventTypeEffectivenessMetricsAssessed,
		component:      ogenclient.EffectivenessAssessmentAuditPayloadComponentMetrics,
		action:         ActionAssessed,
		newEventDataFn: ogenclient.NewAuditEventRequestEventDataEffectivenessMetricsAssessedAuditEventRequestEventData,
	},
}

// Manager handles audit trail recording for Effectiveness Monitor lifecycle events.
//
// The Manager provides typed methods for each audit event type, ensuring
// consistent audit event structure across all EM events.
//
// Pattern: WE Pattern 2 (audit.Manager holds store + provides Record* methods)
// Authority: DD-AUDIT-002, DD-AUDIT-003, ADR-EM-001
//
// Usage:
//
//	mgr := audit.NewManager(auditStore, logger)
//	mgr.RecordComponentAssessed(ctx, ea, "health", result)
//	mgr.RecordAssessmentCompleted(ctx, ea, "full")
type Manager struct {
	store  pkgaudit.AuditStore
	logger logr.Logger
}

// NewManager creates a new EM audit manager.
//
// Parameters:
//   - store: AuditStore for writing audit events (from pkg/audit)
//   - logger: Logger for audit operations
//
// The store may be nil to disable audit (graceful degradation).
func NewManager(store pkgaudit.AuditStore, logger logr.Logger) *Manager {
	return &Manager{
		store:  store,
		logger: logger,
	}
}

// RecordComponentAssessed records an audit event for a component assessment result.
// This is called after each of the 4 assessment components (health, hash, alert, metrics).
//
// Parameters:
//   - ctx: Context for the audit call
//   - ea: The EffectivenessAssessment being assessed
//   - component: Component name ("health", "hash", "alert", "metrics")
//   - result: The component result containing score, assessed, details, error
func (m *Manager) RecordComponentAssessed(ctx context.Context, ea *eav1.EffectivenessAssessment, component string, result emtypes.ComponentResult) error {
	if m.store == nil {
		m.logger.V(1).Info("AuditStore is nil, skipping component audit event",
			"component", component)
		return nil
	}

	cfg, ok := componentConfigs[component]
	if !ok {
		return fmt.Errorf("unknown component type: %s", component)
	}

	// Build the typed payload
	payload := ogenclient.EffectivenessAssessmentAuditPayload{
		EventType:     cfg.payloadType,
		CorrelationID: ea.Spec.CorrelationID,
		Namespace:     ea.Namespace,
		EaName:        ogenclient.NewOptString(ea.Name),
		Component:     cfg.component,
		Assessed:      ogenclient.NewOptBool(result.Assessed),
		Details:       ogenclient.NewOptString(result.Details),
	}

	if result.Score != nil {
		payload.Score = ogenclient.NewOptNilFloat64(*result.Score)
	}

	// Build the AuditEventRequest
	outcome := pkgaudit.OutcomeSuccess
	if result.Error != nil {
		outcome = pkgaudit.OutcomeFailure
	}

	event := pkgaudit.NewAuditEventRequest()
	event.Version = "1.0"
	pkgaudit.SetEventType(event, cfg.eventType)
	pkgaudit.SetEventCategory(event, CategoryEffectiveness)
	pkgaudit.SetEventAction(event, cfg.action)
	pkgaudit.SetEventOutcome(event, outcome)
	pkgaudit.SetActor(event, "service", ServiceName)
	pkgaudit.SetResource(event, "EffectivenessAssessment", ea.Name)
	pkgaudit.SetCorrelationID(event, ea.Spec.CorrelationID)
	event.Namespace = ogenclient.NewOptNilString(ea.Namespace)
	event.EventData = cfg.newEventDataFn(payload)

	if err := m.store.StoreAudit(ctx, event); err != nil {
		m.logger.Error(err, "Failed to store component audit event",
			"component", component,
			"correlationID", ea.Spec.CorrelationID)
		return err
	}

	m.logger.V(1).Info("Component audit event stored",
		"component", component,
		"correlationID", ea.Spec.CorrelationID)
	return nil
}

// HealthAssessedData contains the structured health check fields for the
// effectiveness.health.assessed audit event (DD-017 v2.5 typed sub-objects).
type HealthAssessedData struct {
	// TotalReplicas is the total number of desired replicas.
	TotalReplicas int32
	// ReadyReplicas is the number of replicas that are ready.
	ReadyReplicas int32
	// RestartsSinceRemediation is the total restart count since remediation started.
	RestartsSinceRemediation int32
	// CrashLoops indicates whether any container is in CrashLoopBackOff waiting state.
	CrashLoops bool
	// OOMKilled indicates whether any container was terminated with OOMKilled reason.
	OOMKilled bool
	// PendingCount is the number of pods still in Pending phase.
	PendingCount int32
}

// AlertAssessedData contains the structured alert resolution fields for the
// effectiveness.alert.assessed audit event (DD-017 v2.5 typed sub-objects).
type AlertAssessedData struct {
	// AlertResolved indicates whether the triggering alert is no longer active.
	AlertResolved bool
	// ActiveCount is the number of matching active alerts found in AlertManager.
	ActiveCount int32
	// ResolutionTimeSeconds is the time from remediation to alert resolution (nil if not resolved).
	ResolutionTimeSeconds *float64
}

// MetricsAssessedData contains the structured metric delta fields for the
// effectiveness.metrics.assessed audit event (DD-017 v2.5 typed sub-objects).
// Phase A (V1.0): Only CPUBefore/CPUAfter are populated.
// Phase B: Memory, latency, error rate, throughput fields will be added.
type MetricsAssessedData struct {
	// CPUBefore is the CPU utilization before remediation (nil if unavailable).
	CPUBefore *float64
	// CPUAfter is the CPU utilization after remediation (nil if unavailable).
	CPUAfter *float64
	// MemoryBefore is the memory utilization before remediation (Phase B, nil for now).
	MemoryBefore *float64
	// MemoryAfter is the memory utilization after remediation (Phase B, nil for now).
	MemoryAfter *float64
	// LatencyP95BeforeMs is the p95 latency in ms before remediation (Phase B, nil for now).
	LatencyP95BeforeMs *float64
	// LatencyP95AfterMs is the p95 latency in ms after remediation (Phase B, nil for now).
	LatencyP95AfterMs *float64
	// ErrorRateBefore is the error rate before remediation (Phase B, nil for now).
	ErrorRateBefore *float64
	// ErrorRateAfter is the error rate after remediation (Phase B, nil for now).
	ErrorRateAfter *float64
	// ThroughputBeforeRPS is the request throughput (req/s) before remediation (nil if unavailable).
	ThroughputBeforeRPS *float64
	// ThroughputAfterRPS is the request throughput (req/s) after remediation (nil if unavailable).
	ThroughputAfterRPS *float64
}

// RecordHealthAssessed records the effectiveness.health.assessed audit event with
// the health_checks typed sub-object (DD-017 v2.5).
func (m *Manager) RecordHealthAssessed(ctx context.Context, ea *eav1.EffectivenessAssessment, result emtypes.ComponentResult, healthData HealthAssessedData) error {
	if m.store == nil {
		m.logger.V(1).Info("AuditStore is nil, skipping health assessed audit event")
		return nil
	}

	cfg := componentConfigs["health"]

	payload := m.buildBasePayload(cfg, ea, result)

	// Set health_checks typed sub-object (DD-017 v2.5)
	podRunning := healthData.TotalReplicas > 0
	readinessPass := healthData.ReadyReplicas == healthData.TotalReplicas && healthData.TotalReplicas > 0
	payload.HealthChecks = ogenclient.NewOptEffectivenessAssessmentAuditPayloadHealthChecks(
		ogenclient.EffectivenessAssessmentAuditPayloadHealthChecks{
			PodRunning:    ogenclient.NewOptBool(podRunning),
			ReadinessPass: ogenclient.NewOptBool(readinessPass),
			TotalReplicas: ogenclient.NewOptInt32(healthData.TotalReplicas),
			ReadyReplicas: ogenclient.NewOptInt32(healthData.ReadyReplicas),
			RestartDelta:  ogenclient.NewOptInt32(healthData.RestartsSinceRemediation),
			CrashLoops:    ogenclient.NewOptBool(healthData.CrashLoops),
			OomKilled:     ogenclient.NewOptBool(healthData.OOMKilled),
			PendingCount:  ogenclient.NewOptInt32(healthData.PendingCount),
		},
	)

	return m.storeEvent(ctx, cfg, ea, payload, result)
}

// RecordAlertAssessed records the effectiveness.alert.assessed audit event with
// the alert_resolution typed sub-object (DD-017 v2.5).
func (m *Manager) RecordAlertAssessed(ctx context.Context, ea *eav1.EffectivenessAssessment, result emtypes.ComponentResult, alertData AlertAssessedData) error {
	if m.store == nil {
		m.logger.V(1).Info("AuditStore is nil, skipping alert assessed audit event")
		return nil
	}

	cfg := componentConfigs["alert"]

	payload := m.buildBasePayload(cfg, ea, result)

	// Set alert_resolution typed sub-object (DD-017 v2.5)
	ar := ogenclient.EffectivenessAssessmentAuditPayloadAlertResolution{
		AlertResolved: ogenclient.NewOptBool(alertData.AlertResolved),
		ActiveCount:   ogenclient.NewOptInt32(alertData.ActiveCount),
	}
	if alertData.ResolutionTimeSeconds != nil {
		ar.ResolutionTimeSeconds = ogenclient.NewOptNilFloat64(*alertData.ResolutionTimeSeconds)
	}
	payload.AlertResolution = ogenclient.NewOptEffectivenessAssessmentAuditPayloadAlertResolution(ar)

	return m.storeEvent(ctx, cfg, ea, payload, result)
}

// RecordMetricsAssessed records the effectiveness.metrics.assessed audit event with
// the metric_deltas typed sub-object (DD-017 v2.5).
func (m *Manager) RecordMetricsAssessed(ctx context.Context, ea *eav1.EffectivenessAssessment, result emtypes.ComponentResult, metricsData MetricsAssessedData) error {
	if m.store == nil {
		m.logger.V(1).Info("AuditStore is nil, skipping metrics assessed audit event")
		return nil
	}

	cfg := componentConfigs["metrics"]

	payload := m.buildBasePayload(cfg, ea, result)

	// Set metric_deltas typed sub-object (DD-017 v2.5)
	md := ogenclient.EffectivenessAssessmentAuditPayloadMetricDeltas{}
	if metricsData.CPUBefore != nil {
		md.CPUBefore = ogenclient.NewOptNilFloat64(*metricsData.CPUBefore)
	}
	if metricsData.CPUAfter != nil {
		md.CPUAfter = ogenclient.NewOptNilFloat64(*metricsData.CPUAfter)
	}
	if metricsData.MemoryBefore != nil {
		md.MemoryBefore = ogenclient.NewOptNilFloat64(*metricsData.MemoryBefore)
	}
	if metricsData.MemoryAfter != nil {
		md.MemoryAfter = ogenclient.NewOptNilFloat64(*metricsData.MemoryAfter)
	}
	if metricsData.LatencyP95BeforeMs != nil {
		md.LatencyP95BeforeMs = ogenclient.NewOptNilFloat64(*metricsData.LatencyP95BeforeMs)
	}
	if metricsData.LatencyP95AfterMs != nil {
		md.LatencyP95AfterMs = ogenclient.NewOptNilFloat64(*metricsData.LatencyP95AfterMs)
	}
	if metricsData.ErrorRateBefore != nil {
		md.ErrorRateBefore = ogenclient.NewOptNilFloat64(*metricsData.ErrorRateBefore)
	}
	if metricsData.ErrorRateAfter != nil {
		md.ErrorRateAfter = ogenclient.NewOptNilFloat64(*metricsData.ErrorRateAfter)
	}
	if metricsData.ThroughputBeforeRPS != nil {
		md.ThroughputBeforeRps = ogenclient.NewOptNilFloat64(*metricsData.ThroughputBeforeRPS)
	}
	if metricsData.ThroughputAfterRPS != nil {
		md.ThroughputAfterRps = ogenclient.NewOptNilFloat64(*metricsData.ThroughputAfterRPS)
	}
	payload.MetricDeltas = ogenclient.NewOptEffectivenessAssessmentAuditPayloadMetricDeltas(md)

	return m.storeEvent(ctx, cfg, ea, payload, result)
}

// buildBasePayload constructs the common payload fields shared by all component events.
func (m *Manager) buildBasePayload(cfg componentEventConfig, ea *eav1.EffectivenessAssessment, result emtypes.ComponentResult) ogenclient.EffectivenessAssessmentAuditPayload {
	payload := ogenclient.EffectivenessAssessmentAuditPayload{
		EventType:     cfg.payloadType,
		CorrelationID: ea.Spec.CorrelationID,
		Namespace:     ea.Namespace,
		EaName:        ogenclient.NewOptString(ea.Name),
		Component:     cfg.component,
		Assessed:      ogenclient.NewOptBool(result.Assessed),
		Details:       ogenclient.NewOptString(result.Details),
	}
	if result.Score != nil {
		payload.Score = ogenclient.NewOptNilFloat64(*result.Score)
	}
	return payload
}

// storeEvent builds the AuditEventRequest from a payload and stores it.
func (m *Manager) storeEvent(ctx context.Context, cfg componentEventConfig, ea *eav1.EffectivenessAssessment, payload ogenclient.EffectivenessAssessmentAuditPayload, result emtypes.ComponentResult) error {
	outcome := pkgaudit.OutcomeSuccess
	if result.Error != nil {
		outcome = pkgaudit.OutcomeFailure
	}

	event := pkgaudit.NewAuditEventRequest()
	event.Version = "1.0"
	pkgaudit.SetEventType(event, cfg.eventType)
	pkgaudit.SetEventCategory(event, CategoryEffectiveness)
	pkgaudit.SetEventAction(event, cfg.action)
	pkgaudit.SetEventOutcome(event, outcome)
	pkgaudit.SetActor(event, "service", ServiceName)
	pkgaudit.SetResource(event, "EffectivenessAssessment", ea.Name)
	pkgaudit.SetCorrelationID(event, ea.Spec.CorrelationID)
	event.Namespace = ogenclient.NewOptNilString(ea.Namespace)
	event.EventData = cfg.newEventDataFn(payload)

	if err := m.store.StoreAudit(ctx, event); err != nil {
		m.logger.Error(err, "Failed to store component audit event",
			"component", string(cfg.component),
			"correlationID", ea.Spec.CorrelationID)
		return err
	}

	m.logger.V(1).Info("Component audit event stored (typed sub-object)",
		"component", string(cfg.component),
		"correlationID", ea.Spec.CorrelationID)
	return nil
}

// HashComputedData contains the hash-specific fields for the effectiveness.hash.computed audit event.
// These fields extend the generic ComponentResult with pre/post hash comparison data (DD-EM-002).
type HashComputedData struct {
	// PostHash is the post-remediation canonical spec hash ("sha256:<hex>").
	PostHash string
	// PreHash is the pre-remediation hash from the DS audit trail.
	// Empty string if not available.
	PreHash string
	// Match indicates whether pre and post hashes are identical.
	// nil if no pre-hash was available (comparison not possible).
	Match *bool
}

// RecordHashComputed records the effectiveness.hash.computed audit event with
// the hash-specific fields (pre_remediation_spec_hash, post_remediation_spec_hash,
// hash_match) per DD-EM-002.
//
// This method is used instead of RecordComponentAssessed for hash events because
// the hash component produces additional structured data that needs to be included
// in the audit payload.
func (m *Manager) RecordHashComputed(ctx context.Context, ea *eav1.EffectivenessAssessment, result emtypes.ComponentResult, hashData HashComputedData) error {
	if m.store == nil {
		m.logger.V(1).Info("AuditStore is nil, skipping hash computed audit event")
		return nil
	}

	cfg := componentConfigs["hash"]

	// Build the typed payload with hash-specific fields
	payload := ogenclient.EffectivenessAssessmentAuditPayload{
		EventType:     cfg.payloadType,
		CorrelationID: ea.Spec.CorrelationID,
		Namespace:     ea.Namespace,
		EaName:        ogenclient.NewOptString(ea.Name),
		Component:     cfg.component,
		Assessed:      ogenclient.NewOptBool(result.Assessed),
		Details:       ogenclient.NewOptString(result.Details),
	}

	// Set hash-specific fields (DD-EM-002)
	if hashData.PostHash != "" {
		payload.PostRemediationSpecHash = ogenclient.NewOptString(hashData.PostHash)
	}
	if hashData.PreHash != "" {
		payload.PreRemediationSpecHash = ogenclient.NewOptString(hashData.PreHash)
	}
	if hashData.Match != nil {
		payload.HashMatch = ogenclient.NewOptBool(*hashData.Match)
	}

	// Build the AuditEventRequest
	outcome := pkgaudit.OutcomeSuccess
	if result.Error != nil {
		outcome = pkgaudit.OutcomeFailure
	}

	event := pkgaudit.NewAuditEventRequest()
	event.Version = "1.0"
	pkgaudit.SetEventType(event, cfg.eventType)
	pkgaudit.SetEventCategory(event, CategoryEffectiveness)
	pkgaudit.SetEventAction(event, cfg.action)
	pkgaudit.SetEventOutcome(event, outcome)
	pkgaudit.SetActor(event, "service", ServiceName)
	pkgaudit.SetResource(event, "EffectivenessAssessment", ea.Name)
	pkgaudit.SetCorrelationID(event, ea.Spec.CorrelationID)
	event.Namespace = ogenclient.NewOptNilString(ea.Namespace)
	event.EventData = cfg.newEventDataFn(payload)

	if err := m.store.StoreAudit(ctx, event); err != nil {
		m.logger.Error(err, "Failed to store hash computed audit event",
			"correlationID", ea.Spec.CorrelationID)
		return err
	}

	m.logger.V(1).Info("Hash computed audit event stored",
		"correlationID", ea.Spec.CorrelationID,
		"postHash", hashData.PostHash,
		"preHash", hashData.PreHash,
		"match", hashData.Match)
	return nil
}

// RecordAssessmentScheduled records the assessment.scheduled audit event on first reconciliation.
// This event captures all derived timing values computed by the EM (BR-EM-009.4).
//
// Parameters:
//   - ctx: Context for the audit call
//   - ea: The EffectivenessAssessment being processed (status must have timing fields set)
//   - validityWindow: The validity window duration from EM config
func (m *Manager) RecordAssessmentScheduled(ctx context.Context, ea *eav1.EffectivenessAssessment, validityWindow time.Duration) error {
	if m.store == nil {
		m.logger.V(1).Info("AuditStore is nil, skipping assessment scheduled audit event")
		return nil
	}

	payload := ogenclient.EffectivenessAssessmentAuditPayload{
		EventType:     ogenclient.EffectivenessAssessmentAuditPayloadEventTypeEffectivenessAssessmentScheduled,
		CorrelationID: ea.Spec.CorrelationID,
		Namespace:     ea.Namespace,
		EaName:        ogenclient.NewOptString(ea.Name),
		Component:     ogenclient.EffectivenessAssessmentAuditPayloadComponentScheduled,
		Assessed:      ogenclient.NewOptBool(false),
		Details:       ogenclient.NewOptString("Assessment timeline computed on first reconciliation"),
	}

	// Set the scheduling-specific fields (BR-EM-009.4)
	if ea.Status.ValidityDeadline != nil {
		payload.ValidityDeadline = ogenclient.NewOptDateTime(ea.Status.ValidityDeadline.Time)
	}
	if ea.Status.PrometheusCheckAfter != nil {
		payload.PrometheusCheckAfter = ogenclient.NewOptDateTime(ea.Status.PrometheusCheckAfter.Time)
	}
	if ea.Status.AlertManagerCheckAfter != nil {
		payload.AlertmanagerCheckAfter = ogenclient.NewOptDateTime(ea.Status.AlertManagerCheckAfter.Time)
	}
	payload.ValidityWindow = ogenclient.NewOptString(validityWindow.String())
	payload.StabilizationWindow = ogenclient.NewOptString(ea.Spec.Config.StabilizationWindow.Duration.String())

	event := pkgaudit.NewAuditEventRequest()
	event.Version = "1.0"
	pkgaudit.SetEventType(event, string(emtypes.AuditAssessmentScheduled))
	pkgaudit.SetEventCategory(event, CategoryEffectiveness)
	pkgaudit.SetEventAction(event, ActionScheduled)
	pkgaudit.SetEventOutcome(event, pkgaudit.OutcomeSuccess)
	pkgaudit.SetActor(event, "service", ServiceName)
	pkgaudit.SetResource(event, "EffectivenessAssessment", ea.Name)
	pkgaudit.SetCorrelationID(event, ea.Spec.CorrelationID)
	event.Namespace = ogenclient.NewOptNilString(ea.Namespace)
	event.EventData = ogenclient.NewAuditEventRequestEventDataEffectivenessAssessmentScheduledAuditEventRequestEventData(payload)

	if err := m.store.StoreAudit(ctx, event); err != nil {
		m.logger.Error(err, "Failed to store assessment scheduled audit event",
			"correlationID", ea.Spec.CorrelationID)
		return err
	}

	m.logger.V(1).Info("Assessment scheduled audit event stored",
		"correlationID", ea.Spec.CorrelationID)
	return nil
}

// RecordAssessmentCompleted records the final assessment.completed audit event.
// This is called when the EA transitions to the Completed phase.
//
// ADR-EM-001 Section 9.2: Populates all enrichment fields:
//   - alert_name: Original alert name from EA spec SignalName (OBS-1)
//   - components_assessed: List of assessed component names
//   - completed_at: EA completion timestamp
//   - assessment_duration_seconds: Time from RR creation to assessment completion (OBS-2)
//   - (throughput is part of metric_deltas in component events, not here)
//
// Parameters:
//   - ctx: Context for the audit call
//   - ea: The EffectivenessAssessment that completed
//   - reason: Assessment completion reason (full, partial, expired, etc.)
func (m *Manager) RecordAssessmentCompleted(ctx context.Context, ea *eav1.EffectivenessAssessment, reason string) error {
	if m.store == nil {
		m.logger.V(1).Info("AuditStore is nil, skipping assessment completed audit event")
		return nil
	}

	payload := ogenclient.EffectivenessAssessmentAuditPayload{
		EventType:     ogenclient.EffectivenessAssessmentAuditPayloadEventTypeEffectivenessAssessmentCompleted,
		CorrelationID: ea.Spec.CorrelationID,
		Namespace:     ea.Namespace,
		EaName:        ogenclient.NewOptString(ea.Name),
		Component:     ogenclient.EffectivenessAssessmentAuditPayloadComponentCompleted,
		Assessed:      ogenclient.NewOptBool(true),
		Reason:        ogenclient.NewOptString(reason),
	}

	// Set details with the assessment reason
	payload.Details = ogenclient.NewOptString(fmt.Sprintf("Assessment completed: %s", reason))

	// ADR-EM-001, Batch 3: Populate all 5 audit payload gaps.

	// 1. alert_name: Original alert/signal name from the parent RemediationRequest.
	//    OBS-1: Uses ea.Spec.SignalName (set by the RO creator from rr.Spec.SignalName),
	//    which is the actual alert name — distinct from CorrelationID (the RR name).
	payload.SignalName = ogenclient.NewOptString(ea.Spec.SignalName)

	// 2. components_assessed: Build array from EA status component flags.
	var assessed []string
	if ea.Status.Components.HealthAssessed {
		assessed = append(assessed, "health")
	}
	if ea.Status.Components.HashComputed {
		assessed = append(assessed, "hash")
	}
	if ea.Status.Components.AlertAssessed {
		assessed = append(assessed, "alert")
	}
	if ea.Status.Components.MetricsAssessed {
		assessed = append(assessed, "metrics")
	}
	payload.ComponentsAssessed = assessed

	// 3. completed_at: EA completion timestamp.
	if ea.Status.CompletedAt != nil {
		payload.CompletedAt = ogenclient.NewOptDateTime(ea.Status.CompletedAt.Time)
	}

	// 4. assessment_duration_seconds (OBS-2: renamed from resolution_time_seconds):
	//    Time from RR creation to assessment completion. Distinct from
	//    alert_resolution.resolution_time_seconds which is alert-level.
	if ea.Spec.RemediationCreatedAt != nil && ea.Status.CompletedAt != nil {
		duration := ea.Status.CompletedAt.Time.Sub(ea.Spec.RemediationCreatedAt.Time).Seconds()
		payload.AssessmentDurationSeconds = ogenclient.NewOptNilFloat64(duration)
	}

	event := pkgaudit.NewAuditEventRequest()
	event.Version = "1.0"
	pkgaudit.SetEventType(event, string(emtypes.AuditAssessmentCompleted))
	pkgaudit.SetEventCategory(event, CategoryEffectiveness)
	pkgaudit.SetEventAction(event, ActionAssessed)
	pkgaudit.SetEventOutcome(event, pkgaudit.OutcomeSuccess)
	pkgaudit.SetActor(event, "service", ServiceName)
	pkgaudit.SetResource(event, "EffectivenessAssessment", ea.Name)
	pkgaudit.SetCorrelationID(event, ea.Spec.CorrelationID)
	event.Namespace = ogenclient.NewOptNilString(ea.Namespace)
	event.EventData = ogenclient.NewAuditEventRequestEventDataEffectivenessAssessmentCompletedAuditEventRequestEventData(payload)

	if err := m.store.StoreAudit(ctx, event); err != nil {
		m.logger.Error(err, "Failed to store assessment completed audit event",
			"correlationID", ea.Spec.CorrelationID,
			"reason", reason)
		return err
	}

	m.logger.V(1).Info("Assessment completed audit event stored",
		"correlationID", ea.Spec.CorrelationID,
		"reason", reason,
		"alertName", ea.Spec.SignalName,
		"componentsAssessed", assessed,
	)
	return nil
}
