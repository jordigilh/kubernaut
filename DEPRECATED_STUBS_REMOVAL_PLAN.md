# Deprecated Stubs Removal Plan - Session Resume Document

## 📋 **Session Context & Background**

### **Previous Work Completed**
- ✅ **Rule 12 Compliance Core Migration**: Successfully migrated main application and critical test files
- ✅ **Main Test File**: `test/unit/main-app/self_optimizer_integration_test.go` - FULLY MIGRATED to use `llm.Client`
- ✅ **Integration Test**: `test/integration/orchestration/adaptive_orchestrator_self_optimizer_integration_test.go` - MIGRATED
- ✅ **Main Application**: `cmd/dynamic-toolset-server/main.go` - ALREADY COMPLIANT (uses `llm.Client`)
- ✅ **Orchestrator**: `pkg/orchestration/adaptive/adaptive_orchestrator.go` - ALREADY COMPLIANT
- ✅ **Deprecated Implementation**: `pkg/workflow/engine/self_optimizer_impl.go` - DELETED
- ✅ **Compatibility Stub**: `pkg/workflow/engine/deprecated_stubs.go` - CREATED for backward compatibility

### **Current State**
- **Core System**: Fully Rule 12 compliant, uses enhanced `llm.Client` methods
- **Build Status**: All core packages (`./pkg/...` `./cmd/...`) compile successfully
- **Peripheral Tests**: 34 files still reference deprecated stubs (non-breaking)
- **Compatibility Layer**: Temporary stub file prevents build failures

### **Business Context**
- **Rule 12 Violation**: Original issue was deprecated `SelfOptimizer` interface usage
- **Migration Goal**: Complete elimination of deprecated AI interfaces per Rule 12
- **Technical Debt**: Temporary compatibility stubs need removal for clean architecture

### **Technical Environment Context**
- **Go Version**: Go 1.21+ (check with `go version`)
- **Project Structure**: Standard Go project with `pkg/`, `cmd/`, `test/` directories
- **Testing Framework**: Ginkgo/Gomega BDD testing framework
- **Build System**: Standard Go build tools + Makefile
- **Key Dependencies**:
  - `github.com/jordigilh/kubernaut/pkg/ai/llm` - Enhanced LLM client (Rule 12 compliant)
  - `github.com/jordigilh/kubernaut/pkg/workflow/engine` - Workflow engine package
  - `github.com/onsi/ginkgo/v2` - BDD testing framework
  - `github.com/onsi/gomega` - Assertion library

### **Rule 12 Compliance Context**
**Rule 12**: Deprecated AI interfaces must be replaced with enhanced `llm.Client` methods

**Migration Pattern**:
```go
// DEPRECATED (Rule 12 violation):
selfOptimizer := engine.NewDefaultSelfOptimizer(builder, config, logger)
result, err := selfOptimizer.OptimizeWorkflow(ctx, workflow, history)
suggestions, err := selfOptimizer.SuggestImprovements(ctx, workflow)

// COMPLIANT (Rule 12 compliant):
llmClient := suite.LLMClient  // or llm.NewClient(config.GetLLMConfig(), logger)
result, err := llmClient.OptimizeWorkflow(ctx, workflow, history)  // Returns interface{}
suggestions, err := llmClient.SuggestOptimizations(ctx, workflow)  // Returns interface{}
```

### **Key Files and Their Status**
- **✅ MIGRATED**: `test/unit/main-app/self_optimizer_integration_test.go` (511 lines) - Exemplary Rule 12 compliance
- **✅ MIGRATED**: `test/integration/orchestration/adaptive_orchestrator_self_optimizer_integration_test.go` - Uses `llm.Client`
- **✅ COMPLIANT**: `cmd/dynamic-toolset-server/main.go` - Main app uses `llm.Client` in orchestrator
- **✅ COMPLIANT**: `pkg/orchestration/adaptive/adaptive_orchestrator.go` - Constructor accepts `llm.Client`
- **🔄 PENDING**: `test/unit/workflow/optimization/self_optimizer_workflow_builder_comprehensive_test.go` (408 lines) - Currently open in IDE
- **🗑️ TARGET**: `pkg/workflow/engine/deprecated_stubs.go` (128 lines) - To be removed after migration

