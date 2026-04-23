# Test Plan: RO NotificationRequest for Blocked/ManualReviewRequired

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-803-v1
**Feature**: Create NotificationRequest when RO blocks an RR due to IneffectiveChain
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/803-blocked-notification`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that the Remediation Orchestrator creates a
NotificationRequest when an RR is blocked due to `IneffectiveChain`, closing the
gap where `ManualReviewRequired` outcomes go unnotified. The `handleBlocked`
function currently sets `Outcome=ManualReviewRequired` but never creates the NR,
violating BR-ORCH-036.

### 1.2 Objectives

1. **Notification creation**: `handleBlocked` with `IneffectiveChain` creates a ManualReview NotificationRequest with `ReviewSource=RoutingEngine`
2. **Idempotency**: Re-reconcile does not duplicate the NR or emit duplicate events
3. **Scoping**: Non-IneffectiveChain block reasons (ConsecutiveFailures, RecentlyRemediated, etc.) do NOT create NRs
4. **Event emission**: K8s `NotificationCreated` event is emitted on NR creation

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `make test-unit-remediationorchestrator` |
| Integration test pass rate | 100% | `make test-integration-remediationorchestrator` |
| Backward compatibility | 0 regressions | All existing RO tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-ORCH-036**: Manual Review & Escalation Notifications — "Any failure without automatic recovery MUST be notified"
- **BR-ORCH-042.5**: Notification on Block — "NotificationRequest created when RR enters Blocked"
- **Issue #803**: RO does not create NotificationRequest for Blocked/ManualReviewRequired outcomes

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Duplicate NR on re-reconcile | Data pollution, alert fatigue | Medium | UT-RO-803-006, IT-RO-803-003 | `hasNotificationRef` guard + deterministic NR name (`nr-manual-review-{rr.Name}`) |
| R2 | Non-IneffectiveChain blocks create unwanted NRs | Alert fatigue | High | UT-RO-803-005, IT-RO-803-002 | Scope notification to `IneffectiveChain` ONLY via explicit block reason check |
| R3 | ReviewSourceType enum validation rejects RoutingEngine | CRD admission failure | Medium | UT-RO-803-001 | Update kubebuilder enum marker and regenerate CRDs |
| R4 | Reconcile loop from owned NR | Performance degradation | Low | UT-RO-803-006 | Same ownership pattern as all other NR paths; keep work O(1) |

### 3.1 Risk-to-Test Traceability

- **R1** (Duplicate NR): Directly mitigated by UT-RO-803-006 and IT-RO-803-003
- **R2** (Unwanted NRs): Directly mitigated by UT-RO-803-005 and IT-RO-803-002
- **R3** (Enum validation): Mitigated by UT-RO-803-001 (compile-time constant check)
- **R4** (Reconcile loop): Mitigated by UT-RO-803-006 (idempotency under re-reconcile)

---

## 4. Scope

### 4.1 Features to be Tested

- **handleBlocked NR creation** (`internal/controller/remediationorchestrator/reconciler.go`): NotificationRequest creation for IneffectiveChain blocks
- **ReviewSourceRoutingEngine constant** (`api/notification/v1alpha1/notificationrequest_types.go`): New enum value for routing-engine-sourced notifications
- **CreateManualReviewNotification with RoutingEngine source** (`pkg/remediationorchestrator/creator/notification.go`): Existing creator accepts new source type

### 4.2 Features Not to be Tested

- **Routing engine IneffectiveChain detection**: Covered by existing tests in `test/unit/remediationorchestrator/routing/ineffective_chain_test.go`
- **Notification delivery**: Out of scope; covered by notification controller tests
- **E2E with full Kind cluster**: Deferred; requires DS + routing engine + real IneffectiveChain triggering

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Scope NR to IneffectiveChain only | Only block reason that sets ManualReviewRequired; others are transitory |
| Use MockBlockingRoutingEngine for IT | Tests NR creation behavior, not routing logic (already covered) |
| Add ReviewSourceRoutingEngine constant | Distinguishes blocked-path notifications from AI/WE-sourced ones |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code in `handleBlocked` NR creation path
- **Integration**: >=80% of integration-testable code (full reconcile through Blocked with NR)

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- **Unit tests**: Validate NR creation logic, idempotency, scoping, and event emission
- **Integration tests**: Validate end-to-end reconcile with real fake client and CRD creation

### 5.3 Business Outcome Quality Bar

Tests validate business outcomes:
- "Operator receives notification when remediation chain is ineffective" (not "CreateManualReviewNotification is called")
- "Operator does NOT receive spurious notifications for transient blocks" (not "code path skipped")

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:
1. All 9 tests pass (6 UT + 3 IT)
2. No regressions in existing RO test suites
3. `go build ./...` clean

**FAIL** — any of the following:
1. Any test fails
2. Existing tests regress
3. Build broken

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Build broken; unit tests cannot execute

**Resume testing when**:
- Build fixed and green

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | `handleBlocked` (IneffectiveChain NR creation block) | ~20 |
| `api/notification/v1alpha1/notificationrequest_types.go` | `ReviewSourceRoutingEngine` constant | ~3 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | Full `Reconcile` path through `handleBlocked` | ~100 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-036 | Manual review notification for IneffectiveChain blocks | P0 | Unit | UT-RO-803-001 | Pending |
| BR-ORCH-036 | Manual review notification for IneffectiveChain blocks | P0 | Unit | UT-RO-803-003 | Pending |
| BR-ORCH-036 | Manual review notification for IneffectiveChain blocks | P0 | Integration | IT-RO-803-001 | Pending |
| BR-ORCH-042.5 | Notification on Block (idempotency) | P0 | Unit | UT-RO-803-002 | Pending |
| BR-ORCH-042.5 | Notification on Block (idempotency) | P0 | Unit | UT-RO-803-006 | Pending |
| BR-ORCH-042.5 | Notification on Block (idempotency) | P0 | Integration | IT-RO-803-003 | Pending |
| BR-ORCH-036 | No spurious notifications for non-IneffectiveChain blocks | P0 | Unit | UT-RO-803-005 | Pending |
| BR-ORCH-036 | No spurious notifications for non-IneffectiveChain blocks | P0 | Integration | IT-RO-803-002 | Pending |
| BR-ORCH-095 | K8s event emission on NR creation | P1 | Unit | UT-RO-803-004 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-803-001` | CreateManualReviewNotification accepts ReviewSourceRoutingEngine and produces NR with correct source | Pending |
| `UT-RO-803-002` | CreateManualReviewNotification is idempotent (second call returns existing NR name) | Pending |
| `UT-RO-803-003` | handleBlocked with IneffectiveChain creates ManualReview NR and appends to NotificationRequestRefs | Pending |
| `UT-RO-803-004` | handleBlocked with IneffectiveChain emits NotificationCreated K8s event | Pending |
| `UT-RO-803-005` | handleBlocked with non-IneffectiveChain reasons does NOT create NR | Pending |
| `UT-RO-803-006` | handleBlocked with IneffectiveChain re-reconcile does NOT duplicate NR or events | Pending |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-RO-803-001` | RR blocked due to IneffectiveChain -> NotificationRequest CRD exists with Type=ManualReview, ReviewSource=RoutingEngine | Pending |
| `IT-RO-803-002` | RR blocked due to ConsecutiveFailures -> NO NotificationRequest created | Pending |
| `IT-RO-803-003` | RR blocked due to IneffectiveChain re-reconcile -> still exactly 1 NR (idempotent) | Pending |

### Tier Skip Rationale

- **E2E**: Deferred; requires full Kind cluster + DataStorage + routing engine with IneffectiveChain threshold configuration. The unit + integration tiers provide sufficient behavioral coverage.

---

## 9. Test Cases

### UT-RO-803-001: RoutingEngine source accepted by CreateManualReviewNotification

**BR**: BR-ORCH-036
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Preconditions**:
- `ReviewSourceRoutingEngine` constant defined
- NotificationCreator with fake client

**Test Steps**:
1. **Given**: A ManualReviewContext with `Source: ReviewSourceRoutingEngine`, `Reason: "IneffectiveChain"`, `Message: "3 consecutive ineffective remediations detected"`
2. **When**: `CreateManualReviewNotification` is called
3. **Then**: NR is created with `Spec.ReviewSource == "RoutingEngine"` and `Spec.Type == "ManualReview"`

### UT-RO-803-002: Idempotent creation with RoutingEngine source

**BR**: BR-ORCH-042.5
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Preconditions**:
- Same as UT-RO-803-001

**Test Steps**:
1. **Given**: First call to `CreateManualReviewNotification` succeeds
2. **When**: Second call with same RR
3. **Then**: Returns same NR name, no error, no duplicate created

### UT-RO-803-003: handleBlocked creates NR for IneffectiveChain

**BR**: BR-ORCH-036
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/blocked_notification_test.go`

