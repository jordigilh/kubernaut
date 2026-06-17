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
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	aiav1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
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
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Summary   string `json:"summary,omitempty"`
	RRID      string `json:"rr_id,omitempty"`
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
	return HandleInvestigationMCPWithRegistry(ctx, mcpClient, client, namespace, args, auditor, nil, nil, false, nil, "", nil, nil)
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
func HandleInvestigationMCPWithRegistry(ctx context.Context, mcpClient ka.MCPClient, client crclient.Client, namespace string, args InvestigateMCPArgs, auditor audit.Emitter, registry *MonitorRegistry, onStarted SessionStartedHook, blocking bool, pool *ka.KASessionPool, username string, signaler ISSignaler, triager *severity.Triager) (InvestigateMCPResult, error) {
	if mcpClient == nil {
		return InvestigateMCPResult{}, fmt.Errorf("KA MCP client unavailable")
	}

	hasRRID := args.RRID != ""
	hasResourceArgs := args.APIVersion != "" || args.Kind != "" || args.Name != "" || args.Namespace != ""

	if !hasRRID && !hasResourceArgs {
		return InvestigateMCPResult{}, fmt.Errorf("rr_id or api_version/kind/name required")
	}

	if hasRRID {
		if err := validate.RRID(args.RRID); err != nil {
			return InvestigateMCPResult{}, fmt.Errorf("invalid rr_id: %w", err)
		}
	}

	identity := auth.UserIdentityFromContext(ctx)

	var rrSeverity string
	if !hasRRID && hasResourceArgs {
		if err := validate.APIVersion(args.APIVersion); err != nil {
			return InvestigateMCPResult{}, fmt.Errorf("%w", err)
		}
		if args.Kind == "" || args.Name == "" {
			return InvestigateMCPResult{}, fmt.Errorf("kind and name required when providing api_version/kind/name")
		}

		clusterScoped := args.Namespace == ""

		if identity != nil && identity.IsServiceAccount {
			return InvestigateMCPResult{}, fmt.Errorf("interactive investigation cannot be started by service accounts")
		}

		if client == nil {
			return InvestigateMCPResult{}, fmt.Errorf("k8s client unavailable for RR creation")
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
		result, err := HandleCreateRR(ctx, client, nil, namespace, createArgs, createUser, triager, auditor)
		if err != nil {
			return InvestigateMCPResult{}, fmt.Errorf("create RR for investigation: %w", err)
		}
		args.RRID = result.RRID
		rrSeverity = result.Severity

		// #1423 (AU-3, SI-4): Set RR context on EventBridge so all
		// subsequent status events include rr_id, namespace, kind, target,
		// alert_name, and phase for Console banner population.
		launcher.SetRRContextSafe(ctx, &launcher.RRContext{
			RRID:      result.RRID,
			Namespace: args.Namespace,
			Kind:      args.Kind,
			Target:    args.Name,
			AlertName: result.SignalName,
			Phase:     "Investigating",
		})
	}

	if args.RRID == "" {
		return InvestigateMCPResult{}, fmt.Errorf("rr_id is required for MCP investigation")
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

	// Determine if this RR already has an autonomous investigation running.
	// When signaler is available, create IS CRD BEFORE the await loop to signal
	// interactive intent to AA (DD-INTERACTIVE-002, BR-INTERACTIVE-010).
	var isCRDName string
	if signaler != nil && client != nil && namespace != "" {
		joinMode := "start"
		if isAutonomousInvestigation(ctx, client, namespace, args.RRID) {
			joinMode = "takeover"
			_ = launcher.EmitStatusSafe(ctx, "Autonomous investigation detected, signaling takeover...")
		}

		username := ""
		var groups []string
		if identity != nil {
			username = identity.Username
			groups = identity.Groups
		}

		taskID := fmt.Sprintf("a2a-%s", args.RRID)
		var sigErr error
		isCRDName, sigErr = signaler.SignalInteractive(ctx, namespace, args.RRID, taskID, username, groups, joinMode)
		if sigErr != nil {
			logger := logr.FromContextOrDiscard(ctx)
			if strings.Contains(sigErr.Error(), "session_active") {
				logger.Info("IS CRD single-driver enforcement: rejecting duplicate session", "rr_id", args.RRID, "error", sigErr)
				return InvestigateMCPResult{}, sigErr
			}
			logger.Error(sigErr, "failed to create IS CRD (proceeding without IS signal)", "rr_id", args.RRID)
		}
	}

	// Wait for AIA CRD to show a session ID, confirming AA has submitted
	// to KA with interactive=true and KA has created a pending session.
	// Blocking path uses a longer timeout because the IS CRD (created by
	// kubernaut_investigate) needs time for AA to detect and resubmit to KA.
	var kaSessionID string
	if client != nil && namespace != "" {
		awaitTimeout := 10 * time.Second
		if blocking {
			awaitTimeout = 60 * time.Second
		}
		checkCtx, checkCancel := context.WithTimeout(ctx, awaitTimeout)
		awaitResult, awaitErr := HandleAwaitSession(checkCtx, client, AwaitSessionArgs{
			Namespace: namespace,
			RRName:    args.RRID,
		})
		checkCancel()
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
		if AwaitISPhaseActive(isCtx, client, namespace, args.RRID) {
			_ = launcher.EmitStatusSafe(ctx, "Interactive session acknowledged by AA, starting investigation...")
		}
		isCancel()
	}

	logger := logr.FromContextOrDiscard(ctx)
	logger.Info("StartInvestigation: calling MCP client",
		"rr_id", args.RRID, "ka_session_id", kaSessionID, "ctx_err", ctx.Err())

	result, err := mcpClient.StartInvestigation(ctx, ka.StartInvestigationArgs{
		RRID:      args.RRID,
		SessionID: kaSessionID,
	})
	if err != nil {
		if strings.Contains(err.Error(), "session_active") {
			driver := extractDriverFromSessionActiveError(err)
			logger.Info("session_active from KA: returning structured result instead of error",
				"rr_id", args.RRID, "driver", driver)

			if rrSeverity != "" {
				rca := &InvestigateRCA{
					Severity:   rrSeverity,
					Confidence: 0.6,
					RCASummary: fmt.Sprintf("Severity assessed from resource metadata (investigation in progress by %s)", driver),
				}
				emitEarlyRCA(ctx, rca)
				emitFallbackInvestigationArtifact(ctx, rca, args.RRID)
				logger.Info("emitted early_rca on session_active path",
					"rr_id", args.RRID, "severity", rrSeverity, "driver", driver)
			}

			return InvestigateMCPResult{
				Status: "session_active",
				RRID:   args.RRID,
				Error: fmt.Sprintf(
					"An investigation for this resource is already in progress, driven by %s. "+
						"Do not retry kubernaut_investigate. "+
						"Use kubernaut_get_remediation with rr_id %s to check its status.",
					driver, args.RRID),
			}, nil
		}
		return InvestigateMCPResult{}, fmt.Errorf("start MCP investigation: %w", err)
	}
	logger.Info("StartInvestigation: MCP session established",
		"rr_id", args.RRID, "session_id", result.SessionID,
		"status", result.Status, "events_nil", result.Events == nil)

	if auditor != nil {
		auditor.Emit(ctx, &audit.Event{
			Type: audit.EventKADelegated,
			Detail: map[string]string{
				"rr_id":             args.RRID,
				"session_id":        result.SessionID,
				"ka_correlation_id": result.SessionID,
				"delegation_type":   "interactive",
			},
		})
	}

	if onStarted != nil && result.SessionID != "" {
		if hookErr := onStarted(ctx, namespace, args.RRID, result.SessionID); hookErr != nil {
			logr.FromContextOrDiscard(ctx).Error(hookErr, "IS CRD creation failed after investigate",
				"rr_id", args.RRID,
				"session_id", result.SessionID,
				"namespace", namespace,
			)
			_ = launcher.EmitStatusSafe(ctx, fmt.Sprintf("Warning: IS CRD creation failed (%s), investigation continues", security.RedactError(hookErr)))
		}
	}

	if signaler != nil && isCRDName != "" && result.SessionID != "" {
		if corrErr := signaler.UpdateCorrelation(ctx, isCRDName, result.SessionID); corrErr != nil {
			logger.Error(corrErr, "IS CRD correlation update failed (non-fatal)",
				"crd_name", isCRDName, "session_id", result.SessionID)
		}
	}

	// Track session in registry before starting goroutine so StopAll can
	// force-close on SIGTERM. The goroutine deregisters on natural exit.
	if registry != nil {
		registry.Register(result.SessionID, result.Closer)
	}

	cleanup := func() {
		if result.Closer != nil {
			result.Closer()
		}
		if registry != nil {
			registry.Deregister(result.SessionID)
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
		logger.Info("bridgeEventsCollectSummary: starting blocking event bridge",
			"rr_id", args.RRID, "session_id", result.SessionID, "ctx_err", ctx.Err())
		bridgeCtx := WithRRID(ctx, args.RRID)
		summary, rca := bridgeEventsCollectSummary(bridgeCtx, result.Events, BridgeInactivityTimeout)
		status := "completed"
		if ctx.Err() != nil {
			status = "timeout"
			logger.Info("bridgeEventsCollectSummary: context cancelled",
				"rr_id", args.RRID, "ctx_err", ctx.Err(), "summary_len", len(summary))
		}
		logger.Info("bridgeEventsCollectSummary: finished",
			"rr_id", args.RRID, "status", status, "summary_len", len(summary))

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
			emitFallbackInvestigationArtifact(ctx, rca, args.RRID)
			logger.Info("emitted fallback early_rca from severity triage",
				"rr_id", args.RRID, "severity", rrSeverity)
			if summary == "" {
				summary = rca.RCASummary
			}
		}

		// Hand off the MCP session to the pool so discover_workflows /
		// select_workflow reuse the same connection and driver lease.
		// If no pool is available, fall back to closing the session.
		if pool != nil && result.Session != nil && username != "" {
			watchDone := make(chan struct{})
			onRelease := func() { close(watchDone) }
			if injectErr := pool.InjectVerified(ctx, args.RRID, username, result.Session, onRelease); injectErr != nil {
				logger.Info("investigation session dead on handoff, skipping pool inject",
					"rr_id", args.RRID, "session_id", result.SessionID, "error", injectErr.Error())
				if registry != nil {
					registry.Deregister(result.SessionID)
				}
			} else {
				if registry != nil {
					registry.Deregister(result.SessionID)
				}
				watchCtx := context.WithoutCancel(ctx)
				go WatchTerminalEvents(watchCtx, result.Events, args.RRID, watchDone)
				logger.Info("investigation session handed off to pool",
					"rr_id", args.RRID, "session_id", result.SessionID, "username", username)
			}
		} else {
			cleanup()
		}

		return InvestigateMCPResult{
			SessionID: result.SessionID,
			Status:    status,
			Summary:   summary,
			RRID:      args.RRID,
			RCA:       rca,
		}, nil
	}

	// Non-blocking: spawn background goroutine for MCP bridge path.
	// Detach from the tool context (which is cancelled by wrapTool's defer cancel()
	// on handler return) and use an explicit investigation TTL to bound the bridge
	// lifetime. Without this, the bridge goroutine would exit immediately when the
	// handler returns.
	bridgeTTL := NonBlockingBridgeTTL
	bridgeInactivity := BridgeInactivityTimeout
	bridgeCtx, bridgeCancel := context.WithTimeout(context.WithoutCancel(ctx), bridgeTTL)
	go func() {
		defer bridgeCancel()
		defer cleanup()
		BridgeEventsToA2A(bridgeCtx, result.Events, bridgeInactivity)
	}()

	return InvestigateMCPResult{
		SessionID: result.SessionID,
		Status:    result.Status,
		RRID:      args.RRID,
	}, nil
}

// BridgeEventsToA2A reads investigation events from the KA MCP session and
// emits filtered reasoning artifacts to the A2A stream. A keepalive is sent
// every 5s to prevent idle SSE timeouts during long KA tool executions.
// This complements the streaming executor-level keepalive which covers
// gaps between tool calls.
//
// NOTE: This is the non-blocking (legacy) bridge path. It does NOT handle
// EventTypeAlignmentVerdict structured emission — that is handled only by
// bridgeEventsCollectSummary (the blocking path used by the A2A agent).
// The non-blocking path emits the raw event text via emitEventToA2A, but
// FormatEventForUser returns "" for alignment_verdict, so the event is
// effectively dropped. If the non-blocking path needs alignment verdict
// support in the future, add a handler here and inject WithRRID on the
// bridgeCtx at the call site (line ~438).
func BridgeEventsToA2A(ctx context.Context, events <-chan ka.InvestigationEvent, inactivityTimeout time.Duration) {
	keepalive := time.NewTicker(5 * time.Second)
	defer keepalive.Stop()

	inactivity := time.NewTimer(inactivityTimeout)
	defer inactivity.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-inactivity.C:
			return
		case <-keepalive.C:
			_ = launcher.EmitKeepaliveDotSafe(ctx)
		case evt, ok := <-events:
			if !ok {
				return
			}
			if !inactivity.Stop() {
				select {
				case <-inactivity.C:
				default:
				}
			}
			inactivity.Reset(inactivityTimeout)

			if evt.Type == ka.EventTypeSessionEnded {
				phase := mapReasonToPhase(evt.Phase)
				_ = launcher.EmitStatusWithMetaSafe(ctx,
					FormatEventForUser(evt),
					map[string]any{
						"type":     launcher.MetaTypeInvestigation,
						"phase":    phase,
						"reason":   evt.Phase,
						"terminal": true,
					})
				return
			}
			emitEventToA2A(ctx, evt, FormatEventForUser(evt))
			if evt.Type == ka.EventTypeComplete || evt.Type == ka.EventTypeCancelled {
				return
			}
		}
	}
}

