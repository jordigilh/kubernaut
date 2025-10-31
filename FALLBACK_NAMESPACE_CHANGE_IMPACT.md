# Fallback Namespace Change Impact Analysis

**Date**: October 31, 2025  
**Change**: Fallback namespace changed from `default` to `kubernaut-system`  
**File**: `pkg/gateway/processing/crd_creator.go:233-263`  
**Confidence**: 95%

---

## 🎯 **Change Summary**

### **Before**
```go
// Fallback to "default" namespace for cluster-scoped signals
rr.Namespace = "default"
```

### **After**
```go
// Fallback to "kubernaut-system" namespace for cluster-scoped signals
rr.Namespace = "kubernaut-system"

// Add labels to preserve origin namespace information
rr.Labels["kubernaut.io/origin-namespace"] = signal.Namespace
rr.Labels["kubernaut.io/cluster-scoped"] = "true"
```

### **Rationale**
- `default` namespace is for user workloads, not infrastructure
- `kubernaut-system` is the proper home for Kubernaut infrastructure
- Consistent with other Kubernaut components (Gateway, controllers, etc.)
- Easier to find cluster-scoped RemediationRequests

---

## 📊 **Test Impact Analysis**

### **IMPACTED TESTS** (1 test requires update)

#### **1. Integration Test: Namespace Fallback**
**File**: `test/integration/gateway/error_handling_test.go:271-330`  
**Test**: `"handles namespace not found by using default namespace fallback"`  
**Impact**: ⚠️ **BREAKING** - Test expects CRD in `default`, but will be in `kubernaut-system`  
**Action**: ✅ **UPDATE REQUIRED**

**Current Test Logic**:
```go
// Line 319-322: Checks "default" namespace
err2 := k8sClient.Client.List(context.Background(), rrList,
    client.InNamespace("default"))
return err2 == nil && len(rrList.Items) > 0
```

**Required Fix**:
```go
// Change "default" to "kubernaut-system"
err2 := k8sClient.Client.List(context.Background(), rrList,
    client.InNamespace("kubernaut-system"))
return err2 == nil && len(rrList.Items) > 0
```

**Additional Validation** (recommended):
```go
// Verify cluster-scoped label is set
if len(rrList.Items) > 0 {
    crd := rrList.Items[0]
    Expect(crd.Labels["kubernaut.io/cluster-scoped"]).To(Equal("true"))
    Expect(crd.Labels["kubernaut.io/origin-namespace"]).To(Equal(nonExistentNamespace))
}
```

---

### **NON-IMPACTED TESTS** (tests that use "default" but are NOT affected)

#### **1. Helper Function Default Namespace**
**File**: `test/integration/gateway/helpers.go:403-406`  
**Code**: `if opts.Namespace == "" { opts.Namespace = "default" }`  
**Impact**: ✅ **NO IMPACT** - This is for test alert generation, not fallback logic  
**Reason**: Tests explicitly set namespace; fallback only triggers for non-existent namespaces

#### **2. Context API Aggregation Tests**
**File**: `test/integration/contextapi/04_aggregation_test.go`  
**Impact**: ✅ **NO IMPACT** - Tests use `default` namespace for test data, not fallback  
**Reason**: These tests create valid `default` namespace with test incidents

#### **3. Notification Service Tests**
**Files**:
- `test/integration/notification/edge_cases_v31_test.go:26`
- `test/integration/notification/notification_delivery_v31_test.go:29`

**Impact**: ✅ **NO IMPACT** - Tests use `default` namespace for test setup  
**Reason**: These tests don't trigger namespace fallback logic

#### **4. Priority Classification Unit Test**
**File**: `test/unit/gateway/priority_classification_test.go:188`  
**Code**: `Entry("unknown namespace → safe default (treat as production)")`  
**Impact**: ✅ **NO IMPACT** - Test description only, not checking fallback namespace  
**Reason**: This tests environment classification, not CRD namespace placement

#### **5. Error Propagation Test**
**File**: `test/integration/gateway/priority1_error_propagation_test.go:200`  
**Impact**: ✅ **NO IMPACT** - Uses `default` namespace for test alert  
**Reason**: Test creates valid `default` namespace; fallback not triggered

#### **6. Metrics Integration Test (CORRUPTED)**
**File**: `test/integration/gateway/metrics_integration_test.go.CORRUPTED`  
**Impact**: ✅ **NO IMPACT** - File is corrupted and not in test suite  
**Reason**: File marked as CORRUPTED, not executed

---

## 🔧 **Implementation Changes Required**

