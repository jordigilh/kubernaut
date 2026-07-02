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

package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-faster/jx"
	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/jordigilh/kubernaut/pkg/agentclient"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// InvestigationRunner abstracts the investigation entry point so that
// decorators (e.g. alignment.InvestigatorWrapper) can wrap it transparently.
type InvestigationRunner interface {
	Investigate(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error)
}

// Handler implements the ogen-generated Handler interface for the 3 business
// endpoints. Operational endpoints (/healthz, /readyz, /config, /metrics) are
// served directly by the HTTP mux in cmd/kubernautagent/main.go.
type Handler struct {
	agentclient.UnimplementedHandler

	sessions     *session.Manager
	investigator InvestigationRunner
	logger       logr.Logger
	metrics      *metrics.Metrics
}

var _ agentclient.Handler = (*Handler)(nil)

// NewHandler creates a Kubernaut Agent ogen handler.
// metrics may be nil (all metric calls are nil-safe per OPS-1).
func NewHandler(sessions *session.Manager, inv InvestigationRunner, logger logr.Logger, m *metrics.Metrics) *Handler {
	return &Handler{
		sessions:     sessions,
		investigator: inv,
		logger:       logger,
		metrics:      m,
	}
}

// IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost implements POST /api/v1/incident/analyze.
// Returns HTTP 202 with {"session_id": "<uuid>"}.
func (h *Handler) IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(
	ctx context.Context, req *agentclient.IncidentRequest,
) (agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostRes, error) {
	if req.RemediationID == "" {
		return &agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostUnprocessableEntityApplicationProblemJSON{
			Type:     "https://kubernaut.ai/problems/validation-error",
			Title:    "Validation Error",
			Detail:   "remediation_id is required (DD-WORKFLOW-002)",
			Status:   422,
			Instance: "/api/v1/incident/analyze",
		}, nil
	}

	if req.IncidentID == "" {
		return &agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostUnprocessableEntityApplicationProblemJSON{
			Type:     "https://kubernaut.ai/problems/validation-error",
			Title:    "Validation Error",
			Detail:   "incident_id is required",
			Status:   422,
			Instance: "/api/v1/incident/analyze",
		}, nil
	}

	if h.investigator == nil {
		h.logger.Error(nil, "investigator not configured")
		return &agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostInternalServerErrorApplicationProblemJSON{
			Type:     "https://kubernaut.ai/problems/internal-error",
			Title:    "Internal Server Error",
			Detail:   "investigator not configured",
			Status:   500,
			Instance: "/api/v1/incident/analyze",
		}, nil
	}

	signal := MapIncidentRequestToSignal(req)
	actor := auth.GetUserFromContext(ctx)
	h.logger.Info("investigation submitted",
		"incident_id", req.IncidentID,
		"signal", signal.Name,
		"namespace", signal.Namespace,
		"actor", actor,
	)

	sctx := session.SessionContext{
		IncidentID:    req.IncidentID,
		RemediationID: req.RemediationID,
		Signal:        signal,
	}
	investigateFn := func(bgCtx context.Context) (*katypes.InvestigationResult, error) {
		bgCtx = audit.WithActor(bgCtx, actor, "User")
		return h.investigator.Investigate(bgCtx, signal)
	}

	var (
		sessionID string
		err       error
	)
	if signal.Interactive {
		sessionID, err = h.sessions.StartInteractiveSessionWithContext(ctx, investigateFn, sctx)
	} else {
		sessionID, err = h.sessions.StartInvestigationWithContext(ctx, investigateFn, sctx)
	}
	if err != nil {
		if errors.Is(err, session.ErrMaxInvestigationsReached) {
			h.logger.Info("investigation rejected: max concurrent investigations reached")
			return &agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostInternalServerErrorApplicationProblemJSON{
				Type:     "https://kubernaut.ai/problems/capacity-exhausted",
				Title:    "Capacity Exhausted",
				Detail:   "maximum concurrent investigations reached, retry later",
				Status:   500,
				Instance: "/api/v1/incident/analyze",
			}, nil
		}
		h.logger.Error(err, "failed to start investigation")
		return &agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostInternalServerErrorApplicationProblemJSON{
			Type:     "https://kubernaut.ai/problems/internal-error",
			Title:    "Internal Server Error",
			Detail:   "failed to start investigation",
			Status:   500,
			Instance: "/api/v1/incident/analyze",
		}, nil
	}

	sid, err := uuid.Parse(sessionID)
	if err != nil {
		h.logger.Error(err, "invalid session ID format", "session_id", sessionID)
		return &agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostApplicationJSONInternalServerError{
			Type:     "https://kubernaut.ai/problems/internal-error",
			Title:    "Internal Server Error",
			Detail:   "invalid session ID",
			Status:   500,
			Instance: "/api/v1/incident/analyze",
		}, nil
	}
	return &agentclient.AnalyzeAccepted{SessionID: sid}, nil
}

// IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet implements GET /api/v1/incident/session/{session_id}.
func (h *Handler) IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(
	ctx context.Context,
	params agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams,
) (agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetRes, error) {
	endpoint := fmt.Sprintf("/api/v1/incident/session/%s", params.SessionID)
	sess, err := h.getAuthorizedSession(ctx, params.SessionID, endpoint)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return &agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetNotFound{
				Type:     "https://kubernaut.ai/problems/not-found",
				Title:    "Session Not Found",
				Detail:   fmt.Sprintf("session %s not found", params.SessionID),
				Status:   404,
				Instance: endpoint,
			}, nil
		}
		// Design: log+return-generic is intentional at handler boundaries (SOC2
		// CC8.1). The logger captures the root cause for operators; the client
		// receives a sanitized message with no internal details. This is NOT
		// double handling (#52) — it is the boundary contract.
		h.logger.Error(err, "session lookup failed", "session_id", params.SessionID)
		return nil, errors.New("internal server error")
	}

	status := mapSessionStatusToAPI(sess.Status)
	resp := &agentclient.SessionStatus{
		SessionID: sess.ID,
		Status:    status,
	}
	if sess.Status == session.StatusUserDriving && sess.Metadata != nil {
		if u := sess.Metadata["acting_user"]; u != "" {
			resp.ActingUser = agentclient.NewOptString(u)
		}
		if raw := sess.Metadata["acting_user_groups"]; raw != "" {
			var groups []string
			if err := json.Unmarshal([]byte(raw), &groups); err == nil {
				resp.ActingUserGroups = groups
			}
		}
	}
	return resp, nil
}

// IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet implements
// GET /api/v1/incident/session/{session_id}/result.
func (h *Handler) IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(
	ctx context.Context,
	params agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams,
) (agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetRes, error) {
	endpoint := fmt.Sprintf("/api/v1/incident/session/%s/result", params.SessionID)
	sess, err := h.getAuthorizedSession(ctx, params.SessionID, endpoint)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return &agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetNotFound{
				Type:     "https://kubernaut.ai/problems/not-found",
				Title:    "Session Not Found",
				Detail:   fmt.Sprintf("session %s not found", params.SessionID),
				Status:   404,
				Instance: endpoint,
			}, nil
		}
		h.logger.Error(err, "session lookup failed", "session_id", params.SessionID)
		return nil, errors.New("internal server error")
	}

	if !session.IsTerminal(sess.Status) {
		return &agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetConflict{
			Type:     "https://kubernaut.ai/problems/session-not-completed",
			Title:    "Session Not Completed",
			Detail:   fmt.Sprintf("session %s is %s, not completed", params.SessionID, mapSessionStatusToAPI(sess.Status)),
			Status:   409,
			Instance: fmt.Sprintf("/api/v1/incident/session/%s/result", params.SessionID),
		}, nil
	}

	var incidentID string
	if sess.Metadata != nil {
		incidentID = sess.Metadata["incident_id"]
	}

	result := sess.Result
	if result == nil {
		h.logger.Info("nil_result_synthesized", "session_id", sess.ID, "status", string(sess.Status))
		synthetic := synthesizeNilResult(sess)
		resp := mapInvestigationResultToResponse(h.logger, synthetic, incidentID)
		return resp, nil
	}

	resp := mapInvestigationResultToResponse(h.logger, result, incidentID)
	return resp, nil
}

