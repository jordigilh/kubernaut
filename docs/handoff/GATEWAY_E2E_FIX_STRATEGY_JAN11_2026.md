# Gateway E2E Test Fix Strategy

**Date**: January 11, 2026
**Status**: 🔧 IN PROGRESS - Systematic Fixing
**Current State**: 43/109 passing (39%), 66 failures, 13 skipped, 1 panic

---

## 🎯 Executive Summary

**Infrastructure**: ✅ **WORKING**
- Kind cluster created successfully (3min 27s)
- Gateway deployed at `http://127.0.0.1:8080`
- DataStorage deployed at `http://127.0.0.1:18091`
- PostgreSQL + Redis operational

**Root Cause**: Tests migrated from integration to E2E tier but still using integration patterns (direct business logic calls vs HTTP requests)

**Fix Strategy**: Systematic refactoring to use proper E2E patterns with HTTP requests

---

## 📋 Failure Analysis

### **Category 1: HTTP Endpoint Issues** (~50 tests)

**Symptom**: Tests receiving `HTTP 404 (Not Found)` when expecting `201 (Created)`

**Root Cause**: Tests using incorrect or missing HTTP endpoints

**Examples**:
```
Expected: <int>: 201
Actual:   <int>: 404
```

**Tests Affected**:
- `31_prometheus_adapter_test.go` - BR-GATEWAY-001 (Prometheus alert processing)
- `33_webhook_integration_test.go` - BR-GATEWAY-001-015 (End-to-end webhook)
- Multiple deduplication tests
- Environment classification tests

**Fix Required**:
1. Ensure tests use correct Gateway endpoints:
   - Prometheus: `POST http://127.0.0.1:8080/api/v1/signals/prometheus`
   - K8s Events: `POST http://127.0.0.1:8080/api/v1/signals/kubernetes-event`
2. Use proper Prometheus AlertManager webhook format (see `createPrometheusWebhookPayload`)
3. Add required headers: `Content-Type: application/json`, `X-Timestamp: <unix-timestamp>`

---

### **Category 2: Test Infrastructure Issues** (~10 tests)

**Symptom**: Tests failing in `BeforeAll` or `BeforeEach` setup

**Root Cause**: Missing namespace creation, K8s client setup, or test data preparation

**Examples**:
```
[FAILED] Failed to create test namespace
Expected success, but got an error
```

**Tests Affected**:
- `05_multi_namespace_isolation_test.go` - BeforeAll failure
- `02_state_based_deduplication_test.go` - BeforeAll failure
- `14_deduplication_ttl_expiration_test.go` - BeforeAll failure
- `06_concurrent_alerts_test.go` - BeforeAll failure

**Fix Required**:
1. Ensure namespaces are created with proper labels (environment classification)
2. Wait for namespace creation to complete (async operation)
3. Initialize K8s client using `getKubernetesClient()` helper
4. Use `GenerateUniqueNamespace()` for test isolation

---

### **Category 3: DataStorage Query Issues** (~5 tests)

**Symptom**: Audit event queries not finding expected events

**Root Cause**: Tests may be querying before events are flushed, or using incorrect query parameters

**Tests Affected**:
- `15_audit_trace_validation_test.go` - Audit event emission
- `22_audit_errors_test.go` - Error audit standardization (if exists in E2E)
- `24_audit_signal_data_test.go` - Signal data capture (if exists in E2E)

**Fix Required**:
1. Use `Eventually` with proper timeout for audit queries (events are buffered)
2. Query DataStorage at `http://127.0.0.1:18091`
3. Use correct correlation ID from Gateway response
4. Wait for audit buffer flush (default: 60s flush interval)

---

### **Category 4: Panic** (1 test)

**Symptom**: Test panicking during execution

**Test Affected**:
- `32_service_resilience_test.go` - Combined infrastructure failures (GW-RES-003)

**Root Cause**: Likely nil pointer or unexpected infrastructure state

**Fix Required**:
1. Add nil checks before accessing response data
2. Ensure K8s client is initialized before use
3. Handle DataStorage unavailability gracefully
4. Add proper error recovery in test logic

---

## 🔧 Fix Implementation Plan

### **Phase 1: Fix HTTP Webhook Pattern** (Priority 1)

