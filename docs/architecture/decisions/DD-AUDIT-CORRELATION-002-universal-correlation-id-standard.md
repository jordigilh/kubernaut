# DD-AUDIT-CORRELATION-002: Universal Correlation ID Standard

**Status**: ✅ APPROVED
**Date**: 2026-01-17
**Priority**: P0 - Foundational (Audit Trail Integrity)
**Scope**: System-wide - All services (Gateway, RemediationOrchestrator, WorkflowExecution, SignalProcessing, AIAnalysis, Notification)
**Supersedes**: DD-AUDIT-CORRELATION-001 (extended to system-wide), DD-015 (timestamp-based CRD naming)
**Related**: BR-AUDIT-005 (Audit Trail), DD-AUDIT-001 (Audit Responsibility), ADR-032 (Data Access Layer)

---

## 🎯 **Executive Summary**

**Decision**: `RemediationRequest.Name` is the **universal correlation ID** for all audit events across all 6 services.

**Breaking Change**: RemediationOrchestrator migrates from `rr.UID` → `rr.Name` for correlation_id.

**Gateway Requirement**: RemediationRequest names MUST use UUID-based generation to guarantee uniqueness.

---

## 📋 **Context & Problem**

### **Issue Discovered: Inconsistent Correlation IDs**

During RemediationOrchestrator integration test triage (January 17, 2026), a critical inconsistency was discovered:

**Current State (INCONSISTENT)**:
```go
// RemediationOrchestrator emits:
correlationID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"  // rr.UID (UUID)

// WorkflowExecution emits:
correlationID = "rr-pod-crash-abc123"  // rr.Name (human-readable)

// SignalProcessing emits:
correlationID = "rr-pod-crash-abc123"  // rr.Name (human-readable)

// Notification emits:
correlationID = "rr-pod-crash-abc123"  // rr.Name (human-readable)
```

**Impact**:
- ❌ **Cannot query all events for a remediation** with single correlation_id
- ❌ **Violates DD-AUDIT-CORRELATION-001** principle: "Parent RR Name is Root Correlation ID"
- ❌ **Debugging nightmare**: Must join on both `rr.UID` AND `rr.Name`
- ❌ **Inconsistent with 4/6 services**: Only RO + AIAnalysis use `rr.UID`

### **Root Cause**

**No authoritative documentation** explained why RemediationOrchestrator used `rr.UID` instead of `rr.Name`.

**Likely reasons** (undocumented):
1. Historical decision (RO predates DD-AUDIT-CORRELATION-001)
2. Perceived benefit of global uniqueness (UID vs Name)
3. Assumed immutability requirement (UID never changes)

### **User Insight: "UUID is good, but not human-readable"**

**Key observation**: Using `rr.UID` as correlation_id has merit (globally unique), but sacrifices human readability.

**Solution**: **Move UUID into RemediationRequest name** instead of correlation_id.

---

## ✅ **Decision**

### **Universal Correlation ID Standard**

**ALL services MUST use `RemediationRequest.Name` as correlation_id:**

```go
// ✅ CORRECT (MANDATORY for all services)
correlationID := remediationRequest.Name

// ❌ INCORRECT (FORBIDDEN - breaks audit trail continuity)
correlationID := string(remediationRequest.UID)
```

### **Gateway Name Generation Requirement**

**Gateway MUST generate RemediationRequest names using UUID suffix:**

```go
// ✅ CORRECT: UUID-based name generation
import "github.com/google/uuid"

shortUUID := uuid.New().String()[:8]  // 8-char suffix
crdName := fmt.Sprintf("rr-%s-%s", fingerprintPrefix, shortUUID)
// Result: "rr-pod-crash-f8a3b9c2"

// ❌ INCORRECT: Timestamp-based (collision risk)
timestamp := time.Now().Unix()
crdName := fmt.Sprintf("rr-%s-%d", fingerprintPrefix, timestamp)
// Result: "rr-pod-crash-1737138721"
```

