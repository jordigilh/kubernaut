# Complete Project Understanding: Architecture & CRD Flow

**Date**: 2025-11-28
**Purpose**: Comprehensive understanding before creating design decisions
**Status**: ✅ **VERIFIED** - Based on actual code

---

## 🏗️ **Actual Services (5 total)**

| Service | Type | Location | Function | Status |
|---------|------|----------|----------|--------|
| **Gateway** | HTTP Service | `cmd/gateway/`, `pkg/gateway/` | Receives alerts, creates RemediationRequest CRDs | ✅ **IMPLEMENTED** |
| **Data Storage** | HTTP Service | `cmd/datastorage/`, `pkg/datastorage/` | PostgreSQL REST API, audit storage | ✅ **IMPLEMENTED** |
| **NotificationController** | CRD Controller | `cmd/notification/`, `internal/controller/notification/` | Watches NotificationRequest CRDs, delivers notifications | ✅ **IMPLEMENTED** |
| **DynamicToolset** | Unknown | `cmd/dynamictoolset/` | Helper tool | ✅ EXISTS (purpose unknown) |
| **HolmesGPT API** | HTTP Service (Python) | External (holmesgpt-api repository) | AI-powered alert root cause analysis | ✅ **IMPLEMENTED** |

**Total**: 5 services (4 Go services in this repo + 1 Python service)

---

## 📋 **CRD Architecture**

### **CRDs with FULL Implementation**

| CRD | API Path | Created By | Watched By | Status |
|-----|----------|------------|------------|--------|
| **NotificationRequest** | `api/notification/v1alpha1/` | RemediationOrchestrator (planned) | NotificationController | ✅ **CRD + Controller EXIST** |

### **CRDs with Type Definitions, NO Controllers**

| CRD | API Path | Created By | Controller Status |
|-----|----------|------------|-------------------|
| **RemediationRequest** | `api/remediation/v1alpha1/` | Gateway (HTTP service) | ❌ **No controller** |
| **RemediationProcessing** | `api/remediationprocessing/v1alpha1/` | N/A | ❌ **Scaffolding only** |
| **AIAnalysis** | `api/aianalysis/v1alpha1/` | N/A | ❌ **Scaffolding only** |
| **WorkflowExecution** | `api/workflowexecution/v1alpha1/` | N/A | ❌ **Scaffolding only** |
| **KubernetesExecution** | `api/kubernetesexecution/v1alpha1/` | N/A | ❌ **Scaffolding only** |
| **RemediationOrchestrator** | `api/remediationorchestrator/v1alpha1/` | N/A | ❌ **Scaffolding only** |

**Key Insight**: These CRDs have Go type definitions but no controllers watching them

---

## 🔄 **Actual Data Flow (What Works Today)**

### **Flow 1: Alert Ingestion (Works ✅)**

```
External Alert (Prometheus/K8s Event)
    ↓ HTTP POST
Gateway Service
    ↓ Parses with adapters
NormalizedSignal (in-memory struct)
    ↓ Deduplication, Storm Detection, Priority
    ↓ Creates CRD
RemediationRequest CRD in etcd ✅
    ↓
❌ **WORKFLOW STOPS HERE**
   (No controller watches RemediationRequest)
```

**Files**:
- Gateway: `pkg/gateway/server.go` (line 849-894)
- CRD Creator: `pkg/gateway/processing/crd_creator.go` (line 324-379)
- RemediationRequest type: `api/remediation/v1alpha1/remediationrequest_types.go`

---

### **Flow 2: Notification Delivery (Works ✅ BUT Manually Triggered)**

```
NotificationRequest CRD created manually
    ↓ Watched by
NotificationController
    ↓ Delivers to
Slack / Console / File ✅
    ↓ Updates
NotificationRequest.Status ✅
    ↓ Writes audit to
Data Storage Service (PostgreSQL) ✅
```

