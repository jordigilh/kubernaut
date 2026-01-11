# E2E Gateway Build Error Triage - Jan 10, 2026

## 🚨 **CRITICAL BUILD ERRORS**

**Status**: ❌ E2E Gateway tests fail to compile
**Root Cause**: Tests moved from integration → E2E still using integration test patterns
**Affected Files**: 15 test files (22-36)

---

## 📋 **ERROR SUMMARY**

```bash
# github.com/jordigilh/kubernaut/test/e2e/gateway [github.com/jordigilh/kubernaut/test/e2e/gateway.test]
test/e2e/gateway/36_deduplication_state_test.go:660:40: undefined: PrometheusAlertOptions
test/e2e/gateway/36_deduplication_state_test.go:712:52: undefined: K8sTestClient
test/e2e/gateway/22_audit_errors_test.go:68:19: undefined: K8sTestClient
test/e2e/gateway/22_audit_errors_test.go:77:16: undefined: SetupK8sTestClient
test/e2e/gateway/22_audit_errors_test.go:101:3: undefined: EnsureTestNamespace
test/e2e/gateway/22_audit_errors_test.go:102:3: undefined: RegisterTestNamespace
test/e2e/gateway/22_audit_errors_test.go:105:24: undefined: StartTestGateway
test/e2e/gateway/23_audit_emission_test.go:94:22: undefined: K8sTestClient
test/e2e/gateway/23_audit_emission_test.go:105:16: undefined: SetupK8sTestClient
test/e2e/gateway/23_audit_emission_test.go:142:3: undefined: EnsureTestNamespace
... (many more errors)
```

---

## 🔍 **ROOT CAUSE ANALYSIS**

### **What Happened**
During Phase 3 of HTTP anti-pattern refactoring, 15 Gateway integration tests were moved from `test/integration/gateway` to `test/e2e/gateway` (tests 22-36). However, these tests were **NOT adapted** to E2E patterns - they still reference integration test helpers.

### **Why This is Wrong**
**Integration Tests** use:
- `K8sTestClient` - wrapper around envtest client
- `SetupK8sTestClient()` - creates envtest client
- `StartTestGateway()` - creates Gateway server instance
- `EnsureTestNamespace()`, `RegisterTestNamespace()` - integration-specific helpers
- `PrometheusAlertOptions` - integration test payload struct
- Direct business logic calls (NO HTTP)

**E2E Tests** use:
- `getKubernetesClient()` - real Kind cluster client
- `gatewayURL` - deployed Gateway service URL (from suite)
- `createPrometheusWebhookPayload()` - E2E payload helper
- `PrometheusAlertPayload` - E2E payload struct
- HTTP requests to deployed Gateway service

---

## 🛠️ **FIX STRATEGY**

### **Option A: Adapt Tests to E2E Patterns** (RECOMMENDED)
**Effort**: ~2-3 hours
**Impact**: Tests become true E2E tests
**Approach**: Systematically refactor each test file

#### **Refactoring Pattern**
For each test file (22-36):

1. **Imports**:
   ```go
   // ❌ REMOVE
   - K8sTestClient types
   - gateway package (for StartTestGateway)

   // ✅ ADD
   + net/http (for HTTP requests)
   + bytes (for request bodies)
   ```

2. **BeforeEach Setup**:
   ```go
   // ❌ REMOVE
   - testClient := SetupK8sTestClient(ctx)
   - gatewayServer, err := StartTestGateway(ctx, testClient, dataStorageURL)
   - EnsureTestNamespace(ctx, testClient, testNamespace)
   - RegisterTestNamespace(testNamespace)

   // ✅ REPLACE WITH
   + k8sClient := getKubernetesClient() // From deduplication_helpers.go
   + // Use gatewayURL from suite (already deployed)
   + // Create namespace directly with k8sClient
   + ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
   + Expect(k8sClient.Create(ctx, ns)).To(Succeed())
   ```

3. **Test Body**:
   ```go
   // ❌ REMOVE
   - payload := GeneratePrometheusAlert(PrometheusAlertOptions{...})
   - resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)

   // ✅ REPLACE WITH
   + payload := createPrometheusWebhookPayload(PrometheusAlertPayload{...})
   + req, _ := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
   + req.Header.Set("Content-Type", "application/json")
   + req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
   + resp, err := http.DefaultClient.Do(req)
   ```

