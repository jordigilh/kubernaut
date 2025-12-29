# WorkflowExecution E2E Infrastructure Stabilization Plan

**Document Type**: Implementation Plan
**Status**: 🟡 TRIAGE - Pending investigation and implementation
**Priority**: P1 - Blocking V1.0 GA confidence
**Estimated Effort**: 3-4 hours
**Related Documents**:
- `docs/handoff/HANDOFF_WORKFLOWEXECUTION_SERVICE_OWNERSHIP.md`
- `docs/handoff/E2E_PARALLEL_INFRASTRUCTURE_OPTIMIZATION.md`
- `test/infrastructure/workflowexecution.go`

---

## 📋 Executive Summary

WorkflowExecution E2E tests currently experience infrastructure timeout issues that reduce test reliability and extend CI/CD execution time. This plan addresses both immediate stabilization (timeout increases) and longer-term optimization (parallel infrastructure setup).

**Current E2E Status**: 🟡 DEGRADED (60% confidence)
- ⚠️  Infrastructure timeout issues documented in handoff
- ✅ Tests themselves are functional and pass when infrastructure succeeds
- ⚠️  Sequential setup totals ~6.5 minutes (can timeout on slow networks/CI)

**Expected After Stabilization**: 🟢 HEALTHY (90% confidence)
- ✅ Reliable E2E tests in CI/CD and local environments
- ✅ Faster feedback loops for developers
- ✅ V1.0 GA readiness unblocked

---

## 🔍 Root Cause Analysis

### Current Infrastructure Setup (Sequential)

**Total Time**: ~6.5 minutes (all sequential)

```
1. Kind cluster creation: ~30s
   ├─ Download Kind node image (if not cached)
   └─ Start Docker/Podman containers

2. Tekton Pipelines installation: ~3-4 minutes
   ├─ kubectl apply https://github.com/tektoncd/pipeline/.../release.yaml
   ├─ Wait for tekton-pipelines-controller (300s timeout)
   └─ Wait for tekton-pipelines-webhook (300s timeout)
   Note: Image pulls from gcr.io/ghcr.io can be slow

3. PostgreSQL deployment: ~30s
   ├─ Deploy postgres deployment + service
   └─ Wait for ready (120s timeout)

4. Redis deployment: ~20s
   ├─ Deploy redis deployment + service
   └─ Wait for ready (120s timeout)

5. Data Storage build+deploy: ~1 minute
   ├─ Build image with podman: ~30s
   ├─ Load into Kind: ~20s
   ├─ Deploy DS with config+secrets: ~10s
   └─ Wait for ready (120s timeout): ~10s

6. Audit migrations: ~15s
   └─ Apply audit_events table + partitions

7. WE Controller build+deploy: ~1 minute
   ├─ Build image with podman: ~30s
   ├─ Save to tarball: ~10s
   ├─ Load into Kind: ~10s
   ├─ Deploy controller: ~10s
   └─ Wait for ready (120s timeout in suite): ~10s
```

### Identified Issues

| Issue | Impact | Evidence |
|-------|--------|----------|
| **Slow Tekton image pulls** | High | gcr.io/ghcr.io images can timeout on slow networks/CI |
| **Sequential setup** | Medium | No parallelization of independent tasks |
| **Tight timeouts** | Medium | 120s may be insufficient for CI environments |
| **Podman build + tarball** | Low | Extra save/load step adds ~20s |

### Why Timeouts Happen

1. **Tekton Images**: `gcr.io/tekton-releases/*` and `ghcr.io/tektoncd/*` images are large (200-300MB each)
2. **Network Variability**: CI environments have variable network speeds
3. **Resource Contention**: CI runners may be resource-constrained
4. **Cascading Delays**: Sequential setup means delays in one step cascade to total time

---

## 🎯 Stabilization Strategy

### Phase 1: Immediate Stabilization (1 hour)
**Goal**: Make E2E tests reliable in current CI/CD environment

**Actions**:
1. Increase timeout values for known slow operations
2. Add better timeout diagnostics
3. Verify fixes in CI

### Phase 2: Parallel Infrastructure Optimization (2-3 hours)
**Goal**: Reduce total setup time by parallelizing independent tasks

**Actions**:
1. Parallelize PostgreSQL + Redis + Tekton installation
2. Parallelize Data Storage build + WE Controller build
3. Measure and validate improvements

