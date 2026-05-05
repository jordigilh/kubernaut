# Test Plan: Audit Events Data Quality — Outcome Vocabulary + Workflow Name

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1033-v1
**Feature**: Fix audit event data quality gaps: crd_outcome vocabulary mismatch and missing workflow_name
**Version**: 1.0
**Created**: 2026-05-05
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/1033-audit-data-quality`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the fixes for Issue #1033, which identifies two data quality gaps in audit events that prevent accurate reconstruction of golden transcripts from the database after CR cleanup.

Gap 1: `orchestrator.lifecycle.completed` audit events always store `crd_outcome: "success"` instead of the actual CRD outcome value (`Remediated`, `VerificationTimedOut`, `Inconclusive`, etc.), making post-cleanup transcript reconstruction ambiguous.

Gap 2: `workflowexecution.selection.completed` audit events omit the human-readable workflow name (e.g., `fix-security-context-job`), forcing fragile heuristics to parse `container_image` paths after WFE CR cleanup.

### 1.2 Objectives

1. **Gap 1 — Outcome Vocabulary**: All 3 `EmitCompletionAudit` call sites in `VerifyingHandler` pass `rr.Status.Outcome` instead of the literal `"success"`, ensuring `crd_outcome` in audit events matches the RR CR status.
2. **Gap 2 — Workflow Name**: The `workflowexecution.selection.completed` audit event includes `workflow_name` when the DS catalog provides it, and gracefully omits it when unavailable.
3. **Backward Compatibility**: Existing audit event consumers (DS query API, `eval_report.py`, `capture-eval.sh`) continue to function — the `outcome` (OpenAPI-level) field remains `"Success"` for completion events.
4. **Per-Tier Coverage**: >=80% of unit-testable code and >=80% of integration-testable code for the changed files.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/... ./test/unit/workflowexecution/...` |
| Integration test pass rate | 100% | `go test ./test/integration/remediationorchestrator/... ./test/integration/workflowexecution/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on changed unit-testable files |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on changed integration-testable files |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-AUDIT-005**: Audit trail completeness for SOC2 compliance — workflow selection, lifecycle events
- **BR-ORCH-045**: Orchestrator verification phase — notification and audit emission on phase transitions
- **BR-WE-013**: Audit-tracked workflow execution
- **DD-AUDIT-003**: Orchestrator lifecycle.completed event specification (P1)
- **DD-AUDIT-CORRELATION-001**: WorkflowExecution correlation ID policy
- **Issue #1033**: Audit events: outcome field and workflow name gaps degrade DB-based transcript capture

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Audit Infrastructure Anti-Pattern](../../handoff/AUDIT_INFRASTRUCTURE_TESTING_ANTI_PATTERN_TRIAGE_DEC_26_2025.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `rr.Status.Outcome` empty at audit emission time | `crd_outcome` omitted (OptString Set=false) — graceful degradation but loses data | Medium | UT-RO-1033-003, UT-RO-1033-006 | Test empty outcome path; `BuildCompletionEvent` already handles empty via `Set: outcome != ""` |
| R2 | `schemaMeta` nil when audit fires | `workflow_name` omitted — graceful degradation | Medium | UT-WE-1033-004, UT-WE-1033-005 | Guard with nil check before passing to audit; test both nil and non-nil paths |
| R3 | Downstream consumers parse `crd_outcome` value with old expectations | Scripts expecting `"success"` break when they receive `"VerificationTimedOut"` | Low | Manual verification | The issue explicitly requests this fix; `crd_outcome` was always documented to carry CRD vocabulary per Issue #722 |
| R4 | Ogen regeneration introduces unrelated schema drift | Large diff, merge conflicts | Low | Build validation | Pin ogen version (v1.18.0); diff review limited to `WorkflowExecutionAuditPayload` |
| R5 | Breaking existing integration tests that assert audit event shapes | Test failures in existing WFE audit integration tests | Medium | IT-WE-1033-001 | New `workflow_name` field is optional — existing assertions on `WorkflowID`, `WorkflowVersion` etc. remain valid |
| R6 | Concurrency: VerifyingHandler accessed from parallel reconciles | Data race if outcome read/write is not synchronized | Low | UT-RO-1033-007 | K8s controller-runtime defaults to MaxConcurrentReconciles=1; status update uses optimistic concurrency (resourceVersion) |

### 3.1 Risk-to-Test Traceability

- **R1 (empty outcome)**: UT-RO-1033-003 (empty outcome → crd_outcome omitted), UT-RO-1033-006 (nil/zero edge)
- **R2 (nil schemaMeta)**: UT-WE-1033-004 (nil schema → workflow_name omitted), UT-WE-1033-005 (empty name)
- **R3 (downstream)**: Manual verification + backward compat assertion in UT-RO-1033-001 (OpenAPI outcome field unchanged)
- **R5 (integration shape)**: IT-WE-1033-001 (optional field does not break existing shape assertions)

---

## 4. Scope

### 4.1 Features to be Tested

- **VerifyingHandler completion audit** (`internal/controller/remediationorchestrator/verifying_handler.go`): 3 `EmitCompletionAudit` call sites pass `rr.Status.Outcome` instead of `"success"`
- **BuildCompletionEvent** (`pkg/remediationorchestrator/audit/manager.go`): Existing `crd_outcome` mapping works correctly with all RR outcome values
- **RecordWorkflowSelectionCompleted** (`pkg/workflowexecution/audit/manager.go`): New `workflowName` parameter populates `workflow_name` in the audit payload
- **WorkflowExecutionAuditPayload** (`api/openapi/data-storage-v1.yaml`): New optional `workflow_name` field
- **Controller call site** (`internal/controller/workflowexecution/workflowexecution_controller.go`): Passes `schemaMeta.WorkflowName` to audit manager

### 4.2 Features Not to be Tested

- **DataStorage persistence/query of workflow_name**: DS service responsibility — tested by DS's own test suite
- **Audit buffered store / batching**: Audit infrastructure — per anti-pattern policy
- **E2E full pipeline with new fields**: Deferred to E2E re-validation after RC6 deploy
- **`recordAuditEvent` / `recordFailureAuditWithDetails`**: Workflow name propagation to started/completed/failed events is a REFACTOR-phase enhancement, tested if time permits

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Test `EmitCompletionAudit` outcome via callback capture, not by inspecting audit store | Follows correct pattern: test business logic (handler behavior), not audit infrastructure |
| `workflow_name` is an optional field in OpenAPI | Backward compatible; DS catalog may not always provide a name |
| Test all 5 possible `rr.Status.Outcome` values in unit tests | Exhaustive coverage per defense-in-depth mandate |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code in `pkg/remediationorchestrator/audit/manager.go` and `pkg/workflowexecution/audit/manager.go`
- **Integration**: >=80% of integration-testable code in `internal/controller/remediationorchestrator/verifying_handler.go` and `internal/controller/workflowexecution/workflowexecution_controller.go`
- **E2E**: Deferred — existing E2E suite validates audit event shapes; new optional field is additive

### 5.2 Two-Tier Minimum

Every business requirement covered by at least UT + IT:
- **Gap 1**: UT validates callback argument; IT validates end-to-end reconciler path
- **Gap 2**: UT validates payload construction; IT validates real controller + audit store integration

### 5.3 Business Outcome Quality Bar

Tests validate business outcomes:
- "When verification times out, the audit event records the CRD outcome VerificationTimedOut (not success) so transcript reconstruction is accurate"
- "When a workflow is selected, the audit event includes the workflow catalog name so operators can identify it after CR cleanup"

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage meets >=80% threshold
4. No regressions in existing test suites (RO, WE)
5. `crd_outcome` in completion audit events matches `rr.Status.Outcome` for all 5 possible values
6. `workflow_name` appears in selection audit events when DS provides it

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier
3. Existing RO or WE tests regress

### 5.5 Suspension & Resumption Criteria

**Suspend**: Ogen regeneration breaks unrelated schema types; build does not compile.
**Resume**: Schema issue resolved; build green.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/audit/manager.go` | `BuildCompletionEvent` | ~40 |
| `pkg/workflowexecution/audit/manager.go` | `RecordWorkflowSelectionCompleted` (payload construction only) | ~70 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/verifying_handler.go` | `Handle` (3 EmitCompletionAudit call sites) | ~65 |
| `internal/controller/workflowexecution/workflowexecution_controller.go` | `reconcilePending` (audit emission with schemaMeta) | ~120 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/1033-audit-data-quality` branch from `main` | Post-PR #1035 merge |
| Ogen | v1.18.0 | Pinned in `pkg/datastorage/ogen-client/gen.go` and Makefile |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AUDIT-005 | Audit trail completeness — crd_outcome reflects CRD status | P0 | Unit | UT-RO-1033-001 | Pending |
| BR-AUDIT-005 | Audit trail completeness — crd_outcome for VerificationTimedOut | P0 | Unit | UT-RO-1033-002 | Pending |
| BR-AUDIT-005 | Audit trail completeness — crd_outcome empty/missing outcome | P1 | Unit | UT-RO-1033-003 | Pending |
| BR-AUDIT-005 | Audit trail completeness — all 5 outcome values | P0 | Unit | UT-RO-1033-004 | Pending |
| BR-AUDIT-005 | Audit trail completeness — OpenAPI outcome field unchanged (backward compat) | P1 | Unit | UT-RO-1033-005 | Pending |
| BR-AUDIT-005 | Nil/zero edge — empty outcome at emission time | P1 | Unit | UT-RO-1033-006 | Pending |
| BR-AUDIT-005 | Concurrency — parallel reconcile does not race on outcome | P1 | Unit | UT-RO-1033-007 | Pending |
| BR-AUDIT-005 | Adversarial — outcome string with injection characters | P2 | Unit | UT-RO-1033-008 | Pending |
| BR-AUDIT-005 | Workflow name in selection audit — present when DS provides it | P0 | Unit | UT-WE-1033-001 | Pending |
| BR-AUDIT-005 | Workflow name in selection audit — omitted when DS unavailable | P0 | Unit | UT-WE-1033-002 | Pending |
| BR-AUDIT-005 | Workflow name in selection audit — empty string from DS | P1 | Unit | UT-WE-1033-003 | Pending |
| BR-AUDIT-005 | Workflow name — schemaMeta nil (querier unavailable) | P1 | Unit | UT-WE-1033-004 | Pending |
| BR-AUDIT-005 | Workflow name — schemaMeta non-nil but WorkflowName empty | P1 | Unit | UT-WE-1033-005 | Pending |
| BR-AUDIT-005 | Adversarial — workflow name with max-length+1, path traversal, Unicode | P2 | Unit | UT-WE-1033-006 | Pending |
| BR-AUDIT-005 | Cross-phase integration — VerifyingHandler outcome reaches BuildCompletionEvent crd_outcome | P0 | Integration | IT-RO-1033-001 | Pending |
| BR-WE-013 | Cross-phase integration — controller passes schemaMeta.WorkflowName to audit event | P0 | Integration | IT-WE-1033-001 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `RO` (RemediationOrchestrator), `WE` (WorkflowExecution)
- **BR_NUMBER**: 1033

