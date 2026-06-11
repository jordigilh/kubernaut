package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

// WorkflowParameter describes a single input parameter for a workflow.
type WorkflowParameter struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Default     any      `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// DiscoverWorkflowsArgs defines the input for kubernaut_discover_workflows.
type DiscoverWorkflowsArgs struct {
	RRID       string `json:"rr_id"`
	WorkflowID string `json:"workflow_id,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

// WorkflowDetail holds a workflow definition with its parameter schemas.
type WorkflowDetail struct {
	WorkflowID  string              `json:"workflow_id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Kind        string              `json:"kind,omitempty"`
	Confidence  float64             `json:"confidence,omitempty"`
	Parameters  []WorkflowParameter `json:"parameters"`
}

// DiscoverWorkflowsResult is the output of kubernaut_discover_workflows.
type DiscoverWorkflowsResult struct {
	Workflows []WorkflowDetail `json:"workflows"`
	Count     int              `json:"count"`
}

// HandleDiscoverWorkflows implements kubernaut_discover_workflows via KA MCP.
//
//nolint:gocritic // hugeParam: args passed by value for simplicity
func HandleDiscoverWorkflows(ctx context.Context, mcpClient ka.MCPClient, args DiscoverWorkflowsArgs) (DiscoverWorkflowsResult, error) {
	if args.RRID != "" {
		if err := validate.RRID(args.RRID); err != nil {
			return DiscoverWorkflowsResult{}, fmt.Errorf("invalid rr_id: %w", err)
		}
	}
	if mcpClient == nil {
		return DiscoverWorkflowsResult{}, fmt.Errorf("workflow discovery is not available: MCP client not configured")
	}

	kaResult, err := mcpClient.DiscoverWorkflows(ctx, ka.DiscoverWorkflowsArgs{
		RRID:       args.RRID,
		WorkflowID: args.WorkflowID,
		Kind:       args.Kind,
	})
	if err != nil {
		return DiscoverWorkflowsResult{}, fmt.Errorf("discover workflows: %w", err)
	}

	workflows := make([]WorkflowDetail, 0, len(kaResult.Workflows))
	for _, w := range kaResult.Workflows {
		params := make([]WorkflowParameter, 0, len(w.Parameters))
		for _, p := range w.Parameters {
			params = append(params, WorkflowParameter{
				Name:        p.Name,
				Type:        p.Type,
				Description: p.Description,
				Required:    p.Required,
				Default:     p.Default,
				Enum:        p.Enum,
			})
		}
		workflows = append(workflows, WorkflowDetail{
			WorkflowID:  w.WorkflowID,
			Name:        w.Name,
			Description: w.Description,
			Kind:        w.Kind,
			Confidence:  w.Confidence,
			Parameters:  params,
		})
	}

	return DiscoverWorkflowsResult{
		Workflows: workflows,
		Count:     len(workflows),
	}, nil
}

// ValidateWorkflowParameters validates supplied parameters against a discovered schema.
func ValidateWorkflowParameters(schema []WorkflowParameter, params map[string]any) error {
	if err := validateDefaults(schema); err != nil {
		return err
	}

	knownParams := make(map[string]WorkflowParameter, len(schema))
	for _, p := range schema {
		knownParams[p.Name] = p
	}

	for key := range params {
		if _, ok := knownParams[key]; !ok {
			return fmt.Errorf("unknown parameter %q", key)
		}
	}

	for _, p := range schema {
		val, provided := params[p.Name]
		if !provided && p.Required {
			return fmt.Errorf("required parameter %q missing", p.Name)
		}
		if !provided {
			continue
		}
		if err := validateParamType(p, val); err != nil {
			return err
		}
		if len(p.Enum) > 0 {
			strVal := fmt.Sprintf("%v", val)
			if !slices.Contains(p.Enum, strVal) {
				return fmt.Errorf("parameter %q value %q not in enum %v", p.Name, strVal, p.Enum)
			}
		}
	}
	return nil
}

func validateDefaults(schema []WorkflowParameter) error {
	for _, p := range schema {
		if p.Default == nil || p.Required {
			continue
		}
		if err := validateParamType(p, p.Default); err != nil {
			return fmt.Errorf("default value for parameter %q: %w", p.Name, err)
		}
	}
	return nil
}

func validateParamType(p WorkflowParameter, val any) error {
	switch p.Type {
	case "string":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("parameter %q: expected type string, got %T", p.Name, val)
		}
	case "int":
		switch v := val.(type) {
		case int, int32, int64, float64:
			_ = v
		case json.Number:
			if _, err := v.Int64(); err != nil {
				return fmt.Errorf("parameter %q: expected type int, got non-integer number", p.Name)
			}
		default:
			return fmt.Errorf("parameter %q: expected type int, got %T", p.Name, val)
		}
	case "float":
		switch val.(type) {
		case float32, float64, int, int32, int64, json.Number:
		default:
			return fmt.Errorf("parameter %q: expected type float, got %T", p.Name, val)
		}
	case "bool":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("parameter %q: expected type bool, got %T", p.Name, val)
		}
	}
	return nil
}

// NewDiscoverWorkflowsTool creates the kubernaut_discover_workflows tool.
func NewDiscoverWorkflowsTool(mcpClient ka.MCPClient) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_discover_workflows",
		Description: "Discover available workflows with their parameter schemas for LLM-populated execution. Requires an active interactive driver session — call kubernaut_investigate first.",
	}, func(ctx tool.Context, args DiscoverWorkflowsArgs) (DiscoverWorkflowsResult, error) {
		return HandleDiscoverWorkflows(ctx, mcpClient, args)
	})
}

// SelectWorkflowArgs defines the input for kubernaut_select_workflow.
type SelectWorkflowArgs struct {
	RRID       string         `json:"rr_id"`
	WorkflowID string         `json:"workflow_id"`
	Kind       string         `json:"kind,omitempty"`
	Name       string         `json:"name,omitempty"`
	Namespace  string         `json:"namespace,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// SelectWorkflowResult is the output of kubernaut_select_workflow.
type SelectWorkflowResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// HandleSelectWorkflow implements kubernaut_select_workflow via KA MCP.
//
//nolint:gocritic // hugeParam: args passed by value for simplicity; not performance-critical
func HandleSelectWorkflow(ctx context.Context, mcpClient ka.MCPClient, args SelectWorkflowArgs, auditor audit.Emitter) (SelectWorkflowResult, error) {
	if err := validate.RRID(args.RRID); err != nil {
		return SelectWorkflowResult{}, fmt.Errorf("invalid rr_id: %w", err)
	}
	if mcpClient == nil {
		return SelectWorkflowResult{}, fmt.Errorf("workflow selection is not available: MCP client not configured")
	}
	result, err := mcpClient.SelectWorkflow(ctx, ka.SelectWorkflowArgs{
		RRID:       args.RRID,
		WorkflowID: args.WorkflowID,
		Kind:       args.Kind,
		Name:       args.Name,
		Namespace:  args.Namespace,
		Parameters: args.Parameters,
	})
	if err != nil {
		return SelectWorkflowResult{}, fmt.Errorf("selecting workflow: %w", err)
	}

	if auditor != nil {
		auditor.Emit(ctx, &audit.Event{
			Type: audit.EventUserDecision,
			Detail: map[string]string{
				"rr_id":       args.RRID,
				"workflow_id": args.WorkflowID,
				"decision":    "accept",
				"status":      result.Status,
			},
		})
	}

	return SelectWorkflowResult{
		Status:  result.Status,
		Message: result.Message,
	}, nil
}

// NewSelectWorkflowTool creates the kubernaut_select_workflow tool.
func NewSelectWorkflowTool(mcpClient ka.MCPClient, auditor audit.Emitter) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_select_workflow",
		Description: "Select a remediation workflow for execution. Triggers enrichment and workflow selection in the backend. Requires an active interactive driver session — call kubernaut_investigate first.",
	}, func(ctx tool.Context, args SelectWorkflowArgs) (SelectWorkflowResult, error) {
		return HandleSelectWorkflow(ctx, mcpClient, args, auditor)
	})
}

