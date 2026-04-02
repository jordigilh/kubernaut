# Phase 2 Test Plan: K8s Core Tools + Kind Expansion + Summarizer

**Issue**: #433 — Kubernaut Agent Go Rewrite
**Phase**: 2 of 3
**Date**: 2026-03-04
**Status**: Active
**Authority**: DD-TEST-006

---

## Scope

Phase 2 adds 7 missing K8s core tools (cluster-scoped list, find, YAML, memory requests, jq_query, count), expands supported resource kinds to match HAPI, and wires the `llm_summarize` post-processing into large-output tools.

### Components Under Test

- `pkg/kubernautagent/tools/k8s/` — 7 new tools + expanded kind registry
- `pkg/kubernautagent/tools/summarizer/` — Summarizer wiring into tool execution
- `pkg/kubernautagent/tools/registry/` — Updated tool count
- `internal/kubernautagent/investigator/` — Summarizer integration

---

## Test Scenario Naming Convention

Format: `{TIER}-KA-433-{SEQUENCE}`

---

## Unit Tests

### 2A. New K8s Core Tools

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-501 | kubectl_get_by_kind_in_cluster lists resources across all namespaces | |
| UT-KA-433-502 | kubectl_get_by_kind_in_cluster has valid JSON schema (kind required) | |
| UT-KA-433-503 | kubectl_find_resource searches by label selector in namespace | |
| UT-KA-433-504 | kubectl_find_resource has valid JSON schema (kind, namespace, label_selector required) | |
| UT-KA-433-505 | kubectl_get_yaml returns resource as YAML string | |
| UT-KA-433-506 | kubectl_get_yaml has valid JSON schema (kind, name, namespace required) | |
| UT-KA-433-507 | kubectl_get_memory_requests returns container memory requests/limits for pod | |
| UT-KA-433-508 | kubectl_get_deployment_memory_requests returns memory for all pods in deployment | |
| UT-KA-433-509 | kubernetes_jq_query applies jq expression to resource JSON | |
| UT-KA-433-510 | kubernetes_jq_query has valid JSON schema (kind, namespace, jq_expression required) | |
| UT-KA-433-511 | kubernetes_count counts resources matching kind in namespace | |
| UT-KA-433-512 | kubernetes_count with jq_filter applies post-filter before counting | |

### 2B. Expanded Resource Kinds

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-520 | kindToGVR resolves ReplicaSet to apps/replicasets | |
| UT-KA-433-521 | kindToGVR resolves StatefulSet to apps/statefulsets | |
| UT-KA-433-522 | kindToGVR resolves DaemonSet to apps/daemonsets | |
| UT-KA-433-523 | kindToGVR resolves Node to nodes (no namespace) | |
| UT-KA-433-524 | kindToGVR resolves Namespace to namespaces | |
| UT-KA-433-525 | kindToGVR resolves PodDisruptionBudget to policy/poddisruptionbudgets | |
| UT-KA-433-526 | kindToGVR resolves HorizontalPodAutoscaler to autoscaling/horizontalpodautoscalers | |
| UT-KA-433-527 | kindToGVR resolves NetworkPolicy to networking.k8s.io/networkpolicies | |
| UT-KA-433-528 | kindToGVR resolves Secret to secrets | |
| UT-KA-433-529 | kindToGVR resolves Job to batch/jobs | |
| UT-KA-433-530 | kindToGVR resolves CronJob to batch/cronjobs | |
| UT-KA-433-531 | getResource supports all expanded kinds | |
| UT-KA-433-532 | listResources supports all expanded kinds | |

### 2C. Summarizer Integration

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-540 | Summarizer passes through output below threshold | |
| UT-KA-433-541 | Summarizer calls LLM for output above threshold | |
| UT-KA-433-542 | Summarizer error returns original text (graceful fallback) | |

### 2D. AllToolNames Count

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-550 | AllToolNames contains 18 entries (11 existing + 7 new) | |

---

## Acceptance Criteria

1. `go build ./...` passes
2. All UT pass with `-race -count=1`
3. 18 K8s tools registered
4. All expanded kinds resolve correctly
5. jq_query and count tools use gojq
6. No new lint errors
