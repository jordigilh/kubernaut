# P4: Day 4 Edge Case Tests - Implementation Plan

**Date**: October 28, 2025
**Status**: 🚀 **IN PROGRESS**
**Duration**: 3-4 hours
**Target**: 8 edge case tests for Day 4 (Environment + Priority + Remediation Path)

---

## 🎯 **Objective**

Create comprehensive edge case tests for Day 4 components:
1. **Priority Engine** (`pkg/gateway/processing/priority.go`)
2. **Remediation Path Decider** (`pkg/gateway/processing/remediation_path.go`)

---

## 📋 **Day 4 Components Analysis**

### **Priority Engine**
- **Purpose**: Assign priority (P0/P1/P2/P3) based on severity + environment
- **Methods**:
  - `NewPriorityEngine()` - Fallback table only
  - `NewPriorityEngineWithRego()` - Rego policy + fallback
  - `Assign(ctx, severity, environment)` - Returns priority
- **Fallback Table**:
  ```
  ┌──────────┬────────────┬─────────┬─────────────┬──────────────┐
  │ Severity │ production │ staging │ development │ unknown (*)  │
  ├──────────┼────────────┼─────────┼─────────────┼──────────────┤
  │ critical │     P0     │   P1    │     P2      │     P1       │
  │ warning  │     P1     │   P2    │     P2      │     P2       │
  │ info     │     P2     │   P2    │     P2      │     P3       │
  └──────────┴────────────┴─────────┴─────────────┴──────────────┘
  ```
- **Rego Support**: Optional Rego policy evaluation with fallback

### **Remediation Path Decider**
- **Purpose**: Determine remediation strategy based on environment + priority
- **Methods**:
  - `NewRemediationPathDecider()` - Fallback table only
  - `NewRemediationPathDeciderWithRego()` - Rego policy + fallback
  - `DeterminePath(ctx, signalCtx)` - Returns path
- **Fallback Table**:
  ```
  ┌─────────────┬─────────────┬──────────┬──────────────┬────────┐
  │ Environment │     P0      │    P1    │      P2      │  P99+  │
  ├─────────────┼─────────────┼──────────┼──────────────┼────────┤
  │ production  │ aggressive  │ conserv  │ conservative │ manual │
  │ staging     │  moderate   │ moderate │    manual    │ manual │
  │ development │ aggressive  │ moderate │    manual    │ manual │
  │ unknown     │ conservative│ conserv  │ conservative │ manual │
  │ * (catch-all)│ moderate   │ moderate │ conservative │ manual │
  └─────────────┴─────────────┴──────────┴──────────────┴────────┘
  ```
- **Rego Support**: Optional Rego policy evaluation with fallback
- **Caching**: Caches decisions to avoid redundant Rego evaluations

---

## 🧪 **Edge Case Test Plan** (8 tests)

### **Category 1: Priority Engine Edge Cases** (4 tests)

#### **Test 1: Catch-All Environment Matching**
**Business Outcome**: Unknown environments (canary, qa-eu, blue, green) get sensible priorities
**Scenario**: Priority assignment for custom environment names
**Test Cases**:
- `critical` + `canary` → `P1` (catch-all)
- `warning` + `qa-eu` → `P2` (catch-all)
- `info` + `blue-green` → `P3` (catch-all)
**BR**: BR-GATEWAY-013 (Priority assignment)

#### **Test 2: Unknown Severity Fallback**
**Business Outcome**: Invalid/unknown severities default to safe priority (P2)
**Scenario**: Graceful degradation for malformed severity values
**Test Cases**:
- `unknown-severity` + `production` → `P2` (safe default)
- `invalid` + `staging` → `P2` (safe default)
- Empty severity + `development` → `P2` (safe default)
**BR**: BR-GATEWAY-013 (Priority assignment graceful degradation)

#### **Test 3: Rego Evaluation Fallback**
**Business Outcome**: System continues working when Rego fails
**Scenario**: Rego policy evaluation error triggers fallback table
**Test Cases**:
- Rego returns error → fallback table used
- Rego returns invalid priority → fallback table used
- Rego timeout → fallback table used
**BR**: BR-GATEWAY-013 (Rego graceful degradation)

#### **Test 4: Case Sensitivity**
**Business Outcome**: Priority assignment is case-insensitive for robustness
**Scenario**: Mixed-case severity and environment values
**Test Cases**:
- `Critical` + `Production` → normalized to lowercase
- `WARNING` + `STAGING` → normalized to lowercase
- `InFo` + `DeVeLoPmEnT` → normalized to lowercase
**BR**: BR-GATEWAY-013 (Priority assignment robustness)

---

### **Category 2: Remediation Path Decider Edge Cases** (4 tests)

#### **Test 5: Catch-All Environment Matching**
**Business Outcome**: Custom environments (canary, qa-eu) get sensible remediation paths
**Scenario**: Remediation path for custom environment names
**Test Cases**:
- `canary` + `P0` → `moderate` (catch-all)
- `qa-eu` + `P1` → `moderate` (catch-all)
- `blue-green` + `P2` → `conservative` (catch-all)
**BR**: BR-GATEWAY-014 (Remediation path decision)

