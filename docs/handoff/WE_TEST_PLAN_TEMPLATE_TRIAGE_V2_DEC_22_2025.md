# WorkflowExecution Test Plan - Template Compliance Triage (v2)

**Date**: December 22, 2025
**Comparison**: WE Unit Test Plan 1.0.0 vs Template 1.3.0 + NT 1.3.0 + Best Practices 1.0.0
**Status**: 🚨 **CRITICAL GAPS IDENTIFIED**
**Action Required**: Comprehensive rewrite of WE test plan

---

## 📊 **Updated Template Comparison Matrix (Template 1.3.0)**

| Feature Area | Template 1.3.0 Requirement | WE Plan Status | Gap Severity | Impact |
|--------------|----------------------------|----------------|--------------|--------|
| **Tier Headers Notation** | `(70%+ BR Coverage \| 70%+ Code Coverage)` | ❌ Not used | 🟡 **HIGH** | Ambiguous coverage meaning |
| **Current Test Status** | Pre-feature test status (NEW in 1.3.0) | ❌ Not included | 🟡 **HIGH** | Can't see what's done vs. new |
| **Pre/Post Comparison** | Quantified value proposition (NEW in 1.3.0) | ❌ Not included | 🟠 **MEDIUM** | Stakeholders can't see ROI |
| **Day-by-Day Timeline** | Actionable execution plan (NEW in 1.3.0) | ❌ Not included | 🟠 **MEDIUM** | No concrete implementation plan |
| **Infrastructure Setup** | MANDATORY for Integration/E2E (NEW in 1.2.1) | ❌ Not included | 🔴 **CRITICAL** | Can't reproduce test environment |
| **Code Coverage Column** | In "Test Outcomes by Tier" (NEW in 1.2.2) | ❌ Not included | 🟡 **HIGH** | Missing code coverage info |
| **Cross-References** | Links to Best Practices + example plans | ❌ Not included | 🟠 **MEDIUM** | No guidance for teams |
| **Scope limited to unit tests** | All V1.0 maturity features | ❌ Unit only | 🔴 **CRITICAL** | Missing 80% of V1.0 features |
| **Coverage targets outdated** | 70%/50%/50% (empirical data) | ⚠️ 70%/N/A/N/A | 🟡 **HIGH** | E2E should be 50%, not undefined |
| **Metrics testing** | Integration + E2E sections | ❌ Not covered | 🔴 **CRITICAL** | V1.0 maturity blocker |
| **Audit testing** | Integration + E2E with OpenAPI | ❌ Not covered | 🔴 **CRITICAL** | V1.0 maturity blocker |
| **Graceful shutdown** | Unit + Integration + E2E | ❌ Not covered | 🔴 **CRITICAL** | V1.0 maturity blocker |
| **Health probes** | E2E tests | ❌ Not covered | 🔴 **CRITICAL** | V1.0 maturity blocker |
| **Predicates** | Unit test | ❌ Not covered | 🟡 **HIGH** | CRD controller requirement |
| **EventRecorder** | E2E tests | ❌ Not covered | 🟡 **HIGH** | V1.0 maturity feature |

---

## 🆕 **New Gaps from Template 1.3.0 Updates**

### Gap 13: Tier Headers Use Old Notation (**HIGH**)

**Template 1.3.0 Requirement** (per v1.2.1):
```markdown
# 🧪 **TIER 1: UNIT TESTS** (70%+ BR Coverage | 70%+ Code Coverage)
# 🔗 **TIER 2: INTEGRATION TESTS** (>50% BR Coverage | 50% Code Coverage)
# 🚀 **TIER 3: E2E TESTS** (<10% BR Coverage | 50% Code Coverage)
```

**WE Plan Current State**:
```markdown
# 🧪 **TIER 1: UNIT TESTS** (70% Coverage) - ✅ COMPLETE
```

**Problem**: Ambiguous - is it BR coverage or code coverage?

**Impact**:
- ❌ Unclear what "70% coverage" means
- ❌ Doesn't communicate defense-in-depth strategy
- ❌ Inconsistent with NT, template standards

