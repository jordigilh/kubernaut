# DD-AF-001: Pod-Based Alert Correlation for Severity Triage

**Status**: ✅ APPROVED
**Decision Date**: 2026-05-30
**Version**: 1.0
**Confidence**: 95%
**Applies To**: API Frontend (AF) — Severity Triage Pipeline

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-30 | Architecture Team | Initial design: pod-based alert correlation in Tier 1, PodResolver interface, GA readiness audit incorporated. |

---

## Context & Problem

### Current State

AF's severity triage pipeline (`pkg/apifrontend/severity/triage.go`) determines alert severity for `RemediationRequest` resources through a multi-tier pipeline:

1. **Tier 1**: Match firing Prometheus alerts against target resource labels via `labelsOverlap`
2. **Tier 1.5**: Match against Prometheus recording rules
3. **Tier 2**: Match against Alertmanager silences
4. **Tier 3**: LLM-based fallback via `NoopLLMTriager` (defaults to "medium")

The `labelsOverlap` function correlates alerts to resources by finding common label keys with equal values, excluding the `namespace` key.

### Problem

Prometheus pod-level alerts (e.g., `KubePodCrashLooping`) carry pod-specific labels:

```yaml
alertLabels:
  pod: "worker-abc-xyz"
  namespace: "default"
  severity: "critical"
  alertname: "KubePodCrashLooping"
```

The target workload's `TriageInput` carries resource-level labels:

```yaml
targetLabels:
  kind: "Deployment"
  name: "worker"
```

Zero keys overlap between these two label sets. Every pod-level alert falls through to Tier 3, which defaults severity to "medium" — preventing workflow discovery for crashloop scenarios.

### Impact

- **Crashloop workflows never trigger**: All crashloop alerts resolve as "medium" severity regardless of actual alert severity
- **Observability blind spot**: Silent failure — no errors, no logs, just wrong severity
- **User-visible symptom**: "No workflows discovered." in OCP cluster for crashloop scenarios

### Business Requirements

- **BR-AF-TRIAGE-001**: Tier 1 must correctly resolve severity for pod-level Prometheus alerts targeting workload resources (Deployment, StatefulSet, DaemonSet)

---

## Decision

### Overview

Introduce a `PodResolver` abstraction that bridges the gap between workload-level TriageInput and pod-level Prometheus alerts. The resolver uses the Kubernetes API to discover pods owned by a workload resource via its `spec.selector`, then uses those pod names as a fallback correlation path in `labelsOverlap`.

### Architecture

```
┌─────────────────────────────────────────────────────┐
│                  Triage Pipeline                     │
│                                                     │
│  TriageInput(kind=Deployment, name=worker)           │
│       │                                             │
│       ▼                                             │
│  ┌─────────────┐   ┌──────────────────────────┐     │
│  │ PodResolver │──▶│ K8s API: list pods via    │     │
│  │             │   │ spec.selector labels      │     │
│  └─────────────┘   └──────────────────────────┘     │
│       │                                             │
│       ▼                                             │
│  PodNames: [worker-abc-xyz, worker-def-123]          │
│       │                                             │
│       ▼                                             │
│  labelsOverlap(alertLabels, targetLabels,            │
│                podNameSet, targetNamespace)           │
│       │                                             │
│       ├── Path 1: Key overlap (original behavior)    │
│       │                                             │
│       └── Path 2: Pod-based correlation (NEW)        │
│           alert["pod"] ∈ podNameSet                  │
│           && alert["namespace"] == targetNamespace    │
│                                                     │
└─────────────────────────────────────────────────────┘
```

### Design Decisions

| ID | Decision | Alternatives Considered | Rationale |
|----|----------|------------------------|-----------|
| M1 | Selector-based pod resolution | ownerReference chain traversal (Pod → ReplicaSet → Deployment) | Selector-based is simpler (one API call), handles all workload types uniformly, uses native K8s machinery (`LabelSelectorAsSelector`). ownerReference requires N+1 calls and ReplicaSet intermediary logic. |
| M2 | Exact pod name matching | Label-based correlation (add kind/name labels to pods) | Exact name match is deterministic, no false positives. Label-based correlation requires mutating pod specs or relying on conventions that may not hold. |
| M3 | Namespace guard | Trust alert labels implicitly | Cross-namespace alert pollution is a real risk in multi-tenant clusters. Explicit guard (`alert.namespace == target.namespace`) prevents false matches with zero cost. |
| M4 | Graceful degradation on failure | Fail-fast with error propagation | Pod resolution is an enhancement, not a gate. If K8s API is unavailable, unsupported kind, or workload not found, return `(nil, nil)` and let existing tiers handle the alert. This preserves backward compatibility. |
| M5 | Deployment, StatefulSet, DaemonSet support | Include Job/CronJob | These three cover >95% of long-running workloads that generate crashloop alerts. Job/CronJob pods are ephemeral and rarely targeted by severity triage. Deferred to future iteration. |
| M6 | `LabelSelectorAsSelector` for matchExpressions | Manual label parsing | Native K8s function handles both `matchLabels` and `matchExpressions` correctly, including `In`, `NotIn`, `Exists`, `DoesNotExist` operators. No custom parsing needed. |
| M7 | Empty selector guard | Allow empty selector to list all pods | An empty or missing selector would list all pods in the namespace, causing false positives. Return `(nil, nil)` instead. |
| M8 | O(1) pod name lookup via `map[string]struct{}` | Linear scan via `[]string` | The pod name set is constructed once per `runTier1` invocation and checked against every firing alert. Map provides O(1) lookup vs O(n) for slice, meaningful when alert count is high. |
| M9 | `WithPodResolver` TriagerOption | Direct parameter on HandleCreateRR (52 call sites) | Option pattern has zero blast radius — no changes to any existing call site. The resolver is an internal detail of the Triager, not a concern of the caller. |
| M10 | Logger on all silent-return branches | Silent `(nil, nil)` returns | Six branches in `ResolvePodNames` previously returned `(nil, nil)` without logging, creating observability blind spots. `V(1)` debug logging makes degradation visible without noise in production. |