// CancelSessionAPIV1IncidentSessionSessionIDCancelPost implements POST /api/v1/incident/session/{session_id}/cancel.
func (h *Handler) CancelSessionAPIV1IncidentSessionSessionIDCancelPost(
	ctx context.Context,
	params agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostParams,
) (agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostRes, error) {
	endpoint := fmt.Sprintf("/api/v1/incident/session/%s/cancel", params.SessionID)
	if _, authzErr := h.getAuthorizedSession(ctx, params.SessionID, endpoint); authzErr != nil {
		if errors.Is(authzErr, session.ErrSessionNotFound) {
			return &agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostNotFound{
				Type:     "https://kubernaut.ai/problems/not-found",
				Title:    "Session Not Found",
				Detail:   fmt.Sprintf("session %s not found", params.SessionID),
				Status:   404,
				Instance: endpoint,
			}, nil
		}
		return nil, errors.New("internal server error")
	}
	err := h.sessions.CancelInvestigation(params.SessionID)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return &agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostNotFound{
				Type:     "https://kubernaut.ai/problems/not-found",
				Title:    "Session Not Found",
				Detail:   fmt.Sprintf("session %s not found", params.SessionID),
				Status:   404,
				Instance: fmt.Sprintf("/api/v1/incident/session/%s/cancel", params.SessionID),
			}, nil
		}
		if errors.Is(err, session.ErrSessionTerminal) {
			return &agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostConflict{
				Type:     "https://kubernaut.ai/problems/session-already-terminal",
				Title:    "Session Already Terminal",
				Detail:   fmt.Sprintf("session %s is already in a terminal state", params.SessionID),
				Status:   409,
				Instance: fmt.Sprintf("/api/v1/incident/session/%s/cancel", params.SessionID),
			}, nil
		}
		h.logger.Error(err, "cancel session failed", "session_id", params.SessionID)
		return nil, errors.New("internal server error")
	}

	return &agentclient.CancelSessionResponse{
		SessionID: params.SessionID,
		Status:    "cancelled",
	}, nil
}

// SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGet implements GET /api/v1/incident/session/{session_id}/snapshot.
func (h *Handler) SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGet(
	ctx context.Context,
	params agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetParams,
) (agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetRes, error) {
	endpoint := fmt.Sprintf("/api/v1/incident/session/%s/snapshot", params.SessionID)
	sess, err := h.getAuthorizedSession(ctx, params.SessionID, endpoint)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return &agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetNotFound{
				Type:     "https://kubernaut.ai/problems/not-found",
				Title:    "Session Not Found",
				Detail:   fmt.Sprintf("session %s not found", params.SessionID),
				Status:   404,
				Instance: endpoint,
			}, nil
		}
		h.logger.Error(err, "snapshot lookup failed", "session_id", params.SessionID)
		return nil, errors.New("internal server error")
	}

	if !session.IsTerminal(sess.Status) {
		return &agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetConflict{
			Type:     "https://kubernaut.ai/problems/session-in-progress",
			Title:    "Session In Progress",
			Detail:   fmt.Sprintf("session %s is %s; use the stream endpoint for live updates", params.SessionID, mapSessionStatusToAPI(sess.Status)),
			Status:   409,
			Instance: fmt.Sprintf("/api/v1/incident/session/%s/snapshot", params.SessionID),
		}, nil
	}

	snap := &agentclient.SessionSnapshot{
		SessionID: sess.ID,
		Status:    mapSessionStatusToAPI(sess.Status),
		CreatedAt: sess.CreatedAt.UTC().Format(time.RFC3339),
	}
	if sess.Metadata != nil {
		md := agentclient.SessionSnapshotMetadata(sess.Metadata)
		snap.Metadata.SetTo(md)
	}
	if sess.Error != nil {
		snap.Error.SetTo(sess.Error.Error())
	}
	if ir := sess.Result; ir != nil {
		if ir.CancelledPhase != "" {
			snap.CancelledPhase.SetTo(ir.CancelledPhase)
		}
		if ir.CancelledAtTurn > 0 {
			snap.CancelledAtTurn.SetTo(ir.CancelledAtTurn)
		}
		if ir.RCASummary != "" {
			snap.RcaSummary.SetTo(ir.RCASummary)
		}
		if ir.TokenUsage != nil {
			snap.TotalPromptTokens.SetTo(ir.TokenUsage.PromptTokens)
			snap.TotalCompletionTokens.SetTo(ir.TokenUsage.CompletionTokens)
		}
	}
	return snap, nil
}

