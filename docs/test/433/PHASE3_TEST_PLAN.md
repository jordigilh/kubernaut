# Phase 3 Test Plan: Logs + Metrics + fetch_pod_logs Parity

**Issue**: #433 — Kubernaut Agent Go Rewrite
**Phase**: 3 of 3
**Date**: 2026-03-04
**Status**: Active
**Authority**: DD-TEST-006

---

## Scope

Phase 3 adds `kubectl_logs_all_containers_grep`, `fetch_pod_logs` (HAPI parity), and `kubectl_top_pods`/`kubectl_top_nodes` (metrics). After this phase, the KA tool surface reaches full 36-tool parity with HAPI v1.2.

### Components Under Test

- `pkg/kubernautagent/tools/k8s/` — logs grep, metrics tools
- `pkg/kubernautagent/tools/logs/` — fetch_pod_logs (new package)
- `internal/kubernautagent/investigator/types.go` — PhaseToolMap update for fetch_pod_logs

---

## Unit Tests

### 3A. kubectl_logs_all_containers_grep

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-600 | kubectl_logs_all_containers_grep is registered (covers 601/602) | ✅ Pass |

### 3B. fetch_pod_logs

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-610 | fetch_pod_logs returns logs for pod with default params | ✅ Pass |
| UT-KA-433-611 | fetch_pod_logs schema validation | ✅ Pass |
| UT-KA-433-612 | fetch_pod_logs accepts filter parameters without error | ✅ Pass |
| UT-KA-433-613 | fetch_pod_logs accepts time range without error | ✅ Pass |
| UT-KA-433-614 | fetch_pod_logs accepts limit parameter | ✅ Pass |
| UT-KA-433-615 | fetch_pod_logs metadata footer | ✅ Pass |
| UT-KA-433-616 | fetch_pod_logs returns error for missing pod | ✅ Pass |
| UT-KA-433-617 | fetch_pod_logs filter functions via ApplyFilters | ✅ Pass |

### 3C. Metrics Tools

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-620 | kubectl_top_pods has valid JSON schema (namespace required) | ✅ Pass |
| UT-KA-433-621 | kubectl_top_nodes has valid JSON schema (no required params) | ✅ Pass |

### 3D. Final Tool Count

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-630 | Total registered tools = 36 (19 K8s + 2 K8s-metrics + 8 Prom + 5 custom + 1 TodoWrite + 1 fetch_pod_logs — see parity matrix) | ✅ Pass |

---

## Acceptance Criteria

1. `go build ./...` passes
2. All UT pass with `-race -count=1`
3. Full parity matrix validated: 36 KA tools
4. No new lint errors
