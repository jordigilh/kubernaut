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
	"strings"
	"time"

	"github.com/go-faster/jx"
	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/jordigilh/kubernaut/pkg/agentclient"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
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
	logger       logr.Logger
}

var _ agentclient.Handler = (*Handler)(nil)

// NewHandler creates a Kubernaut Agent ogen handler.
func NewHandler(sessions *session.Manager, inv InvestigationRunner, logger logr.Logger) *Handler {
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
	h.logger.Info("investigation submitted",
		"incident_id", req.IncidentID,
		"signal", signal.Name,
		"namespace", signal.Namespace,
	)

	metadata := map[string]string{
		"incident_id": req.IncidentID,
	}
	sessionID, err := h.sessions.StartInvestigation(ctx, func(bgCtx context.Context) (*katypes.InvestigationResult, error) {
		return h.investigator.Investigate(bgCtx, signal)
	}, metadata)
	if err != nil {
		h.logger.Error(err, "failed to start investigation")
		return &agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostInternalServerErrorApplicationProblemJSON{
			Type:     "https://kubernaut.ai/problems/internal-error",
			Title:    "Internal Server Error",
			Detail:   "internal server error",
			Status:   500,
			Instance: "/api/v1/incident/analyze",
		}, nil
	}

	sid, err := uuid.Parse(sessionID)
	if err != nil {
		h.logger.Error(err, "invalid session UUID", "session_id", sessionID)
		return &agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostInternalServerErrorApplicationProblemJSON{
			Type:     "https://kubernaut.ai/problems/internal-error",
			Title:    "Internal Server Error",
			Detail:   "internal server error",
			Status:   500,
			Instance: "/api/v1/incident/analyze",
		}, nil
	}
	return &agentclient.AnalyzeAccepted{SessionID: sid}, nil
}

// IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet implements GET /api/v1/incident/session/{session_id}.
func (h *Handler) IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(
	_ context.Context,
	params agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams,
) (agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetRes, error) {
	sess, err := h.sessions.GetSession(params.SessionID)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return &agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetNotFound{
				Type:     "https://kubernaut.ai/problems/not-found",
				Title:    "Session Not Found",
				Detail:   fmt.Sprintf("session %s not found", params.SessionID),
				Status:   404,
				Instance: fmt.Sprintf("/api/v1/incident/session/%s", params.SessionID),
			}, nil
		}
		h.logger.Error(err, "session lookup failed", "session_id", params.SessionID)
		return &agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetInternalServerError{
			Type:     "https://kubernaut.ai/problems/internal-error",
			Title:    "Internal Server Error",
			Detail:   "internal server error",
			Status:   500,
			Instance: fmt.Sprintf("/api/v1/incident/session/%s", params.SessionID),
		}, nil
	}

	status := mapSessionStatusToAPI(sess.Status)
	resp := &agentclient.SessionStatus{
		SessionID: sess.ID,
		Status:    status,
	}
	if sess.Status == session.StatusFailed && sess.Error != nil {
		resp.Error = agentclient.NewOptString(sess.Error.Error())
	}
	return resp, nil
}

// IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet implements
// GET /api/v1/incident/session/{session_id}/result.
func (h *Handler) IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(
	_ context.Context,
	params agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams,
) (agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetRes, error) {
	sess, err := h.sessions.GetSession(params.SessionID)
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
		h.logger.Error(err, "session lookup failed", "session_id", params.SessionID)
		return &agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetInternalServerError{
			Type:     "https://kubernaut.ai/problems/internal-error",
			Title:    "Internal Server Error",
			Detail:   "internal server error",
			Status:   500,
			Instance: fmt.Sprintf("/api/v1/incident/session/%s/result", params.SessionID),
		}, nil
	}

	switch sess.Status {
	case session.StatusCompleted:
		// fall through to result mapping
	case session.StatusFailed:
		return mapFailedSessionToResponse(sess), nil
	default:
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

	resp := mapInvestigationResultToResponse(h.logger, sess.Result, incidentID)
	return resp, nil
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
	default:
		return "unknown"
	}
}

