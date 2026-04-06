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
	"log/slog"
	"strings"
	"time"

	"github.com/go-faster/jx"

	hapiclient "github.com/jordigilh/kubernaut/pkg/agentclient"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

// Handler implements the ogen-generated Handler interface for the 3 business
// endpoints. Operational endpoints (/health, /ready, /config, /metrics) are
// served directly by the HTTP mux in cmd/kubernautagent/main.go.
type Handler struct {
	hapiclient.UnimplementedHandler

	sessions     *session.Manager
	investigator *investigator.Investigator
	logger       *slog.Logger
}

var _ hapiclient.Handler = (*Handler)(nil)

// NewHandler creates a Kubernaut Agent ogen handler.
func NewHandler(sessions *session.Manager, inv *investigator.Investigator, logger *slog.Logger) *Handler {
	return &Handler{
		sessions:     sessions,
		investigator: inv,
		logger:       logger,
	}
}

// IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost implements POST /api/v1/incident/analyze.
// Returns HTTP 202 with {"session_id": "<uuid>"}.
func (h *Handler) IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(
	ctx context.Context, req *hapiclient.IncidentRequest,
) (hapiclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostRes, error) {
	if h.investigator == nil {
		h.logger.Error("investigator not configured")
		resp := hapiclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostApplicationJSONInternalServerError{
			Detail: "investigator not configured",
		}
		return &resp, nil
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
	sessionID, err := h.sessions.StartInvestigation(ctx, func(bgCtx context.Context) (interface{}, error) {
		return h.investigator.Investigate(bgCtx, signal)
	}, metadata)
	if err != nil {
		h.logger.Error("failed to start investigation", "error", err)
		resp := hapiclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostApplicationJSONInternalServerError{
			Detail: "failed to start investigation: " + err.Error(),
		}
		return &resp, nil
	}

	body, _ := json.Marshal(map[string]string{"session_id": sessionID})
	raw := hapiclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostAcceptedApplicationJSON(body)
	return &raw, nil
}

// IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet implements GET /api/v1/incident/session/{session_id}.
func (h *Handler) IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(
	_ context.Context,
	params hapiclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams,
) (hapiclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetRes, error) {
	sess, err := h.sessions.GetSession(params.SessionID)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return &hapiclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetNotFound{}, nil
		}
		h.logger.Error("session lookup failed", "session_id", params.SessionID, "error", err)
		return nil, fmt.Errorf("session lookup: %w", err)
	}

	status := mapSessionStatusToAPI(sess.Status)
	body, _ := json.Marshal(map[string]string{
		"session_id": sess.ID,
		"status":     status,
	})
	raw := hapiclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetOKApplicationJSON(body)
	return &raw, nil
}

// IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet implements
// GET /api/v1/incident/session/{session_id}/result.
func (h *Handler) IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(
	_ context.Context,
	params hapiclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams,
) (hapiclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetRes, error) {
	sess, err := h.sessions.GetSession(params.SessionID)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return &hapiclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetNotFound{}, nil
		}
		h.logger.Error("session lookup failed", "session_id", params.SessionID, "error", err)
		return nil, fmt.Errorf("session lookup: %w", err)
	}

	if sess.Status != session.StatusCompleted {
		return &hapiclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetConflict{}, nil
	}

	result, ok := sess.Result.(*katypes.InvestigationResult)
	if !ok {
		h.logger.Error("unexpected result type in session", "session_id", sess.ID)
		return &hapiclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetConflict{}, nil
	}

	var incidentID string
	if sess.Metadata != nil {
		incidentID = sess.Metadata["incident_id"]
	}

	resp := mapInvestigationResultToResponse(result, incidentID)
	return resp, nil
}

