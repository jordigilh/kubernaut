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
package launcher

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
)

// #2110/#2111: a2a-go@v0.3.15's own internal/taskstore/store.go registers
// map[string]any and []any at init() too, but relying solely on that
// package being transitively imported by *something else* in this binary
// is fragile -- it depends on unrelated import-graph decisions elsewhere in
// this repo that have nothing to do with EmitArtifact's own correctness.
// Registering them here as well makes sanitizeArtifactData's gob-safety
// guarantee self-contained. gob.Register is idempotent, so duplicate
// registration (here and in a2a-go) is harmless.
func init() {
	gob.Register(map[string]any{})
	gob.Register([]any{})
}

type contextKey struct{}

// BridgeMetrics collects metrics for the EventBridge write path.
// Artifact and status channels have separate failure counters (SI-4)
// so operators can distinguish which channel is experiencing write failures.
type BridgeMetrics interface {
	IncBridgeEvents()
	IncBridgeWriteFailures()
	IncBridgeStatusEvents()
	IncBridgeStatusWriteFailures()
}

// Metadata type constants for TaskStatusUpdateEvent classification.
// Clients inspect metadata.type to style events differently (dimmed for
// reasoning, ephemeral for status, full render for output). Clients that
// ignore metadata still render every status.message as streaming text.
const (
	MetaTypeReasoning     = "reasoning"
	MetaTypeStatus        = "status"
	MetaTypeOutput        = "output"
	MetaTypeInvestigation = "investigation"
	MetaTypeKeepalive     = "keepalive"
	MetaTypeDecision      = "decision"

	MetaTypeVerificationStep = "verification_step"

	MetaTypeApprovalRequest         = "approval_request"
	MetaTypeApprovalRequestResolved = "approval_request_resolved"
	MetaTypeAlignmentCheckFailed    = "alignment_check_failed"
)

