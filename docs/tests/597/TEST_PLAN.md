# Test Plan: Continue Route Fanout (#597)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-597-v3
**Feature**: Implement `continue` route fanout in the notification routing engine (BR-NOT-068)
**Version**: 3.0
**Created**: 2026-04-10
**Author**: AI Assistant
**Status**: Active
**Branch**: `development/v1.3_part4`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that the notification routing engine correctly implements
Alertmanager-style `continue` fanout (BR-NOT-068), enabling a single notification
to match multiple sibling routes and be delivered to multiple receivers.

### 1.2 Objectives

1. **Fanout correctness**: When `continue: true`, all matching sibling routes after the first match are also evaluated and their receivers collected
2. **Backward compatibility**: When `continue` is false or omitted, routing behavior is identical to the pre-change first-match-wins logic
3. **Channel deduplication**: When multiple receivers resolve to overlapping channels, duplicates are removed by qualified channel name
4. **Thread safety**: Multi-receiver resolution under `Router.mu.RLock()` is safe for concurrent reads (design property validated by code inspection of `Router.FindReceivers`; no concurrent stress test — concurrency testing deferred to load/soak testing)

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `make test-unit-notification` |
| Integration test pass rate | 100% | `make test-integration-notification` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `pkg/notification/routing/` |
| Backward compatibility | 0 regressions | All existing routing tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-NOT-068: Multi-Channel Fanout support
- BR-NOT-065: Channel Routing Based on Spec Fields
- Issue #597: Continue route fanout on notification routing

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- DD-NOTIFICATION-001: Alertmanager routing reuse

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `FindReceivers` breaks existing callers | Compilation failures across codebase | Medium | All | Keep `FindReceiver` as convenience wrapper returning first result |
| R2 | Channel deduplication misses qualified names | Duplicate Slack messages sent | Low | UT-NOT-597-008 | Dedup by qualified channel name (e.g., `slack:sre-critical`) |
| R3 | `continue=true` with empty Receiver field | Possible nil receiver in results | Medium | (code-level mitigation) | `collectReceivers` skips routes with `child.Receiver == ""`; verified by code inspection. No dedicated UT — empty receivers are a configuration error caught by `Validate()` |
| R4 | Deeply nested routes with mixed continue | Incorrect receiver ordering | Low | UT-NOT-597-004 | Depth-first traversal with continue only affecting siblings |

---

## 4. Scope

### 4.1 Features to be Tested

- **Route.FindReceivers** (`pkg/notification/routing/config.go`): New method returning `[]string` with continue-aware sibling fanout
- **Router.FindReceivers** (`pkg/notification/routing/router.go`): Thread-safe wrapper returning `[]*Receiver`

### 4.2 Features Not to be Tested

- **Controller routing handler** (`internal/controller/notification/routing_handler.go`): `receiversToChannels` merge/dedup is an existing integration-testable path exercised by the broader notification integration suite, not isolated by #597 tests. Coverage of this file is measured by `make test-integration-notification` across all notification ITs, not by the #597 subset alone.
- **Delivery orchestration**: How channels are actually delivered (covered by existing delivery tests)
- **ConfigMap hot-reload**: Already covered by BR-NOT-067 tests
- **matchRe regex matching**: Deferred to #416

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Add `FindReceivers` alongside `FindReceiver` | Non-breaking API evolution; 20+ existing call sites preserved |
| `continue` only affects siblings, not parent traversal | Matches Alertmanager documented semantics |
| Channel dedup by qualified name | Prevents duplicate delivery while preserving per-receiver credential binding |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of routing engine code (`pkg/notification/routing/config.go`, `router.go`)
- **Integration**: Router-level multi-receiver resolution validated through `Router.FindReceivers` with realistic configs and `RoutingAttributesFromSpec`. Controller handler coverage is provided by the broader notification integration suite, not isolated to #597 tests.

### 5.2 Two-Tier Minimum

