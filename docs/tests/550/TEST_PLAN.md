# Test Plan: No-Workflow ManualReviewRequired Completion Path (#550)

**Feature**: When the LLM intentionally omits a workflow selection (`has_workflow: False, needs_human_review: True`), the RR should complete with `Outcome=ManualReviewRequired` instead of transitioning to `Failed`.
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/v1.1.0-rc13`

**Authority**:
- BR-ORCH-036: Manual Review & Escalation Notification
- BR-HAPI-197: needs_human_review field
- BR-ORCH-037: Workflow Not Needed (template for Completed terminal path)
- Issue #550: list_available_actions component filter / no-workflow ManualReviewRequired

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../development/business-requirements/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- **AIAnalysisHandler** (`pkg/remediationorchestrator/handler/aianalysis.go`): New `handleManualReviewCompleted` path that transitions RR to `Completed + ManualReviewRequired` when AIAnalysis has `NeedsHumanReview=true` and no `SelectedWorkflow`. Creates ManualReview notification, sets `NextAllowedExecution` cooldown, and does NOT call `transitionToFailed`.
- **Routing preservation**: Infrastructure failures (APIError, Timeout, etc.) and low-confidence-with-workflow failures MUST remain on the existing `Failed + ManualReviewRequired` path via `transitionToFailed`.

### Out of Scope

- **CRD schema changes**: `ManualReviewRequired` outcome already exists; no enum additions needed.
- **AA response processor** (`pkg/aianalysis/handlers/response_processor.go`): No changes to how HAPI responses are classified into AIAnalysis phases.
- **Notification routing** (`pkg/notification/routing/`): No changes to notification delivery routing rules.
- **Gateway deduplication**: `NextAllowedExecution` on terminal-phase RRs is already respected by the Gateway's `ShouldDeduplicate`; no changes needed.

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Change at RO handler level, not AA level | AA faithfully represents what happened (Failed + NeedsHumanReview). RO decides the RR outcome. Minimal blast radius. |
| Reuse `NoActionRequiredDelayHours` (24h) for cooldown | Avoids new config field. Same suppression semantics: prevent repeated RRs for same signal while operator reviews. |
| Split by `ai.Status.SelectedWorkflow == nil` to distinguish paths | Low-confidence WITH a selected workflow remains `Failed` (operator should review the rejected workflow). No-workflow is a valid "nothing to do" outcome. The check is on the AIAnalysis status field, accessible from `handleHumanReviewRequired`. |
| Reuse `CreateManualReviewNotification` for the notification | Notification content (RCA, warnings, humanReviewReason) is identical; only the RR terminal state differs. |
| `handleWorkflowResolutionFailed` path unchanged | This path is only reached when `NeedsHumanReview=false AND Reason=WorkflowResolutionFailed`. In practice, the AA response processor always sets `NeedsHumanReview=true` for `WorkflowResolutionFailed`, so this path is unreachable from standard HAPI flows. Changing it would add risk for zero practical benefit. Documented in UT-RO-550-009 as regression guard. |
| Emit metric `NoActionNeededTotal` with reason `"manual_review"` | Reuses existing metric counter (avoids new metric registration). Dashboards filtering by reason can distinguish `problem_resolved` from `manual_review`. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of the new handler method (`handleManualReviewCompleted`) and the routing changes in `handleHumanReviewRequired`
- **Integration**: >=80% of the full reconciler flow for NeedsHumanReview + no-workflow scenarios (reconciler → handler → status update + notification)

### 2-Tier Minimum

- **Unit tests**: Validate handler routing logic (which path is taken), RR status fields, notification creation, cooldown
- **Integration tests**: Validate the full RO reconciler processes an AIAnalysis CRD with `NeedsHumanReview=true` and produces a `Completed` RR with notification

### Business Outcome Quality Bar

Tests validate that:
1. **Operators see the correct RR phase** (`Completed` not `Failed`) for intentional no-workflow decisions
2. **Notifications are still sent** — operators are informed even though it's not a failure
3. **Cooldown prevents duplicate RRs** — same signal doesn't generate repeated ManualReviewRequired RRs
4. **Infrastructure failures remain as Failed** — genuine failures are not misclassified

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/handler/aianalysis.go` | `handleManualReviewCompleted` (new), `handleHumanReviewRequired` (modified routing) | ~50 new |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/handler/aianalysis.go` | `HandleAIAnalysisStatus` → full handler dispatch | ~20 (routing changes) |
| `internal/controller/remediationorchestrator/reconciler.go` | Analyzing phase handler that delegates to `AIAnalysisHandler` | ~10 (no changes, but validates wiring) |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-036 | No-workflow + NeedsHumanReview → Completed + ManualReviewRequired | P0 | Unit | UT-RO-550-001 | Pending |
| BR-ORCH-036 | ManualReview notification created on Completed path | P0 | Unit | UT-RO-550-002 | Pending |
| BR-ORCH-036 | NextAllowedExecution cooldown set (24h default) | P0 | Unit | UT-RO-550-003 | Pending |
| BR-ORCH-036 | NextAllowedExecution NOT set when delay=0 (opt-out) | P1 | Unit | UT-RO-550-003b | Pending |
| BR-ORCH-036 | CompletedAt timestamp and Message propagated | P0 | Unit | UT-RO-550-004 | Pending |
| BR-ORCH-036 | Ready condition set (terminal success — manual review) | P1 | Unit | UT-RO-550-005 | Pending |
| BR-ORCH-036 | transitionToFailed NOT called (no ConsecutiveFailureCount increment) | P0 | Unit | UT-RO-550-006 | Pending |
| BR-ORCH-036 | Infrastructure failures (APIError) still go to Failed | P0 | Unit | UT-RO-550-007 | Pending |
| BR-ORCH-036 | Low confidence WITH selected workflow still goes to Failed | P0 | Unit | UT-RO-550-008 | Pending |
| BR-ORCH-036 | WorkflowResolutionFailed WITHOUT NeedsHumanReview still goes to Failed | P1 | Unit | UT-RO-550-009 | Pending |
| BR-HAPI-197 | HumanReviewReason and RCA preserved in notification metadata on Completed path | P1 | Unit | UT-RO-550-010 | Pending |
| BR-ORCH-035 | NotificationRequestRefs tracked on RR status on Completed path | P1 | Unit | UT-RO-550-011 | Pending |
| BR-ORCH-044 | NoActionNeededTotal metric recorded with reason="manual_review" | P1 | Unit | UT-RO-550-012 | Pending |
| BR-ORCH-036 | Full reconciler: NeedsHumanReview + no workflow → Completed RR + notification | P0 | Integration | IT-RO-550-001 | Pending |
| BR-ORCH-036 | Full reconciler: NeedsHumanReview + HAS workflow → Failed RR (unchanged) | P0 | Integration | IT-RO-550-002 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-RO-550-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `RO` (RemediationOrchestrator)
- **ISSUE**: `550`

### Tier 1: Unit Tests

**Testable code scope**: `pkg/remediationorchestrator/handler/aianalysis.go` — new `handleManualReviewCompleted` method and modified routing in `handleHumanReviewRequired`. Target: >=80% of new/modified code.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-550-001` | Operator sees RR phase=Completed, outcome=ManualReviewRequired when LLM intentionally omits workflow | Pending |
| `UT-RO-550-002` | Operator receives ManualReview notification even though RR is Completed (not silently closed) | Pending |
| `UT-RO-550-003` | Gateway suppresses duplicate RRs for 24h via NextAllowedExecution cooldown | Pending |
| `UT-RO-550-003b` | NextAllowedExecution NOT set when delay=0 (explicit opt-out honored) | Pending |
| `UT-RO-550-004` | CompletedAt timestamp and Message propagated from AI for audit trail and SLA tracking | Pending |
| `UT-RO-550-005` | Ready condition reflects terminal success (manual review recommended, not failure) | Pending |
| `UT-RO-550-006` | ConsecutiveFailureCount is NOT incremented (transitionToFailed not called) — prevents false escalation | Pending |
| `UT-RO-550-007` | Infrastructure failures (APIError/MaxRetriesExceeded) still correctly transition to Failed — no regression | Pending |
| `UT-RO-550-008` | Low-confidence rejection WITH a selected workflow still correctly transitions to Failed — no regression | Pending |
| `UT-RO-550-009` | WorkflowResolutionFailed WITHOUT NeedsHumanReview still correctly transitions to Failed — no regression | Pending |
| `UT-RO-550-010` | Notification metadata includes humanReviewReason and RCA on the new Completed path | Pending |
| `UT-RO-550-011` | NotificationRequestRefs tracked on RR status (BR-ORCH-035 ref tracking) | Pending |
| `UT-RO-550-012` | NoActionNeededTotal metric incremented with reason="manual_review" | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Full RO reconciler flow (reconciler → AIAnalysis handler → RR status update + NotificationRequest creation). Target: >=80% of the handler wiring path.

