# TRIAGE: SignalProcessing Controller - Rego/ConfigMap Evaluation Gap

**Date**: 2025-12-12 Morning
**Service**: SignalProcessing
**Status**: 🔴 **CRITICAL GAP** - Tests expect features controller doesn't implement
**Impact**: 23 of 71 integration tests failing (32%)

---

## 🎯 **ISSUE SUMMARY**

**Symptom**: 23 integration tests failing with business logic errors

**Root Cause**: **Controller implementation incomplete** - Missing Rego/ConfigMap evaluation

**Tests Expect** (from test code):
- ✅ ConfigMap-based environment classification (BR-SP-052)
- ✅ Rego policy evaluation for priority assignment (BR-SP-070)
- ✅ Rego policy evaluation for CustomLabels extraction (BR-SP-102)
- ✅ ConfigMap hot-reload (BR-SP-072)

**Controller Implements** (from controller code):
- ✅ Namespace label checking
- ✅ Signal label checking
- ❌ NO ConfigMap reading
- ❌ NO Rego policy evaluation
- ❌ NO hot-reload support

---

## 🔍 **EVIDENCE**

### **Controller Code** (signalprocessing_controller.go:620-649):

```go
func (r *SignalProcessingReconciler) classifyEnvironment(...) *EnvironmentClassification {
    result := &EnvironmentClassification{
        Environment:  "unknown",  // ← DEFAULT (no ConfigMap read)
        Confidence:   0.0,
        Source:       "default",
        ClassifiedAt: metav1.Now(),
    }

    // Check namespace labels (BR-SP-051)
    if k8sCtx != nil && k8sCtx.Namespace != nil {
        if env, ok := k8sCtx.Namespace.Labels["kubernaut.ai/environment"]; ok {
            result.Environment = env
            result.Confidence = 0.95
            result.Source = "namespace-labels"
            return result
        }
    }

    // Check signal labels fallback
    if signal != nil && signal.Labels != nil {
        if env, ok := signal.Labels["kubernaut.ai/environment"]; ok {
            result.Environment = env
            result.Confidence = 0.80
            result.Source = "signal-labels"
            return result
        }
    }

    return result  // ← Returns "unknown" (no ConfigMap/Rego)
}
```

**Missing**: ConfigMap reading, Rego policy evaluation

### **Test Expectation** (reconciler_integration_test.go:311):

```go
// Test creates namespace "staging-app-*" (no labels)
// Expects environment classification to return "staging" via ConfigMap/Rego
Expect(final.Status.EnvironmentClassification.Environment).To(Equal("staging"))

// FAILS: Gets "unknown" because controller doesn't read ConfigMap
```

---

## 📊 **FAILING TESTS BREAKDOWN**

### **By Root Cause**:

| Root Cause | Tests Affected | Examples |
|---|---|---|
| **No ConfigMap/Rego for Environment** | 10 tests | BR-SP-052 (ConfigMap fallback) |
| **No Rego for Priority** | 7 tests | BR-SP-070 (Rego priority) |
| **No Rego for CustomLabels** | 4 tests | BR-SP-102 (CustomLabels extraction) |
| **No ConfigMap Hot-Reload** | 3 tests | BR-SP-072 (Policy hot-reload) |
| **Test Resource Setup** | ~4 tests | BR-SP-100, BR-SP-101 (missing Pods/HPAs) |

**Note**: Some overlap - many tests exercise multiple features

### **By Business Requirement**:

| BR | Description | Status | Tests Failing |
|---|---|---|---|
| **BR-SP-052** | ConfigMap fallback classification | ❌ Not implemented | ~3 tests |
| **BR-SP-070** | Rego-based priority assignment | ❌ Not implemented | ~5 tests |
| **BR-SP-072** | Policy hot-reload | ❌ Not implemented | 3 tests |
| **BR-SP-102** | CustomLabels via Rego extraction | ❌ Not implemented | ~7 tests |
| **BR-SP-104** | System prefix filtering | ❌ Not implemented | ~2 tests |

---

## 🚨 **ARCHITECTURAL ASSESSMENT**

### **TDD Phase Analysis**:

**Current Controller State**: ✅ GREEN Phase (Minimal Implementation)
- Handles basic reconciliation loop
- Updates status through phases (Pending → Enriching → Classifying → Categorizing → Completed)
- Has audit client wired (BR-SP-090)
- **BUT**: Uses hardcoded/simplified logic (no Rego/ConfigMap)

**Tests Expect**: 🔴 REFACTOR Phase (Sophisticated Implementation)
- ConfigMap-based dynamic classification
- Rego policy evaluation
- Hot-reload support
- Complex business logic

**Gap**: Tests are **ahead** of implementation (expecting REFACTOR-level features in GREEN-phase controller)

---

## 💡 **OPTIONS**

