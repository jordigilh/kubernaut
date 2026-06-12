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
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
)

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

	MetaTypeApprovalRequest         = "approval_request"
	MetaTypeApprovalRequestResolved = "approval_request_resolved"
)

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
}

const maxBridgeTextLen = 512

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
// for LLM inner thoughts and investigation reasoning deltas. The text is
// sanitized (control characters stripped, secrets redacted) and truncated.
func (b *EventBridge) EmitReasoning(ctx context.Context, text string) error {
	return b.EmitStatusWithMeta(ctx, text, map[string]any{"type": MetaTypeReasoning})
}

// EmitOutput writes a TaskStatusUpdateEvent with metadata.type="output" for
// final LLM response content (the markdown answer). Renderers display this
// as primary content while reasoning/status events are secondary.
func (b *EventBridge) EmitOutput(ctx context.Context, text string) error {
	return b.EmitStatusWithMeta(ctx, text, map[string]any{"type": MetaTypeOutput})
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
	now := time.Now().UTC()
	event := &a2a.TaskStatusUpdateEvent{
		TaskID:    b.taskID,
		ContextID: b.contextID,
		Status: a2a.TaskStatus{
			State:     a2a.TaskStateWorking,
			Timestamp: &now,
		},
		Metadata: map[string]any{
			"type": "keepalive",
			"dot":  ".",
		},
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
// SC-7 boundary protection (secret redaction), and length truncation.
func sanitizeBridgeText(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)

	text = security.RedactText(text)

	if len([]rune(text)) > maxBridgeTextLen {
		text = string([]rune(text)[:maxBridgeTextLen]) + "..."
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
func (b *EventBridge) EmitArtifact(ctx context.Context, data map[string]any, textFallback string, meta map[string]any) error {
	if b == nil || b.queue == nil {
		return nil
	}

	parts := make(a2a.ContentParts, 0, 2)
	if data != nil {
		parts = append(parts, a2a.DataPart{
			Data:     data,
			Metadata: map[string]any{"mediaType": "application/json"},
		})
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
