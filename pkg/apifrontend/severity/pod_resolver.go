package severity

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// PodResolver resolves workload resources to their pod names for alert correlation.
// Implementations must be safe for concurrent use.
type PodResolver interface {
	ResolvePodNames(ctx context.Context, namespace, kind, name string) ([]string, error)
}

var workloadGVR = map[string]schema.GroupVersionResource{
	"Deployment":  {Group: "apps", Version: "v1", Resource: "deployments"},
	"StatefulSet": {Group: "apps", Version: "v1", Resource: "statefulsets"},
	"DaemonSet":   {Group: "apps", Version: "v1", Resource: "daemonsets"},
}

var podGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

// K8sPodResolver resolves workload resources to their pod names via spec.selector.
// Uses LabelSelectorAsSelector to support both matchLabels and matchExpressions.
// Safe for concurrent use (stateless beyond the injected client and logger).
type K8sPodResolver struct {
	client dynamic.Interface
	logger logr.Logger
}

// NewK8sPodResolver creates a PodResolver backed by a dynamic K8s client.
func NewK8sPodResolver(client dynamic.Interface, logger logr.Logger) *K8sPodResolver {
	return &K8sPodResolver{client: client, logger: logger}
}

// ResolvePodNames returns the names of pods owned by the specified workload resource.
//
// Supported kinds: Deployment, StatefulSet, DaemonSet.
// Unsupported kinds return (nil, nil) for graceful degradation.
// Missing workloads (NotFound) return (nil, nil).
// Empty selectors return (nil, nil) to avoid listing all namespace pods (M8).
func (r *K8sPodResolver) ResolvePodNames(ctx context.Context, namespace, kind, name string) ([]string, error) {
	gvr, ok := workloadGVR[kind]
	if !ok {
		r.logger.V(1).Info("unsupported kind for pod resolution, skipping",
			"kind", kind, "name", name, "namespace", namespace)
		return nil, nil
	}

	obj, err := r.client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.logger.V(1).Info("workload not found, skipping pod resolution",
				"kind", kind, "name", name, "namespace", namespace)
			return nil, nil
		}
		return nil, fmt.Errorf("get %s %s/%s: %w", kind, name, namespace, err)
	}

	spec, ok := obj.Object["spec"].(map[string]interface{})
	if !ok {
		r.logger.V(1).Info("workload has no spec, skipping pod resolution",
			"kind", kind, "name", name, "namespace", namespace)
		return nil, nil
	}
	selectorRaw, ok := spec["selector"].(map[string]interface{})
	if !ok {
		r.logger.V(1).Info("workload spec has no selector, skipping pod resolution",
			"kind", kind, "name", name, "namespace", namespace)
		return nil, nil
	}

	selectorJSON, err := json.Marshal(selectorRaw)
	if err != nil {
		r.logger.V(1).Info("failed to marshal selector, skipping pod resolution",
			"kind", kind, "name", name, "namespace", namespace, "error", err.Error())
		return nil, nil
	}
	var labelSelector metav1.LabelSelector
	if err := json.Unmarshal(selectorJSON, &labelSelector); err != nil {
		r.logger.V(1).Info("failed to unmarshal selector, skipping pod resolution",
			"kind", kind, "name", name, "namespace", namespace, "error", err.Error())
		return nil, nil
	}

	selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
	if err != nil {
		r.logger.V(1).Info("invalid label selector, skipping pod resolution",
			"kind", kind, "name", name, "namespace", namespace, "error", err.Error())
		return nil, nil
	}

	if selector.Empty() {
		r.logger.V(1).Info("empty selector guard: skipping pod resolution to avoid listing all pods",
			"kind", kind, "name", name, "namespace", namespace)
		return nil, nil
	}

	podList, err := r.client.Resource(podGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("list pods for %s/%s in %s: %w", kind, name, namespace, err)
	}

	names := make([]string, 0, len(podList.Items))
	for _, pod := range podList.Items {
		names = append(names, pod.GetName())
	}
	return names, nil
}
