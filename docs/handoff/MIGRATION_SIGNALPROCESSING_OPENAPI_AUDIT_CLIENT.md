# SignalProcessing Service: OpenAPI Audit Client Migration

**Date**: 2025-12-13
**To**: SignalProcessing Team
**From**: Platform/Data Storage Team
**Priority**: 🔴 **HIGH**
**Effort**: 5-10 minutes
**Status**: ⚠️ **ACTION REQUIRED**

---

## 🎯 Summary

**Action Required**: Migrate SignalProcessing service from deprecated `HTTPDataStorageClient` to OpenAPI-generated `OpenAPIAuditClient`.

**Why**:
- ✅ Type safety from OpenAPI specification
- ✅ Compile-time contract validation
- ✅ Breaking API changes caught during development
- ✅ Consistency with other services

**Impact**: SignalProcessing treats audit as **MANDATORY** (crashes on failure per ADR-032), making type safety critical.

---

## 📋 Current State

**File**: `cmd/signalprocessing/main.go`
**Line**: 151

**Current Code** (Deprecated):
```go
httpClient := &http.Client{Timeout: 5 * time.Second}
dsClient := sharedaudit.NewHTTPDataStorageClient(dataStorageURL, httpClient)

auditStore, err := sharedaudit.NewBufferedStore(
    dsClient,
    sharedaudit.DefaultConfig(),
    "signalprocessing",
    ctrl.Log.WithName("audit"),
)
if err != nil {
    setupLog.Error(err, "FATAL: failed to create audit store - audit is MANDATORY per ADR-032")
    os.Exit(1)
}
```

**Problem**:
- ❌ No type safety (errors only at runtime)
- ❌ No compile-time contract validation
- ❌ Service crashes on audit failure (ADR-032) - type safety critical!

---

## ✅ Required Changes

### Step 1: Update Imports

**Add this import**:
```go
import dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
```

### Step 2: Replace Client Creation

**OLD** (Lines 150-162 - REPLACE):
```go
httpClient := &http.Client{Timeout: 5 * time.Second}
dsClient := sharedaudit.NewHTTPDataStorageClient(dataStorageURL, httpClient)

auditStore, err := sharedaudit.NewBufferedStore(
    dsClient,
    sharedaudit.DefaultConfig(),
    "signalprocessing",
    ctrl.Log.WithName("audit"),
)
if err != nil {
    setupLog.Error(err, "FATAL: failed to create audit store - audit is MANDATORY per ADR-032")
    os.Exit(1)
}
```

**NEW** (REPLACE WITH):
```go
dsClient, err := dsaudit.NewOpenAPIAuditClient(dataStorageURL, 5*time.Second)
if err != nil {
    setupLog.Error(err, "FATAL: failed to create audit client - audit is MANDATORY per ADR-032")
    os.Exit(1)
}

auditStore, err := sharedaudit.NewBufferedStore(
    dsClient,
    sharedaudit.DefaultConfig(),
    "signalprocessing",
    ctrl.Log.WithName("audit"),
)
if err != nil {
    setupLog.Error(err, "FATAL: failed to create audit store - audit is MANDATORY per ADR-032")
    os.Exit(1)
}
```

**Key Changes**:
1. Client creation now returns error (handle it!)
2. Remove manual `httpClient` creation
3. Keep existing `auditStore` creation (unchanged)

---

## 🧪 Testing Instructions

### 1. Unit Tests
```bash
make test-unit-signalprocessing
```

**Expected**: All tests pass

### 2. Integration Tests
```bash
make test-integration-signalprocessing
```

**Expected**: All tests pass

### 3. Controller Smoke Test
```bash
# Start SignalProcessing controller
make run-signalprocessing

# Create test SignalProcessing resource
kubectl apply -f test/e2e/signalprocessing/test-signal.yaml

# Verify audit traces
kubectl logs -n kubernaut-system deployment/signalprocessing | grep "audit client configured successfully"
```

---

## ✅ Acceptance Criteria

- [ ] Import added: `dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"`
- [ ] Client creation replaced with `dsaudit.NewOpenAPIAuditClient`
- [ ] Error handling added for client creation (crashes on failure per ADR-032)
- [ ] Unit tests pass: `make test-unit-signalprocessing`
- [ ] Integration tests pass: `make test-integration-signalprocessing`
- [ ] No compilation errors: `go build ./cmd/signalprocessing`
- [ ] Controller starts successfully in test environment
- [ ] Audit traces still being written for BR-SP-090 (Categorization Audit Trail)

---

## 🚨 Why This Matters for SignalProcessing

**SignalProcessing treats audit as MANDATORY** (ADR-032):
```go
if err != nil {
    setupLog.Error(err, "FATAL: failed to create audit store - audit is MANDATORY per ADR-032")
    os.Exit(1) // Controller CRASHES on audit failure
}
```

**Type safety is critical** because:
- Service crashes immediately if audit fails
- No graceful degradation
- Type errors at runtime = service unavailable
- OpenAPI client catches errors at compile time

**Business Requirements**:
- BR-SP-090: Categorization Audit Trail (MANDATORY)
- ADR-032: Audit is non-optional for SignalProcessing

---

## 📚 References

**OpenAPI Spec**: `api/openapi/data-storage-v1.yaml`
**OpenAPI Client**: `pkg/datastorage/audit/openapi_adapter.go`
**Business Requirements**: `docs/services/crd-controllers/signalprocessing/BUSINESS_REQUIREMENTS.md`

**Related ADRs**:
- ADR-032: Audit is MANDATORY for SignalProcessing
- ADR-038: Async Buffered Audit Ingestion

---

## 🤝 Support

**Questions?** Ask in `#signalprocessing` or `#platform` Slack channels

**Issues?** The OpenAPI client is a drop-in replacement. If you encounter issues:
1. Verify imports are correct
2. Check error handling preserves "crash on failure" behavior
3. Run tests to identify specific failures
4. Contact Platform team if blocked

---

## ⏱️ Timeline

**Effort**: 5-10 minutes
**Deadline**: Next sprint
**Urgency**: High (audit is MANDATORY, type safety critical)

---

**Status**: ⚠️ **WAITING FOR SIGNALPROCESSING TEAM**

---

**Created**: 2025-12-13
**Owner**: SignalProcessing Team
**Reviewer**: Platform/Data Storage Team


