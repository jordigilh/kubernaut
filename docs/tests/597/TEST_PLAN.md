# Test Plan: Route Fanout (Continue Flag) — BR-NOT-068

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-597-v4
**Feature**: Implement `continue: true` route fanout in notification routing engine
**Version**: 4.0
**Created**: 2026-04-09
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.3_part2`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the implementation of BR-NOT-068 (Multi-Channel Fanout) in the
notification routing engine. The `Route.Continue` field exists in the configuration schema
but is ignored at runtime. This plan ensures that `continue: true` routes fan out to
multiple receivers, matching Alertmanager semantics, and that the existing single-match
behavior is preserved when `continue` is false or absent.

### 1.2 Objectives

1. **Fanout correctness**: When `continue: true`, all matching sibling routes are collected and their channels aggregated for delivery
2. **Backward compatibility**: Existing routing behavior (first-match-wins when `continue` is false) is preserved with zero regressions
3. **Deduplication**: When the same receiver appears in multiple matching routes, receivers are deduplicated at the `Route.FindReceivers` level (canonical location)
4. **Controller integration**: The production controller path (`routing_handler.go`) correctly resolves multi-receiver channels and formats the `RoutingResolved` condition
5. **End-to-end delivery**: Multi-receiver fanout produces actual delivery to all matched channels in a Kind cluster
6. **Coverage**: >=80% of unit-testable code per tier

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/notification/... -ginkgo.focus="597"` |
| Integration test pass rate | 100% | `go test ./test/integration/notification/... -ginkgo.focus="597"` |
| E2E test pass rate | 100% | `go test ./test/e2e/notification/... -ginkgo.focus="597"` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `pkg/notification/routing/config.go` |
| Backward compatibility | 0 regressions | All existing tests pass without modification |
| BR-NOT-068 acceptance criteria | See Section 7 | Per BR Coverage Matrix |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-NOT-068**: Multi-Channel Fanout — `docs/services/crd-controllers/06-notification/BUSINESS_REQUIREMENTS.md` line 607
- **BR-NOT-065**: Channel Routing Based on Spec Fields
- **BR-NOT-066**: Alertmanager-Compatible Configuration Format
- **BR-NOT-069**: Routing Rule Visibility via Kubernetes Conditions
- **Issue #597**: `Continue` flag parsed but ignored in `FindReceiver`

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- Alertmanager routing tree semantics: `Route.Continue` in `github.com/prometheus/alertmanager/dispatch`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Breaking existing single-match routing | All notifications route to wrong receiver | Medium | UT-NOT-597-002, UT-NOT-597-006, IT-NOT-597-001 | `FindReceiver` (singular) delegates to `FindReceivers` and takes first element — regression tests validate |
| R2 | Duplicate channels delivered to orchestrator | Same message delivered twice to same Slack channel | Medium | UT-NOT-597-005 | Deduplication by receiver name inside `Route.FindReceivers` (canonical location) |
| R3 | `Router.FindReceiver` callers not updated | Controller uses stale single-receiver path | Low | Existing `routing_hotreload_test.go` tests | Backward-compat wrapper preserves `FindReceiver` API |
| R4 | Condition message (`RoutingResolved`) truncated for many receivers | Operator cannot debug routing | Low | UT-NOT-597-009, IT-NOT-597-001 | Format helper handles multi-receiver display |
| R5 | Channel aggregation logic in controller incorrect | Channels from some receivers missing | Medium | UT-NOT-597-010, IT-NOT-597-001 | Dedicated unit + integration tests |
| R6 | `Router.FindReceivers` nil/empty config fallback wrong | Panic or no channels returned | Medium | UT-NOT-597-008 | Explicit nil-handling test |
| R7 | Fanout route present from initial deployment | Parallel test interference if config mutated at runtime | Low | IT-NOT-597-001, E2E-NOT-597-001 | Fanout routes added to INITIAL suite/E2E configs — no runtime mutation; parallel-safe |

### 3.1 Risk-to-Test Traceability

- **R1** (breaking single-match): Mitigated by UT-NOT-597-002, IT-NOT-597-001 (backward-compat scenario), and all existing tests passing unchanged
- **R2** (duplicate channels): Mitigated by UT-NOT-597-005 (deduplication test)
- **R5** (channel aggregation): Mitigated by UT-NOT-597-010 + IT-NOT-597-001 (real controller delivers to all channels)
- **R7** (parallel safety): Mitigated by adding fanout routes to initial suite configs — no runtime mutation in IT or E2E

---

## 4. Scope

