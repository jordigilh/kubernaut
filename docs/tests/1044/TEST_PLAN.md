# Test Plan: apiVersion Validation Gate for Ambiguous Kubernetes Kinds

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1044-v1.0
**Feature**: Post-RCA validation gate that rejects LLM `submit_result` calls omitting `api_version` for Kubernetes kinds that exist in multiple API groups
**Version**: 1.0
**Created**: 2026-05-07
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/1044-apiversion-validation-gate`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the `apiVersionValidationGate` — a post-RCA harness enforcement mechanism that prevents incorrect GVK resolution and subsequent RBAC failures caused by the LLM omitting `api_version` for non-core Kubernetes kinds.

**Production evidence**: In v1.4.0-rc7 on OCP 4.21, the LLM correctly identified `Subscription/etcd` in `demo-operator` as the remediation target (confidence 0.80, 24 tool calls including `kubectl_describe` that returned `apiVersion: operators.coreos.com/v1alpha1`), but submitted the RCA *without* `api_version`. The enrichment pipeline resolved `Subscription` to `messaging.knative.dev` (Knative Eventing) instead of `operators.coreos.com` (OLM) because both CRDs exist on the cluster. This caused an RBAC denial (`subscriptions.messaging.knative.dev "etcd" is forbidden`), failing the scenario despite a correct RCA.

### 1.2 Feature Description

The fix introduces three changes:

1. **`apiVersionValidationGate`** (Approach C — Post-RCA validation gate): After RCA parsing but before re-enrichment, the gate queries the Kubernetes REST mapper to determine if the `RemediationTarget.Kind` is ambiguous (exists in multiple API groups). If ambiguous and `api_version` is empty, the gate injects a correction message naming the conflicting groups and retries once. On exhaustion (LLM still omits `api_version`), the gate sets `HumanReviewNeeded = true` with reason `"rca_incomplete"` to prevent incorrect RBAC grants.

2. **`IsAmbiguousKind`** method on `ScopeResolver`: Queries `meta.RESTMapper.ResourcesFor()` to detect kinds registered in multiple API groups.

3. **`retryRCASubmit` correction message fix**: The existing parse-retry correction example omits `api_version`, contradicting the schema. Fix adds `"api_version":"apps/v1"` to the example JSON.

### 1.3 Objectives

1. Validate ambiguous kind detection via dynamic REST mapper queries
2. Validate gate triggers for multi-group kinds and skips for unambiguous kinds
3. Validate correction message includes conflicting API group names
4. Validate gate exhaustion triggers `HumanReviewNeeded = true` (security-critical)
5. Validate nil `scopeResolver` graceful degradation
6. Validate gate chaining with existing `sameKindValidationGate`
7. Validate `retryRCASubmit` correction example includes `api_version`
8. Validate audit event emission with ambiguity details

### 1.4 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/investigator/... -ginkgo.v` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/investigator/... -ginkgo.v` |
| Unit test code coverage | >=80% of unit-testable code | `go test -coverprofile` on gate + resolver |
| Race detector | 0 races | `go test -race ./test/unit/kubernautagent/investigator/...` |
| Build success | 0 errors | `go build ./...` |
| Lint compliance | 0 new errors | `golangci-lint run --timeout=5m` |
| BR coverage | All ACs | Coverage matrix below |

---

## 2. References

### 2.1 Authoritative Documents

- [Issue #1044](https://github.com/jordigilh/kubernaut/issues/1044) — Enhancement: tool-call-based apiVersion inference when LLM omits api_version
- [Issue #1040](https://github.com/jordigilh/kubernaut/issues/1040) — Parent: apiVersion disambiguation for multi-group kinds
- [PR #1042](https://github.com/jordigilh/kubernaut/pull/1042) — Schema enforcement (prerequisite, merged)
- BR-AI-1044 — apiVersion validation gate for ambiguous kinds
- BR-HAPI-261 — Hierarchy-aware target resolution
- [DD-TEST-006](../../architecture/decisions/DD-TEST-006-test-plan-policy.md) — Test Plan Policy
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) — Per-tier coverage >=80%
- [ANTI_PATTERN_DETECTION.md](../../testing/ANTI_PATTERN_DETECTION.md) — Forbidden test patterns
- [100 Go Mistakes](https://100go.co) — TDD REFACTOR reference

### 2.2 Implementation Files

| File | Role |
|------|------|
| `internal/kubernautagent/investigator/investigator_gates.go` | `apiVersionValidationGate` (new), `sameKindValidationGate` (existing pattern) |
| `internal/kubernautagent/investigator/investigator_phases.go` | `ScopeResolver` interface extension, `mapperScopeResolver.IsAmbiguousKind` (new) |
| `internal/kubernautagent/investigator/investigator.go` | Gate wiring in `runRCA`, `retryRCASubmit` correction message fix |
| `internal/kubernautagent/audit/emitter.go` | `ActionAPIVersionGate` constant (new) |
| `pkg/kubernautagent/types/types.go` | `RemediationTarget.APIVersion` (existing, from #1040) |

### 2.3 Existing Related Tests

| File | Test IDs | Relationship |
|------|----------|-------------|
| `test/unit/kubernautagent/investigator/scope_resolver_test.go` | UT-KA-763-001, -002 | Existing `ScopeResolver` tests (extend for `IsAmbiguousKind`) |
| `test/unit/kubernautagent/investigator/inject_target_apiversion_test.go` | UT-KA-1040-005..007 | `InjectRemediationTarget` apiVersion propagation |
| `test/unit/kubernautagent/parser/parser_apiversion_test.go` | UT-KA-1040-001, -002 | Parser extraction of `api_version` |
| `test/integration/kubernautagent/investigator/investigator_test.go` | IT-KA-847-D-001 | `sameKindValidationGate` integration test (gate pattern) |

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | REST mapper stale after CRD install | Gate misses ambiguity for newly installed CRDs | Low | UT-KA-1044-008 | Use `resettableMapper` retry pattern from `k8s_adapter.go` |
| R2 | `ResourcesFor()` pluralization mismatch | Gate fails to detect ambiguity for irregular plurals | Low | UT-KA-1044-009 | Same pluralization as existing `IsClusterScoped`; known limitation |
| R3 | Correction message doesn't cause LLM to provide `api_version` | LLM retries without improvement | Medium | UT-KA-1044-002 | Gate exhaustion sets `HumanReviewNeeded=true` (fail-safe) |
| R4 | Chained gates (same-kind + api_version) add 4-10s latency | Degraded response time for critical signals | Low | IT-KA-1044-005 | Accepted tradeoff; only fires for ambiguous kinds |
| R5 | Gate fires for core kinds with only one API group | False-positive retries for unambiguous kinds | None | UT-KA-1044-003 | `IsAmbiguousKind` returns false when `len(gvrs) <= 1` |
| R6 | `scopeResolver == nil` in test environments | Panic on nil dereference | None | UT-KA-1044-004 | Nil guard at gate entry (same pattern as `normalizeNamespace`) |
| R7 | Incorrect RBAC grant on gate exhaustion | Security: wrong API group in RBAC | High | UT-KA-1044-002 | Exhaustion → `HumanReviewNeeded=true` prevents automated remediation |

### 3.1 Risk-to-Test Traceability

- **R1**: UT-KA-1044-008 (mapper reset test)
- **R3, R7**: UT-KA-1044-002 (exhaustion → human review)
- **R5**: UT-KA-1044-003 (unambiguous kind bypass)
- **R6**: UT-KA-1044-004 (nil resolver)

---

## 4. Scope

### 4.1 Features to be Tested

- **`IsAmbiguousKind(kind string) (bool, []schema.GroupVersionResource, error)`**: New method on `ScopeResolver` interface; detects kinds in multiple API groups via `meta.RESTMapper.ResourcesFor()`
- **`apiVersionValidationGate`**: Post-RCA gate that rejects ambiguous kinds missing `api_version`, retries once with correction message naming conflicting groups, sets `HumanReviewNeeded=true` on exhaustion
- **Gate audit event**: `ActionAPIVersionGate` emitted with `ambiguous_kind`, `conflicting_groups`, `retry_outcome` fields
- **`retryRCASubmit` correction message**: Includes `"api_version":"apps/v1"` in the example JSON
- **Gate chaining**: `sameKindValidationGate` → `apiVersionValidationGate` sequential execution

### 4.2 Features Not to be Tested

- **E2E with live LLM and CRD collision**: Requires OCP cluster with both OLM and Knative; deferred to post-merge E2E validation
- **Tool-call-based apiVersion inference** (Issue #1044 Option A): Separate feature, not in scope for this gate fix
- **Parser hard-rejection of missing `api_version`**: Parser remains soft (log + proceed); enforcement is in the gate
- **Prompt template changes**: Schema description improvement for `api_version` is a follow-up

### 4.3 Design Decisions

- **Gate exhaustion → `HumanReviewNeeded`**: Security-critical decision. When the LLM fails to provide `api_version` for an ambiguous kind after retry, automated remediation is blocked to prevent incorrect RBAC grants.
- **Dynamic REST mapper over static table**: Uses `ResourcesFor()` for real-time CRD awareness instead of a hardcoded list of known ambiguous kinds. Handles cluster-specific CRD combinations.
- **Correction message includes group names**: The LLM correction message names the specific conflicting API groups (e.g., "operators.coreos.com, messaging.knative.dev") so the LLM can select from its investigation context.

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: TESTING_GUIDELINES.md — Per-Tier Testable Code Coverage (>=80% per tier).

| Tier | Scope | Target | Code Subset |
|------|-------|--------|-------------|
| Unit | `IsAmbiguousKind`, `apiVersionValidationGate`, correction message fix | >=80% | Pure logic: resolver, gate trigger/exhaustion, message formatting |
| Integration | Full `Investigate()` pipeline with ambiguous kind | >=80% | I/O: LLM mock, REST mapper, audit store, enricher |
| E2E | Deferred | N/A | Requires OCP cluster with CRD collision |

### 5.2 TDD Phases

| Phase | Description | Deliverables | Checkpoint |
|-------|-------------|-------------|------------|
| **Phase 1: TDD RED** | Write all failing tests | Test files with `Expect` assertions against unimplemented behavior | CHECKPOINT 1 |
| **Phase 2: TDD GREEN** | Minimal implementation to pass all tests | Production code changes in gates, resolver, investigator | CHECKPOINT 2 |
| **Phase 3: TDD REFACTOR** | Code quality: 100 Go Mistakes audit, lint, dedup | Cleaned code, no new lint errors | CHECKPOINT 3 |

### 5.3 Anti-Pattern Compliance

Per ANTI_PATTERN_DETECTION.md, all tests MUST:

- Test business outcomes, not implementation details (no NULL-TESTING)
- Use real business logic (mock only LLM client, REST mapper, audit store)
- Reference business requirements in test descriptions
- Use `Eventually()` for async operations (none expected in this feature)

---

## 6. Test Design Specification

### 6.1 Unit Tests — `IsAmbiguousKind` Resolver (Tier 1)

**Test file**: `test/unit/kubernautagent/investigator/ambiguous_kind_resolver_test.go`

| Test ID | Description | BR | Category |
|---------|-------------|-----|----------|
| UT-KA-1044-010 | `IsAmbiguousKind` returns true + both GVRs for `Subscription` registered in `operators.coreos.com` and `messaging.knative.dev` | BR-AI-1044 | Happy Path |
| UT-KA-1044-011 | `IsAmbiguousKind` returns false for `Deployment` registered only in `apps/v1` | BR-AI-1044 | Happy Path |
| UT-KA-1044-012 | `IsAmbiguousKind` returns false for unknown kind (mapper returns error) | BR-AI-1044 | Error |
| UT-KA-1044-013 | `IsAmbiguousKind` with empty string kind returns false, no panic | BR-AI-1044 | Nil/Zero |
| UT-KA-1044-014 | `IsAmbiguousKind` with `"../../etc/passwd"` adversarial input returns false, no panic | BR-AI-1044 | Adversarial |
| UT-KA-1044-015 | `IsAmbiguousKind` with Unicode kind `"Ünïcödé"` returns false, no panic | BR-AI-1044 | Adversarial |
| UT-KA-1044-016 | `IsAmbiguousKind` with max-length+1 kind (256 chars) returns false, no panic | BR-AI-1044 | Adversarial |
| UT-KA-1044-017 | `IsAmbiguousKind` for kind in 3+ API groups returns true with all GVRs | BR-AI-1044 | Edge Case |

### 6.2 Unit Tests — `apiVersionValidationGate` (Tier 1)

**Test file**: `test/unit/kubernautagent/investigator/apiversion_gate_test.go`

| Test ID | Description | BR | Category |
|---------|-------------|-----|----------|
| UT-KA-1044-001 | Ambiguous kind, LLM omits `api_version`, gate fires, retry provides `api_version` → result accepted with `api_version` populated | BR-AI-1044 | Happy Path |
| UT-KA-1044-002 | Ambiguous kind, LLM omits `api_version`, gate fires, retry still omits → `HumanReviewNeeded=true`, `HumanReviewReason="rca_incomplete"` | BR-AI-1044 | Security |
| UT-KA-1044-003 | Unambiguous kind (`Deployment`) with empty `api_version` → gate does not fire, result unchanged | BR-AI-1044 | Happy Path |
| UT-KA-1044-004 | `scopeResolver == nil` → gate skips, result unchanged | BR-AI-1044 | Nil/Zero |
| UT-KA-1044-005 | `RemediationTarget.Kind == ""` → gate skips | BR-AI-1044 | Nil/Zero |
| UT-KA-1044-006 | `RemediationTarget.APIVersion` already populated → gate skips (even for ambiguous kind) | BR-AI-1044 | Happy Path |
| UT-KA-1044-007 | Correction message contains both conflicting API group names from mapper | BR-AI-1044 | UX |
| UT-KA-1044-008 | Gate audit event emitted with `ambiguous_kind`, `conflicting_groups`, `retry_outcome` fields | BR-AI-1044 | Observability |
| UT-KA-1044-009 | `IsAmbiguousKind` returns error (mapper failure) → gate skips, result unchanged, error logged | BR-AI-1044 | Error |
| UT-KA-1044-018 | Gate retry: LLM response is unparseable → gate keeps original result | BR-AI-1044 | Error |
| UT-KA-1044-019 | Gate retry: LLM response drops `RemediationTarget` entirely → gate keeps original result | BR-AI-1044 | Error |
| UT-KA-1044-020 | Gate retry: LLM returns empty content → gate keeps original result | BR-AI-1044 | Error |
| UT-KA-1044-021 | Gate retry: LLM client returns error → gate keeps original result | BR-AI-1044 | Error |
| UT-KA-1044-022 | Adversarial `api_version` from LLM: `"../../etc/passwd"` → accepted by gate (validation is downstream in `ParseGroupVersion`) | BR-AI-1044 | Adversarial |
| UT-KA-1044-023 | `HumanReviewNeeded` on exhaustion includes `Warning` with conflicting group names | BR-AI-1044 | Observability |

### 6.3 Unit Tests — `retryRCASubmit` Correction Message (Tier 1)

**Test file**: `test/unit/kubernautagent/investigator/rca_retry_correction_test.go`

| Test ID | Description | BR | Category |
|---------|-------------|-----|----------|
| UT-KA-1044-030 | `retryRCASubmit` correction example JSON includes `"api_version":"apps/v1"` in `remediation_target` | BR-AI-1044 | Spec Compliance |

### 6.4 Integration Tests — Full Pipeline (Tier 2)

**Test file**: `test/integration/kubernautagent/investigator/apiversion_gate_integration_test.go`

| Test ID | Description | BR | Category |
|---------|-------------|-----|----------|
| IT-KA-1044-001 | Full `Investigate()`: signal=Pod, RCA target=Subscription (ambiguous), LLM omits `api_version`, gate fires, retry succeeds → workflow selection proceeds with correct `api_version` | BR-AI-1044 | Happy Path |
| IT-KA-1044-002 | Full `Investigate()`: signal=Pod, RCA target=Subscription (ambiguous), LLM omits `api_version`, gate fires, retry fails → `HumanReviewNeeded=true` | BR-AI-1044 | Security |
| IT-KA-1044-003 | Full `Investigate()`: signal=Pod, RCA target=Deployment (unambiguous), LLM omits `api_version` → gate skips, normal pipeline | BR-AI-1044 | Happy Path |
| IT-KA-1044-004 | Full `Investigate()`: chained gates — signal=Subscription, RCA target=Subscription (same-kind gate fires first, then api_version gate on retry result) | BR-AI-1044 | Cross-Phase |
| IT-KA-1044-005 | Audit event for `api_version_validation_gate` action persisted via `DSAuditStore` with all required fields | BR-AI-1044 | Observability |

---

## 7. BR Coverage Matrix

| BR ID | AC | Description | Test Type | Test ID | Status |
|-------|----|-------------|-----------|---------|--------|
| BR-AI-1044 | AC1 | Gate detects ambiguous kind via REST mapper | Unit | UT-KA-1044-010, -011, -017 | Pending |
| BR-AI-1044 | AC2 | Gate rejects ambiguous kind missing `api_version` and retries | Unit | UT-KA-1044-001 | Pending |
| BR-AI-1044 | AC2 | Gate retry happy path — full pipeline | Integration | IT-KA-1044-001 | Pending |
| BR-AI-1044 | AC3 | Gate exhaustion sets `HumanReviewNeeded=true` (security fail-safe) | Unit | UT-KA-1044-002 | Pending |
| BR-AI-1044 | AC3 | Gate exhaustion — full pipeline | Integration | IT-KA-1044-002 | Pending |
| BR-AI-1044 | AC4 | Unambiguous kinds bypass gate | Unit | UT-KA-1044-003 | Pending |
| BR-AI-1044 | AC4 | Unambiguous kind — full pipeline | Integration | IT-KA-1044-003 | Pending |
| BR-AI-1044 | AC5 | Correction message names conflicting API groups | Unit | UT-KA-1044-007 | Pending |
| BR-AI-1044 | AC6 | Gate emits audit event with ambiguity details | Unit | UT-KA-1044-008 | Pending |
| BR-AI-1044 | AC6 | Audit event — full pipeline | Integration | IT-KA-1044-005 | Pending |
| BR-AI-1044 | AC7 | Nil resolver graceful degradation | Unit | UT-KA-1044-004 | Pending |
| BR-AI-1044 | AC8 | `retryRCASubmit` example includes `api_version` | Unit | UT-KA-1044-030 | Pending |
| BR-AI-1044 | AC9 | Chained gates (same-kind → api_version) | Integration | IT-KA-1044-004 | Pending |

---

## 8. Test Case Specifications

### 8.1 UT-KA-1044-001: Ambiguous kind gate fires, retry succeeds

**BR**: BR-AI-1044 AC2
**Type**: Unit
**Category**: Happy Path
**Priority**: P0

**Preconditions**:
- Fake REST mapper with `Subscription` registered under `operators.coreos.com/v1alpha1` and `messaging.knative.dev/v1`
- Mock LLM client returning retry response with `"api_version": "operators.coreos.com/v1alpha1"`
- Mock audit store

**Steps**:
1. **Given**: An `InvestigationResult` with `RemediationTarget = {Kind: "Subscription", Name: "etcd", Namespace: "demo-operator", APIVersion: ""}`
2. **When**: `apiVersionValidationGate` is called
3. **Then**: The gate detects ambiguity, invokes LLM retry, and returns updated result

**Expected Result**:
- Returned result has `RemediationTarget.APIVersion == "operators.coreos.com/v1alpha1"`
- Returned result has `RemediationTarget.Kind == "Subscription"` (unchanged)
- `HumanReviewNeeded == false`
- Audit event emitted with `retry_outcome == "resolved"`

### 8.2 UT-KA-1044-002: Ambiguous kind gate fires, retry fails — human review (Security)

**BR**: BR-AI-1044 AC3
**Type**: Unit
**Category**: Security
**Priority**: P0

**Preconditions**:
- Fake REST mapper with `Subscription` in two API groups
- Mock LLM client returning retry response *still without* `api_version`
- Mock audit store

**Steps**:
1. **Given**: An `InvestigationResult` with `RemediationTarget = {Kind: "Subscription", Name: "etcd", Namespace: "demo-operator", APIVersion: ""}`
2. **When**: `apiVersionValidationGate` is called
3. **Then**: The gate detects ambiguity, retries, LLM still omits `api_version` → exhaustion

**Expected Result**:
- `result.HumanReviewNeeded == true`
- `result.HumanReviewReason == "rca_incomplete"`
- `result.Warnings` contains string mentioning `"operators.coreos.com"` and `"messaging.knative.dev"`
- Audit event emitted with `retry_outcome == "exhausted"`

### 8.3 UT-KA-1044-003: Unambiguous kind bypasses gate

**BR**: BR-AI-1044 AC4
**Type**: Unit
**Category**: Happy Path
**Priority**: P1

**Preconditions**:
- Fake REST mapper with `Deployment` only in `apps/v1`
- No LLM client call expected

**Steps**:
1. **Given**: An `InvestigationResult` with `RemediationTarget = {Kind: "Deployment", Name: "api-server", Namespace: "production", APIVersion: ""}`
2. **When**: `apiVersionValidationGate` is called
3. **Then**: Gate returns immediately without LLM call

**Expected Result**:
- Result unchanged (no `APIVersion` added, no `HumanReviewNeeded`)
- Mock LLM client received zero calls

### 8.4 UT-KA-1044-007: Correction message names conflicting groups

**BR**: BR-AI-1044 AC5
**Type**: Unit
**Category**: UX
**Priority**: P0

**Preconditions**:
- Fake REST mapper with `Subscription` in `operators.coreos.com/v1alpha1` and `messaging.knative.dev/v1`
- Capture LLM request messages

**Steps**:
1. **Given**: An `InvestigationResult` with ambiguous kind, no `api_version`
2. **When**: `apiVersionValidationGate` fires
3. **Then**: The correction message sent to the LLM is inspected

**Expected Result**:
- Message contains `"operators.coreos.com"`
- Message contains `"messaging.knative.dev"`
- Message contains `"api_version"`
- Message contains `"Subscription"` (the kind)

### 8.5 UT-KA-1044-010: `IsAmbiguousKind` detects multi-group kind

**BR**: BR-AI-1044 AC1
**Type**: Unit
**Category**: Happy Path
**Priority**: P0

**Preconditions**:
- `meta.NewDefaultRESTMapper` with `Subscription` added under two groups

**Steps**:
1. **Given**: A `mapperScopeResolver` backed by the mapper
2. **When**: `IsAmbiguousKind("Subscription")` is called
3. **Then**: Returns `(true, [operators.coreos.com/v1alpha1/subscriptions, messaging.knative.dev/v1/subscriptions], nil)`

**Expected Result**:
- `ambiguous == true`
- `gvrs` has length >= 2
- GVR groups include `operators.coreos.com` and `messaging.knative.dev`

### 8.6 IT-KA-1044-001: Full pipeline — ambiguous kind, retry succeeds

**BR**: BR-AI-1044 AC2
**Type**: Integration
**Category**: Happy Path
**Priority**: P0

**Preconditions**:
- Full `Investigator` with mock LLM, real parser, real enricher with fake K8s client
- Fake REST mapper with `Subscription` in two groups
- Mock LLM sequence: [RCA without api_version] → [gate retry with api_version] → [workflow selection]

**Steps**:
1. **Given**: Signal `{Kind: "Pod", Name: "etcd-operator-xyz", Namespace: "demo-operator"}`
2. **When**: `Investigate()` is called
3. **Then**: Pipeline completes with workflow selected

**Expected Result**:
- `result.RemediationTarget.APIVersion == "operators.coreos.com/v1alpha1"`
- `result.WorkflowID` is non-empty (workflow selection succeeded)
- `result.HumanReviewNeeded == false`
- Mock LLM received exactly 3 calls (RCA, gate retry, workflow)

### 8.7 IT-KA-1044-002: Full pipeline — ambiguous kind, exhaustion → human review

**BR**: BR-AI-1044 AC3
**Type**: Integration
**Category**: Security
**Priority**: P0

**Preconditions**:
- Same as IT-KA-1044-001 but retry response still lacks `api_version`

**Steps**:
1. **Given**: Signal `{Kind: "Pod", Name: "etcd-operator-xyz", Namespace: "demo-operator"}`
2. **When**: `Investigate()` is called
3. **Then**: Pipeline terminates with human review

**Expected Result**:
- `result.HumanReviewNeeded == true`
- `result.HumanReviewReason == "rca_incomplete"`
- Mock LLM received exactly 2 calls (RCA, gate retry — no workflow selection)

---

## 9. Checkpoint Specifications

### CHECKPOINT 1 — After TDD RED Phase

**Gate criteria**: All tests written and verified to FAIL (compile but red).

#### 9-Category Audit

| # | Category | Tests That Satisfy | Notes |
|---|----------|--------------------|-------|
| 1 | **Observability wiring** | UT-KA-1044-008 (audit event fields), IT-KA-1044-005 (audit persistence) | Verify `ActionAPIVersionGate` constant emitted and audit data keys match |
| 2 | **Adversarial inputs** | UT-KA-1044-013 (empty kind), UT-KA-1044-014 (path traversal), UT-KA-1044-015 (Unicode), UT-KA-1044-016 (max-length+1), UT-KA-1044-022 (adversarial `api_version`) | All external string inputs covered |
| 3 | **Resource bounds** | N/A | Gate is stateless per-call; no growing maps/slices/caches. `enrichmentCache` is per-investigation (bounded by `maxTurns`). No new caches introduced. |
| 4 | **Concurrency** | N/A for gate itself (synchronous, single-goroutine per investigation). `IsAmbiguousKind` backed by `meta.RESTMapper` which is thread-safe. No new mutexes or goroutines introduced. |
| 5 | **Nil/zero edge cases** | UT-KA-1044-004 (nil resolver), UT-KA-1044-005 (empty kind), UT-KA-1044-013 (empty string to `IsAmbiguousKind`), UT-KA-1044-012 (unknown kind) | All nil/zero paths covered |
| 6 | **Error-path observability** | UT-KA-1044-009 (mapper error logged with kind), UT-KA-1044-021 (LLM error logged), UT-KA-1044-023 (exhaustion warning with group names) | Every error return has structured log assertion |
| 7 | **Cross-phase integration** | IT-KA-1044-005 (`ActionAPIVersionGate` constant from `emitter.go` used in gate, verified in audit event) | Phase 2 code uses Phase 1 constant |
| 8 | **Spec compliance** | UT-KA-1044-030 (`retryRCASubmit` example matches JSON schema requiring `api_version`) | JSON schema compliance in correction message |
| 9 | **API surface hygiene** | Verify: `IsAmbiguousKind` exported (used by tests + main package), `apiVersionValidationGate` unexported (receiver method). No test helpers exported from production packages. | Audit at checkpoint |

### CHECKPOINT 2 — After TDD GREEN Phase

**Gate criteria**: All tests PASS. `go build ./...` succeeds. `go vet ./...` clean.

#### 9-Category Audit

| # | Category | Verification |
|---|----------|-------------|
| 1 | **Observability wiring** | Run UT-KA-1044-008: assert `ActionAPIVersionGate` event emitted with correct data keys. Verify `ActionAPIVersionGate` is defined in `emitter.go` and the gate code references it. |
| 2 | **Adversarial inputs** | Run UT-KA-1044-013..016, -022: all return false/skip without panic or unexpected behavior. |
| 3 | **Resource bounds** | Code review: no new maps, slices, or caches that grow unboundedly. Gate creates a fixed-size `retryMessages` slice (copy of history + 1 correction message). |
| 4 | **Concurrency** | Run `go test -race ./test/unit/kubernautagent/investigator/... ./test/integration/kubernautagent/investigator/...`: zero races. |
| 5 | **Nil/zero edge cases** | Run UT-KA-1044-004, -005, -012, -013: all pass with correct early-return behavior. |
| 6 | **Error-path observability** | Run UT-KA-1044-009, -021, -023: all error logs include `kind`, `correlation_id`, and (where applicable) `conflicting_groups`. |
| 7 | **Cross-phase integration** | Run IT-KA-1044-005: audit event with `ActionAPIVersionGate` persisted through `DSAuditStore`. Verify `emitter.go` constant is the same string used in `investigator_gates.go`. |
| 8 | **Spec compliance** | Run UT-KA-1044-030: correction example JSON includes `"api_version":"apps/v1"` in `remediation_target`. Verify JSON is valid. |
| 9 | **API surface hygiene** | Run: `grep -r 'func [A-Z]' internal/kubernautagent/investigator/investigator_gates.go` — only `CheckWorkflowTargetAlignment` should be exported (existing). New gate must be unexported. Verify no test helpers in production packages. |

### CHECKPOINT 3 — After TDD REFACTOR Phase

**Gate criteria**: All tests still PASS. Code quality validated.

#### 100 Go Mistakes Audit

| Mistake # | Check | Status |
|-----------|-------|--------|
| #1 | Unintended variable shadowing | Verify no `err :=` inside `if` blocks that shadow outer `err` |
| #2 | Unnecessary nested code | Gate should use early returns for skip conditions |
| #4 | Overusing getters/setters | N/A (no getters introduced) |
| #10 | Not being aware of possible side effects in type embedding | N/A (no embedding) |
| #16 | Not using linters effectively | Run `golangci-lint run --timeout=5m` |
| #26 | Slices and memory leaks | Verify `retryMessages` slice doesn't retain large LLM history references after gate returns |
| #48 | Ignoring `context.Context` | Gate receives and forwards `ctx` to all LLM calls |
| #49 | Not using `errgroup` for goroutine error handling | N/A (no goroutines) |
| #53 | Not handling defer errors | N/A (no defers in gate) |
| #77 | JSON handling mistakes | Verify correction message is valid JSON; `schema.ParseGroupVersion` handles malformed input |
| #78 | Common SQL mistakes | N/A |
| #84 | Not using testing utility packages | Tests use Ginkgo/Gomega (project standard) |
| #91 | Not using `httptest` | N/A (no HTTP in gate) |
| #100 | Not understanding Go diagnostics tooling | `go vet`, `golangci-lint` pass |

#### 9-Category Re-Audit (Refactored Code)

| # | Category | Verification |
|---|----------|-------------|
| 1-9 | All categories | Re-run full test suite: `go test -race -count=1 ./test/unit/kubernautagent/investigator/... ./test/integration/kubernautagent/investigator/... -ginkgo.v` |

Additional REFACTOR checks:
- [ ] No duplicated code between `sameKindValidationGate` and `apiVersionValidationGate` — extract shared patterns if >30 lines duplicated
- [ ] `golangci-lint run --timeout=5m` — zero new errors
- [ ] `go vet ./...` — clean
- [ ] All `Expect` assertions include test ID and business-outcome context string

---

## 10. Implementation Phases (TDD)

### Phase 1: TDD RED — Write Failing Tests

**Files to create**:
1. `test/unit/kubernautagent/investigator/ambiguous_kind_resolver_test.go` — UT-KA-1044-010..017
2. `test/unit/kubernautagent/investigator/apiversion_gate_test.go` — UT-KA-1044-001..009, 018..023
3. `test/unit/kubernautagent/investigator/rca_retry_correction_test.go` — UT-KA-1044-030
4. `test/integration/kubernautagent/investigator/apiversion_gate_integration_test.go` — IT-KA-1044-001..005

**Expected state**: All tests compile but FAIL (no implementation yet).

**Checkpoint 1 gate**: 9-category audit before proceeding.

### Phase 2: TDD GREEN — Minimal Implementation

**Files to modify**:
1. `internal/kubernautagent/investigator/investigator.go` — Add `apiVersionValidationGate` call in `runRCA` after `sameKindValidationGate` (1 line). Fix `retryRCASubmit` correction message (~3 lines).
2. `internal/kubernautagent/investigator/investigator_gates.go` — Add `apiVersionValidationGate` method (~90 lines, following `sameKindValidationGate` pattern).
3. `internal/kubernautagent/investigator/investigator_phases.go` — Extend `ScopeResolver` interface with `IsAmbiguousKind`. Add implementation on `mapperScopeResolver` (~20 lines).
4. `internal/kubernautagent/audit/emitter.go` — Add `ActionAPIVersionGate` constant (1 line).

**Expected state**: All tests PASS. `go build ./...` succeeds.

**Checkpoint 2 gate**: 9-category audit + build validation before proceeding.

### Phase 3: TDD REFACTOR — Code Quality

**Activities**:
1. 100 Go Mistakes audit (table above)
2. Extract shared gate patterns if duplication >30 lines
3. `golangci-lint run --timeout=5m`
4. `go vet ./...`
5. Review all `Expect` assertions for business-outcome wording

**Expected state**: All tests still PASS. No new lint errors. Code quality improved.

**Checkpoint 3 gate**: 9-category re-audit + 100 Go Mistakes verification.

---

## 11. Coverage Targets

| Metric | Target | Actual |
|--------|--------|--------|
| Unit test coverage (gate + resolver) | >=80% | Pending |
| Integration test coverage | >=80% | Pending |
| BR-AI-1044 AC coverage | 100% (9/9 ACs) | Pending |
| Race detector | 0 races | Pending |
| Lint compliance | 0 new errors | Pending |

---

## 12. Execution Commands

```bash
# Unit tests (all #1044 tests)
go test -race -count=1 ./test/unit/kubernautagent/investigator/... -ginkgo.v -ginkgo.focus="1044"

