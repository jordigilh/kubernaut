/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package creator

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// Label key constants for owner labels.
// Issue #416: Label-based notification routing
const (
	LabelTeam                = "kubernaut.ai/team"
	LabelOwner               = "kubernaut.ai/owner"
	LabelNotificationChannel = "kubernaut.ai/notification-channel"
)

// ownerLabelKeys lists the kubernaut labels to extract from resources.
var ownerLabelKeys = []string{LabelTeam, LabelOwner, LabelNotificationChannel}

// kindToGVR maps common Kubernetes resource kinds to their GroupVersionResource.
var kindToGVR = map[string]schema.GroupVersionResource{
	"Deployment":  {Group: "apps", Version: "v1", Resource: "deployments"},
	"StatefulSet": {Group: "apps", Version: "v1", Resource: "statefulsets"},
	"DaemonSet":   {Group: "apps", Version: "v1", Resource: "daemonsets"},
	"ReplicaSet":  {Group: "apps", Version: "v1", Resource: "replicasets"},
	"Pod":         {Group: "", Version: "v1", Resource: "pods"},
	"Service":     {Group: "", Version: "v1", Resource: "services"},
	"Node":        {Group: "", Version: "v1", Resource: "nodes"},
	"ConfigMap":   {Group: "", Version: "v1", Resource: "configmaps"},
	"Secret":      {Group: "", Version: "v1", Resource: "secrets"},
}

// ResolveOwnerLabels reads kubernaut owner labels from the target resource,
// falling back to namespace labels for namespaced resources.
// Returns an empty map (not error) when the resource is deleted or has no labels.
// Issue #416: Label-based notification routing
func ResolveOwnerLabels(ctx context.Context, c client.Client, target remediationv1.ResourceIdentifier) (map[string]string, error) {
	logger := log.FromContext(ctx)

	labels := extractLabelsFromResource(ctx, c, target)
	if hasAnyOwnerLabel(labels) {
		return filterOwnerLabels(labels), nil
	}

	// Namespace fallback for namespaced resources only
	if target.Namespace == "" {
		logger.V(1).Info("Cluster-scoped resource with no owner labels, returning empty",
			"kind", target.Kind, "name", target.Name)
		return map[string]string{}, nil
	}

	nsLabels := extractNamespaceLabels(ctx, c, target.Namespace)
	if hasAnyOwnerLabel(nsLabels) {
		logger.V(1).Info("Owner labels resolved from namespace",
			"namespace", target.Namespace)
		return filterOwnerLabels(nsLabels), nil
	}

	return map[string]string{}, nil
}

// extractLabelsFromResource gets labels from the target K8s resource.
// Returns nil on any error (graceful degradation).
func extractLabelsFromResource(ctx context.Context, c client.Client, target remediationv1.ResourceIdentifier) map[string]string {
	logger := log.FromContext(ctx)

	// Try typed resources first for common kinds
	switch target.Kind {
	case "Deployment":
		obj := &appsv1.Deployment{}
		if err := c.Get(ctx, client.ObjectKey{Name: target.Name, Namespace: target.Namespace}, obj); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.V(1).Info("Failed to get resource labels", "kind", target.Kind, "error", err)
			}
			return nil
		}
		return obj.GetLabels()
	case "Node":
		obj := &corev1.Node{}
		if err := c.Get(ctx, client.ObjectKey{Name: target.Name}, obj); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.V(1).Info("Failed to get resource labels", "kind", target.Kind, "error", err)
			}
			return nil
		}
		return obj.GetLabels()
	default:
		return extractLabelsViaUnstructured(ctx, c, target)
	}
}

// extractLabelsViaUnstructured uses unstructured client for unknown resource kinds.
func extractLabelsViaUnstructured(ctx context.Context, c client.Client, target remediationv1.ResourceIdentifier) map[string]string {
	logger := log.FromContext(ctx)

	gvr, ok := kindToGVR[target.Kind]
	if !ok {
		logger.V(1).Info("Unknown resource kind, skipping label resolution", "kind", target.Kind)
		return nil
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gvr.Group,
		Version: gvr.Version,
		Kind:    target.Kind,
	})

	key := client.ObjectKey{Name: target.Name, Namespace: target.Namespace}
	if err := c.Get(ctx, key, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.V(1).Info("Failed to get resource via unstructured", "kind", target.Kind, "error", err)
		}
		return nil
	}
	return obj.GetLabels()
}

// extractNamespaceLabels reads labels from a Namespace object.
func extractNamespaceLabels(ctx context.Context, c client.Client, namespace string) map[string]string {
	logger := log.FromContext(ctx)
	ns := &corev1.Namespace{}
	if err := c.Get(ctx, client.ObjectKey{Name: namespace}, ns); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.V(1).Info("Failed to get namespace labels", "namespace", namespace, "error", err)
		}
		return nil
	}
	return ns.GetLabels()
}

// hasAnyOwnerLabel checks if any kubernaut owner label is present.
func hasAnyOwnerLabel(labels map[string]string) bool {
	for _, key := range ownerLabelKeys {
		if _, ok := labels[key]; ok {
			return true
		}
	}
	return false
}

// filterOwnerLabels extracts only the kubernaut owner labels from a labels map.
func filterOwnerLabels(labels map[string]string) map[string]string {
	result := make(map[string]string)
	for _, key := range ownerLabelKeys {
		if val, ok := labels[key]; ok {
			result[key] = val
		}
	}
	return result
}
