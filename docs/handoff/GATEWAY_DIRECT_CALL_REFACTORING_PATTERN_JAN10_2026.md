# Gateway Integration Test Refactoring: HTTP to Direct Business Logic Calls

**Date**: January 10, 2026
**Phase**: HTTP Anti-Pattern Refactoring - Phase 4 (Gateway)
**Status**: 🚧 IN PROGRESS

---

## 🎯 **Objective**

Refactor 3 Gateway integration tests from HTTP anti-pattern to direct business logic calls:
1. **`adapter_interaction_test.go`** - Adapter pipeline transformation
2. **`k8s_api_integration_test.go`** - K8s API operations (CRD creation)
3. **`k8s_api_interaction_test.go`** - K8s API interaction (full pipeline)

**Estimated Effort**: 4 hours

---

## 📚 **Gateway Business Logic Components**

### 1. SignalAdapter Interface (`pkg/gateway/adapters/adapter.go`)

```go
type SignalAdapter interface {
    Name() string
    Parse(ctx context.Context, rawData []byte) (*types.NormalizedSignal, error)
    Validate(signal *types.NormalizedSignal) error
    GetMetadata() AdapterMetadata
    GetSourceService() string // e.g., "prometheus"
    GetSourceType() string     // e.g., "prometheus-alert"
}
```

**Implementations**:
- `pkg/gateway/adapters/prometheus_adapter.go` - `PrometheusAdapter`
- `pkg/gateway/adapters/kubernetes_event_adapter.go` - `KubernetesEventAdapter`

**Constructor**:
```go
// No dependencies, can instantiate directly
adapter := adapters.NewPrometheusAdapter()
```

### 2. CRDCreator (`pkg/gateway/processing/crd_creator.go`)

```go
type CRDCreator struct {
    k8sClient         k8s.ClientInterface
    logger            logr.Logger
    metrics           *metrics.Metrics
    fallbackNamespace string
    retryConfig       *config.RetrySettings
    clock             Clock
}

func NewCRDCreator(
    k8sClient k8s.ClientInterface,
    logger logr.Logger,
    metricsInstance *metrics.Metrics,
    fallbackNamespace string,
    retryConfig *config.RetrySettings,
) *CRDCreator
```

**Key Method**:
```go
func (c *CRDCreator) CreateRemediationRequest(
    ctx context.Context,
    signal *types.NormalizedSignal,
) (*remediationv1alpha1.RemediationRequest, error)
```

### 3. PhaseBasedDeduplicationChecker (`pkg/gateway/processing/phase_checker.go`)

```go
type PhaseBasedDeduplicationChecker struct {
    client client.Client
}

func NewPhaseBasedDeduplicationChecker(k8sClient client.Client) *PhaseBasedDeduplicationChecker
```

**Key Method**:
```go
func (c *PhaseBasedDeduplicationChecker) ShouldDeduplicate(
    ctx context.Context,
    namespace, fingerprint string,
) (bool, *remediationv1alpha1.RemediationRequest, error)
```

**Returns**:
- `bool`: `true` if duplicate (should deduplicate), `false` if unique (process)
- `*RemediationRequest`: Existing RR if duplicate, `nil` if unique
- `error`: Error during check

---

## 🔄 **Refactoring Pattern: HTTP → Direct Calls**

### ❌ **BEFORE: HTTP Anti-Pattern**

```go
var (
    testServer    *httptest.Server
    gatewayServer *gateway.Server
)

BeforeEach(func() {
    // Start Gateway server
    gatewayServer, _ = StartTestGateway(ctx, k8sClient, dataStorageURL)
    
    // Create HTTP test server
    testServer = httptest.NewServer(gatewayServer.Handler())
})

It("should process Prometheus alert", func() {
    payload := GeneratePrometheusAlert(...)
    
    // HTTP call
    resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
    
    // Validate HTTP status
    Expect(resp.StatusCode).To(Equal(201))
    
    // Verify CRD created
    Eventually(func() error {
        // ...
    }).Should(Succeed())
})
```

### ✅ **AFTER: Direct Business Logic Calls**