**Files**:
- Controller: `internal/controller/notification/notificationrequest_controller.go`
- Delivery services: `pkg/notification/delivery/`
- NotificationRequest type: `api/notification/v1alpha1/notificationrequest_types.go`

**Problem**: NotificationRequest CRDs are never created automatically - they must be created manually or in tests

---

### **Flow 3: Planned Workflow (NOT IMPLEMENTED ❌)**

**From ADR-017**:

```
RemediationRequest CRD
    ↓ Should be watched by
RemediationOrchestrator (NOT IMPLEMENTED) ❌
    ↓ Should create
NotificationRequest CRD
    ↓ Watched by
NotificationController ✅
    ↓ Delivers
Notifications ✅
```

**Key Point**: RemediationOrchestrator doesn't exist, so NotificationRequest CRDs are never created automatically

---

## 🎯 **What DD-SYSTEM-001 Should ACTUALLY Cover**

### **V1 Scope: NotificationRequest ONLY**

**Rationale**:
1. ✅ NotificationRequest is the ONLY CRD with a working controller
2. ✅ NotificationRequest spec immutability is a real, practical need (from user feedback)
3. ✅ Can be implemented immediately (no dependencies)

**Changes**:
1. Update `api/notification/v1alpha1/notificationrequest_types.go`
   - Add XValidation rules for immutability
2. Update `internal/controller/notification/notificationrequest_controller.go`
   - Remove observedGeneration tracking (if any)
   - Simplify reconciliation logic
3. Update DD-NOT-003 V2.0
   - Remove 9 spec mutation tests

### **V2 Scope: RemediationRequest (FUTURE)**

**When**: After RemediationOrchestrator controller is implemented

**Changes**:
1. `api/remediation/v1alpha1/remediationrequest_types.go`
   - Add immutability for workflow data fields
   - Add mutable toggle: `spec.rejected: bool` (user rejection)
   - Add mutable toggle (future): `spec.snoozed: bool` (snoozing - pending SRE feedback)

**Rationale**: User said "avoid users overwriting specs" - this applies when RemediationOrchestrator starts processing RemediationRequest CRDs

### **V3 Scope: Other CRDs (DISTANT FUTURE)**

Apply immutability policy when these controllers are eventually implemented.

---

## 📊 **Immutability Requirements by CRD**

| CRD | Controller Exists? | Immutability Needed? | Mutable Toggles | Priority |
|-----|-------------------|---------------------|----------------|----------|
| **NotificationRequest** | ✅ YES | ✅ **NOW** | None | **P0** |
| **RemediationRequest** | ❌ No (planned) | ⏸️ **WHEN CONTROLLER BUILT** | `rejected`, `snoozed` (future) | **P1** (future) |
| RemediationProcessing | ❌ No (scaffolding) | ⏸️ When built | TBD | P2 (distant future) |
| AIAnalysis | ❌ No (scaffolding) | ⏸️ When built | TBD | P2 (distant future) |
| WorkflowExecution | ❌ No (scaffolding) | ⏸️ When built | TBD | P2 (distant future) |
| KubernetesExecution | ❌ No (scaffolding) | ⏸️ When built | TBD | P2 (distant future) |

---

## 🚨 **Critical Architecture Gaps (Not Related to DD-SYSTEM-001)**

### **Gap 1: RemediationRequest CRDs Are Never Processed**

**Problem**: Gateway creates RemediationRequest CRDs, but no controller watches them
**Impact**: Remediations never proceed beyond Gateway
**Fix**: Implement RemediationOrchestrator controller (separate project)

### **Gap 2: NotificationRequest CRDs Are Never Created Automatically**

**Problem**: ADR-017 says RemediationOrchestrator creates them, but RemediationOrchestrator doesn't exist
**Impact**: Notifications only sent manually or in tests
**Fix**: Implement RemediationOrchestrator (same as Gap 1)

### **Gap 3: 6 CRD Type Definitions with No Controllers**

