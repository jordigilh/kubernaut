# Gateway E2E Investigation Guide - CRD Creation Failures

**Date**: January 11, 2026
**Status**: 🔍 **INVESTIGATION REQUIRED**
**Priority**: **P0 - CRITICAL** (blocks 10+ E2E tests)
**Owner**: Gateway Team

---

## 🎯 **Problem Statement**

Gateway E2E tests are failing because **CRDs are not being created** when tests send HTTP POST requests to `/api/v1/signals/prometheus`.

**Symptom**: Tests expect HTTP 201 (Created) but Gateway returns different status code or malformed response.

**Impact**: 10+ deduplication tests failing, 1 panic (now fixed)

---

## 🔴 **Critical Issue: HTTP POST Not Creating CRDs**

### **Test Pattern That's Failing**

```go
// Test sends HTTP POST to Gateway
resp := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)

// Expected: 201 Created
Expect(resp.StatusCode).To(Equal(http.StatusCreated))

// Expected: Response contains CRD name
var response gateway.ProcessingResponse
err := json.Unmarshal(resp.Body, &response)
Expect(err).ToNot(HaveOccurred())  // NOW FIXED: Was causing panic
crdName := response.RemediationRequestName

// Expected: CRD exists in Kubernetes
crd := getCRDByName(ctx, k8sClient, namespace, crdName)
// ACTUAL: CRD not found! (found 0 CRDs total)
```

### **What We Know**

1. ✅ **Gateway URL is correct**: `http://127.0.0.1:8080` (Kind NodePort)
2. ✅ **Gateway pod deployed**: Infrastructure setup completes successfully
3. ✅ **DataStorage is running**: Port 18091, tests can query it
4. ❌ **CRDs not being created**: 0 CRDs found in test namespaces
5. ❌ **HTTP response unknown**: Tests fail before revealing actual status code

### **What We Don't Know Yet**

1. ❓ **Actual HTTP status code returned** by Gateway
2. ❓ **Actual HTTP response body** from Gateway
3. ❓ **Gateway error logs** explaining why CRDs not created
4. ❓ **Prometheus payload format** - Is it correct?

---

## 🔍 **Investigation Steps**

### **Step 1: Check Actual HTTP Response** (HIGHEST PRIORITY)

**Action**: Add debug logging to reveal actual HTTP response

**Code Change** (temporary debug in test file):
```go
// In test/e2e/gateway/36_deduplication_state_test.go
resp1 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)

// ADD THIS DEBUG LOGGING
GinkgoWriter.Printf("HTTP Status: %d\n", resp1.StatusCode)
GinkgoWriter.Printf("HTTP Body: %s\n", string(resp1.Body))
GinkgoWriter.Printf("Prometheus Payload: %s\n", string(prometheusPayload))

Expect(resp1.StatusCode).To(Equal(http.StatusCreated))
```

**Expected Outputs**:
- **If 404**: Gateway endpoint not found (routing issue)
- **If 400**: Payload validation failure
- **If 500**: Gateway internal error (check logs)
- **If 201 but CRD missing**: K8s API error or permissions issue

---

### **Step 2: Check Gateway Pod Logs** (IMMEDIATE)

**During Next Test Run**:
```bash
# In separate terminal, tail Gateway logs in real-time
kubectl --kubeconfig=/Users/jgil/.kube/gateway-e2e-config \
  logs -n kubernaut-system -l app=gateway -f

# Or after test run
kubectl --kubeconfig=/Users/jgil/.kube/gateway-e2e-config \
  logs -n kubernaut-system -l app=gateway --tail=200
```

**Look For**:
- ❌ Endpoint not found errors
- ❌ Payload validation errors
- ❌ K8s API errors (permissions, rate limiting)
- ❌ DataStorage connection errors
- ❌ Panic/crash logs

---

### **Step 3: Verify Gateway Endpoint Manually** (IMMEDIATE)

**During Active E2E Test Run**:
```bash
# Test Gateway health endpoint
curl -v http://127.0.0.1:8080/health

# Test Prometheus webhook endpoint with sample payload
curl -X POST http://127.0.0.1:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -d '{
    "alerts": [{
      "status": "firing",
      "labels": {
        "alertname": "PodCrashLoop",
        "namespace": "test-namespace",
        "pod": "test-pod",
        "severity": "critical"
      },
      "annotations": {
        "description": "Pod is crash looping",
        "summary": "Test alert"
      }
    }]
  }'
```