---

## 📝 Phase 1: Immediate Stabilization (PRIORITY: P0)

### 1.1 Timeout Adjustments

**File**: `test/infrastructure/workflowexecution.go`

**Changes**:

```go
// Line 299: Tekton controller wait (INCREASE: 300s → 600s)
// Rationale: gcr.io image pulls can be slow, 5 min insufficient for CI
waitCmd := exec.Command("kubectl", "wait",
    "-n", "tekton-pipelines",
    "--for=condition=available",
    "deployment/tekton-pipelines-controller",
    "--timeout=600s",  // INCREASED FROM 300s
    "--kubeconfig", kubeconfigPath,
)

// Line 314: Tekton webhook wait (INCREASE: 300s → 600s)
webhookWaitCmd := exec.Command("kubectl", "wait",
    "-n", "tekton-pipelines",
    "--for=condition=available",
    "deployment/tekton-pipelines-webhook",
    "--timeout=600s",  // INCREASED FROM 300s
    "--kubeconfig", kubeconfigPath,
)

// Line 772: Deployment ready wait (INCREASE: 120s → 180s)
// Rationale: DS may take time to connect to PostgreSQL/Redis
waitCmd := exec.Command("kubectl", "wait",
    "-n", WorkflowExecutionNamespace,
    "--for=condition=available",
    "deployment/"+deploymentName,
    "--timeout=180s",  // INCREASED FROM 120s
    "--kubeconfig", kubeconfigPath,
)
```

**File**: `test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go`

**Changes**:

```go
// Line 141: WE controller deployment wait (INCREASE: 120s → 180s)
waitDeployCmd := exec.Command("kubectl", "wait",
    "-n", controllerNamespace,
    "--for=condition=available",
    "deployment/workflowexecution-controller",
    "--timeout=180s",  // INCREASED FROM 120s
    "--kubeconfig", kubeconfigPath)

// Line 155: WE controller pod wait (INCREASE: 120s → 180s)
waitCmd := exec.Command("kubectl", "wait",
    "--for=condition=ready",
    "pod",
    "-l", "app=workflowexecution-controller",
    "--timeout=180s",  // INCREASED FROM 120s
    "--kubeconfig", kubeconfigPath)
```

### 1.2 Timeout Diagnostics

**Add diagnostic output when waiting for deployments**:

```go
// In waitForDeploymentReady (line 766+)
func waitForDeploymentReady(kubeconfigPath, deploymentName string, output io.Writer) error {
    fmt.Fprintf(output, "  ⏳ Waiting for deployment/%s (timeout: 180s)...\n", deploymentName)

    // Add progress tracking
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    go func() {
        elapsed := 0
        for range ticker.C {
            elapsed += 30
            fmt.Fprintf(output, "    Still waiting... (%ds elapsed)\n", elapsed)
        }
    }()

    waitCmd := exec.Command("kubectl", "wait",
        "-n", WorkflowExecutionNamespace,
        "--for=condition=available",
        "deployment/"+deploymentName,
        "--timeout=180s",
        "--kubeconfig", kubeconfigPath,
    )
    waitCmd.Stdout = output
    waitCmd.Stderr = output
    if err := waitCmd.Run(); err != nil {
        // On timeout, show pod status for debugging
        fmt.Fprintf(output, "\n❌ Deployment %s timeout - showing pod status:\n", deploymentName)
        statusCmd := exec.Command("kubectl", "get", "pods",
            "-n", WorkflowExecutionNamespace,
            "-l", "app="+deploymentName,
            "-o", "wide",
            "--kubeconfig", kubeconfigPath)
        statusCmd.Stdout = output
        statusCmd.Stderr = output
        _ = statusCmd.Run()

        return fmt.Errorf("deployment %s did not become available: %w", deploymentName, err)
    }
    return nil
}
```

### 1.3 Validation

**Success Criteria**:
- E2E tests pass consistently in CI (3/3 runs)
- No infrastructure timeouts for 24 hours of CI runs
- Total setup time remains < 10 minutes

**Verification Commands**:
```bash
# Local validation
make test-e2e-workflowexecution

# CI validation (after merging)
# Monitor GitHub Actions / CI logs for timeout errors
```

---

