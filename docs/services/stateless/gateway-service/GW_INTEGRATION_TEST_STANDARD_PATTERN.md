# Gateway Integration Test - Standard Pattern
**Status**: 🚨 **MANDATORY for ALL integration tests**
**Date**: January 15, 2026

---

## ❌ WRONG: Mock-Based Pattern (Unit Test)

```go
var _ = Describe("BR-GATEWAY-055: Signal Received Audit Events", func() {
    var (
        adapter       adapters.SignalAdapter
        auditStore    *MockAuditStore           // ❌ WRONG: Mock not allowed in integration tests
        ctx           context.Context
    )

    BeforeEach(func() {
        auditStore = NewMockAuditStore()         // ❌ WRONG: Creating mock
        ctx = context.Background()
    })

    It("should emit audit event", func() {
        adapter = prometheus.NewAdapter(auditStore)  // ❌ WRONG: Using mock
        signal, _ := adapter.Parse(ctx, alert)

        // ❌ WRONG: Querying mock
        events := auditStore.Events
        auditEvent := findEventByType(events, "gateway.signal.received")
    })
})
```

---

## ✅ CORRECT: Real DataStorage Pattern (Integration Test)

```go
var _ = Describe("BR-GATEWAY-055: Signal Received Audit Events", func() {
    var (
        dsClient     *api.Client                    // ✅ Real DataStorage client
        gateway      *gateway.Service               // ✅ Real Gateway service
        k8sClient    client.Client                  // ✅ Real K8s client (envtest/Kind)
        ctx          context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()

        // ✅ Connect to real DataStorage (Podman container)
        dsClient = suite.GetDataStorageClient()

        // ✅ Get real K8s client (from suite setup)
        k8sClient = suite.GetK8sClient()

        // ✅ Initialize real Gateway service with real dependencies
        gateway = gateway.NewService(dsClient, k8sClient, suite.GetLogger())
    })

    // Test ID: GW-INT-AUD-001
    It("[GW-INT-AUD-001] should emit gateway.signal.received audit event", func() {
        // Given: Prometheus alert
        alert := createTestPrometheusAlert()

        // When: Gateway processes signal (real business logic)
        correlationID, err := gateway.ProcessSignal(ctx, alert)
        Expect(err).ToNot(HaveOccurred())

        // Then: Query REAL DataStorage by correlation ID (parallel-safe)
        auditEvent := FindAuditEventByTypeAndCorrelationID(
            ctx,
            dsClient,                                                      // ✅ Real DataStorage
            api.GatewayAuditPayloadEventTypeGatewaySignalReceived,        // ✅ OpenAPI constant
            correlationID,                                                 // ✅ Test isolation
            30*time.Second,
        )

        Expect(auditEvent).ToNot(BeNil())
        Expect(auditEvent.CorrelationID).To(Equal(correlationID))

        // Parse typed payload
        gatewayPayload := ParseGatewayPayload(auditEvent)
        Expect(gatewayPayload.SignalType).To(Equal(api.GatewayAuditPayloadSignalTypePrometheusAlert))
    })
})
```

---

## 📋 Standard Patterns by Component

### **Pattern 1: Audit Emission Tests** (Scenarios 1.1-1.4)

```go
var _ = Describe("BR-GATEWAY-055: Signal Received Audit Events", func() {
    var (
        dsClient  *api.Client      // Real DataStorage
        gateway   *gateway.Service // Real Gateway
        k8sClient client.Client    // Real K8s
        ctx       context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        dsClient = suite.GetDataStorageClient()
        k8sClient = suite.GetK8sClient()
        gateway = gateway.NewService(dsClient, k8sClient, suite.GetLogger())
    })

    // Tests use gateway.ProcessSignal() and query dsClient
})
```

### **Pattern 2: CRD Creation Tests** (Scenario 1.2)

```go
var _ = Describe("BR-GATEWAY-056: CRD Created Audit Events", func() {
    var (
        dsClient    *api.Client
        gateway     *gateway.Service
        k8sClient   client.Client
        crdCreator  *processing.CRDCreator  // Real CRD creator
        ctx         context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        dsClient = suite.GetDataStorageClient()
        k8sClient = suite.GetK8sClient()

        // Initialize real CRD creator with real dependencies
        crdCreator = processing.NewCRDCreator(k8sClient, dsClient, suite.GetLogger())
    })

    // Tests use crdCreator.CreateRemediationRequest() and query dsClient
})
```

