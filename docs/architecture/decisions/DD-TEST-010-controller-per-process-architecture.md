# DD-TEST-010: Controller-Per-Process Architecture for Parallel Test Execution

**Status**: ✅ **APPROVED**  
**Date**: 2026-01-10  
**Related**: DD-TEST-002 (Parallel Test Execution), DD-METRICS-001 (Metrics Wiring), DD-PERF-001 (Atomic Status Updates)  
**Applies To**: ALL CRD controller services (AIAnalysis, RemediationOrchestrator, SignalProcessing, Notification, WorkflowExecution, Gateway)  
**Confidence**: 95%  

---

## Context & Problem

### Discovery (January 2026)

During parallel test execution investigation, we discovered **two distinct architectural patterns** in kubernaut integration tests:

**Pattern A: Multi-Controller (WorkflowExecution ONLY)**
- Each parallel process creates its own controller instance
- True parallelism: ALL tests can run simultaneously across all processes
- Example: WorkflowExecution runs 12 controllers in 12 parallel processes

**Pattern B: Single-Controller (4 other services)**
- Only process 1 creates a controller instance
- Partial parallelism: Only client-based tests run in parallel
- Example: AIAnalysis, RemediationOrchestrator, SignalProcessing, Notification
- **Impact**: Controller-dependent tests serialize on process 1 while processes 2-12 sit idle

### Performance Impact

| Service | Pattern | Architecture | Parallel Utilization | Wasted Resources |
|---------|---------|-------------|---------------------|-----------------|
| **WorkflowExecution** | Multi-Controller | Process 1-12: controllers | 100% (ALL tests parallel) | 0% |
| **AIAnalysis** | Single-Controller | Process 1: controller, 2-12: idle | ~20-40% (client tests only) | 60-80% |
| **RemediationOrchestrator** | Single-Controller | Process 1: controller, 2-12: idle | ~20-40% (client tests only) | 60-80% |
| **SignalProcessing** | Single-Controller | Process 1: controller, 2-12: idle | ~20-40% (client tests only) | 60-80% |
| **Notification** | Single-Controller | Process 1: controller, 2-12: idle | ~20-40% (client tests only) | 60-80% |

**Problem**: 4 out of 5 CRD controller services waste 60-80% of available parallel test capacity.

---

## Decision

**APPROVED: Multi-Controller Architecture (Controller-Per-Process Pattern)**

### Authoritative Standard

**ALL CRD controller services MUST use the multi-controller pattern** for integration and E2E tests:

