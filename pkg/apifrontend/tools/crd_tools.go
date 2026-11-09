package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/google/jsonschema-go/jsonschema"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"

	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
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
		return ListRemediationsResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
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
	}, func(ctx agent.Context, args ListRemediationsArgs) (ListRemediationsResult, error) {
		args.Namespace = controllerNS
		return HandleListRemediations(ctx, client, args)
	})
}

// GetRemediationArgs defines the input for kubernaut_get_remediation.
type GetRemediationArgs struct {
	Namespace string `json:"-"`
	Name      string `json:"name,omitempty"`
	RRID      string `json:"rr_id,omitempty"`
	// ClusterID and SessionID are ambient hints the LLM propagates from fleet
	// context and cross-phase preservation (#2364 Tier 2). Tolerated here so
	// strict ADK schema validation does not kill the turn; ignored by the
	// handler.
	ClusterID string `json:"cluster_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
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
	}, func(ctx agent.Context, args GetRemediationArgs) (GetRemediationResult, error) {
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
	RCAConfidence *float64 `json:"rca_confidence,omitempty"`
	// WorkflowConfidence is the canonical workflow-selection confidence.
	WorkflowConfidence float64 `json:"workflow_confidence"`
	Name               string  `json:"name"`
	Namespace          string  `json:"namespace"`
	Decision           string  `json:"decision"`
	RemediationRequest string  `json:"remediation_request"`
	// Confidence is retained as a legacy alias for compatibility with existing clients.
	// Deprecated: use WorkflowConfidence.
	Confidence      float64 `json:"confidence"`
	ConfidenceLevel string  `json:"confidence_level"`
	TimeRemaining   string  `json:"time_remaining,omitempty"`
	RequiredBy      string  `json:"required_by,omitempty"`
}

// HandleListApprovalRequests implements the kubernaut_list_approval_requests logic.
func HandleListApprovalRequests(ctx context.Context, client crclient.Client, args ListApprovalRequestsArgs) (ListApprovalRequestsResult, error) {
	if client == nil {
		return ListApprovalRequestsResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return ListApprovalRequestsResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
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
			RCAConfidence:      item.Spec.RCAConfidence,
			WorkflowConfidence: item.Spec.Confidence,
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
	}, func(ctx agent.Context, args ListApprovalRequestsArgs) (ListApprovalRequestsResult, error) {
		args.Namespace = controllerNS
		return HandleListApprovalRequests(ctx, client, args)
	})
}

// ────────────────────────────────────────────────────────────────────────────
// kubernaut_get_approval_request
// ────────────────────────────────────────────────────────────────────────────

// GetApprovalRequestArgs defines the input for kubernaut_get_approval_request.
type GetApprovalRequestArgs struct {
	Namespace             string `json:"-"`
	Name                  string `json:"name,omitempty"`
	RARID                 string `json:"rar_id,omitempty"`
	IncludeParentEvidence bool   `json:"include_parent_evidence,omitempty"`
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
	RCAConfidence *float64 `json:"rca_confidence,omitempty"`
	// WorkflowConfidence is the canonical workflow-selection confidence.
	WorkflowConfidence float64 `json:"workflow_confidence"`
	Name               string  `json:"name"`
	Namespace          string  `json:"namespace"`
	RemediationRequest string  `json:"remediation_request"`
	AIAnalysis         string  `json:"ai_analysis"`
	// Confidence is retained as a legacy alias for compatibility with existing clients.
	// Deprecated: use WorkflowConfidence.
	Confidence             float64                                              `json:"confidence"`
	ConfidenceLevel        string                                               `json:"confidence_level"`
	Reason                 string                                               `json:"reason"`
	InvestigationSummary   string                                               `json:"investigation_summary"`
	WhyApprovalRequired    string                                               `json:"why_approval_required"`
	RecommendedWorkflow    RecommendedWorkflowInfo                              `json:"recommended_workflow"`
	Workflow               *sharedtypes.WorkflowSnapshot                        `json:"workflow"`
	Parameters             map[string]string                                    `json:"parameters"`
	ParameterSchema        []remediationworkflowv1.RemediationWorkflowParameter `json:"parameter_schema"`
	RecommendedActions     []RecommendedActionSummary                           `json:"recommended_actions"`
	EvidenceCollected      []string                                             `json:"evidence_collected"`
	AlternativesConsidered []AlternativeSummary                                 `json:"alternatives_considered"`
	RequiredBy             string                                               `json:"required_by,omitempty"`
	Decision               string                                               `json:"decision"`
	DecidedBy              string                                               `json:"decided_by,omitempty"`
	DecidedAt              string                                               `json:"decided_at,omitempty"`
	TimeRemaining          string                                               `json:"time_remaining,omitempty"`
	Expired                bool                                                 `json:"expired"`
	ParentEvidence         *ParentEvidence                                      `json:"parent_evidence,omitempty"`
}

const (
	maxParentEvidenceStringBytes = 4096
	maxParentEvidenceBytes       = 16 * 1024
)

// ParentEvidence is a bounded, server-resolved projection of the owning RR.
// It intentionally contains no raw signal payload, labels, audit rows, or
// workflow override data.
type ParentEvidence struct {
	State     string            `json:"state"`
	Complete  bool              `json:"complete"`
	Truncated bool              `json:"truncated"`
	Parent    *ParentProjection `json:"parent,omitempty"`
}

// ParentProjection is the fixed allowlist exposed by the approval-review
// tool. Pointer fields preserve JSON null for absent optional source values.
type ParentProjection struct {
	Name           string                   `json:"name"`
	Namespace      string                   `json:"namespace"`
	UID            string                   `json:"uid"`
	OverallPhase   string                   `json:"overall_phase"`
	SignalSource   *string                  `json:"signal_source"`
	ClusterID      *string                  `json:"cluster_id"`
	TargetResource ParentTargetResource     `json:"target_resource"`
	Approval       ParentApprovalProjection `json:"approval"`
}

// ParentTargetResource is the bounded target allowlist.
type ParentTargetResource struct {
	Kind       string  `json:"kind"`
	Name       string  `json:"name"`
	Namespace  *string `json:"namespace"`
	APIVersion *string `json:"api_version"`
}

// ParentApprovalProjection carries the RAR's existing decision context,
// rather than reconstructing a new decision from the parent.
type ParentApprovalProjection struct {
	RequiredBy string  `json:"required_by"`
	Decision   *string `json:"decision"`
	DecidedBy  *string `json:"decided_by"`
	DecidedVia *string `json:"decided_via"`
	DecidedAt  *string `json:"decided_at"`
	Expired    bool    `json:"expired"`
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
		return GetApprovalRequestResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, errV)
	}
	if errV := validate.ResourceName(name); errV != nil {
		return GetApprovalRequestResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, errV)
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

	snapshot := rar.DeepCopy()
	result := GetApprovalRequestResult{
		RCAConfidence:          snapshot.Spec.RCAConfidence,
		WorkflowConfidence:     rar.Spec.Confidence,
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
		Workflow:               snapshot.Spec.Workflow,
		Parameters:             snapshot.Spec.Parameters,
		ParameterSchema:        snapshot.Spec.ParameterSchema,
		RecommendedActions:     actions,
		EvidenceCollected:      evidence,
		AlternativesConsidered: alternatives,
		RequiredBy:             requiredBy,
		Decision:               displayDecision,
		DecidedBy:              rar.Status.DecidedBy,
		DecidedAt:              decidedAt,
		TimeRemaining:          rar.Status.TimeRemaining,
		Expired:                rar.Status.Expired,
	}
	if args.IncludeParentEvidence {
		result.ParentEvidence = resolveParentEvidence(ctx, client, &rar)
	}
	return result, nil
}

func resolveParentEvidence(ctx context.Context, client crclient.Client, rar *remediationv1.RemediationApprovalRequest) *ParentEvidence {
	owner, found := approvalRequestOwner(rar)
	if !found {
		return &ParentEvidence{State: "missing", Complete: false}
	}

	parent, earlyResult := fetchParentEvidence(ctx, client, rar, owner)
	if earlyResult != nil {
		return earlyResult
	}
	if owner.UID != "" && owner.UID != parent.UID {
		return &ParentEvidence{State: "missing", Complete: false}
	}

	if !parentEvidenceHasRequiredFields(parent, rar) {
		return &ParentEvidence{State: "unavailable", Complete: false}
	}

	if !parentEvidenceStringsWithinBounds(parentEvidenceRequiredValues(parent, rar)) {
		return &ParentEvidence{State: "unavailable", Complete: false, Truncated: true}
	}

	truncated := false
	projection := parentEvidenceProjection(parent, rar, &truncated)

	evidence := &ParentEvidence{
		State:     "complete",
		Complete:  !truncated,
		Truncated: truncated,
		Parent:    projection,
	}
	return boundParentEvidence(evidence)
}

func fetchParentEvidence(ctx context.Context, client crclient.Client, rar *remediationv1.RemediationApprovalRequest, owner metav1.OwnerReference) (*remediationv1.RemediationRequest, *ParentEvidence) {
	parent := &remediationv1.RemediationRequest{}
	if err := client.Get(ctx, crclient.ObjectKey{Namespace: rar.Namespace, Name: owner.Name}, parent); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, &ParentEvidence{State: "missing", Complete: false}
		}
		return nil, &ParentEvidence{State: "unavailable", Complete: false}
	}
	return parent, nil
}

func parentEvidenceHasRequiredFields(parent *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest) bool {
	return parent.Name != "" && parent.Namespace != "" && parent.UID != "" &&
		string(parent.Status.OverallPhase) != "" &&
		parent.Spec.TargetResource.Kind != "" && parent.Spec.TargetResource.Name != "" &&
		!rar.Spec.RequiredBy.IsZero()
}

func parentEvidenceRequiredValues(parent *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest) []string {
	return []string{
		parent.Name,
		parent.Namespace,
		string(parent.UID),
		string(parent.Status.OverallPhase),
		parent.Spec.TargetResource.Kind,
		parent.Spec.TargetResource.Name,
		rar.Spec.RequiredBy.Format(time.RFC3339),
	}
}

func parentEvidenceStringsWithinBounds(values []string) bool {
	for _, value := range values {
		if len([]byte(value)) > maxParentEvidenceStringBytes {
			return false
		}
	}
	return true
}

func parentEvidenceProjection(parent *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest, truncated *bool) *ParentProjection {
	return &ParentProjection{
		Name:         parent.Name,
		Namespace:    parent.Namespace,
		UID:          string(parent.UID),
		OverallPhase: string(parent.Status.OverallPhase),
		SignalSource: boundedOptionalParentString(parent.Spec.SignalSource, truncated),
		ClusterID:    boundedOptionalParentString(parent.Spec.ClusterID, truncated),
		TargetResource: ParentTargetResource{
			Kind:       parent.Spec.TargetResource.Kind,
			Name:       parent.Spec.TargetResource.Name,
			Namespace:  boundedOptionalParentString(parent.Spec.TargetResource.Namespace, truncated),
			APIVersion: boundedOptionalParentString(parent.Spec.TargetResource.APIVersion, truncated),
		},
		Approval: ParentApprovalProjection{
			RequiredBy: rar.Spec.RequiredBy.Format(time.RFC3339),
			Decision:   optionalApprovalString(string(rar.Status.Decision)),
			DecidedBy:  optionalApprovalString(rar.Status.DecidedBy),
			DecidedVia: optionalApprovalString(rar.Status.DecidedVia),
			DecidedAt:  optionalApprovalTime(rar.Status.DecidedAt),
			Expired:    rar.Status.Expired,
		},
	}
}

func boundParentEvidence(evidence *ParentEvidence) *ParentEvidence {
	encoded, err := json.Marshal(evidence)
	if err != nil || len(encoded) > maxParentEvidenceBytes {
		return &ParentEvidence{State: "unavailable", Complete: false, Truncated: true}
	}
	return evidence
}

func approvalRequestOwner(rar *remediationv1.RemediationApprovalRequest) (metav1.OwnerReference, bool) {
	for _, owner := range rar.OwnerReferences {
		if owner.APIVersion == remediationv1.GroupVersion.String() &&
			owner.Kind == "RemediationRequest" &&
			owner.Name != "" &&
			owner.UID != "" &&
			owner.Controller != nil &&
			*owner.Controller {
			return owner, true
		}
	}
	return metav1.OwnerReference{}, false
}

func boundedOptionalParentString(value string, truncated *bool) *string {
	if value == "" {
		return nil
	}
	if len([]byte(value)) > maxParentEvidenceStringBytes {
		*truncated = true
		return nil
	}
	copyValue := value
	return &copyValue
}

func optionalApprovalString(value string) *string {
	if value == "" {
		return nil
	}
	copyValue := value
	return &copyValue
}

func optionalApprovalTime(value *metav1.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format(time.RFC3339)
	return &formatted
}

// NewGetApprovalRequestTool creates the kubernaut_get_approval_request tool.
func NewGetApprovalRequestTool(client crclient.Client, controllerNS string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_get_approval_request",
		Description: "Get full details of a specific remediation approval request for review before deciding",
	}, func(ctx agent.Context, args GetApprovalRequestArgs) (GetApprovalRequestResult, error) {
		args.Namespace = controllerNS
		return HandleGetApprovalRequest(ctx, client, args)
	})
}

// ApproveArgs defines the input for kubernaut_approve.
type ApproveArgs struct {
	Namespace        string                          `json:"-"`
	RARName          string                          `json:"rar_name"`
	Decision         string                          `json:"decision"`
	Reason           string                          `json:"reason,omitempty"`
	WorkflowOverride *remediationv1.WorkflowOverride `json:"workflow_override,omitempty"`
}

// ApproveResult is the output of kubernaut_approve.
type ApproveResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	// Audit carries the frozen decision context to the tool bridge, not to the MCP caller.
	Audit *ApprovalAuditContext `json:"-"`
}

// ApprovalAuditContext contains the values needed to correlate approval intent.
type ApprovalAuditContext struct {
	CorrelationID       string
	WorkflowID          string
	OriginalParameters  map[string]string
	OverrideParameters  map[string]string
	EffectiveParameters map[string]string
	OverrideRationale   *string
}

// ApprovalDecisionDeniedError marks an approval request rejected locally
// before the status write. The MCP bridge uses the attached reviewed context
// to emit the same structured denial audit payload as an admission denial.
type ApprovalDecisionDeniedError struct {
	Err   error
	Audit *ApprovalAuditContext
}

func (e *ApprovalDecisionDeniedError) Error() string {
	if e == nil || e.Err == nil {
		return "approval decision denied"
	}
	return fmt.Sprintf("approval decision denied: %v", e.Err)
}

func (e *ApprovalDecisionDeniedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ApprovalDecisionWriteError reports a conditional decision write that could
// not be proven from the canonical object after the API response.
type ApprovalDecisionWriteError struct {
	Outcome string
	Err     error
}

func (e *ApprovalDecisionWriteError) Error() string {
	if e == nil || e.Err == nil {
		return "approval decision write outcome is unknown"
	}
	return fmt.Sprintf("approval decision write %s: %v", e.Outcome, e.Err)
}

func (e *ApprovalDecisionWriteError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// HandleApprove implements the kubernaut_approve logic.
//
//nolint:gocritic // hugeParam: args passed by value for simplicity; not performance-critical
func HandleApprove(ctx context.Context, client crclient.Client, args ApproveArgs, username string) (result ApproveResult, resultErr error) {
	if client == nil {
		return ApproveResult{}, ErrK8sUnavailable
	}
	if err := validateApproveArgs(args); err != nil {
		return ApproveResult{}, err
	}

	var rar remediationv1.RemediationApprovalRequest
	if err := client.Get(ctx, crclient.ObjectKey{Namespace: args.Namespace, Name: args.RARName}, &rar); err != nil {
		return ApproveResult{}, ToUserFriendlyError(err)
	}
	defer setApprovalAuditOnSuccess(&result, &resultErr, &rar, args.WorkflowOverride)
	if err := validateApprovalOverrideArgs(args); err != nil {
		return ApproveResult{}, &ApprovalDecisionDeniedError{
			Err:   err,
			Audit: approvalAuditContext(&rar, args.WorkflowOverride),
		}
	}

	if rar.DeletionTimestamp != nil {
		return ApproveResult{}, fmt.Errorf("%w: RemediationApprovalRequest is terminating", ErrAlreadyTerminal)
	}

	if rar.Status.Decision != remediationv1.ApprovalDecisionPending {
		if approvalRequestMatches(args, username, &rar) {
			return approvalResult(args, username), nil
		}
		return ApproveResult{}, fmt.Errorf("%w: RemediationApprovalRequest already has terminal decision %q", ErrAlreadyTerminal, rar.Status.Decision)
	}
	if args.WorkflowOverride != nil {
		if err := remediationapprovalrequest.ValidateParameterOverride(&rar.Spec, args.WorkflowOverride.Parameters); err != nil {
			return ApproveResult{}, &ApprovalDecisionDeniedError{
				Err:   fmt.Errorf("%w: parameter override: %w", ErrInvalidInput, err),
				Audit: approvalAuditContext(&rar, args.WorkflowOverride),
			}
		}
	}

	// Status().Update carries the fetched metadata.resourceVersion. The API
	// server therefore makes the first concurrent business decision win.
	rar.Status.Decision = remediationv1.ApprovalDecision(args.Decision)
	rar.Status.DecidedBy = username
	if args.Reason != "" {
		rar.Status.DecisionMessage = args.Reason
	}
	if args.WorkflowOverride != nil {
		rar.Status.WorkflowOverride = args.WorkflowOverride.DeepCopy()
	}

	if err := client.Status().Update(ctx, &rar); err != nil {
		// The apiserver may retry admission after another decision commits. In
		// that case the webhook denies the stale decision as terminal, rather
		// than returning a resourceVersion conflict. Prove the winner by readback.
		if isTerminalAdmissionDenial(err) {
			return proveConditionalDecision(ctx, client, args, username, rar.UID, err)
		}
		if isDefinitiveApprovalDecisionRejection(err) {
			return ApproveResult{}, toUserFriendlyApprovalDecisionError(err)
		}
		return proveConditionalDecision(ctx, client, args, username, rar.UID, err)
	}

	return ApproveResult{
		Status:  args.Decision,
		Message: fmt.Sprintf("Remediation approval %s by %s", args.Decision, username),
	}, nil
}

func setApprovalAuditOnSuccess(result *ApproveResult, resultErr *error, rar *remediationv1.RemediationApprovalRequest, override *remediationv1.WorkflowOverride) {
	if *resultErr == nil {
		result.Audit = approvalAuditContext(rar, override)
	}
}

func approvalAuditContext(rar *remediationv1.RemediationApprovalRequest, override *remediationv1.WorkflowOverride) *ApprovalAuditContext {
	info := &ApprovalAuditContext{CorrelationID: rar.Spec.RemediationRequestRef.Name}
	if rar.Spec.Workflow != nil {
		info.WorkflowID = rar.Spec.Workflow.WorkflowID
	}
	if rar.Spec.Parameters != nil {
		info.OriginalParameters = make(map[string]string, len(rar.Spec.Parameters))
		info.EffectiveParameters = make(map[string]string, len(rar.Spec.Parameters))
		for name, value := range rar.Spec.Parameters {
			info.OriginalParameters[name] = value
			info.EffectiveParameters[name] = value
		}
	}
	if override != nil {
		rationale := override.Rationale
		info.OverrideRationale = &rationale
		if override.Parameters != nil {
			info.OverrideParameters = make(map[string]string, len(override.Parameters))
			for name, value := range override.Parameters {
				info.OverrideParameters[name] = value
				if info.EffectiveParameters != nil {
					info.EffectiveParameters[name] = value
				}
			}
		}
	}
	return info
}

func validateApproveArgs(args ApproveArgs) error {
	if err := validate.Namespace(args.Namespace); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if err := validate.ResourceName(args.RARName); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if args.Decision == "" {
		return fmt.Errorf("%w: decision must not be empty", ErrInvalidInput)
	}
	if args.Decision != string(remediationv1.ApprovalDecisionApproved) && args.Decision != string(remediationv1.ApprovalDecisionRejected) {
		return fmt.Errorf("%w: decision %q is not valid; must be exactly one of: Approved, Rejected", ErrInvalidInput, args.Decision)
	}
	return nil
}

func validateApprovalOverrideArgs(args ApproveArgs) error {
	if args.WorkflowOverride != nil && (args.Decision != string(remediationv1.ApprovalDecisionApproved) || args.WorkflowOverride.WorkflowName != "") {
		return fmt.Errorf("%w: overrides are parameter-only and require approval", ErrInvalidInput)
	}
	return nil
}

func isDefinitiveApprovalDecisionRejection(err error) bool {
	return apierrors.IsForbidden(err) ||
		apierrors.IsUnauthorized(err) ||
		apierrors.IsInvalid(err) ||
		apierrors.IsBadRequest(err) ||
		apierrors.IsNotFound(err)
}

func isTerminalAdmissionDenial(err error) bool {
	return apierrors.IsForbidden(err) && strings.Contains(err.Error(), "admission webhook") && strings.Contains(err.Error(), "terminal decision is immutable")
}

func toUserFriendlyApprovalDecisionError(err error) error {
	if reason := knownApprovalAdmissionDenial(err); reason != "" {
		return fmt.Errorf("approval decision rejected: %s", reason)
	}
	return ToUserFriendlyError(err)
}

func knownApprovalAdmissionDenial(err error) string {
	var statusErr *apierrors.StatusError
	if !errors.As(err, &statusErr) || !apierrors.IsForbidden(err) {
		return ""
	}

	const deniedRequestMarker = "denied the request:"
	message := statusErr.ErrStatus.Message
	markerIndex := strings.Index(strings.ToLower(message), deniedRequestMarker)
	if markerIndex < 0 {
		return ""
	}

	reason := strings.ToLower(strings.TrimSpace(message[markerIndex+len(deniedRequestMarker):]))
	switch {
	case strings.Contains(reason, "approval decision deadline has passed"):
		return "approval decision deadline has passed"
	case strings.Contains(reason, "override rejected:"):
		return "workflow override was rejected"
	default:
		return ""
	}
}

func approvalRequestMatches(args ApproveArgs, username string, rar *remediationv1.RemediationApprovalRequest) bool {
	if rar.Status.Decision != remediationv1.ApprovalDecision(args.Decision) || rar.Status.DecidedBy != username {
		return false
	}
	if rar.Status.DecisionMessage != args.Reason {
		return false
	}
	return reflect.DeepEqual(rar.Status.WorkflowOverride, args.WorkflowOverride)
}

func approvalResult(args ApproveArgs, username string) ApproveResult {
	return ApproveResult{
		Status:  args.Decision,
		Message: fmt.Sprintf("Remediation approval %s by %s", args.Decision, username),
	}
}

func proveConditionalDecision(ctx context.Context, client crclient.Client, args ApproveArgs, username string, expectedUID types.UID, writeErr error) (ApproveResult, error) {
	canonical := &remediationv1.RemediationApprovalRequest{}
	if err := client.Get(ctx, crclient.ObjectKey{Namespace: args.Namespace, Name: args.RARName}, canonical); err == nil {
		if expectedUID == "" || canonical.UID != expectedUID {
			return ApproveResult{}, &ApprovalDecisionWriteError{Outcome: "unknown", Err: fmt.Errorf("canonical approval request UID does not match the fetched object")}
		}
		if approvalRequestMatches(args, username, canonical) {
			return approvalResult(args, username), nil
		}
		outcome := "unknown"
		if apierrors.IsConflict(writeErr) || (apierrors.IsForbidden(writeErr) && canonical.Status.Decision != remediationv1.ApprovalDecisionPending) {
			outcome = "conflict"
		}
		return ApproveResult{}, &ApprovalDecisionWriteError{Outcome: outcome, Err: writeErr}
	} else {
		return ApproveResult{}, &ApprovalDecisionWriteError{Outcome: "unknown", Err: errors.Join(writeErr, err)}
	}
}

// NewApproveTool creates the kubernaut_approve tool.
func NewApproveTool(client crclient.Client, controllerNS string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_approve",
		Description: "Approve or reject a pending remediation approval request. The decision field accepts exactly 'Approved' or 'Rejected'.",
		InputSchema: approveInputSchema(),
	}, func(ctx agent.Context, args ApproveArgs) (ApproveResult, error) {
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
				Type:        "object",
				Description: "Optional changes to existing workflow parameter values; workflow selection cannot be changed",
				Properties: map[string]*jsonschema.Schema{
					"parameters": {Type: "object", AdditionalProperties: &jsonschema.Schema{Type: "string"}},
					"rationale":  {Type: "string"},
				},
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
	// ClusterID and SessionID are ambient hints the LLM propagates from fleet
	// context and cross-phase preservation (#2364 Tier 2). Tolerated here so
	// strict ADK schema validation does not kill the turn; ignored by the
	// handler.
	ClusterID string `json:"cluster_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
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
	}, func(ctx agent.Context, args CancelRemediationArgs) (CancelRemediationResult, error) {
		args.Namespace = controllerNS
		return HandleCancelRemediation(ctx, client, args)
	})
}
