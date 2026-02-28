# AIAnalysis Rego Policy Examples

**Status**: 🟡 DRAFT - For Design Discussion
**Version**: 1.5
**Date**: 2025-12-05
**Purpose**: Explore approval policy input/output schemas with sample policies
**Rego Syntax**: OPA v1 (`if` keyword, `:=` operator)

---

## ⚠️ OPA v1 Syntax

All policies in this document use OPA v1 syntax (required for `github.com/open-policy-agent/opa/v1/rego`):
- Default values use `:=` operator: `default x := false`
- All rules use `if` keyword: `rule if { condition }`
- See: [DD-AIANALYSIS-001](../../../architecture/decisions/DD-AIANALYSIS-001-rego-policy-loading-strategy.md)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| **1.5** | 2025-12-05 | Added OPA v1 syntax header; All policies verified to use `if` keyword and `:=` operator |
| **1.4** | 2025-12-02 | Added `failed_detections` to input schema per DD-WORKFLOW-001 v2.1; Added detection failure handling policies; Key distinction: "Resource doesn't exist" ≠ detection failure |
| 1.3 | 2025-12-02 | Added `target_in_owner_chain` and `warnings` to input schema (from HolmesGPT-API); Added data quality rules for label accuracy |
| 1.2 | 2025-11-30 | Aligned with DD-WORKFLOW-001 v1.8: snake_case for all API fields; Updated input schema to use `git_ops_managed`, `pdb_protected`, etc. |
| 1.1 | 2025-11-30 | Added `DetectedLabels` and `CustomLabels` to input schema; Added GitOps/constraint-aware policies |
| 1.0 | 2025-11-29 | Initial draft with sample policies |

---

## Policy Input Schema

> **Note**: Per DD-WORKFLOW-001 v1.8, all API field names use **snake_case**.
> CRD fields (K8s resources) remain camelCase per K8s convention.

```go
// ApprovalPolicyInput is passed to Rego policy for evaluation
// All field names use snake_case per DD-WORKFLOW-001 v1.8
type ApprovalPolicyInput struct {
    // From AIAnalysis.status.selectedWorkflow
    Action     string  `json:"action"`     // e.g., "restart-pod", "scale-deployment"
    Confidence float64 `json:"confidence"` // 0.0-1.0 from HolmesGPT

    // From SignalProcessing context (5 mandatory labels)
    Environment      string `json:"environment"`       // production, staging, dev
    Severity         string `json:"severity"`          // critical, warning, info
    BusinessPriority string `json:"business_priority"` // P0, P1, P2

    // Target resource
    TargetResource TargetResourceInput `json:"target_resource"`

    // Timestamp for business hours calculation
    Timestamp string `json:"timestamp"` // ISO 8601

    // ========================================
    // CLUSTER CONTEXT (v1.1)
    // From SignalProcessing EnrichmentResults
    // ========================================

    // DetectedLabels: Auto-detected cluster characteristics
    // SignalProcessing populates these automatically - no config needed
    DetectedLabels DetectedLabelsInput `json:"detected_labels"`

    // CustomLabels: Customer-defined labels via Rego policies
    // Key = subdomain (e.g., "constraint", "team", "region")
    // Value = list of label values
    // Example: {"constraint": ["cost-constrained"], "team": ["name=payments"]}
    CustomLabels map[string][]string `json:"custom_labels"`

    // ========================================
    // DATA QUALITY INDICATORS (Dec 2025)
    // From HolmesGPT-API response
    // ========================================

    // Whether RCA-identified target resource was found in OwnerChain
    // If false, DetectedLabels may be from different scope than affected resource
    // Use for stricter approval policies when label accuracy is uncertain
    TargetInOwnerChain bool `json:"target_in_owner_chain"`

    // Warnings from HolmesGPT-API (e.g., "Low confidence selection", "OwnerChain mismatch")
    Warnings []string `json:"warnings"`
}

type TargetResourceInput struct {
    Kind      string `json:"kind"`      // Pod, Deployment, StatefulSet
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

// DetectedLabelsInput contains auto-detected cluster characteristics
// Field names use snake_case per DD-WORKFLOW-001 v2.1
type DetectedLabelsInput struct {
    // Detection Metadata (DD-WORKFLOW-001 v2.1)
    // Lists fields where detection failed (RBAC, timeout, etc.)
    // If a field is in this array, ignore its value in policy decisions
    // Valid values: git_ops_managed, pdb_protected, hpa_enabled, stateful,
    //               helm_managed, network_isolated, pod_security_level, service_mesh
    FailedDetections []string `json:"failed_detections"`

    // GitOps
    GitOpsManaged bool   `json:"git_ops_managed"` // ArgoCD/Flux detected
    GitOpsTool    string `json:"git_ops_tool"`    // "argocd", "flux", ""

    // Workload Protection
    PDBProtected bool `json:"pdb_protected"` // PodDisruptionBudget exists
    HPAEnabled   bool `json:"hpa_enabled"`   // HorizontalPodAutoscaler targets workload

    // Workload Characteristics
    Stateful    bool `json:"stateful"`     // StatefulSet or PVCs
    HelmManaged bool `json:"helm_managed"` // helm.sh/chart label

    // Security
    NetworkIsolated  bool   `json:"network_isolated"`   // NetworkPolicy exists
    PodSecurityLevel string `json:"pod_security_level"` // "privileged", "baseline", "restricted"
    ServiceMesh      string `json:"service_mesh"`       // "istio", "linkerd", ""
}
```

