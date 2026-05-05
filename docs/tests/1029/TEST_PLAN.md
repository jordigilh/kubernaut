# Test Plan: Gateway Dynamic Owner Resolution + Batch-Independent Alert Processing

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1029-v1.0
**Feature**: Replace static kind maps with dynamic API discovery, add multi-candidate scoring with existence validation, and process alerts independently within batches
**Version**: 1.0
**Created**: 2026-05-04
**Author**: AI Assistant
**Status**: Review
**Branch**: `fix/1029-1032-dynamic-owner-resolver`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the replacement of the gateway's hardcoded resource resolution
maps (`resourceCandidates`, `kindToGroup`) with a fully dynamic API resource registry
backed by Kubernetes API discovery. It also validates batch-independent alert processing
(#1032) which changes the adapter interface to process each alert in an AlertManager
webhook independently rather than returning only the first successful match.

The change modifies the most critical path in the gateway — signal ingestion and
fingerprinting. Every signal that flows through the system is affected. This plan
provides defense-in-depth assurance that:

1. All 12 previously static label-to-kind mappings produce identical results via discovery
2. OpenShift CRD kinds (BuildConfig, DeploymentConfig, Route) are correctly resolved
3. Cross-namespace alerts resolve to the correct resource namespace
4. Multi-alert webhook batches process every alert independently
5. Owner resolution uses the registry for GVR lookup with no static maps remaining

### 1.2 Objectives

1. **Discovery Parity**: 11 of the 12 previously static `resourceCandidates` label-to-kind mappings produce identical results via the dynamic registry. The 12th (`job_name` -> `Job`) is intentionally dropped — `job_name` is a Prometheus scrape label, not a K8s `APIResource.SingularName`. The correct mapping is `job` -> `Job`.
2. **CRD Recognition**: Alert labels matching OpenShift CRD singular names (`buildconfig`, `deploymentconfig`, `route`) produce correctly-typed `ResourceIdentifier` values
3. **Existence Validation**: Multi-candidate label matches are disambiguated by verifying resource existence in the cluster, eliminating false positives
4. **Cross-Namespace**: Alerts with `exported_namespace != namespace` resolve to the resource's home namespace via existence checks in both namespaces
5. **Batch Independence**: Optional `BatchParser` interface with `ParseBatch()` returns `[]*NormalizedSignal` — one signal per resolved alert. Failed alerts are logged and counted but do not fail the batch. Handler returns HTTP 207 Multi-Status for batch adapters.
6. **Owner Resolution**: `ResolveTopLevelOwner` uses registry-backed GVR lookup. CRD kinds are treated as top-level owners (no traversal). Static `kindToGroup` is removed
7. **Fail-Fast Startup**: Gateway refuses to start if API discovery fails at startup
8. **Thread Safety**: Registry refresh under `RWMutex` does not race with concurrent signal processing
9. **Observability**: New metrics (`gateway_owner_resolution_total`, `gateway_signals_parse_dropped_total`) and audit event (`gateway.signal.dropped`) are emitted correctly
10. **Backward Compatibility**: All pre-existing gateway tests pass. No fingerprint regression for vanilla K8s alerts

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/gateway/...` |
| Integration test pass rate | 100% | `go test ./test/integration/gateway/...` |
| Unit-testable code coverage (resource_registry.go) | >=80% | `go test -coverprofile` on registry code |
| Unit-testable code coverage (prometheus_adapter.go changes) | >=80% | `go test -coverprofile` on extraction/scoring logic |
| Unit-testable code coverage (owner_resolver.go changes) | >=80% | `go test -coverprofile` on GVR lookup logic |
| Integration-testable code coverage (wiring + handler) | >=80% | `go test -coverprofile` on server signal handler |
| Race detector clean | 0 warnings | `go test -race ./test/unit/gateway/...` |
| Backward compatibility | 0 regressions | All pre-existing tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-GATEWAY-001**: Signal ingestion from AlertManager webhooks
- **BR-GATEWAY-004**: Cross-adapter deduplication consistency via fingerprinting
- **BR-GATEWAY-181**: Severity and event type pass-through for normalized signals
- **BR-GATEWAY-184**: Resource extraction priority (controllers > workloads > leaf)
- Issue #1029: Gateway owner-resolver: static kind map breaks OpenShift CRD scenarios
- Issue #1032: Gateway silently drops entire alert batch when one alert fails owner resolution

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [100 Go Mistakes and How to Avoid Them](https://100go.co/) — Refactoring checklist
- [Implementation Plan](../../../.cursor/plans/gateway_dynamic_owner_resolver_353db9cf.plan.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Fingerprint regression: changing how `extractTargetResource` resolves kind/name alters the fingerprint for existing alerts, breaking deduplication | Duplicate RRs created for the same alert | High | UT-GW-1029-001 through 012 | Parity tests: all 12 old static mappings produce identical results via discovery |
| R2 | Discovery RBAC denied at startup: gateway fails to start in environments where `system:discovery` is restricted | Gateway unavailable | Medium | UT-GW-1029-013 | Fail-fast test validates clear error message |
| R3 | Existence check 403 for specific API groups: RBAC denies `GET` on a CRD resource type | Candidate silently skipped; may resolve to wrong resource | Medium | UT-GW-1029-017 | 403 is treated as "candidate not found" with warning log |
| R4 | Concurrent registry refresh races with signal processing | Stale or partial map read | Low | UT-GW-1029-015 | RWMutex + atomic map swap; dedicated concurrency test |
| R5 | Batch processing changes HTTP response shape; upstream consumers break | AlertManager or monitoring expects old response format | Medium | IT-GW-1029-005, IT-GW-1032-001 | AlertManager ignores response bodies; integration test validates new shape |
| R6 | Cross-namespace existence checks return false positive (resource exists in both namespaces) | Resolves to wrong namespace | Low | UT-GW-1029-020 | `exported_namespace` takes precedence; test validates priority |
| R7 | CRD kind treated as top-level but actually has ownerReferences | Owner chain traversal skipped; fingerprint is less specific | Low | UT-GW-1029-024 | Acceptable for v1.4; documented in plan |
| R8 | `LabelFilter` removal breaks monitoring metadata filtering for vanilla K8s | Monitoring infrastructure pods/services incorrectly matched as targets | Medium | UT-GW-1029-019, UT-GW-1029-020 | Existence validation replaces heuristic: monitoring infra resources typically don't exist in alert namespaces |
| R9 | Adversarial input: label keys with special characters cause panic or injection | Gateway crash or unexpected K8s API calls | Low | UT-GW-1029-033 | Label keys sanitized; only alphanumeric + underscore/hyphen match APIResource.SingularName |
| R10 | Resource exhaustion: alert with many labels causes unbounded API calls | API server overload, gateway latency spike | Medium | UT-GW-1029-034 | Candidate count bounded by labels that match discovered SingularNames; typically 1-3 per alert |
| R11 | Memory leak: existence cache grows unbounded under alert storms | OOM kill | Medium | UT-GW-1029-036 | TTL expiration + cache cleared on registry refresh |
| R12 | Post-startup discovery refresh failure leaves registry in broken state | All signals resolve to Unknown until next refresh | High | UT-GW-1029-037 | Refresh failure preserves previous good map; logged as error |

### 3.1 Risk-to-Test Traceability

- **R1 (HIGH)**: Covered by UT-GW-1029-001 through UT-GW-1029-012 (parity suite)
- **R2 (MEDIUM)**: Covered by UT-GW-1029-014 (fail-fast startup)
- **R3 (MEDIUM)**: Covered by UT-GW-1029-018 (403 graceful skip)
- **R4 (LOW)**: Covered by UT-GW-1029-016, UT-GW-1029-038 (concurrent refresh + partial map)
- **R5 (MEDIUM)**: Covered by IT-GW-1029-005, IT-GW-1032-001 (batch response)
- **R6 (LOW)**: Covered by UT-GW-1029-021, UT-GW-1029-022 (cross-ns precedence)
- **R7 (LOW)**: Covered by UT-GW-1029-025 (CRD as top-level)
- **R8 (MEDIUM)**: Covered by UT-GW-1029-019, UT-GW-1029-020 (monitoring infra filtered by existence)

---

## 4. Scope

### 4.1 Features to be Tested

- **API Resource Registry** (`pkg/gateway/adapters/resource_registry.go`): Discovery-based label-to-kind and kind-to-GVR resolution with fail-fast startup, tier-based priority, periodic refresh, existence validation cache
- **Multi-Candidate Scoring** (`pkg/gateway/adapters/prometheus_adapter.go`): Replace `extractTargetResource` static scan with registry-backed multi-candidate scoring and existence validation
- **Owner Resolution** (`pkg/gateway/adapters/owner_resolver.go`): Replace `kindToGroup` static map with registry-backed GVR lookup; Option C for CRD kinds
- **Cross-Namespace Resolution** (`pkg/gateway/adapters/prometheus_adapter.go`): `exported_namespace` precedence with dual-namespace existence checks
- **Batch-Independent Processing** (`pkg/gateway/adapters/prometheus_adapter.go`, `pkg/gateway/adapters/adapter.go`, `pkg/gateway/server.go`): Optional `BatchParser` interface with `ParseBatch()` returns `[]*NormalizedSignal`; handler processes each signal independently with HTTP 207 Multi-Status response
- **Observability** (`pkg/gateway/metrics/metrics.go`): New metrics and audit event
- **Wiring** (`cmd/gateway/main.go`): Discovery client, registry construction, refresh loop

### 4.2 Features Not to be Tested

- **SignalProcessing `getGVKForKind()`** (`pkg/signalprocessing/ownerchain/builder.go`): Known duplicate static map; separate issue, not on the signal ingestion path
- **Shared scope manager** (`pkg/shared/scope/manager.go`): Static map copy; separate cleanup
- **RBAC for production Operator deployment**: Managed by operator team; out of scope
- **KubernetesEventAdapter owner resolution**: Already uses `OwnerResolver` interface; only signature change (`*NormalizedSignal` -> `[]*NormalizedSignal`)
- **Performance benchmarking**: Deferred to v1.5; existence cache provides bounded API calls

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Parity tests for all 12 static candidates | Highest risk area — fingerprint regression affects every existing deployment |
| Mock discovery client in unit tests | Discovery API is external I/O; unit tests use a fake that returns controlled `APIResourceList` |
| Real envtest cluster for integration tests | Existence validation requires real K8s API; envtest provides authentic API server |
| Install test CRD in integration tests | Validates dynamic discovery of unknown kinds without requiring OCP |
| No `LabelFilter` replacement tests | Existence validation subsumes heuristic filtering; old `LabelFilter` tests are removed |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (registry construction, tier sorting, label scanning, existence cache, multi-candidate scoring, GVR lookup, batch aggregation)
- **Integration**: >=80% of integration-testable code (real discovery, existence checks against envtest, handler loop with real K8s, wiring validation)
- **E2E**: Container contract only — verify that a multi-alert webhook through a full Kind cluster gateway creates the expected RRs. Deferred to CI if CRD installation in Kind is required.

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:

- **Unit tests**: Registry logic, scoring algorithm, tier priority, existence cache TTL, cross-namespace precedence, batch aggregation, fail-fast, concurrent refresh
- **Integration tests**: Real discovery against envtest, CRD discovery, handler loop creating RRs, batch response aggregation, metrics emission

### 5.3 Business Outcome Quality Bar

Tests validate business outcomes — not code paths:

- "Alert with `buildconfig: webapp` label creates an RR with `TargetResource.Kind = BuildConfig`" (not "extractTargetResource is called")
- "Multi-alert webhook with 3 alerts creates 3 independent signals" (not "Parse returns a slice of length 3")
- "Gateway refuses to start when discovery is unavailable" (not "NewAPIResourceRegistry returns an error")

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage meets >=80% threshold
4. No regressions in existing gateway test suites
5. All 12 parity tests pass: 11 produce identical results to the static map; 1 (`job_name`) intentionally dropped with `job` replacing it
6. Race detector clean on all new tests

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier
3. Any pre-existing gateway test regresses
4. Fingerprint parity broken for any of the 11 preserved static candidates (the 12th, `job_name`, is intentionally dropped)

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- Code does not compile — unit tests cannot execute
- envtest binary unavailable — integration tests blocked
- Discovery mock does not return expected `APIResourceList` structure — parity tests invalid

**Resume testing when**:

- Build fixed and green
- envtest restored
- Mock updated to match `client-go` discovery interface

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/gateway/adapters/resource_registry.go` (NEW) | `NewAPIResourceRegistry`, `LabelToKind`, `KindToGVR`, `TierForKind`, `Refresh`, `ExistenceCheck` | ~250 |
| `pkg/gateway/adapters/prometheus_adapter.go` (MODIFIED) | `extractTargetResource` (replaced), `extractNamespace` (updated), `Parse` (batch) | ~150 changed |
| `pkg/gateway/adapters/owner_resolver.go` (MODIFIED) | `ResolveTopLevelOwner` (registry lookup), `OwnerChainCacheObjects` (registry-backed) | ~80 changed |
| `pkg/gateway/adapters/adapter.go` (MODIFIED) | `SignalAdapter.Parse` signature | ~5 changed |
| `pkg/gateway/metrics/metrics.go` (MODIFIED) | New metric registrations | ~20 added |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/gateway/server.go` (MODIFIED) | `readParseValidateSignal` (batch), `createAdapterHandler` (loop) | ~60 changed |
| `cmd/gateway/main.go` (MODIFIED) | Discovery wiring, registry construction, refresh loop | ~40 added |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/1029-1032-dynamic-owner-resolver` HEAD | Branch from v1.4.0-rc5 |
| client-go discovery | v0.35.3 | Same version used by KA `BuildKindIndex` |
| envtest | controller-runtime v0.20.x | Existing gateway integration test infrastructure |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-GATEWAY-001 | Signal ingestion from AlertManager | P0 | Unit | UT-GW-1029-001..012 | Pass |
| BR-GATEWAY-001 | Signal ingestion from AlertManager | P0 | Integration | IT-GW-1029-001..003 | Pass |
| BR-GATEWAY-004 | Fingerprint deduplication consistency | P0 | Unit | UT-GW-1029-001..012 | Pass |
| BR-GATEWAY-004 | Fingerprint deduplication consistency | P0 | Integration | IT-GW-1029-004 | Pass |
| BR-GATEWAY-184 | Resource extraction priority | P0 | Unit | UT-GW-1029-015 | Pass |
| BR-GATEWAY-1029 | Prometheus heuristic labels intentionally dropped | P0 | Unit | UT-GW-1029-013 | Pass |
| BR-GATEWAY-1029 | Fail-fast startup on discovery failure | P0 | Unit | UT-GW-1029-014 | Pass |
| BR-GATEWAY-1029 | Dynamic CRD label recognition | P0 | Unit | UT-GW-1029-017..020 | Pass |
| BR-GATEWAY-1029 | Dynamic CRD label recognition | P0 | Integration | IT-GW-1029-002 | Pass |
| BR-GATEWAY-1029 | Cross-namespace resolution | P0 | Unit | UT-GW-1029-021..023 | Pass |
| BR-GATEWAY-1029 | Cross-namespace resolution | P0 | Integration | IT-GW-1029-003 | Pass |
| BR-GATEWAY-1029 | Owner resolution for CRD kinds | P0 | Unit | UT-GW-1029-024..027 | Pass |
| BR-GATEWAY-1032 | Batch-independent alert processing | P0 | Unit | UT-GW-1032-001..005 | Pass |
| BR-GATEWAY-1032 | Batch-independent alert processing | P0 | Integration | IT-GW-1032-001..003 | Pass |
| BR-GATEWAY-1029 | Registry thread safety | P1 | Unit | UT-GW-1029-016, 038 | Pass |
| BR-GATEWAY-1029 | Observability (new metrics) | P1 | Unit | UT-GW-1029-028..029 | Pass |
| BR-GATEWAY-1029 | Observability (audit event) | P1 | Integration | IT-GW-1029-006 | Pass |
| BR-GATEWAY-1029 | Existence cache lifecycle | P1 | Unit | UT-GW-1029-030..032 | Pass |
| BR-SECURITY-1029 | Adversarial input resilience | P1 | Unit | UT-GW-1029-033..040 | Pass |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: `GW` (Gateway)
- **ISSUE**: `1029` or `1032`
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `resource_registry.go`, `prometheus_adapter.go` (extraction/scoring), `owner_resolver.go` (GVR lookup), `adapter.go` (signature), `metrics.go` (registration). Target: >=80% per file.

#### Registry Construction & Discovery

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-GW-1029-001` | Registry built from discovery maps `deployment` label to `Deployment` kind (parity with static #1) | Pass |
| `UT-GW-1029-002` | Registry maps `statefulset` label to `StatefulSet` kind (parity #2) | Pass |
| `UT-GW-1029-003` | Registry maps `daemonset` label to `DaemonSet` kind (parity #3) | Pass |
| `UT-GW-1029-004` | Registry maps `replicaset` label to `ReplicaSet` kind (parity #4) | Pass |
| `UT-GW-1029-005` | Registry maps `pod` label to `Pod` kind (parity #5) | Pass |
| `UT-GW-1029-006` | Registry maps `node` label to `Node` kind (parity #6) | Pass |
| `UT-GW-1029-007` | Registry maps `service` label to `Service` kind (parity #7) | Pass |
| `UT-GW-1029-008` | Registry maps `cronjob` label to `CronJob` kind (parity #8) | Pass |
| `UT-GW-1029-009` | Registry maps `job` label to `Job` kind (parity #9 — old static map used `job_name` which is a Prometheus scrape label, NOT a K8s SingularName; dynamic discovery correctly uses `job`) | Pass |
| `UT-GW-1029-010` | Registry maps `horizontalpodautoscaler` label to `HorizontalPodAutoscaler` kind (parity #10) | Pass |
| `UT-GW-1029-011` | Registry maps `poddisruptionbudget` label to `PodDisruptionBudget` kind (parity #11) | Pass |
| `UT-GW-1029-012` | Registry maps `persistentvolumeclaim` label to `PersistentVolumeClaim` kind (parity #12) | Pass |
| `UT-GW-1029-013` | Registry does NOT map `job_name` label to any kind (Prometheus scrape label, not a K8s SingularName; verifies old static heuristic is intentionally dropped) | Pass |
| `UT-GW-1029-014` | Gateway startup fails with clear error when discovery is unavailable (fail-fast) | Pass |
| `UT-GW-1029-015` | Tier-based priority: when labels match both `Deployment` (Tier 1) and `Pod` (Tier 3), `Deployment` wins | Pass |
| `UT-GW-1029-016` | Concurrent goroutines reading registry while refresh writes produce no data races (race detector clean) | Pass |

#### CRD Recognition & Existence Validation

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-GW-1029-017` | Alert with `buildconfig: webapp` label resolves to `Kind: BuildConfig` when BuildConfig exists in namespace | Pass |
| `UT-GW-1029-018` | Existence check returning 403 Forbidden skips candidate gracefully (no panic, warning logged) | Pass |
| `UT-GW-1029-019` | Alert with `service: router-internal-default` where Service does not exist in alert namespace is filtered out | Pass |
| `UT-GW-1029-020` | Alert with both `route: storefront` (exists) and `service: router-internal-default` (not exists) resolves to Route | Pass |

#### Cross-Namespace Resolution

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-GW-1029-021` | Alert with `exported_namespace: demo-route` and `namespace: demo-route` where Route exists in `exported_namespace` resolves to `demo-route` | Pass |
| `UT-GW-1029-022` | Alert with `exported_namespace: ns-a` and `namespace: ns-b` checks `exported_namespace` first; if found, does not check `namespace` | Pass |
| `UT-GW-1029-023` | Alert with `exported_namespace: ns-a` where no candidate exists falls back to `namespace` | Pass |

#### Owner Resolution

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-GW-1029-024` | `ResolveTopLevelOwner` for `Pod` uses registry-provided GVR (no static `kindToGroup`) | Pass |
| `UT-GW-1029-025` | `ResolveTopLevelOwner` for CRD kind `BuildConfig` returns identity (no traversal — Option C) | Pass |
| `UT-GW-1029-026` | `ResolveTopLevelOwner` for CRD kind `Route` returns identity (no traversal — Option C) | Pass |
| `UT-GW-1029-027` | `OwnerChainCacheObjects` returns only core/apps/batch kinds from registry (no CRDs in informer cache) | Pass |

#### Observability

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-GW-1029-028` | `gateway_owner_resolution_total{kind=BuildConfig,outcome=success}` incremented on successful CRD resolution | Pass |
| `UT-GW-1029-029` | `gateway_signals_parse_dropped_total{reason=no_candidate,adapter=prometheus}` incremented when no label matches any resource | Pass |

#### Existence Cache

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-GW-1029-030` | Existence check result cached; second call within TTL does not hit API | Pass |
| `UT-GW-1029-031` | Cache entry expires after TTL; next call hits API again | Pass |
| `UT-GW-1029-032` | Cache invalidated on registry refresh | Pass |

#### Adversarial & Security

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-GW-1029-033` | Alert label key containing path traversal characters (`../`, `%2F`) does not cause panic or unexpected behavior in registry lookup | Pass |
| `UT-GW-1029-034` | Alert with 100+ labels does not cause unbounded API calls; candidates are capped or bounded by label count | Pass |
| `UT-GW-1029-035` | Discovery response containing 500+ API resources builds registry within 100ms (no O(n^2) in construction) | Pass |
| `UT-GW-1029-036` | Existence cache does not leak memory: after N unique cache keys, oldest entries are evicted or TTL-expired | Pass |
| `UT-GW-1029-037` | Registry refresh failure (post-startup) preserves previous good map; does not leave empty/nil maps | Pass |
| `UT-GW-1029-038` | Concurrent Parse() calls during registry refresh do not observe partial map state | Pass |
| `UT-GW-1029-039` | Alert label value with empty string `""` is handled gracefully (not used as resource name for existence check) | Pass |
| `UT-GW-1029-040` | Alert with duplicate label keys (same label appearing twice with different values) does not panic; last-write-wins or first-match per Prometheus spec | Pass |

#### Batch-Independent Processing (#1032)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-GW-1032-001` | `ParseBatch()` with 3-alert webhook returns 3 independent `NormalizedSignal` entries | Pass |
| `UT-GW-1032-002` | `ParseBatch()` with 3-alert webhook where 1st alert fails returns 2 signals (partial success) | Pass |
| `UT-GW-1032-003` | `ParseBatch()` with all alerts failing returns empty slice and no error (not payload-level failure) | Pass |
| `UT-GW-1032-004` | `ParseBatch()` with malformed JSON returns error (payload-level failure) | Pass |
| `UT-GW-1032-005` | Each signal from batch gets independent fingerprint and dedup check | Pass |

### Tier 2: Integration Tests

**Testable code scope**: `server.go` handler loop, `cmd/gateway/main.go` wiring, real discovery against envtest. Target: >=80%.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-GW-1029-001` | Alert with `deployment: nginx` through real envtest creates RR with `TargetResource.Kind = Deployment` (regression gate) | Pass |
| `IT-GW-1029-002` | Custom CRD installed in envtest; alert with CRD singular-name label creates RR with correct Kind | Pass |
| `IT-GW-1029-003` | Alert with `exported_namespace != namespace` resolves target in correct namespace via existence check | Pass |
| `IT-GW-1029-004` | Fingerprint for `deployment: nginx` alert is identical before and after the change (dedup parity) | Pass |
| `IT-GW-1029-005` | Handler returns aggregated response `{processed: N, deduplicated: M, failed: F}` for multi-alert webhook | Pass |
| `IT-GW-1029-006` | `gateway.signal.dropped` audit event emitted when alert cannot resolve any resource | Pass |
| `IT-GW-1032-001` | 3-alert webhook creates 3 independent RRs (one per alert) | Pass |
| `IT-GW-1032-002` | 3-alert webhook with 1 duplicate creates 2 RRs + 1 dedup response | Pass |
| `IT-GW-1032-003` | 3-alert webhook with 1 unresolvable alert creates 2 RRs + 1 dropped metric | Pass |

### Tier 3: E2E Tests (if applicable)

**Testable code scope**: Full gateway binary in Kind cluster with CRD installed.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-GW-1029-001` | Multi-alert Prometheus webhook to running gateway creates expected RRs | Pass |

### Tier Skip Rationale

- **E2E**: Only one E2E test planned. CRD installation in Kind adds CI complexity. Core logic is thoroughly covered by UT + IT tiers. E2E validates the complete binary wiring path only.

---

## 9. Test Cases

### UT-GW-1029-001: Registry parity — deployment label

**BR**: BR-GATEWAY-001, BR-GATEWAY-004
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/adapters/resource_registry_test.go`

**Preconditions**:
- Fake discovery client configured with standard `apps/v1` API resources including `Deployment` (`SingularName: deployment`)

**Test Steps**:
1. **Given**: A registry constructed from a fake discovery response containing `apps/v1` resources
2. **When**: `LabelToKind("deployment")` is called
3. **Then**: Returns `"Deployment"` — identical to the old `resourceCandidates[3]` entry

**Expected Results**:
1. Kind is `"Deployment"` (exact string match)
2. GVR is `apps/v1/deployments`

**Acceptance Criteria**:
- **Behavior**: Label key resolves to the same kind as the static map
- **Correctness**: GVR matches the real Kubernetes API group/version/resource

**Dependencies**: None

---

### UT-GW-1029-013: job_name label intentionally not matched

**BR**: BR-GATEWAY-1029
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/adapters/resource_registry_test.go`

**Preconditions**:
- Registry built from discovery containing `batch/v1` resources including `Job` (`SingularName: job`)

**Test Steps**:
1. **Given**: A registry with standard K8s resources
2. **When**: `LabelToKind("job_name")` is called
3. **Then**: Returns empty string (no match)

**Expected Results**:
1. No kind returned — `job_name` is a Prometheus scrape configuration label, not a K8s `APIResource.SingularName`
2. `LabelToKind("job")` returns `"Job"` (the correct K8s mapping)

**Acceptance Criteria**:
- **Behavior**: Prometheus heuristic labels are not treated as K8s resource references
- **Correctness**: Only `APIResource.SingularName` and lowercase `Kind` match label keys

**Dependencies**: UT-GW-1029-009

---

### UT-GW-1029-014: Fail-fast startup on discovery failure

**BR**: BR-GATEWAY-1029
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/adapters/resource_registry_test.go`

**Preconditions**:
- Fake discovery client configured to return an error on `ServerPreferredResources()`

**Test Steps**:
1. **Given**: A discovery client that returns `(nil, errors.New("RBAC denied"))`
2. **When**: `NewAPIResourceRegistry(discoveryClient)` is called
3. **Then**: Returns a non-nil error containing remediation guidance

**Expected Results**:
1. Error is non-nil
2. Error message contains "discovery" and guidance text
3. No panic

**Acceptance Criteria**:
- **Behavior**: Gateway startup aborts cleanly
- **Correctness**: Error is descriptive for operator remediation

**Dependencies**: None

---

### UT-GW-1029-017: CRD BuildConfig recognition

**BR**: BR-GATEWAY-1029
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/adapters/resource_extraction_dynamic_test.go`

**Preconditions**:
- Registry includes `build.openshift.io/v1` with `BuildConfig` (`SingularName: buildconfig`)
- Fake existence checker returns "exists" for `BuildConfig/webapp` in namespace `demo-build`

**Test Steps**:
1. **Given**: Alert labels `{buildconfig: webapp, namespace: demo-build}`
2. **When**: `extractTargetResource(labels, registry, existenceChecker)` is called
3. **Then**: Returns `ResourceIdentifier{Kind: "BuildConfig", Name: "webapp"}`

**Expected Results**:
1. Kind is `"BuildConfig"` (not `"Unknown"`)
2. Name is `"webapp"` (from label value)

**Acceptance Criteria**:
- **Behavior**: CRD label keys are recognized via discovery
- **Correctness**: Kind matches the cluster's API resource kind name

**Dependencies**: UT-GW-1029-001 (registry construction)

---

### UT-GW-1032-001: Batch returns independent signals

**BR**: BR-GATEWAY-1032
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/adapters/prometheus_adapter_batch_test.go`

**Preconditions**:
- Registry contains standard K8s resources
- 3 alerts in webhook: `deployment: nginx`, `statefulset: redis`, `daemonset: fluentd`
- All three resources exist in their respective namespaces

**Test Steps**:
1. **Given**: AlertManager webhook JSON with 3 firing alerts
2. **When**: `PrometheusAdapter.Parse(ctx, rawData)` is called
3. **Then**: Returns a slice of 3 `NormalizedSignal` entries

**Expected Results**:
1. Slice length is 3
2. Each signal has a distinct fingerprint
3. Signal[0].TargetResource.Kind is `Deployment`, Signal[1] is `StatefulSet`, Signal[2] is `DaemonSet`

**Acceptance Criteria**:
- **Behavior**: Every alert in the batch is processed independently
- **Correctness**: Each signal carries the correct resource identifier

**Dependencies**: UT-GW-1029-001 (registry construction)

---

### UT-GW-1032-002: Partial batch success

**BR**: BR-GATEWAY-1032
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/adapters/prometheus_adapter_batch_test.go`

**Preconditions**:
- Registry contains standard K8s resources
- 3 alerts: 1st has unrecognized label `foobar: baz`, 2nd is `deployment: nginx`, 3rd is `pod: redis-0`
- `Deployment/nginx` and `Pod/redis-0` exist; `foobar/baz` does not match any API resource

**Test Steps**:
1. **Given**: AlertManager webhook with 3 alerts (1 unresolvable, 2 resolvable)
2. **When**: `PrometheusAdapter.Parse(ctx, rawData)` is called
3. **Then**: Returns 2 signals (the resolvable ones)

**Expected Results**:
1. Slice length is 2
2. No error returned (batch-level success)
3. `gateway_signals_parse_dropped_total` incremented by 1

**Acceptance Criteria**:
- **Behavior**: Failed alerts don't block successful ones
- **Correctness**: Drop metric accurately reflects the count

**Dependencies**: UT-GW-1029-001, UT-GW-1029-028

---

### IT-GW-1029-002: CRD discovery in envtest

**BR**: BR-GATEWAY-1029
**Priority**: P0
**Type**: Integration
**File**: `test/integration/gateway/dynamic_discovery_integration_test.go`

**Preconditions**:
- envtest cluster running with a custom test CRD installed (`test.kubernaut.io/v1`, Kind: `TestWidget`, SingularName: `testwidget`)
- Real `K8sOwnerResolver` with registry
- A `TestWidget` resource `my-widget` exists in namespace `test-ns`

**Test Steps**:
1. **Given**: envtest with custom CRD and resource instance
2. **When**: Prometheus webhook with label `testwidget: my-widget` and namespace `test-ns` is processed
3. **Then**: RR is created with `TargetResource.Kind = TestWidget`

**Expected Results**:
1. RR `spec.targetResource.kind` is `"TestWidget"`
2. RR `spec.targetResource.name` is `"my-widget"`
3. RR `spec.targetResource.namespace` is `"test-ns"`

**Acceptance Criteria**:
- **Behavior**: Dynamic discovery recognizes CRDs installed in the cluster
- **Correctness**: RR target matches the actual CRD resource

**Dependencies**: envtest CRD installation

---

### IT-GW-1032-001: Batch creates independent RRs

**BR**: BR-GATEWAY-1032
**Priority**: P0
**Type**: Integration
**File**: `test/integration/gateway/batch_processing_integration_test.go`

**Preconditions**:
- envtest with Deployment `nginx` and StatefulSet `redis` in namespace `default`

**Test Steps**:
1. **Given**: envtest with two real resources
2. **When**: Prometheus webhook with 2 alerts (`deployment: nginx`, `statefulset: redis`) is submitted
3. **Then**: 2 independent RRs are created

**Expected Results**:
1. Two `RemediationRequest` CRDs exist in the controller namespace
2. Each has a distinct fingerprint
3. Each has the correct `targetResource`

**Acceptance Criteria**:
- **Behavior**: Batch processing creates one RR per alert
- **Correctness**: No alert is silently dropped

**Dependencies**: envtest setup

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Fake discovery client (returns controlled `APIResourceList`), fake existence checker (returns controlled existence results)
- **Location**: `test/unit/gateway/adapters/`
- **Resources**: Standard CPU/memory

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see No-Mocks Policy)
- **Infrastructure**: envtest (real API server), custom test CRD YAML
- **Location**: `test/integration/gateway/`
- **Resources**: ~512MB for envtest API server