**Required Action**: Update all tier headers to use `(BR Coverage | Code Coverage)` notation

---

### Gap 14: Current Test Status Section Missing (**HIGH**)

**Template 1.3.0 Requirement** (NEW in v1.3.0):

Shows stakeholders **what's already done vs. new work needed**. Critical for existing codebases.

**Example from NT 1.3.0**:
```markdown
## 📊 **Current Test Status**

### Pre-MVP Status

| Test Suite | Tests | Status | Coverage |
|---|---|---|---|
| Controller reconciliation | 35 | ✅ Passing | BR-NOT-052, 053, 056 |
| Delivery services | 25 | ✅ Passing | BR-NOT-053 |

**Total Existing Tests**: 131 tests (117 unit + 9 integration + 5 E2E)

### Assessment

**Unit Tests**: ✅ **NO NEW UNIT TESTS NEEDED** - Existing coverage is comprehensive
**Integration Tests**: ✅ **NO NEW INTEGRATION TESTS NEEDED** - Existing coverage is sufficient
**E2E Tests**: ⏸️ **3 NEW E2E TESTS NEEDED** - Validate retry, fanout, priority routing
```

**WE Plan Current State**:
- ❌ No "Current Test Status" section
- ❌ Existing 173 unit tests not summarized for stakeholders
- ❌ No assessment of what's done vs. needed

**Impact**:
- ❌ Stakeholders can't see what's already passing
- ❌ Can't distinguish existing from new work
- ❌ Effort estimation unclear

**Required Action**: Add Current Test Status section with:
- Pre-implementation test status
- Assessment per tier (new tests needed?)
- Total existing test count

---

### Gap 15: Pre/Post Comparison Section Missing (**MEDIUM** for stakeholders)

**Template 1.3.0 Requirement** (NEW in v1.3.0):

**Guidance** (per Best Practices):
> Use this section to quantify value proposition for stakeholders. Shows confidence improvement and test coverage increase.

**Example from NT 1.3.0**:
```markdown
## 🎉 **Expected Outcomes**

### Pre-MVP Status:
- ✅ 131 tests passing (117 unit + 9 integration + 5 E2E)
- ✅ 100% pass rate
- ✅ 95% confidence for production

### Post-MVP Status (Target):
- ✅ 134 tests passing (117 unit + 9 integration + 8 E2E)
- ✅ 100% pass rate
- ✅ 99% confidence for production

### Confidence Improvement:
- **Before MVP**: 95% confidence
- **After MVP**: 99% confidence
- **Improvement**: +4% confidence increase
```

**WE Plan Current State**:
- ❌ No Pre/Post Comparison
- ❌ No confidence quantification

**Impact**:
- ❌ Stakeholders can't see ROI
- ❌ Value proposition unclear
- ❌ Effort justification missing

**Required Action**: Add Pre/Post Comparison section with confidence improvement

---

### Gap 16: Day-by-Day Timeline Missing (**MEDIUM** for complex features)

**Template 1.3.0 Requirement** (NEW in v1.3.0):

**Guidance** (per Best Practices):
> **When to Use**: Complex feature (10+ new tests, multiple owners) = **MANDATORY**

**Example from NT 1.3.0**:
```markdown
## ⏱️ **Execution Timeline**

### Week 1: Core MVP E2E Tests

| Day | Task | Time | Owner | Deliverable |
|---|---|---|---|---|
| **Day 1** | E2E-1: Retry and Exponential Backoff | 1 day | NT Team | Test file + passing |
| **Day 2 AM** | E2E-2: Multi-Channel Fanout | 0.5 day | NT Team | Test file + passing |
| **Day 2 PM** | E2E-3: Priority-Based Routing | 0.5 day | NT Team | Test file + passing |

**Total Time**: **2 days**
```

**WE Plan Current State**:
- ⚠️ High-level "Implementation Plan" exists (Phase 1-5)
- ❌ No day-by-day breakdown
- ❌ No owner assignments
- ❌ No deliverable specifications