### Tier 1: Unit Tests

**Gap 1: Outcome Vocabulary (RO)**

**File**: `test/unit/remediationorchestrator/controller/verifying_handler_outcome_test.go`

| ID | Business Outcome Under Test | Phase |
|----|---------------------------|-------|
| `UT-RO-1033-001` | EA terminal completion → `EmitCompletionAudit` receives `rr.Status.Outcome` (e.g. "Remediated"), not "success" | Pending |
| `UT-RO-1033-002` | Safety-net timeout → `EmitCompletionAudit` receives "VerificationTimedOut" | Pending |
| `UT-RO-1033-003` | Verification deadline expired → `EmitCompletionAudit` receives "VerificationTimedOut" | Pending |
| `UT-RO-1033-004` | Table-driven: all 5 outcome values (Remediated, Inconclusive, VerificationTimedOut, DryRun, ManualReviewRequired) produce correct crd_outcome in `BuildCompletionEvent` | Pending |
| `UT-RO-1033-005` | Backward compatibility: OpenAPI-level `outcome` field is always `Success` for completion events regardless of crd_outcome | Pending |
| `UT-RO-1033-006` | Nil/zero edge: `rr.Status.Outcome` is empty string → `crd_outcome` is unset (OptString.Set=false) | Pending |
| `UT-RO-1033-007` | Concurrency: 10 goroutines calling `BuildCompletionEvent` with different outcomes under `-race` → no data race | Pending |
| `UT-RO-1033-008` | Adversarial: outcome = `""`, `strings.Repeat("x", 1024)`, `"../../etc/passwd"`, `"\u0000\uffff"` → no panic, correct crd_outcome set/unset behavior | Pending |