#### **Test 6: Invalid Priority Handling**
**Business Outcome**: Invalid priorities default to safe remediation path (manual)
**Scenario**: Graceful degradation for malformed priority values
**Test Cases**:
- `production` + `P99` → `manual` (safe default)
- `staging` + `invalid` → `manual` (safe default)
- `development` + empty priority → `manual` (safe default)
**BR**: BR-GATEWAY-014 (Remediation path graceful degradation)

#### **Test 7: Rego Evaluation Fallback**
**Business Outcome**: System continues working when Rego fails
**Scenario**: Rego policy evaluation error triggers fallback table
**Test Cases**:
- Rego returns error → fallback table used
- Rego returns invalid path → fallback table used
- Rego timeout → fallback table used
**BR**: BR-GATEWAY-014 (Rego graceful degradation)

#### **Test 8: Cache Consistency**
**Business Outcome**: Cached decisions are consistent across multiple calls
**Scenario**: Cache returns same result for identical inputs
**Test Cases**:
- First call: `production` + `P0` → `aggressive` (cache miss)
- Second call: `production` + `P0` → `aggressive` (cache hit)
- Third call: `production` + `P0` → `aggressive` (cache hit)
- Verify cache hit count increases
**BR**: BR-GATEWAY-014 (Performance optimization)

---

## 📁 **File Structure**

### **Test File**
```
test/unit/gateway/processing/priority_remediation_edge_cases_test.go
```

### **Test Suite**
```go
var _ = Describe("Priority Engine + Remediation Path Decider - Edge Cases", func() {
    // Category 1: Priority Engine Edge Cases
    Context("Priority Engine - Catch-All Environment Matching", func() { ... })
    Context("Priority Engine - Unknown Severity Fallback", func() { ... })
    Context("Priority Engine - Rego Evaluation Fallback", func() { ... })
    Context("Priority Engine - Case Sensitivity", func() { ... })

    // Category 2: Remediation Path Decider Edge Cases
    Context("Remediation Path Decider - Catch-All Environment Matching", func() { ... })
    Context("Remediation Path Decider - Invalid Priority Handling", func() { ... })
    Context("Remediation Path Decider - Rego Evaluation Fallback", func() { ... })
    Context("Remediation Path Decider - Cache Consistency", func() { ... })
})
```

---

## 🛡️ **Defense-in-Depth Strategy**

### **Unit Tier** (This Work)
- 8 edge case tests
- Fast (<2s), deterministic
- Mocked Rego evaluators
- 100% business logic coverage

### **Integration Tier** (Future Work)
5 integration tests planned:
1. Real Rego policy evaluation with OPA
2. Rego policy hot-reload from ConfigMap
3. Concurrent priority assignment with caching
4. Rego policy syntax errors
5. Cross-component integration (Priority → Remediation Path)

**Value**: Catches differences between mocked and real Rego behavior

---

## 📊 **Success Criteria**

### **Test Coverage**
- ✅ 8 edge case tests created
- ✅ 100% pass rate
- ✅ All edge cases covered

### **Business Requirements**
- ✅ BR-GATEWAY-013: Priority assignment validated
- ✅ BR-GATEWAY-014: Remediation path decision validated

### **Confidence**
- ✅ Day 4 confidence: 100%
- ✅ Edge case coverage: 100%
- ✅ Graceful degradation: 100%

---

## 🚀 **Implementation Steps**

### **Step 1: Create Test File** (15 min)
- Create `test/unit/gateway/processing/priority_remediation_edge_cases_test.go`
- Add test suite setup
- Add imports and test infrastructure

### **Step 2: Implement Priority Engine Tests** (1-1.5h)
- Test 1: Catch-all environment matching
- Test 2: Unknown severity fallback
- Test 3: Rego evaluation fallback
- Test 4: Case sensitivity

### **Step 3: Implement Remediation Path Tests** (1-1.5h)
- Test 5: Catch-all environment matching
- Test 6: Invalid priority handling
- Test 7: Rego evaluation fallback
- Test 8: Cache consistency

### **Step 4: Run Tests and Fix Issues** (30 min)
- Run all 8 tests
- Fix any failures
- Verify 100% pass rate

### **Step 5: Documentation** (15 min)
- Update P4 status
- Create completion report
- Update TODO list

---

## 📝 **Risk Assessment**

### **Low Risk**
- ✅ Components already exist and compile
- ✅ Similar patterns to P3 (deduplication, storm detection)
- ✅ Clear edge cases identified

### **Medium Risk**
- ⚠️ Rego mock behavior may differ from real OPA
- ⚠️ Cache testing may require careful synchronization
- ⚠️ Case sensitivity normalization may not exist yet

### **Mitigation**
- Use simple mock Rego evaluators
- Test cache with sequential calls (no concurrency)
- Add case normalization if missing (implementation bug)

---

## 🎯 **Expected Outcomes**

### **Immediate**
- 8 new edge case tests
- 100% pass rate
- Day 4 confidence: 100%

### **Long-Term**
- Robust priority assignment
- Reliable remediation path decisions
- Production-ready Day 4 components

---

**Status**: 🚀 **READY TO IMPLEMENT**
**Next Step**: Create test file and start with Priority Engine tests

