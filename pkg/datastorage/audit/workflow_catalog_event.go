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
	"encoding/json"

	"github.com/google/uuid"

	pkgaudit "github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// Workflow catalog event type constants (L-3 SOC2 Fix)
const (
	EventTypeWorkflowCreated = "datastorage.workflow.created"
	EventTypeWorkflowUpdated = "datastorage.workflow.updated"
)

// Event category and action constants (L-3 SOC2 Fix: compile-time safety)
const (
	EventCategoryWorkflow = "workflow" // Per OpenAPI schema (not "workflow_catalog")
	ActionCreate          = "create"
	ActionUpdate          = "update"
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
// DD-AUDIT-004 V2.0: Uses OpenAPI-generated typed schemas (no unstructured data)
func NewWorkflowCreatedAuditEvent(workflow *models.RemediationWorkflow) (*ogenclient.AuditEventRequest, error) {
	// Create OpenAPI audit event
	auditEvent := pkgaudit.NewAuditEventRequest()
	pkgaudit.SetEventType(auditEvent, EventTypeWorkflowCreated)
	pkgaudit.SetEventCategory(auditEvent, EventCategoryWorkflow)
	pkgaudit.SetEventAction(auditEvent, ActionCreate)
	pkgaudit.SetEventOutcome(auditEvent, pkgaudit.OutcomeSuccess)
	pkgaudit.SetActor(auditEvent, "service", "datastorage")
	pkgaudit.SetResource(auditEvent, "Workflow", workflow.WorkflowID)
	pkgaudit.SetCorrelationID(auditEvent, workflow.WorkflowID)
	auditEvent.Version = "1.0"

	// V2.0: Use OpenAPI-generated typed schema (eliminates map[string]interface{})
	// Convert workflow.Labels (MandatoryLabels struct) to map[string]interface{}
	labelsMap := make(map[string]interface{})
	if workflow.Labels.SignalType != "" {
		labelsMap["signal_type"] = workflow.Labels.SignalType
	}
	if workflow.Labels.Severity != "" {
		labelsMap["severity"] = workflow.Labels.Severity
	}
	if workflow.Labels.Component != "" {
		labelsMap["component"] = workflow.Labels.Component
	}
	// DD-WORKFLOW-001 v2.5: Environment is []string
	if len(workflow.Labels.Environment) > 0 {
		labelsMap["environment"] = workflow.Labels.Environment
	}
	if workflow.Labels.Priority != "" {
		labelsMap["priority"] = workflow.Labels.Priority
	}

	// Parse WorkflowID as UUID
	workflowUUID, err := uuid.Parse(workflow.WorkflowID)
	if err != nil {
		// Fallback: use zero UUID if parse fails
		workflowUUID = uuid.Nil
	}

	// Convert status enum
	status := ogenclient.WorkflowCatalogCreatedPayloadStatus(workflow.Status)

	payload := ogenclient.WorkflowCatalogCreatedPayload{
		WorkflowID:      workflowUUID,
		WorkflowName:    workflow.WorkflowName,
		Version:         workflow.Version,
		Status:          status,
		IsLatestVersion: workflow.IsLatestVersion,
		ExecutionEngine: string(workflow.ExecutionEngine), // Convert enum to string
		Name:            workflow.Name,
	}
	// Set optional fields using .SetTo()
	if workflow.Description.What != "" {
		payload.Description.SetTo(workflow.Description.What)
	}
	if len(labelsMap) > 0 {
		// Convert map[string]interface{} to map[string]jx.Raw
		labels := make(ogenclient.WorkflowCatalogCreatedPayloadLabels)
		for k, v := range labelsMap {
			// Marshal each value to JSON bytes (jx.Raw)
			jsonBytes, _ := json.Marshal(v)
			labels[k] = jsonBytes
		}
		payload.Labels.SetTo(labels)
	}

	// Use ogen union constructor (OGEN-MIGRATION)
	auditEvent.EventData = ogenclient.NewWorkflowCatalogCreatedPayloadAuditEventRequestEventData(payload)

	return auditEvent, nil
}

// NewWorkflowUpdatedAuditEvent creates an audit event for workflow updates
// BR-STORAGE-183: Audit workflow updates (business logic operation)
// DD-AUDIT-004 V2.0: Uses OpenAPI-generated typed schemas (no unstructured data)
func NewWorkflowUpdatedAuditEvent(workflowID string, updatedFields ogenclient.WorkflowCatalogUpdatedFields) (*ogenclient.AuditEventRequest, error) {
	// Create OpenAPI audit event
	auditEvent := pkgaudit.NewAuditEventRequest()
	pkgaudit.SetEventType(auditEvent, EventTypeWorkflowUpdated)
	pkgaudit.SetEventCategory(auditEvent, EventCategoryWorkflow)
	pkgaudit.SetEventAction(auditEvent, ActionUpdate)
	pkgaudit.SetEventOutcome(auditEvent, pkgaudit.OutcomeSuccess)
	pkgaudit.SetActor(auditEvent, "service", "datastorage")
	pkgaudit.SetResource(auditEvent, "Workflow", workflowID)
	pkgaudit.SetCorrelationID(auditEvent, workflowID)
	auditEvent.Version = "1.0"

	// V2.0: Use OpenAPI-generated typed schema (eliminates map[string]interface{})
	// Parse WorkflowID as UUID
	workflowUUID, err := uuid.Parse(workflowID)
	if err != nil {
		// Fallback: use zero UUID if parse fails
		workflowUUID = uuid.Nil
	}

	payload := ogenclient.WorkflowCatalogUpdatedPayload{
		WorkflowID:    workflowUUID,
		UpdatedFields: updatedFields,
	}

	// Use ogen union constructor (OGEN-MIGRATION)
	auditEvent.EventData = ogenclient.NewWorkflowCatalogUpdatedPayloadAuditEventRequestEventData(payload)

	return auditEvent, nil
}
