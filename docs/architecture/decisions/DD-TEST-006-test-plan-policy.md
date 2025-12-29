# DD-TEST-006: Test Plan Policy for New Services

## Status
**✅ APPROVED**
**Version**: 1.0
**Decision Date**: December 17, 2025
**Effective**: V1.1 onwards
**Authority Level**: TESTING METHODOLOGY

---

## 🚨 ENFORCEMENT SCOPE

| Version | Enforcement |
|---------|-------------|
| **V1.0** | ⚠️ **NOT ENFORCED** - Existing services grandfathered |
| **V1.1+** | ✅ **MANDATORY** - All new services must comply |

**Rationale**: V1.0 services were developed without formal test plans. Retroactive enforcement would delay V1.0 release. Starting V1.1, all new services MUST have test plans.

---

## Context

During V1.0 development, tests were created alongside implementation without formal test plans. This resulted in:

1. **Non-Standard Test Identifiers**: Ad-hoc patterns like `PE-ER-02`, `EC-HP-06`, `CL-SEC-01` instead of BR-* format
2. **Untraceable Tests**: ~63% of SignalProcessing unit test identifiers cannot be traced to business requirements
3. **Coverage Gaps**: No systematic way to verify all BRs have corresponding tests
4. **Review Difficulty**: Code reviewers cannot validate test completeness against requirements

**Discovery**: December 17, 2025 (TRIAGE_ADR_032_COMPLIANCE_DEC_17_2025.md)

---

## Decision

### V1.1+ Mandate: Test Plans Required for New Services

**REQUIREMENT**: Every new service (V1.1 onwards) MUST have a formal Test Plan document created **BEFORE** implementation begins.

---

## Test Plan Specification

### Document Location

```
docs/services/{service-type}/{service-name}/TEST_PLAN.md
```

**Examples**:
- `docs/services/crd-controllers/05-new-controller/TEST_PLAN.md`
- `docs/services/stateless/new-service/TEST_PLAN.md`

### Required Sections

#### 1. Business Requirements Coverage Matrix

```markdown
## BR Coverage Matrix

| BR ID | Description | Test Type | Test ID | Status |
|-------|-------------|-----------|---------|--------|
| BR-NEW-001 | Feature description | Unit | UT-NEW-001-01 | ⏸️ Pending |
| BR-NEW-001 | Feature description | Integration | IT-NEW-001-01 | ⏸️ Pending |
| BR-NEW-002 | Another feature | Unit | UT-NEW-002-01 | ⏸️ Pending |
```

#### 2. Test Identifier Convention

**Format**: `{TestType}-{ServiceCode}-{BR#}-{Sequence}`

| Component | Values | Example |
|-----------|--------|---------|
| **TestType** | `UT` (Unit), `IT` (Integration), `E2E` (End-to-End) | `UT` |
| **ServiceCode** | Service abbreviation (e.g., `SP`, `WE`, `RO`, `NOT`) | `SP` |
| **BR#** | BR number (zero-padded 3 digits) | `070` |
| **Sequence** | Test sequence within BR (zero-padded 2 digits) | `01` |

**Examples**:
- `UT-SP-070-01` - Unit test #1 for BR-SP-070
- `IT-WE-005-03` - Integration test #3 for BR-WE-005
- `E2E-RO-102-01` - E2E test #1 for BR-RO-102

#### 3. Test Categories

```markdown
## Test Categories

### Happy Path Tests
Tests that validate expected behavior with valid inputs.

### Error Handling Tests
Tests that validate behavior with invalid inputs, timeouts, failures.

### Security Tests
Tests that validate security controls, input validation, authorization.

### Performance Tests
Tests that validate SLO compliance (latency, throughput).
```

#### 4. Coverage Requirements

```markdown
## Coverage Requirements

| Test Type | Minimum Coverage | Target Coverage |
|-----------|------------------|-----------------|
| Unit | 70% | 85% |
| Integration | 50% | 70% |
| E2E | 10% | 15% |

## BR Coverage Target
- 100% of BRs must have at least one test
- Critical BRs (P0) must have happy path + error handling tests
```

---

## Test Plan Template

