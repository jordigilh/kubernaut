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
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"

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
//
// The fourth return value (#2071, forward-port of release/v1.5's #2034 Stage
// 3b) is the last AlignmentVerdictResult seen (nil if none arrived), letting
// the caller wire KA's optional #1096 shadow-agent verdict into
// InvestigateMCPResult -- previously this was only used locally to fire the
// human-facing SSE alignment_check_failed notification and then discarded,
// leaving phase_guard.go's #2047 grounding guard with no way to consult it.
func BridgeEventsCollectSummary(ctx context.Context, events <-chan ka.InvestigationEvent, inactivityTimeout time.Duration) (string, *InvestigateRCA, string, *katypes.AlignmentVerdictResult) {
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
func bridgeEventsCollectSummary(ctx context.Context, events <-chan ka.InvestigationEvent, inactivityTimeout time.Duration) (string, *InvestigateRCA, string, *katypes.AlignmentVerdictResult) {
	var summary strings.Builder
	var rcaResult *InvestigateRCA
	var verdictResult *katypes.AlignmentVerdictResult
	keepalive := time.NewTicker(5 * time.Second)
	defer keepalive.Stop()
	inactivity := time.NewTimer(inactivityTimeout)
	defer inactivity.Stop()
	for {
		select {
		case <-ctx.Done():
			return summary.String(), rcaResult, ExitReasonCtxCancelled, verdictResult
		case <-inactivity.C:
			return summary.String(), rcaResult, ExitReasonInactivityTimeout, verdictResult
		case <-keepalive.C:
			_ = launcher.EmitKeepaliveDotSafe(ctx)
		case evt, ok := <-events:
			if !ok {
				return summary.String(), rcaResult, ExitReasonChannelClosed, verdictResult
			}
			inactivity.Reset(inactivityTimeout)
			if done, exitReason := processBridgeEvent(ctx, evt, &summary, &rcaResult, &verdictResult); done {
				return summary.String(), rcaResult, exitReason, verdictResult
			}
		}
	}
}

// processBridgeEvent handles a single investigation event: emits it to A2A,
// accumulates streamed text into summary, and captures the RCA result when
// the investigation completes. Returns done=true (with the terminal exit
// reason) when bridgeEventsCollectSummary's loop should stop.
func processBridgeEvent(ctx context.Context, evt ka.InvestigationEvent, summary *strings.Builder, rcaResult **InvestigateRCA, verdictResult **katypes.AlignmentVerdictResult) (bool, string) {
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
		captureAlignmentVerdict(ctx, evt, verdictResult)
	case ka.EventTypeComplete:
		captureCompleteEventRCA(ctx, evt, summary, rcaResult)
		return true, ExitReasonChannelClosed
	case ka.EventTypeCancelled:
		return true, ExitReasonChannelClosed
	}
	return false, ""
}

