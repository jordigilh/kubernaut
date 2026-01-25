# Missing OpenAPI Schemas for EventData - Audit Trail

**Date**: January 8, 2026
**Context**: Complete OpenAPI schema coverage for all audit event payloads
**Status**: 🔍 **DISCOVERY** - Found 11 missing schemas + 2 using unstructured data

---

## 📊 **Current Status**

### ✅ **Already in OpenAPI Spec (9 schemas)**

| Event Type Pattern | Schema | Service |
|---|---|---|
| `gateway.*` | `GatewayAuditPayload` | Gateway |
| `orchestrator.lifecycle.*` | `RemediationOrchestratorAuditPayload` | RemediationOrchestrator |
| `signalprocessing.*` | `SignalProcessingAuditPayload` | SignalProcessing |
| `aianalysis.analysis.completed/failed` | `AIAnalysisAuditPayload` | AIAnalysis (top-level) |
| `workflowexecution.workflow.*` | `WorkflowExecutionAuditPayload` | WorkflowExecution |
| `webhook.notification.cancelled` | `NotificationAuditPayload` | Webhooks (Notification) |
| `webhook.notification.acknowledged` | `NotificationAuditPayload` | Webhooks (Notification) |
| `webhook.workflow.unblocked` | `WorkflowExecutionWebhookAuditPayload` | Webhooks (WorkflowExecution) |
| `webhook.approval.decided` | `RemediationApprovalAuditPayload` | Webhooks (Approval) |
| `workflow.catalog.search_completed` | `WorkflowSearchAuditPayload` | DataStorage (just added) |

---

## ❌ **MISSING from OpenAPI Spec (12 schemas)**

### 0. **Webhook Deletion Event (1 schema) - CRITICAL**

**Location**: `pkg/authwebhook/notificationrequest_validator.go`  
**Current State**: ✅ Using structured `NotificationAuditPayload` BUT ❌ Event type NOT in discriminator

| Event Type | Go Struct | Status |
|---|---|---|
| `notification.request.deleted` | `NotificationAuditPayload` | ⚠️ **Missing from discriminator mapping** |

**Issue**: Code emits `notification.request.deleted` but OpenAPI only has `webhook.notification.cancelled` and `webhook.notification.acknowledged` in the discriminator.

**Required Action**: Add `'notification.request.deleted': '#/components/schemas/NotificationAuditPayload'` to discriminator mapping.

---

### 1. AIAnalysis Internal Events (5 schemas)

**Location**: `pkg/aianalysis/audit/event_types.go`
**Current State**: ❌ Manually defined Go structs, NOT in OpenAPI spec

| Event Type | Go Struct | Current Usage |
|---|---|---|
| `aianalysis.phase.transition` | `PhaseTransitionPayload` | `audit.SetEventData(event, payload)` |
| `aianalysis.holmesgpt.call` | `HolmesGPTCallPayload` | `audit.SetEventData(event, payload)` |
| `aianalysis.approval.decision` | `ApprovalDecisionPayload` | `audit.SetEventData(event, payload)` |
| `aianalysis.rego.evaluation` | `RegoEvaluationPayload` | `audit.SetEventData(event, payload)` |
| `aianalysis.error.occurred` | `ErrorPayload` | `audit.SetEventData(event, payload)` |

**Fields Summary**:
```go
// PhaseTransitionPayload
type PhaseTransitionPayload struct {
    OldPhase string `json:"old_phase"`
    NewPhase string `json:"new_phase"`
}

// HolmesGPTCallPayload
type HolmesGPTCallPayload struct {
    Endpoint       string `json:"endpoint"`
    HTTPStatusCode int    `json:"http_status_code"`
    DurationMs     int    `json:"duration_ms"`
}

// ApprovalDecisionPayload
type ApprovalDecisionPayload struct {
    ApprovalRequired bool     `json:"approval_required"`
    ApprovalReason   string   `json:"approval_reason"`
    AutoApproved     bool     `json:"auto_approved"`
    Decision         string   `json:"decision"`
    Reason           string   `json:"reason"`
    Environment      string   `json:"environment"`
    Confidence       *float64 `json:"confidence,omitempty"`
    WorkflowID       *string  `json:"workflow_id,omitempty"`
}

// RegoEvaluationPayload
type RegoEvaluationPayload struct {
    Outcome    string `json:"outcome"`
    Degraded   bool   `json:"degraded"`
    DurationMs int    `json:"duration_ms"`
    Reason     string `json:"reason"`
}

// ErrorPayload
type ErrorPayload struct {
    Phase        string `json:"phase"`
    ErrorMessage string `json:"error_message"`
}
```

