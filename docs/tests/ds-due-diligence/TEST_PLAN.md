# Test Plan: DataStorage Due Diligence Findings (F1-F10)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-DS-DD-v1.0
**Feature**: Fix 10 findings from DataStorage due diligence audit (SQL query, correlation logic, handler validation, JSON serialization, effectiveness routing)
**Version**: 1.0
**Created**: 2026-04-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/v1.2.0-rc3`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates fixes for 10 findings (F1-F10) discovered during a rigorous due diligence audit of the DataStorage service codebase. The findings range from HIGH-severity SQL query bugs causing false negatives to LOW-severity JSON serialization inconsistencies.

### 1.2 Objectives

1. **F1**: EM subquery returns results regardless of tier boundary alignment
2. **F2**: CorrelateTier1Chain infers postRemediation when EM data is unavailable
3. **F3**: Chain entries sorted ascending by completedAt per OpenAPI spec
4. **F4**: Handler rejects negative, zero, and inverted window durations
5. **F5**: JSON response buffered before writing HTTP status to prevent truncation
6. **F7**: queryEffectivenessEvents merges event_type column into EventData
7. **F9**: SideEffects serializes as JSON `[]` not `null`

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/datastorage/...` |
| Integration test pass rate | 100% | `go test ./test/integration/datastorage/...` |
| Backward compatibility | 0 regressions | All existing 47 UT + 8 IT pass |

---

## 2. References

### 2.1 Authority

- **BR-HAPI-016**: Remediation History Context Enrichment (P0 CRITICAL)
- **DD-HAPI-016 v1.4**: Remediation History Context — two-tier query, three-way hash comparison
- **Issue #616**: RO routing guardrails not triggering — ineffective chain / recurrence detection

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [TP-616-v1.1](../616/TEST_PLAN.md) — Issue #616 test plan (prerequisite work)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | F1 time-unbounded EM subquery scans too many rows | Slow history API responses | Low | IT-DS-F1-001 | `idx_audit_events_post_remediation_spec_hash` partial index limits scan scope |
| R2 | F2 inference changes semantics of CorrelateTier1Chain | Existing consumers get unexpected hashMatch | Medium | UT-DS-F2-001/002 | Only inferred when preHash != currentSpecHash AND postHash is empty |
| R3 | F3 sort order change breaks RO countIneffectiveChain | RO guardrails miscalculate chain length | Medium | UT-RH-LOGIC-007/013, UT-DS-616-003 | Blast radius analysis confirmed ascending fixes latent bugs in RO |
| R4 | F4 window validation rejects valid edge cases | HAPI requests fail unnecessarily | Low | UT-DS-F4-001/002/003 | Only reject negative/zero/inverted; accept any positive pair |

---

## 4. Scope

### 4.1 Features to be Tested

- **F1** (`pkg/datastorage/repository/remediation_history_repository.go`): EM subquery timestamp removal
- **F2** (`pkg/datastorage/server/remediation_history_logic.go`): postRemediation inference in CorrelateTier1Chain and BuildTier2Summaries
- **F3** (`pkg/datastorage/server/remediation_history_logic.go`): Sort order fix for both tier chains
- **F4** (`pkg/datastorage/server/remediation_history_handler.go`): Window duration validation
- **F5** (`pkg/datastorage/server/remediation_history_handler.go`): Buffered JSON response
- **F7** (`pkg/datastorage/server/effectiveness_handler.go`): event_type column merge
- **F9** (`pkg/datastorage/server/remediation_history_logic.go`): SideEffects initialization

### 4.2 Features Not to be Tested

- **F6**: Migration 005 — index creation is operational, not code-testable
- **F8**: Doc comments — documentation quality, no runtime behavior
- **F10**: GitHub issue — process artifact, no code

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| F2 inference in CorrelateTier1Chain (not handler) | Keeps correlation logic self-contained; only caller is the handler after QueryROEventsBySpecHash |
| F3 ascending sort matches OpenAPI spec | Blast radius analysis confirmed ascending fixes latent bugs in RO and HAPI consumers |
| F5 buffer with bytes.Buffer | Standard Go pattern; avoids partial 200 responses on encoding failure |

---

## 5. Approach

### 5.1 TDD Phases

- **Phase 1 (RED)**: Write all failing tests — every test must fail for the expected reason
- **Phase 2 (GREEN)**: Implement minimal fixes — no refactoring, no doc improvements
- **Phase 3 (REFACTOR)**: Code quality, doc comments, helper extraction

### 5.2 Pass/Fail Criteria

**PASS**: All P0 tests pass; per-tier coverage >= 80%; zero regressions in existing suites.

