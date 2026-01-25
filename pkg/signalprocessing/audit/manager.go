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

// Package audit provides centralized audit management for SignalProcessing.
//
// Reference: CONTROLLER_REFACTORING_PATTERN_LIBRARY.md - Pattern 7 (Audit Manager)
//
// The Audit Manager pattern centralizes audit operations to provide:
// - ADR-032 enforcement (audit is MANDATORY - fail if not configured)
// - Consistent audit event formatting across the controller
// - Single point of control for audit operations
//
// Per Controller Refactoring Pattern Library (P3: Audit Manager):
// - Extracted from internal/controller/signalprocessing/signalprocessing_controller.go
// - Follows RemediationOrchestrator/WorkflowExecution/AIAnalysis/Notification patterns
// - Consistent package structure across all controllers
//
// Business Requirements:
// - BR-SP-090: Categorization Audit Trail
// - ADR-032: Audit is MANDATORY - fail if not configured
//
// **Refactoring**: 2026-01-22 - Phase 3 audit manager extraction complete
package audit

import (
	"context"
	"fmt"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// Manager centralizes audit event recording for SignalProcessing.
// It wraps the AuditClient with ADR-032 enforcement and provides a consistent
// interface for the controller.
//
// Business Requirements:
// - BR-SP-090: Categorization Audit Trail
// - ADR-032: Audit is MANDATORY - fail if not configured
type Manager struct {
	client *AuditClient
}

// NewManager creates a new Audit Manager.
//
// The Manager wraps the AuditClient and provides ADR-032 enforcement,
// ensuring audit operations fail explicitly if misconfigured.
//
// Parameters:
// - client: AuditClient for writing audit events
//
// Per ADR-032: client should never be nil in production. The Manager
// will return explicit errors if audit operations are attempted with nil client.
func NewManager(client *AuditClient) *Manager {
	return &Manager{
		client: client,
	}
}

// RecordPhaseTransition records a phase transition audit event.
//
// This method provides ADR-032 enforcement: returns error if AuditClient is nil,
// preventing silent audit failures. Delegates to AuditClient for event construction.
//
// Parameters:
// - ctx: Context for the operation
// - sp: SignalProcessing CRD with current state
// - fromPhase: Previous phase (for idempotency check)
// - toPhase: New phase
//
// Returns:
// - error if AuditClient is nil (ADR-032 mandate)
// - error if phase transition is duplicate (idempotency guard)
// - nil if audit event was successfully submitted (fire-and-forget)
func (m *Manager) RecordPhaseTransition(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, fromPhase, toPhase string) error {
	if m.client == nil {
		return fmt.Errorf("AuditClient is nil - audit is MANDATORY per ADR-032")
	}

	// SP-BUG-002: Skip audit if no actual transition occurred
	// This prevents duplicate events when controller processes same phase twice due to K8s cache/watch timing
	if fromPhase == toPhase {
		return nil
	}

	m.client.RecordPhaseTransition(ctx, sp, fromPhase, toPhase)
	return nil
}

// RecordEnrichmentComplete records enrichment completion audit event.
//
// This method provides ADR-032 enforcement and idempotency guards.
//
// Parameters:
// - ctx: Context for the operation
// - sp: SignalProcessing CRD with enriched context
// - durationMs: Enrichment duration in milliseconds (for performance metrics)
// - alreadyCompleted: Idempotency guard - skip if enrichment was already completed
//
// Returns:
// - error if AuditClient is nil (ADR-032 mandate)
// - error if alreadyCompleted is true (idempotency guard)
// - nil if audit event was successfully submitted (fire-and-forget)
func (m *Manager) RecordEnrichmentComplete(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, durationMs int, alreadyCompleted bool) error {
	if m.client == nil {
		return fmt.Errorf("AuditClient is nil - audit is MANDATORY per ADR-032")
	}

	// SP-BUG-ENRICHMENT-001: Idempotency guard - skip if enrichment was already completed before this reconciliation
	// This prevents duplicate events when controller processes same enrichment phase twice due to K8s cache/watch timing
	if alreadyCompleted {
		// Enrichment already completed and audited - skip to prevent duplicate
		return nil
	}

	m.client.RecordEnrichmentComplete(ctx, sp, durationMs)
	return nil
}

// RecordCompletion records final signal processing completion audit events.
//
// This method emits two audit events:
// 1. signalprocessing.signal.processed (primary completion event)
// 2. signalprocessing.business.classified (business classification details)
//
// Per DD-SEVERITY-001: Classification decision is emitted ONCE during Classifying phase,
// not duplicated here to maintain "one classification decision = one audit event" principle.
//
// Parameters:
// - ctx: Context for the operation
// - sp: SignalProcessing CRD with final status
//
// Returns:
// - error if AuditClient is nil (ADR-032 mandate)
// - nil if audit events were successfully submitted (fire-and-forget)
func (m *Manager) RecordCompletion(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) error {
	if m.client == nil {
		return fmt.Errorf("AuditClient is nil - audit is MANDATORY per ADR-032")
	}

	m.client.RecordSignalProcessed(ctx, sp)

	// DD-SEVERITY-001: Classification decision emitted ONCE during Classifying phase
	// Not duplicated here to maintain "one classification decision = one audit event" principle

	// AUDIT-06: Emit dedicated business classification event (per integration-test-plan.md v1.1.0)
	m.client.RecordBusinessClassification(ctx, sp)

	return nil
}

// RecordClassificationDecision records classification decision audit event.
//
// This method is called during the Classifying phase to record severity,
// priority, environment, and business classification decisions.
//
// Per DD-SEVERITY-001: Includes both external and normalized severity for audit trail.
//
// Parameters:
// - ctx: Context for the operation
// - sp: SignalProcessing CRD with classification results
// - durationMs: Classification duration in milliseconds (for performance metrics)
//
// Returns:
// - error if AuditClient is nil (ADR-032 mandate)
// - nil if audit event was successfully submitted (fire-and-forget)
func (m *Manager) RecordClassificationDecision(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, durationMs int) error {
	if m.client == nil {
		return fmt.Errorf("AuditClient is nil - audit is MANDATORY per ADR-032")
	}

	m.client.RecordClassificationDecision(ctx, sp, durationMs)
	return nil
}

// RecordError records error audit event.
//
// This method is called when the controller encounters errors during processing.
//
// Parameters:
// - ctx: Context for the operation
// - sp: SignalProcessing CRD when error occurred
// - phase: Phase where error occurred (for troubleshooting)
// - err: The error that occurred
//
// Returns:
// - error if AuditClient is nil (ADR-032 mandate)
// - nil if audit event was successfully submitted (fire-and-forget)
func (m *Manager) RecordError(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, phase string, err error) error {
	if m.client == nil {
		return fmt.Errorf("AuditClient is nil - audit is MANDATORY per ADR-032")
	}

	m.client.RecordError(ctx, sp, phase, err)
	return nil
}













