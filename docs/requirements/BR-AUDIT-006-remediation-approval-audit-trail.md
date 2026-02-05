# BR-AUDIT-006: Remediation Approval Audit Trail

**Status**: ✅ **APPROVED** - V1.0 Critical Feature
**Date**: February 1, 2026
**Priority**: P0 (SOC 2 Compliance Mandatory)
**Version**: 1.0
**Related**: ADR-040, DD-AUDIT-003 v1.6, DD-WEBHOOK-001

---

## Executive Summary

RemediationApprovalRequest decisions (approved/rejected/expired) MUST be captured in audit events to satisfy SOC 2 CC8.1 (User Attribution) and CC6.8 (Non-Repudiation) requirements. This closes a critical compliance gap where approval decisions are stored in CRD status but have no tamper-evident audit trail after CRD deletion.

**Business Impact**: 
- ✅ SOC 2 Type II certification (critical for enterprise customers)
- ✅ Legal defensibility (prove WHO approved high-risk remediations)
- ✅ Forensic investigation (reconstruct approval decisions 90-365 days later)
- ❌ **Current State**: COMPLIANCE FAILURE - no audit trail for approval decisions

**Urgency**: **MANDATORY for V1.0** - SOC 2 audit will flag this as a control gap

---

## Context

### **The Problem**

**Current State** (RAR CRD only):
```yaml
apiVersion: remediation.kubernaut.ai/v1alpha1
kind: RemediationApprovalRequest
status:
  decision: approved                    # ← Stored in CRD
  decidedBy: alice@example.com         # ← Captured by webhook
  decidedAt: 2026-01-15T10:30:00Z      # ← Timestamp recorded
  decisionMessage: "Root cause accurate" # ← Rationale stored
```

**Problem**:
- ✅ Decision stored in RAR CRD
- ✅ User identity authenticated via webhook
- ❌ **NO audit event emitted**
- ❌ **NO tamper-evident record**
- ❌ **NO queryable history after CRD deletion (90 days)**

**Auditor Question**: "Who approved the production OOMKilled remediation on January 15?"

**After 90 days**: ❌ **NO ANSWER** - RAR CRD deleted, no audit trail

---

### **SOC 2 Requirements**

#### **SOC 2 CC8.1: User Attribution**
> "The entity identifies, captures, and retains sufficient, reliable information to achieve its service commitments and system requirements."

**What auditors need**:
1. **WHO** approved/rejected the remediation? (authenticated user)
2. **WHEN** was the decision made? (timestamp)
3. **WHAT** was approved? (workflow ID, confidence score, rationale)
4. **WHY** was it approved/rejected? (decision message)
5. **PROOF**: Tamper-evident record (SHA-256 hash)

#### **SOC 2 CC6.8: Non-Repudiation**
> "The entity implements mechanisms to prevent individuals from denying they performed specific actions."

**Requirements**:
- ✅ Cryptographic proof (event hashing)
- ✅ Long-term retention (90-365 days)
- ✅ Immutable audit log
- ✅ Digital signatures for legal evidence

---

## Business Requirement

### **BR-AUDIT-006: Approval Decision Audit Events**

#### **Description**

RemediationApprovalRequest controller MUST emit audit events to DataStorage when approval decisions are made, capturing WHO, WHEN, WHAT, and WHY for compliance and forensic investigation.

#### **Priority**

**P0 (CRITICAL)** - SOC 2 compliance mandatory for V1.0

#### **Rationale**

**Without audit events**:
- ❌ SOC 2 Type II certification FAILS (control gap)
- ❌ Cannot prove who approved high-risk remediations
- ❌ Legal liability (no defensible evidence)
- ❌ Forensic investigation impossible after CRD deletion

**With audit events**:
- ✅ SOC 2 Type II certified (92% enterprise compliance)
- ✅ Legal defensibility (tamper-proof evidence)
- ✅ Complete audit trail (queryable for 90-365 days)
- ✅ Accountability (non-repudiation)

---

## Functional Requirements

### **FR-001: Approval Decision Event**

**Event Type**: `approval.decision`

**Trigger**: RAR status.decision changes from empty to `approved|rejected|expired`

**Event Data Payload**:
```yaml
event_type: approval.decision
event_category: approval
event_action: decision_made
event_outcome: success
correlation_id: rr-oomkilled-abc123        # Parent RR name
actor_type: user
actor_id: alice@example.com                # From webhook
resource_type: RemediationApprovalRequest
resource_name: rar-rr-oomkilled-abc123
namespace: production
event_timestamp: 2026-01-15T10:30:00Z

event_data:
  remediation_request_name: rr-oomkilled-abc123
  ai_analysis_name: ai-rr-oomkilled-abc123
  decision: approved                       # approved|rejected|expired
  decided_by: alice@example.com           # Authenticated username
  decided_at: 2026-01-15T10:30:00Z        # Decision timestamp
  decision_message: "Root cause accurate. Safe to proceed."
  confidence: 0.75                         # AI confidence score
  workflow_id: oomkill-increase-memory-limits
  workflow_version: v1.2.0
  target_resource: payment/deployment/payment-api
  timeout_deadline: 2026-01-15T10:45:00Z   # Approval deadline
  decision_duration_seconds: 180           # Time to decision
```