**Impact**:
- ⚠️ Less critical for WE (16 new unit tests, not complex E2E)
- ❌ May be needed if WE expands to full V1.0 maturity (integration + E2E)

**Required Action**:
- **Now**: Keep high-level plan (simple unit test addition)
- **Later**: Add day-by-day timeline when expanding to integration/E2E

---

### Gap 17: Infrastructure Setup Sections Missing (**CRITICAL** for Integration/E2E)

**Template 1.3.0 Requirement** (NEW in v1.2.1):

**Guidance** (per Best Practices):
```
| Tier | When to Include | When to Skip |
|------|----------------|--------------|
| Unit | Requires special fixtures/setup | Simple unit tests with just mocks |
| Integration | **MANDATORY** - Always include | Never skip |
| E2E | **MANDATORY** - Always include | Never skip |
```

**Example from NT 1.3.0**:
```markdown
## 🏗️ **E2E Infrastructure Setup**

### Prerequisites
- Kind cluster running
- Notification controller deployed
- File delivery channel configured

### Setup Commands
```bash
# 1. Create Kind cluster
make kind-up

# 2. Deploy controller
make deploy-notification

# 3. Verify controller
kubectl get pods -n kubernaut-system | grep notification
```

### Infrastructure Validation
```bash
make validate-e2e-notification-infrastructure

# Expected checks:
# ✅ Kind cluster accessible
# ✅ Controller deployed
# ✅ CRDs registered
```
```

**WE Plan Current State**:
- ❌ No Infrastructure Setup for unit tests (acceptable - no special setup)
- ❌ No Infrastructure Setup for integration tests (CRITICAL - not even planned)
- ❌ No Infrastructure Setup for E2E tests (CRITICAL - not even planned)

**Impact**:
- ❌ Can't reproduce integration/E2E environments
- ❌ New team members blocked
- ❌ CI/CD pipeline setup unclear

**Required Action**:
- **Now**: N/A (WE plan is unit-only, no infrastructure needed)
- **Critical**: Add Infrastructure Setup when expanding to integration/E2E

---

### Gap 18: Code Coverage Column Missing in Test Outcomes (**HIGH**)

**Template 1.3.0 Requirement** (NEW in v1.2.2):

**Example from NT 1.3.0**:
```markdown
# 🎯 **Test Outcomes by Tier**

| Tier | What It Proves | Failure Means | Code Coverage |
|------|----------------|---------------|---------------|
| **Unit** | Controller logic is correct | Bug in NT controller code | 70%+ |
| **Integration** | CRD operations and audit work | Kubernetes integration issue | 50% |
| **E2E** | Complete notification lifecycle works | System doesn't serve business need | 50% |
```

**WE Plan Current State**:
- ❌ No "Test Outcomes by Tier" section
- ❌ No code coverage column

**Impact**:
- ❌ Can't see what each tier validates
- ❌ Code coverage per tier unclear
- ❌ Diagnostic guidance missing

**Required Action**: Add "Test Outcomes by Tier" section with code coverage column

---

### Gap 19: Cross-References to Best Practices Missing (**MEDIUM**)

**Template 1.3.0 Requirement** (NEW in v1.3.0):

**Example from NT 1.3.0 Header**:
```markdown
**Cross-References**:
- [Test Plan Best Practices](../../../docs/development/testing/TEST_PLAN_BEST_PRACTICES.md) - When/why to use each section
- [NT Test Plan Example](../../../docs/services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md) - Complete implementation reference
```

**WE Plan Current State**:
- ❌ No cross-references
- ❌ No guidance links

**Impact**:
- ❌ Teams don't know where to find guidance
- ❌ No reference implementations

**Required Action**: Add cross-references in header

---

## 📋 **Updated Proposed WE Test Plan Structure (Template 1.3.0 Compliant)**

### Recommended File Name
```
docs/services/crd-controllers/03-workflowexecution/TEST_PLAN_WE_V1_0.md
```

### Recommended Structure

