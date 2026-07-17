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
	"sync"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
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

// AutonomousSessionQuerier provides read-only lookups of autonomous
// investigation session state. Split out from AutonomousSessionManager for
// ISP (GO-ANTIPATTERN-AUDIT-2026-07-01 Phase 5) — none of these methods
// mutate session state.
type AutonomousSessionQuerier interface {
	FindByRemediationID(rrID string) (string, bool)
	// BR-INTERACTIVE-010: Find pending interactive session by remediation ID.
	FindPendingByRemediationID(rrID string) (string, bool)
	// BR-INTERACTIVE-010: Get RCA summary from latest completed session for context reconstruction.
	GetLatestRCASummaryByRemediationID(rrID string) (string, bool)
	// Get full RCA result from latest completed session for workflow discovery.
	GetLatestRCAResultByRemediationID(rrID string) (*katypes.InvestigationResult, bool)
	// #1384: Get the LazySink for a session so workflow_discovery can stream events.
	GetSessionLazySink(id string) (*session.LazySink, bool)
}

// AutonomousSessionLifecycle provides suspension, user-driving transition,
// and start/subscribe control of autonomous investigation sessions. Used by
// handleTakeover to transition the running autonomous session to
// user-driven mode before acquiring the interactive Lease. Split out from
// AutonomousSessionManager for ISP (GO-ANTIPATTERN-AUDIT-2026-07-01 Phase 5).
//
// v1.5: TransitionToUserDriving replaces SuspendInvestigation in the takeover
// path (BR-INTERACTIVE-004, #774). It cancels the autonomous goroutine, sets
// StatusUserDriving, and writes identity metadata so the poll response carries
// acting_user / acting_user_groups to AA for Rego policy evaluation.
type AutonomousSessionLifecycle interface {
	CancelInvestigation(id string) error
	SuspendInvestigation(id string) error
	TransitionToUserDriving(id, username string, groups []string) error
	ForceTransitionToUserDriving(rrID, username string, groups []string) error
	// #1390: Upgrade a running autonomous session to interactive in-place.
	UpgradeToInteractive(id, username string, groups []string) error
	// BR-INTERACTIVE-010: Launch a deferred investigation for a pending session.
	LaunchDeferredInvestigation(id string) error
	// BR-MCP-002: Start an autonomous investigation and return the session ID.
	StartInvestigation(ctx context.Context, fn session.InvestigateFunc, metadata map[string]string) (string, error)
	// BR-MCP-003: Subscribe to the event channel for a running investigation, activating the LazySink.
	Subscribe(ctx context.Context, id string) (<-chan session.InvestigationEvent, error)
}

// AutonomousSessionManager composes the query and lifecycle role interfaces
// for InvestigateTool's single autoMgr dependency (*session.Manager in
// production). Kept as a named union — rather than inlining the two
// interfaces at the call site — so existing implementers/mocks (which
// already implement every method) need no changes (GO-ANTIPATTERN-AUDIT-2026-07-01
// Phase 5; see docs/architecture/audits for rationale).
type AutonomousSessionManager interface {
	AutonomousSessionQuerier
	AutonomousSessionLifecycle
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

func (NopAutonomousManager) FindByRemediationID(string) (string, bool)                   { return "", false }
func (NopAutonomousManager) CancelInvestigation(string) error                            { return nil }
func (NopAutonomousManager) SuspendInvestigation(string) error                           { return nil }
func (NopAutonomousManager) TransitionToUserDriving(string, string, []string) error      { return nil }
func (NopAutonomousManager) ForceTransitionToUserDriving(string, string, []string) error { return nil }
func (NopAutonomousManager) UpgradeToInteractive(string, string, []string) error         { return nil }
func (NopAutonomousManager) FindPendingByRemediationID(string) (string, bool)            { return "", false }
func (NopAutonomousManager) LaunchDeferredInvestigation(string) error                    { return nil }
func (NopAutonomousManager) GetLatestRCASummaryByRemediationID(string) (string, bool) {
	return "", false
}
func (NopAutonomousManager) GetLatestRCAResultByRemediationID(string) (*katypes.InvestigationResult, bool) {
	return nil, false
}
func (NopAutonomousManager) StartInvestigation(context.Context, session.InvestigateFunc, map[string]string) (string, error) {
	return "", nil
}
func (NopAutonomousManager) Subscribe(context.Context, string) (<-chan session.InvestigationEvent, error) {
	// nolint:nilnil // no-op test double, never the production
	// AutonomousSessionManager (that's session.Manager) — nil channel/nil
	// error here just means this stub method is unused by the tests that
	// construct NopAutonomousManager (Issue #1546 Tier 2).
	return nil, nil
}
func (NopAutonomousManager) GetSessionLazySink(string) (*session.LazySink, bool) { return nil, false }

// InvestigateTool handles the kubernaut_investigate MCP tool actions:
// start, message, complete, cancel, takeover, discover_workflows.
// BR-INTERACTIVE-001, BR-INTERACTIVE-004.
type InvestigateTool struct {
	sessions       mcpinternal.SessionManager
	runner         InvestigatorRunner
	recon          mcpinternal.ContextReconstructor
	autoMgr        AutonomousSessionManager
	httpCompleter  HTTPSessionCompleter
	signalResolver SignalContextResolver
	rrChecker      RRExistenceChecker
	catalog        WorkflowCatalog
	metrics        ToolMetrics
	rateLimiter    MessageRateLimiter
	timeoutTracker TimeoutTracker
	auditStore     audit.AuditStore
	logger         logr.Logger
	notifyFn       func(sessionID, msg string) // optional: delivers timeout warnings to client
	sessionMu      sync.Map                    // rrID -> *sync.Mutex (per-session serialization)
	reconHistory   sync.Map                    // rrID -> []LLMMessage (reconstructed context for LLM)
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
		return t.handleComplete(input, user) //nolint:contextcheck // emitInteractiveCompleted uses audit.StoreBestEffort by design (ADR-038); see investigate_autonomous.go doc comment
	case ActionCancel:
		return t.handleCancel(input, user) //nolint:contextcheck // emitInteractiveCompleted uses audit.StoreBestEffort by design (ADR-038); see investigate_autonomous.go doc comment
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
