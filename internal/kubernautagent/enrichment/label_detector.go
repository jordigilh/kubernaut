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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	hpaGVR           = schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}
	pdbGVR           = schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}
	networkPolicyGVR = schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	resourceQuotaGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "resourcequotas"}
	namespaceGVR     = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
)

// LabelDetector detects cluster infrastructure characteristics for a resource.
// Per ADR-056 v1.7, label detection runs in EnrichmentService Phase 2 of the
// three-phase RCA, after owner chain resolution provides the root owner.
//
// GVR resolution uses the REST mapper to support any standard Kubernetes kind,
// including non-workload root owners like ConfigMap, Secret, and Service (#679).
type LabelDetector struct {
	dynClient dynamic.Interface
	mapper    meta.RESTMapper
}

// NewLabelDetector creates a LabelDetector backed by the given dynamic K8s client
// and REST mapper. The mapper resolves any Kubernetes kind to its GVR (#679).
func NewLabelDetector(dynClient dynamic.Interface, mapper meta.RESTMapper) *LabelDetector {
	return &LabelDetector{dynClient: dynClient, mapper: mapper}
}

// DetectLabels detects infrastructure characteristics for the given resource,
// using the resolved owner chain to inspect the root owner's annotations/labels
// and querying related resources (HPA, PDB, NetworkPolicy, ResourceQuota).
// AllDetectionCategories lists every label detection category. When the root
// resource cannot be fetched, all categories are marked as failed.
var AllDetectionCategories = []string{
	"gitOpsManaged", "helmManaged", "stateful", "serviceMesh",
	"hpaEnabled", "pdbProtected", "networkIsolated", "resourceQuotaConstrained",
}

func (d *LabelDetector) DetectLabels(ctx context.Context, kind, name, namespace string, ownerChain []OwnerChainEntry) (*DetectedLabels, map[string]QuotaResourceUsage, error) {
	result := &DetectedLabels{}
	var failed []string

	rootKind, rootName, rootNS := resolveRootOwner(kind, name, namespace, ownerChain)

	rootObj, err := d.fetchResource(ctx, rootKind, rootName, rootNS)
	if err != nil {
		slog.Warn("label detection: root owner fetch failed", "kind", rootKind, "name", rootName, "error", err)
		result.FailedDetections = append([]string(nil), AllDetectionCategories...)
		return result, nil, nil
	}

	podTemplateAnnotations := extractPodTemplateAnnotations(rootObj)

	var nsObj *unstructured.Unstructured
	if rootNS != "" {
		nsObj = d.fetchNamespace(ctx, rootNS)
	}

	detectGitOps(rootObj, podTemplateAnnotations, nsObj, result)
	detectHelm(rootObj, result)
	detectStateful(ownerChain, result)
	detectServiceMesh(rootObj, podTemplateAnnotations, result)
	d.detectHPA(ctx, ownerChain, rootNS, result, &failed)
	d.detectPDB(ctx, rootObj, rootNS, result, &failed)
	d.detectNetworkPolicy(ctx, rootNS, result, &failed)
	quotaSummary := d.detectResourceQuota(ctx, rootNS, result, &failed)

	if len(failed) > 0 {
		result.FailedDetections = failed
	}

	slog.Debug("label detection complete",
		"root", rootKind+"/"+rootName,
		"namespace", rootNS,
		"gitOpsManaged", result.GitOpsManaged,
		"gitOpsTool", result.GitOpsTool,
		"helmManaged", result.HelmManaged,
		"stateful", result.Stateful,
		"serviceMesh", result.ServiceMesh,
		"hpaEnabled", result.HPAEnabled,
		"pdbProtected", result.PDBProtected,
		"networkIsolated", result.NetworkIsolated,
		"resourceQuotaConstrained", result.ResourceQuotaConstrained,
		"quotaSummaryKeys", len(quotaSummary),
		"failedDetections", result.FailedDetections,
	)

	return result, quotaSummary, nil
}

// fetchResource retrieves a resource using scope-aware client dispatch (#762).
func (d *LabelDetector) fetchResource(ctx context.Context, kind, name, namespace string) (*unstructured.Unstructured, error) {
	mapping, err := d.resolveMapping(kind)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve GVR for kind %q: %w", kind, err)
	}
	var client dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		client = d.dynClient.Resource(mapping.Resource).Namespace(namespace)
	} else {
		client = d.dynClient.Resource(mapping.Resource)
	}
	return client.Get(ctx, name, metav1.GetOptions{})
}

// resolveMapping returns the full RESTMapping for a Kind, including Scope.
func (d *LabelDetector) resolveMapping(kind string) (*meta.RESTMapping, error) {
	if d.mapper == nil {
		return nil, fmt.Errorf("REST mapper is required for GVR resolution of kind %q", kind)
	}
	plural := strings.ToLower(kind) + "s"
	gvr, err := d.mapper.ResourceFor(schema.GroupVersionResource{Resource: plural})
	if err != nil {
		return nil, err
	}
	gvk, err := d.mapper.KindFor(gvr)
	if err != nil {
		return nil, fmt.Errorf("resolve kind for %s: %w", gvr, err)
	}
	return d.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
}