Every business requirement covered by UT + IT.

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes**: "a notification matching multiple continue routes is resolved to all corresponding receivers" and "existing single-receiver routing is unchanged." Channel-level dedup is validated at the routing engine level via `QualifiedChannels()`.

### 5.4 Pass/Fail Criteria

**PASS**: All 10 tests pass, all pre-existing routing tests pass, `go build ./...` succeeds.

**FAIL**: Any test fails, any existing test regresses, build breaks.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/notification/routing/config.go` | `FindReceivers`, `collectReceivers`, `hasNestedMatch`, `matchesAttributes` | ~60 new |
| `pkg/notification/routing/router.go` | `Router.FindReceivers` | ~40 new |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/notification/routing/router.go` | `Router.FindReceivers` with loaded config | ~40 (same code, exercised through `NewRouter` + `LoadConfig` wiring) |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-NOT-068 | Multi-Channel Fanout | P0 | Unit | UT-NOT-597-002 | Pass |
| BR-NOT-068 | Continue with siblings | P0 | Unit | UT-NOT-597-003 | Pass |
| BR-NOT-068 | Nested continue | P1 | Unit | UT-NOT-597-004 | Pass |
| BR-NOT-068 | Receiver dedup | P1 | Unit | UT-NOT-597-005 | Pass |
| BR-NOT-065 | First match backward compat | P0 | Unit | UT-NOT-597-001 | Pass |
| BR-NOT-065 | Fallback to root | P0 | Unit | UT-NOT-597-006 | Pass |
| BR-NOT-068 | Router multi-receiver | P0 | Unit | UT-NOT-597-007 | Pass |
| BR-NOT-068 | Channel dedup (qualified names) | P1 | Unit | UT-NOT-597-008 | Pass |
| BR-NOT-068 | Router resolves multiple receivers with merged channel sets | P0 | Integration | IT-NOT-597-001 | Pass |
| BR-NOT-065 | Single-receiver routing preserved when continue absent | P0 | Integration | IT-NOT-597-002 | Pass |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `pkg/notification/routing/` — >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-NOT-597-001` | continue=false returns single receiver (backward compat) | Pass |
| `UT-NOT-597-002` | continue=true fans out to multiple sibling receivers | Pass |
| `UT-NOT-597-003` | continue=true on first child, false on second — stops at second | Pass |
| `UT-NOT-597-004` | Nested routes with continue propagation | Pass |
| `UT-NOT-597-005` | Same receiver matched twice — deduplicated in results | Pass |
| `UT-NOT-597-006` | No matching routes — falls back to root receiver (unchanged) | Pass |
| `UT-NOT-597-007` | Router.FindReceivers returns multiple *Receiver objects | Pass |
| `UT-NOT-597-008` | Channel dedup when multiple receivers have overlapping channels | Pass |

### Tier 2: Integration Tests

**Testable code scope**: `pkg/notification/routing/` Router wiring with loaded config — validates multi-receiver resolution through `NewRouter` + `LoadConfig` + `RoutingAttributesFromSpec` path

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-NOT-597-001` | Router resolves NR to multiple receivers via continue, returning merged channel sets | Pass |
| `IT-NOT-597-002` | Single-receiver routing preserved when continue is absent (no regression) | Pass |

---

## 9. Test Cases

### UT-NOT-597-001: continue=false returns single receiver (backward compat)

**BR**: BR-NOT-065
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/routing_fanout_test.go`

**Test Steps**:
1. **Given**: Config with 2 child routes, both matching, neither has continue=true
2. **When**: `FindReceivers(attrs)` is called
3. **Then**: Returns exactly 1 receiver name (first match only)

### UT-NOT-597-002: continue=true fans out to multiple sibling receivers

**BR**: BR-NOT-068
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/routing_fanout_test.go`

**Test Steps**:
1. **Given**: Config with 3 child routes, first has continue=true, second has continue=true, third has continue=false. All match.
2. **When**: `FindReceivers(attrs)` is called
3. **Then**: Returns 3 receiver names (all three matched)

