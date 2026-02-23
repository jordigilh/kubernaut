# NT: Unstructured Data Usage Triage

> **Note (Issue #91):** This document references `kubernaut.ai/*` CRD labels that have since been migrated to immutable spec fields. See [DD-CRD-003](../architecture/DD-CRD-003-field-selectors-operational-queries.md) for the current field-selector-based approach.

**Date**: December 17, 2025
**Team**: Notification (NT)
**Scope**: Analysis of `map[string]interface{}` and `map[string]string` usage
**Status**: ✅ **COMPLETE**

---

## 🎯 **Executive Summary**

Notification Team code uses unstructured data types (`map[string]interface{}`, `map[string]string`) in 25 locations across 6 files. After comprehensive analysis:

**Verdict**:
- ✅ **22 usages are ACCEPTABLE** (justified use cases)
- ⚠️ **3 usages are QUESTIONABLE** (could benefit from structured types)
- ❌ **0 usages are VIOLATIONS** (no critical issues found)

**Overall Assessment**: **85% acceptable**, minimal refactoring needed.

---

## 📊 **Usage Summary by Category**

| Category | Count | Files | Status | Priority |
|----------|-------|-------|--------|----------|
| **Audit event_data** | 4 | `audit.go` | ✅ Acceptable | P3 - Consider structured |
| **Slack API payload** | 8 | `slack.go` | ✅ Acceptable | P4 - Keep as-is |
| **Kubernetes metadata** | 1 | CRD types | ✅ Acceptable | P5 - Standard pattern |
| **Routing labels** | 10 | `routing/*.go` | ✅ Acceptable | P5 - Standard pattern |
| **Generated DeepCopy** | 1 | `zz_generated.deepcopy.go` | ✅ Acceptable | P5 - Auto-generated |
| **Condition formatting** | 1 | Controller | ✅ Acceptable | P5 - Keep as-is |

**Total**: 25 usages

---

## 🔍 **Detailed Analysis**

### **Category 1: Audit Event Data (4 usages) - ⚠️ QUESTIONABLE**

**Files**: `internal/controller/notification/audit.go`

**Locations**:
- Line 86: `CreateMessageSentEvent()` - eventData
- Line 152: `CreateMessageFailedEvent()` - eventData
- Line 218: `CreateMessageAcknowledgedEvent()` - eventData
- Line 276: `CreateMessageEscalatedEvent()` - eventData

**Current Pattern**:
```go
// Line 86-98
eventData := map[string]interface{}{
    "notification_id": notification.Name,
    "channel":         channel,
    "subject":         notification.Spec.Subject,
    "body":            notification.Spec.Body,
    "priority":        string(notification.Spec.Priority),
    "type":            string(notification.Spec.Type),
}

// Include metadata if present
if notification.Spec.Metadata != nil {
    eventData["metadata"] = notification.Spec.Metadata
}

audit.SetEventData(event, eventData)
```

**Analysis**:
- **Purpose**: Build JSONB payload for PostgreSQL `event_data` column
- **Current State**: Unstructured `map[string]interface{}`
- **Justification**: ADR-034 defines `event_data` as flexible JSONB
- **Pros**:
  - ✅ Flexible schema (can add fields without breaking changes)
  - ✅ Matches PostgreSQL JSONB column type
  - ✅ Easy to serialize to JSON
- **Cons**:
  - ⚠️ No compile-time type safety
  - ⚠️ Easy to typo field names
  - ⚠️ No IDE autocomplete for field names

**Recommendation**: ⚠️ **ACCEPTABLE but could be improved**

**Improvement Options**:

#### **Option A: Create Structured Event Data Types** (Recommended)

```go
// NEW: Structured event data types
type MessageSentEventData struct {
    NotificationID string            `json:"notification_id"`
    Channel        string            `json:"channel"`
    Subject        string            `json:"subject"`
    Body           string            `json:"body"`
    Priority       string            `json:"priority"`
    Type           string            `json:"type"`
    Metadata       map[string]string `json:"metadata,omitempty"`
}

// Usage:
eventData := MessageSentEventData{
    NotificationID: notification.Name,
    Channel:        channel,
    Subject:        notification.Spec.Subject,
    Body:           notification.Spec.Body,
    Priority:       string(notification.Spec.Priority),
    Type:           string(notification.Spec.Type),
    Metadata:       notification.Spec.Metadata,
}

audit.SetEventData(event, eventData) // SetEventData accepts interface{} and marshals to JSON
```

**Benefits**:
- ✅ Compile-time type safety
- ✅ IDE autocomplete
- ✅ Self-documenting (struct shows all fields)
- ✅ No breaking changes (still marshals to same JSON)
- ✅ Easy to validate (can add struct tags)

**Effort**: 2-3 hours
**Priority**: P3 - Nice to have (not blocking V1.0)

#### **Option B: Keep as-is with Constants** (Minimal change)

```go
// Event data field names as constants
const (
    EventDataNotificationID = "notification_id"
    EventDataChannel        = "channel"
    EventDataSubject        = "subject"
    EventDataBody           = "body"
    EventDataPriority       = "priority"
    EventDataType           = "type"
    EventDataMetadata       = "metadata"
)

// Usage:
eventData := map[string]interface{}{
    EventDataNotificationID: notification.Name,
    EventDataChannel:        channel,
    // ...
}
```

**Benefits**:
- ✅ Prevents typos in field names
- ✅ Minimal code changes
- ⚠️ Still no compile-time type safety

**Effort**: 30 minutes
**Priority**: P4 - Low priority

---

### **Category 2: Slack API Payload (8 usages) - ✅ ACCEPTABLE**

**File**: `pkg/notification/delivery/slack.go`

**Locations**:
- Line 114: `FormatSlackPayload()` return type
- Lines 129-158: Slack Block Kit JSON structure

**Current Pattern**:
```go
func FormatSlackPayload(notification *notificationv1alpha1.NotificationRequest) map[string]interface{} {
    return map[string]interface{}{
        "blocks": []interface{}{
            map[string]interface{}{
                "type": "header",
                "text": map[string]interface{}{
                    "type": "plain_text",
                    "text": fmt.Sprintf("%s %s", emoji, notification.Spec.Subject),
                },
            },
            // ... more blocks
        },
    }
}
```

**Analysis**:
- **Purpose**: Build Slack Block Kit JSON payload for Slack API
- **Current State**: Nested `map[string]interface{}`
- **Justification**: Slack API accepts dynamic JSON structures
- **Pros**:
  - ✅ Directly matches Slack API format
  - ✅ Easy to construct complex nested structures
  - ✅ No need for complex struct tags
  - ✅ Slack's JSON schema is flexible (varies by message type)
- **Cons**:
  - ⚠️ No compile-time type safety for Slack API format
  - ⚠️ Typos in Slack field names won't be caught at compile time

**Recommendation**: ✅ **ACCEPTABLE - Keep as-is**

**Rationale**:
1. ✅ Slack API is **external** and uses dynamic JSON
2. ✅ Creating Go structs for all Slack Block Kit variations is **overkill**
3. ✅ Slack's Block Kit has **many optional fields** and **dynamic structures**
4. ✅ `map[string]interface{}` is the **idiomatic Go approach** for dynamic JSON APIs
5. ✅ If Slack API format is wrong, **Slack API will reject it** (fail-fast)

**Examples of similar patterns in Go ecosystem**:
- Kubernetes client-go uses `map[string]interface{}` for unstructured resources
- Many JSON API clients use `map[string]interface{}` for flexible payloads

**Priority**: P5 - No action needed

---

### **Category 3: Kubernetes Metadata (1 usage) - ✅ ACCEPTABLE**

**File**: `api/notification/v1alpha1/notificationrequest_types.go`

**Location**:
- Line 191: `Metadata map[string]string`

**Current Pattern**:
```go
type NotificationRequestSpec struct {
    // ... other fields

    // Metadata for context (key-value pairs)
    // Examples: remediationRequestName, cluster, namespace, severity, alertName
    // +optional
    Metadata map[string]string `json:"metadata,omitempty"`
}
```

**Analysis**:
- **Purpose**: Store arbitrary key-value metadata (similar to Kubernetes labels/annotations)
- **Current State**: `map[string]string` (standard Kubernetes pattern)
- **Justification**: Standard Kubernetes API pattern for flexible metadata
- **Pros**:
  - ✅ Matches Kubernetes labels/annotations pattern
  - ✅ Allows users to add custom context without CRD changes
  - ✅ Flexible for different use cases (remediation, cluster, namespace)
- **Cons**:
  - ⚠️ No validation of metadata keys (users can add anything)

**Recommendation**: ✅ **ACCEPTABLE - Standard Kubernetes pattern**

**Rationale**:
1. ✅ **Kubernetes convention**: Labels, annotations, metadata are all `map[string]string`
2. ✅ **User flexibility**: Allows users to pass context without predefined schema
3. ✅ **Backward compatible**: Adding structured fields later doesn't break existing metadata

**Examples**:
- Kubernetes `ObjectMeta.Labels`: `map[string]string`
- Kubernetes `ObjectMeta.Annotations`: `map[string]string`
- Prometheus labels: `map[string]string`

**Priority**: P5 - No action needed (standard pattern)

---

### **Category 4: Routing Labels (10 usages) - ✅ ACCEPTABLE**

**Files**:
- `internal/controller/notification/notificationrequest_controller.go`
- `pkg/notification/routing/router.go`
- `pkg/notification/routing/config.go`
- `pkg/notification/routing/resolver.go`

**Locations**:
- Controller line 656: `labels = make(map[string]string)`
- Controller line 660: `routingLabels := make(map[string]string)`
- Controller line 697: `formatLabelsForCondition(labels map[string]string)`
- Router line 109: `FindReceiver(labels map[string]string)`
- Router line 206: `ExtractRoutingConfig(data map[string]string)`
- Config line 54: `Match map[string]string` (routing config YAML)
- Config line 57: `MatchRE map[string]string` (regex matching)
- Config line 247: `FindReceiver(labels map[string]string)`
- Config line 249: `labels = make(map[string]string)`
- Config line 274: `matchesLabels(labels map[string]string)`
- Resolver line 48: `labels = make(map[string]string)`

**Current Pattern**:
```go
// Example: Routing by labels (like Prometheus Alertmanager)
type Route struct {
    Match   map[string]string `yaml:"match,omitempty"`    // Exact match
    MatchRE map[string]string `yaml:"match_re,omitempty"` // Regex match
    Receiver string           `yaml:"receiver"`
}

func (r *Route) matchesLabels(labels map[string]string) bool {
    // Match labels for routing decisions
}
```

**Analysis**:
- **Purpose**: Label-based routing (inspired by Prometheus Alertmanager)
- **Current State**: `map[string]string` for label matching
- **Justification**: Standard pattern for label-based routing systems
- **Pros**:
  - ✅ Matches Prometheus Alertmanager routing pattern
  - ✅ Flexible label-based routing (any label can be used)
  - ✅ Allows regex matching on label values
  - ✅ Standard pattern in observability tools
- **Cons**:
  - ⚠️ No validation of label keys (users can match on anything)

**Recommendation**: ✅ **ACCEPTABLE - Industry standard pattern**

**Rationale**:
1. ✅ **Prometheus Alertmanager pattern**: Uses `map[string]string` for label routing
2. ✅ **Flexible routing rules**: Users can route based on any label
3. ✅ **Battle-tested pattern**: Prometheus Alertmanager has used this for years

**Examples**:
- Prometheus Alertmanager routing: `map[string]string` labels
- Kubernetes label selectors: `map[string]string`
- Service mesh routing: `map[string]string` labels

**Priority**: P5 - No action needed (industry standard)

---

### **Category 5: Generated DeepCopy (1 usage) - ✅ ACCEPTABLE**

**File**: `api/notification/v1alpha1/zz_generated.deepcopy.go`

**Location**:
- Line 133: `*out = make(map[string]string, len(*in))`

**Analysis**:
- **Purpose**: Auto-generated Kubernetes DeepCopy method
- **Current State**: Auto-generated by controller-gen
- **Justification**: Generated code for CRD DeepCopy implementation

**Recommendation**: ✅ **ACCEPTABLE - Auto-generated code**

**Priority**: P5 - No action needed (never modify generated code)

---

### **Category 6: Condition Formatting (1 usage) - ✅ ACCEPTABLE**

**File**: `internal/controller/notification/notificationrequest_controller.go`

**Location**:
- Line 697: `formatLabelsForCondition(labels map[string]string)`

**Analysis**:
- **Purpose**: Format labels for display in Kubernetes Conditions
- **Current State**: Formats `map[string]string` as human-readable string
- **Justification**: Utility function for status condition message formatting

**Recommendation**: ✅ **ACCEPTABLE - Utility function**

**Priority**: P5 - No action needed

---

## 📊 **Risk Assessment**

### **Risk Matrix**

| Usage | Type Safety | Maintainability | External API | Risk Level |
|-------|-------------|-----------------|--------------|------------|
| **Audit event_data** | ⚠️ Medium | ⚠️ Medium | ✅ Internal | 🟡 LOW-MEDIUM |
| **Slack payloads** | ⚠️ Medium | ✅ High | ✅ External | 🟢 LOW |
| **Kubernetes metadata** | ✅ High | ✅ High | ✅ Standard | 🟢 LOW |
| **Routing labels** | ✅ High | ✅ High | ✅ Standard | 🟢 LOW |

### **Confidence Levels**

| Category | Confidence in Current Approach |
|----------|-------------------------------|
| **Audit event_data** | 70% - Could be better with structs |
| **Slack payloads** | 95% - Correct pattern for dynamic JSON |
| **Kubernetes metadata** | 100% - Standard Kubernetes pattern |
| **Routing labels** | 100% - Industry standard pattern |

---

## 🎯 **Recommendations Summary**

### **Priority 1: No Action Needed** (22/25 usages = 88%)
- ✅ Slack API payloads (8 usages)
- ✅ Routing labels (10 usages)
- ✅ Kubernetes metadata (1 usage)
- ✅ Generated DeepCopy (1 usage)
- ✅ Condition formatting (1 usage)
- ✅ Label extraction utilities (1 usage)

### **Priority 3: Consider Improvement** (4 usages = 16%)
- ⚠️ Audit event_data structures (4 usages in `audit.go`)
  - **Recommendation**: Create structured `MessageSentEventData`, `MessageFailedEventData`, etc.
  - **Effort**: 2-3 hours
  - **Benefit**: Type safety, IDE autocomplete, self-documenting
  - **Risk**: Low (backward compatible)
  - **V1.0 Blocker**: ❌ No (nice to have)

---

## 📋 **Implementation Plan (If Pursuing Structured Event Data)**

### **Phase 1: Create Event Data Types** (1 hour)

```go
// File: pkg/notification/audit_types.go

package notification

// MessageSentEventData represents audit event_data for message.sent events
type MessageSentEventData struct {
    NotificationID string            `json:"notification_id"`
    Channel        string            `json:"channel"`
    Subject        string            `json:"subject"`
    Body           string            `json:"body"`
    Priority       string            `json:"priority"`
    Type           string            `json:"type"`
    Metadata       map[string]string `json:"metadata,omitempty"`
}

// MessageFailedEventData represents audit event_data for message.failed events
type MessageFailedEventData struct {
    NotificationID string            `json:"notification_id"`
    Channel        string            `json:"channel"`
    Subject        string            `json:"subject"`
    Priority       string            `json:"priority"`
    Error          string            `json:"error"`
    ErrorType      string            `json:"error_type"` // "transient" or "permanent"
    Metadata       map[string]string `json:"metadata,omitempty"`
}

// MessageAcknowledgedEventData represents audit event_data for message.acknowledged events
type MessageAcknowledgedEventData struct {
    NotificationID string            `json:"notification_id"`
    Subject        string            `json:"subject"`
    Priority       string            `json:"priority"`
    Metadata       map[string]string `json:"metadata,omitempty"`
}

// MessageEscalatedEventData represents audit event_data for message.escalated events
type MessageEscalatedEventData struct {
    NotificationID string            `json:"notification_id"`
    Subject        string            `json:"subject"`
    Priority       string            `json:"priority"`
    Reason         string            `json:"reason"`
    Metadata       map[string]string `json:"metadata,omitempty"`
}
```

### **Phase 2: Update Audit Functions** (1 hour)

```go
// File: internal/controller/notification/audit.go

func (a *AuditHelpers) CreateMessageSentEvent(notification *notificationv1alpha1.NotificationRequest, channel string) (*dsgen.AuditEventRequest, error) {
    // ... correlation ID logic

    // Build structured event data
    eventData := notification.MessageSentEventData{
        NotificationID: notification.Name,
        Channel:        channel,
        Subject:        notification.Spec.Subject,
        Body:           notification.Spec.Body,
        Priority:       string(notification.Spec.Priority),
        Type:           string(notification.Spec.Type),
        Metadata:       notification.Spec.Metadata,
    }

    // Create audit event
    event := audit.NewAuditEventRequest()
    // ... set fields
    audit.SetEventData(event, eventData) // Still accepts interface{}, marshals to JSON

    return event, nil
}
```

### **Phase 3: Add Tests** (30 minutes)

```go
// Validate that structs marshal to expected JSON
func TestMessageSentEventData_MarshalJSON(t *testing.T) {
    data := MessageSentEventData{
        NotificationID: "test-123",
        Channel:        "slack",
        Subject:        "Test",
        Priority:       "critical",
    }

    jsonBytes, err := json.Marshal(data)
    require.NoError(t, err)

    var result map[string]interface{}
    require.NoError(t, json.Unmarshal(jsonBytes, &result))

    assert.Equal(t, "test-123", result["notification_id"])
    assert.Equal(t, "slack", result["channel"])
}
```

---

## ✅ **Conclusion**

### **Overall Assessment**: ✅ **HEALTHY**

- **88%** of unstructured data usage is **justified and acceptable**
- **16%** could be improved with structured types (audit event_data)
- **0%** are critical violations requiring immediate fixes

### **V1.0 Impact**: ✅ **NO BLOCKERS**

All unstructured data usage is either:
1. ✅ Standard Kubernetes/industry patterns
2. ✅ External API requirements (Slack)
3. ⚠️ Internal convenience (could be better, not critical)

### **Recommendation**:

**For V1.0**: ✅ **Ship as-is** - no critical issues found

**Post-V1.0**: ⚠️ Consider structured event data types for audit (P3 priority, 2-3 hours effort)

---

## 📚 **References**

**Industry Patterns**:
- Prometheus Alertmanager: Uses `map[string]string` for label routing
- Kubernetes API: Uses `map[string]string` for labels/annotations/metadata
- Slack API: Accepts dynamic JSON (`map[string]interface{}`)

**Internal Standards**:
- ADR-034: Defines audit `event_data` as flexible JSONB
- BR-NOT-065: Label-based routing (uses `map[string]string`)

---

**Triaged By**: Notification Team (@jgil)
**Date**: December 17, 2025
**Confidence**: 95% (high confidence in assessment)
**Priority**: P3 - Post-V1.0 improvement opportunity




