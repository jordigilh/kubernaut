# ADR-033: BR Category Migration Plan

**Date**: November 5, 2025  
**Status**: 📋 Ready for Execution  
**Purpose**: Migrate BR-WORKFLOW-XXX to BR-REMEDIATION-XXX and establish BR-PLAYBOOK-XXX category

---

## 🎯 **MIGRATION OBJECTIVES**

### **Problem Statement**
1. **Naming Confusion**: BR-WORKFLOW-XXX historically referred to "Workflow Executor" service (now renamed to "RemediationExecutor")
2. **Category Conflict**: Playbook Catalog service needs its own BR category, not BR-WORKFLOW-XXX
3. **ADR-033 Clarity**: Multi-dimensional success tracking requires clear BR ownership across services

### **Solution**
1. **Migrate**: BR-WORKFLOW-XXX → **BR-REMEDIATION-XXX** (align with RemediationExecutor service)
2. **Create**: **BR-PLAYBOOK-XXX** category (for Playbook Catalog service)
3. **Document**: Cross-service BR ownership for ADR-033

---

## 📊 **BR CATEGORY MAPPING**

### **BEFORE (Incorrect)**
```
BR-WORKFLOW-XXX → Used for both:
  - Workflow execution/orchestration (RemediationExecutor)
  - Playbook catalog management (Playbook Catalog)
```

### **AFTER (Correct)**
```
BR-REMEDIATION-XXX → RemediationExecutor Service
  - Workflow execution, step orchestration
  - Populate incident_type, playbook_id in audits
  - AI execution mode tracking

BR-PLAYBOOK-XXX → Playbook Catalog Service
  - Playbook registry management
  - Versioning and deprecation
  - Playbook metadata API
```

---

## 📁 **FILES TO UPDATE (28 Files)**

### **Phase 1: Documentation** (10 files)

#### **High Priority** (User-facing docs)
1. `docs/DOCUMENTATION_INDEX.md` - Index references
2. `docs/architecture/WORKFLOW_ENGINE_ORCHESTRATION_ARCHITECTURE.md` - Architecture diagrams
3. `docs/architecture/decisions/ADR-001-crd-microservices-architecture.md` - Core ADR
4. `docs/architecture/decisions/ADR-022-v1-native-jobs-v2-tekton-migration.md` - Tekton migration
5. `docs/architecture/decisions/ADR-023-tekton-from-v1.md` - Tekton integration
6. `docs/architecture/decisions/ADR-024-eliminate-actionexecution-layer.md` - Action execution
7. `docs/architecture/decisions/ADR-004-fake-kubernetes-client.md` - Testing infrastructure
8. `docs/architecture/decisions/ADR-005-integration-test-coverage.md` - Test coverage
9. `docs/architecture/decisions/DD-006-controller-scaffolding-strategy.md` - Controller design
10. `docs/services/crd-controllers/archive/04-kubernetes-executor.md` - Archived service

#### **Implementation Plans**
11. `docs/services/crd-controllers/03-workflowexecution/implementation/EXPANSION_PLAN_TO_95_PERCENT.md`
12. `docs/services/crd-controllers/03-workflowexecution/implementation/phase0/DAY_02_EXPANDED.md`
13. `docs/services/crd-controllers/03-workflowexecution/implementation/phase0/DAY_03_EXPANDED.md`
14. `docs/services/stateless/context-api/implementation/QUALITY_AUDIT_VS_PHASE3.md`

---

### **Phase 2: Test Files** (14 files)

#### **E2E Tests**
1. `test/e2e/workflow_engine/workflow_engine_e2e_test.go`
2. `test/e2e/orchestration/workflow_orchestration_e2e_suite_test.go`

#### **Integration Tests**
3. `test/integration/workflow_pgvector/workflow_state_persistence_test.go`
4. `test/integration/workflow_automation/execution/intelligent_workflow_builder_suite_test.go`

#### **Unit Tests**
5. `test/unit/workflow/persistence/workflow_state_persistence_comprehensive_test.go`
6. `test/unit/workflow/persistence/workflow_state_persistence_comprehensive_suite_test.go`
7. `test/unit/workflow/actions/workflow_validation_enhanced_business_logic_suite_test.go`
8. `test/unit/workflow-engine/high_load_workflow_extensions_test.go`
9. `test/unit/api/workflow/workflow_client_test.go`
10. `test/unit/api/workflow/workflow_client_suite_test.go`

---

### **Phase 3: Production Code** (4 files)

1. `pkg/workflow/service.go`
2. `pkg/workflow/persistence/pgvector_persistence.go`
3. `pkg/intelligence/anomaly/anomaly_detector.go`
4. `pkg/api/workflow/client.go`

---

## 🔧 **MIGRATION STEPS**

### **Step 1: Create ADR-033 Cross-Service BR Document** ✅
**File**: `docs/architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md`

**Content**: Define BR ownership across all impacted services

---

### **Step 2: Update Documentation (Phase 1)** 📝
**Scope**: 10 documentation files

**Find/Replace Pattern**:
```bash
# Pattern 1: Inline BR references
OLD: BR-WORKFLOW-001
NEW: BR-REMEDIATION-001

# Pattern 2: BR range references
OLD: BR-WORKFLOW-001 through BR-WORKFLOW-031
NEW: BR-REMEDIATION-001 through BR-REMEDIATION-031

# Pattern 3: Descriptive text
OLD: "Workflow execution business requirements"
NEW: "Remediation execution business requirements"
```

**Manual Review Required**: 
- Architecture diagrams (verify service names updated)
- ADR decision rationale (ensure consistency)

---

### **Step 3: Update Test Files (Phase 2)** 🧪
**Scope**: 14 test files