```markdown
# WorkflowExecution (WE) V1.0 - Test Plan

**Version**: 2.0.0
**Last Updated**: [DATE]
**Status**: READY FOR EXECUTION

**Cross-References**:
- [Test Plan Best Practices](../../../docs/development/testing/TEST_PLAN_BEST_PRACTICES.md)
- [NT Test Plan Example](../../crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md)
- [Template](../../../holmesgpt-api/tests/e2e/TEST_PLAN_WORKFLOW_CATALOG_EDGE_CASES.md)

**Business Requirements**: BR-WE-001 through BR-WE-006
**Design Decisions**: DD-METRICS-001, DD-005 V3.0, DD-007, ADR-032
**Authority**: DD-TEST-001 - Port Allocation Strategy

---

## 📋 Changelog

### Version 2.0.0 ([DATE])
- **BREAKING**: Expanded scope to full V1.0 Service Maturity Test Plan
- **RESTRUCTURED**: Per Template 1.3.0 + NT 1.3.0 + Best Practices 1.0.0
- **ADDED**: Tier headers with BR/Code coverage notation `(70%+ BR Coverage | 70%+ Code Coverage)`
- **ADDED**: Current Test Status section (pre-implementation baseline)
- **ADDED**: Pre/Post Comparison section (value proposition)
- **ADDED**: Infrastructure Setup sections (Integration/E2E)
- **ADDED**: Test Outcomes by Tier with code coverage column
- **ADDED**: Cross-references to Best Practices and NT example
- **ADDED**: All V1.0 maturity features (metrics, audit, shutdown, probes, predicates, events)
- **ADDED**: Defense-in-depth examples for key BRs
- **ADDED**: Compliance sign-off section
- **ADDED**: E2E coverage implementation plan (DD-TEST-007)

### Version 1.0.0 (2025-12-22)
- Initial unit test plan (scope limited to unit tests only)

---

## 🎯 Testing Scope

[Visual diagram of WE controller + dependencies]

---

## 📊 Defense-in-Depth Testing Summary

**Strategy**: Overlapping BR coverage + cumulative code coverage approaching 100%

### BR Coverage (Overlapping) + Code Coverage (Cumulative)

| Tier | Tests | Infrastructure | BR Coverage | Code Coverage | Status |
|------|-------|----------------|-------------|---------------|--------|
| **Unit** | 173 | None (mocked external deps) | 70%+ of ALL BRs | 70%+ | ✅ 69.2% |
| **Integration** | 9 | Real K8s (envtest) + Mock DS | >50% of ALL BRs | 50% | ⚠️ Failing |
| **E2E** | 8 | Real K8s (Kind) + Real DS/Tekton | <10% BR coverage | 50% | ⏸️ Needs instrumentation |

**Example - Cooldown Logic (BR-WE-003)**:
- **Unit (70%)**: Cooldown calculation algorithm correctness
  - Tests: `test/unit/workflowexecution/controller_test.go`
  - Code: `ReconcileDelete()` cooldown logic
- **Integration (50%)**: Cooldown with real CRD and K8s API
  - Tests: `test/integration/workflowexecution/reconciler_test.go`
  - Infrastructure: envtest
- **E2E (50%)**: Cooldown with full controller deployment
  - Tests: `test/e2e/workflowexecution/01_lifecycle_test.go`
  - Infrastructure: Kind cluster + Tekton

If cooldown calculation has a bug, it must slip through **ALL 3 defense layers**!

---

## 📊 **Current Test Status**

> Shows what's already passing vs. new work needed per tier

### Pre-Implementation Status

| Test Suite | Tests | Status | Coverage |
|---|---|---|---|
| Controller instantiation | 2 | ✅ Passing | BR-WE-001 |
| PipelineRun naming | 6 | ✅ Passing | BR-WE-002 |
| HandleAlreadyExists | 3 | ✅ Passing | BR-WE-002 |
| BuildPipelineRun | 9 | ✅ Passing | BR-WE-002 |
| ConvertParameters | 4 | ✅ Passing | BR-WE-002 |
| FindWFEForPipelineRun | 4 | ✅ Passing | BR-WE-002 |
| BuildPipelineRunStatusSummary | 3 | ✅ Passing | BR-WE-004 |
| MarkCompleted | 4 | ✅ Passing | BR-WE-004 |
| MarkFailed (Basic) | 17 | ✅ Passing | BR-WE-004 |
| ReconcileDelete (Cooldown & Cleanup) | 12 | ✅ Passing | BR-WE-003 |
| Metrics | 6 | ✅ Passing | DD-METRICS-001 |
| Audit (ADR-032) | 11 | ✅ Passing | BR-WE-005 |
| Spec Validation | 7 | ✅ Passing | BR-WE-001 |
| updateStatus | 3 | ✅ Passing | BR-WE-004 |
| Failure Detection | 8 | ✅ Passing | BR-WE-004 |
| Conditions | 7 | ✅ Passing | BR-WE-006 |

**Total Existing Tests**: **173 unit tests** (157 controller + 8 conditions + 8 P1/P2/P3 new)
**Unit Coverage**: 69.2% (target: 70%+)
**Integration Tests**: 9 tests (100% failing due to infrastructure issues)
**E2E Tests**: 8 tests (100% passing, but no coverage instrumentation)

### Assessment

**Unit Tests**: ⚠️ **0.8% BELOW TARGET** - 173 tests at 69.2%, need minimal additions to reach 70%+
- Gap identified: `MarkFailedWithReason` (62.1%), `updateStatus` (60.0%), `sanitizeLabelValue` (75.0%)
- Plan: P1 (8 tests), P2 (5 tests), P3 (3 tests) = 16 new tests to reach 75%

**Integration Tests**: 🔴 **FAILING** - 9 tests exist but infrastructure issues prevent execution
- Status: Consistent Data Storage connection failures
- Required: Fix integration test infrastructure (separate issue)

**E2E Tests**: ⏸️ **MISSING E2E COVERAGE INSTRUMENTATION** - Tests passing but no coverage measurement
- Status: 8 E2E tests passing (lifecycle, observability)
- Required: Implement DD-TEST-007 E2E coverage capture (50% target)

---

# 🧪 **TIER 1: UNIT TESTS** (70%+ BR Coverage | 70%+ Code Coverage)

**Location**: `test/unit/workflowexecution/`
**Infrastructure**: None (mocked external dependencies)
**Execution**: `make test-unit-workflowexecution` or `go test ./test/unit/workflowexecution/... -v`

[Existing unit test content from WE plan 1.0.0, updated with new P1/P2/P3 tests]

---

# 🔗 **TIER 2: INTEGRATION TESTS** (>50% BR Coverage | 50% Code Coverage)

**Location**: `test/integration/workflowexecution/`
**Infrastructure**: Real Kubernetes (envtest) + Mock DataStorage
**Execution**: `make test-integration-workflowexecution`

## 🏗️ **Integration Test Infrastructure Setup**

> **MANDATORY** for integration tests per Template 1.3.0

### Prerequisites

- Go 1.22+
- envtest (Kubernetes control plane binaries)
- Mock DataStorage service (podman-compose)

### Setup Commands

```bash
# 1. Start Data Storage infrastructure
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
podman compose -f podman-compose.test.yml up -d