// PresentDecisionArgs defines the input for present_decision.
// RCAData is the structured root cause analysis data that the LLM passes
// through from the kubernaut_investigate response into present_decision.
// This field is required — ADK schema validation enforces self-correction
// if omitted by the LLM (#1396).
type RCAData struct {
	Severity       string   `json:"severity"`
	Confidence     float64  `json:"confidence"`
	CausalChain    []string `json:"causal_chain,omitempty"`
	Target         string   `json:"target"`
	ToolCallsCount int      `json:"tool_calls_count"`
	LLMTurns       int      `json:"llm_turns"`
}

type PresentDecisionArgs struct {
	SessionID string           `json:"session_id"`
	Summary   string           `json:"summary"`
	RCA       RCAData          `json:"rca"`
	Options   []WorkflowOption `json:"options"`
}

// WorkflowOption represents a remediation workflow choice.
type WorkflowOption struct {
	WorkflowID     string            `json:"workflow_id"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Risk           string            `json:"risk,omitempty"`
	Recommended    bool              `json:"recommended,omitempty"`
	Parameters     map[string]string `json:"parameters,omitempty"`
	RuledOutReason string            `json:"ruled_out_reason,omitempty"`
}

// PresentDecisionResult is the output of present_decision.
type PresentDecisionResult struct {
	Presented bool   `json:"presented"`
	Message   string `json:"message"`
}

// HandlePresentDecision formats RCA and options for user presentation.
func HandlePresentDecision(args PresentDecisionArgs) PresentDecisionResult {
	msg := fmt.Sprintf("Investigation complete.\n\nSummary: %s", args.Summary)
	if args.RCA.Severity != "" {
		msg += fmt.Sprintf("\nSeverity: %s (confidence: %.2f)", args.RCA.Severity, args.RCA.Confidence)
	}
	msg += "\n\nAvailable actions:"
	for i, opt := range args.Options {
		msg += fmt.Sprintf("\n  %d. %s", i+1, opt.Name)
		if opt.Description != "" {
			msg += fmt.Sprintf(" - %s", opt.Description)
		}
	}
	return PresentDecisionResult{
		Presented: true,
		Message:   msg,
	}
}

// NewPresentDecisionTool creates the present_decision tool (IsLongRunning).
func NewPresentDecisionTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:          "kubernaut_present_decision",
		Description:   "Present investigation results and remediation options to the user for a decision",
		IsLongRunning: true,
	}, func(ctx tool.Context, args PresentDecisionArgs) (PresentDecisionResult, error) {
		return HandlePresentDecision(args), nil
	})
}
