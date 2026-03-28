# Test Plan: Completion Notification with EA Verification Results

**Feature**: Enrich completion notifications with operator-facing EA verification summary and typed VerificationContext for programmatic routing
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `development/v1.2`

**Authority**:
- BR-ORCH-045: Completion notification must include verification outcome
- ADR-EM-001: EA lifecycle tracking
- #304: Notification deferred until after EA completes (post-verification)
- #318: Completion notification should include EA verification results

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- GitHub Issue: [#318](https://github.com/jordigilh/kubernaut/issues/318)

---

## 1. Scope

### In Scope

- **Verification summary builder** (`pkg/remediationorchestrator/creator/notification.go`): New `buildVerificationSummary` and `buildComponentBullets` functions that map EA `AssessmentReason` to static, operator-friendly messages
- **VerificationContext type** (`api/notification/v1alpha1/notificationrequest_types.go`): New typed struct on `NotificationContext` enabling programmatic routing for escalation channels
- **FlattenToMap extension** (`api/notification/v1alpha1/notificationrequest_types.go`): New `verificationAssessed`, `verificationOutcome`, `verificationReason` keys in flat map
- **CreateCompletionNotification wiring** (`pkg/remediationorchestrator/creator/notification.go`): Accept optional EA param, populate body + typed context
- **EA fetch in reconciler** (`internal/controller/remediationorchestrator/reconciler.go`): Fetch EA via `EffectivenessAssessmentRef` and pass to notification creator

### Out of Scope

- Routing rule configuration (operators configure their own rules using the new flat map keys)
- Notification delivery mechanism changes (delivery orchestrator is unmodified)
- EA CRD schema changes (EA is read-only for this feature)
- RR CRD schema changes (RR already has `EffectivenessAssessmentRef`)
- E2E tests (reconciler wiring is integration-testable; no Kind cluster needed)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Static operator-friendly messages keyed on AssessmentReason | Operators care about "did it work?" not raw scores. Messages like "Verification passed" or "spec modified externally" are actionable without domain expertise. |
| Component bullets only for non-passing assessed items | Avoids noise -- "Verification passed" doesn't need 4 green checkmarks. Only noteworthy items (failures, drift) are surfaced. |
| Components not assessed are omitted entirely (not "N/A") | Handles heterogeneous resources naturally (e.g., Node has no metrics) |
| Typed VerificationContext on NotificationContext + body text | Body for human consumption (Slack/email); typed struct for programmatic routing to escalation channels |
| EA param is nilable with graceful degradation | Notification must still be created when EA is unavailable; shows "Verification: not available" |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (verification summary builder, component bullets, VerificationContext type, FlattenToMap)
- **Integration**: >=80% of integration-testable code (CreateCompletionNotification wiring, reconciler EA fetch, notification creation with envtest)

### 2-Tier Minimum

Every business requirement gap is covered by Unit + Integration tiers:
- **Unit tests** validate message mapping correctness, component bullet logic, FlattenToMap keys, edge cases (nil EA, partial components)
- **Integration tests** validate CreateCompletionNotification with real fake K8s client, EA fetch from EffectivenessAssessmentRef, notification spec correctness

### Business Outcome Quality Bar

Tests validate business outcomes -- "operator sees accurate verification summary in notification" and "routing rules can match on verification outcome" -- not just code path coverage. Each test asserts the operator experience (readable message content, correct routing keys).

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/creator/notification.go` (NEW) | `buildVerificationSummary`, `buildComponentBullets`, `mapAssessmentReasonToOutcome` | ~80 |
| `api/notification/v1alpha1/notificationrequest_types.go` | `VerificationContext` struct, `FlattenToMap` extension | ~25 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/creator/notification.go` | `CreateCompletionNotification` (EA param, body + context wiring) | ~30 (delta) |
| `internal/controller/remediationorchestrator/reconciler.go` | `ensureNotificationsCreated` (EA fetch) | ~15 (delta) |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-045 | Full verification: operator sees "passed" summary | P0 | Unit | UT-RO-318-001 | GREEN |
| BR-ORCH-045 | Spec drift: operator sees actionable drift warning | P0 | Unit | UT-RO-318-002 | GREEN |
| BR-ORCH-045 | Partial verification: summary with non-passing bullets only | P0 | Unit | UT-RO-318-003 | GREEN |
| BR-ORCH-045 | Alert decay timeout: operator sees alert persistence warning | P1 | Unit | UT-RO-318-004 | GREEN |
| BR-ORCH-045 | Metrics timed out: operator sees metrics unavailable message | P1 | Unit | UT-RO-318-005 | GREEN |
| BR-ORCH-045 | Expired assessment: operator sees expiration message | P1 | Unit | UT-RO-318-006 | GREEN |
| BR-ORCH-045 | No execution: operator sees verification skipped message | P1 | Unit | UT-RO-318-007 | GREEN |
| BR-ORCH-045 | Nil EA: operator sees "not available" (graceful degradation) | P0 | Unit | UT-RO-318-008 | GREEN |
| BR-ORCH-045 | Component bullets omitted for unassessed components (Node) | P0 | Unit | UT-RO-318-009 | GREEN |
| BR-ORCH-045 | Component bullets omitted when all checks pass (full) | P0 | Unit | UT-RO-318-010 | GREEN |
| BR-ORCH-045 | FlattenToMap includes verificationOutcome for routing | P0 | Unit | UT-RO-318-011 | GREEN |
| BR-ORCH-045 | FlattenToMap omits verification keys when nil | P1 | Unit | UT-RO-318-012 | GREEN |
| BR-ORCH-045 | Completion notification body contains verification section | P0 | Integration | IT-RO-318-001 | GREEN |
| BR-ORCH-045 | Completion notification Context.Verification populated | P0 | Integration | IT-RO-318-002 | GREEN |
| BR-ORCH-045 | Completion notification with nil EA still created | P0 | Integration | IT-RO-318-003 | GREEN |
| BR-ORCH-045 | Completion notification with EA ref fetches and includes EA | P0 | Integration | IT-RO-318-004 | GREEN |
| BR-ORCH-045 | Completion notification with missing EA ref degrades gracefully | P1 | Integration | IT-RO-318-005 | GREEN |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-RO-318-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: RO (Remediation Orchestrator)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `buildVerificationSummary` (100%), `buildComponentBullets` (100%), `FlattenToMap` verification keys (100%), `mapAssessmentReasonToOutcome` (100%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-318-001` | Operator sees "Verification passed: all checks confirmed the remediation was effective" when EA reason is `full` | Pending |
| `UT-RO-318-002` | Operator sees "resource spec was modified by an external entity" warning when EA reason is `spec_drift`, with hash mismatch bullet | Pending |
| `UT-RO-318-003` | Operator sees "some checks could not be performed" with bullets for non-passing components only when EA reason is `partial` | Pending |
| `UT-RO-318-004` | Operator sees "related alerts persisted beyond the assessment window" when EA reason is `alert_decay_timeout` | Pending |
| `UT-RO-318-005` | Operator sees "metrics were not available" when EA reason is `metrics_timed_out` | Pending |
| `UT-RO-318-006` | Operator sees "assessment window expired" when EA reason is `expired` | Pending |
| `UT-RO-318-007` | Operator sees "no workflow execution was found" when EA reason is `no_execution` | Pending |
| `UT-RO-318-008` | Operator sees "Verification: not available" when EA is nil (graceful degradation) | Pending |
| `UT-RO-318-009` | Node resource (no metrics) produces no metrics bullet; only assessed components that failed appear | Pending |
| `UT-RO-318-010` | Full verification produces no component bullets (summary line only, no noise) | GREEN |
| `UT-RO-318-011` | FlattenToMap returns `verificationOutcome: "inconclusive"` for spec_drift, enabling routing rules | GREEN |
| `UT-RO-318-012` | FlattenToMap omits all verification keys when Verification is nil (backward compat) | GREEN |

### Tier 2: Integration Tests

**Testable code scope**: `CreateCompletionNotification` EA wiring (100%), `ensureNotificationsCreated` EA fetch (~80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-RO-318-001` | Completion notification body contains "Verification Results" section with correct summary for full EA | GREEN |
| `IT-RO-318-002` | Completion notification `Spec.Context.Verification` populated with assessed=true, outcome, reason, summary | GREEN |
| `IT-RO-318-003` | Completion notification created successfully when EA is nil, body says "not available" | GREEN |
| `IT-RO-318-004` | `ensureNotificationsCreated` fetches EA via `EffectivenessAssessmentRef` and passes to creator | GREEN |
| `IT-RO-318-005` | `ensureNotificationsCreated` passes nil EA when ref is absent, notification still created | GREEN |

### Tier Skip Rationale

- **E2E**: The feature is a data-enrichment pass (read EA, format message, write to NotificationRequest spec). No new K8s resource types, no new API endpoints, no multi-service coordination. Unit + Integration provide sufficient coverage. E2E would require Kind + full RO + EM + NT stack for minimal incremental value over integration tests with envtest/fake client.

---

## 6. Test Cases (Detail)

### UT-RO-318-001: Full verification summary

**BR**: BR-ORCH-045
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Given**: An EA with `AssessmentReason = "full"`, `Phase = "Completed"`, all component scores healthy
**When**: `buildVerificationSummary(ea)` is called
**Then**: Returns summary "Verification passed: all checks confirmed the remediation was effective." and outcome "passed"

**Acceptance Criteria**:
- Summary string contains "Verification passed"
- `VerificationContext.Outcome == "passed"`
- `VerificationContext.Reason == "full"`
- `VerificationContext.Assessed == true`
- No component bullets in the body text (empty string for bullets)

### UT-RO-318-002: Spec drift warning

**BR**: BR-ORCH-045
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Given**: An EA with `AssessmentReason = "spec_drift"`, `Components.HashComputed = true`, `PostRemediationSpecHash != CurrentSpecHash`
**When**: `buildVerificationSummary(ea)` is called
**Then**: Returns summary containing "resource spec was modified by an external entity" and a hash mismatch bullet

**Acceptance Criteria**:
- Summary contains "modified by an external entity"
- `VerificationContext.Outcome == "inconclusive"`
- Component bullets contain "Resource integrity: spec modified externally after remediation"
- `VerificationContext.Assessed == true`

### UT-RO-318-003: Partial verification with selective bullets

**BR**: BR-ORCH-045
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Given**: An EA with `AssessmentReason = "partial"`, `HealthAssessed = true`, `HealthScore = 1.0`, `AlertAssessed = true`, `AlertScore = 0.0`, `MetricsAssessed = false`
**When**: `buildVerificationSummary(ea)` is called
**Then**: Summary says "some checks could not be performed"; bullets include only the failing alert; health (passed) and metrics (unassessed) omitted

**Acceptance Criteria**:
- Summary contains "some checks could not be performed"
- Bullets contain "Related alerts: still firing"
- Bullets do NOT contain "Pod health" (it passed)
- Bullets do NOT contain "Metrics" (not assessed)
- `VerificationContext.Outcome == "partial"`

### UT-RO-318-004: Alert decay timeout

**BR**: BR-ORCH-045
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Given**: An EA with `AssessmentReason = "alert_decay_timeout"`
**When**: `buildVerificationSummary(ea)` is called
**Then**: Summary contains "related alerts persisted beyond the assessment window"

**Acceptance Criteria**:
- `VerificationContext.Outcome == "inconclusive"`
- `VerificationContext.Reason == "alert_decay_timeout"`

### UT-RO-318-005: Metrics timed out

**BR**: BR-ORCH-045
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Given**: An EA with `AssessmentReason = "metrics_timed_out"`
**When**: `buildVerificationSummary(ea)` is called
**Then**: Summary contains "metrics were not available"

**Acceptance Criteria**:
- `VerificationContext.Outcome == "partial"`
- `VerificationContext.Reason == "metrics_timed_out"`

### UT-RO-318-006: Expired assessment

**BR**: BR-ORCH-045
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Given**: An EA with `AssessmentReason = "expired"`
**When**: `buildVerificationSummary(ea)` is called
**Then**: Summary contains "assessment window expired"

**Acceptance Criteria**:
- `VerificationContext.Outcome == "unavailable"`
- `VerificationContext.Reason == "expired"`

### UT-RO-318-007: No execution

**BR**: BR-ORCH-045
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Given**: An EA with `AssessmentReason = "no_execution"`
**When**: `buildVerificationSummary(ea)` is called
**Then**: Summary contains "no workflow execution was found"

**Acceptance Criteria**:
- `VerificationContext.Outcome == "unavailable"`
- `VerificationContext.Reason == "no_execution"`

### UT-RO-318-008: Nil EA graceful degradation

**BR**: BR-ORCH-045
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Given**: EA is nil
**When**: `buildVerificationSummary(nil)` is called
**Then**: Returns "Verification: not available." with assessed=false

**Acceptance Criteria**:
- Summary == "Verification: not available."
- `VerificationContext.Assessed == false`
- `VerificationContext.Outcome == "unavailable"`
- `VerificationContext.Reason == ""`

### UT-RO-318-009: Node resource (no metrics) omits metrics bullet

**BR**: BR-ORCH-045
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Given**: An EA with `AssessmentReason = "partial"`, `HealthAssessed = true`, `HealthScore = 0.0`, `MetricsAssessed = false`, `AlertAssessed = false`, `HashComputed = false`
**When**: `buildComponentBullets(ea)` is called
**Then**: Only health bullet appears; no metrics, alert, or hash bullets

**Acceptance Criteria**:
- Result contains "Pod health: not recovered"
- Result does NOT contain "Metrics"
- Result does NOT contain "alerts"
- Result does NOT contain "Resource integrity"

### UT-RO-318-010: Full verification has no component bullets

**BR**: BR-ORCH-045
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Given**: An EA with `AssessmentReason = "full"`, all components assessed and passing
**When**: `buildComponentBullets(ea)` is called
**Then**: Returns empty string (no bullets needed for full pass)

**Acceptance Criteria**:
- Result == ""

### UT-RO-318-011: FlattenToMap includes verification routing keys

**BR**: BR-ORCH-045
**Type**: Unit
**File**: `test/unit/notification/notification_context_flatten_test.go`

**Given**: A `NotificationContext` with `Verification` set to `{Assessed: true, Outcome: "inconclusive", Reason: "spec_drift"}`
**When**: `FlattenToMap()` is called
**Then**: Map contains `verificationOutcome: "inconclusive"`, `verificationReason: "spec_drift"`, `verificationAssessed: "true"`

**Acceptance Criteria**:
- `m["verificationOutcome"] == "inconclusive"`
- `m["verificationReason"] == "spec_drift"`
- `m["verificationAssessed"] == "true"`

### UT-RO-318-012: FlattenToMap backward compat with nil Verification

**BR**: BR-ORCH-045
**Type**: Unit
**File**: `test/unit/notification/notification_context_flatten_test.go`

**Given**: A `NotificationContext` with `Verification = nil`
**When**: `FlattenToMap()` is called
**Then**: Map does NOT contain any `verification*` keys

**Acceptance Criteria**:
- `m["verificationOutcome"]` not present
- `m["verificationReason"]` not present
- `m["verificationAssessed"]` not present

### IT-RO-318-001: Completion notification body with verification section

**BR**: BR-ORCH-045
**Type**: Integration
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Given**: A completed RR with EA ref pointing to a `full` EA, AIAnalysis with root cause
**When**: `CreateCompletionNotification(ctx, rr, ai, ea)` is called
**Then**: Created NotificationRequest body contains "Verification Results" section with "Verification passed"

**Acceptance Criteria**:
- `nr.Spec.Body` contains "Verification Results"
- `nr.Spec.Body` contains "Verification passed"
- Existing body content (Signal, Severity, Root Cause, etc.) unchanged

### IT-RO-318-002: Completion notification typed context populated

**BR**: BR-ORCH-045
**Type**: Integration
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Given**: A completed RR with EA ref pointing to a `spec_drift` EA
**When**: `CreateCompletionNotification(ctx, rr, ai, ea)` is called
**Then**: `nr.Spec.Context.Verification` populated with assessed=true, outcome="inconclusive", reason="spec_drift"

**Acceptance Criteria**:
- `nr.Spec.Context.Verification != nil`
- `nr.Spec.Context.Verification.Assessed == true`
- `nr.Spec.Context.Verification.Outcome == "inconclusive"`
- `nr.Spec.Context.Verification.Reason == "spec_drift"`
- `nr.Spec.Context.Verification.Summary` contains "modified by an external entity"

### IT-RO-318-003: Completion notification with nil EA

**BR**: BR-ORCH-045
**Type**: Integration
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Given**: A completed RR, AIAnalysis, and nil EA
**When**: `CreateCompletionNotification(ctx, rr, ai, nil)` is called
**Then**: NotificationRequest created successfully; body says "Verification: not available."

**Acceptance Criteria**:
- No error returned
- `nr.Spec.Body` contains "Verification: not available"
- `nr.Spec.Context.Verification.Assessed == false`
- `nr.Spec.Context.Verification.Outcome == "unavailable"`

### IT-RO-318-004: Reconciler fetches EA via ref

**BR**: BR-ORCH-045
**Type**: Integration
**File**: `test/unit/remediationorchestrator/controller/notification_retry_test.go`

**Given**: A completed RR with `Status.EffectivenessAssessmentRef` pointing to an EA object in the fake client, plus AIAnalysis
**When**: `ensureNotificationsCreated(ctx, rr)` is called (via reconciler)
**Then**: The completion notification includes EA verification data

**Acceptance Criteria**:
- EA is fetched via the ref (no error)
- Notification body contains "Verification Results" section
- Context.Verification is populated

### IT-RO-318-005: Reconciler handles missing EA ref gracefully

**BR**: BR-ORCH-045
**Type**: Integration
**File**: `test/unit/remediationorchestrator/controller/notification_retry_test.go`

**Given**: A completed RR with `Status.EffectivenessAssessmentRef = nil`
**When**: `ensureNotificationsCreated(ctx, rr)` is called
**Then**: Notification is still created; body says "Verification: not available"

**Acceptance Criteria**:
- No error or panic
- Notification created with name `nr-completion-{rr.Name}`
- Body contains "not available"

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None needed -- `buildVerificationSummary` and `buildComponentBullets` are pure functions taking EA struct pointers
- **Location**: `test/unit/remediationorchestrator/verification_summary_test.go`, `test/unit/notification/notification_context_test.go`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks -- uses `fake.NewClientBuilder()` for K8s API (standard envtest pattern)
- **Infrastructure**: No external services required
- **Location**: `test/unit/remediationorchestrator/notification_creator_test.go` (IT-001 through 003), `test/unit/remediationorchestrator/controller/notification_retry_test.go` (IT-004, IT-005 -- reconciler-level)

---

## 8. Execution

```bash
# Unit tests -- verification summary
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-318"

# Unit tests -- FlattenToMap
go test ./test/unit/notification/... -ginkgo.focus="UT-RO-318-01[12]"

# Integration tests -- notification creator
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="IT-RO-318"

# All RO unit tests
make test

# Full build
go build ./...
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan: 12 unit + 5 integration = 17 scenarios |
