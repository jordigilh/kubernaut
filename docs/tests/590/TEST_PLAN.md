# Test Plan: Self-Resolved Notification (Issue #590)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-590-v1
**Feature**: `handleWorkflowNotNeeded` emits optional `status-update` notification when configured
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/v1.2.0-rc0`

---

## 1. Introduction

### 1.1 Purpose

This test plan verifies that the Remediation Orchestrator's `handleWorkflowNotNeeded` handler
creates an optional `status-update` NotificationRequest when a signal self-resolves and the
operator has explicitly opted in via `notifications.notifySelfResolved: true` in the RO ConfigMap.
It also verifies that the default behavior (no notification) is preserved, and that the new
configuration structure loads, validates, and defaults correctly.

### 1.2 Objectives

1. **AC-037-08**: When `notifySelfResolved` is `true`, a `NotificationRequest` with `spec.type: status-update` and `spec.priority: low` is created and its reference is appended to `rr.Status.NotificationRequestRefs`.
2. **AC-037-09**: When `notifySelfResolved` is `false` (default), no `NotificationRequest` is created and the handler's existing behavior is unchanged.
3. **Config correctness**: `NotificationsConfig` struct loads from YAML, defaults to `NotifySelfResolved: false`, and passes validation.
4. **Creator correctness**: `CreateSelfResolvedNotification` follows the established idempotency, owner reference, deterministic naming, and `RemediationRequestRef` patterns.
5. **Non-fatal error handling**: Notification creation failure does not block the handler's primary responsibility (RR status update + metric recording).

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/... -ginkgo.v` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on new/modified files |
| Backward compatibility | 0 regressions | Existing `aianalysis_handler_test.go` passes without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-ORCH-037**: WorkflowNotNeeded / Self-Resolved Scenario (`docs/requirements/BR-ORCH-037-workflow-not-needed.md`)
- **ADR-030**: Service Configuration Management
- **Issue #590**: `handleWorkflowNotNeeded` does not emit status-update notification

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Notification failure blocks handler, causing RR to hang in non-terminal state | RR stuck in Processing — operator has no visibility | Low | UT-RO-590-006 | Notification error is logged but not returned; handler continues to completion |
| R2 | Default config omission causes unexpected notification emission on existing deployments | Operators receive unwanted Slack messages on upgrade | Low | UT-RO-590-007, UT-RO-590-009 | `NotifySelfResolved` defaults to `false`; zero-value Go bool is safe |
| R3 | Idempotency collision with other `nr-*` names | Duplicate NR creation or wrong NR reused | Very Low | UT-RO-590-003 | Distinct prefix `nr-self-resolved-{rr.Name}` — no overlap with `nr-completion-*`, `nr-approval-*`, `nr-manual-review-*` |
| R4 | Setter pattern not called in production wiring | Feature silently disabled despite config being `true` | Low | UT-RO-590-010 | Config loading test + main.go wiring verified via config round-trip test |

### 3.1 Risk-to-Test Traceability

- **R1 (High impact)**: Covered by UT-RO-590-006 (notification failure non-fatal)
- **R2 (High impact)**: Covered by UT-RO-590-007 (no notification by default) and UT-RO-590-009 (default config value)
- **R3**: Covered by UT-RO-590-003 (deterministic naming + idempotency)
- **R4**: Covered by UT-RO-590-010 (config loads from YAML with correct defaults)

---

## 4. Scope

### 4.1 Features to be Tested

- **NotificationCreator.CreateSelfResolvedNotification** (`pkg/remediationorchestrator/creator/notification.go`): Creates a `status-update` NR with correct spec, context, owner ref, and idempotency
- **AIAnalysisHandler.handleWorkflowNotNeeded** (`pkg/remediationorchestrator/handler/aianalysis.go`): Conditionally calls creator when `notifySelfResolved=true`, appends NR ref, handles errors non-fatally
- **NotificationsConfig** (`internal/config/remediationorchestrator/config.go`): Config struct, default, validation, YAML loading

### 4.2 Features Not to be Tested

