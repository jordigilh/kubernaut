# Ogen Migration Progress - Real-Time Tracking

**Date**: January 8, 2026
**Status**: 🔄 **IN PROGRESS** - Phase 2 Complete, Moving to Phase 3
**Current Phase**: Phase 3 - Service Audit Managers (8 files)

---

## ✅ **Completed Phases**

### Phase 1: Setup & Build ✅ (COMPLETE)
- [x] Generate ogen client (`pkg/datastorage/ogen-client/`)
- [x] Update Makefile to use ogen for Go client
- [x] Add ogen dependencies to `go.mod`
- [x] Vendor ogen dependencies (`go mod vendor`)
- [x] Fix package name conflict (all files now use `package api`)

**Files Modified**:
- `pkg/datastorage/ogen-client/gen.go` - Updated package name
- `Makefile` - Changed Go client generation to use ogen
- `go.mod` - Added ogen@v1.18.0
- `vendor/` - Vendored ogen dependencies

**Ogen Generated Output**: 19 files, 1.4MB, perfect tagged unions!

---

### Phase 2: Core Audit Helpers ✅ (COMPLETE)
- [x] Update `pkg/audit/helpers.go` import to ogen-client
- [x] Replace all `dsgen` references with `ogenclient`
- [x] Update `SetEventData` signature to accept `ogenclient.AuditEventRequestEventData`
- [x] Add deprecation notice explaining migration to direct ogen constructors
- [x] Verify compilation (compiles successfully!)

**Files Modified**:
- `pkg/audit/helpers.go` - Import updated, `SetEventData` refactored

**Key Change**:
```go
// Before (oapi-codegen):
func SetEventData(e *dsgen.AuditEventRequest, data interface{}) {
    e.EventData = data  // interface{}
}

// After (ogen):
func SetEventData(e *ogenclient.AuditEventRequest, data ogenclient.AuditEventRequestEventData) {
    e.EventData = data  // Typed union!
}
```

---

## 🔄 **Current Phase: Service Audit Managers** (8 files)

### Service Files Requiring Updates:

#### 1. Gateway Service
**File**: `pkg/gateway/server.go`
**Lines**: 4 audit event creation functions
**Status**: ⏳ Pending

**Changes Needed**:
```go
// Before:
import dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
payload := GatewayAuditPayload{...}
audit.SetEventData(event, payload)

// After:
import ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
payload := ogenclient.GatewayAuditPayload{...}
event.EventData = ogenclient.NewGatewayAuditPayloadAuditEventRequestEventData(payload)
```

---

#### 2. RemediationOrchestrator Service
**File**: `pkg/remediationorchestrator/audit/manager.go`
**Status**: ⏳ Pending

**Changes Needed**:
```go
// Before:
payload := RemediationOrchestratorAuditPayload{...}
audit.SetEventData(event, payload)

// After:
payload := ogenclient.RemediationOrchestratorAuditPayload{...}
event.EventData = ogenclient.NewRemediationOrchestratorAuditPayloadAuditEventRequestEventData(payload)
```

---

#### 3. SignalProcessing Service
**File**: `pkg/signalprocessing/audit/client.go`
**Status**: ⏳ Pending

**Changes Needed**:
```go
// Before:
payload := SignalProcessingAuditPayload{...}
audit.SetEventData(event, payload)

// After:
payload := ogenclient.SignalProcessingAuditPayload{...}
event.EventData = ogenclient.NewSignalProcessingAuditPayloadAuditEventRequestEventData(payload)
```

---

#### 4. AIAnalysis Service
**File**: `pkg/aianalysis/audit/audit.go`
**Status**: ⏳ Pending

**Changes Needed**:
```go
// Before:
payload := dsgen.AIAnalysisAuditPayload{...}
audit.SetEventData(event, payload)

// After:
payload := ogenclient.AIAnalysisAuditPayload{...}
event.EventData = ogenclient.NewAIAnalysisAuditPayloadAuditEventRequestEventData(payload)

// PLUS: 5 other event types (PhaseTransition, HolmesGPTCall, ApprovalDecision, RegoEvaluation, Error)
```

---

#### 5. WorkflowExecution Service
**File**: `pkg/workflowexecution/audit/manager.go`
**Status**: ⏳ Pending