**Note**: Existing integration tests IT-RO-197-001 and IT-RO-197-003 will be updated to expect `PhaseCompleted` (see Section 10). IT-RO-550-001 and IT-RO-550-002 provide additional coverage for the new routing split.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-RO-550-001` | End-to-end: AIAnalysis Failed + NeedsHumanReview + no SelectedWorkflow → RR Completed + ManualReviewRequired + NotificationRequest created + NextAllowedExecution set | Pending |
| `IT-RO-550-002` | End-to-end: AIAnalysis Failed + NeedsHumanReview + HAS SelectedWorkflow (low confidence) → RR Failed + ManualReviewRequired (unchanged behavior) | Pending |

### Tier Skip Rationale

- **E2E**: Deferred — the RO handler change is fully testable at unit and integration tiers. E2E would require a full Kind cluster with HAPI mock returning `needs_human_review: true` with no `selected_workflow`, which adds significant infrastructure cost for marginal additional coverage. Can be added to the full pipeline E2E suite later.

---

## 6. Test Cases (Detail)

### UT-RO-550-001: No-workflow + NeedsHumanReview → Completed + ManualReviewRequired

**BR**: BR-ORCH-036, Issue #550
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Given**: An AIAnalysis with `Phase=Failed`, `NeedsHumanReview=true`, `HumanReviewReason="no_matching_workflows"`, `SelectedWorkflow=nil`
**When**: `HandleAIAnalysisStatus` is called
**Then**: RR status is `OverallPhase=Completed`, `Outcome="ManualReviewRequired"`, `RequiresManualReview=true`

**Acceptance Criteria**:
- Behavior: RR phase is `Completed` (not `Failed`) — the LLM made a valid decision
- Correctness: `Outcome` is exactly `"ManualReviewRequired"` (existing CRD enum value)
- Accuracy: `RequiresManualReview` is `true` (operator can filter on this field)

---

### UT-RO-550-002: ManualReview notification created on Completed path

**BR**: BR-ORCH-036, Issue #550
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Given**: An AIAnalysis with `Phase=Failed`, `NeedsHumanReview=true`, `SelectedWorkflow=nil`
**When**: `HandleAIAnalysisStatus` is called
**Then**: A `NotificationRequest` of `type=manual-review` is created with the correct name pattern

**Acceptance Criteria**:
- Behavior: Notification is created even though RR is Completed (operator must still be informed)
- Correctness: NotificationRequest name follows `nr-manual-review-{rr-name}` convention
- Accuracy: NotificationRequest type is `NotificationTypeManualReview`

---

### UT-RO-550-003: NextAllowedExecution cooldown set (24h default)

**BR**: BR-ORCH-036, Issue #550, Issue #314
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Given**: An AIAnalysis with `Phase=Failed`, `NeedsHumanReview=true`, `SelectedWorkflow=nil`; handler configured with `noActionRequiredDelay=24h`
**When**: `HandleAIAnalysisStatus` is called
**Then**: RR `NextAllowedExecution` is set to approximately `now + 24h`

**Acceptance Criteria**:
- Behavior: Gateway will suppress duplicate RR creation for the same signal fingerprint for 24h
- Correctness: `NextAllowedExecution` is approximately `now + 24h` (within 1 minute tolerance)

---

### UT-RO-550-003b: NextAllowedExecution NOT set when delay=0 (opt-out)

**BR**: BR-ORCH-036, Issue #550, Issue #314
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Given**: An AIAnalysis with `Phase=Failed`, `NeedsHumanReview=true`, `SelectedWorkflow=nil`; handler configured with `noActionRequiredDelay=0`
**When**: `HandleAIAnalysisStatus` is called
**Then**: RR `NextAllowedExecution` is `nil` and `OverallPhase` is still `Completed`

**Acceptance Criteria**:
- Behavior: Operator can opt out of suppression by setting delay to 0
- Correctness: `NextAllowedExecution` is `nil`
- Accuracy: All other Completed-path fields are still set (Outcome, CompletedAt, RequiresManualReview)

---

### UT-RO-550-004: CompletedAt timestamp and Message propagated

**BR**: BR-ORCH-036, Issue #550
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Given**: An AIAnalysis with `Phase=Failed`, `NeedsHumanReview=true`, `SelectedWorkflow=nil`, `Message="Orphaned PVCs detected - operator should review"`
**When**: `HandleAIAnalysisStatus` is called
**Then**: RR `CompletedAt` is set to approximately the current time, and `Message` is propagated from AI

**Acceptance Criteria**:
- Behavior: CompletedAt enables SLA tracking and audit trail
- Correctness: Timestamp is within 5 seconds of `time.Now()`
- Accuracy: `rr.Status.Message` matches `ai.Status.Message` (operator sees the AI's assessment)

---

### UT-RO-550-005: Ready condition set (terminal success)

**BR**: BR-ORCH-043, Issue #550
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Given**: An AIAnalysis with `Phase=Failed`, `NeedsHumanReview=true`, `SelectedWorkflow=nil`
**When**: `HandleAIAnalysisStatus` is called
**Then**: RR Ready condition is set to `True` with reason indicating manual review

**Acceptance Criteria**:
- Behavior: Ready=True signals terminal completion (not a stuck/failed state)
- Correctness: Condition reason communicates "manual review recommended"

---

### UT-RO-550-006: transitionToFailed NOT called

**BR**: BR-ORCH-036, Issue #550
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Given**: An AIAnalysis with `Phase=Failed`, `NeedsHumanReview=true`, `SelectedWorkflow=nil`
**When**: `HandleAIAnalysisStatus` is called
**Then**: The `transitionToFailed` callback is NOT invoked (0 calls)

**Acceptance Criteria**:
- Behavior: `ConsecutiveFailureCount` is not incremented — this is not a failure
- Correctness: `transitionFailedCalls` counter remains at 0
- Accuracy: No `FailurePhase` or `FailureReason` fields set on RR status

---

### UT-RO-550-007: Infrastructure failures still go to Failed (regression guard)

**BR**: BR-ORCH-036 v3.0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Given**: An AIAnalysis with `Phase=Failed`, `Reason=APIError`, `SubReason=MaxRetriesExceeded`, `NeedsHumanReview=false`
**When**: `HandleAIAnalysisStatus` is called
**Then**: RR status is `OverallPhase=Failed` and `transitionToFailed` IS called

**Acceptance Criteria**:
- Behavior: Infrastructure failures are NOT reclassified as Completed
- Correctness: `transitionFailedCalls` counter is 1
- Note: This test already exists (AC-036-34) — included here as explicit regression guard

---

### UT-RO-550-008: Low confidence WITH workflow still goes to Failed (regression guard)

**BR**: BR-HAPI-197
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Given**: An AIAnalysis with `Phase=Failed`, `NeedsHumanReview=true`, `HumanReviewReason=low_confidence`, `SelectedWorkflow=&SelectedWorkflow{WorkflowID: "restart-pod-v1", Confidence: 0.55}` (non-nil — has a rejected workflow)
**When**: `HandleAIAnalysisStatus` is called
**Then**: RR status is `OverallPhase=Failed` and `transitionToFailed` IS called

**Acceptance Criteria**:
- Behavior: Low-confidence rejections WITH a selected workflow remain failures (operator should review the rejected workflow)
- Correctness: The new Completed path is NOT taken when `ai.Status.SelectedWorkflow` is non-nil
- Accuracy: `FailurePhase` is `"ai_analysis"`

---

### UT-RO-550-009: WorkflowResolutionFailed without NeedsHumanReview still goes to Failed

**BR**: BR-ORCH-036
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Given**: An AIAnalysis with `Phase=Failed`, `Reason=WorkflowResolutionFailed`, `NeedsHumanReview=false`, `SelectedWorkflow=nil`
**When**: `HandleAIAnalysisStatus` is called
**Then**: RR status is `OverallPhase=Failed` (goes through `handleWorkflowResolutionFailed`, not the new Completed path)

**Acceptance Criteria**:
- Behavior: The Completed path is gated on `NeedsHumanReview=true` — without the flag, existing behavior preserved
- Correctness: `transitionFailedCalls` counter is 1

---

### UT-RO-550-010: Notification metadata includes humanReviewReason on Completed path

**BR**: BR-HAPI-197, Issue #550
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Given**: An AIAnalysis with `Phase=Failed`, `NeedsHumanReview=true`, `HumanReviewReason=no_matching_workflows`, `SelectedWorkflow=nil`, `RootCause="Orphaned PVCs detected"`
**When**: `HandleAIAnalysisStatus` is called
**Then**: Created NotificationRequest metadata includes `humanReviewReason` and `rootCauseAnalysis`

**Acceptance Criteria**:
- Behavior: Operator receives full context in the notification even on the Completed path
- Correctness: `metadata["humanReviewReason"]` is `"no_matching_workflows"`
- Accuracy: `metadata["rootCauseAnalysis"]` is `"Orphaned PVCs detected"`

---

### UT-RO-550-011: NotificationRequestRefs tracked on RR status

**BR**: BR-ORCH-035, Issue #550
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Given**: An AIAnalysis with `Phase=Failed`, `NeedsHumanReview=true`, `SelectedWorkflow=nil`
**When**: `HandleAIAnalysisStatus` is called
**Then**: RR `NotificationRequestRefs` contains a reference to the created `nr-manual-review-{rr-name}` NotificationRequest

**Acceptance Criteria**:
- Behavior: RR tracks all child NotificationRequests for cascade deletion and audit
- Correctness: At least one ref has `Name` matching `nr-manual-review-{rr-name}`
- Accuracy: Ref `Kind` is `NotificationRequest`

---

### UT-RO-550-012: NoActionNeededTotal metric recorded

**BR**: BR-ORCH-044, Issue #550
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Given**: An AIAnalysis with `Phase=Failed`, `NeedsHumanReview=true`, `SelectedWorkflow=nil`; handler with non-nil Metrics
**When**: `HandleAIAnalysisStatus` is called
**Then**: `NoActionNeededTotal` counter is incremented with `reason="manual_review"`

**Acceptance Criteria**:
- Behavior: Dashboards can track manual review completions separately from problem-resolved completions
- Correctness: Counter value increases by 1 after the call
- Accuracy: Label `reason` is `"manual_review"` (not `"problem_resolved"`)

---

### IT-RO-550-001: Full reconciler — NeedsHumanReview + no workflow → Completed

**BR**: BR-ORCH-036, Issue #550
**Type**: Integration
**File**: `test/integration/remediationorchestrator/needs_human_review_integration_test.go`

**Given**: An RR in `Analyzing` phase with a child AIAnalysis CRD that has `Phase=Failed`, `NeedsHumanReview=true`, `SelectedWorkflow=nil`
**When**: The RO reconciler processes the RR
**Then**: RR transitions to `Completed` with `Outcome=ManualReviewRequired`, a `NotificationRequest` is created, and `NextAllowedExecution` is set

**Acceptance Criteria**:
- Behavior: Full reconciler flow produces `Completed` RR (not `Failed`)
- Correctness: NotificationRequest CRD exists in the namespace
- Accuracy: `NextAllowedExecution` is in the future (cooldown active)

---

### IT-RO-550-002: Full reconciler — NeedsHumanReview + HAS workflow → Failed (regression)

**BR**: BR-HAPI-197
**Type**: Integration
**File**: `test/integration/remediationorchestrator/needs_human_review_integration_test.go`

**Given**: An RR in `Analyzing` phase with a child AIAnalysis CRD that has `Phase=Failed`, `NeedsHumanReview=true`, `HumanReviewReason=low_confidence`, `SelectedWorkflow=&SelectedWorkflow{WorkflowID: "restart-pod-v1", Confidence: 0.55}` (non-nil)
**When**: The RO reconciler processes the RR
**Then**: RR transitions to `Failed` with `Outcome=ManualReviewRequired` (unchanged behavior)

**Acceptance Criteria**:
- Behavior: Low-confidence with workflow is still a failure in the full reconciler
- Correctness: `OverallPhase=Failed`, `FailurePhase` is set
- Accuracy: The presence of `SelectedWorkflow` on the AIAnalysis causes the Failed path, not the new Completed path

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `fake.NewClientBuilder()` for K8s client; `mockTransitionFailed` callback to track invocations (existing pattern in test file)
- **Location**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (envtest with real K8s API server)
- **Infrastructure**: envtest (real K8s API via controller-runtime test framework)
- **Location**: `test/integration/remediationorchestrator/needs_human_review_integration_test.go`

---

## 8. Execution

```bash
# Unit tests (all RO unit tests)
go test ./test/unit/remediationorchestrator/... -ginkgo.v