### 10.3 E2E Tests (if applicable)

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind cluster with gateway binary deployed
- **Location**: `test/e2e/gateway/`
- **Resources**: Kind cluster (~2GB), Docker daemon

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.25.7 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| controller-runtime envtest | v0.20.x | Integration test API server |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| v1.4.0-rc5 | Tag | Merged | Branch base is stale | Rebase on main |
| client-go v0.35.3 | Library | Available | Discovery client API unavailable | N/A (already in go.mod) |
| envtest binary | Tool | Available | Integration tests blocked | Use unit tests only |

### 11.2 Execution Order

Each phase follows strict TDD RED -> GREEN -> REFACTOR with checkpoints between phases.

#### Phase 1: TDD RED — API Resource Registry

Write failing tests for registry construction, label-to-kind, kind-to-GVR, tier priority,
fail-fast, concurrent refresh, existence cache, adversarial inputs.

**Tests**: UT-GW-1029-001 through UT-GW-1029-016, UT-GW-1029-030 through UT-GW-1029-040

**Checkpoint 1A — RED Validation**:
- [ ] All tests compile
- [ ] All tests fail with expected assertion errors (not panics or compile errors)
- [ ] No test uses `Skip()` or `XIt`
- [ ] Test file structure follows existing `test/unit/gateway/adapters/` conventions
- [ ] `go vet ./test/unit/gateway/...` passes

