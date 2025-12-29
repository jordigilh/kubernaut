# DataStorage: Workflow Model NESTED Structure Implementation Plan - V1.0

**Date**: December 17, 2025
**Status**: 📋 **APPROVED FOR V1.0** - Ready for execution
**Priority**: P0 - V1.0 Blocking (pre-release breaking change)
**Estimated Effort**: 8-10 hours
**Confidence**: 85%

---

## 🎯 **Executive Summary**

**Decision**: Change workflow API structure from FLAT (36 fields) to NESTED (9 semantic groups)
**Authority**: DD-WORKFLOW-002 v4.0, [CONFIDENCE_ASSESSMENT](./CONFIDENCE_ASSESSMENT_FLAT_VS_NESTED_WORKFLOW_MODEL_DEC_17_2025.md)
**Status**: 50% complete (steps 1-5 of 10 done)
**Remaining**: Steps 6-10 (server handlers, OpenAPI, clients, tests)

---

## 📊 **Current Progress**

### **✅ COMPLETED** (Steps 1-5)

| Step | Task | Status | Files | Effort |
|---|---|---|---|---|
| 1 | Create grouped API model | ✅ DONE | `workflow_api.go` (236 lines) | 1 hour |
| 2 | Create flat DB model | ✅ DONE | `workflow_db.go` (147 lines) | 30 min |
| 3 | Create conversion layer | ✅ DONE | `workflow_convert.go` (168 lines) | 45 min |
| 4 | Update repository CRUD | ✅ DONE | `repository/workflow/crud.go` (9 methods) | 1 hour |
| 5 | Update repository search | ✅ DONE | `repository/workflow/search.go` (1 type) | 15 min |

**Total Completed**: ~3.5 hours

---

### **⏳ REMAINING** (Steps 6-10)

| Step | Task | Status | Files | Effort |
|---|---|---|---|---|
| 6 | Complete server handlers | ⏳ TODO | `server/workflow_handlers.go` (~8 handlers) | 2 hours |
| 7 | Update OpenAPI spec | ⏳ TODO | `api/openapi/data-storage-v1.yaml` | 1.5 hours |
| 8 | Regenerate Go/Python clients | ⏳ TODO | `pkg/datastorage/client/`, `holmesgpt-api/src/clients/` | 30 min |
| 9 | Update test fixtures | ⏳ TODO | `test/` (integration/e2e) | 1.5 hours |
| 10 | Verify compilation & tests | ⏳ TODO | All DS tests | 1 hour |

**Total Remaining**: ~6.5 hours

---

## 📋 **Step-by-Step Implementation Plan**

---

### **STEP 6: Complete Server Handlers** ⏳

**Status**: 🔄 PARTIALLY COMPLETE (1/8 handlers done)
**Effort**: 2 hours
**Risk**: LOW

#### **Files to Update**

**File**: `pkg/datastorage/server/workflow_handlers.go`

**Handlers to Update** (8 total):

| Handler | Method | Status | Changes |
|---|---|---|---|
| `HandleCreateWorkflow` | POST | 🔄 50% DONE | Convert API→DB, create, convert DB→API response |
| `HandleGetWorkflow` | GET | ⏳ TODO | Fetch DB model, convert to API response |
| `HandleListWorkflows` | GET | ⏳ TODO | Fetch list, convert each to API response |
| `HandleUpdateWorkflow` | PUT | ⏳ TODO | Parse API, convert to DB, update, convert back |
| `HandleSearchWorkflows` | POST | ⏳ TODO | Already returns `WorkflowSearchResult` (has `WorkflowDB`) |
| `HandleGetLatestVersion` | GET | ⏳ TODO | Fetch DB model, convert to API response |
| `HandleGetVersionsByName` | GET | ⏳ TODO | Fetch list, convert each to API response |
| `HandleUpdateStatus` | PATCH | ⏳ TODO | Update DB model directly (no full conversion needed) |

#### **Pattern to Apply**

```go
// BEFORE (FLAT):
func (h *Handler) HandleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
    var workflow models.RemediationWorkflow  // ❌ FLAT
    json.NewDecoder(r.Body).Decode(&workflow)
    h.workflowRepo.Create(r.Context(), &workflow)
    json.NewEncoder(w).Encode(workflow)      // ❌ FLAT response
}

// AFTER (NESTED):
func (h *Handler) HandleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
    var workflowAPI models.WorkflowAPI       // ✅ NESTED API model
    json.NewDecoder(r.Body).Decode(&workflowAPI)

    workflowDB := workflowAPI.ToDB()         // ✅ Convert to DB model
    h.workflowRepo.Create(r.Context(), workflowDB)

    responseAPI := workflowDB.ToAPI()        // ✅ Convert back to API model
    json.NewEncoder(w).Encode(responseAPI)   // ✅ NESTED response
}
```

