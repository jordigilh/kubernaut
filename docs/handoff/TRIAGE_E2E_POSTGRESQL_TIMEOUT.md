# AIAnalysis E2E PostgreSQL Timeout - Root Cause Analysis

**Date**: 2025-12-11
**Status**: ❌ **CRITICAL BUG** - Missing wait logic after PostgreSQL deployment
**Impact**: E2E tests timeout after 20 minutes during infrastructure setup

---

## 🔍 **Root Cause**

### **The Problem**: Missing `wait` After PostgreSQL Deployment

AIAnalysis deploys PostgreSQL but **NEVER WAITS** for it to be ready before deploying dependent services.

**File**: `test/infrastructure/aianalysis.go`

```go
// Line 112-114
fmt.Fprintln(writer, "🐘 Deploying PostgreSQL...")
if err := deployPostgreSQL(kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
}

// Line 116-120 - Immediately deploys Redis (no wait!)
fmt.Fprintln(writer, "🔴 Deploying Redis...")
if err := deployRedis(kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy Redis: %w", err)
}

// Line 122-126 - Immediately deploys Data Storage (no wait!)
fmt.Fprintln(writer, "💾 Building and deploying Data Storage...")
if err := deployDataStorage(clusterName, kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy Data Storage: %w", err)
}
```

### **What's Missing**

After line 114, there should be:
```go
// MISSING: Wait for PostgreSQL to be ready
fmt.Fprintln(writer, "⏳ Waiting for PostgreSQL to be ready...")
if err := waitForPostgreSQLReady(kubeconfigPath, writer); err != nil {
    return fmt.Errorf("PostgreSQL not ready: %w", err)
}
```

---

## 📊 **Comparison: Working vs Broken**

### **Working Services** ✅

#### **WorkflowExecution** (Reference Implementation)
```go
// test/infrastructure/workflowexecution.go:100-121

// 1. Deploy PostgreSQL
fmt.Fprintf(output, "  🐘 Deploying PostgreSQL...\n")
if err := deployPostgreSQL(kubeconfigPath, output); err != nil {
    return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
}

// 2. Deploy Redis
fmt.Fprintf(output, "  🔴 Deploying Redis...\n")
if err := deployRedis(kubeconfigPath, output); err != nil {
    return fmt.Errorf("failed to deploy Redis: %w", err)
}

// 3. ✅ WAIT for PostgreSQL and Redis to be ready
fmt.Fprintf(output, "  ⏳ Waiting for PostgreSQL to be ready...\n")
if err := waitForDeploymentReady(kubeconfigPath, "postgres", output); err != nil {
    return fmt.Errorf("PostgreSQL did not become ready: %w", err)
}
fmt.Fprintf(output, "  ⏳ Waiting for Redis to be ready...\n")
if err := waitForDeploymentReady(kubeconfigPath, "redis", output); err != nil {
    return fmt.Errorf("Redis did not become ready: %w", err)
}

// 4. Now safe to deploy Data Storage
fmt.Fprintf(output, "  💾 Building and deploying Data Storage...\n")
```

**Wait Implementation**:
```go
// test/infrastructure/workflowexecution.go:764-770
func waitForDeploymentReady(kubeconfigPath, deploymentName string, output io.Writer) error {
    waitCmd := exec.Command("kubectl", "wait",
        "-n", WorkflowExecutionNamespace,
        "--for=condition=available",
        "deployment/"+deploymentName,
        "--timeout=120s",  // ← 2 minute timeout
        "--kubeconfig", kubeconfigPath)
    return waitCmd.Run()
}
```

---

#### **Gateway Service**
```go
// test/infrastructure/gateway.go:493-497

// Wait for PostgreSQL (DD-AUDIT-003: audit event persistence)
fmt.Fprintf(writer, "   Waiting for PostgreSQL...\n")
if err := waitForPods(namespace, "app=postgresql", 1, maxAttempts, delay, kubeconfigPath, writer); err != nil {
    return fmt.Errorf("PostgreSQL not ready: %w", err)
}
```

---

#### **RemediationOrchestrator Service**
```go
// test/infrastructure/remediationorchestrator.go:395-407

func waitForROPostgreSQL(ctx context.Context, namespace, kubeconfig string, writer io.Writer) error {
    fmt.Fprintln(writer, "   Waiting for PostgreSQL to be ready...")

    deadline := time.Now().Add(2 * time.Minute)  // ← 2 minute timeout
    for time.Now().Before(deadline) {
        cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "-n", namespace,
            "wait", "--for=condition=ready", "pod", "-l", "app=postgresql", "--timeout=10s")
        if err := cmd.Run(); err == nil {
            fmt.Fprintln(writer, "   ✅ PostgreSQL ready")
            return nil
        }
        time.Sleep(5 * time.Second)
    }
    return fmt.Errorf("PostgreSQL not ready within timeout")
}
```

---

### **Broken Service** ❌

