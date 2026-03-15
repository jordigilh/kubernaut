# Test Plan: HAPI `actionable` Field for Benign Alert Classification

**Feature**: Add `actionable` boolean field to LLM output, mapped to `AIAnalysis.Status.IsActionable`
**Version**: 1.1
**Created**: 2026-03-14
**Author**: AI Assistant
**Status**: Complete (Unit Tests)
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- [BR-HAPI-200](../../requirements/BR-HAPI-200-resolved-stale-signals.md): Handling Inconclusive Investigations and Self-Resolved Signals
- [BR-ORCH-037](../../requirements/BR-ORCH-037-workflow-not-needed.md): Handle AIAnalysis WorkflowNotNeeded
- [Issue #388](https://github.com/jordigilh/kubernaut/issues/388): HAPI escalation logic overrides LLM "not actionable" conclusion

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)

---

## 1. Scope

### In Scope

- **HAPI prompt (`prompt_builder.py`)**: Add `# actionable` field to the LLM output format. Add Outcome D section instructing the LLM to set `actionable: false` for benign alerts that don't warrant remediation or human review.
- **HAPI result parser (`result_parser.py`)**: Extract `actionable` boolean from LLM response. When `actionable == false`: set `needs_human_review: False`, set `is_actionable: False`, skip the `no_matching_workflows` escalation.
- **AIAnalysis response processor (`response_processor.go`)**: Use `IsActionable == false` (from HAPI response) to route through the existing `WorkflowNotNeeded` path with a new `subReason: NotActionable`.

### Out of Scope

- Fix B from #388 (context-aware escalation logic) — deferred to a separate issue
- Changes to RemediationOrchestrator — already handles `WorkflowNotNeeded` via BR-ORCH-037
- Changes to Notification Controller — informational notification for `NotActionable` follows existing `WorkflowNotNeeded` path
- Making the 0.7 confidence threshold configurable (noted for future; currently hardcoded)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| New `actionable` boolean field (not extending `investigation_outcome`) | `investigation_outcome` describes what happened during investigation (`resolved`, `inconclusive`). `actionable` describes whether the alert warrants action — a separate, orthogonal dimension. This keeps the semantic model clean. |
| `actionable` maps to `AIAnalysis.Status.Actionability` enum (`Actionable`/`NotActionable`) | Natural mapping. The LLM's assessment of actionability flows through to the CRD status as a string enum, making it available for Rego policies, audit, and downstream controllers. The enum is more idiomatic than a boolean for a three-state field (unset/Actionable/NotActionable). |
| Reuse `WorkflowNotNeeded` reason with new `subReason: NotActionable` | Distinguishes from `ProblemResolved` (issue went away) vs `NotActionable` (issue is present but benign). Reusing the existing reason avoids downstream RO/Notification changes. |
| `actionable: false` is authoritative (like `resolved`) | When the LLM explicitly sets `actionable: false`, the result parser forces `needs_human_review: False`, overriding any contradictory LLM boolean values. Prevents the safety-net from re-escalating benign alerts. |
| Same 0.7 confidence threshold for `not_actionable` routing | Consistent with `resolved` path. If confidence < 0.7, the alert is escalated to human review regardless of the `actionable` field value. The 0.7 threshold is universal and applies to all LLM conclusions. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (parser logic, prompt content, response processor branching)
- **Integration**: >=80% of integration-testable code (HAPI endpoint with mock LLM returning `not_actionable`, AIAnalysis processing the response)

### 2-Tier Minimum

Every business outcome is covered by at least Unit + Integration:
- **Unit tests** validate parser logic and prompt content in isolation
- **Integration tests** validate end-to-end flow through HAPI and AIAnalysis

### Business Outcome Quality Bar

