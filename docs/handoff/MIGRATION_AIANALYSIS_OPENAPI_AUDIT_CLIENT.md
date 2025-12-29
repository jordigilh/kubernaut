# AIAnalysis Service: OpenAPI Audit Client Migration

**Date**: 2025-12-13
**To**: AIAnalysis Team
**From**: Platform/Data Storage Team
**Priority**: 🔴 **HIGH**
**Effort**: 5-10 minutes
**Status**: ⚠️ **ACTION REQUIRED**

---

## 🎯 Summary

**Action Required**: Migrate AIAnalysis service from deprecated `HTTPDataStorageClient` to OpenAPI-generated `OpenAPIAuditClient`.

**Why**:
- ✅ Type safety from OpenAPI specification
- ✅ Compile-time contract validation
- ✅ Breaking API changes caught during development
- ✅ Consistency with HAPI's Python OpenAPI client approach

**Impact**: AIAnalysis generates workflow selection audit traces for BR-AUDIT-005, critical for effectiveness monitoring.

---

## 📋 Current State

**File**: `cmd/aianalysis/main.go`
**Line**: 131

**Current Code** (Deprecated):
```go
setupLog.Info("Creating audit client", "dataStorageURL", dataStorageURL)
httpClient := &http.Client{Timeout: 5 * time.Second}
dsClient := sharedaudit.NewHTTPDataStorageClient(dataStorageURL, httpClient)
auditStore, err := sharedaudit.NewBufferedStore(
    dsClient,
    sharedaudit.DefaultConfig(),
    "aianalysis",
    ctrl.Log.WithName("audit"),
)
if err != nil {
    setupLog.Error(err, "failed to create audit store, audit will be disabled")
    // Continue without audit - graceful degradation per DD-AUDIT-002
}
```

**Problem**:
- ❌ No type safety (errors only at runtime)
- ❌ No compile-time contract validation
- ❌ Manual HTTP client (no OpenAPI spec)
- ❌ Inconsistent with HAPI (which uses Python OpenAPI client)

---

## ✅ Required Changes

### Step 1: Update Imports

**Add this import**:
```go
import dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
```

### Step 2: Replace Client Creation

**OLD** (Lines 129-141 - REPLACE):
```go
setupLog.Info("Creating audit client", "dataStorageURL", dataStorageURL)
httpClient := &http.Client{Timeout: 5 * time.Second}
dsClient := sharedaudit.NewHTTPDataStorageClient(dataStorageURL, httpClient)
auditStore, err := sharedaudit.NewBufferedStore(
    dsClient,
    sharedaudit.DefaultConfig(),
    "aianalysis",
    ctrl.Log.WithName("audit"),
)
if err != nil {
    setupLog.Error(err, "failed to create audit store, audit will be disabled")
    // Continue without audit - graceful degradation per DD-AUDIT-002
}
```

**NEW** (REPLACE WITH):
```go
setupLog.Info("Creating audit client", "dataStorageURL", dataStorageURL)
dsClient, err := dsaudit.NewOpenAPIAuditClient(dataStorageURL, 5*time.Second)
if err != nil {
    setupLog.Error(err, "failed to create audit client, audit will be disabled")
    // Continue without audit - graceful degradation per DD-AUDIT-002
} else {
    auditStore, err = sharedaudit.NewBufferedStore(
        dsClient,
        sharedaudit.DefaultConfig(),
        "aianalysis",
        ctrl.Log.WithName("audit"),
    )
    if err != nil {
        setupLog.Error(err, "failed to create audit store, audit will be disabled")
        // Continue without audit - graceful degradation per DD-AUDIT-002
    }
}
```

**Key Changes**:
1. Client creation now returns error (handle it with graceful degradation)
2. Remove manual `httpClient` creation
3. Keep existing audit store creation (unchanged)
4. Preserve graceful degradation behavior

---

## 🧪 Testing Instructions

### 1. Unit Tests
```bash
make test-unit-aianalysis
```

**Expected**: All tests pass

### 2. Integration Tests
```bash
make test-integration-aianalysis
```

**Expected**: All tests pass

### 3. Controller Smoke Test
```bash
# Start AIAnalysis controller
make run-aianalysis

# Create test AIAnalysis resource
kubectl apply -f test/e2e/aianalysis/test-analysis.yaml

# Verify audit traces
kubectl logs -n kubernaut-system deployment/aianalysis | grep "audit client configured"

# Check workflow selection audit (BR-AUDIT-005)
kubectl logs -n kubernaut-system deployment/aianalysis | grep "workflow_selected"
```

---

## ✅ Acceptance Criteria

- [ ] Import added: `dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"`
- [ ] Client creation replaced with `dsaudit.NewOpenAPIAuditClient`
- [ ] Error handling preserves graceful degradation
- [ ] Unit tests pass: `make test-unit-aianalysis`
- [ ] Integration tests pass: `make test-integration-aianalysis`
- [ ] No compilation errors: `go build ./cmd/aianalysis`
- [ ] Controller starts successfully in test environment
- [ ] Workflow selection audit traces still being written (BR-AUDIT-005)

---

## 🚨 Why This Matters for AIAnalysis

**AIAnalysis is the AI decision engine**:
- Receives signals from Gateway
- Calls HolmesGPT API (HAPI) for workflow recommendations
- Generates workflow selection audit traces (BR-AUDIT-005)
- Feeds RemediationOrchestrator with workflow decisions

**Type safety is critical** because:
- Workflow selection audit is MANDATORY (BR-AUDIT-005)
- Audit data drives effectiveness monitoring
- AI decisions must be traceable for compliance
- Consistency with HAPI's Python OpenAPI client approach

**Business Requirements**:
- BR-AUDIT-005: Workflow Selection Audit Trail (MANDATORY)
- BR-AI-012: Rego-based validation (audit required for compliance)

**Integration Context**:
- AIAnalysis calls HAPI (which uses Python OpenAPI client)
- Both should use OpenAPI clients for consistency
- Type safety ensures audit contract compliance

---

## 📚 References

**OpenAPI Spec**: `api/openapi/data-storage-v1.yaml`
**OpenAPI Client**: `pkg/datastorage/audit/openapi_adapter.go`
**Business Requirements**: `docs/services/crd-controllers/02-aianalysis/BUSINESS_REQUIREMENTS.md`

**Related Services**:
- HAPI (HolmesGPT API): Uses Python OpenAPI client ✅
- Data Storage: OpenAPI spec authoritative source

**Related ADRs**:
- ADR-038: Async Buffered Audit Ingestion
- DD-AUDIT-003: AIAnalysis MUST generate audit traces

---

## 🤝 Support

**Questions?** Ask in `#aianalysis` or `#platform` Slack channels

**Issues?** The OpenAPI client is a drop-in replacement. If you encounter issues:
1. Verify imports are correct
2. Check error handling preserves graceful degradation
3. Run tests to identify specific failures
4. Contact Platform team or HAPI team if blocked

---

## ⏱️ Timeline

**Effort**: 5-10 minutes
**Deadline**: Next sprint
**Urgency**: High (workflow selection audit is critical, consistency with HAPI)

---

**Status**: ⚠️ **WAITING FOR AIANALYSIS TEAM**

---

**Created**: 2025-12-13
**Owner**: AIAnalysis Team
**Reviewer**: Platform/Data Storage Team