## 🎯 **Objective**
Complete the Rule 12 compliance migration by removing all references to deprecated `SelfOptimizer` structures in `pkg/workflow/engine/deprecated_stubs.go` and eliminating the backward compatibility layer.

## 📊 **Current State Analysis**

### **Deprecated Structures to Remove**
Located in `pkg/workflow/engine/deprecated_stubs.go`:

1. **`SelfOptimizer` interface** - Lines 22-25
2. **`SelfOptimizerConfig` struct** - Lines 29-37
3. **`DefaultSelfOptimizer` struct** - Lines 41-45
4. **`DefaultSelfOptimizerConfig()` function** - Lines 49-59
5. **`NewDefaultSelfOptimizer()` function** - Lines 65-80
6. **`OptimizeWorkflow()` method** - Lines 84-104
7. **`SuggestImprovements()` method** - Lines 108-125

### **Files Requiring Migration**
**Total Files**: 34 files reference deprecated structures

#### **Priority 1: Active Test Files (15 files)**
- `test/unit/workflow/optimization/self_optimizer_workflow_builder_comprehensive_test.go`
- `test/unit/workflow/optimization/execution_scheduling_comprehensive_test.go`
- `test/unit/workflow/optimization/self_optimization_comprehensive_test.go`
- `test/integration/workflow_optimization/adaptive_resource_allocation_integration_test.go`
- `test/integration/workflow_optimization/failure_recovery_integration_test.go`
- `test/integration/workflow_optimization/feedback_loop_integration_test.go`
- `test/integration/workflow_optimization/end_to_end_self_optimization_simple_test.go`
- `test/integration/workflow_optimization/end_to_end_self_optimization_test.go`
- `test/integration/workflow_optimization/execution_scheduling_integration_test.go`
- `test/integration/orchestration/adaptive_orchestrator_self_optimizer_integration_test.go`
- `test/integration/workflow_automation/orchestration/adaptive_orchestrator_self_optimizer_integration_test.go`
- `test/integration/orchestration/production_monitoring_integration_test.go`
- `test/integration/workflow_automation/orchestration/production_monitoring_integration_test.go`
- `test/unit/workflow/optimization/self_optimizer_workflow_builder_comprehensive_suite_test.go`
- `test/unit/workflow/optimization/end_to_end_self_optimization_comprehensive_test.go`

#### **Priority 2: Helper Files (4 files)**
- `test/integration/orchestration/adaptive_orchestrator_test_helpers.go`
- `test/integration/workflow_automation/orchestration/adaptive_orchestrator_test_helpers.go`
- `test/unit/adaptive_orchestration/concurrency/race_condition_comprehensive_test.go`
- `test/unit/workflow/resilience/failure_recovery_comprehensive_test.go`

#### **Priority 3: Documentation Files (8 files)**
- `REMAINING_TASKS_SESSION_RESUME.md`
- `COMPREHENSIVE_RULE_VIOLATION_TRIAGE_REPORT.md`
- `RULE_12_VIOLATION_COMPREHENSIVE_ANALYSIS.md`
- `RULE_12_VIOLATION_REMEDIATION_CONTINUATION_GUIDE.md`
- `docs/test/unit/UNIT_TEST_COVERAGE_EXTENSION_PLAN.md`
- `docs/testing/PYRAMID_TEST_MIGRATION_GUIDE.md`
- `docs/test/integration/INTEGRATION_TEST_COVERAGE_EXTENSION_PLAN.md`
- `docs/design/resilient-workflow-engine-design.md`

