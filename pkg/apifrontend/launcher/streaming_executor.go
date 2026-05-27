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

	"github.com/go-logr/logr"
	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

// SessionPhaseUpdater provides the subset of session.CRDSessionService needed
// by StreamingExecutor for disconnect detection (BR-SESS-003, SI-4).
type SessionPhaseUpdater interface {
	IsMaterialized(sessionID string) bool
	UpdatePhase(ctx context.Context, sessionID string, to isv1alpha1.SessionPhase, message, userID string) error
}

// StreamingExecutor wraps an AgentExecutor to inject an EventBridge into the
// execution context. This enables tool handlers (e.g., kubernaut_stream_investigation)
// to emit progressive reasoning artifacts directly to the A2A event queue.
type StreamingExecutor struct {
	inner         a2asrv.AgentExecutor
	logger        logr.Logger
	bridgeMetrics BridgeMetrics
	sessionSvc    SessionPhaseUpdater
}

// NewStreamingExecutor creates a StreamingExecutor that wraps the given executor.
func NewStreamingExecutor(inner a2asrv.AgentExecutor, logger logr.Logger, m BridgeMetrics, spu SessionPhaseUpdater) *StreamingExecutor {
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	return &StreamingExecutor{inner: inner, logger: logger, bridgeMetrics: m, sessionSvc: spu}
}

// Execute injects the EventBridge into the context and delegates to the inner executor.
// Stream lifecycle is logged (not audited) because a2a.stream_opened/closed lack
// OpenAPI payload schemas in data-storage-v1.yaml. The A2A task lifecycle is
// already audited by buildBeforeExecuteCallback / buildAfterExecuteCallback.
func (s *StreamingExecutor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	ctx = WithEventBridge(ctx, queue, reqCtx.TaskID, reqCtx.ContextID, s.bridgeMetrics)

	user := auth.UserIdentityFromContext(ctx)
	username := ""
	if user != nil {
		username = user.Username
	}
	s.logger.Info("a2a stream opened",
		"task_id", string(reqCtx.TaskID),
		"user", username,
	)

	err := s.inner.Execute(ctx, reqCtx, queue)

	s.logger.Info("a2a stream closed",
		"task_id", string(reqCtx.TaskID),
		"user", username,
		"error", err != nil,
	)

	// BR-SESS-003 / SI-4: On client SSE disconnect, transition materialized
	// sessions to Disconnected phase so tracker slots are released promptly
	// and the CRD reflects the actual connection state.
	//
	// The a2a-go library runs executors in a detached context
	// (context.WithoutCancel), so ctx.Err() won't reflect SSE disconnects.
	// We stored the original HTTP request context as a value (values survive
	// WithoutCancel). Go's net/http cancels r.Context() when the client's
	// connection closes — before ServeHTTP returns — making it a reliable
	// disconnect signal even from within the detached goroutine.
	sseCtx := SSEDisconnectCtxFromContext(ctx)
	disconnected := ctx.Err() == context.Canceled ||
		(sseCtx != nil && sseCtx.Err() == context.Canceled)

	if disconnected && s.sessionSvc != nil {
		// Use reqCtx.ContextID directly as the session ID. The
		// BeforeExecuteCallback injects CreateContext inside the inner
		// executor's scope, so session.CreateContextFromContext(ctx)
		// would return nil here. reqCtx.ContextID is the A2A context ID
		// that the ADK maps 1:1 to the session ID.
		sessionID := reqCtx.ContextID
		if sessionID != "" && s.sessionSvc.IsMaterialized(sessionID) {
			if uerr := s.sessionSvc.UpdatePhase(
				context.Background(), sessionID,
				isv1alpha1.SessionPhaseDisconnected,
				"client SSE disconnect", username,
			); uerr != nil {
				s.logger.Error(uerr, "failed to transition session to Disconnected on SSE disconnect",
					"session_id", sessionID,
				)
			} else {
				s.logger.Info("session transitioned to Disconnected on SSE disconnect",
					"session_id", sessionID,
				)
			}
		}
	}

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
