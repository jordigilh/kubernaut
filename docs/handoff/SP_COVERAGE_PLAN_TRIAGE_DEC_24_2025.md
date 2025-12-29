# SignalProcessing Coverage Extension Plan - Guidelines Triage

**Document ID**: `SP_COVERAGE_PLAN_TRIAGE_DEC_24_2025`
**Status**: 🔍 **TRIAGE ASSESSMENT**
**Created**: December 24, 2025
**Reviewed Against**: `docs/development/business-requirements/TESTING_GUIDELINES.md` (v2.4.0)
**Plan Under Review**: `docs/handoff/SP_INTEGRATION_COVERAGE_EXTENSION_PLAN_DEC_24_2025.md`

---

## 🎯 **Executive Summary**

### **Verdict**: ⚠️ **PLAN NEEDS REVISION**

**Key Finding**: SignalProcessing integration tests already **EXCEED** the 50% code coverage target (currently at 53.2%). The proposed plan to extend coverage to 68% **CONFLICTS** with `TESTING_GUIDELINES.md` defense-in-depth strategy.

**Critical Issues Identified**:
1. ❌ **Misaligned Goal**: Plan targets 68% integration coverage when guideline is 50%
2. ❌ **Defense-in-Depth Violation**: Should prioritize **unit tests** for uncovered code (70% target)
3. ⚠️ **BR Coverage Not Assessed**: Plan focuses on code coverage, not BR coverage
4. ⚠️ **V1.0 Maturity Gaps**: Plan doesn't validate maturity feature coverage

---

## 📊 **Coverage Target Alignment Analysis**

### **TESTING_GUIDELINES.md Targets** (v2.4.0, lines 49-82)

| Tier | Code Coverage Target | BR Coverage Target | Purpose |
|------|---------------------|-------------------|---------|
| **Unit** | **70%+** | **70%+ of ALL BRs** | Algorithm correctness, edge cases |
| **Integration** | **50%** | **>50% of ALL BRs** | Cross-component flows, CRD operations |
| **E2E** | **50%** | **<10% BR coverage** | Full stack validation |

**Key Insight from Guidelines**:
> "With 70%/50%/50% code coverage targets, **50%+ of codebase is tested in ALL 3 tiers** - ensuring bugs must slip through multiple defense layers to reach production."

### **Current SignalProcessing Status**

| Tier | Code Coverage | Status vs Target | Assessment |
|------|---------------|------------------|------------|
| **Unit** | Not measured | Unknown | ⚠️ **MEASURE FIRST** |
| **Integration** | **53.2%** | ✅ **EXCEEDS 50%** | Target met |
| **E2E** | Not measured | Unknown | ⚠️ **MEASURE FIRST** |

### **Proposed Plan Status**

| Element | Plan Goal | Guidelines Target | Alignment |
|---------|-----------|------------------|-----------|
| Overall Integration Coverage | 68% | 50% | ❌ **OVER-TARGET** (+18%) |
| Detection Module | 65% | No module-specific target | ⚠️ **NOT IN GUIDELINES** |
| Classifier Module | 65% | No module-specific target | ⚠️ **NOT IN GUIDELINES** |
| Enricher Module | 60% | No module-specific target | ⚠️ **NOT IN GUIDELINES** |

---

## 🚨 **Critical Misalignment: Defense-in-Depth Strategy**

### **The Problem**

The plan identifies uncovered functions:
- `DetectLabels()` - 0% coverage
- `Classify()` - 0% coverage
- `BuildDegradedContext()` - 0% coverage

**Plan's Approach**: Add integration tests for these functions

**Guidelines' Approach** (lines 64-81):
```
| Tier | Code Coverage Target | What It Validates |
|------|---------------------|-------------------|
| Unit | 70%+ | Algorithm correctness, edge cases, error handling |
| Integration | 50% | Cross-component flows, CRD operations, real K8s API |
| E2E | 50% | Full stack: main.go, reconciliation, business logic |
```

### **Defense-in-Depth Violation**

**Issue**: Functions with 0% coverage should be covered by **UNIT TESTS FIRST**, not integration tests.

**Correct Defense-in-Depth Sequence**:
1. **Unit Test**: Verify `DetectLabels()` correctly parses ArgoCD annotations (70% target)
2. **Integration Test**: Verify detection works with real K8s resources (50% target - already met)
3. **E2E Test**: Verify detection in deployed controller (50% target)