// NonBlockingBridgeTTL bounds the maximum lifetime of a non-blocking bridge
// goroutine. This prevents goroutine leaks if KA never sends a terminal event.
// Matches the per-tool investigate timeout (15m) from production config.
var NonBlockingBridgeTTL = 15 * time.Minute

// BridgeInactivityTimeout is the maximum silence duration (no events from KA)
// before the bridge assumes the investigation is hung and returns whatever
// summary has been collected so far. Each received event resets the timer,
// so investigations of any wall-clock duration succeed as long as KA keeps
// producing events (token deltas, tool calls, keepalives).
// Exported so that tests can override it without modifying production code.
var BridgeInactivityTimeout = 60 * time.Second

// BridgeEventsCollectSummary is the exported entry point for bridgeEventsCollectSummary.
// It is used by integration tests and the blocking MCP investigation path.
func BridgeEventsCollectSummary(ctx context.Context, events <-chan ka.InvestigationEvent, inactivityTimeout time.Duration) (string, *InvestigateRCA) {
	return bridgeEventsCollectSummary(ctx, events, inactivityTimeout)
}

// bridgeEventsCollectSummary bridges events (same as BridgeEventsToA2A) and
// accumulates reasoning_delta text into a summary returned when the channel
// closes, the context is cancelled, or no events arrive within
// inactivityTimeout (hang detection).
func bridgeEventsCollectSummary(ctx context.Context, events <-chan ka.InvestigationEvent, inactivityTimeout time.Duration) (string, *InvestigateRCA) {
	var summary strings.Builder
	var rcaResult *InvestigateRCA
	keepalive := time.NewTicker(5 * time.Second)
	defer keepalive.Stop()
	inactivity := time.NewTimer(inactivityTimeout)
	defer inactivity.Stop()
	for {
		select {
		case <-ctx.Done():
			return summary.String(), rcaResult
		case <-inactivity.C:
			return summary.String(), rcaResult
		case <-keepalive.C:
			_ = launcher.EmitKeepaliveDotSafe(ctx)
		case evt, ok := <-events:
			if !ok {
				return summary.String(), rcaResult
			}
			inactivity.Reset(inactivityTimeout)
			// #1438: Handle session_ended before generic emit to avoid double-emit.
			if evt.Type == ka.EventTypeSessionEnded {
				phase := mapReasonToPhase(evt.Phase)
				_ = launcher.EmitStatusWithMetaSafe(ctx,
					FormatEventForUser(evt),
					map[string]any{
						"type":     launcher.MetaTypeInvestigation,
						"phase":    phase,
						"reason":   evt.Phase,
						"terminal": true,
					})
				return summary.String(), rcaResult
			}
			emitEventToA2A(ctx, evt, FormatEventForUser(evt))
			switch evt.Type {
			case ka.EventTypeReasoningDelta:
				if chunk := extractJSONField(evt.Data, "text"); chunk != "" {
					summary.WriteString(chunk)
				}
			case ka.EventTypeTokenDelta:
				if chunk := extractJSONField(evt.Data, "delta"); chunk != "" {
					summary.WriteString(chunk)
				}
			case ka.EventTypeAlignmentVerdict:
				if len(evt.Data) > 0 {
					var avr katypes.AlignmentVerdictResult
					if json.Unmarshal(evt.Data, &avr) == nil && avr.Result != "aligned" {
						meta := map[string]any{
							"type":  launcher.MetaTypeAlignmentCheckFailed,
							"rr_id": extractRRIDFromContext(ctx),
						}
						_ = launcher.EmitStructuredMetaSafe(ctx, string(evt.Data), meta)
					}
				}
			case ka.EventTypeComplete:
				if len(evt.Data) > 0 {
					var rca InvestigateRCA
					if json.Unmarshal(evt.Data, &rca) == nil && rca.Severity != "" {
						rcaResult = &rca
						if rca.RCASummary != "" && summary.Len() == 0 {
							summary.WriteString(rca.RCASummary)
						}
						emitEarlyRCA(ctx, &rca)
					}
				}
				return summary.String(), rcaResult
			case ka.EventTypeCancelled:
				return summary.String(), rcaResult
			}
		}
	}
}

