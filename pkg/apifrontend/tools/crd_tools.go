package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

var rrGVR = schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}
var rarGVR = schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationapprovalrequests"}

// ListRemediationsArgs defines the input for kubernaut_list_remediations.
type ListRemediationsArgs struct {
	Namespace string `json:"namespace"`
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
func HandleListRemediations(ctx context.Context, client dynamic.Interface, args ListRemediationsArgs) (ListRemediationsResult, error) {
	if client == nil {
		return ListRemediationsResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return ListRemediationsResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	list, err := client.Resource(rrGVR).Namespace(args.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return ListRemediationsResult{}, ToUserFriendlyError(err)
	}

	var result []RemediationSummary
	for _, item := range list.Items {
		phase, _, _ := unstructured.NestedString(item.Object, "status", "overallPhase")
		if args.Phase != "" && phase != args.Phase {
			continue
		}
		kind, _, _ := unstructured.NestedString(item.Object, "spec", "targetResource", "kind")
		target, _, _ := unstructured.NestedString(item.Object, "spec", "targetResource", "name")
		if args.Kind != "" && kind != args.Kind {
			continue
		}
		if args.Name != "" && target != args.Name {
			continue
		}
		result = append(result, RemediationSummary{
			ID:        item.GetNamespace() + "/" + item.GetName(),
			Namespace: item.GetNamespace(),
			Name:      item.GetName(),
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
func NewListRemediationsTool(client dynamic.Interface) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_list_remediations",
		Description: "List active remediations with optional filtering by namespace, phase, kind, or name",
	}, func(ctx tool.Context, args ListRemediationsArgs) (ListRemediationsResult, error) {
		return HandleListRemediations(ctx, client, args)
	})
}

// GetRemediationArgs defines the input for kubernaut_get_remediation.
type GetRemediationArgs struct {
	Namespace string `json:"namespace,omitempty"`
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
func HandleGetRemediation(ctx context.Context, client dynamic.Interface, args GetRemediationArgs) (GetRemediationResult, error) {
	if client == nil {
		return GetRemediationResult{}, ErrK8sUnavailable
	}
	ns, name, err := ParseRRID(args.RRID, args.Namespace, args.Name)
	if err != nil {
		return GetRemediationResult{}, err
	}

	obj, err := client.Resource(rrGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return GetRemediationResult{}, ToUserFriendlyError(err)
	}

	phase, _, _ := unstructured.NestedString(obj.Object, "status", "overallPhase")
	kind, _, _ := unstructured.NestedString(obj.Object, "spec", "targetResource", "kind")
	target, _, _ := unstructured.NestedString(obj.Object, "spec", "targetResource", "name")

	return GetRemediationResult{
		ID:        ns + "/" + name,
		Namespace: ns,
		Name:      name,
		Phase:     phase,
		Kind:      kind,
		Target:    target,
	}, nil
}

// NewGetRemediationTool creates the kubernaut_get_remediation tool.
func NewGetRemediationTool(client dynamic.Interface) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_get_remediation",
		Description: "Get detailed information about a specific remediation by namespace/name or rr_id",
	}, func(ctx tool.Context, args GetRemediationArgs) (GetRemediationResult, error) {
		return HandleGetRemediation(ctx, client, args)
	})
}

// ────────────────────────────────────────────────────────────────────────────
// kubernaut_list_approval_requests
// ────────────────────────────────────────────────────────────────────────────

// ListApprovalRequestsArgs defines the input for kubernaut_list_approval_requests.
type ListApprovalRequestsArgs struct {
	Namespace string `json:"namespace"`
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
	Name                string  `json:"name"`
	Namespace           string  `json:"namespace"`
	Decision            string  `json:"decision"`
	RemediationRequest  string  `json:"remediation_request"`
	Confidence          float64 `json:"confidence"`
	ConfidenceLevel     string  `json:"confidence_level"`
	TimeRemaining       string  `json:"time_remaining,omitempty"`
	RequiredBy          string  `json:"required_by,omitempty"`
}

// HandleListApprovalRequests implements the kubernaut_list_approval_requests logic.
func HandleListApprovalRequests(ctx context.Context, client dynamic.Interface, args ListApprovalRequestsArgs) (ListApprovalRequestsResult, error) {
	if client == nil {
		return ListApprovalRequestsResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return ListApprovalRequestsResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	list, err := client.Resource(rarGVR).Namespace(args.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return ListApprovalRequestsResult{}, ToUserFriendlyError(err)
	}

	result := make([]ApprovalRequestSummary, 0)
	for _, item := range list.Items {
		decision, _, _ := unstructured.NestedString(item.Object, "status", "decision")
		if !matchesDecisionFilter(decision, args.Decision) {
			continue
		}

		rrName, _, _ := unstructured.NestedString(item.Object, "spec", "remediationRequestRef", "name")
		confidence, _, _ := unstructured.NestedFloat64(item.Object, "spec", "confidence")
		confidenceLevel, _, _ := unstructured.NestedString(item.Object, "spec", "confidenceLevel")
		timeRemaining, _, _ := unstructured.NestedString(item.Object, "status", "timeRemaining")
		requiredBy, _, _ := unstructured.NestedString(item.Object, "spec", "requiredBy")

		displayDecision := decision
		if displayDecision == "" {
			displayDecision = "Pending"
		}

		result = append(result, ApprovalRequestSummary{
			Name:               item.GetName(),
			Namespace:          item.GetNamespace(),
			Decision:           displayDecision,
			RemediationRequest: rrName,
			Confidence:         confidence,
			ConfidenceLevel:    confidenceLevel,
			TimeRemaining:      timeRemaining,
			RequiredBy:         requiredBy,
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
func NewListApprovalRequestsTool(client dynamic.Interface) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_list_approval_requests",
		Description: "List remediation approval requests with optional filtering by decision status (pending, approved, rejected, expired)",
	}, func(ctx tool.Context, args ListApprovalRequestsArgs) (ListApprovalRequestsResult, error) {
		return HandleListApprovalRequests(ctx, client, args)
	})
}

// ────────────────────────────────────────────────────────────────────────────
// kubernaut_get_approval_request
// ────────────────────────────────────────────────────────────────────────────

// GetApprovalRequestArgs defines the input for kubernaut_get_approval_request.
type GetApprovalRequestArgs struct {
	Namespace string `json:"namespace,omitempty"`
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
	Name                   string                      `json:"name"`
	Namespace              string                      `json:"namespace"`
	RemediationRequest     string                      `json:"remediation_request"`
	AIAnalysis             string                      `json:"ai_analysis"`
	Confidence             float64                     `json:"confidence"`
	ConfidenceLevel        string                      `json:"confidence_level"`
	Reason                 string                      `json:"reason"`
	InvestigationSummary   string                      `json:"investigation_summary"`
	WhyApprovalRequired    string                      `json:"why_approval_required"`
	RecommendedWorkflow    RecommendedWorkflowInfo     `json:"recommended_workflow"`
	RecommendedActions     []RecommendedActionSummary  `json:"recommended_actions"`
	EvidenceCollected      []string                    `json:"evidence_collected"`
	AlternativesConsidered []AlternativeSummary        `json:"alternatives_considered"`
	RequiredBy             string                      `json:"required_by,omitempty"`
	Decision               string                      `json:"decision"`
	DecidedBy              string                      `json:"decided_by,omitempty"`
	DecidedAt              string                      `json:"decided_at,omitempty"`
	TimeRemaining          string                      `json:"time_remaining,omitempty"`
	Expired                bool                        `json:"expired"`
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
func HandleGetApprovalRequest(ctx context.Context, client dynamic.Interface, args GetApprovalRequestArgs) (GetApprovalRequestResult, error) {
	if client == nil {
		return GetApprovalRequestResult{}, ErrK8sUnavailable
	}

	ns, name, err := ParseResourceID(args.RARID, args.Namespace, args.Name)
	if err != nil {
		return GetApprovalRequestResult{}, err
	}
	if errV := validate.Namespace(ns); errV != nil {
		return GetApprovalRequestResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, errV)
	}
	if errV := validate.ResourceName(name); errV != nil {
		return GetApprovalRequestResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, errV)
	}

	obj, err := client.Resource(rarGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return GetApprovalRequestResult{}, ToUserFriendlyError(err)
	}

	// Spec fields
	rrName, _, _ := unstructured.NestedString(obj.Object, "spec", "remediationRequestRef", "name")
	aiaName, _, _ := unstructured.NestedString(obj.Object, "spec", "aiAnalysisRef", "name")
	confidence, _, _ := unstructured.NestedFloat64(obj.Object, "spec", "confidence")
	confidenceLevel, _, _ := unstructured.NestedString(obj.Object, "spec", "confidenceLevel")
	reason, _, _ := unstructured.NestedString(obj.Object, "spec", "reason")
	investigationSummary, _, _ := unstructured.NestedString(obj.Object, "spec", "investigationSummary")
	whyApprovalRequired, _, _ := unstructured.NestedString(obj.Object, "spec", "whyApprovalRequired")
	requiredBy, _, _ := unstructured.NestedString(obj.Object, "spec", "requiredBy")

	// Recommended workflow
	wfName, _, _ := unstructured.NestedString(obj.Object, "spec", "recommendedWorkflow", "workflowId")
	wfVersion, _, _ := unstructured.NestedString(obj.Object, "spec", "recommendedWorkflow", "version")

	// Evidence
	evidenceRaw, _, _ := unstructured.NestedStringSlice(obj.Object, "spec", "evidenceCollected")
	if evidenceRaw == nil {
		evidenceRaw = []string{}
	}

	// Recommended actions
	actionsRaw, _, _ := unstructured.NestedSlice(obj.Object, "spec", "recommendedActions")
	actions := make([]RecommendedActionSummary, 0, len(actionsRaw))
	for _, a := range actionsRaw {
		if m, ok := a.(map[string]interface{}); ok {
			action, _ := m["action"].(string)
			rationale, _ := m["rationale"].(string)
			actions = append(actions, RecommendedActionSummary{Action: action, Rationale: rationale})
		}
	}

	// Alternatives
	altsRaw, _, _ := unstructured.NestedSlice(obj.Object, "spec", "alternativesConsidered")
	alternatives := make([]AlternativeSummary, 0, len(altsRaw))
	for _, a := range altsRaw {
		if m, ok := a.(map[string]interface{}); ok {
			approach, _ := m["approach"].(string)
			prosCons, _ := m["prosCons"].(string)
			alternatives = append(alternatives, AlternativeSummary{
				Approach: approach,
				ProsCons: prosCons,
			})
		}
	}

	// Status fields
	decision, _, _ := unstructured.NestedString(obj.Object, "status", "decision")
	decidedBy, _, _ := unstructured.NestedString(obj.Object, "status", "decidedBy")
	decidedAt, _, _ := unstructured.NestedString(obj.Object, "status", "decidedAt")
	timeRemaining, _, _ := unstructured.NestedString(obj.Object, "status", "timeRemaining")
	expired, _, _ := unstructured.NestedBool(obj.Object, "status", "expired")

	displayDecision := decision
	if displayDecision == "" {
		displayDecision = "Pending"
	}

	return GetApprovalRequestResult{
		Name:                   obj.GetName(),
		Namespace:              obj.GetNamespace(),
		RemediationRequest:     rrName,
		AIAnalysis:             aiaName,
		Confidence:             confidence,
		ConfidenceLevel:        confidenceLevel,
		Reason:                 reason,
		InvestigationSummary:   investigationSummary,
		WhyApprovalRequired:    whyApprovalRequired,
		RecommendedWorkflow:    RecommendedWorkflowInfo{Name: wfName, Version: wfVersion},
		RecommendedActions:     actions,
		EvidenceCollected:      evidenceRaw,
		AlternativesConsidered: alternatives,
		RequiredBy:             requiredBy,
		Decision:               displayDecision,
		DecidedBy:              decidedBy,
		DecidedAt:              decidedAt,
		TimeRemaining:          timeRemaining,
		Expired:                expired,
	}, nil
}

// NewGetApprovalRequestTool creates the kubernaut_get_approval_request tool.
func NewGetApprovalRequestTool(client dynamic.Interface) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_get_approval_request",
		Description: "Get full details of a specific remediation approval request for review before deciding",
	}, func(ctx tool.Context, args GetApprovalRequestArgs) (GetApprovalRequestResult, error) {
		return HandleGetApprovalRequest(ctx, client, args)
	})
}

