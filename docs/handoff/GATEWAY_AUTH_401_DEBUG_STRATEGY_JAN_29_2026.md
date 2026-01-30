# Gateway Integration Auth 401 Debug Strategy

**Date:** January 29, 2026  
**Status:** 🔍 DEBUGGING IN PROGRESS - Container cleanup didn't fix issue  
**Authority:** DD-AUTH-014 (Middleware-Based SAR Authentication)

---

## 🚨 **Current Status**

**After Container Cleanup Test:**
- ❌ **Still 16 audit failures**
- ❌ **Still 50 HTTP 401 errors**
- ✅ **Processing tests pass (10/10)**
- ❌ **Audit emission tests fail (73/90)**

**Key Finding:** Container reuse was NOT the problem.

---

## ✅ **What We Know Works (AIAnalysis)**

**Test Results:**
- ✅ 58/59 passed (98%)
- ✅ Zero HTTP 401 errors
- ✅ ServiceAccount authentication functional
- ✅ DataStorage SAR middleware validates tokens
- ✅ Audit events written successfully

**Evidence:**
```
✅ [Process 4] Authenticated DataStorage clients created
"buffer_count":18,"written_count":18,"dropped_count":0
```

---

## ❌ **What Doesn't Work (Gateway)**

**Test Results:**
- ⚠️ 73/90 passed (81%)
- ❌ 50 HTTP 401 errors
- ❌ All 16 audit failures due to authentication
- ✅ Gateway processing logic works (non-audit tests pass)

**Evidence:**
```
ERROR audit-store Failed to write audit batch
{"error": "Data Storage Service returned status 401: HTTP 401 error"}
```

---

## 🔍 **Why Same Code Produces Different Results?**

### **Identical Code Paths**

Both Gateway and AIAnalysis use:
1. ✅ Same `CreateIntegrationServiceAccountWithDataStorageAccess()` helper
2. ✅ Same `ServiceAccountTransport` for Bearer tokens
3. ✅ Same `StartDSBootstrap()` for infrastructure
4. ✅ Same envtest kubeconfig mounting
5. ✅ Same Phase 1/Phase 2 setup pattern

### **Possible Differences**

| Aspect | AIAnalysis | Gateway | Impact? |
|--------|-----------|---------|---------|
| **DataStorage Port** | 18095 | 18091 | Unlikely |
| **ServiceAccount Name** | aianalysis-ds-client | gateway-integration-sa | Possible |
| **Test Suite Size** | 59 tests | 90 tests | Unlikely |
| **Extra Services** | Mock LLM + HAPI | None | Timing? |
| **envtest Timing** | Started ~5s before DS | Started ~5s before DS | Timing? |

---

## 🎯 **Debug Hypotheses (Ordered by Likelihood)**

### **Hypothesis 1: Token Validation Timing Issue (60% confidence)**

**Theory:** DataStorage starts before envtest is fully ready to validate tokens.

**Evidence:**
- Gateway: envtest starts at 17:47:32, DataStorage starts immediately after
- AIAnalysis: Similar timing, but extra services give envtest more "settle time"
- 401 errors occur 1 second after tests start (timer tick = 1s flush interval)

**Test:**
```bash
# Add 5-second sleep between envtest start and DataStorage start
# in test/integration/gateway/suite_test.go Phase 1

time.Sleep(5 * time.Second) // Give envtest time to settle
dsInfra, err = infrastructure.StartDSBootstrap(...)
```

**Expected:** If timing is the issue, this should fix the 401s.

---

### **Hypothesis 2: envtest API Server Not Responding to TokenReview (30% confidence)**

**Theory:** Gateway's envtest isn't responding to TokenReview requests from DataStorage.

**Evidence:**
- AIAnalysis envtest validates tokens successfully
- Gateway envtest created identically
- No obvious differences in setup

**Test:**
```bash
# During Gateway test run, manually test TokenReview from DataStorage container
podman exec gateway_datastorage_test kubectl --kubeconfig=/envtest/kubeconfig auth can-i list pods --as=system:serviceaccount:default:gateway-integration-sa
```

**Expected:** Should return "yes" if RBAC is working, "error" if envtest not responding.

---

### **Hypothesis 3: ServiceAccount Token Invalid/Not Propagated (5% confidence)**

**Theory:** Token from Phase 1 isn't reaching Phase 2 processes correctly.

**Evidence Against:**
- Logs show "✅ Authenticated DataStorage client created" in all 12 processes
- Same Phase 1/Phase 2 pattern works for AIAnalysis
- Token is a simple byte slice passed via Ginkgo

**Test:**
```bash
# Add debug logging in Phase 2 to print token length
logger.Info(fmt.Sprintf("[Process %d] Token length: %d bytes", processNum, len(data)))
```

**Expected:** All processes should show same token length (~650-700 bytes).

---

### **Hypothesis 4: DataStorage Middleware Not Loading Kubeconfig (5% confidence)**

**Theory:** DataStorage container not loading envtest kubeconfig for SAR middleware.

**Evidence Against:**
- Logs show "🔐 Mounting envtest kubeconfig"
- Same DataStorage image/config works for AIAnalysis
- StartDSBootstrap code identical

