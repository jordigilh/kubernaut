# Workflow Seeding Refactoring Triage

**Date**: February 2, 2026  
**Status**: ⚠️ **PARTIAL REFACTORING - INCONSISTENCIES FOUND**  
**Issue**: Multiple implementations causing confusion and potential bugs

---

## 🎯 Executive Summary

**Finding**: The workflow seeding refactoring is **incomplete**. There are:
- 1 shared library (✅ correct implementation)
- 3 wrapper functions (2 correct, 1 OLD)
- 2 inline definitions (duplication)
- 1 OLD integration test helper (not refactored)

**Impact**: Code duplication, inconsistent patterns, maintenance burden

---

## 📊 Current State Analysis

### ✅ Shared Library (CORRECT)

**File**: `test/infrastructure/workflow_seeding.go`

**Key Functions**:
1. `SeedWorkflowsInDataStorage(client, workflows, testSuiteName, output)` - Main seeding function
2. `RegisterWorkflowInDataStorage(client, workflow, output)` - Single workflow registration

**Pattern**: 
- Accepts `[]infrastructure.TestWorkflow` (shared type)
- Uses OpenAPI client with DD-AUTH-014 authentication
- Returns `map[string]string` (workflow_name:environment → UUID)
- Proper error handling and logging

**Status**: ✅ **CORRECT** - This is the authoritative implementation

---

### 🔄 AIAnalysis Workflow Code

#### File 1: `test/integration/aianalysis/test_workflows.go`

**Functions**:
- `GetAIAnalysisTestWorkflows()` - Returns 6 workflows × 3 environments = 18 total
- `SeedTestWorkflowsInDataStorage()` - **Wrapper that delegates to shared library** ✅

**Pattern**:
```go
func SeedTestWorkflowsInDataStorage(client *ogenclient.Client, output io.Writer) (map[string]string, error) {
    workflows := GetAIAnalysisTestWorkflows()
    sharedWorkflows := make([]infrastructure.TestWorkflow, len(workflows))
    for i, wf := range workflows {
        sharedWorkflows[i] = infrastructure.TestWorkflow{...}  // Convert
    }
    return infrastructure.SeedWorkflowsInDataStorage(client, sharedWorkflows, "AIAnalysis Integration", output)
}
```

**Status**: ✅ **CORRECT** - Properly delegates to shared library

---

#### File 2: `test/infrastructure/aianalysis_e2e.go` (lines 286-310)

**Pattern**:
```go
// INLINE workflow definitions (NOT using GetAIAnalysisTestWorkflows!)
testWorkflows := []TestWorkflow{
    {WorkflowID: "oomkill-increase-memory-v1", ...},
    {WorkflowID: "crashloop-config-fix-v1", ...},
    // ... 10+ inline definitions
}

workflowUUIDs, err := SeedWorkflowsInDataStorage(seedClient, testWorkflows, "AIAnalysis E2E (via infrastructure)", writer)
```

**Status**: ⚠️ **CODE DUPLICATION** - Inline definitions duplicate `GetAIAnalysisTestWorkflows()`

**Comment on line 283**:
```go
// Note: GetAIAnalysisTestWorkflows() is still defined in test/integration/aianalysis/test_workflows.go
// We need to convert those workflows to the shared infrastructure.TestWorkflow type
// For now, inline the workflow definitions temporarily (TODO: refactor GetAIAnalysisTestWorkflows)
```

**Issue**: This TODO was never completed. Should call:
```go
workflows := aianalysis.GetAIAnalysisTestWorkflows()
sharedWorkflows := make([]infrastructure.TestWorkflow, len(workflows))
// Convert to infrastructure.TestWorkflow...
workflowUUIDs, err := infrastructure.SeedWorkflowsInDataStorage(seedClient, sharedWorkflows, "AIAnalysis E2E", writer)
```

---

### 🔄 HAPI Workflow Code

#### File 1: `test/e2e/holmesgpt-api/test_workflows.go`

**Functions**:
- `GetHAPIE2ETestWorkflows()` - Returns 5 workflows (1 environment each)
- `SeedTestWorkflowsInDataStorage()` - **Wrapper that delegates to shared library** ✅