**Format**: `rr-{fingerprint-prefix}-{uuid-suffix}`
- `fingerprint-prefix`: First 12 chars of signal fingerprint (human-readable context)
- `uuid-suffix`: 8 chars from UUID (guaranteed uniqueness)

**Example**: `"rr-pod-crash-f8a3b9c2"`

---

## 🎯 **Rationale**

### **Why rr.Name is Superior to rr.UID**

| Aspect | rr.Name (Human-Readable) | rr.UID (UUID) |
|--------|-------------------------|---------------|
| **Readability** | ✅ `"rr-pod-crash-f8a3b9c2"` | ❌ `"a1b2c3d4-e5f6-7890-abcd-ef1234567890"` |
| **Debugging** | ✅ Easy to grep logs | ❌ Copy-paste UUID required |
| **Query Simplicity** | ✅ Single correlation_id per remediation | ❌ Must join rr.UID + rr.Name |
| **Consistency** | ✅ 4 services already use it | ❌ Only 2 services (RO, AA) |
| **Authority** | ✅ DD-AUDIT-CORRELATION-001 | ❌ No authoritative doc |
| **Uniqueness** | ✅ UUID suffix guarantees uniqueness | ✅ Globally unique |

### **Why UUID-Based Names Solve Collision Risk**

**Concern**: `rr.Name` is namespace-scoped (not globally unique)

**OLD Gateway Pattern (Timestamp-Based)**:
```go
crdName := fmt.Sprintf("rr-%s-%d", fingerprintPrefix, timestamp)
// Collision risk: Two signals at same Unix second
```

**NEW Gateway Pattern (UUID-Based)**:
```go
shortUUID := uuid.New().String()[:8]
crdName := fmt.Sprintf("rr-%s-%s", fingerprintPrefix, shortUUID)
// Zero collision risk: UUID guarantees uniqueness (2^128 space)
```

**Benefits**:
1. ✅ **Zero collision risk**: UUID v4 has 122 bits of randomness
2. ✅ **Human-readable**: Fingerprint prefix provides context
3. ✅ **Consistent length**: 24 chars (well under K8s 253-char limit)
4. ✅ **Industry standard**: Docker, Kubernetes use similar patterns

---

## 📊 **Affected Services**

### **Migration Required**

| Service | Current Pattern | New Pattern | Files Affected |
|---------|----------------|-------------|----------------|
| **Gateway** | `rr-{fp}-{timestamp}` | `rr-{fp}-{uuid}` | `pkg/gateway/processing/crd_creator.go` |
| **RemediationOrchestrator** | `string(rr.UID)` | `rr.Name` | `internal/controller/remediationorchestrator/reconciler.go` (9 locations) |
| **AIAnalysis** | `analysis.Spec.RemediationID` (rr.UID) | `analysis.Spec.RemediationRequestRef.Name` | `pkg/aianalysis/audit/audit.go` |

### **No Changes Required** (Already Compliant)

| Service | Current Pattern | Status |
|---------|----------------|--------|
| **WorkflowExecution** | `wfe.Spec.RemediationRequestRef.Name` | ✅ Compliant (per DD-AUDIT-CORRELATION-001) |
| **SignalProcessing** | `sp.Spec.RemediationRequestRef.Name` | ✅ Compliant |
| **Notification** | `notification.Spec.RemediationRequestRef.Name` | ✅ Compliant |

---

## 🔧 **Implementation**

### **Phase 1: Gateway - UUID-Based Names**

**File**: `pkg/gateway/processing/crd_creator.go`

**BEFORE** (line 412-417):
```go
fingerprintPrefix := signal.Fingerprint
if len(fingerprintPrefix) > 12 {
    fingerprintPrefix = fingerprintPrefix[:12]
}
timestamp := c.clock.Now().Unix()
crdName := fmt.Sprintf("rr-%s-%d", fingerprintPrefix, timestamp)
```

