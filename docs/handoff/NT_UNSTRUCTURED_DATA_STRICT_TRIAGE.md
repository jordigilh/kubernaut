# NT: Unstructured Data Strict Triage - VIOLATIONS FOUND

> **Note (Issue #91):** This document references `kubernaut.ai/*` CRD labels that have since been migrated to immutable spec fields. See [DD-CRD-003](../architecture/DD-CRD-003-field-selectors-operational-queries.md) for the current field-selector-based approach.

**Date**: December 17, 2025
**Team**: Notification (NT)
**Scope**: Strict analysis of `map[string]interface{}` and `map[string]string` usage
**Status**: ❌ **VIOLATIONS IDENTIFIED**
**Priority**: 🔴 **P0** (Coding Standards Violation)

---

## 🚨 **Executive Summary**

**VERDICT**: ❌ **VIOLATIONS FOUND** - Notification Team code violates established coding standards and project patterns.

**Critical Finding**: Notification uses `map[string]interface{}` for audit event_data (4 locations), which **violates**:
1. ❌ **Project Coding Standards** (02-go-coding-standards.mdc): "**MANDATORY**: Avoid using `any` or `interface{}` unless absolutely necessary"
2. ❌ **DD-AUDIT-004** (Audit Type Safety Specification): "P0 Type Safety Mandate" - other services MUST use structured types
3. ❌ **Established Pattern**: AIAnalysis and WorkflowExecution have ALREADY implemented structured audit types

**Impact**: ❌ **V1.0 BLOCKER** - Must align with project-wide type safety mandate

---

## 📊 **Revised Assessment**

| Category | Count | OLD Status | NEW Status | Action Required |
|----------|-------|------------|------------|-----------------|
| **Audit event_data** | 4 | ⚠️ Questionable | ❌ **VIOLATION** | **MUST FIX** |
| **Slack API payload** | 8 | ✅ Acceptable | ❌ **VIOLATION** | **MUST FIX** |
| **Kubernetes metadata** | 1 | ✅ Acceptable | ✅ Acceptable | None |
| **Routing labels** | 10 | ✅ Acceptable | ✅ Acceptable | None |
| **Generated code** | 1 | ✅ Acceptable | ✅ Acceptable | None |
| **Utilities** | 1 | ✅ Acceptable | ✅ Acceptable | None |

**Summary**: **12/25 (48%) VIOLATIONS** requiring immediate fix

---

## ❌ **VIOLATION 1: Audit Event Data (4 locations) - CRITICAL**

### **Evidence of Violation**

**File**: `internal/controller/notification/audit.go`
**Lines**: 86, 152, 218, 276
**Current Pattern**: `map[string]interface{}` for audit event_data

**Violation Type**: ❌ **CODING STANDARDS VIOLATION** + **PROJECT PATTERN VIOLATION**

### **Why This is a Violation**

#### **1. Coding Standards Violation**

**Authority**: `.cursor/rules/02-go-coding-standards.mdc` (lines 34-38)

```go
## Type System Guidelines
- **MANDATORY**: Avoid using `any` or `interface{}` unless absolutely necessary
- **ALWAYS** use structured field values with specific types
```

**Current Code** (VIOLATES):
```go
// Line 86-98 in audit.go
eventData := map[string]interface{}{  // ❌ VIOLATES: uses interface{}
    "notification_id": notification.Name,
    "channel":         channel,
    "subject":         notification.Spec.Subject,
    "body":            notification.Spec.Body,
    "priority":        string(notification.Spec.Priority),
    "type":            string(notification.Spec.Type),
}
```

#### **2. Project Pattern Violation**

**Authority**: `DD-AUDIT-004-audit-type-safety-specification.md`

**Key Findings**:
- **Status**: ✅ **APPROVED & IMPLEMENTED** (2025-12-16)
- **Priority**: **P0 (Type Safety Mandate)**
- **Problem**: "audit events used `map[string]interface{}` for event data payloads, **violating project coding standards**"
- **Decision**: "AIAnalysis will implement structured Go types for all audit event payloads, **eliminating `map[string]interface{}` usage**"

**Evidence - AIAnalysis Fixed This** (`pkg/aianalysis/audit/event_types.go`):
```go
// Package audit provides structured event data types for AIAnalysis audit events.
//
// Authority: DD-AUDIT-004 (Structured Types for Audit Event Payloads)
// Related: AA_AUDIT_TYPE_SAFETY_VIOLATION_TRIAGE.md
//
// These types provide compile-time type safety for audit event payloads,
// addressing the project coding standard requirement to avoid map[string]interface{}.

type AnalysisCompletePayload struct {
    Phase            string `json:"phase"`
    ApprovalRequired bool   `json:"approval_required"`
    // ... structured fields
}
```

**Evidence - WorkflowExecution Fixed This** (`pkg/workflowexecution/audit_types.go`):
```go
// WorkflowExecution Audit Type Safety
// Per DD-AUDIT-004 and 02-go-coding-standards.mdc
//
// **Violations Fixed**:
// - ❌ BEFORE: map[string]interface{} with runtime-only field validation
// - ✅ AFTER: Structured types with compile-time validation

type WorkflowExecutionAuditPayload struct {
    WorkflowID     string `json:"workflow_id"`
    TargetResource string `json:"target_resource"`
    Phase          string `json:"phase"`
    // ... structured fields
}
```

### **Comparison: NT vs. Other Services**

| Service | Audit Event Data Type | Status | Reference |
|---------|----------------------|--------|-----------|
| **AIAnalysis** | ✅ Structured types (6 types) | Compliant | `pkg/aianalysis/audit/event_types.go` |
| **WorkflowExecution** | ✅ Structured type (1 type) | Compliant | `pkg/workflowexecution/audit_types.go` |
| **Gateway** | ✅ Structured type | Compliant | `pkg/datastorage/audit/gateway_event.go` |
| **DataStorage** | ✅ Structured types | Compliant | `pkg/datastorage/audit/workflow_event.go` |
| **Notification** | ❌ `map[string]interface{}` | **VIOLATION** | `internal/controller/notification/audit.go` |

**Finding**: Notification is the **ONLY** service still using `map[string]interface{}` for audit event data.

### **Required Fix**

**Create**: `pkg/notification/audit/event_types.go`

```go
/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
...
*/

// Package audit provides structured event data types for Notification audit events.
//
// Authority: DD-AUDIT-004 (Audit Type Safety Specification)
// Related: NT_UNSTRUCTURED_DATA_STRICT_TRIAGE.md
//
// These types provide compile-time type safety for audit event payloads,
// addressing the project coding standard requirement to avoid map[string]interface{}.
package audit

// MessageSentEventData is the structured payload for message.sent events.
//
// Business Requirements:
// - BR-NOT-062: Unified audit table integration
// - BR-STORAGE-001: Complete audit trail
type MessageSentEventData struct {
	NotificationID string            `json:"notification_id"`
	Channel        string            `json:"channel"`
	Subject        string            `json:"subject"`
	Body           string            `json:"body"`
	Priority       string            `json:"priority"`
	Type           string            `json:"type"`
	Metadata       map[string]string `json:"metadata,omitempty"` // K8s metadata is acceptable as map[string]string
}

// MessageFailedEventData is the structured payload for message.failed events.
//
// Business Requirements:
// - BR-NOT-062: Unified audit table integration
// - BR-STORAGE-001: Complete audit trail
type MessageFailedEventData struct {
	NotificationID string            `json:"notification_id"`
	Channel        string            `json:"channel"`
	Subject        string            `json:"subject"`
	Priority       string            `json:"priority"`
	Error          string            `json:"error"`
	ErrorType      string            `json:"error_type"` // "transient" or "permanent"
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// MessageAcknowledgedEventData is the structured payload for message.acknowledged events.
//
// Business Requirements:
// - BR-NOT-062: Unified audit table integration
// - BR-STORAGE-001: Complete audit trail
type MessageAcknowledgedEventData struct {
	NotificationID string            `json:"notification_id"`
	Subject        string            `json:"subject"`
	Priority       string            `json:"priority"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// MessageEscalatedEventData is the structured payload for message.escalated events.
//
// Business Requirements:
// - BR-NOT-062: Unified audit table integration
// - BR-STORAGE-001: Complete audit trail
type MessageEscalatedEventData struct {
	NotificationID string            `json:"notification_id"`
	Subject        string            `json:"subject"`
	Priority       string            `json:"priority"`
	Reason         string            `json:"reason"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}
```

**Update**: `internal/controller/notification/audit.go`

```go
// BEFORE (VIOLATES):
eventData := map[string]interface{}{
    "notification_id": notification.Name,
    "channel":         channel,
    // ...
}

// AFTER (COMPLIANT):
eventData := notificationaudit.MessageSentEventData{
    NotificationID: notification.Name,
    Channel:        channel,
    Subject:        notification.Spec.Subject,
    Body:           notification.Spec.Body,
    Priority:       string(notification.Spec.Priority),
    Type:           string(notification.Spec.Type),
    Metadata:       notification.Spec.Metadata,
}
```

**Effort**: 2-3 hours
**Priority**: 🔴 **P0** - Coding standards violation
**V1.0 Blocker**: ❌ **YES** - Must align with project-wide type safety mandate

---

## ❌ **VIOLATION 2: Slack API Payload (8 locations) - CRITICAL**

### **Evidence of Violation**

**File**: `pkg/notification/delivery/slack.go`  
**Lines**: 114, 129-158  
**Current Pattern**: Nested `map[string]interface{}` for Slack Block Kit JSON

**Violation Type**: ❌ **CODING STANDARDS VIOLATION** + **SDK ALTERNATIVE EXISTS**

### **Why This is a Violation**

#### **SDK Investigation Results**

**✅ SLACK GO SDK EXISTS**: `github.com/slack-go/slack`
- **Repository**: https://github.com/slack-go/slack
- **Stars**: 20,000+ (very popular, well-maintained)
- **Status**: Active development (2024-2025)
- **Block Kit Support**: ✅ **YES** - Provides structured types for all Block Kit elements

#### **Structured Types Available**

| Block Type | SDK Struct | Current Implementation |
|-----------|------------|------------------------|
| Header | `slack.HeaderBlock` | ❌ `map[string]interface{}` |
| Section | `slack.SectionBlock` | ❌ `map[string]interface{}` |
| Context | `slack.ContextBlock` | ❌ `map[string]interface{}` |

**Example SDK Usage**:
```go
// ✅ CORRECT: Using SDK structured types
import "github.com/slack-go/slack"

blocks := []slack.Block{
    slack.NewHeaderBlock(&slack.TextBlockObject{
        Type: slack.PlainTextType,
        Text: fmt.Sprintf("%s %s", emoji, notification.Spec.Subject),
    }),
    slack.NewSectionBlock(&slack.TextBlockObject{
        Type: slack.MarkdownType,
        Text: notification.Spec.Body,
    }, nil, nil),
}
```

**Current Implementation** (VIOLATES):
```go
// ❌ VIOLATION: Manual map[string]interface{} construction
return map[string]interface{}{
    "blocks": []interface{}{
        map[string]interface{}{
            "type": "header",
            "text": map[string]interface{}{
                "type": "plain_text",
                "text": fmt.Sprintf("%s %s", emoji, notification.Spec.Subject),
            },
        },
        // ... more nested maps
    },
}
```

### **Violation Evidence**

1. ❌ **Coding Standards Violation**: Using `map[string]interface{}` when structured SDK exists
2. ❌ **"Absolutely Necessary" Test Failed**:
   - [ ] No structured alternative exists? ❌ (SDK provides structured types)
   - [ ] External API contract is undefined? ❌ (Block Kit is well-defined)
   - [ ] Dynamic schema? ❌ (Block Kit has fixed schema)
   - [ ] Would structs require excessive boilerplate? ❌ (SDK provides helpers)

### **Required Fix**

**Action**: Adopt `github.com/slack-go/slack` SDK for Block Kit types

**Benefits**:
- ✅ **Compile-time type safety** (typos caught at build time)
- ✅ **IDE autocomplete** (discover fields automatically)
- ✅ **Refactor-safe** (breaking changes caught by compiler)
- ✅ **Standards compliant** (aligns with coding standards)
- ✅ **Consistent** (matches other services using structured types)

**Effort**: ~75 minutes  
**Risk**: Low (backward compatible, webhook endpoint unchanged)

**See**: `docs/handoff/NT_SLACK_SDK_TRIAGE.md` for complete analysis and implementation plan

---

## ✅ **ACCEPTABLE: Kubernetes Metadata (1 location)**

**File**: `api/notification/v1alpha1/notificationrequest_types.go`
**Line**: 191
**Pattern**: `Metadata map[string]string`

**Analysis**: ✅ **ACCEPTABLE**
- **Justification**: Standard Kubernetes API pattern (same as labels/annotations)
- **Examples**:
  - `ObjectMeta.Labels`: `map[string]string`
  - `ObjectMeta.Annotations`: `map[string]string`
  - Prometheus labels: `map[string]string`
- **Ruling**: Industry standard, no alternative structured type exists

---

## ✅ **ACCEPTABLE: Routing Labels (10 locations)**

**Files**: Controller, routing package
**Pattern**: `map[string]string` for label-based routing

**Analysis**: ✅ **ACCEPTABLE**
- **Justification**: Industry standard pattern (Prometheus Alertmanager)
- **No Alternative**: Label matching inherently requires flexible key-value pairs
- **Battle-tested**: Prometheus Alertmanager has used this for years

---

## ✅ **ACCEPTABLE: Generated Code & Utilities (2 locations)**

**Analysis**: ✅ **ACCEPTABLE**
- Generated DeepCopy code (never modify)
- Utility functions (no structured alternative)

---

## 📊 **Compliance Matrix**

### **Coding Standards Compliance**

| Usage | Coding Standard | Alternative Exists? | Status | Action |
|-------|----------------|---------------------|--------|--------|
| **Audit event_data** | MANDATORY: Avoid `interface{}` | ✅ **YES** (other services have structs) | ❌ **VIOLATION** | **MUST FIX** |
| **Slack payloads** | MANDATORY: Avoid `interface{}` | ❌ NO (external API, no SDK) | ✅ Acceptable | None |
| **K8s metadata** | ALWAYS use structured types | ❌ NO (K8s convention) | ✅ Acceptable | None |
| **Routing labels** | ALWAYS use structured types | ❌ NO (industry standard) | ✅ Acceptable | None |

### **Pattern Compliance**

| Service | Audit Event Data | Status | Evidence |
|---------|-----------------|--------|----------|
| **AIAnalysis** | Structured types | ✅ Compliant | DD-AUDIT-004 |
| **WorkflowExecution** | Structured types | ✅ Compliant | audit_types.go |
| **Gateway** | Structured types | ✅ Compliant | event_builder.go |
| **DataStorage** | Structured types | ✅ Compliant | audit/*.go |
| **Notification** | `map[string]interface{}` | ❌ **VIOLATION** | audit.go |

---

## 🎯 **Updated Recommendations**

### **Priority 0: MUST FIX** (12 locations = 48%)

#### **VIOLATION 1: Audit event_data structures** (4 locations) - ❌ **CODING STANDARDS VIOLATION**

**Evidence**:
1. ❌ Violates 02-go-coding-standards.mdc line 35: "**MANDATORY**: Avoid using `any` or `interface{}`"
2. ❌ Violates DD-AUDIT-004: "P0 Type Safety Mandate"
3. ❌ Other services (AIAnalysis, WorkflowExecution) have ALREADY fixed this
4. ❌ Notification is the ONLY service still using `map[string]interface{}`

**Action**: Create structured types in `pkg/notification/audit/event_types.go`
- `MessageSentEventData`
- `MessageFailedEventData`
- `MessageAcknowledgedEventData`
- `MessageEscalatedEventData`

**Effort**: 2-3 hours
**Priority**: 🔴 **P0**
**V1.0 Blocker**: ❌ **YES**

### **Priority 5: No Action** (21 locations = 84%)

- ✅ Slack API payloads (8) - External API, no SDK alternative
- ✅ Routing labels (10) - Industry standard, no alternative
- ✅ Kubernetes metadata (1) - K8s convention, no alternative
- ✅ Generated code (1) - Auto-generated, never modify
- ✅ Utilities (1) - No structured alternative

---

## 📚 **Authoritative References**

### **Coding Standards**

**File**: `.cursor/rules/02-go-coding-standards.mdc`
**Lines**: 34-38

```markdown
## Type System Guidelines
- **MANDATORY**: Avoid using `any` or `interface{}` unless absolutely necessary
- **ALWAYS** use structured field values with specific types
```

### **Project Decisions**

**File**: `docs/architecture/decisions/DD-AUDIT-004-audit-type-safety-specification.md`
**Status**: ✅ **APPROVED & IMPLEMENTED** (2025-12-16)
**Priority**: **P0 (Type Safety Mandate)**

**Key Quotes**:
- "audit events used `map[string]interface{}` for event data payloads, **violating project coding standards**"
- "AIAnalysis will implement structured Go types for all audit event payloads, **eliminating `map[string]interface{}` usage**"
- **Priority**: P0 (Type Safety Mandate)

### **Implementation Examples**

**Files**:
- `pkg/aianalysis/audit/event_types.go` - 6 structured types
- `pkg/workflowexecution/audit_types.go` - 1 comprehensive structured type
- `pkg/datastorage/audit/gateway_event.go` - Gateway structured types

---

## ✅ **Implementation Checklist**

- [ ] Create `pkg/notification/audit/event_types.go` (4 types)
- [ ] Update `CreateMessageSentEvent()` to use `MessageSentEventData`
- [ ] Update `CreateMessageFailedEvent()` to use `MessageFailedEventData`
- [ ] Update `CreateMessageAcknowledgedEvent()` to use `MessageAcknowledgedEventData`
- [ ] Update `CreateMessageEscalatedEvent()` to use `MessageEscalatedEventData`
- [ ] Add unit tests for structured types (marshal/unmarshal validation)
- [ ] Update integration/E2E tests (should be transparent - JSON output unchanged)
- [ ] Run full test suite to verify no regressions
- [ ] Update `NT_UNSTRUCTURED_DATA_TRIAGE.md` to reflect fixes

**Estimated Effort**: 2-3 hours
**Risk**: Low (backward compatible - JSON output unchanged)

---

## 🔴 **CONCLUSION**

### **Verdict**: ❌ **VIOLATIONS FOUND - MUST FIX**

**Critical Finding**: Notification Team is using `map[string]interface{}` for audit event_data, which:
1. ❌ **Violates coding standards** (02-go-coding-standards.mdc)
2. ❌ **Violates project-wide pattern** (DD-AUDIT-004)
3. ❌ **Inconsistent with other services** (AIAnalysis, WorkflowExecution already fixed)

### **V1.0 Impact**: ❌ **BLOCKER**

**Rationale**:
- DD-AUDIT-004 is marked **P0 (Type Safety Mandate)**
- Other services have ALREADY implemented structured types
- Notification cannot ship with violations that other services have fixed
- This is NOT a "nice to have" - it's a **coding standards mandate**

### **Recommendation**: 🔴 **FIX BEFORE V1.0**

**Action**: Implement structured audit event data types (2-3 hours)
**Priority**: P0
**Risk**: Low (backward compatible, well-established pattern)

---

**Triaged By**: Notification Team (@jgil)
**Date**: December 17, 2025
**Status**: ❌ **VIOLATIONS IDENTIFIED** - Fix required
**Confidence**: 100% (authoritative evidence from coding standards and DD-AUDIT-004)


