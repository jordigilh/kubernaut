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

// Workflow catalog + execution-engine resolution helpers, split out of
// workflowexecution_controller.go per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520) to keep the file under the 700-line convention threshold.
// Pure structural move — no behavior change.
package workflowexecution

import (
	"fmt"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ========================================
// Issue #1661 Change 11e (DD-WORKFLOW-018): CRD-Embedded Execution Snapshot
// ========================================
//
// Prior to #1661 this file resolved ExecutionEngine/ServiceAccountName/
// Resources/WorkflowName/ActionType from a DataStorage catalog round-trip
// (Issue #518/#650) into wfe.Status, and additionally overrode
// WorkflowRef.ExecutionBundle/EngineConfig at runtime from that catalog
// entry. WorkflowRef is now RO's already-validated, CRD-embedded snapshot
// (Change 11c/11d) copied verbatim from AIAnalysis.Status.SelectedWorkflow —
// there is no DS entry left to consult, and mutating the spec at runtime is
// no longer appropriate.
//
// Post-firefight follow-up (Issue #1661 Change 11f): the former
// resolveWorkflowCatalog function used to mirror
// ExecutionEngine/ServiceAccountName/Resources from Spec.WorkflowRef into
// Status once during Pending, guarded by an idempotency check on
// Status.ExecutionEngine != "". That mirror step is now removed entirely —
// every consumer reads wfe.Spec.WorkflowRef.{ExecutionEngine,
// ServiceAccountName,Resources,ActionType} directly, since WorkflowRef is
// immutable from CRD-creation time (ADR-001 "self == oldSelf" CEL) and
// there is nothing left to "resolve" or guard against re-resolving. This
// closes the exact bug class that motivated the removal: ActionType was
// left off the original Status-mirror list and its Status.ActionType
// equivalent was silently never wired, breaking
// workflowexecution.execution.started's audit payload (event_data.action_type
// always empty) without any test catching it until a full-pipeline E2E
// journey exercised it. Reading straight from the immutable Spec snapshot
// removes the possibility of a consumer/mirror-step drifting apart again.
//
// WorkflowName remains permanently unset: WorkflowRef carries no such field
// (KA's autonomous selection path never emits a display name distinct from
// WorkflowID either), so there is no source to read regardless of which
// struct holds it. WorkflowID remains the functional/join key for SOC2
// CC8.1 reconstruction either way (IT-AW-1111-001).

// validateExecutionEngineResolved checks that the WFE's CRD-embedded
// WorkflowRef snapshot declares an execution engine. Name retained from the
// pre-Change-11f Status-mirroring design (git history/callers reference it
// as "resolved") even though there is no runtime resolution step left: RO's
// validateSelectedWorkflow (pkg/remediationorchestrator/creator/
// workflowexecution.go) already enforces this before the WFE is ever
// created, so this is a defensive fail-closed guard against corrupted or
// manually-constructed WorkflowExecution objects (e.g. test fixtures), not
// a real per-reconcile resolution step.
func (r *WorkflowExecutionReconciler) validateExecutionEngineResolved(wfe *workflowexecutionv1alpha1.WorkflowExecution) error {
	if wfe.Spec.WorkflowRef.ExecutionEngine != "" {
		return nil
	}
	return fmt.Errorf("execution engine not declared for WFE %s/%s — expected on spec.workflowRef at creation time", wfe.Namespace, wfe.Name)
}