**FAIL**: Any P0 test fails; existing tests regress; build errors.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `remediation_history_logic.go` | `CorrelateTier1Chain`, `BuildTier2Summaries` | ~140 |
| `remediation_history_handler.go` | `HandleGetRemediationHistoryContext` (validation + response) | ~180 |
| `effectiveness_handler.go` | `queryEffectivenessEvents`, `BuildEffectivenessResponse` | ~80 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `remediation_history_repository.go` | `QueryROEventsBySpecHash` | ~30 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-016 | EM subquery returns events across tier boundaries | P0 | Integration | IT-DS-F1-001 | Pending |
| BR-HAPI-016 | Infer postRemediation when EM data missing (Tier 1) | P0 | Unit | UT-DS-F2-001 | Pending |
| BR-HAPI-016 | Infer postRemediation when EM data missing (Tier 2) | P0 | Unit | UT-DS-F2-002 | Pending |
| BR-HAPI-016 | Chain sorted ascending by completedAt (Tier 1) | P0 | Unit | UT-RH-LOGIC-007 (mod) | Pending |
| BR-HAPI-016 | Chain sorted ascending by completedAt (Tier 2) | P0 | Unit | UT-RH-LOGIC-013 (mod) | Pending |
| BR-HAPI-016 | Chain sorted ascending (regression gate) | P0 | Unit | UT-DS-616-003 (mod) | Pending |
| BR-STORAGE-024 | Reject negative window duration | P1 | Unit | UT-DS-F4-001 | Pending |
| BR-STORAGE-024 | Reject zero window duration | P1 | Unit | UT-DS-F4-002 | Pending |
| BR-STORAGE-024 | Reject inverted window durations | P1 | Unit | UT-DS-F4-003 | Pending |
| BR-STORAGE-024 | Buffered JSON response integrity | P1 | Unit | UT-DS-F5-001 | Pending |
| BR-EM-001 | event_type merge in single-event query | P1 | Unit | UT-DS-F7-001 | Pending |
| BR-HAPI-016 | SideEffects serialized as empty array | P2 | Unit | UT-DS-F9-001 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-DS-F{FINDING}-{SEQUENCE}` for new tests. Existing test IDs retained where modified.

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-F2-001` | CorrelateTier1Chain infers hashMatch=postRemediation when preHash != currentSpecHash and no EM data | Pending |
| `UT-DS-F2-002` | BuildTier2Summaries infers hashMatch=postRemediation under same conditions | Pending |
| `UT-RH-LOGIC-007` (mod) | CorrelateTier1Chain sorts entries ascending by completedAt | Pending |
| `UT-RH-LOGIC-013` (mod) | BuildTier2Summaries sorts summaries ascending by completedAt | Pending |
| `UT-DS-616-003` (mod) | Mixed pre/post hash entries sorted ascending | Pending |
| `UT-DS-F4-001` | Handler rejects negative tier1Window with 400 | Pending |
| `UT-DS-F4-002` | Handler rejects zero tier1Window with 400 | Pending |
| `UT-DS-F4-003` | Handler rejects inverted tier2Window < tier1Window with 400 | Pending |
| `UT-DS-F5-001` | Handler returns complete, valid JSON response | Pending |
| `UT-DS-F7-001` | BuildEffectivenessResponse returns no_data when events lack event_type | Pending |
| `UT-DS-F9-001` | CorrelateTier1Chain entry.SideEffects is non-nil empty slice | Pending |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-DS-F1-001` | QueryROEventsBySpecHash returns RO event when EM hash event is outside query time window | Pending |

### Tier Skip Rationale

- **E2E**: Existing 5 E2E tests provide regression coverage. Findings are in SQL/logic layers already covered by UT+IT.

---

## 9. Test Cases

### IT-DS-F1-001: EM subquery cross-tier boundary

**BR**: BR-HAPI-016
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/ds_due_diligence_integration_test.go`

**Preconditions**:
- Real PostgreSQL with schema migrations applied
- Insert RO event at T-25h (within tier 2 window [T-90d, T-24h])
- Insert EM hash.computed at T-23h (OUTSIDE tier 2 window) with postHash=B

**Test Steps**:
1. **Given**: EM hash event timestamp falls outside the query time window
2. **When**: `QueryROEventsBySpecHash(ctx, "sha256:B", T-90d, T-24h)` is called
3. **Then**: Returns the RO event via cross-boundary EM correlation

**Expected Results**:
1. Query returns exactly 1 row (the RO event)
2. Row's correlation_id matches the inserted event
3. Event was found via the post-hash subquery path

### UT-DS-F2-001: postRemediation inference (Tier 1)

**BR**: BR-HAPI-016
**Priority**: P0
**Type**: Unit

**Preconditions**:
- RO event with preHash="sha256:other" (not matching currentSpecHash="sha256:target")
- Empty EM events map

**Test Steps**:
1. **When**: `CorrelateTier1Chain(roEvents, emptyEM, "sha256:target")` is called
2. **Then**: entry.HashMatch = postRemediation (inferred)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `RemediationHistoryQuerier` mock for handler tests (F4, F5)
- **Location**: `test/unit/datastorage/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: Real PostgreSQL (existing suite_test.go)
- **Location**: `test/integration/datastorage/`

---

## 11. Existing Tests Requiring Updates

| Test ID | Current Assertion | Required Change | Reason |
|---------|-------------------|-----------------|--------|
| UT-RH-LOGIC-006 | hashMatch=none (no EM data) | hashMatch=postRemediation | F2: inference when preHash != currentSpecHash |
| UT-RH-LOGIC-007 | entries[0]=rr-newer (desc) | entries[0]=rr-older (asc) | F3: ascending per OpenAPI spec |
| UT-RH-LOGIC-013 | summaries[0]=rr-t2-new (desc) | summaries[0]=rr-t2-old (asc) | F3: ascending per OpenAPI spec |
| UT-DS-616-003 | entries[0]=rr-post (desc) | entries[0]=rr-pre (asc) | F3: ascending per OpenAPI spec |

---

## 12. Execution

```bash
# All DS unit tests
go test ./test/unit/datastorage/... -ginkgo.v

# DS integration tests
go test ./test/integration/datastorage/... -ginkgo.v

# Focus on due diligence tests
go test ./test/unit/datastorage/... -ginkgo.v -ginkgo.focus="F[1-9]"

# Focus on modified sort order tests
go test ./test/unit/datastorage/... -ginkgo.v -ginkgo.focus="LOGIC-007|LOGIC-013|616-003"
```

---

## 13. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-04 | Initial test plan covering F1-F10 due diligence findings |