```markdown
# Test Plan: {Service Name}

**Service**: {service-name}
**Version**: 1.0
**Created**: {date}
**Author**: {author}
**Status**: Draft | Review | Approved

---

## 1. Scope

### In Scope
- BR-{SERVICE}-001: {description}
- BR-{SERVICE}-002: {description}

### Out of Scope
- {items explicitly not tested}

---

## 2. BR Coverage Matrix

| BR ID | Description | Priority | Test Type | Test ID | Status |
|-------|-------------|----------|-----------|---------|--------|
| BR-{SVC}-001 | {desc} | P0 | Unit | UT-{SVC}-001-01 | ⏸️ |
| BR-{SVC}-001 | {desc} | P0 | Integration | IT-{SVC}-001-01 | ⏸️ |

---

## 3. Test Cases

### UT-{SVC}-001-01: {Test Name}
**BR**: BR-{SVC}-001
**Type**: Unit
**Category**: Happy Path
**Description**: {what is being tested}
**Preconditions**: {setup required}
**Steps**:
1. {step 1}
2. {step 2}
**Expected Result**: {expected outcome}
**Actual Result**: ⏸️ Pending

---

## 4. Coverage Targets

| Metric | Target | Actual |
|--------|--------|--------|
| Unit Test Coverage | 70% | ⏸️ |
| BR Coverage | 100% | ⏸️ |
| Critical Path Coverage | 100% | ⏸️ |

---

## 5. Sign-off

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Author | | | ⏸️ |
| Reviewer | | | ⏸️ |
| Approver | | | ⏸️ |
```

---

## Integration with Development Workflow

### Pre-Implementation Checkpoint

**BEFORE** creating any test files for a new service:

```
✅ TEST PLAN CHECKPOINT:
- [ ] TEST_PLAN.md exists in service docs directory
- [ ] All BRs listed in coverage matrix
- [ ] Test IDs assigned in {TestType}-{SVC}-{BR#}-{Seq} format
- [ ] Coverage targets defined

❌ STOP: Cannot create test files until ALL checkboxes are ✅
```

### Implementation Phase

1. **Create Test Plan** → Review → Approve
2. **Implement Tests** using assigned Test IDs
3. **Update Status** in coverage matrix as tests pass
4. **Verify Coverage** meets targets before release

### Code Review Validation

Reviewers MUST verify:
- [ ] Test file uses Test IDs from approved TEST_PLAN.md
- [ ] Test descriptions reference BR IDs
- [ ] Coverage matrix updated with test status

---

## V1.0 Grandfather Clause

### Existing Services (V1.0)

The following services are **exempt** from this policy:

| Service | Reason |
|---------|--------|
| SignalProcessing | V1.0 service - tests already written |
| WorkflowExecution | V1.0 service - tests already written |
| RemediationOrchestrator | V1.0 service - tests already written |
| AIAnalysis | V1.0 service - tests already written |
| Notification | V1.0 service - tests already written |
| Gateway | V1.0 service - tests already written |
| DataStorage | V1.0 service - tests already written |

### Future Remediation (Optional)

V1.0 services MAY be retroactively updated to comply with this policy:
- **Priority**: P4 (after V1.0 release)
- **Effort**: ~2-4 hours per service
- **Benefit**: Improved traceability and coverage visibility

---

## Enforcement

### V1.1+ Services

| Checkpoint | Enforcement |
|------------|-------------|
| **PR Creation** | TEST_PLAN.md must exist before test files |
| **Code Review** | Reviewer validates Test ID compliance |
| **CI Pipeline** | (Future) Automated Test ID validation |

### Violation Handling

```
🚨 DD-TEST-006 VIOLATION: Test files created without approved TEST_PLAN.md
- Block PR merge until TEST_PLAN.md is created and approved
- Retroactively assign Test IDs to existing tests
```

---

## Related Documents

- [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc) - Testing framework
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) - BR reference requirements
- [00-core-development-methodology.mdc](mdc:.cursor/rules/00-core-development-methodology.mdc) - TDD methodology

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-12-17 | AI Assistant | Initial policy - effective V1.1+ |

---

## Approval

**Decision Approved By**: Project Lead
**Approval Date**: December 17, 2025
**Effective Version**: V1.1 onwards