// MapIncidentRequestToSignal converts an OpenAPI IncidentRequest to an internal SignalContext.
func MapIncidentRequestToSignal(req *agentclient.IncidentRequest) katypes.SignalContext {
	sc := katypes.SignalContext{
		Name:             req.SignalName,
		Namespace:        req.ResourceNamespace,
		Severity:         string(req.Severity),
		Message:          req.ErrorMessage,
		IncidentID:       req.IncidentID,
		ResourceKind:     req.ResourceKind,
		ResourceName:     req.ResourceName,
		ClusterName:      req.ClusterName,
		Environment:      req.Environment,
		Priority:         req.Priority,
		RiskTolerance:    req.RiskTolerance,
		SignalSource:     req.SignalSource,
		BusinessCategory: req.BusinessCategory,
		RemediationID:    req.RemediationID,
	}
	if v, ok := req.Description.Get(); ok {
		sc.Description = v
	}
	if v, ok := req.SignalMode.Get(); ok {
		sc.SignalMode = strings.ToLower(string(v))
	}
	if v, ok := req.FiringTime.Get(); ok {
		sc.FiringTime = v
	}
	if v, ok := req.ReceivedTime.Get(); ok {
		sc.ReceivedTime = v
	}
	if v, ok := req.IsDuplicate.Get(); ok {
		sc.IsDuplicate = &v
	}
	if v, ok := req.OccurrenceCount.Get(); ok {
		sc.OccurrenceCount = &v
	}
	if v, ok := req.SignalAnnotations.Get(); ok {
		sc.SignalAnnotations = map[string]string(v)
	}
	if v, ok := req.SignalLabels.Get(); ok {
		sc.SignalLabels = map[string]string(v)
	}
	if v, ok := req.DeduplicationWindowMinutes.Get(); ok {
		sc.DeduplicationWindowMinutes = &v
	}
	if v, ok := req.FirstSeen.Get(); ok {
		sc.FirstSeen = v
	}
	if v, ok := req.LastSeen.Get(); ok {
		sc.LastSeen = v
	}
	if v, ok := req.Interactive.Get(); ok {
		sc.Interactive = v
	}
	return sc
}

// SessionStreamAPIV1IncidentSessionSessionIDStreamGet implements
// GET /api/v1/incident/session/{session_id}/stream.
// Returns an SSE event stream via io.Pipe. The ogen encoder copies from the
// pipe reader while a goroutine writes SSE-framed events from the session's
// event channel into the pipe writer. The pipe is closed when the channel
// closes (investigation ends) or the request context is cancelled (client
// disconnect).
func (h *Handler) SessionStreamAPIV1IncidentSessionSessionIDStreamGet(
	ctx context.Context,
	params agentclient.SessionStreamAPIV1IncidentSessionSessionIDStreamGetParams,
) (agentclient.SessionStreamAPIV1IncidentSessionSessionIDStreamGetRes, error) {
	endpoint := fmt.Sprintf("/api/v1/incident/session/%s/stream", params.SessionID)
	if _, authzErr := h.getAuthorizedSession(ctx, params.SessionID, endpoint); authzErr != nil {
		if errors.Is(authzErr, session.ErrSessionNotFound) {
			return &agentclient.HTTPError{
				Type:     "https://kubernaut.ai/problems/not-found",
				Title:    "Session Not Found",
				Detail:   fmt.Sprintf("session %s not found", params.SessionID),
				Status:   404,
				Instance: endpoint,
			}, nil
		}
		h.logger.Error(authzErr, "stream authz failed", "session_id", params.SessionID)
		return nil, errors.New("internal server error")
	}

	ch, err := h.sessions.Subscribe(ctx, params.SessionID)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return &agentclient.HTTPError{
				Type:     "https://kubernaut.ai/problems/not-found",
				Title:    "Session Not Found",
				Detail:   fmt.Sprintf("session %s not found", params.SessionID),
				Status:   404,
				Instance: fmt.Sprintf("/api/v1/incident/session/%s/stream", params.SessionID),
			}, nil
		}
		if errors.Is(err, session.ErrSessionTerminal) {
			return h.terminalSessionSSE(params.SessionID)
		}
		h.logger.Error(err, "subscribe failed", "session_id", params.SessionID)
		return nil, errors.New("internal server error")
	}

	pr, pw := io.Pipe()
	go h.streamSessionEvents(ctx, params.SessionID, ch, pw)

	return &agentclient.SessionStreamAPIV1IncidentSessionSessionIDStreamGetOK{Data: pr}, nil
}

// streamSessionEvents drains ch and writes each event to pw as an SSE frame
// until ctx is cancelled or ch is closed. Intended to run in its own
// goroutine (spawned by the stream handler); always closes pw on exit so the
// paired reader observes io.EOF, and recovers from panics so a single
// malformed event cannot crash the process.
func (h *Handler) streamSessionEvents(ctx context.Context, sessionID string, ch <-chan session.InvestigationEvent, pw *io.PipeWriter) {
	// CloseWithError(nil) behaves like Close but makes the intent explicit:
	// the pipe writer is always closed when the goroutine exits, regardless
	// of the exit path. The reader sees io.EOF. (#54 defer error handling)
	defer func() { _ = pw.CloseWithError(nil) }()
	defer func() {
		if r := recover(); r != nil {
			h.logger.Error(fmt.Errorf("panic: %v", r), "SSE writer panic recovered", "session_id", sessionID)
		}
	}()
	seq := 1
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				h.logger.Error(err, "SSE event marshal failed, skipping frame",
					"session_id", sessionID, "event_type", ev.Type)
				continue
			}
			frame := fmt.Sprintf("id: %d\nevent: %s\ndata: %s\n\n", seq, ev.Type, string(data))
			if _, writeErr := pw.Write([]byte(frame)); writeErr != nil {
				return
			}
			seq++
		}
	}
}