**Preconditions**:
- RR in Analyzing phase with completed AI and SP
- MockBlockingRoutingEngine returning IneffectiveChain block

**Test Steps**:
1. **Given**: RR in PhaseAnalyzing with AI completed (high confidence, auto-approve)
2. **When**: `Reconcile` is called and routing returns IneffectiveChain block
3. **Then**: NR `nr-manual-review-{rr.Name}` exists, `rr.Status.NotificationRequestRefs` contains the NR ref

### UT-RO-803-004: handleBlocked emits NotificationCreated event

**BR**: BR-ORCH-095
**Priority**: P1
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/blocked_notification_test.go`

**Preconditions**:
- Same as UT-RO-803-003, with FakeRecorder

**Test Steps**:
1. **Given**: Same as UT-RO-803-003
2. **When**: `Reconcile` is called
3. **Then**: FakeRecorder contains `NotificationCreated` event

### UT-RO-803-005: Non-IneffectiveChain blocks do NOT create NR

**BR**: BR-ORCH-036
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/blocked_notification_test.go`

**Preconditions**:
- Same RR setup, but MockBlockingRoutingEngine returning ConsecutiveFailures

**Test Steps**:
1. **Given**: RR in PhaseAnalyzing with AI completed
2. **When**: `Reconcile` is called and routing returns ConsecutiveFailures block
3. **Then**: No NR with prefix `nr-manual-review-` exists

