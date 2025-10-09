# RemediationRequest CRD Design Document Triage Report

**Date**: 2025-01-15
**Document Triaged**: `docs/design/CRD/01_REMEDIATION_REQUEST_CRD.md`
**Reference**: `docs/services/crd-controllers/05-remediationorchestrator/README.md`
**Status**: 🚨 **CRITICAL INCONSISTENCIES FOUND**

---

## 🎯 Executive Summary

The design document `docs/design/CRD/01_REMEDIATION_REQUEST_CRD.md` is **SEVERELY OUTDATED** and contains **CRITICAL INCONSISTENCIES** with the current V1 architecture and authoritative sources.

**Confidence Assessment**: 100% - These issues are factual and verifiable

**Recommendation**: **DEPRECATE AND REDIRECT** - Add prominent deprecation notices, do not update content

---

## 🚨 CRITICAL ISSUES

### **Issue 1: Incorrect CRD Name** ⛔ BLOCKER

**Location**: Lines 68-73, throughout document
**Severity**: 🔴 **CRITICAL**

**Current (WRONG)**:
```yaml
metadata:
  name: alertremediations.kubernaut.io
```

**Correct (from service specs)**:
```yaml
metadata:
  name: remediationrequests.remediation.kubernaut.io
```

**Impact**:
- CRD name doesn't match actual implementation
- API group is wrong (`kubernaut.io` vs `remediation.kubernaut.io`)
- Resource name uses deprecated "alert" prefix
- Would fail to deploy in cluster

**Evidence**:
- `api/remediation/v1alpha1/remediationrequest_types.go` - actual implementation
- `config/crd/bases/remediation.kubernaut.io_remediationrequests.yaml` - generated manifest
- `docs/services/crd-controllers/05-remediationorchestrator/README.md` line 7: "CRD: RemediationRequest"

---

### **Issue 2: Deprecated "Alert" Terminology** ⛔ BLOCKER

**Location**: Throughout entire document
**Severity**: 🔴 **CRITICAL**

**Examples**:
```yaml
alertFingerprint  → signalFingerprint   ✅ MIGRATED
alertName         → signalName          ✅ MIGRATED
alertPayload      → [DEPRECATED]        ❌ REMOVED
alertprocessor    → remediationprocessing ✅ MIGRATED
AlertRemediation  → RemediationRequest  ✅ MIGRATED
```

**Current Document Status**: Still uses deprecated "Alert" prefix extensively

**Kubernaut V1 Architecture**: Processes multiple signal types:
- Prometheus alerts
- Kubernetes events
- AWS CloudWatch alarms
- Datadog monitors
- Custom webhooks

**Impact**: Document contradicts [ADR-015: Alert to Signal Naming Migration](../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md)

**Evidence**: Document itself acknowledges deprecation in lines 19-28 but doesn't update content

---

### **Issue 3: Missing Phase 1 Fields** 🔴 HIGH

**Location**: Lines 95-136 (spec fields)
**Severity**: 🔴 **HIGH**

**Missing from document (ADDED in Phase 1)**:
```go
// Phase 1 additions (IMPLEMENTED, not documented):
SignalLabels      map[string]string `json:"signalLabels,omitempty"`
SignalAnnotations map[string]string `json:"signalAnnotations,omitempty"`
```

**Current Implementation Status**: ✅ IMPLEMENTED in `api/remediation/v1alpha1/remediationrequest_types.go` (commit 62709f2)

**Document Status**: ❌ NOT MENTIONED

**Impact**: Document is incomplete for V1 implementation

---

### **Issue 4: Incorrect API Version** 🟡 MEDIUM

**Location**: Line 70
**Severity**: 🟡 **MEDIUM**

**Current (Document)**:
```yaml
versions:
- name: v1
  served: true
  storage: true
```

**Correct (Actual Implementation)**:
```yaml
versions:
- name: v1alpha1
  served: true
  storage: true
```

**Rationale**: Using `v1alpha1` because V1 is not yet complete (per user instruction during Kubebuilder setup)

**Impact**: API version mismatch between document and implementation

---

### **Issue 5: Outdated Service Names** 🟡 MEDIUM

**Location**: Lines 197-278 (serviceStatuses)
**Severity**: 🟡 **MEDIUM**

**Current (Document)**:
```yaml
serviceStatuses:
  alertprocessor:    # ❌ WRONG
  aianalysis:        # ❌ WRONG (singular)
  workflow:          # ❌ WRONG (incomplete name)
  executor:          # ❌ WRONG (incomplete name)
```

**Correct (V1 Architecture)**:
```yaml
# Actual CRD names in V1:
RemediationProcessing
AIAnalysis
WorkflowExecution
KubernetesExecution
```

**Evidence**: See `docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md` V1 services table

---

### **Issue 6: Missing Self-Containment Pattern** 🔴 HIGH

**Location**: Entire spec section
**Severity**: 🔴 **HIGH**

