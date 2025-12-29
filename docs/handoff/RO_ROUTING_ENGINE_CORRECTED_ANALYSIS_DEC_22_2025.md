# Routing Engine Dependency Analysis - CORRECTED

**Date**: December 22, 2025
**Status**: ⚠️ **ASSUMPTION CORRECTED**
**Result**: ✅ **Routing Engine IS Phase 1 Ready** (but NOT for Redis reasons!)

---

## 🚨 **Critical Correction**

### **INCORRECT ASSUMPTION**:
> "Routing engine uses Redis for state management"

### **REALITY**:
**Routing Engine uses Kubernetes API exclusively!**

---

## 🔍 **What Routing Engine Actually Uses**

### **From `pkg/remediationorchestrator/routing/blocking.go`**:

```go
type RoutingEngine struct {
    client    client.Client  // ✅ Kubernetes API client
    namespace string         // ✅ Namespace for queries
    config    Config          // ✅ Configuration (thresholds, cooldowns)
}

// NO Redis client field!
// NO cache client field!
// NO external state store!
```

### **How It Works**:

1. **Consecutive Failures**: Reads `rr.Status.ConsecutiveFailureCount` (in-memory field)
2. **Duplicate Detection**: Uses `client.List()` to find active RRs with same `SignalFingerprint`
3. **Resource Busy**: Uses `client.List()` to find running WEs on same `TargetResource`
4. **Recently Remediated**: Uses `client.List()` to find completed WEs within cooldown
5. **Exponential Backoff**: Calculates based on `rr.Status.ConsecutiveFailureCount`

**Key Insight**: All state is stored in **Kubernetes CRD status fields**, NOT in Redis!

---

## 🎯 **What Redis IS For**

Redis is a dependency of **Data Storage**, not Routing Engine!

### **From `test/integration/remediationorchestrator/config/config.yaml`**:

```yaml
redis:
  addr: ro-e2e-redis:6379  # Data Storage uses this

datastorage:
  secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"  # Data Storage config
```

**Purpose**: Data Storage uses Redis for:
- Caching audit events
- Session management
- Rate limiting (potentially)

**Routing Engine**: Does NOT use Redis at all!

---

## ✅ **Corrected Phase 1 Feasibility**

### **Routing Engine Dependencies**:

| Dependency | Purpose | Phase 1 Available? |
|------------|---------|-------------------|
| **Kubernetes API (envtest)** | Query RemediationRequests | ✅ YES |
| **Kubernetes API (envtest)** | Query WorkflowExecutions | ✅ YES |
| **RemediationRequest CRDs** | Read status fields (ConsecutiveFailureCount) | ✅ YES (manual creation) |
| **WorkflowExecution CRDs** | Query for blocking conditions | ✅ YES (manual creation) |
| **Redis** | ❌ NOT USED | ❌ N/A |

**Conclusion**: ✅ **Routing Engine CAN be tested in Phase 1!**

---

## 🧪 **Corrected Phase 1 Test Strategy**

### **RT-1: Consecutive Failure Blocking**

```go
It("should block RR after consecutive failure threshold", func() {
    // Setup: Configure routing engine with threshold=3
    routingConfig := routing.Config{
        ConsecutiveFailureThreshold: 3,
        ConsecutiveFailureCooldown:  3600, // 1 hour
    }

    // Create 3 RemediationRequests with same SignalFingerprint, all Failed
    for i := 0; i < 3; i++ {
        rr := &remediationv1.RemediationRequest{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("rr-failed-%d", i),
                Namespace: "test-ns",
            },
            Spec: remediationv1.RemediationRequestSpec{
                SignalFingerprint: "signal-123",  // Same fingerprint
                TargetResource: &remediationv1.ResourceReference{
                    Kind: "Pod", Name: "test-pod", Namespace: "default",
                },
            },
            Status: remediationv1.RemediationRequestStatus{
                OverallPhase:             remediationv1.PhaseFailed,
                ConsecutiveFailureCount: int32(i + 1),  // Increment count
            },
        }
        Expect(k8sClient.Create(ctx, rr)).To(Succeed())
    }

    // Create new RR with same signal fingerprint
    // Controller should:
    // 1. Query Kubernetes API for RRs with same SignalFingerprint
    // 2. Count consecutive failures (3)
    // 3. Block because threshold reached

    newRR := newRemediationRequest("rr-blocked", "test-ns", "")
    newRR.Spec.SignalFingerprint = "signal-123"
    Expect(k8sClient.Create(ctx, newRR)).To(Succeed())

    // Reconcile - routing engine queries Kubernetes API (envtest)
    Eventually(func() remediationv1.RemediationPhase {
        _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(newRR), newRR)
        return newRR.Status.OverallPhase
    }, "5s").Should(Equal(remediationv1.PhaseBlocked))

    // Verify blocking details
    Expect(newRR.Status.BlockReason).To(Equal(string(remediationv1.BlockReasonConsecutiveFailures)))
    Expect(newRR.Status.BlockedUntil).ToNot(BeNil())
    Expect(newRR.Status.ConsecutiveFailureCount).To(Equal(int32(3)))
})
```

