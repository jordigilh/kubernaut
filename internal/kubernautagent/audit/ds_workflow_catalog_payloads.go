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
	"github.com/google/uuid"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// WorkflowActionAuditResult is the minimized Step 1 action choice captured
// for Data Storage audit. Keep it separate from the LLM response DTO so audit
// payload changes cannot accidentally change model-visible data.
type WorkflowActionAuditResult struct {
	ActionType    string                         `json:"action_type"`
	Description   WorkflowActionAuditDescription `json:"description"`
	WorkflowCount int                            `json:"workflow_count"`
}

// WorkflowActionAuditDescription contains only the taxonomy guidance shown
// to the agent during Step 1.
type WorkflowActionAuditDescription struct {
	What          string `json:"what"`
	WhenToUse     string `json:"when_to_use"`
	WhenNotToUse  string `json:"when_not_to_use,omitempty"`
	Preconditions string `json:"preconditions,omitempty"`
}

// WorkflowCandidateAuditResult is the minimized Step 2 ranking evidence.
// Parameters and execution details are intentionally not represented.
type WorkflowCandidateAuditResult struct {
	WorkflowID uuid.UUID `json:"workflow_id"`
	Title      string    `json:"title"`
	Version    string    `json:"version,omitempty"`
	Rank       int       `json:"rank"`
	FinalScore float64   `json:"final_score"`
}

// ========================================
// WORKFLOW CATALOG DISCOVERY AUDIT PAYLOADS (Issue #1677 Phase 2c)
// ========================================
// Authority: DD-AUDIT-009 (event-specific typed result variants),
// DD-WORKFLOW-019 (KA owns discovery directly), amending BR-AUDIT-023/
// DD-WORKFLOW-014's "who generates" language: KA, not DS, now
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
	returned := dataInt(event.Data, "returned")
	if _, ok := event.Data["returned"]; !ok {
		returned = dataInt(event.Data, "total_count")
	}
	payload := ogenclient.WorkflowActionsListedAuditPayload{
		EventType: ogenclient.WorkflowActionsListedAuditPayloadEventTypeWorkflowCatalogActionsListed,
		Query:     buildDiscoveryQueryMetadata(event),
		Results: ogenclient.WorkflowActionsResultsMetadata{
			TotalFound: int32(dataInt(event.Data, "total_count")),
			Returned:   int32(returned),
			Actions:    workflowActionResults(event.Data["actions"]),
		},
		SearchMetadata: buildSearchExecutionMetadata(event),
	}
	return ogenclient.NewWorkflowActionsListedAuditPayloadAuditEventRequestEventData(payload)
}

func buildWorkflowsListedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	returned := dataInt(event.Data, "returned")
	if _, ok := event.Data["returned"]; !ok {
		returned = dataInt(event.Data, "total_count")
	}
	payload := ogenclient.WorkflowCandidatesListedAuditPayload{
		EventType: ogenclient.WorkflowCandidatesListedAuditPayloadEventTypeWorkflowCatalogWorkflowsListed,
		Query:     buildDiscoveryQueryMetadata(event),
		Results: ogenclient.WorkflowCandidatesResultsMetadata{
			TotalFound: int32(dataInt(event.Data, "total_count")),
			Returned:   int32(returned),
			Workflows:  workflowCandidateResults(event.Data["workflows"]),
		},
		SearchMetadata: buildSearchExecutionMetadata(event),
	}
	if actionType := dataString(event.Data, "action_type"); actionType != "" {
		payload.ActionType.SetTo(actionType)
	}
	return ogenclient.NewWorkflowCandidatesListedAuditPayloadAuditEventRequestEventData(payload)
}

func buildWorkflowRetrievedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := buildWorkflowDiscoveryPayload(event, ogenclient.WorkflowDiscoveryAuditPayloadEventTypeWorkflowCatalogWorkflowRetrieved)
	return ogenclient.NewAuditEventRequestEventDataWorkflowCatalogWorkflowRetrievedAuditEventRequestEventData(payload)
}

func buildSelectionValidatedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := buildWorkflowDiscoveryPayload(event, ogenclient.WorkflowDiscoveryAuditPayloadEventTypeWorkflowCatalogSelectionValidated)
	return ogenclient.NewAuditEventRequestEventDataWorkflowCatalogSelectionValidatedAuditEventRequestEventData(payload)
}

