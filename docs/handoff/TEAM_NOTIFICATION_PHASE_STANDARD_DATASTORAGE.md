# TEAM NOTIFICATION: Phase Value Format Standard

**To**: DataStorage Team
**From**: SignalProcessing Team
**Date**: 2025-12-11
**Priority**: 🟡 **MEDIUM** - Informational with Clarification
**Type**: Standard Notification

---

## 📋 **Summary**

A new cross-service standard **BR-COMMON-001: Phase Value Format Standard** has been created requiring all CRD phase values to be capitalized per Kubernetes API conventions.

**DataStorage Impact**: ✅ **NONE** - Audit event strings intentionally use lowercase.

---

## 📚 **New Standard (BR-COMMON-001)**

### **Requirement**
All Kubernaut **CRD phase/status fields** MUST use capitalized values:
- ✅ `"Pending"`, `"Processing"`, `"Completed"`, `"Failed"`
- ❌ `"pending"`, `"processing"`, `"completed"`, `"failed"`

### **Important Clarification for DataStorage**
**BR-COMMON-001 applies ONLY to CRD phase fields, NOT audit event strings.**

Audit events stored in DataStorage **intentionally use lowercase** per ADR-034:
- ✅ Audit `event_action`: `"completed"`, `"failed"` - **CORRECT** (audit schema)
- ✅ Audit `event_outcome`: `"success"`, `"failure"` - **CORRECT** (audit schema)
- ✅ CRD phase values: `"Completed"`, `"Failed"` - **CORRECT** (CRD schema)

**These are different domains with different conventions.**

---

## ✅ **DataStorage Service Status**

| Aspect | Status | Notes |
|--------|--------|-------|
| **Has CRD Phase Field?** | ❌ No | Stateless service |
| **Stores Audit Events?** | ✅ Yes | Uses lowercase per ADR-034 |
| **Compliance Required?** | ✅ **N/A** | BR-COMMON-001 doesn't apply to audit schemas |
| **Action Needed?** | ✅ None | Continue using lowercase in audit events |

---

## 🔍 **Schema Domains Clarification**

### **Domain 1: CRD Phase Values (BR-COMMON-001 Applies)**
```yaml
# api/signalprocessing/v1alpha1/signalprocessing_types.go
status:
  phase: "Completed"  # ✅ CORRECT: Capitalized per BR-COMMON-001
```

### **Domain 2: Audit Event Strings (BR-COMMON-001 Does NOT Apply)**
```json
{
  "event_action": "completed",   // ✅ CORRECT: Lowercase per ADR-034
  "event_outcome": "success",    // ✅ CORRECT: Lowercase per ADR-034
  "event_type": "signalprocessing.signal.processed"
}
```

**Why Different?**
- **CRD phases**: Follow Kubernetes conventions (user-facing, `kubectl` output)
- **Audit events**: Follow internal audit schema (database schema, analytics)

---

## 🔗 **What Triggered This Standard**

**Incident**: SignalProcessing used lowercase phase values (`"pending"`, `"completed"`) while RemediationOrchestrator expected capitalized values (`"Pending"`, `"Completed"`).

**Impact**: RO couldn't detect SP completion → 5 integration tests failed → RemediationRequest stuck indefinitely.

**Resolution**: SP fixed on 2025-12-11 (same day), BR-COMMON-001 created to prevent future occurrences in **CRD phase fields only**.

**DataStorage Clarification**: Your lowercase audit event strings are **intentional and correct** per ADR-034. No changes needed.

---

## 📊 **Service Compliance Matrix**

| Service | CRD Phase Field | Compliant | Audit Event Schema |
|---------|-----------------|-----------|-------------------|
| **DataStorage** | N/A | ✅ N/A | ✅ Lowercase (ADR-034) |
| SignalProcessing | `status.phase` | ✅ | ✅ Lowercase (ADR-034) |
| AIAnalysis | `status.phase` | ✅ | ✅ Lowercase (ADR-034) |
| WorkflowExecution | `status.phase` | ✅ | ✅ Lowercase (ADR-034) |
| Notification | `status.phase` | ✅ | ✅ Lowercase (ADR-034) |
| RemediationRequest | `status.overallPhase` | ✅ | ✅ Lowercase (ADR-034) |

**All services use capitalized CRD phases AND lowercase audit events - this is correct!**

---

## 📚 **Reference Documents**

- **CRD Standard**: `docs/requirements/BR-COMMON-001-phase-value-format-standard.md`
- **Audit Schema**: ADR-034 (Audit Event Schema)
- **Original Issue**: `docs/handoff/NOTICE_SP_PHASE_CAPITALIZATION_BUG.md`
- **Kubernetes Conventions**: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status

---

## ✅ **No Action Required**

DataStorage team: **Continue using lowercase in audit events per ADR-034.** BR-COMMON-001 applies ONLY to CRD phase fields, not audit schemas.

**Acknowledgment**: No response required (informational notification with clarification).

---

**Document Status**: ✅ Informational with Clarification
**Created**: 2025-12-11
**From**: SignalProcessing Team
**Note**: Your audit schema is correct and unaffected by BR-COMMON-001. ✅

