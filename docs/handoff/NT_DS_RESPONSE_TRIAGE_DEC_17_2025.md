# NT DS Response Triage - December 17, 2025

**Date**: December 17, 2025
**Context**: DS team responded to `QUESTION_DS_AUDIT_EVENT_DATA_STRUCTURE.md`
**Status**: ✅ **DS RESPONSE TRIAGED** - Follow-up questions prepared

---

## 📋 DS Team Answer Summary

**Question**: What is the correct way to structure `event_data` for audit events?

**DS Team Answer**: **Use Pattern 2 with `audit.StructToMap()` helper**

### ✅ Authoritative Pattern

```go
// STEP 1: Define structured type
type MessageSentEventData struct {
    NotificationID string `json:"notification_id"`
    Channel        string `json:"channel"`
    MessageType    string `json:"message_type"`
    RecipientCount int    `json:"recipient_count"`
}

// STEP 2: Use in business logic
payload := MessageSentEventData{
    NotificationID: notification.Name,
    Channel:        notification.Spec.Channel,
    MessageType:    notification.Spec.Type,
    RecipientCount: len(notification.Status.Recipients),
}

// STEP 3: Convert at API boundary using shared helper
eventDataMap, err := audit.StructToMap(payload)
if err != nil {
    return err
}

// STEP 4: Set event data
audit.SetEventData(event, eventDataMap)
```

---

## 🎯 Key Principles from DS Team

| Principle | Explanation | Impact on NT |
|-----------|-------------|--------------|
| **Type Safety in Business Logic** | Use structured Go types for all audit event payloads | Must create 4 structured types |
| **Boundary Conversion** | Convert to `map[string]interface{}` ONLY at API boundary | Use `audit.StructToMap()` in audit functions |
| **Shared Helper** | Use `audit.StructToMap()` - NO custom `ToMap()` methods | Remove custom methods if any exist |
| **CommonEnvelope is Optional** | Only use if you need the outer envelope structure | NT doesn't need envelope |
| **DD-AUDIT-004 Compliance** | Structured types eliminate `map[string]interface{}` from business logic | Current direct map construction violates this |

---

## 🔍 Current State Analysis

### Notification Service Audit Implementation

**Current Pattern** (`internal/controller/notification/audit.go`):
```go
// Direct map construction (Pattern 1 - VIOLATES DS guidance)
eventData := map[string]interface{}{
    "notification_id": notification.Name,
    "channel":        notification.Spec.Channel,
    "message_type":   notification.Spec.Type,
    // ...
}
```

**Required Pattern** (Pattern 2 with `audit.StructToMap()`):
```go
// Structured type (complies with DS guidance)
payload := MessageSentEventData{
    NotificationID: notification.Name,
    Channel:        notification.Spec.Channel,
    MessageType:    notification.Spec.Type,
}
eventDataMap, err := audit.StructToMap(payload)
```

### Infrastructure Status

✅ **Helper Functions Exist**:
- `audit.StructToMap()` - `pkg/audit/helpers.go:127-153`
- `audit.SetEventData()` - `pkg/audit/helpers.go:93-95`
- `audit.SetEventDataFromEnvelope()` - `pkg/audit/helpers.go:98-105`

❌ **No Current Usage**:
- Grep search for `audit.StructToMap` in `internal/controller` returned **ZERO results**
- No services are currently using the recommended pattern

### Migration Required

| Service | Current Pattern | Action Required |
|---------|----------------|-----------------|
| **Notification** | Pattern 1 (Direct map) | Create 4 structured types + use `audit.StructToMap()` |
| **SignalProcessing** | Pattern 1 (Direct map) | Migrate (DS team notes: 1-2 hours) |
| **WorkflowExecution** | Pattern 2 (Custom `ToMap()`) | Replace custom `ToMap()` with `audit.StructToMap()` |
| **AIAnalysis** | Pattern 2 (Custom `ToMap()`) | Replace custom `ToMap()` with `audit.StructToMap()` |

---

## ❓ Follow-Up Questions for DS Team

### 🔴 CRITICAL - Unblocks NT Implementation

#### Q1: Where Should Structured Types Be Defined?

**Context**: DS response shows example type `MessageSentEventData` but doesn't specify location.

**Options**:
- **Option A**: `pkg/notification/audit/event_types.go` (service-specific, private)
- **Option B**: `pkg/audit/notification_events.go` (shared, cross-service visibility)
- **Option C**: `internal/controller/notification/audit_types.go` (controller-local, private)

