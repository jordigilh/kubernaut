# Notification Service: OpenAPI Audit Client Migration

**Date**: 2025-12-13
**To**: Notification Team
**From**: Platform/Data Storage Team
**Priority**: 🟡 **MEDIUM**
**Effort**: 5-10 minutes
**Status**: ⚠️ **ACTION REQUIRED**

---

## 🎯 Summary

**Action Required**: Migrate Notification service from deprecated `HTTPDataStorageClient` to OpenAPI-generated `OpenAPIAuditClient`.

**Why**:
- ✅ Type safety from OpenAPI specification
- ✅ Compile-time contract validation
- ✅ Breaking API changes caught during development
- ✅ Consistency with other services

**Impact**: Notification generates delivery audit traces for notification reliability tracking.

---

## 📋 Current State

**File**: `cmd/notification/main.go`
**Line**: 162 (via `dataStorageClient` variable)

**Current Code** (Deprecated):
```go
dataStorageClient := audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)

// Create buffered audit store (fire-and-forget pattern, ADR-038)
auditConfig := audit.Config{
    BufferSize:    10000,           // In-memory buffer size
    BatchSize:     100,             // Batch size for Data Storage writes
    FlushInterval: 5 * time.Second, // Flush interval
    MaxRetries:    3,               // Max retry attempts for failed writes
}

auditStore, err := audit.NewBufferedStore(dataStorageClient, auditConfig, "notification-controller", auditLogger)
if err != nil {
    setupLog.Error(err, "Failed to create audit store")
    os.Exit(1)
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

**Find the line** (around line 162):
```go
dataStorageClient := audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)
```

**REPLACE WITH**:
```go
dataStorageClient, err := dsaudit.NewOpenAPIAuditClient(dataStorageURL, 5*time.Second)
if err != nil {
    setupLog.Error(err, "Failed to create audit client")
    os.Exit(1)
}
```

**Note**: Keep the rest of the code unchanged (audit store creation, config, etc.)

---

## 🧪 Testing Instructions

### 1. Unit Tests
```bash
make test-unit-notification
```

**Expected**: All tests pass

### 2. Integration Tests
```bash
make test-integration-notification
```

**Expected**: All tests pass

### 3. Controller Smoke Test
```bash
# Start Notification controller
make run-notification

# Create test Notification resource
kubectl apply -f test/e2e/notification/test-notification.yaml

# Verify audit traces
kubectl logs -n kubernaut-system deployment/notification | grep "Audit store initialized"

# Check notification delivery audit
kubectl logs -n kubernaut-system deployment/notification | grep "notification_delivery"
```

---

## ✅ Acceptance Criteria

- [ ] Import added: `dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"`
- [ ] Client creation replaced with `dsaudit.NewOpenAPIAuditClient`
- [ ] Error handling added (crashes on failure to match existing behavior)
- [ ] Unit tests pass: `make test-unit-notification`
- [ ] Integration tests pass: `make test-integration-notification`
- [ ] No compilation errors: `go build ./cmd/notification`
- [ ] Controller starts successfully in test environment
- [ ] Notification delivery audit traces still being written

---

## 🚨 Why This Matters for Notification

**Notification tracks delivery attempts**:
- Audit every notification delivery attempt
- Record delivery success/failure
- Track channel-specific errors (email, Slack, PagerDuty, webhook)
- Support notification reliability monitoring

**Type safety prevents**:
- Lost delivery audit traces
- Incorrect delivery tracking
- Breaking changes at runtime

**Business Context**:
- Notification is the final step in alerting flow
- Delivery audit is critical for SLA tracking
- Audit data supports notification reliability metrics

---

## 📚 References

**OpenAPI Spec**: `api/openapi/data-storage-v1.yaml`
**OpenAPI Client**: `pkg/datastorage/audit/openapi_adapter.go`
**Business Requirements**: `docs/services/crd-controllers/notification/BUSINESS_REQUIREMENTS.md`

**Related ADRs**:
- ADR-038: Async Buffered Audit Ingestion
- DD-AUDIT-002: Use pkg/audit shared library

---

## 🤝 Support

**Questions?** Ask in `#notification` or `#platform` Slack channels

**Issues?** The OpenAPI client is a drop-in replacement. If you encounter issues:
1. Verify imports are correct
2. Check error handling preserves crash-on-failure behavior
3. Run tests to identify specific failures
4. Contact Platform team if blocked

---

## ⏱️ Timeline

**Effort**: 5-10 minutes
**Deadline**: Next sprint
**Urgency**: Medium (delivery audit is important, but not blocking)

---

**Status**: ⚠️ **WAITING FOR NOTIFICATION TEAM**

---

**Created**: 2025-12-13
**Owner**: Notification Team
**Reviewer**: Platform/Data Storage Team


