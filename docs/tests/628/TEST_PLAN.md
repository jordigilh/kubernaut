# Test Plan: Notification — Standardized Status Block (#628)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-628-v1.0
**Feature**: Standardize **Status** label and finite display enum across all notification body types built by the remediation orchestrator
**Version**: 1.0
**Created**: 2026-04-09
**Author**: AI Assistant
**Status**: Executing
**Branch**: `development/v1.3`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

This test plan validates Issue #628: eliminating inconsistent notification body vocabulary (e.g., **Outcome** vs **Result** vs absent status) by introducing a single **Status** line mapped to a finite operator-facing enum, positioned consistently after cluster/remediation prefix and before **Signal**, while preserving deprecated **Outcome** / **Result** for one release. Notification bodies are built in `pkg/remediationorchestrator/creator/notification.go` (not `pkg/notification/`).

### 1.2 Objectives

1. **Completion**: Body includes `**Status**:` with value **Remediated** (or correct mapping from `rr.Status.Outcome`).
2. **Bulk duplicate**: Body includes `**Status**:` **Duplicate Handled**; legacy `**Result**:` remains present for backward compatibility.
3. **Manual review / Approval / Self-resolved / Global timeout / Phase timeout**: Each body includes `**Status**:` with **Manual Review Required**, **Pending Approval**, **Self-Resolved**, or **Timed Out** as specified.
4. **Layout**: For all types that include **Signal**, `index("**Status**:") < index("**Signal**:")`.
5. **Label collision**: Status value **Timed Out** is distinguishable from any timestamp line that uses a “timed out” phrasing (timestamp line remains semantically distinct from `**Status**:`).
6. **Deprecation**: Old fields remain for one release with documented deprecation; tests assert both new **Status** and preserved legacy fields where required.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/...` |
| Integration test pass rate | 100% | N/A (unit-sufficient; see Section 8) |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `notification.go` and helpers |
| Backward compatibility | 0 unintended removals | **Outcome** / **Result** still present per scenario where specified |
| Regressions | 0 | `notification_creator_test.go`, `notification_body_order_test.go` (#627) updated only as planned |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- BR-ORCH-045: Completion notification outcome / completion path
- BR-ORCH-034: Bulk duplicate handling
- BR-ORCH-036: Manual review notifications
- BR-ORCH-027 / BR-ORCH-028: Global and phase timeout notifications
- Issue #628: Notification — Standardize Status Block across all notification types
- Issue #627: Notification body field ordering (prefix / Signal / ordering prerequisites)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Operators confuse **Status: Timed Out** with a timestamp label also implying timeout | Wrong triage | Medium | UT-NOT-628-006 | Assert distinct line prefixes; document human-readable distinction in implementation comments |
| R2 | Mapping from internal `Outcome` to display enum drifts | Wrong status shown | Medium | UT-NOT-628-001 | Table-driven mapping tests; single `FormatStatusLine` + mapper |
| R3 | Removing **Outcome**/**Result** too early breaks downstream parsers | Integration breakage | Low | UT-NOT-628-002 | Keep legacy fields one release; deprecation comments |
| R4 | **Status** placed after **Signal** | Regresses #627 intent | Low | UT-NOT-628-008 | Explicit index ordering test for all types with Signal |

### 3.1 Risk-to-Test Traceability

- **R1**: UT-NOT-628-006 verifies **Status** line vs timeout timestamp line are distinct.
- **R2**: UT-NOT-628-001 locks Completion mapping to **Remediated** (or approved mapping table).
- **R3**: UT-NOT-628-002 asserts **Result** still present alongside **Status**.
- **R4**: UT-NOT-628-008 covers all notification types that include **Signal**.

### 3.2 Preflight Validation Results (2026-04-09)

**All original risks confirmed manageable. Additional findings:**

| ID | Finding | Resolution |
|----|---------|------------|
| R1-PF | Timeout bodies use `**Timed Out**:` as timestamp label. `**Status**: Timed Out` has a structurally different prefix — no collision. | Confirmed safe. UT-NOT-628-006 validates. |
| R3-PF | UT-RO-627-001 asserts `**Outcome**:` before `**Signal**:`. Inserting `**Status**:` before `**Outcome**:` preserves the Outcome-before-Signal invariant. | No breakage. Update UT-RO-627-001 in REFACTOR to also check `**Status**:`. |
| R4-PF | Bulk duplicate body has `**Signal**:` before `**Result**:`. Inserting `**Status**:` before `**Signal**:` does not invert any tested ordering. | `**Result**:` stays after `**Signal**:` (legacy position). Only `**Status**:` added before. |
| G1 | `FormatStatusLine` signature undefined. | **Decision**: `FormatStatusLine(status string) string` → `"**Status**: " + status + "\n\n"`. Builders pass the correct hardcoded enum or `rr.Status.Outcome` for Completion. |
| G2 | Completion `rr.Status.Outcome` has 4 values (`Remediated`, `NoActionRequired`, `ManualReviewRequired`, `VerificationTimedOut`); plan enum only maps `Remediated`. | **Decision**: Pass `rr.Status.Outcome` directly — values are already human-readable. No translation needed. |
| I1 | UT-RO-627-001 checks `**Outcome**:` ordering; canonical label now `**Status**:`. | Update in REFACTOR: UT-RO-627-001 adds `**Status**:` check or UT-NOT-628-008 subsumes it. |

**Post-preflight confidence**: 97%

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **Notification body builders** (`pkg/remediationorchestrator/creator/notification.go`): Completion, Bulk Duplicate, Manual Review, Approval, Self-Resolved, Global Timeout, Phase Timeout — each emits `**Status**:` with the agreed enum value; position relative to **Signal** where applicable.
- **Shared formatting helpers**: `FormatStatusLine` (or equivalent) and centralized mapping from remediation/request outcomes to display enum.
- **Backward compatibility**: Deprecated **Outcome** / **Result** fields retained per human decision.

### 4.2 Features Not to be Tested

- **`pkg/notification/` controller delivery**: Out of scope; bodies are authored in RO creator.
- **SMTP/Teams/slack rendering**: Channel-specific formatting not covered here.
- **E2E operator inbox**: Deferred; string contract covered at unit level.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Label **Status** everywhere | Single scan pattern for operators |
| Finite enum (Remediated \| Pending Approval \| Timed Out \| Manual Review Required \| Self-Resolved \| Duplicate Handled) | Predictable UI/triage; maps from internal states |
| Position after cluster/remediation prefix + intro line, before **Signal** | Aligns with #627 ordering goals |
| Keep **Outcome**/**Result** one release | Safe migration for consumers parsing legacy fields |
| `FormatStatusLine(status string) string` as shared helper | Single format source; no mapping logic inside — caller passes final display value |
| Completion passes `rr.Status.Outcome` directly (no translation) | Outcome enum values are already human-readable (`Remediated`, etc.) |

### 4.4 Insertion Points (per builder, confirmed by preflight)

| Builder | Insertion Point | Status Value |
|---------|----------------|--------------|
| `buildCompletionBody` | After title line, before `**Outcome**:` | `rr.Status.Outcome` (passthrough) |
| `buildBulkDuplicateBody` | After intro line, before `**Signal**:` | `Duplicate Handled` (fixed) |
| `buildManualReviewBody` | After header line, before `**Signal**:` | `Manual Review Required` (fixed) |
| `buildApprovalBody` | After intro line, before `**Signal**:` | `Pending Approval` (fixed) |
| `buildSelfResolvedBody` | After intro line, before `**Signal**:` | `Self-Resolved` (fixed) |
| `BuildGlobalTimeoutBody` | After intro paragraph, before `**Signal**:` | `Timed Out` (fixed) |
| `BuildPhaseTimeoutBody` | After intro paragraph, before `**Signal**:` | `Timed Out` (fixed) |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code in notification body builders and status mapping helpers.
- **Integration**: Notification strings are pure builder output; integration tier not required for #628 (see Section 8).
- **E2E**: Not in scope for this test plan.

### 5.2 Two-Tier Minimum

BR coverage is met via **unit tests** that assert final body strings; integration with Kubernetes/DS is unchanged. Tier skip rationale documented in Section 8.

### 5.3 Business Outcome Quality Bar

Tests answer: “Does every notification type expose a consistent **Status** line with the correct enum, correct position, and required legacy fields?”

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 unit tests pass (UT-NOT-628-001 through UT-NOT-628-008).
2. Per-tier unit coverage >=80% on targeted files.
3. No regressions in existing remediation orchestrator notification tests except planned assertion updates (Section 14).
4. Legacy **Outcome**/**Result** fields present where this plan requires them.

**FAIL** — any of the following:

1. Any P0 unit test fails.
2. Coverage below 80% on notification builder/mapping code.
3. Unplanned regressions in #627 ordering tests.
4. **Status** missing on any of the seven scenarios.

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- Issue #627 body-order contract is in flux on the branch without coordinated merge.
- `notification.go` refactor in progress with non-compiling intermediate commits.

**Resume testing when**:

- #627 expectations stable or Section 14 updates merged.
- Build green; `go test ./test/unit/remediationorchestrator/...` executable.

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/creator/notification.go` | Body builders for Completion, Bulk Duplicate, Manual Review, Approval, Self-Resolved, Global/Phase timeout | TBD |
| `pkg/remediationorchestrator/creator/notification.go` (or extracted helper) | `FormatStatusLine`, outcome→enum mapping | TBD |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| — | — | Not applicable for #628 body contract |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.3` HEAD | Post-#627 merge recommended |
| Dependency: Issue #627 | Merged or branch includes ordering tests | `notification_body_order_test.go` |

