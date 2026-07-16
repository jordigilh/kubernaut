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
	"context"
	"fmt"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	weclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"
)

// ========================================
// Issue #1661 Change 11e (DD-WORKFLOW-018): CRD-Embedded Execution Snapshot
// ========================================

// resolveWorkflowCatalog copies the execution-engine snapshot from
// wfe.Spec.WorkflowRef onto Status. Prior to #1661 this resolved
// ExecutionEngine/ServiceAccountName/Resources/WorkflowName/ActionType from a
// DataStorage catalog round-trip (Issue #518/#650) and additionally
// overrode WorkflowRef.ExecutionBundle/EngineConfig at runtime from that
// catalog entry. WorkflowRef is now RO's already-validated, CRD-embedded
// snapshot (Change 11c/11d) copied verbatim from AIAnalysis.Status.SelectedWorkflow
// -- there is no DS entry left to consult, and mutating the spec at runtime is
// no longer appropriate. WorkflowName/ActionType are deliberately NOT set here:
// WorkflowRef carries no such fields (KA's autonomous selection path never
// emits them either), so Status.WorkflowName/ActionType simply remain empty. A
// fast-follow issue tracks wiring these end-to-end for audit readability; they
// are not required for SOC2 CC8.1 reconstruction, which joins on workflow_id
// against the immutable workflow_content already captured in the Postgres
// audit_events ledger (IT-AW-1111-001).
// Idempotent: returns nil immediately if the engine is already resolved.
func (r *WorkflowExecutionReconciler) resolveWorkflowCatalog(_ context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (*weclient.WorkflowCatalogMetadata, error) {
	if wfe.Status.ExecutionEngine != "" {
		return nil, nil
	}

	ref := wfe.Spec.WorkflowRef
	if ref.ExecutionEngine == "" {
		return nil, fmt.Errorf("no engine defined in remediation workflow %s", ref.WorkflowID)
	}

	wfe.Status.ExecutionEngine = ref.ExecutionEngine
	wfe.Status.ServiceAccountName = ref.ServiceAccountName
	// BR-WE-019 / DD-WE-008: resolved once during Pending (this function is a
	// no-op on subsequent reconciles per the idempotency guard above), applied
	// to the Job's "workflow" container by resourcesFor(). nil when the
	// snapshot declares none (BestEffort QoS, unchanged behavior).
	wfe.Status.Resources = ref.Resources

	return nil, nil
}

// resolveExecutionEngine returns the cached execution engine from the WFE status.
// In non-Pending phases the engine was already resolved during Pending and persisted
// to wfe.Status.ExecutionEngine. Returns an error only if the engine is missing,
// which indicates a programming error (Pending handler should have set it).
func (r *WorkflowExecutionReconciler) resolveExecutionEngine(_ context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (string, error) {
	if wfe.Status.ExecutionEngine != "" {
		return wfe.Status.ExecutionEngine, nil
	}
	return "", fmt.Errorf("execution engine not resolved for WFE %s/%s — expected to be set during Pending phase", wfe.Namespace, wfe.Name)
}
