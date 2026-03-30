# Test Plan: Fix #588 — ManualReviewRequired Slack Notification Rendering

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-588-v1.0
**Feature**: Fix ManualReviewRequired Slack notification: duplicate content, sentinel RCA, empty code blocks, dead code, test gaps
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/v1.2.0-rc0`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the fixes for five bugs discovered in the ManualReviewRequired
Slack notification rendering path (Issue #588). The bugs cause: duplicate content between
Details and Warnings sections, sentinel RCA strings displayed verbatim, empty code blocks
from unbalanced triple backticks, dead code in the formatting layer, and missing unit test
coverage for the rendering path.

### 1.2 Objectives

1. **No duplicate content**: `Status.Message` (Details) and `Status.Warnings` (Warnings) are independent — warnings never appear in both sections
2. **Sentinel suppression**: Known sentinel RCA values ("Failed to parse RCA", "No structured RCA found") are omitted or replaced in notification body
3. **Clean Slack rendering**: Fenced code blocks with language tags are stripped, empty code blocks are removed, unbalanced backticks are handled gracefully
4. **Dead code removed**: `SlackFormatter`/`NewSlackFormatter` deleted from `pkg/notification/formatting/slack.go`
5. **Direct FormatSlackBlocks coverage**: Unit tests exercise the active Slack formatting path directly

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/aianalysis/... ./test/unit/remediationorchestrator/... ./test/unit/notification/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on affected files |
| Backward compatibility | 0 regressions | Existing tests pass without modification (except UT-NOT-048-071 updated) |
| Build integrity | 0 errors | `go build ./...` and `go vet ./...` clean |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #588: ManualReviewRequired Slack notification renders with empty code blocks, sentinel RCA, and duplicate content
- BR-NOT-083: Markdown to Slack mrkdwn conversion for notification body formatting
- BR-NOT-051: Multi-channel delivery (Slack mrkdwn format)
- BR-ORCH-036: Manual review notification creation and body building
- BR-HAPI-197: NeedsHumanReview handling and RCA propagation

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Test Plan Template](../../testing/TEST_PLAN_TEMPLATE.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Changing Status.Message content breaks downstream consumers | RR status message and K8s events show different text | Medium | UT-AA-588-001..003 | Traced all 6 consumers; all set Message directly in tests, not via response_processor |
| R2 | Sentinel list incomplete — new sentinels added later | New sentinel values render verbatim in Slack | Low | UT-RO-588-001..002 | Centralize sentinel list as package-level var for easy extension |
| R3 | MarkdownToMrkdwn changes affect non-ManualReview notifications | Other notification types have broken formatting | Medium | UT-NOT-588-001..004, UT-NOT-048-* | Existing comprehensive test suite (UT-NOT-048-*) validates all conversion paths |
| R4 | Deleting slack.go breaks compilation | Import cycle or missing symbol | Low | Build validation | grep confirmed zero production references |

### 3.1 Risk-to-Test Traceability

- **R1**: Mitigated by UT-AA-588-001 (Message excludes warnings), UT-AA-588-002 (Warnings independent)
- **R2**: Mitigated by UT-RO-588-001, UT-RO-588-002 (both known sentinels filtered)
- **R3**: Mitigated by UT-NOT-588-001..004 (new) + all existing UT-NOT-048-* (regression)
- **R4**: Mitigated by full `go build ./...` validation

---

## 4. Scope

### 4.1 Features to be Tested

- **AA Response Processor** (`pkg/aianalysis/handlers/response_processor.go`): Status.Message construction — must not include warnings
- **RO Notification Creator** (`pkg/remediationorchestrator/creator/notification.go`): buildManualReviewBody — sentinel RCA filtering, no duplication
- **NT Markdown Converter** (`pkg/notification/formatting/markdown_to_mrkdwn.go`): Fenced code block handling — strip language tags, remove empty blocks, handle unbalanced backticks
- **NT Slack Blocks** (`pkg/notification/delivery/slack_blocks.go`): Direct FormatSlackBlocks — block structure, mrkdwn conversion, priority emojis
- **NT Dead Code** (`pkg/notification/formatting/slack.go`): Removal of unused SlackFormatter

### 4.2 Features Not to be Tested

- **HAPI result_parser.py**: Sentinel generation is upstream behavior; we filter at the Go boundary
- **E2E Slack delivery**: Requires live webhook; deferred to E2E tier in separate plan
- **Other notification types** (approval, completion, escalation): Not affected by these changes

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Filter sentinels in `buildManualReviewBody`, not `populateManualReviewContext` | Body builder is the rendering boundary — keeps context struct as a faithful mirror of upstream data |
| Strip language tags rather than convert to Slack format | Slack mrkdwn in section blocks does not support language-tagged fences; stripping is the correct behavior |
| Centralize sentinel list as `var rcaSentinels` | Allows easy extension without code changes when HAPI adds new sentinel values |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (pure logic: `MarkdownToMrkdwn`, `buildManualReviewBody`, message construction in `response_processor`)
- **Integration**: Not applicable — all changes are pure logic with no I/O
- **E2E**: Deferred — requires Kind cluster with Slack webhook mock

### 5.2 Two-Tier Minimum

Integration tier is not applicable for these changes (pure logic, no I/O, no K8s API calls).
The unit tier provides sufficient coverage. E2E tier can validate the full rendering pipeline
in a future plan.

### 5.3 Business Outcome Quality Bar

Each test validates a business outcome:
- "Operator sees no duplicate text in Slack notification" (not "function returns correct string")
- "Sentinel RCA is not shown to operator" (not "filter function called")
- "Slack renders cleanly without empty code blocks" (not "regex matched")

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All 14 unit tests pass (0 failures)
2. Per-tier code coverage meets >=80% threshold on affected files
3. No regressions in existing test suites (UT-NOT-048-*, notification_creator_test, aianalysis_handler_test)
4. `go build ./...` and `go vet ./...` clean after dead code removal

**FAIL** — any of the following:

1. Any test fails
2. Per-tier coverage falls below 80% on affected files
3. Existing tests regress
4. Build breaks after slack.go deletion

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Build broken — code does not compile after response_processor changes
- Cascading failures — more than 3 existing tests fail for the same root cause

**Resume testing when**:
- Build fixed and green
- Root cause identified and fix deployed

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/aianalysis/handlers/response_processor.go` | Message construction (lines 320-357) | ~37 |
| `pkg/remediationorchestrator/creator/notification.go` | `buildManualReviewBody` (lines 726-764) | ~38 |
| `pkg/notification/formatting/markdown_to_mrkdwn.go` | `MarkdownToMrkdwn` (lines 66-140) | ~74 |
| `pkg/notification/delivery/slack_blocks.go` | `FormatSlackBlocks` (lines 29-75) | ~46 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