// ApproveArgs defines the input for kubernaut_approve.
type ApproveArgs struct {
	Namespace        string `json:"namespace"`
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
func HandleApprove(ctx context.Context, client dynamic.Interface, args ApproveArgs, username string) (ApproveResult, error) {
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
	// CRD enum requires capitalized values: Approved, Rejected, Expired
	normalizedDecision := strings.ToUpper(args.Decision[:1]) + strings.ToLower(args.Decision[1:])
	_, err := client.Resource(rarGVR).Namespace(args.Namespace).Get(ctx, args.RARName, metav1.GetOptions{})
	if err != nil {
		return ApproveResult{}, ToUserFriendlyError(err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	statusPatch := map[string]interface{}{
		"decision":  normalizedDecision,
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

	_, err = client.Resource(rarGVR).Namespace(args.Namespace).Patch(
		ctx, args.RARName, types.MergePatchType, patchBytes, metav1.PatchOptions{}, "status",
	)
	if err != nil {
		return ApproveResult{}, ToUserFriendlyError(err)
	}

	return ApproveResult{
		Status:  args.Decision,
		Message: fmt.Sprintf("Remediation approval %s by %s", args.Decision, username),
	}, nil
}

// NewApproveTool creates the kubernaut_approve tool.
func NewApproveTool(client dynamic.Interface) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_approve",
		Description: "Approve or reject a pending remediation approval request",
	}, func(ctx tool.Context, args ApproveArgs) (ApproveResult, error) {
		return HandleApprove(ctx, client, args, usernameFromContext(ctx))
	})
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
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	RRID      string `json:"rr_id,omitempty"`
}

// CancelRemediationResult is the output of kubernaut_cancel_remediation.
type CancelRemediationResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// HandleCancelRemediation implements the kubernaut_cancel_remediation logic.
func HandleCancelRemediation(ctx context.Context, client dynamic.Interface, args CancelRemediationArgs) (CancelRemediationResult, error) {
	if client == nil {
		return CancelRemediationResult{}, ErrK8sUnavailable
	}
	ns, name, err := ParseRRID(args.RRID, args.Namespace, args.Name)
	if err != nil {
		return CancelRemediationResult{}, err
	}

	obj, err := client.Resource(rrGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return CancelRemediationResult{}, ToUserFriendlyError(err)
	}

	overallPhase, _, _ := unstructured.NestedString(obj.Object, "status", "overallPhase")
	if IsTerminalPhase(overallPhase) {
		return CancelRemediationResult{}, fmt.Errorf("%w: remediation %s/%s is in terminal state %q", ErrAlreadyTerminal, ns, name, overallPhase)
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

	_, err = client.Resource(rrGVR).Namespace(ns).Patch(
		ctx, name, types.MergePatchType, patchBytes, metav1.PatchOptions{}, "status",
	)
	if err != nil {
		return CancelRemediationResult{}, ToUserFriendlyError(err)
	}

	return CancelRemediationResult{
		Status:  "Cancelled",
		Message: fmt.Sprintf("Remediation %s/%s cancelled", ns, name),
	}, nil
}

// NewCancelRemediationTool creates the kubernaut_cancel_remediation tool.
func NewCancelRemediationTool(client dynamic.Interface) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_cancel_remediation",
		Description: "Cancel an active remediation that has not yet reached a terminal state",
	}, func(ctx tool.Context, args CancelRemediationArgs) (CancelRemediationResult, error) {
		return HandleCancelRemediation(ctx, client, args)
	})
}