**Gap 2: Workflow Name (WE)**

**File**: `test/unit/workflowexecution/audit/selection_workflow_name_test.go`

| ID | Business Outcome Under Test | Phase |
|----|---------------------------|-------|
| `UT-WE-1033-001` | Workflow name provided → audit payload includes `workflow_name` with correct value | Pending |
| `UT-WE-1033-002` | Workflow name empty string → audit payload omits `workflow_name` (OptString.Set=false) | Pending |
| `UT-WE-1033-003` | Table-driven: various workflow names (short, max-length, Unicode, hyphenated) → all correctly serialized | Pending |
| `UT-WE-1033-004` | schemaMeta nil → controller passes empty workflow name → audit payload omits field | Pending |
| `UT-WE-1033-005` | schemaMeta non-nil, WorkflowName empty → audit payload omits field | Pending |
| `UT-WE-1033-006` | Adversarial: name = `""`, `strings.Repeat("a", 256)`, `"../../etc/passwd"`, `"fix-sec\u0000ctx-job"`, `"名前"` → no panic, field set only when non-empty | Pending |

### Tier 2: Integration Tests

**File (RO)**: `test/integration/remediationorchestrator/audit_crd_outcome_integration_test.go` (or extend existing)

| ID | Business Outcome Under Test | Phase |
|----|---------------------------|-------|
| `IT-RO-1033-001` | Full reconciler path: RR in Verifying phase with expired deadline → audit event stored in DS contains `crd_outcome: "VerificationTimedOut"` | Pending |

