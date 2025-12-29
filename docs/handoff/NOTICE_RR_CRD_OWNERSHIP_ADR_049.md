# NOTICE: RemediationRequest CRD Ownership Clarification (ADR-049)

**Date**: December 7, 2025
**From**: Architecture Team
**To**: Gateway Service Team, Remediation Orchestrator Team
**Priority**: 🟡 MEDIUM (Documentation & Organizational Change)
**Status**: ✅ **APPROVED**

---

## 📋 Summary

**ADR-049** clarifies that **Remediation Orchestrator (RO) owns the RemediationRequest CRD schema definition**.

This resolves the previous ambiguity where documentation stated:
- "Owner: Central Controller Service"
- "Created By: Gateway Service"

---

## 🔄 What Changed

### Before (Ambiguous)

```
CRD_SCHEMAS.md:
  Owner: Central Controller Service
  Created By: Gateway Service

Gateway docs: Define RR schema
RO docs: Reference Gateway's schema
```

### After (Clear)

```
CRD_SCHEMAS.md:
  Schema Owner: Remediation Orchestrator
  Instances Created By: Gateway Service

RO docs: Authoritative source for RR schema
Gateway docs: Reference RO's schema, import RO types
```

---

## 🎯 Why This Change

| Factor | Rationale |
|--------|-----------|
| **K8s Pattern** | Controller that reconciles a CRD should own its schema |
| **Domain Ownership** | RR represents remediation lifecycle = RO's domain |
| **Clean Dependencies** | Gateway → RO types (not reverse) |
| **Schema Evolution** | RO can evolve RR based on orchestration needs |

---

## 📊 Impact by Team

### Gateway Service Team

| Aspect | Impact |
|--------|--------|
| **Instance Creation** | ✅ No change - Gateway still creates RR |
| **Status Updates** | ✅ No change - DD-GATEWAY-011 still valid |
| **Type Imports** | ⚠️ Import RR types from RO package |
| **Documentation** | ⚠️ Remove RR schema definitions, reference RO |

**Code Change (if needed)**:
```go
// Before (if Gateway defined types)
import remediationv1 "github.com/jordigilh/kubernaut/api/gateway/v1alpha1"

// After (import from RO)
import remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
```

### Remediation Orchestrator Team

| Aspect | Impact |
|--------|--------|
| **Schema Ownership** | ✅ RO now owns RR schema |
| **Type Definitions** | ✅ RR types live in `api/remediation/v1alpha1/` |
| **Documentation** | ⚠️ RO docs become authoritative for RR schema |
| **Reconciliation** | ✅ No change |

---

## 🔗 Compatibility

### DD-GATEWAY-011 (Shared Status Ownership)

**Fully compatible**. Gateway can still update its status sections:
- `status.deduplication` - Gateway owned
- `status.stormAggregation` - Gateway owned

The shared status pattern is about runtime behavior, not schema ownership.

### Redis Deprecation

**Fully compatible**. This ADR is about schema ownership, not data flow.

---

## ✅ Action Items

| Task | Owner | Priority |
|------|-------|----------|
| Update `CRD_SCHEMAS.md` | Architecture Team | ✅ Done |
| Update RO docs as authoritative source | RO Team | 🟡 Medium |
| Update Gateway docs to reference RO | Gateway Team | ✅ Done (2025-12-07) |
| Verify type import paths | Both Teams | ✅ **VERIFIED** (Gateway: `api/remediation/v1alpha1/`) |

---

## ✅ Acknowledgment Required

| Team | Acknowledged | Date | Notes |
|------|--------------|------|-------|
| Gateway Service | ✅ **ACKNOWLEDGED** | 2025-12-07 | **No code changes required**. Gateway already imports from `api/remediation/v1alpha1/`. DD-GATEWAY-011 shared status pattern confirmed compatible. Will update docs to reference RO as authoritative schema source. |
| Remediation Orchestrator | ✅ **ACKNOWLEDGED** | 2025-12-07 | RO accepts schema ownership. Types in `api/remediation/v1alpha1/` |

---

## 🔗 Related Documents

| Document | Purpose |
|----------|---------|
| [ADR-049](../architecture/decisions/ADR-049-remediationrequest-crd-ownership.md) | Full decision record |
| [DD-GATEWAY-011](../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md) | Shared status (compatible) |
| [CRD_SCHEMAS.md](../architecture/CRD_SCHEMAS.md) | Updated schema reference |

---

**Issued By**: Architecture Team
**Date**: December 7, 2025

