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

// EventBridge enables tool handlers to emit progressive A2A events (reasoning
// artifacts) directly to the A2A event queue during execution. This bridges KA
// SSE investigation events into the A2A stream without waiting for the tool to
// return a FunctionResponse.
//
// The bridge manages a stable ArtifactID across emissions: the first emission
// creates a new artifact (Append=false) and captures the ID; subsequent
// emissions append to that artifact (Append=true) with the same ID. This
// satisfies the a2a-go taskupdate.Manager contract that requires a matching
// ArtifactID for append operations.
type EventBridge struct {
	mu         sync.Mutex
	queue      eventqueue.Writer
	taskID     a2a.TaskID
	contextID  string
	metrics    BridgeMetrics
	artifactID a2a.ArtifactID
	hasEmitted bool
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

// EmitReasoning writes a progressive reasoning artifact to the A2A event queue.
// The text is sanitized (control characters stripped, secrets redacted) and
// truncated to maxBridgeTextLen to prevent flooding.
//
// On the first call the bridge creates a new artifact (Append=false) with a
// generated ArtifactID. Subsequent calls append to that same artifact
// (Append=true) so the a2a-go taskupdate.Manager can locate the existing
// artifact by its ID.
//
// A leading newline is prepended when prior content exists so that each
// reasoning message renders on its own line after A2A clients (e.g. kagenti)
// strip trailing whitespace from individual text parts before concatenation.
func (b *EventBridge) EmitReasoning(ctx context.Context, text string) error {
	text = sanitizeBridgeText(text)
	if text == "" {
		return nil
	}

	b.mu.Lock()
	if b.hasEmitted {
		text = "\n" + text
	}
	b.hasEmitted = true
	err := b.emitArtifact(ctx, text)
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

func (b *EventBridge) emitArtifact(ctx context.Context, text string) error {
	isAppend := b.artifactID != ""
	if !isAppend {
		b.artifactID = a2a.NewArtifactID()
	}

	event := &a2a.TaskArtifactUpdateEvent{
		TaskID:    b.taskID,
		ContextID: b.contextID,
		Append:    isAppend,
		Artifact: &a2a.Artifact{
			ID: b.artifactID,
			Parts: []a2a.Part{
				&a2a.TextPart{Text: text},
			},
		},
	}
	err := b.queue.Write(ctx, event)
	if b.metrics != nil {
		if err != nil {
			b.metrics.IncBridgeWriteFailures()
		} else {
			b.metrics.IncBridgeEvents()
		}
	}
	return err
}

// EmitStatus writes a TaskStatusUpdateEvent to the A2A event queue for
// ephemeral progress/status messages (orchestration updates, tool call starts,
// keepalive dots). Status events are separate from the artifact stream and do
// not affect the artifact's hasEmitted / Append state.
func (b *EventBridge) EmitStatus(ctx context.Context, text string) error {
	text = sanitizeBridgeText(text)
	if text == "" {
		return nil
	}

	b.mu.Lock()
	err := b.emitStatusEvent(ctx, text)
	b.mu.Unlock()
	return err
}

func (b *EventBridge) emitStatusEvent(ctx context.Context, text string) error {
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
					&a2a.TextPart{Text: text},
				},
			},
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

// EmitReasoningSafe is a nil-safe helper that emits reasoning via the bridge
// in the given context. If no bridge is present, it's a no-op. Write failures
// are logged (AU-2) so callers can use fire-and-forget semantics without
// silently swallowing errors.
func EmitReasoningSafe(ctx context.Context, text string) error {
	bridge := EventBridgeFromContext(ctx)
	if bridge == nil {
		return nil
	}
	if err := bridge.EmitReasoning(ctx, text); err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "A2A bridge write failed", "channel", "artifact")
		return err
	}
	return nil
}

// EmitStatusSafe is a nil-safe helper that emits a status update via the
// bridge. If no bridge is present, it's a no-op. Write failures are logged
// (AU-2) so callers can use fire-and-forget semantics without silently
// swallowing errors.
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