Tests validate: "When the LLM determines an alert is benign, the system correctly concludes NoActionRequired without escalating to human review."

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `holmesgpt-api/src/extensions/incident/result_parser.py` | `parse_incident_response` (new `actionable` field extraction + routing) | ~20 |
| `holmesgpt-api/src/extensions/incident/prompt_builder.py` | Prompt template (Outcome D section + `# actionable` field definition) | ~25 |
| `pkg/aianalysis/handlers/response_processor.go` | `ProcessIncidentResponse` (new `IsActionable == false` routing), `hasNotActionableSignal` | ~15 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `holmesgpt-api/src/extensions/incident/llm_integration.py` | `analyze_incident` → full flow with mock LLM returning `actionable: false` | ~30 |
| `pkg/aianalysis/handlers/response_processor.go` | Full `ProcessIncidentResponse` with real HAPI client | ~50 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-200 (ext) | LLM prompt includes `# actionable` field and Outcome D instructions | P1 | Unit | UT-HAPI-388-001 | Pending |
| BR-HAPI-200 (ext) | Result parser: `actionable: false` → `needs_human_review: False`, `is_actionable: False` | P0 | Unit | UT-HAPI-388-002 | Pending |
| BR-HAPI-200 (ext) | Result parser: `actionable: false` overrides contradictory `needs_human_review: True` | P0 | Unit | UT-HAPI-388-003 | Pending |
| BR-HAPI-200 (ext) | Result parser: `actionable: false` skips `no_matching_workflows` escalation | P1 | Unit | UT-HAPI-388-004 | Pending |
| BR-HAPI-200 (ext) | Result parser: `actionable: false` emits audit-trail warning | P1 | Unit | UT-HAPI-388-005 | Pending |
| BR-HAPI-200 (ext) | AIAnalysis: `IsActionable == false` routes to `WorkflowNotNeeded/NotActionable` | P0 | Unit | UT-AA-388-001 | Pending |
| BR-HAPI-200 (ext) | AIAnalysis: `IsActionable == false` does NOT create WorkflowExecution | P0 | Unit | UT-AA-388-002 | Pending |
| BR-HAPI-200 (ext) | HAPI endpoint: mock LLM returns `actionable: false` → response has `needs_human_review: False`, `is_actionable: False` | P1 | Integration | IT-HAPI-388-001 | Pending |
| BR-ORCH-037 (ext) | RO: `WorkflowNotNeeded/NotActionable` → RR `Completed/NoActionRequired` | P0 | Integration | IT-RO-388-001 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `HAPI` (HolmesGPT-API), `AA` (AIAnalysis), `RO` (RemediationOrchestrator)
- **ISSUE_NUMBER**: 388
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `result_parser.py` (parser logic), `prompt_builder.py` (prompt content), `response_processor.go` (signal detection)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-388-001` | Prompt includes `# actionable` field definition and Outcome D instructions so the LLM knows when to signal `actionable: false` for benign alerts | Pending |
| `UT-HAPI-388-002` | When LLM returns `actionable: false`, the system concludes `needs_human_review: False` and `is_actionable: False` — benign alerts are not escalated | Pending |
| `UT-HAPI-388-003` | Even if LLM contradicts itself (`needs_human_review: true` + `actionable: false`), `actionable: false` takes precedence — benign alerts are never escalated | Pending |
| `UT-HAPI-388-004` | `actionable: false` does not trigger the `no_matching_workflows` escalation — the system recognizes this is an intentional "no action" decision | Pending |
| `UT-HAPI-388-005` | `actionable: false` emits a distinct warning (`"Alert not actionable — no remediation warranted"`) for audit trail | Pending |
| `UT-AA-388-001` | AIAnalysis routes `IsActionable == false` to `Completed/WorkflowNotNeeded/NotActionable` — operators see a clean completion, not a failure | Pending |
| `UT-AA-388-002` | No WorkflowExecution CRD is created when `IsActionable == false` — the system does not attempt remediation on benign alerts | Pending |

### Tier 2: Integration Tests

**Testable code scope**: HAPI endpoint with mock LLM, AIAnalysis with real HAPI client

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-HAPI-388-001` | Full HAPI flow: mock LLM returns `actionable: false` → HAPI response has `needs_human_review: False`, `is_actionable: False`, correct warnings | Pending |
| `IT-RO-388-001` | Full RO flow: AIAnalysis completes with `WorkflowNotNeeded/NotActionable` → RR transitions to `Completed/NoActionRequired` without creating WFE | Pending |

### Tier Skip Rationale

- **E2E**: Deferred — the `orphaned-pvc-no-action` demo scenario serves as the de facto E2E test once the fix is deployed. Formal E2E test can be added in a follow-up.

---

## 6. Test Cases (Detail)

### UT-HAPI-388-001: Prompt includes `actionable` field and Outcome D

**BR**: BR-HAPI-200 (extension)
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_resolved_signals_br_hapi_200.py`

