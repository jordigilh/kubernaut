# Test Plan: RR Name in Notification Bodies (#626)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-626-v1.0
**Feature**: Include RemediationRequest name in all notification bodies for traceability
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/v1.2.0-rc4`

---

## 1. Introduction

### 1.1 Purpose

Validates that all notification types include the `RemediationRequest` CR name in their body text, enabling operators to directly trace notifications back to the specific CRD pipeline chain.

### 1.2 Objectives

1. **FormatRemediationLine helper**: Correctly formats `**Remediation**: {name}` with graceful empty handling
2. **All body builders**: All 5 `NotificationCreator` body builders include the RR name
3. **Timeout bodies**: Both inline timeout body builders include the RR name
4. **No regression**: Existing notification tests continue to pass

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/... -ginkgo.focus="626"` |
| Backward compatibility | 0 regressions | All existing notification tests pass |

---

## 2. References

### 2.1 Authority

- Issue #626: Notification include RR name for traceability
- BR-ORCH-001: Approval notification
- BR-ORCH-027: Global timeout
- BR-ORCH-028: Phase timeout

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| Issue #626 | FormatRemediationLine non-empty | P0 | Unit | UT-RO-626-001 | Pending |
| Issue #626 | FormatRemediationLine empty | P0 | Unit | UT-RO-626-002 | Pending |
| Issue #626 | Approval body contains RR name | P0 | Unit | UT-RO-626-003 | Pending |
| Issue #626 | Completion body contains RR name | P0 | Unit | UT-RO-626-004 | Pending |
| Issue #626 | Bulk duplicate body contains RR name | P0 | Unit | UT-RO-626-005 | Pending |
| Issue #626 | Manual review body contains RR name | P0 | Unit | UT-RO-626-006 | Pending |
| Issue #626 | Self-resolved body contains RR name | P0 | Unit | UT-RO-626-007 | Pending |
| Issue #626 | Global timeout body contains RR name | P0 | Unit | UT-RO-626-008 | Pending |
| Issue #626 | Phase timeout body contains RR name | P0 | Unit | UT-RO-626-009 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**File**: `test/unit/remediationorchestrator/notification_cluster_test.go` (extends #615 pattern)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-626-001` | `FormatRemediationLine("rr-abc")` returns `"**Remediation**: rr-abc\n\n"` | Pending |
| `UT-RO-626-002` | `FormatRemediationLine("")` returns empty string | Pending |
| `UT-RO-626-003` | Approval notification body contains RR name | Pending |
| `UT-RO-626-004` | Completion notification body contains RR name | Pending |
| `UT-RO-626-005` | Bulk duplicate notification body contains RR name | Pending |
| `UT-RO-626-006` | Manual review notification body contains RR name | Pending |
| `UT-RO-626-007` | Self-resolved notification body contains RR name | Pending |
| `UT-RO-626-008` | Global timeout notification body contains RR name | Pending |
| `UT-RO-626-009` | Phase timeout notification body contains RR name | Pending |

### Tier Skip Rationale

- **Integration/E2E**: Notification body content is unit-testable. Delivery rendering passes `Spec.Body` as-is.

---

## 10. Environmental Needs

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: fake K8s client
- **Location**: `test/unit/remediationorchestrator/`

---

## 11. Execution

```bash
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-626" -ginkgo.v
```

---

## 12. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| notification_cluster_test.go UT-RO-615-005..009 | `HavePrefix(clusterLine)` | None needed | `HavePrefix` still matches since cluster line remains first |
| notification_creator_test.go | Various `ContainSubstring` | May need mechanical updates if body structure changes | RR name line added before existing content |

---

## 13. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