### **Option A: Implement Rego/ConfigMap Evaluation in Controller** ⭐ RECOMMENDED
**Action**: Enhance controller to match test expectations

**Changes Required**:
1. Add Rego policy loading from ConfigMaps
2. Implement environment classification via Rego evaluation
3. Implement priority assignment via Rego evaluation
4. Implement CustomLabels extraction via Rego evaluation
5. Add ConfigMap watching for hot-reload (BR-SP-072)

**Effort**: 6-8 hours (significant REFACTOR work)

**Pros**:
- ✅ Implements missing business requirements (BR-SP-052, BR-SP-070, BR-SP-102, BR-SP-072)
- ✅ Tests pass (by design - TDD RED→GREEN→REFACTOR)
- ✅ Production-ready feature set

**Cons**:
- ⚠️ Large change (not a "test fix")
- ⚠️ Requires REFACTOR phase work

---

### **Option B: Downgrade Tests to Match Controller** ❌ NOT RECOMMENDED
**Action**: Update tests to expect hardcoded behavior

**Changes Required**:
1. Remove ConfigMap/Rego expectations from 23 tests
2. Update assertions to expect "unknown" or label-only classification
3. Skip or remove BR-SP-052, BR-SP-070, BR-SP-072, BR-SP-102 tests

**Effort**: 2-3 hours

**Pros**:
- ✅ Tests would pass quickly

**Cons**:
- ❌ Loses test coverage for important BRs
- ❌ Violates TDD (tests define contract, not implementation)
- ❌ Business requirements not met (BR-SP-052, BR-SP-070, BR-SP-102, BR-SP-072)

---

### **Option C: Mark Tests as Pending/Skipped** ❌ NOT RECOMMENDED
**Action**: Mark 23 tests as "Pending" until REFACTOR phase

**Effort**: 1 hour

**Pros**:
- ✅ Quick fix

**Cons**:
- ❌ Hides real issues
- ❌ Business requirements not validated
- ❌ Violates "NEVER use Skip()" principle

---

## 🎯 **RECOMMENDATION**

**Choose Option A: Implement Rego/ConfigMap Evaluation**

**Rationale**:
1. **TDD Compliance**: Tests define the contract (RED phase done)
2. **BR Coverage**: BR-SP-052, BR-SP-070, BR-SP-072, BR-SP-102 are V1.0 requirements
3. **Architecture**: Rego-based classification is core SP functionality
4. **Phase Progression**: Controller is in GREEN, ready for REFACTOR enhancements

**Implementation Path** (REFACTOR Phase):
1. Add Rego policy loader (reads environment.rego from ConfigMap)
2. Add Rego evaluator to `classifyEnvironment()` method
3. Add Rego evaluator to `assignPriority()` method  
4. Add Rego evaluator to `classifyBusiness()` method (for CustomLabels)
5. Add ConfigMap watcher for hot-reload (BR-SP-072)

**TDD Phase**: ✅ RED (tests written) → ✅ GREEN (basic controller) → 🟡 REFACTOR (Rego evaluation) ← WE ARE HERE

---

## 📋 **ALTERNATIVE: Hybrid Approach** (Quick Win)

If full Rego implementation is too large for now:

**Mini-Option**: **Implement ConfigMap Reading Only** (No Rego Engine)
- Read environment.rego ConfigMap
- Parse it manually for specific patterns (`startswith(namespace, "staging")`)
- Skip full Rego evaluation initially

**Effort**: 2-3 hours  
**Impact**: Fixes ~10 tests (environment classification only)  
**Trade-off**: Not the "proper" Rego evaluation, but unblocks tests

---

## 🚦 **USER DECISION REQUIRED**

Which option should I proceed with?

**A**: Implement full Rego/ConfigMap evaluation (6-8 hours, REFACTOR phase, production-ready)  
**B**: Implement ConfigMap reading only (2-3 hours, quick win, partial solution)  
**C**: Something else?

**Current Progress**:
- ✅ Infrastructure: Complete
- ✅ Architecture: Fixed (parent RR)
- ✅ Controller retry: Fixed
- 🟡 Business Logic: Missing Rego evaluation

---

## 📚 **RELATED DOCUMENTS**

- [SP_NIGHT_WORK_SUMMARY.md](./SP_NIGHT_WORK_SUMMARY.md) - Infrastructure work completed
- [MORNING_BRIEFING_SP.md](./MORNING_BRIEFING_SP.md) - Status as of morning
- [TRIAGE_SP_BUSINESS_LOGIC_FAILURES.md](./TRIAGE_SP_BUSINESS_LOGIC_FAILURES.md) - Original business logic triage (outdated - was status update conflicts, now Rego gap)

---

**Bottom Line**: The controller needs REFACTOR phase enhancements (Rego evaluation) to pass the tests. This is proper TDD progression: RED (tests) → GREEN (basic controller) → REFACTOR (sophisticated logic). We're at the REFACTOR step.

