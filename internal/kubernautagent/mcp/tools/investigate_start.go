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
	"time"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

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

	// BR-INTERACTIVE-010: Check for pending interactive session and launch it.
	// When launched, the investigation will self-transition to StatusUserDriving
	// via InteractiveHold — skip TransitionToUserDriving below to avoid cancelling
	// the RCA goroutine prematurely.
	launchedPending, investigationSessionID := t.launchPendingInteractiveSession(input)

	sess, startErr := t.startInteractiveSession(ctx, input, user)
	if startErr != nil {
		return InvestigateOutput{}, startErr
	}

	// #1390: Upgrade running autonomous session in-place (Jump In) instead of
	// cancelling and recreating. UpgradeToInteractive sets the atomic flag so
	// the goroutine's next InteractiveHold check sees it, and store.Update's
	// deterministic check catches completion that already happened.
	// Skip when we just launched a pending session — its RCA goroutine will
	// self-transition via InteractiveHold once complete.
	if !launchedPending {
		investigationSessionID = t.upgradeOrCreateInteractiveSession(ctx, input, user)
	}

	if t.metrics != nil {
		t.metrics.RecordInteractiveSessionStarted()
		t.metrics.RecordInteractiveTakeover("start_success")
	}

	t.emitInteractiveStarted(sess.SessionID, input.RRID, user.Username)
	t.startTimeoutTracking(sess.SessionID)
	t.storeReconstructedContext(ctx, input.RRID, sess.SessionID)

	return InvestigateOutput{
		SessionID:              sess.SessionID,
		Status:                 "started",
		InvestigationSessionID: investigationSessionID,
	}, nil
}

// launchPendingInteractiveSession launches a previously-deferred interactive
// investigation, if one is pending for this session/RR. Returns whether a
// pending session was launched and, when so, its session ID (used as the
// InvestigationSessionID so the caller skips the autonomous-session upgrade
// path below, since the deferred RCA goroutine self-transitions via
// InteractiveHold once complete).
func (t *InvestigateTool) launchPendingInteractiveSession(input InvestigateInput) (bool, string) {
	pendingID := input.SessionID
	hasPending := pendingID != ""
	if !hasPending {
		pendingID, hasPending = t.autoMgr.FindPendingByRemediationID(input.RRID)
	}
	if !hasPending {
		return false, ""
	}
	if launchErr := t.autoMgr.LaunchDeferredInvestigation(pendingID); launchErr != nil {
		t.logger.Error(launchErr, "start: failed to launch deferred investigation",
			"rr_id", input.RRID, "pending_session_id", pendingID)
		return false, ""
	}
	return true, pendingID
}

// acquireInteractiveLease takes over the interactive lease for rrID and maps
// each failure mode (lease held, max sessions reached, or a generic takeover
// error) to the appropriate MCP error and metric. raceLostMetric is recorded
// when another driver holds the lease; failedMetric is recorded for every
// other failure. genericErrPrefix labels the wrapped error text for the
// generic-failure case (callers use distinct prefixes for start vs takeover).
// Does not evaluate sess.Reconnected — callers handle the reconnect case
// differently (handleStart rejects it, handleTakeover treats it as a
// successful rejoin).
func (t *InvestigateTool) acquireInteractiveLease(ctx context.Context, rrID string, user mcpinternal.UserInfo, raceLostMetric, failedMetric, genericErrPrefix string) (*mcpinternal.InteractiveSession, error) {
	sess, err := t.sessions.Takeover(ctx, rrID, user)
	if err == nil {
		return sess, nil
	}

	if errors.Is(err, mcpinternal.ErrLeaseHeld) {
		if t.metrics != nil {
			t.metrics.RecordInteractiveLeaseContention()
			t.metrics.RecordInteractiveTakeover(raceLostMetric)
		}
		driver, _ := t.sessions.GetDriver(rrID)
		driverName := "unknown"
		if driver != nil {
			driverName = driver.ActingUser.Username
		}
		return nil, ErrCodeSessionActive.WithDetail("driver", driverName)
	}
	if errors.Is(err, mcpinternal.ErrMaxSessionsReached) {
		if t.metrics != nil {
			t.metrics.RecordInteractiveTakeover(failedMetric)
		}
		return nil, &MCPError{Code: "max_sessions", Message: "Maximum concurrent sessions reached"}
	}
	if t.metrics != nil {
		t.metrics.RecordInteractiveTakeover(failedMetric)
	}
	return nil, fmt.Errorf("%s: %w", genericErrPrefix, err)
}