**Pattern**:
```go
func SeedTestWorkflowsInDataStorage(client *ogenclient.Client, output io.Writer) (map[string]string, error) {
    workflows := GetHAPIE2ETestWorkflows()
    sharedWorkflows := make([]infrastructure.TestWorkflow, len(workflows))
    for i, wf := range workflows {
        sharedWorkflows[i] = infrastructure.TestWorkflow{...}  // Convert
    }
    return infrastructure.SeedWorkflowsInDataStorage(client, sharedWorkflows, "HAPI E2E", output)
}
```

**Status**: ✅ **CORRECT** - Properly delegates to shared library

---

#### File 2: `test/integration/holmesgptapi/workflow_seeding.go`

**Functions**:
- `GetHAPITestWorkflows()` - Returns 23 workflows
- `SeedTestWorkflowsInDataStorage()` - **OLD IMPLEMENTATION** ❌
- `registerWorkflowInDataStorage()` - **OLD IMPLEMENTATION** ❌

**Pattern**:
```go
func SeedTestWorkflowsInDataStorage(client *ogenclient.Client, output io.Writer) (map[string]string, error) {
    workflows := GetHAPITestWorkflows()
    workflowUUIDs := make(map[string]string)
    
    for _, wf := range workflows {
        workflowID, err := registerWorkflowInDataStorage(client, wf, output)  // ❌ OLD function
        // ... custom logic ...
    }
    return workflowUUIDs, nil
}

func registerWorkflowInDataStorage(client *ogenclient.Client, wf HAPIWorkflowFixture, output io.Writer) (string, error) {
    // ... 100+ lines of custom logic ...
    // ❌ Does NOT use infrastructure.RegisterWorkflowInDataStorage()
}
```

**Status**: ❌ **OLD CODE - NOT REFACTORED**

**Issue**: This file contains ~200 lines of duplicate registration logic that should use the shared library

---

## 🚨 Identified Issues

### Issue #1: AIAnalysis Infrastructure Uses Inline Definitions
**Location**: `test/infrastructure/aianalysis_e2e.go:286-304`  
**Problem**: 10+ inline workflow definitions duplicating `GetAIAnalysisTestWorkflows()`  
**Impact**: Code duplication, maintenance burden

**Fix**:
```go
// BEFORE: Inline definitions
testWorkflows := []TestWorkflow{
    {WorkflowID: "oomkill-increase-memory-v1", ...},
    // ... 10+ more lines
}

// AFTER: Use wrapper
workflowUUIDs, err := aianalysis.SeedTestWorkflowsInDataStorage(seedClient, writer)
```

**Risk**: Low (AIAnalysis E2E tests currently passing)

---

### Issue #2: HAPI Integration Tests Use OLD Implementation
**Location**: `test/integration/holmesgptapi/workflow_seeding.go`  
**Problem**: Entire file (~200 lines) contains OLD workflow registration logic  
**Impact**: Code duplication (~100 lines duplicate shared library logic)

**Files to Update**:
```go
// OLD file (should be DELETED after refactoring):
test/integration/holmesgptapi/workflow_seeding.go

// Should use instead:
import "github.com/jordigilh/kubernaut/test/infrastructure"

// Then call:
workflows := GetHAPITestWorkflows()
sharedWorkflows := convertToInfrastructureWorkflows(workflows)
workflowUUIDs, err := infrastructure.SeedWorkflowsInDataStorage(client, sharedWorkflows, "HAPI Integration", output)
```

**Risk**: **HIGH** - HAPI integration tests may be using this OLD code and could break if not migrated properly

---

### Issue #3: Parameter Order Inconsistency Fixed ✅
**Location**: `test/infrastructure/aianalysis_e2e.go:265`  
**Problem**: `GetServiceAccountToken()` called with wrong parameter order  
**Fix Applied**: Changed from `(ctx, kubeconfigPath, namespace, saName)` to `(ctx, namespace, saName, kubeconfigPath)`

**Before**:
```go
saToken, err := GetServiceAccountToken(context.Background(), kubeconfigPath, namespace, "aianalysis-e2e-sa")
```

**After**:
```go
saToken, err := GetServiceAccountToken(context.Background(), namespace, "aianalysis-e2e-sa", kubeconfigPath)
```

**Status**: ✅ FIXED (this was likely the cause of AIAnalysis E2E failure)

---

## 📋 Refactoring Status Matrix

