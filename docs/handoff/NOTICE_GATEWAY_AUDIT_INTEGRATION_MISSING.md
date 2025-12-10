# NOTICE: Gateway Service Missing Audit Integration

**Date**: 2025-12-10
**Version**: 1.0
**From**: Development Team (Comprehensive Service Triage)
**To**: Gateway Service Team
**Status**: 🟡 **GAP IDENTIFIED** - Audit Integration Missing
**Priority**: MEDIUM (P1)

---

## 📋 Summary

**Issue**: The Gateway Service is the **only** production service without audit integration. All other services (SignalProcessing, AIAnalysis, Notification, WorkflowExecution, RemediationOrchestrator, DataStorage) have audit capabilities per DD-AUDIT-003 and ADR-032.

**Impact**: Gateway signal ingestion events are NOT recorded in the unified audit trail, creating a blind spot in the remediation lifecycle audit.

---

## 🔍 Comprehensive Service Triage Results

| Service | Has Audit Field | Initialized in main.go | Nil-Check Pattern | Design Pattern | Status |
|---------|----------------|------------------------|-------------------|----------------|--------|
| **SignalProcessing** | ✅ `AuditClient` | ✅ MANDATORY | ❌ Removed (ADR-032) | MANDATORY audit | ✅ OK |
| **AIAnalysis** | ✅ `AuditClient` | ✅ Optional | ✅ Graceful | Graceful degradation | ✅ OK |
| **Notification** | ✅ `AuditStore` | ✅ Yes | ✅ Graceful | Graceful degradation | ✅ OK |
| **WorkflowExecution** | ✅ `AuditStore` | ✅ Yes | ✅ Graceful | Graceful degradation | ✅ OK |
| **RemediationOrchestrator** | ✅ `auditStore` | ✅ Yes | ✅ Graceful | Graceful degradation | ✅ OK |
| **DataStorage** | ✅ `auditStore` | ✅ Yes | ✅ Graceful | Graceful degradation | ✅ OK |
| **Gateway** | ❌ **NONE** | ❌ N/A | ❌ N/A | **NO INTEGRATION** | 🟡 **GAP** |

---

## 🚨 Problem Description

### What's Missing

The Gateway Service handles the **first touchpoint** in the remediation lifecycle:
1. **Signal Ingestion** - Receives alerts from Prometheus AlertManager
2. **Signal Deduplication** - Prevents duplicate CRD creation
3. **Storm Aggregation** - Groups related alerts
4. **CRD Creation** - Creates RemediationRequest CRDs

**None of these critical events are audited.**

### Business Requirements Affected

| BR | Description | Impact |
|----|-------------|--------|
| **BR-STORAGE-001** | Complete audit trail with no data loss | ❌ Gateway events missing from audit |
| **BR-GATEWAY-008** | Deduplication logging | ⚠️ Logged but not audited |
| **BR-GATEWAY-016** | Storm aggregation events | ⚠️ Logged but not audited |

### Design Decisions to Follow

| DD | Description | Reference |
|----|-------------|-----------|
| **DD-AUDIT-003** | P0 Audit Traces | All controllers must emit audit events |
| **DD-AUDIT-002** | Buffered Audit Store | Use `pkg/audit/` shared library |
| **ADR-032** | Audit is mandatory for critical paths | SignalProcessing uses MANDATORY pattern |
| **ADR-038** | Async buffered audit ingestion | Fire-and-forget via Data Storage |

---

## 📊 Current Gateway Code (No Audit)

### `cmd/gateway/main.go`

```go
// Current: No audit import or initialization
import (
    // ... standard imports
    // ❌ NO: "github.com/jordigilh/kubernaut/pkg/audit"
)

func main() {
    // ... setup code

    // ❌ NO: auditStore initialization
    // ❌ NO: auditClient creation

    // Start server
    server := gateway.NewServer(cfg, k8sClient, logger)
    // ...
}
```

### `pkg/gateway/processing/server.go`

