# NOTICE: Gateway Classification Removal - SP Team Coordination

> **Note (Issue #91):** CRD routing labels (`kubernaut.ai/signal-type`, `kubernaut.ai/severity`, etc.) referenced in this document were migrated to immutable spec fields. See Issue #91.

**Date**: 2025-12-06
**Version**: 1.1
**From**: Gateway Service Team
**To**: Signal Processing (SP) Service Team
**Status**: 🟡 **IN PROGRESS** - SP acknowledged, Gateway waiting for Day 5 completion
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
| 2025-12-06 | SP | ✅ **Acknowledged with corrections** - Default is `"unknown"` not `"development"`, label is `kubernaut.ai/environment` not `environment` |
| 2025-12-06 | SP | Selected **Option A** (remove fields from RemediationRequest CRD) |
| 2025-12-06 | SP | Created [NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md](./NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md) for RO team |
| 2025-12-06 | SP | **ETA 2025-12-09** for full readiness (Day 5 Priority Engine) |
| 2025-12-06 | Gateway | ✅ **Acknowledged SP corrections** - Answers to questions provided below |
| 2025-12-06 | SP | ✅ **Acknowledged Gateway response** - Coordination complete, proceeding with Day 5 |

---

## 🔄 Status Updates

### SP Team Response

**Status**: ✅ **ACKNOWLEDGED WITH CORRECTIONS**

```
Date: 2025-12-06
Acknowledged by: SP Team
Approach selected: Option A (Remove environment/priority from RemediationRequest CRD)
Handoff created: NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md (for RO team)
Estimated completion: 2025-12-09 (Day 5 Priority Engine completion)
```

---

### ⚠️ **CRITICAL CORRECTIONS REQUIRED**

The Gateway notice contains **discrepancies with authoritative SP documentation**:

#### **Correction 1: Default Environment is `"unknown"`, NOT `"development"`**

| Document | Default Value | Status |
|----------|---------------|--------|
| **BR-SP-053** (Authoritative) | `"unknown"` | ✅ CORRECT |
| Gateway Notice (line 100) | `"development"` | ❌ INCORRECT |

**BR-SP-053 Rationale**: *"Using `unknown` is more accurate than assuming `development` - organizations have varied environment taxonomies and `development` could mean different things to different customers."*

**Action Required**: Gateway team must update notice to reflect correct default.

---

#### **Correction 2: Only `kubernaut.ai/environment` Label, NOT `environment`**

| Document | Label Key | Status |
|----------|-----------|--------|
| **BR-SP-051** (Authoritative) | `kubernaut.ai/environment` ONLY | ✅ CORRECT |
| Gateway `classification.go` (line 128) | `environment` | ❌ INCORRECT |

**BR-SP-051 Rationale**: *"Using only `kubernaut.ai/` prefixed labels prevents accidentally capturing labels from other systems and ensures clear ownership of environment classification."*

**Impact**: Gateway's current implementation would NOT be compatible with SP's authoritative BRs. SP will NOT look for unqualified `environment` label.

---

### ✅ **SP Implementation Status**

| BR | Requirement | SP Status | Notes |
|----|-------------|-----------|-------|
| BR-SP-051 | Namespace labels (`kubernaut.ai/environment`) | ✅ **Day 4 COMPLETE** | Rego-based, case-insensitive |
| BR-SP-052 | ConfigMap fallback | ✅ **Day 4 COMPLETE** | Pattern matching supported |
| BR-SP-053 | Default `"unknown"` | ✅ **Day 4 COMPLETE** | 0.0 confidence |
| BR-SP-070 | Priority via Rego | ✅ **Day 5 COMPLETE** | Rego policy evaluation |
| BR-SP-071 | Priority fallback matrix | ✅ **Day 5 COMPLETE** | Severity-based fallback |
| BR-SP-072 | Rego hot-reload | ✅ **Day 5 COMPLETE** | fsnotify + FileWatcher |

> **Update (Dec 9, 2025)**: SignalProcessing V1.0 is 100% complete. All 17 BRs implemented, 270 tests passing.

---

### 📋 **Approach: Option A (Remove Fields from RemediationRequest CRD)**

**Architecture Clarification**:
- **RemediationRequest (RR) spec**: Contains ONLY what Gateway delivers (severity, signalType, targetResource, etc.)
- **SignalProcessing (SP) CRD**: SP populates its OWN `SignalProcessingStatus` with environment and priority
- **RemediationOrchestrator (RO)**: Handles data transitions between services

**Result**: `environment` and `priority` fields should be **REMOVED from RemediationRequestSpec**.

**RO Team Action Required**: See [NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md](./NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md)

```go
// api/remediation/v1alpha1/remediationrequest_types.go
type RemediationRequestSpec struct {
    Severity       string `json:"severity"`
    // REMOVED: Environment string `json:"environment"` -- Now in SignalProcessingStatus
    // REMOVED: Priority    string `json:"priority"`    -- Now in SignalProcessingStatus
    SignalType     string `json:"signalType"`
    SignalSource   string `json:"signalSource"`
    TargetResource TargetResource `json:"targetResource"`
    Fingerprint    string `json:"fingerprint"`
    // ... other Gateway-populated fields
}
```

**Data Flow**:
```
Gateway → RemediationRequest (no env/priority)
    ↓
RO creates SignalProcessing CRD
    ↓
SP enriches SignalProcessingStatus.EnvironmentClassification
SP enriches SignalProcessingStatus.PriorityAssignment
    ↓
RO reads from SignalProcessingStatus for downstream services
```

**Labels**: SP will populate `kubernaut.ai/environment` and `kubernaut.ai/priority` labels on the **SignalProcessing CRD**, not on RemediationRequest.

---

### ❓ **Questions for Gateway Team**

1. **Label Migration**: Will Gateway update existing code to use `kubernaut.ai/environment` instead of `environment` before removal, or will it simply stop populating?

2. **Backwards Compatibility**: Are there any downstream consumers that expect `environment` label (unqualified) that need migration?

3. **Removal Timeline**: Can Gateway wait until 2025-12-09 for SP Day 5 (Priority Engine) completion before removing?

---

### ✅ **Gateway Team Response to SP Questions** (2025-12-06)

#### **Answer 1: Label Migration**

**Action**: Gateway will **simply stop populating** environment and priority fields/labels.

**Rationale**:
- Gateway's current `environment` label lookup was incorrect per SP's authoritative BR-SP-051
- Since SP will own this entirely and use `kubernaut.ai/environment`, no migration needed
- Gateway will delete the classification code entirely, not modify it

#### **Answer 2: Backwards Compatibility**

**Confirmed**: **No downstream consumers** expect the unqualified `environment` label.

**Analysis**:
- Gateway was the only producer of this label
- The `kubernaut.ai/environment` label on RemediationRequest was being set correctly
- SP will now own label population on the SignalProcessing CRD
- No migration required for any downstream service

#### **Answer 3: Removal Timeline**

**Confirmed**: Gateway **WILL wait until 2025-12-09** for SP Day 5 completion.

**Updated Timeline**:
| Milestone | Date | Status |
|-----------|------|--------|
| SP acknowledges | 2025-12-06 | ✅ Complete |
| SP confirms approach | 2025-12-06 | ✅ Complete (Option A) |
| SP Day 5 Priority Engine | 2025-12-09 | ✅ **Complete** (Days 1-9 done) |
| SP signals ready | 2025-12-08 | ✅ **READY** |
| Gateway removes classification | 2025-12-10+ | 🟢 **UNBLOCKED** |

**SignalProcessing Status Update (Dec 8, 2025)**:
- ✅ Environment Classification: BR-SP-051, BR-SP-052, BR-SP-053 - **Complete**
- ✅ Priority Assignment: BR-SP-070, BR-SP-071, BR-SP-072 - **Complete**
- ✅ Unit tests: 184/184 passing (100%)
- ✅ Integration tests: 65/65 passing (100%)

**Gateway Team**: You may proceed with classification removal at your convenience. SignalProcessing is fully operational.

---

### ✅ **Gateway Acknowledgment of SP Corrections**

#### **Correction 1: Default Environment** ✅ ACKNOWLEDGED

Gateway agrees with SP's correction:
- **Correct default**: `"unknown"` (per BR-SP-053)
- **Note**: Gateway's implementation in `classification.go` line 196 already used `"unknown"` as default
- **Notice line 100**: This was documentation error referencing DD-WORKFLOW-001 incorrectly; SP's BR-SP-053 is authoritative

#### **Correction 2: Label Key** ✅ ACKNOWLEDGED

Gateway agrees with SP's correction:
- **Correct label**: `kubernaut.ai/environment` ONLY (per BR-SP-051)
- **Gateway error**: `classification.go` line 128 incorrectly looked for unqualified `environment` label
- **Impact**: This incorrect implementation will be deleted, not fixed
- **Going forward**: SP will own all environment/priority label population

---

### 📝 **SP Team Checklist Response**

| Checklist Item | Status | Notes |
|----------------|--------|-------|
| Acknowledge receipt | ✅ | Acknowledged 2025-12-06 |
| Confirm approach | ✅ | Option A selected (remove fields from RR) |
| BR-SP-051 (namespace labels) | ✅ | Using `kubernaut.ai/environment` ONLY |
| BR-SP-051 (`kubernaut.ai/environment`) | ✅ | Same as above |
| BR-SP-052 (ConfigMap fallback) | ✅ | Day 4 complete |
| BR-SP-053 (default) | ✅ | Default is `"unknown"` (corrected) |
| BR-SP-070 (Rego priority) | ✅ | **Day 5 complete** (2025-12-06) |
| BR-SP-071 (priority matrix) | ✅ | **Day 5 complete** - severity-only fallback |
| BR-SP-072 (hot-reload) | ✅ | **Day 5 complete** - `pkg/shared/hotreload/FileWatcher` |
| Update CRD schema | ⏳ | RO team to remove fields (see [NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md](./NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md)) |
| Update labels | ✅ | Will populate `kubernaut.ai/environment`, `kubernaut.ai/priority` |
| Signal ready | ✅ | **SP READY** - Environment + Priority classification implemented |

---

## ✅ Timeline

| Milestone | Target Date | Status |
|-----------|-------------|--------|
| SP acknowledges receipt | 2025-12-06 | ✅ Complete |
| SP confirms approach | 2025-12-06 | ✅ Complete (Option A) |
| Gateway acknowledges corrections | 2025-12-06 | ✅ Complete |
| SP Day 4 Environment Classifier | 2025-12-06 | ✅ **Complete** |
| SP Day 5 Priority Engine | 2025-12-06 | ✅ **Complete** (ahead of schedule) |
| SP signals ready | 2025-12-06 | ✅ **READY** |
| Gateway removes classification | 2025-12-07+ | 🟢 **UNBLOCKED** - Gateway may proceed |

**✅ SP READY**: Signal Processing environment and priority classification is fully implemented. Gateway team may proceed with removing their classification logic at their convenience.

**✅ GATEWAY COMPLETE** (2025-12-06): Gateway classification removal completed (commit 65b93fe0):
- Removed `EnvironmentClassifier` and `PriorityEngine` from server
- Removed `environment`/`priority` from `CreateRemediationRequest`, CRD labels, and HTTP response
- Deleted `classification.go`, `priority.go`, `priority.rego`, and related tests
- Updated `api-specification.md` with removal notes
- 20 files changed, 157 insertions, 1853 deletions

### SP Implementation Summary

| Component | Status | Location |
|-----------|--------|----------|
| Environment Classifier (Rego) | ✅ | `pkg/signalprocessing/classifier/environment.go` |
| Priority Engine (Rego) | ✅ | `pkg/signalprocessing/classifier/priority.go` |
| ConfigMap Hot-Reload | ✅ | `pkg/shared/hotreload/file_watcher.go` |
| Rego Policies | ✅ | `deploy/signalprocessing/policies/` |

---

**Document Version**: 1.3
**Last Updated**: 2025-12-06
**Status**: 🟢 **SP IMPLEMENTATION COMPLETE** - Gateway team may proceed with classification removal

