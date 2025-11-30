// Copyright 2025 Jordi Gil.
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	pkgaudit "github.com/jordigilh/kubernaut/pkg/audit"
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
type QueryMetadata struct {
	Text          string                 `json:"text"`
	TopK          int                    `json:"top_k"`
	MinSimilarity *float64               `json:"min_similarity,omitempty"`
	Filters       map[string]interface{} `json:"filters,omitempty"`
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
// - *pkgaudit.AuditEvent: The constructed audit event (from pkg/audit)
// - error: Any error during construction
//
// The event follows the ADR-034 unified audit schema with:
// - EventType: "workflow.catalog.search_completed"
// - EventCategory: "workflow"
// - CorrelationID: remediation_id from request
// - EventData: Structured WorkflowSearchEventData (as JSON bytes)
func (b *WorkflowSearchAuditEventBuilder) Build() (*pkgaudit.AuditEvent, error) {
	// Generate resource ID from query hash
	resourceID := b.generateQueryHash()

	// Build structured event_data and marshal to JSON bytes
	eventData := b.buildEventData()
	eventDataBytes, err := json.Marshal(eventData)
	if err != nil {
		return nil, err
	}

	// Calculate duration in milliseconds
	durationMs := int(b.duration.Milliseconds())

	// Create event using pkg/audit.AuditEvent (for AuditStore compatibility)
	event := pkgaudit.NewAuditEvent()
	event.EventType = "workflow.catalog.search_completed"
	event.EventCategory = "workflow"
	event.EventAction = "search_completed"
	event.EventOutcome = "success"
	event.ActorType = "service"
	event.ActorID = "datastorage"
	event.ResourceType = "workflow_catalog"
	event.ResourceID = resourceID
	event.CorrelationID = resourceID // Use generated resource ID as correlation ID
	event.DurationMs = &durationMs
	event.RetentionDays = 90 // Default retention per BR-AUDIT-029
	event.IsSensitive = false
	event.EventData = eventDataBytes

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
func (b *WorkflowSearchAuditEventBuilder) buildQueryMetadata() QueryMetadata {
	query := QueryMetadata{
		Text: b.request.Query,
		TopK: b.request.TopK,
	}

	// Add min_similarity if specified
	if b.request.MinSimilarity != nil {
		query.MinSimilarity = b.request.MinSimilarity
	}

	// Add filters if present
	if b.request.Filters != nil {
		filters := make(map[string]interface{})

		if b.request.Filters.SignalType != "" {
			filters["signal-type"] = b.request.Filters.SignalType
		}
		if b.request.Filters.Severity != "" {
			filters["severity"] = b.request.Filters.Severity
		}
		if b.request.Filters.Environment != nil {
			filters["environment"] = *b.request.Filters.Environment
		}
		if b.request.Filters.BusinessCategory != nil {
			filters["business-category"] = *b.request.Filters.BusinessCategory
		}
		if b.request.Filters.Priority != nil {
			filters["priority"] = *b.request.Filters.Priority
		}
		if b.request.Filters.RiskTolerance != nil {
			filters["risk-tolerance"] = *b.request.Filters.RiskTolerance
		}
		if b.request.Filters.ResourceManagement != nil {
			filters["resource-management"] = *b.request.Filters.ResourceManagement
		}
		if b.request.Filters.GitOpsTool != nil {
			filters["gitops-tool"] = *b.request.Filters.GitOpsTool
		}

		if len(filters) > 0 {
			query.Filters = filters
		}
	}

	return query
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
// Uses nested Workflow structure from models.WorkflowSearchResult
func (b *WorkflowSearchAuditEventBuilder) buildWorkflowMetadata(result models.WorkflowSearchResult, rank int) WorkflowResultAudit {
	workflow := WorkflowResultAudit{
		WorkflowID:  result.Workflow.WorkflowID,
		Title:       result.Workflow.Name, // Name is used as Title in audit
		Description: result.Workflow.Description,
		Rank:        rank,
		// Hybrid scoring: use FinalScore as confidence
		Scoring: ScoringV1{
			Confidence: result.FinalScore,
		},
	}

	// Add signal_type from labels for audit (Labels is json.RawMessage)
	if result.Workflow.Labels != nil {
		var labels map[string]interface{}
		if err := json.Unmarshal(result.Workflow.Labels, &labels); err == nil {
			if signalType, ok := labels["signal-type"].(string); ok && signalType != "" {
				workflow.Labels = map[string]interface{}{
					"signal_type": signalType,
				}
			}
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
func (b *WorkflowSearchAuditEventBuilder) generateQueryHash() string {
	hash := sha256.Sum256([]byte(b.request.Query))
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
// - *pkgaudit.AuditEvent: The constructed audit event (from pkg/audit)
// - error: Any error during construction
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
) (*pkgaudit.AuditEvent, error) {
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
// Returns:
// - error: Validation error if event is invalid, nil if valid
func ValidateWorkflowAuditEvent(event *pkgaudit.AuditEvent) error {
	if event == nil {
		return &ValidationError{Field: "event", Message: "event is nil"}
	}

	if len(event.EventData) == 0 {
		return &ValidationError{Field: "event_data", Message: "event_data is empty"}
	}

	// Unmarshal to structured type for type-safe validation
	var eventData WorkflowSearchEventData
	if err := json.Unmarshal(event.EventData, &eventData); err != nil {
		return &ValidationError{Field: "event_data", Message: "invalid JSON: " + err.Error()}
	}

	// Check for query field (Text is required)
	if eventData.Query.Text == "" {
		return &ValidationError{Field: "query", Message: "missing required field: query"}
	}

	// Results field is always present due to struct initialization
	// But we need to validate that workflows have confidence
	for i, wf := range eventData.Results.Workflows {
		// V1.0: confidence is required (Scoring.Confidence is always present due to struct)
		// But we validate it's a valid value (>= 0)
		if wf.Scoring.Confidence < 0 {
			return &ValidationError{
				Field:   "confidence",
				Message: "confidence must be >= 0",
				Index:   i,
			}
		}
	}

	return nil
}

// ValidateWorkflowAuditEventUnstructured validates a workflow search audit event
// using unstructured JSON for backwards compatibility with existing tests.
//
// Deprecated: Use ValidateWorkflowAuditEvent with structured types instead.
func ValidateWorkflowAuditEventUnstructured(event *pkgaudit.AuditEvent) error {
	if event == nil {
		return &ValidationError{Field: "event", Message: "event is nil"}
	}

	if len(event.EventData) == 0 {
		return &ValidationError{Field: "event_data", Message: "event_data is empty"}
	}

	// Unmarshal event data to validate structure
	var eventData map[string]interface{}
	if err := json.Unmarshal(event.EventData, &eventData); err != nil {
		return &ValidationError{Field: "event_data", Message: "invalid JSON: " + err.Error()}
	}

	// Check for query field
	if _, ok := eventData["query"]; !ok {
		return &ValidationError{Field: "query", Message: "missing required field: query"}
	}

	// Check for results field
	results, ok := eventData["results"]
	if !ok {
		return &ValidationError{Field: "results", Message: "missing required field: results"}
	}

	// Validate workflow scoring (V1.0: confidence required)
	resultsMap, ok := results.(map[string]interface{})
	if !ok {
		return &ValidationError{Field: "results", Message: "results must be a map"}
	}

	workflows, ok := resultsMap["workflows"].([]interface{})
	if !ok {
		// Empty workflows array is valid
		return nil
	}

	// Check each workflow has confidence
	for i, wf := range workflows {
		wfMap, ok := wf.(map[string]interface{})
		if !ok {
			continue
		}

		scoring, ok := wfMap["scoring"].(map[string]interface{})
		if !ok {
			return &ValidationError{
				Field:   "scoring",
				Message: "missing required field: scoring",
				Index:   i,
			}
		}

		if _, ok := scoring["confidence"]; !ok {
			return &ValidationError{
				Field:   "confidence",
				Message: "missing required field: confidence",
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