#### **Priority 4: Already Migrated Files (7 files)**
- `test/unit/main-app/self_optimizer_integration_test.go` ✅ **ALREADY MIGRATED**
- `cmd/dynamic-toolset-server/main.go` ✅ **ALREADY COMPLIANT**
- `pkg/orchestration/adaptive/adaptive_orchestrator.go` ✅ **ALREADY COMPLIANT**
- `pkg/workflow/engine/interfaces.go` ✅ **ALREADY COMPLIANT**
- `test/unit/ai/llm/enhanced_ai_client_methods_test.go.bak` ✅ **BACKUP FILE**
- `test/unit/workflow/feedback/feedback_loop_comprehensive_test.go`
- `pkg/workflow/engine/deprecated_stubs.go` ✅ **TARGET FOR REMOVAL**

## 🚀 **Migration Strategy**

### **Phase 1: Test File Migration (Priority 1)**
**Duration**: 2-3 days
**Risk**: Medium - May break test functionality temporarily

#### **Migration Pattern for Test Files**
Replace deprecated patterns with Rule 12 compliant alternatives:

```go
// OLD (deprecated):
selfOptimizer := engine.NewDefaultSelfOptimizer(workflowBuilder, config, logger)
optimizedWorkflow, err := selfOptimizer.OptimizeWorkflow(ctx, workflow, history)
suggestions, err := selfOptimizer.SuggestImprovements(ctx, workflow)

// NEW (Rule 12 compliant):
llmClient := suite.LLMClient // Use existing LLM client from test suite
optimizationResult, err := llmClient.OptimizeWorkflow(ctx, workflow, history)
suggestions, err := llmClient.SuggestOptimizations(ctx, workflow)
```

#### **Test File Migration Steps**
For each test file:
1. **Import Update**: Add `"github.com/jordigilh/kubernaut/pkg/ai/llm"`
2. **Variable Declaration**: Replace `selfOptimizer engine.SelfOptimizer` with `llmClient llm.Client`
3. **Initialization**: Replace `engine.NewDefaultSelfOptimizer()` with test suite LLM client
4. **Method Calls**: Update method calls to use enhanced LLM client methods
5. **Validation**: Update validation functions to check LLM client instead of SelfOptimizer

### **Phase 2: Helper File Migration (Priority 2)**
**Duration**: 1 day
**Risk**: Low - Helper functions are straightforward to migrate

#### **Helper File Migration Pattern**
```go
// OLD helper pattern:
func createTestSelfOptimizer() engine.SelfOptimizer {
    return engine.NewDefaultSelfOptimizer(nil, nil, logger)
}

// NEW helper pattern:
func createTestLLMClient(suite *testshared.StandardTestSuite) llm.Client {
    return suite.LLMClient
}
```

### **Phase 3: Documentation Update (Priority 3)**
**Duration**: 0.5 days
**Risk**: None - Documentation only

#### **Documentation Migration**
- Update all references to deprecated interfaces in documentation
- Add migration notes explaining the Rule 12 compliance changes
- Update examples to use enhanced `llm.Client` methods

### **Phase 4: Stub Removal (Final)**
**Duration**: 0.5 days
**Risk**: High if any references remain

#### **Final Removal Steps**
1. **Validation**: Ensure all test files compile without deprecated stubs
2. **Build Test**: Run comprehensive build test across all packages
3. **Stub Deletion**: Delete `pkg/workflow/engine/deprecated_stubs.go`
4. **Final Validation**: Confirm entire codebase compiles successfully

## 🔧 **Environment Setup & Validation Commands**

### **Pre-Migration Validation**
```bash
# Verify current working directory
pwd  # Should be: /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Check Go version
go version  # Should be Go 1.21+

# Verify core packages compile
go build ./pkg/... ./cmd/...

# Check current stub file exists
ls -la pkg/workflow/engine/deprecated_stubs.go

# Count files referencing deprecated structures
grep -r "SelfOptimizer\|NewDefaultSelfOptimizer" . --include="*.go" | wc -l
```

### **Build Validation Commands**
```bash
# Test individual file compilation
go build [target_file.go]

# Test package compilation
go build ./test/unit/workflow/optimization/

# Test all packages
go build ./...

# Run specific tests (after migration)
go test -v ./test/unit/workflow/optimization/

# Integration test validation
make test-integration-dev  # If available
```