### 4.1 Features to be Tested

- **Route.FindReceivers** (`pkg/notification/routing/config.go`): New method implementing multi-receiver collection with `Continue` flag
- **Route.FindReceiver** (`pkg/notification/routing/config.go`): Updated to delegate to `FindReceivers` for backward compatibility
- **Router.FindReceivers** (`pkg/notification/routing/router.go`): Thread-safe multi-receiver resolution with nil-handling and fallback
- **ResolveChannelsForNotification** (`pkg/notification/routing/resolver.go`): Updated for consistency (test utility, not production path)
- **resolveChannelsFromRoutingWithDetails** (`internal/controller/notification/routing_handler.go`): Production path updated for multi-receiver channel aggregation and condition formatting
- **Controller wiring** (integration): Real controller with envtest resolves multi-receiver channels and delivers via orchestrator
- **End-to-end delivery** (E2E): Pre-deployed ConfigMap with fanout routes + multi-receiver delivery in Kind cluster

### 4.2 Features Not to be Tested

- **DeliverToChannels** (`pkg/notification/delivery/orchestrator.go`): Already supports `[]Channel` — no changes needed
- **matchesAttributes** (`pkg/notification/routing/config.go`): Matching logic unchanged
- **ParseConfig / Validate**: Configuration parsing unchanged — `Continue` field already parsed correctly

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Add `FindReceivers` (plural) alongside `FindReceiver` | Backward compatibility — existing callers unchanged; new callers opt in to multi-receiver |
| Deduplication by receiver name inside `Route.FindReceivers` | Canonical single location for dedup; prevents ambiguity (Audit F6) |
| Integration test uses pre-loaded fanout routes in suite config | Parallel-safe: no runtime mutation of shared `testRouter`; routes present from suite startup |
| E2E fanout routes in initial ConfigMap | Parallel-safe: no runtime ConfigMap mutation; routes present from cluster bootstrap |

### 4.4 Production Path vs. Test Utilities

> `ResolveChannelsForNotification` in `resolver.go` is NOT the production controller path.
> It is only called from `test/unit/notification/routing_integration_test.go`.
>
> The **production controller path** is:
> `notificationrequest_controller.go` → `resolveChannelsFromRoutingWithDetails` (routing_handler.go:70)
>   → `r.Router.FindReceiver(routingAttrs)` (router.go:128) → `r.config.Route.FindReceiver(attrs)` (config.go:289)

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code in `pkg/notification/routing/config.go`
- **Integration**: >=80% of integration-testable code in `internal/controller/notification/routing_handler.go` (controller wiring + channel aggregation)
- **E2E**: Full stack validation (ConfigMap → routing → delivery → status)

### 5.2 Three-Tier Coverage

BR-NOT-068 is covered across all three tiers:

- **Unit tests** (new): UT-NOT-597-001..010 — routing logic, Router wrapper, condition formatting
- **Unit tests** (existing): `routing_hotreload_test.go`, `routing_config_test.go` — backward compat regression guard
- **Integration test** (new): IT-NOT-597-001 — real controller (envtest) + pre-loaded `continue: true` routing config + multi-channel delivery verification
- **E2E test** (new): E2E-NOT-597-001 — Kind cluster + ConfigMap deployment + multi-receiver delivery + per-channel status

### 5.3 Business Outcome Quality Bar

Tests validate that operators configuring `continue: true` in their routing YAML get notifications delivered to **all** matching receivers, not just the first — matching documented Alertmanager behavior. Integration and E2E tests validate the actual delivery path, not just routing resolution.

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All UT-NOT-597-* tests pass (0 failures)
2. IT-NOT-597-001 passes (0 failures)
3. E2E-NOT-597-001 passes (0 failures)
4. All existing tests pass without modification
5. Unit coverage of `pkg/notification/routing/config.go` >=80%

**FAIL** — any of the following:

1. Any new test fails
2. Coverage below 80%
3. Any existing test regresses

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Build broken — code does not compile
- Existing routing tests fail before new code is written
- Integration infrastructure (PostgreSQL, DataStorage) unavailable
- Kind cluster provisioning fails

**Resume testing when**:
- Build fixed and green
- Infrastructure restored

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/notification/routing/config.go` | `FindReceivers` (new), `FindReceiver` (updated) | ~40 new |
| `pkg/notification/routing/router.go` | `FindReceivers` (new) | ~25 new |
| `pkg/notification/routing/resolver.go` | `ResolveChannelsForNotification` (updated, test utility) | ~10 changed |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/notification/routing_handler.go` | `resolveChannelsFromRoutingWithDetails` (updated) | ~15 changed |