// fetchNamespace retrieves the Namespace object for namespace-level label/annotation
// checks per DD-HAPI-018. Returns nil on any error (best-effort, no failedDetections).
func (d *LabelDetector) fetchNamespace(ctx context.Context, namespace string) *unstructured.Unstructured {
	obj, err := d.dynClient.Resource(namespaceGVR).Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		slog.Warn("label detection: namespace fetch failed (best-effort skip)", "namespace", namespace, "error", err)
		return nil
	}
	return obj
}

// extractStringMap converts an unstructured map[string]interface{} to map[string]string,
// skipping non-string values. Returns nil if the input is nil or empty.
func extractStringMap(raw map[string]interface{}) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// podTemplateMetadata returns the metadata map from spec.template.metadata,
// or nil for non-workload types without a pod template.
func podTemplateMetadata(obj *unstructured.Unstructured) map[string]interface{} {
	spec, ok := obj.Object["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	tmpl, ok := spec["template"].(map[string]interface{})
	if !ok {
		return nil
	}
	md, ok := tmpl["metadata"].(map[string]interface{})
	if !ok {
		return nil
	}
	return md
}

// extractPodTemplateAnnotations extracts annotations from spec.template.metadata.annotations
// on workload resources (Deployment, StatefulSet, DaemonSet). Returns nil for non-workload types.
func extractPodTemplateAnnotations(obj *unstructured.Unstructured) map[string]string {
	md := podTemplateMetadata(obj)
	if md == nil {
		return nil
	}
	raw, _ := md["annotations"].(map[string]interface{})
	return extractStringMap(raw)
}

// detectGitOps implements the DD-HAPI-018 Detection 1 ten-priority cascade plus
// three legacy fallback keys for backward compatibility. First match wins.
func detectGitOps(rootObj *unstructured.Unstructured, podTemplateAnnotations map[string]string, nsObj *unstructured.Unstructured, result *DetectedLabels) {
	podTemplateLabels := extractPodTemplateLabels(rootObj)
	rootAnnotations := rootObj.GetAnnotations()
	rootLabels := rootObj.GetLabels()

	var nsAnnotations, nsLabels map[string]string
	if nsObj != nil {
		nsAnnotations = nsObj.GetAnnotations()
		nsLabels = nsObj.GetLabels()
	}

	type gitOpsCheck struct {
		source map[string]string
		key    string
		tool   string
	}

	// DD-HAPI-018 priorities 1-10 then legacy 11-13
	checks := []gitOpsCheck{
		{podTemplateAnnotations, "argocd.argoproj.io/tracking-id", "argocd"},  // P1
		{podTemplateLabels, "argocd.argoproj.io/instance", "argocd"},          // P2
		{rootAnnotations, "argocd.argoproj.io/tracking-id", "argocd"},         // P3
		{rootLabels, "fluxcd.io/sync-gc-mark", "flux"},                        // P4
		{rootAnnotations, "argocd.argoproj.io/instance", "argocd"},            // P5 (annotation)
		{rootLabels, "argocd.argoproj.io/instance", "argocd"},                 // P5 (label)
		{nsLabels, "argocd.argoproj.io/instance", "argocd"},                   // P6
		{nsAnnotations, "argocd.argoproj.io/tracking-id", "argocd"},           // P7
		{nsAnnotations, "fluxcd.io/sync-gc-mark", "flux"},                     // P8
		{nsAnnotations, "argocd.argoproj.io/managed", "argocd"},               // P9
		{nsAnnotations, "fluxcd.io/sync-status", "flux"},                      // P10
		// Legacy fallbacks (backward compatibility, not in DD-HAPI-018)
		{rootAnnotations, "argocd.argoproj.io/managed-by", "argocd"},          // L11
		{rootAnnotations, "fluxcd.io/sync-checksum", "flux"},                  // L12
		{rootAnnotations, "kustomize.toolkit.fluxcd.io/name", "flux"},         // L13
	}

	for _, c := range checks {
		if c.source == nil {
			continue
		}
		if v, ok := c.source[c.key]; ok && v != "" {
			result.GitOpsManaged = true
			result.GitOpsTool = c.tool
			return
		}
	}
}

func detectHelm(obj *unstructured.Unstructured, result *DetectedLabels) {
	labels := obj.GetLabels()
	if labels == nil {
		return
	}
	if v, ok := labels["app.kubernetes.io/managed-by"]; ok && strings.EqualFold(v, "Helm") {
		result.HelmManaged = true
		return
	}
	if _, ok := labels["helm.sh/chart"]; ok {
		result.HelmManaged = true
	}
}

// detectStateful iterates the full owner chain looking for any StatefulSet entry,
// per DD-HAPI-018 Detection 4 and HAPI v1.2.1 _detect_stateful.
func detectStateful(ownerChain []OwnerChainEntry, result *DetectedLabels) {
	for _, entry := range ownerChain {
		if entry.Kind == "StatefulSet" {
			result.Stateful = true
			return
		}
	}
}

// detectServiceMesh checks pod template annotations first (DD-HAPI-018 spec keys),
// then falls back to root owner intent annotations (legacy backward compat).
func detectServiceMesh(rootObj *unstructured.Unstructured, podTemplateAnnotations map[string]string, result *DetectedLabels) {
	// DD-HAPI-018 Detection 7: pod template status annotations (any value)
	if podTemplateAnnotations != nil {
		if _, ok := podTemplateAnnotations["sidecar.istio.io/status"]; ok {
			result.ServiceMesh = "istio"
			return
		}
		if _, ok := podTemplateAnnotations["linkerd.io/proxy-version"]; ok {
			result.ServiceMesh = "linkerd"
			return
		}
	}

	// Legacy fallback: root owner intent annotations (value-checked)
	rootAnnotations := rootObj.GetAnnotations()
	if rootAnnotations == nil {
		return
	}
	if v, ok := rootAnnotations["sidecar.istio.io/inject"]; ok && v == "true" {
		result.ServiceMesh = "istio"
		return
	}
	if v, ok := rootAnnotations["linkerd.io/inject"]; ok && v == "enabled" {
		result.ServiceMesh = "linkerd"
	}
}

// detectHPA matches HPA scaleTargetRef against the full owner chain, per DD-HAPI-018
// Detection 3 and HAPI v1.2.1 _detect_hpa (target_names set).
func (d *LabelDetector) detectHPA(ctx context.Context, ownerChain []OwnerChainEntry, rootNS string, result *DetectedLabels, failed *[]string) {
	if rootNS == "" {
		*failed = append(*failed, "hpaEnabled")
		return
	}

	type kindName struct{ Kind, Name string }
	targetNames := make(map[kindName]bool, len(ownerChain))
	for _, entry := range ownerChain {
		targetNames[kindName{entry.Kind, entry.Name}] = true
	}

	list, err := d.dynClient.Resource(hpaGVR).Namespace(rootNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Warn("label detection: HPA list failed", "namespace", rootNS, "error", err)
		*failed = append(*failed, "hpaEnabled")
		return
	}
	for i := range list.Items {
		targetKind, targetName := extractScaleTargetRef(&list.Items[i])
		if targetNames[kindName{targetKind, targetName}] {
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
	if rootNS == "" {
		*failed = append(*failed, "pdbProtected")
		return
	}
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
	md := podTemplateMetadata(obj)
	if md == nil {
		return nil
	}
	raw, _ := md["labels"].(map[string]interface{})
	return extractStringMap(raw)
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
	raw, _ := selector["matchLabels"].(map[string]interface{})
	return extractStringMap(raw)
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
	if namespace == "" {
		*failed = append(*failed, "networkIsolated")
		return
	}
	list, err := d.dynClient.Resource(networkPolicyGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Warn("label detection: NetworkPolicy list failed", "namespace", namespace, "error", err)
		*failed = append(*failed, "networkIsolated")
		return
	}
	result.NetworkIsolated = len(list.Items) > 0
}

// detectResourceQuota checks for ResourceQuota presence and builds a typed
// quota summary per DD-HAPI-018 Detection 8 and HAPI v1.2.1 _summarize_quotas.
func (d *LabelDetector) detectResourceQuota(ctx context.Context, namespace string, result *DetectedLabels, failed *[]string) map[string]QuotaResourceUsage {
	if namespace == "" {
		*failed = append(*failed, "resourceQuotaConstrained")
		return nil
	}
	list, err := d.dynClient.Resource(resourceQuotaGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Warn("label detection: ResourceQuota list failed", "namespace", namespace, "error", err)
		*failed = append(*failed, "resourceQuotaConstrained")
		return nil
	}
	if len(list.Items) == 0 {
		return nil
	}
	result.ResourceQuotaConstrained = true
	return summarizeQuotas(list.Items)
}

// summarizeQuotas aggregates status.hard and status.used from all ResourceQuota
// items. Per resource key, the first quota that defines it wins (matching HAPI
// v1.2.1 _summarize_quotas first-wins semantics).
func summarizeQuotas(items []unstructured.Unstructured) map[string]QuotaResourceUsage {
	summary := make(map[string]QuotaResourceUsage)
	for i := range items {
		status, ok := items[i].Object["status"].(map[string]interface{})
		if !ok {
			continue
		}
		hard, _ := status["hard"].(map[string]interface{})
		used, _ := status["used"].(map[string]interface{})

		for resource, val := range hard {
			if _, exists := summary[resource]; exists {
				continue
			}
			entry := QuotaResourceUsage{Hard: fmt.Sprintf("%v", val)}
			if usedVal, ok := used[resource]; ok {
				entry.Used = fmt.Sprintf("%v", usedVal)
			}
			summary[resource] = entry
		}

		for resource, val := range used {
			if _, exists := summary[resource]; exists {
				continue
			}
			summary[resource] = QuotaResourceUsage{Used: fmt.Sprintf("%v", val)}
		}
	}
	if len(summary) == 0 {
		return nil
	}
	return summary
}