**Key Point**: No Redis needed! Routing engine uses `k8sClient.List()` to query envtest.

---

### **RT-2: Duplicate In Progress Blocking**

```go
It("should block duplicate RR when original is active", func() {
    // Create original RR (active, non-terminal phase)
    originalRR := newRemediationRequest("rr-original", "test-ns", remediationv1.PhaseProcessing)
    originalRR.Spec.SignalFingerprint = "signal-duplicate-123"
    Expect(k8sClient.Create(ctx, originalRR)).To(Succeed())

    // Create duplicate RR with same SignalFingerprint
    duplicateRR := newRemediationRequest("rr-duplicate", "test-ns", remediationv1.PhasePending)
    duplicateRR.Spec.SignalFingerprint = "signal-duplicate-123"  // Same fingerprint
    Expect(k8sClient.Create(ctx, duplicateRR)).To(Succeed())

    // Reconcile - routing engine queries envtest for active RRs with same fingerprint
    Eventually(func() remediationv1.RemediationPhase {
        _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(duplicateRR), duplicateRR)
        return duplicateRR.Status.OverallPhase
    }, "5s").Should(Equal(remediationv1.PhaseBlocked))

    // Verify blocking details
    Expect(duplicateRR.Status.BlockReason).To(Equal(string(remediationv1.BlockReasonDuplicateInProgress)))
    Expect(duplicateRR.Status.Message).To(ContainSubstring("Duplicate of active remediation"))
})
```

**Key Point**: Routing engine uses `FindActiveRRForFingerprint()` which calls `client.List()` on envtest!

---

### **RT-3: Resource Busy Blocking**

```go
It("should block RR when another WE is running on same target", func() {
    // Create running WorkflowExecution on target resource
    activeWE := &workflowexecutionv1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "we-active",
            Namespace: "test-ns",
        },
        Spec: workflowexecutionv1.WorkflowExecutionSpec{
            TargetResource: &remediationv1.ResourceReference{
                Kind: "Pod", Name: "test-pod", Namespace: "default",
            },
        },
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            Phase: workflowexecutionv1.PhaseRunning,  // Active!
        },
    }
    Expect(k8sClient.Create(ctx, activeWE)).To(Succeed())

    // Create new RR targeting same resource
    newRR := newRemediationRequest("rr-blocked", "test-ns", remediationv1.PhaseAnalyzing)
    newRR.Spec.TargetResource = &remediationv1.ResourceReference{
        Kind: "Pod", Name: "test-pod", Namespace: "default",
    }
    Expect(k8sClient.Create(ctx, newRR)).To(Succeed())

    // Reconcile - routing engine queries envtest for active WEs on same target
    Eventually(func() remediationv1.RemediationPhase {
        _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(newRR), newRR)
        return newRR.Status.OverallPhase
    }, "5s").Should(Equal(remediationv1.PhaseBlocked))

    // Verify blocking details
    Expect(newRR.Status.BlockReason).To(Equal(string(remediationv1.BlockReasonResourceBusy)))
    Expect(newRR.Status.Message).To(ContainSubstring("Another workflow"))
})
```

**Key Point**: Routing engine uses `FindActiveWFEForTarget()` which calls `client.List()` on envtest!

---

### **RT-4: Recently Remediated Cooldown**

