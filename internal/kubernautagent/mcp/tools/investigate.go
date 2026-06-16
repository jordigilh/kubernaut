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

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
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
	// RunRCAExtraction appends a submit-RCA prompt to the conversation and
	// runs a single LLM call with submit_result as the only available tool.
	// Returns the parsed structured RCA as an InvestigationResult.
	RunRCAExtraction(ctx context.Context, messages []LLMMessage, correlationID string) (*katypes.InvestigationResult, error)
	// RunWorkflowDiscovery runs the autonomous Phase 3 pipeline (workflow
	// selection) using the structured RCA, signal context, and enrichment data.
	// Returns the full InvestigationResult including the selected workflow.
	RunWorkflowDiscovery(ctx context.Context, signal katypes.SignalContext, rcaResult *katypes.InvestigationResult, enrichData *prompt.EnrichmentData, correlationID string) (*katypes.InvestigationResult, error)
	// RunFullInvestigation executes the full autonomous RCA + workflow pipeline.
	// F4 (#1374): Used by handleStartAutonomous to wire real investigation logic.
	RunFullInvestigation(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error)
}

// SignalContextResolver resolves the SignalContext for a given remediation ID.
// Used by discover_workflows and handleMessage to obtain the signal parameters
// needed for Phase 3 workflow discovery and interactive message context.
//
// Enrichment is handled internally by the investigator's enrichment pipeline
// (F5 #1374), so this interface only resolves the signal context.
type SignalContextResolver interface {
	ResolveSignalContext(ctx context.Context, rrID string) (*katypes.SignalContext, error)
}

// HTTPSessionCompleter bridges MCP tool completion to the HTTP session store.
// select_workflow and complete_no_action use this to transition the session
// from StatusUserDriving to StatusCompleted with the final InvestigationResult.
//
// ForceCompleteByRemediationID is a fallback for when FindUserDrivingByRemediationID
// returns not-found: the autonomous session may still be running (submitted after
// MCP start) or already completed (investigation finished before takeover). It
// locates the session by remediation ID regardless of status and forces completion.
type HTTPSessionCompleter interface {
	FindUserDrivingByRemediationID(rrID string) (string, bool)
	CompleteUserDriving(id string, result *katypes.InvestigationResult) error
	ForceCompleteByRemediationID(rrID string, result *katypes.InvestigationResult) error
}

// SessionMutexProvider exposes per-rrID mutexes for concurrency control.
// Shared by InvestigateTool, SelectWorkflowTool, and CompleteNoActionTool
// to prevent races on InteractiveSession state (DiscoveryResult, RCAResult).
type SessionMutexProvider interface {
	GetSessionMutex(rrID string) *sync.Mutex
}

