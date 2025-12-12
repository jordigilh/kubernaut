# Use Shared Infrastructure Functions - The Right Fix

**Date**: 2025-12-11
**Discovery**: AIAnalysis reinvents the wheel instead of using shared `test/infrastructure` functions
**Impact**: Duplicated code, missing wait logic, maintenance burden

---

## 🎯 **The Real Problem**

AIAnalysis has **its own custom PostgreSQL deployment function** instead of using the **shared function** that everyone else uses!

---

## 📊 **What's Available and Shared**

### **Package Structure**
```
test/infrastructure/
├── datastorage.go          ← Shared PostgreSQL/Redis functions
├── gateway.go              ← Uses shared functions ✅
├── signalprocessing.go     ← Uses shared functions ✅
├── notification.go         ← Uses shared functions ✅
├── workflowexecution.go    ← Has own functions (different pattern)
├── remediationorchestrator.go  ← Has own functions (minimal pattern)
└── aianalysis.go           ← ❌ Has own functions (WRONG)
```

**All files are in `package infrastructure`** → They can all access the same functions!

---

## ✅ **Shared Functions Available**

### **1. `deployPostgreSQLInNamespace`** (datastorage.go:193)

**Signature**:
```go
func deployPostgreSQLInNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error
```

**What it does**:
- Creates ConfigMap with init script (pgvector extension)
- Creates Secret with credentials
- Creates Service (NodePort for host access)
- Creates Deployment (with readiness/liveness probes)
- Uses production-ready image: `quay.io/jordigilh/pgvector:pg16`

**Used by**: SignalProcessing, Gateway, DataStorage, Notification

---

### **2. `deployRedisInNamespace`** (datastorage.go:418)

**Signature**:
```go
func deployRedisInNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error
```

**What it does**:
- Creates Service (NodePort for host access)
- Creates Deployment (with readiness/liveness probes)
- Uses production-ready image: `redis:7-alpine`

**Used by**: SignalProcessing, Gateway, DataStorage, Notification

---

### **3. `waitForDataStorageServicesReady`** (datastorage.go:780)

**Signature**:
```go
func waitForDataStorageServicesReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error
```

**What it does**:
- Waits for PostgreSQL pod to be ready
- Waits for Redis pod to be ready
- Waits for DataStorage pod to be ready
- Uses Ginkgo's `Eventually` with proper timeouts

**Used by**: DataStorage E2E tests

---

### **4. `deployDataStorageServiceInNamespace`** (datastorage.go:544)

**Signature**:
```go
func deployDataStorageServiceInNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error
```

**What it does**:
- Deploys Data Storage service with proper configuration
- Sets up NodePort service
- Configures environment variables

**Used by**: SignalProcessing, Gateway, DataStorage, Notification

---

## ❌ **What AIAnalysis Currently Does (Wrong)**

### **Current Code** (aianalysis.go:384-424)
```go
func deployPostgreSQL(kubeconfigPath string, writer io.Writer) error {
    // ❌ Custom implementation
    // ❌ NO context parameter
    // ❌ NO wait logic included
    // ❌ Uses kubectl manifests instead of programmatic API
    // ❌ Fallback to inline YAML
    // ❌ Different image handling

    // Create namespace
    createNamespaceCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
        "create", "namespace", "kubernaut-system", "--dry-run=client", "-o", "yaml")
    // ... complex piping logic ...

    // Deploy PostgreSQL using manifest
    manifestPath := findManifest("postgres.yaml", "test/e2e/aianalysis/manifests")
    if manifestPath != "" {
        cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
            "apply", "-f", manifestPath)
        return cmd.Run()
    }

    // Fallback: create inline deployment
    return createInlinePostgreSQL(kubeconfigPath, writer)
}
```

**Problems**:
1. **Reinvents the wheel** - doesn't use shared `deployPostgreSQLInNamespace`
2. **No context** - can't be cancelled
3. **No wait logic** - deploying without verification
4. **Manifest-based** - looks for external files instead of programmatic
5. **Maintenance burden** - changes to shared function don't benefit AIAnalysis

---

## ✅ **The Right Fix: Use Shared Functions**

### **Change #1: Replace Custom Deploy with Shared Function**

**Before** (aianalysis.go:110-114):
```go
// 3. Deploy PostgreSQL
fmt.Fprintln(writer, "🐘 Deploying PostgreSQL...")
if err := deployPostgreSQL(kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
}
```

**After**:
```go
// 3. Deploy PostgreSQL (using shared function)
fmt.Fprintln(writer, "🐘 Deploying PostgreSQL...")
ctx := context.Background()
if err := deployPostgreSQLInNamespace(ctx, "kubernaut-system", kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
}
```