### 6.3 E2E-Testable Code (full stack)

| Component | What is validated |
|-----------|-------------------|
| Routing ConfigMap hot-reload | `continue: true` config deployed and picked up by controller |
| Multi-receiver channel resolution | Controller resolves channels from all matched receivers |
| Delivery orchestrator fanout | Notifications delivered to all channels (console + file) |
| Per-channel delivery status | `NotificationRequest.Status.DeliveryAttempts` has entries for each channel |

### 6.4 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.3_part2` HEAD | Branch |
| `Route.Continue` field | Already present | YAML/JSON tags correct |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-NOT-068 | Single notification delivered to multiple channels via `continue: true` | P0 | Unit | UT-NOT-597-001 | Pending |
| BR-NOT-068 | Single notification delivered to multiple channels via `continue: true` | P0 | Integration | IT-NOT-597-001 | Pending |
| BR-NOT-068 | Single notification delivered to multiple channels via `continue: true` | P0 | E2E | E2E-NOT-597-001 | Pending |
| BR-NOT-068 | Mixed continue true/false stops at first false | P0 | Unit | UT-NOT-597-003 | Pending |
| BR-NOT-068 | Per-channel delivery status tracked (partial success) | P1 | Integration | IT-NOT-597-001 | Pending |
| BR-NOT-068 | Per-channel delivery status tracked (partial success) | P1 | E2E | E2E-NOT-597-001 | Pending |
| BR-NOT-068 | All channels attempted even if some fail | P1 | Unit | (existing orchestrator tests) | Pass |
| BR-NOT-065 | First matching route wins when `continue` is false | P0 | Unit | UT-NOT-597-002 | Pending |
| BR-NOT-065 | Fallback to parent receiver when no children match | P0 | Unit | UT-NOT-597-006 | Pending |
| BR-NOT-068 | Duplicate receiver deduplication | P1 | Unit | UT-NOT-597-005 | Pending |
| BR-NOT-068 | Nested routes with `continue` at different levels | P1 | Unit | UT-NOT-597-004 | Pending |
| BR-NOT-065 | Backward-compat: `FindReceiver` (singular) returns first match | P0 | Unit | UT-NOT-597-007 | Pending |
| BR-NOT-068 | Router nil/empty config returns console fallback | P0 | Unit | UT-NOT-597-008 | Pending |
| BR-NOT-069 | Multi-receiver condition message formatted correctly | P1 | Unit | UT-NOT-597-009 | Pending |
| BR-NOT-069 | Multi-receiver condition message visible on CRD | P1 | Integration | IT-NOT-597-001 | Pending |
| BR-NOT-068 | Router-level multi-receiver channel aggregation | P0 | Unit | UT-NOT-597-010 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-NOT-597-{SEQUENCE}`
- `UT`: Unit, `IT`: Integration, `E2E`: End-to-End

### Tier 1: Unit Tests (10 tests)

**Testable code scope**: `pkg/notification/routing/config.go`, `pkg/notification/routing/router.go` — target >=80% coverage

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-NOT-597-001` | `continue: true` on matching route collects multiple receivers | Pending |
| `UT-NOT-597-002` | `continue: false` (default) stops at first match — regression guard | Pending |
| `UT-NOT-597-003` | Mixed `continue: true` and `false` in sibling list — stops at first `false` | Pending |
| `UT-NOT-597-004` | Nested routes with `continue` at parent and child levels | Pending |
| `UT-NOT-597-005` | Same receiver matched by two routes — deduplicated in result | Pending |
| `UT-NOT-597-006` | No children match — falls back to parent receiver | Pending |
| `UT-NOT-597-007` | `FindReceiver` (singular) returns first receiver from `FindReceivers` | Pending |
| `UT-NOT-597-008` | `Router.FindReceivers` with nil config returns console fallback | Pending |
| `UT-NOT-597-009` | Multi-receiver condition message lists all receiver names | Pending |
| `UT-NOT-597-010` | `Router.FindReceivers` resolves names to `[]*Receiver` and aggregates channels | Pending |

### Tier 2: Integration Test (1 test)

**Testable code scope**: `internal/controller/notification/routing_handler.go` — controller wiring with real envtest

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-NOT-597-001` | Real controller delivers notification to channels from ALL matched receivers when `continue: true` is configured, with correct RoutingResolved condition | Pending |

### Tier 3: E2E Test (1 test)

**Testable code scope**: Full stack in Kind cluster

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-NOT-597-001` | ConfigMap with `continue: true` deployed to Kind; notification delivered to all matched channels with per-channel delivery status | Pending |

