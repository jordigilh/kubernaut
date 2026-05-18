# Test Plan: NL Signal Intake Tools (#52)

**Test Plan Identifier:** TP-AF-052
**Issue:** #52
**Version:** 1.0
**Date:** 2026-05-08
**Status:** Draft

---

## 1. Introduction

This test plan validates the six Kubernetes observability and remediation tools that enable natural-language-driven signal intake (ARCHITECTURE.md Section 5, Flow 1). These tools allow the agent to inspect cluster state, identify issues, and create remediation requests through conversational interaction.

### 1.1 Scope

- `af_list_events`: List K8s Events filtered by namespace (impersonated)
- `af_get_pods`: Get pod status summaries with container states (impersonated)
- `af_get_workloads`: Get Deployment/StatefulSet health (impersonated)
- `af_resolve_owner`: Walk owner references to root workload (impersonated)
- `af_check_existing_rr`: Check for duplicate RR by fingerprint (AF SA)
- `af_create_rr`: Create RemediationRequest with singleflight dedup (AF SA)

### 1.2 Out of Scope

- Session management (covered by TP-AF-056)
- Existing CRD tools (kubernaut_list_remediations, etc.) — already tested
- Full E2E with real cluster (covered by PR-D E2E CI)
- LLM model integration (PR5)

### 1.3 References

- ARCHITECTURE.md Section 5: Agent Flows (Flow 1 — NL Signal Intake)
- ARCHITECTURE.md Section 7: Observability (af_tool_calls_total)
- ADR-013: JWT Forwarding (impersonation for read tools)
- FedRAMP Controls: SI-10 (input validation), AC-3 (access enforcement)
- `k8s.io/apimachinery/pkg/util/validation` for RFC 1123 compliance

---

## 2. Test Items

| Item | Package | File |
|------|---------|------|
| `HandleListEvents` | `internal/tools` | `af_list_events.go` |
| `HandleGetPods` | `internal/tools` | `af_get_pods.go` |
| `HandleGetWorkloads` | `internal/tools` | `af_get_workloads.go` |
| `HandleResolveOwner` | `internal/tools` | `af_resolve_owner.go` |
| `HandleCheckExistingRR` | `internal/tools` | `af_check_existing_rr.go` |
| `HandleCreateRR` | `internal/tools` | `af_create_rr.go` |
| `validate.Namespace` | `internal/validate` | `k8s.go` |
| `validate.ResourceName` | `internal/validate` | `k8s.go` |

---

## 3. Business Acceptance Criteria

| ID | Criterion | Source | Priority |
|----|-----------|--------|----------|
| BAC-52-001 | `af_list_events` returns K8s Events filtered by namespace | ARCH §5.1 | P0 |
| BAC-52-002 | `af_get_pods` returns pod status summary with container states | ARCH §5.1 | P0 |
| BAC-52-003 | `af_get_workloads` returns Deployment/SS health (replicas, conditions) | ARCH §5.1 | P0 |
| BAC-52-004 | `af_resolve_owner` walks owner refs to root workload (max 10 hops) | ARCH §5.1 | P0 |
| BAC-52-005 | `af_check_existing_rr` detects duplicate RR by namespace+kind+name | ARCH §5.2 | P0 |
| BAC-52-006 | `af_create_rr` creates RR CRD with spec (targetRef, severity, description) | ARCH §5.2 | P0 |
| BAC-52-007 | `af_create_rr` uses singleflight per fingerprint to prevent duplicates | ARCH §5.2 | P0 |
| BAC-52-008 | Tool outputs are trimmed to max 4KB for model context safety | ARCH §7 | P1 |
| BAC-52-009 | All namespace/name inputs validated as RFC 1123 before K8s API call | SI-10 | P0 |
| BAC-52-010 | Nil K8s client returns ErrK8sUnavailable (circuit breaker open) | Resilience | P0 |

---

## 4. Test Cases

### 4.1 af_list_events (8 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-052-001 | Happy path: returns events in namespace | BAC-52-001 | P0 |
| UT-AF-052-002 | Empty namespace rejected with validation error | BAC-52-009 | P0 |
| UT-AF-052-003 | Path traversal namespace (../../etc) rejected | BAC-52-009 | P0 |
| UT-AF-052-004 | Namespace not found returns empty list (not error) | BAC-52-001 | P1 |
| UT-AF-052-005 | Large result (100+ events) is trimmed to 4KB | BAC-52-008 | P1 |
| UT-AF-052-006 | Nil client returns ErrK8sUnavailable | BAC-52-010 | P0 |
| UT-AF-052-007 | Concurrent calls are safe under -race | Concurrency | P1 |
| UT-AF-052-008 | Tool increments af_tool_calls_total metric | Observability | P0 |

### 4.2 af_get_pods (8 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-052-010 | Happy path: returns pod summaries with container states | BAC-52-002 | P0 |
| UT-AF-052-011 | Empty namespace rejected | BAC-52-009 | P0 |
| UT-AF-052-012 | Unicode namespace rejected | BAC-52-009 | P0 |
| UT-AF-052-013 | Max-length+1 namespace rejected | BAC-52-009 | P1 |
| UT-AF-052-014 | Large result trimmed to 4KB | BAC-52-008 | P1 |
| UT-AF-052-015 | Nil client returns ErrK8sUnavailable | BAC-52-010 | P0 |
| UT-AF-052-016 | Concurrent calls safe | Concurrency | P1 |
| UT-AF-052-017 | Filters by label selector when provided | BAC-52-002 | P1 |

