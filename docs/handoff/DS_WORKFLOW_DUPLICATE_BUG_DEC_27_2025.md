# Data Storage Workflow Duplicate Key Handling Bug
**Date**: 2025-12-27
**Component**: Data Storage Service (cmd/datastorage)
**Status**: ⚠️ BUG IDENTIFIED - Needs Fix

## Problem Summary
Data Storage service returns **500 Internal Server Error** when attempting to create a workflow that already exists, instead of the correct **409 Conflict** status code.

## Technical Details

### Current Behavior (INCORRECT)
```bash
$ curl -X POST http://localhost:18098/api/v1/workflows -H "Content-Type: application/json" -d '{...existing workflow...}'
HTTP/1.1 500 Internal Server Error
{
  "type": "https://kubernaut.ai/problems/internal-error",
  "title": "Internal Server Error",
  "status": 500,
  "detail": "Failed to create workflow"
}
```

### Data Storage Logs
```
2025-12-27T14:04:03.802Z [ERROR] datastorage server/workflow_handlers.go:84 Failed to create workflow
{"workflow_name": "oomkill-increase-memory-limits", "version": "1.0.0",
 "error": "failed to create workflow: ERROR: duplicate key value violates unique constraint \"uq_workflow_name_version\" (SQLSTATE 23505)"}
```

### Expected Behavior (RFC 9110)
```bash
HTTP/1.1 409 Conflict
{
  "type": "https://kubernaut.ai/problems/conflict",
  "title": "Workflow Already Exists",
  "status": 409,
  "detail": "Workflow 'oomkill-increase-memory-limits' version '1.0.0' already exists"
}
```

## Root Cause
The Data Storage service catches the PostgreSQL `duplicate key` constraint violation but returns a generic 500 error instead of:
1. Detecting the specific error type (SQLSTATE 23505)
2. Returning the appropriate 409 Conflict status code
3. Providing a user-friendly error message

## Impact

### HAPI Integration Tests
- **Blocked**: Workflow bootstrapping fails with 500 errors when workflows already exist in PostgreSQL
- **Workaround Applied**: Clear PostgreSQL volume before test runs using `docker-compose down -v`

### API Clients
- Cannot distinguish between:
  - Actual server errors (500) → Requires investigation
  - Duplicate workflows (409) → Expected, safe to ignore

## Reproduction Steps

1. Start Data Storage service with clean PostgreSQL database:
```bash
cd holmesgpt-api/tests/integration
docker-compose -f docker-compose.workflow-catalog.yml -p kubernaut-hapi-workflow-catalog-integration up -d
sleep 15
```

2. Create a workflow (succeeds):
```bash
curl -X POST http://localhost:18098/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "workflow_name": "test-workflow",
    "version": "1.0.0",
    "name": "Test Workflow",
    "description": "A test workflow",
    "content": "apiVersion: kubernaut.io/v1alpha1\nkind: WorkflowSchema",
    "content_hash": "a".repeat(64),
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
# Response: 201 Created
```

3. Attempt to create the same workflow again (returns 500):
```bash
# Same curl command as above
# Response: 500 Internal Server Error (INCORRECT - should be 409 Conflict)
```

4. Verify logs:
```bash
podman logs kubernaut-hapi-data-storage-integration 2>&1 | grep "duplicate key"
# Shows: ERROR: duplicate key value violates unique constraint "uq_workflow_name_version" (SQLSTATE 23505)
```

## Fix Required

### Location
`pkg/datastorage/server/workflow_handlers.go:84` (or surrounding error handling logic)

### Required Changes
1. **Detect PostgreSQL duplicate key error**:
```go
if strings.Contains(err.Error(), "duplicate key value violates unique constraint") ||
   strings.Contains(err.Error(), "SQLSTATE 23505") {
    // Return 409 Conflict
    writeError(w, r, http.StatusConflict, "conflict", "Workflow already exists")
    return
}
```

2. **Provide detailed error message**:
```go
detail := fmt.Sprintf("Workflow '%s' version '%s' already exists", workflowName, version)
```

3. **Follow RFC 7807 problem details**:
```json
{
  "type": "https://kubernaut.ai/problems/conflict",
  "title": "Workflow Already Exists",
  "status": 409,
  "detail": "Workflow 'oomkill-increase-memory-limits' version '1.0.0' already exists"
}
```

## Testing Verification

### Acceptance Criteria
1. Duplicate workflow creation returns `409 Conflict`
2. Error message includes workflow name and version
3. RFC 7807 problem details format used
4. Logs still record the duplicate key attempt (INFO level, not ERROR)

### Test Case
```bash
# 1. Create workflow (should succeed)
curl -X POST http://localhost:18098/api/v1/workflows -d '{...}'
# Expected: 201 Created

# 2. Create same workflow again
curl -X POST http://localhost:18098/api/v1/workflows -d '{...}'
# Expected: 409 Conflict with RFC 7807 problem details

# 3. Verify logs
# Expected: INFO level message, not ERROR
```

## Workarounds

### Current HAPI Integration Tests
```bash
# Before test run - clear all volumes
cd holmesgpt-api/tests/integration
docker-compose -f docker-compose.workflow-catalog.yml -p kubernaut-hapi-workflow-catalog-integration down -v
docker-compose -f docker-compose.workflow-catalog.yml -p kubernaut-hapi-workflow-catalog-integration up -d
```

### Alternative: Idempotent Bootstrapping
```python
# Check if workflow exists first (inefficient but works)
try:
    api.create_workflow(workflow)
except Exception as e:
    if "500" in str(e):
        # Assume it might be a duplicate (can't distinguish from real errors)
        pass
    else:
        raise
```

## Related Issues

### Business Requirements
- **BR-STORAGE-005**: Workflow catalog CRUD operations must follow REST best practices

### Design Decisions
- **DD-007**: Error responses must follow RFC 7807 problem details format
- **ADR-031**: OpenAPI 3.0+ specification for all stateless REST APIs

### Testing
- **03-testing-strategy.mdc**: Integration tests must not depend on test execution order

## Priority
**HIGH** - Blocks proper integration test infrastructure and violates RFC 9110 HTTP semantics

## Next Steps
1. **Immediate**: Use `docker-compose down -v` workaround in HAPI integration tests
2. **Short-term**: Fix Data Storage duplicate key handling
3. **Verification**: Run integration tests with existing workflows to verify 409 handling
4. **Documentation**: Update OpenAPI spec to document 409 response for workflow creation

---
**Last Updated**: 2025-12-27
**Reporter**: AI Assistant (Kubernaut TDD Session)
**Assigned To**: Signal Processing Team (Data Storage Service owners)


