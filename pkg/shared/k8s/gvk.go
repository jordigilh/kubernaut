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

package k8s

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResolveGVKForKind resolves a Kubernetes Kind string to its canonical
// GroupVersionKind. Well-known core and apps kinds are resolved statically
// to avoid ambiguity when multiple API groups register the same Kind
// (e.g., metrics-server registering metrics.k8s.io/v1beta1/Node alongside
// core/v1/Node — see #310). Unknown kinds fall back to the REST mapper
// for dynamic resolution (CRDs, etc.).
//
// Callers: RemediationOrchestrator reconciler, EffectivenessMonitor reconciler.
func ResolveGVKForKind(mapper meta.RESTMapper, kind string) (schema.GroupVersionKind, error) {
	switch kind {
	case "Deployment":
		return schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, nil
	case "StatefulSet":
		return schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, nil
	case "DaemonSet":
		return schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}, nil
	case "ReplicaSet":
		return schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}, nil
	case "Pod":
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, nil
	case "Service":
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}, nil
	case "ConfigMap":
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, nil
	case "Secret":
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}, nil
	case "Endpoints":
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Endpoints"}, nil
	case "Namespace":
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, nil
	case "Job":
		return schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}, nil
	case "CronJob":
		return schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}, nil
	case "Ingress":
		return schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"}, nil
	case "NetworkPolicy":
		return schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}, nil
	case "PersistentVolumeClaim":
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}, nil
	case "Node":
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"}, nil
	case "HorizontalPodAutoscaler":
		return schema.GroupVersionKind{Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"}, nil
	case "PodDisruptionBudget":
		return schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"}, nil
	case "Certificate":
		return schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "Certificate"}, nil
	case "RemediationRequest":
		return schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationRequest"}, nil
	case "RemediationWorkflow":
		return schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationWorkflow"}, nil
	case "InvestigationSession":
		return schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "InvestigationSession"}, nil
	case "AIAnalysis":
		return schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "AIAnalysis"}, nil
	case "SignalProcessing":
		return schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "SignalProcessing"}, nil
	case "EffectivenessAssessment":
		return schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "EffectivenessAssessment"}, nil
	case "WorkflowExecution":
		return schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "WorkflowExecution"}, nil
	case "ActionType":
		return schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "ActionType"}, nil
	case "NotificationRequest":
		return schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "NotificationRequest"}, nil
	}

	if mapper != nil {
		pluralGVR, _ := meta.UnsafeGuessKindToResource(schema.GroupVersionKind{Kind: kind})
		gvks, err := mapper.KindsFor(schema.GroupVersionResource{Resource: pluralGVR.Resource})
		if err == nil && len(gvks) > 0 {
			return gvks[0], nil
		}
	}

	return schema.GroupVersionKind{}, fmt.Errorf("cannot resolve GVK for kind %q", kind)
}

// ResolveGVKWithAPIVersion resolves a Kind to its GVK using an explicit apiVersion
// when provided (e.g. "route.openshift.io/v1"), bypassing the static table and
// plural-guess heuristic. When apiVersion is empty, falls back to ResolveGVKForKind.
// Issue #1040: this eliminates ambiguity when multiple API groups register the
// same Kind (e.g. Route in route.openshift.io vs serving.knative.dev).
func ResolveGVKWithAPIVersion(mapper meta.RESTMapper, kind, apiVersion string) (schema.GroupVersionKind, error) {
	if apiVersion != "" {
		gv, err := schema.ParseGroupVersion(apiVersion)
		if err != nil {
			return schema.GroupVersionKind{}, fmt.Errorf("invalid apiVersion %q: %w", apiVersion, err)
		}
		mapping, err := mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: kind}, gv.Version)
		if err != nil {
			return schema.GroupVersionKind{}, fmt.Errorf("kind %q not found in apiVersion %q: %w", kind, apiVersion, err)
		}
		return mapping.GroupVersionKind, nil
	}
	return ResolveGVKForKind(mapper, kind)
}
