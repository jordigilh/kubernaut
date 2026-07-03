package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"github.com/google/jsonschema-go/jsonschema"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

// ListRemediationsArgs defines the input for kubernaut_list_remediations.
type ListRemediationsArgs struct {
	Namespace string `json:"-"`
	Phase     string `json:"phase,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Name      string `json:"name,omitempty"`
}

// ListRemediationsResult is the output of kubernaut_list_remediations.
type ListRemediationsResult struct {
	Remediations []RemediationSummary `json:"remediations"`
	Count        int                  `json:"count"`
}

// RemediationSummary is a compact view of a remediation.
type RemediationSummary struct {
	ID        string `json:"id"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Phase     string `json:"phase"`
	Kind      string `json:"kind,omitempty"`
	Target    string `json:"target,omitempty"`
}

// HandleListRemediations implements the kubernaut_list_remediations logic.
func HandleListRemediations(ctx context.Context, client crclient.Client, args ListRemediationsArgs) (ListRemediationsResult, error) {
	if client == nil {
		return ListRemediationsResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return ListRemediationsResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	var list remediationv1.RemediationRequestList
	if err := client.List(ctx, &list, crclient.InNamespace(args.Namespace)); err != nil {
		return ListRemediationsResult{}, ToUserFriendlyError(err)
	}

	var result []RemediationSummary
	for i := range list.Items {
		item := &list.Items[i]
		phase := string(item.Status.OverallPhase)
		if args.Phase != "" && phase != args.Phase {
			continue
		}
		kind := item.Spec.TargetResource.Kind
		target := item.Spec.TargetResource.Name
		if args.Kind != "" && kind != args.Kind {
			continue
		}
		if args.Name != "" && target != args.Name {
			continue
		}
		result = append(result, RemediationSummary{
			ID:        item.Name,
			Namespace: item.Namespace,
			Name:      item.Name,
			Phase:     phase,
			Kind:      kind,
			Target:    target,
		})
	}

	return ListRemediationsResult{
		Remediations: result,
		Count:        len(result),
	}, nil
}

// NewListRemediationsTool creates the kubernaut_list_remediations tool.
func NewListRemediationsTool(client crclient.Client, controllerNS string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_list_remediations",
		Description: "List active remediations with optional filtering by phase or kind",
	}, func(ctx tool.Context, args ListRemediationsArgs) (ListRemediationsResult, error) {
		args.Namespace = controllerNS
		return HandleListRemediations(ctx, client, args)
	})
}

// GetRemediationArgs defines the input for kubernaut_get_remediation.
type GetRemediationArgs struct {
	Namespace string `json:"-"`
	Name      string `json:"name,omitempty"`
	RRID      string `json:"rr_id,omitempty"`
}

// GetRemediationResult is the output of kubernaut_get_remediation.
type GetRemediationResult struct {
	ID        string `json:"id"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Phase     string `json:"phase"`
	Kind      string `json:"kind,omitempty"`
	Target    string `json:"target,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// HandleGetRemediation implements the kubernaut_get_remediation logic.
func HandleGetRemediation(ctx context.Context, client crclient.Client, args GetRemediationArgs) (GetRemediationResult, error) {
	if client == nil {
		return GetRemediationResult{}, ErrK8sUnavailable
	}
	ns, name, err := ParseRRID(args.RRID, args.Namespace, args.Name)
	if err != nil {
		return GetRemediationResult{}, err
	}

	var rr remediationv1.RemediationRequest
	if err := client.Get(ctx, crclient.ObjectKey{Namespace: ns, Name: name}, &rr); err != nil {
		return GetRemediationResult{}, ToUserFriendlyError(err)
	}

	return GetRemediationResult{
		ID:        name,
		Namespace: ns,
		Name:      name,
		Phase:     string(rr.Status.OverallPhase),
		Kind:      rr.Spec.TargetResource.Kind,
		Target:    rr.Spec.TargetResource.Name,
	}, nil
}

// NewGetRemediationTool creates the kubernaut_get_remediation tool.
// controllerNS is always injected as the namespace (all RR CRDs live in the
// controller namespace per ADR-057). The namespace field is hidden from the LLM.
func NewGetRemediationTool(client crclient.Client, controllerNS string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_get_remediation",
		Description: "Get detailed information about a specific remediation by rr_id or name",
	}, func(ctx tool.Context, args GetRemediationArgs) (GetRemediationResult, error) {
		args.Namespace = controllerNS
		return HandleGetRemediation(ctx, client, args)
	})
}

