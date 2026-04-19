# Test Plan: EM False-Positive Remediated Outcome Fix

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-722-v1
**Feature**: Score-aware RR outcome derivation, RO audit data completeness, KA outcome mapping
**Version**: 1.0
**Created**: 2026-04-18
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/722-691-outcome-rename`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the fix for issue #722: the EffectivenessMonitor declares
`Remediated` with `alertScore=0` while the alert is still firing. The fix spans three
layers: RO outcome derivation, RO audit data completeness, and KA recurring pattern
detection.

### 1.2 Objectives

1. **Score-aware outcome**: `completeVerificationIfNeeded` derives outcome from EA component scores — `alertAssessed && alertScore == 0` always yields `Inconclusive`
2. **Audit completeness**: `BuildRemediationWorkflowCreatedEvent` emits `signal_type` and `signal_fingerprint`; `BuildCompletionEvent` emits actual CRD outcome
3. **KA detection**: `DetectCompletedButRecurring` and `AllZeroEffectiveness` recognize `Remediated` and `Inconclusive` as completed outcomes

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/... ./test/unit/kubernautagent/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on outcome derivation + history detection |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-EM-012: EM scoring must reflect actual signal state
- BR-ORCH-042: RO must escalate after repeated ineffective remediations
- BR-AI-056: KA must detect recurring ineffective patterns in remediation history
- ADR-EM-001: EM collects; DS computes weighted score
- DD-AUDIT-003: Audit event payload requirements
- Issue #722: EM declares Remediated with alertScore=0

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | CRD enum rejects Inconclusive | Outcome not persisted | High if missed | UT-RO-722-001..003 | Update kubebuilder enum + 3 CRD YAMLs |
| R2 | EA not in scope for outcome derivation | Cannot read scores | Low | UT-RO-722-001..003 | EA already fetched in trackEffectivenessStatus; pass to helper |
| R3 | Audit signal_type empty breaks DS correlation | History entries lack signal context | Medium | UT-RO-722-004..005 | Populate in BuildRemediationWorkflowCreatedEvent |
| R4 | KA misses Remediated outcomes | No escalation detection | High | UT-KA-722-001..004 | Add to completedOutcomes map |

---

## 4. Scope

### 4.1 Features to be Tested

- **Outcome derivation** (`internal/controller/remediationorchestrator/effectiveness_tracking.go`): Score-aware outcome assignment
- **Audit builders** (`pkg/remediationorchestrator/audit/manager.go`): signal_type, signal_fingerprint, crd_outcome population
- **KA history detection** (`internal/kubernautagent/prompt/history.go`): Expanded completedOutcomes

### 4.2 Features Not to be Tested

- EM scoring logic (correct — alertScore=0 already means "still firing")
- DS CorrelateTier1Chain (already reads fields from event_data — no code change)
- Gateway dedup (Inconclusive correctly allows new RRs)

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| alertAssessed && alertScore==0 -> Inconclusive (unconditional) | Alert is ground truth; health/metrics improvement without alert resolution is transient |
| !alertAssessed -> Remediated (fail-open) | Preserves current behavior when AM is unavailable; WFE succeeded |
| Add Inconclusive to CRD enum | Clean API surface; no backwards compatibility needed |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of outcome derivation logic, audit builder signal population, KA detection
- **Integration**: Deferred — DS correlation already works when data is populated

### 5.2 Pass/Fail Criteria

**PASS**: All P0 tests pass, existing RO/KA tests pass, per-tier coverage >=80%.
**FAIL**: Any P0 test fails, existing tests regress, coverage below 80%.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/effectiveness_tracking.go` | `completeVerificationIfNeeded`, `DeriveOutcomeFromEA` | ~40 |
| `pkg/remediationorchestrator/audit/manager.go` | `BuildRemediationWorkflowCreatedEvent`, `BuildCompletionEvent` | ~80 |
| `internal/kubernautagent/prompt/history.go` | `DetectCompletedButRecurring`, `AllZeroEffectiveness` | ~65 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-EM-012 | alertScore=0 must not yield Remediated | P0 | Unit | UT-RO-722-001 | Pending |
| BR-EM-012 | alertScore>0 yields Remediated | P0 | Unit | UT-RO-722-002 | Pending |
| BR-EM-012 | !alertAssessed fails open to Remediated | P0 | Unit | UT-RO-722-003 | Pending |
| BR-ORCH-042 | signal_type populated in workflow_created event | P0 | Unit | UT-RO-722-004 | Pending |
| BR-ORCH-042 | crd_outcome populated in completion event | P0 | Unit | UT-RO-722-005 | Pending |
| BR-AI-056 | Remediated detected as completed outcome | P0 | Unit | UT-KA-722-001 | Pending |
| BR-AI-056 | Inconclusive detected as completed outcome | P0 | Unit | UT-KA-722-002 | Pending |
| BR-AI-056 | AllZeroEffectiveness includes Remediated | P0 | Unit | UT-KA-722-003 | Pending |
| BR-AI-056 | AllZeroEffectiveness includes Inconclusive | P0 | Unit | UT-KA-722-004 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-722-001` | RR outcome is Inconclusive when alertAssessed=true, alertScore=0 | Pending |
| `UT-RO-722-002` | RR outcome is Remediated when alertAssessed=true, alertScore=1.0 | Pending |
| `UT-RO-722-003` | RR outcome is Remediated when alertAssessed=false (fail-open) | Pending |
| `UT-RO-722-004` | BuildRemediationWorkflowCreatedEvent includes signal_type and signal_fingerprint | Pending |
| `UT-RO-722-005` | BuildCompletionEvent includes crd_outcome reflecting actual RR outcome | Pending |
| `UT-KA-722-001` | DetectCompletedButRecurring detects entries with outcome=Remediated | Pending |
| `UT-KA-722-002` | DetectCompletedButRecurring detects entries with outcome=Inconclusive | Pending |
| `UT-KA-722-003` | AllZeroEffectiveness includes entries with outcome=Remediated | Pending |
| `UT-KA-722-004` | AllZeroEffectiveness includes entries with outcome=Inconclusive | Pending |

---

## 9. Test Cases

### UT-RO-722-001: alertScore=0 yields Inconclusive

**BR**: BR-EM-012
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/effectiveness_outcome_test.go`