#### **Validation Required**

- ✅ Request body parses correctly into `WorkflowAPI` (grouped structure)
- ✅ Conversion `WorkflowAPI → WorkflowDB` preserves all fields
- ✅ Conversion `WorkflowDB → WorkflowAPI` preserves all fields
- ✅ Response JSON matches OpenAPI spec (grouped structure)

---

### **STEP 7: Update OpenAPI Spec** ⏳

**Status**: ⏳ TODO
**Effort**: 1.5 hours
**Risk**: MEDIUM (affects client generation)

#### **File to Update**

**File**: `api/openapi/data-storage-v1.yaml`

#### **Changes Required**

**1. Update `RemediationWorkflow` Schema** (Lines ~1218-1350)

**BEFORE** (FLAT - Current):
```yaml
RemediationWorkflow:
  type: object
  required: [workflow_name, version, name, description, content, content_hash, labels, execution_engine, status]
  properties:
    workflow_id:
      type: string
      format: uuid
    workflow_name:
      type: string
    version:
      type: string
    name:
      type: string
    description:
      type: string
    # ... 31 more flat fields
```

**AFTER** (NESTED - Target):
```yaml
RemediationWorkflow:
  type: object
  required: [identity, metadata, content, execution, labels, lifecycle, metrics, audit]
  properties:
    identity:
      $ref: '#/components/schemas/WorkflowIdentity'
    metadata:
      $ref: '#/components/schemas/WorkflowMetadata'
    content:
      $ref: '#/components/schemas/WorkflowContent'
    execution:
      $ref: '#/components/schemas/WorkflowExecution'
    labels:
      $ref: '#/components/schemas/WorkflowLabels'
    lifecycle:
      $ref: '#/components/schemas/WorkflowLifecycle'
    metrics:
      $ref: '#/components/schemas/WorkflowMetrics'
    audit:
      $ref: '#/components/schemas/WorkflowAudit'
```

**2. Add 8 New Component Schemas**

```yaml
WorkflowIdentity:
  type: object
  required: [workflow_id, workflow_name, version]
  properties:
    workflow_id:
      type: string
      format: uuid
      description: "UUID primary key (auto-generated)"
    workflow_name:
      type: string
      maxLength: 255
      description: "Human-readable identifier"
    version:
      type: string
      maxLength: 50
      description: "Semantic version (e.g., v1.0.0)"

WorkflowMetadata:
  type: object
  required: [name, description]
  properties:
    name:
      type: string
      maxLength: 255
      description: "Workflow title"
    description:
      type: string
      description: "Workflow description"
    owner:
      type: string
      maxLength: 255
      description: "Workflow owner"
    maintainer:
      type: string
      maxLength: 255
      format: email
      description: "Maintainer email"

WorkflowContent:
  type: object
  required: [content, content_hash]
  properties:
    content:
      type: string
      description: "YAML workflow definition"
    content_hash:
      type: string
      minLength: 64
      maxLength: 64
      description: "SHA-256 hash"

WorkflowExecution:
  type: object
  required: [execution_engine]
  properties:
    parameters:
      type: object
      additionalProperties: true
      description: "Execution parameters"
    execution_engine:
      type: string
      description: "Engine (tekton, argo-workflows, etc.)"
    container_image:
      type: string
      description: "OCI image reference"
    container_digest:
      type: string
      description: "SHA-256 digest"

WorkflowLabels:
  type: object
  required: [mandatory]
  properties:
    mandatory:
      $ref: '#/components/schemas/MandatoryLabels'
    custom:
      $ref: '#/components/schemas/CustomLabels'
    detected:
      $ref: '#/components/schemas/DetectedLabels'

WorkflowLifecycle:
  type: object
  required: [status, is_latest_version]
  properties:
    status:
      type: string
      enum: [active, disabled, deprecated, archived]
    status_reason:
      type: string
    disabled_at:
      type: string
      format: date-time
    disabled_by:
      type: string
    disabled_reason:
      type: string
    is_latest_version:
      type: boolean
    previous_version:
      type: string
    deprecation_notice:
      type: string
    version_notes:
      type: string
    change_summary:
      type: string
    approved_by:
      type: string
    approved_at:
      type: string
      format: date-time

WorkflowMetrics:
  type: object
  required: [total_executions, successful_executions]
  properties:
    expected_success_rate:
      type: number
      format: float
      minimum: 0
      maximum: 1
    expected_duration_seconds:
      type: integer
      minimum: 0
    actual_success_rate:
      type: number
      format: float
      minimum: 0
      maximum: 1
    total_executions:
      type: integer
      minimum: 0
    successful_executions:
      type: integer
      minimum: 0

WorkflowAudit:
  type: object
  required: [created_at, updated_at]
  properties:
    created_at:
      type: string
      format: date-time
    updated_at:
      type: string
      format: date-time
    created_by:
      type: string
    updated_by:
      type: string
```