**AFTER**:
```go
import "github.com/google/uuid"

fingerprintPrefix := signal.Fingerprint
if len(fingerprintPrefix) > 12 {
    fingerprintPrefix = fingerprintPrefix[:12]
}
// DD-AUDIT-CORRELATION-002: Use UUID suffix for guaranteed uniqueness
shortUUID := uuid.New().String()[:8]
crdName := fmt.Sprintf("rr-%s-%s", fingerprintPrefix, shortUUID)
```

### **Phase 2: RemediationOrchestrator - Use rr.Name**

**File**: `internal/controller/remediationorchestrator/reconciler.go`

**BEFORE** (9 locations):
```go
correlationID := string(rr.UID)
```

**AFTER**:
```go
// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
// Per universal standard: All services use RemediationRequest.Name
correlationID := rr.Name
```

**Locations**:
- Line 1651: `emitRemediationCreatedAudit()`
- Line 1701: `emitLifecycleStartedAudit()`
- Line 1739: `emitPhaseTransitionAudit()`
- Line 1778: `emitLifecycleCompletedAudit()`
- Line 1818: `emitLifecycleFailedAudit()`
- Line 1857: `emitApprovalRequestedAudit()`
- Line 1921: `emitApprovalApprovedAudit()`
- Line 1964: `emitApprovalRejectedAudit()`
- Line 2038: `emitManualReviewAudit()`

### **Phase 3: AIAnalysis - Use RemediationRequestRef.Name**

**File**: `pkg/aianalysis/audit/audit.go`

**BEFORE**:
```go
correlationID := analysis.Spec.RemediationID  // Uses rr.UID
```

**AFTER**:
```go
// DD-AUDIT-CORRELATION-002: Use parent RR name (not RemediationID)
correlationID := analysis.Spec.RemediationRequestRef.Name
```

**Note**: AIAnalysis inconsistency (internal development reference, removed in v1.0).

### **Phase 4: Integration Tests - Update Queries**

**Files**:
- `test/integration/remediationorchestrator/gap8_timeout_config_audit_test.go`
- `test/integration/remediationorchestrator/audit_emission_integration_test.go`

**BEFORE**:
```go
correlationID := string(rr.UID)
```

**AFTER**:
```go
// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) for audit queries
correlationID := rr.Name
```

---

## 📊 **Verification**

### **How to Verify Compliance**

**1. Audit Event Query** (DataStorage):
```sql
-- All events for a remediation should have SAME correlation_id
SELECT
  event_type,
  correlation_id,
  event_timestamp
FROM audit_events
WHERE correlation_id = 'rr-pod-crash-f8a3b9c2'
ORDER BY event_timestamp;
```

**Expected**: All events from ALL services share same `correlation_id`.

**2. Code Pattern Check**:
```bash
# Verify NO services use rr.UID for correlation_id
grep -r "correlationID := string(.*\.UID)" \
  internal/controller/ \
  pkg/ \
  --include="*.go"

# Expected: Zero results (all migrated to rr.Name)
```

**3. Gateway Name Format Check**:
```bash
# Verify UUID-based name generation
grep -A 5 "crdName :=" pkg/gateway/processing/crd_creator.go

# Expected: uuid.New().String()[:8]
```

---

## 🔄 **Audit Trail Flow (After Migration)**

### **Correct Flow (DD-AUDIT-CORRELATION-002)**

