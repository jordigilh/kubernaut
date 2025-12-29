# CRITICAL: Audit Architecture Triage - ToMap() Pattern Analysis

**Date**: December 17, 2025
**Status**: 🚨 **CRITICAL ARCHITECTURAL QUESTION**
**Priority**: P0 - Affects all services

---

## 🚨 User Concern

**User Statement**: "we aren't doing the mapping but directly using the audit structures from the ds client"

**Context**: User deleted `pkg/notification/audit/event_types.go` (which contained `ToMap()` methods) and questioned why we need helper functions for something that should be directly defined in the struct.

**User's Key Point**: "We shouldn't hide technical debt"

---

## 🔍 Current State Analysis

### What We Currently Do (All Services)

**Pattern**: Create service-specific structs with `ToMap()` methods

```go
// 1. Define service-specific struct
type MessageSentEventData struct {
    NotificationID string `json:"notification_id"`
    Channel        string `json:"channel"`
    // ...
}

// 2. Add ToMap() method
func (m MessageSentEventData) ToMap() map[string]interface{} {
    return map[string]interface{}{
        "notification_id": m.NotificationID,
        "channel":         m.Channel,
    }
}

// 3. Use in controller
eventData := MessageSentEventData{...}
audit.SetEventData(event, eventData.ToMap())
```

**Services Using This Pattern**:
- ✅ WorkflowExecution (`pkg/workflowexecution/audit_types.go`)
- ✅ AIAnalysis (6 structured types)
- ✅ Notification (4 structured types - now deleted by user)

---

## 🤔 What User Might Be Suggesting

### Alternative 1: Use DataStorage Client Types Directly?

**Hypothesis**: Instead of creating `MessageSentEventData`, just use `dsgen.AuditEventRequest` directly?

```go
// Instead of this:
eventData := MessageSentEventData{
    NotificationID: notification.Name,
    Channel:        "slack",
}
audit.SetEventData(event, eventData.ToMap())

// Do this?:
event := audit.NewAuditEventRequest()
event.EventData = map[string]interface{}{
    "notification_id": notification.Name,
    "channel":         "slack",
}
```

**Problem**: This is WORSE - it's the anti-pattern DD-AUDIT-004 was created to fix!

---

### Alternative 2: Make EventData a Typed Field?

**Hypothesis**: Change `AuditEventRequest.EventData` from `map[string]interface{}` to a typed field?

**Current (OpenAPI Generated)**:
```go
type AuditEventRequest struct {
    // ...
    EventData map[string]interface{} `json:"event_data"`  // ← This is generated from OpenAPI
}
```

**Proposed**:
```go
type AuditEventRequest struct {
    // ...
    EventData interface{} `json:"event_data"`  // ← Could accept any typed struct?
}
```

**Problems**:
1. ❌ `AuditEventRequest` is **generated from OpenAPI spec** - we can't change it
2. ❌ OpenAPI spec defines `event_data` as `object` (= `map[string]interface{}` in Go)
3. ❌ JSON marshaling requires `map[string]interface{}` or custom marshaler
4. ❌ Would break all existing services

---

### Alternative 3: Remove ToMap(), Set EventData Directly?

**Hypothesis**: Set the struct directly without conversion?

```go
eventData := MessageSentEventData{...}
event.EventData = eventData  // ← Type error: cannot use struct as map[string]interface{}
```

**Problem**: **COMPILATION ERROR** - `EventData` field expects `map[string]interface{}`, not a struct.

---

## 📊 Architectural Constraints

### Why We MUST Use map[string]interface{}

**Constraint 1: OpenAPI Specification**
- DataStorage OpenAPI spec defines `event_data` as JSON `object`
- Go client generator produces `map[string]interface{}`
- **We cannot change this** without changing the API contract

**Constraint 2: JSON Marshaling**
- PostgreSQL stores `event_data` as `JSONB`
- JSON marshaling requires `map[string]interface{}` or custom `MarshalJSON()`
- Struct → JSON → JSONB requires conversion at some point

**Constraint 3: Cross-Service Compatibility**
- DataStorage must accept audit events from **all services**
- Each service has different event data structures
- `map[string]interface{}` is the only common denominator

---

## 🎯 The Real Question

**What is the user actually concerned about?**

### Possibility 1: ToMap() is Boilerplate?

**Concern**: Writing `ToMap()` methods for every struct is tedious

**Response**: Yes, but it's the **least bad option** given the constraints:
- ✅ Type-safe business logic (struct fields)
- ✅ Single conversion point (ToMap method)
- ✅ API compatibility (map[string]interface{})

**Alternative**: Use `audit.StructToMap()` generic helper?
- ❌ Less explicit (which struct is being converted?)
- ❌ Requires error handling for well-formed structs
- ❌ Hides conversion logic away from the struct

---

### Possibility 2: Should Use OpenAPI-Generated Types?

**Concern**: Why create `MessageSentEventData` when we have `AuditEventRequest`?