---

### **Change #2: Use Shared Redis Deploy**

**Before** (aianalysis.go:116-120):
```go
// 4. Deploy Redis
fmt.Fprintln(writer, "🔴 Deploying Redis...")
if err := deployRedis(kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy Redis: %w", err)
}
```

**After**:
```go
// 4. Deploy Redis (using shared function)
fmt.Fprintln(writer, "🔴 Deploying Redis...")
if err := deployRedisInNamespace(ctx, "kubernaut-system", kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy Redis: %w", err)
}
```

---

### **Change #3: Add Wait Logic**

**Add After Redis Deploy**:
```go
// 5. Wait for infrastructure to be ready
fmt.Fprintln(writer, "⏳ Waiting for PostgreSQL and Redis to be ready...")
if err := waitForAIAnalysisInfraReady(ctx, "kubernaut-system", kubeconfigPath, writer); err != nil {
    return fmt.Errorf("infrastructure not ready: %w", err)
}
```

**New Helper Function** (add to aianalysis.go):
```go
// waitForAIAnalysisInfraReady waits for PostgreSQL and Redis to be ready
// Simplified version of waitForDataStorageServicesReady for just infra (no DS yet)
func waitForAIAnalysisInfraReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
    clientset, err := getKubernetesClient(kubeconfigPath)
    if err != nil {
        return err
    }

    // Wait for PostgreSQL pod to be ready
    fmt.Fprintf(writer, "   ⏳ Waiting for PostgreSQL pod...\n")
    Eventually(func() bool {
        pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
            LabelSelector: "app=postgresql",
        })
        if err != nil || len(pods.Items) == 0 {
            return false
        }
        for _, pod := range pods.Items {
            if pod.Status.Phase == corev1.PodRunning {
                for _, condition := range pod.Status.Conditions {
                    if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
                        return true
                    }
                }
            }
        }
        return false
    }, 3*time.Minute, 5*time.Second).Should(Succeed(), "PostgreSQL pod should become ready")
    fmt.Fprintf(writer, "   ✅ PostgreSQL ready\n")

    // Wait for Redis pod to be ready
    fmt.Fprintf(writer, "   ⏳ Waiting for Redis pod...\n")
    Eventually(func() bool {
        pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
            LabelSelector: "app=redis",
        })
        if err != nil || len(pods.Items) == 0 {
            return false
        }
        for _, pod := range pods.Items {
            if pod.Status.Phase == corev1.PodRunning {
                for _, condition := range pod.Status.Conditions {
                    if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
                        return true
                    }
                }
            }
        }
        return false
    }, 2*time.Minute, 5*time.Second).Should(Succeed(), "Redis pod should become ready")
    fmt.Fprintf(writer, "   ✅ Redis ready\n")

    return nil
}
```

---

### **Change #4: Delete Custom Functions**

**Remove These** (no longer needed):
```go
// DELETE: func deployPostgreSQL(kubeconfigPath string, writer io.Writer) error
// DELETE: func createInlinePostgreSQL(kubeconfigPath string, writer io.Writer) error
// DELETE: func deployRedis(kubeconfigPath string, writer io.Writer) error
// DELETE: func createInlineRedis(kubeconfigPath string, writer io.Writer) error
// DELETE: func findManifest(filename, startDir string) string

// ~200 lines of duplicate code removed!
```

---

## 📊 **Before vs After Comparison**

### **Before** (Current - Broken)
```go
// aianalysis.go: ~1400 lines

// Custom deployment functions (lines 384-520)
func deployPostgreSQL(kubeconfigPath string, writer io.Writer) error { ... }
func createInlinePostgreSQL(kubeconfigPath string, writer io.Writer) error { ... }
func deployRedis(kubeconfigPath string, writer io.Writer) error { ... }
func createInlineRedis(kubeconfigPath string, writer io.Writer) error { ... }
func findManifest(filename, startDir string) string { ... }

// CreateAIAnalysisCluster (lines 90-140)
deployPostgreSQL(kubeconfigPath, writer)          // ❌ No wait
deployRedis(kubeconfigPath, writer)               // ❌ No wait
deployDataStorage(clusterName, kubeconfigPath, writer)  // Fails!
```

### **After** (Fixed - Uses Shared)
```go
// aianalysis.go: ~1300 lines (200 lines removed!)

// No custom deployment functions - use shared ones!

// CreateAIAnalysisCluster (lines 90-150)
ctx := context.Background()
deployPostgreSQLInNamespace(ctx, "kubernaut-system", kubeconfigPath, writer)  // ✅ Shared
deployRedisInNamespace(ctx, "kubernaut-system", kubeconfigPath, writer)       // ✅ Shared
waitForAIAnalysisInfraReady(ctx, "kubernaut-system", kubeconfigPath, writer)  // ✅ Wait
deployDataStorage(clusterName, kubeconfigPath, writer)                        // ✅ Works!
```