func mapFailedSessionToResponse(sess *session.Session) *agentclient.IncidentResponse {
	detail := "investigation failed"
	if sess.Error != nil {
		detail = "investigation failed"
	}
	var incidentID string
	if sess.Metadata != nil {
		incidentID = sess.Metadata["incident_id"]
	}
	return &agentclient.IncidentResponse{
		IncidentID:       incidentID,
		Analysis:         detail,
		Confidence:       0,
		Timestamp:        sess.CreatedAt.Format(time.RFC3339),
		NeedsHumanReview: agentclient.NewOptBool(true),
	}
}

func mapInvestigationResultToResponse(log logr.Logger, r *katypes.InvestigationResult, incidentID string) *agentclient.IncidentResponse {
	resp := &agentclient.IncidentResponse{
		IncidentID: incidentID,
		Analysis:   r.RCASummary,
		Confidence: r.Confidence,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	rca := agentclient.IncidentResponseRootCauseAnalysis{}
	if r.RCASummary != "" {
		if raw := marshalField(log, "summary", r.RCASummary); raw != nil {
			rca["summary"] = raw
		}
	}
	if r.Severity != "" {
		if raw := marshalField(log, "severity", r.Severity); raw != nil {
			rca["severity"] = raw
		}
	}
	if r.RemediationTarget.Kind != "" {
		if raw := marshalField(log, "remediationTarget", r.RemediationTarget); raw != nil {
			rca["remediationTarget"] = raw
		}
	}
	if r.SignalName != "" {
		if raw := marshalField(log, "signal_name", r.SignalName); raw != nil {
			rca["signal_name"] = raw
		}
	}
	if len(r.ContributingFactors) > 0 {
		if raw := marshalField(log, "contributing_factors", r.ContributingFactors); raw != nil {
			rca["contributing_factors"] = raw
		}
	}
	if len(r.CausalChain) > 0 {
		if raw := marshalField(log, "causal_chain", r.CausalChain); raw != nil {
			rca["causal_chain"] = raw
		}
	}
	if r.DueDiligence != nil {
		if raw := marshalField(log, "due_diligence", r.DueDiligence); raw != nil {
			rca["due_diligence"] = raw
		}
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
			log.Info("unrecognized human review reason, falling back to investigation_inconclusive",
				"original_reason", reason)
		}
		resp.HumanReviewReason.SetTo(mapped)
	} else {
		resp.NeedsHumanReview.SetTo(false)
	}

	if r.WorkflowID != "" {
		sw := agentclient.IncidentResponseSelectedWorkflow{}
		if raw := marshalField(log, "workflow_id", r.WorkflowID); raw != nil {
			sw["workflow_id"] = raw
		}
		if len(r.Parameters) > 0 {
			if raw := marshalField(log, "parameters", r.Parameters); raw != nil {
				sw["parameters"] = raw
			}
		}
		if raw := marshalField(log, "confidence", r.Confidence); raw != nil {
			sw["confidence"] = raw
		}
		if r.ExecutionBundle != "" {
			if raw := marshalField(log, "execution_bundle", r.ExecutionBundle); raw != nil {
				sw["execution_bundle"] = raw
			}
		}
		if r.ExecutionBundleDigest != "" {
			if raw := marshalField(log, "execution_bundle_digest", r.ExecutionBundleDigest); raw != nil {
				sw["execution_bundle_digest"] = raw
			}
		}
		if r.ExecutionEngine != "" {
			if raw := marshalField(log, "execution_engine", r.ExecutionEngine); raw != nil {
				sw["execution_engine"] = raw
			}
		}
		if r.ServiceAccountName != "" {
			if raw := marshalField(log, "service_account_name", r.ServiceAccountName); raw != nil {
				sw["service_account_name"] = raw
			}
		}
		if r.WorkflowVersion != "" {
			if raw := marshalField(log, "version", r.WorkflowVersion); raw != nil {
				sw["version"] = raw
			}
		}
		if r.WorkflowRationale != "" {
			if raw := marshalField(log, "rationale", r.WorkflowRationale); raw != nil {
				sw["rationale"] = raw
			}
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
			if raw := marshalField(log, "detected_label:"+k, v); raw != nil {
				dl[k] = raw
			}
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
		return agentclient.HumanReviewReasonAlignmentCheckFailed, false
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

// marshalField marshals v to JSON. On failure it logs the error and returns nil.
func marshalField(log logr.Logger, key string, v interface{}) jx.Raw {
	b, err := json.Marshal(v)
	if err != nil {
		log.Error(err, "failed to marshal response field", "field", key)
		return nil
	}
	return jx.Raw(b)
}