// startInteractiveSession takes over the interactive lease for the RR and
// records the appropriate metrics/error mapping for each failure mode
// (lease held, max sessions reached, reconnect-in-progress, or a generic
// takeover error).
func (t *InvestigateTool) startInteractiveSession(ctx context.Context, input InvestigateInput, user mcpinternal.UserInfo) (*mcpinternal.InteractiveSession, error) {
	sess, err := t.acquireInteractiveLease(ctx, input.RRID, user, "start_failed", "start_failed", "start session")
	if err != nil {
		return nil, err
	}

	if sess.Reconnected {
		return nil, &MCPError{
			Code:    "session_active",
			Message: "You already have an active session for this investigation; use action=reconnect to rejoin",
			Details: map[string]string{
				"driver":     user.Username,
				"session_id": sess.SessionID,
			},
		}
	}

	return sess, nil
}

// upgradeOrCreateInteractiveSession upgrades the running autonomous session
// for this RR in-place (Jump In), or — when no autonomous session exists, or
// the existing one is terminal — creates a fresh interactive session so the
// user is never left with a lease but no investigation to drive (#1440
// SC-24). Returns the resulting investigation session ID.
func (t *InvestigateTool) upgradeOrCreateInteractiveSession(ctx context.Context, input InvestigateInput, user mcpinternal.UserInfo) string {
	autoSessionID, found := t.autoMgr.FindByRemediationID(input.RRID)
	if !found {
		// No session exists for this RR — create fresh interactive session so
		// the user is never left with a lease but no investigation.
		if freshID := t.createFallbackSession(ctx, input.RRID, user); freshID != "" {
			return freshID
		}
		if forceErr := t.autoMgr.ForceTransitionToUserDriving(input.RRID, user.Username, user.Groups); forceErr != nil {
			t.logger.Error(forceErr, "start: force-transition to user-driving (no running session found)",
				"rr_id", input.RRID)
		}
		return ""
	}

	upgradeErr := t.autoMgr.UpgradeToInteractive(autoSessionID, user.Username, user.Groups)
	if upgradeErr == nil {
		return autoSessionID
	}
	if !errors.Is(upgradeErr, session.ErrSessionTerminal) {
		t.logger.Error(upgradeErr, "start: upgrade autonomous session to interactive",
			"rr_id", input.RRID, "auto_session_id", autoSessionID)
		return autoSessionID
	}

	if forceErr := t.autoMgr.ForceTransitionToUserDriving(input.RRID, user.Username, user.Groups); forceErr != nil {
		t.logger.Error(forceErr, "start: force-transition to user-driving failed (session terminal)",
			"rr_id", input.RRID, "auto_session_id", autoSessionID)
	}
	// #1440 SC-24: Terminal session — create fresh interactive session so the
	// user always has an investigation to drive.
	if freshID := t.createFallbackSession(ctx, input.RRID, user); freshID != "" {
		return freshID
	}
	return autoSessionID
}

// transitionAutonomousToUserDriving transitions the running autonomous
// session for rrID to user-driven (#774: TransitionToUserDriving replaces
// SuspendInvestigation so the session enters StatusUserDriving — pollable,
// not terminal — with identity written to session metadata for the AA poll
// response's Rego input.identity). When no autonomous session is found by RR
// ID, retries ForceTransitionToUserDriving briefly to allow for the race
// between MCP takeover and AA reconcile. Returns a non-nil error only for a
// non-terminal TransitionToUserDriving failure; terminal-session and
// not-found cases are logged and treated as best-effort.
func (t *InvestigateTool) transitionAutonomousToUserDriving(rrID string, user mcpinternal.UserInfo) error {
	autoSessionID, found := t.autoMgr.FindByRemediationID(rrID)
	if !found {
		// No running session found by RR ID. The AA session submit may still
		// be in-flight (race between MCP takeover and AA reconcile). Retry
		// briefly to allow the session to appear before giving up.
		var forceErr error
		for attempt := 0; attempt < 5; attempt++ {
			forceErr = t.autoMgr.ForceTransitionToUserDriving(rrID, user.Username, user.Groups)
			if forceErr == nil || !errors.Is(forceErr, session.ErrSessionNotFound) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if forceErr != nil {
			t.logger.Error(forceErr, "takeover: force-transition to user-driving failed after retries",
				"rr_id", rrID)
		}
		return nil
	}

	err := t.autoMgr.TransitionToUserDriving(autoSessionID, user.Username, user.Groups)
	if err == nil {
		return nil
	}
	if !errors.Is(err, session.ErrSessionTerminal) {
		return fmt.Errorf("transition autonomous session to user-driving: %w", err)
	}
	if forceErr := t.autoMgr.ForceTransitionToUserDriving(rrID, user.Username, user.Groups); forceErr != nil {
		t.logger.Error(forceErr, "takeover: force-transition to user-driving failed",
			"rr_id", rrID, "auto_session_id", autoSessionID)
	}
	return nil
}
