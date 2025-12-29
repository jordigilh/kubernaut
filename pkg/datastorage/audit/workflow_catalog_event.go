/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF the License governing permissions and
limitations under the License.
*/

package audit

import (
	pkgaudit "github.com/jordigilh/kubernaut/pkg/audit"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// WORKFLOW CATALOG AUDIT EVENTS
// ========================================
// BR-STORAGE-183: Workflow Catalog Operation Auditing
// DD-AUDIT-002 V2.0.1: Workflow catalog operations are business logic
//
// These functions create audit events for workflow catalog management operations:
// - datastorage.workflow.created - Workflow added to catalog
// - datastorage.workflow.updated - Workflow mutable fields updated (including disable via status change)
//
// Rationale: Workflow operations involve business logic (state changes, version management)
// unlike pure CRUD operations (audit persistence) which are redundant to audit.
//
// Note: Workflow disabling is captured via workflow.updated with status="disabled" in updated_fields.
// ========================================

// NewWorkflowCreatedAuditEvent creates an audit event for workflow creation
// BR-STORAGE-183: Audit workflow creation (business logic operation)
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
func NewWorkflowCreatedAuditEvent(workflow *models.RemediationWorkflow) (*dsgen.AuditEventRequest, error) {
	// Create OpenAPI audit event
	auditEvent := pkgaudit.NewAuditEventRequest()
	pkgaudit.SetEventType(auditEvent, "datastorage.workflow.created")
	pkgaudit.SetEventCategory(auditEvent, "workflow_catalog")
	pkgaudit.SetEventAction(auditEvent, "create")
	pkgaudit.SetEventOutcome(auditEvent, pkgaudit.OutcomeSuccess)
	pkgaudit.SetActor(auditEvent, "service", "datastorage")
	pkgaudit.SetResource(auditEvent, "Workflow", workflow.WorkflowID)
	pkgaudit.SetCorrelationID(auditEvent, workflow.WorkflowID)
	auditEvent.Version = "1.0"

	// Event data payload
	payload := map[string]interface{}{
		"workflow_id":       workflow.WorkflowID,
		"workflow_name":     workflow.WorkflowName,
		"version":           workflow.Version,
		"status":            workflow.Status,
		"is_latest_version": workflow.IsLatestVersion,
		"execution_engine":  workflow.ExecutionEngine,
		"name":              workflow.Name,
		"description":       workflow.Description,
		"labels":            workflow.Labels,
	}

	// Create common envelope format event_data
	eventData := pkgaudit.NewEventData("datastorage", "workflow_created", "success", payload)
	eventDataMap, err := pkgaudit.EnvelopeToMap(eventData)
	if err != nil {
		return nil, err
	}
	pkgaudit.SetEventData(auditEvent, eventDataMap)

	return auditEvent, nil
}

// NewWorkflowUpdatedAuditEvent creates an audit event for workflow updates
// BR-STORAGE-183: Audit workflow updates (business logic operation)
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
func NewWorkflowUpdatedAuditEvent(workflowID string, updatedFields map[string]interface{}) (*dsgen.AuditEventRequest, error) {
	// Create OpenAPI audit event
	auditEvent := pkgaudit.NewAuditEventRequest()
	pkgaudit.SetEventType(auditEvent, "datastorage.workflow.updated")
	pkgaudit.SetEventCategory(auditEvent, "workflow_catalog")
	pkgaudit.SetEventAction(auditEvent, "update")
	pkgaudit.SetEventOutcome(auditEvent, pkgaudit.OutcomeSuccess)
	pkgaudit.SetActor(auditEvent, "service", "datastorage")
	pkgaudit.SetResource(auditEvent, "Workflow", workflowID)
	pkgaudit.SetCorrelationID(auditEvent, workflowID)
	auditEvent.Version = "1.0"

	// Event data payload
	payload := map[string]interface{}{
		"workflow_id":    workflowID,
		"updated_fields": updatedFields,
	}

	// Create common envelope format event_data
	eventData := pkgaudit.NewEventData("datastorage", "workflow_updated", "success", payload)
	eventDataMap, err := pkgaudit.EnvelopeToMap(eventData)
	if err != nil {
		return nil, err
	}
	pkgaudit.SetEventData(auditEvent, eventDataMap)

	return auditEvent, nil
}
