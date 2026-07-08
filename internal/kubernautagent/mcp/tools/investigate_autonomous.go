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
	"errors"
	"fmt"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

func (t *InvestigateTool) handleCancel(input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	mu := t.getSessionMutex(input.RRID)
	mu.Lock()
	defer mu.Unlock()

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
		if !errors.Is(err, mcpinternal.ErrSessionNotFound) {
			return InvestigateOutput{}, fmt.Errorf("release session: %w", err)
		}
	}

	t.emitInteractiveCompleted(sess.SessionID, input.RRID, user.Username, "cancel")

	CompleteHTTPSession(t.httpCompleter, input.RRID, nil, t.logger, "cancel")

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

func (t *InvestigateTool) handleStartAutonomous(ctx context.Context, input InvestigateInput, _ mcpinternal.UserInfo) (InvestigateOutput, error) {
	if t.rrChecker != nil {
		exists, err := t.rrChecker.RemediationRequestExists(ctx, input.RRID)
		if err != nil {
			return InvestigateOutput{}, fmt.Errorf("validate remediation request: %w", err)
		}
		if !exists {
			return InvestigateOutput{}, ErrCodeRRNotFound.WithDetail("rr_id", input.RRID)
		}
	}

	if existingID, found := t.autoMgr.FindByRemediationID(input.RRID); found {
		return InvestigateOutput{
			SessionID: existingID,
			Status:    "already_running",
		}, nil
	}

	metadata := map[string]string{
		"remediation_id": input.RRID,
	}

	// F4 (#1374): Resolve signal context to build a real InvestigateFunc.
	// The autonomous investigation uses the same full pipeline as the HTTP path.
	if t.signalResolver == nil {
		return InvestigateOutput{}, fmt.Errorf("start autonomous investigation: signal resolver not configured")
	}
	resolved, resolveErr := t.signalResolver.ResolveSignalContext(ctx, input.RRID)
	if resolveErr != nil {
		return InvestigateOutput{}, fmt.Errorf("resolve signal context for autonomous investigation: %w", resolveErr)
	}
	if resolved == nil {
		return InvestigateOutput{}, fmt.Errorf("start autonomous investigation: no signal context for rr_id %s", input.RRID)
	}
	signal := *resolved

	sessionID, err := t.autoMgr.StartInvestigation(ctx, func(bgCtx context.Context) (*katypes.InvestigationResult, error) {
		return t.runner.RunFullInvestigation(bgCtx, signal)
	}, metadata)
	if err != nil {
		if errors.Is(err, session.ErrMaxInvestigationsReached) {
			return InvestigateOutput{}, ErrCodeMaxInvestigations
		}
		return InvestigateOutput{}, fmt.Errorf("start autonomous investigation: %w", err)
	}

	if _, subErr := t.autoMgr.Subscribe(ctx, sessionID); subErr != nil {
		t.logger.Error(subErr, "start_autonomous: Subscribe failed, events may be lost",
			"session_id", sessionID, "rr_id", input.RRID)
	}

	return InvestigateOutput{
		SessionID: sessionID,
		Status:    "autonomous_started",
	}, nil
}

// createFallbackSession creates a fresh interactive session when no viable
// autonomous session exists (no session found, or terminal session). This
// ensures the user always has an investigation to drive after acquiring the
// MCP lease (SC-24, #1440).
func (t *InvestigateTool) createFallbackSession(ctx context.Context, rrID string, user mcpinternal.UserInfo) string {
	// #1640: key must be "remediation_id" to match every other by-RR-ID
	// lookup (FindByRemediationID, FindUserDrivingByRemediationID, etc.) —
	// using "rr_id" here made fallback sessions invisible to those lookups,
	// including the one #1639 needs to attach a live-streaming LazySink.
	metadata := map[string]string{
		"remediation_id": rrID,
		"username":       user.Username,
		"mode":           "interactive_fallback",
	}
	investigateFn := session.InvestigateFunc(func(_ context.Context) (*katypes.InvestigationResult, error) {
		return &katypes.InvestigationResult{
			RCASummary:      "Interactive session — awaiting user direction",
			InteractiveHold: true,
		}, nil
	})
	sessionID, err := t.autoMgr.StartInvestigation(ctx, investigateFn, metadata)
	if err != nil {
		t.logger.Error(err, "start: fallback session creation failed",
			"rr_id", rrID, "username", user.Username)
		return ""
	}
	return sessionID
}