**Question**: Which location aligns with project standards and future extensibility?

**Recommendation**: Option A (`pkg/notification/audit/`) for service encapsulation

---

#### Q2: Error Handling Pattern for `audit.StructToMap()`

**Context**: `audit.StructToMap()` returns `(map[string]interface{}, error)` but DS example doesn't show full error handling.

**Current Audit Function Signature**:
```go
func (r *NotificationRequestReconciler) auditMessageSent(
    ctx context.Context,
    notification *notificationv1.NotificationRequest,
) error
```

**Question**: When `audit.StructToMap()` fails, should we:
- **Option A**: Return error immediately (fail reconciliation)
- **Option B**: Log error and continue (degrade gracefully)
- **Option C**: Panic (marshal should never fail for valid structs)

**Recommendation**: Option A (return error) - aligns with ADR-032 §1 "No Audit Loss"

---

#### Q3: Migration Scope - NT Only or Multi-Service?

**Context**: DS response identifies 4 services needing migration (NT, SP, WE, AA).

**Question**: Should these migrations happen:
- **Option A**: Independently per service (NT now, others later)
- **Option B**: Coordinated cross-service (all at once)
- **Option C**: Phase 1 (NT), Phase 2 (others in V1.1)

**Recommendation**: Option A - NT proceeds independently, other services migrate on their own schedule

---

### 🟡 MEDIUM - Documentation and Standards

#### Q4: Should Structured Types Be Exported?

**Context**: Types will be in `pkg/notification/audit/`, but should they be public?

**Options**:
- **Option A**: Exported (`MessageSentEventData`) - testable from other packages
- **Option B**: Unexported (`messageSentEventData`) - internal implementation detail

**Current Practice Check**:
- WorkflowExecution: `WorkflowExecutionAuditPayload` (exported)
- AIAnalysis: Need to check

**Recommendation**: Exported - enables test validation from integration/e2e tests

---

#### Q5: JSON Tag Naming Convention

**Context**: DS example shows snake_case JSON tags (`notification_id`).

**OpenAPI Spec** (`api/openapi/data-storage-v1.yaml`):
```yaml
event_data:
  type: object
  description: Service-specific event data (CommonEnvelope structure)
  additionalProperties: true
```

**Question**: Are field names within `event_data` standardized, or service-defined?

**Observation**: DS example uses snake_case, aligning with PostgreSQL column naming

**Recommendation**: Follow DS example (snake_case) for consistency

---

#### Q6: DD-AUDIT-004 Update Required?

**Context**: DD-AUDIT-004 currently documents the principle but DS response suggests updating with concrete patterns.

**DS Response** (line 182):
> ⏸️ Update DD-AUDIT-004 with recommended pattern examples

**Question**: Should NT team:
- **Option A**: Update DD-AUDIT-004 with NT implementation as reference example
- **Option B**: DS team will update DD-AUDIT-004 centrally
- **Option C**: Create separate implementation guide referencing DD-AUDIT-004

**Recommendation**: Option A - NT updates DD-AUDIT-004 after implementation proves pattern

---

### 🟢 LOW - Future Considerations

#### Q7: Validation Strategy for Structured Types

**Context**: Structured types enable compile-time validation, but runtime validation might still be needed.

**Question**: Should structured types include validation tags (e.g., `validate:"required"`) or rely on OpenAPI validation?

**Current Pattern** (`pkg/audit/openapi_validator.go`):
- Validation happens at API boundary using OpenAPI spec

**Recommendation**: No validation tags - rely on OpenAPI validator

---

#### Q8: Backward Compatibility

**Context**: Migration from Pattern 1 → Pattern 2 changes internal implementation but not API contract.

**Question**: Are there any audit event consumers that might be affected by field name/structure changes?

**Observation**: `event_data` is JSONB in PostgreSQL, consumer queries might be brittle

**Recommendation**: Maintain exact same field names during migration (no breaking changes)

---

## 📊 NT Implementation Checklist

Based on DS team response, NT must:

### Phase 1: Create Structured Types
- [ ] Create `pkg/notification/audit/event_types.go`
- [ ] Define `MessageSentEventData` struct
- [ ] Define `MessageFailedEventData` struct
- [ ] Define `MessageAcknowledgedEventData` struct
- [ ] Define `MessageEscalatedEventData` struct
- [ ] Add JSON tags (snake_case)
- [ ] Add godoc comments referencing DD-AUDIT-004

