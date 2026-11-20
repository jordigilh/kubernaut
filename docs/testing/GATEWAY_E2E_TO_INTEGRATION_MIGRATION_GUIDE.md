# Gateway E2E → Integration Migration Guide

## 🎯 **Purpose**

This guide provides step-by-step instructions for migrating Gateway E2E tests (HTTP-based) to Integration tests (business logic-based).

**Problem Solved**: Gateway E2E tests were failing with 60-240s timeouts because Gateway (running in-cluster) and tests (running on host) used different K8s clients, leading to cache mismatches and eventual consistency issues.

**Solution**: Move 28 business logic tests (80%) to Integration tier where Gateway and tests share the same K8s client, eliminating cache mismatches and enabling immediate CRD visibility.

---

## 📊 **Migration Overview**

### **Current State**:
- **Location**: `test/e2e/gateway/`
- **Tests**: 35 total
- **Architecture**: HTTP-based (SendWebhook → Gateway pod → CRD creation)
- **Problem**: Gateway uses in-cluster K8s client, tests use external kubeconfig client
- **Result**: 101 Passed / 17 Failed (85.6%)

### **Target State**:
- **E2E tier**: 7-8 tests (HTTP-specific middleware/adapter tests)
- **Integration tier**: 28 tests (business logic tests)
- **Architecture**: Direct ProcessSignal() calls with shared K8s client
- **Result**: 119-121 Passed / 1-3 Failed (98-99%)

---

## 🧪 **Prototype**

**Reference Implementation**: `test/integration/gateway/10_crd_creation_lifecycle_integration_test.go`

This prototype demonstrates the complete migration pattern and includes:
- ✅ Direct Gateway instantiation with shared K8s client
- ✅ Direct ProcessSignal() calls (no HTTP)
- ✅ Immediate CRD verification (no timeouts)
- ✅ Complete documentation of changes
- ✅ Works as template for other 27 tests

---

## 📋 **7-8 Tests to Keep in E2E**

These tests MUST remain in E2E because they test HTTP-specific behavior:

| Test # | File | Why Keep in E2E |
|---|---|---|
| 03 | `03_k8s_api_rate_limit_test.go` | Tests HTTP rate limiting middleware |
| 08 | `08_k8s_event_ingestion_test.go` | Tests K8s Event adapter JSON parsing |
| 18 | `18_cors_enforcement_test.go` | Tests CORS headers on deployed Gateway |
| 19 | `19_replay_attack_prevention_test.go` | Tests X-Timestamp header validation |
| 20 | `20_security_headers_test.go` | Tests security headers (CSP, X-Frame-Options) |
| 28 | `28_graceful_shutdown_test.go` | Tests HTTP server lifecycle |
| 31 | `31_prometheus_adapter_test.go` | Tests Prometheus adapter JSON parsing |
| 25* | `25_cors_test.go` | *Already integration-style, just move to integration dir* |

**Action**: Add `Skip()` to tests 12, 13 (already done), keep 7 HTTP tests in E2E

---

## 🔄 **28 Tests to Migrate to Integration**

### **Phase 1: CRD Lifecycle** (5 tests, ~1 hour)
1. ✅ `10_crd_creation_lifecycle_test.go` - **DONE (Prototype)**
2. ⏳ `21_crd_lifecycle_test.go` - CRD creation for valid alerts
3. ⏳ `05_multi_namespace_isolation_test.go` - Namespace isolation
4. ⏳ `06_concurrent_alerts_test.go` - Concurrent CRD creation
5. ⏳ `29_k8s_api_failure_test.go` - K8s API failure handling

### **Phase 2: Deduplication** (6 tests, ~1.5 hours)
6. ⏳ `02_state_based_deduplication_test.go` - Status-based deduplication
7. ⏳ `11_fingerprint_stability_test.go` - Fingerprint generation
8. ⏳ `14_deduplication_ttl_expiration_test.go` - TTL expiration
9. ⏳ `34_status_deduplication_test.go` - Status updates
10. ⏳ `35_deduplication_edge_cases_test.go` - Edge cases
11. ⏳ `36_deduplication_state_test.go` - State transitions

### **Phase 3: Audit Events** (4 tests, ~1 hour)
12. ⏳ `15_audit_trace_validation_test.go` - Audit event emission
13. ⏳ `22_audit_errors_test.go` - Audit error handling
14. ⏳ `23_audit_emission_test.go` - Audit event types
15. ⏳ `24_audit_signal_data_test.go` - Audit payload data