// terminalSessionSSE returns an SSE stream with a synthetic complete event
// for sessions that have already concluded. This prevents a race condition
// where clients connecting after investigation completion would receive a
// JSON 404 instead of an SSE stream — breaking reconnection flows (e.g. AF
// reconnecting after a dropped connection).
func (h *Handler) terminalSessionSSE(sessionID string) (agentclient.SessionStreamAPIV1IncidentSessionSessionIDStreamGetRes, error) {
	sess, err := h.sessions.GetSession(sessionID)
	if err != nil {
		return &agentclient.HTTPError{
			Type:     "https://kubernaut.ai/problems/not-found",
			Title:    "Session Not Found",
			Detail:   fmt.Sprintf("session %s not found", sessionID),
			Status:   404,
			Instance: fmt.Sprintf("/api/v1/incident/session/%s/stream", sessionID),
		}, nil
	}

	completeEvent := session.InvestigationEvent{
		Type:  session.EventTypeComplete,
		Turn:  0,
		Phase: "complete",
	}
	if sess.Result != nil {
		if data, marshalErr := json.Marshal(sess.Result); marshalErr == nil {
			completeEvent.Data = data
		}
	}

	frame, err := json.Marshal(completeEvent)
	if err != nil {
		h.logger.Error(err, "terminal SSE marshal failed", "session_id", sessionID)
		return nil, errors.New("internal server error")
	}

	pr, pw := io.Pipe()
	go func() {
		defer func() { _ = pw.CloseWithError(nil) }()
		sseFrame := fmt.Sprintf("id: 1\nevent: %s\ndata: %s\n\n", session.EventTypeComplete, string(frame))
		_, _ = pw.Write([]byte(sseFrame))
	}()

	return &agentclient.SessionStreamAPIV1IncidentSessionSessionIDStreamGetOK{Data: pr}, nil
}

// getAuthorizedSession retrieves a session and checks that the requesting user
// is the session owner. Returns the session if authorized, or nil with
// ErrSessionNotFound if the session doesn't exist or the user is not the owner.
// When auth middleware is disabled (user is empty), ownership checks are skipped.
// Denied access attempts are recorded via aiagent.session.access_denied for
// SOC2 CC8.1 failed-access audit trail.
func (h *Handler) getAuthorizedSession(ctx context.Context, sessionID, endpoint string) (*session.Session, error) {
	sess, err := h.sessions.GetSession(sessionID)
	if err != nil {
		h.metrics.RecordAuthzDenied("session_not_found")
		return nil, err
	}

	requestUser := auth.GetUserFromContext(ctx)
	if requestUser == "" {
		return sess, nil
	}

	owner := sess.Metadata["created_by"]
	if owner != "" && subtle.ConstantTimeCompare([]byte(owner), []byte(requestUser)) != 1 {
		h.metrics.RecordAuthzDenied("owner_mismatch")
		h.sessions.EmitAccessDenied(ctx, sessionID, endpoint, requestUser)
		return nil, session.ErrSessionNotFound
	}

	return sess, nil
}

// TestGetAuthorizedSession exposes getAuthorizedSession for unit tests.
// It is not used in production code paths.
func (h *Handler) TestGetAuthorizedSession(ctx context.Context, sessionID, endpoint string) (*session.Session, error) {
	return h.getAuthorizedSession(ctx, sessionID, endpoint)
}

func mapSessionStatusToAPI(s session.Status) string {
	switch s {
	case session.StatusPending:
		return "pending"
	case session.StatusRunning:
		return "investigating"
	case session.StatusCompleted:
		return "completed"
	case session.StatusFailed:
		return "failed"
	case session.StatusCancelled:
		return "cancelled"
	case session.StatusUserDriving:
		return "user_driving"
	default:
		return "unknown"
	}
}

// synthesizeNilResult creates a minimal InvestigationResult for terminal sessions
// whose goroutine produced no result. This prevents AA from entering a 409 polling
// loop (#1390). The synthetic result is always non-actionable.
func synthesizeNilResult(sess *session.Session) *katypes.InvestigationResult {
	switch sess.Status {
	case session.StatusFailed:
		reason := "Investigation failed"
		if sess.Error != nil {
			reason = fmt.Sprintf("Investigation failed: %s", sess.Error.Error())
		}
		return &katypes.InvestigationResult{
			RCASummary:        reason,
			Confidence:        0,
			HumanReviewNeeded: true,
			HumanReviewReason: "investigation_failed",
		}
	case session.StatusCancelled:
		return &katypes.InvestigationResult{
			RCASummary: "Investigation cancelled",
			Confidence: 0,
		}
	default:
		return &katypes.InvestigationResult{
			RCASummary: "Investigation completed without result",
			Confidence: 0,
		}
	}
}

