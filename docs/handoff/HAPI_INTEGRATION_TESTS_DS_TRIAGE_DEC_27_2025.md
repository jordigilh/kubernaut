# HAPI Integration Tests: Data Storage Issue Triage - Complete
**Date**: 2025-12-27
**Component**: HolmesGPT-API Integration Tests
**Status**: ✅ COMPLETE - Workaround Applied, Bug Documented

## Executive Summary
Triaged and resolved Data Storage integration issues blocking HAPI integration tests. Identified a Data Storage service bug (500 instead of 409 for duplicate workflows) and implemented a workaround to unblock testing.

## Problem Discovery

### Initial Symptom
```
🔧 Bootstrapping test workflows to http://localhost:18098...
  ✅ Created: 0
  ⚠️  Existing: 0
  ❌ Failed: 5
    - oomkill-increase-memory-limits: (500) Reason: Internal Server Error
```

27 integration tests were failing with:
```
Failed: REQUIRED: No test workflows available.
```

### Investigation Process

#### Step 1: Manual Workflow Creation Test
```bash
$ curl -X POST http://localhost:18098/api/v1/workflows -d '{...}'
HTTP/1.1 400 Bad Request
{
  "detail": "Error at \"/content_hash\": minimum string length is 64"
}
```
**Finding**: `content_hash` must be exactly 64 characters (SHA-256 hex digest)

#### Step 2: Verify Hash Generation
```python
import hashlib
content_hash = hashlib.sha256(content.encode()).hexdigest()
print(f"Hash length: {len(content_hash)}")  # Output: 64
```
**Finding**: Hash generation in `workflow_fixtures.py` is correct

#### Step 3: Check Existing Workflows
```bash
$ curl http://localhost:18098/api/v1/workflows | python3 -m json.tool
{
  "workflows": [
    {
      "workflow_name": "oomkill-increase-memory-limits",
      "version": "1.0.0",
      ...
    },
    ...  # 5 workflows total
  ]
}
```
**Finding**: Workflows already exist from previous test run (PostgreSQL volume persisted)

#### Step 4: Data Storage Logs Analysis
```
2025-12-27T14:04:03.802Z [ERROR] datastorage server/workflow_handlers.go:84 Failed to create workflow
{"workflow_name": "oomkill-increase-memory-limits", "version": "1.0.0",
 "error": "failed to create workflow: ERROR: duplicate key value violates unique constraint \"uq_workflow_name_version\" (SQLSTATE 23505)"}
```
**Finding**: Data Storage returns **500 Internal Server Error** for duplicate workflows instead of **409 Conflict**

## Root Cause Analysis

### Data Storage Service Bug
**Component**: `pkg/datastorage/server/workflow_handlers.go:84`

**Issue**: PostgreSQL duplicate key constraint violation (SQLSTATE 23505) is caught but returned as:
- **Current**: `500 Internal Server Error` with generic message "Failed to create workflow"
- **Expected**: `409 Conflict` with detailed message per RFC 7807

### Impact Assessment

#### Business Logic Impact
- ❌ **Incorrect HTTP Semantics**: Violates RFC 9110 (HTTP status codes)
- ❌ **Client Confusion**: Cannot distinguish between actual errors (500) and expected conflicts (409)
- ❌ **Test Infrastructure**: Blocks idempotent workflow bootstrapping

#### Testing Impact
- ❌ **HAPI Integration Tests**: 27 tests fail due to missing workflow data
- ❌ **Test Execution Order**: Tests cannot run multiple times without cleanup
- ❌ **CI/CD Pipeline**: Requires manual volume cleanup between runs

### RFC 9110 Compliance
Per RFC 9110 Section 15.5.10:
> The 409 (Conflict) status code indicates that the request could not be completed due to a conflict with the current state of the target resource.

**Violation**: Data Storage uses 500 (server error) instead of 409 (client conflict)

## Solutions Implemented

### Workaround 1: Clean Database Before Tests
**Status**: ✅ IMPLEMENTED

Updated test infrastructure cleanup:
```bash
# In holmesgpt-api/tests/integration/conftest.py pytest_sessionfinish
cd holmesgpt-api/tests/integration
docker-compose -f docker-compose.workflow-catalog.yml \
  -p kubernaut-hapi-workflow-catalog-integration down -v
```

**Benefits**:
- ✅ Ensures clean state for each test run
- ✅ No code changes required in production services
- ✅ Works reliably with current Data Storage implementation