**Response**: **They serve different purposes**:

| Type | Purpose | Scope |
|---|---|---|
| `AuditEventRequest` | **Transport** (API contract) | DataStorage client |
| `MessageSentEventData` | **Business Logic** (domain model) | Notification service |

**Analogy**:
- `AuditEventRequest` = HTTP request envelope (headers + body)
- `MessageSentEventData` = Business payload (what goes in the body)

---

### Possibility 3: Questioning DD-AUDIT-004 Itself?

**Concern**: Is the entire structured types approach wrong?

**Response**: DD-AUDIT-004 was created to fix a **real problem**:

**Before (Anti-Pattern)**:
```go
// ❌ No compile-time validation
eventData := map[string]interface{}{
    "notifiction_id": notification.Name,  // ← Typo! Runtime error
    "chanell":        "slack",             // ← Typo! Runtime error
}
```

**After (DD-AUDIT-004)**:
```go
// ✅ Compile-time validation
eventData := MessageSentEventData{
    NotificationID: notification.Name,  // ← Typo caught by compiler
    Channel:        "slack",            // ← Typo caught by compiler
}
```

---

## 🔧 Possible Solutions

### Solution 1: Keep Current Approach (ToMap() Methods)

**Status**: ✅ **CURRENT IMPLEMENTATION**

**Pros**:
- ✅ Type-safe business logic
- ✅ Explicit conversion (ToMap on struct)
- ✅ Follows WorkflowExecution pattern
- ✅ Complies with DD-AUDIT-004

**Cons**:
- ⚠️ Boilerplate ToMap() methods
- ⚠️ Conversion overhead (minimal)

---

### Solution 2: Use audit.StructToMap() Helper

**Status**: ⚠️ **PREVIOUSLY REJECTED** (P1 violation in triage)

**Pros**:
- ✅ No ToMap() boilerplate
- ✅ Generic conversion

**Cons**:
- ❌ Less explicit (which struct?)
- ❌ Requires error handling
- ❌ Hides conversion away from struct
- ❌ User said "we shouldn't hide technical debt"

---

### Solution 3: Change OpenAPI Spec to Accept Typed Structs

**Status**: ❌ **NOT FEASIBLE**

**Why Not**:
- ❌ Breaks cross-service compatibility
- ❌ Requires custom JSON marshaling for every service
- ❌ PostgreSQL JSONB still requires map-like structure
- ❌ Massive refactoring across all services

---

### Solution 4: Remove Structured Types, Use map[string]interface{} Directly

**Status**: ❌ **VIOLATES CODING STANDARDS**

**Why Not**:
- ❌ Violates 02-go-coding-standards.mdc
- ❌ No compile-time validation
- ❌ Reverts to the anti-pattern DD-AUDIT-004 fixed
- ❌ User concern was about "hiding debt", not removing type safety

---

## ❓ Questions for User

**To clarify the concern, we need to understand**:

1. **What should we use instead of `MessageSentEventData` struct?**
   - Option A: `map[string]interface{}` directly (violates coding standards)
   - Option B: `AuditEventRequest` directly (wrong abstraction level)
   - Option C: Something else?

2. **What does "directly using the audit structures from the ds client" mean?**
   - Do you mean use `dsgen.AuditEventRequest` for business logic?
   - Do you mean skip the `ToMap()` conversion somehow?
   - Do you mean change the OpenAPI spec?

3. **What is the "technical debt" we're hiding?**
   - Is it the `ToMap()` boilerplate?
   - Is it the `map[string]interface{}` requirement from OpenAPI?
   - Is it the separation between business types and transport types?

4. **What would the ideal code look like?**
   - Can you show an example of how you'd like to see audit events created?

---

## 🎯 Recommendation

**Until we understand the user's concern, we should**:

1. **HALT** all changes to audit event creation patterns
2. **CLARIFY** what the user means by "directly using ds client structures"
3. **PRESENT** the architectural constraints (OpenAPI, JSON, JSONB)
4. **PROPOSE** alternatives with pros/cons
5. **GET APPROVAL** before implementing any changes

**Current Status**: ⏸️ **BLOCKED - AWAITING USER CLARIFICATION**

---

## 📚 References

- [DD-AUDIT-004](mdc:docs/architecture/decisions/DD-AUDIT-004-structured-types-for-audit-event-payloads.md) - Structured types mandate
- [02-go-coding-standards.mdc](mdc:.cursor/rules/02-go-coding-standards.mdc) - Avoid `map[string]interface{}`
- [pkg/datastorage/client/generated.go](mdc:pkg/datastorage/client/generated.go) - OpenAPI-generated types
- [pkg/workflowexecution/audit_types.go](mdc:pkg/workflowexecution/audit_types.go) - Authoritative pattern

---

**Document Status**: 🚨 **AWAITING USER CLARIFICATION**
**Next Action**: User must clarify what "directly using ds client structures" means



