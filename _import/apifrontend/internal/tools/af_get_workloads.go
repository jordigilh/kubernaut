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

// GetWorkloadsArgs defines the input for af_get_workloads.
type GetWorkloadsArgs struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name,omitempty"`
}

// WorkloadReplicaStatus summarizes desired and observed replica counts.
type WorkloadReplicaStatus struct {
	Desired   int64 `json:"desired"`
	Ready     int64 `json:"ready"`
	Available int64 `json:"available"`
}

// WorkloadSummary is a compact view of a Deployment or StatefulSet.
type WorkloadSummary struct {
	Name       string                `json:"name"`
	Kind       string                `json:"kind"`
	Namespace  string                `json:"namespace"`
	Replicas   WorkloadReplicaStatus `json:"replicas"`
	Conditions []string              `json:"conditions,omitempty"`
}

// GetWorkloadsResult is the output of af_get_workloads.
type GetWorkloadsResult struct {
	Workloads []WorkloadSummary `json:"workloads"`
	Count     int               `json:"count"`
	Truncated bool              `json:"truncated,omitempty"`
}

var (
	deploymentGVR  = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	statefulSetGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
)

// HandleGetWorkloads implements the af_get_workloads logic.
func HandleGetWorkloads(ctx context.Context, client dynamic.Interface, args GetWorkloadsArgs) (GetWorkloadsResult, error) {
	if client == nil {
		return GetWorkloadsResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return GetWorkloadsResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if args.Name != "" {
		if err := validate.ResourceName(args.Name); err != nil {
			return GetWorkloadsResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
	}

	deployList, err := client.Resource(deploymentGVR).Namespace(args.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return GetWorkloadsResult{}, ToUserFriendlyError(err)
	}
	ssList, err := client.Resource(statefulSetGVR).Namespace(args.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return GetWorkloadsResult{}, ToUserFriendlyError(err)
	}

	result := make([]WorkloadSummary, 0, len(deployList.Items)+len(ssList.Items))
	for i := range deployList.Items {
		item := &deployList.Items[i]
		if args.Name != "" && item.GetName() != args.Name {
			continue
		}
		result = append(result, workloadSummaryFromUnstructured(item, "Deployment", args.Namespace))
	}
	for i := range ssList.Items {
		item := &ssList.Items[i]
		if args.Name != "" && item.GetName() != args.Name {
			continue
		}
		result = append(result, workloadSummaryFromUnstructured(item, "StatefulSet", args.Namespace))
	}

	result, truncated := TrimSliceToFit(result)

	return GetWorkloadsResult{
		Workloads: result,
		Count:     len(result),
		Truncated: truncated,
	}, nil
}

func workloadSummaryFromUnstructured(item *unstructured.Unstructured, kind, namespace string) WorkloadSummary {
	desired, _, _ := unstructured.NestedInt64(item.Object, "spec", "replicas")
	ready, _, _ := unstructured.NestedInt64(item.Object, "status", "readyReplicas")
	available, _, _ := unstructured.NestedInt64(item.Object, "status", "availableReplicas")

	return WorkloadSummary{
		Name:      item.GetName(),
		Kind:      kind,
		Namespace: namespace,
		Replicas: WorkloadReplicaStatus{
			Desired:   desired,
			Ready:     ready,
			Available: available,
		},
		Conditions: extractWorkloadConditions(item.Object),
	}
}

func extractWorkloadConditions(obj map[string]interface{}) []string {
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
		typ, _, _ := unstructured.NestedString(cond, "type")
		status, _, _ := unstructured.NestedString(cond, "status")
		reason, _, _ := unstructured.NestedString(cond, "reason")
		line := typ + "=" + status
		if reason != "" {
			line += ": " + reason
		}
		result = append(result, line)
	}
	return result
}

// NewGetWorkloadsTool creates the af_get_workloads tool.
// Uses DynamicClientFactory to obtain a per-request impersonated client (SEC-05).
func NewGetWorkloadsTool(factory auth.DynamicClientFactory) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "af_get_workloads",
		Description: "List Deployment and StatefulSet workloads in a namespace with replica status and conditions, optionally filtered by resource name",
	}, func(ctx tool.Context, args GetWorkloadsArgs) (GetWorkloadsResult, error) {
		client, err := factory(ctx)
		if err != nil {
			return GetWorkloadsResult{}, fmt.Errorf("%w", ErrK8sUnavailable)
		}
		return HandleGetWorkloads(ctx, client, args)
	})
}