| File | Location | Type | Uses Shared Library? | Status |
|------|----------|------|---------------------|--------|
| `workflow_seeding.go` | `test/infrastructure/` | Shared library | N/A | ✅ Authoritative |
| `test_workflows.go` | `test/integration/aianalysis/` | Wrapper | ✅ YES | ✅ Correct |
| `aianalysis_e2e.go` | `test/infrastructure/` | E2E setup | ❌ Inline defs | ⚠️ Duplication |
| `test_workflows.go` | `test/e2e/holmesgpt-api/` | Wrapper | ✅ YES | ✅ Correct |
| `workflow_seeding.go` | `test/integration/holmesgptapi/` | OLD integration | ❌ NO | ❌ Not refactored |

---

## 🔧 Recommended Refactoring Steps

### Step 1: Refactor AIAnalysis Infrastructure (Low Risk)

**File**: `test/infrastructure/aianalysis_e2e.go:286-310`

**Change**:
```go
// BEFORE: Inline definitions
testWorkflows := []infrastructure.TestWorkflow{
    {WorkflowID: "oomkill-increase-memory-v1", ...},
    // ... 10+ more inline definitions
}
workflowUUIDs, err := SeedWorkflowsInDataStorage(seedClient, testWorkflows, "AIAnalysis E2E (via infrastructure)", writer)

// AFTER: Use existing wrapper from test/integration/aianalysis
// Import at top: aianalysis "github.com/jordigilh/kubernaut/test/integration/aianalysis"
workflowUUIDs, err := aianalysis.SeedTestWorkflowsInDataStorage(seedClient, writer)
```

**Benefits**:
- Eliminates 20+ lines of duplication
- Single source of truth for AIAnalysis workflows
- Consistent with HAPI E2E pattern

**Risk**: Low (wrapper already tested and working)

---

### Step 2: Refactor HAPI Integration Tests (High Risk - Requires Testing)

**File**: `test/integration/holmesgptapi/workflow_seeding.go` (DELETE after refactoring)

**Change**:
1. Move `GetHAPITestWorkflows()` to `test/e2e/holmesgpt-api/test_workflows.go` (if needed)
2. Delete `SeedTestWorkflowsInDataStorage()` (OLD implementation)
3. Delete `registerWorkflowInDataStorage()` (OLD implementation)
4. Update all callers to use wrapper from `test/e2e/holmesgpt-api/test_workflows.go`

**Before**:
```go
// test/integration/holmesgptapi/workflow_seeding.go
func SeedTestWorkflowsInDataStorage(client *ogenclient.Client, output io.Writer) (map[string]string, error) {
    workflows := GetHAPITestWorkflows()
    // ... 50+ lines of custom logic ...
    workflowID, err := registerWorkflowInDataStorage(client, wf, output)  // ❌ OLD
}
```

**After**:
```go
// Use wrapper from test/e2e/holmesgpt-api/test_workflows.go
import hapitest "github.com/jordigilh/kubernaut/test/e2e/holmesgpt-api"

workflowUUIDs, err := hapitest.SeedTestWorkflowsInDataStorage(client, output)
```

**Benefits**:
- Eliminates ~200 lines of duplicate code
- Single source of truth for workflow registration
- Consistent error handling and logging
- Easier maintenance

**Risk**: **HIGH** - Integration tests depend on this code, must verify all callers

---

### Step 3: Check for Old Callers

**Search for**:
```bash
grep -r "holmesgptapi.SeedTestWorkflowsInDataStorage" test/
# vs
grep -r "test/e2e/holmesgpt-api.*SeedTestWorkflowsInDataStorage" test/
```

**Verify**:
- All HAPI E2E tests use `test/e2e/holmesgpt-api/test_workflows.go` (NEW)
- No integration tests still use `test/integration/holmesgptapi/workflow_seeding.go` (OLD)

---

## 🎓 Root Cause Analysis: Why Multiple Implementations Exist

### Timeline (Reconstructed)

1. **Original**: Each test suite had its own workflow seeding code
   - AIAnalysis: `test/integration/aianalysis/test_workflows.go`
   - HAPI Integration: `test/integration/holmesgptapi/workflow_seeding.go`
   - HAPI E2E: No file (used integration code)