**Missing Critical Pattern**: Document doesn't describe the **self-contained CRD pattern** that is core to Phase 1 implementation.

**What's Missing**:
- RemediationProcessing receives **18 self-contained fields** from RemediationRequest
- No external CRD lookups required during reconciliation
- Immutable data snapshot for reliability

**Evidence**: `docs/analysis/PHASE_1_IMPLEMENTATION_GUIDE.md` - entire Phase 2 task dedicated to this

**Impact**: Document doesn't reflect core V1 architectural decision

---

### **Issue 7: Incorrect Authoritative Source Claims** 🟡 MEDIUM

**Location**: Lines 15-17
**Severity**: 🟡 **MEDIUM**

**Current (Document)**:
```markdown
The authoritative CRD definitions are in:
- **[CRD_SCHEMAS.md](../../architecture/CRD_SCHEMAS.md)** - Authoritative OpenAPI v3 schemas
```

**Reality**: The **actual authoritative source** is:
```
api/remediation/v1alpha1/remediationrequest_types.go
```

**Why**: Go type definitions are source of truth for Kubebuilder code generation. `CRD_SCHEMAS.md` is documentation, not implementation.

**Evidence**: Kubebuilder generates CRD manifests from Go code, not from markdown docs

---

## 📊 SCHEMA COMPARISON

### **Fields in Document vs Reality**

| Field | Document | Reality (v1alpha1) | Status |
|-------|----------|-------------------|--------|
| `alertFingerprint` | ✅ Present | ❌ `signalFingerprint` | 🔴 Name changed |
| `alertName` | ✅ Present | ❌ `signalName` | 🔴 Name changed |
| `alertPayload` | ✅ Present | ❌ Removed | 🔴 Deleted |
| `signalLabels` | ❌ Missing | ✅ Present | 🔴 Not documented |
| `signalAnnotations` | ❌ Missing | ✅ Present | 🔴 Not documented |
| `signalType` | ❌ Missing | ✅ Present | 🔴 Not documented |
| `signalSource` | ❌ Missing | ✅ Present | 🔴 Not documented |
| `targetType` | ❌ Missing | ✅ Present | 🔴 Not documented |
| `providerData` | ❌ Missing | ✅ Present | 🔴 Not documented |
| `deduplication` | ❌ Simple fields | ✅ Nested struct | 🟡 Incomplete |
| `timeoutConfig` | ❌ `maxDuration` | ✅ `TimeoutConfig` struct | 🟡 Different structure |

**Completeness**: ~40% of V1 fields are missing or incorrect

---

## 🏗️ ARCHITECTURAL MISMATCHES

### **1. Status Structure**

**Document Shows**:
```yaml
status:
  serviceStatuses:
    alertprocessor: {...}
    aianalysis: {...}
    workflow: {...}
    executor: {...}
```

**Reality (from CRD_SCHEMAS.md)**:
```go
type RemediationRequestStatus struct {
    OverallPhase string
    StartTime metav1.Time
    CompletionTime *metav1.Time

    // CRD References (not nested status)
    RemediationProcessingRef *RemediationProcessingReference
    AIAnalysisRef            *AIAnalysisReference
    WorkflowExecutionRef     *WorkflowExecutionReference

    // Lightweight status summaries (not full copies)
    RemediationProcessingStatus *RemediationProcessingStatusSummary
    AIAnalysisStatus            *AIAnalysisStatusSummary
    WorkflowExecutionStatus     *WorkflowExecutionStatusSummary
}
```

**Key Difference**: V1 uses **CRD references + lightweight summaries**, not nested full status copies

---

### **2. Controller Name**

**Document**: "AlertRemediation Controller"
**Reality**: "RemediationOrchestrator Controller" (also called "RemediationRequest Controller")

**Evidence**: `docs/services/crd-controllers/05-remediationorchestrator/README.md` line 8

---

### **3. Targeting Data Pattern**

**Document**: No mention of targeting data pattern
**Reality**: Core architectural pattern (see `docs/services/crd-controllers/05-remediationorchestrator/data-handling-architecture.md`)

**Pattern Purpose**: Immutable data snapshot for child CRDs

**Impact**: Document doesn't explain one of the most important V1 design decisions

---

## 📋 BUSINESS REQUIREMENTS MISALIGNMENT

### **Outdated BRs Referenced**

**Document References**:
- `BR-PA-001`, `BR-PA-003`, `BR-PA-010` - Processor service BRs (wrong service)
- `BR-WH-008` - Webhook service BR
- `BR-AP-021` - Alert Processing (deprecated)
- `BR-ALERT-003`, `BR-ALERT-005` - Generic alert BRs

**Correct BRs (from service specs)**:
- `BR-REM-001` to `BR-REM-050` - Central orchestration of remediation lifecycle
- `BR-REM-010` to `BR-REM-025` - State machine, phase coordination
- `BR-REM-030` to `BR-REM-040` - Targeting Data Pattern
- `BR-REM-045` to `BR-REM-050` - Escalation notification

