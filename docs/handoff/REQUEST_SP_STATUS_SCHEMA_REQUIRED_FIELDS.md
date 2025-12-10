# 📋 REQUEST: SignalProcessing Status Schema - Required Fields Clarification

**From**: Remediation Orchestrator Team
**To**: SignalProcessing Team
**Date**: December 10, 2025
**Priority**: 🟡 MEDIUM
**Status**: ⏳ **AWAITING SP TEAM RESPONSE**

---

## 📋 Summary

RO integration tests fail when simulating SignalProcessing status updates because the SP CRD schema requires fields that aren't being populated.

---

## 🔴 Issue Details

### Error Message

```
SignalProcessing.signalprocessing.kubernaut.ai "sp-rr-phase-xxx" is invalid:
  [status.priorityAssignment.assignedAt: Required value,
   status.environmentClassification.classifiedAt: Required value]
```

### Context

RO creates SignalProcessing CRDs and waits for them to complete. In integration tests, we simulate SP completion by updating the SP status. However, the SP CRD schema requires these fields when updating status:

- `status.priorityAssignment.assignedAt` (Required)
- `status.environmentClassification.classifiedAt` (Required)

### Affected RO Tests

| Test | File | Line |
|------|------|------|
| Phase progression | `lifecycle_test.go` | 158 |
| ManualReview flow | `lifecycle_test.go` | 221, 290 |
| Approval flow | `lifecycle_test.go` | 365, 428 |

---

## ❓ Questions for SP Team

### Q1: Should these fields be required?

When a test or external system updates SP status to simulate completion, must it also provide:
- `status.priorityAssignment.assignedAt`
- `status.environmentClassification.classifiedAt`

**RO's Perspective**: These seem like SP-internal fields that SP controller should manage, not external consumers updating status.

### Q2: Recommended approach?

**Option A**: Make fields optional in SP CRD schema
- Pros: Simpler for consumers, follows Kubernetes conventions
- Cons: May require SP logic changes

**Option B**: SP provides test utilities
- SP team provides helper functions that set these required fields
- RO imports and uses these in integration tests

**Option C**: Document required fields for status updates
- SP documents what fields are required when updating status
- RO tests comply with these requirements

---

## 📊 Impact

| Metric | Value |
|--------|-------|
| RO Integration Tests Blocked | 4 tests |
| Severity | Medium (tests only, production RO doesn't update SP status) |

---

## 🔗 Related Documents

- RO Integration Tests: `test/integration/remediationorchestrator/lifecycle_test.go`
- SP CRD Schema: `api/signalprocessing/v1alpha1/signalprocessing_types.go`

---

## 📞 Response Requested

Please respond with:
1. Your preferred option (A/B/C) or alternative
2. Timeline for implementation (if needed)
3. Any clarifying questions

---

**Document Version**: 1.0
**Created**: December 10, 2025
**Maintained By**: Remediation Orchestrator Team