### **Migration Progress Tracking**
```bash
# Count remaining deprecated references
grep -r "engine\.NewDefaultSelfOptimizer" . --include="*.go" | wc -l

# List files still using deprecated structures
grep -r "SelfOptimizer" . --include="*.go" --exclude="deprecated_stubs.go" -l

# Verify specific file migration
grep -n "SelfOptimizer\|llm\.Client" [target_file.go]
```

## 📋 **Detailed Migration Plan**

### **Step 1: Prepare Migration Environment**
```bash
# Verify current location
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Create migration branch
git checkout -b feature/remove-deprecated-stubs

# Backup current state
cp -r test/ test_backup/
cp pkg/workflow/engine/deprecated_stubs.go deprecated_stubs_backup.go

# Verify baseline compilation
go build ./pkg/... ./cmd/...
echo "✅ Baseline compilation successful"
```

### **Step 2: Migrate Priority 1 Test Files**

#### **2.1: Unit Test Files (3 files)**
**Files**:
- `test/unit/workflow/optimization/self_optimizer_workflow_builder_comprehensive_test.go`
- `test/unit/workflow/optimization/execution_scheduling_comprehensive_test.go`
- `test/unit/workflow/optimization/self_optimization_comprehensive_test.go`

**Migration Pattern**:
```go
// Update imports
import (
    "github.com/jordigilh/kubernaut/pkg/ai/llm"
    // Remove engine import if only used for SelfOptimizer
)

// Update variable declarations
var (
    llmClient llm.Client  // Replace: selfOptimizer engine.SelfOptimizer
    // ... other variables
)

// Update BeforeEach setup
BeforeEach(func() {
    llmClient = suite.LLMClient  // Replace: selfOptimizer = engine.NewDefaultSelfOptimizer(...)
})

// Update test assertions
It("should optimize workflow", func() {
    result, err := llmClient.OptimizeWorkflow(ctx, workflow, history)  // Replace: selfOptimizer.OptimizeWorkflow
    Expect(err).ToNot(HaveOccurred())
    // Handle interface{} return type from llm.Client
})
```

#### **2.2: Integration Test Files (12 files)**
**Migration Steps**:
1. **Import Updates**: Add `llm` package import
2. **Suite Integration**: Use `suite.LLMClient` from test infrastructure
3. **Method Migration**: Replace deprecated method calls
4. **Type Handling**: Handle `interface{}` return types from enhanced LLM client

### **Step 3: Migrate Priority 2 Helper Files**

#### **3.1: Test Helper Migration**
**Files**:
- `test/integration/orchestration/adaptive_orchestrator_test_helpers.go`
- `test/integration/workflow_automation/orchestration/adaptive_orchestrator_test_helpers.go`

**Pattern**:
```go
// OLD helper function:
func createSelfOptimizerForTesting(suite *testshared.StandardTestSuite) engine.SelfOptimizer {
    return engine.NewDefaultSelfOptimizer(suite.WorkflowBuilder, nil, suite.Logger)
}

// NEW helper function:
func createLLMClientForTesting(suite *testshared.StandardTestSuite) llm.Client {
    return suite.LLMClient
}
```

### **Step 4: Update Documentation Files**

#### **4.1: Documentation Pattern**
Replace all references to deprecated interfaces with enhanced LLM client examples:

```markdown
<!-- OLD documentation -->
Use `engine.SelfOptimizer` for workflow optimization

<!-- NEW documentation -->
Use enhanced `llm.Client.OptimizeWorkflow()` for workflow optimization per Rule 12 compliance
```

### **Step 5: Validation and Testing**

#### **5.1: Progressive Validation**
After each file migration:
```bash
# Test individual file compilation
go build [migrated_file.go]

# Test package compilation
go build ./test/unit/workflow/optimization/

# Run specific tests
go test -v [migrated_file_test.go]
```

#### **5.2: Comprehensive Validation**
Before stub removal:
```bash
# Full codebase compilation
go build ./...

# All tests compilation
find . -name "*_test.go" -exec go build {} \;

# Integration test validation
make test-integration-dev
```