### 4.3 af_get_workloads (8 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-052-020 | Happy path: returns Deployments and StatefulSets | BAC-52-003 | P0 |
| UT-AF-052-021 | Empty namespace rejected | BAC-52-009 | P0 |
| UT-AF-052-022 | Specific workload name filters results | BAC-52-003 | P1 |
| UT-AF-052-023 | Includes replica counts and conditions | BAC-52-003 | P0 |
| UT-AF-052-024 | Large result trimmed | BAC-52-008 | P1 |
| UT-AF-052-025 | Nil client returns ErrK8sUnavailable | BAC-52-010 | P0 |
| UT-AF-052-026 | Concurrent calls safe | Concurrency | P1 |
| UT-AF-052-027 | Path traversal in workload name rejected | BAC-52-009 | P0 |

### 4.4 af_resolve_owner (10 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-052-030 | Happy path: Pod -> ReplicaSet -> Deployment | BAC-52-004 | P0 |
| UT-AF-052-031 | Single hop: Pod with no owner returns Pod itself | BAC-52-004 | P0 |
| UT-AF-052-032 | Max depth (10 hops) stops traversal | BAC-52-004 | P0 |
| UT-AF-052-033 | Circular ownership detected and stopped | BAC-52-004 | P1 |
| UT-AF-052-034 | Invalid namespace rejected | BAC-52-009 | P0 |
| UT-AF-052-035 | Invalid resource name rejected | BAC-52-009 | P0 |
| UT-AF-052-036 | Nil client returns ErrK8sUnavailable | BAC-52-010 | P0 |
| UT-AF-052-037 | Owner reference to non-existent resource is handled | BAC-52-004 | P1 |
| UT-AF-052-038 | Concurrent calls safe | Concurrency | P1 |
| UT-AF-052-039 | Returns full chain in output | BAC-52-004 | P0 |

### 4.5 af_check_existing_rr (8 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-052-040 | Happy path: finds existing RR by fingerprint labels | BAC-52-005 | P0 |
| UT-AF-052-041 | No match returns exists=false | BAC-52-005 | P0 |
| UT-AF-052-042 | Empty namespace rejected | BAC-52-009 | P0 |
| UT-AF-052-043 | Empty kind rejected | BAC-52-009 | P0 |
| UT-AF-052-044 | Empty name rejected | BAC-52-009 | P0 |
| UT-AF-052-045 | Nil client returns ErrK8sUnavailable | BAC-52-010 | P0 |
| UT-AF-052-046 | Concurrent calls safe | Concurrency | P1 |
| UT-AF-052-047 | Terminal-phase RRs excluded from duplicate check | BAC-52-005 | P1 |

### 4.6 af_create_rr (12 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-052-050 | Happy path: creates RR with correct spec | BAC-52-006 | P0 |
| UT-AF-052-051 | Singleflight dedup: concurrent identical creates yield one RR | BAC-52-007 | P0 |
| UT-AF-052-052 | Check-then-create: existing RR returns AlreadyExists | BAC-52-005 | P0 |
| UT-AF-052-053 | Empty namespace rejected | BAC-52-009 | P0 |
| UT-AF-052-054 | Empty kind rejected | BAC-52-009 | P0 |
| UT-AF-052-055 | Empty name rejected | BAC-52-009 | P0 |
| UT-AF-052-056 | Path traversal in description sanitized | BAC-52-009 | P1 |
| UT-AF-052-057 | Nil client returns ErrK8sUnavailable | BAC-52-010 | P0 |
| UT-AF-052-058 | RR spec includes severity field | BAC-52-006 | P0 |
| UT-AF-052-059 | RR spec includes reportedBy from username | BAC-52-006 | P0 |
| UT-AF-052-060 | Max-length+1 description truncated (not rejected) | BAC-52-008 | P1 |
| UT-AF-052-061 | Concurrent different fingerprints create independent RRs | BAC-52-007 | P0 |

---

## 5. Pass/Fail Criteria

- All 54 tests pass with `-race` flag
- Coverage >= 80% per tool file
- `golangci-lint run` reports 0 errors on new files
- No `panic()` in production code paths
- All validation errors include field name for SRE diagnosis
- `go mod tidy` clean

---

## 6. Test Environment

- Go 1.25.6
- `k8s.io/client-go/dynamic/fake` for K8s API simulation
- `golang.org/x/sync/singleflight` for deduplication
- Ginkgo v2 + Gomega test framework (ADR-015)
- `-race` flag mandatory for concurrency tests

---

## 7. Design Notes

### 7.1 Client Split

| Tool | Client | Rationale |
|------|--------|-----------|
| af_list_events | Impersonated | User-scoped RBAC enforcement |
| af_get_pods | Impersonated | User-scoped RBAC enforcement |
| af_get_workloads | Impersonated | User-scoped RBAC enforcement |
| af_resolve_owner | Impersonated | User-scoped RBAC enforcement |
| af_check_existing_rr | AF SA | CRD access not granted to end users |
| af_create_rr | AF SA | CRD write requires privileged SA |

### 7.2 Output Trimming

All tool handlers trim output to 4KB max (consistent with `session.TrimToolResult` threshold) to prevent model context overflow. Trimming appends `"...(truncated)"` marker.

### 7.3 Singleflight Dedup (af_create_rr)

```
fingerprint = sha256(namespace + "/" + kind + "/" + name)

singleflight.Do(fingerprint, func() {
    1. af_check_existing_rr (label query)
    2. if exists -> return AlreadyExists
    3. create RR
})
```

Multi-replica safety: K8s API-level conflict + label check on retry catches cross-pod races.