// WatchArgs defines the input for kubernaut_watch.
type WatchArgs struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// WatchEvent represents a single status change event.
type WatchEvent struct {
	Timestamp string `json:"timestamp"`
	Resource  string `json:"resource"`
	Phase     string `json:"phase"`
	Message   string `json:"message,omitempty"`
}

// WatchResult is the output of kubernaut_watch.
type WatchResult struct {
	Events []WatchEvent `json:"events"`
	Status string       `json:"status"`
}

// maxWatchDuration is the maximum time HandleWatch will block before returning.
const maxWatchDuration = 10 * time.Minute

// HandleWatch implements the kubernaut_watch logic.
func HandleWatch(ctx context.Context, client dynamic.Interface, args WatchArgs) (WatchResult, error) {
	if client == nil {
		return WatchResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return WatchResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := validate.ResourceName(args.Name); err != nil {
		return WatchResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Verify the RR exists before blocking on a watch — a watch on a
	// non-existent resource would silently block until the 10-minute timeout.
	if _, err := client.Resource(rrGVR).Namespace(args.Namespace).Get(ctx, args.Name, metav1.GetOptions{}); err != nil {
		return WatchResult{}, ToUserFriendlyError(err)
	}

	watchCtx, cancel := context.WithTimeout(ctx, maxWatchDuration)
	defer cancel()

	watcher, err := client.Resource(rrGVR).Namespace(args.Namespace).Watch(watchCtx, metav1.ListOptions{
		FieldSelector: "metadata.name=" + args.Name,
	})
	if err != nil {
		return WatchResult{}, ToUserFriendlyError(err)
	}
	defer watcher.Stop()

	var events []WatchEvent

	for {
		select {
		case <-ctx.Done():
			return WatchResult{Events: events, Status: "cancelled"}, nil
		case evt, ok := <-watcher.ResultChan():
			if !ok {
				return WatchResult{Events: events, Status: "completed"}, nil
			}
			if evt.Type == watch.Modified || evt.Type == watch.Added {
				obj, ok := evt.Object.(*unstructured.Unstructured)
				if !ok {
					continue
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "overallPhase")
				events = append(events, WatchEvent{
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Resource:  "RemediationRequest",
					Phase:     phase,
					Message:   fmt.Sprintf("Phase changed to %s", phase),
				})
				if IsTerminalPhase(phase) {
					return WatchResult{Events: events, Status: "completed"}, nil
				}
			}
		}
	}
}

// NewWatchTool creates the kubernaut_watch tool.
func NewWatchTool(client dynamic.Interface) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_watch",
		Description: "Stream live status updates for a remediation and its related resources",
	}, func(ctx tool.Context, args WatchArgs) (WatchResult, error) {
		return HandleWatch(ctx, client, args)
	})
}