**File (WE)**: `test/integration/workflowexecution/audit_workflow_name_integration_test.go` (or extend `audit_workflow_refs_integration_test.go`)

| ID | Business Outcome Under Test | Phase |
|----|---------------------------|-------|
| `IT-WE-1033-001` | WFE reconciled with DS-populated workflow name → stored audit event contains `workflow_name` | Pending |

### Tier Skip Rationale

- **E2E**: Deferred to post-RC6 validation. The changes are additive (optional field) and backward-compatible. Existing E2E audit tests validate event shape and counts, which remain unchanged.

---

## 9. Test Cases

### UT-RO-1033-001: EA terminal completion passes rr.Status.Outcome to EmitCompletionAudit

**BR**: BR-AUDIT-005
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/verifying_handler_outcome_test.go`

**Preconditions**:
- RR in Verifying phase with `Status.Outcome = "Remediated"`
- EA exists, phase = Completed (terminal)
- `Status.StartTime` is set

**Test Steps**:
1. **Given**: RR with `Outcome = "Remediated"`, EA completed, `OverallPhase` transitions to Completed via `TrackEffectivenessStatus`
2. **When**: `VerifyingHandler.Handle` is called
3. **Then**: The `EmitCompletionAudit` callback is invoked with `outcome = "Remediated"`

**Expected Results**:
1. Callback captures `outcome == "Remediated"` (not `"success"`)
2. `rr.Status.OverallPhase == Completed`

**Acceptance Criteria**:
- **Behavior**: The audit outcome argument matches the CRD status outcome
- **Correctness**: The literal `"success"` is never passed for the EA terminal path
- **Accuracy**: `crd_outcome` in the resulting audit event would be `"Remediated"`

### UT-RO-1033-002: Safety-net timeout passes VerificationTimedOut

**BR**: BR-AUDIT-005
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/verifying_handler_outcome_test.go`

**Preconditions**:
- RR in Verifying phase
- EA exists with `ValidityDeadline` in the past
- `Status.StartTime` is set

**Test Steps**:
1. **Given**: RR with EA whose `ValidityDeadline` has expired
2. **When**: `VerifyingHandler.Handle` is called
3. **Then**: RR outcome transitions to `"VerificationTimedOut"` and `EmitCompletionAudit` receives `"VerificationTimedOut"`

**Expected Results**:
1. Callback captures `outcome == "VerificationTimedOut"`
2. `rr.Status.Outcome == "VerificationTimedOut"`