```
Gateway → RemediationRequest (Name: "rr-pod-crash-f8a3b9c2")
  ↓
  correlation_id = "rr-pod-crash-f8a3b9c2"
  ↓
RemediationOrchestrator Audit Events:
  - orchestrator.lifecycle.started (correlation_id: "rr-pod-crash-f8a3b9c2")
  - orchestrator.lifecycle.created (correlation_id: "rr-pod-crash-f8a3b9c2")
  - orchestrator.phase.transitioned (correlation_id: "rr-pod-crash-f8a3b9c2")
  ↓
SignalProcessing → SP CRD (Spec.RemediationRequestRef.Name: "rr-pod-crash-f8a3b9c2")
  ↓
  correlation_id = "rr-pod-crash-f8a3b9c2"
  ↓
SP Audit Events:
  - signalprocessing.classification.decision (correlation_id: "rr-pod-crash-f8a3b9c2")
  ↓
AIAnalysis → AA CRD (Spec.RemediationRequestRef.Name: "rr-pod-crash-f8a3b9c2")
  ↓
  correlation_id = "rr-pod-crash-f8a3b9c2"
  ↓
AA Audit Events:
  - aianalysis.analysis.completed (correlation_id: "rr-pod-crash-f8a3b9c2")
  ↓
WorkflowExecution → WFE CRD (Spec.RemediationRequestRef.Name: "rr-pod-crash-f8a3b9c2")
  ↓
  correlation_id = "rr-pod-crash-f8a3b9c2"
  ↓
WFE Audit Events:
  - workflow.selection.completed (correlation_id: "rr-pod-crash-f8a3b9c2")
  - execution.workflow.started (correlation_id: "rr-pod-crash-f8a3b9c2")
  ↓
Notification → Notification CRD (Spec.RemediationRequestRef.Name: "rr-pod-crash-f8a3b9c2")
  ↓
  correlation_id = "rr-pod-crash-f8a3b9c2"
  ↓
Notification Audit Events:
  - notification.message.sent (correlation_id: "rr-pod-crash-f8a3b9c2")
```

**Result**: **Single correlation_id** traces entire remediation flow across all 6 services! ✅

---

## 📋 **Alternatives Considered**

### **Alternative 1: Keep rr.UID for RemediationOrchestrator** (REJECTED)

**Pattern**: RemediationOrchestrator continues using `string(rr.UID)` while other services use `rr.Name`

❌ **Rejected**:
- Requires complex queries joining both correlation IDs
- Violates "Parent RR Name is Root Correlation ID" principle
- Inconsistent with 4/6 services
- No documented rationale for exception

### **Alternative 2: All Services Use rr.UID** (REJECTED)

**Pattern**: Migrate ALL services from `rr.Name` → `string(rr.UID)`

❌ **Rejected**:
- Loses human readability (`a1b2c3d4-e5f6-...` vs `rr-pod-crash-f8a3b9c2`)
- Requires breaking changes to 4 services (vs 2 services)
- Violates DD-AUDIT-CORRELATION-001 (already approved)
- User feedback: "not human-readable"

### **Alternative 3: Dual Correlation ID (rr.UID + rr.Name)** (REJECTED)

**Pattern**: Store both `rr.UID` and `rr.Name` as correlation IDs

❌ **Rejected**:
- Adds complexity to audit schema
- Wastes storage (duplicate indexing)
- Confusing API (which ID to query?)
- Solves no real problem (UUID in name already guarantees uniqueness)

### **Alternative 4: Universal rr.Name + UUID Suffix** (APPROVED)

**Pattern**: All services use `rr.Name`, Gateway generates UUID-based names

✅ **APPROVED**:
- ✅ Human-readable correlation IDs
- ✅ Zero collision risk (UUID suffix)
- ✅ Single correlation_id per remediation
- ✅ Minimal migration (2 services vs 4)
- ✅ Aligns with DD-AUDIT-CORRELATION-001 principle
- ✅ User-approved architecture

---

## 🚨 **Breaking Changes**

### **Audit Event Correlation ID Changes**

**RemediationOrchestrator Events** (Breaking Change):
```diff
# BEFORE (OLD)
correlation_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"  # rr.UID

# AFTER (NEW)
correlation_id: "rr-pod-crash-f8a3b9c2"  # rr.Name
```