---

## 7. BR Coverage Matrix

> Kubernaut-specific. Maps every business requirement to test scenarios across tiers.

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-045 | Completion notification reflects remediation outcome | P0 | Unit | UT-NOT-628-001 | Pass |
| BR-ORCH-034 | Bulk duplicate handled notification | P0 | Unit | UT-NOT-628-002 | Pass |
| BR-ORCH-036 | Manual review required path | P0 | Unit | UT-NOT-628-003 | Pass |
| BR-ORCH-027 / BR-ORCH-028 | Timeout notifications (global / phase) | P0 | Unit | UT-NOT-628-006, UT-NOT-628-007 | Pass |
| BR-ORCH-045 (approval branch) | Pending approval notification | P0 | Unit | UT-NOT-628-004 | Pass |
| BR-ORCH-045 (self-resolved) | Self-resolved notification | P0 | Unit | UT-NOT-628-005 | Pass |
| Issue #628 | Consistent **Status** position vs **Signal** | P0 | Unit | UT-NOT-628-008 | Pass |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

> **IEEE 829 Test Design Specification** — How test cases are organized and grouped.

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}` — **SERVICE** = `NOT` (notification body contract).

### Tier 1: Unit Tests

**Testable code scope**: `pkg/remediationorchestrator/creator/notification.go` and status mapping helpers — >=80% coverage target.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-NOT-628-001` | Completion body contains `**Status**:` **Remediated** (or correct mapping from `rr.Status.Outcome`) | Pass |
| `UT-NOT-628-002` | Bulk duplicate: `**Status**:` **Duplicate Handled**; `**Result**:` still present | Pass |
| `UT-NOT-628-003` | Manual review: `**Status**:` **Manual Review Required** | Pass |
| `UT-NOT-628-004` | Approval: `**Status**:` **Pending Approval** | Pass |
| `UT-NOT-628-005` | Self-resolved: `**Status**:` **Self-Resolved** | Pass |
| `UT-NOT-628-006` | Global timeout: `**Status**:` **Timed Out**; timestamp/timeout wording line distinct from status line | Pass |
| `UT-NOT-628-007` | Phase timeout: `**Status**:` **Timed Out** | Pass |
| `UT-NOT-628-008` | Position: `Index("**Status**:") < Index("**Signal**:")` for all types that include **Signal** | Pass |