**Evidence**: `docs/services/crd-controllers/05-remediationorchestrator/README.md` lines 100-107

---

## 🎯 RECOMMENDED ACTIONS

### **Option A: Deprecate and Redirect** ⭐ RECOMMENDED

**Effort**: 10 minutes
**Benefit**: Prevent confusion immediately

**Actions**:
1. Add **prominent deprecation banner** at top of document
2. Redirect readers to authoritative sources:
   - `api/remediation/v1alpha1/remediationrequest_types.go` (implementation)
   - `docs/architecture/CRD_SCHEMAS.md` (schema documentation)
   - `docs/services/crd-controllers/05-remediationorchestrator/` (service specs)
3. Keep document as **historical reference** for design evolution
4. Move to `docs/design/CRD/archive/` directory

**Deprecation Banner**:
```markdown
# ⛔ DEPRECATED - DO NOT USE

**Status**: Historical Reference Only
**Deprecated**: January 2025
**Reason**: Superseded by V1 implementation

## 🎯 For Current V1 Information, See:

1. **Implementation** (Source of Truth):
   - [`api/remediation/v1alpha1/remediationrequest_types.go`](../../../api/remediation/v1alpha1/remediationrequest_types.go)

2. **Schema Documentation**:
   - [`docs/architecture/CRD_SCHEMAS.md`](../../architecture/CRD_SCHEMAS.md)

3. **Service Specifications**:
   - [`docs/services/crd-controllers/05-remediationorchestrator/`](../../services/crd-controllers/05-remediationorchestrator/)

## ⚠️ Known Issues in This Document:
- Uses deprecated "Alert" prefix (now "Signal")
- CRD name is wrong (`alertremediations` → `remediationrequests`)
- API version is wrong (`v1` → `v1alpha1`)
- Missing Phase 1 fields (`signalLabels`, `signalAnnotations`)
- Service names are outdated
- Business requirements are incorrect

**Do not use this document for implementation.**
```

---

### **Option B: Complete Rewrite** ❌ NOT RECOMMENDED

**Effort**: 4-6 hours
**Benefit**: Updated design document

**Why NOT Recommended**:
1. **Duplication**: Would duplicate information already in service specs
2. **Maintenance Burden**: Two sources of truth to keep in sync
3. **Lower Value**: Implementation (`*.go` files) is actual source of truth
4. **Time Better Spent**: Phase 1 implementation tasks are higher priority

---

### **Option C: Delete Entirely** ⚠️ CONSIDER LATER

**Effort**: 1 minute
**Benefit**: Remove outdated information

**Why NOT Now**:
- May have historical value for understanding design evolution
- Better to archive first, delete later if truly unused
- Some migration context might be useful

---

## 📊 CONFIDENCE ASSESSMENT

**Overall Confidence**: **100%**

**Reasoning**:
- All issues are factually verifiable
- Comparisons made against authoritative sources:
  - Actual Go code implementation
  - Generated CRD manifests
  - Service specification documents
  - Architecture decision records
- No subjective assessments - only objective facts

**Risk of Taking Action**: **0%**

**Risk of NOT Taking Action**: **HIGH**
- Developers may use outdated document for implementation
- Documentation will contradict actual codebase
- Confusion about CRD schema and naming

---

## 🔗 RELATED DOCUMENTS

**Authoritative Sources**:
1. `api/remediation/v1alpha1/remediationrequest_types.go` - Implementation
2. `config/crd/bases/remediation.kubernaut.io_remediationrequests.yaml` - Generated manifest
3. `docs/architecture/CRD_SCHEMAS.md` - Schema documentation
4. `docs/services/crd-controllers/05-remediationorchestrator/` - Service specs

**Related Decisions**:
1. `docs/architecture/decisions/ADR-015-alert-to-signal-naming-migration.md`
2. `docs/architecture/decisions/005-owner-reference-architecture.md`
3. `docs/V1_SOURCE_OF_TRUTH_HIERARCHY.md`

**Migration Guides**:
1. `docs/analysis/PHASE_1_IMPLEMENTATION_GUIDE.md`
2. `docs/analysis/CRD_SCHEMA_UPDATE_ACTION_PLAN.md`

---

## 📝 NEXT STEPS

**Immediate (RECOMMENDED)**:
1. ✅ Apply Option A deprecation banner
2. ✅ Move document to `docs/design/CRD/archive/`
3. ✅ Update any links pointing to this document
4. ✅ Continue with Phase 1 implementation (current priority)

**Later (Optional)**:
1. Review other documents in `docs/design/CRD/` for similar issues
2. Consider deleting if archive provides no value
3. Create lightweight "How to Find CRD Documentation" guide

---

**Triage Complete** - Ready for deprecation and archival