### Supported Workload Types

| Kind | GVR | Status |
|------|-----|--------|
| Deployment | `apps/v1/deployments` | ✅ Supported |
| StatefulSet | `apps/v1/statefulsets` | ✅ Supported |
| DaemonSet | `apps/v1/daemonsets` | ✅ Supported |
| Job | `batch/v1/jobs` | ⏳ Deferred |
| CronJob | `batch/v1/cronjobs` | ⏳ Deferred |

---

## Implementation

### Component Inventory

| Component | Location | Purpose |
|-----------|----------|---------|
| `PodResolver` interface | `pkg/apifrontend/severity/pod_resolver.go` | Abstraction for workload-to-pod resolution |
| `K8sPodResolver` | `pkg/apifrontend/severity/pod_resolver.go` | Implementation using K8s dynamic client |
| `WithPodResolver` | `pkg/apifrontend/severity/triage.go` | TriagerOption for dependency injection |
| `PodNames` field | `pkg/apifrontend/severity/types.go` | TriageInput extension for pod correlation |
| Auto-resolve in `Triage()` | `pkg/apifrontend/severity/triage.go` | Transparent pod resolution before pipeline |
| Pod fallback in `labelsOverlap` | `pkg/apifrontend/severity/triage.go` | Correlation path 2 |

### Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| K8sPodResolver | `NewTriager()` via `WithPodResolver` | `cmd/apifrontend/main.go:buildBackendDeps` | IT-AF-TRIAGE-W01 |
| Pod correlation in labelsOverlap | `runTier1()` → `labelsOverlap()` | `pkg/apifrontend/severity/triage.go` | IT-AF-TRIAGE-010 |

### Production Wiring (main.go)

The K8s dynamic client is initialized **before** the Triager in `buildBackendDeps`, enabling construction-time injection:

```go
// K8s dynamic client init (moved before Triager)
deps.k8sDynClient = resilience.NewResilientDynamicClient(inner, deps.K8sCB)

// Triager construction with PodResolver
var triagerOpts []severity.TriagerOption
if deps.k8sDynClient != nil {
    triagerOpts = append(triagerOpts, severity.WithPodResolver(
        severity.NewK8sPodResolver(deps.k8sDynClient, logger.WithName("pod-resolver")),
    ))
}
deps.Triager = severity.NewTriager(promClient, llmTriager, severityCfg,
    logger.WithName("severity-triage"), triagerOpts...)
```

When `k8sDynClient` is nil (no kubeconfig), the option is omitted and the pipeline degrades gracefully to pre-change behavior.

### Error Handling & Observability

All six degradation branches in `K8sPodResolver.ResolvePodNames` log at `V(1)` level:

| Branch | Log Message | Return |
|--------|-------------|--------|
| Unsupported kind | `"unsupported workload kind for pod resolution"` | `nil, nil` |
| Workload not found (404) | `"workload not found, skipping pod resolution"` | `nil, nil` |
| Workload get error | — | `nil, err` (propagated) |
| Missing spec or selector | `"workload has no spec.selector, skipping pod resolution"` | `nil, nil` |
| JSON marshal/unmarshal error | `"failed to parse workload selector"` | `nil, nil` |
| Invalid label selector | `"invalid label selector in workload"` | `nil, nil` |
| Empty selector string | `"workload selector is empty, skipping pod resolution"` | `nil, nil` |

The `Triage()` method logs pod resolution failure at `Info` level and continues without pod correlation:

```go
t.logger.Info("pod resolution failed, continuing without pod correlation", "error", err.Error())
```

### Concurrency Safety

- `K8sPodResolver` is stateless — all state is in method arguments
- `WithPodResolver` is used exclusively at construction time (single-threaded startup)
- No post-construction mutation of `Triager.podResolver` field
- `podNameSet` is constructed per-invocation in `runTier1` — no shared mutable state

### Performance Characteristics

| Operation | Complexity | Notes |
|-----------|-----------|-------|
| Pod resolution | 2 K8s API calls | 1 GET workload + 1 LIST pods (per `Triage()` call, only when `PodNames` empty) |
| Pod name set construction | O(n) | Once per `runTier1`, where n = number of resolved pods |
| Pod name lookup in `labelsOverlap` | O(1) | `map[string]struct{}` lookup per alert |
| Overall Tier 1 with pod correlation | O(a) | Where a = number of firing alerts, same as before |

