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
	"time"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

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
// bridgeCtx at the call site in HandleInvestigationMCPWithRegistry.
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
var BridgeInactivityTimeout = 180 * time.Second

// Exit reason constants returned as the third value from bridgeEventsCollectSummary.
const (
	ExitReasonInactivityTimeout = "inactivity_timeout"
	ExitReasonChannelClosed     = "channel_closed"
	ExitReasonCtxCancelled      = "ctx_cancelled"
)

// BridgeEventsCollectSummary is the exported entry point for bridgeEventsCollectSummary.
// It is used by integration tests and the blocking MCP investigation path.
func BridgeEventsCollectSummary(ctx context.Context, events <-chan ka.InvestigationEvent, inactivityTimeout time.Duration) (string, *InvestigateRCA, string) {
	return bridgeEventsCollectSummary(ctx, events, inactivityTimeout)
}

// ExitReasonToStatus maps the exit reason returned by bridgeEventsCollectSummary
// to a user-facing investigation status string.
func ExitReasonToStatus(exitReason string) string {
	switch exitReason {
	case ExitReasonInactivityTimeout:
		return "timed_out"
	case ExitReasonCtxCancelled:
		return "timeout"
	default:
		return "completed"
	}
}

// bridgeEventsCollectSummary bridges events (same as BridgeEventsToA2A) and
// accumulates reasoning_delta text into a summary returned when the channel
// closes, the context is cancelled, or no events arrive within
// inactivityTimeout (hang detection).
func bridgeEventsCollectSummary(ctx context.Context, events <-chan ka.InvestigationEvent, inactivityTimeout time.Duration) (string, *InvestigateRCA, string) {
	var summary strings.Builder
	var rcaResult *InvestigateRCA
	keepalive := time.NewTicker(5 * time.Second)
	defer keepalive.Stop()
	inactivity := time.NewTimer(inactivityTimeout)
	defer inactivity.Stop()
	for {
		select {
		case <-ctx.Done():
			return summary.String(), rcaResult, ExitReasonCtxCancelled
		case <-inactivity.C:
			return summary.String(), rcaResult, ExitReasonInactivityTimeout
		case <-keepalive.C:
			_ = launcher.EmitKeepaliveDotSafe(ctx)
		case evt, ok := <-events:
			if !ok {
				return summary.String(), rcaResult, ExitReasonChannelClosed
			}
			inactivity.Reset(inactivityTimeout)
			if done, exitReason := processBridgeEvent(ctx, evt, &summary, &rcaResult); done {
				return summary.String(), rcaResult, exitReason
			}
		}
	}
}

// processBridgeEvent handles a single investigation event: emits it to A2A,
// accumulates streamed text into summary, and captures the RCA result when
// the investigation completes. Returns done=true (with the terminal exit
// reason) when bridgeEventsCollectSummary's loop should stop.
func processBridgeEvent(ctx context.Context, evt ka.InvestigationEvent, summary *strings.Builder, rcaResult **InvestigateRCA) (bool, string) {
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
		return true, ExitReasonChannelClosed
	}
	emitEventToA2A(ctx, evt, FormatEventForUser(evt))
	// #1635 / DD-LLM-009: EventTypeReasoningContentDelta (genuine captured LLM
	// reasoning) is deliberately NOT accumulated into summary here, unlike
	// EventTypeReasoningDelta (orchestration narration). Raw model
	// deliberation must never leak into the final chat-answer/RCA summary
	// text shown to the operator; the live SSE channel (emitEventToA2A above)
	// and the audit trail are its only surfaces.
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
		emitAlignmentVerdictIfMisaligned(ctx, evt)
	case ka.EventTypeComplete:
		captureCompleteEventRCA(ctx, evt, summary, rcaResult)
		return true, ExitReasonChannelClosed
	case ka.EventTypeCancelled:
		return true, ExitReasonChannelClosed
	}
	return false, ""
}

// emitAlignmentVerdictIfMisaligned emits an alignment-check-failed event when
// evt carries a non-"aligned" AlignmentVerdictResult payload.
func emitAlignmentVerdictIfMisaligned(ctx context.Context, evt ka.InvestigationEvent) {
	if len(evt.Data) == 0 {
		return
	}
	var avr katypes.AlignmentVerdictResult
	if json.Unmarshal(evt.Data, &avr) != nil || avr.Result == "aligned" {
		return
	}
	meta := map[string]any{
		"type":  launcher.MetaTypeAlignmentCheckFailed,
		"rr_id": extractRRIDFromContext(ctx),
	}
	_ = launcher.EmitStructuredMetaSafe(ctx, string(evt.Data), meta)
}

// captureCompleteEventRCA parses the terminal "complete" event's RCA payload,
// stores it in rcaResult, seeds summary with the RCA text when no streamed
// text was accumulated, and emits the progressive early-RCA artifact.
func captureCompleteEventRCA(ctx context.Context, evt ka.InvestigationEvent, summary *strings.Builder, rcaResult **InvestigateRCA) {
	if len(evt.Data) == 0 {
		return
	}
	var rca InvestigateRCA
	if json.Unmarshal(evt.Data, &rca) != nil || rca.Severity == "" {
		return
	}
	*rcaResult = &rca
	if rca.RCASummary != "" && summary.Len() == 0 {
		summary.WriteString(rca.RCASummary)
	}
	emitEarlyRCA(ctx, &rca)
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
	case ka.EventTypeReasoningContentDelta:
		// #1635 / BR-AI-086 AC10: KA's wire payload is redaction-transparent
		// (empty text on a redacted turn) — extractJSONField naturally
		// returns "" in that case, and emitEventToA2A's text=="" guard
		// no-ops, matching EmitReasoning's existing empty-text behavior.
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
	switch {
	case isStatusEvent(evt.Type):
		_ = launcher.EmitStatusSafe(ctx, text)
	case evt.Type == ka.EventTypeReasoningContentDelta:
		// #1635 / DD-LLM-009: dedicated channel, kept distinct from the
		// EmitReasoningSafe path used by orchestration narration below.
		_ = launcher.EmitReasoningContentSafe(ctx, text)
	default:
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
