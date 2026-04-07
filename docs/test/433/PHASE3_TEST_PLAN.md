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
| UT-KA-433-601 | kubectl_logs_all_containers_grep returns grep-filtered logs from all containers | |
| UT-KA-433-602 | kubectl_logs_all_containers_grep has valid JSON schema | |

### 3B. fetch_pod_logs

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-610 | fetch_pod_logs returns logs for pod with default params | |
| UT-KA-433-611 | fetch_pod_logs filters by since_seconds time range | |
| UT-KA-433-612 | fetch_pod_logs filters lines by regex include pattern | |
| UT-KA-433-613 | fetch_pod_logs excludes lines by regex exclude pattern | |
| UT-KA-433-614 | fetch_pod_logs merges current + previous logs when previous=true | |
| UT-KA-433-615 | fetch_pod_logs returns structured metadata (pod, namespace, container, line_count) | |
| UT-KA-433-616 | fetch_pod_logs has valid JSON schema | |
| UT-KA-433-617 | fetch_pod_logs is registered in PhaseRCA | |

### 3C. Metrics Tools

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-620 | kubectl_top_pods has valid JSON schema (namespace required) | |
| UT-KA-433-621 | kubectl_top_nodes has valid JSON schema (no required params) | |

### 3D. Final Tool Count

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-630 | Total registered tools = 36 (19 K8s + 2 K8s-metrics + 8 Prom + 5 custom + 1 TodoWrite + 1 fetch_pod_logs — see parity matrix) | |

---

## Acceptance Criteria

1. `go build ./...` passes
2. All UT pass with `-race -count=1`
3. Full parity matrix validated: 36 KA tools
4. No new lint errors