**Checkpoint 1B — RED Security & Adversarial Audit**:
- [ ] Adversarial tests (UT-GW-1029-033..040) cover: path traversal in label keys, high-cardinality labels, empty label values, memory exhaustion via cache, partial map visibility during refresh
- [ ] Every test that involves string input from alert labels validates boundary conditions (empty, very long, special characters)
- [ ] Tests for `job_name` non-match (UT-GW-1029-013) explicitly document WHY the old heuristic is dropped — Prometheus scrape label is not a K8s resource
- [ ] No test hardcodes namespace `default` without justification — most tests should use explicit namespaces to catch namespace leakage bugs

#### Phase 2: TDD GREEN — API Resource Registry

Implement `resource_registry.go` with minimal code to pass all Phase 1 tests.

**Checkpoint 2A — GREEN Validation**:
- [ ] All UT-GW-1029-001..016, 030..040 pass
- [ ] `go build ./...` succeeds (no compile errors across codebase)
- [ ] `go vet ./...` passes
- [ ] No pre-existing gateway tests regressed (`go test ./test/unit/gateway/...`)
- [ ] Race detector clean: `go test -race ./test/unit/gateway/adapters/...`

**Checkpoint 2B — GREEN Security & Adversarial Audit**:
- [ ] **Input validation**: Registry construction rejects or safely handles `APIResourceList` entries with empty `Kind`, empty `SingularName`, or `SingularName` containing `/` or `..`
- [ ] **Memory bounds**: Map sizes are proportional to discovery response size; no unbounded growth
- [ ] **No secrets in logs**: Discovery results (API group names, resource names) logged only at aggregate level (count), never full list at INFO or below
- [ ] **No panics**: `recover()` not needed because no `panic()` exists — all error paths return errors
- [ ] **Fail-fast is fail-LOUD**: Error message from failed startup includes: (a) what failed, (b) which ServiceAccount, (c) what RBAC is needed, (d) link to docs
- [ ] **Cache key safety**: Existence cache keys are constructed from validated strings only; no user-controlled raw strings as map keys without bounds

