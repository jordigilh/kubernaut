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

// ADR-032-compliant audit helper functions, split out of signalprocessing_controller.go
// per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520) to keep the file under
// the 700-line convention threshold. Pure structural move — no behavior change.
package signalprocessing

import (
	"context"
	"fmt"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// ========================================
// ADR-032 COMPLIANT AUDIT FUNCTIONS
// ========================================
// ADR-032 §2: "No Audit Loss" - Audit writes are MANDATORY, not best-effort
// Services MUST NOT implement "graceful degradation" that silently skips audit
// Services MUST return error if audit client is nil

// recordPhaseTransitionAudit records a phase transition audit event.
// ADR-032: Returns error if AuditManager is nil (no silent skip allowed).
// SP-BUG-002: Prevents duplicate audit events when phase hasn't actually changed (race condition mitigation).
//
// **Refactoring**: 2026-01-22 - Phase 3: Delegates to AuditManager for ADR-032 enforcement
func (r *SignalProcessingReconciler) recordPhaseTransitionAudit(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, oldPhase, newPhase string) error {
	if r.AuditManager == nil {
		return fmt.Errorf("AuditManager is nil - audit is MANDATORY per ADR-032")
	}
	return r.AuditManager.RecordPhaseTransition(ctx, sp, oldPhase, newPhase)
}

// recordEnrichmentCompleteAudit records an enrichment completion audit event.
// ADR-032: Returns error if AuditManager is nil (no silent skip allowed).
// RF-SP-003: Now tracks actual enrichment duration for audit metrics.
// SP-BUG-ENRICHMENT-001: Prevents duplicate audit events when enrichment already completed (race condition mitigation).
//
// **Refactoring**: 2026-01-22 - Phase 3: Delegates to AuditManager for ADR-032 enforcement
func (r *SignalProcessingReconciler) recordEnrichmentCompleteAudit(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, k8sCtx *signalprocessingv1alpha1.KubernetesContext, durationMs int, alreadyCompleted bool) error {
	if r.AuditManager == nil {
		return fmt.Errorf("AuditManager is nil - audit is MANDATORY per ADR-032")
	}

	// Create a temporary SP with enriched context for audit
	auditSP := sp.DeepCopy()
	auditSP.Status.KubernetesContext = k8sCtx
	return r.AuditManager.RecordEnrichmentComplete(ctx, auditSP, durationMs, alreadyCompleted)
}

// recordCompletionAudit records the final signal processed and classification decision audit events.
// ADR-032: Returns error if AuditManager is nil (no silent skip allowed).
// AUDIT-06: Now also emits business.classified event for granular audit trail.
//
// **Refactoring**: 2026-01-22 - Phase 3: Delegates to AuditManager for ADR-032 enforcement
func (r *SignalProcessingReconciler) recordCompletionAudit(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) error {
	if r.AuditManager == nil {
		return fmt.Errorf("AuditManager is nil - audit is MANDATORY per ADR-032")
	}
	return r.AuditManager.RecordCompletion(ctx, sp)
}
