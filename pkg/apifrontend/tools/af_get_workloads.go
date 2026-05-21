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

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
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

// WorkloadSummary is a compact view of a workload (Deployment, StatefulSet, DaemonSet, Job, or CronJob).
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
	daemonSetGVR   = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	jobGVR         = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
	cronJobGVR     = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}
)

type workloadQuery struct {
	gvr  schema.GroupVersionResource
	kind string
}

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

	queries := []workloadQuery{
		{gvr: deploymentGVR, kind: "Deployment"},
		{gvr: statefulSetGVR, kind: "StatefulSet"},
		{gvr: daemonSetGVR, kind: "DaemonSet"},
		{gvr: jobGVR, kind: "Job"},
		{gvr: cronJobGVR, kind: "CronJob"},
	}

	var result []WorkloadSummary
	for _, q := range queries {
		list, err := client.Resource(q.gvr).Namespace(args.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return GetWorkloadsResult{}, ToUserFriendlyError(err)
		}
		for i := range list.Items {
			item := &list.Items[i]
			if args.Name != "" && item.GetName() != args.Name {
				continue
			}
			result = append(result, workloadSummaryFromUnstructured(item, q.kind, args.Namespace))
		}
	}

	result, truncated := TrimSliceToFit(result)

	return GetWorkloadsResult{
		Workloads: result,
		Count:     len(result),
		Truncated: truncated,
	}, nil
}

func workloadSummaryFromUnstructured(item *unstructured.Unstructured, kind, namespace string) WorkloadSummary {
	var replicas WorkloadReplicaStatus

	switch kind {
	case "DaemonSet":
		replicas.Desired, _, _ = unstructured.NestedInt64(item.Object, "status", "desiredNumberScheduled")
		replicas.Ready, _, _ = unstructured.NestedInt64(item.Object, "status", "numberReady")
		replicas.Available, _, _ = unstructured.NestedInt64(item.Object, "status", "numberAvailable")
	case "Job":
		replicas.Desired, _, _ = unstructured.NestedInt64(item.Object, "spec", "completions")
		replicas.Ready, _, _ = unstructured.NestedInt64(item.Object, "status", "succeeded")
	case "CronJob":
		// CronJobs have no replica semantics; fields remain zero.
	default:
		replicas.Desired, _, _ = unstructured.NestedInt64(item.Object, "spec", "replicas")
		replicas.Ready, _, _ = unstructured.NestedInt64(item.Object, "status", "readyReplicas")
		replicas.Available, _, _ = unstructured.NestedInt64(item.Object, "status", "availableReplicas")
	}

	return WorkloadSummary{
		Name:       item.GetName(),
		Kind:       kind,
		Namespace:  namespace,
		Replicas:   replicas,
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
		Description: "List Deployment, StatefulSet, DaemonSet, Job, and CronJob workloads in a namespace with replica status and conditions, optionally filtered by resource name",
	}, func(ctx tool.Context, args GetWorkloadsArgs) (GetWorkloadsResult, error) {
		client, err := factory(ctx)
		if err != nil {
			return GetWorkloadsResult{}, fmt.Errorf("%w", ErrK8sUnavailable)
		}
		return HandleGetWorkloads(ctx, client, args)
	})
}
