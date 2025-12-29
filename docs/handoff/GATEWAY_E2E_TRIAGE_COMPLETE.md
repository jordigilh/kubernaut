# Gateway E2E Triage - Complete Analysis

**Date**: 2025-12-15
**Team**: Gateway
**Status**: ⚠️ **BLOCKED - Requires Pod Log Analysis**
**Issue**: Gateway pod fails readiness check after 180 seconds, cause unknown without logs

---

## ✅ **Successfully Fixed Issues**

### **1. Gateway Integration Test - Data Storage Field Mapping**
**Files Fixed**:
- `pkg/datastorage/repository/audit_events_repository.go`
- `pkg/datastorage/query/audit_events_builder.go`

**Problem**: Data Storage query endpoint wasn't selecting/mapping critical audit fields (`event_version`, `namespace`, `cluster_name`)

**Fix**:
- Added `Version` field to repository struct
- Updated SQL SELECT to include `event_version`, `namespace`, `cluster_name`
- Fixed JSON tags to match OpenAPI spec (`namespace` not `resource_namespace`)
- Increased `rows.Scan()` to include new columns

**Result**: ✅ **Gateway integration tests: 96/96 PASSING (100%)**

---

### **2. Incorrect Dockerfile Path**
**File Fixed**: `test/infrastructure/gateway_e2e.go:334`

**Problem**: Infrastructure code was pointing to wrong Dockerfile
- Old: `Dockerfile.gateway` (deleted, had hardcoded Rego policy)
- Correct: `docker/gateway-ubi9.Dockerfile` (proper externalized config)

**Fix**: Updated build command to use `docker/gateway-ubi9.Dockerfile`

---

### **3. Readiness Probe Timeouts**
**File Fixed**: `test/e2e/gateway/gateway-deployment.yaml`

**Problem**: Readiness probe had insufficient timeout for RBAC propagation

**Changes**:
```yaml
# BEFORE
initialDelaySeconds: 5
timeoutSeconds: 3
failureThreshold: 3

# AFTER
initialDelaySeconds: 30
timeoutSeconds: 5
failureThreshold: 6
```

**Kubectl wait timeout**: Increased from 120s → 180s

---

### **4. Generated DeepCopy Code Out of Sync**
**File Fixed**: `api/remediation/v1alpha1/zz_generated.deepcopy.go`

**Problem**: `BlockReason` field changed from `*string` to `string` but generated code wasn't updated

**Fix**: Ran `make generate` to regenerate DeepCopy code

---

## ❌ **Still Failing: Gateway Pod Readiness**

### **Symptom**
Gateway pod deployed successfully but **never becomes ready** after 180+ seconds:
```
Waiting for Gateway pod (may take up to 3 minutes for RBAC propagation)...
error: timed out waiting for the condition on pods/gateway-8444c96fb6-gz78r
```

### **What's Working**
✅ **Gateway Docker image**: Builds successfully
✅ **Gateway Docker image**: Loads into Kind cluster successfully
✅ **Gateway deployment**: Creates pod successfully
✅ **PostgreSQL**: Running
✅ **Redis**: Running
✅ **Data Storage**: Deployed and running
✅ **RBAC**: ServiceAccount, ClusterRole, ClusterRoleBinding all applied

### **What's Failing**
❌ **Gateway readiness check**: `/ready` endpoint consistently fails for 180+ seconds

### **Readiness Check Logic**
From `pkg/gateway/server.go:891`:
```go
// Check Kubernetes API connectivity by listing namespaces
namespaceList := &corev1.NamespaceList{}
if err := s.ctrlClient.List(ctx, namespaceList, client.Limit(1)); err != nil {
    // Return 503 Service Unavailable
}
```

### **Possible Causes** (Unverified)
1. **RBAC permissions not propagating** (unlikely after 180s)
2. **Gateway fails to start** due to configuration error
3. **Gateway crashes** before readiness check completes
4. **Network issue** - can't reach Kubernetes API
5. **Missing required configuration** that prevents startup

---

## 🚫 **BLOCKER: Need Pod Logs**

**Cannot proceed without examining Gateway pod logs during the timeout period.**

### **Required Investigation**
To diagnose the actual failure, need to:

1. **Keep cluster alive during failure**:
   ```bash
   export KUBECONFIG=~/.kube/gateway-e2e-config
   kubectl get pods -n kubernaut-system -w
   ```

