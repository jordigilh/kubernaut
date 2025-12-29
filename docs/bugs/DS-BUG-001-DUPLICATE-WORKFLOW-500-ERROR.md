# BUG REPORT: DS-BUG-001 - Duplicate Workflow Returns 500 Instead of 409

**Date Reported**: 2025-12-27
**Reporter**: HolmesGPT-API Team
**Component**: Data Storage Service (cmd/datastorage)
**Severity**: HIGH (Blocks integration tests, violates HTTP standards)
**Priority**: P1 (Should be fixed in next sprint)

---

## Executive Summary

Data Storage service returns **500 Internal Server Error** when attempting to create a workflow that already exists, instead of the RFC 9110-compliant **409 Conflict** status code. This prevents API clients from distinguishing between actual server errors and expected conflict conditions.

---

## Problem Statement

### Current Behavior (INCORRECT) ❌

```bash
# First request - creates workflow successfully
$ curl -X POST http://localhost:18098/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{"workflow_name": "test", "version": "1.0.0", ...}'
HTTP/1.1 201 Created

# Second request - same workflow
$ curl -X POST http://localhost:18098/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{"workflow_name": "test", "version": "1.0.0", ...}'
HTTP/1.1 500 Internal Server Error  ❌ WRONG
{
  "type": "https://kubernaut.ai/problems/internal-error",
  "title": "Internal Server Error",
  "status": 500,
  "detail": "Failed to create workflow"
}
```

### Expected Behavior (RFC 9110 Compliant) ✅

```bash
HTTP/1.1 409 Conflict  ✅ CORRECT
{
  "type": "https://kubernaut.ai/problems/conflict",
  "title": "Workflow Already Exists",
  "status": 409,
  "detail": "Workflow 'test' version '1.0.0' already exists"
}
```

---

## Root Cause

**File**: `pkg/datastorage/server/workflow_handlers.go`
**Function**: `HandleCreateWorkflow`
**Lines**: 83-90

### Current Code (Problematic)

```go
// Line 83-90
if err := h.workflowRepo.Create(r.Context(), &workflow); err != nil {
    h.logger.Error(err, "Failed to create workflow",
        "workflow_name", workflow.WorkflowName,
        "version", workflow.Version,
    )
    // ❌ PROBLEM: Returns 500 for ALL errors
    response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
        "Failed to create workflow", h.logger)
    return
}
```

### PostgreSQL Error Received

```
ERROR: duplicate key value violates unique constraint "uq_workflow_name_version" (SQLSTATE 23505)
```

The handler catches this error but doesn't distinguish it from other database errors, returning a generic 500 for all cases.

---

## Recommended Fix

### Proposed Code Change

```go
// pkg/datastorage/server/workflow_handlers.go:83-97

import (
    "strings"
)

if err := h.workflowRepo.Create(r.Context(), &workflow); err != nil {
    h.logger.Error(err, "Failed to create workflow",
        "workflow_name", workflow.WorkflowName,
        "version", workflow.Version,
    )

    // ✅ FIX: Detect PostgreSQL duplicate key constraint
    if strings.Contains(err.Error(), "duplicate key value violates unique constraint") ||
       strings.Contains(err.Error(), "SQLSTATE 23505") {
        // Return 409 Conflict per RFC 9110
        detail := fmt.Sprintf("Workflow '%s' version '%s' already exists",
                            workflow.WorkflowName, workflow.Version)
        response.WriteRFC7807Error(w, http.StatusConflict, "conflict",
            "Workflow Already Exists", detail, h.logger)
        return
    }

    // Other errors remain 500 Internal Server Error
    response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error",
        "Internal Server Error", "Failed to create workflow", h.logger)
    return
}
```

### Alternative: Use PostgreSQL Error Codes

For a more robust solution, use the `pgx` error types:

```go
import (
    "github.com/jackc/pgx/v5/pgconn"
)

if err := h.workflowRepo.Create(r.Context(), &workflow); err != nil {
    // Check for PostgreSQL unique constraint violation
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) && pgErr.Code == "23505" {
        detail := fmt.Sprintf("Workflow '%s' version '%s' already exists",
                            workflow.WorkflowName, workflow.Version)
        response.WriteRFC7807Error(w, http.StatusConflict, "conflict",
            "Workflow Already Exists", detail, h.logger)
        return
    }

    // Other errors remain 500
    h.logger.Error(err, "Failed to create workflow",
        "workflow_name", workflow.WorkflowName,
        "version", workflow.Version,
    )
    response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error",
        "Internal Server Error", "Failed to create workflow", h.logger)
    return
}
```

---

## Impact Assessment