# Specific test by ID
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-550"

# Integration tests
go test ./test/integration/remediationorchestrator/... -ginkgo.focus="IT-RO-550"
```

---

## 9. Risk Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| **R1: Existing unit tests break** | UT-RO-197-004 asserts `OverallPhase=Failed` for `NeedsHumanReview=true` + no `SelectedWorkflow` | Certain | Update test to expect `Completed`. See Section 10 for full inventory. |
| **R2: Existing integration tests break** | IT-RO-197-001, IT-RO-197-002, IT-RO-197-003 all assert `PhaseFailed` for `NeedsHumanReview=true` with no `SelectedWorkflow` | Certain | Update all 3 integration tests to expect `PhaseCompleted`. See Section 10. |
| **R3: Audit trail gap** | `transitionToFailed` emits DD-AUDIT-003 phase-transition audit. New Completed path skips this. | High | `handleManualReviewCompleted` must emit an equivalent audit event via the existing audit recording pattern (e.g., `Recorder.Eventf` K8s event). Validated by UT-RO-550-005 (Ready condition = terminal state observable by audit). |
| **R4: Metric gap** | `NoActionNeededTotal` is only recorded in `handleWorkflowNotNeeded`. New path would have no metric. | High | Record `NoActionNeededTotal` with `reason="manual_review"` in `handleManualReviewCompleted`. Validated by UT-RO-550-012. |
| **R5: NotificationRequestRefs not tracked** | If the new path doesn't call `buildNotificationRef` + update `NotificationRequestRefs`, cascade deletion (BR-ORCH-031) breaks. | Medium | Explicitly track notification ref in `handleManualReviewCompleted`. Validated by UT-RO-550-011. |
| **R6: Gateway dedup** | `ShouldDeduplicate` must respect `NextAllowedExecution` on `Completed` phase. | Low | No change needed — `Completed` is already a terminal phase. Gateway dedup code checks terminal phases + `NextAllowedExecution` generically. |
| **R7: UT-RO-197-005 silent behavior change** | This test iterates all 8 `HumanReviewReason` values with `NeedsHumanReview=true` and no `SelectedWorkflow`. After the change, all 8 take the new Completed path. Test only checks notification creation (still passes), but RR status changes silently. | Medium | Add a comment to UT-RO-197-005 noting the #550 behavior change. No assertion update needed (test validates notification, not phase). |
| **R8: `handleWorkflowResolutionFailed` dead path** | When `NeedsHumanReview=false AND Reason=WorkflowResolutionFailed AND SelectedWorkflow=nil`, this path still goes to `Failed`. In practice, the AA response processor always sets `NeedsHumanReview=true` for `WorkflowResolutionFailed`, making this path unreachable from standard HAPI flows. | Low | UT-RO-550-009 guards this as a regression test. Document in code comment that this path is for non-standard AA inputs. |

---

## 10. Existing Tests Requiring Updates

### Unit Tests

| Test ID / Line | File | Current Assertion | Required Change | Reason |
|----------------|------|-------------------|-----------------|--------|
| **UT-RO-197-004** (line 669) | `aianalysis_handler_test.go` | `OverallPhase=Failed`, `FailurePhase="ai_analysis"` | Change to `OverallPhase=Completed`, remove `FailurePhase` assertion, add `CompletedAt` check | `NeedsHumanReview=true` + `SelectedWorkflow=nil` → Completed path |
| **UT-RO-197-001** (line 586) | `aianalysis_handler_test.go` | Uses `createMockTransitionFailed(client)` | Change to default `mockTransitionFailed` (no-op) since `transitionToFailed` is no longer called. Test checks notification creation — still valid. | New path doesn't call `transitionToFailed` |
| **UT-RO-197-003** (line 639) | `aianalysis_handler_test.go` | Uses `createMockTransitionFailed(client)` | Same as UT-RO-197-001 — switch to no-op mock. Test checks metadata — still valid. | New path doesn't call `transitionToFailed` |
| **UT-RO-197-005** (line 696) | `aianalysis_handler_test.go` | Uses `createMockTransitionFailed(client)` | Switch to no-op mock. Add comment noting all 8 reasons now take Completed path (#550). Test checks notification creation — still valid. | New path doesn't call `transitionToFailed` |
| **UT-RO-197-006** (line 739) | `aianalysis_handler_test.go` | Uses default `mockTransitionFailed` (no-op) | No change needed | Test only checks notification metadata |
| **Test #16** (line 319) | `aianalysis_handler_test.go` | `OverallPhase=Failed`, `FailurePhase="ai_analysis"` | **No change** — this test uses `Reason=WorkflowResolutionFailed` without `NeedsHumanReview=true`, routes through `handleWorkflowResolutionFailed` (not `handleHumanReviewRequired`) | Different routing path, `NeedsHumanReview=false` |

### Integration Tests

| Test ID | File | Current Assertion | Required Change | Reason |
|---------|------|-------------------|-----------------|--------|
| **IT-RO-197-001** (line 66) | `needs_human_review_integration_test.go` | `Should(Equal(remediationv1.PhaseFailed))` (lines 129, 136) | Change to `PhaseCompleted`. Add assertions for `CompletedAt`, `NextAllowedExecution`. Keep `Outcome=ManualReviewRequired` and `RequiresManualReview=true` assertions. | `NeedsHumanReview=true` + `SelectedWorkflow=nil` → Completed |
| **IT-RO-197-002** (line 158) | `needs_human_review_integration_test.go` | Expects `PhaseFailed` implicitly (checks NO WorkflowExecution created) | No phase assertion to update (test doesn't assert phase). Add comment: after #550, RR is Completed not Failed. WE still not created (correct). | Test validates absence of WE, not RR phase |
| **IT-RO-197-003** (line 230) | `needs_human_review_integration_test.go` | `Should(Equal(remediationv1.PhaseFailed))` (line 295) | Change to `PhaseCompleted`. Keep `Outcome=ManualReviewRequired` and `RequiresManualReview=true`. | `NeedsHumanReview=true` + `SelectedWorkflow=nil` → Completed |

### Tests NOT Requiring Updates (Verified Safe)

| Test ID / Line | File | Why Safe |
|----------------|------|----------|
| Test #15 (line 293) | `aianalysis_handler_test.go` | `WorkflowResolutionFailed` without `NeedsHumanReview` → `handleWorkflowResolutionFailed` path, unchanged |
| Test #17 (line 346) | `aianalysis_handler_test.go` | Same as Test #15, checks RCA in notification metadata |
| AC-036-30..34 (lines 431-581) | `aianalysis_handler_test.go` | Infrastructure failures (APIError) → `propagateFailure` path, `NeedsHumanReview=false`, unchanged |
| UT-RO-197-002 (line 612) | `aianalysis_handler_test.go` | `NeedsHumanReview=false` on normal completion — no overlap with new path |

---

## 11. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
| 1.1 | 2026-03-04 | Mitigations: Added UT-RO-550-003b (delay=0 opt-out), UT-RO-550-011 (NotificationRequestRefs), UT-RO-550-012 (metric). Expanded Section 9 with 8 risks + probabilities. Expanded Section 10 with full inventory of 6 unit tests + 3 integration tests requiring updates, plus 4 verified-safe tests. Fixed UT-RO-550-004 to include Message propagation. Fixed UT-RO-550-008 Given clause to explicitly specify non-nil SelectedWorkflow. Added design decision for `handleWorkflowResolutionFailed` path (unchanged, documented as unreachable from standard HAPI flows). Added design decision for metric reuse (`NoActionNeededTotal` with `reason="manual_review"`). |