### UT-WE-1033-001: Workflow name present in selection audit

**BR**: BR-AUDIT-005
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/audit/selection_workflow_name_test.go`

**Preconditions**:
- `AuditStore` mock available
- Valid WFE object

**Test Steps**:
1. **Given**: A WFE spec with `WorkflowRef.WorkflowID = "wf-123"`
2. **When**: `RecordWorkflowSelectionCompleted(ctx, wfe, "fix-security-context-job")` is called
3. **Then**: The stored audit event payload contains `workflow_name = "fix-security-context-job"`

**Expected Results**:
1. `payload.WorkflowName.Set == true`
2. `payload.WorkflowName.Value == "fix-security-context-job"`

### UT-WE-1033-002: Workflow name omitted when empty

**BR**: BR-AUDIT-005
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/audit/selection_workflow_name_test.go`

**Preconditions**:
- `AuditStore` mock available
- Valid WFE object

**Test Steps**:
1. **Given**: A WFE spec with valid fields
2. **When**: `RecordWorkflowSelectionCompleted(ctx, wfe, "")` is called
3. **Then**: The stored audit event payload does NOT set `workflow_name`

**Expected Results**:
1. `payload.WorkflowName.Set == false`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `AuditStore` (external dependency), `client.Client` via `fake.NewClientBuilder()`
- **Location**: `test/unit/remediationorchestrator/controller/`, `test/unit/workflowexecution/audit/`
- **Resources**: Minimal

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (per No-Mocks Policy)
- **Infrastructure**: envtest (K8s API), PostgreSQL, Redis, DataStorage (real service)
- **Location**: `test/integration/remediationorchestrator/`, `test/integration/workflowexecution/`
- **Resources**: ~2GB RAM for envtest + DS stack

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| ogen | v1.18.0 | OpenAPI client generation |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| PR #1035 | Code | Merged | Base code for batch processing | N/A (merged) |
| Ogen v1.18.0 | Tool | Available | Cannot regenerate DS client | Use pinned version |

### 11.2 Execution Order

1. **Phase 1 (TDD RED)**: Write all failing unit tests (Gap 1 + Gap 2)
2. **Checkpoint 1**: Audit categories 1-9 on RED phase
3. **Phase 2 (TDD GREEN)**: Implement Gap 1 (verifying_handler.go) + Gap 2 (OpenAPI + ogen + audit manager + controller)
4. **Checkpoint 2**: Audit categories 1-9 on GREEN phase
5. **Phase 3 (TDD REFACTOR)**: 100-go-mistakes validation, extend workflow_name to other event types, clean up
6. **Checkpoint 3**: Final audit categories 1-9 on REFACTOR phase

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/1033/TEST_PLAN.md` | Strategy and test design |
| Unit test suite (Gap 1) | `test/unit/remediationorchestrator/controller/verifying_handler_outcome_test.go` | VerifyingHandler outcome audit tests |
| Unit test suite (Gap 2) | `test/unit/workflowexecution/audit/selection_workflow_name_test.go` | Workflow name audit tests |
| Integration test (Gap 1) | `test/integration/remediationorchestrator/audit_crd_outcome_integration_test.go` | Full reconciler crd_outcome test |
| Integration test (Gap 2) | `test/integration/workflowexecution/audit_workflow_name_integration_test.go` | WFE controller workflow name test |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests (Gap 1)
go test ./test/unit/remediationorchestrator/controller/... -ginkgo.v --race -ginkgo.focus="UT-RO-1033"

# Unit tests (Gap 2)
go test ./test/unit/workflowexecution/audit/... -ginkgo.v --race -ginkgo.focus="UT-WE-1033"

# Integration tests (Gap 1)
go test ./test/integration/remediationorchestrator/... -ginkgo.v --race -ginkgo.focus="IT-RO-1033"

# Integration tests (Gap 2)
go test ./test/integration/workflowexecution/... -ginkgo.v --race -ginkgo.focus="IT-WE-1033"

# Coverage
go test ./test/unit/remediationorchestrator/... -coverprofile=coverage_ut_ro.out
go tool cover -func=coverage_ut_ro.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/remediationorchestrator/controller/verifying_handler_test.go:49` | `EmitCompletionAudit` callback ignores `outcome` param (`_ string`) | No change required — existing tests test different behavior (notification, EA creation). New outcome tests are in a separate file. | Separation of concerns |
| `test/integration/workflowexecution/audit_workflow_refs_integration_test.go` | Asserts `WorkflowID`, `WorkflowVersion`, `ContainerImage`, `Phase` on selection payload | May need update if `workflow_name` presence is mandatory. Since it's optional, no change expected. | Additive field |

