# SignalProcessing E2E Coverage Improvement - Option D Implementation Plan

**Date**: 2025-12-24
**Author**: AI Assistant
**Decision**: Option D - Full improvement (B+C)
**Target**: Improve E2E coverage from 28.7% to 42%

---

## 🎯 **Implementation Plan Overview**

### **Option D: Full Coverage Improvement**
- **Option B**: Add 6 non-Pod E2E tests
- **Option C**: Add 3 business classifier E2E tests
- **Total**: 9 new E2E tests
- **Expected Result**: E2E coverage 28.7% → 42%

---

## 📋 **Test Implementation Schedule**

### **Phase 1: Non-Pod Resource Type Tests (Option B)** - 6 tests

#### **Test 1: Deployment Signal E2E** (BR-SP-103-D)
**File**: `test/e2e/signalprocessing/workload_types_test.go` (new section)
**Duration**: 30-45 minutes

```
Scenario: Deployment signal enrichment
Given: Deployment with 3 replicas in production namespace
When: SignalProcessing CR targets the Deployment
Then: Should enrich with DeploymentDetails (replicas, strategy, conditions)
BR: BR-SP-103-D (Deployment enrichment)
Expected Coverage Impact:
  - enrichDeploymentSignal: 0% → 75%
  - Enricher package: 24.9% → 28%
```

---

#### **Test 2: StatefulSet Signal E2E** (BR-SP-103-A - Already exists, verify execution)
**File**: `test/e2e/signalprocessing/business_requirements_test.go:1261`
**Duration**: 15 minutes (verification + fix if needed)

```
Scenario: StatefulSet signal enrichment
Given: StatefulSet with persistent volumes
When: SignalProcessing CR targets the StatefulSet
Then: Should enrich with StatefulSetDetails (ordinal, PVC templates)
BR: BR-SP-103-A
Expected Coverage Impact:
  - enrichStatefulSetSignal: 0% → 75%
  - Enricher package: 28% → 31%
```

**Action**: Verify why existing test doesn't show coverage. May need to adjust signal target or coverage collection.

---

#### **Test 3: DaemonSet Signal E2E** (BR-SP-103-B - Already exists, verify execution)
**File**: `test/e2e/signalprocessing/business_requirements_test.go:1376`
**Duration**: 15 minutes (verification + fix if needed)

```
Scenario: DaemonSet signal enrichment
Given: DaemonSet deployed to all nodes
When: SignalProcessing CR targets the DaemonSet
Then: Should enrich with DaemonSetDetails (desired/current/ready)
BR: BR-SP-103-B
Expected Coverage Impact:
  - enrichDaemonSetSignal: 0% → 75%
  - Enricher package: 31% → 34%
```

---

#### **Test 4: ReplicaSet Signal E2E** (BR-SP-103-E - New)
**File**: `test/e2e/signalprocessing/workload_types_test.go` (new)
**Duration**: 30-45 minutes

```
Scenario: ReplicaSet signal enrichment
Given: Standalone ReplicaSet (not managed by Deployment)
When: SignalProcessing CR targets the ReplicaSet
Then: Should enrich with ReplicaSetDetails (replicas, owner references)
BR: BR-SP-103-E
Expected Coverage Impact:
  - enrichReplicaSetSignal: 0% → 75%
  - Enricher package: 34% → 37%
```

---

#### **Test 5: Service Signal E2E** (BR-SP-103-C - Already exists, verify execution)
**File**: `test/e2e/signalprocessing/business_requirements_test.go:1469`
**Duration**: 15 minutes (verification + fix if needed)

```
Scenario: Service signal enrichment
Given: ClusterIP Service with endpoints
When: SignalProcessing CR targets the Service
Then: Should enrich with ServiceDetails (type, ports, endpoints)
BR: BR-SP-103-C
Expected Coverage Impact:
  - enrichServiceSignal: 0% → 75%
  - Enricher package: 37% → 40%
```

---

#### **Test 6: Node Signal E2E** (BR-SP-103-F - New)
**File**: `test/e2e/signalprocessing/workload_types_test.go` (new)
**Duration**: 30-45 minutes

```
Scenario: Node signal enrichment
Given: Kubernetes Node in Ready state
When: SignalProcessing CR targets the Node directly
Then: Should enrich with NodeDetails (capacity, allocatable, conditions)
BR: BR-SP-103-F
Expected Coverage Impact:
  - enrichNodeSignal: 0% → 75%
  - Enricher package: 40% → 43%
```