#### Phase 3: TDD REFACTOR — API Resource Registry

Refactor registry code for quality, applying 100 Go Mistakes checklist.

**100 Go Mistakes Refactoring Checklist** (applicable items):
- [ ] #1: No unintended variable shadowing in registry methods
- [ ] #2: Unnecessary nested code reduced; happy path left-aligned
- [ ] #5: No interface pollution — registry interface is minimal (consumer-side)
- [ ] #8: No `any`/`interface{}` used where concrete types suffice
- [ ] #10: Type embedding used correctly (no hidden field promotion issues)
- [ ] #11: Functional options pattern for registry configuration (refresh interval, TTL)
- [ ] #21: Slice initialization efficient (pre-allocated where size is known)
- [ ] #27: Map initialization efficient (pre-allocated from discovery result count)
- [ ] #28: No map memory leaks (cache entries bounded by TTL, cleared on refresh)
- [ ] #39: String concatenation uses `strings.Builder` if building composite keys
- [ ] #42: Receiver type correct (pointer for mutable registry, value for read-only helpers)
- [ ] #48: No panicking — all errors returned, never `panic()`
- [ ] #49: Errors wrapped with context (`fmt.Errorf("registry: %w", err)`)
- [ ] #52: No error handled twice (logged then returned, or returned only)
- [ ] #53: No errors silently ignored
- [ ] #54: Defer errors handled (if any deferred close)
- [ ] #57: RWMutex correctly chosen over channels for concurrent map access
- [ ] #58: No data races — verified by `-race`
- [ ] #60: Context propagated correctly for cancellation during refresh
- [ ] #62: No goroutine leaks — refresh goroutine stops when context cancelled
- [ ] #70: Mutex usage correct with maps (RLock for reads, Lock for writes)
- [ ] #74: sync types not copied (mutex is a struct field, not passed by value)
- [ ] #75: Time durations use typed constants, not raw integers
- [ ] #79: No transient resources left open (discovery client responses)
- [ ] #81: No default HTTP client used (discovery client uses kubeconfig transport)
- [ ] #100: Aware of Go-in-K8s implications (GOMAXPROCS, memory limits)