```go
// ═══════════════════════════════════════════════════════════════════════════════
// Phase 1: Infrastructure ONLY (Process 1 ONLY)
// ═══════════════════════════════════════════════════════════════════════════════
var _ = SynchronizedBeforeSuite(func() []byte {
    // START ONLY shared infrastructure (containers, databases)
    // Per DD-TEST-002: PostgreSQL, Redis, DataStorage, etc.
    dsInfra, err := infrastructure.StartDSBootstrap(infrastructure.DSBootstrapConfig{
        ServiceName:     "[service]",
        PostgresPort:    XXXXX,  // Per DD-TEST-001
        RedisPort:       XXXXX,  // Per DD-TEST-001
        DataStoragePort: XXXXX,  // Per DD-TEST-001
        MetricsPort:     XXXXX,
        ConfigDir:       "test/integration/[service]/config",
    }, GinkgoWriter)
    Expect(err).ToNot(HaveOccurred())
    
    // Clean up on exit
    DeferCleanup(func() {
        infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
    })
    
    // DO NOT create: envtest, k8sManager, controller, handlers, metrics
    // These MUST be created in Phase 2 (per-process)
    
    return []byte{} // Share NO data (each process creates own environment)
    
}, func(data []byte) {
    // ═══════════════════════════════════════════════════════════════════════════════
    // Phase 2: Per-Process Controller Environment (ALL Processes)
    // ═══════════════════════════════════════════════════════════════════════════════
    
    By("Creating per-process test context")
    ctx, cancel = context.WithCancel(context.Background())
    
    By("Registering CRD schemes")
    err := [service]v1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())
    
    By("Bootstrapping per-process envtest environment")
    // Each process gets its OWN Kubernetes API server (envtest)
    testEnv = &envtest.Environment{
        CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
        ErrorIfCRDPathMissing: true,
    }
    
    if getFirstFoundEnvTestBinaryDir() != "" {
        testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
    }
    
    cfg, err = testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    Expect(cfg).NotTo(BeNil())
    
    By("Creating per-process K8s client")
    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    Expect(err).NotTo(HaveOccurred())
    Expect(k8sClient).NotTo(BeNil())
    
    By("Creating per-process namespaces")
    systemNs := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{Name: "kubernaut-system"},
    }
    err = k8sClient.Create(ctx, systemNs)
    Expect(err).NotTo(HaveOccurred())
    
    defaultNs := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{Name: "default"},
    }
    _ = k8sClient.Create(ctx, defaultNs) // May already exist
    
    By("Setting up per-process controller manager")
    k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
        Scheme: scheme.Scheme,
        Metrics: metricsserver.Options{
            BindAddress: "0", // Random port per process (no conflicts)
        },
    })
    Expect(err).ToNot(HaveOccurred())
    
    By("Creating per-process isolated metrics registry")
    // Per DD-METRICS-001: Each process needs isolated Prometheus registry
    testRegistry := prometheus.NewRegistry()
    testMetrics := [service]metrics.NewMetricsWithRegistry(testRegistry)
    
    By("Creating per-process audit store")
    // Each process connects to shared DataStorage (from Phase 1)
    // but maintains its own buffer and client
    mockTransport := testutil.NewMockUserTransport(
        fmt.Sprintf("test-[service]@integration.test-p%d", ginkgo.GinkgoParallelProcess()),
    )
    dsClient, err := audit.NewOpenAPIClientAdapterWithTransport(
        dataStorageBaseURL,
        5*time.Second,
        mockTransport,
    )
    Expect(err).ToNot(HaveOccurred())
    
    auditStore, err = audit.NewBufferedStore(
        dsClient,
        audit.DefaultConfig(),
        "[service]-controller",
        ctrl.Log.WithName("audit"),
    )
    Expect(err).ToNot(HaveOccurred())
    
    By("Setting up per-process controller with handlers")
    // Create service-specific handlers/dependencies per process
    // Example for AIAnalysis:
    hapiClient := hgclient.NewHolmesGPTClient(...)
    regoEvaluator := rego.NewEvaluator(...)
    investigatingHandler := handlers.NewInvestigatingHandler(...)
    analyzingHandler := handlers.NewAnalyzingHandler(...)
    
    // Create per-process controller instance
    reconciler = &[service].Reconciler{
        Client:        k8sManager.GetClient(),
        Scheme:        k8sManager.GetScheme(),
        Recorder:      k8sManager.GetEventRecorderFor("[service]-controller"),
        Metrics:       testMetrics,    // Per-process metrics
        AuditStore:    auditStore,     // Per-process audit buffer
        StatusManager: status.NewManager(k8sManager.GetClient()),
        // Service-specific dependencies...
    }
    err = reconciler.SetupWithManager(k8sManager)
    Expect(err).ToNot(HaveOccurred())
    
    By("Starting per-process controller manager")
    go func() {
        defer GinkgoRecover()
        err = k8sManager.Start(ctx)
        Expect(err).ToNot(HaveOccurred())
    }()
    
    By("Waiting for per-process controller manager to be ready")
    Eventually(func() bool {
        return k8sManager.GetCache().WaitForCacheSync(ctx)
    }, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
    
    GinkgoWriter.Printf("✅ [Process %d] Controller ready\n", ginkgo.GinkgoParallelProcess())
})
```

---

## Rationale

### Why Controller-Per-Process?

