# Test Migration Strategy - CRD Architecture

**Status**: ✅ Approved
**Date**: 2025-01-15
**Priority**: HIGH (blocks implementation phase)

---

## 📋 **Approved Strategy Summary**

**Focus**: Only architecture-impacted tests (74% of existing tests)
**Total Effort**: 800 hours (6-7 months with 3 devs)
**Cost**: $360K @ $150/hr
**Confidence**: 92%

### **Key Decision**: Architecture-Agnostic Tests Require 0 Effort

- ✅ **26% of tests** (pure algorithmic logic) remain valid with 0 changes
- ⚠️ **74% of tests** (controller/integration patterns) require work

---

## 🎯 **Migration Breakdown**

| Test Category | Files | Architecture Impact | Effort | Decision |
|---------------|-------|---------------------|--------|----------|
| **Pure Algorithmic Logic** | 40 (26%) | ✅ None | **0 hours** | ✅ Keep as-is |
| **Controller Entry Points** | 60 (39%) | ❌ Critical (100% rewrite) | **400 hours** | ❌ REWRITE |
| **Integration Patterns** | 35 (23%) | ❌ High (80% rewrite) | **250 hours** | ❌ REWRITE |
| **E2E Flow Orchestration** | 17 (11%) | ⚠️ Medium (50% adapt) | **150 hours** | ✅ ADAPT |
| **TOTAL** | **152** | **74% impacted** | **800 hours** | **HYBRID** |

---

## ✅ **Phase 1: Verification & Baseline** (PRIORITY 1)

**Duration**: 1 week
**Effort**: 40 hours
**Status**: 🔴 TODO

### **Objectives**:
1. Verify architecture-agnostic tests run successfully
2. Create architecture-agnostic test baseline
3. Document test categorization

### **Tasks**:

#### **Task 1.1: Run Algorithmic Tests**
```bash
# Run pure algorithmic tests to verify 0 failures
make test-unit-ai-algorithms
make test-unit-orchestration-algorithms
make test-unit-rego-policies
```

**Expected Result**: All tests pass with 0 changes
**If Failures**: Reclassify as architecture-dependent

#### **Task 1.2: Categorize All Test Files**
Create inventory: `docs/testing/TEST_INVENTORY.md`

| File | Category | Architecture Impact | Action | Effort |
|------|----------|---------------------|--------|--------|
| `test/unit/ai/confidence_scoring_test.go` | Algorithmic | ✅ None | Keep | 0h |
| `test/unit/alert/handler_test.go` | Controller | ❌ Critical | Rewrite | 8h |
| ... | ... | ... | ... | ... |

#### **Task 1.3: Document Architecture-Agnostic Baseline**
Create: `test/unit/algorithmic/README.md`

**Contents**:
- List of all architecture-agnostic tests
- Why these tests don't change (pure functions)
- Expected behavior validation

**Deliverables**:
- [ ] Test categorization spreadsheet
- [ ] Architecture-agnostic baseline document
- [ ] Verification report (pass/fail for each category)

---

## ❌ **Phase 2: Rewrite Controller Tests** (PRIORITY 2)

**Duration**: 3 months
**Effort**: 400 hours
**Status**: 🔴 TODO

### **Objectives**:
1. Rewrite HTTP handlers as CRD reconcilers
2. Use old tests as reference documentation
3. Follow APDC-TDD methodology

### **Tasks by Service**:

#### **Task 2.1: AlertProcessing Controller** (80 hours)
**Location**: `test/unit/alertprocessor/`

**Subtasks**:
- [ ] Create `controller_test.go` - reconciliation loop (20h)
- [ ] Create `enrichment_test.go` - enrichment phase (15h)
- [ ] Create `classification_test.go` - environment classification (15h)
- [ ] Create `context_integration_test.go` - Context Service mock (15h)
- [ ] Create `routing_test.go` - routing decisions (15h)

**Reference**: `test/unit/alert/handler_test.go` (for expected behavior)

#### **Task 2.2: AIAnalysis Controller** (90 hours)
**Location**: `test/unit/aianalysis/`

**Subtasks**:
- [ ] Create `controller_test.go` - reconciliation loop (20h)
- [ ] Create `investigation_test.go` - HolmesGPT integration (20h)
- [ ] Create `approval_test.go` - approval workflow (15h)
- [ ] Create `rego_policy_test.go` - policy evaluation (reuse existing!) (5h)
- [ ] Create `aiapprovalrequest_watch_test.go` - child CRD watch (15h)
- [ ] Create `approval_workflow_states_test.go` - state machine (15h)