#### **Validation Required**

- ✅ OpenAPI spec validates (no errors)
- ✅ All existing endpoints still work
- ✅ Example responses match new structure

---

### **STEP 8: Regenerate Go and Python Clients** ⏳

**Status**: ⏳ TODO
**Effort**: 30 minutes
**Risk**: LOW (automated generation)

#### **Commands to Execute**

**1. Regenerate Go Client**

```bash
# From project root
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Regenerate Go client
make generate-datastorage-client
# OR directly:
oapi-codegen -package client -generate types,client \
  -o pkg/datastorage/client/generated.go \
  api/openapi/data-storage-v1.yaml
```

**Expected Changes**:
- `pkg/datastorage/client/generated.go`: New types `WorkflowIdentity`, `WorkflowMetadata`, etc.
- `RemediationWorkflow` struct now has nested fields

**2. Regenerate Python Client**

```bash
# From project root
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut/holmesgpt-api

# Regenerate Python client
./src/clients/generate-datastorage-client.sh
```

**Expected Changes**:
- `holmesgpt-api/src/clients/datastorage/`: New Python models with nested structure

#### **Validation Required**

- ✅ Go client compiles without errors
- ✅ Python client generates without errors
- ✅ Type checking passes (Go + Python)

---

### **STEP 9: Update Test Fixtures** ⏳

**Status**: ⏳ TODO
**Effort**: 1.5 hours
**Risk**: MEDIUM (many test files)

#### **Files to Update**

**Identified Test Files** (from earlier grep):

| File | Lines | Type | Changes Required |
|---|---|---|---|
| `test/infrastructure/workflow_bundles.go` | 340 | Infrastructure | Update `BuildAndRegisterTestWorkflows` |
| `test/integration/datastorage/workflow_repository_integration_test.go` | ~500 | Integration | Update 5 test workflows |
| `test/integration/datastorage/openapi_helpers.go` | ~100 | Helpers | Update helper functions |
| `test/integration/datastorage/workflow_bulk_import_performance_test.go` | ~200 | Performance | Update bulk fixtures |

#### **Pattern to Apply**

**BEFORE** (FLAT):
```go
testWorkflow := &models.RemediationWorkflow{
    WorkflowName:    "test-workflow",
    Version:         "v1.0.0",
    Name:            "Test Workflow",
    Description:     "Test description",
    Content:         content,
    ContentHash:     contentHash,
    Labels:          models.MandatoryLabels{...},
    Status:          "active",
    ExecutionEngine: "tekton",
    IsLatestVersion: true,
}
```

**AFTER** (NESTED):
```go
testWorkflowAPI := &models.WorkflowAPI{
    Identity: models.WorkflowIdentity{
        WorkflowName: "test-workflow",
        Version:      "v1.0.0",
    },
    Metadata: models.WorkflowMetadata{
        Name:        "Test Workflow",
        Description: "Test description",
    },
    Content: models.WorkflowContent{
        Content:     content,
        ContentHash: contentHash,
    },
    Execution: models.WorkflowExecution{
        ExecutionEngine: "tekton",
    },
    Labels: models.WorkflowLabels{
        Mandatory: models.MandatoryLabels{...},
    },
    Lifecycle: models.WorkflowLifecycle{
        Status:          "active",
        IsLatestVersion: true,
    },
}

// Convert to DB model for repository operations
testWorkflowDB := testWorkflowAPI.ToDB()
```

#### **Validation Required**

- ✅ All integration tests pass
- ✅ All E2E tests pass
- ✅ Bulk import tests pass

---

### **STEP 10: Verify Compilation and Run Tests** ⏳

**Status**: ⏳ TODO
**Effort**: 1 hour
**Risk**: MEDIUM (may find issues requiring fixes)

#### **Validation Commands**

```bash
# 1. Verify Go compilation
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go build ./pkg/datastorage/...

# 2. Run linter
golangci-lint run ./pkg/datastorage/...

# 3. Run unit tests
go test ./pkg/datastorage/models/... -v
go test ./pkg/datastorage/repository/... -v
go test ./pkg/datastorage/server/... -v

# 4. Run integration tests
go test ./test/integration/datastorage/... -v

# 5. Run E2E tests (if applicable)
go test ./test/e2e/workflowexecution/... -v
```

