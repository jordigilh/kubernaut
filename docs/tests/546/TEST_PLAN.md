# Test Plan: Notification Indicates EA Degraded on Hash Capture Failure (#546)

**Feature**: Completion notification and EA status surface pre/post-remediation hash capture degradation to operators with actionable guidance
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Agent
**Status**: Draft
**Branch**: `development/v1.2`

**Authority**:
- Issue #546: Notification should indicate EA degraded when pre/post-remediation hash capture fails
- Issue #545: RO ClusterRole RBAC and resilient hash capture (prerequisite -- soft-fail signature)
- DD-EM-002: Pre-remediation spec hash capture for effectiveness assessment
- DD-CRD-002: EffectivenessAssessment conditions (canonical SetStatusCondition pattern)
- BR-ORCH-043: RemediationRequest conditions (canonical SetStatusCondition pattern)

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../development/business-requirements/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Test Plan #545](../545/TEST_PLAN.md) — prerequisite soft-fail implementation

---

## 1. Scope

### In Scope

- **RR Condition `PreRemediationHashCaptured`**: New K8s condition on RemediationRequest persisting hash capture success/failure with the degradation reason. Visible in `kubectl describe rr`.
- **EA Condition `PostHashCaptured`**: New K8s condition on EffectivenessAssessment persisting post-hash capture success/failure. Follows DD-CRD-002 pattern.
- **`VerificationContext` degradation fields**: `Degraded` (bool) and `DegradedReason` (string) on the notification CRD's `VerificationContext` struct for programmatic routing.
- **`BuildVerificationSummary` enhancement**: Extended to accept RR, read both RR and EA conditions, and produce degradation text + typed context.
- **EM `getTargetSpec` degradation reason**: Returns `(map, degradedReason)` so `assessHash` can set the EA condition.
- **Completion notification body**: Includes actionable degradation warning section when pre- or post-hash capture failed.
- **`FlattenToMap` update**: New `verificationDegraded` and `verificationDegradedReason` keys for routing compatibility.

### Out of Scope

