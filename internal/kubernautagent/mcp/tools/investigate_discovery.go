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
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

func (t *InvestigateTool) handleDiscoverWorkflows(ctx context.Context, input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	mu := t.getSessionMutex(input.RRID)
	mu.Lock()
	defer mu.Unlock()

	sess, authErr := t.authorizeActiveDriver(input.RRID, user)
	if authErr != nil {
		return InvestigateOutput{}, authErr
	}
	if t.catalog == nil {
		return InvestigateOutput{}, fmt.Errorf("workflow catalog not configured: cannot enrich discovery names")
	}

	// Reset inactivity timer before the (potentially long) LLM calls.
	t.sessions.TouchActivity(input.RRID)
	if t.timeoutTracker != nil {
		t.timeoutTracker.ResetInactivity(sess.SessionID)
	}

	// BR-KA-267, #1949: cascade session-inactivity expiry into cancellation
	// of this handler's in-flight context, so a stuck dependency call (e.g.
	// a catalog read or LLM tool dispatch) cannot outlive the interactive
	// session's stated inactivity bound and leak a goroutine.
	var cancelInactivity func()
	ctx, cancelInactivity = t.withInactivityCancel(ctx, sess.SessionID)
	defer cancelInactivity()

	// Step 1: Obtain the structured RCA result for Phase 3 workflow discovery.
	rcaResult, err := t.resolveRCAForDiscovery(ctx, input.RRID, sess.SessionID, user)
	if err != nil {
		return InvestigateOutput{}, err
	}

	// Step 2: Resolve signal context for Phase 3. Enrichment is handled
	// internally by the investigator's enrichment pipeline (F5 #1374), so we
	// only resolve the signal here.
	signal := t.resolveDiscoverySignal(ctx, input.RRID)

	// Step 3: Enrich context with the HTTP investigation session so that
	// workflow discovery can emit audit events with session_id and stream
	// events to the subscriber (#1384: Bug A fix).
	ctx = t.enrichLiveEventContext(ctx, input.RRID, "discover_workflows")

	// Step 4: Run Phase 3 workflow discovery using the structured RCA.
	// The investigator resolves enrichment internally when its enricher is wired,
	// falling back to nil when not available.
	workflowResult, err := t.runner.RunWorkflowDiscovery(ctx, signal, rcaResult, nil, input.RRID)
	if err != nil {
		return InvestigateOutput{}, fmt.Errorf("workflow discovery failed: %w", err)
	}

	// #2013: When discovery concludes with no_matching_workflows, nothing
	// else in the system ever terminates this interactive session unless a
	// human/agent explicitly calls complete_no_action — it sits "active"
	// until KA's blunt 10-minute inactivity timeout eventually tears it
	// down, and that inactivity teardown is the only thing that emits the
	// session_ended terminal event AF's WatchTerminalEvents (no timer-based
	// safety net, #1438) is waiting on. Auto-close here so the console's
	// "Investigating" phase resolves promptly instead of hanging for up to
	// 10 minutes (BR-INTERACTIVE-010, AU-3, SI-4).
	t.autoCloseOnNoMatchingWorkflows(input.RRID, sess.SessionID, workflowResult)
	t.persistPendingDecision(input.RRID, workflowResult)

	// Step 5: Store results on the interactive session.
	sess.RCAResult = rcaResult
	sess.DiscoveryResult = extractDiscoveryResult(workflowResult)

	// Step 6: Populate discovery target visibility fields (#1437).
	populateDiscoveryTargets(sess.DiscoveryResult, signal, workflowResult.RemediationTarget)

	// Enrich workflow names from catalog.
	t.enrichDiscoveryNames(ctx, sess.DiscoveryResult)

	// Reset inactivity timer after the LLM calls complete.
	t.sessions.TouchActivity(input.RRID)
	if t.timeoutTracker != nil {
		t.timeoutTracker.ResetInactivity(sess.SessionID)
	}

	// Build the JSON response for the user.
	discoveryJSON, err := json.Marshal(sess.DiscoveryResult)
	if err != nil {
		return InvestigateOutput{}, fmt.Errorf("marshal discovery result: %w", err)
	}

	return InvestigateOutput{
		SessionID: sess.SessionID,
		Status:    "workflows_discovered",
		Response:  string(discoveryJSON),
	}, nil
}