> **Detection Failure Semantics (DD-WORKFLOW-001 v2.1)**:
> | Scenario | `pdb_protected` | `failed_detections` | Policy Implication |
> |----------|-----------------|---------------------|-------------------|
> | PDB exists | `true` | `[]` | ✅ Trust value |
> | No PDB | `false` | `[]` | ✅ Trust value |
> | RBAC denied | `false` | `["pdb_protected"]` | ⚠️ Ignore field |
>
> Key: "Resource doesn't exist" ≠ detection failure (successful detection with result `false`)

---

## Policy Output Schema

```go
// ApprovalDecision is returned by Rego policy
type ApprovalDecision struct {
    RequireApproval bool   `json:"require_approval"` // true = notify and stop (V1.0)
    AutoApprove     bool   `json:"auto_approve"`     // true = proceed without approval
    Reason          string `json:"reason"`           // Human-readable explanation
}
```

---

## Sample Policies

### 1. Production Safety Policy (`production.rego`)

```rego
package kubernaut.aianalysis.approval.production

import future.keywords.if

# Default: Require approval in production
default require_approval := true
default auto_approve := false

# ========================================
# AUTO-APPROVE RULES (Safe Actions)
# ========================================

# Auto-approve safe scaling actions with high confidence
auto_approve if {
    input.environment == "production"
    is_safe_scaling_action
    input.confidence >= 0.90
}

# Auto-approve diagnostic actions (no state change)
auto_approve if {
    input.environment == "production"
    is_diagnostic_action
}

# ========================================
# REQUIRE APPROVAL RULES (Risky Actions)
# ========================================

# Always require approval for destructive actions
require_approval if {
    input.environment == "production"
    is_destructive_action
}

# Require approval for low confidence (< 80%)
require_approval if {
    input.environment == "production"
    input.confidence < 0.80
}

# Require approval for critical severity + risky action
require_approval if {
    input.environment == "production"
    input.severity == "critical"
    is_risky_action
}

# ========================================
# ACTION CLASSIFICATIONS
# ========================================

is_safe_scaling_action if {
    input.action in [
        "increase-memory-limit",
        "increase-cpu-limit",
        "scale-up-replicas",
        "adjust-hpa-max",
    ]
}

is_diagnostic_action if {
    input.action in [
        "collect-diagnostics",
        "capture-heap-dump",
        "collect-logs",
        "run-health-check",
    ]
}

is_destructive_action if {
    input.action in [
        "delete-pod",
        "delete-deployment",
        "drain-node",
        "delete-pvc",
        "delete-namespace",
    ]
}

is_risky_action if {
    input.action in [
        "restart-pod",
        "restart-deployment",
        "rollback-deployment",
        "cordon-node",
        "scale-down-replicas",
    ]
}

# ========================================
# OUTPUT
# ========================================

reason := sprintf("Production safety: %s action with %.0f%% confidence", [input.action, input.confidence * 100]) if {
    require_approval
}

reason := sprintf("Auto-approved: safe %s action with %.0f%% confidence", [input.action, input.confidence * 100]) if {
    auto_approve
}
```