```go
type GatewayServer struct {
    // ... existing fields
    // ❌ NO: AuditStore audit.AuditStore
}
```

---

## ✅ Recommended Implementation

### Pattern to Follow (Graceful Degradation)

Based on other services, Gateway should use the **Graceful Degradation** pattern:

```go
// cmd/gateway/main.go

import (
    "github.com/jordigilh/kubernaut/pkg/audit"
)

func main() {
    // ... existing setup ...

    // ========================================
    // DD-AUDIT-003: Initialize AuditStore
    // Per DD-AUDIT-002: Use pkg/audit/ shared library
    // Per ADR-038: Async buffered audit ingestion
    // ========================================
    setupLog.Info("Initializing audit store (DD-AUDIT-003)")

    httpClient := &http.Client{Timeout: 10 * time.Second}
    dsClient := audit.NewHTTPDataStorageClient(cfg.DataStorageURL, httpClient)

    auditConfig := audit.RecommendedConfig("gateway")
    auditStore, err := audit.NewBufferedStore(
        dsClient,
        auditConfig,
        "gateway",
        ctrl.Log.WithName("audit"),
    )
    if err != nil {
        // Graceful degradation: Don't crash, operate without audit
        setupLog.Error(err, "Failed to initialize audit store - Gateway will operate without audit")
        auditStore = nil
    }

    // Pass to server
    server := gateway.NewServer(cfg, k8sClient, logger, auditStore)
}
```

### Audit Events to Emit

| Event | When | Severity | Data |
|-------|------|----------|------|
| `signal_received` | On webhook receipt | INFO | alertname, namespace, fingerprint |
| `signal_deduplicated` | When duplicate detected | INFO | fingerprint, existing_crd |
| `signal_storm_detected` | When storm threshold hit | WARN | fingerprint_count, storm_id |
| `crd_created` | On successful CRD creation | INFO | crd_name, fingerprint, priority |
| `crd_creation_failed` | On CRD creation failure | ERROR | error, fingerprint |

---

## 📋 Implementation Checklist

### Phase 1: Infrastructure (Day 1)

- [ ] **Add audit imports** to `cmd/gateway/main.go`
- [ ] **Initialize AuditStore** with graceful degradation pattern
- [ ] **Add `AuditStore` field** to `GatewayServer` struct
- [ ] **Add `DataStorageURL` config** to gateway config.yaml

### Phase 2: Audit Event Emission (Day 2)

> ⚠️ **DD-GATEWAY-012 Update (2025-12-10)**: Redis removed, file paths changed

- [ ] **Signal received audit** in `pkg/gateway/server.go:ProcessSignal()`
- [ ] **Deduplication audit** in `pkg/gateway/server.go:ProcessSignal()` (duplicate branch)
- [ ] **Storm detection audit** in `pkg/gateway/server.go:ProcessSignal()` (storm threshold check)
- [ ] **CRD creation audit** in `pkg/gateway/processing/crd_creator.go`

**Note**: `deduplication.go` and `storm_aggregator.go` were **DELETED** in DD-GATEWAY-012.
All deduplication and storm logic is now in `server.go` using K8s status-based tracking.

### Phase 3: Testing (Day 3)

- [ ] **Unit tests** for audit event creation
- [ ] **Integration tests** for audit emission to Data Storage
- [ ] **Graceful degradation test** when Data Storage unavailable

### Phase 4: Documentation

- [ ] **Update BUSINESS_REQUIREMENTS.md** with new BRs for audit
- [ ] **Create DD-GATEWAY-XXX** for audit integration design

---

## 🎯 Success Criteria

1. ✅ Gateway emits audit events for all signal lifecycle stages
2. ✅ Audit events visible in unified audit table (Data Storage)
3. ✅ Graceful degradation when Data Storage unavailable
4. ✅ No performance impact on signal ingestion latency
5. ✅ Complete audit trail from signal receipt to CRD creation

---

## 💬 Questions for Gateway Team

