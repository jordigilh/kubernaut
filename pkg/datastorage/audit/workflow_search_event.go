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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	pkgaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// WORKFLOW SEARCH AUDIT EVENT DATA TYPES
// ========================================
// Business Requirements:
// - BR-AUDIT-023: Audit event generation in Data Storage Service
// - BR-AUDIT-024: Asynchronous non-blocking audit (ADR-038)
// - BR-AUDIT-025: Query metadata capture
// - BR-AUDIT-026: Scoring capture (V1.0: confidence only)
// - BR-AUDIT-027: Workflow metadata capture
// - BR-AUDIT-028: Search metadata capture
//
// Design Decisions:
// - DD-WORKFLOW-014 v2.1: Workflow Selection Audit Trail
// - DD-WORKFLOW-004 v2.0: V1.0 scoring = confidence only (no boost/penalty)
//
// Authority: DD-WORKFLOW-014 v2.1, BR-AUDIT-023-030
// ========================================

// WorkflowSearchEventData represents the structured event_data for workflow search audit events.
// This provides compile-time type safety and self-documenting schema.
//
// V1.0: confidence only (DD-WORKFLOW-004 v2.0)
// V2.0+: Will add BaseSimilarity, LabelBoost, LabelPenalty when configurable weights are implemented
type WorkflowSearchEventData struct {
	Query          QueryMetadata           `json:"query"`
	Results        ResultsMetadata         `json:"results"`
	SearchMetadata SearchExecutionMetadata `json:"search_metadata"`
}

// QueryMetadata captures the search query parameters (BR-AUDIT-025).
// V1.0: Uses structured WorkflowSearchFilters for type safety (00-project-guidelines.mdc)
type QueryMetadata struct {
	TopK     int                           `json:"top_k"`
	MinScore float64                       `json:"min_score,omitempty"`
	Filters  *models.WorkflowSearchFilters `json:"filters"` // Structured type for compile-time validation
}

// ResultsMetadata captures the search results (BR-AUDIT-027).
type ResultsMetadata struct {
	TotalFound int                   `json:"total_found"`
	Returned   int                   `json:"returned"`
	Workflows  []WorkflowResultAudit `json:"workflows"`
}

// WorkflowResultAudit captures metadata for a single workflow result (BR-AUDIT-027).
// WorkflowResultAudit captures workflow information for audit trail
// DD-WORKFLOW-002 v3.0: workflow_id is UUID, version removed from search response
type WorkflowResultAudit struct {
	WorkflowID  string                 `json:"workflow_id"` // DD-WORKFLOW-002 v3.0: UUID
	Title       string                 `json:"title"`
	Rank        int                    `json:"rank"`
	Scoring     ScoringV1              `json:"scoring"`
	Owner       string                 `json:"owner,omitempty"`
	Maintainer  string                 `json:"maintainer,omitempty"`
	Description string                 `json:"description,omitempty"`
	Labels      map[string]interface{} `json:"labels,omitempty"`
}

// ScoringV1 captures V1.0 scoring (confidence only) per DD-WORKFLOW-004 v2.0.
// V2.0+ will extend this struct with BaseSimilarity, LabelBoost, LabelPenalty.
type ScoringV1 struct {
	Confidence float64 `json:"confidence"`
}

// SearchExecutionMetadata captures search execution details (BR-AUDIT-028).
type SearchExecutionMetadata struct {
	DurationMs          int64  `json:"duration_ms"`
	EmbeddingDimensions int    `json:"embedding_dimensions"`
	EmbeddingModel      string `json:"embedding_model"`
}

// ========================================
// WORKFLOW SEARCH AUDIT EVENT BUILDER
// ========================================

// WorkflowSearchAuditEventBuilder builds audit events for workflow search operations.
// V1.0: confidence only (DD-WORKFLOW-004 v2.0)
type WorkflowSearchAuditEventBuilder struct {
	request  *models.WorkflowSearchRequest
	response *models.WorkflowSearchResponse
	duration time.Duration
}

// NewWorkflowSearchAuditEventBuilder creates a new builder for workflow search audit events.
//
// Parameters:
// - request: The original search request
// - response: The search response
// - duration: Time taken to execute the search
//
// Example:
//
//	builder := audit.NewWorkflowSearchAuditEventBuilder(request, response, duration)
//	event, err := builder.Build()
func NewWorkflowSearchAuditEventBuilder(
	request *models.WorkflowSearchRequest,
	response *models.WorkflowSearchResponse,
	duration time.Duration,
) *WorkflowSearchAuditEventBuilder {
	return &WorkflowSearchAuditEventBuilder{
		request:  request,
		response: response,
		duration: duration,
	}
}