// RRContext holds RemediationRequest metadata that is merged into every status
// event emitted after SetRRContext is called. This enables Console banner
// population from the first status event after RR creation (#1423).
// Fields are server-sourced (validated at creation time per SI-10) and contain
// only identifiers (safe for boundary crossing per SC-7).
type RRContext struct {
	RRID      string `json:"rr_id"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Target    string `json:"target"`
	AlertName string `json:"alert_name"`
	Phase     string `json:"phase"`
}

// EventBridge enables tool handlers to emit progressive A2A events directly to
// the A2A event queue during execution. All streaming content is emitted as
// TaskStatusUpdateEvent with a metadata.type tag for semantic classification.
// This ensures A2A clients (Kagenti, Agent Stack, etc.) render streaming text
// correctly, since the A2A ecosystem convention routes streaming content through
// status-update events, not artifact-update events.
type EventBridge struct {
	mu        sync.Mutex
	queue     eventqueue.Writer
	taskID    a2a.TaskID
	contextID string
	metrics   BridgeMetrics
	rrCtx     *RRContext
}

const (
	maxBridgeTextLen    = 512
	maxReasoningTextLen = 4096
)

// SetRRContext stores RR metadata on the bridge so all subsequent status events
// include rr_id, namespace, kind, target, alert_name, and phase in their
// metadata map (AU-3 traceability, SI-4 monitoring, SC-7 Console association).
// Thread-safe: protected by the bridge mutex.
func (b *EventBridge) SetRRContext(rc *RRContext) {
	b.mu.Lock()
	b.rrCtx = rc
	b.mu.Unlock()
}

// UpdatePhase updates the phase field in the stored RR context so subsequent
// status events reflect the current lifecycle phase (SI-4). No-op if
// SetRRContext has not been called.
func (b *EventBridge) UpdatePhase(phase string) {
	b.mu.Lock()
	if b.rrCtx != nil {
		b.rrCtx.Phase = phase
	}
	b.mu.Unlock()
}

// mergeRRContext copies RR context fields into meta without overwriting
// caller-supplied keys (SI-10: caller metadata takes precedence).
// Must be called under the bridge mutex.
func (b *EventBridge) mergeRRContext(meta map[string]any) map[string]any {
	if b.rrCtx == nil {
		return meta
	}
	if meta == nil {
		meta = map[string]any{}
	}
	fields := map[string]string{
		"rr_id":      b.rrCtx.RRID,
		"namespace":  b.rrCtx.Namespace,
		"kind":       b.rrCtx.Kind,
		"target":     b.rrCtx.Target,
		"alert_name": b.rrCtx.AlertName,
		"phase":      b.rrCtx.Phase,
	}
	for k, v := range fields {
		if v == "" {
			continue
		}
		if _, exists := meta[k]; !exists {
			meta[k] = v
		}
	}
	return meta
}

// SetRRContextSafe sets RR context on the EventBridge from context. Nil-safe:
// no-op when no bridge is present. Used by tool handlers after RR creation.
func SetRRContextSafe(ctx context.Context, rc *RRContext) {
	bridge := EventBridgeFromContext(ctx)
	if bridge == nil {
		return
	}
	bridge.SetRRContext(rc)
}

// UpdatePhaseSafe updates the phase on the EventBridge from context. Nil-safe.
func UpdatePhaseSafe(ctx context.Context, phase string) {
	bridge := EventBridgeFromContext(ctx)
	if bridge == nil {
		return
	}
	bridge.UpdatePhase(phase)
}

// WithEventBridge attaches an EventBridge to the context, enabling
// downstream tool handlers to emit progressive reasoning artifacts.
func WithEventBridge(ctx context.Context, queue eventqueue.Writer, taskID a2a.TaskID, contextID string, m BridgeMetrics) context.Context {
	bridge := &EventBridge{queue: queue, taskID: taskID, contextID: contextID, metrics: m}
	return context.WithValue(ctx, contextKey{}, bridge)
}

// EventBridgeFromContext retrieves the EventBridge from the context.
// Returns nil if not in a streaming A2A execution.
func EventBridgeFromContext(ctx context.Context) *EventBridge {
	v := ctx.Value(contextKey{})
	if v == nil {
		return nil
	}
	bridge, _ := v.(*EventBridge)
	return bridge
}

// EmitReasoning writes a TaskStatusUpdateEvent with metadata.type="reasoning"
// for LLM inner thoughts and investigation reasoning deltas. Uses a 4096-rune
// limit (vs. 512 for ephemeral status) since reasoning can be multi-paragraph.
// #1435: raised from 512 to prevent mid-sentence truncation in the Console.
func (b *EventBridge) EmitReasoning(ctx context.Context, text string) error {
	return b.emitWithLimit(ctx, text, maxReasoningTextLen, map[string]any{"type": MetaTypeReasoning})
}

// EmitOutput writes a TaskStatusUpdateEvent with metadata.type="output" for
// final LLM response content (the markdown answer). Uses a 4096-rune limit.
// #1435: raised from 512 to prevent truncation of final answers.
func (b *EventBridge) EmitOutput(ctx context.Context, text string) error {
	return b.emitWithLimit(ctx, text, maxReasoningTextLen, map[string]any{"type": MetaTypeOutput})
}

// emitWithLimit sanitizes text with a caller-specified rune limit and emits
// a TaskStatusUpdateEvent. Used by EmitReasoning/EmitOutput (4096 runes) to
// differentiate from EmitStatus/EmitStatusWithMeta (512 runes).
func (b *EventBridge) emitWithLimit(ctx context.Context, text string, maxLen int, meta map[string]any) error {
	text = sanitizeTextWithLimit(ctx, text, maxLen)
	if text == "" {
		return nil
	}

	b.mu.Lock()
	err := b.emitStatusEventWithMeta(ctx, text, meta)
	b.mu.Unlock()
	return err
}

// EmitKeepaliveDot writes a metadata-only TaskStatusUpdateEvent to prevent
// proxy/gateway idle timeouts. The event carries no Status.Message (avoiding
// Task.History pollution in the a2a-go taskupdate.Manager) and instead tags the
// dot in Metadata for renderers: {"type":"keepalive", "dot":"."}.
func (b *EventBridge) EmitKeepaliveDot(ctx context.Context) error {
	b.mu.Lock()
	err := b.emitKeepaliveEvent(ctx)
	b.mu.Unlock()
	return err
}

func (b *EventBridge) emitKeepaliveEvent(ctx context.Context) error {
	meta := b.mergeRRContext(map[string]any{
		"type": "keepalive",
		"dot":  ".",
	})
	now := time.Now().UTC()
	event := &a2a.TaskStatusUpdateEvent{
		TaskID:    b.taskID,
		ContextID: b.contextID,
		Status: a2a.TaskStatus{
			State:     a2a.TaskStateWorking,
			Timestamp: &now,
		},
		Metadata: meta,
	}
	err := b.queue.Write(ctx, event)
	if b.metrics != nil {
		if err != nil {
			b.metrics.IncBridgeStatusWriteFailures()
		} else {
			b.metrics.IncBridgeStatusEvents()
		}
	}
	return err
}

// EmitStatus writes a TaskStatusUpdateEvent with metadata.type="status" for
// ephemeral progress messages (orchestration updates, tool call starts).
func (b *EventBridge) EmitStatus(ctx context.Context, text string) error {
	return b.EmitStatusWithMeta(ctx, text, map[string]any{"type": MetaTypeStatus})
}

// EmitStatusWithMeta writes a TaskStatusUpdateEvent with caller-supplied
// metadata. The text is sanitized and the metadata.type field controls how
// A2A clients classify and render the event.
func (b *EventBridge) EmitStatusWithMeta(ctx context.Context, text string, meta map[string]any) error {
	text = sanitizeBridgeText(text)
	if text == "" {
		return nil
	}

	b.mu.Lock()
	err := b.emitStatusEventWithMeta(ctx, text, meta)
	b.mu.Unlock()
	return err
}

func (b *EventBridge) emitStatusEventWithMeta(ctx context.Context, text string, meta map[string]any) error {
	meta = b.mergeRRContext(meta)
	now := time.Now().UTC()
	event := &a2a.TaskStatusUpdateEvent{
		TaskID:    b.taskID,
		ContextID: b.contextID,
		Status: a2a.TaskStatus{
			State:     a2a.TaskStateWorking,
			Timestamp: &now,
			Message: &a2a.Message{
				Role: a2a.MessageRoleAgent,
				Parts: []a2a.Part{
					a2a.TextPart{Text: text},
				},
			},
		},
		Metadata: meta,
	}
	err := b.queue.Write(ctx, event)
	if b.metrics != nil {
		if err != nil {
			b.metrics.IncBridgeStatusWriteFailures()
		} else {
			b.metrics.IncBridgeStatusEvents()
		}
	}
	return err
}

// sanitizeBridgeText applies SI-10 input validation (control char stripping),
// SC-7 boundary protection (secret redaction), and length truncation at 512 runes.
// Used for ephemeral status messages ("Connecting to KA...") which are short by nature.
func sanitizeBridgeText(text string) string {
	return sanitizeTextWithLimit(context.Background(), text, maxBridgeTextLen)
}

// sanitizeTextWithLimit applies SI-10 control-char stripping, SC-7 secret
// redaction, and length truncation at the specified rune limit. Logs when
// truncation occurs so operators can monitor payload sizing (#1435).
func sanitizeTextWithLimit(ctx context.Context, text string, maxLen int) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)

	text = security.RedactText(text)

	runeCount := len([]rune(text))
	if runeCount > maxLen {
		logr.FromContextOrDiscard(ctx).V(1).Info("bridge text truncated",
			"original_runes", runeCount,
			"max_runes", maxLen,
			"truncated_runes", runeCount-maxLen,
		)
		text = string([]rune(text)[:maxLen]) + "..."
	}
	return text
}

const maxStructuredPayloadLen = 8192

// EmitStructuredMeta writes a TaskStatusUpdateEvent for structured JSON payloads
// (decision cards, remediation output) that must NOT be truncated at 512 runes.
// Applies SI-10 control-char sanitization and SC-7 secret redaction, but skips
// length truncation. Rejects payloads exceeding 8KB with a fallback status
// message and metric increment (defense-in-depth).
func (b *EventBridge) EmitStructuredMeta(ctx context.Context, text string, meta map[string]any) error {
	text = sanitizeStructuredText(text)
	if text == "" {
		return nil
	}
	if len(text) > maxStructuredPayloadLen {
		if b.metrics != nil {
			b.metrics.IncBridgeWriteFailures()
		}
		logr.FromContextOrDiscard(ctx).Error(nil, "structured payload exceeds size limit",
			"len", len(text), "max", maxStructuredPayloadLen)
		return b.EmitStatus(ctx, "Decision payload too large to render.\n\n")
	}
	b.mu.Lock()
	err := b.emitStatusEventWithMeta(ctx, text, meta)
	b.mu.Unlock()
	return err
}

// sanitizeStructuredText applies SI-10 control-char stripping and SC-7 secret
// redaction WITHOUT length truncation. Used for structured JSON payloads that
// must preserve their full content for machine parsing.
func sanitizeStructuredText(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)
	return security.RedactText(text)
}

// EmitReasoningSafe is a nil-safe helper that emits reasoning via the bridge.
// If no bridge is present, it's a no-op. Write failures are logged (AU-2).
func EmitReasoningSafe(ctx context.Context, text string) error {
	bridge := EventBridgeFromContext(ctx)
	if bridge == nil {
		return nil
	}
	if err := bridge.EmitReasoning(ctx, text); err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "A2A bridge write failed", "channel", "reasoning")
		return err
	}
	return nil
}

// EmitOutputSafe is a nil-safe helper that emits final LLM output via the
// bridge. If no bridge is present, it's a no-op. Write failures are logged.
func EmitOutputSafe(ctx context.Context, text string) error {
	bridge := EventBridgeFromContext(ctx)
	if bridge == nil {
		return nil
	}
	if err := bridge.EmitOutput(ctx, text); err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "A2A bridge write failed", "channel", "output")
		return err
	}
	return nil
}

// EmitStatusSafe is a nil-safe helper that emits a status update via the
// bridge. If no bridge is present, it's a no-op. Write failures are logged.
func EmitStatusSafe(ctx context.Context, text string) error {
	bridge := EventBridgeFromContext(ctx)
	if bridge == nil {
		return nil
	}
	if err := bridge.EmitStatus(ctx, text); err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "A2A bridge write failed", "channel", "status")
		return err
	}
	return nil
}

// EmitStatusWithMetaSafe is a nil-safe helper that emits a status update with
// caller-supplied metadata via the bridge. If no bridge is present, it's a
// no-op. Write failures are logged (AU-2). Used by HandleWatch to emit
// phase-level metadata (stabilization_window, validity_deadline) on Verifying.
func EmitStatusWithMetaSafe(ctx context.Context, text string, meta map[string]any) error {
	bridge := EventBridgeFromContext(ctx)
	if bridge == nil {
		return nil
	}
	if err := bridge.EmitStatusWithMeta(ctx, text, meta); err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "A2A bridge write failed", "channel", "status")
		return err
	}
	return nil
}

// EmitKeepaliveDotSafe is a nil-safe helper that emits a keepalive dot via
// the bridge. If no bridge is present, it's a no-op. Write failures are
// logged (AU-2) so callers can use fire-and-forget semantics without silently
// swallowing errors.
func EmitKeepaliveDotSafe(ctx context.Context) error {
	bridge := EventBridgeFromContext(ctx)
	if bridge == nil {
		return nil
	}
	if err := bridge.EmitKeepaliveDot(ctx); err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "A2A bridge write failed", "channel", "keepalive")
		return err
	}
	return nil
}

// EmitStructuredMetaSafe is a nil-safe helper that emits structured JSON via
// the bridge. If no bridge is present, it's a no-op. Write failures are logged.
func EmitStructuredMetaSafe(ctx context.Context, text string, meta map[string]any) error {
	bridge := EventBridgeFromContext(ctx)
	if bridge == nil {
		return nil
	}
	if err := bridge.EmitStructuredMeta(ctx, text, meta); err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "A2A bridge write failed", "channel", "structured")
		return err
	}
	return nil
}

// EmitArtifactSafe is a nil-safe helper that emits a TaskArtifactUpdateEvent
// via the bridge. If no bridge is present, it's a no-op. Write failures are logged.
func EmitArtifactSafe(ctx context.Context, data map[string]any, textFallback string, meta map[string]any) error {
	bridge := EventBridgeFromContext(ctx)
	if bridge == nil {
		return nil
	}
	if err := bridge.EmitArtifact(ctx, data, textFallback, meta); err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "A2A bridge write failed", "channel", "artifact")
		return err
	}
	return nil
}

// stripEmoji removes Unicode emoji codepoints from the input string.
// Targets emoji presentation sequences while preserving valid Unicode
// (math symbols, currency, CJK, accented Latin, arrows, box drawing).
func stripEmoji(s string) string {
	return strings.Map(func(r rune) rune {
		if isEmoji(r) {
			return -1
		}
		return r
	}, s)
}

// EmitArtifact writes a TaskArtifactUpdateEvent with multi-part content:
// Part[0] = DataPart (structured JSON), Part[1] = TextPart (human-readable fallback).
// This is the A2A v1.0-compliant way to deliver structured results to clients.
//
// EmitArtifact is the single choke point every artifact producer in this
// package (present_decision's emitDecisionEvent, crd_tools.go's progress
// snapshot, ka_investigate_mcp.go's RCA artifact) shares before data
// reaches a2a-go@v0.3.15's task manager, which gob-encodes every artifact
// for its internal deep-copy fan-out (#2110/#2111: a *tools.RCAData struct
// pointer reaching that pipeline unregistered crashed every grounded
// present_decision call in v1.5.6-rc4 with "gob: type not registered for
// interface: tools.RCAData"). #2112's fix on this branch closed that one
// call site by changing its return type, but nothing stopped a *future*
// caller from reintroducing the identical crash by assigning any other
// struct into any nested field of an artifact's data map.
// sanitizeArtifactData below closes that gap here, at the boundary every
// caller shares, instead of relying on each caller to remember the
// map[string]any convention.
func (b *EventBridge) EmitArtifact(ctx context.Context, data map[string]any, textFallback string, meta map[string]any) error {
	if b == nil || b.queue == nil {
		return nil
	}

	parts := make(a2a.ContentParts, 0, 2)
	if data != nil {
		safe, err := sanitizeArtifactData(data)
		if err != nil {
			logr.FromContextOrDiscard(ctx).Error(err, "artifact data is not gob-safe for a2a-go's deep-copy pipeline -- "+
				"dropping structured DataPart and falling back to text-only artifact (#2110/#2111 boundary guard)")
			if b.metrics != nil {
				b.metrics.IncBridgeWriteFailures()
			}
		} else {
			parts = append(parts, a2a.DataPart{
				Data:     safe,
				Metadata: map[string]any{"mediaType": "application/json"},
			})
		}
	}
	parts = append(parts, a2a.TextPart{Text: textFallback})

	evt := &a2a.TaskArtifactUpdateEvent{
		TaskID:    b.taskID,
		ContextID: b.contextID,
		Artifact: &a2a.Artifact{
			ID:       a2a.NewArtifactID(),
			Parts:    parts,
			Metadata: meta,
		},
		LastChunk: true,
	}

	b.mu.Lock()
	err := b.queue.Write(ctx, evt)
	b.mu.Unlock()

	if b.metrics != nil {
		if err != nil {
			b.metrics.IncBridgeStatusWriteFailures()
		} else {
			b.metrics.IncBridgeStatusEvents()
		}
	}
	return err
}

// sanitizeArtifactData guarantees data can survive a2a-go@v0.3.15's task
// manager's gob-encoded deep-copy fan-out (#2110/#2111 -- see EmitArtifact's
// doc comment for full context), regardless of what any current or future
// artifact producer puts into an interface{}-typed field.
//
// The fast path probes data with the REAL encoding/gob encoder (gobProbe)
// rather than a hand-maintained list of "safe" types, so it stays correct
// if Go's or a2a-go's own registered type set ever changes. Data that is
// already gob-safe -- which is every existing caller in this package,
// since map[string]any/[]any/string/bool/nil/float64 and Go's own
// gob-builtin slice types ([]string, []int, etc., see encoding/gob's own
// type.go init()) all pass -- is returned completely unchanged, byte-for-
// byte and type-for-type. This matters: an earlier version of this fix
// naively JSON-round-tripped every call unconditionally, which silently
// coerced already-compliant []string/int values into []any/float64 and
// broke existing callers asserting on the original Go types (regression
// caught by UT-AF-1922-001/002 and UT-AF-WIRE-SESSION-003 in
// pkg/apifrontend/tools) -- exactly the kind of new, avoidable breakage a
// boundary hardening fix must not introduce.
//
// Only when gobProbe finds something actually unsafe (e.g. a bare struct
// value/pointer, exactly #2110's mistake) does this fall back to a JSON
// marshal/unmarshal round-trip, which normalizes the offending value into
// the same map[string]any/[]any/primitive shape every compliant caller in
// this package already produces by convention -- turning that convention
// into a structural guarantee instead of something merely documented.
func sanitizeArtifactData(data map[string]any) (map[string]any, error) {
	if gobProbe(data) == nil {
		return data, nil
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("artifact data is not JSON-marshalable: %w", err)
	}
	var safe map[string]any
	if err := json.Unmarshal(raw, &safe); err != nil {
		return nil, fmt.Errorf("artifact data failed JSON round-trip: %w", err)
	}
	if err := gobProbe(safe); err != nil {
		return nil, fmt.Errorf("artifact data remains gob-unsafe after JSON normalization: %w", err)
	}
	return safe, nil
}

// gobProbe reports whether v can be gob-encoded through an interface{}
// value, using a scratch encoder discarded after one use so probing never
// has observable side effects (gob.Encoder accumulates per-instance type
// definitions across calls, purely as a wire-size optimization, but that
// state never escapes this function).
func gobProbe(v any) error {
	return gob.NewEncoder(io.Discard).Encode(v)
}

// isEmoji returns true if the rune is a Unicode emoji codepoint.
func isEmoji(r rune) bool {
	switch {
	case r >= 0x1F600 && r <= 0x1F64F: // Emoticons
		return true
	case r >= 0x1F300 && r <= 0x1F5FF: // Misc Symbols and Pictographs
		return true
	case r >= 0x1F680 && r <= 0x1F6FF: // Transport and Map
		return true
	case r >= 0x1F900 && r <= 0x1F9FF: // Supplemental Symbols and Pictographs
		return true
	case r >= 0x1FA00 && r <= 0x1FA6F: // Chess Symbols
		return true
	case r >= 0x1FA70 && r <= 0x1FAFF: // Symbols and Pictographs Extended-A
		return true
	case r >= 0x2600 && r <= 0x26FF: // Misc symbols (sun, cloud, etc.)
		return true
	case r >= 0x2700 && r <= 0x27BF: // Dingbats
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // Variation Selectors
		return true
	case r >= 0x200D && r <= 0x200D: // Zero Width Joiner
		return true
	case r == 0x20E3: // Combining Enclosing Keycap
		return true
	case r >= 0x2300 && r <= 0x23FF: // Misc Technical (includes hourglass, keyboard, etc.)
		return true
	case r >= 0x2B50 && r <= 0x2B55: // Stars and circles
		return true
	}
	return false
}
