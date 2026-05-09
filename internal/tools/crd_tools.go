package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/validate"
)

var rrGVR = schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}
var rarGVR = schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationapprovalrequests"}
var spGVR = schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "signalprocessings"}

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
	if args.Kind != "" {
		if err := validate.LabelValue(args.Kind); err != nil {
			return ListRemediationsResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
	}
	if args.Name != "" {
		if err := validate.LabelValue(args.Name); err != nil {
			return ListRemediationsResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
	}

	opts := metav1.ListOptions{}

	var labelSelectors []string
	if args.Kind != "" {
		labelSelectors = append(labelSelectors, "kubernaut.ai/target-kind="+args.Kind)
	}
	if args.Name != "" {
		labelSelectors = append(labelSelectors, "kubernaut.ai/target-name="+args.Name)
	}
	if len(labelSelectors) > 0 {
		sel := ""
		for i, s := range labelSelectors {
			if i > 0 {
				sel += ","
			}
			sel += s
		}
		opts.LabelSelector = sel
	}

	list, err := client.Resource(rrGVR).Namespace(args.Namespace).List(ctx, opts)
	if err != nil {
		return ListRemediationsResult{}, ToUserFriendlyError(err)
	}

	var result []RemediationSummary
	for _, item := range list.Items {
		phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
		if args.Phase != "" && phase != args.Phase {
			continue
		}
		kind, _, _ := unstructured.NestedString(item.Object, "spec", "targetRef", "kind")
		target, _, _ := unstructured.NestedString(item.Object, "spec", "targetRef", "name")
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

	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	kind, _, _ := unstructured.NestedString(obj.Object, "spec", "targetRef", "kind")
	target, _, _ := unstructured.NestedString(obj.Object, "spec", "targetRef", "name")

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

// SubmitSignalArgs defines the input for kubernaut_submit_signal.
type SubmitSignalArgs struct {
	Namespace   string `json:"namespace"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Severity    string `json:"severity,omitempty"`
}

// SubmitSignalResult is the output of kubernaut_submit_signal.
type SubmitSignalResult struct {
	SignalName string `json:"signal_name"`
	Message    string `json:"message"`
}

// HandleSubmitSignal implements the kubernaut_submit_signal logic.
//
//nolint:gocritic // hugeParam: args passed by value for simplicity; not performance-critical
func HandleSubmitSignal(ctx context.Context, client dynamic.Interface, args SubmitSignalArgs, username string) (SubmitSignalResult, error) {
	if client == nil {
		return SubmitSignalResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return SubmitSignalResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if args.Kind == "" {
		return SubmitSignalResult{}, fmt.Errorf("%w: kind must not be empty", ErrInvalidInput)
	}
	if err := validate.ResourceName(args.Name); err != nil {
		return SubmitSignalResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if args.Severity != "" && !validSeverities[args.Severity] {
		return SubmitSignalResult{}, fmt.Errorf("%w: severity must be one of critical, high, medium, low, info", ErrInvalidInput)
	}
	signalName := fmt.Sprintf("sp-%s-%s-%d", args.Kind, args.Name, time.Now().UnixMilli())

	sp := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubernaut.ai/v1alpha1",
			"kind":       "SignalProcessing",
			"metadata": map[string]interface{}{
				"name":      signalName,
				"namespace": args.Namespace,
			},
			"spec": map[string]interface{}{
				"kind":        args.Kind,
				"name":        args.Name,
				"description": args.Description,
				"severity":    args.Severity,
				"reportedBy":  username,
			},
		},
	}

	created, err := client.Resource(spGVR).Namespace(args.Namespace).Create(ctx, sp, metav1.CreateOptions{})
	if err != nil {
		return SubmitSignalResult{}, ToUserFriendlyError(err)
	}

	return SubmitSignalResult{
		SignalName: created.GetName(),
		Message:    fmt.Sprintf("Signal submitted by %s for %s/%s", username, args.Kind, args.Name),
	}, nil
}

// NewSubmitSignalTool creates the kubernaut_submit_signal tool.
func NewSubmitSignalTool(client dynamic.Interface) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_submit_signal",
		Description: "Submit a new incident signal for triage and potential remediation",
	}, func(ctx tool.Context, args SubmitSignalArgs) (SubmitSignalResult, error) {
		return HandleSubmitSignal(ctx, client, args, usernameFromContext(ctx))
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
	_, err := client.Resource(rarGVR).Namespace(args.Namespace).Get(ctx, args.RARName, metav1.GetOptions{})
	if err != nil {
		return ApproveResult{}, ToUserFriendlyError(err)
	}

	patch := map[string]interface{}{
		"status": map[string]interface{}{
			"phase":     args.Decision,
			"decidedBy": username,
			"reason":    args.Reason,
		},
	}
	if args.WorkflowOverride != "" {
		if statusMap, ok := patch["status"].(map[string]interface{}); ok {
			statusMap["workflowOverride"] = args.WorkflowOverride
		}
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

	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	if IsTerminalPhase(phase) {
		return CancelRemediationResult{}, fmt.Errorf("%w: remediation %s/%s is in terminal state %q", ErrAlreadyTerminal, ns, name, phase)
	}

	patch := map[string]interface{}{
		"status": map[string]interface{}{
			"phase": "Cancelled",
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
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
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