// autoCloseOnNoMatchingWorkflows releases the interactive session when
// discovery concludes with no_matching_workflows (#2013). No-op for every
// other outcome (a legitimate recommendation leaves the session active so
// the user can select a workflow).
//
// Mirrors completeSelectionAsync's (select_workflow.go) fire-and-forget
// goroutine pattern so the discover_workflows response reaches the caller
// before session teardown closes the transport, with one addition: the
// #1438 ordering invariant requires EmitSessionEndedByRR to fire BEFORE
// CompleteHTTPSession/Release, so EventLogBridge can forward the terminal
// event to AF before the channel closes -- see cmd/kubernautagent/routes.go's
// inactivity-timeout and disconnect handlers, the only other two call sites,
// both carrying the identical ordering comment.
func (t *InvestigateTool) autoCloseOnNoMatchingWorkflows(rrID, sessionID string, workflowResult *katypes.InvestigationResult) {
	if workflowResult == nil || !workflowResult.HumanReviewNeeded || workflowResult.HumanReviewReason != "no_matching_workflows" {
		return
	}
	if t.timeoutTracker != nil {
		t.timeoutTracker.StopTracking(sessionID)
	}
	if t.httpCompleter == nil {
		return
	}
	autoMgr := t.autoMgr
	completer := t.httpCompleter
	sessions := t.sessions
	tombstone := t.autoCloseTombstone
	result := workflowResult
	logger := t.logger
	go func() {
		// KA-HIGH-3: re-acquire the session mutex to prevent TOCTOU between
		// response delivery and HTTP/lease cleanup.
		mu := t.getSessionMutex(rrID)
		mu.Lock()
		defer mu.Unlock()

		autoMgr.EmitSessionEndedByRR(rrID, "no_matching_workflows")
		CompleteHTTPSession(completer, rrID, result, logger, "no_matching_workflows")
		// #2076 (v1.6 clone of #2075): mark BEFORE releasing the lease, so a
		// complete_no_action call that observes IsDriverActive==false can
		// never find a tombstone miss for a release that already happened
		// by the time it checks.
		if tombstone != nil {
			tombstone.Mark(rrID)
		}
		if releaseErr := sessions.Release(sessionID, "no_matching_workflows"); releaseErr != nil {
			if !errors.Is(releaseErr, mcpinternal.ErrSessionNotFound) {
				logger.Error(releaseErr, "failed to release MCP lease", "session_id", sessionID)
			}
		}
	}()
}

// persistPendingDecision preserves a discovered-and-about-to-be-presented
// workflow recommendation as a pending decision (#2019/#2020), so that if
// the interactive session's inactivity timeout fires before anything (human
// or automation) answers, CompleteHTTPSession's hardcoded nil result
// (cmd/kubernautagent/routes.go) preserves this preview instead of silently
// finalizing as has_workflow:false. If the user does respond
// (select_workflow/complete_no_action), that explicit
// CompleteUserDriving(id, result) call overwrites this preview with the
// confirmed outcome, since Store.CompleteUserDriving only skips the
// overwrite when passed a nil result.
//
// No-op for the no_matching_workflows outcome -- that terminal case is
// already handled by autoCloseOnNoMatchingWorkflows, which proactively
// closes the session instead of leaving a decision pending.
func (t *InvestigateTool) persistPendingDecision(rrID string, workflowResult *katypes.InvestigationResult) {
	if workflowResult == nil || (workflowResult.HumanReviewNeeded && workflowResult.HumanReviewReason == "no_matching_workflows") {
		return
	}
	if t.httpCompleter == nil {
		return
	}
	httpSessionID, found := t.httpCompleter.FindUserDrivingByRemediationID(rrID)
	if !found {
		return
	}
	preview := *workflowResult
	preview.HumanReviewNeeded = true
	preview.HumanReviewReason = katypes.HumanReviewReasonDecisionExpired
	t.httpCompleter.PersistPendingDecisionResult(httpSessionID, &preview)
}