2. **Check pod logs in real-time**:
   ```bash
   kubectl logs -f gateway-xxx -n kubernaut-system
   ```

3. **Check pod events**:
   ```bash
   kubectl describe pod gateway-xxx -n kubernaut-system | grep -A 20 "Events:"
   ```

4. **Test readiness endpoint manually**:
   ```bash
   kubectl exec gateway-xxx -n kubernaut-system -- curl -v http://localhost:8080/ready
   ```

---

## 📋 **Files Modified**

| File | Change | Status |
|------|--------|--------|
| `pkg/datastorage/repository/audit_events_repository.go` | Added Version field, fixed JSON tags | ✅ Verified |
| `pkg/datastorage/query/audit_events_builder.go` | Updated SQL SELECT query | ✅ Verified |
| `test/infrastructure/gateway_e2e.go` | Fixed Dockerfile path | ✅ Applied |
| `test/e2e/gateway/gateway-deployment.yaml` | Increased readiness timeouts | ✅ Applied |
| `api/remediation/v1alpha1/zz_generated.deepcopy.go` | Regenerated | ✅ Applied |
| `Dockerfile.gateway` | Deleted (wrong file) | ✅ Removed |

---

## 📊 **Test Results**

### **✅ Gateway Integration Tests**
```
✅ 96/96 tests passing (100%)
✅ BR-GATEWAY-190: signal.received audit event - PASSING
✅ BR-GATEWAY-191: signal.deduplicated audit event - PASSING
⏱️  Test duration: ~70 seconds
```

### **❌ Gateway E2E Tests**
```
❌ 0/24 tests run (infrastructure setup failed)
❌ Gateway pod readiness timeout after 180 seconds
⏱️  Test duration: ~6 minutes (all spent waiting for pod)
```

---

## 🎯 **Recommendations**

### **Option A: Manual Debug Cluster** (Recommended)
1. Create persistent Kind cluster:
   ```bash
   cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
   export KUBECONFIG=~/.kube/gateway-debug.kubeconfig

   # Run infrastructure setup manually with pauses
   kind create cluster --name gateway-debug --kubeconfig $KUBECONFIG
   kubectl apply -f config/crd/bases/kubernaut.ai_remediationrequests.yaml
   kubectl create namespace kubernaut-system

   # Deploy infrastructure manually...
   # Then check Gateway logs when it fails
   ```

2. Watch Gateway pod startup in real-time
3. Capture logs and events
4. Share findings for diagnosis

### **Option B: Add Debug Logging to Readiness Handler**
Temporarily add detailed logging to `pkg/gateway/server.go:891`:
```go
func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
    s.logger.Info("Readiness check started")

    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    s.logger.Info("Attempting to list namespaces for readiness check")
    namespaceList := &corev1.NamespaceList{}
    if err := s.ctrlClient.List(ctx, namespaceList, client.Limit(1)); err != nil {
        s.logger.Error(err, "Readiness check FAILED: Kubernetes API not reachable",
            "error_type", fmt.Sprintf("%T", err),
            "error_message", err.Error())
        // ... return 503
    }

    s.logger.Info("Readiness check PASSED", "namespace_count", len(namespaceList.Items))
    // ... return 200
}
```

### **Option C: Simplify Readiness Check Temporarily**
Comment out K8s API check to isolate issue:
```go
// Skip K8s check for debugging
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
return
```

If this makes the pod ready, the issue is specifically with K8s API connectivity or RBAC.

---

## 🔗 **Related Issues**

- ✅ **Gateway Integration Tests**: Fixed Data Storage field mapping
- ✅ **Gateway Audit Tests**: BR-GATEWAY-190 & BR-GATEWAY-191 now passing
- ❌ **Gateway E2E Tests**: Blocked by pod readiness failure

---

## 📞 **Next Steps**

**User Decision Required**:
1. Which debugging approach to take (A/B/C above)?
2. Can you provide access to check pod logs during timeout?
3. Should I proceed with Option B (add debug logging)?

**Confidence**: **20%** - Cannot diagnose without pod logs

**Priority**: **P1** - Blocks all Gateway E2E test execution

**Estimated Time to Fix**: 30-60 minutes once logs are available

---

**Last Updated**: 2025-12-15 09:42 EST
**Status**: Awaiting pod log analysis to identify root cause