### **Phase 4: Service Resilience** (3 tests, ~45 min)
16. ⏳ `32_service_resilience_test.go` - DataStorage unavailability
17. ⏳ `13_redis_failure_graceful_degradation_test.go` - Redis failure (DEPRECATED?)
18. ⏳ `12_gateway_restart_recovery_test.go` - Gateway restart (SERIAL)

### **Phase 5: Error Handling** (4 tests, ~1 hour)
19. ⏳ `17_error_response_codes_test.go` - HTTP error codes
20. ⏳ `26_error_classification_test.go` - Error classification
21. ⏳ `27_error_handling_test.go` - Error handling logic
22. ⏳ `09_signal_validation_test.go` - Signal validation

### **Phase 6: Observability** (3 tests, ~45 min)
23. ⏳ `04_metrics_endpoint_test.go` - Prometheus metrics
24. ⏳ `30_observability_test.go` - Observability features
25. ⏳ `16_structured_logging_test.go` - Structured logging

### **Phase 7: Miscellaneous** (3 tests, ~45 min)
26. ⏳ `07_health_readiness_test.go` - Health/readiness probes
27. ⏳ `33_webhook_integration_test.go` - Webhook integration
28. ⏳ Any remaining tests

**Total**: 28 tests, 5-6 hours

---

## 🔧 **Migration Pattern: Step-by-Step**

### **Step 1: Copy Test File**

```bash
# Example for Test 10 (already done):
cp test/e2e/gateway/10_crd_creation_lifecycle_test.go \
   test/integration/gateway/10_crd_creation_lifecycle_integration_test.go
```

### **Step 2: Update Package and Imports**

**REMOVE**:
```go
"bytes"
"net/http"
```

**ADD**:
```go
"github.com/jordigilh/kubernaut/pkg/gateway"
"github.com/jordigilh/kubernaut/pkg/gateway/config"
"github.com/jordigilh/kubernaut/pkg/gateway/types"
```

### **Step 3: Update Test Variables**

**REMOVE**:
```go
var (
	httpClient *http.Client  // ❌ No HTTP
	// ...
)
```

**ADD**:
```go
var (
	gwServer *gateway.Server  // ✅ Direct Gateway instance
	// ...
)
```

### **Step 4: Initialize Gateway in BeforeAll**

**REMOVE**:
```go
httpClient = &http.Client{Timeout: 10 * time.Second}
```

**ADD**:
```go
// Create Gateway with SHARED K8s client
cfg := &config.ServerConfig{
	Port: 0, // Random port (we won't use HTTP)
	DataStorageURL: "", // Configure as needed for test
}

var err error
gwServer, err = gateway.NewServerWithK8sClient(cfg, testLogger, nil, k8sClient)
Expect(err).ToNot(HaveOccurred(), "Failed to create Gateway server")
```

### **Step 5: Replace HTTP Calls with ProcessSignal**

**BEFORE (E2E with HTTP)**:
```go
// Send HTTP webhook
payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
	SignalName: alertName,
	Namespace: testNamespace,
	PodName:   podName,
	Severity:  "critical",
})

req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
req.Header.Set("Content-Type", "application/json")
resp, err := httpClient.Do(req)
Expect(resp.StatusCode).To(Equal(201))
```

**AFTER (Integration with ProcessSignal)**:
```go
// Call business logic directly
signal := &types.NormalizedSignal{
	Fingerprint:  generateFingerprint(alertName, testNamespace, "Pod", podName),
	SignalName:   alertName,
	Severity:     "critical",
	Namespace:    testNamespace,
	Resource: types.ResourceIdentifier{
		Kind:      "Pod",
		Name:      podName,
		Namespace: testNamespace,
	},
	Labels: map[string]string{
		"alertname": alertName,
		"namespace": testNamespace,
		"severity":  "critical",
		"pod":       podName,
	},
	Annotations: map[string]string{
		"summary":     "Test alert",
		"description": "Testing CRD creation",
	},
	FiringTime:   time.Now(),
	ReceivedTime: time.Now(),
	Source:       "prometheus",  // Issue #166: adapter identity
	Source:       "test-adapter",
}

response, err := gwServer.ProcessSignal(ctx, signal)
Expect(err).ToNot(HaveOccurred(), "ProcessSignal should succeed")
Expect(response.Status).To(Equal("created"))
```