---

## 9. Test Cases

### Unit Tests (UT-NOT-597-001 through 010)

*(Unchanged from v2 — see previous version for full specifications)*

### UT-NOT-597-001 through UT-NOT-597-010

Same as v2 test plan. These test `Route.FindReceivers`, `Route.FindReceiver`, `Router.FindReceivers`, condition message formatting, and channel aggregation at the unit level.

---

### IT-NOT-597-001: Controller multi-receiver fanout delivery

**BR**: BR-NOT-068, BR-NOT-069
**Priority**: P0
**Type**: Integration
**File**: `test/integration/notification/fanout_routing_test.go` (new)

**Preconditions**:
- envtest controller running (from `suite_test.go`)
- Delivery orchestrator with console, file, and log channels registered
- `testRouter` pre-loaded with fanout routes at suite startup (no runtime mutation)
  - Route: match `test-channel-set: fanout-test` → `fanout-console` (console), `continue: true`
  - Route: match `test-channel-set: fanout-test` → `fanout-file` (file)

**Parallel Safety**: No `testRouter.LoadConfig()` calls — fanout routes are added to the
initial routing config in `suite_test.go`. This ensures parallel test execution (`-procs=4`)
cannot be impacted by config mutations.

**Test Steps**:
1. **Given**: Routing config already contains `continue: true` fanout routes from suite setup
2. **When**: Create a NotificationRequest with `spec.extensions["test-channel-set"] = "fanout-test"`
3. **Then**:
   - Notification reaches `Sent` phase
   - `status.deliveryAttempts` has entries for BOTH console AND file channels
   - `status.successfulDeliveries >= 2`
   - RoutingResolved condition message contains both receiver names ("fanout-console", "fanout-file")

**Acceptance Criteria**:
- **Behavior**: Real controller resolves multi-receiver channels and delivers to all
- **Correctness**: Delivery attempts recorded for each channel from each matched receiver
- **Observability**: RoutingResolved condition lists all matched receivers (BR-NOT-069)

**Dependencies**: `suite_test.go` must include fanout routes in initial config

---

### E2E-NOT-597-001: End-to-end multi-receiver fanout in Kind

