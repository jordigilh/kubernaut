package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// PermissionDecision represents KA's response to a tool permission request.
type PermissionDecision struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"`
}

// PermissionRequest represents a pending tool call awaiting KA approval.
type PermissionRequest struct {
	RequestID string                 `json:"request_id"`
	RunID     string                 `json:"run_id"`
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
	CreatedAt time.Time              `json:"created_at"`
}

// PermissionGate bridges the in-process SDK PermissionRequest hook with the
// HTTP-based ACP resume handler. When the SDK hook fires, it creates a pending
// request and blocks on a channel. When KA calls POST /runs/{id} with the
// decision, the gate writes to the channel and unblocks the hook.
type PermissionGate struct {
	mu       sync.Mutex
	pending  map[string]*pendingRequest
	timeout  time.Duration
	onAwait  func(req PermissionRequest) // callback to emit run.awaiting SSE event
}

type pendingRequest struct {
	req      PermissionRequest
	decision chan PermissionDecision
}

func NewPermissionGate(timeout time.Duration, onAwait func(PermissionRequest)) *PermissionGate {
	return &PermissionGate{
		pending: make(map[string]*pendingRequest),
		timeout: timeout,
		onAwait: onAwait,
	}
}

// RequestPermission is called from within the SDK's PermissionRequest hook.
// It blocks until KA responds via Decide() or the timeout expires.
// Returns ("", nil) for approval or ("denial reason", nil) for denial.
func (g *PermissionGate) RequestPermission(runID, toolName string, toolInput map[string]interface{}) (string, error) {
	reqID := uuid.New().String()
	ch := make(chan PermissionDecision, 1)

	req := PermissionRequest{
		RequestID: reqID,
		RunID:     runID,
		ToolName:  toolName,
		ToolInput: toolInput,
		CreatedAt: time.Now().UTC(),
	}

	g.mu.Lock()
	g.pending[reqID] = &pendingRequest{req: req, decision: ch}
	g.mu.Unlock()

	if g.onAwait != nil {
		g.onAwait(req)
	}

	select {
	case decision := <-ch:
		g.mu.Lock()
		delete(g.pending, reqID)
		g.mu.Unlock()

		if decision.Approved {
			return "", nil
		}
		return decision.Reason, nil

	case <-time.After(g.timeout):
		g.mu.Lock()
		delete(g.pending, reqID)
		g.mu.Unlock()

		return fmt.Sprintf("permission timeout after %v (fail-closed: denied)", g.timeout), nil
	}
}

// Decide delivers KA's approval or denial for a pending permission request.
// Returns an error if the request_id is not found (expired or already decided).
func (g *PermissionGate) Decide(requestID string, decision PermissionDecision) error {
	g.mu.Lock()
	pr, ok := g.pending[requestID]
	g.mu.Unlock()

	if !ok {
		return fmt.Errorf("permission request %q not found (expired or already decided)", requestID)
	}

	pr.decision <- decision
	return nil
}

// PendingRequests returns a snapshot of all currently pending permission requests.
func (g *PermissionGate) PendingRequests() []PermissionRequest {
	g.mu.Lock()
	defer g.mu.Unlock()

	reqs := make([]PermissionRequest, 0, len(g.pending))
	for _, pr := range g.pending {
		reqs = append(reqs, pr.req)
	}
	return reqs
}

// PendingCount returns the number of currently blocked permission requests.
func (g *PermissionGate) PendingCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.pending)
}
