# WorkflowExecution Service: OpenAPI Audit Client Migration

**Date**: 2025-12-13
**To**: WorkflowExecution Team
**From**: Platform/Data Storage Team
**Priority**: 🔴 **HIGH**
**Effort**: 5-10 minutes
**Status**: ⚠️ **ACTION REQUIRED**

---

## 🎯 Summary

**Action Required**: Migrate WorkflowExecution service from deprecated `HTTPDataStorageClient` to OpenAPI-generated `OpenAPIAuditClient`.

**Why**:
- ✅ Type safety from OpenAPI specification
- ✅ Compile-time contract validation
- ✅ Breaking API changes caught during development
- ✅ Consistency with other services

**Impact**: WorkflowExecution generates execution audit traces for BR-WE-005 (workflow execution audit trail).

---

## 📋 Current State

**File**: `cmd/workflowexecution/main.go`
**Line**: 161

**Current Code** (Deprecated):
```go
// Create HTTP client for Data Storage Service
httpClient := &http.Client{
    Timeout: 10 * time.Second,
}
dsClient := audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)

// Create buffered audit store using shared library (DD-AUDIT-002)
auditConfig := audit.RecommendedConfig("workflowexecution")
auditStore, err := audit.NewBufferedStore(
    dsClient,
    auditConfig,
    "workflowexecution",
    ctrl.Log.WithName("audit"),
)
if err != nil {
    setupLog.Error(err, "Failed to initialize audit store - controller will operate without audit (graceful degradation)")
    auditStore = nil
}
```

**Problem**:
- ❌ No type safety (errors only at runtime)
- ❌ No compile-time contract validation
- ❌ Manual HTTP client (no OpenAPI spec)

---

## ✅ Required Changes

### Step 1: Update Imports

**Add this import**:
```go
import dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
```

### Step 2: Replace Client Creation

**OLD** (Lines 157-178 - REPLACE):
```go
// Create HTTP client for Data Storage Service
httpClient := &http.Client{
    Timeout: 10 * time.Second,
}
dsClient := audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)

// Create buffered audit store using shared library (DD-AUDIT-002)
auditConfig := audit.RecommendedConfig("workflowexecution")
auditStore, err := audit.NewBufferedStore(
    dsClient,
    auditConfig,
    "workflowexecution",
    ctrl.Log.WithName("audit"),
)
if err != nil {
    setupLog.Error(err, "Failed to initialize audit store - controller will operate without audit (graceful degradation)")
    auditStore = nil
} else {
    setupLog.Info("Audit store initialized successfully",
        "buffer_size", auditConfig.BufferSize,
        "batch_size", auditConfig.BatchSize,
        "flush_interval", auditConfig.FlushInterval,
    )
}
```

**NEW** (REPLACE WITH):
```go
// Create OpenAPI-based audit client for Data Storage Service
dsClient, err := dsaudit.NewOpenAPIAuditClient(dataStorageURL, 10*time.Second)
if err != nil {
    setupLog.Error(err, "Failed to create audit client - controller will operate without audit (graceful degradation)")
    auditStore = nil
} else {
    // Create buffered audit store using shared library (DD-AUDIT-002)
    auditConfig := audit.RecommendedConfig("workflowexecution")
    auditStore, err = audit.NewBufferedStore(
        dsClient,
        auditConfig,
        "workflowexecution",
        ctrl.Log.WithName("audit"),
    )
    if err != nil {
        setupLog.Error(err, "Failed to initialize audit store - controller will operate without audit (graceful degradation)")
        auditStore = nil
    } else {
        setupLog.Info("Audit store initialized successfully",
            "buffer_size", auditConfig.BufferSize,
            "batch_size", auditConfig.BatchSize,
            "flush_interval", auditConfig.FlushInterval,
        )
    }
}
```

**Key Changes**:
1. Client creation now returns error (handle it with graceful degradation)
2. Remove manual `httpClient` creation
3. Keep existing audit store creation (unchanged)
4. Preserve graceful degradation behavior (controller continues without audit)

---

## 🧪 Testing Instructions

### 1. Unit Tests
```bash
make test-unit-workflowexecution
```

**Expected**: All tests pass

### 2. Integration Tests
```bash
make test-integration-workflowexecution
```

**Expected**: All tests pass

### 3. Controller Smoke Test
```bash
# Start WorkflowExecution controller
make run-workflowexecution

# Create test WorkflowExecution resource
kubectl apply -f test/e2e/workflowexecution/test-execution.yaml

# Verify audit traces
kubectl logs -n kubernaut-system deployment/workflowexecution | grep "Audit store initialized successfully"

# Check PipelineRun execution audit
kubectl logs -n kubernaut-system deployment/workflowexecution | grep "BR-WE-005"
```

---

## ✅ Acceptance Criteria

- [ ] Import added: `dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"`
- [ ] Client creation replaced with `dsaudit.NewOpenAPIAuditClient`
- [ ] Error handling preserves graceful degradation (controller continues without audit)
- [ ] Unit tests pass: `make test-unit-workflowexecution`
- [ ] Integration tests pass: `make test-integration-workflowexecution`
- [ ] No compilation errors: `go build ./cmd/workflowexecution`
- [ ] Controller starts successfully in test environment
- [ ] Audit traces still being written for BR-WE-005 (Workflow Execution Audit Trail)
- [ ] Controller gracefully degrades if audit client creation fails

---

## 🚨 Why This Matters for WorkflowExecution

**WorkflowExecution generates critical execution audit traces**:
- BR-WE-005: Workflow Execution Audit Trail (MANDATORY)
- Tracks PipelineRun creation and execution
- Records execution outcomes and failures
- Supports BR-WE-012: Exponential backoff based on execution history

**Type safety prevents**:
- Lost execution audit traces
- Incorrect execution tracking
- Breaking changes at runtime

**Business Context**:
- WorkflowExecution is the final step in remediation flow
- Execution audit is critical for effectiveness monitoring
- Audit data drives BR-WE-012 (exponential backoff on failures)

---

## 📚 References

**OpenAPI Spec**: `api/openapi/data-storage-v1.yaml`
**OpenAPI Client**: `pkg/datastorage/audit/openapi_adapter.go`
**Business Requirements**: `docs/services/crd-controllers/03-workflowexecution/BUSINESS_REQUIREMENTS.md`

**Related Design Decisions**:
- DD-AUDIT-003: WorkflowExecution MUST generate audit traces
- DD-AUDIT-002: Use pkg/audit shared library
- DD-WE-004: Exponential Backoff (depends on audit history)

---

## 🤝 Support

**Questions?** Ask in `#workflowexecution` or `#platform` Slack channels

**Issues?** The OpenAPI client is a drop-in replacement. If you encounter issues:
1. Verify imports are correct
2. Check error handling preserves graceful degradation
3. Run tests to identify specific failures
4. Contact Platform team if blocked

---

## ⏱️ Timeline

**Effort**: 5-10 minutes
**Deadline**: Next sprint
**Urgency**: High (execution audit is critical for effectiveness monitoring)

---

**Status**: ⚠️ **WAITING FOR WORKFLOWEXECUTION TEAM**

---

**Created**: 2025-12-13
**Owner**: WorkflowExecution Team
**Reviewer**: Platform/Data Storage Team