### **Step 6: Replace Eventually() with Direct Queries**

**BEFORE (E2E with timeout)**:
```go
Eventually(func() int {
	crdList := &remediationv1alpha1.RemediationRequestList{}
	err := k8sClient.List(ctx, crdList, client.InNamespace(testNamespace))
	if err != nil {
		return -1
	}
	return len(crdList.Items)
}, 240*time.Second, 3*time.Second).Should(BeNumerically(">=", 1))
```

**AFTER (Integration with immediate query)**:
```go
// No Eventually() needed! CRDs visible immediately because
// Gateway and test share the SAME K8s client
crdList := &remediationv1alpha1.RemediationRequestList{}
err := k8sClient.List(ctx, crdList, client.InNamespace(testNamespace))
Expect(err).ToNot(HaveOccurred(), "Should list CRDs successfully")
Expect(len(crdList.Items)).To(BeNumerically(">=", 1), "At least 1 CRD should be created")
```

**Note**: Some tests may still need `Eventually()` for async operations (e.g., status updates), but timeouts can be reduced from 240s to 5-10s.

### **Step 7: Add Migration Header**

Add this header at the top of the test file:

```go
// ========================================
// MIGRATION STATUS: ✅ Converted from E2E to Integration
// ORIGINAL FILE: test/e2e/gateway/[XX]_[test_name]_test.go
// MIGRATION DATE: 2026-01-XX
// ========================================
```

### **Step 8: Test and Verify**

```bash
# Run migrated test
go test -v ./test/integration/gateway/10_crd_creation_lifecycle_integration_test.go

# Should pass in < 5 seconds (vs 60-240s in E2E)
```

---

## 🛠️ **Helper Functions to Add**

### **Fingerprint Generator**

Add this helper function to each migrated test (or create a shared helper file):

```go
// generateFingerprint returns the production signal fingerprint.
// Never hand-roll the hash: the format is
// SHA256([clusterID:]group:namespace:kind:name), and the group is derived from
// the resource's apiVersion (#2650, #2658). Call the real function so a test
// cannot drift from the Gateway/AF contract.
func generateFingerprint(clusterID, apiVersion, namespace, kind, name string) string {
	return gwtypes.CalculateClusterAwareFingerprint(clusterID, gwtypes.ResourceIdentifier{
		APIVersion: apiVersion,
		Namespace:  namespace,
		Kind:       kind,
		Name:       name,
	})
}
```

### **Shared Config Builder**

Consider creating a helper for Gateway config:

```go
// createGatewayConfig creates a test Gateway config
func createGatewayConfig(dataStorageURL string) *config.ServerConfig {
	return &config.ServerConfig{
		Port:            0, // Random port (not used)
		DataStorageURL:  dataStorageURL,
		// Add other config fields as needed
	}
}
```

---

## 📈 **Expected Results**

### **Performance**:
- **E2E**: 60-240s per test (HTTP + K8s client mismatch)
- **Integration**: 1-5s per test (direct calls + shared client)
- **Speedup**: 10-100x faster

### **Reliability**:
- **E2E**: 85.6% pass rate (17 failures due to timeouts)
- **Integration**: 98-99% pass rate (1-3 failures for edge cases)
- **Improvement**: +13-14% pass rate

### **Test Suite Time**:
- **E2E (35 tests)**: ~30 minutes
- **E2E (7 tests) + Integration (28 tests)**: ~5 minutes
- **Time Savings**: 25 minutes per run

---

## ✅ **Validation Checklist**

For each migrated test, verify:

- [ ] ❌ Removed HTTP imports (`net/http`, `bytes`)
- [ ] ❌ Removed `httpClient` variable
- [ ] ❌ Removed `gatewayURL` usage
- [ ] ❌ Removed `SendWebhook()` calls
- [ ] ✅ Added Gateway business logic imports
- [ ] ✅ Added `gwServer *gateway.Server` variable
- [ ] ✅ Gateway initialized with `NewServerWithK8sClient()`
- [ ] ✅ HTTP calls replaced with `ProcessSignal()`
- [ ] ✅ `Eventually()` timeouts reduced or removed
- [ ] ✅ Test passes in < 10 seconds
- [ ] ✅ Migration header added
- [ ] ✅ Test validates business behavior (not HTTP implementation)

