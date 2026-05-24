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
	"unicode"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
)

type contextKey struct{}

// BridgeMetrics collects metrics for the EventBridge write path.
type BridgeMetrics interface {
	IncBridgeEvents()
	IncBridgeWriteFailures()
}

// EventBridge enables tool handlers to emit progressive A2A events (reasoning
// artifacts) directly to the A2A event queue during execution. This bridges KA
// SSE investigation events into the A2A stream without waiting for the tool to
// return a FunctionResponse.
type EventBridge struct {
	queue   eventqueue.Writer
	taskID  a2a.TaskID
	metrics BridgeMetrics
}

const maxBridgeTextLen = 512

// WithEventBridge attaches an EventBridge to the context, enabling
// downstream tool handlers to emit progressive reasoning artifacts.
func WithEventBridge(ctx context.Context, queue eventqueue.Writer, taskID a2a.TaskID, m BridgeMetrics) context.Context {
	bridge := &EventBridge{queue: queue, taskID: taskID, metrics: m}
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
// truncated to maxBridgeTextLen to prevent flooding. The artifact uses
// Append=true so it extends the existing narrative rather than replacing.
func (b *EventBridge) EmitReasoning(ctx context.Context, text string) error {
	text = sanitizeBridgeText(text)
	if text == "" {
		return nil
	}

	event := &a2a.TaskArtifactUpdateEvent{
		TaskID: b.taskID,
		Append: true,
		Artifact: &a2a.Artifact{
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
// in the given context. If no bridge is present, it's a no-op.
func EmitReasoningSafe(ctx context.Context, text string) error {
	bridge := EventBridgeFromContext(ctx)
	if bridge == nil {
		return nil
	}
	return bridge.EmitReasoning(ctx, text)
}