- **Notification routing/delivery**: Downstream `status-update` handling is already tested in notification controller integration/E2E tests
- **Helm rendering**: Template correctness verified by `helm template` in CI; not part of this unit test plan
- **Reconciler.SetNotifySelfResolved wiring**: Setter is a one-liner forwarding to handler; tested indirectly via handler tests

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Setter pattern (`SetNotifySelfResolved`) instead of constructor param | Follows `SetDSClient`, `SetRESTMapper`, `SetAsyncPropagation` precedent; avoids changing ~85 existing test call sites |
| Non-fatal notification error | BR-ORCH-037 spec says handler must complete even if notification fails; RR status update is the primary responsibility |
| `nr-self-resolved-{rr.Name}` deterministic name | Consistent with `nr-completion-*`, `nr-approval-*`, `nr-manual-review-*` naming; BR says `nr-info-*` but project convention is more descriptive |
| Unit tests only (no integration tier for this feature) | All new code is pure logic (creator builds CRD in-memory, handler is conditionally gated); K8s client is faked via `fake.NewClientBuilder()` — this is standard unit test territory per testing guidelines |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (new creator method, handler conditional path, config struct)
- **Integration**: Not applicable — see Tier Skip Rationale in Section 8
- **E2E**: Not applicable — see Tier Skip Rationale in Section 8

### 5.2 Two-Tier Minimum

The two-tier requirement is satisfied by:
- **Unit tests**: Cover all logic paths (creator, handler, config)
- **Existing integration/E2E tests**: `status-update` notification type is already exercised by notification controller integration and E2E suites; this feature adds a new emission point, not a new processing path

### 5.3 Business Outcome Quality Bar

Each test validates what the **operator** gets:
- When configured: a Slack/console notification saying the signal self-resolved
- When not configured: silence (existing behavior preserved)
- When notification fails: the RR still completes normally (no operator-facing disruption)

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All 10 unit tests pass (0 failures)
2. Unit-testable code coverage >=80% on new/modified files
3. No regressions in existing `aianalysis_handler_test.go` or `notification_creator_test.go`
4. `go build ./...` and `go vet ./...` pass

**FAIL** — any of the following:

1. Any test fails
2. Coverage below 80% on new code
3. Existing tests regress
4. Build or vet errors

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Build broken — code does not compile
- Issue #588 changes to `notification.go` cause merge conflicts (unlikely — different sections)

**Resume testing when**:
- Build fixed and green
- Conflicts resolved

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/creator/notification.go` | `CreateSelfResolvedNotification`, `buildSelfResolvedBody` | ~70 new |
| `pkg/remediationorchestrator/handler/aianalysis.go` | `handleWorkflowNotNeeded` (modified), `SetNotifySelfResolved` (new) | ~20 new |
| `internal/config/remediationorchestrator/config.go` | `NotificationsConfig` struct, `DefaultConfig` (modified), `Validate` (modified) | ~15 new |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | `SetNotifySelfResolved` | ~3 new |
| `cmd/remediationorchestrator/main.go` | Config wiring | ~1 new |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/v1.2.0-rc0` HEAD | After #588 commits |
| Dependency: #588 | Merged (same branch) | Sentinel RCA filtering in same creator file |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-037 AC-037-08 | Informational notification created when configured | P0 | Unit | UT-RO-590-001 | Pending |
| BR-ORCH-037 AC-037-08 | NR has correct type (status-update) and priority (low) | P0 | Unit | UT-RO-590-002 | Pending |
| BR-ORCH-037 AC-037-08 | NR has deterministic name and idempotency | P1 | Unit | UT-RO-590-003 | Pending |
| BR-ORCH-037 AC-037-08 | NR body contains signal name, target, AI message, RCA | P0 | Unit | UT-RO-590-004 | Pending |
| BR-ORCH-037 AC-037-08 | NR reference appended to RR.Status.NotificationRequestRefs | P0 | Unit | UT-RO-590-005 | Pending |
| BR-ORCH-037 AC-037-08 | Notification failure does not block handler completion | P0 | Unit | UT-RO-590-006 | Pending |
| BR-ORCH-037 AC-037-09 | No notification created when notifySelfResolved=false | P0 | Unit | UT-RO-590-007 | Pending |
| BR-ORCH-037 AC-037-09 | Existing handler behavior unchanged (RR status, metrics) | P1 | Unit | UT-RO-590-008 | Pending |
| BR-ORCH-037 | NotificationsConfig defaults to NotifySelfResolved=false | P0 | Unit | UT-RO-590-009 | Pending |
| BR-ORCH-037 | NotificationsConfig loads from YAML correctly | P1 | Unit | UT-RO-590-010 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-RO-590-{SEQUENCE}` (Unit Test, Remediation Orchestrator, Issue 590)

### Tier 1: Unit Tests

**Testable code scope**: `pkg/remediationorchestrator/creator/notification.go`, `pkg/remediationorchestrator/handler/aianalysis.go`, `internal/config/remediationorchestrator/config.go` — >=80% coverage target on new/modified lines.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-590-001` | Operator receives a status-update notification when self-resolved notification is enabled | Pending |
| `UT-RO-590-002` | Notification has type=status-update and priority=low per BR spec | Pending |
| `UT-RO-590-003` | Second reconciliation for same RR reuses existing NR (idempotency) | Pending |
| `UT-RO-590-004` | Notification body contains signal name, target resource, AI message, and RCA summary | Pending |
| `UT-RO-590-005` | RR.Status.NotificationRequestRefs includes the self-resolved NR reference | Pending |
| `UT-RO-590-006` | RR completes normally even when notification creation fails | Pending |
| `UT-RO-590-007` | No notification created when notifySelfResolved is false (default behavior) | Pending |
| `UT-RO-590-008` | Existing handler outcomes (RR Completed, NoActionRequired, metric) are preserved regardless of config | Pending |
| `UT-RO-590-009` | DefaultConfig().Notifications.NotifySelfResolved is false | Pending |
| `UT-RO-590-010` | NotificationsConfig round-trips through YAML marshal/unmarshal | Pending |

