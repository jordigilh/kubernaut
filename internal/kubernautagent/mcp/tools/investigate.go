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

package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

// LLMMessage represents a single conversation message for the investigator.
type LLMMessage struct {
	Role    string
	Content string
}

// InvestigatorRunner is the interface for executing interactive LLM turns.
// Implemented by the real Investigator.RunInteractiveTurn via an adapter.
type InvestigatorRunner interface {
	RunInteractiveTurn(ctx context.Context, messages []LLMMessage, correlationID string) (string, error)
}

// AutonomousSessionManager provides lookup and cancellation of autonomous
// investigation sessions. Used by handleTakeover to cancel the running
// autonomous session before acquiring the interactive Lease.
type AutonomousSessionManager interface {
	FindByRemediationID(rrID string) (string, bool)
	CancelInvestigation(id string) error
}

// InvestigateTool handles the kubernaut_investigate MCP tool actions:
// start, message, complete, cancel, takeover. BR-INTERACTIVE-001, BR-INTERACTIVE-004.
type InvestigateTool struct {
	sessions     mcpinternal.SessionManager
	runner       InvestigatorRunner
	recon        mcpinternal.ContextReconstructor
	autoMgr      AutonomousSessionManager
	metrics      ToolMetrics
	sessionMu    sync.Map // rrID -> *sync.Mutex (per-session serialization)
	reconHistory sync.Map // rrID -> []LLMMessage (reconstructed context for LLM)
}

// InvestigateOption configures optional dependencies for InvestigateTool.
type InvestigateOption func(*InvestigateTool)

// WithAutonomousManager enables autonomous session takeover.
func WithAutonomousManager(mgr AutonomousSessionManager) InvestigateOption {
	return func(t *InvestigateTool) {
		if mgr != nil {
			t.autoMgr = mgr
		}
	}
}

// WithToolMetrics enables metrics recording on tool operations (PROD-01).
func WithToolMetrics(m ToolMetrics) InvestigateOption {
	return func(t *InvestigateTool) {
		if m != nil {
			t.metrics = m
		}
	}
}