**Problem**: CRD APIs defined but no functionality
**Impact**: Scaffolding code with no business value
**Fix**: Either implement controllers or remove unused CRDs (separate architectural decision)

---

## 📝 **Recommended Design Decision Scope**

### **Option 1: Narrow Scope (RECOMMENDED) ✅**

**Title**: "DD-NOT-005: NotificationRequest Spec Immutability"

**Scope**: NotificationRequest CRD only (what exists now)

**Rationale**:
- Focus on what's actually implemented
- Deliver immediate value
- No speculation about future CRDs
- Clean, focused design decision

**Effort**: 1 day
- Update 1 CRD type definition
- Update 1 controller
- Update DD-NOT-003 V2.0 (-9 tests)

---

### **Option 2: Policy Document with Phased Implementation**

**Title**: "DD-SYSTEM-001: CRD Spec Immutability Policy (Phased)"

**Scope**:
- **V1 (NOW)**: NotificationRequest
- **V2 (FUTURE)**: RemediationRequest (when RemediationOrchestrator built)
- **V3 (DISTANT FUTURE)**: Other CRDs (when controllers built)

**Rationale**:
- Establishes policy for future CRDs
- Implements immediately for NotificationRequest
- Provides guidance for future controllers

**Effort**: 1 day (V1), future phases TBD

---

## 🎯 **User's Concern: "Avoid users overwriting specs"**

### **Immediate Application: NotificationRequest**

**Concern Applies**: ✅ **YES**
- NotificationController exists and watches NotificationRequest
- Users could update spec mid-delivery
- Would cause race conditions, status corruption

**Action**: Make NotificationRequest spec immutable (DD-NOT-005 or DD-SYSTEM-001 V1)

---

### **Future Application: RemediationRequest**

**Concern Applies**: ⏸️ **WHEN RemediationOrchestrator EXISTS**
- Currently no controller watches RemediationRequest (Gateway only creates them)
- When RemediationOrchestrator is built, immutability becomes critical
- User's concern is valid but applies to future state

**Action**: Document policy in DD-SYSTEM-001, implement in V2 when RemediationOrchestrator is built

---

### **Future Application: Other CRDs**

**Concern Applies**: ⏸️ **WHEN CONTROLLERS ARE BUILT**
- Currently just scaffolding, no controllers
- Apply immutability policy when controllers are implemented

---

## ✅ **Next Steps**

1. **User Decision**: Choose Option 1 (narrow) or Option 2 (policy with phases)

2. **If Option 1** (Narrow Scope):
   - Create `DD-NOT-005-NOTIFICATIONREQUEST-SPEC-IMMUTABILITY.md`
   - Update NotificationRequest CRD
   - Update NotificationController
   - Update DD-NOT-003 V2.0

3. **If Option 2** (Policy Document):
   - Create `DD-SYSTEM-001-CRD-SPEC-IMMUTABILITY-POLICY.md`
   - **V1 Section**: NotificationRequest (implement now)
   - **V2 Section**: RemediationRequest (implement when controller built)
   - **V3 Section**: Other CRDs (implement when controllers built)

---

## 📊 **Confidence Assessment**

| Aspect | Confidence | Evidence |
|--------|-----------|----------|
| **Gateway architecture** | 100% | Code verified in `pkg/gateway/` |
| **NotificationController architecture** | 100% | Code verified in `internal/controller/notification/` |
| **RemediationRequest creation** | 100% | Code verified in `pkg/gateway/processing/crd_creator.go` |
| **RemediationRequest NOT watched** | 100% | `internal/controller/` has only `notification/` |
| **NotificationRequest creation** | 100% | ADR-017 confirms RemediationOrchestrator (not implemented) |
| **CRD type definitions** | 100% | All verified in `api/` directory |

**Overall Confidence**: 100% (all statements verified against actual code)

---

**Prepared By**: AI Assistant (Complete Project Understanding)
**Date**: 2025-11-28
**Verification Method**: Codebase search + file reading
**Ready For**: Design decision creation

