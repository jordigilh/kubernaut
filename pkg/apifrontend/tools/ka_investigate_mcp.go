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
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"k8s.io/apimachinery/pkg/api/meta"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	aiav1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// isPhaseActivePollTimeout caps the IS phase Active polling after AIA readiness.
// Short because the phase transition should follow almost immediately after AA submits.
const isPhaseActivePollTimeout = 5 * time.Second

type rrIDContextKey struct{}

// WithRRID attaches the remediation request ID to the context so that
// bridgeEventsCollectSummary can include it in structured event metadata.
func WithRRID(ctx context.Context, rrID string) context.Context {
	return context.WithValue(ctx, rrIDContextKey{}, rrID)
}

func extractRRIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(rrIDContextKey{}).(string); ok {
		return v
	}
	return ""
}

// takeoverISPhaseTimeout is used for the takeover path where AA must cancel
// the autonomous session and re-submit before setting IS phase=Active.
const takeoverISPhaseTimeout = 15 * time.Second

// ISSignaler abstracts IS CRD creation for the investigate tool.
// Implemented by session.CRDSessionService via an adapter in the handler layer.
type ISSignaler interface {
	// SignalInteractive creates an IS CRD before the await/connect loop.
	// joinMode should be "start" for fresh interactive or "takeover" for upgrading autonomous.
	// Returns the CRD name for later correlation updates.
	SignalInteractive(ctx context.Context, rrNamespace, rrName, taskID, username string, groups []string, joinMode string) (string, error)

	// UpdateCorrelation writes the KA session ID to IS CRD status after MCP connect.
	UpdateCorrelation(ctx context.Context, crdName, kaSessionID string) error
}

