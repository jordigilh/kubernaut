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

package audit

import (
	"fmt"

	pkgaudit "github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// THREE-STEP WORKFLOW DISCOVERY AUDIT EVENTS
// ========================================
// Authority: DD-WORKFLOW-014 v3.0 (Workflow Selection Audit Trail)
// Authority: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing)
// Authority: DD-HAPI-017 (Three-Step Workflow Discovery Integration)
// Business Requirement: BR-AUDIT-023 (Per-Step Audit Events)
// Business Requirement: BR-HAPI-017-005 (remediationId Propagation)
//
// Four discovery audit events:
// 1. workflow.catalog.actions_listed - Step 1 completed
// 2. workflow.catalog.workflows_listed - Step 2 completed
// 3. workflow.catalog.workflow_retrieved - Step 3 completed
// 4. workflow.catalog.selection_validated - Post-selection validation
// ========================================

// Discovery event type constants
const (
	EventTypeActionsListed      = "workflow.catalog.actions_listed"
	EventTypeWorkflowsListed    = "workflow.catalog.workflows_listed"
	EventTypeWorkflowRetrieved  = "workflow.catalog.workflow_retrieved"
	EventTypeSelectionValidated = "workflow.catalog.selection_validated"
)

// Discovery event action constants
const (
	ActionDiscovery = "discovery"
	ActionRetrieve  = "retrieve"
	ActionValidate  = "validate"
)

// newBaseDiscoveryEvent creates a base audit event with fields common to all discovery events.
// Reduces duplication across the 4 discovery event constructors.
func newBaseDiscoveryEvent(eventType, action string) *ogenclient.AuditEventRequest {
	event := pkgaudit.NewAuditEventRequest()
	pkgaudit.SetEventType(event, eventType)
	pkgaudit.SetEventCategory(event, EventCategoryWorkflow)
	pkgaudit.SetEventAction(event, action)
	pkgaudit.SetEventOutcome(event, pkgaudit.OutcomeSuccess)
	pkgaudit.SetActor(event, "service", "datastorage")
	event.Version = "1.0"
	return event
}

// setCorrelationIDFromFilters sets the correlation ID from filters.RemediationID,
// falling back to fallbackID when no remediation ID is present.
// BR-HAPI-017-005: remediationId propagation for audit trail correlation.
func setCorrelationIDFromFilters(event *ogenclient.AuditEventRequest, filters *models.WorkflowDiscoveryFilters, fallbackID string) {
	if filters != nil && filters.RemediationID != "" {
		pkgaudit.SetCorrelationID(event, filters.RemediationID)
	} else if fallbackID != "" {
		pkgaudit.SetCorrelationID(event, fallbackID)
	}
}

// NewActionsListedAuditEvent creates an audit event for Step 1: list available actions.
// Emitted when DS returns action types matching signal context.
// BR-AUDIT-023: Per-step audit event for three-step discovery
func NewActionsListedAuditEvent(filters *models.WorkflowDiscoveryFilters, totalCount int) (*ogenclient.AuditEventRequest, error) {
	event := newBaseDiscoveryEvent(EventTypeActionsListed, ActionDiscovery)
	setCorrelationIDFromFilters(event, filters, "")

	payload := buildDiscoveryPayload(
		ogenclient.WorkflowDiscoveryAuditPayloadEventTypeWorkflowCatalogActionsListed,
		filters, totalCount, "",
	)
	event.EventData = ogenclient.NewAuditEventRequestEventDataWorkflowCatalogActionsListedAuditEventRequestEventData(payload)

	return event, nil
}

// NewWorkflowsListedAuditEvent creates an audit event for Step 2: list workflows by action type.
// Emitted when DS returns workflows for a specific action type.
func NewWorkflowsListedAuditEvent(actionType string, filters *models.WorkflowDiscoveryFilters, totalCount int) (*ogenclient.AuditEventRequest, error) {
	event := newBaseDiscoveryEvent(EventTypeWorkflowsListed, ActionDiscovery)
	setCorrelationIDFromFilters(event, filters, "")

	payload := buildDiscoveryPayload(
		ogenclient.WorkflowDiscoveryAuditPayloadEventTypeWorkflowCatalogWorkflowsListed,
		filters, totalCount, actionType,
	)
	event.EventData = ogenclient.NewAuditEventRequestEventDataWorkflowCatalogWorkflowsListedAuditEventRequestEventData(payload)

	return event, nil
}

// NewWorkflowRetrievedAuditEvent creates an audit event for Step 3: get workflow with context filters.
// Emitted when DS returns a specific workflow (security gate passed).
func NewWorkflowRetrievedAuditEvent(workflowID string, filters *models.WorkflowDiscoveryFilters) (*ogenclient.AuditEventRequest, error) {
	event := newBaseDiscoveryEvent(EventTypeWorkflowRetrieved, ActionRetrieve)
	pkgaudit.SetResource(event, "Workflow", workflowID)
	setCorrelationIDFromFilters(event, filters, workflowID)

	payload := buildDiscoveryPayload(
		ogenclient.WorkflowDiscoveryAuditPayloadEventTypeWorkflowCatalogWorkflowRetrieved,
		filters, 1, "",
	)
	event.EventData = ogenclient.NewAuditEventRequestEventDataWorkflowCatalogWorkflowRetrievedAuditEventRequestEventData(payload)

	return event, nil
}

// NewSelectionValidatedAuditEvent creates an audit event for post-selection validation.
// Emitted when DS validates a HAPI-selected workflow against context filters.
func NewSelectionValidatedAuditEvent(workflowID string, filters *models.WorkflowDiscoveryFilters, valid bool) (*ogenclient.AuditEventRequest, error) {
	event := newBaseDiscoveryEvent(EventTypeSelectionValidated, ActionValidate)
	if !valid {
		pkgaudit.SetEventOutcome(event, pkgaudit.OutcomeFailure)
	}
	pkgaudit.SetResource(event, "Workflow", workflowID)
	setCorrelationIDFromFilters(event, filters, workflowID)

	resultCount := 0
	if valid {
		resultCount = 1
	}
	payload := buildDiscoveryPayload(
		ogenclient.WorkflowDiscoveryAuditPayloadEventTypeWorkflowCatalogSelectionValidated,
		filters, resultCount, "",
	)
	event.EventData = ogenclient.NewAuditEventRequestEventDataWorkflowCatalogSelectionValidatedAuditEventRequestEventData(payload)

	return event, nil
}

// buildDiscoveryPayload creates a WorkflowDiscoveryAuditPayload encoding context filters
// and result counts for the audit trail.
func buildDiscoveryPayload(eventType ogenclient.WorkflowDiscoveryAuditPayloadEventType, filters *models.WorkflowDiscoveryFilters, totalCount int, actionType string) ogenclient.WorkflowDiscoveryAuditPayload {
	var searchFilters ogenclient.OptWorkflowSearchFilters
	if filters != nil {
		wsf := ogenclient.WorkflowSearchFilters{
			Severity:    ogenclient.WorkflowSearchFiltersSeverity(filters.Severity),
			Component:   filters.Component,
			Environment: filters.Environment,
			Priority:    ogenclient.WorkflowSearchFiltersPriority(filters.Priority),
		}
		if actionType != "" {
			wsf.SignalType = fmt.Sprintf("discovery:action_type=%s", actionType)
		} else {
			wsf.SignalType = "discovery"
		}
		searchFilters.SetTo(wsf)
	}

	return ogenclient.WorkflowDiscoveryAuditPayload{
		EventType: eventType,
		Query: ogenclient.QueryMetadata{
			TopK:    int32(totalCount),
			Filters: searchFilters,
		},
		Results: ogenclient.ResultsMetadata{
			TotalFound: int32(totalCount),
			Returned:   int32(totalCount),
		},
		SearchMetadata: ogenclient.SearchExecutionMetadata{
			DurationMs: 0,
		},
	}
}
