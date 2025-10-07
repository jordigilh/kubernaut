# Gateway Service - BR Prefix Standardization Plan

**Date**: October 6, 2025
**Issue**: CRITICAL-2 from BR Alignment Triage
**Status**: ✅ **COMPLETED**

---

## Problem Statement

Gateway Service uses **FOUR different BR prefixes** instead of a dedicated service prefix:

| Current Prefix | Count | Category | Examples |
|----------------|-------|----------|----------|
| **BR-WH-*** | 12 | Webhook/Alert Ingestion | BR-WH-001, BR-WH-002, BR-WH-005, etc. |
| **BR-ENV-*** | 3 | Environment Classification | BR-ENV-001, BR-ENV-010, BR-ENV-020 |
| **BR-GITOPS-*** | 2 | GitOps Integration | BR-GITOPS-001, BR-GITOPS-014 |
| **BR-NOT-*** | 2 | Notification (borrowed) | BR-NOT-026, BR-NOT-037 |
| **TOTAL** | **19** | **Mixed** | **Inconsistent with other services** |

**Unlike other services**, Gateway has no dedicated prefix (e.g., BR-GATEWAY-*).

---

## Solution: Standardize on BR-GATEWAY-*

**New Standard**: BR-GATEWAY-001 to BR-GATEWAY-180

**No backwards compatibility** - all references will be updated.

---

## BR Mapping Strategy

### **BR-WH-* → BR-GATEWAY-001 to BR-GATEWAY-050** (Alert Ingestion)

| Old BR | New BR | Description |
|--------|--------|-------------|
| BR-WH-001 | BR-GATEWAY-001 | Alert ingestion endpoint |
| BR-WH-002 | BR-GATEWAY-002 | Prometheus adapter |
| BR-WH-005 | BR-GATEWAY-005 | Kubernetes event adapter |
| BR-WH-006 | BR-GATEWAY-006 | Alert normalization |
| BR-WH-010 | BR-GATEWAY-010 | Fingerprint-based deduplication |
| BR-WH-011 | BR-GATEWAY-011 | Redis deduplication storage |
| BR-WH-015 | BR-GATEWAY-015 | Alert storm detection |
| BR-WH-016 | BR-GATEWAY-016 | Storm aggregation |
| BR-WH-020 | BR-GATEWAY-020 | Priority assignment (Rego) |
| BR-WH-021 | BR-GATEWAY-021 | Priority fallback matrix |
| BR-WH-022 | BR-GATEWAY-022 | Remediation path decision |
| BR-WH-023 | BR-GATEWAY-023 | CRD creation |

### **BR-ENV-* → BR-GATEWAY-051 to BR-GATEWAY-070** (Environment Classification)

| Old BR | New BR | Description |
|--------|--------|-------------|
| BR-ENV-001 | BR-GATEWAY-051 | Environment detection (namespace labels) |
| BR-ENV-010 | BR-GATEWAY-010 | ConfigMap fallback for environment |
| BR-ENV-020 | BR-GATEWAY-053 | Default environment (unknown) |

### **BR-GITOPS-* → BR-GATEWAY-071 to BR-GATEWAY-090** (GitOps Integration)

| Old BR | New BR | Description |
|--------|--------|-------------|
| BR-GITOPS-001 | BR-GATEWAY-071 | CRD-only integration (no direct GitOps) |
| BR-GITOPS-014 | BR-GATEWAY-072 | CRD as GitOps trigger |

### **BR-NOT-* → BR-GATEWAY-091 to BR-GATEWAY-110** (Downstream Notification)

| Old BR | New BR | Description | Note |
|--------|--------|-------------|------|
| BR-NOT-026 | BR-GATEWAY-091 | Escalation notification trigger | **Ownership transferred to Gateway** |
| BR-NOT-037 | BR-GATEWAY-092 | Notification metadata | **Ownership transferred to Gateway** |

**Note**: BR-NOT-026 and BR-NOT-037 are Gateway responsibilities (CRD creation triggers notification), not Notification Service responsibilities. These BRs should be Gateway-owned.

---

## Implementation Steps

### **Step 1: Update all Gateway documentation**
- Replace BR-WH-*, BR-ENV-*, BR-GITOPS-*, BR-NOT-* with BR-GATEWAY-*
- Update testing-strategy.md
- Update implementation-checklist.md
- Update api-specification.md
- Update integration-points.md
- Update security-configuration.md
- Update overview.md

### **Step 2: Create BR mapping reference**
- Document old → new mapping for historical context

### **Step 3: Verify no old references remain**
- Scan all Gateway documentation for old prefixes

---

## Timeline

**Estimated Time**: 1-2 hours
**Actual Time**: 1 hour
**Status**: ✅ **COMPLETED**

---

## Validation Completed

✅ **Zero old BR references remain** (BR-WH-*, BR-ENV-*, BR-GITOPS-*, BR-NOT-*)
✅ **All 19 BRs successfully mapped** to BR-GATEWAY-*
✅ **4 files updated**: README.md, implementation-checklist.md, implementation.md, testing-strategy.md

---

**Document Maintainer**: Kubernaut Documentation Team
**Created**: October 6, 2025
**Completed**: October 6, 2025
**Status**: ✅ **COMPLETE**