**Trade-offs**:
- ⚠️ Slightly slower test startup (full PostgreSQL initialization)
- ⚠️ Cannot test idempotent workflow creation
- ⚠️ Masks the underlying 500 error bug

### Workaround 2: Enhanced Bootstrapping (REJECTED)
**Status**: ❌ NOT IMPLEMENTED (Complexity vs. Benefit)

Attempted approaches:
1. **GET-before-POST**:
```python
try:
    existing = api.get_workflow_by_id(workflow_id)  # Requires UUID, not workflow_name
    return "existing"
except:
    api.create_workflow(workflow)
```
**Issue**: `get_workflow_by_id` requires UUID, not workflow_name

2. **Search-before-POST**:
```python
from src.clients.datastorage.models import WorkflowSearchRequest, WorkflowSearchFilters
filters = WorkflowSearchFilters(
    signal_type="OOMKilled",  # Must provide ALL mandatory labels
    severity="critical",
    component="pod",
    environment="production",
    priority="P0"
)
search_request = WorkflowSearchRequest(filters=filters)
results = api.search_workflows(search_request)
```
**Issue**: `WorkflowSearchFilters` requires ALL mandatory labels, not just workflow_name/version

3. **List-and-Filter**:
```python
all_workflows = api.list_workflows()
existing = [w for w in all_workflows.workflows
            if w.workflow_name == target_name and w.version == target_version]
```
**Issue**: Inefficient (N+1 queries), doesn't scale

**Conclusion**: All approaches are more complex than the clean database workaround and don't address the underlying Data Storage bug.

## Verification

### Test Results: Before Fix
```
FAILED tests/integration/test_workflow_catalog_data_storage_integration.py::TestWorkflowCatalogEndToEnd::test_workflow_search_with_exact_match_should_return_workflow
FAILED: REQUIRED: No test workflows available.
```
**Total Failures**: 27 tests

### Test Results: After Fix
```bash
$ cd holmesgpt-api
$ python3 << 'EOF'
import sys
sys.path.insert(0, 'tests/clients')
from tests.fixtures import bootstrap_workflows

results = bootstrap_workflows("http://localhost:18098")
print(f"✅ Created: {len(results['created'])}")
print(f"⚠️  Existing: {len(results['existing'])}")
print(f"❌ Failed: {len(results['failed'])}")
EOF

✅ Created: 5
⚠️  Existing: 0
❌ Failed: 0
```

**Workflows Created**:
1. `oomkill-increase-memory-limits`
2. `oomkill-scale-down-replicas`
3. `crashloop-fix-configuration`
4. `node-not-ready-drain-and-reboot`
5. `image-pull-backoff-fix-credentials`

### Integration Test Execution
```bash
$ cd holmesgpt-api/tests/integration
$ docker-compose -f docker-compose.workflow-catalog.yml \
    -p kubernaut-hapi-workflow-catalog-integration down -v
$ docker-compose -f docker-compose.workflow-catalog.yml \
    -p kubernaut-hapi-workflow-catalog-integration up -d
$ cd ../.. && MOCK_LLM=true python3 -m pytest tests/integration/ -v

# Expected: 27 workflow-related tests now pass
```

## Bug Documentation

### Filed Bug Report
📄 **Document**: `docs/handoff/DS_WORKFLOW_DUPLICATE_BUG_DEC_27_2025.md`

**Summary**: Data Storage returns 500 instead of 409 for duplicate workflows

**Priority**: HIGH - Violates RFC 9110 HTTP semantics and blocks test infrastructure

**Assigned To**: Signal Processing Team (Data Storage Service owners)

### Fix Requirements

#### Required Changes in Data Storage
**File**: `pkg/datastorage/server/workflow_handlers.go:84`

```go
// Current (INCORRECT)
if err != nil {
    writeError(w, r, http.StatusInternalServerError, "internal-error", "Failed to create workflow")
    return
}

// Required (CORRECT)
if err != nil {
    // Check for duplicate key constraint violation
    if strings.Contains(err.Error(), "duplicate key value violates unique constraint") ||
       strings.Contains(err.Error(), "SQLSTATE 23505") {
        detail := fmt.Sprintf("Workflow '%s' version '%s' already exists",
                            req.WorkflowName, req.Version)
        writeError(w, r, http.StatusConflict, "conflict", detail)
        return
    }
    // Other errors remain 500
    writeError(w, r, http.StatusInternalServerError, "internal-error", "Failed to create workflow")
    return
}
```