## 🚀 Phase 2: Parallel Infrastructure Optimization (PRIORITY: P1)

### 2.1 Parallelization Opportunities

**Independent Tasks** (can run in parallel):
- ✅ PostgreSQL deployment
- ✅ Redis deployment
- ✅ Tekton Pipelines installation
- ⏸️ Data Storage build (depends on PostgreSQL+Redis ready)
- ⏸️ WE Controller build (depends on CRDs applied)

**Expected Savings**:
- Current: PostgreSQL (30s) + Redis (20s) + Tekton (3-4min) = ~4.5 min sequential
- Parallel: max(PostgreSQL, Redis, Tekton) = ~3-4 min
- **Savings**: ~60-90 seconds (~15-20% improvement)

### 2.2 Implementation Approach

**Use Go routines + channels** (following SignalProcessing E2E pattern):

```go
// In CreateWorkflowExecutionCluster (line 60)
func CreateWorkflowExecutionCluster(clusterName, kubeconfigPath string, output io.Writer) error {
    // ... Kind cluster creation (unchanged) ...

    // PARALLEL: Deploy PostgreSQL, Redis, and Tekton concurrently
    fmt.Fprintf(output, "\n🚀 Deploying infrastructure in parallel...\n")

    type infraResult struct {
        name string
        err  error
    }

    infraChan := make(chan infraResult, 3)
    ctx := context.Background()

    // Parallel Task 1: PostgreSQL
    go func() {
        fmt.Fprintf(output, "  🐘 [Parallel] Deploying PostgreSQL...\n")
        err := deployPostgreSQLInNamespace(ctx, WorkflowExecutionNamespace, kubeconfigPath, output)
        if err == nil {
            err = waitForDeploymentReady(kubeconfigPath, "postgres", output)
        }
        infraChan <- infraResult{name: "PostgreSQL", err: err}
    }()

    // Parallel Task 2: Redis
    go func() {
        fmt.Fprintf(output, "  🔴 [Parallel] Deploying Redis...\n")
        err := deployRedisInNamespace(ctx, WorkflowExecutionNamespace, kubeconfigPath, output)
        if err == nil {
            err = waitForDeploymentReady(kubeconfigPath, "redis", output)
        }
        infraChan <- infraResult{name: "Redis", err: err}
    }()

    // Parallel Task 3: Tekton Pipelines
    go func() {
        fmt.Fprintf(output, "  🔧 [Parallel] Installing Tekton Pipelines...\n")
        err := installTektonPipelines(kubeconfigPath, output)
        infraChan <- infraResult{name: "Tekton", err: err}
    }()

    // Collect results
    var postgresErr, redisErr, tektonErr error
    for i := 0; i < 3; i++ {
        result := <-infraChan
        if result.err != nil {
            fmt.Fprintf(output, "❌ %s failed: %v\n", result.name, result.err)
            switch result.name {
            case "PostgreSQL":
                postgresErr = result.err
            case "Redis":
                redisErr = result.err
            case "Tekton":
                tektonErr = result.err
            }
        } else {
            fmt.Fprintf(output, "✅ %s ready\n", result.name)
        }
    }

    // Return first error encountered
    if postgresErr != nil {
        return fmt.Errorf("PostgreSQL deployment failed: %w", postgresErr)
    }
    if redisErr != nil {
        return fmt.Errorf("Redis deployment failed: %w", redisErr)
    }
    if tektonErr != nil {
        return fmt.Errorf("Tekton installation failed: %w", tektonErr)
    }

    // SEQUENTIAL (depends on PostgreSQL+Redis): Data Storage
    fmt.Fprintf(output, "\n💾 Building and deploying Data Storage...\n")
    if err := deployDataStorageWithConfig(clusterName, kubeconfigPath, output); err != nil {
        return fmt.Errorf("failed to deploy Data Storage: %w", err)
    }

    // ... rest of setup (migrations, controller) remains sequential ...
}
```

### 2.3 Additional Parallelization: Image Builds

**Parallel Task 4: Build images concurrently**