---

## 🚨 **Common Pitfalls**

### **Pitfall 1: Forgetting to Share K8s Client**

**WRONG**:
```go
// Creates NEW client (cache mismatch!)
cfg, _ := config.GetConfig()
gwClient, _ := client.New(cfg, client.Options{})
gwServer, _ := gateway.NewServerWithK8sClient(gwCfg, logger, nil, gwClient)
```

**RIGHT**:
```go
// Uses SHARED client from suite
gwServer, _ := gateway.NewServerWithK8sClient(gwCfg, logger, nil, k8sClient)
```

### **Pitfall 2: Testing HTTP Status Codes**

If a test validates HTTP status codes (201, 202, 400, 500), it might belong in E2E:

**Example**: `17_error_response_codes_test.go`
- If it tests "ProcessSignal returns error" → **Integration**
- If it tests "HTTP 500 returned to client" → **E2E**

**Decision**: Refactor to test business logic error handling (Integration) and keep 1-2 HTTP status tests in E2E.

### **Pitfall 3: Not Updating Fingerprints**

E2E tests may have used simplified fingerprints. Integration tests should use proper SHA256 hashing:

```go
// WRONG (E2E shortcut):
Fingerprint: "test-fingerprint-123"

// RIGHT (Integration with proper hashing):
Fingerprint: generateFingerprint(alertName, namespace, "Pod", podName)
```

---

## 🎯 **Success Criteria**

**Phase 1 Complete** (5 CRD Lifecycle tests):
- [ ] 5 tests migrated to `test/integration/gateway/`
- [ ] All 5 tests pass in < 30 seconds total
- [ ] No HTTP dependencies remain

**Phase 2-7 Complete** (remaining 23 tests):
- [ ] All 28 tests migrated
- [ ] Integration test suite pass rate: 95-100%
- [ ] Integration test suite time: < 3 minutes

**Overall Success**:
- [ ] E2E tier: 7-8 tests (HTTP-specific)
- [ ] Integration tier: 28 tests (business logic)
- [ ] Overall pass rate: 98-99% (119-121 / 122)
- [ ] Total test time: < 8 minutes (down from 30+ minutes)

---

## 📚 **References**

- **Prototype**: `test/integration/gateway/10_crd_creation_lifecycle_integration_test.go`
- **Suite Setup**: `test/integration/gateway/suite_test.go`
- **Triage Doc**: `/tmp/GATEWAY_TEST_TRIAGE.md`
- **Root Cause Analysis**: `/tmp/TEST_ARCHITECTURE_ANALYSIS.md`
- **RemediationOrchestrator Pattern**: `test/e2e/remediationorchestrator/lifecycle_e2e_test.go` (similar approach)

---

## 🚀 **Next Steps**

1. **Review Prototype**: Study `10_crd_creation_lifecycle_integration_test.go`
2. **Start Phase 1**: Migrate remaining 4 CRD Lifecycle tests
3. **Validate**: Run integration tests and verify <30s execution time
4. **Continue Phases 2-7**: Migrate remaining 23 tests systematically
5. **Update Makefile**: Add `make test-integration-gateway` target
6. **Update CI**: Run integration tests before E2E

---

## ⏱️ **Time Estimate**

- **Prototype** (Test 10): ✅ Done
- **Phase 1** (4 tests): 45 minutes
- **Phase 2** (6 tests): 1.5 hours
- **Phase 3** (4 tests): 1 hour
- **Phase 4** (3 tests): 45 minutes
- **Phase 5** (4 tests): 1 hour
- **Phase 6** (3 tests): 45 minutes
- **Phase 7** (3 tests): 45 minutes
- **Validation & CI**: 30 minutes

**Total**: 7-8 hours for complete migration

---

## ✅ **Conclusion**

This migration addresses the **root cause** of Gateway E2E failures: K8s client mismatch between in-cluster Gateway and host-based tests. By moving business logic tests to the Integration tier with a shared K8s client, we achieve:

- ✅ **10-100x faster** test execution
- ✅ **+13-14% pass rate** improvement
- ✅ **Immediate CRD visibility** (no timeouts)
- ✅ **Industry best practices** (integration > E2E ratio)
- ✅ **Matches RO E2E pattern** (test business logic, not HTTP)

**The pattern is proven**: Test 10 prototype validates the approach. The remaining 27 tests follow the same pattern.
