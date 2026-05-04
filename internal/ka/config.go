// Package ka provides clients for communicating with the Kubernaut Agent (KA).
package ka

import (
	"errors"
	"time"
)

// ErrMCPUnavailable indicates the KA MCP endpoint is unreachable.
var ErrMCPUnavailable = errors.New("KA MCP endpoint unavailable")

// Config holds the configuration for KA REST and MCP clients.
type Config struct {
	// BaseURL is the KA REST API base URL.
	BaseURL string
	// MCPEndpoint is the KA MCP endpoint URL.
	MCPEndpoint string
	// Token is the JWT for authentication (forwarded as Bearer).
	Token string
	// Timeout for HTTP requests to KA.
	Timeout time.Duration
	// CBMaxRequests is the circuit breaker max requests in half-open state.
	CBMaxRequests uint32
	// CBInterval is the circuit breaker interval.
	CBInterval time.Duration
	// CBTimeout is the circuit breaker timeout.
	CBTimeout time.Duration
	// CBFailureThreshold is the number of failures before circuit opens.
	CBFailureThreshold uint32
}

// AnalyzeRequest is the request body for POST /api/v1/incident/analyze.
type AnalyzeRequest struct {
	Namespace string `json:"namespace,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Name      string `json:"name,omitempty"`
}

// SessionStatus is the response from GET /api/v1/incident/session/{id}.
type SessionStatus struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

// IncidentResponse is the response from GET /api/v1/incident/session/{id}/result.
type IncidentResponse struct {
	SessionID string `json:"session_id"`
	Summary   string `json:"summary"`
}

// SelectWorkflowArgs is the input for the kubernaut_select_workflow MCP tool call.
type SelectWorkflowArgs struct {
	RRID       string `json:"rr_id"`
	WorkflowID string `json:"workflow_id"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
}

// SelectWorkflowResult is the response from kubernaut_select_workflow MCP call.
type SelectWorkflowResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}
