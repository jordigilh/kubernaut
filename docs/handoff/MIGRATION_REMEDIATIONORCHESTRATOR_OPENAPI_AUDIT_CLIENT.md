# RemediationOrchestrator Service: OpenAPI Audit Client Migration

**Date**: 2025-12-13
**To**: RemediationOrchestrator Team
**From**: Platform/Data Storage Team
**Priority**: 🟡 **MEDIUM**
**Effort**: 5-10 minutes
**Status**: ⚠️ **ACTION REQUIRED**

---

## 🎯 Summary

**Action Required**: Migrate RemediationOrchestrator service from deprecated `HTTPDataStorageClient` to OpenAPI-generated `OpenAPIAuditClient`.

**Why**:
- ✅ Type safety from OpenAPI specification
- ✅ Compile-time contract validation
- ✅ Breaking API changes caught during development
- ✅ Consistency with other services

**Impact**: RemediationOrchestrator generates orchestration audit traces for BR-ORCH-042 (action execution audit).

---

## 📋 Current State

**File**: `cmd/remediationorchestrator/main.go`
**Line**: TBD (search for `NewHTTPDataStorageClient`)

**Current Code** (Deprecated):
```go
dataStorageClient := audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)
```

**Problem**:
- ❌ No type safety (errors only at runtime)
- ❌ No compile-time contract validation
- ❌ Manual HTTP client (no OpenAPI spec)

---

## ✅ Required Changes

### Step 1: Find the Audit Client Creation

**Search for**:
```bash
grep -n "NewHTTPDataStorageClient" cmd/remediationorchestrator/main.go
```

This will show the exact line number where the client is created.

### Step 2: Update Imports

**Add this import**:
```go
import dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
```

### Step 3: Replace Client Creation

**OLD** (FIND AND REPLACE):
```go
dataStorageClient := audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)
```

**NEW** (REPLACE WITH):
```go
dataStorageClient, err := dsaudit.NewOpenAPIAuditClient(dataStorageURL, 5*time.Second)
if err != nil {
    return fmt.Errorf("failed to create audit client: %w", err)
}
```

**Note**: Adjust error handling based on surrounding code context. If RemediationOrchestrator has graceful degradation, use that pattern instead.

---

## 🧪 Testing Instructions

### 1. Unit Tests
```bash
make test-unit-remediationorchestrator
```

**Expected**: All tests pass

### 2. Integration Tests
```bash
make test-integration-remediationorchestrator
```

**Expected**: All tests pass

### 3. Controller Smoke Test
```bash
# Start RemediationOrchestrator controller
make run-remediationorchestrator

# Create test RemediationRequest resource
kubectl apply -f test/e2e/remediationorchestrator/test-request.yaml

# Verify audit traces
kubectl logs -n kubernaut-system deployment/remediationorchestrator | grep "audit"

# Check orchestration audit (BR-ORCH-042)
kubectl logs -n kubernaut-system deployment/remediationorchestrator | grep "action_execution"
```

---

## ✅ Acceptance Criteria

- [ ] Import added: `dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"`
- [ ] Client creation replaced with `dsaudit.NewOpenAPIAuditClient`
- [ ] Error handling added appropriately
- [ ] Unit tests pass: `make test-unit-remediationorchestrator`
- [ ] Integration tests pass: `make test-integration-remediationorchestrator`
- [ ] No compilation errors: `go build ./cmd/remediationorchestrator`
- [ ] Controller starts successfully in test environment
- [ ] Orchestration audit traces still being written (BR-ORCH-042)

---

## 🚨 Why This Matters for RemediationOrchestrator

**RemediationOrchestrator is the central orchestrator**:
- Coordinates entire remediation flow
- Creates AIAnalysis, WorkflowExecution, Notification CRDs
- Tracks execution state and outcomes
- Generates orchestration audit traces (BR-ORCH-042)

**Type safety prevents**:
- Lost orchestration audit traces
- Incorrect state tracking
- Breaking changes at runtime

**Business Requirements**:
- BR-ORCH-042: Action Execution Audit Trail (MANDATORY)
- BR-ORCH-029-034: Consecutive failure tracking (depends on audit)
- BR-ORCH-012: Remediation outcome tracking

**Business Context**:
- RemediationOrchestrator is the coordinator
- Audit data critical for effectiveness monitoring
- Tracks end-to-end remediation flow
- Supports consecutive failure detection (BR-ORCH-029-034)

---

## 📚 References

**OpenAPI Spec**: `api/openapi/data-storage-v1.yaml`
**OpenAPI Client**: `pkg/datastorage/audit/openapi_adapter.go`
**Business Requirements**: `docs/services/crd-controllers/05-remediationorchestrator/BUSINESS_REQUIREMENTS.md`

**Related Design Decisions**:
- DD-AUDIT-003: RemediationOrchestrator MUST generate audit traces
- DD-AUDIT-002: Use pkg/audit shared library
- BR-ORCH-042: Action Execution Audit Trail

**Related Business Requirements**:
- BR-ORCH-029-034: Consecutive failure tracking
- BR-ORCH-012: Remediation outcome tracking

---

## 🤝 Support

**Questions?** Ask in `#remediationorchestrator` or `#platform` Slack channels

**Issues?** The OpenAPI client is a drop-in replacement. If you encounter issues:
1. Verify imports are correct
2. Check error handling matches existing patterns
3. Run tests to identify specific failures
4. Contact Platform team if blocked

---

## ⏱️ Timeline

**Effort**: 5-10 minutes
**Deadline**: Next sprint
**Urgency**: Medium (orchestration audit is important, but not blocking)

---

**Status**: ⚠️ **WAITING FOR REMEDIATIONORCHESTRATOR TEAM**

---

**Created**: 2025-12-13
**Owner**: RemediationOrchestrator Team
**Reviewer**: Platform/Data Storage Team