**Note**: Rego policy tests are architecture-agnostic - copy from existing

#### **Task 2.3: WorkflowExecution Controller** (85 hours)
**Location**: `test/unit/workflowexecution/`

**Subtasks**:
- [ ] Create `controller_test.go` - reconciliation loop (20h)
- [ ] Create `orchestration_test.go` - step orchestration (25h)
- [ ] Create `dependency_resolution_test.go` - reuse existing algorithm tests! (5h)
- [ ] Create `kubernetesexecution_creation_test.go` - child CRD creation (20h)
- [ ] Create `step_status_watch_test.go` - child CRD status monitoring (15h)

**Note**: Dependency resolution tests are architecture-agnostic - copy from existing

#### **Task 2.4: KubernetesExecution (DEPRECATED - ADR-025) Controller** (95 hours)
**Location**: `test/unit/kubernetesexecution/`

**Subtasks**:
- [ ] Create `controller_test.go` - reconciliation loop (20h)
- [ ] Create `job_creation_test.go` - Kubernetes Job creation (20h)
- [ ] Create `rbac_validation_test.go` - per-action RBAC (15h)
- [ ] Create `per_action_test.go` - 10 predefined actions (25h)
- [ ] Create `dryrun_validation_test.go` - dry-run tests (15h)

#### **Task 2.5: AlertRemediation Controller** (50 hours)
**Location**: `test/unit/alertremediation/`

**Subtasks**:
- [ ] Create `controller_test.go` - central orchestration (15h)
- [ ] Create `child_crd_creation_test.go` - 4 child CRD creation (15h)
- [ ] Create `timeout_detection_test.go` - phase timeout (10h)
- [ ] Create `escalation_test.go` - notification escalation (10h)

**Deliverables**:
- [ ] 60 new test files for CRD controllers
- [ ] 70%+ unit test coverage
- [ ] All tests follow APDC-TDD methodology

---

## ❌ **Phase 3: Rewrite Integration Tests** (PRIORITY 3)

**Duration**: 2 months
**Effort**: 250 hours
**Status**: 🔴 TODO

### **Objectives**:
1. Rewrite HTTP client patterns as CRD watch patterns
2. Test cross-CRD coordination
3. Real Kubernetes API integration (Kind cluster)

### **Tasks**:

#### **Task 3.1: CRD Watch Patterns** (90 hours)
**Location**: `test/integration/crd_coordination/`

**Subtasks**:
- [ ] AlertRemediation → AlertProcessing watch (20h)
- [ ] AlertRemediation → AIAnalysis watch (20h)
- [ ] AlertRemediation → WorkflowExecution watch (20h)
- [ ] WorkflowExecution → KubernetesExecution watch (20h)
- [ ] AIAnalysis → AIApprovalRequest watch (10h)

#### **Task 3.2: Owner Reference & Cascade Deletion** (80 hours)
**Location**: `test/integration/lifecycle/`

**Subtasks**:
- [ ] Owner reference validation (20h)
- [ ] Cascade deletion (parent → children) (30h)
- [ ] Finalizer cleanup (20h)
- [ ] CRD retention policy (10h)

#### **Task 3.3: Cross-Service Coordination** (80 hours)
**Location**: `test/integration/cross_service/`

**Subtasks**:
- [ ] AlertProcessing → Context Service integration (15h)
- [ ] AIAnalysis → HolmesGPT integration (real API) (20h)
- [ ] WorkflowExecution → multi-step coordination (25h)
- [ ] KubernetesExecution → Job execution (real K8s) (20h)

**Deliverables**:
- [ ] 35 new integration test files
- [ ] >50% integration test coverage
- [ ] Real Kind cluster integration

---

## ✅ **Phase 4: Adapt E2E Tests** (PRIORITY 4)

**Duration**: 1 month
**Effort**: 150 hours
**Status**: 🔴 TODO

### **Objectives**:
1. Keep business scenarios & expected outcomes
2. Update execution flow to 5-CRD cascade
3. Validate end-to-end alert-to-resolution flows

### **Tasks**:

#### **Task 4.1: Alert Remediation Scenarios** (80 hours)
**Location**: `test/e2e/scenarios/`

**Subtasks**:
- [ ] Scenario 1: OOM remediation (pod restart) (15h)
- [ ] Scenario 1b: Repeated OOM (memory increase + GitOps PR) (20h)
- [ ] Scenario 2: High CPU (scale deployment) (15h)
- [ ] Scenario 3: Pod CrashLoop (rollback) (15h)
- [ ] Scenario 4: Multi-step workflow (15h)