// ────────────────────────────────────────────────────────────────────────────
// kubernaut_list_approval_requests
// ────────────────────────────────────────────────────────────────────────────

// ListApprovalRequestsArgs defines the input for kubernaut_list_approval_requests.
type ListApprovalRequestsArgs struct {
	Namespace string `json:"-"`
	Decision  string `json:"decision,omitempty"`
}

// AuditFields implements AuditableInput for audit enrichment.
func (a ListApprovalRequestsArgs) AuditFields() map[string]string {
	return map[string]string{"namespace": a.Namespace}
}

// ListApprovalRequestsResult is the output of kubernaut_list_approval_requests.
type ListApprovalRequestsResult struct {
	ApprovalRequests []ApprovalRequestSummary `json:"approval_requests"`
	Count            int                      `json:"count"`
}

// ApprovalRequestSummary is a compact view of a RemediationApprovalRequest.
type ApprovalRequestSummary struct {
	Name               string  `json:"name"`
	Namespace          string  `json:"namespace"`
	Decision           string  `json:"decision"`
	RemediationRequest string  `json:"remediation_request"`
	Confidence         float64 `json:"confidence"`
	ConfidenceLevel    string  `json:"confidence_level"`
	TimeRemaining      string  `json:"time_remaining,omitempty"`
	RequiredBy         string  `json:"required_by,omitempty"`
}

// HandleListApprovalRequests implements the kubernaut_list_approval_requests logic.
func HandleListApprovalRequests(ctx context.Context, client crclient.Client, args ListApprovalRequestsArgs) (ListApprovalRequestsResult, error) {
	if client == nil {
		return ListApprovalRequestsResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return ListApprovalRequestsResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	var list remediationv1.RemediationApprovalRequestList
	if err := client.List(ctx, &list, crclient.InNamespace(args.Namespace)); err != nil {
		return ListApprovalRequestsResult{}, ToUserFriendlyError(err)
	}

	result := make([]ApprovalRequestSummary, 0)
	for i := range list.Items {
		item := &list.Items[i]
		decision := string(item.Status.Decision)
		if !matchesDecisionFilter(decision, args.Decision) {
			continue
		}

		displayDecision := decision
		if displayDecision == "" {
			displayDecision = "Pending"
		}

		result = append(result, ApprovalRequestSummary{
			Name:               item.Name,
			Namespace:          item.Namespace,
			Decision:           displayDecision,
			RemediationRequest: item.Spec.RemediationRequestRef.Name,
			Confidence:         item.Spec.Confidence,
			ConfidenceLevel:    item.Spec.ConfidenceLevel,
			TimeRemaining:      item.Status.TimeRemaining,
			RequiredBy:         item.Spec.RequiredBy.Format(time.RFC3339),
		})
	}

	return ListApprovalRequestsResult{
		ApprovalRequests: result,
		Count:            len(result),
	}, nil
}

// matchesDecisionFilter checks if a RAR's decision matches the requested filter.
// Empty filter means all. "pending" matches empty decision (no decision yet).
func matchesDecisionFilter(actual, filter string) bool {
	if filter == "" {
		return true
	}
	normalized := strings.ToLower(filter)
	if normalized == "pending" {
		return actual == ""
	}
	return strings.EqualFold(actual, filter)
}

// NewListApprovalRequestsTool creates the kubernaut_list_approval_requests tool.
func NewListApprovalRequestsTool(client crclient.Client, controllerNS string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_list_approval_requests",
		Description: "List remediation approval requests with optional filtering by decision status (pending, approved, rejected, expired)",
	}, func(ctx tool.Context, args ListApprovalRequestsArgs) (ListApprovalRequestsResult, error) {
		args.Namespace = controllerNS
		return HandleListApprovalRequests(ctx, client, args)
	})
}

// ────────────────────────────────────────────────────────────────────────────
// kubernaut_get_approval_request
// ────────────────────────────────────────────────────────────────────────────

// GetApprovalRequestArgs defines the input for kubernaut_get_approval_request.
type GetApprovalRequestArgs struct {
	Namespace string `json:"-"`
	Name      string `json:"name,omitempty"`
	RARID     string `json:"rar_id,omitempty"`
}