// NewInvestigateTool creates the tool handler with its dependencies.
func NewInvestigateTool(sessions mcpinternal.SessionManager, runner InvestigatorRunner, recon mcpinternal.ContextReconstructor, opts ...InvestigateOption) *InvestigateTool {
	t := &InvestigateTool{
		sessions: sessions,
		runner:   runner,
		recon:    recon,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// getSessionMutex returns a per-rrID mutex for serializing concurrent requests.
func (t *InvestigateTool) getSessionMutex(rrID string) *sync.Mutex {
	val, _ := t.sessionMu.LoadOrStore(rrID, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// Handle dispatches the input to the correct action handler.
func (t *InvestigateTool) Handle(ctx context.Context, input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	if err := ValidateInput(input); err != nil {
		return InvestigateOutput{}, err
	}

	start := time.Now()
	output, err := t.dispatch(ctx, input, user)

	// PROD-01: Record command duration for all actions.
	if t.metrics != nil {
		t.metrics.RecordInteractiveCommandDuration("kubernaut_investigate", input.Action, time.Since(start).Seconds())
	}

	return output, err
}

func (t *InvestigateTool) dispatch(ctx context.Context, input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	switch input.Action {
	case ActionStart:
		return t.handleStart(ctx, input, user)
	case ActionTakeover:
		return t.handleTakeover(ctx, input, user)
	case ActionMessage:
		return t.handleMessage(ctx, input, user)
	case ActionComplete:
		return t.handleComplete(input)
	case ActionCancel:
		return t.handleCancel(input)
	case ActionStatus:
		return t.handleStatus(input)
	default:
		return InvestigateOutput{}, ErrInvalidAction
	}
}

func (t *InvestigateTool) handleStart(ctx context.Context, input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	sess, err := t.sessions.Takeover(ctx, input.RRID, user)
	if err != nil {
		if errors.Is(err, mcpinternal.ErrLeaseHeld) && t.metrics != nil {
			t.metrics.RecordInteractiveLeaseContention()
		}
		return InvestigateOutput{}, err
	}

	if t.metrics != nil {
		t.metrics.RecordInteractiveSessionStarted()
		t.metrics.RecordInteractiveTakeover("start_success")
	}

	t.storeReconstructedContext(ctx, input.RRID, sess.SessionID)

	return InvestigateOutput{
		SessionID: sess.SessionID,
		Status:    "started",
	}, nil
}

func (t *InvestigateTool) handleTakeover(ctx context.Context, input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	mu := t.getSessionMutex(input.RRID)
	mu.Lock()
	defer mu.Unlock()

	if t.autoMgr != nil {
		autoSessionID, found := t.autoMgr.FindByRemediationID(input.RRID)
		if found {
			if err := t.autoMgr.CancelInvestigation(autoSessionID); err != nil {
				if errors.Is(err, session.ErrSessionTerminal) {
					return InvestigateOutput{}, ErrCodeInvestigationCompleted
				}
				return InvestigateOutput{}, fmt.Errorf("cancel autonomous session: %w", err)
			}
		}
	}

	sess, err := t.sessions.Takeover(ctx, input.RRID, user)
	if err != nil {
		if errors.Is(err, mcpinternal.ErrLeaseHeld) {
			if t.metrics != nil {
				t.metrics.RecordInteractiveLeaseContention()
			}
			driver, _ := t.sessions.GetDriver(input.RRID)
			driverName := "unknown"
			if driver != nil {
				driverName = driver.ActingUser.Username
			}
			return InvestigateOutput{}, ErrCodeSessionActive.WithDetail("driver", driverName)
		}
		return InvestigateOutput{}, fmt.Errorf("takeover session: %w", err)
	}

	if t.metrics != nil {
		t.metrics.RecordInteractiveSessionStarted()
		t.metrics.RecordInteractiveTakeover("takeover_success")
	}

	reconCount := t.storeReconstructedContext(ctx, input.RRID, sess.SessionID)
	contextSummary := fmt.Sprintf("%d prior turns reconstructed", reconCount)

	return InvestigateOutput{
		SessionID: sess.SessionID,
		Status:    "takeover_started",
		Response:  contextSummary,
	}, nil
}

func (t *InvestigateTool) handleMessage(ctx context.Context, input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	mu := t.getSessionMutex(input.RRID)
	mu.Lock()
	defer mu.Unlock()

	if !t.sessions.IsDriverActive(input.RRID) {
		return InvestigateOutput{}, ErrCodeNotDriving
	}

	sess, err := t.sessions.GetDriver(input.RRID)
	if err != nil {
		if errors.Is(err, mcpinternal.ErrSessionExpired) {
			return InvestigateOutput{}, ErrCodeSessionExpired
		}
		return InvestigateOutput{}, ErrCodeNotDriving
	}
	if sess == nil {
		return InvestigateOutput{}, ErrCodeNotDriving
	}

	if sess.ActingUser.Username != user.Username {
		return InvestigateOutput{}, ErrCodeSessionActive.WithDetail("driver", sess.ActingUser.Username)
	}

	// SEC-04: Touch activity to reset inactivity timer.
	t.sessions.TouchActivity(input.RRID)

	// PROD-02: Prepend reconstructed context turns so the LLM has full history.
	messages := t.buildMessagesWithContext(input.RRID, input.Message)

	response, err := t.runner.RunInteractiveTurn(ctx, messages, input.RRID)
	if err != nil {
		return InvestigateOutput{}, fmt.Errorf("interactive turn failed: %w", err)
	}

	return InvestigateOutput{
		SessionID: sess.SessionID,
		Status:    "message_received",
		Response:  response,
	}, nil
}

func (t *InvestigateTool) handleComplete(input InvestigateInput) (InvestigateOutput, error) {
	if !t.sessions.IsDriverActive(input.RRID) {
		return InvestigateOutput{}, nil
	}

	sess, err := t.sessions.GetDriver(input.RRID)
	if err != nil || sess == nil {
		return InvestigateOutput{}, nil
	}

	if err := t.sessions.Release(sess.SessionID, "complete"); err != nil {
		if errors.Is(err, mcpinternal.ErrSessionNotFound) {
			return InvestigateOutput{SessionID: sess.SessionID, Status: "completed"}, nil
		}
		return InvestigateOutput{}, fmt.Errorf("release session: %w", err)
	}

	if t.metrics != nil {
		t.metrics.RecordInteractiveSessionEnded()
	}

	t.sessionMu.Delete(input.RRID)
	t.reconHistory.Delete(input.RRID)

	return InvestigateOutput{
		SessionID: sess.SessionID,
		Status:    "completed",
	}, nil
}

func (t *InvestigateTool) handleStatus(input InvestigateInput) (InvestigateOutput, error) {
	status := StatusOutput{RRID: input.RRID}

	if t.sessions.IsDriverActive(input.RRID) {
		driver, _ := t.sessions.GetDriver(input.RRID)
		status.Mode = StatusModeInteractive
		if driver != nil {
			status.Driver = driver.ActingUser.Username
		}
	} else if t.autoMgr != nil {
		if _, found := t.autoMgr.FindByRemediationID(input.RRID); found {
			status.Mode = StatusModeAutonomous
		} else {
			status.Mode = StatusModeNotFound
		}
	} else {
		status.Mode = StatusModeNotFound
	}

	data, err := json.Marshal(status)
	if err != nil {
		return InvestigateOutput{}, fmt.Errorf("marshal status: %w", err)
	}

	return InvestigateOutput{
		Status:   "status",
		Response: string(data),
	}, nil
}

func (t *InvestigateTool) handleCancel(input InvestigateInput) (InvestigateOutput, error) {
	if !t.sessions.IsDriverActive(input.RRID) {
		return InvestigateOutput{}, ErrNoActiveSession
	}

	sess, err := t.sessions.GetDriver(input.RRID)
	if err != nil || sess == nil {
		return InvestigateOutput{}, ErrNoActiveSession
	}

	if err := t.sessions.Release(sess.SessionID, "explicit"); err != nil {
		return InvestigateOutput{}, fmt.Errorf("release session: %w", err)
	}

	if t.metrics != nil {
		t.metrics.RecordInteractiveSessionEnded()
	}

	t.sessionMu.Delete(input.RRID)
	t.reconHistory.Delete(input.RRID)

	return InvestigateOutput{
		SessionID: sess.SessionID,
		Status:    "cancelled",
	}, nil
}

// storeReconstructedContext queries the reconstructor and caches prior turns
// for the session's lifetime. Returns the number of turns stored.
func (t *InvestigateTool) storeReconstructedContext(ctx context.Context, rrID, sessionID string) int {
	turns, _ := t.recon.Reconstruct(ctx, rrID, sessionID)
	if len(turns) == 0 {
		return 0
	}

	history := make([]LLMMessage, len(turns))
	for i, turn := range turns {
		history[i] = LLMMessage{Role: turn.Role, Content: turn.Content}
	}
	t.reconHistory.Store(rrID, history)
	return len(history)
}

// buildMessagesWithContext prepends any cached reconstruction history to the
// current user message, giving the LLM full prior context (PROD-02).
func (t *InvestigateTool) buildMessagesWithContext(rrID, userMessage string) []LLMMessage {
	var messages []LLMMessage
	if raw, ok := t.reconHistory.Load(rrID); ok {
		messages = append(messages, raw.([]LLMMessage)...)
	}
	messages = append(messages, LLMMessage{Role: "user", Content: userMessage})
	return messages
}