**BR**: BR-NOT-068
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/notification/10_fanout_routing_test.go` (new)

**Preconditions**:
- Kind cluster with Notification Controller deployed
- Routing ConfigMap (`notification-routing-config`) pre-deployed at cluster bootstrap with fanout routes:
  - Route: match `test-channel-set: e2e-fanout` → `fanout-console` (console), `continue: true`
  - Route: match `test-channel-set: e2e-fanout` → `fanout-file-log` (file + log)

**Parallel Safety**: No `kubectl apply` of updated ConfigMap — fanout routes are added to the
initial ConfigMap in `test/infrastructure/notification_e2e.go`. This ensures parallel E2E test
execution cannot be impacted by ConfigMap mutations.

**Test Steps**:
1. **Given**: Routing ConfigMap already contains `continue: true` fanout routes from cluster bootstrap
2. **When**: Create a NotificationRequest with `spec.extensions["test-channel-set"] = "e2e-fanout"`
3. **Then**:
   - Notification reaches `Sent` phase within 30s
   - `status.deliveryAttempts` has entries for console, file, AND log channels (3 total)
   - `status.successfulDeliveries == 3`
   - File delivery creates audit file (verifiable via kubectl cp from pod)

**Acceptance Criteria**:
- **Behavior**: Pre-deployed `continue: true` routing delivers to all matched receivers
- **Correctness**: Per-channel delivery status tracked (BR-NOT-068 acceptance criterion)
- **Accuracy**: All 3 channels from 2 receivers delivered successfully

**Dependencies**: `test/infrastructure/notification_e2e.go` must include fanout routes in initial ConfigMap

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None — pure in-memory routing logic
- **Location**: `test/unit/notification/routing_config_test.go`, `test/unit/notification/routing_hotreload_test.go`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Mock Slack webhook server (from suite)
- **Infrastructure**: envtest (real K8s API), PostgreSQL, Redis, DataStorage
- **Location**: `test/integration/notification/fanout_routing_test.go`

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind cluster (2 nodes), deployed Notification Controller
- **Location**: `test/e2e/notification/10_fanout_routing_test.go`

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| Kind | v0.20+ | E2E cluster |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None. All code to be modified is in-tree with no external dependencies.

### 11.2 Execution Order — TDD Phases

#### Phase 1: TDD RED — Write Failing Tests (all tiers)

Write all 12 test cases:
- 10 unit tests (UT-NOT-597-001..010)
- 1 integration test (IT-NOT-597-001) in new file `fanout_routing_test.go`
- 1 E2E test (E2E-NOT-597-001) in new file `10_fanout_routing_test.go`

Stub `FindReceivers` returning `nil` so tests compile but fail.

**Exit criteria**: All 12 tests compile and FAIL (RED).

#### CHECKPOINT 1: RED Phase Due Diligence

- All 12 tests present and failing for the right reason
- Zero modifications to existing test code
- Integration test uses pre-loaded fanout routes in `suite_test.go` initial config (parallel-safe)
- E2E test uses pre-deployed fanout routes in `notification_e2e.go` initial ConfigMap (parallel-safe)
- `go build ./...` succeeds

#### Phase 2: TDD GREEN — Minimal Implementation

Implement code to make all 12 tests pass:
1. `Route.FindReceivers` with dedup (config.go)
2. `Route.FindReceiver` backward-compat wrapper (config.go)
3. `Router.FindReceivers` with nil-handling (router.go)
4. `ResolveChannelsForNotification` update (resolver.go — test utility)
5. `resolveChannelsFromRoutingWithDetails` update (routing_handler.go — production path)

**Exit criteria**: All 12 new tests pass. All existing tests pass. `go build ./...` succeeds.

#### CHECKPOINT 2: GREEN Phase Due Diligence

- `go test ./test/unit/notification/...` — 0 failures
- `go test ./test/integration/notification/...` — 0 failures
- `go build ./...` — clean
- Backward compat: existing routing tests pass unchanged
- Coverage >=80% on `pkg/notification/routing/config.go`
- Dedup confirmed inside `Route.FindReceivers`
- No anti-patterns (time.Sleep, Skip, interface{})

#### Phase 3: TDD REFACTOR — Code Quality

- Extract dedup helper if needed
- Doc comments referencing BR-NOT-068, Alertmanager semantics
- Clean dead code
- Consistent naming

**Exit criteria**: All tests pass. Code clean and documented.

#### CHECKPOINT 3: REFACTOR Phase Final Validation

- All unit + integration tests green
- `go build ./...` clean
- `golangci-lint` clean
- Coverage >=80%
- Scope limited to routing package + controller handler + tests

**Note**: E2E tests are validated separately via CI (Kind cluster required).

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/597/TEST_PLAN.md` | Strategy and test design |
| Unit test suite (routing) | `test/unit/notification/routing_config_test.go` | 8 fanout tests |
| Unit test suite (router) | `test/unit/notification/routing_hotreload_test.go` | 2 Router fanout tests |
| Integration test | `test/integration/notification/fanout_routing_test.go` | 1 controller fanout test |
| E2E test | `test/e2e/notification/10_fanout_routing_test.go` | 1 Kind cluster fanout test |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/notification/... -ginkgo.focus="597" -ginkgo.v

# Integration tests (requires infrastructure)
go test ./test/integration/notification/... -ginkgo.focus="597" -ginkgo.v

# E2E tests (requires Kind cluster)
go test ./test/e2e/notification/... -ginkgo.focus="597" -ginkgo.v

# Coverage
go test ./test/unit/notification/... -coverprofile=cov.out -coverpkg=./pkg/notification/routing/...
go tool cover -func=cov.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | — | — | `FindReceiver` backward compat wrapper ensures all existing callers produce identical results |

---

## 15. Audit Trail

### Adversarial Audit (v2.0)

10 findings identified and addressed. See v2.0 changelog for details.

### Tier Coverage Expansion (v3.0)

Added integration and E2E tests per user request:
- **IT-NOT-597-001**: Real controller (envtest) with pre-loaded `continue: true` routing config (parallel-safe)
- **E2E-NOT-597-001**: Kind cluster with pre-deployed fanout ConfigMap and full delivery validation (parallel-safe)

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-09 | Initial test plan (7 unit tests) |
| 2.0 | 2026-04-09 | Adversarial audit: +3 unit tests, corrected BR matrix, honest tier assessment |
| 3.0 | 2026-04-09 | Added IT-NOT-597-001 (integration) and E2E-NOT-597-001 (E2E) for full three-tier coverage |
| 4.0 | 2026-04-09 | Parallel safety: IT/E2E no longer hot-reload shared router/ConfigMap; fanout routes added to initial suite configs instead |
