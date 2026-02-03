# AuthWebhook RAR Integration Tests - Table-Driven Refactor

**Date**: February 3, 2026  
**Status**: ✅ **Refactored - Ready for Validation**  
**Related**: BR-AUDIT-006, DD-WEBHOOK-003  
**Priority**: P0 (SOC 2 Compliance Critical)

---

## Executive Summary

Refactored AuthWebhook RemediationApprovalRequest integration tests to use table-driven pattern with business outcome validation focus, addressing user feedback about RAR audit trail testing requirements.

**Key Insight**: AuthWebhook is the **source of truth** for `DecidedBy` field (authenticated user identity) - critical for SOC 2 CC8.1 compliance.

---

## What Changed

### 1. Converted to Table-Driven Pattern

**Before** (Context-based):
```go
Context("INT-RAR-01: when operator approves remediation request", func() {
    It("should capture operator identity via webhook", func() {
        // Test implementation
    })
})

Context("INT-RAR-02: when operator rejects remediation request", func() {
    It("should capture operator identity for rejection", func() {
        // Test implementation
    })
})
```

**After** (Table-driven):
```go
DescribeTable("Approval Decision Scenarios - SOC 2 Compliance Validation",
    func(scenario ApprovalDecisionScenario) {
        // Unified test logic with business outcome validation
    },
    Entry("INT-RAR-01: Operator approves production remediation", ...),
    Entry("INT-RAR-02: Operator rejects risky remediation", ...),
)
```

---

### 2. Added Business Outcome Validation

**Before** (Technical Focus):
```go
Expect(rar.Status.DecidedBy).ToNot(BeEmpty(),
    "DecidedBy should be populated by webhook with K8s UserInfo")
```

**After** (Business Outcome):
```go
Expect(rar.Status.DecidedBy).ToNot(BeEmpty(),
    "COMPLIANCE FAILURE: Missing WHO - cannot satisfy SOC 2 CC8.1")

Expect(rar.Status.DecidedBy).To(Or(
    Equal("admin"),
    ContainSubstring("system:serviceaccount")),
    "BUSINESS OUTCOME: Authenticated user identity is valid K8s UserInfo")
```

---

### 3. Added 3 New Tests

**NEW: INT-RAR-04 - Identity Forgery Prevention**:
- **Business Risk**: Operator can frame another operator by setting `DecidedBy`
- **Test**: User tries to set `DecidedBy="malicious-user@example.com"`
- **Expected**: Webhook OVERWRITES with authenticated identity
- **Compliance**: SOC 2 CC8.1 (tamper-proof attribution)

**NEW: INT-RAR-05 - Webhook Audit Event Emission**:
- **Business Outcome**: Webhook authentication has its own audit trail
- **Test**: Query DataStorage for webhook audit event (event_category="webhook")
- **Expected**: Event exists with `actor_id` matching authenticated user
- **Compliance**: DD-WEBHOOK-003 (Webhook-Complete Audit Pattern)

**NEW: INT-RAR-06 - DecidedBy Preservation for RO Audit**:
- **Business Flow**: AuthWebhook sets `DecidedBy` → RO controller reads → RO emits approval audit
- **Test**: Verify `DecidedBy` field is accessible after webhook mutation
- **Expected**: Field contains authenticated user (not self-reported)
- **Compliance**: SOC 2 CC8.1 (cross-service attribution)

---

## Test Matrix

| Test ID | Business Outcome | Auditor Question | Compliance | Pattern |
|---------|------------------|------------------|------------|---------|
| INT-RAR-01 | User Attribution | "WHO approved this remediation?" | SOC 2 CC8.1 | Table-driven |
| INT-RAR-02 | Non-Repudiation | "WHY was it rejected?" | SOC 2 CC6.8 | Table-driven |
| INT-RAR-03 | Audit Trail Integrity | "Are decisions valid?" | SOC 2 CC7.4 | Context |
| INT-RAR-04 | Forgery Prevention | "Can identity be forged?" | SOC 2 CC8.1 | Context |
| INT-RAR-05 | Webhook Audit Trail | "Is webhook auth audited?" | DD-WEBHOOK-003 | Context |
| INT-RAR-06 | Cross-Service Attribution | "Is identity preserved for RO?" | SOC 2 CC8.1 | Context |

---

## Architectural Insight: Two Audit Events Per Approval

**Critical Discovery**: Each approval decision generates **TWO separate audit events**:

| Service | Event Category | Purpose | Fields |
|---------|---------------|---------|--------|
| **AuthWebhook** | `webhook` | Captures WHO (authenticated user) | `actor_id`, `actor_type`, `event_action="approval_decided"` |
| **RemediationOrchestrator** | `approval` | Captures complete approval context | `workflow_id`, `confidence`, `decision_message`, `decided_by` |

**Why Two Events?**:
- **AuthWebhook**: Tamper-proof authentication at webhook interception
- **RO Controller**: Complete business context for forensic investigation

**Integration Point**: `DecidedBy` field in RAR status bridges both events.

---

## Business Outcome Validation Examples

### Example 1: Identity Forgery Prevention (INT-RAR-04)

```go
// BUSINESS RISK: Operator frames another operator
forgedIdentity := "malicious-user@example.com"
rar.Status.DecidedBy = forgedIdentity  // USER TRIES TO SET THIS

updateStatusAndWaitForWebhook(...)

// SECURITY VALIDATION: Webhook OVERWRITES user-provided value
Expect(rar.Status.DecidedBy).ToNot(Equal(forgedIdentity),
    "SECURITY FAILURE: User was able to forge identity")

Expect(rar.Status.DecidedBy).To(Equal("admin"),
    "SECURITY OUTCOME: DecidedBy is from webhook authentication")
```