# 2. Wait for Data Storage to be ready
curl -s http://localhost:18100/health
# Expected: HTTP 200 OK

# 3. Run integration tests
make test-integration-workflowexecution

# 4. Cleanup
podman compose -f podman-compose.test.yml down
```

### Infrastructure Validation

```bash
# Verify all integration prerequisites are met
make validate-integration-workflowexecution-infrastructure

# Expected checks:
# ✅ envtest binaries available
# ✅ Data Storage service accessible at http://localhost:18100
# ✅ Data Storage /health endpoint responding
```

---

## 1. Metrics Testing (Integration)

[Integration test templates for metrics]

---

## 2. Audit Trace Testing (Integration)

[Integration test templates for audit with OpenAPI client mandate]

---

## 3. Graceful Shutdown Testing (Integration)

[Integration test templates for graceful shutdown]

---

# 🚀 **TIER 3: E2E TESTS** (<10% BR Coverage | 50% Code Coverage)

**Location**: `test/e2e/workflowexecution/`
**Infrastructure**: Real Kubernetes (Kind) + Real DataStorage + Tekton
**Execution**: `make test-e2e-workflowexecution`

## 🏗️ **E2E Infrastructure Setup**

> **MANDATORY** for E2E tests per Template 1.3.0

### Prerequisites

- Kind cluster
- Tekton Pipelines v0.56.0+
- DataStorage service
- WorkflowExecution controller deployed

### Setup Commands

```bash
# 1. Create Kind cluster
make kind-up