// authorizeActiveDriver verifies the requesting user is the active driver of
// the interactive session for rrID, returning ErrCodeNotDriving /
// ErrCodeSessionExpired / ErrCodeSessionActive as appropriate. Shared by the
// message/reconnect/discover_workflows actions, which all reject non-drivers
// identically.
func (t *InvestigateTool) authorizeActiveDriver(rrID string, user mcpinternal.UserInfo) (*mcpinternal.InteractiveSession, error) {
	if !t.sessions.IsDriverActive(rrID) {
		return nil, ErrCodeNotDriving
	}

	sess, err := t.sessions.GetDriver(rrID)
	if err != nil {
		if errors.Is(err, mcpinternal.ErrSessionExpired) {
			return nil, ErrCodeSessionExpired
		}
		return nil, ErrCodeNotDriving
	}
	if sess == nil {
		return nil, ErrCodeNotDriving
	}

	if sess.ActingUser.Username != user.Username {
		return nil, ErrCodeSessionActive.WithDetail("driver", sess.ActingUser.Username)
	}

	return sess, nil
}

// resolveRCAForDiscovery obtains the structured RCA result used for Phase 3
// workflow discovery.
//
// Preferred path: reuse the full InvestigationResult already produced by the
// autonomous Phase 1 RCA and stored in the session manager. This preserves
// the complete RemediationTarget (Kind, APIVersion, Name, Namespace) that
// the LLM emitted during the original investigation.
//
// Fallback: if no stored result exists (e.g. pure interactive session),
// reconstruct conversation from audit traces and re-extract RCA.
//
// Dead end (#2259): when neither a stored RCA nor any reconstructable
// conversation exists, self-heal via triggerFreshInvestigationForDiscovery
// instead of failing permanently -- see that function's doc comment for why.
func (t *InvestigateTool) resolveRCAForDiscovery(ctx context.Context, rrID, sessionID string, user mcpinternal.UserInfo) (*katypes.InvestigationResult, error) {
	if storedResult, ok := t.autoMgr.GetLatestRCAResultByRemediationID(rrID); ok && storedResult != nil {
		t.logger.Info("discover_workflows: using stored RCA result from autonomous investigation",
			"rr_id", rrID,
			"rca_target_kind", storedResult.RemediationTarget.Kind,
			"rca_target_api_version", storedResult.RemediationTarget.APIVersion,
			"rca_target_name", storedResult.RemediationTarget.Name)
		return storedResult, nil
	}

	messages := t.buildMessagesWithContext(rrID, "")
	if len(messages) > 0 && messages[len(messages)-1].Content == "" {
		messages = messages[:len(messages)-1]
	}
	if len(messages) == 0 {
		reconCount := t.storeReconstructedContext(ctx, rrID, sessionID)
		t.logger.Info("discover_workflows: reconHistory was empty, reconstructed from audit traces",
			"rr_id", rrID, "recon_turns", reconCount)
		messages = t.buildMessagesWithContext(rrID, "")
		if len(messages) > 0 && messages[len(messages)-1].Content == "" {
			messages = messages[:len(messages)-1]
		}
	}

	if len(messages) == 0 {
		t.logger.Info("discover_workflows: no conversation context available after reconstruction, self-healing via a fresh investigation (#2259)",
			"rr_id", rrID)
		return t.triggerFreshInvestigationForDiscovery(ctx, rrID, user)
	}

	rcaResult, err := t.runner.RunRCAExtraction(ctx, messages, rrID)
	if err != nil {
		return nil, fmt.Errorf("rca extraction failed: %w", err)
	}

	// Phase 2 extraction from conversation reconstructs a best-effort RCA,
	// but its RemediationTarget is unreliable: the conversation messages lack
	// the system prompt (with signal name/resource), so the LLM may fall back
	// to a generic target. Clear it so RunWorkflowDiscoveryFromRCA preserves
	// the signal resolver's authoritative identity instead of overwriting it
	// via SyncSignalFromRCA with the extraction's guess.
	rcaResult.RemediationTarget = katypes.RemediationTarget{}
	return rcaResult, nil
}