**Checkpoint 3A — REFACTOR Validation**:
- [ ] All tests still pass after refactoring
- [ ] `golangci-lint run --timeout=5m ./pkg/gateway/adapters/...` clean
- [ ] No new lint warnings introduced
- [ ] Code review: no `any`, no panics, no shadowed variables, no goroutine leaks
- [ ] `go test -race ./test/unit/gateway/adapters/...` still clean

**Checkpoint 3B — REFACTOR Security Audit**:
- [ ] **Exported surface area**: Only the minimum necessary API is exported. Internal maps, cache entries, and refresh state are unexported.
- [ ] **Defensive copying**: `LabelToKind()` and `KindToGVR()` return copies or immutable views, not references to internal state that callers could mutate.
- [ ] **Thread safety proof**: Every public method documents whether it is safe for concurrent use. The `RWMutex` is held for the minimum necessary duration.
- [ ] **Error messages do not leak cluster topology**: Error strings from failed existence checks do not include full API server URLs or bearer tokens.
- [ ] **No time-of-check-time-of-use (TOCTOU)**: Registry read operations take a consistent snapshot (read lock held for entire label scan, not per-label).

**Checkpoint 3C — REFACTOR Architectural Audit**:
- [ ] Registry does NOT depend on any `internal/` package (it lives in `pkg/gateway/adapters/`)
- [ ] Registry depends only on `k8s.io/client-go/discovery` and `k8s.io/apimachinery` (no controller-runtime dependency for the core logic)
- [ ] No circular dependency introduced
- [ ] `go build ./...` still succeeds after refactor