// InvestigateMCPArgs defines the input for the MCP-based kubernaut_investigate tool.
// Either RRID (for an existing RR) or APIVersion/Kind/Name (to create a new one)
// must be provided. When creating, an IS CRD is also created for the interactive flow.
type InvestigateMCPArgs struct {
	RRID string `json:"rr_id,omitempty"`
	// APIVersion is the Kubernetes API group/version (e.g., "apps/v1", "v1").
	// Required when creating a new RR (not using rr_id) (#1372).
	APIVersion string `json:"api_version,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
}

// InvestigateMCPResult is the output of the MCP investigate tool.
type InvestigateMCPResult struct {
	SessionID string          `json:"session_id"`
	Status    string          `json:"status"`
	Summary   string          `json:"summary,omitempty"`
	RRID      string          `json:"rr_id,omitempty"`
	Error     string          `json:"error,omitempty"`
	RCA       *InvestigateRCA `json:"rca,omitempty"`
}

// InvestigateRCA is the structured RCA data extracted from the KA complete event.
// It carries the AF-relevant subset of InvestigationResult for the LLM to pass
// through into present_decision (#1396).
type InvestigateRCA struct {
	Severity       string   `json:"severity,omitempty"`
	Confidence     float64  `json:"confidence,omitempty"`
	CausalChain    []string `json:"causal_chain,omitempty"`
	Target         string   `json:"target,omitempty"`
	RCASummary     string   `json:"rca_summary,omitempty"`
	TotalLLMTurns  int      `json:"total_llm_turns,omitempty"`
	TotalToolCalls int      `json:"total_tool_calls,omitempty"`
}

// SessionStartedHook is called after a successful StartInvestigation with the
// session context. Implementations typically create an InvestigationSession CRD.
// Errors are logged but do not fail the investigation.
type SessionStartedHook func(ctx context.Context, namespace, rrID, sessionID string) error

// InvestigateConfig bundles the dependencies for HandleInvestigationMCPWithRegistry
// and NewInvestigateMCPTool, replacing positional parameters.
type InvestigateConfig struct {
	MCPClient ka.MCPClient
	Client    crclient.Client
	Namespace string
	Auditor   audit.Emitter
	Registry  *MonitorRegistry
	OnStarted SessionStartedHook
	Pool      *ka.KASessionPool
	Signaler  ISSignaler
	Triager   *severity.Triager
}

// HandleInvestigationMCP starts a dedicated MCP investigation session. When a
// K8s client and namespace are provided, it first polls the AIAnalysis CRD to
// wait for AA to submit the investigation to KA (BR-INTERACTIVE-010). After
// confirmation (or best-effort timeout), it calls action=start on KA via the
// dedicated MCP session, starts a background goroutine that bridges
// investigation events to the A2A stream, and returns immediately.
//
// Event streaming is conditional: events only flow when KA's deferred
// investigation launches successfully (InvestigationSessionID is populated).
// If no pending session exists, the MCP session still acquires the interactive
// lease but the Events channel may be nil or empty.
func HandleInvestigationMCP(ctx context.Context, mcpClient ka.MCPClient, client crclient.Client, namespace string, args InvestigateMCPArgs, auditor audit.Emitter) (InvestigateMCPResult, error) {
	return HandleInvestigationMCPWithRegistry(ctx, &InvestigateConfig{
		MCPClient: mcpClient,
		Client:    client,
		Namespace: namespace,
		Auditor:   auditor,
	}, args, false, "")
}

// HandleInvestigationMCPWithRegistry is like HandleInvestigationMCP but also
// registers the session in a MonitorRegistry for lifecycle management and
// invokes onStarted (if provided) to create the IS CRD after a successful start.
//
// When blocking is true, the function waits for the investigation to complete
// (or ctx cancellation) and returns the collected summary in InvestigateMCPResult.
// Events are streamed to the A2A SSE via the EventBridge during the wait.
// After the investigation completes, if pool is non-nil the MCP session is
// handed off to the pool (keyed by rr_id + username) so that subsequent tool
// calls (discover_workflows, select_workflow) reuse the same connection and
// driver lease without requiring a separate takeover.
// When blocking is false, a background goroutine bridges events and the function
// returns immediately (legacy behavior for the MCP bridge path).
//
// signaler (optional): When provided, creates the IS CRD BEFORE the await loop
// (pure CRD-driven coordination per DD-INTERACTIVE-002). This enables AA to detect
// interactive intent via IS watch and resubmit with interactive=true. After successful
// MCP connect, UpdateCorrelation writes the KA session ID to the IS status.
func HandleInvestigationMCPWithRegistry(ctx context.Context, cfg *InvestigateConfig, args InvestigateMCPArgs, blocking bool, username string) (InvestigateMCPResult, error) {
	if cfg.MCPClient == nil {
		return InvestigateMCPResult{}, fmt.Errorf("KA MCP client unavailable")
	}

	rrSeverity, err := resolveInvestigationRR(ctx, cfg, &args)
	if err != nil {
		return InvestigateMCPResult{}, err
	}

	identity := auth.UserIdentityFromContext(ctx)
	isCRDName, err := signalInteractiveSession(ctx, cfg, args.RRID, identity)
	if err != nil {
		return InvestigateMCPResult{}, err
	}

	kaSessionID := awaitInvestigationReady(ctx, cfg, args.RRID, isCRDName, blocking)

	logger := logr.FromContextOrDiscard(ctx)
	result, earlyResult, err := startKAInvestigation(ctx, cfg, args.RRID, kaSessionID, rrSeverity, logger)
	if err != nil {
		return InvestigateMCPResult{}, err
	}
	if earlyResult != nil {
		return *earlyResult, nil
	}

	finalizeInvestigationStart(ctx, cfg, args.RRID, isCRDName, result, logger)

	cleanup := func() {
		if result.Closer != nil {
			result.Closer()
		}
		if cfg.Registry != nil {
			cfg.Registry.Deregister(result.SessionID)
		}
	}

	if result.Events == nil {
		cleanup()
		return InvestigateMCPResult{
			SessionID: result.SessionID,
			Status:    result.Status,
			RRID:      args.RRID,
		}, nil
	}

	if blocking {
		return runBlockingInvestigation(ctx, cfg, blockingInvestigationParams{
			RRID:       args.RRID,
			Username:   username,
			RRSeverity: rrSeverity,
			Result:     result,
			Cleanup:    cleanup,
			Logger:     logger,
		}), nil
	}

	// Non-blocking: spawn background goroutine for MCP bridge path.
	startNonBlockingBridge(ctx, result.Events, cleanup)

	return InvestigateMCPResult{
		SessionID: result.SessionID,
		Status:    result.Status,
		RRID:      args.RRID,
	}, nil
}

// resolveInvestigationRR validates args, optionally creates a new RR from
// resource args (rr_id vs api_version/kind/name), and seeds the EventBridge
// RR context (#1423) for Console banner population. Returns the severity
// assessed during RR creation (if any) for later fallback-RCA use.
func resolveInvestigationRR(ctx context.Context, cfg *InvestigateConfig, args *InvestigateMCPArgs) (rrSeverity string, err error) {
	hasRRID := args.RRID != ""
	hasResourceArgs := args.APIVersion != "" || args.Kind != "" || args.Name != "" || args.Namespace != ""

	if !hasRRID && !hasResourceArgs {
		return "", fmt.Errorf("rr_id or api_version/kind/name required")
	}
	if hasRRID {
		if err := validate.RRID(args.RRID); err != nil {
			return "", fmt.Errorf("invalid rr_id: %w", err)
		}
	}

	identity := auth.UserIdentityFromContext(ctx)

	if !hasRRID && hasResourceArgs {
		rrSeverity, err = createRRForInvestigation(ctx, cfg, args, identity)
		if err != nil {
			return "", err
		}
	}

	if args.RRID == "" {
		return "", fmt.Errorf("rr_id is required for MCP investigation")
	}

	// For the existing-RR path (rr_id provided as input), set minimal RR context.
	// Resource metadata may not be available without a K8s read; rr_id + phase
	// is sufficient for Console escape-hatch buttons (#1423).
	if !hasResourceArgs {
		launcher.SetRRContextSafe(ctx, &launcher.RRContext{
			RRID:  args.RRID,
			Phase: "Investigating",
		})
	}

	return rrSeverity, nil
}

// createRRForInvestigation creates a new RemediationRequest from
// api_version/kind/name/namespace args, resolving cluster-scoped namespace
// stripping and rejecting service-account-initiated interactive
// investigations. Mutates args.RRID/args.Namespace and seeds the
// EventBridge RR context (#1423, AU-3, SI-4).
func createRRForInvestigation(ctx context.Context, cfg *InvestigateConfig, args *InvestigateMCPArgs, identity *auth.UserIdentity) (string, error) {
	if err := validate.APIVersion(args.APIVersion); err != nil {
		return "", fmt.Errorf("%w", err)
	}
	if args.Kind == "" || args.Name == "" {
		return "", fmt.Errorf("kind and name required when providing api_version/kind/name")
	}

	clusterScoped := resolveClusterScoped(ctx, args)
	if clusterScoped && args.Namespace != "" {
		args.Namespace = ""
	}

	if identity != nil && identity.IsServiceAccount {
		return "", fmt.Errorf("interactive investigation cannot be started by service accounts")
	}
	if cfg.Client == nil {
		return "", fmt.Errorf("k8s client unavailable for RR creation")
	}

	createArgs := &CreateRRArgs{
		Namespace:     args.Namespace,
		Kind:          args.Kind,
		Name:          args.Name,
		APIVersion:    args.APIVersion,
		ClusterScoped: clusterScoped,
	}
	createUser := ""
	if identity != nil {
		createUser = identity.Username
	}
	result, err := HandleCreateRR(ctx, &ToolDeps{Client: cfg.Client, ControllerNS: cfg.Namespace, Triager: cfg.Triager, Auditor: cfg.Auditor}, createArgs, createUser)
	if err != nil {
		return "", fmt.Errorf("create RR for investigation: %w", err)
	}
	args.RRID = result.RRID

	launcher.SetRRContextSafe(ctx, &launcher.RRContext{
		RRID:      result.RRID,
		Namespace: args.Namespace,
		Kind:      args.Kind,
		Target:    remediationrequest.FormatResourceDisplay(args.Kind, args.Name),
		AlertName: result.SignalName,
		Phase:     "Investigating",
	})

	return result.Severity, nil
}

// resolveClusterScoped determines whether the target resource is
// cluster-scoped, preferring the REST mapper (if attached to ctx) over the
// static fallback map.
func resolveClusterScoped(ctx context.Context, args *InvestigateMCPArgs) bool {
	if args.Namespace == "" {
		return true
	}
	mapper := RESTMapperFromContext(ctx)
	if mapper != nil {
		resolved := ResolveEffectiveNamespace(mapper, args.Kind, args.Namespace, logr.FromContextOrDiscard(ctx))
		return resolved == ""
	}
	clusterScoped := scope.IsClusterScopedKind(args.Kind)
	if clusterScoped {
		logr.FromContextOrDiscard(ctx).Info("stripping namespace for cluster-scoped resource (static fallback)",
			"kind", args.Kind,
			"stripped_namespace", args.Namespace,
		)
	}
	return clusterScoped
}

// signalInteractiveSession creates the IS CRD before the await loop when a
// signaler is configured (DD-INTERACTIVE-002, BR-INTERACTIVE-010), detecting
// and announcing a takeover if an autonomous investigation is already
// running. Returns the created IS CRD name (used later for
// UpdateCorrelation), or an error only when KA's single-driver enforcement
// rejects a duplicate session.
func signalInteractiveSession(ctx context.Context, cfg *InvestigateConfig, rrID string, identity *auth.UserIdentity) (string, error) {
	if cfg.Signaler == nil || cfg.Client == nil || cfg.Namespace == "" {
		return "", nil
	}
	joinMode := "start"
	if isAutonomousInvestigation(ctx, cfg.Client, cfg.Namespace, rrID) {
		joinMode = "takeover"
		_ = launcher.EmitStatusSafe(ctx, "Autonomous investigation detected, signaling takeover...")
	}

	signalUsername := ""
	var groups []string
	if identity != nil {
		signalUsername = identity.Username
		groups = identity.Groups
	}

	taskID := fmt.Sprintf("a2a-%s", rrID)
	isCRDName, sigErr := cfg.Signaler.SignalInteractive(ctx, cfg.Namespace, rrID, taskID, signalUsername, groups, joinMode)
	if sigErr != nil {
		logger := logr.FromContextOrDiscard(ctx)
		if strings.Contains(sigErr.Error(), "session_active") {
			logger.Info("IS CRD single-driver enforcement: rejecting duplicate session", "rr_id", rrID, "error", sigErr)
			return "", sigErr
		}
		logger.Error(sigErr, "failed to create IS CRD (proceeding without IS signal)", "rr_id", rrID)
		return "", nil
	}
	return isCRDName, nil
}

// awaitInvestigationReady waits for the AIA CRD to show a pending KA session
// (confirming AA submitted with interactive=true) and for the IS CRD phase
// to become Active (confirming AA acknowledged the interactive session).
// Both waits are best-effort: on timeout the investigation proceeds without
// a resolved kaSessionID. The blocking path uses a longer await timeout
// because AA needs time to detect the IS CRD and resubmit to KA.
func awaitInvestigationReady(ctx context.Context, cfg *InvestigateConfig, rrID, isCRDName string, blocking bool) string {
	if cfg.Client == nil || cfg.Namespace == "" {
		return ""
	}

	awaitTimeout := 10 * time.Second
	if blocking {
		awaitTimeout = 60 * time.Second
	}
	checkCtx, checkCancel := context.WithTimeout(ctx, awaitTimeout)
	awaitResult, awaitErr := HandleAwaitSession(checkCtx, cfg.Client, AwaitSessionArgs{
		Namespace: cfg.Namespace,
		RRName:    rrID,
	})
	checkCancel()

	var kaSessionID string
	if awaitErr == nil && awaitResult.Status == "ready" {
		kaSessionID = awaitResult.SessionID
		_ = launcher.EmitStatusSafe(ctx, "Investigation session ready, connecting to KA...")
	}

	// Wait for the IS CRD phase to become Active — AA sets this after
	// acknowledging the interactive session and submitting to KA with
	// interactive=true. Without this, action=start may arrive before
	// KA has a pending session to activate.
	isPhaseTimeout := isPhaseActivePollTimeout
	if isCRDName != "" {
		isPhaseTimeout = takeoverISPhaseTimeout
	}
	isCtx, isCancel := context.WithTimeout(ctx, isPhaseTimeout)
	if AwaitISPhaseActive(isCtx, cfg.Client, cfg.Namespace, rrID) {
		_ = launcher.EmitStatusSafe(ctx, "Interactive session acknowledged by AA, starting investigation...")
	}
	isCancel()

	return kaSessionID
}

// startKAInvestigation calls action=start on KA via the dedicated MCP
// session. When KA reports session_active (another driver already owns the
// investigation), this returns a non-nil structured "in progress" result
// (emitting an early RCA from rrSeverity, if available) instead of an
// error, so the LLM sees a normal tool response rather than a
// retry-triggering error.
func startKAInvestigation(ctx context.Context, cfg *InvestigateConfig, rrID, kaSessionID, rrSeverity string, logger logr.Logger) (*ka.StartInvestigationResult, *InvestigateMCPResult, error) {
	logger.Info("StartInvestigation: calling MCP client",
		"rr_id", rrID, "ka_session_id", kaSessionID, "ctx_err", ctx.Err())

	result, err := cfg.MCPClient.StartInvestigation(ctx, ka.StartInvestigationArgs{
		RRID:      rrID,
		SessionID: kaSessionID,
	})
	if err != nil {
		if strings.Contains(err.Error(), "session_active") {
			driver := extractDriverFromSessionActiveError(err)
			logger.Info("session_active from KA: returning structured result instead of error",
				"rr_id", rrID, "driver", driver)

			if rrSeverity != "" {
				rca := &InvestigateRCA{
					Severity:   rrSeverity,
					Confidence: 0.6,
					RCASummary: fmt.Sprintf("Severity assessed from resource metadata (investigation in progress by %s)", driver),
				}
				emitEarlyRCA(ctx, rca)
				emitFallbackInvestigationArtifact(ctx, rca, rrID)
				logger.Info("emitted early_rca on session_active path",
					"rr_id", rrID, "severity", rrSeverity, "driver", driver)
			}

			return nil, &InvestigateMCPResult{
				Status: "session_active",
				RRID:   rrID,
				Error: fmt.Sprintf(
					"An investigation for this resource is already in progress, driven by %s. "+
						"Do not retry kubernaut_investigate. "+
						"Use kubernaut_get_remediation with rr_id %s to check its status.",
					driver, rrID),
			}, nil
		}
		return nil, nil, fmt.Errorf("start MCP investigation: %w", err)
	}
	logger.Info("StartInvestigation: MCP session established",
		"rr_id", rrID, "session_id", result.SessionID,
		"status", result.Status, "events_nil", result.Events == nil)
	return result, nil, nil
}

// finalizeInvestigationStart emits the KA-delegation audit event, invokes
// the OnStarted hook (IS CRD creation) and IS correlation update, and
// registers the session in the MonitorRegistry so StopAll can force-close
// on shutdown (the bridge goroutine/blocking path deregisters on exit).
func finalizeInvestigationStart(ctx context.Context, cfg *InvestigateConfig, rrID, isCRDName string, result *ka.StartInvestigationResult, logger logr.Logger) {
	if cfg.Auditor != nil {
		cfg.Auditor.Emit(ctx, &audit.Event{
			Type: audit.EventKADelegated,
			Detail: map[string]string{
				"rr_id":             rrID,
				"session_id":        result.SessionID,
				"ka_correlation_id": result.SessionID,
				"delegation_type":   "interactive",
			},
		})
	}

	if cfg.OnStarted != nil && result.SessionID != "" {
		if hookErr := cfg.OnStarted(ctx, cfg.Namespace, rrID, result.SessionID); hookErr != nil {
			logr.FromContextOrDiscard(ctx).Error(hookErr, "IS CRD creation failed after investigate",
				"rr_id", rrID,
				"session_id", result.SessionID,
				"namespace", cfg.Namespace,
			)
			_ = launcher.EmitStatusSafe(ctx, fmt.Sprintf("Warning: IS CRD creation failed (%s), investigation continues", security.RedactError(hookErr)))
		}
	}

	if cfg.Signaler != nil && isCRDName != "" && result.SessionID != "" {
		if corrErr := cfg.Signaler.UpdateCorrelation(ctx, isCRDName, result.SessionID); corrErr != nil {
			logger.Error(corrErr, "IS CRD correlation update failed (non-fatal)",
				"crd_name", isCRDName, "session_id", result.SessionID)
		}
	}

	// Track session in registry before starting goroutine so StopAll can
	// force-close on SIGTERM. The goroutine deregisters on natural exit.
	if cfg.Registry != nil {
		cfg.Registry.Register(result.SessionID, result.Closer)
	}
}

// blockingInvestigationParams groups the values threaded through
// runBlockingInvestigation. Extracted per AGENTS.md's 8+-param
// Options-pattern rule (GO-ANTIPATTERN-AUDIT-2026-07-01 Phase 4h).
type blockingInvestigationParams struct {
	RRID       string
	Username   string
	RRSeverity string
	Result     *ka.StartInvestigationResult
	Cleanup    func()
	Logger     logr.Logger
}

// runBlockingInvestigation bridges KA events into a collected RCA summary,
// synthesizes a fallback RCA from severity triage when KA produced none,
// and hands the MCP session off to the pool (if available) so subsequent
// tool calls reuse the connection; otherwise it closes the session via
// cleanup.
func runBlockingInvestigation(ctx context.Context, cfg *InvestigateConfig, p blockingInvestigationParams) InvestigateMCPResult {
	rrID, username, rrSeverity, result, cleanup, logger := p.RRID, p.Username, p.RRSeverity, p.Result, p.Cleanup, p.Logger
	logger.Info("bridgeEventsCollectSummary: starting blocking event bridge",
		"rr_id", rrID, "session_id", result.SessionID, "ctx_err", ctx.Err())
	bridgeCtx := WithRRID(ctx, rrID)
	summary, rca, exitReason := bridgeEventsCollectSummary(bridgeCtx, result.Events, BridgeInactivityTimeout)
	status := ExitReasonToStatus(exitReason)
	logger.Info("bridgeEventsCollectSummary: finished",
		"rr_id", rrID, "status", status, "exit_reason", exitReason, "summary_len", len(summary))

	if exitReason == ExitReasonInactivityTimeout && cfg.Auditor != nil {
		cfg.Auditor.Emit(ctx, &audit.Event{
			Type: audit.EventInvestigationTimeout,
			Detail: map[string]string{
				"rr_id":              rrID,
				"session_id":         result.SessionID,
				"exit_reason":        exitReason,
				"inactivity_timeout": BridgeInactivityTimeout.String(),
				"summary_len":        fmt.Sprintf("%d", len(summary)),
			},
		})
	}

	// Fallback: when KA produced no RCA (e.g. user-driving mode with
	// no autonomous session) but severity triage completed during RR
	// creation, emit progressive events using the triage data so the
	// user gets immediate severity feedback.
	if rca == nil && rrSeverity != "" {
		rca = &InvestigateRCA{
			Severity:   rrSeverity,
			Confidence: 0.6,
			RCASummary: "Severity assessed from resource metadata (full investigation pending)",
		}
		emitEarlyRCA(ctx, rca)
		emitFallbackInvestigationArtifact(ctx, rca, rrID)
		logger.Info("emitted fallback early_rca from severity triage",
			"rr_id", rrID, "severity", rrSeverity)
		if summary == "" {
			summary = rca.RCASummary
		}
	}

	handoffOrCloseSession(ctx, cfg, rrID, username, result, cleanup, logger)

	return InvestigateMCPResult{
		SessionID: result.SessionID,
		Status:    status,
		Summary:   summary,
		RRID:      rrID,
		RCA:       rca,
	}
}

// handoffOrCloseSession hands the MCP session off to the pool (keyed by
// rr_id+username) so discover_workflows/select_workflow reuse the same
// connection and driver lease, or falls back to closing it via cleanup.
func handoffOrCloseSession(ctx context.Context, cfg *InvestigateConfig, rrID, username string, result *ka.StartInvestigationResult, cleanup func(), logger logr.Logger) {
	if cfg.Pool == nil || result.Session == nil || username == "" {
		cleanup()
		return
	}
	watchDone := make(chan struct{})
	onRelease := func() { close(watchDone) }
	if injectErr := cfg.Pool.InjectVerified(ctx, rrID, username, result.Session, onRelease); injectErr != nil {
		logger.Info("investigation session dead on handoff, skipping pool inject",
			"rr_id", rrID, "session_id", result.SessionID, "error", injectErr.Error())
		if cfg.Registry != nil {
			cfg.Registry.Deregister(result.SessionID)
		}
		return
	}
	if cfg.Registry != nil {
		cfg.Registry.Deregister(result.SessionID)
	}
	watchCtx := context.WithoutCancel(ctx)
	go WatchTerminalEvents(watchCtx, result.Events, rrID, watchDone)
	logger.Info("investigation session handed off to pool",
		"rr_id", rrID, "session_id", result.SessionID, "username", username)
}

// startNonBlockingBridge spawns a background goroutine that bridges KA
// events to the A2A stream and returns immediately (legacy MCP bridge
// path). The goroutine is detached from the tool context (which wrapTool
// cancels via its deferred cancel() on handler return) and bounded by
// NonBlockingBridgeTTL instead, so it survives past the handler call.
func startNonBlockingBridge(ctx context.Context, events <-chan ka.InvestigationEvent, cleanup func()) {
	bridgeCtx, bridgeCancel := context.WithTimeout(context.WithoutCancel(ctx), NonBlockingBridgeTTL)
	// Snapshot the inactivity timeout synchronously, before spawning the
	// goroutine. Reading the package-level BridgeInactivityTimeout directly
	// inside the goroutine closure would defer the read until the goroutine
	// is actually scheduled, which can race with a concurrent write to the
	// same var (e.g. from a test overriding it for a different case).
	inactivityTimeout := BridgeInactivityTimeout
	go func() {
		defer bridgeCancel()
		defer cleanup()
		BridgeEventsToA2A(bridgeCtx, events, inactivityTimeout)
	}()
}

// NewInvestigateMCPTool creates the kubernaut_investigate tool backed by MCP
// for the A2A agent path. The tool blocks until the investigation completes,
// streaming live events to kagenti while collecting the final RCA summary.
// The LLM receives the full results in the tool response and can proceed to
// the next phase deterministically.
//
// client and namespace enable AIA CRD polling before starting the
// investigation (BR-INTERACTIVE-010). Pass nil client to skip polling.
// registry is optional; when provided, sessions are tracked for graceful shutdown.
// onStarted is called after a successful start to create the IS CRD.
// pool is optional; when provided, the MCP session is handed off to the pool
// after the investigation so that discover_workflows / select_workflow reuse
// the same connection and driver lease.
func NewInvestigateMCPTool(cfg *InvestigateConfig, mapper meta.RESTMapper) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "kubernaut_investigate",
		Description: "Investigate an infrastructure incident via MCP. " +
			"Provide rr_id to resume an existing investigation, or " +
			"api_version/kind/name (and optional namespace for namespaced resources) " +
			"to create a new investigation. " +
			"This tool blocks until the investigation completes and returns " +
			"the root-cause analysis summary. Live progress events stream " +
			"to the user automatically while the investigation runs.",
	}, func(ctx tool.Context, args InvestigateMCPArgs) (InvestigateMCPResult, error) {
		user := usernameFromContext(ctx)
		toolCtx := ContextWithRESTMapper(ctx, mapper)
		return HandleInvestigationMCPWithRegistry(toolCtx, cfg, args, true, user)
	})
}

// isAutonomousInvestigation checks if the given RR has an active AIA CRD with
// a session ID already assigned (indicating autonomous investigation in progress).
// Returns true when a takeover is needed instead of a fresh start.
func isAutonomousInvestigation(ctx context.Context, client crclient.Client, namespace, rrName string) bool {
	if client == nil || namespace == "" || rrName == "" {
		return false
	}
	var list aiav1alpha1.AIAnalysisList
	if err := client.List(ctx, &list, crclient.InNamespace(namespace)); err != nil {
		return false
	}
	for i := range list.Items {
		item := &list.Items[i]
		if item.Spec.RemediationRequestRef.Name != rrName {
			continue
		}
		if item.Status.KASession != nil && item.Status.KASession.ID != "" {
			return true
		}
	}
	return false
}

var reDriverFromMap = regexp.MustCompile(`driver:(\S+?)[\]\)\s,}]`)

func extractDriverFromSessionActiveError(err error) string {
	if m := reDriverFromMap.FindStringSubmatch(err.Error()); len(m) > 1 {
		return m[1]
	}
	return "another user"
}
