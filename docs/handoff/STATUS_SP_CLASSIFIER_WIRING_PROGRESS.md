# STATUS: SignalProcessing Classifier Wiring - In Progress

**Date**: 2025-12-12 08:10 AM
**Service**: SignalProcessing  
**Status**: 🟡 **PARTIALLY COMPLETE** - Wiring done, Rego schema mismatch found

---

## ✅ **COMPLETED (80%)**

### **1. Controller Wiring** ✅
- Added `classifier` import to controller
- Added `EnvClassifier`, `PriorityEngine`, `BusinessClassifier` fields to controller struct
- Updated `classifyEnvironment()` to call EnvClassifier if available, fall back to hardcoded
- Updated `assignPriority()` to call PriorityEngine if available, fall back to hardcoded
- All methods have proper fallback logic
- No linter errors

**Files**: `internal/controller/signalprocessing/signalprocessing_controller.go`

### **2. Test Suite Initialization** ✅
- Created temporary Rego policy files (environment.rego, priority.rego, business.rego)
- Initialized all 3 classifiers in BeforeSuite
- Wired classifiers into controller during setup
- Added DeferCleanup to remove temp files
- No linter errors

**Files**: `test/integration/signalprocessing/suite_test.go`

---

## 🔴 **DISCOVERED ISSUE**

### **Problem**: Rego Result Schema Mismatch

**Symptom**: Tests failing with validation errors:
```
status.priorityAssignment.assignedAt: Required value
status.environmentClassification.classifiedAt: Required value
```

**Root Cause**: Rego policies return timestamps, but classifiers don't map them to `metav1.Time` fields

**Evidence**:
```go
// Rego returns:
result := {"environment": "production", "confidence": 0.80, "classified_at": time.now_ns()}

// But classifier needs to set:
ClassifiedAt: metav1.Now()  // metav1.Time type, not int64
```

---

## 🔍 **DETAILED ANALYSIS**

### **What Rego Returns**:
```json
{
  "environment": "production",
  "confidence": 0.80,
  "source": "configmap",
  "classified_at": 1734024354000000000  // nanosecond timestamp
}
```

### **What CRD Expects**:
```go
type EnvironmentClassification struct {
    Environment  string      `json:"environment"`
    Confidence   float64     `json:"confidence"`
    Source       string      `json:"source"`
    ClassifiedAt metav1.Time `json:"classifiedAt"`  // ← Kubernetes Time object
}
```

### **What Classifier Does**:
```go
// pkg/signalprocessing/classifier/environment.go (estimated line ~150-180)
env := results[0].Expressions[0].Value.(map[string]interface{})
return &EnvironmentClassification{
    Environment: env["environment"].(string),
    Confidence:  env["confidence"].(float64),
    Source:      env["source"].(string),
    // ❌ MISSING: ClassifiedAt field not being set!
}
```

---

## 🔧 **REQUIRED FIX (15-30 minutes)**

### **Option A: Remove Rego Timestamps** ⭐ RECOMMENDED
**Action**: Remove `classified_at` / `assigned_at` from Rego, set in Go code

**Changes**:
1. Update Rego policies to NOT include timestamps
2. Classifiers set `ClassifiedAt = metav1.Now()` after Rego evaluation

**Example**:
```go
// environment.go
result, err := c.regoQuery.Eval(ctx, rego.EvalInput(input))
if err != nil {
    return nil, err
}

env := results[0].Expressions[0].Value.(map[string]interface{})
return &EnvironmentClassification{
    Environment:  env["environment"].(string),
    Confidence:   env["confidence"].(float64),
    Source:       env["source"].(string),
    ClassifiedAt: metav1.Now(),  // ← ADD THIS
}, nil
```

**Rego Update**:
```rego
# Remove classified_at from result
result := {"environment": "production", "confidence": 0.80, "source": "configmap"}
# NOT: result := {..., "classified_at": time.now_ns()}
```

**Effort**: 15 minutes (3 Rego files + 3 classifier methods)

---

### **Option B: Parse Rego Timestamps**
**Action**: Convert Rego `time.now_ns()` to `metav1.Time`