#### Phase 4: TDD RED — Multi-Candidate Scoring + Existence Validation

Write failing tests for `extractTargetResource` replacement, CRD recognition,
existence validation, cross-namespace resolution.

**Tests**: UT-GW-1029-017 through UT-GW-1029-023

**Checkpoint 4A — RED Validation**:
- [ ] All tests compile
- [ ] All tests fail with expected assertion errors
- [ ] No `Skip()` or `XIt`
- [ ] `go vet ./test/unit/gateway/...` passes

**Checkpoint 4B — RED Security Audit**:
- [ ] Existence validation tests include: resource exists, resource does not exist, API returns 403, API returns 404, API returns 500, API returns timeout
- [ ] Cross-namespace tests validate that `exported_namespace` is never used to bypass namespace isolation — existence checks are scoped to the namespace, not cluster-wide
- [ ] No test assumes cluster-admin permissions — all existence checks use `PartialObjectMetadata` `GET` (not `LIST`)

#### Phase 5: TDD GREEN — Multi-Candidate Scoring + Existence Validation

Implement scoring logic in `prometheus_adapter.go`. Remove `resourceCandidates`.
Remove `LabelFilter` interface and `label_filter.go`.

**Checkpoint 5A — GREEN Validation**:
- [ ] All UT-GW-1029-017..023 pass
- [ ] All parity tests UT-GW-1029-001..012 still pass
- [ ] `go build ./...` succeeds
- [ ] Pre-existing tests: `go test ./test/unit/gateway/...` — identify which existing tests need updating due to removed `LabelFilter` parameter
- [ ] Race detector clean

**Checkpoint 5B — GREEN Security & Completeness Audit**:
- [ ] **Static map removed**: `grep -r "resourceCandidates" pkg/gateway/` returns zero results
- [ ] **LabelFilter removed**: `grep -r "LabelFilter\|MonitoringMetadataFilter\|IsMonitoringMetadata" pkg/gateway/` returns zero results
- [ ] **No fallback to static**: There is no conditional path that falls back to a hardcoded list — discovery is the only code path
- [ ] **Existence check authorization**: `GET` calls use the same client as the rest of the gateway (ServiceAccount-scoped), not a privileged client
- [ ] **Error handling on existence check**: 403, 404, timeout, network error — each produces a distinct log message and skips the candidate without crashing
- [ ] **No namespace escalation**: Existence checks in `exported_namespace` do not grant the gateway implicit access to that namespace — the SA's RBAC still governs access
- [ ] **Candidate count is bounded**: The number of existence checks per alert is bounded by `min(len(alert.Labels), len(registry))` — verify this is capped at a reasonable maximum (e.g., no `LIST` operations)

#### Phase 6: TDD REFACTOR — Multi-Candidate Scoring

Refactor scoring and existence validation code.

**100 Go Mistakes Refactoring Checklist** (additional items for this phase):
- [ ] #22: Nil vs empty slice — `Parse()` returns empty slice `[]*NormalizedSignal{}`, not nil, when all alerts fail
- [ ] #23: Empty slice check uses `len(signals) == 0`, not `signals == nil`
- [ ] #25: No unexpected slice append side effects when building candidates
- [ ] #29: Comparison values correct — string comparisons for kind matching are case-insensitive where needed
- [ ] #43: Named return parameters used only where they add clarity
- [ ] #46: No filename as function input — paths are not hardcoded

**Checkpoint 6A — REFACTOR Validation**:
- [ ] All tests pass
- [ ] `golangci-lint run` clean on modified files
- [ ] Removed `label_filter.go`, `LabelFilter` interface, `MonitoringMetadataFilter`
- [ ] All consumers of `LabelFilter` updated (prometheus_adapter.go, cmd/gateway/main.go, tests)
- [ ] `go build ./...` succeeds