---

### **Phase 2: Business Classifier Tests (Option C)** - 3 tests

#### **Test 7: Label-Based Business Classification E2E** (BR-SP-052-A - New)
**File**: `test/e2e/signalprocessing/business_classification_test.go` (new file)
**Duration**: 45-60 minutes

```
Scenario: Business classification via labels
Given: Pod with labels app=payment-gateway, tier=critical, team=fintech
When: SignalProcessing CR is created
Then: Should classify as BusinessService=payment, Criticality=high, Team=fintech
BR: BR-SP-052-A
Expected Coverage Impact:
  - Classify (business): 0% → 60%
  - classifyFromLabels: 0% → 80%
  - Classifier package: 38.1% → 48%
```

**Test Setup**:
```go
// Deploy Pod with business labels
pod := &corev1.Pod{
    ObjectMeta: metav1.ObjectMeta{
        Name: "payment-gateway",
        Namespace: testNs,
        Labels: map[string]string{
            "app":  "payment-gateway",
            "tier": "critical",
            "team": "fintech",
            "business.kubernaut.ai/service": "payment",
        },
    },
    // ... spec
}

// Create SignalProcessing CR
sp := &signalprocessingv1alpha1.SignalProcessing{
    // ... target Pod
}

// Verify BusinessClassification
Eventually(func() string {
    var sp signalprocessingv1alpha1.SignalProcessing
    k8sClient.Get(ctx, client.ObjectKey{...}, &sp)
    return sp.Status.BusinessClassification.ServiceName
}).Should(Equal("payment"))
```

---

#### **Test 8: Pattern-Based Business Classification E2E** (BR-SP-052-B - New)
**File**: `test/e2e/signalprocessing/business_classification_test.go`
**Duration**: 45-60 minutes

```
Scenario: Business classification via name patterns
Given: Pod named prod-api-gateway-xyz in production namespace
When: SignalProcessing CR is created
Then: Should classify as Environment=production, ServiceType=api
BR: BR-SP-052-B
Expected Coverage Impact:
  - classifyFromPatterns: 0% → 75%
  - Classifier package: 48% → 53%
```

**Test Setup**:
```go
// Deploy Pod with pattern-matching name
pod := &corev1.Pod{
    ObjectMeta: metav1.ObjectMeta{
        Name: "prod-api-gateway-7d4f9",
        Namespace: prodNs,
    },
    // ... spec
}

// Create SignalProcessing CR
sp := &signalprocessingv1alpha1.SignalProcessing{
    // ... target Pod
}

// Verify pattern-based classification
Eventually(func() string {
    var sp signalprocessingv1alpha1.SignalProcessing
    k8sClient.Get(ctx, client.ObjectKey{...}, &sp)
    return sp.Status.BusinessClassification.Environment
}).Should(Equal("production"))
```

---

#### **Test 9: Rego-Based Business Classification E2E** (BR-SP-052-C - New)
**File**: `test/e2e/signalprocessing/business_classification_test.go`
**Duration**: 45-60 minutes

```
Scenario: Business classification via Rego policies
Given: Custom Rego policy for business classification deployed as ConfigMap
And: Pod with annotations matching Rego conditions
When: SignalProcessing CR is created
Then: Should apply Rego-defined business classification
BR: BR-SP-052-C, BR-SP-102
Expected Coverage Impact:
  - classifyFromRego: 0% → 70%
  - extractRegoResults: 0% → 75%
  - buildRegoInput: 0% → 80%
  - Classifier package: 53% → 58%
  - Overall E2E: 28.7% → 42%
```

**Test Setup**:
```go
// Deploy business classification Rego policy
businessPolicyConfigMap := &corev1.ConfigMap{
    ObjectMeta: metav1.ObjectMeta{
        Name: "business-classification-policy",
        Namespace: "kubernaut-system",
    },
    Data: map[string]string{
        "policy.rego": `
package signalprocessing.business

import rego.v1

