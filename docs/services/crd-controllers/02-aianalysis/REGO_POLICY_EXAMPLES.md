# AIAnalysis Rego Policy Examples

**Status**: 🟡 DRAFT - For Design Discussion
**Version**: 1.0
**Date**: 2025-11-29
**Purpose**: Explore approval policy input/output schemas with sample policies

---

## Policy Input Schema

```go
// ApprovalPolicyInput is passed to Rego policy for evaluation
type ApprovalPolicyInput struct {
    // From AIAnalysis.status.selectedWorkflow
    Action     string  `json:"action"`     // e.g., "restart-pod", "scale-deployment"
    Confidence float64 `json:"confidence"` // 0.0-1.0 from HolmesGPT

    // From SignalProcessing context
    Environment      string `json:"environment"`      // production, staging, dev
    Severity         string `json:"severity"`         // critical, warning, info
    BusinessPriority string `json:"businessPriority"` // P0, P1, P2

    // Target resource
    TargetResource TargetResourceInput `json:"targetResource"`

    // Timestamp for business hours calculation
    Timestamp string `json:"timestamp"` // ISO 8601
}

type TargetResourceInput struct {
    Kind      string `json:"kind"`      // Pod, Deployment, StatefulSet
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}
```

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

### 2. Development/Staging Policy (`development.rego`)

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

# Confidence thresholds
high_confidence_threshold := 0.90
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

---

## Policy Selection Strategy

For V1.0, we'll use a **single combined policy** that incorporates:
1. Environment-based rules
2. Confidence-based rules
3. Action classification

Future V1.1+ can support:
- Multiple policy files per environment
- Policy composition/inheritance
- Customer-defined policies via ConfigMap

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

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-11-29 | Initial draft with sample policies |


