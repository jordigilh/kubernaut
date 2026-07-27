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

package custom

import (
	"context"

	kaaudit "github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// WORKFLOW CATALOG DISCOVERY AUDIT EMISSION (Issue #1677 Phase 2d)
// ========================================
// Authority: DD-WORKFLOW-019 (KA owns discovery directly), BR-AUDIT-023.
// KA, not DS, now generates the 4 workflow.catalog.* discovery events --
// this file wires the 3 custom MCP tools into the audit infrastructure
// built in Phase 2c (internal/kubernautagent/audit/{emitter,ds_store,
// ds_workflow_catalog_payloads}.go). Mirrors DS's former
// pkg/datastorage/audit/workflow_discovery_event.go emission call sites
// (workflow_discovery_handlers.go / workflow_query_handlers.go), now
// reading from Go-native filters instead of query params.
// ========================================

// applyDiscoveryFilterData records the signal-context filter dimensions
// used for this query onto the audit event's Data map -- read back by
// ds_workflow_catalog_payloads.go's hasDiscoveryFilters/buildWorkflowDiscoveryPayload.
func applyDiscoveryFilterData(data map[string]interface{}, filters *models.WorkflowDiscoveryFilters) {
	if filters == nil {
		return
	}
	data["severity"] = filters.Severity
	data["component"] = filters.Component
	data["environment"] = filters.Environment
	data["priority"] = filters.Priority
}

// correlationIDFromFilters mirrors DS's setCorrelationIDFromFilters:
// prefer filters.RemediationID, falling back to fallbackID (e.g. the
// workflowID itself for Step 3) when absent.
func correlationIDFromFilters(filters *models.WorkflowDiscoveryFilters, fallback string) string {
	if filters != nil && filters.RemediationID != "" {
		return filters.RemediationID
	}
	return fallback
}

// emitAuditEvent emits workflow.catalog.actions_listed (BR-AUDIT-023, Step 1).
func (t *listActionsTool) emitAuditEvent(ctx context.Context, filters *models.WorkflowDiscoveryFilters, totalCount int, durationMs int64) {
	if t.auditStore == nil {
		return
	}
	ev := kaaudit.NewEvent(kaaudit.EventTypeActionsListed, correlationIDFromFilters(filters, ""),
		kaaudit.WithEventCategory(kaaudit.WorkflowCatalogEventCategory))
	ev.EventAction = kaaudit.ActionDiscovery
	ev.EventOutcome = kaaudit.OutcomeSuccess
	ev.Data["total_count"] = totalCount
	ev.Data["duration_ms"] = durationMs
	applyDiscoveryFilterData(ev.Data, filters)
	kaaudit.StoreBestEffort(ctx, t.auditStore, ev, t.logger)
}

// emitAuditEvent emits workflow.catalog.workflows_listed (BR-AUDIT-023, Step 2).
func (t *listWorkflowsTool) emitAuditEvent(ctx context.Context, actionType string, filters *models.WorkflowDiscoveryFilters, totalCount int, durationMs int64) {
	if t.auditStore == nil {
		return
	}
	ev := kaaudit.NewEvent(kaaudit.EventTypeWorkflowsListed, correlationIDFromFilters(filters, ""),
		kaaudit.WithEventCategory(kaaudit.WorkflowCatalogEventCategory))
	ev.EventAction = kaaudit.ActionDiscovery
	ev.EventOutcome = kaaudit.OutcomeSuccess
	ev.Data["total_count"] = totalCount
	ev.Data["duration_ms"] = durationMs
	ev.Data["action_type"] = actionType
	applyDiscoveryFilterData(ev.Data, filters)
	kaaudit.StoreBestEffort(ctx, t.auditStore, ev, t.logger)
}

// emitAuditEvents emits workflow.catalog.workflow_retrieved and
// workflow.catalog.selection_validated together (BR-AUDIT-023, Step 3),
// mirroring DS's former emitWorkflowRetrievedAuditEvents: both events are
// emitted only when context filters are present (DD-WORKFLOW-014 v3.0 --
// their presence signals KA is validating a prior selection), and reaching
// this call site means the security gate already passed, so
// selection_validated always records a successful validation.
func (t *getWorkflowTool) emitAuditEvents(ctx context.Context, workflowID string, filters *models.WorkflowDiscoveryFilters, durationMs int64) {
	if t.auditStore == nil || filters == nil || !filters.HasContextFilters() {
		return
	}

	retrieved := kaaudit.NewEvent(kaaudit.EventTypeWorkflowRetrieved, correlationIDFromFilters(filters, workflowID),
		kaaudit.WithEventCategory(kaaudit.WorkflowCatalogEventCategory),
		kaaudit.WithResource("Workflow", workflowID))
	retrieved.EventAction = kaaudit.ActionRetrieve
	retrieved.EventOutcome = kaaudit.OutcomeSuccess
	retrieved.Data["total_count"] = 1
	retrieved.Data["duration_ms"] = durationMs
	applyDiscoveryFilterData(retrieved.Data, filters)
	kaaudit.StoreBestEffort(ctx, t.auditStore, retrieved, t.logger)

	validated := kaaudit.NewEvent(kaaudit.EventTypeSelectionValidated, correlationIDFromFilters(filters, workflowID),
		kaaudit.WithEventCategory(kaaudit.WorkflowCatalogEventCategory),
		kaaudit.WithResource("Workflow", workflowID))
	validated.EventAction = kaaudit.ActionValidate
	validated.EventOutcome = kaaudit.OutcomeSuccess
	validated.Data["total_count"] = 1
	validated.Data["duration_ms"] = durationMs
	applyDiscoveryFilterData(validated.Data, filters)
	kaaudit.StoreBestEffort(ctx, t.auditStore, validated, t.logger)
}