// AuditFields implements AuditableInput for audit enrichment.
func (a GetApprovalRequestArgs) AuditFields() map[string]string {
	fields := map[string]string{}
	if a.RARID != "" {
		fields["resource_id"] = a.RARID
	} else {
		if a.Namespace != "" {
			fields["namespace"] = a.Namespace
		}
		if a.Name != "" {
			fields["resource_name"] = a.Name
		}
	}
	return fields
}

// GetApprovalRequestResult is the detailed output of kubernaut_get_approval_request.
type GetApprovalRequestResult struct {
	Name                   string                     `json:"name"`
	Namespace              string                     `json:"namespace"`
	RemediationRequest     string                     `json:"remediation_request"`
	AIAnalysis             string                     `json:"ai_analysis"`
	Confidence             float64                    `json:"confidence"`
	ConfidenceLevel        string                     `json:"confidence_level"`
	Reason                 string                     `json:"reason"`
	InvestigationSummary   string                     `json:"investigation_summary"`
	WhyApprovalRequired    string                     `json:"why_approval_required"`
	RecommendedWorkflow    RecommendedWorkflowInfo    `json:"recommended_workflow"`
	RecommendedActions     []RecommendedActionSummary `json:"recommended_actions"`
	EvidenceCollected      []string                   `json:"evidence_collected"`
	AlternativesConsidered []AlternativeSummary       `json:"alternatives_considered"`
	RequiredBy             string                     `json:"required_by,omitempty"`
	Decision               string                     `json:"decision"`
	DecidedBy              string                     `json:"decided_by,omitempty"`
	DecidedAt              string                     `json:"decided_at,omitempty"`
	TimeRemaining          string                     `json:"time_remaining,omitempty"`
	Expired                bool                       `json:"expired"`
}

// RecommendedWorkflowInfo is a compact view of a recommended workflow in an approval request.
type RecommendedWorkflowInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// RecommendedActionSummary is a compact view of a recommended action.
type RecommendedActionSummary struct {
	Action    string `json:"action"`
	Rationale string `json:"rationale,omitempty"`
}

// AlternativeSummary is a compact view of an alternative considered.
type AlternativeSummary struct {
	Approach string `json:"approach"`
	ProsCons string `json:"pros_cons,omitempty"`
}

// HandleGetApprovalRequest implements the kubernaut_get_approval_request logic.
func HandleGetApprovalRequest(ctx context.Context, client crclient.Client, args GetApprovalRequestArgs) (GetApprovalRequestResult, error) {
	if client == nil {
		return GetApprovalRequestResult{}, ErrK8sUnavailable
	}

	ns, name, err := ParseRARID(args.RARID, args.Namespace, args.Name)
	if err != nil {
		return GetApprovalRequestResult{}, err
	}
	if errV := validate.Namespace(ns); errV != nil {
		return GetApprovalRequestResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, errV)
	}
	if errV := validate.ResourceName(name); errV != nil {
		return GetApprovalRequestResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, errV)
	}

	var rar remediationv1.RemediationApprovalRequest
	if err := client.Get(ctx, crclient.ObjectKey{Namespace: ns, Name: name}, &rar); err != nil {
		return GetApprovalRequestResult{}, ToUserFriendlyError(err)
	}

	evidence := rar.Spec.EvidenceCollected
	if evidence == nil {
		evidence = []string{}
	}

	actions := make([]RecommendedActionSummary, 0, len(rar.Spec.RecommendedActions))
	for _, a := range rar.Spec.RecommendedActions {
		actions = append(actions, RecommendedActionSummary{Action: a.Action, Rationale: a.Rationale})
	}

	alternatives := make([]AlternativeSummary, 0, len(rar.Spec.AlternativesConsidered))
	for _, a := range rar.Spec.AlternativesConsidered {
		alternatives = append(alternatives, AlternativeSummary{Approach: a.Approach, ProsCons: a.ProsCons})
	}

	decision := string(rar.Status.Decision)
	displayDecision := decision
	if displayDecision == "" {
		displayDecision = "Pending"
	}

	decidedAt := ""
	if rar.Status.DecidedAt != nil {
		decidedAt = rar.Status.DecidedAt.Format(time.RFC3339)
	}

	requiredBy := ""
	if !rar.Spec.RequiredBy.IsZero() {
		requiredBy = rar.Spec.RequiredBy.Format(time.RFC3339)
	}

	return GetApprovalRequestResult{
		Name:                   rar.Name,
		Namespace:              rar.Namespace,
		RemediationRequest:     rar.Spec.RemediationRequestRef.Name,
		AIAnalysis:             rar.Spec.AIAnalysisRef.Name,
		Confidence:             rar.Spec.Confidence,
		ConfidenceLevel:        rar.Spec.ConfidenceLevel,
		Reason:                 rar.Spec.Reason,
		InvestigationSummary:   rar.Spec.InvestigationSummary,
		WhyApprovalRequired:    rar.Spec.WhyApprovalRequired,
		RecommendedWorkflow:    RecommendedWorkflowInfo{Name: rar.Spec.RecommendedWorkflow.WorkflowID, Version: rar.Spec.RecommendedWorkflow.Version},
		RecommendedActions:     actions,
		EvidenceCollected:      evidence,
		AlternativesConsidered: alternatives,
		RequiredBy:             requiredBy,
		Decision:               displayDecision,
		DecidedBy:              rar.Status.DecidedBy,
		DecidedAt:              decidedAt,
		TimeRemaining:          rar.Status.TimeRemaining,
		Expired:                rar.Status.Expired,
	}, nil
}

