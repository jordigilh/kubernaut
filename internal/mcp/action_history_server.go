package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/actionhistory"
	"github.com/jordigilh/prometheus-alerts-slm/internal/errors"
	"github.com/jordigilh/prometheus-alerts-slm/internal/oscillation"
	"github.com/jordigilh/prometheus-alerts-slm/internal/validation"
	"github.com/sirupsen/logrus"
)

// OscillationDetector interface for testing
type OscillationDetector interface {
	AnalyzeResource(ctx context.Context, resourceRef actionhistory.ResourceReference, windowMinutes int) (*oscillation.OscillationAnalysisResult, error)
}

// ActionHistoryMCPServer provides MCP tools for action history analysis
type ActionHistoryMCPServer struct {
	repository   actionhistory.Repository
	detector     OscillationDetector
	capabilities MCPCapabilities
	logger       *logrus.Logger
}

// MCPCapabilities defines the capabilities exposed by this MCP server
type MCPCapabilities struct {
	Tools []MCPTool `json:"tools"`
}

// MCPTool represents a tool available through MCP
type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema MCPInputSchema `json:"inputSchema"`
}

// MCPInputSchema defines the schema for tool inputs
type MCPInputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]MCPProperty `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

// MCPProperty defines a property in the input schema
type MCPProperty struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
}

// MCPToolRequest represents an incoming tool request
type MCPToolRequest struct {
	Method string        `json:"method"`
	Params MCPToolParams `json:"params"`
}

// MCPToolParams contains the parameters for a tool call
type MCPToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// MCPToolResponse represents the response to a tool call
type MCPToolResponse struct {
	Content []MCPContent `json:"content"`
}

// MCPContent represents content in the response
type MCPContent struct {
	Type string      `json:"type"`
	Text string      `json:"text,omitempty"`
	Data interface{} `json:"data,omitempty"`
}

// Structured response types for MCP tools

// ActionHistoryResponse represents structured action history data
type ActionHistoryResponse struct {
	ResourceInfo ResourceInfo    `json:"resource_info"`
	TotalActions int             `json:"total_actions"`
	Actions      []ActionSummary `json:"actions"`
}

// ResourceInfo provides resource identification
type ResourceInfo struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
}

// ActionSummary provides structured action data
type ActionSummary struct {
	ID              string                 `json:"id"`
	Timestamp       time.Time              `json:"timestamp"`
	ActionType      string                 `json:"action_type"`
	ModelUsed       string                 `json:"model_used"`
	Confidence      float64                `json:"confidence"`
	ExecutionStatus string                 `json:"execution_status"`
	Effectiveness   *float64               `json:"effectiveness,omitempty"`
	Reasoning       string                 `json:"reasoning,omitempty"`
	AlertName       string                 `json:"alert_name,omitempty"`
	AlertSeverity   string                 `json:"alert_severity,omitempty"`
	Parameters      map[string]interface{} `json:"parameters,omitempty"`
}

// OscillationAnalysisResponse represents structured oscillation analysis
type OscillationAnalysisResponse struct {
	ResourceInfo      ResourceInfo              `json:"resource_info"`
	AnalysisWindow    int                       `json:"analysis_window_minutes"`
	OverallSeverity   string                    `json:"overall_severity"`
	Confidence        float64                   `json:"confidence"`
	RecommendedAction string                    `json:"recommended_action"`
	ScaleOscillation  *ScaleOscillationPattern  `json:"scale_oscillation,omitempty"`
	ResourceThrashing *ResourceThrashingPattern `json:"resource_thrashing,omitempty"`
	IneffectiveLoops  []IneffectiveLoopPattern  `json:"ineffective_loops,omitempty"`
	CascadingFailures []CascadingFailurePattern `json:"cascading_failures,omitempty"`
	SafetyReasoning   string                    `json:"safety_reasoning"`
}

// Pattern types for structured oscillation data
type ScaleOscillationPattern struct {
	DirectionChanges int     `json:"direction_changes"`
	Severity         string  `json:"severity"`
	AvgEffectiveness float64 `json:"avg_effectiveness"`
	DurationMinutes  float64 `json:"duration_minutes"`
}

type ResourceThrashingPattern struct {
	ThrashingTransitions int     `json:"thrashing_transitions"`
	Severity             string  `json:"severity"`
	AvgEffectiveness     float64 `json:"avg_effectiveness"`
	AvgTimeGapMinutes    float64 `json:"avg_time_gap_minutes"`
}

type IneffectiveLoopPattern struct {
	ActionType       string  `json:"action_type"`
	RepetitionCount  int     `json:"repetition_count"`
	Severity         string  `json:"severity"`
	AvgEffectiveness float64 `json:"avg_effectiveness"`
	SpanMinutes      float64 `json:"span_minutes"`
}

type CascadingFailurePattern struct {
	ActionType     string  `json:"action_type"`
	Severity       string  `json:"severity"`
	AvgNewAlerts   float64 `json:"avg_new_alerts"`
	RecurrenceRate float64 `json:"recurrence_rate"`
}

// EffectivenessMetricsResponse represents structured effectiveness data
type EffectivenessMetricsResponse struct {
	ResourceInfo        ResourceInfo                   `json:"resource_info"`
	AnalysisPeriod      TimeRange                      `json:"analysis_period"`
	TotalActions        int                            `json:"total_actions"`
	ActionEffectiveness map[string]ActionEffectiveness `json:"action_effectiveness"`
}

// ActionEffectiveness provides metrics for a specific action type
type ActionEffectiveness struct {
	ActionType       string  `json:"action_type"`
	SampleSize       int     `json:"sample_size"`
	AvgEffectiveness float64 `json:"avg_effectiveness"`
	MinEffectiveness float64 `json:"min_effectiveness"`
	MaxEffectiveness float64 `json:"max_effectiveness"`
}

// TimeRange represents a time period
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// SafetyCheckResponse represents structured safety check data
type SafetyCheckResponse struct {
	ResourceInfo      ResourceInfo `json:"resource_info"`
	ActionType        string       `json:"action_type"`
	IsSafe            bool         `json:"is_safe"`
	OverallSeverity   string       `json:"overall_severity"`
	Confidence        float64      `json:"confidence"`
	RecommendedAction string       `json:"recommended_action"`
	SafetyReasoning   string       `json:"safety_reasoning"`
}

// NewActionHistoryMCPServer creates a new MCP server for action history
func NewActionHistoryMCPServer(repository actionhistory.Repository, detector OscillationDetector, logger *logrus.Logger) *ActionHistoryMCPServer {
	server := &ActionHistoryMCPServer{
		repository: repository,
		detector:   detector,
		logger:     logger,
	}

	server.capabilities = MCPCapabilities{
		Tools: []MCPTool{
			{
				Name:        "get_action_history",
				Description: "Retrieve action history for a specific Kubernetes resource",
				InputSchema: MCPInputSchema{
					Type: "object",
					Properties: map[string]MCPProperty{
						"namespace": {
							Type:        "string",
							Description: "Kubernetes namespace of the resource",
						},
						"kind": {
							Type:        "string",
							Description: "Kubernetes resource kind (e.g., Deployment, Pod)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the Kubernetes resource",
						},
						"limit": {
							Type:        "string",
							Description: "Maximum number of actions to return (default: 50)",
						},
						"timeRange": {
							Type:        "string",
							Description: "Time range for actions (e.g., '24h', '7d', '30d')",
						},
					},
					Required: []string{"namespace", "kind", "name"},
				},
			},
			{
				Name:        "analyze_oscillation_patterns",
				Description: "Analyze a resource for oscillation patterns and safety concerns",
				InputSchema: MCPInputSchema{
					Type: "object",
					Properties: map[string]MCPProperty{
						"namespace": {
							Type:        "string",
							Description: "Kubernetes namespace of the resource",
						},
						"kind": {
							Type:        "string",
							Description: "Kubernetes resource kind (e.g., Deployment, Pod)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the Kubernetes resource",
						},
						"windowMinutes": {
							Type:        "string",
							Description: "Analysis window in minutes (default: 120)",
						},
					},
					Required: []string{"namespace", "kind", "name"},
				},
			},
			{
				Name:        "get_action_effectiveness",
				Description: "Get effectiveness metrics for actions on a resource",
				InputSchema: MCPInputSchema{
					Type: "object",
					Properties: map[string]MCPProperty{
						"namespace": {
							Type:        "string",
							Description: "Kubernetes namespace of the resource",
						},
						"kind": {
							Type:        "string",
							Description: "Kubernetes resource kind",
						},
						"name": {
							Type:        "string",
							Description: "Name of the Kubernetes resource",
						},
						"actionType": {
							Type:        "string",
							Description: "Specific action type to analyze (optional)",
						},
						"timeRange": {
							Type:        "string",
							Description: "Time range for analysis (e.g., '24h', '7d', '30d')",
						},
					},
					Required: []string{"namespace", "kind", "name"},
				},
			},
			{
				Name:        "check_action_safety",
				Description: "Check if a proposed action is safe based on historical patterns",
				InputSchema: MCPInputSchema{
					Type: "object",
					Properties: map[string]MCPProperty{
						"namespace": {
							Type:        "string",
							Description: "Kubernetes namespace of the resource",
						},
						"kind": {
							Type:        "string",
							Description: "Kubernetes resource kind",
						},
						"name": {
							Type:        "string",
							Description: "Name of the Kubernetes resource",
						},
						"actionType": {
							Type:        "string",
							Description: "Type of action being proposed",
						},
						"parameters": {
							Type:        "string",
							Description: "JSON string of action parameters",
						},
					},
					Required: []string{"namespace", "kind", "name", "actionType"},
				},
			},
		},
	}

	return server
}

// GetCapabilities returns the MCP capabilities
func (s *ActionHistoryMCPServer) GetCapabilities() MCPCapabilities {
	return s.capabilities
}

// HandleToolCall processes an incoming tool call request
func (s *ActionHistoryMCPServer) HandleToolCall(ctx context.Context, request MCPToolRequest) (MCPToolResponse, error) {
	switch request.Params.Name {
	case "get_action_history":
		return s.handleGetActionHistory(ctx, request.Params.Arguments)
	case "analyze_oscillation_patterns":
		return s.handleAnalyzeOscillationPatterns(ctx, request.Params.Arguments)
	case "get_action_effectiveness":
		return s.handleGetActionEffectiveness(ctx, request.Params.Arguments)
	case "check_action_safety":
		return s.handleCheckActionSafety(ctx, request.Params.Arguments)
	default:
		return MCPToolResponse{}, fmt.Errorf("unknown tool: %s", request.Params.Name)
	}
}

func (s *ActionHistoryMCPServer) handleGetActionHistory(ctx context.Context, args map[string]interface{}) (MCPToolResponse, error) {
	// Extract and validate required parameters
	namespace, ok := args["namespace"].(string)
	if !ok {
		return MCPToolResponse{}, errors.NewValidationError("namespace is required")
	}

	kind, ok := args["kind"].(string)
	if !ok {
		return MCPToolResponse{}, errors.NewValidationError("kind is required")
	}

	name, ok := args["name"].(string)
	if !ok {
		return MCPToolResponse{}, errors.NewValidationError("name is required")
	}

	// Validate resource reference
	resourceRef := actionhistory.ResourceReference{
		Namespace: namespace,
		Kind:      kind,
		Name:      name,
	}

	if err := validation.ValidateResourceReference(resourceRef); err != nil {
		s.logger.WithFields(errors.LogFields(err)).Warn("Invalid resource reference in MCP request")
		return MCPToolResponse{}, errors.Wrap(err, errors.ErrorTypeValidation, "invalid resource reference")
	}

	// Parse optional parameters
	limit := 50
	if limitStr, ok := args["limit"].(string); ok {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			limit = parsedLimit
		}
	}

	// Parse time range
	var timeRange actionhistory.TimeRange
	if timeRangeStr, ok := args["timeRange"].(string); ok {
		duration, err := parseDuration(timeRangeStr)
		if err == nil {
			timeRange.Start = time.Now().Add(-duration)
			timeRange.End = time.Now()
		}
	}

	query := actionhistory.ActionQuery{
		Namespace:    namespace,
		ResourceKind: kind,
		ResourceName: name,
		TimeRange:    timeRange,
		Limit:        limit,
	}

	traces, err := s.repository.GetActionTraces(ctx, query)
	if err != nil {
		return MCPToolResponse{}, fmt.Errorf("failed to get action traces: %w", err)
	}

	// Convert traces to structured response format
	actions := make([]ActionSummary, len(traces))
	for i, trace := range traces {
		actionSummary := ActionSummary{
			ID:              trace.ActionID,
			Timestamp:       trace.ActionTimestamp,
			ActionType:      trace.ActionType,
			ModelUsed:       trace.ModelUsed,
			Confidence:      trace.ModelConfidence,
			ExecutionStatus: trace.ExecutionStatus,
			Effectiveness:   trace.EffectivenessScore,
			AlertName:       trace.AlertName,
			AlertSeverity:   trace.AlertSeverity,
		}

		if trace.ModelReasoning != nil {
			actionSummary.Reasoning = *trace.ModelReasoning
		}

		// Convert JSONB parameters to map
		if trace.ActionParameters != nil {
			actionSummary.Parameters = trace.ActionParameters
		}

		actions[i] = actionSummary
	}

	// Format as human-readable text for MCP
	var text strings.Builder
	text.WriteString(fmt.Sprintf("Action History for %s/%s\n", kind, name))
	text.WriteString(fmt.Sprintf("Namespace: %s\n", namespace))
	text.WriteString(fmt.Sprintf("Total actions found: %d\n\n", len(traces)))

	if len(traces) == 0 {
		text.WriteString("No action history found for this resource.\n")
	} else {
		for i, action := range actions {
			text.WriteString(fmt.Sprintf("%d. Action: %s (ID: %s)\n", i+1, action.ActionType, action.ID))
			text.WriteString(fmt.Sprintf("   Timestamp: %s\n", action.Timestamp.Format("2006-01-02 15:04:05")))
			text.WriteString(fmt.Sprintf("   Alert: %s (%s)\n", action.AlertName, action.AlertSeverity))
			text.WriteString(fmt.Sprintf("   Model: %s (Confidence: %.2f)\n", action.ModelUsed, action.Confidence))
			text.WriteString(fmt.Sprintf("   Status: %s\n", action.ExecutionStatus))
			if action.Effectiveness != nil && *action.Effectiveness > 0 {
				text.WriteString(fmt.Sprintf("   Effectiveness: %.2f\n", *action.Effectiveness))
			}
			if action.Reasoning != "" {
				text.WriteString(fmt.Sprintf("   Reasoning: %s\n", action.Reasoning))
			}
			text.WriteString("\n")
		}
	}

	// Create structured response
	structuredResponse := ActionHistoryResponse{
		ResourceInfo: ResourceInfo{
			Namespace: namespace,
			Kind:      kind,
			Name:      name,
		},
		TotalActions: len(traces),
		Actions:      actions,
	}

	return MCPToolResponse{
		Content: []MCPContent{
			{
				Type: "application/json",
				Data: structuredResponse,
			},
			{
				Type: "text",
				Text: text.String(),
			},
		},
	}, nil
}

func (s *ActionHistoryMCPServer) handleAnalyzeOscillationPatterns(ctx context.Context, args map[string]interface{}) (MCPToolResponse, error) {
	namespace, ok := args["namespace"].(string)
	if !ok {
		return MCPToolResponse{}, errors.NewValidationError("namespace is required")
	}

	kind, ok := args["kind"].(string)
	if !ok {
		return MCPToolResponse{}, errors.NewValidationError("kind is required")
	}

	name, ok := args["name"].(string)
	if !ok {
		return MCPToolResponse{}, errors.NewValidationError("name is required")
	}

	windowMinutes := 120
	if windowStr, ok := args["windowMinutes"].(string); ok {
		if parsedWindow, err := strconv.Atoi(windowStr); err == nil {
			windowMinutes = parsedWindow
		}
	}

	resourceRef := actionhistory.ResourceReference{
		Namespace: namespace,
		Kind:      kind,
		Name:      name,
	}

	analysis, err := s.detector.AnalyzeResource(ctx, resourceRef, windowMinutes)
	if err != nil {
		return MCPToolResponse{}, fmt.Errorf("failed to analyze oscillation patterns: %w", err)
	}

	// Convert to structured response format
	response := OscillationAnalysisResponse{
		ResourceInfo: ResourceInfo{
			Namespace: namespace,
			Kind:      kind,
			Name:      name,
		},
		AnalysisWindow:    windowMinutes,
		OverallSeverity:   string(analysis.OverallSeverity),
		Confidence:        analysis.Confidence,
		RecommendedAction: string(analysis.RecommendedAction),
	}

	// Convert pattern data
	if analysis.ScaleOscillation != nil {
		response.ScaleOscillation = &ScaleOscillationPattern{
			DirectionChanges: analysis.ScaleOscillation.DirectionChanges,
			Severity:         string(analysis.ScaleOscillation.Severity),
			AvgEffectiveness: analysis.ScaleOscillation.AvgEffectiveness,
			DurationMinutes:  analysis.ScaleOscillation.DurationMinutes,
		}
	}

	if analysis.ResourceThrashing != nil {
		response.ResourceThrashing = &ResourceThrashingPattern{
			ThrashingTransitions: analysis.ResourceThrashing.ThrashingTransitions,
			Severity:             string(analysis.ResourceThrashing.Severity),
			AvgEffectiveness:     analysis.ResourceThrashing.AvgEffectiveness,
			AvgTimeGapMinutes:    analysis.ResourceThrashing.AvgTimeGapMinutes,
		}
	}

	// Convert ineffective loops
	for _, loop := range analysis.IneffectiveLoops {
		response.IneffectiveLoops = append(response.IneffectiveLoops, IneffectiveLoopPattern{
			ActionType:       loop.ActionType,
			RepetitionCount:  loop.RepetitionCount,
			Severity:         string(loop.Severity),
			AvgEffectiveness: loop.AvgEffectiveness,
			SpanMinutes:      loop.SpanMinutes,
		})
	}

	// Convert cascading failures
	for _, cascade := range analysis.CascadingFailures {
		response.CascadingFailures = append(response.CascadingFailures, CascadingFailurePattern{
			ActionType:     cascade.ActionType,
			Severity:       string(cascade.Severity),
			AvgNewAlerts:   cascade.AvgNewAlerts,
			RecurrenceRate: cascade.RecurrenceRate,
		})
	}

	// Format as human-readable text for MCP
	var text strings.Builder
	text.WriteString(fmt.Sprintf("Oscillation Analysis for %s/%s\n", kind, name))
	text.WriteString(fmt.Sprintf("Namespace: %s\n", namespace))
	text.WriteString(fmt.Sprintf("Analysis Window: %d minutes\n", windowMinutes))
	text.WriteString(fmt.Sprintf("Overall Severity: %s\n", analysis.OverallSeverity))
	text.WriteString(fmt.Sprintf("Confidence: %.2f\n", analysis.Confidence))
	text.WriteString(fmt.Sprintf("Recommended Action: %s\n\n", analysis.RecommendedAction))

	// Add detailed analysis
	if analysis.OverallSeverity != actionhistory.SeverityNone {
		text.WriteString("Detected Issues:\n")

		if analysis.ScaleOscillation != nil {
			text.WriteString("• Scale Oscillation Detected\n")
			text.WriteString(fmt.Sprintf("  Direction Changes: %d\n", analysis.ScaleOscillation.DirectionChanges))
			text.WriteString(fmt.Sprintf("  Severity: %s\n", analysis.ScaleOscillation.Severity))
		}

		if analysis.ResourceThrashing != nil {
			text.WriteString(fmt.Sprintf("• Resource Thrashing: %d transitions (Severity: %s)\n",
				analysis.ResourceThrashing.ThrashingTransitions, analysis.ResourceThrashing.Severity))
		}

		for _, loop := range analysis.IneffectiveLoops {
			text.WriteString(fmt.Sprintf("• Ineffective Loop: %s repeated %d times (Severity: %s)\n",
				loop.ActionType, loop.RepetitionCount, loop.Severity))
		}

		for _, cascade := range analysis.CascadingFailures {
			text.WriteString(fmt.Sprintf("• Cascading Failure: %s causing avg %.1f new alerts (Severity: %s)\n",
				cascade.ActionType, cascade.AvgNewAlerts, cascade.Severity))
		}

		text.WriteString("\nSafety Reasoning:\n")
		text.WriteString(oscillation.GenerateSafetyReasoning(analysis))
	} else {
		text.WriteString("No concerning oscillation patterns detected. Actions appear safe to proceed.")
	}

	return MCPToolResponse{
		Content: []MCPContent{
			{
				Type: "application/json",
				Data: response,
			},
			{
				Type: "text",
				Text: text.String(),
			},
		},
	}, nil
}

func (s *ActionHistoryMCPServer) handleGetActionEffectiveness(ctx context.Context, args map[string]interface{}) (MCPToolResponse, error) {
	namespace, ok := args["namespace"].(string)
	if !ok {
		return MCPToolResponse{}, errors.NewValidationError("namespace is required")
	}

	kind, ok := args["kind"].(string)
	if !ok {
		return MCPToolResponse{}, errors.NewValidationError("kind is required")
	}

	name, ok := args["name"].(string)
	if !ok {
		return MCPToolResponse{}, errors.NewValidationError("name is required")
	}

	actionType, _ := args["actionType"].(string)

	// Parse time range
	var timeRange actionhistory.TimeRange
	if timeRangeStr, ok := args["timeRange"].(string); ok {
		duration, err := parseDuration(timeRangeStr)
		if err == nil {
			timeRange.Start = time.Now().Add(-duration)
			timeRange.End = time.Now()
		}
	} else {
		// Default to last 7 days
		timeRange.Start = time.Now().Add(-7 * 24 * time.Hour)
		timeRange.End = time.Now()
	}

	query := actionhistory.ActionQuery{
		Namespace:    namespace,
		ResourceKind: kind,
		ResourceName: name,
		ActionType:   actionType,
		TimeRange:    timeRange,
		Limit:        1000, // Get all actions in the time range
	}

	traces, err := s.repository.GetActionTraces(ctx, query)
	if err != nil {
		return MCPToolResponse{}, fmt.Errorf("failed to get action traces: %w", err)
	}

	// Calculate effectiveness metrics by action type
	effectivenessMap := make(map[string][]float64)
	for _, trace := range traces {
		if trace.EffectivenessScore != nil {
			effectivenessMap[trace.ActionType] = append(effectivenessMap[trace.ActionType], *trace.EffectivenessScore)
		}
	}

	// Convert to structured response format
	actionEffectiveness := make(map[string]ActionEffectiveness)
	for actionType, scores := range effectivenessMap {
		if len(scores) > 0 {
			actionEffectiveness[actionType] = ActionEffectiveness{
				ActionType:       actionType,
				SampleSize:       len(scores),
				AvgEffectiveness: calculateAverage(scores),
				MinEffectiveness: minFloat64(scores),
				MaxEffectiveness: maxFloat64(scores),
			}
		}
	}

	response := EffectivenessMetricsResponse{
		ResourceInfo: ResourceInfo{
			Namespace: namespace,
			Kind:      kind,
			Name:      name,
		},
		AnalysisPeriod: TimeRange{
			Start: timeRange.Start,
			End:   timeRange.End,
		},
		TotalActions:        len(traces),
		ActionEffectiveness: actionEffectiveness,
	}

	return MCPToolResponse{
		Content: []MCPContent{
			{
				Type: "application/json",
				Data: response,
			},
		},
	}, nil
}

func (s *ActionHistoryMCPServer) handleCheckActionSafety(ctx context.Context, args map[string]interface{}) (MCPToolResponse, error) {
	namespace, ok := args["namespace"].(string)
	if !ok {
		return MCPToolResponse{}, errors.NewValidationError("namespace is required")
	}

	kind, ok := args["kind"].(string)
	if !ok {
		return MCPToolResponse{}, errors.NewValidationError("kind is required")
	}

	name, ok := args["name"].(string)
	if !ok {
		return MCPToolResponse{}, errors.NewValidationError("name is required")
	}

	actionType, ok := args["actionType"].(string)
	if !ok {
		return MCPToolResponse{}, errors.NewValidationError("actionType is required")
	}

	resourceRef := actionhistory.ResourceReference{
		Namespace: namespace,
		Kind:      kind,
		Name:      name,
	}

	// Analyze oscillation patterns to determine safety
	analysis, err := s.detector.AnalyzeResource(ctx, resourceRef, 120) // 2-hour window
	if err != nil {
		return MCPToolResponse{}, fmt.Errorf("failed to analyze safety: %w", err)
	}

	content := fmt.Sprintf("Action Safety Check for %s on %s/%s in namespace %s:\n\n", actionType, kind, name, namespace)

	if analysis.OverallSeverity == actionhistory.SeverityNone {
		content += "✅ SAFE: No concerning oscillation patterns detected.\n"
		content += "The proposed action appears safe to execute.\n\n"
		content += fmt.Sprintf("Confidence: %.3f\n", analysis.Confidence)
	} else {
		content += fmt.Sprintf("⚠️  WARNING: %s severity oscillation patterns detected.\n", strings.ToUpper(string(analysis.OverallSeverity)))
		content += fmt.Sprintf("Recommended action: %s\n\n", analysis.RecommendedAction)

		// Add specific safety concerns
		content += "Safety Concerns:\n"
		content += oscillation.GenerateSafetyReasoning(analysis)
		content += "\n\n"

		content += fmt.Sprintf("Confidence: %.3f\n", analysis.Confidence)

		switch analysis.RecommendedAction {
		case actionhistory.PreventionBlock:
			content += "\n🚫 RECOMMENDATION: Block this action temporarily to prevent further oscillation."
		case actionhistory.PreventionEscalate:
			content += "\n🔺 RECOMMENDATION: Escalate to human operator for manual intervention."
		case actionhistory.PreventionCoolingPeriod:
			content += "\n⏸️ RECOMMENDATION: Apply cooling period before allowing similar actions."
		case actionhistory.PreventionAlternative:
			content += "\n🔄 RECOMMENDATION: Consider alternative actions to achieve the desired outcome."
		}
	}

	// Create structured response
	safetyResponse := SafetyCheckResponse{
		ResourceInfo: ResourceInfo{
			Namespace: namespace,
			Kind:      kind,
			Name:      name,
		},
		ActionType:        actionType,
		IsSafe:            analysis.OverallSeverity == actionhistory.SeverityNone,
		OverallSeverity:   string(analysis.OverallSeverity),
		Confidence:        analysis.Confidence,
		RecommendedAction: string(analysis.RecommendedAction),
		SafetyReasoning:   oscillation.GenerateSafetyReasoning(analysis),
	}

	return MCPToolResponse{
		Content: []MCPContent{
			{
				Type: "application/json",
				Data: safetyResponse,
			},
			{
				Type: "text",
				Text: content,
			},
		},
	}, nil
}

// Helper functions

func parseDuration(durationStr string) (time.Duration, error) {
	// Handle common duration formats
	switch {
	case strings.HasSuffix(durationStr, "h"):
		hours, err := strconv.Atoi(strings.TrimSuffix(durationStr, "h"))
		if err != nil {
			return 0, err
		}
		return time.Duration(hours) * time.Hour, nil
	case strings.HasSuffix(durationStr, "d"):
		days, err := strconv.Atoi(strings.TrimSuffix(durationStr, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	case strings.HasSuffix(durationStr, "m"):
		minutes, err := strconv.Atoi(strings.TrimSuffix(durationStr, "m"))
		if err != nil {
			return 0, err
		}
		return time.Duration(minutes) * time.Minute, nil
	default:
		return time.ParseDuration(durationStr)
	}
}

func calculateAverage(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	sum := 0.0
	for _, score := range scores {
		sum += score
	}
	return sum / float64(len(scores))
}

func minFloat64(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	min := scores[0]
	for _, score := range scores[1:] {
		if score < min {
			min = score
		}
	}
	return min
}

func maxFloat64(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	max := scores[0]
	for _, score := range scores[1:] {
		if score > max {
			max = score
		}
	}
	return max
}

// StartMCPServer starts the MCP server
func (s *ActionHistoryMCPServer) StartMCPServer(ctx context.Context) error {
	log.Printf("Starting Action History MCP Server with %d tools", len(s.capabilities.Tools))

	// In a real implementation, this would set up the MCP transport layer
	// For now, we'll just log the available capabilities
	capabilitiesJSON, err := json.MarshalIndent(s.capabilities, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal capabilities: %w", err)
	}

	log.Printf("MCP Server Capabilities:\n%s", string(capabilitiesJSON))

	// Block until context is cancelled
	<-ctx.Done()
	return ctx.Err()
}