**Current Plan's Sequence**:
1. ❌ Skip unit tests entirely
2. ✅ Add integration tests (over-covers integration tier)
3. ❌ Skip E2E validation

**Result**: Creates a **SINGLE LAYER** defense (integration only), not **THREE LAYERS** (unit + integration + E2E).

---

## 📋 **Business Requirement Coverage Assessment**

### **Missing Analysis: BR Coverage vs Code Coverage**

**TESTING_GUIDELINES.md Distinction** (lines 53-61):

| Coverage Type | Meaning | Target |
|--------------|---------|--------|
| **BR Coverage** | Which business requirements are tested? | Overlapping across tiers |
| **Code Coverage** | Which lines of code are executed? | Cumulative across tiers |

**Plan's Focus**: Code coverage (53.2% → 68%)
**Missing**: BR coverage assessment (>50% of all BRs in integration tier?)

### **BR Coverage Gaps**

The plan maps to these BRs:
- ✅ **BR-SP-001**: K8s Context Enrichment (Test 3.1)
- ✅ **BR-SP-002**: Business Classification (Test 2.1)
- ✅ **BR-SP-080**: Classification Source Tracking (Test 2.1)
- ✅ **BR-SP-101**: DetectedLabels Auto-Detection (Tests 1.1-1.4)
- ✅ **BR-SP-103**: FailedDetections Tracking (Test 1.5)

**But**: Are there **other BRs** that are:
1. Not covered by ANY integration tests?
2. More critical than improving Detection module coverage?

**Recommendation**: Run BR coverage analysis BEFORE adding more code coverage.

---

## ✅ **Compliance Assessment: Mandatory Patterns**

### **time.Sleep() Prohibition** (Guidelines lines 573-853)

**Status**: ✅ **COMPLIANT**

All test code in the plan uses `Eventually()` for asynchronous operations:
```go
// ✅ Correct pattern from plan
Eventually(func() bool {
    sp := &signalprocessingv1alpha1.SignalProcessing{}
    if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(spCR), sp); err != nil {
        return false
    }
    return sp.Status.Phase == signalprocessingv1alpha1.PhaseCompleted
}, 30*time.Second, 1*time.Second).Should(BeTrue())
```

**No violations found** ✅

---

### **Skip() Prohibition** (Guidelines lines 855-985)

**Status**: ✅ **COMPLIANT**

No `Skip()` calls found in test plan. All tests use `Fail()` pattern for missing dependencies ✅

---

### **Integration Test Infrastructure** (Guidelines lines 996-1234)

**Status**: ✅ **COMPLIANT**

- ✅ Uses `envtest` for K8s API
- ✅ Creates unique namespaces per test
- ✅ Cleans up in `AfterEach()`
- ✅ Uses `Eventually()` for waits
- ✅ No race conditions in parallel execution

---

## 🏗️ **V1.0 Service Maturity Requirements Assessment**

### **Guidelines Section** (lines 1368-1804)

**Mandatory Testing Requirements**:
1. **Metrics Testing**: Integration + E2E tests for all metrics
2. **Audit Trace Testing**: Integration tests with OpenAPI client validation
3. **EventRecorder Testing**: E2E tests for Kubernetes Events
4. **Graceful Shutdown**: Integration tests for flush behavior
5. **Health Probes**: E2E tests for probe endpoints

### **Plan's Coverage of Maturity Features**

| Feature | Current Coverage | Plan Addresses | Gap |
|---------|-----------------|----------------|-----|
| **Metrics** | Already tested (Audit 72.6%) | ❌ Not in plan | ✅ Already covered |
| **Audit Traces** | Already tested (audit_integration_test.go) | ❌ Not in plan | ✅ Already covered |
| **EventRecorder** | Unknown | ❌ Not in plan | ⚠️ **ASSESS** |
| **Graceful Shutdown** | Unknown | ❌ Not in plan | ⚠️ **ASSESS** |
| **Health Probes** | Unknown | ❌ Not in plan | ⚠️ **ASSESS** |

**Finding**: Plan focuses on **business logic coverage** but ignores **maturity feature validation**.

**Risk**: Service may not be V1.0 production-ready even with 68% integration coverage.

---

## 🎯 **Module-Specific Coverage Analysis**

### **Is Module-Specific Coverage a Valid Goal?**

**Plan's Argument**: Detection (27.3%), Classifier (41.6%), Enricher (44.0%) are below 50%

