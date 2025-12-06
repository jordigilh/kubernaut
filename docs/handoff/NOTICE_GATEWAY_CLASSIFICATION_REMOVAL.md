# NOTICE: Gateway Classification Removal - SP Team Coordination

**Date**: 2025-12-06
**Version**: 1.0
**From**: Gateway Service Team
**To**: Signal Processing (SP) Service Team
**Status**: 🔴 **ACTION REQUIRED**
**Priority**: HIGH

---

## 📋 Summary

The Gateway service is **removing all environment and priority classification logic**. The Signal Processing service must take full ownership of these fields in the `RemediationRequest` CRD.

---

## 🎯 What Gateway Is Removing

### Files Being Deleted

| File | Description | Lines |
|------|-------------|-------|
| `pkg/gateway/processing/classification.go` | `EnvironmentClassifier` implementation | 259 |
| `pkg/gateway/processing/priority.go` | `PriorityEngine` with Rego evaluation | 220 |
| `test/unit/gateway/processing/environment_classification_test.go` | Environment tests | 386 |
| `test/unit/gateway/priority_classification_test.go` | Priority tests | ~200 |
| `config/rego/gateway/priority.rego` | Priority Rego policy | ~50 |

### CRD Fields Gateway Will NO LONGER Populate

| Field | CRD Location | Current Gateway Logic | SP Must Handle |
|-------|--------------|----------------------|----------------|
| **`spec.environment`** | `RemediationRequestSpec.Environment` | Namespace label lookup + ConfigMap fallback | ✅ YES |
| **`spec.priority`** | `RemediationRequestSpec.Priority` | Rego policy: severity × environment matrix | ✅ YES |
| **Label: `kubernaut.ai/environment`** | `metadata.labels` | Same as spec.environment | ✅ YES |
| **Label: `kubernaut.ai/priority`** | `metadata.labels` | Same as spec.priority | ✅ YES |

### What Gateway Will Continue to Populate

| Field | CRD Location | Gateway Logic |
|-------|--------------|---------------|
| `spec.severity` | `RemediationRequestSpec.Severity` | From alert payload |
| `spec.signalType` | `RemediationRequestSpec.SignalType` | Adapter source type |
| `spec.signalSource` | `RemediationRequestSpec.SignalSource` | Alert source |
| `spec.targetResource` | `RemediationRequestSpec.TargetResource` | Extracted from alert |
| `spec.fingerprint` | `RemediationRequestSpec.Fingerprint` | Generated hash |
| All `kubernaut.ai/*` labels except environment/priority | `metadata.labels` | Various |

---

## 📐 CRD Schema Changes Required

### Option A: Remove Fields from CRD (Recommended)

The `environment` and `priority` fields should be **removed from the CRD spec** if SP will populate them via status updates or a different mechanism.

```go
// api/remediation/v1alpha1/remediationrequest_types.go
type RemediationRequestSpec struct {
    Severity       string `json:"severity"`
    // REMOVED: Environment string `json:"environment"`  
    // REMOVED: Priority    string `json:"priority"`     
    SignalType     string `json:"signalType"`
    // ... other fields
}
```

### Option B: Keep Fields, SP Populates via Controller

If SP will watch `RemediationRequest` CRDs and enrich them:

```go
// SP controller pseudocode
func (r *SPReconciler) Reconcile(ctx context.Context, req ctrl.Request) {
    rr := &RemediationRequest{}
    r.Get(ctx, req.NamespacedName, rr)
    
    if rr.Spec.Environment == "" {
        // Classify and update
        rr.Spec.Environment = r.classifier.Classify(ctx, rr.Spec.TargetResource.Namespace)
        rr.Spec.Priority = r.priorityEngine.Assign(ctx, rr.Spec.Severity, rr.Spec.Environment)
        r.Update(ctx, rr)
    }
}
```

---

## ✅ SP Team Checklist

### Required Before Gateway Removal