### Tier Skip Rationale

- **Integration**: All new code is pure logic operating on in-memory CRD objects with a faked K8s client. The `status-update` notification type is already exercised end-to-end by existing notification controller integration tests. Adding a new emission point does not require a new integration test tier.
- **E2E**: Same rationale. The downstream notification delivery for `status-update` type is already covered. A new E2E test would only verify Helm ConfigMap rendering, which is out of scope for this plan.

---

## 9. Test Cases

### UT-RO-590-001: CreateSelfResolvedNotification creates NR with status-update type

**BR**: BR-ORCH-037 AC-037-08
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Preconditions**:
- Fake K8s client with RR and AIAnalysis objects
- RR has UID set (required for owner reference)

**Test Steps**:
1. **Given**: A `RemediationRequest` in namespace "default" with a valid UID and signal name
2. **When**: `CreateSelfResolvedNotification(ctx, rr, ai)` is called
3. **Then**: A `NotificationRequest` CRD is created in the cluster

**Expected Results**:
1. Return value is `("nr-self-resolved-{rr.Name}", nil)`
2. NR exists in fake client with `spec.type = "status-update"`

**Acceptance Criteria**:
- **Behavior**: Creator produces a NR CRD in the cluster
- **Correctness**: NR name follows deterministic pattern
- **Accuracy**: Type is `status-update`, not `manual-review` or `completion`

---

### UT-RO-590-002: NR has correct spec fields (type, priority, severity, subject)

**BR**: BR-ORCH-037 AC-037-08
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Preconditions**:
- Same as UT-RO-590-001

**Test Steps**:
1. **Given**: RR with severity "warning" and signal name "HighMemoryUsage"
2. **When**: `CreateSelfResolvedNotification` is called
3. **Then**: NR spec matches BR-ORCH-037 specification

**Expected Results**:
1. `spec.type` = `status-update`
2. `spec.priority` = `low`
3. `spec.severity` = RR's severity
4. `spec.subject` contains signal name and "Auto-Resolved" indicator
5. `spec.remediationRequestRef` points to the parent RR with correct UID

**Acceptance Criteria**:
- **Behavior**: All spec fields populated per BR
- **Correctness**: Priority is always `low` (informational, not actionable)
- **Accuracy**: RemediationRequestRef UID matches the parent RR

---

### UT-RO-590-003: Idempotency — second call returns existing NR name

**BR**: BR-ORCH-037 AC-037-08
**Priority**: P1
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Preconditions**:
- NR `nr-self-resolved-{rr.Name}` already exists in fake client

**Test Steps**:
1. **Given**: NR already created from a previous call
2. **When**: `CreateSelfResolvedNotification` is called again for the same RR
3. **Then**: Returns the existing NR name without creating a duplicate