Not applicable — all changes are pure logic.

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/v1.2.0-rc0` HEAD | Active development branch |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-NOT-083 | Markdown to Slack mrkdwn conversion | P0 | Unit | UT-NOT-588-001..004 | Pending |
| BR-NOT-051 | Multi-channel delivery (Slack format) | P0 | Unit | UT-NOT-588-005..007 | Pending |
| BR-ORCH-036 | Manual review notification body | P0 | Unit | UT-RO-588-001..004 | Pending |
| BR-HAPI-197 | NeedsHumanReview RCA propagation | P0 | Unit | UT-AA-588-001..003 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`

- **TIER**: `UT` (Unit)
- **SERVICE**: AA (AIAnalysis), RO (RemediationOrchestrator), NOT (Notification)
- **ISSUE**: 588
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `response_processor.go` (message construction), `notification.go` (buildManualReviewBody), `markdown_to_mrkdwn.go` (MarkdownToMrkdwn), `slack_blocks.go` (FormatSlackBlocks). Target: >=80% coverage per file.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AA-588-001` | Operator sees only validation errors in Details section — warnings are separate | Pending |
| `UT-AA-588-002` | Status.Warnings is populated independently from Status.Message | Pending |
| `UT-AA-588-003` | Empty validation attempts + empty warnings = no misleading content in either field | Pending |
| `UT-RO-588-001` | Sentinel "Failed to parse RCA" is not shown to operator in Slack notification | Pending |
| `UT-RO-588-002` | Sentinel "No structured RCA found" is not shown to operator in Slack notification | Pending |
| `UT-RO-588-003` | Legitimate RCA summary is preserved and visible in Slack notification | Pending |
| `UT-RO-588-004` | Message and Warnings sections do not contain duplicate text | Pending |
| `UT-NOT-588-001` | Unbalanced triple backticks from HAPI errors do not produce empty code blocks in Slack | Pending |
| `UT-NOT-588-002` | Fenced code block language tags are stripped for Slack compatibility | Pending |
| `UT-NOT-588-003` | Empty fenced code blocks are removed from output | Pending |
| `UT-NOT-588-004` | Mixed content with backticks and formatting converts cleanly | Pending |
| `UT-NOT-588-005` | FormatSlackBlocks produces correct 3-block structure (header, section, context) | Pending |
| `UT-NOT-588-006` | Section block body applies MarkdownToMrkdwn conversion | Pending |
| `UT-NOT-588-007` | All priority levels map to correct emoji in header | Pending |

### Tier Skip Rationale

- **Integration**: All changes are pure logic (string formatting, filtering). No I/O, no K8s API, no DB. Unit tests provide full coverage.
- **E2E**: Requires Kind cluster with Slack webhook mock. Deferred to separate plan.

---

## 9. Test Cases

### UT-AA-588-001: Status.Message excludes warnings

**BR**: BR-HAPI-197
**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/response_processor_message_test.go`