---

## 🎯 **Benefits of Using Shared Functions**

### **1. Consistency** ✅
- All services use same PostgreSQL/Redis deployment
- Same configuration across the board
- Same images, same probes, same patterns

### **2. Maintenance** ✅
- Updates to shared functions benefit ALL services
- Bug fixes in one place
- No duplicate code to maintain

### **3. Reliability** ✅
- Shared functions are battle-tested (4+ services use them)
- Production-ready images
- Proper readiness/liveness probes

### **4. Context Support** ✅
- Proper context.Context for cancellation
- Can timeout gracefully
- Follows Go best practices

### **5. Code Reduction** ✅
- Remove ~200 lines of duplicate code
- Simpler codebase
- Easier to understand

---

## 📋 **Implementation Plan**

### **Step 1: Update CreateAIAnalysisCluster** (5 min)
```go
// test/infrastructure/aianalysis.go:90-140

// Add context
ctx := context.Background()

// Use shared PostgreSQL deploy
if err := deployPostgreSQLInNamespace(ctx, "kubernaut-system", kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
}

// Use shared Redis deploy
if err := deployRedisInNamespace(ctx, "kubernaut-system", kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy Redis: %w", err)
}

// Add wait logic
if err := waitForAIAnalysisInfraReady(ctx, "kubernaut-system", kubeconfigPath, writer); err != nil {
    return fmt.Errorf("infrastructure not ready: %w", err)
}
```

### **Step 2: Add Wait Helper Function** (10 min)
- Copy pattern from `waitForDataStorageServicesReady`
- Adapt for just PostgreSQL + Redis (no DataStorage yet)
- Use `Eventually` with proper timeouts

### **Step 3: Delete Custom Functions** (2 min)
- Remove `deployPostgreSQL`
- Remove `createInlinePostgreSQL`
- Remove `deployRedis`
- Remove `createInlineRedis`
- Remove `findManifest`

### **Step 4: Test** (5 min)
- Run `make test-e2e-aianalysis`
- Verify infrastructure setup completes in 2-3 minutes
- Verify E2E tests can run

---

## 🔗 **Who Else Uses Shared Functions**

| Service | Uses Shared PostgreSQL? | Uses Shared Redis? | Uses Shared Wait? | Status |
|---------|------------------------|-------------------|------------------|--------|
| **DataStorage** | ✅ Yes | ✅ Yes | ✅ Yes | Working |
| **Gateway** | ✅ Yes | ✅ Yes | ✅ Custom | Working |
| **SignalProcessing** | ✅ Yes | ✅ Yes | ✅ Custom | Working |
| **Notification** | ✅ Yes | ✅ Yes | ✅ Custom | Working |
| **WorkflowExecution** | ❌ Custom | ❌ Custom | ✅ Yes | Working |
| **RemediationOrchestrator** | ❌ Minimal | ❌ N/A | ✅ Custom | Working |
| **AIAnalysis** | ❌ **Custom** | ❌ **Custom** | ❌ **NONE** | **BROKEN** |

**Recommendation**: AIAnalysis should join the majority and use shared functions!

---

## 💡 **Why WorkflowExecution & RO Are Different**

### **WorkflowExecution**
- Has different deployment pattern (manifest-based)
- BUT still has proper wait logic (`waitForDeploymentReady`)
- Works because it waits!

### **RemediationOrchestrator**
- Minimal infrastructure (lightweight PostgreSQL)
- Custom pattern for specific needs
- BUT still has proper wait logic (`waitForROPostgreSQL`)
- Works because it waits!

### **AIAnalysis**
- Should use standard pattern (like Gateway, SignalProcessing, Notification)
- NO reason for custom deployment
- NO wait logic at all
- BROKEN because of both issues!

---

## ✅ **Recommendation**

**Use shared functions from `datastorage.go`** + add simple wait helper.

**Why?**
1. ✅ Proven pattern (4 services use it)
2. ✅ Less code (~200 lines removed)
3. ✅ Consistent across services
4. ✅ Proper context support
5. ✅ Includes wait logic pattern to follow

**Estimated Time**: 20 minutes
**Risk**: Low (using battle-tested functions)
**Benefit**: High (E2E tests will work + cleaner codebase)

---

**Date**: 2025-12-11
**Status**: 🎯 **RECOMMENDED APPROACH** - Use shared infrastructure functions
**Next**: Implement shared function usage + wait helper