4. **AfterEach Cleanup**:
   ```go
   // ❌ REMOVE
   - if gatewayServer != nil { gatewayServer.Stop(ctx) }
   - k8sClient.Cleanup(ctx)

   // ✅ REPLACE WITH
   + // Delete namespace directly
   + ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
   + _ = k8sClient.Delete(ctx, ns)
   ```

---

## 📝 **AFFECTED FILES (15 Total)**

All files need the same pattern of changes:

| File | Current Pattern | Target Pattern | Complexity |
|------|----------------|----------------|------------|
| `22_audit_errors_test.go` | Integration | E2E | Medium |
| `23_audit_emission_test.go` | Integration | E2E | Medium |
| `24_audit_signal_data_test.go` | Integration | E2E | Medium |
| `25_cors_test.go` | Integration | E2E | Simple |
| `26_error_classification_test.go` | Integration | E2E | Medium |
| `27_error_handling_test.go` | Integration | E2E | Medium |
| `28_graceful_shutdown_test.go` | Integration | E2E | Complex |
| `29_k8s_api_failure_test.go` | Integration | E2E | Medium |
| `30_observability_test.go` | Integration | E2E | Simple |
| `31_prometheus_adapter_test.go` | Integration | E2E | Medium |
| `32_service_resilience_test.go` | Integration | E2E | Complex |
| `33_webhook_integration_test.go` | Integration | E2E | Simple |
| `34_status_deduplication_test.go` | Integration | E2E | Medium |
| `35_deduplication_edge_cases_test.go` | Integration | E2E | Medium |
| `36_deduplication_state_test.go` | Integration | E2E | Simple |

---

## ⏱️ **ESTIMATED FIX TIME**

- **Per File**: 8-12 minutes (simple), 12-20 minutes (medium), 20-30 minutes (complex)
- **Total**: ~2.5-3 hours for all 15 files
- **Validation**: 30 minutes (compile + run subset)
- **Total**: ~3-3.5 hours

---

## 🎯 **FIX WORKFLOW**

### **Phase 1: Simple Files (30 min)**
1. `25_cors_test.go` (CORS enforcement)
2. `30_observability_test.go` (metrics/health)
3. `33_webhook_integration_test.go` (webhook patterns)
4. `36_deduplication_state_test.go` (deduplication state)

### **Phase 2: Medium Files (90 min)**
5. `22_audit_errors_test.go` (audit error handling)
6. `23_audit_emission_test.go` (audit event emission)
7. `24_audit_signal_data_test.go` (audit signal data)
8. `26_error_classification_test.go` (error types)
9. `27_error_handling_test.go` (error handling)
10. `29_k8s_api_failure_test.go` (K8s API failures)
11. `31_prometheus_adapter_test.go` (Prometheus adapter)
12. `34_status_deduplication_test.go` (status-based dedup)
13. `35_deduplication_edge_cases_test.go` (dedup edge cases)

### **Phase 3: Complex Files (45 min)**
14. `28_graceful_shutdown_test.go` (shutdown behavior)
15. `32_service_resilience_test.go` (resilience patterns)

### **Phase 4: Validation (30 min)**
- Compile all tests: `go test -c ./test/e2e/gateway/...`
- Run simple test: `make test-e2e-gateway FOCUS="CORS"`
- Fix any remaining issues

---

## ✅ **SUCCESS CRITERIA**

1. ✅ All 15 test files compile without errors
2. ✅ Tests use E2E patterns (HTTP → deployed Gateway)
3. ✅ Tests create/cleanup namespaces using Kind cluster client
4. ✅ No references to integration test helpers
5. ✅ At least 1 test runs successfully (smoke test)

---

## 📚 **REFERENCE**

- **E2E Helpers**: `test/e2e/gateway/deduplication_helpers.go`
- **E2E Suite**: `test/e2e/gateway/gateway_e2e_suite_test.go`
- **Existing E2E Pattern**: `test/e2e/gateway/02_state_based_deduplication_test.go`
- **Integration Helpers**: `test/integration/gateway/helpers.go` (DON'T use in E2E)

---

**Created**: 2026-01-10 23:15 EST
**Priority**: P0 - Blocking E2E test execution
**Next**: Start with Phase 1 (simple files)