// AutonomousSessionManager provides lookup, suspension, and user-driving
// transition of autonomous investigation sessions. Used by handleTakeover
// to transition the running autonomous session to user-driven mode before
// acquiring the interactive Lease.
//
// v1.5: TransitionToUserDriving replaces SuspendInvestigation in the takeover
// path (BR-INTERACTIVE-004, #774). It cancels the autonomous goroutine, sets
// StatusUserDriving, and writes identity metadata so the poll response carries
// acting_user / acting_user_groups to AA for Rego policy evaluation.
type AutonomousSessionManager interface {
	FindByRemediationID(rrID string) (string, bool)
	CancelInvestigation(id string) error
	SuspendInvestigation(id string) error
	TransitionToUserDriving(id, username string, groups []string) error
	ForceTransitionToUserDriving(rrID, username string, groups []string) error
	// #1390: Upgrade a running autonomous session to interactive in-place.
	UpgradeToInteractive(id, username string, groups []string) error

	// BR-INTERACTIVE-010: Find pending interactive session by remediation ID.
	FindPendingByRemediationID(rrID string) (string, bool)
	// BR-INTERACTIVE-010: Launch a deferred investigation for a pending session.
	LaunchDeferredInvestigation(id string) error
	// BR-INTERACTIVE-010: Get RCA summary from latest completed session for context reconstruction.
	GetLatestRCASummaryByRemediationID(rrID string) (string, bool)
	// Get full RCA result from latest completed session for workflow discovery.
	GetLatestRCAResultByRemediationID(rrID string) (*katypes.InvestigationResult, bool)

	// BR-MCP-002: Start an autonomous investigation and return the session ID.
	StartInvestigation(ctx context.Context, fn session.InvestigateFunc, metadata map[string]string) (string, error)
	// BR-MCP-003: Subscribe to the event channel for a running investigation, activating the LazySink.
	Subscribe(ctx context.Context, id string) (<-chan session.InvestigationEvent, error)
	// #1384: Get the LazySink for a session so workflow_discovery can stream events.
	GetSessionLazySink(id string) (*session.LazySink, bool)
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

// NopAutonomousManager is a no-op implementation for tests that exercise
// actions unrelated to autonomous session management (start, message, etc.).
type NopAutonomousManager struct{}

func (NopAutonomousManager) FindByRemediationID(string) (string, bool)                  { return "", false }
func (NopAutonomousManager) CancelInvestigation(string) error                            { return nil }
func (NopAutonomousManager) SuspendInvestigation(string) error                           { return nil }
func (NopAutonomousManager) TransitionToUserDriving(string, string, []string) error      { return nil }
func (NopAutonomousManager) ForceTransitionToUserDriving(string, string, []string) error { return nil }
func (NopAutonomousManager) UpgradeToInteractive(string, string, []string) error         { return nil }
func (NopAutonomousManager) FindPendingByRemediationID(string) (string, bool)            { return "", false }
func (NopAutonomousManager) LaunchDeferredInvestigation(string) error                    { return nil }
func (NopAutonomousManager) GetLatestRCASummaryByRemediationID(string) (string, bool)    { return "", false }
func (NopAutonomousManager) GetLatestRCAResultByRemediationID(string) (*katypes.InvestigationResult, bool) {
	return nil, false
}
func (NopAutonomousManager) StartInvestigation(context.Context, session.InvestigateFunc, map[string]string) (string, error) {
	return "", nil
}
func (NopAutonomousManager) Subscribe(context.Context, string) (<-chan session.InvestigationEvent, error) {
	return nil, nil
}
func (NopAutonomousManager) GetSessionLazySink(string) (*session.LazySink, bool) { return nil, false }

// InvestigateTool handles the kubernaut_investigate MCP tool actions:
// start, message, complete, cancel, takeover, discover_workflows.
// BR-INTERACTIVE-001, BR-INTERACTIVE-004.
type InvestigateTool struct {
	sessions        mcpinternal.SessionManager
	runner          InvestigatorRunner
	recon           mcpinternal.ContextReconstructor
	autoMgr         AutonomousSessionManager
	httpCompleter   HTTPSessionCompleter
	signalResolver  SignalContextResolver
	rrChecker       RRExistenceChecker
	catalog         WorkflowCatalog
	metrics         ToolMetrics
	rateLimiter     MessageRateLimiter
	timeoutTracker  TimeoutTracker
	auditStore      audit.AuditStore
	logger          logr.Logger
	notifyFn        func(sessionID, msg string) // optional: delivers timeout warnings to client
	sessionMu       sync.Map                    // rrID -> *sync.Mutex (per-session serialization)
	reconHistory    sync.Map                    // rrID -> []LLMMessage (reconstructed context for LLM)
}

// InvestigateOption configures optional dependencies for InvestigateTool.
type InvestigateOption func(*InvestigateTool)

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

// WithHTTPCompleter sets the HTTP session completer for bridging MCP complete
// to the HTTP session store. Without this, action:complete releases the MCP
// lease but does not update the HTTP session that the AA controller polls.
func WithHTTPCompleter(completer HTTPSessionCompleter) InvestigateOption {
	return func(t *InvestigateTool) {
		if completer != nil {
			t.httpCompleter = completer
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

// WithSignalContextResolver sets the resolver for obtaining SignalContext and
// EnrichmentData for Phase 3 workflow discovery.
func WithSignalContextResolver(resolver SignalContextResolver) InvestigateOption {
	return func(t *InvestigateTool) {
		if resolver != nil {
			t.signalResolver = resolver
		}
	}
}

// WithAuditStore enables audit event emission for interactive session lifecycle
// events (BR-INTERACTIVE-003, DD-INTERACTIVE-002).
func WithAuditStore(store audit.AuditStore, logger logr.Logger) InvestigateOption {
	return func(t *InvestigateTool) {
		if store != nil {
			t.auditStore = store
			t.logger = logger
		}
	}
}

// WithWorkflowCatalog injects the workflow catalog for enriching discovered
// workflow names in handleDiscoverWorkflows. DS is a hard dependency — KA does
// not start without it — so this option must always be provided in production.
func WithWorkflowCatalog(catalog WorkflowCatalog) InvestigateOption {
	return func(t *InvestigateTool) {
		t.catalog = catalog
	}
}

// NewInvestigateTool creates the tool handler with its dependencies.
// autoMgr is required — KA always runs with an autonomous session manager.
// Passing nil will panic to surface wiring bugs immediately.
func NewInvestigateTool(sessions mcpinternal.SessionManager, runner InvestigatorRunner, recon mcpinternal.ContextReconstructor, autoMgr AutonomousSessionManager, opts ...InvestigateOption) *InvestigateTool {
	if autoMgr == nil {
		panic("NewInvestigateTool: autoMgr must not be nil — KA requires an autonomous session manager")
	}
	t := &InvestigateTool{
		sessions: sessions,
		runner:   runner,
		recon:    recon,
		autoMgr:  autoMgr,
		logger:   logr.Discard(),
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// SubscribeEvents activates the LazySink for the given investigation session
// and returns the event channel. Used by registration.go to wire EventLogBridge
// for live event streaming via MCP LoggingMessage (BR-MCP-003).
func (t *InvestigateTool) SubscribeEvents(ctx context.Context, sessionID string) (<-chan session.InvestigationEvent, error) {
	return t.autoMgr.Subscribe(ctx, sessionID)
}

// getSessionMutex returns a per-rrID mutex for serializing concurrent requests.
func (t *InvestigateTool) getSessionMutex(rrID string) *sync.Mutex {
	val, _ := t.sessionMu.LoadOrStore(rrID, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// GetSessionMutex implements SessionMutexProvider, exposing the per-rrID mutex
// for use by SelectWorkflowTool and CompleteNoActionTool.
func (t *InvestigateTool) GetSessionMutex(rrID string) *sync.Mutex {
	return t.getSessionMutex(rrID)
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
		return t.handleStatus(input, user)
	case ActionReconnect:
		return t.handleReconnect(input, user)
	case ActionDiscoverWorkflows:
		return t.handleDiscoverWorkflows(ctx, input, user)
	case ActionStartAutonomous:
		return t.handleStartAutonomous(ctx, input, user)
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

	// BR-INTERACTIVE-010: Check for pending interactive session and launch it.
	// When launched, the investigation will self-transition to StatusUserDriving
	// via InteractiveHold — skip TransitionToUserDriving below to avoid cancelling
	// the RCA goroutine prematurely.
	var launchedPending bool
	var investigationSessionID string
	if pendingID, hasPending := t.autoMgr.FindPendingByRemediationID(input.RRID); hasPending {
		if launchErr := t.autoMgr.LaunchDeferredInvestigation(pendingID); launchErr != nil {
			t.logger.Error(launchErr, "start: failed to launch deferred investigation",
				"rr_id", input.RRID, "pending_session_id", pendingID)
		} else {
			launchedPending = true
			investigationSessionID = pendingID
		}
	}

	sess, err := t.sessions.Takeover(ctx, input.RRID, user)
	if err != nil {
		if errors.Is(err, mcpinternal.ErrLeaseHeld) {
			if t.metrics != nil {
				t.metrics.RecordInteractiveLeaseContention()
				t.metrics.RecordInteractiveTakeover("start_failed")
			}
			driver, _ := t.sessions.GetDriver(input.RRID)
			driverName := "unknown"
			if driver != nil {
				driverName = driver.ActingUser.Username
			}
			return InvestigateOutput{}, ErrCodeSessionActive.WithDetail("driver", driverName)
		}
		if errors.Is(err, mcpinternal.ErrMaxSessionsReached) {
			if t.metrics != nil {
				t.metrics.RecordInteractiveTakeover("start_failed")
			}
			return InvestigateOutput{}, &MCPError{Code: "max_sessions", Message: "Maximum concurrent sessions reached"}
		}
		if t.metrics != nil {
			t.metrics.RecordInteractiveTakeover("start_failed")
		}
		return InvestigateOutput{}, fmt.Errorf("start session: %w", err)
	}

	if sess.Reconnected {
		return InvestigateOutput{}, &MCPError{
			Code:    "session_active",
			Message: "You already have an active session for this investigation; use action=reconnect to rejoin",
			Details: map[string]string{
				"driver":     user.Username,
				"session_id": sess.SessionID,
			},
		}
	}

	// #1390: Upgrade running autonomous session in-place (Jump In) instead of
	// cancelling and recreating. UpgradeToInteractive sets the atomic flag so
	// the goroutine's next InteractiveHold check sees it, and store.Update's
	// deterministic check catches completion that already happened.
	// Skip when we just launched a pending session — its RCA goroutine will
	// self-transition via InteractiveHold once complete.
	if !launchedPending {
		autoSessionID, found := t.autoMgr.FindByRemediationID(input.RRID)
		if found {
			if upgradeErr := t.autoMgr.UpgradeToInteractive(autoSessionID, user.Username, user.Groups); upgradeErr != nil {
				if errors.Is(upgradeErr, session.ErrSessionTerminal) {
					if forceErr := t.autoMgr.ForceTransitionToUserDriving(input.RRID, user.Username, user.Groups); forceErr != nil {
						t.logger.Error(forceErr, "start: force-transition to user-driving failed (session terminal)",
							"rr_id", input.RRID, "auto_session_id", autoSessionID)
					}
				} else {
					t.logger.Error(upgradeErr, "start: upgrade autonomous session to interactive",
						"rr_id", input.RRID, "auto_session_id", autoSessionID)
				}
			}
			investigationSessionID = autoSessionID
		} else {
			if forceErr := t.autoMgr.ForceTransitionToUserDriving(input.RRID, user.Username, user.Groups); forceErr != nil {
				t.logger.Error(forceErr, "start: force-transition to user-driving (no running session found)",
					"rr_id", input.RRID)
			}
		}
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

func (t *InvestigateTool) handleTakeover(ctx context.Context, input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	mu := t.getSessionMutex(input.RRID)
	mu.Lock()
	defer mu.Unlock()

	// H4: Acquire the interactive Lease BEFORE suspending autonomous. This ensures
	// that if Takeover fails (lease contention, max sessions), the autonomous
	// investigation is NOT irreversibly cancelled.
	sess, err := t.sessions.Takeover(ctx, input.RRID, user)
	if err != nil {
		if errors.Is(err, mcpinternal.ErrLeaseHeld) {
			if t.metrics != nil {
				t.metrics.RecordInteractiveLeaseContention()
				t.metrics.RecordInteractiveTakeover("takeover_race_lost")
			}
			driver, _ := t.sessions.GetDriver(input.RRID)
			driverName := "unknown"
			if driver != nil {
				driverName = driver.ActingUser.Username
			}
			return InvestigateOutput{}, ErrCodeSessionActive.WithDetail("driver", driverName)
		}
		if errors.Is(err, mcpinternal.ErrMaxSessionsReached) {
			if t.metrics != nil {
				t.metrics.RecordInteractiveTakeover("takeover_failed")
			}
			return InvestigateOutput{}, &MCPError{Code: "max_sessions", Message: "Maximum concurrent sessions reached"}
		}
		if t.metrics != nil {
			t.metrics.RecordInteractiveTakeover("takeover_failed")
		}
		return InvestigateOutput{}, fmt.Errorf("takeover session: %w", err)
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
	// #774: TransitionToUserDriving replaces SuspendInvestigation so that:
	// 1. The session enters StatusUserDriving (pollable, not terminal)
	// 2. Identity (username + groups) is written to session metadata
	// 3. AA poll response carries identity → Rego input.identity
	autoSessionID, found := t.autoMgr.FindByRemediationID(input.RRID)
	if found {
		if err := t.autoMgr.TransitionToUserDriving(autoSessionID, user.Username, user.Groups); err != nil {
			if errors.Is(err, session.ErrSessionTerminal) {
				if forceErr := t.autoMgr.ForceTransitionToUserDriving(input.RRID, user.Username, user.Groups); forceErr != nil {
					t.logger.Error(forceErr, "takeover: force-transition to user-driving failed",
						"rr_id", input.RRID, "auto_session_id", autoSessionID)
				}
			} else {
				if t.metrics != nil {
					t.metrics.RecordInteractiveTakeover("takeover_failed")
				}
				return InvestigateOutput{}, fmt.Errorf("transition autonomous session to user-driving: %w", err)
			}
		}
	} else {
		// No running session found by RR ID. The AA session submit may still
		// be in-flight (race between MCP takeover and AA reconcile). Retry
		// briefly to allow the session to appear before giving up.
		var forceErr error
		for attempt := 0; attempt < 5; attempt++ {
			forceErr = t.autoMgr.ForceTransitionToUserDriving(input.RRID, user.Username, user.Groups)
			if forceErr == nil || !errors.Is(forceErr, session.ErrSessionNotFound) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if forceErr != nil {
			t.logger.Error(forceErr, "takeover: force-transition to user-driving failed after retries",
				"rr_id", input.RRID)
		}
	}

	if t.metrics != nil {
		t.metrics.RecordInteractiveSessionStarted()
		t.metrics.RecordInteractiveTakeover("takeover_success")
	}

	t.emitInteractiveStarted(sess.SessionID, input.RRID, user.Username)
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

	// #898-S5: Attach session ID for audit attribution on K8s API calls.
	ctx = transport.WithAuditSessionID(ctx, sess.SessionID)

	// F9 / #1374: Attach signal context for PhaseRCA tool parity with
	// the autonomous path. Future tools may read SignalContextFromContext.
	if t.signalResolver != nil {
		if resolved, resolveErr := t.signalResolver.ResolveSignalContext(ctx, input.RRID); resolveErr == nil && resolved != nil {
			ctx = katypes.WithSignalContext(ctx, *resolved)
		}
	}

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

	t.sessions.TouchActivity(input.RRID)
	if t.timeoutTracker != nil {
		t.timeoutTracker.ResetInactivity(sess.SessionID)
	}

	return InvestigateOutput{
		SessionID: sess.SessionID,
		Status:    "reconnected",
	}, nil
}

func (t *InvestigateTool) handleDiscoverWorkflows(ctx context.Context, input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
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

	if t.catalog == nil {
		return InvestigateOutput{}, fmt.Errorf("workflow catalog not configured: cannot enrich discovery names")
	}

	// Reset inactivity timer before the (potentially long) LLM calls.
	t.sessions.TouchActivity(input.RRID)
	if t.timeoutTracker != nil {
		t.timeoutTracker.ResetInactivity(sess.SessionID)
	}

	// Step 1: Obtain the structured RCA result for Phase 3 workflow discovery.
	//
	// Preferred path: reuse the full InvestigationResult already produced by the
	// autonomous Phase 1 RCA and stored in the session manager. This preserves
	// the complete RemediationTarget (Kind, APIVersion, Name, Namespace) that
	// the LLM emitted during the original investigation.
	//
	// Fallback: if no stored result exists (e.g. pure interactive session),
	// reconstruct conversation from audit traces and re-extract RCA.
	var rcaResult *katypes.InvestigationResult
	if storedResult, ok := t.autoMgr.GetLatestRCAResultByRemediationID(input.RRID); ok && storedResult != nil {
		rcaResult = storedResult
		t.logger.Info("discover_workflows: using stored RCA result from autonomous investigation",
			"rr_id", input.RRID,
			"rca_target_kind", storedResult.RemediationTarget.Kind,
			"rca_target_api_version", storedResult.RemediationTarget.APIVersion,
			"rca_target_name", storedResult.RemediationTarget.Name)
	} else {
		messages := t.buildMessagesWithContext(input.RRID, "")
		if len(messages) > 0 && messages[len(messages)-1].Content == "" {
			messages = messages[:len(messages)-1]
		}
		if len(messages) == 0 {
			reconCount := t.storeReconstructedContext(ctx, input.RRID, sess.SessionID)
			t.logger.Info("discover_workflows: reconHistory was empty, reconstructed from audit traces",
				"rr_id", input.RRID, "recon_turns", reconCount)
			messages = t.buildMessagesWithContext(input.RRID, "")
			if len(messages) > 0 && messages[len(messages)-1].Content == "" {
				messages = messages[:len(messages)-1]
			}
		}

		if len(messages) == 0 {
			t.logger.Info("discover_workflows: no conversation context available after reconstruction",
				"rr_id", input.RRID)
			return InvestigateOutput{}, fmt.Errorf("rca extraction failed: no conversation context available — investigation audit traces not found in data storage")
		}

		var err error
		rcaResult, err = t.runner.RunRCAExtraction(ctx, messages, input.RRID)
		if err != nil {
			return InvestigateOutput{}, fmt.Errorf("rca extraction failed: %w", err)
		}

		// Phase 2 extraction from conversation reconstructs a best-effort RCA,
		// but its RemediationTarget is unreliable: the conversation messages lack
		// the system prompt (with signal name/resource), so the LLM may fall back
		// to a generic target. Clear it so RunWorkflowDiscoveryFromRCA preserves
		// the signal resolver's authoritative identity instead of overwriting it
		// via SyncSignalFromRCA with the extraction's guess.
		rcaResult.RemediationTarget = katypes.RemediationTarget{}
	}

	// Step 2: Resolve signal context for Phase 3.
	// Enrichment is handled internally by the investigator's enrichment pipeline
	// (F5 #1374), so we only resolve the signal here.
	var signal katypes.SignalContext
	if t.signalResolver != nil {
		resolved, resolveErr := t.signalResolver.ResolveSignalContext(ctx, input.RRID)
		if resolveErr != nil {
			t.logger.V(1).Info("signal context resolution failed, using empty context",
				"rr_id", input.RRID, "error", resolveErr)
		} else if resolved != nil {
			signal = *resolved
		}
	}

	// Step 3: Enrich context with the HTTP investigation session so that
	// workflow discovery can emit audit events with session_id and stream
	// events to the subscriber (#1384: Bug A fix).
	if t.httpCompleter != nil {
		if httpSessionID, found := t.httpCompleter.FindUserDrivingByRemediationID(input.RRID); found {
			ctx = session.WithSessionID(ctx, httpSessionID)
			if ls, ok := t.autoMgr.GetSessionLazySink(httpSessionID); ok {
				ctx = session.WithLazySink(ctx, ls)
			}
			t.logger.V(1).Info("discover_workflows: enriched context with HTTP session",
				"rr_id", input.RRID, "http_session_id", httpSessionID)
		}
	}

	// Step 4: Run Phase 3 workflow discovery using the structured RCA.
	// The investigator resolves enrichment internally when its enricher is wired,
	// falling back to nil when not available.
	workflowResult, err := t.runner.RunWorkflowDiscovery(ctx, signal, rcaResult, nil, input.RRID)
	if err != nil {
		return InvestigateOutput{}, fmt.Errorf("workflow discovery failed: %w", err)
	}

	// Step 5: Store results on the interactive session.
	sess.RCAResult = rcaResult
	sess.DiscoveryResult = extractDiscoveryResult(workflowResult)

	// Step 6: Populate discovery target visibility fields (#1437).
	// SignalTarget is always the original alert resource (captured before RunWorkflowDiscovery).
	// SearchedTarget is the resource actually searched against the catalog — sourced from
	// workflowResult.RemediationTarget (set by SyncSignalFromRCA inside the investigator).
	// Falls back to signal when RemediationTarget is empty (e.g., fallback RCA path).
	signalTarget := &mcpinternal.DiscoveryTargetInfo{
		APIVersion: signal.ResourceAPIVersion,
		Kind:       signal.ResourceKind,
		Name:       signal.ResourceName,
		Namespace:  signal.Namespace,
	}
	sess.DiscoveryResult.SignalTarget = signalTarget

	rt := workflowResult.RemediationTarget
	if rt.Kind != "" {
		sess.DiscoveryResult.SearchedTarget = &mcpinternal.DiscoveryTargetInfo{
			APIVersion: rt.APIVersion,
			Kind:       rt.Kind,
			Name:       rt.Name,
			Namespace:  rt.Namespace,
		}
	} else {
		sess.DiscoveryResult.SearchedTarget = signalTarget
	}

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