// triggerFreshInvestigationForDiscovery self-heals the #2259 dead end:
// resolveRCAForDiscovery reaches here only when neither a stored RCA result
// nor any reconstructable audit trail exists for rrID. The most likely
// cause is a takeover (handleTakeover) that raced ahead of the autonomous
// investigation's first LLM call -- TransitionToUserDriving cancels that
// goroutine synchronously (session/manager_interactive.go), so it exits
// before emitting a single aiagent.llm.request/response audit event,
// leaving nothing for DSContextReconstructor to ever reconstruct. Retrying
// discover_workflows against that state can never succeed on its own; it is
// a permanent dead end unless something runs a real investigation.
//
// This reuses createFreshInteractiveSession (investigate_autonomous.go), the
// same already-shipped, E2E-validated pipeline the action=start fallback
// chain uses for a genuinely-fresh RR: a real RunFullInvestigation call
// followed by UpgradeToInteractive. Reusing it means the fix requires zero
// changes to any of TransitionToUserDriving's cancellation call sites, and
// guarantees a fresh, correlation_id-keyed audit trail afterward (BR-AUDIT-005,
// SOC2 CC7.2) -- exactly what the cancelled-before-anything path lacked.
//
// Idempotency: the per-rrID session mutex held by handleDiscoverWorkflows
// (getSessionMutex) already serializes every discover_workflows/takeover
// call for the same rrID, so no other caller can race in here. The
// FindByRemediationID/FindPendingByRemediationID check below is a defensive
// second guard against launching a duplicate self-heal investigation when
// this function is entered more than once for the same rrID (e.g. a
// client-side retry that lands after the mutex was briefly released between
// calls): if one is already in flight, this call waits on it instead of
// starting another.
//
// Returns a distinguishable, retriable error (never the old "no conversation
// context available" wording, which implied a permanent dead end) when ctx
// is cancelled/inactivity-times-out before the investigation completes, or
// when the fresh investigation produced no result at all.
func (t *InvestigateTool) triggerFreshInvestigationForDiscovery(ctx context.Context, rrID string, user mcpinternal.UserInfo) (*katypes.InvestigationResult, error) {
	if _, running := t.autoMgr.FindByRemediationID(rrID); !running {
		if _, pending := t.autoMgr.FindPendingByRemediationID(rrID); !pending {
			t.createFreshInteractiveSession(ctx, rrID, user)
		}
	}

	select {
	case <-t.autoMgr.WaitForCompletionByRemediationID(rrID):
	case <-ctx.Done():
		return nil, fmt.Errorf("rca extraction failed: self-healing investigation still in progress, retry discover_workflows: %w", ctx.Err())
	}

	if result, ok := t.autoMgr.GetLatestRCAResultByRemediationID(rrID); ok && result != nil {
		t.logger.Info("discover_workflows: self-healed via a fresh investigation after a dead-context reconstruction (#2259)",
			"rr_id", rrID)
		return result, nil
	}

	return nil, fmt.Errorf("rca extraction failed: self-healing investigation produced no result, retry discover_workflows")
}

// resolveDiscoverySignal resolves the signal context for Phase 3 workflow
// discovery, logging (but not failing) on resolution errors.
func (t *InvestigateTool) resolveDiscoverySignal(ctx context.Context, rrID string) katypes.SignalContext {
	var signal katypes.SignalContext
	if t.signalResolver == nil {
		return signal
	}
	resolved, resolveErr := t.signalResolver.ResolveSignalContext(ctx, rrID)
	if resolveErr != nil {
		t.logger.V(1).Info("signal context resolution failed, using empty context",
			"rr_id", rrID, "error", resolveErr)
	} else if resolved != nil {
		signal = *resolved
	}
	return signal
}

// enrichLiveEventContext attaches the HTTP investigation session ID (and its
// lazy audit sink, when available) to ctx so the calling interactive action
// emits audit events with session_id and streams live KA events to any
// subscriber. Originally added for discover_workflows (#1384: Bug A fix);
// reused by handleMessage (#1639) so kubernaut_message turns get the same
// live-streaming wiring — action is used only for the debug log line
// (e.g. "discover_workflows", "message").
func (t *InvestigateTool) enrichLiveEventContext(ctx context.Context, rrID, action string) context.Context {
	if t.httpCompleter == nil {
		return ctx
	}
	httpSessionID, found := t.httpCompleter.FindUserDrivingByRemediationID(rrID)
	if !found {
		return ctx
	}
	ctx = session.WithSessionID(ctx, httpSessionID)
	if ls, ok := t.autoMgr.GetSessionLazySink(httpSessionID); ok {
		ctx = session.WithLazySink(ctx, ls)
	}
	t.logger.V(1).Info(action+": enriched context with HTTP session",
		"rr_id", rrID, "http_session_id", httpSessionID)
	return ctx
}

