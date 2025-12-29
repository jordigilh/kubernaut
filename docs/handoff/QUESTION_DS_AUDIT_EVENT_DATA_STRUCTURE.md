# Question for Data Services Team: Audit Event Data Structure

**From**: Notification Team (NT)
**To**: Data Services Team (DS)
**Date**: December 17, 2025
**Priority**: P1 - Affects All Services
**Status**: ⏳ **AWAITING DS TEAM RESPONSE**

---

## 🚨 Question Summary

**What is the correct/authoritative way to structure `event_data` for audit events?**

We've identified three different patterns in the codebase and need DS team clarification on which is correct.

---

## 📋 Context

While implementing audit events for the Notification service, we discovered conflicting patterns for how services populate the `EventData` field in `AuditEventRequest`.

**The Field Definition** (`pkg/datastorage/client/generated.go:174`):
```go
type AuditEventRequest struct {
    // ...
    // EventData Service-specific event data (CommonEnvelope structure)
    EventData map[string]interface{} `json:"event_data"`
    // ...
}
```

**OpenAPI Spec Comment**: "Service-specific event data (**CommonEnvelope structure**)"

---

## 🔍 Three Patterns We Found

### Pattern 1: Direct `map[string]interface{}` (SignalProcessing)

**File**: `pkg/signalprocessing/audit/client.go` (lines 98-152)

```go
// Build event_data directly as map
eventData := map[string]interface{}{
    "signal_type":    string(sp.Spec.SignalType),
    "target_cluster": sp.Spec.TargetCluster,
    "phase":          string(sp.Status.Phase),
    // ... more fields
}

// Set directly
audit.SetEventData(event, eventData)
```

**Pros**:
- ✅ Simple and direct
- ✅ No extra conversion step

**Cons**:
- ❌ No compile-time validation
- ❌ Doesn't use `CommonEnvelope` (despite OpenAPI comment)

---

### Pattern 2: Custom Structs with `ToMap()` Methods (WorkflowExecution, AIAnalysis)

**File**: `pkg/workflowexecution/audit_types.go` (lines 60-191)

```go
// Define service-specific struct
type WorkflowExecutionAuditPayload struct {
    WorkflowID     string `json:"workflow_id"`
    TargetResource string `json:"target_resource"`
    Phase          string `json:"phase"`
    // ... 15+ fields
}

// Add ToMap() method
func (p WorkflowExecutionAuditPayload) ToMap() map[string]interface{} {
    result := map[string]interface{}{
        "workflow_id":     p.WorkflowID,
        "target_resource": p.TargetResource,
        "phase":           p.Phase,
        // ... manual mapping
    }
    return result
}

// Use in controller
payload := WorkflowExecutionAuditPayload{...}
audit.SetEventData(event, payload.ToMap())
```

**Pros**:
- ✅ Type-safe business logic
- ✅ Compile-time field validation

**Cons**:
- ❌ Manual `ToMap()` boilerplate
- ❌ Doesn't use `CommonEnvelope` (despite OpenAPI comment)

---

### Pattern 3: CommonEnvelope (Documented Standard, But Rarely Used?)

**File**: `pkg/audit/event_data.go` (lines 22-46)

```go
// CommonEnvelope is the standard event_data format for audit events.
//
// Authority: ADR-034 (Unified Audit Table Design)
type CommonEnvelope struct {
    Version  string                 `json:"version"`
    Service  string                 `json:"service"`
    Operation string                `json:"operation"`
    Status   string                 `json:"status"`
    Payload  map[string]interface{} `json:"payload"`  // ← Service-specific fields here
    SourcePayload map[string]interface{} `json:"source_payload,omitempty"`
}

// Helper function available
func NewEventData(service, operation, status string, payload map[string]interface{}) *CommonEnvelope {
    return &CommonEnvelope{
        Version:   "1.0",
        Service:   service,
        Operation: operation,
        Status:    status,
        Payload:   payload,
    }
}

// Use with helper
envelope := audit.NewEventData(
    "notification",
    "message.sent",
    "success",
    map[string]interface{}{
        "notification_id": notification.Name,
        "channel":         "slack",
        // ... service-specific fields in Payload
    },
)
audit.SetEventDataFromEnvelope(event, envelope)
```