- [ ] **Acknowledge receipt** of this notice (update Status section below)
- [ ] **Confirm approach**: Option A (remove fields) or Option B (SP enriches)
- [ ] **Verify existing SP implementation** covers:
  - [ ] Environment classification from namespace labels (BR-SP-051)
  - [ ] Environment classification from `kubernaut.ai/environment` label (BR-SP-051)
  - [ ] ConfigMap-based environment fallback (BR-SP-052)
  - [ ] Default to `"development"` (BR-SP-053, per DD-WORKFLOW-001 v2.2)
  - [ ] Priority assignment via Rego (BR-SP-070)
  - [ ] Priority matrix: severity × environment (BR-SP-071)
  - [ ] Hot-reload of Rego policies (BR-SP-072)
- [ ] **Update CRD schema** if removing fields
- [ ] **Update labels** (`kubernaut.ai/environment`, `kubernaut.ai/priority`) population
- [ ] **Signal ready** for Gateway to proceed with removal

---

## 📚 Reference: Current Gateway Implementation

### Environment Classification (`classification.go`)

```go
// Lookup order:
// 1. Cache (fast path)
// 2. Namespace label: "environment"
// 3. ConfigMap: kubernaut-system/kubernaut-environment-overrides
// 4. Default: "unknown"

func (c *EnvironmentClassifier) Classify(ctx context.Context, namespace string) string {
    if env := c.getFromCache(namespace); env != "" {
        return env
    }
    
    ns := &corev1.Namespace{}
    if err := c.k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns); err == nil {
        if env, ok := ns.Labels["environment"]; ok && env != "" {
            c.setCache(namespace, env)
            return env
        }
    }
    
    // ConfigMap fallback...
    // Default: "unknown"
}
```

### Priority Assignment (`priority.go`)

```go
// Rego policy query: data.kubernaut.gateway.priority.priority
// Input: { "severity": "critical", "environment": "production", "labels": {...} }
// Output: "P0", "P1", "P2", or "P3"

func (p *PriorityEngine) Assign(ctx context.Context, severity, environment string) string {
    priority, err := p.evaluateRego(ctx, severity, environment)
    if err == nil {
        return priority
    }
    return "P2" // Safe default
}
```

### Current Rego Policy (`config/rego/gateway/priority.rego`)

```rego
package kubernaut.gateway.priority

default priority := "P2"

# Critical production → P0
priority := "P0" {
    input.severity == "critical"
    input.environment == "production"
}

# Critical staging → P1
priority := "P1" {
    input.severity == "critical"
    input.environment == "staging"
}

# Warning production → P1
priority := "P1" {
    input.severity == "warning"
    input.environment == "production"
}

# Info production → P2
priority := "P2" {
    input.severity == "info"
    input.environment == "production"
}
```

---

## 🔗 Related Documents

| Document | Description |
|----------|-------------|
| [ADR-047](../architecture/decisions/ADR-047-policy-engine-selection.md) | Policy engine selection (Rego approved) |
| [DD-WORKFLOW-001 v2.2](../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) | Mandatory label schema, canonical environments |
| [NOTICE_CLASSIFIER_SPEC_VIOLATIONS](./NOTICE_CLASSIFIER_SPEC_VIOLATIONS.md) | Issues found in SP classifier (being fixed) |
| [BR-SP-051-053](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md) | Environment classification BRs |
| [BR-SP-070-072](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md) | Priority assignment BRs |

---

## 📝 Communication Log

| Date | Team | Action |
|------|------|--------|
| 2025-12-06 | Gateway | Created notice, awaiting SP acknowledgment |
| | SP | *Pending acknowledgment* |

---

## 🔄 Status Updates

### SP Team Response

**Status**: ⏳ **PENDING SP ACKNOWLEDGMENT**

*SP team: Update this section with your response*

```
Date: [YYYY-MM-DD]
Acknowledged by: [Name]
Approach selected: [Option A / Option B]
Estimated completion: [Date]
Notes: [Any concerns or questions]
```

---

## ⚠️ Timeline

| Milestone | Target Date | Status |
|-----------|-------------|--------|
| SP acknowledges receipt | 2025-12-06 | ⏳ Pending |
| SP confirms approach | 2025-12-07 | ⏳ Pending |
| SP signals ready | 2025-12-09 | ⏳ Pending |
| Gateway removes classification | After SP ready | 🔒 Blocked |

**BLOCKING**: Gateway will NOT remove classification until SP confirms readiness.

---

**Document Version**: 1.0
**Last Updated**: 2025-12-06

