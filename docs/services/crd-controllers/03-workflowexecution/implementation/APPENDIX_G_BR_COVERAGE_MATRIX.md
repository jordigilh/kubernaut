# WorkflowExecution - BR Coverage Matrix

**Parent Document**: [IMPLEMENTATION_PLAN_V3.0.md](./IMPLEMENTATION_PLAN_V3.0.md)
**Version**: v1.0
**Last Updated**: 2025-12-03
**Status**: 🚧 Template (To be populated during implementation)

---

## Document Purpose

This appendix provides the Business Requirements Coverage Matrix for the WorkflowExecution Controller, aligned with Day 9 of [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md).

---

## 📊 Coverage Summary

### Overall Coverage Calculation

**Formula**: `(BRs with tests / Total BRs) × 100 = Coverage %`

| Metric | Value | Target | Status |
|--------|-------|--------|--------|
| Total BRs | 11 | - | - |
| BRs with Unit Tests | 0/11 | 100% | 🚧 Pending |
| BRs with Integration Tests | 0/11 | 80%+ | 🚧 Pending |
| BRs with E2E Tests | 0/11 | 30%+ | 🚧 Pending |
| **Overall BR Coverage** | 0% | ≥97% | 🚧 Pending |

### Coverage By Test Type

| Test Type | BR Coverage | Test Count | Code Coverage | Status |
|-----------|-------------|------------|---------------|--------|
| **Unit Tests** | 0% (0/11 BRs) | 0 tests | 0% | 🚧 Target: >70% |
| **Integration Tests** | 0% (0/11 BRs) | 0 tests | 0% | 🚧 Target: >50% |
| **E2E Tests** | 0% (0/11 BRs) | 0 tests | 0% | 🚧 Target: <10% |

---

## 🔍 Per-BR Coverage Breakdown

### **BR-WE-001: Create PipelineRun from OCI Bundle**

**Requirement**: The controller MUST create a Tekton PipelineRun using the bundles resolver with the OCI image reference from WorkflowRef.ContainerImage.

#### Unit Tests
- **File**: `test/unit/workflowexecution/pipelinerun_test.go`
- **Tests**:
  - `It("should use bundles resolver")` - Lines TBD
  - `It("should extract OCI reference")` - Lines TBD
  - `DescribeTable("should handle various OCI formats")` - Lines TBD (5 scenarios)
- **Coverage**: 0/7 test cases 🚧

#### Integration Tests
- **File**: `test/integration/workflowexecution/lifecycle_test.go`
- **Tests**:
  - `It("should transition from Pending to Running when PipelineRun is created")` - Lines TBD
- **Coverage**: 0/1 integration test 🚧

#### E2E Tests
- **File**: `test/e2e/workflowexecution/workflow_test.go`
- **Tests**:
  - Covered in comprehensive E2E workflow
- **Coverage**: Implicit 🚧

**Status**: 🚧 **0% Coverage** (pending implementation)

---

### **BR-WE-002: Pass Parameters to Execution Engine**

**Requirement**: The controller MUST pass all parameters from WorkflowExecution.Spec.Parameters to the PipelineRun.Spec.Params.

#### Unit Tests
- **File**: `test/unit/workflowexecution/pipelinerun_test.go`
- **Tests**:
  - `DescribeTable("should handle parameters correctly")` - Lines TBD (5 entries)
    - Entry: "no parameters"
    - Entry: "single parameter"
    - Entry: "multiple parameters"
    - Entry: "large parameter value"
    - Entry: "unicode parameter value"
- **Coverage**: 0/5 test cases 🚧

#### Integration Tests
- **File**: `test/integration/workflowexecution/lifecycle_test.go`
- **Tests**:
  - Parameters verified in lifecycle test
- **Coverage**: 0/1 integration test 🚧

**Status**: 🚧 **0% Coverage** (pending implementation)

---

### **BR-WE-003: Execute in Dedicated Namespace**

**Requirement**: The controller MUST create all PipelineRuns in the dedicated `kubernaut-workflows` namespace, regardless of the WorkflowExecution source namespace.

#### Unit Tests
- **File**: `test/unit/workflowexecution/pipelinerun_test.go`
- **Tests**:
  - `It("should create in execution namespace")` - Lines TBD
  - `It("should not use source namespace")` - Lines TBD