### Business Impact

| Area | Impact | Details |
|------|--------|---------|
| **API Clients** | HIGH | Cannot distinguish between server errors (500) and conflicts (409) |
| **Integration Tests** | HIGH | HAPI integration tests blocked - requires workaround |
| **HTTP Compliance** | HIGH | Violates RFC 9110 Section 15.5.10 (409 Conflict) |
| **Error Handling** | MEDIUM | Clients can't implement proper retry logic |
| **Observability** | MEDIUM | Logs show ERROR level for expected conditions |

### Current Workarounds

**HAPI Integration Tests** use this workaround:
```bash
# Clear database before each test run
docker-compose down -v && docker-compose up -d
```

This is inefficient and prevents testing idempotent workflow creation.

---

## Reproduction Steps

### Prerequisites
- Data Storage service running
- PostgreSQL database accessible
- Any HTTP client (curl, Postman, etc.)

### Steps to Reproduce

1. **Start Data Storage with clean database**:
```bash
cd holmesgpt-api/tests/integration
docker-compose -f docker-compose.workflow-catalog.yml up -d
sleep 15  # Wait for service startup
```

2. **Create a workflow** (first attempt - succeeds):
```bash
curl -X POST http://localhost:18098/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "workflow_name": "test-workflow",
    "version": "1.0.0",
    "name": "Test Workflow",
    "description": "A test workflow",
    "content": "apiVersion: kubernaut.io/v1alpha1\nkind: WorkflowSchema",
    "content_hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "labels": {
      "signal_type": "OOMKilled",
      "severity": "critical",
      "component": "pod",
      "environment": "production",
      "priority": "P0"
    },
    "execution_engine": "tekton",
    "status": "active",
    "container_image": "test:v1.0.0",
    "container_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000001"
  }'

# Expected: 201 Created
```

3. **Attempt to create the same workflow again** (triggers bug):
```bash
# Use exact same curl command as step 2

# Actual: 500 Internal Server Error ❌
# Expected: 409 Conflict ✅
```

4. **Check Data Storage logs**:
```bash
podman logs [data-storage-container] 2>&1 | grep "duplicate key"

# Shows:
# ERROR: duplicate key value violates unique constraint "uq_workflow_name_version" (SQLSTATE 23505)
```

---

## Testing Verification

### Acceptance Criteria

After implementing the fix, verify:

1. **✅ Duplicate workflow creation returns 409 Conflict**
   ```bash
   $ curl -X POST http://localhost:18098/api/v1/workflows -d '{...duplicate...}'
   HTTP/1.1 409 Conflict
   ```

2. **✅ Error message includes workflow name and version**
   ```json
   {
     "detail": "Workflow 'test-workflow' version '1.0.0' already exists"
   }
   ```

3. **✅ RFC 7807 problem details format used**
   ```json
   {
     "type": "https://kubernaut.ai/problems/conflict",
     "title": "Workflow Already Exists",
     "status": 409,
     "detail": "..."
   }
   ```

4. **✅ Logs use INFO level for duplicate attempts (not ERROR)**
   ```
   INFO: Workflow creation skipped - already exists
   ```

5. **✅ Other database errors still return 500**
   - Connection failures
   - Constraint violations (other than unique key)
   - Transaction errors

### Test Cases

```go
// Suggested integration test
func TestHandleCreateWorkflow_Duplicate_Returns409(t *testing.T) {
    // 1. Create workflow (should succeed with 201)
    workflow := createTestWorkflow("test", "1.0.0")
    resp1 := createWorkflow(t, workflow)
    assert.Equal(t, http.StatusCreated, resp1.StatusCode)

    // 2. Create same workflow again (should fail with 409)
    resp2 := createWorkflow(t, workflow)
    assert.Equal(t, http.StatusConflict, resp2.StatusCode)

    // 3. Verify error message
    var problemDetails ProblemDetails
    json.Unmarshal(resp2.Body, &problemDetails)
    assert.Equal(t, "conflict", problemDetails.Type)
    assert.Contains(t, problemDetails.Detail, "already exists")
    assert.Contains(t, problemDetails.Detail, "test")
    assert.Contains(t, problemDetails.Detail, "1.0.0")
}
```

---

## OpenAPI Specification Update

After implementing the fix, update the OpenAPI spec:

**File**: `api/datastorage/openapi.yaml` (or embedded spec)