// captureAlignmentVerdict parses evt's AlignmentVerdictResult payload and
// stores it in verdictResult for the caller (#2071, forward-port of
// release/v1.5's #2034 Stage 3b) -- previously this only fired the
// human-facing SSE alignment_check_failed notification and discarded the
// verdict, leaving phase_guard.go's #2047 grounding guard with no way to
// consult it (investigateHasGroundedContent). The SSE notification itself is
// unchanged: only emitted when the verdict is not "aligned".
func captureAlignmentVerdict(ctx context.Context, evt ka.InvestigationEvent, verdictResult **katypes.AlignmentVerdictResult) {
	if len(evt.Data) == 0 {
		return
	}
	var avr katypes.AlignmentVerdictResult
	if json.Unmarshal(evt.Data, &avr) != nil {
		return
	}
	*verdictResult = &avr
	if avr.Result == "aligned" {
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

// EmitFallbackInvestigationArtifact is the exported entry point for
// emitFallbackInvestigationArtifact. It is used by unit tests to verify the
// artifact's content in isolation, without driving the full
// HandleInvestigationMCPWithRegistry wiring path.
func EmitFallbackInvestigationArtifact(ctx context.Context, rca *InvestigateRCA, rrID string) {
	emitFallbackInvestigationArtifact(ctx, rca, rrID)
}

// fallbackCausalChainPlaceholder is the truthful, non-fabricating
// causal_chain entry used when a fallback InvestigateRCA carries no real
// causal chain (severity-triage-only data). It must never describe a
// specific root cause that was not actually observed (AU-3).
const fallbackCausalChainPlaceholder = "Full investigation in progress; preliminary severity assessed from resource metadata only"

// emitFallbackInvestigationArtifact emits an artifact-update event with the
// investigation_summary schema for any concluded investigation, whether rca
// is KA's own genuine result or a severity-triage-only value synthesized by
// the caller when KA produced none (e.g. user-driving mode with no
// autonomous session, or KA slow/unavailable). Despite the name (kept for
// compatibility with existing call sites/tests), this is the single
// terminal producer of the investigation_summary artifact for BOTH cases as
// of #2247 -- it is not itself a "fallback-only" code path; SI-10 requires
// every concluded investigation to leave behind this structured record.
//
// The emitted rca object always includes causal_chain/tool_calls_count/
// llm_turns using the same field names as the final present_decision
// artifact's RCAData (ka_tools.go), even when rca carries no real causal
// chain yet: an empty causal_chain leaves the Console's hasRCAData render
// guard (AgentBubble.tsx) permanently false, silently dropping the fallback
// message instead of showing a renderable RCA card (#1922).
// FedRAMP: AU-3 (truthful content, no fabricated findings), SI-10 (data
// integrity through schema self-identification consistent with the final
// artifact's shape).
func emitFallbackInvestigationArtifact(ctx context.Context, rca *InvestigateRCA, rrID string) {
	if rca == nil {
		return
	}
	causalChain := rca.CausalChain
	if len(causalChain) == 0 {
		causalChain = []string{fallbackCausalChainPlaceholder}
	}
	data := map[string]any{
		"session_id": rrID,
		"summary":    rca.RCASummary,
		"rca": map[string]any{
			"explanation":      rca.RCASummary,
			"severity":         rca.Severity,
			"confidence":       rca.Confidence,
			"causal_chain":     causalChain,
			"tool_calls_count": rca.TotalToolCalls,
			"llm_turns":        rca.TotalLLMTurns,
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
		// #2090 (main port of #2089): KA's real emission (investigator's
		// emitToSink calls, e.g. runLLMLoop's tool-dispatch block) uses key
		// "tool_name" -- this previously read "tool", a wire-format
		// mismatch AF's own unit tests never caught because they
		// hand-constructed fixtures with the wrong key instead of
		// exercising the real KA emission path.
		toolName := extractJSONField(evt.Data, "tool_name")
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
			reason = unknownValue
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
//
// EventTypeReasoningContentDelta has its own branch, checked before the
// generic text=="" guard below: a redacted turn (#1716, DD-LLM-009 redaction
// sub-decision revisited) carries redacted=true with empty text on the wire,
// and must still emit a content-free live signal rather than no-op, so
// Console can render a "reasoning hidden by provider" placeholder.
func emitEventToA2A(ctx context.Context, evt ka.InvestigationEvent, text string) {
	if evt.Type == ka.EventTypeReasoningContentDelta {
		redacted := extractJSONBool(evt.Data, "redacted")
		if text == "" && !redacted {
			return
		}
		// #1635 / DD-LLM-009: dedicated channel, kept distinct from the
		// EmitReasoningSafe path used by orchestration narration below.
		_ = launcher.EmitReasoningContentSafe(ctx, text, redacted)
		return
	}
	if text == "" {
		return
	}
	switch {
	case isStatusEvent(evt.Type):
		_ = launcher.EmitStatusSafe(ctx, text)
	default:
		_ = launcher.EmitReasoningSafe(ctx, text)
	}
}

// watchTerminalEventsSafetyNetNs bounds how long WatchTerminalEvents will
// wait for a terminal event before giving up (#2094, v1.6 clone #2095). It
// is deliberately much larger than KA's own inactivity timeout (~10m
// default) or DefaultSessionTTL (30m per ka/session_pool.go) so it never
// fires for a legitimately long-running session -- it only bounds the case
// where the expected session_ended/done signal is truly lost (a pool
// onRelease callback that never fires, or a dropped events channel). Stored
// as an atomic.Int64 (nanoseconds) rather than a plain var: WatchTerminalEvents
// reads it from a newly-spawned goroutine with no happens-before
// relationship to a test's setup/teardown, so a plain var would be a data
// race under -race whenever a test overrides it via
// SetWatchTerminalEventsSafetyNetForTest while a previously-spawned watcher
// goroutine is still starting up.
var watchTerminalEventsSafetyNetNs atomic.Int64

func init() {
	watchTerminalEventsSafetyNetNs.Store(int64(30 * time.Minute))
}

// SetWatchTerminalEventsSafetyNetForTest overrides the WatchTerminalEvents
// safety-net timeout for the duration of a test. Test-only. Returns a
// restore function that must be deferred to reset the previous value.
func SetWatchTerminalEventsSafetyNetForTest(d time.Duration) (restore func()) {
	prev := watchTerminalEventsSafetyNetNs.Swap(int64(d))
	return func() { watchTerminalEventsSafetyNetNs.Store(prev) }
}

// WatchTerminalEvents watches a residual event channel for events arriving
// on a pooled MCP session after handoff. When a session_ended event is
// received, it emits a terminal TaskStatusUpdateEvent to the A2A queue via
// the EventBridge in ctx and exits. Exits deterministically on:
// session_ended received, events closed, done closed (pool
// Release/EvictIdle/DrainAll), or the safety-net timer
// (watchTerminalEventsSafetyNetNs) elapsing (#2094, v1.6 clone #2095). The
// safety-net timer is created once, before the loop, so a steady stream of
// non-terminal events cannot indefinitely postpone the deadline.
//
// relay is non-nil only for entries handed off via KASessionPool.InjectVerified
// (#1637/DD-AF-009). When a pooled call (kubernaut_message,
// discover_workflows, select_workflow, complete_no_action) is in flight,
// relay.Current() returns that call's ctx; every event — terminal or not —
// is then relayed live to that ctx's EventBridge instead of (for
// non-terminal events) being dropped, or (for the terminal event) landing
// on the watcher's own detached ctx. When idle (relay is nil or
// relay.Current() is nil), behavior is unchanged from #1438: non-terminal
// events are dropped, and the terminal event uses the watcher's own ctx.
func WatchTerminalEvents(ctx context.Context, events <-chan ka.InvestigationEvent, rrID string, done <-chan struct{}, relay *ka.EventRelay) {
	safetyNetTimeout := time.Duration(watchTerminalEventsSafetyNetNs.Load())
	safetyNet := time.NewTimer(safetyNetTimeout)
	defer safetyNet.Stop()

	for {
		select {
		case evt, ok := <-events:
			if !ok {
				return
			}
			if evt.Type == ka.EventTypeSessionEnded {
				emitWatcherTerminal(relayCtxOrDefault(relay, ctx), evt)
				return
			}
			relayLiveEvent(relay, evt) //nolint:contextcheck // relayLiveEvent deliberately ignores the watcher's own ctx and sources relay.Current() instead -- the whole point is to relay onto whichever pooled call's ctx is currently in flight, not the watcher's detached one
		case <-done:
			drainBufferedTerminalEvent(relayCtxOrDefault(relay, ctx), events)
			return
		case <-safetyNet.C:
			logr.FromContextOrDiscard(ctx).Info(
				"WatchTerminalEvents: safety-net timeout reached without a terminal event; exiting to prevent goroutine leak",
				"rr_id", rrID, "timeout", safetyNetTimeout)
			return
		}
	}
}

// relayLiveEvent forwards a non-terminal event to whichever pooled call's
// ctx is currently attached to relay. Only relays while a pooled call is
// actually in flight (relay.Current() != nil); when idle, the event is
// silently dropped — unchanged from #1438, since the watcher's own detached
// ctx was never meant to receive live business content, only the terminal
// session_ended signal.
func relayLiveEvent(relay *ka.EventRelay, evt ka.InvestigationEvent) {
	if relay == nil {
		return
	}
	if liveCtx := relay.Current(); liveCtx != nil {
		emitEventToA2A(liveCtx, evt, FormatEventForUser(evt))
	}
}

// drainBufferedTerminalEvent performs a non-blocking check for a
// session_ended event that may already be buffered when the pool fires
// onRelease, emitting it before the watcher exits (#1438).
func drainBufferedTerminalEvent(ctx context.Context, events <-chan ka.InvestigationEvent) {
	select {
	case evt, ok := <-events:
		if ok && evt.Type == ka.EventTypeSessionEnded {
			emitWatcherTerminal(ctx, evt)
		}
	default:
	}
}

// relayCtxOrDefault returns relay.Current() when a pooled call is currently
// in flight, otherwise fallbackCtx (the watcher's own detached ctx). Safe to
// call with a nil relay (#1637).
func relayCtxOrDefault(relay *ka.EventRelay, fallbackCtx context.Context) context.Context {
	if relay == nil {
		return fallbackCtx
	}
	if live := relay.Current(); live != nil {
		return live
	}
	return fallbackCtx
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

// extractJSONBool extracts a boolean field from a JSON RawMessage. Used by
// emitEventToA2A (#1716) to read KA's "redacted" flag on a
// reasoning_content_delta event without changing FormatEventForUser's
// string-only return type.
func extractJSONBool(data json.RawMessage, field string) bool {
	if len(data) == 0 {
		return false
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	v, _ := m[field].(bool)
	return v
}
