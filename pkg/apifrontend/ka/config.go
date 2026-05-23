// Package ka provides clients for communicating with the Kubernaut Agent (KA).
package ka

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	gobreaker "github.com/sony/gobreaker/v2"
)

// ErrMCPUnavailable indicates the KA MCP endpoint is unreachable.
var ErrMCPUnavailable = errors.New("KA MCP endpoint unavailable")

// Config holds the configuration for KA REST and MCP clients.
type Config struct {
	// BaseURL is the KA REST API base URL.
	BaseURL string
	// MCPEndpoint is the KA MCP endpoint URL.
	MCPEndpoint string
	// Timeout for HTTP requests to KA.
	Timeout time.Duration
	// BaseTransport is the underlying transport used for outbound requests.
	// When nil, http.DefaultTransport is used. Inject a TLS-configured
	// transport here for mTLS/CA-verified connections.
	BaseTransport http.RoundTripper
	// CBMaxRequests is the circuit breaker max requests in half-open state.
	CBMaxRequests uint32
	// CBInterval is the circuit breaker interval.
	CBInterval time.Duration
	// CBTimeout is the circuit breaker timeout.
	CBTimeout time.Duration
	// CBFailureThreshold is the number of failures before circuit opens.
	CBFailureThreshold uint32
	// RetryMax is the maximum number of retries (0 = no retries, only the initial attempt).
	RetryMax int
	// RetryInitBackoff is the initial backoff duration for retries.
	RetryInitBackoff time.Duration
	// RetryMaxBackoff is the max backoff duration.
	RetryMaxBackoff time.Duration
	// RetryableStatuses are HTTP status codes that trigger a retry.
	RetryableStatuses []int
	// CBAuditFunc is called on circuit breaker state transitions for SOC2 AU-2 compliance.
	CBAuditFunc func(dependency string, from, to gobreaker.State)
}

// AnalyzeRequest is the request body for POST /api/v1/incident/analyze.
type AnalyzeRequest struct {
	Namespace string `json:"namespace,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Name      string `json:"name,omitempty"`
}

// SessionStatus is the response from GET /api/v1/incident/session/{id}.
// KA v1.5 may return status as either a string or a number; the custom
// UnmarshalJSON handles both representations transparently.
type SessionStatus struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

// UnmarshalJSON accepts SessionStatus payloads where "status" is either a JSON string or number.
func (s *SessionStatus) UnmarshalJSON(data []byte) error {
	var raw struct {
		SessionID string          `json:"session_id"`
		Status    json.RawMessage `json:"status"`
		Error     string          `json:"error,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.SessionID = raw.SessionID
	s.Error = raw.Error

	if len(raw.Status) == 0 {
		return nil
	}
	var str string
	if err := json.Unmarshal(raw.Status, &str); err == nil {
		s.Status = str
		return nil
	}
	var num json.Number
	if err := json.Unmarshal(raw.Status, &num); err == nil {
		s.Status = num.String()
		return nil
	}
	return fmt.Errorf("SessionStatus.status: expected string or number, got %s", string(raw.Status))
}

// IncidentResponse is the response from GET /api/v1/incident/session/{id}/result.
type IncidentResponse struct {
	SessionID string `json:"session_id"`
	Summary   string `json:"summary"`
}

// DiscoverWorkflowsArgs is the input for the kubernaut_discover_workflows MCP tool call.
type DiscoverWorkflowsArgs struct {
	RRID       string `json:"rr_id"`
	WorkflowID string `json:"workflow_id,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

// WorkflowParameterSchema describes a single parameter from a KA discovery response.
type WorkflowParameterSchema struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Default     any      `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// DiscoveredWorkflow represents a workflow returned by KA's discover_workflows.
type DiscoveredWorkflow struct {
	WorkflowID  string                    `json:"workflow_id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Kind        string                    `json:"kind,omitempty"`
	Parameters  []WorkflowParameterSchema `json:"parameters"`
}

// DiscoverWorkflowsResult is the response from kubernaut_discover_workflows MCP call.
type DiscoverWorkflowsResult struct {
	Workflows []DiscoveredWorkflow `json:"workflows"`
}

// SelectWorkflowArgs is the input for the kubernaut_select_workflow MCP tool call.
type SelectWorkflowArgs struct {
	RRID       string         `json:"rr_id"`
	WorkflowID string         `json:"workflow_id"`
	Kind       string         `json:"kind,omitempty"`
	Name       string         `json:"name,omitempty"`
	Namespace  string         `json:"namespace,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// SelectWorkflowResult is the response from kubernaut_select_workflow MCP call.
type SelectWorkflowResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// InvestigateArgs is the input for the kubernaut_investigate MCP tool call.
type InvestigateArgs struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
}

// InvestigateResult is the response from kubernaut_investigate MCP call.
type InvestigateResult struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Summary   string `json:"summary,omitempty"`
}

// InvestigationEvent represents a discrete event from KA's SSE stream.
// Wire-format compatible with internal/kubernautagent/session.InvestigationEvent.
type InvestigationEvent struct {
	Type  string          `json:"type"`
	Turn  int             `json:"turn"`
	Phase string          `json:"phase,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// InvokeActionArgs is the input for the generic kubernaut_investigate action dispatch.
// The acting_user and acting_user_groups are extracted from context by the
// implementation and added to the MCP args map automatically.
type InvokeActionArgs struct {
	RRID    string `json:"rr_id"`
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
}

// InvokeActionResult is the response from a generic kubernaut_investigate action.
type InvokeActionResult struct {
	SessionID string          `json:"session_id,omitempty"`
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// SSE event type constants matching KA's wire format.
const (
	EventTypeReasoningDelta = "reasoning_delta"
	EventTypeTokenDelta     = "token_delta"
	EventTypeToolCallStart  = "tool_call_start"
	EventTypeToolCall       = "tool_call"
	EventTypeToolResult     = "tool_result"
	EventTypeError          = "error"
	EventTypeComplete       = "complete"
	EventTypeCancelled      = "cancelled"
)