---

## 15. Due Diligence Findings

### Finding DD-1: `NewOverrideNotFoundError` exported from production code (API surface hygiene)

**Location**: `pkg/remediationorchestrator/override/merge.go:54-56`
**Issue**: Exported function with doc comment "for testing" — only referenced from `test/unit/remediationorchestrator/controller/awaiting_approval_handler_test.go:219`
**Severity**: P3 (pre-existing, not introduced by this PR)
**Action**: Flag for future cleanup. Not addressed in this PR to avoid scope creep.

### Finding DD-2: `NewMetricsWithRegistry` dual-purpose export

**Location**: `pkg/remediationorchestrator/metrics/metrics.go:222`
**Issue**: Used both in tests and in some integration packages. Not strictly test-only.
**Severity**: P3 (pre-existing, acceptable pattern for metrics)
**Action**: No action needed.

### Finding DD-3: No validation of `outcome` parameter in `BuildCompletionEvent`

**Location**: `pkg/remediationorchestrator/audit/manager.go:362-396`
**Issue**: The `outcome` string parameter is unbounded — no length check, no allowlist. An adversarial value (e.g., 1MB string) would pass through.
**Severity**: P2 (low probability in production — outcome is set by controller, not external input)
**Action**: Add adversarial test (UT-RO-1033-008) to document behavior. Consider adding validation in REFACTOR phase.

### Finding DD-4: Verifying handler test coverage gap for EmitCompletionAudit outcome

**Location**: `test/unit/remediationorchestrator/controller/verifying_handler_test.go:49`
**Issue**: All existing tests use a noop callback that discards the `outcome` parameter (`_ string`). No test asserts the outcome value.
**Severity**: P0 (this IS the bug — Issue #1033 Gap 1)
**Action**: New test file `verifying_handler_outcome_test.go` addresses this directly.

### Finding DD-5: `RecordWorkflowSelectionCompleted` has no unit tests

**Location**: `pkg/workflowexecution/audit/manager.go:136-196`
**Issue**: Only tested indirectly via integration tests (`audit_workflow_refs_integration_test.go`). No unit-level payload construction test.
**Severity**: P1 (coverage gap)
**Action**: New test file `selection_workflow_name_test.go` provides direct unit coverage.

### Finding DD-6: VerifyingHandler concurrency model

**Location**: `internal/controller/remediationorchestrator/verifying_handler.go`
**Issue**: Handler holds no mutex. Correctness relies on controller-runtime's serial reconciliation (default MaxConcurrentReconciles=1) and K8s optimistic concurrency on status updates.
**Severity**: P2 (safe under default config, but worth documenting)
**Action**: UT-RO-1033-007 tests `BuildCompletionEvent` under concurrent access. Handler-level concurrency is safe due to controller-runtime defaults.

---

## 16. Checkpoint Audit Categories

Each checkpoint MUST satisfy all 9 categories before advancing:

### Category Checklist (applied at each checkpoint)

1. **Observability wiring**: Every metric/gauge/counter has a test asserting value change
2. **Adversarial inputs**: Every external string parameter tested with `""`, max-length+1, `../../etc/passwd`, Unicode
3. **Resource bounds**: Maps/slices/caches tested with 50+ lifecycle cycles (N/A for this PR — no new maps/caches)
4. **Concurrency**: Mutex-protected methods tested with 10+ goroutines under `-race`
5. **Nil/zero edge cases**: Every nullable field tested through all consumers
6. **Error-path observability**: Every error log includes resource name, namespace, phase
7. **Cross-phase integration**: Phase N component proven wired to Phase M code
8. **Spec compliance**: K8s naming, HTTP status codes, OpenAPI discriminator
9. **API surface hygiene**: No test-only exports in production packages (flag pre-existing)

---

## 17. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-05 | Initial test plan |