func mapInvestigationResultToResponse(log logr.Logger, r *katypes.InvestigationResult, incidentID string) *agentclient.IncidentResponse {
	resp := &agentclient.IncidentResponse{
		IncidentID:        incidentID,
		Analysis:          r.RCASummary,
		Confidence:        r.Confidence,
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		RootCauseAnalysis: buildRootCauseAnalysis(r),
	}

	applyHumanReviewFields(resp, log, r)

	if r.WorkflowID != "" {
		resp.SelectedWorkflow.SetTo(buildSelectedWorkflow(r))
	}

	if r.IsActionable != nil {
		resp.IsActionable.SetTo(*r.IsActionable)
	}
	resp.Warnings = buildResponseWarnings(r)

	if len(r.DetectedLabels) > 0 {
		resp.DetectedLabels.SetTo(buildDetectedLabelsResponse(r))
	}

	if len(r.AlternativeWorkflows) > 0 {
		resp.AlternativeWorkflows = buildAlternativeWorkflowsResponse(r)
	}

	if len(r.ValidationAttemptsHistory) > 0 {
		resp.ValidationAttemptsHistory = buildValidationAttemptsResponse(r)
	}

	if r.AlignmentVerdict != nil {
		resp.AlignmentVerdict.SetTo(buildAlignmentVerdictResponse(r))
	}

	return resp
}

// buildRootCauseAnalysis maps the RCA-related InvestigationResult fields into
// the wire-format RootCauseAnalysis map, omitting any field that was never
// populated by the investigator.
func buildRootCauseAnalysis(r *katypes.InvestigationResult) agentclient.IncidentResponseRootCauseAnalysis {
	rca := agentclient.IncidentResponseRootCauseAnalysis{}
	if r.RCASummary != "" {
		summaryRaw, _ := json.Marshal(r.RCASummary)
		rca["summary"] = jx.Raw(summaryRaw)
	}
	if r.Severity != "" {
		sevRaw, _ := json.Marshal(r.Severity)
		rca["severity"] = jx.Raw(sevRaw)
	}
	if r.RemediationTarget.Kind != "" {
		targetRaw, _ := json.Marshal(r.RemediationTarget)
		rca["remediationTarget"] = jx.Raw(targetRaw)
	}
	if r.SignalName != "" {
		snRaw, _ := json.Marshal(r.SignalName)
		rca["signal_name"] = jx.Raw(snRaw)
	}
	if len(r.ContributingFactors) > 0 {
		cfRaw, _ := json.Marshal(r.ContributingFactors)
		rca["contributing_factors"] = jx.Raw(cfRaw)
	}
	if len(r.CausalChain) > 0 {
		ccRaw, _ := json.Marshal(r.CausalChain)
		rca["causal_chain"] = jx.Raw(ccRaw)
	}
	if r.DueDiligence != nil {
		ddRaw, _ := json.Marshal(r.DueDiligence)
		rca["due_diligence"] = jx.Raw(ddRaw)
	}
	return rca
}

// applyHumanReviewFields sets resp.NeedsHumanReview and, when review is
// needed, maps the (possibly LLM-provided, possibly unrecognized) reason
// string to the response's HumanReviewReason enum.
func applyHumanReviewFields(resp *agentclient.IncidentResponse, log logr.Logger, r *katypes.InvestigationResult) {
	if !r.HumanReviewNeeded {
		resp.NeedsHumanReview.SetTo(false)
		return
	}
	resp.NeedsHumanReview.SetTo(true)
	reason := r.HumanReviewReason
	if reason == "" {
		reason = r.Reason
	}
	mapped, isDefault := mapHumanReviewReason(reason)
	if isDefault && reason != "" {
		log.Info("unrecognized human review reason, falling back to investigation_inconclusive",
			"original_reason", reason)
	}
	resp.HumanReviewReason.SetTo(mapped)
}

