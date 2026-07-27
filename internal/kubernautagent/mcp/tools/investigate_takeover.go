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

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

func (t *InvestigateTool) handleTakeover(ctx context.Context, input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	mu := t.getSessionMutex(input.RRID)
	mu.Lock()
	defer mu.Unlock()

	// H4: Acquire the interactive Lease BEFORE suspending autonomous. This ensures
	// that if Takeover fails (lease contention, max sessions), the autonomous
	// investigation is NOT irreversibly cancelled.
	sess, err := t.acquireInteractiveLease(ctx, input.RRID, user, "takeover_race_lost", "takeover_failed", "takeover session")
	if err != nil {
		return InvestigateOutput{}, err
	}

	if sess.Reconnected {
		if t.timeoutTracker != nil {
			t.timeoutTracker.ResetInactivity(sess.SessionID)
		}
		return InvestigateOutput{
			SessionID: sess.SessionID,
			Status:    "reconnected",
		}, nil
	}

	// Lease acquired — now safe to transition autonomous investigation to user-driven.
	if transitionErr := t.transitionAutonomousToUserDriving(input.RRID, user); transitionErr != nil {
		if t.metrics != nil {
			t.metrics.RecordInteractiveTakeover("takeover_failed")
		}
		return InvestigateOutput{}, transitionErr
	}

	if t.metrics != nil {
		t.metrics.RecordInteractiveSessionStarted()
		t.metrics.RecordInteractiveTakeover("takeover_success")
	}

	t.emitInteractiveStarted(sess.SessionID, input.RRID, user.Username) //nolint:contextcheck // emitInteractiveStarted uses audit.StoreBestEffort by design (ADR-038); see investigate_autonomous.go doc comment
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

	sess, authErr := t.authorizeActiveDriver(input.RRID, user)
	if authErr != nil {
		return InvestigateOutput{}, authErr
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

	// #898-S5: Attach session ID for audit attribution on K8s API calls.
	ctx = transport.WithAuditSessionID(ctx, sess.SessionID)

	// F9 / #1374: Attach signal context for PhaseRCA tool parity with
	// the autonomous path. Future tools may read SignalContextFromContext.
	if t.signalResolver != nil {
		if resolved, resolveErr := t.signalResolver.ResolveSignalContext(ctx, input.RRID); resolveErr == nil && resolved != nil {
			ctx = katypes.WithSignalContext(ctx, *resolved)
		}
	}

	// #1639: Enrich context with the HTTP investigation session's LazySink so
	// this turn's RunInteractiveTurn call streams live KA events (reasoning,
	// reasoning_content, etc.) to any subscriber — the same wiring
	// discover_workflows has had since #1384. Without this, live streaming
	// only ever worked for the initial kubernaut_investigate call.
	ctx = t.enrichLiveEventContext(ctx, input.RRID, "message")

	// Clear DiscoveryResult before the LLM call: any message after
	// discover_workflows invalidates stale recommendations, forcing re-discovery
	// before select_workflow can be called.
	if sess.DiscoveryResult != nil {
		sess.DiscoveryResult = nil
	}

	// PROD-02: Prepend reconstructed context turns so the LLM has full history.
	messages := t.buildMessagesWithContext(input.RRID, input.Message)

	response, err := t.runner.RunInteractiveTurn(ctx, messages, input.RRID)
	if err != nil {
		return InvestigateOutput{}, fmt.Errorf("interactive turn failed: %w", err)
	}

	// Accumulate the user message + LLM response in reconHistory so that
	// subsequent actions (discover_workflows) can extract RCA from the
	// full conversation without relying on audit trace reconstruction.
	t.appendConversationTurn(input.RRID, input.Message, response)

	// Reset inactivity timer AFTER the LLM call completes. The pre-call reset
	// (above) prevents timeout during user think-time; this post-call reset
	// prevents timeout during slow LLM responses that exceed the inactivity
	// window. Without this, a 90s LLM response with a 60s timeout would
	// expire the session mid-turn.
	t.sessions.TouchActivity(input.RRID)
	if t.timeoutTracker != nil {
		t.timeoutTracker.ResetInactivity(sess.SessionID)
	}

	return InvestigateOutput{
		SessionID: sess.SessionID,
		Status:    "message_received",
		Response:  response,
	}, nil
}

func (t *InvestigateTool) handleComplete(input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	mu := t.getSessionMutex(input.RRID)
	mu.Lock()
	defer mu.Unlock()

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
			// H3: Session already released (race with timeout/disconnect), but the
			// user sees "completed" — still emit audit for completeness.
			t.emitInteractiveCompleted(sess.SessionID, input.RRID, user.Username, "complete_already_released")
			return InvestigateOutput{SessionID: sess.SessionID, Status: "completed"}, nil
		}
		return InvestigateOutput{}, fmt.Errorf("release session: %w", err)
	}

	t.emitInteractiveCompleted(sess.SessionID, input.RRID, user.Username, "complete")

	var finalResult *katypes.InvestigationResult
	if sess.RCAResult != nil {
		r := *sess.RCAResult
		r.Reason = "investigation completed by user"
		finalResult = &r
	} else {
		finalResult = &katypes.InvestigationResult{
			RCASummary: "Investigation completed without workflow selection",
			Reason:     "investigation completed by user",
		}
	}
	notActionable := false
	finalResult.IsActionable = &notActionable
	finalResult.Warnings = append(finalResult.Warnings, "Alert not actionable")

	CompleteHTTPSession(t.httpCompleter, input.RRID, finalResult, t.logger, "complete")

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

func (t *InvestigateTool) handleStatus(input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	status := StatusOutput{RRID: input.RRID}

	if t.sessions.IsDriverActive(input.RRID) {
		driver, _ := t.sessions.GetDriver(input.RRID)
		status.Mode = StatusModeInteractive
		if driver != nil && driver.ActingUser.Username == user.Username {
			status.Driver = driver.ActingUser.Username
		}
	} else if _, found := t.autoMgr.FindByRemediationID(input.RRID); found {
		status.Mode = StatusModeAutonomous
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

func (t *InvestigateTool) handleReconnect(input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	sess, authErr := t.authorizeActiveDriver(input.RRID, user)
	if authErr != nil {
		return InvestigateOutput{}, authErr
	}

	t.sessions.TouchActivity(input.RRID)
	if t.timeoutTracker != nil {
		t.timeoutTracker.ResetInactivity(sess.SessionID)
	}

	return InvestigateOutput{
		SessionID: sess.SessionID,
		Status:    "reconnected",
	}, nil
}