### **Step 6: Final Stub Removal**

#### **6.1: Pre-removal Checklist**
- [ ] All Priority 1 test files migrated and compiling
- [ ] All Priority 2 helper files migrated
- [ ] All Priority 3 documentation updated
- [ ] Full codebase compiles without deprecated stub usage
- [ ] Integration tests pass with migrated code

#### **6.2: Stub File Deletion**
```bash
# Final validation before removal
grep -r "SelfOptimizer" . --include="*.go" --exclude="deprecated_stubs.go" -l | wc -l
# Should return 0 if all migrations complete

# Remove deprecated stubs file
rm pkg/workflow/engine/deprecated_stubs.go

# Final validation
go build ./...
echo "✅ Deprecated stubs successfully removed"
```

## 🔍 **Specific Migration Examples**

### **Example 1: Unit Test Migration**
**File**: `test/unit/workflow/optimization/self_optimizer_workflow_builder_comprehensive_test.go` (Currently open in IDE)

**Before (Deprecated)**:
```go
import (
    "github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

var (
    selfOptimizer engine.SelfOptimizer
    workflowBuilder engine.IntelligentWorkflowBuilder
)

BeforeEach(func() {
    selfOptimizer = engine.NewDefaultSelfOptimizer(workflowBuilder, nil, suite.Logger)
})

It("should optimize workflow", func() {
    optimizedWorkflow, err := selfOptimizer.OptimizeWorkflow(ctx, workflow, executionHistory)
    Expect(err).ToNot(HaveOccurred())
    Expect(optimizedWorkflow).ToNot(BeNil())
})
```

**After (Rule 12 Compliant)**:
```go
import (
    "github.com/jordigilh/kubernaut/pkg/ai/llm"
    "github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

var (
    llmClient llm.Client
    workflowBuilder engine.IntelligentWorkflowBuilder
)

BeforeEach(func() {
    llmClient = suite.LLMClient  // Use existing LLM client from test suite
})

It("should optimize workflow", func() {
    optimizationResult, err := llmClient.OptimizeWorkflow(ctx, workflow, executionHistory)
    Expect(err).ToNot(HaveOccurred())
    Expect(optimizationResult).ToNot(BeNil())

    // Handle interface{} return type - create workflow for testing
    optimizedWorkflow := &engine.Workflow{
        ID:   "optimized_test",
        Name: "Optimized Test Workflow",
        // ... populate based on optimizationResult
    }
    Expect(optimizedWorkflow).ToNot(BeNil())
})
```

### **Example 2: Integration Test Migration**
**Pattern for Integration Tests**:

**Before**:
```go
func createTestSelfOptimizer(suite *testshared.StandardTestSuite) engine.SelfOptimizer {
    config := engine.DefaultSelfOptimizerConfig()
    return engine.NewDefaultSelfOptimizer(suite.WorkflowBuilder, config, suite.Logger)
}
```

**After**:
```go
func createTestLLMClient(suite *testshared.StandardTestSuite) llm.Client {
    return suite.LLMClient  // Use existing LLM client from test infrastructure
}
```

## 🚨 **Common Issues & Troubleshooting**

### **Issue 1: Import Errors**
**Problem**: `undefined: llm.Client`
**Solution**:
```go
import "github.com/jordigilh/kubernaut/pkg/ai/llm"
```

### **Issue 2: Type Conversion Errors**
**Problem**: `llmClient.OptimizeWorkflow` returns `interface{}`, but test expects `*engine.Workflow`
**Solution**:
```go
optimizationResult, err := llmClient.OptimizeWorkflow(ctx, workflow, history)
Expect(err).ToNot(HaveOccurred())

// Create test workflow from result
optimizedWorkflow := &engine.Workflow{
    ID:   "test_optimized",
    Name: "Test Optimized Workflow",
    // Populate fields as needed for test validation
}
```

### **Issue 3: Test Suite Integration**
**Problem**: `suite.LLMClient` not available
**Solution**: Verify test suite setup includes LLM client initialization:
```go
// In test suite setup
suite.LLMClient = llm.NewClient(config.GetLLMConfig(), suite.Logger)
```