### Phase 2: Refactor Audit Functions
- [ ] Update `auditMessageSent()` to use structured type + `audit.StructToMap()`
- [ ] Update `auditMessageFailed()` to use structured type + `audit.StructToMap()`
- [ ] Update `auditMessageAcknowledged()` to use structured type + `audit.StructToMap()`
- [ ] Update `auditMessageEscalated()` to use structured type + `audit.StructToMap()`
- [ ] Add error handling for `audit.StructToMap()` failures

### Phase 3: Update Tests
- [ ] Update unit tests to validate structured types
- [ ] Update integration tests to validate structured types via REST API
- [ ] Update E2E tests to validate structured types via REST API
- [ ] Verify field names match previous implementation (backward compatible)

### Phase 4: Documentation
- [ ] Update `NT_UNSTRUCTURED_DATA_TRIAGE_DEC_17_2025.md` with resolution
- [ ] Update `NT_DD_AUDIT_004_COMPLIANCE_COMPLETE.md` with final pattern
- [ ] Create completion handoff document
- [ ] Update DD-AUDIT-004 with NT implementation example (if approved)

---

## 🚨 Blocking Questions vs. Proceed Now

### 🔴 MUST ANSWER BEFORE PROCEEDING
**NONE** - DS team provided authoritative pattern with sufficient detail

### 🟡 SHOULD ANSWER FOR CONSISTENCY
- Q1: Type definition location (can default to `pkg/notification/audit/`)
- Q2: Error handling pattern (can default to return error)
- Q3: Migration scope (can proceed with NT only)

### 🟢 NICE TO KNOW
- Q4-Q8: Refinements that can be addressed during implementation review

---

## 🎯 Recommended Next Action

**Proceed with NT implementation using DS team's authoritative pattern:**

1. ✅ **Create structured types** in `pkg/notification/audit/event_types.go`
2. ✅ **Use `audit.StructToMap()`** for all conversions
3. ✅ **Return errors** if conversion fails (ADR-032 compliance)
4. ✅ **Export types** for test validation
5. ✅ **Use snake_case JSON tags** per DS example

**Defer to later**:
- Multi-service migration coordination (SP, WE, AA)
- DD-AUDIT-004 comprehensive update (after NT proves pattern)

---

## 📝 Follow-Up Document for DS Team

**If clarification needed, create**: `FOLLOWUP_DS_AUDIT_STRUCTURE_QUESTIONS.md`

**Priority Questions** (if blocking):
- Q1: Type definition location
- Q2: Error handling pattern

**Optional Questions** (can implement with reasonable defaults):
- Q4-Q8: Documentation and standards refinements

---

## ✅ Confidence Assessment

**Confidence in DS Response**: **100%**
**Justification**:
- DS team provided authoritative pattern with code examples
- Pattern references existing infrastructure (`audit.StructToMap()`, `audit.SetEventData()`)
- Clear rationale and authority references (DD-AUDIT-004, coding standards)
- Migration guidance for other services included

**Confidence in Proceeding**: **95%**
**Justification**:
- Core pattern is clear and unambiguous
- Infrastructure exists and is documented
- Follow-up questions are refinements, not blockers
- Can use reasonable defaults for Q1-Q3 if needed

**Risk Assessment**:
- ⚠️ **Low Risk**: Type definition location choice might need refactoring if wrong
- ✅ **No Risk**: Pattern itself is authoritative and well-documented
- ✅ **No Risk**: Backward compatible at API level (JSONB field names unchanged)

---

## 🔗 Related Documents

- **DS Team Response**: `docs/handoff/DS_AUDIT_EVENT_DATA_STRUCTURE_RESPONSE.md`
- **Original Question**: `docs/handoff/QUESTION_DS_AUDIT_EVENT_DATA_STRUCTURE.md`
- **Current Triage**: `docs/handoff/NT_UNSTRUCTURED_DATA_TRIAGE_DEC_17_2025.md`
- **Helper Implementation**: `pkg/audit/helpers.go:127-153`
- **DD-AUDIT-004**: `docs/architecture/decisions/DD-AUDIT-004-structured-types-for-audit-event-payloads.md`

---

**Next Step**: Create follow-up questions document if clarification needed, or proceed with implementation using reasonable defaults.