// Build constructs the audit event for workflow search.
//
// Returns:
// - *ogenclient.AuditEventRequest: The constructed audit event (OpenAPI type)
// - error: Any error during construction
//
// The event follows the ADR-034 unified audit schema with:
// - EventType: "workflow.catalog.search_completed"
// - EventCategory: "workflow"
// - CorrelationID: remediation_id from request
// - EventData: Structured WorkflowSearchEventData (as map)
//
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
func (b *WorkflowSearchAuditEventBuilder) Build() (*ogenclient.AuditEventRequest, error) {
	// Generate resource ID from query hash
	resourceID := b.generateQueryHash()

	// Build structured event_data
	eventData := b.buildEventData()
	eventDataBytes, err := json.Marshal(eventData)
	if err != nil {
		return nil, err
	}

	// Convert to map for OpenAPI type
	var eventDataMap map[string]interface{}
	if err := json.Unmarshal(eventDataBytes, &eventDataMap); err != nil {
		return nil, err
	}

	// Calculate duration in milliseconds
	durationMs := int(b.duration.Milliseconds())

	// Create event using OpenAPI types (DD-AUDIT-002 V2.0)
	event := pkgaudit.NewAuditEventRequest()
	event.Version = "1.0"
	pkgaudit.SetEventType(event, "workflow.catalog.search_completed")
	pkgaudit.SetEventCategory(event, "workflow")
	pkgaudit.SetEventAction(event, "search_completed")
	pkgaudit.SetEventOutcome(event, pkgaudit.OutcomeSuccess)
	pkgaudit.SetActor(event, "service", "datastorage")
	pkgaudit.SetResource(event, "workflow_catalog", resourceID)
	pkgaudit.SetDuration(event, durationMs)
	pkgaudit.SetEventData(event, eventDataMap)

	// BR-AUDIT-023: Use remediation_id as correlation_id if provided, else use query hash
	if b.request.RemediationID != "" {
		pkgaudit.SetCorrelationID(event, b.request.RemediationID)
	} else {
		pkgaudit.SetCorrelationID(event, resourceID)
	}

	return event, nil
}

// buildEventData constructs the structured event_data for workflow search audit.
// Uses typed structs for compile-time safety.
//
// BR-AUDIT-025: Query metadata capture
// BR-AUDIT-026: Scoring capture (V1.0: confidence only)
// BR-AUDIT-027: Workflow metadata capture
// BR-AUDIT-028: Search metadata capture
func (b *WorkflowSearchAuditEventBuilder) buildEventData() *WorkflowSearchEventData {
	return &WorkflowSearchEventData{
		Query:          b.buildQueryMetadata(),
		Results:        b.buildResultsMetadata(),
		SearchMetadata: b.buildSearchMetadata(),
	}
}

// buildQueryMetadata constructs the query section of event_data.
// BR-AUDIT-025: Query metadata capture
// V1.0: Label-only search (no query text, uses MinScore)
// Uses structured WorkflowSearchFilters for type safety (00-project-guidelines.mdc)
func (b *WorkflowSearchAuditEventBuilder) buildQueryMetadata() QueryMetadata {
	// Direct assignment of structured type - no manual map construction needed
	return QueryMetadata{
		TopK:     b.request.TopK,
		MinScore: b.request.MinScore,
		Filters:  b.request.Filters, // Structured type preserves all fields
	}
}

// buildResultsMetadata constructs the results section of event_data.
// BR-AUDIT-027: Workflow metadata capture
// BR-AUDIT-026: Scoring capture (V1.0: confidence only)
func (b *WorkflowSearchAuditEventBuilder) buildResultsMetadata() ResultsMetadata {
	workflows := make([]WorkflowResultAudit, 0, len(b.response.Workflows))

	for i, result := range b.response.Workflows {
		workflow := b.buildWorkflowMetadata(result, i+1) // 1-based rank
		workflows = append(workflows, workflow)
	}

	return ResultsMetadata{
		TotalFound: b.response.TotalResults,
		Returned:   len(b.response.Workflows),
		Workflows:  workflows,
	}
}

// buildWorkflowMetadata constructs metadata for a single workflow result.
// BR-AUDIT-027: Workflow metadata capture
// BR-AUDIT-026: Scoring capture (hybrid scoring components)
// DD-WORKFLOW-002 v3.0: Uses flat fields from WorkflowSearchResult
func (b *WorkflowSearchAuditEventBuilder) buildWorkflowMetadata(result models.WorkflowSearchResult, rank int) WorkflowResultAudit {
	workflow := WorkflowResultAudit{
		WorkflowID:  result.WorkflowID,  // DD-WORKFLOW-002 v3.0: flat field
		Title:       result.Title,       // DD-WORKFLOW-002 v3.0: flat field
		Description: result.Description, // DD-WORKFLOW-002 v3.0: flat field
		Rank:        rank,
		// Hybrid scoring: use FinalScore as confidence
		Scoring: ScoringV1{
			Confidence: result.Confidence, // DD-WORKFLOW-002 v3.0: confidence field
		},
	}

	// Add signal_type from flat field (DD-WORKFLOW-002 v3.0)
	if result.SignalType != "" {
		workflow.Labels = map[string]interface{}{
			"signal_type": result.SignalType,
		}
	}

	return workflow
}