---

### 2. GitOps & Constraint-Aware Policy (`gitops_constraints.rego`) - v1.2

**Purpose**: Demonstrates approval decisions based on detected_labels and custom_labels.

> **Note**: Field names use snake_case per DD-WORKFLOW-001 v1.8

```rego
package kubernaut.aianalysis.approval.gitops_constraints

import future.keywords.if
import future.keywords.in

# Default: Require approval
default require_approval := true
default auto_approve := false

# ========================================
# GITOPS-AWARE RULES
# ========================================

# GitOps-managed namespaces: Only allow GitOps-compatible actions
require_approval if {
    input.detected_labels.git_ops_managed
    not is_gitops_compatible_action
}

# Auto-approve GitOps-compatible actions in GitOps namespaces
auto_approve if {
    input.detected_labels.git_ops_managed
    is_gitops_compatible_action
    input.confidence >= 0.85
}

is_gitops_compatible_action if {
    # Actions that don't conflict with GitOps sync
    input.action in [
        "collect-diagnostics",
        "capture-heap-dump",
        "collect-logs",
        "run-health-check",
        # Scaling is OK - ArgoCD/Flux won't override HPA decisions
        "scale-up-replicas",
        "adjust-hpa-max",
    ]
}

# ========================================
# PDB-AWARE RULES
# ========================================

# PDB-protected workloads: Be more conservative
require_approval if {
    input.detected_labels.pdb_protected
    is_pod_disruptive_action
}

is_pod_disruptive_action if {
    input.action in [
        "restart-pod",
        "restart-deployment",
        "delete-pod",
        "drain-node",
    ]
}

# ========================================
# STATEFUL WORKLOAD RULES
# ========================================

# Stateful workloads: Extra caution required
require_approval if {
    input.detected_labels.stateful
    is_data_risk_action
}

is_data_risk_action if {
    input.action in [
        "delete-pod",
        "delete-pvc",
        "scale-down-replicas",
        "restart-deployment",
    ]
}

# ========================================
# CUSTOM CONSTRAINT RULES
# ========================================

# Cost-constrained namespaces: Block resource increases
require_approval if {
    has_constraint("cost-constrained")
    is_resource_increase_action
}

is_resource_increase_action if {
    input.action in [
        "increase-memory-limit",
        "increase-cpu-limit",
        "scale-up-replicas",
        "adjust-hpa-max",
    ]
}

# High-availability constraint: Block single-replica scaling
require_approval if {
    has_constraint("high-availability")
    input.action == "scale-down-replicas"
}

# ========================================
# CUSTOM LABEL HELPERS
# ========================================

# Check if a constraint exists in custom_labels
has_constraint(name) if {
    constraints := input.custom_labels["constraint"]
    name in constraints
}

# Get team name from custom_labels
get_team := team if {
    teams := input.custom_labels["team"]
    some t in teams
    startswith(t, "name=")
    team := substring(t, 5, -1)
}

# ========================================
# RISK TOLERANCE RULES (from custom_labels)
# ========================================

# Low risk tolerance: Require approval for all risky actions
require_approval if {
    has_risk_tolerance("low")
    is_risky_action
}

# High risk tolerance: Auto-approve more actions
auto_approve if {
    has_risk_tolerance("high")
    input.confidence >= 0.70
    not is_always_require_approval
}

has_risk_tolerance(level) if {
    tolerances := input.custom_labels["risk"]
    some t in tolerances
    t == sprintf("tolerance=%s", [level])
}

is_risky_action if {
    input.action in [
        "restart-pod",
        "restart-deployment",
        "rollback-deployment",
        "scale-down-replicas",
    ]
}

is_always_require_approval if {
    input.action in ["delete-namespace", "drain-node", "delete-pvc"]
}

# ========================================
# DATA QUALITY RULES (Dec 2025)
# Based on target_in_owner_chain from HolmesGPT-API
# ========================================

# If RCA target is NOT in OwnerChain, DetectedLabels may not be accurate
# Require approval for production + label mismatch
require_approval if {
    input.environment == "production"
    not input.target_in_owner_chain
}

# Auto-approve can proceed in non-prod even if labels might not match
auto_approve if {
    input.environment != "production"
    not input.target_in_owner_chain
    input.confidence >= 0.85
}

# If there are warnings, always require approval for risky actions
require_approval if {
    count(input.warnings) > 0
    is_risky_action
}

# ========================================
# OUTPUT
# ========================================

reason := sprintf("GitOps namespace (%s): %s action requires manual sync", [input.detected_labels.git_ops_tool, input.action]) if {
    require_approval
    input.detected_labels.git_ops_managed
    not is_gitops_compatible_action
}

reason := sprintf("PDB-protected workload: %s action requires approval", [input.action]) if {
    require_approval
    input.detected_labels.pdb_protected
    is_pod_disruptive_action
}

reason := sprintf("Cost-constrained namespace: %s action blocked", [input.action]) if {
    require_approval
    has_constraint("cost-constrained")
    is_resource_increase_action
}

reason := sprintf("Auto-approved: %s action (high risk tolerance)", [input.action]) if {
    auto_approve
    has_risk_tolerance("high")
}

reason := "Requires approval (default)" if {
    require_approval
    not input.detected_labels.git_ops_managed
    not input.detected_labels.pdb_protected
    not has_constraint("cost-constrained")
}
```

