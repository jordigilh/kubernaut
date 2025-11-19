// Copyright 2025 Jordi Gil.
// SPDX-License-Identifier: Apache-2.0

package audit

// AIAnalysisEventData represents AI Analysis service event_data structure.
//
// AI Analysis Service creates audit events for:
// - LLM analysis lifecycle (started, completed, failed)
// - Token usage tracking (prompt, completion, total)
// - Root cause analysis (RCA) results
// - Workflow selection
// - MCP tool invocations
//
// Business Requirement: BR-STORAGE-033-007
type AIAnalysisEventData struct {
	AnalysisID       string   `json:"analysis_id"`                  // Unique analysis identifier
	LLMProvider      string   `json:"llm_provider,omitempty"`       // "anthropic", "openai", etc.
	LLMModel         string   `json:"llm_model,omitempty"`          // "claude-haiku-4-5", etc.
	PromptTokens     int      `json:"prompt_tokens"`                // Input tokens
	CompletionTokens int      `json:"completion_tokens"`            // Output tokens
	TotalTokens      int      `json:"total_tokens"`                 // Total tokens
	DurationMs       int64    `json:"duration_ms,omitempty"`        // Analysis duration
	RCASignalType    string   `json:"rca_signal_type,omitempty"`    // Root cause signal type
	RCASeverity      string   `json:"rca_severity,omitempty"`       // Root cause severity
	Confidence       float64  `json:"confidence,omitempty"`         // Analysis confidence score
	WorkflowID       string   `json:"workflow_id,omitempty"`        // Selected workflow
	ToolsInvoked     []string `json:"tools_invoked,omitempty"`      // MCP tools used
	ErrorCode        string   `json:"error_code,omitempty"`         // Error code if failed
}

// AIAnalysisEventBuilder builds AI Analysis-specific event data.
//
// Usage:
//
//	eventData, err := audit.NewAIAnalysisEvent("analysis.completed").
//	    WithAnalysisID("analysis-2025-001").
//	    WithLLM("anthropic", "claude-haiku-4-5").
//	    WithTokenUsage(2500, 750).
//	    WithDuration(4200).
//	    WithRCA("OOMKilled", "critical", 0.92).
//	    WithWorkflow("workflow-increase-memory").
//	    WithToolsInvoked([]string{"kubernetes/describe_pod", "workflow/search_catalog"}).
//	    Build()
//
// Business Requirement: BR-STORAGE-033-008
type AIAnalysisEventBuilder struct {
	*BaseEventBuilder
	aiData AIAnalysisEventData
}

// NewAIAnalysisEvent creates a new AI Analysis event builder.
//
// Parameters:
// - eventType: Specific AI Analysis event type (e.g., "analysis.started", "analysis.completed", "analysis.failed")
//
// Example:
//
//	builder := audit.NewAIAnalysisEvent("analysis.completed")
func NewAIAnalysisEvent(eventType string) *AIAnalysisEventBuilder {
	return &AIAnalysisEventBuilder{
		BaseEventBuilder: NewEventBuilder("aianalysis", eventType),
		aiData:           AIAnalysisEventData{},
	}
}

// WithAnalysisID sets the analysis identifier.
//
// Example:
//
//	builder.WithAnalysisID("analysis-2025-11-18-001")
func (b *AIAnalysisEventBuilder) WithAnalysisID(analysisID string) *AIAnalysisEventBuilder {
	b.aiData.AnalysisID = analysisID
	return b
}

// WithLLM sets the LLM provider and model.
//
// Parameters:
// - provider: LLM provider (e.g., "anthropic", "openai", "google")
// - model: Model identifier (e.g., "claude-haiku-4-5-20251001", "gpt-4-turbo")
//
// Example:
//
//	builder.WithLLM("anthropic", "claude-haiku-4-5")
//
// Business Requirement: BR-STORAGE-033-008
func (b *AIAnalysisEventBuilder) WithLLM(provider, model string) *AIAnalysisEventBuilder {
	b.aiData.LLMProvider = provider
	b.aiData.LLMModel = model
	return b
}