// ========================================
// kubernaut_await_session: Wait for KA investigation session readiness
// BR-INTERACTIVE-010: AF waits for AA to submit to KA before connecting
// ========================================

var aianalysisGVR = schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "aianalyses"}

// AwaitSessionArgs defines the input for kubernaut_await_session.
type AwaitSessionArgs struct {
	Namespace string `json:"namespace"`
	RRName    string `json:"rr_name"`
}

// AwaitSessionResult is the output of kubernaut_await_session.
type AwaitSessionResult struct {
	SessionID string `json:"session_id,omitempty"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

const awaitSessionTimeout = 2 * time.Minute
const awaitSessionPollInterval = 3 * time.Second

// HandleAwaitSession waits for an AIAnalysis resource (matching the given RR) to have
// a non-empty status.investigationSession.id. Returns the session ID when ready, or
// times out after 2 minutes.
func HandleAwaitSession(ctx context.Context, client dynamic.Interface, args AwaitSessionArgs) (AwaitSessionResult, error) {
	if client == nil {
		return AwaitSessionResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return AwaitSessionResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if args.RRName == "" {
		return AwaitSessionResult{}, fmt.Errorf("%w: rr_name is required", ErrInvalidInput)
	}

	// Check existing AIAnalysis first (may already have session).
	if sessionID := findSessionIDByList(ctx, client, args); sessionID != "" {
		return AwaitSessionResult{SessionID: sessionID, Status: "ready"}, nil
	}

	watchCtx, cancel := context.WithTimeout(ctx, awaitSessionTimeout)
	defer cancel()

	watcher, err := client.Resource(aianalysisGVR).Namespace(args.Namespace).Watch(watchCtx, metav1.ListOptions{
		FieldSelector: "spec.remediationRequestRef.name=" + args.RRName,
	})
	if err != nil {
		// Field selectors on custom fields may not be supported via dynamic client.
		// Fall back to polling.
		return pollForSessionID(watchCtx, client, args)
	}
	defer watcher.Stop()

	for {
		select {
		case <-watchCtx.Done():
			return AwaitSessionResult{Status: "timeout", Message: "KA session not ready within timeout"}, nil
		case evt, ok := <-watcher.ResultChan():
			if !ok {
				return AwaitSessionResult{Status: "timeout", Message: "watch closed unexpectedly"}, nil
			}
			if evt.Type == watch.Modified || evt.Type == watch.Added {
				obj, ok := evt.Object.(*unstructured.Unstructured)
				if !ok {
					continue
				}
				sessionID, _, _ := unstructured.NestedString(obj.Object, "status", "investigationSession", "id")
				if sessionID != "" {
					return AwaitSessionResult{SessionID: sessionID, Status: "ready"}, nil
				}
			}
		}
	}
}

// pollForSessionID is a fallback that polls AIAnalysis resources until session ID appears.
func pollForSessionID(ctx context.Context, client dynamic.Interface, args AwaitSessionArgs) (AwaitSessionResult, error) {
	ticker := time.NewTicker(awaitSessionPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return AwaitSessionResult{Status: "timeout", Message: "KA session not ready within timeout"}, nil
		case <-ticker.C:
			if sessionID := findSessionIDByList(ctx, client, args); sessionID != "" {
				return AwaitSessionResult{SessionID: sessionID, Status: "ready"}, nil
			}
		}
	}
}

// findSessionIDByList lists AIAnalysis for the given RR and returns the first non-empty session ID.
func findSessionIDByList(ctx context.Context, client dynamic.Interface, args AwaitSessionArgs) string {
	list, err := client.Resource(aianalysisGVR).Namespace(args.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return ""
	}
	for _, item := range list.Items {
		rrName, _, _ := unstructured.NestedString(item.Object, "spec", "remediationRequestRef", "name")
		if rrName != args.RRName {
			continue
		}
		sessionID, _, _ := unstructured.NestedString(item.Object, "status", "investigationSession", "id")
		if sessionID != "" {
			return sessionID
		}
	}
	return ""
}

// NewAwaitSessionTool creates the kubernaut_await_session tool.
func NewAwaitSessionTool(client dynamic.Interface) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_await_session",
		Description: "Wait for the AI investigation session to become ready for a given remediation request. Returns the KA session ID when available.",
	}, func(ctx tool.Context, args AwaitSessionArgs) (AwaitSessionResult, error) {
		return HandleAwaitSession(ctx, client, args)
	})
}
