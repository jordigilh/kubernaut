# Test Plan: Notification Body Field Reordering (#627)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-627-v1.0
**Feature**: Reorder notification body fields for faster operator triage
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/v1.2.0-rc4`

---

## 1. Introduction

### 1.1 Purpose

Validates that Completion, Manual Review, and Approval notification bodies place actionable information (Outcome, Call-to-Action) near the top of the message, before detail sections (RCA, warnings), reducing time-to-action for operators scanning notifications.

### 1.2 Objectives

1. **Completion**: `Outcome` field appears before `Signal` field
2. **Manual Review**: "Action Required" block appears before "Failure Source"
3. **Approval**: "approve/reject" prompt appears before "Selection Rationale"
4. **No regression**: Bulk Duplicate, Self-Resolved, and Timeout body structures unchanged

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/... -ginkgo.focus="627"` |
| Backward compatibility | 0 regressions in unaffected notification types |

---

## 2. References

### 2.1 Authority

- Issue #627: Notification body field reordering for faster operator triage
- Issue #628: (v1.3) Standardized Status Block (deferred)

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| Issue #627 | Completion Outcome before Signal | P0 | Unit | UT-RO-627-001 | Pending |
| Issue #627 | Manual Review CTA before Failure Source | P0 | Unit | UT-RO-627-002 | Pending |
| Issue #627 | Approval prompt before Rationale | P0 | Unit | UT-RO-627-003 | Pending |
| Issue #627 | Unaffected types unchanged | P1 | Unit | UT-RO-627-004 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**File**: `test/unit/remediationorchestrator/notification_body_order_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-627-001` | Completion body: `Outcome` appears before `Signal` | Pending |
| `UT-RO-627-002` | Manual Review body: "Action Required" appears before "Failure Source" | Pending |
| `UT-RO-627-003` | Approval body: "approve/reject" appears before "Selection Rationale" | Pending |
| `UT-RO-627-004` | Bulk Duplicate, Self-Resolved bodies contain expected fields (regression guard) | Pending |

### Tier Skip Rationale

- **Integration/E2E**: Body field ordering is unit-testable via string position assertions.

---

## 9. Test Cases

### UT-RO-627-001: Completion Outcome before Signal

**Priority**: P0
**Type**: Unit

**Test Steps**:
1. **Given**: A NotificationCreator that creates a Completion notification
2. **When**: The notification body is generated
3. **Then**: The index of `**Outcome**:` in the body is less than the index of `**Signal**:`

### UT-RO-627-002: Manual Review CTA before Failure Source

**Test Steps**:
1. **Given**: A NotificationCreator that creates a Manual Review notification
2. **When**: The notification body is generated
3. **Then**: The index of `**Action Required**:` is less than the index of `**Failure Source**:`

### UT-RO-627-003: Approval prompt before Rationale

**Test Steps**:
1. **Given**: A NotificationCreator that creates an Approval notification with a rationale
2. **When**: The notification body is generated
3. **Then**: The index of `approve/reject` is less than the index of `Selection Rationale`

---

## 10. Environmental Needs

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: fake K8s client
- **Location**: `test/unit/remediationorchestrator/`

---

## 11. Execution

```bash
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-627" -ginkgo.v
```

---

## 12. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| notification_creator_test.go (section slicing) | Body[detailsIdx:warningsIdx] | May need index recalculation | Field reordering shifts string positions |
| notification_creator_test.go (Completion) | `ContainSubstring("Outcome")` | Position validation added | Outcome moved up |

---

## 13. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
