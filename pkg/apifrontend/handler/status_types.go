package handler

import (
	"fmt"
	"time"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
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
	phase := rr.Status.OverallPhase

	switch phase {
	case remediationv1.PhaseExecuting:
		if rr.Status.SelectedWorkflowRef != nil {
			meta["workflow_id"] = rr.Status.SelectedWorkflowRef.WorkflowID
		}
		if rr.Status.ExecutingStartTime != nil {
			meta["started_at"] = rr.Status.ExecutingStartTime.Time.Format(time.RFC3339)
		}

	case remediationv1.PhaseVerifying:
		if rr.Status.VerificationDeadline != nil {
			meta["verification_deadline"] = rr.Status.VerificationDeadline.Time.Format(time.RFC3339)
		}
		if rr.Status.ExecutingStartTime != nil {
			meta["started_at"] = rr.Status.ExecutingStartTime.Time.Format(time.RFC3339)
		}
		if ea != nil {
			meta["ea_phase"] = ea.Status.Phase
			if ea.Status.PrometheusCheckAfter != nil {
				meta["stabilization_deadline"] = ea.Status.PrometheusCheckAfter.Time.Format(time.RFC3339)
			}
		}

	case remediationv1.PhaseBlocked:
		if rr.Status.BlockedUntil != nil {
			meta["blocked_until"] = rr.Status.BlockedUntil.Time.Format(time.RFC3339)
		}
		if rr.Status.BlockReason != "" {
			meta["block_reason"] = string(rr.Status.BlockReason)
		}
		if rr.Status.BlockMessage != "" {
			meta["block_message"] = rr.Status.BlockMessage
		}

	case remediationv1.PhaseAwaitingApproval:
		rarName := fmt.Sprintf("rar-%s", rr.Name)
		meta["approval_request_name"] = rarName

	case remediationv1.PhaseCompleted:
		if rr.Status.Outcome != "" {
			meta["outcome"] = rr.Status.Outcome
		}

	case remediationv1.PhaseFailed:
		if rr.Status.FailureReason != nil {
			meta["failure_reason"] = *rr.Status.FailureReason
		}
		if rr.Status.FailurePhase != nil {
			meta["failure_phase"] = string(*rr.Status.FailurePhase)
		}

	case remediationv1.PhaseTimedOut:
		if rr.Status.TimeoutPhase != nil {
			meta["failure_phase"] = string(*rr.Status.TimeoutPhase)
		}

	case remediationv1.PhaseSkipped:
		if rr.Status.SkipReason != "" {
			meta["skip_reason"] = string(rr.Status.SkipReason)
		}
	}

	return meta
}