#### **Task 4.2: Failure & Escalation Scenarios** (40 hours)
**Location**: `test/e2e/failure/`

**Subtasks**:
- [ ] Phase timeout → escalation (15h)
- [ ] Step failure → rollback (15h)
- [ ] Manual approval → operator intervention (10h)

#### **Task 4.3: GitOps Integration** (30 hours)
**Location**: `test/e2e/gitops/`

**Subtasks**:
- [ ] GitOps PR creation (GitHub) (15h)
- [ ] Direct patch (non-GitOps dev env) (10h)
- [ ] GitOps annotation detection (5h)

**Deliverables**:
- [ ] 17 adapted E2E test files
- [ ] <10% E2E test coverage
- [ ] Complete alert-to-resolution validation

---

## 📊 **Progress Tracking**

### **Overall Progress**

```
Phase 1: Verification          🔴 TODO  (  0/40h complete)
Phase 2: Controller Rewrite    🔴 TODO  (  0/400h complete)
Phase 3: Integration Rewrite   🔴 TODO  (  0/250h complete)
Phase 4: E2E Adaptation        🔴 TODO  (  0/150h complete)
──────────────────────────────────────────────────────────
TOTAL:                         🔴 TODO  (  0/800h complete)
```

### **Test File Count**

| Category | Existing | After Migration | Status |
|----------|----------|-----------------|--------|
| **Unit Tests** | ~50 | ~100 (40 kept + 60 new) | 🔴 0% |
| **Integration Tests** | ~52 | ~75 (40 kept + 35 new) | 🔴 0% |
| **E2E Tests** | ~50 | ~67 (50 kept + 17 new) | 🔴 0% |
| **TOTAL** | **152** | **242** | **🔴 0%** |

---

## 🎯 **Success Criteria**

### **Phase 1 Complete**:
- [ ] 100% of algorithmic tests verified to run with 0 changes
- [ ] Test inventory created with all 152 files categorized
- [ ] Architecture-agnostic baseline documented

### **Phase 2 Complete**:
- [ ] 60 new CRD controller test files created
- [ ] 70%+ unit test coverage achieved
- [ ] All tests follow APDC-TDD methodology
- [ ] All tests use fake K8s client for compile-time safety

### **Phase 3 Complete**:
- [ ] 35 new integration test files created
- [ ] >50% integration test coverage achieved
- [ ] All tests use real Kubernetes API (Kind cluster)
- [ ] CRD watch patterns validated

### **Phase 4 Complete**:
- [ ] 17 E2E test files adapted to 5-CRD cascade
- [ ] <10% E2E test coverage maintained
- [ ] All critical business scenarios validated
- [ ] GitOps integration scenarios tested

### **Overall Complete**:
- [ ] Defense-in-depth overlap validated (>130% total coverage)
- [ ] All business requirements (BR-XXX-YYY) mapped to tests
- [ ] Test migration complete in 6-7 months
- [ ] 800 hours total effort achieved

---

## 📚 **Reference Documents**

1. **TEST_ARCHITECTURE_IMPACT_ANALYSIS.md** - Detailed analysis of architecture impact
2. **TEST_MIGRATION_VS_REWRITE_ANALYSIS.md** - Migration vs. rewrite decision rationale
3. **TESTING_CONFIDENCE_ASSESSMENT.md** - Existing test coverage analysis
4. **TESTING_STRATEGY_SUMMARY.md** - Overall testing strategy for all services

---

## 🚨 **Risks & Mitigations**

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| **Some "pure" tests have hidden architecture dependencies** | Medium | Medium | Phase 1 verification catches these early |
| **Rewrite effort exceeds estimates** | Low | Medium | 15% buffer in effort estimates |
| **Losing edge case coverage** | Medium | High | Document edge cases before deletion |
| **New tests miss business requirements** | Low | Critical | Map all tests to BR-XXX-YYY |

---

## ✅ **Next Steps**

**Immediate Actions** (Week 1):
1. ✅ Review and approve this migration strategy
2. 🔴 Assign team members to phases
3. 🔴 Start Phase 1: Verification & Baseline (1 week)
4. 🔴 Set up Kind cluster for integration testing
5. 🔴 Create test inventory spreadsheet

**After Phase 1 Complete**:
- Begin Phase 2: Controller rewrite (AlertProcessing first)
- Parallel work: 3 developers can work on different services simultaneously

---

**Status**: ✅ **APPROVED - Ready for implementation**
**Last Updated**: 2025-01-15

