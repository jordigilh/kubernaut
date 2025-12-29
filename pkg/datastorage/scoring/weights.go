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

package scoring

// ========================================
// V1.5 FIXED LABEL WEIGHTS
// ========================================
// Authority: DD-WORKFLOW-004 v1.5 (Fixed Weights for DetectedLabels)
// Business Requirement: BR-STORAGE-013 (Context-Aware Workflow Selection)
//
// Design Decision: Fixed weights for auto-detected labels in V1.5
//   - Weights are HARD-CODED (not configurable)
//   - Based on "correctness impact" (GitOps conflict = workflow failure)
//   - Customer configuration deferred to V2.0+
//
// Rationale:
//   - DetectedLabels are universal (GitOps, PDBs exist in all K8s clusters)
//   - Fixed weights prevent configuration complexity in V1.0/V1.5
//   - Customers can request configurable weights in V2.0 after validation
//
// See: docs/architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-label-scoring.md
// ========================================

// DetectedLabelWeights defines fixed boost weights for auto-detected labels.
// These weights represent the importance of context matching for workflow selection.
//
// Weight Categories:
//   - HIGH (0.10): Critical context that affects workflow correctness
//   - MEDIUM (0.05): Important context that affects workflow safety/reliability
//   - LOW (0.02-0.03): Informational context that may optimize workflow behavior
//
// Authority: DD-WORKFLOW-004 v1.5
var DetectedLabelWeights = map[string]float64{
	// ========================================
	// HIGH-IMPACT CONTEXT (0.10)
	// ========================================
	// Critical: Workflow will FAIL if context doesn't match

	// GitOpsManaged: Whether resource is managed by GitOps (ArgoCD/Flux)
	// Impact: Manual kubectl changes will be reverted by GitOps → workflow FAILS
	// Example: kubectl patch in GitOps-managed cluster = failure
	"gitOpsManaged": 0.10,

	// GitOpsTool: Specific GitOps tool (argocd, flux)
	// Impact: Tool-specific workflows use different APIs/approaches
	// Example: ArgoCD PR workflow vs. Flux commit workflow
	"gitOpsTool": 0.10,

	// ========================================
	// MEDIUM-IMPACT CONTEXT (0.05)
	// ========================================
	// Important: Affects workflow safety, reliability, or performance

	// PDBProtected: Whether PodDisruptionBudget protects this workload
	// Impact: Workflows must respect PDB constraints to avoid service disruption
	// Example: Drain node workflow must honor PDB min-available
	"pdbProtected": 0.05,

	// ServiceMesh: Service mesh deployment (istio, linkerd)
	// Impact: Workflows must be mesh-aware for correct traffic management
	// Example: Restart workflow must drain Envoy connections first
	"serviceMesh": 0.05,

	// ========================================
	// LOW-IMPACT CONTEXT (0.02-0.03)
	// ========================================
	// Informational: May optimize workflow behavior but not critical

	// NetworkIsolated: Whether NetworkPolicy restricts traffic
	// Impact: Workflows may need to verify connectivity after changes
	// Example: Scale workflow validates service mesh connectivity
	"networkIsolated": 0.03,

	// HelmManaged: Whether workload is Helm-managed
	// Impact: Workflows may prefer Helm operations over direct kubectl
	// Example: Upgrade workflow uses `helm upgrade` instead of `kubectl apply`
	"helmManaged": 0.02,

	// Stateful: Whether workload has persistent state (StatefulSet, PVCs)
	// Impact: Workflows must preserve data during operations
	// Example: Restart workflow waits for volume reattachment
	"stateful": 0.02,

	// HPAEnabled: Whether HorizontalPodAutoscaler manages replicas
	// Impact: Workflows must coordinate with HPA to avoid conflicts
	// Example: Manual scale workflow temporarily disables HPA
	"hpaEnabled": 0.02,
}

// MaxLabelBoost is the maximum possible boost from DetectedLabel matching.
// Sum of all DetectedLabel weights: 0.10 + 0.10 + 0.05 + 0.05 + 0.03 + 0.02 + 0.02 + 0.02 = 0.39
const MaxLabelBoost = 0.39

// MaxLabelPenalty is the maximum possible penalty from DetectedLabel conflicts.
// Only high-impact labels cause penalties (gitOpsManaged, gitOpsTool): 0.10 + 0.10 = 0.20
const MaxLabelPenalty = 0.20

// GetDetectedLabelWeight returns the boost weight for a given DetectedLabel field.
// Returns 0.0 if the field is not recognized or has no weight assigned.
func GetDetectedLabelWeight(fieldName string) float64 {
	weight, exists := DetectedLabelWeights[fieldName]
	if !exists {
		return 0.0
	}
	return weight
}

// ShouldApplyPenalty determines if a DetectedLabel mismatch should apply a penalty.
// Only high-impact labels (gitOpsManaged, gitOpsTool) apply penalties because
// mismatches indicate workflow incompatibility that will cause failures.
//
// Authority: DD-WORKFLOW-004 v1.5
func ShouldApplyPenalty(fieldName string) bool {
	highImpactFields := map[string]bool{
		"gitOpsManaged": true,
		"gitOpsTool":    true,
	}
	return highImpactFields[fieldName]
}