// emitEarlyRCA emits a progressive RCA status-update via the EventBridge so
// the console can render investigation findings immediately (before workflow
// discovery completes). Uses metadata.type="decision" with schema="early_rca"
// to differentiate from the final present_decision artifact.
// FedRAMP: SI-4 (audit classification), AU-3 (content traceability).
func emitEarlyRCA(ctx context.Context, rca *InvestigateRCA) {
	if rca == nil {
		return
	}
	payload := fmt.Sprintf(
		`{"severity":"%s","confidence":%.2f,"target":"%s","rca_summary":"%s"}`,
		rca.Severity, rca.Confidence, rca.Target, rca.RCASummary,
	)
	meta := map[string]any{
		"type":           launcher.MetaTypeDecision,
		"schema":         "early_rca",
		"schema_version": "1.0",
	}
	_ = launcher.EmitStructuredMetaSafe(ctx, payload, meta)
}

// emitFallbackInvestigationArtifact emits an artifact-update event with the
// investigation_summary schema when the KA bridge produced no events but
// severity triage data is available. This ensures the Console gets a
// structured artifact even when the KA investigation is slow or unavailable.
// FedRAMP: SI-10 (data integrity through schema self-identification).
func emitFallbackInvestigationArtifact(ctx context.Context, rca *InvestigateRCA, rrID string) {
	if rca == nil {
		return
	}
	data := map[string]any{
		"session_id": rrID,
		"summary":    rca.RCASummary,
		"rca": map[string]any{
			"explanation": rca.RCASummary,
			"severity":    rca.Severity,
			"confidence":  rca.Confidence,
		},
	}
	meta := map[string]any{
		"type":           launcher.MetaTypeDecision,
		"schema":         "investigation_summary",
		"schema_version": "1.0",
	}
	_ = launcher.EmitArtifactSafe(ctx, data, fmt.Sprintf("Severity: %s (confidence %.0f%%)\n%s", rca.Severity, rca.Confidence*100, rca.RCASummary), meta)
}