- **Coverage**: 0/2 test cases 🚧

#### Integration Tests
- **File**: `test/integration/workflowexecution/lifecycle_test.go`
- **Tests**:
  - Namespace verified in "Verifying PipelineRun was created in execution namespace" assertion
- **Coverage**: 0/1 integration test 🚧

**Status**: 🚧 **0% Coverage** (pending implementation)

---

### **BR-WE-004: Owner Reference for Cascade Deletion**

**Requirement**: WorkflowExecution Controller MUST set owner reference on created PipelineRun to enable cascade deletion when WorkflowExecution is deleted.

#### Unit Tests
- **File**: `test/unit/workflowexecution/owner_ref_test.go`
- **Tests**:
  - `It("should set owner reference on PipelineRun")` - Lines TBD
  - `It("should set controller=true in owner reference")` - Lines TBD
  - `It("should reference correct WorkflowExecution")` - Lines TBD
- **Coverage**: 0/3 test cases 🚧

#### Integration Tests
- **File**: `test/integration/workflowexecution/lifecycle_test.go`
- **Tests**:
  - `It("should cascade delete PipelineRun when WFE deleted")` - Lines TBD
- **Coverage**: 0/1 integration test 🚧

**Status**: 🚧 **0% Coverage** (pending implementation)

---

### **BR-WE-005: Audit Events for Execution Lifecycle**

**Requirement**: WorkflowExecution Controller MUST emit audit events for key lifecycle transitions (created, running, completed, failed) to support compliance and debugging.

#### Unit Tests
- **File**: `test/unit/workflowexecution/audit_test.go`
- **Tests**:
  - `DescribeTable("should emit events for phase transitions")` - Lines TBD
    - Entry: "Pending → Running"
    - Entry: "Running → Completed"
    - Entry: "Running → Failed"
    - Entry: "Pending → Skipped"
  - `It("should include correlation_id in audit")` - Lines TBD
- **Coverage**: 0/5 test cases 🚧

#### Integration Tests
- **File**: `test/integration/workflowexecution/lifecycle_test.go`
- **Tests**:
  - Events verified via K8s event recorder
  - Audit verified via Data Storage API calls
- **Coverage**: 0/2 integration tests 🚧

**Status**: 🚧 **0% Coverage** (pending implementation)

---

### **BR-WE-006: ServiceAccount Configuration**

**Requirement**: WorkflowExecution Controller MUST support optional ServiceAccountName configuration for PipelineRun execution.

#### Unit Tests
- **File**: `test/unit/workflowexecution/service_account_test.go`
- **Tests**:
  - `It("should use ServiceAccountName from spec when provided")` - Lines TBD
  - `It("should use default ServiceAccountName when not specified")` - Lines TBD
  - `It("should propagate ServiceAccountName to PipelineRun")` - Lines TBD
- **Coverage**: 0/3 test cases 🚧

#### Integration Tests
- **File**: `test/integration/workflowexecution/lifecycle_test.go`
- **Tests**:
  - ServiceAccountName verified in created PipelineRun
- **Coverage**: 0/1 integration test 🚧

**Status**: 🚧 **0% Coverage** (pending implementation)

---

### **BR-WE-007: Handle Externally Deleted PipelineRun**

**Requirement**: WorkflowExecution Controller MUST gracefully handle PipelineRun deletion by external actors (operators, garbage collection) and mark WorkflowExecution as Failed.

#### Unit Tests
- **File**: `test/unit/workflowexecution/external_deletion_test.go`
- **Tests**:
  - `It("should handle NotFound error when getting PipelineRun")` - Lines TBD
  - `It("should mark WFE as Failed when PipelineRun deleted externally")` - Lines TBD
  - `It("should set message indicating external deletion")` - Lines TBD
- **Coverage**: 0/3 test cases 🚧

#### Integration Tests
- **File**: `test/integration/workflowexecution/lifecycle_test.go`
- **Tests**:
  - `It("should handle external PipelineRun deletion")` - Lines TBD
- **Coverage**: 0/1 integration test 🚧

**Status**: 🚧 **0% Coverage** (pending implementation)

---