| Aspect | Single-Controller | Multi-Controller | Winner |
|--------|------------------|------------------|--------|
| **Parallel Utilization** | 20-40% (process 1 only) | 100% (all processes) | ✅ Multi |
| **Test Speed** | Slow (serialized controller tests) | Fast (all tests parallel) | ✅ Multi |
| **Resource Usage** | Low (1 controller) | Higher (12 controllers) | ❌ Single |
| **Isolation** | Shared K8s API state | Fully isolated envtest per process | ✅ Multi |
| **Debugging** | Simple (one controller) | More complex (12 controllers) | ❌ Single |
| **Consistency** | N/A | Same pattern as unit tests (isolated) | ✅ Multi |

**Verdict**: Multi-controller wins on **performance, isolation, and consistency**. Resource usage is acceptable for CI/CD.

### Resource Impact Analysis

**Memory Usage** (per process):
- envtest (in-memory K8s API): ~100-150MB
- Controller manager: ~50-100MB
- Test process overhead: ~50MB
- **Total per process**: ~200-300MB
- **12 processes**: ~2.4-3.6GB

**CPU Usage**:
- envtest API server: 1-2% per process (idle)
- Controller reconciliation: 5-10% per process (during tests)
- **Total**: 10-20% CPU across 12 processes

**CI/CD Impact**: GitHub Actions runners have 7GB RAM + 4 cores = sufficient for 4-12 parallel processes.

### Proven Pattern

**WorkflowExecution has used multi-controller since inception**:
- 100% parallel test execution
- 0 test interference issues
- Stable across 100+ tests
- Pattern proven in production CI/CD

---

## Implementation Checklist

### Service Migration Checklist

For each service migrating from single-controller → multi-controller:

#### Phase 1: Audit Current Architecture

- [ ] Read existing `test/integration/[service]/suite_test.go`
- [ ] Identify what's in `SynchronizedBeforeSuite` Phase 1 (process 1 only)
- [ ] Identify what's in `SynchronizedBeforeSuite` Phase 2 (all processes)
- [ ] Document current controller/manager/metrics creation location

#### Phase 2: Refactor Suite Setup

**Move to Phase 1 (process 1 only)**:
- [ ] Keep ONLY infrastructure: `StartDSBootstrap()`
- [ ] Keep service-specific containers (e.g., HAPI for AIAnalysis)
- [ ] Remove: envtest, k8sManager, controller, handlers, metrics
- [ ] Change `return configBytes` → `return []byte{}`

**Move to Phase 2 (all processes)**:
- [ ] Add: envtest.Start() (per-process K8s API)
- [ ] Add: ctrl.NewManager() (per-process manager)
- [ ] Add: NewMetricsWithRegistry() (per-process metrics, isolated registry)
- [ ] Add: audit.NewBufferedStore() (per-process audit buffer)
- [ ] Add: Service handlers/dependencies (per-process)
- [ ] Add: &Reconciler{} (per-process controller instance)
- [ ] Add: k8sManager.Start() (per-process manager start)

#### Phase 3: Update Global Variables

- [ ] Convert shared globals → per-process locals (e.g., `testMetrics`)
- [ ] Ensure no shared state between processes
- [ ] Use `ginkgo.GinkgoParallelProcess()` for process-specific logging

#### Phase 4: Test Validation

- [ ] Run with 1 process: `ginkgo -procs=1 ./test/integration/[service]/...`
- [ ] Run with 2 processes: `ginkgo -procs=2 ./test/integration/[service]/...`
- [ ] Run with 4 processes: `ginkgo -procs=4 ./test/integration/[service]/...`
- [ ] Run with 12 processes: `ginkgo -procs=12 ./test/integration/[service]/...`
- [ ] Fix any shared-state assumptions in tests
- [ ] Validate 100% pass rate at all parallelism levels

#### Phase 5: E2E Extension

- [ ] Apply same pattern to `test/e2e/[service]/suite_test.go`
- [ ] Ensure E2E uses controller-per-process for Kind cluster tests
- [ ] Validate E2E tests pass with parallel execution

---

## Anti-Patterns (FORBIDDEN)

### ❌ Single-Controller Pattern