### **Change 1: Update Integration Test**
**File**: `test/integration/gateway/error_handling_test.go`  
**Lines**: 271-330  
**Action**: Update test to check `kubernaut-system` instead of `default`

**Changes**:
1. Line 273: Update comment from "default namespace" to "kubernaut-system"
2. Line 307: Update `By()` description
3. Line 320-322: Change `"default"` to `"kubernaut-system"`
4. Add label validation for `kubernaut.io/cluster-scoped` and `kubernaut.io/origin-namespace`

### **Change 2: Ensure kubernaut-system Namespace Exists**
**File**: `test/integration/gateway/helpers.go` or test setup  
**Action**: Ensure `kubernaut-system` namespace exists in Kind cluster

**Implementation**:
```go
// In test setup (BeforeSuite or similar)
EnsureTestNamespace(ctx, k8sClient, "kubernaut-system")
```

---

## ✅ **Validation Plan**

### **Step 1: Update Test**
1. Update `error_handling_test.go` to check `kubernaut-system`
2. Add label validation for cluster-scoped signals

### **Step 2: Run Affected Test**
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
export KUBECONFIG=/Users/jgil/.kube/kind-config
go test -v ./test/integration/gateway/error_handling_test.go \
    ./test/integration/gateway/helpers.go \
    ./test/integration/gateway/suite_test.go \
    -ginkgo.focus="handles namespace not found" \
    -timeout 5m
```

### **Step 3: Run Full Integration Test Suite**
```bash
go test -v ./test/integration/gateway/... -timeout 30m
```

### **Step 4: Run Unit Tests**
```bash
go test -v ./test/unit/gateway/... -timeout 10m
```

---

## 📈 **Risk Assessment**

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Test failure in CI/CD | High | Low | Update test before merge |
| Existing CRDs in `default` | Low | None | Change only affects new CRDs |
| Namespace doesn't exist | Low | Low | Test setup creates namespace |
| Label validation breaks | Low | Low | Labels are additive, not breaking |

---

## 🎯 **Confidence Assessment**

**Overall Confidence**: 95%

**Breakdown**:
- **Code Change**: 95% confidence (simple, well-tested path)
- **Test Impact**: 95% confidence (only 1 test affected, easy to fix)
- **Production Impact**: 95% confidence (only affects cluster-scoped signals, rare case)
- **Rollback**: 100% confidence (single-line change, easy to revert)

**Why 95% and not 100%?**:
- Small possibility of edge cases in cluster-scoped signal handling
- Need to verify `kubernaut-system` namespace exists in all environments
- Need to confirm no downstream services hardcode "default" namespace

---

## ✅ **Action Items**

### **Immediate** (5 minutes)
- [x] Update `pkg/gateway/processing/crd_creator.go` (DONE)
- [ ] Update `test/integration/gateway/error_handling_test.go`
- [ ] Ensure `kubernaut-system` namespace exists in test setup

### **Validation** (10 minutes)
- [ ] Run affected integration test
- [ ] Run full integration test suite
- [ ] Run unit test suite

### **Documentation** (5 minutes)
- [ ] Update test comments to reflect new fallback namespace
- [ ] Commit changes with clear description

---

## 📝 **Commit Message Template**

```
refactor(gateway): change fallback namespace from default to kubernaut-system

**Purpose**: Align cluster-scoped signal placement with Kubernaut infrastructure standards

**Changes**:
- Changed fallback namespace from "default" to "kubernaut-system" for cluster-scoped signals
- Added labels to preserve origin namespace information:
  - kubernaut.io/origin-namespace: <original-namespace>
  - kubernaut.io/cluster-scoped: "true"
- Updated integration test to verify new fallback behavior

**Business Outcome**:
✅ Cluster-scoped signals (NodeNotReady, etc.) placed in proper infrastructure namespace
✅ Origin namespace preserved in labels for audit/troubleshooting
✅ Consistent with other Kubernaut components

**Test Impact**:
- Updated: test/integration/gateway/error_handling_test.go (1 test)
- No impact on other tests (validated via grep analysis)

**Confidence**: 95% (simple change, well-tested path, easy rollback)
```

---

## 🔗 **Related Files**

**Implementation**:
- `pkg/gateway/processing/crd_creator.go` (lines 233-263)

**Tests**:
- `test/integration/gateway/error_handling_test.go` (lines 271-330)
- `test/integration/gateway/helpers.go` (namespace creation helpers)

**Documentation**:
- This file: `FALLBACK_NAMESPACE_CHANGE_IMPACT.md`