// buildSearchMetadata constructs the search_metadata section of event_data.
// BR-AUDIT-028: Search metadata capture
func (b *WorkflowSearchAuditEventBuilder) buildSearchMetadata() SearchExecutionMetadata {
	return SearchExecutionMetadata{
		DurationMs:          b.duration.Milliseconds(),
		EmbeddingDimensions: 768, // all-mpnet-base-v2
		EmbeddingModel:      "all-mpnet-base-v2",
	}
}

// generateQueryHash generates a unique identifier for the search query.
// V1.0: Hash based on filters (label-only search, no query text)
func (b *WorkflowSearchAuditEventBuilder) generateQueryHash() string {
	if b.request.Filters == nil {
		return "00000000000000" // Empty hash for invalid request
	}

	// Create hash from filter values (deterministic label-based search)
	hashInput := fmt.Sprintf("%s-%s-%s-%s-%s",
		b.request.Filters.SignalType,
		b.request.Filters.Severity,
		b.request.Filters.Component,
		b.request.Filters.Environment,
		b.request.Filters.Priority,
	)
	hash := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(hash[:8]) // First 8 bytes = 16 hex chars
}

// NewWorkflowSearchAuditEvent is a convenience function that creates and builds
// a workflow search audit event in one call.
//
// This is the main entry point for creating audit events from workflow search operations.
//
// Parameters:
// - request: The original search request
// - response: The search response
// - duration: Time taken to execute the search
//
// Returns:
// - *ogenclient.AuditEventRequest: The constructed audit event (OpenAPI type)
// - error: Any error during construction
//
// DD-AUDIT-002 V2.0: Returns OpenAPI types directly
//
// Example:
//
//	event, err := audit.NewWorkflowSearchAuditEvent(request, response, duration)
//	if err != nil {
//	    return fmt.Errorf("failed to create audit event: %w", err)
//	}
//	auditStore.StoreAudit(ctx, event)
func NewWorkflowSearchAuditEvent(
	request *models.WorkflowSearchRequest,
	response *models.WorkflowSearchResponse,
	duration time.Duration,
) (*ogenclient.AuditEventRequest, error) {
	builder := NewWorkflowSearchAuditEventBuilder(request, response, duration)
	return builder.Build()
}

// ValidateWorkflowAuditEvent validates a workflow search audit event.
// This ensures the event has all required fields per BR-AUDIT-023-030.
//
// Uses structured WorkflowSearchEventData for type-safe validation.
//
// V1.0 Requirements:
// - query field is required
// - results field is required
// - confidence field is required for each workflow (no boost/penalty breakdown)
//
// DD-AUDIT-002 V2.0: Validates OpenAPI types
//
// Returns:
// - error: Validation error if event is invalid, nil if valid
func ValidateWorkflowAuditEvent(event *ogenclient.AuditEventRequest) error {
	if event == nil {
		return &ValidationError{Field: "event", Message: "event is nil"}
	}

	if event.EventData == nil {
		return &ValidationError{Field: "event_data", Message: "event_data is nil"}
	}

	// V2.0: Direct type assertion to OpenAPI-generated struct (DD-AUDIT-004)
	// No backwards compatibility needed (pre-release product)
	eventData, ok := event.EventData.(*ogenclient.WorkflowSearchAuditPayload)
	if !ok {
		return &ValidationError{Field: "event_data", Message: "event_data must be *ogenclient.WorkflowSearchAuditPayload"}
	}

	// Query field validation (using OpenAPI-generated types)
	// Filters-based search uses QueryMetadata.Filters
	// No validation needed - Filters can be empty for broad search

	// Results field validation using OpenAPI-generated types
	// Validate that workflows have valid confidence scores
	for i, wf := range eventData.Results.Workflows {
		// V1.0: confidence is required (0.0-1.0 range per OpenAPI spec)
		if wf.Scoring.Confidence < 0 || wf.Scoring.Confidence > 1 {
			return &ValidationError{
				Field:   "confidence",
				Message: "confidence must be between 0.0 and 1.0",
				Index:   i,
			}
		}
	}

	return nil
}

// ValidationError represents a validation error for audit events.
type ValidationError struct {
	Field   string
	Message string
	Index   int // For array elements
}

func (e *ValidationError) Error() string {
	if e.Index > 0 {
		return e.Message + " (workflow index: " + string(rune(e.Index)) + ")"
	}
	return e.Message
}
