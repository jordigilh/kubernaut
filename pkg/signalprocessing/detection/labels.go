// Package detection provides auto-detection of cluster characteristics for Signal Processing.
// BR-SP-101: DetectedLabels Auto-Detection
// DD-WORKFLOW-001 v2.2: 7 auto-detected cluster characteristics
package detection

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// LabelDetector auto-detects cluster characteristics (V1.0)
// Convention: Boolean fields only included when true, omit when false
// Uses FailedDetections to track query failures per DD-WORKFLOW-001 v2.1
type LabelDetector struct {
	client client.Client
	logger logr.Logger
}

// NewLabelDetector creates a new label detector.
func NewLabelDetector(c client.Client, logger logr.Logger) *LabelDetector {
	return &LabelDetector{
		client: c,
		logger: logger.WithName("detection"),
	}
}

// DetectLabels detects 7 label types from K8s context.
// Per DD-WORKFLOW-001 v2.1: Tracks QUERY FAILURES in FailedDetections field.
//
// IMPORTANT DISTINCTION:
// - Resource doesn't exist (PDB not found) → false (normal, NOT an error)
// - Can't query resource (RBAC denied, timeout) → false + FailedDetections + warn log
func (d *LabelDetector) DetectLabels(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext) *sharedtypes.DetectedLabels {
	if k8sCtx == nil {
		return nil
	}

	labels := &sharedtypes.DetectedLabels{}
	var failedDetections []string // Track QUERY failures only (DD-WORKFLOW-001 v2.1)

	// 1. GitOps detection (ArgoCD/Flux)
	tool, err := d.detectGitOpsTool(ctx, k8sCtx)
	if err != nil {
		d.logger.V(1).Info("Could not query for GitOps annotations", "error", err)
		failedDetections = append(failedDetections, "gitOpsManaged")
	} else {
		labels.GitOpsManaged = (tool != "")
		labels.GitOpsTool = tool // "" if not GitOps managed
	}

	// 2. PDB protection detection
	hasPDB, err := d.hasPDB(ctx, k8sCtx)
	if err != nil {
		d.logger.V(1).Info("Could not query PodDisruptionBudgets", "error", err)
		failedDetections = append(failedDetections, "pdbProtected")
	} else {
		labels.PDBProtected = hasPDB
	}

	// 3. HPA detection
	hasHPA, err := d.hasHPA(ctx, k8sCtx)
	if err != nil {
		d.logger.V(1).Info("Could not query HorizontalPodAutoscalers", "error", err)
		failedDetections = append(failedDetections, "hpaEnabled")
	} else {
		labels.HPAEnabled = hasHPA
	}

	// 4. StatefulSet detection
	isStateful, err := d.isStateful(ctx, k8sCtx)
	if err != nil {
		d.logger.V(1).Info("Could not determine if StatefulSet", "error", err)
		failedDetections = append(failedDetections, "stateful")
	} else {
		labels.Stateful = isStateful
	}

	// 5. Helm managed detection
	isHelm, err := d.isHelmManaged(ctx, k8sCtx)
	if err != nil {
		d.logger.V(1).Info("Could not check Helm labels", "error", err)
		failedDetections = append(failedDetections, "helmManaged")
	} else {
		labels.HelmManaged = isHelm
	}

	// 6. Network isolation detection
	hasNetPol, err := d.hasNetworkPolicy(ctx, k8sCtx)
	if err != nil {
		d.logger.V(1).Info("Could not query NetworkPolicies", "error", err)
		failedDetections = append(failedDetections, "networkIsolated")
	} else {
		labels.NetworkIsolated = hasNetPol
	}

	// 7. Service Mesh detection
	mesh, err := d.detectServiceMesh(ctx, k8sCtx)
	if err != nil {
		d.logger.V(1).Info("Could not detect service mesh", "error", err)
		failedDetections = append(failedDetections, "serviceMesh")
	} else {
		labels.ServiceMesh = mesh // "" is valid - just means no mesh
	}

	// Set FailedDetections only if we had QUERY failures (DD-WORKFLOW-001 v2.1)
	if len(failedDetections) > 0 {
		labels.FailedDetections = failedDetections
		d.logger.Info("Some label detections failed (RBAC or timeout)",
			"failedDetections", failedDetections)
	}

	return labels
}

// detectGitOpsTool checks for ArgoCD or Flux management.
// Returns ("argocd" | "flux" | "", nil) on success, ("", err) on RBAC/timeout.
func (d *LabelDetector) detectGitOpsTool(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext) (string, error) {
	if k8sCtx.DeploymentDetails == nil {
		return "", nil
	}

	// Fetch the deployment to check annotations
	deploy := &appsv1.Deployment{}
	if err := d.client.Get(ctx, types.NamespacedName{
		Namespace: k8sCtx.Namespace,
		Name:      k8sCtx.DeploymentDetails.Name,
	}, deploy); err != nil {
		// Not found is not an error - just means no GitOps
		return "", nil
	}

	// Check for ArgoCD: argocd.argoproj.io/instance annotation
	if _, ok := deploy.Annotations["argocd.argoproj.io/instance"]; ok {
		return "argocd", nil
	}

	// Check for Flux: fluxcd.io/sync-gc-mark label
	if _, ok := deploy.Labels["fluxcd.io/sync-gc-mark"]; ok {
		return "flux", nil
	}

	return "", nil
}