---

### 2b. Failed Detection Handling Policy (`failed_detections.rego`) - v1.4

**Purpose**: Demonstrates handling of detection failures per DD-WORKFLOW-001 v2.1.

> **Key Principle**: When a field is in `failed_detections`, its boolean value should be **ignored** in policy decisions. The detection failed (RBAC, timeout, etc.), so the value is unreliable.

```rego
package kubernaut.aianalysis.approval.failed_detections

import future.keywords.if
import future.keywords.in

# Default: Require approval if any critical detection failed
default require_approval := true
default auto_approve := false

# ========================================
# DETECTION FAILURE HELPERS
# ========================================

# Check if a specific detection failed
detection_failed(field) if {
    field in input.detected_labels.failed_detections
}

# Check if any detection failed
has_any_failed_detection if {
    count(input.detected_labels.failed_detections) > 0
}

# ========================================
# SAFE HANDLING OF FAILED DETECTIONS
# ========================================

# If PDB detection failed, require approval for any pod-disruptive action
# (we don't know if there's a PDB or not)
require_approval if {
    detection_failed("pdb_protected")
    is_pod_disruptive_action
}

# If GitOps detection failed, require approval for any state-changing action
# (we don't know if GitOps sync will revert our changes)
require_approval if {
    detection_failed("git_ops_managed")
    is_state_changing_action
}

# If stateful detection failed, require approval for data-risk actions
# (we don't know if there are PVCs attached)
require_approval if {
    detection_failed("stateful")
    is_data_risk_action
}

# ========================================
# TRUSTING SUCCESSFUL DETECTIONS
# ========================================

# If PDB detection succeeded and says no PDB exists, allow pod disruption
auto_approve if {
    not detection_failed("pdb_protected")
    not input.detected_labels.pdb_protected
    is_pod_disruptive_action
    input.confidence >= 0.85
}

# If GitOps detection succeeded and says not GitOps-managed, allow direct changes
auto_approve if {
    not detection_failed("git_ops_managed")
    not input.detected_labels.git_ops_managed
    is_state_changing_action
    input.confidence >= 0.85
}

# ========================================
# ACTION CLASSIFICATIONS
# ========================================

is_pod_disruptive_action if {
    input.action in ["restart-pod", "delete-pod", "restart-deployment", "drain-node"]
}

is_state_changing_action if {
    input.action in ["scale-up-replicas", "scale-down-replicas", "rollback-deployment"]
}

is_data_risk_action if {
    input.action in ["delete-pod", "delete-pvc", "scale-down-replicas"]
}

# ========================================
# OUTPUT
# ========================================

reason := sprintf("Detection failed for '%s': requiring approval for %s (cannot verify safety)",
    [input.detected_labels.failed_detections[0], input.action]) if {
    require_approval
    has_any_failed_detection
}

reason := sprintf("All detections succeeded: auto-approving %s", [input.action]) if {
    auto_approve
    not has_any_failed_detection
}
```

---

### 3. Development/Staging Policy (`development.rego`)