// FormatEventForUser converts an investigation event into user-readable text.
// Returns empty string for event types that should not be shown to the user.
func FormatEventForUser(evt ka.InvestigationEvent) string {
	switch evt.Type {
	case ka.EventTypeReasoningDelta:
		return extractJSONField(evt.Data, "text")
	case ka.EventTypeTokenDelta:
		return extractJSONField(evt.Data, "delta")
	case ka.EventTypeToolCallStart:
		toolName := extractJSONField(evt.Data, "tool")
		if toolName != "" {
			return "Calling " + toolName + "..."
		}
		return ""
	case ka.EventTypeError:
		errMsg := extractJSONField(evt.Data, "error")
		if errMsg != "" {
			return "Error: " + security.RedactError(fmt.Errorf("%s", errMsg))
		}
		return "Investigation error occurred"
	case ka.EventTypeComplete:
		return "Investigation complete."
	case ka.EventTypeSessionEnded:
		reason := evt.Phase
		if reason == "" {
			reason = "unknown"
		}
		return "Session ended: " + reason
	case ka.EventTypeAlignmentVerdict:
		return ""
	default:
		return ""
	}
}

// isStatusEvent returns true for event types that should be routed to the
// A2A status channel (TaskStatusUpdateEvent) rather than the artifact channel.
// LLM-generated content (reasoning_delta, token_delta) belongs on the artifact
// stream. Orchestration updates (tool_call_start, complete, cancelled) and
// errors belong on the status channel as ephemeral messages (AC-4).
func isStatusEvent(evtType string) bool {
	switch evtType {
	case ka.EventTypeToolCallStart, ka.EventTypeComplete, ka.EventTypeCancelled, ka.EventTypeError, ka.EventTypeAlignmentVerdict, ka.EventTypeSessionEnded:
		return true
	default:
		return false
	}
}

