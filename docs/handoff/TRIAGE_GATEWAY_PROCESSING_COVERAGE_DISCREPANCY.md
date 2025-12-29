# TRIAGE: Gateway Processing Coverage Discrepancy

**Date**: 2025-12-13
**Issue**: Unit + Integration test coverage claims don't match actual numbers
**Status**: 🔍 **TRIAGING**

---

## 🚨 **Problem Statement**

**Claimed**:
- Unit tests: 80.4% coverage
- Integration tests: Cover ShouldDeduplicate PRIMARY path
- Combined: ~95% coverage target

**Actual Numbers**:
- **Unit tests alone**: 80.4% ✅
- **Integration tests alone**: 6.1% ⚠️ (only tests ShouldDeduplicate)
- **ShouldDeduplicate from integration**: 55.6% (not 100%)
- **Combined coverage**: TBD (checking...)

---

## 🔍 **Coverage Breakdown**

### **Unit Tests** (test/unit/gateway/processing/)
```
Ran 78 of 78 Specs in 3.894 seconds
Coverage: 80.4% of statements in ./pkg/gateway/processing/...
```

### **Integration Tests** (test/integration/gateway/processing/)
```
Ran 8 of 8 Specs in 9.141 seconds
Coverage: 6.1% of statements in ./pkg/gateway/processing/...
ShouldDeduplicate: 55.6%
```

### **What Integration Tests Actually Cover**
- ✅ ShouldDeduplicate: 55.6% (field selector path)
- ❌ Everything else: ~0% (integration tests only test one function)

---

## 🎯 **Root Cause Analysis**

### **Issue 1: Integration Tests Only Test One Function**
- Integration tests test **ShouldDeduplicate** exclusively
- They don't test CreateRemediationRequest, buildProviderData, etc.
- Result: Only 6.1% of Processing package covered by integration tests

### **Issue 2: ShouldDeduplicate Coverage Split**
- **Unit tests**: Test fallback path (when field selector fails)
- **Integration tests**: Test PRIMARY path (field selector queries) - 55.6%
- **Combined**: Likely ~80-90% of ShouldDeduplicate, not 100%

### **Issue 3: Overlapping Coverage**
- Unit and integration coverage overlap
- Can't just add 80.4% + 6.1% = 86.5%
- Need to measure COMBINED coverage

---

## 📊 **Expected vs Actual**

| Metric | Expected | Actual | Gap |
|--------|----------|--------|-----|
| Unit Coverage | 80.4% | 80.4% | ✅ Match |
| Integration Coverage | "PRIMARY path" | 6.1% total | ⚠️ Misleading claim |
| ShouldDeduplicate (integration) | ~100% | 55.6% | ❌ Gap |
| Combined Coverage | ~95% | TBD | ❓ Unknown |

---

## 🔍 **What Went Wrong**

### **Claim**: "Integration tests cover PRIMARY path (field selectors)"
- **Reality**: Integration tests cover 55.6% of ShouldDeduplicate
- **Gap**: 44.4% of ShouldDeduplicate still not covered

### **Claim**: "Combined coverage ~95%"
- **Reality**: Need to check actual combined coverage
- **Issue**: Made assumption without measuring

### **Claim**: "Processing package achieves 95% coverage"
- **Reality**: 80.4% unit, unknown combined
- **Issue**: Never verified the 95% target was reached

---

## 📋 **Action Items**

1. ✅ Measure COMBINED coverage (unit + integration)
2. ⏳ Identify what's NOT covered in ShouldDeduplicate (44.4% gap)
3. ⏳ Determine if 95% target is realistic or if 80-85% is acceptable
4. ⏳ Update documentation with ACCURATE coverage numbers
5. ⏳ Provide honest assessment of coverage gaps

---

## 🎯 **Honest Assessment Needed**

### **Questions to Answer**:
1. What is the ACTUAL combined coverage?
2. What parts of ShouldDeduplicate are NOT covered?
3. Is 95% coverage achievable or should target be adjusted?
4. Are the integration tests providing value beyond unit tests?

---

**Status**: Investigation in progress...