// buildWorkflowDiscoveryPayload builds the legacy WorkflowDiscoveryAuditPayload
// shared by the two Step 3 discovery events, mirroring DS's buildDiscoveryPayload
// (pkg/datastorage/audit/workflow_discovery_event.go) field-for-field, but
// reading from the flat event.Data map instead of a typed
// *models.WorkflowDiscoveryFilters (KA's audit package does not import
// pkg/datastorage/models, to avoid runtime coupling to DS's domain types).
func buildWorkflowDiscoveryPayload(event *AuditEvent, eventType ogenclient.WorkflowDiscoveryAuditPayloadEventType) ogenclient.WorkflowDiscoveryAuditPayload {
	totalCount := dataInt(event.Data, "total_count")

	return ogenclient.WorkflowDiscoveryAuditPayload{
		EventType: eventType,
		Query:     buildDiscoveryQueryMetadata(event),
		Results: ogenclient.ResultsMetadata{
			TotalFound: int32(totalCount),
			Returned:   int32(totalCount),
		},
		SearchMetadata: buildSearchExecutionMetadata(event),
	}
}

func buildDiscoveryQueryMetadata(event *AuditEvent) ogenclient.QueryMetadata {
	pageLimit := dataInt(event.Data, "limit")
	if pageLimit == 0 {
		pageLimit = dataInt(event.Data, "total_count")
	}
	query := ogenclient.QueryMetadata{TopK: int32(pageLimit)}
	if _, ok := event.Data["offset"]; ok {
		query.Offset.SetTo(int32(dataInt(event.Data, "offset")))
	}
	if hasDiscoveryFilters(event.Data) {
		filters := ogenclient.WorkflowSearchFilters{
			Severity:    ogenclient.WorkflowSearchFiltersSeverity(dataString(event.Data, "severity")),
			Component:   dataString(event.Data, "component"),
			Environment: dataString(event.Data, "environment"),
			Priority:    ogenclient.WorkflowSearchFiltersPriority(dataString(event.Data, "priority")),
		}
		if labels, ok := detectedLabelsFromJSON(event.Data); ok {
			filters.DetectedLabels.SetTo(labels)
		}
		query.Filters.SetTo(filters)
	}
	return query
}

func buildSearchExecutionMetadata(event *AuditEvent) ogenclient.SearchExecutionMetadata {
	return ogenclient.SearchExecutionMetadata{DurationMs: int64(dataInt(event.Data, "duration_ms"))}
}

func workflowActionResults(value interface{}) []ogenclient.WorkflowActionResultAudit {
	actions, ok := value.([]WorkflowActionAuditResult)
	if !ok {
		return []ogenclient.WorkflowActionResultAudit{}
	}
	results := make([]ogenclient.WorkflowActionResultAudit, 0, len(actions))
	for _, action := range actions {
		description := ogenclient.WorkflowActionDescriptionAudit{
			What:      action.Description.What,
			WhenToUse: action.Description.WhenToUse,
		}
		if action.Description.WhenNotToUse != "" {
			description.WhenNotToUse.SetTo(action.Description.WhenNotToUse)
		}
		if action.Description.Preconditions != "" {
			description.Preconditions.SetTo(action.Description.Preconditions)
		}
		results = append(results, ogenclient.WorkflowActionResultAudit{
			ActionType: action.ActionType, Description: description, WorkflowCount: int32(action.WorkflowCount),
		})
	}
	return results
}

func workflowCandidateResults(value interface{}) []ogenclient.WorkflowResultAudit {
	candidates, ok := value.([]WorkflowCandidateAuditResult)
	if !ok {
		return []ogenclient.WorkflowResultAudit{}
	}
	results := make([]ogenclient.WorkflowResultAudit, 0, len(candidates))
	for _, candidate := range candidates {
		workflow := ogenclient.WorkflowResultAudit{
			WorkflowID: candidate.WorkflowID,
			Title:      candidate.Title,
			Rank:       int32(candidate.Rank),
			Scoring:    ogenclient.ScoringV1Audit{Confidence: candidate.FinalScore},
		}
		if candidate.Version != "" {
			workflow.Version.SetTo(candidate.Version)
		}
		workflow.FinalScore.SetTo(candidate.FinalScore)
		results = append(results, workflow)
	}
	return results
}

func detectedLabelsFromJSON(data map[string]interface{}) (ogenclient.DetectedLabels, bool) {
	raw := dataString(data, "detected_labels_json")
	if raw == "" {
		return ogenclient.DetectedLabels{}, false
	}
	return detectedLabelsFromJSONText(raw)
}

// hasDiscoveryFilters reports whether any signal-context filter dimension
// was recorded on the event, matching the `if filters != nil` gate DS's
// buildDiscoveryPayload used against its typed *models.WorkflowDiscoveryFilters.
func hasDiscoveryFilters(data map[string]interface{}) bool {
	return dataString(data, "severity") != "" ||
		dataString(data, "component") != "" ||
		dataString(data, "environment") != "" ||
		dataString(data, "priority") != "" ||
		dataBool(data, "detected_labels_present")
}
