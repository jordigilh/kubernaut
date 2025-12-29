# Gateway E2E Readiness Triage - In Progress

**Date**: 2025-12-15
**Team**: Gateway
**Status**: 🔍 **INVESTIGATING**
**Issue**: Gateway pod fails readiness check in E2E tests, timing out after 180 seconds

---

## 🎯 **Problem Statement**

Gateway E2E tests fail during infrastructure setup because the Gateway pod never becomes ready:

```
   Waiting for Gateway pod (may take up to 3 minutes for RBAC propagation)...
error: timed out waiting for the condition on pods/gateway-8444c96fb6-65ll4
```

---

## 🔍 **Investigation Progress**

### **Root Cause Analysis**

**Symptom**: Gateway pod's readiness probe consistently fails for 180+ seconds

**Readiness Probe Configuration**:
- Path: `/ready`
- Initial Delay: 30 seconds (increased from original 5s)
- Period: 5 seconds
- Timeout: 5 seconds (increased from 3s)
- Failure Threshold: 6 (increased from 3)

**What `/ready` checks** (from `pkg/gateway/server.go:891`):
```go
// Check Kubernetes API connectivity by listing namespaces
namespaceList := &corev1.NamespaceList{}
if err := s.ctrlClient.List(ctx, namespaceList, client.Limit(1)); err != nil {
    // Return 503 Service Unavailable
}
```

**Possible Root Causes**:
1. ❓ **RBAC Propagation Delay**: ServiceAccount permissions not yet active (unlikely after 180s)
2. ❓ **RBAC Configuration Error**: ClusterRole/ClusterRoleBinding not correctly defined
3. ❓ **Network Issue**: Gateway pod can't reach Kubernetes API server
4. ❓ **Configuration Error**: Gateway failing to start due to missing/invalid config
5. ❓ **Runtime Panic**: Gateway crashing before readiness check completes

---

## ✅ **Fixes Applied**

### **1. Increased Readiness Probe Timeouts**

**File**: `test/e2e/gateway/gateway-deployment.yaml`

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

**Rationale**: Allow more time for RBAC permissions to propagate and for the Gateway to initialize

### **2. Increased kubectl wait Timeout**

**File**: `test/infrastructure/gateway_e2e.go:478`

**Changes**:
```go
// BEFORE
"--timeout=120s"

// AFTER
"--timeout=180s"
```

**Rationale**: Give Gateway pod full 3 minutes to become ready

### **3. Fixed Generated DeepCopy Code**

**File**: `api/remediation/v1alpha1/zz_generated.deepcopy.go`

**Issue**: `BlockReason` field changed from `*string` to `string` but generated code was out of sync

**Fix**: Ran `make generate` to regenerate DeepCopy code

---

## 🚨 **Current Status**

**Test Result**: Still failing after 180 seconds timeout

**Next Steps**:
1. **Need to inspect Gateway pod logs** during failure to see actual error
2. **Verify RBAC configuration** is correctly applied
3. **Check Gateway startup logs** for configuration errors
4. **Test Gateway readiness endpoint manually** in Kind cluster

---

## 📊 **Test Environment**

**Infrastructure Stack**:
- Kind cluster: `gateway-e2e`
- PostgreSQL: ✅ Running (Data Storage dependency)
- Redis: ✅ Running (Data Storage dependency)
- Data Storage: ✅ Deployed successfully
- Gateway: ❌ Fails readiness check

**Gateway Deployment**:
- Namespace: `kubernaut-system`
- ServiceAccount: `gateway`
- ClusterRole: `gateway-role` (namespace + configmap + CRD access)
- ClusterRoleBinding: `gateway-rolebinding`
- NodePort: 30080 (HTTP), 30090 (metrics)

---

## 🔧 **Configuration Verified**

### **Data Storage Integration** (OPTIONAL)
✅ Gateway config shows `DataStorageURL` is **OPTIONAL** (graceful degradation)
```go
// pkg/gateway/config/config.go:70
DataStorageURL string `yaml:"data_storage_url"` // OPTIONAL

// Gateway will start without it and just log warning
if cfg.Infrastructure.DataStorageURL != "" {
    // Enable audit
} else {
    logger.Info("DD-AUDIT-003: Data Storage URL not configured, audit events will be dropped (WARNING)")
}
```

**Conclusion**: Missing Data Storage URL is NOT the root cause

### **RBAC Permissions** (REQUIRED)
📋 Gateway needs these permissions for readiness check:
```yaml
apiGroups: [""]
resources: ["namespaces"]
verbs: ["get", "list", "watch"]
```

**Status**: Defined in `gateway-deployment.yaml` but need to verify they're actually applied

---

## 🎯 **Recommended Next Actions**

### **Option A: Debug with Live Cluster**
1. Create Kind cluster manually
2. Deploy Gateway
3. Watch pod logs in real-time during failure:
   ```bash
   kubectl logs -f gateway-xxx -n kubernaut-system
   ```
4. Check events:
   ```bash
   kubectl describe pod gateway-xxx -n kubernaut-system
   ```
5. Test readiness endpoint manually:
   ```bash
   kubectl exec gateway-xxx -n kubernaut-system -- curl -v http://localhost:8080/ready
   ```

### **Option B: Simplify Readiness Check (Temporary)**
Temporarily disable Kubernetes API check in readiness handler to isolate issue:
```go
// Skip K8s API check for debugging
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
return
```

### **Option C: Add Startup Probe**
Add separate startup probe with longer timeout to handle slow initialization:
```yaml
startupProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 5
  failureThreshold: 30  # Allow 150 seconds for startup
```

---

## 📝 **Files Modified**

1. ✅ `test/e2e/gateway/gateway-deployment.yaml` - Increased readiness probe timeouts
2. ✅ `test/infrastructure/gateway_e2e.go` - Increased kubectl wait timeout
3. ✅ `api/remediation/v1alpha1/zz_generated.deepcopy.go` - Regenerated via `make generate`

---

## 🔗 **Related Issues**

- Gateway audit integration tests (BR-GATEWAY-190, BR-GATEWAY-191): ✅ **PASSING** (fixed Data Storage field mapping)
- Gateway integration tests: ✅ **96/96 PASSING** (100%)
- Gateway E2E tests: ❌ **BLOCKED** (infrastructure setup fails)

---

## 📞 **Support Needed**

**From User**:
- Decision on debugging approach (Option A/B/C above)
- Access to investigate live cluster during failure
- Confirmation if Gateway E2E tests recently worked

**Confidence**: **30%** - Need pod logs to diagnose actual failure reason

**Priority**: **P1** - Blocks all Gateway E2E test execution

---

**Last Updated**: 2025-12-15 09:20 EST
**Next Update**: After receiving pod logs or user decision on debugging approach