- **Helm chart changes**: No RBAC changes needed (covered by #545).
- **E2E tests**: Full E2E validation (deploy to Kind, trigger RBAC-denied scenario) deferred to release pipeline. The degradation is triggered by RBAC gaps that are difficult to reproduce safely in E2E without a dedicated operator.
- **DataStorage remediation history**: The degradation is a notification/condition concern; no DS schema changes.
- **New notification type**: Uses existing `completion` type; no new `NotificationType` constant.

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Persist degradation via K8s Conditions (not a new status field) | Follows established K8s API conventions (KEP-1623). Conditions are visible in `kubectl describe`, support `LastTransitionTime`, and are the canonical mechanism for both RR (BR-ORCH-043) and EA (DD-CRD-002). No CRD schema change needed since `Conditions` fields already exist on both types. |
| Separate conditions for pre-hash (RR) and post-hash (EA) | Pre-hash is captured by the RO controller and stored on the RR. Post-hash is captured by the EM controller and stored on the EA. Each controller owns its own condition lifecycle. |
| `BuildVerificationSummary` takes `rr` parameter | The function already has access to EA; adding RR gives it access to the pre-hash condition without a separate helper or extra plumbing. The reconciler's `ensureNotificationsCreated` already has both objects loaded. |
| `getTargetSpec` returns `(map, string)` tuple | Minimal blast radius. The second value is empty on success or NotFound (not degraded). Non-empty for Forbidden/transient errors (degraded). Follows the same pattern as `CapturePreRemediationHash`'s `degradedReason`. |
| `VerificationContext.Degraded` + `DegradedReason` | Enables routing rules to match on degradation (e.g., `verification.degraded == true` → escalation channel). Separate from `Outcome` because degradation is orthogonal — the EA may still complete with outcome "passed" but the hash comparison is meaningless. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of new and modified unit-testable code. Targets:
  - `pkg/remediationrequest/conditions.go`: new condition type + helper (100% of new code)
  - `pkg/effectivenessmonitor/conditions/conditions.go`: new condition type + reasons (100% of new code)
  - `pkg/remediationorchestrator/creator/notification.go`: `BuildVerificationSummary` degradation paths, `buildCompletionBody` degradation section, `FlattenToMap` new keys (>=80%)
- **Integration**: >=80% of new and modified integration-testable code. Targets:
  - `internal/controller/remediationorchestrator/reconciler.go`: condition set at 2 call sites (>=80%)
  - `internal/controller/effectivenessmonitor/reconciler.go`: `getTargetSpec` degradation + `assessHash` condition (>=80%)

### 2-Tier Minimum

Every business requirement gap is covered by at least 2 tiers:
- **Unit tests**: Validate condition helpers, notification body construction, verification context population, `FlattenToMap` output
- **Integration tests**: Validate reconciler sets conditions correctly during hash capture, EM sets post-hash condition, notification is created with degradation context end-to-end

### Tier Skip Rationale

- **E2E**: Deferred. Requires deploying to Kind and triggering a Forbidden error on a specific CRD read. The only reliable way is to create a ClusterRole that explicitly denies a CRD, which is operationally complex. E2E coverage will come from the release pipeline's cert-manager scenario where `view` may not cover custom CRDs.

### Business Outcome Quality Bar

Every test validates an observable business outcome:
- "Does the operator see the degradation reason in `kubectl describe rr`?"
- "Does the completion notification tell the operator what went wrong and how to fix it?"
- "Can routing rules match on `verification.degraded` to escalate degraded notifications?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationrequest/conditions.go` | `SetPreRemediationHashCaptured` (new) | ~15 |
| `pkg/effectivenessmonitor/conditions/conditions.go` | `ConditionPostHashCaptured` constant + reasons (new) | ~15 |
| `api/notification/v1alpha1/notificationrequest_types.go` | `VerificationContext.Degraded/DegradedReason` (new), `FlattenToMap` update | ~10 |
| `pkg/remediationorchestrator/creator/notification.go` | `BuildVerificationSummary` (modified), `buildCompletionBody` (modified) | ~40 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | 2 `CapturePreRemediationHash` call sites + condition setting | ~20 |
| `internal/controller/effectivenessmonitor/reconciler.go` | `getTargetSpec` (modified return), `assessHash` (condition setting) | ~30 |

---

## 4. BR Coverage Matrix

| BR/Issue | Description | Priority | Tier | Test ID | Status |
|----------|-------------|----------|------|---------|--------|
| #546-RR-COND | RR condition persists pre-hash degradation reason | P0 | Unit | UT-RO-546-001 | Pending |
| #546-RR-COND | RR condition persists pre-hash success | P0 | Unit | UT-RO-546-002 | Pending |
| #546-EA-COND | EA condition persists post-hash degradation reason | P0 | Unit | UT-EM-546-001 | Pending |
| #546-EA-COND | EA condition persists post-hash success | P0 | Unit | UT-EM-546-002 | Pending |
| #546-NOTIF-BODY | Completion body includes pre-hash degradation warning | P0 | Unit | UT-RO-546-003 | Pending |
| #546-NOTIF-BODY | Completion body includes post-hash degradation warning | P0 | Unit | UT-RO-546-004 | Pending |
| #546-NOTIF-BODY | Completion body includes both degradation warnings | P1 | Unit | UT-RO-546-005 | Pending |
| #546-NOTIF-BODY | Completion body has no degradation when hashes OK | P0 | Unit | UT-RO-546-006 | Pending |
| #546-VERIF-CTX | VerificationContext.Degraded=true with pre-hash failure | P0 | Unit | UT-RO-546-007 | Pending |
| #546-VERIF-CTX | VerificationContext.Degraded=true with post-hash failure | P0 | Unit | UT-RO-546-008 | Pending |
| #546-VERIF-CTX | VerificationContext.Degraded=false when hashes OK | P0 | Unit | UT-RO-546-009 | Pending |
| #546-FLATTEN | FlattenToMap includes verificationDegraded keys | P1 | Unit | UT-RO-546-010 | Pending |
| #546-EM-SPEC | getTargetSpec returns degradation reason for Forbidden | P0 | Unit | UT-EM-546-003 | Pending |
| #546-EM-SPEC | getTargetSpec returns empty reason for NotFound | P0 | Unit | UT-EM-546-004 | Pending |
| #546-EM-SPEC | getTargetSpec returns empty reason on success | P0 | Unit | UT-EM-546-005 | Pending |
| #546-RECONCILER | RO sets PreRemediationHashCaptured=False when degraded | P0 | Integration | IT-RO-546-001 | Pending |
| #546-RECONCILER | RO sets PreRemediationHashCaptured=True on success | P0 | Integration | IT-RO-546-002 | Pending |
| #546-EM-HASH | EM sets PostHashCaptured=False when spec fetch fails | P0 | Integration | IT-EM-546-001 | Pending |
| #546-EM-HASH | EM sets PostHashCaptured=True on success | P0 | Integration | IT-EM-546-002 | Pending |
| #546-NOTIF-E2E | Completion notification created with degradation context | P0 | Integration | IT-RO-546-003 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-546-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `RO` (Remediation Orchestrator), `EM` (Effectiveness Monitor)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**:
- `pkg/remediationrequest/conditions.go` (new helper): 100%
- `pkg/effectivenessmonitor/conditions/conditions.go` (new constants + usage): 100%
- `pkg/remediationorchestrator/creator/notification.go` (`BuildVerificationSummary`, `buildCompletionBody`, `FlattenToMap`): >=80%
- `api/notification/v1alpha1/notificationrequest_types.go` (`FlattenToMap`): >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-546-001` | Operator can see "PreRemediationHashCaptured=False" with reason in `kubectl describe rr` when hash capture is degraded | Pending |
| `UT-RO-546-002` | Operator can see "PreRemediationHashCaptured=True" in `kubectl describe rr` when hash capture succeeds | Pending |
| `UT-EM-546-001` | EA status shows "PostHashCaptured=False" with reason when EM cannot read target spec | Pending |
| `UT-EM-546-002` | EA status shows "PostHashCaptured=True" when EM successfully captures post-hash | Pending |
| `UT-RO-546-003` | Completion notification body warns operator about pre-hash degradation with actionable RBAC guidance | Pending |
| `UT-RO-546-004` | Completion notification body warns operator about post-hash degradation with actionable RBAC guidance | Pending |
| `UT-RO-546-005` | Completion notification body shows both degradation warnings when both pre- and post-hash failed | Pending |
| `UT-RO-546-006` | Completion notification body has no degradation section when both hashes captured successfully | Pending |
| `UT-RO-546-007` | VerificationContext.Degraded=true and DegradedReason populated when pre-hash condition is False | Pending |
| `UT-RO-546-008` | VerificationContext.Degraded=true and DegradedReason populated when EA post-hash condition is False | Pending |
| `UT-RO-546-009` | VerificationContext.Degraded=false and DegradedReason empty when all hash captures succeed | Pending |
| `UT-RO-546-010` | FlattenToMap produces "verificationDegraded" and "verificationDegradedReason" keys for routing rules | Pending |
| `UT-EM-546-003` | getTargetSpec returns degradation reason (non-empty) when K8s API returns Forbidden | Pending |
| `UT-EM-546-004` | getTargetSpec returns empty degradation reason when resource is NotFound (not degraded, just absent) | Pending |
| `UT-EM-546-005` | getTargetSpec returns empty degradation reason when spec retrieved successfully | Pending |

### Tier 2: Integration Tests

**Testable code scope**:
- `internal/controller/remediationorchestrator/reconciler.go` (condition-setting at hash capture): >=80%
- `internal/controller/effectivenessmonitor/reconciler.go` (`assessHash` condition-setting): >=80%
- `pkg/remediationorchestrator/creator/notification.go` (end-to-end notification creation): >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-RO-546-001` | RO controller persists PreRemediationHashCaptured=False condition on RR when CapturePreRemediationHash returns degradedReason | Pending |
| `IT-RO-546-002` | RO controller persists PreRemediationHashCaptured=True condition on RR when hash capture succeeds | Pending |
| `IT-EM-546-001` | EM controller persists PostHashCaptured=False condition on EA when target spec fetch returns Forbidden | Pending |
| `IT-EM-546-002` | EM controller persists PostHashCaptured=True condition on EA when spec fetch succeeds | Pending |
| `IT-RO-546-003` | Completion notification created with VerificationContext.Degraded=true when RR has PreRemediationHashCaptured=False | Pending |

### Tier Skip Rationale

- **E2E**: Deferred. Triggering a Forbidden error on a specific CRD read requires a dedicated ClusterRole that denies access, which is operationally complex in Kind. The cert-manager E2E scenario in the release pipeline provides indirect coverage.

---

## 6. Test Cases (Detail)

### UT-RO-546-001: RR condition set to False when pre-hash degraded

**BR**: #546-RR-COND
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go` (or new `test/unit/remediationrequest/conditions_test.go`)

**Given**: A RemediationRequest with empty Conditions
**When**: `SetPreRemediationHashCaptured(rr, false, "HashCaptureFailed", "failed to fetch target resource Certificate/demo-app-cert: Forbidden", metrics)` is called
**Then**: RR has condition `PreRemediationHashCaptured` with Status=False, Reason="HashCaptureFailed", Message containing "Forbidden"

**Acceptance Criteria**:
- Condition type is exactly `"PreRemediationHashCaptured"`
- Condition.Status is `metav1.ConditionFalse`
- Condition.Reason is `"HashCaptureFailed"`
- Condition.Message contains the original Forbidden error text
- Condition.LastTransitionTime is set (not zero)

---

### UT-RO-546-002: RR condition set to True when pre-hash succeeds

**BR**: #546-RR-COND
**Type**: Unit
**File**: `test/unit/remediationrequest/conditions_test.go`

**Given**: A RemediationRequest with empty Conditions
**When**: `SetPreRemediationHashCaptured(rr, true, "HashCaptured", "Pre-remediation hash captured for Certificate/demo-app-cert", metrics)` is called
**Then**: RR has condition `PreRemediationHashCaptured` with Status=True, Reason="HashCaptured"

**Acceptance Criteria**:
- Condition.Status is `metav1.ConditionTrue`
- Condition.Reason is `"HashCaptured"`
- Condition.Message describes the captured target

---

### UT-EM-546-001: EA condition set to False when post-hash degraded

**BR**: #546-EA-COND
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/conditions_test.go`

**Given**: An EffectivenessAssessment with empty Conditions
**When**: `conditions.SetCondition(ea, ConditionPostHashCaptured, ConditionFalse, "PostHashCaptureFailed", "failed to fetch target resource Deployment/nginx: Forbidden")` is called
**Then**: EA has condition `PostHashCaptured` with Status=False, Message containing "Forbidden"

**Acceptance Criteria**:
- Condition type is exactly `"PostHashCaptured"`
- Condition.Status is `metav1.ConditionFalse`
- Condition.Reason is `"PostHashCaptureFailed"`
- Condition.Message contains the degradation reason

---

### UT-EM-546-002: EA condition set to True when post-hash succeeds

**BR**: #546-EA-COND
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/conditions_test.go`

**Given**: An EffectivenessAssessment with empty Conditions
**When**: `conditions.SetCondition(ea, ConditionPostHashCaptured, ConditionTrue, "PostHashCaptured", "Post-remediation spec hash captured")` is called
**Then**: EA has condition `PostHashCaptured` with Status=True

**Acceptance Criteria**:
- Condition.Status is `metav1.ConditionTrue`
- Condition.Reason is `"PostHashCaptured"`

---

### UT-RO-546-003: Completion body includes pre-hash degradation warning

**BR**: #546-NOTIF-BODY
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Given**: A RemediationRequest with condition `PreRemediationHashCaptured=False` (Message="failed to fetch Certificate/demo-app-cert: Forbidden"), a completed EA with no post-hash degradation
**When**: `BuildVerificationSummary(ea, rr)` is called
**Then**: The summary text contains a degradation warning with the pre-hash failure reason and actionable RBAC guidance

**Acceptance Criteria**:
- Summary contains "Effectiveness Assessment: Degraded" (or equivalent warning heading)
- Summary contains "pre-remediation spec hash" (identifies which hash failed)
- Summary contains "Forbidden" (the original error)
- Summary contains actionable guidance mentioning RBAC or `view` ClusterRoleBinding
- Standard verification text (from EA assessment) is still present (degradation is additive, not replacing)

---

### UT-RO-546-004: Completion body includes post-hash degradation warning

**BR**: #546-NOTIF-BODY
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Given**: A RemediationRequest with condition `PreRemediationHashCaptured=True` (hash OK), an EA with condition `PostHashCaptured=False` (Message="failed to fetch Deployment/nginx: Forbidden")
**When**: `BuildVerificationSummary(ea, rr)` is called
**Then**: The summary text contains a degradation warning with the post-hash failure reason

**Acceptance Criteria**:
- Summary contains "post-remediation spec hash" (identifies which hash failed)
- Summary contains the EM-specific RBAC guidance
- `VerificationContext.Degraded` is true

---

### UT-RO-546-005: Completion body includes both degradation warnings

**BR**: #546-NOTIF-BODY
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Given**: RR with `PreRemediationHashCaptured=False`, EA with `PostHashCaptured=False`
**When**: `BuildVerificationSummary(ea, rr)` is called
**Then**: Summary contains both pre-hash and post-hash degradation warnings

**Acceptance Criteria**:
- Summary contains both "pre-remediation" and "post-remediation" warnings
- `VerificationContext.Degraded` is true
- `VerificationContext.DegradedReason` mentions both failures

---

### UT-RO-546-006: No degradation section when hashes are OK

**BR**: #546-NOTIF-BODY
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Given**: RR with `PreRemediationHashCaptured=True`, EA with `PostHashCaptured=True` and AssessmentReason="full"
**When**: `BuildVerificationSummary(ea, rr)` is called
**Then**: Summary does NOT contain any degradation warning text

**Acceptance Criteria**:
- Summary does not contain "Degraded" or "degradation"
- `VerificationContext.Degraded` is false
- `VerificationContext.DegradedReason` is empty
- Standard verification text is present (e.g., "Verification passed")

---

### UT-RO-546-007: VerificationContext.Degraded=true for pre-hash failure

**BR**: #546-VERIF-CTX
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Given**: RR with `PreRemediationHashCaptured=False` (Reason="HashCaptureFailed", Message="Forbidden"), EA with normal status
**When**: `BuildVerificationSummary(ea, rr)` is called
**Then**: Returned `VerificationContext` has `Degraded=true`, `DegradedReason` containing "pre-remediation" and "Forbidden"

**Acceptance Criteria**:
- `ctx.Degraded` is `true`
- `ctx.DegradedReason` is non-empty and references the pre-hash failure
- Other `VerificationContext` fields (Assessed, Outcome, Reason) remain populated from the EA

---

### UT-RO-546-008: VerificationContext.Degraded=true for post-hash failure

**BR**: #546-VERIF-CTX
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Given**: RR with `PreRemediationHashCaptured=True`, EA with `PostHashCaptured=False`
**When**: `BuildVerificationSummary(ea, rr)` is called
**Then**: Returned `VerificationContext` has `Degraded=true`, `DegradedReason` referencing the post-hash failure

**Acceptance Criteria**:
- `ctx.Degraded` is `true`
- `ctx.DegradedReason` references "post-remediation"

---

### UT-RO-546-009: VerificationContext.Degraded=false when hashes OK

**BR**: #546-VERIF-CTX
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Given**: RR with `PreRemediationHashCaptured=True`, EA with `PostHashCaptured=True`
**When**: `BuildVerificationSummary(ea, rr)` is called
**Then**: `VerificationContext.Degraded` is false, `DegradedReason` is empty

**Acceptance Criteria**:
- `ctx.Degraded` is `false`
- `ctx.DegradedReason` is `""`

---

### UT-RO-546-010: FlattenToMap includes degradation keys

**BR**: #546-FLATTEN
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go` (or `test/unit/notification/context_test.go`)

**Given**: A `NotificationContext` with `Verification.Degraded=true` and `Verification.DegradedReason="pre-remediation hash Forbidden"`
**When**: `FlattenToMap()` is called
**Then**: The resulting map contains `"verificationDegraded": "true"` and `"verificationDegradedReason": "pre-remediation hash Forbidden"`

**Acceptance Criteria**:
- Key `"verificationDegraded"` exists with value `"true"`
- Key `"verificationDegradedReason"` exists with the reason string
- When `Degraded=false`, the `"verificationDegraded"` key has value `"false"`

---

### UT-EM-546-003: getTargetSpec returns degradation reason for Forbidden

**BR**: #546-EM-SPEC
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/hash_test.go` (or new file)

**Given**: A Kubernetes API client that returns `Forbidden` for `Get()` on the target resource
**When**: `getTargetSpec(ctx, target)` is called
**Then**: Returns `(emptyMap, "failed to fetch target resource Deployment/nginx: Forbidden")`

**Acceptance Criteria**:
- First return value is an empty map
- Second return value is a non-empty string containing "Forbidden" and the resource identity
- No error is propagated (function is soft-fail)

---

### UT-EM-546-004: getTargetSpec returns empty reason for NotFound

**BR**: #546-EM-SPEC
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/hash_test.go`

**Given**: A Kubernetes API client that returns `NotFound` for `Get()` on the target resource
**When**: `getTargetSpec(ctx, target)` is called
**Then**: Returns `(emptyMap, "")`

**Acceptance Criteria**:
- First return value is an empty map
- Second return value is empty string (NotFound is "not applicable", not "degraded")

---

### UT-EM-546-005: getTargetSpec returns empty reason on success

**BR**: #546-EM-SPEC
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/hash_test.go`

**Given**: A Kubernetes API client that returns a valid resource with `.spec`
**When**: `getTargetSpec(ctx, target)` is called
**Then**: Returns `(specMap, "")`

**Acceptance Criteria**:
- First return value is the target resource's spec as a map
- Second return value is empty string

---

### IT-RO-546-001: RO sets PreRemediationHashCaptured=False when degraded

**BR**: #546-RECONCILER
**Type**: Integration
**File**: `test/unit/remediationorchestrator/controller/reconcile_phases_test.go`

**Given**: A RemediationRequest in Analyzing phase with a target resource whose Kind is resolvable but the apiReader returns Forbidden on Get
**When**: The reconciler invokes `CapturePreRemediationHash` and receives `degradedReason != ""`
**Then**: The RR status has condition `PreRemediationHashCaptured=False` with Message containing the Forbidden error, and the pipeline continues (RR does NOT transition to Failed)

**Acceptance Criteria**:
- `GetCondition(rr, "PreRemediationHashCaptured")` returns non-nil
- Condition.Status is `False`
- Condition.Reason is `"HashCaptureFailed"`
- Condition.Message contains "Forbidden"
- RR phase is NOT `Failed` (pipeline continued)

---

### IT-RO-546-002: RO sets PreRemediationHashCaptured=True on success

**BR**: #546-RECONCILER
**Type**: Integration
**File**: `test/unit/remediationorchestrator/controller/reconcile_phases_test.go`

**Given**: A RemediationRequest in Analyzing phase with a target resource that the apiReader can successfully fetch
**When**: The reconciler invokes `CapturePreRemediationHash` and receives a valid hash
**Then**: The RR status has condition `PreRemediationHashCaptured=True`

**Acceptance Criteria**:
- `GetCondition(rr, "PreRemediationHashCaptured")` returns non-nil
- Condition.Status is `True`
- Condition.Reason is `"HashCaptured"`
- `rr.Status.PreRemediationSpecHash` is non-empty

---

### IT-EM-546-001: EM sets PostHashCaptured=False when spec fetch fails

**BR**: #546-EM-HASH
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/` (existing EM integration test suite)

**Given**: An EffectivenessAssessment in Assessing phase whose RemediationTarget refers to a resource the EM cannot read (Forbidden)
**When**: The EM reconciler runs `assessHash`
**Then**: EA has condition `PostHashCaptured=False` with Message containing the Forbidden error

**Acceptance Criteria**:
- EA condition `PostHashCaptured` exists with Status=False
- The EA is NOT set to Failed phase (pipeline continues)
- The hash component is still marked as computed (with empty hash)

---

### IT-EM-546-002: EM sets PostHashCaptured=True on success

**BR**: #546-EM-HASH
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/`

**Given**: An EffectivenessAssessment in Assessing phase whose RemediationTarget refers to a readable resource
**When**: The EM reconciler runs `assessHash`
**Then**: EA has condition `PostHashCaptured=True`

**Acceptance Criteria**:
- EA condition `PostHashCaptured` exists with Status=True
- `ea.Status.Components.PostRemediationSpecHash` is non-empty

---

### IT-RO-546-003: Completion notification with degradation context

**BR**: #546-NOTIF-E2E
**Type**: Integration
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Given**: A RemediationRequest with `PreRemediationHashCaptured=False` condition, Outcome="Remediated", a completed AIAnalysis, and a completed EA
**When**: `CreateCompletionNotification(ctx, rr, ai, "tekton", ea)` is called
**Then**: The created NotificationRequest has:
  - `Spec.Body` containing degradation warning text
  - `Spec.Context.Verification.Degraded` = true
  - `Spec.Context.Verification.DegradedReason` containing the failure reason

**Acceptance Criteria**:
- NotificationRequest is created successfully (no error)
- Body contains "Degraded" or "degraded" warning heading
- Body contains actionable guidance
- Context.Verification.Degraded is `true`
- Context.Verification.DegradedReason is non-empty
- Other completion fields (Subject, Outcome, RootCause) are still populated correctly

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: K8s fake client (`sigs.k8s.io/controller-runtime/pkg/client/fake`) for notification creation tests
- **Location**: `test/unit/remediationorchestrator/notification_creator_test.go`, `test/unit/effectivenessmonitor/conditions_test.go`
- **Anti-patterns avoided**:
  - No `time.Sleep()` (use `Eventually()` if async needed)
  - No `Skip()` (all tests must run or not exist)
  - No `Expect(x).ToNot(BeNil())` without follow-up business assertion (NULL-TESTING anti-pattern)
  - No direct audit infrastructure testing
  - All assertions validate business outcomes (what the operator sees), not implementation details

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks for internal logic. K8s envtest for controller reconciliation.
- **Infrastructure**: envtest (controller-runtime test environment) for K8s API
- **Location**: `test/unit/remediationorchestrator/controller/reconcile_phases_test.go`, `test/integration/effectivenessmonitor/`

---

## 8. Execution

```bash
# All unit tests
make test

# RO notification creator tests (including #546 scenarios)
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="546"

# EM conditions tests (including #546 scenarios)
go test ./test/unit/effectivenessmonitor/... -ginkgo.focus="546"

# RO reconciler integration tests (including #546 scenarios)
go test ./test/unit/remediationorchestrator/controller/... -ginkgo.focus="546"

# EM integration tests (including #546 scenarios)
go test ./test/integration/effectivenessmonitor/... -ginkgo.focus="546"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan: 15 unit tests + 5 integration tests covering RR condition, EA condition, notification body degradation, VerificationContext, FlattenToMap, getTargetSpec return change, and reconciler condition-setting |