**Objective**: Get basic HTTP → CRD flow working

**Files to Fix** (Estimated: 30-40 tests):
1. `31_prometheus_adapter_test.go` - Prometheus alert processing
2. `33_webhook_integration_test.go` - End-to-end webhook processing
3. `27_error_handling_test.go` - Error handling & edge cases
4. `28_graceful_shutdown_test.go` - Concurrent request handling
5. `29_k8s_api_failure_test.go` - K8s API recovery

**Fix Pattern**:
```go
// ❌ WRONG (Integration pattern - direct business logic call)
signal, err := prometheusAdapter.Parse(ctx, payload)
Expect(err).ToNot(HaveOccurred())
_, err = crdCreator.CreateRemediationRequest(ctx, signal)

// ✅ CORRECT (E2E pattern - HTTP request)
gatewayURL := "http://127.0.0.1:8080"
webhookPayload := createPrometheusWebhookPayload(PrometheusAlertPayload{
    AlertName: "HighMemoryUsage",
    Namespace: testNamespace,
    Severity:  "critical",
})

resp := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", webhookPayload)
Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Gateway should create CRD")

// Verify CRD was created
Eventually(func() int {
    return len(ListRemediationRequests(ctx, k8sClient, testNamespace))
}, "30s").Should(Equal(1), "CRD should be created")
```

**Key Changes**:
1. Remove direct adapter/CRDCreator usage
2. Use `sendWebhook()` helper for HTTP POST
3. Use `createPrometheusWebhookPayload()` for proper format
4. Query K8s to verify CRD creation
5. Use `Eventually` for async operations

---

### **Phase 2: Fix Test Infrastructure** (Priority 1)

**Objective**: Fix BeforeAll/BeforeEach setup failures

**Files to Fix** (Estimated: 10 tests):
1. `05_multi_namespace_isolation_test.go`
2. `02_state_based_deduplication_test.go`
3. `14_deduplication_ttl_expiration_test.go`
4. `06_concurrent_alerts_test.go`
5. `03_k8s_api_rate_limit_test.go`
6. `08_k8s_event_ingestion_test.go`
7. `16_structured_logging_test.go`
8. `20_security_headers_test.go`

**Fix Pattern**:
```go
BeforeAll(func() {
    // Initialize K8s client
    k8sClient = getKubernetesClient()
    Expect(k8sClient).ToNot(BeNil(), "K8s client required")

    // Create unique test namespaces
    prodNs := GenerateUniqueNamespace("prod")
    stagingNs := GenerateUniqueNamespace("staging")

    // Create namespaces with environment labels
    for _, nsConfig := range []struct{name, env string}{
        {prodNs, "production"},
        {stagingNs, "staging"},
    } {
        ns := &corev1.Namespace{}
        ns.Name = nsConfig.name
        ns.Labels = map[string]string{"environment": nsConfig.env}

        err := k8sClient.Create(context.Background(), ns)
        Expect(err).ToNot(HaveOccurred(), "Namespace creation should succeed")

        // Wait for namespace to be ready
        Eventually(func() error {
            checkNs := &corev1.Namespace{}
            return k8sClient.Get(context.Background(),
                client.ObjectKey{Name: nsConfig.name}, checkNs)
        }, "10s").Should(Succeed(), "Namespace should become available")
    }
})
```

---

### **Phase 3: Fix Deduplication Tests** (Priority 2)

**Objective**: Fix state-based deduplication logic

**Files to Fix** (Estimated: 15-20 tests):
1. `34_status_deduplication_test.go` - Status-based tracking
2. `35_deduplication_edge_cases_test.go` - Edge cases
3. `36_deduplication_state_test.go` - State transitions
4. `26_error_classification_test.go` - Error classification & retry