**Expected Results**:
1. Return value is `("nr-self-resolved-{rr.Name}", nil)`
2. Only 1 NR exists (not 2)

**Acceptance Criteria**:
- **Behavior**: No duplicate CRDs created
- **Correctness**: Same name returned on retry

---

### UT-RO-590-004: NR body contains signal, target, AI message, and RCA

**BR**: BR-ORCH-037 AC-037-08
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Preconditions**:
- AIAnalysis with `Status.Message` set and `Status.RootCauseAnalysis.Summary` set
- RR with `Spec.TargetResource` populated

**Test Steps**:
1. **Given**: AI message = "Problem self-resolved", RCA summary = "Node memory pressure cleared"
2. **When**: `CreateSelfResolvedNotification` is called
3. **Then**: NR body contains all expected information

**Expected Results**:
1. Body contains signal name
2. Body contains target resource Kind/Name
3. Body contains AI message
4. Body contains RCA summary
5. Body contains "audit purposes only" tagline per BR spec

**Acceptance Criteria**:
- **Behavior**: Operator sees actionable context in the notification
- **Correctness**: All BR-specified fields present
- **Accuracy**: RCA comes from `ai.Status.RootCauseAnalysis.Summary` (not deprecated `ai.Status.RootCause`)

---

### UT-RO-590-005: Handler appends NR ref to RR.Status.NotificationRequestRefs

**BR**: BR-ORCH-037 AC-037-08, BR-ORCH-035
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Preconditions**:
- Handler with `notifySelfResolved = true`
- Fake K8s client with RR (status subresource enabled)

**Test Steps**:
1. **Given**: `notifySelfResolved = true`, AIAnalysis with `Reason=WorkflowNotNeeded`
2. **When**: `HandleAIAnalysisStatus` is called
3. **Then**: RR status updated with NR ref appended

**Expected Results**:
1. `rr.Status.NotificationRequestRefs` has exactly 1 entry
2. Entry has `Kind=NotificationRequest`, `Name=nr-self-resolved-{rr.Name}`
3. RR phase is still `Completed` with `Outcome=NoActionRequired`

**Acceptance Criteria**:
- **Behavior**: NR reference tracked for audit lineage (BR-ORCH-035)
- **Correctness**: Handler completes both RR status update AND notification creation
- **Accuracy**: NotificationRequestRefs contains correct reference

---

### UT-RO-590-006: Notification failure does not block handler

**BR**: BR-ORCH-037 AC-037-08
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Preconditions**:
- Handler with `notifySelfResolved = true`
- Fake K8s client configured to return error on NR creation

**Test Steps**:
1. **Given**: `notifySelfResolved = true`, K8s client returns error on `Create` for NotificationRequest
2. **When**: `HandleAIAnalysisStatus` is called with WorkflowNotNeeded AIAnalysis
3. **Then**: Handler returns success (no error), RR status is Completed

**Expected Results**:
1. Return error is `nil`
2. RR `OverallPhase` = `Completed`
3. RR `Outcome` = `NoActionRequired`
4. RR `CompletedAt` is set
5. `NotificationRequestRefs` is empty (creation failed)

**Acceptance Criteria**:
- **Behavior**: Handler's primary duty (RR completion) is unblocked by notification failure
- **Correctness**: No error propagated to reconciler (no requeue churn)
- **Accuracy**: RR status reflects normal completion

---

### UT-RO-590-007: No notification when notifySelfResolved is false

**BR**: BR-ORCH-037 AC-037-09
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Preconditions**:
- Handler with `notifySelfResolved = false` (default — setter never called)

**Test Steps**:
1. **Given**: `notifySelfResolved = false`, AIAnalysis with `Reason=WorkflowNotNeeded`
2. **When**: `HandleAIAnalysisStatus` is called
3. **Then**: No NotificationRequest is created

**Expected Results**:
1. No `NotificationRequest` objects exist in the fake client
2. `rr.Status.NotificationRequestRefs` is empty
3. RR completes normally (Completed, NoActionRequired)

**Acceptance Criteria**:
- **Behavior**: Existing behavior preserved — operators who haven't opted in see no change
- **Correctness**: Zero NR CRDs created
- **Accuracy**: All existing RR status fields unchanged from pre-#590 behavior