### **BR-WE-008: Prometheus Metrics for Execution Outcomes**

**Requirement**: WorkflowExecution Controller MUST expose Prometheus metrics for execution outcomes (success/failure counts, duration histograms) on port 9090.

#### Unit Tests
- **File**: `test/unit/workflowexecution/metrics_test.go`
- **Tests**:
  - `It("should record workflowexecution_total on completion")` - Lines TBD
  - `It("should record workflowexecution_duration_seconds histogram")` - Lines TBD
  - `It("should include outcome and workflow_id labels")` - Lines TBD
  - `It("should record pipelinerun_creation_total counter")` - Lines TBD
- **Coverage**: 0/4 test cases 🚧

#### Integration Tests
- **File**: `test/integration/workflowexecution/metrics_test.go`
- **Tests**:
  - Metrics verified via Prometheus scrape endpoint
- **Coverage**: 0/1 integration test 🚧

**Status**: 🚧 **0% Coverage** (pending implementation)

---

### **BR-WE-009: Prevent Parallel Execution**

**Requirement**: The controller MUST prevent parallel remediation on the same target resource by checking for existing Running WFEs before creating a new PipelineRun.

#### Unit Tests
- **File**: `test/unit/workflowexecution/lock_test.go`
- **Tests**:
  - `It("should block when Running WFE exists for target")` - Lines TBD
  - `It("should not block when no Running WFE exists")` - Lines TBD
  - `It("should block for same target different workflow")` - Lines TBD
- **Coverage**: 0/3 test cases 🚧

#### Integration Tests
- **File**: `test/integration/workflowexecution/locking_test.go`
- **Tests**:
  - `It("should block second WFE when first is Running")` - Lines TBD
- **Coverage**: 0/1 integration test 🚧

#### E2E Tests
- **File**: `test/e2e/workflowexecution/locking_test.go`
- **Tests**:
  - `It("should efficiently skip duplicate remediations")` (BR SLA test)
- **Coverage**: 0/1 E2E test 🚧

**Status**: 🚧 **0% Coverage** (pending implementation)

---

### **BR-WE-010: Enforce Cooldown Period**

**Requirement**: The controller MUST enforce a configurable cooldown period (default 5 minutes) after successful/failed remediation to prevent wasteful sequential executions.

#### Unit Tests
- **File**: `test/unit/workflowexecution/lock_test.go`
- **Tests**:
  - `DescribeTable("should enforce cooldown period")` - Lines TBD (4 entries)
    - Entry: "1 minute ago (within cooldown)"
    - Entry: "4 minutes ago (within cooldown)"
    - Entry: "5 minutes ago (at boundary)"
    - Entry: "10 minutes ago (past cooldown)"
- **Coverage**: 0/4 test cases 🚧

#### Integration Tests
- **File**: `test/integration/workflowexecution/locking_test.go`
- **Tests**:
  - `It("should block WFE during cooldown after completion")` - Lines TBD
- **Coverage**: 0/1 integration test 🚧

**Status**: 🚧 **0% Coverage** (pending implementation)

---

### **BR-WE-011: Handle Race Conditions**

**Requirement**: The controller MUST gracefully handle race conditions where multiple controller instances attempt to create PipelineRuns for the same target, using the AlreadyExists error to mark WFE as Skipped.

#### Unit Tests
- **File**: `test/unit/workflowexecution/reconcile_pending_test.go`
- **Tests**:
  - `It("should handle AlreadyExists by marking Skipped")` - Lines TBD
  - `It("should not retry on AlreadyExists")` - Lines TBD
- **Coverage**: 0/2 test cases 🚧

#### Integration Tests
- **File**: `test/integration/workflowexecution/locking_test.go`
- **Tests**:
  - `It("should handle concurrent WFE creation gracefully")` - Lines TBD
- **Coverage**: 0/1 integration test 🚧

**Status**: 🚧 **0% Coverage** (pending implementation)

---

## 📈 Coverage Gap Analysis

### ✅ Fully Covered BRs (100% coverage)

| BR | Requirement | Unit | Integration | E2E | Status |
|----|-------------|------|-------------|-----|--------|
| - | - | - | - | - | 🚧 None yet |

### ⚠️ Partially Covered BRs (50-99% coverage)