**Guidelines' Position**: No module-specific targets defined. Only tier-level targets (70%/50%/50%).

**Defense-in-Depth Perspective**:
```
Question: If Detection module has only 27.3% integration coverage,
          does that weaken defense-in-depth?

Answer: Only if Detection module also has LOW UNIT COVERAGE.

Example:
- Detection Unit Coverage: 85% → Strong first layer ✅
- Detection Integration Coverage: 27.3% → Acceptable (overall 53.2% meets 50%)
- Detection E2E Coverage: 50% → Strong third layer ✅

Result: 3-layer defense is intact, even with low module-specific integration coverage.

BUT IF:
- Detection Unit Coverage: 30% → Weak first layer ❌
- Detection Integration Coverage: 27.3% → Weak second layer ❌
- Detection E2E Coverage: 50% → Only strong layer

Result: Bug can slip through 2 weak layers!
```

**Critical Question**: **What is Detection module's UNIT TEST coverage?**

---

## 📊 **Recommended Action Plan - REVISED APPROACH**

### **PHASE 0: Measure Current State** (HIGHEST PRIORITY)

**Before ANY new tests**, measure:

```bash
# 1. Unit test coverage (tier-level)
go test ./pkg/signalprocessing/... -coverprofile=unit-coverage.out
go tool cover -func=unit-coverage.out | tail -1

# 2. Unit test coverage (module-level)
go test ./pkg/signalprocessing/detection/... -coverprofile=detection-unit.out
go test ./pkg/signalprocessing/classifier/... -coverprofile=classifier-unit.out
go test ./pkg/signalprocessing/enricher/... -coverprofile=enricher-unit.out

# 3. E2E coverage (if DD-TEST-007 implemented)
# See: docs/architecture/decisions/DD-TEST-007-e2e-coverage-capture-standard.md

# 4. BR coverage analysis
# Manual: Map all BRs to existing tests
```

**Output**: 3-tier coverage matrix
```
| Module | Unit | Integration | E2E | Defense Status |
|--------|------|-------------|-----|----------------|
| Detection | ???% | 27.3% | ???% | UNKNOWN |
| Classifier | ???% | 41.6% | ???% | UNKNOWN |
| Enricher | ???% | 44.0% | ???% | UNKNOWN |
```

---

### **PHASE 1: Address Unit Test Gaps** (If unit < 70%)

**IF Detection unit coverage < 70%**, prioritize unit tests:

**Example Unit Tests Needed**:
```go
// pkg/signalprocessing/detection/labels_test.go
var _ = Describe("LabelDetector Unit Tests", func() {
    Context("detectGitOps", func() {
        It("should detect ArgoCD from pod annotations", func() {
            k8sCtx := &sharedtypes.KubernetesContext{
                PodDetails: &sharedtypes.PodContext{
                    Annotations: map[string]string{
                        "argocd.argoproj.io/instance": "my-app",
                    },
                },
            }
            result := &sharedtypes.DetectedLabels{}

            detector := NewLabelDetector(logger)
            detector.detectGitOps(k8sCtx, result)

            Expect(result.GitOpsManaged).To(BeTrue())
            Expect(result.GitOpsTool).To(Equal("argocd"))
        })
    })
})
```

**Why Unit First?**:
- Faster execution (<100ms per test)
- No external dependencies (no K8s API)
- Tests algorithm correctness directly
- Establishes **first layer of defense**

**Effort**: 6-8 hours for Detection/Classifier/Enricher unit tests
**Coverage Gain**: Detection 27.3% → 70%+ (**UNIT tier**)

---

### **PHASE 2: Validate Integration Coverage Balance** (If unit ≥ 70%)

**IF unit coverage ≥ 70%**, then assess:

**Question 1**: Are critical **cross-component flows** tested?
- Detection → Classifier → Enricher integration?
- Real K8s API interactions?
- CRD reconciliation loops?

**Question 2**: Are **>50% of BRs** covered in integration tier?
- Map all BRs to integration tests
- Identify gaps in BR coverage

**IF** critical flows or BRs are missing → Add integration tests (from original plan)
**IF** coverage is balanced → Move to E2E tests

---

### **PHASE 3: E2E Coverage Validation** (If DD-TEST-007 implemented)

**Check**: Does E2E coverage reach 50% target?