```go
var (
    adapter      adapters.SignalAdapter
    crdCreator   *processing.CRDCreator
    dedupChecker *processing.PhaseBasedDeduplicationChecker
    logger       logr.Logger
    metrics      *metrics.Metrics
)

BeforeEach(func() {
    // Setup logger
    logger = logr.Discard() // Or use test logger
    
    // Setup metrics (required by CRDCreator)
    metrics = metrics.New("gateway-test")
    
    // Initialize adapters
    adapter = adapters.NewPrometheusAdapter()
    
    // Initialize deduplication checker
    dedupChecker = processing.NewPhaseBasedDeduplicationChecker(k8sClient.Client)
    
    // Initialize CRD creator
    retryConfig := &config.RetrySettings{
        MaxRetries: 3,
        BaseDelay:  time.Millisecond * 100,
    }
    crdCreator = processing.NewCRDCreator(
        k8sClient, // k8s.ClientInterface
        logger,
        metrics,
        testNamespace, // fallbackNamespace
        retryConfig,
    )
})

It("should process Prometheus alert", func() {
    // Generate raw payload
    payload := GeneratePrometheusAlert(...)
    
    // Step 1: Parse payload (adapter)
    signal, err := adapter.Parse(ctx, payload)
    Expect(err).ToNot(HaveOccurred())
    Expect(signal).ToNot(BeNil())
    
    // Step 2: Validate signal (adapter)
    err = adapter.Validate(signal)
    Expect(err).ToNot(HaveOccurred())
    
    // Step 3: Check deduplication
    shouldDedup, existingRR, err := dedupChecker.ShouldDeduplicate(
        ctx,
        testNamespace,
        signal.Fingerprint,
    )
    Expect(err).ToNot(HaveOccurred())
    Expect(shouldDedup).To(BeFalse(), "First signal should NOT be duplicate")
    Expect(existingRR).To(BeNil())
    
    // Step 4: Create CRD
    rr, err := crdCreator.CreateRemediationRequest(ctx, signal)
    Expect(err).ToNot(HaveOccurred())
    Expect(rr).ToNot(BeNil())
    Expect(rr.Name).ToNot(BeEmpty())
    
    // Step 5: Verify CRD in Kubernetes
    Eventually(func() bool {
        var created remediationv1alpha1.RemediationRequest
        err := k8sClient.Client.Get(ctx, client.ObjectKey{
            Name:      rr.Name,
            Namespace: rr.Namespace,
        }, &created)
        return err == nil
    }, "30s", "500ms").Should(BeTrue())
    
    // Business validation
    var finalRR remediationv1alpha1.RemediationRequest
    err = k8sClient.Client.Get(ctx, client.ObjectKey{
        Name:      rr.Name,
        Namespace: rr.Namespace,
    }, &finalRR)
    Expect(err).ToNot(HaveOccurred())
    Expect(finalRR.Spec.SignalType).To(Equal("prometheus-alert"))
    Expect(finalRR.Spec.SignalSource).To(Equal("prometheus"))
    Expect(finalRR.Spec.Severity).To(Equal("critical"))
})
```

---

## 🧪 **Test Case Refactoring Patterns**

### Pattern 1: First Alert Processing (No Duplication)

```go
It("should process first alert (no duplication)", func() {
    payload := GeneratePrometheusAlert(...)
    
    // Parse → Validate → Check Dedup (false) → Create CRD → Verify
    signal, _ := adapter.Parse(ctx, payload)
    _ = adapter.Validate(signal)
    
    shouldDedup, existingRR, _ := dedupChecker.ShouldDeduplicate(ctx, ns, signal.Fingerprint)
    Expect(shouldDedup).To(BeFalse())
    Expect(existingRR).To(BeNil())
    
    rr, _ := crdCreator.CreateRemediationRequest(ctx, signal)
    Expect(rr).ToNot(BeNil())
    
    // Verify CRD exists
    Eventually(func() bool {
        var created remediationv1alpha1.RemediationRequest
        err := k8sClient.Client.Get(ctx, client.ObjectKey{Name: rr.Name, Namespace: rr.Namespace}, &created)
        return err == nil
    }).Should(BeTrue())
})
```

### Pattern 2: Duplicate Alert Detection