// MapIncidentRequestToSignal converts an OpenAPI IncidentRequest to an internal SignalContext.
func MapIncidentRequestToSignal(req *hapiclient.IncidentRequest) katypes.SignalContext {
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

func mapInvestigationResultToResponse(r *katypes.InvestigationResult, incidentID string) *hapiclient.IncidentResponse {
	resp := &hapiclient.IncidentResponse{
		IncidentID: incidentID,
		Analysis:   r.RCASummary,
		Confidence: r.Confidence,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	rca := hapiclient.IncidentResponseRootCauseAnalysis{}
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
		rca["remediation_target"] = jx.Raw(targetRaw)
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
		sw := hapiclient.IncidentResponseSelectedWorkflow{}
		wfIDRaw, _ := json.Marshal(r.WorkflowID)
		sw["workflow_id"] = jx.Raw(wfIDRaw)
		if len(r.Parameters) > 0 {
			paramsRaw, _ := json.Marshal(r.Parameters)
			sw["parameters"] = jx.Raw(paramsRaw)
		}
		confRaw, _ := json.Marshal(r.Confidence)
		sw["confidence"] = jx.Raw(confRaw)
		// GAP-009: Include execution_bundle in selected_workflow per OpenAPI schema
		if r.ExecutionBundle != "" {
			ebRaw, _ := json.Marshal(r.ExecutionBundle)
			sw["execution_bundle"] = jx.Raw(ebRaw)
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
	}

	if len(r.DetectedLabels) > 0 {
		dl := make(hapiclient.IncidentResponseDetectedLabels, len(r.DetectedLabels))
		for k, v := range r.DetectedLabels {
			raw, _ := json.Marshal(v)
			dl[k] = jx.Raw(raw)
		}
		resp.DetectedLabels.SetTo(dl)
	}

	if len(r.AlternativeWorkflows) > 0 {
		alts := make([]hapiclient.AlternativeWorkflow, 0, len(r.AlternativeWorkflows))
		for _, aw := range r.AlternativeWorkflows {
			alt := hapiclient.AlternativeWorkflow{
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
		attempts := make([]hapiclient.ValidationAttempt, 0, len(r.ValidationAttemptsHistory))
		for _, va := range r.ValidationAttemptsHistory {
			attempt := hapiclient.ValidationAttempt{
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
func mapHumanReviewReason(reason string) (hapiclient.HumanReviewReason, bool) {
	switch reason {
	case "rca_incomplete":
		return hapiclient.HumanReviewReasonRcaIncomplete, false
	case "investigation_inconclusive":
		return hapiclient.HumanReviewReasonInvestigationInconclusive, false
	case "workflow_not_found":
		return hapiclient.HumanReviewReasonWorkflowNotFound, false
	case "no_matching_workflows":
		return hapiclient.HumanReviewReasonNoMatchingWorkflows, false
	case "image_mismatch":
		return hapiclient.HumanReviewReasonImageMismatch, false
	case "parameter_validation_failed":
		return hapiclient.HumanReviewReasonParameterValidationFailed, false
	case "low_confidence":
		return hapiclient.HumanReviewReasonLowConfidence, false
	case "llm_parsing_error":
		return hapiclient.HumanReviewReasonLlmParsingError, false
	}

	switch {
	case strings.Contains(reason, "exhausted during RCA"):
		return hapiclient.HumanReviewReasonRcaIncomplete, false
	case strings.Contains(reason, "exhausted during workflow selection"):
		return hapiclient.HumanReviewReasonInvestigationInconclusive, false
	case strings.Contains(reason, "not found") && strings.Contains(reason, "catalog"):
		return hapiclient.HumanReviewReasonWorkflowNotFound, false
	case strings.Contains(reason, "no matching"):
		return hapiclient.HumanReviewReasonNoMatchingWorkflows, false
	case strings.Contains(reason, "mismatch") || strings.Contains(reason, "image"):
		return hapiclient.HumanReviewReasonImageMismatch, false
	case strings.Contains(reason, "parameter") || strings.Contains(reason, "validation"):
		return hapiclient.HumanReviewReasonParameterValidationFailed, false
	case strings.Contains(reason, "confidence"):
		return hapiclient.HumanReviewReasonLowConfidence, false
	case strings.Contains(reason, "parse") || strings.Contains(reason, "parsing"):
		return hapiclient.HumanReviewReasonLlmParsingError, false
	default:
		return hapiclient.HumanReviewReasonInvestigationInconclusive, true
	}
}