**Pros**:
- ✅ Uses documented `CommonEnvelope` standard
- ✅ Structured outer envelope (version, service, operation, status)
- ✅ Helper functions available (`SetEventDataFromEnvelope`)
- ✅ Matches OpenAPI comment ("CommonEnvelope structure")

**Cons**:
- ❌ `Payload` field is still `map[string]interface{}`
- ❌ **We can't find any service actually using this pattern in controllers**

---

## ❓ Our Questions

### Q1: Which Pattern is Correct?

**Options**:
- **A)** Pattern 1: Direct `map[string]interface{}` (simplest)
- **B)** Pattern 2: Custom structs with `ToMap()` (type-safe)
- **C)** Pattern 3: `CommonEnvelope` (documented standard)
- **D)** Something else we're missing?

### Q2: Why Does OpenAPI Say "CommonEnvelope structure"?

The OpenAPI spec comment says:
```
EventData Service-specific event data (CommonEnvelope structure)
```

But we can't find any service controller using `CommonEnvelope`. Why is it documented but not used?

### Q3: Is CommonEnvelope Mandatory?

**If CommonEnvelope is the standard**:
- Should ALL services refactor to use it?
- Or is it optional/legacy?
- Why do WorkflowExecution and AIAnalysis not use it?

### Q4: What About Type Safety?

**If we use CommonEnvelope**:
```go
envelope := audit.NewEventData(
    "notification",
    "message.sent",
    "success",
    map[string]interface{}{  // ← Still unstructured here
        "notification_id": notification.Name,
        "channel":         "slack",
    },
)
```

We still have `map[string]interface{}` in the `Payload` field. So we haven't eliminated unstructured data - we've just moved it inside the envelope.

**Question**: Is this acceptable, or should we have structured types for the payload too?

### Q5: DD-AUDIT-004 Conflict?

**DD-AUDIT-004** (Structured Types for Audit Event Payloads) mandates:
> "Services will implement structured Go types for all audit event payloads, eliminating `map[string]interface{}` usage."

But if `CommonEnvelope.Payload` is `map[string]interface{}`, how does this comply with DD-AUDIT-004?

**Options**:
- A) DD-AUDIT-004 only applies to business logic (structs → map at boundary)
- B) `CommonEnvelope` is an exception
- C) DD-AUDIT-004 needs clarification

---

## 🎯 What We Need from DS Team

### Immediate Need

**Authoritative guidance on**:
1. ✅ Which pattern ALL services should use
2. ✅ Whether `CommonEnvelope` is mandatory
3. ✅ How to balance type safety with `CommonEnvelope` usage

### Documentation Update

If `CommonEnvelope` is the standard:
- ✅ Update DD-AUDIT-004 to clarify its usage
- ✅ Provide example of `CommonEnvelope` with typed payloads
- ✅ Document migration path for WorkflowExecution/AIAnalysis

If `CommonEnvelope` is NOT mandatory:
- ✅ Update OpenAPI comment to remove "CommonEnvelope structure"
- ✅ Clarify that Pattern 1 or Pattern 2 is acceptable
- ✅ Update DD-AUDIT-004 to reflect actual practice

---

## 📊 Service Audit Pattern Survey

| Service | Pattern Used | File Reference |
|---|---|---|
| **SignalProcessing** | Pattern 1 (Direct map) | `pkg/signalprocessing/audit/client.go:98-152` |
| **WorkflowExecution** | Pattern 2 (Custom struct) | `pkg/workflowexecution/audit_types.go:60-191` |
| **AIAnalysis** | Pattern 2 (Custom struct) | `pkg/aianalysis/audit/audit.go` |
| **Notification** | ⏳ **BLOCKED** | Waiting for DS guidance |
| **Gateway** | Pattern 1? | (Need to verify) |
| **RemediationOrchestrator** | Pattern 1? | (Need to verify) |

**Observation**: No service controller uses `CommonEnvelope` despite it being documented as the standard.

---

## 🔗 Related Documents