**Acceptance Criteria**:
- ✅ Event emitted ONLY when decision changes (idempotency)
- ✅ Authenticated user captured from webhook (not self-reported)
- ✅ Complete approval context included
- ✅ Correlation ID matches parent RemediationRequest
- ✅ Event hash computed for tamper-evidence
- ✅ Fire-and-forget (no reconciliation failure on audit failure)

---

### **FR-002: Approval Request Created Event** (Optional - Context)

**Event Type**: `approval.request.created`

**Trigger**: RAR CRD created

**Purpose**: Capture approval request context (what was requested, by whom)

**Event Data Payload**:
```yaml
event_type: approval.request.created
event_category: approval
event_action: request_created
event_outcome: success
correlation_id: rr-oomkilled-abc123
actor_type: service
actor_id: aianalysis-controller            # Controller that requested
resource_type: RemediationApprovalRequest
resource_name: rar-rr-oomkilled-abc123

event_data:
  remediation_request_name: rr-oomkilled-abc123
  ai_analysis_name: ai-rr-oomkilled-abc123
  confidence: 0.75
  workflow_id: oomkill-increase-memory-limits
  approval_reason: "Confidence below 80% auto-approve threshold"
  required_by: 2026-01-15T10:45:00Z        # Approval deadline
  request_severity: high
```

**Acceptance Criteria**:
- ✅ Event emitted when RAR CRD created
- ✅ Captures approval context (why approval needed)
- ✅ Includes deadline and severity

---

### **FR-003: Approval Timeout Event**

**Event Type**: `approval.timeout`

**Trigger**: RAR decision times out (no operator response before deadline)

**Event Data Payload**:
```yaml
event_type: approval.timeout
event_category: approval
event_action: timeout
event_outcome: failure
correlation_id: rr-oomkilled-abc123
actor_type: system
actor_id: remediationorchestrator-controller
resource_type: RemediationApprovalRequest

event_data:
  remediation_request_name: rr-oomkilled-abc123
  ai_analysis_name: ai-rr-oomkilled-abc123
  timeout_deadline: 2026-01-15T10:45:00Z
  timeout_duration_seconds: 900            # 15 minutes
  timeout_reason: "No operator response within deadline"
```

**Acceptance Criteria**:
- ✅ Event emitted when timeout occurs
- ✅ Captures timeout context (deadline, duration)
- ✅ Outcome marked as "failure"

---

## Implementation Requirements

### **IR-001: Audit Package Creation**

**Location**: `pkg/remediationapprovalrequest/audit/`

**Files**:
- `audit.go` - Core audit client
- `types.go` - Event type constants
- `audit_test.go` - Unit tests

**Pattern**: Follow `pkg/aianalysis/audit/` pattern

**Estimated LOC**: 400-600 LOC

---

### **IR-002: OpenAPI Schema Extension**

**Location**: `api/openapi/data-storage-v1.yaml`

**New Schema**: `RemediationApprovalDecisionPayload`

```yaml
RemediationApprovalDecisionPayload:
  type: object
  required:
    - event_type
    - remediation_request_name
    - ai_analysis_name
    - decision
    - decided_by
  properties:
    event_type:
      type: string
      enum: [approval.decision, approval.request.created, approval.timeout]
    remediation_request_name:
      type: string
    ai_analysis_name:
      type: string
    decision:
      type: string
      enum: [approved, rejected, expired]
    decided_by:
      type: string
      description: Authenticated username from webhook
    decided_at:
      type: string
      format: date-time
    decision_message:
      type: string
    confidence:
      type: number
      format: float
    workflow_id:
      type: string
    workflow_version:
      type: string
    target_resource:
      type: string
    timeout_deadline:
      type: string
      format: date-time
    decision_duration_seconds:
      type: integer
    approval_reason:
      type: string
    timeout_reason:
      type: string
```

**Estimated LOC**: 50-80 LOC (YAML)

---

### **IR-003: Controller Integration**

**Location**: `pkg/remediationapprovalrequest/controller/`

**Hook Point**: Status update reconciliation

```go
// When decision is made
func (r *Reconciler) handleDecisionChange(ctx context.Context, rar *remediationapprovalrequestv1alpha1.RemediationApprovalRequest, oldDecision string) {
    // Only emit audit event when decision changes from empty to decided
    if oldDecision == "" && rar.Status.Decision != "" {
        r.auditClient.RecordApprovalDecision(ctx, rar)
    }
}
```