// populateDiscoveryTargets fills in the SignalTarget/SearchedTarget
// visibility fields on dr (#1437). SignalTarget is always the original alert
// resource. SearchedTarget is the resource actually searched against the
// catalog — sourced from the RCA-resolved remediation target, falling back
// to the signal target when empty (e.g. fallback RCA path).
func populateDiscoveryTargets(dr *mcpinternal.WorkflowDiscoveryResult, signal katypes.SignalContext, rt katypes.RemediationTarget) {
	signalTarget := &mcpinternal.DiscoveryTargetInfo{
		APIVersion: signal.ResourceAPIVersion,
		Kind:       signal.ResourceKind,
		Name:       signal.ResourceName,
		Namespace:  signal.Namespace,
	}
	dr.SignalTarget = signalTarget

	if rt.Kind == "" {
		dr.SearchedTarget = signalTarget
		return
	}
	dr.SearchedTarget = &mcpinternal.DiscoveryTargetInfo{
		APIVersion: rt.APIVersion,
		Kind:       rt.Kind,
		Name:       rt.Name,
		Namespace:  rt.Namespace,
	}
}

// extractDiscoveryResult builds a WorkflowDiscoveryResult from the Phase 3
// InvestigationResult, separating the selected workflow from alternatives.
func extractDiscoveryResult(result *katypes.InvestigationResult) *mcpinternal.WorkflowDiscoveryResult {
	if result == nil {
		return &mcpinternal.WorkflowDiscoveryResult{}
	}

	dr := &mcpinternal.WorkflowDiscoveryResult{
		FullResult: result,
	}

	if result.WorkflowID != "" {
		dr.Recommended = &mcpinternal.DiscoveredWorkflow{
			WorkflowID:      result.WorkflowID,
			ExecutionBundle: result.ExecutionBundle,
			Confidence:      result.Confidence,
			Rationale:       result.WorkflowRationale,
			Parameters:      cloneParameterMap(result.Parameters),
		}
	}

	if len(result.AlternativeWorkflows) > 0 {
		dr.Alternatives = make([]mcpinternal.DiscoveredWorkflow, 0, len(result.AlternativeWorkflows))
		for _, alt := range result.AlternativeWorkflows {
			dr.Alternatives = append(dr.Alternatives, mcpinternal.DiscoveredWorkflow{
				WorkflowID:      alt.WorkflowID,
				ExecutionBundle: alt.ExecutionBundle,
				Confidence:      alt.Confidence,
				Rationale:       alt.Rationale,
				Parameters:      cloneParameterMap(alt.Parameters),
			})
		}
	}

	return dr
}

// enrichDiscoveryNames resolves human-readable workflow names from the catalog
// for each discovered workflow. Lookup failures are logged but do not fail the
// operation (the workflow is still usable by ID).
func (t *InvestigateTool) enrichDiscoveryNames(ctx context.Context, dr *mcpinternal.WorkflowDiscoveryResult) {
	if dr == nil {
		return
	}
	if dr.Recommended != nil {
		dr.Recommended.Name = t.resolveWorkflowName(ctx, dr.Recommended.WorkflowID)
	}
	for i := range dr.Alternatives {
		dr.Alternatives[i].Name = t.resolveWorkflowName(ctx, dr.Alternatives[i].WorkflowID)
	}
}

func (t *InvestigateTool) resolveWorkflowName(ctx context.Context, workflowID string) string {
	if workflowID == "" {
		return ""
	}
	wf, err := t.catalog.GetWorkflowByID(ctx, workflowID)
	if err != nil {
		t.logger.V(1).Info("catalog lookup failed for workflow name enrichment",
			"workflow_id", workflowID, "error", err)
		return ""
	}
	return wf.WorkflowName
}