```go
// ❌ WRONG: Controller created in Phase 1 (process 1 only)
var _ = SynchronizedBeforeSuite(func() []byte {
    StartDSBootstrap()
    
    // ❌ FORBIDDEN: envtest in Phase 1
    testEnv = envtest.Start()
    
    // ❌ FORBIDDEN: controller in Phase 1
    k8sManager = ctrl.NewManager()
    testMetrics = metrics.NewMetrics()  // ❌ Global metrics
    reconciler = &Reconciler{...}       // ❌ Single controller
    k8sManager.Start()
    
    // Share REST config with other processes
    return serializeConfig(cfg)
}, func(data []byte) {
    // Phase 2: Only create per-process K8s clients
    cfg = deserializeConfig(data)
    k8sClient = client.New(cfg)  // ← Other processes have client but NO controller
    // Tests needing controller MUST run on process 1 only = serialization!
})
```

**Problem**: Processes 2-12 have no controller → controller-dependent tests serialize on process 1.

### ❌ Shared Metrics Registry

```go
// ❌ WRONG: Global Prometheus registry shared across processes
testMetrics := metrics.NewMetrics() // Uses default registry
// Problem: Processes interfere with each other's metrics
```

**✅ CORRECT**:
```go
// ✅ RIGHT: Isolated registry per process
testRegistry := prometheus.NewRegistry()          // Isolated
testMetrics := metrics.NewMetricsWithRegistry(testRegistry)  // Per-process
```

### ❌ Sharing envtest Config

```go
// ❌ WRONG: Serialize/share REST config from Phase 1
func() []byte {
    cfg, _ = testEnv.Start()  // Phase 1 only
    return serializeConfig(cfg)  // Share with other processes
}
// Problem: All processes share same K8s API = no isolation
```

**✅ CORRECT**:
```go
// ✅ RIGHT: Each process creates own envtest
func() []byte {
    StartDSBootstrap()  // Infrastructure only
    return []byte{}     // Share nothing
}

func(data []byte) {
    testEnv = envtest.Start()  // Per-process K8s API
}
```

---

## Migration Priority

### High Priority (Immediate Migration)

1. **AIAnalysis** - Currently being worked on (57 tests)
2. **RemediationOrchestrator** - Orchestrator with many controller-dependent tests
3. **SignalProcessing** - High test count, metrics-heavy
4. **Notification** - Delivery orchestrator pattern

### Lower Priority (Defer)

- Gateway (no controller, different pattern)
- DataStorage (no controller, REST API service)
- AuthWebhook (no controller, webhook service)
- HolmesGPT-API (Python FastAPI, different pattern)

---

## Performance Benchmarks

### AIAnalysis (Before/After Migration)

**Before (Single-Controller)**:
- Total tests: 57
- Processes: 12
- Utilization: ~30% (process 1 busy, 2-12 idle)
- Time: ~180 seconds

**After (Multi-Controller - Expected)**:
- Total tests: 57
- Processes: 12
- Utilization: ~90% (all processes busy)
- Time: ~60 seconds (3x faster)

### WorkflowExecution (Multi-Controller Baseline)

- Total tests: 100+
- Processes: 12
- Utilization: 100%
- Time: ~120 seconds
- Pattern: ✅ Reference implementation

---

## Success Criteria

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Parallel utilization** | ≥90% | All processes running tests simultaneously |
| **Test speed improvement** | ≥2.5x | Compared to single-controller baseline |
| **Pass rate** | 100% | No flaky tests due to isolation issues |
| **Resource usage** | <4GB RAM | CI/CD runners can handle load |

---

## Validation Commands

### Check Current Pattern

```bash
# Identify where controller is created
grep -A 20 "SynchronizedBeforeSuite" test/integration/[service]/suite_test.go | \
  grep -E "reconciler =|SetupWithManager|k8sManager.Start"

# If found in first function (before }, func) → Single-Controller ❌
# If found in second function (after }, func) → Multi-Controller ✅
```

### Validate Parallel Execution