# Integration tests (#1044)
go test -race -count=1 ./test/integration/kubernautagent/investigator/... -ginkgo.v -ginkgo.focus="1044"

# Full investigator suite (regression check)
go test -race -count=1 ./test/unit/kubernautagent/investigator/... ./test/integration/kubernautagent/investigator/... -ginkgo.v

# Coverage
go test -coverprofile=coverage.out ./internal/kubernautagent/investigator/... && go tool cover -func=coverage.out

# Build + lint
go build ./...
golangci-lint run --timeout=5m
```

---

## 13. Dependencies

| Dependency | Version | Usage |
|------------|---------|-------|
| `k8s.io/apimachinery/pkg/api/meta` | v0.35.3 | `RESTMapper.ResourcesFor()`, `NewDefaultRESTMapper` (test fake) |
| `github.com/onsi/ginkgo/v2` | latest | BDD test framework |
| `github.com/onsi/gomega` | latest | Assertion library |
| No new external dependencies | — | Gate uses existing investigator dependencies |

---

## 14. Sign-off

| Role | Name | Date | Status |
|------|------|------|--------|
| Author | AI Assistant | 2026-05-07 | Draft |
| Technical Review | | | Pending |
| QE Review | | | Pending |
| Security Review | | | Pending |

---

## 15. Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-07 | AI Assistant | Initial test plan following IEEE 829-2008 + Kubernaut hybrid format |

---

## Appendix A: Audit Findings from Multi-Persona Readiness Review

The following findings from the pre-implementation audit are addressed by this test plan:

| Audit ID | Finding | Test Coverage |
|----------|---------|---------------|
| GAP-SEC-1 | Gate exhaustion must trigger `HumanReviewNeeded` | UT-KA-1044-002, IT-KA-1044-002 |
| GAP-UX-1 | Correction message must name conflicting groups | UT-KA-1044-007 |
| GAP-QE-3 / GAP-UX-2 | `retryRCASubmit` example omits `api_version` | UT-KA-1044-030 |
| GAP-SEC-2 | Audit trail must capture ambiguity details | UT-KA-1044-008, IT-KA-1044-005 |
| GAP-SEC-3 | Mapper freshness | UT-KA-1044-008 (reset-retry pattern) |
| GAP-PROD-2 | Warning on exhaustion | UT-KA-1044-023 |

## Appendix B: Production Failure Reference

**Incident**: `operator-health` scenario, RR `rr-44c1b6ab3fff-153c2a3f`, OCP 4.21, agent v1.4.0-rc7

**Root cause**: LLM submitted `Subscription/etcd` without `api_version`. Enrichment resolved to `messaging.knative.dev` (Knative) instead of `operators.coreos.com` (OLM). RBAC denied on wrong API group.

**Evidence**: LLM's `kubectl_describe` tool call (tool #18) returned `apiVersion: operators.coreos.com/v1alpha1` — the data was available in the investigation context but not passed through to `submit_result`.

**Fix validation**: IT-KA-1044-001 reproduces this exact scenario (Subscription in two API groups, LLM omits `api_version`, gate forces retry, pipeline succeeds).