**Test:**
```bash
# During Gateway test run, check DataStorage container environment
podman exec gateway_datastorage_test env | grep -i KUBECONFIG
podman exec gateway_datastorage_test ls -la /envtest/
```

**Expected:** Should show KUBECONFIG env var and kubeconfig file present.

---

## 🔧 **Recommended Debug Plan**

### **Step 1: Add Timing Sleep (5 minutes - Highest Priority)**

**Rationale:** Easiest to test, 60% confidence this will fix it.

**Action:**
```go
// In test/integration/gateway/suite_test.go, Phase 1
// After envtest creation, before DataStorage start

logger.Info("[Process 1] ✅ envtest started", "api", sharedTestEnv.Config.Host)
logger.Info("[Process 1] ✅ envtest kubeconfig written", "path", kubeconfigPath)

// Give envtest API server time to fully initialize before DataStorage connects
logger.Info("[Process 1] Waiting for envtest API server to settle...")
time.Sleep(5 * time.Second)

logger.Info("[Process 1] Creating ServiceAccount for DataStorage authentication...")
authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(...)
```

**Run Test:**
```bash
make test-integration-gateway 2>&1 | tee /tmp/integration-gateway-timing-fix.log
```

**If this works:** Document timing requirement, apply to all integration tests  
**If this fails:** Move to Step 2

---

### **Step 2: Compare envtest Configurations (10 minutes)**

**Action:**
```bash
# Extract envtest config from both test runs
grep "envtest started" /tmp/integration-aianalysis-baseline.log
grep "envtest started" /tmp/integration-gateway-clean.log

# Check if there are differences in envtest setup
diff <(grep "envtest\|ServiceAccount\|ClusterRole" /tmp/integration-aianalysis-baseline.log | head -50) \
     <(grep "envtest\|ServiceAccount\|ClusterRole" /tmp/integration-gateway-clean.log | head -50)
```

---

### **Step 3: Manual TokenReview Test (15 minutes)**

**Action:**
```bash
# Start Gateway integration test and pause before audit tests run
# In separate terminal, test TokenReview manually

# Get Gateway envtest kubeconfig
export KUBECONFIG=/Users/jgil/tmp/kubernaut-envtest/envtest-kubeconfig-gateway-integration.yaml

# Verify ServiceAccount exists
kubectl get serviceaccount gateway-integration-sa -n default

# Test TokenReview directly (simulate what DataStorage does)
kubectl create --raw=/apis/authentication.k8s.io/v1/tokenreviews \
  -f <(cat <<EOF
{
  "apiVersion": "authentication.k8s.io/v1",
  "kind": "TokenReview",
  "spec": {
    "token": "$(kubectl get secret -n default $(kubectl get sa gateway-integration-sa -n default -o jsonpath='{.secrets[0].name}') -o jsonpath='{.data.token}' | base64 -d)"
  }
}
EOF
)
```

**Expected:** TokenReview should return `authenticated: true`.

---

### **Step 4: Add Detailed Logging (20 minutes)**

**If Steps 1-3 don't identify the issue, add extensive logging:**

**Gateway suite_test.go Phase 2:**
```go
// After receiving token from Phase 1
saToken := string(data)
logger.Info(fmt.Sprintf("[Process %d] Token received", processNum), 
	"length", len(saToken),
	"prefix", saToken[:min(20, len(saToken))],
	"suffix", saToken[max(0, len(saToken)-20):])

authTransport := testauth.NewServiceAccountTransport(saToken)
logger.Info(fmt.Sprintf("[Process %d] ServiceAccountTransport created", processNum))

dsClient, err = audit.NewOpenAPIClientAdapterWithTransport(
	fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort),
	5*time.Second,
	authTransport,
)
Expect(err).ToNot(HaveOccurred(), "DataStorage client creation must succeed")
logger.Info(fmt.Sprintf("[Process %d] ✅ Authenticated DataStorage client created", processNum))

// Test the client immediately
logger.Info(fmt.Sprintf("[Process %d] Testing DataStorage connectivity...", processNum))
testCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()
_, testErr := dsClient.StoreBatch(testCtx, []*ogenclient.AuditEventRequest{})
if testErr != nil {
	logger.Info(fmt.Sprintf("[Process %d] ⚠️  DataStorage test call failed: %v", processNum, testErr))
} else {
	logger.Info(fmt.Sprintf("[Process %d] ✅ DataStorage test call succeeded", processNum))
}
```

---

## 📊 **Success Criteria**

**Fix is successful when:**
- ✅ All 90 Gateway integration tests pass
- ✅ Zero HTTP 401 errors in logs
- ✅ Audit events written successfully to DataStorage
- ✅ `dropped_count=0` in audit store logs

---

## 🎯 **Next Steps**

**Immediate Action:**
1. **Implement Step 1 (timing sleep)** - 5 minutes
2. **Run Gateway integration tests** - 2 minutes
3. **Analyze results** - 3 minutes

**Total Time:** ~10 minutes for first debug iteration

---

**Status:** Awaiting user decision to proceed with Step 1 (timing sleep fix)