```rego
package kubernaut.aianalysis.approval.development

import future.keywords.if

# Default: Auto-approve most actions in dev/staging
default require_approval := false
default auto_approve := true

# ========================================
# REQUIRE APPROVAL RULES (Even in Dev)
# ========================================

# Require approval for truly destructive actions
require_approval if {
    input.action in ["delete-namespace", "delete-pvc", "drain-node"]
}

# Require approval for very low confidence (< 50%)
require_approval if {
    input.confidence < 0.50
}

# ========================================
# OUTPUT
# ========================================

reason := sprintf("Dev/staging: %s action requires approval (destructive or low confidence)", [input.action]) if {
    require_approval
}

reason := sprintf("Dev/staging: auto-approved %s action", [input.action]) if {
    auto_approve
}
```

---

### 3. Confidence-Based Policy (`confidence.rego`)

```rego
package kubernaut.aianalysis.approval.confidence

import future.keywords.if

# Confidence thresholds — defaults that operators can override via input.confidence_threshold (#225)
# The high_confidence_threshold is configurable: if input.confidence_threshold is set by the
# controller config (rego.confidenceThreshold), it overrides the Rego default.
default high_confidence_threshold := 0.90

high_confidence_threshold := input.confidence_threshold if {
    input.confidence_threshold
}

medium_confidence_threshold := 0.70
low_confidence_threshold := 0.50

# Default based on confidence
default require_approval := true
default auto_approve := false

# ========================================
# HIGH CONFIDENCE (>= 90%)
# ========================================

auto_approve if {
    input.confidence >= high_confidence_threshold
    not is_always_require_approval
}

# ========================================
# MEDIUM CONFIDENCE (70-89%)
# ========================================

# Medium confidence + safe action = auto-approve
auto_approve if {
    input.confidence >= medium_confidence_threshold
    input.confidence < high_confidence_threshold
    is_safe_action
}

# Medium confidence + risky action = require approval
require_approval if {
    input.confidence >= medium_confidence_threshold
    input.confidence < high_confidence_threshold
    not is_safe_action
}

# ========================================
# LOW CONFIDENCE (< 70%)
# ========================================

require_approval if {
    input.confidence < medium_confidence_threshold
}

# ========================================
# ALWAYS REQUIRE APPROVAL
# ========================================

is_always_require_approval if {
    input.action in ["delete-namespace", "drain-node", "delete-pvc"]
}

require_approval if {
    is_always_require_approval
}

# ========================================
# ACTION SAFETY CLASSIFICATION
# ========================================

is_safe_action if {
    input.action in [
        "increase-memory-limit",
        "increase-cpu-limit",
        "scale-up-replicas",
        "collect-diagnostics",
    ]
}

# ========================================
# OUTPUT
# ========================================

reason := sprintf("Confidence %.0f%%: %s", [input.confidence * 100, action_reason]) if {
    true
}

action_reason := "auto-approved (high confidence)" if {
    auto_approve
    input.confidence >= high_confidence_threshold
}

action_reason := "auto-approved (medium confidence + safe action)" if {
    auto_approve
    input.confidence >= medium_confidence_threshold
    input.confidence < high_confidence_threshold
}

action_reason := "requires approval (medium confidence + risky action)" if {
    require_approval
    input.confidence >= medium_confidence_threshold
    input.confidence < high_confidence_threshold
}

action_reason := "requires approval (low confidence)" if {
    require_approval
    input.confidence < medium_confidence_threshold
}

action_reason := "requires approval (always-require action)" if {
    require_approval
    is_always_require_approval
}
```

---

### 4. Business Hours Policy (`business_hours.rego`)