**Fix Pattern**:
```go
It("should detect duplicate and increment occurrence count", func() {
    // Send first alert
    payload1 := createPrometheusWebhookPayload(PrometheusAlertPayload{
        AlertName: "TestAlert",
        Namespace: testNs,
        Severity:  "critical",
    })
    resp1 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", payload1)
    Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

    // Wait for CRD
    var rr *remediationv1alpha1.RemediationRequest
    Eventually(func() int {
        rrs := ListRemediationRequests(ctx, k8sClient, testNs)
        if len(rrs) > 0 {
            rr = &rrs[0]
        }
        return len(rrs)
    }, "30s").Should(Equal(1))

    // Send duplicate alert
    resp2 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", payload1)
    Expect(resp2.StatusCode).To(Equal(http.StatusAccepted), "Duplicate should be accepted")

    // Verify deduplication count incremented
    Eventually(func() int {
        k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
        return rr.Status.Deduplication.Count
    }, "10s").Should(BeNumerically(">", 1), "Duplicate count should increment")
})
```

---

### **Phase 4: Fix DataStorage Queries** (Priority 2)

**Objective**: Fix audit event validation

**Files to Fix** (Estimated: 5 tests):
1. `15_audit_trace_validation_test.go`

**Fix Pattern**:
```go
It("should emit audit event when signal is ingested", func() {
    // Send signal
    payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
        AlertName: "AuditTest",
        Namespace: testNs,
        Severity:  "warning",
    })
    resp := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", payload)
    Expect(resp.StatusCode).To(Equal(http.StatusCreated))

    // Parse response for correlation ID
    var gwResp GatewayResponse
    json.Unmarshal(resp.Body, &gwResp)
    correlationID := gwResp.Fingerprint

    // Query DataStorage for audit event (with Eventually for buffer flush)
    dataStorageURL := "http://127.0.0.1:18091"
    Eventually(func() int {
        auditEvents := queryAuditEvents(dataStorageURL, correlationID, "gateway.signal.received")
        return len(auditEvents)
    }, "90s", "1s").Should(BeNumerically(">=", 1), "Audit event should be emitted")
})
```

---

### **Phase 5: Fix Panic Test** (Priority 3)

**Objective**: Fix infrastructure resilience test

**Files to Fix** (Estimated: 1 test):
1. `32_service_resilience_test.go`

**Investigation Required**:
1. Identify exact panic location
2. Add nil checks
3. Handle infrastructure failures gracefully
4. Consider skipping if testing infrastructure resilience is not feasible in E2E

---

## 📊 Success Metrics

**Target**: 100% E2E pass rate (109/109 passing, 0 failed)

**Intermediate Milestones**:
- Phase 1 complete: 70+ tests passing (HTTP patterns working)
- Phase 2 complete: 80+ tests passing (Infrastructure setup fixed)
- Phase 3 complete: 95+ tests passing (Deduplication logic fixed)
- Phase 4 complete: 100+ tests passing (DataStorage queries fixed)
- Phase 5 complete: 109 tests passing (All issues resolved)

---

## 🔍 Validation Commands

### **Run All E2E Tests**
```bash
make test-e2e-gateway
```

### **Run Specific Test File**
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
ginkgo -v --timeout=30m ./test/e2e/gateway/31_prometheus_adapter_test.go
```

### **Check Gateway Health**
```bash
curl http://127.0.0.1:8080/health
```

### **Verify Kind Cluster**
```bash
kubectl --kubeconfig ~/.kube/gateway-e2e-config get pods -n kubernaut-system
```

---

## 🎯 Current Status

**Analysis Phase**: ✅ **COMPLETE**
- Root causes identified
- Failure patterns categorized
- Fix strategy documented

**Implementation Phase**: 🔄 **IN PROGRESS**
- Helper function verified (`getKubernetesClient()` exists)
- Code compiles successfully
- Ready to start systematic fixes

---

## 📝 Next Actions

1. ✅ Complete analysis and categorization
2. ⏭️ Fix Phase 1: HTTP webhook patterns (~30-40 tests)
3. ⏭️ Fix Phase 2: Test infrastructure (~10 tests)
4. ⏭️ Fix Phase 3: Deduplication tests (~15-20 tests)
5. ⏭️ Fix Phase 4: DataStorage queries (~5 tests)
6. ⏭️ Fix Phase 5: Panic test (1 test)
7. ⏭️ Run full E2E suite to validate
8. ⏭️ Document final results

**Estimated Total Effort**: 2-3 days (systematic fixing across all categories)

---

**Document Status**: ✅ Ready for Implementation
**Created**: January 11, 2026
**Last Updated**: January 11, 2026
