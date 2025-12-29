# DataStorage: Workflow Model Refactoring - DEFERRED TO V1.1 📋

**Date**: December 17, 2025
**Status**: 📋 **DEFERRED TO V1.1** - Comprehensive plan created for future implementation
**Decision**: Ship V1.0 with current (working) flat structure, execute refactoring in V1.1
**Reason**: 6-8 hour refactoring best done fresh; current structure works perfectly
**Plan**: See `DS_WORKFLOW_MODEL_REFACTORING_PLAN_V1_1.md` for detailed implementation guide

---

## 🎯 **Goal**

Refactor `RemediationWorkflow` from a flat 36-field struct to:
- **Grouped API models** (external interface) - Clean, logical grouping
- **Flat DB models** (internal persistence) - Simple sqlx scanning
- **Explicit conversion layer** - Testable, maintainable mapping

---

## 📊 **Current Structure** (BEFORE)

**File**: `pkg/datastorage/models/workflow.go`
- **Single flat struct**: `RemediationWorkflow` with 36 fields
- **Mixed concerns**: DB tags (`db:`) + API tags (`json:`) + validation
- **Good organization**: Comment-based grouping (10 logical sections)
- **Problem**: Verbose, hard to work with, mixed API/DB concerns

---

## 🎯 **Target Structure** (AFTER)

### **New File Structure**

```
pkg/datastorage/models/
  workflow.go          # API models (grouped structs) - External interface
  workflow_db.go       # DB models (flat struct) - Internal persistence
  workflow_convert.go  # Conversion layer - Explicit mapping
  workflow_labels.go   # Label types (UNCHANGED)
```

### **API Model Groups** (workflow.go)

1. **WorkflowIdentity** (3 fields)
   - WorkflowID, WorkflowName, Version

2. **WorkflowMetadata** (4 fields)
   - Name, Description, Owner, Maintainer

3. **WorkflowContent** (2 fields)
   - Content, ContentHash

4. **WorkflowExecution** (4 fields)
   - Parameters, ExecutionEngine, ContainerImage, ContainerDigest

5. **WorkflowLabels** (3 already-structured types)
   - Mandatory (MandatoryLabels), Custom (CustomLabels), Detected (DetectedLabels)

6. **WorkflowLifecycle** (5 fields)
   - Status, StatusReason, DisabledAt, DisabledBy, DisabledReason

7. **WorkflowVersion** (7 fields)
   - IsLatestVersion, PreviousVersion, DeprecationNotice
   - VersionNotes, ChangeSummary, ApprovedBy, ApprovedAt

8. **WorkflowMetrics** (5 fields)
   - ExpectedSuccessRate, ExpectedDurationSeconds
   - ActualSuccessRate, TotalExecutions, SuccessfulExecutions

9. **WorkflowAudit** (4 fields)
   - CreatedAt, UpdatedAt, CreatedBy, UpdatedBy

### **Grouped API Model**

```go
type RemediationWorkflow struct {
    Identity  WorkflowIdentity  `json:"identity"`
    Metadata  WorkflowMetadata  `json:"metadata"`
    Content   WorkflowContent   `json:"content"`
    Execution WorkflowExecution `json:"execution"`
    Labels    WorkflowLabels    `json:"labels"`
    Lifecycle WorkflowLifecycle `json:"lifecycle"`
    Version   WorkflowVersion   `json:"version"`
    Metrics   WorkflowMetrics   `json:"metrics"`
    Audit     WorkflowAudit     `json:"audit"`
}
```

### **Flat DB Model** (workflow_db.go)

```go
type workflowDB struct {
    // All 36 fields flat with db: tags
    WorkflowID   string `db:"workflow_id"`
    WorkflowName string `db:"workflow_name"`
    // ... (same as current, but only db tags)
}
```

### **Conversion Layer** (workflow_convert.go)

```go
func (db *workflowDB) ToAPI() *RemediationWorkflow
func (api *RemediationWorkflow) ToDB() *workflowDB
```

---

## 📋 **Refactoring Checklist** (10 tasks)

- [ ] **Task 1**: Create grouped API models in workflow.go
- [ ] **Task 2**: Create flat DB model in workflow_db.go
- [ ] **Task 3**: Create conversion layer in workflow_convert.go
- [ ] **Task 4**: Update repository CRUD to use DB model
- [ ] **Task 5**: Update repository search to use DB model
- [ ] **Task 6**: Update server handlers for grouped API model
- [ ] **Task 7**: Update OpenAPI spec with grouped schemas
- [ ] **Task 8**: Regenerate Go and Python clients
- [ ] **Task 9**: Update test fixtures
- [ ] **Task 10**: Verify compilation and run tests

---

## 📊 **Impact Analysis**

