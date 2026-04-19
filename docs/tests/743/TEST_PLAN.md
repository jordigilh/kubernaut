# Test Plan: Dedup Timing Fields Dead Code (#743)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-743-v1
**Feature**: Wire DeduplicationWindowMinutes, FirstSeen, LastSeen through 4 layers so duplicate signal prompts have complete timing context
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/742-743-pdb-guidance-dedup-fields`

---

## 1. Introduction

### 1.1 Purpose

Validate that deduplication timing fields (`DeduplicationWindowMinutes`, `FirstSeen`,
`LastSeen`) are correctly wired from the OpenAPI `IncidentRequest` through
`MapIncidentRequestToSignal` → `signalToPrompt` → `RenderInvestigation` so that
when `IsDuplicate=true` and `OccurrenceCount>0`, the prompt renders complete timing data
instead of empty/zero values.

### 1.2 Objectives

1. **Handler wiring**: `MapIncidentRequestToSignal` maps all 3 dedup fields from `IncidentRequest` to `SignalContext`
2. **Prompt rendering**: `RenderInvestigation` populates `DeduplicationWindowMinutes`, `FirstSeen`, `LastSeen` into template data
3. **Negative case**: Fields remain zero/empty when not provided in request
4. **Template output**: Dedup section renders correct values when fields are populated

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/... -ginkgo.focus="743"` |
| Unit-testable code coverage | >=80% | Coverage of dedup wiring paths |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority

- Issue #743: Prompt builder DeduplicationWindowMinutes, FirstSeen, LastSeen never wired
- GAP-014: Deduplication fields from IncidentRequest OpenAPI schema

### 2.2 Cross-References

- Handler: `internal/kubernautagent/server/handler.go`
- Types: `internal/kubernautagent/types/types.go`
- Investigator: `internal/kubernautagent/investigator/investigator.go`
- Builder: `internal/kubernautagent/prompt/builder.go`
- Generated client: `pkg/agentclient/oas_schemas_gen.go`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | OptNilInt/OptNilString nil handling incorrect | Panic or wrong values | Low | UT-KA-743-005, UT-KA-743-006 | Follow existing IsDuplicate/OccurrenceCount pattern |
| R2 | Upstream BuildIncidentRequest never sends fields | Fields always empty in production | High | Out-of-scope | Documented as separate AA controller enhancement |
| R3 | Template renders misleading zero values when dedup inactive | Confusing LLM context | Low | UT-KA-743-002, UT-KA-743-003 | Template guard (IsDuplicate && OccurrenceCount > 0) already exists |

---

## 4. Scope

### 4.1 Features to be Tested

- **`SignalContext`** (`internal/kubernautagent/types/types.go`): New fields for dedup timing
- **`SignalData`** (`internal/kubernautagent/prompt/builder.go`): New fields for dedup timing
- **`MapIncidentRequestToSignal`** (`internal/kubernautagent/server/handler.go`): Map 3 new fields
- **`signalToPrompt`** (`internal/kubernautagent/investigator/investigator.go`): Map 3 new fields
- **`RenderInvestigation`** (`internal/kubernautagent/prompt/builder.go`): Populate template data

### 4.2 Features Not to be Tested

- **`BuildIncidentRequest`** (`pkg/aianalysis/handlers/request_builder.go`): Upstream caller does not read `RR.Status.Deduplication` — separate AA controller issue
- **Gateway deduplication logic**: Already tested separately

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of dedup wiring across all 4 layers

### 5.2 Pass/Fail Criteria

**PASS**: All 6 tests pass, no regressions.
**FAIL**: Any test fails or existing tests regress.

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| GAP-014 | Dedup timing fields in prompt | P0 | Unit | UT-KA-743-001 | Pending |
| GAP-014 | Dedup timing fields in prompt | P0 | Unit | UT-KA-743-002 | Pending |
| GAP-014 | Dedup timing fields in prompt | P0 | Unit | UT-KA-743-003 | Pending |
| GAP-014 | Dedup timing fields in prompt | P1 | Unit | UT-KA-743-004 | Pending |
| GAP-014 | Dedup timing fields in handler | P0 | Unit | UT-KA-743-005 | Pending |
| GAP-014 | Dedup timing fields in handler | P0 | Unit | UT-KA-743-006 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-743-001` | RenderInvestigation renders dedup section with correct FirstSeen, LastSeen, DeduplicationWindowMinutes when IsDuplicate=true, OccurrenceCount=3 | Pending |
| `UT-KA-743-002` | RenderInvestigation does NOT render dedup section when IsDuplicate=false | Pending |
| `UT-KA-743-003` | RenderInvestigation does NOT render dedup section when OccurrenceCount=0 (even if IsDuplicate=true) | Pending |
| `UT-KA-743-004` | Dedup fields render correctly with zero-value DeduplicationWindowMinutes (fallback behavior) | Pending |
| `UT-KA-743-005` | MapIncidentRequestToSignal maps deduplication_window_minutes, first_seen, last_seen from IncidentRequest to SignalContext | Pending |
| `UT-KA-743-006` | MapIncidentRequestToSignal leaves dedup fields empty when not set in request | Pending |

### Tier Skip Rationale

- **Integration**: No I/O involved — pure field mapping and template rendering. Unit tests provide full coverage.
- **E2E**: Deferred until upstream `BuildIncidentRequest` is wired.

---

## 9. Test Cases

### UT-KA-743-001: Dedup section renders with timing fields

**BR**: GAP-014
**Priority**: P0
**File**: `test/unit/kubernautagent/prompt/builder_test.go`

**Test Steps**:
1. **Given**: SignalData with IsDuplicate=true, OccurrenceCount=3, DeduplicationWindowMinutes=30, FirstSeen="2026-04-01T10:00:00Z", LastSeen="2026-04-01T10:30:00Z"
2. **When**: `RenderInvestigation` is called
3. **Then**: Output contains "First Seen: 2026-04-01T10:00:00Z", "Last Seen: 2026-04-01T10:30:00Z", "Deduplication Window: 30 minutes"

### UT-KA-743-005: Handler maps dedup timing fields

**BR**: GAP-014
**Priority**: P0
**File**: `test/unit/kubernautagent/server/adversarial_http_test.go`

**Test Steps**:
1. **Given**: IncidentRequest with DeduplicationWindowMinutes=60, FirstSeen="2026-04-01T10:00:00Z", LastSeen="2026-04-01T11:00:00Z"
2. **When**: `MapIncidentRequestToSignal` is called
3. **Then**: SignalContext.DeduplicationWindowMinutes=60, FirstSeen and LastSeen match

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: None required
- **Location**: `test/unit/kubernautagent/prompt/`, `test/unit/kubernautagent/server/`

---

## 13. Execution

```bash
go test ./test/unit/kubernautagent/prompt/... -ginkgo.focus="743" -ginkgo.v
go test ./test/unit/kubernautagent/server/... -ginkgo.focus="743" -ginkgo.v
```

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan |
