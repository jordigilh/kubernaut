# DataStorage Label Weights Test Coverage Analysis

**Date**: December 17, 2025
**Status**: ⚠️ **CRITICAL FINDING** - `scoring/weights.go` is **DEAD CODE**
**Action**: Document and decide on path forward

---

## 🔍 **Key Finding: Weights Package is NOT Used**

### **What We Discovered**

1. ✅ **Label weights ARE being applied** in workflow search SQL (CONFIRMED)
2. ❌ **`pkg/datastorage/scoring/weights.go` is NOT being used** (DEAD CODE)
3. ⚠️ **Weights are hardcoded** in `pkg/datastorage/repository/workflow/search.go`

---

## 📊 **Evidence**

### **1. Weights ARE Applied in SQL Generation** ✅

**File**: `pkg/datastorage/repository/workflow/search.go`

```go
// Line 417-426: Weights hardcoded inline
weights := map[string]float64{
    "git_ops_managed":  0.10,
    "git_ops_tool":     0.10,
    "pdb_protected":    0.05,
    "service_mesh":     0.05,
    "network_isolated": 0.03,
    "helm_managed":     0.02,
    "stateful":         0.02,
    "hpa_enabled":      0.02,
}

// Line 433: Weight used in SQL generation
weight := weights["git_ops_managed"]
boostCases = append(boostCases,
    fmt.Sprintf("CASE WHEN detected_labels->>'gitOpsManaged' = 'true' THEN %.2f ELSE 0.0 END", weight))
```

**SQL Generated**:
```sql
-- detected_label_boost calculation
CASE WHEN detected_labels->>'gitOpsManaged' = 'true' THEN 0.10 ELSE 0.0 END +
CASE WHEN detected_labels->>'pdbProtected' = 'true' THEN 0.05 ELSE 0.0 END +
...
```

**Result**: ✅ **Weights ARE being applied correctly in scoring algorithm**

---

### **2. `pkg/datastorage/scoring/weights.go` is DEAD CODE** ❌

**Evidence**:
```bash
$ grep -r "GetDetectedLabelWeight" pkg/datastorage/
pkg/datastorage/scoring/weights.go:func GetDetectedLabelWeight(field string) float64 {

# Only found in weights.go itself - NOT imported anywhere!
```

```bash
$ grep -r "scoring\.Get" pkg/datastorage/
# NO RESULTS - scoring package not imported anywhere
```

```bash
$ grep -r "import.*scoring" pkg/datastorage/repository/
# NO RESULTS - repository doesn't import scoring package
```

**Result**: ❌ **`scoring/weights.go` is dead code - never imported or used**

---

### **3. Why Weights Are Hardcoded**

**Comment in `search.go` line 416**:
```go
// Weights from scoring package (inline to avoid circular deps)
weights := map[string]float64{
    "git_ops_managed":  0.10,
    ...
}
```

**Reason**: **Circular dependency avoidance**
- `pkg/datastorage/repository/workflow` → depends on `pkg/datastorage/models`
- `pkg/datastorage/scoring` → also depends on `pkg/datastorage/models`
- **Cannot import**: `repository/workflow` → `scoring` (would create cycle)

**Solution Used**: Duplicate the weights inline in SQL generation

---

## 🧪 **Test Coverage Status**

### **What Tests DO WE Have?**

#### **✅ E2E Tests** (`test/e2e/datastorage/04_workflow_search_test.go`)

**Tests**:
- Workflow search returns results ✅
- Results ordered by confidence ✅
- Mandatory label filtering enforced ✅
- Search latency < 200ms ✅

**BUT**:
```go
// Line 416: E2E test says V1.0 doesn't use weights yet!
testLogger.Info("  ✅ V1.0: Base similarity only (no boost/penalty)")
testLogger.Info("  ✅ V2.0+: Configurable label weights (future)")
```

**Status**: ⚠️ **E2E test contradicts actual implementation!**

---

#### **❌ NO Integration Tests for Weight Application**

```bash
$ grep -r "DetectedLabel.*weight|label.*boost|label.*penalty" test/integration/datastorage/
# NO RESULTS
```

**Result**: ❌ **No integration tests validate actual weight/scoring behavior**

---

#### **❌ NO Unit Tests for Weight Application**

```bash
$ grep -r "GetDetectedLabelWeight" test/
test/unit/datastorage/workflow_search_audit_test.go  # Only in audit event structure
```

**Result**: ❌ **No unit tests validate weights are correctly applied in scoring**

---

## 📋 **What We Deleted: `weights_test.go`**

### **What It Tested** ❌

```go
It("should have high-impact weights for GitOps fields", func() {
    gitOpsManagedWeight := prodscoring.GetDetectedLabelWeight("gitOpsManaged")
    Expect(gitOpsManagedWeight).To(Equal(0.10))  // ❌ Just tests constants
})
```

### **Why Deletion Was Correct** ✅

1. ❌ Tested `GetDetectedLabelWeight()` function that **is never called**
2. ❌ Tested constants (0.10 == 0.10) with **zero business value**
3. ❌ Did NOT test actual scoring behavior in SQL
4. ✅ **Conclusion**: Deleting it was the right call - it was testing dead code!

---

## 🎯 **Test Coverage Gap Analysis**

### **What IS Tested** ✅

| What | Where | How Well |
|------|-------|----------|
| **Workflow search returns results** | E2E | ✅ Excellent |
| **Results ordered correctly** | E2E | ✅ Excellent |
| **Mandatory label filtering** | E2E | ✅ Excellent |
| **Search latency** | E2E | ✅ Excellent |

### **What is NOT Tested** ❌

| What | Missing Test | Business Impact |
|------|--------------|-----------------|
| **GitOps workflow ranked higher than manual** | Integration | **HIGH** - Core scoring logic |
| **Boost values (0.10, 0.05, 0.02) applied correctly** | Integration | **HIGH** - Business requirements |
| **Penalty for GitOps mismatch (-0.10)** | Integration | **HIGH** - Selection accuracy |
| **Custom label boost (0.05 per key)** | Integration | **MEDIUM** - V1.0 new feature |
| **Wildcard matching gives half boost** | Integration | **MEDIUM** - V1.0 new feature |
| **Score capping at 1.0** | Integration | **LOW** - Edge case |

---

## 🚨 **Contradiction: E2E Test vs. Implementation**

### **E2E Test Says** (line 416):
```
"✅ V1.0: Base similarity only (no boost/penalty)"
"✅ V2.0+: Configurable label weights (future)"
```

### **Implementation Says** (search.go line 144-154):
```go
detectedLabelBoostSQL := r.buildDetectedLabelsBoostSQLWithWildcard(request)
labelPenaltySQL := r.buildDetectedLabelsPenaltySQL(request)

SELECT
    %s AS detected_label_boost,  -- ✅ BOOST IS CALCULATED
    %s AS label_penalty,          -- ✅ PENALTY IS CALCULATED
    ...
```

**Result**: ⚠️ **Implementation DOES use weights in V1.0, but E2E test says it doesn't!**

---

## 💡 **Recommendations**

### **Option A: Add Integration Tests for Weight Application** ✅ RECOMMENDED

Create integration tests that validate:

```go
// Test: GitOps workflow ranked higher than manual workflow
It("should rank GitOps workflow higher when signal is also GitOps", func() {
    // ARRANGE: Create 2 workflows with identical mandatory labels
    gitopsWorkflow := createWorkflow(
        signalType="OOMKilled",
        detectedLabels={GitOpsManaged: true},  // +0.10 boost
    )
    manualWorkflow := createWorkflow(
        signalType="OOMKilled",
        detectedLabels={GitOpsManaged: false}, // No boost
    )

    // ACT: Search for GitOps workflows
    results := searchWorkflows(filters={
        SignalType: "OOMKilled",
        DetectedLabels: {GitOpsManaged: true},
    })

    // ASSERT: GitOps workflow ranked first
    Expect(results[0].WorkflowID).To(Equal(gitopsWorkflow.ID))
    Expect(results[0].LabelBoost).To(Equal(0.10))  // ✅ Tests actual weight
    Expect(results[0].FinalScore).To(BeNumerically(">", results[1].FinalScore))
})
```

**Coverage**: Test each weight value (0.10, 0.05, 0.02) with real workflows

---

### **Option B: Fix E2E Test Comment** ✅ REQUIRED

**Update**: `test/e2e/datastorage/04_workflow_search_test.go` line 416

```go
// BEFORE:
testLogger.Info("  ✅ V1.0: Base similarity only (no boost/penalty)")
testLogger.Info("  ✅ V2.0+: Configurable label weights (future)")

// AFTER:
testLogger.Info("  ✅ V1.0: Semantic search with label-based boost/penalty")
testLogger.Info("  ✅ V2.0+: Vector similarity + label weights (hybrid)")
```

---

### **Option C: Consolidate Weight Definitions** ⚠️ OPTIONAL (Post-V1.0)

**Problem**: Weights defined in 2 places:
1. `pkg/datastorage/scoring/weights.go` (NOT USED)
2. `pkg/datastorage/repository/workflow/search.go` (ACTUALLY USED)

**Solution** (Post-V1.0):
1. **Delete** `pkg/datastorage/scoring/weights.go` entirely (it's dead code)
2. **Keep** weights inline in `search.go` (avoids circular dependency)
3. **OR**: Move weights to `pkg/datastorage/models/scoring_weights.go` (importable by both)

**Priority**: **P2 - Post-V1.0** (not blocking)

---

## ✅ **Answer to Your Question**

### **"Do we cover test weights with label scoring in other tests?"**

**Short Answer**: **NO** ❌

**Long Answer**:
1. ❌ **`weights_test.go`** tested `GetDetectedLabelWeight()` which is **never called** (dead code)
2. ❌ **Integration tests** don't exist for label scoring validation
3. ✅ **E2E tests** validate workflow search works BUT don't specifically test weight values
4. ⚠️ **E2E test comment** incorrectly says V1.0 doesn't use weights (it does!)

**What We're Missing**:
- Integration tests that validate **"GitOps workflow scores 0.10 higher than manual workflow"**
- Integration tests that validate **penalty for GitOps mismatch is -0.10**
- Integration tests that validate **custom label boost is 0.05 per key**

---

## 🎯 **V1.0 Decision Required**

### **For V1.0 Ship:**

**MINIMAL** (Ship Now):
1. ✅ Fix E2E test comment (5 min)
2. ✅ Document that integration tests for weight validation are P2

**COMPLETE** (Delay Ship):
1. ✅ Add integration tests for each weight value
2. ✅ Validate boost/penalty calculations
3. ✅ Test wildcard matching

**Your Call**: Which path?

---

**Created**: December 17, 2025
**Status**: ⚠️ **DECISION REQUIRED** - Ship with gap or add tests?
**User Question Answered**: ✅ **NO, we don't test weights in other tests**