// hasPDB checks if a PodDisruptionBudget exists for the workload.
// Returns (true/false, nil) on success, (false, err) on RBAC/timeout.
func (d *LabelDetector) hasPDB(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext) (bool, error) {
	// List all PDBs in the namespace
	pdbList := &policyv1.PodDisruptionBudgetList{}
	if err := d.client.List(ctx, pdbList, client.InNamespace(k8sCtx.Namespace)); err != nil {
		return false, err
	}

	// Check if any PDB selector matches the deployment labels
	if k8sCtx.DeploymentDetails != nil && len(k8sCtx.DeploymentDetails.Labels) > 0 {
		for _, pdb := range pdbList.Items {
			if pdb.Spec.Selector != nil {
				if matchesLabels(pdb.Spec.Selector.MatchLabels, k8sCtx.DeploymentDetails.Labels) {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// hasHPA checks if a HorizontalPodAutoscaler targets the workload.
func (d *LabelDetector) hasHPA(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext) (bool, error) {
	if k8sCtx.DeploymentDetails == nil {
		return false, nil
	}

	// List all HPAs in the namespace
	hpaList := &autoscalingv2.HorizontalPodAutoscalerList{}
	if err := d.client.List(ctx, hpaList, client.InNamespace(k8sCtx.Namespace)); err != nil {
		return false, err
	}

	// Check if any HPA targets the deployment
	for _, hpa := range hpaList.Items {
		if hpa.Spec.ScaleTargetRef.Kind == "Deployment" &&
			hpa.Spec.ScaleTargetRef.Name == k8sCtx.DeploymentDetails.Name {
			return true, nil
		}
	}

	return false, nil
}

// isStateful checks if the pod is owned by a StatefulSet.
func (d *LabelDetector) isStateful(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext) (bool, error) {
	if k8sCtx.PodDetails == nil {
		return false, nil
	}

	// Fetch the pod to check ownerReferences
	pod := &corev1.Pod{}
	if err := d.client.Get(ctx, types.NamespacedName{
		Namespace: k8sCtx.Namespace,
		Name:      k8sCtx.PodDetails.Name,
	}, pod); err != nil {
		return false, nil // Not found is not an error
	}

	// Check if owned by StatefulSet
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "StatefulSet" {
			return true, nil
		}
	}

	return false, nil
}

// isHelmManaged checks if the workload is managed by Helm.
func (d *LabelDetector) isHelmManaged(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext) (bool, error) {
	if k8sCtx.DeploymentDetails == nil {
		return false, nil
	}

	// Check for Helm label from KubernetesContext first
	if k8sCtx.DeploymentDetails.Labels != nil {
		if k8sCtx.DeploymentDetails.Labels["app.kubernetes.io/managed-by"] == "Helm" {
			return true, nil
		}
	}

	// Fetch deployment to check labels and annotations on the actual resource
	deploy := &appsv1.Deployment{}
	if err := d.client.Get(ctx, types.NamespacedName{
		Namespace: k8sCtx.Namespace,
		Name:      k8sCtx.DeploymentDetails.Name,
	}, deploy); err != nil {
		return false, nil // Not found is not an error
	}

	// Check app.kubernetes.io/managed-by label on deployment
	if deploy.Labels != nil {
		if deploy.Labels["app.kubernetes.io/managed-by"] == "Helm" {
			return true, nil
		}
	}

	// Check helm.sh/chart annotation
	if _, ok := deploy.Annotations["helm.sh/chart"]; ok {
		return true, nil
	}

	return false, nil
}

// hasNetworkPolicy checks if any NetworkPolicy exists in the namespace.
func (d *LabelDetector) hasNetworkPolicy(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext) (bool, error) {
	// List all NetworkPolicies in the namespace
	netpolList := &networkingv1.NetworkPolicyList{}
	if err := d.client.List(ctx, netpolList, client.InNamespace(k8sCtx.Namespace)); err != nil {
		return false, err
	}

	return len(netpolList.Items) > 0, nil
}

// detectServiceMesh checks for Istio or Linkerd service mesh.
// Returns ("istio" | "linkerd" | "", nil) on success.
func (d *LabelDetector) detectServiceMesh(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext) (string, error) {
	if k8sCtx.PodDetails == nil {
		return "", nil
	}

	// Check annotations from KubernetesContext
	if k8sCtx.PodDetails.Annotations != nil {
		// Check for Istio sidecar
		if _, ok := k8sCtx.PodDetails.Annotations["sidecar.istio.io/status"]; ok {
			return "istio", nil
		}

		// Check for Linkerd proxy
		if _, ok := k8sCtx.PodDetails.Annotations["linkerd.io/proxy-version"]; ok {
			return "linkerd", nil
		}
	}

	return "", nil
}

// matchesLabels checks if all selector labels are present in the target labels.
func matchesLabels(selector, target map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for key, value := range selector {
		if target[key] != value {
			return false
		}
	}
	return true
}

