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

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	weclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"
)

// ========================================
// Issue #518: Runtime Execution Engine Resolution
// ========================================

// resolveWorkflowCatalog fetches all workflow metadata from the DS catalog in a
// single GetWorkflowByID call (Issue #650). Consolidates resolveExecutionEngine,
// resolveExecutionBundle, resolveDependencies, and GetWorkflowEngineConfig.
// Idempotent: returns nil immediately if the engine is already resolved.
func (r *WorkflowExecutionReconciler) resolveWorkflowCatalog(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (*weclient.WorkflowCatalogMetadata, error) {
	if wfe.Status.ExecutionEngine != "" {
		return nil, nil
	}

	if r.WorkflowQuerier == nil {
		return nil, fmt.Errorf("DataStorage workflow querier not available — cannot resolve workflow catalog for %s", wfe.Spec.WorkflowRef.WorkflowID)
	}

	workflowID := wfe.Spec.WorkflowRef.WorkflowID
	if workflowID == "" {
		return nil, fmt.Errorf("workflowRef.workflowId is empty — cannot resolve workflow catalog")
	}

	logger := log.FromContext(ctx)

	meta, err := r.WorkflowQuerier.ResolveWorkflowCatalogMetadata(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve workflow catalog from DS for workflow %s: %w", workflowID, err)
	}

	if meta.ExecutionEngine == "" {
		label := workflowID
		if meta.WorkflowName != "" {
			label = fmt.Sprintf("%s - %s", meta.WorkflowName, workflowID)
		}
		return nil, fmt.Errorf("no engine defined in remediation workflow %s", label)
	}

	wfe.Status.ExecutionEngine = meta.ExecutionEngine
	wfe.Status.ServiceAccountName = meta.ServiceAccountName
	// #1661 Change 3: audit-readability only (WorkflowID stays the functional/
	// join key); resolved once here, immutable thereafter, same as the two
	// fields above.
	wfe.Status.WorkflowName = meta.WorkflowName
	wfe.Status.ActionType = meta.ActionType
	// BR-WE-019 / DD-WE-008: resolved once during Pending (this function is a
	// no-op on subsequent reconciles per the idempotency guard above), applied
	// to the Job's "workflow" container by resourcesFor(). nil when the
	// catalog entry declares none (BestEffort QoS, unchanged behavior).
	wfe.Status.Resources = meta.Resources

	// WE-H1: In-memory spec mutation is acceptable here because ResolveWorkflowCatalogMetadata
	// is called on every Pending-phase reconcile. If the controller requeues in Pending,
	// the catalog lookup re-applies the override. Once the WFE leaves Pending, the bundle
	// value is already consumed (execution resource created).
	if meta.ExecutionBundle != "" {
		if wfe.Spec.WorkflowRef.ExecutionBundle != meta.ExecutionBundle {
			logger.Info("Overriding execution bundle from DS catalog",
				"specBundle", wfe.Spec.WorkflowRef.ExecutionBundle,
				"catalogBundle", meta.ExecutionBundle,
				"workflowID", workflowID,
			)
		}
		wfe.Spec.WorkflowRef.ExecutionBundle = meta.ExecutionBundle
		if meta.ExecutionBundleDigest != "" {
			wfe.Spec.WorkflowRef.ExecutionBundleDigest = meta.ExecutionBundleDigest
		}
	}

	if wfe.Spec.WorkflowRef.EngineConfig == nil && meta.EngineConfig != nil {
		logger.Info("Resolved engineConfig from DS catalog",
			"workflowID", workflowID,
			"engine", meta.ExecutionEngine)
		wfe.Spec.WorkflowRef.EngineConfig = &apiextensionsv1.JSON{Raw: meta.EngineConfig}
	}

	return meta, nil
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