```go
// After Data Storage is ready, build both WE controller and any other images in parallel
fmt.Fprintf(output, "\n🏗️  Building controller images in parallel...\n")

buildChan := make(chan infraResult, 1)

// Parallel Build: WE Controller
go func() {
    fmt.Fprintf(output, "  🔨 [Parallel] Building WE controller image...\n")
    // Extract build logic from DeployWorkflowExecutionController
    err := buildAndLoadControllerImage(clusterName, kubeconfigPath, output)
    buildChan <- infraResult{name: "WEController", err: err}
}()

// Collect build result
buildResult := <-buildChan
if buildResult.err != nil {
    return fmt.Errorf("controller build failed: %w", buildResult.err)
}
fmt.Fprintf(output, "✅ Controller image built\n")
```

### 2.4 Validation

**Success Criteria**:
- E2E setup completes in < 5 minutes (down from ~6.5 minutes)
- All parallel tasks complete successfully
- No increase in timeout failures

**Measurement**:
```bash
# Before optimization
time make test-e2e-workflowexecution
# Expected: ~6.5 minutes total

# After optimization
time make test-e2e-workflowexecution
# Expected: ~5 minutes total (15-20% improvement)
```

---

## 📊 Expected Outcomes

### Phase 1 Outcomes (Immediate)
- ✅ E2E tests pass reliably in CI/CD (>95% success rate)
- ✅ Infrastructure timeout errors eliminated
- ✅ Better diagnostic output for debugging timeouts

### Phase 2 Outcomes (Optimized)
- ✅ E2E setup time reduced by 15-20% (~60-90 seconds)
- ✅ Faster feedback loops for developers
- ✅ Reduced CI/CD resource contention

### V1.0 GA Impact
- ✅ E2E confidence raised from 60% → 90%
- ✅ WE service overall confidence: 75% → 85%
- ✅ V1.0 GA readiness unblocked

---

## 🛠️ Implementation Checklist

### Phase 1: Immediate Stabilization (1 hour)
- [ ] Update timeout values in `test/infrastructure/workflowexecution.go`:
  - [ ] Tekton controller: 300s → 600s
  - [ ] Tekton webhook: 300s → 600s
  - [ ] Deployment ready: 120s → 180s
- [ ] Update timeout values in `test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go`:
  - [ ] WE controller deployment: 120s → 180s
  - [ ] WE controller pod: 120s → 180s
- [ ] Add timeout diagnostics to `waitForDeploymentReady`
- [ ] Test locally: `make test-e2e-workflowexecution`
- [ ] Commit and push to CI
- [ ] Monitor CI for 3 successful runs

### Phase 2: Parallel Optimization (2-3 hours)
- [ ] Refactor `CreateWorkflowExecutionCluster` to use goroutines
- [ ] Implement parallel deployment for PostgreSQL, Redis, Tekton
- [ ] Implement parallel image builds (if applicable)
- [ ] Add synchronization using channels
- [ ] Test locally and measure time savings
- [ ] Validate in CI with 3 successful runs
- [ ] Update documentation with new timing expectations

### Documentation Updates
- [ ] Update `docs/handoff/HANDOFF_WORKFLOWEXECUTION_SERVICE_OWNERSHIP.md`
  - Change E2E status from "🟡 DEGRADED (60%)" to "🟢 HEALTHY (90%)"
- [ ] Update `docs/handoff/E2E_PARALLEL_INFRASTRUCTURE_OPTIMIZATION.md`
  - Mark WorkflowExecution as "✅ Implemented"

---

## 🔗 References

- **Handoff Document**: `docs/handoff/HANDOFF_WORKFLOWEXECUTION_SERVICE_OWNERSHIP.md`
- **E2E Parallel Infrastructure**: `docs/handoff/E2E_PARALLEL_INFRASTRUCTURE_OPTIMIZATION.md`
- **WE Infrastructure Code**: `test/infrastructure/workflowexecution.go`
- **WE E2E Suite**: `test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go`
- **SignalProcessing Reference**: `test/e2e/signalprocessing/signalprocessing_e2e_suite_test.go` (parallel pattern)

---

## 📞 Escalation Path

If timeout issues persist after Phase 1:
1. **Increase timeouts further** (Tekton: 600s → 900s)
2. **Investigate CI runner resources** (CPU/memory constraints)
3. **Consider pre-pulling images** in CI environment
4. **Evaluate alternative Tekton versions** (e.g., v1.6.0 with smaller images)

**Contact**: WorkflowExecution Team Lead
**Slack**: #workflowexecution
**Priority**: P1 - Required for V1.0 GA