// storeReconstructedContext queries the reconstructor and caches prior turns
// for the session's lifetime. Prefers RCA summary from a prior completed
// session (more concise, prevents token bloat) over full audit trail
// reconstruction. Returns the number of turns stored.
func (t *InvestigateTool) storeReconstructedContext(ctx context.Context, rrID, sessionID string) int {
	// BR-INTERACTIVE-010: If a prior session produced an RCA summary, use it
	// as a concise seed instead of reconstructing the full audit trail.
	if rcaSummary, hasRCA := t.autoMgr.GetLatestRCASummaryByRemediationID(rrID); hasRCA {
		history := []LLMMessage{
			{Role: "assistant", Content: "Previous investigation RCA summary: " + rcaSummary},
		}
		t.reconHistory.Store(rrID, history)
		return 1
	}

	turns, reconErr := t.recon.Reconstruct(ctx, rrID, sessionID)
	if reconErr != nil {
		t.logger.Error(reconErr, "context reconstruction from DS failed, proceeding with empty context",
			"rr_id", rrID, "session_id", sessionID)
	}
	if len(turns) == 0 {
		return 0
	}

	history := make([]LLMMessage, 0, len(turns))
	for _, turn := range turns {
		if turn.Content == "" {
			continue
		}
		history = append(history, LLMMessage{Role: turn.Role, Content: turn.Content})
	}
	if len(history) == 0 {
		return 0
	}
	t.reconHistory.Store(rrID, history)
	return len(history)
}

// appendConversationTurn appends a user message and the LLM response to
// reconHistory so that discover_workflows can extract RCA from the
// accumulated interactive conversation.
func (t *InvestigateTool) appendConversationTurn(rrID, userMessage, assistantResponse string) {
	var history []LLMMessage
	if raw, ok := t.reconHistory.Load(rrID); ok {
		history = raw.([]LLMMessage)
	}
	history = append(history,
		LLMMessage{Role: "user", Content: userMessage},
		LLMMessage{Role: "assistant", Content: assistantResponse},
	)
	t.reconHistory.Store(rrID, history)
}

// buildMessagesWithContext prepends any cached reconstruction history to the
// current user message, giving the LLM full prior context (PROD-02).
// Copies the cached slice to avoid aliasing the sync.Map entry.
func (t *InvestigateTool) buildMessagesWithContext(rrID, userMessage string) []LLMMessage {
	var history []LLMMessage
	if raw, ok := t.reconHistory.Load(rrID); ok {
		cached := raw.([]LLMMessage)
		history = make([]LLMMessage, len(cached))
		copy(history, cached)
	}
	messages := make([]LLMMessage, 0, len(history)+1)
	messages = append(messages, history...)
	messages = append(messages, LLMMessage{Role: "user", Content: userMessage})
	return messages
}

// emitInteractiveStarted emits aiagent.interactive.started (BR-INTERACTIVE-003, DD-INTERACTIVE-002).
// Uses context.Background() because audit is fire-and-forget (ADR-038) and must not
// be tied to the request lifecycle — a cancelled request context must not drop the event.
func (t *InvestigateTool) emitInteractiveStarted(sessionID, correlationID, actingUser string) {
	if t.auditStore == nil {
		return
	}
	event := audit.NewEvent(audit.EventTypeInteractiveStarted, correlationID,
		audit.WithSessionID(sessionID),
		audit.WithActingUser(actingUser),
	)
	event.EventAction = audit.ActionInteractiveStarted
	event.EventOutcome = audit.OutcomeSuccess
	audit.StoreBestEffort(context.Background(), t.auditStore, event, t.logger)
}

// emitInteractiveCompleted emits aiagent.interactive.completed (BR-INTERACTIVE-003, DD-INTERACTIVE-002).
func (t *InvestigateTool) emitInteractiveCompleted(sessionID, correlationID, actingUser, reason string) {
	if t.auditStore == nil {
		return
	}
	event := audit.NewEvent(audit.EventTypeInteractiveCompleted, correlationID,
		audit.WithSessionID(sessionID),
		audit.WithActingUser(actingUser),
	)
	event.EventAction = audit.ActionInteractiveCompleted
	event.EventOutcome = audit.OutcomeSuccess
	event.Data["reason"] = reason
	audit.StoreBestEffort(context.Background(), t.auditStore, event, t.logger)
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