// emitEventToA2A routes a formatted event text to the correct A2A channel
// based on the event type: status channel for orchestration events, artifact
// channel for LLM content. Write failures are logged by the Safe helpers (AU-2).
func emitEventToA2A(ctx context.Context, evt ka.InvestigationEvent, text string) {
	if text == "" {
		return
	}
	if isStatusEvent(evt.Type) {
		_ = launcher.EmitStatusSafe(ctx, text)
	} else {
		_ = launcher.EmitReasoningSafe(ctx, text)
	}
}

// WatchTerminalEvents watches a residual event channel for a session_ended
// event after pool inject. When received, it emits a terminal
// TaskStatusUpdateEvent to the A2A queue via the EventBridge in ctx.
// Exits deterministically on: session_ended received, events closed, or
// done closed (pool Release/EvictIdle/DrainAll).  No timer-based safety net.
// #1438, SI-4.
func WatchTerminalEvents(ctx context.Context, events <-chan ka.InvestigationEvent, rrID string, done <-chan struct{}) {
	for {
		select {
		case evt, ok := <-events:
			if !ok {
				return
			}
			if evt.Type == ka.EventTypeSessionEnded {
				emitWatcherTerminal(ctx, evt)
				return
			}
		case <-done:
			// Priority drain: a session_ended may already be buffered when
			// the pool fires onRelease. Drain it before exiting (#1438).
			select {
			case evt, ok := <-events:
				if ok && evt.Type == ka.EventTypeSessionEnded {
					emitWatcherTerminal(ctx, evt)
				}
			default:
			}
			return
		}
	}
}

