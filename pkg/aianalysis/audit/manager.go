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

// Package audit provides audit trail management for AIAnalysis.
//
// Pattern: P3 - Audit Manager
// This manager provides high-level audit orchestration and common patterns.
package audit

import (
	"context"
	"fmt"
	"time"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

// Manager orchestrates audit event recording for AIAnalysis operations.
//
// This manager provides:
// - Consistent audit patterns across handlers
// - Error wrapping with automatic audit recording
// - Phase transition audit orchestration
// - Performance metric audit helpers
//
// Pattern: P3 - Audit Manager (from Controller Refactoring Pattern Library)
type Manager struct {
	client *AuditClient
}

// NewManager creates a new audit manager.
func NewManager(client *AuditClient) *Manager {
	return &Manager{
		client: client,
	}
}

// RecordPhaseTransitionWithTimestamp records a phase transition with timestamp metadata.
//
// This helper ensures consistent phase transition audit events with:
// - Old and new phase values
// - Transition timestamp
// - Correlation ID from analysis
func (m *Manager) RecordPhaseTransitionWithTimestamp(ctx context.Context, analysis *aianalysisv1.AIAnalysis, from, to string) {
	m.client.RecordPhaseTransition(ctx, analysis, from, to)
}

// RecordErrorWithContext wraps error recording with additional context.
//
// This helper ensures errors are audited with:
// - Current phase information
// - Error message and type
// - Correlation ID
//
// Returns the original error (for convenience in error handling).
func (m *Manager) RecordErrorWithContext(ctx context.Context, analysis *aianalysisv1.AIAnalysis, phase string, err error) error {
	if err != nil {
		m.client.RecordError(ctx, analysis, phase, err)
	}
	return err
}

// RecordOperationTiming records operation timing with automatic duration calculation.
//
// Usage:
//   startTime := time.Now()
//   // ... perform operation ...
//   manager.RecordOperationTiming(ctx, analysis, "rego_evaluation", startTime, outcome, degraded, reason)
func (m *Manager) RecordOperationTiming(ctx context.Context, analysis *aianalysisv1.AIAnalysis, operation string, startTime time.Time, outcome string, degraded bool, reason string) {
	durationMs := int(time.Since(startTime).Milliseconds())

	switch operation {
	case "rego_evaluation":
		m.client.RecordRegoEvaluation(ctx, analysis, outcome, degraded, durationMs, reason)
	default:
		// For other operations, could extend with additional cases
	}
}

// RecordAIAgentCallWithTiming records an AI agent API call with timing.
//
// This helper ensures AI agent calls are audited consistently with:
// - API endpoint
// - HTTP status code
// - Call duration
func (m *Manager) RecordAIAgentCallWithTiming(ctx context.Context, analysis *aianalysisv1.AIAnalysis, endpoint string, statusCode int, startTime time.Time) {
	durationMs := int(time.Since(startTime).Milliseconds())
	m.client.RecordAIAgentCall(ctx, analysis, endpoint, statusCode, durationMs)
}

// RecordApprovalDecisionWithMetadata records an approval decision with full metadata.
//
// This helper ensures approval decisions are audited with:
// - Decision outcome (approved/requires_approval)
// - Reason for decision
// - Auto-approval metadata
func (m *Manager) RecordApprovalDecisionWithMetadata(ctx context.Context, analysis *aianalysisv1.AIAnalysis, decision string, reason string) {
	m.client.RecordApprovalDecision(ctx, analysis, decision, reason)
}

// RecordCompletionWithFinalStatus records analysis completion with final status.
//
// This helper ensures analysis completion is audited with:
// - Final phase (Completed/Failed)
// - Selected workflow information
// - Approval status
func (m *Manager) RecordCompletionWithFinalStatus(ctx context.Context, analysis *aianalysisv1.AIAnalysis) {
	m.client.RecordAnalysisComplete(ctx, analysis)
}

// WithPhaseContext creates a scoped audit context for a specific phase.
//
// This helper simplifies error recording within a phase by capturing the phase context.
//
// Usage:
//   phaseAudit := manager.WithPhaseContext(analysis, "Investigating")
//   if err := doSomething(); err != nil {
//       return phaseAudit.RecordError(ctx, err)
//   }
type PhaseAuditContext struct {
	manager  *Manager
	analysis *aianalysisv1.AIAnalysis
	phase    string
}

// WithPhaseContext creates a phase-scoped audit context.
func (m *Manager) WithPhaseContext(analysis *aianalysisv1.AIAnalysis, phase string) *PhaseAuditContext {
	return &PhaseAuditContext{
		manager:  m,
		analysis: analysis,
		phase:    phase,
	}
}

// RecordError records an error within the phase context.
func (pac *PhaseAuditContext) RecordError(ctx context.Context, err error) error {
	return pac.manager.RecordErrorWithContext(ctx, pac.analysis, pac.phase, err)
}

// RecordPhaseTransition records a transition from the current phase to a new phase.
func (pac *PhaseAuditContext) RecordPhaseTransition(ctx context.Context, newPhase string) {
	pac.manager.RecordPhaseTransitionWithTimestamp(ctx, pac.analysis, pac.phase, newPhase)
}

// AuditMiddleware provides a consistent pattern for auditing operations with error handling.
//
// This helper wraps an operation with:
// - Automatic error audit recording
// - Duration tracking
// - Phase context
//
// Usage:
//   err := manager.AuditMiddleware(ctx, analysis, "Investigating", func() error {
//       return doSomethingRisky()
//   })
func (m *Manager) AuditMiddleware(ctx context.Context, analysis *aianalysisv1.AIAnalysis, phase string, operation func() error) error {
	err := operation()
	if err != nil {
		m.client.RecordError(ctx, analysis, phase, err)
		return fmt.Errorf("operation failed in phase %s: %w", phase, err)
	}
	return nil
}

// GetUnderlyingClient returns the underlying AuditClient for direct access.
//
// Use this when you need to call audit methods not wrapped by the manager.
func (m *Manager) GetUnderlyingClient() *AuditClient {
	return m.client
}