**Test Steps**:
1. **Given**: EA with `Components.AlertAssessed=true`, `Components.AlertScore=ptr(0.0)`, `Components.HealthScore=ptr(0.75)`
2. **When**: `DeriveOutcomeFromEA(ea)` is called
3. **Then**: Returns `"Inconclusive"`

**Acceptance Criteria**:
- Alert firing overrides any positive health/metrics signal
- Outcome string matches CRD enum value exactly

### UT-RO-722-002: alertScore>0 yields Remediated

**BR**: BR-EM-012
**Priority**: P0
**File**: `test/unit/remediationorchestrator/effectiveness_outcome_test.go`

**Test Steps**:
1. **Given**: EA with `Components.AlertAssessed=true`, `Components.AlertScore=ptr(1.0)`
2. **When**: `DeriveOutcomeFromEA(ea)` is called
3. **Then**: Returns `"Remediated"`

### UT-RO-722-003: !alertAssessed fails open

**BR**: BR-EM-012
**Priority**: P0
**File**: `test/unit/remediationorchestrator/effectiveness_outcome_test.go`

**Test Steps**:
1. **Given**: EA with `Components.AlertAssessed=false`, `Components.AlertScore=nil`
2. **When**: `DeriveOutcomeFromEA(ea)` is called
3. **Then**: Returns `"Remediated"` (fail-open: no evidence against success)

### UT-RO-722-004: signal_type in workflow_created event

**BR**: BR-ORCH-042
**Priority**: P0
**File**: `test/unit/remediationorchestrator/audit_manager_test.go`

