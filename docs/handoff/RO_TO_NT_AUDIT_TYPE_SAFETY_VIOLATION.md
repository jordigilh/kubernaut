# RO Team → NT Team: P0 Audit Type Safety Violation Detected

**From**: RO Team
**To**: NT Team
**Date**: December 17, 2025
**Priority**: 🔴 **P0 (V1.0 Blocker)**
**Type**: Cross-Team Notification

---

## 🚨 **Issue Discovered**

While implementing RO routing blocked audit events, we discovered that **Notification service is using `map[string]interface{}` for audit event data**, which violates:

1. ❌ **02-go-coding-standards.mdc** (line 35): "MANDATORY: Avoid using `any` or `interface{}` unless absolutely necessary"
2. ❌ **DD-AUDIT-004**: "P0 Type Safety Mandate" - all services MUST use structured types
3. ❌ **Project-wide pattern**: All other services have already fixed this

---

## 📊 **Evidence**

**File**: `internal/controller/notification/audit.go`
**Lines**: 86, 152, 218, 276

**Current Code** (VIOLATES):
```go
// ❌ CODING STANDARDS VIOLATION
eventData := map[string]interface{}{
    "notification_id": notification.Name,
    "channel":         channel,
    "subject":         notification.Spec.Subject,
    // ...
}
audit.SetEventData(event, eventData)
```

---

## 📋 **Service Compliance Status**

| Service | Audit Event Data Type | Status |
|---|---|---|
| **AIAnalysis** | Structured types (6 types) | ✅ Compliant |
| **WorkflowExecution** | Structured types + ToMap() | ✅ Compliant |
| **Gateway** | Structured types | ✅ Compliant |
| **DataStorage** | Structured types | ✅ Compliant |
| **RemediationOrchestrator** | Structured types | ✅ Compliant |
| **Notification** | `map[string]interface{}` | ❌ **VIOLATION** |

**Finding**: Notification is the **ONLY** service still using `map[string]interface{}`

---

## ✅ **Recommended Solution**

### **Step 1**: Create `pkg/notification/audit/event_types.go`

Follow the **WorkflowExecution pattern** (`pkg/workflowexecution/audit_types.go`):

```go
package audit

// MessageSentEventData is the structured payload for message.sent events.
type MessageSentEventData struct {
	NotificationID string            `json:"notification_id"`
	Channel        string            `json:"channel"`
	Subject        string            `json:"subject"`
	Body           string            `json:"body"`
	Priority       string            `json:"priority"`
	Type           string            `json:"type"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// ToMap converts to map[string]interface{} for audit.SetEventData
func (m MessageSentEventData) ToMap() map[string]interface{} {
	result := map[string]interface{}{
		"notification_id": m.NotificationID,
		"channel":         m.Channel,
		"subject":         m.Subject,
		"body":            m.Body,
		"priority":        m.Priority,
		"type":            m.Type,
	}

	if m.Metadata != nil {
		result["metadata"] = m.Metadata
	}

	return result
}
```

**Create 4 types**:
1. `MessageSentEventData`
2. `MessageFailedEventData`
3. `MessageAcknowledgedEventData`
4. `MessageEscalatedEventData`

Each with a `ToMap()` method.

---

### **Step 2**: Update `internal/controller/notification/audit.go`

**Before** (VIOLATES):
```go
eventData := map[string]interface{}{
    "notification_id": notification.Name,
    // ...
}
audit.SetEventData(event, eventData)
```

**After** (COMPLIANT):
```go
eventData := notificationaudit.MessageSentEventData{
    NotificationID: notification.Name,
    Channel:        channel,
    Subject:        notification.Spec.Subject,
    Body:           notification.Spec.Body,
    Priority:       string(notification.Spec.Priority),
    Type:           string(notification.Spec.Type),
    Metadata:       notification.Spec.Metadata,
}
audit.SetEventData(event, eventData.ToMap())
```

---

## 📚 **Pattern References**

### **Example 1: WorkflowExecution** (BEST REFERENCE)

**File**: `pkg/workflowexecution/audit_types.go`

```go
type WorkflowExecutionAuditPayload struct {
	WorkflowID     string `json:"workflow_id"`
	TargetResource string `json:"target_resource"`
	Phase          string `json:"phase"`
	// ...
}

func (p WorkflowExecutionAuditPayload) ToMap() map[string]interface{} {
	result := map[string]interface{}{
		"workflow_id":     p.WorkflowID,
		"target_resource": p.TargetResource,
		"phase":           p.Phase,
		// ...
	}
	return result
}
```

**Usage**:
```go
payload := WorkflowExecutionAuditPayload{...}
audit.SetEventData(event, payload.ToMap())
```

---

### **Example 2: AIAnalysis**

**File**: `pkg/aianalysis/audit/event_types.go`

6 structured types for AI analysis events.

---

## 🎯 **Benefits**

**Before** ❌:
- ❌ Field name typos only caught at runtime
- ❌ No IDE autocomplete
- ❌ Refactoring breaks events silently

**After** ✅:
- ✅ Compile-time type safety
- ✅ IDE autocomplete for all fields
- ✅ Refactor-safe (compiler catches breaks)
- ✅ Aligned with all other services

---

## ⏱️ **Estimated Effort**

**Total**: 2-3 hours

1. Create `event_types.go` with 4 types + ToMap methods: **1.5 hours**
2. Update 4 audit helper functions: **30 minutes**
3. Test compilation and verify: **30 minutes**

---

## 🚨 **V1.0 Impact**

**Status**: ❌ **BLOCKER**

**Rationale**:
- DD-AUDIT-004 is marked **P0 (Type Safety Mandate)**
- All other services have already implemented structured types
- Cannot ship with violations that other services have fixed

---

## 📋 **Action Items for NT Team**

- [ ] Review this document
- [ ] Create `pkg/notification/audit/event_types.go` (4 types)
- [ ] Add `ToMap()` methods to all 4 types
- [ ] Update `CreateMessageSentEvent()` in audit.go
- [ ] Update `CreateMessageFailedEvent()` in audit.go
- [ ] Update `CreateMessageAcknowledgedEvent()` in audit.go
- [ ] Update `CreateMessageEscalatedEvent()` in audit.go
- [ ] Verify compilation succeeds
- [ ] Run linter
- [ ] Update `NT_UNSTRUCTURED_DATA_STRICT_TRIAGE.md` to mark as fixed

---

## 📚 **Authoritative References**

1. **Coding Standards**: `.cursor/rules/02-go-coding-standards.mdc` (lines 34-38)
2. **Design Decision**: `docs/architecture/decisions/DD-AUDIT-004-audit-type-safety-specification.md`
3. **Triage Document**: `docs/handoff/NT_UNSTRUCTURED_DATA_STRICT_TRIAGE.md`

---

## 🤝 **Support Available**

If you have questions:
- Reference WorkflowExecution pattern: `pkg/workflowexecution/audit_types.go`
- Reference AIAnalysis pattern: `pkg/aianalysis/audit/event_types.go`
- See DD-AUDIT-004 for full rationale

---

**Prepared by**: RO Team
**Date**: December 17, 2025
**Status**: ⏳ Awaiting NT Team Implementation