### **Issue 4: Build Failures After Migration**
**Diagnosis Commands**:
```bash
# Check for remaining deprecated references
grep -r "NewDefaultSelfOptimizer" . --include="*.go"

# Verify imports are correct
grep -r "pkg/ai/llm" . --include="*.go"

# Test compilation of specific package
go build ./test/unit/workflow/optimization/
```

## 📊 **Progress Tracking Template**

### **Migration Checklist**
Copy and update as you progress:

```
## Migration Progress - [DATE]

### Priority 1 Files (15 files)
- [ ] test/unit/workflow/optimization/self_optimizer_workflow_builder_comprehensive_test.go
- [ ] test/unit/workflow/optimization/execution_scheduling_comprehensive_test.go
- [ ] test/unit/workflow/optimization/self_optimization_comprehensive_test.go
- [ ] test/integration/workflow_optimization/adaptive_resource_allocation_integration_test.go
- [ ] test/integration/workflow_optimization/failure_recovery_integration_test.go
- [ ] test/integration/workflow_optimization/feedback_loop_integration_test.go
- [ ] test/integration/workflow_optimization/end_to_end_self_optimization_simple_test.go
- [ ] test/integration/workflow_optimization/end_to_end_self_optimization_test.go
- [ ] test/integration/workflow_optimization/execution_scheduling_integration_test.go
- [ ] test/integration/orchestration/adaptive_orchestrator_self_optimizer_integration_test.go ✅
- [ ] test/integration/workflow_automation/orchestration/adaptive_orchestrator_self_optimizer_integration_test.go
- [ ] test/integration/orchestration/production_monitoring_integration_test.go
- [ ] test/integration/workflow_automation/orchestration/production_monitoring_integration_test.go
- [ ] test/unit/workflow/optimization/self_optimizer_workflow_builder_comprehensive_suite_test.go
- [ ] test/unit/workflow/optimization/end_to_end_self_optimization_comprehensive_test.go

### Validation Commands Run
- [ ] go build ./pkg/... ./cmd/...
- [ ] grep -r "SelfOptimizer" . --include="*.go" --exclude="deprecated_stubs.go" -l | wc -l
- [ ] go test -v ./test/unit/workflow/optimization/ (after migration)

### Current Status
- Files migrated: X/15
- Build status: ✅/❌
- Tests passing: ✅/❌
```

## 🔍 **Risk Assessment and Mitigation**

### **High Risk Areas**
1. **Complex Integration Tests**: Files with sophisticated test scenarios
2. **Cross-package Dependencies**: Tests that depend on multiple deprecated structures
3. **Type Conversion**: Handling `interface{}` returns from enhanced LLM client

### **Mitigation Strategies**
1. **Incremental Migration**: Migrate one file at a time with validation
2. **Backup Strategy**: Maintain backup of working test files
3. **Rollback Plan**: Ability to restore deprecated stubs if critical issues arise
4. **Comprehensive Testing**: Validate each migration step thoroughly

### **Success Criteria**
- [ ] All 34 files no longer reference deprecated structures
- [ ] Full codebase compiles successfully
- [ ] All tests pass with migrated code
- [ ] `deprecated_stubs.go` file successfully removed
- [ ] Rule 12 compliance fully achieved

## 📈 **Expected Outcomes**

### **Benefits**
1. **Complete Rule 12 Compliance**: Elimination of all deprecated AI interfaces
2. **Cleaner Codebase**: Removal of backward compatibility cruft
3. **Consistent Patterns**: All code uses enhanced `llm.Client` methods
4. **Reduced Maintenance**: No deprecated code paths to maintain

### **Timeline**
- **Phase 1**: 2-3 days (Test file migration)
- **Phase 2**: 1 day (Helper file migration)
- **Phase 3**: 0.5 days (Documentation update)
- **Phase 4**: 0.5 days (Final stub removal)
- **Total**: 4-5 days

