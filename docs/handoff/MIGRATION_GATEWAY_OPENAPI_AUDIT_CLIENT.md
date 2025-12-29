# Gateway Service: OpenAPI Audit Client Migration

**Date**: 2025-12-13
**To**: Gateway Team
**From**: Platform/Data Storage Team
**Priority**: 🔴 **HIGHEST**
**Effort**: 5-10 minutes
**Status**: ⚠️ **ACTION REQUIRED**

---

## 🎯 Summary

**Action Required**: Migrate Gateway service from deprecated `HTTPDataStorageClient` to OpenAPI-generated `OpenAPIAuditClient`.

**Why**:
- ✅ Type safety from OpenAPI specification
- ✅ Compile-time contract validation
- ✅ Breaking API changes caught during development
- ✅ Consistency with other services (HAPI uses Python OpenAPI client)

**Impact**: Gateway is the **first service in event flow** - all signals pass through it, making this the highest priority migration.

---

## 📋 Current State

**File**: `pkg/gateway/server.go`
**Line**: 314

**Current Code** (Deprecated):
```go
dsClient := audit.NewHTTPDataStorageClient(cfg.Infrastructure.DataStorageURL, httpClient)
```

**Problem**:
- ❌ No type safety (errors only at runtime)
- ❌ No compile-time contract validation
- ❌ Manual HTTP client (no OpenAPI spec)
- ❌ Breaking changes not caught during development

---

## ✅ Required Changes

### Step 1: Update Imports

**Add this import**:
```go
import dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
```

### Step 2: Replace Client Creation

**OLD** (Line 314 - REMOVE):
```go
dsClient := audit.NewHTTPDataStorageClient(cfg.Infrastructure.DataStorageURL, httpClient)
```

**NEW** (REPLACE WITH):
```go
dsClient, err := dsaudit.NewOpenAPIAuditClient(
    cfg.Infrastructure.DataStorageURL,
    5*time.Second, // Or use cfg.Infrastructure.DataStorageTimeout if available
)
if err != nil {
    return fmt.Errorf("failed to create audit client: %w", err)
}
```

**That's it!** The rest of the code remains unchanged.

---

## 🧪 Testing Instructions

### 1. Unit Tests
```bash
make test-unit-gateway
```

**Expected**: All tests pass

### 2. Integration Tests
```bash
make test-integration-gateway
```

**Expected**: All tests pass

### 3. Smoke Test (Optional)
```bash
# Start Gateway locally
make run-gateway

# Verify audit traces are being written
kubectl logs -n kubernaut-system deployment/gateway | grep audit

# Check Data Storage receives events
kubectl logs -n kubernaut-system deployment/data-storage | grep "audit event received"
```

---

## ✅ Acceptance Criteria

- [ ] Import added: `dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"`
- [ ] Client creation replaced with `dsaudit.NewOpenAPIAuditClient`
- [ ] Error handling added for client creation
- [ ] Unit tests pass: `make test-unit-gateway`
- [ ] Integration tests pass (if applicable): `make test-integration-gateway`
- [ ] No compilation errors: `go build ./cmd/gateway`
- [ ] Audit traces still being written to Data Storage

---

## 📊 Benefits

### Before (HTTPDataStorageClient)
- ❌ No type safety
- ❌ No contract validation
- ❌ Runtime errors only
- ❌ Manual JSON marshaling

### After (OpenAPIAuditClient)
- ✅ Type-safe from OpenAPI spec
- ✅ Compile-time validation
- ✅ Breaking changes caught early
- ✅ Automatic JSON marshaling
- ✅ Consistent with HAPI Python OpenAPI approach

---

## 🚨 Why This Matters for Gateway

**Gateway is the first service in the Kubernaut event flow**:
1. Gateway receives ALL signals from external sources
2. Gateway enriches with initial context
3. Gateway writes first audit traces
4. All downstream services depend on Gateway's audit data

**High audit volume + First in chain = Highest priority for type safety**

---

## 📚 References

**OpenAPI Spec**: `api/openapi/data-storage-v1.yaml`
**OpenAPI Client**: `pkg/datastorage/audit/openapi_adapter.go`
**Migration Guide**: `docs/handoff/COMPLETE_AUDIT_OPENAPI_TRIAGE_2025-12-13.md`

**Example Migration**: RemediationOrchestrator (completed - see audit client setup)

---

## 🤝 Support

**Questions?** Ask in `#data-storage` or `#platform` Slack channels

**Issues?** The OpenAPI client is a drop-in replacement. If you encounter issues:
1. Verify imports are correct
2. Check error handling is present
3. Run tests to identify specific failures
4. Contact Platform team if blocked

---

## ⏱️ Timeline

**Effort**: 5-10 minutes
**Deadline**: Next sprint
**Urgency**: High (first service in event flow)

---

**Status**: ⚠️ **WAITING FOR GATEWAY TEAM**

---

**Created**: 2025-12-13
**Owner**: Gateway Team
**Reviewer**: Platform/Data Storage Team