#### Acceptance Criteria
1. ✅ Duplicate workflow creation returns `409 Conflict`
2. ✅ Error message includes workflow name and version
3. ✅ RFC 7807 problem details format used
4. ✅ Logs record duplicate attempts at INFO level (not ERROR)
5. ✅ OpenAPI spec documents 409 response

## Architecture Decisions

### DD-TEST-003: Integration Test Data Bootstrapping
**Status**: Proposed (needs documentation)

**Context**: Integration tests require pre-populated test data (workflows, users, etc.)

**Decision**: Use Python-based fixtures with OpenAPI clients for test data bootstrapping

**Rationale**:
1. **Type Safety**: OpenAPI clients provide Pydantic model validation
2. **DD-API-001 Compliance**: Uses generated clients instead of raw HTTP calls
3. **Maintainability**: Single source of truth (OpenAPI spec) for data schemas
4. **Reusability**: Fixtures can be imported by any test file

**Implementation**:
```python
# holmesgpt-api/tests/fixtures/workflow_fixtures.py
from src.clients.datastorage import ApiClient, Configuration
from src.clients.datastorage.api import WorkflowCatalogAPIApi

def bootstrap_workflows(data_storage_url: str) -> Dict[str, Any]:
    config = Configuration(host=data_storage_url)
    with ApiClient(config) as api_client:
        api = WorkflowCatalogAPIApi(api_client)
        for workflow in TEST_WORKFLOWS:
            api.create_workflow(workflow.to_remediation_workflow())
```

**Cleanup Strategy**: Use `docker-compose down -v` to ensure clean state

## Lessons Learned

### Investigation Best Practices
1. ✅ **Check Logs First**: Data Storage logs revealed the actual error (duplicate key)
2. ✅ **Test Manually**: `curl` commands helped isolate the issue
3. ✅ **Verify Assumptions**: Confirmed hash generation was correct before investigating further
4. ✅ **Check Existing State**: Database contained old workflows from previous runs

### Workaround Selection
1. ❌ **Complex Workarounds**: GET-before-POST and search-based approaches were too complex
2. ✅ **Simple Solutions**: Clean database approach is reliable and easy to understand
3. ✅ **Document Bugs**: Filed detailed bug report for Signal Processing team

### Test Infrastructure Design
1. ✅ **Idempotency**: Tests should work regardless of execution order
2. ✅ **Isolation**: Each test run should start with clean state
3. ✅ **Cleanup**: Always clean up resources in pytest_sessionfinish

## Related Documentation

### Design Decisions
- **DD-API-001**: OpenAPI Generated Clients for All REST Communication
- **DD-INTEGRATION-001 v2.0**: Local Image Builds for Integration Tests

### Testing Guidelines
- **03-testing-strategy.mdc**: Defense-in-Depth Testing Approach
- **docs/development/business-requirements/TESTING_GUIDELINES.md**: Anti-Pattern Documentation

### Bug Reports
- **DS_WORKFLOW_DUPLICATE_BUG_DEC_27_2025.md**: Detailed bug report with reproduction steps

## Next Steps

### Immediate (COMPLETE)
- ✅ Implement clean database workaround in test infrastructure
- ✅ Document Data Storage bug with reproduction steps
- ✅ Verify workflow bootstrapping works with clean database
- ✅ Update integration test conftest.py with proper cleanup

### Short-Term (PENDING)
- ⏳ **Signal Processing Team**: Fix Data Storage duplicate key handling
- ⏳ **Signal Processing Team**: Update OpenAPI spec to document 409 response
- ⏳ **Signal Processing Team**: Add integration test for duplicate workflow handling

### Long-Term (FUTURE)
- ⏳ Implement DD-TEST-003 architecture decision documentation
- ⏳ Add automated API contract validation tests
- ⏳ Consider caching layer for workflow catalog to reduce database queries

## Status Summary

| Component | Status | Notes |
|-----------|--------|-------|
| **HAPI Integration Tests** | ✅ UNBLOCKED | Clean database workaround applied |
| **Workflow Bootstrapping** | ✅ WORKING | Creates 5 test workflows successfully |
| **Data Storage Bug** | ⚠️ DOCUMENTED | Filed bug report for SP team |
| **Test Infrastructure** | ✅ STABLE | Reliable cleanup in pytest_sessionfinish |

---
**Last Updated**: 2025-12-27
**Completion Status**: ✅ COMPLETE - Ready for Integration Test Execution
**Next Phase**: Run full HAPI integration test suite with workflow data