**Impact**:
- ⚠️ **Historical queries break**: Old audit events use `rr.UID`, new events use `rr.Name`
- ⚠️ **Migration required**: Dashboards, alerts, reports must update queries
- ⚠️ **No backward compatibility**: Cannot query mixed UID/Name easily

**Mitigation**:
1. **Transition period**: Document cutover date (January 17, 2026)
2. **Query pattern**:
   ```sql
   -- For historical + new events
   WHERE correlation_id IN (rr.UID, rr.Name)
   ```
3. **Documentation**: Update all audit query examples

### **RemediationRequest Name Format Changes**

**Gateway-Generated Names** (Breaking Change):
```diff
# BEFORE (OLD)
rr.Name: "rr-pod-crash-1737138721"  # Timestamp-based

# AFTER (NEW)
rr.Name: "rr-pod-crash-f8a3b9c2"  # UUID-based
```

**Impact**:
- ⚠️ **Name length changes**: 24 chars (consistent) vs variable length
- ⚠️ **Timestamp no longer embedded**: Cannot infer creation time from name
- ✅ **Zero collision risk**: UUID guarantees uniqueness

**Mitigation**:
1. **CreationTimestamp**: Use `rr.CreationTimestamp` for time queries
2. **Backward compatibility**: Old RRs keep timestamp names (no migration needed)

---

## 📚 **Related Documentation**

### **Supersedes / Extends**

- **DD-AUDIT-CORRELATION-001**: Extended from WorkflowExecution to system-wide
- **AA_CORRELATION_ID_INCONSISTENCY_JAN14_2026.md**: AIAnalysis migration now MANDATORY

### **Related Standards**

- **BR-AUDIT-005**: Audit Trail Requirements (correlation ID for request-response reconstruction)
- **DD-AUDIT-001**: Audit Responsibility Pattern (who audits what)
- **DD-AUDIT-002**: Audit Shared Library (correlation ID helpers)
- **DD-AUDIT-003**: Service Audit Trace Requirements (which services audit)
- **ADR-032**: Data Access Layer Isolation (audit is mandatory for P0 services)

### **Industry Precedents**

- **AWS CloudTrail**: Uses human-readable resource names (not ARNs) for correlation
- **Kubernetes**: Uses Pod names (not UIDs) for event correlation
- **DataDog APM**: Uses service names (not UUIDs) for trace correlation
- **Docker**: Uses `{name}-{short-uuid}` pattern for container names

---

## ✅ **Acceptance Criteria**

This decision is considered successfully implemented when:

1. ✅ **Gateway**: Generates RemediationRequest names with UUID suffix
2. ✅ **RemediationOrchestrator**: Uses `rr.Name` for all 9 audit emission functions
3. ✅ **AIAnalysis**: Uses `RemediationRequestRef.Name` (not `RemediationID`)
4. ✅ **Tests**: All integration tests query with `rr.Name` (not `rr.UID`)
5. ✅ **Code Search**: Zero instances of `correlationID := string(rr.UID)` in codebase
6. ✅ **Audit Query**: All events for a remediation share same `correlation_id`
7. ✅ **Documentation**: All code comments reference DD-AUDIT-CORRELATION-002

---

## 📊 **Success Metrics**

**Measurement Period**: 30 days post-deployment

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Audit Query Success Rate** | >99% | % of queries finding complete event trail |
| **Correlation ID Uniqueness** | 100% | Zero collision errors in DataStorage |
| **Human Readability Score** | >90% | User survey: "Can you understand this correlation_id?" |
| **Query Performance** | <100ms | Avg audit event query latency |

---

## 🔄 **Version History**

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-01-17 | Initial approval - Universal correlation ID standard |

---

**Status**: ✅ APPROVED
**Decision Maker**: Product/Engineering (based on user feedback)
**Implementation**: Required for RO integration test fixes
**Priority**: P0 - Foundational (blocks testing)