### Authoritative Standards
- **ADR-034**: Unified Audit Table Design (mentions CommonEnvelope)
- **DD-AUDIT-004**: Structured Types for Audit Event Payloads
- **OpenAPI Spec**: `api/openapi/data-storage-v1.yaml` (line 912: "CommonEnvelope structure")

### Implementation References
- `pkg/audit/event_data.go` - CommonEnvelope definition
- `pkg/audit/helpers.go:97-105` - `SetEventDataFromEnvelope()` helper
- `pkg/workflowexecution/audit_types.go` - Pattern 2 example
- `pkg/signalprocessing/audit/client.go` - Pattern 1 example

### Triage Documents
- `docs/handoff/NT_AUDIT_ARCHITECTURE_TRIAGE_DEC_17_2025.md` - Our analysis
- `docs/handoff/CRITICAL_AUDIT_ARCHITECTURE_TRIAGE.md` - Architectural questions

---

## 📝 Response Format

Please reply inline below with:

1. **✅ Authoritative Pattern**: Which pattern (A/B/C/D) should ALL services use?
2. **📋 Rationale**: Why this pattern over the others?
3. **🔧 Migration Plan**: Do existing services need to change?
4. **📚 Documentation**: What needs to be updated?

---

## 💬 DS Team Response

**Responded By**: Data Services Team
**Date**: December 17, 2025
**Authority**: DD-AUDIT-004, ADR-034, pkg/audit/helpers.go

---

### ✅ Authoritative Pattern

**APPROVED PATTERN**: **Pattern 2 with `audit.StructToMap()` helper**

```go
// STEP 1: Define service-specific structured type (compile-time type safety)
type MessageSentEventData struct {
    NotificationID string `json:"notification_id"`
    Channel        string `json:"channel"`
    MessageType    string `json:"message_type"`
    RecipientCount int    `json:"recipient_count"`
    // ... all service-specific fields
}

// STEP 2: Use structured type in business logic
payload := MessageSentEventData{
    NotificationID: notification.Name,
    Channel:        notification.Spec.Channel,
    MessageType:    notification.Spec.Type,
    RecipientCount: len(notification.Status.Recipients),
}

// STEP 3: Convert to map at API boundary using shared helper
eventDataMap, err := audit.StructToMap(payload)
if err != nil {
    return err
}

// STEP 4: Set event data
audit.SetEventData(event, eventDataMap)
```

**Key Points**:
1. ✅ **Use structured types in business logic** (type-safe, compile-time validated)
2. ✅ **Convert to `map[string]interface{}` ONLY at API boundary** using `audit.StructToMap()`
3. ✅ **NO custom `ToMap()` methods** - use the shared `audit.StructToMap()` helper
4. ✅ **`CommonEnvelope` is OPTIONAL** - only use if you need the outer envelope structure

---

### 📋 Rationale

#### Why Pattern 2 with `audit.StructToMap()`?

**Authority**: `pkg/audit/helpers.go` lines 127-153

```go
// StructToMap converts any structured type to a map for use in AuditEventRequest.EventData
//
// This is the recommended approach per DD-AUDIT-004 for services using structured audit event types.
// It allows services to use type-safe structs while still providing the map[string]interface{}
// required by the audit API.
//
// DD-AUDIT-004: Structured Types for Audit Event Payloads
func StructToMap(data interface{}) (map[string]interface{}, error) { ... }
```

**Benefits**:
1. ✅ **Type Safety**: Structured types in business logic provide compile-time field validation
2. ✅ **Coding Standards Compliance**: Eliminates `map[string]interface{}` from business logic
3. ✅ **Maintainability**: Refactor-safe, IDE autocomplete, no field name typos
4. ✅ **Single Conversion Point**: `audit.StructToMap()` is the canonical boundary conversion
5. ✅ **Consistent Pattern**: All services use the same helper function
6. ✅ **Test Coverage**: Structured types enable 100% field validation in tests

**Why NOT Pattern 1 (Direct `map[string]interface{}`):**
- ❌ Violates project coding standards (avoid `any`/`interface{}`)
- ❌ No compile-time type safety
- ❌ Runtime-only error detection
- ❌ Harder to maintain and refactor