```go
It("should detect duplicate alert", func() {
    payload := GeneratePrometheusAlert(...)
    
    // Process first alert
    signal, _ := adapter.Parse(ctx, payload)
    _ = adapter.Validate(signal)
    
    shouldDedup1, _, _ := dedupChecker.ShouldDeduplicate(ctx, ns, signal.Fingerprint)
    Expect(shouldDedup1).To(BeFalse(), "First alert should NOT be duplicate")
    
    rr1, _ := crdCreator.CreateRemediationRequest(ctx, signal)
    Eventually(func() bool {
        var created remediationv1alpha1.RemediationRequest
        err := k8sClient.Client.Get(ctx, client.ObjectKey{Name: rr1.Name, Namespace: rr1.Namespace}, &created)
        return err == nil
    }).Should(BeTrue())
    
    // Wait for K8s to index the new RR (deduplication queries by fingerprint)
    time.Sleep(1 * time.Second)
    
    // Process duplicate alert
    signal2, _ := adapter.Parse(ctx, payload) // Same payload → same fingerprint
    _ = adapter.Validate(signal2)
    
    // Should detect duplicate
    Eventually(func() bool {
        shouldDedup2, existingRR, err := dedupChecker.ShouldDeduplicate(ctx, ns, signal2.Fingerprint)
        if err != nil {
            return false
        }
        return shouldDedup2 && existingRR != nil && existingRR.Name == rr1.Name
    }, "20s", "1s").Should(BeTrue(), "Duplicate alert should be detected")
    
    // IMPORTANT: Do NOT call crdCreator.CreateRemediationRequest for duplicates
    // The deduplication checker returned true, so we skip CRD creation
    
    // Verify still only 1 CRD
    Eventually(func() int {
        crdList := &remediationv1alpha1.RemediationRequestList{}
        _ = k8sClient.Client.List(ctx, crdList, client.InNamespace(ns))
        return len(crdList.Items)
    }).Should(Equal(1), "Should still have only 1 CRD")
})
```

### Pattern 3: Error Handling (Invalid Payload)

```go
It("should reject invalid payload", func() {
    invalidPayload := []byte(`{"invalid": "json"}`)
    
    // Parse should fail
    signal, err := adapter.Parse(ctx, invalidPayload)
    Expect(err).To(HaveOccurred(), "Invalid payload should fail parsing")
    Expect(signal).To(BeNil())
    
    // No CRD should be created
    Eventually(func() int {
        crdList := &remediationv1alpha1.RemediationRequestList{}
        _ = k8sClient.Client.List(ctx, crdList, client.InNamespace(ns))
        return len(crdList.Items)
    }, "5s").Should(Equal(0), "No CRD should be created for invalid payload")
})
```

### Pattern 4: Validation Errors

```go
It("should reject signal with missing required fields", func() {
    // Generate payload missing required fields
    payload := GeneratePrometheusAlert(PrometheusAlertOptions{
        AlertName: "", // Missing alertname
        Namespace: testNamespace,
        Severity:  "critical",
    })
    
    // Parse succeeds (valid JSON)
    signal, err := adapter.Parse(ctx, payload)
    Expect(err).ToNot(HaveOccurred())
    
    // Validate should fail
    err = adapter.Validate(signal)
    Expect(err).To(HaveOccurred(), "Signal missing required fields should fail validation")
    
    // No CRD creation should be attempted
})
```

---

## 🚨 **Key Differences: HTTP vs Direct Calls**

| Aspect | HTTP Anti-Pattern | Direct Business Logic Calls |
|---|---|---|
| **Test Focus** | HTTP status codes (201, 202, 400) | Business outcomes (CRD created, deduplication works) |
| **Validation** | `Expect(resp.StatusCode).To(Equal(201))` | `Expect(err).ToNot(HaveOccurred())` |
| **Duplication** | HTTP 202 Accepted | `shouldDedup == true` |
| **Errors** | HTTP 400 Bad Request | `adapter.Parse()` or `adapter.Validate()` returns error |
| **Infrastructure** | `httptest.Server`, `gateway.Server` | Direct component initialization |
| **Dependencies** | Full HTTP stack | Only business logic components |
| **Test Tier** | E2E (tests HTTP transport) | Integration (tests component coordination) |

---

## 📝 **Implementation Checklist**

### For Each Test File:

- [ ] **Remove HTTP Infrastructure**
  - [ ] Remove `testServer *httptest.Server` variable
  - [ ] Remove `gatewayServer *gateway.Server` variable
  - [ ] Remove `StartTestGateway()` call in `BeforeEach`
  - [ ] Remove `httptest.NewServer()` call
  - [ ] Remove `testServer.Close()` in `AfterEach`

