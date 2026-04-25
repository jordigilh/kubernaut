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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/go-faster/jx"

	"github.com/jordigilh/kubernaut/pkg/agentclient"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// InvestigationRunner abstracts the investigation entry point so that
// decorators (e.g. alignment.InvestigatorWrapper) can wrap it transparently.
type InvestigationRunner interface {
	Investigate(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error)
}

// Handler implements the ogen-generated Handler interface for the 3 business
// endpoints. Operational endpoints (/health, /ready, /config, /metrics) are
// served directly by the HTTP mux in cmd/kubernautagent/main.go.
type Handler struct {
	agentclient.UnimplementedHandler

	sessions     *session.Manager
	investigator InvestigationRunner
	logger       *slog.Logger
}

var _ agentclient.Handler = (*Handler)(nil)

// NewHandler creates a Kubernaut Agent ogen handler.
func NewHandler(sessions *session.Manager, inv InvestigationRunner, logger *slog.Logger) *Handler {
	return &Handler{
		sessions:     sessions,
		investigator: inv,
		logger:       logger,
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
		h.logger.Error("investigator not configured")
		return &agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostInternalServerErrorApplicationProblemJSON{
			Type:     "https://kubernaut.ai/problems/internal-error",
			Title:    "Internal Server Error",
			Detail:   "investigator not configured",
			Status:   500,
			Instance: "/api/v1/incident/analyze",
		}, nil
	}

	signal := MapIncidentRequestToSignal(req)
	h.logger.Info("investigation submitted",
		"incident_id", req.IncidentID,
		"signal", signal.Name,
		"namespace", signal.Namespace,
	)

	metadata := map[string]string{
		"incident_id":    req.IncidentID,
		"remediation_id": req.RemediationID,
	}
	sessionID, err := h.sessions.StartInvestigation(ctx, func(bgCtx context.Context) (interface{}, error) {
		return h.investigator.Investigate(bgCtx, signal)
	}, metadata)
	if err != nil {
		h.logger.Error("failed to start investigation", "error", err)
		return &agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostInternalServerErrorApplicationProblemJSON{
			Type:     "https://kubernaut.ai/problems/internal-error",
			Title:    "Internal Server Error",
			Detail:   "failed to start investigation: " + err.Error(),
			Status:   500,
			Instance: "/api/v1/incident/analyze",
		}, nil
	}

	body, _ := json.Marshal(map[string]string{"session_id": sessionID})
	raw := agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostAcceptedApplicationJSON(body)
	return &raw, nil
}

// IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet implements GET /api/v1/incident/session/{session_id}.
func (h *Handler) IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(
	ctx context.Context,
	params agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams,
) (agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetRes, error) {
	sess, err := h.getAuthorizedSession(ctx, params.SessionID)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return &agentclient.HTTPError{
				Type:     "https://kubernaut.ai/problems/not-found",
				Title:    "Session Not Found",
				Detail:   fmt.Sprintf("session %s not found", params.SessionID),
				Status:   404,
				Instance: fmt.Sprintf("/api/v1/incident/session/%s", params.SessionID),
			}, nil
		}
		h.logger.Error("session lookup failed", "session_id", params.SessionID, "error", err)
		return nil, fmt.Errorf("session lookup: %w", err)
	}

	status := mapSessionStatusToAPI(sess.Status)
	body, _ := json.Marshal(map[string]string{
		"session_id": sess.ID,
		"status":     status,
	})
	raw := agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetOKApplicationJSON(body)
	return &raw, nil
}

// IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet implements
// GET /api/v1/incident/session/{session_id}/result.
func (h *Handler) IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(
	ctx context.Context,
	params agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams,
) (agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetRes, error) {
	sess, err := h.getAuthorizedSession(ctx, params.SessionID)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return &agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetNotFound{
				Type:     "https://kubernaut.ai/problems/not-found",
				Title:    "Session Not Found",
				Detail:   fmt.Sprintf("session %s not found", params.SessionID),
				Status:   404,
				Instance: fmt.Sprintf("/api/v1/incident/session/%s/result", params.SessionID),
			}, nil
		}
		h.logger.Error("session lookup failed", "session_id", params.SessionID, "error", err)
		return nil, fmt.Errorf("session lookup: %w", err)
	}

	if sess.Status != session.StatusCompleted {
		return &agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetConflict{
			Type:     "https://kubernaut.ai/problems/session-not-completed",
			Title:    "Session Not Completed",
			Detail:   fmt.Sprintf("session %s is %s, not completed", params.SessionID, mapSessionStatusToAPI(sess.Status)),
			Status:   409,
			Instance: fmt.Sprintf("/api/v1/incident/session/%s/result", params.SessionID),
		}, nil
	}

	result, ok := sess.Result.(*katypes.InvestigationResult)
	if !ok {
		h.logger.Error("unexpected result type in session", "session_id", sess.ID)
		return &agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetConflict{
			Type:     "https://kubernaut.ai/problems/session-not-completed",
			Title:    "Session Not Completed",
			Detail:   "session result is not an investigation result",
			Status:   409,
			Instance: fmt.Sprintf("/api/v1/incident/session/%s/result", params.SessionID),
		}, nil
	}

	var incidentID string
	if sess.Metadata != nil {
		incidentID = sess.Metadata["incident_id"]
	}

	resp := mapInvestigationResultToResponse(result, incidentID)
	return resp, nil
}

// CancelSessionAPIV1IncidentSessionSessionIDCancelPost implements POST /api/v1/incident/session/{session_id}/cancel.
func (h *Handler) CancelSessionAPIV1IncidentSessionSessionIDCancelPost(
	ctx context.Context,
	params agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostParams,
) (agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostRes, error) {
	if _, authzErr := h.getAuthorizedSession(ctx, params.SessionID); authzErr != nil {
		if errors.Is(authzErr, session.ErrSessionNotFound) {
			return &agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostNotFound{
				Type:     "https://kubernaut.ai/problems/not-found",
				Title:    "Session Not Found",
				Detail:   fmt.Sprintf("session %s not found", params.SessionID),
				Status:   404,
				Instance: fmt.Sprintf("/api/v1/incident/session/%s/cancel", params.SessionID),
			}, nil
		}
		return nil, fmt.Errorf("session authz: %w", authzErr)
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
		h.logger.Error("cancel session failed", "session_id", params.SessionID, "error", err)
		return nil, fmt.Errorf("cancel session: %w", err)
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
	sess, err := h.getAuthorizedSession(ctx, params.SessionID)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return &agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetNotFound{
				Type:     "https://kubernaut.ai/problems/not-found",
				Title:    "Session Not Found",
				Detail:   fmt.Sprintf("session %s not found", params.SessionID),
				Status:   404,
				Instance: fmt.Sprintf("/api/v1/incident/session/%s/snapshot", params.SessionID),
			}, nil
		}
		h.logger.Error("snapshot lookup failed", "session_id", params.SessionID, "error", err)
		return nil, fmt.Errorf("snapshot lookup: %w", err)
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
	if _, authzErr := h.getAuthorizedSession(ctx, params.SessionID); authzErr != nil {
		if errors.Is(authzErr, session.ErrSessionNotFound) {
			return &agentclient.HTTPError{
				Type:     "https://kubernaut.ai/problems/not-found",
				Title:    "Session Not Found",
				Detail:   fmt.Sprintf("session %s not found", params.SessionID),
				Status:   404,
				Instance: fmt.Sprintf("/api/v1/incident/session/%s/stream", params.SessionID),
			}, nil
		}
		h.logger.Error("stream authz failed", "session_id", params.SessionID, "error", authzErr)
		return nil, fmt.Errorf("stream authz: %w", authzErr)
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
			return &agentclient.HTTPError{
				Type:     "https://kubernaut.ai/problems/session-terminal",
				Title:    "Session Terminal",
				Detail:   fmt.Sprintf("session %s has already concluded; use the snapshot endpoint", params.SessionID),
				Status:   404,
				Instance: fmt.Sprintf("/api/v1/incident/session/%s/stream", params.SessionID),
			}, nil
		}
		h.logger.Error("subscribe failed", "session_id", params.SessionID, "error", err)
		return nil, fmt.Errorf("subscribe: %w", err)
	}

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		seq := 1
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				data, _ := json.Marshal(ev)
				frame := fmt.Sprintf("id: %d\nevent: %s\ndata: %s\n\n", seq, ev.Type, string(data))
				if _, writeErr := pw.Write([]byte(frame)); writeErr != nil {
					return
				}
				seq++
			}
		}
	}()

	return &agentclient.SessionStreamAPIV1IncidentSessionSessionIDStreamGetOK{Data: pr}, nil
}