**Changes Needed**:
```go
// Before:
payload := workflowexecution.WorkflowExecutionAuditPayload{...}
audit.SetEventData(event, payload)

// After:
payload := ogenclient.WorkflowExecutionAuditPayload{...}
event.EventData = ogenclient.NewWorkflowExecutionAuditPayloadAuditEventRequestEventData(payload)
```

---

#### 6. Notification Service
**File**: `pkg/notification/audit/manager.go`
**Status**: ⏳ Pending

**Changes Needed**:
```go
// Before:
payload := dsgen.NotificationMessageSentPayload{...}
audit.SetEventData(event, payload)

// After:
payload := ogenclient.NotificationMessageSentPayload{...}
event.EventData = ogenclient.NewNotificationMessageSentPayloadAuditEventRequestEventData(payload)

// PLUS: 3 other event types (Failed, Acknowledged, Escalated)
```

---

#### 7. Webhooks Service (4 handlers)
**Files**:
- `pkg/authwebhook/notificationrequest_handler.go`
- `pkg/authwebhook/notificationrequest_validator.go`
- `pkg/authwebhook/workflowexecution_handler.go`
- `pkg/authwebhook/remediationapprovalrequest_handler.go`

**Status**: ⏳ Pending

**Changes Needed**: Similar pattern for 4 webhook event types

---

#### 8. DataStorage Service (2 files)
**Files**:
- `pkg/datastorage/audit/workflow_catalog_event.go`
- `pkg/datastorage/audit/workflow_search_event.go`

**Status**: ⏳ Pending

**Changes Needed**:
```go
// Before:
payload := dsgen.WorkflowCatalogCreatedPayload{...}
audit.SetEventData(event, payload)

// After:
payload := ogenclient.WorkflowCatalogCreatedPayload{...}
event.EventData = ogenclient.NewWorkflowCatalogCreatedPayloadAuditEventRequestEventData(payload)
```

---

## 📋 **Remaining Phases**

### Phase 4: Integration Tests (~15 files) ⏳
Integration tests currently cast `event.EventData` to `map[string]interface{}`. With ogen:
```go
// Before:
eventData := event.EventData.(map[string]interface{})
Expect(eventData["workflow_name"]).To(Equal("my-workflow"))

// After:
payload, ok := event.EventData.GetWorkflowExecutionAuditPayload()
Expect(ok).To(BeTrue())
Expect(payload.WorkflowName).To(Equal("my-workflow"))
```

---

### Phase 5: Python Code Migration ⏳
**Files**:
- `holmesgpt-api/src/audit/events.py` - 5 functions returning dicts → return Pydantic models
- `holmesgpt-api/src/audit/buffered_store.py` - Remove lines 434-435 (conversion logic)

---

### Phase 6: Testing & Validation ⏳
- [ ] Compile all Go code
- [ ] Run unit tests
- [ ] Run integration tests (Go)
- [ ] Run integration tests (Python)
- [ ] Validate no regressions

---

### Phase 7: Cleanup ⏳
- [ ] Delete `pkg/datastorage/client/` (old oapi-codegen client)
- [ ] Delete duplicate `audit_types.go` files (8 files)
- [ ] Update documentation

---

## 📊 **Progress Summary**

| Phase | Status | Files Affected | Time Estimate |
|-------|--------|----------------|---------------|
| 1. Setup & Build | ✅ COMPLETE | 4 files | ~15 min |
| 2. Core Helpers | ✅ COMPLETE | 1 file | ~10 min |
| **3. Service Managers** | **🔄 IN PROGRESS** | **12 files** | **~2 hours** |
| 4. Integration Tests | ⏳ Pending | ~15 files | ~1 hour |
| 5. Python Migration | ⏳ Pending | 2 files | ~1 hour |
| 6. Testing | ⏳ Pending | - | ~1 hour |
| 7. Cleanup | ⏳ Pending | ~10 files | ~30 min |

**Total Estimate**: 5-6 hours
**Completed**: ~25 minutes
**Remaining**: 5 hours

---

## 🎯 **Next Steps**

1. ✅ Systematically update all 12 service audit manager files
2. Compile and fix any errors
3. Move to integration tests
4. Python migration
5. Full test suite validation

---

**Last Updated**: January 8, 2026 18:15 PST