2. **Refactoring Phase 1**: Created shared library
   - Created: `test/infrastructure/workflow_seeding.go`
   - Pattern: `SeedWorkflowsInDataStorage()` + `RegisterWorkflowInDataStorage()`

3. **Refactoring Phase 2**: Created wrappers
   - AIAnalysis: Updated `test/integration/aianalysis/test_workflows.go` to use shared library ✅
   - HAPI E2E: Created NEW `test/e2e/holmesgpt-api/test_workflows.go` to use shared library ✅
   - HAPI Integration: **NOT UPDATED** ❌

4. **Incomplete Step**: AIAnalysis E2E infrastructure
   - Created inline definitions instead of using wrapper
   - Added TODO comment (never completed)

---

## 🚨 Current Bugs and Risks

### Bug #1: AIAnalysis E2E Parameter Order (FIXED ✅)
**Impact**: Caused AIAnalysis E2E tests to fail  
**Root Cause**: `GetServiceAccountToken()` called with wrong parameter order  
**Fix**: Corrected to `(ctx, namespace, saName, kubeconfigPath)`

### Bug #2: Code Duplication (Maintenance Risk)
**Impact**: Changes to workflows must be made in 3 places  
**Locations**:
1. `test/integration/aianalysis/test_workflows.go:57-138` (18 workflows)
2. `test/infrastructure/aianalysis_e2e.go:286-304` (18 workflows DUPLICATED)
3. `test/integration/holmesgptapi/workflow_seeding.go` (~200 lines OLD logic)

**Risk**: High - easy to update one location and forget others

### Bug #3: Type Mismatches
**`test/integration/aianalysis/test_workflows.go`**:
```go
type TestWorkflow struct {
    WorkflowID  string
    Name        string
    // ... 8 fields (no ContainerImage)
}
```

**`test/e2e/holmesgpt-api/test_workflows.go`**:
```go
type TestWorkflow struct {
    WorkflowID     string
    Name           string
    // ... 8 fields
    ContainerImage string  // ← Added for HAPI E2E
}
```

**`test/infrastructure/workflow_seeding.go`**:
```go
type TestWorkflow struct {
    WorkflowID     string
    Name           string
    // ... 8 fields
    ContainerImage string  // ← Shared standard
}
```

**Issue**: AIAnalysis has its own local `TestWorkflow` type that's incompatible with shared type  
**Impact**: Requires conversion in wrapper function (works but adds complexity)

---

## ✅ What Works Correctly

### HAPI E2E Suite
**File**: `test/e2e/holmesgpt-api/holmesgpt_api_e2e_suite_test.go:223`

```go
workflowUUIDs, err := SeedTestWorkflowsInDataStorage(seedClient, GinkgoWriter)
```

**Flow**:
1. Calls wrapper in `test/e2e/holmesgpt-api/test_workflows.go`
2. Wrapper calls `GetHAPIE2ETestWorkflows()` (5 workflows)
3. Wrapper converts to `infrastructure.TestWorkflow`
4. Delegates to `infrastructure.SeedWorkflowsInDataStorage()` ✅

**Status**: ✅ Fully refactored, working correctly

---

### AIAnalysis Integration Tests
**File**: `test/integration/aianalysis/*_integration_test.go`

**Flow**:
1. Calls `aianalysis.SeedTestWorkflowsInDataStorage(client, output)`
2. Wrapper calls `GetAIAnalysisTestWorkflows()` (18 workflows)
3. Wrapper converts to `infrastructure.TestWorkflow`
4. Delegates to `infrastructure.SeedWorkflowsInDataStorage()` ✅

**Status**: ✅ Fully refactored, working correctly

---

## 🚀 Refactoring Recommendations

### Priority 1: Fix AIAnalysis E2E Infrastructure (Low Risk)

**File**: `test/infrastructure/aianalysis_e2e.go:260-310`