// NewGetApprovalRequestTool creates the kubernaut_get_approval_request tool.
func NewGetApprovalRequestTool(client crclient.Client, controllerNS string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_get_approval_request",
		Description: "Get full details of a specific remediation approval request for review before deciding",
	}, func(ctx tool.Context, args GetApprovalRequestArgs) (GetApprovalRequestResult, error) {
		args.Namespace = controllerNS
		return HandleGetApprovalRequest(ctx, client, args)
	})
}

// ApproveArgs defines the input for kubernaut_approve.
type ApproveArgs struct {
	Namespace        string `json:"-"`
	RARName          string `json:"rar_name"`
	Decision         string `json:"decision"`
	Reason           string `json:"reason,omitempty"`
	WorkflowOverride string `json:"workflow_override,omitempty"`
}

// ApproveResult is the output of kubernaut_approve.
type ApproveResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// HandleApprove implements the kubernaut_approve logic.
//
//nolint:gocritic // hugeParam: args passed by value for simplicity; not performance-critical
func HandleApprove(ctx context.Context, client crclient.Client, args ApproveArgs, username string) (ApproveResult, error) {
	if client == nil {
		return ApproveResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return ApproveResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := validate.ResourceName(args.RARName); err != nil {
		return ApproveResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if args.Decision == "" {
		return ApproveResult{}, fmt.Errorf("%w: decision must not be empty", ErrInvalidInput)
	}
	if args.Decision != "Approved" && args.Decision != "Rejected" {
		return ApproveResult{}, fmt.Errorf("%w: decision %q is not valid; must be exactly one of: Approved, Rejected", ErrInvalidInput, args.Decision)
	}

	var rar remediationv1.RemediationApprovalRequest
	if err := client.Get(ctx, crclient.ObjectKey{Namespace: args.Namespace, Name: args.RARName}, &rar); err != nil {
		return ApproveResult{}, ToUserFriendlyError(err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	statusPatch := map[string]interface{}{
		"decision":  args.Decision,
		"decidedBy": username,
		"decidedAt": now,
	}
	if args.Reason != "" {
		statusPatch["decisionMessage"] = args.Reason
	}
	if args.WorkflowOverride != "" {
		statusPatch["workflowOverride"] = map[string]interface{}{
			"workflowName": args.WorkflowOverride,
			"rationale":    "User override via kubernaut_approve",
		}
	}
	patch := map[string]interface{}{
		"status": statusPatch,
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return ApproveResult{}, fmt.Errorf("marshaling patch: %w", err)
	}

	if err := client.Status().Patch(ctx, &rar, crclient.RawPatch(types.MergePatchType, patchBytes)); err != nil {
		return ApproveResult{}, ToUserFriendlyError(err)
	}

	return ApproveResult{
		Status:  args.Decision,
		Message: fmt.Sprintf("Remediation approval %s by %s", args.Decision, username),
	}, nil
}

// NewApproveTool creates the kubernaut_approve tool.
func NewApproveTool(client crclient.Client, controllerNS string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_approve",
		Description: "Approve or reject a pending remediation approval request. The decision field accepts exactly 'Approved' or 'Rejected'.",
		InputSchema: approveInputSchema(),
	}, func(ctx tool.Context, args ApproveArgs) (ApproveResult, error) {
		args.Namespace = controllerNS
		return HandleApprove(ctx, client, args, usernameFromContext(ctx))
	})
}

// approveInputSchema returns a JSON schema with an enum constraint on the
// decision field, so the LLM sees the valid values directly in the tool definition.
func approveInputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"rar_name": {
				Type:        "string",
				Description: "Name of the RemediationApprovalRequest resource to approve or reject",
			},
			"decision": {
				Type:        "string",
				Description: "The approval decision",
				Enum:        []any{"Approved", "Rejected"},
			},
			"reason": {
				Type:        "string",
				Description: "Optional reason for the decision",
			},
			"workflow_override": {
				Type:        "string",
				Description: "Optional workflow name to override the recommended workflow",
			},
		},
		Required:      []string{"rar_name", "decision"},
		PropertyOrder: []string{"rar_name", "decision", "reason", "workflow_override"},
	}
}

