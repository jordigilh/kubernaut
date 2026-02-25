# RO Test Metrics Parameter Fix - Scope Analysis

**Date**: December 20, 2025
**Service**: RemediationOrchestrator
**Task**: Update all test calls to pass `nil` for metrics parameter
**Status**: 🔄 **IN PROGRESS**

---

## 🎯 **Task Overview**

After refactoring RO metrics to use dependency injection (DD-METRICS-001), multiple test files now have compilation errors because they call functions that were updated to accept a `*metrics.Metrics` parameter.

---

## 📊 **Functions That Now Require Metrics Parameter**

### **1. RemediationRequest Condition Helpers** (`pkg/remediationrequest/conditions.go`)

All condition setters now require `*metrics.Metrics` as the last parameter:

- `SetCondition(rr, conditionType, status, reason, message, m *metrics.Metrics)`
- `SetSignalProcessingReady(rr, ready bool, reason, message string, m *metrics.Metrics)`
- `SetSignalProcessingComplete(rr, complete bool, reason, message string, m *metrics.Metrics)`
- `SetAIAnalysisReady(rr, ready bool, reason, message string, m *metrics.Metrics)`
- `SetAIAnalysisComplete(rr, complete bool, reason, message string, m *metrics.Metrics)`
- `SetWorkflowExecutionReady(rr, ready bool, reason, message string, m *metrics.Metrics)`
- `SetWorkflowExecutionComplete(rr, complete bool, reason, message string, m *metrics.Metrics)`
- `SetRecoveryComplete(rr, complete bool, reason, message string, m *metrics.Metrics)` [Deprecated - Issue #180]

###  **2. RemediationApprovalRequest Condition Helpers** (`pkg/remediationapprovalrequest/conditions.go`)

- `SetApprovalPending(rar, message string, m *metrics.Metrics)`
- `SetApprovalDecided(rar, decision, operator, comments string, m *metrics.Metrics)`
- `SetApprovalExpired(rar, message string, m *metrics.Metrics)`

### **3. Creator Constructors** (`pkg/remediationorchestrator/creator/*.go`)

- `NewAIAnalysisCreator(client, scheme, auditStore, llmClient, metrics, ...) *AIAnalysisCreator`
- `NewSignalProcessingCreator(client, scheme, auditStore, metrics, ...) *SignalProcessingCreator`
- `NewWorkflowExecutionCreator(client, scheme, auditStore, metrics, ...) *WorkflowExecutionCreator`
- `NewApprovalCreator(client, scheme, auditStore, metrics, ...) *ApprovalCreator`

### **4. Helper Functions** (`pkg/remediationorchestrator/helpers/retry.go`)

- `UpdateRemediationRequestStatus(ctx, client, rr, metrics, logger) error`

---

## 📋 **Test Files Requiring Updates**

| File | Function Calls | Estimated Updates |
|---|---|---|
| `remediationrequest/conditions_test.go` | `Set*` condition helpers | 24 |
| `remediationapprovalrequest/conditions_test.go` | `Set*` condition helpers | 12 |
| `notification_creator_test.go` | `creator.New*` | 27 |
| `workflowexecution_creator_test.go` | `creator.New*` | 11 |
| `aianalysis_creator_test.go` | `creator.New*` | 10 |
| `aianalysis_handler_test.go` | `helpers.UpdateRemediationRequestStatus` | 10 |
| `helpers/retry_test.go` | `helpers.UpdateRemediationRequestStatus` | 7 |
| `signalprocessing_creator_test.go` | `creator.New*` | 6 |
| `creator_edge_cases_test.go` | `creator.New*` | 2 |
| `approval_orchestration_test.go` | `creator.New*` | 1 |
| **TOTAL** | | **~110 call sites** |

**Note**: Original estimate was 47 calls, but actual scope is larger due to creator constructors and helper functions.

---

## 🔧 **Fix Strategy**

### **Phase 1: Condition Test Files** (24 + 12 = 36 calls)
- `test/unit/remediationorchestrator/remediationrequest/conditions_test.go`
- `test/unit/remediationorchestrator/remediationapprovalrequest/conditions_test.go`

**Fix**: Add `, nil` as the last parameter to all `Set*` calls.

### **Phase 2: Creator Test Files** (27 + 11 + 10 + 6 + 2 + 1 = 57 calls)
- `notification_creator_test.go`
- `workflowexecution_creator_test.go`
- `aianalysis_creator_test.go`
- `signalprocessing_creator_test.go`
- `creator_edge_cases_test.go`
- `approval_orchestration_test.go`

**Fix**: Add `, nil` as a parameter to all `creator.New*` calls.

### **Phase 3: Helper Test Files** (10 + 7 = 17 calls)
- `aianalysis_handler_test.go`
- `helpers/retry_test.go`

**Fix**: Add `, nil` as a parameter to all `helpers.UpdateRemediationRequestStatus` calls.

---

## ✅ **Success Criteria**

- ✅ All test files compile without errors
- ✅ Zero lint errors
- ✅ All unit tests pass
- ✅ `nil` is used for metrics parameter (tests don't need real metrics instances)

---

**Status**: 🔄 IN PROGRESS - Starting with Phase 1