#### **Expected Results**

- ✅ Zero compilation errors
- ✅ Zero linter errors
- ✅ All unit tests pass (100%)
- ✅ All integration tests pass (100%)
- ✅ All E2E tests pass (100%)

#### **If Issues Found**

1. **Compilation Errors**: Fix conversion layer bugs
2. **Linter Errors**: Add missing godoc comments, fix unused vars
3. **Test Failures**: Update test expectations, fix conversion bugs
4. **Performance Issues**: Profile and optimize conversion layer

---

## 📊 **Risk Assessment**

### **Overall Risk: LOW-MEDIUM** ⚠️

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| **Conversion layer bugs** | MEDIUM (40%) | MEDIUM | Comprehensive unit tests for ToAPI/ToDB |
| **OpenAPI spec errors** | LOW (20%) | HIGH | Validate with `swagger-cli validate` |
| **Client generation failures** | LOW (15%) | MEDIUM | Test with sample payloads |
| **Test fixture update errors** | MEDIUM (30%) | MEDIUM | Update incrementally, test after each |
| **Performance regression** | LOW (10%) | LOW | Conversion is cheap (field copying) |
| **Breaking existing integrations** | LOW (5%) | HIGH | Pre-release - no customers yet |

**Overall Assessment**: LOW-MEDIUM risk - Most risks are mitigatable through testing

---

## ⏱️ **Timeline Estimate**

### **Optimistic** (No Issues Found)
- Step 6: 1.5 hours
- Step 7: 1 hour
- Step 8: 20 minutes
- Step 9: 1 hour
- Step 10: 30 minutes
- **Total**: 4.5 hours

### **Realistic** (Minor Issues Found)
- Step 6: 2 hours
- Step 7: 1.5 hours
- Step 8: 30 minutes
- Step 9: 1.5 hours
- Step 10: 1 hour (includes fixes)
- **Total**: 6.5 hours

### **Pessimistic** (Major Issues Found)
- Step 6: 3 hours
- Step 7: 2 hours
- Step 8: 1 hour (client bugs)
- Step 9: 2 hours (many test failures)
- Step 10: 2 hours (debugging)
- **Total**: 10 hours

**Most Likely**: **6.5 hours** (realistic scenario)

---

## ✅ **Success Criteria**

### **Definition of Done**

- ✅ All 10 steps completed
- ✅ Zero compilation errors
- ✅ Zero linter errors
- ✅ 100% unit test pass rate
- ✅ 100% integration test pass rate
- ✅ OpenAPI spec validates
- ✅ Go client generates without errors
- ✅ Python client generates without errors
- ✅ DD-WORKFLOW-002 updated to v4.0
- ✅ Conversion layer has comprehensive tests

### **Quality Gates**

**Before Merge**:
1. All tests pass (`go test ./... -v`)
2. No linter errors (`golangci-lint run`)
3. OpenAPI spec validates (`swagger-cli validate`)
4. Clients regenerate cleanly
5. Code review approved

---

## 📋 **Documentation Updates Required**

### **Design Decisions** (Already Done ✅)

- ✅ DD-WORKFLOW-002 updated to v4.0
- ✅ Confidence assessment created

### **Additional Docs** (TODO)

| Document | Status | Changes |
|---|---|---|
| DD-STORAGE-008 | ⏳ TODO | Add v2.1: Clarify DB schema remains flat, API response is nested |
| API documentation | ⏳ TODO | Update examples to show nested structure |
| Client usage guides | ⏳ TODO | Update Go/Python examples |

---

## 🎯 **Execution Order**

### **Recommended Sequence**

1. **Step 7** (OpenAPI spec) - Do FIRST so clients can be generated
2. **Step 8** (Regenerate clients) - Depends on Step 7
3. **Step 6** (Server handlers) - Can use new clients for testing
4. **Step 9** (Test fixtures) - Update after handlers work
5. **Step 10** (Verify & test) - Final validation

**Alternative Sequence** (if handlers are simpler):
1. Step 6 → Step 7 → Step 8 → Step 9 → Step 10

---

## 🚀 **Ready to Execute**

**Status**: 📋 **APPROVED FOR V1.0**
**Confidence**: 85%
**Estimated Time**: 6.5 hours (realistic)
**Risk**: LOW-MEDIUM

**Next Steps**:
1. Review this plan with user
2. Get approval to proceed
3. Execute steps 6-10 in order
4. Validate at each step
5. Create handoff document when complete

---

**Plan Complete**: December 17, 2025
**Status**: Ready for execution
**Awaiting**: User approval to proceed

---

**End of Implementation Plan**