**Test Steps**:
1. **Given**: An IncidentResponse with 2 validation attempts (each with errors) and 2 warnings
2. **When**: processValidationFailure builds Status.Message and Status.Warnings
3. **Then**: Status.Message contains only "Attempt 1: err1; Attempt 2: err2" — no warning text

**Acceptance Criteria**:
- **Behavior**: Message field separates validation errors from warnings
- **Correctness**: Message does not contain any string from resp.Warnings
- **Accuracy**: Status.Warnings == resp.Warnings (unchanged)

### UT-AA-588-002: Status.Warnings independent from Status.Message

**BR**: BR-HAPI-197
**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/response_processor_message_test.go`

**Test Steps**:
1. **Given**: An IncidentResponse with no validation attempts but 2 warnings
2. **When**: processValidationFailure builds Status.Message and Status.Warnings
3. **Then**: Status.Message is empty, Status.Warnings contains both warnings

**Acceptance Criteria**:
- **Behavior**: Warnings populate independently even when no validation errors exist
- **Correctness**: Status.Message == "" (not warnings joined)
- **Accuracy**: Status.Warnings == resp.Warnings

### UT-AA-588-003: Both fields empty when no errors and no warnings

**BR**: BR-HAPI-197
**Priority**: P1
**Type**: Unit
**File**: `test/unit/aianalysis/response_processor_message_test.go`

**Test Steps**:
1. **Given**: An IncidentResponse with no validation attempts and no warnings
2. **When**: processValidationFailure builds Status.Message and Status.Warnings
3. **Then**: Both Status.Message and Status.Warnings are empty/nil

### UT-RO-588-001: Sentinel "Failed to parse RCA" omitted

**BR**: BR-ORCH-036
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Test Steps**:
1. **Given**: ManualReviewContext with RootCauseAnalysis = "Failed to parse RCA"
2. **When**: CreateManualReviewNotification builds the notification body
3. **Then**: Body does not contain "Failed to parse RCA" and does not contain "Root Cause Analysis" section header

### UT-RO-588-002: Sentinel "No structured RCA found" omitted

**BR**: BR-ORCH-036
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Test Steps**:
1. **Given**: ManualReviewContext with RootCauseAnalysis = "No structured RCA found"
2. **When**: CreateManualReviewNotification builds the notification body
3. **Then**: Body does not contain "No structured RCA found"

### UT-RO-588-003: Legitimate RCA preserved

**BR**: BR-ORCH-036
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Test Steps**:
1. **Given**: ManualReviewContext with RootCauseAnalysis = "Pod OOM due to memory leak in container ml-worker"
2. **When**: CreateManualReviewNotification builds the notification body
3. **Then**: Body contains "Root Cause Analysis" section and the RCA summary text

### UT-RO-588-004: No duplicate content between Details and Warnings

**BR**: BR-ORCH-036
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Test Steps**:
1. **Given**: ManualReviewContext with Message = "Confidence below threshold" and Warnings = ["Low confidence score", "Missing resource limits"]
2. **When**: CreateManualReviewNotification builds the notification body
3. **Then**: "Confidence below threshold" appears exactly once (in Details), warning texts appear exactly once each (in Warnings)

### UT-NOT-588-001: Unbalanced triple backticks render cleanly

**BR**: BR-NOT-083
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/markdown_to_mrkdwn_test.go`

**Test Steps**:
1. **Given**: Input containing `Expected ```json code block or # section_header format.`
2. **When**: MarkdownToMrkdwn processes the input
3. **Then**: Output does not contain empty code blocks; the literal backticks are preserved as text

### UT-NOT-588-002: Language tags stripped from fenced code blocks

