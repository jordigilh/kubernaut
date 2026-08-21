package tools

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/watch"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"

	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

// ========================================
// kubernaut_await_session: Wait for KA investigation session readiness
// BR-INTERACTIVE-010: AF waits for AA to submit to KA before connecting
// ========================================

// AwaitSessionArgs defines the input for kubernaut_await_session.
type AwaitSessionArgs struct {
	Namespace string `json:"-"`
	RRName    string `json:"rr_name"`
}

// AwaitSessionResult is the output of kubernaut_await_session.
type AwaitSessionResult struct {
	SessionID string `json:"session_id,omitempty"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

// AwaitSessionTimeout is the maximum duration HandleAwaitSession waits for an
// AgentSession CRD with a session ID. In production the AA controller may take
// minutes to process an RR; in E2E tests this can be shortened.
// Exported so that tests can override it without modifying production code.
var AwaitSessionTimeout = 3 * time.Minute

const awaitSessionPollInterval = 3 * time.Second

// HandleAwaitSession waits for an AgentSession resource (matching the given RR) to
// have a non-empty status.sessionID -- KA's own internal investigation session
// identifier (api/agentsession/v1alpha1: Status.SessionID), NOT
// AIAnalysis.Status.KASession.ID.
//
// #2170/DD-AA-KA-001 correction (CI evidence: run 32215236666, E2E-FLEET-018):
// this previously watched AIAnalysis and read Status.KASession.ID, which
// DD-AA-KA-001 repurposed to hold the AgentSession's own deterministic object
// name (see pkg/aianalysis/handlers/investigating.go's syncKASessionStatus
// doc comment), not KA's real session ID -- a holdover from the pre-redesign
// HTTP architecture where that field genuinely was KA's session ID. Passing
// the AgentSession's object name back to KA as MCP action=start's
// session_id caused LaunchDeferredInvestigation to fail with "session not
// found" (real ID mismatch, not the benign ErrSessionNotPending race),
// orphaning the correctly fleet-scoped pending session and falling through
// to a duplicate, generically-enriched investigation. AgentSession's own
// Status.SessionID is written by the dispatcher (writeDispatchedStatus,
// internal/kubernautagent/agentsession/status_writer.go) as soon as the
// pending session is created -- available well before interactive takeover.
// Returns the session ID when ready, or times out after AwaitSessionTimeout.
func HandleAwaitSession(ctx context.Context, client crclient.Client, args AwaitSessionArgs) (AwaitSessionResult, error) {
	if client == nil {
		return AwaitSessionResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return AwaitSessionResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if args.RRName == "" {
		return AwaitSessionResult{}, fmt.Errorf("%w: rr_name is required", ErrInvalidInput)
	}

	if sessionID := findSessionIDByList(ctx, client, args); sessionID != "" {
		return AwaitSessionResult{SessionID: sessionID, Status: "ready"}, nil
	}

	watchCtx, cancel := context.WithTimeout(ctx, AwaitSessionTimeout)
	defer cancel()

	wc, ok := client.(crclient.WithWatch)
	if !ok {
		return pollForSessionID(watchCtx, client, args)
	}

	var asList agentsessionv1.AgentSessionList
	watcher, err := wc.Watch(watchCtx, &asList, crclient.InNamespace(args.Namespace))
	if err != nil {
		return pollForSessionID(watchCtx, client, args)
	}
	defer watcher.Stop()

	return watchForSessionID(watchCtx, watcher, args.RRName)
}

// watchForSessionID drains watcher's event channel until an AgentSession event
// matching rrName carries a non-empty Status.SessionID, the watch closes, or
// watchCtx is done (timeout).
//
//nolint:unparam // error is always nil here; signature matches pollForSessionID's (AwaitSessionResult, error), the interchangeable sibling branch at the shared call site (Issue #1546 Tier 4)
func watchForSessionID(watchCtx context.Context, watcher watch.Interface, rrName string) (AwaitSessionResult, error) {
	for {
		select {
		case <-watchCtx.Done():
			return AwaitSessionResult{Status: "timeout", Message: "KA session not ready within timeout"}, nil
		case evt, ok := <-watcher.ResultChan():
			if !ok {
				return AwaitSessionResult{Status: "timeout", Message: "watch closed unexpectedly"}, nil
			}
			if sessionID, matched := sessionIDFromEvent(evt, rrName); matched {
				return AwaitSessionResult{SessionID: sessionID, Status: "ready"}, nil
			}
		}
	}
}

// sessionIDFromEvent extracts KA's real session ID from a watch event, if it
// is an Added/Modified event for the AgentSession matching rrName with a
// non-empty Status.SessionID already set. matched is true only in that case.
func sessionIDFromEvent(evt watch.Event, rrName string) (sessionID string, matched bool) {
	if evt.Type != watch.Modified && evt.Type != watch.Added {
		return "", false
	}
	as, ok := evt.Object.(*agentsessionv1.AgentSession)
	if !ok || as.Spec.RemediationRequestRef.Name != rrName {
		return "", false
	}
	if as.Status.SessionID == "" {
		return "", false
	}
	return as.Status.SessionID, true
}

// pollForSessionID is a fallback that polls AgentSession resources until session ID appears.
func pollForSessionID(ctx context.Context, client crclient.Client, args AwaitSessionArgs) (AwaitSessionResult, error) {
	ticker := time.NewTicker(awaitSessionPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return AwaitSessionResult{Status: "timeout", Message: "KA session not ready within timeout"}, nil
		case <-ticker.C:
			if sessionID := findSessionIDByList(ctx, client, args); sessionID != "" {
				return AwaitSessionResult{SessionID: sessionID, Status: "ready"}, nil
			}
		}
	}
}

// findSessionIDByList lists AgentSessions for the given RR and returns the first non-empty session ID.
func findSessionIDByList(ctx context.Context, client crclient.Client, args AwaitSessionArgs) string {
	var list agentsessionv1.AgentSessionList
	if err := client.List(ctx, &list, crclient.InNamespace(args.Namespace)); err != nil {
		return ""
	}
	for i := range list.Items {
		item := &list.Items[i]
		if item.Spec.RemediationRequestRef.Name != args.RRName {
			continue
		}
		if item.Status.SessionID != "" {
			return item.Status.SessionID
		}
	}
	return ""
}

// NewAwaitSessionTool creates the kubernaut_await_session tool.
func NewAwaitSessionTool(client crclient.Client, controllerNS string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_await_session",
		Description: "Wait for the AI investigation session to become ready for a given remediation request. Returns the KA session ID when available.",
	}, func(ctx agent.Context, args AwaitSessionArgs) (AwaitSessionResult, error) {
		args.Namespace = controllerNS
		return HandleAwaitSession(ctx, client, args)
	})
}

// ========================================
// AwaitAgentSessionInteractive: watch AgentSession.Status.Interactive
// DD-AA-KA-001 Amendment Gap 1 / Issue #2172: AF watches KA's own
// authoritative ack directly instead of polling the retired
// IS.Status.Phase=Active signal AA used to write (that write was itself
// removed from AA in #2170). #2214 Amendment: AA's remaining write-only
// InvestigationSession terminal-close path was retired too -- AF's own
// AgentSessionTerminalCloseReconciler (watching AgentSession) now owns IS
// terminal-phase closure exclusively; AA no longer interacts with IS at all.
// ========================================

// agentSessionInteractivePollInterval is the poll-fallback interval, used
// only when client does not implement crclient.WithWatch.
const agentSessionInteractivePollInterval = 500 * time.Millisecond

// AwaitAgentSessionInteractive waits for the AgentSession matching rrName
// (in namespace) to have Status.Interactive == true -- KA's own
// authoritative record that a human driver has taken over, written
// atomically with the interactive-driver Lease actually being held
// (BR-AA-KA-065.5). Returns (true, nil) once observed, (false, nil) on
// timeout (the ctx deadline elapses first -- not treated as an error, since
// the caller proceeds autonomously in that case), or (false, err) for
// invalid input / an unavailable client. Mirrors HandleAwaitSession's
// watch-first, poll-fallback shape: the AgentSession may not exist yet at
// fresh-start time, so both branches list first before waiting.
func AwaitAgentSessionInteractive(ctx context.Context, client crclient.Client, namespace, rrName string) (bool, error) {
	if client == nil {
		return false, ErrK8sUnavailable
	}
	if err := validate.Namespace(namespace); err != nil {
		return false, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if rrName == "" {
		return false, fmt.Errorf("%w: rr_name is required", ErrInvalidInput)
	}

	if agentSessionInteractivePresent(ctx, client, namespace, rrName) {
		return true, nil
	}

	watchCtx, cancel := context.WithTimeout(ctx, agentSessionInteractiveTimeout(ctx))
	defer cancel()

	wc, ok := client.(crclient.WithWatch)
	if !ok {
		return pollForAgentSessionInteractive(watchCtx, client, namespace, rrName)
	}

	var asList agentsessionv1.AgentSessionList
	watcher, err := wc.Watch(watchCtx, &asList, crclient.InNamespace(namespace))
	if err != nil {
		return pollForAgentSessionInteractive(watchCtx, client, namespace, rrName)
	}
	defer watcher.Stop()

	return watchForAgentSessionInteractive(watchCtx, watcher, rrName)
}

// agentSessionInteractiveTimeout caps the wait, respecting ctx's own
// deadline if it is shorter than isPhaseDefaultTimeout (the same outer
// bound the retired poll loop used; callers already apply their own
// tighter timeouts via context.WithTimeout, e.g. isPhaseActivePollTimeout/
// takeoverISPhaseTimeout in ka_investigate_mcp.go).
func agentSessionInteractiveTimeout(ctx context.Context) time.Duration {
	timeout := isPhaseDefaultTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	return timeout
}

// watchForAgentSessionInteractive drains watcher's event channel until an
// AgentSession event matching rrName carries Status.Interactive == true,
// the watch closes, or watchCtx is done (timeout).
//
//nolint:unparam // error is always nil here; signature matches pollForAgentSessionInteractive's (bool, error), the interchangeable sibling branch at the shared call site (same precedent as watchForSessionID above, Issue #1546 Tier 4)
func watchForAgentSessionInteractive(watchCtx context.Context, watcher watch.Interface, rrName string) (bool, error) {
	for {
		select {
		case <-watchCtx.Done():
			return false, nil
		case evt, ok := <-watcher.ResultChan():
			if !ok {
				return false, nil
			}
			if agentSessionInteractiveFromEvent(evt, rrName) {
				return true, nil
			}
		}
	}
}

// agentSessionInteractiveFromEvent reports whether evt is an Added/Modified
// event for the AgentSession matching rrName with Status.Interactive true.
func agentSessionInteractiveFromEvent(evt watch.Event, rrName string) bool {
	if evt.Type != watch.Modified && evt.Type != watch.Added {
		return false
	}
	as, ok := evt.Object.(*agentsessionv1.AgentSession)
	if !ok || as.Spec.RemediationRequestRef.Name != rrName {
		return false
	}
	return as.Status.Interactive
}

// pollForAgentSessionInteractive is a fallback that polls AgentSession
// resources until Status.Interactive appears, for clients that do not
// implement crclient.WithWatch.
func pollForAgentSessionInteractive(ctx context.Context, client crclient.Client, namespace, rrName string) (bool, error) {
	ticker := time.NewTicker(agentSessionInteractivePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false, nil
		case <-ticker.C:
			if agentSessionInteractivePresent(ctx, client, namespace, rrName) {
				return true, nil
			}
		}
	}
}

// agentSessionInteractivePresent lists AgentSessions in the namespace and
// returns true if any matching rrName has Status.Interactive == true.
func agentSessionInteractivePresent(ctx context.Context, client crclient.Client, namespace, rrName string) bool {
	var list agentsessionv1.AgentSessionList
	if err := client.List(ctx, &list, crclient.InNamespace(namespace)); err != nil {
		return false
	}
	for i := range list.Items {
		item := &list.Items[i]
		if item.Spec.RemediationRequestRef.Name != rrName {
			continue
		}
		if item.Status.Interactive {
			return true
		}
	}
	return false
}

// isPhaseDefaultTimeout is the outer wait bound for
// AwaitAgentSessionInteractive, respected unless ctx's own deadline is
// shorter (the retired AwaitISPhaseActive poll loop used this same 30s
// default; kept as-is since callers already apply their own tighter
// timeouts, e.g. isPhaseActivePollTimeout/takeoverISPhaseTimeout in
// ka_investigate_mcp.go).
const isPhaseDefaultTimeout = 30 * time.Second