```go
It("should block RR when same workflow+target executed within cooldown", func() {
    // Create completed WorkflowExecution within cooldown window
    recentWE := &workflowexecutionv1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "we-recent",
            Namespace: "test-ns",
        },
        Spec: workflowexecutionv1.WorkflowExecutionSpec{
            WorkflowRef: &remediationv1.WorkflowReference{
                WorkflowID: "workflow-restart",
            },
            TargetResource: &remediationv1.ResourceReference{
                Kind: "Pod", Name: "test-pod", Namespace: "default",
            },
        },
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            Phase:         workflowexecutionv1.PhaseCompleted,
            CompletionTime: &metav1.Time{Time: time.Now().Add(-2 * time.Minute)},  // Within 5min cooldown
        },
    }
    Expect(k8sClient.Create(ctx, recentWE)).To(Succeed())

    // Create AIAnalysis recommending SAME workflow
    ai := newAIAnalysisCompleted("ai-test", "test-ns", "rr-test", 0.95)
    ai.Status.SelectedWorkflow.WorkflowID = "workflow-restart"  // Same workflow!
    Expect(k8sClient.Create(ctx, ai)).To(Succeed())

    // Create RR with AIAnalysis ref
    newRR := newRemediationRequestWithChildRefs("rr-blocked", "test-ns", remediationv1.PhaseAnalyzing, "", "ai-test", "")
    newRR.Spec.TargetResource = &remediationv1.ResourceReference{
        Kind: "Pod", Name: "test-pod", Namespace: "default",
    }
    Expect(k8sClient.Create(ctx, newRR)).To(Succeed())

    // Reconcile - routing engine queries envtest for recent WEs
    Eventually(func() remediationv1.RemediationPhase {
        _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(newRR), newRR)
        return newRR.Status.OverallPhase
    }, "5s").Should(Equal(remediationv1.PhaseBlocked))

    // Verify blocking details
    Expect(newRR.Status.BlockReason).To(Equal(string(remediationv1.BlockReasonRecentlyRemediated)))
    Expect(newRR.Status.Message).To(ContainSubstring("Same workflow recently executed"))
})
```

**Key Point**: Routing engine uses `FindRecentCompletedWFE()` which calls `client.List()` on envtest!

---

### **RT-5: Blocked Phase Expiry**

```go
It("should transition from Blocked to Failed when cooldown expires", func() {
    // Create RR in Blocked phase with expired BlockedUntil
    rr := newRemediationRequest("rr-expired", "test-ns", remediationv1.PhaseBlocked)
    expiredTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
    rr.Status.BlockedUntil = &expiredTime
    rr.Status.BlockReason = string(remediationv1.BlockReasonConsecutiveFailures)
    rr.Status.ConsecutiveFailureCount = 3
    Expect(k8sClient.Create(ctx, rr)).To(Succeed())

    // Reconcile - should detect expiry
    Eventually(func() remediationv1.RemediationPhase {
        _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
        return rr.Status.OverallPhase
    }, "5s").Should(Equal(remediationv1.PhaseFailed))

    // Verify failure details
    Expect(rr.Status.Outcome).To(Equal("Blocked"))
    Expect(rr.Status.Message).To(ContainSubstring("Cooldown expired"))
})
```

**Key Point**: Pure controller logic, no external state needed!

---

## 📋 **Updated Phase 1 Infrastructure**

### **Required**:
- ✅ **envtest** (Kubernetes API server) - Routing engine queries this
- ✅ **Data Storage** (podman) - For audit validation tests
- ✅ **Redis** (podman) - For Data Storage, NOT routing engine
- ✅ **RO Controller** - Running in envtest

### **NOT Required**:
- ❌ SP Controller
- ❌ AI Controller
- ❌ WE Controller
- ❌ NT Controller

---

## 🎯 **Corrected Phase 1 Test Count**

| Category | Tests | Dependencies | Phase 1 Ready |
|----------|-------|--------------|---------------|
| **Audit Emission** | 8 | DS + RO | ✅ YES |
| **Core Metrics** | 3 | RO only | ✅ YES |
| **Timeout Edge Cases** | 7 | RO only | ✅ YES |
| **Timeout Metrics** | 1 | RO only | ✅ YES |
| **Retry Metrics** | 2 | RO only | ✅ YES |
| **Notification Creation** | 2 | RO only | ✅ YES |
| **Routing Engine** | 5 | **RO + envtest** | ✅ **YES!** |
| **TOTAL** | **28** | **RO + DS + Redis** | **All Phase 1** |

---

## ✅ **Conclusion**

**Original Assumption**: ❌ "Routing engine uses Redis, so it's Phase 1 ready"
**Corrected Reality**: ✅ "Routing engine uses Kubernetes API (envtest), so it's Phase 1 ready"

**Key Insight**:
- Redis is for Data Storage, not Routing Engine
- Routing engine queries envtest via `client.List()` for CRD objects
- All routing state is in Kubernetes CRD status fields
- No external state stores needed!

**Result**: Routing engine tests ARE still Phase 1 ready, but for the correct reasons!

---

**Status**: ✅ **CORRECTED & VALIDATED**
**Next Step**: Update test plan with corrected infrastructure dependencies