**IF E2E < 50%**:
1. Implement DD-TEST-007 (E2E coverage capture)
2. Measure current E2E coverage
3. Add E2E tests for critical paths:
   - Full reconciliation loop (Pending → Completed)
   - Deployed controller metrics
   - Audit event emission

---

### **PHASE 4: V1.0 Maturity Feature Validation**

**Mandatory Tests** (per TESTING_GUIDELINES.md lines 1368-1804):

```bash
# Check maturity feature coverage
grep -r "EventRecorder" test/integration/signalprocessing/ || echo "❌ Missing EventRecorder tests"
grep -r "graceful shutdown" test/integration/signalprocessing/ || echo "❌ Missing shutdown tests"
grep -r "/health" test/e2e/signalprocessing/ || echo "❌ Missing health probe tests"
```

**Add missing maturity tests**:
- EventRecorder E2E tests
- Graceful shutdown integration tests
- Health probe E2E tests

---

## 🚨 **Critical Recommendations**

### **1. STOP Current Plan Implementation**

**Reason**: Risk of over-testing integration tier while unit tier has gaps.

**Action**: Measure unit coverage BEFORE proceeding.

---

### **2. Prioritize Defense-in-Depth Balance**

**Goal**: Ensure **50%+ of codebase tested in ALL 3 tiers**

**Current Risk**:
```
Detection Module Example:
- Unit: ???% (UNKNOWN - could be 30%)
- Integration: 27.3% (LOW)
- E2E: ???% (UNKNOWN - could be 20%)

Result: Bug can slip through 2-3 weak layers!
```

**Correct Approach**:
```
Detection Module Goal:
- Unit: 70%+ (STRONG FIRST LAYER)
- Integration: 50%+ (STRONG SECOND LAYER)
- E2E: 50%+ (STRONG THIRD LAYER)

Result: Bug must slip through 3 strong layers!
```

---

### **3. Assess BR Coverage Gaps**

**Question**: What **business requirements** are not tested in integration tier?

**Method**:
```bash
# 1. List all BRs
grep -E "^### BR-SP-" docs/services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md

# 2. Map to integration tests
for br in $(grep -oE "BR-SP-[0-9]+" docs/services/.../BUSINESS_REQUIREMENTS.md | sort -u); do
    echo -n "$br: "
    grep -r "$br" test/integration/signalprocessing/ --include="*_test.go" | wc -l
done

# 3. Identify BRs with 0 integration tests
```

**Priority**: BR coverage gaps > code coverage gaps

---

### **4. Validate V1.0 Maturity Compliance**

**Checklist** (from TESTING_GUIDELINES.md):
- [ ] All metrics have integration + E2E tests
- [ ] All audit traces validated via OpenAPI client
- [ ] EventRecorder E2E tests exist
- [ ] Graceful shutdown integration tests exist
- [ ] Health probe E2E tests exist

**IF any box is unchecked** → Add maturity tests BEFORE business logic tests

---

## 📋 **Revised Test Plan Proposal**

### **Option A: Measure-First Approach** (RECOMMENDED)

**Week 1: Measurement**
- Day 1: Measure unit coverage (Detection, Classifier, Enricher)
- Day 2: Measure E2E coverage (if DD-TEST-007 implemented)
- Day 3: Perform BR coverage analysis
- Day 4: Create defense-in-depth matrix
- Day 5: Present findings + revised plan

**Week 2-3: Targeted Coverage Extension**
- Prioritize tier with biggest gaps
- Balance coverage across all 3 tiers
- Validate V1.0 maturity features

**Deliverable**: Evidence-based test plan with 3-tier balance

---

### **Option B: Unit-First Approach** (IF unit coverage < 70%)

**Week 1: Unit Test Extension**
- Day 1-2: Detection unit tests (detectGitOps, detectPDB, detectHPA, etc.)
- Day 3: Classifier unit tests (Classify, classifyFromLabels, etc.)
- Day 4: Enricher unit tests (BuildDegradedContext, ValidateContextSize)
- Day 5: Measure new unit coverage

**Week 2: Integration Balance**
- Day 6-8: Add integration tests ONLY for cross-component flows
- Day 9-10: Validate defense-in-depth balance

**Deliverable**: 70%+ unit coverage, balanced 3-tier defense

---

### **Option C: V1.0 Maturity-First Approach** (IF maturity gaps exist)

**Week 1: Maturity Feature Validation**
- Day 1-2: Add EventRecorder E2E tests
- Day 3: Add graceful shutdown integration tests
- Day 4: Add health probe E2E tests
- Day 5: Metrics + audit validation