---

### UT-RO-590-008: Handler outcomes preserved regardless of notification config

**BR**: BR-ORCH-037 AC-037-03, AC-037-04, AC-037-07
**Priority**: P1
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Preconditions**:
- Handler with `notifySelfResolved = true` and metrics initialized

**Test Steps**:
1. **Given**: `notifySelfResolved = true`, AIAnalysis with `Reason=WorkflowNotNeeded`, `SubReason=problem_resolved`
2. **When**: `HandleAIAnalysisStatus` is called
3. **Then**: All existing handler outcomes are preserved

**Expected Results**:
1. RR `OverallPhase` = `Completed`
2. RR `Outcome` = `NoActionRequired`
3. RR `CompletedAt` is set
4. RR `Message` = AI's message
5. Ready condition set to true
6. `NoActionNeededTotal` metric incremented with correct labels

**Acceptance Criteria**:
- **Behavior**: Enabling notifications doesn't alter the handler's primary outcomes
- **Correctness**: Same RR terminal state as when notifications are disabled

---

### UT-RO-590-009: DefaultConfig sets NotifySelfResolved to false

**BR**: BR-ORCH-037 AC-037-09
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/config_test.go` (new or existing)

**Preconditions**: None

**Test Steps**:
1. **Given**: No configuration override
2. **When**: `DefaultConfig()` is called
3. **Then**: `Notifications.NotifySelfResolved` is `false`

**Expected Results**:
1. `cfg.Notifications.NotifySelfResolved == false`

**Acceptance Criteria**:
- **Behavior**: Safe default — no surprise notifications on upgrade
- **Correctness**: Zero-value bool gives correct default

---

### UT-RO-590-010: NotificationsConfig round-trips through YAML

**BR**: BR-ORCH-037 (config correctness)
**Priority**: P1
**Type**: Unit
**File**: `test/unit/remediationorchestrator/config_test.go` (new or existing)

**Preconditions**: None

**Test Steps**:
1. **Given**: YAML string containing `notifications:\n  notifySelfResolved: true`
2. **When**: YAML is unmarshalled into `Config` struct
3. **Then**: `cfg.Notifications.NotifySelfResolved` is `true`

**Expected Results**:
1. YAML with `true` produces `NotifySelfResolved = true`
2. YAML with field omitted produces `NotifySelfResolved = false` (zero-value default)

**Acceptance Criteria**:
- **Behavior**: Operators can enable the feature via ConfigMap YAML
- **Correctness**: camelCase YAML key (`notifySelfResolved`) maps to Go field
- **Accuracy**: Omitted field defaults safely to `false`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: K8s client via `fake.NewClientBuilder()` (external dependency)
- **Location**: `test/unit/remediationorchestrator/`
- **Resources**: Standard CI runner

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22 | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #588 | Code | Merged (same branch) | Sentinel filtering in same `notification.go` file | None needed |

### 11.2 Execution Order

1. **Phase 1**: Config tests (UT-RO-590-009, UT-RO-590-010) — establish config structure
2. **Phase 2**: Creator tests (UT-RO-590-001..004) — establish notification creation
3. **Phase 3**: Handler tests (UT-RO-590-005..008) — wire creator into handler with conditional logic
4. **Phase 4**: Validate build + full regression

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/590/TEST_PLAN.md` | Strategy and test design |
| Config unit tests | `test/unit/remediationorchestrator/config_test.go` | Config loading and defaults |
| Creator unit tests | `test/unit/remediationorchestrator/notification_creator_test.go` | Self-resolved NR creation |
| Handler unit tests | `test/unit/remediationorchestrator/aianalysis_handler_test.go` | Conditional notification in handler |

---

## 13. Execution

```bash
# All unit tests for RO service
go test ./test/unit/remediationorchestrator/... -ginkgo.v

# Specific test by ID
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-590"

# Coverage
go test ./test/unit/remediationorchestrator/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates

None. The setter pattern (`SetNotifySelfResolved`) avoids constructor signature changes,
so no existing `NewAIAnalysisHandler` or `NewReconciler` call sites need updating.
Existing tests that exercise `handleWorkflowNotNeeded` test with the default behavior
(`notifySelfResolved = false`), which is unchanged.

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