// usernameFromContext extracts the authenticated username from tool context.
// Falls back to "system" when no identity is present (e.g. in tests).
func usernameFromContext(ctx context.Context) string {
	if identity := auth.UserIdentityFromContext(ctx); identity != nil && identity.Username != "" {
		return identity.Username
	}
	return "system"
}

// CancelRemediationArgs defines the input for kubernaut_cancel_remediation.
type CancelRemediationArgs struct {
	Namespace string `json:"-"`
	Name      string `json:"name,omitempty"`
	RRID      string `json:"rr_id,omitempty"`
}

// CancelRemediationResult is the output of kubernaut_cancel_remediation.
type CancelRemediationResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// HandleCancelRemediation implements the kubernaut_cancel_remediation logic.
func HandleCancelRemediation(ctx context.Context, client crclient.Client, args CancelRemediationArgs) (CancelRemediationResult, error) {
	if client == nil {
		return CancelRemediationResult{}, ErrK8sUnavailable
	}
	ns, name, err := ParseRRID(args.RRID, args.Namespace, args.Name)
	if err != nil {
		return CancelRemediationResult{}, err
	}

	var rr remediationv1.RemediationRequest
	if err := client.Get(ctx, crclient.ObjectKey{Namespace: ns, Name: name}, &rr); err != nil {
		return CancelRemediationResult{}, ToUserFriendlyError(err)
	}

	if IsTerminalPhase(string(rr.Status.OverallPhase)) {
		return CancelRemediationResult{}, fmt.Errorf("%w: remediation %s/%s is in terminal state %q", ErrAlreadyTerminal, ns, name, rr.Status.OverallPhase)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	patch := map[string]interface{}{
		"status": map[string]interface{}{
			"overallPhase":   "Cancelled",
			"completedAt":    now,
			"lastModifiedAt": now,
			"message":        "Cancelled by user via kubernaut_cancel_remediation",
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return CancelRemediationResult{}, fmt.Errorf("marshaling patch: %w", err)
	}

	if err := client.Status().Patch(ctx, &rr, crclient.RawPatch(types.MergePatchType, patchBytes)); err != nil {
		return CancelRemediationResult{}, ToUserFriendlyError(err)
	}

	return CancelRemediationResult{
		Status:  "Cancelled",
		Message: fmt.Sprintf("Remediation %s/%s cancelled", ns, name),
	}, nil
}

// NewCancelRemediationTool creates the kubernaut_cancel_remediation tool.
// controllerNS is always injected as the namespace (all RR CRDs live in the
// controller namespace per ADR-057). The namespace field is hidden from the LLM.
func NewCancelRemediationTool(client crclient.Client, controllerNS string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_cancel_remediation",
		Description: "Cancel an active remediation that has not yet reached a terminal state",
	}, func(ctx tool.Context, args CancelRemediationArgs) (CancelRemediationResult, error) {
		args.Namespace = controllerNS
		return HandleCancelRemediation(ctx, client, args)
	})
}