# 2. Deploy Tekton
kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml

# 3. Deploy DataStorage
make deploy-datastorage

# 4. Deploy WorkflowExecution controller
make deploy-workflowexecution

# 5. Verify all components
kubectl get pods -n kubernaut-system
kubectl get pods -n tekton-pipelines

# Expected output:
# workflowexecution-controller-manager-xxxx   2/2   Running
# tekton-pipelines-controller-xxxx            1/1   Running
# datastorage-xxxx                            1/1   Running

# 6. Run E2E tests
make test-e2e-workflowexecution
```

### Infrastructure Validation

```bash
# Verify all E2E prerequisites are met
make validate-e2e-workflowexecution-infrastructure

# Expected checks:
# ✅ Kind cluster accessible
# ✅ Tekton controller deployed
# ✅ DataStorage service accessible
# ✅ WorkflowExecution CRD registered
# ✅ WorkflowExecution controller running
```

---

## 1. Metrics Testing (E2E)

[E2E test templates for /metrics endpoint]

---

## 2. Audit Trace Testing (E2E)

[E2E test templates for audit client wiring]

---

## 3. EventRecorder Testing (E2E)

[E2E test templates for event emission]

---

## 4. Graceful Shutdown Testing (E2E)

[E2E test templates for SIGTERM handling]

---

## 5. Health Probes Testing (E2E)

[E2E test templates for /healthz and /readyz]

---

## 6. Predicates Testing (Unit)

[Unit test templates for predicates]

---

# 🎯 **Test Outcomes by Tier**

| Tier | What It Proves | Failure Means | Code Coverage |
|------|----------------|---------------|---------------|
| **Unit** | WE controller logic is correct | Bug in controller code | 70%+ |
| **Integration** | CRD operations and audit work with real K8s API | Kubernetes integration issue or audit client problem | 50% |
| **E2E** | Complete workflow execution lifecycle works in production-like environment | System doesn't orchestrate Tekton workflows correctly | 50% |

---

# 🎉 **Expected Outcomes**

### Pre-Implementation Status:
- ✅ 173 unit tests passing (69.2% coverage)
- ⚠️ 9 integration tests (failing - infrastructure issues)
- ✅ 8 E2E tests passing (no coverage instrumentation)
- ✅ 85% confidence for V1.0 (unit + E2E validated, integration issues)

### Post-Implementation Status (Target):
- ✅ 173 unit tests passing (75%+ coverage - P1/P2/P3 complete)
- ✅ 9+ integration tests passing (infrastructure fixed, V1.0 maturity tests added)
- ✅ 8+ E2E tests passing (50% coverage via DD-TEST-007)
- ✅ 99% confidence for V1.0 (all tiers validated, V1.0 maturity complete)

### Confidence Improvement:
- **Before**: 85% confidence (unit strong, integration failing, E2E no coverage)
- **After**: 99% confidence (unit 75%, integration 50%, E2E 50% with instrumentation)
- **Improvement**: +14% confidence increase

**Rationale**:
- Unit coverage increased from 69.2% to 75%+ (CRD enum coverage complete)
- Integration tests fixed and expanded (metrics, audit, graceful shutdown validated)
- E2E coverage instrumented per DD-TEST-007 (50% target achieved)
- All V1.0 maturity features validated (metrics, audit, shutdown, probes, predicates, events)

---

# 📊 E2E Coverage Implementation (DD-TEST-007)

[Detailed plan for E2E coverage instrumentation]

---

# 📁 Test File Structure

[Test file structure with existing vs. new files]

---

# 📋 Compliance Sign-Off

[Test execution summary and approval table]

---

# 📚 References

- **[Test Plan Best Practices](../../../docs/development/testing/TEST_PLAN_BEST_PRACTICES.md)** - When/why to use each section
- **[NT Test Plan](../../crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md)** - Complete implementation reference (v1.3.0)
- **[Template](../../../holmesgpt-api/tests/e2e/TEST_PLAN_WORKFLOW_CATALOG_EDGE_CASES.md)** - Reusable template (v1.3.0)
- **[Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)** - Defense-in-depth approach
- **[Testing Guidelines](../../../docs/development/business-requirements/TESTING_GUIDELINES.md)** - BR vs code coverage (v2.4.0)
- **[DD-TEST-007](../../../docs/architecture/decisions/DD-TEST-007-e2e-coverage-capture-standard.md)** - E2E coverage standard
```