**None** - All BRs pending implementation 🚧

### ❌ Uncovered BRs (0-49% coverage)

| BR | Requirement | Gap |
|----|-------------|-----|
| BR-WE-001 | PipelineRun Creation | All tests pending |
| BR-WE-002 | Parameter Passing | All tests pending |
| BR-WE-003 | Dedicated Namespace | All tests pending |
| BR-WE-004 | Owner Reference for Cascade Deletion | All tests pending |
| BR-WE-005 | Audit Events for Execution Lifecycle | All tests pending |
| BR-WE-006 | ServiceAccount Configuration | All tests pending |
| BR-WE-007 | Handle Externally Deleted PipelineRun | All tests pending |
| BR-WE-008 | Prometheus Metrics for Execution Outcomes | All tests pending |
| BR-WE-009 | Parallel Prevention | All tests pending |
| BR-WE-010 | Cooldown | All tests pending |
| BR-WE-011 | Race Conditions | All tests pending |

---

## 🎯 Testing Strategy Validation

### Target Coverage

| Test Type | Target | Current | Gap |
|-----------|--------|---------|-----|
| Unit Test Coverage | >70% | 0% | 70% |
| Integration Test Coverage | >50% | 0% | 50% |
| E2E Test Coverage | ~10% | 0% | 10% |
| BR Coverage | ≥97% | 0% | 97% |

### Test Count Targets

| Test Type | Target | Current | Gap |
|-----------|--------|---------|-----|
| Unit Tests | ~65 | 0 | 65 |
| Integration Tests | ~25 | 0 | 25 |
| E2E Tests | ~10 | 0 | 10 |
| **Total** | ~100 | 0 | 100 |

---

## 📝 Test File Reference Index (To Be Populated)

### Unit Tests
- `test/unit/workflowexecution/pipelinerun_test.go` - BR-WE-001, BR-WE-002, BR-WE-003
- `test/unit/workflowexecution/owner_ref_test.go` - BR-WE-004 (Owner Reference)
- `test/unit/workflowexecution/audit_test.go` - BR-WE-005 (Audit Events)
- `test/unit/workflowexecution/service_account_test.go` - BR-WE-006 (ServiceAccount)
- `test/unit/workflowexecution/external_deletion_test.go` - BR-WE-007 (External Deletion)
- `test/unit/workflowexecution/metrics_test.go` - BR-WE-008 (Metrics)
- `test/unit/workflowexecution/lock_test.go` - BR-WE-009, BR-WE-010
- `test/unit/workflowexecution/reconcile_pending_test.go` - BR-WE-011

### Integration Tests
- `test/integration/workflowexecution/suite_test.go` - Setup and teardown
- `test/integration/workflowexecution/lifecycle_test.go` - BR-WE-001 to BR-WE-005
- `test/integration/workflowexecution/locking_test.go` - BR-WE-009 to BR-WE-011
- `test/integration/workflowexecution/finalizer_test.go` - BR-WE-004 (Owner Reference cascade)
- `test/integration/workflowexecution/audit_test.go` - BR-WE-005 (Audit Events)
- `test/integration/workflowexecution/metrics_test.go` - BR-WE-008 (Metrics)

### E2E Tests
- `test/e2e/workflowexecution/workflow_test.go` - BR-WE-001, BR-WE-004, BR-WE-005
- `test/e2e/workflowexecution/locking_test.go` - BR-WE-009 (business outcome)

---

## 🔄 Coverage Maintenance

### When to Update This Matrix
- [x] After each day of implementation
- [ ] When tests are added/modified
- [ ] Before release (final validation)
- [ ] During code reviews

### Quality Indicators
- ✅ **Excellent**: >95% BR coverage (V3.0 standard)
- ✅ **Good**: 90-95% BR coverage
- ⚠️ **Acceptable**: 85-90% BR coverage
- ❌ **Insufficient**: <85% BR coverage

**Current Status**: 🚧 **0%** (implementation pending)

---

## References

- [BR Coverage Matrix Template](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md#enhanced-br-coverage-matrix-template-complete)
- [BUSINESS_REQUIREMENTS.md](../BUSINESS_REQUIREMENTS.md)
- [testing-strategy.md](../testing-strategy.md)

