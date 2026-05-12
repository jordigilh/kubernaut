# Test Plan: Audit Trail Coverage (#1111)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1111-v1
**Feature**: Fix audit event persistence for workflow discovery and achieve 100% audit trace test coverage
**Version**: 1.0
**Created**: 2026-05-12
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/1111-audit-trace-coverage`

---

## 1. Introduction

### 1.1 Purpose

This test plan ensures that every audit event type emitted by Kubernaut production code is validated at the highest applicable test tier. It addresses Issue #1111 where 4 `workflow.catalog.*` audit events are silently dropped due to a missing `remediation_id` parameter, and closes the broader gap where 25+ event types are emitted but never asserted in any test tier.

### 1.2 Objectives

1. **Fix #1111**: KA forwards `signal.RemediationID` to all 3 DS discovery tools, restoring 4 `workflow.catalog.*` audit events
2. **FP E2E coverage**: Promote 10 events from uncovered/MAY to the FP assertion list (30 -> 40 events)
3. **Service E2E coverage**: Add assertions for ~22 error/conditional path events at the service E2E tier
4. **IT coverage**: Verify ~18 webhook/bootstrap events are asserted at the integration tier
5. **100% emittable coverage**: Every event type with a production emitter path is validated at some tier

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| FP E2E event assertion count | 40 events | Count items in `exactlyOnceEvents` + `atLeastOnceEvents` |
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/tools/custom/... ./test/unit/datastorage/...` |
| Build clean | 0 errors | `go build ./...` |
| Lint clean | 0 new warnings | `golangci-lint run --timeout=5m` |
| Regression check | 0 regressions | All existing tests pass |
| Emittable event coverage | >=80% | (FP + Service E2E + IT asserted) / (total defined - never-emitted) |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-AUDIT-005: Audit trail completeness for SOC2 compliance
- BR-AUDIT-021: Remediation ID propagation for audit correlation
- BR-AUDIT-023: Per-step audit events for workflow discovery
- DD-WORKFLOW-014 v3.0: Workflow Selection Audit Trail
- DD-AUDIT-CORRELATION-001: Universal Correlation ID Standard
- Issue #1111: Workflow discovery audit events fail OpenAPI validation (empty correlation_id)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [100 Go Mistakes](https://github.com/teivah/100-go-mistakes)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `get_workflow` context filter change breaks non-investigator callers | Regression | Low | UT-KA-1111-010 | Best-effort signal loading (don't fail if missing) |
| R2 | `NewOptString("")` sends empty `remediation_id=` to DS | Silent failure | Medium | UT-KA-1111-006 | Guard with `if signal.RemediationID != ""` |
| R3 | `aiagent.enrichment.completed` uses `ai-rr-*` correlation | FP query miss | High | N/A | Moved to KA E2E tier instead of FP |
| R4 | FP E2E timeout with 10 more events | Flaky test | Low | FP Step 11 | 240s timeout sufficient; avg is ~80s |
| R5 | Service E2E failure injection infrastructure gaps | Deferred tests | Medium | Phase 3 | Defer to separate issues if infrastructure missing |

### 3.1 Risk-to-Test Traceability

- R1: Mitigated by UT-KA-1111-010 (get_workflow without signal context succeeds)
- R2: Mitigated by UT-KA-1111-006 (empty RemediationID does not send param)
- R3: Documented in FP Step 11 comments explaining why enrichment events are excluded

---

## 4. Scope

### 4.1 Features to be Tested

- **KA tool remediation_id forwarding** (`internal/kubernautagent/tools/custom/tools.go`): All 3 DS discovery tools forward `signal.RemediationID` as `remediation_id` query parameter
- **DS audit correlation** (`pkg/datastorage/audit/workflow_discovery_event.go`): `setCorrelationIDFromFilters` correctly sets correlation_id from filters or fallback
- **FP E2E audit completeness** (`test/e2e/fullpipeline/01_full_remediation_lifecycle_test.go`): Step 11 validates 40 event types (up from 30)
- **Service E2E error paths**: Error/conditional audit events validated per service
- **IT webhook/bootstrap events**: Administrative audit events validated at integration tier

### 4.2 Features Not to be Tested

