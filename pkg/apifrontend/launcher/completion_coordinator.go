package launcher

import (
	"context"
	"encoding/json"
	"fmt"

	adksession "google.golang.org/adk/v2/session"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// completeBusinessOutcome enforces business-output obligations after an ADK
// turn and before A2A finalization. It is deliberately separate from generic
// reinvocation so presentation recovery cannot bypass consent checkpoints.
func (r *reinvokingRunner) completeBusinessOutcome(ctx context.Context, userID, sessionID string) error {
	resp, err := r.sessionService.Get(ctx, &adksession.GetRequest{
		AppName:   r.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		r.logger.Error(err, "failed to load session for business-outcome completion", "session_id", sessionID)
		return fmt.Errorf("load session for business-outcome completion: %w", err)
	}
	if resp == nil || resp.Session == nil {
		return nil
	}

	state := resp.Session.State()
	statusValue, _ := state.Get(session.StateKeyDecisionArtifactStatus)
	status, _ := statusValue.(string)
	if status != session.DecisionArtifactRequired {
		return nil
	}
	return r.recoverBusinessOutcome(ctx, resp.Session, state, sessionID)
}

func (r *reinvokingRunner) recoverBusinessOutcome(ctx context.Context, sess adksession.Session, state adksession.State, sessionID string) error {
	if hasDecisionArtifact(sess.Events()) {
		return r.persistCompletionStatus(ctx, sess, session.DecisionArtifactEmitted)
	}

	rca := stateMap(state, session.StateKeyGroundedRCAPayload)
	groundedSummary, _ := state.Get(session.StateKeyGroundedSummary)
	summary, _ := groundedSummary.(string)
	provisionalValue, _ := state.Get(session.StateKeyGroundedSummaryProvisional)
	provisional, _ := provisionalValue.(bool)
	discovery := discoveryFromState(state)
	if discovery == nil {
		discovery = discoveryFromEvents(sess.Events())
	}

	activeSessionID := sessionID
	if value, ok := state.Get(session.StateKeyActiveSession); ok == nil {
		if stored, ok := value.(string); ok && stored != "" {
			activeSessionID = stored
		}
	}
	rrID, _ := state.Get(session.StateKeyActiveRRID)
	rr, _ := rrID.(string)

	artifact, buildErr := BuildRecoveredDecisionArtifact(DecisionRecoveryInput{
		SessionID:          activeSessionID,
		RRID:               rr,
		RCA:                rca,
		Summary:            summary,
		SummaryProvisional: provisional,
		Discovery:          discovery,
	})
	if buildErr != nil {
		r.logger.Error(buildErr, "failed to build recovered decision artifact", "session_id", sessionID)
		artifact, _ = BuildRecoveredDecisionArtifact(DecisionRecoveryInput{
			SessionID: activeSessionID,
			RRID:      rr,
		})
	}

	if err := EmitArtifactSafe(ctx, artifact.Data, artifact.TextFallback, artifact.Metadata); err != nil {
		return fmt.Errorf("emit recovered decision artifact: %w", err)
	}

	status := session.DecisionArtifactRecovered
	if !artifact.Complete {
		status = session.DecisionArtifactFailed
		if phase3Blocked(state) {
			return r.persistCompletionStatus(ctx, sess, status)
		}
		if err := r.escalateIncompleteDecision(ctx, rr, sessionID); err != nil {
			return err
		}
	}

	return r.persistCompletionStatus(ctx, sess, status)
}

func (r *reinvokingRunner) escalateIncompleteDecision(ctx context.Context, rrID, sessionID string) error {
	if r.presentationRecoveryTerminalizer == nil {
		err := fmt.Errorf("presentation recovery requires operator escalation but no terminalizer is configured")
		r.logger.Error(err, "business-outcome completion cannot terminate incomplete decision", "session_id", sessionID)
		return err
	}
	if rrID == "" {
		err := fmt.Errorf("presentation recovery requires operator escalation but active rr_id is missing")
		r.logger.Error(err, "business-outcome completion cannot terminate incomplete decision", "session_id", sessionID)
		return err
	}
	if err := r.presentationRecoveryTerminalizer(ctx, rrID); err != nil {
		return fmt.Errorf("escalate incomplete decision for rr %q: %w", rrID, err)
	}
	return nil
}

func (r *reinvokingRunner) persistCompletionStatus(ctx context.Context, sess adksession.Session, status string) error {
	event := adksession.NewEvent(ctx, "business-outcome-completion")
	event.Actions.StateDelta = map[string]any{session.StateKeyDecisionArtifactStatus: status}
	if err := r.sessionService.AppendEvent(ctx, sess, event); err != nil {
		return fmt.Errorf("persist decision artifact status %q: %w", status, err)
	}
	return nil
}

func hasDecisionArtifact(events adksession.Events) bool {
	for event := range events.All() {
		if event == nil || event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part != nil && part.FunctionCall != nil && decisionMetaTools[part.FunctionCall.Name] {
				return true
			}
		}
	}
	return false
}

func discoveryFromState(state adksession.State) *ka.DiscoverWorkflowsResult {
	value, err := state.Get(session.StateKeyDiscoveryResult)
	if err != nil || value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	result, err := ka.ParseDiscoverWorkflowsResponse(raw)
	if err != nil {
		return nil
	}
	return result
}

func discoveryFromEvents(events adksession.Events) *ka.DiscoverWorkflowsResult {
	var found *ka.DiscoverWorkflowsResult
	for event := range events.All() {
		if event == nil || event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part == nil || part.FunctionResponse == nil || part.FunctionResponse.Name != "kubernaut_discover_workflows" {
				continue
			}
			raw, err := json.Marshal(part.FunctionResponse.Response)
			if err != nil {
				continue
			}
			parsed, err := ka.ParseDiscoverWorkflowsResponse(raw)
			if err == nil {
				found = parsed
			}
		}
	}
	return found
}

func stateMap(state adksession.State, key string) map[string]any {
	value, err := state.Get(key)
	if err != nil || value == nil {
		return nil
	}
	result, _ := value.(map[string]any)
	return result
}

func phase3Blocked(state adksession.State) bool {
	value, err := state.Get(session.StateKeyPhase3Blocked)
	blocked, ok := value.(bool)
	return err == nil && ok && blocked
}