### **Pattern 3: Deduplication Tests** (Scenario 1.3)

```go
var _ = Describe("BR-GATEWAY-057: Signal Deduplicated Audit Events", func() {
    var (
        dsClient     *api.Client
        k8sClient    client.Client
        phaseChecker *processing.PhaseChecker  // Real phase checker
        ctx          context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        dsClient = suite.GetDataStorageClient()
        k8sClient = suite.GetK8sClient()

        // Initialize real phase checker with real dependencies
        phaseChecker = processing.NewPhaseChecker(k8sClient, dsClient, suite.GetLogger())
    })

    // Tests use phaseChecker.ShouldDeduplicate() and query dsClient
})
```

### **Pattern 4: Adapter Tests** (Scenarios 3.1-3.2)

```go
var _ = Describe("BR-GATEWAY-001: Prometheus Adapter Signal Parsing", func() {
    var (
        dsClient *api.Client
        adapter  adapters.SignalAdapter  // Real adapter
        ctx      context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        dsClient = suite.GetDataStorageClient()

        // Initialize real Prometheus adapter with DataStorage
        adapter = prometheus.NewAdapter(dsClient, suite.GetLogger())
    })

    // Tests use adapter.Parse() and query dsClient
})
```

### **Pattern 5: Metrics Tests** (Scenarios 2.1-2.3)

```go
var _ = Describe("BR-GATEWAY-066: Signals Received Metrics", func() {
    var (
        dsClient        *api.Client
        gateway         *gateway.Service
        k8sClient       client.Client
        metricsRegistry *prometheus.Registry  // Real Prometheus registry
        ctx             context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        dsClient = suite.GetDataStorageClient()
        k8sClient = suite.GetK8sClient()
        metricsRegistry = prometheus.NewRegistry()

        // Initialize Gateway with metrics
        gateway = gateway.NewServiceWithMetrics(dsClient, k8sClient, metricsRegistry, suite.GetLogger())
    })

    // Tests use gateway.ProcessSignal() and query metricsRegistry
})
```

---

## 🛠️ Suite Setup Pattern

### **BeforeSuite - Infrastructure Initialization**

```go
var (
    suite     *kind.IntegrationSuite
    dsClient  *api.Client
    k8sClient client.Client
)

var _ = BeforeSuite(func() {
    // Setup Kind cluster or envtest
    suite = kind.Setup("gateway-integration-test")

    // Connect to real DataStorage (Podman container)
    dsURL := os.Getenv("DATA_STORAGE_URL")
    if dsURL == "" {
        dsURL = "http://localhost:8080"  // Default Podman port
    }

    var err error
    dsClient, err = api.NewClient(dsURL)
    Expect(err).ToNot(HaveOccurred(), "Failed to connect to DataStorage")

    // Get K8s client
    k8sClient = suite.GetK8sClient()

    GinkgoWriter.Println("✅ Gateway integration test environment ready")
    GinkgoWriter.Printf("   DataStorage: %s\n", dsURL)
})

var _ = AfterSuite(func() {
    suite.Cleanup()
})
```

---

## 📝 Key Rules

### **MUST DO**
1. ✅ Use `dsClient` (real DataStorage client)
2. ✅ Use `k8sClient` (real K8s client from envtest/Kind)
3. ✅ Query by `correlationID` for test isolation
4. ✅ Use OpenAPI constants for event types
5. ✅ Initialize real business logic components with real dependencies

### **MUST NOT DO**
1. ❌ Use `MockAuditStore` or any mocks
2. ❌ Use `fake.NewClientBuilder()` for K8s (use real envtest/Kind)
3. ❌ Query `auditStore.Events` (in-memory mock)
4. ❌ Use magic strings for event types
5. ❌ Query DataStorage without correlation ID filter

---

## 🔍 Variable Naming Convention

| Old (Mock Pattern) | New (Real Pattern) | Type |
|--------------------|-------------------|------|
| `auditStore` | `dsClient` | `*api.Client` |
| `fake.NewClientBuilder()` | `suite.GetK8sClient()` | `client.Client` |
| `NewMockAuditStore()` | `suite.GetDataStorageClient()` | `*api.Client` |
| `auditStore.Events` | `dsClient.ListAuditEvents(...)` | Query call |

---

**Status**: ✅ **APPROVED** - Use this pattern for ALL 77 integration tests
**Authority**: INTEGRATION_E2E_NO_MOCKS_POLICY.md, 03-testing-strategy.mdc
