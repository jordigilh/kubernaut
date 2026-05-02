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
	"github.com/jordigilh/kubernaut/pkg/shared/transport"
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

// RRExistenceChecker validates that a RemediationRequest exists before
// creating a Lease. Prevents orphaned Lease resources for non-existent RRs
// (HARM-004). Implemented by a thin K8s client wrapper at wiring time.
type RRExistenceChecker interface {
	RemediationRequestExists(ctx context.Context, rrID string) (bool, error)
}

// MessageRateLimiter enforces per-session rate limits on tool messages.
// Implemented by *mcp.SessionRateLimiter.
type MessageRateLimiter interface {
	Allow(sessionID string, messageSize int) error
}

// TimeoutTracker manages per-session inactivity timeouts.
// Implemented by *mcp.TimeoutManager.
type TimeoutTracker interface {
	StartTracking(sessionID string, notify func(msg string))
	ResetInactivity(sessionID string)
	StopTracking(sessionID string)
}

// InvestigateTool handles the kubernaut_investigate MCP tool actions:
// start, message, complete, cancel, takeover. BR-INTERACTIVE-001, BR-INTERACTIVE-004.
type InvestigateTool struct {
	sessions       mcpinternal.SessionManager
	runner         InvestigatorRunner
	recon          mcpinternal.ContextReconstructor
	autoMgr        AutonomousSessionManager
	rrChecker      RRExistenceChecker
	metrics        ToolMetrics
	rateLimiter    MessageRateLimiter
	timeoutTracker TimeoutTracker
	notifyFn       func(sessionID, msg string) // optional: delivers timeout warnings to client
	sessionMu      sync.Map                    // rrID -> *sync.Mutex (per-session serialization)
	reconHistory   sync.Map                    // rrID -> []LLMMessage (reconstructed context for LLM)
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

// WithRateLimiter enables per-session message rate limiting (SEC-HIGH-01).
func WithRateLimiter(rl MessageRateLimiter) InvestigateOption {
	return func(t *InvestigateTool) {
		if rl != nil {
			t.rateLimiter = rl
		}
	}
}

// WithTimeoutTracker enables inactivity timeout tracking for sessions.
func WithTimeoutTracker(tt TimeoutTracker) InvestigateOption {
	return func(t *InvestigateTool) {
		if tt != nil {
			t.timeoutTracker = tt
		}
	}
}

// WithRRExistenceChecker enables pre-Lease validation that the target
// RemediationRequest exists (HARM-004: prevents orphaned Lease resources).
func WithRRExistenceChecker(checker RRExistenceChecker) InvestigateOption {
	return func(t *InvestigateTool) {
		if checker != nil {
			t.rrChecker = checker
		}
	}
}

// WithNotifyFunc sets the callback for delivering timeout warnings to the client.
func WithNotifyFunc(fn func(sessionID, msg string)) InvestigateOption {
	return func(t *InvestigateTool) {
		if fn != nil {
			t.notifyFn = fn
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
		return t.handleComplete(input, user)
	case ActionCancel:
		return t.handleCancel(input, user)
	case ActionStatus:
		return t.handleStatus(input)
	default:
		return InvestigateOutput{}, ErrInvalidAction
	}
}

func (t *InvestigateTool) handleStart(ctx context.Context, input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	if t.rrChecker != nil {
		exists, err := t.rrChecker.RemediationRequestExists(ctx, input.RRID)
		if err != nil {
			return InvestigateOutput{}, fmt.Errorf("validate remediation request: %w", err)
		}
		if !exists {
			return InvestigateOutput{}, ErrCodeRRNotFound.WithDetail("rr_id", input.RRID)
		}
	}

	sess, err := t.sessions.Takeover(ctx, input.RRID, user)
	if err != nil {
		if errors.Is(err, mcpinternal.ErrLeaseHeld) {
			if t.metrics != nil {
				t.metrics.RecordInteractiveLeaseContention()
			}
			return InvestigateOutput{}, ErrCodeSessionActive
		}
		if errors.Is(err, mcpinternal.ErrMaxSessionsReached) {
			return InvestigateOutput{}, &MCPError{Code: "max_sessions", Message: "Maximum concurrent sessions reached"}
		}
		return InvestigateOutput{}, fmt.Errorf("start session: %w", err)
	}

	if t.metrics != nil {
		t.metrics.RecordInteractiveSessionStarted()
		t.metrics.RecordInteractiveTakeover("start_success")
	}

	t.startTimeoutTracking(sess.SessionID)
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

	t.startTimeoutTracking(sess.SessionID)

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

	// SEC-HIGH-01: Enforce per-session message rate limit before processing.
	if t.rateLimiter != nil {
		if rlErr := t.rateLimiter.Allow(sess.SessionID, len(input.Message)); rlErr != nil {
			if errors.Is(rlErr, mcpinternal.ErrRateLimited) {
				return InvestigateOutput{}, ErrCodeRateLimited
			}
			return InvestigateOutput{}, ErrCodeRateLimited
		}
	}

	// SEC-04: Touch activity to reset inactivity timer.
	t.sessions.TouchActivity(input.RRID)
	if t.timeoutTracker != nil {
		t.timeoutTracker.ResetInactivity(sess.SessionID)
	}

	// SEC-06 (#703): Enrich context with session user identity so the
	// ImpersonatingRoundTripper injects Impersonate-User/Group headers on
	// K8s API calls, enforcing the user's RBAC during tool execution.
	ctx = transport.WithImpersonatedUser(ctx, user.Username, user.Groups)

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

func (t *InvestigateTool) handleComplete(input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	if !t.sessions.IsDriverActive(input.RRID) {
		return InvestigateOutput{}, ErrCodeNotFound
	}

	sess, err := t.sessions.GetDriver(input.RRID)
	if err != nil || sess == nil {
		return InvestigateOutput{}, ErrCodeNotFound
	}

	// SEC-CRIT-01: Only the active driver may terminate the session.
	if sess.ActingUser.Username != user.Username {
		return InvestigateOutput{}, ErrCodeSessionActive.WithDetail("driver", sess.ActingUser.Username)
	}

	if t.timeoutTracker != nil {
		t.timeoutTracker.StopTracking(sess.SessionID)
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

func (t *InvestigateTool) handleCancel(input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	if !t.sessions.IsDriverActive(input.RRID) {
		return InvestigateOutput{}, ErrNoActiveSession
	}

	sess, err := t.sessions.GetDriver(input.RRID)
	if err != nil || sess == nil {
		return InvestigateOutput{}, ErrNoActiveSession
	}

	// SEC-CRIT-01: Only the active driver may cancel the session.
	if sess.ActingUser.Username != user.Username {
		return InvestigateOutput{}, ErrCodeSessionActive.WithDetail("driver", sess.ActingUser.Username)
	}

	if t.timeoutTracker != nil {
		t.timeoutTracker.StopTracking(sess.SessionID)
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

// startTimeoutTracking begins inactivity tracking for a session if configured.
func (t *InvestigateTool) startTimeoutTracking(sessionID string) {
	if t.timeoutTracker == nil {
		return
	}
	notify := func(msg string) {
		if t.notifyFn != nil {
			t.notifyFn(sessionID, msg)
		}
	}
	t.timeoutTracker.StartTracking(sessionID, notify)
}
