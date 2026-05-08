package tools

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/validate"
)

// GetPodsArgs defines the input for af_get_pods.
type GetPodsArgs struct {
	Namespace     string `json:"namespace"`
	LabelSelector string `json:"label_selector,omitempty"`
}

// ContainerStatus is a compact view of a container's state.
type ContainerStatus struct {
	Name     string `json:"name"`
	Ready    bool   `json:"ready"`
	State    string `json:"state"`
	Reason   string `json:"reason,omitempty"`
	Restarts int64  `json:"restarts"`
}

// PodSummary is a compact view of a Pod's status.
type PodSummary struct {
	Name       string            `json:"name"`
	Phase      string            `json:"phase"`
	Conditions []string          `json:"conditions,omitempty"`
	Containers []ContainerStatus `json:"containers"`
	NodeName   string            `json:"node_name,omitempty"`
}

// GetPodsResult is the output of af_get_pods.
type GetPodsResult struct {
	Pods      []PodSummary `json:"pods"`
	Count     int          `json:"count"`
	Truncated bool         `json:"truncated,omitempty"`
}

var podsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

// HandleGetPods implements the af_get_pods logic.
func HandleGetPods(ctx context.Context, client dynamic.Interface, args GetPodsArgs) (GetPodsResult, error) {
	if client == nil {
		return GetPodsResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return GetPodsResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	opts := metav1.ListOptions{}
	if args.LabelSelector != "" {
		opts.LabelSelector = args.LabelSelector
	}

	list, err := client.Resource(podsGVR).Namespace(args.Namespace).List(ctx, opts)
	if err != nil {
		return GetPodsResult{}, ToUserFriendlyError(err)
	}

	result := make([]PodSummary, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
		nodeName, _, _ := unstructured.NestedString(item.Object, "spec", "nodeName")

		containers := extractContainerStatuses(item.Object)
		conditions := extractTrueConditions(item.Object)

		result = append(result, PodSummary{
			Name:       item.GetName(),
			Phase:      phase,
			Conditions: conditions,
			Containers: containers,
			NodeName:   nodeName,
		})
	}

	result, truncated := TrimSliceToFit(result)

	return GetPodsResult{
		Pods:      result,
		Count:     len(result),
		Truncated: truncated,
	}, nil
}

func extractContainerStatuses(obj map[string]interface{}) []ContainerStatus {
	statuses, found, _ := unstructured.NestedSlice(obj, "status", "containerStatuses")
	if !found {
		return nil
	}
	result := make([]ContainerStatus, 0, len(statuses))
	for _, s := range statuses {
		cs, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(cs, "name")
		ready, _, _ := unstructured.NestedBool(cs, "ready")
		restarts, _, _ := unstructured.NestedInt64(cs, "restartCount")
		state, reason := parseContainerState(cs)

		result = append(result, ContainerStatus{
			Name:     name,
			Ready:    ready,
			State:    state,
			Reason:   reason,
			Restarts: restarts,
		})
	}
	return result
}

func parseContainerState(cs map[string]interface{}) (state, reason string) {
	stateMap, found, _ := unstructured.NestedMap(cs, "state")
	if !found {
		return "unknown", ""
	}
	if _, ok := stateMap["running"]; ok {
		return "running", ""
	}
	if waiting, ok := stateMap["waiting"]; ok {
		if w, ok := waiting.(map[string]interface{}); ok {
			reason, _, _ := unstructured.NestedString(w, "reason")
			return "waiting", reason
		}
		return "waiting", ""
	}
	if terminated, ok := stateMap["terminated"]; ok {
		if t, ok := terminated.(map[string]interface{}); ok {
			reason, _, _ := unstructured.NestedString(t, "reason")
			return "terminated", reason
		}
		return "terminated", ""
	}
	return "unknown", ""
}

func extractTrueConditions(obj map[string]interface{}) []string {
	conditions, found, _ := unstructured.NestedSlice(obj, "status", "conditions")
	if !found {
		return nil
	}
	var result []string
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		status, _, _ := unstructured.NestedString(cond, "status")
		if status == "True" {
			condType, _, _ := unstructured.NestedString(cond, "type")
			result = append(result, condType)
		}
	}
	return result
}

// NewGetPodsTool creates the af_get_pods tool.
// Uses DynamicClientFactory to obtain a per-request impersonated client (SEC-05).
func NewGetPodsTool(factory auth.DynamicClientFactory) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "af_get_pods",
		Description: "Get pod status summaries with container states in a namespace, optionally filtered by label selector",
	}, func(ctx tool.Context, args GetPodsArgs) (GetPodsResult, error) {
		client, err := factory(ctx)
		if err != nil {
			return GetPodsResult{}, fmt.Errorf("%w", ErrK8sUnavailable)
		}
		return HandleGetPods(ctx, client, args)
	})
}