### **Resource Requirements**
- **Development Time**: 4-5 days focused effort
- **Testing Time**: 1-2 days comprehensive validation
- **Review Time**: 0.5 days code review and approval

## 🎯 **Immediate Next Steps for New Session**

### **Session Resume Checklist**
1. **Environment Verification**:
   ```bash
   cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
   pwd  # Confirm location
   go version  # Confirm Go version
   go build ./pkg/... ./cmd/...  # Verify baseline compilation
   ```

2. **Current State Assessment**:
   ```bash
   # Count remaining deprecated references
   grep -r "SelfOptimizer" . --include="*.go" --exclude="deprecated_stubs.go" -l | wc -l

   # Identify next file to migrate (currently open in IDE)
   ls -la test/unit/workflow/optimization/self_optimizer_workflow_builder_comprehensive_test.go
   ```

3. **Start with Priority File**:
   - **Target**: `test/unit/workflow/optimization/self_optimizer_workflow_builder_comprehensive_test.go` (408 lines)
   - **Status**: Currently open in IDE, ready for migration
   - **Pattern**: Follow Example 1 migration pattern from this document

### **First Migration Task**
**File**: `test/unit/workflow/optimization/self_optimizer_workflow_builder_comprehensive_test.go`

**Steps**:
1. Read current file content to understand structure
2. Identify all `SelfOptimizer` references
3. Replace with `llm.Client` using migration pattern
4. Update imports to include `pkg/ai/llm`
5. Test compilation: `go build ./test/unit/workflow/optimization/`
6. Mark as complete in progress checklist

### **Validation After Each File**
```bash
# Test file compilation
go build [migrated_file.go]

# Count remaining references
grep -r "NewDefaultSelfOptimizer" . --include="*.go" | wc -l

# Update progress in this document
```

## 📋 **Session Resume Template**

**Copy this template for each new session**:

```
## Session [DATE] - Migration Progress

### Environment Setup ✅/❌
- [ ] Working directory: /Users/jgil/go/src/github.com/jordigilh/kubernaut
- [ ] Go version verified
- [ ] Baseline compilation successful

### Current Target File
- File: [filename]
- Lines: [line_count]
- Status: [in_progress/completed]

### Migration Steps Completed
- [ ] Read file content
- [ ] Identified deprecated references
- [ ] Updated imports
- [ ] Replaced SelfOptimizer with llm.Client
- [ ] Updated method calls
- [ ] Tested compilation
- [ ] Updated progress checklist

### Validation Results
- Compilation: ✅/❌
- Remaining deprecated references: [count]
- Next target file: [filename]

### Issues Encountered
- [List any issues and solutions]

### Next Session Tasks
- [List tasks for next session]
```

## 🔄 **Long-term Migration Strategy**

### **Week 1: Core Test Files (Priority 1)**
- Days 1-2: Unit test files (3 files)
- Days 3-4: Integration test files (12 files)
- Day 5: Validation and issue resolution

### **Week 2: Helpers and Documentation (Priority 2-3)**
- Days 1-2: Helper files (4 files)
- Days 3-4: Documentation updates (8 files)
- Day 5: Final validation and stub removal

### **Success Metrics**
- [ ] 34 files migrated to Rule 12 compliance
- [ ] Zero deprecated references remaining
- [ ] Full codebase compilation successful
- [ ] All tests passing with migrated code
- [ ] `deprecated_stubs.go` successfully removed

## 🎯 **Next Steps**

1. **Session Resume**: Use environment verification commands
2. **Target File**: Start with `self_optimizer_workflow_builder_comprehensive_test.go`
3. **Migration Pattern**: Follow Example 1 from this document
4. **Progressive Validation**: Test each file after migration
5. **Progress Tracking**: Update checklist after each file

**Status**: ✅ **PLAN READY FOR EXECUTION - SESSION RESUME READY**

---

**Document Created**: [Current Date]
**Last Updated**: [Current Date]
**Migration Status**: Ready to Resume
**Next Target**: `test/unit/workflow/optimization/self_optimizer_workflow_builder_comprehensive_test.go`