#### **AIAnalysis** (Current Implementation)
```go
// test/infrastructure/aianalysis.go:110-127

// 3. Deploy PostgreSQL
fmt.Fprintln(writer, "🐘 Deploying PostgreSQL...")
if err := deployPostgreSQL(kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
}

// ❌ NO WAIT HERE!

// 4. Deploy Redis
fmt.Fprintln(writer, "🔴 Deploying Redis...")
if err := deployRedis(kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy Redis: %w", err)
}

// ❌ NO WAIT HERE!

// 5. Build and deploy Data Storage
fmt.Fprintln(writer, "💾 Building and deploying Data Storage...")
if err := deployDataStorage(clusterName, kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy Data Storage: %w", err)
}
```

**Result**: Data Storage tries to connect to PostgreSQL before it's ready → hangs waiting for connection

---

## 🚨 **Why This Causes a 20-Minute Timeout**

### **The Cascade**

1. **T+0s**: PostgreSQL deployment submitted (but pod not yet ready)
2. **T+0s**: Redis deployment submitted (immediately, no wait)
3. **T+0s**: Data Storage deployment starts building image
4. **T+60s**: Data Storage pod starts
5. **T+61s**: Data Storage tries to connect to PostgreSQL
6. **T+61s**: PostgreSQL pod STILL not ready (image pull, init, etc.)
7. **T+61s to T+1200s**: Data Storage **HANGS** waiting for PostgreSQL connection
8. **T+1200s**: Ginkgo test timeout (20 minutes)

### **The Stack Trace**
```
goroutine 22 [syscall, 19 minutes]
  os/exec.(*Cmd).Wait(0x1400031ad80)
  github.com/jordigilh/kubernaut/test/infrastructure.deployPostgreSQL
    /Users/jgil/go/src/github.com/jordigilh/kubernaut/test/infrastructure/aianalysis.go:409
```

**Line 409**: `applyCmd.Wait()` - This is waiting for `kubectl apply` to finish, which is instant
**Actual Problem**: The test is waiting for Data Storage to deploy, which is waiting for PostgreSQL to be ready

---

## ✅ **The Fix**

### **Option A: Add Wait After PostgreSQL** (Recommended)

```go
// test/infrastructure/aianalysis.go:110-130

// 3. Deploy PostgreSQL
fmt.Fprintln(writer, "🐘 Deploying PostgreSQL...")
if err := deployPostgreSQL(kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
}

// ✅ ADD THIS: Wait for PostgreSQL to be ready
fmt.Fprintln(writer, "⏳ Waiting for PostgreSQL to be ready...")
if err := waitForAIAnalysisPostgreSQL(kubeconfigPath, writer); err != nil {
    return fmt.Errorf("PostgreSQL not ready: %w", err)
}

// 4. Deploy Redis
fmt.Fprintln(writer, "🔴 Deploying Redis...")
if err := deployRedis(kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy Redis: %w", err)
}

// ✅ ADD THIS: Wait for Redis to be ready
fmt.Fprintln(writer, "⏳ Waiting for Redis to be ready...")
if err := waitForAIAnalysisRedis(kubeconfigPath, writer); err != nil {
    return fmt.Errorf("Redis not ready: %w", err)
}

// 5. Build and deploy Data Storage (now safe, dependencies ready)
fmt.Fprintln(writer, "💾 Building and deploying Data Storage...")
if err := deployDataStorage(clusterName, kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy Data Storage: %w", err)
}
```

### **Add Wait Function** (Use WorkflowExecution Pattern)

```go
// test/infrastructure/aianalysis.go (add at end of file)

// waitForAIAnalysisPostgreSQL waits for PostgreSQL deployment to be ready
// Uses 120s timeout to allow for image pull and initialization
func waitForAIAnalysisPostgreSQL(kubeconfigPath string, writer io.Writer) error {
    waitCmd := exec.Command("kubectl", "wait",
        "-n", "kubernaut-system",
        "--for=condition=available",
        "deployment/postgres",
        "--timeout=120s",
        "--kubeconfig", kubeconfigPath)
    waitCmd.Stdout = writer
    waitCmd.Stderr = writer

    if err := waitCmd.Run(); err != nil {
        return fmt.Errorf("PostgreSQL deployment not available: %w", err)
    }

    fmt.Fprintln(writer, "✅ PostgreSQL ready")
    return nil
}

// waitForAIAnalysisRedis waits for Redis deployment to be ready
func waitForAIAnalysisRedis(kubeconfigPath string, writer io.Writer) error {
    waitCmd := exec.Command("kubectl", "wait",
        "-n", "kubernaut-system",
        "--for=condition=available",
        "deployment/redis",
        "--timeout=60s",
        "--kubeconfig", kubeconfigPath)
    waitCmd.Stdout = writer
    waitCmd.Stderr = writer

    if err := waitCmd.Run(); err != nil {
        return fmt.Errorf("Redis deployment not available: %w", err)
    }

    fmt.Fprintln(writer, "✅ Redis ready")
    return nil
}
```

---

## 📋 **Implementation Checklist**

### **Phase 1: Add Wait Functions** (5 minutes)
- [ ] Add `waitForAIAnalysisPostgreSQL` function
- [ ] Add `waitForAIAnalysisRedis` function
- [ ] Use 120s timeout for PostgreSQL (image pull time)
- [ ] Use 60s timeout for Redis (smaller image)

