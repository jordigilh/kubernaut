# Test Plan: Native CRD Tools (Issue #19)

**Test Plan Identifier:** TP-AF-019
**Version:** 1.0
**Date:** 2026-05-04
**Author:** AI Assistant

---

## 1. Introduction

This test plan covers the 6 CRD-native MCP tools that operate directly on Kubernaut K8s CRDs via impersonated K8s API calls. These tools provide the Create/Read/Approve/Cancel lifecycle for remediations.

## 2. Test Items

- `internal/tools/kubernaut_list_remediations.go`
- `internal/tools/kubernaut_get_remediation.go`
- `internal/tools/kubernaut_submit_signal.go`
- `internal/tools/kubernaut_approve.go`
- `internal/tools/kubernaut_cancel_remediation.go`
- `internal/tools/kubernaut_watch.go`
- `internal/tools/helpers.go`

## 3. Features to Be Tested

| Feature | Source | Priority |
|---------|--------|----------|
| List RRs with namespace/phase/kind/name filtering | Issue #19 | P0 |
| Get single RR detail | Issue #19 | P0 |
| Create SignalProcessing CRD (submit signal) | Issue #19 | P0 |
| Patch RAR status (approve/reject) | Issue #19, ADR-040 | P0 |
| Cancel active RR | Issue #19 | P0 |
| Watch RR + related CRD phase transitions via SSE | Issue #3, #19 | P0 |
| Impersonation on all tools | ARCHITECTURE.md Section 6 | P0 |
| User-friendly error messages (no raw K8s errors) | ADK Investigation Plan UX | P0 |
| Dedup via Lease for submit_signal | Issue #19 | P1 |
| decidedBy from JWT identity | Issue #19 | P0 |
| OwnerRef correlation for watch | Issue #3 | P1 |
| Heartbeat on watch stream | kubernaut#874 step 13 | P1 |

## 4. Approach

- **Mock K8s API:** `k8s.io/client-go/kubernetes/fake` with reactor-based responses
- **Impersonation verification:** Assert `ClientFactory.ClientForScope(ScopeUserImpersonation)` called
- **Error mapping:** Verify `ToUserFriendlyError()` wraps all K8s 403/404 errors
- **Watch tests:** Use fake watch interface with injected events

## 5. Pass/Fail Criteria

- 30 specs pass with `ginkgo -race`
- Coverage >= 80% per tool file
- All 403 errors produce user-friendly messages
- No CRD internal field names in tool output (e.g., use "status" not "spec.phase")
- Watch goroutines exit cleanly on context cancel

## 6. Test Case Matrix

### kubernaut_list_remediations (5 specs)

| ID | Description | Input | Expected |
|----|-------------|-------|----------|
| UT-AF-101-001 | Lists RRs in namespace | {namespace: "payments"} | RR summaries returned |
| UT-AF-101-002 | Filters by phase | {namespace: "payments", phase: "Executing"} | Only matching RRs |
| UT-AF-101-003 | Filters by kind and name | {kind: "Deployment", name: "api"} | Label-filtered results |
| UT-AF-101-004 | Empty result | {namespace: "empty"} | Empty list, no error |
| UT-AF-101-005 | 403 user-friendly | Forbidden namespace | User-friendly error msg |

### kubernaut_get_remediation (4 specs)

| ID | Description | Input | Expected |
|----|-------------|-------|----------|
| UT-AF-102-001 | Full RR detail | {namespace: "pay", name: "rr-1"} | Spec + Status |
| UT-AF-102-002 | Not found | {namespace: "pay", name: "missing"} | User-friendly 404 |
| UT-AF-102-003 | 403 | Forbidden namespace | User-friendly 403 |
| UT-AF-102-004 | rr_id shorthand | {rr_id: "pay/rr-1"} | Same as ns+name |

### kubernaut_submit_signal (5 specs)

| ID | Description | Input | Expected |
|----|-------------|-------|----------|
| UT-AF-103-001 | Creates SP CRD | Valid signal params | SP created, name returned |
| UT-AF-103-002 | Spec populated | All fields provided | Fields match input |
| UT-AF-103-003 | decidedBy set | JWT user: "alice" | SP.spec.reportedBy == "alice" |
| UT-AF-103-004 | Dedup via Lease | Duplicate fingerprint | Second create blocked |
| UT-AF-103-005 | 403 | Forbidden namespace | User-friendly 403 |

### kubernaut_approve (5 specs)

| ID | Description | Input | Expected |
|----|-------------|-------|----------|
| UT-AF-104-001 | Approve RAR | {rar_name, decision: "Approved"} | Status patched |
| UT-AF-104-002 | Reject RAR | {rar_name, decision: "Rejected"} | Status patched |
| UT-AF-104-003 | decidedBy set | JWT user: "bob" | status.decidedBy == "bob" |
| UT-AF-104-004 | RAR not found | Missing RAR | User-friendly 404 |
| UT-AF-104-005 | workflowOverride | Override fields | Included in patch |

### kubernaut_cancel_remediation (3 specs)

| ID | Description | Input | Expected |
|----|-------------|-------|----------|
| UT-AF-105-001 | Cancels RR | {rr_id: "ns/rr-1"} | RR condition set |
| UT-AF-105-002 | Not found | Missing RR | User-friendly 404 |
| UT-AF-105-003 | Already terminal | Completed RR | Error: already done |

### kubernaut_watch (8 specs)

| ID | Description | Input | Expected |
|----|-------------|-------|----------|
| UT-AF-106-001 | Phase change events | RR transitions | Events emitted |
| UT-AF-106-002 | OwnerRef correlation | AA created for RR | AA events linked |
| UT-AF-106-003 | Chronological order | Multiple events | Sorted by time |
| UT-AF-106-004 | Terminal closes stream | RR → Completed | Stream ends |
| UT-AF-106-005 | Heartbeat 30s | Idle period | Heartbeat sent |
| UT-AF-106-006 | Context cancel | Client disconnect | Goroutine exits |
| UT-AF-106-007 | Impersonated watch | User identity | Watch uses impersonation |
| UT-AF-106-008 | 403 on watch | Forbidden namespace | User-friendly 403 |

## 7. Environmental Needs

- `k8s.io/client-go/kubernetes/fake`
- `k8s.io/client-go/testing` (reactors)
- `k8s.io/apimachinery/pkg/watch` (fake watcher)