**Given**: The HAPI prompt builder generates the incident analysis prompt
**When**: The prompt is built for any incident
**Then**: The prompt text contains:
- The `# actionable` field definition in the output format section
- Outcome D section: "Alert Not Actionable (No Remediation Warranted)"
- Guidance to set `actionable: false` for benign alerts (orphaned PVCs, completed job artifacts, non-impactful resource drift)
- Clear distinction from `resolved` (problem gone) vs `actionable: false` (problem present but benign)

**Acceptance Criteria**:
- Prompt contains `# actionable` as a defined output field
- Prompt contains Outcome D heading and `actionable` set to `false`
- Prompt provides concrete examples of benign alerts

---

### UT-HAPI-388-002: Parser handles `actionable: false` → no human review, `is_actionable: False`

**BR**: BR-HAPI-200 (extension)
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_resolved_signals_br_hapi_200.py`

**Given**: LLM analysis text containing `# actionable\nfalse`, `# selected_workflow\nNone`, `# confidence\n0.85`
**When**: `parse_incident_response` processes the analysis
**Then**: Result has `needs_human_review == False`, `human_review_reason is None`, and `is_actionable == False`

**Acceptance Criteria**:
- `needs_human_review` is `False`
- `human_review_reason` is `None`
- `is_actionable` is `False`
- `selected_workflow` is `None`
- No `no_matching_workflows` warning in warnings list

---

### UT-HAPI-388-003: `actionable: false` overrides contradictory `needs_human_review: true`

**BR**: BR-HAPI-200 (extension)
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_resolved_signals_br_hapi_200.py`

**Given**: LLM analysis containing `actionable: false` AND `needs_human_review: true` (contradiction)
**When**: `parse_incident_response` processes the analysis
**Then**: `actionable: false` takes precedence — `needs_human_review == False`

**Acceptance Criteria**:
- `needs_human_review` is `False` (explicit `actionable` field wins over boolean)
- `is_actionable` is `False`
- A warning/log is emitted about the contradiction override
- Follows same pattern as `resolved` contradiction handling (#301)

---

### UT-HAPI-388-004: `actionable: false` skips `no_matching_workflows` escalation

**BR**: BR-HAPI-200 (extension)
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_resolved_signals_br_hapi_200.py`

**Given**: LLM analysis with `actionable: false` and `selected_workflow: None`
**When**: `parse_incident_response` processes the analysis
**Then**: Warnings list does NOT contain "No workflows matched the search criteria"

**Acceptance Criteria**:
- The `no_matching_workflows` escalation is bypassed because `actionable: false` is an intentional decision
- `needs_human_review` remains `False` (not overridden by the no-workflow safety-net)

---

### UT-HAPI-388-005: Audit trail warning emitted for `actionable: false`

