// Package detection provides auto-detection of cluster characteristics for Signal Processing.
//
// # Purpose
//
// The LabelDetector automatically discovers K8s resource characteristics
// without any user configuration. These detected labels are used by
// HolmesGPT-API for:
//   - Workflow filtering (deterministic SQL WHERE clauses)
//   - LLM context enrichment (natural language in prompts)
//
// # Business Requirement
//
// BR-SP-101: DetectedLabels Auto-Detection
//
// # Design Decision
//
// DD-WORKFLOW-001 v2.2: 7 auto-detected cluster characteristics.
// PodSecurityLevel was removed as it's unreliable at pod level.
//
// # Detection Types (V1.0)
//
// 1. GitOpsManaged + GitOpsTool:
//   - ArgoCD: argocd.argoproj.io/instance annotation
//   - Flux: fluxcd.io/sync-gc-mark label
//
// 2. PDBProtected:
//   - PodDisruptionBudget exists with matching selector
//
// 3. HPAEnabled:
//   - HorizontalPodAutoscaler targets the deployment
//
// 4. Stateful:
//   - Pod owned by StatefulSet
//
// 5. HelmManaged:
//   - app.kubernetes.io/managed-by=Helm label
//   - helm.sh/chart annotation
//
// 6. NetworkIsolated:
//   - NetworkPolicy exists in namespace
//
// 7. ServiceMesh:
//   - Istio: sidecar.istio.io/status annotation
//   - Linkerd: linkerd.io/proxy-version annotation
//
// # Error Handling (DD-WORKFLOW-001 v2.1)
//
// IMPORTANT DISTINCTION in failure handling:
//
//   - Resource not found (no PDB exists) → false value, NOT in FailedDetections
//   - Query failed (RBAC denied, timeout) → false value, field name IN FailedDetections
//
// The FailedDetections array tracks which detections had QUERY failures.
// Consumers should check FailedDetections before trusting a false value.
//
// # Usage
//
//	detector := detection.NewLabelDetector(k8sClient, logger)
//	labels := detector.DetectLabels(ctx, kubernetesContext)
//
//	// Check for query failures
//	if len(labels.FailedDetections) > 0 {
//	    log.Warn("Some detections failed", "failed", labels.FailedDetections)
//	}
//
//	// Use detected values
//	if labels.GitOpsManaged {
//	    // Workflow can assume GitOps sync will happen
//	}
//
// # Thread Safety
//
// The LabelDetector is safe for concurrent use. Each DetectLabels() call
// is independent and uses its own K8s queries.
package detection

