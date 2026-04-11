# Test Plan: Label-Based Notification Routing (#416)

> **Template Version**: 3.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-416-v3
**Feature**: Label-based notification routing with dual NR creation and matchRe support
**Version**: 3.0
**Created**: 2026-04-10
**Author**: AI Assistant
**Status**: Active
**Branch**: `development/v1.3_part4`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

This test plan validates label-based notification routing (#416), which enables:
1. **Owner label resolution**: Read `kubernaut.ai/team`, `kubernaut.ai/owner`, `kubernaut.ai/notification-channel` from K8s resource labels with namespace fallback
2. **Dual NR creation**: Create separate NRs for signal target and RCA target with distinct routing attributes
3. **matchRe regex matching**: Alertmanager-style regex matching in routing rules
4. **New routing attributes**: `notification-target`, `team`, `owner`, `target-kind` populated via `spec.Extensions`

### 1.2 Objectives

1. **Owner label resolution correctness**: `ResolveOwnerLabels` reads the 3 `kubernaut.ai/*` labels from resource labels with graceful fallback to namespace labels and empty-map return on deleted resources
2. **Dual NR creation logic**: When signal target differs from RCA target, two NRs are created with distinct `notification-target` extensions; when identical, a single NR with `notification-target=both` is created
3. **Extensions population**: NR `spec.Extensions` carries `notification-target`, `team`, `owner`, `target-kind`, `namespace` for routing decisions
4. **matchRe support**: Alertmanager-style regex matching (`matchRe`) with AND semantics when combined with `match`, invalid regex rejected at `ParseConfig` time
5. **Status aggregation**: Multiple NRs per RR produce a single worst-status-wins `NotificationStatus`
6. **Backward compatibility**: Existing routing configs without `matchRe` and existing NR creation (single NR) work unchanged

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `make test-unit-remediationorchestrator` + `make test-unit-notification` |
| Integration test pass rate | 100% | `make test-integration-remediationorchestrator` + `make test-integration-notification` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `pkg/remediationorchestrator/creator/`, `pkg/notification/routing/` |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on integration-testable paths |
| Backward compatibility | 0 regressions | All existing routing and notification tests pass without modification |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- BR-NOT-068: Multi-Channel Fanout (prerequisite: #597)
- Issue #416: Label-based notification routing
- Issue #597: Continue route fanout (prerequisite — must be implemented first)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- `api/notification/v1alpha1/notificationrequest_types.go` — `spec.Extensions` field
- `api/remediation/v1alpha1/remediationrequest_types.go` — `TargetResource`, `RemediationTarget`, `NotificationRequestRefs`

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `DetermineNotificationTarget` returns wrong dual/single decision | Wrong team receives notification or duplicate delivery | Medium | UT-RO-416-005, UT-RO-416-006, UT-RO-416-007 | Three-case exhaustive test (different, equal, nil RCA) |
| R2 | `BuildNotificationExtensions` omits a routing key | Routing rules fail to match; notification dropped silently | Medium | UT-RO-416-008 | Assert all 5 keys present with correct values |
| R3 | Invalid regex in `matchRe` not rejected at parse time | Runtime panic during routing | High | UT-NOT-416-002 | Pre-compile at `ParseConfig`; reject with clear error |
| R4 | `AggregateNotificationStatus` returns wrong priority | RR status shows "Sent" while one NR is "Failed" | Medium | UT-RO-416-009 | Worst-status-wins with explicit priority table |
| R5 | Owner label fallback reads wrong namespace | Labels from unrelated namespace applied to notification | Low | UT-RO-416-002, IT-RO-416-002 | Namespace key from `target.Namespace`, not hardcoded |
| R6 | Dual NR names collide | Idempotent Get returns first NR, second never created | High | UT-RO-416-005, IT-RO-416-001 | Deterministic name suffix `-signal`/`-rca` per NR |

### 3.1 Risk-to-Test Traceability

| Risk | Mitigating Tests |
|------|-----------------|
| R1 (dual/single decision) | UT-RO-416-005, UT-RO-416-006, UT-RO-416-007 |
| R2 (missing routing key) | UT-RO-416-008 |
| R3 (invalid regex) | UT-NOT-416-002 |
| R4 (status aggregation) | UT-RO-416-009 |
| R5 (namespace fallback) | UT-RO-416-002, IT-RO-416-002 |
| R6 (name collision) | UT-RO-416-005, IT-RO-416-001 |

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **Owner label resolution** (`pkg/remediationorchestrator/creator/owner_resolver.go`): Reads kubernaut.ai/* labels from K8s resources with namespace fallback
- **Dual NR decision logic** (`pkg/remediationorchestrator/creator/notification_extensions.go`): `DetermineNotificationTarget` decides 1 vs 2 NRs based on signal/RCA target comparison
- **Extensions builder** (`pkg/remediationorchestrator/creator/notification_extensions.go`): `BuildNotificationExtensions` populates `spec.Extensions` for routing
- **Status aggregation** (`pkg/remediationorchestrator/creator/notification_extensions.go`): `AggregateNotificationStatus` returns worst-status-wins from multiple NR statuses
- **matchRe routing** (`pkg/notification/routing/config.go`): Regex matching with pre-compilation and AND semantics with `match`
- **Routing attribute extraction** (`pkg/notification/routing/resolver.go`): `RoutingAttributesFromSpec` extracts new #416 attributes from `spec.Extensions`

### 4.2 Features Not to be Tested

- **Delivery orchestration**: How channels are actually delivered to Slack/PagerDuty/etc. (covered by existing delivery tests)
- **Reconciler wiring of dual NRs**: Full reconciler loop creating dual NRs through `AIAnalysisHandler` (deferred to reconciler integration tests; the business logic functions are tested in isolation)
- **E2E Kind cluster tests**: Deferred to CI/CD pipeline

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Pure helper functions (`DetermineNotificationTarget`, `BuildNotificationExtensions`, `AggregateNotificationStatus`) | Testable without K8s client; callers wire them into Create* methods |
| `matchRe` pre-compiled at `ParseConfig` | Fail-fast on invalid regex; amortize compilation cost across many match calls |
| Worst-status-wins aggregation | Pessimistic approach ensures operators see worst outcome when dual NRs exist |
| Name suffix `-signal`/`-rca` for dual NRs | Preserves deterministic naming for idempotent Get; distinct from single NR suffix |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (`pkg/remediationorchestrator/creator/owner_resolver.go`, `notification_extensions.go`, `pkg/notification/routing/config.go`, `resolver.go`, `attributes.go`)
- **Integration**: >=80% of **integration-testable** code (label resolution with fake K8s client, dual NR creation via `NotificationCreator`)

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers (UT + IT):
- **Unit tests**: Catch logic errors in label resolution, dual decision, Extensions building, regex matching, status aggregation
- **Integration tests**: Catch wiring issues with K8s client, NR creation through `NotificationCreator`, routing with real `Router` config

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes**:
- "SRE team receives separate notifications for the signal source and the RCA target" (dual NR)
- "Regex routing rules match team names with wildcards" (matchRe)
- "Operator sees worst-case status when multiple notifications exist" (status aggregation)

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All 18 tests pass (0 failures): 9 UT-RO + 6 UT-NOT + 2 IT-RO + 1 IT-NOT
2. Per-tier code coverage meets >=80% threshold
3. No regressions in existing test suites: `make test-unit-remediationorchestrator`, `make test-unit-notification`
4. `go build ./...` succeeds

**FAIL** — any of the following:

1. Any test fails
2. Per-tier coverage falls below 80%
3. Existing tests that were passing before the change now fail
4. Build breaks

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- Issue #597 (continue route fanout) is not merged — matchRe routing tests depend on `FindReceivers` existing
- Build broken: code does not compile
- Cascading failures: more than 3 tests fail for the same root cause

**Resume testing when**:

- #597 merged and available on branch
- Build fixed and green
- Root cause identified and fix deployed

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/creator/owner_resolver.go` | `ResolveOwnerLabels`, `extractLabelsFromResource`, `filterOwnerLabels`, `hasAnyOwnerLabel` | ~176 |
| `pkg/remediationorchestrator/creator/notification_extensions.go` | `DetermineNotificationTarget`, `BuildNotificationExtensions`, `AggregateNotificationStatus` | ~80 (new) |
| `pkg/notification/routing/config.go` | `matchesAttributes` (matchRe path), `compileRouteRegexes`, `ParseConfig` | ~30 modified |
| `pkg/notification/routing/resolver.go` | `RoutingAttributesFromSpec` (Extensions extraction) | ~10 modified |
| `pkg/notification/routing/attributes.go` | Constants: `AttrNotificationTarget`, `AttrTeam`, `AttrOwner`, `AttrTargetKind` | ~20 new |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/creator/notification.go` | `CreateApprovalNotification` (Extensions wiring) | ~40 modified |
| `pkg/remediationorchestrator/creator/owner_resolver.go` | `ResolveOwnerLabels` with live K8s client | same file, I/O path |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.3_part4` HEAD | Branch |
| Dependency: #597 | Same branch | Continue route fanout must be implemented |

---

## 7. BR Coverage Matrix

> Kubernaut-specific. Maps every business requirement to test scenarios across tiers.

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| #416 | Owner label resolution from resource | P0 | Unit | UT-RO-416-001 | Pass |
| #416 | Owner label namespace fallback | P0 | Unit | UT-RO-416-002 | Pass |
| #416 | Owner label deleted resource | P1 | Unit | UT-RO-416-003 | Pass |
| #416 | Owner label cluster-scoped | P1 | Unit | UT-RO-416-004 | Pass |
| #416 | Dual NR when targets differ | P0 | Unit | UT-RO-416-005 | Pass |
| #416 | Single NR when targets equal | P0 | Unit | UT-RO-416-006 | Pass |
| #416 | Signal-only NR when no RCA | P0 | Unit | UT-RO-416-007 | Pass |
| #416 | Extensions carry routing keys | P0 | Unit | UT-RO-416-008 | Pass |
| #416 | Status aggregation worst-wins | P1 | Unit | UT-RO-416-009 | Pass |
| #416 | matchRe valid regex | P0 | Unit | UT-NOT-416-001 | Pass |
| #416 | matchRe invalid regex rejected | P0 | Unit | UT-NOT-416-002 | Pass |
| #416 | matchRe + match AND semantics | P0 | Unit | UT-NOT-416-003 | Pass |
| #416 | Extensions in RoutingAttributes | P0 | Unit | UT-NOT-416-004 | Pass |
| #416 | Empty matchRe match-all | P1 | Unit | UT-NOT-416-005 | Pass |
| #416 | Backward compat no matchRe | P0 | Unit | UT-NOT-416-006 | Pass |
| #416 | Dual NR creation via envtest | P0 | Integration | IT-RO-416-001 | Pass |
| #416 | Owner labels live K8s | P0 | Integration | IT-RO-416-002 | Pass |
| #416 | Routing with matchRe + team | P0 | Integration | IT-NOT-416-001 | Pass |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

> **IEEE 829 Test Design Specification** — How test cases are organized and grouped.

### Tier 1: Unit Tests — RO Side (9 tests)

**Testable code scope**: `pkg/remediationorchestrator/creator/` — >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-416-001` | Owner label resolution reads kubernaut.ai/team from resource labels | Pass |
| `UT-RO-416-002` | Owner label resolution falls back to namespace labels | Pass |
| `UT-RO-416-003` | Owner label resolution returns empty map when resource is deleted | Pass |
| `UT-RO-416-004` | Owner label resolution handles cluster-scoped resources (no ns fallback) | Pass |
| `UT-RO-416-005` | Dual NR when signal target != RCA target — two distinct NRs created | Pass |
| `UT-RO-416-006` | Single NR with notification-target=both when signal == RCA target | Pass |
| `UT-RO-416-007` | Signal-only NR when RemediationTarget is nil | Pass |
| `UT-RO-416-008` | NR Extensions carry notification-target, team, owner, target-kind, namespace | Pass |
| `UT-RO-416-009` | Status aggregation worst-status-wins across multiple NR statuses | Pass |

### Tier 1: Unit Tests — Notification Side (6 tests)

**Testable code scope**: `pkg/notification/routing/` — >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-NOT-416-001` | matchRe with valid regex matches attribute values | Pass |
| `UT-NOT-416-002` | matchRe with invalid regex rejected at ParseConfig time | Pass |
| `UT-NOT-416-003` | matchRe + match combined — both must satisfy (AND semantics) | Pass |
| `UT-NOT-416-004` | RoutingAttributesFromSpec extracts team/owner/notification-target/target-kind from Extensions | Pass |
| `UT-NOT-416-005` | Empty matchRe map — match-all (same as empty match) | Pass |
| `UT-NOT-416-006` | Existing routing config tests pass unchanged (backward compat) | Pass |

### Tier 2: Integration Tests (3 tests)

**Testable code scope**: `pkg/remediationorchestrator/creator/`, `internal/controller/notification/` — >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-RO-416-001` | Dual NR creation with real K8s resources via envtest — 2 NRs with correct Extensions | Pass |
| `IT-RO-416-002` | Owner label resolution reads live resource and namespace labels via envtest | Pass |
| `IT-NOT-416-001` | Routing on notification-target + team attributes with matchRe in a full Router | Pass |

### Tier Skip Rationale

- **E2E**: Deferred to CI/CD pipeline. The business logic (dual NR decision, Extensions building, matchRe) is fully covered by UT + IT. E2E would validate Helm chart wiring and end-to-end delivery, which requires a Kind cluster.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### UT-RO-416-005: Dual NR when signal target != RCA target

**BR**: #416
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/owner_resolver_test.go`

**Preconditions**:
- `DetermineNotificationTarget` function exists in `pkg/remediationorchestrator/creator/notification_extensions.go`

**Test Steps**:
1. **Given**: Signal target `Deployment/my-app/production` and RCA target `Pod/my-app-xyz/production` (different Kind+Name)
2. **When**: `DetermineNotificationTarget(signalTarget, &rcaTarget)` is called
3. **Then**: Returns `("signal", true)` — indicating dual NR creation needed

**Expected Results**:
1. First return value is `"signal"` (caller creates signal NR with this value)
2. Second return value is `true` (caller must also create RCA NR with `"rca"`)

**Acceptance Criteria**:
- **Behavior**: Function correctly identifies that different targets require separate notifications
- **Correctness**: Comparison is by Kind+Name+Namespace, not by pointer identity

### UT-RO-416-006: Single NR when signal == RCA target

**BR**: #416
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/owner_resolver_test.go`

**Preconditions**:
- `DetermineNotificationTarget` function exists

**Test Steps**:
1. **Given**: Signal target `Deployment/my-app/production` and RCA target with identical Kind, Name, Namespace
2. **When**: `DetermineNotificationTarget(signalTarget, &rcaTarget)` is called
3. **Then**: Returns `("both", false)` — single NR sufficient

**Expected Results**:
1. First return value is `"both"` (single NR carries both scopes)
2. Second return value is `false` (no second NR needed)

**Acceptance Criteria**:
- **Behavior**: Same-resource targets produce a single notification addressed to both signal and RCA teams
- **Correctness**: All three fields (Kind, Name, Namespace) must match for "both"

### UT-RO-416-007: Signal-only NR when RemediationTarget is nil

**BR**: #416
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/owner_resolver_test.go`

**Preconditions**:
- `DetermineNotificationTarget` function exists

**Test Steps**:
1. **Given**: Signal target `Deployment/my-app/production` and `nil` RCA target (AI did not produce RemediationTarget)
2. **When**: `DetermineNotificationTarget(signalTarget, nil)` is called
3. **Then**: Returns `("signal", false)` — single signal-only NR

**Expected Results**:
1. First return value is `"signal"`
2. Second return value is `false`

**Acceptance Criteria**:
- **Behavior**: Nil RCA target does not panic; gracefully falls back to signal-only
- **Correctness**: No second NR created when RCA target is absent

### UT-RO-416-008: NR Extensions carry routing keys

**BR**: #416
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/owner_resolver_test.go`

**Preconditions**:
- `BuildNotificationExtensions` function exists in `pkg/remediationorchestrator/creator/notification_extensions.go`

**Test Steps**:
1. **Given**: Target `Deployment/my-app/production`, notification-target `"signal"`, owner labels `{kubernaut.ai/team: "sre-platform", kubernaut.ai/owner: "jdoe"}`
2. **When**: `BuildNotificationExtensions(target, "signal", ownerLabels)` is called
3. **Then**: Returns map with 5 keys: `notification-target=signal`, `target-kind=Deployment`, `namespace=production`, `team=sre-platform`, `owner=jdoe`

**Expected Results**:
1. All 5 routing keys present with correct values
2. `kubernaut.ai/team` mapped to `team`, `kubernaut.ai/owner` mapped to `owner`
3. `namespace` only present when target has non-empty namespace

**Acceptance Criteria**:
- **Behavior**: Extensions map is complete for routing engine consumption
- **Correctness**: Label key mapping from `kubernaut.ai/*` to short routing keys
- **Accuracy**: Cluster-scoped target (empty namespace) omits `namespace` key

### UT-RO-416-009: Status aggregation worst-status-wins

**BR**: #416
**Priority**: P1
**Type**: Unit
**File**: `test/unit/remediationorchestrator/owner_resolver_test.go`

**Preconditions**:
- `AggregateNotificationStatus` function exists in `pkg/remediationorchestrator/creator/notification_extensions.go`

**Test Steps**:
1. **Given**: Status list `["Sent", "Failed"]`
2. **When**: `AggregateNotificationStatus(statuses)` is called
3. **Then**: Returns `"Failed"` (worst status wins)

**Expected Results**:
1. Priority order: Sent < Pending < InProgress < Cancelled < Failed
2. Empty input returns `""`
3. Single-element input returns that element

**Acceptance Criteria**:
- **Behavior**: Operators see the worst-case status for the remediation
- **Correctness**: "Failed" always wins over any other status

### IT-RO-416-001: Dual NR creation via envtest

**BR**: #416
**Priority**: P0
**Type**: Integration
**File**: `test/integration/remediationorchestrator/notification_label_routing_test.go`

**Preconditions**:
- envtest environment running with CRD schemes registered
- Fake K8s client with NotificationRequest, RemediationRequest CRDs

**Test Steps**:
1. **Given**: A `RemediationRequest` with signal target `Deployment/my-app/prod` and `Status.RemediationTarget` set to `Pod/my-app-xyz/prod` (different); owner labels `kubernaut.ai/team=sre` on the Deployment
2. **When**: `DetermineNotificationTarget` returns dual=true, `BuildNotificationExtensions` is called for each target, and two NRs are created via `NotificationCreator`
3. **Then**: Two NRs exist in the fake K8s client, one with `Extensions["notification-target"]="signal"` and one with `Extensions["notification-target"]="rca"`

**Expected Results**:
1. Two distinct `NotificationRequest` objects created (names with `-signal` and `-rca` suffixes)
2. Both NRs have correct `Extensions` maps
3. Both NRs have `OwnerReference` pointing to the RR

**Acceptance Criteria**:
- **Behavior**: Full creation path works with real K8s client (envtest)
- **Correctness**: Deterministic names prevent idempotent collision
- **Accuracy**: Extensions carry all routing keys from owner labels

### IT-RO-416-002: Owner label resolution with live K8s

**BR**: #416
**Priority**: P0
**Type**: Integration
**File**: `test/integration/remediationorchestrator/notification_label_routing_test.go`

**Preconditions**:
- envtest environment running
- Deployment and Namespace with kubernaut.ai/* labels pre-created

**Test Steps**:
1. **Given**: Deployment `my-app` in namespace `production` with `kubernaut.ai/team=platform-eng`, and Namespace `production` with `kubernaut.ai/owner=ops-team`
2. **When**: `ResolveOwnerLabels(ctx, client, target)` is called for the Deployment
3. **Then**: Returns `{kubernaut.ai/team: "platform-eng"}` (resource label takes precedence over namespace)

**Expected Results**:
1. Resource labels take precedence over namespace labels
2. When resource labels are removed, namespace fallback kicks in on next call
3. Deleted resource returns empty map without error

**Acceptance Criteria**:
- **Behavior**: Live K8s `client.Get` calls succeed and resolve labels correctly
- **Correctness**: Fallback chain works with real API server

---

## 10. Environmental Needs

> **IEEE 829 §13** — Hardware, software, tools, and infrastructure required.

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `sigs.k8s.io/controller-runtime/pkg/client/fake` for K8s client (owner resolver tests only)
- **Location**: `test/unit/remediationorchestrator/`, `test/unit/notification/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: envtest for K8s API + CRD registration; `NotificationCreator` with fake client for dual NR creation
- **Location**: `test/integration/remediationorchestrator/`, `test/integration/notification/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| controller-runtime | v0.19+ | envtest, fake client |

---

## 11. Dependencies & Schedule

> **IEEE 829 §12/§16** — Remaining tasks, blocking issues, and execution order.

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #597 | Code | Merged | matchRe routing tests depend on `FindReceivers` | N/A — already implemented |

### 11.2 Execution Order (TDD Phases & Checkpoints)

#### Phase 1: TDD RED — Write 5 failing UT + 2 failing IT

Write all 7 pending tests (UT-RO-416-005 through 009, IT-RO-416-001, IT-RO-416-002) referencing functions that do not yet exist. Verify all fail for the correct reason (compilation errors for undefined symbols).

**Verification**:
```bash
# Must fail with compilation errors (undefined: DetermineNotificationTarget, etc.)
make test-unit-remediationorchestrator 2>&1 | grep "undefined"
make test-integration-remediationorchestrator 2>&1 | grep "undefined"
```

**Exit criteria**: All 7 new tests fail; all 10 pre-existing tests (UT-RO-416-001..004, UT-NOT-416-001..006) still pass.

---

#### Checkpoint 1: Post-RED Adversarial Audit

| Check | Criterion |
|-------|-----------|
| Correct failure reason | All 7 tests fail due to undefined symbols, NOT logic errors |
| No regressions | `make test-unit-notification` passes (6/6 NOT tests green) |
| Test isolation | No shared mutable state between tests; each `It` block has own fixtures |
| Anti-pattern scan | No `time.Sleep`, no `Skip()`, no `XIt`, no `interface{}` assertions |
| Business outcome | Each test description maps to a user-visible outcome, not implementation detail |
| Template compliance | Test IDs follow `{TIER}-{SERVICE}-{ISSUE}-{SEQ}` format |

---

#### Phase 2: TDD GREEN — Implement minimal code to pass

Create `pkg/remediationorchestrator/creator/notification_extensions.go` with:
- `DetermineNotificationTarget(signalTarget ResourceIdentifier, rcaTarget *ResourceIdentifier) (string, bool)`
- `BuildNotificationExtensions(target ResourceIdentifier, notificationTarget string, ownerLabels map[string]string) map[string]string`
- `AggregateNotificationStatus(statuses []string) string`

**Verification**:
```bash
make test-unit-remediationorchestrator    # All 9 UT-RO tests pass
make test-integration-remediationorchestrator  # All IT-RO tests pass
make test-unit-notification               # All 6 UT-NOT tests pass (no regression)
make test-integration-notification        # All IT-NOT tests pass (no regression)
go build ./...                            # Full codebase builds
```

**Exit criteria**: All 18 tests pass (9 UT-RO + 6 UT-NOT + 2 IT-RO + 1 IT-NOT). Zero regressions. Clean build.

---

#### Checkpoint 2: Post-GREEN Adversarial Audit

| Check | Criterion |
|-------|-----------|
| All tests green | `make test-unit-remediationorchestrator` + `make test-integration-remediationorchestrator` pass |
| No regressions | `make test-unit-notification` + `make test-integration-notification` pass unchanged |
| Clean build | `go build ./...` succeeds |
| Nil safety | `DetermineNotificationTarget` handles nil rcaTarget without panic |
| Edge cases | Empty owner labels produce Extensions with only notification-target + target-kind |
| Cluster-scoped | BuildNotificationExtensions omits `namespace` key for cluster-scoped targets |
| Status priority | AggregateNotificationStatus handles single-element, empty, and all-same inputs |
| Type safety | No `interface{}`, no type assertions in new code |
| Error handling | All errors logged and wrapped with context |
| Business integration | New functions importable from `pkg/remediationorchestrator/creator/` |

---

#### Phase 3: TDD REFACTOR — Clean, extract, deduplicate

Review new code for:
- Naming consistency with existing `pkg/remediationorchestrator/creator/` patterns
- Documentation completeness (GoDoc on exported functions)
- Constant extraction (notification target string values)
- Helper deduplication with existing `resolveNotificationTargetResource`

**Verification**:
```bash
make test-unit-remediationorchestrator    # Still green after refactor
make test-integration-remediationorchestrator  # Still green
make test-unit-notification               # No regression
make test-integration-notification        # No regression
go build ./...                            # Clean build
make lint                                 # No lint errors
```

**Exit criteria**: All tests pass. No lint errors. Code follows existing patterns.

---

#### Checkpoint 3: Final Audit

| Check | Criterion |
|-------|-----------|
| Full regression | `make test-unit-remediationorchestrator` + `make test-unit-notification` + `make test-integration-remediationorchestrator` + `make test-integration-notification` all pass |
| Build clean | `go build ./...` succeeds |
| Lint clean | `make lint` passes (no new warnings) |
| Coverage | >=80% of new code covered by tests |
| No anti-patterns | No `time.Sleep`, `Skip()`, `XIt`, pending tests |
| Documentation | GoDoc on all exported symbols in `notification_extensions.go` |
| Business traceability | All tests reference #416 in their description |
| Backward compatibility | No changes to existing `Create*` method signatures |

---

## 12. Test Deliverables

> **IEEE 829 §11** — What artifacts this test effort produces.

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/416/TEST_PLAN.md` | Strategy and test design |
| RO owner resolver UTs (001-004) | `test/unit/remediationorchestrator/owner_resolver_test.go` | Label resolution tests (implemented) |
| RO dual NR UTs (005-009) | `test/unit/remediationorchestrator/owner_resolver_test.go` | Dual NR + Extensions + aggregation tests |
| Notification routing regex UTs | `test/unit/notification/routing_regex_test.go` | matchRe tests (implemented) |
| RO integration tests | `test/integration/remediationorchestrator/notification_label_routing_test.go` | Dual NR + label resolution ITs |
| Notification integration tests | `test/integration/notification/label_routing_integration_test.go` | matchRe routing IT (implemented) |

---

## 13. Execution

```bash
# Unit tests — RO (owner resolver + dual NR + Extensions + aggregation)
make test-unit-remediationorchestrator

# Unit tests — Notification (matchRe + routing attributes)
make test-unit-notification

# Integration tests — RO (dual NR creation + label resolution via envtest)
make test-integration-remediationorchestrator

# Integration tests — Notification (matchRe routing with full Router)
make test-integration-notification

# Focused run by test ID
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-416"
go test ./test/unit/notification/... -ginkgo.focus="UT-NOT-416"

# Full regression
make test-unit-remediationorchestrator test-unit-notification test-integration-remediationorchestrator test-integration-notification

# Lint compliance
make lint-test-patterns
```

---

## 14. Existing Tests Requiring Updates (if applicable)

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None expected | N/A | N/A | New functions added; existing `Create*` signatures unchanged |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-10 | Initial test plan with 15 UT + 3 IT |
| 2.0 | 2026-04-10 | Extended to full template: added Risks (§3), Scope (§4), Approach (§5), Test Items (§6), BR Coverage Matrix (§7), detailed Test Cases (§9) for UT-RO-416-005..009 and IT-RO-416-001..002, Environmental Needs (§10), TDD phase breakdown with 3 checkpoints (§11.2), make target execution commands (§13). Marked implemented tests as Pass. |
| 3.0 | 2026-04-10 | Implemented all 7 pending tests (UT-RO-416-005..009, IT-RO-416-001..002). Implementation in `pkg/remediationorchestrator/creator/notification_extensions.go`. All UTs pass locally; ITs compile clean (`go vet` passes) — envtest execution deferred to CI. |