- **Enrichment correlation fix**: `aiagent.enrichment.completed` uses `signal.IncidentID` (AIAnalysis name) instead of `signal.RemediationID`. This is a design decision, not a bug. Separate issue if alignment is needed.
- **Webhook correlation alignment**: `remediationworkflow.admitted.*` uses admission UID. Aligning to RR name would require webhook handler changes. Out of scope.
- **Never-emitted events** (~14 types): Error/edge-case events with no production emitter path in current scenarios (e.g., `gateway.crd.failed` requires K8s API server errors).

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| FP queries by `correlation_id = RR.Name` | DD-AUDIT-CORRELATION-001 establishes RR name as the universal correlation ID |
| Events with non-RR correlation are tested at service/IT tier | FP cannot query events correlated by admission UID or AIAnalysis name |
| `get_workflow` signal context is best-effort | Non-investigator callers (notification resolver, RO adapter, WE querier) may not have a signal context |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code in `tools.go` and `workflow_discovery_event.go`
- **Integration**: Covered by existing IT suites; new assertions added where gaps exist
- **E2E**: FP asserts 40 events; service E2E covers error/conditional paths

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- BR-AUDIT-021: Unit tests (UT-KA-1111-*) + FP E2E (Step 11)
- BR-AUDIT-023: Unit tests (UT-DS-1111-*) + FP E2E (Step 11)

### 5.3 Business Outcome Quality Bar

Tests validate that audit events are **persisted in DataStorage** with the correct **correlation_id**, enabling post-mortem queries by `remediation_id`.

### 5.4 Pass/Fail Criteria

**PASS**: All of the following:
1. All P0 tests pass (0 failures)
2. `go build ./...` succeeds
3. FP E2E Step 11 passes with 40 events
4. No regressions in existing test suites

**FAIL**: Any of the following:
1. Any P0 test fails
2. Build errors introduced
3. Existing tests regress

### 5.5 Suspension & Resumption Criteria

**Suspend**: Phase 3/4 service E2E tests requiring failure injection infrastructure that doesn't exist
**Resume**: When infrastructure is available or alternative approach identified

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/tools/custom/tools.go` | `listActionsTool.Execute`, `listWorkflowsTool.Execute`, `getWorkflowTool.Execute` | ~130 |
| `pkg/datastorage/audit/workflow_discovery_event.go` | `setCorrelationIDFromFilters`, `NewActionsListedAuditEvent`, `NewWorkflowsListedAuditEvent` | ~60 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `test/e2e/fullpipeline/01_full_remediation_lifecycle_test.go` | Step 11 audit assertions | ~160 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/1111-audit-trace-coverage` HEAD | Branch from main |
| Dependency: Phase 1 fix | Same branch | Phase 2 depends on Phase 1 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AUDIT-021 | Remediation ID forwarded for audit correlation | P0 | Unit | UT-KA-1111-001 | Pass |
| BR-AUDIT-021 | Remediation ID forwarded for list_workflows | P0 | Unit | UT-KA-1111-002 | Pass |
| BR-AUDIT-021 | Remediation ID forwarded for get_workflow | P0 | Unit | UT-KA-1111-003 | Pass |
| BR-AUDIT-023 | setCorrelationIDFromFilters uses RemediationID | P0 | Unit | UT-DS-1111-001 | Pass |
| BR-AUDIT-023 | setCorrelationIDFromFilters uses fallback | P0 | Unit | UT-DS-1111-002 | Pass |
| BR-AUDIT-023 | setCorrelationIDFromFilters leaves unset when empty | P0 | Unit | UT-DS-1111-003 | Pass |
| BR-AUDIT-023 | ActionsListed event has valid correlation_id | P0 | Unit | UT-DS-1111-004 | Pass |
| BR-AUDIT-023 | WorkflowsListed event has valid correlation_id | P0 | Unit | UT-DS-1111-005 | Pass |
| BR-AUDIT-021 | Empty RemediationID does not send param | P0 | Unit | UT-KA-1111-004 | Pass |
| BR-AUDIT-021 | Max-length RemediationID forwarded | P1 | Unit | UT-KA-1111-005 | Pass |
| BR-AUDIT-021 | Path traversal RemediationID forwarded | P1 | Unit | UT-KA-1111-006 | Pass |
| BR-AUDIT-021 | Unicode RemediationID forwarded | P1 | Unit | UT-KA-1111-007 | Pass |
| BR-AUDIT-021 | get_workflow without signal context succeeds | P0 | Unit | UT-KA-1111-008 | Pass |
| BR-AUDIT-021 | get_workflow with empty RemediationID skips | P0 | Unit | UT-KA-1111-009 | Pass |
| BR-AUDIT-023 | Nil filters with non-empty fallback | P0 | Unit | UT-DS-1111-006 | Pass |
| BR-AUDIT-023 | Empty filters with empty fallback | P0 | Unit | UT-DS-1111-007 | Pass |
| BR-AUDIT-005 | FP E2E validates 40 audit event types | P0 | E2E | FP-Step-11 | Pass |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: KA (Kubernaut Agent), DS (DataStorage), FP (Full Pipeline)
- **BR_NUMBER**: Issue number (1111)
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Phase 1 — KA tool remediation_id forwarding**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-1111-001` | list_available_actions forwards signal.RemediationID to DS params | Pass |
| `UT-KA-1111-002` | list_workflows forwards signal.RemediationID to DS params | Pass |
| `UT-KA-1111-003` | get_workflow loads signal context and forwards RemediationID + context filters | Pass |
| `UT-KA-1111-004` | Empty signal.RemediationID does NOT send remediation_id= to DS | Pass |
| `UT-KA-1111-005` | Max-length+1 (256 char) RemediationID forwarded without truncation | Pass |
| `UT-KA-1111-006` | Path traversal RemediationID (../../etc/passwd) forwarded as-is | Pass |
| `UT-KA-1111-007` | Unicode RemediationID (rr-テスト-123) forwarded correctly | Pass |
| `UT-KA-1111-008` | get_workflow without signal context in ctx succeeds (best-effort) | Pass |
| `UT-KA-1111-009` | get_workflow with signal context where RemediationID="" does not populate params | Pass |

**Phase 1 — DS setCorrelationIDFromFilters**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-1111-001` | setCorrelationIDFromFilters sets correlation from filters.RemediationID when non-empty | Pass |
| `UT-DS-1111-002` | setCorrelationIDFromFilters uses fallbackID when RemediationID is empty | Pass |
| `UT-DS-1111-003` | setCorrelationIDFromFilters leaves correlation unset when both are empty | Pass |
| `UT-DS-1111-004` | NewActionsListedAuditEvent with non-empty RemediationID produces valid correlation_id | Pass |
| `UT-DS-1111-005` | NewWorkflowsListedAuditEvent with non-empty RemediationID produces valid correlation_id | Pass |
| `UT-DS-1111-006` | Nil filters with non-empty fallbackID uses fallback | Pass |
| `UT-DS-1111-007` | Empty filters.RemediationID with empty fallbackID leaves correlation unset | Pass |