**Week 2: Business Logic Coverage** (if time permits)
- Proceed with original plan's tests

**Deliverable**: V1.0 production-ready service

---

## 🎯 **Decision Matrix for SP Team**

| Question | Answer | Recommended Approach |
|----------|--------|---------------------|
| Is unit coverage < 70%? | **YES** | **Option B** (Unit-First) |
| Is unit coverage < 70%? | **NO** | Continue below... |
| Are V1.0 maturity tests missing? | **YES** | **Option C** (Maturity-First) |
| Are V1.0 maturity tests missing? | **NO** | Continue below... |
| Are >50% of BRs covered in integration? | **NO** | **Revised Plan** (BR-focused) |
| Are >50% of BRs covered in integration? | **YES** | **Option A** (Measure-First) |

---

## 📊 **Risk Assessment**

### **Risk: Proceeding with Current Plan**

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| **Over-test integration tier** | HIGH | Medium | Measure unit coverage first |
| **Under-test unit tier** | MEDIUM | HIGH | Prioritize unit tests |
| **Miss V1.0 maturity features** | MEDIUM | HIGH | Validate maturity checklist |
| **Weak defense-in-depth** | HIGH | HIGH | Balance all 3 tiers |
| **BR coverage gaps** | MEDIUM | Medium | Perform BR analysis |

**Overall Risk Level**: 🔴 **HIGH** - Plan may create imbalanced test suite

---

## ✅ **Acceptance Criteria for Revised Plan**

Before implementing ANY new tests:

- [ ] **Unit coverage measured** for Detection, Classifier, Enricher
- [ ] **E2E coverage measured** (if DD-TEST-007 exists)
- [ ] **BR coverage analysis completed** (>50% of BRs in integration?)
- [ ] **Defense-in-depth matrix created** (module × tier coverage)
- [ ] **V1.0 maturity checklist validated** (all features tested?)
- [ ] **Tier priority determined** (unit vs integration vs E2E focus)
- [ ] **Test plan revised** based on evidence, not assumptions

**Only proceed when ALL boxes are checked** ✅

---

## 🔗 **References**

### **Authoritative Documents**
- **TESTING_GUIDELINES.md** (v2.4.0) - Defense-in-depth strategy (lines 49-82)
- **TESTING_GUIDELINES.md** - V1.0 Maturity Requirements (lines 1368-1804)
- **TESTING_GUIDELINES.md** - time.Sleep() prohibition (lines 573-853)
- **TESTING_GUIDELINES.md** - Skip() prohibition (lines 855-985)

### **Related Plans**
- **SP_INTEGRATION_COVERAGE_EXTENSION_PLAN_DEC_24_2025.md** - Original plan under review
- **SP_INTEGRATION_TEST_COVERAGE_DEC_24_2025.md** - Current coverage baseline (53.2%)

### **Business Requirements**
- **BUSINESS_REQUIREMENTS.md** - All BR-SP-XXX definitions

---

## 📝 **Final Recommendation**

### **Verdict**: ⚠️ **PAUSE CURRENT PLAN - MEASURE FIRST**

**Rationale**:
1. ✅ Integration coverage (53.2%) already **EXCEEDS** 50% target
2. ❌ Unit coverage is **UNMEASURED** - could be < 70%
3. ❌ E2E coverage is **UNMEASURED** - could be < 50%
4. ⚠️ BR coverage is **UNANALYZED** - may have gaps
5. ⚠️ V1.0 maturity features are **UNVALIDATED**

**Recommended Next Step**:
```bash
# IMMEDIATE ACTION: Measure unit coverage
make test-unit-signalprocessing
go tool cover -func=unit-coverage.out

# IF unit < 70% → Implement Option B (Unit-First)
# IF unit ≥ 70% AND maturity gaps → Implement Option C (Maturity-First)
# IF unit ≥ 70% AND maturity OK → Implement Option A (Measure-First)
```

**Expected Outcome**: Evidence-based test plan that balances all 3 tiers per defense-in-depth strategy.

---

**Document Status**: 🔍 **TRIAGE COMPLETE**
**Next Action**: Present findings to SP team for decision
**Decision Needed**: Which option (A/B/C) to pursue?
**Owner**: SignalProcessing Team Lead

---

**END OF TRIAGE ASSESSMENT**