1. **Config**: Does `config.yaml` already have `data_storage_url` or needs to be added?
2. **Latency**: Is there a hard latency SLA for signal processing that audit must not violate?
3. **Priority**: Should audit be MANDATORY (like SignalProcessing) or graceful degradation?
4. **Timeline**: When can this be prioritized for V1.0 release?

---

## 🔗 Related Documentation

| Document | Purpose |
|----------|---------|
| [DD-AUDIT-003](../architecture/DESIGN_DECISIONS.md) | P0 Audit Traces design |
| [DD-AUDIT-002](../architecture/DESIGN_DECISIONS.md) | Buffered Audit Store |
| [ADR-032](../architecture/decisions/) | Mandatory audit policy |
| [pkg/audit/](../../pkg/audit/) | Shared audit library |
| [SignalProcessing main.go](../../cmd/signalprocessing/main.go) | Reference: MANDATORY pattern |
| [WorkflowExecution main.go](../../cmd/workflowexecution/main.go) | Reference: Graceful degradation pattern |

---

## 📊 Confidence Assessment

**Confidence**: 85%

**Justification**:
- Pattern is well-established across 6 other services
- `pkg/audit/` shared library provides all needed infrastructure
- Graceful degradation pattern prevents Gateway crashes
- Risk: May need config.yaml changes for DataStorageURL

**Remaining Risk**: 15%
- Config changes may need coordination with deployment team
- Performance impact unknown until measured

---

**Document Status**: ✅ **GATEWAY TEAM IMPLEMENTING**
**Created**: 2025-12-10
**Gateway Response**: 2025-12-10

---

## 📬 Gateway Team Response (2025-12-10)

### Acknowledgment

Gateway team **ACKNOWLEDGES** the audit integration gap and is implementing it now.

### Answers to Questions

| # | Question | Answer |
|---|----------|--------|
| **1** | `data_storage_url` in config? | ❌ No - will add |
| **2** | Latency SLA? | P95 <50ms - will use async pattern (DD-GATEWAY-013) |
| **3** | MANDATORY or graceful? | **Graceful degradation** |
| **4** | Timeline? | **Implementing now** (TDD GREEN phase) |

### Implementation Approach

Per DD-GATEWAY-013 (Async Status Update Pattern), audit events will use **fire-and-forget async pattern**:
- Audit emission does NOT block HTTP response
- Uses Data Storage batch API (per NOTICE_DATASTORAGE_BATCH_AUDIT_ENDPOINT_COMPLETE.md)
- Graceful degradation if Data Storage unavailable

### DD-GATEWAY-012 Impact

Notice updated to reflect file path changes:
- ~~`deduplication.go`~~ → `server.go` (deleted in DD-GATEWAY-012)
- ~~`storm_aggregator.go`~~ → `server.go` (deleted in DD-GATEWAY-012)

### Status

| Task | Status |
|------|--------|
| Notice updated | ✅ Done |
| Config changes | ✅ Done |
| Audit implementation | ✅ Done |
| TDD GREEN | ⏳ Pending (waiting for real DS integration test) |

### Implementation Details (Completed 2025-12-10)

**Files Modified**:
- `pkg/gateway/config/config.go`: Added `DataStorageURL` to `InfrastructureSettings`, removed Redis
- `pkg/gateway/server.go`: Added `auditStore` field, initialized in `createServerWithClients`, emits 3 audit event types
- `deploy/gateway/base/02-configmap.yaml`: Added `infrastructure.data_storage_url`
- `test/unit/gateway/config/config_test.go`: Updated for Redis-free config

**Audit Events Implemented**:
1. `gateway.signal.received` (BR-GATEWAY-190) - emitted when new RR created
2. `gateway.signal.deduplicated` (BR-GATEWAY-191) - emitted on duplicate detection
3. `gateway.storm.detected` (BR-GATEWAY-192) - emitted when storm threshold reached

**Pattern**: Async buffered (DD-AUDIT-002, DD-GATEWAY-013) - fire-and-forget, non-blocking