**Change**:
```go
// Add import at top
import aianalysistest "github.com/jordigilh/kubernaut/test/integration/aianalysis"

// Replace lines 263-310 with:
_, _ = fmt.Fprintln(writer, "  🔐 Creating authenticated DataStorage client for workflow seeding...")

// Get ServiceAccount token for authentication
saToken, err := GetServiceAccountToken(context.Background(), namespace, "aianalysis-e2e-sa", kubeconfigPath)
if err != nil {
    return fmt.Errorf("failed to get ServiceAccount token: %w", err)
}

// Create authenticated OpenAPI client for DataStorage
seedClient, err := ogenclient.NewClient(
    dataStorageURL,
    ogenclient.WithClient(&http.Client{
        Transport: testauth.NewServiceAccountTransport(saToken),
        Timeout:   30 * time.Second,
    }),
)
if err != nil {
    return fmt.Errorf("failed to create DataStorage client: %w", err)
}

// Use wrapper from test/integration/aianalysis
workflowUUIDs, err := aianalysistest.SeedTestWorkflowsInDataStorage(seedClient, writer)
if err != nil {
    return fmt.Errorf("failed to seed test workflows: %w", err)
}
_, _ = fmt.Fprintf(writer, "  ✅ Seeded %d workflows in DataStorage\n", len(workflowUUIDs))
```

**Lines Removed**: ~50 (inline workflow definitions)  
**Lines Added**: ~3 (wrapper call + import)  
**Test Impact**: None (same workflows, same logic)

---

### Priority 2: Refactor HAPI Integration Tests (High Risk - Requires Validation)

**Files to Modify**:
1. `test/integration/holmesgptapi/workflow_seeding.go` - DELETE or gut most functions
2. Update all callers in `test/integration/holmesgptapi/*_test.go`

**Verification Required**:
```bash
# Find all callers
grep -r "holmesgptapi.SeedTestWorkflowsInDataStorage" test/integration/

# Check if any callers depend on OLD implementation behavior
```

**Risk**: HIGH - Must verify all HAPI integration tests pass after refactoring

---

### Priority 3: Consolidate TestWorkflow Types (Future Enhancement)

**Current**: 3 local `TestWorkflow` types
- `test/integration/aianalysis/test_workflows.go`
- `test/e2e/holmesgpt-api/test_workflows.go`
- `test/integration/holmesgptapi/workflow_seeding.go` (OLD)

**Goal**: Single type in `test/infrastructure/workflow_seeding.go`

**Benefits**:
- No conversion needed in wrappers
- Type safety across all test suites
- Simpler code

**Risk**: Medium (affects multiple packages, requires careful migration)

---

## 📊 Code Duplication Summary

| Code Block | Lines | Duplicated In | Fix Priority |
|------------|-------|---------------|--------------|
| Workflow definitions | ~30 | AIAnalysis infrastructure (inline) | P1 - Low risk |
| Registration logic | ~100 | HAPI integration (OLD impl) | P2 - High risk |
| Type conversion | ~10 | Each wrapper | P3 - Future |

**Total Duplication**: ~140 lines that could be eliminated

---

## ✅ Immediate Actions

### 1. Fix AIAnalysis E2E Infrastructure (DO NOW)
- **Risk**: Low
- **Impact**: Eliminates 50 lines of duplication
- **Test**: `make test-e2e-aianalysis` should still pass
- **Time**: 5 minutes

### 2. Validate HAPI Integration Test Dependencies (DO BEFORE REFACTORING)
```bash
# Check if integration tests use OLD workflow_seeding.go
grep -r "import.*holmesgptapi" test/integration/ | grep -v test/integration/holmesgptapi

# If NO external callers, can safely refactor
# If YES external callers, must migrate them first
```

### 3. Run ALL Tests After Refactoring
```bash
make test-integration-holmesgptapi  # Verify HAPI integration tests
make test-e2e-aianalysis             # Verify AIAnalysis E2E
make test-e2e-holmesgpt-api          # Verify HAPI E2E
```

---

## 🔗 Related Documentation

- **DD-TEST-011 v2.0**: Go-Based Workflow Seeding Standard
- **DD-AUTH-014**: DataStorage SubjectAccessReview Authentication
- **DD-WORKFLOW-002 v3.0**: DataStorage UUID Generation

---

**Triage Complete**: February 2, 2026  
**Status**: ⚠️ **PARTIAL REFACTORING - 2 ISSUES IDENTIFIED**  
**Priority 1 Fix**: Remove inline definitions from AIAnalysis infrastructure  
**Priority 2 Fix**: Migrate HAPI integration tests to shared library  
**Immediate Action**: Fix AIAnalysis E2E GetServiceAccountToken parameter order ✅ DONE