### Tier 3: E2E Tests

**Phase 2 — FP audit trail completeness**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `FP-Step-11` | FP E2E validates 40 audit event types (14 exactlyOnce + 26 atLeastOnce) | Pass |

---

## 9. Test Cases

### UT-KA-1111-001: list_available_actions forwards RemediationID

**BR**: BR-AUDIT-021
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/remediation_id_1111_test.go`

**Test Steps**:
1. **Given**: SignalContext with RemediationID="rr-test-123" in context
2. **When**: listActionsTool.Execute(ctx, emptyArgs)
3. **Then**: fakeWorkflowDS.listActionsParams.RemediationID equals OptString("rr-test-123")

### UT-DS-1111-001: setCorrelationIDFromFilters uses RemediationID

**BR**: BR-AUDIT-023
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/workflow_discovery_audit_test.go`

**Test Steps**:
1. **Given**: filters with RemediationID="rr-abc-123", fallbackID=""
2. **When**: NewActionsListedAuditEvent(filters, 5, 42)
3. **Then**: event.CorrelationID equals "rr-abc-123"

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `fakeWorkflowDS` (satisfies `WorkflowDiscoveryClient` interface)
- **Location**: `test/unit/kubernautagent/tools/custom/`, `test/unit/datastorage/`

### 10.2 E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind cluster with full Kubernaut deployment
- **Location**: `test/e2e/fullpipeline/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23 | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Phase 1 fix | Code | Same branch | Phase 2 FP assertions fail | Sequential execution |

### 11.2 Execution Order

1. **Phase 0**: Create this test plan
2. **Phase 1**: TDD RED/GREEN/REFACTOR for #1111 fix (UT-KA-1111-*, UT-DS-1111-*)
3. **Checkpoint 1**: 9-category quality gate
4. **Phase 2**: TDD RED/GREEN/REFACTOR for FP promotions
5. **Checkpoint 2**: 9-category quality gate
6. **Phase 3**: Service E2E error/conditional paths (parallel with Phase 4)
7. **Phase 4**: IT webhook/bootstrap events (parallel with Phase 3)
8. **Checkpoint 3+4**: Final quality gates

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/1111/TEST_PLAN.md` | Strategy and test design |
| KA unit tests | `test/unit/kubernautagent/tools/custom/remediation_id_1111_test.go` | TDD RED/GREEN/REFACTOR |
| DS unit tests | `test/unit/datastorage/workflow_discovery_audit_test.go` | Correlation ID tests |
| FP E2E update | `test/e2e/fullpipeline/01_full_remediation_lifecycle_test.go` | Step 11 promotions |

