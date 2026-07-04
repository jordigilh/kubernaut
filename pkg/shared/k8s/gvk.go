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

// staticGVKByKind holds the well-known core/apps/kubernaut.ai Kind ->
// GroupVersionKind mappings resolved without consulting the REST mapper (see
// ResolveGVKForKind). Expressed as a map rather than a switch to keep
// cyclomatic complexity low for what is otherwise a flat lookup table.
var staticGVKByKind = map[string]schema.GroupVersionKind{
	"Deployment":              {Group: "apps", Version: "v1", Kind: "Deployment"},
	"StatefulSet":             {Group: "apps", Version: "v1", Kind: "StatefulSet"},
	"DaemonSet":               {Group: "apps", Version: "v1", Kind: "DaemonSet"},
	"ReplicaSet":              {Group: "apps", Version: "v1", Kind: "ReplicaSet"},
	"Pod":                     {Group: "", Version: "v1", Kind: "Pod"},
	"Service":                 {Group: "", Version: "v1", Kind: "Service"},
	"ConfigMap":               {Group: "", Version: "v1", Kind: "ConfigMap"},
	"Secret":                  {Group: "", Version: "v1", Kind: "Secret"},
	"Endpoints":               {Group: "", Version: "v1", Kind: "Endpoints"},
	"Namespace":               {Group: "", Version: "v1", Kind: "Namespace"},
	"Job":                     {Group: "batch", Version: "v1", Kind: "Job"},
	"CronJob":                 {Group: "batch", Version: "v1", Kind: "CronJob"},
	"Ingress":                 {Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},
	"NetworkPolicy":           {Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"},
	"PersistentVolumeClaim":   {Group: "", Version: "v1", Kind: "PersistentVolumeClaim"},
	"Node":                    {Group: "", Version: "v1", Kind: "Node"},
	"HorizontalPodAutoscaler": {Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"},
	"PodDisruptionBudget":     {Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"},
	"Certificate":             {Group: "cert-manager.io", Version: "v1", Kind: "Certificate"},
	"RemediationRequest":      {Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationRequest"},
	"RemediationWorkflow":     {Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationWorkflow"},
	"InvestigationSession":    {Group: "kubernaut.ai", Version: "v1alpha1", Kind: "InvestigationSession"},
	"AIAnalysis":              {Group: "kubernaut.ai", Version: "v1alpha1", Kind: "AIAnalysis"},
	"SignalProcessing":        {Group: "kubernaut.ai", Version: "v1alpha1", Kind: "SignalProcessing"},
	"EffectivenessAssessment": {Group: "kubernaut.ai", Version: "v1alpha1", Kind: "EffectivenessAssessment"},
	"WorkflowExecution":       {Group: "kubernaut.ai", Version: "v1alpha1", Kind: "WorkflowExecution"},
	"ActionType":              {Group: "kubernaut.ai", Version: "v1alpha1", Kind: "ActionType"},
	"NotificationRequest":     {Group: "kubernaut.ai", Version: "v1alpha1", Kind: "NotificationRequest"},
}

// ResolveGVKForKind resolves a Kubernetes Kind string to its canonical
// GroupVersionKind. Well-known core and apps kinds are resolved statically
// to avoid ambiguity when multiple API groups register the same Kind
// (e.g., metrics-server registering metrics.k8s.io/v1beta1/Node alongside
// core/v1/Node — see #310). Unknown kinds fall back to the REST mapper
// for dynamic resolution (CRDs, etc.).
//
// Callers: RemediationOrchestrator reconciler, EffectivenessMonitor reconciler.
func ResolveGVKForKind(mapper meta.RESTMapper, kind string) (schema.GroupVersionKind, error) {
	if gvk, ok := staticGVKByKind[kind]; ok {
		return gvk, nil
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