```yaml
paths:
  /api/v1/workflows:
    post:
      summary: Create a new workflow
      operationId: createWorkflow
      responses:
        '201':
          description: Workflow created successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/RemediationWorkflow'
        '409':  # ✅ ADD THIS
          description: Workflow already exists
          content:
            application/problem+json:
              schema:
                $ref: '#/components/schemas/ProblemDetails'
              example:
                type: "https://kubernaut.ai/problems/conflict"
                title: "Workflow Already Exists"
                status: 409
                detail: "Workflow 'test-workflow' version '1.0.0' already exists"
        '400':
          description: Invalid request
          ...
        '500':
          description: Internal server error
          ...
```

---

## Related Standards & Requirements

### RFC 9110 (HTTP Semantics)

**Section 15.5.10 - 409 Conflict**:
> The 409 (Conflict) status code indicates that the request could not be completed due to a conflict with the current state of the target resource.

**Why 409 is correct**:
- The request is valid (passes OpenAPI validation)
- The conflict is with existing database state (duplicate workflow)
- The client can take action (use existing workflow or create different version)

### RFC 7807 (Problem Details)

All error responses should follow problem details format:
```json
{
  "type": "https://kubernaut.ai/problems/conflict",
  "title": "Workflow Already Exists",
  "status": 409,
  "detail": "Workflow 'name' version 'version' already exists"
}
```

### Business Requirements

- **BR-STORAGE-014**: Workflow catalog management must follow REST best practices
- **DD-007**: Error responses must follow RFC 7807 problem details format
- **ADR-031**: OpenAPI 3.0+ specification for all stateless REST APIs

---

## Similar Issues to Check

While fixing this issue, please check if similar problems exist in:

1. **Other Create Operations**:
   - ✅ Check: Incident creation (duplicate incident_id?)
   - ✅ Check: Audit event creation (duplicate event_id?)
   - ✅ Check: User/tenant creation (duplicate username?)

2. **Update Operations**:
   - ✅ Check: Workflow updates with version conflicts
   - ✅ Check: Optimistic locking failures

3. **Error Code Patterns**:
   - Search for: `http.StatusInternalServerError` in all handlers
   - Verify: Are PostgreSQL errors properly categorized?

---

## Timeline & Priority

### Recommended Timeline

| Phase | Duration | Deliverable |
|-------|----------|-------------|
| **Analysis** | 0.5 days | Confirm root cause, identify similar issues |
| **Implementation** | 0.5 days | Fix workflow handler, add tests |
| **Testing** | 0.5 days | Integration tests, verify fix |
| **Documentation** | 0.25 days | Update OpenAPI spec, changelog |
| **Total** | **1.75 days** | |

### Priority Justification

**P1 (High Priority)** because:
1. ❌ Violates HTTP standards (RFC 9110)
2. ❌ Blocks HAPI integration tests (requires workaround)
3. ❌ Poor API client experience (can't distinguish errors)
4. ✅ Easy fix (< 10 lines of code)
5. ✅ No breaking changes (only improves error responses)

---

## Questions for Data Storage Team

1. **PostgreSQL Error Handling**: Do you prefer string matching or `pgconn.PgError` types?
2. **Logging Level**: Should duplicate workflow attempts log at INFO or WARN level?
3. **Similar Issues**: Have you seen similar error handling issues in other endpoints?
4. **Testing**: Do you have integration tests for error conditions we should update?
5. **OpenAPI Spec**: Where is the canonical OpenAPI spec stored for updates?

---

## Contact Information

**Reporter**: HolmesGPT-API Team
**For Questions**: Reach out through team channels
**Related Documentation**:
- Bug Details: `docs/handoff/DS_WORKFLOW_DUPLICATE_BUG_DEC_27_2025.md`
- HAPI Triage: `docs/handoff/HAPI_INTEGRATION_TESTS_DS_TRIAGE_DEC_27_2025.md`

---

## Appendix: Log Evidence

### Data Storage Service Logs

```
2025-12-27T14:04:03.802Z [ERROR] datastorage server/workflow_handlers.go:84 Failed to create workflow
{
  "workflow_name": "oomkill-increase-memory-limits",
  "version": "1.0.0",
  "error": "failed to create workflow: ERROR: duplicate key value violates unique constraint \"uq_workflow_name_version\" (SQLSTATE 23505)"
}
```

### HTTP Response Evidence

```
HTTP/1.1 500 Internal Server Error
Content-Type: application/problem+json
Date: Sat, 27 Dec 2025 14:04:03 GMT

{
  "type": "https://kubernaut.ai/problems/internal-error",
  "title": "Internal Server Error",
  "status": 500,
  "detail": "Failed to create workflow"
}
```

---

**Last Updated**: 2025-12-27
**Status**: 🔴 **OPEN** - Awaiting Data Storage Team Review
**Tracking**: DS-BUG-001