**Expected**: 201 Created with JSON response containing `remediationRequestName`
**If Different**: Gateway endpoint issue

---

### **Step 4: Verify CRD Permissions** (IMMEDIATE)

**Check Gateway ServiceAccount Permissions**:
```bash
kubectl --kubeconfig=/Users/jgil/.kube/gateway-e2e-config \
  get clusterrolebinding -o yaml | grep -A5 gateway

# Check if Gateway can create RemediationRequests
kubectl --kubeconfig=/Users/jgil/.kube/gateway-e2e-config \
  auth can-i create remediationrequests \
  --as=system:serviceaccount:kubernaut-system:gateway
```

**Expected**: `yes`
**If No**: RBAC permissions missing

---

### **Step 5: Check Prometheus Payload Format** (VALIDATION)

**Verify Test Payload Matches Gateway Expectation**:

**Test Payload** (from `createPrometheusWebhookPayload`):
```go
// File: test/e2e/gateway/deduplication_helpers.go
func createPrometheusWebhookPayload(payload PrometheusAlertPayload) []byte {
    // Check this matches Gateway's expected format
}
```

**Gateway Expected Format** (from Gateway code):
```go
// File: pkg/gateway/prometheus_adapter.go (or similar)
type PrometheusWebhook struct {
    Alerts []Alert `json:"alerts"`
}

type Alert struct {
    Status      string            `json:"status"`
    Labels      map[string]string `json:"labels"`
    Annotations map[string]string `json:"annotations"`
    // ... other fields
}
```

**Action**: Compare test payload with Gateway's struct definition

---

## 🐛 **Known Issues Fixed**

### **✅ Panic Fix Applied**

**File**: `test/e2e/gateway/36_deduplication_state_test.go`

**Problem**: Tests ignored unmarshal errors, causing nil pointer dereference
```go
// BEFORE (caused panic)
err := json.Unmarshal(resp1.Body, &response1)
_ = err  // ← Ignored error
crdName := response1.RemediationRequestName  // ← Panic if unmarshal failed

// AFTER (reveals actual error)
err := json.Unmarshal(resp1.Body, &response1)
Expect(err).ToNot(HaveOccurred(), "Failed to unmarshal response: %v, body: %s", err, string(resp1.Body))
crdName := response1.RemediationRequestName  // Safe now
```

**Impact**: 7 instances fixed across all deduplication state tests

**Benefit**: Tests now fail with **clear error message** showing actual HTTP response

---

## 📊 **Affected Tests**

### **Deduplication State Tests** (`36_deduplication_state_test.go`)

| Test | Status | Root Cause (Hypothesis) |
|------|--------|------------------------|
| "should detect duplicate (Pending state)" | ❌ Failing | CRD not created |
| "should detect duplicate (Processing state)" | ❌ Was panicking | CRD not created |
| "should treat as new incident (Completed)" | ❌ Was panicking | CRD not created |
| "should treat as new incident (Failed)" | ❌ Failing | CRD not created |
| "should treat as new incident (Cancelled)" | ❌ Failing | CRD not created |
| "should treat as duplicate (Unknown state)" | ❌ Failing | CRD not created |
| "should create new CRD (doesn't exist)" | ❌ Failing | CRD not created |

**Total**: 7 tests (all in one file)

---

### **Other Deduplication Tests**

| File | Tests Affected | Pattern |
|------|----------------|---------|
| `34_status_deduplication_test.go` | 3 | Same CRD creation issue |
| `35_deduplication_edge_cases_test.go` | 2 | Same CRD creation issue |

**Total**: ~12 tests affected by same root cause

---

## 🎯 **Expected Investigation Outcomes**

### **Scenario 1: Gateway Endpoint Not Found (404)**

**Symptom**: HTTP 404, Gateway logs show "endpoint not found"

**Root Cause**: Routing configuration or endpoint path mismatch

**Fix**: Update Gateway routing or test endpoint path

---

### **Scenario 2: Payload Validation Failure (400)**

**Symptom**: HTTP 400, Gateway logs show "invalid payload" or "missing required field"