---

### 2. Notification Events (4 schemas)

**Location**: `pkg/notification/audit/event_types.go`
**Current State**: ❌ Manually defined Go structs, NOT in OpenAPI spec

| Event Type | Go Struct | Current Usage |
|---|---|---|
| `notification.message.sent` | `MessageSentEventData` | `audit.StructToMap(payload)` |
| `notification.message.failed` | `MessageFailedEventData` | `audit.StructToMap(payload)` |
| `notification.message.acknowledged` | `MessageAcknowledgedEventData` | `audit.StructToMap(payload)` |
| `notification.message.escalated` | `MessageEscalatedEventData` | `audit.StructToMap(payload)` |

**Fields Summary**:
```go
// MessageSentEventData
type MessageSentEventData struct {
    NotificationID string            `json:"notification_id"`
    Channel        string            `json:"channel"`
    Subject        string            `json:"subject"`
    Body           string            `json:"body"`
    Priority       string            `json:"priority"`
    Type           string            `json:"type"`
    Metadata       map[string]string `json:"metadata,omitempty"`
}

// MessageFailedEventData
type MessageFailedEventData struct {
    NotificationID string            `json:"notification_id"`
    Channel        string            `json:"channel"`
    FailureReason  string            `json:"failure_reason"`
    ErrorMessage   string            `json:"error_message"`
    RetryCount     int               `json:"retry_count"`
    Metadata       map[string]string `json:"metadata,omitempty"`
}

// MessageAcknowledgedEventData
type MessageAcknowledgedEventData struct {
    NotificationID   string `json:"notification_id"`
    Channel          string `json:"channel"`
    AcknowledgedBy   string `json:"acknowledged_by"`
    AcknowledgedAt   string `json:"acknowledged_at"`
    AcknowledgeToken string `json:"acknowledge_token"`
}

// MessageEscalatedEventData
type MessageEscalatedEventData struct {
    NotificationID    string   `json:"notification_id"`
    FromChannel       string   `json:"from_channel"`
    ToChannel         string   `json:"to_channel"`
    EscalationReason  string   `json:"escalation_reason"`
    FailedAttempts    int      `json:"failed_attempts"`
    EscalationLevel   int      `json:"escalation_level"`
    PreviousRecipients []string `json:"previous_recipients"`
}
```

---

## ⚠️ **USING UNSTRUCTURED DATA (2 events - HIGHEST PRIORITY)**

### 3. DataStorage Workflow Catalog Events (2 events)

**Location**: `pkg/datastorage/audit/workflow_catalog_event.go`
**Current State**: ⚠️ **USING `map[string]interface{}` + envelope pattern**

| Event Type | Current Implementation | Problem |
|---|---|---|
| `datastorage.workflow.created` | `map[string]interface{}` → `EnvelopeToMap()` | ❌ No type safety |
| `datastorage.workflow.updated` | `map[string]interface{}` → `EnvelopeToMap()` | ❌ No type safety |

**Current Code (NEEDS REFACTORING)**:
```go
// ❌ CURRENT: Uses unstructured data
payload := map[string]interface{}{
    "workflow_id":       workflow.WorkflowID,
    "workflow_name":     workflow.WorkflowName,
    "version":           workflow.Version,
    "status":            workflow.Status,
    "is_latest_version": workflow.IsLatestVersion,
    "execution_engine":  workflow.ExecutionEngine,
    "name":              workflow.Name,
    "description":       workflow.Description,
    "labels":            workflow.Labels,
}
eventData := pkgaudit.NewEventData("datastorage", "workflow_created", "success", payload)
eventDataMap, err := pkgaudit.EnvelopeToMap(eventData)
pkgaudit.SetEventData(auditEvent, eventDataMap)
```

**Required Schemas**:
```go
// WorkflowCatalogCreatedPayload
type WorkflowCatalogCreatedPayload struct {
    WorkflowID       string                 `json:"workflow_id"`
    WorkflowName     string                 `json:"workflow_name"`
    Version          string                 `json:"version"`
    Status           string                 `json:"status"`
    IsLatestVersion  bool                   `json:"is_latest_version"`
    ExecutionEngine  string                 `json:"execution_engine"`
    Name             string                 `json:"name"`
    Description      string                 `json:"description"`
    Labels           map[string]interface{} `json:"labels"`
}

// WorkflowCatalogUpdatedPayload
type WorkflowCatalogUpdatedPayload struct {
    WorkflowID     string                 `json:"workflow_id"`
    UpdatedFields  map[string]interface{} `json:"updated_fields"`
}
```