**Find/Replace Pattern**:
```go
// Pattern 1: Ginkgo test labels
OLD: Label("BR-WORKFLOW-001", "BR-WORKFLOW-002")
NEW: Label("BR-REMEDIATION-001", "BR-REMEDIATION-002")

// Pattern 2: Test descriptions
OLD: Describe("BR-WORKFLOW-001: Basic Workflow Execution", func() {
NEW: Describe("BR-REMEDIATION-001: Basic Remediation Execution", func() {

// Pattern 3: Comments
OLD: // BR-WORKFLOW-001: Execute workflow steps
NEW: // BR-REMEDIATION-001: Execute remediation steps
```

**Validation**:
```bash
# Run tests after migration to ensure no breakage
go test ./test/... -v | grep "BR-REMEDIATION"
```

---

### **Step 4: Update Production Code (Phase 3)** 💻
**Scope**: 4 production code files

**Find/Replace Pattern**:
```go
// Pattern 1: Code comments
OLD: // BR-WORKFLOW-001: Workflow execution logic
NEW: // BR-REMEDIATION-001: Remediation execution logic

// Pattern 2: Struct tags
OLD: `json:"workflow_execution" db:"workflow_execution" br:"BR-WORKFLOW-001"`
NEW: `json:"remediation_execution" db:"remediation_execution" br:"BR-REMEDIATION-001"`
```

**Validation**:
```bash
# Build production code to ensure no compilation errors
go build ./pkg/...
```

---

### **Step 5: Update V5.0 Implementation Plan** 📄
**File**: `docs/services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V5.0.md`

**Changes**:
1. Update BR references in test scenarios (lines 1100-1300)
2. Update BR coverage matrix (lines 3189-3211)
3. Update ADR-033 cross-service references

---

### **Step 6: Create BR-PLAYBOOK Category** 🆕
**File**: `docs/architecture/business-requirements/BR-PLAYBOOK-CATEGORY.md`

**Define**:
- BR-PLAYBOOK-001: Playbook registry management
- BR-PLAYBOOK-002: Playbook versioning and deprecation
- BR-PLAYBOOK-003: Playbook metadata API

---

## ✅ **VALIDATION CHECKLIST**

### **Documentation Validation**
- [ ] All ADRs updated with BR-REMEDIATION-XXX
- [ ] Architecture diagrams reflect RemediationExecutor naming
- [ ] No orphaned BR-WORKFLOW-XXX references remain

### **Test Validation**
- [ ] All unit tests pass: `go test ./test/unit/... -v`
- [ ] All integration tests pass: `go test ./test/integration/... -v`
- [ ] All E2E tests pass: `go test ./test/e2e/... -v`
- [ ] Test labels updated: `grep -r "BR-WORKFLOW" test/ | wc -l` → 0 results

### **Production Code Validation**
- [ ] Code compiles: `go build ./...`
- [ ] No BR-WORKFLOW references in production code: `grep -r "BR-WORKFLOW" pkg/ | wc -l` → 0 results
- [ ] Code comments updated appropriately

### **ADR-033 Validation**
- [ ] Cross-service BR document created
- [ ] All 5 services have clear BR ownership
- [ ] Implementation plan updated with correct BR categories

---

## 📊 **MIGRATION IMPACT ANALYSIS**

| Area | Files Affected | Estimated Effort | Risk Level |
|---|---|---|---|
| **Documentation** | 10 files | 2-3 hours | 🟢 Low |
| **Test Files** | 14 files | 3-4 hours | 🟡 Medium |
| **Production Code** | 4 files | 1-2 hours | 🟡 Medium |
| **New BR Category** | 1 file | 1 hour | 🟢 Low |
| **Validation** | All | 2-3 hours | 🟢 Low |
| **TOTAL** | 29 files | **9-13 hours** | **🟡 Medium** |

---

## 🚀 **EXECUTION PLAN**

### **Day 1 (Morning)**: Documentation + BR Creation (4 hours)
1. Create `ADR-033-CROSS-SERVICE-BRS.md` (1h)
2. Create `BR-PLAYBOOK-CATEGORY.md` (1h)
3. Update 10 documentation files (2h)

### **Day 1 (Afternoon)**: Test Files (4 hours)
1. Update 14 test files with find/replace (3h)
2. Run full test suite validation (1h)

### **Day 2 (Morning)**: Production Code + Validation (5 hours)
1. Update 4 production code files (2h)
2. Build and validate production code (1h)
3. Update V5.0 implementation plan (1h)
4. Final validation checklist (1h)

---

## 🔗 **RELATED DOCUMENTS**

- [ADR-033: Remediation Playbook Catalog](ADR-033-remediation-playbook-catalog.md)
- [ADR-033: Executor Service Naming](ADR-033-EXECUTOR-SERVICE-NAMING-ASSESSMENT.md)
- [Data Storage Implementation Plan V5.0](../../services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V5.0.md)

---

## 📝 **MIGRATION STATUS**

**Current Status**: 📋 **PLANNED** (Ready for Execution)

**Milestones**:
- [ ] Phase 1: Documentation Complete
- [ ] Phase 2: Test Files Complete
- [ ] Phase 3: Production Code Complete
- [ ] Phase 4: Validation Complete
- [ ] Phase 5: V5.0 Plan Updated

**Estimated Completion**: November 6, 2025 (2 days)

---

## ⚠️ **RISKS & MITIGATION**

| Risk | Impact | Probability | Mitigation |
|---|---|---|---|
| **Breaking test labels** | High | Low | Validate all tests pass after migration |
| **Missing BR references** | Medium | Medium | Use grep to find all references before migration |
| **Incorrect BR mapping** | Medium | Low | Manual review of critical ADRs |
| **Production code breakage** | High | Very Low | Build validation before committing |

---

**Confidence**: **95%** - Straightforward find/replace with comprehensive validation