---

## 13. Execution

```bash
# Unit tests (Phase 1)
go test ./test/unit/kubernautagent/tools/custom/... -ginkgo.v
go test ./test/unit/datastorage/... -ginkgo.v

# Specific test by ID
go test ./test/unit/kubernautagent/tools/custom/... -ginkgo.focus="UT-KA-1111"

# Build validation
go build ./...
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `custom_tools_test.go` fakeWorkflowDS | GetWorkflowByID ignores params | Capture `getWorkflowParams` | Phase 1 needs to assert context filter forwarding |
| FP Step 11 exactlyOnceEvents | 13 events | 14 events (add `orchestrator.ea.created`) | Phase 2 promotion |
| FP Step 11 atLeastOnceEvents | 17 events | 26 events (add 9) | Phase 2 promotion |
| FP Step 11 MAY comments | 3 events as comments | Promote 3 to atLeastOnceEvents, 2 remain MAY | Phase 2 cleanup |

---

## 15. Audit Event Coverage Matrix

### Events Validated in FP E2E (40 total after Phase 2)

#### exactlyOnceEvents (14)

| Event Type | Service | Status |
|---|---|---|
| `gateway.signal.received` | Gateway | Current |
| `gateway.crd.created` | Gateway | Current |
| `orchestrator.lifecycle.created` | RO | Current |
| `orchestrator.lifecycle.started` | RO | Current |
| `orchestrator.lifecycle.verifying_started` | RO | Current |
| `orchestrator.lifecycle.verification_completed` | RO | Current |
| `orchestrator.lifecycle.completed` | RO | Current |
| `effectiveness.assessment.scheduled` | EM | Current |
| `effectiveness.health.assessed` | EM | Current |
| `effectiveness.hash.computed` | EM | Current |
| `effectiveness.alert.assessed` | EM | Current |
| `effectiveness.metrics.assessed` | EM | Current |
| `effectiveness.assessment.completed` | EM | Current |
| `orchestrator.ea.created` | RO | **NEW (Phase 2)** |

#### atLeastOnceEvents (26)

| Event Type | Service | Status |
|---|---|---|
| `orchestrator.lifecycle.transitioned` | RO | Current |
| `signalprocessing.enrichment.completed` | SP | Current |
| `signalprocessing.classification.decision` | SP | Current |
| `signalprocessing.signal.processed` | SP | Current |
| `signalprocessing.phase.transition` | SP | Current |
| `aianalysis.phase.transition` | AA | Current |
| `aianalysis.aiagent.call` | AA | Current |
| `aianalysis.rego.evaluation` | AA | Current |
| `aianalysis.analysis.completed` | AA | Current |
| `aiagent.llm.request` | KA | Current |
| `aiagent.llm.response` | KA | Current |
| `aiagent.workflow.validation_attempt` | KA | Current |
| `aiagent.response.complete` | KA | Current |
| `workflowexecution.selection.completed` | WE | Current |
| `workflowexecution.execution.started` | WE | Current |
| `workflowexecution.workflow.completed` | WE | Current |
| `notification.message.sent` | NOT | Current |
| `workflow.catalog.actions_listed` | DS | **NEW (Phase 2)** |
| `workflow.catalog.workflows_listed` | DS | **NEW (Phase 2)** |
| `workflow.catalog.workflow_retrieved` | DS | **NEW (Phase 2)** |
| `workflow.catalog.selection_validated` | DS | **NEW (Phase 2)** |
| `aiagent.rca.complete` | KA | **NEW (Phase 2)** |
| `remediation.workflow_created` | RO | **NEW (Phase 2)** |
| `aiagent.llm.tool_call` | KA | **PROMOTED from MAY** |
| `signalprocessing.business.classified` | SP | **PROMOTED from MAY** |
| `aianalysis.approval.decision` | AA | **PROMOTED from MAY** |

### Events NOT in FP (correlation mismatch — covered at other tiers)

| Event Type | Correlation Pattern | Test Tier |
|---|---|---|
| `aiagent.enrichment.completed` | `ai-rr-*` (AIAnalysis name) | KA E2E |
| `remediationworkflow.admitted.create` | Admission UID | Auth Webhook IT |
| `remediationworkflow.admitted.update` | Admission UID | Auth Webhook IT |

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-12 | Initial test plan with full coverage matrix |
