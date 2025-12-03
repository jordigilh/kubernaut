# RO Contract Gaps - WorkflowExecution Team

**From**: Remediation Orchestrator Team
**To**: WorkflowExecution Team
**Date**: December 1, 2025
**Status**: ✅ ALL GAPS RESOLVED

---

## Summary

| Gap ID | Issue | Severity | Status |
|--------|-------|----------|--------|
| GAP-C5-01 | WorkflowId vs Name naming | 🟡 Medium | ✅ Resolved in v3.1 |
| GAP-C5-02 | ContainerImage missing | 🔴 Critical | ✅ Resolved in v3.1 |
| GAP-C5-03 | Steps required but not provided | 🔴 Critical | ✅ Resolved in v3.1 |
| GAP-C5-04 | ExecutionStrategy source unclear | 🟠 High | ✅ Resolved in v3.1 |

---

## 🎉 All Gaps Already Resolved

**Important**: The RO team was referencing an **outdated CRD schema (v1.x)**. The WorkflowExecution CRD was significantly simplified in **v2.0 (2025-11-28)** per ADR-044 (Engine Delegation) and enhanced in **v3.0/v3.1 (2025-12-01)**.

---

## WorkflowExecution Team Response

**Date**: December 1, 2025
**Respondent**: WorkflowExecution Team

**GAP-C5-01 (WorkflowId vs Name)**:
- [x] ✅ **RESOLVED** - `WorkflowDefinition` replaced with `WorkflowRef`
- `WorkflowRef.workflowId` is now an explicit field

**GAP-C5-02 (ContainerImage)**:
- [x] ✅ **RESOLVED** - Fields added to `WorkflowRef`:
  - `containerImage` - OCI bundle for Tekton
  - `containerDigest` - for audit trail
- `Parameters` added as top-level spec field

**GAP-C5-03 (Steps)**:
- [x] ✅ **RESOLVED** - Field **completely removed** per ADR-044
- Tekton handles step orchestration
- Steps live inside the OCI bundle (Tekton Pipeline)

**GAP-C5-04 (ExecutionStrategy)**:
- [x] ✅ **RESOLVED** - Simplified to `ExecutionConfig`:
  - `timeout` (default: 30m)
  - `serviceAccountName` (default: "kubernaut-workflow-runner")
- Removed fields: `ApprovalRequired`, `DryRunFirst`, `RollbackStrategy`, `MaxRetries`, `SafetyChecks`

---

## 🔴 NEW: v3.1 Requirements for RO

### REQ-WE-01: Populate `targetResource` (REQUIRED)

**Format**:
- Namespaced resources: `namespace/kind/name` (e.g., `payment/Deployment/payment-api`)
- Cluster-scoped resources: `kind/name` (e.g., `Node/worker-node-1`)

```go
func buildTargetResource(rr *RemediationRequest) string {
    tr := rr.Spec.TargetResource
    if tr.Namespace != "" {
        return fmt.Sprintf("%s/%s/%s", tr.Namespace, tr.Kind, tr.Name)
    }
    return fmt.Sprintf("%s/%s", tr.Kind, tr.Name)
}
```

### REQ-WE-02: Handle `Skipped` Phase (REQUIRED)

**Skip Reasons**:
| Reason | Meaning | RO Action |
|--------|---------|-----------|
| `ResourceBusy` | Another workflow running on same target | Mark RR as Skipped |
| `RecentlyRemediated` | Same workflow+target ran <5min ago | Mark RR as Skipped (dedup) |

---

## ✅ RO Team Acknowledgment

**Date**: December 1, 2025
**Respondent**: Remediation Orchestrator Team

**REQ-WE-01 (targetResource)**:
- [x] ✅ Acknowledged
- [x] Implementation planned for: V1.0 (BR-ORCH-032)

**REQ-WE-02 (Skipped phase)**:
- [x] ✅ Acknowledged
- [x] Implementation planned for: V1.0 (BR-ORCH-032, BR-ORCH-033, BR-ORCH-034)
- [x] Design documented in: **DD-RO-001 (Resource Lock Deduplication Handling)**

**REQ-WE-03 (Schema review)**:
- [x] ✅ Documents reviewed

**REQ-WE-04 (Acknowledgment)**:
- [x] ✅ Contract changes acknowledged

---

## ✅ Clarification Questions Resolved

**Q1 (Casing)**:
- [x] **Option A - Preserve casing** (matches Kubernetes conventions)

**Q2 (APIVersion)**:
- [x] **Agree - Kind alone sufficient for V1.0**

**Q3 (Namespace for cluster-scoped)** - Gateway confirmed:
- [x] ✅ **Empty string for cluster-scoped resources**

---

## 📚 RO Implementation References

| Item | BR/DD | Status |
|------|-------|--------|
| Handle Skipped phase | BR-ORCH-032 | Planned |
| Track duplicates | BR-ORCH-033 | Planned |
| Bulk notification | BR-ORCH-034 | Planned |
| Design decision | DD-RO-001 | ✅ Approved |

---

**Document Version**: 1.6
**Last Updated**: December 2, 2025
**Migrated From**: `docs/services/crd-controllers/03-workflowexecution/RO_CONTRACT_GAPS.md`
**Changelog**:
- v1.6: Migrated to `docs/handoff/` as authoritative Q&A directory
- v1.5: Gateway team confirmed Q3
- v1.3: Added RO acknowledgment
- v1.1: Added WE response and requirements
- v1.0: Initial RO contract gaps