### **Files to Modify**

| File | Type | Effort | Status |
|------|------|--------|--------|
| `models/workflow.go` | Major rewrite | High | ⏳ In Progress |
| `models/workflow_db.go` | New file | Medium | ⏳ Pending |
| `models/workflow_convert.go` | New file | Medium | ⏳ Pending |
| `repository/workflow/crud.go` | Update scanning | Medium | ⏳ Pending |
| `repository/workflow/search.go` | Update scanning | Medium | ⏳ Pending |
| `server/workflow_handlers.go` | Minor updates | Low | ⏳ Pending |
| `api/openapi/data-storage-v1.yaml` | Schema restructure | High | ⏳ Pending |
| Test fixtures | Update structures | Medium | ⏳ Pending |

**Estimated Total Effort**: 4-6 hours

---

## 🎯 **Benefits**

### **API Model** (External)
- ✅ **Logical Grouping** - Identity, Metadata, Content, etc. are explicit
- ✅ **Self-Documenting** - Structure conveys meaning
- ✅ **Easier to Work With** - Pass around subgroups (e.g., just Metadata)
- ✅ **Better JSON Structure** - Nested, organized API responses
- ✅ **Reusability** - Can compose models from groups

### **DB Model** (Internal)
- ✅ **Simple Scanning** - sqlx works naturally with flat structs
- ✅ **No Magic** - Direct column → field mapping
- ✅ **Performance** - Zero overhead for DB operations
- ✅ **Clear Intent** - DB concerns separated from API concerns

### **Conversion Layer**
- ✅ **Explicit Mapping** - No hidden logic
- ✅ **Testable** - Can unit test conversions
- ✅ **Maintainable** - Clear place for transformation logic
- ✅ **Type-Safe** - Compile-time validation of field mappings

---

## 🚨 **Risks & Mitigations**

| Risk | Mitigation |
|------|-----------|
| **Conversion bugs** | Comprehensive unit tests for ToAPI()/ToDB() |
| **Performance overhead** | Minimal - just field copying, no allocations |
| **Missed fields** | Compile-time errors will catch missing mappings |
| **Test breakage** | Update fixtures systematically, one test suite at a time |

---

## 📝 **Migration Strategy**

### **Phase 1: Create New Models** ✅
1. Create grouped API models (workflow.go)
2. Create flat DB model (workflow_db.go)
3. Create conversion layer (workflow_convert.go)
4. Verify compilation

### **Phase 2: Update Repository Layer**
1. Update CRUD operations to use workflowDB
2. Update search operations to use workflowDB
3. Convert to/from API models at boundaries
4. Run repository tests

### **Phase 3: Update Server Layer**
1. Update handlers to use grouped API models
2. Update validation for grouped structure
3. Run server tests

### **Phase 4: Update API Contracts**
1. Update OpenAPI spec with grouped schemas
2. Regenerate Go client
3. Regenerate Python client
4. Verify client compilation

### **Phase 5: Update Tests**
1. Update test fixtures for new structure
2. Update assertions for grouped fields
3. Run full test suite

---

## 🎓 **Design Decisions**

### **Why Separate DB/API Models?**

**Decision**: Use separate models instead of embedded structs

**Rationale**:
1. **Clear Separation of Concerns** - API structure ≠ DB structure
2. **Simple DB Layer** - sqlx works naturally with flat structs
3. **Flexible API Evolution** - Can change API without DB impact
4. **Explicit Conversions** - No hidden magic, testable
5. **Performance** - Zero overhead for DB operations

**Alternative Considered**: Embedded structs with db:"-" tags
**Rejected Because**: Complex, confusing, hard to maintain

### **Why Group at All?**

**Decision**: Group fields into logical sections

**Rationale**:
1. **Better Developer Experience** - Easier to understand and work with
2. **Self-Documenting** - Structure conveys intent
3. **Composability** - Can pass around subgroups
4. **Better JSON** - Clients get organized responses
5. **Pre-Release Timing** - No external users yet

---

## 📚 **Related Documentation**

- **Workflow Label Refactoring**: `DS_WORKFLOW_LABELS_V1_0_COMPLETE_DEC_17_2025.md`
- **Authority**: DD-STORAGE-008 v2.0 (Workflow Catalog Schema)
- **Business Requirement**: BR-STORAGE-012 (Workflow Semantic Search)

---

## 🔄 **Progress Tracking**

**Started**: December 17, 2025
**Target Completion**: December 17, 2025 (same day)
**Current Phase**: Phase 1 - Create New Models

**Latest Update**: Refactoring approved, starting implementation

---

**Status**: ⏳ **IN PROGRESS** - Creating grouped API models
**Next Step**: Complete workflow.go with grouped structure