**Business Value**: Prevents operator from framing colleagues. Critical for SOC 2 CC8.1.

---

### Example 2: Webhook Audit Event Emission (INT-RAR-05)

```go
// BUSINESS OUTCOME: Webhook authentication has its own audit trail

// Query DataStorage for webhook audit events
events, err := queryAuditEvents(dsClient, rar.Name, nil)

// Filter for webhook category
var webhookEvents []string
for _, event := range events {
    if string(event.EventCategory) == "webhook" {
        webhookEvents = append(webhookEvents, event.EventType)
    }
}

Expect(webhookEvents).ToNot(BeEmpty(),
    "COMPLIANCE FAILURE: No webhook audit event (DD-WEBHOOK-003)")
```

**Business Value**: Proves authentication step was audited. Critical for forensic investigation.

---

## Files Modified

**Modified**:
- `test/integration/authwebhook/remediationapprovalrequest_test.go` (~400 LOC)
  - Refactored existing 3 tests to business outcome focus
  - Converted to table-driven pattern (2 scenarios)
  - Added 3 new tests (INT-RAR-04, 05, 06)

**Unchanged** (already correct):
- `test/integration/authwebhook/helpers.go` - Query helpers already support DataStorage queries
- `pkg/authwebhook/remediationapprovalrequest_handler.go` - Handler implementation correct
- `cmd/authwebhook/main.go` - Webhook registration correct

---

## Compliance Mapping

### SOC 2 CC8.1 (User Attribution)

**Requirements**:
- Capture WHO made approval decision
- Identity must be authenticated (not self-reported)
- Identity must be tamper-proof

**Tests**:
- ✅ INT-RAR-01: Captures authenticated user identity
- ✅ INT-RAR-04: Prevents identity forgery
- ✅ INT-RAR-06: Preserves identity for cross-service audit

---

### SOC 2 CC6.8 (Non-Repudiation)

**Requirements**:
- Prove operator made decision
- Capture operator's rationale
- Tamper-proof evidence

**Tests**:
- ✅ INT-RAR-02: Captures rejection rationale
- ✅ INT-RAR-05: Webhook audit event (tamper-proof)

---

### DD-WEBHOOK-003 (Webhook-Complete Audit Pattern)

**Requirements**:
- Webhook emits complete audit event
- Event includes authenticated user
- Event stored in DataStorage

**Tests**:
- ✅ INT-RAR-05: Webhook audit event emission

---

## Testing Strategy

### Table-Driven Tests (2 scenarios)

**Use for**: Common approval/rejection flows with similar structure

**Scenarios**:
- INT-RAR-01: Operator approves
- INT-RAR-02: Operator rejects

**Benefits**:
- Single test logic (DRY principle)
- Easy to add new scenarios
- Consistent validation

---

### Context-Based Tests (4 scenarios)

**Use for**: Specialized validation requiring unique setup

**Scenarios**:
- INT-RAR-03: Invalid decision validation
- INT-RAR-04: Identity forgery prevention
- INT-RAR-05: Webhook audit event emission
- INT-RAR-06: Cross-service attribution

**Benefits**:
- Flexible setup for unique scenarios
- Clear test isolation
- Better for infrastructure tests

---

## Next Steps

### 1. Run Refactored Tests

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
make test-integration-authwebhook
```

**Expected**: 6/6 PASSING (2 table-driven + 4 context-based)

---

### 2. Update RAR Audit Test Plan

**Action**: Update `TEST_PLAN_BR_AUDIT_006_RAR_AUDIT_TRAIL_V1_0.md` to document:
- AuthWebhook integration tests (6 tests)
- Cross-service audit trail (webhook → RO)
- Identity forgery prevention

---

### 3. Implement RO Controller Audit Integration

**Next Phase**: Integration tests for RO controller that:
- Reads `DecidedBy` from RAR status
- Emits approval audit event (event_category="approval")
- Includes authenticated user in `actor_id` field

---

## Questions & Answers

**Q: Why are AuthWebhook tests critical for BR-AUDIT-006?**
A: AuthWebhook is the **source of truth** for authenticated user identity (`DecidedBy` field). Without AuthWebhook tests, we cannot prove SOC 2 CC8.1 compliance.

**Q: Why two audit events (webhook + approval)?**
A: Separation of concerns - webhook proves WHO (authentication), RO proves WHAT (business context). Both are needed for complete audit trail.

**Q: Why INT-RAR-04 (forgery prevention) is critical?**
A: Without it, malicious operator could set `DecidedBy="alice@example.com"` and frame Alice. This test proves webhook OVERWRITES user-provided values.

**Q: Why INT-RAR-05 (webhook audit event) is critical?**
A: Proves webhook authentication was audited. Without this, auditors cannot verify tamper-proof authentication step.

---

## Approval

**Refactoring Status**: ✅ **COMPLETE**  
**Testing Status**: ⏳ **PENDING VALIDATION**  
**User Feedback**: Incorporated all 4 requirements:
- Q1 ✅: Added 3 new tests
- Q2 ✅: Refactored existing tests to business outcome validation
- Q3 ✅: Query DataStorage for webhook audit events
- Q4 ✅: Converted to table-driven pattern

---

**Document Version**: 1.0  
**Last Updated**: February 3, 2026  
**Maintained By**: Kubernaut Testing Team
