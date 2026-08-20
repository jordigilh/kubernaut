package handler

import (
	"fmt"
	"time"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

// StatusSubscribeRequest represents a JSON-RPC 2.0 status/subscribe request.
type StatusSubscribeRequest struct {
	JSONRPC string                `json:"jsonrpc"`
	ID      any                   `json:"id"`
	Method  string                `json:"method"`
	Params  StatusSubscribeParams `json:"params"`
}

// StatusSubscribeParams contains the parameters for a status/subscribe request.
type StatusSubscribeParams struct {
	RRID string `json:"rr_id"`
}

// StatusUpdateParams represents the params of a status/update SSE event.
type StatusUpdateParams struct {
	RRID      string         `json:"rr_id"`
	Phase     string         `json:"phase"`
	Timestamp string         `json:"timestamp"`
	Final     bool           `json:"final"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// StatusClosingParams represents the params of a status/closing SSE event.
type StatusClosingParams struct {
	Reason    string `json:"reason"`
	Reconnect bool   `json:"reconnect"`
}

// jsonRPCError represents a JSON-RPC 2.0 error object.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC error codes per the agreed contract (DD-AF-008).
const (
	errCodeInvalidRequest = -32600
	errCodeMethodNotFound = -32601
	errCodeInvalidParams  = -32602
	errCodeRRNotFound     = -32001
	// errCodeAccessDenied is reserved for future per-resource authz (SAR).
	// Today all authenticated users are implicitly authorized; the auth
	// middleware returns HTTP 401/403 before reaching the handler.
	errCodeAccessDenied = -32002 //nolint:unused // reserved per DD-AF-008 contract
)

// BuildPhaseMetadata constructs per-phase metadata from CRD status fields.
// The returned map uses raw CRD field names per the agreed console contract.
func BuildPhaseMetadata(rr *remediationv1.RemediationRequest, ea *eav1alpha1.EffectivenessAssessment) map[string]any {
	meta := make(map[string]any)

	// RR identity context — present in every phase so the console can
	// populate the investigation banner on reconnect (#1468).
	// Field names match RRContext in event_bridge.go for cross-path
	// consistency (AU-3 traceability, SC-7 server-sourced, SI-4 monitoring).
	if rr.Spec.TargetResource.Namespace != "" {
		meta["namespace"] = rr.Spec.TargetResource.Namespace
	}
	if targetDisplay := remediationrequest.FormatResourceDisplay(
		rr.Spec.TargetResource.Kind, rr.Spec.TargetResource.Name); targetDisplay != "" {
		meta["target"] = targetDisplay
	}
	if rr.Spec.TargetResource.Kind != "" {
		meta["kind"] = rr.Spec.TargetResource.Kind
	}
	if rr.Spec.SignalName != "" {
		meta["alert_name"] = rr.Spec.SignalName
	}

	addPhaseSpecificMetadata(meta, rr, ea)

	return meta
}

// addPhaseSpecificMetadata dispatches to the per-phase metadata builder for
// rr's current OverallPhase (split out of BuildPhaseMetadata to keep both
// functions under the complexity gate).
func addPhaseSpecificMetadata(meta map[string]any, rr *remediationv1.RemediationRequest, ea *eav1alpha1.EffectivenessAssessment) {
	switch rr.Status.OverallPhase {
	case remediationv1.PhaseExecuting:
		addExecutingPhaseMetadata(meta, rr)
	case remediationv1.PhaseVerifying:
		addVerifyingPhaseMetadata(meta, rr, ea)
	case remediationv1.PhaseBlocked:
		addBlockedPhaseMetadata(meta, rr)
	case remediationv1.PhaseAwaitingApproval:
		// #1959: bare name only — every RAR lives in the controller namespace
		// (ADR-057), so a namespace prefix here is redundant. ParseRARID
		// resolves rar_id the same way ParseRRID resolves rr_id: namespace
		// always comes from the injected controllerNS, never from the ID.
		meta["approval_request_name"] = fmt.Sprintf("rar-%s", rr.Name)
	case remediationv1.PhaseCompleted:
		if outcome := rr.Status.GetCompletionStatus().Outcome; outcome != "" {
			meta["outcome"] = outcome
		}
	case remediationv1.PhaseFailed:
		addFailedPhaseMetadata(meta, rr)
	case remediationv1.PhaseTimedOut:
		if tp := rr.Status.GetCompletionStatus().TimeoutPhase; tp != nil {
			meta["failure_phase"] = string(*tp)
		}
	case remediationv1.PhaseSkipped:
		if skipReason := rr.Status.GetRoutingStatus().SkipReason; skipReason != "" {
			meta["skip_reason"] = string(skipReason)
		}
	default:
		// Pending/Processing/Analyzing/Cancelled have no phase-specific
		// metadata to add beyond the generic fields BuildPhaseMetadata
		// already set before calling here.
	}
}

// addExecutingPhaseMetadata populates phase metadata for PhaseExecuting.
func addExecutingPhaseMetadata(meta map[string]any, rr *remediationv1.RemediationRequest) {
	if wf := rr.Status.GetWorkflowSelection().SelectedWorkflowRef; wf != nil {
		meta["workflow_id"] = wf.WorkflowID
	}
	if st := rr.Status.GetPhaseProgress().ExecutingStartTime; st != nil {
		meta["started_at"] = st.Format(time.RFC3339)
	}
}

// addVerifyingPhaseMetadata populates phase metadata for PhaseVerifying.
func addVerifyingPhaseMetadata(meta map[string]any, rr *remediationv1.RemediationRequest, ea *eav1alpha1.EffectivenessAssessment) {
	if vd := rr.Status.GetPhaseProgress().VerificationDeadline; vd != nil {
		meta["verification_deadline"] = vd.Format(time.RFC3339)
	}
	if st := rr.Status.GetPhaseProgress().ExecutingStartTime; st != nil {
		meta["started_at"] = st.Format(time.RFC3339)
	}
	if ea != nil {
		meta["ea_phase"] = ea.Status.Phase
		if ea.Status.PrometheusCheckAfter != nil {
			meta["stabilization_deadline"] = ea.Status.PrometheusCheckAfter.Format(time.RFC3339)
		}
	}
}

// addBlockedPhaseMetadata populates phase metadata for PhaseBlocked.
func addBlockedPhaseMetadata(meta map[string]any, rr *remediationv1.RemediationRequest) {
	routing := rr.Status.GetRoutingStatus()
	if routing.BlockedUntil != nil {
		meta["blocked_until"] = routing.BlockedUntil.Format(time.RFC3339)
	}
	if routing.BlockReason != "" {
		meta["block_reason"] = string(routing.BlockReason)
	}
	if routing.BlockMessage != "" {
		meta["block_message"] = routing.BlockMessage
	}
}

// addFailedPhaseMetadata populates phase metadata for PhaseFailed.
func addFailedPhaseMetadata(meta map[string]any, rr *remediationv1.RemediationRequest) {
	completion := rr.Status.GetCompletionStatus()
	if completion.FailureReason != nil {
		meta["failure_reason"] = *completion.FailureReason
	}
	if completion.FailurePhase != nil {
		meta["failure_phase"] = string(*completion.FailurePhase)
	}
}