### UT-RO-803-006: Re-reconcile idempotency for IneffectiveChain NR

**BR**: BR-ORCH-042.5
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/blocked_notification_test.go`

**Preconditions**:
- RR already in PhaseBlocked with existing NR ref
- MockBlockingRoutingEngine returning IneffectiveChain

**Test Steps**:
1. **Given**: RR already blocked with NR ref in NotificationRequestRefs
2. **When**: Re-reconcile triggers
3. **Then**: Still exactly 1 NR, no duplicate events

### IT-RO-803-001: End-to-end IneffectiveChain NR creation

**BR**: BR-ORCH-036
**Priority**: P0
**Type**: Integration
**File**: `test/integration/remediationorchestrator/blocked_notification_integration_test.go`

**Preconditions**:
- envtest cluster with CRDs
- Reconciler with MockBlockingRoutingEngine returning IneffectiveChain

**Test Steps**:
1. **Given**: RR created in cluster in PhaseAnalyzing with completed AI and SP
2. **When**: Reconciler processes the RR
3. **Then**: NotificationRequest CRD exists with `ReviewSource=RoutingEngine`, `Type=ManualReview`

### IT-RO-803-002: ConsecutiveFailures block does NOT create NR

**BR**: BR-ORCH-036
**Priority**: P0
**Type**: Integration
**File**: `test/integration/remediationorchestrator/blocked_notification_integration_test.go`

**Preconditions**:
- envtest cluster, MockBlockingRoutingEngine returning ConsecutiveFailures

**Test Steps**:
1. **Given**: RR in PhaseAnalyzing
2. **When**: Reconciler processes the RR
3. **Then**: No NotificationRequest exists with prefix `nr-manual-review-`

### IT-RO-803-003: Idempotent NR creation under re-reconcile

**BR**: BR-ORCH-042.5
**Priority**: P0
**Type**: Integration
**File**: `test/integration/remediationorchestrator/blocked_notification_integration_test.go`

**Preconditions**:
- envtest cluster, IneffectiveChain block

**Test Steps**:
1. **Given**: First reconcile creates NR
2. **When**: Second reconcile runs
3. **Then**: Still exactly 1 NR in cluster

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `MockBlockingRoutingEngine` (returns configured BlockingCondition), `fake.NewClientBuilder` (K8s fake client)
- **Location**: `test/unit/remediationorchestrator/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: envtest (Kubernetes API server)
- **Location**: `test/integration/remediationorchestrator/`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None. All required code and infrastructure exists.

### 11.2 Execution Order

1. **Phase 1**: Unit tests for creator and handleBlocked (TDD RED)
2. **Phase 2**: Minimal implementation (TDD GREEN)
3. **Phase 3**: Integration tests (TDD RED)
4. **Phase 4**: Integration tests pass (TDD GREEN)
5. **Phase 5**: Refactor

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/803/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/remediationorchestrator/controller/blocked_notification_test.go` | 4 Ginkgo BDD tests |
| Unit test suite | `test/unit/remediationorchestrator/notification_creator_test.go` | 2 new Ginkgo BDD tests |
| Integration test suite | `test/integration/remediationorchestrator/blocked_notification_integration_test.go` | 3 Ginkgo BDD tests |

---

## 13. Execution

```bash
# Unit tests
make test-unit-remediationorchestrator

# Integration tests
make test-integration-remediationorchestrator

# All RO tests
make test-all-remediationorchestrator

# Specific test by ID
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-803"
```

---

## 14. Existing Tests Requiring Updates

None. The new code adds behavior to `handleBlocked` for `IneffectiveChain` only.
Existing blocking tests cover other block reasons and are unaffected.

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan |
