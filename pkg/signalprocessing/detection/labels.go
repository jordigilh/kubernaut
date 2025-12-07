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

// Package detection provides auto-detection of cluster characteristics.
//
// # Business Requirements
//
// BR-SP-101: DetectedLabels Auto-Detection - 8 cluster characteristics
// BR-SP-103: FailedDetections Tracking - RBAC, timeout, network errors
//
// # Design Decisions
//
// DD-WORKFLOW-001 v2.3: Detection methods documented with specific annotations
//
// # Detection Types (8)
//
//  1. gitOpsManaged/gitOpsTool - ArgoCD/Flux annotations
//  2. pdbProtected - PodDisruptionBudget query
//  3. hpaEnabled - HorizontalPodAutoscaler query
//  4. stateful - StatefulSet in owner chain (no API call)
//  5. helmManaged - Helm labels
//  6. networkIsolated - NetworkPolicy query
//  7. serviceMesh - Istio/Linkerd pod annotations
package detection

import (
	"context"

	"github.com/go-logr/logr"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// LabelDetector auto-detects 8 cluster characteristics from K8s resources.
// BR-SP-101: DetectedLabels Auto-Detection
// BR-SP-103: FailedDetections Tracking
// DD-WORKFLOW-001 v2.3: Detection methods documented
type LabelDetector struct {
	client client.Client
	logger logr.Logger
}

// NewLabelDetector creates a new LabelDetector.
// Per BR-SP-101: Auto-detect cluster characteristics without customer configuration.
func NewLabelDetector(c client.Client, logger logr.Logger) *LabelDetector {
	return &LabelDetector{
		client: c,
		logger: logger.WithName("detection"),
	}
}

// DetectLabels detects 8 label types from K8s context.
// Per DD-WORKFLOW-001 v2.3: Tracks QUERY FAILURES in FailedDetections field.
//
// Parameters:
//   - ctx: Context for K8s API calls
//   - k8sCtx: Kubernetes context with namespace, pod, deployment details
//   - ownerChain: Owner chain from Day 7 OwnerChain Builder (for stateful detection)
//
// IMPORTANT DISTINCTION (BR-SP-103):
//   - Resource doesn't exist (PDB not found) → false (normal, NOT an error)
//   - Can't query resource (RBAC denied, timeout) → false + FailedDetections + warn log
func (d *LabelDetector) DetectLabels(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext, ownerChain []sharedtypes.OwnerChainEntry) *sharedtypes.DetectedLabels {
	if k8sCtx == nil {
		return nil
	}

	labels := &sharedtypes.DetectedLabels{}
	var failedDetections []string // Track QUERY failures only (DD-WORKFLOW-001 v2.3)

	// 1. GitOps detection (ArgoCD/Flux) - NO API call needed
	d.detectGitOps(k8sCtx, labels)

	// 2. PDB protection detection - K8s API query
	if err := d.detectPDB(ctx, k8sCtx, labels); err != nil {
		d.logger.V(1).Info("Could not query PodDisruptionBudgets", "error", err)
		failedDetections = append(failedDetections, "pdbProtected")
	}

	// 3. HPA detection - K8s API query
	if err := d.detectHPA(ctx, k8sCtx, labels); err != nil {
		d.logger.V(1).Info("Could not query HorizontalPodAutoscalers", "error", err)
		failedDetections = append(failedDetections, "hpaEnabled")
	}

	// 4. StatefulSet detection - uses owner chain (NO API call)
	labels.Stateful = d.isStateful(ownerChain)

	// 5. Helm managed detection - NO API call needed
	d.detectHelm(k8sCtx, labels)

	// 6. Network isolation detection - K8s API query
	if err := d.detectNetworkPolicy(ctx, k8sCtx, labels); err != nil {
		d.logger.V(1).Info("Could not query NetworkPolicies", "error", err)
		failedDetections = append(failedDetections, "networkIsolated")
	}

	// 7. Service Mesh detection (Istio/Linkerd) - NO API call needed
	d.detectServiceMesh(k8sCtx, labels)

	// Set FailedDetections only if we had QUERY failures (DD-WORKFLOW-001 v2.3)
	if len(failedDetections) > 0 {
		labels.FailedDetections = failedDetections
		d.logger.Info("Some label detections failed (RBAC or timeout)",
			"failedDetections", failedDetections)
	}

	return labels
}

// detectGitOps detects ArgoCD or Flux GitOps management.
// DD-WORKFLOW-001 v2.3:
//   - ArgoCD: argocd.argoproj.io/instance annotation
//   - Flux: fluxcd.io/sync-gc-mark label
//
// NO API call needed - uses existing data from KubernetesContext.
func (d *LabelDetector) detectGitOps(k8sCtx *sharedtypes.KubernetesContext, result *sharedtypes.DetectedLabels) {
	// Check pod annotations first (most reliable - post-injection)
	if k8sCtx.PodDetails != nil {
		// ArgoCD annotation
		if _, ok := k8sCtx.PodDetails.Annotations["argocd.argoproj.io/instance"]; ok {
			result.GitOpsManaged = true
			result.GitOpsTool = "argocd"
			return
		}
	}

	// Check deployment labels
	if k8sCtx.DeploymentDetails != nil {
		// Flux label
		if _, ok := k8sCtx.DeploymentDetails.Labels["fluxcd.io/sync-gc-mark"]; ok {
			result.GitOpsManaged = true
			result.GitOpsTool = "flux"
			return
		}
		// ArgoCD can also be on deployment
		if _, ok := k8sCtx.DeploymentDetails.Labels["argocd.argoproj.io/instance"]; ok {
			result.GitOpsManaged = true
			result.GitOpsTool = "argocd"
			return
		}
	}

	// Check namespace labels
	if k8sCtx.NamespaceLabels != nil {
		if _, ok := k8sCtx.NamespaceLabels["argocd.argoproj.io/instance"]; ok {
			result.GitOpsManaged = true
			result.GitOpsTool = "argocd"
			return
		}
		if _, ok := k8sCtx.NamespaceLabels["fluxcd.io/sync-gc-mark"]; ok {
			result.GitOpsManaged = true
			result.GitOpsTool = "flux"
			return
		}
	}

	// No GitOps detected
	result.GitOpsManaged = false
	result.GitOpsTool = ""
}

// detectPDB checks if a PodDisruptionBudget exists matching the pod labels.
// Returns error only on query failure (RBAC, timeout), not when PDB doesn't exist.
func (d *LabelDetector) detectPDB(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext, result *sharedtypes.DetectedLabels) error {
	// Need pod labels to match PDB selector
	if k8sCtx.PodDetails == nil || len(k8sCtx.PodDetails.Labels) == 0 {
		result.PDBProtected = false
		return nil
	}

	// List all PDBs in namespace
	pdbList := &policyv1.PodDisruptionBudgetList{}
	if err := d.client.List(ctx, pdbList, client.InNamespace(k8sCtx.Namespace)); err != nil {
		return err // Query failed - caller will track in FailedDetections
	}

	// Check if any PDB selector matches pod labels
	podLabels := labels.Set(k8sCtx.PodDetails.Labels)
	for _, pdb := range pdbList.Items {
		if pdb.Spec.Selector == nil {
			continue
		}
		selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err != nil {
			continue
		}
		if selector.Matches(podLabels) {
			result.PDBProtected = true
			return nil
		}
	}

	// No matching PDB found - this is NOT an error
	result.PDBProtected = false
	return nil
}

// detectHPA checks if a HorizontalPodAutoscaler targets the deployment.
// Returns error only on query failure (RBAC, timeout), not when HPA doesn't exist.
func (d *LabelDetector) detectHPA(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext, result *sharedtypes.DetectedLabels) error {
	// Need deployment name to match HPA scaleTargetRef
	if k8sCtx.DeploymentDetails == nil || k8sCtx.DeploymentDetails.Name == "" {
		result.HPAEnabled = false
		return nil
	}

	// List all HPAs in namespace
	hpaList := &autoscalingv2.HorizontalPodAutoscalerList{}
	if err := d.client.List(ctx, hpaList, client.InNamespace(k8sCtx.Namespace)); err != nil {
		return err // Query failed - caller will track in FailedDetections
	}

	// Check if any HPA targets this deployment
	for _, hpa := range hpaList.Items {
		if hpa.Spec.ScaleTargetRef.Kind == "Deployment" &&
			hpa.Spec.ScaleTargetRef.Name == k8sCtx.DeploymentDetails.Name {
			result.HPAEnabled = true
			return nil
		}
	}

	// No matching HPA found - this is NOT an error
	result.HPAEnabled = false
	return nil
}

// isStateful checks if the owner chain contains a StatefulSet.
// Per DD-WORKFLOW-001 v2.3: Uses owner chain from Day 7, no API call needed.
func (d *LabelDetector) isStateful(ownerChain []sharedtypes.OwnerChainEntry) bool {
	for _, owner := range ownerChain {
		if owner.Kind == "StatefulSet" {
			return true
		}
	}
	return false
}

// detectHelm checks for Helm management labels.
// DD-WORKFLOW-001 v2.3:
//   - app.kubernetes.io/managed-by: Helm
//   - helm.sh/chart annotation
//
// NO API call needed - uses existing data from KubernetesContext.
func (d *LabelDetector) detectHelm(k8sCtx *sharedtypes.KubernetesContext, result *sharedtypes.DetectedLabels) {
	if k8sCtx.DeploymentDetails == nil {
		result.HelmManaged = false
		return
	}

	// Check for Helm managed-by label
	if managedBy, ok := k8sCtx.DeploymentDetails.Labels["app.kubernetes.io/managed-by"]; ok {
		if managedBy == "Helm" {
			result.HelmManaged = true
			return
		}
	}

	// Check for Helm chart label
	if _, ok := k8sCtx.DeploymentDetails.Labels["helm.sh/chart"]; ok {
		result.HelmManaged = true
		return
	}

	result.HelmManaged = false
}

// detectNetworkPolicy checks if any NetworkPolicy exists in the namespace.
// Returns error only on query failure (RBAC, timeout), not when NetworkPolicy doesn't exist.
func (d *LabelDetector) detectNetworkPolicy(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext, result *sharedtypes.DetectedLabels) error {
	// List NetworkPolicies in namespace
	netpolList := &networkingv1.NetworkPolicyList{}
	if err := d.client.List(ctx, netpolList, client.InNamespace(k8sCtx.Namespace)); err != nil {
		return err // Query failed - caller will track in FailedDetections
	}

	// Any NetworkPolicy in namespace means network isolation is active
	result.NetworkIsolated = len(netpolList.Items) > 0
	return nil
}

// detectServiceMesh checks for Istio or Linkerd service mesh.
// DD-WORKFLOW-001 v2.3:
//   - Istio: sidecar.istio.io/status annotation (present after injection)
//   - Linkerd: linkerd.io/proxy-version annotation (present after injection)
//
// NO API call needed - uses existing pod annotation data.
func (d *LabelDetector) detectServiceMesh(k8sCtx *sharedtypes.KubernetesContext, result *sharedtypes.DetectedLabels) {
	if k8sCtx.PodDetails == nil || k8sCtx.PodDetails.Annotations == nil {
		result.ServiceMesh = ""
		return
	}

	annotations := k8sCtx.PodDetails.Annotations

	// Check for Istio sidecar
	if _, ok := annotations["sidecar.istio.io/status"]; ok {
		result.ServiceMesh = "istio"
		return
	}

	// Check for Linkerd proxy
	if _, ok := annotations["linkerd.io/proxy-version"]; ok {
		result.ServiceMesh = "linkerd"
		return
	}

	result.ServiceMesh = ""
}