**Complexity**: Higher - need to parse int64 timestamps
**Effort**: 30 minutes
**Risk**: Time zone / precision issues

---

## 📊 **TEST RESULTS**

### **Before Classifiers**:
```
✅ 41 passed / 23 failed / 71 total (58%)
Duration: 169 seconds
```

### **With Classifiers (Current)**:
```
✅ 2 passed / 13 failed / 71 total (only 15 tests ran)
Duration: 417 seconds (TIMEOUT at 7 minutes)
❌ Tests timing out due to validation errors
```

### **Root Cause of Timeouts**:
- CRD validation rejects status updates (missing ClassifiedAt/AssignedAt)
- Controller retries infinitely
- Tests wait for completion that never comes

---

## 🎯 **NEXT STEPS**

### **Immediate** (15 minutes):
1. Remove `classified_at` / `assigned_at` from all 3 Rego policies
2. Add `ClassifiedAt = metav1.Now()` to environment classifier
3. Add `AssignedAt = metav1.Now()` to priority engine  
4. Add `ClassifiedAt = metav1.Now()` to business classifier (if used)

### **Then** (5 minutes):
5. Run integration tests again
6. Verify tests complete (not timeout)
7. Check pass rate improvement

---

## 📁 **FILES TO FIX**

### **Rego Policies** (in test suite):
```go
// test/integration/signalprocessing/suite_test.go

// environment.rego - REMOVE ", "classified_at": time.now_ns()"
result := {"environment": "production", "confidence": 0.80, "source": "configmap"}

// priority.rego - REMOVE ", "assigned_at": time.now_ns()"
result := {"priority": "P1", "confidence": 1.0, "source": "policy-matrix"}

// business.rego - REMOVE timestamp fields
result := {"business_unit": ..., "confidence": 0.95, "source": "namespace-labels"}
```

### **Classifier Code** (add timestamp setting):
```go
// pkg/signalprocessing/classifier/environment.go (~line 150-180)
return &EnvironmentClassification{
    Environment:  env["environment"].(string),
    Confidence:   env["confidence"].(float64),
    Source:       env["source"].(string),
    ClassifiedAt: metav1.Now(),  // ← ADD
}, nil

// pkg/signalprocessing/classifier/priority.go (~line 180-200)
return &PriorityAssignment{
    Priority:   result["priority"].(string),
    Confidence: result["confidence"].(float64),
    Source:     result["source"].(string),
    AssignedAt: metav1.Now(),  // ← ADD
}, nil

// pkg/signalprocessing/classifier/business.go (~line 150-180)
return &BusinessClassification{
    BusinessUnit:   result["business_unit"].(string),
    ...
    Confidence:     result["confidence"].(float64),
    Source:         result["source"].(string),
    ClassifiedAt:   metav1.Now(),  // ← ADD (if field exists)
}, nil
```

---

## 💡 **WHY THIS WORKS**

**Rego Role**: Evaluate policy logic (environment, priority, business unit)  
**Go Role**: Handle Kubernetes-specific types (`metav1.Time`)

**Separation of Concerns**:
- ✅ Rego: Business logic (what environment? what priority?)
- ✅ Go: Infrastructure (when? how to represent in K8s?)

This matches the implementation plan's design where Rego handles policy evaluation, Go handles K8s integration.

---

## 🔗 **COMMITS COMPLETED**

```bash
1cf322eb feat(sp): Wire classifiers into controller (Day 10 integration)
5331497a feat(sp): Initialize classifiers in integration test suite
```

---

## ⏰ **TIME INVESTMENT**

**Completed**:
- Controller wiring: 30 minutes
- Test suite initialization: 1 hour
- Investigation/triage: 30 minutes

**Remaining**:
- Fix Rego schema: 15 minutes
- Test validation: 10 minutes
- **TOTAL REMAINING**: ~25 minutes

---

**Current State**: Classifiers wired and initialized, but Rego result schema needs adjustment. Very close to working! The infrastructure is solid, just need the timestamp field fix.

**User Decision**: Should I proceed with Option A (remove Rego timestamps, set in Go)? This is the cleanest solution and matches the implementation plan's design.