func emitWatcherTerminal(ctx context.Context, evt ka.InvestigationEvent) {
	phase := mapReasonToPhase(evt.Phase)
	launcher.UpdatePhaseSafe(ctx, phase)
	_ = launcher.EmitStatusWithMetaSafe(ctx,
		FormatEventForUser(evt),
		map[string]any{
			"type":     launcher.MetaTypeInvestigation,
			"phase":    phase,
			"reason":   evt.Phase,
			"terminal": true,
		})
}

func mapReasonToPhase(reason string) string {
	switch reason {
	case "inactivity_timeout", "ttl_expired":
		return "TimedOut"
	case "disconnect":
		return "Disconnected"
	default:
		return reason
	}
}

// extractJSONField extracts a string field from a JSON RawMessage.
func extractJSONField(data json.RawMessage, field string) string {
	if len(data) == 0 {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	if v, ok := m[field].(string); ok {
		return v
	}
	return ""
}

// MonitorRegistry tracks active investigation sessions and their cleanup
// functions. It provides lifecycle management for background goroutines.
type MonitorRegistry struct {
	mu       sync.Mutex
	sessions map[string]func()
}

// NewMonitorRegistry creates a new empty monitor registry.
func NewMonitorRegistry() *MonitorRegistry {
	return &MonitorRegistry{
		sessions: make(map[string]func()),
	}
}

// Register adds a session to the registry with its closer function.
func (r *MonitorRegistry) Register(sessionID string, closer func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[sessionID] = closer
}

// Deregister removes a session from the registry without calling its closer.
// Safe to call if the session is not registered.
func (r *MonitorRegistry) Deregister(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, sessionID)
}

// Active returns true if the session is tracked in the registry.
func (r *MonitorRegistry) Active(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.sessions[sessionID]
	return ok
}

// Stop calls the closer for the session and removes it from the registry.
func (r *MonitorRegistry) Stop(sessionID string) {
	r.mu.Lock()
	closer, ok := r.sessions[sessionID]
	if ok {
		delete(r.sessions, sessionID)
	}
	r.mu.Unlock()

	if ok && closer != nil {
		closer()
	}
}

// StopAll calls all closers and clears the registry.
func (r *MonitorRegistry) StopAll() {
	r.mu.Lock()
	sessions := r.sessions
	r.sessions = make(map[string]func())
	r.mu.Unlock()

	for _, closer := range sessions {
		if closer != nil {
			closer()
		}
	}
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
func NewInvestigateMCPTool(mcpClient ka.MCPClient, client crclient.Client, namespace string, auditor audit.Emitter, registry *MonitorRegistry, onStarted SessionStartedHook, pool *ka.KASessionPool, signaler ISSignaler, triager *severity.Triager) (tool.Tool, error) {
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
		return HandleInvestigationMCPWithRegistry(ctx, mcpClient, client, namespace, args, auditor, registry, onStarted, true, pool, user, signaler, triager)
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