// buildSelectedWorkflow maps the selected-workflow fields of r into the
// wire-format SelectedWorkflow map. Callers must check r.WorkflowID != "".
func buildSelectedWorkflow(r *katypes.InvestigationResult) agentclient.IncidentResponseSelectedWorkflow {
	sw := agentclient.IncidentResponseSelectedWorkflow{}
	wfIDRaw, _ := json.Marshal(r.WorkflowID)
	sw["workflow_id"] = jx.Raw(wfIDRaw)
	if len(r.Parameters) > 0 {
		paramsRaw, _ := json.Marshal(r.Parameters)
		sw["parameters"] = jx.Raw(paramsRaw)
	}
	confRaw, _ := json.Marshal(r.Confidence)
	sw["confidence"] = jx.Raw(confRaw)
	if r.ExecutionBundle != "" {
		ebRaw, _ := json.Marshal(r.ExecutionBundle)
		sw["execution_bundle"] = jx.Raw(ebRaw)
	}
	if r.ExecutionBundleDigest != "" {
		ebdRaw, _ := json.Marshal(r.ExecutionBundleDigest)
		sw["execution_bundle_digest"] = jx.Raw(ebdRaw)
	}
	if r.ExecutionEngine != "" {
		eeRaw, _ := json.Marshal(r.ExecutionEngine)
		sw["execution_engine"] = jx.Raw(eeRaw)
	}
	if r.ServiceAccountName != "" {
		saRaw, _ := json.Marshal(r.ServiceAccountName)
		sw["service_account_name"] = jx.Raw(saRaw)
	}
	if r.WorkflowVersion != "" {
		vRaw, _ := json.Marshal(r.WorkflowVersion)
		sw["version"] = jx.Raw(vRaw)
	}
	if r.WorkflowRationale != "" {
		rRaw, _ := json.Marshal(r.WorkflowRationale)
		sw["rationale"] = jx.Raw(rRaw)
	}
	return sw
}

// buildResponseWarnings returns r.Warnings verbatim when set, a synthesized
// human-review warning (BR-HAPI-197) when review is needed but no warning
// was set, or an empty (non-nil) slice otherwise.
func buildResponseWarnings(r *katypes.InvestigationResult) []string {
	if len(r.Warnings) > 0 {
		return r.Warnings
	}
	if r.HumanReviewNeeded {
		return []string{synthesizeHumanReviewWarning(r)}
	}
	return []string{}
}

// buildDetectedLabelsResponse maps r.DetectedLabels into the wire format.
// Callers must check len(r.DetectedLabels) > 0.
func buildDetectedLabelsResponse(r *katypes.InvestigationResult) agentclient.IncidentResponseDetectedLabels {
	dl := make(agentclient.IncidentResponseDetectedLabels, len(r.DetectedLabels))
	for k, v := range r.DetectedLabels {
		raw, _ := json.Marshal(v)
		dl[k] = jx.Raw(raw)
	}
	return dl
}

// buildAlternativeWorkflowsResponse maps r.AlternativeWorkflows into the wire
// format. Callers must check len(r.AlternativeWorkflows) > 0.
func buildAlternativeWorkflowsResponse(r *katypes.InvestigationResult) []agentclient.AlternativeWorkflow {
	alts := make([]agentclient.AlternativeWorkflow, 0, len(r.AlternativeWorkflows))
	for _, aw := range r.AlternativeWorkflows {
		alt := agentclient.AlternativeWorkflow{
			WorkflowID: aw.WorkflowID,
			Confidence: aw.Confidence,
			Rationale:  aw.Rationale,
		}
		if aw.ExecutionBundle != "" {
			alt.ExecutionBundle.SetTo(aw.ExecutionBundle)
		}
		alts = append(alts, alt)
	}
	return alts
}

// buildValidationAttemptsResponse maps r.ValidationAttemptsHistory into the
// wire format. Callers must check len(r.ValidationAttemptsHistory) > 0.
func buildValidationAttemptsResponse(r *katypes.InvestigationResult) []agentclient.ValidationAttempt {
	attempts := make([]agentclient.ValidationAttempt, 0, len(r.ValidationAttemptsHistory))
	for _, va := range r.ValidationAttemptsHistory {
		attempt := agentclient.ValidationAttempt{
			Attempt:   va.Attempt,
			IsValid:   va.IsValid,
			Errors:    va.Errors,
			Timestamp: va.Timestamp,
		}
		if va.WorkflowID != "" {
			attempt.WorkflowID.SetTo(va.WorkflowID)
		}
		attempts = append(attempts, attempt)
	}
	return attempts
}