---

## ✅ **Updated Recommendations**

### Priority 1: Immediate Actions (**CRITICAL** - Template 1.3.0 compliance)

1. **Update Tier Headers** (15 minutes)
   - Change from `(70% Coverage)` to `(70%+ BR Coverage | 70%+ Code Coverage)`
   - Update all 3 tier headers

2. **Add Cross-References** (10 minutes)
   - Link to Test Plan Best Practices
   - Link to NT Test Plan Example
   - Link to Template

3. **Add Current Test Status Section** (30 minutes)
   - Document 173 existing unit tests
   - Assessment per tier (what's needed?)
   - Pre-implementation baseline

4. **Add Test Outcomes by Tier** (20 minutes)
   - What each tier proves
   - What failure means
   - **NEW**: Code coverage column

5. **Add Infrastructure Setup Sections** (1 hour)
   - Integration tier setup (envtest + Data Storage)
   - E2E tier setup (Kind + Tekton + Data Storage)
   - Validation commands

6. **Add Pre/Post Comparison** (30 minutes)
   - Pre-implementation: 173 tests, 69.2% coverage, 85% confidence
   - Post-implementation: 173 tests, 75%+ coverage, 99% confidence
   - Confidence improvement justification

7. **Expand to Full V1.0 Maturity Scope** (2-3 hours)
   - Add all V1.0 maturity sections
   - Metrics, audit, shutdown, probes, predicates, events

**Total Immediate Effort**: 5-6 hours

---

### Priority 2: Follow Template 1.3.0 Best Practices (**MEDIUM**)

8. **Add Day-by-Day Timeline** (30 minutes)
   - Optional for simple features (<10 tests)
   - **MANDATORY** when expanding to complex integration/E2E work

9. **Use Inline Guidance Comments** (15 minutes per section)
   - Add `> **GUIDANCE**: [when to use this section]` notes
   - Help future teams understand template

---

## 📊 **Updated Estimated Effort Summary**

| Task Category | Estimated Time | Priority |
|---------------|----------------|----------|
| **Template 1.3.0 Compliance Updates** | 3 hours | P1 (CRITICAL) |
| **Full V1.0 Maturity Expansion** | 2-3 hours | P1 (CRITICAL) |
| **Best Practices Guidance** | 1 hour | P2 (MEDIUM) |
| **Test Implementation** | 6-10 days | P3 (after plan) |
| **E2E Coverage Instrumentation** | 1-2 days | P3 (after plan) |

---

## 🎯 **Success Criteria**

### Template 1.3.0 Compliance
- ✅ Tier headers use `(BR Coverage | Code Coverage)` notation
- ✅ Current Test Status section included
- ✅ Pre/Post Comparison section included
- ✅ Infrastructure Setup sections for Integration/E2E
- ✅ Test Outcomes by Tier with code coverage column
- ✅ Cross-references to Best Practices, NT example, Template
- ✅ Follows Template 1.3.0 structure
- ✅ All V1.0 maturity features covered

---

**Status**: 📋 **READY FOR TEAM REVIEW**
**Owner**: WE Team
**Next Milestone**: V1.0 Maturity Validation Complete


