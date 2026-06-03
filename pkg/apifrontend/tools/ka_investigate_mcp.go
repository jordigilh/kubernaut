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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

// isPhaseActivePollTimeout caps the IS phase Active polling after AIA readiness.
// Short because the phase transition should follow almost immediately after AA submits.
const isPhaseActivePollTimeout = 5 * time.Second

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
// Either RRID (for an existing RR) or Namespace/Kind/Name (to create a new one)
// must be provided. When creating, an IS CRD is also created for the interactive flow.
type InvestigateMCPArgs struct {
	RRID      string `json:"rr_id,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Name      string `json:"name,omitempty"`
}

// InvestigateMCPResult is the output of the MCP investigate tool.
type InvestigateMCPResult struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Summary   string `json:"summary,omitempty"`
	RRID      string `json:"rr_id,omitempty"`
	Error     string `json:"error,omitempty"`
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
func HandleInvestigationMCP(ctx context.Context, mcpClient ka.MCPClient, k8sClient dynamic.Interface, namespace string, args InvestigateMCPArgs, auditor audit.Emitter) (InvestigateMCPResult, error) {
	return HandleInvestigationMCPWithRegistry(ctx, mcpClient, k8sClient, namespace, args, auditor, nil, nil, false, nil, "", nil, nil)
}

// HandleInvestigationMCPWithRegistry is like HandleInvestigationMCP but also
// registers the session in a MonitorRegistry for lifecycle management and
// invokes onStarted (if provided) to create the IS CRD after a successful start.
//
// When blocking is true, the function waits for the investigation to complete
// (or ctx cancellation) and returns the collected summary in InvestigateMCPResult.
// Events are still streamed to the A2A SSE via EmitReasoningSafe during the wait.
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
func HandleInvestigationMCPWithRegistry(ctx context.Context, mcpClient ka.MCPClient, k8sClient dynamic.Interface, namespace string, args InvestigateMCPArgs, auditor audit.Emitter, registry *MonitorRegistry, onStarted SessionStartedHook, blocking bool, pool *ka.KASessionPool, username string, signaler ISSignaler, triager *severity.Triager) (InvestigateMCPResult, error) {
	if mcpClient == nil {
		return InvestigateMCPResult{}, fmt.Errorf("KA MCP client unavailable")
	}

	hasRRID := args.RRID != ""
	hasResourceArgs := args.Namespace != "" || args.Kind != "" || args.Name != ""

	if !hasRRID && !hasResourceArgs {
		return InvestigateMCPResult{}, fmt.Errorf("rr_id or namespace/kind/name required")
	}

	if hasRRID {
		if err := validate.RRID(args.RRID); err != nil {
			return InvestigateMCPResult{}, fmt.Errorf("invalid rr_id: %w", err)
		}
	}

	identity := auth.UserIdentityFromContext(ctx)

	if !hasRRID && hasResourceArgs {
		if args.Kind == "" || args.Name == "" {
			return InvestigateMCPResult{}, fmt.Errorf("kind and name required when providing namespace/kind/name")
		}
		if args.Namespace == "" {
			return InvestigateMCPResult{}, fmt.Errorf("rr_id or namespace/kind/name required")
		}

		if identity != nil && identity.IsServiceAccount {
			return InvestigateMCPResult{}, fmt.Errorf("interactive investigation cannot be started by service accounts")
		}

		if k8sClient == nil {
			return InvestigateMCPResult{}, fmt.Errorf("k8s client unavailable for RR creation")
		}

		createArgs := &CreateRRArgs{
			Namespace: args.Namespace,
			Kind:      args.Kind,
			Name:      args.Name,
		}
		createUser := ""
		if identity != nil {
			createUser = identity.Username
		}
		result, err := HandleCreateRR(ctx, k8sClient, namespace, createArgs, createUser, triager, auditor)
		if err != nil {
			return InvestigateMCPResult{}, fmt.Errorf("create RR for investigation: %w", err)
		}
		args.RRID = result.RRID
	}

	if args.RRID == "" {
		return InvestigateMCPResult{}, fmt.Errorf("rr_id is required for MCP investigation")
	}

	// Determine if this RR already has an autonomous investigation running.
	// When signaler is available, create IS CRD BEFORE the await loop to signal
	// interactive intent to AA (DD-INTERACTIVE-002, BR-INTERACTIVE-010).
	var isCRDName string
	if signaler != nil && k8sClient != nil && namespace != "" {
		joinMode := "start"
		if isAutonomousInvestigation(ctx, k8sClient, namespace, args.RRID) {
			joinMode = "takeover"
			_ = launcher.EmitReasoningSafe(ctx, "Autonomous investigation detected, signaling takeover...")
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
	if k8sClient != nil && namespace != "" {
		awaitTimeout := 10 * time.Second
		if blocking {
			awaitTimeout = 60 * time.Second
		}
		checkCtx, checkCancel := context.WithTimeout(ctx, awaitTimeout)
		awaitResult, awaitErr := HandleAwaitSession(checkCtx, k8sClient, AwaitSessionArgs{
			Namespace: namespace,
			RRName:    args.RRID,
		})
		checkCancel()
		if awaitErr == nil && awaitResult.Status == "ready" {
			_ = launcher.EmitReasoningSafe(ctx, "Investigation session ready, connecting to KA...")
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
		if AwaitISPhaseActive(isCtx, k8sClient, namespace, args.RRID) {
			_ = launcher.EmitReasoningSafe(ctx, "Interactive session acknowledged by AA, starting investigation...")
		}
		isCancel()
	}

	logger := logr.FromContextOrDiscard(ctx)
	logger.Info("StartInvestigation: calling MCP client", "rr_id", args.RRID, "ctx_err", ctx.Err())

	result, err := mcpClient.StartInvestigation(ctx, ka.StartInvestigationArgs{
		RRID: args.RRID,
	})
	if err != nil {
		if strings.Contains(err.Error(), "session_active") {
			driver := extractDriverFromSessionActiveError(err)
			logger.Info("session_active from KA: returning structured result instead of error",
				"rr_id", args.RRID, "driver", driver)
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
			_ = launcher.EmitReasoningSafe(ctx, fmt.Sprintf("Warning: IS CRD creation failed (%v), investigation continues", hookErr))
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
		summary := bridgeEventsCollectSummary(ctx, result.Events, BridgeInactivityTimeout)
		status := "completed"
		if ctx.Err() != nil {
			status = "timeout"
			logger.Info("bridgeEventsCollectSummary: context cancelled",
				"rr_id", args.RRID, "ctx_err", ctx.Err(), "summary_len", len(summary))
		}
		logger.Info("bridgeEventsCollectSummary: finished",
			"rr_id", args.RRID, "status", status, "summary_len", len(summary))

		// Hand off the MCP session to the pool so discover_workflows /
		// select_workflow reuse the same connection and driver lease.
		// If no pool is available, fall back to closing the session.
		if pool != nil && result.Session != nil && username != "" {
			pool.Inject(args.RRID, username, result.Session)
			if registry != nil {
				registry.Deregister(result.SessionID)
			}
			logger.Info("investigation session handed off to pool",
				"rr_id", args.RRID, "session_id", result.SessionID, "username", username)
		} else {
			cleanup()
		}

		return InvestigateMCPResult{
			SessionID: result.SessionID,
			Status:    status,
			Summary:   summary,
			RRID:      args.RRID,
		}, nil
	}

	// Non-blocking: spawn background goroutine for MCP bridge path.
	// Detach from the tool context (which is cancelled by wrapTool's defer cancel()
	// on handler return) and use an explicit investigation TTL to bound the bridge
	// lifetime. Without this, the bridge goroutine would exit immediately when the
	// handler returns.
	bridgeCtx, bridgeCancel := context.WithTimeout(context.WithoutCancel(ctx), NonBlockingBridgeTTL)
	go func() {
		defer bridgeCancel()
		defer cleanup()
		BridgeEventsToA2A(bridgeCtx, result.Events)
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
func BridgeEventsToA2A(ctx context.Context, events <-chan ka.InvestigationEvent) {
	keepalive := time.NewTicker(5 * time.Second)
	defer keepalive.Stop()

	inactivity := time.NewTimer(BridgeInactivityTimeout)
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
			inactivity.Reset(BridgeInactivityTimeout)

			text := FormatEventForUser(evt)
			if text != "" {
				_ = launcher.EmitReasoningSafe(ctx, text)
			}
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

// bridgeEventsCollectSummary bridges events (same as BridgeEventsToA2A) and
// accumulates reasoning_delta text into a summary returned when the channel
// closes, the context is cancelled, or no events arrive within
// inactivityTimeout (hang detection).
func bridgeEventsCollectSummary(ctx context.Context, events <-chan ka.InvestigationEvent, inactivityTimeout time.Duration) string {
	var summary strings.Builder
	keepalive := time.NewTicker(5 * time.Second)
	defer keepalive.Stop()
	inactivity := time.NewTimer(inactivityTimeout)
	defer inactivity.Stop()
	for {
		select {
		case <-ctx.Done():
			return summary.String()
		case <-inactivity.C:
			return summary.String()
		case <-keepalive.C:
			_ = launcher.EmitKeepaliveDotSafe(ctx)
		case evt, ok := <-events:
			if !ok {
				return summary.String()
			}
			inactivity.Reset(inactivityTimeout)
			text := FormatEventForUser(evt)
			if text != "" {
				_ = launcher.EmitReasoningSafe(ctx, text)
			}
			switch evt.Type {
			case ka.EventTypeReasoningDelta:
				if chunk := extractJSONField(evt.Data, "text"); chunk != "" {
					summary.WriteString(chunk)
				}
			case ka.EventTypeTokenDelta:
				if chunk := extractJSONField(evt.Data, "delta"); chunk != "" {
					summary.WriteString(chunk)
				}
			}
			if evt.Type == ka.EventTypeComplete || evt.Type == ka.EventTypeCancelled {
				return summary.String()
			}
		}
	}
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
	default:
		return ""
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
// k8sClient and namespace enable AIA CRD polling before starting the
// investigation (BR-INTERACTIVE-010). Pass nil k8sClient to skip polling.
// registry is optional; when provided, sessions are tracked for graceful shutdown.
// onStarted is called after a successful start to create the IS CRD.
// pool is optional; when provided, the MCP session is handed off to the pool
// after the investigation so that discover_workflows / select_workflow reuse
// the same connection and driver lease.
func NewInvestigateMCPTool(mcpClient ka.MCPClient, k8sClient dynamic.Interface, namespace string, auditor audit.Emitter, registry *MonitorRegistry, onStarted SessionStartedHook, pool *ka.KASessionPool, signaler ISSignaler, triager *severity.Triager) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "kubernaut_investigate",
		Description: "Investigate an infrastructure incident via MCP. " +
			"Provide rr_id to start and run the full investigation. " +
			"This tool blocks until the investigation completes and returns " +
			"the root-cause analysis summary. Live progress events stream " +
			"to the user automatically while the investigation runs.",
	}, func(ctx tool.Context, args InvestigateMCPArgs) (InvestigateMCPResult, error) {
		user := usernameFromContext(ctx)
		return HandleInvestigationMCPWithRegistry(ctx, mcpClient, k8sClient, namespace, args, auditor, registry, onStarted, true, pool, user, signaler, triager)
	})
}

// isAutonomousInvestigation checks if the given RR has an active AIA CRD with
// a session ID already assigned (indicating autonomous investigation in progress).
// Returns true when a takeover is needed instead of a fresh start.
func isAutonomousInvestigation(ctx context.Context, client dynamic.Interface, namespace, rrName string) bool {
	if client == nil || namespace == "" || rrName == "" {
		return false
	}
	list, err := client.Resource(aianalysisGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false
	}
	for _, item := range list.Items {
		specRR, _, _ := unstructured.NestedString(item.Object, "spec", "remediationRequestRef", "name")
		if specRR != rrName {
			continue
		}
		sessionID, _, _ := unstructured.NestedString(item.Object, "status", "investigationSession", "id")
		if sessionID != "" {
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