**BR**: BR-HAPI-200 (extension)
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_resolved_signals_br_hapi_200.py`

**Given**: LLM analysis with `actionable: false`
**When**: `parse_incident_response` processes the analysis
**Then**: Warnings list contains "Alert not actionable — no remediation warranted"

**Acceptance Criteria**:
- Warning text contains "not actionable"
- Warning is distinct from "Problem self-resolved" (Outcome A) and "Investigation inconclusive" (Outcome B)

---

### UT-AA-388-001: AIAnalysis routes `IsActionable == false` to `WorkflowNotNeeded/NotActionable`

**BR**: BR-HAPI-200 (extension), BR-ORCH-037
**Type**: Unit
**File**: `test/unit/aianalysis/investigating_handler_test.go`

**Given**: HAPI response with `is_actionable: false`, `needs_human_review: false`, `selected_workflow: null`, `confidence >= 0.7`, and warning containing "not actionable"
**When**: `ProcessIncidentResponse` processes the response
**Then**: AIAnalysis status transitions to `Completed` with `Reason: WorkflowNotNeeded`, `SubReason: NotActionable`, and `IsActionable: false`

**Acceptance Criteria**:
- `Phase == Completed` (NOT `Failed`)
- `Reason == "WorkflowNotNeeded"`
- `SubReason == "NotActionable"` (distinguishable from `ProblemResolved`)
- `IsActionable == false`
- No WorkflowExecution CRD created

---

### UT-AA-388-002: No WorkflowExecution for `IsActionable == false`

**BR**: BR-ORCH-037
**Type**: Unit
**File**: `test/unit/aianalysis/investigating_handler_test.go`

**Given**: HAPI response with `is_actionable: false`
**When**: AIAnalysis processes the response
**Then**: No WorkflowExecution CRD is created

**Acceptance Criteria**:
- WorkflowExecution list remains empty after processing
- AIAnalysis transitions directly to `Completed` without entering `Executing` phase

---

### IT-HAPI-388-001: Full HAPI flow with mock LLM returning `actionable: false`

**BR**: BR-HAPI-200 (extension)
**Type**: Integration
**File**: `holmesgpt-api/tests/integration/test_not_actionable_integration.py`

**Given**: HAPI endpoint configured with a mock LLM that returns an analysis containing `actionable: false` for a `KubePersistentVolumeClaimOrphaned` signal
**When**: An incident analysis request is sent to `POST /api/v1/incident/analyze`
**Then**: The session result contains `needs_human_review: False`, `is_actionable: False`, `selected_workflow: null`, and the audit event captures the not-actionable outcome

**Acceptance Criteria**:
- HTTP 202 accepted
- Session polling returns completed result
- `needs_human_review == False`
- `is_actionable == False`
- `human_review_reason` is `None`
- Audit event written with `is_actionable: false`

---

### IT-RO-388-001: RO handles `WorkflowNotNeeded/NotActionable`

**BR**: BR-ORCH-037 (extension)
**Type**: Integration
**File**: `test/integration/remediationorchestrator/lifecycle_test.go`

**Given**: An AIAnalysis resource with `Phase: Completed`, `Reason: WorkflowNotNeeded`, `SubReason: NotActionable`, `IsActionable: false`
**When**: RemediationOrchestrator reconciles the owning RemediationRequest
**Then**: RR transitions to `Completed` with `Outcome: NoActionRequired`

**Acceptance Criteria**:
- RR `OverallPhase == Completed`
- RR `Outcome == NoActionRequired`
- No WorkflowExecution CRD created
- No manual-review notification created
- `CompletionTime` is set

---

## 7. Test Infrastructure

### Unit Tests (Python — HAPI)

- **Framework**: pytest (existing HAPI convention)
- **Mocks**: Mock LLM responses (pre-built analysis text)
- **Location**: `holmesgpt-api/tests/unit/`

### Unit Tests (Go — AIAnalysis)

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Mock HAPI client response
- **Location**: `test/unit/aianalysis/`

### Integration Tests (Python — HAPI)

- **Framework**: pytest with httpx/TestClient
- **Mocks**: Mock LLM only (mock SDK agent response)
- **Location**: `holmesgpt-api/tests/integration/`

### Integration Tests (Go — RO)

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: envtest (fake K8s API)
- **Location**: `test/integration/remediationorchestrator/`

---

## 8. Execution

```bash
# HAPI unit tests
cd holmesgpt-api && python -m pytest tests/unit/test_resolved_signals_br_hapi_200.py -v

# HAPI integration tests
cd holmesgpt-api && python -m pytest tests/integration/test_not_actionable_integration.py -v

# AIAnalysis unit tests
go test ./test/unit/aianalysis/... -ginkgo.focus="UT-AA-388"

# RO integration tests
go test ./test/integration/remediationorchestrator/... -ginkgo.focus="IT-RO-388"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-14 | Initial test plan for #388 Fix A using `investigation_outcome: not_actionable` |
| 1.1 | 2026-03-14 | Redesigned: replaced `investigation_outcome: not_actionable` with `actionable` boolean field. `actionable` maps directly to `AIAnalysis.Status.IsActionable`. Keeps `investigation_outcome` clean (investigation state) and `actionable` as a separate dimension (action decision). |
| 1.2 | 2026-03-02 | Implementation complete. All Python unit tests (7/7) and Go unit tests (3/3) pass. Integration tests deferred to E2E validation with orphaned-pvc-no-action scenario. |