classification := result if {
    input.labels["business.kubernaut.ai/critical"] == "true"
    result := {
        "service_name": input.labels["app"],
        "criticality": "high",
        "sla": "99.9"
    }
}
`,
    },
}

// Create Pod with business annotations
pod := &corev1.Pod{
    ObjectMeta: metav1.ObjectMeta{
        Name: "critical-service",
        Namespace: testNs,
        Labels: map[string]string{
            "app": "authentication",
            "business.kubernaut.ai/critical": "true",
        },
    },
    // ... spec
}

// Create SignalProcessing CR
sp := &signalprocessingv1alpha1.SignalProcessing{
    // ... target Pod
}

// Verify Rego-based classification
Eventually(func() string {
    var sp signalprocessingv1alpha1.SignalProcessing
    k8sClient.Get(ctx, client.ObjectKey{...}, &sp)
    return sp.Status.BusinessClassification.Criticality
}).Should(Equal("high"))
```

---

## 📊 **Expected Coverage Improvements**

### **Before Implementation**
| Package | Current E2E Coverage |
|---------|---------------------|
| Controller | 55.3% |
| Enricher | 24.9% |
| Classifier | 38.1% |
| **Overall** | **28.7%** |

### **After Phase 1 (Non-Pod Tests)**
| Package | Projected E2E Coverage | Delta |
|---------|------------------------|-------|
| Controller | 55.3% | No change |
| Enricher | **43%** | +18.1% |
| Classifier | 38.1% | No change |
| **Overall** | **35%** | **+6.3%** |

### **After Phase 2 (Business Classifier Tests)**
| Package | Projected E2E Coverage | Delta |
|---------|------------------------|-------|
| Controller | **60%** | +4.7% |
| Enricher | 43% | No change |
| Classifier | **58%** | +19.9% |
| **Overall** | **42%** | **+13.3%** |

---

## ⏱️ **Implementation Timeline**

| Phase | Tasks | Duration | Cumulative |
|-------|-------|----------|------------|
| **Phase 1 - Part A** | Tests 1-3 (Deployment, StatefulSet, DaemonSet) | 2 hours | 2 hours |
| **Phase 1 - Part B** | Tests 4-6 (ReplicaSet, Service, Node) | 2 hours | 4 hours |
| **Coverage Check** | Run E2E with coverage, validate Phase 1 | 30 min | 4.5 hours |
| **Phase 2** | Tests 7-9 (Business Classifier) | 2.5 hours | 7 hours |
| **Final Validation** | Run E2E with coverage, generate report | 30 min | **7.5 hours** |

**Total Estimated Time**: **7.5 hours** (can be split across multiple sessions)

---

## ✅ **Implementation Checklist**

### **Phase 1: Non-Pod Resource Type Tests**
- [ ] **Test 1**: Deployment signal E2E (new)
- [ ] **Test 2**: Verify StatefulSet signal E2E (existing)
- [ ] **Test 3**: Verify DaemonSet signal E2E (existing)
- [ ] **Test 4**: ReplicaSet signal E2E (new)
- [ ] **Test 5**: Verify Service signal E2E (existing)
- [ ] **Test 6**: Node signal E2E (new)
- [ ] **Phase 1 Validation**: Run E2E coverage, verify 35% overall

### **Phase 2: Business Classifier Tests**
- [ ] Create `business_classification_test.go`
- [ ] **Test 7**: Label-based classification E2E (new)
- [ ] **Test 8**: Pattern-based classification E2E (new)
- [ ] **Test 9**: Rego-based classification E2E (new)
- [ ] **Phase 2 Validation**: Run E2E coverage, verify 42% overall

### **Final Deliverables**
- [ ] Updated E2E test suite (16 → 25 tests)
- [ ] E2E coverage report showing 42% overall
- [ ] Updated BR coverage matrix
- [ ] Handoff document with results

---

## 🚀 **Next Steps**

1. **Start Phase 1 - Part A**: Implement/verify tests 1-3
2. **Checkpoint**: Run E2E coverage after Part A
3. **Continue Phase 1 - Part B**: Implement tests 4-6
4. **Checkpoint**: Run E2E coverage after Phase 1
5. **Start Phase 2**: Implement business classifier tests
6. **Final Validation**: Run full E2E suite with coverage
7. **Generate Report**: Document improvements and handoff

---

**Document Status**: ✅ **READY FOR IMPLEMENTATION**
**Next Action**: Begin Phase 1 - Part A (Deployment, StatefulSet, DaemonSet tests)
**Authority**: Per user decision "D" - Full coverage improvement (Option B+C)