**Why NOT Pattern 3 (Mandatory `CommonEnvelope`):**
- ⚠️ `CommonEnvelope` is **OPTIONAL**, not mandatory
- ⚠️ `CommonEnvelope.Payload` is still `map[string]interface{}`, so type safety still requires structured types
- ⚠️ Adds extra nesting when not needed (service/operation/status fields)
- ✅ Use `CommonEnvelope` ONLY when you need the outer envelope structure (e.g., preserving external API payloads)

---

### 🔧 Migration Plan

#### Services Using Pattern 1 (Direct Map) - MUST MIGRATE

**Affected Services**:
- SignalProcessing (`pkg/signalprocessing/audit/client.go:98-152`)
- Gateway (to be verified)
- RemediationOrchestrator (to be verified)

**Migration Steps**:

1. **Define structured types** for each event:
```go
// Before (Pattern 1):
eventData := map[string]interface{}{
    "signal_type":    string(sp.Spec.SignalType),
    "target_cluster": sp.Spec.TargetCluster,
    "phase":          string(sp.Status.Phase),
}

// After (Pattern 2):
type SignalReceivedEventData struct {
    SignalType    string `json:"signal_type"`
    TargetCluster string `json:"target_cluster"`
    Phase         string `json:"phase"`
}

payload := SignalReceivedEventData{
    SignalType:    string(sp.Spec.SignalType),
    TargetCluster: sp.Spec.TargetCluster,
    Phase:         string(sp.Status.Phase),
}
```

2. **Replace direct map usage** with `audit.StructToMap()`:
```go
// Before:
audit.SetEventData(event, eventData)

// After:
eventDataMap, err := audit.StructToMap(payload)
if err != nil {
    return err
}
audit.SetEventData(event, eventDataMap)
```

3. **Update tests** to validate structured types

**Effort**: 1-2 hours per service

---

#### Services Using Pattern 2 (Custom `ToMap()`) - REFACTOR RECOMMENDED

**Affected Services**:
- WorkflowExecution (`pkg/workflowexecution/audit_types.go:60-191`)
- AIAnalysis (`pkg/aianalysis/audit/audit.go`)

**Migration Steps**:

1. **Remove custom `ToMap()` methods**:
```go
// Before:
func (p WorkflowExecutionAuditPayload) ToMap() map[string]interface{} {
    result := map[string]interface{}{
        "workflow_id":     p.WorkflowID,
        "target_resource": p.TargetResource,
        // ... manual mapping
    }
    return result
}

// After: DELETE this method, use audit.StructToMap() instead
```

2. **Replace custom `ToMap()` calls** with `audit.StructToMap()`:
```go
// Before:
audit.SetEventData(event, payload.ToMap())

// After:
eventDataMap, err := audit.StructToMap(payload)
if err != nil {
    return err
}
audit.SetEventData(event, eventDataMap)
```

**Effort**: 30 minutes per service (simple find/replace)

---

#### Notification Service - PROCEED WITH PATTERN 2

**Guidance for NT Team**:
1. ✅ Define structured types for each event (e.g., `MessageSentEventData`, `ChannelConfiguredEventData`)
2. ✅ Use `audit.StructToMap()` to convert at API boundary
3. ✅ **DO NOT** create custom `ToMap()` methods
4. ✅ **DO NOT** use direct `map[string]interface{}`
5. ✅ Reference DD-AUDIT-004 in code comments

**Example for NT**:
```go
// pkg/notification/audit/types.go
type MessageSentEventData struct {
    NotificationID string `json:"notification_id"`
    Channel        string `json:"channel"`
    MessageType    string `json:"message_type"`
    RecipientCount int    `json:"recipient_count"`
    DurationMs     int    `json:"duration_ms,omitempty"`
}

// internal/controller/notification/audit.go
payload := MessageSentEventData{...}
eventDataMap, err := audit.StructToMap(payload)
if err != nil {
    return err
}
audit.SetEventData(event, eventDataMap)
```

---

### 📚 Documentation Updates Needed

#### 1. Update DD-AUDIT-004

**File**: `docs/architecture/decisions/DD-AUDIT-004-structured-types-for-audit-event-payloads.md`