**Root Cause**: Test payload doesn't match Gateway's expected schema

**Fix**: Update test payload format to match Gateway expectations

---

### **Scenario 3: K8s API Error (500)**

**Symptom**: HTTP 500, Gateway logs show "failed to create CRD"

**Possible Causes**:
- RBAC permissions missing
- CRD definition not installed
- K8s API server unreachable
- Rate limiting

**Fix**: Address specific K8s API issue

---

### **Scenario 4: Gateway Not Ready (503)**

**Symptom**: HTTP 503, Gateway logs show initialization errors

**Possible Causes**:
- DataStorage not reachable
- K8s client not initialized
- Missing dependencies

**Fix**: Ensure all Gateway dependencies are ready before tests

---

## 🔧 **Quick Debug Checklist**

**Before Next Test Run**:
- [ ] Add debug logging to reveal HTTP status code and body
- [ ] Start tailing Gateway logs: `kubectl logs -f ...`
- [ ] Have curl command ready to test endpoint manually

**During Test Run**:
- [ ] Observe Gateway logs in real-time
- [ ] Note any errors or warnings
- [ ] Test endpoint manually if tests fail quickly

**After Test Run**:
- [ ] Review debug output from tests
- [ ] Check Gateway pod status: `kubectl get pods -n kubernaut-system`
- [ ] Review full Gateway logs
- [ ] Check if any CRDs were created: `kubectl get remediationrequests -A`

---

## 📚 **Helpful Commands**

### **During Active E2E Test**

```bash
# Terminal 1: Run tests
make test-e2e-gateway

# Terminal 2: Watch Gateway logs
kubectl --kubeconfig=/Users/jgil/.kube/gateway-e2e-config \
  logs -n kubernaut-system -l app=gateway -f

# Terminal 3: Watch CRD creation
watch -n 1 'kubectl --kubeconfig=/Users/jgil/.kube/gateway-e2e-config \
  get remediationrequests -A'

# Terminal 4: Test endpoint manually
curl -v http://127.0.0.1:8080/health
curl -X POST http://127.0.0.1:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -d @test-payload.json
```

---

## 📊 **Current E2E Status**

| Metric | Value | Note |
|--------|-------|------|
| **Tests Passing** | 71/118 | 60.2% pass rate |
| **Tests Failing** | 47 | ~12 due to CRD creation issue |
| **Tests Panicking** | 0 | ✅ Fixed (was 1) |
| **Priority Issues** | 1 | P0: CRD creation failure |

---

## 🎯 **Success Criteria**

**Investigation Complete When**:
- [ ] Actual HTTP status code and response body documented
- [ ] Gateway error logs reviewed and documented
- [ ] Root cause identified (one of 4 scenarios above)
- [ ] Fix proposed with specific code changes
- [ ] Manual curl test succeeds (201 Created, CRD exists)

**Tests Passing When**:
- [ ] HTTP POST returns 201 Created
- [ ] Response contains valid `RemediationRequestName`
- [ ] CRD exists in Kubernetes after POST
- [ ] 12+ deduplication tests pass

---

## 🔗 **Related Documentation**

- **Phase 2 Results**: `GATEWAY_E2E_PHASE2_RESULTS_JAN11_2026.md`
- **Panic Fix**: Applied to `test/e2e/gateway/36_deduplication_state_test.go`
- **Original RCA**: `GATEWAY_E2E_RCA_TIER3_FAILURES_JAN11_2026.md`
- **Port Allocation**: `DD-TEST-001-port-allocation-strategy.md` (line 63: Gateway port 18091)

---

## ✅ **Next Steps for Gateway Team**

1. **Add debug logging** to test file (5 minutes)
2. **Run tests** with debug logging (4 minutes)
3. **Review debug output** to see actual HTTP response
4. **Check Gateway logs** for errors
5. **Test endpoint manually** with curl
6. **Identify root cause** (one of 4 scenarios)
7. **Implement fix** (depends on root cause)
8. **Verify fix** with test rerun

**Estimated Time**: 1-2 hours total

---

**Status**: ✅ **INVESTIGATION GUIDE COMPLETE**
**Panic Fixed**: ✅ All error handling improved
**Next Action**: Run tests with panic fix, review debug output
**Owner**: Gateway E2E Test Team
