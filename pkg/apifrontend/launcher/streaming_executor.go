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

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

// StreamingExecutor wraps an AgentExecutor to inject an EventBridge into the
// execution context. This enables tool handlers (e.g., kubernaut_stream_investigation)
// to emit progressive reasoning artifacts directly to the A2A event queue.
type StreamingExecutor struct {
	inner         a2asrv.AgentExecutor
	auditor       audit.Emitter
	bridgeMetrics BridgeMetrics
}

// NewStreamingExecutor creates a StreamingExecutor that wraps the given executor.
func NewStreamingExecutor(inner a2asrv.AgentExecutor, auditor audit.Emitter, m BridgeMetrics) *StreamingExecutor {
	return &StreamingExecutor{inner: inner, auditor: auditor, bridgeMetrics: m}
}

// Execute injects the EventBridge into the context and delegates to the inner executor.
// Emits a2a.stream_opened / a2a.stream_closed audit events (AU-6 compliance).
func (s *StreamingExecutor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	ctx = WithEventBridge(ctx, queue, reqCtx.TaskID, s.bridgeMetrics)

	s.emitLifecycleEvent(ctx, reqCtx, audit.EventA2AStreamOpened)

	err := s.inner.Execute(ctx, reqCtx, queue)

	detail := map[string]string{"task_id": string(reqCtx.TaskID)}
	if err != nil {
		detail["error"] = "true"
	}
	s.emitLifecycleAudit(ctx, reqCtx, audit.EventA2AStreamClosed, detail)

	return err
}

// Cancel delegates directly to the inner executor.
func (s *StreamingExecutor) Cancel(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	return s.inner.Cancel(ctx, reqCtx, queue)
}

// Cleanup delegates to the inner executor if it implements AgentExecutionCleaner.
func (s *StreamingExecutor) Cleanup(ctx context.Context, reqCtx *a2asrv.RequestContext, result a2a.SendMessageResult, err error) {
	if cleaner, ok := s.inner.(a2asrv.AgentExecutionCleaner); ok {
		cleaner.Cleanup(ctx, reqCtx, result, err)
	}
}

func (s *StreamingExecutor) emitLifecycleEvent(ctx context.Context, reqCtx *a2asrv.RequestContext, eventType audit.EventType) {
	s.emitLifecycleAudit(ctx, reqCtx, eventType, map[string]string{"task_id": string(reqCtx.TaskID)})
}

func (s *StreamingExecutor) emitLifecycleAudit(ctx context.Context, _ *a2asrv.RequestContext, eventType audit.EventType, detail map[string]string) {
	if s.auditor == nil {
		return
	}
	user := auth.UserIdentityFromContext(ctx)
	username := ""
	if user != nil {
		username = user.Username
	}
	s.auditor.Emit(ctx, &audit.Event{
		Type:   eventType,
		UserID: username,
		Detail: detail,
	})
}