**Test Steps**:
1. **Given**: Signal type "alert", fingerprint "hash-abc123"
2. **When**: `BuildRemediationWorkflowCreatedEvent(correlationID, ns, rrName, preHash, target, wfID, wfVersion, actionType, signalType, signalFingerprint)` is called
3. **Then**: Event payload contains `signal_type="alert"` and `signal_fingerprint="hash-abc123"`

### UT-RO-722-005: crd_outcome in completion event

**BR**: BR-ORCH-042
**Priority**: P0
**File**: `test/unit/remediationorchestrator/audit_manager_test.go`

**Test Steps**:
1. **Given**: CRD outcome "Inconclusive"
2. **When**: `BuildCompletionEvent(correlationID, ns, rrName, "Inconclusive", durationMs)` is called
3. **Then**: Event payload contains `crd_outcome="Inconclusive"`

### UT-KA-722-001: DetectCompletedButRecurring with Remediated

**BR**: BR-AI-056
**Priority**: P0
**File**: `test/unit/kubernautagent/prompt/history_test.go`

**Test Steps**:
1. **Given**: 3 history entries with outcome="Remediated", same actionType+signalType
2. **When**: `DetectCompletedButRecurring(entries, 2)` is called
3. **Then**: Returns RecurringPattern with count=3

### UT-KA-722-002: DetectCompletedButRecurring with Inconclusive

**BR**: BR-AI-056
**Priority**: P0
**File**: `test/unit/kubernautagent/prompt/history_test.go`

**Test Steps**:
1. **Given**: 2 entries with outcome="Inconclusive", same actionType+signalType
2. **When**: `DetectCompletedButRecurring(entries, 2)` is called
3. **Then**: Returns RecurringPattern with count=2

### UT-KA-722-003: AllZeroEffectiveness with Remediated

**BR**: BR-AI-056
**Priority**: P0
**File**: `test/unit/kubernautagent/prompt/history_test.go`

**Test Steps**:
1. **Given**: 2 entries with outcome="Remediated", effectivenessScore=0, signalResolved=false
2. **When**: `AllZeroEffectiveness(entries, actionType, signalType)` is called
3. **Then**: Returns true

### UT-KA-722-004: AllZeroEffectiveness with Inconclusive

**BR**: BR-AI-056
**Priority**: P0
**File**: `test/unit/kubernautagent/prompt/history_test.go`

**Test Steps**:
1. **Given**: 2 entries with outcome="Inconclusive", effectivenessScore=nil, signalResolved=false
2. **When**: `AllZeroEffectiveness(entries, actionType, signalType)` is called
3. **Then**: Returns true

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: EA CRD struct (no K8s API needed for pure logic tests)
- **Location**: `test/unit/remediationorchestrator/`, `test/unit/kubernautagent/prompt/`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available |
|------------|------|--------|-------------------------|
| OpenAPI spec update | Schema | This PR | UT-RO-722-004/005 need ogen types |
| CRD enum update | API | This PR | Outcome value rejected by apiserver |

### 11.2 Execution Order

1. **Phase 1 (RED)**: Write all 9 failing tests
2. **Phase 2 (GREEN)**: OpenAPI + ogen + CRD enum + implementation
3. **Phase 3 (REFACTOR)**: Extract `DeriveOutcomeFromEA`, clean signatures

---

## 12. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/tests/722/TEST_PLAN.md` |
| Unit tests (RO) | `test/unit/remediationorchestrator/effectiveness_outcome_test.go` |
| Unit tests (audit) | `test/unit/remediationorchestrator/audit_manager_test.go` |
| Unit tests (KA) | `test/unit/kubernautagent/prompt/history_test.go` |

---

## 13. Execution

```bash
# Unit tests (RO outcome)
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-722"

# Unit tests (KA history)
go test ./test/unit/kubernautagent/prompt/... -ginkgo.focus="UT-KA-722"

# All #722 tests
go test ./test/unit/remediationorchestrator/... ./test/unit/kubernautagent/prompt/... -ginkgo.focus="722"
```

---

## 14. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-18 | Initial test plan |