**BR**: BR-NOT-083
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/markdown_to_mrkdwn_test.go`

**Test Steps**:
1. **Given**: Input containing a fenced code block with language tag (```yaml\nkey: value\n```)
2. **When**: MarkdownToMrkdwn processes the input
3. **Then**: Output fenced block has no language tag (```\nkey: value\n```)

### UT-NOT-588-003: Empty fenced code blocks removed

**BR**: BR-NOT-083
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/markdown_to_mrkdwn_test.go`

**Test Steps**:
1. **Given**: Input containing "Before\n```\n```\nAfter"
2. **When**: MarkdownToMrkdwn processes the input
3. **Then**: Output is "Before\nAfter" (empty block removed, no extra blank lines)

### UT-NOT-588-004: Mixed content with backticks and formatting

**BR**: BR-NOT-083
**Priority**: P1
**Type**: Unit
**File**: `test/unit/notification/markdown_to_mrkdwn_test.go`

**Test Steps**:
1. **Given**: Input with fenced code block (with content), inline code, bold, and a link
2. **When**: MarkdownToMrkdwn processes the input
3. **Then**: Code block preserved (language tag stripped), inline code preserved, bold converted, link converted

### UT-NOT-588-005: FormatSlackBlocks block structure

**BR**: BR-NOT-051
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/delivery/slack_blocks_test.go`

**Test Steps**:
1. **Given**: A NotificationRequest with subject, body, priority, and type
2. **When**: FormatSlackBlocks formats the notification
3. **Then**: Returns exactly 3 blocks: header (plain_text), section (mrkdwn), context (mrkdwn)

### UT-NOT-588-006: Section block applies mrkdwn conversion

**BR**: BR-NOT-051
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/delivery/slack_blocks_test.go`

**Test Steps**:
1. **Given**: A NotificationRequest with body containing `**bold**` markdown
2. **When**: FormatSlackBlocks formats the notification
3. **Then**: Section block text contains `*bold*` (mrkdwn), not `**bold**` (markdown)

### UT-NOT-588-007: Priority emoji mapping

**BR**: BR-NOT-051
**Priority**: P1
**Type**: Unit
**File**: `test/unit/notification/delivery/slack_blocks_test.go`

**Test Steps**:
1. **Given**: NotificationRequests with each of the 4 priority levels (critical, high, medium, low)
2. **When**: FormatSlackBlocks formats each notification
3. **Then**: Header text starts with the correct emoji for each priority

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None required — all code under test is pure logic
- **Location**: `test/unit/aianalysis/`, `test/unit/remediationorchestrator/`, `test/unit/notification/`, `test/unit/notification/delivery/`

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22 | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None. All changes are self-contained within the fix/v1.2.0-rc0 branch.

### 11.2 Execution Order

1. **Phase 1**: Bug 3 — AA response_processor dedup (UT-AA-588-001..003)
2. **Phase 2**: Bug 2 — RO sentinel filtering (UT-RO-588-001..004)
3. **Phase 3**: Bug 1 — NOT fenced code blocks (UT-NOT-588-001..004)
4. **Phase 4**: Bug 4 — Dead code removal (build validation)
5. **Phase 5**: Bug 5 — FormatSlackBlocks direct tests (UT-NOT-588-005..007)

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/588/TEST_PLAN.md` | Strategy and test design |
| AA unit tests | `test/unit/aianalysis/response_processor_message_test.go` | Message construction tests |
| RO unit tests | `test/unit/remediationorchestrator/notification_creator_test.go` | Sentinel and dedup tests (added to existing file) |
| NOT formatting tests | `test/unit/notification/markdown_to_mrkdwn_test.go` | Fenced code block tests (added to existing file) |
| NOT delivery tests | `test/unit/notification/delivery/slack_blocks_test.go` | FormatSlackBlocks tests |

---

## 13. Execution

```bash
# Unit tests — all affected services
go test ./test/unit/aianalysis/... -v --ginkgo.v --ginkgo.focus="588"
go test ./test/unit/remediationorchestrator/... -v --ginkgo.v --ginkgo.focus="588"
go test ./test/unit/notification/... -v --ginkgo.v --ginkgo.focus="588"

# Full regression
go test ./test/unit/aianalysis/... -v --ginkgo.v
go test ./test/unit/remediationorchestrator/... -v --ginkgo.v
go test ./test/unit/notification/... -v --ginkgo.v

# Build validation
go build ./...
go vet ./...
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| UT-NOT-048-071 (`markdown_to_mrkdwn_test.go` line ~139) | `Expect(formatting.MarkdownToMrkdwn(input)).To(Equal(input))` — language tag preserved | Update expected to have language tag stripped | MarkdownToMrkdwn now strips language tags from fenced code blocks for Slack compatibility |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