### UT-NOT-597-007: Router.FindReceivers returns multiple *Receiver objects

**BR**: BR-NOT-068
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/routing_fanout_test.go`

**Test Steps**:
1. **Given**: Router loaded with config having continue routes
2. **When**: `Router.FindReceivers(attrs)` is called
3. **Then**: Returns slice of `*Receiver` objects with correct Names and channel configs

### IT-NOT-597-001: Router resolves multiple receivers via continue

**BR**: BR-NOT-068
**Priority**: P0
**Type**: Integration
**File**: `test/integration/notification/routing_fanout_integration_test.go`

**Test Steps**:
1. **Given**: Router loaded via `NewRouter` + `LoadConfig` with a 3-route config (continue=true on first two), and a `NotificationRequest` with matching attributes built via `RoutingAttributesFromSpec`
2. **When**: `Router.FindReceivers(attrs)` is called
3. **Then**: Returns 3 receivers; merged qualified channel set contains all channels from all receivers with duplicates removed

**Note**: This test validates the Router API wiring with realistic config loading and NR attribute extraction. Controller-level handler (`receiversToChannels`) coverage is provided by the broader notification integration suite.

### IT-NOT-597-002: Single-receiver routing preserved when continue absent

**BR**: BR-NOT-065
**Priority**: P0
**Type**: Integration
**File**: `test/integration/notification/routing_fanout_integration_test.go`

**Test Steps**:
1. **Given**: Router loaded with config where no routes have continue=true
2. **When**: `Router.FindReceivers(attrs)` and `Router.FindReceiver(attrs)` are both called
3. **Then**: `FindReceivers` returns exactly 1 receiver; `FindReceiver` returns the same receiver name — confirming backward-compatible behavior

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None (pure routing logic)
- **Location**: `test/unit/notification/routing_fanout_test.go`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Standalone Router (no envtest needed — routing engine is pure Go)
- **Location**: `test/integration/notification/routing_fanout_integration_test.go`

---

## 11. Dependencies & Schedule

### 11.1 Execution Order

1. **Phase 1**: Unit tests for routing engine (FindReceivers, dedup)
2. **Phase 2**: Integration tests for Router wiring

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/597/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/notification/routing_fanout_test.go` | Ginkgo BDD test file |
| Integration test suite | `test/integration/notification/routing_fanout_integration_test.go` | Ginkgo BDD test file |

---

## 13. Execution

```bash
# Unit tests (all notification unit tests including 597)
make test-unit-notification

# Integration tests (all notification integration tests including 597)
make test-integration-notification

# Focused run by test ID
go test ./test/unit/notification/... -ginkgo.focus="597"
go test ./test/integration/notification/... -ginkgo.focus="597"

# Full regression (both tiers)
make test-unit-notification test-integration-notification

# Lint compliance
make lint-test-patterns
```

---

## 14. Existing Tests Requiring Updates (if applicable)

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None expected | N/A | N/A | `FindReceiver` preserved as wrapper; all existing callers unchanged |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-10 | Initial test plan |
| 2.0 | 2026-04-10 | Marked all 10 tests as Pass (implemented). Updated execution commands to use make targets. |
| 3.0 | 2026-04-10 | Adversarial audit remediation: (C3) Corrected IT-001 description from "delivery" to "Router resolves multiple receivers with merged channel sets." (C4) Corrected IT-002 from "existing IT tests pass unchanged" to "Single-receiver routing preserved when continue absent." (H1) Removed routing_handler.go from section 6.2; documented controller handler coverage as provided by broader notification IT suite, not #597 subset. (H3) Fixed Risk R3: removed wrong test ID mapping; documented code-level mitigation (collectReceivers skips empty Receiver). (M1) Fixed section 1.3 to use make targets instead of incorrect `-run` flag. (M7) Added missing section 5.3 (Business Outcome Quality Bar). (M9) Clarified thread safety as design property validated by code inspection, not concurrent stress test. Added detailed IT-001/IT-002 test cases to section 9. |