**Checkpoint 6B — REFACTOR Security Audit**:
- [ ] **Scoring algorithm is deterministic**: Given the same alert labels and registry state, the same candidate always wins. No random tiebreaking, no map iteration order dependency.
- [ ] **No information leakage in error responses**: When scoring fails (no candidate), the HTTP response does not reveal which labels were scanned or which API resources exist in the cluster.
- [ ] **Log levels appropriate**: Successful resolution logged at DEBUG. Failed resolution logged at INFO (not WARN — it's a normal condition for unknown alerts). Existence check errors logged at WARN.

#### Phase 7: TDD RED — Owner Resolution (Registry-Backed)

Write failing tests for `ResolveTopLevelOwner` with registry GVR lookup and Option C.

**Tests**: UT-GW-1029-024 through UT-GW-1029-027

**Checkpoint 7A — RED Validation**:
- [ ] All tests compile and fail with expected assertions
- [ ] `go vet` passes

**Checkpoint 7B — RED Security Audit**:
- [ ] Option C tests (UT-GW-1029-025, 026) verify that CRD kinds are NEVER traversed — this is a security boundary (traversal would require RBAC grants on every CRD API group)
- [ ] Owner chain depth limit (MaxOwnerChainDepth=5) is preserved — no path through registry-backed lookup increases the depth
- [ ] Tests verify behavior when registry returns no GVR for a kind (kind removed from cluster between refresh cycles)

#### Phase 8: TDD GREEN — Owner Resolution

Implement registry-backed GVR lookup in `owner_resolver.go`. Remove `kindToGroup`.
Implement Option C for CRD kinds.

**Checkpoint 8A — GREEN Validation**:
- [ ] All UT-GW-1029-024..027 pass
- [ ] Pre-existing owner resolver tests still pass
- [ ] `go build ./...` succeeds
- [ ] `KindToGroup()` and static `kindToGroup` map removed
- [ ] `OwnerChainCacheObjects()` uses registry
- [ ] Race detector clean

**Checkpoint 8B — GREEN Security & Completeness Audit**:
- [ ] **Static map removed**: `grep -r "kindToGroup" pkg/gateway/` returns zero results (except comments explaining removal)
- [ ] **`KindToGroup()` removed**: `grep -r "KindToGroup()" .` returns zero results
- [ ] **No new RBAC required for CRD resolution**: Option C means CRD kinds return identity — no `Get` call needed for the CRD resource itself during owner traversal
- [ ] **Owner chain traversal still bounded**: `MaxOwnerChainDepth` unchanged, no new loop introduced
- [ ] **Fallback reader behavior preserved**: When cache misses, fallback reader (direct API) is still used for core kinds — this path is not affected by the registry change

#### Phase 9: TDD REFACTOR — Owner Resolution

Refactor owner resolver code.

**100 Go Mistakes Checklist** (owner resolver specific):
- [ ] #35: No defer inside loop (owner chain traversal loop)
- [ ] #47: Defer argument evaluation correct
- [ ] #60: Context deadline propagated to each `Get` call in traversal
- [ ] #75: `ownerLookupTimeout` uses `time.Duration` constant

**Checkpoint 9A — REFACTOR Validation**:
- [ ] All tests pass
- [ ] Lint clean
- [ ] No static maps remain in `owner_resolver.go`

**Checkpoint 9B — REFACTOR Security Audit**:
- [ ] **TOCTOU on registry**: If registry refreshes between `KindToGVR()` and the actual `Get()` call, the code still works correctly (GVR used for the request was valid at time of read; server validates it)
- [ ] **Error wrapping does not expose internal API addresses**: Error messages from failed `Get` calls are wrapped with business context, not raw client-go errors that may contain API server URLs

#### Phase 10: TDD RED — Batch-Independent Processing (#1032)

Write failing tests for `Parse()` returning `[]*NormalizedSignal`, handler loop,
aggregated response.

**Tests**: UT-GW-1032-001 through UT-GW-1032-005

**Checkpoint 10A — RED Validation**:
- [ ] All tests compile and fail
- [ ] Interface change in `adapter.go` is reflected
- [ ] `KubernetesEventAdapter.Parse()` updated to return single-element slice

**Checkpoint 10B — RED Adversarial Audit**:
- [ ] Tests include: webhook with 0 alerts (empty array), webhook with 1 alert, webhook with 100 alerts (stress), webhook with all alerts identical (dedup storm)
- [ ] Tests verify that partial failure does not corrupt the signals from successful alerts
- [ ] Tests verify that the order of signals in the returned slice matches the order of alerts in the webhook (deterministic)

#### Phase 11: TDD GREEN — Batch-Independent Processing

Implement batch processing in `PrometheusAdapter.Parse()` and handler loop in `server.go`.

**Checkpoint 11A — GREEN Validation**:
- [ ] All UT-GW-1032-001..005 pass
- [ ] `go build ./...` succeeds (all adapter implementations updated)
- [ ] Pre-existing adapter tests updated for new signature
- [ ] Handler returns aggregated response
- [ ] Race detector clean

**Checkpoint 11B — GREEN Security & Completeness Audit**:
- [ ] **No silent drops**: Every alert in the batch is either (a) in the returned signal slice or (b) counted in `gateway_signals_parse_dropped_total` — sum must equal input alert count
- [ ] **Handler timeout isolation**: Each signal processed in the handler loop gets its own `context.WithTimeout` — one slow signal does not consume the timeout budget for subsequent signals
- [ ] **Response HTTP status logic is correct**: 201 (any RR created) > 202 (all deduplicated) > 200 (all rejected/dropped). No edge case where 0 signals returns 201.
- [ ] **Aggregated response JSON is valid**: Response body is well-formed JSON even when all signals fail (empty processed list, non-zero failed count)
- [ ] **Pre-existing tests updated**: All files in Section 14 (Existing Tests Requiring Updates) have been modified and pass

#### Phase 12: TDD REFACTOR — Batch-Independent Processing

Refactor batch processing code.

**100 Go Mistakes Checklist** (batch specific):
- [ ] #22: Return empty slice, not nil, when all alerts fail
- [ ] #25: No append side effects when building signal slice
- [ ] #30: Range loop does not copy large structs (use pointer or index)
- [ ] #69: No data races with concurrent append (signals collected sequentially per alert)

**Checkpoint 12A — REFACTOR Validation**:
- [ ] All tests pass
- [ ] Lint clean
- [ ] Response shape documented

**Checkpoint 12B — REFACTOR Adversarial Audit**:
- [ ] **Memory allocation**: Signal slice pre-allocated to `len(alerts)` capacity to avoid repeated grow+copy
- [ ] **No goroutine-per-alert**: Alerts are processed sequentially within `Parse()` — no unbounded goroutine creation from a single webhook
- [ ] **Error accumulation**: Per-alert errors are logged individually, not accumulated into a massive error string that could be used for log injection

#### Phase 13: TDD RED — Observability

Write failing tests for new metrics and audit event.

**Tests**: UT-GW-1029-028, UT-GW-1029-029

**Checkpoint 13A — RED Validation**:
- [ ] Tests compile and fail

#### Phase 14: TDD GREEN — Observability

Register new metrics, emit audit events.

**Checkpoint 14A — GREEN Validation**:
- [ ] All observability tests pass
- [ ] Metrics registered without name conflicts
- [ ] `go build ./...` succeeds

**Checkpoint 14B — GREEN Security Audit**:
- [ ] **Metric label cardinality bounded**: `kind` label in `gateway_owner_resolution_total` is the K8s Kind string (bounded by cluster's API resources, typically <100). No user-controlled unbounded labels.
- [ ] **Audit event does not contain sensitive data**: `gateway.signal.dropped` event includes signal_name, namespace, drop_reason — does NOT include raw alert labels (which may contain sensitive values)
- [ ] **Audit event is emitted even when all processing fails**: The "fire-and-forget" pattern from `StoreBestEffort` is used; audit emission failure does not block signal processing

#### Phase 15: TDD REFACTOR — Observability

Refactor metrics registration.

**Checkpoint 15A — REFACTOR Validation**:
- [ ] No duplicate metric names
- [ ] Label cardinality bounded
- [ ] Lint clean

#### Phase 16: Wiring + Integration Tests

Wire discovery client, registry, and refresh loop in `cmd/gateway/main.go`.
Write and execute integration tests.

**Tests**: IT-GW-1029-001 through IT-GW-1029-006, IT-GW-1032-001 through IT-GW-1032-003

**Checkpoint 16A — Wiring Validation**:
- [ ] `go build ./cmd/gateway/...` succeeds
- [ ] `cmd/gateway/main.go` creates registry with fail-fast
- [ ] Registry passed to `PrometheusAdapter` and `K8sOwnerResolver`
- [ ] Refresh goroutine started with context cancellation
- [ ] `LabelFilter` parameter removed from `NewPrometheusAdapter`

**Checkpoint 16B — Integration Validation**:
- [ ] All integration tests pass
- [ ] All pre-existing gateway integration tests pass
- [ ] `go test -race ./test/integration/gateway/...` clean

**Checkpoint 16C — Integration Security & Adversarial Audit**:
- [ ] **CRD discovery in envtest**: Custom test CRD installed and discovered; alert with CRD label creates correct RR
- [ ] **Cross-namespace isolation**: Existence check in `exported_namespace` does not return resources from other namespaces; envtest RBAC scoped correctly
- [ ] **Batch independence**: Multi-alert webhook creates independent RRs; one alert's failure does not affect others
- [ ] **Audit event emission**: `gateway.signal.dropped` event appears in audit store for unresolvable alerts
- [ ] **No regression**: Full pre-existing integration suite passes without modification (except Section 14 updates)
- [ ] **Discovery refresh during processing**: If feasible, test that a registry refresh mid-processing does not cause errors (may require test timing control)

#### Phase 17: Deploy Manifest Fixes

Fix pre-existing RBAC, probe, and configmap issues.

**Checkpoint 17A — Manifest Validation**:
- [ ] `deploy/gateway/01-rbac.yaml` uses `kubernaut.ai` API group
- [ ] `deploy/gateway/base/01-rbac.yaml` has no stale secrets access
- [ ] Probe port is 8081, paths are `/healthz` and `/readyz`
- [ ] No stale `rate_limit` in configmap

**Checkpoint 17B — Manifest Security Audit**:
- [ ] **RBAC least privilege**: ClusterRole grants only the verbs and API groups needed. No wildcard `*` on verbs or resources.
- [ ] **No secrets access**: `secrets` resource is NOT in the ClusterRole (removed per #673)
- [ ] **ServiceAccount exists**: Gateway ServiceAccount is referenced correctly in ClusterRoleBinding
- [ ] **Diff review**: `git diff deploy/` shows only the intended changes; no accidental whitespace or formatting drift

#### Phase 18: Final Validation

**Checkpoint 18A — Full Suite**:
- [ ] `go build ./...` succeeds
- [ ] `go test ./test/unit/gateway/... -race` — all pass
- [ ] `go test ./test/integration/gateway/... -race` — all pass
- [ ] `golangci-lint run --timeout=5m` — clean
- [ ] No `resourceCandidates` static slice remains
- [ ] No `kindToGroup` static map remains
- [ ] No `LabelFilter` interface remains
- [ ] No `KindToGroup()` exported function remains
- [ ] Coverage >=80% per tier on modified files

**Checkpoint 18B — Final Security & Adversarial Audit**:
- [ ] **grep sweep**: `grep -r "panic(" pkg/gateway/` returns zero new panics (only pre-existing, if any)
- [ ] **grep sweep**: `grep -r "interface{}\|any" pkg/gateway/adapters/resource_registry.go` returns zero
- [ ] **grep sweep**: `grep -r "TODO\|FIXME\|HACK\|XXX" pkg/gateway/adapters/resource_registry.go` returns zero
- [ ] **grep sweep**: `grep -r "os.Exit\|log.Fatal" pkg/gateway/` returns zero (graceful shutdown only)
- [ ] **No hardcoded credentials, tokens, or API server URLs** in any modified file
- [ ] **Error messages reviewed**: No error message exposes: API server URL, bearer token, ServiceAccount token path, or cluster internal DNS names
- [ ] **Discovery data classification**: Registry contents (API group names, resource names) are internal cluster metadata — verified they are NOT logged at INFO level, NOT serialized to disk, NOT exposed via any API endpoint

**Checkpoint 18C — Final Architectural Audit**:
- [ ] **No `internal/` imports from `pkg/gateway/`**: `grep -r 'internal/' pkg/gateway/ --include="*.go"` returns zero (preserving layering)
- [ ] **No circular dependencies**: `go build ./...` succeeds (Go compiler enforces this, but verify no import cycles were introduced)
- [ ] **Interface compatibility**: `SignalAdapter` interface is the only breaking change; all implementations updated
- [ ] **Test anti-patterns**: No `time.Sleep` in new tests (use `Eventually`); no `Skip()` or `XIt`; no direct audit/metrics testing (business behavior only)

**Checkpoint 18D — Escalation Review**:
- [ ] If any checkpoint finding was deferred or accepted-as-is, document it here with justification
- [ ] If any pre-existing test was deleted (not updated), document why and confirm the business requirement is still covered by another test
- [ ] Confidence assessment provided (target: >=95%)

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/1029/TEST_PLAN.md` | Strategy and test design |
| Unit test: registry | `test/unit/gateway/adapters/resource_registry_test.go` | Registry construction, parity, priority, thread safety |
| Unit test: scoring | `test/unit/gateway/adapters/scoring_test.go` | Multi-candidate scoring, existence validation, cross-ns |
| Unit test: owner resolver | `test/unit/gateway/adapters/owner_resolver_registry_test.go` | Registry-backed GVR lookup, Option C |
| Unit test: batch | `test/unit/gateway/adapters/batch_processing_test.go` | Batch-independent processing |
| Unit test: observability | `test/unit/gateway/adapters/observability_test.go` | New metrics emission |
| Integration test: adapters | `test/integration/gateway/adapters_integration_test.go` | CRD discovery, existence checks |
| Integration test: audit | `test/integration/gateway/audit_emission_integration_test.go` | Audit event emission |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests (all gateway)
go test ./test/unit/gateway/... -ginkgo.v

# Unit tests (registry only)
go test ./test/unit/gateway/adapters/... -ginkgo.focus="UT-GW-1029"

# Unit tests (batch only)
go test ./test/unit/gateway/adapters/... -ginkgo.focus="UT-GW-1032"

# Integration tests
go test ./test/integration/gateway/... -ginkgo.v

# Race detector
go test -race ./test/unit/gateway/...
go test -race ./test/integration/gateway/...

# Coverage (unit)
go test ./test/unit/gateway/... -coverprofile=coverage-ut.out
go tool cover -func=coverage-ut.out

# Coverage (integration)
go test ./test/integration/gateway/... -coverprofile=coverage-it.out
go tool cover -func=coverage-it.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/gateway/adapters/prometheus_adapter_test.go` (GW-RE-01..16) | `Parse()` returns `*NormalizedSignal` | Update to assert `[]*NormalizedSignal` with `Expect(signals).To(HaveLen(1))` | Interface change: `Parse()` now returns a slice |
| `test/unit/gateway/adapters/label_filter_test.go` | Tests `LabelFilter` interface | **Delete entire file** | `LabelFilter` removed; existence validation replaces it |
| `test/unit/gateway/adapters/resource_extraction_test.go` | Tests `extractTargetResource(labels, filter)` | Update to `extractTargetResource(labels, registry, existenceChecker)` | Function signature changed |
| `test/unit/gateway/cache_config_test.go` | Tests `kindToGroup` map consistency | **Delete or rewrite** | Static `kindToGroup` removed; test should validate `OwnerChainCacheObjects()` from registry |
| `test/unit/gateway/k8s_event_adapter_test.go` | `Parse()` returns `*NormalizedSignal` | Update to assert `[]*NormalizedSignal` with `Expect(signals).To(HaveLen(1))` | Interface change |
| `test/unit/gateway/prometheus_batch_alert_test.go` (UT-GW-451-*) | First successful alert returned | Update to assert all successful alerts returned as slice | Batch semantics changed |
| `test/unit/gateway/adapters/adapter_interface_test.go` | Tests `SignalAdapter.Parse()` returning `*NormalizedSignal` | Update interface assertions | Signature change |
| `test/integration/gateway/adapters_integration_test.go` (IT-GW-184-*) | Single signal returned from Parse | Update to expect slice | Interface change |
| `cmd/gateway/main.go` constructor calls | `NewPrometheusAdapter(resolver, labelFilter)` | `NewPrometheusAdapter(resolver, registry)` | `LabelFilter` parameter removed, registry added |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-04 | Initial test plan covering #1029 + #1032 |