The K8s API calls are wrapped in the circuit-breaker-protected `ResilientDynamicClient`, inheriting the existing retry and timeout configuration.

---

## Test Strategy

### Test Pyramid

| Tier | Count | Coverage |
|------|-------|----------|
| Unit Tests | 12 | PodResolver logic, labelsOverlap paths, Triager auto-resolve, workload types, selector variants |
| Integration Tests | 2 | HandleCreateRR end-to-end with envtest, wiring verification |
| E2E Tests | — | Covered by existing crashloop E2E scenario (validates workflow discovery) |

### Unit Test Matrix

| Test ID | Component | Scenario |
|---------|-----------|----------|
| UT-AF-TRIAGE-001 | labelsOverlap | Pod name correlation returns true |
| UT-AF-TRIAGE-002 | labelsOverlap | Namespace guard prevents cross-NS false match |
| UT-AF-TRIAGE-003 | labelsOverlap | Key-overlap regression (original behavior preserved) |
| UT-AF-TRIAGE-004 | K8sPodResolver | Deployment resolution via matchLabels |
| UT-AF-TRIAGE-004b | K8sPodResolver | StatefulSet resolution via matchLabels |
| UT-AF-TRIAGE-004c | K8sPodResolver | DaemonSet resolution via matchLabels |
| UT-AF-TRIAGE-004d | K8sPodResolver | Deployment with matchExpressions |
| UT-AF-TRIAGE-005 | K8sPodResolver | Unsupported kind returns nil |
| UT-AF-TRIAGE-006 | K8sPodResolver | Workload not found returns nil |
| UT-AF-TRIAGE-007 | Triager | Auto-resolves pods via WithPodResolver |
| UT-AF-TRIAGE-008 | K8sPodResolver | Empty selector guard returns nil |
| UT-AF-TRIAGE-009 | Triager | Multiple pod-correlated alerts → highest severity |

### Integration Tests

| Test ID | Scope | Description |
|---------|-------|-------------|
| IT-AF-TRIAGE-010 | End-to-end | HandleCreateRR with envtest cluster, real Deployment + Pod, mock Prometheus, pod correlation resolves severity |
| IT-AF-TRIAGE-W01 | Wiring | WithPodResolver at Triager construction matches main.go pattern |

### Race Detector

All 58 tests pass with `-race` flag enabled. No data races detected.

---

## GA Readiness Assessment

### Audit Summary

All findings from the GA readiness audit have been addressed. No tech debt deferred.

| Finding | Category | Severity | Resolution | Status |
|---------|----------|----------|------------|--------|
| DEV-001 | Plan deviation | High | Reordered main.go; eliminated SetPodResolver | ✅ Resolved |
| OBS-001 | Observability | Medium | Added V(1) logging to all 6 silent branches | ✅ Resolved |
| TST-001 | Test coverage | Medium | Added StatefulSet + DaemonSet tests | ✅ Resolved |
| TST-002 | Test coverage | Medium | Added matchExpressions test | ✅ Resolved |
| TST-003 | Test coverage | Medium | Added multi-alert highest-severity test | ✅ Resolved |
| PERF-001 | Performance | Low | O(1) map lookup in labelsOverlap | ✅ Resolved |
| CON-001 | Concurrency | High | Eliminated by DEV-001 (construction-time only) | ✅ Resolved |
| STYLE-001 | Code style | Low | Fixed import ordering | ✅ Resolved |

### Confidence Assessment

**Overall Confidence: 95%**

| Dimension | Score | Justification |
|-----------|-------|---------------|
| Correctness | 95% | All correlation paths tested; namespace guard prevents false positives; graceful degradation verified |
| Backward compatibility | 98% | Zero changes to `HandleCreateRR` signature or call sites; `WithPodResolver` is additive |
| Performance | 95% | 2 additional K8s API calls per triage (circuit-breaker-wrapped); O(1) alert matching |
| Observability | 95% | All degradation branches logged; pod resolution status visible at startup |
| Concurrency | 98% | No shared mutable state; construction-time injection only; race detector clean |
| Test coverage | 95% | 12 UT + 2 IT covering all workload types, selector variants, edge cases, and wiring |

### Known Limitations

1. **Job/CronJob not supported**: Deferred — ephemeral pods rarely targeted by severity triage
2. **Pod resolution adds 2 K8s API calls**: Mitigated by circuit breaker + graceful degradation
3. **No caching of resolved pods**: Each `Triage()` call resolves pods fresh. Acceptable for current volume; caching can be added if latency becomes a concern.

---

## References

- **Issue**: [#1336](https://github.com/jordigilh/kubernaut/issues/1336)
- **PR**: [#1328](https://github.com/jordigilh/kubernaut/pull/1328)
- **Branch**: `feat/af-severity-pod-correlation`
- **Related**: DD-HAPI-017 (Three-Step Workflow Discovery), ADR-019 (Circuit Breaker Strategy)