// getAuthorizedSession retrieves a session and checks that the requesting user
// is the session owner. Returns the session if authorized, or nil with
// ErrSessionNotFound if the session doesn't exist or the user is not the owner.
// When auth middleware is disabled (user is empty), ownership checks are skipped.
func (h *Handler) getAuthorizedSession(ctx context.Context, sessionID string) (*session.Session, error) {
	sess, err := h.sessions.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	requestUser := auth.GetUserFromContext(ctx)
	if requestUser == "" {
		return sess, nil
	}

	owner := sess.Metadata["created_by"]
	if owner != "" && owner != requestUser {
		return nil, session.ErrSessionNotFound
	}

	return sess, nil
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
	default:
		return "unknown"
	}
}

func mapInvestigationResultToResponse(r *katypes.InvestigationResult, incidentID string) *agentclient.IncidentResponse {
	resp := &agentclient.IncidentResponse{
		IncidentID: incidentID,
		Analysis:   r.RCASummary,
		Confidence: r.Confidence,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

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
	resp.RootCauseAnalysis = rca

	if r.HumanReviewNeeded {
		resp.NeedsHumanReview.SetTo(true)
		reason := r.HumanReviewReason
		if reason == "" {
			reason = r.Reason
		}
		mapped, isDefault := mapHumanReviewReason(reason)
		if isDefault && reason != "" {
			slog.Warn("unrecognized human review reason, falling back to investigation_inconclusive",
				"original_reason", reason)
		}
		resp.HumanReviewReason.SetTo(mapped)
	} else {
		resp.NeedsHumanReview.SetTo(false)
	}

	if r.WorkflowID != "" {
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
		resp.SelectedWorkflow.SetTo(sw)
	}

	if r.IsActionable != nil {
		resp.IsActionable.SetTo(*r.IsActionable)
	}
	if len(r.Warnings) > 0 {
		resp.Warnings = r.Warnings
	} else if r.HumanReviewNeeded {
		resp.Warnings = []string{synthesizeHumanReviewWarning(r)}
	} else {
		resp.Warnings = []string{}
	}

	if len(r.DetectedLabels) > 0 {
		dl := make(agentclient.IncidentResponseDetectedLabels, len(r.DetectedLabels))
		for k, v := range r.DetectedLabels {
			raw, _ := json.Marshal(v)
			dl[k] = jx.Raw(raw)
		}
		resp.DetectedLabels.SetTo(dl)
	}

	if len(r.AlternativeWorkflows) > 0 {
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
		resp.AlternativeWorkflows = alts
	}

	if len(r.ValidationAttemptsHistory) > 0 {
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
		resp.ValidationAttemptsHistory = attempts
	}

	return resp
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
func mapHumanReviewReason(reason string) (agentclient.HumanReviewReason, bool) {
	switch reason {
	case "rca_incomplete":
		return agentclient.HumanReviewReasonRcaIncomplete, false
	case "investigation_inconclusive":
		return agentclient.HumanReviewReasonInvestigationInconclusive, false
	case "workflow_not_found":
		return agentclient.HumanReviewReasonWorkflowNotFound, false
	case "no_matching_workflows":
		return agentclient.HumanReviewReasonNoMatchingWorkflows, false
	case "image_mismatch":
		return agentclient.HumanReviewReasonImageMismatch, false
	case "parameter_validation_failed":
		return agentclient.HumanReviewReasonParameterValidationFailed, false
	case "low_confidence":
		return agentclient.HumanReviewReasonLowConfidence, false
	case "llm_parsing_error":
		return agentclient.HumanReviewReasonLlmParsingError, false
	case "alignment_check_failed":
		return agentclient.HumanReviewReasonInvestigationInconclusive, false
	}

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