```rego
package kubernaut.aianalysis.approval.business_hours

import future.keywords.if

# Default
default require_approval := true
default auto_approve := false

# ========================================
# BUSINESS HOURS DETECTION
# ========================================

is_business_hours if {
    time_obj := time.parse_rfc3339_ns(input.timestamp)
    weekday := time.weekday(time_obj)
    not weekday in ["Saturday", "Sunday"]
    hour := time.clock(time_obj)[0]
    hour >= 9
    hour < 17
}

# ========================================
# BUSINESS HOURS RULES
# ========================================

# During business hours: auto-approve safe actions
auto_approve if {
    is_business_hours
    is_safe_action
    input.confidence >= 0.80
}

# During business hours: require approval for risky actions
require_approval if {
    is_business_hours
    not is_safe_action
}

# ========================================
# AFTER HOURS RULES (More Conservative)
# ========================================

# After hours: require approval for everything except diagnostics
require_approval if {
    not is_business_hours
    not is_diagnostic_action
}

# After hours: auto-approve only diagnostics
auto_approve if {
    not is_business_hours
    is_diagnostic_action
}

# ========================================
# ACTION CLASSIFICATIONS
# ========================================

is_safe_action if {
    input.action in [
        "increase-memory-limit",
        "increase-cpu-limit",
        "scale-up-replicas",
    ]
}

is_diagnostic_action if {
    input.action in [
        "collect-diagnostics",
        "capture-heap-dump",
        "collect-logs",
    ]
}

# ========================================
# OUTPUT
# ========================================

reason := sprintf("%s: %s action", [time_context, input.action]) if {
    true
}

time_context := "Business hours" if { is_business_hours }
time_context := "After hours" if { not is_business_hours }
```

---

## Edge Cases to Test

### Basic Edge Cases (v1.0)

| ID | Scenario | Input | Expected Output |
|----|----------|-------|-----------------|
| EC-REGO-01 | High confidence + safe action + production | `{confidence: 0.95, action: "increase-memory-limit", environment: "production"}` | `auto_approve: true` |
| EC-REGO-02 | High confidence + destructive action + production | `{confidence: 0.95, action: "delete-pod", environment: "production"}` | `require_approval: true` |
| EC-REGO-03 | Low confidence + safe action + production | `{confidence: 0.65, action: "increase-memory-limit", environment: "production"}` | `require_approval: true` |
| EC-REGO-04 | Medium confidence + safe action + staging | `{confidence: 0.75, action: "restart-pod", environment: "staging"}` | `auto_approve: true` |
| EC-REGO-05 | Any confidence + destructive action + dev | `{confidence: 0.99, action: "delete-namespace", environment: "dev"}` | `require_approval: true` |
| EC-REGO-06 | High confidence + safe action + after hours | `{confidence: 0.95, action: "scale-up-replicas", timestamp: "2025-01-15T22:00:00Z"}` | `require_approval: true` |
| EC-REGO-07 | Missing environment field | `{confidence: 0.85, action: "restart-pod"}` | Default to `require_approval: true` |
| EC-REGO-08 | Invalid action type | `{confidence: 0.85, action: "unknown-action", environment: "production"}` | Default to `require_approval: true` |

### DetectedLabels Edge Cases (v1.2 - snake_case)

| ID | Scenario | Input | Expected Output |
|----|----------|-------|-----------------|
| EC-DL-01 | GitOps namespace + non-compatible action | `{detected_labels: {git_ops_managed: true, git_ops_tool: "argocd"}, action: "restart-deployment"}` | `require_approval: true` (manual sync needed) |
| EC-DL-02 | GitOps namespace + compatible action | `{detected_labels: {git_ops_managed: true}, action: "collect-diagnostics", confidence: 0.90}` | `auto_approve: true` |
| EC-DL-03 | PDB-protected + pod disruptive action | `{detected_labels: {pdb_protected: true}, action: "restart-pod"}` | `require_approval: true` |
| EC-DL-04 | Stateful workload + delete action | `{detected_labels: {stateful: true}, action: "delete-pod"}` | `require_approval: true` |
| EC-DL-05 | Service mesh + restart action | `{detected_labels: {service_mesh: "istio"}, action: "restart-deployment"}` | Consider sidecar injection delay |
| EC-DL-06 | Empty detected_labels | `{detected_labels: {}}` | Use default rules only |
| EC-DL-07 | Wildcard match (v1.6) | `{detected_labels: {git_ops_tool: "argocd"}}` matches workflow `{git_ops_tool: "*"}` | Workflow requires SOME GitOps tool |

### CustomLabels Edge Cases (v1.2 - snake_case)