// WithTokenUsage sets token usage metrics.
//
// Parameters:
// - promptTokens: Number of input tokens
// - completionTokens: Number of output tokens
//
// The total tokens will be automatically calculated.
//
// Example:
//
//	builder.WithTokenUsage(2500, 750) // total: 3250 tokens
//
// Business Requirement: BR-STORAGE-033-008
func (b *AIAnalysisEventBuilder) WithTokenUsage(promptTokens, completionTokens int) *AIAnalysisEventBuilder {
	b.aiData.PromptTokens = promptTokens
	b.aiData.CompletionTokens = completionTokens
	b.aiData.TotalTokens = promptTokens + completionTokens
	return b
}

// WithDuration sets analysis duration in milliseconds.
//
// Example:
//
//	builder.WithDuration(4200) // 4.2 seconds
//
// Business Requirement: BR-STORAGE-033-008
func (b *AIAnalysisEventBuilder) WithDuration(durationMs int64) *AIAnalysisEventBuilder {
	b.aiData.DurationMs = durationMs
	return b
}

// WithRCA sets root cause analysis results.
//
// Parameters:
// - signalType: Determined root cause signal type (e.g., "OOMKilled", "DiskPressure")
// - severity: Root cause severity (e.g., "critical", "warning")
// - confidence: Confidence score (0.0-1.0)
//
// Example:
//
//	builder.WithRCA("OOMKilled", "critical", 0.95)
//
// Business Requirement: BR-STORAGE-033-009
func (b *AIAnalysisEventBuilder) WithRCA(signalType, severity string, confidence float64) *AIAnalysisEventBuilder {
	b.aiData.RCASignalType = signalType
	b.aiData.RCASeverity = severity
	b.aiData.Confidence = confidence
	return b
}

// WithWorkflow sets the selected workflow.
//
// Parameters:
// - workflowID: Selected workflow identifier
//
// Example:
//
//	builder.WithWorkflow("workflow-increase-memory-limits")
//
// Business Requirement: BR-STORAGE-033-009
func (b *AIAnalysisEventBuilder) WithWorkflow(workflowID string) *AIAnalysisEventBuilder {
	b.aiData.WorkflowID = workflowID
	return b
}

// WithToolsInvoked sets the MCP tools that were invoked during analysis.
//
// Parameters:
// - tools: List of MCP tool names (e.g., ["kubernetes/describe_pod", "workflow/search_catalog"])
//
// Example:
//
//	builder.WithToolsInvoked([]string{
//	    "kubernetes/describe_pod",
//	    "kubernetes/get_logs",
//	    "workflow/search_catalog",
//	})
//
// Business Requirement: BR-STORAGE-033-009
func (b *AIAnalysisEventBuilder) WithToolsInvoked(tools []string) *AIAnalysisEventBuilder {
	b.aiData.ToolsInvoked = tools
	return b
}

// WithErrorCode sets the error code if analysis failed.
//
// Common error codes:
// - "LLM_TIMEOUT": LLM API timeout
// - "LLM_RATE_LIMIT": LLM API rate limit exceeded
// - "INVALID_RESPONSE": LLM returned invalid JSON
// - "NO_WORKFLOW_FOUND": No suitable workflow found
//
// Example:
//
//	builder.WithErrorCode("LLM_TIMEOUT")
func (b *AIAnalysisEventBuilder) WithErrorCode(errorCode string) *AIAnalysisEventBuilder {
	b.aiData.ErrorCode = errorCode
	return b
}

// Build constructs the final event_data JSONB.
//
// Returns:
// - map[string]interface{}: JSONB-ready event data
// - error: JSON marshaling error (should not occur for valid inputs)
//
// Example:
//
//	eventData, err := builder.Build()
//	if err != nil {
//	    return fmt.Errorf("failed to build AI Analysis event: %w", err)
//	}
func (b *AIAnalysisEventBuilder) Build() (map[string]interface{}, error) {
	// Add AI Analysis-specific data to base event
	b.BaseEventBuilder.WithCustomField("ai_analysis", b.aiData)
	return b.BaseEventBuilder.Build()
}