- [ ] **Add Business Logic Components**
  - [ ] Add `adapter adapters.SignalAdapter` variable
  - [ ] Add `crdCreator *processing.CRDCreator` variable
  - [ ] Add `dedupChecker *processing.PhaseBasedDeduplicationChecker` variable
  - [ ] Add `logger logr.Logger` variable
  - [ ] Add `metrics *metrics.Metrics` variable

- [ ] **Initialize Components in BeforeEach**
  - [ ] Initialize logger (`logr.Discard()` or test logger)
  - [ ] Initialize metrics (`metrics.New("gateway-test")`)
  - [ ] Initialize adapter (`adapters.NewPrometheusAdapter()`)
  - [ ] Initialize dedupChecker (`processing.NewPhaseBasedDeduplicationChecker(k8sClient.Client)`)
  - [ ] Initialize crdCreator (`processing.NewCRDCreator(...)`)

- [ ] **Refactor Test Cases**
  - [ ] Replace `SendWebhook()` → `adapter.Parse()` + `adapter.Validate()`
  - [ ] Replace HTTP status checks → error checks
  - [ ] Add explicit deduplication check (`dedupChecker.ShouldDeduplicate()`)
  - [ ] Replace HTTP 202 checks → `shouldDedup == true`
  - [ ] Add CRD creation call (`crdCreator.CreateRemediationRequest()`)
  - [ ] Keep CRD verification (`Eventually` check for CRD in K8s)

- [ ] **Update Imports**
  - [ ] Remove `net/http`, `net/httptest`
  - [ ] Add `github.com/jordigilh/kubernaut/pkg/gateway/adapters`
  - [ ] Add `github.com/jordigilh/kubernaut/pkg/gateway/processing`
  - [ ] Add `github.com/jordigilh/kubernaut/pkg/gateway/config`
  - [ ] Add `github.com/jordigilh/kubernaut/pkg/gateway/metrics`
  - [ ] Add `github.com/go-logr/logr`

- [ ] **Update Comments**
  - [ ] Update test headers to remove HTTP references
  - [ ] Update business outcome descriptions
  - [ ] Add comments explaining direct business logic flow

---

## 🎯 **Success Criteria**

After refactoring, tests should:
1. ✅ **Not use HTTP**: No `httptest.Server`, no HTTP status code checks
2. ✅ **Test business logic**: Direct component calls, error checks
3. ✅ **Still test integration**: Components coordinate with real K8s API
4. ✅ **Pass all test cases**: All existing test scenarios still covered
5. ✅ **Be faster**: No HTTP server startup overhead
6. ✅ **Be clearer**: Explicit business logic flow visible in test code

---

## 📊 **Expected Impact**

**Before (HTTP Anti-Pattern)**:
- 3 test files use HTTP stack unnecessarily
- Test focus on HTTP transport layer
- Slower execution (HTTP server startup)
- Less clear what business logic is being tested

**After (Direct Business Logic Calls)**:
- 3 test files use direct component calls
- Test focus on business logic coordination
- Faster execution (no HTTP overhead)
- Clear visibility into adapter → dedup → CRD pipeline

---

## 🔗 **References**

- **Approved Execution Plan**: `HTTP_ANTIPATTERN_REFACTORING_ANSWERS_JAN10_2026.md` (Q4)
- **Reconnaissance Report**: `HTTP_ANTIPATTERN_RECONNAISSANCE_JAN10_2026.md`
- **Gateway Adapters**: `pkg/gateway/adapters/adapter.go`
- **CRD Creator**: `pkg/gateway/processing/crd_creator.go`
- **Deduplication Checker**: `pkg/gateway/processing/phase_checker.go`
- **DD-TESTING-001**: Integration vs E2E test boundaries

---

## 📅 **Progress Tracking**

| Test File | Status | Complexity | Estimated Time |
|---|---|---|---|
| `adapter_interaction_test.go` | 🚧 TODO | Medium | 1.5 hours |
| `k8s_api_integration_test.go` | 📋 TODO | Medium | 1.5 hours |
| `k8s_api_interaction_test.go` | 📋 TODO | High | 1 hour |

**Total**: 4 hours

---

**Status**: 📝 Guide created - Ready for implementation
**Next Step**: Refactor `adapter_interaction_test.go`
