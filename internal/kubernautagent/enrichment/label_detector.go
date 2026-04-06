/*
Copyright 2026 Jordi Gil.

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

package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var knownWorkloadGVRs = map[string]schema.GroupVersionResource{
	"Deployment":  {Group: "apps", Version: "v1", Resource: "deployments"},
	"StatefulSet": {Group: "apps", Version: "v1", Resource: "statefulsets"},
	"DaemonSet":   {Group: "apps", Version: "v1", Resource: "daemonsets"},
	"ReplicaSet":  {Group: "apps", Version: "v1", Resource: "replicasets"},
	"Job":         {Group: "batch", Version: "v1", Resource: "jobs"},
	"CronJob":     {Group: "batch", Version: "v1", Resource: "cronjobs"},
	"Pod":         {Group: "", Version: "v1", Resource: "pods"},
}

var (
	hpaGVR           = schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}
	pdbGVR           = schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}
	networkPolicyGVR = schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	resourceQuotaGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "resourcequotas"}
)

// LabelDetector detects cluster infrastructure characteristics for a resource.
// Per ADR-056 v1.7, label detection runs in EnrichmentService Phase 2 of the
// three-phase RCA, after owner chain resolution provides the root owner.
type LabelDetector struct {
	dynClient dynamic.Interface
}

// NewLabelDetector creates a LabelDetector backed by the given dynamic K8s client.
func NewLabelDetector(dynClient dynamic.Interface) *LabelDetector {
	return &LabelDetector{dynClient: dynClient}
}

// DetectLabels detects infrastructure characteristics for the given resource,
// using the resolved owner chain to inspect the root owner's annotations/labels
// and querying related resources (HPA, PDB, NetworkPolicy, ResourceQuota).
func (d *LabelDetector) DetectLabels(ctx context.Context, kind, name, namespace string, ownerChain []OwnerChainEntry) (*DetectedLabels, error) {
	result := &DetectedLabels{}
	var failed []string

	rootKind, rootName, rootNS := resolveRootOwner(kind, name, namespace, ownerChain)

	rootObj, err := d.fetchResource(ctx, rootKind, rootName, rootNS)
	if err != nil {
		slog.Warn("label detection: root owner fetch failed", "kind", rootKind, "name", rootName, "error", err)
		result.FailedDetections = []string{
			"gitOpsManaged", "helmManaged", "stateful", "serviceMesh",
			"hpaEnabled", "pdbProtected", "networkIsolated", "resourceQuotaConstrained",
		}
		return result, nil
	}

	detectGitOps(rootObj, result)
	detectHelm(rootObj, result)
	detectStateful(rootKind, result)
	detectServiceMesh(rootObj, result)
	d.detectHPA(ctx, rootKind, rootName, rootNS, result, &failed)
	d.detectPDB(ctx, rootObj, rootNS, result, &failed)
	d.detectNetworkPolicy(ctx, rootNS, result, &failed)
	d.detectResourceQuota(ctx, rootNS, result, &failed)

	if len(failed) > 0 {
		result.FailedDetections = failed
	}
	return result, nil
}

func (d *LabelDetector) fetchResource(ctx context.Context, kind, name, namespace string) (*unstructured.Unstructured, error) {
	gvr, ok := knownWorkloadGVRs[kind]
	if !ok {
		return nil, fmt.Errorf("unknown kind %q for label detection", kind)
	}
	var client dynamic.ResourceInterface
	if namespace != "" {
		client = d.dynClient.Resource(gvr).Namespace(namespace)
	} else {
		client = d.dynClient.Resource(gvr)
	}
	return client.Get(ctx, name, metav1.GetOptions{})
}

func detectGitOps(obj *unstructured.Unstructured, result *DetectedLabels) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return
	}
	if v, ok := annotations["argocd.argoproj.io/managed-by"]; ok && v != "" {
		result.GitOpsManaged = true
		result.GitOpsTool = "argocd"
		return
	}
	if _, ok := annotations["fluxcd.io/sync-checksum"]; ok {
		result.GitOpsManaged = true
		result.GitOpsTool = "flux"
		return
	}
	if _, ok := annotations["kustomize.toolkit.fluxcd.io/name"]; ok {
		result.GitOpsManaged = true
		result.GitOpsTool = "flux"
	}
}

func detectHelm(obj *unstructured.Unstructured, result *DetectedLabels) {
	labels := obj.GetLabels()
	if labels == nil {
		return
	}
	if v, ok := labels["app.kubernetes.io/managed-by"]; ok && strings.EqualFold(v, "Helm") {
		result.HelmManaged = true
	}
}

func detectStateful(rootKind string, result *DetectedLabels) {
	result.Stateful = rootKind == "StatefulSet"
}

func detectServiceMesh(obj *unstructured.Unstructured, result *DetectedLabels) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return
	}
	if v, ok := annotations["sidecar.istio.io/inject"]; ok && v == "true" {
		result.ServiceMesh = "istio"
		return
	}
	if v, ok := annotations["linkerd.io/inject"]; ok && v == "enabled" {
		result.ServiceMesh = "linkerd"
	}
}

func (d *LabelDetector) detectHPA(ctx context.Context, rootKind, rootName, rootNS string, result *DetectedLabels, failed *[]string) {
	list, err := d.dynClient.Resource(hpaGVR).Namespace(rootNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Warn("label detection: HPA list failed", "namespace", rootNS, "error", err)
		*failed = append(*failed, "hpaEnabled")
		return
	}
	for i := range list.Items {
		targetKind, targetName := extractScaleTargetRef(&list.Items[i])
		if targetKind == rootKind && targetName == rootName {
			result.HPAEnabled = true
			return
		}
	}
}

func extractScaleTargetRef(hpa *unstructured.Unstructured) (string, string) {
	spec, ok := hpa.Object["spec"].(map[string]interface{})
	if !ok {
		return "", ""
	}
	ref, ok := spec["scaleTargetRef"].(map[string]interface{})
	if !ok {
		return "", ""
	}
	kind, _ := ref["kind"].(string)
	name, _ := ref["name"].(string)
	return kind, name
}

func (d *LabelDetector) detectPDB(ctx context.Context, rootObj *unstructured.Unstructured, rootNS string, result *DetectedLabels, failed *[]string) {
	// PDB selectors match pod labels, not the controller's own metadata.labels.
	// For Deployments/StatefulSets/DaemonSets/ReplicaSets, extract the pod
	// template labels from spec.template.metadata.labels.
	podLabels := extractPodTemplateLabels(rootObj)
	rootLabels := rootObj.GetLabels()

	if len(podLabels) == 0 && len(rootLabels) == 0 {
		return
	}
	list, err := d.dynClient.Resource(pdbGVR).Namespace(rootNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Warn("label detection: PDB list failed", "namespace", rootNS, "error", err)
		*failed = append(*failed, "pdbProtected")
		return
	}
	for i := range list.Items {
		matchLabels := extractPDBMatchLabels(&list.Items[i])
		if len(matchLabels) == 0 {
			continue
		}
		if (len(podLabels) > 0 && labelsSubset(matchLabels, podLabels)) ||
			(len(rootLabels) > 0 && labelsSubset(matchLabels, rootLabels)) {
			result.PDBProtected = true
			return
		}
	}
}

func extractPodTemplateLabels(obj *unstructured.Unstructured) map[string]string {
	spec, ok := obj.Object["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	tmpl, ok := spec["template"].(map[string]interface{})
	if !ok {
		return nil
	}
	meta, ok := tmpl["metadata"].(map[string]interface{})
	if !ok {
		return nil
	}
	raw, ok := meta["labels"].(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func extractPDBMatchLabels(pdb *unstructured.Unstructured) map[string]string {
	spec, ok := pdb.Object["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	selector, ok := spec["selector"].(map[string]interface{})
	if !ok {
		return nil
	}
	raw, ok := selector["matchLabels"].(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func labelsSubset(subset, superset map[string]string) bool {
	for k, v := range subset {
		if superset[k] != v {
			return false
		}
	}
	return true
}

func (d *LabelDetector) detectNetworkPolicy(ctx context.Context, namespace string, result *DetectedLabels, failed *[]string) {
	list, err := d.dynClient.Resource(networkPolicyGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Warn("label detection: NetworkPolicy list failed", "namespace", namespace, "error", err)
		*failed = append(*failed, "networkIsolated")
		return
	}
	result.NetworkIsolated = len(list.Items) > 0
}

func (d *LabelDetector) detectResourceQuota(ctx context.Context, namespace string, result *DetectedLabels, failed *[]string) {
	list, err := d.dynClient.Resource(resourceQuotaGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Warn("label detection: ResourceQuota list failed", "namespace", namespace, "error", err)
		*failed = append(*failed, "resourceQuotaConstrained")
		return
	}
	result.ResourceQuotaConstrained = len(list.Items) > 0
}