| ID | Scenario | Input | Expected Output |
|----|----------|-------|-----------------|
| EC-CL-01 | Cost-constrained + scale-up | `{custom_labels: {"constraint": ["cost-constrained"]}, action: "scale-up-replicas"}` | `require_approval: true` |
| EC-CL-02 | High-availability + scale-down | `{custom_labels: {"constraint": ["high-availability"]}, action: "scale-down-replicas"}` | `require_approval: true` |
| EC-CL-03 | Low risk tolerance + risky action | `{custom_labels: {"risk": ["tolerance=low"]}, action: "restart-pod"}` | `require_approval: true` |
| EC-CL-04 | High risk tolerance + risky action | `{custom_labels: {"risk": ["tolerance=high"]}, action: "restart-pod", confidence: 0.75}` | `auto_approve: true` |
| EC-CL-05 | Multiple constraints | `{custom_labels: {"constraint": ["cost-constrained", "stateful-safe"]}, action: "increase-memory-limit"}` | `require_approval: true` (any constraint blocks) |
| EC-CL-06 | Empty custom_labels | `{custom_labels: {}}` | Use default rules only |
| EC-CL-07 | Team-specific rules (future) | `{custom_labels: {"team": ["name=payments"]}, action: "restart-pod"}` | Team-specific policy (V2.0) |

### Combined detected_labels + custom_labels Edge Cases

| ID | Scenario | Input | Expected Output |
|----|----------|-------|-----------------|
| EC-COMBO-01 | GitOps + cost-constrained | `{detected_labels: {git_ops_managed: true}, custom_labels: {"constraint": ["cost-constrained"]}, action: "scale-up-replicas"}` | `require_approval: true` (both block) |
| EC-COMBO-02 | PDB + high risk tolerance | `{detected_labels: {pdb_protected: true}, custom_labels: {"risk": ["tolerance=high"]}, action: "restart-pod"}` | `require_approval: true` (PDB takes precedence) |
| EC-COMBO-03 | No labels (empty) | `{detected_labels: {}, custom_labels: {}}` | Default rules only |

### Data Quality Edge Cases (v1.3 - Dec 2025)

| ID | Scenario | Input | Expected Output |
|----|----------|-------|-----------------|
| EC-DQ-01 | Production + OwnerChain mismatch | `{environment: "production", target_in_owner_chain: false, action: "restart-pod"}` | `require_approval: true` (labels may not apply) |
| EC-DQ-02 | Non-production + OwnerChain mismatch | `{environment: "staging", target_in_owner_chain: false, confidence: 0.90}` | `auto_approve: true` (non-prod is permissive) |
| EC-DQ-03 | Warnings + risky action | `{warnings: ["Low confidence selection"], action: "restart-deployment"}` | `require_approval: true` |
| EC-DQ-04 | Warnings + safe action | `{warnings: ["OwnerChain incomplete"], action: "collect-diagnostics"}` | `auto_approve: true` (safe action) |
| EC-DQ-05 | OwnerChain OK + no warnings | `{target_in_owner_chain: true, warnings: [], confidence: 0.85}` | Follow standard rules |
| EC-DQ-06 | OwnerChain mismatch + warnings + production | `{environment: "production", target_in_owner_chain: false, warnings: ["DetectedLabels may not apply"]}` | `require_approval: true` (multiple indicators) |

---

## Policy Selection Strategy

For V1.0, we'll use a **single combined policy** that incorporates:
1. Environment-based rules
2. Confidence-based rules
3. Action classification
4. `detected_labels`-based rules (GitOps, PDB, Stateful, etc.)
5. `custom_labels`-based rules (constraints, risk tolerance, team)

**V1.6 Updates**:
- All API field names use **snake_case** (per DD-WORKFLOW-001 v1.8)
- `detected_labels` supports **wildcard matching** (`"*"` = requires SOME value)
- `custom_labels` auto-appended by HolmesGPT-API (per DD-HAPI-001)

Future V2.0+ can support:
- Multiple policy files per environment
- Policy composition/inheritance
- Customer-defined policies via ConfigMap
- Team-specific policies

---

## ConfigMap Structure (V1.0)

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ai-approval-policies
  namespace: kubernaut-system
data:
  policy.rego: |
    # Combined policy for V1.0
    package kubernaut.aianalysis.approval

    # ... policy content ...
```

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [integration-points.md](./integration-points.md) | Rego policy input schema from HolmesGPT-API |
| [crd-schema.md](./crd-schema.md) | AIAnalysis status fields populated from Rego |
| [DD-WORKFLOW-001](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) | Label schema (snake_case convention) |
| [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](../../../handoff/AIANALYSIS_TO_HOLMESGPT_API_TEAM.md) | `target_in_owner_chain` and `warnings` source |