---

## 📋 **Action Items - Priority Order**

### **PRIORITY 1: Eliminate Unstructured Data (2 schemas)**

**Impact**: ⚠️ **HIGHEST** - Violates project coding standards

1. ✅ Create `WorkflowCatalogCreatedPayload` schema in OpenAPI
2. ✅ Create `WorkflowCatalogUpdatedPayload` schema in OpenAPI
3. ✅ Add to discriminator mapping
4. ✅ Regenerate Go client
5. ✅ Refactor `workflow_catalog_event.go` to use structured types
6. ✅ Remove `EnvelopeToMap()` usage
7. ✅ Update to direct `audit.SetEventData(event, payload)`

### **PRIORITY 2: Add AIAnalysis Internal Events (5 schemas)**

**Impact**: 🔶 **HIGH** - Missing type safety for internal audit trail

1. ✅ Create 5 OpenAPI schemas for AIAnalysis internal events
2. ✅ Add to discriminator mapping (5 new event types)
3. ✅ Regenerate Go client
4. ✅ Verify `audit.SetEventData()` uses OpenAPI-generated types

### **PRIORITY 3: Add Notification Events (4 schemas)**

**Impact**: 🔶 **HIGH** - Using `StructToMap()` workaround

1. ✅ Create 4 OpenAPI schemas for Notification events
2. ✅ Add to discriminator mapping (4 new event types)
3. ✅ Regenerate Go client
4. ✅ Refactor `pkg/notification/audit/manager.go` to remove `StructToMap()`
5. ✅ Update to direct `audit.SetEventData(event, payload)`

---

## 🎯 **Expected Outcome**

### **Before (Current State)**:
```go
// ❌ Unstructured data
payload := map[string]interface{}{
    "field1": value1,
    "field2": value2,
}
audit.SetEventData(event, payload)

// ❌ StructToMap() workaround
payload := MessageSentEventData{...}
eventDataMap, err := audit.StructToMap(payload)
audit.SetEventData(event, eventDataMap)
```

### **After (Target State)**:
```go
// ✅ Direct type-safe assignment (all services)
payload := dsgen.MessageSentAuditPayload{
    NotificationID: notification.Name,
    Channel:        notification.Spec.Channel,
    // ... all fields type-safe
}
audit.SetEventData(event, payload)  // Direct assignment, no conversion

// ✅ Type-safe validation (e.g., in DataStorage)
eventData, ok := event.EventData.(*dsgen.MessageSentAuditPayload)
if !ok {
    return errors.New("invalid payload type")
}
```

---

## 📊 **Summary Statistics**

| Category | Count | Status |
|---|---|---|
| **Total Event Types** | 23 | - |
| **Already in OpenAPI** | 10 | ✅ Complete |
| **Missing Discriminator Entry** | 1 | ⚠️ **CRITICAL** (schema exists, mapping missing) |
| **Missing Schemas** | 11 | ❌ Need to add |
| **Using Unstructured Data** | 2 | ⚠️ **CRITICAL** |
| **OpenAPI Coverage** | 43% | 🎯 Target: 100% |

---

## 🔗 **Related Documents**

- [AUDIT_PAYLOAD_STRUCTURING_JAN08.md](./AUDIT_PAYLOAD_STRUCTURING_JAN08.md) - Initial 18 violations fixed
- [OPENAPI_HYBRID_APPROACH_JAN08.md](./OPENAPI_HYBRID_APPROACH_JAN08.md) - OpenAPI + interface{} hybrid approach
- [DD-AUDIT-004](../architecture/DESIGN_DECISIONS.md#dd-audit-004) - Structured Types for Audit Event Payloads
- [00-project-guidelines.mdc](.cursor/rules/00-project-guidelines.mdc) - Avoid `map[string]interface{}` mandate

---

## 🚀 **Next Steps**

1. **User Decision**: Prioritize which schemas to add first
2. **Implementation**: Add schemas to `api/openapi/data-storage-v1.yaml`
3. **Validation**: Regenerate client and verify compilation
4. **Refactoring**: Update business logic to use OpenAPI-generated types
5. **Testing**: Validate no regressions in audit event emission

**Estimated Effort**: 2-3 hours for all 11 schemas + refactoring