**Estimated LOC**: 100-150 LOC (controller integration + tests)

---

### **IR-004: Event Type Registration**

**Location**: `docs/architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md`

**Update**: Add RemediationApprovalRequest section

**New Event Types**:
- `approval.request.created` - Request context
- `approval.decision` - Decision made (CRITICAL)
- `approval.timeout` - Timeout occurred

**Estimated Volume**: +150 events/day (5-10% of remediations require approval)

---

## Testing Requirements

### **TR-001: Unit Tests**

**Test Suite**: `pkg/remediationapprovalrequest/audit/audit_test.go`

**Test Cases**:
1. Event emitted when decision changes to "approved"
2. Event emitted when decision changes to "rejected"
3. Event emitted when decision changes to "expired"
4. NO event emitted if decision already set (idempotency)
5. Authenticated user captured correctly
6. Correlation ID matches parent RR
7. Complete approval context included
8. Fire-and-forget (no reconciliation failure on audit failure)

---

### **TR-002: Integration Tests**

**Test Suite**: `test/integration/remediationapprovalrequest/audit_integration_test.go`

**Test Scenarios**:
```gherkin
Scenario: Approval decision audit event emitted
  Given RemediationApprovalRequest "rar-1" exists with decision = ""
  When Operator approves "rar-1" via webhook
  Then approval.decision event should be in DataStorage
  And event.actor_id should be "alice@example.com"
  And event.correlation_id should be parent RR name
  And event.event_data.decision should be "approved"

Scenario: Audit event queryable after CRD deletion
  Given RemediationApprovalRequest "rar-1" was approved 100 days ago
  And RAR CRD has been deleted (TTL expired)
  When Auditor queries approval events for correlation_id "rr-1"
  Then approval.decision event should be returned
  And event should contain complete approval context
```

---

## Compliance Mapping

| SOC 2 Control | Requirement | Implementation |
|---------------|-------------|----------------|
| **CC8.1** - User Attribution | Capture WHO made decision | `actor_id` from auth webhook |
| **CC6.8** - Non-Repudiation | Tamper-proof record | SHA-256 event hashing |
| **CC7.2** - Monitoring | Audit all approvals | `approval.decision` event |
| **CC7.3** - Retention | 90-365 day retention | DataStorage legal hold |
| **AU-2** - Auditable Events | Approval decisions logged | All approval lifecycle events |
| **AC-2** - Account Management | Link actions to users | Authenticated `actor_id` |

---

## Success Criteria

### **Compliance**:
- ✅ SOC 2 Type II auditor accepts approval audit trail
- ✅ All approval decisions queryable for 90-365 days
- ✅ Tamper-evidence verified (SHA-256 hashing)
- ✅ User attribution authenticated (webhook integration)

### **Functional**:
- ✅ `approval.decision` event emitted for all decisions
- ✅ Correlation ID links to parent RemediationRequest
- ✅ Complete approval context captured
- ✅ Fire-and-forget (no controller failure on audit failure)

### **Performance**:
- ✅ Audit event latency <100ms (fire-and-forget)
- ✅ No impact on RAR controller reconciliation time
- ✅ Storage overhead: ~1KB per approval decision

---

## Implementation Plan

| Phase | Deliverable | Effort | Priority |
|-------|-------------|--------|----------|
| 1 | Create audit package | 2 days | P0 |
| 2 | Update OpenAPI schema | 1 day | P0 |
| 3 | Integrate with controller | 1 day | P0 |
| 4 | Unit tests | 1 day | P0 |
| 5 | Integration tests | 2 days | P0 |
| **TOTAL** | | **1 week** | **P0** |

---

## Related Documents

- [ADR-040: RemediationApprovalRequest CRD Architecture](../architecture/decisions/ADR-040-remediation-approval-request-architecture.md)
- [DD-AUDIT-003: Service Audit Trace Requirements](../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md)
- [DD-WEBHOOK-001: CRD Webhook Requirements Matrix](../architecture/decisions/DD-WEBHOOK-001-crd-webhook-requirements-matrix.md)
- [BR-AUDIT-005: Hybrid Provider Data Capture](./11_SECURITY_ACCESS_CONTROL.md)
- [DD-AUDIT-002: Audit Shared Library Design](../architecture/decisions/DD-AUDIT-002-audit-shared-library-design.md)

---

## Approval

**Approved By**: Architecture Team, Compliance Team
**Date**: February 1, 2026
**Priority**: P0 - SOC 2 Compliance Mandatory
**Version**: 1.0
**Status**: ✅ **APPROVED for V1.0 Implementation**

---

**Document Version**: 1.0
**Last Updated**: February 1, 2026
**Maintained By**: Kubernaut Architecture Team