```bash
# Run with different parallelism levels
ginkgo -procs=1 -v ./test/integration/[service]/...  # Baseline
ginkgo -procs=4 -v ./test/integration/[service]/...  # Standard
ginkgo -procs=12 -v ./test/integration/[service]/... # Maximum

# Check for process distribution in output
# Should see: [Process X] messages from ALL processes running tests
```

### Verify Isolation

```bash
# Check for shared state issues
go test -race ./test/integration/[service]/...

# Should show: 0 race conditions
```

---

## Migration Roadmap

### Phase 1: Pilot (January 2026)

- [ ] **AIAnalysis**: Migrate to multi-controller pattern (4-6 hours)
- [ ] Validate 100% test pass rate
- [ ] Measure performance improvement
- [ ] Document lessons learned

### Phase 2: Core Services (Q1 2026)

- [ ] **RemediationOrchestrator**: Apply pattern (4-6 hours)
- [ ] **SignalProcessing**: Apply pattern (4-6 hours)
- [ ] **Notification**: Apply pattern (4-6 hours)

### Phase 3: E2E Extension (Q1 2026)

- [ ] Extend pattern to E2E tests for all services
- [ ] Validate Kind cluster compatibility
- [ ] Measure CI/CD pipeline improvement

### Phase 4: Documentation (Q1 2026)

- [ ] Update service implementation templates
- [ ] Create migration guide
- [ ] Add to onboarding documentation

**Total Estimated Effort**: 20-24 hours across 4 services

---

## Cross-References

1. **DD-TEST-002**: Parallel Test Execution Standard (isolation requirements)
2. **DD-METRICS-001**: Controller Metrics Wiring Pattern (per-process metrics)
3. **DD-PERF-001**: Atomic Status Updates Mandate (per-process status managers)
4. **DD-TEST-001**: Port Allocation Strategy (infrastructure ports)
5. **WorkflowExecution**: Reference implementation (`test/integration/workflowexecution/suite_test.go:113-268`)

---

## Appendix A: Pattern Comparison Matrix

| Component | Single-Controller | Multi-Controller |
|-----------|------------------|------------------|
| **Infrastructure** | Phase 1 (shared) | Phase 1 (shared) ✅ |
| **envtest** | Phase 1 (1 instance) | Phase 2 (12 instances) ✅ |
| **k8sManager** | Phase 1 (1 instance) | Phase 2 (12 instances) ✅ |
| **Controller** | Phase 1 (1 instance) | Phase 2 (12 instances) ✅ |
| **testMetrics** | Phase 1 (shared registry) | Phase 2 (isolated registry) ✅ |
| **auditStore** | Phase 1 or 2 (varies) | Phase 2 (isolated buffer) ✅ |
| **Handlers** | Phase 1 (shared) | Phase 2 (per-process) ✅ |
| **Test Isolation** | Partial (shared K8s API) | Full (isolated envtest) ✅ |
| **Parallel Utilization** | 20-40% | 100% ✅ |

---

## Appendix B: Service Implementation Status

| Service | Current Pattern | Migration Status | Priority | Estimated Effort |
|---------|----------------|------------------|----------|-----------------|
| **WorkflowExecution** | ✅ Multi-Controller | N/A (reference) | - | 0h (complete) |
| **AIAnalysis** | ❌ Single-Controller | 🔄 In Progress | P0 | 4-6h |
| **RemediationOrchestrator** | ❌ Single-Controller | 📋 Planned | P1 | 4-6h |
| **SignalProcessing** | ❌ Single-Controller | 📋 Planned | P1 | 4-6h |
| **Notification** | ❌ Single-Controller | 📋 Planned | P1 | 4-6h |
| **Gateway** | N/A (no controller) | N/A | - | 0h |
| **DataStorage** | N/A (no controller) | N/A | - | 0h |

**Total Services Needing Migration**: 4  
**Total Estimated Effort**: 16-24 hours

---

**Document Owner**: Platform Architecture Team  
**Created**: 2026-01-10  
**Last Updated**: 2026-01-10  
**Next Review**: After Phase 1 pilot complete (AIAnalysis migration)  
**Status**: ✅ **APPROVED** - Authoritative standard for all CRD controller services