// buildAlignmentVerdictResponse maps r.AlignmentVerdict into the wire format.
// Callers must check r.AlignmentVerdict != nil.
func buildAlignmentVerdictResponse(r *katypes.InvestigationResult) agentclient.AlignmentVerdict {
	av := agentclient.AlignmentVerdict{
		Result:  agentclient.AlignmentVerdictResult(r.AlignmentVerdict.Result),
		Flagged: r.AlignmentVerdict.Flagged,
		Total:   r.AlignmentVerdict.Total,
	}
	if r.AlignmentVerdict.CircuitBreakerActivated {
		av.CircuitBreakerActivated.SetTo(true)
	}
	if r.AlignmentVerdict.Summary != "" {
		av.Summary.SetTo(r.AlignmentVerdict.Summary)
	}
	if len(r.AlignmentVerdict.Findings) > 0 {
		findings := make([]agentclient.AlignmentFinding, 0, len(r.AlignmentVerdict.Findings))
		for _, f := range r.AlignmentVerdict.Findings {
			finding := agentclient.AlignmentFinding{
				StepIndex:   f.StepIndex,
				StepKind:    agentclient.AlignmentFindingStepKind(f.StepKind),
				Explanation: f.Explanation,
			}
			if f.Tool != "" {
				finding.Tool.SetTo(f.Tool)
			}
			findings = append(findings, finding)
		}
		av.Findings = findings
	}
	return av
}

// synthesizeHumanReviewWarning generates a warning when human review is required
// but no explicit warnings were set by the parser. Per BR-HAPI-197, human review
// responses must include at least one warning explaining why automation is unavailable.
func synthesizeHumanReviewWarning(r *katypes.InvestigationResult) string {
	reason := r.HumanReviewReason
	if reason == "" {
		reason = r.Reason
	}
	if reason != "" {
		return fmt.Sprintf("Human review required: %s", reason)
	}
	return "Human review required: investigation could not determine automated remediation"
}

// mapHumanReviewReason maps free-form investigator reason strings to valid
// HumanReviewReason enum values. Returns the mapped enum and whether the
// default fallback was used (M4: enables caller logging of unrecognized reasons).
// exactHumanReviewReasons maps the canonical (already-normalized)
// HumanReviewReason string values emitted by the investigator to their
// OpenAPI enum. Kept as a package-level map (rather than a switch) so
// mapHumanReviewReason stays a simple two-step lookup.
var exactHumanReviewReasons = map[string]agentclient.HumanReviewReason{
	"rca_incomplete":              agentclient.HumanReviewReasonRcaIncomplete,
	"investigation_inconclusive":  agentclient.HumanReviewReasonInvestigationInconclusive,
	"workflow_not_found":          agentclient.HumanReviewReasonWorkflowNotFound,
	"no_matching_workflows":       agentclient.HumanReviewReasonNoMatchingWorkflows,
	"image_mismatch":              agentclient.HumanReviewReasonImageMismatch,
	"parameter_validation_failed": agentclient.HumanReviewReasonParameterValidationFailed,
	"low_confidence":              agentclient.HumanReviewReasonLowConfidence,
	"llm_parsing_error":           agentclient.HumanReviewReasonLlmParsingError,
	"alignment_check_failed":      agentclient.HumanReviewReasonAlignmentCheckFailed,
	"operator_escalation":         agentclient.HumanReviewReasonOperatorEscalation,
}

func mapHumanReviewReason(reason string) (agentclient.HumanReviewReason, bool) {
	if mapped, ok := exactHumanReviewReasons[reason]; ok {
		return mapped, false
	}
	return mapHumanReviewReasonHeuristic(reason)
}

// mapHumanReviewReasonHeuristic falls back to substring matching for
// free-form reason strings (e.g. raw error messages) that don't match one of
// the canonical exactHumanReviewReasons values.
func mapHumanReviewReasonHeuristic(reason string) (agentclient.HumanReviewReason, bool) {
	switch {
	case strings.Contains(reason, "exhausted during RCA"):
		return agentclient.HumanReviewReasonRcaIncomplete, false
	case strings.Contains(reason, "exhausted during workflow selection"):
		return agentclient.HumanReviewReasonInvestigationInconclusive, false
	case strings.Contains(reason, "not found") && strings.Contains(reason, "catalog"):
		return agentclient.HumanReviewReasonWorkflowNotFound, false
	case strings.Contains(reason, "no matching"):
		return agentclient.HumanReviewReasonNoMatchingWorkflows, false
	case strings.Contains(reason, "mismatch") || strings.Contains(reason, "image"):
		return agentclient.HumanReviewReasonImageMismatch, false
	case strings.Contains(reason, "parameter") || strings.Contains(reason, "validation"):
		return agentclient.HumanReviewReasonParameterValidationFailed, false
	case strings.Contains(reason, "confidence"):
		return agentclient.HumanReviewReasonLowConfidence, false
	case strings.Contains(reason, "parse") || strings.Contains(reason, "parsing"):
		return agentclient.HumanReviewReasonLlmParsingError, false
	default:
		return agentclient.HumanReviewReasonInvestigationInconclusive, true
	}
}