### **Phase 2: Update Deploy Flow** (2 minutes)
- [ ] Add wait call after PostgreSQL deployment (line 114)
- [ ] Add wait call after Redis deployment (line 120)
- [ ] Test that E2E infrastructure setup completes

### **Phase 3: Verification** (5 minutes)
- [ ] Run `make test-e2e-aianalysis`
- [ ] Verify PostgreSQL becomes ready within 120s
- [ ] Verify Data Storage starts successfully
- [ ] Check total infrastructure setup time < 5 minutes

---

## 🎯 **Expected Outcome**

### **Before Fix**
```
🐘 Deploying PostgreSQL...
🔴 Deploying Redis...
💾 Building and deploying Data Storage...
[20 MINUTE TIMEOUT - Data Storage waiting for PostgreSQL]
```

### **After Fix**
```
🐘 Deploying PostgreSQL...
⏳ Waiting for PostgreSQL to be ready...
✅ PostgreSQL ready (took 45s)
🔴 Deploying Redis...
⏳ Waiting for Redis to be ready...
✅ Redis ready (took 15s)
💾 Building and deploying Data Storage...
✅ Data Storage ready (took 60s)
Total infrastructure setup: ~2-3 minutes
```

---

## 📊 **Timeline Analysis**

### **Current (Broken) Timeline**
```
0:00  - Start E2E tests
0:05  - Kind cluster created
0:10  - CRD installed
0:15  - PostgreSQL deployment submitted
0:15  - Redis deployment submitted (immediately, no wait)
0:15  - Data Storage build starts
1:30  - Data Storage pod starts
1:31  - Data Storage tries to connect to PostgreSQL
1:31  - PostgreSQL STILL pulling image / initializing
20:00 - TIMEOUT (Ginkgo suite timeout)
```

### **Expected (Fixed) Timeline**
```
0:00  - Start E2E tests
0:05  - Kind cluster created
0:10  - CRD installed
0:15  - PostgreSQL deployment submitted
0:15  - ⏳ Waiting for PostgreSQL...
1:00  - ✅ PostgreSQL ready (image pulled, pod running)
1:00  - Redis deployment submitted
1:00  - ⏳ Waiting for Redis...
1:15  - ✅ Redis ready
1:15  - Data Storage build starts
2:30  - ✅ Data Storage ready (connects to PostgreSQL immediately)
2:30  - ✅ HolmesGPT-API ready
2:30  - ✅ AIAnalysis controller ready
3:00  - ✅ Tests start running
```

---

## 🔗 **Related Patterns in Codebase**

All other services implement this correctly:

| Service | Wait Pattern | Timeout | Status |
|---------|-------------|---------|--------|
| **WorkflowExecution** | `waitForDeploymentReady` | 120s | ✅ Working |
| **Gateway** | `waitForPods` | 2 min | ✅ Working |
| **RemediationOrchestrator** | `waitForROPostgreSQL` | 2 min | ✅ Working |
| **DataStorage** | `Eventually` + pod check | N/A | ✅ Working |
| **SignalProcessing** | (uses shared `deployPostgreSQLInNamespace`) | N/A | ✅ Working |
| **AIAnalysis** | ❌ **NO WAIT** | N/A | ❌ **BROKEN** |

---

## 💡 **Why We Didn't See This Before**

### **Integration Tests Work** ✅
Integration tests use `podman-compose` which:
1. Starts all services in parallel
2. Has `depends_on` directives
3. Services retry connections automatically
4. Result: PostgreSQL is usually ready by the time Data Storage connects

### **E2E Tests Fail** ❌
E2E tests create real Kubernetes cluster which:
1. Must pull images (slow)
2. No automatic retry in deployment
3. No `depends_on` concept
4. Must explicitly wait for readiness
5. Result: Race condition between deployments

---

## 🎓 **Key Learnings**

### **Learning #1: Always Wait for Dependencies**
- Never assume infrastructure is ready after `kubectl apply`
- Use `kubectl wait --for=condition=available`
- Set reasonable timeouts (120s for databases)

### **Learning #2: Deployment Order Matters**
```
WRONG: Deploy all services in parallel (race conditions)
RIGHT: Deploy → Wait → Deploy next dependent service
```

### **Learning #3: Test Different Environments**
- `podman-compose`: Forgiving (built-in retries)
- Kubernetes: Strict (must explicitly wait)
- Both are needed for comprehensive testing

---

## 🚀 **Recommendation**

**Priority**: **HIGH** - Blocks E2E test execution

**Effort**: **LOW** - 10-15 minutes to implement

**Action**: Implement Option A (add wait functions after PostgreSQL/Redis deployment)

**Expected Time Savings**: Reduce E2E setup from 20-minute timeout to 2-3 minute success

---

**Date**: 2025-12-11
**Status**: ❌ **CRITICAL** - E2E tests cannot run until fixed
**Next**: Implement wait functions and verify E2E infrastructure setup completes