**Add Section**:
```markdown
## 🎯 Recommended Pattern for All Services

### Pattern: Structured Types with `audit.StructToMap()`

**Authority**: `pkg/audit/helpers.go:127-153`

```go
// 1. Define structured type
type ServiceEventData struct {
    Field1 string `json:"field1"`
    Field2 int    `json:"field2"`
}

// 2. Use in business logic
payload := ServiceEventData{...}

// 3. Convert at API boundary
eventDataMap, err := audit.StructToMap(payload)
audit.SetEventData(event, eventDataMap)
```

**Key Principles**:
- ✅ Structured types in business logic (type-safe)
- ✅ `map[string]interface{}` ONLY at API boundary
- ✅ Use `audit.StructToMap()` for conversion (no custom `ToMap()` methods)
- ✅ `CommonEnvelope` is OPTIONAL (only use if needed)
```

#### 2. Update OpenAPI Spec Comment

**File**: `api/openapi/data-storage-v1.yaml:912`

**Current**:
```yaml
event_data:
  description: Service-specific event data (CommonEnvelope structure)
```

**Updated**:
```yaml
event_data:
  description: |
    Service-specific event data.

    Services should define structured Go types and convert to map at API boundary using audit.StructToMap().
    CommonEnvelope is optional (use only if you need the outer envelope structure).

    See DD-AUDIT-004 for the recommended pattern.
```

#### 3. Add Migration Guide

**Create**: `docs/guides/AUDIT_EVENT_DATA_MIGRATION_GUIDE.md`

**Content**:
- Pattern comparison (1 vs 2 vs 3)
- Step-by-step migration from Pattern 1 → Pattern 2
- Examples for each service type
- Testing recommendations

---

### 🎯 Summary: What NT Team Should Do

#### Immediate Actions

1. ✅ **Define structured types** for all Notification audit events:
   - `MessageSentEventData`
   - `MessageFailedEventData`
   - `ChannelConfiguredEventData`
   - `RetryAttemptedEventData`
   - etc.

2. ✅ **Use `audit.StructToMap()`** for conversion:
```go
payload := MessageSentEventData{...}
eventDataMap, err := audit.StructToMap(payload)
audit.SetEventData(event, eventDataMap)
```

3. ✅ **DO NOT** create custom `ToMap()` methods

4. ✅ **DO NOT** use `CommonEnvelope` unless you specifically need the outer envelope structure

5. ✅ **Reference DD-AUDIT-004** in code comments

#### Testing

1. ✅ Verify all structured types have 100% field coverage
2. ✅ Integration tests validate audit event structure
3. ✅ Verify audit events are queryable in DataStorage

---

### ❓ Follow-Up Questions Answered

#### Q1: Which Pattern is Correct?
**Answer**: **Pattern 2 with `audit.StructToMap()`** (not custom `ToMap()` methods)

#### Q2: Why Does OpenAPI Say "CommonEnvelope structure"?
**Answer**: Historical documentation. `CommonEnvelope` is OPTIONAL, not mandatory. We'll update the OpenAPI spec comment to clarify.

#### Q3: Is CommonEnvelope Mandatory?
**Answer**: **NO** - `CommonEnvelope` is optional. Use it only if you need the outer envelope structure (version, service, operation, status). Most services don't need it.

#### Q4: What About Type Safety?
**Answer**: Type safety comes from **structured types in business logic**, not from `CommonEnvelope`. Use structured types + `audit.StructToMap()` for full type safety while complying with the API contract.

#### Q5: DD-AUDIT-004 Conflict?
**Answer**: **NO CONFLICT** - DD-AUDIT-004 mandates structured types in business logic. The `map[string]interface{}` in the API contract is the **boundary layer**. Type safety is maintained through structured types + `audit.StructToMap()` conversion.

---

**Document Status**: ✅ **DS TEAM RESPONSE COMPLETE**
**NT Team**: Unblocked - proceed with Pattern 2 using `audit.StructToMap()`
**Migration**: SignalProcessing, WorkflowExecution, AIAnalysis should migrate to use `audit.StructToMap()` instead of custom methods