### Tier 2: Integration Tests

Not applicable — notification strings are validated in unit tests with fake inputs; no external I/O.

### Tier 3: E2E Tests (if applicable)

Not applicable.

### Tier Skip Rationale (if any tier is omitted)

- **Integration / E2E**: Body composition is deterministic string generation; existing pattern in `test/unit/remediationorchestrator/notification_creator_test.go` and #627 order tests.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### UT-NOT-628-001: Completion status line

**BR**: BR-ORCH-045
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go` (or companion file)

**Preconditions**:
- Test fixtures construct a Completion notification context with representative `rr.Status.Outcome` values.

**Test Steps**:
1. **Given**: Notification creator builds Completion body for a remediated outcome.
2. **When**: Body string is produced.
3. **Then**: Body contains `**Status**:` followed by **Remediated** (or approved mapping from outcome enum).

**Expected Results**:
1. `**Status**:` appears exactly once as the status discriminator (value matches mapping table).
2. Legacy **Outcome** field behavior per deprecation policy still satisfied if present.

**Acceptance Criteria**:
- **Behavior**: Operators see unified **Status** on Completion.
- **Correctness**: Mapped value matches internal outcome.
- **Accuracy**: No duplicate conflicting status labels.

**Dependencies**: Issue #627 ordering (Status before Signal).

---

### UT-NOT-628-002: Bulk duplicate status + legacy Result

**BR**: BR-ORCH-034
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Preconditions**:
- Fixture for bulk duplicate notification generation.

**Test Steps**:
1. **Given**: Bulk duplicate scenario inputs.
2. **When**: Body is built.
3. **Then**: Body contains `**Status**:` **Duplicate Handled** and still contains `**Result**:` (legacy).

**Expected Results**:
1. **Status** line present with correct enum.
2. **Result** line present (backward compat).

**Acceptance Criteria**:
- **Behavior**: Single **Status** scan path works; parsers using **Result** do not break.

**Dependencies**: None.

---

### UT-NOT-628-003: Manual review status

**BR**: BR-ORCH-036
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Preconditions**:
- Manual review notification inputs.

**Test Steps**:
1. **Given**: Manual review body build.
2. **When**: String generated.
3. **Then**: Contains `**Status**:` **Manual Review Required**.

**Expected Results**:
1. Status value exact match (spacing/punctuation as implemented).

**Acceptance Criteria**:
- **Behavior**: Operator sees explicit manual review state in standard slot.

**Dependencies**: None.

---

### UT-NOT-628-004: Approval pending status

**BR**: BR-ORCH-045 (approval path)
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Preconditions**:
- Approval notification with rationale/prompt as today.

**Test Steps**:
1. **Given**: Approval body build.
2. **When**: String generated.
3. **Then**: Contains `**Status**:` **Pending Approval**.

**Expected Results**:
1. **Status** present; approval CTA content unchanged except for ordering per #627.

**Acceptance Criteria**:
- **Behavior**: Pending approval visible via **Status**.

**Dependencies**: Issue #627.

---

### UT-NOT-628-005: Self-resolved status

**BR**: BR-ORCH-045
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Test Steps**:
1. **Given**: Self-resolved scenario.
2. **When**: Body built.
3. **Then**: `**Status**:` **Self-Resolved**.

**Expected Results**:
1. Enum exact match.

**Acceptance Criteria**:
- **Behavior**: Self-resolved distinguishable from Remediated/Duplicate.

**Dependencies**: None.

---

### UT-NOT-628-006: Global timeout status vs timestamp

**BR**: BR-ORCH-027 / BR-ORCH-028
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Test Steps**:
1. **Given**: Global timeout body with timeout instant line (however currently labeled).
2. **When**: Body built.
3. **Then**: `**Status**:` **Timed Out** exists; timestamp or “timed out at” line is not the same line as `**Status**:` (distinct prefix or structure).

**Expected Results**:
1. Two distinguishable lines: one is `**Status**:`; the other carries temporal information without replacing **Status**.

**Acceptance Criteria**:
- **Behavior**: No ambiguity between “when” and “what state”.
- **Correctness**: Regex/index assertions prove separation.

**Dependencies**: Human decision on timestamp label wording.

---

### UT-NOT-628-007: Phase timeout status

**BR**: BR-ORCH-027 / BR-ORCH-028
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Test Steps**:
1. **Given**: Phase timeout scenario.
2. **When**: Body built.
3. **Then**: `**Status**:` **Timed Out**.

**Expected Results**:
1. Same enum as global timeout where appropriate.

**Acceptance Criteria**:
- **Behavior**: Phase timeout surfaced as timed-out status.

**Dependencies**: None.

---

### UT-NOT-628-008: Status before Signal

**BR**: Issue #628 / BR-ORCH-*
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_body_order_test.go` (extend) or dedicated file

**Test Steps**:
1. **Given**: Each notification type that includes `**Signal**:`.
2. **When**: Body built.
3. **Then**: `strings.Index(body, "**Status**:") < strings.Index(body, "**Signal**:")`.

**Expected Results**:
1. Ordering holds for every applicable type.

**Acceptance Criteria**:
- **Behavior**: Consistent scan order for operators.

**Dependencies**: Issue #627.

---

## 10. Environmental Needs

> **IEEE 829 §13** — Hardware, software, tools, and infrastructure required.

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Fake Kubernetes client only where existing notification tests require it (external dependency mock)
- **Location**: `test/unit/remediationorchestrator/`
- **Resources**: None special

### 10.2 Integration Tests

- Not required for this plan.

### 10.3 E2E Tests (if applicable)

- Not required.

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ (project standard) | Build and test |
| Ginkgo CLI | v2.x | BDD runner |

---

## 11. Dependencies & Schedule

> **IEEE 829 §12/§16** — Remaining tasks, blocking issues, and execution order.

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #627 | Code / tests | Merged preferred | UT-NOT-628-008 may conflict with ordering assumptions | Coordinate merge or adjust indices in same PR |

### 11.2 Execution Order

1. **RED**: Add failing assertions for `**Status**:` in each builder scenario.
2. **GREEN**: Implement `FormatStatusLine` + mapping; insert **Status** after prefix, before **Signal**; keep **Outcome**/**Result** with deprecation comments.
3. **REFACTOR**: Shared helper for status line; deduplicate mapping; update Section 14 tests.

**TDD sequence**: RED (assert Status field in each builder) → GREEN (add FormatStatusLine + mapping) → REFACTOR (shared helper, deprecation comments on old fields).

---

## 12. Test Deliverables

> **IEEE 829 §11** — What artifacts this test effort produces.

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/628/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/remediationorchestrator/` | Ginkgo tests for #628 |
| Coverage report | CI artifact | Unit coverage on notification builder |

---

## 13. Execution

```bash
# Unit tests — remediation orchestrator notification suite
go test ./test/unit/remediationorchestrator/... -ginkgo.v

# Focus by issue / ID prefix
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-NOT-628" -ginkgo.v

# Coverage
go test ./test/unit/remediationorchestrator/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates (if applicable)

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `notification_creator_test.go` | May assert **Outcome**/**Result** without **Status** | Add `**Status**:` assertions; keep legacy where required | #628 contract |
| `notification_body_order_test.go` (#627) | **Outcome** before **Signal** | Also assert **Status** before **Signal** (or align with UT-NOT-628-008) | Unified label |
| `notification_creator_test.go` | Per-type string snapshots | Update expected strings to include **Status** line | New field |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-09 | Initial test plan for Issue #628 |
| 1.1 | 2026-04-09 | Preflight validation: confirmed insertion points, resolved G1/G2 gaps, added Section 4.4 |
| 1.2 | 2026-04-09 | TDD complete: all 8 tests pass (RED→GREEN→REFACTOR). #627 ordering test updated. |
