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
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"k8s.io/client-go/dynamic"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

// isPhaseActivePollTimeout caps the IS phase Active polling after AIA readiness.
// Short because the phase transition should follow almost immediately after AA submits.
const isPhaseActivePollTimeout = 5 * time.Second

// InvestigateMCPArgs defines the input for the MCP-based kubernaut_investigate tool.
type InvestigateMCPArgs struct {
	RRID string `json:"rr_id"`
}

// InvestigateMCPResult is the output of the MCP investigate tool.
type InvestigateMCPResult struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Summary   string `json:"summary,omitempty"`
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
	return HandleInvestigationMCPWithRegistry(ctx, mcpClient, k8sClient, namespace, args, auditor, nil, nil, false)
}

// HandleInvestigationMCPWithRegistry is like HandleInvestigationMCP but also
// registers the session in a MonitorRegistry for lifecycle management and
// invokes onStarted (if provided) to create the IS CRD after a successful start.
//
// When blocking is true, the function waits for the investigation to complete
// (or ctx cancellation) and returns the collected summary in InvestigateMCPResult.
// Events are still streamed to the A2A SSE via EmitReasoningSafe during the wait.
// When blocking is false, a background goroutine bridges events and the function
// returns immediately (legacy behavior for the MCP bridge path).
func HandleInvestigationMCPWithRegistry(ctx context.Context, mcpClient ka.MCPClient, k8sClient dynamic.Interface, namespace string, args InvestigateMCPArgs, auditor audit.Emitter, registry *MonitorRegistry, onStarted SessionStartedHook, blocking bool) (InvestigateMCPResult, error) {
	if args.RRID == "" {
		return InvestigateMCPResult{}, fmt.Errorf("rr_id is required for MCP investigation")
	}

	// Wait for AIA CRD to show a session ID, confirming AA has submitted
	// to KA with interactive=true and KA has created a pending session.
	// Blocking path uses a longer timeout because the IS CRD (created by
	// af_create_rr) needs time for AA to detect and resubmit to KA.
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
		// Short timeout: by the time AIA is ready, the IS phase transition
		// should follow almost immediately.
		isCtx, isCancel := context.WithTimeout(ctx, isPhaseActivePollTimeout)
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
		}, nil
	}

	if blocking {
		// Synchronous: bridge events inline, collecting the summary.
		// Events stream to kagenti via EmitReasoningSafe during the wait.
		defer cleanup()
		logger.Info("bridgeEventsCollectSummary: starting blocking event bridge",
			"rr_id", args.RRID, "session_id", result.SessionID, "ctx_err", ctx.Err())
		summary := bridgeEventsCollectSummary(ctx, result.Events)
		status := "completed"
		if ctx.Err() != nil {
			status = "timeout"
			logger.Info("bridgeEventsCollectSummary: context cancelled/timed out",
				"rr_id", args.RRID, "ctx_err", ctx.Err(), "summary_len", len(summary))
		}
		logger.Info("bridgeEventsCollectSummary: finished",
			"rr_id", args.RRID, "status", status, "summary_len", len(summary))
		return InvestigateMCPResult{
			SessionID: result.SessionID,
			Status:    status,
			Summary:   summary,
		}, nil
	}

	// Non-blocking: spawn background goroutine for MCP bridge path.
	go func() {
		defer cleanup()
		BridgeEventsToA2A(ctx, result.Events)
	}()

	return InvestigateMCPResult{
		SessionID: result.SessionID,
		Status:    result.Status,
	}, nil
}

// BridgeEventsToA2A reads investigation events from the KA MCP session and
// emits filtered reasoning artifacts to the A2A stream. A keepalive is sent
// every 20s to prevent idle SSE timeouts during long tool executions.
func BridgeEventsToA2A(ctx context.Context, events <-chan ka.InvestigationEvent) {
	keepalive := time.NewTicker(5 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			_ = launcher.EmitReasoningSafe(ctx, "\nProcessing...\n")
		case evt, ok := <-events:
			if !ok {
				return
			}
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

// bridgeEventsCollectSummary bridges events (same as BridgeEventsToA2A) and
// accumulates reasoning_delta text into a summary returned when the channel
// closes or the context is cancelled. Used by the blocking A2A path so the
// LLM receives the full investigation results in the tool response.
func bridgeEventsCollectSummary(ctx context.Context, events <-chan ka.InvestigationEvent) string {
	var summary strings.Builder
	keepalive := time.NewTicker(5 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-ctx.Done():
			return summary.String()
		case <-keepalive.C:
			_ = launcher.EmitReasoningSafe(ctx, "\nProcessing...\n")
		case evt, ok := <-events:
			if !ok {
				return summary.String()
			}
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
			return "Error: " + errMsg
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
func NewInvestigateMCPTool(mcpClient ka.MCPClient, k8sClient dynamic.Interface, namespace string, auditor audit.Emitter, registry *MonitorRegistry, onStarted SessionStartedHook) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "kubernaut_investigate",
		Description: "Investigate an infrastructure incident via MCP. " +
			"Provide rr_id to start and run the full investigation. " +
			"This tool blocks until the investigation completes and returns " +
			"the root-cause analysis summary. Live progress events stream " +
			"to the user automatically while the investigation runs.",
	}, func(ctx tool.Context, args InvestigateMCPArgs) (InvestigateMCPResult, error) {
		return HandleInvestigationMCPWithRegistry(ctx, mcpClient, k8sClient, namespace, args, auditor, registry, onStarted, true)
	})
}
