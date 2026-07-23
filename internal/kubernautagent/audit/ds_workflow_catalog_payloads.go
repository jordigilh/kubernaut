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

package audit

import (
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// WORKFLOW CATALOG DISCOVERY AUDIT PAYLOADS (Issue #1677 Phase 2c)
// ========================================
// Authority: DD-WORKFLOW-019 (KA owns discovery directly), amending
// BR-AUDIT-023/DD-WORKFLOW-014's "who generates" language: KA, not DS, now
// emits these 4 events (workflow.catalog.{actions_listed,workflows_listed,
// workflow_retrieved,selection_validated}). Reimplemented independently of
// DS's pkg/datastorage/audit/workflow_discovery_event.go constructors
// (deliberately not imported from KA) so actor attribution falls through to
// ds_store.go/ds_buffered_store.go's existing "kubernaut-agent" default
// instead of DS's hardcoded "datastorage".
//
// Field values are read from the flat AuditEvent.Data map, matching every
// other eventDataBuilder in this package (see ds_payloads.go) -- the 3
// custom MCP tools (Phase 2d) populate these keys when constructing the
// event via audit.NewEvent(..., WithEventCategory(WorkflowCatalogEventCategory), ...).
// ========================================

func buildActionsListedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := buildWorkflowDiscoveryPayload(event, ogenclient.WorkflowDiscoveryAuditPayloadEventTypeWorkflowCatalogActionsListed)
	return ogenclient.NewAuditEventRequestEventDataWorkflowCatalogActionsListedAuditEventRequestEventData(payload)
}

func buildWorkflowsListedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := buildWorkflowDiscoveryPayload(event, ogenclient.WorkflowDiscoveryAuditPayloadEventTypeWorkflowCatalogWorkflowsListed)
	return ogenclient.NewAuditEventRequestEventDataWorkflowCatalogWorkflowsListedAuditEventRequestEventData(payload)
}

func buildWorkflowRetrievedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := buildWorkflowDiscoveryPayload(event, ogenclient.WorkflowDiscoveryAuditPayloadEventTypeWorkflowCatalogWorkflowRetrieved)
	return ogenclient.NewAuditEventRequestEventDataWorkflowCatalogWorkflowRetrievedAuditEventRequestEventData(payload)
}

func buildSelectionValidatedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := buildWorkflowDiscoveryPayload(event, ogenclient.WorkflowDiscoveryAuditPayloadEventTypeWorkflowCatalogSelectionValidated)
	return ogenclient.NewAuditEventRequestEventDataWorkflowCatalogSelectionValidatedAuditEventRequestEventData(payload)
}

// buildWorkflowDiscoveryPayload builds the WorkflowDiscoveryAuditPayload
// shared by all 4 discovery events, mirroring DS's buildDiscoveryPayload
// (pkg/datastorage/audit/workflow_discovery_event.go) field-for-field, but
// reading from the flat event.Data map instead of a typed
// *models.WorkflowDiscoveryFilters (KA's audit package does not import
// pkg/datastorage/models, to avoid runtime coupling to DS's domain types).
func buildWorkflowDiscoveryPayload(event *AuditEvent, eventType ogenclient.WorkflowDiscoveryAuditPayloadEventType) ogenclient.WorkflowDiscoveryAuditPayload {
	totalCount := dataInt(event.Data, "total_count")

	var searchFilters ogenclient.OptWorkflowSearchFilters
	if hasDiscoveryFilters(event.Data) {
		wsf := ogenclient.WorkflowSearchFilters{
			Severity:    ogenclient.WorkflowSearchFiltersSeverity(dataString(event.Data, "severity")),
			Component:   dataString(event.Data, "component"),
			Environment: dataString(event.Data, "environment"),
			Priority:    ogenclient.WorkflowSearchFiltersPriority(dataString(event.Data, "priority")),
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
			DurationMs: int64(dataInt(event.Data, "duration_ms")),
		},
	}
}

// hasDiscoveryFilters reports whether any signal-context filter dimension
// was recorded on the event, matching the `if filters != nil` gate DS's
// buildDiscoveryPayload used against its typed *models.WorkflowDiscoveryFilters.
func hasDiscoveryFilters(data map[string]interface{}) bool {
	return dataString(data, "severity") != "" ||
		dataString(data, "component") != "" ||
		dataString(data, "environment") != "" ||
		dataString(data, "priority") != ""
}
